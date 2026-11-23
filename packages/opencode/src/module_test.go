package opencode

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type stored struct {
	version uint64
	value   json.RawMessage
}

type sdkHarness struct {
	t              *testing.T
	conn           net.Conn
	encoder        *json.Encoder
	decoder        *json.Decoder
	next           int
	state          map[string]stored
	proxies        []sdk.Scope
	proxyURL       string
	headers        []sdk.HeadersEvent
	accepted       map[string]bool
	acceptedSignal *atomic.Bool
	storage        *os.File
}

func newHarness(t *testing.T, options ModuleOptions) *sdkHarness {
	return serveHarness(t, NewModule(options), json.RawMessage(`{"projectId":"global"}`))
}

func serveHarness(t *testing.T, module sdk.Module, config json.RawMessage) *sdkHarness {
	t.Helper()
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- sdk.Serve(ctx, worker, worker, module) }()
	h := &sdkHarness{t: t, conn: host, encoder: json.NewEncoder(host), decoder: json.NewDecoder(host),
		state: map[string]stored{}, accepted: map[string]bool{}, proxyURL: "http://fixture-proxy.invalid:8080"}
	t.Cleanup(func() {
		cancel()
		host.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("SDK fixture did not close")
		}
	})
	frame, _ := h.invoke(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: "primary", Instance: "fixture", Config: sdk.ConfigSnapshot{
			Version: "1", Values: config,
		},
	}, nil)
	if frame.Error != nil {
		t.Fatalf("configure: %+v", frame.Error)
	}
	return h
}

func raw(value any) json.RawMessage {
	result, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return result
}

func (h *sdkHarness) invoke(method string, scope sdk.Scope, params any, auth json.RawMessage) (sdk.Frame, []byte) {
	h.t.Helper()
	h.next++
	id := fmt.Sprintf("fixture-%d", h.next)
	scope.Plugin, scope.Operation = "opencode", id
	h.conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := h.encoder.Encode(sdk.Frame{Kind: "call", ID: id, Method: method, Scope: scope, Params: raw(params), Auth: auth}); err != nil {
		h.t.Fatal(err)
	}
	var body []byte
	for {
		var frame sdk.Frame
		if err := h.decoder.Decode(&frame); err != nil {
			h.t.Fatal(err)
		}
		switch frame.Kind {
		case "host.call":
			if frame.Parent != id || frame.Scope != scope || len(frame.Auth) != 0 {
				h.t.Fatal("SDK callback changed parent authority")
			}
			var result any
			switch frame.Method {
			case sdk.MethodNetworkProxy:
				var request sdk.NetworkProxyRequest
				json.Unmarshal(frame.Params, &request)
				u, err := neturl(request.URL)
				if err != nil || u == "" {
					h.t.Fatal("invalid network callback")
				}
				h.proxies = append(h.proxies, frame.Scope)
				result = sdk.NetworkProxyResult{URL: h.proxyURL}
			case sdk.MethodStateGet:
				var request sdk.StateGetRequest
				json.Unmarshal(frame.Params, &request)
				value := h.state[stateKey(scope, request.Key)]
				result = sdk.StateValue{Found: value.version != 0, Version: strconv.FormatUint(value.version, 10), Value: value.value}
			case sdk.MethodStateCAS:
				var request sdk.StateCASRequest
				json.Unmarshal(frame.Params, &request)
				key := stateKey(scope, request.Key)
				value := h.state[key]
				if request.Version == strconv.FormatUint(value.version, 10) {
					value.version++
					value.value = append(json.RawMessage(nil), request.Value...)
					h.state[key] = value
					result = sdk.StateCASResult{Swapped: true, Version: strconv.FormatUint(value.version, 10)}
				} else {
					result = sdk.StateCASResult{Version: strconv.FormatUint(value.version, 10)}
				}
			case sdk.MethodStorageNative:
				if h.storage == nil {
					h.t.Fatal("storage callback requires an explicitly opened synthetic root")
				}
				var lease sdk.StorageReadLease
				if err := json.Unmarshal(frame.Params, &lease); err != nil {
					h.t.Fatal(err)
				}
				result = sdk.StorageNativeAccess{Lease: lease, Descriptor: int(h.storage.Fd()), Family: HistoryDescriptor().Families[0]}
			case sdk.MethodStorageCheck:
				if h.storage == nil {
					h.t.Fatal("storage publication requires its synthetic root")
				}
				result = struct{}{}
			default:
				h.t.Fatalf("unexpected host method %s", frame.Method)
			}
			if err := h.encoder.Encode(sdk.Frame{Kind: "host.result", ID: frame.ID, Parent: id, Result: raw(result)}); err != nil {
				h.t.Fatal(err)
			}
		case "event":
			if frame.ID != id {
				h.t.Fatal("foreign event")
			}
			if frame.Event == "accepted" {
				var receipt sdk.AcceptedEvent
				json.Unmarshal(frame.Data, &receipt)
				if receipt.OperationID != id {
					h.t.Fatal("accepted another operation")
				}
				h.accepted[id] = true
				if h.acceptedSignal != nil {
					h.acceptedSignal.Store(true)
				}
			}
			if frame.Event == "data" {
				if !h.accepted[id] {
					h.t.Fatal("published output before the accepted fence")
				}
				var chunk []byte
				json.Unmarshal(frame.Data, &chunk)
				body = append(body, chunk...)
			}
			if frame.Event == "headers" {
				var headers sdk.HeadersEvent
				if err := json.Unmarshal(frame.Data, &headers); err != nil {
					h.t.Fatal(err)
				}
				h.headers = append(h.headers, headers)
			}
			if err := h.encoder.Encode(sdk.Frame{Kind: "ack", ID: id, Seq: frame.Seq}); err != nil {
				h.t.Fatal(err)
			}
		case "result":
			if frame.ID != id {
				h.t.Fatal("foreign result")
			}
			return frame, body
		default:
			h.t.Fatalf("unexpected frame %s", frame.Kind)
		}
	}
}

