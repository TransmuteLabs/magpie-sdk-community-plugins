package commandcode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSubscriptionFailureStopsExecutionAndDiscovery(t *testing.T) {
	for _, subscription := range []string{"http503", `{`, `{}`, `{"success":false,"data":null}`, `{"data":{}}`} {
		t.Run(subscription, func(t *testing.T) {
			var other, reads atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/alpha/billing/subscriptions" {
					reads.Add(1)
					if subscription == "http503" {
						w.WriteHeader(503)
						return
					}
					io.WriteString(w, subscription)
					return
				}
				other.Add(1)
				w.WriteHeader(500)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL})
			callbacks := 0
			if subscription == "http503" {
				worker.recover = func(request sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult {
					callbacks++
					if request.Step != "commandcode.subscription" || request.Evidence != "" || request.Facts.Status != 503 ||
						request.Facts.Complete || request.Facts.Code != "" || request.Facts.Type != "" || request.Facts.Message != "" ||
						request.Facts.Operation != sdk.MethodProviderExecute || request.Facts.Phase != "response" {
						t.Fatal("subscription denial lost its named original facts", request)
					}
					return sdk.RequestRecoveryResult{Step: request.Step, Action: "terminal", Retry: false}
				}
			}
			for index, method := range []string{sdk.MethodProviderExecute, sdk.MethodProviderModels} {
				var input any = sdk.ExecuteRequest{Protocol: "chat", Model: "fixture", Body: []byte(`{"model":"fixture"}`)}
				if method == sdk.MethodProviderModels {
					input = struct{}{}
				}
				_, events, err := worker.call(method, input, testAuth())
				var failure *sdk.Error
				if !errors.As(err, &failure) || failure.Code != "unavailable" || failure.Outcome != "not-started" || len(events) != 0 || other.Load() != 0 {
					t.Fatalf("%s treated unknown plan as authority: error=%v events=%v requests=%d", method, err, events, other.Load())
				}
				wantCallbacks := 0
				if subscription == "http503" {
					wantCallbacks = 1
				}
				if callbacks != wantCallbacks || reads.Load() != int64(index+1) {
					t.Fatalf("%s bypassed denial or repeated: callbacks=%d reads=%d", method, callbacks, reads.Load())
				}
			}
		})
	}
}

func TestProviderRequestPathsPreserveOperation(t *testing.T) {
	for _, sample := range []struct {
		name, plan, protocol, path, query, endpoint string
		stream, refuse                              bool
	}{
		{name: "count", plan: "pro", protocol: "anthropic", path: "/v1/messages/count_tokens", endpoint: "/provider/v1/messages/count_tokens"},
		{name: "unversioned-count", plan: "pro", protocol: "anthropic", path: "/messages/count_tokens", endpoint: "/provider/v1/messages/count_tokens"},
		{name: "messages", plan: "pro", protocol: "anthropic", path: "/v1/messages", endpoint: "/provider/v1/messages"},
		{name: "chat", plan: "pro", protocol: "chat", path: "/v1/chat/completions", endpoint: "/provider/v1/chat/completions"},
		{name: "responses", plan: "pro", protocol: "responses", path: "/v1/responses", endpoint: "/provider/v1/responses"},
		{name: "explicit-no-plan", protocol: "chat", path: "/v1/chat/completions", endpoint: "/provider/v1/chat/completions"},
		{name: "go-count", plan: "go", protocol: "anthropic", path: "/v1/messages/count_tokens", refuse: true},
		{name: "count-protocol-mismatch", plan: "pro", protocol: "chat", path: "/v1/messages/count_tokens", refuse: true},
		{name: "unknown-path", plan: "pro", protocol: "anthropic", path: "/v1/messages/unknown", refuse: true},
		{name: "ambiguous-path", plan: "pro", protocol: "anthropic", path: "/v1/../messages", refuse: true},
		{name: "existing-query", plan: "pro", protocol: "anthropic", path: "/v1/messages", query: "beta=true", endpoint: "/provider/v1/messages"},
		{name: "streaming-count", plan: "pro", protocol: "anthropic", path: "/v1/messages/count_tokens", stream: true, refuse: true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/alpha/billing/subscriptions" {
					if sample.plan == "" {
						io.WriteString(w, `{"data":null}`)
					} else {
						io.WriteString(w, `{"data":{"planId":"individual-`+sample.plan+`","status":"active"}}`)
					}
					return
				}
				calls.Add(1)
				if r.URL.Path != sample.endpoint {
					t.Errorf("unexpected dispatch %s, want %s", r.URL.Path, sample.endpoint)
				}
				io.WriteString(w, `{"input_tokens":7}`)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL})
			options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: sample.path, Query: sample.query})
			body, _ := json.Marshal(map[string]any{"model": "fixture", "messages": []any{}, "stream": sample.stream})
			_, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: sample.protocol, Model: "fixture", Body: body, Options: options}, testAuth())
			if sample.refuse {
				var failure *sdk.Error
				if !errors.As(err, &failure) || failure.Outcome != "not-started" || len(events) != 0 || calls.Load() != 0 {
					t.Fatalf("refusal dispatched or accepted: %v %v %d", err, events, calls.Load())
				}
			} else {
				status, reply := eventsBody(t, events)
				if err != nil || calls.Load() != 1 || status != 200 || string(reply) != `{"input_tokens":7}` {
					t.Fatalf("operation forwarding changed: error=%v calls=%d status=%d body=%s", err, calls.Load(), status, reply)
				}
			}
		})
	}
}

func TestKnownPlanDiscoveryKeepsTransport(t *testing.T) {
	for _, plan := range []string{"go", "pro", ""} {
		t.Run(plan, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/alpha/billing/subscriptions":
					if plan == "" {
						io.WriteString(w, `{"data":null}`)
					} else {
						io.WriteString(w, `{"data":{"planId":"individual-`+plan+`","status":"active"}}`)
					}
				case "/alpha/billing/credits":
					io.WriteString(w, `{"credits":{"monthlyCredits":10}}`)
				case "/provider/v1/models":
					if (r.Header.Get("Authorization") == "") != (plan == "go") {
						t.Error("catalog used the wrong plan authentication")
					}
					io.WriteString(w, `{"data":[{"id":"fixture","supported_endpoints":["/messages"]}]}`)
				default:
					t.Errorf("unexpected read %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL})
			raw, _, err := worker.call(sdk.MethodProviderModels, struct{}{}, testAuth())
			var result sdk.ModelsResult
			if err != nil || json.Unmarshal(raw, &result) != nil || len(result.Models) != 2 {
				t.Fatalf("known plan catalog failed: %s %v", raw, err)
			}
			want := "anthropic"
			if plan == "go" {
				want = "chat"
			}
			if len(result.Models[0].Protocols) != 1 || string(result.Models[0].Protocols[0]) != want {
				t.Fatalf("wrong plan protocol: %+v", result.Models[0])
			}
		})
	}
}
