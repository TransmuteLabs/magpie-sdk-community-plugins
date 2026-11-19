package devin

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"unicode/utf8"
)

const NativeClientVersion = "3000.11.3"

type EncodedChat struct {
	Payload   []byte
	Stream    bool
	Model     string
	Effort    string
	SentModel string
}

type outboundMessage struct {
	blockBoundary    bool
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	ToolCallID       string          `json:"tool_call_id"`
	ReasoningContent string          `json:"reasoning_content"`
	Reasoning        string          `json:"reasoning"`
	ReasoningDetails []struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		Signature string `json:"signature"`
		Format    string `json:"format"`
		Index     int    `json:"index"`
	} `json:"reasoning_details"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
	Cache json.RawMessage `json:"cache_control"`
}

type outboundChat struct {
	Model         string            `json:"model"`
	Effort        string            `json:"reasoning_effort"`
	Messages      []outboundMessage `json:"messages"`
	Stream        bool              `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
	Tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
			Strict      *bool           `json:"strict"`
		} `json:"function"`
	} `json:"tools"`
	ToolChoice    json.RawMessage   `json:"tool_choice"`
	Parallel      *bool             `json:"parallel_tool_calls"`
	MaxTokens     *uint64           `json:"max_tokens"`
	MaxCompletion *uint64           `json:"max_completion_tokens"`
	Temperature   *float64          `json:"temperature"`
	TopP          *float64          `json:"top_p"`
	TopK          *uint64           `json:"top_k"`
	MaxNewlines   *uint64           `json:"max_newlines"`
	Completions   *uint64           `json:"n"`
	Stop          json.RawMessage   `json:"stop"`
	Seed          *int64            `json:"seed"`
	ServiceTier   string            `json:"service_tier"`
	CacheKey      string            `json:"prompt_cache_key"`
	Metadata      map[string]string `json:"metadata"`
}

func NativeMetadata(key string) ([]byte, error) {
	if len(key) == 0 || len(key) > 4096 || strings.TrimSpace(key) != key {
		return nil, errors.New("invalid Devin credential")
	}
	for _, value := range []byte(key) {
		if value < 33 || value > 126 {
			return nil, errors.New("invalid Devin credential")
		}
	}
	metadata := addText(nil, 1, "devin-cli")
	metadata = addText(metadata, 2, NativeClientVersion)
	metadata = addText(metadata, 3, key)
	metadata = addText(metadata, 4, "en")
	metadata = addText(metadata, 5, runtime.GOOS)
	metadata = addText(metadata, 7, NativeClientVersion)
	metadata = addText(metadata, 12, "chisel")
	return addText(metadata, 28, "chisel"), nil
}

func NativeCatalogRequest(key string) ([]byte, error) {
	metadata, err := NativeMetadata(key)
	if err != nil {
		return nil, err
	}
	return addBytes(nil, 1, metadata), nil
}

func strictJSON(body []byte, target any) error {
	if !utf8.Valid(body) || !jsonObject(body) {
		return errors.New("expected a UTF-8 JSON object")
	}
	if _, err := catalogObject(body); err != nil {
		return errors.New("invalid or duplicate JSON object fields")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("unsupported or malformed canonical request fields")
	}
	return nil
}

func EncodeChatRequest(body []byte, key string, resolvers ...SignatureResolver) (EncodedChat, error) {
	if len(resolvers) > 1 {
		return EncodedChat{}, errors.New("only one scoped signature resolver is supported")
	}
	var resolve SignatureResolver
	if len(resolvers) == 1 {
		resolve = resolvers[0]
	}
	if len(body) > maxBody {
		return EncodedChat{}, errors.New("canonical request exceeds its byte limit")
	}
	var request outboundChat
	if err := strictJSON(body, &request); err != nil {
		return EncodedChat{}, err
	}
	if request.Model == "" || len(request.Messages) == 0 {
		return EncodedChat{}, errors.New("explicit native model UID and conversation are required")
	}
	effort, err := normalizeEffort(request.Effort)
	if err != nil {
		return EncodedChat{}, err
	}
	request.Effort = effort
	expanded, err := expandOutboundCacheContent(request.Messages)
	if err != nil {
		return EncodedChat{}, err
	}
	messages, err := normalizeOutboundTurns(expanded, request.Model)
	if err != nil {
		return EncodedChat{}, err
	}
	messages, err = attachDeferredToolImages(messages)
	if err != nil {
		return EncodedChat{}, err
	}
	payload := addText(nil, 21, request.Model)
	var prompts [][]byte
	signed := false
	for _, entry := range messages {
		message, err := encodeOutboundMessage(entry)
		if err != nil {
			return EncodedChat{}, err
		}
		payload = addBytes(payload, 3, message)
		blocks, err := encodeChatDetails(entry, message, request.Model, resolve)
		if err != nil {
			return EncodedChat{}, err
		}
		prompts = append(prompts, blocks...)
		signed = signed || len(entry.ReasoningDetails) > 0
	}
	encoded, err := finishOutboundRequest(request, payload, key)
	if err != nil || !signed {
		return encoded, err
	}
	return replaceOutboundPrompts(encoded, prompts)
}