func stateKey(scope sdk.Scope, key string) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s", scope.Provider, scope.Account, scope.Generation, key)
}

func neturl(value string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, value, nil)
	if err != nil {
		return "", err
	}
	return req.URL.Host, nil
}

func TestPublicSDKDeviceFlowModelsUsageAndScopedChat(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	var mu sync.Mutex
	var seen []http.Header
	var requests atomic.Int32
	var acceptedBeforeDispatch atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/console/auth/device/code":
			io.WriteString(w, `{"device_code":"private-device","user_code":"CODE","verification_uri_complete":"/console/verify","expires_in":60,"interval":1}`)
		case "/console/auth/device/token":
			io.WriteString(w, `{"access_token":"access-a","refresh_token":"refresh-a","expires_in":3600,"org_id":"org-a"}`)
		case "/console/api/user":
			io.WriteString(w, `{"id":"a","email":"a@example.invalid"}`)
		case "/console/api/orgs":
			io.WriteString(w, `[{"id":"org-a","name":"Workspace A"}]`)
		case "/zen/go/v1/usage":
			io.WriteString(w, `{"usage":{"rolling":{"percent":23},"weekly":{"percent":75}}}`)
		case "/inference/go/openai/v1/chat/completions":
			mu.Lock()
			seen = append(seen, r.Header.Clone())
			mu.Unlock()
			io.WriteString(w, `{"id":"fixture","choices":[{"message":{"content":"from selected account"}}]}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	var proxySelections atomic.Int32
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL}, Now: now,
		HTTPFactory: func(proxy string) (Doer, func(), error) {
			if proxy != "http://fixture-proxy.invalid:8080" {
				t.Error("effective proxy was ignored")
			}
			proxySelections.Add(1)
			return server.Client(), nil, nil
		}})
	h.acceptedSignal = &acceptedBeforeDispatch
	scope := sdk.Scope{Provider: ProviderGo}
	list, _ := h.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, nil)
	var models sdk.ModelsResult
	json.Unmarshal(list.Result, &models)
	if list.Error != nil || len(models.Models) != 4 || requests.Load() != 0 {
		t.Fatalf("model discovery made up an endpoint: %+v", list.Error)
	}
	start, _ := h.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "device"}, nil)
	var code sdk.AuthStartResult
	json.Unmarshal(start.Result, &code)
	if start.Error != nil || code.PollAfterMs != 1000 || bytesContains(code.Continuation, "private-device") {
		t.Fatalf("device start/opaque continuation: %+v", start.Error)
	}
	early, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	var pending sdk.AuthResult
	json.Unmarshal(early.Result, &pending)
	if early.Error != nil || !pending.Pending || requests.Load() != 1 {
		t.Fatal("poll interval was bypassed")
	}
	clock.Add(int64(time.Second))
	finished, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	var account sdk.AuthResult
	json.Unmarshal(finished.Result, &account)
	if finished.Error != nil || account.ID == "" || len(account.Credentials) == 0 {
		t.Fatalf("device finish: %+v", finished.Error)
	}
	after := requests.Load()
	reused, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	if reused.Error == nil || requests.Load() != after {
		t.Fatal("finished device authorization was exchanged again")
	}
	scope.Account, scope.Generation = account.ID, 1
	for _, revision := range []string{"", "17"} {
		usage, _ := h.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{Revision: revision}, account.Credentials)
		quota, fresh, err := sdk.ValidateUsageResult(Manifest(), usage.Result)
		if usage.Error != nil || err != nil || !fresh || quota.Revision != revision ||
			len(quota.Windows) != 2 || quota.Windows[0].Used != 23 || quota.Windows[0].Limit != 100 {
			t.Fatalf("SDK quota mapping for revision %q: %+v, error %v, fresh %v, result %s",
				revision, usage.Error, err, fresh, usage.Result)
		}
		if revision == "" && bytesContains(usage.Result, `"revision"`) {
			t.Fatal("the omitted host usage revision was synthesized")
		}
	}
	beforeUsage := requests.Load()
	for _, request := range []sdk.UsageRequest{{Revision: "017"}, {Credentials: account.Credentials}} {
		usage, _ := h.invoke(sdk.MethodProviderUsage, scope, request, account.Credentials)
		if usage.Error == nil || usage.Error.Code != "invalid_params" || requests.Load() != beforeUsage {
			t.Fatal("invalid usage parameters reached the vendor")
		}
	}
	request := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
		Body:    []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}]}`),
		Headers: http.Header{"Cookie": {"private-other"}, "X-Org-Id": {"wrong"}, "X-Fixed": {"kept"}},
		Options: raw(map[string]string{"method": "POST", "path": "/v1/chat/completions", "escapedPath": "/v1/chat/completions", "callerId": "caller"})}
	scope.Session = "conversation"
	executed, body := h.invoke(sdk.MethodProviderExecute, scope, request, account.Credentials)
	if executed.Error != nil || !bytesContains(body, "from selected account") || !h.accepted[executed.ID] {
		t.Fatalf("SDK chat did not complete under accepted scope: %+v", executed.Error)
	}
	b := credential(now(), "b", "org-b")
	acceptedBeforeDispatch.Store(false)
	scope.Account, scope.Generation = b.AccountID(ProviderGo), 7
	second, _ := h.invoke(sdk.MethodProviderExecute, scope, request, raw(b))
	if second.Error != nil {
		t.Fatalf("account B: %+v", second.Error)
	}
	mu.Lock()
	if len(seen) != 2 || seen[0].Get("Authorization") != "Bearer access-a" || seen[0].Get("X-Opencode-Org-Id") != "org-a" ||
		seen[1].Get("Authorization") != "Bearer access-b" || seen[1].Get("X-Opencode-Org-Id") != "org-b" ||
		seen[0].Get("X-Opencode-Session") == seen[1].Get("X-Opencode-Session") ||
		seen[0].Get("Cookie") != "" || seen[0].Get("X-Org-Id") != "" || seen[0].Get("X-Fixed") != "kept" {
		t.Error("A/B account or header isolation failed")
	}
	mu.Unlock()
	beforeRefresh := requests.Load()
	refresh, _ := h.invoke(sdk.MethodAuthRefresh, scope, sdk.AuthRefreshRequest{Credentials: raw(account)}, raw(b))
	if refresh.Error == nil || refresh.Error.Code != "permission_denied" || refresh.Error.Outcome != "not-started" || requests.Load() != beforeRefresh {
		t.Fatal("caller-provided refresh credentials were dispatched")
	}
	b.Expires = now().Add(30 * time.Second)
	due, _ := h.invoke(sdk.MethodProviderExecute, scope, request, raw(b))
	if due.Error == nil || due.Error.Code != "auth_refresh_required" || due.Error.Outcome != "not-started" || requests.Load() != beforeRefresh {
		t.Fatal("due token was sent or silently refreshed")
	}
	if proxySelections.Load() == 0 || len(h.proxies) == 0 {
		t.Fatal("SDK proxy context was not consulted")
	}
}

