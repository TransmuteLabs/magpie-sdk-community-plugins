package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"slices"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func legacyFiles(ctx context.Context, stream *sdk.Stream, lease sdk.StorageReadLease, directory string) ([]sdk.StorageFileEntry, error) {
	result := []sdk.StorageFileEntry{}
	cursor := ""
	for {
		page, err := stream.StorageFiles(ctx, sdk.StorageFilesRequest{Lease: lease, Directory: directory, Cursor: cursor, Limit: 256})
		if err != nil {
			return nil, err
		}
		result = append(result, page.Entries...)
		if len(result) > lease.Limits.MaxEntries {
			return nil, errors.New("OpenCode legacy directory budget exceeded")
		}
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == cursor {
			return nil, errors.New("OpenCode legacy cursor did not advance")
		}
		cursor = page.NextCursor
	}
	return result, nil
}

func legacyJSON(ctx context.Context, stream *sdk.Stream, lease sdk.StorageReadLease, file sdk.StorageFileEntry) (map[string]json.RawMessage, error) {
	if file.Kind != "file" || file.Size < 0 || file.Size > int64(lease.Limits.MaxTextBytes) {
		return nil, errors.New("OpenCode legacy document budget exceeded")
	}
	body := make([]byte, 0, int(file.Size))
	for {
		chunk, err := stream.StorageBytes(ctx, sdk.StorageBytesRequest{Lease: lease, FileRef: file.FileRef, Offset: int64(len(body)), Length: 256 << 10})
		if err != nil {
			return nil, err
		}
		body = append(body, chunk.Bytes...)
		if int64(len(body)) > file.Size || len(chunk.Bytes) == 0 && !chunk.EOF {
			return nil, errors.New("OpenCode legacy file changed during snapshot")
		}
		if chunk.EOF {
			break
		}
	}
	if int64(len(body)) != file.Size {
		return nil, errors.New("OpenCode legacy file length changed")
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(body, &value) != nil || value == nil {
		return nil, errors.New("malformed OpenCode legacy JSON document")
	}
	return value, nil
}

func listLegacyHistory(ctx context.Context, stream *sdk.Stream, lease sdk.StorageReadLease) ([]sdk.StorageEntry, bool, error) {
	projects, err := legacyFiles(ctx, stream, lease, "storage/session")
	if err != nil {
		return nil, false, err
	}
	result, seen, text := []sdk.StorageEntry{}, map[string]bool{}, 0
	for _, project := range projects {
		if project.Kind != "directory" {
			continue
		}
		files, err := legacyFiles(ctx, stream, lease, project.RelativeName)
		if err != nil {
			return nil, false, err
		}
		for _, file := range files {
			if file.Kind != "file" || !strings.HasSuffix(file.RelativeName, ".json") {
				continue
			}
			value, err := legacyJSON(ctx, stream, lease, file)
			if err != nil {
				return nil, false, err
			}
			id := historyString(value, "id")
			if !historyLegacyID(id) || path.Base(file.RelativeName) != id+".json" || seen[id] {
				return nil, false, errors.New("ambiguous OpenCode legacy session identity")
			}
			seen[id] = true
			summary, err := legacySummary(ctx, stream, lease, id, value)
			if err != nil {
				return nil, false, err
			}
			encoded, _ := json.Marshal(summary)
			text += len(encoded)
			if len(result) >= lease.Limits.MaxEntries || text > lease.Limits.MaxTextBytes {
				return result, true, nil
			}
			result = append(result, sdk.StorageEntry{Locator: sdk.StorageSessionLocator{RootRef: lease.RootRef,
				SessionID: id, Key: "legacy:" + file.RelativeName}, Summary: summary})
		}
	}
	slices.SortFunc(result, func(first, second sdk.StorageEntry) int {
		return strings.Compare(first.Locator.SessionID, second.Locator.SessionID)
	})
	return result, false, nil
}

func historyLegacyID(id string) bool {
	return id != "" && len(id) <= 128 && sdk.SessionSourcePath(id) && !strings.Contains(id, "/")
}

func legacySummary(ctx context.Context, stream *sdk.Stream, lease sdk.StorageReadLease, id string, value map[string]json.RawMessage) (sdk.SessionSourceSummary, error) {
	var stamp struct {
		Created int64 `json:"created"`
		Updated int64 `json:"updated"`
	}
	if json.Unmarshal(value["time"], &stamp) != nil {
		return sdk.SessionSourceSummary{}, errors.New("malformed OpenCode legacy session time")
	}
	summary := sdk.SessionSourceSummary{Title: historyString(value, "title"), Cwd: historyString(value, "directory"),
		ParentID: historyString(value, "parentID"), Start: historyTime(stamp.Created), Last: historyTime(stamp.Updated),
		Usage: []sdk.SessionUsagePoint{}}
	messages, err := legacyMessages(ctx, stream, lease, id)
	if err != nil {
		return summary, err
	}
	for _, message := range messages {
		if message.kind != "assistant" {
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

func legacyMessages(ctx context.Context, stream *sdk.Stream, lease sdk.StorageReadLease, session string) ([]historyMessage, error) {
	files, err := legacyFiles(ctx, stream, lease, "storage/message/"+session)
	if err != nil {
		return nil, err
	}
	messages := []historyMessage{}
	for _, file := range files {
		if file.Kind != "file" || !strings.HasSuffix(file.RelativeName, ".json") {
			continue
		}
		value, err := legacyJSON(ctx, stream, lease, file)
		if err != nil {
			return nil, err
		}
		id := historyString(value, "id")
		if !historyLegacyID(id) || path.Base(file.RelativeName) != id+".json" || historyString(value, "sessionID") != session {
			return nil, errors.New("foreign OpenCode legacy message")
		}
		var stamp struct {
			Created int64 `json:"created"`
		}
		if json.Unmarshal(value["time"], &stamp) != nil {
			return nil, errors.New("malformed OpenCode legacy message time")
		}
		message := historyMessage{id: id, kind: historyString(value, "role"), created: stamp.Created, seq: stamp.Created, data: value}
		parts, err := legacyFiles(ctx, stream, lease, "storage/part/"+id)
		if err != nil {
			return nil, err
		}
		for _, part := range parts {
			if part.Kind != "file" || !strings.HasSuffix(part.RelativeName, ".json") {
				continue
			}
			content, err := legacyJSON(ctx, stream, lease, part)
			if err != nil {
				return nil, err
			}
			if historyString(content, "messageID") != id || historyString(content, "sessionID") != session {
				return nil, errors.New("foreign OpenCode legacy part")
			}
			message.content = append(message.content, content)
		}
		if len(messages) >= lease.Limits.MaxParts {
			return nil, errors.New("OpenCode legacy message budget exceeded")
		}
		messages = append(messages, message)
	}
	slices.SortFunc(messages, func(first, second historyMessage) int {
		if first.created < second.created {
			return -1
		}
		if first.created > second.created {
			return 1
		}
		return strings.Compare(first.id, second.id)
	})
	return messages, nil
}

func readLegacyHistory(ctx context.Context, stream *sdk.Stream, request sdk.StorageRequest, result sdk.StorageResult) (sdk.StorageResult, error) {
	name := strings.TrimPrefix(request.Locator.Key, "legacy:")
	segments := strings.Split(name, "/")
	if len(segments) != 4 || segments[0] != "storage" || segments[1] != "session" ||
		!sdk.SessionSourcePath(name) || segments[3] != request.Locator.SessionID+".json" {
		return result, errors.New("invalid OpenCode legacy locator")
	}
	files, err := legacyFiles(ctx, stream, request.Lease, path.Dir(name))
	if err != nil {
		return result, err
	}
	index := slices.IndexFunc(files, func(file sdk.StorageFileEntry) bool { return file.RelativeName == name })
	if index < 0 {
		return result, errors.New("selected OpenCode legacy session disappeared")
	}
	value, err := legacyJSON(ctx, stream, request.Lease, files[index])
	if err != nil || historyString(value, "id") != request.Locator.SessionID {
		return result, errors.New("selected OpenCode legacy session identity changed")
	}
	if request.Mode == "summary" {
		summary, err := legacySummary(ctx, stream, request.Lease, request.Locator.SessionID, value)
		result.Summary, result.Parts = &summary, nil
		return result, err
	}
	messages, err := legacyMessages(ctx, stream, request.Lease, request.Locator.SessionID)
	if err != nil {
		return result, err
	}
	projection := &historyProjection{request: request, result: result}
	for _, message := range messages {
		if err := projection.message(ctx, nil, message); err != nil {
			return result, err
		}
		if projection.result.Cut {
			break
		}
	}
	return projection.result, nil
}
