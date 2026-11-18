package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestFailureFactsChatKeepsWireAndEffectiveStatus(test *testing.T) {
	for _, fixture := range []struct {
		name      string
		status    int
		body      string
		effective int
		code      string
	}{
		{"business_credit", 200, `{"base_resp":{"status_code":1008,"status_msg":"积分余额不足"}}`, 429, "1008"},
		{"permission", 403, `{"error":{"type":"permission_error","message":"insufficient permissions"}}`, 403, ""},
		{"permission_credit", 403, `{"error":{"type":"permission_error","message":"积分余额不足"}}`, 429, ""},
		{"content", 403, `{"error":{"type":"content_policy_error","message":"quota exceeded"}}`, 403, ""},
		{"unknown", 200, `{"code":2013,"message":"vendor failure"}`, 502, "2013"},
		{"malformed", 200, `{"base_resp":{"status_code":1008.5,"status_msg":"quota exceeded"}}`, 502, ""},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			headers := http.Header{"Content-Type": {"application/json"}, "Retry-After": {"7200"}}
			response := ByteResponse{Status: fixture.status, Header: headers, Body: []byte(fixture.body)}
			prepared := prepareChatResponse(response, headers, Credentials{})
			facts := prepared.Failure
			if prepared.Status != fixture.effective || facts == nil || facts.WireStatus != fixture.status || facts.Status != fixture.status || facts.EffectiveStatus != fixture.effective || facts.Code != fixture.code || facts.Timing.DelayMS == nil || *facts.Timing.DelayMS != 7200000 {
				test.Fatalf("wire, business and effective facts merged: %+v %+v", prepared, facts)
			}
			if fixture.name == "malformed" && (facts.BodyComplete || facts.CodeValid || !facts.CodePresent) {
				test.Fatal("malformed code became valid evidence")
			}
			if fixture.effective == 403 && string(prepared.Body) != fixture.body {
				test.Fatal("non-quota refusal body changed")
			}
			if string(response.Body) != fixture.body || headers.Get("Retry-After") != "7200" {
				test.Fatal("original response mutated")
			}
		})
	}
	response := ByteResponse{Status: 200, Header: http.Header{}, Body: []byte("{\n \"id\": \"response\", \"usage\": {\"input_tokens\": 0}\n}")}
	prepared := prepareChatResponse(response, response.Header, Credentials{})
	if prepared.Failure != nil || string(prepared.Body) != string(response.Body) || prepared.Status != 200 {
		test.Fatal("successful sparse usage response changed")
	}
}

func TestFailureFactsConnectorAndUploadUnknowns(test *testing.T) {
	for _, fixture := range []struct {
		name     string
		body     string
		code     string
		complete bool
	}{
		{"nested", `{"is_error":true,"result_json":"{\"code\":1008,\"message\":\"quota exceeded\"}"}`, "1008", true},
		{"nested_numeric", `{"is_error":false,"result_json":"{\"code\":1008,\"message\":\"quota exceeded\"}"}`, "1008", true},
		{"malformed", `{"is_error":true,"result_json":"{broken"}`, "", false},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			_, err := connectorResult(ByteResponse{Status: 200, Header: http.Header{"Retry-After": {"42"}}, Body: []byte(fixture.body)})
			facts := requireOriginalFailure(test, err)
			if facts.Code != fixture.code || facts.BodyComplete != fixture.complete || facts.WireStatus != 200 || facts.Origin != "connector-result" || facts.Timing.DelayMS == nil || *facts.Timing.DelayMS != 42000 {
				test.Fatalf("nested facts lost: %+v", facts)
			}
		})
	}
	_, err := signedUploadResult(map[string]any{"result_json": `{"code":1008,"message":"quota exceeded"}`})
	facts := requireOriginalFailure(test, err)
	if facts.WireStatus != 0 || facts.TrustedEndpoint || facts.Code != "1008" || facts.Origin != "upload-envelope" || facts.Timing.State != "absent" {
		test.Fatalf("nested upload invented transport authority: %+v", facts)
	}
	for _, body := range []string{`{"code":null}`, `{"code":1008.5}`, `{"code":"01008"}`} {
		facts := refusalFacts(403, nil, []byte(body), true)
		if !facts.CodePresent || facts.CodeValid || facts.BodyComplete || facts.accountQuota() {
			test.Fatalf("invalid code became known: %+v", facts)
		}
	}
}

type incompleteFactsBody struct {
	closed bool
}

func (body *incompleteFactsBody) Read(buffer []byte) (int, error) {
	return copy(buffer, `{"error":{"code":"denied","message":"quota exceeded"}}`), io.ErrUnexpectedEOF
}

func (body *incompleteFactsBody) Close() error {
	body.closed = true
	return nil
}

