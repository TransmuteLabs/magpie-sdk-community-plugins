package kimiwork

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type authContinuation struct {
	Method string `json:"method"`
	ID     string `json:"id"`
}

type savedRecordInput struct {
	Record string `json:"record"`
}

type storedAuthFlow struct {
	Method string      `json:"method"`
	Saved  Credentials `json:"saved,omitempty"`
}

func tokenClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(decoded, &claims) != nil {
		return nil
	}
	return claims
}

func TokenExpiryMs(token string) int64 {
	claims := tokenClaims(token)
	if claims == nil {
		return 0
	}
	rawExp, ok := claims["exp"]
	if !ok {
		return 0
	}
	num, ok := asFloat(rawExp)
	if !ok || num <= 0 || math.IsNaN(num) || math.IsInf(num, 0) {
		return 0
	}
	return int64(num * 1000)
}

func tokenSub(token string) string {
	claims := tokenClaims(token)
	if claims == nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case json.Number:
		val, err := typed.Float64()
		return val, err == nil && !math.IsNaN(val) && !math.IsInf(val, 0)
	default:
		return 0, false
	}
}

func credentialKey(creds Credentials) string {
	raw, _ := json.Marshal([]string{creds.UID, creds.Access, creds.Refresh})
	return string(raw)
}

