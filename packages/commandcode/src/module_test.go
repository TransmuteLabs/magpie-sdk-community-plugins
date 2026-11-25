package commandcode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type moduleFixture struct {
	t       *testing.T
	input   *io.PipeWriter
	reader  *bufio.Scanner
	next    int
	recover func(sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult
}

func fixture(t *testing.T, config Config) *moduleFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	input, writer := io.Pipe()
	reader, output := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- sdk.Serve(ctx, input, output, Module()) }()
	client := &moduleFixture{t: t, input: writer, reader: bufio.NewScanner(reader)}
	client.reader.Buffer(make([]byte, 4096), sdk.MaxFrameBytes)
	t.Cleanup(func() {
		cancel()
		writer.Close()
		reader.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("SDK fixture failed to stop")
		}
	})
	raw, _ := json.Marshal(config)
	_, _, err := client.call(sdk.MethodComponentConfigure, sdk.ComponentConfigureRequest{
		Component: "primary", Instance: "fixture-instance", Config: sdk.ConfigSnapshot{Version: "1", Values: raw},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (fixture *moduleFixture) send(frame sdk.Frame) {
	fixture.t.Helper()
	body, err := json.Marshal(frame)
	if err != nil {
		fixture.t.Fatal(err)
	}
	if _, err := fixture.input.Write(append(body, '\n')); err != nil {
		fixture.t.Fatal(err)
	}
}

func (fixture *moduleFixture) call(method string, input any, auth json.RawMessage) (json.RawMessage, []sdk.Frame, error) {
	scope := sdk.Scope{Plugin: ModuleID, Provider: ProviderID, Account: "fixture-account", Generation: 1}
	if (method == sdk.MethodAuthStart || method == sdk.MethodAuthFinish) && auth == nil {
		scope.Account, scope.Generation = "", 0
	}
	return fixture.callScoped(method, scope, input, auth)
}

func (fixture *moduleFixture) callScoped(method string, scope sdk.Scope, input any, auth json.RawMessage) (json.RawMessage, []sdk.Frame, error) {
	fixture.t.Helper()
	fixture.next++
	id := fmt.Sprintf("call-%d", fixture.next)
	scope.Operation = id
	body, _ := json.Marshal(input)
	fixture.send(sdk.Frame{Kind: sdk.KindCall, ID: id, Method: method, Params: body, Auth: auth,
		Scope: scope, Deadline: time.Now().Add(10 * time.Second).UnixMilli()})
	var events []sdk.Frame
	for fixture.reader.Scan() {
		var frame sdk.Frame
		if err := json.Unmarshal(fixture.reader.Bytes(), &frame); err != nil {
			fixture.t.Fatal(err)
		}
		switch frame.Kind {
		case sdk.KindHostCall:
			var result any = struct{}{}
			if frame.Method == sdk.MethodRequestRecover && fixture.recover != nil {
				var request sdk.RequestRecoveryRequest
				if err := json.Unmarshal(frame.Params, &request); err != nil {
					fixture.t.Fatal(err)
				}
				result = fixture.recover(request)
			} else if frame.Method != sdk.MethodNetworkProxy {
				fixture.t.Fatalf("unexpected host callback %s", frame.Method)
			}
			encoded, _ := json.Marshal(result)
			fixture.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent, Result: encoded})
		case sdk.KindEvent:
			events = append(events, frame)
			fixture.send(sdk.Frame{Kind: sdk.KindAck, ID: frame.ID, Seq: frame.Seq})
		case sdk.KindResult:
			if frame.ID != id {
				fixture.t.Fatalf("unexpected result ID %s", frame.ID)
			}
			if frame.Error != nil {
				return nil, events, frame.Error
			}
			return frame.Result, events, nil
		default:
			fixture.t.Fatalf("unexpected SDK frame kind %s", frame.Kind)
		}
	}
	fixture.t.Fatal("SDK fixture output ended", fixture.reader.Err())
	return nil, nil, io.ErrUnexpectedEOF
}

func testAuth() json.RawMessage {
	return json.RawMessage(`{"apiKey":"fixture-key","userId":"fixture-user"}`)
}

func eventsBody(t *testing.T, events []sdk.Frame) (int, []byte) {
	t.Helper()
	var status int
	var body []byte
	for _, event := range events {
		if event.Event == "headers" {
			var value sdk.HeadersEvent
			if err := json.Unmarshal(event.Data, &value); err != nil {
				t.Fatal(err)
			}
			status = value.Status
		}
		if event.Event == "data" {
			var chunk []byte
			if err := json.Unmarshal(event.Data, &chunk); err != nil {
				t.Fatal(err)
			}
			body = append(body, chunk...)
		}
	}
	return status, body
}