func TestFailureFactsIncompleteReadAndSanitization(test *testing.T) {
	body := &incompleteFactsBody{}
	_, err := bounded(&http.Response{StatusCode: 403, Header: http.Header{"Retry-After": {"42"}}, Body: body}, 1024)
	facts := requireOriginalFailure(test, err)
	if !body.closed || facts.BodyComplete || facts.WireStatus != 403 || !errors.Is(err, io.ErrUnexpectedEOF) {
		test.Fatalf("partial read lost completeness: %+v %v", facts, err)
	}
	client := &SelectedClient{credentials: Credentials{Access: "fixture-access", Refresh: "fixture-refresh"}}
	err = responseFailure(&APIError{Status: 401, Code: "http"}, ByteResponse{Status: 401, Header: http.Header{"X-Request-Id": {"fixture-access"}}, Body: []byte(`{"error":{"code":"fixture-access","type":"fixture-refresh","message":"fixture-access fixture-refresh"}}`)}, "http-body")
	err = client.selectedFailure(err)
	for current := err; current != nil; current = errors.Unwrap(current) {
		if carrier, ok := current.(originalFailure); ok {
			encoded, _ := json.Marshal(carrier.OriginalFailure())
			if strings.Contains(string(encoded), "fixture-access") || strings.Contains(string(encoded), "fixture-refresh") {
				test.Fatal("inner error retained unredacted facts")
			}
		}
	}
	err = safeError(err)
	facts = requireOriginalFailure(test, err)
	encoded, _ := json.Marshal(facts)
	if strings.Contains(string(encoded), "fixture-access") || strings.Contains(string(encoded), "fixture-refresh") || !facts.TrustedEndpoint {
		test.Fatalf("selected credentials leaked: %s", encoded)
	}
}

func TestFailureFactsOrdinaryHandlersKeepAcceptedFence(test *testing.T) {
	for _, name := range []string{"tool", "media", "search_tool", "stream"} {
		test.Run(name, func(test *testing.T) {
			var requests atomic.Int64
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
					requests.Add(1)
					payload := string(raw(toolsCatalog(toolsEntry("generate_image", `{"type":"object"}`))))
					contentType := "application/json"
					if name == "stream" {
						contentType = "text/event-stream"
						payload = "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"code\":1008,\"message\":\"fixture-access quota exceeded\"}}\n\n"
					} else if request.URL.Path == matrixCallPath {
						payload = string(raw(map[string]any{"is_error": true, "result_json": `{"code":1008,"message":"fixture-access quota exceeded"}`}))
					} else if request.URL.Path == webSearchPath {
						payload = `{"code":1008,"message":"fixture-access quota exceeded"}`
					}
					response := coreResponse(200, payload, contentType)
					response.Header.Set("Retry-After", "7200")
					return response, nil
				}), func() {}, nil
			}})
			method := sdk.MethodToolCall
			parameters := any(sdk.ToolRequest{ID: "generate_image", Arguments: raw(map[string]any{"requests": []any{map[string]any{"prompt": "fixture"}}})})
			if name == "media" {
				method = sdk.MethodProviderExecute
				parameters = mediaRequest(sdk.OperationImageGenerate, imageModel, map[string]any{"prompt": "fixture"})
			} else if name == "stream" {
				method = sdk.MethodProviderExecute
				parameters = sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}
			} else if name == "search_tool" {
				parameters = sdk.ToolRequest{ID: "web_search", Arguments: raw(map[string]any{"query": "fixture"})}
			}
			original := configured.Handlers[method]
			captured := make(chan error, 1)
			configured.Handlers[method] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
				result, err := original(ctx, call, stream)
				captured <- err
				return result, err
			}
			accepted := 0
			terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(method, parameters), func(frame sdk.Frame) {
				if frame.Event == "accepted" {
					accepted++
				}
			})
			facts := requireOriginalFailure(test, <-captured)
			if facts.Code != "1008" || facts.WireStatus != 200 || !facts.TrustedEndpoint || strings.Contains(facts.Message, "fixture-access") || facts.Timing.DelayMS == nil || *facts.Timing.DelayMS != 7200000 {
				test.Fatalf("handler lost selected original facts: %+v", facts)
			}
			if name != "search_tool" && (accepted != 1 || terminal.Error == nil || terminal.Error.Outcome != "uncertain") {
				test.Fatalf("accepted request gained retry permission: %+v, accepted=%d", terminal, accepted)
			}
			expectedRequests := int64(2)
			if name == "search_tool" || name == "stream" {
				expectedRequests = 1
			}
			if requests.Load() != expectedRequests || (name == "stream" && (!facts.OutputStarted || facts.accountQuota())) {
				test.Fatalf("failure replayed or stream became pre-output quota: %+v requests=%d", facts, requests.Load())
			}
		})
	}
}

func TestFailureFactsForeignTransferHasNoAccountAuthority(test *testing.T) {
	var requests atomic.Int64
	configured := NewModule(ModuleOptions{})
	owner := &module{options: ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			if request.Header.Get("Authorization") != "" {
				test.Error("account credentials reached transfer host")
			}
			return coreResponse(401, `{"error":{"code":"invalid_token","message":"foreign token refused"}}`, "application/json"), nil
		}), func() {}, nil
	}}}
	captured := make(chan error, 1)
	configured.Handlers[sdk.MethodToolCall] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		client := &SelectedClient{owner: owner, stream: stream, credentials: Credentials{Access: "fixture-access"}}
		response, err := client.Transfer(ctx, http.MethodGet, "https://8.8.8.8/fixture", nil, nil, 1024)
		if err == nil {
			err = transferFailure(toolError("fixture transfer failed"), response)
		}
		captured <- err
		return nil, err
	}
	toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodToolCall, sdk.ToolRequest{ID: "list_tools", Arguments: raw(map[string]any{})}), func(sdk.Frame) {})
	facts := requireOriginalFailure(test, <-captured)
	if facts.TrustedEndpoint || facts.WireStatus != 401 || facts.Code != "invalid_token" || facts.Origin != "media-transfer" || requests.Load() != 1 {
		test.Fatalf("foreign failure gained account authority: %+v requests=%d", facts, requests.Load())
	}
}
