package historydb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	"modernc.org/libc"
	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

const (
	Format          = "zcode-sqlite-db"
	Bundle          = "zcode-sqlite-bundle-v1"
	MaxBytes        = 8 << 20
	MaxRows         = 16384
	MaxReadDuration = 10 * time.Second
)

var ErrSnapshot = errors.New("invalid, unsupported or uncheckpointed ZCode SQLite snapshot")
var ErrLimit = errors.New("ZCode SQLite snapshot exceeds its bounds")

func init() {
	values := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if strings.Contains(value, "=") {
			values = append(values, value)
		}
	}
	libc.SetEnviron(nil, values)
}

type Row map[string]json.RawMessage

type Summary struct {
	Kind       string         `json:"kind"`
	SHA256     string         `json:"sha256"`
	Rows       map[string]int `json:"rows"`
	Schema     int            `json:"schema"`
	Pending    int            `json:"pending,omitempty"`
	LegacyDebt int            `json:"legacyDebt,omitempty"`
	Calls      int            `json:"calls,omitempty"`
	Copied     int            `json:"copied,omitempty"`
	ToolParts  int            `json:"toolParts,omitempty"`
	MediaParts int            `json:"mediaParts,omitempty"`
	BodyRefs   int            `json:"bodyRefs,omitempty"`
}

type Document struct {
	source  []byte
	tables  map[string][]Row
	summary Summary
}

func digest(body []byte) string {
	value := sha256.Sum256(body)
	return hex.EncodeToString(value[:])
}

func (document Document) Source() []byte { return bytes.Clone(document.source) }

func (document Document) Summary() Summary {
	result := document.summary
	result.Rows = make(map[string]int, len(document.summary.Rows))
	for name, count := range document.summary.Rows {
		result.Rows[name] = count
	}
	return result
}

func (document Document) Tables() map[string][]Row {
	result := make(map[string][]Row, len(document.tables))
	for name, rows := range document.tables {
		result[name] = []Row{}
		for _, row := range rows {
			value := Row{}
			for key, field := range row {
				value[key] = bytes.Clone(field)
			}
			result[name] = append(result[name], value)
		}
	}
	return result
}

func Read(ctx context.Context, body []byte) (result Document, readErr error) {
	if ctx == nil {
		return Document{}, ErrSnapshot
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	ctx, cancel := readContext(ctx)
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			result, readErr = Document{}, ctx.Err()
		}
	}()
	if len(body) > MaxBytes {
		return Document{}, ErrLimit
	}
	if len(body) < 100 || !bytes.Equal(body[:16], []byte("SQLite format 3\x00")) || body[18] != 1 || body[19] != 1 {
		return Document{}, ErrSnapshot
	}
	directory, err := os.MkdirTemp("", "zcode-immutable-sqlite-")
	if err != nil {
		return Document{}, ErrSnapshot
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "snapshot.sqlite")
	if os.WriteFile(path, body, 0600) != nil {
		return Document{}, ErrSnapshot
	}
	pool, err := sql.Open("sqlite", snapshotURI(path))
	if err != nil {
		return Document{}, ErrSnapshot
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	database, err := pool.Conn(ctx)
	if err != nil {
		return Document{}, ErrSnapshot
	}
	defer database.Close()
	if err := limitConnection(database); err != nil {
		return Document{}, ErrSnapshot
	}
	for _, statement := range []string{"PRAGMA query_only=ON", "PRAGMA trusted_schema=OFF"} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return Document{}, ErrSnapshot
		}
	}
	var integrity string
	var application, version int
	if validateSchema(ctx, database) != nil ||
		database.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity) != nil || integrity != "ok" ||
		database.QueryRowContext(ctx, "PRAGMA application_id").Scan(&application) != nil ||
		database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version) != nil {
		return Document{}, ErrSnapshot
	}
	document := Document{source: bytes.Clone(body), tables: map[string][]Row{},
		summary: Summary{SHA256: digest(body), Rows: map[string]int{}, Schema: version}}
	var tables []string
	switch {
	case application == 1296518721 && (version == 1 || version == 2):
		if !hasColumns(ctx, database, "native_journal", "scope_key", "origin", "scope", "version", "payload") ||
			version == 2 && !hasColumns(ctx, database, "legacy_zcode_reset", "scope_key", "payload", "mapped_scope") {
			return Document{}, ErrSnapshot
		}
		document.summary.Kind, tables = "archived-controller", []string{"native_journal"}
		if version == 2 {
			tables = append(tables, "legacy_zcode_reset")
		}
	case hasColumns(ctx, database, "session", "id", "directory", "title", "time_created", "time_updated") &&
		hasColumns(ctx, database, "message", "id", "session_id", "data") &&
		hasColumns(ctx, database, "part", "id", "message_id", "session_id", "data"):
		document.summary.Kind, tables = "cli-history", []string{"session", "message", "part"}
	case hasColumns(ctx, database, "tasks", "task_id", "title", "updated_at", "deleted"):
		document.summary.Kind, tables = "desktop-task-index", []string{"tasks"}
	default:
		return Document{}, ErrSnapshot
	}
	if document.summary.Kind == "cli-history" {
		for _, table := range []string{"model_usage", "turn_usage", "tool_usage"} {
			var present int
			if database.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name=?", table).Scan(&present) != nil {
				return Document{}, ErrSnapshot
			}
			if present != 0 {
				if !hasColumns(ctx, database, table, "session_id", "status", "started_at") {
					return Document{}, ErrSnapshot
				}
				tables = append(tables, table)
			}
		}
	}
	total, projected := 0, 0
	for _, table := range tables {
		rows, err := readRows(ctx, database, table, MaxRows-total, &projected)
		if err != nil {
			return Document{}, err
		}
		for _, row := range rows {
			if err := document.validateRow(table, row); err != nil {
				return Document{}, err
			}
		}
		document.tables[table], document.summary.Rows[table] = rows, len(rows)
		total += len(rows)
	}
	document.inspectHistory()
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func readContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, MaxReadDuration)
}

