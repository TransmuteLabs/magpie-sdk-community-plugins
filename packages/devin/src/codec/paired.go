package devin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"
)

type NativeClaim struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
	Format string `json:"format,omitempty"`
	Model  string `json:"model,omitempty"`
}

type NativeEnvelope struct {
	Payload    []byte
	Projection []byte
	Model      string
	Session    string
	RequestID  string
	Timeout    time.Duration
	Claims     []NativeClaim
}

func SignatureDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func RawFields(payload []byte) (map[int][]byte, error) {
	if _, err := readMessage(payload, nil); err != nil {
		return nil, err
	}
	result := map[int][]byte{}
	for len(payload) > 0 {
		start := payload
		tag, used := binary.Uvarint(payload)
		payload = payload[used:]
		number, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0:
			_, used = binary.Uvarint(payload)
			payload = payload[used:]
		case 1:
			payload = payload[8:]
		case 5:
			payload = payload[4:]
		case 2:
			size, count := binary.Uvarint(payload)
			payload = payload[count+int(size):]
		}
		result[number] = append(result[number], start[:len(start)-len(payload)]...)
	}
	return result, nil
}

func ReplaceNativeAuthority(payload []byte, model, key string) ([]byte, error) {
	if model == "" || strings.TrimSpace(model) != model || strings.ContainsAny(model, "\r\n\x00") {
		return nil, errors.New("invalid selected native model")
	}
	chunks, err := RawFields(payload)
	if err != nil {
		return nil, errors.New("invalid native envelope")
	}
	metadata, err := NativeMetadata(key)
	if err != nil {
		return nil, err
	}
	var result []byte
	for number := 1; number <= 28; number++ {
		if number != 1 && number != 14 && number != 21 {
			result = append(result, chunks[number]...)
		}
	}
	for number := range chunks {
		if number > 28 {
			return nil, errors.New("unsupported native envelope field")
		}
	}
	result = addText(result, 14, model)
	result = addText(result, 21, model)
	result = addBytes(result, 1, metadata)
	if len(result) > maxBody {
		return nil, errors.New("native request exceeds its byte limit")
	}
	return result, nil
}

// ApplyNativeOutputLimit reconciles the host's admitted output limit with the
// retained native request. All other native controls and signed history remain
// byte-for-byte intact; an absent canonical limit leaves the native limit alone.
func ApplyNativeOutputLimit(payload, body []byte) ([]byte, error) {
	object, err := catalogObject(body)
	if err != nil {
		return nil, err
	}
	var controls outboundChat
	for _, item := range []struct {
		name   string
		target **uint64
	}{
		{"max_tokens", &controls.MaxTokens}, {"max_completion_tokens", &controls.MaxCompletion},
	} {
		if raw, found := object[item.name]; found {
			if err := json.Unmarshal(raw, item.target); err != nil {
				return nil, err
			}
		}
	}
	if controls.MaxTokens == nil && controls.MaxCompletion == nil {
		return bytes.Clone(payload), nil
	}
	maximum, err := outboundOutputLimit(controls)
	if err != nil {
		return nil, err
	}
	fields, err := projectedMessage(payload, map[int]int{8: 2})
	if err != nil {
		return nil, err
	}
	if _, err := projectedMessage(fields.data(8), map[int]int{2: 0}); err != nil {
		return nil, err
	}
	replace := func(raw []byte, number int, value []byte) ([]byte, error) {
		chunks, err := orderedChunks(raw)
		if err != nil {
			return nil, err
		}
		var result []byte
		found := false
		for _, chunk := range chunks {
			if chunk.number == number {
				result = append(result, value...)
				found = true
			} else {
				result = append(result, chunk.raw...)
			}
		}
		if !found {
			result = append(result, value...)
		}
		return result, nil
	}
	sampling, err := replace(fields.data(8), 2, addNumber(nil, 2, maximum))
	if err != nil {
		return nil, err
	}
	result, err := replace(payload, 8, addBytes(nil, 8, sampling))
	if err == nil && len(result) > maxBody {
		return nil, errors.New("native request exceeds its byte limit")
	}
	return result, err
}

func ParseNativeEnvelope(ctx context.Context, header http.Header, body []byte) (NativeEnvelope, error) {
	return parseNativeEnvelope(ctx, header, body, false)
}

func ParseAgentEnvelope(ctx context.Context, header http.Header, body []byte) (NativeEnvelope, error) {
	return parseNativeEnvelope(ctx, header, body, true)
}

