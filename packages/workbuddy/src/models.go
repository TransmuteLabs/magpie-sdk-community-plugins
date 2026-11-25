package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

var (
	cnModels = []staticModelSpec{
		{ID: "auto", Name: "Auto", Context: 168000},
		{ID: "hy4-preview-f", Name: "Hy4 preview", Context: 1000000, Efforts: []string{"high"}},
		{ID: "hy3", Name: "Hy3", Context: 192000, Efforts: []string{"low", "high"}},
		{ID: "hy3-x", Name: "Hy3-X", Context: 192000, Efforts: []string{"low", "high"}},
		{ID: "deepseek-v4.1-flash", Name: "Deepseek-V4.1-Flash", Context: 1000000, Efforts: []string{"none", "low", "high", "max"}, Output: 128000},
		{ID: "glm-5.3", Name: "GLM-5.3", Context: 1000000, Efforts: []string{"low", "high", "max"}},
		{ID: "glm-5.3-flash", Name: "GLM-5.3-Flash", Context: 1000000, Efforts: []string{"low", "high", "max"}, Output: 131072},
		{ID: "glm-5.2", Name: "GLM-5.2", Context: 1000000, Efforts: []string{"high", "xhigh"}},
		{ID: "glm-5.1", Name: "GLM-5.1", Context: 200000},
		{ID: "glm-5v-turbo", Name: "GLM-5v-Turbo", Context: 200000},
		{ID: "minimax-m3", Name: "MiniMax-M3", Context: 512000},
		{ID: "kimi-k3-1", Name: "Kimi-K3", Context: 1000000, Efforts: []string{"low", "high", "xhigh"}},
		{ID: "kimi-k2.7", Name: "Kimi-K2.7-Code", Context: 256000},
		{ID: "kimi-k2.6", Name: "Kimi-K2.6", Context: 256000},
		{ID: "deepseek-v4-pro", Name: "Deepseek-V4-Pro", Context: 1000000, Efforts: []string{"none", "high", "xhigh"}, Output: 128000},
	}

	aiModels = []staticModelSpec{
		{ID: "default-model", Name: "Default", Context: 176000, Output: 24000},
		{ID: "fast-model", Name: "Fast", Context: 200000, Output: 32000},
		{ID: "balanced-model", Name: "Balanced", Context: 256000},
		{ID: "primary-model", Name: "Primary", Context: 272000, Output: 72000},
		{ID: "deep-model", Name: "Deep", Context: 176000},
		{ID: "gpt-5.5", Name: "GPT-5.5", Context: 1000000},
		{ID: "gpt-5.4", Name: "GPT-5.4", Context: 272000},
		{ID: "gpt-5.3-codex", Name: "GPT-5.3-Codex", Context: 272000},
		{ID: "gemini-3.1-pro", Name: "Gemini-3.1-Pro", Context: 400000},
		{ID: "gemini-3.5-flash", Name: "Gemini-3.5-Flash", Context: 1000000, Output: 65536},
		{ID: "glm-5.3", Name: "GLM-5.3", Context: 1000000, Efforts: []string{"low", "high", "max"}},
		{ID: "glm-5.2", Name: "GLM-5.2", Context: 1000000, Efforts: []string{"high", "xhigh"}, Output: 48000},
		{ID: "hy3", Name: "Hy3", Context: 192000, Efforts: []string{"low", "high"}, Output: 64000},
		{ID: "kimi-k3", Name: "Kimi-K3", Context: 1000000, Output: 32000},
		{ID: "kimi-k2.6", Name: "Kimi-K2.6", Context: 256000},
		{ID: "minimax-m3", Name: "MiniMax-M3", Context: 512000},
	}

	offTakenModels = map[string]bool{
		"deepseek-v4.1-flash":    true,
		"deepseek-v4.1-flash-sg": true,
	}

	effortOrder  = []string{"low", "medium", "high", "xhigh", "max"}
	creditSuffix = regexp.MustCompile(`\s+credits?$`)
)

type liveModel struct {
	ID      string
	Name    string
	Context int64
	Output  int64
	Images  bool
	Efforts []string
	Effort  string
	Free    bool
	Rate    float64
	RateWas float64
}

