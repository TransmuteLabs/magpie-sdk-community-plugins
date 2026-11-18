package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestCheckinRateHoldHonorsRetryAfterAcrossRestartAndOtherPlan(t *testing.T) {
	for _, duration := range []time.Duration{90 * time.Second, 2 * time.Hour} {
		t.Run(duration.String(), func(t *testing.T) {
			var feeds, claims atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/marketing/touch":
					feeds.Add(1)
					writer.Header().Set("Retry-After", fixtureNow.Add(duration).Format(http.TimeFormat))
					writer.WriteHeader(429)
					io.WriteString(writer, `{"code":3002}`)
				case "/api/v1/zcode-plan/billing/claim":
					claims.Add(1)
					t.Error("principal held by the feed reached claim")
				default:
					t.Errorf("held principal reached unexpected target %s", request.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			for range 2 {
				h := newHarnessConfigured(t, endpointOptions(server), config)
				frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("billing.checkin", map[string]string{}), raw(account))
				var result struct {
					Outcome string `json:"outcome"`
					Until   int64  `json:"until"`
				}
				if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Outcome != "rate_limited" ||
					result.Until != fixtureNow.Add(duration).UnixMilli() {
					t.Fatalf("feed hold lost Retry-After: %+v %s", frame.Error, frame.Result)
				}
				frame, _ = h.call(sdk.MethodModuleCall, scope(account), actionInput("billing.claim", map[string]string{"planId": "another-gift"}), raw(account))
				if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Outcome != "rate_limited" {
					t.Fatal("manual claim bypassed principal feed hold")
				}
				frame, _ = h.call(sdk.MethodModuleCall, scope(account), actionInput("billing.offers", map[string]string{}), raw(account))
				if frame.Error == nil || frame.Error.Code != "rate_limit_error" {
					t.Fatal("offer discovery ignored principal feed hold")
				}
			}
			if feeds.Load() != 1 || claims.Load() != 0 {
				t.Fatal("durable feed hold replayed")
			}
		})
	}
}
func TestClaimAcknowledgmentsAndDefinitiveRefusalsSurviveRestart(t *testing.T) {
	for _, scenario := range []struct {
		answer  string
		status  int
		outcome string
	}{
		{`{"code":0,"msg":"claimed"}`, 200, "claimed"},
		{`{"code":"0","msg":"claimed"}`, 200, "claimed"},
		{`{"code":1003,"msg":"already exists"}`, 200, "done"},
		{`{"code":1004,"msg":"not eligible"}`, 200, "ineligible"},
		{`{"code":3007,"msg":"captcha verify failed"}`, 400, "captcha"},
		{`{"code":3002,"msg":"too frequent"}`, 429, "rate_limited"},
	} {
		t.Run(scenario.outcome+scenario.answer, func(t *testing.T) {
			var claims atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/client/configs":
					io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[],"balances":[]}}`)
				case "/api/v1/zcode-plan/billing/claim":
					claims.Add(1)
					writer.WriteHeader(scenario.status)
					io.WriteString(writer, scenario.answer)
				case "/quota/zai":
					io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":20}]}}`)
				default:
					t.Errorf("unexpected claim target %s", request.URL.Path)
					writer.WriteHeader(404)
				}
			}))
			t.Cleanup(server.Close)
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			for range 2 {
				h := newHarnessConfigured(t, endpointOptions(server), config)
				frame, _ := h.call(sdk.MethodModuleCall, scope(account),
					actionInput("billing.claim", map[string]string{"planId": "gift"}), raw(account))
				var result struct {
					Outcome string `json:"outcome"`
				}
				if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Outcome != scenario.outcome {
					t.Fatalf("definitive claim became uncertain or was mislabeled: %+v %s", frame.Error, frame.Result)
				}
				frame, _ = h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
				var report sdk.UsageResult
				if frame.Error != nil || json.Unmarshal(frame.Result, &report) != nil || report.ReadDeferred {
					t.Fatal("definitive claim left an unresolved financial barrier")
				}
			}
			if claims.Load() != 1 {
				t.Fatal("definitive claim or refusal was reposted after restart")
			}
		})
	}
}

