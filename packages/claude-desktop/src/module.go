package claudedesktop

import (
	"context"
	"encoding/json"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	ModuleID             = "agent-claude-desktop"
	AgentID              = "claude-desktop-sdk"
	TargetID             = "claude-desktop"
	Version              = "0.1.0"
	ProfileID            = "a363652b-0f53-4cd6-aede-cd0000000001"
	ConfigPath           = "Library/Application Support/Claude/claude_desktop_config.json"
	ThirdPartyConfigPath = "Library/Application Support/Claude-3p/claude_desktop_config.json"
	MetaPath             = "Library/Application Support/Claude-3p/configLibrary/_meta.json"
	ProfilePath          = "Library/Application Support/Claude-3p/configLibrary/" + ProfileID + ".json"
	ReceiptPath          = "Library/Application Support/Magpie SDK/agents/claude-desktop.json"
)

func Manifest() sdk.Manifest {
	return sdk.Manifest{
		Schema: sdk.ManifestSchema, ID: ModuleID, Version: Version,
		Runtime: "native", Entry: "lib/claude-desktop.dylib",
		Contract:      sdk.Contract{Version: sdk.APIVersion{Major: 1, Minor: 5}, Capabilities: []sdk.Capability{sdk.AgentWiring}},
		Compatibility: &sdk.Compatibility{HostAPI: "^1.5", NativeABI: 1, Platforms: []string{"darwin/arm64", "darwin/amd64"}},
		Agents: []sdk.AgentDescriptor{{
			ID: AgentID, Name: "Claude Desktop SDK", Profile: &sdk.AgentProfile{Target: TargetID},
			ConfigFiles: []string{ConfigPath, ThirdPartyConfigPath, MetaPath, ProfilePath, ReceiptPath},
		}},
	}
}

func Module() sdk.Module {
	return sdk.Module{Manifest: Manifest(), Handlers: map[string]sdk.Handler{
		sdk.MethodAgentInspect: inspectAgent,
		sdk.MethodAgentPlan:    planAgent,
	}}
}

func inspectAgent(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.AgentInspectRequest
	if decode(call.Params, &request) != nil || checkBinding(request.ID, request.Binding) != nil {
		return nil, invalid()
	}
	files, err := readFiles(request.Settings)
	if err != nil {
		return nil, err
	}
	result := sdk.AgentInspectResult{Connected: request.Binding.Connected, Values: map[string]string{}}
	if !result.Connected {
		return result, nil
	}
	options, err := readOptions(request.Options)
	if err != nil {
		return nil, err
	}
	sealed, err := checkConnection(files, options)
	if err != nil {
		return nil, err
	}
	result.Detail = "Restart Claude Desktop to load the gateway profile. A managed device policy may override local configuration."
	if sealed {
		result.Detail += " Desktop sealed its gateway credential; its value cannot be verified from the configuration snapshot."
	}
	return result, nil
}

func checkBinding(id string, binding *sdk.AgentBindingSnapshot) error {
	manifest := Manifest()
	if id != AgentID || binding == nil || sdk.ValidateAgentBindingSnapshot(manifest, manifest.Agents[0], binding) != nil {
		return invalid()
	}
	return nil
}

func raw(value any) json.RawMessage {
	body, _ := json.Marshal(value)
	return body
}

func invalid() error {
	return &sdk.Error{Code: "invalid_params", Message: "Claude Desktop requires valid host-owned configuration and binding snapshots", Outcome: "not-started"}
}
