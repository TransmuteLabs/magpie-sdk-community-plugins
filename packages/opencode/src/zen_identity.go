package opencode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const ProviderZenPublic = "opencode-zen-free"

type ZenCredentials struct {
	Kind  string       `json:"kind"`
	OAuth *Credentials `json:"oauth,omitempty"`
	Key   string       `json:"key,omitempty"`
}

func (identity ZenCredentials) Validate(provider string) error {
	switch {
	case provider == ProviderZen && identity.Kind == "oauth" && identity.OAuth != nil && identity.Key == "":
		return identity.OAuth.Validate()
	case provider == ProviderZen && identity.Kind == "key" && identity.OAuth == nil && validServiceKey(identity.Key):
		return nil
	case provider == ProviderZenPublic && identity.Kind == "public" && identity.Key == "public" && identity.OAuth == nil:
		return nil
	}
	return errors.New("OpenCode credential type does not match the selected provider")
}

func validServiceKey(key string) bool {
	return key != "" && key != "public" && len(key) <= 8192 && !strings.ContainsAny(key, " \t\r\n") &&
		!strings.ContainsFunc(key, func(value rune) bool { return value < 0x21 || value > 0x7e })
}

func (identity ZenCredentials) AccountID(provider string) string {
	if identity.Kind == "oauth" && identity.OAuth != nil {
		return identity.OAuth.AccountID(provider)
	}
	sum := sha256.Sum256([]byte("OpenCode Console\x00" + provider + "\x00" + identity.Kind + "\x00" + identity.Key))
	return hex.EncodeToString(sum[:16])
}

func (identity ZenCredentials) access() string {
	if identity.OAuth != nil {
		return identity.OAuth.Access
	}
	return identity.Key
}

func (identity ZenCredentials) organization() string {
	if identity.OAuth != nil {
		return identity.OAuth.OrgID
	}
	return ""
}

func selectedZen(call sdk.Call) (ZenCredentials, error) {
	var identity ZenCredentials
	if call.Scope.Account == "" || call.Scope.Generation == 0 ||
		json.Unmarshal(call.Auth, &identity) != nil || identity.Validate(call.Scope.Provider) != nil ||
		identity.AccountID(call.Scope.Provider) != call.Scope.Account {
		return identity, &sdk.Error{Code: "permission_denied", Message: "An explicit provider-bound Zen account generation is required"}
	}
	return identity, nil
}

func oauthProvider(provider string) string {
	if provider == "" {
		return ProviderGo
	}
	return provider
}

func oauthPayload(provider string, credentials Credentials) json.RawMessage {
	var value any = credentials
	if provider == ProviderZen {
		value = ZenCredentials{Kind: "oauth", OAuth: &credentials}
	}
	body, _ := json.Marshal(value)
	return body
}

func (owner *module) provider(call sdk.Call) error {
	if call.Scope.Provider == "" || call.Scope.Provider == ProviderGo ||
		owner.zen && (call.Scope.Provider == ProviderZen || call.Scope.Provider == ProviderZenPublic) {
		return nil
	}
	return &sdk.Error{Code: "permission_denied", Message: "The selected provider is not served by this distribution"}
}

func (owner *module) startZenIdentity(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.AuthStartRequest) (any, error) {
	if request.RedirectURI != "" || !owner.zen {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Zen identity requires its declared authentication method"}
	}
	var identity ZenCredentials
	switch {
	case request.Method == "public" && call.Scope.Provider == ProviderZenPublic:
		if len(request.Input) != 0 && string(request.Input) != "null" && string(request.Input) != "{}" {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Public Zen accepts no personal credential"}
		}
		identity = ZenCredentials{Kind: "public", Key: "public"}
	case request.Method == "key" && call.Scope.Provider == ProviderZen:
		var input struct {
			Key string `json:"key"`
		}
		if decode(request.Input, &input) != nil || !validServiceKey(input.Key) {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Zen requires a private service-account key through authentication input"}
		}
		identity = ZenCredentials{Kind: "key", Key: input.Key}
	default:
		return nil, &sdk.Error{Code: "permission_denied", Message: "Zen authentication method belongs to another provider"}
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	private, _ := json.Marshal(identity)
	flow := deviceFlow{State: "pending", Deadline: owner.options.Now().Add(10 * time.Minute),
		Provider: call.Scope.Provider, Method: request.Method, Private: private}
	if _, err := stateWrite(ctx, stream, id, "0", flow); err != nil {
		return nil, err
	}
	value, _ := json.Marshal(continuation{ID: id})
	return sdk.AuthStartResult{Continuation: value, PollAfterMs: 1}, nil
}

func (owner *module) finishZenIdentity(ctx context.Context, call sdk.Call, stream *sdk.Stream, id, version string, flow deviceFlow) (any, error) {
	if flow.State != "pending" || !owner.options.Now().Before(flow.Deadline) {
		return nil, &sdk.Error{Code: "expired", Message: "Zen authentication flow has ended"}
	}
	var identity ZenCredentials
	if json.Unmarshal(flow.Private, &identity) != nil || identity.Validate(call.Scope.Provider) != nil {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Zen authentication authority changed"}
	}
	if call.Scope.Account != "" {
		previous, err := selectedZen(call)
		if err != nil || previous.AccountID(call.Scope.Provider) != identity.AccountID(call.Scope.Provider) {
			return nil, &sdk.Error{Code: "permission_denied", Message: "Zen authentication did not match the selected identity"}
		}
	}
	flow.State, flow.Private = "finished", nil
	if _, err := stateWrite(ctx, stream, id, version, flow); err != nil {
		return nil, err
	}
	private, _ := json.Marshal(identity)
	accountID := identity.AccountID(call.Scope.Provider)
	name := "OpenCode Zen service key [" + accountID[:12] + "]"
	if identity.Kind == "public" {
		name = "OpenCode Zen public"
	}
	return sdk.AuthResult{ID: accountID, Name: name, Credentials: private}, nil
}
