package opencode

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

func agentSettings(body []byte) json.RawMessage {
	file := agentFile{Path: agentConfigPath, BeforeHash: "missing"}
	if body != nil {
		sum := sha256.Sum256(body)
		file.Body, file.Exists, file.BeforeHash = body, true, hex.EncodeToString(sum[:])
	}
	return raw(map[string]any{"files": []agentFile{file}})
}

func agentCall(t *testing.T, request sdk.AgentPlanRequest) sdk.AgentPlan {
	t.Helper()
	value, err := planAgent(t.Context(), sdk.Call{Params: raw(request)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := value.(sdk.AgentPlan)
	if sdk.ValidateAgentBindingChange(AgentManifest(), AgentManifest().Agents[0], request.Binding, request.Action, plan.Binding) != nil {
		t.Fatal("invalid host binding result")
	}
	return plan
}

func TestAgentManifestKeepsProviderSliceAndAdoptsExplicitLibraryOwner(t *testing.T) {
	base, agent := NewModule(ModuleOptions{}), NewAgentModule(ModuleOptions{})
	if err := agent.Validate(); err != nil {
		t.Fatal(err)
	}
	if base.Manifest.Version != "0.2.0" || base.Manifest.Contract.Version.Minor != 12 || len(base.Manifest.Agents) != 0 ||
		agent.Manifest.Contract.Version.Minor != 12 || agent.Manifest.Version != "0.3.0" ||
		!reflect.DeepEqual(base.Manifest.Providers, agent.Manifest.Providers) {
		t.Fatal("agent distribution changed the reviewed provider slice")
	}
	descriptor := agent.Manifest.Agents[0]
	if descriptor.ID == "opencode" || descriptor.Profile.Target != "opencode" || descriptor.Library != nil ||
		descriptor.Launch.Env["OPENCODE_CONFIG"] != "{config}" || descriptor.Launch.Model != "--model" {
		t.Fatal("agent did not reuse explicit existing-agent and Library ownership")
	}
	old := agent.Manifest.Clone()
	old.Contract.Version.Minor = 4
	if old.Validate() == nil {
		t.Fatal("agent metadata accepted an older SDK")
	}
}

func TestAgentPlansUseCheckedOverlayAndRetainIndependentUserEdits(t *testing.T) {
	binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}}
	options := agentOptions{Gateway: "http://127.0.0.1:3425", Key: "host-only-key", Field: "model", Value: "magpie/group/executor",
		Models: []json.RawMessage{raw(map[string]any{"value": "magpie/group/executor", "context": 262144, "label": "Executor", "icon": "group"})}}
	request := sdk.AgentPlanRequest{ID: AgentID, Action: "set", Binding: binding, Options: raw(options), Settings: agentSettings(nil)}
	plan := agentCall(t, request)
	if len(plan.Files) != 1 || plan.Files[0].BeforeHash != "missing" || plan.Files[0].Path != agentConfigPath ||
		plan.Files[0].Mode != 0o600 || !plan.Binding.Connected || plan.Binding.Values["model"] != options.Value || len(binding.Values) != 0 {
		t.Fatal("plan changed host authority, selected a model implicitly or wrote outside overlay")
	}
	body := plan.Files[0].Body
	var config map[string]json.RawMessage
	if json.Unmarshal(body, &config) != nil || strings.Contains(string(body), `"provider":`) ||
		!strings.Contains(string(body), `"package": "aisdk:@ai-sdk/openai-compatible"`) ||
		!strings.Contains(string(body), `"supportsPromptCacheKey": true`) {
		t.Fatal("invalid v2 donor shape")
	}
	current := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "1", Connected: true, Values: plan.Binding.Values}
	inspect := sdk.AgentInspectRequest{ID: AgentID, Binding: current, Options: raw(options), Settings: agentSettings(body)}
	value, err := inspectAgent(t.Context(), sdk.Call{Params: raw(inspect)}, nil)
	if err != nil || !value.(sdk.AgentInspectResult).Connected {
		t.Fatal("ordinary inspection failed:", err)
	}
	options.Field, options.Value = "small", "magpie/group/executor"
	next := agentCall(t, sdk.AgentPlanRequest{ID: AgentID, Action: "set", Binding: current, Options: raw(options), Settings: agentSettings(body)})
	if !strings.Contains(string(next.Files[0].Body), `"title"`) {
		t.Fatal("small model was not written to v2 title agent")
	}
	json.Unmarshal(next.Files[0].Body, &config)
	config["theme"], config["mcp"] = raw("user-theme"), raw(map[string]any{"mine": map[string]any{"type": "local", "command": []string{"fixture"}}})
	changed, _ := json.Marshal(config)
	current.Revision, current.Values = "2", next.Binding.Values
	options.Previous = []agentFile{{Path: agentConfigPath, BeforeHash: "missing"}}
	off := agentCall(t, sdk.AgentPlanRequest{ID: AgentID, Action: "disconnect", Binding: current,
		Options: raw(options), Settings: agentSettings(changed)})
	if off.Binding.Connected || off.Files[0].Remove || !strings.Contains(string(off.Files[0].Body), "user-theme") ||
		strings.Contains(string(off.Files[0].Body), "host-only-key") || !strings.Contains(string(off.Files[0].Body), `"mine"`) {
		t.Fatal("disconnect lost independent edits or left gateway credentials")
	}
	if string(body) != string(plan.Files[0].Body) {
		t.Fatal("input buffers changed")
	}
}

