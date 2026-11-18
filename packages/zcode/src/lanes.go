package zcode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func aliasModels() []sdk.Model {
	var aliases []sdk.Model
	for _, base := range Models()[:2] {
		for _, suffix := range []string{"-OffPeak", "-Highspeed"} {
			value := base
			value.ID, value.Name = base.ID+suffix, base.Name+suffix
			aliases = append(aliases, value)
		}
	}
	return aliases
}

func rawLaneModel(model string) string {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "glm-5.3-flash-") {
		return "GLM-5.3-Flash"
	}
	return "GLM-5.3"
}

func nonce() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func uncertain(message string) error {
	return &sdk.Error{Code: "operation_uncertain", Message: message, Outcome: "uncertain"}
}

func validateTicket(value map[string]any, entry controller.Execution) (controller.Execution, error) {
	ticket, _ := value["ticket_id"].(string)
	state, _ := value["state"].(string)
	task, _ := value["task_id"].(string)
	if !safeText(ticket) || task != "" && task != entry.Task {
		return entry, protocol.ErrInvalid
	}
	switch state {
	case "queued", "ready", "active", "expired", "settled", "not_found":
	default:
		return entry, protocol.ErrInvalid
	}
	if entry.Ticket != "" && entry.Ticket != ticket {
		return entry, protocol.ErrInvalid
	}
	entry.Ticket, entry.State = ticket, state
	return entry, nil
}

func (owner *module) admit(ctx context.Context, stream *sdk.Stream, call sdk.Call, account Credentials, store *controller.Store, principal, login, lane, model string) (controller.Execution, bool, error) {
	key := controller.EntryKey(call.Scope.Account, call.Scope.Session)
	var selected controller.Execution
	accepted := false
	err := store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		if entry, exists := transaction.Journal.Executions[key]; exists {
			if entry.Account != call.Scope.Account || entry.Task != call.Scope.Session || entry.Login != login || entry.Lane != lane || entry.Model != model {
				return uncertain("The stored native execution belongs to another login, task, model or lane; explicit reconciliation is required")
			}
			switch entry.State {
			case "pending", "unknown", "settling":
				return uncertain("A previous native admission or settlement remains unconfirmed; no ticket/card was requested again")
			case "queued", "ready", "active":
				if entry.InFlight != "" {
					return uncertain("A native inference remains in flight or unconfirmed; it will not be replayed")
				}
				selected = entry
				return nil
			default:
				return unsupported("The stored native execution is terminal; a new explicit conversation is required")
			}
		}
		if lane == "highspeed" {
			if transaction.Journal.DrawAfter[call.Scope.Account] > owner.options.Now().UnixMilli() {
				return unsupported("Highspeed draw is in its recorded cooldown")
			}
			for _, entry := range transaction.Journal.Executions {
				if entry.Account == call.Scope.Account && entry.Lane == "highspeed" && (entry.State == "pending" || entry.State == "unknown") {
					return uncertain("Another Highspeed draw has an unresolved result")
				}
			}
		}
		reference, err := nonce()
		if err != nil {
			return err
		}
		entry := controller.Execution{
			Account: call.Scope.Account, Login: login, Generation: call.Scope.Generation,
			Task: call.Scope.Session, Lane: lane, Model: model, Operation: call.Scope.Operation,
			Reference: reference, State: "pending", Outcomes: map[string]string{},
		}
		transaction.Journal.Executions[key] = entry
		if err := transaction.Save(); err != nil {
			return invalid("Native admission could not be durably fenced")
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation,
			Reference: json.RawMessage(`{"reference":"` + reference + `"}`)}); err != nil {
			return uncertain("Native admission was fenced but its acceptance receipt was not acknowledged")
		}
		accepted = true
		path, body := "/api/v1/off-peak/ticket", map[string]any{"task_id": entry.Task}
		if lane == "highspeed" {
			path = "/api/v1/highspeed/draw"
			provider := "builtin:zai-coding-plan"
			if account.Site == "bigmodel" {
				provider = "builtin:bigmodel-coding-plan"
			}
			body["provider"], body["model"] = provider, model
		}
		status, response, callErr := owner.control(ctx, stream, account, lane, http.MethodPost, path, body)
		var data map[string]any
		if callErr == nil {
			data, callErr = controlObject(status, response, lane == "highspeed")
		}
		if callErr == nil && lane == "offpeak" {
			entry, callErr = validateTicket(data, entry)
		}
		if callErr == nil && lane == "highspeed" {
			next, valid := finiteNumber(data["next_draw_at"])
			if !valid || next < 0 || next != float64(int64(next)) {
				callErr = protocol.ErrInvalid
			} else {
				if transaction.Journal.DrawAfter == nil {
					transaction.Journal.DrawAfter = map[string]int64{}
				}
				transaction.Journal.DrawAfter[call.Scope.Account] = int64(next)
			}
			card, valid := data["card"].(map[string]any)
			if !valid {
				callErr = protocol.ErrInvalid
			} else {
				cardID, _ := card["card_id"].(string)
				task, _ := card["task_id"].(string)
				rawModel, _ := card["model"].(string)
				provider, _ := card["provider"].(string)
				expires, valid := finiteNumber(card["expires_at"])
				expectedProvider, _ := body["provider"].(string)
				if !safeText(cardID) || task != entry.Task || rawModel != model || provider != expectedProvider ||
					!valid || expires <= 0 || expires != float64(int64(expires)) {
					callErr = protocol.ErrInvalid
				} else {
					entry.Card, entry.ExpiresAt, entry.State = cardID, int64(expires), "active"
				}
			}
		}
		if callErr != nil {
			entry.State = "unknown"
		}
		transaction.Journal.Executions[key] = entry
		if err := transaction.Save(); err != nil {
			return uncertain("Native admission may have succeeded but its result could not be persisted")
		}
		if callErr != nil {
			return uncertain("Native admission has an unconfirmed result; no retry or ordinary fallback was made")
		}
		selected = entry
		return nil
	})
	return selected, accepted, err
}

