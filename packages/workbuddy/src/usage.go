package workbuddy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	dailyCode  = "TCACA_code_001_PqouKr6QWV"
	freeMon    = "TCACA_code_008_cfWoLwvjU4"
	sliceType  = "4"
	summaryURI = "/billing/meter/get-user-resource-summary"
)

var (
	paidCodes = []string{
		"TCACA_code_002_AkiJS3ZHF5",
		"TCACA_code_005_maRGyrHhw1",
		"TCACA_code_003_FAnt7lcmRT",
		"TCACA_code_023_4xbGhMrE6q",
		"TCACA_code_026_BaESVICNoi",
		"TCACA_code_027_0FCGVA6vSa",
		"TCACA_code_043_6oIkxT7Z0S",
		"TCACA_code_009_0XmEQc2xOf",
		"TCACA_code_038_OhvqZtiPKr",
		"TCACA_code_036_lupO5WgNdG",
	}
	freeCodes = []string{
		dailyCode,
		freeMon,
		"TCACA_code_035_ArVxJcGDsm",
		"TCACA_code_006_DbXS0lrypC",
		"TCACA_code_039_KRcQj7wUat",
		"TCACA_code_040_mi9rCYg46x",
		"TCACA_code_007_nzdH5h4Nl0",
		"TCACA_code_028_NtpWi0jzXs",
		"TCACA_code_037_WxOD3MpI2o",
		"TCACA_code_029_6wCGEWquYy",
		"TCACA_code_030_BjSt89qTvr",
	}
	planBaseCodes = []string{
		"TCACA_code_002_AkiJS3ZHF5",
		"TCACA_code_005_maRGyrHhw1",
		"TCACA_code_003_FAnt7lcmRT",
		"TCACA_code_023_4xbGhMrE6q",
		"TCACA_code_026_BaESVICNoi",
		"TCACA_code_027_0FCGVA6vSa",
		"TCACA_code_043_6oIkxT7Z0S",
		"TCACA_code_035_ArVxJcGDsm",
		"TCACA_code_039_KRcQj7wUat",
		"TCACA_code_040_mi9rCYg46x",
	}
	extraCodes = []string{
		"TCACA_code_028_NtpWi0jzXs",
		"TCACA_code_037_WxOD3MpI2o",
		"TCACA_code_009_0XmEQc2xOf",
		"TCACA_code_038_OhvqZtiPKr",
		"TCACA_code_036_lupO5WgNdG",
		"TCACA_code_007_nzdH5h4Nl0",
		"TCACA_code_029_6wCGEWquYy",
		"TCACA_code_030_BjSt89qTvr",
	}
	extraAICodes = []string{
		freeMon,
		"TCACA_code_006_DbXS0lrypC",
	}
	beijingDateTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
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

func capacity(raw any) (float64, error) {
	if raw == nil {
		return 0, nil
	}
	switch typed := raw.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, fmt.Errorf("invalid capacity %v", typed)
		}
		return typed, nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case string:
		if typed == "" {
			return 0, nil
		}
		if strings.TrimSpace(typed) != typed {
			return 0, fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", typed)
		}
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", typed)
		}
		return parsed, nil
	default:
		text := fmt.Sprint(raw)
		if text == "" {
			return 0, nil
		}
		if strings.TrimSpace(text) != text {
			return 0, fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", text)
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", text)
		}
		return parsed, nil
	}
}

func compactFloat(val float64) string {
	if math.Trunc(val) == val && !math.IsInf(val, 0) && !math.IsNaN(val) {
		return strconv.FormatInt(int64(val), 10)
	}
	formatted := fmt.Sprintf("%.2f", val)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	return formatted
}

func beijingDate(ms int64) string {
	t := time.UnixMilli(ms).UTC().Add(8 * time.Hour)
	return t.Format("2006-01-02")
}

