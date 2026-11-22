package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDonor0166ChinaRenewalUsesSelectedUserNotDisplayName(t *testing.T) {
	for _, user := range []string{"china-user-a", "china-user-b"} {
		t.Run(user, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				reads.Add(1)
				if request.URL.Path != "/api/v1/zcode-plan/billing/balance" {
					t.Error("CN renewal reached another credential channel")
				}
				io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`)
			}))
			t.Cleanup(server.Close)
			harness := newHarness(t, endpointOptions(server))
			account := fixtureCredentials("start")
			account.Site, account.LoginProvider, account.UserID = "bigmodel", "zai_cn", "china-user-a"
			selected := scope(account)
			flow := readyStartFlow(user)
			flow.Selection = authSelection{Site: "bigmodel", LoginProvider: "zai_cn", Kind: "start"}
			flow.TargetAccount, flow.TargetUser, flow.TargetGeneration = account.AccountID(), account.UserID, selected.Generation
			var ready map[string]any
			json.Unmarshal(flow.Ready, &ready)
			ready["user"].(map[string]any)["email"] = "same-display@example.invalid"
			flow.Ready = raw(ready)
			frame, _ := harness.call(sdk.MethodAuthFinish, selected, seedFlow(harness, selected, flow), raw(account))
			if user != account.UserID {
				if frame.Error == nil || frame.Error.Code != "permission_denied" || reads.Load() != 0 {
					t.Fatal("same display name let another CN user replace the selected account")
				}
				return
			}
			var result sdk.AuthResult
			var renewed Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Name != "same-display@example.invalid · ZCode bigmodel start" ||
				json.Unmarshal(result.Credentials, &renewed) != nil || renewed.UserID != account.UserID ||
				renewed.LoginProvider != "zai_cn" || renewed.AccountID() != account.AccountID() || reads.Load() != 1 {
				t.Fatalf("same-user CN renewal changed stable identity: %+v", frame.Error)
			}
		})
	}
}

func TestDonor0166DiscoveryNeverBillsExcludedGiftModelOrFailedPlanRead(t *testing.T) {
	vendor, options := newCompletionVendor(t)
	vendor.paidActive.Store(false)
	harness := newHarness(t, options)
	account := giftAccount()
	frame, _ := harness.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
	if frame.Error != nil {
		t.Fatal(frame.Error)
	}
	frame, _ = harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.2"), raw(account))
	if frame.Error == nil || vendor.paidCalls.Load() != 0 || vendor.startCalls.Load() != 0 {
		t.Fatal("gift-only account dispatched a model absent from its gift balance")
	}
	vendor.paidList = `{"code":503,"msg":"synthetic plan-list failure"}`
	frame, _ = harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if frame.Error == nil || vendor.paidCalls.Load() != 0 || vendor.startCalls.Load() != 0 {
		t.Fatal("failed plan-list read promoted gift-only selection to billed authority")
	}
	vendor.paidList = ""
	vendor.balance = `{"code":0,"data":{"plans":[{"plan_id":"grant","status":"active"}],"balances":[
		{"plan_id":"grant","total_units":100,"remaining_units":50,"capabilities":["model:GLM-5.2"]}
	]}}`
	frame, _ = harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.2"), raw(account))
	if frame.Error != nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 0 {
		t.Fatalf("newly confirmed gift remained excluded or became paid: %+v", frame.Error)
	}
}

func TestDonor0166ManualStartNeverBillsModelOutsideCurrentGift(t *testing.T) {
	vendor, options := newCompletionVendor(t)
	vendor.balance = `{"code":0,"data":{"plans":[{"plan_id":"grant","status":"active"}],"balances":[
		{"plan_id":"grant","total_units":100,"remaining_units":50,"capabilities":["model:GLM-5.3-Flash"]}
	]}}`
	harness := newHarness(t, options)
	account := fixtureCredentials("start")
	frame, _ := harness.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
	var models sdk.ModelsResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &models) != nil ||
		len(models.Models) != 1 || models.Models[0].ID != "GLM-5.3-Flash" || !models.Models[0].Free {
		t.Fatalf("manual gift-only Start advertised a model outside its current balance: %+v", frame.Error)
	}
	frame, _ = harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.2"), raw(account))
	if frame.Error == nil || vendor.startCalls.Load() != 0 || vendor.paidCalls.Load() != 0 || harness.accepted.Load() {
		t.Fatal("manual gift-only Start sent a model its balance does not name")
	}
	frame, _ = harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Flash"), raw(account))
	if frame.Error != nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 0 {
		t.Fatalf("confirmed Start model was not served on its selected gift: %+v", frame.Error)
	}
	vendor.balance = `{"code":503}`
	frame, _ = harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Flash"), raw(account))
	if frame.Error == nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 0 {
		t.Fatal("unknown Start balance authorized another potentially billed request")
	}
}

func TestDonor0166MissingTrialReadsOnlyBoundedUnclaimedHint(t *testing.T) {
	for _, scenario := range []string{"weekend", "empty", "failed", "spent"} {
		t.Run(scenario, func(t *testing.T) {
			var feeds, inferences, claims atomic.Int32
			var clock atomic.Int64
			clock.Store(fixtureNow.UnixMilli())
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/zcode-plan/billing/balance":
					if scenario == "spent" {
						io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"grant","status":"active"}],"balances":[
							{"plan_id":"grant","total_units":10,"remaining_units":0,"capabilities":["model:GLM-5.3-Flash"]}
						]}}`)
					} else {
						io.WriteString(writer, `{"code":0,"data":{"plans":[],"balances":[]}}`)
					}
				case "/api/v1/marketing/touch":
					feeds.Add(1)
					if request.Method != http.MethodGet || request.Header.Get("X-ZCode-App-Version") != "3.18.2" {
						t.Error("Trial hint mutated state or lost the pinned client version")
					}
					if scenario == "failed" {
						writer.WriteHeader(502)
						io.WriteString(writer, "synthetic unreadable feed")
					} else if scenario == "empty" {
						io.WriteString(writer, `{"code":0,"data":{"deliveries":[]}}`)
					} else {
						io.WriteString(writer, `{"code":0,"data":{"deliveries":[{
							"zcode_plan":{"name":"ZCODE WEEKEND BUILD"},
							"action":{"type":"claim_zcode_plan","args":{"plan_id":"weekend"}}
						}]}}`)
					}
				case "/api/v1/zcode-plan/billing/claim":
					claims.Add(1)
					writer.WriteHeader(500)
				default:
					inferences.Add(1)
					io.WriteString(writer, `{"type":"message","content":[]}`)
				}
			}))
			t.Cleanup(server.Close)
			options := endpointOptions(server)
			options.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
			harness := newHarness(t, options)
			account := nativeCredentials()
			for range 2 {
				frame, _ := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Flash-Trial"), raw(account))
				if frame.Error == nil || frame.Error.Outcome != "not-started" || harness.accepted.Load() {
					t.Fatal("missing or spent Trial acquired dispatch authority")
				}
				hint := strings.Contains(frame.Error.Message, "ZCODE WEEKEND BUILD") &&
					strings.Contains(frame.Error.Message, "claim it in the ZCode app")
				if hint != (scenario == "weekend") {
					t.Fatalf("wrong unclaimed Trial explanation: %+v", frame.Error)
				}
			}
			expected := int32(1)
			if scenario == "failed" {
				expected = 2
			} else if scenario == "spent" {
				expected = 0
			}
			if feeds.Load() != expected || inferences.Load() != 0 || claims.Load() != 0 {
				t.Fatalf("hint repeated cached reads, posted a claim or dispatched: feeds=%d infer=%d claims=%d",
					feeds.Load(), inferences.Load(), claims.Load())
			}
			if scenario == "weekend" {
				clock.Add((11 * time.Minute).Milliseconds())
				harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Flash-Trial"), raw(account))
				account.SessionToken += "-another-session"
				harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Flash-Trial"), raw(account))
				selected := scope(account)
				selected.Generation++
				harness.call(sdk.MethodProviderExecute, selected, giftRequest("GLM-5.3-Flash-Trial"), raw(account))
				if feeds.Load() != 4 {
					t.Fatal("unclaimed names outlived their TTL or crossed the selected session/generation")
				}
			}
		})
	}
}

