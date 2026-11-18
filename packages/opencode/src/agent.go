package opencode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const AgentID = "opencode-sdk"
const agentConfigPath = ".config/magpie/extensions/opencode/agent.json"

type agentFile struct {
	Path       string `json:"path"`
	Body       []byte `json:"body"`
	BeforeHash string `json:"beforeHash"`
	Exists     bool   `json:"exists"`
	Mode       uint32 `json:"mode"`
}

type agentOptions struct {
	Gateway        string                   `json:"gateway"`
	Key            string                   `json:"key"`
	Models         []json.RawMessage        `json:"models"`
	PriorModels    []json.RawMessage        `json:"priorModels,omitempty"`
	DecisionModels []sdk.AgentDecisionModel `json:"decisionModels,omitempty"`
	Field          string                   `json:"field"`
	Value          string                   `json:"value"`
	Previous       []agentFile              `json:"previous"`
	metadata       bool
}

type agentModelOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Context int64  `json:"context"`
	sdk.AgentModelMetadata
}

func checkedAgentOptions(body json.RawMessage) (agentOptions, error) {
	var options agentOptions
	envelope := struct {
		*agentOptions
		DecisionModels json.RawMessage `json:"decisionModels"`
	}{agentOptions: &options}
	if decode(body, &envelope) != nil {
		return options, agentFailure()
	}
	if envelope.DecisionModels != nil {
		options.metadata = true
		decoder := json.NewDecoder(bytes.NewReader(envelope.DecisionModels))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&options.DecisionModels) != nil ||
			sdk.ValidateAgentDecisionModels(agentMetadataManifest(), options.DecisionModels) != nil {
			return options, agentFailure()
		}
	}
	for _, models := range [][]json.RawMessage{options.Models, options.PriorModels} {
		if len(models) > 4096 {
			return options, agentFailure()
		}
		for _, raw := range models {
			if _, _, err := readAgentModelOption(raw); err != nil {
				return options, err
			}
		}
	}
	return options, nil
}

func readAgentModelOption(raw json.RawMessage) (agentModelOption, bool, error) {
	var value agentModelOption
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || json.Unmarshal(raw, &fields) != nil || fields == nil ||
		value.Context < 0 || value.Context > 9007199254740991 ||
		sdk.ValidateAgentModelMetadata(agentMetadataManifest(), value.AgentModelMetadata) != nil {
		return value, false, agentFailure()
	}
	metadata := fields["efforts"] != nil || fields["maxOutput"] != nil || fields["defaultEffort"] != nil
	return value, metadata, nil
}

func agentMetadataManifest() sdk.Manifest {
	return sdk.Manifest{Contract: sdk.Contract{Version: sdk.APIVersion{Major: 1, Minor: 9}, Capabilities: []sdk.Capability{sdk.AgentWiring}}}
}

type agentModelEntry struct {
	id     string
	value  map[string]any
	legacy bool
}

type agentModels []agentModelEntry

