package zcode

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func (owner *module) plans(ctx context.Context, stream *sdk.Stream, account Credentials) ([]map[string]any, error) {
	status, body, err := owner.control(ctx, stream, account, "", http.MethodGet,
		"/api/v1/zcode-plan/billing/balance?app_version="+url.QueryEscape(ClientVersion), nil)
	if err != nil {
		return nil, err
	}
	data, err := controlObject(status, body, true)
	if err != nil {
		return nil, err
	}
	values, valid := data["plans"].([]any)
	if !valid || len(values) > 4096 {
		return nil, protocol.ErrInvalid
	}
	plans := make([]map[string]any, 0, len(values))
	for _, value := range values {
		plan, valid := value.(map[string]any)
		planID, _ := plan["plan_id"].(string)
		userPlanID, _ := plan["user_plan_id"].(string)
		state, _ := plan["status"].(string)
		if !valid || !safeText(planID) || !safeText(userPlanID) || !safeText(state) {
			return nil, protocol.ErrInvalid
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (owner *module) billingStatus(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	if _, _, err := owner.principal(ctx, stream, account, ""); err != nil {
		return nil, err
	}
	plans, err := owner.plans(ctx, stream, account)
	return map[string]any{"plans": plans}, err
}

func (owner *module) claimPolicy(ctx context.Context, stream *sdk.Stream, account Credentials) error {
	path := "/api/v1/client/configs?app_version=" + url.QueryEscape(ClientVersion) + "&platform=" + url.QueryEscape(platform())
	status, body, err := owner.control(ctx, stream, account, "", http.MethodGet, path, nil)
	if err != nil {
		return unsupported("Claim verification policy is unavailable; use the official ZCode app")
	}
	data, err := controlObject(status, body, true)
	configs, valid := data["configs"].(map[string]any)
	if err != nil || !valid {
		return unsupported("Claim verification policy is malformed or unavailable")
	}
	if captcha := configs["captcha"]; captcha != nil {
		policy, valid := captcha.(map[string]any)
		if !valid {
			return unsupported("The selected claim verification policy is malformed; no claim was posted")
		}
		if policy["enabled"] != false {
			return &sdk.Error{Code: "verification_required", Message: "The selected client policy requires verification unavailable in this module; use the official ZCode app", Outcome: "not-started"}
		}
	}
	return nil
}

func planEnd(plan map[string]any) int64 {
	seconds, valid := finiteNumber(plan["ends_at"])
	if !valid || seconds <= 0 || seconds*1000 >= float64(math.MaxInt64) {
		return 0
	}
	return int64(math.Ceil(seconds * 1000))
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (owner *module) billingClaim(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		PlanID string `json:"planId"`
	}
	if err := decode(call.Params, &request); err != nil || !safeText(request.PlanID) || len(request.PlanID) > 128 {
		return nil, invalid("Billing claim requires one exact planId of at most 128 bytes")
	}
	if call.Scope.Operation == "" {
		return nil, invalid("Billing claim requires a host-owned write operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		now := owner.options.Now()
		checkin, _ := ctx.Value(checkinCallKey{}).(*checkinClaim)
		day, name := zcodeCheckinDay(now), request.PlanID
		if checkin != nil {
			day, name = checkin.day, checkin.name
		}
		old, exists := transaction.Journal.Claims[request.PlanID]
		if transaction.Journal.ClaimHoldUntil > now.UnixMilli() {
			result, _ = json.Marshal(map[string]any{"outcome": "rate_limited", "until": transaction.Journal.ClaimHoldUntil})
			return nil
		}
		if exists && old.State == "refused" && old.RetryAt > now.UnixMilli() {
			result = old.Result
			return nil
		}
		if exists && old.State == "observed" {
			result = old.Result
			return nil
		}
		if exists && old.State == "confirmed" {
			settled := claimAnswer(old.Result)
			if settled.Day == day || old.EndsAt > now.UnixMilli() || checkin != nil && settled.Day == "" {
				result = old.Result
				return nil
			}
		}
		plans, err := owner.plans(ctx, stream, account)
		if err != nil {
			if exists && (old.State == "pending" || old.State == "unknown") {
				return uncertain("A previous billing claim remains unconfirmed; the balance read failed and no claim was repeated")
			}
			return unsupported("An authoritative billing baseline is required before a new claim")
		}
		baseline := []string{}
		sawPlan, allExpired := false, true
		for _, plan := range plans {
			if plan["plan_id"] != request.PlanID {
				continue
			}
			sawPlan = true
			userPlan, _ := plan["user_plan_id"].(string)
			baseline = append(baseline, userPlan)
			state, _ := plan["status"].(string)
			if !strings.EqualFold(state, "expired") && (planEnd(plan) == 0 || planEnd(plan) > owner.options.Now().UnixMilli()) {
				allExpired = false
			}
			if exists && old.BaselineKnown && (old.State == "pending" || old.State == "unknown") && !containsString(old.Baseline, userPlan) {
				observed, _ := json.Marshal(map[string]any{"outcome": "observed", "plan": plan, "checkinDay": day, "checkinName": name})
				old.State, old.Result, old.EndsAt = "observed", observed, planEnd(plan)
				transaction.Journal.Claims[request.PlanID] = old
				if err := transaction.Save(); err != nil {
					return uncertain("An entitlement was observed but its reconciliation could not be persisted")
				}
				result = observed
				return nil
			}
		}
		if exists && (old.State == "pending" || old.State == "unknown") {
			return uncertain("A previous billing claim may have succeeded; no new entitlement proves its outcome and no claim was repeated")
		}
		if exists && old.State == "confirmed" && (old.EndsAt == 0 && (!sawPlan || !allExpired) || sawPlan && !allExpired) {
			result = old.Result
			return nil
		}
		if checkin == nil {
			if err := owner.claimPolicy(ctx, stream, account); err != nil {
				return err
			}
		}
		reference, err := nonce()
		if err != nil {
			return err
		}
		claim := controller.Claim{Plan: request.PlanID, Operation: call.Scope.Operation,
			Reference: reference, State: "pending", Generation: call.Scope.Generation, Baseline: baseline, BaselineKnown: true}
		transaction.Journal.Claims[request.PlanID] = claim
		if err := transaction.Save(); err != nil {
			return invalid("Billing claim could not be durably fenced")
		}
		if checkin == nil || !checkin.accepted {
			event := sdk.AcceptedEvent{OperationID: call.Scope.Operation}
			if checkin == nil {
				event.Reference = json.RawMessage(`{"reference":"` + reference + `"}`)
			}
			if err := stream.Emit(ctx, "accepted", event); err != nil {
				return uncertain("The billing claim was fenced but acceptance was not acknowledged")
			}
			if checkin != nil {
				checkin.accepted = true
			}
		}
		if err := invalidateUsage(ctx, stream); err != nil {
			if exists {
				transaction.Journal.Claims[request.PlanID] = old
			} else {
				delete(transaction.Journal.Claims, request.PlanID)
			}
			transaction.Save()
			return err
		}
		if checkin != nil {
			checkin.posted = true
		}
		status, body, headers, callErr := owner.controlDetailed(ctx, stream, account, "", http.MethodPost,
			"/api/v1/zcode-plan/billing/claim", map[string]string{"plan_id": request.PlanID})
		root, parseErr := protocol.ReadObject(body)
		code, codeValid := claimCode(root["code"])
		data := mapObject(root["data"])
		plan, valid := data["plan"].(map[string]any)
		_, hasPlan := data["plan"]
		if callErr != nil || parseErr != nil || !codeValid || root["success"] == false || root["error"] != nil ||
			root["data"] != nil && data == nil || hasPlan && !valid || valid && plan["plan_id"] != request.PlanID ||
			code == 0 && (status < 200 || status >= 300) ||
			code != 0 && code != 1003 && code != 1004 && code != 3002 && code != 3007 {
			claim.State = "unknown"
			transaction.Journal.Claims[request.PlanID] = claim
			transaction.Save()
			return uncertain("The billing claim response is unconfirmed or belongs to another plan; it will not be replayed")
		}
		claim.State, claim.EndsAt = "confirmed", planEnd(plan)
		received := owner.options.Now()
		outcome := "claimed"
		if valid && code == 0 {
			outcome = "confirmed"
		} else if code == 1003 {
			outcome = "done"
		} else if code == 1004 || code == 3002 || code == 3007 {
			outcome = map[float64]string{1004: "ineligible", 3002: "rate_limited", 3007: "captcha"}[code]
			claim.State = "refused"
			claim.RetryAt = received.Add(30 * time.Minute).UnixMilli()
			if code == 3002 {
				delay := retryDelay(headers, received, time.Duration(transaction.Journal.ClaimHoldDelay)*time.Millisecond*2)
				claim.RetryAt = transaction.MergeClaimHold(received.UnixMilli(), received.Add(delay).UnixMilli(), delay.Milliseconds())
			}
			if code == 3007 {
				claim.RetryAt = received.Add(24 * time.Hour).UnixMilli()
			}
		}
		if code != 3002 {
			transaction.MergeClaimHold(received.UnixMilli(), 0, 0)
		}
		value := map[string]any{"outcome": outcome, "refreshUsage": true}
		if claim.State == "confirmed" || outcome == "captcha" {
			value["checkinDay"], value["checkinName"] = day, name
		}
		if valid {
			value["plan"] = plan
		}
		if claim.RetryAt > 0 {
			value["until"] = claim.RetryAt
		}
		claim.Result, _ = json.Marshal(value)
		transaction.Journal.Claims[request.PlanID] = claim
		if err := transaction.Save(); err != nil {
			return uncertain("The billing claim may have succeeded but durable publication failed")
		}
		result = claim.Result
		return nil
	})
	return result, err
}
