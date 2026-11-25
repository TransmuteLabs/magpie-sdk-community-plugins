package qoder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

var (
	effortOrder = []string{"minimal", "low", "medium", "high", "xhigh", "max"}
	eff5        = []string{"low", "medium", "high", "xhigh", "max"}
	identityErr = regexp.MustCompile(`(?i)identity could not be verified`)
)

type ModelEntry struct {
	Key           string
	Source        string
	Name          string
	Images        bool
	Context       int64
	Windows       []int64
	DefaultWindow int64
	Thinks        bool
	AlwaysThinks  bool
	Efforts       []string
	DefaultEffort string
	Free          bool
	Rate          float64
	RateWas       float64
	Config        map[string]any
}

type lingmaModelSpec struct {
	Key  string
	Name string
}

var lingmaModelsList = []lingmaModelSpec{
	{Key: "qmodel_latest", Name: "Qwen3.7-Max"},
	{Key: "dashscope_qmodel", Name: "Qwen3.7-Plus"},
	{Key: "q36fmodel", Name: "Qwen3.6-Flash"},
}

func defaultQoderModels() []StaticModel {
	return []StaticModel{
		{ID: "ultimate", Name: "Ultimate", Context: 1_000_000, Images: true, Efforts: slices.Clone(eff5)},
		{ID: "fast", Name: "Fast", Context: 1_000_000, Images: true},
		{ID: "performance", Name: "Performance", Context: 1_000_000, Images: true, Efforts: slices.Clone(eff5)},
		{ID: "efficient", Name: "Efficient", Context: 200_000, Images: true},
		{ID: "smodel", Name: "Sonus", Context: 180_000, Images: true, Efforts: slices.Clone(eff5)},
		{ID: "cmodel", Name: "Cantus", Context: 180_000, Images: true, Efforts: slices.Clone(eff5)},
		{ID: "qmodel_38max", Name: "Qwen3.8-Max", Context: 180_000, Images: true, Efforts: []string{"low", "medium", "xhigh"}},
		{ID: "qfmodel", Name: "Qwen3.8-Flash", Context: 180_000, Images: true, Efforts: []string{"low", "medium", "xhigh"}},
		{ID: "qmodel_latest", Name: "Qwen3.7-Max", Context: 1_000_000, Images: true},
		{ID: "qmodel", Name: "Qwen3.7-Plus", Context: 1_000_000, Images: true},
		{ID: "kmodel_latest", Name: "Kimi-K3", Context: 180_000, Images: true, Efforts: []string{"low", "high", "max"}},
		{ID: "kmodel", Name: "Kimi-K2.8-Preview", Images: true, Efforts: []string{"low", "high", "max"}},
		{ID: "gmodel", Name: "GLM-5.3", Context: 180_000, Images: true, Efforts: []string{"low", "high", "max"}},
		{ID: "gfmodel", Name: "GLM-5.3-Flash", Context: 1_000_000, Images: true, Efforts: []string{"high", "max"}},
		{ID: "dmodel", Name: "DeepSeek-V4-Pro", Context: 1_000_000, Images: true, Efforts: []string{"high", "max"}},
		{ID: "dfmodel", Name: "DeepSeek-Flash", Context: 1_000_000, Images: true, Efforts: []string{"low", "high", "max"}},
		{ID: "mmodel", Name: "MiniMax-M3", Context: 180_000, Images: true},
	}
}

func defaultQoderCNModels() []StaticModel {
	var out []StaticModel
	for _, model := range defaultQoderModels() {
		if model.ID == "ultimate" || model.ID == "performance" || model.ID == "efficient" {
			out = append(out, model)
		}
	}
	out = append(out, StaticModel{ID: "lite", Name: "Lite"})
	return out
}

func defaultQwenWorkModels() []StaticModel {
	return []StaticModel{
		{ID: "flash", Name: "标准"},
		{ID: "pro", Name: "高级"},
		{ID: "qwen3.8-max-preview", Name: "Qwen3.8-Max"},
	}
}

func qwenworkFallbackListing() map[string][]map[string]any {
	return map[string][]map[string]any{
		"qwork": {
			{"key": "flash", "display_name": "标准", "is_vl": false, "is_reasoning": false, "max_input_tokens": float64(131072)},
			{"key": "pro", "display_name": "高级", "is_vl": false, "is_reasoning": false, "max_input_tokens": float64(131072)},
			{"key": "qwen3.8-max-preview", "display_name": "Qwen3.8-Max", "is_vl": false, "is_reasoning": false, "max_input_tokens": float64(131072)},
		},
	}
}