func limitConnection(connection *sql.Conn) error {
	for _, limit := range []struct{ id, value int }{
		{sqlitelib.SQLITE_LIMIT_LENGTH, 1 << 20},
		{sqlitelib.SQLITE_LIMIT_COLUMN, 256},
		{sqlitelib.SQLITE_LIMIT_EXPR_DEPTH, 64},
		{sqlitelib.SQLITE_LIMIT_VDBE_OP, 100000},
		{sqlitelib.SQLITE_LIMIT_SQL_LENGTH, 64 << 10},
		{sqlitelib.SQLITE_LIMIT_COMPOUND_SELECT, 16},
		{sqlitelib.SQLITE_LIMIT_VARIABLE_NUMBER, 128},
		{sqlitelib.SQLITE_LIMIT_TRIGGER_DEPTH, 0},
	} {
		if _, err := sqlite.Limit(connection, limit.id, limit.value); err != nil {
			return err
		}
	}
	return nil
}

func validateSchema(ctx context.Context, connection *sql.Conn) error {
	rows, err := connection.QueryContext(ctx, "SELECT type,name FROM sqlite_schema")
	if err != nil {
		return ErrSnapshot
	}
	tables := []string{}
	count := 0
	for rows.Next() {
		var kind, name string
		if rows.Scan(&kind, &name) != nil {
			rows.Close()
			return ErrSnapshot
		}
		count++
		if count > 1024 || kind == "view" || kind == "trigger" {
			rows.Close()
			return ErrSnapshot
		}
		if kind == "table" {
			tables = append(tables, name)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrSnapshot
	}
	for _, table := range tables {
		var kind string
		if connection.QueryRowContext(ctx, "SELECT type FROM pragma_table_list WHERE schema='main' AND name=?", table).Scan(&kind) != nil ||
			kind != "table" {
			return ErrSnapshot
		}
		columns, err := connection.QueryContext(ctx, "SELECT hidden FROM pragma_table_xinfo(?)", table)
		if err != nil {
			return ErrSnapshot
		}
		for columns.Next() {
			var hidden int
			if columns.Scan(&hidden) != nil || hidden != 0 {
				columns.Close()
				return ErrSnapshot
			}
		}
		err = columns.Err()
		columns.Close()
		if err != nil {
			return ErrSnapshot
		}
	}
	return nil
}

func snapshotURI(path string) string {
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}
	address := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&immutable=1"}
	return address.String()
}

func hasColumns(ctx context.Context, database *sql.Conn, table string, columns ...string) bool {
	rows, err := database.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return false
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			return false
		}
		found[name] = true
	}
	for _, name := range columns {
		if !found[name] {
			return false
		}
	}
	return rows.Err() == nil
}

