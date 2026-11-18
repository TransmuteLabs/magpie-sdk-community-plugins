package zcode

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type resetSnapshot struct {
	Data          map[string]any
	Five          int64
	Week          int64
	FiveAvailable bool
	WeekAvailable bool
	Unread        bool
}

type resetControlState struct {
	Pending            bool   `json:"pending"`
	Window             string `json:"window,omitempty"`
	OpportunityPending bool   `json:"opportunityPending"`
	NextTryAt          int64  `json:"nextTryAt"`
	AckPending         bool   `json:"ackPending"`
}

func summarizeReset(saved controller.Reset) resetControlState {
	result := resetControlState{Pending: saved.Pending != nil, OpportunityPending: saved.OpportunityKey != "",
		NextTryAt: saved.OpportunityAfter, AckPending: saved.AckPending}
	if saved.Pending != nil {
		result.Window = saved.Pending.Window
	}
	return result
}

func summarizeResetStatus(snapshot resetSnapshot) map[string]any {
	result := map[string]any{"has_unread_history": snapshot.Unread}
	for _, field := range []string{"available_five_hour_resets", "available_week_resets"} {
		values := []any{}
		for _, value := range snapshot.Data[field].([]any) {
			values = append(values, map[string]any{"expire_at": mapObject(value)["expire_at"]})
		}
		result[field] = values
	}
	for _, field := range []string{"latest_five_hour_reset_history", "latest_week_reset_history"} {
		result[field] = nil
		if value := mapObject(snapshot.Data[field]); value != nil {
			result[field] = map[string]any{"used_at": value["used_at"]}
		}
	}
	return result
}

func (owner *module) resetSummary(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	if err := owner.resetMaterial(account); err != nil {
		return nil, err
	}
	var result any
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		snapshot, err := owner.readReset(ctx, stream, account)
		if err != nil {
			return err
		}
		result = struct {
			resetControlState
			Status map[string]any `json:"status"`
		}{summarizeReset(transaction.Journal.Reset[call.Scope.Account]), summarizeResetStatus(snapshot)}
		return nil
	})
	return result, err
}

func (owner *module) resetReconcile(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if decode(call.Params, &request) != nil || call.Scope.Operation == "" {
		return nil, invalid("Reset reconciliation requires a host-owned write operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	if err := owner.resetMaterial(account); err != nil {
		return nil, err
	}
	var result any
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		snapshot, err := owner.readReset(ctx, stream, account)
		if err != nil {
			return uncertain("Reset debt remains unchanged because authoritative history could not be read")
		}
		saved := transaction.Journal.Reset[call.Scope.Account]
		observed := observeReset(&saved, snapshot)
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return err
		}
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return uncertain("Reset history observations could not be persisted")
		}
		outcome := "idle"
		if observed {
			outcome = "observed"
		} else if saved.Pending != nil || saved.OpportunityKey != "" {
			outcome = "unconfirmed"
		}
		result = map[string]any{"outcome": outcome, "state": summarizeReset(saved), "refreshUsage": observed}
		return nil
	})
	return result, err
}

func wholeTimestamp(value any) (int64, bool) {
	number, valid := finiteNumber(value)
	if !valid || number <= 0 || number >= float64(math.MaxInt64) || number != math.Trunc(number) {
		return 0, false
	}
	return int64(number), true
}

func (owner *module) resetMaterial(account Credentials) error {
	if account.Kind != "individual" && account.Kind != "team" || account.BusinessToken == "" ||
		account.BusinessExpiresAt <= owner.options.Now().Add(30*time.Second).UnixMilli() {
		return &sdk.Error{Code: "credentials_required", Message: "Reset controls require selected paid business and ZCode session credentials", Outcome: "not-started"}
	}
	return nil
}