func parseSavedRecord(rawRecord string) (Credentials, error) {
	trimmed := strings.TrimSpace(rawRecord)
	if trimmed == "" || len(trimmed) > 64*1024 {
		return Credentials{}, invalid("Saved Kimi Work sign-in record is missing or oversized")
	}
	var doc struct {
		Source    string `json:"source"`
		UID       string `json:"uid"`
		AccountID string `json:"accountId"`
		Access    string `json:"access"`
		Refresh   string `json:"refresh"`
		Expires   int64  `json:"expires"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	if json.Unmarshal([]byte(trimmed), &doc) != nil {
		return Credentials{}, invalid("Saved Kimi Work sign-in must be a valid JSON object")
	}
	if doc.Source != SourceTag || strings.TrimSpace(doc.UID) == "" {
		return Credentials{}, invalid("Saved Kimi Work sign-in requires source kimi-work-desktop and uid")
	}
	expires := doc.Expires
	if expires == 0 {
		expires = doc.ExpiresAt
	}
	if expires == 0 && doc.Access != "" && doc.Refresh != "" {
		expires = TokenExpiryMs(doc.Access)
	}
	accountID := strings.TrimSpace(doc.AccountID)
	if accountID == "" {
		accountID = defaultAccountLabel(doc.UID)
	}
	creds := Credentials{
		Source:    SourceTag,
		UID:       strings.TrimSpace(doc.UID),
		AccountID: accountID,
	}
	if doc.Access != "" && doc.Refresh != "" {
		creds.Access = doc.Access
		creds.Refresh = doc.Refresh
		creds.ExpiresAt = expires
	}
	return creds, nil
}

func defaultAccountLabel(uid string) string {
	prefix := uid
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}
	return "Kimi Work · " + prefix
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if call.Scope.Provider != "" && call.Scope.Provider != ProviderID {
		return nil, invalid("Unknown Kimi Work provider")
	}
	var request sdk.AuthStartRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid Kimi Work auth start request")
	}
	method := request.Method
	if method == "" {
		method = "desktop"
	}
	flow := storedAuthFlow{Method: method}
	switch method {
	case "desktop":
	case sdk.SavedSignInMethod:
		var input savedRecordInput
		if decodeObject(request.Input, &input) != nil {
			return nil, invalid("Saved Kimi Work sign-in requires a record field")
		}
		saved, err := parseSavedRecord(input.Record)
		if err != nil {
			return nil, err
		}
		flow.Saved = saved
	default:
		return nil, invalid("Unsupported Kimi Work sign-in method")
	}
	flowID := randomHex(16)
	stateBytes, err := json.Marshal(flow)
	if err != nil {
		return nil, err
	}
	if stream != nil {
		var casResult sdk.StateCASResult
		if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{
			Key:     "auth." + flowID,
			Version: "0",
			Value:   stateBytes,
		}, &casResult); err != nil {
			return nil, err
		}
		if !casResult.Swapped {
			return nil, &sdk.Error{Code: "conflict", Message: "Kimi Work auth flow conflict"}
		}
	}
	contBytes, err := json.Marshal(authContinuation{Method: method, ID: flowID})
	if err != nil {
		return nil, err
	}
	return sdk.AuthStartResult{
		PollAfterMs:  1000,
		Continuation: contBytes,
	}, nil
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if call.Scope.Provider != "" && call.Scope.Provider != ProviderID {
		return nil, invalid("Unknown Kimi Work provider")
	}
	var request sdk.AuthFinishRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid Kimi Work auth finish request")
	}
	var cont authContinuation
	if decodeObject(request.Continuation, &cont) != nil || cont.ID == "" {
		return nil, invalid("Missing Kimi Work authorization continuation")
	}
	var flow storedAuthFlow
	if stream != nil {
		var stateVal sdk.StateValue
		if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: "auth." + cont.ID}, &stateVal); err != nil {
			return nil, err
		}
		if !stateVal.Found || json.Unmarshal(stateVal.Value, &flow) != nil {
			return nil, invalid("Kimi Work authorization continuation expired or missing")
		}
	} else {
		flow.Method = cont.Method
	}
	if flow.Method == sdk.SavedSignInMethod {
		if !flow.Saved.Valid() {
			return nil, invalid("Invalid saved Kimi Work sign-in")
		}
		rawCreds, err := json.Marshal(flow.Saved)
		if err != nil {
			return nil, err
		}
		return sdk.AuthResult{
			ID:          flow.Saved.UID,
			Name:        flow.Saved.AccountID,
			Credentials: rawCreds,
			ExpiresAt:   flow.Saved.ExpiresAt,
		}, nil
	}
	session, err := owner.options.ReadSession()
	if err != nil {
		var deskErr *DesktopError
		if errors.As(err, &deskErr) {
			if deskErr.ShareMissing {
				return nil, &sdk.Error{Code: deskErr.Code, Message: deskErr.Message, Outcome: "not-started"}
			}
			suffix := ""
			if deskErr.File != "" {
				if deskErr.Missing {
					suffix = "（未找到 " + deskErr.File + "）"
				} else {
					suffix = "（无法读取 " + deskErr.File + "）"
				}
			}
			return nil, &sdk.Error{
				Code:    "kimi_work_no_desktop_login",
				Message: "没有可用的本机 Kimi Work 登录，请先打开 Kimi 并登录 Work。" + suffix,
				Outcome: "not-started",
			}
		}
		return nil, &sdk.Error{Code: "kimi_work_no_desktop_login", Message: err.Error(), Outcome: "not-started"}
	}
	modelsURL := strings.TrimRight(owner.options.Endpoints.Base, "/") + "/models"
	if _, err := owner.jsonRequest(ctx, stream, modelsURL, session.WorkToken, nil, session.DeviceID); err != nil {
		var stErr *statusError
		if errors.As(err, &stErr) && stErr.Status == http.StatusUnauthorized {
			return nil, unauthenticated("本机 Kimi Work 登录已过期，请打开 Kimi 桌面版进入 Work 刷新登录后再试。")
		}
		why := err.Error()
		if errors.As(err, &stErr) && stErr.Status > 0 {
			why = fmt.Sprintf("HTTP %d", stErr.Status)
		} else if errors.Is(err, context.DeadlineExceeded) {
			why = "超时"
		}
		return nil, &sdk.Error{
			Code:    "kimi_work_connection_failed",
			Message: "已找到本机 Kimi Work 登录，但连接 Kimi Work 失败（" + why + "），请检查网络后再试。",
			Outcome: "not-started",
		}
	}
	creds := Credentials{
		Source:    SourceTag,
		UID:       session.UID,
		AccountID: defaultAccountLabel(session.UID),
	}
	if session.WebRefresh != "" {
		creds.Access = session.WebToken
		creds.Refresh = session.WebRefresh
		creds.ExpiresAt = TokenExpiryMs(session.WebToken)
	}
	rawCreds, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	return sdk.AuthResult{
		ID:          creds.UID,
		Name:        creds.AccountID,
		Credentials: rawCreds,
		ExpiresAt:   creds.ExpiresAt,
	}, nil
}

func (owner *module) latestLocked(creds Credentials) Credentials {
	seen := map[string]bool{}
	current := creds
	for {
		key := credentialKey(current)
		if seen[key] {
			break
		}
		pair, exists := owner.renewed[key]
		if !exists {
			break
		}
		seen[key] = true
		current.Access = pair.Access
		current.Refresh = pair.Refresh
		current.ExpiresAt = pair.Expires
	}
	return current
}

func (owner *module) rememberLocked(key string, pair tokenPair) {
	nowMs := owner.options.Now().UnixMilli()
	for existingKey, val := range owner.renewed {
		until := TokenExpiryMs(val.Refresh)
		if until > 0 && until <= nowMs {
			delete(owner.renewed, existingKey)
		}
	}
	owner.renewed[key] = pair
}

func (owner *module) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AuthRefreshRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid Kimi Work refresh request")
	}
	rawInput := call.Auth
	if !nonempty(rawInput) {
		rawInput = request.Credentials
	}
	var creds Credentials
	if decodeObject(rawInput, &creds) != nil || !creds.Valid() {
		return nil, denied("Selected Kimi Work credentials are invalid")
	}
	if !creds.HasPair() {
		rawCreds, err := json.Marshal(creds)
		if err != nil {
			return nil, err
		}
		return sdk.AuthResult{
			ID:          creds.UID,
			Name:        creds.AccountID,
			Credentials: rawCreds,
			ExpiresAt:   creds.ExpiresAt,
		}, nil
	}
	next, err := owner.renewPair(ctx, stream, creds)
	if err != nil {
		return nil, err
	}
	rawCreds, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	return sdk.AuthResult{
		ID:          next.UID,
		Name:        next.AccountID,
		Credentials: rawCreds,
		ExpiresAt:   next.ExpiresAt,
	}, nil
}

func (owner *module) renewPair(ctx context.Context, stream *sdk.Stream, input Credentials) (Credentials, error) {
	owner.mu.Lock()
	current := owner.latestLocked(input)
	nowMs := owner.options.Now().UnixMilli()
	if current.ExpiresAt <= 0 || current.ExpiresAt > nowMs+RefreshLeadMs {
		owner.mu.Unlock()
		return current, nil
	}
	key := credentialKey(current)
	if pending, exists := owner.renewing[key]; exists {
		owner.mu.Unlock()
		select {
		case <-pending.done:
			if pending.err != nil {
				return Credentials{}, pending.err
			}
			updated := input
			updated.Access = pending.pair.Access
			updated.Refresh = pending.pair.Refresh
			updated.ExpiresAt = pending.pair.Expires
			return updated, nil
		case <-ctx.Done():
			return Credentials{}, ctx.Err()
		}
	}
	callState := &renewCall{done: make(chan struct{})}
	owner.renewing[key] = callState
	owner.mu.Unlock()

	pair, err := owner.fetchRefresh(ctx, stream, current)

	owner.mu.Lock()
	callState.pair = pair
	callState.err = err
	if err == nil {
		owner.rememberLocked(key, pair)
		owner.renewed[credentialKey(input)] = pair
	}
	if owner.renewing[key] == callState {
		delete(owner.renewing, key)
	}
	close(callState.done)
	owner.mu.Unlock()

	if err != nil {
		return Credentials{}, err
	}
	updated := input
	updated.Access = pair.Access
	updated.Refresh = pair.Refresh
	updated.ExpiresAt = pair.Expires
	return updated, nil
}

func (owner *module) fetchRefresh(ctx context.Context, stream *sdk.Stream, current Credentials) (tokenPair, error) {
	payload, err := owner.jsonRequest(ctx, stream, owner.options.Endpoints.Refresh, "", map[string]any{
		"refresh_token": current.Refresh,
	}, "")
	if err != nil {
		var stErr *statusError
		if errors.As(err, &stErr) && stErr.Status == http.StatusUnauthorized {
			return tokenPair{}, &sdk.Error{
				Code:    "kimi_work_membership_expired",
				Message: "Kimi Work 会员登录续期被拒绝，请重新连接 Kimi Work（magpie plugin login kimi-work）以恢复自动续期。",
				Outcome: "not-started",
			}
		}
		return tokenPair{}, &sdk.Error{
			Code:    "upstream",
			Message: "Kimi Work 登录续期暂不可用，请稍后重试。",
			Outcome: "not-started",
		}
	}
	access := pickString(payload, "access_token", "accessToken")
	refresh := pickString(payload, "refresh_token", "refreshToken")
	expires := TokenExpiryMs(access)
	oldSub := tokenSub(current.Access)
	newSub := tokenSub(access)
	if access == "" || refresh == "" || expires <= owner.options.Now().UnixMilli() ||
		(oldSub != "" && newSub != "" && oldSub != newSub) {
		return tokenPair{}, &sdk.Error{
			Code:    "upstream",
			Message: "Kimi Work 续期响应格式无效。",
			Outcome: "not-started",
		}
	}
	return tokenPair{Access: access, Refresh: refresh, Expires: expires}, nil
}

func pickAny(obj map[string]any, first, second string) any {
	if obj == nil {
		return nil
	}
	if val, ok := obj[first]; ok && val != nil {
		return val
	}
	return obj[second]
}

func pickString(obj map[string]any, first, second string) string {
	val := pickAny(obj, first, second)
	str, _ := val.(string)
	return str
}

func pickObject(obj map[string]any, first, second string) map[string]any {
	val := pickAny(obj, first, second)
	sub, _ := val.(map[string]any)
	return sub
}