func lingmaModelEntry(spec lingmaModelSpec) ModelEntry {
	return parseModelInfo(map[string]any{
		"key":              spec.Key,
		"display_name":     spec.Name,
		"format":           "dashscope",
		"source":           "system",
		"max_input_tokens": float64(180_000),
	})
}

func lingmaModelEntries() []ModelEntry {
	out := make([]ModelEntry, 0, len(lingmaModelsList))
	for _, spec := range lingmaModelsList {
		out = append(out, lingmaModelEntry(spec))
	}
	return out
}

func boolVal(raw any) bool {
	if flag, ok := raw.(bool); ok {
		return flag
	}
	return false
}

func positiveInt64(raw any) int64 {
	val, ok := floatField(map[string]any{"v": raw}, "v")
	if !ok || val <= 0 || math.Trunc(val) != val {
		return 0
	}
	return int64(val)
}

func promotionObject(raw map[string]any) map[string]any {
	if promo, ok := raw["promotion"].(map[string]any); ok && promo != nil {
		return promo
	}
	if promo, ok := raw["prommotion"].(map[string]any); ok && promo != nil {
		return promo
	}
	return nil
}

func freeOf(raw map[string]any) bool {
	price, hasPrice := floatField(raw, "price_factor")
	if !hasPrice {
		price, hasPrice = floatField(raw, "priceFactor")
	}
	if hasPrice {
		if price != 0 {
			return false
		}
		promo := promotionObject(raw)
		if promo != nil && boolVal(promo["active"]) {
			before, hasBefore := floatField(promo, "before_promotion_price_factor")
			if !hasBefore {
				before, hasBefore = floatField(promo, "beforePromotionPriceFactor")
			}
			if hasBefore && before > 0 {
				return false
			}
		}
		return true
	}
	if flag, ok := raw["is_free"].(bool); ok {
		return flag
	}
	if flag, ok := raw["isFree"].(bool); ok {
		return flag
	}
	return false
}

func rateOf(raw map[string]any) (float64, float64) {
	price, hasPrice := floatField(raw, "price_factor")
	if !hasPrice {
		price, hasPrice = floatField(raw, "priceFactor")
	}
	if !hasPrice {
		return 0, 0
	}
	promo := promotionObject(raw)
	var before float64
	var hasBefore bool
	var discount float64
	if promo != nil {
		before, hasBefore = floatField(promo, "before_promotion_price_factor")
		if !hasBefore {
			before, hasBefore = floatField(promo, "beforePromotionPriceFactor")
		}
		if val, ok := floatField(promo, "discount_factor"); ok {
			discount = val
		} else if val, ok := floatField(promo, "discountFactor"); ok {
			discount = val
		}
	}
	discounted := promo != nil && boolVal(promo["active"]) && hasBefore && before > 0
	rate := math.Max(price, 0)
	origSnake, _ := floatField(raw, "original_price_factor")
	origCamel, _ := floatField(raw, "originalPriceFactor")
	rateWas := math.Max(origSnake, origCamel)
	if discounted {
		rateWas = before
		if rate == 0 {
			rate = before * discount
		}
	}
	if rateWas <= rate {
		rateWas = 0
	}
	return rate, rateWas
}

func windowsOf(raw map[string]any) ([]int64, int64) {
	var windows []int64
	var defaultWin int64
	contextCfg, _ := raw["context_config"].(map[string]any)
	if contextCfg == nil {
		contextCfg, _ = raw["contextConfig"].(map[string]any)
	}
	if contextCfg != nil {
		for _, item := range contextCfg {
			entry, ok := item.(map[string]any)
			if !ok || entry == nil {
				continue
			}
			tokens := positiveInt64(entry["token_count"])
			if tokens == 0 {
				continue
			}
			if !slices.Contains(windows, tokens) {
				windows = append(windows, tokens)
			}
			switch defFlag := entry["is_default"].(type) {
			case bool:
				if defFlag {
					defaultWin = tokens
				}
			case float64:
				if defFlag == 1 {
					defaultWin = tokens
				}
			case string:
				if defFlag == "1" {
					defaultWin = tokens
				}
			}
		}
	}
	if len(windows) == 0 {
		avail, _ := raw["available_context_windows"].([]any)
		if avail == nil {
			avail, _ = raw["availableContextWindows"].([]any)
		}
		for _, item := range avail {
			tokens := positiveInt64(item)
			if tokens > 0 && !slices.Contains(windows, tokens) {
				windows = append(windows, tokens)
			}
		}
		defaultWin = positiveInt64(raw["default_context_window"])
		if defaultWin == 0 {
			defaultWin = positiveInt64(raw["defaultContextWindow"])
		}
		if !slices.Contains(windows, defaultWin) {
			defaultWin = 0
		}
	}
	slices.Sort(windows)
	return windows, defaultWin
}

