package opencode

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const (
	goOpenAIBase    = "/inference/go/openai/v1"
	goAnthropicBase = "/inference/go/anthropic/v1"
	goGeminiBase    = "/inference/go/google/v1beta"
)

type goCatalogKey struct {
	account    string
	org        string
	generation uint64
}

type goCatalogEntry struct {
	refresh chan struct{}
	listing atomic.Pointer[modelListing]
}

type goCatalog struct {
	mu      sync.Mutex
	entries map[goCatalogKey]*goCatalogEntry
}

func (catalog *goCatalog) entry(scope sdk.Scope, credentials Credentials, create bool) *goCatalogEntry {
	key := goCatalogKey{account: scope.Account, org: credentials.OrgID, generation: scope.Generation}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	entry := catalog.entries[key]
	if entry != nil || !create {
		return entry
	}
	if catalog.entries == nil || len(catalog.entries) >= 128 {
		catalog.entries = make(map[goCatalogKey]*goCatalogEntry)
	}
	entry = &goCatalogEntry{refresh: make(chan struct{}, 1)}
	catalog.entries[key] = entry
	return entry
}

func (owner *module) goModels(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ModelsRequest
	if decode(call.Params, &request) != nil || len(request.Credentials) != 0 && string(request.Credentials) != "null" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Go catalog credentials must come from the selected account"}
	}
	if call.Scope.Account == "" && len(call.Auth) == 0 {
		return sdk.ModelsResult{Models: Models()}, nil
	}
	credentials, err := selected(call)
	if err != nil {
		return nil, err
	}
	if credentials.Due(owner.options.Now()) {
		return nil, failure(ErrRefreshRequired, "not-started")
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	models, err := owner.discoverGo(ctx, call, stream, state, credentials)
	return sdk.ModelsResult{Models: models}, err
}

func (owner *module) discoverGo(ctx context.Context, call sdk.Call, stream *sdk.Stream, state *instance, credentials Credentials) ([]sdk.Model, error) {
	entry := state.goCatalog.entry(call.Scope, credentials, true)
	select {
	case entry.refresh <- struct{}{}:
		defer func() { <-entry.refresh }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	previous := entry.listing.Load()
	if previous != nil && owner.options.Now().Sub(previous.at) < time.Minute {
		return previous.models, nil
	}
	client := owner.client(stream)
	body := client.modelList(ctx, client.Endpoints.Console+"/api/v1/models", credentials.Access, "x-org-id", credentials.OrgID)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if models, ok := goDiscoverModels(body); ok {
		entry.listing.Store(&modelListing{models: models, at: owner.options.Now()})
		return models, nil
	}
	if previous != nil {
		return previous.models, nil
	}
	return nil, &sdk.Error{Code: "upstream", Message: "OpenCode Go workspace catalog unavailable", Outcome: "not-started"}
}

func goDiscoverModels(body []byte) ([]sdk.Model, bool) {
	var providers map[string]struct {
		ID     string                     `json:"id"`
		Models map[string]json.RawMessage `json:"models"`
	}
	if json.Unmarshal(body, &providers) != nil || providers == nil {
		return nil, false
	}
	for id, provider := range providers {
		if id == "" || id != provider.ID || provider.Models == nil {
			return nil, false
		}
	}
	entries := providers[ProviderGo].Models
	pinned := Models()
	models := make([]sdk.Model, 0, len(entries))
	for id, raw := range entries {
		var entry inferenceModel
		if json.Unmarshal(raw, &entry) != nil || entry.ID != id || !validInferenceModelID(id) {
			return nil, false
		}
		if retiredGLM(id) {
			continue
		}
		if entry.hasMetadata() {
			if descriptor, ok := entry.projectLive(true, false); ok {
				models = append(models, descriptor)
			}
		} else if index := slices.IndexFunc(pinned, func(model sdk.Model) bool { return model.ID == entry.ID }); index >= 0 {
			models = append(models, pinned[index])
		}
	}
	slices.SortFunc(models, func(left, right sdk.Model) int { return strings.Compare(left.ID, right.ID) })
	return models, true
}

func supportsCatalogModel(models []sdk.Model, id, protocol string) (known, supported bool) {
	index := slices.IndexFunc(models, func(model sdk.Model) bool { return model.ID == id })
	if index < 0 {
		return false, false
	}
	return true, slices.Contains(models[index].Protocols, protocolext.Protocol(protocol))
}

func (owner *module) canExecuteGo(ctx context.Context, call sdk.Call, stream *sdk.Stream, state *instance, credentials Credentials, model, protocol string) (bool, error) {
	if entry := state.goCatalog.entry(call.Scope, credentials, false); entry != nil {
		if listing := entry.listing.Load(); listing != nil {
			_, supported := supportsCatalogModel(listing.models, model, protocol)
			return supported, nil
		}
	}
	models, err := owner.discoverGo(ctx, call, stream, state, credentials)
	if err != nil {
		if ctx.Err() == nil && modelSupports(model, protocol) {
			return true, nil
		}
		return false, err
	}
	_, supported := supportsCatalogModel(models, model, protocol)
	return supported, nil
}
