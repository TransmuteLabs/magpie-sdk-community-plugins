package zcode

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func agentOptionsFixture(t *testing.T) map[string]any {
	t.Helper()
	request := agentTestRequest(nil, false)
	var options map[string]any
	if err := json.Unmarshal(request.Options, &options); err != nil {
		t.Fatal(err)
	}
	return options
}

func agentDecisionModelsFixture() []sdk.AgentDecisionModel {
	return []sdk.AgentDecisionModel{
		{ID: "decision-fixture/provider-judge", Name: "Provider judge", Provider: "decision-fixture",
			Endpoints: []string{"/v1/systemone", "/v1/decisions"}},
		{ID: "decision-fixture/group-judge", Name: "Group judge", Group: true,
			Endpoints: []string{"/v1/decisions"}},
	}
}

func TestAgentOptionsKeepTypedDecisionModelsSeparateFromChat(t *testing.T) {
	baseline := agentTestPlan(t, agentTestRequest(nil, false))
	for _, test := range []struct {
		name    string
		models  []sdk.AgentDecisionModel
		present bool
	}{
		{name: "legacy-absent"},
		{name: "api9-empty", models: []sdk.AgentDecisionModel{}, present: true},
		{name: "api9-provider-and-group", models: agentDecisionModelsFixture(), present: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := agentOptionsFixture(t)
			if test.present {
				options["decisionModels"] = test.models
			}
			parsed, err := readAgentOptions(raw(options))
			if err != nil {
				t.Fatal("valid host options refused:", err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(raw(parsed), &wire); err != nil {
				t.Fatal(err)
			}
			decisionWire, present := wire["decisionModels"]
			if present != (len(test.models) != 0) {
				t.Fatal("decision models were silently dropped or invented")
			}
			if present {
				var decisions []sdk.AgentDecisionModel
				if err := json.Unmarshal(decisionWire, &decisions); err != nil ||
					!reflect.DeepEqual(decisions, test.models) ||
					sdk.ValidateAgentDecisionModels(ControlManifest(), decisions) != nil {
					t.Fatal("typed public decision facts changed:", err)
				}
			}
			request := agentTestRequest(nil, false)
			request.Options = raw(options)
			plan := agentTestPlan(t, request)
			if len(plan.Files) != 1 || !bytes.Equal(plan.Files[0].Body, baseline.Files[0].Body) ||
				!reflect.DeepEqual(plan.Binding, baseline.Binding) ||
				bytes.Contains(plan.Files[0].Body, []byte("decision-fixture")) ||
				bytes.Contains(plan.Files[0].Body, []byte("decisionModels")) {
				t.Fatal("decision metadata leaked into ZCode chat config or binding")
			}
			if len(test.models) != 0 {
				options["field"], options["value"] = "model", "magpie/"+test.models[0].ID
				request.Action, request.Options = "set", raw(options)
				result, err := agentTestCall(t, sdk.MethodAgentPlan, request)
				var failure *sdk.Error
				if len(result) != 0 || !errors.As(err, &failure) || failure.Code != "unsupported" ||
					failure.Outcome != "not-started" {
					t.Fatal("decision-only model became selectable as a chat model:", err)
				}
			}
		})
	}
}

func TestAgentOptionsAcceptOrdinaryHostModelPresentation(t *testing.T) {
	options := agentOptionsFixture(t)
	model := options["models"].([]any)[0].(map[string]any)
	for key, value := range map[string]any{
		"label": "Chat model", "note": "Synthetic ordinary host option",
		"icon": "provider", "icons": []string{"provider", "group"},
		"group": "Provider", "groupIcon": "provider", "ref": "zcode/GLM-5.3",
		"free": true, "zero": true, "rate": 0.5, "rateWas": 1.0,
		"maxOutput": 64000, "efforts": []string{"none", "low", "high"}, "defaultEffort": "low",
		"takes": "low", "direct": "", "via": true, "same": true, "served": true,
		"alias": "zcode/chat", "fastFor": "zcode-native", "fast": true, "ownFast": true,
		"ultraOk": true, "ultra": true,
	} {
		model[key] = value
	}
	options["priorModels"] = []map[string]any{{"value": "personal/old-chat", "context": 8192}}
	options["decisionModels"] = agentDecisionModelsFixture()
	request := agentTestRequest(nil, false)
	request.Options = raw(options)
	plan := agentTestPlan(t, request)
	if len(plan.Files) != 1 || bytes.Contains(plan.Files[0].Body, []byte("decision-fixture")) ||
		bytes.Contains(plan.Files[0].Body, []byte("personal/old-chat")) {
		t.Fatal("host presentation or prior-model snapshot became an extra chat model")
	}
}

func TestAgentOptionsRejectUnknownAndInvalidDecisionFacts(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(map[string]any)
	}{
		{"unknown-option", func(options map[string]any) { options["futureOption"] = true }},
		{"unknown-model-option", func(options map[string]any) {
			options["models"].([]any)[0].(map[string]any)["futureModelOption"] = true
		}},
		{"unknown-previous-file", func(options map[string]any) {
			options["previous"] = []map[string]any{{"path": agentConfigPath, "beforeHash": "missing", "unexpected": true}}
		}},
		{"null-decisions", func(options map[string]any) { options["decisionModels"] = nil }},
		{"object-decisions", func(options map[string]any) { options["decisionModels"] = map[string]any{} }},
		{"unknown-decision-field", func(options map[string]any) {
			options["decisionModels"] = []map[string]any{{"id": "judge", "name": "Judge", "provider": "fixture",
				"endpoints": []string{"/v1/decisions"}, "futureDecisionOption": true}}
		}},
		{"missing-identity", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{Name: "Judge", Provider: "fixture", Endpoints: []string{"/v1/decisions"}}}
		}},
		{"missing-name", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{ID: "judge", Provider: "fixture", Endpoints: []string{"/v1/decisions"}}}
		}},
		{"missing-provider-or-group", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{ID: "judge", Name: "Judge", Endpoints: []string{"/v1/decisions"}}}
		}},
		{"provider-and-group", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{ID: "judge", Name: "Judge", Provider: "fixture",
				Group: true, Endpoints: []string{"/v1/decisions"}}}
		}},
		{"missing-endpoints", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{ID: "judge", Name: "Judge", Provider: "fixture"}}
		}},
		{"chat-endpoint", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{ID: "judge", Name: "Judge", Provider: "fixture",
				Endpoints: []string{"/v1/chat/completions"}}}
		}},
		{"duplicate-endpoint", func(options map[string]any) {
			options["decisionModels"] = []sdk.AgentDecisionModel{{ID: "judge", Name: "Judge", Provider: "fixture",
				Endpoints: []string{"/v1/decisions", "/v1/decisions"}}}
		}},
		{"invalid-model-metadata", func(options map[string]any) {
			options["models"].([]any)[0].(map[string]any)["efforts"] = []string{"invented"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := agentOptionsFixture(t)
			options["decisionModels"] = agentDecisionModelsFixture()
			test.alter(options)
			request := agentTestRequest(nil, false)
			request.Options = raw(options)
			result, err := agentTestCall(t, sdk.MethodAgentPlan, request)
			var failure *sdk.Error
			if len(result) != 0 || !errors.As(err, &failure) ||
				failure.Code != "invalid_params" || failure.Outcome != "not-started" {
				t.Fatal("malformed agent options produced a plan instead of invalid_params/not-started:", err)
			}
		})
	}
}

func TestAgentOptionsKeepStrictJSONBoundary(t *testing.T) {
	options := agentOptionsFixture(t)
	options["decisionModels"] = []sdk.AgentDecisionModel{}
	body := string(raw(options))
	for _, malformed := range []string{
		"null", "[]",
		body + `{}`,
		strings.TrimSuffix(body, "}") + `,"decisionModels":[]}`,
		strings.Replace(body, `"decisionModels":[]`, `"decisionModels":[null]`, 1),
		strings.Replace(body, `"decisionModels":[]`, `"decisionModels":[{"id":"judge","id":"other","name":"Judge","provider":"fixture","endpoints":["/v1/decisions"]}]`, 1),
	} {
		if _, err := readAgentOptions(json.RawMessage(malformed)); err == nil {
			t.Fatalf("ambiguous or malformed agent option JSON was accepted: %s", malformed)
		}
	}
}