func TestAgentRefusesUnknownAuthorityMalformedSnapshotsAndImplicitModel(t *testing.T) {
	binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}}
	options := agentOptions{Gateway: "http://127.0.0.1:3425", Key: "host-key", Field: "model", Value: "magpie/known/model",
		Models: []json.RawMessage{raw(map[string]string{"value": "magpie/known/model"})}}
	valid := sdk.AgentPlanRequest{ID: AgentID, Action: "set", Binding: binding, Options: raw(options), Settings: agentSettings(nil)}
	if _, err := planAgent(t.Context(), sdk.Call{Params: raw(valid)}, nil); err != nil {
		t.Fatal("valid authority control failed:", err)
	}
	for _, mutate := range []func(*sdk.AgentPlanRequest){
		func(r *sdk.AgentPlanRequest) { r.ID = "opencode" },
		func(r *sdk.AgentPlanRequest) { r.Binding = nil },
		func(r *sdk.AgentPlanRequest) {
			r.Binding = &sdk.AgentBindingSnapshot{Target: "foreign", Revision: "0", Values: map[string]string{}}
		},
		func(r *sdk.AgentPlanRequest) {
			r.Settings = raw(map[string]any{"files": []agentFile{{Path: "../outside"}}})
		},
		func(r *sdk.AgentPlanRequest) { r.Settings = agentSettings([]byte(`{"providers":null}`)) },
		func(r *sdk.AgentPlanRequest) { r.Action = "connect" },
		func(r *sdk.AgentPlanRequest) {
			r.Options = raw(map[string]any{"gateway": "file:///tmp", "key": "host-key", "field": "model", "value": "magpie/known/model"})
		},
		func(r *sdk.AgentPlanRequest) {
			r.Options = raw(map[string]any{"gateway": options.Gateway, "key": "host-key", "field": "model", "value": "magpie/foreign/model", "models": options.Models})
		},
	} {
		request := valid
		mutate(&request)
		if _, err := planAgent(context.Background(), sdk.Call{Params: raw(request)}, nil); err == nil {
			t.Fatal("unsafe agent plan accepted")
		}
	}
}

