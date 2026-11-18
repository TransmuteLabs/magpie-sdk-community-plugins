package zcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const recoveryFlowID = "0123456789abcdef0123456789abcdef"

func seedFlow(h *harness, selected sdk.Scope, flow authFlow) sdk.AuthFinishRequest {
	key := fmt.Sprintf("%s/%s/%d/auth.%s", selected.Provider, selected.Account, selected.Generation, recoveryFlowID)
	h.state[key] = sdk.StateValue{Found: true, Version: "1", Value: raw(flow)}
	return sdk.AuthFinishRequest{Continuation: raw(authContinuation{ID: recoveryFlowID})}
}

func readyStartFlow(user string) authFlow {
	return authFlow{
		Selection: authSelection{Site: "zai", Kind: "start"}, PollToken: "private-poll",
		FlowID: "vendor-flow", Device: "fixture-device", Interval: 1000, Phase: "ready",
		Deadline: fixtureNow.Add(5 * time.Minute).UnixMilli(), ReadyAt: fixtureNow.UnixMilli(),
		Ready: raw(map[string]any{"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
			"user": map[string]string{"user_id": user}}),
	}
}

func TestSelectedStartReauthorizationKeepsAccountAndReplacesSession(t *testing.T) {
	account := fixtureCredentials("start")
	flow := readyStartFlow(account.UserID)
	flow.TargetAccount, flow.TargetUser, flow.TargetGeneration = account.AccountID(), account.UserID, scope(account).Generation
	flow.Device = account.Device
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/zcode-plan/billing/balance" ||
			request.Header.Get("Authorization") != "Bearer "+fixtureJWT(fixtureNow.Add(time.Hour)) {
			t.Error("reauthorization used the old session or dispatched another exchange")
		}
		io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	selected := scope(account)
	finish := seedFlow(h, selected, flow)
	account.ExpiresAt = fixtureNow.Add(-time.Minute).UnixMilli()
	frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, raw(account))
	var result sdk.AuthResult
	var renewed Credentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
		json.Unmarshal(result.Credentials, &renewed) != nil ||
		result.ID != account.AccountID() || renewed.SessionToken == account.SessionToken ||
		renewed.Device != account.Device || !renewed.Discovery || renewed.ExpiresAt != fixtureNow.Add(time.Hour).UnixMilli() {
		t.Fatalf("same-account Start renewal: %+v %+v", frame.Error, result)
	}
}

func TestStartRenewalRejectsChangedGenerationBeforePolling(t *testing.T) {
	account := fixtureCredentials("start")
	flow := readyStartFlow(account.UserID)
	flow.TargetAccount, flow.TargetUser, flow.TargetGeneration = account.AccountID(), account.UserID, 6
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("stale generation reached fixture vendor")
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	finish := seedFlow(h, scope(account), flow)
	frame, _ := h.call(sdk.MethodAuthFinish, scope(account), finish, raw(account))
	if frame.Error == nil || frame.Error.Code != "permission_denied" || h.proxies != 0 {
		t.Fatal("a stale authorization generation dispatched vendor work")
	}
}

func TestLostAuthorizationReceiptNeverRepeatsCompletion(t *testing.T) {
	var completions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		completions.Add(1)
		io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	selected := sdk.Scope{Provider: ProviderID}
	finish := seedFlow(h, selected, readyStartFlow("receipt-user"))
	h.rejectCAS = h.casCalls + 2
	frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
	if frame.Error == nil {
		t.Fatal("lost receipt was returned as a durable success")
	}
	frame, _ = h.call(sdk.MethodAuthFinish, selected, finish, nil)
	if frame.Error == nil || frame.Error.Outcome != "uncertain" || completions.Load() != 1 {
		t.Fatal("an unrecorded completion was exchanged again")
	}
}

