package commandcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func client(proxy string) (*http.Client, func(), error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 90 * time.Second
	if proxy != "" {
		location, err := url.Parse(proxy)
		if err != nil || location.Host == "" || location.Scheme != "http" && location.Scheme != "https" && location.Scheme != "socks5" && location.Scheme != "socks5h" {
			return nil, nil, invalid("invalid host-selected network proxy")
		}
		transport.Proxy = http.ProxyURL(location)
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, transport.CloseIdleConnections, nil
}

func (controller *controller) proxy(ctx context.Context, stream *sdk.Stream, target string) (string, error) {
	var result sdk.NetworkProxyResult
	if stream == nil {
		return "", errors.New("network proxy requires the owning SDK call")
	}
	err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: target}, &result)
	return result.URL, err
}

func (controller *controller) read(ctx context.Context, stream *sdk.Stream, endpoint, key string, policy ...bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	target := strings.TrimRight(controller.config.APIURL, "/") + endpoint
	proxy, err := controller.proxy(ctx, stream, target)
	if err != nil {
		return nil, err
	}
	client, closeClient, err := client(proxy)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "node")
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("x-api-key", key)
	}
	var response *http.Response
	if len(policy) > 0 && policy[0] {
		response, _, err = sdk.RequestWithPolicy(ctx, stream, "commandcode.subscription", request, client.Transport,
			func(response *http.Response) (sdk.RequestRecoveryRequest, error) {
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					return sdk.RequestRecoveryRequest{}, nil
				}
				body, err := bounded(response.Body, 64<<10)
				response.Body.Close()
				response.Body = io.NopCloser(bytes.NewReader(body))
				if err != nil {
					return sdk.RequestRecoveryRequest{}, err
				}
				facts, _ := responseFailure(response.StatusCode, response.Header, body, key)
				return sdk.RequestRecoveryRequest{Facts: facts, Rules: commandFailureRules(), Evidence: "read-only"}, nil
			})
	} else {
		response, err = client.Do(request)
	}
	if err != nil {
		return nil, &sdk.Error{Code: "unavailable", Message: "CommandCode account endpoint is unavailable"}
	}
	return readAllBounded(response, 4<<20)
}

func bounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("body is unreadable or oversized")
	}
	return body, nil
}

func execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	auth, err := account(call)
	if err != nil {
		return nil, err
	}
	var input sdk.ExecuteRequest
	if decode(call.Params, &input) != nil || input.Model == "" || len(input.Body) > 16<<20 {
		return nil, invalid("invalid CommandCode execution request")
	}
	if input.Operation == sdk.OperationSystemOne {
		return controller.systemOne(ctx, call, stream, auth, input)
	}
	if isDecision(input.Model) || input.Operation != "" {
		return nil, &sdk.Error{Code: "unnegotiated_capability", Message: "Jev requires its systemone operation; other non-conversational operations are not declared", Outcome: "not-started"}
	}
	var options sdk.ExecuteOptions
	if len(input.Options) > 0 && decode(input.Options, &options) != nil {
		return nil, invalid("invalid CommandCode execution options")
	}
	if options.Method != "" && options.Method != "POST" {
		return nil, invalid("CommandCode inference requires POST")
	}
	var fields map[string]json.RawMessage
	if decode(input.Body, &fields) != nil || fields == nil {
		return nil, invalid("CommandCode requires an inference JSON object")
	}
	var model string
	if json.Unmarshal(fields["model"], &model) != nil || model != input.Model {
		return nil, invalid("CommandCode body model differs from selected model")
	}
	var streaming bool
	if raw, found := fields["stream"]; found && json.Unmarshal(raw, &streaming) != nil {
		return nil, invalid("invalid stream flag")
	}
	body := bytes.Clone(input.Body)
	endpoint := ""
	switch input.Protocol {
	case "chat":
		endpoint = "/provider/v1/chat/completions"
	case "responses":
		endpoint = "/provider/v1/responses"
	case "anthropic":
		endpoint = "/provider/v1/messages"
	default:
		return nil, invalid("CommandCode protocol is not declared")
	}
	requestedPath := strings.TrimPrefix(options.Path, "/v1")
	countTokens := input.Protocol == "anthropic" && requestedPath == "/messages/count_tokens"
	if countTokens {
		endpoint += "/count_tokens"
		if streaming {
			return nil, invalid("CommandCode token counting does not stream")
		}
	}
	if options.Path != "" && requestedPath != strings.TrimPrefix(endpoint, "/provider/v1") {
		return nil, invalid("CommandCode request path does not match its protocol or supported operation")
	}
	sub, err := controller.subscription(ctx, stream, auth.Key, true)
	if err != nil {
		return nil, err
	}
	goPlan := sub.Plan == "Go"
	if goPlan {
		if countTokens {
			return nil, &sdk.Error{Code: "unnegotiated_capability", Message: "CommandCode Go-plan token counting is not supported", Outcome: "not-started"}
		}
		if input.Protocol != "chat" {
			return nil, invalid("CommandCode Go-plan native adapter supports Chat only")
		}
		place := Place{Directory: controller.config.WorkingDirectory, Date: time.Now().Format("2006-01-02"), Environment: runtime.GOOS}
		if place.Environment == "windows" {
			place.Environment = "win32"
		}
		body, err = GenerateBody(input.Body, place)
		if err != nil {
			return nil, invalid(err.Error())
		}
		endpoint = "/alpha/generate"
	} else if input.Protocol == "chat" {
		body, err = providerBody(input.Body, input.Model)
		if err != nil {
			return nil, invalid(err.Error())
		}
	}
	target := strings.TrimRight(controller.config.APIURL, "/") + endpoint
	networkProxy := options.Proxy
	if networkProxy == "" {
		networkProxy, err = controller.proxy(ctx, stream, target)
		if err != nil {
			return nil, err
		}
	}
	httpClient, closeClient, err := client(networkProxy)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	request, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(body))
	if err != nil {
		return nil, invalid("invalid CommandCode inference URL")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+auth.Key)
	request.Header.Set("x-api-key", auth.Key)
	if input.Protocol == "anthropic" {
		request.Header.Set("anthropic-version", "2023-06-01")
		if beta := input.Headers.Get("anthropic-beta"); beta != "" {
			request.Header.Set("anthropic-beta", beta)
		}
	}
	if controller.config.ZDR {
		request.Header.Set("x-cmd-zdr", "1")
	}
	if goPlan {
		request.Header.Set("User-Agent", "cli")
		request.Header.Set("x-command-code-version", cliVersion)
		request.Header.Set("x-cli-environment", "production")
		request.Header.Set("x-project-slug", url.PathEscape(path.Base(strings.ReplaceAll(controller.config.WorkingDirectory, "\\", "/"))))
		request.Header.Set("x-taste-learning", "false")
		request.Header.Set("x-session-id", controller.session)
	}
	accept := func() error {
		return stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation})
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if emitErr := accept(); emitErr != nil {
			return nil, emitErr
		}
		return nil, &sdk.Error{Code: "upstream_error", Message: "CommandCode request outcome is uncertain; it was not replayed", Outcome: "uncertain"}
	}
	defer response.Body.Close()
	status := response.StatusCode
	headers := responseHeaders(response.Header)
	headers.Set("X-Magpie-Sign-In", "kept")
	if status == 401 {
		if controller.keyRefused(ctx, stream, auth.Key) {
			headers.Set("X-Magpie-Sign-In", "expired")
		}
	}
	emitData := func(data []byte) error {
		for len(data) > 0 {
			count := min(len(data), sdk.MaxChunkBytes)
			if err := stream.Emit(ctx, "data", data[:count]); err != nil {
				return err
			}
			data = data[count:]
		}
		return stream.Emit(ctx, "flush", nil)
	}
	if status < 200 || status >= 300 {
		errorBody, err := bounded(response.Body, 1<<20)
		if err == nil {
			if refused, proved := commandRefusal(status, response.Header, errorBody, auth.Key, "commandcode.execute"); proved {
				return refused, nil
			}
		}
		if emitErr := accept(); emitErr != nil {
			return nil, emitErr
		}
		if err != nil {
			return nil, &sdk.Error{Code: "upstream_error", Message: "CommandCode refused with an unreadable response", Outcome: "uncertain"}
		}
		status = refusalStatus(status, errorBody)
		if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: headers}); err != nil {
			return nil, err
		}
		if err := emitData(errorBody); err != nil {
			return nil, err
		}
		return sdk.ExecuteResult{Status: status, Headers: headers, Outcome: "not-started"}, nil
	}
	if err := accept(); err != nil {
		return nil, err
	}
	if goPlan {
		headers.Set("Content-Type", "application/json")
		if streaming {
			headers.Set("Content-Type", "text/event-stream")
		}
	}
	if goPlan {
		sentHeaders := false
		send := func(body []byte) error {
			if !sentHeaders {
				if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: headers}); err != nil {
					return err
				}
				sentHeaders = true
			}
			return emitData(body)
		}
		resume := func() (io.ReadCloser, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			response.Body.Close()
			next := request.Clone(ctx)
			body, err := request.GetBody()
			if err != nil {
				return nil, errors.New("native continuation could not reopen its original body")
			}
			next.Body = body
			answer, err := httpClient.Do(next)
			if err != nil {
				return nil, errors.New("native continuation was interrupted; turn outcome is uncertain")
			}
			if answer.StatusCode < 200 || answer.StatusCode >= 300 {
				answer.Body.Close()
				return nil, fmt.Errorf("native continuation refused with upstream status %d; turn outcome is uncertain", answer.StatusCode)
			}
			return answer.Body, nil
		}
		counts, err := translateGenerate(response.Body, input.Model, streaming, send, resume)
		if err != nil {
			var failure *NativeFailure
			if !sentHeaders && errors.As(err, &failure) && failure.NotStarted {
				headers.Set("Content-Type", "application/json")
				errorBody, _ := json.Marshal(map[string]any{"error": map[string]any{"message": failure.Message, "type": failure.Type, "code": failure.Status}})
				if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: failure.Status, Headers: headers}); err != nil {
					return nil, err
				}
				if err := emitData(errorBody); err != nil {
					return nil, err
				}
				return sdk.ExecuteResult{Status: failure.Status, Headers: headers, Outcome: "not-started"}, nil
			}
			return nil, &sdk.Error{Code: "upstream_error", Message: err.Error(), Outcome: "uncertain"}
		}
		counts.InputTokens -= counts.CachedTokens + counts.CacheWriteTokens
		return sdk.ExecuteResult{Status: status, Headers: headers, Usage: &counts, Outcome: "completed"}, nil
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: headers}); err != nil {
		return nil, err
	}
	var completion *providerStreamCompletion
	mediaType, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	if streaming || mediaType == "text/event-stream" {
		completion = &providerStreamCompletion{protocol: input.Protocol}
	}
	buffer := make([]byte, sdk.MaxChunkBytes)
	total := 0
	for {
		count, readErr := response.Body.Read(buffer)
		total += count
		if total > 64<<20 {
			return nil, &sdk.Error{Code: "resource_exhausted", Message: "CommandCode provider response exceeded its limit", Outcome: "uncertain"}
		}
		if count > 0 {
			if completion != nil {
				completion.write(buffer[:count])
			}
			if err := emitData(buffer[:count]); err != nil {
				return nil, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, &sdk.Error{Code: "upstream_error", Message: "CommandCode provider stream was interrupted", Outcome: "uncertain"}
		}
	}
	if completion != nil {
		if err := completion.finish(); err != nil {
			return nil, &sdk.Error{Code: "upstream_error", Message: err.Error(), Outcome: "uncertain"}
		}
	}
	return sdk.ExecuteResult{Status: status, Headers: headers, Outcome: "completed"}, nil
}