func readRows(ctx context.Context, database *sql.Conn, table string, remaining int, projected *int) ([]Row, error) {
	var kind, schema string
	var triggers int
	if database.QueryRowContext(ctx, "SELECT type,sql FROM sqlite_schema WHERE name=?", table).Scan(&kind, &schema) != nil ||
		kind != "table" || strings.HasPrefix(strings.ToUpper(schema), "CREATE VIRTUAL") ||
		database.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type='trigger' AND tbl_name=?", table).Scan(&triggers) != nil || triggers != 0 {
		return nil, ErrSnapshot
	}
	rows, err := database.QueryContext(ctx, "SELECT * FROM "+table)
	if err != nil {
		return nil, ErrSnapshot
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil || len(columns) > 256 {
		return nil, ErrSnapshot
	}
	result := []Row{}
	for rows.Next() {
		if len(result) >= remaining {
			return nil, ErrLimit
		}
		values, pointers := make([]any, len(columns)), make([]any, len(columns))
		for index := range pointers {
			pointers[index] = &values[index]
		}
		if rows.Scan(pointers...) != nil {
			return nil, ErrSnapshot
		}
		row := Row{}
		for index, name := range columns {
			if value, ok := values[index].(string); ok && !utf8.ValidString(value) {
				return nil, ErrSnapshot
			}
			if value, ok := values[index].([]byte); ok {
				values[index] = map[string][]byte{"sqliteBlob": value}
			}
			encoded, err := json.Marshal(values[index])
			if err != nil || len(encoded) > 1<<20 {
				return nil, ErrLimit
			}
			*projected += len(encoded) + len(name) + 4
			if *projected > MaxBytes {
				return nil, ErrLimit
			}
			row[name] = encoded
		}
		result = append(result, row)
	}
	if rows.Err() != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrSnapshot
	}
	return result, nil
}

func stringField(row Row, key string) (string, bool) {
	var value string
	err := json.Unmarshal(row[key], &value)
	return value, err == nil
}

func objectField(row Row, key string) (map[string]any, error) {
	text, valid := stringField(row, key)
	if !valid {
		var blob struct {
			Value []byte `json:"sqliteBlob"`
		}
		if json.Unmarshal(row[key], &blob) != nil || blob.Value == nil {
			return nil, ErrSnapshot
		}
		text = string(blob.Value)
	}
	if !utf8.ValidString(text) {
		return nil, ErrSnapshot
	}
	return protocol.ReadObject([]byte(text))
}

func (document *Document) validateRow(table string, row Row) error {
	key := ""
	switch table {
	case "message", "part":
		key = "data"
	case "native_journal", "legacy_zcode_reset":
		key = "payload"
	}
	if key == "" {
		return nil
	}
	object, err := objectField(row, key)
	if err != nil {
		return ErrSnapshot
	}
	if table == "native_journal" {
		version, ok := object["version"].(json.Number)
		scope, scopeOK := stringField(row, "scope")
		origin, originOK := stringField(row, "origin")
		key, keyOK := stringField(row, "scope_key")
		if !ok || version.String() != "1" || !scopeOK || !originOK || !keyOK || scope == "" || origin == "" ||
			key != digest([]byte(origin+"\x00"+scope)) || object["scope"] != scope || object["origin"] != origin || string(row["version"]) != "1" {
			return ErrSnapshot
		}
		reset, _ := object["reset"].(map[string]any)
		if reset == nil {
			return ErrSnapshot
		}
		if reset["pending"] != nil {
			document.summary.Pending++
		}
		for _, name := range []string{"claims", "executions"} {
			records, _ := object[name].(map[string]any)
			for _, value := range records {
				record, _ := value.(map[string]any)
				if state, _ := record["state"].(string); state != "confirmed" && state != "settled" && state != "observed" && state != "completed" {
					document.summary.Pending++
				}
			}
		}
	}
	if table == "legacy_zcode_reset" {
		document.summary.LegacyDebt++
	}
	return nil
}

func (document *Document) inspectHistory() {
	present := map[string]bool{}
	for _, row := range document.tables["message"] {
		id, _ := stringField(row, "id")
		present[id] = true
	}
	for _, row := range document.tables["message"] {
		object, _ := objectField(row, "data")
		if object["role"] != "assistant" || object["tokens"] == nil {
			continue
		}
		metadata, _ := object["metadata"].(map[string]any)
		origin, _ := metadata["forkOrigin"].(map[string]any)
		copied, _ := origin["messageId"].(string)
		id, _ := stringField(row, "id")
		if copied != "" && copied != id && present[copied] {
			document.summary.Copied++
		} else {
			document.summary.Calls++
		}
	}
	for _, row := range document.tables["part"] {
		object, _ := objectField(row, "data")
		switch object["type"] {
		case "tool", "tool-call", "tool-result":
			document.summary.ToolParts++
		case "image", "file", "audio", "video":
			document.summary.MediaParts++
		}
		if object["bodyRef"] != nil || object["bodyReference"] != nil {
			document.summary.BodyRefs++
		}
	}
}
