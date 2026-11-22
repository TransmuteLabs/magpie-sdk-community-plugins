package zcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const agentTestPath = ".zcode/v2/provider_config.json"

type agentTestFile struct {
	Path       string `json:"path"`
	Body       []byte `json:"body"`
	BeforeHash string `json:"beforeHash"`
	Mode       uint32 `json:"mode"`
	Exists     bool   `json:"exists"`
}

func agentTestSnapshot(body []byte) agentTestFile {
	file := agentTestFile{Path: agentTestPath, Body: body, BeforeHash: "missing"}
	if body != nil {
		hash := sha256.Sum256(body)
		file.BeforeHash, file.Exists, file.Mode = hex.EncodeToString(hash[:]), true, 0o600
	}
	return file
}

func agentTestRequest(body []byte, connected bool) sdk.AgentPlanRequest {
	revision := "0"
	values := map[string]string{}
	if connected {
		revision, values["model"] = "9007199254740993", "magpie/zcode/GLM-5.3"
	}
	return sdk.AgentPlanRequest{
		ID: "zcode-native", Action: "connect",
		Settings: raw(map[string]any{"files": []agentTestFile{agentTestSnapshot(body)}}),
		Options: raw(map[string]any{
			"gateway": "http://127.0.0.1:3425", "key": "synthetic-gateway-agent-key",
			"models": []map[string]any{
				{"value": "magpie/zcode/GLM-5.3", "label": "GLM-5.3", "context": 1000000, "note": "fixture",
					"maxOutput": 128000, "efforts": []string{"low", "high", "max"}},
				{"value": "magpie/zcode/GLM-5.2", "label": "GLM-5.2", "context": 1000000, "note": "fixture",
					"maxOutput": 128000, "efforts": []string{"none", "high", "max"}},
			},
		}),
		Binding: &sdk.AgentBindingSnapshot{Target: "zcode", Revision: revision, Connected: connected, Values: values},
	}
}

func agentTestCall(t *testing.T, method string, request any) (json.RawMessage, error) {
	t.Helper()
	handler := NewControlModule(ModuleOptions{}).Handlers[method]
	if handler == nil {
		t.Fatalf("owned ZCode %s handler is missing", method)
	}
	result, err := handler(t.Context(), sdk.Call{Scope: sdk.Scope{Plugin: ProviderID, Operation: "agent.fixture"}, Params: raw(request)}, nil)
	if err != nil {
		return nil, err
	}
	return raw(result), nil
}

func agentTestPlan(t *testing.T, request sdk.AgentPlanRequest) sdk.AgentPlan {
	t.Helper()
	body, err := agentTestCall(t, sdk.MethodAgentPlan, request)
	if err != nil {
		t.Fatal("owned agent plan:", err)
	}
	var plan sdk.AgentPlan
	if err := json.Unmarshal(body, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Binding == nil || plan.Binding.Revision != request.Binding.Revision {
		t.Fatal("plan did not echo the exact host binding revision")
	}
	return plan
}

func agentTestDocument(t *testing.T, plan sdk.AgentPlan) map[string]any {
	t.Helper()
	var document map[string]any
	if len(plan.Files) != 1 || plan.Files[0].Path != agentTestPath || plan.Files[0].Remove ||
		json.Unmarshal(plan.Files[0].Body, &document) != nil {
		t.Fatal("plan did not return the declared provider configuration")
	}
	return document["config"].(map[string]any)
}

func TestOwnedAgentIsInTheSingleNativeControlBundle(t *testing.T) {
	manifest := ControlManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatal("invalid native control manifest:", err)
	}
	if !manifest.Contract.Has(sdk.AgentWiring) || len(manifest.Agents) != 1 {
		t.Fatal("the native/UI bundle does not declare its own AgentWiring")
	}
	agent := manifest.Agents[0]
	if agent.ID != "zcode-native" || agent.Bin != "zcode" || agent.Profile == nil || agent.Profile.Target != "zcode" ||
		!reflect.DeepEqual(agent.ConfigFiles, []string{agentTestPath}) ||
		agent.Launch == nil || agent.Launch.Env["ZCODE_PERSONAL_PROVIDER_CONFIG_FILE"] != "{config}" ||
		agent.Library != nil || manifest.Runtime != "native" || len(manifest.Providers) != 1 {
		t.Fatal("owned profile lost its config path, launch or inherited ZCode Library owner")
	}
	headless := Manifest()
	if len(headless.Agents) != 0 || headless.Contract.Has(sdk.AgentWiring) || headless.Contract.Version.Minor != 20 {
		t.Fatal("provider-only manifest lost its SDK floor or acquired agent wiring")
	}
}

