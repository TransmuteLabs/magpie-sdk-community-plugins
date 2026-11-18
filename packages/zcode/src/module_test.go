package zcode

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func raw(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

type harness struct {
	t                 *testing.T
	conn              net.Conn
	write             *json.Encoder
	read              *json.Decoder
	next              int
	headers           []sdk.HeadersEvent
	usage             []sdk.Usage
	diagnostics       []sdk.Diagnostic
	diagnosticParams  []json.RawMessage
	rejectDiagnostics bool
	accepted          atomic.Bool
	proxies           int
	writeMu           sync.Mutex
	state             map[string]sdk.StateValue
	casCalls          int
	rejectCAS         int
	invalidations     int
	invalidationError bool
	onInvalidation    func()
	recoveries        []sdk.RequestRecoveryRequest
	recoverRequest    func(sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult
}

func (value *harness) send(frame sdk.Frame) error {
	value.writeMu.Lock()
	defer value.writeMu.Unlock()
	return value.write.Encode(frame)
}

func newHarness(t *testing.T, options ModuleOptions) *harness {
	return newHarnessConfigured(t, options, struct{}{})
}

func newHarnessConfigured(t *testing.T, options ModuleOptions, config any) *harness {
	return newHarnessModule(t, options, config, NewModule)
}

func newHarnessModule(t *testing.T, options ModuleOptions, config any, factory func(ModuleOptions) sdk.Module) *harness {
	t.Helper()
	if options.Now == nil {
		options.Now = func() time.Time { return fixtureNow }
	}
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- sdk.Serve(ctx, worker, worker, factory(options)) }()
	value := &harness{t: t, conn: host, write: json.NewEncoder(host), read: json.NewDecoder(host), state: map[string]sdk.StateValue{}}
	t.Cleanup(func() {
		cancel()
		host.Close()
		worker.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("ZCode SDK fixture did not stop")
		}
	})
	frame, _ := value.call(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: "primary", Instance: "fixture", Config: sdk.ConfigSnapshot{Version: "1", Values: raw(config)},
	}, nil)
	if frame.Error != nil {
		t.Fatalf("configure: %+v", frame.Error)
	}
	return value
}

