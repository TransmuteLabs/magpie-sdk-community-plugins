package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type giftFixture struct {
	remaining atomic.Int64
	start     atomic.Int32
	paid      atomic.Int32
	quota     atomic.Int64
	reserve   atomic.Bool
	answer    string
	status    int
}

func newGiftFixture(t *testing.T, answer string, status int) (*giftFixture, *httptest.Server) {
	t.Helper()
	fixture := &giftFixture{answer: answer, status: status}
	fixture.remaining.Store(75)
	fixture.quota.Store(100)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Date", fixtureNow.Format(http.TimeFormat))
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
			io.WriteString(writer, `{"code":200,"data":[{"status":"VALID","inCurrentPeriod":true,"productName":"GLM Coding Pro"}]}`)
		case "/api/v1/zcode-plan/billing/balance":
			balances := []any{map[string]any{"plan_id": "grant", "user_plan_id": "owned", "total_units": 100,
				"remaining_units": fixture.remaining.Load(), "capabilities": []string{
					"model:glm-5.3", "model:glm-limited-preview", "model:GLM-LIMITED-PREVIEW", "tool:web-search",
				}}}
			if fixture.reserve.Load() {
				balances = append(balances, map[string]any{"plan_id": "grant", "user_plan_id": "owned",
					"show_name": "Reserve", "total_units": 25, "remaining_units": 0, "capabilities": []string{"model:GLM-5.3"}})
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"plans": []any{map[string]any{"plan_id": "grant", "user_plan_id": "owned", "status": "active",
					"ends_at": fixtureNow.Add(time.Hour).Unix()}},
				"balances": balances,
			}})
		case "/quota/zai":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"limits": []any{map[string]any{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": fixture.quota.Load()}},
			}})
		case "/start":
			fixture.start.Add(1)
			if request.Header.Get("Authorization") != "Bearer fixture-zcode-session" || request.Header.Get("X-Api-Key") != "" {
				t.Error("gift request used a paid credential")
			}
			var body map[string]any
			json.NewDecoder(request.Body).Decode(&body)
			if strings.HasSuffix(body["model"].(string), "-Trial") {
				t.Error("gift alias reached the vendor")
			}
			writer.WriteHeader(fixture.status)
			io.WriteString(writer, fixture.answer)
		case "/zai":
			fixture.paid.Add(1)
			io.WriteString(writer, `{"type":"message","content":[{"type":"text","text":"paid"}]}`)
		default:
			t.Errorf("unexpected gift fixture %s", request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return fixture, server
}

func giftAccount() Credentials {
	account := nativeCredentials()
	account.Discovery = true
	return account
}

func giftRequest(model string) sdk.ExecuteRequest {
	return sdk.ExecuteRequest{Protocol: "messages", Model: model, Body: raw(map[string]any{"model": model, "messages": []any{}})}
}

func TestGiftCatalogKeepsSpentAliasesAndModelsAbsentFromPaidCatalog(t *testing.T) {
	fixture, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
	h := newHarness(t, endpointOptions(server))
	for _, remaining := range []int64{75, 0} {
		fixture.remaining.Store(remaining)
		account := giftAccount()
		frame, _ := h.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
		var result sdk.ModelsResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
			t.Fatalf("gift catalog: %+v", frame.Error)
		}
		seen := map[string]sdk.Model{}
		for _, model := range result.Models {
			seen[model.ID] = model
		}
		if !seen["GLM-5.3-Trial"].Free || !seen["glm-limited-preview-Trial"].Free || seen["GLM-5.3"].Free ||
			len(result.Models) != 3 {
			t.Fatalf("grant discovery lost a spent/uncatalogued free model or changed the paid model: %+v", result.Models)
		}
	}
}

