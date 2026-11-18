package outbound

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type renderTool struct {
	ID   string
	Name string
	Args strings.Builder
}

type renderThought struct {
	text      strings.Builder
	signature strings.Builder
	format    string
}

type ChatRenderer struct {
	id            string
	writer        http.ResponseWriter
	model         string
	stream        bool
	started       bool
	finished      bool
	text          strings.Builder
	thinking      strings.Builder
	thoughts      []*renderThought
	thoughtByID   map[string]int
	signatureType string
	tools         []*renderTool
	byID          map[string]int
	lastTool      int
	buffered      int
}

func NewChatRenderer(writer http.ResponseWriter, model string, streaming bool) *ChatRenderer {
	return &ChatRenderer{id: "chatcmpl-devin-" + rand.Text(), writer: writer, model: model, stream: streaming, byID: map[string]int{}, thoughtByID: map[string]int{}, lastTool: -1}
}

func (renderer *ChatRenderer) Started() bool      { return renderer.started }
func (renderer *ChatRenderer) ResponseID() string { return renderer.id }

func (renderer *ChatRenderer) start() {
	if renderer.started {
		return
	}
	renderer.started = true
	renderer.writer.Header().Set("Content-Type", "application/json")
	if renderer.stream {
		renderer.writer.Header().Set("Content-Type", "text/event-stream")
	}
	renderer.writer.WriteHeader(http.StatusOK)
}