type modelPricing struct {
	Free    bool
	Rate    float64
	RateWas float64
}

func fallbackModels(site Site) []sdk.Model {
	out := make([]sdk.Model, 0, len(site.Models))
	for _, spec := range site.Models {
		hasImages := !textOnly(site, spec.ID)
		inMods := []string{"text"}
		if hasImages {
			inMods = []string{"text", "image"}
		}
		efforts := []string{}
		if len(spec.Efforts) > 0 {
			efforts = slices.Clone(spec.Efforts)
		}
		protos := []protocolext.Protocol{protocolext.Chat}
		if nativeOf(site, spec.ID) {
			protos = []protocolext.Protocol{protocolext.Messages}
		}
		temp := true
		out = append(out, sdk.Model{
			ID:               spec.ID,
			Name:             spec.Name,
			Protocols:        protos,
			ContextWindow:    spec.Context,
			MaxOutput:        spec.Output,
			Tools:            true,
			Vision:           hasImages,
			Reasoning:        len(efforts) > 0,
			Efforts:          efforts,
			InputModalities:  inMods,
			OutputModalities: []string{"text"},
			Temperature:      &temp,
		})
	}
	return out
}

func sdkModelOf(site Site, model liveModel) sdk.Model {
	inMods := []string{"text"}
	if model.Images {
		inMods = []string{"text", "image"}
	}
	efforts := []string{}
	if len(model.Efforts) > 0 {
		efforts = slices.Clone(model.Efforts)
	}
	protos := []protocolext.Protocol{protocolext.Chat}
	if nativeOf(site, model.ID) {
		protos = []protocolext.Protocol{protocolext.Messages}
	}
	temp := true
	return sdk.Model{
		ID:               model.ID,
		Name:             model.Name,
		Protocols:        protos,
		ContextWindow:    model.Context,
		MaxOutput:        model.Output,
		Tools:            true,
		Vision:           model.Images,
		Reasoning:        len(efforts) > 0,
		Efforts:          efforts,
		InputModalities:  inMods,
		OutputModalities: []string{"text"},
		Temperature:      &temp,
		Free:             model.Free,
		Rate:             model.Rate,
		RateWas:          model.RateWas,
		DefaultEffort:    model.Effort,
	}
}

func creditsOf(raw any) (float64, bool) {
	if raw == nil {
		return 0, false
	}
	var text string
	switch typed := raw.(type) {
	case string:
		text = typed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed < 0 {
			return 0, false
		}
		return typed, true
	case int:
		if typed < 0 {
			return 0, false
		}
		return float64(typed), true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return float64(typed), true
	default:
		text = fmt.Sprint(raw)
	}
	cleaned := strings.ToLower(strings.TrimSpace(text))
	if cleaned == "" {
		return 0, false
	}
	cleaned = creditSuffix.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimPrefix(cleaned, "x")
	if cleaned == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func freeCredits(raw any) bool {
	val, ok := creditsOf(raw)
	return ok && val == 0
}

func discountOf(promo map[string]any, base float64, hasBase bool) (float64, bool) {
	disc, _ := promo["discount"].(map[string]any)
	if disc == nil {
		return 0, false
	}
	if rawDC, ok := disc["discountedCredits"]; ok && rawDC != nil {
		trimmed := strings.TrimSpace(fmt.Sprint(rawDC))
		trimmed = strings.TrimRight(trimmed, "xX")
		if val, valid := creditsOf(trimmed); valid {
			return val, true
		}
	}
	if factor, ok := asFloat(disc["factor"]); ok && factor >= 0 && hasBase {
		return base * factor, true
	}
	return 0, false
}

func parseISOMs(raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UnixMilli(), true
		}
	}
	return 0, false
}

func promoRunning(promo map[string]any, nowMs int64) bool {
	if promo == nil {
		return false
	}
	if enabled, ok := promo["enabled"].(bool); ok && !enabled {
		return false
	}
	sched, _ := promo["schedule"].(map[string]any)
	if sched == nil {
		return true
	}
	if fromMs, ok := parseISOMs(strField(sched, "validFrom")); ok && fromMs > nowMs {
		return false
	}
	if untilMs, ok := parseISOMs(strField(sched, "validUntil")); ok && untilMs <= nowMs {
		return false
	}
	return true
}

