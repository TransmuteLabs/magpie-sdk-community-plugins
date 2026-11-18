package zcode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type marketingLocaleKey struct{}
type checkinCallKey struct{}

var marketingSequence atomic.Uint64

type giftOffer struct {
	PlanID string `json:"planId"`
	Name   string `json:"name"`
}

func claimCode(value any) (float64, bool) {
	text := ""
	switch value := value.(type) {
	case json.Number:
		text = string(value)
	case string:
		text = value
	}
	number, err := strconv.ParseUint(text, 10, 32)
	return float64(number), err == nil
}

func retryDelay(headers http.Header, now time.Time, fallback time.Duration) time.Duration {
	text := headers.Get("Retry-After")
	if seconds, err := strconv.ParseUint(text, 10, 32); err == nil && seconds > 0 {
		return min(time.Duration(seconds)*time.Second, 6*time.Hour)
	}
	if date, err := http.ParseTime(text); err == nil && date.After(now) {
		return min(date.Sub(now), 6*time.Hour)
	}
	return min(max(fallback, 30*time.Minute), 6*time.Hour)
}

func marketingLocale(body json.RawMessage) (string, error) {
	var request struct {
		Locale string `json:"locale"`
	}
	if err := decode(body, &request); err != nil {
		return "", err
	}
	if request.Locale == "" {
		request.Locale = "en-US"
	}
	if !slices.Contains([]string{"zh-CN", "en-US", "ja-JP", "ru-RU", "ko-KR"}, request.Locale) {
		return "", invalid("Use an exact locale supported by the pinned ZCode client")
	}
	return request.Locale, nil
}

