package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/extension/native"
	_ "modernc.org/sqlite"
)

const HistorySourceID = "opencode-history"

type historyRoot interface {
	Open(string) (*os.File, error)
	Consume(int64) error
	Family() sdk.StorageFamily
}

type historySQL struct {
	db     *sql.DB
	tx     *sql.Tx
	vfs    *historyVFS
	tables map[string]bool
}

type historyCatalog struct {
	lease   sdk.StorageReadLease
	entries []sdk.StorageEntry
	cut     bool
	expires time.Time
}

type historyReader struct {
	mu       sync.Mutex
	catalogs map[string]historyCatalog
}

func HistoryDescriptor() sdk.StorageSourceDescriptor {
	return sdk.StorageSourceDescriptor{ID: HistorySourceID, Agent: "opencode", Codec: "opencode-storage-v2",
		Roots: []sdk.SessionSourceStore{{Slot: "data", NativeRootSlot: "opencode.data"}},
		Families: []sdk.StorageFamily{{ID: "history", Primary: "opencode.db",
			Sidecars: []string{"opencode.db-wal", "opencode.db-shm"},
			Trees:    []string{"storage/session", "storage/message", "storage/part", "storage/media"}}},
		Transcript: true, Resume: "native"}
}

func declareHistory(manifest *sdk.Manifest) {
	manifest.StorageSources = []sdk.StorageSourceDescriptor{HistoryDescriptor()}
	if manifest.Contract.Version.Minor < 8 {
		manifest.Contract.Version.Minor = 8
	}
	if !slices.Contains(manifest.Contract.Capabilities, sdk.StorageSource) {
		manifest.Contract.Capabilities = append(manifest.Contract.Capabilities, sdk.StorageSource)
	}
	if manifest.Compatibility != nil {
		manifest.Compatibility.HostAPI = "^1." + strconv.Itoa(manifest.Contract.Version.Minor)
	}
}

func EnableHistory(module *sdk.Module) {
	declareHistory(&module.Manifest)
	reader := &historyReader{catalogs: map[string]historyCatalog{}}
	module.Handlers[sdk.MethodStorageList] = reader.list
	module.Handlers[sdk.MethodStorageRead] = reader.read
}

func openHistorySQL(ctx context.Context, root historyRoot) (*historySQL, error) {
	vfs, err := newHistoryVFS(root)
	if err != nil {
		return nil, err
	}
	return openHistorySQLVFS(ctx, root, vfs)
}

func openHistorySQLVFS(ctx context.Context, root historyRoot, vfs *historyVFS) (*historySQL, error) {
	if _, err := vfs.probe(root.Family().Primary); err != nil {
		vfs.Close()
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+root.Family().Primary+"?mode=ro&vfs="+vfs.name)
	if err != nil {
		vfs.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		db.Close()
		vfs.Close()
		return nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		db.Close()
		vfs.Close()
		return nil, err
	}
	reader := &historySQL{db: db, tx: tx, vfs: vfs, tables: map[string]bool{}}
	rows, err := tx.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name IN ('session','session_v2','message','part','session_message','session_part','instruction_state','blob','blob_ref','kv')")
	if err != nil {
		reader.Close()
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			reader.Close()
			return nil, err
		}
		reader.tables[name] = true
	}
	if err := rows.Err(); err != nil {
		reader.Close()
		return nil, err
	}
	if !reader.tables["session_v2"] && !reader.tables["session"] {
		reader.Close()
		return nil, errors.New("unsupported OpenCode database schema")
	}
	return reader, nil
}

func (reader *historySQL) Close() error {
	err := reader.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		err = nil
	}
	return errors.Join(err, reader.db.Close(), reader.vfs.Close())
}

func historyRequest(call sdk.Call) (sdk.StorageRequest, error) {
	var request sdk.StorageRequest
	err := decode(call.Params, &request)
	return request, err
}