func TestDonor0166DiscoveredTrialNamesWeekendWithoutPaidAuthority(t *testing.T) {
	for _, kind := range []string{"start", "individual"} {
		t.Run(kind, func(t *testing.T) {
			var feeds, inferences, plans atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/client/configs":
					json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
						"configs": map[string]string{"builtin_provider_config_json": server.URL + "/public"},
					}})
				case "/public":
					config := modelConfigFixture()
					providers := config["config"].(map[string]any)["providerConfigRules"].(map[string]any)
					providers["providerRules"] = append(providers["providerRules"].([]any),
						map[string]any{"providerId": "account:zai-start-plan", "config": map[string]any{"builtinModelIds": []string{"GLM-5.3"}}})
					json.NewEncoder(writer).Encode(config)
				case "/api/biz/subscription/list":
					plans.Add(1)
					io.WriteString(writer, `{"code":0,"data":[]}`)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[],"balances":[]}}`)
				case "/api/v1/marketing/touch":
					feeds.Add(1)
					io.WriteString(writer, `{"code":0,"data":{"deliveries":[{
						"zcode_plan":{"name":"ZCODE WEEKEND BUILD"},
						"action":{"type":"claim_zcode_plan","args":{"plan_id":"weekend"}}
					}]}}`)
				default:
					inferences.Add(1)
					t.Error("unavailable discovered Trial acquired a write or inference path")
					http.NotFound(writer, request)
				}
			}))
			t.Cleanup(server.Close)
			harness := newHarness(t, endpointOptions(server))
			account := nativeCredentials()
			if kind == "start" {
				account = fixtureCredentials(kind)
			}
			account.Discovery = true
			frame, _ := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Flash-Trial"), raw(account))
			if frame.Error == nil || frame.Error.Outcome != "not-started" ||
				!strings.Contains(frame.Error.Message, "ZCODE WEEKEND BUILD") ||
				feeds.Load() != 1 || inferences.Load() != 0 || plans.Load() != 0 || harness.accepted.Load() {
				t.Fatalf("discovered Trial lost the weekend explanation behind paid/catalog admission: %+v feeds=%d plans=%d",
					frame.Error, feeds.Load(), plans.Load())
			}
		})
	}
}
