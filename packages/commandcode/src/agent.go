package commandcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type settingFile struct {
	Path       string `json:"path"`
	Body       []byte `json:"body"`
	BeforeHash string `json:"beforeHash"`
	Mode       uint32 `json:"mode"`
	Exists     bool   `json:"exists"`
}

type agentOptions struct {
	Gateway        string                   `json:"gateway"`
	Field          string                   `json:"field"`
	Value          string                   `json:"value"`
	Models         []agentModelOption       `json:"models"`
	PriorModels    []agentModelOption       `json:"priorModels"`
	Previous       []settingFile            `json:"previous"`
	DecisionModels []sdk.AgentDecisionModel `json:"decisionModels,omitempty"`
}

type agentModelOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Context int64  `json:"context"`
	sdk.AgentModelMetadata
}

var commandCodeEfforts = []string{"none", "low", "medium", "high", "xhigh", "max"}

func commandCodeEffort(value string) string {
	if value == "none" {
		return "off"
	}
	return value
}

func canonicalAgentEffort(value string) string {
	if value == "off" {
		return "none"
	}
	return value
}

func checkedAgentOptions(raw json.RawMessage) (agentOptions, error) {
	var options agentOptions
	envelope := struct {
		*agentOptions
		DecisionModels json.RawMessage `json:"decisionModels"`
	}{agentOptions: &options}
	if err := decode(raw, &envelope); err != nil {
		return agentOptions{}, err
	}
	if envelope.DecisionModels != nil {
		decoder := json.NewDecoder(bytes.NewReader(envelope.DecisionModels))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&options.DecisionModels); err != nil {
			return agentOptions{}, err
		}
		if err := sdk.ValidateAgentDecisionModels(Manifest(), options.DecisionModels); err != nil {
			return agentOptions{}, err
		}
	}
	return options, nil
}

func commandCodeModelMetadata(option agentModelOption) map[string]any {
	metadata := map[string]any{}
	if option.Context > 0 {
		metadata["contextWindow"] = option.Context
	}
	if option.MaxOutput > 0 {
		metadata["maxOutput"] = option.MaxOutput
	}
	if option.Efforts != nil {
		efforts := make([]string, 0, len(option.Efforts))
		for _, effort := range option.Efforts {
			if slices.Contains(commandCodeEfforts, effort) {
				efforts = append(efforts, commandCodeEffort(effort))
			}
		}
		metadata["reasoningEfforts"], metadata["reasoning"] = efforts, len(efforts) > 0
	}
	return metadata
}

func sameAgentMetadata(first, second any) bool {
	firstJSON, firstErr := json.Marshal(first)
	secondJSON, secondErr := json.Marshal(second)
	return firstErr == nil && secondErr == nil && string(firstJSON) == string(secondJSON)
}

func updateAgentModelMetadata(current, previous, next map[string]any) {
	for _, key := range []string{"contextWindow", "maxOutput", "reasoning", "reasoningEfforts"} {
		value, exists := current[key]
		prior, wasGenerated := previous[key]
		if exists && (!wasGenerated || !sameAgentMetadata(value, prior)) {
			continue
		}
		if !exists && wasGenerated {
			continue
		}
		restoreKey(current, next, key)
	}
}

func agentDescriptor() sdk.AgentDescriptor {
	return sdk.AgentDescriptor{ID: "commandcode-sdk", Name: "Command Code SDK", Bin: "command-code",
		Profile:     &sdk.AgentProfile{Target: "commandcode"},
		ConfigFiles: []string{".commandcode/settings.json", ".commandcode/providers.json"},
		Fields:      []sdk.AgentField{{Key: "model", Label: "model"}, {Key: "effort", Label: "effort"}},
		Launch:      &sdk.AgentLaunch{},
		Library:     &sdk.AgentLibrary{Instructions: ".commandcode/AGENTS.md", MCP: ".commandcode/mcp.json", MCPFormat: "commandcode", Skills: ".commandcode/skills"},
	}
}

