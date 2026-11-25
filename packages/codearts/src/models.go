package codearts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

var (
	benefitModelFallback = []string{
		"glm-5.3-flash",
		"deepseek-v4.1-flash",
		"deepseek-v4-flash-0731",
		"deepseek-v4-pro-0813",
	}
	contextWindows = map[string]int64{
		"GLM-5.2":             202752,
		"glm-5.3-flash":       1048576,
		"deepseek-v4-flash":   1048576,
		"deepseek-v4-pro":     1048576,
		"deepseek-v4.1-flash": 1000000,
	}
	oaUnavailableIDs = map[string]bool{
		"deepseek-v4.1-flash-1017": true,
		"deepseek-v4.1-flash-bnt":  true,
		"deepseek-v4-pro":          true,
	}
	vlModelPattern        = regexp.MustCompile(`(?i)-vl(-|$)`)
	dateSuffixPattern     = regexp.MustCompile(`-\d{4}$`)
	noBenefitChannelRegex = regexp.MustCompile(`(?i)group not found`)
)

func benefitModelID(id string) string {
	return id + BenefitModelSuffix
}

func normalizeModelID(id string) string {
	return dateSuffixPattern.ReplaceAllString(strings.TrimSpace(id), "")
}

func prettifyModelName(id string) string {
	parts := strings.Split(strings.TrimSpace(id), "-")
	for idx, p := range parts {
		switch strings.ToLower(p) {
		case "glm":
			parts[idx] = "GLM"
		case "deepseek":
			parts[idx] = "DeepSeek"
		case "openpangu":
			parts[idx] = "openPangu"
		case "kimi":
			parts[idx] = "Kimi"
		case "flash":
			parts[idx] = "Flash"
		case "pro":
			parts[idx] = "Pro"
		default:
			if len(p) > 0 && p[0] >= 'a' && p[0] <= 'z' && !strings.HasPrefix(p, "v") {
				parts[idx] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
	}
	return strings.Join(parts, " ")
}

func codeArtsFallbackModels() []sdk.Model {
	defs := []struct {
		id, name string
	}{
		{"GLM-5.2", "GLM-5.2"},
		{"GLM-5.1", "GLM-5.1"},
		{"glm-5.3-flash", "GLM-5.3 Flash"},
		{"openpangu-2.0-flash", "openPangu 2.0 Flash"},
		{"openpangu-2.0-pro", "openPangu 2.0 Pro"},
		{"deepseek-v4-flash-0731", "DeepSeek V4 Flash"},
		{"deepseek-v4-pro-0813", "DeepSeek V4 Pro"},
		{"deepseek-v4.1-flash", "DeepSeek V4.1 Flash"},
	}
	out := make([]sdk.Model, 0, len(defs))
	for _, d := range defs {
		out = append(out, buildCodeArtsSDKModel(liveModel{
			ID:      d.id,
			Name:    d.name,
			Context: contextWindows[normalizeModelID(d.id)],
		}))
	}
	return out
}

type oaModelDef struct {
	id      string
	name    string
	context int64
	output  int64
}

var oaFallbackOrder = []oaModelDef{
	{"glm-5.3", "GLM-5.3", 1000000, 128000},
	{"glm-5.3-flash", "GLM-5.3-Flash", 1000000, 128000},
	{"glm-5.2", "GLM-5.2", 1000000, 128000},
	{"glm-5.1", "GLM-5.1", 1000000, 128000},
	{"deepseek-v4.1-flash", "DeepSeek-V4.1-Flash", 1000000, 128000},
	{"deepseek-v4-flash-0731", "DeepSeek-V4-Flash-0731", 1000000, 128000},
	{"kimi-k3", "Kimi-K3", 1000000, 128000},
	{"kimi-k3.1-pro-highspeed", "Kimi-K3.1-Pro-HighSpeed", 1000000, 128000},
	{"openpangu-2.1-pro", "openPangu-2.1-Pro", 1000000, 128000},
}

func officeAceFallbackByID(id string) (oaModelDef, bool) {
	for _, d := range oaFallbackOrder {
		if d.id == id {
			return d, true
		}
	}
	return oaModelDef{}, false
}

func officeAceStaticModels() []sdk.Model {
	out := make([]sdk.Model, 0, len(oaFallbackOrder))
	for _, d := range oaFallbackOrder {
		out = append(out, sdk.Model{
			ID:               d.id,
			Name:             d.name,
			Protocols:        []protocolext.Protocol{protocolext.Chat},
			ContextWindow:    d.context,
			MaxOutput:        d.output,
			Tools:            true,
			Reasoning:        true,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		})
	}
	return out
}

type liveModel struct {
	ID        string
	Name      string
	Context   int64
	Output    int64
	Rate      float64
	Reasoning bool
	Benefit   bool
	Own       bool
	Hidden    bool
}

func buildCodeArtsSDKModel(m liveModel) sdk.Model {
	ctxLen := m.Context
	if ctxLen <= 0 {
		ctxLen = contextWindows[normalizeModelID(m.ID)]
	}
	if ctxLen <= 0 {
		ctxLen = 128000
	}
	maxOut := m.Output
	if maxOut <= 0 || maxOut > MaxOutputTokens {
		maxOut = MaxOutputTokens
	}
	efforts := ReasoningLevelsOf(m.ID)
	reasoning := m.Reasoning || len(efforts) > 0
	return sdk.Model{
		ID:               m.ID,
		Name:             firstOf(m.Name, prettifyModelName(m.ID)),
		Protocols:        []protocolext.Protocol{protocolext.Chat},
		ContextWindow:    ctxLen,
		MaxOutput:        maxOut,
		Tools:            true,
		Vision:           false,
		Reasoning:        reasoning,
		Efforts:          efforts,
		Rate:             m.Rate,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
	}
}

func (owner *module) signedRead(ctx context.Context, stream *sdk.Stream, targetURL string, cred CodeArtsCredential, extraUnsigned map[string]string) (status int, text string, ok bool) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	signed, err := SignRequestHuawei(cred.AccessKeyID, cred.SecretAccessKey, cred.SecurityToken, http.MethodGet, targetURL, nil, nil, sdkDateStamp(owner.options.Now()))
	if err != nil {
		return 0, "", false
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, "", false
	}
	for k, v := range signed {
		if !strings.EqualFold(k, "host") {
			req.Header.Set(k, v)
		}
	}
	for k, v := range extraUnsigned {
		req.Header.Set(k, v)
	}
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return 0, "", false
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return resp.StatusCode, "", false
	}
	return resp.StatusCode, string(raw), true
}