func (value *harness) call(method string, scope sdk.Scope, params any, auth json.RawMessage) (sdk.Frame, []byte) {
	value.t.Helper()
	value.next++
	id := fmt.Sprintf("fixture-%d", value.next)
	scope.Plugin, scope.Operation = ProviderID, id
	value.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := value.send(sdk.Frame{Kind: sdk.KindCall, ID: id, Method: method, Scope: scope, Params: raw(params), Auth: auth}); err != nil {
		value.t.Fatal(err)
	}
	var body []byte
	callAccepted := false
	for {
		var frame sdk.Frame
		if err := value.read.Decode(&frame); err != nil {
			value.t.Fatal(err)
		}
		switch frame.Kind {
		case sdk.KindHostCall:
			if frame.Scope != scope || frame.Parent != id || len(frame.Auth) != 0 {
				value.t.Fatal("unexpected callback authority")
			}
			var reply any
			switch frame.Method {
			case sdk.MethodRequestRecover:
				var failure sdk.RequestRecoveryRequest
				if err := json.Unmarshal(frame.Params, &failure); err != nil {
					value.t.Fatal(err)
				}
				value.recoveries = append(value.recoveries, failure)
				decision := sdk.RequestRecoveryResult{Step: failure.Step, Action: "terminal"}
				if value.recoverRequest != nil {
					decision = value.recoverRequest(failure)
				}
				reply = decision
			case sdk.MethodNetworkProxy:
				value.proxies++
				reply = sdk.NetworkProxyResult{}
			case sdk.MethodDiagnosticEmit:
				var diagnostic sdk.Diagnostic
				if json.Unmarshal(frame.Params, &diagnostic) != nil ||
					sdk.ValidateDiagnostic(ControlManifest(), diagnostic) != nil {
					value.t.Fatal("invalid scoped diagnostic")
				}
				value.diagnostics = append(value.diagnostics, diagnostic)
				value.diagnosticParams = append(value.diagnosticParams, bytes.Clone(frame.Params))
				if value.rejectDiagnostics {
					if err := value.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: id,
						Error: &sdk.Error{Code: "fixture", Message: "Synthetic diagnostic sink unavailable"}}); err != nil {
						value.t.Fatal(err)
					}
					continue
				}
				reply = sdk.DiagnosticResult{Recorded: true}
			case sdk.MethodUsageInvalidate:
				if !callAccepted || scope.Account == "" {
					value.t.Fatal("quota invalidation lacked selected-account write acceptance")
				}
				value.invalidations++
				if value.onInvalidation != nil {
					value.onInvalidation()
				}
				if value.invalidationError {
					if err := value.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: id,
						Error: &sdk.Error{Code: "fixture", Message: "Synthetic invalidation failure"}}); err != nil {
						value.t.Fatal(err)
					}
					continue
				}
				reply = sdk.UsageInvalidateResult{Revision: strconv.Itoa(value.invalidations)}
			case sdk.MethodStateGet:
				var request sdk.StateGetRequest
				json.Unmarshal(frame.Params, &request)
				key := fmt.Sprintf("%s/%s/%d/%s", scope.Provider, scope.Account, scope.Generation, request.Key)
				current := value.state[key]
				if current.Version == "" {
					current.Version = "0"
				}
				reply = current
			case sdk.MethodStateCAS:
				value.casCalls++
				var request sdk.StateCASRequest
				json.Unmarshal(frame.Params, &request)
				key := fmt.Sprintf("%s/%s/%d/%s", scope.Provider, scope.Account, scope.Generation, request.Key)
				current := value.state[key]
				expected := request.Version
				if expected == "" {
					expected = "0"
				}
				version := current.Version
				if version == "" {
					version = "0"
				}
				if expected == version && value.casCalls != value.rejectCAS {
					number, _ := strconv.ParseUint(version, 10, 64)
					current = sdk.StateValue{Found: true, Version: strconv.FormatUint(number+1, 10), Value: bytes.Clone(request.Value)}
					value.state[key] = current
					reply = sdk.StateCASResult{Swapped: true, Version: current.Version}
				} else {
					reply = sdk.StateCASResult{Version: version}
				}
			default:
				value.t.Fatal("unexpected host callback", frame.Method)
			}
			if err := value.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: id, Result: raw(reply)}); err != nil {
				value.t.Fatal(err)
			}
		case sdk.KindEvent:
			if frame.ID != id {
				value.t.Fatal("foreign event")
			}
			switch frame.Event {
			case "accepted":
				var receipt sdk.AcceptedEvent
				if json.Unmarshal(frame.Data, &receipt) != nil || receipt.OperationID != id {
					value.t.Fatal("invalid acceptance receipt")
				}
				value.accepted.Store(true)
				callAccepted = true
			case "headers":
				var headers sdk.HeadersEvent
				if json.Unmarshal(frame.Data, &headers) != nil {
					value.t.Fatal("invalid headers event")
				}
				value.headers = append(value.headers, headers)
			case "data":
				var chunk []byte
				if json.Unmarshal(frame.Data, &chunk) != nil {
					value.t.Fatal("invalid data event")
				}
				body = append(body, chunk...)
			case "usage":
				var usage sdk.Usage
				if json.Unmarshal(frame.Data, &usage) != nil {
					value.t.Fatal("invalid usage event")
				}
				value.usage = append(value.usage, usage)
			default:
				value.t.Fatal("unexpected event")
			}
			if err := value.send(sdk.Frame{Kind: sdk.KindAck, ID: id, Seq: frame.Seq}); err != nil {
				value.t.Fatal(err)
			}
		case sdk.KindResult:
			if frame.ID != id {
				value.t.Fatal("foreign result")
			}
			return frame, body
		default:
			value.t.Fatal("unexpected SDK frame", frame.Kind)
		}
	}
}

func scope(account Credentials) sdk.Scope {
	return sdk.Scope{Provider: ProviderID, Account: account.AccountID(), Generation: 7, Session: "fixture-session"}
}

func execute(body string) sdk.ExecuteRequest {
	return sdk.ExecuteRequest{Protocol: "messages", Model: "GLM-5.3", Body: []byte(body)}
}

func endpointOptions(server *httptest.Server) ModuleOptions {
	return ModuleOptions{Endpoints: Endpoints{
		Zai: server.URL + "/zai", BigModel: server.URL + "/bigmodel", Start: server.URL + "/start",
		ZaiQuota: server.URL + "/quota/zai", BigModelQuota: server.URL + "/quota/bigmodel", LegacyBigModelQuota: server.URL + "/quota/legacy-bigmodel",
	}}
}

const confirmedStartGLM52 = `{"code":0,"data":{"plans":[{"plan_id":"grant","status":"active"}],"balances":[
	{"plan_id":"grant","total_units":10,"remaining_units":10,"capabilities":["model:GLM-5.2"]}
]}}`

