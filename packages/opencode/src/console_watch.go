package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type ConsoleWatchEvent struct {
	ID        string `json:"id"`
	WatchID   string `json:"watchID"`
	SessionID string `json:"sessionID"`
	Source    string `json:"source"`
	Kind      string `json:"kind"`
	Actor     struct {
		Login string `json:"login"`
		Bot   bool   `json:"bot"`
		Self  *bool  `json:"self,omitempty"`
	} `json:"actor"`
	Title  string  `json:"title"`
	Body   string  `json:"body"`
	URL    string  `json:"url"`
	Status *string `json:"status"`
}

type ConsoleWatchPage struct {
	Cursor  string              `json:"cursor"`
	Events  []ConsoleWatchEvent `json:"events"`
	Receipt string              `json:"receipt,omitempty"`
}

type ConsoleWatchType struct {
	Server      string          `json:"server"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func (owner *module) consoleWatchTypes(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if decode(call.Params, &struct{}{}) != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console event types accept no substituted authority"}
	}
	identity, err := owner.watchesAllowed(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Types *[]struct {
			Server      *string         `json:"server"`
			Name        *string         `json:"name"`
			Description json.RawMessage `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"types"`
	}
	if err := owner.watchRead(ctx, stream, identity, "/api/event-watches/types", &wire); err != nil {
		return nil, failure(err, "")
	}
	if wire.Types == nil || len(*wire.Types) > 1000 {
		return nil, failure(errors.New("invalid Console event types"), "")
	}
	result := struct {
		Types []ConsoleWatchType `json:"types"`
	}{Types: []ConsoleWatchType{}}
	for _, item := range *wire.Types {
		var schema map[string]json.RawMessage
		if item.Server == nil || item.Name == nil || len(*item.Server) > 256 || len(*item.Name) > 256 ||
			json.Unmarshal(item.InputSchema, &schema) != nil || schema == nil {
			return nil, failure(errors.New("invalid Console event type"), "")
		}
		description := ""
		if len(item.Description) != 0 {
			if bytes.Equal(bytes.TrimSpace(item.Description), []byte("null")) || json.Unmarshal(item.Description, &description) != nil {
				return nil, failure(errors.New("invalid Console event type description"), "")
			}
			description = strings.Join(strings.Fields(description), " ")
			length := 0
			for index, character := range description {
				length++
				if character > 0xffff {
					length++
				}
				if length > 300 {
					description = description[:index]
					break
				}
			}
		}
		result.Types = append(result.Types, ConsoleWatchType{Server: *item.Server, Name: *item.Name,
			Description: description, InputSchema: item.InputSchema})
	}
	return result, nil
}

func (owner *module) watchesAllowed(ctx context.Context, call sdk.Call, stream *sdk.Stream) (ZenCredentials, error) {
	identity, err := owner.consoleIdentity(call)
	if err != nil {
		return identity, err
	}
	snapshotCall := call
	snapshotCall.Params = json.RawMessage("{}")
	value, err := owner.consoleSnapshot(ctx, snapshotCall, stream)
	if err != nil {
		return identity, err
	}
	if !value.(ConsoleSnapshot).Watches || value.(ConsoleSnapshot).Status != "" {
		return identity, &sdk.Error{Code: "permission_denied", Message: "Console watches require the explicit module flag and workspace response header", Outcome: "not-started"}
	}
	return identity, nil
}

func (owner *module) consoleWatchList(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if string(call.Params) != "{}" && string(call.Params) != "null" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console watch list accepts no substituted authority"}
	}
	identity, err := owner.watchesAllowed(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var result struct {
		Watches []json.RawMessage `json:"watches"`
	}
	if err := owner.watchRead(ctx, stream, identity, "/api/event-watches", &result); err != nil {
		return nil, failure(err, "")
	}
	if result.Watches == nil || len(result.Watches) > 1000 {
		return nil, failure(errors.New("invalid Console watch list"), "")
	}
	for _, value := range result.Watches {
		var watch struct {
			ID         *string `json:"id"`
			SessionID  *string `json:"sessionID"`
			RoutingKey *string `json:"routingKey"`
			Mode       *string `json:"mode"`
			ExpiresAt  *string `json:"expiresAt"`
		}
		if json.Unmarshal(value, &watch) != nil || watch.ID == nil || watch.SessionID == nil || watch.RoutingKey == nil || watch.Mode == nil || watch.ExpiresAt == nil {
			return nil, failure(errors.New("invalid Console watch"), "")
		}
	}
	return result, nil
}

func (owner *module) watchRead(ctx context.Context, stream *sdk.Stream, identity ZenCredentials, endpoint string, out any) error {
	response, cancel, err := owner.consoleRequest(ctx, stream, identity, http.MethodGet, endpoint, nil, 35*time.Second)
	if err != nil {
		return err
	}
	defer cancel()
	defer response.Body.Close()
	body, err := readBounded(response.Body, maxReplyBytes)
	if err != nil || response.StatusCode != 200 {
		return errors.New("Console watch read failed")
	}
	return json.Unmarshal(body, out)
}

func (owner *module) consoleWatchPoll(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Cursor string `json:"cursor,omitempty"`
	}
	if decode(call.Params, &request) != nil || len(request.Cursor) > 2048 {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console watch cursor must be bounded"}
	}
	identity, err := owner.watchesAllowed(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	query := url.Values{"wait": {"25"}}
	if request.Cursor != "" {
		query.Set("cursor", request.Cursor)
	}
	var wire struct {
		Cursor *string           `json:"cursor"`
		Events []json.RawMessage `json:"events"`
	}
	if err := owner.watchRead(ctx, stream, identity, "/api/event-watches/events?"+query.Encode(), &wire); err != nil {
		return nil, failure(err, "")
	}
	if wire.Cursor == nil || len(*wire.Cursor) > 2048 || wire.Events == nil || len(wire.Events) > 200 {
		return nil, failure(errors.New("invalid Console watch page"), "")
	}
	result := ConsoleWatchPage{Cursor: *wire.Cursor, Events: []ConsoleWatchEvent{}}
	ids := map[string]bool{}
	for _, value := range wire.Events {
		var fields map[string]json.RawMessage
		var actor map[string]json.RawMessage
		var event ConsoleWatchEvent
		valid := json.Unmarshal(value, &fields) == nil && json.Unmarshal(value, &event) == nil &&
			json.Unmarshal(fields["actor"], &actor) == nil
		for _, key := range []string{"id", "watchID", "sessionID", "source", "kind", "title", "body", "url"} {
			var text string
			valid = valid && string(fields[key]) != "null" && json.Unmarshal(fields[key], &text) == nil
		}
		var login string
		var bot bool
		valid = valid && fields["status"] != nil && json.Unmarshal(actor["login"], &login) == nil &&
			string(actor["login"]) != "null" && json.Unmarshal(actor["bot"], &bot) == nil && string(actor["bot"]) != "null"
		if !valid || event.ID == "" || len(event.ID) > 128 || ids[event.ID] {
			return nil, failure(errors.New("invalid Console watch event"), "")
		}
		ids[event.ID] = true
		result.Events = append(result.Events, event)
	}
	if len(ids) != 0 {
		state, _ := currentInstance(ctx)
		cache := state.console
		if err := cache.lock(ctx); err != nil {
			return nil, err
		}
		defer func() { <-cache.gate }()
		for id, receipt := range cache.receipts {
			if !owner.options.Now().Before(receipt.expires) {
				delete(cache.receipts, id)
			}
		}
		if len(cache.receipts) >= 128 {
			return nil, &sdk.Error{Code: "resource_exhausted", Message: "Console watch acknowledgements are awaiting delivery"}
		}
		result.Receipt, err = randomHex(16)
		if err != nil {
			return nil, err
		}
		cache.receipts[result.Receipt] = watchReceipt{owner: consoleKey(call, identity), ids: ids, expires: owner.options.Now().Add(10 * time.Minute)}
	}
	return result, nil
}

func (owner *module) consoleWatchAck(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Receipt string   `json:"receipt"`
		IDs     []string `json:"ids"`
	}
	if decode(call.Params, &request) != nil || len(request.Receipt) != 32 || len(request.IDs) == 0 || len(request.IDs) > 1000 || call.Scope.Operation == "" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console acknowledgement requires a delivered-page receipt", Outcome: "not-started"}
	}
	identity, err := owner.watchesAllowed(ctx, call, stream)
	if err != nil {
		return nil, notStarted(err)
	}
	state, _ := currentInstance(ctx)
	cache := state.console
	if err := cache.lock(ctx); err != nil {
		return nil, err
	}
	receipt, found := cache.receipts[request.Receipt]
	valid := found && receipt.owner == consoleKey(call, identity) && owner.options.Now().Before(receipt.expires) && len(request.IDs) == len(receipt.ids)
	seen := map[string]bool{}
	for _, id := range request.IDs {
		valid = valid && len(id) > 0 && len(id) <= 128 && receipt.ids[id] && !seen[id]
		seen[id] = true
	}
	if valid {
		delete(cache.receipts, request.Receipt)
	}
	<-cache.gate
	if !valid {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Console acknowledgement does not belong to this delivered page and account generation", Outcome: "not-started"}
	}
	if call.Scope.Operation == "" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console acknowledgement requires a host operation", Outcome: "not-started"}
	}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	response, cancel, err := owner.consoleRequest(ctx, stream, identity, http.MethodPost, "/api/event-watches/ack", map[string]any{"ids": request.IDs}, 5*time.Second)
	if err != nil {
		return nil, failure(err, "uncertain")
	}
	defer cancel()
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, failure(errors.New("Console acknowledgement failed"), "uncertain")
	}
	return struct {
		Acknowledged int `json:"acknowledged"`
	}{len(request.IDs)}, nil
}
