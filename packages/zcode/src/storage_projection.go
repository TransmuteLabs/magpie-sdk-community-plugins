package zcode

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func historyTime(milliseconds int64) string {
	if milliseconds < 0 || milliseconds > 253402300799999 {
		return ""
	}
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339Nano)
}

func historyString(value any) string {
	text, _ := value.(string)
	return text
}

func historyCount(value any) (uint64, bool) {
	number, valid := value.(json.Number)
	count, err := number.Int64()
	return uint64(count), valid && err == nil && count >= 0
}

func historyRealUser(message map[string]any) bool {
	semantics := mapObject(message["semantics"])
	origin := historyString(semantics["origin"])
	kind := historyString(semantics["kind"])
	return message["role"] == "user" && message["synthetic"] != true && historyString(message["source"]) == "" &&
		(origin == "" || origin == "real_user") && (kind == "" || kind == "user_prompt")
}

func (reader *historySQL) summary(ctx context.Context, session historySession, messages []historyMessage) (sdk.SessionSourceSummary, error) {
	summary := sdk.SessionSourceSummary{Title: session.title, Cwd: session.cwd, ParentID: session.parent,
		Start: historyTime(session.created), Last: historyTime(session.updated), Usage: []sdk.SessionUsagePoint{}}
	if session.titleSource == "default" {
		summary.Title = ""
	}
	known := true
	for _, message := range messages {
		if summary.Title == "" && historyRealUser(message.data) {
			for _, part := range message.parts {
				if part["type"] == "text" && strings.TrimSpace(historyString(part["text"])) != "" {
					title := strings.Join(strings.Fields(historyString(part["text"])), " ")
					runes := []rune(title)
					summary.Title = string(runes[:min(len(runes), 180)])
					break
				}
			}
		}
		if message.data["role"] != "assistant" {
			continue
		}
		origin := mapObject(mapObject(message.data["metadata"])["forkOrigin"])
		sourceID, sourceSession := historyString(origin["messageId"]), historyString(origin["sessionId"])
		if sourceID != "" && sourceID != message.id && sourceSession != "" {
			var copied bool
			if err := reader.tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM message WHERE id=? AND session_id=?)",
				sourceID, sourceSession).Scan(&copied); err != nil {
				return sdk.SessionSourceSummary{}, err
			}
			if copied {
				continue
			}
		}
		model := historyString(message.data["modelId"])
		if model == "" {
			model = historyString(message.data["modelID"])
		}
		tokens := mapObject(message.data["tokens"])
		if tokens == nil && model == "" {
			continue
		}
		cache := mapObject(tokens["cache"])
		input, inputOK := historyCount(tokens["input"])
		output, outputOK := historyCount(tokens["output"])
		read, readOK := historyCount(cache["read"])
		write, writeOK := historyCount(cache["write"])
		if !inputOK || !outputOK || !readOK || !writeOK || model == "" || len(model) > 300 {
			known = false
			continue
		}
		summary.Usage = append(summary.Usage, sdk.SessionUsagePoint{
			At: historyTime(message.created), Model: model, Input: input, Output: output,
			CacheRead: read, CacheWrite: write,
		})
	}
	if !known {
		summary.Usage = nil
	}
	return summary, nil
}

func historyBlock(part map[string]any) bool {
	return (part["type"] == "text" || part["type"] == "reasoning" || part["type"] == "tool") &&
		(part["type"] != "tool" || mapObject(mapObject(part["state"])["input"])["source"] != "browser_turn_end")
}

func historyOrderedParts(message historyMessage) []map[string]any {
	parts := append([]map[string]any(nil), message.parts...)
	if message.data["role"] != "assistant" {
		return parts
	}
	blocks := []map[string]any{}
	for _, part := range parts {
		if historyBlock(part) {
			if _, valid := historyCount(part["blockIndex"]); !valid {
				return parts
			}
			blocks = append(blocks, part)
		}
	}
	sort.SliceStable(blocks, func(left, right int) bool {
		before, _ := historyCount(blocks[left]["blockIndex"])
		after, _ := historyCount(blocks[right]["blockIndex"])
		return before < after
	})
	next := 0
	for index, part := range parts {
		if historyBlock(part) {
			parts[index] = blocks[next]
			next++
		}
	}
	return parts
}