func (models agentModels) MarshalJSON() ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte('{')
	for index, model := range models {
		if index > 0 {
			output.WriteByte(',')
		}
		key, _ := json.Marshal(model.id)
		value, err := json.Marshal(model.value)
		if err != nil {
			return nil, err
		}
		output.Write(key)
		output.WriteByte(':')
		output.Write(value)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

func AgentManifest() sdk.Manifest {
	manifest := Manifest()
	manifest.Version = "0.3.0"
	manifest.Contract.Capabilities = append(manifest.Contract.Capabilities, sdk.AgentWiring)
	manifest.Agents = []sdk.AgentDescriptor{{
		ID: AgentID, Name: "OpenCode SDK", Bin: "opencode",
		Profile:     &sdk.AgentProfile{Target: "opencode"},
		ConfigFiles: []string{agentConfigPath},
		Fields:      []sdk.AgentField{{Key: "model", Label: "Model"}, {Key: "small", Label: "Small model"}},
		Launch: &sdk.AgentLaunch{
			Env: map[string]string{"OPENCODE_CONFIG": "{config}"}, Model: "--model",
			Plain: []string{"help", "--help", "-h", "version", "--version", "-v", "auth", "upgrade"},
		},
	}}
	return manifest
}

func NewAgentModule(options ModuleOptions) sdk.Module {
	module := NewModule(options)
	module.Manifest = AgentManifest()
	module.Handlers[sdk.MethodAgentInspect] = inspectAgent
	module.Handlers[sdk.MethodAgentPlan] = planAgent
	return module
}

func agentFailure() error {
	return &sdk.Error{Code: "invalid_params", Message: "OpenCode agent requires valid host-owned binding and config snapshots", Outcome: "not-started"}
}

func agentBinding(id string, binding *sdk.AgentBindingSnapshot) error {
	manifest := AgentManifest()
	if id != AgentID || binding == nil || sdk.ValidateAgentBindingSnapshot(manifest, manifest.Agents[0], binding) != nil {
		return agentFailure()
	}
	return nil
}

func readAgentFile(raw json.RawMessage) (agentFile, map[string]json.RawMessage, error) {
	var settings struct {
		Files []agentFile `json:"files"`
	}
	if json.Unmarshal(raw, &settings) != nil || len(settings.Files) != 1 || settings.Files[0].Path != agentConfigPath {
		return agentFile{}, nil, agentFailure()
	}
	file := settings.Files[0]
	values := map[string]json.RawMessage{}
	if file.Exists {
		hash := sha256.Sum256(file.Body)
		if file.BeforeHash != hex.EncodeToString(hash[:]) || json.Unmarshal(file.Body, &values) != nil || values == nil {
			return file, nil, agentFailure()
		}
	} else if len(file.Body) != 0 || file.BeforeHash != "missing" {
		return file, nil, agentFailure()
	}
	return file, values, nil
}

func inspectAgent(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var request sdk.AgentInspectRequest
	if decode(call.Params, &request) != nil || agentBinding(request.ID, request.Binding) != nil {
		return nil, agentFailure()
	}
	file, values, err := readAgentFile(request.Settings)
	if err != nil {
		return nil, err
	}
	if request.Binding.Connected && !file.Exists {
		return nil, agentFailure()
	}
	if request.Binding.Connected {
		options, err := checkedAgentOptions(request.Options)
		if err != nil {
			return nil, agentFailure()
		}
		model, _ := json.Marshal(request.Binding.Values["model"])
		if !bytes.Equal(values["model"], model) {
			return nil, agentFailure()
		}
		if small, owned := request.Binding.Values["small"]; owned {
			actual := values["small_model"]
			if agentFormat(ctx) != "v1" {
				var agents, title map[string]json.RawMessage
				json.Unmarshal(values["agents"], &agents)
				json.Unmarshal(agents["title"], &title)
				actual = title["model"]
			}
			expected, _ := json.Marshal(small)
			if small == "" && len(actual) != 0 || small != "" && !bytes.Equal(actual, expected) {
				return nil, agentFailure()
			}
		}
		providerKey, optionKey := "providers", "settings"
		if agentFormat(ctx) == "v1" {
			providerKey, optionKey = "provider", "options"
		}
		var providers, block map[string]json.RawMessage
		var settings struct {
			BaseURL string `json:"baseURL"`
			Key     string `json:"apiKey"`
		}
		if json.Unmarshal(values[providerKey], &providers) != nil ||
			json.Unmarshal(providers["magpie"], &block) != nil || json.Unmarshal(block[optionKey], &settings) != nil ||
			settings.BaseURL != strings.TrimRight(options.Gateway, "/")+"/v1" || settings.Key != options.Key || options.Key == "" {
			return nil, agentFailure()
		}
	}
	return sdk.AgentInspectResult{Connected: request.Binding.Connected, Values: request.Binding.Clone().Values}, nil
}

func agentCatalog(options agentOptions) (map[string]any, map[string]bool, error) {
	address, err := url.Parse(options.Gateway)
	if err != nil || address.Scheme != "http" && address.Scheme != "https" || address.Host == "" ||
		address.User != nil || address.RawQuery != "" || address.Fragment != "" || address.Path != "" && address.Path != "/" ||
		options.Key == "" || strings.ContainsAny(options.Key, "\r\n\x00") || len(options.Models) > 4096 {
		return nil, nil, agentFailure()
	}
	models := agentModels{}
	allowed := map[string]bool{}
	providerHash := sha256.Sum256([]byte(Manifest().ID + "\x00" + ProviderGo))
	ownPrefix := "extension-" + hex.EncodeToString(providerHash[:12]) + "/"
	for _, raw := range options.Models {
		value, metadata, err := readAgentModelOption(raw)
		metadata = metadata || options.metadata
		if err != nil {
			return nil, nil, err
		}
		if !strings.HasPrefix(value.Value, "magpie/") {
			continue
		}
		id := strings.TrimPrefix(value.Value, "magpie/")
		if id == "" || strings.ContainsAny(id, "\r\n\x00") || allowed[value.Value] {
			return nil, nil, agentFailure()
		}
		allowed[value.Value] = true
		model := map[string]any{"name": value.Label}
		if value.Label == "" {
			model["name"] = id
		}
		limit := map[string]int64{}
		if value.Context > 0 {
			limit["context"] = value.Context
		}
		if value.MaxOutput > 0 {
			limit["output"] = value.MaxOutput
		}
		efforts := value.Efforts
		for _, known := range Models() {
			if id != ownPrefix+known.ID {
				continue
			}
			if !metadata {
				if value.Context == 0 {
					limit["context"] = known.ContextWindow
				}
				limit["output"] = known.MaxOutput
				efforts = known.Efforts
			}
			model["capabilities"] = map[string]any{"tools": known.Tools, "input": known.InputModalities, "output": known.OutputModalities}
		}
		if len(limit) > 0 {
			model["limit"] = limit
		}
		if efforts != nil {
			variants := []any{}
			for _, effort := range efforts {
				variants = append(variants, map[string]any{"id": effort, "settings": map[string]any{"reasoningEffort": effort}})
			}
			model["variants"] = variants
		}
		if value.DefaultEffort != "" {
			model["settings"] = map[string]any{"reasoningEffort": value.DefaultEffort}
		}
		models = append(models, agentModelEntry{id: id, value: model, legacy: !metadata})
	}
	return map[string]any{"name": "Magpie", "package": "aisdk:@ai-sdk/openai-compatible",
		"settings":      map[string]any{"baseURL": strings.TrimRight(options.Gateway, "/") + "/v1", "apiKey": options.Key},
		"compatibility": map[string]any{"supportsPromptCacheKey": true}, "models": models}, allowed, nil
}

func agentFormattedCatalog(ctx context.Context, options agentOptions) (map[string]any, map[string]bool, error) {
	block, allowed, err := agentCatalog(options)
	if err != nil || agentFormat(ctx) != "v1" {
		return block, allowed, err
	}
	for _, model := range block["models"].(agentModels) {
		if capabilities, present := model.value["capabilities"].(map[string]any); present {
			model.value["modalities"] = map[string]any{"input": capabilities["input"], "output": capabilities["output"]}
			delete(model.value, "capabilities")
		}
		if variants, present := model.value["variants"].([]any); present {
			model.value["reasoning"] = len(variants) > 0
			legacy := agentModels{}
			for _, variant := range variants {
				item := variant.(map[string]any)
				legacy = append(legacy, agentModelEntry{id: item["id"].(string), value: item["settings"].(map[string]any)})
			}
			model.value["variants"] = legacy
		}
		if settings, present := model.value["settings"]; present {
			model.value["options"] = settings
			delete(model.value, "settings")
		}
	}
	return map[string]any{"name": block["name"], "npm": "@ai-sdk/openai-compatible",
		"options": map[string]any{"baseURL": strings.TrimRight(options.Gateway, "/") + "/v1", "apiKey": options.Key, "setCacheKey": true},
		"models":  block["models"]}, allowed, nil
}

func agentFormat(ctx context.Context) string {
	format := "v2"
	if current, found := sdk.ModuleContextFrom(ctx); found {
		var config moduleConfig
		if json.Unmarshal(current.Config.Values, &config) == nil && config.AgentFormat != "" {
			format = config.AgentFormat
		}
	}
	return format
}

func restoreAgentField(values, previous map[string]json.RawMessage, path ...string) error {
	key := path[0]
	if len(path) == 1 {
		if value, exists := previous[key]; exists {
			values[key] = value
		} else {
			delete(values, key)
		}
		return nil
	}
	currentChild, beforeChild := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if value, exists := values[key]; exists && (json.Unmarshal(value, &currentChild) != nil || currentChild == nil) {
		return agentFailure()
	}
	if value, exists := previous[key]; exists && (json.Unmarshal(value, &beforeChild) != nil || beforeChild == nil) {
		return agentFailure()
	}
	if err := restoreAgentField(currentChild, beforeChild, path[1:]...); err != nil {
		return err
	}
	if len(currentChild) == 0 {
		delete(values, key)
	} else {
		values[key], _ = json.Marshal(currentChild)
	}
	return nil
}

func restoreAgentGenerated(current, previous map[string]json.RawMessage, generated map[string]any) error {
	for key, fresh := range generated {
		have, found := current[key]
		if !found {
			continue
		}
		freshJSON, err := json.Marshal(fresh)
		if err != nil {
			return err
		}
		var haveObject, freshObject map[string]json.RawMessage
		if json.Unmarshal(have, &haveObject) == nil && haveObject != nil &&
			json.Unmarshal(freshJSON, &freshObject) == nil && freshObject != nil {
			oldObject := map[string]json.RawMessage{}
			if raw, exists := previous[key]; exists && (json.Unmarshal(raw, &oldObject) != nil || oldObject == nil) {
				return agentFailure()
			}
			nested := map[string]any{}
			for name, value := range freshObject {
				nested[name] = value
			}
			if err := restoreAgentGenerated(haveObject, oldObject, nested); err != nil {
				return err
			}
			if len(haveObject) == 0 {
				delete(current, key)
			} else {
				current[key], _ = json.Marshal(haveObject)
			}
		} else {
			var compactHave, compactFresh bytes.Buffer
			if json.Compact(&compactHave, have) != nil || json.Compact(&compactFresh, freshJSON) != nil {
				return agentFailure()
			}
			if !bytes.Equal(compactHave.Bytes(), compactFresh.Bytes()) {
				continue
			}
			if old, existed := previous[key]; existed {
				current[key] = old
			} else {
				delete(current, key)
			}
		}
	}
	return nil
}

func keepAgentObject(before json.RawMessage, fields map[string]any) (json.RawMessage, error) {
	values := map[string]json.RawMessage{}
	if len(before) > 0 && (json.Unmarshal(before, &values) != nil || values == nil) {
		return nil, agentFailure()
	}
	for key, field := range fields {
		value, err := json.Marshal(field)
		if err != nil {
			return nil, err
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) == nil && object != nil {
			nested := map[string]any{}
			for name, part := range object {
				nested[name] = part
			}
			value, err = keepAgentObject(values[key], nested)
			if err != nil {
				return nil, err
			}
		}
		values[key] = value
	}
	return json.Marshal(values)
}

func keepAgentVariants(before json.RawMessage, next []any, legacy bool) ([]any, error) {
	if len(before) == 0 {
		return next, nil
	}
	var previous []json.RawMessage
	if json.Unmarshal(before, &previous) != nil {
		return nil, agentFailure()
	}
	indices := map[string]int{}
	for index, variant := range next {
		indices[variant.(map[string]any)["id"].(string)] = index
	}
	seen := map[string]bool{}
	for _, variant := range previous {
		var entry struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(variant, &entry) != nil || entry.ID == "" || seen[entry.ID] {
			return nil, agentFailure()
		}
		seen[entry.ID] = true
		var fields, settings map[string]json.RawMessage
		json.Unmarshal(variant, &fields)
		json.Unmarshal(fields["settings"], &settings)
		if legacy && len(fields) == 2 && ownAgentEffort(entry.ID, settings) {
			continue
		}
		if index, found := indices[entry.ID]; found {
			next[index] = variant
		} else {
			next = append(next, variant)
		}
	}
	return next, nil
}

func ownAgentEffort(id string, fields map[string]json.RawMessage) bool {
	var effort string
	if len(fields) != 1 || json.Unmarshal(fields["reasoningEffort"], &effort) != nil || effort != id {
		return false
	}
	for _, model := range Models() {
		for _, known := range model.Efforts {
			if id == known {
				return true
			}
		}
	}
	return false
}

func keepAgentLegacyVariants(before, generated json.RawMessage, legacy bool) (json.RawMessage, error) {
	next := map[string]json.RawMessage{}
	previous := map[string]json.RawMessage{}
	if json.Unmarshal(generated, &next) != nil || next == nil ||
		len(before) != 0 && (json.Unmarshal(before, &previous) != nil || previous == nil) {
		return nil, agentFailure()
	}
	for id, value := range previous {
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil || fields == nil {
			return nil, agentFailure()
		}
		if !legacy || !ownAgentEffort(id, fields) {
			next[id] = value
		}
	}
	return json.Marshal(next)
}

func sameAgentJSON(first, second json.RawMessage) bool {
	values := [2]any{}
	for index, raw := range []json.RawMessage{first, second} {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&values[index]) != nil {
			return false
		}
	}
	return reflect.DeepEqual(values[0], values[1])
}

