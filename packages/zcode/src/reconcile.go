package zcode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type reconcileRequest struct {
	OperationID                string          `json:"operationId,omitempty"`
	Reference                  nativeReference `json:"reference"`
	TicketID                   string          `json:"ticketId,omitempty"`
	AcknowledgeUncertainOutput bool            `json:"acknowledgeUncertainOutput,omitempty"`
}

func (owner *module) reconcile(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request reconcileRequest
	if decode(call.Params, &request) != nil || request.Reference.Reference == "" || call.Scope.Operation == "" {
		return nil, invalid("Reconciliation requires an exact execution reference and a host-owned write operation")
	}
	account, store, principal, login, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var result executionView
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		key, entry, err := findExecution(transaction, call.Scope.Account, request.OperationID, request.Reference.Reference)
		if err != nil {
			return err
		}
		if entry.State == "settled" || entry.State == "abandoned" {
			result = viewExecution(entry, login)
			return nil
		}
		if entry.Lane == "offpeak" {
			ticket := entry.Ticket
			if ticket == "" {
				ticket = request.TicketID
			} else if request.TicketID != "" && request.TicketID != ticket {
				return invalid("A known ticket cannot be replaced by another ticket")
			}
			if !safeText(ticket) {
				return unsupported("A lost Off-Peak admission needs an explicit ticket ID whose task can be verified by the vendor")
			}
			if account.ExpiresAt <= owner.options.Now().UnixMilli() {
				return &sdk.Error{Code: "auth_refresh_required", Message: "Off-Peak status requires a current project token", Outcome: "not-started"}
			}
			status, body, callErr := owner.control(ctx, stream, account, "offpeak", http.MethodPost,
				"/api/v1/off-peak/ticket/status", map[string]any{"ticket_ids": []string{ticket}})
			if callErr != nil {
				return uncertain("The stored admission could not be verified; no ticket was retaken")
			}
			data, callErr := controlObject(status, body, false)
			tickets, valid := data["tickets"].([]any)
			if callErr != nil || !valid || len(tickets) != 1 {
				return uncertain("The vendor did not identify the requested ticket")
			}
			value, valid := tickets[0].(map[string]any)
			if !valid || value["task_id"] != entry.Task {
				return uncertain("The vendor did not authenticate the ticket's original task")
			}
			next, callErr := validateTicket(value, entry)
			if callErr != nil || next.Ticket != ticket {
				return uncertain("The vendor ticket binding does not match the original request")
			}
			entry = next
		} else {
			if request.TicketID != "" || entry.Card == "" {
				return unsupported("A lost Highspeed draw has no authenticated card/task binding in the recovered status protocol; retain uncertainty or explicitly abandon")
			}
			status, body, callErr := owner.control(ctx, stream, account, "", http.MethodGet,
				"/api/v1/highspeed/"+url.PathEscape(entry.Card)+"/healthy", nil)
			if callErr != nil {
				return uncertain("The stored Highspeed card could not be verified")
			}
			data, callErr := controlObject(status, body, true)
			if callErr != nil || data["card_id"] != entry.Card {
				return uncertain("The vendor did not authenticate the stored Highspeed card")
			}
			entry.State = "active"
			if entry.ExpiresAt <= owner.options.Now().UnixMilli() {
				entry.State = "expired"
			}
		}
		entry.Login, entry.Generation = login, call.Scope.Generation
		if request.AcknowledgeUncertainOutput && entry.InFlight != "" {
			entry.Outcomes[entry.InFlight] = "uncertain"
			entry.InFlight = ""
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation,
			Reference: json.RawMessage(`{"reference":"` + entry.Reference + `"}`)}); err != nil {
			return err
		}
		transaction.Journal.Executions[key] = entry
		if err := transaction.Save(); err != nil {
			return uncertain("Vendor state was read but reconciliation could not be durably saved")
		}
		result = viewExecution(entry, login)
		return nil
	})
	return result, err
}

func (owner *module) abandon(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Reference      nativeReference `json:"reference"`
		Acknowledgment string          `json:"acknowledgment"`
	}
	if decode(call.Params, &request) != nil || request.Reference.Reference == "" ||
		request.Acknowledgment != "vendor-operation-may-have-succeeded" || call.Scope.Operation == "" {
		return nil, invalid("Abandonment requires an execution reference and explicit vendor-operation-may-have-succeeded acknowledgment")
	}
	_, store, principal, login, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var result executionView
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		key, entry, err := findExecution(transaction, call.Scope.Account, "", request.Reference.Reference)
		if err != nil {
			return err
		}
		if entry.InFlight != "" {
			entry.Outcomes[entry.InFlight] = "uncertain"
			entry.InFlight = ""
		}
		entry.State = "abandoned"
		transaction.Journal.Executions[key] = entry
		if err := transaction.Save(); err != nil {
			return uncertain("Abandonment could not be persisted")
		}
		result = viewExecution(entry, login)
		return nil
	})
	return result, err
}