func TestOwnedAgentConnectUsesGatewayOnlyAndInspectRequiresOwnership(t *testing.T) {
	request := agentTestRequest(nil, false)
	plan := agentTestPlan(t, request)
	config := agentTestDocument(t, plan)
	if plan.Files[0].BeforeHash != "missing" || plan.Files[0].Mode != 0o600 ||
		!plan.Binding.Connected || plan.Binding.Values["model"] != "magpie/zcode/GLM-5.3" {
		t.Fatal("connect lost private-file preconditions or explicit model binding")
	}
	provider := config["providerConfigRules"].(map[string]any)["providerRules"].([]any)[0].(map[string]any)
	providerConfig := provider["config"].(map[string]any)
	if provider["providerId"] != "magpie" ||
		providerConfig["access"].(map[string]any)["apiKey"] != "synthetic-gateway-agent-key" ||
		providerConfig["api"].(map[string]any)["baseUrl"] != "http://127.0.0.1:3425" ||
		providerConfig["api"].(map[string]any)["type"] != "anthropic-messages" {
		t.Fatal("agent did not use the host gateway/key and vendor API schema")
	}
	selection := config["defaultModelSelection"].(map[string]any)
	if selection["providerId"] != "magpie" || selection["modelId"] != "zcode/GLM-5.3" {
		t.Fatal("selected model is not a gateway reference")
	}
	for _, connected := range []bool{false, true} {
		inspectRequest := agentTestRequest(plan.Files[0].Body, connected)
		body, err := agentTestCall(t, sdk.MethodAgentInspect, sdk.AgentInspectRequest{
			ID: request.ID, Settings: inspectRequest.Settings, Options: request.Options, Binding: inspectRequest.Binding,
		})
		var inspected sdk.AgentInspectResult
		if err != nil || json.Unmarshal(body, &inspected) != nil || inspected.Connected != connected ||
			strings.Contains(string(body), "synthetic-gateway-agent-key") {
			t.Fatal("inspection invented ownership or exported gateway credentials:", err)
		}
	}
}

func TestOwnedAgentSyncKeepsUserFormatManualRulesAndSelectedOptions(t *testing.T) {
	request := agentTestRequest(nil, false)
	initial := agentTestPlan(t, request).Files[0].Body
	var doc map[string]any
	json.Unmarshal(initial, &doc)
	config := doc["config"].(map[string]any)
	provider := config["providerConfigRules"].(map[string]any)["providerRules"].([]any)[0].(map[string]any)
	provider["providerName"] = "My gateway"
	provider["config"].(map[string]any)["api"].(map[string]any)["type"] = "openai-responses"
	modelRules := config["modelConfigRules"].(map[string]any)
	modelRules["providerModelRules"] = []any{
		map[string]any{"providerId": "magpie", "modelId": "zcode/GLM-5.2", "config": map[string]any{"optionSpecs": map[string]any{"maxOutputTokens": map[string]any{"max": 7777}}}},
	}
	manual := map[string]any{"providerId": "magpie", "modelId": "zcode/GLM-5.3", "config": map[string]any{
		"properties": map[string]any{
			"contextWindow": float64(1000000), "supportsJsonSchemaOutput": false, "supportsNativeWebSearch": false, "supportsMidConversationSystem": true,
			"inputFormat": map[string]any{"supportsImage": false, "supportsVideo": false, "supportsPdf": false},
		},
		"optionSpecs": map[string]any{
			"reasoningLevel":  map[string]any{"values": []any{"low", "high"}, "map": `{"reasoning_effort": reasoningLevel}`},
			"maxOutputTokens": map[string]any{"max": float64(7777)},
		},
	}}
	modelRules["manualProviderModelRules"] = []any{manual}
	config["defaultModelSelection"].(map[string]any)["options"] = map[string]any{"reasoningLevel": "low"}
	request = agentTestRequest(raw(doc), true)
	request.Action = "sync"
	plan := agentTestPlan(t, request)
	config = agentTestDocument(t, plan)
	provider = config["providerConfigRules"].(map[string]any)["providerRules"].([]any)[0].(map[string]any)
	if provider["providerName"] != "My gateway" || provider["config"].(map[string]any)["api"].(map[string]any)["type"] != "openai-responses" ||
		config["defaultModelSelection"].(map[string]any)["options"].(map[string]any)["reasoningLevel"] != "low" {
		t.Fatal("sync overwrote a user format, name or selected reasoning option")
	}
	modelRules = config["modelConfigRules"].(map[string]any)
	if !reflect.DeepEqual(modelRules["manualProviderModelRules"], []any{manual}) {
		t.Fatal("sync overwrote manual model settings")
	}
	for _, entry := range modelRules["providerModelRules"].([]any) {
		rule := entry.(map[string]any)
		if rule["modelId"] == "zcode/GLM-5.3" {
			t.Fatal("sync produced conflicting smart and manual rules")
		}
		if rule["modelId"] == "zcode/GLM-5.2" &&
			rule["config"].(map[string]any)["optionSpecs"].(map[string]any)["maxOutputTokens"].(map[string]any)["max"] != float64(7777) {
			t.Fatal("sync overwrote the user's output limit")
		}
	}
}