func trimAgentPriorVariants(current map[string]json.RawMessage, prior map[string]any) error {
	generated, found := prior["variants"]
	if !found || len(current["variants"]) == 0 {
		return nil
	}
	if variants, native := generated.([]any); native {
		var previous []json.RawMessage
		if json.Unmarshal(current["variants"], &previous) != nil {
			return agentFailure()
		}
		retained := []json.RawMessage{}
		for _, variant := range previous {
			owned := false
			for _, candidate := range variants {
				expected, _ := json.Marshal(candidate)
				owned = owned || sameAgentJSON(variant, expected)
			}
			if !owned {
				retained = append(retained, variant)
			}
		}
		if len(retained) == 0 {
			delete(current, "variants")
		} else {
			current["variants"], _ = json.Marshal(retained)
		}
		return nil
	}
	var previous, expected map[string]json.RawMessage
	body, _ := json.Marshal(generated)
	if json.Unmarshal(current["variants"], &previous) != nil || previous == nil || json.Unmarshal(body, &expected) != nil {
		return agentFailure()
	}
	for id, variant := range expected {
		if sameAgentJSON(previous[id], variant) {
			delete(previous, id)
		}
	}
	if len(previous) == 0 {
		delete(current, "variants")
	} else {
		current["variants"], _ = json.Marshal(previous)
	}
	return nil
}

