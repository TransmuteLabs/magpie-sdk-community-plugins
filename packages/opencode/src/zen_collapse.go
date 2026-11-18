package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
)

type zenCollapsedTool struct {
	index int
	value map[string]any
}

type zenCollapsedChoice struct {
	value map[string]any
	open  map[int]map[string]any
	calls []zenCollapsedTool
}

type zenCollapse struct {
	protocol  string
	body      map[string]any
	choices   map[int]*zenCollapsedChoice
	blocks    map[int]map[string]any
	arguments map[int]string
	complete  bool
	bytes     int
	positions int
}

func (collapse *zenCollapse) position() error {
	collapse.positions++
	if collapse.positions > 10000 {
		return errors.New("collapsed Zen reply exceeds its position limit")
	}
	return nil
}

func collapseIndex(value any) (int, error) {
	index, err := zenToolIndex(value)
	if err != nil || index > 65535 {
		return 0, errors.New("invalid collapsed Zen item index")
	}
	return index, nil
}

func collapseUsage(target map[string]any, value any) error {
	if value == nil {
		return nil
	}
	usage := zenToolObject(value)
	if usage == nil {
		return errors.New("invalid collapsed Zen usage")
	}
	current := zenToolObject(target["usage"])
	if current == nil {
		current = map[string]any{}
	}
	maps.Copy(current, usage)
	target["usage"] = current
	return nil
}

func collapseStrings(target, delta map[string]any, fields ...string) error {
	for _, key := range fields {
		if delta[key] == nil {
			continue
		}
		text, ok := delta[key].(string)
		if !ok {
			return errors.New("invalid collapsed Zen text delta")
		}
		target[key] = zenToolString(target[key]) + text
	}
	return nil
}

func (collapse *zenCollapse) chat(value map[string]any) error {
	for _, key := range []string{"id", "created", "model", "system_fingerprint", "service_tier"} {
		if item, found := value[key]; found {
			collapse.body[key] = item
		}
	}
	if err := collapseUsage(collapse.body, value["usage"]); err != nil {
		return err
	}
	parts, err := zenToolList(value["choices"])
	if err != nil {
		return err
	}
	for _, item := range parts {
		part := zenToolObject(item)
		if part == nil {
			return errors.New("invalid collapsed Zen choice")
		}
		index, err := collapseIndex(part["index"])
		if err != nil {
			return err
		}
		choice := collapse.choices[index]
		if choice == nil {
			if err := collapse.position(); err != nil {
				return err
			}
			choice = &zenCollapsedChoice{
				value: map[string]any{"index": part["index"], "message": map[string]any{"role": "assistant", "content": nil}, "finish_reason": nil},
				open:  map[int]map[string]any{},
			}
			if choice.value["index"] == nil {
				choice.value["index"] = 0
			}
			collapse.choices[index] = choice
		}
		message, delta := zenToolObject(choice.value["message"]), zenToolObject(part["delta"])
		if part["delta"] != nil && delta == nil {
			return errors.New("invalid collapsed Zen choice delta")
		}
		if role := zenToolString(delta["role"]); role != "" {
			message["role"] = role
		}
		if err := collapseStrings(message, delta, "content", "reasoning_content", "reasoning", "refusal"); err != nil {
			return err
		}
		if delta["function_call"] != nil {
			function := zenToolObject(message["function_call"])
			if function == nil {
				function = map[string]any{"name": "", "arguments": ""}
			}
			if err := zenAppendFunction(function, delta["function_call"]); err != nil {
				return err
			}
			message["function_call"] = function
		}
		fragments, err := zenToolList(delta["tool_calls"])
		if err != nil {
			return err
		}
		for _, fragment := range fragments {
			tool := zenToolObject(fragment)
			if tool == nil {
				return errors.New("invalid collapsed Zen tool fragment")
			}
			position, err := collapseIndex(tool["index"])
			if err != nil {
				return err
			}
			call := choice.open[position]
			id := zenToolString(tool["id"])
			if call == nil || id != "" && zenToolString(call["id"]) != "" && call["id"] != id {
				if err := collapse.position(); err != nil {
					return err
				}
				call = map[string]any{"type": "function", "function": map[string]any{"name": "", "arguments": ""}}
				choice.open[position] = call
				choice.calls = append(choice.calls, zenCollapsedTool{index: position, value: call})
			}
			for _, key := range []string{"id", "type"} {
				if value, found := tool[key]; found {
					call[key] = value
				}
			}
			if tool["function"] != nil {
				if err := zenAppendFunction(zenToolObject(call["function"]), tool["function"]); err != nil {
					return err
				}
			}
		}
		if part["finish_reason"] != nil {
			choice.value["finish_reason"] = part["finish_reason"]
		}
		if part["logprobs"] != nil {
			probabilities := zenToolObject(part["logprobs"])
			if probabilities == nil {
				return errors.New("invalid collapsed Zen logprobs")
			}
			current := zenToolObject(choice.value["logprobs"])
			if current == nil {
				current = map[string]any{}
			}
			for key, value := range probabilities {
				if values, ok := value.([]any); ok {
					previous, err := zenToolList(current[key])
					if err != nil {
						return err
					}
					current[key] = append(previous, values...)
				} else {
					current[key] = value
				}
			}
			choice.value["logprobs"] = current
		}
	}
	return nil
}

