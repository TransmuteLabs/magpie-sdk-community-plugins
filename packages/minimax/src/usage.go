package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type membership struct {
	HasPlan                     *bool
	Unknown                     bool
	Group, Tier, Balance, Title string
	ExpiresAt                   int64
}

func membershipOf(envelope map[string]any) membership {
	nested := object(envelope["data"])
	pick := func(key string) any {
		if value, exists := envelope[key]; exists && value != nil {
			return value
		}
		return nested[key]
	}
	result := membership{Group: text(pick("op_group_id")), Tier: text(pick("token_plan_tier"))}
	if present, valid := pick("has_token_plan").(bool); valid {
		result.HasPlan = &present
	}
	if expiry := positive(pick("token_plan_expires_at")); expiry > 0 && expiry < 1<<53 {
		result.ExpiresAt = int64(expiry)
		if expiry < 1e12 {
			result.ExpiresAt *= 1000
		}
	}
	result.Balance = text(object(pick("op_credit_summary"))["total_remaining_amount"])
	if result.Balance == "" {
		result.Balance = text(pick("opcredit_balance"))
		if balance, valid := number(pick("opcredit_balance")); valid {
			result.Balance = strconv.FormatFloat(balance, 'f', -1, 64)
		}
	}
	return result
}

func parseObject(body []byte) map[string]any {
	var result map[string]any
	_ = json.Unmarshal(body, &result)
	return result
}

func refusalStatus(err error) int {
	var refusal *APIError
	if errors.As(err, &refusal) {
		return refusal.Status
	}
	return 0
}

func (client *SelectedClient) membership(ctx context.Context) (membership, error) {
	body, workspaceError := client.accountJSON(ctx, "/matrix/api/v1/user/get_user_extra_info", map[string]any{}, nil, "")
	if workspaceError != nil {
		if refusalStatus(workspaceError) == 401 || refusalStatus(workspaceError) == 403 {
			return membership{}, workspaceError
		}
		body, membershipError := client.accountJSON(ctx, "/matrix/api/v1/commerce/get_membership_info", map[string]any{}, nil, "")
		if membershipError != nil {
			if refusalStatus(workspaceError) == 404 && refusalStatus(membershipError) == 404 {
				return membership{Unknown: true}, nil
			}
			return membership{}, membershipError
		}
		return membershipOf(parseObject(body)), nil
	}
	envelope := parseObject(body)
	workspaces, valid := envelope["workspaces"].([]any)
	if !valid {
		workspaces, _ = object(envelope["data"])["workspaces"].([]any)
	}
	for _, entry := range workspaces {
		workspace := object(entry)
		kind, valid := number(workspace["workspace_type"])
		if !valid || kind != 0 {
			continue
		}
		identifier := workspace["workspace_id"]
		_, numeric := number(identifier)
		if text(identifier) == "" && !numeric {
			continue
		}
		existing := membershipOf(workspace)
		body, err := client.accountJSON(ctx, "/matrix/api/v1/commerce/get_membership_info", map[string]any{"workspace_id": identifier}, nil, "")
		if err != nil {
			if refusalStatus(err) == 401 || refusalStatus(err) == 403 || ctx.Err() != nil {
				return membership{}, err
			}
			return existing, nil
		}
		current := membershipOf(parseObject(body))
		if existing.HasPlan != nil && *existing.HasPlan || current.HasPlan != nil && *current.HasPlan {
			current.HasPlan = new(true)
		} else if existing.HasPlan != nil && !*existing.HasPlan || current.HasPlan != nil && !*current.HasPlan {
			current.HasPlan = new(false)
		}
		if existing.Group != "" {
			current.Group = existing.Group
		}
		if current.Tier == "" {
			current.Tier = existing.Tier
		}
		if current.Balance == "" {
			current.Balance = existing.Balance
		}
		if current.ExpiresAt == 0 {
			current.ExpiresAt = existing.ExpiresAt
		}
		return current, nil
	}
	return membership{}, nil
}

