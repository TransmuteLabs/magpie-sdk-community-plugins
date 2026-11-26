package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const refusalFixtureCatalog = `{"data":[{"id":"fixture-model","type":"chat","limit":{"context":8192,"output":4096},"cost":{"text":{"input":{"price":0},"output":{"price":0}}},"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"},"responses":{"base_url":"https://opencode.ai/inference/openai/v1"},"generate-content":{"base_url":"https://opencode.ai/inference/google/v1beta"}}}]}`

func TestRequestPolicyOriginalFinalRefusal(t *testing.T) {
	var sends atomic.Int32
	server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		if serveFixtureGoCatalog(writer, request) {
			return
		}
		sends.Add(1)
		writer.Header().Set("Retry-After", "3600")
		writer.WriteHeader(429)
		io.WriteString(writer, `{"error":{"type":"GoUsageLimitError","message":"usage limit reached"}}`)
	})
	defer server.Close()
	harness := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	credentials := Credentials{Access: "fixture-access", Refresh: "fixture-refresh", UserID: "fixture-user", Email: "fixture@example.invalid", Expires: time.Now().Add(time.Hour)}
	auth := raw(credentials)
	frame, body := harness.invoke(sdk.MethodProviderExecute, sdk.Scope{Provider: ProviderGo, Account: credentials.AccountID(ProviderGo), Generation: 1},
		sdk.ExecuteRequest{Protocol: "chat", Model: Models()[0].ID, Body: raw(map[string]any{"model": Models()[0].ID, "messages": []any{}})}, auth)
	var final sdk.ExecuteResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &final) != nil || final.Refusal == nil {
		t.Fatalf("whole preflight refusal lost final policy facts: error=%v result=%s", frame.Error, frame.Result)
	}
	facts := final.Refusal.Facts
	if final.Status != 429 || final.Outcome != "not-started" || !facts.Complete ||
		facts.Type != "GoUsageLimitError" || facts.Message != "usage limit reached" ||
		facts.Headers["Retry-After"] != "3600" || final.Refusal.Evidence != "rejected-before-effect" ||
		sends.Load() != 1 || len(body) != 0 || len(harness.headers) != 0 || harness.accepted["fixture-2"] {
		t.Fatalf("refusal lost original facts or crossed acceptance/output: %+v body=%s sends=%d", final, body, sends.Load())
	}
	for _, rule := range final.Refusal.Rules {
		if rule.ID == "opencode.go-limit" && rule.Scope == "model" {
			return
		}
	}
	t.Fatal("Go refusal did not confine the timed hold to the attempted model")
}

