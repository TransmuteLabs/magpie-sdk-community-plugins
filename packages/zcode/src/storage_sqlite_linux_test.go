package zcode

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type automaticWriter struct {
	input   io.WriteCloser
	output  *json.Decoder
	initial map[string]any
}

func automaticDatabase(t *testing.T) (*automaticWriter, string) {
	t.Helper()
	directory := t.TempDir()
	writerContext, cancelWriter := context.WithCancel(context.Background())
	command := exec.CommandContext(writerContext, "python3", "-I", "-S", "testdata/automatic-history/writer.py", directory)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer cancelWriter()
		input.Close()
		if err := command.Wait(); err != nil {
			t.Error("separate SQLite writer:", err, stderr.String())
		}
	})
	writer := &automaticWriter{input: input, output: json.NewDecoder(bufio.NewReader(output))}
	if err := writer.output.Decode(&writer.initial); err != nil || writer.initial["pid"] == float64(os.Getpid()) {
		t.Fatal("separate WAL writer failed:", err)
	}
	return writer, directory
}

func (writer *automaticWriter) ask(t *testing.T, command string) map[string]any {
	t.Helper()
	if err := json.NewEncoder(writer.input).Encode(command); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := writer.output.Decode(&result); err != nil {
		t.Fatal("separate SQLite writer:", err)
	}
	return result
}

type automaticRoot struct {
	ctx    context.Context
	root   *os.Root
	mu     sync.Mutex
	budget int64
}

func (root *automaticRoot) Open(name string) (*os.File, error) {
	if !root.Family().Allows(name) || root.ctx.Err() != nil {
		return nil, os.ErrPermission
	}
	return root.root.Open(name)
}

func (root *automaticRoot) Consume(count int64) error {
	root.mu.Lock()
	defer root.mu.Unlock()
	root.budget -= count
	if root.ctx.Err() != nil || root.budget < 0 {
		return errors.New("rooted storage lease revoked or budget exhausted")
	}
	return nil
}

func (*automaticRoot) Family() sdk.StorageFamily { return historyDescriptor().Families[0] }

func automaticOpen(t *testing.T, directory string, limits sdk.StorageLimits) (*historySQL, *automaticRoot, error) {
	t.Helper()
	opened, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { opened.Close() })
	root := &automaticRoot{ctx: t.Context(), root: opened, budget: limits.MaxReadBytes}
	reader, err := openHistorySQL(t.Context(), root, limits)
	if err == nil {
		t.Cleanup(func() { reader.Close() })
	}
	return reader, root, err
}

func automaticRequest() sdk.StorageRequest {
	return sdk.StorageRequest{Lease: sdk.StorageReadLease{SourceID: storageSourceID, RootRef: "root-one",
		SnapshotRef: "snapshot-one", FamilyID: "cli", Limits: sdk.DefaultStorageLimits()}, Limit: 20}
}

