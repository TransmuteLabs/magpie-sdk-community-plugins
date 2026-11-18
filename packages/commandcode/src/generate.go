package commandcode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Place struct {
	Directory   string
	Date        string
	Environment string
}

func GenerateBody(raw []byte, place Place) ([]byte, error) {
	var fields map[string]json.RawMessage
	if decode(raw, &fields) != nil || fields == nil {
		return nil, errors.New("native generate requires a Chat JSON object")
	}
	allowed := set("model", "messages", "tools", "stream", "stream_options", "max_tokens", "max_completion_tokens", "temperature", "reasoning_effort", "tool_choice", "parallel_tool_calls")
	for field, value := range fields {
		if !allowed[field] && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("native generate does not implement Chat field %q", field)
		}
	}
	if rawOptions, found := fields["stream_options"]; found && !bytes.Equal(bytes.TrimSpace(rawOptions), []byte("null")) {
		var options map[string]json.RawMessage
		if decode(rawOptions, &options) != nil || options == nil {
			return nil, errors.New("native generate stream_options must be a JSON object")
		}
		for name, value := range options {
			if name != "include_usage" || !bytes.Equal(bytes.TrimSpace(value), []byte("true")) {
				return nil, errors.New("native generate supports only stream_options.include_usage=true")
			}
		}
	}
	var chat struct {
		Model    string `json:"model"`
		Messages []struct {
			Role           string          `json:"role"`
			Content        json.RawMessage `json:"content"`
			Reasoning      string          `json:"reasoning_content"`
			ReasoningAlias string          `json:"reasoning"`
			ToolID         string          `json:"tool_call_id"`
			Calls          []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
		Max           *int64          `json:"max_tokens"`
		MaxCompletion *int64          `json:"max_completion_tokens"`
		Temperature   *float64        `json:"temperature"`
		Effort        string          `json:"reasoning_effort"`
		ToolChoice    json.RawMessage `json:"tool_choice"`
		Parallel      *bool           `json:"parallel_tool_calls"`
		Stream        bool            `json:"stream"`
		StreamOptions json.RawMessage `json:"stream_options"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&chat); err != nil {
		return nil, fmt.Errorf("native generate request has unsupported or malformed fields: %w", err)
	}
	if chat.Model == "" || isDecision(chat.Model) || len(chat.Messages) == 0 || len(chat.Messages) > 10000 {
		return nil, errors.New("native generate requires a conversational model and bounded messages")
	}
	if len(chat.ToolChoice) > 0 && string(chat.ToolChoice) != `"auto"` && string(chat.ToolChoice) != "null" {
		return nil, errors.New("native generate tool_choice is not recovered beyond auto")
	}
	if chat.Parallel != nil && !*chat.Parallel {
		return nil, errors.New("native generate cannot enforce parallel_tool_calls=false")
	}
	if chat.Effort == "none" || chat.Effort == "off" {
		known, found := knownGoModel(chat.Model)
		if !found || len(known.Efforts) == 0 {
			return nil, errors.New("native generate cannot enforce disabled reasoning for a model without recovered thinking support")
		}
		chat.Effort = "off"
	}
	if place.Directory == "" || !strings.HasPrefix(place.Directory, "/") || strings.ContainsAny(place.Directory, "\x00\r\n") {
		return nil, errors.New("native generate requires a configured absolute working directory")
	}
	messages := make([]any, 0, len(chat.Messages))
	var system []string
	names := make(map[string]string)
	resolved := make(map[string]bool)
	for _, message := range chat.Messages {
		parts, err := generateParts(message.Content, message.Role == "user")
		if err != nil {
			return nil, err
		}
		switch message.Role {
		case "system", "developer":
			for _, part := range parts {
				system = append(system, part["text"].(string))
			}
		case "user":
			messages = append(messages, map[string]any{"role": "user", "content": parts})
		case "assistant":
			thought := message.Reasoning
			if thought == "" {
				thought = message.ReasoningAlias
			}
			if thought != "" {
				parts = append([]map[string]any{{"type": "reasoning", "text": thought}}, parts...)
			}
			for _, call := range message.Calls {
				if call.Type != "function" || call.ID == "" || call.Function.Name == "" || names[call.ID] != "" {
					return nil, errors.New("native generate tool identity is invalid or duplicated")
				}
				var input map[string]any
				if decode([]byte(call.Function.Arguments), &input) != nil || input == nil {
					return nil, errors.New("native generate tool arguments must be a JSON object")
				}
				names[call.ID] = call.Function.Name
				parts = append(parts, map[string]any{"type": "tool-call", "toolCallId": call.ID, "toolName": call.Function.Name, "input": input})
			}
			messages = append(messages, map[string]any{"role": "assistant", "content": parts})
		case "tool":
			if message.ToolID == "" || names[message.ToolID] == "" || resolved[message.ToolID] {
				return nil, errors.New("native generate tool result has no unique preceding call")
			}
			resolved[message.ToolID] = true
			var text []string
			for _, part := range parts {
				text = append(text, part["text"].(string))
			}
			messages = append(messages, map[string]any{"role": "tool", "content": []any{map[string]any{
				"type": "tool-result", "toolCallId": message.ToolID, "toolName": names[message.ToolID],
				"output": map[string]any{"type": "text", "value": strings.Join(text, "\n")},
			}}})
		default:
			return nil, errors.New("native generate message role is unsupported")
		}
	}
	tools := make([]any, 0, len(chat.Tools))
	toolNames := make(map[string]bool)
	for _, tool := range chat.Tools {
		if tool.Type != "function" || tool.Function.Name == "" || toolNames[tool.Function.Name] {
			return nil, errors.New("native generate requires uniquely named function tools")
		}
		toolNames[tool.Function.Name] = true
		schema := tool.Function.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		var object map[string]any
		if decode(schema, &object) != nil || object == nil {
			return nil, errors.New("native generate tool schema must be a JSON object")
		}
		if isGoogle(chat.Model) {
			normalized, err := googleSchema(object)
			if err != nil {
				return nil, err
			}
			object = normalized
		}
		tools = append(tools, map[string]any{"name": tool.Function.Name, "description": tool.Function.Description, "input_schema": object})
	}
	maxTokens := int64(64000)
	for _, candidate := range []*int64{chat.Max, chat.MaxCompletion} {
		if candidate != nil {
			if *candidate <= 0 {
				return nil, errors.New("native generate token limit must be positive")
			}
			maxTokens = min(*candidate, 200000)
		}
	}
	params := map[string]any{"model": chat.Model, "messages": messages, "tools": tools, "system": strings.Join(system, "\n\n"), "max_tokens": maxTokens, "stream": true}
	if chat.Temperature != nil {
		if *chat.Temperature < 0 || *chat.Temperature > 2 {
			return nil, errors.New("native generate temperature is invalid")
		}
		params["temperature"] = *chat.Temperature
	}
	if chat.Effort != "" {
		if !set("off", "minimal", "low", "medium", "high", "xhigh", "max", "ultra")[chat.Effort] {
			return nil, errors.New("native generate reasoning effort is unsupported")
		}
		if known, found := knownGoModel(chat.Model); found && len(known.Efforts) == 0 {
			return nil, errors.New("native generate model has no recovered thinking support")
		}
		effort := chat.Effort
		if effort != "off" {
			effort = fitEffort(chat.Model, effort)
		}
		if effort == "minimal" {
			return nil, errors.New("native generate cannot recover a supported reasoning effort for this model")
		}
		params["reasoning_effort"] = effort
	}
	return json.Marshal(map[string]any{
		"config": map[string]any{"workingDir": place.Directory, "date": place.Date, "environment": place.Environment, "structure": []any{}, "isGitRepo": false, "currentBranch": "", "mainBranch": "", "gitStatus": "", "recentCommits": []any{}},
		"memory": nil, "taste": nil, "skills": nil, "permissionMode": "standard", "params": params,
	})
}

func generateParts(raw json.RawMessage, images bool) ([]map[string]any, error) {
	parts := make([]map[string]any, 0)
	if len(raw) == 0 || string(raw) == "null" {
		return parts, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return append(parts, map[string]any{"type": "text", "text": text}), nil
	}
	var source []map[string]json.RawMessage
	if decode(raw, &source) != nil || source == nil {
		return nil, errors.New("native generate content must be text or typed parts")
	}
	for _, part := range source {
		if len(part) != 2 {
			return nil, errors.New("native generate content has unsupported part fields")
		}
		var kind string
		if json.Unmarshal(part["type"], &kind) != nil {
			return nil, errors.New("native generate content type is invalid")
		}
		switch kind {
		case "text":
			if json.Unmarshal(part["text"], &text) != nil {
				return nil, errors.New("native generate text is invalid")
			}
			parts = append(parts, map[string]any{"type": "text", "text": text})
		case "image_url":
			var image struct {
				URL    string `json:"url"`
				Detail string `json:"detail"`
			}
			decoder := json.NewDecoder(bytes.NewReader(part["image_url"]))
			decoder.DisallowUnknownFields()
			if !images || decoder.Decode(&image) != nil || image.URL == "" || image.Detail != "" && image.Detail != "auto" {
				return nil, errors.New("native generate image semantics are unsupported")
			}
			location, err := url.Parse(image.URL)
			if err != nil || location.User != nil || location.Scheme != "https" && location.Scheme != "data" {
				return nil, errors.New("native generate image URL must be HTTPS or inline data")
			}
			parts = append(parts, map[string]any{"type": "image", "image": image.URL})
		default:
			return nil, fmt.Errorf("native generate content part %q is unsupported", kind)
		}
	}
	return parts, nil
}

func fitEffort(model, want string) string {
	known, _ := knownGoModel(model)
	if want == "ultra" {
		want = "max"
	}
	rank := []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}
	wanted := 0
	for index, level := range rank {
		if level == want {
			wanted = index
		}
	}
	best, distance := want, len(rank)
	for index, level := range rank {
		for _, supported := range known.Efforts {
			if level != supported || level == "none" {
				continue
			}
			difference := index - wanted
			if difference < 0 {
				difference = -difference
			}
			if difference <= distance {
				best, distance = level, difference
			}
		}
	}
	return best
}