func TestGiftTrialRefusalNeverSpendsPaidAndQuotaInside200Is429(t *testing.T) {
	for _, scenario := range []struct {
		answer string
		status int
		want   int
	}{
		{`{"type":"message","content":[{"type":"text","text":"gift"}]}`, 200, 200},
		{`{"code":1005,"msg":"exceed quota limit"}`, 200, 429},
		{`{"code":"1113","msg":"no resource package"}`, 200, 429},
		{`{"error":{"type":"permission_error","message":"refused"}}`, 403, 403},
	} {
		t.Run(scenario.answer, func(t *testing.T) {
			fixture, server := newGiftFixture(t, scenario.answer, scenario.status)
			fixture.quota.Store(20)
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
			var result sdk.ExecuteResult
			proved := giftSpentReply(scenario.status, []byte(scenario.answer))
			statusMatches := len(h.headers) == 1 && h.headers[0].Status == scenario.want
			if proved {
				statusMatches = json.Unmarshal(frame.Result, &result) == nil && result.Refusal != nil &&
					result.Refusal.Facts.Status == scenario.status && result.Outcome == "not-started" && !h.accepted.Load()
			}
			if frame.Error != nil || !statusMatches ||
				fixture.start.Load() != 1 || fixture.paid.Load() != 0 {
				t.Fatalf("strict gift route or quota translation failed: %+v %+v", frame.Error, h.headers)
			}
		})
	}
}

func TestGiftSpentTrialRefusesLocallyButStaysDiscoverable(t *testing.T) {
	fixture, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
	fixture.remaining.Store(0)
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "not-started" || fixture.start.Load() != 0 ||
		fixture.paid.Load() != 0 || h.accepted.Load() {
		t.Fatal("spent Trial admitted inference or paid fallback")
	}
}

func TestGiftFreeFirstDefaultsOnAndCanBeDisabled(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "default", false: "disabled"}[enabled], func(t *testing.T) {
			fixture, server := newGiftFixture(t, `{"type":"message","content":[{"type":"text","text":"gift"}]}`, 200)
			config := map[string]any{}
			if !enabled {
				config["freeFirst"] = false
			}
			h := newHarnessConfigured(t, endpointOptions(server), config)
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			if frame.Error != nil || enabled && (fixture.start.Load() != 1 || fixture.paid.Load() != 0) ||
				!enabled && (fixture.start.Load() != 0 || fixture.paid.Load() != 1) {
				t.Fatalf("free-first policy ignored: %+v", frame.Error)
			}
		})
	}
}

func TestGiftFreeFirstUnknownReplyNeverReplaysOntoPaid(t *testing.T) {
	for _, answer := range []string{`{"code":500,"msg":"unknown server failure"}`, `{"type":"message","content":null}`,
		`{"code":1005,"content":[{"type":"text","text":"partial"}],"usage":{"output_tokens":1}}`, `garbled`} {
		t.Run(answer, func(t *testing.T) {
			fixture, server := newGiftFixture(t, answer, 200)
			fixture.quota.Store(20)
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			if fixture.start.Load() != 1 || fixture.paid.Load() != 0 {
				t.Fatal("unknown gift result reached paid fallback")
			}
		})
	}
}

func TestGiftFreeFirstReplaysOnlyDefinitiveUnusedQuotaWithConfirmedPaidRoom(t *testing.T) {
	for _, quota := range []int64{50, 100} {
		t.Run(map[int64]string{50: "room", 100: "full"}[quota], func(t *testing.T) {
			fixture, server := newGiftFixture(t, `{"code":1005,"msg":"exceed quota limit"}`, 200)
			fixture.quota.Store(quota)
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			wantPaid := int32(0)
			if quota < 100 {
				wantPaid = 1
			}
			if frame.Error != nil || fixture.start.Load() != 1 || fixture.paid.Load() != wantPaid {
				t.Fatalf("definitive quota fallback or paid-full barrier failed: %+v", frame.Error)
			}
		})
	}
}

func TestGiftSpentReplyRestsOnlyThatModelAndNeverReplaysTrial(t *testing.T) {
	fixture, server := newGiftFixture(t, `{"code":1005,"msg":"exceed quota limit"}`, 200)
	fixture.quota.Store(20)
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	for range 2 {
		frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
		if frame.Error != nil {
			t.Fatalf("free-first paid fallback failed: %+v", frame.Error)
		}
	}
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "not-started" || fixture.start.Load() != 1 || fixture.paid.Load() != 2 {
		t.Fatalf("model rest repeated gift or spent paid through Trial: %+v gift=%d paid=%d", frame.Error, fixture.start.Load(), fixture.paid.Load())
	}
}

