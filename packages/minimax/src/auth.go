package minimax

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const deviceGrant = "urn:ietf:params:oauth:grant-type:device_code"

type deviceFlow struct {
	State, Site, Verifier, Code, CodeField string
	Deadline, Next, Interval               int64
	Failures                               int
}
type continuation struct {
	ID string `json:"id"`
}

func number(value any) (float64, bool) {
	var numeric float64
	switch typedValue := value.(type) {
	case float64:
		numeric = typedValue
	case json.Number:
		var err error
		numeric, err = typedValue.Float64()
		if err != nil {
			return 0, false
		}
	case string:
		var err error
		numeric, err = json.Number(strings.TrimSpace(typedValue)).Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return numeric, !math.IsNaN(numeric) && !math.IsInf(numeric, 0)
}
func positive(value any) float64 {
	numeric, ok := number(value)
	if !ok || numeric <= 0 {
		return 0
	}
	return numeric
}
func nonempty(value json.RawMessage) bool {
	literal := strings.TrimSpace(string(value))
	return literal != "" && literal != "null" && literal != "{}"
}

func (owner *module) oauth(ctx context.Context, stream *sdk.Stream, site Site, path string, fields url.Values) (map[string]any, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := owner.siteRequest(ctx, stream, "POST", site.Account+path, []byte(fields.Encode()), http.Header{"Accept": {"application/json"}, "Content-Type": {"application/x-www-form-urlencoded"}})
	if err != nil {
		return nil, 0, err
	}
	buffer, err := bounded(response, 1<<20)
	if err != nil {
		return nil, response.StatusCode, err
	}
	var result map[string]any
	if json.Unmarshal(buffer.Body, &result) != nil || result == nil {
		return nil, buffer.Status, &APIError{Status: buffer.Status, Code: "invalid_json_response"}
	}
	code := text(result["error"])
	if code != "" || buffer.Status < 200 || buffer.Status >= 300 {
		if !contains([]string{"authorization_pending", "slow_down", "invalid_grant", "access_denied", "expired_token", "invalid_client", "unauthorized_client"}, code) {
			code = "oauth_request_failed"
		}
		return nil, buffer.Status, &APIError{Status: buffer.Status, Code: code}
	}
	return result, buffer.Status, nil
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, err := owner.site(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var request sdk.AuthStartRequest
	if decode(call.Params, &request) != nil || request.Method != "" && request.Method != "device" || request.RedirectURI != "" || nonempty(request.Input) {
		return nil, invalid("Use MiniMax device authorization without custom input")
	}
	var buffer [32]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buffer[:])
	sum := sha256.Sum256([]byte(verifier))
	value, _, err := owner.oauth(ctx, stream, site, "/oauth2/device/code", url.Values{"client_id": {"mcode-public"}, "scope": {"agent.default"}, "audience": {"agent-backend"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}})
	if err != nil {
		return nil, safeError(err)
	}
	now := owner.options.Now().UnixMilli()
	user := text(value["user_code"])
	code := text(value["device_code"])
	field := "device_code"
	expired := positive(value["expired_in"])
	byUser := code == "" && user != "" && expired > 0
	if byUser {
		code = user
		field = "user_code"
	}
	uri := text(value["verification_uri_complete"])
	if uri == "" {
		uri = text(value["verification_uri"])
	}
	if uri == "" {
		uri = text(value["verification_url"])
	}
	parsed, urlErr := url.Parse(uri)
	if code == "" || len(code) > 8192 || user == "" || len(user) > 256 || urlErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, invalid("MiniMax returned an invalid device challenge")
	}
	ttl := positive(value["expires_in"]) * 1000
	if ttl == 0 && expired > 0 {
		if expired >= 1e12 {
			ttl = expired - float64(now)
		} else {
			ttl = expired * 1000
		}
	}
	if expired >= 1e12 && expired <= float64(now) {
		return nil, invalid("MiniMax returned an expired device challenge")
	}
	if ttl <= 0 {
		ttl = 600000
	}
	ttl = min(ttl, 600000)
	interval := positive(value["interval"])
	if interval == 0 {
		interval = 5000
	} else if !byUser {
		interval *= 1000
	}
	interval = max(1, min(interval, 300000))
	flow := deviceFlow{State: "pending", Site: site.ID, Verifier: verifier, Code: code, CodeField: field, Deadline: now + int64(ttl), Next: now + int64(interval), Interval: int64(interval)}
	id := strings.ReplaceAll(randomID(), "-", "")
	if _, err := writeFlow(ctx, stream, id, "0", flow); err != nil {
		return nil, err
	}
	return sdk.AuthStartResult{URL: uri, UserCode: user, PollAfterMs: int64(interval), Continuation: raw(continuation{ID: id})}, nil
}

