package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAutoStartWithoutLinkedPaidMaterialKeepsSelectedFamily(t *testing.T) {
	for _, site := range []string{"bigmodel", "zai"} {
		t.Run(site, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/v1/zcode-plan/billing/balance" ||
					request.Header.Get("Authorization") != "Bearer "+fixtureJWT(fixtureNow.Add(time.Hour)) {
					t.Error("unlinked Start used a paid endpoint or another credential channel")
					writer.WriteHeader(http.StatusForbidden)
					return
				}
				reads.Add(1)
				if request.URL.Query().Get("app_version") != "3.18.2" {
					t.Error("Start request did not advance to the tested vendor baseline")
				}
				io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`)
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			selected := sdk.Scope{Provider: ProviderID}
			flow := readyStartFlow("unlinked-user")
			flow.Selection.Site, flow.Selection.Kind = site, "auto"
			var receipt map[string]any
			json.Unmarshal(flow.Ready, &receipt)
			other := "bigmodel"
			if site == other {
				other = "zai"
			}
			receipt[other] = map[string]string{"access_token": "foreign-paid-material"}
			flow.Ready = raw(receipt)
			finish := seedFlow(h, selected, flow)
			frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
			var result sdk.AuthResult
			var account Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
				json.Unmarshal(result.Credentials, &account) != nil {
				t.Fatalf("valid unlinked Start rejected: %+v", frame.Error)
			}
			if account.Site != site || account.Kind != "start" || account.UserID != "unlinked-user" ||
				account.SessionToken != fixtureJWT(fixtureNow.Add(time.Hour)) || account.ExpiresAt != fixtureNow.Add(time.Hour).UnixMilli() ||
				account.OAuthToken != "" || account.BusinessToken != "" || account.ProjectToken != "" || !account.Discovery {
				t.Fatal("unlinked Start invented or borrowed paid material/identity")
			}
			again, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
			if again.Error != nil || string(again.Result) != string(frame.Result) || reads.Load() != 1 {
				t.Fatal("completed unlinked Start flow was probed or exchanged again")
			}
		})
	}
}

func TestChinaLoginChannelPersistsAcrossSelectedReauthorization(t *testing.T) {
	account := fixtureCredentials("start")
	account.Site, account.LoginProvider = "bigmodel", "zai_cn"
	priorID := account.AccountID()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input map[string]string
		json.NewDecoder(request.Body).Decode(&input)
		if request.URL.Path != "/api/v1/oauth/cli/init" || input["provider"] != "zai_cn" {
			t.Error("selected China login silently changed authorization channel")
		}
		json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
			"flow_id": "cn", "authorize_url": "https://zcode.z.ai/authorize?state=cn",
			"poll_interval_sec": 1, "expires_at": fixtureNow.Add(5 * time.Minute).Unix(),
		}})
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	frame, _ := h.call(sdk.MethodAuthStart, scope(account), sdk.AuthStartRequest{Method: "browser"}, raw(account))
	if frame.Error != nil {
		t.Fatal("selected China login cannot renew:", frame.Error)
	}
	account.LoginProvider = ""
	if priorID != account.AccountID() {
		t.Fatal("login channel changed existing stable vendor identity")
	}
	account.Site, account.LoginProvider = "zai", "zai_cn"
	if account.validateStructure() == nil {
		t.Fatal("cross-family login channel was accepted in a credential snapshot")
	}
}

func TestNullLinkedPaidMaterialDoesNotGrantPaidAccess(t *testing.T) {
	for _, kind := range []string{"auto", "individual"} {
		t.Run(kind, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/v1/zcode-plan/billing/balance" {
					t.Error("null linked material reached paid exchange")
				}
				reads.Add(1)
				io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`)
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			selected := sdk.Scope{Provider: ProviderID}
			flow := readyStartFlow("china-user")
			flow.Selection = authSelection{Site: "bigmodel", Kind: kind, LoginProvider: "zai_cn"}
			var receipt map[string]any
			json.Unmarshal(flow.Ready, &receipt)
			receipt["bigmodel"] = nil
			receipt["zai"] = map[string]string{"access_token": "China-bootstrap-not-paid"}
			flow.Ready = raw(receipt)
			frame, _ := h.call(sdk.MethodAuthFinish, selected, seedFlow(h, selected, flow), nil)
			if kind == "individual" {
				if frame.Error == nil || reads.Load() != 0 {
					t.Fatal("null linked material was treated as a paid account")
				}
				return
			}
			var result sdk.AuthResult
			var account Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
				json.Unmarshal(result.Credentials, &account) != nil || account.Kind != "start" ||
				account.Site != "bigmodel" || account.LoginProvider != "zai_cn" || account.OAuthToken != "" || reads.Load() != 1 {
				t.Fatalf("null linked material blocked or mixed confirmed Start: %+v", frame.Error)
			}
		})
	}
}

