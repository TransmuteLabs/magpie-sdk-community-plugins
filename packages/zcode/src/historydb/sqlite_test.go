package historydb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, statements ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func historyFixture(t *testing.T) []byte {
	t.Helper()
	return fixture(t,
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, title TEXT, title_source TEXT, time_created INTEGER, time_updated INTEGER, future BLOB)`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, data TEXT)`,
		`CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT, session_id TEXT, data TEXT)`,
		`INSERT INTO session VALUES ('ses_one','/synthetic','Generated','default',9007199254740993,9007199254740994,x'0080ff')`,
		`INSERT INTO message VALUES ('msg_sql_id','ses_one','{"role":"assistant","modelId":"GLM-5.3","tokens":{"input":9007199254740993,"output":4,"cache":{"read":6}},"metadata":{"forkOrigin":{"sessionId":"other","messageId":"original"}},"unknown":{"keep":true}}')`,
		`INSERT INTO part VALUES ('part_one','msg_sql_id','ses_one','{"type":"reasoning","signature":"synthetic-opaque","bodyRef":"external-not-loaded"}')`)
}

func TestSQLiteRealHistoryPreservesRowsOpaqueFieldsAndExactBinaryRestoration(t *testing.T) {
	body := historyFixture(t)
	document, err := Read(t.Context(), body)
	if err != nil || document.Summary().Kind != "cli-history" || document.Summary().Rows["message"] != 1 {
		t.Fatalf("real SQLite history: %v %+v", err, document.Summary())
	}
	tables := document.Tables()
	if string(tables["session"][0]["time_created"]) != "9007199254740993" ||
		!bytes.Contains(tables["message"][0]["data"], []byte("9007199254740993")) ||
		!bytes.Contains(tables["part"][0]["data"], []byte("synthetic-opaque")) ||
		!bytes.Contains(tables["session"][0]["future"], []byte("sqliteBlob")) {
		t.Fatal("SQLite rows lost exact integers, opaque data or future BLOB")
	}
	tables["message"][0]["data"][0] = 'x'
	source := document.Source()
	source[0] = 'x'
	if document.Source()[0] != 'S' || document.Tables()["message"][0]["data"][0] == 'x' {
		t.Fatal("caller mutated the stored immutable snapshot")
	}
	bundle, hash, err := Convert(t.Context(), Format, Bundle, body)
	if err != nil {
		t.Fatal(err)
	}
	restored, restoredHash, err := Convert(t.Context(), Bundle, Format, bundle)
	if err != nil || hash != restoredHash || !bytes.Equal(restored, body) {
		t.Fatal("real SQLite bundle did not restore exact DB bytes:", err)
	}
	if _, _, err := Convert(t.Context(), Format, "zcode-native-jsonl", body); err == nil {
		t.Fatal("SQLite was silently substituted with JSONL")
	}
}

func TestSQLiteWALNeedsConsistentCheckpointedSnapshotAndPreservesUncommittedBoundary(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "live-fixture.sqlite")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)",
		"INSERT INTO tasks VALUES ('committed','Visible',1,0)", "PRAGMA wal_checkpoint(FULL)",
		"INSERT INTO tasks VALUES ('wal-committed','Visible in WAL',2,0)"} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Read(t.Context(), raw); err == nil {
		t.Fatal("uncheckpointed main file was interpreted without its committed WAL")
	}
	transaction, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec("INSERT INTO tasks VALUES ('uncommitted','Invisible',2,0)"); err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	snapshot := filepath.Join(directory, "backup.sqlite")
	walBefore, err := os.ReadFile(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ExecContext(t.Context(), "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal(err)
	}
	mainAfter, mainErr := os.ReadFile(path)
	walAfter, walErr := os.ReadFile(path + "-wal")
	if mainErr != nil || walErr != nil || !bytes.Equal(mainAfter, raw) || !bytes.Equal(walAfter, walBefore) {
		t.Fatal("snapshot fixture changed original DB/WAL instead of capturing its read view")
	}
	captured, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Read(t.Context(), captured)
	if err != nil || document.Summary().Rows["tasks"] != 2 {
		t.Fatal("consistent SQLite backup did not capture the committed WAL view:", err)
	}
	for _, row := range document.Tables()["tasks"] {
		if id, _ := stringField(row, "task_id"); id == "uncommitted" {
			t.Fatal("uncommitted transaction became visible")
		}
	}
}

