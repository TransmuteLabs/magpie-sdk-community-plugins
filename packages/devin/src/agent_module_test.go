package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func agentModuleFixture(t *testing.T) sdk.Module {
	t.Helper()
	module, err := NewAgent(ModuleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func agentRequest(t *testing.T, module sdk.Module, method string, value any) (any, error) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return module.Handlers[method](context.Background(), sdk.Call{Params: raw}, nil)
}

func TestAgentSlicePreservesPairedContractAndDeclaresExplicitProfile(t *testing.T) {
	paired, err := New()
	if err != nil {
		t.Fatal(err)
	}
	module := agentModuleFixture(t)
	if paired.Manifest.Version != "0.5.0" || paired.Manifest.Contract.Version.Minor != 12 || !paired.Manifest.Contract.Has(sdk.RequestPolicy) ||
		len(paired.Manifest.Agents) != 0 || paired.Manifest.Contract.Has(sdk.AgentWiring) ||
		paired.Handlers[sdk.MethodAgentPlan] != nil {
		t.Fatal("agent slice changed the paired distribution")
	}
	if module.Manifest.Contract.Version.Minor != 12 || !module.Manifest.Contract.Has(sdk.RequestPolicy) || !module.Manifest.Contract.Has(sdk.AgentWiring) ||
		module.Manifest.ID != paired.Manifest.ID || len(module.Manifest.Agents) != 1 {
		t.Fatal("agent slice did not declare its own negotiation floor")
	}
	descriptor := module.Manifest.Agents[0]
	if descriptor.ID != AgentID || descriptor.Profile == nil || descriptor.Profile.Target != "devin" ||
		descriptor.Bin != "devin" || descriptor.Launch.Model != "--model" ||
		!reflect.DeepEqual(descriptor.Launch.Env, map[string]string{
			"WINDSURF_API_SERVER_URL": "{url}", "DEVIN_API_KEY": "{key}"}) ||
		len(descriptor.ConfigFiles) != 0 {
		t.Fatal("native CLI launch metadata does not match recovered endpoint/key/model")
	}
	for index, inbound := range paired.Inbounds {
		if module.Inbounds[index].ID() != inbound.ID() || module.Inbounds[index].Path() != inbound.Path() ||
			!reflect.DeepEqual(module.Inbounds[index].Contract(), inbound.Contract()) {
			t.Fatal("agent slice changed a paired codec contract")
		}
	}
	lowered := module.Manifest.Clone()
	lowered.Contract.Version.Minor = 4
	if lowered.Validate() == nil {
		t.Fatal("agent launch/profile metadata accepted an SDK 1.4 floor")
	}
}

func TestAgentHostBindingLifecycle(t *testing.T) {
	module := agentModuleFixture(t)
	before := &sdk.AgentBindingSnapshot{Target: "devin", Revision: "7", Values: map[string]string{}}
	raw, _ := json.Marshal(agentOptions{Gateway: "http://127.0.0.1:3425", Key: "private-host-key",
		Field: "model", Value: "magpie/group/executor"})
	value, err := agentRequest(t, module, sdk.MethodAgentPlan, sdk.AgentPlanRequest{
		ID: AgentID, Action: "set", Binding: before, Options: raw})
	if err != nil {
		t.Fatal(err)
	}
	plan := value.(sdk.AgentPlan)
	if len(plan.Files) != 0 || !plan.Binding.Connected || plan.Binding.Revision != "7" ||
		plan.Binding.Values["model"] != "group/executor" || len(plan.Binding.Values) != 1 ||
		len(before.Values) != 0 || before.Connected {
		t.Fatal("plan lost host CAS, leaked credentials, mutated input or failed model translation")
	}
	current := &sdk.AgentBindingSnapshot{Target: "devin", Revision: "8", Connected: true, Values: plan.Binding.Values}
	value, err = agentRequest(t, module, sdk.MethodAgentInspect, sdk.AgentInspectRequest{ID: AgentID, Binding: current})
	if err != nil || !value.(sdk.AgentInspectResult).Connected ||
		value.(sdk.AgentInspectResult).Values["model"] != "group/executor" {
		t.Fatal("inspection lost the host-owned binding", err)
	}
	value, err = agentRequest(t, module, sdk.MethodAgentPlan, sdk.AgentPlanRequest{
		ID: AgentID, Action: "disconnect", Binding: current})
	if err != nil || value.(sdk.AgentPlan).Binding.Connected || value.(sdk.AgentPlan).Binding.Revision != "8" {
		t.Fatal("disconnect required a live gateway or lost CAS", err)
	}
	for _, action := range []string{"sync", "connect"} {
		value, err = agentRequest(t, module, sdk.MethodAgentPlan, sdk.AgentPlanRequest{
			ID: AgentID, Action: action, Binding: current, Options: raw})
		if err != nil || !value.(sdk.AgentPlan).Binding.Connected ||
			value.(sdk.AgentPlan).Binding.Values["model"] != "group/executor" {
			t.Fatal("existing binding failed "+action, err)
		}
	}
}

func TestAgentModelTransitionAcceptsAPI9PriorCatalogWithoutGrantingAuthority(t *testing.T) {
	module := agentModuleFixture(t)
	before := &sdk.AgentBindingSnapshot{Target: "devin", Revision: "7", Connected: true,
		Values: map[string]string{"model": "group/previous"}}
	options := json.RawMessage(`{"gateway":"http://127.0.0.1:3425","field":"model","value":"magpie/group/current","models":[{"value":"magpie/group/current","maxOutput":4096,"efforts":["low","high"],"defaultEffort":"low"}],"priorModels":[{"value":"magpie/group/previous","maxOutput":2048,"efforts":null}]}`)
	value, err := agentRequest(t, module, sdk.MethodAgentPlan, sdk.AgentPlanRequest{
		ID: AgentID, Action: "set", Binding: before, Options: options})
	if err != nil {
		t.Fatal("API9 model transition rejected its prior catalog", err)
	}
	plan := value.(sdk.AgentPlan)
	if len(plan.Files) != 0 || plan.Binding.Revision != "7" || !plan.Binding.Connected ||
		plan.Binding.Values["model"] != "group/current" || len(plan.Binding.Values) != 1 ||
		before.Values["model"] != "group/previous" || !before.Connected {
		t.Fatal("prior catalog changed host authority, selected model, input snapshot or file-free planning")
	}
}

func TestAgentAPI9DecisionCatalogPreservesConversationBinding(t *testing.T) {
	module := agentModuleFixture(t)
	for _, sample := range []struct{ name, catalog string }{
		{"omitted", ""},
		{"empty", `[]`},
		{"typed provider and group", `[{"id":"judge/native","name":"Native judge","provider":"judge","endpoints":["/v1/systemone","/v1/decisions"]},` +
			`{"id":"group/judges","name":"Judges","group":true,"endpoints":["/v1/systemone"]}]`},
	} {
		for _, action := range []string{"connect", "set", "sync", "disconnect"} {
			t.Run(action+"/"+sample.name, func(t *testing.T) {
				before := &sdk.AgentBindingSnapshot{Target: "devin", Revision: "11", Connected: true,
					Values: map[string]string{"model": "group/conversation"}}
				options := map[string]any{
					"gateway": "http://127.0.0.1:3425", "key": "private-host-key",
					"field": "model", "value": "magpie/group/conversation",
					"models": json.RawMessage(`[{"value":"magpie/group/conversation","maxOutput":4096,"efforts":["low","high"],"defaultEffort":"low"}]`),
				}
				if sample.catalog != "" {
					options["decisionModels"] = json.RawMessage(sample.catalog)
				}
				raw, err := json.Marshal(options)
				if err != nil {
					t.Fatal(err)
				}
				value, err := agentRequest(t, module, sdk.MethodAgentPlan, sdk.AgentPlanRequest{
					ID: AgentID, Action: action, Binding: before, Options: raw})
				if err != nil {
					t.Fatal("API9 decision catalog rejected before agent planning", err)
				}
				plan := value.(sdk.AgentPlan)
				if len(plan.Files) != 0 || plan.Binding == nil || plan.Binding.Revision != before.Revision ||
					plan.Binding.Connected != (action != "disconnect") ||
					!reflect.DeepEqual(plan.Binding.Values, map[string]string{"model": "group/conversation"}) {
					t.Fatalf("decision catalog leaked into client files, conversation models or binding authority: %+v", plan)
				}
				if !before.Connected || before.Revision != "11" ||
					!reflect.DeepEqual(before.Values, map[string]string{"model": "group/conversation"}) {
					t.Fatal("decision catalog mutated the host-owned input binding")
				}
			})
		}
	}
}

func TestAgentAPI9DecisionCatalogRejectsInvalidAndUnknownOptions(t *testing.T) {
	module := agentModuleFixture(t)
	for _, sample := range []struct{ name, catalog string }{
		{"null", `null`},
		{"object", `{}`},
		{"missing model identity", `[{"name":"Judge","provider":"judge","endpoints":["/v1/systemone"]}]`},
		{"missing model provider", `[{"id":"judge/native","name":"Judge","endpoints":["/v1/systemone"]}]`},
		{"group with provider", `[{"id":"group/judge","name":"Judge","group":true,"provider":"judge","endpoints":["/v1/systemone"]}]`},
		{"conversation endpoint", `[{"id":"judge/native","name":"Judge","provider":"judge","endpoints":["/v1/chat/completions"]}]`},
		{"duplicate endpoint", `[{"id":"judge/native","name":"Judge","provider":"judge","endpoints":["/v1/decisions","/v1/decisions"]}]`},
		{"unknown decision field", `[{"id":"judge/native","name":"Judge","provider":"judge","endpoints":["/v1/decisions"],"overrideAccount":"foreign"}]`},
	} {
		t.Run(sample.name, func(t *testing.T) {
			request := sdk.AgentPlanRequest{ID: AgentID, Action: "sync",
				Binding: &sdk.AgentBindingSnapshot{Target: "devin", Revision: "11", Connected: true,
					Values: map[string]string{"model": "group/conversation"}},
				Options: json.RawMessage(`{"gateway":"http://127.0.0.1:3425","decisionModels":` + sample.catalog + `}`)}
			if _, err := agentRequest(t, module, sdk.MethodAgentPlan, request); err == nil {
				t.Fatal("invalid or unknown decision facts were accepted")
			}
		})
	}
}

func TestAgentRejectsMissingAuthorityAndUnrepresentableChanges(t *testing.T) {
	module := agentModuleFixture(t)
	valid := sdk.AgentPlanRequest{ID: AgentID, Action: "set",
		Binding: &sdk.AgentBindingSnapshot{Target: "devin", Revision: "0", Values: map[string]string{}},
		Options: json.RawMessage(`{"gateway":"http://127.0.0.1:3425","field":"model","value":"magpie/group/work"}`)}
	for _, name := range []string{"no-binding", "foreign-target", "foreign-id", "bad-revision",
		"undeclared-field", "empty-model", "flag-model", "control-model", "remote-gateway",
		"user-url", "gateway-query", "unknown-action", "implicit-model", "unknown-option"} {
		t.Run(name, func(t *testing.T) {
			request := valid
			request.Binding = valid.Binding.Clone()
			options := map[string]any{}
			_ = json.Unmarshal(valid.Options, &options)
			switch name {
			case "no-binding":
				request.Binding = nil
			case "foreign-target":
				request.Binding.Target = "other"
			case "foreign-id":
				request.ID = "other"
			case "bad-revision":
				request.Binding.Revision = "01"
			case "undeclared-field":
				options["field"] = "credentials"
			case "empty-model":
				options["value"] = ""
			case "flag-model":
				options["value"] = "magpie/--cloud"
			case "control-model":
				options["value"] = "magpie/group/work\nignored"
			case "remote-gateway":
				options["gateway"] = "https://public.example"
			case "user-url":
				options["gateway"] = "http://secret@localhost"
			case "gateway-query":
				options["gateway"] = "http://localhost/?key=secret"
			case "unknown-action":
				request.Action = "execute"
			case "implicit-model":
				request.Action = "connect"
			case "unknown-option":
				options["overrideAccount"] = "other"
			}
			request.Options, _ = json.Marshal(options)
			if _, err := agentRequest(t, module, sdk.MethodAgentPlan, request); err == nil {
				t.Fatal("unsupported or unowned change was accepted")
			}
		})
	}
}

func TestAgentBootstrapRequiresSelectedAccountDispatch(t *testing.T) {
	module := agentModuleFixture(t)
	for _, inbound := range module.Inbounds[8:] {
		headers := http.Header{"Content-Type": {"application/proto"}, "Connect-Protocol-Version": {"1"}}
		body := nativeText(1, string(nativeText(3, "local-not-vendor")))
		switch inbound.Path() {
		case codec.ManagedPluginBundle.Path():
			body = append(body, nativeFields(nativeText(2, "owned-fixture-bundle"), nativeNumber(3, 1))...)
		case codec.ModelAssignment.Path():
			body = append(body, nativeFields(nativeText(2, "model"), nativeText(3, "session"))...)
		}
		request, err := inbound.Decode(context.Background(), headers, body)
		if err != nil || request.Protocol != protocolext.AccountOperation ||
			request.Executor != protocolext.SameModuleExecutor || bytes.Contains(request.Body, []byte("local-not-vendor")) {
			t.Fatal("agent bootstrap did not defer to its selected account authority", err)
		}
		if _, declared := module.Manifest.AccountOperation(request.AccountOperation); !declared {
			t.Fatal("agent bootstrap selected an undeclared account operation")
		}
		recorder := httptest.NewRecorder()
		reply := inbound.Reply(recorder)
		reply.WriteHeader(http.StatusForbidden)
		_, _ = reply.Write([]byte(`{"error":{"message":"private host failure"}}`))
		if err := reply.Finish(nil); err != nil {
			t.Fatal(err)
		}
		var status map[string]string
		if recorder.Code != http.StatusForbidden || json.Unmarshal(recorder.Body.Bytes(), &status) != nil ||
			status["code"] != "permission_denied" || bytes.Contains(recorder.Body.Bytes(), []byte("private host failure")) {
			t.Fatal("agent bootstrap fabricated success or exposed a private host failure")
		}
	}
}