func (owner *module) waitTicket(ctx context.Context, stream *sdk.Stream, account Credentials, store *controller.Store, principal string, entry controller.Execution) (controller.Execution, error) {
	for entry.State == "queued" {
		status, body, err := owner.control(ctx, stream, account, "offpeak", http.MethodPost,
			"/api/v1/off-peak/ticket/status", map[string]any{"ticket_ids": []string{entry.Ticket}})
		if err != nil {
			return entry, uncertain("The admitted Off-Peak ticket could not be polled; no new ticket was taken")
		}
		data, err := controlObject(status, body, false)
		tickets, valid := data["tickets"].([]any)
		if err != nil || !valid || len(tickets) != 1 {
			return entry, uncertain("The admitted Off-Peak ticket status is incomplete")
		}
		ticket, _ := tickets[0].(map[string]any)
		next, err := validateTicket(ticket, entry)
		if err != nil {
			return entry, uncertain("Off-Peak returned another ticket or malformed state")
		}
		err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
			key := controller.EntryKey(entry.Account, entry.Task)
			current := transaction.Journal.Executions[key]
			// Polling happened outside the journal lock. A later abandon,
			// reconciliation or inference owns its progress; this observation
			// may only advance the same still-queued ticket.
			if current.Reference != entry.Reference || current.Login != entry.Login ||
				current.Generation != entry.Generation || current.Ticket != entry.Ticket ||
				current.State != "queued" || current.InFlight != "" {
				return uncertain("The Off-Peak ticket was changed during polling")
			}
			current.State = next.State
			next = current
			transaction.Journal.Executions[key] = current
			return transaction.Save()
		})
		if err != nil {
			return entry, err
		}
		entry = next
		if entry.State == "queued" {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return entry, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return entry, nil
}