func parseNativeEnvelope(ctx context.Context, header http.Header, body []byte, assignments bool) (NativeEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return NativeEnvelope{}, err
	}
	kind, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	if len(header.Values("Content-Type")) != 1 || err != nil || kind != "application/connect+proto" {
		return NativeEnvelope{}, refusal(415, "expected application/connect+proto")
	}
	if values := header.Values("Connect-Protocol-Version"); len(values) != 1 || values[0] != "1" {
		return NativeEnvelope{}, refusal(400, "expected Connect-Protocol-Version 1")
	}
	encoding := strings.ToLower(strings.TrimSpace(header.Get("Connect-Content-Encoding")))
	if len(header.Values("Connect-Content-Encoding")) > 1 || encoding != "" && encoding != "identity" && encoding != "gzip" {
		return NativeEnvelope{}, refusal(415, "unsupported Connect compression")
	}
	if values := header.Values("Content-Encoding"); len(values) > 1 || len(values) == 1 && values[0] != "" && !strings.EqualFold(values[0], "identity") {
		return NativeEnvelope{}, refusal(415, "unsupported HTTP content encoding")
	}
	timeout, err := decodeTimeout(header)
	if err != nil {
		return NativeEnvelope{}, err
	}
	if len(body) > 0 && body[0]&1 != 0 && encoding != "gzip" {
		return NativeEnvelope{}, refusal(400, "compressed Connect frame requires gzip encoding")
	}
	payload, err := unframe(body)
	if err != nil {
		return NativeEnvelope{}, refusal(400, "invalid native Connect envelope")
	}
	fields, err := projectedMessage(payload, map[int]int{1: 2, 2: textField, 3: 2 | repeated, 5: 0, 6: 0, 7: 0, 8: 2, 9: 2, 10: 2 | repeated, 11: 0, 12: 2, 13: 2, 14: textField, 15: 2, 16: textField, 17: textField, 18: 0, 19: 0, 20: 0, 21: textField, 22: textField, 24: 0, 25: textField, 26: textField, 27: textField, 28: textField})
	if err != nil {
		return NativeEnvelope{}, refusal(400, "invalid native protobuf envelope")
	}
	for number := range fields {
		if number > 28 || number == 4 || number == 23 {
			return NativeEnvelope{}, refusal(400, "unsupported native request field")
		}
	}
	checkMetadata := validateNativeMetadata
	if assignments {
		checkMetadata = validateAccountMetadata
	}
	if err := checkMetadata(fields.data(1)); err != nil {
		return NativeEnvelope{}, refusal(400, "invalid native credential envelope")
	}
	if fields.number(5) != 0 || fields.number(11) > 1 || fields.number(7) != 0 && fields.number(7) != 5 || len(fields.data(9)) > 0 ||
		fields.number(24) != 0 || fields.text(25) != "" || !assignments && fields.text(26) != "" {
		return NativeEnvelope{}, refusal(400, "native internal-model, experiment or assignment state is unsupported")
	}
	model := fields.text(21)
	if model == "" {
		model = fields.text(14)
	}
	if model == "" || len(model) > 512 || strings.TrimSpace(model) != model || strings.ContainsAny(model, "\r\n\x00") {
		return NativeEnvelope{}, refusal(400, "explicit native model UID is required")
	}
	if len(fields[3]) > 2048 || len(fields[10]) > 256 {
		return NativeEnvelope{}, refusal(413, "native conversation exceeds its item limit")
	}
	envelope := NativeEnvelope{Model: model, Timeout: timeout, Session: fields.text(28), RequestID: fields.text(22)}
	if token := fields.text(26); token != "" {
		if fields.text(16) == "" {
			return NativeEnvelope{}, refusal(400, "native assignment requires its cascade identity")
		}
		envelope.Claims = append(envelope.Claims, NativeClaim{Kind: "assignment",
			Digest: SignatureDigest([]byte(token)), Format: fields.text(16)})
	}
	if envelope.Session == "" {
		envelope.Session = fields.text(16)
	}
	if envelope.RequestID == "" {
		envelope.RequestID = fields.text(17)
	}
	if len(envelope.RequestID) > 256 || strings.ContainsAny(envelope.RequestID+envelope.Session, "\r\n\x00") {
		return NativeEnvelope{}, refusal(400, "invalid native correlation identity")
	}
	var messages []map[string]any
	if system := fields.text(2); system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}
	for _, part := range fields[3] {
		prompt, err := projectedMessage(part.b, map[int]int{2: 0, 3: textField, 6: 2 | repeated, 7: textField, 10: 2 | repeated, 11: textField, 12: textField, 13: 0, 15: textField, 16: textField, 17: 2, 18: textField, 22: textField})
		if err != nil {
			return NativeEnvelope{}, refusal(400, "invalid native prompt")
		}
		role := map[uint64]string{1: "user", 2: "assistant", 4: "tool", 5: "system"}[prompt.number(2)]
		if role == "" {
			return NativeEnvelope{}, refusal(400, "unsupported native prompt role")
		}
		message := map[string]any{"role": role, "content": prompt.text(3)}
		if thinking := prompt.text(11); thinking != "" {
			message["reasoning_content"] = thinking
		}
		if role == "tool" {
			message["tool_call_id"] = CanonicalToolID(prompt.text(7))
		}
		var calls []any
		for _, call := range prompt[6] {
			tool, err := projectedMessage(call.b, map[int]int{1: textField, 2: textField, 3: textField})
			if err != nil {
				return NativeEnvelope{}, refusal(400, "invalid native tool call")
			}
			calls = append(calls, map[string]any{"id": CanonicalToolID(tool.text(1)), "type": "function",
				"function": map[string]any{"name": tool.text(2), "arguments": tool.text(3)}})
		}
		if len(calls) > 0 {
			message["tool_calls"] = calls
		}
		if len(prompt[10]) > 0 {
			content := []any{map[string]any{"type": "text", "text": message["content"]}}
			for _, image := range prompt[10] {
				value, err := projectedMessage(image.b, map[int]int{1: textField, 2: textField, 3: textField})
				if err != nil {
					return NativeEnvelope{}, refusal(400, "invalid native image")
				}
				content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + value.text(2) + ";base64," + value.text(1)}})
				if caption := value.text(3); caption != "" {
					content = append(content, map[string]any{"type": "text", "text": caption})
				}
			}
			message["content"] = content
		}
		for _, entry := range []struct {
			number int
			kind   string
		}{{12, "signature"}, {17, "gemini"}} {
			if value := prompt.data(entry.number); len(value) > 0 {
				if len(prompt.text(18)) > 256 || len(prompt.text(22)) > 512 {
					return NativeEnvelope{}, refusal(400, "native signature origin identity exceeds its limit")
				}
				envelope.Claims = append(envelope.Claims, NativeClaim{Kind: entry.kind, Digest: SignatureDigest(value), Format: prompt.text(18), Model: prompt.text(22)})
			}
		}
		if prompt.number(13) != 0 && len(prompt.data(12)) == 0 && len(prompt.data(17)) == 0 {
			return NativeEnvelope{}, refusal(400, "redacted history requires a known native signature")
		}
		messages = append(messages, message)
	}
	if len(messages) == 0 {
		return NativeEnvelope{}, refusal(400, "native conversation is empty")
	}
	var tools []any
	for _, entry := range fields[10] {
		tool, err := projectedMessage(entry.b, map[int]int{1: textField, 2: textField, 3: textField, 10: textField, 11: textField})
		if err != nil {
			return NativeEnvelope{}, refusal(400, "invalid native tool declaration")
		}
		parameters := json.RawMessage(tool.text(3))
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{}`)
		}
		var schema map[string]json.RawMessage
		if json.Unmarshal(parameters, &schema) != nil || schema == nil {
			return NativeEnvelope{}, refusal(400, "native tool schema is not an object")
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": tool.text(1), "description": tool.text(2), "parameters": parameters}})
		if tool.text(10) != "" || tool.text(11) != "" {
			messages = append(messages, map[string]any{"role": "system", "content": tool.text(10) + "\n" + tool.text(11)})
		}
	}
	projection := map[string]any{"model": model, "messages": messages, "stream": true, "stream_options": map[string]bool{"include_usage": true}}
	if len(tools) > 0 {
		projection["tools"] = tools
	}
	if len(fields[8]) > 0 {
		config, err := projectedMessage(fields.data(8), map[int]int{1: 0, 2: 0})
		if err != nil || config.number(1) > 1 || config.number(2) > math.MaxInt32 {
			return NativeEnvelope{}, refusal(400, "unsupported native completion count or output limit")
		}
		if config.number(2) > 0 {
			projection["max_tokens"] = config.number(2)
		}
	}
	envelope.Projection, err = json.Marshal(projection)
	if err != nil || len(envelope.Projection) > maxBody {
		return NativeEnvelope{}, refusal(413, "native admission projection exceeds its byte limit")
	}
	chunks, err := RawFields(payload)
	if err != nil {
		return NativeEnvelope{}, refusal(400, "invalid native request envelope")
	}
	var clean []byte
	for number := 2; number <= 28; number++ {
		clean = append(clean, chunks[number]...)
	}
	envelope.Payload = bytes.Clone(clean)
	return envelope, nil
}

func validateNativeMetadata(payload []byte) error {
	fields, err := projectedMessage(payload, map[int]int{
		1: textField, 2: textField, 3: textField, 4: textField, 5: textField, 6: 0, 7: textField, 8: textField,
		9: 0, 10: textField, 11: textField, 12: textField, 13: textField, 14: textField, 15: 0, 16: 2, 17: textField,
		20: textField, 21: textField, 22: textField, 24: textField, 25: textField, 26: textField, 27: textField,
		28: textField, 29: textField, 31: textField, 32: textField, 33: 0,
	})
	if err != nil {
		return err
	}
	for number := range fields {
		if number > 33 || number == 18 || number == 19 || number == 23 || number == 30 {
			return errors.New("unsupported native metadata field")
		}
	}
	if fields.number(6) > 1 || fields.number(33) != 0 || fields.text(22) != "" || fields.text(29) != "" ||
		fields.text(31) != "" || fields.text(32) != "" {
		return errors.New("unsupported native team, tier or evaluation selection")
	}
	return nil
}

type NativeFrame struct {
	Flags   byte
	Payload []byte
}

func ReadNativeRawStream(ctx context.Context, reader interface{ Read([]byte) (int, error) }, gzipAllowed bool, emit func(NativeFrame) error) (NativeSummary, error) {
	var result NativeSummary
	said, tools := false, false
	seen := false
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		flags, payload, err := ReadConnectFrame(reader, gzipAllowed)
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			return result, err
		}
		if flags&2 == 0 {
			seen = true
			fields, err := nativeResponseFields(payload)
			if err != nil {
				return result, err
			}
			content := fields.text(3) != "" || fields.text(9) != "" || len(fields[6]) != 0 ||
				fields.number(8) != 0 || fields.number(11) != 0
			if result.Stop != 0 && content {
				return result, &NativeError{Code: "data_loss"}
			}
			if fields.number(5) != 0 {
				if result.Stop != 0 && fields.number(5) != result.Stop {
					return result, &NativeError{Code: "data_loss"}
				}
				result.Stop = fields.number(5)
			}
			metrics, err := nativeMetrics(fields)
			if err != nil {
				return result, err
			}
			result.Merge(metrics)
			said = said || content
			tools = tools || len(fields[6]) != 0
			if err := emit(NativeFrame{Flags: 0, Payload: payload}); err != nil {
				return result, err
			}
			continue
		}
		var end struct {
			Error *struct {
				Code    string          `json:"code"`
				Type    string          `json:"type"`
				Message string          `json:"message"`
				Details json.RawMessage `json:"details"`
			} `json:"error"`
			Metadata map[string][]string `json:"metadata"`
		}
		if strictJSON(payload, &end) != nil {
			return result, &NativeError{Code: "data_loss"}
		}
		if end.Error != nil && (end.Error.Code == "" || len(end.Error.Code) > 128 || strings.TrimSpace(end.Error.Code) != end.Error.Code ||
			strings.ContainsAny(end.Error.Code, "\r\n\x00")) {
			return result, &NativeError{Code: "data_loss"}
		}
		var trailing [1]byte
		count, tailErr := io.ReadFull(reader, trailing[:])
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if count != 0 || tailErr != io.EOF {
			return result, &NativeError{Code: "data_loss"}
		}
		if end.Error == nil {
			if err := nativeCompletion(result.Stop, said, tools); err != nil {
				return result, err
			}
		}
		if err := emit(NativeFrame{Flags: 2, Payload: payload}); err != nil {
			return result, err
		}
		if end.Error != nil {
			failure := ParseResponseFailure(payload, true)
			return result, &NativeError{Code: end.Error.Code, Failure: failure, BeforeEffect: !seen && failure.RejectsBeforeEffect()}
		}
		return result, nil
	}
}

type NativeOriginDelta struct {
	Signature   []byte
	Gemini      []byte
	Format      string
	ActualModel string
	ThinkingID  string
}

func DecodeOriginDelta(payload []byte) (NativeOriginDelta, error) {
	fields, err := projectedMessage(payload, map[int]int{10: textField, 20: 2, 21: textField, 23: textField, 16: textField})
	if err != nil {
		return NativeOriginDelta{}, err
	}
	if len(fields.text(21)) > 256 || len(fields.text(23)) > 512 || len(fields.text(16)) > 256 {
		return NativeOriginDelta{}, errors.New("native signature origin identity exceeds its limit")
	}
	return NativeOriginDelta{Signature: bytes.Clone(fields.data(10)), Gemini: bytes.Clone(fields.data(20)), Format: fields.text(21), ActualModel: fields.text(23), ThinkingID: fields.text(16)}, nil
}