func percent(value any) (float64, bool) {
	if literal, valid := value.(string); valid {
		return number(strings.TrimSuffix(strings.TrimSpace(literal), "%"))
	}
	return number(value)
}

func planWindows(body []byte) ([]sdk.UsageWindow, error) {
	envelope := parseObject(body)
	entries, valid := envelope["model_remains"].([]any)
	if !valid || len(entries) > 128 {
		return nil, invalid("MiniMax plan response has no bounded model list")
	}
	windows := []sdk.UsageWindow{}
	for _, entry := range entries {
		model := object(entry)
		name := text(model["model_name"])
		if name == "" {
			continue
		}
		intervalStatus, _ := number(model["current_interval_status"])
		weeklyStatus, _ := number(model["current_weekly_status"])
		intervalTotal, hasIntervalTotal := number(model["current_interval_total_count"])
		weeklyTotal, hasWeeklyTotal := number(model["current_weekly_total_count"])
		if intervalStatus == 3 && weeklyStatus == 3 && hasIntervalTotal && hasWeeklyTotal && intervalTotal == 0 && weeklyTotal == 0 {
			continue
		}
		general := strings.EqualFold(name, "general")
		for _, weekly := range []bool{false, true} {
			prefix, startKey, endKey := "current_interval_", "start_time", "end_time"
			if weekly {
				prefix, startKey, endKey = "current_weekly_", "weekly_start_time", "weekly_end_time"
			}
			status, _ := number(model[prefix+"status"])
			remaining, hasRemaining := number(model[prefix+"remaining_percent"])
			told, hasTold := percent(model[prefix+"used_percent"])
			if status != 3 && status != 2 && !hasRemaining && !hasTold {
				continue
			}
			span := int64(0)
			start, end := positive(model[startKey]), positive(model[endKey])
			if start > 0 && end > start {
				if start < 1e12 {
					start *= 1000
				}
				if end < 1e12 {
					end *= 1000
				}
				if end-start < 1<<53 {
					span = int64((end - start) / 1000)
				}
			} else if weekly {
				span = 7 * 86400
			}
			label := "Allowance"
			switch {
			case weekly:
				label = "7 days"
			case span > 86400 && span%86400 == 0:
				label = fmt.Sprintf("%d days", span/86400)
			case span >= 3600 && span%3600 == 0:
				label = fmt.Sprintf("%d hours", span/3600)
			case span > 0:
				label = fmt.Sprintf("%d minutes", span/60)
			}
			if status == 3 {
				if general {
					windows = append(windows, sdk.UsageWindow{Name: label, Display: "Unlimited", Aside: true})
				}
				continue
			}
			total, hasTotal := percent(model[prefix+"total_percent"])
			if !hasTotal && weekly {
				if boost := positive(model["weekly_boost_permille"]); boost > 1000 {
					total = boost / 10
				}
			}
			if total <= 0 {
				total = 100
			}
			used := max(0, min(100, 100-remaining))
			if hasTold {
				used = max(0, min(100, told/total*100))
			}
			if status == 2 {
				used = 100
			}
			window := sdk.UsageWindow{Name: label, Used: used, Span: span}
			if end > 0 && end < 1<<53 {
				if end < 1e12 {
					end *= 1000
				}
				window.ResetsAt = int64(end)
			}
			items, hasItems := number(model[prefix+"used_count"])
			limit, hasLimit := number(model[prefix+"total_count"])
			if hasItems && hasLimit && items >= 0 && limit > 0 && items <= limit {
				if status == 2 {
					items = limit
				}
				window.Amount, window.Limit = &items, limit
				if strings.EqualFold(name, "video") {
					window.Unit = "videos"
				}
			} else if total != 100 {
				amount := used * total / 100
				if hasTold {
					amount = told
				}
				if status == 2 {
					amount = total
				}
				window.Amount, window.Limit, window.Unit = &amount, total, "%"
			}
			if !general {
				window.Name = strings.ToUpper(name[:1]) + name[1:] + " · " + label
				window.Aside = true
			}
			windows = append(windows, window)
		}
	}
	return windows, nil
}

