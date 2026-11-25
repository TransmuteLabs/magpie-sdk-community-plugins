package kimiwork

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

var spentQuotaPattern = regexp.MustCompile(`(?i)resource_exhausted|usage limit|billing cycle|insufficient balance|quota`)

func IsSpentQuota(text string) bool {
	return spentQuotaPattern.MatchString(text)
}

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ExecuteRequest
	if decodeObject(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, invalid("Execution uses the selected host frame credentials")
	}
	if request.Protocol != "chat" || len(request.Body) == 0 || len(request.Body) > sdk.MaxNativeBodyBytes {
		return nil, invalid("Kimi Work requires a bounded Chat Completions request")
	}
	var parsedBody map[string]any
	if json.Unmarshal(request.Body, &parsedBody) != nil || parsedBody == nil {
		return nil, invalid("Kimi Work requires a JSON Chat Completions body")
	}
	bodyModel, _ := parsedBody["model"].(string)
	if request.Model == "" || bodyModel != request.Model {
		return nil, invalid("Chat model must match the selected model")
	}
	if call.Scope.Provider != ProviderID || call.Scope.Account == "" || call.Scope.Generation == 0 {
		return nil, denied("An exact selected Kimi Work account is required")
	}
	var creds Credentials
	if decodeObject(call.Auth, &creds) != nil || !creds.Valid() {
		return nil, denied("Selected Kimi Work credentials do not match the call")
	}
	var options sdk.ExecuteOptions
	if len(request.Options) > 0 && json.Unmarshal(request.Options, &options) != nil {
		return nil, invalid("Invalid execute options")
	}
	if options.Method != "" && options.Method != http.MethodPost ||
		options.Path != "" && options.Path != "/chat/completions" && options.Path != "/v1/chat/completions" ||
		options.Query != "" {
		return nil, invalid("Kimi Work only executes /chat/completions")
	}
	session, err := owner.currentSession(creds)
	if err != nil {
		var deskErr *DesktopError
		if errors.As(err, &deskErr) && deskErr.SignIn == "expired" {
			return nil, unauthenticated(deskErr.Message)
		}
		return nil, &sdk.Error{Code: "kimi_work_unavailable", Message: err.Error(), Outcome: "not-started"}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	outboundBody := MoonshotTools(request.Body)
	var fast bool
	if asksFast(outboundBody) {
		offered := owner.fastOffered(ctx, stream, session, creds)
		outboundBody, fast = FastSpeed(outboundBody, offered)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	headers := http.Header{}
	for key, vals := range request.Headers {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "x-api-key" || lower == "cookie" ||
			lower == "host" || lower == "content-length" || lower == "origin" ||
			strings.HasPrefix(lower, "x-magpie-") {
			continue
		}
		headers[key] = append([]string(nil), vals...)
	}
	for key, vals := range owner.workHeaders(session.DeviceID) {
		headers[key] = vals
	}
	headers.Set("Authorization", "Bearer "+session.WorkToken)
	headers.Set("Content-Type", "application/json")
	if fast {
		headers.Set(SpeedHeader, "fast")
	}
	endpointURL := strings.TrimRight(owner.options.Endpoints.Base, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(outboundBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header = headers
	resp, err := owner.doHTTP(ctx, stream, httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &sdk.Error{
			Code:    "kimi_work_connection_failed",
			Message: "Kimi Work 连接失败：" + err.Error(),
			Outcome: "uncertain",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if session.Away {
			return nil, unauthenticated(notHereError(true).Message)
		}
		return nil, unauthenticated(expiredError("", false).Message)
	}

	outHeaders := http.Header{}
	for _, key := range []string{"Content-Type", "Retry-After", "X-Request-Id", "Request-Id", "Content-Encoding"} {
		if val := resp.Header.Get(key); val != "" {
			outHeaders.Set(key, val)
		}
	}

	if resp.StatusCode == http.StatusForbidden {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes+1))
		if readErr != nil || len(bodyBytes) > sdk.MaxNativeBodyBytes {
			return nil, uncertain("Kimi Work 403 refusal was incomplete")
		}
		bodyText := string(bodyBytes)
		if IsSpentQuota(bodyText) {
			refusalHeaders := map[string]string{}
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				refusalHeaders["Retry-After"] = ra
			}
			return sdk.RefusedExecution(sdk.RequestRecoveryRequest{
				Step:     "kimi-work-chat",
				Evidence: "rejected-before-effect",
				Facts: sdk.RequestFailureFacts{
					Status:    http.StatusTooManyRequests,
					Code:      "kimi_work_quota_exhausted",
					Message:   truncateText(strings.TrimSpace(bodyText), 1024),
					Headers:   refusalHeaders,
					Operation: sdk.MethodProviderExecute,
					Phase:     "response",
					Complete:  true,
				},
				Rules: []sdk.RequestFailureRule{
					{
						ID:     "kimi-work-spent-quota",
						Status: []int{http.StatusTooManyRequests},
						Code:   "kimi_work_quota_exhausted",
						Class:  "quota",
						Scope:  "account",
					},
				},
			}, nil), nil
		}
		outHeaders.Del("Content-Encoding")
		outHeaders.Del("Content-Length")
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return nil, err
		}
		if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: http.StatusForbidden, Headers: outHeaders}); err != nil {
			return nil, err
		}
		if len(bodyBytes) > 0 {
			if err := stream.Emit(ctx, "data", bodyBytes); err != nil {
				return nil, err
			}
		}
		return sdk.ExecuteResult{Status: http.StatusForbidden, Headers: outHeaders, Outcome: "completed"}, nil
	}

	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: resp.StatusCode, Headers: outHeaders}); err != nil {
		return nil, err
	}
	var usage *sdk.Usage
	isSSE := strings.Contains(strings.ToLower(outHeaders.Get("Content-Type")), "text/event-stream")
	var jsonAccum bytes.Buffer
	var sseLineBuf bytes.Buffer
	buf := make([]byte, 32*1024)
	for {
		count, readErr := resp.Body.Read(buf)
		if count > 0 {
			chunk := buf[:count]
			if err := stream.Emit(ctx, "data", chunk); err != nil {
				return nil, err
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if isSSE {
					if parsed := extractSSEUsage(&sseLineBuf, chunk); parsed != nil {
						usage = parsed
						if err := stream.Emit(ctx, "usage", *usage); err != nil {
							return nil, err
						}
					}
				} else if jsonAccum.Len()+len(chunk) <= 4*1024*1024 {
					jsonAccum.Write(chunk)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, uncertain("Kimi Work stream ended unexpectedly")
		}
	}
	if !isSSE && resp.StatusCode >= 200 && resp.StatusCode < 300 && jsonAccum.Len() > 0 {
		if parsed := extractChatUsage(jsonAccum.Bytes()); parsed != nil {
			usage = parsed
			_ = stream.Emit(ctx, "usage", *usage)
		}
	}
	return sdk.ExecuteResult{
		Status:  resp.StatusCode,
		Headers: outHeaders,
		Usage:   usage,
		Outcome: "completed",
	}, nil
}

func extractSSEUsage(buffer *bytes.Buffer, chunk []byte) *sdk.Usage {
	buffer.Write(chunk)
	var latest *sdk.Usage
	for {
		data := buffer.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(string(data[:idx]))
		buffer.Next(idx + 1)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if u := extractChatUsage([]byte(payload)); u != nil {
			latest = u
		}
	}
	return latest
}

func extractChatUsage(raw []byte) *sdk.Usage {
	var doc struct {
		Usage *struct {
			PromptTokens        int64 `json:"prompt_tokens"`
			CompletionTokens    int64 `json:"completion_tokens"`
			CachedTokens        int64 `json:"cached_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Usage == nil {
		return nil
	}
	cached := doc.Usage.CachedTokens
	if cached == 0 && doc.Usage.PromptTokensDetails != nil {
		cached = doc.Usage.PromptTokensDetails.CachedTokens
	}
	return &sdk.Usage{
		InputTokens:  doc.Usage.PromptTokens,
		OutputTokens: doc.Usage.CompletionTokens,
		CachedTokens: cached,
	}
}

func truncateText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	scanner := bufio.NewScanner(strings.NewReader(text[:limit]))
	if scanner.Scan() {
		return scanner.Text()
	}
	return text[:limit]
}
