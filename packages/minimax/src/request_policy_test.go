package minimax

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestRequestPolicyFinalChatRefusal(test *testing.T) {
	for _, fixture := range []struct {
		name, body, code, kind string
		status                 int
	}{
		{"business", `{"base_resp":{"status_code":1008,"status_msg":"fixture-access fixture-refresh 积分余额不足"}}`, "1008", "", 200},
		{"credit", `{"base_resp":{"status_code":2053,"status_msg":"credit exhausted"}}`, "2053", "", 403},
		{"permission", `{"type":"error","error":{"type":"permission_error","message":"insufficient permissions"}}`, "", "permission_error", 403},
		{"content", `{"type":"error","error":{"type":"content_policy_error","message":"quota exceeded"}}`, "", "content_policy_error", 403},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			var requests atomic.Int64
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					requests.Add(1)
					response := coreResponse(fixture.status, fixture.body, "application/json")
					response.Header.Set("Retry-After", "7200")
					return response, nil
				}), func() {}, nil
			}})
			published := 0
			terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodProviderExecute,
				sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}),
				func(frame sdk.Frame) {
					if frame.Kind == sdk.KindEvent {
						published++
					}
				})
			var result sdk.ExecuteResult
			if terminal.Error != nil || json.Unmarshal(terminal.Result, &result) != nil {
				test.Fatalf("refusal failed to reach the public result: %+v", terminal)
			}
			if result.Refusal == nil {
				test.Fatalf("original refusal facts were discarded: status=%d outcome=%s events=%d", result.Status, result.Outcome, published)
			}
			facts := result.Refusal.Facts
			if result.Status != fixture.status || facts.Status != fixture.status || facts.Code != fixture.code || facts.Type != fixture.kind ||
				!facts.Complete || facts.Operation != sdk.MethodProviderExecute || facts.Phase != "response" ||
				facts.RetryAfterMS == nil || *facts.RetryAfterMS != 7200000 || result.Refusal.Evidence != "rejected-before-effect" ||
				result.Outcome != "not-started" || published != 0 || requests.Load() != 1 {
				test.Fatalf("final refusal changed wire facts or replay boundary: result=%+v facts=%+v events=%d", result, facts, published)
			}
			if strings.Contains(string(terminal.Result), "fixture-access") || strings.Contains(string(terminal.Result), "fixture-refresh") {
				test.Fatal("selected credentials leaked through public refusal")
			}
			if len(result.Refusal.Rules) == 0 || !configured.Manifest.Contract.Has(sdk.RequestPolicy) ||
				configured.Manifest.Contract.Version.Minor != 12 || configured.Manifest.Compatibility.HostAPI != "^1.12" {
				test.Fatal("public request policy was not negotiated")
			}
		})
	}
}

func TestRequestPolicyUnprovedChatRemainsFenced(test *testing.T) {
	for _, fixture := range []struct {
		name, body, contentType string
		status                  int
		partial                 bool
	}{
		{"unknown", `{"code":2013,"message":"quota exceeded"}`, "application/json", 200, false},
		{"prose_only", `{"message":"quota exceeded"}`, "application/json", 403, false},
		{"content", `{"base_resp":{"status_code":1008,"status_msg":"quota exceeded"},"content":[{"type":"text","text":"paid output"}]}`, "application/json", 200, false},
		{"usage", `{"base_resp":{"status_code":1008,"status_msg":"quota exceeded"},"usage":{"output_tokens":0}}`, "application/json", 200, false},
		{"receipt", `{"base_resp":{"status_code":1008,"status_msg":"quota exceeded"},"task_id":"accepted"}`, "application/json", 200, false},
		{"duplicate", `{"base_resp":{"status_code":0,"status_code":1008,"status_msg":"quota exceeded"}}`, "application/json", 200, false},
		{"fractional", `{"base_resp":{"status_code":1008.5,"status_msg":"quota exceeded"}}`, "application/json", 200, false},
		{"null", `{"base_resp":{"status_code":null,"status_msg":"quota exceeded"}}`, "application/json", 200, false},
		{"malformed", `{"base_resp":{"status_code":1008`, "application/json", 200, false},
		{"partial", "", "application/json", 403, true},
		{"sse", "data: {\"type\":\"message_start\",\"message\":{\"id\":\"accepted\"}}\n\ndata: {\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"code\":1008}}\n\n", "text/event-stream", 200, false},
		{"sse_http_error", "data: {\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"code\":1008}}\n\n", "text/event-stream", 429, false},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					response := coreResponse(fixture.status, fixture.body, fixture.contentType)
					if fixture.partial {
						response.Body = &incompleteFactsBody{}
					}
					return response, nil
				}), func() {}, nil
			}})
			terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodProviderExecute,
				sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}), func(sdk.Frame) {})
			var result sdk.ExecuteResult
			if terminal.Error == nil {
				if json.Unmarshal(terminal.Result, &result) != nil || result.Refusal != nil || result.Outcome != "completed" {
					test.Fatalf("unproved failure gained final authority: %s", terminal.Result)
				}
			} else if terminal.Error.Outcome != "uncertain" {
				test.Fatalf("unproved failure lost uncertainty: %+v", terminal.Error)
			}
		})
	}
}

