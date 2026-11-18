package devin

import (
	"errors"
	"slices"
	"strings"
	"unicode"
)

var effortOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

var nativeAliases = map[string][]string{
	"adaptive":                 {},
	"swe-2":                    {"swe"},
	"swe-2-flash":              {},
	"claude-fable-5-1":         {"fable"},
	"claude-opus-5-5":          {},
	"gpt-6-astra":              {},
	"gpt-6-sol":                {},
	"gpt-6-luna":               {},
	"gemini-4-argon":           {},
	"kimi-k3-1-pro":            {},
	"glm-5.2":                  {},
	"glm-5-3":                  {},
	"claude-sonnet-5-5":        {},
	"gemini-3-8-flash":         {"gemini"},
	"claude-opus-4.7":          {},
	"claude-opus-4.8":          {},
	"claude-opus-5":            {"opus"},
	"claude-5-fable":           {},
	"claude-sonnet-5":          {"claude", "sonnet"},
	"claude-haiku-5-5":         {},
	"gemini-3.5-flash":         {},
	"gemini-3.6-flash":         {},
	"gemini-3-7-flash":         {},
	"gpt-5.6-sol":              {},
	"gpt-5.6-terra":            {},
	"gpt-5.6-luna":             {},
	"gpt-6-1-sol":              {},
	"gpt-6-1-terra":            {},
	"grok-4-6":                 {},
	"grok-4-7":                 {},
	"muse-wonder-1-0":          {},
	"nemotron-3-5-super":       {},
	"kimi-k3":                  {},
	"glm-5-3-flash":            {},
	"deepseek-v4-flash":        {},
	"deepseek-v4-1-flash-1017": {},
	"swe-1.7":                  {},
	"fusion":                   {},
	"claude-opus-4.6":          {},
	"gpt-5.4":                  {},
	"gpt-5.5":                  {"gpt"},
	"gpt-5.4-mini":             {},
	"claude-sonnet-4.6":        {},
	"kimi-k2.6":                {},
	"kimi-k2.7":                {},
	"nemotron-3-ultra":         {},
	"swe-1.6":                  {},
	"deepseek-v4-pro":          {},
	"swe-1.7-lightning":        {},
	"inkling":                  {},
	"grok-4.5":                 {},
	"gpt-5.2":                  {},
	"claude-opus-4.5":          {},
	"Claude Haiku 4.5":         {},
	"claude-sonnet-4.5":        {},
	"GPT-4.1":                  {},
	"gpt-5.1":                  {},
	"gpt-5.3-codex":            {"codex"},
	"swe-1.6-fast":             {},
	"gemini-3.1-pro":           {},
	"gemini-3.0-flash":         {},
}

type nativeFamily struct {
	id, name string
	models   []NativeModel
}

func modelLevel(id string) string {
	lower := strings.ToLower(id)
	for _, level := range effortOrder {
		for _, separator := range []string{"-", "_"} {
			if strings.HasSuffix(lower, separator+level) {
				return level
			}
			if level == "minimal" && strings.HasSuffix(lower, separator+"min") {
				return level
			}
		}
	}
	return ""
}

func modelTier(id string) (string, string) {
	lower := strings.ToLower(id)
	for _, tier := range []string{"fast", "priority"} {
		for _, separator := range []string{"-", "_"} {
			if base, found := strings.CutSuffix(lower, separator+tier); found {
				if level := modelLevel(base); level != "" {
					return tier, level
				}
			}
		}
	}
	return "", modelLevel(id)
}

func nativeFamilies(models []NativeModel) []nativeFamily {
	var result []nativeFamily
	indices := map[string]int{}
	for _, model := range models {
		id := model.Family
		if id == "" {
			id = model.FamilyName
		}
		if id == "" {
			id = model.Name
		}
		if id == "" {
			id = model.UID
		}
		index, found := indices[id]
		if !found {
			name := model.FamilyName
			if name == "" {
				name = model.Name
			}
			index = len(result)
			indices[id] = index
			result = append(result, nativeFamily{id: id, name: name})
		}
		result[index].models = append(result[index].models, model)
	}
	return result
}

func (family nativeFamily) levels(tier string) ([]string, map[string]NativeModel) {
	byLevel := map[string]NativeModel{}
	for _, model := range family.models {
		foundTier, level := modelTier(model.UID)
		if foundTier == tier && level != "" && model.UID != family.id {
			if _, found := byLevel[level]; !found {
				byLevel[level] = model
			}
		}
	}
	var levels []string
	for _, level := range effortOrder {
		if _, found := byLevel[level]; found {
			levels = append(levels, level)
		}
	}
	return levels, byLevel
}

func (family nativeFamily) defaultModel() NativeModel {
	for _, model := range family.models {
		index := strings.LastIndex(model.UID, "-")
		if index < 0 || modelLevel(model.UID[:index]) == "" {
			return model
		}
		word := strings.ToLower(model.UID[index+1:])
		if word == "fast" || word == "priority" {
			return model
		}
	}
	return family.models[0]
}

func nearestEffort(want string, levels []string) string {
	if slices.Contains(levels, want) {
		return want
	}
	position := slices.Index(effortOrder, want)
	best, distance := "", len(effortOrder)+1
	for _, level := range levels {
		candidate := slices.Index(effortOrder, level)
		gap := position - candidate
		if gap < 0 {
			gap = -gap
		}
		if gap < distance || gap == distance && candidate > position {
			best, distance = level, gap
		}
	}
	return best
}