func (reader *historyReader) list(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	request, err := historyRequest(call)
	if err != nil {
		return nil, err
	}
	reader.mu.Lock()
	now := time.Now()
	for key, catalog := range reader.catalogs {
		if !now.Before(catalog.expires) {
			delete(reader.catalogs, key)
		}
	}
	catalog, found := reader.catalogs[request.Lease.SnapshotRef]
	reader.mu.Unlock()
	offset := 0
	if request.Cursor != "" {
		prefix := request.Lease.SnapshotRef + ":"
		if !found || catalog.lease != request.Lease || !strings.HasPrefix(request.Cursor, prefix) {
			return nil, errors.New("expired or foreign OpenCode storage cursor")
		}
		offset, err = strconv.Atoi(strings.TrimPrefix(request.Cursor, prefix))
		if err != nil || offset < 0 || offset >= len(catalog.entries) {
			return nil, errors.New("invalid OpenCode storage cursor")
		}
	} else {
		root, err := native.OpenStorage(ctx, stream, request.Lease)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		sqlite, err := openHistorySQL(ctx, root)
		var entries []sdk.StorageEntry
		var cut bool
		if errors.Is(err, fs.ErrNotExist) {
			entries, cut, err = listLegacyHistory(ctx, stream, request.Lease)
		} else if err == nil {
			entries, cut, err = sqlite.catalog(ctx, request.Lease)
			closeErr := sqlite.Close()
			if err == nil && closeErr != nil {
				err = closeErr
			}
		}
		if err != nil {
			return nil, err
		}
		expires := now.Add(10 * time.Second)
		if deadline, exists := ctx.Deadline(); exists && deadline.Before(expires) {
			expires = deadline
		}
		catalog = historyCatalog{lease: request.Lease, entries: entries, cut: cut, expires: expires}
	}
	end := min(offset+request.Limit, len(catalog.entries))
	result := sdk.StorageResult{SnapshotRef: request.Lease.SnapshotRef, Entries: slices.Clone(catalog.entries[offset:end]), Cut: catalog.cut}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if end < len(catalog.entries) {
		if !found && len(reader.catalogs) >= sdk.MaxConcurrent {
			return nil, errors.New("OpenCode storage snapshot capacity exceeded")
		}
		reader.catalogs[request.Lease.SnapshotRef] = catalog
		result.NextCursor = fmt.Sprintf("%s:%d", request.Lease.SnapshotRef, end)
	} else {
		delete(reader.catalogs, request.Lease.SnapshotRef)
	}
	return result, nil
}

func (reader *historyReader) read(ctx context.Context, call sdk.Call, stream *sdk.Stream) (response any, readErr error) {
	request, err := historyRequest(call)
	if err != nil || request.Locator == nil {
		return nil, errors.New("OpenCode history requires its selected locator")
	}
	result := sdk.StorageResult{SnapshotRef: request.Lease.SnapshotRef, Mode: request.Mode,
		SessionID: request.Locator.SessionID, Parts: []sdk.SessionSourcePart{}}
	root, err := native.OpenStorage(ctx, stream, request.Lease)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if strings.HasPrefix(request.Locator.Key, "legacy:") {
		file, openErr := root.Open(root.Family().Primary)
		if openErr == nil {
			closeHistoryProbe(file)
			return nil, errors.New("OpenCode database superseded selected legacy storage")
		}
		if !errors.Is(openErr, fs.ErrNotExist) {
			return nil, openErr
		}
		return readLegacyHistory(ctx, stream, request, result)
	}
	sqlite, err := openHistorySQL(ctx, root)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := sqlite.Close(); err != nil {
			response = nil
			readErr = errors.Join(readErr, err)
		}
	}()
	version, id, valid := strings.Cut(request.Locator.Key, ":")
	if !valid || id != request.Locator.SessionID || version != "v1" && version != "v2" {
		return nil, errors.New("invalid OpenCode database locator")
	}
	if request.Mode == "summary" {
		entry, err := sqlite.summary(ctx, version, id, request.Lease.Limits)
		result.Summary = &entry
		result.Parts = nil
		return result, err
	}
	messages, err := sqlite.messages(ctx, version, id, request.Lease.Limits)
	if err != nil {
		return nil, err
	}
	if version == "v2" {
		messages, err = sqlite.localHistory(ctx, id, messages, request.Lease.Limits)
		if err != nil {
			return nil, err
		}
	}
	result, err = sqlite.transcript(ctx, request, result, messages)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (reader *historySQL) migrationComplete(ctx context.Context) (bool, error) {
	if !reader.tables["kv"] {
		return false, nil
	}
	var value sql.NullString
	err := reader.tx.QueryRowContext(ctx, "SELECT CASE WHEN length(CAST(value AS BLOB)) <= 65536 THEN value END FROM kv WHERE key='migration.v1-v2'").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state struct {
		Phase string `json:"phase"`
	}
	if !value.Valid || json.Unmarshal([]byte(value.String), &state) != nil {
		return false, errors.New("invalid OpenCode migration state")
	}
	return state.Phase == "completed", nil
}