func promoHasDaily(promo map[string]any) bool {
	sched, _ := promo["schedule"].(map[string]any)
	if sched == nil {
		return false
	}
	daily, ok := sched["daily"].([]any)
	return ok && len(daily) > 0
}

func promoIncludesModel(promo map[string]any, modelID string) bool {
	ids, ok := promo["modelIds"].([]any)
	if !ok {
		return false
	}
	for _, item := range ids {
		if str, isStr := item.(string); isStr && str == modelID {
			return true
		}
	}
	return false
}

func priceOf(modelObj map[string]any, promotions []any, nowMs int64) modelPricing {
	base, hasBase := creditsOf(modelObj["credits"])
	modelID := strField(modelObj, "id")
	var mine []map[string]any
	for _, rawPromo := range promotions {
		promo, ok := rawPromo.(map[string]any)
		if !ok || strField(promo, "kind") != "discount" {
			continue
		}
		if promoIncludesModel(promo, modelID) && promoRunning(promo, nowMs) {
			mine = append(mine, promo)
		}
	}
	hasDaily := false
	for _, promo := range mine {
		if promoHasDaily(promo) {
			hasDaily = true
			break
		}
	}
	if hasDaily {
		if !hasBase {
			return modelPricing{Free: false}
		}
		if base == 0 {
			for _, promo := range mine {
				if promoHasDaily(promo) {
					if discRate, ok := discountOf(promo, base, hasBase); ok && discRate == 0 {
						return modelPricing{Free: false}
					}
				}
			}
		}
		return modelPricing{Free: false, Rate: base}
	}
	var offRates []float64
	for _, promo := range mine {
		if discRate, ok := discountOf(promo, base, hasBase); ok {
			offRates = append(offRates, discRate)
		}
	}
	if len(offRates) > 0 {
		minRate := offRates[0]
		for _, candidate := range offRates[1:] {
			if candidate < minRate {
				minRate = candidate
			}
		}
		res := modelPricing{Free: minRate == 0, Rate: minRate}
		if hasBase && base > minRate {
			res.RateWas = base
		}
		return res
	}
	if !hasBase {
		return modelPricing{}
	}
	return modelPricing{Free: base == 0, Rate: base}
}

func effortsOf(modelObj map[string]any, site Site, modelID string) []string {
	reasoning, _ := modelObj["reasoning"].(map[string]any)
	if reasoning != nil {
		if rawSupported, ok := reasoning["supportedEfforts"].([]any); ok && len(rawSupported) > 0 {
			var said []string
			for _, item := range rawSupported {
				if eff, isStr := item.(string); isStr && eff != "" {
					said = append(said, eff)
				}
			}
			if len(said) > 0 {
				if nativeOf(site, modelID) && !slices.Contains(said, "max") {
					said = append(said, "max")
				}
				return said
			}
		}
	}
	supportsReasoning, _ := modelObj["supportsReasoning"].(bool)
	if !supportsReasoning {
		return nil
	}
	set := map[string]bool{"low": true, "high": true, "max": true}
	if reasoning != nil {
		if own, ok := reasoning["effort"].(string); ok && slices.Contains(effortOrder, own) {
			set[own] = true
		}
	}
	var out []string
	for _, eff := range effortOrder {
		if set[eff] {
			out = append(out, eff)
		}
	}
	return out
}

func defaultEffortOf(modelObj map[string]any, efforts []string) string {
	reasoning, _ := modelObj["reasoning"].(map[string]any)
	if reasoning == nil {
		return ""
	}
	own := strField(reasoning, "defaultEffort", "effort")
	if own != "" && slices.Contains(efforts, own) {
		return own
	}
	return ""
}

