package commandcode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestRequestPolicyOriginalFinalRefusal(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "anthropic", "systemone"} {
		t.Run(protocol, func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/alpha/billing/subscriptions" {
					io.WriteString(writer, `{"data":{"planId":"individual-pro","status":"active"}}`)
					return
				}
				sends.Add(1)
				writer.Header().Set("Retry-After", "3600")
				writer.WriteHeader(429)
				io.WriteString(writer, `{"error":{"code":"PREMIUM_CREDITS_EXHAUSTED","type":"payment","message":"PREMIUM_CREDITS_EXHAUSTED: insufficient credits"}}`)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL})
			input := sdk.ExecuteRequest{Protocol: protocol, Model: "fixture", Body: []byte(`{"model":"fixture"}`)}
			if protocol == "systemone" {
				input.Protocol, input.Operation, input.Model = "", sdk.OperationSystemOne, Jev
				input.Body = []byte(`{"model":"typesafe/jev"}`)
			}
			result, events, err := worker.call(sdk.MethodProviderExecute, input, testAuth())
			var final sdk.ExecuteResult
			if err != nil || json.Unmarshal(result, &final) != nil || final.Refusal == nil {
				t.Fatalf("whole preflight refusal lost final policy facts: error=%v result=%s", err, result)
			}
			facts := final.Refusal.Facts
			if final.Status != 429 || final.Outcome != "not-started" || !facts.Complete || facts.Status != 429 ||
				facts.Code != "PREMIUM_CREDITS_EXHAUSTED" || facts.Type != "payment" ||
				facts.Message != "PREMIUM_CREDITS_EXHAUSTED: insufficient credits" || facts.Headers["Retry-After"] != "3600" ||
				final.Refusal.Evidence != "rejected-before-effect" || sends.Load() != 1 {
				t.Fatalf("refusal lost original response or repeated dispatch: %+v sends=%d", final, sends.Load())
			}
			if len(events) != 0 {
				t.Fatal("preflight refusal crossed acceptance/output fence", events)
			}
		})
	}
}

func TestRequestPolicyReadOnlySubscriptionUsesOnlyHostPermit(t *testing.T) {
	var reads, sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/alpha/billing/subscriptions" {
			if reads.Add(1) == 1 {
				writer.Header().Set("Retry-After", "0")
				writer.WriteHeader(503)
				io.WriteString(writer, `{"error":{"type":"temporary","message":"fixture read refused"}}`)
				return
			}
			io.WriteString(writer, `{"data":{"planId":"individual-pro","status":"active"}}`)
			return
		}
		sends.Add(1)
		io.WriteString(writer, `{"id":"paid","choices":[]}`)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL})
	callbacks := 0
	worker.recover = func(request sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult {
		callbacks++
		if request.Step != "commandcode.subscription" || request.Evidence != "read-only" ||
			request.Facts.Status != 503 || request.Facts.Type != "temporary" || !request.Facts.Complete {
			t.Fatal("read callback lost its named original facts", request)
		}
		return sdk.RequestRecoveryResult{Step: request.Step, Action: "wait", Retry: true}
	}
	_, _, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "fixture", Body: []byte(`{"model":"fixture"}`)}, testAuth())
	if err != nil || callbacks != 1 || reads.Load() != 2 || sends.Load() != 1 {
		t.Fatal("read permit repeated business work or bypassed host", err, callbacks, reads.Load(), sends.Load())
	}
}

