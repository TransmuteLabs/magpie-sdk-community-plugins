package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func (owner *module) executeZen(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	identity, err := selectedZen(call)
	if err != nil {
		return nil, notStarted(err)
	}
	if identity.OAuth != nil && identity.OAuth.Due(owner.options.Now()) {
		return nil, failure(ErrRefreshRequired, "not-started")
	}
	var request sdk.ExecuteRequest
	var options struct {
		Method      string          `json:"method"`
		Path        string          `json:"path"`
		EscapedPath string          `json:"escapedPath"`
		Query       string          `json:"query"`
		Proxy       json.RawMessage `json:"proxy"`
		CallerID    string          `json:"callerId"`
		Native      json.RawMessage `json:"native"`
		RequestID   string          `json:"requestId"`
	}
	if decode(call.Params, &request) != nil || len(request.Options) != 0 && decode(request.Options, &options) != nil ||
		len(request.Credentials) != 0 && string(request.Credentials) != "null" ||
		request.Operation != "" || call.Scope.Operation == "" || options.Method != "" && options.Method != http.MethodPost ||
		options.EscapedPath != "" && options.EscapedPath != options.Path {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Zen requires a native POST and a host operation", Outcome: "not-started"}
	}
	if request.Protocol == "anthropic" {
		request.Protocol = "messages"
	}
	var body map[string]json.RawMessage
	if len(request.Body) > sdk.MaxNativeBodyBytes || json.Unmarshal(request.Body, &body) != nil || body == nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Zen requires a bounded native JSON object", Outcome: "not-started"}
	}
	var bodyModel string
	if request.Protocol != "gemini" || body["model"] != nil {
		if json.Unmarshal(body["model"], &bodyModel) != nil || bodyModel != request.Model {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Zen body model must match the selected model", Outcome: "not-started"}
		}
	}
	var requestedStream bool
	if body["stream"] != nil && json.Unmarshal(body["stream"], &requestedStream) != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Zen stream must be a boolean", Outcome: "not-started"}
	}
	endpoint := ""
	switch request.Protocol {
	case "chat", "responses", "messages":
		base, suffix := zenOpenAIBase, "/chat/completions"
		if request.Protocol == "responses" {
			suffix = "/responses"
		}
		if request.Protocol == "messages" {
			base, suffix = zenAnthropicBase, "/messages"
		}
		if options.Query == "" && (options.Path == "" || options.Path == suffix || options.Path == "/v1"+suffix) {
			endpoint = base + suffix
		}
	case "gemini":
		path := strings.TrimPrefix(options.Path, "/v1beta")
		switch path {
		case "", "/models/" + request.Model + ":generateContent":
			if options.Query == "" {
				endpoint = zenGeminiBase + "/models/" + url.PathEscape(request.Model) + ":generateContent"
			}
		case "/models/" + request.Model + ":streamGenerateContent":
			if options.Query == "alt=sse" {
				requestedStream = true
				endpoint = zenGeminiBase + "/models/" + url.PathEscape(request.Model) + ":streamGenerateContent?alt=sse"
			}
		}
	}
	if endpoint == "" {
		return nil, &sdk.Error{Code: "unsupported", Message: "Zen does not serve this native endpoint or query", Outcome: "not-started"}
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, notStarted(err)
	}
	var tools *zenToolPolicy
	if identity.Kind == "public" && request.Protocol != "gemini" {
		request.Body, err = prepareZenPublic(string(request.Protocol), request.Body, state.config.PublicEnvironment)
		if err == nil {
			request.Body, tools, err = prepareZenTools(string(request.Protocol), request.Body)
		}
		if err != nil {
			return nil, &sdk.Error{Code: "invalid_params", Message: err.Error(), Outcome: "not-started"}
		}
	}
	var gemini geminiTerminal
	if request.Protocol == "gemini" {
		if err := gemini.configure(body); err != nil {
			return nil, notStarted(err)
		}
	}
	headers := request.Headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	project := state.config.ProjectID
	if project == "" {
		project = headers.Get("x-opencode-project")
	}
	if err := state.headers.Stamp(headers, call.Scope.Account, options.CallerID, call.Scope.Session, project, ClientVersion, owner.options.Now()); err != nil {
		return nil, notStarted(err)
	}
	models, err := owner.discoverZen(ctx, call, stream, identity)
	if err != nil {
		return nil, notStarted(err)
	}
	if !slices.ContainsFunc(models, func(model sdk.Model) bool {
		return model.ID == request.Model && slices.ContainsFunc(model.Protocols, func(protocol protocolext.Protocol) bool { return string(protocol) == request.Protocol })
	}) {
		return nil, &sdk.Error{Code: "unsupported", Message: "Zen model is absent, retired, paid for public access, or uses another native protocol", Outcome: "not-started"}
	}
	ClearAuth(headers)
	var anthropicVersion, anthropicBeta string
	for key, values := range headers {
		if strings.EqualFold(key, "anthropic-version") {
			anthropicVersion = strings.Join(values, ", ")
		}
		if strings.EqualFold(key, "anthropic-beta") {
			anthropicBeta = strings.Join(values, ", ")
		}
		if strings.EqualFold(key, "Accept-Encoding") || strings.EqualFold(key, "anthropic-version") || strings.EqualFold(key, "anthropic-beta") {
			delete(headers, key)
		}
	}
	switch request.Protocol {
	case "messages":
		headers.Set("x-api-key", identity.access())
		if anthropicVersion == "" {
			anthropicVersion = "2023-06-01"
		}
		headers.Set("anthropic-version", anthropicVersion)
		if anthropicBeta != "" {
			headers.Set("anthropic-beta", anthropicBeta)
		}
	case "gemini":
		headers.Set("x-goog-api-key", identity.access())
	default:
		headers.Set("Authorization", "Bearer "+identity.access())
	}
	if identity.organization() != "" {
		headers.Set("x-opencode-org-id", identity.organization())
	}
	headers.Set("Content-Type", "application/json")
	if tools != nil {
		headers.Del("Content-Length")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, owner.options.Endpoints.Inference+endpoint, bytes.NewReader(request.Body))
	if err != nil {
		return nil, notStarted(err)
	}
	req.Header = headers
	response, err := owner.client(stream).HTTP.Do(req)
	if err != nil {
		if emitErr := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); emitErr != nil {
			return nil, emitErr
		}
		return nil, failure(err, "uncertain")
	}
	defer response.Body.Close()
	if refused, proved, err := executionRefusal(ctx, stream, call.Scope, response, identity.access()); err != nil {
		return nil, err
	} else if proved {
		return refused, nil
	}
	success := response.StatusCode >= 200 && response.StatusCode < 300
	contentType, _, _ := strings.Cut(response.Header.Get("Content-Type"), ";")
	streaming := success && strings.EqualFold(strings.TrimSpace(contentType), "text/event-stream")
	if success && requestedStream && !streaming {
		return nil, &sdk.Error{Code: "truncated", Message: "Zen streaming returned a non-SSE response", Outcome: "uncertain"}
	}
	if tools != nil && success {
		response.Header.Del("Content-Length")
		response.Header.Del("Content-Encoding")
		if streaming && !requestedStream {
			body, err := collapseZenReply(ctx, response.Body, tools, string(request.Protocol))
			if err != nil {
				if errors.Is(err, errZenToolsTruncated) {
					return nil, &sdk.Error{Code: "truncated", Message: err.Error(), Outcome: "uncertain"}
				}
				return nil, failure(err, "uncertain")
			}
			response.Header.Set("Content-Type", "application/json")
			if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: response.StatusCode, Headers: response.Header}); err != nil {
				return nil, err
			}
			if err := emitResponseData(ctx, stream, body); err != nil {
				return nil, err
			}
			return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed"}, nil
		}
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: response.StatusCode, Headers: response.Header}); err != nil {
		return nil, err
	}
	if streaming && request.Protocol == "gemini" {
		if err := gemini.relay(ctx, response.Body, stream); err != nil {
			return nil, err
		}
		return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed"}, nil
	}
	if tools != nil && success {
		if err := relayZenTools(ctx, response.Body, tools, string(request.Protocol), streaming, func(body []byte) error {
			return emitResponseData(ctx, stream, body)
		}); err != nil {
			if errors.Is(err, errZenToolsTruncated) {
				return nil, &sdk.Error{Code: "truncated", Message: err.Error(), Outcome: "uncertain"}
			}
			return nil, failure(err, "uncertain")
		}
		return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed"}, nil
	}
	terminal := zenTerminal{protocol: string(request.Protocol)}
	buffer := make([]byte, sdk.MaxChunkBytes)
	for {
		count, readErr := response.Body.Read(buffer)
		if count != 0 {
			if streaming {
				if err := terminal.write(buffer[:count]); err != nil {
					return nil, failure(err, "uncertain")
				}
			}
			if err := stream.Emit(ctx, "data", buffer[:count]); err != nil {
				return nil, err
			}
			if streaming && terminal.complete() {
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, failure(readErr, "uncertain")
		}
	}
	if streaming && !terminal.complete() {
		return nil, &sdk.Error{Code: "truncated", Message: "Zen stream ended without its native terminal event", Outcome: "uncertain"}
	}
	return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed"}, nil
}
