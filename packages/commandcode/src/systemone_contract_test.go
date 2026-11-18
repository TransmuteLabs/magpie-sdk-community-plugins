package commandcode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type systemOneFixtures struct {
	Wire struct {
		Aliases []string        `json:"aliases"`
		Request json.RawMessage `json:"request"`
	} `json:"donorWire"`
	Refusal struct {
		Action         sdk.Error `json:"action"`
		Conversational sdk.Error `json:"conversationalExecute"`
		Operation      sdk.Error `json:"operationExecuteOnSDK16"`
	} `json:"currentModuleRefusal"`
}

func loadSystemOneFixtures(t *testing.T) systemOneFixtures {
	t.Helper()
	body, err := os.ReadFile("../docs/SYSTEMONE-SDK-FIXTURES.json")
	if err != nil {
		t.Fatal(err)
	}
	var value systemOneFixtures
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Wire.Aliases) != 4 || len(value.Wire.Request) == 0 {
		t.Fatal("pinned decision wire is missing")
	}
	return value
}

func TestSystemOneActionsCannotBypassOperation(t *testing.T) {
	spec := loadSystemOneFixtures(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "decision reached an unnegotiated transport", http.StatusInternalServerError)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	for _, path := range []struct {
		name     string
		method   string
		input    any
		expected sdk.Error
	}{
		{name: "explicit-action", method: sdk.MethodModuleCall, input: sdk.ModuleCallRequest{Action: "systemone", Input: spec.Wire.Request}, expected: spec.Refusal.Action},
		{name: "operation-cannot-borrow-chat", method: sdk.MethodProviderExecute, input: sdk.ExecuteRequest{Protocol: "chat", Operation: "systemone", Model: Jev, Body: spec.Wire.Request}, expected: spec.Refusal.Operation},
	} {
		t.Run(path.name, func(t *testing.T) {
			result, events, err := worker.call(path.method, path.input, testAuth())
			var failure *sdk.Error
			if !errors.As(err, &failure) || failure.Code != path.expected.Code || failure.Outcome != path.expected.Outcome {
				t.Fatalf("decision refusal lost its precise code/outcome: %v; want %s/%s", err, path.expected.Code, path.expected.Outcome)
			}
			if len(result) != 0 || len(events) != 0 || requests.Load() != 0 {
				t.Fatalf("unnegotiated decision had side effects: result=%s events=%d requests=%d", result, len(events), requests.Load())
			}
		})
	}
}

func TestJevAliasesCannotBecomeConversationModels(t *testing.T) {
	spec := loadSystemOneFixtures(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "Jev reached conversational transport", http.StatusInternalServerError)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	for _, alias := range spec.Wire.Aliases {
		t.Run(alias, func(t *testing.T) {
			catalog, err := json.Marshal(map[string]any{"data": []map[string]any{
				{"id": "synthetic-chat", "supported_endpoints": []string{"/v1/chat/completions"}},
				{"id": alias, "supported_endpoints": []string{"/v1/chat/completions"}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			models, err := ParseModels(catalog, Subscription{}, false, false)
			if err != nil || len(models) != 2 || models[0].ID != "synthetic-chat" ||
				models[1].ID != Jev || models[1].Kind != "decision" || len(models[1].Protocols) != 0 {
				t.Fatalf("Jev alias entered conversational discovery: %+v %v", models, err)
			}
			body, err := json.Marshal(map[string]any{"model": alias, "messages": []map[string]string{{"role": "user", "content": "ping"}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, protocol := range []string{"chat", "responses", "anthropic"} {
				t.Run(protocol, func(t *testing.T) {
					_, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: protocol, Model: alias, Body: body}, testAuth())
					var failure *sdk.Error
					if !errors.As(err, &failure) || failure.Code != spec.Refusal.Conversational.Code || failure.Outcome != spec.Refusal.Conversational.Outcome {
						t.Fatalf("Jev alias was not refused before dispatch: %v", err)
					}
					if len(events) != 0 || requests.Load() != 0 {
						t.Fatalf("Jev alias reached conversation execution: events=%d requests=%d", len(events), requests.Load())
					}
				})
			}
		})
	}
}