func TestModuleManifestNegotiatesOnlyImplementedCapabilities(t *testing.T) {
	if err := Module().Validate(); err != nil {
		t.Fatal(err)
	}
	if Manifest().Contract.Version.Minor != 12 || !Manifest().Contract.Has(sdk.RequestPolicy) {
		t.Fatal("unexpected contract increment")
	}
	worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
	_, events, err := worker.call(sdk.MethodModuleCall, sdk.ModuleCallRequest{Action: "systemone", Input: json.RawMessage(`{"model":"typesafe/jev"}`)}, nil)
	if err == nil || !strings.Contains(err.Error(), "SystemOne") || len(events) != 0 {
		t.Fatalf("decision silently became chat: %v %+v", err, events)
	}
}

func TestProviderProtocolsRemainOpaqueAndKeysNeverFollowRedirects(t *testing.T) {
	var leaked atomic.Bool
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true); w.WriteHeader(500) }))
	defer trap.Close()
	for _, protocol := range []string{"chat", "responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			var got []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/alpha/billing/subscriptions" {
					io.WriteString(w, `{"data":{"planId":"individual-pro","status":"active"}}`)
					return
				}
				got, _ = io.ReadAll(r.Body)
				if r.Header.Get("Authorization") != "Bearer fixture-key" || r.Header.Get("x-api-key") != "fixture-key" || r.Header.Get("Cookie") != "" || r.Header.Get("x-cmd-zdr") != "1" {
					t.Error("credential/ZDR headers differ")
				}
				if r.URL.Path == "/provider/v1/responses" {
					w.Header().Set("Location", trap.URL)
					w.WriteHeader(307)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"fixture":true}`)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai", ZDR: true})
			input := sdk.ExecuteRequest{Protocol: protocol, Model: "fixture", Body: []byte(`{"model":"fixture","stream":false,"vendor_extension":{"keep":true}}`),
				Headers: http.Header{"Cookie": {"secret=absent"}, "Authorization": {"Bearer attacker"}}}
			result, events, err := worker.call(sdk.MethodProviderExecute, input, testAuth())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got, []byte(`"vendor_extension":{"keep":true}`)) {
				t.Fatalf("provider semantic field was dropped: %s", got)
			}
			status, body := eventsBody(t, events)
			if protocol == "responses" {
				if status != 307 {
					t.Fatalf("redirect status: %d", status)
				}
			} else if status != 200 || string(body) != `{"fixture":true}` {
				t.Fatalf("provider reply: %d %s %s", status, body, result)
			}
		})
	}
	if leaked.Load() {
		t.Fatal("credentials followed an upstream redirect")
	}
}

func TestModuleGoPlanUsesNativeAndDoesNotReplayTruncation(t *testing.T) {
	var generated atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/alpha/billing/subscriptions":
			io.WriteString(w, `{"data":{"planId":"individual-go","status":"active"}}`)
		case "/alpha/generate":
			generated.Add(1)
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["params"] == nil {
				t.Error("not a native generate request")
			}
			if r.Header.Get("x-command-code-version") != "1.93.3" || r.Header.Get("x-cmd-zdr") != "1" {
				t.Errorf("native request does not use the reviewed CLI version and ZDR: %v", r.Header)
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			io.WriteString(w, "{\"type\":\"text-delta\",\"text\":\"partial\"}\n")
		default:
			t.Errorf("unexpected native route %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai", WorkingDirectory: "/fixture", ZDR: true})
	_, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "m", Body: []byte(`{"model":"m","messages":[{"role":"user","content":"x"}],"stream":true}`)}, testAuth())
	var failure *sdk.Error
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("truncated native reply succeeded: %v", err)
	}
	if typed, ok := err.(*sdk.Error); ok {
		failure = typed
	}
	if failure == nil || failure.Outcome != "uncertain" {
		t.Fatalf("lost uncertain outcome: %v", err)
	}
	_, body := eventsBody(t, events)
	if generated.Load() != 1 || !bytes.Contains(body, []byte("partial")) || bytes.Contains(body, []byte("[DONE]")) {
		t.Fatalf("replay/false terminal: %d %s", generated.Load(), body)
	}
}

func TestAuthenticationFlowIsOpaqueVerifiedAndSingleUse(t *testing.T) {
	var valid atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !valid.Load() {
			w.WriteHeader(503)
			return
		}
		io.WriteString(w, `{"user":{"id":"fixture-user","userName":"Fixture"}}`)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	result, _, err := worker.call(sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "api-key", Input: json.RawMessage(`{"apiKey":"fixture-key"}`)}, nil)
	if err != nil || bytes.Contains(result, []byte("fixture-key")) {
		t.Fatalf("opaque flow: %s %v", result, err)
	}
	var start sdk.AuthStartResult
	json.Unmarshal(result, &start)
	input := sdk.AuthFinishRequest{Continuation: start.Continuation}
	if _, _, err := worker.call(sdk.MethodAuthFinish, input, nil); err == nil {
		t.Fatal("unknown whoami fabricated successful auth")
	}
	valid.Store(true)
	result, _, err = worker.call(sdk.MethodAuthFinish, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var auth sdk.AuthResult
	if json.Unmarshal(result, &auth) != nil || auth.ID == "" || auth.ID == "fixture-user" || !bytes.Contains(auth.Credentials, []byte("fixture-key")) {
		t.Fatalf("key identity/credential: %s", result)
	}
	if _, _, err := worker.call(sdk.MethodAuthFinish, input, nil); err == nil {
		t.Fatal("completed sign-in was replayed")
	}
}

func TestBrowserCallbackRequiresExactStateAndOrigin(t *testing.T) {
	worker := fixture(t, Config{APIURL: "http://127.0.0.1:1", StudioURL: "https://commandcode.ai"})
	result, _, err := worker.call(sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "browser"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var start sdk.AuthStartResult
	json.Unmarshal(result, &start)
	location, _ := url.Parse(start.URL)
	query := location.Query()
	callback := query.Get("callback")
	for _, sample := range []struct {
		state, origin string
		status        int
	}{
		{"wrong", "https://commandcode.ai", 403},
		{query.Get("state"), "https://evil.invalid", 403},
		{query.Get("state"), "https://commandcode.ai", 200},
		{query.Get("state"), "https://commandcode.ai", 409},
	} {
		body, _ := json.Marshal(map[string]string{"state": sample.state, "apiKey": "fixture-key"})
		request, _ := http.NewRequest("POST", callback, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", sample.origin)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != sample.status {
			t.Fatalf("callback status %d != %d", response.StatusCode, sample.status)
		}
	}
}

func TestUsageAcknowledgesTheHostRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alpha/billing/subscriptions" {
			io.WriteString(w, `{"data":{"planId":"individual-go","status":"active"}}`)
		} else {
			io.WriteString(w, `{"credits":{"monthlyCredits":9}}`)
		}
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	for _, revision := range []string{"0", "1", "18446744073709551615"} {
		body, _, err := worker.call(sdk.MethodProviderUsage, sdk.UsageRequest{Revision: revision}, testAuth())
		var result sdk.UsageResult
		if err != nil || json.Unmarshal(body, &result) != nil || result.Revision != revision {
			t.Fatalf("usage revision lost: %s %v", body, err)
		}
		if _, fresh, err := sdk.ValidateUsageResult(Manifest(), body); err != nil || !fresh {
			t.Fatalf("live usage cannot acknowledge the host barrier: %s %v", body, err)
		}
	}
}

func TestUsageReportsResetTimesInMilliseconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/alpha/billing/subscriptions":
			io.WriteString(writer, `{"data":{"planId":"individual-go","status":"active","currentPeriodEnd":"2026-11-28T12:00:00.125Z"}}`)
		case "/alpha/billing/credits":
			io.WriteString(writer, `{"credits":{"monthlyCredits":9},"windowLimits":{"fiveHour":{"used":1,"cap":2,"resetAt":1790000000},"weekly":{"used":3,"cap":4,"resetAt":"1792149831123"}}}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	body, _, err := worker.call(sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "1"}, testAuth())
	if err != nil {
		t.Fatal(err)
	}
	result, fresh, err := sdk.ValidateUsageResult(Manifest(), body)
	if err != nil || !fresh || len(result.Windows) != 3 {
		t.Fatalf("usage: %s %v", body, err)
	}
	for index, want := range []int64{1790000000000, 1792149831123, time.Date(2026, 11, 28, 12, 0, 0, 125000000, time.UTC).UnixMilli()} {
		if got := result.Windows[index]; got.ResetsAt != want {
			t.Errorf("%s reset %d, want %d", got.Name, got.ResetsAt, want)
		}
	}
}

