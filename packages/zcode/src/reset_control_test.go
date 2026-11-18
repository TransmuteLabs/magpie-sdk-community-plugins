package zcode

import (
	"bytes"
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

func TestResetOpportunityAcceptsOnlyOfficialEnvelopes(t *testing.T) {
	for _, test := range []struct {
		name, body string
		confirmed  bool
	}{
		{"granted", `{"code":0,"data":{"granted":true}}`, true},
		{"denied", `{"code":3301,"data":{"granted":false,"next_try_at":2000000000000}}`, true},
		{"zero-false", `{"code":0,"data":{"granted":false,"next_try_at":2000000000000}}`, false},
		{"denial-true", `{"code":3301,"data":{"granted":true,"next_try_at":2000000000000}}`, false},
		{"denial-without-boundary", `{"code":3301,"data":{"granted":false}}`, false},
		{"denial-fractional-boundary", `{"code":3301,"data":{"granted":false,"next_try_at":2000000000000.5}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/coding-plan/reset/opportunity":
					posts.Add(1)
					io.WriteString(writer, test.body)
				default:
					t.Errorf("unexpected control request %s", request.URL.Path)
				}
			}))
			defer server.Close()
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
			account := nativeCredentials()
			for range 2 {
				frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("reset.opportunity", struct{}{}), raw(account))
				if test.confirmed {
					if frame.Error != nil {
						t.Fatalf("official opportunity envelope was refused: %+v", frame.Error)
					}
					if test.name == "denied" && !bytes.Contains(frame.Result, []byte(`"nextTryAt":2000000000000`)) {
						t.Fatalf("authoritative denial boundary was shortened: %s", frame.Result)
					}
				} else if frame.Error == nil || frame.Error.Outcome != "uncertain" {
					t.Fatalf("unproven opportunity envelope cleared durable uncertainty: %s %+v", frame.Result, frame.Error)
				}
			}
			if posts.Load() != 1 {
				t.Fatalf("opportunity was replayed %d times", posts.Load())
			}
		})
	}
}

func TestResetControlsReconcileAcceptedUseAfterRestartWithoutReposting(t *testing.T) {
	var uses, histories atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/coding-plan/reset/status":
			reply := resetResponse(histories.Load() != 0)
			reply["data"].(map[string]any)["private_future_field"] = "private-reset-material"
			for _, value := range reply["data"].(map[string]any)["available_five_hour_resets"].([]any) {
				value.(map[string]any)["private_future_field"] = "private-reset-material"
			}
			json.NewEncoder(writer).Encode(reply)
		case "/api/v1/coding-plan/reset/use":
			uses.Add(1)
			io.WriteString(writer, `{"code":0,"data":{"used":true}}`)
		default:
			t.Errorf("unexpected control request %s", request.URL.Path)
		}
	}))
	defer server.Close()
	config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
	account := nativeCredentials()
	first := newHarnessConfigured(t, endpointOptions(server), config)
	frame, _ := first.call(sdk.MethodModuleCall, scope(account), actionInput("reset.use", map[string]string{"window": "FIVE_HOUR"}), raw(account))
	if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"outcome":"unconfirmed"`)) {
		t.Fatalf("accepted use was falsely completed without new history: %s %+v", frame.Result, frame.Error)
	}
	histories.Store(1)
	second := newHarnessModule(t, endpointOptions(server), config, NewControlModule)
	selected := scope(account)
	selected.Generation++
	frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput("reset.summary", struct{}{}), raw(account))
	if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"pending":true`)) {
		t.Fatalf("reset debt was hidden after restart/generation change: %s %+v", frame.Result, frame.Error)
	}
	for _, secret := range []string{`"Key"`, `"Operation"`, `"Generation"`, `"idempotency_key"`, "fixture-business-token", "fixture-zcode-session",
		"private_future_field", "private-reset-material"} {
		if bytes.Contains(frame.Result, []byte(secret)) {
			t.Fatalf("reset summary leaked private journal authority: %s", frame.Result)
		}
	}
	frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput("reset.reconcile", struct{}{}), raw(account))
	if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"outcome":"observed"`)) {
		t.Fatalf("confirmed use did not reconcile its newer history: %s %+v", frame.Result, frame.Error)
	}
	frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput("reset.summary", struct{}{}), raw(account))
	if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"pending":false`)) || uses.Load() != 1 {
		t.Fatalf("read/reconcile replayed use or retained resolved debt: %s %+v uses=%d", frame.Result, frame.Error, uses.Load())
	}
}

func TestResetControlsKeepUnknownDebtUntilSeparateExplicitResolution(t *testing.T) {
	for _, kind := range []string{"use", "opportunity"} {
		t.Run(kind, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/coding-plan/reset/status":
					json.NewEncoder(writer).Encode(resetResponse(posts.Load() != 0))
				case "/api/v1/coding-plan/reset/" + kind:
					posts.Add(1)
					writer.WriteHeader(http.StatusServiceUnavailable)
				default:
					t.Errorf("unexpected control request %s", request.URL.Path)
				}
			}))
			defer server.Close()
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			first := newHarnessModule(t, endpointOptions(server), config, NewControlModule)
			input := map[string]string{}
			if kind == "use" {
				input["window"] = "FIVE_HOUR"
			}
			frame, _ := first.call(sdk.MethodModuleCall, scope(account), actionInput("reset."+kind, input), raw(account))
			if frame.Error == nil || frame.Error.Outcome != "uncertain" {
				t.Fatal("lost mutation response was not fenced")
			}
			second := newHarnessModule(t, endpointOptions(server), config, NewControlModule)
			selected := scope(account)
			selected.Generation++
			frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput("reset.reconcile", struct{}{}), raw(account))
			if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"outcome":"unconfirmed"`)) {
				t.Fatalf("observed history erased unknown debt: %s %+v", frame.Result, frame.Error)
			}
			action, acknowledgment := "reset.resolve", "vendor-may-have-spent-reset"
			if kind == "opportunity" {
				action, acknowledgment = "reset.opportunity.resolve", "vendor-may-have-granted-reset"
			}
			frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput(action, map[string]string{"acknowledgment": "retry"}), raw(account))
			if frame.Error == nil || frame.Error.Outcome != "not-started" {
				t.Fatal("resolution accepted an implicit retry acknowledgment")
			}
			frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput(action, map[string]string{"acknowledgment": acknowledgment}), raw(account))
			if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"replayed":false`)) || posts.Load() != 1 {
				t.Fatalf("explicit resolution failed or replayed mutation: %s %+v posts=%d", frame.Result, frame.Error, posts.Load())
			}
			frame, _ = second.call(sdk.MethodModuleCall, selected, actionInput("reset.summary", struct{}{}), raw(account))
			if frame.Error != nil || !bytes.Contains(frame.Result, []byte(`"pending":false`)) ||
				!bytes.Contains(frame.Result, []byte(`"opportunityPending":false`)) {
				t.Fatalf("resolution did not clear only acknowledged debt: %s %+v", frame.Result, frame.Error)
			}
			if kind == "opportunity" && !strings.Contains(string(frame.Result), `"nextTryAt":`+string(raw(fixtureNow.Add(5*time.Minute).UnixMilli()))) {
				t.Fatalf("opportunity resolution dropped its existing cooldown: %s", frame.Result)
			}
		})
	}
}
