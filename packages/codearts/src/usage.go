package codearts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const totalCreditMetric = "usageTotalPackageCredit"

var (
	creditMetricLabels = map[string]string{
		"usageTotalPackageCredit":    "总积分",
		"usageBasicPackageCredit":    "基础积分包",
		"usageOnDemandPackageCredit": "按需积分包",
		"usageBonusPackageCredit":    "赠送积分包",
	}
	monthlyCreditMetrics = map[string]bool{
		"usageTotalPackageCredit": true,
		"usageBasicPackageCredit": true,
	}
	packageTypeMetrics = map[string]string{
		"basic":    "usageBasicPackageCredit",
		"ondemand": "usageOnDemandPackageCredit",
		"bonus":    "usageBonusPackageCredit",
	}
)

func clampPct(val float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) || val < 0 {
		return 0
	}
	if val > 100 {
		return 100
	}
	return val
}

func formatCredit(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		n = 0
	}
	if n >= 1e8 {
		return fmt.Sprintf("%.1f亿", n/1e8)
	}
	if n >= 1e4 {
		return fmt.Sprintf("%.1f万", n/1e4)
	}
	return strconv.FormatInt(int64(math.Round(n)), 10)
}

func formatCount(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		n = 0
	}
	if n >= 1e9 {
		return fmt.Sprintf("%.1fB", n/1e9)
	}
	if n >= 1e6 {
		return fmt.Sprintf("%.1fM", n/1e6)
	}
	if n >= 1e4 {
		return fmt.Sprintf("%.1fK", n/1e3)
	}
	return strconv.FormatInt(int64(math.Round(n)), 10)
}

func parseVendorDate(raw any) int64 {
	switch typed := raw.(type) {
	case float64:
		if typed <= 0 || math.IsNaN(typed) {
			return 0
		}
		if typed < 1e12 {
			return int64(typed * 1000)
		}
		return int64(typed)
	case string:
		s := strings.TrimSpace(typed)
		if s == "" {
			return 0
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UnixMilli()
			}
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			if f < 1e12 {
				return int64(f * 1000)
			}
			return int64(f)
		}
	}
	return 0
}

func dailyTokenCount(raw any) (float64, bool) {
	val, ok := asFloat(raw)
	if !ok || math.IsNaN(val) || math.IsInf(val, 0) || val < 0 || math.Trunc(val) != val {
		return 0, false
	}
	return val, true
}

func parseBenefitWindow(raw map[string]any, nowMs int64) *sdk.UsageWindow {
	if raw == nil {
		return nil
	}
	if ec, ok := raw["error_code"]; ok && ec != nil {
		s := fmt.Sprint(ec)
		if s != "0000" && s != "0" {
			return nil
		}
	}
	if c, ok := raw["code"]; ok && c != nil {
		s := fmt.Sprint(c)
		if s != "0000" && s != "0" {
			return nil
		}
	}
	rec := raw
	if sub, ok := raw["result"].(map[string]any); ok {
		rec = sub
	} else if sub, ok := raw["data"].(map[string]any); ok {
		rec = sub
	}
	limit, okLim := dailyTokenCount(rec["daily_token_limit"])
	amount, okAmt := dailyTokenCount(rec["daily_tokens_used"])
	if !okLim || limit <= 0 || !okAmt {
		return nil
	}
	used := math.Min(amount, limit)
	usedCopy := used
	return &sdk.UsageWindow{
		Name:     BenefitWindowLabel,
		Used:     clampPct((used / limit) * 100),
		Display:  fmt.Sprintf("%s / %s", formatCredit(used), formatCredit(limit)),
		Amount:   &usedCopy,
		Limit:    limit,
		Unit:     "tokens",
		ResetsAt: nextBeijingMidnight(nowMs),
		Span:     86400,
	}
}

func buildCreditWindow(name string, limit, remain, expiring float64, resetsAt int64) sdk.UsageWindow {
	used := math.Max(0, limit-remain)
	usedCopy := used
	pct := 0.0
	if limit > 0 {
		pct = clampPct((used / limit) * 100)
	}
	display := fmt.Sprintf("%s / %s", formatCredit(used), formatCredit(limit))
	if expiring > 0 {
		display += fmt.Sprintf(" · %s 即将过期", formatCredit(expiring))
	}
	return sdk.UsageWindow{
		Name:     name,
		Used:     pct,
		Display:  display,
		Amount:   &usedCopy,
		Limit:    limit,
		Unit:     "credits",
		ResetsAt: resetsAt,
		Aside:    name != creditMetricLabels[totalCreditMetric],
	}
}

