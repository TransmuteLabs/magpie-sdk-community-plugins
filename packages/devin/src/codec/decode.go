// Package devin implements a deliberately limited experimental inbound
// GetChatMessage codec. It neither authenticates nor contacts Devin.
package devin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/yetone/magpie/pkg/protocolext"
)

const Path = "/exa.api_server_pb.ApiServerService/GetChatMessage"

type Adapter struct{}

var _ protocolext.Inbound = Adapter{}

func (Adapter) ID() string                     { return "devin" }
func (Adapter) Path() string                   { return Path }
func (Adapter) Contract() protocolext.Contract { return protocolext.InboundContract() }

func refusal(status int, message string) error {
	return &protocolext.Error{Status: status, Message: message}
}

func (Adapter) Decode(ctx context.Context, h http.Header, body []byte) (protocolext.Request, error) {
	if err := ctx.Err(); err != nil {
		return protocolext.Request{}, err
	}
	contentTypes := h.Values("Content-Type")
	mt, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if len(contentTypes) != 1 || err != nil || mt != "application/connect+proto" {
		return protocolext.Request{}, refusal(415, "expected application/connect+proto")
	}
	versions := h.Values("Connect-Protocol-Version")
	if len(versions) != 1 || versions[0] != "1" {
		return protocolext.Request{}, refusal(400, "expected Connect-Protocol-Version 1")
	}
	encodings := h.Values("Connect-Content-Encoding")
	encoding := strings.ToLower(strings.TrimSpace(h.Get("Connect-Content-Encoding")))
	if len(encodings) > 1 || encoding != "" && encoding != "identity" && encoding != "gzip" {
		return protocolext.Request{}, refusal(415, "unsupported Connect compression")
	}
	if values := h.Values("Content-Encoding"); len(values) > 1 || len(values) == 1 && values[0] != "" && !strings.EqualFold(values[0], "identity") {
		return protocolext.Request{}, refusal(415, "unsupported HTTP content encoding")
	}
	timeout, err := decodeTimeout(h)
	if err != nil {
		return protocolext.Request{}, err
	}
	if len(body) > 0 && body[0]&1 != 0 && encoding != "gzip" {
		return protocolext.Request{}, refusal(400, "compressed Connect frame requires gzip encoding")
	}
	b, err := unframe(body)
	if err != nil {
		return protocolext.Request{}, refusal(400, err.Error())
	}
	c, session, err := decodeRequest(b)
	if err != nil {
		return protocolext.Request{}, refusal(400, "unsupported or malformed Devin request: "+err.Error())
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return protocolext.Request{}, refusal(400, "invalid Devin conversation")
	}
	if len(encoded) > maxBody {
		return protocolext.Request{}, refusal(413, "translated conversation exceeds limit")
	}
	return protocolext.Request{Protocol: protocolext.Chat, Body: encoded, Session: session, Timeout: timeout}, nil
}

