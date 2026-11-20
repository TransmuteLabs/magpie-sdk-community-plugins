package devin

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

type NativeSignature struct {
	Format string
	Model  string
}

type SignatureResolver func(string) (NativeSignature, error)

type messagesBlock struct {
	Type      string          `json:"type"`
	Text      *string         `json:"text,omitempty"`
	Thinking  *string         `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   *bool           `json:"is_error,omitempty"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source,omitempty"`
	Cache json.RawMessage `json:"cache_control,omitempty"`
}

type messagesRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	System   json.RawMessage `json:"system"`
	Messages []struct {
		Role         string          `json:"role"`
		Content      json.RawMessage `json:"content"`
		OutputConfig json.RawMessage `json:"output_config"`
	} `json:"messages"`
	MaxTokens   *uint64  `json:"max_tokens"`
	Temperature *float64 `json:"temperature"`
	TopP        *float64 `json:"top_p"`
	TopK        *uint64  `json:"top_k"`
	Stop        []string `json:"stop_sequences"`
	Thinking    *struct {
		Type   string  `json:"type"`
		Budget *uint64 `json:"budget_tokens"`
	} `json:"thinking"`
	OutputConfig *struct {
		Effort string `json:"effort"`
	} `json:"output_config"`
	Tools []struct {
		Type        string          `json:"type"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Schema      json.RawMessage `json:"input_schema"`
		Cache       json.RawMessage `json:"cache_control"`
	} `json:"tools"`
	ToolChoice *struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Disabled *bool  `json:"disable_parallel_tool_use"`
	} `json:"tool_choice"`
}

func parseMessagesBlocks(raw json.RawMessage) ([]messagesBlock, error) {
	var text string
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' && json.Unmarshal(raw, &text) == nil {
		return []messagesBlock{{Type: "text", Text: &text}}, nil
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || parts == nil {
		return nil, errors.New("Messages content requires text or blocks")
	}
	blocks := make([]messagesBlock, 0, len(parts))
	for _, part := range parts {
		var block messagesBlock
		if strictJSON(part, &block) != nil {
			return nil, errors.New("unsupported Messages block")
		}
		object, _ := catalogObject(part)
		allowed := map[string]bool{"type": true, "cache_control": true}
		fields := map[string][]string{
			"text": {"text"}, "image": {"source"}, "thinking": {"thinking", "signature"},
			"tool_use": {"id", "name", "input"}, "tool_result": {"tool_use_id", "content", "is_error"},
		}[block.Type]
		if fields == nil {
			return nil, errors.New("unsupported Messages block type")
		}
		for _, field := range fields {
			allowed[field] = true
		}
		for field := range object {
			if !allowed[field] {
				return nil, errors.New("Messages block has incompatible fields")
			}
		}
		if len(block.Cache) > 0 {
			cache, err := normalizePromptCache(block.Cache)
			if err != nil {
				return nil, errors.New("unsupported Messages cache control")
			}
			block.Cache = cache
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func messagesImage(block messagesBlock) (json.RawMessage, error) {
	if block.Source == nil || block.Source.Type != "base64" {
		return nil, errors.New("Messages images require inline base64 data")
	}
	part, _ := json.Marshal(map[string]any{"type": "image_url", "image_url": map[string]string{
		"url": "data:" + block.Source.MediaType + ";base64," + block.Source.Data}})
	content := append(append([]byte{'['}, part...), ']')
	if _, images, err := encodeOutboundContent(content); err != nil || len(images) != 1 {
		return nil, errors.New("unsupported Messages image")
	}
	return content, nil
}

func EncodeMessagesRequest(body []byte, key string, resolve SignatureResolver) (EncodedChat, error) {
	if len(body) > maxBody {
		return EncodedChat{}, errors.New("Messages request exceeds its byte limit")
	}
	var request messagesRequest
	if strictJSON(body, &request) != nil || request.Model == "" || len(request.Messages) == 0 ||
		request.MaxTokens == nil {
		return EncodedChat{}, errors.New("invalid Messages request")
	}
	controls := outboundChat{Model: request.Model, Stream: request.Stream, MaxTokens: request.MaxTokens,
		Temperature: request.Temperature, TopP: request.TopP, TopK: request.TopK}
	var err error
	controls.Effort, err = request.effort()
	if err != nil {
		return EncodedChat{}, err
	}
	if request.Stop != nil {
		controls.Stop, _ = json.Marshal(request.Stop)
	}
	for _, tool := range request.Tools {
		if tool.Type != "" && tool.Type != "custom" || len(tool.Cache) != 0 {
			return EncodedChat{}, errors.New("server tools and tool cache boundaries are unsupported")
		}
		var entry struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
				Strict      *bool           `json:"strict"`
			} `json:"function"`
		}
		entry.Type, entry.Function.Name, entry.Function.Description, entry.Function.Parameters =
			"function", tool.Name, tool.Description, tool.Schema
		controls.Tools = append(controls.Tools, entry)
	}
	if choice := request.ToolChoice; choice != nil {
		switch choice.Type {
		case "auto", "none", "any":
			if choice.Name != "" {
				return EncodedChat{}, errors.New("unexpected named Messages tool choice")
			}
			mode := choice.Type
			if mode == "any" {
				mode = "required"
			}
			controls.ToolChoice, _ = json.Marshal(mode)
		case "tool":
			if choice.Name == "" {
				return EncodedChat{}, errors.New("Messages tool choice needs a name")
			}
			controls.ToolChoice, _ = json.Marshal(map[string]any{"type": "function", "function": map[string]string{"name": choice.Name}})
		default:
			return EncodedChat{}, errors.New("unsupported Messages tool choice")
		}
		if choice.Disabled != nil {
			parallel := !*choice.Disabled
			controls.Parallel = &parallel
		}
	}
	var prompts, validation [][]byte
	pending, seen := map[string]bool{}, map[string]bool{}
	var notices [][]byte
	var thought, thoughtSignature string
	var thoughtOrigin NativeSignature
	appendPrompt := func(entry outboundMessage, signature string) error {
		if signature != "" {
			if resolve == nil {
				return errors.New("signed Messages history requires a scoped origin")
			}
			origin, err := resolve(signature)
			if err != nil {
				return err
			}
			if origin.Format == "" || len(origin.Format) > 256 || len(origin.Model) > 512 {
				return errors.New("signed Messages history has no native signature format")
			}
			thought, thoughtSignature, thoughtOrigin = entry.ReasoningContent, signature, origin
			return nil
		}
		if thoughtSignature != "" {
			text, _, err := encodeOutboundContent(entry.Content)
			if err != nil || entry.Role != "assistant" {
				return errors.New("signed thinking requires assistant text or a tool call")
			}
			if text == "" && len(entry.ToolCalls) == 0 {
				if len(entry.Cache) > 0 {
					return errors.New("empty assistant cache boundary cannot carry signed thinking")
				}
				return nil
			}
			entry.ReasoningContent = thought
		}
		prompt, err := encodeOutboundMessage(entry)
		if err != nil {
			return err
		}
		validation = append(validation, prompt)
		if thoughtSignature != "" {
			prompt = addText(addText(prompt, 12, thoughtSignature), 18, thoughtOrigin.Format)
			prompt = addText(prompt, 22, thoughtOrigin.Model)
			thought, thoughtSignature, thoughtOrigin = "", "", NativeSignature{}
		}
		prompts = append(prompts, prompt)
		return nil
	}
	flushNotices := func() {
		prompts = append(prompts, notices...)
		validation = append(validation, notices...)
		notices = nil
	}
	textEntry := func(role, text string, cache json.RawMessage) outboundMessage {
		content, _ := json.Marshal(text)
		return outboundMessage{Role: role, Content: content, Cache: cache}
	}
	if len(request.System) > 0 {
		blocks, err := parseMessagesBlocks(request.System)
		if err != nil {
			return EncodedChat{}, err
		}
		for _, block := range blocks {
			if block.Type != "text" || block.Text == nil {
				return EncodedChat{}, errors.New("Messages system requires text blocks")
			}
			if err := appendPrompt(textEntry("system", *block.Text, block.Cache), ""); err != nil {
				return EncodedChat{}, err
			}
		}
	}
	for _, turn := range request.Messages {
		if turn.Role != "user" && turn.Role != "assistant" && turn.Role != "system" {
			return EncodedChat{}, errors.New("unsupported Messages role")
		}
		blocks, err := parseMessagesBlocks(turn.Content)
		if err != nil {
			return EncodedChat{}, err
		}
		for _, block := range blocks {
			entry := outboundMessage{Role: turn.Role, Cache: block.Cache}
			signature := ""
			switch block.Type {
			case "text":
				if block.Text == nil {
					return EncodedChat{}, errors.New("Messages text is missing")
				}
				if turn.Role == "user" && strings.TrimSpace(*block.Text) == "" {
					if len(block.Cache) > 0 {
						return EncodedChat{}, errors.New("whitespace-only user cache boundary is unsupported")
					}
					continue
				}
				entry = textEntry(turn.Role, *block.Text, block.Cache)
			case "image":
				if turn.Role != "user" {
					return EncodedChat{}, errors.New("Messages image role is unsupported")
				}
				entry.Content, err = messagesImage(block)
				if err != nil {
					return EncodedChat{}, err
				}
			case "thinking":
				if turn.Role != "assistant" || block.Thinking == nil || len(block.Cache) > 0 {
					return EncodedChat{}, errors.New("unsupported Messages thinking")
				}
				if block.Signature == "" && isClaudeModel(request.Model) {
					continue
				}
				entry.ReasoningContent, signature = *block.Thinking, block.Signature
				if signature != "" && !isClaudeModel(request.Model) {
					return EncodedChat{}, errors.New("signed Messages history requires a matching native model")
				}
			case "tool_use":
				if turn.Role != "assistant" || block.ID == "" || block.Name == "" || !jsonObject(block.Input) ||
					seen[NativeToolID(block.ID)] {
					return EncodedChat{}, errors.New("invalid Messages tool call")
				}
				id := NativeToolID(block.ID)
				seen[id], pending[id] = true, true
				call, _ := json.Marshal(map[string]any{"role": "assistant", "cache_control": json.RawMessage(block.Cache),
					"tool_calls": []any{map[string]any{"id": block.ID, "type": "function",
						"function": map[string]string{"name": block.Name, "arguments": string(block.Input)}}}})
				if strictJSON(call, &entry) != nil {
					return EncodedChat{}, errors.New("invalid Messages tool call")
				}
				if len(block.Cache) == 0 {
					entry.Cache = nil
				}
			case "tool_result":
				id := NativeToolID(block.ToolUseID)
				if turn.Role != "user" || !pending[id] || block.IsError != nil && *block.IsError {
					return EncodedChat{}, errors.New("orphan, duplicate or failed Messages tool result")
				}
				inner, err := parseMessagesBlocks(block.Content)
				if err != nil {
					return EncodedChat{}, err
				}
				var parts []json.RawMessage
				for index, part := range inner {
					if len(part.Cache) > 0 {
						if index != len(inner)-1 {
							return EncodedChat{}, errors.New("nested tool result cache boundary is unsupported")
						}
						entry.Cache = part.Cache
					}
					switch part.Type {
					case "text":
						if part.Text == nil {
							return EncodedChat{}, errors.New("tool result text is missing")
						}
						value, _ := json.Marshal(map[string]string{"type": "text", "text": *part.Text})
						parts = append(parts, value)
					case "image":
						image, err := messagesImage(part)
						if err != nil {
							return EncodedChat{}, err
						}
						var values []json.RawMessage
						_ = json.Unmarshal(image, &values)
						parts = append(parts, values...)
					default:
						return EncodedChat{}, errors.New("unsupported Messages tool result block")
					}
				}
				entry.Role, entry.ToolCallID = "tool", block.ToolUseID
				entry.Content, _ = json.Marshal(parts)
				if parts == nil {
					entry.Content = json.RawMessage(`""`)
				}
				delete(pending, id)
			}
			if turn.Role == "system" {
				if block.Type != "text" {
					return EncodedChat{}, errors.New("Messages notice must be text")
				}
				entry.Role = "user"
				if len(pending) > 0 {
					prompt, err := encodeOutboundMessage(entry)
					if err != nil {
						return EncodedChat{}, err
					}
					notices = append(notices, prompt)
					continue
				}
			}
			if len(pending) > 0 && entry.Role != "assistant" && entry.Role != "tool" {
				return EncodedChat{}, errors.New("Messages tool results are incomplete")
			}
			if err := appendPrompt(entry, signature); err != nil {
				return EncodedChat{}, err
			}
			if entry.Role == "tool" && len(pending) == 0 {
				flushNotices()
			}
			if len(prompts)+len(notices) > 2048 {
				return EncodedChat{}, errors.New("too many Messages blocks")
			}
		}
		if thoughtSignature != "" {
			return EncodedChat{}, errors.New("signed thinking requires assistant text or a tool call")
		}
	}
	if len(pending) > 0 {
		return EncodedChat{}, errors.New("Messages tool results are incomplete")
	}
	flushNotices()
	payload := addText(nil, 21, request.Model)
	for _, prompt := range validation {
		payload = addBytes(payload, 3, prompt)
	}
	encoded, err := finishOutboundRequest(controls, payload, key)
	if err != nil {
		return EncodedChat{}, err
	}
	fields, _ := RawFields(encoded.Payload)
	encoded.Payload = nil
	for number := 1; number <= 28; number++ {
		if number == 3 {
			for _, prompt := range prompts {
				encoded.Payload = addBytes(encoded.Payload, 3, prompt)
			}
		} else {
			encoded.Payload = append(encoded.Payload, fields[number]...)
		}
	}
	if len(encoded.Payload) > maxBody {
		return EncodedChat{}, errors.New("native Messages request exceeds its byte limit")
	}
	return encoded, nil
}
