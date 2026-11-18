package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func convertToCodex(ctx context.Context, request sdk.SessionRequest) (sdk.SessionResult, error) {
	var options struct {
		Cwd      string `json:"cwd"`
		Provider string `json:"provider"`
		Model    string `json:"model,omitempty"`
	}
	if strictJSON(request.Options, &options) != nil || !filepath.IsAbs(options.Cwd) || options.Provider == "" || len(options.Provider) > 256 ||
		len(options.Model) > 256 || strings.ContainsAny(options.Cwd+options.Provider+options.Model, "\x00\r\n") {
		return sdk.SessionResult{}, errors.New("native Codex migration requires selected cwd and target provider")
	}
	body, assets, err := sessionInputDocuments(ctx, request, true)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	store, err := nativeOMPInputs(ctx, body, assets)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	items, err := codexHistory(ctx, body, store)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	now := time.Now()
	id, err := newUUID(now, 7)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	var journal bytes.Buffer
	ordinal := 0
	add := func(at time.Time, kind string, payload any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := json.Marshal(struct {
			Timestamp string `json:"timestamp"`
			Ordinal   int    `json:"ordinal"`
			Type      string `json:"type"`
			Payload   any    `json:"payload"`
		}{carryStamp(at, now), ordinal, kind, payload})
		if err != nil {
			return err
		}
		if journal.Len()+len(raw)+1 > MaxBytes {
			return errors.New("native Codex rollout exceeds its byte budget")
		}
		journal.Write(raw)
		journal.WriteByte('\n')
		ordinal++
		return nil
	}
	if err := add(now, "session_meta", map[string]any{"id": id, "timestamp": carryStamp(now, now), "cwd": options.Cwd,
		"originator": "codex_cli_rs", "cli_version": "0.0.0", "source": "cli", "model_provider": options.Provider,
		"history_mode": "paginated"}); err != nil {
		return sdk.SessionResult{}, err
	}
	turn := ""
	hasUser := false
	complete := func(at time.Time, item map[string]any) error {
		if _, exists := item["id"]; !exists {
			itemID, err := newUUID(now, 4)
			if err != nil {
				return err
			}
			item["id"] = itemID
		}
		return add(at, "event_msg", map[string]any{"type": "item_completed", "thread_id": id, "turn_id": turn, "item": item})
	}
	start := func(at time.Time) error {
		if turn != "" {
			return nil
		}
		turn, err = newUUID(now, 4)
		hasUser = false
		if err != nil {
			return err
		}
		if err := add(at, "event_msg", map[string]any{"type": "task_started", "turn_id": turn, "model_context_window": nil}); err != nil {
			return err
		}
		if options.Model != "" {
			return add(at, "turn_context", map[string]any{"turn_id": turn, "cwd": options.Cwd, "approval_policy": "never",
				"sandbox_policy": map[string]any{"type": "read-only"}, "model": options.Model, "summary": "auto"})
		}
		return nil
	}
	pending := map[string]codexExportCall{}
	var pendingOrder []string
	seen := map[string]bool{}
	finish := func(at time.Time, completed bool) error {
		if turn == "" {
			return nil
		}
		payload := map[string]any{"type": "task_complete", "turn_id": turn, "last_agent_message": nil}
		if !completed || len(pending) != 0 {
			payload = map[string]any{"type": "turn_aborted", "turn_id": turn, "reason": "interrupted"}
		}
		for _, key := range pendingOrder {
			call, exists := pending[key]
			if !exists {
				continue
			}
			if err := complete(at, call.materialized(nil)); err != nil {
				return err
			}
		}
		clear(pending)
		pendingOrder = nil
		if err := add(at, "event_msg", payload); err != nil {
			return err
		}
		turn = ""
		return nil
	}
	for _, item := range items {
		if item.Kind == "context" {
			if err := start(item.Time); err != nil {
				return sdk.SessionResult{}, err
			}
			if err := add(item.Time, "compacted", map[string]any{"message": item.Text, "replacement_history": []any{
				map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": codexSummaryPrefix + "\n" + item.Text}}},
			}}); err != nil {
				return sdk.SessionResult{}, err
			}
			if err := complete(item.Time, map[string]any{"type": "ContextCompaction"}); err != nil {
				return sdk.SessionResult{}, err
			}
			continue
		}
		if item.Kind == "branch_context" {
			if err := add(item.Time, "response_item", map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": codexSummaryPrefix + "\n" + item.Text}}}); err != nil {
				return sdk.SessionResult{}, err
			}
			continue
		}
		if item.Kind == "user" && hasUser {
			if err := finish(item.Time, false); err != nil {
				return sdk.SessionResult{}, err
			}
		}
		if err := start(item.Time); err != nil {
			return sdk.SessionResult{}, err
		}
		switch item.Kind {
		case "user", "steering", "assistant":
			role := item.Kind
			if role == "steering" {
				role = "user"
			}
			content, err := codexContent(item.Content)
			if err != nil {
				return sdk.SessionResult{}, err
			}
			var text []string
			for _, block := range content {
				fields := block.(map[string]any)
				if value, ok := fields["text"].(string); ok {
					text = append(text, value)
				}
				if item.Kind == "assistant" {
					fields["type"] = "output_text"
				}
			}
			payload := map[string]any{"type": "message", "role": role, "content": content}
			var nativeContent []any
			for _, block := range content {
				fields := block.(map[string]any)
				if value, ok := fields["text"].(string); ok {
					nativeContent = append(nativeContent, map[string]any{"type": "text", "text": value, "text_elements": []any{}})
				} else {
					nativeContent = append(nativeContent, map[string]any{"type": "image", "image_url": fields["image_url"]})
				}
			}
			event := map[string]any{"type": "UserMessage", "content": nativeContent}
			hasUser = hasUser || role == "user"
			if item.Kind == "assistant" {
				event = map[string]any{"type": "AgentMessage", "content": []any{map[string]any{"type": "Text", "text": strings.Join(text, "\n")}}}
				if item.phase != "" {
					payload["phase"], event["phase"] = item.phase, item.phase
				}
			}
			if err := add(item.Time, "response_item", payload); err != nil {
				return sdk.SessionResult{}, err
			}
			if err := complete(item.Time, event); err != nil {
				return sdk.SessionResult{}, err
			}
		case "reasoning":
			if err := add(item.Time, "response_item", map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": item.Text}}, "encrypted_content": nil}); err != nil {
				return sdk.SessionResult{}, err
			}
			if err := complete(item.Time, map[string]any{"type": "Reasoning", "summary_text": []string{item.Text}, "raw_content": []any{}}); err != nil {
				return sdk.SessionResult{}, err
			}
		case "call":
			if seen[item.ID] || item.ID == "" || item.Name == "" {
				return sdk.SessionResult{}, errors.New("ambiguous native Codex tool call")
			}
			seen[item.ID] = true
			callID, err := newUUID(now, 4)
			if err != nil {
				return sdk.SessionResult{}, err
			}
			call := codexExportCall{id: "call_" + callID, name: item.Name, cwd: options.Cwd, shell: item.Shell}
			arguments := json.RawMessage(item.Input)
			if item.Shell != "" {
				call.name = "exec_command"
				if item.Workdir != "" {
					call.cwd = item.Workdir
					if !filepath.IsAbs(call.cwd) {
						call.cwd = filepath.Join(options.Cwd, call.cwd)
					}
				}
				arguments, _ = json.Marshal(map[string]string{"cmd": item.Shell, "workdir": call.cwd})
			}
			if !json.Valid(arguments) {
				return sdk.SessionResult{}, errors.New("native Codex tool arguments must be valid JSON")
			}
			call.arguments = arguments
			pending[item.ID] = call
			pendingOrder = append(pendingOrder, item.ID)
			if err := add(item.Time, "response_item", map[string]any{"type": "function_call", "call_id": call.id, "name": call.name, "arguments": string(arguments)}); err != nil {
				return sdk.SessionResult{}, err
			}
		case "result":
			call, exists := pending[item.ID]
			if !exists {
				return sdk.SessionResult{}, errors.New("native Codex tool result has no unique preceding call")
			}
			delete(pending, item.ID)
			content := item.Content
			if len(content) == 0 {
				content = []Content{{Type: "text", Text: item.Text}}
			}
			output, err := codexContent(content)
			if err != nil {
				return sdk.SessionResult{}, err
			}
			if err := add(item.Time, "response_item", map[string]any{"type": "function_call_output", "call_id": call.id, "output": output}); err != nil {
				return sdk.SessionResult{}, err
			}
			if err := complete(item.Time, call.materialized(&codexExportOutput{item: item, content: output})); err != nil {
				return sdk.SessionResult{}, err
			}
		case "turn_end":
			if err := finish(item.Time, true); err != nil {
				return sdk.SessionResult{}, err
			}
		default:
			return sdk.SessionResult{}, errors.New("unsupported native Codex history item")
		}
	}
	if err := finish(now, false); err != nil {
		return sdk.SessionResult{}, err
	}
	local := now.Local()
	return boundedNativeResult(sdk.SessionResult{ID: id, Files: []sdk.SessionDocument{{Path: local.Format("2006/01/02") + "/rollout-" + local.Format("2006-01-02T15-04-05") + "-" + id + ".jsonl", Format: "codex", Body: journal.Bytes()}}})
}

