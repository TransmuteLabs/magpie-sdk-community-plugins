package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type readPolicyTransport struct {
	client *SelectedClient
}

func (transport readPolicyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport.client.owner.request(request.Context(), transport.client.stream, request)
}

func (client *SelectedClient) readWithPolicy(ctx context.Context, path, target string, data []byte, headers http.Header, maxBytes int64) (ByteResponse, error) {
	step := "minimax-tool-catalog"
	if path == webSearchPath {
		step = "minimax-web-search"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return ByteResponse{}, err
	}
	request.Header = headers.Clone()
	var buffer ByteResponse
	response, _, err := sdk.RequestWithPolicy(ctx, client.stream, step, request, readPolicyTransport{client},
		func(response *http.Response) (sdk.RequestRecoveryRequest, error) {
			var err error
			buffer, err = bounded(response, maxBytes)
			response.Body = io.NopCloser(bytes.NewReader(buffer.Body))
			if err != nil {
				return sdk.RequestRecoveryRequest{}, err
			}
			if _, err := checkedJSON(buffer); err == nil || !readFailureComplete(buffer) {
				return sdk.RequestRecoveryRequest{}, nil
			}
			facts := redactFailureFacts(refusalFacts(buffer.Status, buffer.Header, buffer.Body, true), client.credentials)
			if !facts.BodyComplete || facts.CodePresent && !facts.CodeValid {
				return sdk.RequestRecoveryRequest{}, nil
			}
			projected := sdk.RequestFailureFacts{
				Status: facts.WireStatus, Code: facts.Code, Type: facts.Type, Message: facts.Message,
				Operation: client.call.Method, Phase: "response", Complete: true,
			}
			if facts.Timing.State == "delay" || facts.Timing.State == "reset" {
				projected.Headers = map[string]string{"Retry-After": facts.Timing.Raw}
				projected.ResetAt = facts.Timing.ResetAt
				if facts.Timing.DelayMS != nil && *facts.Timing.DelayMS <= 30*24*60*60*1000 {
					projected.RetryAfterMS = facts.Timing.DelayMS
				}
			}
			return sdk.RequestRecoveryRequest{
				Facts: projected, Evidence: "read-only",
				Rules: []sdk.RequestFailureRule{
					{ID: "minimax-read-content", Type: "content_policy_error", Class: "content-refusal", Scope: "request"},
					{ID: "minimax-read-request", Type: "invalid_request_error", Class: "request-refusal", Scope: "request"},
					{ID: "minimax-read-auth", Type: "authentication_error", Class: "auth-refusal", Scope: "account"},
					{ID: "minimax-read-credit-1008", Code: "1008", Class: "quota", Scope: "account"},
					{ID: "minimax-read-credit-2053", Code: "2053", Class: "quota", Scope: "account"},
					{ID: "minimax-read-permission", Type: "permission_error", Class: "request-refusal", Scope: "request"},
					{ID: "minimax-read-rate", Status: []int{429}, Class: "transient-rate", Scope: "request"},
					{ID: "minimax-read-outage", Status: []int{500, 502, 503, 504}, Class: "transient-outage", Scope: "request"},
				},
			}, nil
		})
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	return buffer, err
}

func readFailureComplete(response ByteResponse) bool {
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || len(response.Header.Values("Content-Type")) != 1 || contentType != "application/json" ||
		response.Status >= 300 && response.Status < 400 || len(response.Header.Values("Content-Encoding")) > 1 ||
		response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return false
	}
	envelope, valid := refusalObject(response.Body, "base_resp", "statusInfo", "code", "message", "msg", "error", "type", "request_id", "trace_id")
	if !valid {
		return false
	}
	statuses := 0
	for _, status := range []struct {
		field, code, message string
	}{
		{"base_resp", "status_code", "status_msg"},
		{"statusInfo", "code", "message"},
		{"error", "code", "message"},
	} {
		value, present := envelope[status.field]
		if !present {
			continue
		}
		statuses++
		allowed := []string{status.code, status.message}
		if status.field == "error" {
			allowed = append(allowed, "type")
		}
		fields, valid := refusalObject(value, allowed...)
		if !valid || fields[status.code] == nil || fields[status.message] == nil || !readStatusFields(fields, status.code) {
			return false
		}
	}
	if _, present := envelope["code"]; present {
		statuses++
		if envelope["message"] == nil && envelope["msg"] == nil {
			return false
		}
	}
	return statuses == 1 && readStatusFields(envelope, "code")
}

func readStatusFields(fields map[string]json.RawMessage, codeKey string) bool {
	for key, value := range fields {
		if key == codeKey {
			var code any
			if json.Unmarshal(value, &code) != nil {
				return false
			}
			if number, valid := apiCode(code); !valid || number == 0 {
				return false
			}
		} else if key != "base_resp" && key != "statusInfo" && key != "error" {
			var literal string
			if json.Unmarshal(value, &literal) != nil || bytes.Equal(value, []byte("null")) ||
				len(literal) > 1024 || key == "type" && len(literal) > 128 {
				return false
			}
		}
	}
	return len(fields) != 0
}
