package zcode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const AgentID = "zcode-native"
const agentConfigPath = ".zcode/v2/provider_config.json"

type agentSettingFile struct {
	Path       string `json:"path"`
	Body       []byte `json:"body"`
	BeforeHash string `json:"beforeHash"`
	Mode       uint32 `json:"mode"`
	Exists     bool   `json:"exists"`
}

type agentOptions struct {
	Gateway        string                   `json:"gateway"`
	Key            string                   `json:"key"`
	Models         []agentModelOption       `json:"models"`
	DecisionModels []sdk.AgentDecisionModel `json:"decisionModels,omitempty"`
	Field          string                   `json:"field,omitempty"`
	Value          string                   `json:"value,omitempty"`
	Previous       []agentSettingFile       `json:"previous,omitempty"`
	PriorModels    []agentModelOption       `json:"priorModels,omitempty"`
}

type agentModelOption struct {
	sdk.AgentModelMetadata
	Value     string   `json:"value"`
	Label     string   `json:"label,omitempty"`
	Note      string   `json:"note"`
	Icon      string   `json:"icon,omitempty"`
	Icons     []string `json:"icons,omitempty"`
	Group     string   `json:"group,omitempty"`
	GroupIcon string   `json:"groupIcon,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Free      bool     `json:"free,omitempty"`
	Zero      bool     `json:"zero,omitempty"`
	Rate      float64  `json:"rate,omitempty"`
	RateWas   float64  `json:"rateWas,omitempty"`
	Context   int64    `json:"context,omitempty"`
	Takes     string   `json:"takes,omitempty"`
	Direct    string   `json:"direct,omitempty"`
	Via       bool     `json:"via,omitempty"`
	Same      bool     `json:"same,omitempty"`
	Served    bool     `json:"served,omitempty"`
	Alias     string   `json:"alias,omitempty"`
	FastFor   string   `json:"fastFor,omitempty"`
	Fast      bool     `json:"fast,omitempty"`
	OwnFast   bool     `json:"ownFast,omitempty"`
	UltraOK   bool     `json:"ultraOk,omitempty"`
	Ultra     bool     `json:"ultra,omitempty"`
}

type agentDocument struct {
	root, config, providers, models map[string]any
}

func agentDescriptor() sdk.AgentDescriptor {
	return sdk.AgentDescriptor{
		ID: AgentID, Name: "ZCode native", Bin: "zcode",
		ConfigFiles: []string{agentConfigPath}, Fields: []sdk.AgentField{{Key: "model", Label: "Model"}},
		Profile: &sdk.AgentProfile{Target: "zcode"},
		Launch:  &sdk.AgentLaunch{Env: map[string]string{"ZCODE_PERSONAL_PROVIDER_CONFIG_FILE": "{config}"}},
	}
}

func agentBinding(call sdk.Call, id string, binding *sdk.AgentBindingSnapshot) error {
	if id != AgentID || call.Scope.Plugin != "" && call.Scope.Plugin != ProviderID ||
		call.Scope.Account != "" || call.Scope.Generation != 0 || len(call.Auth) != 0 ||
		binding == nil || sdk.ValidateAgentBindingSnapshot(ControlManifest(), agentDescriptor(), binding) != nil {
		return invalid("ZCode agent wiring requires its exact host-owned profile binding")
	}
	return nil
}

func readAgentSetting(files []agentSettingFile) (agentSettingFile, error) {
	if len(files) != 1 {
		return agentSettingFile{}, invalid("ZCode wiring requires exactly its declared provider configuration snapshot")
	}
	file := files[0]
	if file.Path != agentConfigPath || file.Mode&^0o777 != 0 {
		return file, invalid("ZCode wiring cannot modify another configuration file")
	}
	if !file.Exists {
		if len(file.Body) != 0 || file.Mode != 0 || file.BeforeHash != "missing" {
			return file, invalid("ZCode configuration has inconsistent missing-file preconditions")
		}
	} else {
		digest := sha256.Sum256(file.Body)
		if file.BeforeHash != hex.EncodeToString(digest[:]) {
			return file, invalid("ZCode configuration snapshot does not match its hash")
		}
	}
	return file, nil
}

func readAgentSettings(body json.RawMessage) (agentSettingFile, error) {
	var settings struct {
		Files []agentSettingFile `json:"files"`
	}
	if err := decode(body, &settings); err != nil {
		return agentSettingFile{}, err
	}
	return readAgentSetting(settings.Files)
}

func readAgentDocument(file agentSettingFile) (agentDocument, error) {
	if !file.Exists {
		file.Body = []byte(`{"schemaVersion":1,"config":{"providerConfigRules":{"providerRules":[]},"modelConfigRules":{"providerModelRules":[],"manualProviderModelRules":[]}}}`)
	}
	root, err := protocol.ReadObject(file.Body)
	if err != nil || root["schemaVersion"] != json.Number("1") || mapObject(root["config"]) == nil {
		return agentDocument{}, invalid("ZCode provider configuration must be an intact schemaVersion 1 object")
	}
	document := agentDocument{root: root, config: mapObject(root["config"])}
	document.providers, document.models = mapObject(document.config["providerConfigRules"]), mapObject(document.config["modelConfigRules"])
	modelIdentities := map[string]bool{}
	for _, group := range []struct {
		object map[string]any
		key    string
		model  bool
	}{
		{document.providers, "providerRules", false},
		{document.models, "providerModelRules", true},
		{document.models, "manualProviderModelRules", true},
	} {
		rules, valid := group.object[group.key].([]any)
		if !valid {
			return agentDocument{}, invalid("ZCode provider/model rule lists are missing or malformed")
		}
		seen := map[string]bool{}
		for _, entry := range rules {
			rule := mapObject(entry)
			provider, valid := rule["providerId"].(string)
			key := provider
			if group.model {
				model, valid := rule["modelId"].(string)
				if !valid || !safeText(model) {
					return agentDocument{}, invalid("ZCode model rule identity is malformed")
				}
				identity, _ := json.Marshal([]string{provider, model})
				key = string(identity)
				if modelIdentities[key] {
					return agentDocument{}, invalid("ZCode model has conflicting or duplicate rules")
				}
				modelIdentities[key] = true
			}
			if !valid || !safeText(provider) || mapObject(rule["config"]) == nil || seen[key] {
				return agentDocument{}, invalid("ZCode configuration contains malformed or duplicate rules")
			}
			seen[key] = true
		}
	}
	return document, nil
}

func agentProvider(document agentDocument) map[string]any {
	for _, entry := range document.providers["providerRules"].([]any) {
		rule := mapObject(entry)
		if rule["providerId"] == "magpie" {
			return rule
		}
	}
	return nil
}

func agentSelection(document agentDocument) string {
	selection := mapObject(document.config["defaultModelSelection"])
	model, _ := selection["modelId"].(string)
	if selection["providerId"] == "magpie" && safeText(model) {
		return "magpie/" + model
	}
	return ""
}

func (owner *module) agentInspect(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.AgentInspectRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if err := agentBinding(call, request.ID, request.Binding); err != nil {
		return nil, err
	}
	file, err := readAgentSettings(request.Settings)
	if err != nil {
		return nil, err
	}
	document, err := readAgentDocument(file)
	if err != nil {
		return nil, err
	}
	result := sdk.AgentInspectResult{Values: map[string]string{}}
	if !request.Binding.Connected {
		return result, nil
	}
	provider := agentProvider(document)
	config := mapObject(provider["config"])
	if provider == nil || provider["enabled"] == false || agentSelection(document) != request.Binding.Values["model"] ||
		!safeText(request.Binding.Values["model"]) || mapObject(config["access"])["type"] != "api-key" {
		return nil, invalid("Owned ZCode configuration changed; sync or select an offered model before launch")
	}
	result.Connected, result.Values["model"] = true, request.Binding.Values["model"]
	return result, nil
}

func readAgentOptions(body json.RawMessage) (agentOptions, error) {
	var options agentOptions
	object, err := protocol.ReadObject(body)
	if err != nil || decode(body, &options) != nil {
		return options, invalid("ZCode agent options are malformed")
	}
	if _, present := object["decisionModels"]; present &&
		sdk.ValidateAgentDecisionModels(ControlManifest(), options.DecisionModels) != nil {
		return options, invalid("ZCode agent received invalid decision model facts")
	}
	address, err := url.Parse(options.Gateway)
	if err != nil || address.Host == "" || address.Scheme != "http" && address.Scheme != "https" ||
		address.User != nil || address.RawQuery != "" || address.Fragment != "" || !safeText(options.Key) ||
		len(options.Models) > 4096 || len(options.PriorModels) > 4096 {
		return options, invalid("ZCode agent requires the host gateway and agent-scoped key")
	}
	seen := map[string]bool{}
	for _, model := range options.Models {
		if !strings.HasPrefix(model.Value, "magpie/") || !safeText(strings.TrimPrefix(model.Value, "magpie/")) ||
			seen[model.Value] || model.Context < 0 ||
			sdk.ValidateAgentModelMetadata(ControlManifest(), model.AgentModelMetadata) != nil {
			return options, invalid("ZCode agent received malformed or duplicate gateway model options")
		}
		seen[model.Value] = true
	}
	if _, present := object["priorModels"]; present {
		if options.PriorModels == nil {
			return options, invalid("ZCode agent previous catalog must be an explicit array")
		}
		seen := map[string]bool{}
		for _, model := range options.PriorModels {
			if !safeText(model.Value) || seen[model.Value] || model.Context < 0 ||
				sdk.ValidateAgentModelMetadata(ControlManifest(), model.AgentModelMetadata) != nil {
				return options, invalid("ZCode agent previous catalog contains malformed or duplicate model facts")
			}
			seen[model.Value] = true
		}
	}
	return options, nil
}

func (owner *module) agentPlan(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.AgentPlanRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if err := agentBinding(call, request.ID, request.Binding); err != nil {
		return nil, err
	}
	if !slices.Contains([]string{"connect", "set", "sync", "disconnect"}, request.Action) {
		return nil, invalid("Unsupported ZCode agent wiring action")
	}
	file, err := readAgentSettings(request.Settings)
	if err != nil {
		return nil, err
	}
	document, err := readAgentDocument(file)
	if err != nil {
		return nil, err
	}
	options, err := readAgentOptions(request.Options)
	if err != nil {
		return nil, err
	}
	binding := &sdk.AgentBindingChange{Revision: request.Binding.Revision, Connected: request.Binding.Connected, Values: map[string]string{}}
	plan := sdk.AgentPlan{Files: []sdk.FileChange{}, Binding: binding}
	if request.Action == "disconnect" || request.Action == "set" && options.Field == "model" && options.Value == "" {
		if !request.Binding.Connected {
			return plan, nil
		}
		previous, err := readAgentSetting(options.Previous)
		if err != nil {
			return nil, err
		}
		original, err := readAgentDocument(previous)
		if err != nil {
			return nil, err
		}
		document.providers["providerRules"] = restoreAgentRules(document.providers["providerRules"], original.providers["providerRules"])
		for _, key := range []string{"providerModelRules", "manualProviderModelRules"} {
			document.models[key] = restoreAgentRules(document.models[key], original.models[key])
		}
		if mapObject(document.config["defaultModelSelection"])["providerId"] == "magpie" {
			if value, present := original.config["defaultModelSelection"]; present {
				document.config["defaultModelSelection"] = value
			} else {
				delete(document.config, "defaultModelSelection")
			}
		}
		change := sdk.FileChange{Path: file.Path, BeforeHash: file.BeforeHash}
		if reflect.DeepEqual(document.root, original.root) {
			change.Body, change.Remove = previous.Body, !previous.Exists
		} else {
			body, err := json.Marshal(document.root)
			if err != nil {
				return nil, err
			}
			change.Body = append(body, '\n')
		}
		binding.Connected = false
		plan.Files = []sdk.FileChange{change}
		return plan, nil
	}
	if request.Action == "sync" && !request.Binding.Connected {
		return plan, nil
	}
	selected := request.Binding.Values["model"]
	if request.Action == "set" {
		if options.Field != "model" {
			return nil, invalid("ZCode agent has no such configurable field")
		}
		selected = options.Value
	}
	if selected == "" && len(options.Models) != 0 {
		selected = options.Models[0].Value
	}
	if !slices.ContainsFunc(options.Models, func(model agentModelOption) bool { return model.Value == selected }) {
		return nil, unsupported("Select an explicitly offered gateway model before wiring ZCode")
	}
	if err := configureAgentDocument(document, options); err != nil {
		return nil, err
	}
	if agentSelection(document) != selected {
		document.config["defaultModelSelection"] = map[string]any{"providerId": "magpie", "modelId": strings.TrimPrefix(selected, "magpie/")}
	}
	if err := configureAgentSelection(document, options); err != nil {
		return nil, err
	}
	binding.Connected, binding.Values["model"] = true, selected
	body, err := json.Marshal(document.root)
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	if !bytes.Equal(body, file.Body) {
		plan.Files = []sdk.FileChange{{Path: file.Path, Body: body, BeforeHash: file.BeforeHash, Mode: 0o600}}
	}
	return plan, nil
}

func restoreAgentRules(current, original any) []any {
	result := []any{}
	index := -1
	for _, entry := range current.([]any) {
		if mapObject(entry)["providerId"] == "magpie" {
			if index < 0 {
				index = len(result)
			}
		} else {
			result = append(result, entry)
		}
	}
	if index < 0 {
		index = len(result)
	}
	for _, entry := range original.([]any) {
		if mapObject(entry)["providerId"] == "magpie" {
			result = slices.Insert(result, index, entry)
			index++
		}
	}
	return result
}

func configureAgentDocument(document agentDocument, options agentOptions) error {
	provider := agentProvider(document)
	if provider == nil {
		provider = map[string]any{"providerId": "magpie", "providerName": "magpie", "enabled": true, "config": map[string]any{}}
		document.providers["providerRules"] = append(document.providers["providerRules"].([]any), provider)
	}
	config := mapObject(provider["config"])
	api := mapObject(config["api"])
	if api == nil {
		api = map[string]any{"type": "anthropic-messages"}
	}
	if !slices.Contains([]any{"anthropic-messages", "openai-chat-completions", "openai-responses"}, api["type"]) {
		return unsupported("ZCode provider uses an unsupported API format")
	}
	api["baseUrl"], config["api"], config["group"] = strings.TrimRight(options.Gateway, "/"), api, "standard-personal"
	access := mapObject(config["access"])
	if access == nil {
		access = map[string]any{}
	}
	if kind, present := access["type"]; present && kind != "api-key" {
		return unsupported("ZCode agent cannot replace an account-owned credential channel")
	}
	access["type"], access["apiKey"], config["access"] = "api-key", options.Key, access
	ids := []string{}
	for _, model := range options.Models {
		ids = append(ids, strings.TrimPrefix(model.Value, "magpie/"))
	}
	config["personalModelIds"], config["modelOrder"] = ids, ids
	manual := map[string]bool{}
	for _, entry := range document.models["manualProviderModelRules"].([]any) {
		rule := mapObject(entry)
		if rule["providerId"] == "magpie" {
			manual[rule["modelId"].(string)] = true
		}
	}
	old := map[string]map[string]any{}
	result := []any{}
	for _, entry := range document.models["providerModelRules"].([]any) {
		rule := mapObject(entry)
		if rule["providerId"] == "magpie" {
			old[rule["modelId"].(string)] = rule
		} else {
			result = append(result, entry)
		}
	}
	for _, model := range options.Models {
		id := strings.TrimPrefix(model.Value, "magpie/")
		if manual[id] {
			continue
		}
		rule := old[id]
		if rule == nil {
			rule = map[string]any{"providerId": "magpie", "modelId": id, "config": map[string]any{}}
		}
		prior := previousAgentModel(options, model.Value)
		configureAgentModel(mapObject(rule["config"]), model, prior)
		result = append(result, rule)
	}
	document.models["providerModelRules"] = result
	return nil
}

func previousAgentModel(options agentOptions, value string) agentModelOption {
	for _, model := range options.PriorModels {
		if model.Value == value {
			return model
		}
	}
	return agentModelOption{}
}

func agentEffort(value string) string {
	if value == "none" {
		return "disabled"
	}
	return value
}

func agentModelFields(model agentModelOption) (map[string]any, map[string]any) {
	properties, specs := map[string]any{}, map[string]any{}
	if model.Context > 0 {
		properties["contextWindow"] = model.Context
	}
	if model.MaxOutput > 0 {
		specs["maxOutputTokens"] = map[string]any{"max": model.MaxOutput}
	}
	if model.Efforts != nil {
		levels := []string{}
		for _, effort := range model.Efforts {
			levels = append(levels, agentEffort(effort))
		}
		if len(levels) == 0 {
			levels = append(levels, "disabled")
		}
		specs["reasoningLevel"] = map[string]any{"values": levels}
	}
	return properties, specs
}

func equalAgentValue(first, second any) bool {
	left, leftErr := json.Marshal(first)
	right, rightErr := json.Marshal(second)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func configureAgentModel(config map[string]any, model, prior agentModelOption) {
	properties, specs := agentModelFields(model)
	oldProperties, oldSpecs := agentModelFields(prior)
	for _, section := range []struct {
		name       string
		next, prev map[string]any
	}{
		{"properties", properties, oldProperties},
		{"optionSpecs", specs, oldSpecs},
	} {
		fields := mapObject(config[section.name])
		if _, present := config[section.name]; present && fields == nil {
			continue
		}
		if fields == nil {
			fields = map[string]any{}
		}
		for _, key := range []string{"contextWindow", "maxOutputTokens", "reasoningLevel"} {
			current, present := fields[key]
			old, generated := section.prev[key]
			if present && (!generated || !equalAgentValue(current, old)) {
				continue
			}
			if next, known := section.next[key]; known {
				fields[key] = next
			} else {
				delete(fields, key)
			}
		}
		if len(fields) == 0 {
			delete(config, section.name)
		} else {
			config[section.name] = fields
		}
	}
}

func defaultAgentEffort(model agentModelOption) string {
	if model.DefaultEffort != "" {
		return agentEffort(model.DefaultEffort)
	}
	if model.Efforts == nil {
		return ""
	}
	if slices.Contains(model.Efforts, "high") {
		return "high"
	}
	if len(model.Efforts) == 0 {
		return "disabled"
	}
	return agentEffort(model.Efforts[0])
}

func configureAgentSelection(document agentDocument, options agentOptions) error {
	selection := mapObject(document.config["defaultModelSelection"])
	value := "magpie/" + selection["modelId"].(string)
	model := agentModelOption{}
	for _, candidate := range options.Models {
		if candidate.Value == value {
			model = candidate
		}
	}
	prior := previousAgentModel(options, value)
	for _, key := range []string{"manualProviderModelRules", "providerModelRules"} {
		for _, entry := range document.models[key].([]any) {
			rule := mapObject(entry)
			if rule["providerId"] != "magpie" || rule["modelId"] != selection["modelId"] {
				continue
			}
			spec := mapObject(mapObject(mapObject(rule["config"])["optionSpecs"])["reasoningLevel"])
			var levels []string
			body, err := json.Marshal(spec["values"])
			if err != nil || json.Unmarshal(body, &levels) != nil || len(levels) == 0 {
				selected := mapObject(selection["options"])
				if level, present := selected["reasoningLevel"]; present && key == "providerModelRules" &&
					level == defaultAgentEffort(prior) && defaultAgentEffort(prior) != "" {
					delete(selected, "reasoningLevel")
					if len(selected) == 0 {
						delete(selection, "options")
					}
				}
				if len(selected) != 0 {
					return unsupported("Selected ZCode model options have no confirmed reasoning specification")
				}
				return nil
			}
			selected := mapObject(selection["options"])
			if selected == nil {
				selected = map[string]any{}
				selection["options"] = selected
			}
			if current, present := selected["reasoningLevel"]; present {
				level, valid := current.(string)
				if !valid || !slices.Contains(levels, level) {
					if key != "providerModelRules" || !valid || level == "" || level != defaultAgentEffort(prior) {
						return unsupported("Selected ZCode reasoning option is unavailable; choose the model again")
					}
					delete(selected, "reasoningLevel")
				}
			}
			if _, present := selected["reasoningLevel"]; !present {
				selected["reasoningLevel"] = levels[0]
				if slices.Contains(levels, "high") {
					selected["reasoningLevel"] = "high"
				}
				if preferred := defaultAgentEffort(model); slices.Contains(levels, preferred) {
					selected["reasoningLevel"] = preferred
				}
			}
			return nil
		}
	}
	return nil
}