func TestAutomaticHistoryRealWALProjectionAndSourcePreservation(t *testing.T) {
	writer, directory := automaticDatabase(t)
	request := automaticRequest()
	reader, _, err := automaticOpen(t, directory, request.Lease.Limits)
	if err != nil {
		t.Fatal("rooted WAL transaction:", err)
	}
	listing, err := reader.result(t.Context(), request)
	if err != nil || len(listing.Entries) != 5 || listing.Cut {
		t.Fatal("committed WAL list:", err, listing)
	}
	entries := map[string]sdk.StorageEntry{}
	for _, entry := range listing.Entries {
		entries[entry.Locator.SessionID] = entry
	}
	main := entries["ses_one"]
	if main.Summary.Title != "Committed WAL session" || len(main.Summary.Usage) != 1 ||
		main.Summary.Usage[0].Input != 9007199254740993 || main.Summary.Usage[0].Output != 7 ||
		main.Summary.Usage[0].CacheRead != 11 || main.Summary.Usage[0].CacheWrite != 2 {
		t.Fatal("exact native counters were rounded or cache/reasoning was counted again:", main.Summary)
	}
	child := entries["ses_child"]
	if child.Summary.ParentID != "ses_one" || child.Summary.Title != "Actual child title" ||
		len(child.Summary.Usage) != 1 || child.Summary.Usage[0].Input != 5 {
		t.Fatal("fork copy inflated usage, hid later work or a synthetic user became title:", child.Summary)
	}
	if len(entries["ses_orphan"].Summary.Usage) != 1 ||
		entries["ses_unknown"].Summary.Usage != nil || entries["ses_zero"].Summary.Usage == nil ||
		len(entries["ses_zero"].Summary.Usage) != 1 || entries["ses_zero"].Summary.Usage[0].Input != 0 {
		t.Fatal("orphan/unknown/explicit-zero history counters were conflated")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	reader, _, err = automaticOpen(t, directory, request.Lease.Limits)
	if err != nil {
		t.Fatal(err)
	}
	request.Limit, request.Locator, request.Mode = 0, &main.Locator, "transcript"
	result, err := reader.result(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw(result.Parts))
	thinking, tool, last := strings.Index(body, "first reasoning"), strings.Index(body, "tool fixture"), strings.Index(body, "last block")
	if !result.Cut || thinking < 0 || tool <= thinking || last <= tool ||
		!strings.Contains(body, "[body unavailable]") || !strings.Contains(body, "media unavailable") ||
		strings.Contains(body, "file:///must-not-open") || strings.Contains(body, "opaque-synthetic-signature") {
		t.Fatal("ordered native transcript or opaque external boundaries changed:", body)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	after := writer.ask(t, "state")
	for _, name := range []string{"db", "wal", "shm"} {
		if writer.initial[name] != after[name] {
			t.Fatal("automatic reader wrote original SQLite family:", name)
		}
	}
}

func TestAutomaticHistoryTransactionExcludesConcurrentAndUncommittedWrites(t *testing.T) {
	writer, directory := automaticDatabase(t)
	request := automaticRequest()
	reader, _, err := automaticOpen(t, directory, request.Lease.Limits)
	if err != nil {
		t.Fatal(err)
	}
	writer.ask(t, "append")
	result, err := reader.result(t.Context(), request)
	if err != nil || result.Entries[0].Summary.Title != "Committed WAL session" {
		t.Fatal("concurrent writer changed the pinned read transaction:", err)
	}
	if state := writer.ask(t, "checkpoint"); state["busy"] != float64(1) {
		t.Fatal("separate writer reset WAL under the reader:", state)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if state := writer.ask(t, "checkpoint"); state["busy"] != float64(0) {
		t.Fatal("reader retained checkpoint locks after Close:", state)
	}
	writer.ask(t, "uncommitted")
	reader, _, err = automaticOpen(t, directory, request.Lease.Limits)
	if err != nil {
		t.Fatal(err)
	}
	locator := reader.locator(request.Lease, "ses_one")
	request.Limit, request.Locator, request.Mode = 0, &locator, "transcript"
	transcript, err := reader.result(t.Context(), request)
	if err != nil || strings.Contains(string(raw(transcript)), "uncommitted secret") ||
		!strings.Contains(string(raw(transcript)), "later committed message") {
		t.Fatal("committed/uncommitted WAL boundary changed:", err, transcript)
	}
	reader.Close()
	writer.ask(t, "rollback")
}

func TestAutomaticHistoryPagesAndLocatorsRejectForeignAndChangedViews(t *testing.T) {
	writer, directory := automaticDatabase(t)
	request := automaticRequest()
	request.Limit = 1
	reader, _, err := automaticOpen(t, directory, request.Lease.Limits)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.result(t.Context(), request)
	if err != nil || result.NextCursor == "" {
		t.Fatal("bounded page:", err)
	}
	reader.Close()
	for _, changed := range []string{"root", "snapshot", "limits", "catalog", "locator"} {
		t.Run(changed, func(t *testing.T) {
			selected := request
			selected.Cursor = result.NextCursor
			switch changed {
			case "root":
				selected.Lease.RootRef = "foreign-root"
			case "snapshot":
				selected.Lease.SnapshotRef = "foreign-snapshot"
			case "limits":
				selected.Lease.Limits.MaxParts--
			case "catalog":
				writer.ask(t, "append")
			case "locator":
				selected.Limit, selected.Cursor, selected.Mode = 0, "", "transcript"
				locator := result.Entries[0].Locator
				locator.SessionID = "ses_child"
				selected.Locator = &locator
			}
			next, _, err := automaticOpen(t, directory, selected.Lease.Limits)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := next.result(t.Context(), selected); err == nil {
				t.Fatal("stale or foreign history view was accepted:", changed)
			}
			next.Close()
		})
	}
}

func TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection(t *testing.T) {
	for _, change := range []string{"identity", "replace", "legacy-replace"} {
		t.Run(change, func(t *testing.T) {
			writer, directory := automaticDatabase(t)
			if change == "legacy-replace" {
				writer.ask(t, "legacy")
			}
			request := automaticRequest()
			request.Limit = 1
			reader, _, err := automaticOpen(t, directory, request.Lease.Limits)
			if err != nil {
				t.Fatal(err)
			}
			before, err := reader.result(t.Context(), request)
			if err != nil || len(before.Entries) != 1 || before.NextCursor == "" {
				t.Fatal("initial database selection:", err, before)
			}
			reader.Close()
			if change == "identity" {
				writer.ask(t, "identity")
			} else {
				writer.ask(t, "replace")
			}
			reader, _, err = automaticOpen(t, directory, request.Lease.Limits)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := reader.result(t.Context(), request)
			if err != nil || len(fresh.Entries) != 1 ||
				!bytes.Equal(raw(before.Entries[0].Summary), raw(fresh.Entries[0].Summary)) {
				t.Fatal("database rotation changed the fixture's visible metadata:", err)
			}
			request.Cursor = before.NextCursor
			if _, err := reader.result(t.Context(), request); err == nil {
				t.Error("stale database cursor survived same-root database rotation:", change)
			}
			request.Cursor, request.Limit, request.Mode = "", 0, "transcript"
			request.Locator = &before.Entries[0].Locator
			if _, err := reader.result(t.Context(), request); err == nil {
				t.Error("stale database locator survived same-root database rotation:", change)
			}
			request.Locator = &fresh.Entries[0].Locator
			if _, err := reader.result(t.Context(), request); err != nil {
				t.Fatal("fresh rotated database selection was rejected:", err)
			}
		})
	}
}

func TestAutomaticHistoryUnreadableMalformedAndMissingSHMStayUnknown(t *testing.T) {
	for _, scenario := range []string{"missing-shm", "missing-wal", "rollback-journal", "malformed", "read-budget",
		"generated", "identity-view", "identity-column", "identity-oversized"} {
		t.Run(scenario, func(t *testing.T) {
			writer, directory := automaticDatabase(t)
			request := automaticRequest()
			switch scenario {
			case "missing-shm":
				if err := os.Rename(filepath.Join(directory, "db.sqlite-shm"), filepath.Join(directory, "saved-shm")); err != nil {
					t.Fatal(err)
				}
			case "missing-wal":
				if err := os.Rename(filepath.Join(directory, "db.sqlite-wal"), filepath.Join(directory, "saved-wal")); err != nil {
					t.Fatal(err)
				}
			case "rollback-journal":
				if err := os.WriteFile(filepath.Join(directory, "db.sqlite-journal"), []byte("unresolved"), 0600); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				writer.ask(t, "malformed")
			case "read-budget":
				request.Lease.Limits.MaxReadBytes = 100
			case "generated", "identity-view", "identity-column", "identity-oversized":
				database, err := sql.Open("sqlite", filepath.Join(directory, "db.sqlite"))
				if err != nil {
					t.Fatal(err)
				}
				statements := map[string]string{
					"generated":          "ALTER TABLE message ADD COLUMN generated TEXT GENERATED ALWAYS AS (hex(zeroblob(100000))) VIRTUAL",
					"identity-view":      "DROP TABLE zcode_database_identity; CREATE VIEW zcode_database_identity AS SELECT 'session' AS scope,'foreign' AS identity",
					"identity-column":    "ALTER TABLE zcode_database_identity RENAME COLUMN identity TO foreign_column",
					"identity-oversized": "UPDATE zcode_database_identity SET identity=hex(zeroblob(200))",
				}
				_, err = database.Exec(statements[scenario])
				database.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			reader, _, err := automaticOpen(t, directory, request.Lease.Limits)
			if err == nil {
				_, err = reader.result(t.Context(), request)
				reader.Close()
			}
			if err == nil {
				t.Fatal("failed/unsafe SQLite read became a valid empty or partial history:", scenario)
			}
		})
	}
}

func TestAutomaticHistorySDKChecksLeaseBeforeAndAfterProjection(t *testing.T) {
	for _, revokeAt := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+revokeAt)), func(t *testing.T) {
			_, directory := automaticDatabase(t)
			root, err := os.Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			host, worker := net.Pipe()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- sdk.Serve(ctx, worker, worker, NewControlModule(ModuleOptions{})) }()
			t.Cleanup(func() {
				cancel()
				host.Close()
				worker.Close()
				<-done
			})
			host.SetDeadline(time.Now().Add(10 * time.Second))
			encoder, decoder := json.NewEncoder(host), json.NewDecoder(host)
			if err := encoder.Encode(sdk.Frame{Kind: sdk.KindCall, ID: "configure",
				Method: sdk.MethodComponentConfigure, Scope: sdk.Scope{Plugin: "zcode", Operation: "configure"},
				Params: raw(sdk.ComponentConfigureRequest{Component: "primary", Instance: "fixture",
					Config: sdk.ConfigSnapshot{Version: "1", Values: raw(map[string]any{})}})}); err != nil {
				t.Fatal(err)
			}
			var configured sdk.Frame
			if err := decoder.Decode(&configured); err != nil || configured.Kind != sdk.KindResult || configured.Error != nil {
				t.Fatal("storage fixture configure:", err, configured)
			}
			request := automaticRequest()
			if err := encoder.Encode(sdk.Frame{Kind: sdk.KindCall, ID: "history",
				Method: sdk.MethodStorageList, Scope: sdk.Scope{Plugin: "zcode", Operation: "history"}, Params: raw(request)}); err != nil {
				t.Fatal(err)
			}
			checks, opened := 0, 0
			for {
				var frame sdk.Frame
				if err := decoder.Decode(&frame); err != nil {
					t.Fatal(err)
				}
				if frame.Kind == sdk.KindResult {
					if revokeAt == 0 {
						var result sdk.StorageResult
						if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
							len(result.Entries) != 5 || checks != 3 || opened != 1 {
							t.Fatal("actual SDK storage route failed:", frame.Error, result, checks)
						}
					} else if frame.Error == nil || len(frame.Result) != 0 || revokeAt == 1 && opened != 0 {
						t.Fatal("revoked storage lease published history or opened before admission:", revokeAt, frame)
					}
					break
				}
				if frame.Kind != sdk.KindHostCall || frame.Parent != "history" || len(frame.Auth) != 0 || frame.Scope.Account != "" {
					t.Fatal("history acquired foreign callback authority:", frame)
				}
				reply := sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent}
				switch frame.Method {
				case sdk.MethodStorageCheck:
					checks++
					if revokeAt != 0 && checks >= revokeAt {
						reply.Error = &sdk.Error{Code: "revoked", Message: "Synthetic publication revocation"}
					} else {
						reply.Result = raw(map[string]any{})
					}
				case sdk.MethodStorageNative:
					opened++
					reply.Result = raw(sdk.StorageNativeAccess{Lease: request.Lease, Descriptor: int(root.Fd()),
						Family: historyDescriptor().Families[0]})
				default:
					t.Fatal("history attempted a non-storage callback:", frame.Method)
				}
				if err := encoder.Encode(reply); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