func (client *SelectedClient) platformJSON(ctx context.Context, path, group string, timeout time.Duration) ([]byte, error) {
	timeoutContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	headers := http.Header{"Accept": {"application/json"}, "Authorization": {"Bearer " + client.credentials.Access}}
	if group != "" {
		headers.Set("X-Group-Id", group)
	}
	response, err := client.owner.siteRequest(timeoutContext, client.stream, "GET", client.site.Platform+path, nil, headers)
	if err != nil {
		return nil, err
	}
	result, err := bounded(response, 4<<20)
	if err != nil {
		return nil, err
	}
	return checkedJSON(result)
}

func (client *SelectedClient) plan(ctx context.Context, group string) ([]sdk.UsageWindow, string, error) {
	var windows []sdk.UsageWindow
	var title string
	var planError error
	var pending sync.WaitGroup
	pending.Add(2)
	go func() {
		defer pending.Done()
		body, err := client.platformJSON(ctx, "/backend/account/token_plan/remains_percent", group, 5*time.Second)
		if err == nil {
			windows, err = planWindows(body)
		}
		if err == nil && len(windows) > 0 {
			return
		}
		body, err = client.platformJSON(ctx, "/v1/api/openplatform/coding_plan/remains", group, 10*time.Second)
		if err != nil {
			planError = err
			return
		}
		windows, planError = planWindows(body)
	}()
	go func() {
		defer pending.Done()
		body, err := client.platformJSON(ctx, "/v1/api/openplatform/charge/combo/cycle_audio_resource_package?biz_line=2&cycle_type=1&resource_package_type=7", group, 5*time.Second)
		if err == nil {
			title = text(object(parseObject(body)["current_subscribe"])["current_subscribe_title"])
		}
	}()
	pending.Wait()
	return windows, title, planError
}