func responseHeaders(source http.Header) http.Header {
	headers := make(http.Header)
	for _, name := range []string{"Content-Type", "Retry-After", "Request-Id", "X-Request-Id"} {
		for _, value := range source.Values(name) {
			headers.Add(name, value)
		}
	}
	return headers
}

func refusalStatus(status int, body []byte) int {
	text := strings.ToLower(string(body))
	if strings.Contains(text, "model_not_in_plan") {
		return 403
	}
	if strings.Contains(text, "premium_credits_exhausted") || strings.Contains(text, "insufficient credits") {
		return 402
	}
	if (status == 500 || status < 400) && (strings.Contains(text, "rate_limited") || strings.Contains(text, "window_limit") || strings.Contains(text, "usage limit")) {
		return 429
	}
	return status
}

func providerBody(body []byte, model string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if decode(body, &fields) != nil {
		return nil, errors.New("invalid provider body")
	}
	if raw, found := fields["tools"]; found && isGoogle(model) && string(raw) != "null" {
		var tools []map[string]json.RawMessage
		if decode(raw, &tools) != nil {
			return nil, errors.New("invalid Google tool declarations")
		}
		for _, tool := range tools {
			if string(tool["type"]) != `"function"` {
				continue
			}
			var function map[string]json.RawMessage
			if decode(tool["function"], &function) != nil || function == nil {
				return nil, errors.New("invalid Google function tool")
			}
			parameters, found := function["parameters"]
			if !found {
				continue
			}
			var schema map[string]any
			if decode(parameters, &schema) != nil || schema == nil {
				return nil, errors.New("Google tool schema must be a JSON object")
			}
			normalized, err := googleSchema(schema, false)
			if err != nil {
				return nil, err
			}
			function["parameters"], _ = json.Marshal(normalized)
			tool["function"], _ = json.Marshal(function)
		}
		fields["tools"], _ = json.Marshal(tools)
	}
	if effort, found := fields["reasoning_effort"]; found {
		var name string
		if json.Unmarshal(effort, &name) != nil {
			return nil, errors.New("invalid reasoning effort")
		}
		if name == "none" || name == "off" {
			delete(fields, "reasoning_effort")
			if thinkOff[model] {
				fields["reasoning_effort"] = json.RawMessage(`"off"`)
			}
		}
	}
	if strings.HasPrefix(strings.ToLower(model), "reflection/beam") {
		var messages []map[string]json.RawMessage
		if decode(fields["messages"], &messages) != nil {
			return nil, errors.New("invalid provider history")
		}
		for _, message := range messages {
			if string(message["role"]) != `"assistant"` {
				continue
			}
			var thought string
			json.Unmarshal(message["reasoning_content"], &thought)
			if strings.TrimSpace(thought) == "" {
				json.Unmarshal(message["reasoning"], &thought)
				if strings.TrimSpace(thought) == "" {
					thought = "."
				}
				message["reasoning_content"], _ = json.Marshal(thought)
			}
		}
		fields["messages"], _ = json.Marshal(messages)
	}
	return json.Marshal(fields)
}