func TestAgentV1AndV2KeepDonorReasoningAndCatalogOrder(t *testing.T) {
	hash := sha256.Sum256([]byte("opencode\x00" + ProviderGo))
	provider := "extension-" + hex.EncodeToString(hash[:12])
	first, second := "magpie/"+provider+"/glm-5.2", "magpie/"+provider+"/deepseek-v4.1-flash"
	for _, format := range []string{"v1", "v2"} {
		t.Run(format, func(t *testing.T) {
			host := serveHarness(t, NewAgentModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global", AgentFormat: format}))
			options := agentOptions{Gateway: "http://127.0.0.1:3425", Key: "fixture-only", Field: "model", Value: first,
				Models: []json.RawMessage{raw(map[string]any{"value": first}), raw(map[string]any{"value": second})}}
			frame, _ := host.invoke(sdk.MethodAgentPlan, sdk.Scope{}, sdk.AgentPlanRequest{ID: AgentID, Action: "set",
				Binding: &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}},
				Options: raw(options), Settings: agentSettings(nil)}, nil)
			if frame.Error != nil {
				t.Fatalf("SDK agent plan rejected: %+v", frame.Error)
			}
			var plan sdk.AgentPlan
			if json.Unmarshal(frame.Result, &plan) != nil || len(plan.Files) != 1 {
				t.Fatal("SDK lost checked plan")
			}
			body := string(plan.Files[0].Body)
			var root, providers, block map[string]json.RawMessage
			json.Unmarshal(plan.Files[0].Body, &root)
			key := "providers"
			if format == "v1" {
				key = "provider"
			}
			json.Unmarshal(root[key], &providers)
			json.Unmarshal(providers["magpie"], &block)
			models := string(block["models"])
			if strings.Index(models, provider+"/glm-5.2") > strings.Index(models, provider+"/deepseek-v4.1-flash") {
				t.Fatal("catalog order changed")
			}
			if format == "v2" && (!strings.Contains(body, `"capabilities"`) || !strings.Contains(body, `"reasoningEffort"`) ||
				strings.Contains(body, `"reasoning":`)) {
				t.Fatal("v2 catalog lost native capabilities/variants or retained unsupported legacy reasoning")
			}
			if format == "v1" && (!strings.Contains(body, `"modalities"`) || !strings.Contains(body, `"reasoning": true`) ||
				strings.Contains(body, `"package":`)) {
				t.Fatal("v1 catalog lost donor modalities/reasoning")
			}
		})
	}
}

func TestAgentSyncAndDisconnectPreserveOutputOverridesAndUnknownFields(t *testing.T) {
	hash := sha256.Sum256([]byte("opencode\x00" + ProviderGo))
	ref := "extension-" + hex.EncodeToString(hash[:12]) + "/glm-5.2"
	before := []byte(`{"providers":{"magpie":{"settings":{"custom":"keep"},"compatibility":{"supportsPromptCacheKey":false},"models":{"` + ref +
		`":{"limit":{"output":7000},"custom":"keep-model"}}}},"agents":{"title":{"model":"own/title","prompt":"keep-title"}},"big":9007199254740993}`)
	options := agentOptions{Gateway: "http://127.0.0.1:3425", Key: "host-only-key", Field: "model", Value: "magpie/" + ref,
		Models: []json.RawMessage{raw(map[string]string{"value": "magpie/" + ref})}}
	binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}}
	plan := agentCall(t, sdk.AgentPlanRequest{ID: AgentID, Action: "set", Binding: binding, Options: raw(options), Settings: agentSettings(before)})
	after := plan.Files[0].Body
	for _, value := range []string{"7000", `"keep-model"`, `"keep-title"`, `"own/title"`, "9007199254740993", `"supportsPromptCacheKey": false`} {
		if !strings.Contains(string(after), value) {
			t.Fatal("connect dropped user config:", value)
		}
	}
	if _, invented := plan.Binding.Values["small"]; invented {
		t.Fatal("connect claimed an untouched user small model")
	}
	current := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "1", Connected: true, Values: plan.Binding.Values}
	var root map[string]json.RawMessage
	json.Unmarshal(after, &root)
	root["later"] = raw("independent-new-value")
	userEdited, _ := json.MarshalIndent(root, "", "  ")
	sync := agentCall(t, sdk.AgentPlanRequest{ID: AgentID, Action: "sync", Binding: current, Options: raw(options), Settings: agentSettings(userEdited)})
	if len(sync.Files) > 0 {
		userEdited = sync.Files[0].Body
	}
	var original struct {
		Files []agentFile `json:"files"`
	}
	json.Unmarshal(agentSettings(before), &original)
	options.Previous = original.Files
	off := agentCall(t, sdk.AgentPlanRequest{ID: AgentID, Action: "disconnect", Binding: current, Options: raw(options), Settings: agentSettings(userEdited)})
	if off.Binding.Connected || strings.Contains(string(off.Files[0].Body), "host-only-key") {
		t.Fatal("disconnect retained owned gateway authority")
	}
	for _, value := range []string{"7000", "keep-model", "own/title", "9007199254740993", "independent-new-value"} {
		if !strings.Contains(string(off.Files[0].Body), value) {
			t.Fatal("disconnect dropped user config:", value)
		}
	}
}

