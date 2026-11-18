package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func ownAgentModelID() string {
	digest := sha256.Sum256([]byte("opencode\x00" + ProviderGo))
	return "extension-" + hex.EncodeToString(digest[:12]) + "/glm-5.2"
}

func plannedAgentModel(t *testing.T, format string, body []byte, id string) map[string]json.RawMessage {
	t.Helper()
	providerKey := "providers"
	if format == "v1" {
		providerKey = "provider"
	}
	var root, providers, provider, models, model map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(root[providerKey], &providers) != nil || json.Unmarshal(providers["magpie"], &provider) != nil ||
		json.Unmarshal(provider["models"], &models) != nil || json.Unmarshal(models[id], &model) != nil {
		t.Fatal("ordinary agent plan lost the selected catalog model")
	}
	return model
}

func agentModelEfforts(t *testing.T, format string, model map[string]json.RawMessage) []string {
	t.Helper()
	if format == "v1" {
		var variants map[string]struct {
			Effort string `json:"reasoningEffort"`
		}
		if json.Unmarshal(model["variants"], &variants) != nil {
			t.Fatal("invalid legacy effort variants")
		}
		efforts := []string{}
		for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
			if variant, exists := variants[effort]; exists {
				if variant.Effort != effort {
					t.Fatal("legacy effort variant changed its native setting")
				}
				efforts = append(efforts, effort)
			}
		}
		return efforts
	}
	var variants []struct {
		ID       string `json:"id"`
		Settings struct {
			Effort string `json:"reasoningEffort"`
		} `json:"settings"`
	}
	if json.Unmarshal(model["variants"], &variants) != nil {
		t.Fatal("invalid native effort variants")
	}
	efforts := []string{}
	for _, variant := range variants {
		if variant.ID != variant.Settings.Effort {
			t.Fatal("native effort variant changed its setting")
		}
		efforts = append(efforts, variant.ID)
	}
	return efforts
}

func TestAgentEffectiveHostContext(t *testing.T) {
	id := ownAgentModelID()
	for _, format := range []string{"v1", "v2"} {
		t.Run(format, func(t *testing.T) {
			host := serveHarness(t, NewWindowModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global", AgentFormat: format}))
			options := map[string]any{
				"gateway": "http://127.0.0.1:3425", "key": "synthetic-only", "field": "model", "value": "magpie/" + id,
				"models": []any{
					map[string]any{"value": "magpie/" + id, "label": "Effective own", "context": 32768},
					map[string]any{"value": "magpie/group/executor", "label": "Effective group", "context": 65536},
				},
			}
			frame, _ := host.invoke(sdk.MethodAgentPlan, sdk.Scope{}, sdk.AgentPlanRequest{
				ID: AgentID, Action: "set", Options: raw(options), Settings: agentSettings(nil),
				Binding: &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}},
			}, nil)
			if frame.Error != nil {
				t.Fatalf("ordinary agent plan failed: %+v", frame.Error)
			}
			var plan sdk.AgentPlan
			if json.Unmarshal(frame.Result, &plan) != nil || len(plan.Files) != 1 {
				t.Fatal("ordinary agent plan lost its checked file")
			}
			for modelID, expected := range map[string]int64{id: 32768, "group/executor": 65536} {
				var limit struct {
					Context int64 `json:"context"`
				}
				model := plannedAgentModel(t, format, plan.Files[0].Body, modelID)
				if json.Unmarshal(model["limit"], &limit) != nil || limit.Context != expected {
					t.Errorf("%s effective context = %d, want host %d", modelID, limit.Context, expected)
				}
			}
		})
	}
}

