package opencode

import (
	"context"
	_ "embed"
	"encoding/json"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

//go:embed testdata/donors/zen-catalog.json
var zenCatalog []byte

type inferenceModel struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Status         string          `json:"status"`
	Lifecycle      string          `json:"lifecycle"`
	Deprecated     bool            `json:"deprecated"`
	DeprecatedAt   json.RawMessage `json:"deprecated_at"`
	RetirementDate json.RawMessage `json:"retirement_date"`
	Released       string          `json:"release_date"`
	Modalities     struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Capabilities struct {
		Tools struct {
			Supported bool `json:"supported"`
		} `json:"tools"`
		Reasoning struct {
			Supported bool     `json:"supported"`
			Effort    []string `json:"effort"`
		} `json:"reasoning"`
		Temperature *bool `json:"temperature"`
	} `json:"capabilities"`
	Limit struct {
		Context int64 `json:"context"`
		Input   int64 `json:"input"`
		Output  int64 `json:"output"`
	} `json:"limit"`
	Cost struct {
		Text struct {
			Input struct {
				Price *float64 `json:"price"`
			} `json:"input"`
			Output struct {
				Price *float64 `json:"price"`
			} `json:"output"`
		} `json:"text"`
	} `json:"cost"`
	API map[string]json.RawMessage `json:"api"`
}

func zenMetadata() map[string]inferenceModel {
	var catalog struct {
		Models map[string]inferenceModel `json:"models"`
	}
	if json.Unmarshal(zenCatalog, &catalog) != nil {
		panic("invalid pinned Zen catalog")
	}
	return catalog.Models
}

func modalities(values []string) []string {
	var result []string
	for _, value := range values {
		name := strings.SplitN(value, "/", 2)[0]
		if value == "application/pdf" {
			name = "pdf"
		}
		if slices.Contains([]string{"text", "image", "audio", "video", "pdf"}, name) && !slices.Contains(result, name) {
			result = append(result, name)
		}
	}
	return result
}

func zenBuildModels(listed []string, public bool) []sdk.Model {
	return zenProjectModels(listed, nil, public)
}

const (
	zenOpenAIBase    = "/inference/openai/v1"
	zenAnthropicBase = "/inference/anthropic/v1"
	zenGeminiBase    = "/inference/google/v1beta"
)