func finishOutboundRequest(request outboundChat, payload []byte, key string) (EncodedChat, error) {
	for _, entry := range request.Tools {
		if entry.Type != "function" {
			return EncodedChat{}, errors.New("only ordinary function tools are supported")
		}
		tool := addText(nil, 1, entry.Function.Name)
		tool = addText(tool, 2, entry.Function.Description)
		tool = addBytes(tool, 3, entry.Function.Parameters)
		if entry.Function.Strict != nil {
			var strict uint64
			if *entry.Function.Strict {
				strict = 1
			}
			tool = addNumber(tool, 12, strict)
		}
		payload = addBytes(payload, 10, tool)
	}
	if request.Parallel != nil {
		var disabled uint64
		if !*request.Parallel {
			disabled = 1
		}
		payload = addNumber(payload, 11, disabled)
	}
	if len(request.ToolChoice) != 0 {
		var option string
		if json.Unmarshal(request.ToolChoice, &option) == nil {
			payload = addBytes(payload, 12, addText(nil, 1, option))
		} else {
			var named struct {
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if strictJSON(request.ToolChoice, &named) != nil || named.Type != "function" {
				return EncodedChat{}, errors.New("unsupported canonical tool choice")
			}
			payload = addBytes(payload, 12, addText(nil, 2, named.Function.Name))
		}
	}
	for number, name := range map[int]string{16: "devin_cascade_id", 17: "devin_prompt_id", 22: "devin_execution_id", 28: "devin_session_id"} {
		if value, found := request.Metadata[name]; found {
			payload = addText(payload, number, value)
			delete(request.Metadata, name)
		}
	}
	if len(request.Metadata) != 0 {
		return EncodedChat{}, errors.New("unsupported canonical metadata")
	}
	payload = addText(payload, 27, request.CacheKey)
	if _, _, err := decodeRequest(payload); err != nil {
		return EncodedChat{}, errors.New("canonical conversation has unsupported or inconsistent native semantics")
	}
	sampling, err := encodeOutboundSampling(request)
	if err != nil {
		return EncodedChat{}, err
	}
	metadata, err := NativeMetadata(key)
	if err != nil {
		return EncodedChat{}, err
	}
	payload = addBytes(payload, 1, metadata)
	payload = addNumber(payload, 7, 5)
	payload = addBytes(payload, 8, sampling)
	if len(payload) > maxBody {
		return EncodedChat{}, errors.New("native request exceeds its byte limit")
	}
	return EncodedChat{Payload: payload, Stream: request.Stream, Model: request.Model, Effort: request.Effort}, nil
}

func outboundOutputLimit(request outboundChat) (uint64, error) {
	maximum := uint64(128000)
	if request.MaxTokens != nil {
		maximum = *request.MaxTokens
	}
	if request.MaxCompletion != nil {
		if request.MaxTokens != nil && *request.MaxCompletion != *request.MaxTokens {
			return 0, errors.New("conflicting canonical output limits")
		}
		maximum = *request.MaxCompletion
	}
	if maximum == 0 || maximum > math.MaxInt32 {
		return 0, errors.New("invalid canonical output limit")
	}
	return maximum, nil
}

func encodeOutboundSampling(request outboundChat) ([]byte, error) {
	if request.Completions != nil && *request.Completions != 1 {
		return nil, errors.New("only one native completion is supported")
	}
	maximum, err := outboundOutputLimit(request)
	if err != nil {
		return nil, err
	}
	configuration := addNumber(nil, 1, 1)
	configuration = addNumber(configuration, 2, maximum)
	for _, item := range []struct {
		number       int
		value        *float64
		defaultValue float64
	}{{5, request.Temperature, 1}, {8, request.TopP, 0.95}} {
		value := item.defaultValue
		if item.value != nil {
			value = *item.value
		}
		configuration = binary.AppendUvarint(configuration, uint64(item.number<<3|1))
		configuration = binary.LittleEndian.AppendUint64(configuration, math.Float64bits(value))
	}
	if len(request.Stop) > 0 {
		var patterns []string
		var single string
		if json.Unmarshal(request.Stop, &single) == nil {
			patterns = []string{single}
		} else if json.Unmarshal(request.Stop, &patterns) != nil || patterns == nil {
			return nil, errors.New("invalid canonical stop patterns")
		}
		for _, pattern := range patterns {
			configuration = addBytes(configuration, 9, []byte(pattern))
		}
	}
	if request.Seed != nil {
		if *request.Seed < 0 {
			return nil, errors.New("negative seeds have no unsigned native mapping")
		}
		configuration = addNumber(configuration, 10, uint64(*request.Seed))
	}
	configuration = addText(configuration, 17, request.ServiceTier)
	if err := decodeSampling(configuration, map[string]any{}); err != nil {
		return nil, errors.New("invalid canonical sampling controls")
	}
	for _, item := range []struct {
		number       int
		value        *uint64
		defaultValue uint64
	}{{3, request.MaxNewlines, 400}, {7, request.TopK, 40}} {
		value := item.defaultValue
		if item.value != nil {
			value = *item.value
		}
		if value > math.MaxInt32 {
			return nil, errors.New("native generation control exceeds its integer range")
		}
		configuration = addNumber(configuration, item.number, value)
	}
	return configuration, nil
}

func encodeOutboundMessage(entry outboundMessage) ([]byte, error) {
	role, known := map[string]uint64{"system": 5, "developer": 5, "user": 1, "assistant": 2, "tool": 4}[entry.Role]
	if !known {
		return nil, errors.New("unsupported canonical message role")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, errors.New("could not allocate a native message identity")
	}
	random[6], random[8] = random[6]&15|64, random[8]&63|128
	id := hex.EncodeToString(random[:])
	message := addText(nil, 1, fmt.Sprintf("%s-%s-%s-%s-%s", id[:8], id[8:12], id[12:16], id[16:20], id[20:]))
	message = addNumber(message, 2, role)
	text, images, err := encodeOutboundContent(entry.Content)
	if err != nil {
		return nil, err
	}
	message = addBytes(message, 3, []byte(text))
	for _, image := range images {
		message = addBytes(message, 10, image)
	}
	message = addText(message, 7, NativeToolID(entry.ToolCallID))
	message = addText(message, 11, entry.ReasoningContent)
	for _, call := range entry.ToolCalls {
		if call.Type != "function" {
			return nil, errors.New("only ordinary function calls are supported")
		}
		tool := addText(nil, 1, NativeToolID(call.ID))
		tool = addText(tool, 2, call.Function.Name)
		tool = addBytes(tool, 3, []byte(call.Function.Arguments))
		message = addBytes(message, 6, tool)
	}
	if len(entry.Cache) > 0 {
		if _, err := normalizePromptCache(entry.Cache); err != nil {
			return nil, errors.New("unsupported canonical cache options")
		}
		message = addBytes(message, 8, addNumber(nil, 1, 1))
	}
	if _, err := decodePrompt(message); err != nil {
		return nil, errors.New("unsupported canonical message semantics")
	}
	return message, nil
}

func encodeOutboundContent(content json.RawMessage) (string, [][]byte, error) {
	if len(content) == 0 || bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
		return "", nil, nil
	}
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text, nil, nil
	}
	var parts []json.RawMessage
	if json.Unmarshal(content, &parts) != nil || parts == nil {
		return "", nil, errors.New("unsupported canonical message content")
	}
	var joined strings.Builder
	var images [][]byte
	for _, part := range parts {
		object, err := catalogObject(part)
		if err != nil {
			return "", nil, errors.New("invalid canonical content part")
		}
		kind, err := catalogString(object, "type")
		if err != nil {
			return "", nil, errors.New("invalid canonical content type")
		}
		switch kind {
		case "text":
			value, err := catalogString(object, "text")
			if err != nil || len(object) != 2 || len(images) > 0 && value != "" {
				return "", nil, errors.New("native content cannot reorder text after images")
			}
			joined.WriteString(value)
		case "image_url":
			var image struct {
				URL    string `json:"url"`
				Detail string `json:"detail"`
			}
			if len(object) != 2 || strictJSON(object["image_url"], &image) != nil || image.Detail != "" && image.Detail != "auto" {
				return "", nil, errors.New("unsupported canonical image options")
			}
			prefix, data, found := strings.Cut(image.URL, ";base64,")
			mime := strings.TrimPrefix(prefix, "data:")
			if !found || prefix == mime || data == "" ||
				mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp" {
				return "", nil, errors.New("native images require a supported inline image")
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(data)
			if err != nil {
				return "", nil, errors.New("invalid inline image encoding")
			}
			mime = inlineImageType(decoded, mime)
			images = append(images, addText(addText(nil, 1, data), 2, mime))
		default:
			return "", nil, errors.New("unsupported canonical content part")
		}
	}
	return joined.String(), images, nil
}

func inlineImageType(data []byte, declared string) string {
	switch {
	case bytes.HasPrefix(data, []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "image/png"
	case bytes.HasPrefix(data, []byte{255, 216, 255}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	default:
		return declared
	}
}

func CanonicalToolID(id string) string {
	safe := id != ""
	for _, value := range []byte(id) {
		safe = safe && (value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' || value == '-')
	}
	if safe && !strings.HasPrefix(id, "dv_") {
		return id
	}
	return "dv_" + base64.RawURLEncoding.EncodeToString([]byte(id))
}

func NativeToolID(id string) string {
	if !strings.HasPrefix(id, "dv_") {
		return id
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(id, "dv_"))
	if err == nil && len(decoded) > 0 && utf8.Valid(decoded) && CanonicalToolID(string(decoded)) == id {
		return string(decoded)
	}
	return id
}
