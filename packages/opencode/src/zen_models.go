package opencode

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

//go:embed testdata/donors/zen-catalog.json
var zenCatalog []byte

type zenModel struct {
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

func zenMetadata() map[string]zenModel {
	var catalog struct {
		Models map[string]zenModel `json:"models"`
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
	metadata := zenMetadata()
	seen := map[string]bool{}
	result := []sdk.Model{}
	for _, id := range listed {
		if id == "" || len(id) > 256 || strings.ContainsAny(id, "/\\?#") || strings.ContainsFunc(id, func(character rune) bool { return character < 0x20 || character == 0x7f }) || seen[id] {
			continue
		}
		seen[id] = true
		model, known := metadata[id]
		if id == "glm-5.3-flash-free" || zenRetired(model) || known && model.Type != "chat" {
			continue
		}
		free := model.Cost.Text.Input.Price != nil && *model.Cost.Text.Input.Price == 0 &&
			model.Cost.Text.Output.Price != nil && *model.Cost.Text.Output.Price == 0
		if public && !free && !strings.Contains(strings.ToLower(id), "free") || !known && !strings.HasSuffix(id, "-free") {
			continue
		}
		var protocols []protocolext.Protocol
		for _, pair := range []struct{ API, Protocol string }{
			{"chat-completions", "chat"}, {"responses", "responses"}, {"messages", "messages"}, {"generate-content", "gemini"},
		} {
			if model.API[pair.API] != nil {
				protocols = append(protocols, protocolext.Protocol(pair.Protocol))
			}
		}
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
		input := modalities(model.Modalities.Input)
		result = append(result, sdk.Model{ID: id, Name: model.Name, Protocols: protocols,
			ContextWindow: model.Limit.Context, MaxOutput: model.Limit.Output,
			Tools: model.Capabilities.Tools.Supported, Reasoning: model.Capabilities.Reasoning.Supported,
			Efforts: model.Capabilities.Reasoning.Effort, Temperature: model.Capabilities.Temperature,
			Vision: slices.Contains(input, "image"), Kind: "chat", InputModalities: input,
			OutputModalities: modalities(model.Modalities.Output), Released: model.Released, Free: public || free})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func zenRetired(model zenModel) bool {
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

type zenListing struct {
	models []sdk.Model
	at     time.Time
}

type zenLists struct {
	mu     sync.Mutex
	values map[string]zenListing
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
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, owner.options.Endpoints.Inference+"/inference/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+identity.access())
	req.Header.Set("User-Agent", "opencode/latest/"+ClientVersion+"/cli")
	req.Header.Set("x-opencode-client", "cli")
	if org := identity.organization(); org != "" {
		req.Header.Set("x-opencode-org-id", org)
	}
	response, err := owner.client(stream).HTTP.Do(req)
	if err == nil {
		defer response.Body.Close()
		var listing struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		body, readErr := readBounded(response.Body, maxReplyBytes)
		if readErr == nil && response.StatusCode == 200 && json.Unmarshal(body, &listing) == nil && listing.Data != nil {
			var ids []string
			for _, entry := range listing.Data {
				ids = append(ids, entry.ID)
			}
			models := zenBuildModels(ids, call.Scope.Provider == ProviderZenPublic)
			if len(state.console.catalog.values) >= 128 {
				clear(state.console.catalog.values)
			}
			state.console.catalog.values[key] = zenListing{models: models, at: owner.options.Now()}
			return slices.Clone(models), nil
		}
	}
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	if found {
		return slices.Clone(previous.models), nil
	}
	return zenBootstrap(call.Scope.Provider == ProviderZenPublic), nil
}