func (owner *module) readReset(ctx context.Context, stream *sdk.Stream, account Credentials) (resetSnapshot, error) {
	status, body, err := owner.control(ctx, stream, account, "reset", http.MethodGet, "/api/v1/coding-plan/reset/status", nil)
	if err != nil {
		return resetSnapshot{}, err
	}
	data, err := controlObject(status, body, true)
	if err != nil {
		return resetSnapshot{}, err
	}
	unread, valid := data["has_unread_history"].(bool)
	if !valid {
		return resetSnapshot{}, protocol.ErrInvalid
	}
	result := resetSnapshot{Data: data, Unread: unread}
	for _, field := range []string{"available_five_hour_resets", "available_week_resets"} {
		items, valid := data[field].([]any)
		if !valid || len(items) > 4096 {
			return resetSnapshot{}, protocol.ErrInvalid
		}
		for _, item := range items {
			entry, valid := item.(map[string]any)
			expires, validTime := wholeTimestamp(entry["expire_at"])
			if !valid || !validTime {
				return resetSnapshot{}, protocol.ErrInvalid
			}
			if expires > owner.options.Now().UnixMilli() {
				if field == "available_five_hour_resets" {
					result.FiveAvailable = true
				} else {
					result.WeekAvailable = true
				}
			}
		}
	}
	for _, field := range []string{"latest_five_hour_reset_history", "latest_week_reset_history"} {
		value, present := data[field]
		if !present {
			return resetSnapshot{}, protocol.ErrInvalid
		}
		if value == nil {
			continue
		}
		entry, valid := value.(map[string]any)
		used, validTime := wholeTimestamp(entry["used_at"])
		if !valid || !validTime {
			return resetSnapshot{}, protocol.ErrInvalid
		}
		if field == "latest_five_hour_reset_history" {
			result.Five = used
		} else {
			result.Week = used
		}
	}
	return result, nil
}

func observeReset(saved *controller.Reset, snapshot resetSnapshot) bool {
	saved.FiveHigh, saved.WeekHigh = max(saved.FiveHigh, snapshot.Five), max(saved.WeekHigh, snapshot.Week)
	if snapshot.Unread && (saved.FiveHigh > 0 || saved.WeekHigh > 0) {
		saved.AckPending = true
	}
	if pending := saved.Pending; pending != nil {
		used := snapshot.Five
		if pending.Window == "WEEK" {
			used = snapshot.Week
		}
		if used > pending.Baseline && used > pending.HighWater {
			pending.ObservedAt = used
		}
		if pending.Accepted && pending.ObservedAt > 0 {
			saved.Pending = nil
			return true
		}
	}
	return false
}

func (owner *module) resetStatus(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	if err := owner.resetMaterial(account); err != nil {
		return nil, err
	}
	if _, _, err := owner.principal(ctx, stream, account, ""); err != nil {
		return nil, err
	}
	snapshot, err := owner.readReset(ctx, stream, account)
	return snapshot.Data, err
}