func parseBeijingDateTime(raw any) int64 {
	str, ok := raw.(string)
	if !ok {
		return 0
	}
	trimmed := strings.TrimSpace(str)
	if !beijingDateTimeRe.MatchString(trimmed) {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339, strings.Replace(trimmed, " ", "T", 1)+"+08:00")
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func parseFlexibleTime(raw any) int64 {
	str, ok := raw.(string)
	if !ok {
		return 0
	}
	trimmed := strings.TrimSpace(str)
	if trimmed == "" {
		return 0
	}
	if beijingDateTimeRe.MatchString(trimmed) {
		return parseBeijingDateTime(trimmed)
	}
	if ms, ok := parseISOMs(trimmed); ok && ms > 0 {
		return ms
	}
	return 0
}

func planCodesMatcher(sum map[string]any, site Site) func(string) bool {
	sub := strField(sum, "SubscriptionPackageCode")
	return func(code string) bool {
		if code == "" {
			return false
		}
		if code == sub || slices.Contains(planBaseCodes, code) {
			return true
		}
		return !site.AI && code == freeMon
	}
}

type creditBucket struct {
	Total float64
	Used  float64
}

type summaryKinds struct {
	Plan  creditBucket
	Extra creditBucket
	Left  float64
	Bags  int
}

func hasTotalField(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	raw, exists := obj["CycleTotalCapacity"]
	if !exists || raw == nil {
		return false
	}
	if str, isStr := raw.(string); isStr && str == "" {
		return false
	}
	return true
}

func hasRemainField(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	raw, exists := obj["CycleRemainCapacity"]
	if !exists || raw == nil {
		return false
	}
	if str, isStr := raw.(string); isStr && str == "" {
		return false
	}
	return true
}

func kindsOf(sum map[string]any, site Site) (summaryKinds, error) {
	ofPlan := planCodesMatcher(sum, site)
	var kinds summaryKinds
	addBucket := func(dst *creditBucket, item map[string]any) error {
		tot, err := capacity(item["CycleTotalCapacity"])
		if err != nil {
			return err
		}
		used, err := capacity(item["CycleUsedCapacity"])
		if err != nil {
			return err
		}
		dst.Total += tot
		dst.Used += used
		return nil
	}

	var groups []map[string]any
	if rawGroups, ok := sum["CreditGroups"].([]any); ok {
		for _, rawItem := range rawGroups {
			if item, ok := rawItem.(map[string]any); ok && hasTotalField(item) {
				groups = append(groups, item)
			}
		}
	}
	for _, group := range groups {
		if strField(group, "Kind") == "plan_base" {
			if err := addBucket(&kinds.Plan, group); err != nil {
				return summaryKinds{}, err
			}
		} else {
			if err := addBucket(&kinds.Extra, group); err != nil {
				return summaryKinds{}, err
			}
		}
	}

	if rawPkgs, ok := sum["Packages"].([]any); ok {
		for _, rawItem := range rawPkgs {
			pkg, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if hasTotalField(pkg) {
				if len(groups) == 0 {
					if ofPlan(strField(pkg, "PackageCode")) {
						if err := addBucket(&kinds.Plan, pkg); err != nil {
							return summaryKinds{}, err
						}
					} else {
						if err := addBucket(&kinds.Extra, pkg); err != nil {
							return summaryKinds{}, err
						}
					}
				}
			} else if hasRemainField(pkg) {
				rem, err := capacity(pkg["CycleRemainCapacity"])
				if err != nil {
					return summaryKinds{}, err
				}
				kinds.Left += rem
				kinds.Bags++
			}
		}
	}
	return kinds, nil
}

func creditWindow(name string, used, total float64) sdk.UsageWindow {
	pct := clampPct((100 * used) / total)
	amt := used
	return sdk.UsageWindow{
		Name:    name,
		Used:    pct,
		Display: fmt.Sprintf("%s / %s", compactFloat(used), compactFloat(total)),
		Amount:  &amt,
		Limit:   total,
		Unit:    "credits",
	}
}

type runsOutInfo struct {
	AtMs   int64
	Amount float64
}

func totalWindowOf(planWin, extraWin *sdk.UsageWindow) sdk.UsageWindow {
	planWin.Aside = true
	extraWin.Aside = true
	var planAmt, extraAmt float64
	if planWin.Amount != nil {
		planAmt = *planWin.Amount
	}
	if extraWin.Amount != nil {
		extraAmt = *extraWin.Amount
	}
	win := creditWindow("Total credits", planAmt+extraAmt, planWin.Limit+extraWin.Limit)
	if planWin.ResetsAt > 0 {
		win.ResetsAt = planWin.ResetsAt
	}
	if planWin.Span > 0 {
		win.Span = planWin.Span
	}
	return win
}

type usageParsed struct {
	Plan    string
	Windows []sdk.UsageWindow
}

func usageOf(sum map[string]any, plan string, site Site) (usageParsed, error) {
	planName := plan
	if planName == "" {
		if truthy(sum["IsPaidUser"]) {
			planName = "Pro"
		} else {
			planName = "Free plan"
		}
	}
	out := usageParsed{Plan: planName}
	kinds, err := kindsOf(sum, site)
	if err != nil {
		return usageParsed{}, err
	}
	if kinds.Plan.Total > 0 && kinds.Extra.Total > 0 {
		credits := creditWindow("Credits", kinds.Plan.Used, kinds.Plan.Total)
		more := creditWindow("Extra credits", kinds.Extra.Used, kinds.Extra.Total)
		totalWin := totalWindowOf(&credits, &more)
		out.Windows = append(out.Windows, credits, more, totalWin)
	} else {
		total := kinds.Plan.Total + kinds.Extra.Total
		used := kinds.Plan.Used + kinds.Extra.Used
		if total > 0 {
			out.Windows = append(out.Windows, creditWindow("Credits", used, total))
		}
	}
	total := kinds.Plan.Total + kinds.Extra.Total
	if kinds.Bags > 0 && (total == 0 || kinds.Left > 0) {
		bal := kinds.Left
		out.Windows = append(out.Windows, sdk.UsageWindow{
			Name:    "Prepaid credits",
			Balance: &bal,
			Display: fmt.Sprintf("%s credits", compactFloat(kinds.Left)),
			Unit:    "credits",
		})
	}
	return out, nil
}

type listedExtraInfo struct {
	Total   float64
	Used    float64
	RunsOut *runsOutInfo
}

func endOfPackage(pkg map[string]any) int64 {
	if pkg == nil {
		return 0
	}
	if ms, ok := asFloat(pkg["DeductionEndTime"]); ok && ms > 0 {
		return int64(ms)
	}
	for _, key := range []string{"ExpiredTime", "CycleEndTime"} {
		if ts := parseFlexibleTime(pkg[key]); ts > 0 {
			return ts
		}
	}
	return 0
}

func listedExtra(sum map[string]any, packages []map[string]any, site Site, nowMs int64) *listedExtraInfo {
	summed := map[string]bool{}
	if rawPkgs, ok := sum["Packages"].([]any); ok {
		for _, rawItem := range rawPkgs {
			if item, ok := rawItem.(map[string]any); ok {
				if code := strField(item, "PackageCode"); code != "" {
					summed[code] = true
				}
			}
		}
	}
	allowed := extraCodes
	if site.AI {
		allowed = append(slices.Clone(extraCodes), extraAICodes...)
	}
	var total, left float64
	var soonAt int64
	var lapsing float64
	posFloat := func(raw any) float64 {
		val, ok := asFloat(raw)
		if !ok || val <= 0 {
			return 0
		}
		return val
	}
	for _, pkg := range packages {
		code := strField(pkg, "PackageCode")
		if !slices.Contains(allowed, code) || summed[code] {
			continue
		}
		end := endOfPackage(pkg)
		if end > 0 && end <= nowMs {
			continue
		}
		var slice map[string]any
		if fmt.Sprint(pkg["CapacityType"]) == sliceType {
			if details, ok := pkg["SlicePeriodUsageDetails"].([]any); ok && len(details) > 0 {
				slice, _ = details[0].(map[string]any)
			}
		}
		var pkgTotal, pkgLeft float64
		if slice != nil {
			pkgTotal = posFloat(slice["SlicePeriodCapacitySizePrecise"])
			pkgLeft = math.Min(pkgTotal, posFloat(slice["SlicePeriodCapacityRemainPrecise"]))
		} else {
			pkgTotal = posFloat(pkg["CycleCapacitySizePrecise"])
			pkgLeft = math.Min(pkgTotal, posFloat(pkg["CycleCapacityRemainPrecise"]))
		}
		if pkgTotal <= 0 {
			continue
		}
		total += pkgTotal
		left += pkgLeft
		if pkgLeft > 0 && end > 0 && slice == nil {
			if soonAt == 0 || end < soonAt {
				soonAt = end
				lapsing = pkgLeft
			} else if end == soonAt {
				lapsing += pkgLeft
			}
		}
	}
	if total <= 0 {
		return nil
	}
	info := &listedExtraInfo{Total: total, Used: total - left}
	if soonAt > 0 {
		info.RunsOut = &runsOutInfo{AtMs: soonAt, Amount: lapsing}
	}
	return info
}

func enterpriseUsageOf(ent map[string]any) (*sdk.UsageWindow, error) {
	if ent == nil {
		return nil, nil
	}
	limit, err := capacity(ent["limitNum"])
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	used, err := capacity(ent["credit"])
	if err != nil {
		return nil, err
	}
	win := creditWindow("Credits", used, limit)
	reset := parseBeijingDateTime(ent["cycleResetTime"])
	start := parseBeijingDateTime(ent["cycleStartTime"])
	if reset > 0 {
		win.ResetsAt = reset
	}
	if reset > start && start > 0 {
		win.Span = (reset - start) / 1000
	}
	return &win, nil
}

func runsOutOf(sum map[string]any, packages []map[string]any, nowMs int64, only func(string) bool, site *Site) *runsOutInfo {
	counted := map[string]bool{}
	if rawPkgs, ok := sum["Packages"].([]any); ok {
		for _, rawItem := range rawPkgs {
			if item, ok := rawItem.(map[string]any); ok {
				if code := strField(item, "PackageCode"); code != "" {
					counted[code] = true
				}
			}
		}
	}
	if site != nil {
		for _, code := range extraCodes {
			counted[code] = true
		}
		if site.AI {
			for _, code := range extraAICodes {
				counted[code] = true
			}
		}
	}
	var soonAt int64
	var amount float64
	for _, pkg := range packages {
		code := strField(pkg, "PackageCode")
		if !counted[code] || (only != nil && !only(code)) || code == dailyCode || fmt.Sprint(pkg["CapacityType"]) == sliceType {
			continue
		}
		left, err := capacity(firstPresent(pkg, "CycleCapacityRemainPrecise", "CycleCapacityRemain"))
		if err != nil || left <= 0 {
			continue
		}
		end := endOfPackage(pkg)
		if end <= nowMs {
			continue
		}
		if soonAt == 0 || end < soonAt {
			soonAt = end
			amount = left
		} else if end == soonAt {
			amount += left
		}
	}
	if soonAt == 0 {
		return nil
	}
	return &runsOutInfo{AtMs: soonAt, Amount: amount}
}

type cycleInfo struct {
	ResetsAt int64
	Span     int64
}

func cycleOf(sum map[string]any, packages []map[string]any, nowMs int64) *cycleInfo {
	sub := strField(sum, "SubscriptionPackageCode")
	extract := func(pkg map[string]any) *cycleInfo {
		end := parseBeijingDateTime(pkg["CycleEndTime"])
		start := parseBeijingDateTime(pkg["CycleStartTime"])
		if end <= nowMs {
			return nil
		}
		info := &cycleInfo{ResetsAt: end}
		if start > 0 && start < end {
			info.Span = (end - start) / 1000
		}
		return info
	}
	if sub != "" {
		for _, pkg := range packages {
			if strField(pkg, "PackageCode") == sub {
				return extract(pkg)
			}
		}
		return nil
	}
	for _, pkg := range packages {
		if fmt.Sprint(pkg["CapacityType"]) == sliceType {
			return extract(pkg)
		}
	}
	return nil
}

func (owner *module) packagesOf(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, nowMs int64) ([]map[string]any, error) {
	day := beijingDate(nowMs)
	queries := []struct {
		path  string
		extra map[string]any
	}{
		{
			path: "/billing/meter/get-user-resource-paid-packages",
			extra: map[string]any{
				"PackageCodes":  paidCodes,
				"NeedRenewInfo": true,
			},
		},
		{
			path: "/billing/meter/get-user-resource-free-packages",
			extra: map[string]any{
				"PackageCodes":         freeCodes,
				"SlicePeriodStartTime": day + " 00:00:00",
				"SlicePeriodEndTime":   day + " 23:59:59",
			},
		},
	}
	var out []map[string]any
	for _, query := range queries {
		var listCount int
		for page := 1; page <= 10; page++ {
			body := map[string]any{
				"PageNumber": page,
				"PageSize":   200,
				"Status":     []int{0, 3},
			}
			for key, val := range query.extra {
				body[key] = val
			}
			data, err := owner.meter(ctx, stream, site, acct, query.path, body)
			if err != nil {
				return nil, err
			}
			rawAccounts, _ := data["Accounts"].([]any)
			for _, item := range rawAccounts {
				if pkg, ok := item.(map[string]any); ok {
					out = append(out, pkg)
				}
			}
			listCount += len(rawAccounts)
			totalCount := parseIntCode(data["TotalCount"])
			if len(rawAccounts) < 200 || (totalCount > 0 && listCount >= totalCount) {
				break
			}
		}
	}
	return out, nil
}

func (owner *module) spareFree(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, windows []sdk.UsageWindow) {
	if len(windows) == 0 {
		return
	}
	list, err := owner.liveModels(ctx, stream, site, acct)
	if err != nil {
		return
	}
	var freeIDs []string
	seen := map[string]bool{}
	for _, model := range list {
		if model.Free && model.ID != "" {
			key := strings.ToLower(model.ID)
			if !seen[key] {
				seen[key] = true
				freeIDs = append(freeIDs, model.ID)
			}
		}
	}
	if len(freeIDs) == 0 {
		return
	}
	for idx := range windows {
		if len(windows[idx].Models) == 0 {
			windows[idx].NotModels = slices.Clone(freeIDs)
		}
	}
}

func (owner *module) appendFreeWindows(site Site, acct Credential, nowMs int64, windows []sdk.UsageWindow) []sdk.UsageWindow {
	account := firstNonEmpty(acct.AccountID, acct.User, acct.UID)
	if account == "" {
		return windows
	}
	specs := cnEstimatedWindows
	if site.AI {
		specs = []freeWindowSpec{freeWindowAI}
	}
	cwd := owner.options.WorkDir()
	home := owner.options.HomeDir()
	for _, spec := range specs {
		rows := owner.freeWindowRows(account, nowMs, cwd, owner.options.Env, home, spec)
		if win := freeWindowOf(rows, account, nowMs, spec); win != nil {
			windows = append(windows, *win)
		}
	}
	return windows
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.UsageRequest
	if nonempty(call.Params) && decodeObject(call.Params, &req) != nil {
		return nil, invalid("Usage request must be a JSON object")
	}
	site, ok := owner.siteFor(call.Scope.Provider)
	if !ok {
		return nil, invalid("Unknown WorkBuddy provider")
	}
	rawAuth := call.Auth
	if !nonempty(rawAuth) {
		rawAuth = req.Credentials
	}
	if !nonempty(rawAuth) {
		return sdk.UsageResult{Error: "not signed in"}, nil
	}
	var stored Credential
	if decodeObject(rawAuth, &stored) != nil {
		return sdk.UsageResult{Error: "not signed in"}, nil
	}
	if stored.isAPIKey() {
		base := strings.TrimSuffix(site.Keys, "/profile/keys")
		return sdk.UsageResult{
			Error: fmt.Sprintf("credits unknown with an API key (WorkBuddy's own client reads none with one); see %s", base),
		}, nil
	}
	if !stored.isOAuth() || (stored.Access == "" && stored.Source == "") {
		return sdk.UsageResult{Error: "not signed in"}, nil
	}

	active, err := owner.current(ctx, stream, site, stored)
	if err != nil {
		return sdk.UsageResult{Error: err.Error()}, nil
	}
	sum, err := owner.meter(ctx, stream, site, active, summaryURI, map[string]any{})
	if err != nil {
		var mErr *meterError
		if errors.As(err, &mErr) && mErr.Status == http.StatusUnauthorized {
			reissue := owner.reissued(ctx, stream, site, active)
			if !reissue.HasCred {
				return sdk.UsageResult{Error: refusedText(site, active, reissue.Why, "")}, nil
			}
			active = reissue.Cred
			sum, err = owner.meter(ctx, stream, site, active, summaryURI, map[string]any{})
			if err != nil {
				var mErr2 *meterError
				if errors.As(err, &mErr2) && mErr2.Status == http.StatusUnauthorized {
					return sdk.UsageResult{Error: refusedText(site, active, "it refused the token renewed for it too", "")}, nil
				}
				return sdk.UsageResult{Error: err.Error()}, nil
			}
		} else {
			return sdk.UsageResult{Error: err.Error()}, nil
		}
	}

	parsed, err := usageOf(sum, stored.Plan, site)
	if err != nil {
		return sdk.UsageResult{Error: err.Error()}, nil
	}
	nowMs := owner.nowMs()

	if len(parsed.Windows) == 0 {
		entData, entErr := owner.meter(ctx, stream, site, active, "/billing/meter/get-enterprise-user-usage", map[string]any{})
		if entErr == nil {
			if entWin, parseErr := enterpriseUsageOf(entData); parseErr == nil && entWin != nil {
				windows := []sdk.UsageWindow{*entWin}
				owner.spareFree(ctx, stream, site, active, windows)
				return sdk.UsageResult{Windows: windows}, nil
			}
		}
	}

	creditsIdx := -1
	extraIdx := -1
	totalIdx := -1
	for idx := range parsed.Windows {
		switch parsed.Windows[idx].Name {
		case "Credits":
			creditsIdx = idx
		case "Extra credits":
			extraIdx = idx
		case "Total credits":
			totalIdx = idx
		}
	}
	if creditsIdx >= 0 {
		if packages, pkgErr := owner.packagesOf(ctx, stream, site, active, nowMs); pkgErr == nil {
			if cyc := cycleOf(sum, packages, nowMs); cyc != nil {
				parsed.Windows[creditsIdx].ResetsAt = cyc.ResetsAt
				parsed.Windows[creditsIdx].Span = cyc.Span
			}
			if extraIdx >= 0 && totalIdx >= 0 {
				parsed.Windows[totalIdx] = totalWindowOf(&parsed.Windows[creditsIdx], &parsed.Windows[extraIdx])
			} else if extraIdx < 0 {
				if kinds, kErr := kindsOf(sum, site); kErr == nil && kinds.Plan.Total > 0 && kinds.Extra.Total == 0 {
					if more := listedExtra(sum, packages, site, nowMs); more != nil {
						extraWin := creditWindow("Extra credits", more.Used, more.Total)
						totalWin := totalWindowOf(&parsed.Windows[creditsIdx], &extraWin)
						insertAt := creditsIdx + 1
						tail := slices.Clone(parsed.Windows[insertAt:])
						parsed.Windows = append(append(parsed.Windows[:insertAt], extraWin, totalWin), tail...)
					}
				}
			}
		}
	}

	owner.spareFree(ctx, stream, site, active, parsed.Windows)
	parsed.Windows = owner.appendFreeWindows(site, active, nowMs, parsed.Windows)
	return sdk.UsageResult{Windows: parsed.Windows}, nil
}
