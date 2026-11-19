package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func TestPublicSDKGoCatalogAndExecuteUseSelectedSnapshot(t *testing.T) {
	const live = `{"object":"list","data":[
		{"id":"step-5-preview","type":"chat","name":"Step-5 Preview",
			"limit":{"context":1000000,"input":1000000,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		{"id":"gemini-4-argon-contributor","type":"chat","name":"Gemini 4 Argon Contributor",
			"limit":{"context":1048576,"output":65536},
			"api":{"generate-content":{"base_url":"https://opencode.ai/inference/go/google/v1beta"}}},
		{"id":"minimax-m3","type":"chat","limit":{"context":1000000,"output":131072},
			"api":{"messages":{"base_url":"https://opencode.ai/inference/go/anthropic/v1"}}},
		{"id":"gpt-6-luna","type":"chat","limit":{"context":1050000,"output":128000},
			"api":{"responses":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		{"id":"wrong-lane","type":"chat","limit":{"context":1000000,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"unknown-bare","object":"model"},
		{"id":"kimi-k3","object":"model"}
	]}`
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	var failA, expireA atomic.Bool
	var lists, dispatches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/inference/go/v1/models" {
			lists.Add(1)
			if request.Method != http.MethodGet || request.Header.Get("User-Agent") != "opencode/latest/2.3.0/cli" {
				t.Error("Go catalog did not use the documented request")
			}
			switch request.Header.Get("Authorization") {
			case "Bearer access-a":
				if request.Header.Get("X-Opencode-Org-Id") != "org-a" {
					t.Error("Go catalog used the wrong workspace")
				}
				if expireA.Load() {
					clock.Add(int64(2 * time.Hour))
				}
				if failA.Load() {
					writer.WriteHeader(http.StatusServiceUnavailable)
				} else {
					io.WriteString(writer, live)
				}
			case "Bearer access-b":
				io.WriteString(writer, `{"object":"list","data":[]}`)
			case "Bearer access-rotated":
				writer.WriteHeader(http.StatusForbidden)
			default:
				t.Error("Go catalog used a foreign credential")
			}
			return
		}
		dispatches.Add(1)
		switch request.URL.Path {
		case "/inference/go/openai/v1/chat/completions":
			if request.Header.Get("Authorization") != "Bearer access-a" &&
				request.Header.Get("Authorization") != "Bearer access-rotated" {
				t.Error("Go Chat used a foreign credential")
			}
			io.WriteString(writer, `{"choices":[]}`)
		case "/inference/go/google/v1beta/models/gemini-4-argon-contributor:streamGenerateContent":
			if request.Header.Get("X-Goog-Api-Key") != "access-a" || request.URL.RawQuery != "alt=sse" {
				t.Error("Go Gemini used a foreign credential or an incorrect query")
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(writer, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n")
		default:
			t.Errorf("unknown Go wire route: %s", request.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	options := ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		Now: now, HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}
	harness := serveHarness(t, NewWindowModule(options), raw(moduleConfig{ProjectID: "global"}))
	accountA, accountB := credential(now(), "a", "org-a"), credential(now(), "b", "org-b")
	scopeA, scopeB := sdk.Scope{Provider: ProviderGo, Account: accountA.AccountID(ProviderGo), Generation: 1},
		sdk.Scope{Provider: ProviderGo, Account: accountB.AccountID(ProviderGo), Generation: 1}
	list := func(scope sdk.Scope, account Credentials) []sdk.Model {
		t.Helper()
		frame, _ := harness.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, raw(account))
		var result sdk.ModelsResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
			t.Fatalf("Go catalog request failed: %+v", frame.Error)
		}
		return result.Models
	}
	modelsA := list(scopeA, accountA)
	var ids []string
	for _, model := range modelsA {
		ids = append(ids, model.ID)
		if model.ID == "step-5-preview" && (model.ContextWindow != 1000000 || model.MaxOutput != 65536 || !slices.Equal(model.Protocols, []protocolext.Protocol{"chat"})) ||
			model.ID == "gemini-4-argon-contributor" && !slices.Equal(model.Protocols, []protocolext.Protocol{"gemini"}) {
			t.Errorf("Go discovery changed the native declaration: %+v", model)
		}
	}
	if !slices.Equal(ids, []string{"gemini-4-argon-contributor", "kimi-k3", "step-5-preview"}) {
		t.Errorf("Go catalog got %q from the selected list", ids)
	}
	if models := list(scopeB, accountB); len(models) != 0 {
		t.Errorf("Go empty list was replaced by another account or the bootstrap: %v", models)
	}
	step := sdk.ExecuteRequest{Protocol: "chat", Model: "step-5-preview",
		Body: json.RawMessage(`{"model":"step-5-preview","messages":[]}`)}
	pinned := sdk.ExecuteRequest{Protocol: "chat", Model: "kimi-k3",
		Body: json.RawMessage(`{"model":"kimi-k3","messages":[]}`)}
	gemini := sdk.ExecuteRequest{Protocol: "gemini", Model: "gemini-4-argon-contributor", Body: json.RawMessage(`{"contents":[{}]}`),
		Options: raw(sdk.ExecuteOptions{Path: "/v1beta/models/gemini-4-argon-contributor:streamGenerateContent", Query: "alt=sse"})}
	for _, request := range []sdk.ExecuteRequest{step, gemini} {
		frame, _ := harness.invoke(sdk.MethodProviderExecute, scopeA, request, raw(accountA))
		if frame.Error != nil || !harness.accepted[frame.ID] {
			t.Errorf("Go discovered model could not execute: %v", frame.Error)
		}
	}
	for _, request := range []sdk.ExecuteRequest{step, pinned} {
		frame, _ := harness.invoke(sdk.MethodProviderExecute, scopeB, request, raw(accountB))
		if frame.Error == nil || frame.Error.Outcome != "not-started" || harness.accepted[frame.ID] {
			t.Errorf("known empty Go catalog gained inference authority: %v", frame.Error)
		}
	}
	if lists.Load() != 2 || dispatches.Load() != 2 {
		t.Errorf("execute did not reuse the selected snapshots: lists=%d dispatches=%d", lists.Load(), dispatches.Load())
	}
	clock.Add(int64(2 * time.Minute))
	failA.Store(true)
	if models := list(scopeA, accountA); len(models) != len(modelsA) {
		t.Error("failed refresh discarded last-good Go catalog")
	}
	frame, _ := harness.invoke(sdk.MethodProviderExecute, scopeA, step, raw(accountA))
	if frame.Error != nil || !harness.accepted[frame.ID] {
		t.Error("failed refresh changed the executable snapshot", frame.Error)
	}
	rotated := accountA
	rotated.Access = "access-rotated"
	scopeA.Generation++
	if models := list(scopeA, rotated); len(models) != len(Models()) {
		t.Error("a generation without permission to list did not retain static compatibility")
	}
	frame, _ = harness.invoke(sdk.MethodProviderExecute, scopeA, step, raw(rotated))
	if frame.Error == nil || frame.Error.Outcome != "not-started" {
		t.Error("a new credential generation inherited a dynamic model", frame.Error)
	}
	frame, _ = harness.invoke(sdk.MethodProviderExecute, scopeA, pinned, raw(rotated))
	if frame.Error != nil || !harness.accepted[frame.ID] {
		t.Error("a missing list permission broke static Go inference", frame.Error)
	}
	failA.Store(false)
	fresh := newHarness(t, options)
	frame, _ = fresh.invoke(sdk.MethodProviderExecute, sdk.Scope{Provider: ProviderGo, Account: accountA.AccountID(ProviderGo), Generation: 1}, step, raw(accountA))
	if frame.Error != nil || !fresh.accepted[frame.ID] {
		t.Error("cold worker failed to recover a model discovered by the host", frame.Error)
	}
	expireA.Store(true)
	scopeA.Generation++
	started := dispatches.Load()
	frame, _ = fresh.invoke(sdk.MethodProviderExecute, scopeA, step, raw(accountA))
	if frame.Error == nil || frame.Error.Code != "auth_refresh_required" || frame.Error.Outcome != "not-started" ||
		fresh.accepted[frame.ID] || dispatches.Load() != started {
		t.Error("credential renewal during catalog lookup was treated as accepted inference", frame.Error)
	}
}
