package commandcode

import (
	"context"
	sdk "github.com/yetone/magpie/pkg/extension"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAstraUnknownPlanDoesNotStartAlternateTransport(t *testing.T) {
	for _, known := range []bool{true, false} {
		name := "unknown-plan"
		if known {
			name = "known-go-plan"
		}
		t.Run(name, func(t *testing.T) {
			var calls, reads atomic.Int64
			path := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/alpha/billing/subscriptions" {
					reads.Add(1)
					if known {
						io.WriteString(w, `{"data":{"planId":"individual-go","status":"active"}}`)
					} else {
						w.WriteHeader(503)
					}
					return
				}
				calls.Add(1)
				path <- r.URL.Path
				if r.URL.Path == "/alpha/generate" {
					io.WriteString(w, "data: {\"type\":\"finish\",\"finishReason\":\"stop\",\"totalUsage\":{\"inputTokens\":1,\"outputTokens\":0}}\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"fixture","choices":[]}`)
				}
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai", WorkingDirectory: "/synthetic/project"})
			callbacks := 0
			if !known {
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
			result, _, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "fixture", Body: []byte(`{"model":"fixture","messages":[{"role":"user","content":"hi"}]}`)}, testAuth())
			wantCallbacks := 0
			if !known {
				wantCallbacks = 1
			}
			if callbacks != wantCallbacks || reads.Load() != 1 {
				t.Fatalf("subscription bypassed denial or repeated: callbacks=%d reads=%d", callbacks, reads.Load())
			}
			if known {
				if err != nil || calls.Load() != 1 {
					t.Fatalf("known plan control failed: calls=%d err=%v", calls.Load(), err)
				}
				if got := <-path; got != "/alpha/generate" {
					t.Fatalf("known Go selected %s", got)
				}
			} else if calls.Load() != 0 {
				t.Fatalf("unknown plan sent accepted inference to %s, calls=%d terminal=%s error=%v", <-path, calls.Load(), result, err)
			}
		})
	}
}

func TestAstraExpiredKeyFlowsReleaseAdmission(t *testing.T) {
	c := &controller{ctx: context.Background(), flows: map[string]*authFlow{}}
	for i := 0; i < 32; i++ {
		_, flow, err := c.addFlow("synthetic-key")
		if err != nil {
			t.Fatal(err)
		}
		// Same state reached after the declared ten-minute expiry. Non-browser
		// authStart returns before registering either cleanup timer or callback.
		flow.expires = time.Now().Add(-time.Minute)
	}
	if _, _, err := c.addFlow("synthetic-key"); err != nil {
		t.Fatalf("32 expired key sign-ins still occupy all admission slots: retained=%d error=%v", len(c.flows), err)
	}
}
