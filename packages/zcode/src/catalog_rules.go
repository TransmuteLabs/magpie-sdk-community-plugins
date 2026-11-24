package zcode

import (
	"math"
	"regexp"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
	codec "github.com/yetone/magpie/pkg/protocolext"
)

func mapObject(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func modelLimit(value any) (int64, error) {
	if value == nil {
		return 0, nil
	}
	number, valid := finiteNumber(value)
	if !valid || number < 0 || number >= float64(math.MaxInt64) || number != math.Trunc(number) {
		return 0, protocol.ErrInvalid
	}
	return int64(number), nil
}

func matchRule(pattern any, value string, insensitive bool) (bool, error) {
	text, valid := pattern.(string)
	if !valid || len(text) > 16384 {
		return false, protocol.ErrInvalid
	}
	if insensitive && text == `.*glm-4\.6(?!v).*` {
		text = `[^\r\n\x{2028}\x{2029}]*glm-4\.6(?:[^v\r\n\x{2028}\x{2029}][^\r\n\x{2028}\x{2029}]*)?`
	}
	expression := "^(?:" + text + ")$"
	if insensitive {
		expression = "(?i)" + expression
	}
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return false, unsupported("Provider configuration requires unsupported or malformed regexp semantics")
	}
	return compiled.MatchString(value), nil
}

func applyModelRule(model *sdk.Model, enabled *bool, config map[string]any) error {
	if value, present := config["enabled"]; present {
		active, valid := value.(bool)
		if !valid {
			if value != nil {
				return protocol.ErrInvalid
			}
			active = false
		}
		*enabled = active
	}
	if properties, present := config["properties"]; present {
		if properties == nil {
			model.ContextWindow, model.Vision, model.InputModalities = 0, false, nil
		} else {
			values, valid := properties.(map[string]any)
			if !valid {
				return protocol.ErrInvalid
			}
			if context, present := values["contextWindow"]; present {
				window, err := modelLimit(context)
				if err != nil {
					return err
				}
				model.ContextWindow = window
			}
			if format, present := values["inputFormat"]; present {
				if format == nil {
					model.Vision, model.InputModalities = false, nil
				} else {
					input, valid := format.(map[string]any)
					if !valid {
						return protocol.ErrInvalid
					}
					known := map[string]bool{}
					for _, modality := range model.InputModalities {
						known[modality] = true
					}
					for _, entry := range []struct{ key, value string }{
						{"supportsImage", "image"}, {"supportsVideo", "video"}, {"supportsPdf", "pdf"}, {"supportsAudio", "audio"},
					} {
						if value, present := input[entry.key]; present {
							active, valid := value.(bool)
							if !valid {
								return protocol.ErrInvalid
							}
							known[entry.value] = active
						}
					}
					model.InputModalities = []string{"text"}
					for _, modality := range []string{"image", "video", "pdf", "audio"} {
						if known[modality] {
							model.InputModalities = append(model.InputModalities, modality)
						}
					}
					model.Vision = known["image"]
				}
			}
		}
	}
	if options, present := config["optionSpecs"]; present {
		if options == nil {
			model.MaxOutput, model.Efforts, model.Reasoning = 0, nil, false
		} else {
			values, valid := options.(map[string]any)
			if !valid {
				return protocol.ErrInvalid
			}
			if maximum, present := values["maxOutputTokens"]; present {
				maximumValue := mapObject(maximum)
				if maximum != nil && maximumValue == nil {
					return protocol.ErrInvalid
				}
				if maximum == nil {
					model.MaxOutput = 0
				} else if value, present := maximumValue["max"]; present {
					limit, err := modelLimit(value)
					if err != nil {
						return err
					}
					model.MaxOutput = limit
				}
			}
			if reasoning, present := values["reasoningLevel"]; present {
				value := mapObject(reasoning)
				if reasoning != nil && value == nil {
					return protocol.ErrInvalid
				}
				options, present := value["values"]
				if reasoning == nil || present && options == nil {
					model.Efforts, model.Reasoning = nil, false
				} else if present {
					options, valid := options.([]any)
					if !valid {
						return protocol.ErrInvalid
					}
					model.Efforts, model.Reasoning = nil, false
					seen := map[string]bool{}
					for _, option := range options {
						effort, valid := option.(string)
						if !valid {
							return protocol.ErrInvalid
						}
						if effort == "enabled" {
							effort = "high"
						} else if effort == "disabled" {
							effort = "none"
						}
						switch effort {
						case "none", "minimal", "low", "medium", "high", "xhigh", "max":
						default:
							return unsupported("Provider reasoning option cannot be represented by this SDK contract")
						}
						if !seen[effort] {
							model.Efforts = append(model.Efforts, effort)
							model.Reasoning = model.Reasoning || effort != "none"
							seen[effort] = true
						}
					}
				}
			}
		}
	}
	return nil
}

