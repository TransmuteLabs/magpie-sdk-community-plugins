package zcode

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type startAvailability struct {
	Models   map[string]bool
	Declared map[string]string
	Active   bool
	Pending  bool
	Windows  []sdk.UsageWindow
	Upcoming []sdk.UsageWindow
}

type startBucket struct {
	models    []string
	window    sdk.UsageWindow
	spendable bool
}

func metric(value any) (float64, bool) {
	if text, valid := value.(string); valid {
		text = strings.TrimSpace(text)
		if text == "" {
			return 0, false
		}
		value = json.Number(text)
	}
	return finiteNumber(value)
}

func (owner *module) startEntitlement(ctx context.Context, stream *sdk.Stream, account Credentials, includePending bool) (startAvailability, error) {
	response, err := owner.vendorRequest(ctx, stream, http.MethodGet,
		owner.zcodeOrigin()+"/api/v1/zcode-plan/billing/balance?app_version="+url.QueryEscape(ClientVersion),
		http.Header{"Authorization": {"Bearer " + account.SessionToken}, "X-Device-Mid": {account.Device}}, nil)
	if err != nil {
		return startAvailability{}, err
	}
	data, err := responseData(response)
	plans, validPlans := data["plans"].([]any)
	balances, validBalances := data["balances"].([]any)
	if err != nil || !validPlans || !validBalances || len(plans) > 4096 || len(balances) > 16384 {
		return startAvailability{}, protocol.ErrInvalid
	}
	now := owner.options.Now().UnixMilli()
	if date, err := http.ParseTime(response.Headers.Get("Date")); err == nil {
		now = date.UnixMilli()
	} else if server, valid := metric(data["server_time"]); valid && server >= 0 && server*1000 < float64(math.MaxInt64) {
		now = int64(server * 1000)
	}
	result := startAvailability{Models: map[string]bool{}, Declared: map[string]string{}}
	buckets := []startBucket{}
	for _, value := range plans {
		plan, valid := value.(map[string]any)
		if !valid {
			return result, protocol.ErrInvalid
		}
		state, _ := plan["status"].(string)
		if _, valid := plan["status"].(string); !valid {
			return result, protocol.ErrInvalid
		}
		state = strings.ToLower(strings.TrimSpace(state))
		start, starts := metric(plan["starts_at"])
		end, ends := metric(plan["ends_at"])
		if plan["starts_at"] != nil && !starts || plan["ends_at"] != nil && !ends ||
			start < 0 || end < 0 || start*1000 >= float64(math.MaxInt64) || end*1000 >= float64(math.MaxInt64) {
			return result, protocol.ErrInvalid
		}
		if state == "expired" || ends && end > 0 && end*1000 <= float64(now) {
			continue
		}
		if starts && start*1000 > float64(now) {
			result.Pending = true
			name, _ := plan["name"].(string)
			if !safeText(name) || len(name) > 256 {
				name = "Start Plan"
			}
			result.Upcoming = append(result.Upcoming, sdk.UsageWindow{
				Name: "ZCode gift plan not started yet · " + name, Aside: true, ResetsAt: int64(start * 1000),
			})
			continue
		}
		if state != "active" {
			continue
		}
		result.Active = true
		planID, _ := plan["plan_id"].(string)
		userPlan, _ := plan["user_plan_id"].(string)
		for _, value := range balances {
			bucket, valid := value.(map[string]any)
			if !valid {
				return result, protocol.ErrInvalid
			}
			bucketUserPlan, _ := bucket["user_plan_id"].(string)
			bucketPlan, _ := bucket["plan_id"].(string)
			if bucket["user_plan_id"] != nil && bucketUserPlan == "" || bucket["plan_id"] != nil && bucketPlan == "" {
				return result, protocol.ErrInvalid
			}
			matches := planID != "" && bucketPlan == planID
			if userPlan != "" && bucketUserPlan != "" {
				matches = userPlan == bucketUserPlan
			}
			if !matches {
				continue
			}
			expires, hasExpiry := metric(bucket["expires_at"])
			if bucket["expires_at"] != nil && !hasExpiry || expires < 0 || expires*1000 >= float64(math.MaxInt64) {
				return result, protocol.ErrInvalid
			}
			if hasExpiry && expires > 0 && expires*1000 <= float64(now) {
				continue
			}
			remaining, hasRemaining := metric(bucket["remaining_units"])
			total, hasTotal := metric(bucket["total_units"])
			used, hasUsed := metric(bucket["used_units"])
			if bucket["remaining_units"] != nil && !hasRemaining || bucket["total_units"] != nil && !hasTotal ||
				bucket["used_units"] != nil && !hasUsed || total < 0 || used < 0 {
				return result, protocol.ErrInvalid
			}
			if !hasRemaining && !(hasTotal && hasUsed) {
				return result, unsupported("Start bucket spendability is unknown")
			}
			if !hasRemaining {
				remaining = total - used
			}
			if !hasTotal && hasUsed {
				total, hasTotal = max(remaining, 0)+used, true
			}
			if hasTotal && remaining > total {
				return result, protocol.ErrInvalid
			}
			var names []string
			if capabilities, exists := bucket["capabilities"]; exists {
				values, valid := capabilities.([]any)
				if !valid {
					return result, protocol.ErrInvalid
				}
				for _, value := range values {
					capability, valid := value.(string)
					if !valid {
						return result, protocol.ErrInvalid
					}
					if strings.HasPrefix(strings.ToLower(strings.TrimSpace(capability)), "model:") {
						names = append(names, strings.TrimSpace(strings.TrimSpace(capability)[6:]))
					}
				}
			}
			if len(names) == 0 {
				if name, valid := bucket["show_name"].(string); valid && strings.TrimSpace(name) != "" {
					names = append(names, name)
				}
			}
			seen := map[string]bool{}
			models := []string{}
			for _, name := range names {
				name = strings.ToLower(strings.TrimSpace(name))
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				if !safeText(name) || len(name) > 290 || strings.ContainsAny(name, " \t\r\n") {
					return result, protocol.ErrInvalid
				}
				models = append(models, name)
				result.Declared[name] = wireGiftModel(name)
				if remaining > 0 {
					result.Models[name] = true
				}
			}
			name, _ := bucket["show_name"].(string)
			if !safeText(name) || len(name) > 256 {
				name = strings.Join(models, ", ")
			}
			if name == "" {
				name = "Credits"
			}
			planName, _ := plan["name"].(string)
			if !safeText(planName) || len(planName) > 256 {
				planName = "Start Plan"
			}
			window := sdk.UsageWindow{Name: name + " · " + planName, Limit: 100}
			if owner.richQuota {
				window.Limit, window.Span = total, startQuotaSpan(plan, bucket)
				if hasTotal && total > 0 {
					amount := used
					if !hasUsed {
						amount = total - remaining
					}
					quotaCounts(&window, amount, total)
				}
			}
			if remaining <= 0 {
				window.Used = 100
			} else if !hasTotal || total <= 0 {
				window.Unknown, window.Error = true, "Start balance has spendability but no authoritative total capacity"
			} else {
				window.Used = max(0, min(100, 100*(total-remaining)/total))
			}
			if hasExpiry && expires > 0 {
				window.ResetsAt = int64(expires * 1000)
			}
			if ends && end > 0 && (window.ResetsAt == 0 || int64(end*1000) < window.ResetsAt) {
				window.ResetsAt = int64(end * 1000)
			}
			buckets = append(buckets, startBucket{models: models, window: window, spendable: remaining > 0})
			if len(buckets) > 1024 || len(result.Declared) > 1024 {
				return result, protocol.ErrInvalid
			}
		}
	}
	sort.SliceStable(result.Upcoming, func(first, second int) bool {
		return result.Upcoming[first].ResetsAt < result.Upcoming[second].ResetsAt
	})
	if !result.Active && !(includePending && result.Pending) {
		return result, unsupported("No active or permitted pending Start entitlement was confirmed")
	}
	result.Windows = projectStartBuckets(buckets)
	return result, nil
}

func projectStartBuckets(buckets []startBucket) []sdk.UsageWindow {
	best := map[string]int{}
	for index, bucket := range buckets {
		for _, model := range bucket.models {
			old, found := best[model]
			if !found || !buckets[old].spendable && bucket.spendable ||
				buckets[old].spendable == bucket.spendable &&
					(buckets[old].window.Unknown && !bucket.window.Unknown ||
						buckets[old].window.Unknown == bucket.window.Unknown && bucket.window.Used < buckets[old].window.Used) {
				best[model] = index
			}
		}
	}
	result, aside := []sdk.UsageWindow{}, []sdk.UsageWindow{}
	for index, bucket := range buckets {
		selected := false
		for _, model := range bucket.models {
			if best[model] != index {
				continue
			}
			window := bucket.window
			window.Models = []string{model}
			result, selected = append(result, window), true
		}
		if !selected {
			window := bucket.window
			window.Models = bucket.models
			window.Aside = true
			aside = append(aside, window)
		}
	}
	return append(result, aside...)
}