func TestAgentEffectiveHostMetadata(t *testing.T) {
	id := ownAgentModelID()
	for _, format := range []string{"v1", "v2"} {
		t.Run(format, func(t *testing.T) {
			host := serveHarness(t, NewWindowModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global", AgentFormat: format}))
			options := map[string]any{
				"gateway": "http://127.0.0.1:3425", "key": "synthetic-only", "field": "model", "value": "magpie/" + id,
				"models": []any{
					map[string]any{"value": "magpie/" + id, "context": 32768, "maxOutput": 4096, "efforts": []string{"low", "high"}, "defaultEffort": "low"},
					map[string]any{"value": "magpie/group/executor", "context": 65536, "maxOutput": 8192, "efforts": []string{"medium"}, "defaultEffort": "medium"},
				},
			}
			frame, _ := host.invoke(sdk.MethodAgentPlan, sdk.Scope{}, sdk.AgentPlanRequest{
				ID: AgentID, Action: "set", Options: raw(options), Settings: agentSettings(nil),
				Binding: &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}},
			}, nil)
			if frame.Error != nil {
				t.Fatalf("ordinary agent plan failed: %+v", frame.Error)
			}
			var plan sdk.AgentPlan
			if json.Unmarshal(frame.Result, &plan) != nil || len(plan.Files) != 1 {
				t.Fatal("ordinary agent plan lost its checked file")
			}
			for _, expected := range []struct {
				id      string
				output  int64
				efforts []string
				current string
			}{
				{id, 4096, []string{"low", "high"}, "low"},
				{"group/executor", 8192, []string{"medium"}, "medium"},
			} {
				model := plannedAgentModel(t, format, plan.Files[0].Body, expected.id)
				var limit struct {
					Output int64 `json:"output"`
				}
				if json.Unmarshal(model["limit"], &limit) != nil || limit.Output != expected.output {
					t.Errorf("%s effective output = %d, want host %d", expected.id, limit.Output, expected.output)
				}
				if _, exists := model["variants"]; !exists {
					t.Errorf("%s lost effective effort variants", expected.id)
				} else if actual := agentModelEfforts(t, format, model); !reflect.DeepEqual(actual, expected.efforts) {
					t.Errorf("%s effective efforts = %v, want host %v", expected.id, actual, expected.efforts)
				}
				optionKey := "settings"
				if format == "v1" {
					optionKey = "options"
				}
				var settings struct {
					Effort string `json:"reasoningEffort"`
				}
				if json.Unmarshal(model[optionKey], &settings) != nil || settings.Effort != expected.current {
					t.Errorf("%s native default setting = %q, want %q", expected.id, settings.Effort, expected.current)
				}
			}
		})
	}
}

func invokeAgentMetadataPlan(t *testing.T, host *sdkHarness, action string, options map[string]any,
	body []byte, binding *sdk.AgentBindingSnapshot,
) sdk.AgentPlan {
	t.Helper()
	frame, _ := host.invoke(sdk.MethodAgentPlan, sdk.Scope{}, sdk.AgentPlanRequest{
		ID: AgentID, Action: action, Options: raw(options), Settings: agentSettings(body), Binding: binding,
	}, nil)
	if frame.Error != nil {
		t.Fatalf("ordinary metadata %s failed: %+v", action, frame.Error)
	}
	var plan sdk.AgentPlan
	if json.Unmarshal(frame.Result, &plan) != nil || plan.Binding == nil {
		t.Fatal("ordinary metadata plan lost its binding")
	}
	return plan
}

func TestAgentMetadataUnknownAndEmptyEfforts(t *testing.T) {
	for _, format := range []string{"v1", "v2"} {
		for _, unknown := range []bool{true, false} {
			name := "empty"
			if unknown {
				name = "unknown"
			}
			t.Run(format+"/"+name, func(t *testing.T) {
				host := serveHarness(t, NewWindowModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global", AgentFormat: format}))
				var efforts []string
				if !unknown {
					efforts = []string{}
				}
				id := ownAgentModelID()
				options := map[string]any{
					"gateway": "http://127.0.0.1:3425", "key": "synthetic-only",
					"models":         []any{map[string]any{"value": "magpie/" + id, "efforts": efforts}},
					"decisionModels": []sdk.AgentDecisionModel{},
				}
				binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{"model": "magpie/" + id}}
				plan := invokeAgentMetadataPlan(t, host, "connect", options, nil, binding)
				model := plannedAgentModel(t, format, plan.Files[0].Body, id)
				if _, exists := model["limit"]; exists {
					t.Fatal("unknown effective limits inherited a static own-model limit")
				}
				_, exists := model["variants"]
				if exists == unknown {
					t.Fatal("unknown and explicitly empty efforts were conflated")
				}
				if !unknown && len(agentModelEfforts(t, format, model)) != 0 {
					t.Fatal("explicit empty efforts regained static variants")
				}
				binding.Connected, binding.Revision = true, "1"
				options["priorModels"] = []any{map[string]any{"value": "magpie/" + id}}
				noop := invokeAgentMetadataPlan(t, host, "sync", options, plan.Files[0].Body, binding)
				if len(noop.Files) != 0 {
					t.Fatal("prior snapshot with omitted unknown efforts restored static model limits")
				}
			})
		}
	}
}