func isNoBenefitChannelText(text string) bool {
	var obj map[string]any
	if json.Unmarshal([]byte(text), &obj) != nil || obj == nil {
		return false
	}
	if fmt.Sprint(obj["error_code"]) == "4000" {
		return true
	}
	return noBenefitChannelRegex.MatchString(strField(obj, "error_msg", "message"))
}

func (owner *module) claimBenefit(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, station Station) (ok bool, networkErr bool, code, msg string) {
	if station.OpenGW == "" {
		return false, false, "no_opengw", "station has no opengw"
	}
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	target := strings.TrimRight(station.OpenGW, "/") + "/api/v1/benefit/claim"
	body := []byte("{}")
	signed, err := SignRequestHuawei(cred.AccessKeyID, cred.SecretAccessKey, cred.SecurityToken, http.MethodPost, target, body, nil, sdkDateStamp(owner.options.Now()))
	if err != nil {
		return false, true, "", err.Error()
	}
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(body))
	for k, v := range signed {
		if !strings.EqualFold(k, "host") {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("x-snap-traceid", randomHex(16))
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return false, true, "", err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	errCode := strField(data, "error_code")
	if errCode == "" {
		errCode = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	errMsg := strField(data, "error_msg", "message")
	return resp.StatusCode >= 200 && resp.StatusCode < 300 && errCode == BenefitOK, false, errCode, errMsg
}

func (owner *module) ensureBenefitClaimed(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, st *accountState) bool {
	station := owner.stationOf(&cred)
	if station.OpenGW == "" {
		return false
	}
	nowMs := owner.options.Now().UnixMilli()
	day := beijingDay(nowMs)
	st.mu.Lock()
	if st.claim != nil && st.claim.Day == day && (st.claim.OK || nowMs-st.claim.At < ClaimRetryMs) {
		ok := st.claim.OK
		st.mu.Unlock()
		return ok
	}
	st.mu.Unlock()

	ok, netErr, code, msg := owner.claimBenefit(ctx, stream, cred, station)
	checked := owner.options.Now().UnixMilli()
	if netErr || beijingDay(checked) != day {
		return false
	}
	st.mu.Lock()
	st.claim = &claimState{Day: day, OK: ok, At: checked, Code: code, Message: msg}
	if ok {
		st.noBenefit = map[string]bool{}
		st.aliasRefusals = map[string]bool{}
	}
	st.mu.Unlock()
	return ok
}

func parseModelItem(item map[string]any, seen map[string]bool) *liveModel {
	id := firstOf(strField(item, "model_id", "id", "modelId"))
	if id == "" || seen[id] || vlModelPattern.MatchString(normalizeModelID(id)) {
		return nil
	}
	seen[id] = true
	ctxLen := int64(0)
	if f, ok := asFloat(item["context_window"]); ok && f > 0 {
		ctxLen = int64(f)
	} else if f, ok := asFloat(item["max_context_length"]); ok && f > 0 {
		ctxLen = int64(f)
	}
	maxOut := int64(0)
	if f, ok := asFloat(item["max_tokens"]); ok && f > 0 {
		maxOut = int64(f)
	}
	return &liveModel{
		ID:      id,
		Name:    firstOf(strField(item, "display_name", "model_name", "name"), prettifyModelName(id)),
		Context: ctxLen,
		Output:  maxOut,
	}
}

func extractJSONArray(text string, path ...string) ([]any, bool) {
	var root any
	if json.Unmarshal([]byte(text), &root) != nil {
		return nil, false
	}
	cur := root
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = obj[key]
	}
	arr, ok := cur.([]any)
	return arr, ok
}

func creditRateOf(raw any) float64 {
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if obj, ok := item.(map[string]any); ok {
				if r := creditRateOf(obj); r > 0 {
					return r
				}
			}
		}
	case map[string]any:
		if s := strField(typed, "ratio_display"); s != "" {
			cleaned := strings.TrimSuffix(strings.TrimSpace(s), "x")
			if f, err := strconv.ParseFloat(cleaned, 64); err == nil && f > 0 {
				return f
			}
		}
		if f, ok := asFloat(typed["ratio"]); ok && f > 0 {
			return f
		}
	case float64:
		if typed > 0 {
			return typed
		}
	}
	return 0
}