func TestSQLiteArchivedControllerReportsPendingAndLegacyDebtWithoutAdmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"PRAGMA application_id=1296518721", "PRAGMA user_version=2",
		"CREATE TABLE native_journal(scope_key TEXT PRIMARY KEY,origin TEXT,scope TEXT,version INTEGER,payload BLOB)",
		"CREATE TABLE legacy_zcode_reset(scope_key TEXT PRIMARY KEY,payload BLOB,mapped_scope TEXT)"} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	payload := []byte(`{"version":1,"scope":"old-scope","origin":"https://zcode.z.ai","reset":{"fiveHigh":12.75,"pending":{"generation":"old-login-string","highWater":13.5}},"claims":{"plan":{"state":"pending","claim":"unreplayed"}},"executions":{"task":{"state":"ready","ticketId":"synthetic-private-ticket"}},"future":{"retain":true}}`)
	if _, err := database.Exec("INSERT INTO native_journal VALUES (?,?,?,?,?)",
		digest([]byte("https://zcode.z.ai\x00old-scope")), "https://zcode.z.ai", "old-scope", 1, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO legacy_zcode_reset VALUES ('unmapped',?, '')", []byte(`{"pending":{"key":"lost-reset","baseline":13.5}}`)); err != nil {
		t.Fatal(err)
	}
	database.Close()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Read(t.Context(), body)
	if err != nil || document.Summary().Kind != "archived-controller" || document.Summary().Pending != 3 || document.Summary().LegacyDebt != 1 {
		t.Fatalf("archived pending/debt lost: %v %+v", err, document.Summary())
	}
	encoded, _ := json.Marshal(document.Tables())
	if !bytes.Contains(encoded, []byte("sqliteBlob")) {
		t.Fatal("financial payload was not retained as its original BLOB")
	}
	if !bytes.Equal(document.Source(), body) {
		t.Fatal("read-only financial inspection changed the source")
	}
	if _, _, err := Convert(t.Context(), Format, Bundle, body); err == nil {
		t.Fatal("financial journal inspection became an admitted restore plan")
	}
}

func TestSQLiteRejectsCorruptUnsupportedViewsTriggersAndAlteredBundles(t *testing.T) {
	for _, scenario := range []string{"header", "truncated", "schema", "view", "trigger", "bad-json"} {
		t.Run(scenario, func(t *testing.T) {
			body := historyFixture(t)
			switch scenario {
			case "header":
				body[0] = 'x'
			case "truncated":
				body = body[:120]
			case "schema":
				body = fixture(t, "CREATE TABLE unknown(id TEXT)")
			case "view":
				body = fixture(t, "CREATE TABLE data(id TEXT)", "CREATE VIEW tasks AS SELECT id AS task_id, id AS title,1 AS updated_at,0 AS deleted FROM data")
			case "trigger":
				body = fixture(t, "CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)",
					"CREATE TRIGGER unexpected AFTER INSERT ON tasks BEGIN SELECT 1; END")
			case "bad-json":
				body = fixture(t, "CREATE TABLE session(id TEXT,directory TEXT,title TEXT,time_created INTEGER,time_updated INTEGER)",
					"CREATE TABLE message(id TEXT,session_id TEXT,data TEXT)", "CREATE TABLE part(id TEXT,message_id TEXT,session_id TEXT,data TEXT)",
					`INSERT INTO message VALUES('message','session','{"duplicate":1,"duplicate":2}')`)
			}
			if _, err := Read(t.Context(), body); err == nil {
				t.Fatal("unsafe SQLite source was admitted")
			}
		})
	}
	body := historyFixture(t)
	bundle, _, err := Convert(t.Context(), Format, Bundle, body)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(bundle), `"version":1`, `"version":2`, 1)
	if _, _, err := Convert(t.Context(), Bundle, Format, []byte(changed)); err == nil {
		t.Fatal("unsupported bundle version was restored")
	}
	changed = strings.Replace(string(bundle), digest(body), strings.Repeat("0", 64), 1)
	if _, _, err := Convert(t.Context(), Bundle, Format, []byte(changed)); err == nil {
		t.Fatal("altered snapshot digest was restored")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Read(ctx, body); err != context.Canceled {
		t.Fatal("canceled snapshot read continued:", err)
	}
}

