package opencode

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type historyTestRoot struct {
	mu    sync.Mutex
	root  *os.Root
	read  int64
	limit int64
}

func (root *historyTestRoot) Open(name string) (*os.File, error) {
	if !root.Family().Allows(name) {
		return nil, os.ErrPermission
	}
	return root.root.Open(name)
}

func (root *historyTestRoot) Consume(bytes int64) error {
	root.mu.Lock()
	defer root.mu.Unlock()
	root.read += bytes
	if root.read > root.limit {
		return errors.New("test rooted read budget exceeded")
	}
	return nil
}

func (*historyTestRoot) Family() sdk.StorageFamily {
	return sdk.StorageFamily{ID: "history", Primary: "opencode.db", Sidecars: []string{"opencode.db-wal", "opencode.db-shm"}}
}

func historyDatabaseFixture(t *testing.T) (*sql.DB, *historyTestRoot, string) {
	t.Helper()
	directory := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(directory, "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	schema, err := os.ReadFile("testdata/donors/history-schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", "PRAGMA busy_timeout=30", string(schema),
		"INSERT INTO project(id,worktree,time_created,time_updated,sandboxes) VALUES('global','/fixture',1,1,'[]')"} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return db, &historyTestRoot{root: root, limit: 64 << 20}, directory
}

func historyInsertSession(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec("INSERT INTO session_v2(id,project_id,slug,directory,title,version,time_created,time_updated) VALUES(?,'global',?,'/fixture',?,'2',1,2)", id, id, id)
	if err != nil {
		t.Fatal(err)
	}
}

