package outbound

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type messagesOutput struct {
	kind      string
	id        string
	name      string
	text      strings.Builder
	signature strings.Builder
	format    string
}

type MessagesRenderer struct {
	id              string
	model           string
	writer          http.ResponseWriter
	stream          bool
	started         bool
	finished        bool
	blocks          []*messagesOutput
	thoughts        map[string]int
	tools           map[string]int
	lastTool        int
	buffered        int
	geminiSignature bool
	geminiFormat    string
}

func NewMessagesRenderer(writer http.ResponseWriter, model string, streaming bool) *MessagesRenderer {
	return &MessagesRenderer{id: "msg_devin_" + rand.Text(), model: model, writer: writer, stream: streaming,
		thoughts: map[string]int{}, tools: map[string]int{}, lastTool: -1}
}

func (renderer *MessagesRenderer) ResponseID() string { return renderer.id }

func MessagesUsage(summary codec.NativeSummary) map[string]uint64 {
	if summary.Usage == nil {
		return nil
	}
	value := summary.Usage
	result := map[string]uint64{}
	for _, field := range []struct {
		bit   uint8
		name  string
		value uint64
	}{
		{codec.InputCount, "input_tokens", value.Input}, {codec.OutputCount, "output_tokens", value.Output},
		{codec.CacheReadCount, "cache_read_input_tokens", value.CacheRead}, {codec.CacheWriteCount, "cache_creation_input_tokens", value.CacheWrite},
	} {
		if value.Measured(field.bit) {
			result[field.name] = field.value
		}
	}
	return result
}

