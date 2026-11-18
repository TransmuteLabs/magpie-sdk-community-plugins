package ompsessions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type claudeSourceLine struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	Parent    string `json:"parentUuid"`
	Sidechain bool   `json:"isSidechain"`
	Meta      bool   `json:"isMeta"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type claudeSourceBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	CallID  string          `json:"tool_use_id"`
	IsError bool            `json:"is_error"`
	Content json.RawMessage `json:"content"`
	Source  struct {
		Type string `json:"type"`
		Mime string `json:"media_type"`
		Data string `json:"data"`
	} `json:"source"`
}

func readClaudeNative(ctx context.Context, body []byte) ([]Item, error) {
	nodes := map[string]claudeSourceLine{}
	leaf := ""
	var parseErr error
	if err := eachLine(ctx, body, func(line []byte) {
		if parseErr != nil {
			return
		}
		var entry claudeSourceLine
		if err := json.Unmarshal(line, &entry); err != nil {
			parseErr = errors.New("invalid Claude JSONL")
			return
		}
		if entry.UUID == "" {
			return
		}
		nodes[entry.UUID] = entry
		if (entry.Type == "user" || entry.Type == "assistant") && !entry.Sidechain {
			leaf = entry.UUID
		}
	}); err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	var chain []claudeSourceLine
	seen := map[string]bool{}
	for id := leaf; id != ""; {
		if seen[id] {
			return nil, errors.New("cyclic Claude parent chain")
		}
		seen[id] = true
		entry, exists := nodes[id]
		if !exists {
			break
		}
		chain = append(chain, entry)
		id = entry.Parent
	}
	var result []Item
	for index := len(chain) - 1; index >= 0; index-- {
		entry := chain[index]
		if entry.Meta || entry.Sidechain || entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		var text string
		if json.Unmarshal(entry.Message.Content, &text) == nil {
			if entry.Type == "user" {
				if !claudeInjected(text) {
					result = append(result, Item{Kind: "user", Content: []Content{{Type: "text", Text: text}}, Time: at})
				}
			} else if text != "" {
				result = append(result, Item{Kind: "assistant", Text: text, Time: at})
			}
			continue
		}
		var blocks []claudeSourceBlock
		if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
			return nil, errors.New("invalid Claude message content")
		}
		var user []Content
		flushUser := func() {
			if len(user) != 0 {
				result = append(result, Item{Kind: "user", Content: user, Time: at})
				user = nil
			}
		}
		for _, block := range blocks {
			switch block.Type {
			case "text":
				if entry.Type == "user" {
					if !claudeInjected(block.Text) {
						user = append(user, Content{Type: "text", Text: block.Text})
					}
				} else if strings.TrimSpace(block.Text) != "" {
					result = append(result, Item{Kind: "assistant", Text: block.Text, Time: at})
				}
			case "image":
				if entry.Type != "user" {
					return nil, errors.New("Claude assistant image is not portable")
				}
				image, err := claudeSourceImage(block)
				if err != nil {
					return nil, err
				}
				user = append(user, image)
			case "tool_use":
				flushUser()
				item := Item{Kind: "call", ID: block.ID, Name: block.Name, Input: pretty(block.Input), Time: at}
				if block.Name == "Bash" {
					var input struct {
						Command string `json:"command"`
					}
					if json.Unmarshal(block.Input, &input) == nil {
						item.Shell = input.Command
					}
				}
				result = append(result, item)
			case "tool_result":
				flushUser()
				content, err := claudeResultContent(block.Content)
				if err != nil {
					return nil, err
				}
				result = append(result, Item{Kind: "result", ID: block.CallID, Text: resultText(block.Content), Content: content, IsError: block.IsError, Time: at})
			case "thinking", "redacted_thinking":
			default:
				return nil, fmt.Errorf("unsupported Claude content block %q", block.Type)
			}
		}
		flushUser()
	}
	return result, nil
}

func claudeInjected(text string) bool {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"<command-name>", "<command-message>", "<command-args>", "<local-command-", "<system-reminder>", "Caveat: The messages below were generated"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return text == ""
}

func claudeSourceImage(block claudeSourceBlock) (Content, error) {
	if block.Source.Type != "base64" || block.Source.Mime == "" {
		return Content{}, errors.New("Claude images require explicit base64 bytes")
	}
	if _, err := decodeImage(block.Source.Data); err != nil {
		return Content{}, err
	}
	return Content{Type: "image", MimeType: block.Source.Mime, Data: block.Source.Data}, nil
}

func claudeResultContent(raw json.RawMessage) ([]Content, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []Content{{Type: "text", Text: text}}, nil
	}
	var blocks []claudeSourceBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil, errors.New("invalid Claude tool result")
	}
	var result []Content
	for _, block := range blocks {
		switch block.Type {
		case "text":
			result = append(result, Content{Type: "text", Text: block.Text})
		case "image":
			image, err := claudeSourceImage(block)
			if err != nil {
				return nil, err
			}
			result = append(result, image)
		default:
			return nil, errors.New("unsupported Claude tool result content")
		}
	}
	return result, nil
}

type codexSourceItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Phase     string          `json:"phase"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Content   []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL string `json:"image_url"`
	} `json:"content"`
	Action struct {
		Command []string `json:"command"`
		Workdir string   `json:"working_directory"`
	} `json:"action"`
	Message             string            `json:"message"`
	Replacement         []json.RawMessage `json:"replacement_history"`
	ReplacementMetadata []json.RawMessage `json:"replacement_history_metadata"`
	Encrypted           string            `json:"encrypted_content"`
}