func TestModuleManifestAndSnapshotImport(t *testing.T) {
	module := NewModule(ModuleOptions{})
	if err := module.Validate(); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, ModuleOptions{})
	account := fixtureCredentials("team")
	frame, _ := h.call(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderID},
		sdk.AuthFinishRequest{Input: raw(account)}, nil)
	var result sdk.AuthResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.ID != account.AccountID() ||
		!bytes.Equal(result.Credentials, raw(account)) || h.proxies != 0 {
		t.Fatalf("snapshot import: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodAuthRefresh, scope(account), sdk.AuthRefreshRequest{}, raw(account))
	if frame.Error == nil || frame.Error.Code != "credentials_required" || h.proxies != 0 || h.accepted.Load() {
		t.Fatal("refresh dispatched without serialized publication")
	}
}

func TestOrdinarySelectedPATInferenceAndBusinessErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/zai" || request.Header.Get("Authorization") != "Bearer fixture-pat" || request.Header.Get("X-Api-Key") != "fixture-pat" ||
			request.Header.Get("Cookie") != "" || request.Header.Get("X-Off-Peak-Ticket-ID") != "" {
			t.Error("selected credential or target boundary violated")
		}
		body, _ := io.ReadAll(request.Body)
		if bytes.Contains(body, []byte("budget_tokens")) || !bytes.Contains(body, []byte(`"effort":"max"`)) {
			t.Error("native reasoning not normalized")
		}
		writer.Header().Set("Content-Type", "application/json")
		if requests.Load() == 2 {
			io.WriteString(writer, `{"success":false,"code":3008,"msg":"fixture-private-error"}`)
			return
		}
		io.WriteString(writer, `{"type":"message","content":[{"type":"text","text":"ok"}]}`)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("individual")
	request := execute(`{"model":"GLM-5.3","messages":[{"role":"user","content":"hello"}],"thinking":{"type":"enabled","budget_tokens":8192},"output_config":{"effort":"max"}}`)
	request.Protocol = "anthropic"
	request.Options = raw(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", EscapedPath: "/v1/messages", Proxy: "system", CallerID: "fixture-agent"})
	request.Headers = http.Header{"Cookie": {"secret"}}
	frame, body := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error != nil || !bytes.Contains(body, []byte(`"text":"ok"`)) || requests.Load() != 1 {
		t.Fatalf("ordinary execution: %+v, %s", frame.Error, body)
	}
	frame, body = h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	var refusal sdk.ExecuteResult
	if frame.Error != nil || requests.Load() != 2 || json.Unmarshal(frame.Result, &refusal) != nil ||
		refusal.Refusal == nil || refusal.Refusal.Facts.Code != "3008" || len(body) != 0 {
		t.Fatalf("business refusal: %+v, %s", frame.Error, body)
	}
}

func TestNativeAliasesForgedOptionsAndWrongAccountNeverDispatch(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	for _, mode := range []string{"lane", "forged", "native-metadata", "lane-header", "account", "generation", "expired", "credentials", "path", "escaped-path", "operation", "model-body"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, endpointOptions(server))
			account := fixtureCredentials("individual")
			selected := scope(account)
			request := execute(`{"model":"GLM-5.3","messages":[]}`)
			switch mode {
			case "lane":
				request.Model = "GLM-5.3-OffPeak"
			case "forged":
				request.Options = raw(map[string]any{"execution": map[string]any{"ticketId": "forged", "generation": 7}})
			case "native-metadata":
				request.Options = raw(map[string]any{"native": map[string]any{"execution": map[string]any{"ticketId": "forged", "generation": 7}}})
			case "lane-header":
				request.Headers = http.Header{"x-highspeed-card-id": {"forged"}}
			case "account":
				selected.Account = "another-account"
			case "generation":
				selected.Generation = 0
			case "expired":
				account.ExpiresAt = fixtureNow.Add(-time.Second).UnixMilli()
			case "credentials":
				request.Credentials = raw(fixtureCredentials("team"))
			case "path":
				request.Options = raw(map[string]string{"path": "/api/v1/zcode-plan/billing/claim"})
			case "escaped-path":
				request.Options = raw(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", EscapedPath: "/v1/%6dessages"})
			case "operation":
				request.OperationID = "foreign-operation"
			case "model-body":
				request.Body = []byte(`{"model":"GLM-5.2","messages":[]}`)
			}
			frame, _ := h.call(sdk.MethodProviderExecute, selected, request, raw(account))
			if frame.Error == nil || frame.Error.Outcome != "not-started" || h.proxies != 0 || h.accepted.Load() {
				t.Fatalf("unsafe request was admitted: %+v", frame.Error)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("blocked operation reached the network")
	}
}

func TestSSEPartialErrorNeverReplayed(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(writer, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n")
		writer.(http.Flusher).Flush()
		io.WriteString(writer, "data: {\"success\":false,\"code\":1302,\"msg\":\"private\"}\n\n")
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("individual")
	frame, body := h.call(sdk.MethodProviderExecute, scope(account), execute(`{"model":"GLM-5.3","messages":[],"stream":true}`), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" || requests.Load() != 1 ||
		strings.Count(string(body), `"text":"partial"`) != 1 || !bytes.Contains(body, []byte("event: error")) {
		t.Fatalf("partial output replayed or lost: %+v %s", frame.Error, body)
	}
}

func TestStartAndTeamUseDistinctTargetsAndHeaders(t *testing.T) {
	type observed struct {
		path    string
		headers http.Header
	}
	seen := make(chan observed, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/zcode-plan/billing/balance" && request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, confirmedStartGLM52)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/start" && request.URL.Path != "/bigmodel" {
			t.Error("unexpected target in credential-channel fixture")
			http.NotFound(writer, request)
			return
		}
		seen <- observed{request.URL.Path, request.Header.Clone()}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"type":"message","content":[]}`)
	}))
	defer server.Close()
	for _, kind := range []string{"team", "start"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t, endpointOptions(server))
			account := fixtureCredentials(kind)
			if kind == "team" {
				account.Site = "bigmodel"
			}
			request := sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.2",
				Body:    []byte(`{"model":"GLM-5.2","messages":[]}`),
				Headers: http.Header{"Anthropic-Beta": {"tools-2025"}, "Bigmodel-Project": {"forged"}}}
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
			if frame.Error != nil {
				t.Fatalf("execution failed: %+v", frame.Error)
			}
			actual := <-seen
			if kind == "start" && (actual.path != "/start" || actual.headers.Get("Authorization") != "Bearer fixture-session" ||
				actual.headers.Get("X-Api-Key") != "" || actual.headers.Get("Anthropic-Beta") != "" || actual.headers.Get("Bigmodel-Project") != "") {
				t.Fatal("Start mixed paid and session credential roles")
			}
			if kind == "team" && (actual.path != "/bigmodel" || actual.headers.Get("Authorization") != "Bearer fixture-pat" ||
				actual.headers.Get("Bigmodel-Project") != "fixture-project") {
				t.Fatal("Team selection crossed its project or family")
			}
		})
	}
}

func TestRequestWithoutTerminalCannotReportCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(writer, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n")
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("legacy-key")
	frame, body := h.call(sdk.MethodProviderExecute, scope(account), execute(`{"model":"GLM-5.3","messages":[],"stream":true}`), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" || !bytes.Contains(body, []byte("partial")) {
		t.Fatal("truncated response reported completed")
	}
}

func TestDefaultHTTPDecodesGzipAndRefusesRedirect(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
			return
		}
		writer.Header().Set("Content-Encoding", "gzip")
		writer.Header().Set("Content-Type", "application/json")
		compressed := gzip.NewWriter(writer)
		io.WriteString(compressed, `{"type":"message","content":[{"type":"text","text":"gzip"}]}`)
		compressed.Close()
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("individual")
	request := execute(`{"model":"GLM-5.3","messages":[]}`)
	request.Headers = http.Header{"accept-encoding": {"gzip"}}
	frame, body := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error != nil || !bytes.Contains(body, []byte(`"text":"gzip"`)) {
		t.Fatalf("gzip: %+v %s", frame.Error, body)
	}
	options := endpointOptions(server)
	options.Endpoints.Zai = server.URL + "/redirect"
	h = newHarness(t, options)
	frame, _ = h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error != nil || reached.Load() || h.headers[len(h.headers)-1].Status != http.StatusTemporaryRedirect {
		t.Fatal("redirect followed or its status lost")
	}
}

func TestCallerCancellationStopsInFlightRequest(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(writer, "data: {\"type\":\"message_start\"}\n\n")
		writer.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("individual")
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-started
		h.send(sdk.Frame{Kind: sdk.KindCancel, ID: "fixture-2"})
	}()
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), execute(`{"model":"GLM-5.3","messages":[],"stream":true}`), raw(account))
	if frame.Error == nil {
		t.Fatal("canceled call returned success")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request outlived SDK cancellation")
	}
	<-done
}