func historyInsertMessage(t *testing.T, db *sql.DB, id, kind string, seq int, data string) {
	t.Helper()
	_, err := db.Exec("INSERT INTO session_message(id,session_id,type,seq,time_created,time_updated,data) VALUES(?,'ses_wal',?,?,1,2,?)", id, kind, seq, data)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHistoryRealReadonlyWALTransactionSurvivesCommitAndCheckpoint(t *testing.T) {
	writer, root, directory := historyDatabaseFixture(t)
	if _, err := writer.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	historyInsertSession(t, writer, "ses_wal")
	reader, err := openHistorySQL(t.Context(), root)
	if err != nil {
		t.Fatal("real rooted SQLite WAL transaction could not start", err)
	}
	closed := false
	defer func() {
		if !closed {
			reader.Close()
		}
	}()
	var count int
	if err := reader.tx.QueryRow("SELECT count(*) FROM session_v2").Scan(&count); err != nil || count != 1 {
		t.Fatal("committed WAL-only session was not visible", count, err)
	}
	if _, err := reader.tx.Exec("DELETE FROM session_v2"); err == nil {
		t.Fatal("history transaction wrote the donor database")
	}
	historyInsertSession(t, writer, "ses_later")
	if err := reader.tx.QueryRow("SELECT count(*) FROM session_v2").Scan(&count); err != nil || count != 1 {
		t.Fatal("WAL writer changed an existing read transaction", count, err)
	}
	var busy, log, checkpointed int
	if err := writer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed); err != nil || busy != 1 {
		t.Fatal("checkpoint reset was not held behind the rooted read snapshot", busy, log, checkpointed, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	if err := writer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed); err != nil || busy != 0 {
		t.Fatal("history reader leaked checkpoint locks", busy, log, checkpointed, err)
	}
	historyInsertSession(t, writer, "ses_after_reset")
	next, err := openHistorySQL(t.Context(), root)
	if err != nil {
		t.Fatal("fresh rooted reader could not observe a WAL reset", err)
	}
	if err := next.tx.QueryRow("SELECT count(*) FROM session_v2").Scan(&count); err != nil || count != 3 {
		t.Fatal("fresh transaction lost reset WAL commits", count, err)
	}
	next.Close()
	tx, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO session_v2(id,project_id,slug,directory,version,time_created,time_updated) VALUES('ses_uncommitted','global','uncommitted','/fixture','2',1,2)"); err != nil {
		t.Fatal(err)
	}
	uncommitted, err := openHistorySQL(t.Context(), root)
	if err != nil {
		t.Fatal("uncommitted writer prevented a committed read snapshot", err)
	}
	if err := uncommitted.tx.QueryRow("SELECT count(*) FROM session_v2").Scan(&count); err != nil || count != 3 {
		t.Fatal("uncommitted rows crossed history publication", count, err)
	}
	uncommitted.Close()
	tx.Rollback()
	mainOnly := t.TempDir()
	body, err := os.ReadFile(filepath.Join(directory, "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainOnly, "opencode.db"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	copyDB, err := sql.Open("sqlite", "file:"+filepath.Join(mainOnly, "opencode.db")+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	if err := copyDB.QueryRow("SELECT count(*) FROM session_v2").Scan(&count); err != nil || count != 2 {
		t.Fatal("causal main-only negative did not omit the post-reset WAL session", count, err)
	}
}

func TestHistoryActualPartsMediaEpochAndDedup(t *testing.T) {
	writer, root, _ := historyDatabaseFixture(t)
	historyInsertSession(t, writer, "ses_wal")
	historyInsertMessage(t, writer, "msg_user", "user", 1, `{"text":"hello","files":[]}`)
	historyInsertMessage(t, writer, "msg_native", "compaction", 2, `{"status":"completed","native":{"messages":"[]"},"summary":""}`)
	historyInsertMessage(t, writer, "msg_instruction_old", "system", 3, `{"text":"old instruction","metadata":{"notice":"instructions"}}`)
	historyInsertMessage(t, writer, "msg_instruction_new", "system", 4, `{"text":"current instruction","metadata":{"notice":"instructions"}}`)
	historyInsertMessage(t, writer, "msg_assistant", "assistant", 5, `{"model":{"providerID":"fixture","id":"fixture-model"},"content":[{"type":"text","text":"inline"}],"tokens":{"input":7,"output":3,"reasoning":5,"cache":{"read":2,"write":1}}}`)
	for ordinal, text := range []string{"first", "second"} {
		_, err := writer.Exec("INSERT INTO session_part(message_id,session_id,ordinal,type,time_created,time_updated,data) VALUES('msg_assistant','ses_wal',?,'text',1,2,?)", ordinal, string(historyJSON(map[string]any{"type": "text", "text": text})))
		if err != nil {
			t.Fatal(err)
		}
	}
	media := []byte("actual synthetic media bytes")
	hash := sha256.Sum256(media)
	id := hex.EncodeToString(hash[:])
	if _, err := writer.Exec("INSERT INTO blob(hash,mime,bytes,data,time_created) VALUES(?,'image/png',?,?,1)", id, len(media), media); err != nil {
		t.Fatal(err)
	}
	historyInsertMessage(t, writer, "msg_files", "user", 6, string(historyJSON(map[string]any{"text": "media", "files": []any{
		map[string]any{"mime": "image/png", "name": "present.png", "uri": "media:" + id},
		map[string]any{"mime": "image/png", "name": "same-blob.png", "uri": "media:" + id},
		map[string]any{"mime": "image/png", "name": "missing.png", "uri": "media:" + strings.Repeat("0", 64)},
		map[string]any{"mime": "text/plain", "name": "external", "uri": "file:///etc/passwd"},
	}})))
	for _, statement := range []string{
		"INSERT INTO instruction_state(session_id,epoch_start,through_seq,initial_values,current_values) VALUES('ses_wal',4,4,'[]','[]')",
		"CREATE TABLE kv(key TEXT PRIMARY KEY,value TEXT NOT NULL)",
		"CREATE TABLE session AS SELECT * FROM session_v2",
		"CREATE TABLE message(id TEXT,session_id TEXT,time_created INTEGER,data TEXT)",
		"CREATE TABLE part(id TEXT,session_id TEXT,message_id TEXT,data TEXT)",
		"INSERT INTO session(id,project_id,slug,directory,version,time_created,time_updated) VALUES('ses_v1','global','old','/fixture','1',1,2)",
	} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writer.Exec("INSERT INTO blob_ref(session_id,message_id,hash,seq) VALUES('ses_wal','msg_files',?,6)", id); err != nil {
		t.Fatal(err)
	}
	reader, err := openHistorySQL(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	lease := sdk.StorageReadLease{RootRef: "root", SnapshotRef: "snapshot", Limits: sdk.DefaultStorageLimits()}
	lease.Limits.MaxMediaBytes = len(media)
	entries, cut, err := reader.catalog(t.Context(), lease)
	if err != nil || cut || len(entries) != 2 {
		t.Fatal("V1/V2 coexistence did not deduplicate by migrated ID", entries, cut, err)
	}
	messages, err := reader.messages(t.Context(), "v2", "ses_wal", lease.Limits)
	if err != nil {
		t.Fatal(err)
	}
	messages, err = reader.localHistory(t.Context(), "ses_wal", messages, lease.Limits)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.StorageRequest{Lease: lease, Locator: &sdk.StorageSessionLocator{RootRef: "root", SessionID: "ses_wal", Key: "v2:ses_wal"}, Mode: "transcript"}
	result, err := reader.transcript(t.Context(), request, sdk.StorageResult{SnapshotRef: "snapshot", SessionID: "ses_wal", Mode: "transcript"}, messages)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.Parts)
	text := string(encoded)
	if strings.Contains(text, "old instruction") || strings.Contains(text, "native.messages") ||
		!strings.Contains(text, "current instruction") || strings.Index(text, "first") > strings.Index(text, "second") ||
		strings.Index(text, "second") > strings.Index(text, "inline") || !strings.Contains(text, "[media unavailable: missing.png (image/png)]") ||
		!strings.Contains(text, "[media unavailable: external (text/plain)]") || len(result.Media) != 1 || string(result.Media[0].Bytes) != string(media) {
		t.Fatal("actual parts/replay/media semantics were lost", text, result.Media)
	}
	if err := sdk.ValidateStorageResult(request, result); err != nil {
		t.Fatal("actual history result does not satisfy storage contract", err)
	}
	if len(entries[1].Summary.Usage) != 1 || entries[1].Summary.Usage[0].Input != 7 || entries[1].Summary.Usage[0].Output != 3 {
		t.Fatal("actual history usage projection changed native counters", entries)
	}
	reader.Close()
	if _, err := writer.Exec(`INSERT INTO kv(key,value) VALUES('migration.v1-v2','{"phase":"completed"}')`); err != nil {
		t.Fatal(err)
	}
	completed, err := openHistorySQL(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer completed.Close()
	entries, _, err = completed.catalog(t.Context(), lease)
	if err != nil || len(entries) != 1 || entries[0].Locator.SessionID != "ses_wal" {
		t.Fatal("completed migration resurrected unwritten V1 rows", entries, err)
	}
}

func TestHistoryRejectsStaleLocatorsAndAggregateStoredParts(t *testing.T) {
	for _, scenario := range []string{"removed", "v2-superseded-v1", "completed-migration", "aggregate-parts"} {
		t.Run(scenario, func(t *testing.T) {
			writer, root, _ := historyDatabaseFixture(t)
			for _, statement := range []string{
				"CREATE TABLE session AS SELECT * FROM session_v2",
				"CREATE TABLE message(id TEXT,session_id TEXT,time_created INTEGER,data TEXT)",
				"CREATE TABLE part(id TEXT,session_id TEXT,message_id TEXT,data TEXT)",
				"CREATE TABLE kv(key TEXT PRIMARY KEY,value TEXT NOT NULL)",
			} {
				if _, err := writer.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			version, id, limits := "v1", "ses_wal", sdk.DefaultStorageLimits()
			switch scenario {
			case "removed":
				version = "v2"
			case "v2-superseded-v1":
				historyInsertSession(t, writer, id)
				if _, err := writer.Exec("INSERT INTO session SELECT * FROM session_v2"); err != nil {
					t.Fatal(err)
				}
			case "completed-migration":
				if _, err := writer.Exec("INSERT INTO session(id,directory,time_created,time_updated) VALUES(?,'/fixture',1,2)", id); err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Exec(`INSERT INTO kv VALUES('migration.v1-v2','{"phase":"completed"}')`); err != nil {
					t.Fatal(err)
				}
			case "aggregate-parts":
				version, limits.MaxParts = "v2", 3
				historyInsertSession(t, writer, id)
				for index, message := range []string{"msg_one", "msg_two"} {
					historyInsertMessage(t, writer, message, "assistant", index, `{"model":{"id":"fixture-model"},"content":[]}`)
					for ordinal := range 2 {
						if _, err := writer.Exec("INSERT INTO session_part(message_id,session_id,ordinal,type,time_created,time_updated,data) VALUES(?,'ses_wal',?,'text',1,2,'{\"type\":\"text\",\"text\":\"part\"}')", message, ordinal); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			reader, err := openHistorySQL(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if _, err := reader.messages(t.Context(), version, id, limits); err == nil {
				t.Fatal("stale locator or aggregate stored parts crossed the read boundary", scenario)
			}
		})
	}
}