func feedOffers(data map[string]any) ([]giftOffer, error) {
	offers := []giftOffer{}
	if data["deliveries"] == nil {
		return offers, nil
	}
	deliveries, valid := data["deliveries"].([]any)
	if !valid || len(deliveries) > 256 {
		return nil, protocol.ErrInvalid
	}
	seen := map[string]bool{}
	nodes := 0
	for _, delivery := range deliveries {
		name := ""
		ids := []string{}
		var walk func(any, int) error
		walk = func(value any, depth int) error {
			nodes++
			if nodes > 16384 || depth > 12 {
				return protocol.ErrInvalid
			}
			switch value := value.(type) {
			case []any:
				for _, child := range value {
					if err := walk(child, depth+1); err != nil {
						return err
					}
				}
			case map[string]any:
				if value["type"] == "claim_zcode_plan" {
					id, valid := mapObject(value["args"])["plan_id"].(string)
					if !valid || !safeText(id) || len(id) > 128 {
						return protocol.ErrInvalid
					}
					ids = append(ids, id)
				}
				if text, valid := mapObject(value["zcode_plan"])["name"].(string); valid && safeText(text) && len(text) <= 256 {
					name = text
				}
				keys := make([]string, 0, len(value))
				for key := range value {
					keys = append(keys, key)
				}
				slices.Sort(keys)
				for _, key := range keys {
					if err := walk(value[key], depth+1); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := walk(delivery, 0); err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !seen[id] {
				label := name
				if label == "" {
					label = id
				}
				offers = append(offers, giftOffer{PlanID: id, Name: label})
				seen[id] = true
			}
		}
	}
	return offers, nil
}

func (owner *module) readOffers(ctx context.Context, stream *sdk.Stream, account Credentials, fallback time.Duration) ([]giftOffer, time.Duration, error) {
	path := "/api/v1/marketing/touch?seq=" + strconv.FormatUint(marketingSequence.Add(1)-1, 10)
	status, body, headers, err := owner.controlDetailed(ctx, stream, account, "", http.MethodGet, path, nil)
	if err != nil {
		return nil, 0, err
	}
	root, err := protocol.ReadObject(body)
	code, valid := claimCode(root["code"])
	if status == 429 || valid && code == 3002 {
		return nil, retryDelay(headers, owner.options.Now(), fallback), &sdk.Error{
			Code: "rate_limit_error", Message: "ZCode marketing feed rate limited this principal", Outcome: "not-started"}
	}
	if err != nil || !valid || code != 0 || status < 200 || status >= 300 ||
		root["success"] == false || root["error"] != nil {
		return nil, 0, unsupported("The ZCode feed could not be confirmed; no offers were inferred")
	}
	data := mapObject(root["data"])
	if root["data"] != nil && data == nil {
		return nil, 0, protocol.ErrInvalid
	}
	offers, err := feedOffers(data)
	return offers, 0, err
}

func (owner *module) billingOffers(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	locale, err := marketingLocale(call.Params)
	if err != nil {
		return nil, err
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var offers []giftOffer
	var readErr error
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		now := owner.options.Now()
		if transaction.Journal.ClaimHoldUntil > now.UnixMilli() {
			return &sdk.Error{Code: "rate_limit_error", Message: "ZCode marketing reads are held for this principal", Outcome: "not-started"}
		}
		if entry := transaction.Journal.Checkin; entry != nil && entry.Pending && entry.Until > now.UnixMilli() {
			return uncertain("Another check-in is pending; no concurrent feed was requested")
		}
		var rate time.Duration
		offers, rate, readErr = owner.readOffers(context.WithValue(ctx, marketingLocaleKey{}, locale), stream, account,
			time.Duration(transaction.Journal.ClaimHoldDelay)*time.Millisecond*2)
		if rate > 0 {
			transaction.Journal.ClaimHoldUntil, transaction.Journal.ClaimHoldDelay = now.Add(rate).UnixMilli(), rate.Milliseconds()
		} else if readErr == nil {
			transaction.Journal.ClaimHoldUntil, transaction.Journal.ClaimHoldDelay = 0, 0
		}
		return transaction.Save()
	})
	if err != nil {
		return nil, err
	}
	err = readErr
	return map[string]any{"offers": offers}, err
}

func (owner *module) billingCheckin(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	locale, err := marketingLocale(call.Params)
	if err != nil {
		return nil, err
	}
	if call.Scope.Operation == "" {
		return nil, invalid("Check-in requires a host-owned operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	now := owner.options.Now()
	var cached json.RawMessage
	var delay int64
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		previous := transaction.Journal.Checkin
		delay = transaction.Journal.ClaimHoldDelay
		if transaction.Journal.ClaimHoldUntil > now.UnixMilli() {
			cached, _ = json.Marshal(map[string]any{"outcome": "rate_limited", "until": transaction.Journal.ClaimHoldUntil})
			return nil
		}
		if previous != nil {
			delay = max(delay, previous.Delay)
			if previous.Until > now.UnixMilli() {
				if previous.Pending {
					return uncertain("Another check-in remains pending; no feed or claim was repeated")
				}
				cached = previous.Result
				return nil
			}
		}
		transaction.Journal.Checkin = &controller.Checkin{Operation: call.Scope.Operation, Pending: true,
			Until: now.Add(2 * time.Minute).UnixMilli(), Delay: delay}
		return transaction.Save()
	})
	if err != nil || cached != nil {
		return cached, err
	}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, uncertain("Check-in was fenced but its acceptance was not acknowledged")
	}
	ctx = context.WithValue(context.WithValue(ctx, marketingLocaleKey{}, locale), checkinCallKey{}, true)
	offers, rate, readErr := owner.readOffers(ctx, stream, account, time.Duration(delay)*time.Millisecond*2)
	results := []json.RawMessage{}
	outcome := "inactive"
	until := now.Add(30 * time.Minute)
	if readErr == nil && len(offers) > 0 {
		outcome = "done"
		until = time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day()+1, 0, 0, 0, 0, time.UTC)
		policyErr := owner.claimPolicy(ctx, stream, account)
		var failure *sdk.Error
		if errors.As(policyErr, &failure) && failure.Code == "verification_required" {
			outcome, offers = "captcha", nil
		} else if policyErr != nil {
			readErr, offers = policyErr, nil
		}
		for _, offer := range offers {
			claim := call
			claim.Params = rawClaimPlan(offer.PlanID)
			result, err := owner.billingClaim(ctx, claim, stream)
			if err != nil {
				readErr = err
				break
			}
			body, _ := json.Marshal(result)
			results = append(results, body)
			value, _ := protocol.ReadObject(body)
			switch value["outcome"] {
			case "claimed", "confirmed", "observed":
				if outcome != "captcha" && outcome != "rate_limited" {
					outcome = "claimed"
				}
			case "captcha":
				outcome = "captcha"
			case "ineligible":
				if outcome == "done" {
					outcome = "ineligible"
				}
				until = minTime(until, now.Add(30*time.Minute))
			case "rate_limited":
				outcome = "rate_limited"
				if after, valid := metric(value["until"]); valid && after > float64(now.UnixMilli()) {
					rate = time.Duration(int64(after)-now.UnixMilli()) * time.Millisecond
				}
			}
			if outcome == "rate_limited" || outcome == "captcha" {
				break
			}
		}
	}
	if rate > 0 {
		until, delay, outcome = now.Add(rate), rate.Milliseconds(), "rate_limited"
	}
	value := map[string]any{"outcome": outcome, "results": results, "until": until.UnixMilli()}
	body, _ := json.Marshal(value)
	saveErr := store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		entry := transaction.Journal.Checkin
		if entry == nil || entry.Operation != call.Scope.Operation {
			return uncertain("Check-in ownership changed")
		}
		if readErr != nil && rate == 0 {
			entry.Pending, entry.Until = true, now.UnixMilli()
		} else {
			entry.Pending, entry.Result, entry.Until, entry.Delay = false, body, until.UnixMilli(), delay
			if rate > 0 {
				transaction.Journal.ClaimHoldUntil, transaction.Journal.ClaimHoldDelay = until.UnixMilli(), rate.Milliseconds()
			} else {
				entry.Delay = 0
			}
		}
		return transaction.Save()
	})
	if saveErr != nil {
		return nil, uncertain("Check-in result could not be durably published")
	}
	if readErr != nil && rate == 0 {
		return nil, readErr
	}
	return json.RawMessage(body), nil
}

func rawClaimPlan(id string) json.RawMessage {
	body, _ := json.Marshal(map[string]string{"planId": id})
	return body
}

func minTime(first, second time.Time) time.Time {
	if first.Before(second) {
		return first
	}
	return second
}
