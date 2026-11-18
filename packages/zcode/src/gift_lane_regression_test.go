package zcode

import (
	"encoding/json"
	sdk "github.com/yetone/magpie/pkg/extension"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFullPackageGiftFallbackQuotaUnits(t *testing.T) {
	for _, tc := range []struct {
		name             string
		total, remaining float64
		want             int32
	}{
		{"exhausted_1000", 1000, 0, 0}, {"half_available_20", 20, 10, 1}, {"percentage_only", 0, -1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, upstream := newGiftFixture(t, `{"code":1005,"msg":"exceed quota limit"}`, 200)
			quota := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/quota/zai" {
					upstream.Config.Handler.ServeHTTP(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				limit := map[string]any{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 50}
				if tc.remaining >= 0 {
					limit["usage"] = tc.total
					limit["remaining"] = tc.remaining
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"limits": []any{limit}}})
			}))
			defer quota.Close()
			opts := endpointOptions(quota)
			h := newHarnessModule(t, opts, moduleConfig{}, NewControlModule)
			account := giftAccount()
			frame, body := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			t.Logf("full API9 total=%v remaining=%v gift=%d paid=%d expectedPaid=%d error=%+v body=%s", tc.total, tc.remaining, fixture.start.Load(), fixture.paid.Load(), tc.want, frame.Error, body)
			if frame.Error != nil || fixture.start.Load() != 1 {
				t.Fatalf("ordinary gift dispatch failed: %+v", frame.Error)
			}
			if fixture.paid.Load() != tc.want {
				t.Errorf("paid quota admission mixed percentage and capacity: paid=%d want=%d", fixture.paid.Load(), tc.want)
			}
		})
	}
}

func TestAbandonDuringOrdinaryTicketPoll(t *testing.T) {
	for _, state := range []string{"ready", "active"} {
		t.Run(state, func(t *testing.T) { abandonDuringTicketPoll(t, state) })
	}
}

func abandonDuringTicketPoll(t *testing.T, returnedState string) {
	t.Helper()
	polled, release := make(chan struct{}), make(chan struct{})
	var once sync.Once

	var inferred atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/oauth/profile":
			profile(w, r, "fixture-user")
		case "/api/v1/off-peak/ticket":
			io.WriteString(w, `{"ticket_id":"audit-ticket","task_id":"fixture-session","state":"queued"}`)
		case "/api/v1/off-peak/ticket/status":
			close(polled)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"tickets": []any{map[string]any{"ticket_id": "audit-ticket", "task_id": "fixture-session", "state": returnedState}}})
		case "/api/v1/off-peak/anthropic/v1/messages":
			inferred.Add(1)
			io.WriteString(w, `{"type":"message","content":[]}`)
		default:
			t.Errorf("unexpected fixture path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer func() { once.Do(func() { close(release) }); server.Close() }()
	cfg := moduleConfig{DataDir: filepath.Join(t.TempDir(), "controller")}
	execute := newHarnessModule(t, endpointOptions(server), cfg, NewControlModule)
	controls := newHarnessModule(t, endpointOptions(server), cfg, NewControlModule)
	account := nativeCredentials()
	done := make(chan sdk.Frame, 1)
	go func() {
		f, _ := execute.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-OffPeak"), raw(account))
		done <- f
	}()
	select {
	case <-polled:
	case <-time.After(5 * time.Second):
		t.Fatal("poll was not entered")
	}
	f, _ := controls.call(sdk.MethodModuleCall, scope(account), actionInput("execution.list", struct{}{}), raw(account))
	var entries []executionView
	if f.Error != nil || json.Unmarshal(f.Result, &entries) != nil || len(entries) != 1 {
		t.Fatalf("list: %+v %s", f.Error, f.Result)
	}
	f, _ = controls.call(sdk.MethodModuleCall, scope(account), actionInput("execution.abandon", map[string]any{"reference": nativeReference{Reference: entries[0].Reference}, "acknowledgment": "vendor-operation-may-have-succeeded"}), raw(account))
	var abandoned executionView
	if f.Error != nil || json.Unmarshal(f.Result, &abandoned) != nil || abandoned.State != "abandoned" {
		t.Fatalf("abandon: %+v %s", f.Error, f.Result)
	}
	t.Logf("ordinary control action persisted state=%s before poll returned", abandoned.State)
	once.Do(func() { close(release) })
	select {
	case f = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("execute did not finish")
	}
	t.Logf("execute after abandon: error=%+v inferenceCount=%d", f.Error, inferred.Load())
	f, _ = controls.call(sdk.MethodModuleCall, scope(account), actionInput("execution.list", struct{}{}), raw(account))
	if f.Error != nil || json.Unmarshal(f.Result, &entries) != nil || len(entries) != 1 {
		t.Fatalf("final list: %+v", f.Error)
	}
	t.Logf("final state=%s", entries[0].State)
	if inferred.Load() != 0 || entries[0].State != "abandoned" {
		t.Errorf("terminal abandonment was revived: state=%s inferenceCount=%d", entries[0].State, inferred.Load())
	}
}