func (collapse *zenCollapse) messages(value map[string]any, kind string) error {
	switch kind {
	case "message_start":
		message := zenToolObject(value["message"])
		if message == nil {
			return errors.New("invalid collapsed Zen message start")
		}
		maps.Copy(collapse.body, message)
		parts, err := zenToolList(message["content"])
		if err != nil {
			return err
		}
		for index, part := range parts {
			block := zenToolObject(part)
			if block == nil {
				return errors.New("invalid collapsed Zen content block")
			}
			if err := collapse.position(); err != nil {
				return err
			}
			collapse.blocks[index] = block
		}
	case "content_block_start", "content_block_delta":
		index, err := collapseIndex(value["index"])
		if err != nil {
			return err
		}
		if kind == "content_block_start" {
			block := zenToolObject(value["content_block"])
			if block == nil {
				return errors.New("invalid collapsed Zen content start")
			}
			if err := collapse.position(); err != nil {
				return err
			}
			collapse.blocks[index] = block
			delete(collapse.arguments, index)
			return nil
		}
		block, delta := collapse.blocks[index], zenToolObject(value["delta"])
		if block == nil || delta == nil {
			return errors.New("Zen content delta requires its content block")
		}
		field := map[string]string{"text_delta": "text", "thinking_delta": "thinking", "signature_delta": "signature"}[zenToolString(delta["type"])]
		if field != "" {
			if err := collapseStrings(block, delta, field); err != nil {
				return err
			}
		}
		if delta["type"] == "input_json_delta" {
			text, ok := delta["partial_json"].(string)
			if !ok {
				return errors.New("invalid collapsed Zen tool input delta")
			}
			collapse.arguments[index] += text
		}
	case "message_delta":
		delta := zenToolObject(value["delta"])
		if value["delta"] != nil && delta == nil {
			return errors.New("invalid collapsed Zen message delta")
		}
		maps.Copy(collapse.body, delta)
		return collapseUsage(collapse.body, value["usage"])
	case "message_stop":
		collapse.complete = true
	}
	return nil
}

func (collapse *zenCollapse) write(body []byte) error {
	collapse.bytes += len(body)
	if collapse.bytes > maxZenToolBytes {
		return errors.New("collapsed Zen stream exceeds its byte limit")
	}
	reader, first := bufio.NewReader(bytes.NewReader(body)), true
	for {
		frame, err := readZenToolFrame(reader, &first)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if frame.event == "error" {
			return errors.New("Zen stream reported an upstream error")
		}
		data := bytes.TrimSpace(frame.data)
		if len(data) == 0 {
			continue
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			collapse.complete = collapse.protocol == "chat"
			continue
		}
		value, err := zenToolJSON(data)
		if err != nil {
			return err
		}
		kind := zenToolString(value["type"])
		if kind == "" {
			kind = frame.event
		}
		switch collapse.protocol {
		case "chat":
			err = collapse.chat(value)
		case "responses":
			if kind == "response.completed" {
				response := zenToolObject(value["response"])
				if response == nil || response["status"] != "completed" {
					return errZenToolsTruncated
				}
				collapse.body, collapse.complete = response, true
			}
		case "messages":
			err = collapse.messages(value, kind)
		default:
			return errors.New("unsupported collapsed Zen protocol")
		}
		if err != nil {
			return err
		}
	}
}

func collapseSorted[T any](values map[int]T) []T {
	indices := slices.Sorted(maps.Keys(values))
	result := make([]T, 0, len(indices))
	for _, index := range indices {
		result = append(result, values[index])
	}
	return result
}

func collapseZenReply(ctx context.Context, reader io.Reader, policy *zenToolPolicy, protocol string) ([]byte, error) {
	collapse := zenCollapse{protocol: protocol, body: map[string]any{},
		choices: map[int]*zenCollapsedChoice{}, blocks: map[int]map[string]any{}, arguments: map[int]string{}}
	if protocol == "chat" {
		collapse.body = map[string]any{"object": "chat.completion", "choices": []any{}}
	}
	if protocol == "messages" {
		collapse.body = map[string]any{"type": "message", "role": "assistant", "content": []any{}, "stop_reason": nil, "stop_sequence": nil}
	}
	if err := relayZenTools(ctx, reader, policy, protocol, true, collapse.write); err != nil {
		return nil, err
	}
	if !collapse.complete {
		return nil, errZenToolsTruncated
	}
	if protocol == "chat" {
		choices := []any{}
		for _, choice := range collapseSorted(collapse.choices) {
			if len(choice.calls) != 0 {
				slices.SortStableFunc(choice.calls, func(first, second zenCollapsedTool) int { return first.index - second.index })
				calls := make([]any, 0, len(choice.calls))
				for _, call := range choice.calls {
					calls = append(calls, call.value)
				}
				zenToolObject(choice.value["message"])["tool_calls"] = calls
			}
			choices = append(choices, choice.value)
		}
		collapse.body["choices"] = choices
	}
	if protocol == "messages" {
		for index, text := range collapse.arguments {
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			var value any
			if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
				return nil, errors.New("invalid collapsed Zen tool input JSON")
			}
			collapse.blocks[index]["input"] = value
		}
		collapse.body["content"] = collapseSorted(collapse.blocks)
	}
	body, err := json.Marshal(collapse.body)
	if len(body) > maxZenToolBytes {
		return nil, errors.New("collapsed Zen JSON exceeds its byte limit")
	}
	return body, err
}
