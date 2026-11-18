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

func TestReadPolicySelectedSearchAndCatalog(test *testing.T) {
	for _, fixture := range []struct {
		name, method, path, step string
		params                   any
		success                  any
		accepted                 int
	}{
		{"search", sdk.MethodModuleCall, webSearchPath, "minimax-web-search",
			sdk.ModuleCallRequest{Action: "search." + Global, Input: raw(sdk.SearchRequest{Source: Global, Query: "private-query", Count: 1})},
			map[string]any{"organic": []any{map[string]any{"url": "https://example.invalid/read", "title": "Recovered"}}}, 0},
		{"web-tool", sdk.MethodToolCall, webSearchPath, "minimax-web-search",
			sdk.ToolRequest{ID: "web_search", Arguments: raw(map[string]any{"query": "private-query"})},
			map[string]any{"organic": []any{}}, 1},
		{"catalog", sdk.MethodToolCall, matrixListPath, "minimax-tool-catalog",
			sdk.ToolRequest{ID: "list_tools", Arguments: raw(map[string]any{})},
			toolsCatalog(toolsEntry("generate_image", `{"type":"object"}`)), 0},
		{"schema", sdk.MethodToolCall, matrixListPath, "minimax-tool-catalog",
			sdk.ToolRequest{ID: "tool_schema", Arguments: raw(map[string]any{"tool_name": matrixPrefix + "generate_image"})},
			toolsCatalog(toolsEntry("generate_image", `{"type":"object"}`)), 0},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			var requests atomic.Int64
			var firstBody string
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(request.Body)
					if err != nil || request.URL.Path != fixture.path || request.Header.Get("Authorization") != "Bearer fixture-access" {
						test.Error("read lost its selected endpoint or credentials")
					}
					if requests.Add(1) == 1 {
						firstBody = string(body)
						response := coreResponse(503, `{"base_resp":{"status_code":503,"status_msg":"busy fixture-access fixture-refresh"}}`, "application/json")
						response.Header.Set("Retry-After", "0")
						response.Header.Set("X-Private-Fixture", "private-header")
						return response, nil
					}
					if string(body) != firstBody {
						test.Error("host permit changed the selected read request")
					}
					return coreResponse(200, string(raw(fixture.success)), "application/json"), nil
				}), func() {}, nil
			}})
			call := toolsProtocolFrame(fixture.method, fixture.params)
			decisions, accepted := 0, 0
			terminal := toolsProtocolCall(test, configured, call, func(frame sdk.Frame) {
				if frame.Event == "accepted" {
					accepted++
				}
			}, func(frame sdk.Frame) sdk.RequestRecoveryResult {
				decisions++
				var failure sdk.RequestRecoveryRequest
				if frame.Scope != call.Scope || json.Unmarshal(frame.Params, &failure) != nil ||
					failure.Step != fixture.step || failure.Evidence != "read-only" ||
					failure.Facts.Operation != fixture.method || failure.Facts.Phase != "response" ||
					failure.Facts.Status != 503 || failure.Facts.Code != "503" || !failure.Facts.Complete ||
					failure.Facts.RetryAfterMS == nil || *failure.Facts.RetryAfterMS != 0 {
					test.Fatalf("named read lost its original facts/selection: %s scope=%+v", frame.Params, frame.Scope)
				}
				for _, private := range []string{"fixture-access", "fixture-refresh", "private-query", "private-header", "status_msg"} {
					if strings.Contains(string(frame.Params), private) {
						test.Fatalf("private input leaked into the policy DTO: %s", private)
					}
				}
				return sdk.RequestRecoveryResult{Step: failure.Step, Retry: true, Action: "wait"}
			})
			if terminal.Error != nil || requests.Load() != 2 || decisions != 1 || accepted != fixture.accepted {
				test.Fatalf("named read was not recovered exactly once: terminal=%s error=%+v requests=%d decisions=%d accepted=%d",
					terminal.Result, terminal.Error, requests.Load(), decisions, accepted)
			}
			if fixture.name == "search" && !strings.Contains(string(terminal.Result), "Recovered") {
				test.Fatalf("recovered search lost its page: %s", terminal.Result)
			}
			if strings.Contains(string(terminal.Result), "refusal") {
				test.Fatal("a named read acquired final provider refusal authority")
			}
		})
	}
}

