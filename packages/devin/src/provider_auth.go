package devin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type signInFlow struct {
	ID          string               `json:"id"`
	State       string               `json:"state"`
	Kind        string               `json:"kind"`
	Server      string               `json:"server"`
	Deadline    time.Time            `json:"deadline"`
	Credentials outbound.Credentials `json:"credentials,omitempty"`
	CLIImport   *cliImportIdentity   `json:"cliImport,omitempty"`
	PKCE        outbound.PKCE        `json:"pkce,omitempty"`
	CallbackID  string               `json:"callback,omitempty"`
	Instance    string               `json:"instance,omitempty"`
}

type signInHandle struct {
	ID string `json:"id"`
}

func randomIdentity() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func readState(ctx context.Context, stream *sdk.Stream, key string, target any) (string, error) {
	var state sdk.StateValue
	if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: key}, &state); err != nil {
		return "", err
	}
	if !state.Found {
		return state.Version, &sdk.Error{Code: "invalid_params", Message: "Devin module state is missing"}
	}
	if json.Unmarshal(state.Value, target) != nil {
		return "", &sdk.Error{Code: "invalid_params", Message: "Devin module state is unreadable"}
	}
	return state.Version, nil
}

func swapState(ctx context.Context, stream *sdk.Stream, key, version string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var result sdk.StateCASResult
	if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{Key: key, Version: version, Value: data}, &result); err != nil {
		return "", err
	}
	if !result.Swapped {
		return "", &sdk.Error{Code: "conflict", Message: "Devin module state changed; this action was not repeated"}
	}
	return result.Version, nil
}

func (owner *providerOwner) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := scopeProvider(call, false); err != nil {
		return nil, err
	}
	state, err := currentProviderState(ctx)
	if err != nil {
		return nil, err
	}
	var request sdk.AuthStartRequest
	if err := decodeParams(call.Params, &request); err != nil {
		return nil, err
	}
	flow := signInFlow{State: "pending", Kind: request.Method, Server: state.server, Deadline: owner.options.Now().Add(10 * time.Minute)}
	result := sdk.AuthStartResult{}
	saved := false
	defer func() {
		if !saved && flow.CallbackID != "" {
			state.callbacks.remove(flow.CallbackID)
		}
	}()
	switch request.Method {
	case "api-key", sdk.SavedSignInMethod:
		if request.RedirectURI != "" {
			return nil, &sdk.Error{Code: "invalid_params", Message: "API-key sign-in has no redirect"}
		}
		if request.Method == sdk.SavedSignInMethod {
			flow.Credentials, flow.CLIImport, err = importedSignIn(request.Input, state.server)
		} else {
			err = decodeParams(request.Input, &flow.Credentials)
		}
		if err != nil {
			return nil, err
		}
		if flow.Credentials.Validate() != nil || flow.Credentials.Due(owner.options.Now()) {
			return nil, &sdk.Error{Code: "invalid_params", Message: "An explicit unexpired Devin key is required"}
		}
	case "browser":
		if len(request.Input) != 0 && string(request.Input) != "null" && string(request.Input) != "{}" {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Browser sign-in takes a callback origin, not credentials"}
		}
		if request.RedirectURI == "" {
			var callback *browserCallback
			callback, result.URL, err = state.callbacks.prepare(owner.options.Now)
			if err == nil {
				callback.mu.Lock()
				flow.PKCE = callback.pkce
				ended := callback.closed
				callback.mu.Unlock()
				flow.CallbackID, flow.Instance = callback.id, state.instance
				result.PollAfterMs = 1000
				if ended {
					return nil, &sdk.Error{Code: "expired", Message: "Devin browser callback instance ended"}
				}
			}
		} else {
			flow.PKCE, result.URL, err = outbound.PreparePKCE(request.RedirectURI, owner.options.Now())
		}
		if err != nil {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Browser sign-in requires a 127.0.0.1 callback URL"}
		}
	default:
		return nil, &sdk.Error{Code: "unsupported", Message: "Use Devin browser, API key, or saved sign-in"}
	}
	handle, err := randomIdentity()
	if err != nil {
		return nil, moduleFailure(err, "")
	}
	offset := strings.IndexByte("0123456789abcdef", handle[0])
	for index := 0; index < 16; index++ {
		slot := "0123456789abcdef"[(offset+index)%16]
		flow.ID = string(slot) + handle[1:]
		key := "signin." + string(slot)
		var value sdk.StateValue
		if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: key}, &value); err != nil {
			return nil, err
		}
		if value.Found {
			var old signInFlow
			if json.Unmarshal(value.Value, &old) != nil {
				return nil, &sdk.Error{Code: "invalid_params", Message: "Devin sign-in state is unreadable"}
			}
			if old.State != "complete" && owner.options.Now().Before(old.Deadline) {
				continue
			}
		}
		data, _ := json.Marshal(flow)
		var result sdk.StateCASResult
		if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{Key: key, Version: value.Version, Value: data}, &result); err != nil {
			return nil, err
		}
		if result.Swapped {
			handle = flow.ID
			saved = true
			break
		}
	}
	if !saved {
		return nil, &sdk.Error{Code: "resource_exhausted", Message: "Devin has sixteen active sign-in attempts; finish one or wait for expiry"}
	}
	result.Continuation, _ = json.Marshal(signInHandle{ID: handle})
	return result, nil
}