func (owner *module) fetchAgentModels(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, st *accountState) []liveModel {
	station := owner.stationOf(&cred)
	agentHeaders := map[string]string{
		"Content-Type": "application/json",
		"Agent-Type":   "PromptCenter",
		"X-Language":   "zh-cn",
	}
	readDetail := func(agentID string) []liveModel {
		detailURL := strings.TrimRight(station.Snap, "/") + "/v1/agent-center/agents/detail?agent_id=" + url.QueryEscape(agentID)
		status, text, ok := owner.signedRead(ctx, stream, detailURL, cred, agentHeaders)
		if !ok || status < 200 || status >= 300 {
			return nil
		}
		arr, found := extractJSONArray(text, "gpts", "models")
		if !found {
			arr, found = extractJSONArray(text, "data", "gpts", "models")
		}
		if !found || len(arr) == 0 {
			return nil
		}
		var out []liveModel
		for _, rawItem := range arr {
			m, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			params, _ := m["model_parameters"].(map[string]any)
			id := firstOf(strField(params, "model_id"), strField(m, "model_id"))
			if id == "" {
				continue
			}
			ctxVal, _ := asFloat(params["context_window"])
			outVal, _ := asFloat(params["max_tokens"])
			rate := creditRateOf(m["credit"])
			if rate == 0 && params != nil {
				rate = creditRateOf(params["credit"])
			}
			own := strField(m, "model_type") == "third" || strField(m, "model_type") == "custom"
			if b, ok := m["is_custom_model"].(bool); ok && b {
				own = true
			}
			hidden := false
			if b, ok := m["display_enabled"].(bool); ok && !b {
				hidden = true
			}
			if params != nil {
				if b, ok := params["display_enabled"].(bool); ok && !b {
					hidden = true
				}
			}
			out = append(out, liveModel{
				ID:        id,
				Name:      firstOf(strField(m, "display_name", "model_name")),
				Rate:      rate,
				Reasoning: params != nil && params["thinking_type"] != nil,
				Context:   int64(ctxVal),
				Output:    min(int64(outVal), MaxOutputTokens),
				Own:       own,
				Hidden:    hidden,
			})
		}
		return out
	}

	st.mu.Lock()
	knownID := st.agentID
	st.mu.Unlock()
	if knownID != "" {
		if list := readDetail(knownID); len(list) > 0 {
			return list
		}
		st.mu.Lock()
		st.agentID = ""
		st.mu.Unlock()
	}
	listURL := strings.TrimRight(station.Snap, "/") + "/v1/agent-center/agents/useragents?offset=0&limit=100"
	var candidates []string
	if status, text, ok := owner.signedRead(ctx, stream, listURL, cred, agentHeaders); ok && status >= 200 && status < 300 {
		arr, found := extractJSONArray(text, "agents")
		if !found {
			arr, _ = extractJSONArray(text, "data", "agents")
		}
		type cand struct {
			id   string
			rank int
		}
		var ranked []cand
		for _, raw := range arr {
			obj, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			aid := strField(obj, "agent_id")
			if aid == "" {
				continue
			}
			orig := firstOf(strField(obj, "original_id"), aid)
			rank := 2
			if orig == "build" {
				rank = 0
			} else if orig == "CodeAgent" {
				rank = 1
			}
			ranked = append(ranked, cand{id: aid, rank: rank})
		}
		slices.SortFunc(ranked, func(a, b cand) int { return a.rank - b.rank })
		for _, c := range ranked {
			if !slices.Contains(candidates, c.id) {
				candidates = append(candidates, c.id)
			}
		}
	}
	if !slices.Contains(candidates, MainAgentFallbackID) {
		candidates = append(candidates, MainAgentFallbackID)
	}
	for i := 0; i < len(candidates) && i < 3; i++ {
		if list := readDetail(candidates[i]); len(list) > 0 {
			st.mu.Lock()
			st.agentID = candidates[i]
			st.mu.Unlock()
			return list
		}
	}
	return nil
}