func (reader *historySQL) catalog(ctx context.Context, lease sdk.StorageReadLease) ([]sdk.StorageEntry, bool, error) {
	complete, err := reader.migrationComplete(ctx)
	if err != nil {
		return nil, false, err
	}
	versions := []string{}
	if reader.tables["session_v2"] {
		versions = append(versions, "v2")
	}
	if reader.tables["session"] && !complete {
		versions = append(versions, "v1")
	}
	entries, seen, text := []sdk.StorageEntry{}, map[string]bool{}, 0
	for _, version := range versions {
		table := "session_v2"
		if version == "v1" {
			table = "session"
		}
		rows, err := reader.tx.QueryContext(ctx, "SELECT CASE WHEN length(CAST(id AS BLOB))<=128 THEN id END FROM "+table+" ORDER BY id LIMIT ?", lease.Limits.MaxEntries+1)
		if err != nil {
			return nil, false, err
		}
		var selected []sdk.StorageEntry
		for rows.Next() {
			var id sql.NullString
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, false, err
			}
			if !id.Valid || !historyLegacyID(id.String) {
				rows.Close()
				return nil, false, errors.New("invalid or oversized OpenCode stored session ID")
			}
			if seen[id.String] {
				continue
			}
			seen[id.String] = true
			selected = append(selected, sdk.StorageEntry{Locator: sdk.StorageSessionLocator{
				RootRef: lease.RootRef, SessionID: id.String, Key: version + ":" + id.String}})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false, err
		}
		for _, entry := range selected {
			if len(entries) >= lease.Limits.MaxEntries {
				return entries, true, nil
			}
			summary, err := reader.summary(ctx, version, entry.Locator.SessionID, lease.Limits)
			if err != nil {
				return nil, false, err
			}
			encoded, _ := json.Marshal(summary)
			text += len(encoded)
			if text > lease.Limits.MaxTextBytes {
				return entries, true, nil
			}
			entry.Summary = summary
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(first, second sdk.StorageEntry) int {
		return strings.Compare(first.Locator.SessionID, second.Locator.SessionID)
	})
	return entries, false, nil
}

func historyTime(milliseconds int64) string {
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339Nano)
}

type historyMessage struct {
	id      string
	kind    string
	seq     int64
	created int64
	data    map[string]json.RawMessage
	content []map[string]json.RawMessage
}

func (reader *historySQL) summary(ctx context.Context, version, id string, limits sdk.StorageLimits) (sdk.SessionSourceSummary, error) {
	table := "session_v2"
	if version == "v1" {
		table = "session"
	}
	var summary sdk.SessionSourceSummary
	var title, cwd, parent sql.NullString
	var created, updated int64
	err := reader.tx.QueryRowContext(ctx, "SELECT CASE WHEN length(CAST(coalesce(title,'') AS BLOB))<=4096 THEN coalesce(title,'') END,CASE WHEN length(CAST(directory AS BLOB))<=4096 THEN directory END,CASE WHEN length(CAST(coalesce(parent_id,'') AS BLOB))<=128 THEN coalesce(parent_id,'') END,time_created,time_updated FROM "+table+" WHERE id=?", id).
		Scan(&title, &cwd, &parent, &created, &updated)
	if err != nil {
		return summary, err
	}
	if !title.Valid || !cwd.Valid || !parent.Valid {
		return summary, errors.New("OpenCode stored session metadata budget exceeded")
	}
	summary.Title, summary.Cwd, summary.ParentID = title.String, cwd.String, parent.String
	summary.Start, summary.Last = historyTime(created), historyTime(updated)
	messages, err := reader.messages(ctx, version, id, limits)
	if err != nil {
		return summary, err
	}
	summary.Usage = []sdk.SessionUsagePoint{}
	for _, message := range messages {
		if message.kind != "assistant" && message.kind != "compaction" {
			continue
		}
		point, found, err := historyUsage(message)
		if err != nil {
			return summary, err
		}
		if found {
			summary.Usage = append(summary.Usage, point)
		}
	}
	return summary, nil
}

