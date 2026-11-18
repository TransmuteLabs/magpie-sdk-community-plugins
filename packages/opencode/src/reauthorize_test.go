package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSelectedReauthorizationRetainsInitialIdentityAcrossOrganizationChange(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	var identity atomic.Value
	identity.Store("same-user")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/console/auth/device/code":
			json.NewEncoder(w).Encode(map[string]any{"device_code": "device-private", "user_code": "SAFE",
				"verification_uri_complete": "/console/verify", "expires_in": 600, "interval": 1})
		case "/console/auth/device/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "PRIVATE-new-access", "refresh_token": "PRIVATE-new-refresh",
				"expires_in": 3600, "org_id": "new-org"})
		case "/console/api/user":
			json.NewEncoder(w).Encode(map[string]any{"id": identity.Load().(string), "email": "same@example.invalid"})
		case "/console/api/orgs":
			json.NewEncoder(w).Encode([]map[string]string{{"id": "new-org", "name": "New organization"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	host := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		Now:         func() time.Time { return time.Unix(0, clock.Load()) },
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), func() {}, nil }})
	before := Credentials{Access: "PRIVATE-old-access", Refresh: "PRIVATE-old-refresh", UserID: "same-user", Email: "same@example.invalid",
		InitialOrgID: "initial-org", OrgID: "initial-org", Expires: time.Now().Add(time.Hour)}
	scope := sdk.Scope{Provider: ProviderGo, Account: before.AccountID(ProviderGo), Generation: 7}
	start, _ := host.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "device"}, raw(before))
	if start.Error != nil {
		t.Fatal(start.Error.Message)
	}
	var flow sdk.AuthStartResult
	if json.Unmarshal(start.Result, &flow) != nil {
		t.Fatal("missing device flow")
	}
	clock.Add(int64(2 * time.Second))
	finish, _ := host.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: flow.Continuation}, raw(before))
	if finish.Error != nil {
		t.Fatal(finish.Error.Message)
	}
	var result sdk.AuthResult
	var after Credentials
	if json.Unmarshal(finish.Result, &result) != nil || json.Unmarshal(result.Credentials, &after) != nil {
		t.Fatal("missing reauthorization result")
	}
	if result.ID != scope.Account || after.InitialOrgID != before.InitialOrgID || after.OrgID != "new-org" {
		t.Fatalf("selected reauthorization changed account authority: old=%s returned=%s initial=%s current=%s",
			scope.Account, result.ID, after.InitialOrgID, after.OrgID)
	}
	identity.Store("different-user")
	start, _ = host.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "device"}, raw(before))
	if start.Error != nil || json.Unmarshal(start.Result, &flow) != nil {
		t.Fatal("foreign identity fixture did not start")
	}
	clock.Add(int64(2 * time.Second))
	finish, _ = host.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: flow.Continuation}, raw(before))
	if finish.Error == nil || finish.Error.Outcome != "uncertain" || len(finish.Result) != 0 {
		t.Fatal("selected reauthorization returned another user's credentials")
	}
}