func (owner *module) executeLane(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials, request executeInput, lane string) (any, error) {
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	if state.dataDir == "" {
		return nil, &sdk.Error{Code: "not_ready", Message: "Configure an explicit private dataDir before using native execution models", Outcome: "not-started"}
	}
	if call.Scope.Session == "" || account.Kind != "individual" && account.Kind != "team" {
		return nil, invalid("Native execution requires a selected paid account and an exact conversation session")
	}
	original, err := protocol.ReadObject(request.Body)
	if err != nil || original["model"] != request.Model {
		return nil, invalid("Native Messages body must match the declared selected model alias")
	}
	model := rawLaneModel(request.Model)
	original["model"] = model
	body, err := json.Marshal(original)
	if err != nil {
		return nil, invalid("Invalid native Messages body")
	}
	body, renamed, err := protocol.Prepare(body, model)
	if err != nil {
		return nil, invalid("Invalid native Messages body or tool declarations")
	}
	device := accountDevice(account)
	session := sessionID(call.Scope.Session, device)
	query := queryID(body, session)
	requestID, err := newRequestID()
	if err != nil {
		return nil, err
	}
	body, err = markIdentity(body, device, session)
	if err != nil {
		return nil, err
	}
	principal, login, err := owner.principal(ctx, stream, account, lane)
	if err != nil {
		return nil, err
	}
	store, err := state.controller()
	if err != nil {
		return nil, err
	}
	entry, accepted, err := owner.admit(ctx, stream, call, account, store, principal, login, lane, model)
	if err != nil {
		return nil, err
	}
	if lane == "offpeak" {
		entry, err = owner.waitTicket(ctx, stream, account, store, principal, entry)
		if err != nil {
			return nil, err
		}
	}
	if entry.State != "ready" && entry.State != "active" ||
		lane == "highspeed" && entry.ExpiresAt <= owner.options.Now().UnixMilli() {
		return nil, uncertain("The admitted ticket/card is not ready or has expired; ordinary fallback is not authorized by this model alias")
	}
	key := controller.EntryKey(entry.Account, entry.Task)
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		current := transaction.Journal.Executions[key]
		if current.Reference != entry.Reference || current.Login != login || current.InFlight != "" ||
			current.State != "ready" && current.State != "active" {
			return uncertain("The native execution is changed, busy or unconfirmed")
		}
		if current.Outcomes == nil {
			current.Outcomes = map[string]string{}
		}
		if current.Outcomes[call.Scope.Operation] != "" {
			return uncertain("This native inference operation has already been recorded")
		}
		current.InFlight, current.Outcomes[call.Scope.Operation] = call.Scope.Operation, "pending"
		transaction.Journal.Executions[key] = current
		return transaction.Save()
	})
	if err != nil {
		return nil, err
	}
	if !accepted {
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation,
			Reference: json.RawMessage(`{"reference":"` + entry.Reference + `"}`)}); err != nil {
			return nil, uncertain("Native inference was fenced but its acceptance receipt was not acknowledged")
		}
	}
	path := "/api/v1/off-peak/anthropic/v1/messages"
	if lane == "highspeed" {
		path = "/api/v1/highspeed/anthropic/v1/messages"
	}
	endpoint := owner.zcodeOrigin() + path
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return nil, uncertain("The fenced native inference route could not be resolved")
	}
	outbound, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, uncertain("The fenced native inference could not be constructed")
	}
	outbound.Header = owner.controlHeaders(account, lane)
	outbound.Header.Set("Anthropic-Version", "2023-06-01")
	outbound.Header.Set("X-Api-Key", account.SessionToken)
	outbound.Header.Set("X-Session-Id", session)
	outbound.Header.Set("X-Request-Id", requestID)
	outbound.Header.Set("X-ZCode-Trace-Id", seededUUID("zcode-trace\x00"+session))
	if query != "" {
		outbound.Header.Set("X-Query-Id", query)
	}
	if lane == "offpeak" {
		outbound.Header.Set("X-Off-Peak-Ticket-ID", entry.Ticket)
	} else {
		outbound.Header.Set("X-Highspeed-Card-ID", entry.Card)
	}
	response, callErr := send(ctx, owner.options.HTTPFactory, proxy.URL, outbound)
	var result any
	if callErr == nil {
		defer response.Body.Close()
		result, callErr = streamResponse(ctx, response, stream, renamed, owner.observability)
		if value, valid := result.(sdk.ExecuteResult); !valid || value.Status < 200 || value.Status >= 300 {
			callErr = uncertain("The native inference did not complete successfully; it will not be replayed")
		}
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	persistErr := store.Locked(persistCtx, principal, func(transaction *controller.Transaction) error {
		current := transaction.Journal.Executions[key]
		if current.Reference != entry.Reference || current.InFlight != call.Scope.Operation {
			return controller.ErrJournal
		}
		current.Outcomes[call.Scope.Operation] = "uncertain"
		if callErr == nil {
			current.Outcomes[call.Scope.Operation], current.InFlight = "completed", ""
		}
		transaction.Journal.Executions[key] = current
		return transaction.Save()
	})
	if persistErr != nil || callErr != nil {
		return nil, uncertain("Native inference has an unconfirmed outcome; no replay or ordinary fallback was made")
	}
	return result, nil
}

func settlementPath(ticket string) string {
	return "/api/v1/off-peak/ticket/" + url.PathEscape(ticket) + "/settle"
}
