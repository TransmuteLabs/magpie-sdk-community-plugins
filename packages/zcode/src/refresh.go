package zcode

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func refreshFailure() error {
	return &sdk.Error{Code: "upstream", Message: "ZCode PAT exchange did not yield publishable credentials; no retry was made", Outcome: "uncertain"}
}

func parsePAT(body []byte, started, now time.Time) (string, int64, error) {
	reply, err := protocol.ReadObject(body)
	if err != nil || reply["success"] == false {
		return "", 0, refreshFailure()
	}
	code := ""
	switch value := reply["code"].(type) {
	case json.Number:
		code = string(value)
	case string:
		code = value
	}
	if code != "0" && code != "200" {
		return "", 0, refreshFailure()
	}
	data, _ := reply["data"].(map[string]any)
	if enabled, exists := data["enable"]; exists && enabled != true {
		return "", 0, refreshFailure()
	}
	token, _ := data["accessToken"].(string)
	ttl, validTTL := finiteNumber(data["expiresIn"])
	absolute, validAbsolute := finiteNumber(data["expiresAt"])
	start := started.UnixMilli()
	if !safeText(token) || data["tokenType"] != "Bearer" || !validTTL || !validAbsolute ||
		ttl <= 0 || absolute <= 0 || start < 0 || ttl*1000 >= float64(math.MaxInt64-start) ||
		absolute*1000 >= float64(math.MaxInt64) {
		return "", 0, refreshFailure()
	}
	expiresAt := min(int64(absolute*1000), start+int64(ttl*1000))
	if expiresAt <= now.Add(30*time.Second).UnixMilli() {
		return "", 0, refreshFailure()
	}
	return token, expiresAt, nil
}

func (owner *module) refresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	var request sdk.AuthRefreshRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if len(request.Credentials) != 0 && !bytes.Equal(bytes.TrimSpace(request.Credentials), []byte("null")) {
		return nil, invalid("Refresh credentials must come from the host-owned transaction")
	}
	if account.Kind != "individual" && account.Kind != "team" {
		return nil, &sdk.Error{Code: "reauthorization_required", Message: "This vendor does not expose Start-session silent renewal; use selected-account browser auth.start/auth.finish to renew without changing identity", Outcome: "not-started"}
	}
	if account.Location == nil {
		return nil, &sdk.Error{Code: "credentials_required", Message: "PAT renewal requires a bound saved project/key location", Outcome: "not-started"}
	}
	if call.Scope.Operation == "" || call.Scope.Session != "" {
		return nil, blocked("PAT renewal requires the host-owned API 1.3 refresh transaction")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	businessChanged := false
	if !safeText(account.BusinessToken) || account.BusinessExpiresAt <= owner.options.Now().Add(30*time.Second).UnixMilli() {
		if account.Site != "zai" || !safeText(account.OAuthToken) || account.OAuthExpiresAt <= owner.options.Now().Add(30*time.Second).UnixMilli() {
			return nil, &sdk.Error{Code: "credentials_required", Message: "Renewal requires a valid business credential or selected Z.ai OAuth bootstrap", Outcome: "not-started"}
		}
		account, err = owner.refreshBusiness(ctx, stream, account)
		if err != nil {
			return nil, err
		}
		businessChanged = true
	}
	started := owner.options.Now()
	base, _ := url.Parse(owner.options.Endpoints.quotaForAccount(account))
	location := account.Location
	endpoint := base.Scheme + "://" + base.Host + "/api/biz/v1/organization/" +
		url.PathEscape(location.OrganizationID) + "/projects/" + url.PathEscape(location.ProjectID) +
		"/api_keys/" + url.PathEscape(location.APIKeyID) + "/access_tokens"
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		if businessChanged {
			return nil, refreshFailure()
		}
		return nil, &sdk.Error{Code: "not_ready", Message: "The host could not resolve the selected refresh route", Outcome: "not-started"}
	}
	body := []byte(`{"clientType":"zcode","clientVersion":"` + ClientVersion + `"}`)
	outbound, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		if businessChanged {
			return nil, refreshFailure()
		}
		return nil, invalid("Invalid ZCode PAT endpoint")
	}
	outbound.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(account.BusinessToken, "Bearer "))
	outbound.Header.Set("Content-Type", "application/json")
	response, err := send(ctx, owner.options.HTTPFactory, proxy.URL, outbound)
	if err != nil {
		return nil, refreshFailure()
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, refreshFailure()
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(reply) > 1<<20 {
		return nil, refreshFailure()
	}
	token, expiresAt, err := parsePAT(reply, started, owner.options.Now())
	if err != nil {
		return nil, err
	}
	account.ProjectToken, account.ExpiresAt = token, expiresAt
	credentials, err := json.Marshal(account)
	if err != nil {
		return nil, refreshFailure()
	}
	return sdk.AuthResult{ID: account.AccountID(), Name: "ZCode " + account.Site + " " + account.Kind,
		Credentials: credentials, ExpiresAt: expiresAt}, nil
}

