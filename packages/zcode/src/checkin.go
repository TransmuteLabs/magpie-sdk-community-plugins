package zcode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type marketingLocaleKey struct{}
type checkinCallKey struct{}

const accountCheckinAction = "account.checkin"
const checkinInterval = 30 * time.Minute

var beijing = time.FixedZone("Beijing", 8*60*60)
var marketingSequence atomic.Uint64

type giftOffer struct {
	PlanID string `json:"planId"`
	Name   string `json:"name"`
}

type giftPause struct {
	until int64
	delay time.Duration
}

type checkinClaim struct {
	day, name string
	accepted  bool
	posted    bool
}

type giftClaimAnswer struct {
	Outcome string `json:"outcome"`
	Until   int64  `json:"until"`
	Day     string `json:"checkinDay"`
	Name    string `json:"checkinName"`
}

type checkinReply struct {
	Outcome string            `json:"outcome"`
	Results []json.RawMessage `json:"results,omitempty"`
	Until   int64             `json:"until"`
	Message string            `json:"message,omitempty"`
}

type checkinRecord struct {
	checkinReply
	Day string `json:"checkinDay"`
}

func zcodeCheckinDay(now time.Time) string {
	return now.In(beijing).Format(time.DateOnly)
}

func claimAnswer(body json.RawMessage) giftClaimAnswer {
	var answer giftClaimAnswer
	if json.Unmarshal(body, &answer) != nil {
		return giftClaimAnswer{}
	}
	return answer
}

