package commandcode

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const maxStreamBytes = 16 << 20
const maxEventBytes = 1 << 20

type nativeEvent struct {
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Delta            string          `json:"delta"`
	InputTextDelta   string          `json:"inputTextDelta"`
	ID               string          `json:"id"`
	ToolCallID       string          `json:"toolCallId"`
	ToolName         string          `json:"toolName"`
	Input            json.RawMessage `json:"input"`
	Args             json.RawMessage `json:"args"`
	ProviderExecuted bool            `json:"providerExecuted"`
	FinishReason     string          `json:"finishReason"`
	RawFinishReason  string          `json:"rawFinishReason"`
	TotalUsage       json.RawMessage `json:"totalUsage"`
	CacheWrite       json.RawMessage `json:"cacheWriteTokens"`
	Error            json.RawMessage `json:"error"`
}

type nativeTool struct {
	ID        string
	Name      string
	Arguments string
	Complete  bool
	Server    bool
}

type NativeFailure struct {
	Status     int
	Type       string
	Message    string
	NotStarted bool
}

func (failure *NativeFailure) Error() string { return failure.Message }

func TranslateGenerate(reader io.Reader, model string, streaming bool, emit func([]byte) error) (sdk.Usage, error) {
	return translateGenerate(reader, model, streaming, emit, nil)
}