func businessReply(body []byte, started, now time.Time) (string, int64, error) {
	root, err := protocol.ReadObject(body)
	if err != nil || root["success"] == false {
		return "", 0, refreshFailure()
	}
	if code, present := root["code"]; present {
		text := ""
		switch value := code.(type) {
		case json.Number:
			text = string(value)
		case string:
			text = value
		}
		if text != "0" && text != "200" {
			return "", 0, refreshFailure()
		}
	}
	data, _ := root["data"].(map[string]any)
	token, _ := data["access_token"].(string)
	if token == "" {
		token, _ = data["accessToken"].(string)
	}
	if !safeText(token) {
		return "", 0, refreshFailure()
	}
	deadline := int64(math.MaxInt64)
	ttl := data["expires_in"]
	if ttl == nil {
		ttl = data["expiresIn"]
	}
	if ttl != nil {
		seconds, valid := finiteNumber(ttl)
		if !valid || seconds <= 0 || seconds*1000 >= float64(math.MaxInt64-started.UnixMilli()) {
			return "", 0, refreshFailure()
		}
		deadline = started.UnixMilli() + int64(seconds*1000)
	}
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		payload, decodeErr := base64.RawURLEncoding.DecodeString(parts[1])
		if decodeErr == nil {
			claims, parseErr := protocol.ReadObject(payload)
			if parseErr == nil {
				if seconds, valid := finiteNumber(claims["exp"]); valid && seconds > 0 && seconds*1000 < float64(math.MaxInt64) {
					deadline = min(deadline, int64(seconds*1000))
				}
			}
		}
	}
	if deadline == math.MaxInt64 || deadline <= now.Add(30*time.Second).UnixMilli() {
		return "", 0, refreshFailure()
	}
	return token, deadline, nil
}

func (owner *module) refreshBusiness(ctx context.Context, stream *sdk.Stream, account Credentials) (Credentials, error) {
	base, _ := url.Parse(owner.options.Endpoints.ZaiQuota)
	endpoint := base.Scheme + "://" + base.Host + "/api/auth/z/login"
	started := owner.options.Now()
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return account, &sdk.Error{Code: "not_ready", Message: "The host could not resolve the selected business refresh route", Outcome: "not-started"}
	}
	body, _ := json.Marshal(map[string]string{"token": account.OAuthToken})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return account, invalid("Invalid Z.ai business renewal endpoint")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "ZCode/"+ClientVersion)
	response, err := send(ctx, owner.options.HTTPFactory, proxy.URL, request)
	if err != nil {
		return account, refreshFailure()
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return account, refreshFailure()
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(reply) > 1<<20 {
		return account, refreshFailure()
	}
	token, deadline, err := businessReply(reply, started, owner.options.Now())
	if err != nil {
		return account, err
	}
	account.BusinessToken, account.BusinessExpiresAt = token, deadline
	return account, nil
}