func TestUsagePreservesKnownDollarBalanceApartFromCreditUnits(t *testing.T) {
	for _, sample := range []struct {
		name, subscription, credits string
		balance                     *float64
		windows                     int
		fresh                       bool
	}{
		{"no-plan", `null`, `{"purchasedCredits":7,"freeCredits":0.25}`, new(7.25), 1, false},
		{"known-zero", `null`, `{"purchasedCredits":0}`, new(0.0), 1, false},
		{"unknown", `null`, `{}`, nil, 0, false},
		{"plan-with-credit-units", `{"planId":"individual-go","status":"active"}`, `{"monthlyCredits":9,"purchasedCredits":7.25}`, new(7.25), 2, true},
		{"plan-with-dollar-units", `{"planId":"individual-max","status":"active"}`, `{"monthlyCredits":100,"purchasedCredits":7.25}`, nil, 1, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/alpha/billing/subscriptions":
					io.WriteString(writer, `{"data":`+sample.subscription+`}`)
				case "/alpha/billing/credits":
					io.WriteString(writer, `{"credits":`+sample.credits+`}`)
				default:
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
			body, _, err := worker.call(sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "1"}, testAuth())
			if err != nil {
				t.Fatal(err)
			}
			result, fresh, err := sdk.ValidateUsageResult(Manifest(), body)
			if err != nil || fresh != sample.fresh || len(result.Windows) != sample.windows {
				t.Fatalf("usage: %s %v", body, err)
			}
			for _, window := range result.Windows {
				if window.Name == "Credits" && sample.name == "plan-with-credit-units" && (window.Used != 10 || window.Amount == nil || *window.Amount != 1 || window.Limit != 10 || window.Unit != "credits") {
					t.Errorf("plan credits: %+v", window)
				}
				if window.Balance != nil && (sample.balance == nil || *window.Balance != *sample.balance || window.Unit != "USD" || !window.Aside) {
					t.Errorf("dollar balance: %+v", window)
				}
			}
			if sample.balance != nil && result.Windows[len(result.Windows)-1].Balance == nil {
				t.Fatalf("missing balance: %s", body)
			}
		})
	}
}

