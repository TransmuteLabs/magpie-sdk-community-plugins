package commandcode

import (
	"encoding/json"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAgentEffectiveMetadataAndUserChoices(t *testing.T) {
	worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
	settings := []byte(`{"theme":"dark","model":"vendor/original","reasoningEffort":{"magpie/p/wide":"low","vendor/original":"max"}}`)
	providers := []byte(`{"provider":{"mine":{"keep":true},"magpie":{"models":{"p/wide":{"maxOutput":8192,"userNote":"keep"}}}}}`)
	originals := []settingFile{
		{Path: ".commandcode/settings.json", Body: settings, BeforeHash: "settings", Exists: true},
		{Path: ".commandcode/providers.json", Body: providers, BeforeHash: "providers", Exists: true},
	}
	models := []map[string]any{
		{"value": "magpie/p/wide", "context": 400000, "maxOutput": 16384, "efforts": []string{"low", "high"}, "defaultEffort": "high"},
		{"value": "magpie/p/fresh", "context": 400000, "maxOutput": 16384, "efforts": []string{"low", "high"}, "defaultEffort": "high"},
		{"value": "magpie/p/unknown", "efforts": nil},
		{"value": "magpie/p/empty", "efforts": []string{}},
	}
	var priorModels []map[string]any
	var last sdk.AgentPlan
	run := func(action, field, value string) (map[string]any, map[string]any) {
		t.Helper()
		files := []settingFile{
			{Path: originals[0].Path, Body: settings, BeforeHash: "settings", Exists: true},
			{Path: originals[1].Path, Body: providers, BeforeHash: "providers", Exists: true},
		}
		options, _ := json.Marshal(map[string]any{"gateway": "http://127.0.0.1:3425", "models": models,
			"previous": originals, "priorModels": priorModels, "field": field, "value": value})
		snapshots, _ := json.Marshal(map[string]any{"files": files})
		binding := &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}
		if last.Binding != nil {
			binding.Revision, binding.Connected, binding.Values = "1", true, last.Binding.Values
		}
		raw, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: action,
			Settings: snapshots, Options: options, Binding: binding}, nil)
		if err != nil || json.Unmarshal(raw, &last) != nil || last.Binding == nil {
			t.Fatalf("metadata action %s: %s (%v)", action, raw, err)
		}
		for _, file := range last.Files {
			switch file.Path {
			case originals[0].Path:
				settings = file.Body
			case originals[1].Path:
				providers = file.Body
			default:
				t.Fatal("agent changed an undeclared file")
			}
		}
		modelBytes, _ := json.Marshal(models)
		if err := json.Unmarshal(modelBytes, &priorModels); err != nil {
			t.Fatal(err)
		}
		var state, config map[string]any
		if json.Unmarshal(settings, &state) != nil || json.Unmarshal(providers, &config) != nil {
			t.Fatal("metadata action returned invalid JSON")
		}
		entry := config["provider"].(map[string]any)["magpie"].(map[string]any)
		return state, entry["models"].(map[string]any)
	}
	state, written := run("connect", "", "")
	wide := written["p/wide"].(map[string]any)
	fresh := written["p/fresh"].(map[string]any)
	if wide["maxOutput"] != float64(8192) || wide["userNote"] != "keep" || fresh["maxOutput"] != float64(16384) {
		t.Fatalf("effective limit or preexisting user limit lost: %s", providers)
	}
	if wide["reasoning"] != true || len(wide["reasoningEfforts"].([]any)) != 2 ||
		written["p/empty"].(map[string]any)["reasoning"] != false ||
		written["p/unknown"].(map[string]any)["maxOutput"] != nil {
		t.Fatalf("effective or unknown reasoning metadata lost: %s", providers)
	}
	efforts := state["reasoningEffort"].(map[string]any)
	if efforts["magpie/p/wide"] != "low" || efforts["magpie/p/fresh"] != "high" || efforts["vendor/original"] != "max" {
		t.Fatalf("valid user effort or new model default lost: %s", settings)
	}
	for _, model := range models[:2] {
		model["maxOutput"], model["efforts"] = 32768, []string{"low"}
		delete(model, "defaultEffort")
	}
	state, written = run("sync", "", "")
	if written["p/wide"].(map[string]any)["maxOutput"] != float64(8192) ||
		written["p/fresh"].(map[string]any)["maxOutput"] != float64(32768) ||
		state["reasoningEffort"].(map[string]any)["magpie/p/fresh"] != nil {
		t.Fatalf("sync lost overrides or retained an invalid default: %s %s", settings, providers)
	}
	var config map[string]any
	if json.Unmarshal(providers, &config) != nil {
		t.Fatal("invalid fixture config")
	}
	config["provider"].(map[string]any)["magpie"].(map[string]any)["models"].(map[string]any)["p/fresh"].(map[string]any)["maxOutput"] = 4096
	providers, _ = json.Marshal(config)
	delete(state["reasoningEffort"].(map[string]any), "magpie/p/wide")
	settings, _ = json.Marshal(state)
	for _, limit := range []int{65536, 131072} {
		for _, model := range models[:2] {
			model["maxOutput"], model["defaultEffort"] = limit, "low"
		}
		state, written = run("sync", "", "")
		if written["p/fresh"].(map[string]any)["maxOutput"] != float64(4096) ||
			state["reasoningEffort"].(map[string]any)["magpie/p/wide"] != nil || state["theme"] != "dark" {
			t.Fatalf("later sync adopted a user limit or replaced Auto: %s %s", settings, providers)
		}
	}
}

func TestAgentEffectiveMetadataRefusesInvalidDefaultAndEffort(t *testing.T) {
	for _, option := range []json.RawMessage{
		json.RawMessage(`{"value":"magpie/p/m","efforts":["low"],"defaultEffort":"high"}`),
		json.RawMessage(`{"value":"magpie/p/m","efforts":["low"],"maxOutput":-1}`),
		json.RawMessage(`{"value":"magpie/p/m","efforts":["low","low"]}`),
	} {
		t.Run(string(option), func(t *testing.T) {
			worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
			options, _ := json.Marshal(map[string]any{"gateway": "http://127.0.0.1:3425", "models": []json.RawMessage{option},
				"field": "effort", "value": "high"})
			settings, _ := json.Marshal(map[string]any{"files": []settingFile{
				{Path: ".commandcode/settings.json", Body: []byte(`{"model":"magpie/p/m"}`), BeforeHash: "settings", Exists: true},
				{Path: ".commandcode/providers.json", Body: []byte(`{}`), BeforeHash: "providers", Exists: true},
			}})
			if _, _, err := worker.call(sdk.MethodAgentPlan, sdk.AgentPlanRequest{ID: "commandcode-sdk", Action: "connect",
				Settings: settings, Options: options, Binding: &sdk.AgentBindingSnapshot{Target: "commandcode", Revision: "0", Values: map[string]string{}}}, nil); err == nil {
				t.Fatal("invalid effective model metadata or unoffered effort was accepted")
			}
		})
	}
}
