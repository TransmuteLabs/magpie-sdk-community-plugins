package zcode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestQuotaReadHoldsPrincipalJournalUntilVendorObservationCompletes(t *testing.T) {
	reading, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var quotas, balances atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.EscapedPath() {
		case "GET /api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "GET /quota/zai":
			if quotas.Add(1) != 1 {
				t.Error("duplicate quota observation")
				writer.WriteHeader(http.StatusConflict)
				return
			}
			close(reading)
			<-release
			io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":20}]}}`)
		case "GET /api/v1/zcode-plan/billing/balance":
			balances.Add(1)
			io.WriteString(writer, `{"code":0,"data":{"plans":[],"balances":[]}}`)
		default:
			t.Errorf("unexpected quota fixture %s %s", request.Method, request.URL.EscapedPath())
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	defer once.Do(func() { close(release) })
	path := filepath.Join(t.TempDir(), "controller")
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: path})
	account := nativeCredentials()
	done := make(chan sdk.Frame, 1)
	go func() {
		frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{Revision: "1"}, raw(account))
		done <- frame
	}()
	select {
	case <-reading:
	case <-time.After(3 * time.Second):
		t.Fatal("quota fixture was not reached")
	}
	store, err := controller.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Millisecond)
	err = store.Locked(ctx, controller.Principal(server.URL, account.UserID), func(*controller.Transaction) error {
		return nil
	})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("an entitlement writer could enter during the vendor quota observation: %v", err)
	}
	once.Do(func() { close(release) })
	select {
	case frame := <-done:
		if frame.Error != nil {
			t.Fatalf("serialized quota read failed: %+v", frame.Error)
		}
		if quotas.Load() != 1 || balances.Load() != 1 {
			t.Fatalf("got %d quota and %d balance requests, want one each", quotas.Load(), balances.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quota reader did not release its principal journal")
	}
}

func TestUnsettledResetDefersUsageAcrossModuleInstances(t *testing.T) {
	var writes, quotas atomic.Int32
	var invalidated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/coding-plan/reset/status":
			json.NewEncoder(writer).Encode(resetResponse(false))
		case "/api/v1/coding-plan/reset/use":
			if !invalidated.Load() {
				t.Error("reset POST preceded host invalidation")
			}
			writes.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		case "/quota/zai":
			quotas.Add(1)
			io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":20}]}}`)
		case "/api/v1/zcode-plan/billing/balance":
			io.WriteString(writer, `{"code":0,"data":{"plans":[],"balances":[]}}`)
		default:
			t.Errorf("unexpected quota-barrier fixture %s", request.URL.Path)
		}
	}))
	defer server.Close()
	config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "controller")}
	account := nativeCredentials()
	first := newHarnessConfigured(t, endpointOptions(server), config)
	first.onInvalidation = func() { invalidated.Store(true) }
	frame, _ := first.call(sdk.MethodModuleCall, scope(account),
		actionInput("reset.use", map[string]string{"window": "FIVE_HOUR"}), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" || first.invalidations != 1 || writes.Load() != 1 {
		t.Fatal("lost reset fixture did not invalidate before its single vendor write")
	}
	second := newHarnessConfigured(t, endpointOptions(server), config)
	for range 2 {
		frame, _ = second.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{Revision: "1"}, raw(account))
		var report sdk.UsageResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &report) != nil || !report.ReadDeferred ||
			report.Revision != "1" || quotas.Load() != 0 {
			t.Fatalf("a second controller acknowledged usage while reset remained unsettled: %+v", frame.Error)
		}
	}
	frame, _ = second.call(sdk.MethodModuleCall, scope(account),
		actionInput("reset.resolve", map[string]string{"acknowledgment": "vendor-may-have-spent-reset"}), raw(account))
	if frame.Error != nil {
		t.Fatalf("explicit reset resolution: %+v", frame.Error)
	}
	frame, _ = second.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{Revision: "1"}, raw(account))
	var report sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &report) != nil || report.ReadDeferred ||
		report.Revision != "1" || quotas.Load() != 1 || writes.Load() != 1 {
		t.Fatal("settled journal did not permit a fresh post-resolution quota read")
	}
}

