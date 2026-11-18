package zcode

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func metadataRequest(body []byte, models, prior []agentModelOption) sdk.AgentPlanRequest {
	request := agentTestRequest(body, body != nil)
	request.Action = "sync"
	if body == nil {
		request.Action = "connect"
	}
	options := map[string]any{"gateway": "http://127.0.0.1:3425", "key": "synthetic-gateway-agent-key", "models": models}
	if prior != nil {
		options["priorModels"] = prior
	}
	request.Options = raw(options)
	request.Binding.Values["model"] = models[0].Value
	return request
}

func metadataRules(t *testing.T, body []byte) (map[string]any, map[string]map[string]any) {
	t.Helper()
	var document map[string]any
	if json.Unmarshal(body, &document) != nil {
		t.Fatal("agent emitted invalid JSON")
	}
	config := document["config"].(map[string]any)
	rules := map[string]map[string]any{}
	for _, entry := range config["modelConfigRules"].(map[string]any)["providerModelRules"].([]any) {
		rule := entry.(map[string]any)
		rules[rule["modelId"].(string)] = rule["config"].(map[string]any)
	}
	return config, rules
}

func TestAgentMetadataUsesEffectiveGroupLimitsAndDefault(t *testing.T) {
	models := []agentModelOption{{
		Value: "magpie/routing/GLM-5.3", Context: 8192,
		AgentModelMetadata: sdk.AgentModelMetadata{MaxOutput: 2048, Efforts: []string{"none", "low"}, DefaultEffort: "low"},
	}}
	plan := agentTestPlan(t, metadataRequest(nil, models, nil))
	config, rules := metadataRules(t, plan.Files[0].Body)
	expected := map[string]any{
		"properties": map[string]any{"contextWindow": float64(8192)},
		"optionSpecs": map[string]any{
			"maxOutputTokens": map[string]any{"max": float64(2048)},
			"reasoningLevel":  map[string]any{"values": []any{"disabled", "low"}},
		},
	}
	if !reflect.DeepEqual(rules["routing/GLM-5.3"], expected) {
		t.Fatalf("effective group metadata replaced by suffix defaults: %s", raw(rules))
	}
	selection := config["defaultModelSelection"].(map[string]any)
	if selection["options"].(map[string]any)["reasoningLevel"] != "low" {
		t.Fatalf("host effective default was ignored: %s", raw(selection))
	}
}

func TestAgentMetadataDistinguishesUnknownAndReasonless(t *testing.T) {
	for _, test := range []struct {
		name    string
		efforts []string
	}{
		{"unknown", nil},
		{"reasonless", []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			models := []agentModelOption{{Value: "magpie/zcode/GLM-5.3",
				AgentModelMetadata: sdk.AgentModelMetadata{Efforts: test.efforts}}}
			plan := agentTestPlan(t, metadataRequest(nil, models, nil))
			config, rules := metadataRules(t, plan.Files[0].Body)
			specs, _ := rules["zcode/GLM-5.3"]["optionSpecs"].(map[string]any)
			if _, present := specs["maxOutputTokens"]; present {
				t.Fatal("unknown effective output inherited a static vendor limit")
			}
			selection := config["defaultModelSelection"].(map[string]any)
			if test.efforts == nil {
				if _, present := specs["reasoningLevel"]; present || selection["options"] != nil {
					t.Fatalf("unknown effort became chat configuration: %s", raw(config))
				}
			} else if !reflect.DeepEqual(specs["reasoningLevel"], map[string]any{"values": []any{"disabled"}}) ||
				selection["options"].(map[string]any)["reasoningLevel"] != "disabled" {
				t.Fatalf("confirmed reasonless model retained reasoning: %s", raw(config))
			}
		})
	}
}

func TestAgentMetadataSyncReplacesGeneratedFieldsAcrossCatalogs(t *testing.T) {
	old := []agentModelOption{{Value: "magpie/zcode/GLM-5.3", Context: 8192,
		AgentModelMetadata: sdk.AgentModelMetadata{MaxOutput: 2048, Efforts: []string{"low", "high"}, DefaultEffort: "high"}}}
	first := agentTestPlan(t, metadataRequest(nil, old, nil))
	next := []agentModelOption{{Value: old[0].Value, Context: 4096,
		AgentModelMetadata: sdk.AgentModelMetadata{MaxOutput: 1024, Efforts: []string{"none", "low"}, DefaultEffort: "low"}}}
	second := agentTestPlan(t, metadataRequest(first.Files[0].Body, next, old))
	if len(second.Files) != 1 {
		t.Fatal("changed effective catalog did not update generated metadata")
	}
	config, rules := metadataRules(t, second.Files[0].Body)
	rule := rules["zcode/GLM-5.3"]
	if rule["properties"].(map[string]any)["contextWindow"] != float64(4096) ||
		rule["optionSpecs"].(map[string]any)["maxOutputTokens"].(map[string]any)["max"] != float64(1024) ||
		config["defaultModelSelection"].(map[string]any)["options"].(map[string]any)["reasoningLevel"] != "low" {
		t.Fatalf("generated metadata was frozen at the previous catalog: %s", raw(config))
	}
	unchanged := agentTestPlan(t, metadataRequest(second.Files[0].Body, next, next))
	if len(unchanged.Files) != 0 {
		t.Fatal("an unchanged effective catalog produced a file change")
	}
	unknown := []agentModelOption{{Value: old[0].Value}}
	third := agentTestPlan(t, metadataRequest(second.Files[0].Body, unknown, next))
	if len(third.Files) != 1 {
		t.Fatal("withdrawn effective metadata did not remove generated fields")
	}
	config, rules = metadataRules(t, third.Files[0].Body)
	if len(rules["zcode/GLM-5.3"]) != 0 || config["defaultModelSelection"].(map[string]any)["options"] != nil {
		t.Fatalf("withdrawn metadata retained generated capabilities: %s", raw(config))
	}
}

