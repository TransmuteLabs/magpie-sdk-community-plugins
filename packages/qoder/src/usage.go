package qoder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

var spentInTurnPools = map[string]bool{
	"Credits":        true,
	"Add-on credits": true,
	"Shared credits": true,
}

type usageStatusError struct {
	Status int
}

func (err *usageStatusError) Error() string {
	return fmt.Sprintf("qoder usage: upstream HTTP %d", err.Status)
}

type parsedQoderUsage struct {
	Plan      string
	Until     string
	UntilMs   int64
	Balance   string
	Windows   []sdk.UsageWindow
	ErrorText string
}

func gfmt(val float64) string {
	if val == 0 {
		return "0"
	}
	abs := math.Abs(val)
	if abs < 1e-4 || abs >= 1e6 {
		return strconv.FormatFloat(val, 'e', -1, 64)
	}
	return strconv.FormatFloat(val, 'f', -1, 64)
}

func formatCompactNum(val float64) string {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "0"
	}
	rounded := math.Round(val*100) / 100
	if rounded == 0 {
		return "0"
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

func whenTimestamp(raw any) (int64, string) {
	if raw == nil {
		return 0, ""
	}
	if text, ok := raw.(string); ok {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return 0, ""
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			utc := parsed.UTC()
			return utc.UnixMilli(), utc.Format(time.RFC3339Nano)
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			utc := parsed.UTC()
			return utc.UnixMilli(), utc.Format(time.RFC3339Nano)
		}
		num, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(num) || math.IsInf(num, 0) || num <= 0 {
			return 0, ""
		}
		ms := int64(math.Trunc(num))
		if num < 1e12 {
			ms = int64(math.Trunc(num * 1000))
		}
		return ms, time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
	}
	num, ok := floatField(map[string]any{"v": raw}, "v")
	if !ok || math.IsNaN(num) || math.IsInf(num, 0) || num <= 0 {
		return 0, ""
	}
	ms := int64(math.Trunc(num))
	if num < 1e12 {
		ms = int64(math.Trunc(num * 1000))
	}
	return ms, time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

func isOptionalNumber(raw any) bool {
	if raw == nil {
		return true
	}
	switch typed := raw.(type) {
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case int, int64:
		return true
	case json.Number:
		val, err := typed.Float64()
		return err == nil && !math.IsNaN(val) && !math.IsInf(val, 0)
	default:
		return false
	}
}

func isOptionalString(raw any) bool {
	if raw == nil {
		return true
	}
	_, ok := raw.(string)
	return ok
}

func strictFiniteFloat(raw any) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		val, err := typed.Float64()
		if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
			return 0, false
		}
		return val, true
	default:
		return 0, false
	}
}

func pickField(obj map[string]any, primary, fallback string) any {
	if obj == nil {
		return nil
	}
	if val, exists := obj[primary]; exists {
		return val
	}
	return obj[fallback]
}

func spentOf(env map[string]any) string {
	if env == nil || rawStrField(env, "displayMode") != "qoder" {
		return ""
	}
	usageObj, ok := env["qoderUsage"].(map[string]any)
	if !ok || usageObj == nil {
		return ""
	}
	keys := [][2]string{
		{"userQuota", "user_quota"},
		{"addOnQuota", "add_on_quota"},
		{"orgResourcePackage", "org_resource_package"},
	}
	var usedSum float64
	var totalSum float64
	pools := 0
	for _, pair := range keys {
		poolObj, ok := pickField(usageObj, pair[0], pair[1]).(map[string]any)
		if !ok || poolObj == nil {
			continue
		}
		total, hasTotal := strictFiniteFloat(poolObj["total"])
		if !hasTotal {
			total, hasTotal = strictFiniteFloat(poolObj["cap"])
		}
		if !hasTotal || total < 0 {
			continue
		}
		usedVal, hasUsed := strictFiniteFloat(poolObj["used"])
		if !hasUsed {
			if remVal, hasRem := strictFiniteFloat(poolObj["remaining"]); hasRem {
				usedVal = total - remVal
				hasUsed = true
			}
		}
		if total > 0 && !hasUsed {
			return ""
		}
		pools++
		totalSum += total
		if total > 0 {
			clamped := usedVal
			if clamped < 0 {
				clamped = 0
			}
			if clamped > total {
				clamped = total
			}
			usedSum += clamped
		}
	}
	if pools == 0 || usedSum < totalSum {
		return ""
	}
	return fmt.Sprintf("%s / %s credits", gfmt(usedSum), gfmt(totalSum))
}