func importedSignIn(raw json.RawMessage, server string) (outbound.Credentials, *cliImportIdentity, error) {
	var input struct {
		Record string `json:"record"`
	}
	var record struct {
		Type      string                     `json:"type"`
		Key       string                     `json:"key"`
		AccountID string                     `json:"accountId"`
		Email     string                     `json:"email"`
		Metadata  map[string]json.RawMessage `json:"metadata"`
	}
	if decodeParams(raw, &input) != nil || len(input.Record) > 16<<10 ||
		decodeParams(json.RawMessage(input.Record), &record) != nil || record.Type != "api" {
		return outbound.Credentials{}, nil, &sdk.Error{Code: "invalid_params", Message: "Provide one saved Devin API sign-in record"}
	}
	origin := outbound.DefaultServer
	if value, exists := record.Metadata["server"]; exists {
		origin = ""
		if json.Unmarshal(value, &origin) != nil || origin == "" {
			return outbound.Credentials{}, nil, &sdk.Error{Code: "invalid_params", Message: "Saved Devin sign-in has an invalid server"}
		}
	}
	if strings.TrimRight(origin, "/") != server {
		return outbound.Credentials{}, nil, &sdk.Error{Code: "invalid_params", Message: "Saved Devin sign-in belongs to a different server"}
	}
	identity, err := cliImportFrom(record.Metadata)
	return outbound.Credentials{Key: record.Key}, identity, err
}

