package opencode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestRefreshHandlerPreflightNeverDispatchesSubstitutedCredentials(t *testing.T) {
	owner := &module{}
	account := credential(time.Now(), "a", "org-a")
	validScope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
	for _, test := range []struct {
		name   string
		scope  sdk.Scope
		params json.RawMessage
		auth   json.RawMessage
	}{
		{"null-params", validScope, json.RawMessage(`null`), raw(account)},
		{"array-params", validScope, json.RawMessage(`[]`), raw(account)},
		{"unknown-params", validScope, json.RawMessage(`{"unknown":true}`), raw(account)},
		{"foreign-credentials", validScope, raw(sdk.AuthRefreshRequest{Credentials: raw(credential(time.Now(), "b", "org-b"))}), raw(account)},
		{"same-credentials-in-params", validScope, raw(sdk.AuthRefreshRequest{Credentials: raw(account)}), raw(account)},
		{"missing-auth", validScope, json.RawMessage(`{}`), nil},
		{"foreign-auth", validScope, json.RawMessage(`{}`), raw(credential(time.Now(), "b", "org-b"))},
		{"missing-generation", sdk.Scope{Provider: ProviderGo, Account: validScope.Account}, json.RawMessage(`{}`), raw(account)},
		{"foreign-provider", sdk.Scope{Provider: ProviderZen, Account: validScope.Account, Generation: 1}, json.RawMessage(`{}`), raw(account)},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := owner.authRefresh(t.Context(), sdk.Call{Scope: test.scope, Params: test.params, Auth: test.auth}, nil)
			var rpc *sdk.Error
			if result != nil || !errors.As(err, &rpc) || rpc.Outcome != "not-started" {
				t.Fatalf("refresh preflight reached dispatch or lost its safe outcome: %v", err)
			}
		})
	}
}

func TestPublicSDKRefreshRotatesOnlyTheSelectedAccount(t *testing.T) {
	now := time.Now()
	var exchanges atomic.Int32
	var chat atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveFixtureGoCatalog(w, r) {
			return
		}
		switch r.URL.Path {
		case "/console/auth/device/token":
			exchanges.Add(1)
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			user := strings.TrimPrefix(request["refresh_token"], "refresh-")
			if request["grant_type"] != "refresh_token" || request["client_id"] != ClientID ||
				user != "a" && user != "b" || r.Header.Get("Authorization") != "" {
				t.Error("refresh substituted account authority")
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "rotated-access-" + user,
				"refresh_token": "rotated-refresh-" + user, "expires_in": 3600, "org_id": "new-org-" + user})
		case "/inference/go/openai/v1/chat/completions":
			chat.Add(1)
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer rotated-access-") ||
				!strings.HasPrefix(r.Header.Get("X-Opencode-Org-Id"), "new-org-") {
				t.Error("chat did not use the published replacement")
			}
			io.WriteString(w, `{"choices":[]}`)
		default:
			t.Error("refresh fetched an unrelated identity or operation")
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		Now: func() time.Time { return now }, HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	for _, user := range []string{"a", "b"} {
		old := credential(now, user, "org-"+user)
		old.Expires = now.Add(-time.Hour)
		scope := sdk.Scope{Provider: ProviderGo, Account: old.AccountID(ProviderGo), Generation: 7}
		frame, _ := h.invoke(sdk.MethodAuthRefresh, scope, sdk.AuthRefreshRequest{}, raw(old))
		var result sdk.AuthResult
		var next Credentials
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
			json.Unmarshal(result.Credentials, &next) != nil {
			t.Fatalf("native unary refresh failed: %v", frame.Error)
		}
		if result.ID != scope.Account || result.Pending || result.ExpiresAt != now.Add(time.Hour).UnixMilli() ||
			next.Access != "rotated-access-"+user || next.Refresh != "rotated-refresh-"+user ||
			next.OrgID != "new-org-"+user || next.InitialOrgID != old.InitialOrgID ||
			next.AccountID(ProviderGo) != old.AccountID(ProviderGo) || next.Email != old.Email || len(h.state) != 0 {
			t.Fatal("refresh altered identity, crossed accounts or used private state as a token lock")
		}
		request := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
			Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`)}
		due, _ := h.invoke(sdk.MethodProviderExecute, scope, request, raw(old))
		if due.Error == nil || due.Error.Code != "auth_refresh_required" || due.Error.Outcome != "not-started" {
			t.Fatal("old credentials were dispatched after rotation")
		}
		scope.Generation++
		executed, _ := h.invoke(sdk.MethodProviderExecute, scope, request, result.Credentials)
		if executed.Error != nil {
			t.Fatalf("replacement credentials failed: %v", executed.Error)
		}
	}
	if exchanges.Load() != 2 || chat.Load() != 2 || len(h.proxies) != 6 {
		t.Fatal("module refreshed implicitly, retried rotation or bypassed proxy authority")
	}
}

func TestPublicSDKRefreshRefusalRemainsUncertainWithoutTokenDetails(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		code   string
	}{
		{"revoked", 400, `{"error":"invalid_grant","error_description":"PRIVATE-REFRESH-SENTINEL"}`, "auth_revoked"},
		{"transient", 500, `{"error":"invalid_grant","error_description":"PRIVATE-REFRESH-SENTINEL"}`, "upstream"},
		{"invalid-result", 200, `{"access_token":"PRIVATE-REFRESH-SENTINEL","refresh_token":"","expires_in":3600}`, "upstream"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			}))
			t.Cleanup(server.Close)
			h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
			account := credential(time.Now(), "a", "org-a")
			frame, _ := h.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1},
				sdk.AuthRefreshRequest{}, raw(account))
			if frame.Error == nil || frame.Error.Code != test.code || frame.Error.Outcome != "uncertain" ||
				calls.Load() != 1 || len(frame.Result) != 0 && string(frame.Result) != "null" ||
				strings.Contains(frame.Error.Message, "PRIVATE") || len(h.state) != 0 {
				t.Fatalf("refresh refusal was replayable or disclosed private credentials: %v", frame.Error)
			}
		})
	}
}