func parseUsage(env map[string]any) parsedQoderUsage {
	if env == nil {
		return parsedQoderUsage{ErrorText: "Qoder: missing quota data"}
	}
	if rawStrField(env, "displayMode") == "enterprise" {
		return parsedQoderUsage{Plan: "Enterprise"}
	}
	usageObj, ok := env["qoderUsage"].(map[string]any)
	if rawStrField(env, "displayMode") != "qoder" || !ok || usageObj == nil {
		return parsedQoderUsage{ErrorText: "Qoder: missing quota data"}
	}
	out := parsedQoderUsage{Windows: []sdk.UsageWindow{}}
	if plan, ok := pickField(usageObj, "userType", "user_type").(string); ok {
		out.Plan = plan
	}
	untilMs, untilText := whenTimestamp(pickField(usageObj, "expiresAt", "expires_at"))
	out.UntilMs = untilMs
	out.Until = untilText

	type turnPool struct {
		index int
		used  float64
		total float64
		unit  string
	}
	var turn []turnPool
	addPool := func(raw any, defaultName string) {
		poolObj, ok := raw.(map[string]any)
		if !ok || poolObj == nil {
			return
		}
		if !isOptionalNumber(poolObj["total"]) || !isOptionalNumber(poolObj["cap"]) ||
			!isOptionalNumber(poolObj["used"]) || !isOptionalNumber(poolObj["remaining"]) ||
			!isOptionalString(poolObj["name"]) || !isOptionalString(poolObj["unit"]) {
			return
		}
		total, hasTotal := strictFiniteFloat(poolObj["total"])
		if !hasTotal {
			total, hasTotal = strictFiniteFloat(poolObj["cap"])
		}
		if !hasTotal || total <= 0 {
			return
		}
		used, hasUsed := strictFiniteFloat(poolObj["used"])
		if !hasUsed {
			rem, hasRem := strictFiniteFloat(poolObj["remaining"])
			if !hasRem {
				return
			}
			used = total - rem
		}
		if used < 0 {
			return
		}
		unit := rawStrField(poolObj, "unit")
		if unit == "" {
			unit = "credits"
		}
		name := rawStrField(poolObj, "name")
		if name == "" {
			name = defaultName
		}
		pct := math.Min(100, (100*used)/total)
		win := sdk.UsageWindow{
			Name:    name,
			Used:    pct,
			Display: fmt.Sprintf("%s / %s %s", gfmt(used), gfmt(total), unit),
			Amount:  ptrFloat(used),
			Limit:   total,
			Unit:    unit,
		}
		if defaultName == "Credits" && untilMs > 0 {
			win.ResetsAt = untilMs
		}
		out.Windows = append(out.Windows, win)
		if spentInTurnPools[defaultName] {
			capped := used
			if capped > total {
				capped = total
			}
			turn = append(turn, turnPool{
				index: len(out.Windows) - 1,
				used:  capped,
				total: total,
				unit:  unit,
			})
		}
	}

	addPool(pickField(usageObj, "userQuota", "user_quota"), "Credits")
	addPool(pickField(usageObj, "addOnQuota", "add_on_quota"), "Add-on credits")
	addPool(pickField(usageObj, "orgResourcePackage", "org_resource_package"), "Shared credits")

	if len(turn) > 1 {
		sameUnit := true
		firstUnit := turn[0].unit
		for idx := 1; idx < len(turn); idx++ {
			if turn[idx].unit != firstUnit {
				sameUnit = false
				break
			}
		}
		if sameUnit {
			var sumUsed float64
			var sumTotal float64
			hasReset := false
			for _, item := range turn {
				sumUsed += item.used
				sumTotal += item.total
				out.Windows[item.index].Aside = true
				if out.Windows[item.index].ResetsAt > 0 {
					hasReset = true
				}
			}
			summary := sdk.UsageWindow{
				Name:    "Total credits",
				Used:    math.Min(100, (100*sumUsed)/sumTotal),
				Display: fmt.Sprintf("%s / %s %s", gfmt(sumUsed), gfmt(sumTotal), firstUnit),
				Amount:  ptrFloat(sumUsed),
				Limit:   sumTotal,
				Unit:    firstUnit,
			}
			if untilMs > 0 && hasReset {
				summary.ResetsAt = untilMs
			}
			out.Windows = append([]sdk.UsageWindow{summary}, out.Windows...)
		}
	}

	if dedicated, ok := pickField(usageObj, "dedicatedResourcePackages", "dedicated_resource_packages").([]any); ok {
		for _, item := range dedicated {
			addPool(item, "Dedicated credits")
		}
	}

	if len(out.Windows) == 0 {
		zeroPool := func(raw any) bool {
			poolObj, ok := raw.(map[string]any)
			if !ok || poolObj == nil {
				return false
			}
			total, hasTotal := strictFiniteFloat(poolObj["total"])
			if !hasTotal {
				total, hasTotal = strictFiniteFloat(poolObj["cap"])
			}
			return hasTotal && total == 0
		}
		if zeroPool(pickField(usageObj, "userQuota", "user_quota")) ||
			zeroPool(pickField(usageObj, "addOnQuota", "add_on_quota")) ||
			zeroPool(pickField(usageObj, "orgResourcePackage", "org_resource_package")) {
			out.Windows = append(out.Windows, sdk.UsageWindow{
				Name:    "Credits",
				Used:    100,
				Display: "0 credits left",
			})
		}
	}
	return out
}