func (owner *providerOwner) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := scopeProvider(call, false); err != nil {
		return nil, err
	}
	state, err := currentProviderState(ctx)
	if err != nil {
		return nil, err
	}
	var previous accountCredentials
	if call.Scope.Account != "" {
		previous, _, err = selectedAccount(ctx, call)
		if err != nil {
			return nil, err
		}
	}
	var request sdk.AuthFinishRequest
	if err := decodeParams(call.Params, &request); err != nil {
		return nil, err
	}
	var handle signInHandle
	if request.Code != "" || decodeParams(request.Continuation, &handle) != nil || len(handle.ID) != 32 || strings.Trim(handle.ID, "0123456789abcdef") != "" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "A Devin sign-in continuation is required"}
	}
	var flow signInFlow
	key := "signin." + handle.ID[:1]
	version, err := readState(ctx, stream, key, &flow)
	if err != nil {
		return nil, err
	}
	if flow.ID != handle.ID || flow.State != "pending" || flow.Server != state.server || !owner.options.Now().Before(flow.Deadline) {
		if flow.ID == handle.ID && flow.CallbackID != "" {
			state.callbacks.remove(flow.CallbackID)
		}
		return nil, &sdk.Error{Code: "expired", Message: "Devin sign-in ended or became uncertain; start it again"}
	}
	callback := ""
	if flow.Kind == "browser" {
		if flow.CallbackID != "" {
			if flow.Instance != state.instance {
				return nil, &sdk.Error{Code: "expired", Message: "Devin callback belongs to an ended component instance"}
			}
			if len(request.Input) != 0 && string(request.Input) != "null" && string(request.Input) != "{}" {
				return nil, &sdk.Error{Code: "invalid_params", Message: "Module-owned browser completion takes no callback input"}
			}
			callback, err = state.callbacks.pending(flow.CallbackID, owner.options.Now())
			if err != nil {
				state.callbacks.remove(flow.CallbackID)
				return nil, &sdk.Error{Code: "expired", Message: "Devin callback receiver ended; start sign-in again"}
			}
			if callback == "" {
				return sdk.AuthResult{Pending: true}, nil
			}
		} else {
			var input struct {
				CallbackURL string `json:"callbackURL"`
			}
			if decodeParams(request.Input, &input) != nil {
				return nil, &sdk.Error{Code: "invalid_params", Message: "A complete Devin callback URL is required"}
			}
			callback = input.CallbackURL
		}
		if _, err := flow.PKCE.Callback(callback, owner.options.Now()); err != nil {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Devin callback state or redirect did not match"}
		}
	} else if len(request.Input) != 0 && string(request.Input) != "null" && string(request.Input) != "{}" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "API-key completion takes no replacement credential"}
	}
	credentials := flow.Credentials
	pkce := flow.PKCE
	cliImport := flow.CLIImport
	flow.State = "exchanging"
	flow.Credentials = outbound.Credentials{}
	flow.CLIImport = nil
	flow.PKCE = outbound.PKCE{}
	version, err = swapState(ctx, stream, key, version, flow)
	if err != nil {
		return nil, err
	}
	if flow.CallbackID != "" {
		state.callbacks.remove(flow.CallbackID)
	}
	client, err := owner.client(state, stream)
	if err != nil {
		return nil, moduleFailure(err, "uncertain")
	}
	if flow.Kind == "browser" {
		exchanged, err := client.Exchange(ctx, pkce, callback)
		if err != nil {
			return nil, moduleFailure(err, "uncertain")
		}
		credentials = exchanged.Credentials
	}
	var status outbound.Status
	if cliImport != nil {
		credentials, status, err = cliImport.verify(ctx, client, state.server, credentials)
		if err != nil {
			return nil, err
		}
	} else {
		status, err = client.Status(ctx, credentials, nil)
		if err != nil {
			return nil, moduleFailure(err, "uncertain")
		}
	}
	subject := accountSubject(state.server, status)
	if flow.Kind == sdk.SavedSignInMethod && subject == "" {
		return nil, &sdk.Error{
			Code: "reauthorization_required", Message: "Devin did not return a verified identity for the saved sign-in",
			Outcome: "not-started",
		}
	}
	account := subject
	if call.Scope.Account != "" {
		if !sameAccount(ctx, client, state.server, previous, credentials, subject, owner.options.Now()) {
			return nil, &sdk.Error{Code: "permission_denied", Message: "Could not verify the same Devin user and organization for the selected account"}
		}
		account = call.Scope.Account
	}
	if account == "" {
		account = credentials.AccountID(state.server)
	}
	flow.State = "complete"
	flow.Credentials = outbound.Credentials{}
	flow.PKCE = outbound.PKCE{}
	if _, err := swapState(ctx, stream, key, version, flow); err != nil {
		return nil, err
	}
	return accountResult(account, state.server, subject, credentials, status, cliImport != nil), nil
}

func (owner *providerOwner) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	account, state, err := selectedAccount(ctx, call)
	if err != nil {
		return nil, err
	}
	followsCLI := account.Binding != nil && account.Binding.FollowCLI
	credentials := account.Credentials
	if followsCLI {
		credentials, err = readCLICredentials(state.server)
		if err != nil {
			return nil, err
		}
	} else if account.Due(owner.options.Now()) {
		return nil, &sdk.Error{Code: "unsupported", Message: "This Devin credential is expiring; sign in again", Outcome: "not-started"}
	}
	client, err := owner.client(state, stream)
	if err != nil {
		return nil, moduleFailure(err, "not-started")
	}
	status, err := client.Status(ctx, credentials, nil)
	if err != nil {
		return nil, moduleFailure(err, "not-started")
	}
	subject := accountSubject(state.server, status)
	if followsCLI && cliSubject(state.server, status) != account.Binding.Subject {
		return nil, cliIdentityNotVerified()
	}
	if account.Binding != nil && account.Binding.Subject != subject {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Devin status changed the selected account identity", Outcome: "not-started"}
	}
	return accountResult(call.Scope.Account, state.server, subject, credentials, status, followsCLI), nil
}

func (owner *providerOwner) authLogout(_ context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := scopeProvider(call, true); err != nil {
		return nil, err
	}
	var account accountCredentials
	if decodeParams(call.Auth, &account) != nil || account.Validate() != nil || !account.matchesStoredSlot(call.Scope.Account) {
		return nil, &sdk.Error{Code: "permission_denied", Message: "The selected Devin account generation does not match its credentials", Outcome: "not-started"}
	}
	return nil, nil
}
