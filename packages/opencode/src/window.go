package opencode

import (
	"context"
	"encoding/json"
	"strconv"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const windowVersion = "0.8.2"

func WindowManifest() sdk.Manifest {
	manifest := API8WindowManifest()
	manifest.Version = windowVersion
	manifest.Contract.Capabilities = append(manifest.Contract.Capabilities, sdk.ProviderSignIn)
	device := sdk.WindowAuthMethod{ID: "device", Mode: "poll", Label: "Sign in with OpenCode"}
	manifest.AuthBindings = append(manifest.AuthBindings,
		sdk.WindowAuthBindingDescriptor{
			ID: "provider.go", From: sdk.ProviderSignInPart, Provider: ProviderGo, Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{device},
		},
		sdk.WindowAuthBindingDescriptor{
			ID: "provider.zen", From: sdk.ProviderSignInPart, Provider: ProviderZen, Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{device, {
				ID: "key", Mode: "poll", Label: "Use API key",
				Fields: []sdk.AuthInputField{{Key: "key", Label: "API key", Type: "secret"}},
			}},
		},
		sdk.WindowAuthBindingDescriptor{
			ID: "provider.zen-free", From: sdk.ProviderSignInPart, Provider: ProviderZenPublic,
			Methods: []sdk.WindowAuthMethod{{ID: "public", Mode: "poll", Label: "Enable free models"}},
		},
	)
	return manifest
}

func API8WindowManifest() sdk.Manifest {
	manifest := AgentManifest()
	manifest.Version = "0.8.1"
	protocols := []protocolext.Protocol{"chat", "responses", "messages", "gemini"}
	manifest.Providers = append(manifest.Providers,
		sdk.ProviderDescriptor{ID: ProviderZen, Name: "OpenCode Zen personal", Protocols: protocols, Models: zenBootstrap(false)},
		sdk.ProviderDescriptor{ID: ProviderZenPublic, Name: "OpenCode Zen public", Protocols: protocols, Models: zenBootstrap(true)})
	manifest.Contract.Capabilities = append(manifest.Contract.Capabilities, sdk.ModuleRPC)
	manifest.Window = &sdk.WindowDescriptor{Metadata: "package.json"}
	manifest.Actions = []sdk.ActionDescriptor{{ID: "account.info", Effect: "read", InputSchema: json.RawMessage(`{"type":["object","null"],"additionalProperties":false}`)}}
	manifest.Bindings = []sdk.BindingDescriptor{{ID: "account.info", From: "window:page", Action: "account.info", Provider: ProviderGo}}
	manifest.AuthBindings = []sdk.WindowAuthBindingDescriptor{{
		ID: "signin", From: "window:page", Provider: ProviderGo, Reauthorize: true,
		Methods: []sdk.WindowAuthMethod{{ID: "device", Mode: "poll"}},
	}}
	for _, entry := range []struct {
		Provider, Binding, Auth string
		Methods                 []sdk.WindowAuthMethod
	}{
		{ProviderZen, "zen.account.info", "zen.signin", []sdk.WindowAuthMethod{{ID: "device", Mode: "poll"}, {ID: "key", Mode: "poll"}}},
		{ProviderZenPublic, "public.account.info", "public.signin", []sdk.WindowAuthMethod{{ID: "public", Mode: "poll"}}},
	} {
		manifest.Bindings = append(manifest.Bindings, sdk.BindingDescriptor{ID: entry.Binding, From: "window:page", Action: "account.info", Provider: entry.Provider})
		manifest.AuthBindings = append(manifest.AuthBindings, sdk.WindowAuthBindingDescriptor{ID: entry.Auth, From: "window:page",
			Provider: entry.Provider, Methods: entry.Methods, Reauthorize: true})
	}
	for _, name := range []string{"console.snapshot", "console.skill", "console.watch.list", "console.watch.types", "console.watch.poll", "console.watch.ack"} {
		effect, schema := "read", `{"type":"object","additionalProperties":false}`
		switch name {
		case "console.skill":
			schema = `{"type":"object","required":["slug"],"properties":{"slug":{"type":"string","minLength":1,"maxLength":256}},"additionalProperties":false}`
		case "console.watch.poll":
			schema = `{"type":"object","properties":{"cursor":{"type":"string","maxLength":2048}},"additionalProperties":false}`
		case "console.watch.ack":
			effect = "write"
			schema = `{"type":"object","required":["receipt","ids"],"properties":{"receipt":{"type":"string","minLength":32,"maxLength":32},"ids":{"type":"array","minItems":1,"maxItems":1000,"items":{"type":"string","minLength":1,"maxLength":128}}},"additionalProperties":false}`
		}
		manifest.Actions = append(manifest.Actions, sdk.ActionDescriptor{ID: name, Effect: effect, InputSchema: json.RawMessage(schema)})
		manifest.Bindings = append(manifest.Bindings, sdk.BindingDescriptor{ID: name, From: "window:page", Action: name, Provider: ProviderZen})
	}
	declareHistory(&manifest)
	return manifest
}

func NewAPI8WindowModule(options ModuleOptions) sdk.Module {
	return newWindowModule(options, API8WindowManifest())
}

func NewWindowModule(options ModuleOptions) sdk.Module {
	return newWindowModule(options, WindowManifest())
}

func newWindowModule(options ModuleOptions, manifest sdk.Manifest) sdk.Module {
	module := newModule(options, true)
	module.Manifest = manifest
	module.Handlers[sdk.MethodAgentInspect] = inspectAgent
	module.Handlers[sdk.MethodAgentPlan] = planAgent
	module.Actions["account.info"] = func(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(call.Params) != 0 && string(call.Params) != "null" && string(call.Params) != "{}" {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Account information accepts no business authority"}
		}
		var expires int64
		if call.Scope.Provider == ProviderZen || call.Scope.Provider == ProviderZenPublic {
			identity, err := selectedZen(call)
			if err != nil {
				return nil, notStarted(err)
			}
			if identity.OAuth != nil {
				expires = identity.OAuth.Expires.UnixMilli()
			}
		} else {
			credentials, err := selected(call)
			if err != nil {
				return nil, notStarted(err)
			}
			expires = credentials.Expires.UnixMilli()
		}
		return sdk.AccountSummary{ID: call.Scope.Account, Generation: strconv.FormatUint(call.Scope.Generation, 10),
			ExpiresAt: expires}, nil
	}
	EnableHistory(&module)
	return module
}