func TestAutoStartRejectsMalformedPresentPaidMaterialBeforeProbing(t *testing.T) {
	for _, material := range []any{"invalid", map[string]any{"access_token": 1}, map[string]any{"access_token": "bad\nvalue"}} {
		t.Run(string(raw(material)), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				t.Error("malformed paid material was treated as absent")
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			selected := sdk.Scope{Provider: ProviderID}
			flow := readyStartFlow("unlinked-user")
			flow.Selection.Kind = "auto"
			var receipt map[string]any
			json.Unmarshal(flow.Ready, &receipt)
			receipt["zai"] = material
			flow.Ready = raw(receipt)
			finish := seedFlow(h, selected, flow)
			frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
			if frame.Error == nil {
				t.Fatal("malformed selected paid material published a Start credential")
			}
		})
	}
}

func TestMissingPaidMaterialCannotInventEntitlementOrChangeSelectedPlan(t *testing.T) {
	for _, test := range []struct {
		name, kind, body string
		wantReads        int32
		selectedPaid     bool
	}{
		{"unknown", "auto", `{"code":0,"data":{"plans":[],"balances":[]}}`, 1, false},
		{"malformed", "auto", `{"code":0,"data":{"plans":"unknown","balances":[]}}`, 1, false},
		{"explicit-paid", "individual", ``, 0, false},
		{"selected-paid", "auto", `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`, 1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/v1/zcode-plan/billing/balance" {
					t.Error("missing selected material reached a paid endpoint")
				}
				reads.Add(1)
				io.WriteString(writer, test.body)
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			selected := sdk.Scope{Provider: ProviderID}
			flow := readyStartFlow("unlinked-user")
			flow.Selection.Kind = test.kind
			var credentials json.RawMessage
			if test.selectedPaid {
				account := fixtureCredentials("individual")
				account.UserID = "unlinked-user"
				selected, credentials = scope(account), raw(account)
				flow.TargetAccount, flow.TargetUser, flow.TargetGeneration = account.AccountID(), account.UserID, selected.Generation
			}
			finish := seedFlow(h, selected, flow)
			frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, credentials)
			if frame.Error == nil || reads.Load() != test.wantReads {
				t.Fatalf("missing/unknown paid identity admitted or skipped confirmation: error=%+v reads=%d", frame.Error, reads.Load())
			}
			if test.selectedPaid && frame.Error.Code != "permission_denied" {
				t.Fatal("selected paid account changed silently into Start")
			}
		})
	}
}

func TestChinaLoginProviderIsExplicitAndBoundToBigModel(t *testing.T) {
	for _, test := range []struct {
		site, provider string
		allowed        bool
	}{
		{"bigmodel", "zai_cn", true},
		{"bigmodel", "", true},
		{"zai", "", true},
		{"zai", "zai_cn", false},
		{"bigmodel", "zai", false},
		{"bigmodel", "arbitrary", false},
	} {
		t.Run(test.site+"/"+test.provider, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				var input map[string]string
				json.NewDecoder(request.Body).Decode(&input)
				expected := test.provider
				if expected == "" {
					expected = test.site
				}
				if request.URL.Path != "/api/v1/oauth/cli/init" || input["provider"] != expected || len(input) != 1 {
					t.Error("login channel was confused with paid-family identity")
				}
				json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
					"flow_id": "cn", "authorize_url": "https://zcode.z.ai/authorize?state=cn",
					"poll_interval_sec": 1, "expires_at": fixtureNow.Add(5 * time.Minute).Unix(),
				}})
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			frame, _ := h.call(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderID},
				sdk.AuthStartRequest{Method: "browser", Input: raw(map[string]string{
					"site": test.site, "kind": "auto", "loginProvider": test.provider,
				})}, nil)
			if (frame.Error == nil) != test.allowed || !test.allowed && calls.Load() != 0 || test.allowed && calls.Load() != 1 {
				t.Fatalf("login channel validation: error=%+v calls=%d", frame.Error, calls.Load())
			}
		})
	}
}