func TestReadPolicyMalformedAndRedirectNeverRetry(test *testing.T) {
	for _, fixture := range []struct {
		name, body string
		status     int
		partial    bool
	}{
		{"invalid-code", `{"base_resp":{"status_code":503.5,"status_msg":"busy"}}`, 503, false},
		{"duplicate", `{"base_resp":{"status_code":0,"status_code":503,"status_msg":"busy"}}`, 503, false},
		{"malformed", `{"base_resp":{"status_code":503`, 503, false},
		{"partial", "", 503, true},
		{"redirect", "", 307, false},
		{"progress", `{"base_resp":{"status_code":503,"status_msg":"busy"},"task_id":"accepted"}`, 503, false},
		{"missing-code", `{"base_resp":{"status_msg":"busy"}}`, 503, false},
		{"missing-message", `{"base_resp":{"status_code":503}}`, 503, false},
		{"null-message", `{"base_resp":{"status_code":503,"status_msg":null}}`, 503, false},
		{"success-code", `{"base_resp":{"status_code":0,"status_msg":"busy"}}`, 503, false},
		{"conflicting", `{"base_resp":{"status_code":503,"status_msg":"busy"},"code":0}`, 503, false},
		{"unknown", `{"detail":"busy"}`, 503, false},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			var requests atomic.Int64
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					requests.Add(1)
					response := coreResponse(fixture.status, fixture.body, "application/json")
					if fixture.partial {
						response.Body = &incompleteFactsBody{}
					}
					if fixture.status == 307 {
						response.Header.Set("Location", "https://foreign.invalid/credential-trap")
					}
					return response, nil
				}), func() {}, nil
			}})
			callbacks := 0
			toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodToolCall,
				sdk.ToolRequest{ID: "list_tools", Arguments: raw(map[string]any{})}), func(frame sdk.Frame) {
				if frame.Method == sdk.MethodRequestRecover {
					callbacks++
				}
			})
			if requests.Load() != 1 || callbacks != 0 {
				test.Fatalf("unproved/redirected facts gained recovery: requests=%d callbacks=%d", requests.Load(), callbacks)
			}
		})
	}
}

func TestReadPolicyOriginalBusinessFactsAndTiming(test *testing.T) {
	for _, fixture := range []struct {
		name, body, timing string
		delay              bool
		reset              string
	}{
		{"business", `{"base_resp":{"status_code":1008,"status_msg":"fixture-access fixture-refresh"}}`, "0", true, ""},
		{"status-info", `{"statusInfo":{"code":"503","message":"fixture-access fixture-refresh"}}`, "", false, ""},
		{"top-code", `{"code":503,"message":"fixture-access fixture-refresh"}`, "2592001", false, ""},
		{"error", `{"error":{"code":503,"type":"api_error","message":"fixture-access fixture-refresh"}}`,
			"Mon, 16 Nov 2026 00:00:00 GMT", false, "2026-11-16T00:00:00Z"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			var requests atomic.Int64
			status := 200
			if fixture.name == "error" {
				status = 503
			}
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					requests.Add(1)
					response := coreResponse(status, fixture.body, "application/json")
					if fixture.timing != "" {
						response.Header.Set("Retry-After", fixture.timing)
					}
					return response, nil
				}), func() {}, nil
			}})
			callbacks := 0
			toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodToolCall,
				sdk.ToolRequest{ID: "list_tools", Arguments: raw(map[string]any{})}),
				func(sdk.Frame) {}, func(frame sdk.Frame) sdk.RequestRecoveryResult {
					callbacks++
					var failure sdk.RequestRecoveryRequest
					if json.Unmarshal(frame.Params, &failure) != nil || failure.Facts.Status != status ||
						!failure.Facts.Complete || (failure.Facts.RetryAfterMS != nil) != fixture.delay ||
						failure.Facts.ResetAt != fixture.reset || failure.Facts.Headers["Retry-After"] != fixture.timing ||
						failure.Facts.Message != "<redacted> <redacted>" {
						test.Fatalf("original read facts/timing changed: %s", frame.Params)
					}
					return sdk.RequestRecoveryResult{Action: "terminal", Reason: "fixture-denied"}
				})
			if callbacks != 1 || requests.Load() != 1 {
				test.Fatalf("business refusal escaped its single named decision: callbacks=%d requests=%d", callbacks, requests.Load())
			}
		})
	}
}

func TestReadPolicySuccessBytesAndTransportFailure(test *testing.T) {
	for _, fail := range []bool{false, true} {
		test.Run(map[bool]string{false: "success", true: "transport"}[fail], func(test *testing.T) {
			var requests atomic.Int64
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					requests.Add(1)
					if fail {
						return nil, io.ErrUnexpectedEOF
					}
					return coreResponse(200, `{"organic":[],"text":"fixture-access"}`, "application/json"), nil
				}), func() {}, nil
			}})
			callbacks := 0
			terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodToolCall,
				sdk.ToolRequest{ID: "web_search", Arguments: raw(map[string]any{"query": "fixture"})}),
				func(frame sdk.Frame) {
					if frame.Method == sdk.MethodRequestRecover {
						callbacks++
					}
				})
			if requests.Load() != 1 || callbacks != 0 || (terminal.Error != nil) != fail {
				test.Fatalf("success/transport path gained recovery: requests=%d callbacks=%d error=%+v",
					requests.Load(), callbacks, terminal.Error)
			}
			if !fail && !strings.Contains(string(terminal.Result), "fixture-access") {
				test.Fatalf("successful read bytes were redacted: %s", terminal.Result)
			}
		})
	}
}

