package commandcode

import (
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func ParseModels(body []byte, subscription Subscription, extra, goPlan bool) ([]sdk.Model, error) {
	var envelope struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if decode(body, &envelope) != nil {
		return nil, errors.New("model catalog is malformed")
	}
	rows := envelope.Data
	if len(rows) == 0 {
		rows = envelope.Models
	}
	if len(rows) == 0 || len(rows) > 4096 {
		return nil, errors.New("model catalog is empty or too large")
	}
	models := make([]sdk.Model, 0, len(rows))
	seen := make(map[string]bool)
	for _, raw := range rows {
		var row struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			Display    string   `json:"display_name"`
			Context    int64    `json:"context_length"`
			Endpoints  []string `json:"supported_endpoints"`
			Modalities struct {
				Input []string `json:"input"`
			} `json:"modalities"`
		}
		if decode(raw, &row) != nil {
			return nil, errors.New("model entry is malformed")
		}
		if row.ID == "" {
			row.ID = row.Name
		}
		if row.ID == "" || len(row.ID) > 256 || strings.ContainsAny(row.ID, "\x00\r\n\t ") || seen[row.ID] {
			return nil, errors.New("model identity is invalid or duplicated")
		}
		seen[row.ID] = true
		if isDecision(row.ID) || !extra && refused(subscription.Plan, row.ID) {
			continue
		}
		model := sdk.Model{ID: row.ID, Name: row.Name, ContextWindow: row.Context, Tools: true, OutputModalities: []string{"text"}}
		known, found := knownGoModel(row.ID)
		if found {
			model.Efforts = slices.Clone(known.Efforts)
			model.Reasoning = len(model.Efforts) > 0
			if model.ContextWindow == 0 {
				model.ContextWindow = known.Context
			}
		}
		if row.Context < 0 {
			return nil, errors.New("model context is invalid")
		}
		if model.Name == "" || model.Name == row.ID {
			model.Name = row.Display
		}
		if model.Name == "" {
			model.Name = row.ID
		}
		if model.Name == row.ID && found {
			model.Name = known.Name
		}
		for _, endpoint := range row.Endpoints {
			endpoint = strings.TrimSuffix(strings.TrimPrefix(endpoint, "/v1"), "/")
			var protocol protocolext.Protocol
			switch endpoint {
			case "/chat/completions":
				protocol = "chat"
			case "/responses":
				protocol = "responses"
			case "/messages":
				protocol = "anthropic"
			default:
				continue
			}
			if !slices.Contains(model.Protocols, protocol) {
				model.Protocols = append(model.Protocols, protocol)
			}
		}
		if len(row.Endpoints) > 0 && len(model.Protocols) == 0 {
			continue
		}
		if len(model.Protocols) == 0 {
			protocol := protocolext.Protocol("chat")
			if isClaude(row.ID) {
				protocol = "anthropic"
			}
			model.Protocols = []protocolext.Protocol{protocol}
		}
		if goPlan {
			model.Protocols = []protocolext.Protocol{"chat"}
		}
		if row.Modalities.Input != nil {
			model.InputModalities = slices.Clone(row.Modalities.Input)
			model.Vision = slices.Contains(model.InputModalities, "image")
		} else if isClaude(row.ID) {
			model.InputModalities = []string{"text", "image"}
			model.Vision = true
		} else if found && known.Images {
			model.InputModalities = []string{"text", "image"}
			model.Vision = true
		}
		if goPlan {
			model.MaxOutput = 200000
			if len(model.Efforts) > 0 {
				model.Efforts = append([]string{"none"}, model.Efforts...)
			}
		}
		if !goPlan && thinkOff[row.ID] {
			model.Efforts = []string{"none", "low", "high", "max"}
			model.Reasoning = true
		}
		models = append(models, model)
	}
	return append(models, sdk.Model{ID: Jev, Name: "Jev", Kind: "decision",
		Protocols: []protocolext.Protocol{}, Operations: []sdk.ProviderOperation{sdk.OperationSystemOne}}), nil
}

const Jev = "typesafe/jev"

func isDecision(id string) bool {
	return id == Jev || strings.HasPrefix(id, Jev+"-") || id == "jev" || id == "jev-latest" || id == "typesafe-ai/jev"
}

//go:embed testdata/go-models.json
var knownGoModelsJSON []byte

type knownModel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Context int64    `json:"context"`
	Images  bool     `json:"images"`
	Efforts []string `json:"efforts"`
}

var knownGoModels = func() []knownModel {
	var models []knownModel
	if err := json.Unmarshal(knownGoModelsJSON, &models); err != nil {
		panic(err)
	}
	return models
}()

func knownGoModel(id string) (knownModel, bool) {
	for _, model := range knownGoModels {
		if model.ID == id {
			return model, true
		}
	}
	return knownModel{}, false
}

func isClaude(id string) bool {
	return strings.HasPrefix(strings.ToLower(id), "claude-") || strings.HasPrefix(id, "anthropic/")
}

func set(items ...string) map[string]bool {
	value := make(map[string]bool, len(items))
	for _, item := range items {
		value[item] = true
	}
	return value
}

var premium = set(
	"claude-sonnet-5", "claude-sonnet-4-6", "claude-fable-5-1", "claude-fable-5",
	"claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7",
	"claude-haiku-4-5-20251001", "gpt-5.6-terra", "gpt-6-astra", "gpt-6.1-sol",
	"gpt-6-sol", "gpt-5.5", "gpt-5.4", "gpt-5.3-codex", "gpt-5.4-mini",
	"google/gemini-3.5-flash", "google/gemini-3.1-flash-lite", "sakana/fugu-ultra", "meta/muse-spark-1.1",
)
var goBlocked = set(
	"meta/muse-spark-1.2", "meta/muse-spark-1.3", "xai/grok-4.6", "xai/grok-4.7",
	"xiaomi/mimo-v2.6-pro-ultraspeed", "google/gemini-3.7-flash", "google/gemini-3.8-flash",
	"gpt-5.6-sol", "claude-sonnet-5-5", "claude-haiku-5-5",
)
var proBlocked = set(
	"claude-fable-5-1", "claude-fable-5", "claude-opus-5-5", "claude-opus-5",
	"claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-opus-4-5-20251101",
	"gpt-6-astra", "gpt-6.1-sol", "sakana/fugu-ultra",
)
var thinkOff = set(
	"deepseek/deepseek-v4-pro", "deepseek/deepseek-v4-flash", "deepseek/deepseek-v4-flash-vision-exp",
	"deepseek/deepseek-v4.1-flash", "deepseek/deepseek-v4.1-flash-fast",
)

func refused(plan, id string) bool {
	switch plan {
	case "Go":
		return premium[id] || goBlocked[id]
	case "GOAT":
		return premium[id]
	case "Pro":
		return proBlocked[id]
	case "Provider":
		return id == "gpt-6.1-sol"
	default:
		return false
	}
}