func (family nativeFamily) defaultEffort() string {
	tier, level := modelTier(family.defaultModel().UID)
	if tier == "" && level != "" {
		return level
	}
	return "medium"
}

func familyNames(family nativeFamily) []string {
	return append([]string{family.id}, nativeAliases[family.id]...)
}

func IsNativeModelSelection(id string) bool {
	if nativeAgentRouter(id) {
		return true
	}
	for family, aliases := range nativeAliases {
		for _, name := range append([]string{family}, aliases...) {
			if id == name || id == name+"-fast" || id == name+"-priority" {
				return true
			}
		}
	}
	return false
}

func isClaudeModel(id string) bool {
	if strings.Contains(strings.ToLower(id), "claude") {
		return true
	}
	for family, aliases := range nativeAliases {
		if !strings.HasPrefix(family, "claude-") {
			continue
		}
		for _, alias := range aliases {
			if id == alias || id == alias+"-fast" || id == alias+"-priority" {
				return true
			}
		}
	}
	return false
}

func ResolveNativeModel(models []NativeModel, id, effort string) (string, error) {
	families := nativeFamilies(models)
	for _, family := range families {
		for _, name := range familyNames(family) {
			if id == name {
				levels, byLevel := family.levels("")
				if effort != "" {
					if model, found := byLevel[nearestEffort(effort, levels)]; found {
						return model.UID, nil
					}
				}
				return family.defaultModel().UID, nil
			}
		}
	}
	for _, family := range families {
		for _, name := range familyNames(family) {
			for _, tier := range []string{"fast", "priority"} {
				if id != name+"-"+tier {
					continue
				}
				levels, byLevel := family.levels(tier)
				want := effort
				if want == "" {
					want = family.defaultEffort()
				}
				if model, found := byLevel[nearestEffort(want, levels)]; found {
					return model.UID, nil
				}
			}
		}
	}
	for _, model := range models {
		if id == model.UID {
			return id, nil
		}
	}
	return "", errors.New("selected Devin model is absent from the current account catalog")
}

func ProjectNativeModels(models []NativeModel) []NativeModel {
	var result []NativeModel
	seen := map[string]bool{}
	add := func(model NativeModel) {
		if !seen[model.UID] {
			result = append(result, model)
			seen[model.UID] = true
		}
	}
	families := nativeFamilies(models)
	for _, family := range families {
		if len(family.id) > 300 || strings.ContainsFunc(family.id, func(char rune) bool {
			return unicode.IsSpace(char) || unicode.IsControl(char)
		}) {
			continue
		}
		for _, tier := range []string{"", "fast", "priority"} {
			levels, byLevel := family.levels(tier)
			if tier != "" && len(levels) == 0 {
				continue
			}
			var represented []NativeModel
			for _, level := range levels {
				represented = append(represented, byLevel[level])
			}
			if tier == "" {
				defaultModel := family.defaultModel()
				if !slices.ContainsFunc(represented, func(model NativeModel) bool { return model.UID == defaultModel.UID }) {
					represented = append(represented, defaultModel)
				}
			}
			model := family.models[0]
			model.UID, model.Name, model.Family = family.id, family.name, family.id
			model.Virtual = true
			model.Efforts = levels
			model.DefaultEffort = ""
			if tier != "" {
				model.DefaultEffort = nearestEffort(family.defaultEffort(), levels)
			} else if defaultTier, defaultLevel := modelTier(family.defaultModel().UID); defaultTier == "" && slices.Contains(levels, defaultLevel) {
				model.DefaultEffort = defaultLevel
			}
			model.Free = true
			model.Price = represented[0].Price
			for _, variant := range represented {
				model.Free = model.Free && variant.Free
				if model.Price == nil || variant.Price == nil || *variant.Price != *model.Price {
					model.Price = nil
				}
			}
			for _, variant := range family.models {
				model.Images = model.Images && variant.Images
				model.Tools = model.Tools && variant.Tools
				model.Thinking = model.Thinking && variant.Thinking
				if model.Context == 0 {
					model.Context = variant.Context
				}
				if model.MaxOutput == 0 {
					model.MaxOutput = variant.MaxOutput
				}
			}
			suffix := ""
			if tier != "" {
				suffix = "-" + tier
				label := "Fast"
				if tier == "priority" {
					label = "Priority"
				}
				model.Name += " " + label
			}
			for _, name := range familyNames(family) {
				model.UID = name + suffix
				model.Virtual = model.UID != family.defaultModel().UID || len(family.models) > 1
				if model.UID != family.id && slices.ContainsFunc(models, func(variant NativeModel) bool { return variant.UID == model.UID }) {
					continue
				}
				if tier != "" && slices.ContainsFunc(families, func(other nativeFamily) bool {
					return slices.Contains(familyNames(other), model.UID)
				}) {
					continue
				}
				add(model)
			}
		}
	}
	for _, model := range models {
		_, level := modelTier(model.UID)
		if level != "" {
			model.Efforts, model.DefaultEffort = []string{level}, level
		}
		add(model)
	}
	return result
}
