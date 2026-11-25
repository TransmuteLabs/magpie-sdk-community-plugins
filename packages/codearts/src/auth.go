package codearts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type CodeArtsCredential struct {
	AccessKeyID       string   `json:"access_key_id"`
	SecretAccessKey   string   `json:"secret_access_key"`
	SecurityToken     string   `json:"security_token"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
	RefreshToken      string   `json:"refresh_token,omitempty"`
	CodeVerifier      string   `json:"code_verifier,omitempty"`
	DPoPPrivateKeyJWK *DPoPJWK `json:"dpop_private_key_jwk,omitempty"`
	SessionID         string   `json:"session_id,omitempty"`
	Station           string   `json:"station,omitempty"`
	DomainID          string   `json:"domain_id,omitempty"`
	UserID            string   `json:"user_id,omitempty"`
	UserName          string   `json:"user_name,omitempty"`
	UserPhone         string   `json:"user_phone,omitempty"`
	ClientID          string   `json:"client_id,omitempty"`
	// OfficeAce MaaS fields (stored on officeace records)
	ModelAppKey    string `json:"model_app_key,omitempty"`
	ModelAppSecret string `json:"model_app_secret,omitempty"`
	MaaSBaseURL    string `json:"maas_base_url,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
}

func (c CodeArtsCredential) ValidSTS() bool {
	return strings.TrimSpace(c.AccessKeyID) != "" &&
		strings.TrimSpace(c.SecretAccessKey) != "" &&
		strings.TrimSpace(c.SecurityToken) != ""
}

func (c CodeArtsCredential) ValidOfficeAce() bool {
	return c.ValidSTS() || (strings.TrimSpace(c.ModelAppKey) != "" && strings.TrimSpace(c.ModelAppSecret) != "")
}

func (c CodeArtsCredential) ExpiresMs(now time.Time) int64 {
	if c.ExpiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, c.ExpiresAt); err == nil {
			return parsed.UnixMilli()
		}
		if parsed, err := time.Parse(time.RFC3339Nano, c.ExpiresAt); err == nil {
			return parsed.UnixMilli()
		}
	}
	return now.Add(24 * time.Hour).UnixMilli()
}

func (c CodeArtsCredential) BasicAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.ModelAppKey+":"+c.ModelAppSecret))
}

func (c CodeArtsCredential) BasicFingerprint() string {
	return c.ModelAppKey + ":" + c.ModelAppSecret + ":" + c.MaaSBaseURL
}

func (owner *module) officeAceMaaSBase(cred CodeArtsCredential) string {
	raw := strings.TrimRight(strings.TrimSpace(cred.MaaSBaseURL), "/")
	if raw == "" {
		return strings.TrimRight(owner.options.Endpoints.OAMaaSBase, "/")
	}
	if strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
		return raw
	}
	return "https://" + raw
}

var (
	deadRefreshPattern = regexp.MustCompile(`(?i)ExpiredRefreshToken|STS5\.1806|invalid refresh token|refresh token has been used`)
	identifierPattern  = regexp.MustCompile(`(?i)^(?:iam::|hw\d|hid_|[0-9a-f]{24,}$|urn:)`)
)

func IsDeadRefreshToken(text string) bool {
	return deadRefreshPattern.MatchString(text)
}

func b64JSON(seg string) map[string]any {
	if seg == "" {
		return nil
	}
	dec, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		dec, err = base64.StdEncoding.DecodeString(seg)
		if err != nil {
			return nil
		}
	}
	var obj map[string]any
	if json.Unmarshal(dec, &obj) != nil {
		return nil
	}
	return obj
}

