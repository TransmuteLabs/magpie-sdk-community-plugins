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

func startConfigFixture() map[string]any {
	fixture := modelConfigFixture()
	config := fixture["config"].(map[string]any)
	config["providerConfigRules"] = map[string]any{"providerRules": []any{
		map[string]any{"providerId": "account:zai-start-plan",
			"config": map[string]any{"builtinModelIds": []string{"GLM-5.3", "GLM-5.2"}}},
	}}
	config["modelConfigRules"] = map[string]any{"modelRules": []any{
		map[string]any{"modelMatch": ".*", "config": map[string]any{"enabled": true,
			"properties": map[string]any{"contextWindow": 256000}}},
	}}
	return fixture
}

func TestStartDiscoveryUsesLiveBucketsAndServerDate(t *testing.T) {
	for _, scenario := range []string{"live", "spent", "expired-bucket", "another-user-plan", "unknown-balance",
		"malformed-remaining", "malformed-expiry", "pending-plan", "malformed-plan"} {
		t.Run(scenario, func(t *testing.T) {
			plan := map[string]any{"plan_id": "shared", "user_plan_id": "owned", "status": "active",
				"starts_at": fixtureNow.Add(-time.Hour).Unix(), "ends_at": fixtureNow.Add(time.Hour).Unix()}
			bucket := map[string]any{"plan_id": "shared", "user_plan_id": "owned",
				"remaining_units": "2.5", "expires_at": fixtureNow.Add(time.Hour).Unix(),
				"capabilities": []string{"model:GLM-5.3"}}
			wantError := false
			wantModels := 0
			switch scenario {
			case "live":
				wantModels = 1
			case "spent":
				bucket["remaining_units"], bucket["total_units"], bucket["used_units"] = 0, 100, 1
				wantModels = 1
			case "expired-bucket":
				bucket["expires_at"] = fixtureNow.Add(-time.Second).Unix()
			case "another-user-plan":
				bucket["user_plan_id"] = "not-owned"
			case "unknown-balance":
				delete(bucket, "remaining_units")
				wantError = true
			case "malformed-remaining":
				bucket["remaining_units"], wantError = "garbage", true
			case "malformed-expiry":
				bucket["expires_at"], wantError = "1e300", true
			case "pending-plan":
				plan["starts_at"], wantError = fixtureNow.Add(time.Minute).Unix(), true
			case "malformed-plan":
				plan["ends_at"], wantError = "garbage", true
			}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Date", fixtureNow.Format(http.TimeFormat))
				switch request.URL.Path {
				case "/api/v1/client/configs":
					json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
						"configs": map[string]string{"builtin_provider_config_json": server.URL + "/public-config"},
					}})
				case "/public-config":
					if request.Header.Get("Authorization") != "" || request.Header.Get("X-Api-Key") != "" {
						t.Error("public model configuration received account credentials")
					}
					json.NewEncoder(writer).Encode(startConfigFixture())
				case "/api/v1/zcode-plan/billing/balance":
					json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
						"server_time": fixtureNow.Add(24 * time.Hour).Unix(),
						"plans":       []any{plan}, "balances": []any{bucket},
					}})
				default:
					t.Error("Start model discovery used an unrelated endpoint")
					writer.WriteHeader(404)
				}
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			account := fixtureCredentials("start")
			account.Discovery = true
			frame, _ := h.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
			var result sdk.ModelsResult
			if wantError {
				if frame.Error == nil {
					t.Fatal("unknown or unavailable entitlement invented an eligible catalog")
				}
			} else if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Models) != wantModels {
				t.Fatalf("Start discovery: %+v %+v", frame.Error, result)
			}
		})
	}
}

func TestDiscoveredExecutionRechecksEligibilityBeforeInference(t *testing.T) {
	var inferences atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/client/configs":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"configs": map[string]string{"builtin_provider_config_json": server.URL + "/public"},
			}})
		case "/public":
			json.NewEncoder(writer).Encode(startConfigFixture())
		case "/api/v1/zcode-plan/billing/balance":
			io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"owned","status":"active"}],"balances":[{"plan_id":"owned","remaining_units":1,"capabilities":["model:GLM-5.2"]}]}}`)
		case "/start":
			inferences.Add(1)
		default:
			t.Errorf("unexpected discovery target: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("start")
	account.Discovery = true
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account),
		execute(`{"model":"GLM-5.3","messages":[]}`), raw(account))
	if frame.Error == nil || inferences.Load() != 0 || h.accepted.Load() ||
		!strings.Contains(frame.Error.Message, "model") {
		t.Fatalf("ineligible discovery model was sent upstream: %+v", frame.Error)
	}
}
