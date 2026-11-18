package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func historyUsage(message historyMessage) (sdk.SessionUsagePoint, bool, error) {
	var point sdk.SessionUsagePoint
	if message.data["tokens"] == nil {
		return point, false, nil
	}
	var tokens struct {
		Input  uint64 `json:"input"`
		Output uint64 `json:"output"`
		Cache  struct {
			Read  uint64 `json:"read"`
			Write uint64 `json:"write"`
		} `json:"cache"`
	}
	if json.Unmarshal(message.data["tokens"], &tokens) != nil {
		return point, false, errors.New("unsupported fractional or overflowing OpenCode token counters")
	}
	model := historyObject(message.data, "model")
	name := historyString(model, "id")
	if name == "" {
		name = historyString(message.data, "modelID")
	}
	if name == "" {
		if tokens.Input|tokens.Output|tokens.Cache.Read|tokens.Cache.Write == 0 {
			return point, false, nil
		}
		return point, false, errors.New("OpenCode token usage has no producing model")
	}
	point = sdk.SessionUsagePoint{At: historyTime(message.created), Model: name,
		Input: tokens.Input, Output: tokens.Output, CacheRead: tokens.Cache.Read, CacheWrite: tokens.Cache.Write}
	return point, true, nil
}

func historyNative(message historyMessage) bool {
	if message.kind != "compaction" || historyString(message.data, "status") != "completed" {
		return false
	}
	native := historyObject(message.data, "native")
	return native["messages"] != nil && string(native["messages"]) != "null" ||
		message.data["providerContext"] != nil && string(message.data["providerContext"]) != "null"
}

func (reader *historySQL) localHistory(ctx context.Context, session string, messages []historyMessage, limits sdk.StorageLimits) ([]historyMessage, error) {
	boundary := 0
	for index, message := range messages {
		if message.kind == "compaction" && historyString(message.data, "status") == "completed" && !historyNative(message) {
			boundary = index
		}
	}
	if boundary > 0 {
		compaction := &messages[boundary]
		metadata := historyObject(compaction.data, "metadata")
		keepID := historyString(metadata, "keepID")
		if keepID != "" {
			start := -1
			for index := 0; index < boundary; index++ {
				if messages[index].id == keepID {
					start = index
					break
				}
			}
			if start >= 0 {
				recent := []string{}
				for _, message := range messages[start:boundary] {
					text, err := historyRecentText(message)
					if err != nil {
						return nil, err
					}
					if text != "" {
						recent = append(recent, text)
					}
				}
				if metadata == nil {
					metadata = map[string]json.RawMessage{}
				}
				metadata["recent"] = historyJSON(strings.Join(recent, "\n\n"))
				compaction.data["metadata"] = historyJSON(metadata)
			}
		}
	}
	messages = messages[boundary:]
	epoch := int64(math.MaxInt64)
	if reader.tables["instruction_state"] {
		err := reader.tx.QueryRowContext(ctx, "SELECT epoch_start FROM instruction_state WHERE session_id=?", session).Scan(&epoch)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	nativeSeq := int64(-1)
	for _, message := range messages {
		if historyNative(message) {
			nativeSeq = message.seq
		}
	}
	out := make([]historyMessage, 0, len(messages))
	for _, message := range messages {
		metadata := historyObject(message.data, "metadata")
		if historyNative(message) || message.kind == "system" && (message.seq < nativeSeq ||
			historyString(metadata, "notice") == "instructions" && message.seq < epoch) {
			continue
		}
		out = append(out, message)
	}
	return out, nil
}

func historyJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

type historyProjection struct {
	request sdk.StorageRequest
	result  sdk.StorageResult
	text    int
	media   int
}

func (projection *historyProjection) add(part sdk.SessionSourcePart) bool {
	if part.Text == "" && part.Kind != "tool_use" && part.Kind != "image" {
		return true
	}
	limits := projection.request.Lease.Limits
	if len(projection.result.Parts) >= limits.MaxParts {
		projection.result.Cut = true
		return false
	}
	remaining := limits.MaxTextBytes - projection.text
	if len(part.Text) > remaining {
		end := max(remaining, 0)
		for end > 0 && !utf8.ValidString(part.Text[:end]) {
			end--
		}
		part.Cut, part.Text = len(part.Text)-end, part.Text[:end]
		projection.result.Cut = true
	}
	projection.text += len(part.Text)
	projection.result.Parts = append(projection.result.Parts, part)
	return !projection.result.Cut
}

func (reader *historySQL) transcript(ctx context.Context, request sdk.StorageRequest, result sdk.StorageResult, messages []historyMessage) (sdk.StorageResult, error) {
	projection := &historyProjection{request: request, result: result}
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return sdk.StorageResult{}, err
		}
		if err := projection.message(ctx, reader, message); err != nil {
			return sdk.StorageResult{}, err
		}
		if projection.result.Cut {
			break
		}
	}
	return projection.result, nil
}