func TestCheckinUsesOneFeedAndDurableClaimsWithoutPreviewOrArbitraryActions(t *testing.T) {
	var feeds, claims atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/marketing/touch":
			feeds.Add(1)
			if request.Header.Get("X-ZCode-App-Version") != ClientVersion ||
				request.Header.Get("X-Client-Language") != "ru-RU" || request.URL.Query().Get("seq") == "" {
				t.Error("feed lost version, explicit supported locale or sequence")
			}
			io.WriteString(writer, `{"code":0,"data":{"deliveries":[{"banner":{"background":{"args":{"zcode_plan":{"name":"Synthetic grant"}}},"buttons":[{"action":{"type":"claim_zcode_plan","args":{"plan_id":"gift"}}},{"action":{"type":"claim_zcode_plan","args":{"plan_id":"gift"}}},{"action":{"type":"open_url","args":{"url":"https://must-not-open.invalid"}}}]}}]}}`)
		case "/api/v1/client/configs":
			io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
		case "/api/v1/zcode-plan/billing/balance":
			io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
		case "/api/v1/zcode-plan/billing/claim":
			claims.Add(1)
			io.WriteString(writer, `{"code":0}`)
		default:
			t.Errorf("check-in used unsupported target %s", request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
	account := nativeCredentials()
	for range 2 {
		h := newHarnessConfigured(t, endpointOptions(server), config)
		frame, _ := h.call(sdk.MethodModuleCall, scope(account),
			actionInput("billing.checkin", map[string]string{"locale": "ru-RU"}), raw(account))
		var result struct {
			Outcome string `json:"outcome"`
		}
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Outcome != "claimed" {
			t.Fatalf("check-in did not reuse bounded feed/claim: %+v %s", frame.Error, frame.Result)
		}
	}
	if feeds.Load() != 1 || claims.Load() != 1 {
		t.Fatalf("check-in repeated settled feed or claim: %d/%d", feeds.Load(), claims.Load())
	}
}

func TestCheckinUnknownFeedNeverClaimsAndUnknownClaimNeverReplays(t *testing.T) {
	for _, malformed := range []bool{true, false} {
		t.Run(map[bool]string{true: "feed", false: "claim"}[malformed], func(t *testing.T) {
			var claims atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/marketing/touch":
					if malformed {
						io.WriteString(writer, `{"code":0,"data":{"deliveries":false}}`)
					} else {
						io.WriteString(writer, `{"code":0,"data":{"deliveries":[{"action":{"type":"claim_zcode_plan","args":{"plan_id":"gift"}}}]}}`)
					}
				case "/api/v1/client/configs":
					io.WriteString(writer, `{"code":0,"data":{"configs":{}}}`)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
				case "/api/v1/zcode-plan/billing/claim":
					claims.Add(1)
					writer.WriteHeader(503)
				default:
					t.Errorf("unexpected check-in fixture %s", request.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
			account := nativeCredentials()
			for range 2 {
				frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("billing.checkin", map[string]string{}), raw(account))
				if frame.Error == nil {
					t.Fatal("unknown feed or claim was reported as settled")
				}
			}
			want := int32(1)
			if malformed {
				want = 0
			}
			if claims.Load() != want {
				t.Fatal("unknown outcome admitted a speculative or repeated claim")
			}
		})
	}
}

func TestClaimMalformedDataNeverBecomesAcknowledgedOrReposted(t *testing.T) {
	for _, answer := range []string{`{"code":0,"data":false}`, `{"code":0,"data":{"plan":false}}`,
		`{"code":0,"data":{"plan":null}}`, `{"code":0,"data":{"plan":{"plan_id":"another-plan"}}}`} {
		t.Run(answer, func(t *testing.T) {
			var claims atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/client/configs":
					io.WriteString(writer, `{"code":0,"data":{"configs":{}}}`)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
				case "/api/v1/zcode-plan/billing/claim":
					claims.Add(1)
					io.WriteString(writer, answer)
				default:
					t.Errorf("malformed claim reached unexpected target %s", request.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			for range 2 {
				h := newHarnessConfigured(t, endpointOptions(server), config)
				frame, _ := h.call(sdk.MethodModuleCall, scope(account),
					actionInput("billing.claim", map[string]string{"planId": "gift"}), raw(account))
				if frame.Error == nil || frame.Error.Outcome != "uncertain" {
					t.Fatalf("malformed claim was acknowledged: %+v %s", frame.Error, frame.Result)
				}
			}
			if claims.Load() != 1 {
				t.Fatal("malformed claim was reposted")
			}
		})
	}
}