func TestRequestPolicyReadOnlySubscriptionPreservesHeadersWithoutTrustedBody(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "empty"},
		{name: "malformed", body: `{"error":`},
		{name: "unrecognized", body: `{"message":"slow down"}`},
		{name: "credential", body: `{"error":{"message":"fixture-key"}}`},
		{name: "invalid field", body: `{"error":{"code":"rate_limited","message":42}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reads, sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/alpha/billing/subscriptions" {
					sends.Add(1)
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				reads.Add(1)
				writer.Header().Set("Retry-After", "3600")
				writer.Header().Set("X-RateLimit-Reset-Requests", "50m")
				writer.Header().Set("Set-Cookie", "private=fixture-key")
				writer.WriteHeader(http.StatusTooManyRequests)
				io.WriteString(writer, test.body)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL})
			callbacks := 0
			worker.recover = func(request sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult {
				callbacks++
				facts := request.Facts
				if request.Step != "commandcode.subscription" || request.Evidence != "" ||
					facts.Status != http.StatusTooManyRequests || facts.Complete ||
					facts.Code != "" || facts.Type != "" || facts.Message != "" ||
					len(facts.Headers) != 2 || facts.Headers["Retry-After"] != "3600" || facts.Headers["X-Ratelimit-Reset-Requests"] != "50m" {
					t.Errorf("account refusal lost validated timing or forwarded untrusted body facts: %+v", request)
				}
				return sdk.RequestRecoveryResult{Step: request.Step, Action: "terminal"}
			}
			_, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "fixture", Body: []byte(`{"model":"fixture"}`)}, testAuth())
			var failure *sdk.Error
			if !errors.As(err, &failure) || failure.Outcome != "not-started" || callbacks != 1 ||
				reads.Load() != 1 || sends.Load() != 0 || len(events) != 0 {
				t.Fatal("account refusal repeated a request or started inference", err, callbacks, reads.Load(), sends.Load(), events)
			}
		})
	}
}

func TestRequestPolicyUntrustedHeadersCannotAuthorizeRefusal(t *testing.T) {
	body := []byte(`{"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: gone"}}`)
	for name, values := range map[string][]string{
		"duplicate": {"3600", "5"},
		"control":   {"3600\n"},
		"oversized": {strings.Repeat("1", 257)},
		"secret":    {"fixture-key"},
	} {
		t.Run(name, func(t *testing.T) {
			headers := http.Header{"Retry-After": values, "X-Ratelimit-Reset-Requests": {"3600"}}
			facts, complete := responseFailure(http.StatusTooManyRequests, headers, body, "fixture-key")
			if complete || facts.Complete || len(facts.Headers) != 0 ||
				facts.Code != "" || facts.Type != "" || facts.Message != "" {
				t.Fatal("invalid timing headers yielded trusted refusal facts", facts)
			}
			if _, proved := commandRefusal(http.StatusTooManyRequests, headers, body, "fixture-key", "commandcode.execute"); proved {
				t.Fatal("invalid timing headers acquired paid refusal authority")
			}
		})
	}
}

func TestRequestPolicyAmbiguityRetainsAcceptedFence(t *testing.T) {
	for name, body := range map[string]string{
		"unknown":        `{"error":{"type":"unknown","message":"usage limit reached"}}`,
		"accepted field": `{"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: gone"},"accepted":true}`,
		"output field":   `{"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: gone"},"output":"paid"}`,
		"duplicate":      `{"error":{"message":"x","message":"PREMIUM_CREDITS_EXHAUSTED: gone"}}`,
		"duplicate root": `{"error":{},"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: gone"}}`,
		"null type":      `{"error":{"type":null,"message":"PREMIUM_CREDITS_EXHAUSTED: gone"}}`,
		"credential":     `{"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: fixture-key"}}`,
		"numeric code":   `{"error":{"code":1234,"message":"PREMIUM_CREDITS_EXHAUSTED: gone"}}`,
		"truncated":      `{"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: gone"`,
		"oversized":      `{"error":{"message":"PREMIUM_CREDITS_EXHAUSTED: ` + strings.Repeat("x", 65536) + `"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/alpha/billing/subscriptions" {
					io.WriteString(writer, `{"data":{"planId":"individual-pro","status":"active"}}`)
					return
				}
				sends.Add(1)
				writer.WriteHeader(429)
				io.WriteString(writer, body)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL})
			raw, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "fixture", Body: []byte(`{"model":"fixture"}`)}, testAuth())
			var final sdk.ExecuteResult
			var failure *sdk.Error
			if err == nil {
				if json.Unmarshal(raw, &final) != nil || final.Refusal != nil {
					t.Fatal("ambiguous refusal acquired routing authority", string(raw))
				}
			} else if !errors.As(err, &failure) || failure.Outcome != "uncertain" {
				t.Fatal("ambiguous refusal lost uncertainty", err)
			}
			accepted := 0
			for _, event := range events {
				if event.Event == "accepted" {
					accepted++
				}
			}
			if accepted != 1 || sends.Load() != 1 {
				t.Fatal("ambiguous execution lost accepted fence or repeated", accepted, sends.Load())
			}
		})
	}
}