func TestGiftDualUsageSeparatesTrialFromExhaustedPaidQuota(t *testing.T) {
	_, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
	var result sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
		t.Fatalf("dual usage: %+v", frame.Error)
	}
	paidExcluded, trialOpen := false, false
	for _, window := range result.Windows {
		if window.Name == "5 hours" {
			paidExcluded = containsString(window.NotModels, "GLM-5.3-Trial") && containsString(window.NotModels, "GLM-5.3")
		}
		if containsString(window.Models, "GLM-5.3-Trial") && window.Used == 25 && !window.Unknown {
			trialOpen = true
		}
	}
	if !paidExcluded || !trialOpen {
		t.Fatalf("paid quota blocks gift or gift quota became unknown: %+v", result.Windows)
	}
}

func TestGiftUsagePreservesAsideBucketScope(t *testing.T) {
	for _, test := range []struct {
		name      string
		account   Credentials
		config    map[string]any
		models    []string
		available string
	}{
		{name: "start", account: fixtureCredentials("start"), config: map[string]any{}, models: []string{"glm-5.3"},
			available: "glm-limited-preview"},
		{name: "dual", account: giftAccount(), config: map[string]any{}, models: []string{"GLM-5.3-Trial", "GLM-5.3"},
			available: "glm-limited-preview-Trial"},
		{name: "dual_paid_first", account: giftAccount(), config: map[string]any{"freeFirst": false},
			models: []string{"GLM-5.3-Trial"}, available: "glm-limited-preview-Trial"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
			fixture.reserve.Store(true)
			harness := newHarnessConfigured(t, endpointOptions(server), test.config)
			frame, _ := harness.call(sdk.MethodProviderUsage, scope(test.account), sdk.UsageRequest{}, raw(test.account))
			var result sdk.UsageResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
				t.Fatalf("usage: %+v", frame.Error)
			}
			reserve, other := false, false
			for _, window := range result.Windows {
				if window.Name == "Reserve · Start Plan" {
					reserve = true
					if !window.Aside || window.Used != 100 || !slices.Equal(window.Models, test.models) {
						t.Fatalf("reserve usage = %+v, want an aside at 100%% scoped to %v", window, test.models)
					}
				}
				other = other || !window.Aside && window.Used == 25 && containsString(window.Models, test.available)
			}
			if !reserve || !other {
				t.Fatalf("reserve or ordinary neighboring model missing from usage: %+v", result.Windows)
			}
		})
	}
}

func TestGiftFallbackKeepsUnsettledPrincipalDebt(t *testing.T) {
	for _, debt := range []string{"none", "claim", "reset"} {
		t.Run(debt, func(t *testing.T) {
			fixture, server := newGiftFixture(t, `{"code":1005}`, 200)
			fixture.quota.Store(20)
			path := filepath.Join(t.TempDir(), "controller")
			account := giftAccount()
			store, err := controller.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			err = store.Locked(t.Context(), controller.Principal(server.URL, account.UserID), func(transaction *controller.Transaction) error {
				switch debt {
				case "claim":
					transaction.Journal.Claims["unresolved"] = controller.Claim{Plan: "unresolved", Operation: "claim",
						Reference: "reference", State: "unknown", Generation: 1}
				case "reset":
					transaction.Journal.Reset[scope(account).Account] = controller.Reset{Pending: &controller.ResetClaim{
						Key: "reset", Window: "FIVE_HOUR", Operation: "reset", Generation: 1}}
				}
				return transaction.Save()
			})
			if err != nil {
				t.Fatal(err)
			}
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: path})
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			want := int32(0)
			if debt == "none" {
				want = 1
			}
			if frame.Error != nil || fixture.start.Load() != 1 || fixture.paid.Load() != want {
				t.Fatalf("gift fallback bypassed financial debt or blocked settled principal: %+v paid=%d", frame.Error, fixture.paid.Load())
			}
		})
	}
}