func (owner *module) fetchCodeArtsModels(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, st *accountState) []sdk.Model {
	station := owner.stationOf(&cred)
	var models []liveModel
	seen := map[string]bool{}
	var benefitIDs []string
	var gatewayKnown bool
	var gatewayList []any

	if station.OpenGW == "" {
		gatewayKnown = true
	} else {
		gwURL := strings.TrimRight(station.OpenGW, "/") + "/api/v1/gateway/config"
		status, text, reached := owner.signedRead(ctx, stream, gwURL, cred, nil)
		if reached {
			if isNoBenefitChannelText(text) {
				gatewayKnown = true
			} else if status >= 200 && status < 300 {
				if arr, ok := extractJSONArray(text, "result", "models"); ok {
					gatewayKnown = true
					gatewayList = arr
				}
			}
		}
	}

	claimed := false
	if len(gatewayList) > 0 {
		claimed = owner.ensureBenefitClaimed(ctx, stream, cred, st)
	}
	for _, rawItem := range gatewayList {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		mi := parseModelItem(item, seen)
		if mi == nil {
			continue
		}
		mi.Benefit = true
		benefitIDs = append(benefitIDs, mi.ID)
		if claimed {
			models = append(models, *mi)
		}
	}
	if !claimed {
		seen = map[string]bool{}
	}

	snapURL := strings.TrimRight(station.Snap, "/") + "/v1/model/builtin"
	snapStatus, snapText, snapOK := owner.signedRead(ctx, stream, snapURL, cred, map[string]string{
		"Content-Type": "application/json",
		"Agent-Type":   "PromptCenter",
		"X-Language":   "zh-cn",
	})
	var regularIDs []string
	builtinKnown := false
	if snapOK && snapStatus >= 200 && snapStatus < 300 {
		if builtin, ok := extractJSONArray(snapText, "builtinModels"); ok {
			builtinKnown = true
			for _, rawItem := range builtin {
				item, ok := rawItem.(map[string]any)
				if !ok {
					continue
				}
				if mid := strField(item, "model_id"); mid != "" {
					regularIDs = append(regularIDs, mid)
				}
				if mi := parseModelItem(item, seen); mi != nil {
					models = append(models, *mi)
				}
			}
		}
	}

	agentModels := owner.fetchAgentModels(ctx, stream, cred, st)
	for _, am := range agentModels {
		foundIdx := -1
		for idx := range models {
			if models[idx].ID == am.ID {
				foundIdx = idx
				break
			}
		}
		if foundIdx >= 0 {
			if am.Rate > 0 && (!models[foundIdx].Benefit || slices.Contains(regularIDs, am.ID)) {
				models[foundIdx].Rate = am.Rate
			}
			if am.Reasoning {
				models[foundIdx].Reasoning = true
			}
			if am.Context > 0 {
				models[foundIdx].Context = am.Context
			}
			if am.Output > 0 {
				models[foundIdx].Output = am.Output
			}
		} else if !am.Own && !am.Hidden && !vlModelPattern.MatchString(normalizeModelID(am.ID)) {
			models = append(models, am)
			regularIDs = append(regularIDs, am.ID)
		}
	}

	st.mu.Lock()
	if gatewayKnown {
		bSet := make(map[string]bool, len(benefitIDs))
		for _, id := range benefitIDs {
			bSet[id] = true
		}
		st.benefitModels = bSet
		st.benefitModelsGen++
		st.hasBenefitSet = true
		if claimed {
			st.benefit = bSet
		} else {
			st.benefit = map[string]bool{}
		}
		st.hasActiveBen = true
	}
	if builtinKnown {
		rSet := make(map[string]bool, len(regularIDs))
		for _, id := range regularIDs {
			rSet[id] = true
		}
		st.regular = rSet
		st.hasRegular = true
	}
	day := beijingDay(owner.options.Now().UnixMilli())
	canListAliases := st.claim != nil && st.claim.Day == day && st.claim.OK
	st.mu.Unlock()

	if len(models) == 0 {
		return codeArtsFallbackModels()
	}
	out := make([]sdk.Model, 0, len(models)+len(benefitIDs))
	for _, live := range models {
		auto := buildCodeArtsSDKModel(live)
		out = append(out, auto)
		if canListAliases && live.Benefit {
			alias := auto
			alias.ID = benefitModelID(live.ID)
			alias.Name = auto.Name + " (Benefit)"
			alias.Free = true
			alias.Rate = 0
			out = append(out, alias)
		}
	}
	return out
}