const codexSummaryPrefix = "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:"

func readCodexNative(ctx context.Context, body []byte) ([]Item, error) {
	var result []Item
	failures := map[string]bool{}
	var parseErr error
	if err := eachLine(ctx, body, func(line []byte) {
		if parseErr != nil {
			return
		}
		var envelope struct {
			Type      string          `json:"type"`
			Timestamp string          `json:"timestamp"`
			Payload   json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &envelope) != nil || envelope.Type == "" {
			parseErr = errors.New("invalid Codex JSONL")
			return
		}
		at, _ := time.Parse(time.RFC3339Nano, envelope.Timestamp)
		if envelope.Type == "session_meta" {
			var metadata struct {
				HistoryBase json.RawMessage `json:"history_base"`
			}
			if json.Unmarshal(envelope.Payload, &metadata) != nil {
				parseErr = errors.New("invalid Codex session metadata")
				return
			}
			if ompCarryPresent(metadata.HistoryBase) {
				parseErr = errors.New("Codex sparse rollout requires captured ancestor history from the host")
			}
			return
		}
		if envelope.Type == "event_msg" {
			var event struct {
				Type     string          `json:"type"`
				NumTurns json.RawMessage `json:"num_turns"`
				Item     struct {
					Type   string `json:"type"`
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"item"`
			}
			if json.Unmarshal(envelope.Payload, &event) != nil {
				parseErr = errors.New("invalid Codex event payload")
				return
			}
			if (event.Type == "task_complete" || event.Type == "turn_aborted") &&
				len(result) != 0 && result[len(result)-1].Kind == "assistant" {
				result[len(result)-1].assistantStop = "stop"
				if event.Type == "turn_aborted" {
					result[len(result)-1].assistantStop = "aborted"
				}
			}
			if event.Type == "thread_rolled_back" {
				var count *uint32
				if json.Unmarshal(event.NumTurns, &count) != nil || count == nil {
					parseErr = errors.New("Codex rollback requires a recorded uint32 turn count")
					return
				}
				result = codexRollback(result, *count)
				retained := map[string]bool{}
				for _, item := range result {
					if item.Kind == "call" {
						retained[item.ID] = true
					}
				}
				for id := range failures {
					if !retained[id] {
						delete(failures, id)
					}
				}
			}
			if event.Type == "item_completed" &&
				(event.Item.Type == "CommandExecution" || event.Item.Type == "DynamicToolCall") &&
				event.Item.ID != "" && (event.Item.Status == "completed" || event.Item.Status == "failed") {
				failures[event.Item.ID] = event.Item.Status == "failed"
			}
			return
		}
		if envelope.Type != "response_item" && envelope.Type != "compacted" {
			return
		}
		var payload codexSourceItem
		if json.Unmarshal(envelope.Payload, &payload) != nil {
			parseErr = errors.New("invalid Codex payload")
			return
		}
		appendItem := func(item codexSourceItem) {
			items, err := codexNativeItems(item, at)
			if err != nil {
				parseErr = err
				return
			}
			result = append(result, items...)
		}
		if envelope.Type == "response_item" {
			appendItem(payload)
			return
		}
		if payload.ReplacementMetadata != nil {
			if payload.Replacement == nil || len(payload.ReplacementMetadata) != len(payload.Replacement) {
				parseErr = errors.New("Codex replacement history metadata does not match its history")
				return
			}
			for _, metadata := range payload.ReplacementMetadata {
				var fields map[string]json.RawMessage
				if json.Unmarshal(metadata, &fields) != nil || fields == nil {
					parseErr = errors.New("invalid Codex replacement history metadata")
					return
				}
			}
		}
		if payload.Replacement != nil {
			result = nil
			for _, raw := range payload.Replacement {
				var item codexSourceItem
				if json.Unmarshal(raw, &item) != nil {
					parseErr = errors.New("invalid Codex replacement history")
					return
				}
				appendItem(item)
			}
		} else if payload.Message != "" {
			var retained []Item
			for _, item := range result {
				if item.Kind == "user" {
					retained = append(retained, item)
				}
			}
			result = append(retained, Item{Kind: "user", Content: []Content{{Type: "text", Text: codexSummaryPrefix + "\n" + payload.Message}}, Time: at})
		}
	}); err != nil {
		return nil, err
	}
	for index := range result {
		if result[index].Kind == "result" {
			result[index].IsError = failures[result[index].ID]
		}
	}
	return result, parseErr
}

func codexRollback(items []Item, count uint32) []Item {
	if count == 0 {
		return items
	}
	boundary := len(items)
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].Kind != "user" {
			continue
		}
		summary := false
		for _, content := range items[index].Content {
			if content.Type == "text" && strings.HasPrefix(strings.TrimSpace(content.Text), codexSummaryPrefix) {
				summary = true
				break
			}
		}
		if summary {
			continue
		}
		boundary = index
		count--
		if count == 0 {
			break
		}
	}
	return items[:boundary]
}

func codexNativeItems(payload codexSourceItem, at time.Time) ([]Item, error) {
	switch payload.Type {
	case "message":
		if payload.Role != "user" && payload.Role != "assistant" {
			return nil, nil
		}
		var content []Content
		var words []string
		for _, block := range payload.Content {
			switch block.Type {
			case "input_text", "output_text", "text":
				if payload.Role == "user" && codexInjected(block.Text) {
					continue
				}
				content = append(content, Content{Type: "text", Text: block.Text})
				words = append(words, block.Text)
			case "input_image":
				if payload.Role != "user" {
					return nil, errors.New("Codex assistant image is not portable")
				}
				image, err := nativeDataImage(block.ImageURL)
				if err != nil {
					return nil, err
				}
				content = append(content, image)
			default:
				return nil, fmt.Errorf("unsupported Codex content %q", block.Type)
			}
		}
		if len(content) == 0 {
			return nil, nil
		}
		if payload.Role == "user" {
			return []Item{{Kind: "user", Content: content, Time: at}}, nil
		}
		return []Item{{Kind: "assistant", Text: strings.Join(words, "\n"), Time: at, assistantPhase: payload.Phase}}, nil
	case "function_call", "custom_tool_call", "local_shell_call":
		item := Item{Kind: "call", ID: payload.CallID, Name: payload.Name, Input: payload.Input, Time: at}
		if payload.Type == "local_shell_call" {
			item.Name, item.Shell, item.Workdir = "shell", nativeShellLine(payload.Action.Command), payload.Action.Workdir
			item.Input = item.Shell
		} else if payload.Type == "function_call" {
			item.Input = pretty(json.RawMessage(payload.Arguments))
			var args struct {
				Cmd     string          `json:"cmd"`
				Command json.RawMessage `json:"command"`
				Workdir string          `json:"workdir"`
			}
			if json.Unmarshal([]byte(payload.Arguments), &args) == nil {
				switch payload.Name {
				case "exec_command":
					item.Shell = args.Cmd
				case "shell", "container.exec":
					var argv []string
					if json.Unmarshal(args.Command, &argv) == nil {
						item.Shell = nativeShellLine(argv)
					}
				case "shell_command":
					json.Unmarshal(args.Command, &item.Shell)
				}
				item.Workdir = args.Workdir
			}
		}
		return []Item{item}, nil
	case "function_call_output", "custom_tool_call_output":
		var text string
		if json.Unmarshal(payload.Output, &text) == nil {
			return []Item{{Kind: "result", ID: payload.CallID, Text: text, Content: []Content{{Type: "text", Text: text}}, Time: at}}, nil
		}
		var object struct {
			Content string `json:"content"`
			Output  string `json:"output"`
		}
		if json.Unmarshal(payload.Output, &object) == nil && (object.Content != "" || object.Output != "") {
			text = object.Content
			if text == "" {
				text = object.Output
			}
			return []Item{{Kind: "result", ID: payload.CallID, Text: text, Content: []Content{{Type: "text", Text: text}}, Time: at}}, nil
		}
		var blocks []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL string `json:"image_url"`
		}
		if json.Unmarshal(payload.Output, &blocks) != nil {
			if !json.Valid(payload.Output) {
				return nil, errors.New("invalid structured Codex tool output")
			}
			text := string(payload.Output)
			return []Item{{Kind: "result", ID: payload.CallID, Text: text, Content: []Content{{Type: "text", Text: text}}, Time: at}}, nil
		}
		item := Item{Kind: "result", ID: payload.CallID, Time: at}
		for _, block := range blocks {
			switch block.Type {
			case "text", "input_text", "output_text":
				item.Content = append(item.Content, Content{Type: "text", Text: block.Text})
				item.Text += block.Text
			case "input_image":
				image, err := nativeDataImage(block.ImageURL)
				if err != nil {
					return nil, err
				}
				item.Content = append(item.Content, image)
			default:
				return nil, errors.New("unsupported Codex tool output content")
			}
		}
		return []Item{item}, nil
	case "compaction", "compaction_summary":
		if strings.HasPrefix(payload.Encrypted, "magpie1:") {
			raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(payload.Encrypted, "magpie1:"))
			if err != nil {
				return nil, errors.New("invalid local Codex summary")
			}
			return []Item{{Kind: "user", Content: []Content{{Type: "text", Text: codexSummaryPrefix + "\n" + string(raw)}}, Time: at}}, nil
		}
	}
	return nil, nil
}

func codexInjected(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	for _, prefix := range []string{"# AGENTS.md", "<environment_context>", "<permissions instructions>", "<collaboration_mode>", "<turn_aborted>", "<skills_instructions>", "<user_instructions>"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func nativeDataImage(value string) (Content, error) {
	head, data, ok := strings.Cut(value, ",")
	mime, base64Type := strings.CutSuffix(strings.TrimPrefix(head, "data:"), ";base64")
	if !ok || !strings.HasPrefix(head, "data:") || !base64Type || mime == "" {
		return Content{}, errors.New("native images require explicitly supplied data URLs")
	}
	if _, err := decodeImage(data); err != nil {
		return Content{}, err
	}
	return Content{Type: "image", MimeType: mime, Data: data}, nil
}

func nativeShellLine(argv []string) string {
	if len(argv) == 3 && (argv[1] == "-lc" || argv[1] == "-c") {
		return argv[2]
	}
	quoted := make([]string, len(argv))
	for index, word := range argv {
		quoted[index] = word
		if word == "" || strings.ContainsAny(word, " \t\n'\"\\$`*?[]{}()<>|&;#~!") {
			quoted[index] = "'" + strings.ReplaceAll(word, "'", "'\\''") + "'"
		}
	}
	return strings.Join(quoted, " ")
}