func parseModelInfo(raw map[string]any) ModelEntry {
	windows, defaultWin := windowsOf(raw)
	key := strField(raw, "key")
	displayName := firstNonEmpty(strField(raw, "display_name"), key)
	contextLimit := positiveInt64(raw["max_input_tokens"])
	if len(windows) > 0 {
		contextLimit = windows[len(windows)-1]
	}
	var efforts []string
	if rawEfforts, ok := raw["reasoning_efforts"].([]any); ok {
		for _, item := range rawEfforts {
			if strVal, ok := item.(string); ok && strVal != "" {
				efforts = append(efforts, strVal)
			}
		}
	}
	rate, rateWas := rateOf(raw)
	entry := ModelEntry{
		Key:           key,
		Source:        strField(raw, "source"),
		Name:          displayName,
		Images:        boolVal(raw["is_vl"]),
		Context:       contextLimit,
		Windows:       windows,
		DefaultWindow: defaultWin,
		Thinks:        boolVal(raw["is_reasoning"]),
		Efforts:       efforts,
		Free:          freeOf(raw),
		Rate:          rate,
		RateWas:       rateWas,
		Config:        raw,
	}
	if thinkCfg, ok := raw["thinking_config"].(map[string]any); ok && thinkCfg != nil {
		enabledVal, hasEnabled := thinkCfg["enabled"]
		disabledVal, hasDisabled := thinkCfg["disabled"]
		entry.Thinks = hasEnabled && enabledVal != nil
		entry.AlwaysThinks = entry.Thinks && (!hasDisabled || disabledVal == nil)
		entry.Efforts = nil
		if entry.Thinks {
			if enabledMap, ok := enabledVal.(map[string]any); ok && enabledMap != nil {
				if effMap, ok := enabledMap["efforts"].(map[string]any); ok {
					for name, cfgRaw := range effMap {
						entry.Efforts = append(entry.Efforts, name)
						if cfgMap, ok := cfgRaw.(map[string]any); ok && boolVal(cfgMap["is_default"]) {
							entry.DefaultEffort = name
						}
					}
				}
			}
			rank := func(level string) int {
				idx := slices.Index(effortOrder, level)
				if idx < 0 {
					return len(effortOrder)
				}
				return idx
			}
			sort.Slice(entry.Efforts, func(left, right int) bool {
				lRank, rRank := rank(entry.Efforts[left]), rank(entry.Efforts[right])
				if lRank != rRank {
					return lRank < rRank
				}
				return entry.Efforts[left] < entry.Efforts[right]
			})
		}
	}
	return entry
}

func extractRawEntries(listing map[string]any, scene string) []map[string]any {
	rawSlice, ok := listing[scene].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(rawSlice))
	for _, item := range rawSlice {
		if obj, ok := item.(map[string]any); ok && obj != nil {
			out = append(out, obj)
		}
	}
	return out
}

func sceneModels(listing map[string]any) []map[string]any {
	appList := extractRawEntries(listing, "app")
	chatList := extractRawEntries(listing, "chat")
	out := make([]map[string]any, len(appList))
	copy(out, appList)
	after := -1
	for _, chatModel := range chatList {
		chatKey := strField(chatModel, "key")
		foundIdx := -1
		for idx, existing := range out {
			if strField(existing, "key") == chatKey {
				foundIdx = idx
				break
			}
		}
		if foundIdx >= 0 {
			after = foundIdx
			continue
		}
		after++
		out = slices.Insert(out, after, chatModel)
	}
	return out
}

func isReservedModelKey(key string) bool {
	return key == "" || key == "auto" || key == "default"
}

func isFreeView(entries []map[string]any) bool {
	sawOff := false
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		key := strField(entry, "key")
		if isReservedModelKey(key) || freeOf(entry) {
			continue
		}
		if boolVal(entry["enable"]) {
			return false
		}
		sawOff = true
	}
	return sawOff
}

