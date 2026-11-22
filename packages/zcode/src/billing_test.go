package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func actionInput(action string, input any) sdk.ModuleCallRequest {
	return sdk.ModuleCallRequest{Action: action, Input: raw(input)}
}

func TestBillingClaimFencesTheSameVendorUserAcrossAccountSelections(t *testing.T) {
	var claims atomic.Int32
	_, options := newCompletionServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/client/configs":
			io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
		case "/api/v1/zcode-plan/billing/balance":
			io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
		case "/api/v1/zcode-plan/billing/claim":
			claims.Add(1)
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"plan": map[string]any{
				"plan_id": "gift", "user_plan_id": "new-plan", "status": "active", "ends_at": fixtureNow.Add(time.Hour).Unix(),
			}}})
		default:
			t.Errorf("unexpected billing fixture: owner=%s method=%s host=%q path=%q remote=%q agent=%q authorization=%t",
				t.Name(), request.Method, request.Host, request.URL.Path, request.RemoteAddr, request.UserAgent(), request.Header.Get("Authorization") != "")
		}
	})
	h := newHarnessConfigured(t, options, moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	personal := nativeCredentials()
	team := renewableCredentials("team")
	team.SessionToken, team.SessionExpiresAt = "another-session-same-user", fixtureNow.Add(time.Hour).UnixMilli()
	for _, account := range []Credentials{personal, team} {
		frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("billing.claim", map[string]string{"planId": "gift"}), raw(account))
		if frame.Error != nil {
			t.Fatalf("billing claim: %+v", frame.Error)
		}
	}
	if claims.Load() != 1 {
		t.Fatal("Personal and Team slots of the same verified vendor user claimed twice")
	}
}

func TestLostBillingReplyIsObservedWithoutReplayAcrossGeneration(t *testing.T) {
	var claims atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/client/configs":
			io.WriteString(writer, `{"code":0,"data":{"configs":{}}}`)
		case "/api/v1/zcode-plan/billing/balance":
			plans := []any{}
			if claims.Load() > 0 {
				plans = append(plans, map[string]any{"plan_id": "gift", "user_plan_id": "new-after-baseline", "status": "active"})
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"plans": plans}})
		case "/api/v1/zcode-plan/billing/claim":
			claims.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	params := actionInput("billing.claim", map[string]string{"planId": "gift"})
	frame, _ := h.call(sdk.MethodModuleCall, scope(account), params, raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" {
		t.Fatal("lost billing reply was not fenced")
	}
	selected := scope(account)
	selected.Generation++
	frame, _ = h.call(sdk.MethodModuleCall, selected, params, raw(account))
	if frame.Error != nil || !strings.Contains(string(frame.Result), `"outcome":"observed"`) || claims.Load() != 1 {
		t.Fatalf("claim was repeated or observation was mislabeled: %+v", frame.Error)
	}
}

func TestRequiredClaimVerificationPreventsFinancialPost(t *testing.T) {
	for _, policy := range []string{
		`{"code":0,"data":{"configs":{"captcha":{"enabled":true}}}}`,
		`{"code":0,"data":{"configs":{"captcha":[]}}}`,
		`{"code":0,"data":{}}`,
	} {
		t.Run(policy, func(t *testing.T) {
			var claims atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/client/configs":
					io.WriteString(writer, policy)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
				case "/api/v1/zcode-plan/billing/claim":
					claims.Add(1)
				}
			}))
			defer server.Close()
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
			account := nativeCredentials()
			frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("billing.claim", map[string]string{"planId": "gift"}), raw(account))
			if frame.Error == nil || claims.Load() != 0 || h.accepted.Load() {
				t.Fatal("verification policy was bypassed or financial admission occurred")
			}
		})
	}
}