func zenProjectModels(listed []string, live map[string]inferenceModel, public bool) []sdk.Model {
	metadata := zenMetadata()
	seen := map[string]bool{}
	result := []sdk.Model{}
	for _, id := range listed {
		if !validInferenceModelID(id) || seen[id] {
			continue
		}
		seen[id] = true
		switch id {
		case "glm-5.3-flash-free", "glm-5", "glm-5.1", "glm-5.2":
			continue
		}
		model, known := metadata[id]
		if model.retired() {
			continue
		}
		liveModel, rich := live[id]
		if rich {
			if descriptor, ok := liveModel.projectLive(false, public); ok {
				result = append(result, descriptor)
			}
			continue
		}
		if known && model.Type != "chat" {
			continue
		}
		free := model.free()
		if public && !free && !strings.Contains(strings.ToLower(id), "free") || !known && !strings.HasSuffix(id, "-free") {
			continue
		}
		protocols := model.protocols(false, false)
		if !known {
			protocols = []protocolext.Protocol{"chat"}
			model.Name, model.Limit.Context, model.Limit.Output = id, 32768, 4096
			model.Modalities.Input, model.Modalities.Output = []string{"text"}, []string{"text"}
		}
		if len(protocols) == 0 {
			continue
		}
		if id == "deepseek-v4.1-flash-free" && model.Limit.Context < 258624 {
			model.Limit.Context = 258624
		}
		model.ID = id
		result = append(result, model.descriptor(protocols, public || free))
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func validInferenceModelID(id string) bool {
	return id != "" && len(id) <= 256 && !strings.ContainsAny(id, "/\\?#") &&
		!strings.ContainsFunc(id, func(character rune) bool { return unicode.IsSpace(character) || unicode.IsControl(character) })
}

func (model inferenceModel) free() bool {
	priceIn, priceOut := model.Cost.Text.Input.Price, model.Cost.Text.Output.Price
	return priceIn != nil && *priceIn == 0 && priceOut != nil && *priceOut == 0
}

func (model inferenceModel) hasMetadata() bool {
	return model.Type != "" || model.API != nil || model.Limit.Context != 0 || model.Limit.Input != 0 || model.Limit.Output != 0 ||
		model.Cost.Text.Input.Price != nil || model.Cost.Text.Output.Price != nil || model.retired()
}

func (model inferenceModel) protocols(goPlan, live bool) []protocolext.Protocol {
	routes := []struct{ API, Protocol, Base string }{
		{"chat-completions", "chat", zenOpenAIBase}, {"responses", "responses", zenOpenAIBase},
		{"messages", "messages", zenAnthropicBase}, {"generate-content", "gemini", zenGeminiBase},
	}
	if goPlan {
		routes = []struct{ API, Protocol, Base string }{
			{"chat-completions", "chat", goOpenAIBase}, {"generate-content", "gemini", goGeminiBase},
		}
	}
	var protocols []protocolext.Protocol
	for _, route := range routes {
		if entry := model.API[route.API]; entry != nil && (!live || nativeInferenceEndpoint(entry, route.Base)) {
			protocols = append(protocols, protocolext.Protocol(route.Protocol))
		}
	}
	return protocols
}

func (model inferenceModel) projectLive(goPlan, public bool) (sdk.Model, bool) {
	free := model.free()
	if model.Type != "chat" || model.retired() || !model.validLiveLimits() || public && !free {
		return sdk.Model{}, false
	}
	protocols := model.protocols(goPlan, true)
	if len(protocols) == 0 {
		return sdk.Model{}, false
	}
	if model.Limit.Input > 0 {
		model.Limit.Context = min(model.Limit.Context, model.Limit.Input)
	}
	if _, err := time.Parse(time.DateOnly, model.Released); err != nil {
		model.Released = ""
	}
	model.Capabilities.Reasoning.Effort = knownInferenceEfforts(model.Capabilities.Reasoning.Effort)
	return model.descriptor(protocols, free), true
}

func (model inferenceModel) descriptor(protocols []protocolext.Protocol, free bool) sdk.Model {
	if model.Name == "" {
		model.Name = model.ID
	}
	input := modalities(model.Modalities.Input)
	return sdk.Model{ID: model.ID, Name: model.Name, Protocols: protocols,
		ContextWindow: model.Limit.Context, MaxOutput: model.Limit.Output,
		Tools: model.Capabilities.Tools.Supported, Reasoning: model.Capabilities.Reasoning.Supported,
		Efforts: model.Capabilities.Reasoning.Effort, Temperature: model.Capabilities.Temperature,
		Vision: slices.Contains(input, "image"), Kind: "chat", InputModalities: input,
		OutputModalities: modalities(model.Modalities.Output), Released: model.Released, Free: free}
}

func (model inferenceModel) validLiveLimits() bool {
	limit := model.Limit
	return limit.Context > 0 && limit.Context <= math.MaxInt32 && limit.Output > 0 &&
		limit.Output <= limit.Context && limit.Input >= 0 && limit.Input <= math.MaxInt32
}

func nativeInferenceEndpoint(entry json.RawMessage, base string) bool {
	var api struct {
		BaseURL string `json:"base_url"`
	}
	return json.Unmarshal(entry, &api) == nil &&
		strings.TrimSuffix(api.BaseURL, "/") == DefaultEndpoints().Inference+base
}

func knownInferenceEfforts(efforts []string) []string {
	var known []string
	for _, effort := range efforts {
		switch effort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
			if !slices.Contains(known, effort) {
				known = append(known, effort)
			}
		}
	}
	return known
}

func zenDiscoverModels(body []byte, public bool) ([]sdk.Model, bool) {
	entries, ok := decodeInferenceList(body)
	if !ok {
		return nil, false
	}
	ids := make([]string, 0, len(entries))
	live := map[string]inferenceModel{}
	for _, model := range entries {
		ids = append(ids, model.ID)
		if model.hasMetadata() {
			live[model.ID] = model
		}
	}
	return zenProjectModels(ids, live, public), true
}

func decodeInferenceList(body []byte) ([]inferenceModel, bool) {
	var listing struct {
		Data []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &listing) != nil || listing.Data == nil {
		return nil, false
	}
	entries := make([]inferenceModel, 0, len(listing.Data))
	seen := map[string]bool{}
	for _, entry := range listing.Data {
		var model inferenceModel
		if json.Unmarshal(entry, &model) != nil || !validInferenceModelID(model.ID) || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		entries = append(entries, model)
	}
	return entries, len(entries) != 0 || len(listing.Data) == 0
}

func (model inferenceModel) retired() bool {
	status := model.Status
	if status == "" {
		status = model.Lifecycle
	}
	return model.Deprecated || slices.Contains([]string{"deprecated", "retired", "disabled"}, status) ||
		len(model.DeprecatedAt) > 0 && string(model.DeprecatedAt) != "null" ||
		len(model.RetirementDate) > 0 && string(model.RetirementDate) != "null"
}

func zenBootstrap(public bool) []sdk.Model {
	var ids []string
	for id := range zenMetadata() {
		ids = append(ids, id)
	}
	return zenBuildModels(ids, public)
}

type modelListing struct {
	models []sdk.Model
	at     time.Time
}

type zenLists struct {
	mu     sync.Mutex
	values map[string]modelListing
}

func (owner *module) zenModels(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ModelsRequest
	if decode(call.Params, &request) != nil || len(request.Credentials) != 0 && string(request.Credentials) != "null" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Zen catalog credentials must come from the selected account"}
	}
	if call.Scope.Account == "" && len(call.Auth) == 0 {
		return sdk.ModelsResult{Models: zenBootstrap(call.Scope.Provider == ProviderZenPublic)}, nil
	}
	identity, err := selectedZen(call)
	if err != nil {
		return nil, err
	}
	models, err := owner.discoverZen(ctx, call, stream, identity)
	return sdk.ModelsResult{Models: models}, err
}

func (owner *module) discoverZen(ctx context.Context, call sdk.Call, stream *sdk.Stream, identity ZenCredentials) ([]sdk.Model, error) {
	if identity.OAuth != nil && identity.OAuth.Due(owner.options.Now()) {
		return nil, failure(ErrRefreshRequired, "not-started")
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	key := call.Scope.Provider + ":" + call.Scope.Account + ":" + strconv.FormatUint(call.Scope.Generation, 10)
	state.console.catalog.mu.Lock()
	defer state.console.catalog.mu.Unlock()
	previous, found := state.console.catalog.values[key]
	if found && owner.options.Now().Sub(previous.at) < time.Minute {
		return slices.Clone(previous.models), nil
	}
	body := owner.client(stream).modelList(ctx, "/inference/v1/models", identity.access(), identity.organization())
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if models, valid := zenDiscoverModels(body, call.Scope.Provider == ProviderZenPublic); valid {
		if len(state.console.catalog.values) >= 128 {
			clear(state.console.catalog.values)
		}
		state.console.catalog.values[key] = modelListing{models: models, at: owner.options.Now()}
		return slices.Clone(models), nil
	}
	if found {
		return slices.Clone(previous.models), nil
	}
	return zenBootstrap(call.Scope.Provider == ProviderZenPublic), nil
}
