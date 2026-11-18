package opencode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAgentPreservesNativeConsoleOwnership(t *testing.T) {
	for _, format := range []string{"v1", "v2"} {
		for _, nativeWatches := range []bool{false, true} {
			for _, moduleWatches := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/native-%t/module-%t", format, nativeWatches, moduleWatches), func(t *testing.T) {
					providerKey := "providers"
					if format == "v1" {
						providerKey = "provider"
					}
					original := map[string]any{
						providerKey: map[string]any{"opencode": map[string]any{"name": "Native Console"}},
						"model":     "opencode/fixture-native",
						"policies": []any{map[string]any{
							"action": "tool.use", "resource": "shell:sudo *", "effect": "deny",
						}},
						"experimental": map[string]any{"console_watches": nativeWatches},
						"instructions": []string{"./AGENTS.md"},
						"skills":       []string{"./skills"},
					}
					before := raw(original)
					harness := serveHarness(t, NewWindowModule(ModuleOptions{}),
						raw(moduleConfig{AgentFormat: format, ConsoleWatches: moduleWatches}))
					options := agentOptions{Gateway: "http://127.0.0.1:3425", Key: "gateway-key-only",
						Field: "model", Value: "magpie/fixture/model",
						Models: []json.RawMessage{raw(map[string]any{"value": "magpie/fixture/model"})}}
					binding := &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "0", Values: map[string]string{}}
					request := sdk.AgentPlanRequest{ID: AgentID, Action: "set", Binding: binding,
						Options: raw(options), Settings: agentSettings(before)}
					invoke := func(request sdk.AgentPlanRequest) sdk.AgentPlan {
						t.Helper()
						frame, _ := harness.invoke(sdk.MethodAgentPlan, sdk.Scope{}, request, nil)
						var plan sdk.AgentPlan
						if frame.Error != nil || json.Unmarshal(frame.Result, &plan) != nil {
							t.Fatal("native-owner preservation plan failed", frame.Error)
						}
						return plan
					}
					check := func(body []byte) {
						t.Helper()
						var actual, expected map[string]json.RawMessage
						if json.Unmarshal(body, &actual) != nil || json.Unmarshal(before, &expected) != nil {
							t.Fatal("invalid checked overlay")
						}
						for _, key := range []string{"policies", "experimental", "instructions", "skills"} {
							if !sameToolJSON(actual[key], expected[key]) {
								t.Fatal("Magpie changed native Console-owned configuration", key)
							}
						}
						var providers map[string]json.RawMessage
						if json.Unmarshal(actual[providerKey], &providers) != nil ||
							!sameToolJSON(providers["opencode"], raw(original[providerKey].(map[string]any)["opencode"])) {
							t.Fatal("Magpie replaced native Console provider ownership")
						}
					}
					connected := invoke(request)
					if len(connected.Files) != 1 {
						t.Fatal("connect did not produce one overlay")
					}
					body := connected.Files[0].Body
					check(body)
					request.Binding = &sdk.AgentBindingSnapshot{Target: "opencode", Revision: "1",
						Connected: true, Values: connected.Binding.Values}
					request.Settings, request.Action = agentSettings(body), "sync"
					if len(invoke(request).Files) != 0 {
						t.Fatal("native-owner-preserving sync rewrote unchanged config")
					}
					options.Previous = []agentFile{{Path: agentConfigPath, Exists: true, Body: before}}
					request.Options, request.Action = raw(options), "disconnect"
					disconnected := invoke(request)
					if len(disconnected.Files) != 1 || disconnected.Binding.Connected {
						t.Fatal("disconnect did not restore the checked original")
					}
					check(disconnected.Files[0].Body)
				})
			}
		}
	}
	launch := WindowManifest().Agents[0].Launch
	if !reflect.DeepEqual(launch.Env, map[string]string{"OPENCODE_CONFIG": "{config}"}) {
		t.Fatal("launch added native Console credential or flag overrides")
	}
}
