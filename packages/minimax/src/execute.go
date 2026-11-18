package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ExecuteRequest
	if decode(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, invalid("Execution uses the selected host frame credentials")
	}
	if request.Operation != "" {
		return owner.executeMedia(ctx, call, stream, request)
	}
	if request.Protocol != "anthropic" && request.Protocol != "messages" || len(request.Body) > sdk.MaxNativeBodyBytes {
		return nil, invalid("MiniMax chat requires a bounded Messages request")
	}
	var message map[string]any
	if json.Unmarshal(request.Body, &message) != nil || message == nil || text(message["model"]) != request.Model || request.Model == "" {
		return nil, invalid("Messages model must match the selected model")
	}
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, err
	}
	var options sdk.ExecuteOptions
	if len(request.Options) > 0 && json.Unmarshal(request.Options, &options) != nil {
		return nil, invalid("Invalid execute options")
	}
	if options.Method != "" && options.Method != "POST" || options.Path != "" && options.Path != "/v1/messages" && options.Path != "/messages" || options.Query != "" {
		return nil, invalid("MiniMax only executes the Messages endpoint")
	}
	headers := http.Header{
		"Authorization": {"Bearer " + client.credentials.Access},
		"X-Api-Key":     {"sk-xxx"}, "User-Agent": {"MiniMaxAgent"},
		"X-Mavis-Agent-Id":        {"main"},
		"X-Mavis-Timezone-Offset": {strconv.Itoa(owner.options.TimezoneOffset())},
		"Content-Type":            {"application/json"}, "Accept-Encoding": {"identity"},
		"Anthropic-Version": {"2023-06-01"},
	}
	for _, key := range []string{"Anthropic-Beta", "Anthropic-Version"} {
		if value := request.Headers.Get(key); value != "" {
			headers.Set(key, value)
		}
	}
	session := call.Scope.Session
	if session == "" {
		session = owner.session
	}
	headers.Set("X-Mavis-Session-Id", session)
	response, err := owner.siteRequest(ctx, stream, "POST", client.site.LLM+"/mavis/api/v1/llm/v1/messages", request.Body, headers)
	if err != nil {
		return nil, uncertain("MiniMax chat failed after dispatch; no retry was made")
	}
	defer response.Body.Close()
	_, redirected := response.Body.(*redirectedSiteBody)
	status := response.StatusCode
	outHeaders := http.Header{"X-Magpie-Sign-In": {"kept"}}
	for _, key := range []string{"Content-Type", "Retry-After", "X-Request-Id", "Request-Id", "Content-Encoding"} {
		if value := response.Header.Get(key); value != "" {
			outHeaders.Set(key, value)
		}
	}
	var buffered []byte
	jsonResponse := strings.Contains(outHeaders.Get("Content-Type"), "application/json")
	if status >= 400 || status >= 200 && status < 300 && jsonResponse {
		buffered, err = io.ReadAll(io.LimitReader(response.Body, sdk.MaxNativeBodyBytes+1))
		if err != nil || len(buffered) > sdk.MaxNativeBodyBytes {
			facts := refusalFacts(status, response.Header, nil, true)
			facts.BodyComplete = false
			return nil, client.selectedFailure(withFailureDetails(uncertain("MiniMax refusal was incomplete"), facts))
		}
		original := ByteResponse{Status: status, Header: response.Header, Body: buffered}
		prepared := prepareChatResponse(original, outHeaders, client.credentials)
		if jsonResponse && !redirected {
			if refused, proved := chatRefusal(original, prepared.Failure); proved {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return refused, nil
			}
		}
		status, outHeaders, buffered = prepared.Status, prepared.Header, prepared.Body
	}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: outHeaders}); err != nil {
		return nil, err
	}
	reader := io.Reader(response.Body)
	if buffered != nil {
		reader = bytes.NewReader(buffered)
	}
	observer := usageObserver{sse: strings.Contains(outHeaders.Get("Content-Type"), "text/event-stream"), wireStatus: response.StatusCode, headers: response.Header}
	chunk := make([]byte, 32<<10)
	for {
		count, readErr := reader.Read(chunk)
		if count > 0 {
			if err := stream.Emit(ctx, "data", chunk[:count]); err != nil {
				return nil, err
			}
			if status < 400 && observer.feed(chunk[:count]) {
				if err := stream.Emit(ctx, "usage", observer.usage); err != nil {
					return nil, err
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			failure := uncertain("MiniMax response ended before transport completion")
			if observer.failure != nil {
				facts := *observer.failure
				facts.BodyComplete = false
				failure = client.selectedFailure(withFailureDetails(failure, facts))
			}
			return nil, failure
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if !observer.sse && status < 400 {
		observer.observe(observer.pending)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if status < 400 && observer.sse && (!observer.terminal || observer.failed) {
		failure := uncertain("MiniMax stream ended without a successful message boundary")
		if observer.failure != nil {
			failure = client.selectedFailure(withFailureDetails(failure, *observer.failure))
		}
		return nil, failure
	}
	result := sdk.ExecuteResult{Status: status, Headers: outHeaders, Outcome: "completed"}
	if observer.known {
		result.Usage = &observer.usage
	}
	return result, nil
}

type usageObserver struct {
	sse, known bool
	pending    []byte
	eventData  []byte
	usage      sdk.Usage
	oversized  bool
	terminal   bool
	failed     bool
	wireStatus int
	headers    http.Header
	failure    *FailureFacts
}

func (observer *usageObserver) feed(chunk []byte) bool {
	if observer.oversized && !observer.sse {
		return false
	}
	observer.pending = append(observer.pending, chunk...)
	if len(observer.pending) > 1<<20 {
		observer.oversized = true
		observer.pending = nil
		observer.failed = observer.sse
		return false
	}
	if !observer.sse {
		return false
	}
	changed := false
	for {
		at := bytes.IndexByte(observer.pending, '\n')
		if at < 0 {
			break
		}
		line := bytes.TrimSuffix(observer.pending[:at], []byte("\r"))
		observer.pending = observer.pending[at+1:]
		if len(line) == 0 {
			if !observer.oversized && len(observer.eventData) > 0 {
				changed = observer.observe(observer.eventData) || changed
			}
			observer.eventData = nil
			observer.oversized = false
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			payload := bytes.TrimPrefix(line[5:], []byte(" "))
			if len(observer.eventData)+len(payload)+1 > 1<<20 {
				observer.oversized = true
				observer.eventData = nil
			} else if !observer.oversized {
				observer.eventData = append(observer.eventData, payload...)
				observer.eventData = append(observer.eventData, '\n')
			}
		}
	}
	return changed
}
func (observer *usageObserver) observe(body []byte) bool {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return false
	}
	if root["type"] == "error" {
		observer.failed = true
		facts := refusalFacts(observer.wireStatus, observer.headers, body, true)
		facts.Origin, facts.OutputStarted = "sse-event", true
		observer.failure = &facts
	}
	if root["type"] == "message_stop" {
		observer.terminal = true
	}
	if observer.usage.Facts == nil {
		observer.usage.Facts = &sdk.ExecutionUsageFacts{}
	}
	facts := observer.usage.Facts
	message := object(root["message"])
	if message == nil {
		message = root
	}
	changed := false
	for _, field := range []struct {
		value  any
		target **string
	}{
		{message["model"], &facts.ServedModel},
		{message["id"], &facts.ResponseID},
		{message["stop_reason"], &facts.Stop},
		{object(root["delta"])["stop_reason"], &facts.Stop},
	} {
		literal, valid := field.value.(string)
		if valid && len(literal) <= 300 && !strings.ContainsFunc(literal, unicode.IsControl) {
			*field.target = &literal
			changed = true
		}
	}
	usage := object(root["usage"])
	if usage == nil {
		usage = object(object(root["message"])["usage"])
	}
	for _, field := range []struct {
		name   string
		target **int64
	}{
		{"input_tokens", &facts.InputTokens},
		{"output_tokens", &facts.OutputTokens},
		{"cache_read_input_tokens", &facts.CachedTokens},
		{"cache_creation_input_tokens", &facts.CacheWriteTokens},
	} {
		if count, ok := number(usage[field.name]); ok && count >= 0 && count < 1<<53 && count == float64(int64(count)) {
			amount := int64(count)
			*field.target = &amount
			changed = true
		}
	}
	observer.known = observer.known || changed
	return changed
}
