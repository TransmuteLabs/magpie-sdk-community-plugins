package zcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func startAddressHolders(t *testing.T, guard *startGuard, route string, count int) []func() {
	t.Helper()
	releases := make([]func(), 0, count)
	for index := range count {
		release, err := guard.acquire(t.Context(), fmt.Sprintf("holder-%d", index), route)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
		t.Cleanup(release)
	}
	return releases
}

func TestDonor0168StartAddressLimitsSixAcrossMoreThanTwoAccounts(t *testing.T) {
	guard := &startGuard{lanes: map[string]*startQueue{}, routes: map[string]*startRouteHold{}, now: time.Now}
	releases := startAddressHolders(t, guard, "origin-a/proxy-a", 6)
	release, err := guard.waitFor(t.Context(), "seventh", "origin-a/proxy-a", false, time.Now().Add(25*time.Millisecond))
	if err == nil {
		release()
		t.Fatal("seventh Start transport acquired an already full six-request route")
	}
	for _, route := range []string{"origin-a/proxy-b", "origin-b/proxy-a"} {
		other, err := guard.acquire(t.Context(), "seventh", route)
		if err != nil {
			t.Fatal("Start route capacity leaked across origin/proxy identity:", err)
		}
		other()
	}
	releases[0]()
	releases[0]()
	next, err := guard.acquire(t.Context(), "seventh", "origin-a/proxy-a")
	if err != nil {
		t.Fatal("released shared route slot was not reusable:", err)
	}
	next()
	for _, release := range releases {
		release()
	}
	guard.mu.Lock()
	defer guard.mu.Unlock()
	for _, lane := range guard.lanes {
		if lane.active != 0 || len(lane.waiters) != 0 {
			t.Fatal("route timeout/idempotent release leaked account or route reservations")
		}
	}
}

func TestDonor0168StartAddressCancellationReleasesBothSlots(t *testing.T) {
	guard := &startGuard{lanes: map[string]*startQueue{}, routes: map[string]*startRouteHold{}, now: time.Now}
	releases := startAddressHolders(t, guard, "route", 6)
	ctx, cancel := context.WithCancel(t.Context())
	type result struct {
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() {
		release, err := guard.acquire(ctx, "canceled", "route")
		done <- result{release, err}
	}()
	defer func() {
		cancel()
		outcome := <-done
		if outcome.release != nil {
			outcome.release()
		}
		if outcome.err != context.Canceled {
			t.Error("canceled route waiter gained transport authority:", outcome.err)
		}
	}()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	queued := false
	for !queued {
		guard.mu.Lock()
		for _, lane := range guard.lanes {
			queued = queued || len(lane.waiters) == 1
		}
		guard.mu.Unlock()
		select {
		case <-deadline.C:
			t.Fatal("seventh account never joined the shared Start route queue")
		default:
			if !queued {
				time.Sleep(time.Millisecond)
			}
		}
	}
	cancel()
	for _, release := range releases {
		release()
	}
}

func TestDonor0168FreeFirstUsesPaidWhenStartAccountOrRouteFull(t *testing.T) {
	for _, test := range []struct {
		name string
		hold int
		same bool
		want int32
	}{
		{"route-full", 6, false, 1},
		{"account-full", 3, true, 1},
		{"route-room", 5, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
			account := giftAccount()
			factory := func(options ModuleOptions) sdk.Module {
				component := NewControlModule(options)
				previous := component.Handlers[sdk.MethodProviderExecute]
				component.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
					state, err := currentInstance(ctx)
					if err != nil {
						return nil, err
					}
					owner := &module{options: options}
					state.startGuard.wait = 25 * time.Millisecond
					route := controller.Principal(owner.zcodeOrigin(), "")
					for index := range test.hold {
						key := fmt.Sprintf("holder-%d", index)
						if test.same {
							key = controller.Principal(owner.zcodeOrigin(), account.UserID)
						}
						release, err := state.startGuard.acquire(ctx, key, route)
						if err != nil {
							return nil, err
						}
						defer release()
					}
					return previous(ctx, call, stream)
				}
				return component
			}
			harness := newHarnessModule(t, endpointOptions(server), moduleConfig{}, factory)
			frame, _ := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			if frame.Error != nil || fixture.paid.Load() != test.want || fixture.start.Load() != 1-test.want {
				t.Fatalf("free-first ignored current Start capacity: error=%+v paid=%d start=%d", frame.Error, fixture.paid.Load(), fixture.start.Load())
			}
		})
	}
}

