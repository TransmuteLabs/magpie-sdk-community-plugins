package zcode

import (
	"encoding/json"
	"testing"
)

func modelConfigFixture() map[string]any {
	return map[string]any{"config": map[string]any{
		"providerConfigRules": map[string]any{"providerRules": []any{
			map[string]any{"providerId": "account:zai-individual-coding-plan", "templateId": "template",
				"config": map[string]any{"builtinModelIds": []string{"GLM-5.3", "GLM-5.2", "GLM-5.3"}}},
		}},
		"modelConfigRules": map[string]any{
			"modelRules": []any{map[string]any{"modelMatch": ".*", "config": map[string]any{
				"enabled": true, "properties": map[string]any{"contextWindow": 256000},
				"optionSpecs": map[string]any{"maxOutputTokens": map[string]any{"max": 8000},
					"reasoningLevel": map[string]any{"values": []string{"disabled", "enabled"}}},
			}}},
			"modelApiRules": []any{map[string]any{"modelMatch": "GLM-5\\.3", "apiTypeMatch": "anthropic-messages",
				"config": map[string]any{"properties": map[string]any{"contextWindow": 1000000}}}},
			"providerSiteRules": []any{map[string]any{"modelMatch": "GLM-5\\.3", "baseUrlMatch": "https://api\\.z\\.ai/api/anthropic",
				"config": map[string]any{"properties": map[string]any{"inputFormat": map[string]any{"supportsImage": true}}}}},
			"templateModelRules": []any{map[string]any{"templateId": "template", "modelId": "GLM-5.3",
				"config": map[string]any{"optionSpecs": map[string]any{"maxOutputTokens": map[string]any{"max": 128000}}}}},
			"builtinProviderModelRules": []any{map[string]any{"providerId": "account:zai-individual-coding-plan", "modelId": "GLM-5.2",
				"config": map[string]any{"enabled": false}}},
		},
	}}
}

func TestDiscoveredModelRulesApplyInVendorOrderAndDeduplicate(t *testing.T) {
	body, _ := json.Marshal(modelConfigFixture())
	models, err := configuredModels(body, "account:zai-individual-coding-plan", "https://api.z.ai/api/anthropic")
	if err != nil || len(models) != 1 {
		t.Fatalf("model configuration: %v, %d", err, len(models))
	}
	model := models[0]
	if model.ID != "GLM-5.3" || model.ContextWindow != 1000000 || model.MaxOutput != 128000 ||
		!model.Vision || len(model.Efforts) != 2 || model.Efforts[0] != "none" || model.Efforts[1] != "high" {
		t.Fatalf("ordered rules or model modalities/options were lost: %+v", model)
	}
}

func TestExplicitNullClearsInheritedModelProperties(t *testing.T) {
	fixture := modelConfigFixture()
	config := fixture["config"].(map[string]any)
	rules := config["modelConfigRules"].(map[string]any)
	rules["builtinProviderModelRules"] = []any{map[string]any{
		"providerId": "account:zai-individual-coding-plan", "modelId": "GLM-5.3",
		"config": map[string]any{"properties": nil, "optionSpecs": nil},
	}, map[string]any{"providerId": "account:zai-individual-coding-plan", "modelId": "GLM-5.2",
		"config": map[string]any{"enabled": false}}}
	body, _ := json.Marshal(fixture)
	models, err := configuredModels(body, "account:zai-individual-coding-plan", "https://api.z.ai/api/anthropic")
	if err != nil || len(models) != 1 || models[0].ContextWindow != 0 || models[0].MaxOutput != 0 ||
		models[0].Vision || models[0].Reasoning || len(models[0].Efforts) != 0 {
		t.Fatal("explicit vendor null resurrected stale inherited capabilities")
	}
}

func TestUnsupportedRegexpCannotSilentlyDropAnEligibilityRule(t *testing.T) {
	fixture := modelConfigFixture()
	rules := fixture["config"].(map[string]any)["modelConfigRules"].(map[string]any)
	rules["modelRules"] = []any{map[string]any{"modelMatch": "(?!GLM-5\\.2).*", "config": map[string]any{"enabled": true}}}
	body, _ := json.Marshal(fixture)
	if _, err := configuredModels(body, "account:zai-individual-coding-plan", "https://api.z.ai/api/anthropic"); err == nil {
		t.Fatal("unsupported regexp was treated as an unmatched rule and silently changed model eligibility")
	}
}

func TestMissingRuleFamiliesAndUnspecifiedOptionsPreserveInheritance(t *testing.T) {
	fixture := modelConfigFixture()
	rules := fixture["config"].(map[string]any)["modelConfigRules"].(map[string]any)
	delete(rules, "modelApiRules")
	delete(rules, "providerSiteRules")
	delete(rules, "templateModelRules")
	rules["builtinProviderModelRules"] = []any{map[string]any{
		"providerId": "account:zai-individual-coding-plan", "modelId": "GLM-5.3",
		"config": map[string]any{"optionSpecs": map[string]any{"reasoningLevel": map[string]any{}}},
	}}
	models, err := configuredModels(raw(fixture), "account:zai-individual-coding-plan", "https://fixture.invalid")
	if err != nil || len(models) != 2 || len(models[0].Efforts) != 2 || !models[0].Reasoning {
		t.Fatalf("missing optional rules or options cleared inheritance: %v %+v", err, models)
	}
}