func (owner *module) officeAceSigned(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, targetURL, method string, body []byte) (status int, text string, ok bool) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	headers, err := SignOfficeAceV11(targetURL, cred.AccessKeyID, cred.SecretAccessKey, cred.SecurityToken, cred.ProjectID, method, body, sdkDateStamp(owner.options.Now()))
	if err != nil {
		return 0, err.Error(), false
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, targetURL, reader)
	if err != nil {
		return 0, err.Error(), false
	}
	for k, v := range headers {
		if !strings.EqualFold(k, "host") {
			req.Header.Set(k, v)
		}
	}
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return 0, err.Error(), false
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	return resp.StatusCode, string(raw), resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (owner *module) fetchOfficeAceRates(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential) map[string]float64 {
	id, _ := AccountIdentityOf(cred)
	key := firstOf(id, cred.SessionID, cred.AccessKeyID)
	nowMs := owner.options.Now().UnixMilli()
	owner.mu.Lock()
	hit, hasHit := owner.oaRates[key]
	owner.mu.Unlock()
	if hasHit && len(hit.rates) > 0 && nowMs-hit.at < OARatesTTLMs {
		return hit.rates
	}
	readOnce := func() map[string]float64 {
		target := strings.TrimRight(owner.options.Endpoints.OASubscriptionBase, "/") + OATokenRatesPath
		_, text, ok := owner.officeAceSigned(ctx, stream, cred, target, http.MethodGet, nil)
		if !ok {
			return nil
		}
		var root any
		if json.Unmarshal([]byte(text), &root) != nil {
			return nil
		}
		var rows []any
		switch typed := root.(type) {
		case []any:
			rows = typed
		case map[string]any:
			for _, field := range []string{"models", "data", "rates"} {
				if arr, ok := typed[field].([]any); ok {
					rows = arr
					break
				}
			}
		}
		rates := map[string]float64{}
		for _, rawRow := range rows {
			row, ok := rawRow.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(firstOf(strField(row, "model_name", "modelName", "model", "name")))
			rate, hasRate := asFloat(row["cost_coefficient"])
			if !hasRate {
				rate, hasRate = asFloat(row["costCoefficient"])
			}
			if !hasRate {
				rate, hasRate = asFloat(row["rate"])
			}
			if name != "" && hasRate && !math.IsNaN(rate) && !math.IsInf(rate, 0) {
				rates[name] = rate
			}
		}
		return rates
	}
	tries := 2
	if hasHit {
		tries = 1
	}
	var rates map[string]float64
	for i := 0; i < tries && len(rates) == 0; i++ {
		rates = readOnce()
	}
	if len(rates) == 0 {
		if hasHit {
			owner.mu.Lock()
			owner.oaRates[key] = oaRatesEntry{at: nowMs, rates: hit.rates}
			owner.mu.Unlock()
			return hit.rates
		}
		return map[string]float64{}
	}
	owner.mu.Lock()
	owner.oaRates[key] = oaRatesEntry{at: nowMs, rates: rates}
	owner.mu.Unlock()
	return rates
}