func TestAgentMetadataSyncUsesPriorModelsAndRetainsUserOverrides(t *testing.T) {
	for _, format := range []string{"v1", "v2"} {
		t.Run(format, func(t *testing.T) {
			host := serveHarness(t, NewWindowModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global", AgentFormat: format}))
			id := ownAgentModelID()
			model := map[string]any{"value": "magpie/" + id, "label": "Effective own", "context": 32768,
				"maxOutput": 4096, "efforts": []string{"low", "high"}, "defaultEffort": "low"}
			options := map[string]any{
				"gateway": "http://127.0.0.1:3425", "key": "synthetic-only", "models": []any{model},
				"decisionModels": []sdk.AgentDecisionModel{{ID: "fixture-decision", Name: "Decision only",
					Provider: "fixture", Endpoints: []string{"/v1/systemone"}}},
			}
			binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{"model": "magpie/" + id}}
			plan := invokeAgentMetadataPlan(t, host, "connect", options, nil, binding)
			body := plan.Files[0].Body
			binding.Connected, binding.Revision, binding.Values = true, "1", plan.Binding.Values
			if contains := string(body); len(contains) == 0 || strings.Contains(contains, "fixture-decision") {
				t.Fatal("decision-only catalog entered the conversational configuration")
			}
			options["priorModels"] = []json.RawMessage{raw(model)}
			inspect, _ := host.invoke(sdk.MethodAgentInspect, sdk.Scope{}, sdk.AgentInspectRequest{
				ID: AgentID, Binding: binding, Options: raw(options), Settings: agentSettings(body),
			}, nil)
			if inspect.Error != nil {
				t.Fatalf("ordinary inspection refused the API9 prior/decision envelope: %+v", inspect.Error)
			}
			noop := invokeAgentMetadataPlan(t, host, "sync", options, body, binding)
			if len(noop.Files) != 0 {
				t.Fatal("unchanged effective metadata rewrote the checked config")
			}
			next := map[string]any{"value": "magpie/" + id, "label": "Effective own", "context": 65536,
				"maxOutput": 8192, "efforts": []string{"medium"}, "defaultEffort": "medium"}
			options["models"] = []any{next}
			sync := invokeAgentMetadataPlan(t, host, "sync", options, body, binding)
			body = sync.Files[0].Body
			effective := plannedAgentModel(t, format, body, id)
			var limit struct {
				Context int64 `json:"context"`
				Output  int64 `json:"output"`
			}
			json.Unmarshal(effective["limit"], &limit)
			if limit.Context != 65536 || limit.Output != 8192 ||
				!reflect.DeepEqual(agentModelEfforts(t, format, effective), []string{"medium"}) {
				t.Fatal("prior generated values blocked an ordinary host metadata edit")
			}
			providerKey, optionKey := "providers", "settings"
			if format == "v1" {
				providerKey, optionKey = "provider", "options"
			}
			var root, providers, provider, models map[string]json.RawMessage
			json.Unmarshal(body, &root)
			json.Unmarshal(root[providerKey], &providers)
			json.Unmarshal(providers["magpie"], &provider)
			json.Unmarshal(provider["models"], &models)
			effective["limit"] = raw(map[string]any{"context": 65536, "output": 7000, "personal": json.Number("9007199254740993")})
			effective[optionKey] = raw(map[string]any{"reasoningEffort": "high", "personal": "keep"})
			if format == "v2" {
				effective["variants"] = raw([]any{
					map[string]any{"id": "medium", "settings": map[string]any{"reasoningEffort": "medium", "personal": true}},
					map[string]any{"id": "personal", "settings": map[string]any{"reasoningEffort": "low"}},
				})
			} else {
				effective["variants"] = raw(map[string]any{
					"medium":   map[string]any{"reasoningEffort": "medium", "personal": true},
					"personal": map[string]any{"reasoningEffort": "low"},
				})
			}
			models[id] = raw(effective)
			provider["models"] = raw(models)
			providers["magpie"] = raw(provider)
			root[providerKey] = raw(providers)
			body = raw(root)
			options["priorModels"] = []json.RawMessage{raw(next)}
			options["models"] = []any{map[string]any{"value": "magpie/" + id, "context": 98304,
				"maxOutput": 16384, "efforts": []string{}}}
			edited := invokeAgentMetadataPlan(t, host, "sync", options, body, binding)
			effective = plannedAgentModel(t, format, edited.Files[0].Body, id)
			json.Unmarshal(effective["limit"], &limit)
			if limit.Context != 98304 || limit.Output != 7000 ||
				!strings.Contains(string(effective["limit"]), "9007199254740993") ||
				!strings.Contains(string(effective["variants"]), "personal") ||
				!strings.Contains(string(effective[optionKey]), `"high"`) {
				t.Fatal("host sync erased independent output, settings or variant edits")
			}
			noop = invokeAgentMetadataPlan(t, host, "sync", options, sync.Files[0].Body, binding)
			effective = plannedAgentModel(t, format, noop.Files[0].Body, id)
			if len(agentModelEfforts(t, format, effective)) != 0 || len(effective[optionKey]) != 0 {
				t.Fatal("an invalidated generated default or effort survived an ordinary edit")
			}
		})
	}
}

