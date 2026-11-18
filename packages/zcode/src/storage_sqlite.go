package zcode

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type historySQL struct {
	db          *sql.DB
	conn        *sql.Conn
	tx          *sql.Tx
	vfs         *historyVFS
	columns     map[string]map[string]bool
	limits      sdk.StorageLimits
	projected   int
	incarnation string
}

type historySession struct {
	id, parent, cwd, title, titleSource string
	created, updated                    int64
}

type historyMessage struct {
	id      string
	created int64
	data    map[string]any
	parts   []map[string]any
}

func openHistorySQL(ctx context.Context, root historyRoot, limits sdk.StorageLimits) (*historySQL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vfs, err := newHistoryVFS(root)
	if err != nil {
		return nil, err
	}
	reader := &historySQL{vfs: vfs, limits: limits, columns: map[string]map[string]bool{}}
	fail := func(err error) (*historySQL, error) {
		return nil, errors.Join(err, reader.Close())
	}
	for _, name := range []string{root.Family().Primary, root.Family().Primary + "-journal"} {
		file, err := vfs.probe(name)
		if errors.Is(err, fs.ErrNotExist) && strings.HasSuffix(name, "-journal") {
			continue
		}
		if err != nil {
			return fail(err)
		}
		info, statErr := file.Stat()
		if statErr != nil {
			return fail(statErr)
		}
		if strings.HasSuffix(name, "-journal") {
			if info.Size() != 0 {
				return fail(errors.New("ZCode history has an unresolved rollback journal"))
			}
		} else if info.Size() < 100 || info.Size() > limits.MaxReadBytes {
			return fail(errors.New("ZCode history database exceeds its rooted read budget"))
		} else {
			if err := root.Consume(100); err != nil {
				return fail(err)
			}
			wal, err := historyReadHeader(file)
			if err != nil {
				return fail(err)
			}
			if wal {
				for _, suffix := range []string{"-wal", "-shm"} {
					sidecar, err := vfs.probe(name + suffix)
					if err != nil {
						return fail(err)
					}
					info, err := sidecar.Stat()
					if err != nil || suffix == "-wal" && info.Size() != 0 && info.Size() < 32 ||
						suffix == "-shm" && (info.Size() < 32768 || info.Size() > 4<<20 || info.Size()%32768 != 0) {
						return fail(errors.New("ZCode WAL history requires its current complete sidecar family"))
					}
				}
			}
		}
	}
	reader.db, err = sql.Open("sqlite", "file:"+root.Family().Primary+"?mode=ro&vfs="+vfs.name)
	if err != nil {
		return fail(err)
	}
	reader.db.SetMaxOpenConns(1)
	reader.db.SetMaxIdleConns(1)
	reader.conn, err = reader.db.Conn(ctx)
	if err != nil {
		return fail(err)
	}
	for _, limit := range []struct{ id, value int }{
		{sqlite3.SQLITE_LIMIT_LENGTH, min(limits.MaxTextBytes, 1<<20)},
		{sqlite3.SQLITE_LIMIT_COLUMN, 256}, {sqlite3.SQLITE_LIMIT_EXPR_DEPTH, 64},
		{sqlite3.SQLITE_LIMIT_VDBE_OP, 100000}, {sqlite3.SQLITE_LIMIT_SQL_LENGTH, 64 << 10},
		{sqlite3.SQLITE_LIMIT_COMPOUND_SELECT, 16}, {sqlite3.SQLITE_LIMIT_VARIABLE_NUMBER, 128},
		{sqlite3.SQLITE_LIMIT_TRIGGER_DEPTH, 0},
	} {
		if _, err := sqlite.Limit(reader.conn, limit.id, limit.value); err != nil {
			return fail(err)
		}
	}
	for _, statement := range []string{"PRAGMA query_only=ON", "PRAGMA trusted_schema=OFF", "PRAGMA busy_timeout=0"} {
		if _, err := reader.conn.ExecContext(ctx, statement); err != nil {
			return fail(err)
		}
	}
	reader.tx, err = reader.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fail(err)
	}
	if err := reader.schema(ctx); err != nil {
		return fail(err)
	}
	identity, err := reader.databaseIdentity(ctx)
	if err != nil {
		return fail(err)
	}
	reader.incarnation = historyDigest([]string{vfs.identity, identity})
	return reader, nil
}

func (reader *historySQL) Close() error {
	var err error
	if reader.tx != nil {
		err = reader.tx.Rollback()
		if errors.Is(err, sql.ErrTxDone) {
			err = nil
		}
		reader.tx = nil
	}
	if reader.conn != nil {
		err = errors.Join(err, reader.conn.Close())
		reader.conn = nil
	}
	if reader.db != nil {
		err = errors.Join(err, reader.db.Close())
		reader.db = nil
	}
	if reader.vfs != nil {
		err = errors.Join(err, reader.vfs.Close())
	}
	return err
}