func (renderer *ChatRenderer) event(delta map[string]any, finish *string, usage any) error {
	value := map[string]any{"id": renderer.id, "object": "chat.completion.chunk", "model": renderer.model,
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	if usage != nil {
		value["usage"] = usage
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	renderer.start()
	if _, err := renderer.writer.Write(append(append([]byte("data: "), data...), '\n', '\n')); err != nil {
		return err
	}
	if flusher, ok := renderer.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func splitText(value string, emit func(string) error) error {
	for value != "" {
		size := min(len(value), 8192)
		for size < len(value) && !utf8.RuneStart(value[size]) {
			size--
		}
		if err := emit(value[:size]); err != nil {
			return err
		}
		value = value[size:]
	}
	return nil
}

func (renderer *ChatRenderer) Delta(delta codec.NativeDelta) error {
	if renderer.finished {
		return errors.New("canonical response is finished")
	}
	if delta.Phase != "" {
		return errors.New("native phase requires the paired native route")
	}
	if len(delta.SignatureType) > 256 {
		return errors.New("native signature type exceeds its limit")
	}
	if !renderer.stream {
		renderer.buffered += len(delta.Text) + len(delta.Thinking) + len(delta.Signature)
		if renderer.buffered > 2<<20 {
			return errors.New("canonical response exceeds its buffer limit")
		}
	}
	if delta.SignatureType != "" {
		if renderer.signatureType != "" && renderer.signatureType != delta.SignatureType {
			return errors.New("multiple signature types require the native route")
		}
		renderer.signatureType = delta.SignatureType
	}
	if !renderer.stream {
		renderer.text.WriteString(delta.Text)
		renderer.thinking.WriteString(delta.Thinking)
	} else {
		if err := splitText(delta.Text, func(text string) error { return renderer.event(map[string]any{"content": text}, nil, nil) }); err != nil {
			return err
		}
		if err := splitText(delta.Thinking, func(text string) error { return renderer.event(map[string]any{"reasoning_content": text}, nil, nil) }); err != nil {
			return err
		}
	}
	emit := func(index int, field, value string) error {
		if !renderer.stream {
			return nil
		}
		return splitText(value, func(part string) error {
			return renderer.event(map[string]any{"reasoning_details": []any{map[string]any{"type": "reasoning.text", "index": index, field: part}}}, nil, nil)
		})
	}
	// Signature metadata describes the reply; only content allocates a thought.
	if delta.Thinking != "" || delta.Signature != "" {
		id := delta.ThinkingID
		// Gemini's signature belongs to the reply; Claude's belongs to each thought.
		if codec.IsGeminiModel(renderer.model) {
			id = ""
		}
		index, found := renderer.thoughtByID[id]
		if !found {
			if len(renderer.thoughts) >= 256 {
				return errors.New("canonical response exceeds its thinking block limit")
			}
			index = len(renderer.thoughts)
			renderer.thoughtByID[id] = index
			renderer.thoughts = append(renderer.thoughts, &renderThought{})
		}
		thought := renderer.thoughts[index]
		if !renderer.stream {
			thought.text.WriteString(delta.Thinking)
			thought.signature.WriteString(delta.Signature)
		}
		if err := emit(index, "text", delta.Thinking); err != nil {
			return err
		}
		if err := emit(index, "signature", delta.Signature); err != nil {
			return err
		}
	}
	// A separate metadata frame may precede or follow the signed content.
	for index, thought := range renderer.thoughts {
		if thought.format == "" && renderer.signatureType != "" {
			thought.format = renderer.signatureType
			if err := emit(index, "format", thought.format); err != nil {
				return err
			}
		}
	}
	for _, change := range delta.Tools {
		index, found := renderer.byID[change.ID]
		if change.ID == "" {
			index, found = renderer.lastTool, renderer.lastTool >= 0
		}
		newTool := false
		if !found {
			if change.ID == "" || change.Name == "" || len(renderer.tools) >= 256 {
				return errors.New("invalid native tool association")
			}
			index = len(renderer.tools)
			renderer.byID[change.ID] = index
			renderer.tools = append(renderer.tools, &renderTool{ID: codec.CanonicalToolID(change.ID), Name: change.Name})
			newTool = true
		}
		renderer.lastTool = index
		tool := renderer.tools[index]
		if change.Name != "" && tool.Name != change.Name {
			return errors.New("native tool name changed")
		}
		renderer.buffered += len(change.Arguments)
		if renderer.buffered > 2<<20 {
			return errors.New("native tool arguments exceed the response limit")
		}
		tool.Args.WriteString(change.Arguments)
		if renderer.stream {
			arguments := change.Arguments
			first := true
			if arguments == "" && newTool {
				if err := renderer.event(map[string]any{"tool_calls": []any{map[string]any{"index": index, "id": tool.ID, "type": "function", "function": map[string]any{"name": tool.Name, "arguments": ""}}}}, nil, nil); err != nil {
					return err
				}
			}
			if err := splitText(arguments, func(part string) error {
				call := map[string]any{"index": index, "function": map[string]any{"arguments": part}}
				if newTool && first {
					call["id"], call["type"] = ""+tool.ID, "function"
					call["function"].(map[string]any)["name"] = tool.Name
				}
				first = false
				return renderer.event(map[string]any{"tool_calls": []any{call}}, nil, nil)
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func CanonicalUsage(summary codec.NativeSummary) map[string]any {
	if summary.Usage == nil {
		return nil
	}
	value := summary.Usage
	result := map[string]any{}
	if value.Measured(codec.OutputCount) {
		result["completion_tokens"] = value.Output
	}
	if value.Measured(codec.InputCount | codec.CacheReadCount | codec.CacheWriteCount) {
		prompt := value.Input + value.CacheRead + value.CacheWrite
		result["prompt_tokens"] = prompt
		if value.Measured(codec.OutputCount) {
			result["total_tokens"] = prompt + value.Output
		}
	}
	details := map[string]uint64{}
	if value.Measured(codec.CacheReadCount) {
		details["cached_tokens"] = value.CacheRead
	}
	if value.Measured(codec.CacheWriteCount) {
		details["cache_write_tokens"] = value.CacheWrite
	}
	if len(details) != 0 {
		result["prompt_tokens_details"] = details
	}
	return result
}

func (renderer *ChatRenderer) Validate(summary codec.NativeSummary) error {
	if renderer.finished {
		return errors.New("canonical response is finished")
	}
	if summary.Stop != 2 && summary.Stop != 3 && summary.Stop != 10 {
		return errors.New("native stop reason requires the paired native route")
	}
	for _, tool := range renderer.tools {
		var arguments map[string]json.RawMessage
		if json.Unmarshal([]byte(tool.Args.String()), &arguments) != nil || arguments == nil {
			return errors.New("native tool arguments are incomplete")
		}
	}
	if summary.Stop == 10 && len(renderer.tools) == 0 {
		return errors.New("native tool finish has no calls")
	}
	return nil
}

func (renderer *ChatRenderer) Finish(summary codec.NativeSummary) error {
	if err := renderer.Validate(summary); err != nil {
		return err
	}
	finish := ""
	switch summary.Stop {
	case 2:
		finish = "stop"
		if len(renderer.tools) > 0 {
			finish = "tool_calls"
		}
	case 3:
		finish = "length"
	case 10:
		finish = "tool_calls"
	default:
		return errors.New("native stop reason requires the paired native route")
	}
	if renderer.stream {
		if err := renderer.event(map[string]any{}, &finish, CanonicalUsage(summary)); err != nil {
			return err
		}
		if _, err := renderer.writer.Write([]byte("data: [DONE]\n\n")); err != nil {
			return err
		}
		if flusher, ok := renderer.writer.(http.Flusher); ok {
			flusher.Flush()
		}
	} else {
		message := map[string]any{"role": "assistant", "content": renderer.text.String()}
		if renderer.thinking.Len() != 0 {
			message["reasoning_content"] = renderer.thinking.String()
		}
		var details []any
		for index, thought := range renderer.thoughts {
			if thought.signature.Len() != 0 {
				details = append(details, map[string]any{"type": "reasoning.text", "text": thought.text.String(), "signature": thought.signature.String(), "format": thought.format, "index": index})
			}
		}
		if len(details) != 0 {
			message["reasoning_details"] = details
		}
		if len(renderer.tools) != 0 {
			var calls []any
			for _, tool := range renderer.tools {
				calls = append(calls, map[string]any{"id": tool.ID, "type": "function", "function": map[string]string{"name": tool.Name, "arguments": tool.Args.String()}})
			}
			message["tool_calls"] = calls
		}
		value := map[string]any{"id": renderer.id, "object": "chat.completion", "model": renderer.model,
			"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}}
		if usage := CanonicalUsage(summary); usage != nil {
			value["usage"] = usage
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if len(data) > 16<<20 {
			return errors.New("canonical response exceeds its encoded limit")
		}
		renderer.start()
		if _, err := renderer.writer.Write(data); err != nil {
			return err
		}
	}
	renderer.finished = true
	return nil
}

func (renderer *ChatRenderer) Fail(status int) error {
	if renderer.finished {
		return nil
	}
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	if !renderer.started {
		renderer.writer.Header().Set("Content-Type", "application/json")
		renderer.writer.WriteHeader(status)
		_, err := fmt.Fprintf(renderer.writer, `{"error":{"message":"Devin native request failed","type":"devin_error","code":%d}}`, status)
		renderer.finished = true
		return err
	}
	if renderer.stream {
		_, err := fmt.Fprintf(renderer.writer, "data: {\"error\":{\"message\":\"Devin native request failed\",\"code\":%d}}\n\n", status)
		renderer.finished = true
		return err
	}
	return errors.New("canonical response failed after headers")
}
