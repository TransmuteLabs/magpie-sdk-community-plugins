package zcode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDonor0166StartOutputBearingCapsNeverRetry(t *testing.T) {
	for _, body := range []string{
		`{"code":3008,"usage":{"input_tokens":0}}`,
		`{"code":3008,"content":[]}`,
		`{"code":3008,"data":{"output":[]}}`,
		`{"code":3008,"choices":[]}`,
	} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, confirmedStartGLM52)
				case "/start":
					calls.Add(1)
					writer.WriteHeader(429)
					io.WriteString(writer, body)
				default:
					t.Error("output-bearing Start escaped its inference path")
					http.NotFound(writer, request)
				}
			}))
			t.Cleanup(server.Close)
			harness := newHarness(t, endpointOptions(server))
			account := fixtureCredentials("start")
			harness.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.2"), raw(account))
			if calls.Load() != 1 {
				t.Fatal("an output-bearing concurrency code gained retry authority")
			}
		})
	}
}

func TestDonor0166RequeueFrontKeepsDeadlineAndCancellation(t *testing.T) {
	guard := &startGuard{lanes: map[string]*startQueue{}, routes: map[string]*startRouteHold{}, now: time.Now}
	held, err := guard.acquire(t.Context(), "account", "route")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(held)
	guard.mu.Lock()
	guard.lanes["account"].limit = 1
	guard.lanes["account"].narrowUntil = time.Now().Add(time.Minute)
	guard.mu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	tail := make(chan func(), 1)
	front := make(chan func(), 1)
	failures := make(chan error, 2)
	wait := func(first bool, result chan<- func()) {
		release, failure := guard.waitFor(ctx, "account", "route", first, time.Now().Add(time.Second))
		if failure != nil {
			failures <- failure
			return
		}
		result <- release
	}
	queued := func(want int) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			guard.mu.Lock()
			count := len(guard.lanes["account"].waiters)
			guard.mu.Unlock()
			if count == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("Start request never reached the bounded queue")
	}
	go wait(false, tail)
	queued(1)
	go wait(true, front)
	queued(2)
	held()
	select {
	case release := <-front:
		release()
	case <-tail:
		t.Fatal("requeued Start lost its place to a later request")
	case failure := <-failures:
		t.Fatal(failure)
	case <-time.After(time.Second):
		t.Fatal("requeued Start did not acquire the released slot")
	}
	select {
	case release := <-tail:
		t.Cleanup(release)
		until := time.Now().Add(30 * time.Millisecond)
		if _, err := guard.waitFor(ctx, "account", "route", true, until); err == nil {
			t.Fatal("requeue exceeded its shared wait budget")
		}
		release()
		for range 64 {
			if free, err := guard.waitFor(ctx, "account", "route", true, until); err == nil {
				free()
				t.Fatal("expired requeue budget admitted a new dispatch on an empty lane")
			}
		}
		cancel()
		if _, err := guard.waitFor(ctx, "account", "route", true, time.Now().Add(time.Second)); !errors.Is(err, context.Canceled) {
			t.Fatal("canceled requeue gained dispatch authority")
		}
		guard.mu.Lock()
		defer guard.mu.Unlock()
		if lane := guard.lanes["account"]; lane.active != 0 || len(lane.waiters) != 0 {
			t.Fatal("timed out or canceled requeue leaked a slot or waiter")
		}
	case failure := <-failures:
		t.Fatal(failure)
	case <-time.After(time.Second):
		t.Fatal("later Start request did not acquire its turn")
	}
}

func TestDonor0166StartBesideInflightWaitsInsteadOfReturningCap(t *testing.T) {
	var active, calls, caps atomic.Int32
	firstStarted := make(chan struct{})
	firstDone := make(chan struct{})
	secondCap := make(chan struct{})
	var closeCap sync.Once
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/zcode-plan/billing/balance" {
			io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"grant","status":"active"}],"balances":[
				{"plan_id":"grant","total_units":10,"remaining_units":10,"capabilities":["model:GLM-5.2"]}
			]}}`)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/start" {
			t.Errorf("Start requeue reached another endpoint: %s %s", request.Method, request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		order := calls.Add(1)
		count := active.Add(1)
		defer active.Add(-1)
		if count > 1 {
			if caps.Add(1) == 2 {
				closeCap.Do(func() { close(secondCap) })
			}
			writer.WriteHeader(429)
			io.WriteString(writer, `{"code":3008,"msg":"synthetic unused concurrency refusal"}`)
			return
		}
		if order == 1 {
			close(firstStarted)
			select {
			case <-firstDone:
			case <-request.Context().Done():
				return
			}
		}
		io.WriteString(writer, `{"type":"message","content":[]}`)
	}))
	t.Cleanup(server.Close)
	ctx := t.Context()
	go func() {
		select {
		case <-secondCap:
			timer := time.NewTimer(3500 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
			}
		case <-ctx.Done():
		}
		close(firstDone)
	}()
	options := endpointOptions(server)
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	options.HTTPFactory = func(proxy string) (Doer, func(), error) {
		if proxy != "" {
			return nil, nil, errors.New("unexpected Start fixture proxy")
		}
		return client, func() {}, nil
	}
	harness := newHarness(t, options)
	account := fixtureCredentials("start")
	sent := make(chan error, 1)
	go func() {
		for _, id := range []string{"held", "beside"} {
			if id == "beside" {
				select {
				case <-firstStarted:
				case <-ctx.Done():
					sent <- ctx.Err()
					return
				}
			}
			selected := scope(account)
			selected.Plugin, selected.Operation, selected.Session = ProviderID, id, id
			if err := harness.send(sdk.Frame{Kind: sdk.KindCall, ID: id, Method: sdk.MethodProviderExecute,
				Scope: selected, Auth: raw(account), Params: raw(giftRequest("GLM-5.2"))}); err != nil {
				sent <- err
				return
			}
		}
		sent <- nil
	}()
	statuses := map[string]int{}
	for results := 0; results < 2; {
		var frame sdk.Frame
		if err := harness.read.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		switch frame.Kind {
		case sdk.KindHostCall:
			if frame.Method == sdk.MethodRequestRecover {
				if err := harness.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent,
					Result: raw(sdk.RequestRecoveryResult{Step: "start-messages", Action: "wait", Retry: true})}); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if frame.Method != sdk.MethodNetworkProxy {
				t.Fatal("unexpected Start callback", frame.Method)
			}
			if err := harness.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent,
				Result: raw(sdk.NetworkProxyResult{})}); err != nil {
				t.Fatal(err)
			}
		case sdk.KindEvent:
			if frame.Event == "headers" {
				var headers sdk.HeadersEvent
				if err := json.Unmarshal(frame.Data, &headers); err != nil {
					t.Fatal(err)
				}
				statuses[frame.ID] = headers.Status
			}
			if err := harness.send(sdk.Frame{Kind: sdk.KindAck, ID: frame.ID, Seq: frame.Seq}); err != nil {
				t.Fatal(err)
			}
		case sdk.KindResult:
			if frame.Error != nil {
				t.Fatal("Start requeue failed", frame.Error)
			}
			results++
		default:
			t.Fatal("unexpected Start frame", frame.Kind)
		}
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if statuses["held"] != 200 || statuses["beside"] != 200 || caps.Load() != 2 || calls.Load() != 4 {
		t.Fatalf("a known unused cap beside another request escaped instead of waiting: statuses=%v caps=%d calls=%d",
			statuses, caps.Load(), calls.Load())
	}
}