func bytesContains(value []byte, part string) bool { return strings.Contains(string(value), part) }

func TestPublicSDKGoChatRequiresSSEForStreamingSuccess(t *testing.T) {
	const jsonReply = `{"message":"fixture response"}`
	for _, test := range []struct {
		name        string
		stream      string
		status      int
		contentType string
		reply       string
		truncated   bool
	}{
		{"streaming success", `,"stream":true`, http.StatusOK, "application/json", jsonReply, true},
		{"SSE media type lookalike", `,"stream":true`, http.StatusOK, "text/event-streaming", jsonReply, true},
		{"SSE under JSON label", `,"stream":true`, http.StatusOK, "application/json", "data: [DONE]\n\n", false},
		{"SSE without media type", `,"stream":true`, http.StatusOK, "", "\r\n: heartbeat\r\n\r\ndata: [DONE]\r\n\r\n", false},
		{"SSE event prefix", `,"stream":true`, http.StatusOK, "application/octet-stream", "event: message\ndata: [DONE]\n\n", false},
		{"SSE with parameters", `,"stream":true`, http.StatusOK, "Text/Event-Stream; charset=utf-8", "data: [DONE]\n\n", false},
		{"explicit nonstream", `,"stream":false`, http.StatusOK, "application/json", jsonReply, false},
		{"default", "", http.StatusOK, "application/json", jsonReply, false},
		{"streaming upstream error", `,"stream":true`, http.StatusBadGateway, "application/json", jsonReply, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				io.WriteString(w, test.reply)
			}))
			t.Cleanup(server.Close)
			h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
			account := credential(time.Now(), "user", "org")
			scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
			request := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
				Options: []byte(`{"method":"POST","path":"/v1/chat/completions"}`),
				Body:    []byte(`{"model":"deepseek-v4.1-flash","messages":[]` + test.stream + `}`)}
			frame, body := h.invoke(sdk.MethodProviderExecute, scope, request, raw(account))
			if !h.accepted[frame.ID] || requests.Load() != 1 {
				t.Fatalf("expected one accepted inference: calls=%d accepted=%v", requests.Load(), h.accepted[frame.ID])
			}
			if test.truncated {
				if frame.Error == nil || frame.Error.Code != "truncated" || frame.Error.Outcome != "uncertain" ||
					len(h.headers) != 0 || len(body) != 0 {
					t.Fatalf("non-SSE streaming reply: headers=%+v body=%q error=%+v", h.headers, body, frame.Error)
				}
				return
			}
			var result sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
				result.Outcome != "completed" || result.Status != test.status ||
				len(h.headers) != 1 || h.headers[0].Status != test.status || string(body) != test.reply {
				t.Fatalf("ordinary HTTP response changed: headers=%+v body=%q result=%+v error=%+v", h.headers, body, result, frame.Error)
			}
			if test.stream == `,"stream":true` && test.status == http.StatusOK &&
				!strings.HasPrefix(h.headers[0].Headers.Get("Content-Type"), "text/event-stream") {
				t.Fatal("valid stream reached the host without its SSE media type")
			}
		})
	}
}