func (owner *module) fetchOfficeAceModels(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential) []sdk.Model {
	if cred.ModelAppKey == "" || cred.ModelAppSecret == "" {
		return officeAceStaticModels()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	target := owner.officeAceMaaSBase(cred) + OAModelsPath
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", cred.BasicAuth())
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return officeAceStaticModels()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return officeAceStaticModels()
	}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return officeAceStaticModels()
	}
	var rows []any
	switch typed := root.(type) {
	case []any:
		rows = typed
	case map[string]any:
		if arr, ok := typed["data"].([]any); ok {
			rows = arr
		}
	}
	rates := owner.fetchOfficeAceRates(ctx, stream, cred)
	var out []sdk.Model
	for _, rawRow := range rows {
		var id, name string
		switch typed := rawRow.(type) {
		case string:
			id = strings.TrimSpace(typed)
			name = id
		case map[string]any:
			id = strings.TrimSpace(strField(typed, "id"))
			name = firstOf(strField(typed, "name"), id)
		}
		if id == "" || oaUnavailableIDs[id] {
			continue
		}
		if len(rates) > 0 {
			if _, ok := rates[id]; !ok {
				continue
			}
		}
		ctxLen, maxOut := int64(1000000), int64(128000)
		if known, ok := officeAceFallbackByID(id); ok {
			ctxLen, maxOut = known.context, known.output
		}
		rate, hasRate := rates[id]
		display := name
		if hasRate && rate != 1 {
			display = fmt.Sprintf("%s · x%s", name, strconv.FormatFloat(rate, 'f', -1, 64))
		}
		model := sdk.Model{
			ID:               id,
			Name:             display,
			Protocols:        []protocolext.Protocol{protocolext.Chat},
			ContextWindow:    ctxLen,
			MaxOutput:        maxOut,
			Tools:            true,
			Reasoning:        true,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		}
		if hasRate {
			model.Rate = rate
		}
		out = append(out, model)
	}
	if len(out) == 0 {
		return officeAceStaticModels()
	}
	return out
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ModelsRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid models request")
	}
	provider := firstOf(call.Scope.Provider, CodeArtsProvider)
	rawCreds := call.Auth
	if !nonempty(rawCreds) {
		rawCreds = request.Credentials
	}
	var cred CodeArtsCredential
	if !nonempty(rawCreds) || decodeObject(rawCreds, &cred) != nil {
		if provider == OfficeAceProvider {
			return sdk.ModelsResult{Models: officeAceStaticModels()}, nil
		}
		return sdk.ModelsResult{Models: codeArtsFallbackModels()}, nil
	}
	if provider == OfficeAceProvider {
		return sdk.ModelsResult{Models: owner.fetchOfficeAceModels(ctx, stream, cred)}, nil
	}
	if !cred.ValidSTS() {
		return sdk.ModelsResult{Models: codeArtsFallbackModels()}, nil
	}
	id, _ := AccountIdentityOf(cred)
	st := owner.stateFor(firstOf(call.Scope.Account, id, cred.AccessKeyID))
	return sdk.ModelsResult{Models: owner.fetchCodeArtsModels(ctx, stream, cred, st)}, nil
}