func TestSQLiteForkCountsUseSQLIdentitiesAndRetainRoleToolMediaAndBodyReferences(t *testing.T) {
	body := fixture(t,
		"CREATE TABLE session(id TEXT,directory TEXT,title TEXT,time_created INTEGER,time_updated INTEGER)",
		"CREATE TABLE message(id TEXT,session_id TEXT,data TEXT)",
		"CREATE TABLE part(id TEXT,message_id TEXT,session_id TEXT,data TEXT)",
		`INSERT INTO session VALUES('root','/synthetic','Original',1,2),('fork','/synthetic','Fork',3,4)`,
		`INSERT INTO message VALUES('original','root','{"role":"assistant","tokens":{"input":1000,"output":100,"cache":{"read":600},"reasoning":25}}')`,
		`INSERT INTO message VALUES('copy','fork','{"role":"assistant","tokens":{"input":1000},"metadata":{"forkOrigin":{"messageId":"original","sessionId":"root"}}}')`,
		`INSERT INTO message VALUES('copy2','fork','{"role":"assistant","tokens":{"input":1000},"metadata":{"forkOrigin":{"messageId":"copy","sessionId":"fork"}}}')`,
		`INSERT INTO message VALUES('new','fork','{"role":"assistant","tokens":{"input":20,"output":2}}')`,
		`INSERT INTO message VALUES('orphan','fork','{"role":"assistant","tokens":{"input":300},"metadata":{"forkOrigin":{"messageId":"missing","sessionId":"deleted"}}}')`,
		`INSERT INTO message VALUES('user','fork','{"role":"user","semantics":{"origin":"real_user"},"tokens":{"input":999}}')`,
		`INSERT INTO part VALUES('tool','original','root','{"type":"tool","state":{"input":{"query":"fixture"},"output":"retained"}}')`,
		`INSERT INTO part VALUES('media','new','fork','{"type":"image","mime":"image/png","bodyRef":"missing-body-not-read"}')`)
	document, err := Read(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	summary := document.Summary()
	if summary.Calls != 3 || summary.Copied != 2 || summary.ToolParts != 1 || summary.MediaParts != 1 || summary.BodyRefs != 1 {
		t.Fatalf("fork/role/tool/media summary: %+v", summary)
	}
	bundle, _, err := Convert(t.Context(), Format, Bundle, body)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, err := Convert(t.Context(), Bundle, Format, bundle)
	if err != nil || !bytes.Equal(restored, body) {
		t.Fatal("native history projection dropped opaque role/tool/media fields")
	}
}

func TestSQLiteSnapshotBoundsAndMissingSidecarsFailExplicitly(t *testing.T) {
	if _, err := Read(t.Context(), make([]byte, MaxBytes+1)); err != ErrLimit {
		t.Fatal("oversized database was not bounded:", err)
	}
	body := fixture(t, "CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)",
		"WITH RECURSIVE rows(id) AS (VALUES(1) UNION ALL SELECT id+1 FROM rows WHERE id<16385) INSERT INTO tasks SELECT 'task-'||id,'Bounded',id,0 FROM rows")
	if _, err := Read(t.Context(), body); err != ErrLimit {
		t.Fatal("row limit was not enforced:", err)
	}
	body = historyFixture(t)
	body[18], body[19] = 2, 2
	if _, err := Read(t.Context(), body); err == nil {
		t.Fatal("WAL-mode main file was admitted without an authoritative snapshot")
	}
}

func TestSQLiteSnapshotURIKeepsWindowsDriveOutOfAuthorityAndEscapesPath(t *testing.T) {
	for _, path := range []string{"/private/snapshot #?.db", "C:/private/snapshot #?.db"} {
		address, err := url.Parse(snapshotURI(path))
		expected := path
		if path[1] == ':' {
			expected = "/" + path
		}
		if err != nil || address.Scheme != "file" || address.Host != "" || address.Path != expected ||
			address.Query().Get("mode") != "ro" || address.Query().Get("immutable") != "1" || address.Fragment != "" {
			t.Fatal("SQLite snapshot URI changed path or authority:", address, err)
		}
	}
}