func (reader *historySQL) schema(ctx context.Context) error {
	var count int
	if err := reader.tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&count); err != nil {
		return err
	}
	if count > 1024 {
		return errors.New("ZCode history schema exceeds its bounds")
	}
	for _, table := range []struct {
		name     string
		required []string
	}{
		{"session", []string{"id", "parent_id", "directory", "title", "time_created", "time_updated"}},
		{"message", []string{"id", "session_id", "time_created", "data"}},
		{"part", []string{"id", "message_id", "session_id", "time_created", "data"}},
	} {
		var kind string
		if err := reader.tx.QueryRowContext(ctx,
			"SELECT type FROM pragma_table_list WHERE schema='main' AND name=?", table.name).Scan(&kind); err != nil || kind != "table" {
			return errors.New("unsupported ZCode history table")
		}
		rows, err := reader.tx.QueryContext(ctx, "SELECT name,hidden,pk FROM pragma_table_xinfo(?)", table.name)
		if err != nil {
			return err
		}
		columns := map[string]bool{}
		for rows.Next() {
			var name string
			var hidden, primary int
			if err := rows.Scan(&name, &hidden, &primary); err != nil || hidden != 0 || len(columns) >= 256 ||
				name == "id" && primary != 1 {
				rows.Close()
				return errors.New("unsupported generated or oversized ZCode history schema")
			}
			columns[name] = true
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return err
		}
		for _, name := range table.required {
			if !columns[name] {
				return errors.New("ZCode history is missing a required column")
			}
		}
		reader.columns[table.name] = columns
	}
	return nil
}

func (reader *historySQL) databaseIdentity(ctx context.Context) (string, error) {
	var kind string
	err := reader.tx.QueryRowContext(ctx,
		"SELECT type FROM pragma_table_list WHERE schema='main' AND name='zcode_database_identity'").Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil || kind != "table" {
		return "", errors.New("unsupported ZCode database identity table")
	}
	rows, err := reader.tx.QueryContext(ctx, "SELECT name,hidden,pk FROM pragma_table_xinfo('zcode_database_identity')")
	if err != nil {
		return "", err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		var hidden, primary int
		if err := rows.Scan(&name, &hidden, &primary); err != nil || hidden != 0 || len(columns) >= 256 ||
			name == "scope" && primary != 1 || name != "scope" && primary != 0 {
			rows.Close()
			return "", errors.New("unsupported ZCode database identity columns")
		}
		columns[name] = true
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return "", err
	}
	if !columns["scope"] || !columns["identity"] {
		return "", errors.New("ZCode database identity is missing a required column")
	}
	var identity string
	err = reader.tx.QueryRowContext(ctx,
		"SELECT CASE WHEN length(CAST(identity AS BLOB))<=256 THEN identity END FROM zcode_database_identity WHERE scope='session' LIMIT 1").Scan(&identity)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil || !utf8.ValidString(identity) || strings.ContainsRune(identity, 0) {
		return "", errors.New("invalid ZCode database identity")
	}
	return strings.TrimSpace(identity), nil
}

func historyID(value string) bool {
	return value != "" && len(value) <= 128 && sdk.SessionSourcePath(value) && !strings.Contains(value, "/")
}

func (reader *historySQL) sessionQuery() string {
	source, parent, deleted := "''", "COALESCE(parent_id,'')", ""
	if reader.columns["session"]["title_source"] {
		source = "COALESCE(title_source,'')"
	}
	if reader.columns["session"]["time_deleted"] {
		deleted = " AND time_deleted IS NULL"
	}
	return "SELECT CASE WHEN length(CAST(id AS BLOB))<=128 THEN id END," +
		"CASE WHEN length(CAST(" + parent + " AS BLOB))<=128 THEN " + parent + " END," +
		"CASE WHEN length(CAST(directory AS BLOB))<=4096 THEN directory END," +
		"CASE WHEN length(CAST(title AS BLOB))<=4096 THEN title END," +
		source + ",time_created,time_updated FROM session WHERE 1=1" + deleted
}

func (reader *historySQL) sessions(ctx context.Context, id string) ([]historySession, bool, error) {
	query, arguments := reader.sessionQuery(), []any{}
	if id != "" {
		query += " AND id=?"
		arguments = append(arguments, id)
	}
	query += " ORDER BY time_updated DESC,id LIMIT ?"
	arguments = append(arguments, reader.limits.MaxEntries+1)
	rows, err := reader.tx.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []historySession{}
	for rows.Next() {
		var value historySession
		if err := rows.Scan(&value.id, &value.parent, &value.cwd, &value.title, &value.titleSource, &value.created, &value.updated); err != nil {
			return nil, false, err
		}
		if !historyID(value.id) || value.parent != "" && !historyID(value.parent) ||
			!utf8.ValidString(value.cwd+value.title) || strings.ContainsRune(value.cwd, 0) ||
			historyTime(value.created) == "" || historyTime(value.updated) == "" {
			return nil, false, errors.New("invalid ZCode history session metadata")
		}
		if len(result) == reader.limits.MaxEntries {
			return result, true, nil
		}
		result = append(result, value)
	}
	return result, false, rows.Err()
}