func historyTranscript(request sdk.StorageRequest, messages []historyMessage) (sdk.StorageResult, error) {
	result := sdk.StorageResult{SnapshotRef: request.Lease.SnapshotRef, SessionID: request.Locator.SessionID,
		Mode: "transcript", Parts: []sdk.SessionSourcePart{}}
	total := 0
	appendPart := func(part sdk.SessionSourcePart) error {
		total += len(part.Text)
		if len(result.Parts) >= request.Lease.Limits.MaxParts || total > request.Lease.Limits.MaxTextBytes ||
			len(part.Name) > 1024 || len(part.Model) > 300 || !utf8.ValidString(part.Text) {
			return errors.New("ZCode history transcript exceeds its projection budget")
		}
		result.Parts = append(result.Parts, part)
		return nil
	}
	for _, message := range messages {
		role := historyString(message.data["role"])
		if role != "user" && role != "assistant" {
			return sdk.StorageResult{}, errors.New("unsupported ZCode history message role")
		}
		model := historyString(message.data["modelId"])
		if model == "" {
			model = historyString(message.data["modelID"])
		}
		for _, part := range historyOrderedParts(message) {
			projected, incomplete, err := historyProjectPart(role, model, part)
			if err != nil {
				return sdk.StorageResult{}, err
			}
			result.Cut = result.Cut || incomplete
			for _, value := range projected {
				if err := appendPart(value); err != nil {
					return sdk.StorageResult{}, err
				}
			}
		}
		if message.data["bodyRefs"] != nil || message.data["contentRefs"] != nil {
			result.Cut = true
			if err := appendPart(sdk.SessionSourcePart{Role: role, Kind: "context", Text: "[body unavailable]"}); err != nil {
				return sdk.StorageResult{}, err
			}
		}
	}
	return result, nil
}

func historyProjectPart(role, model string, part map[string]any) ([]sdk.SessionSourcePart, bool, error) {
	kind := historyString(part["type"])
	value := sdk.SessionSourcePart{Role: role, Kind: "context", Model: model}
	incomplete := false
	switch kind {
	case "text", "reasoning":
		value.Kind = "text"
		if kind == "reasoning" {
			value.Kind = "thinking"
		}
		text, valid := part["text"].(string)
		if !valid && part["bodyRef"] == nil {
			return nil, false, errors.New("ZCode history text is missing its body")
		}
		value.Text = text
		if part["bodyRef"] != nil || part["bodyRefs"] != nil {
			value.Text += "\n[body unavailable]"
			value.Cut, incomplete = 1, true
		}
	case "tool":
		name := historyString(part["tool"])
		state := mapObject(part["state"])
		input, err := json.Marshal(state["input"])
		if err != nil || name == "" || state == nil {
			return nil, false, errors.New("invalid ZCode history tool state")
		}
		call := sdk.SessionSourcePart{Role: role, Kind: "tool_use", Name: name, Model: model, Text: string(input)}
		parts := []sdk.SessionSourcePart{call}
		switch state["status"] {
		case "pending", "running":
		case "completed", "error":
			text, valid := state["output"].(string)
			if state["status"] == "error" {
				text, valid = state["error"].(string)
			}
			if !valid {
				return nil, false, errors.New("invalid ZCode history tool output")
			}
			parts = append(parts, sdk.SessionSourcePart{Role: "tool", Kind: "tool_result", Name: name, Text: text})
		default:
			return nil, false, errors.New("unknown ZCode history tool status")
		}
		if attachments, ok := state["attachments"].([]any); ok && len(attachments) > 0 {
			parts = append(parts, sdk.SessionSourcePart{Role: "tool", Kind: "context", Text: "[tool attachments unavailable]", Cut: 1})
			incomplete = true
		}
		return parts, incomplete, nil
	case "file":
		value.Name = historyString(part["filename"])
		value.Text = "[media unavailable: " + value.Name + " (" + historyString(part["mime"]) + ")]"
		value.Cut, incomplete = 1, true
	case "timeline", "compaction":
		value.Text = historyString(part["text"])
		if value.Text == "" {
			value.Text = historyString(part["timelineText"])
		}
		if value.Text == "" {
			value.Text = "[ZCode " + kind + "]"
		}
	case "step-start", "step-finish":
		return nil, false, nil
	default:
		value.Text = "[ZCode part unavailable: " + kind + "]"
		value.Cut, incomplete = 1, true
	}
	return []sdk.SessionSourcePart{value}, incomplete, nil
}