func keepAgentProvider(previous json.RawMessage, next map[string]any, prior agentModels) (map[string]json.RawMessage, error) {
	before := map[string]json.RawMessage{}
	if len(previous) != 0 && (json.Unmarshal(previous, &before) != nil || before == nil) {
		return nil, agentFailure()
	}
	for key, value := range next {
		if key == "models" {
			oldModels := map[string]json.RawMessage{}
			if raw, exists := before[key]; exists && (json.Unmarshal(raw, &oldModels) != nil || oldModels == nil) {
				return nil, agentFailure()
			}
			models := value.(agentModels)
			for index := range models {
				model := &models[index]
				old := map[string]json.RawMessage{}
				if raw, exists := oldModels[model.id]; exists && (json.Unmarshal(raw, &old) != nil || old == nil) {
					return nil, agentFailure()
				}
				for _, candidate := range prior {
					if candidate.id != model.id {
						continue
					}
					generated := map[string]any{}
					for name, field := range candidate.value {
						if name != "variants" {
							generated[name] = field
						}
					}
					if restoreAgentGenerated(old, map[string]json.RawMessage{}, generated) != nil ||
						trimAgentPriorVariants(old, candidate.value) != nil {
						return nil, agentFailure()
					}
				}
				optionKey := "settings"
				if _, legacy := next["npm"]; legacy {
					optionKey = "options"
				}
				var settings map[string]json.RawMessage
				if body, exists := old[optionKey]; exists && (json.Unmarshal(body, &settings) != nil || settings == nil) {
					return nil, agentFailure()
				}
				effort := settings["reasoningEffort"]
				var output json.RawMessage
				if raw, exists := old["limit"]; exists {
					var limit map[string]json.RawMessage
					if json.Unmarshal(raw, &limit) != nil || limit == nil {
						return nil, agentFailure()
					}
					output = limit["output"]
					if len(output) != 0 {
						var tokens int64
						if json.Unmarshal(output, &tokens) != nil || tokens <= 0 {
							return nil, agentFailure()
						}
					}
				}
				for name, field := range model.value {
					generated, err := json.Marshal(field)
					if err != nil {
						return nil, err
					}
					var object map[string]any
					if name != "variants" && json.Unmarshal(generated, &object) == nil && object != nil {
						old[name], err = keepAgentObject(old[name], object)
					} else if name == "variants" {
						if variants, ok := field.([]any); ok {
							field, err = keepAgentVariants(old[name], variants, model.legacy && prior == nil)
							if err == nil {
								old[name], err = json.Marshal(field)
							}
						} else {
							old[name], err = keepAgentLegacyVariants(old[name], generated, model.legacy && prior == nil)
						}
					} else {
						old[name], err = json.Marshal(field)
					}
					if err != nil {
						return nil, err
					}
				}
				if len(effort) != 0 {
					json.Unmarshal(old[optionKey], &settings)
					if settings == nil {
						settings = map[string]json.RawMessage{}
					}
					settings["reasoningEffort"] = effort
					old[optionKey], _ = json.Marshal(settings)
				}
				if len(output) != 0 {
					var limit map[string]json.RawMessage
					json.Unmarshal(old["limit"], &limit)
					if limit == nil {
						limit = map[string]json.RawMessage{}
					}
					limit["output"] = output
					old["limit"], _ = json.Marshal(limit)
				}
				model.value = map[string]any{}
				for name, field := range old {
					model.value[name] = field
				}
			}
			before[key], _ = json.Marshal(value)
		} else if key == "settings" || key == "options" || key == "compatibility" {
			options := map[string]json.RawMessage{}
			if raw, exists := before[key]; exists && (json.Unmarshal(raw, &options) != nil || options == nil) {
				return nil, agentFailure()
			}
			for name, field := range value.(map[string]any) {
				if key == "compatibility" && name == "supportsPromptCacheKey" && options[name] != nil {
					continue
				}
				options[name], _ = json.Marshal(field)
			}
			before[key], _ = json.Marshal(options)
		} else {
			before[key], _ = json.Marshal(value)
		}
	}
	return before, nil
}