func TestOpenCodeKnownContentRefusalsStopBeforeAcceptance(t *testing.T) {
	identity := ZenCredentials{Kind: "key", Key: "fixture-zen"}
	for _, test := range []struct {
		name, protocol, body, code, kind string
		status                           int
	}{
		{"OpenAI", "responses", `{"error":{"message":"Request flagged by a safeguard","type":"invalid_request_error","param":null,"code":"cyber_policy"}}`, "cyber_policy", "invalid_request_error", 400},
		{"Gemini", "gemini", `{"error":{"code":403,"message":"Request rejected in accordance with our terms of use and policies.","status":"PERMISSION_DENIED"}}`, "403", "PERMISSION_DENIED", 403},
		{"ZenWrappedGemini", "chat", `{"error":{"type":"server_error","message":"Upstream request failed: [403] Request rejected in accordance with our terms of use and policies."}}`, "", "server_error", 403},
		{"GenericPermissionDenied", "gemini", `{"error":{"code":403,"message":"Caller does not have permission","status":"PERMISSION_DENIED"}}`, "", "", 403},
		{"WrongWrappedStatus", "chat", `{"error":{"type":"server_error","message":"Upstream request failed: [500] Request rejected in accordance with our terms of use and policies."}}`, "", "", 403},
		{"QuotedWrappedRefusal", "chat", `{"error":{"type":"server_error","message":"Proxy diagnostic quoted \"Upstream request failed: [403] Request rejected in accordance with our terms of use and policies.\""}}`, "", "", 403},
		{"QuotedNativeRefusal", "gemini", `{"error":{"code":403,"message":"Proxy diagnostic quoted \"Request rejected in accordance with our terms of use and policies.\"","status":"PERMISSION_DENIED"}}`, "", "", 403},
		{"WrongStatus", "responses", `{"error":{"message":"Request flagged by a safeguard","type":"invalid_request_error","code":"cyber_policy"}}`, "", "", 500},
		{"Accepted", "responses", `{"error":{"message":"Request flagged by a safeguard","type":"invalid_request_error","code":"cyber_policy"},"accepted":true}`, "", "", 400},
		{"ReflectedCredential", "responses", `{"error":{"message":"Request uses fixture-zen","type":"invalid_request_error","code":"cyber_policy"}}`, "", "", 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			var sends atomic.Int32
			server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/inference/v1/models" {
					io.WriteString(writer, refusalFixtureCatalog)
					return
				}
				sends.Add(1)
				writer.WriteHeader(test.status)
				io.WriteString(writer, test.body)
			})
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{
				Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) {
					return server.Client(), nil, nil
				},
			}), raw(moduleConfig{ProjectID: "global"}))
			payload := map[string]any{"model": "fixture-model"}
			if test.protocol == "gemini" {
				payload["contents"] = []any{map[string]string{"role": "user"}}
			}
			frame, body := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZen, identity),
				sdk.ExecuteRequest{Protocol: test.protocol, Model: "fixture-model", Body: raw(payload)}, raw(identity))
			var final sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &final) != nil || final.Status != test.status || sends.Load() != 1 {
				t.Fatalf("expected exactly one inference attempt and its original HTTP status: error=%v result=%s sends=%d", frame.Error, frame.Result, sends.Load())
			}
			if test.kind == "" {
				if final.Refusal != nil || !harness.accepted[frame.ID] || len(harness.headers) != 1 || string(body) != test.body {
					t.Fatalf("unproved refusal changed authority or reply: result=%s body=%s", frame.Result, body)
				}
				return
			}
			if final.Refusal == nil || final.Outcome != "not-started" || final.Refusal.Evidence != "rejected-before-effect" ||
				harness.accepted[frame.ID] || len(harness.headers) != 0 || len(body) != 0 {
				t.Fatalf("content refusal crossed the acceptance boundary: error=%v result=%s body=%s sends=%d", frame.Error, frame.Result, body, sends.Load())
			}
			facts := final.Refusal.Facts
			if !facts.Complete || facts.Code != test.code || facts.Type != test.kind || facts.Message == "" {
				t.Fatalf("content refusal lost its source fields: %+v", facts)
			}
			for _, rule := range final.Refusal.Rules {
				if rule.Class == "content-refusal" && rule.Scope == "request" {
					return
				}
			}
			t.Fatal("content refusal lacks a request-scoped rule")
		})
	}
}

func TestOpenCodeStreamContentRefusalRetainsAcceptedFence(t *testing.T) {
	const refusal = `{"type":"error","sequence_number":1,"code":"cyber_policy","message":"Request flagged by a safeguard","param":null}`
	personal := ZenCredentials{Kind: "key", Key: "fixture-key"}
	public := ZenCredentials{Kind: "public", Key: "public"}
	for _, test := range []struct {
		name, provider, data, code string
		identity                   ZenCredentials
		stream                     bool
	}{
		{"Personal", ProviderZen, refusal, "content_refusal", personal, true},
		{"PublicStream", ProviderZenPublic, refusal, "content_refusal", public, true},
		{"PublicCollapse", ProviderZenPublic, refusal, "content_refusal", public, false},
		{"TypeConflict", ProviderZen, `{"Type":"response.completed","type":"error","code":"cyber_policy"}`, "upstream", personal, true},
		{"CodeConflict", ProviderZen, `{"type":"error","Code":"server_error","code":"cyber_policy"}`, "upstream", personal, true},
		{"TypeAliasOnly", ProviderZen, `{"Type":"error","code":"cyber_policy"}`, "upstream", personal, true},
		{"CodeAliasOnly", ProviderZen, `{"type":"error","Code":"cyber_policy"}`, "upstream", personal, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var sends atomic.Int32
			server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/inference/v1/models" {
					io.WriteString(writer, refusalFixtureCatalog)
					return
				}
				sends.Add(1)
				writer.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(writer, "event: error\ndata: "+test.data+"\n\n")
			})
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{
				Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) {
					return server.Client(), nil, nil
				},
			}), raw(moduleConfig{ProjectID: "global"}))
			payload := map[string]any{"model": "fixture-model", "input": "hello", "stream": test.stream}
			frame, body := harness.invoke(sdk.MethodProviderExecute, zenScope(test.provider, test.identity),
				sdk.ExecuteRequest{Protocol: "responses", Model: "fixture-model", Body: raw(payload)}, raw(test.identity))
			if frame.Error == nil || frame.Error.Code != test.code || frame.Error.Outcome != "uncertain" ||
				!harness.accepted[frame.ID] || len(body) != 0 || sends.Load() != 1 {
				t.Fatalf("stream error did not retain its expected code %q and acceptance fence: error=%v result=%s body=%s sends=%d", test.code, frame.Error, frame.Result, body, sends.Load())
			}
		})
	}
}

