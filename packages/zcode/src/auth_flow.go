package zcode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const apiKeySignIn = "api-key"

type authSelection struct {
	Site          string `json:"site,omitempty"`
	LoginProvider string `json:"loginProvider,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Organization  string `json:"organization,omitempty"`
	Project       string `json:"project,omitempty"`
	Product       string `json:"product,omitempty"`
	APIKey        string `json:"apiKeyId,omitempty"`
}

func loginProviderMatches(site, provider string) bool {
	return provider == "" || provider == site || site == "bigmodel" && provider == "zai_cn"
}

type authContinuation struct {
	ID string `json:"id"`
}

type authFlow struct {
	Selection        authSelection   `json:"selection"`
	SavedSignIn      string          `json:"savedSignIn,omitempty"`
	PollToken        string          `json:"pollToken"`
	FlowID           string          `json:"flowId"`
	Device           string          `json:"device"`
	Deadline         int64           `json:"deadline"`
	Interval         int64           `json:"interval"`
	Next             int64           `json:"next"`
	Phase            string          `json:"phase"`
	TargetAccount    string          `json:"targetAccount,omitempty"`
	TargetGeneration uint64          `json:"targetGeneration,omitempty"`
	TargetUser       string          `json:"targetUser,omitempty"`
	Ready            json.RawMessage `json:"ready,omitempty"`
	ReadyAt          int64           `json:"readyAt,omitempty"`
	PollLeaseUntil   int64           `json:"pollLeaseUntil,omitempty"`
	Result           *sdk.AuthResult `json:"result,omitempty"`
}

func flowKey(id string) (string, error) {
	identity, saved := strings.CutPrefix(id, savedSignInPrefix)
	if len(identity) != 32 {
		return "", invalid("Invalid authorization continuation")
	}
	for _, character := range identity {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", invalid("Invalid authorization continuation")
		}
	}
	if saved {
		return "auth.saved-signin", nil
	}
	return "auth." + id, nil
}

func loadFlowState(ctx context.Context, stream *sdk.Stream, id string) (sdk.StateValue, error) {
	key, err := flowKey(id)
	if err != nil {
		return sdk.StateValue{}, err
	}
	var value sdk.StateValue
	err = stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: key}, &value)
	return value, err
}

func readFlow(ctx context.Context, stream *sdk.Stream, id string) (authFlow, string, error) {
	value, err := loadFlowState(ctx, stream, id)
	if err != nil {
		return authFlow{}, "", err
	}
	var flow authFlow
	if !value.Found || decode(value.Value, &flow) != nil || flow.Deadline <= 0 {
		return authFlow{}, "", invalid("Authorization continuation is missing or unreadable")
	}
	if strings.HasPrefix(id, savedSignInPrefix) {
		if flow.SavedSignIn != id {
			return authFlow{}, "", invalid("Saved sign-in has been superseded")
		}
	} else if flow.PollToken == "" || flow.FlowID == "" || flow.Device == "" || flow.Interval < 1000 {
		return authFlow{}, "", invalid("Authorization continuation is missing or unreadable")
	}
	return flow, value.Version, nil
}

func writeFlow(ctx context.Context, stream *sdk.Stream, id, version string, flow authFlow) (string, error) {
	key, err := flowKey(id)
	if err != nil {
		return "", err
	}
	value, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	var result sdk.StateCASResult
	if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{Key: key, Version: version, Value: value}, &result); err != nil {
		return "", err
	}
	if !result.Swapped {
		return "", &sdk.Error{Code: "conflict", Message: "Authorization flow is being completed by another invocation", Outcome: "not-started"}
	}
	return result.Version, nil
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := providerScope(call); err != nil {
		return nil, err
	}
	var request sdk.AuthStartRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if request.Method == sdk.SavedSignInMethod {
		return owner.startSavedSignIn(ctx, call, stream, request)
	}
	if request.Method == apiKeySignIn {
		return owner.startAPIKeySignIn(ctx, call, stream, request)
	}
	if request.Method == "snapshot" {
		if request.RedirectURI != "" || len(request.Input) != 0 {
			return nil, invalid("Snapshot import does not create an authorization flow")
		}
		return sdk.AuthStartResult{}, nil
	}
	if request.Method != "" && request.Method != "browser" || request.RedirectURI != "" {
		return nil, unsupported("Use browser authorization with the vendor init/poll flow or explicit snapshot import")
	}
	selection := authSelection{Site: "zai", Kind: "auto"}
	var existing *Credentials
	if call.Scope.Account != "" {
		current, err := owner.identity(call)
		if err != nil {
			return nil, err
		}
		if current.Kind == "legacy-key" {
			return nil, invalid("Use GLM Coding Plan API key to sign in again to this account")
		}
		existing = &current
		selection.Site, selection.Kind, selection.LoginProvider = current.Site, current.Kind, current.LoginProvider
	}
	if len(request.Input) != 0 && !bytes.Equal(bytes.TrimSpace(request.Input), []byte("null")) {
		if err := decode(request.Input, &selection); err != nil {
			return nil, err
		}
	}
	if selection.Site == "zai_cn" && (selection.LoginProvider == "" || selection.LoginProvider == "zai_cn") {
		selection.Site, selection.LoginProvider = "bigmodel", "zai_cn"
	}
	if selection.Site != "zai" && selection.Site != "bigmodel" ||
		!loginProviderMatches(selection.Site, selection.LoginProvider) ||
		selection.Kind != "auto" && selection.Kind != "individual" && selection.Kind != "team" && selection.Kind != "start" {
		return nil, invalid("Authorization requires a supported site and Personal/Team/Start selection")
	}
	for _, value := range []string{selection.Organization, selection.Project, selection.Product, selection.APIKey} {
		if value != "" && !safeText(value) {
			return nil, invalid("Invalid explicit authorization selection")
		}
	}
	flow := authFlow{Selection: selection, Phase: "awaiting"}
	if call.Scope.Account != "" {
		current := *existing
		if current.Site != selection.Site || (current.LoginProvider == "zai_cn") != (selection.LoginProvider == "zai_cn") || selection.Kind != "auto" && selection.Kind != current.Kind {
			return nil, invalid("Renewal must keep the selected account's site and plan")
		}
		flow.Selection = authSelection{Site: current.Site, Kind: current.Kind, LoginProvider: selection.LoginProvider,
			Organization: current.Organization, Project: current.Project, Product: current.Product}
		if current.Location != nil {
			flow.Selection.Organization, flow.Selection.Project, flow.Selection.APIKey = current.Location.OrganizationID, current.Location.ProjectID, current.Location.APIKeyID
		}
		flow.TargetAccount, flow.TargetUser, flow.Device = call.Scope.Account, current.UserID, accountDevice(current)
		flow.TargetGeneration = call.Scope.Generation
	}
	secret, err := nonce()
	if err != nil {
		return nil, err
	}
	second, err := nonce()
	if err != nil {
		return nil, err
	}
	flow.PollToken = secret + second
	if flow.Device == "" {
		flow.Device = seededUUID("zcode-auth-device\x00" + secret)
	}
	started := owner.options.Now()
	loginProvider := flow.Selection.LoginProvider
	if loginProvider == "" {
		loginProvider = flow.Selection.Site
	}
	response, err := owner.vendorRequest(ctx, stream, http.MethodPost, owner.zcodeOrigin()+"/api/v1/oauth/cli/init",
		http.Header{"Authorization": {"Bearer " + flow.PollToken}}, map[string]string{"provider": loginProvider})
	if err != nil {
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode authorization could not be initialized"}
	}
	data, err := responseData(response)
	if err != nil {
		return nil, err
	}
	flowID, _ := data["flow_id"].(string)
	authorizeURL, _ := data["authorize_url"].(string)
	address, parseErr := url.Parse(authorizeURL)
	expiresAt, validExpiry := finiteNumber(data["expires_at"])
	interval, validInterval := finiteNumber(data["poll_interval_sec"])
	now := float64(owner.options.Now().UnixMilli())
	deadline := expiresAt * 1000
	if serverTime, valid := finiteNumber(data["server_time"]); valid {
		deadline = now + (expiresAt-serverTime)*1000
	}
	if !safeText(flowID) || parseErr != nil || address.Scheme != "https" || address.Host == "" ||
		address.User != nil || address.Query().Get("state") == "" || !validExpiry || !validInterval ||
		deadline <= now || deadline > float64(started.Add(24*time.Hour).UnixMilli()) ||
		interval < 1 || interval >= 300 {
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode returned an invalid authorization flow"}
	}
	flow.FlowID, flow.Deadline = flowID, min(int64(deadline), started.Add(5*time.Minute).UnixMilli())
	flow.Interval = int64(interval * 1000)
	if flow.Interval >= flow.Deadline-owner.options.Now().UnixMilli() {
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode authorization interval exceeds its remaining lifetime"}
	}
	id, err := nonce()
	if err != nil {
		return nil, err
	}
	if _, err := writeFlow(ctx, stream, id, "", flow); err != nil {
		return nil, err
	}
	continuation, _ := json.Marshal(authContinuation{ID: id})
	return sdk.AuthStartResult{URL: address.String(), PollAfterMs: flow.Interval, Continuation: continuation}, nil
}

func (owner *module) startAPIKeySignIn(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.AuthStartRequest) (any, error) {
	var input struct {
		Site string `json:"site,omitempty"`
		Key  string `json:"key"`
	}
	if request.RedirectURI != "" || decode(request.Input, &input) != nil {
		return nil, invalid("Select a supported site and enter a GLM Coding Plan API key")
	}
	account := Credentials{Site: "zai", Kind: "legacy-key"}
	if call.Scope.Account != "" {
		current, err := owner.identity(call)
		if err != nil {
			return nil, err
		}
		if current.Kind != account.Kind || current.Key != input.Key || input.Site != "" && input.Site != current.Site {
			return nil, invalid("Use the selected account's API key and site, or add a new account")
		}
		account = current
	}
	if input.Site != "" {
		account.Site = input.Site
	}
	account.Key = input.Key
	if account.Validate(owner.options.Now()) != nil {
		return nil, invalid("Select a supported site and enter a GLM Coding Plan API key")
	}
	if _, err := owner.readPaidUsage(ctx, stream, account, ""); err != nil {
		return nil, &sdk.Error{Code: "authentication_failed", Message: "Could not verify the API key with the selected site's quota service", Outcome: "not-started"}
	}
	return owner.startSnapshotSignIn(ctx, call, stream, account)
}

func (owner *module) finishFlow(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.AuthFinishRequest) (any, error) {
	var continuation authContinuation
	if decode(request.Continuation, &continuation) != nil || request.Code != "" || len(request.Input) != 0 &&
		!bytes.Equal(bytes.TrimSpace(request.Input), []byte("null")) && !bytes.Equal(bytes.TrimSpace(request.Input), []byte("{}")) {
		return nil, invalid("Complete the vendor polling continuation without a callback code or replacement credentials")
	}
	flow, version, err := readFlow(ctx, stream, continuation.ID)
	if err != nil {
		return nil, err
	}
	if flow.TargetAccount != call.Scope.Account || flow.TargetGeneration != call.Scope.Generation {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Authorization continuation belongs to another selected account", Outcome: "not-started"}
	}
	if flow.SavedSignIn != "" {
		return owner.finishSavedSignIn(ctx, call, stream, continuation.ID, version, flow)
	}
	if flow.Deadline <= owner.options.Now().UnixMilli() {
		return nil, invalid("Authorization expired; start a new explicit flow")
	}
	if flow.Phase == "done" && flow.Result != nil {
		return *flow.Result, nil
	}
	if flow.Phase == "polling" && flow.PollLeaseUntil > 0 && flow.PollLeaseUntil <= owner.options.Now().UnixMilli() {
		flow.Phase = "awaiting"
		version, err = writeFlow(ctx, stream, continuation.ID, version, flow)
		if err != nil {
			return nil, err
		}
	}
	if flow.Phase == "polling" || flow.Phase == "exchanging" || flow.Phase == "uncertain" {
		return nil, uncertain("Another completion may have consumed this flow; no exchange was repeated")
	}
	if flow.Phase == "failed" {
		return nil, invalid("Authorization was declined or failed; start a new explicit flow")
	}
	if flow.Phase != "awaiting" && flow.Phase != "ready" {
		return nil, invalid("Authorization continuation has an invalid phase")
	}
	if flow.Phase == "awaiting" {
		if flow.Next > owner.options.Now().UnixMilli() {
			return sdk.AuthResult{Pending: true}, nil
		}
		flow.Phase, flow.Next = "polling", owner.options.Now().Add(time.Duration(flow.Interval)*time.Millisecond).UnixMilli()
		flow.PollLeaseUntil = owner.options.Now().Add(20 * time.Second).UnixMilli()
		version, err = writeFlow(ctx, stream, continuation.ID, version, flow)
		if err != nil {
			return nil, err
		}
		pollStarted := owner.options.Now()
		response, pollErr := owner.vendorRequest(ctx, stream, http.MethodGet,
			owner.zcodeOrigin()+"/api/v1/oauth/cli/poll/"+url.PathEscape(flow.FlowID),
			http.Header{"Authorization": {"Bearer " + flow.PollToken}}, nil)
		data, dataErr := responseData(response)
		if response.Status >= 400 && response.Status < 500 && response.Status != 408 && response.Status != 429 {
			flow.Phase = "failed"
			writeFlow(ctx, stream, continuation.ID, version, flow)
			return nil, invalid("Authorization poll was rejected; start a new explicit flow")
		}
		if pollErr != nil || response.Status == 408 || response.Status == 429 || response.Status >= 500 {
			flow.Phase = "awaiting"
			if _, err := writeFlow(ctx, stream, continuation.ID, version, flow); err != nil {
				return nil, err
			}
			return sdk.AuthResult{Pending: true}, nil
		}
		if dataErr != nil || data["status"] == "failed" {
			flow.Phase = "failed"
			writeFlow(ctx, stream, continuation.ID, version, flow)
			return nil, invalid("Authorization was declined or returned an invalid polling response")
		}
		if data["status"] == "pending" {
			flow.Phase = "awaiting"
			if _, err := writeFlow(ctx, stream, continuation.ID, version, flow); err != nil {
				return nil, err
			}
			return sdk.AuthResult{Pending: true}, nil
		}
		if data["status"] != "ready" {
			return nil, uncertain("Authorization returned an unknown completion state")
		}
		flow.Ready, _ = json.Marshal(data)
		flow.ReadyAt = pollStarted.UnixMilli()
		flow.Phase = "ready"
		version, err = writeFlow(ctx, stream, continuation.ID, version, flow)
		if err != nil {
			return nil, err
		}
	}
	flow.Phase = "exchanging"
	version, err = writeFlow(ctx, stream, continuation.ID, version, flow)
	if err != nil {
		return nil, err
	}
	result, err := owner.completeFlow(ctx, call, stream, flow)
	if err != nil {
		flow.Phase = "uncertain"
		writeFlow(ctx, stream, continuation.ID, version, flow)
		return nil, err
	}
	flow.Phase, flow.Result = "done", &result
	if _, err := writeFlow(ctx, stream, continuation.ID, version, flow); err != nil {
		return nil, uncertain("Authorization completed but its receipt could not be persisted; exchanges will not be repeated")
	}
	return result, nil
}
