package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/yetone/magpie/pkg/protocolext"
)

const (
	maxEvent              = 1 << 20
	stopReasonStopPattern = 2
)

type toolState struct {
	id         string
	name, args strings.Builder
}

type response struct {
	w                                        http.ResponseWriter
	header                                   http.Header
	status                                   int
	started, finished, done, stopped, skipLF bool
	err                                      error
	code                                     string
	line, data, errorBody                    []byte
	event, format                            string
	eventSize, toolBytes                     int
	tools                                    map[int]*toolState
	order                                    []int
}

var _ protocolext.Response = (*response)(nil)
var _ http.Flusher = (*response)(nil)

func (Adapter) Reply(w http.ResponseWriter) protocolext.Response {
	return &response{w: w, header: make(http.Header), tools: make(map[int]*toolState)}
}

func (r *response) Header() http.Header         { return r.header }
func (r *response) Unwrap() http.ResponseWriter { return r.w }
func (r *response) Flush() {
	if r.started {
		if f, ok := r.w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
func (r *response) WriteHeader(status int) {
	if r.status == 0 && status >= 200 && !r.finished {
		r.status = status
	}
}

func (r *response) fail(code, message string) error {
	if r.err == nil {
		r.err, r.code = errors.New(message), code
	}
	return r.err
}

func (r *response) Write(p []byte) (int, error) {
	if r.finished {
		return 0, io.ErrClosedPipe
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.status < 200 || r.status >= 300 {
		if len(p) > (64<<10)-len(r.errorBody) {
			return 0, r.fail("internal", "gateway error body exceeds limit")
		}
		r.errorBody = append(r.errorBody, p...)
		return len(p), nil
	}
	if r.format == "" {
		r.format = "text/event-stream"
		if ct := r.header.Get("Content-Type"); ct != "" {
			var err error
			r.format, _, err = mime.ParseMediaType(ct)
			if err != nil || (r.format != "text/event-stream" && r.format != "application/json") {
				return 0, r.fail("internal", "expected canonical Chat SSE or JSON")
			}
		}
	}
	if r.format == "application/json" {
		if len(p) > maxBody-len(r.data) {
			return 0, r.fail("resource_exhausted", "canonical Chat JSON exceeds limit")
		}
		r.data = append(r.data, p...)
		return len(p), nil
	}
	for i, c := range p {
		if r.skipLF {
			r.skipLF = false
			if c == '\n' {
				continue
			}
		}
		if c == '\n' || c == '\r' {
			r.skipLF = c == '\r'
			if err := r.consumeLine(); err != nil {
				return i + 1, err
			}
			continue
		}
		r.eventSize++
		if r.eventSize > maxEvent {
			return i, r.fail("resource_exhausted", "canonical SSE event exceeds limit")
		}
		r.line = append(r.line, c)
	}
	return len(p), nil
}

func (r *response) consumeLine() error {
	line := r.line
	r.line = nil
	if len(line) == 0 {
		data, event := r.data, r.event
		r.data, r.event, r.eventSize = nil, "", 0
		if len(data) == 0 {
			return nil
		}
		if event != "" && event != "message" && event != "error" {
			return r.fail("unimplemented", "unsupported canonical SSE event")
		}
		return r.consumeEvent(bytes.TrimSuffix(data, []byte{'\n'}))
	}
	if line[0] == ':' {
		return nil
	}
	key, value, _ := bytes.Cut(line, []byte{':'})
	value = bytes.TrimPrefix(value, []byte{' '})
	switch string(key) {
	case "data":
		r.data = append(r.data, value...)
		r.data = append(r.data, '\n')
	case "event":
		r.event = string(value)
	}
	return nil
}

type chatChunk struct {
	ID                string          `json:"id"`
	Object            string          `json:"object"`
	Created           json.RawMessage `json:"created"`
	Model             string          `json:"model"`
	SystemFingerprint json.RawMessage `json:"system_fingerprint"`
	ServiceTier       json.RawMessage `json:"service_tier"`
	Obfuscation       json.RawMessage `json:"obfuscation"`
	Choices           []struct {
		Index    *int            `json:"index"`
		Delta    json.RawMessage `json:"delta"`
		Message  json.RawMessage `json:"message"`
		Finish   *string         `json:"finish_reason"`
		Logprobs json.RawMessage `json:"logprobs"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
	Error json.RawMessage `json:"error"`
}

func (r *response) consumeEvent(data []byte) error {
	if r.done {
		return r.fail("data_loss", "canonical data after DONE")
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		if !r.stopped {
			return r.fail("data_loss", "canonical DONE without finish reason")
		}
		r.done = true
		return nil
	}
	return r.consumeChat(data, false)
}

func (r *response) consumeChat(data []byte, whole bool) error {
	if !utf8.Valid(data) {
		return r.fail("data_loss", "invalid canonical Chat UTF-8")
	}
	var c chatChunk
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !jsonObject(data) || decoder.Decode(&c) != nil {
		return r.fail("data_loss", "malformed canonical Chat response")
	}
	if len(c.Error) > 0 && string(c.Error) != "null" {
		return r.fail("internal", publicError(c.Error, "gateway stream failed"))
	}
	if len(c.Choices) > 1 {
		return r.fail("unimplemented", "multiple Chat choices are unsupported")
	}
	if whole && len(c.Choices) != 1 {
		return r.fail("data_loss", "canonical Chat JSON requires one choice")
	}
	var out []byte
	out = addText(out, 1, c.ID)
	for _, choice := range c.Choices {
		if choice.Index == nil || *choice.Index != 0 || r.stopped {
			return r.fail("data_loss", "invalid Chat choice or data after finish")
		}
		payload := choice.Delta
		if whole {
			if len(choice.Delta) != 0 || choice.Finish == nil {
				return r.fail("data_loss", "canonical Chat JSON requires a message and finish reason")
			}
			payload = choice.Message
		} else if len(choice.Message) != 0 {
			return r.fail("data_loss", "unexpected Chat message in SSE")
		}
		var delta map[string]json.RawMessage
		if json.Unmarshal(payload, &delta) != nil || delta == nil {
			return r.fail("data_loss", "invalid canonical Chat content")
		}
		if whole {
			role, err := rawString(delta, "role")
			if err != nil || role != "assistant" {
				return r.fail("data_loss", "invalid Chat message role")
			}
		}
		if len(choice.Logprobs) > 0 && string(choice.Logprobs) != "null" {
			return r.fail("unimplemented", "Chat logprobs are unsupported")
		}
		var err error
		out, err = r.delta(out, delta, whole)
		if err != nil {
			return err
		}
		if choice.Finish != nil {
			out, err = r.stop(out, *choice.Finish)
			if err != nil {
				return err
			}
			r.stopped = true
		}
	}
	if len(c.Usage) > 0 && string(c.Usage) != "null" {
		usage, err := encodeUsage(c.Usage)
		if err != nil {
			return r.fail("data_loss", "invalid canonical token usage")
		}
		out = addBytes(out, 7, usage)
	}
	if len(c.Choices) == 0 && (len(c.Usage) == 0 || string(c.Usage) == "null") {
		return r.fail("data_loss", "canonical event has no choice or usage")
	}
	if len(out) > 0 {
		if err := r.frame(0, out); err != nil {
			return err
		}
	}
	if whole {
		r.done = true
	}
	return nil
}

func rawString(m map[string]json.RawMessage, key string) (string, error) {
	b := m[key]
	if len(b) == 0 || string(b) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return "", err
	}
	return s, nil
}

func (r *response) delta(out []byte, d map[string]json.RawMessage, whole bool) ([]byte, error) {
	for key := range d {
		switch key {
		case "role", "content", "reasoning_content", "reasoning", "reasoning_details", "tool_calls", "signature", "signature_type", "reasoning_signature", "reasoning_signature_type":
		case "refusal":
			if !whole {
				return nil, r.fail("unimplemented", "unsupported canonical Chat delta field")
			}
			refusal, err := rawString(d, key)
			if err != nil {
				return nil, r.fail("data_loss", "invalid Chat refusal")
			}
			if refusal != "" {
				return nil, r.fail("unimplemented", "Chat refusals are unsupported")
			}
		default:
			return nil, r.fail("unimplemented", "unsupported canonical Chat delta field")
		}
	}
	role, err := rawString(d, "role")
	if err != nil || (role != "" && role != "assistant") {
		return nil, r.fail("data_loss", "invalid Chat delta role")
	}
	for key, n := range map[string]int{"content": 3, "signature": 10, "signature_type": 21, "reasoning_signature": 10, "reasoning_signature_type": 21} {
		s, err := rawString(d, key)
		if err != nil {
			return nil, r.fail("data_loss", "invalid Chat delta string")
		}
		// Refuse ambiguous aliases rather than emit a signature twice.
		if key == "reasoning_signature" && len(d["signature"]) > 0 && s != "" {
			return nil, r.fail("data_loss", "duplicate signature aliases")
		}
		if key == "reasoning_signature_type" && len(d["signature_type"]) > 0 && s != "" {
			return nil, r.fail("data_loss", "duplicate signature type aliases")
		}
		out = addText(out, n, s)
	}
	think, err := rawString(d, "reasoning_content")
	if err != nil {
		return nil, r.fail("data_loss", "invalid Chat reasoning")
	}
	alias, err := rawString(d, "reasoning")
	if err != nil || (alias != "" && think != "" && think != alias) {
		return nil, r.fail("data_loss", "ambiguous Chat reasoning")
	}
	if think == "" {
		think = alias
	}
	var details []struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		Signature string `json:"signature"`
		Data      string `json:"data"`
		Format    string `json:"format"`
		Index     int    `json:"index"`
	}
	if b := d["reasoning_details"]; len(b) > 0 && string(b) != "null" {
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&details) != nil {
			return nil, r.fail("unimplemented", "unsupported reasoning details")
		}
	}
	var detailedText strings.Builder
	for _, v := range details {
		if v.Index != 0 {
			return nil, r.fail("unimplemented", "multiple reasoning blocks are unsupported")
		}
		switch v.Type {
		case "reasoning.text":
			if v.Data != "" {
				return nil, r.fail("unimplemented", "unsupported reasoning data")
			}
			detailedText.WriteString(v.Text)
			if v.Signature != "" && (len(d["signature"]) > 0 || len(d["reasoning_signature"]) > 0) {
				return nil, r.fail("data_loss", "ambiguous thinking signatures")
			}
			out = addText(out, 10, v.Signature)
		case "reasoning.encrypted":
			// A sealed reasoning item has no established Devin signature
			// provenance. Never relabel its opaque bytes as delta_signature.
			return nil, r.fail("unimplemented", "encrypted reasoning has no supported Devin signature mapping")
		default:
			return nil, r.fail("unimplemented", "unsupported reasoning detail type")
		}
		out = addText(out, 21, v.Format)
	}
	if detailedText.Len() > 0 {
		if think != "" && think != detailedText.String() {
			return nil, r.fail("data_loss", "ambiguous reasoning details")
		}
		think = detailedText.String()
	}
	out = addText(out, 9, think)
	if b := d["tool_calls"]; len(b) > 0 && string(b) != "null" {
		var calls []struct {
			Index    *int   `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&calls) != nil {
			return nil, r.fail("data_loss", "invalid Chat tool delta")
		}
		for index, c := range calls {
			if whole {
				if c.Index != nil || c.Type != "function" {
					return nil, r.fail("unimplemented", "unsupported Chat tool call")
				}
				c.Index = &index
			}
			if c.Index == nil || *c.Index < 0 || *c.Index >= 128 || (c.Type != "" && c.Type != "function") {
				return nil, r.fail("unimplemented", "unsupported tool delta")
			}
			t := r.tools[*c.Index]
			if t == nil {
				t = &toolState{}
				r.tools[*c.Index] = t
				r.order = append(r.order, *c.Index)
			}
			if c.ID != "" {
				if t.id != "" && t.id != c.ID {
					return nil, r.fail("data_loss", "tool id changed during stream")
				}
				if t.id == "" {
					r.toolBytes += len(c.ID)
					t.id = c.ID
				}
			}
			r.toolBytes += len(c.Function.Name) + len(c.Function.Arguments)
			if r.toolBytes > maxEvent {
				return nil, r.fail("resource_exhausted", "canonical tool arguments exceed limit")
			}
			t.name.WriteString(c.Function.Name)
			t.args.WriteString(c.Function.Arguments)
		}
	}
	return out, nil
}

func (r *response) stop(out []byte, reason string) ([]byte, error) {
	switch reason {
	case "stop":
		if len(r.tools) > 0 {
			return nil, r.fail("data_loss", "tool calls without tool finish reason")
		}
		out = addNumber(out, 5, stopReasonStopPattern)
	case "length":
		out = addNumber(out, 5, 3)
	case "tool_calls":
		if len(r.tools) == 0 {
			return nil, r.fail("data_loss", "tool finish reason without tool calls")
		}
		out = addNumber(out, 5, 10)
	case "content_filter":
		return nil, r.fail("permission_denied", "completion blocked by content filter")
	default:
		return nil, r.fail("unimplemented", "unsupported Chat finish reason")
	}
	ids := map[string]bool{}
	for _, index := range r.order {
		t := r.tools[index]
		if t.id == "" || t.name.Len() == 0 || !jsonObject([]byte(t.args.String())) || ids[t.id] {
			return nil, r.fail("data_loss", "incomplete or invalid Chat tool call")
		}
		ids[t.id] = true
		call := addText(nil, 1, t.id)
		call = addText(call, 2, t.name.String())
		call = addText(call, 3, t.args.String())
		out = addBytes(out, 6, call)
	}
	return out, nil
}

func encodeUsage(b []byte) ([]byte, error) {
	// The installed plugin's events() maps native fields 2/4/5 to
	// prompt_tokens = input + cacheWrite + cacheRead; v2Usage() independently
	// subtracts those details for noCache. Reverse precisely that contract,
	// rejecting inconsistent counters instead of clamping or guessing.
	var u struct {
		Prompt  int64 `json:"prompt_tokens"`
		Output  int64 `json:"completion_tokens"`
		Details struct {
			Read  int64 `json:"cached_tokens"`
			Write int64 `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if !jsonObject(b) || json.Unmarshal(b, &u) != nil {
		return nil, fmt.Errorf("invalid usage")
	}
	if u.Prompt < 0 || u.Output < 0 || u.Details.Read < 0 || u.Details.Write < 0 || u.Details.Read > u.Prompt || u.Details.Write > u.Prompt-u.Details.Read {
		return nil, fmt.Errorf("invalid usage counters")
	}
	values := []int64{u.Prompt - u.Details.Read - u.Details.Write, u.Output, u.Details.Write, u.Details.Read}
	var out []byte
	for i, v := range values {
		if v > math.MaxInt32 {
			return nil, fmt.Errorf("usage counter overflow")
		}
		out = addNumber(out, i+2, uint64(v))
	}
	return out, nil
}

func (r *response) start() {
	if r.started {
		return
	}
	h := r.w.Header()
	h.Del("Content-Length")
	h.Del("Content-Encoding")
	h.Set("Content-Type", "application/connect+proto")
	h.Set("Connect-Protocol-Version", "1")
	h.Set("Cache-Control", "no-store")
	for _, key := range []string{"Retry-After", "X-Magpie-Session"} {
		if values := r.header.Values(key); len(values) > 0 {
			h[key] = append([]string(nil), values...)
		}
	}
	// Connect Streaming-Response-Headers require HTTP 200 even for errors.
	// Keep r.status intact for canonical accounting and terminal code mapping.
	r.w.WriteHeader(http.StatusOK)
	r.started = true
}

func (r *response) frame(flags byte, b []byte) error {
	r.start()
	var head [5]byte
	head[0] = flags
	binary.BigEndian.PutUint32(head[1:], uint32(len(b)))
	for _, part := range [][]byte{head[:], b} {
		n, err := r.w.Write(part)
		if err == nil && n != len(part) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return r.fail("unavailable", "native response write failed")
		}
	}
	r.Flush()
	return nil
}

func publicError(b []byte, fallback string) string {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &e) == nil && e.Message != "" && len(e.Message) <= 1024 {
		return e.Message
	}
	return fallback
}

func statusCode(status int) string {
	switch status {
	case 400, 415, 422:
		return "invalid_argument"
	case 401:
		return "unauthenticated"
	case 403:
		return "permission_denied"
	case 404, 501:
		return "unimplemented"
	case 408, 504:
		return "deadline_exceeded"
	case 409:
		return "aborted"
	case 413, 429:
		return "resource_exhausted"
	case 499:
		return "canceled"
	case 502, 503:
		return "unavailable"
	default:
		return "internal"
	}
}

func (r *response) Finish(cause error) error {
	if r.finished {
		return r.err
	}
	r.finished = true
	if r.err == nil && r.status >= 300 {
		var envelope struct {
			Error json.RawMessage `json:"error"`
		}
		message := "gateway request failed"
		if json.Unmarshal(r.errorBody, &envelope) == nil {
			message = publicError(envelope.Error, message)
		}
		r.fail(statusCode(r.status), message)
	}
	if r.err == nil && r.format == "application/json" {
		data := r.data
		r.data = nil
		if cause == nil || json.Valid(data) {
			r.consumeChat(data, true)
		}
	}
	if r.err == nil && (!r.done || !r.stopped) && cause != nil {
		switch {
		case errors.Is(cause, context.DeadlineExceeded):
			r.fail("deadline_exceeded", "canonical request deadline exceeded")
		case errors.Is(cause, context.Canceled):
			r.fail("canceled", "canonical request canceled")
		default:
			r.fail("internal", "canonical request failed")
		}
	}
	if r.err == nil && (!r.done || !r.stopped || len(r.line) != 0 || len(r.data) != 0) {
		r.fail("data_loss", "canonical Chat stream ended before explicit DONE")
	}
	trailer := map[string]any{"metadata": map[string]any{}}
	if r.err != nil {
		trailer["error"] = map[string]string{"code": r.code, "message": r.err.Error()}
	}
	b, _ := json.Marshal(trailer)
	if err := r.frame(2, b); err != nil {
		return err
	}
	return r.err
}