func usageFromStatistics(raw map[string]any, benefit *sdk.UsageWindow, benefitIDs []string, benefitSetKnown bool, station Station) sdk.UsageResult {
	if raw == nil {
		return sdk.UsageResult{Error: "CodeArts could not be parsed"}
	}
	data := raw
	if codeVal, hasCode := asFloat(raw["code"]); hasCode {
		if int(codeVal) != 0 {
			return sdk.UsageResult{Error: firstOf(strField(raw, "message", "msg"), fmt.Sprintf("CodeArts code %d", int(codeVal)))}
		}
		if sub, ok := raw["data"].(map[string]any); ok {
			data = sub
		}
	}
	pkg, _ := data["package"].(map[string]any)
	metrics, _ := data["metrics"].([]any)
	resetsAt := parseVendorDate(data["end_date"])
	startAt := parseVendorDate(data["start_date"])

	var windows []sdk.UsageWindow
	for _, rawItem := range metrics {
		rec, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		metricName := strField(rec, "name")
		label, ok := creditMetricLabels[metricName]
		if !ok {
			continue
		}
		limit, _ := asFloat(rec["package_credit_amount"])
		remain, _ := asFloat(rec["package_credit_remain"])
		if limit <= 0 && remain <= 0 {
			continue
		}
		expiring, _ := asFloat(rec["package_credit_expiring_amount"])
		var rAt int64
		if monthlyCreditMetrics[metricName] {
			rAt = resetsAt
		}
		windows = append(windows, buildCreditWindow(label, limit, remain, expiring, rAt))
	}

	totalLabel := creditMetricLabels[totalCreditMetric]
	slices.SortStableFunc(windows, func(a, b sdk.UsageWindow) int {
		if a.Name == totalLabel && b.Name != totalLabel {
			return -1
		}
		if b.Name == totalLabel && a.Name != totalLabel {
			return 1
		}
		return 0
	})

	if isTok, _ := pkg["is_token_package"].(bool); isTok {
		gated := false
		for _, w := range windows {
			if !w.Aside {
				gated = true
				break
			}
		}
		formatFn := formatCount
		if station.ID == "mainland" {
			formatFn = formatCredit
		}
		tokCount := 0
		for _, rawItem := range metrics {
			rec, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			limit, _ := asFloat(rec["package_token_amount"])
			if limit <= 0 {
				continue
			}
			used, _ := asFloat(rec["usage_token_num"])
			if used < 0 {
				used = 0
			}
			usedCopy := used
			var span int64
			if resetsAt > 0 && startAt > 0 && resetsAt > startAt {
				span = (resetsAt - startAt) / 1000
			}
			windows = append(windows, sdk.UsageWindow{
				Name:     "Tokens",
				Used:     clampPct((used / limit) * 100),
				Display:  fmt.Sprintf("%s / %s", formatFn(used), formatFn(limit)),
				Amount:   &usedCopy,
				Limit:    limit,
				Unit:     "tokens",
				ResetsAt: resetsAt,
				Span:     span,
				Aside:    gated || tokCount > 0,
			})
			tokCount++
		}
	}

	var aliases []string
	if benefitSetKnown {
		for _, id := range benefitIDs {
			aliases = append(aliases, benefitModelID(id))
		}
		slices.Sort(aliases)
		if len(aliases) > 0 {
			for idx := range windows {
				windows[idx].NotModels = append([]string(nil), aliases...)
			}
		}
	}

	var dailyModels []string
	if benefitSetKnown {
		dailyModels = append(dailyModels, benefitIDs...)
		dailyModels = append(dailyModels, aliases...)
	} else {
		dailyModels = append(dailyModels, benefitModelFallback...)
	}
	slices.Sort(dailyModels)

	if benefit != nil && len(dailyModels) > 0 {
		bw := *benefit
		bw.Models = dailyModels
		if !benefitSetKnown {
			bw.Aside = true
		}
		windows = append(windows, bw)
	} else if len(aliases) > 0 {
		windows = append(windows, sdk.UsageWindow{
			Name:    BenefitWindowLabel,
			Models:  dailyModels,
			Display: "用量未知",
			Aside:   true,
		})
	}

	if len(windows) > 0 {
		return sdk.UsageResult{Windows: windows}
	}
	if isTok, _ := pkg["is_token_package"].(bool); isTok {
		return sdk.UsageResult{Error: "该账号无 token 额度"}
	}
	return sdk.UsageResult{Error: "该账号无积分额度"}
}