func writeFlow(ctx context.Context, stream *sdk.Stream, id, version string, flow deviceFlow) (string, error) {
	var result sdk.StateCASResult
	if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{Key: "device." + id, Version: version, Value: raw(flow)}, &result); err != nil {
		return "", err
	}
	if !result.Swapped {
		return "", &sdk.Error{Code: "conflict", Message: "MiniMax device flow changed"}
	}
	return result.Version, nil
}
func readFlow(ctx context.Context, stream *sdk.Stream, id string) (deviceFlow, string, error) {
	var value sdk.StateValue
	var flow deviceFlow
	if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: "device." + id}, &value); err != nil {
		return flow, "", err
	}
	if !value.Found || json.Unmarshal(value.Value, &flow) != nil || flow.Deadline <= 0 {
		return flow, "", invalid("MiniMax device continuation is missing or unreadable")
	}
	return flow, value.Version, nil
}

func transientPoll(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		if contains([]string{"invalid_grant", "access_denied", "expired_token", "invalid_client", "unauthorized_client"}, api.Code) {
			return false
		}
		return api.Status == 408 || api.Status == 429 || api.Status >= 500 || api.Status == 400 && api.Code == "invalid_json_response"
	}
	var networkError net.Error
	return errors.As(err, &networkError) || errors.Is(err, context.DeadlineExceeded)
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, err := owner.site(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var request sdk.AuthFinishRequest
	var key continuation
	if decode(call.Params, &request) != nil || request.Code != "" || nonempty(request.Input) || decode(request.Continuation, &key) != nil || len(key.ID) != 32 || strings.Trim(key.ID, "0123456789abcdef") != "" {
		return nil, invalid("A MiniMax device continuation is required")
	}
	flow, version, err := readFlow(ctx, stream, key.ID)
	if err != nil {
		return nil, err
	}
	if flow.Site != site.ID {
		return nil, denied("Device continuation belongs to another provider")
	}
	now := owner.options.Now().UnixMilli()
	if flow.State != "pending" || now >= flow.Deadline {
		return nil, &sdk.Error{Code: "expired", Message: "Device flow ended or is uncertain; start authorization again"}
	}
	if now < flow.Next {
		return sdk.AuthResult{Pending: true}, nil
	}
	flow.State = "polling"
	version, err = writeFlow(ctx, stream, key.ID, version, flow)
	if err != nil {
		return nil, err
	}
	pollCtx, cancel := context.WithTimeout(ctx, time.Duration(flow.Deadline-now)*time.Millisecond)
	defer cancel()
	value, _, pollErr := owner.oauth(pollCtx, stream, site, "/oauth2/token", url.Values{"grant_type": {deviceGrant}, flow.CodeField: {flow.Code}, "client_id": {"mcode-public"}, "code_verifier": {flow.Verifier}})
	state := ""
	if value != nil {
		state = text(value["status"])
	}
	var api *APIError
	if errors.As(pollErr, &api) {
		state = api.Code
	}
	delay := flow.Interval
	pending := state == "pending" || state == "authorization_pending" || state == "slow_down"
	if state == "slow_down" {
		flow.Interval = min(flow.Interval+5000, 300000)
		delay = flow.Interval
	}
	if pollErr != nil && !pending && transientPoll(pollErr) && flow.Failures < 3 && ctx.Err() == nil {
		flow.Failures++
		delay = max(flow.Interval, min(30000, max(flow.Interval, 1000)*(1<<flow.Failures)))
		pending = true
	} else if pending {
		flow.Failures = 0
	}
	if pending && owner.options.Now().UnixMilli() < flow.Deadline {
		flow.State = "pending"
		flow.Next = owner.options.Now().UnixMilli() + delay
		if _, err := writeFlow(ctx, stream, key.ID, version, flow); err != nil {
			return nil, err
		}
		return sdk.AuthResult{Pending: true}, nil
	}

	flow.State = "finished"
	flow.Code = ""
	flow.Verifier = ""
	if _, err := writeFlow(ctx, stream, key.ID, version, flow); err != nil {
		return nil, err
	}
	if pollErr != nil {
		return nil, safeError(pollErr)
	}
	if owner.options.Now().UnixMilli() >= flow.Deadline {
		return nil, &sdk.Error{Code: "expired", Message: "MiniMax device authorization expired"}
	}
	if contains([]string{"denied", "access_denied", "expired", "expired_token"}, state) {
		return nil, &sdk.Error{Code: "auth_denied", Message: "MiniMax device authorization was denied or expired"}
	}
	credentials, err := owner.token(site, value, Credentials{})
	if err != nil {
		return nil, err
	}
	client := &SelectedClient{owner: owner, site: site, credentials: credentials, call: call, stream: stream}
	if who, err := client.identity(ctx); err == nil {
		credentials.UserID, credentials.Email, credentials.Name = who.UserID, who.Email, who.Name
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if owner.options.Now().UnixMilli() >= flow.Deadline {
		return nil, &sdk.Error{Code: "expired", Message: "MiniMax device authorization expired before publication"}
	}
	subject := credentials.UserID
	if subject == "" {
		subject = credentials.Subject
	}
	if subject == "" {
		return nil, invalid("MiniMax token did not identify an account")
	}
	credentials.ID = accountID(site.ID, subject)
	if call.Scope.Account != "" {
		previous, err := owner.SelectedClient(ctx, call, stream, site.ID)
		if err != nil || previous.credentials.ID != credentials.ID {
			return nil, denied("Authorization returned a different MiniMax account")
		}
	}
	return authResult(credentials), nil
}

func (owner *module) token(site Site, value map[string]any, previous Credentials) (Credentials, error) {
	access := text(value["access_token"])
	ttl := positive(value["expires_in"])
	if access == "" || len(access) > 32768 || ttl == 0 || ttl > 366*86400 {
		return Credentials{}, invalid("MiniMax returned invalid token credentials")
	}
	result := previous
	result.Site = site.ID
	result.Access = access
	result.ExpiresAt = owner.options.Now().Add(time.Duration(ttl*1000) * time.Millisecond).UnixMilli()
	if refresh := text(value["refresh_token"]); refresh != "" {
		if len(refresh) > 32768 {
			return Credentials{}, invalid("MiniMax refresh token exceeds its limit")
		}
		result.Refresh = refresh
	}
	parts := strings.Split(access, ".")
	if len(parts) == 3 {
		body, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil && len(body) < 32768 {
			var claims map[string]any
			if json.Unmarshal(body, &claims) == nil {
				kind := previous.SubjectKind
				if kind == "" {
					kind = "account_id"
					if text(claims[kind]) == "" {
						kind = "sub"
					}
				}
				subject := text(claims[kind])
				if previous.Subject != "" && subject != "" && subject != previous.Subject {
					return Credentials{}, denied("Refreshed MiniMax token changed account identity")
				}
				if subject != "" {
					result.Subject = subject
					result.SubjectKind = kind
				}
			}
		}
	}
	return result, nil
}
func authResult(client Credentials) sdk.AuthResult {
	name := client.Email
	if name == "" {
		name = client.Name
	}
	if name == "" {
		name = client.UserID
	}
	if name == "" {
		name = client.ID
	}
	return sdk.AuthResult{ID: client.ID, Name: name, Credentials: raw(client), ExpiresAt: client.ExpiresAt}
}
func (owner *module) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AuthRefreshRequest
	if decode(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Refresh credentials must come from the selected host frame", Outcome: "not-started"}
	}
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Refresh requires the exact selected account", Outcome: "not-started"}
	}
	if client.credentials.Refresh == "" {
		return nil, &sdk.Error{Code: "auth_revoked", Message: "MiniMax account requires device authorization", Outcome: "not-started"}
	}
	value, _, err := owner.oauth(ctx, stream, client.site, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {client.credentials.Refresh}, "client_id": {"mcode-public"}, "scope": {"agent.default"}, "audience": {"agent-backend"}})
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Status == 400 && api.Code == "invalid_grant" {
			return nil, &sdk.Error{Code: "auth_revoked", Message: "MiniMax rejected the rotating refresh token; sign in again", Outcome: "uncertain"}
		}
		return nil, uncertain("MiniMax refresh failed; the rotating token was not retried")
	}
	replacement, err := owner.token(client.site, value, client.credentials)
	if err != nil {
		return nil, uncertain("MiniMax refresh returned invalid credentials; do not replay the exchange")
	}
	return authResult(replacement), nil
}

type identity struct{ UserID, Email, Name string }

func (client *SelectedClient) identity(ctx context.Context) (identity, error) {
	data, err := client.accountJSON(ctx, "/v1/api/user/info", nil, nil, "")
	if err != nil {
		return identity{}, err
	}
	var root map[string]any
	json.Unmarshal(data, &root)
	nested := object(root["data"])
	var user map[string]any
	for _, value := range []any{nested["userInfo"], nested["user_info"], root["userInfo"], root["user_info"]} {
		if object(value) != nil {
			user = object(value)
			break
		}
	}
	first := func(keys ...string) string {
		for _, key := range keys {
			if literal := text(user[key]); literal != "" {
				return literal
			}
		}
		return ""
	}
	result := identity{UserID: first("realUserID", "real_user_id"), Email: first("userEmail", "email", "userMail", "user_email"), Name: first("name", "userName", "user_name")}
	if result.UserID == "" {
		return identity{}, invalid("MiniMax account response has no user identity")
	}
	return result, nil
}
