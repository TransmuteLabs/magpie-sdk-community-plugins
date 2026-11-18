package ompsessions

// Active-branch replay extracted from the archived carry_omp_read.go.
// Filesystem discovery is replaced by explicitly supplied, verified assets.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// An entry is retained even when its payload is not used: metadata entries
// can be parents, and off-branch entries can record the active leaf.
type ompCarryEntry struct {
	Type                         string                     `json:"type"`
	ID                           string                     `json:"id"`
	ParentID                     *string                    `json:"parentId"`
	ActiveLeafID                 json.RawMessage            `json:"activeLeafId"`
	Timestamp                    string                     `json:"timestamp"`
	Message                      json.RawMessage            `json:"message"`
	Summary                      string                     `json:"summary"`
	FirstKeptEntryID             string                     `json:"firstKeptEntryId"`
	ProviderReplayThroughEntryID string                     `json:"providerReplayThroughEntryId"`
	Method                       string                     `json:"method"`
	PreserveData                 map[string]json.RawMessage `json:"preserveData"`
	Details                      json.RawMessage            `json:"details"`
	Unreadable                   bool                       `json:"-"`
}

type ompCarryMessage struct {
	Role               string          `json:"role"`
	Content            json.RawMessage `json:"content"`
	Timestamp          *int64          `json:"timestamp"`
	Attribution        string          `json:"attribution"`
	Synthetic          bool            `json:"synthetic"`
	ToolCallID         string          `json:"toolCallId"`
	IsError            bool            `json:"isError"`
	Command            string          `json:"command"`
	Code               string          `json:"code"`
	Output             string          `json:"output"`
	ExitCode           *int            `json:"exitCode"`
	Cancelled          bool            `json:"cancelled"`
	ExcludeFromContext bool            `json:"excludeFromContext"`
	Files              []struct {
		Image *ompCarryBlock `json:"image"`
	} `json:"files"`
}

type ompCarryBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Narration bool            `json:"narration"`
	Data      string          `json:"data"`
	MimeType  string          `json:"mimeType"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Input     json.RawMessage `json:"input"`
}

// readItems follows replay's active parent chain, then rebuilds its
// portable context. Images are not opened until after both selections.
func readItems(ctx context.Context, body []byte, assets map[string][]byte) ([]Item, error) {
	return readItemsWithResultMedia(ctx, body, assets, false)
}

func readItemsWithResultMedia(ctx context.Context, body []byte, assets map[string][]byte, resultMedia bool) ([]Item, error) {
	return readSelectedItems(ctx, body, assets, resultMedia, nil)
}

func readSelectedItems(ctx context.Context, body []byte, assets map[string][]byte, resultMedia bool, replay *ompNativeReplay) ([]Item, error) {
	var entries []ompCarryEntry
	byID := make(map[string]int)
	if err := eachLine(ctx, body, func(line []byte) {
		var envelope struct {
			Type         string          `json:"type"`
			ID           string          `json:"id"`
			ParentID     *string         `json:"parentId"`
			ActiveLeafID json.RawMessage `json:"activeLeafId"`
			Timestamp    string          `json:"timestamp"`
		}
		if json.Unmarshal(line, &envelope) != nil || envelope.ID == "" || envelope.Type == "session" || envelope.Type == "title" {
			return
		}
		entry := ompCarryEntry{Type: envelope.Type, ID: envelope.ID, ParentID: envelope.ParentID,
			ActiveLeafID: envelope.ActiveLeafID, Timestamp: envelope.Timestamp}
		if entry.Type == "message" || entry.Type == "compaction" || entry.Type == "branch_summary" {
			var payload ompCarryEntry
			if json.Unmarshal(line, &payload) == nil {
				entry = payload
			} else if entry.Type == "compaction" {
				// An unreadable boundary cannot safely stand in for history.
				entry.Unreadable = true
			}
		}
		byID[entry.ID] = len(entries)
		entries = append(entries, entry)
	}); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	branch, err := ompActiveBranch(entries, byID)
	if err != nil {
		return nil, err
	}

	root, compact := 0, -1
	for i := len(branch) - 1; i >= 0; i-- {
		if branch[i].Type == "reset_boundary" {
			root = i + 1
			break
		}
	}
	for i := len(branch) - 1; i >= root; i-- {
		if branch[i].Type == "compaction" && ompCarryPortable(branch[i]) {
			compact = i
			break
		}
	}
	retained := root
	if compact >= 0 {
		c := branch[compact]
		retained = compact
		for i := 0; i < compact; i++ {
			if c.FirstKeptEntryID != "" && branch[i].ID == c.FirstKeptEntryID {
				retained = i
				break
			}
		}
		if retained == compact && c.FirstKeptEntryID == "" && ompCarryPresent(c.PreserveData["anthropicCompaction"]) && c.ProviderReplayThroughEntryID != "" {
			for i := 0; i < compact; i++ {
				if branch[i].ID == c.ProviderReplayThroughEntryID {
					retained = i + 1
					break
				}
			}
		}
		if retained < root {
			retained = root
		}
	}

	resolver := ompCarryBlobs{assets: assets, memo: make(map[string]string)}
	var items []Item
	// Emit the selected summary before the retained tail without copying
	// either region of the branch. Empty summaries still enforce the cutoff.
	if compact >= 0 && branch[compact].Summary != "" {
		at, _ := time.Parse(time.RFC3339Nano, branch[compact].Timestamp)
		items = append(items, ompCarryUserText(branch[compact].Summary, at))
	}
	var executions []int
	for _, entry := range branch[retained:] {
		at, stampErr := time.Parse(time.RFC3339Nano, entry.Timestamp)
		switch entry.Type {
		case "branch_summary":
			if entry.Summary != "" && !strings.Contains(entry.Summary, "¶think:") {
				items = append(items, ompCarryUserText(entry.Summary, at))
			}
		case "message":
			var m ompCarryMessage
			if replay != nil {
				var native ompReplayMessage
				if json.Unmarshal(entry.Message, &native) != nil || !replay.keep(&native) {
					continue
				}
				m = native.ompCarryMessage
			} else if json.Unmarshal(entry.Message, &m) != nil {
				continue
			}
			if stampErr != nil && m.Timestamp != nil {
				at = time.UnixMilli(*m.Timestamp)
			}
			added, err := ompCarryMessageItemsWithResultMedia(entry.ID, m, at, &resolver, resultMedia)
			if err != nil {
				return nil, fmt.Errorf("omp entry %q: %w", entry.ID, err)
			}
			if len(added) == 2 && (m.Role == "bashExecution" || m.Role == "pythonExecution") {
				executions = append(executions, len(items))
			}
			items = append(items, added...)
		}
	}

	// Native execution messages lack tool IDs. Their local pair IDs must not
	// collide with any tool ID recorded by the provider.
	if len(executions) != 0 {
		generated := make(map[int]bool, len(executions)*2)
		for _, i := range executions {
			generated[i], generated[i+1] = true, true
		}
		used := make(map[string]bool)
		for i, item := range items {
			if !generated[i] && item.ID != "" {
				used[item.ID] = true
			}
		}
		for _, i := range executions {
			id := items[i].ID
			for used[id] {
				id += ":"
			}
			items[i].ID, items[i+1].ID = id, id
			used[id] = true
		}
	}

	// pairCarry intentionally preserves missing-output behavior for the other
	// readers. For omp, only calls with a later selected result may cross.
	results := make(map[string]bool)
	keep := make([]bool, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		switch it.Kind {
		case "result":
			if it.ID != "" {
				results[it.ID] = true
			}
			keep[i] = true
		case "call":
			keep[i] = it.ID != "" && results[it.ID]
			if keep[i] {
				delete(results, it.ID)
			}
		default:
			keep[i] = true
		}
	}
	out := items[:0]
	for i, it := range items {
		if keep[i] {
			out = append(out, it)
		}
	}
	return out, nil
}

func ompCarryPresent(raw json.RawMessage) bool {
	return len(raw) != 0 && string(raw) != "null"
}

func ompCarryPortable(entry ompCarryEntry) bool {
	var details struct {
		Kind string `json:"kind"`
	}
	json.Unmarshal(entry.Details, &details)
	return !entry.Unreadable && entry.Method != "snapcompact" && details.Kind != "experimental-context-rollover" &&
		!strings.Contains(entry.Summary, "¶think:") &&
		!ompCarryPresent(entry.PreserveData["openaiRemoteCompaction"]) &&
		!ompCarryPresent(entry.PreserveData["snapcompact"])
}

func ompCarryUserText(text string, at time.Time) Item {
	return Item{Kind: "user", Content: []Content{{Type: "text", Text: text}}, Time: at}
}

func ompCarryMessageItems(id string, m ompCarryMessage, at time.Time, blobs *ompCarryBlobs) ([]Item, error) {
	return ompCarryMessageItemsWithResultMedia(id, m, at, blobs, false)
}

func ompCarryMessageItemsWithResultMedia(id string, m ompCarryMessage, at time.Time, blobs *ompCarryBlobs, resultMedia bool) ([]Item, error) {
	switch m.Role {
	case "user":
		if m.Synthetic || m.Attribution == "agent" {
			return nil, nil
		}
		var text string
		if json.Unmarshal(m.Content, &text) == nil {
			if text == "" {
				return nil, nil
			}
			return []Item{ompCarryUserText(text, at)}, nil
		}
		var blocks []ompCarryBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			return nil, nil
		}
		user := Item{Kind: "user", Time: at}
		for _, block := range blocks {
			switch block.Type {
			case "text":
				user.Content = append(user.Content, Content{Type: "text", Text: block.Text})
			case "image":
				image, err := blobs.image(block)
				if err != nil {
					return nil, err
				}
				user.Content = append(user.Content, image)
			}
		}
		if len(user.Content) != 0 {
			return []Item{user}, nil
		}
	case "fileMention":
		user := Item{Kind: "user", Time: at}
		for _, file := range m.Files {
			if file.Image != nil {
				image, err := blobs.image(*file.Image)
				if err != nil {
					return nil, err
				}
				user.Content = append(user.Content, image)
			}
		}
		if len(user.Content) != 0 {
			return []Item{user}, nil
		}
	case "assistant":
		var blocks []ompCarryBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			return nil, nil
		}
		var out []Item
		for _, block := range blocks {
			switch block.Type {
			case "image":
				if resultMedia {
					return nil, fmt.Errorf("assistant image history cannot be represented in this native target")
				}
			case "text":
				if block.Text != "" {
					out = append(out, Item{Kind: "assistant", Text: block.Text, Time: at})
				}
			case "thinking":
				if block.Narration && block.Thinking != "" {
					out = append(out, Item{Kind: "assistant", Text: block.Thinking, Time: at})
				}
			case "toolCall":
				args := block.Arguments
				if !ompCarryPresent(args) {
					args = block.Input
				}
				call := Item{Kind: "call", ID: block.ID, Name: block.Name, Input: pretty(args), Time: at}
				if block.Name == "bash" {
					var input struct {
						Command string `json:"command"`
						Cwd     string `json:"cwd"`
					}
					if json.Unmarshal(args, &input) == nil {
						call.Shell, call.Workdir = input.Command, input.Cwd
					}
				}
				out = append(out, call)
			}
		}
		return out, nil
	case "toolResult":
		result := Item{Kind: "result", ID: m.ToolCallID, Text: resultText(m.Content), IsError: m.IsError, Time: at}
		if resultMedia {
			var blocks []ompCarryBlock
			if json.Unmarshal(m.Content, &blocks) == nil {
				for _, block := range blocks {
					switch block.Type {
					case "text":
						result.Content = append(result.Content, Content{Type: "text", Text: block.Text})
					case "image":
						image, err := blobs.image(block)
						if err != nil {
							return nil, err
						}
						result.Content = append(result.Content, image)
					}
				}
			}
		}
		return []Item{result}, nil
	case "bashExecution", "pythonExecution":
		if m.ExcludeFromContext {
			return nil, nil
		}
		call := Item{Kind: "call", ID: "\x00omp-execution:" + id, Time: at}
		if m.Role == "bashExecution" {
			call.Name, call.Shell, call.Input = "bash", m.Command, m.Command
		} else {
			call.Name, call.Input = "python", m.Code
		}
		result := Item{Kind: "result", ID: call.ID, Text: m.Output, IsError: m.Cancelled || (m.ExitCode != nil && *m.ExitCode != 0), Time: at}
		return []Item{call, result}, nil
	}
	return nil, nil
}

// ompCarryBlobs resolves only the bytes the caller explicitly supplied.
type ompCarryBlobs struct {
	assets map[string][]byte
	memo   map[string]string
}

func (b *ompCarryBlobs) image(block ompCarryBlock) (Content, error) {
	if block.MimeType == "" || block.Data == "" {
		return Content{}, fmt.Errorf("omp image is missing data or media type")
	}
	data := block.Data
	if len(data) >= len("blob:") && strings.EqualFold(data[:len("blob:")], "blob:") {
		hash, ok := strings.CutPrefix(data, "blob:sha256:")
		if !ok || !validHash(hash) {
			return Content{}, fmt.Errorf("invalid omp image blob reference %q", data)
		}
		if cached, ok := b.memo[hash]; ok {
			data = cached
		} else {
			raw, ok := b.assets[hash]
			if !ok {
				return Content{}, fmt.Errorf("unresolved omp image blob %q: supply blobs/%s explicitly", hash, hash)
			}
			if hashBytes(raw) != hash {
				return Content{}, fmt.Errorf("omp image blob digest mismatch %q", hash)
			}
			data = base64.StdEncoding.EncodeToString(raw)
			b.memo[hash] = data
		}
	} else if _, err := decodeImage(data); err != nil {
		return Content{}, err
	}
	return Content{Type: "image", Data: data, MimeType: block.MimeType}, nil
}
