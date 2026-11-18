package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestRequestPolicyOriginalFinalRefusal(t *testing.T) {
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sends.Add(1)
		writer.Header().Set("Retry-After", "3600")
		writer.WriteHeader(429)
		io.WriteString(writer, `{"error":{"type":"GoUsageLimitError","message":"usage limit reached"}}`)
	}))
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
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				sends.Add(1)
				writer.WriteHeader(429)
				io.WriteString(writer, body)
			}))
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