func planAgent(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var request sdk.AgentPlanRequest
	if decode(call.Params, &request) != nil || agentBinding(request.ID, request.Binding) != nil {
		return nil, agentFailure()
	}
	file, values, err := readAgentFile(request.Settings)
	if err != nil {
		return nil, err
	}
	change := &sdk.AgentBindingChange{Revision: request.Binding.Revision, Connected: request.Binding.Connected,
		Values: request.Binding.Clone().Values}
	if request.Action == "disconnect" {
		options, err := checkedAgentOptions(request.Options)
		if err != nil || len(options.Previous) != 1 || options.Previous[0].Path != file.Path {
			return nil, agentFailure()
		}
		previous := options.Previous[0]
		previousValues := map[string]json.RawMessage{}
		if previous.Exists && (json.Unmarshal(previous.Body, &previousValues) != nil || previousValues == nil) {
			return nil, agentFailure()
		}
		if request.Binding.Connected {
			_, err := inspectAgent(ctx, sdk.Call{Params: rawAgentInspect(request)}, nil)
			if err != nil {
				return nil, err
			}
		}
		paths := [][]string{{"model"}}
		for _, providerKey := range []string{"provider", "providers"} {
			optionKey := "options"
			if providerKey == "providers" {
				optionKey = "settings"
			}
			for _, field := range []string{"name", "package", "npm"} {
				paths = append(paths, []string{providerKey, "magpie", field})
			}
			for _, field := range []string{"baseURL", "apiKey", "setCacheKey"} {
				paths = append(paths, []string{providerKey, "magpie", optionKey, field})
			}
		}
		for _, providerKey := range []string{"provider", "providers"} {
			var currentProviders, oldProviders, currentBlock, oldBlock map[string]json.RawMessage
			json.Unmarshal(values[providerKey], &currentProviders)
			json.Unmarshal(previousValues[providerKey], &oldProviders)
			json.Unmarshal(currentProviders["magpie"], &currentBlock)
			json.Unmarshal(oldProviders["magpie"], &oldBlock)
			if currentBlock == nil {
				continue
			}
			if options.PriorModels != nil {
				options.Models = options.PriorModels
			}
			fresh, _, err := agentFormattedCatalog(ctx, options)
			if err != nil {
				return nil, err
			}
			if err := restoreAgentGenerated(currentBlock, oldBlock, map[string]any{"models": fresh["models"],
				"compatibility": fresh["compatibility"]}); err != nil {
				return nil, err
			}
			currentProviders["magpie"], _ = json.Marshal(currentBlock)
			values[providerKey], _ = json.Marshal(currentProviders)
		}
		if _, owned := request.Binding.Values["small"]; owned {
			paths = append(paths, []string{"small_model"}, []string{"agents", "title", "model"})
		}
		for _, path := range paths {
			if err := restoreAgentField(values, previousValues, path...); err != nil {
				return nil, err
			}
		}
		change.Connected = false
	} else {
		options, err := checkedAgentOptions(request.Options)
		if err != nil {
			return nil, agentFailure()
		}
		block, allowed, err := agentFormattedCatalog(ctx, options)
		if err != nil {
			return nil, err
		}
		switch request.Action {
		case "set":
			if options.Field != "model" && options.Field != "small" || options.Value != "" && !allowed[options.Value] {
				return nil, agentFailure()
			}
			if options.Value == "" && options.Field != "small" {
				delete(change.Values, options.Field)
			} else {
				change.Values[options.Field] = options.Value
			}
			change.Connected = true
		case "connect":
			change.Connected = true
		case "sync":
		default:
			return nil, agentFailure()
		}
		if !allowed[change.Values["model"]] || change.Values["small"] != "" && !allowed[change.Values["small"]] {
			return nil, agentFailure()
		}
		providerKey := "providers"
		if agentFormat(ctx) == "v1" {
			providerKey = "provider"
		}
		var prior agentModels
		if options.PriorModels != nil {
			before := options
			before.Models = options.PriorModels
			catalog, _, err := agentFormattedCatalog(ctx, before)
			if err != nil {
				return nil, err
			}
			prior = catalog["models"].(agentModels)
		}
		providers := map[string]json.RawMessage{}
		if value, exists := values[providerKey]; exists && (json.Unmarshal(value, &providers) != nil || providers == nil) {
			return nil, agentFailure()
		}
		kept, err := keepAgentProvider(providers["magpie"], block, prior)
		if err != nil {
			return nil, err
		}
		providers["magpie"], _ = json.Marshal(kept)
		values[providerKey], _ = json.Marshal(providers)
		values["model"], _ = json.Marshal(change.Values["model"])
		_, ownsSmall := change.Values["small"]
		if ownsSmall {
			if agentFormat(ctx) == "v1" {
				if change.Values["small"] == "" {
					delete(values, "small_model")
				} else {
					values["small_model"], _ = json.Marshal(change.Values["small"])
				}
			} else {
				agents := map[string]json.RawMessage{}
				if value, exists := values["agents"]; exists && (json.Unmarshal(value, &agents) != nil || agents == nil) {
					return nil, agentFailure()
				}
				title := map[string]json.RawMessage{}
				if value, exists := agents["title"]; exists && (json.Unmarshal(value, &title) != nil || title == nil) {
					return nil, agentFailure()
				}
				if change.Values["small"] != "" {
					title["model"], _ = json.Marshal(change.Values["small"])
				} else {
					delete(title, "model")
				}
				if len(title) > 0 {
					agents["title"], _ = json.Marshal(title)
				} else {
					delete(agents, "title")
				}
				if len(agents) > 0 {
					values["agents"], _ = json.Marshal(agents)
				} else {
					delete(values, "agents")
				}
			}
		}
	}
	if sdk.ValidateAgentBindingChange(AgentManifest(), AgentManifest().Agents[0], request.Binding, request.Action, change) != nil {
		return nil, agentFailure()
	}
	body, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	if bytes.Equal(file.Body, body) {
		return sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: change}, nil
	}
	if request.Action == "disconnect" && len(values) == 0 {
		return sdk.AgentPlan{Files: []sdk.FileChange{{Path: file.Path, BeforeHash: file.BeforeHash, Remove: true}}, Binding: change}, nil
	}
	if len(body) > sdk.MaxHostBytes {
		return nil, errors.New("OpenCode agent config exceeds its checked input budget")
	}
	return sdk.AgentPlan{Files: []sdk.FileChange{{Path: file.Path, BeforeHash: file.BeforeHash, Body: body, Mode: 0o600}}, Binding: change}, nil
}

func rawAgentInspect(request sdk.AgentPlanRequest) json.RawMessage {
	body, _ := json.Marshal(sdk.AgentInspectRequest{ID: request.ID, Options: request.Options, Settings: request.Settings, Binding: request.Binding})
	return body
}