func (projection *historyProjection) message(ctx context.Context, reader *historySQL, message historyMessage) error {
	switch message.kind {
	case "user":
		if !projection.add(sdk.SessionSourcePart{Role: "user", Kind: "text", Text: historyString(message.data, "text")}) {
			return nil
		}
		var files []map[string]json.RawMessage
		if json.Unmarshal(message.data["files"], &files) != nil && message.data["files"] != nil {
			return errors.New("malformed OpenCode user attachments")
		}
		for _, file := range files {
			if err := projection.attachment(ctx, reader, message, "user", file); err != nil {
				return err
			}
		}
		fallthrough
	case "assistant":
		model := historyString(historyObject(message.data, "model"), "id")
		if model == "" {
			model = historyString(message.data, "modelID")
		}
		for _, content := range message.content {
			kind := historyString(content, "type")
			if kind == "text" || kind == "reasoning" {
				var ignored, synthetic bool
				_ = json.Unmarshal(content["ignored"], &ignored)
				_ = json.Unmarshal(content["synthetic"], &synthetic)
				if ignored || message.kind == "user" && synthetic {
					continue
				}
				outputKind := "text"
				if kind == "reasoning" {
					outputKind = "thinking"
				}
				if !projection.add(sdk.SessionSourcePart{Role: message.kind, Kind: outputKind,
					Text: historyString(content, "text"), Model: model}) {
					return nil
				}
			} else if kind == "tool" {
				name := historyString(content, "name")
				if name == "" {
					name = historyString(content, "tool")
				}
				state := historyObject(content, "state")
				input := state["input"]
				if input == nil {
					input = json.RawMessage(`{}`)
				}
				if !projection.add(sdk.SessionSourcePart{Role: "assistant", Kind: "tool_use", Name: name,
					Text: string(input), Model: model}) {
					return nil
				}
				status := historyString(state, "status")
				if status != "completed" && status != "error" {
					continue
				}
				text := historyString(state, "output")
				if status == "error" {
					text = historyString(historyObject(state, "error"), "message")
					if text == "" {
						text = historyString(state, "error")
					}
				}
				if text != "" && !projection.add(sdk.SessionSourcePart{Role: "tool", Kind: "tool_result", Name: name, Text: text, Model: model}) {
					return nil
				}
				var items []map[string]json.RawMessage
				if json.Unmarshal(state["content"], &items) != nil && state["content"] != nil {
					return errors.New("malformed OpenCode tool content")
				}
				for _, item := range items {
					if historyString(item, "type") == "text" {
						if !projection.add(sdk.SessionSourcePart{Role: "tool", Kind: "tool_result", Name: name, Text: historyString(item, "text"), Model: model}) {
							return nil
						}
					} else if historyString(item, "type") == "file" {
						if err := projection.attachment(ctx, reader, message, "tool", item); err != nil {
							return err
						}
					} else {
						return errors.New("unsupported OpenCode tool content kind")
					}
				}
			} else if kind == "file" {
				if err := projection.attachment(ctx, reader, message, message.kind, content); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("unsupported OpenCode history part %q", kind)
			}
		}
	case "system", "synthetic", "skill":
		projection.add(sdk.SessionSourcePart{Role: "assistant", Kind: "context", Name: historyString(message.data, "name"),
			Text: historyString(message.data, "text")})
	case "location-switched":
		projection.add(sdk.SessionSourcePart{Role: "user", Kind: "context",
			Text: "The working directory has been changed to " + historyString(historyObject(message.data, "location"), "directory") + "."})
	case "shell":
		text := historyString(message.data, "command")
		output := historyObject(message.data, "output")
		if text != "" {
			text += "\n" + historyString(output, "output")
		}
		projection.add(sdk.SessionSourcePart{Role: "assistant", Kind: "tool_result", Name: "shell", Text: text})
	case "compaction":
		if historyString(message.data, "status") == "completed" && !historyNative(message) {
			text := historyString(message.data, "summary")
			if recent := historyString(historyObject(message.data, "metadata"), "recent"); recent != "" {
				text += "\n\n" + recent
			}
			projection.add(sdk.SessionSourcePart{Role: "assistant", Kind: "context", Name: "compaction", Text: text})
		}
	case "agent-switched", "model-switched", "idle":
	default:
		return fmt.Errorf("unsupported OpenCode history message %q", message.kind)
	}
	return nil
}