func translateGenerate(reader io.Reader, model string, streaming bool, emit func([]byte) error, resume func() (io.ReadCloser, error)) (sdk.Usage, error) {
	var usage sdk.Usage
	var carried sdk.Usage
	var continuation io.ReadCloser
	defer func() {
		if continuation != nil {
			continuation.Close()
		}
	}()
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return usage, err
	}
	id := "chatcmpl-" + hex.EncodeToString(random[:])
	created := time.Now().Unix()
	tools := make(map[string]*nativeTool)
	var order []*nativeTool
	var content, reasoning strings.Builder
	finished := false
	finishReason := ""
	seenRole := false
	progressed := false
	chunk := func(delta map[string]any, finish any, counts any) error {
		body := map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
		if counts != nil {
			body["usage"] = counts
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		return emit(append(append([]byte("data: "), encoded...), []byte("\n\n")...))
	}
	textChunk := func(field, text string) error {
		if !streaming || text == "" {
			return nil
		}
		delta := map[string]any{field: text}
		if !seenRole {
			delta["role"] = "assistant"
			seenRole = true
		}
		return chunk(delta, nil, nil)
	}
	remaining := int64(maxStreamBytes + 1)
	for attempt := 0; ; attempt++ {
		paused := false
		finished = false
		usage = sdk.Usage{}
		input := &io.LimitedReader{R: reader, N: remaining}
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), maxEventBytes)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 || bytes.HasPrefix(line, []byte(":")) || bytes.HasPrefix(line, []byte("event:")) {
				continue
			}
			line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if bytes.Equal(line, []byte("[DONE]")) {
				if !finished {
					return usage, errors.New("native stream ended without a finish event")
				}
				continue
			}
			var event nativeEvent
			if decode(line, &event) != nil || event.Type == "" {
				return usage, errors.New("native stream contains a malformed event")
			}
			if finished && event.Type != "error" && event.Type != "cache-write-tokens" && event.Type != "provider-metadata" {
				return usage, errors.New("native stream emitted work after its finish event")
			}
			switch event.Type {
			case "start", "start-step", "text-start", "text-end", "reasoning-start", "reasoning-end", "finish-step":
			case "text-delta", "reasoning-delta":
				progressed = true
				text := event.Text
				if text == "" {
					text = event.Delta
				}
				field := "content"
				if event.Type == "reasoning-delta" {
					reasoning.WriteString(text)
					field = "reasoning_content"
				} else {
					content.WriteString(text)
				}
				if err := textChunk(field, text); err != nil {
					return usage, err
				}
			case "tool-input-start":
				progressed = true
				if event.ID == "" || event.ToolName == "" || tools[event.ID] != nil || len(order) >= 128 {
					return usage, errors.New("native stream tool identity is invalid or duplicated")
				}
				tool := &nativeTool{ID: event.ID, Name: event.ToolName, Server: event.ProviderExecuted}
				tools[event.ID] = tool
				order = append(order, tool)
			case "tool-input-delta":
				tool := tools[event.ID]
				if tool == nil || tool.Complete {
					return usage, errors.New("native stream tool delta has no open input")
				}
				delta := event.Delta
				if delta == "" {
					delta = event.InputTextDelta
				}
				if len(tool.Arguments)+len(delta) > maxEventBytes {
					return usage, errors.New("native stream tool arguments exceed their limit")
				}
				tool.Arguments += delta
			case "tool-input-end":
				if tools[event.ID] == nil {
					return usage, errors.New("native stream tool input end has no start")
				}
			case "tool-call":
				progressed = true
				key := event.ToolCallID
				if key == "" || event.ToolName == "" {
					return usage, errors.New("native stream tool call lacks an identity")
				}
				tool := tools[key]
				if tool == nil {
					if len(order) >= 128 {
						return usage, errors.New("native stream tool count exceeds its limit")
					}
					tool = &nativeTool{ID: key, Name: event.ToolName}
					tools[key] = tool
					order = append(order, tool)
				}
				if tool.Complete || tool.Name != event.ToolName {
					return usage, errors.New("native stream tool call is inconsistent or duplicated")
				}
				raw := event.Input
				if len(raw) == 0 {
					raw = event.Args
				}
				var encoded string
				if json.Unmarshal(raw, &encoded) == nil {
					raw = []byte(encoded)
				}
				var object map[string]any
				if decode(raw, &object) != nil || object == nil {
					return usage, errors.New("native stream tool arguments are not a JSON object")
				}
				if tool.Arguments != "" {
					var fragments map[string]any
					if decode([]byte(tool.Arguments), &fragments) != nil {
						return usage, errors.New("native stream tool fragments are malformed")
					}
					left, _ := json.Marshal(fragments)
					right, _ := json.Marshal(object)
					if !bytes.Equal(left, right) {
						return usage, errors.New("native stream consolidated tool differs from its fragments")
					}
				}
				arguments, _ := json.Marshal(object)
				tool.Arguments = string(arguments)
				tool.Complete = true
				tool.Server = tool.Server || event.ProviderExecuted
			case "finish":
				paused = event.RawFinishReason == "pause_turn"
				if !paused {
					switch event.FinishReason {
					case "stop", "end-turn", "end_turn":
						finishReason = "stop"
					case "tool-calls", "tool_calls", "tool_use":
						finishReason = "tool_calls"
					case "length", "max_tokens", "max-tokens":
						finishReason = "length"
					case "content-filter", "content_filter":
						finishReason = "content_filter"
					default:
						return usage, fmt.Errorf("native stream finish reason %q is unsupported", event.FinishReason)
					}
				}
				if len(event.TotalUsage) > 0 && string(event.TotalUsage) != "null" {
					value, err := nativeUsage(event.TotalUsage)
					if err != nil {
						return usage, err
					}
					value.CacheWriteTokens = max(value.CacheWriteTokens, usage.CacheWriteTokens)
					usage = value
				}
				finished = true
			case "cache-write-tokens":
				written, err := counter(event.CacheWrite)
				if err != nil {
					return usage, err
				}
				usage.CacheWriteTokens = max(written, usage.CacheWriteTokens)
			case "provider-metadata":
			case "tool-result", "tool-output-available":
				var metadata struct {
					ToolID           string `json:"toolCallId"`
					ProviderExecuted bool   `json:"providerExecuted"`
				}
				if decode(line, &metadata) != nil || !metadata.ProviderExecuted || tools[metadata.ToolID] == nil || !tools[metadata.ToolID].Server {
					return usage, errors.New("native stream server tool result ownership is unknown")
				}
			case "error":
				var failure struct {
					Message string `json:"message"`
					Status  int    `json:"statusCode"`
					Type    string `json:"type"`
					Code    string `json:"code"`
				}
				if json.Unmarshal(event.Error, &failure) != nil {
					json.Unmarshal(event.Error, &failure.Message)
				}
				if failure.Message == "" {
					failure.Message = "CommandCode native stream reported an error"
				}
				status := refusalStatus(failure.Status, []byte(failure.Message))
				if status < 400 || status > 599 {
					status = 502
				}
				if failure.Type == "" {
					failure.Type = "commandcode_error"
				}
				if failure.Code == "CMD_ZDR_NO_PROVIDERS" {
					failure.Type = "cmd_zdr_no_providers"
				}
				notStarted := attempt == 0 && !progressed && !finished && (status == 400 || status == 401 || status == 402 || status == 403 || status == 422 || status == 429)
				return usage, &NativeFailure{Status: status, Type: failure.Type, Message: failure.Message, NotStarted: notStarted}
			case "abort":
				return usage, errors.New("CommandCode native stream was aborted")
			default:
				return usage, fmt.Errorf("native stream event %q is unsupported", event.Type)
			}
		}
		if err := scanner.Err(); err != nil {
			return usage, errors.New("native stream frame is truncated or oversized")
		}
		if input.N <= 0 || !finished {
			return usage, errors.New("native stream is oversized or missing its terminal finish event")
		}
		if usage.CachedTokens+usage.CacheWriteTokens > usage.InputTokens {
			return usage, errors.New("native usage cache split exceeds total input")
		}
		remaining = input.N
		if carried.InputTokens > 1<<52-usage.InputTokens || carried.OutputTokens > 1<<52-usage.OutputTokens ||
			carried.CachedTokens > 1<<52-usage.CachedTokens || carried.CacheWriteTokens > 1<<52-usage.CacheWriteTokens {
			return usage, errors.New("native continuation usage exceeds its bounded limit")
		}
		usage = sdk.Usage{InputTokens: carried.InputTokens + usage.InputTokens, OutputTokens: carried.OutputTokens + usage.OutputTokens,
			CachedTokens: carried.CachedTokens + usage.CachedTokens, CacheWriteTokens: carried.CacheWriteTokens + usage.CacheWriteTokens}
		if !paused {
			break
		}
		if resume == nil {
			return usage, errors.New("native paused turn requires its owning transport continuation")
		}
		if attempt >= 5 {
			return usage, errors.New("native paused turn exceeded five continuations; outcome remains uncertain")
		}
		for _, tool := range order {
			if !tool.Complete {
				return usage, errors.New("native paused turn has unfinished tool arguments")
			}
		}
		carried = usage
		if continuation != nil {
			continuation.Close()
			continuation = nil
		}
		next, err := resume()
		if err != nil {
			return usage, err
		}
		continuation = next
		reader = next
	}
	calls := make([]any, 0, len(order))
	for _, tool := range order {
		if !tool.Complete {
			return usage, errors.New("native stream ended with unfinished tool arguments")
		}
		if tool.Server {
			continue
		}
		calls = append(calls, map[string]any{"id": tool.ID, "type": "function", "function": map[string]string{"name": tool.Name, "arguments": tool.Arguments}})
	}
	if len(calls) > 0 && finishReason == "stop" {
		finishReason = "tool_calls"
	}
	if finishReason == "tool_calls" && len(calls) == 0 {
		return usage, errors.New("native stream requested tool execution without a client tool")
	}
	counts := map[string]any{"prompt_tokens": usage.InputTokens, "completion_tokens": usage.OutputTokens, "total_tokens": usage.InputTokens + usage.OutputTokens,
		"prompt_tokens_details": map[string]int64{"cached_tokens": usage.CachedTokens, "cache_write_tokens": usage.CacheWriteTokens}}
	if streaming {
		for index, tool := range calls {
			call := tool.(map[string]any)
			call["index"] = index
			if err := chunk(map[string]any{"tool_calls": []any{call}}, nil, nil); err != nil {
				return usage, err
			}
		}
		if err := chunk(map[string]any{}, finishReason, counts); err != nil {
			return usage, err
		}
		return usage, emit([]byte("data: [DONE]\n\n"))
	}
	message := map[string]any{"role": "assistant", "content": content.String()}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	if len(calls) > 0 {
		message["tool_calls"] = calls
	}
	body, err := json.Marshal(map[string]any{"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason}}, "usage": counts})
	if err != nil {
		return usage, err
	}
	return usage, emit(body)
}

func counter(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || value < 0 || value > 1<<52 {
		return 0, errors.New("native usage counter is invalid")
	}
	return value, nil
}

func nativeUsage(raw json.RawMessage) (sdk.Usage, error) {
	var value struct {
		Input   json.RawMessage `json:"inputTokens"`
		Output  json.RawMessage `json:"outputTokens"`
		Details struct {
			NoCache json.RawMessage `json:"noCacheTokens"`
			Read    json.RawMessage `json:"cacheReadTokens"`
			Write   json.RawMessage `json:"cacheWriteTokens"`
		} `json:"inputTokenDetails"`
	}
	if decode(raw, &value) != nil {
		return sdk.Usage{}, errors.New("native usage is malformed")
	}
	input, errInput := counter(value.Input)
	output, errOutput := counter(value.Output)
	cached, errCached := counter(value.Details.Read)
	written, errWritten := counter(value.Details.Write)
	uncached, errUncached := counter(value.Details.NoCache)
	if errInput != nil || errOutput != nil || errCached != nil || errWritten != nil || errUncached != nil {
		return sdk.Usage{}, errors.New("native usage counters are invalid")
	}
	if len(value.Input) == 0 {
		input = cached + written + uncached
	}
	if cached+written > input || input > 1<<52 {
		return sdk.Usage{}, errors.New("native usage cache split exceeds total input")
	}
	return sdk.Usage{InputTokens: input, OutputTokens: output, CachedTokens: cached, CacheWriteTokens: written}, nil
}