func (reader *historySQL) messages(ctx context.Context, session string) ([]historyMessage, error) {
	order := "time_created,rowid"
	if reader.columns["message"]["sequence"] {
		order = "sequence IS NULL,sequence," + order
	}
	rows, err := reader.tx.QueryContext(ctx, "SELECT id,time_created,data FROM message WHERE session_id=? ORDER BY "+order+" LIMIT ?",
		session, reader.limits.MaxParts+1)
	if err != nil {
		return nil, err
	}
	messages := []historyMessage{}
	indices := map[string]int{}
	for rows.Next() {
		var message historyMessage
		var data string
		if err := rows.Scan(&message.id, &message.created, &data); err != nil {
			rows.Close()
			return nil, err
		}
		if !historyID(message.id) || len(messages) >= reader.limits.MaxParts || historyTime(message.created) == "" {
			rows.Close()
			return nil, errors.New("ZCode history message identity or count exceeds its bounds")
		}
		message.data, err = reader.object(data)
		if err != nil {
			rows.Close()
			return nil, err
		}
		indices[message.id] = len(messages)
		messages = append(messages, message)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, err
	}
	order = "time_created,id"
	if reader.columns["part"]["sequence"] {
		order = "sequence IS NULL,sequence," + order
	}
	rows, err = reader.tx.QueryContext(ctx, "SELECT id,message_id,data FROM part WHERE session_id=? ORDER BY message_id,"+order+" LIMIT ?",
		session, reader.limits.MaxParts+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, owner, data string
		if err := rows.Scan(&id, &owner, &data); err != nil {
			return nil, err
		}
		index, exists := indices[owner]
		count++
		if !historyID(id) || !exists || count > reader.limits.MaxParts {
			return nil, errors.New("ZCode history part has a foreign owner or exceeds its bounds")
		}
		part, err := reader.object(data)
		if err != nil {
			return nil, err
		}
		messages[index].parts = append(messages[index].parts, part)
	}
	return messages, rows.Err()
}

func (reader *historySQL) object(data string) (map[string]any, error) {
	reader.projected += len(data)
	if reader.projected > reader.limits.MaxTextBytes || !utf8.ValidString(data) {
		return nil, errors.New("ZCode history JSON exceeds its projection budget")
	}
	value, err := protocol.ReadObject([]byte(data))
	if err != nil {
		return nil, errors.New("invalid ZCode history JSON")
	}
	return value, nil
}

func (reader *historySQL) result(ctx context.Context, request sdk.StorageRequest) (sdk.StorageResult, error) {
	id := ""
	if request.Locator != nil {
		id = request.Locator.SessionID
		if *request.Locator != reader.locator(request.Lease, id) {
			return sdk.StorageResult{}, errors.New("foreign ZCode history locator")
		}
	}
	sessions, cut, err := reader.sessions(ctx, id)
	if err != nil {
		return sdk.StorageResult{}, err
	}
	if request.Locator != nil && len(sessions) != 1 {
		return sdk.StorageResult{}, errors.New("selected ZCode history session is unavailable")
	}
	entries := []sdk.StorageEntry{}
	for _, session := range sessions {
		messages, err := reader.messages(ctx, session.id)
		if err != nil {
			return sdk.StorageResult{}, err
		}
		if request.Mode == "transcript" {
			return historyTranscript(request, messages)
		}
		summary, err := reader.summary(ctx, session, messages)
		if err != nil {
			return sdk.StorageResult{}, err
		}
		if request.Mode == "summary" {
			return sdk.StorageResult{SnapshotRef: request.Lease.SnapshotRef, SessionID: id,
				Mode: request.Mode, Summary: &summary}, nil
		}
		reader.projected += len(summary.Title) + len(summary.Cwd) + len(summary.Usage)*128
		if reader.projected > reader.limits.MaxTextBytes {
			return sdk.StorageResult{}, errors.New("ZCode history catalog exceeds its projection budget")
		}
		entries = append(entries, sdk.StorageEntry{Locator: reader.locator(request.Lease, session.id), Summary: summary})
	}
	return historyPage(request, entries, cut)
}

func historyReadHeader(file io.ReaderAt) (bool, error) {
	var header [100]byte
	if _, err := file.ReadAt(header[:], 0); err != nil || string(header[:16]) != "SQLite format 3\x00" ||
		(header[18] != 1 && header[18] != 2) || header[18] != header[19] {
		return false, errors.New("invalid ZCode SQLite database header")
	}
	return header[18] == 2, nil
}
