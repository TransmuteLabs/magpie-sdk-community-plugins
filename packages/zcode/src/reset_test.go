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

func resetResponse(spent bool) map[string]any {
	available := []any{}
	var latest any
	if !spent {
		available = append(available, map[string]any{"expire_at": fixtureNow.Add(time.Hour).UnixMilli()})
	} else {
		latest = map[string]any{"used_at": fixtureNow.Add(time.Second).UnixMilli()}
	}
	return map[string]any{"code": 0, "data": map[string]any{
		"available_five_hour_resets": available, "available_week_resets": []any{},
		"latest_five_hour_reset_history": latest, "latest_week_reset_history": nil,
		"has_unread_history": spent,
	}}
}

func TestResetRequiresNewerHistoryAfterConfirmedUse(t *testing.T) {
	var uses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.RequestURI() == "/" &&
			request.ContentLength == 0 && len(request.TransferEncoding) == 0 &&
			request.Header.Get("Authorization") == "" && request.Header.Get("X-Bigmodel-Authorization") == "" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/coding-plan/reset/status":
			json.NewEncoder(writer).Encode(resetResponse(uses.Load() != 0))
		case "/api/v1/coding-plan/reset/use":
			if request.Header.Get("X-Bigmodel-Authorization") != "fixture-business-token" ||
				request.Header.Get("Authorization") != "Bearer fixture-zcode-session" {
				t.Error("reset used PAT or wrong business authorization form")
			}
			var body map[string]string
			json.NewDecoder(request.Body).Decode(&body)
			if body["reset_type"] != "FIVE_HOUR" || body["idempotency_key"] == "" || len(body["idempotency_key"]) > 64 {
				t.Error("reset parameters are not exact or durably keyed")
			}
			uses.Add(1)
			io.WriteString(writer, `{"code":0,"data":{"used":true}}`)
		default:
			t.Error("unexpected reset target")
		}
	}))
	defer server.Close()
	probe, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	probe.Body.Close()
	if probe.StatusCode != http.StatusNotFound || uses.Load() != 0 {
		t.Fatalf("probe counted as reset: status %d, %d uses", probe.StatusCode, uses.Load())
	}
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	params := actionInput("reset.use", map[string]string{"window": "FIVE_HOUR"})
	frame, _ := h.call(sdk.MethodModuleCall, scope(account), params, raw(account))
	if frame.Error != nil || !strings.Contains(string(frame.Result), `"outcome":"observed"`) || uses.Load() != 1 {
		t.Fatalf("reset did not reconcile its high-water history: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodModuleCall, scope(account), params, raw(account))
	if frame.Error == nil || uses.Load() != 1 {
		t.Fatal("exhausted reset opportunity was spent again")
	}
}

func TestUnknownResetIsNotRepeatedEvenWhenHistoryAdvances(t *testing.T) {
	var uses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/coding-plan/reset/status":
			json.NewEncoder(writer).Encode(resetResponse(uses.Load() != 0))
		case "/api/v1/coding-plan/reset/use":
			uses.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	params := actionInput("reset.use", map[string]string{"window": "FIVE_HOUR"})
	frame, _ := h.call(sdk.MethodModuleCall, scope(account), params, raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" {
		t.Fatal("lost reset reply was not treated as unknown")
	}
	selected := scope(account)
	selected.Generation++
	frame, _ = h.call(sdk.MethodModuleCall, selected, params, raw(account))
	if frame.Error != nil || !strings.Contains(string(frame.Result), `"outcome":"unconfirmed"`) || uses.Load() != 1 {
		t.Fatalf("unknown reset was replayed or falsely confirmed: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodModuleCall, selected,
		actionInput("reset.resolve", map[string]string{"acknowledgment": "vendor-may-have-spent-reset"}), raw(account))
	if frame.Error != nil || uses.Load() != 1 {
		t.Fatal("explicit uncertainty acknowledgment replayed a reset")
	}
}

func TestResetOpportunityCooldownAndUnknownResultNeverRepost(t *testing.T) {
	for _, mode := range []string{"granted", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			var opportunities atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/coding-plan/reset/opportunity":
					opportunities.Add(1)
					if mode == "unknown" {
						writer.WriteHeader(http.StatusServiceUnavailable)
					} else {
						io.WriteString(writer, `{"code":0,"data":{"granted":true}}`)
					}
				default:
					t.Error("unexpected opportunity target")
				}
			}))
			defer server.Close()
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
			account := nativeCredentials()
			for range 2 {
				frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("reset.opportunity", struct{}{}), raw(account))
				if mode == "unknown" && (frame.Error == nil || frame.Error.Outcome != "uncertain") {
					t.Fatal("unknown opportunity was not fenced")
				}
				if mode == "granted" && frame.Error != nil {
					t.Fatalf("known opportunity: %+v", frame.Error)
				}
			}
			if opportunities.Load() != 1 {
				t.Fatal("opportunity was posted twice inside its cooldown or unknown fence")
			}
		})
	}
}

func TestResetHistoryAcknowledgmentUsesGlobalUserHeaders(t *testing.T) {
	var acknowledgments atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/coding-plan/reset/status":
			reply := resetResponse(true)
			if acknowledgments.Load() != 0 {
				reply["data"].(map[string]any)["has_unread_history"] = false
			}
			json.NewEncoder(writer).Encode(reply)
		case "/api/v1/coding-plan/reset/history/read":
			if request.Header.Get("Bigmodel-Target-Type") != "" || request.Header.Get("Bigmodel-Organization") != "" ||
				request.Header.Get("X-Bigmodel-Authorization") != "fixture-business-token" {
				t.Error("global history acknowledgment used selection-targeted headers")
			}
			acknowledgments.Add(1)
			io.WriteString(writer, `{"code":0}`)
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	for range 2 {
		frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("reset.ack", struct{}{}), raw(account))
		if frame.Error != nil {
			t.Fatalf("history acknowledgment: %+v", frame.Error)
		}
	}
	if acknowledgments.Load() != 1 {
		t.Fatal("already-read history was acknowledged again")
	}
}