func checkedAgentFiles(raw json.RawMessage) ([]settingFile, map[string]map[string]any, error) {
	var envelope struct {
		Files []settingFile `json:"files"`
	}
	if decode(raw, &envelope) != nil || len(envelope.Files) != 2 {
		return nil, nil, errors.New("agent requires both host-checked CommandCode settings snapshots")
	}
	objects := make(map[string]map[string]any)
	for _, file := range envelope.Files {
		if !slices.Contains(agentDescriptor().ConfigFiles, file.Path) || file.BeforeHash == "" || objects[file.Path] != nil {
			return nil, nil, errors.New("agent settings snapshot is missing, duplicate or foreign")
		}
		object := make(map[string]any)
		if file.Exists {
			if decode(file.Body, &object) != nil || object == nil {
				return nil, nil, errors.New("CommandCode settings must be JSON objects")
			}
		} else if len(file.Body) > 0 {
			return nil, nil, errors.New("absent settings have unexpected data")
		}
		objects[file.Path] = object
	}
	return envelope.Files, objects, nil
}

func objectAt(parent map[string]any, key string) (map[string]any, error) {
	value, found := parent[key]
	if !found {
		return make(map[string]any), nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("CommandCode settings contain an unexpected object shape")
	}
	return object, nil
}

func agentGateway(raw string) (string, error) {
	value, err := url.Parse(raw)
	if err != nil || value.Host == "" || value.User != nil || value.RawQuery != "" || value.Fragment != "" || value.Scheme != "http" && value.Scheme != "https" {
		return "", errors.New("agent gateway snapshot is invalid")
	}
	base := strings.TrimRight(raw, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base, nil
}

func agentInspect(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AgentInspectRequest
	if decode(call.Params, &request) != nil || request.ID != "commandcode-sdk" {
		return nil, invalid("invalid CommandCode agent inspection")
	}
	_, objects, err := checkedAgentFiles(request.Settings)
	if err != nil {
		return nil, invalid(err.Error())
	}
	options, err := checkedAgentOptions(request.Options)
	if err != nil {
		return nil, invalid("invalid agent options")
	}
	gateway, err := agentGateway(options.Gateway)
	if err != nil {
		return nil, invalid(err.Error())
	}
	settings := objects[".commandcode/settings.json"]
	root, err := objectAt(objects[".commandcode/providers.json"], "provider")
	if err != nil {
		return nil, invalid(err.Error())
	}
	entry, err := objectAt(root, "magpie")
	if err != nil {
		return nil, invalid(err.Error())
	}
	model, _ := settings["model"].(string)
	values := map[string]string{"model": model, "effort": ""}
	efforts, err := objectAt(settings, "reasoningEffort")
	if err != nil {
		return nil, invalid(err.Error())
	}
	values["effort"], _ = efforts[model].(string)
	values["effort"] = canonicalAgentEffort(values["effort"])
	connected := strings.HasPrefix(model, "magpie/") && settings["modelProvider"] == "magpie" && entry["baseURL"] == gateway
	return sdk.AgentInspectResult{Connected: connected, Values: values}, nil
}

func agentPlan(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AgentPlanRequest
	if decode(call.Params, &request) != nil || request.ID != "commandcode-sdk" {
		return nil, invalid("invalid CommandCode agent plan")
	}
	if request.Binding == nil {
		return nil, invalid("CommandCode agent changes require the host ownership snapshot")
	}
	if request.Action == "sync" && !request.Binding.Connected {
		return sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: &sdk.AgentBindingChange{Revision: request.Binding.Revision, Connected: false, Values: request.Binding.Values}}, nil
	}
	files, objects, err := checkedAgentFiles(request.Settings)
	if err != nil {
		return nil, invalid(err.Error())
	}
	options, err := checkedAgentOptions(request.Options)
	if err != nil {
		return nil, invalid("invalid CommandCode agent options")
	}
	for _, option := range options.Models {
		if option.Context < 0 || option.Context > 9007199254740991 ||
			option.MaxOutput > 0 && option.Context > 0 && option.MaxOutput > option.Context ||
			sdk.ValidateAgentModelMetadata(Manifest(), option.AgentModelMetadata) != nil {
			return nil, invalid("invalid effective CommandCode model metadata")
		}
	}
	priorModels := make(map[string]agentModelOption, len(options.PriorModels))
	for _, option := range options.PriorModels {
		priorModels[option.Value] = option
	}
	gateway, err := agentGateway(options.Gateway)
	if err != nil {
		return nil, invalid(err.Error())
	}
	settings := objects[".commandcode/settings.json"]
	providers := objects[".commandcode/providers.json"]
	root, err := objectAt(providers, "provider")
	if err != nil {
		return nil, invalid(err.Error())
	}
	entry, err := objectAt(root, "magpie")
	if err != nil {
		return nil, invalid(err.Error())
	}
	if existing, found := entry["baseURL"]; found && existing != gateway {
		return nil, &sdk.Error{Code: "conflict", Message: "The existing magpie provider belongs to another endpoint", Outcome: "not-started"}
	}
	model, _ := settings["model"].(string)
	efforts, err := objectAt(settings, "reasoningEffort")
	if err != nil {
		return nil, invalid(err.Error())
	}
	if request.Action == "disconnect" {
		if request.Binding == nil {
			return nil, invalid("disconnect requires a host ownership snapshot")
		}
		var originalSettings map[string]any
		var originalProvider map[string]any
		for _, file := range options.Previous {
			object := make(map[string]any)
			if file.Exists && (decode(file.Body, &object) != nil || object == nil) {
				return nil, invalid("original CommandCode snapshot is unreadable")
			}
			switch file.Path {
			case ".commandcode/settings.json":
				if originalSettings != nil {
					return nil, invalid("duplicate original settings")
				}
				originalSettings = object
			case ".commandcode/providers.json":
				if originalProvider != nil {
					return nil, invalid("duplicate original providers")
				}
				originalProvider = object
			default:
				return nil, invalid("foreign original settings")
			}
		}
		if originalSettings == nil || originalProvider == nil {
			return nil, invalid("disconnect requires original host snapshots")
		}
		if model == request.Binding.Values["model"] && settings["modelProvider"] == "magpie" {
			restoreKey(settings, originalSettings, "model")
			restoreKey(settings, originalSettings, "modelProvider")
		}
		previousEfforts, err := objectAt(originalSettings, "reasoningEffort")
		if err != nil {
			return nil, invalid(err.Error())
		}
		if current, _ := efforts[model].(string); canonicalAgentEffort(current) == request.Binding.Values["effort"] {
			restoreKey(efforts, previousEfforts, model)
		}
		oldRoot, err := objectAt(originalProvider, "provider")
		if err != nil {
			return nil, invalid(err.Error())
		}
		oldEntry, err := objectAt(oldRoot, "magpie")
		if err != nil {
			return nil, invalid(err.Error())
		}
		for _, key := range []string{"name", "api", "baseURL", "apiKey"} {
			restoreKey(entry, oldEntry, key)
		}
		currentModels, err := objectAt(entry, "models")
		if err != nil {
			return nil, invalid(err.Error())
		}
		oldModels, err := objectAt(oldEntry, "models")
		if err != nil {
			return nil, invalid(err.Error())
		}
		for _, option := range options.Models {
			if name, ok := strings.CutPrefix(option.Value, "magpie/"); ok {
				current, err := objectAt(currentModels, name)
				if err != nil {
					return nil, invalid(err.Error())
				}
				original, err := objectAt(oldModels, name)
				if err != nil {
					return nil, invalid(err.Error())
				}
				previous, found := priorModels[option.Value]
				if !found {
					previous = option
				}
				generated := commandCodeModelMetadata(previous)
				generated["name"] = previous.Label
				if previous.Label == "" {
					generated["name"] = name
				}
				for key, value := range generated {
					if sameAgentMetadata(current[key], value) {
						restoreKey(current, original, key)
					}
				}
				setObject(currentModels, name, current)
				if current, exists := efforts[option.Value]; exists && current == commandCodeEffort(previous.DefaultEffort) && previous.DefaultEffort != "" {
					restoreKey(efforts, previousEfforts, option.Value)
				}
			}
		}
		setObject(entry, "models", currentModels)
	} else {
		if request.Action != "connect" && request.Action != "sync" && request.Action != "set" {
			return nil, invalid("unsupported CommandCode agent action")
		}
		if options.Field != "" && options.Field != "model" && options.Field != "effort" {
			return nil, invalid("unknown CommandCode field")
		}
		if options.Field == "model" {
			model = options.Value
		}
		if !strings.HasPrefix(model, "magpie/") {
			if request.Action != "connect" || len(options.Models) == 0 {
				return nil, invalid("choose a host model before connecting CommandCode")
			}
			model = options.Models[0].Value
		}
		known := false
		models, err := objectAt(entry, "models")
		if err != nil {
			return nil, invalid(err.Error())
		}
		for _, option := range options.Models {
			name, ok := strings.CutPrefix(option.Value, "magpie/")
			if !ok || name == "" {
				continue
			}
			known = known || option.Value == model
			metadata, err := objectAt(models, name)
			if err != nil {
				return nil, invalid(err.Error())
			}
			label := option.Label
			if label == "" {
				label = name
			}
			metadata["name"] = label
			previous := commandCodeModelMetadata(priorModels[option.Value])
			next := commandCodeModelMetadata(option)
			updateAgentModelMetadata(metadata, previous, next)
			if option.Efforts != nil {
				metadata["reasoningEfforts"], metadata["reasoning"] = next["reasoningEfforts"], next["reasoning"]
				current, exists := efforts[option.Value]
				currentEffort, valid := current.(string)
				if exists && !valid {
					return nil, invalid("CommandCode per-model effort must be a string")
				}
				if exists && currentEffort != "" && !slices.Contains(option.Efforts, canonicalAgentEffort(currentEffort)) {
					delete(efforts, option.Value)
					exists = false
				}
				_, priorDefault := priorModels[option.Value]
				if !exists && !priorDefault && slices.Contains(commandCodeEfforts, option.DefaultEffort) {
					efforts[option.Value] = commandCodeEffort(option.DefaultEffort)
				}
			}
			models[name] = metadata
		}
		if !known {
			return nil, invalid("selected CommandCode model is outside the host catalog snapshot")
		}
		entry["name"], entry["api"], entry["baseURL"], entry["apiKey"], entry["models"] = "magpie", "openai-completions", gateway, false, models
		settings["model"], settings["modelProvider"] = model, "magpie"
		if options.Field == "effort" {
			selected := slices.IndexFunc(options.Models, func(option agentModelOption) bool { return option.Value == model })
			if options.Value != "" && (!slices.Contains(commandCodeEfforts, options.Value) ||
				selected >= 0 && options.Models[selected].Efforts != nil && !slices.Contains(options.Models[selected].Efforts, options.Value)) {
				return nil, invalid("unsupported CommandCode reasoning effort")
			}
			if options.Value == "" {
				delete(efforts, model)
			} else {
				efforts[model] = commandCodeEffort(options.Value)
			}
		}
	}
	setObject(settings, "reasoningEffort", efforts)
	setObject(root, "magpie", entry)
	setObject(providers, "provider", root)
	plan := sdk.AgentPlan{Files: make([]sdk.FileChange, 0, len(files))}
	for _, file := range files {
		body, err := json.MarshalIndent(objects[file.Path], "", "  ")
		if err != nil {
			return nil, err
		}
		body = append(body, '\n')
		var before any
		if file.Exists && json.Unmarshal(file.Body, &before) == nil && sameAgentMetadata(before, objects[file.Path]) {
			continue
		}
		plan.Files = append(plan.Files, sdk.FileChange{Path: file.Path, Body: body, BeforeHash: file.BeforeHash, Mode: 0600})
	}
	if request.Binding != nil {
		values := map[string]string{}
		if current, ok := settings["model"].(string); ok {
			values["model"] = current
			values["effort"], _ = efforts[current].(string)
			values["effort"] = canonicalAgentEffort(values["effort"])
		}
		plan.Binding = &sdk.AgentBindingChange{Revision: request.Binding.Revision, Connected: request.Action != "disconnect", Values: values}
	}
	return plan, nil
}

func restoreKey(target, original map[string]any, key string) {
	if value, found := original[key]; found {
		target[key] = value
	} else {
		delete(target, key)
	}
}

func setObject(parent map[string]any, key string, value map[string]any) {
	if len(value) == 0 {
		delete(parent, key)
	} else {
		parent[key] = value
	}
}
