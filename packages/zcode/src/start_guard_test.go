package zcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestStartGuardCanceledQueueReleasesExactlyOnceAndNarrowsBeforeSharedHold(t *testing.T) {
	now := fixtureNow
	guard := &startGuard{lanes: map[string]*startQueue{}, routes: map[string]*startRouteHold{}, now: func() time.Time { return now }}
	releases := []func(){}
	for range 3 {
		release, err := guard.acquire(t.Context(), "account", "route")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
		t.Cleanup(release)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := guard.acquire(ctx, "account", "route"); err == nil {
		t.Fatal("canceled queued request gained a slot")
	}
	guard.sustained("account", "route")
	if guard.laneLimit(guard.lanes["account"]) != 2 || guard.held("route") {
		t.Fatal("local saturation armed shared hold instead of narrowing")
	}
	releases[0]()
	releases[0]()
	guard.sustained("account", "route")
	if guard.laneLimit(guard.lanes["account"]) != 1 {
		t.Fatal("adaptive cap did not narrow to one")
	}
	releases[1]()
	for range 5 {
		guard.sustained("account", "route")
	}
	if !guard.held("route") || guard.held("another-proxy") {
		t.Fatal("sustained lone caps missed or leaked outbound-route hold")
	}
	now = now.Add(6 * time.Minute)
	if guard.held("route") || guard.laneLimit(guard.lanes["account"]) != 3 {
		t.Fatal("hold or adaptive cap did not expire")
	}
}

func TestStartGuardNeverRetriesPartialStreamOrTransportUncertainty(t *testing.T) {
	for _, scenario := range []string{"partial", "transport"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/zcode-plan/billing/balance" && request.Method == http.MethodGet {
					writer.Header().Set("Content-Type", "application/json")
					io.WriteString(writer, confirmedStartGLM52)
					return
				}
				if request.URL.Path != "/start" || request.Method != http.MethodPost {
					t.Error("unexpected endpoint in uncertain Start fixture")
					http.NotFound(writer, request)
					return
				}
				calls.Add(1)
				if scenario == "transport" {
					connection, _, err := writer.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					connection.Close()
					return
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"content\":[]}}\n\n")
			}))
			t.Cleanup(server.Close)
			h := newHarness(t, endpointOptions(server))
			account := fixtureCredentials("start")
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.2"), raw(account))
			if frame.Error == nil || frame.Error.Outcome != "uncertain" || calls.Load() != 1 {
				t.Fatalf("partial/transport result retried or lost uncertainty: %+v calls=%d", frame.Error, calls.Load())
			}
		})
	}
}
func TestStartGuardLimitsConcurrentRequestsForOneAccount(t *testing.T) {
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/zcode-plan/billing/balance" && request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, confirmedStartGLM52)
			return
		}
		if request.URL.Path != "/start" || request.Method != http.MethodPost {
			t.Error("unexpected endpoint in parallel Start fixture")
			http.NotFound(writer, request)
			return
		}
		count := active.Add(1)
		for old := peak.Load(); count > old && !peak.CompareAndSwap(old, count); old = peak.Load() {
		}
		defer active.Add(-1)
		time.Sleep(40 * time.Millisecond)
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"type":"message","content":[]}`)
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("start")
	h.conn.SetDeadline(time.Now().Add(10 * time.Second))
	sent := make(chan error, 1)
	go func() {
		for index := range 9 {
			selected := scope(account)
			selected.Plugin, selected.Operation = ProviderID, fmt.Sprintf("parallel-%d", index)
			if err := h.send(sdk.Frame{Kind: sdk.KindCall, ID: selected.Operation, Method: sdk.MethodProviderExecute,
				Scope: selected, Auth: raw(account), Params: raw(giftRequest("GLM-5.2"))}); err != nil {
				sent <- err
				return
			}
		}
		sent <- nil
	}()
	results := 0
	for results < 9 {
		var frame sdk.Frame
		if err := h.read.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		switch frame.Kind {
		case sdk.KindHostCall:
			if frame.Method != sdk.MethodNetworkProxy {
				t.Fatal("unexpected callback", frame.Method)
			}
			if err := h.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent,
				Result: raw(sdk.NetworkProxyResult{})}); err != nil {
				t.Fatal(err)
			}
		case sdk.KindEvent:
			if err := h.send(sdk.Frame{Kind: sdk.KindAck, ID: frame.ID, Seq: frame.Seq}); err != nil {
				t.Fatal(err)
			}
		case sdk.KindResult:
			if frame.Error != nil {
				t.Fatalf("concurrent Start failed: %+v", frame.Error)
			}
			results++
		default:
			t.Fatal("unexpected frame", frame.Kind)
		}
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("Start gate peak = %d, want 2..3", peak.Load())
	}
}

func TestStartGuardRetriesOnlyKnownUnusedConcurrencyCaps(t *testing.T) {
	for _, code := range []int{3008, 3009, 3010, 1005, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/zcode-plan/billing/balance" && request.Method == http.MethodGet {
					writer.Header().Set("Content-Type", "application/json")
					io.WriteString(writer, confirmedStartGLM52)
					return
				}
				if request.URL.Path != "/start" || request.Method != http.MethodPost {
					t.Error("unexpected endpoint in Start refusal fixture")
					http.NotFound(writer, request)
					return
				}
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(429)
				json.NewEncoder(writer).Encode(map[string]any{"code": code, "msg": "synthetic refusal"})
			}))
			t.Cleanup(server.Close)
			h := newHarness(t, endpointOptions(server))
			h.recoverRequest = func(failure sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult {
				decision := sdk.RequestRecoveryResult{Step: failure.Step, Action: "terminal"}
				if failure.Evidence != "" && (code == 3008 || code == 3009 || code == 3010) && len(h.recoveries) < 3 {
					decision.Action, decision.Retry = "wait", true
				}
				return decision
			}
			account := fixtureCredentials("start")
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.2"), raw(account))
			want := int32(1)
			if code == 3008 || code == 3009 || code == 3010 {
				want = 3
			}
			if frame.Error != nil || calls.Load() != want {
				t.Fatalf("busy retry boundary %d: calls=%d error=%+v", code, calls.Load(), frame.Error)
			}
		})
	}
}

func TestStartGuardEdgeHoldSpansAccountsButLeavesPaidEndpointAlone(t *testing.T) {
	var starts, paid atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/zcode-plan/billing/balance" && request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, confirmedStartGLM52)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/start" && request.URL.Path != "/zai" {
			t.Error("unexpected endpoint in shared-route hold fixture")
			http.NotFound(writer, request)
			return
		}
		if request.URL.Path == "/start" {
			starts.Add(1)
			writer.Header().Set("Content-Type", "text/html")
			writer.WriteHeader(405)
			io.WriteString(writer, `<html>errors.aliyun.com synthetic firewall</html>`)
			return
		}
		paid.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"type":"message","content":[]}`)
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, endpointOptions(server))
	first := fixtureCredentials("start")
	h.call(sdk.MethodProviderExecute, scope(first), giftRequest("GLM-5.2"), raw(first))
	second := first
	second.UserID = "another-user"
	h.call(sdk.MethodProviderExecute, scope(second), giftRequest("GLM-5.2"), raw(second))
	account := fixtureCredentials("individual")
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if starts.Load() != 1 || paid.Load() != 1 || frame.Error != nil {
		t.Fatalf("shared-route edge hold leaked or missed: starts=%d paid=%d %+v", starts.Load(), paid.Load(), frame.Error)
	}
}