func (renderer *MessagesRenderer) event(kind string, value map[string]any) error {
	value["type"] = kind
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(renderer.writer, "event: %s\ndata: %s\n\n", kind, data); err != nil {
		return err
	}
	if flusher, ok := renderer.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (renderer *MessagesRenderer) start() error {
	if renderer.started {
		return nil
	}
	renderer.started = true
	renderer.writer.Header().Set("Content-Type", "application/json")
	if renderer.stream {
		renderer.writer.Header().Set("Content-Type", "text/event-stream")
	}
	renderer.writer.WriteHeader(http.StatusOK)
	if renderer.stream {
		return renderer.event("message_start", map[string]any{"message": map[string]any{
			"id": renderer.id, "type": "message", "role": "assistant", "model": renderer.model, "content": []any{},
			"stop_reason": nil, "stop_sequence": nil, "usage": map[string]uint64{"input_tokens": 0, "output_tokens": 0}}})
	}
	return nil
}

func (renderer *MessagesRenderer) block(kind, id, name string) (int, error) {
	if len(renderer.blocks) >= 256 {
		return 0, errors.New("Messages response exceeds its block limit")
	}
	index := len(renderer.blocks)
	renderer.blocks = append(renderer.blocks, &messagesOutput{kind: kind, id: id, name: name})
	if !renderer.stream {
		return index, nil
	}
	if err := renderer.start(); err != nil {
		return 0, err
	}
	block := map[string]any{"type": kind}
	switch kind {
	case "text":
		block["text"] = ""
	case "thinking":
		block["thinking"], block["signature"] = "", ""
	case "tool_use":
		block["id"], block["name"], block["input"] = codec.CanonicalToolID(id), name, map[string]any{}
	}
	return index, renderer.event("content_block_start", map[string]any{"index": index, "content_block": block})
}

func (renderer *MessagesRenderer) part(index int, kind, field, value string) error {
	if !renderer.stream {
		return nil
	}
	return splitText(value, func(part string) error {
		return renderer.event("content_block_delta", map[string]any{"index": index, "delta": map[string]string{
			"type": kind, field: part}})
	})
}

func (renderer *MessagesRenderer) Delta(delta codec.NativeDelta) error {
	if renderer.finished || delta.Phase != "" || len(delta.SignatureType) > 256 {
		return errors.New("unsupported native Messages delta")
	}
	renderer.buffered += len(delta.Text) + len(delta.Thinking) + len(delta.Signature)
	for _, change := range delta.Tools {
		renderer.buffered += len(change.ID) + len(change.Name) + len(change.Arguments)
	}
	if renderer.buffered > 2<<20 {
		return errors.New("Messages response exceeds its buffer limit")
	}
	if delta.Text != "" && (delta.Thinking != "" || len(delta.Tools) > 0) ||
		delta.Thinking != "" && len(delta.Tools) > 0 {
		return errors.New("native Messages delta has ambiguous block order")
	}
	if delta.Thinking != "" {
		index, found := renderer.thoughts[delta.ThinkingID]
		if !found {
			var err error
			index, err = renderer.block("thinking", delta.ThinkingID, "")
			if err != nil {
				return err
			}
			renderer.thoughts[delta.ThinkingID] = index
		} else if index != len(renderer.blocks)-1 {
			return errors.New("native thinking resumed after a different block")
		}
		renderer.blocks[index].text.WriteString(delta.Thinking)
		if err := renderer.part(index, "thinking_delta", "thinking", delta.Thinking); err != nil {
			return err
		}
	}
	if delta.Signature != "" || delta.SignatureType != "" {
		if codec.IsGeminiModel(renderer.model) {
			if delta.SignatureType != "" && delta.SignatureType != "gemini" {
				return errors.New("native Gemini signature type changed")
			}
			renderer.geminiSignature = renderer.geminiSignature || delta.Signature != ""
			if delta.SignatureType != "" {
				renderer.geminiFormat = delta.SignatureType
			}
		} else {
			index, found := renderer.thoughts[delta.ThinkingID]
			if !found || !strings.Contains(strings.ToLower(renderer.model), "claude") {
				return errors.New("native signature has no matching Claude thinking block")
			}
			block := renderer.blocks[index]
			if delta.SignatureType != "" {
				if block.format != "" && block.format != delta.SignatureType {
					return errors.New("native thinking signature format changed")
				}
				block.format = delta.SignatureType
			}
			block.signature.WriteString(delta.Signature)
			if err := renderer.part(index, "signature_delta", "signature", delta.Signature); err != nil {
				return err
			}
		}
	}
	if delta.Text != "" {
		index := len(renderer.blocks) - 1
		if index < 0 || renderer.blocks[index].kind != "text" {
			var err error
			index, err = renderer.block("text", "", "")
			if err != nil {
				return err
			}
		}
		renderer.blocks[index].text.WriteString(delta.Text)
		if err := renderer.part(index, "text_delta", "text", delta.Text); err != nil {
			return err
		}
	}
	for _, change := range delta.Tools {
		index, found := renderer.tools[change.ID]
		if change.ID == "" {
			index, found = renderer.lastTool, renderer.lastTool >= 0
		}
		if !found {
			if change.ID == "" || change.Name == "" {
				return errors.New("native Messages tool has no identity")
			}
			var err error
			index, err = renderer.block("tool_use", change.ID, change.Name)
			if err != nil {
				return err
			}
			renderer.tools[change.ID] = index
		}
		renderer.lastTool = index
		block := renderer.blocks[index]
		if change.Name != "" && change.Name != block.name {
			return errors.New("native Messages tool name changed")
		}
		block.text.WriteString(change.Arguments)
		if err := renderer.part(index, "input_json_delta", "partial_json", change.Arguments); err != nil {
			return err
		}
	}
	return nil
}

func (renderer *MessagesRenderer) Validate(summary codec.NativeSummary) error {
	if renderer.finished || len(renderer.blocks) == 0 {
		return errors.New("Messages response has no content")
	}
	if summary.Stop != 2 && summary.Stop != 3 && summary.Stop != 10 {
		return errors.New("native stop reason has no Messages mapping")
	}
	if summary.Stop == 10 && len(renderer.tools) == 0 {
		return errors.New("native Messages stop conflicts with its tools")
	}
	if renderer.geminiSignature != (renderer.geminiFormat == "gemini") {
		return errors.New("native Gemini signature is incomplete")
	}
	for _, block := range renderer.blocks {
		if block.kind == "tool_use" {
			var arguments map[string]json.RawMessage
			if json.Unmarshal([]byte(block.text.String()), &arguments) != nil || arguments == nil {
				return errors.New("native Messages tool arguments are incomplete")
			}
		}
		if block.kind == "thinking" && (block.signature.Len() == 0) != (block.format == "") {
			return errors.New("native Messages thinking signature is incomplete")
		}
	}
	return nil
}

func (renderer *MessagesRenderer) Finish(summary codec.NativeSummary) error {
	if err := renderer.Validate(summary); err != nil {
		return err
	}
	stop := map[uint64]string{2: "end_turn", 3: "max_tokens", 10: "tool_use"}[summary.Stop]
	if summary.Stop == 2 && len(renderer.tools) > 0 {
		stop = "tool_use"
	}
	if err := renderer.start(); err != nil {
		return err
	}
	if renderer.stream {
		for index := range renderer.blocks {
			if err := renderer.event("content_block_stop", map[string]any{"index": index}); err != nil {
				return err
			}
		}
		value := map[string]any{"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}}
		if usage := MessagesUsage(summary); usage != nil {
			value["usage"] = usage
		}
		if err := renderer.event("message_delta", value); err != nil {
			return err
		}
		if err := renderer.event("message_stop", map[string]any{}); err != nil {
			return err
		}
	} else {
		content := []any{}
		for _, block := range renderer.blocks {
			value := map[string]any{"type": block.kind}
			switch block.kind {
			case "text":
				value["text"] = block.text.String()
			case "thinking":
				value["thinking"], value["signature"] = block.text.String(), block.signature.String()
			case "tool_use":
				value["id"], value["name"], value["input"] = codec.CanonicalToolID(block.id), block.name, json.RawMessage(block.text.String())
			}
			content = append(content, value)
		}
		value := map[string]any{"id": renderer.id, "type": "message", "role": "assistant", "model": renderer.model,
			"content": content, "stop_reason": stop, "stop_sequence": nil}
		if usage := MessagesUsage(summary); usage != nil {
			value["usage"] = usage
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := renderer.writer.Write(data); err != nil {
			return err
		}
	}
	renderer.finished = true
	return nil
}

func (renderer *MessagesRenderer) Fail(status int) error {
	if renderer.finished {
		return nil
	}
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	renderer.finished = true
	kind := map[int]string{400: "invalid_request_error", 401: "authentication_error", 403: "permission_error",
		429: "rate_limit_error"}[status]
	if kind == "" {
		kind = "api_error"
	}
	failure := map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": "Devin native Messages request failed"}}
	if renderer.started && renderer.stream {
		return renderer.event("error", failure)
	}
	if renderer.started {
		return errors.New("Messages response failed after headers")
	}
	renderer.writer.Header().Set("Content-Type", "application/json")
	renderer.writer.WriteHeader(status)
	return json.NewEncoder(renderer.writer).Encode(failure)
}
