package devin

import (
	"context"
	_ "embed"
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

//go:embed magpie-agent.json
var agentDeclaration []byte

const AgentID = "devin-native"

type agentOwner struct {
	manifest   sdk.Manifest
	descriptor sdk.AgentDescriptor
}

type agentOptions struct {
	Gateway        string                   `json:"gateway"`
	Key            string                   `json:"key"`
	Models         json.RawMessage          `json:"models"`
	Field          string                   `json:"field,omitempty"`
	Value          string                   `json:"value,omitempty"`
	Previous       json.RawMessage          `json:"previous,omitempty"`
	PriorModels    json.RawMessage          `json:"priorModels,omitempty"`
	DecisionModels []sdk.AgentDecisionModel `json:"decisionModels,omitempty"`
}

func NewAgent(options ModuleOptions) (sdk.Module, error) {
	module, err := NewModule(options)
	if err != nil {
		return sdk.Module{}, err
	}
	var manifest sdk.Manifest
	if err := json.Unmarshal(agentDeclaration, &manifest); err != nil {
		return sdk.Module{}, err
	}
	manifest.Entry = module.Manifest.Entry
	owner := &agentOwner{manifest: manifest, descriptor: manifest.Agents[0]}
	module.Manifest = manifest
	module.Handlers[sdk.MethodAgentInspect] = owner.inspect
	module.Handlers[sdk.MethodAgentPlan] = owner.plan
	for _, kind := range []codec.AccountKind{codec.UserStatus, codec.TeamSettings, codec.ManagedPlugins, codec.ModelAssignment} {
		module.Inbounds = append(module.Inbounds, codec.AccountAdapter{Kind: kind})
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTPFactory
	}
	provider := &providerOwner{options: options}
	module.Actions = make(map[string]sdk.Handler)
	for _, declaration := range manifest.AccountOperations {
		module.Actions[declaration.Action] = provider.accountOperation
	}
	if task, found := manifest.AccountTask(ProviderID, cacheTaskID); found {
		module.Actions[task.Action] = provider.cacheTask
	}
	start := module.Start
	module.Start = func(ctx context.Context, configuration sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
		ready, dispose, err := start(ctx, configuration)
		if err == nil {
			state, ok := ready.State.(*providerState)
			if !ok {
				return sdk.ModuleReady{}, dispose, agentFailure("Missing Devin agent provider state")
			}
			state.agentAssignments = true
			state.richQuota = true
			_, state.cacheTasks = manifest.AccountTask(ProviderID, cacheTaskID)
		}
		return ready, dispose, err
	}
	return module, module.Validate()
}

func agentFailure(message string) error {
	return &sdk.Error{Code: "invalid_params", Message: message, Outcome: "not-started"}
}

func (owner *agentOwner) binding(id string, value *sdk.AgentBindingSnapshot) error {
	if id != AgentID || value == nil || sdk.ValidateAgentBindingSnapshot(owner.manifest, owner.descriptor, value) != nil {
		return agentFailure("Devin native requires its current host-owned agent binding")
	}
	if value.Values["model"] != "" {
		if _, err := nativeAgentModel(value.Values["model"]); err != nil {
			return err
		}
	}
	return nil
}

func nativeAgentModel(value string) (string, error) {
	value = strings.TrimPrefix(value, "magpie/")
	if value == "" || len(value) > 8192 || strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") ||
		strings.ContainsFunc(value, func(char rune) bool { return unicode.IsSpace(char) || unicode.IsControl(char) }) {
		return "", agentFailure("Select an explicit gateway model for Devin native")
	}
	return value, nil
}

func agentLoopback(value string) bool {
	address, err := url.Parse(value)
	if err != nil || address.Scheme != "http" && address.Scheme != "https" ||
		address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" ||
		address.Path != "" && address.Path != "/" {
		return false
	}
	host := address.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

func (owner *agentOwner) inspect(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.AgentInspectRequest
	if err := decodeParams(call.Params, &request); err != nil {
		return nil, err
	}
	if err := owner.binding(request.ID, request.Binding); err != nil {
		return nil, err
	}
	return sdk.AgentInspectResult{Connected: request.Binding.Connected, Values: request.Binding.Clone().Values,
		Detail: "Native Connect chat/catalog and selected-account bootstrap with session identifiers"}, nil
}

func (owner *agentOwner) plan(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.AgentPlanRequest
	if err := decodeParams(call.Params, &request); err != nil {
		return nil, err
	}
	if err := owner.binding(request.ID, request.Binding); err != nil {
		return nil, err
	}
	options := agentOptions{DecisionModels: []sdk.AgentDecisionModel{}}
	if err := decodeParams(request.Options, &options); err != nil {
		return nil, err
	}
	if err := sdk.ValidateAgentDecisionModels(owner.manifest, options.DecisionModels); err != nil {
		return nil, agentFailure("Invalid Devin agent decision catalog")
	}
	change := &sdk.AgentBindingChange{Revision: request.Binding.Revision, Connected: request.Binding.Connected,
		Values: request.Binding.Clone().Values}
	switch request.Action {
	case "connect":
		change.Connected = true
	case "set":
		if options.Field != "model" {
			return nil, agentFailure("Devin native supports only the model field")
		}
		model, err := nativeAgentModel(options.Value)
		if err != nil {
			return nil, err
		}
		change.Values["model"], change.Connected = model, true
	case "sync":
	case "disconnect":
		change.Connected = false
	default:
		return nil, agentFailure("Unsupported Devin native agent action")
	}
	if change.Connected {
		if !agentLoopback(options.Gateway) {
			return nil, agentFailure("Devin native launch requires a loopback gateway origin")
		}
		model, err := nativeAgentModel(change.Values["model"])
		if err != nil {
			return nil, err
		}
		change.Values["model"] = model
	}
	if err := sdk.ValidateAgentBindingChange(owner.manifest, owner.descriptor, request.Binding, request.Action, change); err != nil {
		return nil, agentFailure("Devin native agent change does not match the host binding")
	}
	return sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: change}, nil
}