func giftNames(prefix string, names []string) string {
	var label strings.Builder
	label.WriteString(prefix)
	slices.Sort(names)
	for index, name := range slices.Compact(names) {
		if label.Len()+len(name)+2 > 4096 {
			break
		}
		if index > 0 {
			label.WriteString(", ")
		}
		label.WriteString(name)
	}
	return label.String()
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
	return min(max(fallback, checkinInterval), 6*time.Hour)
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

func (owner *module) readOffers(ctx context.Context, stream *sdk.Stream, account Credentials, fallback time.Duration) ([]giftOffer, giftPause, error) {
	path := "/api/v1/marketing/touch?seq=" + strconv.FormatUint(marketingSequence.Add(1)-1, 10)
	status, body, headers, err := owner.controlDetailed(ctx, stream, account, "", http.MethodGet, path, nil)
	if err != nil {
		return nil, giftPause{}, err
	}
	root, err := protocol.ReadObject(body)
	code, valid := claimCode(root["code"])
	if status == 429 || valid && code == 3002 {
		now := owner.options.Now()
		delay := retryDelay(headers, now, fallback)
		return nil, giftPause{until: now.Add(delay).UnixMilli(), delay: delay}, &sdk.Error{
			Code: "rate_limit_error", Message: "ZCode marketing feed rate limited this principal", Outcome: "not-started"}
	}
	if err != nil || !valid || code != 0 || status < 200 || status >= 300 ||
		root["success"] == false || root["error"] != nil {
		return nil, giftPause{}, unsupported("The ZCode feed could not be confirmed; no offers were inferred")
	}
	data := mapObject(root["data"])
	if root["data"] != nil && data == nil {
		return nil, giftPause{}, protocol.ErrInvalid
	}
	offers, err := feedOffers(data)
	return offers, giftPause{}, err
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
		var pause giftPause
		offers, pause, readErr = owner.readOffers(context.WithValue(ctx, marketingLocaleKey{}, locale), stream, account,
			time.Duration(transaction.Journal.ClaimHoldDelay)*time.Millisecond*2)
		if pause.until != 0 || readErr == nil {
			transaction.MergeClaimHold(owner.options.Now().UnixMilli(), pause.until, pause.delay.Milliseconds())
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
	return owner.runCheckin(ctx, call, stream, zcodeCheckinDay(owner.options.Now()), locale)
}

func (owner *module) accountCheckin(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AccountCheckinRequest
	if err := decode(call.Params, &request); err != nil || request.Validate() != nil || call.Scope.Session != "" ||
		request.Day != zcodeCheckinDay(owner.options.Now()) {
		return nil, invalid("Check-in requires the scheduler's selected account and day")
	}
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	if account.SessionToken == "" {
		return sdk.AccountCheckinResult{Outcome: "inactive", Message: "Gift check-in requires a ZCode session"}, nil
	}
	reply, err := owner.runCheckin(ctx, call, stream, request.Day, "en-US")
	if err != nil {
		return nil, err
	}
	result := sdk.AccountCheckinResult{Outcome: reply.Outcome, Message: reply.Message}
	until := reply.Until
	switch reply.Outcome {
	case "rate_limited":
		result.Outcome = sdk.AccountCheckinRateLimited
	case "captcha":
		return result, nil
	default:
		dayStart, _ := time.ParseInLocation(time.DateOnly, request.Day, beijing)
		until = min(until, dayStart.AddDate(0, 0, 1).UnixMilli())
	}
	result.Until = time.UnixMilli(until).UTC().Format(time.RFC3339Nano)
	return result, nil
}

func (owner *module) runCheckin(ctx context.Context, call sdk.Call, stream *sdk.Stream, day, locale string) (checkinReply, error) {
	if call.Scope.Operation == "" {
		return checkinReply{}, invalid("Check-in requires a host-owned operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return checkinReply{}, err
	}
	now := owner.options.Now()
	var reply checkinReply
	var delay int64
	settled := map[string]giftClaimAnswer{}
	var checked []string
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		previous := transaction.Journal.Checkin
		delay = transaction.Journal.ClaimHoldDelay
		if transaction.Journal.ClaimHoldUntil > now.UnixMilli() {
			reply = checkinReply{Outcome: "rate_limited", Until: transaction.Journal.ClaimHoldUntil, Message: "ZCode rate limited this account"}
			return nil
		}
		if previous != nil {
			delay = max(delay, previous.Delay)
			if previous.Until > now.UnixMilli() {
				if previous.Pending {
					return uncertain("Another check-in remains pending; no feed or claim was repeated")
				}
				var record checkinRecord
				if err := json.Unmarshal(previous.Result, &record); err != nil {
					return err
				}
				if record.Day == day {
					reply = record.checkinReply
					return nil
				}
			}
		}
		for plan, claim := range transaction.Journal.Claims {
			answer := claimAnswer(claim.Result)
			if answer.Day != day {
				continue
			}
			settled[plan] = answer
			if claim.State == "confirmed" || claim.State == "observed" {
				checked = append(checked, answer.Name)
			}
		}
		transaction.Journal.Checkin = &controller.Checkin{Operation: call.Scope.Operation, Pending: true,
			Until: now.Add(2 * time.Minute).UnixMilli(), Delay: delay}
		return transaction.Save()
	})
	if err != nil || reply.Outcome != "" {
		return reply, err
	}
	checkin := &checkinClaim{day: day}
	ctx = context.WithValue(context.WithValue(ctx, marketingLocaleKey{}, locale), checkinCallKey{}, checkin)
	offers, pause, readErr := owner.readOffers(ctx, stream, account, time.Duration(delay)*time.Millisecond*2)
	if readErr != nil && pause.until == 0 && ctx.Err() == nil {
		readErr = &sdk.Error{Code: "upstream", Message: readErr.Error(), Outcome: "not-started"}
	}
	reply = checkinReply{Outcome: "inactive", Until: now.Add(checkinInterval).UnixMilli()}
	var claimed []string
	captcha, needsVerification, ineligible := false, false, false
	if readErr == nil && len(offers) > 0 {
		offers = slices.DeleteFunc(offers, func(offer giftOffer) bool {
			answer, found := settled[offer.PlanID]
			captcha = captcha || answer.Outcome == "captcha"
			return found
		})
	}
	if readErr == nil && len(offers) > 0 {
		policyErr := owner.claimPolicy(ctx, stream, account)
		var failure *sdk.Error
		if errors.As(policyErr, &failure) && failure.Code == "verification_required" {
			needsVerification, offers = true, nil
		} else if policyErr != nil {
			readErr, offers = policyErr, nil
		}
		for _, offer := range offers {
			checkin.name, checkin.posted = offer.Name, false
			claim := call
			claim.Params = rawClaimPlan(offer.PlanID)
			result, err := owner.billingClaim(ctx, claim, stream)
			if err != nil {
				readErr = err
				break
			}
			body, _ := json.Marshal(result)
			reply.Results = append(reply.Results, body)
			answer := claimAnswer(body)
			switch answer.Outcome {
			case "claimed", "confirmed":
				if checkin.posted {
					claimed = append(claimed, offer.Name)
				} else {
					checked = append(checked, offer.Name)
				}
			case "done", "observed":
				checked = append(checked, offer.Name)
			case "captcha":
				captcha = true
				needsVerification = checkin.posted
			case "ineligible":
				ineligible = true
			case "rate_limited":
				if after := time.UnixMilli(answer.Until).Sub(now); after > 0 {
					pause = giftPause{until: answer.Until, delay: after}
				}
			default:
				readErr = uncertain("The stored ZCode claim result could not be confirmed")
			}
			if pause.until != 0 || needsVerification || readErr != nil {
				break
			}
		}
	}
	switch {
	case needsVerification || captcha && len(claimed) == 0 && len(checked) == 0:
		dayStart, _ := time.ParseInLocation(time.DateOnly, day, beijing)
		reply.Outcome, reply.Until, reply.Message = "captcha", dayStart.AddDate(0, 0, 1).UnixMilli(), "Complete verification in the ZCode app"
	case len(claimed) > 0:
		reply.Outcome, reply.Message = "claimed", giftNames("Claimed: ", claimed)
	case len(checked) > 0:
		reply.Outcome, reply.Message = "done", giftNames("Already claimed: ", checked)
	case ineligible:
		reply.Outcome, reply.Message = "ineligible", "ZCode did not offer this account the listed gift plan yet"
	}
	saveErr := store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		entry := transaction.Journal.Checkin
		if entry == nil || entry.Operation != call.Scope.Operation {
			return uncertain("Check-in ownership changed")
		}
		if readErr != nil && pause.until == 0 {
			entry.Pending, entry.Until = true, now.UnixMilli()
		} else {
			until := transaction.MergeClaimHold(owner.options.Now().UnixMilli(), pause.until, pause.delay.Milliseconds())
			if until != 0 {
				reply.Outcome, reply.Until, reply.Message = "rate_limited", until, "ZCode rate limited this account"
			}
			body, _ := json.Marshal(checkinRecord{checkinReply: reply, Day: day})
			entry.Pending, entry.Result, entry.Until, entry.Delay = false, body, reply.Until, transaction.Journal.ClaimHoldDelay
		}
		return transaction.Save()
	})
	if saveErr != nil {
		return checkinReply{}, uncertain("Check-in result could not be durably published")
	}
	if readErr != nil && pause.until == 0 {
		return checkinReply{}, readErr
	}
	return reply, nil
}

func rawClaimPlan(id string) json.RawMessage {
	body, _ := json.Marshal(map[string]string{"planId": id})
	return body
}