func TestSDKOpenCodeProjectRefusalPrecedesAnyDispatch(t *testing.T) {
	account := credential(time.Now(), "user", "org")
	personal := ZenCredentials{Kind: "key", Key: "fixture-personal"}
	public := ZenCredentials{Kind: "public", Key: "public"}
	chat := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
		Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`)}
	gemini := sdk.ExecuteRequest{Protocol: "gemini", Model: "fixture-gemini",
		Body: []byte(`{"contents":[{"parts":[{"text":"fixture"}]}]}`)}
	for _, target := range []struct {
		scope   sdk.Scope
		auth    json.RawMessage
		request sdk.ExecuteRequest
	}{
		{sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}, raw(account), chat},
		{zenScope(ProviderZen, personal), raw(personal), gemini},
		{zenScope(ProviderZenPublic, public), raw(public), gemini},
	} {
		for _, scenario := range []struct {
			name, header, code string
		}{
			{"missing", "", "project_required"},
			{"malformed", "invalid-fixture-project", "invalid_project"},
		} {
			t.Run(target.scope.Provider+"/"+scenario.name, func(t *testing.T) {
				harness := serveHarness(t, NewWindowModule(ModuleOptions{
					HTTPFactory: func(string) (Doer, func(), error) {
						t.Error("project rejection reached the network factory")
						return nil, nil, fmt.Errorf("fixture refused network use")
					},
				}), raw(moduleConfig{}))
				request := target.request
				if scenario.header != "" {
					request.Headers = http.Header{}
					request.Headers.Set("X-Opencode-Project", scenario.header)
				}
				frame, body := harness.invoke(sdk.MethodProviderExecute, target.scope, request, target.auth)
				if frame.Error == nil || frame.Error.Code != scenario.code || frame.Error.Outcome != "not-started" {
					t.Errorf("wrong project classification: %+v", frame.Error)
				}
				if len(harness.proxies) != 0 || len(harness.accepted) != 0 || len(harness.headers) != 0 || len(body) != 0 {
					t.Errorf("project refusal published SDK events or used the network: proxy=%d accepted=%d headers=%d body=%q",
						len(harness.proxies), len(harness.accepted), len(harness.headers), body)
				}
			})
		}
	}
}

func TestSDKOpenCodeConfiguredProjectKeepsHeaderPrecedence(t *testing.T) {
	const project = "0123456789abcdef0123456789abcdef01234567"
	for _, scenario := range []struct {
		name, configured, header, want string
	}{
		{"explicit_header", "", project, project},
		{"configured_over_header", "global", project, "global"},
		{"configured_over_malformed_header", "global", "invalid-fixture-project", "global"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var requests atomic.Int32
			const fixtureReply = `{"message":"fixture response"}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				if project := request.Header.Get("X-Opencode-Project"); project != scenario.want {
					t.Errorf("wire project=%q, want %q", project, scenario.want)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, fixtureReply)
			}))
			t.Cleanup(server.Close)
			harness := serveHarness(t, NewModule(ModuleOptions{
				Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) {
					return server.Client(), nil, nil
				},
			}), raw(moduleConfig{ProjectID: scenario.configured}))
			account := credential(time.Now(), "user", "org")
			scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
			request := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
				Options: []byte(`{"method":"POST","path":"/v1/chat/completions"}`),
				Body:    []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`),
				Headers: http.Header{"X-Opencode-Project": {scenario.header}}}
			frame, body := harness.invoke(sdk.MethodProviderExecute, scope, request, raw(account))
			if frame.Error != nil || !harness.accepted[frame.ID] || requests.Load() != 1 ||
				len(harness.headers) != 1 || string(body) != fixtureReply {
				t.Errorf("valid project did not dispatch once: accepted=%v requests=%d headers=%d body=%q error=%+v",
					harness.accepted[frame.ID], requests.Load(), len(harness.headers), body, frame.Error)
			}
		})
	}
}

func TestPublicSDKNonSSEStreamingResponseClosesWithoutEOF(t *testing.T) {
	for _, name := range []string{"Go Chat", "Go Gemini", "Zen Free"} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			var closeOnce sync.Once
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				defer closeOnce.Do(func() { close(closed) })
				writer.Header().Set("Content-Type", "application/json")
				io.WriteString(writer, "{")
				writer.(http.Flusher).Flush()
				<-request.Context().Done()
			}))
			t.Cleanup(server.Close)
			options := ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}
			account := credential(time.Now(), "user", "org")
			scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
			auth := raw(account)
			request := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
				Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[],"stream":true}`)}
			var harness *sdkHarness
			switch name {
			case "Go Gemini":
				request = sdk.ExecuteRequest{Protocol: "gemini", Model: "gemini-4-argon",
					Body:    []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
					Options: []byte(`{"method":"POST","path":"/v1beta/models/gemini-4-argon:streamGenerateContent","query":"alt=sse"}`)}
			case "Zen Free":
				identity := ZenCredentials{Kind: "public", Key: "public"}
				scope, auth = zenScope(ProviderZenPublic, identity), raw(identity)
				request.Model = "fixture-free"
				request.Body = []byte(`{"model":"fixture-free","messages":[],"stream":true}`)
				harness = serveHarness(t, policyFixtureModule(options, "chat", request.Model), raw(moduleConfig{ProjectID: "global"}))
			}
			if harness == nil {
				harness = newHarness(t, options)
			}
			frame, body := harness.invoke(sdk.MethodProviderExecute, scope, request, auth)
			if !harness.accepted[frame.ID] || requests.Load() != 1 ||
				frame.Error == nil || frame.Error.Code != "truncated" || frame.Error.Outcome != "uncertain" ||
				len(harness.headers) != 0 || len(body) != 0 {
				t.Fatalf("unexpected response: calls=%d headers=%+v body=%q error=%+v", requests.Load(), harness.headers, body, frame.Error)
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("upstream response was not closed")
			}
		})
	}
}