func configuredModels(body []byte, providerID, endpoint string) ([]sdk.Model, error) {
	root, err := protocol.ReadObject(body)
	if err != nil {
		return nil, err
	}
	config := mapObject(root["config"])
	providers := mapObject(config["providerConfigRules"])
	rules := mapObject(config["modelConfigRules"])
	providerRules, valid := providers["providerRules"].([]any)
	if !valid || rules == nil {
		return nil, protocol.ErrInvalid
	}
	var provider map[string]any
	for _, item := range providerRules {
		value := mapObject(item)
		if value == nil {
			return nil, protocol.ErrInvalid
		}
		if value["providerId"] == providerID {
			if provider != nil {
				return nil, protocol.ErrInvalid
			}
			provider = value
		}
	}
	if provider == nil {
		return nil, unsupported("Provider configuration does not contain the selected account plan")
	}
	template, _ := provider["templateId"].(string)
	if provider["templateId"] != nil && template == "" {
		return nil, protocol.ErrInvalid
	}
	definition := mapObject(provider["config"])
	if definition == nil {
		return nil, protocol.ErrInvalid
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, field := range []string{"builtinModelIds", "personalModelIds"} {
		value, present := definition[field]
		if !present || value == nil {
			continue
		}
		items, valid := value.([]any)
		if !valid || len(items) > 4096 {
			return nil, protocol.ErrInvalid
		}
		for _, item := range items {
			id, valid := item.(string)
			if !valid || !safeText(id) {
				return nil, protocol.ErrInvalid
			}
			if !seen[strings.ToLower(id)] {
				ids = append(ids, id)
				seen[strings.ToLower(id)] = true
			}
		}
	}
	order := []string{"modelRules", "modelApiRules", "providerSiteRules", "templateModelRules", "builtinProviderModelRules"}
	for _, field := range order {
		if rules[field] == nil {
			rules[field] = []any{}
		}
		if values, valid := rules[field].([]any); !valid || len(values) > 16384 {
			return nil, protocol.ErrInvalid
		}
	}
	result := []sdk.Model{}
	for _, id := range ids {
		model := sdk.Model{ID: id, Name: id, Protocols: []codec.Protocol{codec.Messages}, Tools: true}
		enabled := false
		for _, group := range order {
			for _, item := range rules[group].([]any) {
				rule := mapObject(item)
				if rule == nil {
					return nil, protocol.ErrInvalid
				}
				matches := false
				if group == "templateModelRules" {
					matches = template != "" && rule["templateId"] == template && rule["modelId"] == id
				} else if group == "builtinProviderModelRules" {
					matches = rule["providerId"] == providerID && rule["modelId"] == id
				} else {
					matches, err = matchRule(rule["modelMatch"], id, true)
					if err != nil {
						return nil, err
					}
					if matches && group == "modelApiRules" {
						matches, err = matchRule(rule["apiTypeMatch"], "anthropic-messages", false)
					} else if matches && group == "providerSiteRules" {
						matches, err = matchRule(rule["baseUrlMatch"], endpoint, false)
						if matches && rule["apiTypeMatch"] != nil {
							matches, err = matchRule(rule["apiTypeMatch"], "anthropic-messages", false)
						}
					}
					if err != nil {
						return nil, err
					}
				}
				if matches {
					value := mapObject(rule["config"])
					if value == nil {
						return nil, protocol.ErrInvalid
					}
					if err := applyModelRule(&model, &enabled, value); err != nil {
						return nil, err
					}
				}
			}
		}
		if enabled {
			result = append(result, model)
		}
	}
	return result, nil
}
