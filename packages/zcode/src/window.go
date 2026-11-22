package zcode

import (
	"context"
	"encoding/json"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func ControlManifest() sdk.Manifest {
	manifest := Manifest()
	manifest.Version = "0.16.0"
	manifest.Providers[0].MaxConcurrency = new(3)
	manifest.Contract.Capabilities = append(manifest.Contract.Capabilities,
		sdk.AgentWiring, sdk.StorageSource, sdk.AgentProcess, sdk.ProviderSignIn, sdk.ProviderSavedSignIn)
	manifest.Processes = []sdk.AgentProcessRecipe{{ID: "native.run", Agent: AgentID, Argv: []string{"app-server", "--stdio"}}}
	manifest.StorageSources = []sdk.StorageSourceDescriptor{historyDescriptor()}
	manifest.Agents = []sdk.AgentDescriptor{agentDescriptor()}
	manifest.Window = &sdk.WindowDescriptor{Metadata: "package.json"}
	manifest.AuthBindings = []sdk.WindowAuthBindingDescriptor{{
		ID: "provider.signin", From: sdk.ProviderSignInPart, Provider: ProviderID, Reauthorize: true,
		Methods: []sdk.WindowAuthMethod{{ID: "browser", Mode: "poll", Label: "Sign in with ZCode", Fields: []sdk.AuthInputField{
			{Key: "site", Label: "Site", Type: "select", Optional: true, Choices: []sdk.AuthInputChoice{
				{Value: "zai", Label: "Z.ai"}, {Value: "bigmodel", Label: "BigModel"}, {Value: "zai_cn", Label: "Z.ai China (BigModel)"},
			}},
			{Key: "kind", Label: "Plan", Type: "select", Optional: true, Choices: []sdk.AuthInputChoice{
				{Value: "auto", Label: "Automatic"}, {Value: "individual", Label: "Personal"}, {Value: "team", Label: "Team"}, {Value: "start", Label: "Start"},
			}},
			{Key: "organization", Label: "Organization ID", Type: "text", Optional: true},
			{Key: "project", Label: "Project ID", Type: "text", Optional: true},
			{Key: "product", Label: "Product ID", Type: "text", Optional: true},
			{Key: "apiKeyId", Label: "API key ID", Type: "text", Optional: true},
		}}, {ID: apiKeySignIn, Mode: "poll", Label: "GLM Coding Plan API key", Fields: []sdk.AuthInputField{
			{Key: "site", Label: "Site", Type: "select", Optional: true, Choices: []sdk.AuthInputChoice{
				{Value: "zai", Label: "Z.ai"}, {Value: "bigmodel", Label: "BigModel"},
			}},
			{Key: "key", Label: "API key", Type: "secret"},
		}}, {ID: sdk.SavedSignInMethod, Mode: "poll", Label: "Import saved ZCode sign-in", Fields: []sdk.AuthInputField{
			{Key: "record", Label: "Saved sign-in", Type: "secret"},
		}}},
	}}
	manifest.Actions = append(manifest.Actions, sdk.ActionDescriptor{
		ID: "execution.summary", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "read",
	})
	manifest.Actions = append(manifest.Actions,
		sdk.ActionDescriptor{ID: "native.run", Effect: "write", InputSchema: json.RawMessage(`{"type":"object","properties":{"sessionId":{"type":"string","maxLength":256},"commandId":{"type":"string","maxLength":256},"model":{"type":"string","maxLength":1024},"text":{"type":"string","maxLength":65536}},"required":["sessionId","commandId","model","text"],"additionalProperties":false}`)},
		sdk.ActionDescriptor{ID: "native.stop", Effect: "write", InputSchema: json.RawMessage(`{"type":"object","properties":{"sessionId":{"type":"string","maxLength":256},"commandId":{"type":"string","maxLength":256}},"required":["sessionId","commandId"],"additionalProperties":false}`)})
	for _, value := range []struct{ id, action string }{
		{"page.execution-list", "execution.summary"},
		{"page.billing-status", "billing.status"},
		{"page.reset-status", "reset.status"},
		{"page.reset-use", "reset.use"},
		{"page.reset-summary", "reset.summary"},
		{"page.reset-reconcile", "reset.reconcile"},
		{"page.reset-opportunity", "reset.opportunity"},
		{"page.reset-opportunity-resolve", "reset.opportunity.resolve"},
		{"page.reset-resolve", "reset.resolve"},
		{"page.reset-ack", "reset.ack"},
		{"page.billing-claim", "billing.claim"},
		{"page.billing-offers", "billing.offers"},
		{"page.billing-checkin", "billing.checkin"},
		{"page.native-run", "native.run"},
		{"page.native-stop", "native.stop"},
	} {
		manifest.Bindings = append(manifest.Bindings, sdk.BindingDescriptor{
			ID: value.id, From: "window:panel:0", Action: value.action, Provider: ProviderID,
		})
	}
	return manifest
}

func NewControlModule(options ModuleOptions) sdk.Module {
	owner := &module{}
	module := newModule(options, ControlManifest())
	module.Handlers[sdk.MethodAgentInspect] = owner.agentInspect
	module.Handlers[sdk.MethodAgentPlan] = owner.agentPlan
	module.Handlers[sdk.MethodStorageList] = automaticHistory
	module.Handlers[sdk.MethodStorageRead] = automaticHistory
	list := module.Actions["execution.list"]
	module.Actions["execution.summary"] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		value, err := list(ctx, call, stream)
		if err != nil {
			return nil, err
		}
		return summarizeExecutions(value.([]executionView)), nil
	}
	for method, phase := range map[string]string{
		sdk.MethodProviderExecute: "execute", sdk.MethodProviderModels: "models", sdk.MethodProviderUsage: "usage",
		sdk.MethodAuthStart: "auth_start", sdk.MethodAuthFinish: "auth_finish", sdk.MethodAuthRefresh: "auth_refresh",
		sdk.MethodOperationStatus: "operation_status", sdk.MethodOperationCancel: "operation_cancel",
		sdk.MethodToolCall: "tool",
	} {
		module.Handlers[method] = diagnosticHandler(phase, module.Handlers[method])
	}
	for action, handler := range module.Actions {
		module.Actions[action] = diagnosticHandler(action, handler)
	}
	return module
}

func summarizeExecutions(entries []executionView) []executionView {
	result := append([]executionView{}, entries...)
	for index := range result {
		result[index].Ticket, result[index].Card = "", ""
	}
	return result
}