func TestOwnedAgentDisconnectRestoresOriginalAndPreservesForeignEdits(t *testing.T) {
	baseline := raw(map[string]any{"schemaVersion": 1, "config": map[string]any{
		"providerConfigRules": map[string]any{"providerRules": []any{
			map[string]any{"providerId": "personal", "config": map[string]any{}},
		}},
		"modelConfigRules":      map[string]any{"providerModelRules": []any{}, "manualProviderModelRules": []any{}},
		"defaultModelSelection": map[string]any{"providerId": "personal", "modelId": "own-model"},
	}})
	connected := agentTestPlan(t, agentTestRequest(baseline, false))
	var doc map[string]any
	json.Unmarshal(connected.Files[0].Body, &doc)
	config := doc["config"].(map[string]any)
	providers := config["providerConfigRules"].(map[string]any)
	providers["providerRules"].([]any)[0].(map[string]any)["providerName"] = "User edit after connect"
	request := agentTestRequest(raw(doc), true)
	request.Action = "disconnect"
	var options map[string]any
	json.Unmarshal(request.Options, &options)
	options["previous"] = []agentTestFile{agentTestSnapshot(baseline)}
	request.Options = raw(options)
	plan := agentTestPlan(t, request)
	config = agentTestDocument(t, plan)
	rules := config["providerConfigRules"].(map[string]any)["providerRules"].([]any)
	if plan.Binding.Connected || len(plan.Binding.Values) != 0 || len(rules) != 1 ||
		rules[0].(map[string]any)["providerName"] != "User edit after connect" ||
		config["defaultModelSelection"].(map[string]any)["modelId"] != "own-model" {
		t.Fatal("disconnect lost baseline selection or foreign user edits")
	}
}

func TestOwnedAgentDisconnectRestoresExactGatewayBaselineOrRemovesNewFile(t *testing.T) {
	for _, baseline := range [][]byte{
		nil,
		[]byte("{\n  \"schemaVersion\":1,\"config\":{\"providerConfigRules\":{\"providerRules\":[{\"providerId\":\"magpie\",\"providerName\":\"Original gateway\",\"enabled\":true,\"config\":{\"group\":\"standard-personal\",\"api\":{\"type\":\"openai-responses\",\"baseUrl\":\"https://original.example.invalid\"},\"access\":{\"type\":\"api-key\",\"apiKey\":\"original-gateway-key\"}}}]},\"modelConfigRules\":{\"providerModelRules\":[{\"providerId\":\"magpie\",\"modelId\":\"old-model\",\"config\":{}}],\"manualProviderModelRules\":[]},\"defaultModelSelection\":{\"providerId\":\"magpie\",\"modelId\":\"old-model\"}}}\n"),
	} {
		name := "existing"
		if baseline == nil {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			connected := agentTestPlan(t, agentTestRequest(baseline, false))
			request := agentTestRequest(connected.Files[0].Body, true)
			request.Action = "sync"
			synced := agentTestPlan(t, request)
			if len(synced.Files) != 0 || !synced.Binding.Connected {
				t.Fatal("unchanged owned configuration was not idempotent")
			}
			request.Action = "disconnect"
			var options map[string]any
			json.Unmarshal(request.Options, &options)
			options["previous"] = []agentTestFile{agentTestSnapshot(baseline)}
			request.Options = raw(options)
			disconnected := agentTestPlan(t, request)
			if disconnected.Binding.Connected || len(disconnected.Files) != 1 ||
				disconnected.Files[0].Remove != (baseline == nil) ||
				string(disconnected.Files[0].Body) != string(baseline) ||
				disconnected.Files[0].BeforeHash != agentTestSnapshot(connected.Files[0].Body).BeforeHash {
				t.Fatal("disconnect failed to restore the exact original provider file")
			}
		})
	}
}

