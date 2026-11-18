package ompsessions

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type codexHistoryItem struct {
	Item
	phase    string
	exitCode *int
}

func codexHistory(ctx context.Context, body []byte, assets map[string][]byte) ([]codexHistoryItem, error) {
	var entries []ompCarryEntry
	byID := map[string]int{}
	var parseErr error
	if err := eachLine(ctx, body, func(line []byte) {
		var entry ompCarryEntry
		if json.Unmarshal(line, &entry) != nil {
			parseErr = errors.New("invalid OMP native history entry")
			return
		}
		if entry.Type != "session" && entry.Type != "title" && entry.ID != "" {
			byID[entry.ID] = len(entries)
			entries = append(entries, entry)
		}
	}); err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	branch, err := ompActiveBranch(entries, byID)
	if err != nil {
		return nil, err
	}
	start, compact := 0, -1
	for index := len(branch) - 1; index >= 0; index-- {
		if branch[index].Type == "reset_boundary" {
			start = index + 1
			break
		}
	}
	for index := len(branch) - 1; index >= start; index-- {
		if branch[index].Type == "compaction" && ompCarryPortable(branch[index]) {
			compact = index
			break
		}
	}
	retained := start
	var result []codexHistoryItem
	if compact >= 0 {
		entry := branch[compact]
		retained = compact
		for index := start; index < compact; index++ {
			if entry.FirstKeptEntryID != "" && branch[index].ID == entry.FirstKeptEntryID {
				retained = index
				break
			}
			if entry.FirstKeptEntryID == "" && ompCarryPresent(entry.PreserveData["anthropicCompaction"]) &&
				entry.ProviderReplayThroughEntryID == branch[index].ID {
				retained = index + 1
				break
			}
		}
		at, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		result = append(result, codexHistoryItem{Item: Item{Kind: "context", Text: entry.Summary, Time: at}})
	}
	blobs := &ompCarryBlobs{assets: assets, memo: map[string]string{}}
	var replay ompNativeReplay
	for _, entry := range branch[retained:] {
		at, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if entry.Type == "branch_summary" {
			if ompCarryPortable(entry) && entry.Summary != "" {
				result = append(result, codexHistoryItem{Item: Item{Kind: "branch_context", Text: entry.Summary, Time: at}})
			}
			continue
		}
		if entry.Type != "message" {
			continue
		}
		var native ompReplayMessage
		if json.Unmarshal(entry.Message, &native) != nil {
			return nil, errors.New("invalid selected OMP native message")
		}
		if !replay.keep(&native) {
			continue
		}
		message := native.ompCarryMessage
		if at.IsZero() && message.Timestamp != nil {
			at = time.UnixMilli(*message.Timestamp)
		}
		if message.Role != "assistant" {
			switch message.Role {
			case "system", "developer":
				continue
			case "user", "toolResult", "fileMention", "bashExecution", "pythonExecution":
			default:
				return nil, errors.New("unsupported OMP message role in Codex history")
			}
			items, err := ompCarryMessageItemsWithResultMedia(entry.ID, message, at, blobs, true)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if message.Role == "bashExecution" || message.Role == "pythonExecution" {
					item.ID = "execution:" + entry.ID
				}
				if message.Role == "pythonExecution" && item.Kind == "call" {
					arguments, err := json.Marshal(map[string]string{"code": item.Input})
					if err != nil {
						return nil, err
					}
					item.Input = string(arguments)
				}
				result = append(result, codexHistoryItem{Item: item, exitCode: message.ExitCode})
			}
			continue
		}
		var stopReason string
		if len(native.StopReason) != 0 && json.Unmarshal(native.StopReason, &stopReason) != nil {
			return nil, errors.New("invalid OMP assistant metadata")
		}
		var blocks []ompCarryBlock
		if json.Unmarshal(message.Content, &blocks) != nil {
			return nil, errors.New("unsupported OMP assistant content")
		}
		phase := ""
		if stopReason == "toolUse" {
			phase = "commentary"
		} else if stopReason == "stop" {
			phase = "final_answer"
		}
		var text []Content
		flush := func() {
			if len(text) != 0 {
				result = append(result, codexHistoryItem{Item: Item{Kind: "assistant", Content: text, Time: at}, phase: phase})
				text = nil
			}
		}
		for _, block := range blocks {
			if block.Type == "redactedThinking" {
				continue
			}
			if block.Type == "text" {
				text = append(text, Content{Type: "text", Text: block.Text})
				continue
			}
			flush()
			if block.Type == "thinking" {
				if block.Narration && block.Thinking != "" {
					result = append(result, codexHistoryItem{Item: Item{Kind: "reasoning", Text: block.Thinking, Time: at}})
				}
				continue
			}
			if block.Type != "toolCall" {
				return nil, errors.New("unsupported OMP assistant block in Codex history")
			}
			encoded, _ := json.Marshal([]ompCarryBlock{block})
			selected := message
			selected.Content = encoded
			items, err := ompCarryMessageItemsWithResultMedia(entry.ID, selected, at, blobs, true)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				result = append(result, codexHistoryItem{Item: item, phase: phase})
			}
		}
		flush()
		if stopReason == "stop" {
			result = append(result, codexHistoryItem{Item: Item{Kind: "turn_end", Time: at}})
		}
	}
	return result, nil
}
