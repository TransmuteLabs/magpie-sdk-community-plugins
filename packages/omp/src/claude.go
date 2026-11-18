package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type claudeHistoryLine struct {
	ParentUUID *string        `json:"parentUuid"`
	Sidechain  bool           `json:"isSidechain"`
	Type       string         `json:"type"`
	Message    map[string]any `json:"message"`
	UUID       string         `json:"uuid"`
	Timestamp  string         `json:"timestamp"`
	UserType   string         `json:"userType"`
	Entrypoint string         `json:"entrypoint"`
	Cwd        string         `json:"cwd"`
	SessionID  string         `json:"sessionId"`
	Version    string         `json:"version"`
}

func convertToClaude(ctx context.Context, request sdk.SessionRequest) (sdk.SessionResult, error) {
	var options struct {
		Cwd string `json:"cwd"`
	}
	if strictJSON(request.Options, &options) != nil || !filepath.IsAbs(options.Cwd) ||
		strings.ContainsAny(options.Cwd, "\x00\r\n") {
		return sdk.SessionResult{}, errors.New("native Claude conversion requires the selected absolute cwd")
	}
	var project strings.Builder
	for _, unit := range utf16.Encode([]rune(options.Cwd)) {
		if unit >= 'a' && unit <= 'z' || unit >= 'A' && unit <= 'Z' || unit >= '0' && unit <= '9' {
			project.WriteByte(byte(unit))
		} else {
			project.WriteByte('-')
		}
	}
	if project.Len() == 0 || project.Len() > 200 {
		return sdk.SessionResult{}, errors.New("long hashed Claude project directories are unsupported")
	}
	source, assets, err := sessionInputDocuments(ctx, request, true)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	store, err := nativeOMPInputs(ctx, source, assets)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	items, err := readSelectedItems(ctx, source, store, true, &ompNativeReplay{})
	if err != nil {
		return sdk.SessionResult{}, err
	}
	now := time.Now()
	id, err := newUUID(now, 4)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	var journal bytes.Buffer
	var parent *string
	messageID := ""
	messageHasTools := false
	lastRole := ""
	line := func(kind string, content []any, at time.Time, stop string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		uuid, err := newUUID(now, 4)
		if err != nil {
			return err
		}
		message := map[string]any{"role": kind, "content": content}
		if kind == "assistant" {
			if messageID == "" {
				messageID = "msg_" + strings.ReplaceAll(uuid, "-", "")
			}
			messageHasTools = messageHasTools || stop == "tool_use"
			if messageHasTools {
				stop = "tool_use"
			}
			message["id"], message["type"], message["model"] = messageID, "message", "<synthetic>"
			message["stop_reason"], message["stop_sequence"] = stop, nil
			message["usage"] = map[string]int{"input_tokens": 0, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0, "output_tokens": 0}
		} else {
			messageID, messageHasTools = "", false
		}
		value := claudeHistoryLine{ParentUUID: parent, Type: kind, Message: message, UUID: uuid,
			Timestamp: carryStamp(at, now), UserType: "external", Entrypoint: "cli",
			Cwd: options.Cwd, SessionID: id, Version: "2.0.0"}
		body, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if journal.Len()+len(body)+1 > MaxDocumentBytes {
			return errors.New("native Claude history exceeds the checked-plan budget")
		}
		journal.Write(body)
		journal.WriteByte('\n')
		parent = &uuid
		lastRole = kind
		return nil
	}
	type nativeCall struct {
		id   string
		name string
	}
	calls := map[string]nativeCall{}
	pending := map[string]bool{}
	for _, item := range items {
		if item.Kind != "result" && lastRole == "user" && len(pending) != 0 {
			return sdk.SessionResult{}, errors.New("interleaved incomplete tool groups cannot be carried into Claude Code")
		}
		var content []any
		kind, stop := "assistant", "end_turn"
		switch item.Kind {
		case "user", "steering":
			if item.Kind == "steering" && len(pending) != 0 {
				return sdk.SessionResult{}, errors.New("steering before pending tool results cannot be carried into Claude Code")
			}
			kind = "user"
			content, err = claudeContent(item.Content)
		case "assistant":
			content = []any{map[string]any{"type": "text", "text": item.Text}}
		case "call":
			if _, exists := calls[item.ID]; exists || item.ID == "" {
				return sdk.SessionResult{}, errors.New("native migration requires unambiguous tool call IDs")
			}
			call := nativeCall{name: item.Name}
			if item.Shell == "" {
				content = []any{map[string]any{"type": "text", "text": fmt.Sprintf("Tool %s was called with:\n%s", item.Name, item.Input)}}
			} else {
				call.id, err = newUUID(now, 4)
				pending[call.id] = true
				command := item.Shell
				if item.Workdir != "" && item.Workdir != options.Cwd {
					command = "cd '" + strings.ReplaceAll(item.Workdir, "'", "'\\''") + "' && " + command
				}
				content = []any{map[string]any{"type": "tool_use", "id": "toolu_" + call.id,
					"name": "Bash", "input": map[string]any{"command": command}}}
				stop = "tool_use"
			}
			calls[item.ID] = call
		case "result":
			call := calls[item.ID]
			resultContent := item.Content
			if len(resultContent) == 0 {
				resultContent = []Content{{Type: "text", Text: item.Text}}
			}
			blocks, contentErr := claudeContent(resultContent)
			if contentErr != nil {
				return sdk.SessionResult{}, contentErr
			}
			if call.id != "" {
				if !pending[call.id] {
					return sdk.SessionResult{}, errors.New("duplicate native tool result")
				}
				delete(pending, call.id)
				kind = "user"
				content = []any{map[string]any{"type": "tool_result", "tool_use_id": "toolu_" + call.id,
					"content": blocks, "is_error": item.IsError}}
			} else {
				kind = "user"
				content = append([]any{map[string]any{"type": "text",
					"text": fmt.Sprintf("Result of previous tool %s (error=%t):", call.name, item.IsError)}}, blocks...)
			}
		default:
			return sdk.SessionResult{}, errors.New("unsupported native migration item")
		}
		if err != nil {
			return sdk.SessionResult{}, err
		}
		if err := line(kind, content, item.Time, stop); err != nil {
			return sdk.SessionResult{}, err
		}
	}
	if len(pending) != 0 {
		return sdk.SessionResult{}, errors.New("native tool history has unresolved results")
	}
	if journal.Len() == 0 {
		return sdk.SessionResult{}, errors.New("selected native context has no portable messages")
	}
	result := sdk.SessionResult{ID: id, Files: []sdk.SessionDocument{{Path: project.String() + "/" + id + ".jsonl",
		Format: "claude", Body: journal.Bytes()}}}
	wire, err := json.Marshal(result)
	if err != nil || len(wire) > maxSessionWire {
		return sdk.SessionResult{}, errors.New("native Claude history exceeds the SDK frame budget")
	}
	return result, nil
}

func claudeContent(items []Content) ([]any, error) {
	var content []any
	for _, item := range items {
		switch item.Type {
		case "text":
			content = append(content, map[string]any{"type": "text", "text": item.Text})
		case "image":
			if _, err := decodeImage(item.Data); err != nil || item.MimeType == "" {
				return nil, errors.New("native image requires MIME type and base64 data")
			}
			content = append(content, map[string]any{"type": "image",
				"source": map[string]any{"type": "base64", "media_type": item.MimeType, "data": item.Data}})
		default:
			return nil, errors.New("unsupported native content block")
		}
	}
	return content, nil
}