func TestPublicSDKTruncatedStreamIsUncertainWithoutReplay(t *testing.T) {
	now := time.Now()
	var complete atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"[DONE] is just text\"}}]}\n\n")
		if complete.Load() {
			io.WriteString(w, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	account := credential(now, "user", "org")
	scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1, Session: "conversation"}
	request := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
		Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[],"stream":true}`)}
	truncated, _ := h.invoke(sdk.MethodProviderExecute, scope, request, raw(account))
	if truncated.Error == nil || truncated.Error.Code != "truncated" || truncated.Error.Outcome != "uncertain" || requests.Load() != 1 {
		t.Fatalf("truncated stream became complete or was replayed: %+v", truncated.Error)
	}
	complete.Store(true)
	finished, _ := h.invoke(sdk.MethodProviderExecute, scope, request, raw(account))
	if finished.Error != nil || requests.Load() != 2 {
		t.Fatalf("valid terminal event was lost: %+v", finished.Error)
	}
}

func TestManifestUsesOnlyMinimumPublicContract(t *testing.T) {
	module := NewModule(ModuleOptions{})
	if err := module.Validate(); err != nil {
		t.Fatal(err)
	}
	if module.Manifest.Contract.Version.Minor != 12 || !module.Manifest.Contract.Has(sdk.RequestPolicy) || module.Manifest.Contract.Has(sdk.OperationControl) ||
		module.Manifest.Contract.Has(sdk.AgentWiring) || len(module.Manifest.Providers) != 1 {
		t.Fatal("minimum provider slice advertises unrelated capabilities")
	}
	models := Models()
	models[0].Name = "mutated"
	if Models()[0].Name == "mutated" || Models()[0].Price != nil {
		t.Fatal("catalog snapshot mutated or guessed static peak pricing")
	}
}

func TestPublicSDKTerminalClosesHeldHTTPStream(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(closed)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	account := credential(time.Now(), "user", "org")
	scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
	finished, _ := h.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash",
		Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[],"stream":true}`)}, raw(account))
	if finished.Error != nil {
		t.Fatalf("terminal event waited for HTTP EOF: %+v", finished.Error)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("completed vendor stream was not closed")
	}
}