func unwrapAdapter(raw map[string]any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	if data, ok := raw["data"].(map[string]any); ok && data != nil {
		return data
	}
	return raw
}

func walletPools(wallets map[string]any) []string {
	data := unwrapAdapter(wallets)
	specs := [][2]string{
		{"daily_credits", "daily"},
		{"monthly_credits", "monthly"},
		{"longterm_credits", "long-term"},
	}
	var out []string
	for _, spec := range specs {
		poolObj, ok := data[spec[0]].(map[string]any)
		if !ok || poolObj == nil {
			continue
		}
		bal, ok := strictFiniteFloat(poolObj["total_balance"])
		if !ok {
			bal, ok = strictFiniteFloat(poolObj["totalBalance"])
		}
		if ok && bal > 0 {
			out = append(out, fmt.Sprintf("%s %s", spec[1], formatCompactNum(bal)))
		}
	}
	return out
}

func parseAccountContext(ctxEnv, walletsEnv map[string]any) parsedQoderUsage {
	data := unwrapAdapter(ctxEnv)
	quotaObj, _ := data["quota"].(map[string]any)
	planObj, _ := data["plan"].(map[string]any)

	out := parsedQoderUsage{}
	if planObj != nil {
		if name := strField(planObj, "name"); name != "" {
			out.Plan = name
		}
		refreshRaw := pickField(planObj, "next_refresh_date", "next_due_date")
		if refreshRaw != nil {
			if text, ok := refreshRaw.(string); ok {
				if parsed, ok := parseJSDate(text); ok {
					out.UntilMs = parsed.UTC().UnixMilli()
					out.Until = parsed.UTC().Format(time.RFC3339Nano)
				}
			} else {
				untilMs, untilText := whenTimestamp(refreshRaw)
				out.UntilMs = untilMs
				out.Until = untilText
			}
		}
	}
	if quotaObj != nil {
		total, hasTotal := strictFiniteFloat(quotaObj["total"])
		remaining, hasRemaining := strictFiniteFloat(quotaObj["remaining"])
		used, hasUsed := strictFiniteFloat(quotaObj["used"])
		if !hasUsed && hasTotal && hasRemaining {
			used = total - remaining
			hasUsed = true
		}
		if hasTotal && total > 0 && hasUsed && used >= 0 {
			win := sdk.UsageWindow{
				Name:    "Credits",
				Used:    math.Min(100, (100*used)/total),
				Display: fmt.Sprintf("%s / %s credits", formatCompactNum(used), formatCompactNum(total)),
				Amount:  ptrFloat(used),
				Limit:   total,
				Unit:    "credits",
			}
			if hasRemaining {
				win.Balance = ptrFloat(remaining)
			}
			if out.UntilMs > 0 {
				win.ResetsAt = out.UntilMs
			}
			out.Windows = append(out.Windows, win)
		}
		if hasRemaining {
			var pools []string
			if walletsEnv != nil {
				pools = walletPools(walletsEnv)
			}
			balanceText := fmt.Sprintf("%s credits", formatCompactNum(remaining))
			if len(pools) > 0 {
				balanceText += fmt.Sprintf(" (%s)", strings.Join(pools, " · "))
			}
			out.Balance = balanceText
			exceeded := boolVal(quotaObj["exceeded"]) ||
				boolVal(quotaObj["is_quota_exceeded"]) ||
				boolVal(quotaObj["isQuotaExceeded"]) ||
				remaining <= 0
			if len(out.Windows) == 0 {
				if exceeded {
					win := sdk.UsageWindow{
						Name:    "Credits",
						Used:    100,
						Display: "no credits left",
						Balance: ptrFloat(remaining),
						Unit:    "credits",
					}
					if out.UntilMs > 0 {
						win.ResetsAt = out.UntilMs
					}
					out.Windows = append(out.Windows, win)
				} else {
					win := sdk.UsageWindow{
						Name:    "Balance",
						Unknown: true,
						Aside:   true,
						Display: balanceText,
						Balance: ptrFloat(remaining),
						Unit:    "credits",
					}
					if out.UntilMs > 0 {
						win.ResetsAt = out.UntilMs
					}
					out.Windows = append(out.Windows, win)
				}
			} else if len(pools) > 0 {
				out.Windows = append(out.Windows, sdk.UsageWindow{
					Name:    "Balance",
					Unknown: true,
					Aside:   true,
					Display: balanceText,
					Balance: ptrFloat(remaining),
					Unit:    "credits",
				})
			}
		}
	}
	if len(out.Windows) == 0 && out.Balance == "" {
		out.ErrorText = "QwenWork: the account reports no credits"
	}
	return out
}