func TestDonor0168ForcedTrialAndManualStartNeverBypassToPaid(t *testing.T) {
	for _, test := range []struct {
		manual bool
		queue  string
	}{
		{false, ""}, {false, "account"}, {false, "outbound route"},
		{true, ""}, {true, "account"}, {true, "outbound route"},
	} {
		t.Run(fmt.Sprintf("manual-%t-queue-%s", test.manual, test.queue), func(t *testing.T) {
			fixture, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
			account := giftAccount()
			model := "GLM-5.3-Trial"
			if test.manual {
				account = fixtureCredentials("start")
				account.SessionToken = "fixture-zcode-session"
				model = "GLM-5.3"
			}
			factory := func(options ModuleOptions) sdk.Module {
				component := NewControlModule(options)
				previous := component.Handlers[sdk.MethodProviderExecute]
				component.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
					state, err := currentInstance(ctx)
					if err != nil {
						return nil, err
					}
					state.startGuard.wait = 25 * time.Millisecond
					owner := &module{options: options}
					route := controller.Principal(owner.zcodeOrigin(), "")
					holders := map[string]int{"account": 3, "outbound route": 6}[test.queue]
					for index := range holders {
						key := fmt.Sprintf("holder-%d", index)
						if test.queue == "account" {
							key = controller.Principal(owner.zcodeOrigin(), account.UserID)
						}
						release, err := state.startGuard.acquire(ctx, key, route)
						if err != nil {
							return nil, err
						}
						defer release()
					}
					return previous(ctx, call, stream)
				}
				return component
			}
			harness := newHarnessModule(t, endpointOptions(server), moduleConfig{}, factory)
			frame, body := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest(model), raw(account))
			if test.queue == "" {
				if frame.Error != nil || fixture.start.Load() != 1 || fixture.paid.Load() != 0 || !harness.accepted.Load() {
					t.Fatalf("valid forced Start fixture did not send with room: error=%+v paid=%d start=%d accepted=%t",
						frame.Error, fixture.paid.Load(), fixture.start.Load(), harness.accepted.Load())
				}
				return
			}
			var result sdk.ExecuteResult
			if err := json.Unmarshal(frame.Result, &result); frame.Error != nil || err != nil ||
				result.Status != http.StatusTooManyRequests || result.Outcome != "not-started" || result.Refusal == nil {
				t.Fatalf("Start queue did not return a typed 429: result=%+v frame error=%+v decode error=%v", result, frame.Error, err)
			}
			refusal := result.Refusal
			if refusal.Step != "start-messages" || refusal.Evidence != "rejected-before-effect" ||
				refusal.Facts.Status != http.StatusTooManyRequests || !refusal.Facts.Complete ||
				refusal.Facts.Operation != sdk.MethodProviderExecute || refusal.Facts.Phase != "admission" ||
				refusal.Facts.RetryAfterMS == nil || *refusal.Facts.RetryAfterMS != 5000 ||
				!strings.Contains(refusal.Facts.Message, "Start "+test.queue+" queue") ||
				len(refusal.Rules) != 0 || len(refusal.Windows) != 0 {
				t.Fatalf("Start queue lost local refusal facts or retry timing: %+v", refusal)
			}
			if harness.accepted.Load() || fixture.paid.Load() != 0 || fixture.start.Load() != 0 ||
				len(harness.headers) != 0 || len(harness.recoveries) != 0 || len(body) != 0 {
				t.Fatalf("Start queue timeout dispatched, accepted, emitted output, or requested recovery: result=%+v", result)
			}
		})
	}
}

