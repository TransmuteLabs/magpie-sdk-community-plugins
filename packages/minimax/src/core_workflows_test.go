package minimax

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func TestCoreDeviceConsecutiveFailuresResetAndStop(test *testing.T) {
	var now atomic.Int64
	now.Store(1790000000000)
	replies := []string{"server", "server", "pending", "slow_down", "server", "server", "server", "server"}
	polls := 0
	fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/oauth2/device/code" {
			return coreResponse(200, `{"user_code":"user-secret","device_code":"device-secret","verification_uri":"https://account.minimax.io/device","expires_in":600,"interval":1}`, "application/json"), nil
		}
		if request.URL.Path != "/oauth2/token" || polls >= len(replies) {
			test.Error("extra token request")
		}
		reply := replies[polls]
		polls++
		switch reply {
		case "pending":
			return coreResponse(400, `{"error":"authorization_pending"}`, "application/json"), nil
		case "slow_down":
			return coreResponse(400, `{"error":"slow_down"}`, "application/json"), nil
		default:
			return coreResponse(503, `{"error":"temporary device-secret"}`, "application/json"), nil
		}
	}))))
	scope := sdk.Scope{Provider: Global}
	started, _ := fixture.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{}, nil)
	if started.Error != nil {
		test.Fatal(started.Error)
	}
	var start sdk.AuthStartResult
	json.Unmarshal(started.Result, &start)
	var key continuation
	json.Unmarshal(start.Continuation, &key)
	for index := range replies {
		now.Add(30000)
		frame, _ := fixture.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: start.Continuation}, nil)
		if index == len(replies)-1 {
			if frame.Error == nil || strings.Contains(frame.Error.Message, "secret") {
				test.Fatal("fourth consecutive failure did not stop safely")
			}
		} else if frame.Error != nil {
			test.Fatalf("poll %d prematurely failed: %+v", index, frame.Error)
		}
		if index == 2 || index == 3 {
			var flow deviceFlow
			json.Unmarshal(fixture.state[Global+"\x00\x00device."+key.ID].body, &flow)
			if flow.Failures != 0 || index == 3 && flow.Interval != 6000 {
				test.Fatalf("pending reset/slow_down lost: %+v", flow)
			}
		}
	}
	fixture.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: start.Continuation}, nil)
	if polls != len(replies) {
		test.Fatal("failed workflow replayed")
	}
}

func TestCoreDeviceNeverPollsPastDeadline(test *testing.T) {
	var now atomic.Int64
	now.Store(1790000000000)
	polls := 0
	fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/oauth2/device/code" {
			polls++
		}
		return coreResponse(200, `{"user_code":"user-secret","device_code":"device-secret","verification_uri":"https://account.minimax.io/device","expires_in":10,"interval":1}`, "application/json"), nil
	}))))
	scope := sdk.Scope{Provider: Global}
	started, _ := fixture.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{}, nil)
	var start sdk.AuthStartResult
	json.Unmarshal(started.Result, &start)
	now.Add(10000)
	finished, _ := fixture.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: start.Continuation}, nil)
	if finished.Error == nil || polls != 0 {
		test.Fatal("expired workflow polled")
	}
}

func TestCoreModelsLiveCapabilitiesAndAnonymousFallback(test *testing.T) {
	var now atomic.Int64
	now.Store(1790000000000)
	requests := 0
	fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Query().Get("region") != "cn" || request.URL.Query().Get("buildEnv") != "prod" {
			test.Error("discovery region changed")
		}
		if requests == 1 {
			if request.Header.Get("Authorization") == "" {
				test.Error("discovery did not use selected account first")
			}
			return coreResponse(401, `{}`, "application/json"), nil
		}
		if request.Header.Get("Authorization") != "" {
			test.Error("anonymous discovery retained token")
		}
		return coreResponse(200, `{"providers":[{"providerId":"minimax","config":{"model_order":["MiniMax-M4","MiniMax-M3"],"models":{"MiniMax-M4":{"limit":{"context":1000000,"output":128000},"modalities":{"input":["text","image","video"]},"thinking_config":{"mode":"forced_on"},"thinking":{"effortOptions":["default","low","high"]}},"MiniMax-M3":{"limit":{"context":512000,"output":128000},"thinking_config":{"mode":"switchable"}}}}}]}`, "application/json"), nil
	}))))
	scope, credentials := coreAccount(China)
	frame, _ := fixture.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, &credentials)
	var result sdk.ModelsResult
	json.Unmarshal(frame.Result, &result)
	if frame.Error != nil || requests != 2 || len(result.Models) < 2 {
		test.Fatalf("discovery failed: %+v", frame)
	}
	live, known := result.Models[0], result.Models[1]
	if live.ID != "MiniMax-M4" || !live.Vision || !contains(live.InputModalities, "video") || len(live.Efforts) != 2 || known.ContextWindow != 1048576 || known.MaxOutput != 524288 || !contains(known.Efforts, "none") {
		test.Fatalf("live capabilities lost: %+v", result.Models[:2])
	}
	for _, model := range result.Models[:2] {
		if len(model.Protocols) != 1 || model.Protocols[0] != protocolext.Messages {
			test.Fatal("chat models advertised chat envelope")
		}
	}
}

