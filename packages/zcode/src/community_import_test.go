package zcode

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSelectedCommunitySnapshot(t *testing.T) {
	expiry := fixtureNow.Add(time.Hour)
	token := fixtureJWT(expiry)
	state := map[string]any{"site": "zai", "base": "https://api.z.ai/api/anthropic", "device": "fixture-device", "jwt": token, "plan": "Start"}
	record := map[string]any{"type": "oauth", "access": token, "expires": expiry.UnixMilli(), "uid": "fixture-user", "accountId": "name@example.invalid"}
	check := func(ok bool) Credentials {
		t.Helper()
		record["refresh"] = string(raw(state))
		value, err := communitySnapshot(raw(map[string]any{"community": record}), fixtureNow)
		if (err == nil) != ok {
			t.Fatalf("unexpected import acceptance %v", err)
		}
		return value
	}
	value := check(true)
	if value.AccountID() != fixtureCredentials("start").AccountID() || value.DisplayName != "name@example.invalid" {
		t.Fatal("Start identity or display changed")
	}
	for _, key := range []string{"source", "key", "org", "project"} {
		state[key] = "other"
		check(false)
		delete(state, key)
	}
	record["uid"] = ""
	check(false)
	record["uid"] = "fixture-user"
	record["expires"] = expiry.Add(time.Hour).UnixMilli()
	check(false)
	api, err := communitySnapshot(raw(map[string]any{"community": map[string]any{"type": "api", "key": "fixture-key", "metadata": map[string]string{"site": "zai"}}}), fixtureNow)
	if err != nil || api.AccountID() != fixtureCredentials("legacy-key").AccountID() {
		t.Fatal("API key identity changed")
	}
}

func TestSelectedCommunityImportPublishesOnlyNativeSnapshot(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	input := raw(map[string]any{"community": map[string]any{"type": "api", "key": "fixture-key"}})
	account := fixtureCredentials("legacy-key")
	frame, _ := h.call(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderID}, sdk.AuthFinishRequest{Input: input}, nil)
	var result sdk.AuthResult
	var stored Credentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || decode(result.Credentials, &stored) != nil || stored.Kind != "legacy-key" || result.ID != account.AccountID() {
		t.Fatal("community record was not published as native credentials")
	}
	other := scope(fixtureCredentials("start"))
	frame, _ = h.call(sdk.MethodAuthFinish, other, sdk.AuthFinishRequest{Input: input}, nil)
	if frame.Error == nil || h.proxies != 0 {
		t.Fatal("migration crossed selected identity or used the network")
	}
}

func TestCommunityDesktopFollowRequiresReauthorization(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	token := fixtureJWT(fixtureNow.Add(time.Hour))
	state := map[string]string{
		"site": "zai", "base": "https://api.z.ai/api/anthropic", "device": "fixture-device",
		"jwt": token, "key": "fixture.plan-key", "source": "zcode",
	}
	record := map[string]any{
		"type": "oauth", "refresh": string(raw(state)), "access": state["key"], "expires": 0,
		"accountId": "fixture@example.invalid", "uid": "fixture-user",
	}
	frame, _ := h.call(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderID},
		sdk.AuthFinishRequest{Input: raw(map[string]any{"community": record})}, nil)
	if frame.Error == nil || frame.Error.Outcome != "not-started" || len(frame.Result) != 0 || h.proxies != 0 {
		t.Fatal("desktop-follow import returned credentials or attempted an exchange")
	}
}

func savedSignInRequest(record map[string]any) sdk.AuthStartRequest {
	return sdk.AuthStartRequest{Method: sdk.SavedSignInMethod, Input: raw(map[string]string{"record": string(raw(record))})}
}

func TestSavedSignInConsumesOnlyTheCurrentSnapshot(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	account := fixtureCredentials("legacy-key")
	selected := scope(account)
	request := savedSignInRequest(map[string]any{"type": "api", "key": account.Key, "metadata": map[string]string{"site": account.Site}})
	start := func() sdk.AuthStartResult {
		t.Helper()
		frame, _ := h.call(sdk.MethodAuthStart, selected, request, raw(account))
		var result sdk.AuthStartResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Continuation) == 0 ||
			bytes.Contains(frame.Result, []byte(account.Key)) || result.URL != "" || result.UserCode != "" {
			t.Fatalf("saved sign-in did not return a private continuation: %+v", frame.Error)
		}
		return result
	}
	stale, current := start(), start()
	if len(h.state) != 1 || bytes.Equal(stale.Continuation, current.Continuation) {
		t.Fatal("the new explicit selection did not supersede the previous continuation")
	}
	finish := sdk.AuthFinishRequest{Continuation: current.Continuation}
	wrongGeneration := selected
	wrongGeneration.Generation++
	frame, _ := h.call(sdk.MethodAuthFinish, wrongGeneration, finish, raw(account))
	if frame.Error == nil {
		t.Fatal("a changed account generation completed the import")
	}
	frame, _ = h.call(sdk.MethodAuthFinish, selected, sdk.AuthFinishRequest{Continuation: stale.Continuation}, raw(account))
	if frame.Error == nil {
		t.Fatal("a superseded continuation completed the import")
	}
	frame, _ = h.call(sdk.MethodAuthFinish, selected, sdk.AuthFinishRequest{Continuation: current.Continuation, Input: request.Input}, raw(account))
	if frame.Error == nil {
		t.Fatal("finish accepted replacement input")
	}
	frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, raw(account))
	var result sdk.AuthResult
	var credentials Credentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.ID != account.AccountID() ||
		decode(result.Credentials, &credentials) != nil || credentials.Key != account.Key || credentials.Site != account.Site {
		t.Fatalf("saved API-key import changed the selected account: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, raw(account))
	if frame.Error == nil || h.proxies != 0 {
		t.Fatal("consumed import returned credentials or used the network")
	}
	for _, value := range h.state {
		if bytes.Contains(value.Value, []byte(account.Key)) {
			t.Fatal("the finished continuation retained a copy of the key")
		}
	}
}

func TestSavedSignInRejectsEvenAnUnexpiredDesktopFollow(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	expires := fixtureNow.Add(time.Hour)
	token := fixtureJWT(expires)
	record := map[string]any{"type": "oauth", "access": token, "expires": expires.UnixMilli(), "uid": "fixture-user",
		"refresh": string(raw(map[string]string{"source": "zcode", "site": "zai", "base": "https://api.z.ai/api/anthropic",
			"device": "fixture-device", "jwt": token, "plan": "Start"}))}
	frame, _ := h.call(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderID}, savedSignInRequest(record), nil)
	if frame.Error == nil || frame.Error.Code != "reauthorization_required" || frame.Error.Outcome != "not-started" ||
		len(frame.Result) != 0 || len(h.state) != 0 || h.proxies != 0 {
		t.Fatal("Desktop-follow was not declined before storage or vendor access")
	}
}