func TestReadPolicyDeniedPermitPreservesRefusal(test *testing.T) {
	var requests atomic.Int64
	configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			response := coreResponse(503, `{"base_resp":{"status_code":503,"status_msg":"busy"}}`, "application/json")
			response.Header.Set("Retry-After", "not-a-reset")
			return response, nil
		}), func() {}, nil
	}})
	callbacks := 0
	terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodModuleCall,
		sdk.ModuleCallRequest{Action: "search." + Global, Input: raw(sdk.SearchRequest{Source: Global, Query: "fixture", Count: 1})}),
		func(sdk.Frame) {}, func(frame sdk.Frame) sdk.RequestRecoveryResult {
			callbacks++
			var failure sdk.RequestRecoveryRequest
			if json.Unmarshal(frame.Params, &failure) != nil || failure.Facts.RetryAfterMS != nil ||
				failure.Facts.ResetAt != "" || len(failure.Facts.Headers) != 0 {
				test.Fatalf("unknown timing became policy authority: %s", frame.Params)
			}
			return sdk.RequestRecoveryResult{Action: "terminal", Reason: "budget-exhausted"}
		})
	var result sdk.SearchResult
	if terminal.Error != nil || json.Unmarshal(terminal.Result, &result) != nil || result.Status != 503 ||
		requests.Load() != 1 || callbacks != 1 {
		test.Fatalf("denied read lost its original failure: terminal=%s error=%+v requests=%d callbacks=%d",
			terminal.Result, terminal.Error, requests.Load(), callbacks)
	}
}

func TestReadPolicyPaidInvokeNeverRepeats(test *testing.T) {
	var catalogs, invokes atomic.Int64
	configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path == matrixListPath {
				if catalogs.Add(1) == 1 {
					return coreResponse(503, `{"base_resp":{"status_code":503,"status_msg":"busy"}}`, "application/json"), nil
				}
				return coreResponse(200, string(raw(toolsCatalog(toolsEntry("generate_image", `{"type":"object"}`)))), "application/json"), nil
			}
			invokes.Add(1)
			return coreResponse(503, `{"base_resp":{"status_code":503,"status_msg":"paid work uncertain"}}`, "application/json"), nil
		}), func() {}, nil
	}})
	callbacks, accepted := 0, 0
	terminal := toolsProtocolCall(test, configured, toolsProtocolFrame(sdk.MethodToolCall,
		sdk.ToolRequest{ID: "generate_image", Arguments: raw(map[string]any{"requests": []any{map[string]any{"prompt": "fixture"}}})}),
		func(frame sdk.Frame) {
			if frame.Event == "accepted" {
				accepted++
			}
		}, func(frame sdk.Frame) sdk.RequestRecoveryResult {
			callbacks++
			var failure sdk.RequestRecoveryRequest
			if json.Unmarshal(frame.Params, &failure) != nil || failure.Step != "minimax-tool-catalog" {
				test.Fatal("generic paid connector request acquired read evidence")
			}
			return sdk.RequestRecoveryResult{Step: failure.Step, Action: "wait", Retry: true}
		})
	if catalogs.Load() != 2 || invokes.Load() != 1 || callbacks != 1 || accepted != 1 ||
		terminal.Error == nil || terminal.Error.Outcome != "uncertain" {
		test.Fatalf("paid write replayed or lost its fence: catalogs=%d invokes=%d callbacks=%d accepted=%d error=%+v",
			catalogs.Load(), invokes.Load(), callbacks, accepted, terminal.Error)
	}
}

func TestReadPolicyMalformedPermitDoesNotRepeat(test *testing.T) {
	for _, permit := range []string{
		`{"retry":"true","action":"wait"}`,
		`{"retry":true,"action":"wait","step":7}`,
		`[]`,
	} {
		test.Run(permit, func(test *testing.T) {
			var requests atomic.Int64
			configured := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
				return toolsProtocolDoer(func(*http.Request) (*http.Response, error) {
					requests.Add(1)
					return coreResponse(503, `{"base_resp":{"status_code":503,"status_msg":"busy"}}`, "application/json"), nil
				}), func() {}, nil
			}})
			callbacks := 0
			terminal := toolsProtocolReplyCall(test, configured, toolsProtocolFrame(sdk.MethodModuleCall,
				sdk.ModuleCallRequest{Action: "search." + Global, Input: raw(sdk.SearchRequest{Source: Global, Query: "fixture", Count: 1})}),
				func(sdk.Frame) {}, func(sdk.Frame) json.RawMessage {
					callbacks++
					return json.RawMessage(permit)
				})
			var result sdk.SearchResult
			if requests.Load() != 1 || callbacks != 1 || terminal.Error != nil ||
				json.Unmarshal(terminal.Result, &result) != nil || result.Status < 400 || len(result.Pages) != 0 {
				test.Fatalf("malformed permit gained a replay: requests=%d callbacks=%d result=%s error=%+v",
					requests.Load(), callbacks, terminal.Result, terminal.Error)
			}
		})
	}
}