func (owner *module) liveModels(ctx context.Context, stream *sdk.Stream, site Site, acct Credential) ([]liveModel, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpointOf(site, acct)+"/v3/config", nil)
	if err != nil {
		return nil, err
	}
	signHeaders(site, acct, req.Header)
	req.Header.Set("User-Agent", cliAgent(site))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json")
	resp, _, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var env map[string]any
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s's config: HTTP %d", site.Name, resp.StatusCode)
	}
	if code := parseIntCode(env["code"]); code != 0 {
		return nil, fmt.Errorf("%s's config: %s (%d)", site.Name, strField(env, "msg"), code)
	}
	cfg, _ := env["data"].(map[string]any)
	if cfg == nil {
		return nil, nil
	}
	rawModels, _ := cfg["models"].([]any)
	modelByID := map[string]map[string]any{}
	var modelObjs []map[string]any
	for _, rawItem := range rawModels {
		obj, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		modelObjs = append(modelObjs, obj)
		if id := strField(obj, "id"); id != "" {
			if _, exists := modelByID[id]; !exists {
				modelByID[id] = obj
			}
		}
	}
	promotions, _ := cfg["modelPromotions"].([]any)
	nowMs := owner.nowMs()
	var out []liveModel
	seen := map[string]bool{}
	addModel := func(id string, obj map[string]any) {
		seen[id] = true
		model := liveModel{ID: id, Name: id}
		if obj != nil {
			if name := strField(obj, "name"); name != "" {
				model.Name = name
			}
			if ctxLen, ok := asInt64(obj["maxInputTokens"]); ok {
				model.Context = ctxLen
			}
			if outLen, ok := asInt64(obj["maxOutputTokens"]); ok {
				model.Output = outLen
			}
			supportsImages, _ := obj["supportsImages"].(bool)
			model.Images = supportsImages && !textOnly(site, id)
			pricing := priceOf(obj, promotions, nowMs)
			model.Free = pricing.Free
			model.Rate = pricing.Rate
			model.RateWas = pricing.RateWas
			effs := effortsOf(obj, site, id)
			if len(effs) > 0 {
				onlyReasoning, _ := obj["onlyReasoning"].(bool)
				canOff := true
				if reasoning, ok := obj["reasoning"].(map[string]any); ok && reasoning != nil {
					if rawCanOff, exists := reasoning["canDisableThinking"]; exists && rawCanOff != nil {
						if bVal, isBool := rawCanOff.(bool); isBool {
							canOff = bVal
						}
					}
				}
				allowOff := (!onlyReasoning && canOff) || offTakenModels[id]
				if allowOff {
					model.Efforts = append([]string{"none"}, effs...)
				} else {
					model.Efforts = slices.Clone(effs)
				}
				model.Effort = defaultEffortOf(obj, effs)
			}
		}
		out = append(out, model)
	}
	foundCLI := false
	rawAgents, _ := cfg["agents"].([]any)
	for _, rawAgent := range rawAgents {
		agent, ok := rawAgent.(map[string]any)
		if !ok || strField(agent, "name") != "cli" {
			continue
		}
		foundCLI = true
		agentModels, _ := agent["models"].([]any)
		for _, rawID := range agentModels {
			id, isStr := rawID.(string)
			if !isStr || id == "" {
				continue
			}
			addModel(id, modelByID[id])
		}
	}
	if foundCLI {
		for _, obj := range modelObjs {
			id, _ := obj["id"].(string)
			if strings.HasPrefix(id, "custom:") && !seen[id] {
				addModel(id, obj)
			}
		}
	}
	firstByName := map[string]string{}
	for idx := range out {
		firstID, exists := firstByName[out[idx].Name]
		if !exists {
			firstByName[out[idx].Name] = out[idx].ID
			continue
		}
		suffix := out[idx].ID
		if strings.HasPrefix(out[idx].ID, firstID+"-") {
			rest := out[idx].ID[len(firstID)+1:]
			if rest != "" {
				suffix = strings.ToUpper(rest)
			}
		}
		out[idx].Name = fmt.Sprintf("%s (%s)", out[idx].Name, suffix)
	}
	return out, nil
}

func listCacheKey(acct Credential) string {
	keyPart := ""
	if acct.Source == "key" {
		keyPart = acct.Key
	}
	return acct.UID + "|" + acct.EnterpriseID + "|" + keyPart
}

