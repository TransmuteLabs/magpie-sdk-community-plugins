package zcode

import (
	"context"
	"encoding/json"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const savedSignInPrefix = sdk.SavedSignInMethod + "-"

// communitySnapshot accepts one explicitly selected donor record. It does not
// open a legacy store or acknowledge deletion of the source account.
func communitySnapshot(input json.RawMessage, now time.Time) (Credentials, error) {
	var envelope struct {
		Community json.RawMessage `json:"community"`
	}
	if decode(input, &envelope) != nil {
		return Credentials{}, ErrCredentials
	}
	return communityCredentials(envelope.Community, now)
}

func communityCredentials(input json.RawMessage, now time.Time) (Credentials, error) {
	var record struct {
		Type     string `json:"type"`
		Key      string `json:"key,omitempty"`
		Metadata struct {
			Site string `json:"site,omitempty"`
		} `json:"metadata,omitempty"`
		Refresh   string `json:"refresh,omitempty"`
		Access    string `json:"access,omitempty"`
		Expires   int64  `json:"expires,omitempty"`
		AccountID string `json:"accountId,omitempty"`
		UID       string `json:"uid,omitempty"`
	}
	if decode(input, &record) != nil {
		return Credentials{}, ErrCredentials
	}
	var account Credentials
	switch record.Type {
	case "api":
		if record.Refresh != "" || record.Access != "" || record.Expires != 0 {
			return Credentials{}, ErrCredentials
		}
		account = Credentials{Kind: "legacy-key", Site: record.Metadata.Site, Key: record.Key}
		if account.Site == "" {
			account.Site = "zai"
		}
	case "oauth":
		legacy, err := protocol.ReadObject([]byte(record.Refresh))
		if err != nil {
			return Credentials{}, ErrCredentials
		}
		if legacy["source"] == "zcode" {
			return Credentials{}, &sdk.Error{Code: "reauthorization_required",
				Message: "This sign-in follows ZCode Desktop. Sign in with ZCode and select the same site and plan.",
				Outcome: "not-started"}
		}
		var state struct {
			Site   string `json:"site"`
			Base   string `json:"base"`
			Device string `json:"device"`
			JWT    string `json:"jwt"`
			Plan   string `json:"plan"`
		}
		if decode([]byte(record.Refresh), &state) != nil || record.Key != "" || record.Metadata.Site != "" ||
			state.JWT != record.Access || tokenDeadline(state.JWT, nil, now) != record.Expires ||
			record.Expires == 0 || !safeText(record.UID) || !safeText(state.Plan) {
			return Credentials{}, ErrCredentials
		}
		bases := map[string]string{"zai": "https://api.z.ai/api/anthropic", "bigmodel": "https://open.bigmodel.cn/api/anthropic"}
		if state.Base != bases[state.Site] {
			return Credentials{}, ErrCredentials
		}
		account = Credentials{Kind: "start", Site: state.Site, UserID: record.UID, Device: state.Device, SessionToken: state.JWT, ExpiresAt: record.Expires}
		account.DisplayName = oauthDisplayName(map[string]any{"name": record.AccountID})
	default:
		return Credentials{}, ErrCredentials
	}
	return account, account.Validate(now)
}

func (owner *module) startSavedSignIn(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.AuthStartRequest) (any, error) {
	var input struct {
		Record string `json:"record"`
	}
	if request.RedirectURI != "" || decode(request.Input, &input) != nil || len(input.Record) > 16*1024 {
		return nil, invalid("A saved ZCode sign-in requires one legacy record and no redirect")
	}
	account, err := communityCredentials(json.RawMessage(input.Record), owner.options.Now())
	if err != nil {
		return nil, communityImportError(err)
	}
	if call.Scope.Account != "" {
		current, err := owner.identity(call)
		if err != nil {
			return nil, err
		}
		if current.AccountID() != account.AccountID() {
			return nil, &sdk.Error{Code: "permission_denied", Message: "The saved sign-in belongs to another ZCode account", Outcome: "not-started"}
		}
	}
	return owner.startSnapshotSignIn(ctx, call, stream, account)
}

func (owner *module) startSnapshotSignIn(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials) (any, error) {
	id, err := nonce()
	if err != nil {
		return nil, err
	}
	id = savedSignInPrefix + id
	value, err := loadFlowState(ctx, stream, id)
	if err != nil {
		return nil, err
	}
	credentials, err := json.Marshal(account)
	if err != nil {
		return nil, err
	}
	flow := authFlow{
		SavedSignIn: id, Deadline: owner.options.Now().Add(5 * time.Minute).UnixMilli(), Phase: "ready", Ready: credentials,
		TargetAccount: call.Scope.Account, TargetGeneration: call.Scope.Generation,
	}
	if _, err := writeFlow(ctx, stream, id, value.Version, flow); err != nil {
		return nil, err
	}
	continuation, _ := json.Marshal(authContinuation{ID: id})
	return sdk.AuthStartResult{Continuation: continuation}, nil
}

func (owner *module) finishSavedSignIn(ctx context.Context, call sdk.Call, stream *sdk.Stream, id, version string, flow authFlow) (any, error) {
	if flow.Phase != "ready" {
		return nil, invalid("Saved sign-in already completed; start a new import")
	}
	credentials := flow.Ready
	flow.Phase, flow.Ready = "done", nil
	if _, err := writeFlow(ctx, stream, id, version, flow); err != nil {
		return nil, err
	}
	if flow.Deadline <= owner.options.Now().UnixMilli() {
		return nil, invalid("Saved sign-in expired; start a new import")
	}
	var account Credentials
	if decode(credentials, &account) != nil || account.Validate(owner.options.Now()) != nil {
		return nil, invalid("Import requires a valid unexpired ZCode credential snapshot")
	}
	if account.Kind == "start" {
		if _, _, err := owner.principal(ctx, stream, account, ""); err != nil {
			return nil, err
		}
		if _, err := owner.startEntitlement(ctx, stream, account, true); err != nil {
			return nil, err
		}
	}
	return owner.snapshotResult(call, account)
}

func communityImportError(err error) error {
	if failure, ok := err.(*sdk.Error); ok {
		return failure
	}
	return invalid("Community record requires reauthorization or a supported credential snapshot")
}