func TestRequestPolicyAmbiguityRetainsAcceptedFence(t *testing.T) {
	for name, body := range map[string]string{
		"unknown":            `{"error":{"type":"server_error","message":"Go usage limit exceeded"}}`,
		"accepted":           `{"error":{"type":"GoUsageLimitError","message":"Go usage limit exceeded"},"accepted":true}`,
		"usage":              `{"error":{"type":"GoUsageLimitError","message":"Go usage limit exceeded"},"usage":{"input_tokens":0}}`,
		"nested effects":     `{"error":{"type":"GoUsageLimitError","message":"Go usage limit exceeded","accepted":true}}`,
		"metadata effects":   `{"error":{"type":"GoUsageLimitError","message":"Go usage limit exceeded"},"metadata":{"accepted":"true"}}`,
		"duplicate":          `{"error":{"type":"server_error","type":"GoUsageLimitError","message":"Go usage limit exceeded"}}`,
		"null type":          `{"error":{"type":null,"message":"Go usage limit exceeded"}}`,
		"wrong type":         `{"error":{"type":["GoUsageLimitError"],"message":"Go usage limit exceeded"}}`,
		"code contradiction": `{"error":{"code":"accepted","type":"GoUsageLimitError","message":"Go usage limit exceeded"}}`,
		"credential":         `{"error":{"type":"GoUsageLimitError","message":"fixture-access"}}`,
		"truncated":          `{"error":{"type":"GoUsageLimitError","message":"Go usage limit exceeded"`,
		"oversized":          `{"error":{"type":"GoUsageLimitError","message":"` + strings.Repeat("x", 65536) + `"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var sends atomic.Int32
			server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				if serveFixtureGoCatalog(writer, request) {
					return
				}
				sends.Add(1)
				writer.WriteHeader(429)
				io.WriteString(writer, body)
			})
			defer server.Close()
			harness := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
			credentials := Credentials{Access: "fixture-access", Refresh: "fixture-refresh", UserID: "fixture-user", Email: "fixture@example.invalid", Expires: time.Now().Add(time.Hour)}
			frame, _ := harness.invoke(sdk.MethodProviderExecute,
				sdk.Scope{Provider: ProviderGo, Account: credentials.AccountID(ProviderGo), Generation: 1},
				sdk.ExecuteRequest{Protocol: "chat", Model: Models()[0].ID, Body: raw(map[string]any{"model": Models()[0].ID, "messages": []any{}})}, raw(credentials))
			var final sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &final) != nil || final.Refusal != nil ||
				!harness.accepted[frame.ID] || sends.Load() != 1 {
				t.Fatal("ambiguous execution acquired routing authority or lost fence", frame.Error, string(frame.Result), sends.Load())
			}
		})
	}
}

func TestRequestPolicyAllConstructorsDeclareActualFloor(t *testing.T) {
	for _, manifest := range []sdk.Manifest{Manifest(), AgentManifest(), WindowManifest(), API8WindowManifest()} {
		if manifest.Contract.Version.Minor != 12 || manifest.Compatibility.HostAPI != "^1.12" ||
			!manifest.Contract.Has(sdk.RequestPolicy) {
			t.Fatal("variant understates the runtime capability", manifest.ID, manifest.Contract)
		}
	}
}