func decodeTimeout(h http.Header) (time.Duration, error) {
	values := h.Values("Connect-Timeout-Ms")
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 10 {
		return 0, refusal(400, "invalid Connect-Timeout-Ms")
	}
	var ms uint64
	for _, c := range values[0] {
		if c < '0' || c > '9' {
			return 0, refusal(400, "invalid Connect-Timeout-Ms")
		}
		ms = ms*10 + uint64(c-'0')
	}
	if ms == 0 {
		return 0, refusal(504, "Connect request deadline exceeded")
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func decodeRequest(b []byte) (map[string]any, string, error) {
	m, err := readMessage(b, map[int]int{1: 2, 2: textField, 3: 2 | repeated, 5: 0, 6: 0, 7: 0, 8: 2, 10: 2 | repeated, 11: 0, 12: 2, 13: 2, 14: textField, 15: 2, 16: textField, 17: textField, 18: 0, 19: 0, 20: 0, 21: textField, 22: textField, 27: textField, 28: textField})
	if err != nil {
		return nil, "", err
	}
	// Metadata can contain auth tokens. Validate only its protobuf envelope;
	// never copy it, log it, or include its contents in public errors.
	if _, err := readMessage(m.data(1), nil); err != nil {
		return nil, "", fmt.Errorf("invalid metadata envelope")
	}
	if m.number(5) > 1 || m.number(11) > 1 {
		return nil, "", fmt.Errorf("invalid boolean")
	}
	if n := m.number(7); n != 0 && n != 5 {
		return nil, "", fmt.Errorf("unsupported request type")
	}
	if m.number(18) != 0 || m.number(19) != 0 {
		return nil, "", fmt.Errorf("native provider-source or language selection is unsupported")
	}
	if mode := m.number(20); mode != 0 && mode != 1 && mode != 3 {
		return nil, "", fmt.Errorf("native planner mode requires an unsupported tool policy")
	}
	model := m.text(21) // encodeRequest uses chat_model_uid, not the legacy enum.
	if model == "" {
		model = m.text(14)
	}
	if strings.TrimSpace(model) == "" || strings.TrimSpace(model) != model || strings.ContainsAny(model, "\r\n\x00") {
		return nil, "", fmt.Errorf("explicit model name or UID required; enum-only models are unsupported")
	}
	if len(m[3]) > 2048 || len(m[10]) > 256 {
		return nil, "", fmt.Errorf("too many prompts or tools")
	}
	var msgs []map[string]any
	if prompt := m.text(2); prompt != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": prompt})
	}
	if len(m[13]) > 0 {
		cache, err := cacheOptions(m.data(13))
		if err != nil {
			return nil, "", err
		}
		if len(msgs) == 0 {
			return nil, "", fmt.Errorf("system cache options without system prompt")
		}
		if cache != nil {
			msgs[0]["cache_control"] = cache
		}
	}
	// Validate call/result associations instead of manufacturing missing results.
	pending, seen := map[string]bool{}, map[string]bool{}
	conversation := false
	for _, f := range m[3] {
		msg, err := decodePrompt(f.b)
		if err != nil {
			return nil, "", err
		}
		role := msg["role"].(string)
		if role == "system" {
			if conversation {
				return nil, "", fmt.Errorf("native system prompts must precede the conversation")
			}
		} else {
			conversation = true
		}
		if role == "tool" {
			id := msg["tool_call_id"].(string)
			if !pending[id] {
				return nil, "", fmt.Errorf("orphan or duplicate tool result")
			}
			delete(pending, id)
		} else if len(pending) > 0 && role != "assistant" {
			return nil, "", fmt.Errorf("unanswered tool calls")
		}
		if calls, ok := msg["tool_calls"].([]map[string]any); ok {
			for _, c := range calls {
				id := c["id"].(string)
				if seen[id] {
					return nil, "", fmt.Errorf("duplicate tool call id")
				}
				seen[id], pending[id] = true, true
			}
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 || len(pending) > 0 {
		return nil, "", fmt.Errorf("empty conversation or unanswered tool calls")
	}
	c := map[string]any{"model": model, "messages": msgs, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(m[8]) > 0 {
		if err := decodeSampling(m.data(8), c); err != nil {
			return nil, "", err
		}
	}
	var tools []map[string]any
	names := map[string]bool{}
	for _, f := range m[10] {
		t, err := decodeTool(f.b)
		if err != nil {
			return nil, "", err
		}
		name := t["function"].(map[string]any)["name"].(string)
		if names[name] {
			return nil, "", fmt.Errorf("duplicate tool definition")
		}
		names[name] = true
		tools = append(tools, t)
	}
	if len(tools) > 0 {
		c["tools"] = tools
	}
	if len(m[11]) > 0 {
		c["parallel_tool_calls"] = m.number(11) == 0
	}
	if len(m[12]) > 0 {
		choice, err := readMessage(m.data(12), map[int]int{1: textField, 2: textField})
		if err != nil {
			return nil, "", err
		}
		if len(choice[1])+len(choice[2]) != 1 {
			return nil, "", fmt.Errorf("tool choice must select exactly one alternative")
		}
		if len(choice[1]) > 0 {
			// The native schema declares a string, not an enum. These three
			// spellings have independent implementation evidence only.
			s := choice.text(1)
			if s != "auto" && s != "none" && s != "required" {
				return nil, "", fmt.Errorf("unsupported tool choice option")
			}
			if s == "required" && len(tools) == 0 {
				return nil, "", fmt.Errorf("required tool choice without tools")
			}
			c["tool_choice"] = s
		} else {
			name := choice.text(2)
			if !names[name] {
				return nil, "", fmt.Errorf("tool choice names an undefined tool")
			}
			c["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": name}}
		}
	}
	if m.number(20) == 3 {
		if choice, present := c["tool_choice"]; present && choice != "none" {
			return nil, "", fmt.Errorf("tool choice conflicts with native no-tool mode")
		}
		c["tool_choice"] = "none"
	}
	meta := map[string]string{}
	for n, key := range map[int]string{16: "devin_cascade_id", 17: "devin_prompt_id", 22: "devin_execution_id", 28: "devin_session_id"} {
		if v := m.text(n); v != "" {
			meta[key] = v
		}
	}
	if len(m[15]) > 0 {
		reference, referenceErr := readMessage(m.data(15), map[int]int{1: textField, 2: 0, 3: 0, 4: 0, 5: 0})
		if referenceErr != nil {
			return nil, "", referenceErr
		}
		if reference.number(5) != 0 {
			return nil, "", fmt.Errorf("native force-billable trajectory is unsupported")
		}
		meta["devin_trajectory_id"] = reference.text(1)
	}
	if len(meta) > 0 {
		c["metadata"] = meta
	}
	if key := m.text(27); key != "" {
		c["prompt_cache_key"] = key
	}
	session := m.text(28)
	if session == "" {
		session = m.text(16)
	}
	if strings.ContainsAny(session, "\r\n\x00") {
		return nil, "", fmt.Errorf("invalid native session identifier")
	}
	return c, session, nil
}

func cacheOptions(b []byte) (map[string]string, error) {
	m, err := readMessage(b, map[int]int{1: 0})
	if err != nil {
		return nil, err
	}
	switch m.number(1) {
	case 0:
		return nil, nil
	case 1:
		return map[string]string{"type": "ephemeral"}, nil
	default:
		return nil, fmt.Errorf("unsupported prompt cache type")
	}
}

func decodePrompt(b []byte) (map[string]any, error) {
	m, err := readMessage(b, map[int]int{1: textField, 2: 0, 3: textField, 4: 0, 5: 0, 6: 2 | repeated, 7: textField, 8: 2, 9: 0, 10: 2 | repeated, 11: textField, 12: textField, 13: 0, 15: textField, 16: textField, 17: 2, 18: textField, 19: textField, 22: textField})
	if err != nil {
		return nil, err
	}
	// The canonical Chat parser reconstructs thinking text without its
	// signature. Refuse signed history before a cross-protocol route can
	// silently discard that state; unsigned thinking remains representable.
	if m.text(12) != "" || m.text(18) != "" {
		return nil, fmt.Errorf("signed thinking history is unsupported by the Chat bridge")
	}
	if m.number(4) > math.MaxUint32 || m.number(5) > 1 {
		return nil, fmt.Errorf("invalid native prompt token count or telemetry flag")
	}
	if m.number(9) != 0 || m.number(13) != 0 {
		return nil, fmt.Errorf("native tool-error or redacted-thinking state is unsupported by the Chat bridge")
	}
	for _, field := range []int{15, 16, 17, 19, 22} {
		if len(m.data(field)) != 0 {
			return nil, fmt.Errorf("native prompt provenance or phase is unsupported by the Chat bridge")
		}
	}
	role := map[uint64]string{1: "user", 2: "assistant", 4: "tool", 5: "system"}[m.number(2)]
	if role == "" {
		return nil, fmt.Errorf("unsupported prompt role")
	}
	if len(m[6]) > 256 {
		return nil, fmt.Errorf("too many tool calls")
	}
	if role != "assistant" && (len(m[6])+len(m[11])+len(m[12])+len(m[18]) > 0) {
		return nil, fmt.Errorf("assistant-only content on another role")
	}
	if role != "tool" && len(m[7]) > 0 {
		return nil, fmt.Errorf("tool result id on another role")
	}
	if role != "user" && role != "tool" && len(m[10]) > 0 {
		return nil, fmt.Errorf("images on assistant or system prompts are unsupported")
	}
	out := map[string]any{"role": role, "content": m.text(3)}
	if role == "tool" {
		if m.text(7) == "" {
			return nil, fmt.Errorf("tool result requires call id")
		}
		out["tool_call_id"] = m.text(7)
	}
	if len(m[10]) > 0 {
		parts := []map[string]any{}
		if m.text(3) != "" {
			parts = append(parts, map[string]any{"type": "text", "text": m.text(3)})
		}
		for _, f := range m[10] {
			im, err := readMessage(f.b, map[int]int{1: textField, 2: textField, 3: textField})
			if err != nil {
				return nil, err
			}
			if im.text(3) != "" {
				return nil, fmt.Errorf("native image captions are unsupported")
			}
			kind := im.text(2)
			if kind != "image/png" && kind != "image/jpeg" && kind != "image/gif" && kind != "image/webp" {
				return nil, fmt.Errorf("unsupported image media type")
			}
			if im.text(1) == "" || strings.ContainsAny(im.text(1), "\r\n") {
				return nil, fmt.Errorf("invalid inline image")
			}
			if _, err := base64.StdEncoding.Strict().DecodeString(im.text(1)); err != nil {
				return nil, fmt.Errorf("invalid inline image base64")
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + kind + ";base64," + im.text(1)}})
		}
		out["content"] = parts
	}
	var calls []map[string]any
	for _, f := range m[6] {
		call, err := readMessage(f.b, map[int]int{1: textField, 2: textField, 3: textField})
		if err != nil {
			return nil, err
		}
		if call.text(1) == "" || call.text(2) == "" || !jsonObject(call.data(3)) {
			return nil, fmt.Errorf("invalid tool call")
		}
		calls = append(calls, map[string]any{"id": call.text(1), "type": "function", "function": map[string]any{"name": call.text(2), "arguments": call.text(3)}})
	}
	if len(calls) > 0 {
		out["tool_calls"] = calls
	}
	if m.text(11) != "" {
		out["reasoning_content"] = m.text(11)
	}
	if len(m[8]) > 0 {
		cache, err := cacheOptions(m.data(8))
		if err != nil {
			return nil, err
		}
		if cache != nil {
			out["cache_control"] = cache
		}
	}
	return out, nil
}

func jsonObject(b []byte) bool {
	var o map[string]json.RawMessage
	return json.Unmarshal(b, &o) == nil && o != nil
}

func decodeTool(b []byte) (map[string]any, error) {
	m, err := readMessage(b, map[int]int{1: textField, 2: textField, 3: textField, 12: 0})
	if err != nil {
		return nil, err
	}
	if m.text(1) == "" || !jsonObject(m.data(3)) || m.number(12) > 1 {
		return nil, fmt.Errorf("invalid tool definition")
	}
	f := map[string]any{"name": m.text(1), "parameters": json.RawMessage(m.data(3))}
	if len(m[2]) > 0 {
		f["description"] = m.text(2)
	}
	if len(m[12]) > 0 {
		f["strict"] = m.number(12) == 1
	}
	return map[string]any{"type": "function", "function": f}, nil
}