func parseModelInfos(listing map[string]any, site Site) []ModelEntry {
	var source []map[string]any
	if site.ModelScene != "" {
		source = extractRawEntries(listing, site.ModelScene)
	} else {
		source = sceneModels(listing)
	}
	var out []ModelEntry
	for _, raw := range source {
		if raw == nil {
			continue
		}
		key := strField(raw, "key")
		if isReservedModelKey(key) {
			continue
		}
		if site.ModelScene != "" {
			if !site.ListAllModels && !boolVal(raw["enable"]) {
				continue
			}
		} else if !boolVal(raw["enable"]) {
			continue
		}
		out = append(out, parseModelInfo(raw))
	}
	return out
}

func listingKey(cred Credential) string {
	return cred.VPC + "\x00" + cred.UID
}

func (owner *module) fetchListing(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, cred Credential) (map[string]any, error) {
	targetURL := site.API + modelsPath
	hdr, err := owner.cosyHeaders(targetURL, cred, "", site, owner.now().Unix())
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = hdr
	resp, cleanup, err := owner.doHTTP(callCtx, stream, proxyOverride, req)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer resp.Body.Close()
	rawBytes, err := readBodyLimited(resp.Body, 4<<20)
	if err != nil {
		return nil, err
	}
	text := string(rawBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusServiceUnavailable && len(site.FallbackListing) > 0 {
			converted := map[string]any{}
			for scene, list := range site.FallbackListing {
				items := make([]any, len(list))
				for idx, entry := range list {
					items[idx] = entry
				}
				converted[scene] = items
			}
			return converted, nil
		}
		again := ""
		if resp.StatusCode == http.StatusForbidden && site.DeviceRefreshTarget != "" && identityErr.MatchString(text) {
			again = fmt.Sprintf(" — %s didn't recognise this sign-in's identity; sign in to the account again", site.Name)
		}
		return nil, fmt.Errorf("%s models: HTTP %d: %s%s", site.Name, resp.StatusCode, truncateText(strings.TrimSpace(text), 512), again)
	}
	var decoded map[string]any
	if err := json.Unmarshal(rawBytes, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (owner *module) loadModels(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, cred Credential, forceReload bool) (*cachedListing, error) {
	if cred.Lingma {
		return &cachedListing{Models: lingmaModelEntries(), Off: map[string]bool{}}, nil
	}
	key := listingKey(cred)
	if !forceReload {
		state.stateMu.Lock()
		cached := state.listings[key]
		state.stateMu.Unlock()
		if cached != nil {
			return cached, nil
		}
	}
	resolved, err := siteOf(site, cred)
	if err != nil {
		return nil, err
	}
	listing, err := owner.fetchListing(ctx, stream, proxyOverride, resolved, cred)
	if err != nil {
		return nil, err
	}
	var allEntries []map[string]any
	if resolved.ModelScene != "" {
		allEntries = extractRawEntries(listing, resolved.ModelScene)
	} else {
		allEntries = sceneModels(listing)
	}
	off := map[string]bool{}
	state.stateMu.Lock()
	for _, raw := range allEntries {
		modelKey := strField(raw, "key")
		if modelKey != "" {
			state.served[modelKey] = true
			if !boolVal(raw["enable"]) {
				off[modelKey] = true
			}
		}
	}
	parsed := &cachedListing{
		Models:   parseModelInfos(listing, resolved),
		Off:      off,
		FreeView: resolved.ModelScene == "" && isFreeView(sceneModels(listing)),
	}
	state.listings[key] = parsed
	state.stateMu.Unlock()
	return parsed, nil
}

func (state *providerRuntimeState) knowsModel(modelID string) bool {
	state.stateMu.Lock()
	defer state.stateMu.Unlock()
	return state.served[modelID]
}

func (state *providerRuntimeState) consumeUnsaid(cred Credential) bool {
	key := listingKey(cred)
	state.stateMu.Lock()
	defer state.stateMu.Unlock()
	if state.unsaid[key] {
		delete(state.unsaid, key)
		return true
	}
	return false
}

func (state *providerRuntimeState) markUnsaid(cred Credential) {
	state.stateMu.Lock()
	state.unsaid[listingKey(cred)] = true
	state.stateMu.Unlock()
}

func staticToSDKModel(item StaticModel) sdk.Model {
	inputs := []string{"text"}
	if item.Images {
		inputs = []string{"text", "image"}
	}
	efforts := slices.Clone(item.Efforts)
	if efforts == nil {
		efforts = []string{}
	}
	return sdk.Model{
		ID:               item.ID,
		Name:             item.Name,
		Protocols:        []protocolext.Protocol{protocolext.Chat},
		ContextWindow:    item.Context,
		MaxContextWindow: item.Context,
		Tools:            true,
		Vision:           item.Images,
		Reasoning:        len(efforts) > 0,
		Efforts:          efforts,
		InputModalities:  inputs,
		OutputModalities: []string{"text"},
		Temperature:      ptrBool(true),
	}
}

func fallbackSDKModels(site Site) []sdk.Model {
	out := make([]sdk.Model, 0, len(site.Models))
	for _, item := range site.Models {
		out = append(out, staticToSDKModel(item))
	}
	return out
}

func entryToSDKModel(entry ModelEntry) sdk.Model {
	inputs := []string{"text"}
	if entry.Images {
		inputs = []string{"text", "image"}
	}
	efforts := []string{}
	if entry.Thinks && len(entry.Efforts) > 0 {
		efforts = slices.Clone(entry.Efforts)
	}
	return sdk.Model{
		ID:               entry.Key,
		Name:             firstNonEmpty(entry.Name, entry.Key),
		Protocols:        []protocolext.Protocol{protocolext.Chat},
		ContextWindow:    entry.Context,
		MaxContextWindow: entry.Context,
		Tools:            true,
		Vision:           entry.Images,
		Reasoning:        len(efforts) > 0,
		Efforts:          efforts,
		DefaultEffort:    entry.DefaultEffort,
		AlwaysThinks:     entry.AlwaysThinks,
		Free:             entry.Free,
		Rate:             entry.Rate,
		RateWas:          entry.RateWas,
		InputModalities:  inputs,
		OutputModalities: []string{"text"},
		Temperature:      ptrBool(true),
	}
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, state, err := owner.siteFor(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var req sdk.ModelsRequest
	if nonemptyRaw(call.Params) {
		_ = json.Unmarshal(call.Params, &req)
	}
	rawCred := req.Credentials
	if !nonemptyRaw(rawCred) {
		rawCred = call.Auth
	}
	stored, hasCred := decodeCredential(rawCred)
	if !hasCred || stored.Type != "oauth" || stored.Access == "" || stored.UID == "" {
		return sdk.ModelsResult{Models: fallbackSDKModels(site)}, nil
	}

	cred, didRenew, err := owner.fresh(ctx, stream, "", site, state, stored)
	if err != nil {
		var gone *signInGoneError
		if errors.As(err, &gone) && gone.Expired {
			return nil, unauthenticated(gone.Message)
		}
		return sdk.ModelsResult{Models: fallbackSDKModels(site)}, nil
	}
	if didRenew {
		state.markUnsaid(cred)
	}

	cached, err := owner.loadModels(ctx, stream, "", site, state, cred, true)
	if err != nil {
		return sdk.ModelsResult{Models: fallbackSDKModels(site)}, nil
	}
	state.stateMu.Lock()
	alreadyFree := state.stillFree[cred.Access]
	state.stateMu.Unlock()
	if cached.FreeView && !alreadyFree {
		nextCred, renewedAgain, reloginErr := owner.relogin(ctx, stream, "", site, state, stored, cred.Access)
		if reloginErr != nil {
			var gone *signInGoneError
			if errors.As(reloginErr, &gone) && gone.Expired {
				return nil, unauthenticated(gone.Message)
			}
		} else if nextCred.Access != "" && nextCred.Access != cred.Access {
			if renewedAgain {
				state.markUnsaid(nextCred)
			}
			cred = nextCred
			if reloaded, reloadErr := owner.loadModels(ctx, stream, "", site, state, cred, true); reloadErr == nil {
				cached = reloaded
				if cached.FreeView {
					state.stateMu.Lock()
					state.stillFree[cred.Access] = true
					state.stateMu.Unlock()
				}
			}
		}
	}
	if len(cached.Models) == 0 {
		return sdk.ModelsResult{Models: fallbackSDKModels(site)}, nil
	}
	out := make([]sdk.Model, 0, len(cached.Models))
	for _, entry := range cached.Models {
		out = append(out, entryToSDKModel(entry))
	}
	return sdk.ModelsResult{Models: out}, nil
}