func (owner *module) resetUse(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Window string `json:"window"`
	}
	if decode(call.Params, &request) != nil || request.Window != "FIVE_HOUR" && request.Window != "WEEK" || call.Scope.Operation == "" {
		return nil, invalid("Reset requires an exact FIVE_HOUR/WEEK window and host-owned write operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	if err := owner.resetMaterial(account); err != nil {
		return nil, err
	}
	var result any
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		saved := transaction.Journal.Reset[call.Scope.Account]
		snapshot, err := owner.readReset(ctx, stream, account)
		if err != nil {
			if saved.Pending != nil {
				return uncertain("A previous reset remains unconfirmed and its status could not be read")
			}
			return unsupported("Reset availability and baseline history must be read before spending a reset")
		}
		hadPending := saved.Pending != nil
		observed := observeReset(&saved, snapshot)
		if hadPending {
			transaction.Journal.Reset[call.Scope.Account] = saved
			if err := transaction.Save(); err != nil {
				return uncertain("Reset observations could not be persisted")
			}
			result = map[string]any{"outcome": "unconfirmed", "state": saved}
			if observed {
				result = map[string]any{"outcome": "observed", "state": saved, "refreshUsage": true}
			}
			return nil
		}
		if request.Window == "FIVE_HOUR" && !snapshot.FiveAvailable || request.Window == "WEEK" && !snapshot.WeekAvailable {
			return unsupported("No unexpired reset is available for the selected window")
		}
		key, err := nonce()
		if err != nil {
			return err
		}
		baseline, high := snapshot.Five, saved.FiveHigh
		if request.Window == "WEEK" {
			baseline, high = snapshot.Week, saved.WeekHigh
		}
		saved.Pending = &controller.ResetClaim{Key: key, Window: request.Window, Operation: call.Scope.Operation,
			Generation: call.Scope.Generation, Baseline: baseline, HighWater: high}
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return invalid("Reset could not be durably fenced")
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation,
			Reference: json.RawMessage(`{"reference":"` + key + `"}`)}); err != nil {
			return uncertain("Reset was fenced but acceptance was not acknowledged")
		}
		if err := invalidateUsage(ctx, stream); err != nil {
			saved.Pending = nil
			transaction.Journal.Reset[call.Scope.Account] = saved
			transaction.Save()
			return err
		}
		status, body, callErr := owner.control(ctx, stream, account, "reset", http.MethodPost,
			"/api/v1/coding-plan/reset/use", map[string]string{"idempotency_key": key, "reset_type": request.Window})
		var data map[string]any
		if callErr == nil {
			data, callErr = controlObject(status, body, true)
		}
		if callErr != nil || data["used"] != true {
			return uncertain("Reset may have been spent; its response is unconfirmed and no POST will be repeated")
		}
		saved.Pending.Accepted = true
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return uncertain("Reset acceptance could not be persisted")
		}
		if after, readErr := owner.readReset(ctx, stream, account); readErr == nil {
			observed = observeReset(&saved, after)
			transaction.Journal.Reset[call.Scope.Account] = saved
			if err := transaction.Save(); err != nil {
				return uncertain("Reset history observation could not be persisted")
			}
		}
		outcome := "unconfirmed"
		if observed {
			outcome = "observed"
		}
		result = map[string]any{"outcome": outcome, "state": saved, "refreshUsage": observed}
		return nil
	})
	return result, err
}

func (owner *module) resetResolve(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Acknowledgment string `json:"acknowledgment"`
	}
	if decode(call.Params, &request) != nil || request.Acknowledgment != "vendor-may-have-spent-reset" || call.Scope.Operation == "" {
		return nil, invalid("Resolving uncertainty requires the explicit acknowledgment vendor-may-have-spent-reset")
	}
	_, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		saved := transaction.Journal.Reset[call.Scope.Account]
		if saved.Pending == nil {
			return invalid("There is no pending reset to resolve")
		}
		saved.Pending = nil
		transaction.Journal.Reset[call.Scope.Account] = saved
		return transaction.Save()
	})
	return map[string]any{"outcome": "acknowledged-uncertainty", "replayed": false}, err
}