func (owner *module) listedModels(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, forceFresh bool) ([]liveModel, error) {
	st := owner.stateFor(site)
	key := listCacheKey(acct)
	now := owner.nowMs()
	if !forceFresh {
		st.mu.Lock()
		cached, ok := st.lists[key]
		st.mu.Unlock()
		if ok && now-cached.at < PriceTTLMs {
			return cached.list, nil
		}
	}
	list, err := owner.liveModels(ctx, stream, site, acct)
	if err != nil {
		return nil, err
	}
	st.mu.Lock()
	if _, exists := st.lists[key]; !exists {
		st.listOrder = append(st.listOrder, key)
	}
	st.lists[key] = modelListCache{at: owner.nowMs(), list: list}
	if len(st.listOrder) > 64 {
		oldest := st.listOrder[0]
		st.listOrder = st.listOrder[1:]
		delete(st.lists, oldest)
	}
	st.mu.Unlock()
	return list, nil
}

func (owner *module) effortList(ctx context.Context, stream *sdk.Stream, site Site, acct Credential) []liveModel {
	if acct.Source == "key" && !remoteConfigKey(acct.Key) {
		return nil
	}
	st := owner.stateFor(site)
	key := listCacheKey(acct)
	now := owner.nowMs()
	st.mu.Lock()
	failedAt, hasFailed := st.unlisted[key]
	st.mu.Unlock()
	if hasFailed && now-failedAt < PriceTTLMs {
		return nil
	}
	list, err := owner.listedModels(ctx, stream, site, acct, false)
	if err != nil {
		st.mu.Lock()
		if _, exists := st.unlisted[key]; !exists {
			st.unlistedOrder = append(st.unlistedOrder, key)
		}
		st.unlisted[key] = owner.nowMs()
		if len(st.unlistedOrder) > 64 {
			oldest := st.unlistedOrder[0]
			st.unlistedOrder = st.unlistedOrder[1:]
			delete(st.unlisted, oldest)
		}
		st.mu.Unlock()
		return nil
	}
	return list
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.ModelsRequest
	if nonempty(call.Params) && decodeObject(call.Params, &req) != nil {
		return nil, invalid("Models request must be a JSON object")
	}
	site, ok := owner.siteFor(call.Scope.Provider)
	if !ok {
		return nil, invalid("Unknown WorkBuddy provider")
	}
	freeOnly := owner.freeOnly(ctx, site)
	rawAuth := call.Auth
	if !nonempty(rawAuth) {
		rawAuth = req.Credentials
	}
	if !nonempty(rawAuth) {
		return sdk.ModelsResult{Models: fallbackModels(site)}, nil
	}
	var cred Credential
	if decodeObject(rawAuth, &cred) != nil {
		if freeOnly {
			return nil, invalid("Invalid WorkBuddy credentials")
		}
		return sdk.ModelsResult{Models: fallbackModels(site)}, nil
	}
	var active Credential
	if cred.isAPIKey() {
		keyed, valid := keyAccount(firstNonEmpty(cred.Key, cred.Access))
		if !valid || !remoteConfigKey(keyed.Key) {
			if freeOnly {
				return nil, upstreamErr(fmt.Sprintf(`%s lists no prices for this key, and "Free models only" is on`, site.Name))
			}
			return sdk.ModelsResult{Models: fallbackModels(site)}, nil
		}
		active = keyed
	} else if cred.isOAuth() {
		var err error
		active, err = owner.current(ctx, stream, site, cred)
		if err != nil {
			if freeOnly {
				return nil, err
			}
			return sdk.ModelsResult{Models: fallbackModels(site)}, nil
		}
	} else {
		return sdk.ModelsResult{Models: fallbackModels(site)}, nil
	}

	list, err := owner.listedModels(ctx, stream, site, active, true)
	if err != nil {
		if freeOnly {
			return nil, upstreamErr(err.Error())
		}
		return sdk.ModelsResult{Models: fallbackModels(site)}, nil
	}
	if len(list) == 0 && !freeOnly {
		return sdk.ModelsResult{Models: fallbackModels(site)}, nil
	}
	if freeOnly {
		var filtered []liveModel
		for _, item := range list {
			if item.Free {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			return nil, upstreamErr(fmt.Sprintf(`%s lists no free model, and "Free models only" is on`, site.Name))
		}
		list = filtered
	}
	result := make([]sdk.Model, 0, len(list))
	for _, item := range list {
		result = append(result, sdkModelOf(site, item))
	}
	return sdk.ModelsResult{Models: result}, nil
}