func TestAgentMetadataSyncPreservesUserEditsAcrossRepeatedCatalogs(t *testing.T) {
	old := []agentModelOption{{Value: "magpie/zcode/GLM-5.3", Context: 8192,
		AgentModelMetadata: sdk.AgentModelMetadata{MaxOutput: 2048, Efforts: []string{"low", "high"}, DefaultEffort: "high"}}}
	first := agentTestPlan(t, metadataRequest(nil, old, nil))
	var document map[string]any
	json.Unmarshal(first.Files[0].Body, &document)
	config, rules := metadataRules(t, first.Files[0].Body)
	rules["zcode/GLM-5.3"]["properties"].(map[string]any)["contextWindow"] = float64(7777)
	rules["zcode/GLM-5.3"]["optionSpecs"].(map[string]any)["maxOutputTokens"] = map[string]any{"max": float64(999), "map": "user-map"}
	config["defaultModelSelection"].(map[string]any)["options"] = map[string]any{"reasoningLevel": "low"}
	document["config"] = config
	next := []agentModelOption{{Value: old[0].Value, Context: 4096,
		AgentModelMetadata: sdk.AgentModelMetadata{MaxOutput: 1024, Efforts: []string{"low", "high"}, DefaultEffort: "high"}}}
	for range 2 {
		plan := agentTestPlan(t, metadataRequest(raw(document), next, old))
		body := raw(document)
		if len(plan.Files) != 0 {
			body = plan.Files[0].Body
		}
		config, rules = metadataRules(t, body)
		rule := rules["zcode/GLM-5.3"]
		if rule["properties"].(map[string]any)["contextWindow"] != float64(7777) ||
			!reflect.DeepEqual(rule["optionSpecs"].(map[string]any)["maxOutputTokens"], map[string]any{"max": float64(999), "map": "user-map"}) ||
			config["defaultModelSelection"].(map[string]any)["options"].(map[string]any)["reasoningLevel"] != "low" {
			t.Fatalf("effective sync erased explicit user metadata: %s", raw(config))
		}
		json.Unmarshal(body, &document)
		old = next
		next = []agentModelOption{{Value: old[0].Value, Context: 2048,
			AgentModelMetadata: sdk.AgentModelMetadata{MaxOutput: 512, Efforts: []string{"low", "high"}, DefaultEffort: "high"}}}
	}
}

func TestAgentMetadataRejectsMalformedPreviousCatalog(t *testing.T) {
	for _, prior := range []json.RawMessage{
		json.RawMessage(`null`), json.RawMessage(`{}`),
		json.RawMessage(`[{"value":"magpie/old","maxOutput":-1}]`),
		json.RawMessage(`[{"value":"magpie/old","efforts":["invented"]}]`),
		json.RawMessage(`[{"value":"magpie/old","futureAuthority":true}]`),
		json.RawMessage(`[{"value":"magpie/old"},{"value":"magpie/old"}]`),
	} {
		options := agentOptionsFixture(t)
		options["priorModels"] = prior
		request := agentTestRequest(nil, false)
		request.Options = raw(options)
		_, err := agentTestCall(t, sdk.MethodAgentPlan, request)
		var failure *sdk.Error
		if !errors.As(err, &failure) || failure.Code != "invalid_params" || failure.Outcome != "not-started" {
			t.Fatalf("invalid prior catalog granted generated-field authority: %s: %v", prior, err)
		}
	}
}

func TestAgentMetadataSyncRejectsUnavailableUserEffort(t *testing.T) {
	old := []agentModelOption{{Value: "magpie/zcode/GLM-5.3",
		AgentModelMetadata: sdk.AgentModelMetadata{Efforts: []string{"low", "high"}, DefaultEffort: "high"}}}
	first := agentTestPlan(t, metadataRequest(nil, old, nil))
	var document map[string]any
	json.Unmarshal(first.Files[0].Body, &document)
	document["config"].(map[string]any)["defaultModelSelection"].(map[string]any)["options"] = map[string]any{"reasoningLevel": "low"}
	next := []agentModelOption{{Value: old[0].Value,
		AgentModelMetadata: sdk.AgentModelMetadata{Efforts: []string{"none", "high"}, DefaultEffort: "high"}}}
	_, err := agentTestCall(t, sdk.MethodAgentPlan, metadataRequest(raw(document), next, old))
	var failure *sdk.Error
	if !errors.As(err, &failure) || failure.Code != "unsupported" || failure.Outcome != "not-started" {
		t.Fatal("an unavailable user effort was silently replaced instead of requiring selection:", err)
	}
}