func TestCorePlanPercentBoostAndZeroCounts(test *testing.T) {
	windows, err := planWindows([]byte(`{"model_remains":[{"model_name":"general","current_weekly_status":1,"current_weekly_used_percent":"24%","current_weekly_total_percent":"150%"},{"model_name":"video","current_interval_status":1,"current_interval_used_percent":"0%","current_interval_total_percent":"100%","current_interval_used_count":0,"current_interval_total_count":3}]}`))
	if err != nil || len(windows) != 2 || windows[0].Used != 16 || windows[0].Amount == nil || *windows[0].Amount != 24 || windows[0].Limit != 150 || windows[1].Amount == nil || *windows[1].Amount != 0 {
		test.Fatalf("quota denominator or explicit zero lost: %+v %v", windows, err)
	}
	windows, err = planWindows([]byte(`{"model_remains":[{"model_name":"general","current_weekly_status":1,"current_weekly_remaining_percent":83,"weekly_boost_permille":1500}]}`))
	if err != nil || len(windows) != 1 || windows[0].Used != 17 || windows[0].Amount == nil || *windows[0].Amount != 25.5 || windows[0].Limit != 150 {
		test.Fatal("boost normalized incorrectly")
	}
}

func TestCoreUsageCreditsAreNotResetAndPartialLedgerIsNotInvented(test *testing.T) {
	for _, partial := range []bool{false, true} {
		test.Run(fmt.Sprint(partial), func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/matrix/api/v1/user/get_user_extra_info":
					return coreResponse(200, `{"workspaces":[{"workspace_type":0,"workspace_id":7,"opcredit_balance":0}]}`, "application/json"), nil
				case "/matrix/api/v1/commerce/get_membership_info":
					return coreResponse(200, `{"has_token_plan":false,"opcredit_balance":0}`, "application/json"), nil
				case "/minimax-cloud/api/v1/credit/details":
					if request.URL.Query().Get("user_id") != "user-7" {
						test.Error("credit ledger lost real user")
					}
					total := 1
					if partial {
						total = 201
					}
					return coreResponse(200, fmt.Sprintf(`{"total_count":%d,"details":[{"credit_type":1,"granted_amount":"100.00","remaining_amount":"40.25","expire_at_ms":1790086400000}]}`, total), "application/json"), nil
				default:
					test.Errorf("unexpected usage write/read: %s %s", request.Method, request.URL.Path)
					return nil, fmt.Errorf("unexpected request")
				}
			}))))
			scope, credentials := coreAccount(Global)
			frame, _ := fixture.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{Revision: "17"}, &credentials)
			var result sdk.UsageResult
			json.Unmarshal(frame.Result, &result)
			if frame.Error != nil || result.Revision != "17" {
				test.Fatalf("usage failed: %+v", frame.Error)
			}
			found := false
			for _, window := range result.Windows {
				if window.Name == "Credits" {
					found = true
					if window.ResetsAt != 0 || window.Amount == nil || *window.Amount != 59.75 || window.Limit != 100 || !strings.Contains(window.Display, "expire") {
						test.Fatalf("credit expiry/count wrong: %+v", window)
					}
				}
			}
			if found == partial {
				test.Fatal("partial ledger reported as complete or complete ledger missing")
			}
		})
	}
}

func coreWeek(todayStatus int) []checkinDay {
	days := make([]checkinDay, 7)
	for index := range days {
		status := 1
		if index < 2 {
			status = 3
		}
		if index == 2 {
			status = todayStatus
		}
		days[index] = checkinDay{Number: index + 1, Status: status, Today: index == 2, Points: 10}
	}
	return days
}