func TestUsageRejectsInvalidAuthorityBeforeNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		io.WriteString(w, `{"credits":{"monthlyCredits":9}}`)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	for _, revision := range []string{"opaque-host-revision", "01", "-1", "18446744073709551616"} {
		_, _, err := worker.call(sdk.MethodProviderUsage, sdk.UsageRequest{Revision: revision}, testAuth())
		failure, ok := err.(*sdk.Error)
		if !ok || failure.Code != "invalid_params" {
			t.Fatalf("invalid revision did not get a precise refusal: %q %v", revision, err)
		}
	}
	_, _, err := worker.call(sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "1", Credentials: json.RawMessage(`{"apiKey":"foreign-key"}`)}, testAuth())
	failure, ok := err.(*sdk.Error)
	if !ok || failure.Code != "invalid_params" {
		t.Fatalf("ordinary params accepted credential authority: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid usage authority reached the network %d times", requests.Load())
	}
}

func TestUsageUnknownDoesNotAcknowledgeTheHostBarrier(t *testing.T) {
	for _, sample := range []struct {
		name, subscription, credits       string
		subscriptionStatus, creditsStatus int
		windows                           int
	}{
		{"unknown-subscription", `{}`, `{"credits":{"monthlyCredits":9},"windowLimits":{"fiveHour":{"used":1,"cap":10}}}`, 200, 200, 1},
		{"unavailable-subscription", `{}`, `{"credits":{"monthlyCredits":9},"windowLimits":{"fiveHour":{"used":1,"cap":10}}}`, 503, 200, 1},
		{"unknown-credits", `{"data":{"planId":"individual-go","status":"active"}}`, `{}`, 200, 200, 0},
		{"unavailable-credits", `{"data":{"planId":"individual-go","status":"active"}}`, `{}`, 200, 503, 0},
	} {
		t.Run(sample.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/alpha/billing/subscriptions" {
					w.WriteHeader(sample.subscriptionStatus)
					io.WriteString(w, sample.subscription)
					return
				}
				w.WriteHeader(sample.creditsStatus)
				io.WriteString(w, sample.credits)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
			body, _, err := worker.call(sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "7"}, testAuth())
			if err != nil {
				t.Fatal(err)
			}
			result, fresh, err := sdk.ValidateUsageResult(Manifest(), body)
			if err != nil || fresh || result.Revision != "7" || result.Error == "" || len(result.Windows) != sample.windows {
				t.Fatalf("unknown usage became authoritative: %s %v", body, err)
			}
		})
	}
}