func (owner *module) resetOpportunity(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if decode(call.Params, &request) != nil || call.Scope.Operation == "" {
		return nil, invalid("Reset opportunity requires a host-owned write operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	if err := owner.resetMaterial(account); err != nil {
		return nil, err
	}
	var result any
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		saved := transaction.Journal.Reset[call.Scope.Account]
		if saved.Pending != nil || saved.OpportunityKey != "" {
			return uncertain("A reset or opportunity remains unconfirmed; no opportunity POST was repeated")
		}
		now := owner.options.Now()
		if saved.OpportunityAfter > now.UnixMilli() {
			result = map[string]any{"granted": false, "nextTryAt": saved.OpportunityAfter, "outcome": "cooldown"}
			return nil
		}
		key, err := nonce()
		if err != nil {
			return err
		}
		saved.OpportunityKey, saved.OpportunityAfter = key, now.Add(5*time.Minute).UnixMilli()
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return invalid("Reset opportunity could not be durably fenced")
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation,
			Reference: json.RawMessage(`{"reference":"` + key + `"}`)}); err != nil {
			return uncertain("Reset opportunity was fenced but acceptance was not acknowledged")
		}
		status, body, callErr := owner.control(ctx, stream, account, "reset", http.MethodPost,
			"/api/v1/coding-plan/reset/opportunity", map[string]string{"idempotency_key": key})
		if callErr != nil || status < 200 || status >= 300 {
			return uncertain("Reset opportunity has an unconfirmed result; its idempotency key remains fenced")
		}
		root, callErr := protocol.ReadObject(body)
		code, valid := finiteNumber(root["code"])
		data, dataValid := root["data"].(map[string]any)
		granted, grantedValid := data["granted"].(bool)
		if callErr != nil || root["success"] == false || !valid || !dataValid || !grantedValid ||
			!(code == 0 && granted || code == 3301 && !granted) {
			return uncertain("Reset opportunity returned malformed or ambiguous success")
		}
		after := owner.options.Now().Add(10 * time.Minute).UnixMilli()
		if !granted {
			next, valid := wholeTimestamp(data["next_try_at"])
			if !valid {
				return uncertain("Reset opportunity refusal omitted its retry boundary")
			}
			after = max(next, owner.options.Now().Add(5*time.Minute).UnixMilli())
		}
		saved.OpportunityKey, saved.OpportunityAfter = "", after
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return uncertain("Reset opportunity may have completed but durable publication failed")
		}
		result = map[string]any{"granted": granted, "nextTryAt": after}
		return nil
	})
	return result, err
}

func (owner *module) resetOpportunityResolve(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Acknowledgment string `json:"acknowledgment"`
	}
	if decode(call.Params, &request) != nil || request.Acknowledgment != "vendor-may-have-granted-reset" || call.Scope.Operation == "" {
		return nil, invalid("Resolving opportunity uncertainty requires the explicit acknowledgment vendor-may-have-granted-reset")
	}
	_, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		saved := transaction.Journal.Reset[call.Scope.Account]
		if saved.OpportunityKey == "" {
			return invalid("There is no pending reset opportunity to resolve")
		}
		saved.OpportunityKey = ""
		transaction.Journal.Reset[call.Scope.Account] = saved
		return transaction.Save()
	})
	return map[string]any{"outcome": "acknowledged-uncertainty", "replayed": false}, err
}

func (owner *module) resetAcknowledge(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if decode(call.Params, &request) != nil || call.Scope.Operation == "" {
		return nil, invalid("Reset history acknowledgment requires a host-owned write operation")
	}
	account, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	if err := owner.resetMaterial(account); err != nil {
		return nil, err
	}
	var result any
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		snapshot, err := owner.readReset(ctx, stream, account)
		if err != nil {
			return unsupported("Reset history cannot be acknowledged without an authoritative status read")
		}
		saved := transaction.Journal.Reset[call.Scope.Account]
		observed := observeReset(&saved, snapshot)
		if !snapshot.Unread {
			saved.AckPending = false
			transaction.Journal.Reset[call.Scope.Account] = saved
			if err := transaction.Save(); err != nil {
				return uncertain("Observed acknowledgment could not be persisted")
			}
			result = map[string]any{"outcome": "already-acknowledged", "refreshUsage": observed}
			return nil
		}
		saved.AckPending = true
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return invalid("Reset history acknowledgment could not be durably recorded")
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return err
		}
		status, body, callErr := owner.control(ctx, stream, account, "reset-history", http.MethodPost,
			"/api/v1/coding-plan/reset/history/read", nil)
		root, parseErr := protocol.ReadObject(body)
		code, valid := finiteNumber(root["code"])
		if callErr != nil || status < 200 || status >= 300 || parseErr != nil || !valid || code != 0 || root["success"] == false {
			return uncertain("Reset history acknowledgment is unconfirmed")
		}
		saved.AckPending = false
		transaction.Journal.Reset[call.Scope.Account] = saved
		if err := transaction.Save(); err != nil {
			return uncertain("Reset history acknowledgment could not be persisted")
		}
		result = map[string]any{"outcome": "acknowledged", "refreshUsage": observed}
		return nil
	})
	return result, err
}