func TestDonor0168AcceptedGiftQueueTimeoutKeepsReplayFence(t *testing.T) {
	fixture, server := newGiftFixture(t, `{"type":"message","content":[]}`, http.StatusOK)
	account := giftAccount()
	options := endpointOptions(server)
	guard := make(chan *startGuard, 1)
	factory := func(options ModuleOptions) sdk.Module {
		component := NewControlModule(options)
		previous := component.Handlers[sdk.MethodProviderExecute]
		component.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
			state, err := currentInstance(ctx)
			if err != nil {
				return nil, err
			}
			state.startGuard.wait = 25 * time.Millisecond
			guard <- state.startGuard
			return previous(ctx, call, stream)
		}
		return component
	}
	harness := newHarnessModule(t, options, moduleConfig{}, factory)
	startProxies := 0
	harness.beforeProxy = func(request sdk.NetworkProxyRequest) {
		if request.URL != options.Endpoints.Start {
			return
		}
		startProxies++
		if startProxies == 2 {
			// Saturate egress after free-first checked availability, before Start dispatch.
			owner := &module{options: options}
			startAddressHolders(t, <-guard, controller.Principal(owner.zcodeOrigin(), ""), 6)
		}
	}
	frame, _ := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if startProxies != 2 || !harness.accepted.Load() || fixture.start.Load() != 0 || fixture.paid.Load() != 0 || len(harness.recoveries) != 0 {
		t.Fatalf("accepted gift did not time out before dispatch: proxies=%d accepted=%t start=%d paid=%d recoveries=%d error=%+v",
			startProxies, harness.accepted.Load(), fixture.start.Load(), fixture.paid.Load(), len(harness.recoveries), frame.Error)
	}
	var result sdk.ExecuteResult
	if err := json.Unmarshal(frame.Result, &result); frame.Error != nil || err != nil ||
		result.Outcome != "completed" || result.Refusal != nil ||
		result.Status != http.StatusTooManyRequests || result.Headers.Get("Retry-After") != "5" {
		t.Fatalf("accepted gift lost its queue result or admitted replay: result=%+v frame error=%+v decode error=%v", result, frame.Error, err)
	}
}