func TestRequestPolicyChatSuccessKeepsBytesAndAcceptance(test *testing.T) {
	payload := "{\n\"type\":\"message\",\"content\":[],\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}\n"
	configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
			return coreResponse(200, payload, "application/json"), nil
		}), func() {}, nil
	}})
	events := []string{}
	var output []byte
	terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodProviderExecute,
		sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}),
		func(frame sdk.Frame) {
			if frame.Kind == sdk.KindEvent {
				events = append(events, frame.Event)
			}
			if frame.Event == "data" {
				var chunk []byte
				if json.Unmarshal(frame.Data, &chunk) != nil {
					test.Fatal("invalid data")
				}
				output = append(output, chunk...)
			}
		})
	if terminal.Error != nil || string(output) != payload || len(events) < 3 || events[0] != "accepted" || events[1] != "headers" {
		test.Fatalf("success changed: terminal=%+v events=%v output=%q", terminal, events, output)
	}
}

func TestRequestPolicyTransportErrorRemainsUncertain(test *testing.T) {
	var requests atomic.Int64
	configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, io.ErrUnexpectedEOF
		}), func() {}, nil
	}})
	terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodProviderExecute,
		sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}), func(sdk.Frame) {})
	if terminal.Error == nil || terminal.Error.Outcome != "uncertain" || requests.Load() != 1 {
		test.Fatalf("ambiguous dispatch became replayable: %+v requests=%d", terminal, requests.Load())
	}
}

func TestRequestPolicyTimingKnownness(test *testing.T) {
	for _, fixture := range []struct {
		name, reset string
		headers     []string
		delay       *int64
		publicValue string
	}{
		{name: "absent"},
		{name: "zero", headers: []string{"0"}, delay: policyDelay(0), publicValue: "0"},
		{name: "seconds", headers: []string{"42"}, delay: policyDelay(42000), publicValue: "42"},
		{name: "date", headers: []string{"Sun, 15 Nov 2026 18:00:00 GMT"}, reset: "2026-11-15T18:00:00Z", publicValue: "Sun, 15 Nov 2026 18:00:00 GMT"},
		{name: "malformed", headers: []string{"soon"}},
		{name: "duplicate", headers: []string{"42", "7200"}},
		{name: "negative", headers: []string{"-1"}},
		{name: "overflow", headers: []string{"9223372036854775807"}},
		{name: "over_public_duration_limit", headers: []string{"2678400"}, publicValue: "2678400"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					response := coreResponse(200, `{"base_resp":{"status_code":1008,"status_msg":"insufficient balance"}}`, "application/json")
					if fixture.headers != nil {
						response.Header["Retry-After"] = fixture.headers
					}
					return response, nil
				}), func() {}, nil
			}})
			terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodProviderExecute,
				sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}), func(sdk.Frame) {})
			var result sdk.ExecuteResult
			if terminal.Error != nil || json.Unmarshal(terminal.Result, &result) != nil || result.Refusal == nil {
				test.Fatalf("timing erased complete refusal: %+v", terminal)
			}
			facts := result.Refusal.Facts
			if (facts.RetryAfterMS == nil) != (fixture.delay == nil) || fixture.delay != nil && *facts.RetryAfterMS != *fixture.delay ||
				facts.ResetAt != fixture.reset || facts.Headers["Retry-After"] != fixture.publicValue {
				test.Fatalf("timing knownness changed: %+v", facts)
			}
		})
	}
}

func policyDelay(value int64) *int64 {
	return &value
}

func TestRequestPolicyRedirectDoesNotProveWholeChatRejection(test *testing.T) {
	var requests atomic.Int64
	configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
			if requests.Add(1) == 1 {
				response := coreResponse(307, "", "application/json")
				response.Header.Set("Location", "/after-dispatch")
				return response, nil
			}
			return coreResponse(200, `{"base_resp":{"status_code":1008,"status_msg":"insufficient balance"}}`, "application/json"), nil
		}), func() {}, nil
	}})
	accepted := 0
	terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodProviderExecute,
		sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}),
		func(frame sdk.Frame) {
			if frame.Event == "accepted" {
				accepted++
			}
		})
	var result sdk.ExecuteResult
	if terminal.Error != nil || json.Unmarshal(terminal.Result, &result) != nil || result.Refusal != nil ||
		result.Outcome != "completed" || accepted != 1 || requests.Load() != 2 {
		test.Fatalf("redirect erased earlier dispatch uncertainty: result=%+v accepted=%d requests=%d", result, accepted, requests.Load())
	}
}