func ProfileOf(cred CodeArtsCredential) map[string]any {
	parts := strings.Split(cred.RefreshToken, ".")
	if len(parts) != 3 {
		return nil
	}
	payload := b64JSON(parts[1])
	if payload == nil {
		return nil
	}
	raw := payload["user_profile"]
	switch typed := raw.(type) {
	case string:
		var parsed map[string]any
		if json.Unmarshal([]byte(typed), &parsed) == nil && parsed != nil {
			return parsed
		}
		return b64JSON(typed)
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func strField(obj map[string]any, keys ...string) string {
	if obj == nil {
		return ""
	}
	for _, k := range keys {
		if val, ok := obj[k].(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func AccountIdentityOf(cred CodeArtsCredential) (id string, name string) {
	profile := ProfileOf(cred)
	id = firstOf(
		strField(profile, "principal_urn", "principal_id", "user_id"),
		cred.UserID,
		cred.DomainID,
		cred.SessionID,
		cred.AccessKeyID,
	)
	candidates := []string{
		cred.UserPhone,
		strField(profile, "nick_name", "display_name", "user_name", "account_name"),
		cred.UserName,
	}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" && !identifierPattern.MatchString(trimmed) {
			name = trimmed
			break
		}
	}
	return id, name
}

func AccountSessionIDs(cred CodeArtsCredential) (sessionID, chatID string) {
	id, _ := AccountIdentityOf(cred)
	seed := firstOf(cred.SessionID, id, cred.AccessKeyID)
	digest := sha256Hex([]byte("codearts-session:" + seed))
	return digest[:32], digest[32:64]
}

func BuildOAuthLoginURL(port int, pkce PKCEPair, ticketID string, station Station) string {
	params := url.Values{
		"client_id":             {ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {fmt.Sprintf("http://127.0.0.1:%d%s", port, RedirectPath)},
		"code_challenge":        {pkce.Challenge},
		"code_challenge_method": {"S256"},
		"plugin_name":           {LoginPluginName},
		"plugin_version":        {LoginPluginVersion},
		"theme":                 {OAuthTheme},
		"locale":                {station.Locale},
		"ticket_id":             {ticketID},
	}
	authorize := strings.TrimRight(station.Portal, "/") + "/authorize?" + params.Encode()
	if station.Login != "" {
		sep := "&"
		if !strings.Contains(station.Login, "?") {
			sep = "?"
		}
		return station.Login + sep + "service=" + url.QueryEscape(authorize)
	}
	return authorize
}

func buildPortalLoginResultURL(ok bool, station Station) string {
	status := "fail"
	if ok {
		status = "success"
	}
	return strings.TrimRight(station.Portal, "/") + "/login-result?status=" + status + "&locale=" + url.QueryEscape(station.Locale)
}

func parseSavedCredential(raw string) (CodeArtsCredential, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 64*1024 {
		return CodeArtsCredential{}, invalid("Saved sign-in record is empty or too large")
	}
	var cred CodeArtsCredential
	if err := json.Unmarshal([]byte(trimmed), &cred); err != nil {
		// Also allow wrapper {"access": "{...}", "refresh": "..."}
		var wrapped struct {
			Access  string `json:"access"`
			Refresh string `json:"refresh"`
		}
		if json.Unmarshal([]byte(trimmed), &wrapped) == nil && wrapped.Access != "" {
			if err2 := json.Unmarshal([]byte(wrapped.Access), &cred); err2 != nil {
				return CodeArtsCredential{}, invalid("Saved sign-in JSON could not be parsed")
			}
			if cred.RefreshToken == "" && wrapped.Refresh != "" {
				cred.RefreshToken = wrapped.Refresh
			}
		} else {
			return CodeArtsCredential{}, invalid("Saved sign-in must be a valid JSON object")
		}
	}
	if cred.SessionID == "" {
		cred.SessionID = randomHex(16)
	}
	return cred, nil
}

type authContinuation struct {
	Method   string `json:"method"`
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

type storedSavedFlow struct {
	Method   string             `json:"method"`
	Provider string             `json:"provider"`
	Saved    CodeArtsCredential `json:"saved"`
}

func (owner *module) requestSTSToken(ctx context.Context, stream *sdk.Stream, form url.Values, jwk DPoPJWK, label string) (map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	dpop, err := SignDPoPJWS(jwk, http.MethodPost, owner.options.Endpoints.STSToken, owner.options.Now())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, owner.options.Endpoints.STSToken, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("DPoP", dpop)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return nil, fmt.Errorf("%s token request network error: %w", label, err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var data map[string]any
	_ = json.Unmarshal(bodyBytes, &data)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || data == nil || data["credentials"] == nil {
		msg := fmt.Sprintf("%s token request failed: %d %s", label, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
		if strField(data, "error") == "invalid_grant" || IsDeadRefreshToken(msg) {
			return nil, unauthenticated(msg)
		}
		return nil, &sdk.Error{Code: "upstream", Message: msg, Outcome: "not-started"}
	}
	return data, nil
}

func credentialFromTokenResponse(token map[string]any, codeVerifier string, jwk DPoPJWK) CodeArtsCredential {
	credsObj, _ := token["credentials"].(map[string]any)
	return CodeArtsCredential{
		AccessKeyID:       strField(credsObj, "access_key_id"),
		SecretAccessKey:   strField(credsObj, "secret_access_key"),
		SecurityToken:     strField(credsObj, "security_token"),
		ExpiresAt:         strField(credsObj, "expiration"),
		RefreshToken:      strField(token, "refresh_token"),
		CodeVerifier:      codeVerifier,
		DPoPPrivateKeyJWK: &jwk,
		SessionID:         randomHex(16),
	}
}

func (owner *module) fetchOfficeAceMaasInfo(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential) (appKey, appSecret, baseURL string, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	target := strings.TrimRight(owner.options.Endpoints.OAClawBase, "/") + OAPermissionPath
	signed, err := SignRequestHuawei(cred.AccessKeyID, cred.SecretAccessKey, cred.SecurityToken, http.MethodGet, target, nil, map[string]string{
		"content-type": "application/json",
	}, sdkDateStamp(owner.options.Now()))
	if err != nil {
		return "", "", "", err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", "", err
	}
	for k, v := range signed {
		if !strings.EqualFold(k, "host") {
			req.Header.Set(k, v)
		}
	}
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("OfficeAce permission check failed: HTTP %d", resp.StatusCode)
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc == nil {
		return "", "", "", nil
	}
	info, _ := doc["model_auth_info"].(map[string]any)
	if info == nil {
		if data, ok := doc["data"].(map[string]any); ok {
			info, _ = data["model_auth_info"].(map[string]any)
		}
	}
	if info == nil {
		return "", "", "", nil
	}
	return strField(info, "app_key", "model_app_key"), strField(info, "app_secret", "model_app_secret"), strField(info, "base_url", "maas_base_url", "endpoint"), nil
}

func (owner *module) withOfficeAceMaas(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential) CodeArtsCredential {
	appKey, appSecret, baseURL, err := owner.fetchOfficeAceMaasInfo(ctx, stream, cred)
	if err != nil {
		return cred
	}
	if appKey != "" && appSecret != "" {
		cred.ModelAppKey = appKey
		cred.ModelAppSecret = appSecret
	}
	if baseURL != "" {
		cred.MaaSBaseURL = baseURL
	}
	return cred
}

func (owner *module) refreshCredential(ctx context.Context, stream *sdk.Stream, provider string, cred CodeArtsCredential) (CodeArtsCredential, error) {
	if strings.TrimSpace(cred.RefreshToken) == "" || cred.CodeVerifier == "" || cred.DPoPPrivateKeyJWK == nil {
		return CodeArtsCredential{}, unauthenticated("CodeArts: credential cannot be refreshed (missing refresh_token/code_verifier/dpop key); sign in again")
	}
	clientID := firstOf(cred.ClientID, ClientID)
	label := "CodeArts"
	if provider == OfficeAceProvider {
		label = "OfficeAce"
	}
	token, err := owner.requestSTSToken(ctx, stream, url.Values{
		"client_id":     {clientID},
		"code_verifier": {cred.CodeVerifier},
		"grant_type":    {GrantRefreshToken},
		"refresh_token": {cred.RefreshToken},
	}, *cred.DPoPPrivateKeyJWK, label)
	if err != nil {
		return CodeArtsCredential{}, err
	}
	next := credentialFromTokenResponse(token, cred.CodeVerifier, *cred.DPoPPrivateKeyJWK)
	if next.RefreshToken == "" {
		next.RefreshToken = cred.RefreshToken
	}
	next.DomainID = cred.DomainID
	next.UserID = cred.UserID
	next.UserName = cred.UserName
	next.UserPhone = cred.UserPhone
	next.Station = cred.Station
	next.ClientID = cred.ClientID
	next.ProjectID = cred.ProjectID
	if cred.SessionID != "" {
		next.SessionID = cred.SessionID
	}
	if provider == OfficeAceProvider {
		next.ModelAppKey = cred.ModelAppKey
		next.ModelAppSecret = cred.ModelAppSecret
		next.MaaSBaseURL = cred.MaaSBaseURL
		next = owner.withOfficeAceMaas(ctx, stream, next)
	}
	return next, nil
}

func listenCallbackPort() (net.Listener, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		port := ln.Addr().(*net.TCPAddr).Port
		if port >= MinCallbackPort {
			return ln, port, nil
		}
		_ = ln.Close()
	}
	for tries := 0; tries < 20; tries++ {
		port := MinCallbackPort + rand.IntN(65536-MinCallbackPort)
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			return ln, port, nil
		}
	}
	return nil, 0, errors.New("could not bind loopback OAuth callback port")
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	provider := call.Scope.Provider
	if provider == "" {
		provider = CodeArtsProvider
	}
	if provider != CodeArtsProvider && provider != OfficeAceProvider {
		return nil, invalid("Unknown provider for CodeArts module")
	}
	var request sdk.AuthStartRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid auth start request")
	}
	method := request.Method
	if method == "" {
		method = "oauth"
	}
	flowID := randomHex(16)

	if method == sdk.SavedSignInMethod {
		var input struct {
			Record string `json:"record"`
		}
		if decodeObject(request.Input, &input) != nil {
			return nil, invalid("Saved sign-in requires a record field")
		}
		cred, err := parseSavedCredential(input.Record)
		if err != nil {
			return nil, err
		}
		if provider == CodeArtsProvider && !cred.ValidSTS() {
			return nil, invalid("Saved CodeArts sign-in requires access_key_id, secret_access_key, and security_token")
		}
		if provider == OfficeAceProvider && !cred.ValidOfficeAce() {
			return nil, invalid("Saved OfficeAce sign-in requires STS credentials or MaaS app key pair")
		}
		storedBytes, err := json.Marshal(storedSavedFlow{Method: method, Provider: provider, Saved: cred})
		if err != nil {
			return nil, err
		}
		var cas sdk.StateCASResult
		if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{
			Key:     "auth." + flowID,
			Version: "0",
			Value:   storedBytes,
		}, &cas); err != nil {
			return nil, err
		}
		if !cas.Swapped {
			return nil, &sdk.Error{Code: "conflict", Message: "Auth flow collision"}
		}
		cont, _ := json.Marshal(authContinuation{Method: method, ID: flowID, Provider: provider})
		return sdk.AuthStartResult{PollAfterMs: 500, Continuation: cont}, nil
	}

	stationID := "mainland"
	if method == "oauth-intl" {
		if provider != CodeArtsProvider {
			return nil, invalid("International OAuth is only supported for CodeArts")
		}
		stationID = "international"
	} else if method != "oauth" {
		return nil, invalid("Unsupported sign-in method")
	}
	station := owner.stationOf(&CodeArtsCredential{Station: stationID})
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	jwk, err := GenerateDPoPKey()
	if err != nil {
		return nil, err
	}
	ln, port, err := listenCallbackPort()
	if err != nil {
		return nil, err
	}
	loginURL := BuildOAuthLoginURL(port, pkce, randomHex(32), station)
	pending := &oauthPending{station: station.ID, provider: provider}

	var proxy sdk.NetworkProxyResult
	_ = stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: owner.options.Endpoints.STSToken}, &proxy)

	mux := http.NewServeMux()
	var answered int
	srv := &http.Server{Handler: mux}
	pending.closeFn = func() { _ = srv.Close() }
	mux.HandleFunc(RedirectPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		pending.mu.Lock()
		if answered > 0 {
			pending.mu.Unlock()
			w.WriteHeader(http.StatusConflict)
			return
		}
		answered = 1
		pending.mu.Unlock()
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Redirect(w, r, buildPortalLoginResultURL(false, station), http.StatusTemporaryRedirect)
			said := firstOf(r.URL.Query().Get("error_description"), r.URL.Query().Get("error"), "missing authorization code")
			pending.mu.Lock()
			pending.done = true
			pending.err = errors.New("CodeArts sign-in failed: " + said)
			pending.mu.Unlock()
			go srv.Close()
			return
		}
		client, cleanup, hErr := owner.options.HTTPFactory(proxy.URL)
		if hErr != nil {
			http.Redirect(w, r, buildPortalLoginResultURL(false, station), http.StatusTemporaryRedirect)
			pending.mu.Lock()
			pending.done = true
			pending.err = hErr
			pending.mu.Unlock()
			go srv.Close()
			return
		}
		defer cleanup()
		dpop, dErr := SignDPoPJWS(jwk, http.MethodPost, owner.options.Endpoints.STSToken, owner.options.Now())
		if dErr != nil {
			http.Redirect(w, r, buildPortalLoginResultURL(false, station), http.StatusTemporaryRedirect)
			pending.mu.Lock()
			pending.done = true
			pending.err = dErr
			pending.mu.Unlock()
			go srv.Close()
			return
		}
		form := url.Values{
			"client_id":     {ClientID},
			"code":          {code},
			"code_verifier": {pkce.Verifier},
			"grant_type":    {GrantAuthorizationCode},
			"redirect_uri":  {fmt.Sprintf("http://127.0.0.1:%d%s", port, RedirectPath)},
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, owner.options.Endpoints.STSToken, strings.NewReader(form.Encode()))
		req.Header.Set("DPoP", dpop)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, rErr := client.Do(req)
		if rErr != nil {
			http.Redirect(w, r, buildPortalLoginResultURL(false, station), http.StatusTemporaryRedirect)
			pending.mu.Lock()
			pending.done = true
			pending.err = rErr
			pending.mu.Unlock()
			go srv.Close()
			return
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var tok map[string]any
		_ = json.Unmarshal(raw, &tok)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || tok == nil || tok["credentials"] == nil {
			http.Redirect(w, r, buildPortalLoginResultURL(false, station), http.StatusTemporaryRedirect)
			pending.mu.Lock()
			pending.done = true
			pending.err = fmt.Errorf("token exchange failed: HTTP %d", resp.StatusCode)
			pending.mu.Unlock()
			go srv.Close()
			return
		}
		cred := credentialFromTokenResponse(tok, pkce.Verifier, jwk)
		if station.ID != "mainland" {
			cred.Station = station.ID
		}
		id, name := AccountIdentityOf(cred)
		rawCred, _ := json.Marshal(cred)
		http.Redirect(w, r, buildPortalLoginResultURL(true, station), http.StatusTemporaryRedirect)
		pending.mu.Lock()
		pending.done = true
		pending.cred = rawCred
		pending.id = id
		pending.name = name
		pending.expires = cred.ExpiresMs(owner.options.Now())
		pending.mu.Unlock()
		go srv.Close()
	})
	go func() {
		_ = srv.Serve(ln)
	}()
	time.AfterFunc(3*time.Minute, func() {
		pending.mu.Lock()
		if !pending.done {
			pending.done = true
			pending.err = errors.New("CodeArts OAuth login timed out")
		}
		pending.mu.Unlock()
		_ = srv.Close()
	})

	owner.mu.Lock()
	owner.oauthFlow[flowID] = pending
	owner.mu.Unlock()

	cont, _ := json.Marshal(authContinuation{Method: method, ID: flowID, Provider: provider})
	return sdk.AuthStartResult{
		URL:          loginURL,
		PollAfterMs:  1500,
		Continuation: cont,
	}, nil
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AuthFinishRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid auth finish request")
	}
	var cont authContinuation
	if decodeObject(request.Continuation, &cont) != nil || cont.ID == "" {
		return nil, invalid("Missing authorization continuation")
	}
	if cont.Method == sdk.SavedSignInMethod {
		var val sdk.StateValue
		if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: "auth." + cont.ID}, &val); err != nil {
			return nil, err
		}
		var stored storedSavedFlow
		if !val.Found || json.Unmarshal(val.Value, &stored) != nil {
			return nil, invalid("Saved sign-in continuation expired or missing")
		}
		cred := stored.Saved
		if stored.Provider == OfficeAceProvider && cred.ValidSTS() && (cred.ModelAppKey == "" || cred.ModelAppSecret == "") {
			cred = owner.withOfficeAceMaas(ctx, stream, cred)
		}
		id, name := AccountIdentityOf(cred)
		rawCred, _ := json.Marshal(cred)
		return sdk.AuthResult{
			ID:          id,
			Name:        name,
			Credentials: rawCred,
			ExpiresAt:   cred.ExpiresMs(owner.options.Now()),
		}, nil
	}

	owner.mu.Lock()
	pending, exists := owner.oauthFlow[cont.ID]
	owner.mu.Unlock()
	if !exists {
		return nil, invalid("OAuth flow not found or expired")
	}
	pending.mu.Lock()
	done := pending.done
	err := pending.err
	rawCred := pending.cred
	id := pending.id
	name := pending.name
	expires := pending.expires
	provider := pending.provider
	pending.mu.Unlock()

	if !done {
		return sdk.AuthResult{Pending: true}, nil
	}
	owner.mu.Lock()
	delete(owner.oauthFlow, cont.ID)
	owner.mu.Unlock()
	if err != nil {
		return nil, &sdk.Error{Code: "auth_failed", Message: err.Error(), Outcome: "not-started"}
	}
	if provider == OfficeAceProvider {
		var cred CodeArtsCredential
		if json.Unmarshal(rawCred, &cred) == nil {
			cred = owner.withOfficeAceMaas(ctx, stream, cred)
			rawCred, _ = json.Marshal(cred)
		}
	}
	return sdk.AuthResult{
		ID:          id,
		Name:        name,
		Credentials: rawCred,
		ExpiresAt:   expires,
	}, nil
}

