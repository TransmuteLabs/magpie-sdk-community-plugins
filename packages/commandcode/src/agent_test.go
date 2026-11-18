package commandcode

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAgentProfileIsExplicitAndPreservesUnknownSettings(t *testing.T) {
	agents := Manifest().Agents
	if len(agents) != 1 || agents[0].Profile == nil || agents[0].Profile.Target != "commandcode" {
		t.Fatal("native agent adoption must be explicit")
	}
	worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
	settings := json.RawMessage(`{"files":[{"path":".commandcode/settings.json","body":"eyJ1c2VyT3B0aW9uIjp7ImtlZXAiOnRydWV9fQ==","beforeHash":"settings-hash","exists":true},{"path":".commandcode/providers.json","body":"e30=","beforeHash":"providers-hash","exists":true}]}`)
	options := json.RawMessage(`{"gateway":"http://127.0.0.1:3425","models":[{"value":"magpie/p/m","label":"Fixture","context":1000000}],"field":"model","value":"magpie/p/m"}`)
	raw, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: "connect", Settings: settings, Options: options,
		Binding: &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var plan sdk.AgentPlan
	if json.Unmarshal(raw, &plan) != nil || len(plan.Files) != 2 || plan.Binding == nil || !plan.Binding.Connected {
		t.Fatalf("invalid adoption: %s", raw)
	}
	var settingsObject map[string]any
	json.Unmarshal(plan.Files[0].Body, &settingsObject)
	if settingsObject["userOption"] == nil || settingsObject["modelProvider"] != "magpie" {
		t.Fatalf("settings destroyed: %s", plan.Files[0].Body)
	}
	var providers map[string]any
	json.Unmarshal(plan.Files[1].Body, &providers)
	entry := providers["provider"].(map[string]any)["magpie"].(map[string]any)
	model := entry["models"].(map[string]any)["p/m"].(map[string]any)
	if model["contextWindow"] != float64(1000000) || entry["apiKey"] != false {
		t.Fatalf("large model lost its window: %s", plan.Files[1].Body)
	}
}

func TestAgentRefusesForeignEndpointAndInvalidEffort(t *testing.T) {
	settings, _ := json.Marshal(map[string]any{"files": []any{
		map[string]any{"path": ".commandcode/settings.json", "body": []byte(`{"model":"magpie/p/m","modelProvider":"magpie"}`), "beforeHash": "settings", "exists": true},
		map[string]any{"path": ".commandcode/providers.json", "body": []byte(`{"provider":{"magpie":{"baseURL":"https://foreign.invalid/v1"}}}`), "beforeHash": "providers", "exists": true},
	}})
	options := json.RawMessage(`{"gateway":"http://127.0.0.1:3425","models":[{"value":"magpie/p/m"}],"field":"model","value":"magpie/p/m"}`)
	params, _ := json.Marshal(sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: "connect", Settings: settings, Options: options,
		Binding: &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}})
	handler := Module().Handlers[sdk.MethodAgentPlan]
	if handler == nil {
		t.Fatal("agent planner is not implemented")
	}
	if _, err := handler(context.Background(), sdk.Call{Params: params}, nil); err == nil {
		t.Fatal("foreign magpie endpoint overwritten")
	}
}

func TestAgentSyncCannotAdoptAnUnownedNativeAgent(t *testing.T) {
	worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
	result, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: "sync",
		Binding: &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}}, nil)
	var plan sdk.AgentPlan
	if err != nil || json.Unmarshal(result, &plan) != nil || len(plan.Files) != 0 || plan.Binding == nil || plan.Binding.Connected {
		t.Fatalf("implicit adoption: %s %v", result, err)
	}
}
