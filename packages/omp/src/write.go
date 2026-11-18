package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Encoded struct {
	ID       string
	Filename string
	Journal  []byte
	Assets   []Asset
}

// Encode returns a journal and content-addressed assets. It never publishes
// files. An archived document is restored only when its projection is intact.
func Encode(ctx context.Context, document Document, now time.Time) (Encoded, error) {
	return encodeWithImageMode(ctx, document, now, true)
}

func encodeWithImageMode(ctx context.Context, document Document, now time.Time, externalize bool) (Encoded, error) {
	if err := ctx.Err(); err != nil {
		return Encoded{}, err
	}
	if document.Schema != 1 {
		return Encoded{}, errors.New("unsupported OMP conversation schema")
	}
	if document.Archive != nil {
		if hashBytes(document.Archive.Journal) != document.Archive.JournalSHA256 {
			return Encoded{}, errors.New("archived OMP journal digest changed")
		}
		original, err := Decode(ctx, document.Archive.Journal, document.Archive.Assets)
		if err != nil {
			return Encoded{}, err
		}
		if projectionHash(document) != document.Archive.ProjectionSHA256 ||
			projectionHash(original) != document.Archive.ProjectionSHA256 {
			return Encoded{}, errors.New("archived OMP projection changed; create a new conversation document explicitly")
		}
		return Encoded{ID: original.ID, Filename: journalName(original.Timestamp, original.ID),
			Journal: bytes.Clone(document.Archive.Journal), Assets: cloneAssets(original.Archive.Assets)}, nil
	}
	if document.Timestamp.IsZero() {
		document.Timestamp = now
	}
	if document.Timestamp.IsZero() {
		return Encoded{}, errors.New("a session timestamp is required")
	}
	if document.ID == "" {
		var err error
		document.ID, err = newUUID(document.Timestamp, 7)
		if err != nil {
			return Encoded{}, err
		}
	}
	if !validUUID(document.ID) || strings.ContainsAny(document.Cwd, "\x00\r\n") || document.Cwd == "" {
		return Encoded{}, errors.New("a UUIDv4/v7 session ID and explicit working directory are required")
	}
	stamp := carryStamp(document.Timestamp, document.Timestamp)
	slot := func(pad string) []byte {
		body, _ := json.Marshal(struct {
			Type      string `json:"type"`
			V         int    `json:"v"`
			Title     string `json:"title"`
			UpdatedAt string `json:"updatedAt"`
			Pad       string `json:"pad"`
		}{"title", 1, "", stamp, pad})
		return body
	}
	var journal bytes.Buffer
	journal.Write(slot(strings.Repeat(" ", 255-len(slot("")))))
	journal.WriteByte('\n')
	write := func(value any) error {
		body, err := json.Marshal(value)
		if err == nil {
			journal.Write(body)
			journal.WriteByte('\n')
		}
		return err
	}
	if err := write(struct {
		Type      string `json:"type"`
		Version   int    `json:"version"`
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Cwd       string `json:"cwd"`
		Title     string `json:"title,omitempty"`
	}{"session", 3, document.ID, stamp, document.Cwd, document.Title}); err != nil {
		return Encoded{}, err
	}
	parent := ""
	add := func(at time.Time, message map[string]any) error {
		id, err := newUUID(time.Time{}, 4)
		if err != nil {
			return err
		}
		var previous *string
		if parent != "" {
			copy := parent
			previous = &copy
		}
		if at.IsZero() {
			at = document.Timestamp
		}
		message["timestamp"] = at.UnixMilli()
		err = write(struct {
			Type      string         `json:"type"`
			ID        string         `json:"id"`
			ParentID  *string        `json:"parentId"`
			Timestamp string         `json:"timestamp"`
			Message   map[string]any `json:"message"`
		}{"message", id, previous, carryStamp(at, document.Timestamp), message})
		parent = id
		return err
	}
	assets := map[string][]byte{}
	content := func(blocks []Content) ([]any, error) {
		return encodeContentWithImageMode(blocks, assets, externalize)
	}
	assistant := func(block any, stop string) map[string]any {
		return map[string]any{"role": "assistant", "content": []any{block},
			"api": "anthropic-messages", "provider": "anthropic", "model": "<synthetic>",
			"stopReason": stop, "usage": map[string]any{
				"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "totalTokens": 0,
				"cost": map[string]int{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "total": 0},
			}}
	}
	var pending map[string]any
	var pendingAt time.Time
	pendingPhase := ""
	flushAssistant := func() error {
		if pending == nil {
			return nil
		}
		if err := add(pendingAt, pending); err != nil {
			return err
		}
		pending, pendingPhase = nil, ""
		if journal.Len() > MaxBytes {
			return errors.New("OMP journal exceeds its byte budget")
		}
		return nil
	}
	appendAssistant := func(at time.Time, block any, phase string) {
		if pending == nil {
			pending, pendingAt = assistant(block, "stop"), at
		} else {
			pending["content"] = append(pending["content"].([]any), block)
		}
		pendingPhase = phase
		if phase == "commentary" {
			pending["stopReason"] = "toolUse"
		}
	}
	calls := map[string]string{}
	seenCalls := map[string]bool{}
	for _, item := range document.Items {
		if err := ctx.Err(); err != nil {
			return Encoded{}, err
		}
		if item.Kind != "assistant" && item.Kind != "call" {
			if err := flushAssistant(); err != nil {
				return Encoded{}, err
			}
		}
		var message map[string]any
		switch item.Kind {
		case "user":
			blocks, err := content(item.Content)
			if err != nil {
				return Encoded{}, err
			}
			message = map[string]any{"role": "user", "content": blocks}
		case "assistant":
			if pending != nil && (item.assistantPhase == "" || pendingPhase != item.assistantPhase) {
				if err := flushAssistant(); err != nil {
					return Encoded{}, err
				}
			}
			appendAssistant(item.Time, map[string]any{"type": "text", "text": item.Text}, item.assistantPhase)
			if item.assistantStop != "" {
				pending["stopReason"] = item.assistantStop
				if err := flushAssistant(); err != nil {
					return Encoded{}, err
				}
			}
			continue
		case "call":
			if item.ID == "" || seenCalls[item.ID] || item.Name == "" {
				return Encoded{}, errors.New("tool calls require unique IDs and names")
			}
			var args any
			name := item.Name
			if item.Shell != "" {
				name = "bash"
				args = map[string]string{"command": item.Shell}
				if item.Workdir != "" {
					args.(map[string]string)["cwd"] = item.Workdir
				}
			} else {
				var object map[string]json.RawMessage
				if json.Unmarshal([]byte(item.Input), &object) != nil || object == nil {
					return Encoded{}, errors.New("tool arguments must be a JSON object")
				}
				args = object
			}
			calls[item.ID], seenCalls[item.ID] = name, true
			if pendingPhase == "final_answer" {
				if err := flushAssistant(); err != nil {
					return Encoded{}, err
				}
			}
			appendAssistant(item.Time, map[string]any{"type": "toolCall", "id": item.ID, "name": name, "arguments": args}, "commentary")
			continue
		case "result":
			name, found := calls[item.ID]
			if !found {
				return Encoded{}, fmt.Errorf("unmatched or repeated tool result %q", item.ID)
			}
			delete(calls, item.ID)
			blocks := item.Content
			if len(blocks) == 0 {
				blocks = []Content{{Type: "text", Text: item.Text}}
			}
			body, err := content(blocks)
			if err != nil {
				return Encoded{}, err
			}
			message = map[string]any{"role": "toolResult", "toolCallId": item.ID, "toolName": name, "content": body, "isError": item.IsError}
		default:
			return Encoded{}, fmt.Errorf("unsupported conversation item kind %q", item.Kind)
		}
		if err := add(item.Time, message); err != nil {
			return Encoded{}, err
		}
		if journal.Len() > MaxBytes {
			return Encoded{}, errors.New("OMP journal exceeds its byte budget")
		}
	}
	if err := flushAssistant(); err != nil {
		return Encoded{}, err
	}
	if len(calls) != 0 {
		return Encoded{}, errors.New("incomplete tool history: every call requires a later result")
	}
	result := Encoded{ID: document.ID, Filename: journalName(document.Timestamp, document.ID), Journal: journal.Bytes()}
	var hashes []string
	for hash := range assets {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	total := len(result.Journal)
	for _, hash := range hashes {
		total += len(assets[hash])
		if total > MaxBytes || len(result.Assets)+1 >= MaxFiles {
			return Encoded{}, errors.New("OMP journal and assets exceed their input byte or file budget")
		}
		result.Assets = append(result.Assets, Asset{SHA256: hash, Body: assets[hash]})
	}
	if total > MaxBytes {
		return Encoded{}, errors.New("OMP journal exceeds its input byte budget")
	}
	return result, nil
}

func encodeContent(blocks []Content, assets map[string][]byte) ([]any, error) {
	return encodeContentWithImageMode(blocks, assets, true)
}

func encodeContentWithImageMode(blocks []Content, assets map[string][]byte, externalize bool) ([]any, error) {
	result := make([]any, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			result = append(result, map[string]any{"type": "text", "text": block.Text})
		case "image":
			raw, err := decodeImage(block.Data)
			if err != nil || block.MimeType == "" {
				return nil, errors.New("image requires a media type and valid base64 bytes")
			}
			data := block.Data
			if externalize && len(data) >= 1024 {
				hash := hashBytes(raw)
				assets[hash] = raw
				data = "blob:sha256:" + hash
			}
			result = append(result, map[string]any{"type": "image", "data": data, "mimeType": block.MimeType})
		default:
			return nil, fmt.Errorf("unsupported content type %q", block.Type)
		}
	}
	return result, nil
}

func carryStamp(at, fallback time.Time) string {
	if at.IsZero() {
		at = fallback
	}
	return at.UTC().Format("2006-01-02T15:04:05.000Z")
}

func journalName(at time.Time, id string) string {
	return strings.NewReplacer(":", "-", ".", "-").Replace(carryStamp(at, at)) + "_" + id + ".jsonl"
}

func cloneAssets(assets []Asset) []Asset {
	result := make([]Asset, len(assets))
	for i, asset := range assets {
		result[i] = Asset{SHA256: asset.SHA256, Body: bytes.Clone(asset.Body)}
	}
	return result
}