func TestCoreCheckinReceiptsPreventAmbiguousReclaim(test *testing.T) {
	for _, unknown := range []bool{false, true} {
		test.Run(fmt.Sprint(unknown), func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			claims := 0
			var fixture *coreHarness
			fixture = newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "agent.minimaxi.com" || request.URL.Query().Get("user_id") != "user-7" || request.URL.Query().Get("device_platform") != "web" || request.Header.Get("Yy") == "" {
					test.Error("check-in authority/signature changed")
				}
				if request.URL.Path == "/minimax-cloud/api/v1/signin/status" {
					return coreResponse(200, string(raw(map[string]any{"data": map[string]any{"days": coreWeek(2)}})), "application/json"), nil
				}
				if request.URL.Path != "/minimax-cloud/api/v1/signin/claim" {
					test.Error("unexpected check-in endpoint")
				}
				body, _ := io.ReadAll(request.Body)
				if string(body) != "{}" {
					test.Error("claim body changed")
				}
				claims++
				accepted, invalidated := false, false
				for _, event := range fixture.events {
					accepted = accepted || event.Event == "accepted"
				}
				for _, callback := range fixture.callbacks {
					invalidated = invalidated || callback.Method == sdk.MethodUsageInvalidate
				}
				if !accepted || !invalidated {
					test.Error("claim preceded receipt/usage fence")
				}
				if unknown {
					return coreResponse(200, `{"data":{"claim_result":99}}`, "application/json"), nil
				}
				return coreResponse(200, `{"data":{"claim_result":1,"points":20}}`, "application/json"), nil
			}))))
			scope, credentials := coreAccount(China)
			request := sdk.ModuleCallRequest{Action: "account.checkin", Input: raw(sdk.AccountCheckinRequest{Day: "2026-11-15"})}
			frame, _ := fixture.invoke(sdk.MethodModuleCall, scope, request, &credentials)
			if unknown {
				if frame.Error == nil || frame.Error.Outcome != "uncertain" {
					test.Fatal("unknown claim declared complete")
				}
			} else {
				var result sdk.AccountCheckinResult
				json.Unmarshal(frame.Result, &result)
				if frame.Error != nil || result.Outcome != "claimed" || result.Streak == nil || *result.Streak != 3 || result.Credit == nil || *result.Credit != 20 {
					test.Fatalf("claim result changed: %+v %+v", frame.Error, result)
				}
			}
			again, _ := fixture.invoke(sdk.MethodModuleCall, scope, request, &credentials)
			if again.Error == nil || claims != 1 {
				test.Fatal("claim repeated despite durable fence")
			}
		})
	}
}

func TestCoreFailureFactsDistinguishQuotaFromPermission(test *testing.T) {
	for _, scenario := range []struct {
		body  string
		quota bool
	}{
		{`{"error":{"type":"permission_error","message":"积分余额不足"}}`, true},
		{`{"error":{"type":"permission_error","message":"insufficient permissions"}}`, false},
		{`{"error":{"message":"model context token limit exceeded"}}`, false},
		{`{"error":{"message":"content denied"},"debug":"credit quota"}`, false},
		{`{"error":{"message":"insufficient credits"}}`, true},
	} {
		facts := refusalFacts(403, http.Header{}, []byte(scenario.body), true)
		if facts.accountQuota() != scenario.quota {
			test.Fatalf("wrong quota classification: %+v", facts)
		}
		facts.TrustedEndpoint = false
		if facts.accountQuota() {
			test.Fatal("foreign refusal changed account classification")
		}
	}
}

func TestCoreMalformedRefusalsDoNotBecomeAuthOrQuota(test *testing.T) {
	for _, value := range []any{1008.5, 1000048.5, "1008.5", "1e3", "01008", -1008.0, true} {
		body := raw(map[string]any{"base_resp": map[string]any{"status_code": value, "status_msg": "insufficient balance"}})
		_, err := checkedJSON(ByteResponse{Status: 200, Body: body})
		if err == nil || refusalStatus(err) != 0 {
			test.Fatalf("malformed code became known refusal: %v %v", value, err)
		}
		if refusalFacts(403, http.Header{"Content-Type": {"application/json"}}, body, true).accountQuota() {
			test.Fatalf("malformed code authorized quota classification: %v", value)
		}
	}
	for _, value := range []any{float64(1008), "1008", float64(1000048), "1000048", float64(402), "402"} {
		_, err := checkedJSON(ByteResponse{Status: 200, Body: raw(map[string]any{"base_resp": map[string]any{"status_code": value}})})
		if err == nil || refusalStatus(err) == 0 {
			test.Fatalf("valid refusal code lost: %v %v", value, err)
		}
	}
	if refusalFacts(403, http.Header{"Content-Type": {"application/json"}}, []byte(`{"message":"insufficient credit"`), true).accountQuota() {
		test.Fatal("malformed JSON authorized quota classification")
	}
	if refusalFacts(403, http.Header{}, []byte(`{"code":0,"base_resp":{"status_code":1008.5,"status_msg":"insufficient balance"}}`), true).accountQuota() {
		test.Fatal("outer success hid malformed inner refusal")
	}
}