func TestSavedSignInDoesNotPublishAfterExpiryOrLostCAS(t *testing.T) {
	var clock atomic.Int64
	clock.Store(fixtureNow.UnixMilli())
	h := newHarness(t, ModuleOptions{Now: func() time.Time { return time.UnixMilli(clock.Load()) }})
	selected := sdk.Scope{Provider: ProviderID}
	request := savedSignInRequest(map[string]any{"type": "api", "key": "fixture-expiring-import"})
	frame, _ := h.call(sdk.MethodAuthStart, selected, request, nil)
	var started sdk.AuthStartResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &started) != nil {
		t.Fatalf("snapshot start: %+v", frame.Error)
	}
	finish := sdk.AuthFinishRequest{Continuation: started.Continuation}
	h.rejectCAS = h.casCalls + 1
	frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, nil)
	if frame.Error == nil || frame.Error.Code != "conflict" || len(frame.Result) != 0 {
		t.Fatal("snapshot import published without owning its continuation")
	}
	clock.Add((5 * time.Minute).Milliseconds())
	frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, nil)
	if frame.Error == nil || len(frame.Result) != 0 || h.proxies != 0 {
		t.Fatal("an expired import published credentials or used the network")
	}
	for _, value := range h.state {
		if bytes.Contains(value.Value, []byte("fixture-expiring-import")) {
			t.Fatal("the expired continuation retained a copy of the key")
		}
	}
}

func TestSavedSignInVerifiesStartIdentityAndEntitlement(t *testing.T) {
	for _, scenario := range []struct {
		name, user, plan string
		firstOnly        bool
	}{
		{name: "same-user", user: "fixture-user", plan: "active"},
		{name: "another-user", user: "other-user", plan: "active", firstOnly: true},
		{name: "inactive-plan", user: "fixture-user", plan: "expired"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			expires := fixtureNow.Add(time.Hour)
			token := fixtureJWT(expires)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer "+token {
					t.Error("saved sign-in did not use the selected session for a read")
				}
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					_, _ = writer.Write(raw(map[string]any{"code": 0, "data": map[string]any{
						"user": map[string]string{"user_id": scenario.user},
					}}))
				case "/api/v1/zcode-plan/billing/balance":
					if scenario.firstOnly {
						t.Error("mismatched UID reached the entitlement check")
					}
					_, _ = writer.Write(raw(map[string]any{"code": 0, "data": map[string]any{
						"plans": []map[string]string{{"plan_id": "start", "user_plan_id": "owned", "status": scenario.plan}}, "balances": []any{},
					}}))
				default:
					t.Error("unexpected saved sign-in vendor operation")
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			selected := sdk.Scope{Provider: ProviderID}
			record := map[string]any{"type": "oauth", "access": token, "expires": expires.UnixMilli(), "uid": "fixture-user",
				"refresh": string(raw(map[string]string{"site": "zai", "base": "https://api.z.ai/api/anthropic",
					"device": "fixture-device", "jwt": token, "plan": "Start"}))}
			frame, _ := h.call(sdk.MethodAuthStart, selected, savedSignInRequest(record), nil)
			var started sdk.AuthStartResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &started) != nil || h.proxies != 0 {
				t.Fatalf("snapshot start: %+v", frame.Error)
			}
			frame, _ = h.call(sdk.MethodAuthFinish, selected, sdk.AuthFinishRequest{Continuation: started.Continuation}, nil)
			var result sdk.AuthResult
			if scenario.name == "same-user" {
				if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
					result.ID != fixtureCredentials("start").AccountID() || result.ExpiresAt != expires.UnixMilli() {
					t.Fatalf("saved Start identity or expiry changed: %+v", frame.Error)
				}
			} else if frame.Error == nil || len(frame.Result) != 0 {
				t.Fatal("saved Start import published credentials without verification")
			}
			wantProxies := 2
			if scenario.firstOnly {
				wantProxies = 1
			}
			if h.proxies != wantProxies {
				t.Fatalf("saved Start import used %d vendor calls, want %d", h.proxies, wantProxies)
			}
			for _, value := range h.state {
				if bytes.Contains(value.Value, []byte(token)) {
					t.Fatal("the consumed continuation retained the session")
				}
			}
		})
	}
}