func (reader *historySQL) messages(ctx context.Context, version, session string, limits sdk.StorageLimits) ([]historyMessage, error) {
	if err := reader.selectedSession(ctx, version, session); err != nil {
		return nil, err
	}
	bounded := "CASE WHEN length(CAST(data AS BLOB)) <= ? THEN data END"
	query := "SELECT id,type,seq,time_created," + bounded + " FROM session_message WHERE session_id=? ORDER BY seq,id LIMIT ?"
	if version == "v1" {
		query = "SELECT id,'legacy',time_created,time_created," + bounded + " FROM message WHERE session_id=? ORDER BY time_created,id LIMIT ?"
	}
	rows, err := reader.tx.QueryContext(ctx, query, limits.MaxTextBytes, session, limits.MaxParts+1)
	if err != nil {
		return nil, err
	}
	messages, total, count := []historyMessage{}, 0, 0
	for rows.Next() {
		var message historyMessage
		var data sql.NullString
		if err := rows.Scan(&message.id, &message.kind, &message.seq, &message.created, &data); err != nil {
			rows.Close()
			return nil, err
		}
		total += len(data.String)
		if !data.Valid || len(messages) >= limits.MaxParts || total > limits.MaxTextBytes {
			rows.Close()
			return nil, errors.New("OpenCode stored message budget exceeded")
		}
		if err := json.Unmarshal([]byte(data.String), &message.data); err != nil {
			rows.Close()
			return nil, errors.New("malformed OpenCode stored message")
		}
		if version == "v1" {
			message.kind = historyString(message.data, "role")
		}
		if err := json.Unmarshal(message.data["content"], &message.content); err != nil && message.data["content"] != nil {
			rows.Close()
			return nil, errors.New("malformed OpenCode inline content")
		}
		count += len(message.content)
		if count > limits.MaxParts {
			rows.Close()
			return nil, errors.New("OpenCode aggregate stored part budget exceeded")
		}
		messages = append(messages, message)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for index := range messages {
		message := &messages[index]
		if version == "v2" && (message.kind != "assistant" || !reader.tables["session_part"]) {
			continue
		}
		query := "SELECT " + bounded + " FROM session_part WHERE session_id=? AND message_id=? ORDER BY ordinal LIMIT ?"
		if version == "v1" {
			query = "SELECT " + bounded + " FROM part WHERE session_id=? AND message_id=? ORDER BY id LIMIT ?"
		}
		parts, err := reader.tx.QueryContext(ctx, query, limits.MaxTextBytes-total, session, message.id, limits.MaxParts+1)
		if err != nil {
			return nil, err
		}
		content := []map[string]json.RawMessage{}
		for parts.Next() {
			var data sql.NullString
			if err := parts.Scan(&data); err != nil {
				parts.Close()
				return nil, err
			}
			total += len(data.String)
			count++
			if !data.Valid || total > limits.MaxTextBytes || count > limits.MaxParts {
				parts.Close()
				return nil, errors.New("OpenCode stored part budget exceeded")
			}
			var part map[string]json.RawMessage
			if json.Unmarshal([]byte(data.String), &part) != nil {
				parts.Close()
				return nil, errors.New("malformed OpenCode stored part")
			}
			content = append(content, part)
		}
		err = parts.Err()
		parts.Close()
		if err != nil {
			return nil, err
		}
		message.content = append(content, message.content...)
	}
	return messages, nil
}

func (reader *historySQL) selectedSession(ctx context.Context, version, id string) error {
	table := "session_v2"
	if version == "v1" {
		table = "session"
		complete, err := reader.migrationComplete(ctx)
		if err != nil {
			return err
		}
		if complete {
			return errors.New("OpenCode migration superseded the selected V1 session")
		}
		if reader.tables["session_v2"] {
			var found bool
			if err := reader.tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM session_v2 WHERE id=?)", id).Scan(&found); err != nil {
				return err
			}
			if found {
				return errors.New("OpenCode V2 superseded the selected V1 session")
			}
		}
	} else if version != "v2" {
		return errors.New("unknown OpenCode session version")
	}
	if !reader.tables[table] {
		return errors.New("selected OpenCode session table is absent")
	}
	var found bool
	if err := reader.tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE id=?)", id).Scan(&found); err != nil {
		return err
	}
	if !found {
		return errors.New("selected OpenCode session was removed")
	}
	return nil
}

func historyString(value map[string]json.RawMessage, key string) string {
	var result string
	_ = json.Unmarshal(value[key], &result)
	return result
}

func historyObject(value map[string]json.RawMessage, key string) map[string]json.RawMessage {
	var result map[string]json.RawMessage
	_ = json.Unmarshal(value[key], &result)
	return result
}