func TestPollingLeaseRecoveryCannotReplayExchangingFlow(t *testing.T) {
	for _, phase := range []string{"polling", "exchanging", "uncertain", "failed", "unrecognized", "expired"} {
		t.Run(phase, func(t *testing.T) {
			var polls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				polls.Add(1)
				if request.Header.Get("Authorization") != "Bearer private-poll" {
					t.Error("private poll credential changed")
				}
				io.WriteString(writer, `{"code":0,"data":{"status":"pending"}}`)
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			flow := readyStartFlow("lease-user")
			flow.Phase, flow.PollLeaseUntil = phase, fixtureNow.Add(-time.Second).UnixMilli()
			if phase == "expired" {
				flow.Phase, flow.Deadline = "awaiting", fixtureNow.Add(-time.Second).UnixMilli()
			}
			selected := sdk.Scope{Provider: ProviderID}
			frame, _ := h.call(sdk.MethodAuthFinish, selected, seedFlow(h, selected, flow), nil)
			if phase == "polling" {
				var result sdk.AuthResult
				if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || !result.Pending || polls.Load() != 1 {
					t.Fatal("expired read-only polling lease did not recover")
				}
			} else if frame.Error == nil || polls.Load() != 0 {
				t.Fatal("terminal/uncertain/invalid authorization was dispatched again")
			}
		})
	}
}

func TestAuthorizationContentionDoesNotDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("contended completion reached fixture vendor")
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	selected := sdk.Scope{Provider: ProviderID}
	finish := seedFlow(h, selected, readyStartFlow("contended"))
	h.rejectCAS = h.casCalls + 1
	frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
	if frame.Error == nil || frame.Error.Code != "conflict" || h.proxies != 0 {
		t.Fatal("completion dispatched after another invoker won its CAS")
	}
}

func TestUnconfirmedCodingKeyCreationIsNeverReplayed(t *testing.T) {
	var created atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/biz/customer/getCustomerInfo":
			io.WriteString(writer, `{"code":0,"data":{"organizations":[{"organizationId":"org","projects":[{"projectId":"project","projectType":1}]}]}}`)
		case "/api/biz/v1/organization/org/projects/project/api_keys":
			if request.Method == http.MethodGet {
				io.WriteString(writer, `{"code":0,"data":[]}`)
			} else {
				created.Add(1)
				io.WriteString(writer, `{"code":0,"data":{"apiKey":"allocated-but-unconfirmed"}}`)
			}
		default:
			t.Error("unconfirmed key creation continued to PAT issuance")
			writer.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	flow := readyStartFlow("key-user")
	flow.Selection = authSelection{Site: "bigmodel", Kind: "individual"}
	flow.Ready = raw(map[string]any{
		"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
		"user":     map[string]string{"user_id": "key-user"},
		"bigmodel": map[string]string{"access_token": fixtureJWT(fixtureNow.Add(2 * time.Hour))},
	})
	selected := sdk.Scope{Provider: ProviderID}
	finish := seedFlow(h, selected, flow)
	for range 2 {
		frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
		if frame.Error == nil || frame.Error.Outcome != "uncertain" {
			t.Fatalf("unconfirmed key mutation lost uncertainty: %+v", frame.Error)
		}
	}
	if created.Load() != 1 {
		t.Fatal("a vendor key mutation was replayed")
	}
}

func TestRejectedPollIsTerminalEvenWithInvalidJSON(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		polls.Add(1)
		writer.WriteHeader(http.StatusUnauthorized)
		io.WriteString(writer, "<html>denied</html>")
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	flow := readyStartFlow("declined")
	flow.Phase = "awaiting"
	selected := sdk.Scope{Provider: ProviderID}
	finish := seedFlow(h, selected, flow)
	for range 2 {
		frame, _ := h.call(sdk.MethodAuthFinish, selected, finish, nil)
		if frame.Error == nil {
			t.Fatal("declined authorization was presented as pending")
		}
	}
	if polls.Load() != 1 {
		t.Fatal("terminal denial was polled again")
	}
}
