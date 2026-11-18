package zcode

import (
	"math"
	"strconv"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func quotaCompact(value float64) string {
	if math.Trunc(value) == value {
		return strconv.FormatFloat(value, 'f', 0, 64)
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

func quotaCounts(window *sdk.UsageWindow, amount, total float64) {
	window.Amount, window.Limit = &amount, total
	window.Display = quotaCompact(amount) + " / " + quotaCompact(total)
}

func quotaSpan(unit, count float64) int64 {
	count = math.Trunc(count)
	if unit == 3 && count <= 0 {
		count = 5
	}
	seconds := map[float64]float64{1: 60, 3: 3600, 4: 86400, 5: 2592000, 6: 604800}[unit] * max(count, 1)
	if math.IsNaN(seconds) || seconds < 0 || seconds > 9223372036 {
		return 0
	}
	return int64(seconds)
}

func startQuotaSpan(plan, bucket map[string]any) int64 {
	bucketID, _ := bucket["entitlement_id"].(string)
	entitlements, _ := plan["entitlements"].([]any)
	for _, value := range entitlements {
		entitlement, valid := value.(map[string]any)
		if !valid {
			continue
		}
		entitlementID, _ := entitlement["entitlement_id"].(string)
		if bucketID == "" || entitlementID != bucketID {
			continue
		}
		period, _ := entitlement["period"].(string)
		period = strings.ToLower(period)
		switch {
		case strings.Contains(period, "day"), strings.Contains(period, "daily"):
			return 86400
		case strings.Contains(period, "week"):
			return 604800
		case strings.Contains(period, "month"):
			return 2592000
		}
	}
	start, hasStart := metric(bucket["period_start"])
	end, hasEnd := metric(bucket["period_end"])
	if hasStart && hasEnd && end > start && end-start <= 9223372036 {
		return int64(end - start)
	}
	return 0
}
