package commandcode

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAgentDecisionModelsStayOutsideConversationConfig(t *testing.T) {
	for _, test := range []struct {
		name   string
		models []sdk.AgentDecisionModel
	}{
		{name: "empty", models: []sdk.AgentDecisionModel{}},
		{name: "provider-and-group", models: []sdk.AgentDecisionModel{
			{ID: "commandcode-native/typesafe/jev", Name: "Jev", Provider: "commandcode-native",
				Endpoints: []string{"/v1/systemone", "/v1/decisions"}},
			{ID: "group/judges", Name: "Judges", Group: true, Endpoints: []string{"/v1/systemone"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
			files := []settingFile{
				{Path: ".commandcode/settings.json", Body: []byte(`{"theme":"keep"}`), BeforeHash: "settings", Exists: true},
				{Path: ".commandcode/providers.json", Body: []byte(`{"provider":{"mine":{"keep":true}}}`), BeforeHash: "providers", Exists: true},
			}
			settings, _ := json.Marshal(map[string]any{"files": files})
			models := []map[string]any{{"value": "magpie/p/chat", "label": "Conversation", "context": 100000,
				"note": "host picker", "icon": "fixture", "efforts": nil}}
			options := map[string]any{"gateway": "http://127.0.0.1:3425", "key": "fixture-gateway-key", "models": models}
			rawOptions, _ := json.Marshal(options)
			binding := &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}
			baseline, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk",
				Action: "connect", Settings: settings, Options: rawOptions, Binding: binding}, nil)
			if err != nil {
				t.Fatal("legacy options without decision discovery failed", err)
			}
			options["decisionModels"] = test.models
			rawOptions, _ = json.Marshal(options)
			raw, events, err := worker.call(sdk.MethodAgentInspect, sdk.AgentInspectRequest{ID: "commandcode-sdk",
				Settings: settings, Options: rawOptions}, nil)
			var inspection sdk.AgentInspectResult
			if err != nil || len(events) != 0 || json.Unmarshal(raw, &inspection) != nil || inspection.Connected {
				t.Fatalf("valid public decision DTO broke inspection: %s (%v)", raw, err)
			}
			raw, events, err = worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk",
				Action: "connect", Settings: settings, Options: rawOptions, Binding: binding}, nil)
			if err != nil || len(events) != 0 || !bytes.Equal(raw, baseline) {
				t.Fatalf("decision discovery changed conversational configuration: %s (%v); baseline %s", raw, err, baseline)
			}
			var plan sdk.AgentPlan
			if json.Unmarshal(raw, &plan) != nil || plan.Binding == nil || !plan.Binding.Connected || len(plan.Files) != 2 {
				t.Fatalf("invalid conversation plan: %s", raw)
			}
			for _, change := range plan.Files {
				for index := range files {
					if files[index].Path == change.Path {
						files[index].Body = change.Body
					}
				}
			}
			settings, _ = json.Marshal(map[string]any{"files": files})
			raw, _, err = worker.call(sdk.MethodAgentInspect, sdk.AgentInspectRequest{ID: "commandcode-sdk",
				Settings: settings, Options: rawOptions}, nil)
			if err != nil || json.Unmarshal(raw, &inspection) != nil || !inspection.Connected ||
				inspection.Values["model"] != "magpie/p/chat" {
				t.Fatalf("decision discovery broke connected inspection: %s (%v)", raw, err)
			}
			binding.Revision, binding.Connected, binding.Values = "1", true, plan.Binding.Values
			plan = sdk.AgentPlan{}
			raw, _, err = worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk",
				Action: "sync", Settings: settings, Options: rawOptions, Binding: binding}, nil)
			if err != nil || json.Unmarshal(raw, &plan) != nil || len(plan.Files) != 0 {
				t.Fatalf("decision discovery caused a conversational sync write: %s (%v)", raw, err)
			}
			options["field"], options["value"] = "model", "magpie/commandcode-native/typesafe/jev"
			rawOptions, _ = json.Marshal(options)
			if _, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk",
				Action: "set", Settings: settings, Options: rawOptions, Binding: binding}, nil); err == nil {
				t.Fatal("Jev discovery granted conversational model selection")
			}
			delete(options, "field")
			delete(options, "value")
			options["models"] = []agentModelOption{}
			rawOptions, _ = json.Marshal(options)
			if _, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk",
				Action: "connect", Settings: settings, Options: rawOptions, Binding: binding}, nil); err == nil {
				t.Fatal("decision discovery replaced an empty conversational catalog")
			}
		})
	}
}

func TestAgentDecisionModelsRejectInvalidPublicDTO(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  json.RawMessage
	}{
		{name: "null", raw: json.RawMessage(`null`)},
		{name: "object", raw: json.RawMessage(`{}`)},
		{name: "null-entry", raw: json.RawMessage(`[null]`)},
		{name: "missing-id", raw: json.RawMessage(`[{"name":"Judge","provider":"p","endpoints":["/v1/systemone"]}]`)},
		{name: "missing-name", raw: json.RawMessage(`[{"id":"p/judge","provider":"p","endpoints":["/v1/systemone"]}]`)},
		{name: "missing-owner", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","endpoints":["/v1/systemone"]}]`)},
		{name: "ambiguous-owner", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","group":true,"endpoints":["/v1/systemone"]}]`)},
		{name: "missing-endpoint", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","endpoints":[]}]`)},
		{name: "chat-endpoint", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","endpoints":["/v1/chat/completions"]}]`)},
		{name: "external-endpoint", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","endpoints":["https://vendor.invalid/v1/systemone"]}]`)},
		{name: "duplicate-endpoint", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","endpoints":["/v1/systemone","/v1/systemone"]}]`)},
		{name: "credential", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","endpoints":["/v1/systemone"],"apiKey":"fixture-private"}]`)},
		{name: "account", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":"p","endpoints":["/v1/systemone"],"account":{"id":"fixture"}}]`)},
		{name: "invalid-provider-type", raw: json.RawMessage(`[{"id":"p/judge","name":"Judge","provider":false,"endpoints":["/v1/systemone"]}]`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
			settings, _ := json.Marshal(map[string]any{"files": []settingFile{
				{Path: ".commandcode/settings.json", Body: []byte(`{}`), BeforeHash: "settings", Exists: true},
				{Path: ".commandcode/providers.json", Body: []byte(`{}`), BeforeHash: "providers", Exists: true},
			}})
			options, _ := json.Marshal(map[string]any{"gateway": "http://127.0.0.1:3425", "decisionModels": test.raw,
				"models": []map[string]any{{"value": "magpie/p/chat"}}})
			for _, method := range []string{sdk.MethodAgentInspect, sdk.MethodAgentPlan} {
				t.Run(method, func(t *testing.T) {
					var request any = sdk.AgentInspectRequest{ID: "commandcode-sdk", Settings: settings, Options: options}
					if method == sdk.MethodAgentPlan {
						request = sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: "connect", Settings: settings, Options: options,
							Binding: &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}}
					}
					_, events, err := worker.call(method, request, nil)
					var refusal *sdk.Error
					if !errors.As(err, &refusal) || refusal.Code != "invalid_params" || refusal.Outcome != "not-started" || len(events) != 0 {
						t.Fatalf("invalid decision DTO reached %s: %v (%d events)", method, err, len(events))
					}
				})
			}
		})
	}
}