type codexExportCall struct {
	id, name, cwd, shell string
	arguments            json.RawMessage
}

type codexExportOutput struct {
	item    codexHistoryItem
	content []any
}

func (call codexExportCall) materialized(output *codexExportOutput) map[string]any {
	status := "in_progress"
	if output != nil {
		status = "completed"
		if output.item.IsError {
			status = "failed"
		}
	}
	if call.shell == "" {
		item := map[string]any{"type": "DynamicToolCall", "id": call.id, "tool": call.name, "arguments": call.arguments, "status": status}
		if output == nil {
			return item
		}
		content := make([]any, 0, len(output.content))
		for _, block := range output.content {
			fields := block.(map[string]any)
			if fields["type"] == "input_image" {
				content = append(content, map[string]any{"type": "inputImage", "imageUrl": fields["image_url"]})
			} else {
				content = append(content, map[string]any{"type": "inputText", "text": fields["text"]})
			}
		}
		item["content_items"], item["success"] = content, !output.item.IsError
		return item
	}
	item := map[string]any{"type": "CommandExecution", "id": call.id, "command": []string{"/bin/sh", "-lc", call.shell},
		"cwd": codexPathURI(call.cwd), "parsed_cmd": []any{}, "source": "agent", "status": status}
	if output != nil {
		item["aggregated_output"] = output.item.Text
		item["exit_code"] = output.item.exitCode
	}
	return item
}

func codexPathURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func codexContent(items []Content) ([]any, error) {
	var result []any
	for _, item := range items {
		switch item.Type {
		case "text":
			result = append(result, map[string]any{"type": "input_text", "text": item.Text})
		case "image":
			if _, err := decodeImage(item.Data); err != nil || item.MimeType == "" {
				return nil, errors.New("native Codex image requires base64 and media type")
			}
			result = append(result, map[string]any{"type": "input_image", "image_url": "data:" + item.MimeType + ";base64," + item.Data})
		default:
			return nil, errors.New("unsupported native Codex content")
		}
	}
	return result, nil
}

func boundedNativeResult(result sdk.SessionResult) (sdk.SessionResult, error) {
	body, err := json.Marshal(result)
	if err != nil || len(body) > maxSessionWire {
		return sdk.SessionResult{}, errors.New("native conversion exceeds its SDK frame budget")
	}
	return result, nil
}