func TestPublicSDKOwnsCompressionNegotiation(t *testing.T) {
	for _, fixture := range []struct {
		name        string
		contentType string
		body        string
		held        bool
	}{
		{"json", "application/json", `{"choices":[{"message":{"content":"decoded"}}]}`, false},
		{"held-sse", "text/event-stream", "data: [DONE]\n\n", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				if values := r.Header.Values("Accept-Encoding"); len(values) != 1 || values[0] != "gzip" {
					t.Errorf("caller changed transport compression negotiation: %q", values)
				}
				w.Header().Set("Content-Type", fixture.contentType)
				w.Header().Set("Content-Encoding", "gzip")
				compressed := gzip.NewWriter(w)
				io.WriteString(compressed, fixture.body)
				if fixture.held {
					compressed.Flush()
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				} else {
					compressed.Close()
				}
			}))
			t.Cleanup(server.Close)
			// Leave HTTPFactory unset: use the actual production transport and
			// its automatic gzip decoding, with a direct fixture proxy result.
			h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL}})
			h.proxyURL = ""
			account := credential(time.Now(), "user", "org")
			scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
			inputHeaders := http.Header{"Accept-Encoding": {"gzip, br"}, "accept-encoding": {"identity"}}
			finished, body := h.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
				Protocol: "chat", Model: "deepseek-v4.1-flash",
				Headers: inputHeaders,
				Body:    []byte(`{"model":"deepseek-v4.1-flash","messages":[],"stream":` + strconv.FormatBool(fixture.held) + `}`),
			}, raw(account))
			if finished.Error != nil || string(body) != fixture.body {
				t.Fatalf("compressed response was not decoded/completed: error=%+v body=%q", finished.Error, body)
			}
			if len(h.headers) != 1 || h.headers[0].Headers.Get("Content-Encoding") != "" || h.headers[0].Headers.Get("Content-Length") != "" {
				t.Fatalf("decoded response retained compressed headers: %+v", h.headers)
			}
			if inputHeaders.Get("Accept-Encoding") != "gzip, br" || inputHeaders["accept-encoding"][0] != "identity" {
				t.Fatal("compression filtering mutated caller headers")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("decoded terminal event did not close held vendor stream")
			}
		})
	}
}

