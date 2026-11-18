package zcode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func quotaWire(t *testing.T, value any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestQuotaMetadataFullPackageDeclaresDonorDefaults(t *testing.T) {
	full := ControlManifest()
	descriptor := quotaWire(t, full.Providers[0])
	if full.Contract.Version.Minor != 12 || full.Compatibility.HostAPI != "^1.12" ||
		descriptor["maxConcurrency"] != float64(3) || descriptor["maxAccounts"] != nil ||
		len(full.Agents) != 1 || full.Window == nil {
		t.Fatal("ordinary full package lacks API9 donor defaults or its agent/UI")
	}
	legacy := quotaWire(t, Manifest().Providers[0])
	if legacy["maxConcurrency"] != nil || legacy["maxAccounts"] != nil {
		t.Fatal("historical API4 provider acquired API9 fields")
	}
}

func TestQuotaMetadataPaidCountsRemainExactAndSourceBacked(t *testing.T) {
	result, err := projectQuota([]byte(`{"code":0,"data":{"limits":[
		{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":99,"usage":10.25,"remaining":10.25,"nextResetTime":2000000000000},
		{"type":"CREDIT_LIMIT","unit":6,"number":1,"percentage":20,"usage":30,"currentValue":7.25},
		{"type":"TIME_LIMIT","unit":5,"number":1,"usage":100,"remaining":75}
	]}}`), 0)
	if err != nil {
		t.Fatal(err)
	}
	windows := quotaWire(t, result)["windows"].([]any)
	first, second, mcp := windows[0].(map[string]any), windows[1].(map[string]any), windows[2].(map[string]any)
	if first["amount"] != float64(0) || first["limit"] != 10.25 || first["display"] != "0 / 10.25" ||
		first["span"] != float64(18000) || first["used"] != float64(0) {
		t.Fatalf("remaining/zero source counts lost: %+v", first)
	}
	if second["amount"] != 7.25 || second["limit"] != float64(30) || second["used"] != float64(20) ||
		second["span"] != float64(604800) || mcp["span"] != float64(2592000) || mcp["aside"] != true {
		t.Fatalf("counts changed share or donor period: second=%+v mcp=%+v", second, mcp)
	}
	for _, value := range windows {
		window := value.(map[string]any)
		for _, key := range []string{"unit", "balance", "currency", "cost", "credits"} {
			if _, exists := window[key]; exists {
				t.Fatalf("invented vendor quota accounting %s: %+v", key, window)
			}
		}
	}
	unknown, err := projectQuota([]byte(`{"code":0,"data":{"limits":[
		{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":10,"usage":20}
	]}}`), 0)
	if err != nil {
		t.Fatal(err)
	}
	window := quotaWire(t, unknown.Windows[0])
	if _, exists := window["amount"]; exists {
		t.Fatal("missing count became measured zero")
	}
}

func TestQuotaMetadataStartCountsAndPeriodFollowPinnedDonor(t *testing.T) {
	for _, test := range []struct {
		name, period string
		start, end   int64
		span         float64
	}{
		{name: "daily", period: "day", span: 86400},
		{name: "weekly", period: "weekly", span: 604800},
		{name: "period-range", start: 100, end: 7300, span: 7200},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/v1/zcode-plan/billing/balance" {
					t.Errorf("unexpected metadata vendor path: %s", request.URL.Path)
					http.NotFound(writer, request)
					return
				}
				json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
					"server_time": fixtureNow.Unix(),
					"plans": []any{map[string]any{"plan_id": "plan", "user_plan_id": "owned", "name": "Trial", "status": "active",
						"entitlements": []any{map[string]any{"entitlement_id": "bucket", "period": test.period}}}},
					"balances": []any{
						map[string]any{"plan_id": "plan", "user_plan_id": "owned", "entitlement_id": "bucket",
							"show_name": "Model", "capabilities": []string{"model:GLM-5.3"}, "total_units": "10.25",
							"remaining_units": "10.25", "used_units": "0", "period_start": test.start, "period_end": test.end},
					},
				}})
			}))
			defer server.Close()
			harness := newHarnessModule(t, endpointOptions(server), moduleConfig{}, NewControlModule)
			account := fixtureCredentials("start")
			frame, _ := harness.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
			if frame.Error != nil {
				t.Fatal(frame.Error)
			}
			var result struct{ Windows []map[string]any }
			if err := json.Unmarshal(frame.Result, &result); err != nil || len(result.Windows) != 1 {
				t.Fatalf("Start metadata: %s, %v", frame.Result, err)
			}
			window := result.Windows[0]
			if window["amount"] != float64(0) || window["limit"] != 10.25 || window["span"] != test.span ||
				window["display"] != "0 / 10.25" {
				t.Fatalf("Start count or donor period lost: %+v", window)
			}
		})
	}
}

func TestQuotaMetadataPaidPeriodFallbackAndHistoricalCompatibility(t *testing.T) {
	body := []byte(`{"code":0,"data":{"limits":[
		{"type":"TOKENS_LIMIT","unit":3,"usage":10.25,"remaining":10.25}
	]}}`)
	result, err := projectQuota(body, 0)
	if err != nil || len(result.Windows) != 1 || quotaWire(t, result.Windows[0])["span"] != float64(18000) {
		t.Fatalf("donor's omitted team count did not mean five hours: %+v, %v", result, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write(body)
	}))
	defer server.Close()
	for _, full := range []bool{false, true} {
		factory := NewModule
		if full {
			factory = NewControlModule
		}
		harness := newHarnessModule(t, endpointOptions(server), moduleConfig{}, factory)
		account := fixtureCredentials("individual")
		frame, _ := harness.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
		if frame.Error != nil {
			t.Fatal(frame.Error)
		}
		var report struct{ Windows []map[string]any }
		if err := json.Unmarshal(frame.Result, &report); err != nil || len(report.Windows) != 1 {
			t.Fatalf("ordinary usage: %s, %v", frame.Result, err)
		}
		window := report.Windows[0]
		if full {
			if window["amount"] != float64(0) || window["limit"] != 10.25 || window["span"] != float64(18000) {
				t.Fatalf("full package lost donor metadata: %+v", window)
			}
		} else {
			for _, key := range []string{"amount", "balance", "display", "unit", "span"} {
				if _, exists := window[key]; exists {
					t.Fatalf("historical API4 emitted API9 field %s", key)
				}
			}
			if window["limit"] != float64(100) {
				t.Fatal("historical percentage limit changed")
			}
		}
	}
}
