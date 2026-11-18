package zcode

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/history"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/historydb"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func sessionFixture() sdk.SessionRequest {
	return sdk.SessionRequest{
		ID: historyAdapterID, From: history.NativeJSONL, To: history.Bundle,
		Files: []sdk.SessionDocument{{Path: "source.jsonl", Format: history.NativeJSONL,
			Body: []byte("{\"type\":\"assistant\",\"sessionId\":\"untrusted-task\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"signature\":\"synthetic-signature\"}]}}\r\n")}},
		Options: raw(map[string]string{"outputPath": "archive/history.json"}),
	}
}

func TestSQLiteSessionAdapterUsesActualBinarySnapshotAndRejectsSourceOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)"); err != nil {
		t.Fatal(err)
	}
	database.Close()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, ModuleOptions{})
	request := sdk.SessionRequest{ID: sqliteAdapterID, From: historydb.Format, To: historydb.Bundle,
		Files:   []sdk.SessionDocument{{Path: "source.db", Format: historydb.Format, Body: body}},
		Options: raw(map[string]string{"outputPath": "bundle.json"})}
	frame, _ := h.call(sdk.MethodSessionConvert, sdk.Scope{}, request, nil)
	var packed sdk.SessionResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &packed) != nil || len(packed.Files) != 1 {
		t.Fatalf("SQLite native SDK conversion: %+v", frame.Error)
	}
	restore := sdk.SessionRequest{ID: sqliteAdapterID, From: historydb.Bundle, To: historydb.Format,
		Files: packed.Files, Options: raw(map[string]string{"outputPath": "restored.db"})}
	frame, _ = h.call(sdk.MethodSessionConvert, sdk.Scope{}, restore, nil)
	var restored sdk.SessionResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &restored) != nil || len(restored.Files) != 1 ||
		!bytes.Equal(restored.Files[0].Body, body) || h.proxies != 0 {
		t.Fatal("real SQLite source bytes were lost or network authority was acquired")
	}
	request.Options = raw(map[string]string{"outputPath": "source.db"})
	frame, _ = h.call(sdk.MethodSessionConvert, sdk.Scope{}, request, nil)
	if frame.Error == nil {
		t.Fatal("SQLite conversion overwrote its explicit source")
	}
}

func TestSQLiteNativeSessionPathCannotPromoteArchivedFinancialDebt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "financial.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"PRAGMA application_id=1296518721", "PRAGMA user_version=2",
		"CREATE TABLE native_journal(scope_key TEXT,origin TEXT,scope TEXT,version INTEGER,payload BLOB)",
		"CREATE TABLE legacy_zcode_reset(scope_key TEXT,payload BLOB,mapped_scope TEXT)",
		`INSERT INTO legacy_zcode_reset VALUES('unmapped','{"pending":{"generation":"old-generation","key":"must-not-replay"}}','')`} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	database.Close()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(body)
	for _, restoring := range []bool{false, true} {
		h := newHarness(t, ModuleOptions{})
		request := sdk.SessionRequest{ID: sqliteAdapterID, From: historydb.Format, To: historydb.Bundle,
			Files:   []sdk.SessionDocument{{Path: "financial.db", Format: historydb.Format, Body: body}},
			Options: raw(map[string]string{"outputPath": "forbidden-copy.db"})}
		if restoring {
			request.From, request.To = historydb.Bundle, historydb.Format
			request.Files[0].Format = historydb.Bundle
			request.Files[0].Body = raw(map[string]any{"kind": historydb.Bundle, "version": 1,
				"sha256": hex.EncodeToString(hash[:]), "source": body})
		}
		frame, events := h.call(sdk.MethodSessionConvert, sdk.Scope{}, request, nil)
		if frame.Error == nil || len(events) != 0 || h.proxies != 0 {
			t.Fatal("archived financial debt became a file restore, vendor call or accepted operation")
		}
	}
}

func TestRegisteredOfflineSessionAdapterConvertsAndRestoresWithoutIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-be-created")
	h := newHarnessConfigured(t, ModuleOptions{}, moduleConfig{DataDir: path})
	request := sessionFixture()
	frame, _ := h.call(sdk.MethodSessionConvert, sdk.Scope{}, request, nil)
	var result sdk.SessionResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Files) != 1 ||
		result.Files[0].Path != "archive/history.json" || result.Files[0].Format != history.Bundle ||
		!strings.HasPrefix(result.ID, "zcode-history-") || strings.Contains(result.ID, "untrusted-task") {
		t.Fatalf("real session adapter: %+v %+v", frame.Error, result)
	}
	restoring := sdk.SessionRequest{ID: historyAdapterID, From: history.Bundle, To: history.NativeJSONL,
		Files: result.Files, Options: raw(map[string]string{"outputPath": "restored.jsonl"})}
	frame, _ = h.call(sdk.MethodSessionConvert, sdk.Scope{}, restoring, nil)
	var restored sdk.SessionResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &restored) != nil ||
		len(restored.Files) != 1 || string(restored.Files[0].Body) != string(request.Files[0].Body) ||
		restored.Files[0].Path != "restored.jsonl" || restored.ID != result.ID || h.proxies != 0 {
		t.Fatalf("offline session restore or ownership failed: %+v", frame.Error)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("offline session adapter opened the business dataDir")
	}
}

func TestOfflineSessionAdapterRejectsUnsafeAndAmbiguousPlans(t *testing.T) {
	for _, scenario := range []string{"descriptor", "multiple", "mixed", "same-format", "cross-agent", "source-overwrite",
		"source-case-overwrite", "traversal", "absolute", "windows", "reserved", "unknown-options", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			h := newHarness(t, ModuleOptions{})
			request := sessionFixture()
			switch scenario {
			case "descriptor":
				request.ID = "another-module"
			case "multiple":
				request.Files = append(request.Files, request.Files[0])
			case "mixed":
				request.Files[0].Format = history.LegacySession
			case "same-format":
				request.To = request.From
			case "cross-agent":
				request.To = "claude"
			case "source-overwrite":
				request.Options = raw(map[string]string{"outputPath": "source.jsonl"})
			case "source-case-overwrite":
				request.Options = raw(map[string]string{"outputPath": "SOURCE.JSONL"})
			case "traversal":
				request.Options = raw(map[string]string{"outputPath": "../escape"})
			case "absolute":
				request.Options = raw(map[string]string{"outputPath": "/escape"})
			case "windows":
				request.Options = raw(map[string]string{"outputPath": `C:\escape`})
			case "reserved":
				request.Options = raw(map[string]string{"outputPath": ".magpie-extension-state/escape"})
			case "unknown-options":
				request.Options = raw(map[string]any{"outputPath": "target.json", "credentials": map[string]string{"token": "not-authority"}})
			case "malformed":
				request.Files[0].Body = []byte("SQLite format 3\x00fixture")
			}
			frame, _ := h.call(sdk.MethodSessionConvert, sdk.Scope{}, request, nil)
			if frame.Error == nil || h.proxies != 0 {
				t.Fatal("unsafe/off-format history plan was accepted or caused network access")
			}
		})
	}
}