func TestDonor0168StartRouteFrontRequeuePreservesFIFOAndDeadline(t *testing.T) {
	guard := &startGuard{lanes: map[string]*startQueue{}, routes: map[string]*startRouteHold{}, now: time.Now}
	releases := startAddressHolders(t, guard, "route", 6)
	ctx, cancel := context.WithCancel(t.Context())
	type result struct {
		name    string
		release func()
		err     error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		workers.Wait()
		for len(results) > 0 {
			if outcome := <-results; outcome.release != nil {
				outcome.release()
			}
		}
	})
	wait := func(name string, front bool) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			release, err := guard.waitFor(ctx, name, "route", front, time.Now().Add(5*time.Second))
			results <- result{name, release, err}
		}()
	}
	queued := func(count int) {
		t.Helper()
		until := time.Now().Add(time.Second)
		for time.Now().Before(until) {
			maximum := 0
			guard.mu.Lock()
			for _, lane := range guard.lanes {
				maximum = max(maximum, len(lane.waiters))
			}
			guard.mu.Unlock()
			if maximum == count {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("Start requeue never reached the shared route FIFO")
	}
	wait("tail", false)
	queued(1)
	wait("front", true)
	queued(2)
	releases[0]()
	select {
	case outcome := <-results:
		if outcome.err != nil || outcome.name != "front" {
			if outcome.release != nil {
				outcome.release()
			}
			t.Fatal("front Start requeue lost its route position:", outcome.name, outcome.err)
		}
		select {
		case tail := <-results:
			if tail.release != nil {
				tail.release()
			}
			outcome.release()
			t.Fatal("second route waiter bypassed the six-request cap")
		default:
		}
		outcome.release()
	case <-time.After(time.Second):
		t.Fatal("released route slot never reached the front requeue")
	}
	select {
	case outcome := <-results:
		if outcome.err != nil || outcome.name != "tail" {
			t.Fatal("tail Start waiter did not follow the front:", outcome.name, outcome.err)
		}
		t.Cleanup(outcome.release)
	case <-time.After(time.Second):
		t.Fatal("tail route waiter never acquired its turn")
	}
	release, err := guard.waitFor(ctx, "expired", "route", true, time.Now().Add(-time.Second))
	if err == nil {
		release()
		t.Fatal("expired aggregate requeue budget gained dispatch authority")
	}
}

func TestDonor0168ConsumedStartRefusalReleasesBeforePaidStream(t *testing.T) {
	fixture, upstream := newGiftFixture(t, `{"code":1005,"msg":"exceed quota limit"}`, 200)
	fixture.quota.Store(0)
	paidStarted, releasePaid := make(chan struct{}), make(chan struct{})
	var closePaid sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/zai" {
			fixture.paid.Add(1)
			writer.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"paid-stream\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"GLM-5.3\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
			writer.(http.Flusher).Flush()
			close(paidStarted)
			<-releasePaid
			io.WriteString(writer, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}
		upstream.Config.Handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)
	states := make(chan *startGuard, 1)
	factory := func(options ModuleOptions) sdk.Module {
		component := NewControlModule(options)
		previous := component.Handlers[sdk.MethodProviderExecute]
		component.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
			state, err := currentInstance(ctx)
			if err != nil {
				return nil, err
			}
			owner := &module{options: options}
			route := controller.Principal(owner.zcodeOrigin(), "")
			for index := range 5 {
				release, err := state.startGuard.acquire(ctx, fmt.Sprintf("held-%d", index), route)
				if err != nil {
					return nil, err
				}
				defer release()
			}
			states <- state.startGuard
			return previous(ctx, call, stream)
		}
		return component
	}
	harness := newHarnessModule(t, endpointOptions(server), moduleConfig{}, factory)
	account := giftAccount()
	done := make(chan sdk.Frame, 1)
	go func() {
		frame, _ := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
		done <- frame
	}()
	defer func() {
		closePaid.Do(func() { close(releasePaid) })
		select {
		case frame := <-done:
			if frame.Error != nil {
				t.Error("paid fallback failed:", frame.Error)
			}
		case <-time.After(5 * time.Second):
			t.Error("paid fallback fixture did not join")
		}
	}()
	var guard *startGuard
	select {
	case guard = <-states:
	case <-time.After(5 * time.Second):
		t.Fatal("execution never acquired its module instance")
	}
	select {
	case <-paidStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmed unused Start refusal never reached paid fallback")
	}
	owner := &module{options: endpointOptions(server)}
	route := controller.Principal(owner.zcodeOrigin(), "")
	release, err := guard.waitFor(t.Context(), "unrelated-trial", route, false, time.Now().Add(25*time.Millisecond))
	if err != nil {
		t.Fatal("consumed Start response retained route capacity during unrelated paid stream:", err)
	}
	release()
}

func TestDonor0168RouteFullnessNeverPersistsAsQuotaApplicability(t *testing.T) {
	_, server := newGiftFixture(t, `{"type":"message","content":[]}`, 200)
	var blocked atomic.Bool
	factory := func(options ModuleOptions) sdk.Module {
		component := NewControlModule(options)
		previous := component.Handlers[sdk.MethodProviderUsage]
		component.Handlers[sdk.MethodProviderUsage] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
			if blocked.Load() {
				state, err := currentInstance(ctx)
				if err != nil {
					return nil, err
				}
				owner := &module{options: options}
				route := controller.Principal(owner.zcodeOrigin(), "")
				for index := range 6 {
					release, err := state.startGuard.acquire(ctx, fmt.Sprintf("held-%d", index), route)
					if err != nil {
						return nil, err
					}
					defer release()
				}
			}
			return previous(ctx, call, stream)
		}
		return component
	}
	harness := newHarnessModule(t, endpointOptions(server), moduleConfig{}, factory)
	account := giftAccount()
	var baseline string
	for index, full := range []bool{false, true, false} {
		blocked.Store(full)
		frame, _ := harness.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
		var report sdk.UsageResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &report) != nil || len(report.Windows) == 0 {
			t.Fatal("quota applicability fixture failed:", frame.Error)
		}
		for index := range report.Windows {
			sort.Strings(report.Windows[index].Models)
			sort.Strings(report.Windows[index].NotModels)
		}
		windows := string(raw(report.Windows))
		if index == 0 {
			baseline = windows
		} else if windows != baseline {
			t.Fatal("transient Start capacity changed persisted quota applicability")
		}
	}
}
