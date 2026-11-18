package zcode

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func fixtureJWT(expires time.Time) string {
	payload := raw(map[string]any{"exp": expires.Unix()})
	return "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".fixture"
}

func TestHeadlessStartFlowHonorsIntervalAndRedeliversWithoutExchange(t *testing.T) {
	var polls atomic.Int32
	var clock atomic.Int64
	clock.Store(fixtureNow.UnixMilli())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Date", fixtureNow.Format(http.TimeFormat))
		switch request.URL.Path {
		case "/api/v1/oauth/cli/init":
			if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
				t.Error("init did not use a separate opaque polling credential")
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"flow_id": "flow", "authorize_url": "https://chat.z.ai/authorize?state=fixture-state",
				"poll_interval_sec": 1, "expires_at": fixtureNow.Add(5 * time.Minute).Unix(),
			}})
		case "/api/v1/oauth/cli/poll/flow":
			polls.Add(1)
			data := map[string]any{"status": "pending"}
			if polls.Load() > 1 {
				data = map[string]any{"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
					"user": map[string]string{"user_id": "headless-user"}, "zai": map[string]string{"access_token": "not-needed-for-start"}}
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": data})
		case "/api/v1/zcode-plan/billing/balance":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"plans": []any{map[string]any{"plan_id": "start", "user_plan_id": "owned", "status": "active",
					"ends_at": fixtureNow.Add(time.Hour).Unix()}}, "balances": []any{},
			}})
		default:
			t.Error("Start login attempted paid issuance or unknown endpoint")
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := endpointOptions(server)
	options.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
	h := newHarness(t, options)
	selected := sdk.Scope{Provider: ProviderID}
	frame, _ := h.call(sdk.MethodAuthStart, selected,
		sdk.AuthStartRequest{Method: "browser", Input: raw(authSelection{Site: "zai", Kind: "start"})}, nil)
	var started sdk.AuthStartResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &started) != nil || started.URL == "" || started.PollAfterMs != 1000 {
		t.Fatalf("headless init: %+v", frame.Error)
	}
	if strings.Contains(string(started.Continuation), "pollToken") || strings.Contains(string(started.Continuation), "fixture-state") {
		t.Fatal("public continuation exposed polling authority")
	}
	finish := sdk.AuthFinishRequest{Continuation: started.Continuation}
	for range 2 {
		frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, nil)
		var pending sdk.AuthResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &pending) != nil || !pending.Pending {
			t.Fatalf("pending flow: %+v", frame.Error)
		}
	}
	if polls.Load() != 1 {
		t.Fatal("finish ignored the server poll interval")
	}
	clock.Add(1000)
	frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, nil)
	var completed sdk.AuthResult
	var account Credentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &completed) != nil ||
		json.Unmarshal(completed.Credentials, &account) != nil || account.Kind != "start" || !account.Discovery {
		t.Fatalf("ready Start flow: %+v", frame.Error)
	}
	if account.UserID != "headless-user" || account.ProjectToken != "" || account.BusinessToken != "" {
		t.Fatal("Start login mixed credential families")
	}
	again, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
	if again.Error != nil || string(again.Result) != string(frame.Result) || polls.Load() != 2 {
		t.Fatal("completed flow was polled/exchanged again instead of redelivering its receipt")
	}
}

func TestSelectedStartRenewalRejectsAnotherVendorUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/cli/init":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"flow_id": "flow", "authorize_url": "https://chat.z.ai/authorize?state=fixture",
				"poll_interval_sec": 1, "expires_at": fixtureNow.Add(5 * time.Minute).Unix(),
			}})
		case "/api/v1/oauth/cli/poll/flow":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
				"user": map[string]string{"user_id": "wrong-user"},
			}})
		default:
			t.Error("mismatched renewal identity reached entitlement/key mutation")
		}
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("start")
	frame, _ := h.call(sdk.MethodAuthStart, scope(account), sdk.AuthStartRequest{Method: "browser"}, raw(account))
	var started sdk.AuthStartResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &started) != nil {
		t.Fatalf("renewal start: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodAuthFinish, scope(account),
		sdk.AuthFinishRequest{Continuation: started.Continuation}, raw(account))
	if frame.Error == nil || frame.Error.Code != "permission_denied" {
		t.Fatal("Start reauthorization replaced the selected account with another vendor user")
	}
}

func TestPaidHeadlessFlowDiscoversExistingCodingKeyAndPlan(t *testing.T) {
	var created, issued atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/oauth/cli/init":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"flow_id": "paid", "authorize_url": "https://bigmodel.cn/login?state=fixture",
				"poll_interval_sec": 1, "expires_at": fixtureNow.Add(5 * time.Minute).Unix(),
			}})
		case "/api/v1/oauth/cli/poll/paid":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
				"user": map[string]string{"user_id": "paid-user"}, "bigmodel": map[string]any{
					"access_token": fixtureJWT(fixtureNow.Add(2 * time.Hour)), "refresh_token": "opaque-not-a-session",
				},
			}})
		case "/api/biz/customer/getCustomerInfo":
			io.WriteString(writer, `{"code":0,"data":{"organizations":[{"organizationId":"org","organizationName":"默认机构","projects":[{"projectId":"project","projectName":"默认项目","projectType":1}]}]}}`)
		case "/api/biz/v1/organization/org/projects/project/api_keys":
			if request.Method == "POST" {
				created.Add(1)
			}
			io.WriteString(writer, `{"code":0,"data":[{"apiKey":"key","name":"zcode-api-key"}]}`)
		case "/api/biz/v1/organization/org/projects/project/api_keys/key/access_tokens":
			issued.Add(1)
			fmt.Fprintf(writer, `{"code":0,"data":{"accessToken":"new-paid-pat","tokenType":"Bearer","expiresIn":600,"expiresAt":%d}}`,
				fixtureNow.Add(10*time.Minute).Unix())
		case "/api/biz/subscription/list":
			if request.Header.Get("Authorization") != "new-paid-pat" {
				t.Error("subscription discovery did not use the selected PAT")
			}
			io.WriteString(writer, `{"code":200,"data":[{"productName":"GLM Coding Pro","status":"VALID","inCurrentPeriod":true}]}`)
		default:
			t.Error("unknown paid authorization endpoint")
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	selected := sdk.Scope{Provider: ProviderID}
	frame, _ := h.call(sdk.MethodAuthStart, selected,
		sdk.AuthStartRequest{Method: "browser", Input: raw(authSelection{Site: "bigmodel", Kind: "individual"})}, nil)
	var start sdk.AuthStartResult
	json.Unmarshal(frame.Result, &start)
	frame, _ = h.call(sdk.MethodAuthFinish, selected, sdk.AuthFinishRequest{Continuation: start.Continuation}, nil)
	var result sdk.AuthResult
	var account Credentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || json.Unmarshal(result.Credentials, &account) != nil {
		t.Fatalf("paid authorization: %+v", frame.Error)
	}
	if account.Kind != "individual" || account.Site != "bigmodel" || account.ProjectToken != "new-paid-pat" ||
		account.Location == nil || account.Location.APIKeyID != "key" || account.VendorRefreshToken != "opaque-not-a-session" ||
		created.Load() != 0 || issued.Load() != 1 {
		t.Fatal("paid flow copied a permanent key, lost role material or created an unnecessary key")
	}
}