func TestAgentPreservesNestedModelOverridesAndCustomVariants(t *testing.T) {
	hash := sha256.Sum256([]byte("opencode\x00" + ProviderGo))
	ref := "extension-" + hex.EncodeToString(hash[:12]) + "/glm-5.2"
	options := agentOptions{Gateway: "http://127.0.0.1:3425", Key: "host-only-key", Field: "model", Value: "magpie/" + ref,
		Models: []json.RawMessage{raw(map[string]string{"value": "magpie/" + ref})}}
	binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}}
	for _, format := range []string{"v1", "v2"} {
		t.Run(format, func(t *testing.T) {
			providerKey, capabilityKey := "providers", "capabilities"
			variants := `[{"id":"low","settings":{"reasoningEffort":"low"}},` +
				`{"id":"high","settings":{"reasoningEffort":"high","customSetting":"keep-high"}},` +
				`{"id":"personal","settings":{"customSetting":"keep-personal"}}]`
			if format == "v1" {
				providerKey, capabilityKey = "provider", "modalities"
				variants = `{"low":{"reasoningEffort":"low"},"high":{"reasoningEffort":"high","customSetting":"keep-high"},` +
					`"personal":{"customSetting":"keep-personal"}}`
			}
			before := []byte(`{"` + providerKey + `":{"magpie":{"models":{"` + ref +
				`":{"limit":{"output":7000,"customLimit":9007199254740993},"` + capabilityKey + `":{"customCapability":true},` +
				`"variants":` + variants + `}}}}}`)
			host := serveHarness(t, NewAgentModule(ModuleOptions{}), raw(moduleConfig{ProjectID: "global", AgentFormat: format}))
			frame, _ := host.invoke(sdk.MethodAgentPlan, sdk.Scope{}, sdk.AgentPlanRequest{ID: AgentID, Action: "set",
				Binding: binding, Options: raw(options), Settings: agentSettings(before)}, nil)
			if frame.Error != nil {
				t.Fatalf("SDK rejected agent plan: %+v", frame.Error)
			}
			var plan sdk.AgentPlan
			if json.Unmarshal(frame.Result, &plan) != nil || len(plan.Files) != 1 {
				t.Fatal("SDK lost config plan")
			}
			for _, expected := range []string{"7000", "9007199254740993", "customCapability", "keep-high", "keep-personal"} {
				if !strings.Contains(string(plan.Files[0].Body), expected) {
					t.Fatalf("agent erased a user model override %q", expected)
				}
			}
			if strings.Contains(string(plan.Files[0].Body), `"low"`) {
				t.Fatal("agent retained an obsolete generated variant")
			}
		})
	}
}