func (owner *module) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AuthRefreshRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid refresh request")
	}
	rawInput := call.Auth
	if !nonempty(rawInput) {
		rawInput = request.Credentials
	}
	var cred CodeArtsCredential
	if decodeObject(rawInput, &cred) != nil {
		return nil, denied("Selected credentials are invalid")
	}
	provider := firstOf(call.Scope.Provider, CodeArtsProvider)
	next, err := owner.refreshCredential(ctx, stream, provider, cred)
	if err != nil {
		return nil, err
	}
	id, name := AccountIdentityOf(next)
	rawNext, _ := json.Marshal(next)
	return sdk.AuthResult{
		ID:          id,
		Name:        name,
		Credentials: rawNext,
		ExpiresAt:   next.ExpiresMs(owner.options.Now()),
	}, nil
}

func maskPhone(raw string) string {
	digits := strings.TrimSpace(raw)
	if idx := strings.LastIndex(digits, "-"); idx >= 0 {
		digits = digits[idx+1:]
	}
	if len(digits) < 7 {
		return digits
	}
	return digits[:3] + "****" + digits[len(digits)-4:]
}

func (owner *module) fetchAccountPhone(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential) string {
	profile := ProfileOf(cred)
	userID := firstOf(strField(profile, "principal_id", "user_id"), cred.UserID)
	if userID == "" || !cred.ValidSTS() {
		return ""
	}
	nowMs := owner.options.Now().UnixMilli()
	owner.mu.Lock()
	if cached, ok := owner.phones[userID]; ok && nowMs-cached.at < 6*60*60*1000 {
		owner.mu.Unlock()
		return cached.phone
	}
	owner.mu.Unlock()

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	target := strings.TrimRight(owner.options.Endpoints.IAMHost, "/") + "/v3.0/OS-USER/users/" + url.PathEscape(userID)
	signed, err := SignRequestHuawei(cred.AccessKeyID, cred.SecretAccessKey, cred.SecurityToken, http.MethodGet, target, nil, nil, sdkDateStamp(owner.options.Now()))
	if err != nil {
		return ""
	}
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	for k, v := range signed {
		if !strings.EqualFold(k, "host") {
			req.Header.Set(k, v)
		}
	}
	var phone string
	if resp, err := owner.doHTTP(reqCtx, stream, req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
			var doc struct {
				User struct {
					Phone string `json:"phone"`
				} `json:"user"`
			}
			if json.Unmarshal(raw, &doc) == nil && strings.TrimSpace(doc.User.Phone) != "" {
				phone = maskPhone(doc.User.Phone)
			}
		}
	}
	owner.mu.Lock()
	owner.phones[userID] = phoneEntry{phone: phone, at: nowMs}
	owner.mu.Unlock()
	return phone
}

func bytesEqualTrimmed(a, b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
}