func TestOwnedAgentSetRejectsUnknownModelsAndNeverCarriesPriorModelOptions(t *testing.T) {
	connected := agentTestPlan(t, agentTestRequest(nil, false))
	var doc map[string]any
	json.Unmarshal(connected.Files[0].Body, &doc)
	doc["config"].(map[string]any)["defaultModelSelection"].(map[string]any)["options"] = map[string]any{"reasoningLevel": "low"}
	request := agentTestRequest(raw(doc), true)
	request.Action = "set"
	var options map[string]any
	json.Unmarshal(request.Options, &options)
	options["field"], options["value"] = "model", "magpie/zcode/GLM-5.2"
	request.Options = raw(options)
	config := agentTestDocument(t, agentTestPlan(t, request))
	selection := config["defaultModelSelection"].(map[string]any)
	if selection["modelId"] != "zcode/GLM-5.2" || selection["options"].(map[string]any)["reasoningLevel"] == "low" {
		t.Fatal("set did not select the requested declared gateway model or carried incompatible old options")
	}
	options["value"] = "magpie/undeclared/paid-fallback"
	request.Options = raw(options)
	if _, err := agentTestCall(t, sdk.MethodAgentPlan, request); err == nil {
		t.Fatal("set invented an unoffered model")
	}
}

func TestOwnedAgentRefusesUnknownFilesMalformedSnapshotsAndForeignBinding(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*sdk.AgentPlanRequest)
	}{
		{"wrong-id", func(request *sdk.AgentPlanRequest) { request.ID = "other" }},
		{"unknown-action", func(request *sdk.AgentPlanRequest) { request.Action = "install" }},
		{"missing-binding", func(request *sdk.AgentPlanRequest) { request.Binding = nil }},
		{"foreign-binding", func(request *sdk.AgentPlanRequest) { request.Binding.Target = "other" }},
		{"noncanonical-revision", func(request *sdk.AgentPlanRequest) { request.Binding.Revision = "00" }},
		{"extra-file", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{{Path: "foreign.json", BeforeHash: "missing"}}})
		}},
		{"wrong-hash", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{{Path: agentTestPath, Body: []byte("{}"), Exists: true, BeforeHash: "missing"}}})
		}},
		{"empty-existing", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{agentTestSnapshot([]byte{})}})
		}},
		{"future-schema", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{agentTestSnapshot([]byte(`{"schemaVersion":2,"config":{}}`))}})
		}},
		{"duplicate-json", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{agentTestSnapshot([]byte(`{"schemaVersion":1,"schemaVersion":1,"config":{}}`))}})
		}},
		{"null-rules", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{agentTestSnapshot([]byte(`{"schemaVersion":1,"config":{"providerConfigRules":null}}`))}})
		}},
		{"duplicate-provider", func(request *sdk.AgentPlanRequest) {
			request.Settings = raw(map[string]any{"files": []agentTestFile{agentTestSnapshot([]byte(`{"schemaVersion":1,"config":{"providerConfigRules":{"providerRules":[{"providerId":"magpie","config":{}},{"providerId":"magpie","config":{}}]},"modelConfigRules":{"providerModelRules":[],"manualProviderModelRules":[]}}}`))}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := agentTestRequest(nil, false)
			test.alter(&request)
			if _, err := agentTestCall(t, sdk.MethodAgentPlan, request); err == nil {
				t.Fatal("unsafe agent plan was accepted")
			}
		})
	}
	handler := NewControlModule(ModuleOptions{}).Handlers[sdk.MethodAgentPlan]
	if handler == nil {
		t.Fatal("owned agent plan handler is missing")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := handler(ctx, sdk.Call{Params: raw(agentTestRequest(nil, false))}, nil); err == nil {
		t.Fatal("cancelled agent plan still prepared writes")
	}
}