func (owner *module) codeArtsUsage(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, st *accountState) sdk.UsageResult {
	station := owner.stationOf(&cred)
	at := owner.options.Now().UnixMilli()
	day := beijingDay(at)
	reading := &dailyReadState{Day: day, Fresh: false}
	st.mu.Lock()
	st.dailyRead = reading
	advertisedKnown := st.hasBenefitSet
	advertisedGen := st.benefitModelsGen
	advertised := make([]string, 0, len(st.benefitModels))
	for id := range st.benefitModels {
		advertised = append(advertised, id)
	}
	st.mu.Unlock()

	defer func() {
		st.mu.Lock()
		if st.dailyRead == reading {
			st.dailyRead = nil
			if !reading.Fresh && st.daily != nil && st.daily.Day == reading.Day && st.daily.Fresh {
				stale := *st.daily
				stale.Fresh = false
				st.daily = &stale
			}
		}
		st.mu.Unlock()
	}()

	statsURL := strings.TrimRight(station.Snap, "/") + "/snap-manager/v1/statistics/plugin"
	status, statsText, ok := owner.signedRead(ctx, stream, statsURL, cred, map[string]string{
		"Agent-Type": "PromptCenter",
		"X-Language": "zh-cn",
	})
	if !ok || status < 200 || status >= 300 {
		return sdk.UsageResult{Error: "CodeArts answered an error"}
	}
	var statsObj map[string]any
	_ = json.Unmarshal([]byte(statsText), &statsObj)

	var measured *sdk.UsageWindow
	if station.OpenGW != "" {
		claimed := false
		if len(advertised) > 0 {
			claimed = owner.ensureBenefitClaimed(ctx, stream, cred, st)
			if claimed {
				st.mu.Lock()
				if st.hasBenefitSet && st.benefitModelsGen == advertisedGen {
					active := make(map[string]bool, len(advertised))
					for _, id := range advertised {
						active[id] = true
					}
					st.benefit = active
					st.hasActiveBen = true
				}
				st.mu.Unlock()
			}
		}
		if !advertisedKnown || claimed {
			balURL := strings.TrimRight(station.OpenGW, "/") + "/api/v1/user/tokens/balance"
			bStatus, bText, bOK := owner.signedRead(ctx, stream, balURL, cred, nil)
			if bOK && bStatus >= 200 && bStatus < 300 {
				var bObj map[string]any
				if json.Unmarshal([]byte(bText), &bObj) == nil {
					measured = parseBenefitWindow(bObj, at)
				}
			}
		}
	}

	today := beijingDay(owner.options.Now().UnixMilli())
	st.mu.Lock()
	latest := st.dailyRead == reading && day == today
	var current *dailyReadState
	if st.daily != nil && st.daily.Day == today {
		current = st.daily
	}
	sameCatalog := st.hasBenefitSet == advertisedKnown && st.benefitModelsGen == advertisedGen
	currentKnown := st.hasBenefitSet
	currentBenefitIDs := make([]string, 0, len(st.benefitModels))
	for id := range st.benefitModels {
		currentBenefitIDs = append(currentBenefitIDs, id)
	}
	st.mu.Unlock()

	var confirmed *sdk.UsageWindow
	if latest && sameCatalog {
		confirmed = measured
	}
	var chosen *sdk.UsageWindow
	if confirmed != nil {
		chosen = confirmed
	} else if current != nil {
		chosen = current.Window
	}
	fresh := confirmed != nil || (!latest && current != nil && current.Fresh)
	var displayBenefit *sdk.UsageWindow
	if chosen != nil {
		copied := *chosen
		if !fresh && !strings.HasPrefix(copied.Display, "上次已知：") {
			copied.Display = "上次已知：" + copied.Display
		}
		displayBenefit = &copied
	}

	result := usageFromStatistics(statsObj, displayBenefit, currentBenefitIDs, currentKnown, station)
	if confirmed != nil && result.Error == "" {
		reading.Window = confirmed
		reading.Fresh = true
		st.mu.Lock()
		st.daily = reading
		st.mu.Unlock()
	}
	return result
}

