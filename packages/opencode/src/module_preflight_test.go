package opencode

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestPublicSDKExecutionPreflightIsNotStarted(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	account := credential(time.Now(), "a", "org-a")
	scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
	for _, params := range []any{
		nil,
		[]string{},
		map[string]any{"unknown": "field"},
		sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
			Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`), Options: raw(map[string]string{"unknown": "field"})},
		sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
			Body:    []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`),
			Options: raw(map[string]string{"path": "/v1/chat/completions", "escapedPath": "/v1/chat%2Fcompletions"})},
	} {
		frame, _ := h.invoke(sdk.MethodProviderExecute, scope, params, raw(account))
		if frame.Error == nil || frame.Error.Outcome != "not-started" || calls.Load() != 0 || h.accepted[frame.ID] {
			t.Fatalf("execution preflight became an uncertain or dispatched operation: %v", frame.Error)
		}
	}
}