func (owner *module) fetchUsage(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, deviceToken string) (map[string]any, error) {
	if strings.TrimSpace(deviceToken) == "" {
		return nil, errors.New("qoder usage: missing device token")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, site.OpenAPI+usagePath, nil)
	if err != nil {
		return nil, fmt.Errorf("qoder usage: request failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	req.Header.Set("Cosy-ClientType", "10")
	req.Header.Set("User-Agent", "Qoder")
	resp, cleanup, err := owner.doHTTP(reqCtx, stream, proxyOverride, req)
	if err != nil {
		return nil, fmt.Errorf("qoder usage: request failed: %w", err)
	}
	defer cleanup()
	defer resp.Body.Close()
	raw, err := readBodyLimited(resp.Body, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("qoder usage: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &usageStatusError{Status: resp.StatusCode}
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		return nil, fmt.Errorf("qoder usage: decode response: %v", err)
	}
	mode := rawStrField(env, "displayMode")
	if mode != "qoder" && mode != "enterprise" {
		return nil, errors.New("qoder usage: unknown display mode")
	}
	if mode == "qoder" && env["qoderUsage"] == nil {
		return nil, errors.New("qoder usage: missing quota data")
	}
	return env, nil
}

func (owner *module) usageEnv(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, stored Credential, cred Credential) (map[string]any, error) {
	resolved, err := siteOf(site, cred)
	if err != nil {
		return nil, err
	}
	env, err := owner.fetchUsage(ctx, stream, proxyOverride, resolved, cred.DeviceToken)
	if err == nil {
		return env, nil
	}
	var statusErr *usageStatusError
	if !errors.As(err, &statusErr) || (statusErr.Status != http.StatusUnauthorized && statusErr.Status != http.StatusForbidden) {
		return nil, err
	}
	nextCred, rotatedToken, rotErr := owner.rotateDeviceToken(ctx, stream, proxyOverride, site, state, stored, cred.DeviceToken)
	if rotErr != nil {
		return nil, rotErr
	}
	resolvedNext, err := siteOf(site, nextCred)
	if err != nil {
		return nil, err
	}
	return owner.fetchUsage(ctx, stream, proxyOverride, resolvedNext, rotatedToken)
}

func (owner *module) spareFree(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, cred Credential, windows []sdk.UsageWindow) {
	if len(windows) == 0 {
		return
	}
	cached, err := owner.loadModels(ctx, stream, proxyOverride, site, state, cred, false)
	if err != nil || cached == nil {
		return
	}
	var freeIDs []string
	for _, entry := range cached.Models {
		if entry.Free && entry.Key != "" {
			freeIDs = append(freeIDs, entry.Key)
		}
	}
	if len(freeIDs) == 0 {
		return
	}
	for idx := range windows {
		windows[idx].NotModels = slices.Clone(freeIDs)
	}
}

func (owner *module) creditsSpent(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, stored Credential) string {
	if site.UsagePath != "" {
		return ""
	}
	cred, _, err := owner.fresh(ctx, stream, proxyOverride, site, state, stored)
	if err != nil || cred.Lingma {
		return ""
	}
	if cred.PATAuth && cred.DeviceToken == "" {
		resolved, siteErr := siteOf(site, cred)
		if siteErr != nil {
			return ""
		}
		env, fetchErr := owner.fetchUsage(ctx, stream, proxyOverride, resolved, cred.Access)
		if fetchErr != nil {
			return ""
		}
		return spentOf(env)
	}
	env, fetchErr := owner.usageEnv(ctx, stream, proxyOverride, site, state, stored, cred)
	if fetchErr != nil {
		return ""
	}
	return spentOf(env)
}

func (owner *module) readAdapterUsage(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, token, path string) (map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, site.OpenAPI+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if site.QwenWorkHeaders {
		for key, values := range owner.qwenworkClientHeaders() {
			for _, val := range values {
				req.Header.Set(key, val)
			}
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, cleanup, err := owner.doHTTP(reqCtx, stream, proxyOverride, req)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer resp.Body.Close()
	raw, err := readBodyLimited(resp.Body, 4<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &usageStatusError{Status: resp.StatusCode}
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		return nil, fmt.Errorf("qwenwork usage: decode response: %v", err)
	}
	return env, nil
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, state, err := owner.siteFor(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var req sdk.UsageRequest
	if nonemptyRaw(call.Params) && json.Unmarshal(call.Params, &req) != nil {
		return nil, invalid("Invalid Qoder usage request")
	}
	if req.Revision != "" {
		if _, err := sdk.ParseUsageRevision(req.Revision); err != nil {
			return nil, invalid("Usage requires a canonical host revision")
		}
	}
	rawCred := call.Auth
	if !nonemptyRaw(rawCred) {
		rawCred = req.Credentials
	}
	stored, hasCred := decodeCredential(rawCred)
	if !hasCred || stored.Type != "oauth" || stored.Access == "" || stored.UID == "" {
		return sdk.UsageResult{Revision: req.Revision, Error: fmt.Sprintf("%s: not signed in", site.Name)}, nil
	}
	cred, didRenew, err := owner.fresh(ctx, stream, "", site, state, stored)
	if err != nil {
		var gone *signInGoneError
		if errors.As(err, &gone) && gone.Expired {
			return nil, unauthenticated(gone.Message)
		}
		return sdk.UsageResult{Revision: req.Revision, Error: err.Error()}, nil
	}
	_ = didRenew || state.consumeUnsaid(cred)
	if cred.Lingma {
		return sdk.UsageResult{Revision: req.Revision, Windows: []sdk.UsageWindow{}}, nil
	}
	resolved, err := siteOf(site, cred)
	if err != nil {
		return sdk.UsageResult{Revision: req.Revision, Error: err.Error()}, nil
	}

	if site.UsagePath != "" {
		token := cred.DeviceToken
		if token == "" {
			token = cred.Access
		}
		env, readErr := owner.readAdapterUsage(ctx, stream, "", resolved, token, site.UsagePath)
		if readErr != nil {
			var statusErr *usageStatusError
			if errors.As(readErr, &statusErr) && (statusErr.Status == http.StatusUnauthorized || statusErr.Status == http.StatusForbidden) && cred.DeviceToken != "" {
				nextCred, rotatedToken, rotErr := owner.rotateDeviceToken(ctx, stream, "", site, state, stored, cred.DeviceToken)
				if rotErr != nil {
					var gone *signInGoneError
					if errors.As(rotErr, &gone) && gone.Expired {
						return nil, unauthenticated(gone.Message)
					}
					return sdk.UsageResult{Revision: req.Revision, Error: rotErr.Error()}, nil
				}
				token = rotatedToken
				if nextSite, nextErr := siteOf(site, nextCred); nextErr == nil {
					resolved = nextSite
				}
				env, readErr = owner.readAdapterUsage(ctx, stream, "", resolved, token, site.UsagePath)
			}
		}
		if readErr != nil {
			return sdk.UsageResult{Revision: req.Revision, Error: readErr.Error()}, nil
		}
		var wallets map[string]any
		ctxData := unwrapAdapter(env)
		ctxUser, _ := ctxData["user"].(map[string]any)
		isBiz := ctxUser != nil && (boolVal(ctxUser["is_biz"]) || boolVal(ctxUser["isBiz"]))
		if site.WalletsPath != "" && !isBiz {
			if gotWallets, walletErr := owner.readAdapterUsage(ctx, stream, "", resolved, token, site.WalletsPath); walletErr == nil {
				wallets = gotWallets
			}
		}
		parsed := parseAccountContext(env, wallets)
		if parsed.ErrorText != "" {
			return sdk.UsageResult{Revision: req.Revision, Error: parsed.ErrorText}, nil
		}
		return sdk.UsageResult{Revision: req.Revision, Windows: parsed.Windows}, nil
	}

	var env map[string]any
	if cred.PATAuth && cred.DeviceToken == "" {
		env, err = owner.fetchUsage(ctx, stream, "", resolved, cred.Access)
	} else {
		env, err = owner.usageEnv(ctx, stream, "", site, state, stored, cred)
	}
	if err != nil {
		var gone *signInGoneError
		if errors.As(err, &gone) && gone.Expired {
			return nil, unauthenticated(gone.Message)
		}
		return sdk.UsageResult{Revision: req.Revision, Error: err.Error()}, nil
	}
	parsed := parseUsage(env)
	if parsed.ErrorText != "" {
		return sdk.UsageResult{Revision: req.Revision, Error: parsed.ErrorText}, nil
	}
	owner.spareFree(ctx, stream, "", site, state, cred, parsed.Windows)
	return sdk.UsageResult{Revision: req.Revision, Windows: parsed.Windows}, nil
}
