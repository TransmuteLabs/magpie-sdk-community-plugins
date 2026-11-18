package historydb

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

func TestSQLiteGeneratedColumnsRefusedBeforeMaterialization(t *testing.T) {
	for _, mode := range []string{"VIRTUAL", "STORED"} {
		t.Run(mode, func(t *testing.T) {
			body := fixture(t,
				"CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER,expanded TEXT GENERATED ALWAYS AS (printf('%2000s','x')) "+mode+")",
				"INSERT INTO tasks(task_id,title,updated_at,deleted)VALUES('safe-fixture','Never evaluate',1,0)")
			if _, err := Read(t.Context(), body); err != ErrSnapshot {
				t.Fatal("generated schema reached projection instead of typed refusal:", err)
			}
		})
	}
}

func TestSQLiteConnectionLimitsPrecedeExpressionEvaluation(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	connection, err := database.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := limitConnection(connection); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []struct{ id, value int }{
		{sqlitelib.SQLITE_LIMIT_LENGTH, 1 << 20},
		{sqlitelib.SQLITE_LIMIT_COLUMN, 256},
		{sqlitelib.SQLITE_LIMIT_EXPR_DEPTH, 64},
		{sqlitelib.SQLITE_LIMIT_VDBE_OP, 100000},
	} {
		value, err := sqlite.Limit(connection, limit.id, -1)
		if err != nil || value != limit.value {
			t.Fatalf("SQLite engine limit %d: got %d, want %d (%v)", limit.id, value, limit.value, err)
		}
	}
	var length int
	if err := connection.QueryRowContext(t.Context(), "SELECT length(printf('%2000000s','x'))").Scan(&length); err == nil {
		t.Fatal("engine materialized an over-limit value before the Go projection bound")
	}
}

func TestSQLiteReadBackgroundHasHardDeadlineAndRetainsEarlierCallerDeadline(t *testing.T) {
	started := time.Now()
	ctx, cancel := readContext(context.Background())
	defer cancel()
	finished := time.Now()
	deadline, ok := ctx.Deadline()
	if !ok || deadline.Before(started.Add(MaxReadDuration)) || deadline.After(finished.Add(MaxReadDuration)) {
		t.Fatal("background SQLite work has no bounded owned deadline")
	}
	earlier, earlierCancel := context.WithDeadline(context.Background(), started.Add(time.Second))
	defer earlierCancel()
	limited, limitedCancel := readContext(earlier)
	defer limitedCancel()
	actual, ok := limited.Deadline()
	if !ok || !actual.Equal(started.Add(time.Second)) {
		t.Fatal("snapshot extended its caller's deadline")
	}
}

func TestSQLiteOrdinaryFutureStoredColumnsAndCheckSchemaRemainLossless(t *testing.T) {
	body := fixture(t,
		"CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER,future TEXT DEFAULT 'VIRTUAL TABLE' CHECK(length(future)<100))",
		"INSERT INTO tasks(task_id,title,updated_at,deleted)VALUES('safe','Future column',1,0)")
	document, err := Read(t.Context(), body)
	if err != nil {
		t.Fatal("ordinary stored column and bounded CHECK schema were rejected:", err)
	}
	if value, _ := stringField(document.Tables()["tasks"][0], "future"); value != "VIRTUAL TABLE" {
		t.Fatal("unknown ordinary stored column was dropped")
	}
}

func TestSQLiteLogicalProjectionCannotExpandSmallStoredDefaultsWithoutBound(t *testing.T) {
	body := fixture(t,
		"CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)",
		"WITH RECURSIVE rows(id) AS (VALUES(1) UNION ALL SELECT id+1 FROM rows WHERE id<2000) INSERT INTO tasks SELECT 'task-'||id,'Stored default',id,0 FROM rows",
		"ALTER TABLE tasks ADD COLUMN future TEXT DEFAULT '"+strings.Repeat("x", 6000)+"'")
	if len(body) >= 1<<20 {
		t.Fatal("resource fixture did not keep its input small")
	}
	if _, err := Read(t.Context(), body); err != ErrLimit {
		t.Fatal("logical stored-default expansion escaped the total projection budget:", err)
	}
}