func historyRecentText(message historyMessage) (string, error) {
	switch message.kind {
	case "compaction", "system", "idle", "model-switched", "agent-switched":
		return "", nil
	case "user":
		lines := []string{"[User]: " + historyString(message.data, "text")}
		var files []map[string]json.RawMessage
		_ = json.Unmarshal(message.data["files"], &files)
		for _, file := range files {
			name := historyString(file, "name")
			if name == "" {
				name = "inline attachment"
			}
			lines = append(lines, fmt.Sprintf("[Attached %s: %s]", historyString(file, "mime"), name))
		}
		return strings.Join(lines, "\n"), nil
	case "assistant":
		lines := []string{}
		for _, content := range message.content {
			switch historyString(content, "type") {
			case "text":
				lines = append(lines, "[Assistant]: "+historyString(content, "text"))
			case "reasoning":
			case "tool":
				state := historyObject(content, "state")
				input := string(state["input"])
				var literal string
				if json.Unmarshal(state["input"], &literal) == nil {
					input = literal
				}
				lines = append(lines, fmt.Sprintf("[Assistant tool call]: %s(%s)", historyString(content, "name"), input))
				var items []map[string]json.RawMessage
				_ = json.Unmarshal(state["content"], &items)
				output := []string{}
				for _, item := range items {
					if historyString(item, "type") == "text" {
						output = append(output, historyString(item, "text"))
					} else {
						label := "[Attached " + historyString(item, "mime")
						if name := historyString(item, "name"); name != "" {
							label += ": " + name
						}
						output = append(output, label+"]")
					}
				}
				if historyString(state, "status") == "completed" {
					lines = append(lines, "[Tool result]: "+historyTruncate(strings.Join(output, "\n")))
				} else if historyString(state, "status") == "error" {
					lines = append(lines, "[Tool error]: "+historyString(historyObject(state, "error"), "message"))
				}
			default:
				return "", errors.New("unsupported OpenCode recent part")
			}
		}
		return strings.Join(lines, "\n"), nil
	case "synthetic":
		return "[Synthetic context]: " + historyString(message.data, "text"), nil
	case "skill":
		return fmt.Sprintf("[Skill activated: %s]\n%s", historyString(message.data, "name"), historyString(message.data, "text")), nil
	case "location-switched":
		return "[User]: The working directory has been changed to " + historyString(historyObject(message.data, "location"), "directory") + ".", nil
	case "shell":
		var background bool
		_ = json.Unmarshal(historyObject(message.data, "metadata")["background"], &background)
		if background {
			return "", nil
		}
		return "[Shell]: " + historyString(message.data, "command") + "\n" +
			historyTruncate(historyString(historyObject(message.data, "output"), "output")), nil
	default:
		return "", fmt.Errorf("unsupported OpenCode recent message %q", message.kind)
	}
}

func historyTruncate(text string) string {
	if utf8.RuneCountInString(text) <= 1250 {
		return text
	}
	return string([]rune(text)[:1250]) + "\n[truncated]"
}
