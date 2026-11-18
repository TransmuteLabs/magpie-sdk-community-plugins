package commandcode

import (
	"encoding/json"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAgentOffUsesVendorSpellingAcrossSyncAndDisconnect(t *testing.T) {
	worker := fixture(t, Config{APIURL: "http://127.0.0.1:1"})
	originals := []settingFile{
		{Path: ".commandcode/settings.json", Body: []byte(`{"theme":"keep","model":"vendor/original","reasoningEffort":{"vendor/original":"high"}}`), BeforeHash: "settings", Exists: true},
		{Path: ".commandcode/providers.json", Body: []byte(`{"provider":{"mine":{"keep":true}}}`), BeforeHash: "providers", Exists: true},
	}
	files := append([]settingFile(nil), originals...)
	models := []map[string]any{{"value": "magpie/p/m", "efforts": []string{"none", "high"}, "defaultEffort": "none"}}
	var priorModels []map[string]any
	binding := &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}
	run := func(action, field, value string) (map[string]any, map[string]any) {
		t.Helper()
		options, _ := json.Marshal(map[string]any{"gateway": "http://127.0.0.1:3425", "models": models,
			"priorModels": priorModels, "previous": originals, "field": field, "value": value})
		settings, _ := json.Marshal(map[string]any{"files": files})
		raw, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: action,
			Settings: settings, Options: options, Binding: binding}, nil)
		var plan sdk.AgentPlan
		if err != nil || json.Unmarshal(raw, &plan) != nil || plan.Binding == nil {
			t.Fatalf("%s %s=%s: %s %v", action, field, value, raw, err)
		}
		for _, change := range plan.Files {
			for index := range files {
				if files[index].Path == change.Path {
					files[index].Body = change.Body
				}
			}
		}
		binding = &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "1",
			Connected: plan.Binding.Connected, Values: plan.Binding.Values}
		priorModels = models
		var state, providers map[string]any
		json.Unmarshal(files[0].Body, &state)
		json.Unmarshal(files[1].Body, &providers)
		return state, providers
	}
	state, providers := run("connect", "", "")
	if state["reasoningEffort"].(map[string]any)["magpie/p/m"] != "off" || binding.Values["effort"] != "none" {
		t.Fatalf("host off default did not use the native spelling: %s", files[0].Body)
	}
	entry := providers["provider"].(map[string]any)["magpie"].(map[string]any)["models"].(map[string]any)["p/m"].(map[string]any)
	levels := entry["reasoningEfforts"].([]any)
	if len(levels) != 2 || levels[0] != "off" || levels[1] != "high" {
		t.Fatalf("vendor metadata lost off: %s", files[1].Body)
	}
	options, _ := json.Marshal(map[string]any{"gateway": "http://127.0.0.1:3425", "models": models})
	settings, _ := json.Marshal(map[string]any{"files": files})
	raw, _, err := worker.call(sdk.MethodAgentInspect, sdk.AgentInspectRequest{
		ID: "commandcode-sdk", Settings: settings, Options: options}, nil)
	var inspection sdk.AgentInspectResult
	if err != nil || json.Unmarshal(raw, &inspection) != nil || !inspection.Connected || inspection.Values["effort"] != "none" {
		t.Fatalf("inspection did not return canonical off: %s %v", raw, err)
	}
	state, _ = run("sync", "", "")
	if state["reasoningEffort"].(map[string]any)["magpie/p/m"] != "off" {
		t.Fatalf("sync discarded the valid vendor off choice: %s", files[0].Body)
	}
	state, _ = run("set", "effort", "")
	state, _ = run("sync", "", "")
	if state["reasoningEffort"].(map[string]any)["magpie/p/m"] != nil {
		t.Fatalf("sync replaced user Auto with an off default: %s", files[0].Body)
	}
	state, _ = run("set", "effort", "none")
	if state["reasoningEffort"].(map[string]any)["magpie/p/m"] != "off" || binding.Values["effort"] != "none" {
		t.Fatalf("explicit canonical off did not use vendor spelling: %s", files[0].Body)
	}
	state, _ = run("disconnect", "", "")
	efforts := state["reasoningEffort"].(map[string]any)
	if state["model"] != "vendor/original" || state["theme"] != "keep" || efforts["magpie/p/m"] != nil || efforts["vendor/original"] != "high" {
		t.Fatalf("disconnect lost original state or retained generated off: %s", files[0].Body)
	}
}