func TestAgentMetadataRejectsInvalidPublicEnvelopes(t *testing.T) {
	host := serveHarness(t, NewWindowModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global"}))
	id := ownAgentModelID()
	for _, invalid := range []map[string]any{
		{"models": []any{map[string]any{"value": "magpie/" + id, "maxOutput": -1}}},
		{"models": []any{map[string]any{"value": "magpie/" + id, "context": json.Number("9007199254740992")}}},
		{"models": []any{map[string]any{"value": "magpie/" + id, "efforts": []string{"low", "low"}}}},
		{"models": []any{map[string]any{"value": "magpie/" + id, "efforts": []string{}, "defaultEffort": "high"}}},
		{"priorModels": []any{map[string]any{"value": "magpie/" + id, "efforts": []string{"invented"}}}},
		{"decisionModels": nil},
		{"decisionModels": []any{map[string]any{"id": "secret", "name": "Secret", "provider": "fixture",
			"endpoints": []string{"/v1/systemone"}, "credentials": "refused"}}},
		{"decisionModels": []sdk.AgentDecisionModel{{ID: "wrong", Name: "Wrong", Provider: "fixture", Endpoints: []string{"/v1/chat/completions"}}}},
	} {
		options := map[string]any{"gateway": "http://127.0.0.1:3425", "key": "synthetic-only",
			"models": []any{map[string]any{"value": "magpie/" + id}}}
		for key, value := range invalid {
			options[key] = value
		}
		frame, _ := host.invoke(sdk.MethodAgentPlan, sdk.Scope{}, sdk.AgentPlanRequest{
			ID: AgentID, Action: "connect", Options: raw(options), Settings: agentSettings(nil),
			Binding: &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{"model": "magpie/" + id}},
		}, nil)
		if frame.Error == nil || frame.Error.Outcome != "not-started" {
			t.Fatal("invalid public metadata produced an agent config plan")
		}
	}
}