func TestPublicSDKDeviceSlowDownExpiryAndClaimAreNotRepolled(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/auth/device/code":
			io.WriteString(w, `{"device_code":"private-device","user_code":"CODE","verification_uri_complete":"/verify","expires_in":30,"interval":1}`)
		case "/console/auth/device/token":
			polls.Add(1)
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"slow_down"}`)
		default:
			t.Error("unexpected identity/network call")
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		Now:         func() time.Time { return time.Unix(0, clock.Load()) },
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	scope := sdk.Scope{Provider: ProviderGo}
	start, _ := h.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{}, nil)
	var code sdk.AuthStartResult
	json.Unmarshal(start.Result, &code)
	var key continuation
	json.Unmarshal(code.Continuation, &key)
	clock.Add(int64(time.Second))
	pending, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	if pending.Error != nil || polls.Load() != 1 {
		t.Fatalf("slow-down response: %+v", pending.Error)
	}
	recordKey := stateKey(scope, "device."+key.ID)
	value := h.state[recordKey]
	var flow deviceFlow
	json.Unmarshal(value.value, &flow)
	if flow.Interval != 6*time.Second {
		t.Fatal("donor's five-second slow-down increment changed")
	}
	clock.Add(int64(5 * time.Second))
	early, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	if early.Error != nil || polls.Load() != 1 {
		t.Fatal("slow-down interval was bypassed")
	}
	flow.State = "polling"
	value.value = raw(flow)
	h.state[recordKey] = value
	claimed, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	if claimed.Error == nil || polls.Load() != 1 {
		t.Fatal("uncertain claimed poll was retried")
	}
	flow.State = "pending"
	value.value = raw(flow)
	h.state[recordKey] = value
	clock.Store(flow.Deadline.UnixNano())
	expired, _ := h.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: code.Continuation}, nil)
	if expired.Error == nil || expired.Error.Code != "expired" || polls.Load() != 1 {
		t.Fatal("expired authorization was polled")
	}
}

func TestPublicSDKRefusesAccountOrOperationSubstitutionBeforeDispatch(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		t.Error("invalid request reached the vendor fixture")
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	account := credential(time.Now(), "a", "org-a")
	scope := sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}
	base := sdk.ExecuteRequest{Protocol: "chat", Model: "deepseek-v4.1-flash", Body: []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`)}
	other := credential(time.Now(), "b", "org-b")
	mismatch, _ := h.invoke(sdk.MethodProviderExecute, scope, base, raw(other))
	if mismatch.Error == nil || mismatch.Error.Code != "permission_denied" {
		t.Fatal("another account's auth was accepted")
	}
	for _, operation := range []string{"/v1/messages", "/v1/messages/count_tokens", "/v1/images/generations"} {
		request := base
		request.Options = raw(map[string]string{"path": operation})
		frame, _ := h.invoke(sdk.MethodProviderExecute, scope, request, raw(account))
		if frame.Error == nil || frame.Error.Code != "unsupported" {
			t.Fatal("unsupported operation became chat")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("rejected requests were dispatched")
	}
}