func round2(n float64) float64 {
	return math.Round(n*100) / 100
}

func formatFloatCompact(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func (owner *module) officeAceUsage(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential) sdk.UsageResult {
	if !cred.ValidSTS() {
		return sdk.UsageResult{Error: "OfficeAce 订阅查询需要有效的华为云 STS 凭据"}
	}
	target := strings.TrimRight(owner.options.Endpoints.OASubscriptionBase, "/") + OASubscriptionPath
	status, text, ok := owner.officeAceSigned(ctx, stream, cred, target, http.MethodGet, nil)
	if !ok {
		detail := strings.TrimSpace(text)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return sdk.UsageResult{Error: fmt.Sprintf("订阅查询失败：HTTP %d %s", status, detail)}
	}
	var doc map[string]any
	if json.Unmarshal([]byte(text), &doc) != nil || doc == nil {
		return sdk.UsageResult{Error: "订阅查询失败：响应不是 JSON"}
	}
	nowMs := owner.options.Now().UnixMilli()
	var (
		windows         []sdk.UsageWindow
		aggregateAmount float64
		aggregateLimit  float64
		expiredLimit    float64
	)
	if skus, ok := doc["skus"].([]any); ok {
		for _, rawSKU := range skus {
			sku, ok := rawSKU.(map[string]any)
			if !ok {
				continue
			}
			isTrial := strings.HasSuffix(strField(sku, "sku_code"), ".trial")
			skuName := firstOf(strField(sku, "sku_name"), "套餐")
			quotas, _ := sku["quotas"].([]any)
			for _, rawQ := range quotas {
				q, ok := rawQ.(map[string]any)
				if !ok || strField(q, "sku_attr_code") != "officeace_points" {
					continue
				}
				total, okTotal := asFloat(q["sku_value"])
				if !okTotal || total <= 0 {
					continue
				}
				usedVal, _ := asFloat(q["current_value"])
				amount := math.Min(total, math.Max(0, usedVal))
				remaining := total - amount
				var expiresAt int64
				if !isTrial {
					expiresAt = parseVendorDate(sku["expired_time"])
					if expiresAt == 0 {
						expiresAt = parseVendorDate(q["expired_time"])
					}
				}
				expired := expiresAt > 0 && expiresAt <= nowMs
				if expired {
					expiredLimit += total
				} else {
					aggregateAmount += amount
					aggregateLimit += total
				}
				w := sdk.UsageWindow{
					Name:     fmt.Sprintf("%s · 剩 %s", skuName, formatFloatCompact(round2(remaining))),
					Used:     clampPct(math.Round((amount / total) * 100)),
					Display:  fmt.Sprintf("%s / %s", formatFloatCompact(round2(remaining)), formatFloatCompact(round2(total))),
					Unit:     "积分",
					ResetsAt: expiresAt,
					Aside:    true,
				}
				if !expired {
					amtCopy := amount
					w.Amount = &amtCopy
					w.Limit = total
				}
				windows = append(windows, w)
			}
		}
	}
	if bonuses, ok := doc["bonus_skus"].([]any); ok {
		for _, rawBonus := range bonuses {
			b, ok := rawBonus.(map[string]any)
			if !ok {
				continue
			}
			points, okPts := asFloat(b["points"])
			if !okPts || points <= 0 {
				continue
			}
			usedVal, _ := asFloat(b["current_value"])
			amount := math.Min(points, math.Max(0, usedVal))
			remaining := points - amount
			expiresAt := parseVendorDate(b["expired_time"])
			expired := expiresAt > 0 && expiresAt <= nowMs
			if expired {
				expiredLimit += points
			} else {
				aggregateAmount += amount
				aggregateLimit += points
			}
			w := sdk.UsageWindow{
				Name:     fmt.Sprintf("%s · 剩 %s", firstOf(strField(b, "activity_name"), "活动奖励"), formatFloatCompact(round2(remaining))),
				Used:     clampPct(math.Round((amount / points) * 100)),
				Display:  fmt.Sprintf("%s / %s", formatFloatCompact(round2(remaining)), formatFloatCompact(round2(points))),
				Unit:     "积分",
				ResetsAt: expiresAt,
				Aside:    true,
			}
			if !expired {
				amtCopy := amount
				w.Amount = &amtCopy
				w.Limit = points
			}
			windows = append(windows, w)
		}
	}

	active := aggregateLimit > 0
	rawLimit := expiredLimit
	if active {
		rawLimit = aggregateLimit
	}
	if rawLimit > 0 {
		rawAmount := rawLimit
		if active {
			rawAmount = math.Min(rawLimit, aggregateAmount)
		}
		rawRemaining := math.Max(0, rawLimit-rawAmount)
		roundedLimit := round2(rawLimit)
		limit := roundedLimit
		amount := math.Min(limit, round2(rawAmount))
		remaining := round2(math.Max(0, limit-amount))
		if roundedLimit == 0 {
			limit = rawLimit
			amount = rawAmount
			remaining = rawRemaining
		}
		amtCopy := amount
		totalWin := sdk.UsageWindow{
			Name:    fmt.Sprintf("总积分 · 剩 %s", formatFloatCompact(remaining)),
			Used:    clampPct(math.Round((rawAmount / rawLimit) * 100)),
			Display: fmt.Sprintf("%s / %s", formatFloatCompact(remaining), formatFloatCompact(limit)),
			Unit:    "积分",
			Amount:  &amtCopy,
			Limit:   limit,
		}
		windows = append([]sdk.UsageWindow{totalWin}, windows...)
	}
	return sdk.UsageResult{Windows: windows}
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.UsageRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid usage request")
	}
	if request.Revision != "" {
		if _, err := sdk.ParseUsageRevision(request.Revision); err != nil {
			return nil, invalid("Usage requires a canonical host revision")
		}
	}
	provider := firstOf(call.Scope.Provider, CodeArtsProvider)
	rawCreds := call.Auth
	if !nonempty(rawCreds) {
		rawCreds = request.Credentials
	}
	var cred CodeArtsCredential
	if decodeObject(rawCreds, &cred) != nil {
		return nil, unauthenticated("Missing credentials")
	}
	if provider == OfficeAceProvider {
		result := owner.officeAceUsage(ctx, stream, cred)
		result.Revision = request.Revision
		return result, nil
	}
	if !cred.ValidSTS() {
		return nil, unauthenticated("Invalid CodeArts credentials")
	}
	id, _ := AccountIdentityOf(cred)
	st := owner.stateFor(firstOf(call.Scope.Account, id, cred.AccessKeyID))
	result := owner.codeArtsUsage(ctx, stream, cred, st)
	result.Revision = request.Revision
	return result, nil
}

func (owner *module) snapRequest(ctx context.Context, stream *sdk.Stream, method, targetURL string, cred CodeArtsCredential, payload any) (ok bool, code int, data any, msg string) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return false, 0, nil, err.Error()
		}
	}
	signed, err := SignRequestHuawei(cred.AccessKeyID, cred.SecretAccessKey, cred.SecurityToken, method, targetURL, body, nil, sdkDateStamp(owner.options.Now()))
	if err != nil {
		return false, 0, nil, err.Error()
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, _ := http.NewRequestWithContext(reqCtx, method, targetURL, reader)
	for k, v := range signed {
		if !strings.EqualFold(k, "host") {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("Agent-Type", "PromptCenter")
	req.Header.Set("X-Language", "zh-cn")
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return false, 0, nil, err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, resp.StatusCode, nil, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc == nil {
		return false, 0, nil, "the answer could not be read"
	}
	if codeVal, hasCode := asFloat(doc["code"]); hasCode && int(codeVal) != 0 {
		return false, int(codeVal), nil, firstOf(strField(doc, "message", "msg"), fmt.Sprintf("code %d", int(codeVal)))
	}
	if d, exists := doc["data"]; exists {
		return true, 0, d, ""
	}
	return true, 0, doc, ""
}