func TestUnknownSharedClaimDefersAnotherSelectedAccount(t *testing.T) {
	var balances, claims atomic.Int32
	var invalidated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/zcode-plan/billing/balance":
			balances.Add(1)
			io.WriteString(writer, `{"code":0,"data":{"plans":[],"balances":[]}}`)
		case "/api/v1/client/configs":
			io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
		case "/api/v1/zcode-plan/billing/claim":
			if !invalidated.Load() {
				t.Error("claim POST preceded host invalidation")
			}
			claims.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected shared-claim fixture %s", request.URL.Path)
		}
	}))
	defer server.Close()
	config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "controller")}
	first := newHarnessConfigured(t, endpointOptions(server), config)
	first.onInvalidation = func() { invalidated.Store(true) }
	account := nativeCredentials()
	frame, _ := first.call(sdk.MethodModuleCall, scope(account),
		actionInput("billing.claim", map[string]string{"planId": "gift"}), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" || first.invalidations != 1 || claims.Load() != 1 {
		t.Fatal("unknown shared claim was not fenced and invalidated")
	}
	second := newHarnessConfigured(t, endpointOptions(server), config)
	other := fixtureCredentials("start")
	frame, _ = second.call(sdk.MethodProviderUsage, scope(other), sdk.UsageRequest{Revision: "2"}, raw(other))
	var report sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &report) != nil || !report.ReadDeferred ||
		report.Revision != "2" || balances.Load() != 1 || claims.Load() != 1 {
		t.Fatal("a different selection acknowledged usage while the shared vendor claim remained unsettled")
	}
}

func TestResetCannotDispatchWithoutConfirmedInvalidation(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/coding-plan/reset/status":
			json.NewEncoder(writer).Encode(resetResponse(false))
		case "/api/v1/coding-plan/reset/use":
			writes.Add(1)
			t.Error("reset mutation escaped failed host invalidation")
		default:
			t.Errorf("unexpected invalidation fixture %s", request.URL.Path)
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "controller")})
	h.invalidationError = true
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodModuleCall, scope(account),
		actionInput("reset.use", map[string]string{"window": "FIVE_HOUR"}), raw(account))
	if frame.Error == nil || h.invalidations != 1 || writes.Load() != 0 {
		t.Fatal("host invalidation failure did not prevent a vendor POST")
	}
}

func TestStartQuotaSelectsBestBucketByModelWithoutInventingCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"owned","status":"active"}],"balances":[
			{"plan_id":"owned","remaining_units":2,"total_units":10,"capabilities":["model:GLM-5.3","model:GLM-5.3"]},
			{"plan_id":"owned","remaining_units":3,"total_units":10,"capabilities":["model:GLM-5.3"]},
			{"plan_id":"owned","remaining_units":0,"capabilities":["model:GLM-5.2"]},
			{"plan_id":"owned","remaining_units":1,"capabilities":["model:GLM-5-Turbo"]}
		]}}`)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("start")
	frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{Revision: "12"}, raw(account))
	var report sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &report) != nil || report.Revision != "12" || len(report.Windows) != 4 {
		t.Fatalf("Start quota: %+v %+v", frame.Error, report)
	}
	windows := map[string]sdk.UsageWindow{}
	aside := 0
	for _, window := range report.Windows {
		if window.Aside {
			aside++
			if len(window.Models) != 0 || window.Used != 80 {
				t.Fatal("redundant bucket became model allowance")
			}
			continue
		}
		if len(window.Models) != 1 || len(window.NotModels) != 0 {
			t.Fatal("Start bucket quota lost exact model applicability")
		}
		windows[window.Models[0]] = window
	}
	if aside != 1 || windows["glm-5.3"].Used != 70 || windows["glm-5.3"].Unknown ||
		windows["glm-5.2"].Used != 100 || windows["glm-5.2"].Unknown ||
		!windows["glm-5-turbo"].Unknown {
		t.Fatal("Start bucket projection duplicated capacity or invented an unknown total")
	}
}
