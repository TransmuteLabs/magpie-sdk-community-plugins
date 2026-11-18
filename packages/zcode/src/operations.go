package zcode

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type nativeReference struct {
	Reference string `json:"reference"`
}

type executionView struct {
	Reference          string `json:"reference"`
	OperationID        string `json:"operationId"`
	Task               string `json:"task"`
	Lane               string `json:"lane"`
	Model              string `json:"model"`
	State              string `json:"state"`
	Ticket             string `json:"ticket,omitempty"`
	Card               string `json:"card,omitempty"`
	ExpiresAt          int64  `json:"expiresAt,omitempty"`
	RequiresResolution bool   `json:"requiresResolution,omitempty"`
}

func viewExecution(entry controller.Execution, login string) executionView {
	return executionView{Reference: entry.Reference, OperationID: entry.Operation, Task: entry.Task, Lane: entry.Lane, Model: entry.Model,
		State: entry.State, Ticket: entry.Ticket, Card: entry.Card, ExpiresAt: entry.ExpiresAt,
		RequiresResolution: entry.Login != login || entry.State == "unknown" || entry.State == "pending" ||
			entry.State == "settling" || entry.InFlight != ""}
}

func (owner *module) executionContext(ctx context.Context, call sdk.Call, stream *sdk.Stream) (Credentials, *controller.Store, string, string, error) {
	account, err := owner.identity(call)
	if err != nil {
		return account, nil, "", "", err
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return account, nil, "", "", err
	}
	if state.dataDir == "" {
		return account, nil, "", "", &sdk.Error{Code: "not_ready", Message: "Native controller dataDir is not configured", Outcome: "not-started"}
	}
	principal, login, err := owner.principal(ctx, stream, account, "")
	if err != nil {
		return account, nil, "", "", err
	}
	store, err := state.controller()
	return account, store, principal, login, err
}

func findExecution(transaction *controller.Transaction, account, operation, reference string) (string, controller.Execution, error) {
	for key, entry := range transaction.Journal.Executions {
		if entry.Account == account && (reference != "" && entry.Reference == reference ||
			reference == "" && operation != "" && (entry.Operation == operation || entry.Outcomes[operation] != "")) {
			if operation != "" && entry.Operation != operation && entry.Outcomes[operation] == "" {
				return "", controller.Execution{}, invalid("Operation does not belong to this execution reference")
			}
			return key, entry, nil
		}
	}
	return "", controller.Execution{}, invalid("Native execution reference was not found for this account")
}

func (owner *module) operationStatus(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.OperationRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	var reference nativeReference
	if len(request.Reference) != 0 && decode(request.Reference, &reference) != nil {
		return nil, invalid("Invalid native execution reference")
	}
	_, store, principal, login, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var result sdk.OperationResult
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		_, entry, err := findExecution(transaction, call.Scope.Account, request.OperationID, reference.Reference)
		if err != nil {
			return err
		}
		outcome := entry.Outcomes[request.OperationID]
		if outcome == "" {
			outcome = "uncertain"
			if entry.State == "queued" || entry.State == "ready" || entry.State == "active" {
				outcome = "accepted"
			}
		} else if outcome == "pending" {
			outcome = "uncertain"
		}
		if entry.Login != login {
			outcome = "uncertain"
		}
		view, _ := json.Marshal(viewExecution(entry, login))
		result = sdk.OperationResult{OperationID: request.OperationID, Outcome: outcome, Result: view}
		return nil
	})
	return result, err
}

func (owner *module) executionList(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct{}
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	_, store, principal, login, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	result := []executionView{}
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		for _, entry := range transaction.Journal.Executions {
			if entry.Account == call.Scope.Account {
				result = append(result, viewExecution(entry, login))
			}
		}
		return nil
	})
	return result, err
}

func (owner *module) settle(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	account, err := owner.selected(call)
	if err != nil {
		return nil, err
	}
	var request sdk.OperationRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	var reference nativeReference
	if len(request.Reference) != 0 && decode(request.Reference, &reference) != nil {
		return nil, invalid("Invalid native execution reference")
	}
	_, store, principal, login, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	if call.Scope.Operation == "" {
		return nil, invalid("Settlement requires a host-owned write operation")
	}
	var result executionView
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		key, entry, err := findExecution(transaction, call.Scope.Account, request.OperationID, reference.Reference)
		if err != nil {
			return err
		}
		if call.Method == sdk.MethodOperationCancel && call.Scope.Operation == entry.Operation {
			return invalid("Cancellation cannot reuse the original admission operation")
		}
		if entry.Login != login || entry.Lane != "offpeak" || entry.Ticket == "" {
			return unsupported("Settlement requires an Off-Peak ticket owned by the current selected login")
		}
		if entry.State == "settled" {
			result = viewExecution(entry, login)
			return nil
		}
		if entry.State == "settling" || entry.State == "unknown" || entry.State == "pending" || entry.InFlight != "" {
			return uncertain("Settlement or inference remains unconfirmed; no settlement was repeated")
		}
		entry.State = "settling"
		transaction.Journal.Executions[key] = entry
		if err := transaction.Save(); err != nil {
			return invalid("Settlement could not be durably fenced")
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation,
			Reference: json.RawMessage(`{"reference":"` + entry.Reference + `"}`)}); err != nil {
			return uncertain("Settlement was fenced but acceptance was not acknowledged")
		}
		status, body, callErr := owner.control(ctx, stream, account, "offpeak", http.MethodPost, settlementPath(entry.Ticket), nil)
		if callErr == nil && (status < 200 || status >= 300) {
			callErr = uncertain("ZCode did not confirm settlement")
		}
		if callErr == nil && len(body) != 0 {
			value, parseErr := controlObject(status, body, false)
			if parseErr != nil || value["ticket_id"] != nil && value["ticket_id"] != entry.Ticket {
				callErr = uncertain("ZCode settlement response did not match its ticket")
			}
		}
		if callErr != nil {
			return uncertain("Settlement remains unconfirmed; no retry was made")
		}
		entry.State = "settled"
		transaction.Journal.Executions[key] = entry
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if persistCtx.Err() != nil || transaction.Save() != nil {
			return uncertain("Settlement succeeded but durable publication is unconfirmed")
		}
		result = viewExecution(entry, login)
		return nil
	})
	return result, err
}

func (owner *module) operationCancel(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.OperationRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if request.OperationID == "" && len(request.Reference) == 0 || call.Scope.Operation == "" ||
		request.OperationID != "" && request.OperationID == call.Scope.Operation {
		return nil, invalid("Business cancellation requires a new write intent and a separate original target operation")
	}
	view, err := owner.settle(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	target := request.OperationID
	if target == "" {
		target = view.(executionView).OperationID
	}
	return sdk.OperationResult{OperationID: target, Outcome: "completed", Result: result}, nil
}
