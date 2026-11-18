package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const MaxBody = 16 << 20

var ErrInvalid = errors.New("invalid ZCode Messages body")
var ErrToolCollision = errors.New("ToolSearch and ToolSearchTool cannot be exposed together on ZCode")

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func ReadObject(body []byte) (map[string]any, error) {
	if len(body) == 0 || len(body) > MaxBody {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := readValue(decoder, 0)
	if err != nil || object(value) == nil {
		return nil, ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	return object(value), nil
}

func readValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return token, nil
	}
	switch delimiter {
	case '{':
		value := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			name, valid := key.(string)
			if err != nil || !valid {
				return nil, ErrInvalid
			}
			if _, duplicate := value[name]; duplicate {
				return nil, ErrInvalid
			}
			value[name], err = readValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return nil, ErrInvalid
		}
		return value, nil
	case '[':
		value := []any{}
		for decoder.More() {
			next, err := readValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value = append(value, next)
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, ErrInvalid
		}
		return value, nil
	default:
		return nil, ErrInvalid
	}
}

func Prepare(body []byte, model string) ([]byte, bool, error) {
	request, err := ReadObject(body)
	if err != nil || request["model"] != model || model == "" {
		return nil, false, ErrInvalid
	}
	messages, valid := request["messages"].([]any)
	if !valid {
		return nil, false, ErrInvalid
	}
	for _, message := range messages {
		value := object(message)
		if value == nil || value["role"] != "user" && value["role"] != "assistant" {
			return nil, false, ErrInvalid
		}
	}
	for _, key := range []string{"thinking", "output_config"} {
		if value := request[key]; value != nil && object(value) == nil {
			return nil, false, ErrInvalid
		}
	}
	normalizeReasoning(request, model)
	renamed, err := searchless(request)
	if err != nil {
		return nil, false, err
	}
	prepared, err := json.Marshal(request)
	if len(prepared) > MaxBody {
		return nil, false, ErrInvalid
	}
	return prepared, renamed, err
}

func normalizeReasoning(request map[string]any, model string) {
	lower := strings.ToLower(model)
	version := 0
	if lower == "glm-5.2" || strings.HasPrefix(lower, "glm-5.2-") {
		version = 2
	} else if lower == "glm-5.3" || strings.HasPrefix(lower, "glm-5.3-") {
		version = 3
	}
	if version == 0 {
		return
	}
	thinking, output := object(request["thinking"]), object(request["output_config"])
	requested, _ := output["effort"].(string)
	requested = strings.ToLower(strings.TrimSpace(requested))
	kind, _ := thinking["type"].(string)
	off := kind == "disabled" || kind == "between_tools" || requested == "none"
	if !off && kind != "enabled" && kind != "adaptive" && !(thinking == nil && requested != "") {
		return
	}
	if thinking == nil {
		thinking = map[string]any{}
	}
	budget, _ := thinking["budget_tokens"].(json.Number)
	tokens, _ := budget.Float64()
	delete(thinking, "budget_tokens")
	delete(thinking, "display")
	if off && version == 2 {
		thinking["type"] = "disabled"
		delete(output, "effort")
	} else {
		thinking["type"] = "enabled"
		effort := requested
		if off {
			effort = "low"
		} else if effort == "" && tokens > 0 {
			switch {
			case tokens <= 4096:
				effort = "low"
			case tokens <= 24000:
				effort = "high"
			default:
				effort = "max"
			}
		}
		switch {
		case effort == "max" || effort == "xhigh" || effort == "ultra":
			effort = "max"
		case version == 3 && (effort == "low" || effort == "minimal"):
			effort = "low"
		default:
			effort = "high"
		}
		if output == nil {
			output = map[string]any{}
		}
		output["effort"] = effort
		request["output_config"] = output
	}
	request["thinking"] = thinking
}

func searchless(request map[string]any) (bool, error) {
	tools, hasTools := request["tools"].([]any)
	if _, present := request["tools"]; present && !hasTools {
		return false, ErrInvalid
	}
	hasSearch, hasAlias := false, false
	for _, tool := range tools {
		value := object(tool)
		if value == nil {
			return false, ErrInvalid
		}
		hasSearch = hasSearch || value["name"] == "ToolSearch"
		hasAlias = hasAlias || value["name"] == "ToolSearchTool"
	}
	if !hasSearch {
		return false, nil
	}
	if hasAlias {
		return false, ErrToolCollision
	}
	kept := make([]any, 0, len(tools))
	for _, tool := range tools {
		value := object(tool)
		if value["name"] == "DeferredToolPlaceholder" {
			continue
		}
		delete(value, "defer_loading")
		if value["name"] == "ToolSearch" {
			value["name"] = "ToolSearchTool"
		}
		kept = append(kept, value)
	}
	request["tools"] = kept
	for _, message := range request["messages"].([]any) {
		value := object(message)
		if value["role"] != "assistant" {
			continue
		}
		blocks, _ := value["content"].([]any)
		for _, block := range blocks {
			value := object(block)
			if value["type"] == "tool_use" && value["name"] == "ToolSearch" {
				value["name"] = "ToolSearchTool"
			}
		}
	}
	choice := object(request["tool_choice"])
	if choice["type"] == "tool" && choice["name"] == "ToolSearch" {
		choice["name"] = "ToolSearchTool"
	}
	return true, nil
}

func Restore(body []byte, renamed bool, responseModel ...string) ([]byte, error) {
	model := ""
	if len(responseModel) > 0 {
		model = responseModel[0]
	}
	if !renamed && model == "" {
		return body, nil
	}
	response, err := ReadObject(body)
	if err != nil {
		return nil, err
	}
	restoreBlock := func(value map[string]any) {
		if renamed && value["type"] == "tool_use" && value["name"] == "ToolSearchTool" {
			value["name"] = "ToolSearch"
		}
	}
	if response["type"] == "content_block_start" {
		restoreBlock(object(response["content_block"]))
	}
	message := response
	if response["type"] == "message_start" {
		message = object(response["message"])
	}
	if message["type"] == "message" {
		if model != "" {
			message["model"] = model
		}
		content, _ := message["content"].([]any)
		for _, block := range content {
			restoreBlock(object(block))
		}
	}
	restored, err := json.Marshal(response)
	if len(restored) > MaxBody {
		return nil, ErrInvalid
	}
	return restored, err
}