func creditAmount(value any, clampNegative bool) (float64, bool) {
	literal := text(value)
	if literal == "" {
		if numeric, valid := number(value); valid {
			literal = strconv.FormatFloat(numeric, 'f', -1, 64)
		}
	}
	literal = strings.ReplaceAll(literal, ",", "")
	if literal == "" || strings.HasPrefix(literal, ".") || strings.HasSuffix(literal, ".") {
		return 0, false
	}
	negative := strings.HasPrefix(literal, "-")
	if negative {
		if !clampNegative {
			return 0, false
		}
		literal = strings.TrimPrefix(literal, "-")
	}
	dot := false
	for _, character := range literal {
		if character == '.' && !dot {
			dot = true
			continue
		}
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	amount, err := strconv.ParseFloat(literal, 64)
	if err != nil || math.IsInf(amount, 0) || math.IsNaN(amount) || amount >= 1<<53 {
		return 0, false
	}
	if negative {
		return 0, true
	}
	return amount, true
}

func (client *SelectedClient) credits(ctx context.Context) (*sdk.UsageWindow, error) {
	batches := []any{}
	complete := false
	for page := 1; page <= 10; page++ {
		query := checkinQuery()
		query.Set("page_number", strconv.Itoa(page))
		query.Set("page_size", "20")
		query.Set("include_expired", "true")
		body, err := client.accountJSON(ctx, "/minimax-cloud/api/v1/credit/details", nil, query, "")
		if err != nil {
			return nil, err
		}
		envelope := parseObject(body)
		if _, valid := envelope["details"].([]any); !valid {
			envelope = object(envelope["data"])
		}
		details, valid := envelope["details"].([]any)
		total, hasTotal := number(envelope["total_count"])
		if !valid || !hasTotal || total < 0 || total > 200 || total != math.Trunc(total) || len(details) > 20 {
			return nil, invalid("MiniMax credit ledger is incomplete")
		}
		batches = append(batches, details...)
		if len(batches) >= int(total) {
			complete = true
			break
		}
		if len(details) == 0 {
			break
		}
	}
	if !complete {
		return nil, invalid("MiniMax credit ledger exceeds its page limit")
	}
	total, left, soonest, expiring := float64(0), float64(0), float64(0), float64(0)
	now := client.owner.options.Now().UnixMilli()
	for _, entry := range batches {
		batch := object(entry)
		kind, _ := number(batch["credit_type"])
		if kind != 1 && kind != 2 {
			continue
		}
		granted, hasGranted := creditAmount(batch["granted_amount"], false)
		remaining, hasRemaining := creditAmount(batch["remaining_amount"], true)
		expiry, hasExpiry := number(batch["expire_at_ms"])
		if !hasGranted || !hasRemaining || !hasExpiry {
			return nil, invalid("MiniMax credit batch is unreadable")
		}
		if expiry <= float64(now) {
			continue
		}
		total += granted
		left += min(remaining, granted)
		if remaining > 0 && (soonest == 0 || expiry < soonest) {
			soonest, expiring = expiry, remaining
		}
	}
	if total <= 0 {
		return nil, nil
	}
	used := max(0, total-left)
	round := func(value float64) float64 { return math.Round(value*100) / 100 }
	amount := round(used)
	window := &sdk.UsageWindow{Name: "Credits", Used: 100 * used / total, Amount: &amount, Limit: round(total), Unit: "credits", Display: fmt.Sprintf("%g / %g", round(used), round(total))}
	if soonest > 0 && soonest < 1<<53 {
		window.Display += fmt.Sprintf("; %g expire %s", round(expiring), time.UnixMilli(int64(soonest)).UTC().Format(time.RFC3339))
	}
	return window, nil
}

func checkinQuery() url.Values {
	return url.Values{"device_platform": {"web"}, "is_desktop": {"1"}, "desktop_version": {"0.6.12"}}
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.UsageRequest
	if decode(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, denied("Usage requires the selected host credentials")
	}
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, err
	}
	result := sdk.UsageResult{Revision: request.Revision, AsOf: owner.options.Now().UnixMilli()}
	if client.credentials.UserID == "" {
		account, identityError := client.identity(ctx)
		if identityError != nil {
			result.Error = "MiniMax account identity is unavailable"
			return result, nil
		}
		client.credentials.UserID = account.UserID
	}
	membership, err := client.membership(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result.Error = "MiniMax membership is unavailable"
		return result, nil
	}
	hasPlan := membership.HasPlan != nil && *membership.HasPlan
	if hasPlan && membership.Group != "" {
		result.Windows, membership.Title, err = client.plan(ctx, membership.Group)
		if err != nil {
			result.Error = "MiniMax plan allowance is unavailable"
			result.Windows = append(result.Windows, sdk.UsageWindow{Name: "M Plan", Unknown: true, Error: "Allowance could not be read"})
		}
	}
	if credits, err := client.credits(ctx); err == nil && credits != nil {
		credits.Aside = hasPlan
		result.Windows = append(result.Windows, *credits)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if balance, valid := creditAmount(membership.Balance, false); valid {
		result.Windows = append(result.Windows, sdk.UsageWindow{Name: "Credit balance", Balance: &balance, Unit: "credits", Display: fmt.Sprintf("%g credits", balance), Aside: true})
	}
	title := "Free"
	if membership.Unknown {
		title = "Membership unavailable"
	}
	if hasPlan {
		title = membership.Title
		if title == "" {
			title = "M Plan"
			if membership.Tier != "" {
				if contains([]string{"plus", "max", "ultra"}, strings.ToLower(membership.Tier)) {
					title = "Token Plan"
				}
				title += " " + membership.Tier
			}
		}
		if membership.ExpiresAt > 0 {
			title += "; ends " + time.UnixMilli(membership.ExpiresAt).UTC().Format(time.RFC3339)
		}
	}
	result.Windows = append(result.Windows, sdk.UsageWindow{Name: "Membership", Display: title, Aside: true})
	return result, nil
}
