package minimax

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type coreStored struct {
	version int
	body    json.RawMessage
}

type coreHarness struct {
	test            *testing.T
	connection      net.Conn
	encoder         *json.Encoder
	decoder         *json.Decoder
	sequence        int
	state           map[string]coreStored
	events          []sdk.Frame
	callbacks       []sdk.Frame
	cancelAfterData bool
}

type coreDoer func(*http.Request) (*http.Response, error)

func (perform coreDoer) Do(request *http.Request) (*http.Response, error) {
	return perform(request)
}

func coreResponse(status int, body string, contentType string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func coreOptions(now *atomic.Int64, perform coreDoer) ModuleOptions {
	return ModuleOptions{
		Now:            func() time.Time { return time.UnixMilli(now.Load()) },
		TimezoneOffset: func() int { return 0 }, Platform: "linux",
		HTTPFactory: func(proxy string) (Doer, func(), error) {
			if proxy != "http://selected-proxy.invalid:8080" {
				return nil, nil, fmt.Errorf("selected proxy lost")
			}
			return perform, func() {}, nil
		},
	}
}

func newCoreHarness(test *testing.T, configured sdk.Module) *coreHarness {
	test.Helper()
	if err := configured.Validate(); err != nil {
		test.Fatal(err)
	}
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(test.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- sdk.Serve(ctx, worker, worker, configured) }()
	fixture := &coreHarness{
		test: test, connection: host, encoder: json.NewEncoder(host), decoder: json.NewDecoder(host),
		state: map[string]coreStored{},
	}
	test.Cleanup(func() {
		cancel()
		host.Close()
		worker.Close()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			test.Error("MiniMax SDK fixture did not stop")
		}
	})
	configuredFrame, _ := fixture.invoke(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent, Instance: "fixture-instance",
		Config: sdk.ConfigSnapshot{Version: "1", Values: json.RawMessage(`{}`)},
	}, nil)
	if configuredFrame.Error != nil {
		test.Fatal(configuredFrame.Error)
	}
	return fixture
}

func (fixture *coreHarness) invoke(method string, scope sdk.Scope, params any, credentials *Credentials) (sdk.Frame, []byte) {
	fixture.test.Helper()
	fixture.sequence++
	identifier := fmt.Sprintf("core-%d", fixture.sequence)
	scope.Plugin, scope.Operation = ModuleID, identifier
	frame := sdk.Frame{Kind: "call", ID: identifier, Method: method, Scope: scope, Params: raw(params)}
	if credentials != nil {
		frame.Auth = raw(credentials)
	}
	fixture.connection.SetDeadline(time.Now().Add(10 * time.Second))
	if err := fixture.encoder.Encode(frame); err != nil {
		fixture.test.Fatal(err)
	}
	var body []byte
	for {
		frame = sdk.Frame{}
		if err := fixture.decoder.Decode(&frame); err != nil {
			fixture.test.Fatal(err)
		}
		switch frame.Kind {
		case "host.call":
			if frame.Scope != scope || frame.Parent != identifier || len(frame.Auth) != 0 {
				fixture.test.Fatal("host callback crossed invocation authority")
			}
			fixture.callbacks = append(fixture.callbacks, frame)
			var result any
			switch frame.Method {
			case sdk.MethodNetworkProxy:
				result = sdk.NetworkProxyResult{URL: "http://selected-proxy.invalid:8080"}
			case sdk.MethodStateGet:
				var request sdk.StateGetRequest
				json.Unmarshal(frame.Params, &request)
				stored := fixture.state[scope.Provider+"\x00"+scope.Account+"\x00"+request.Key]
				result = sdk.StateValue{Found: stored.version != 0, Version: fmt.Sprint(stored.version), Value: stored.body}
			case sdk.MethodStateCAS:
				var request sdk.StateCASRequest
				json.Unmarshal(frame.Params, &request)
				key := scope.Provider + "\x00" + scope.Account + "\x00" + request.Key
				stored := fixture.state[key]
				swapped := request.Version == fmt.Sprint(stored.version)
				if swapped {
					stored.version++
					stored.body = append(json.RawMessage{}, request.Value...)
					fixture.state[key] = stored
				}
				result = sdk.StateCASResult{Swapped: swapped, Version: fmt.Sprint(stored.version)}
			case sdk.MethodUsageInvalidate:
				result = sdk.UsageInvalidateResult{Revision: "new"}
			default:
				fixture.test.Fatalf("unexpected callback %s", frame.Method)
			}
			if err := fixture.encoder.Encode(sdk.Frame{Kind: "host.result", ID: frame.ID, Parent: identifier, Result: raw(result)}); err != nil {
				fixture.test.Fatal(err)
			}
		case "event":
			if frame.ID != identifier {
				fixture.test.Fatal("foreign output")
			}
			fixture.events = append(fixture.events, frame)
			if frame.Event == "data" {
				var chunk []byte
				json.Unmarshal(frame.Data, &chunk)
				body = append(body, chunk...)
				if fixture.cancelAfterData {
					fixture.cancelAfterData = false
					if err := fixture.encoder.Encode(sdk.Frame{Kind: sdk.KindCancel, ID: identifier}); err != nil {
						fixture.test.Fatal(err)
					}
					continue
				}
			}
			if err := fixture.encoder.Encode(sdk.Frame{Kind: "ack", ID: identifier, Seq: frame.Seq}); err != nil {
				fixture.test.Fatal(err)
			}
		case "result":
			if frame.ID != identifier {
				fixture.test.Fatal("foreign result")
			}
			return frame, body
		default:
			fixture.test.Fatalf("unexpected frame %s", frame.Kind)
		}
	}
}

func coreAccount(site string) (sdk.Scope, Credentials) {
	credentials := Credentials{Site: site, ID: accountID(site, "user-7"), Access: "selected-access", Refresh: "rotating-refresh", UserID: "user-7", ExpiresAt: 1890000000000}
	return sdk.Scope{Provider: site, Account: credentials.ID, Generation: 7}, credentials
}

func TestCoreDonorSignatureVectors(test *testing.T) {
	target, headers, body, err := signed(DefaultSites()[China], DefaultSites()[China].Agent, "/v1/api/user/info", "tok", "", nil, nil, 1790000000123, 28800, "darwin")
	if err != nil || len(body) != 0 || headers.Get("Yy") != "ea384f50aae587ea2048557a2e1b30b1" || headers.Get("X-Signature") != "03015677c012162b07bc060014868ead" {
		test.Fatalf("GET signing mismatch: %s %v %v", target, headers, err)
	}
	_, headers, body, err = signed(DefaultSites()[Global], DefaultSites()[Global].Agent, "/matrix/api/v1/commerce/get_membership_info", "tok", "u-42", map[string]any{"workspace_id": 7}, nil, 1790000000123, 0, "linux")
	if err != nil || string(body) != `{"workspace_id":7}` || headers.Get("Yy") != "078fc5529cd0bee0e3f4b587421d726d" || headers.Get("X-Signature") != "cdc435f7a98e23c841ecd445dfa82439" {
		test.Fatalf("POST signing mismatch: %s %v %v", body, headers, err)
	}
}

func TestCoreDevicePKCEVariantsAndConsumption(test *testing.T) {
	for _, variant := range []string{"standard", "user-code", "user-default"} {
		test.Run(variant, func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			polls := 0
			challenge := ""
			transport := coreDoer(func(request *http.Request) (*http.Response, error) {
				request.ParseForm()
				switch request.URL.Path {
				case "/oauth2/device/code":
					challenge = request.Form.Get("code_challenge")
					if request.Form.Get("code_challenge_method") != "S256" || request.Form.Get("client_id") != "mcode-public" {
						test.Error("device PKCE fields lost")
					}
					envelope := map[string]any{"user_code": "user-code-secret", "verification_uri": "https://account.minimax.cn/device", "expires_in": 600, "interval": 5}
					if variant == "standard" {
						envelope["device_code"] = "device-secret"
					} else {
						delete(envelope, "expires_in")
						envelope["expired_in"] = now.Load() + 600000
						envelope["interval"] = 5000
						if variant == "user-default" {
							delete(envelope, "interval")
						}
					}
					return coreResponse(200, string(raw(envelope)), "application/json"), nil
				case "/oauth2/token":
					polls++
					verifier := request.Form.Get("code_verifier")
					digest := sha256.Sum256([]byte(verifier))
					if verifier == "" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
						test.Error("PKCE exchange mismatch")
					}
					if variant == "standard" && request.Form.Get("device_code") != "device-secret" || variant != "standard" && request.Form.Get("user_code") != "user-code-secret" {
						test.Error("wrong device grant variant")
					}
					if polls == 1 {
						return coreResponse(400, `{"error":"authorization_pending"}`, "application/json"), nil
					}
					return coreResponse(200, `{"access_token":"opaque-access","refresh_token":"rotated","expires_in":3600}`, "application/json"), nil
				case "/v1/api/user/info":
					return coreResponse(200, `{"data":{"userInfo":{"realUserID":"user-7","email":"fixture@example.invalid"}}}`, "application/json"), nil
				default:
					test.Errorf("unexpected device request %s", request.URL.Path)
					return nil, fmt.Errorf("unexpected endpoint")
				}
			})
			fixture := newCoreHarness(test, NewModule(coreOptions(&now, transport)))
			scope := sdk.Scope{Provider: China}
			started, _ := fixture.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "device"}, nil)
			if started.Error != nil {
				test.Fatal(started.Error)
			}
			var start sdk.AuthStartResult
			json.Unmarshal(started.Result, &start)
			if start.PollAfterMs != 5000 || strings.Contains(string(start.Continuation), "secret") || strings.Contains(string(start.Continuation), "Verifier") {
				test.Fatal("unsafe continuation or interval")
			}
			finish := sdk.AuthFinishRequest{Continuation: start.Continuation}
			fixture.invoke(sdk.MethodAuthFinish, scope, finish, nil)
			if polls != 0 {
				test.Fatal("polled before interval")
			}
			now.Add(5000)
			pending, _ := fixture.invoke(sdk.MethodAuthFinish, scope, finish, nil)
			if pending.Error != nil || !strings.Contains(string(pending.Result), `"pending":true`) {
				test.Fatal("pending response lost")
			}
			now.Add(5000)
			finished, _ := fixture.invoke(sdk.MethodAuthFinish, scope, finish, nil)
			var result sdk.AuthResult
			json.Unmarshal(finished.Result, &result)
			if finished.Error != nil || result.ID != accountID(China, "user-7") {
				test.Fatalf("finish: %s %+v", finished.Result, finished.Error)
			}
			replay, _ := fixture.invoke(sdk.MethodAuthFinish, scope, finish, nil)
			if replay.Error == nil || polls != 2 {
				test.Fatal("consumed device grant replayed")
			}
		})
	}
}

func TestCoreRefreshRotatesOnceAndPreservesSelectedIdentity(test *testing.T) {
	for _, status := range []int{200, 400, 401, 500} {
		test.Run(fmt.Sprint(status), func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			count := 0
			scope, credentials := coreAccount(Global)
			transport := coreDoer(func(request *http.Request) (*http.Response, error) {
				count++
				request.ParseForm()
				if request.Form.Get("refresh_token") != credentials.Refresh || request.Form.Get("grant_type") != "refresh_token" {
					test.Error("refresh owner lost selected token")
				}
				if status != 200 {
					return coreResponse(status, `{"error":"invalid_grant"}`, "application/json"), nil
				}
				return coreResponse(200, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`, "application/json"), nil
			})
			fixture := newCoreHarness(test, NewModule(coreOptions(&now, transport)))
			frame, _ := fixture.invoke(sdk.MethodAuthRefresh, scope, sdk.AuthRefreshRequest{}, &credentials)
			if count != 1 {
				test.Fatal("rotating refresh replayed")
			}
			if status == 200 {
				var result sdk.AuthResult
				var replacement Credentials
				json.Unmarshal(frame.Result, &result)
				json.Unmarshal(result.Credentials, &replacement)
				if frame.Error != nil || replacement.ID != credentials.ID || replacement.Refresh != "new-refresh" {
					test.Fatal("rotation changed account or lost refresh")
				}
			} else if frame.Error == nil || frame.Error.Outcome != "uncertain" || (frame.Error.Code == "auth_revoked") != (status == 400) {
				test.Fatalf("wrong refresh classification: %+v", frame.Error)
			}
			foreign := scope
			foreign.Account = "another-account"
			rejected, _ := fixture.invoke(sdk.MethodAuthRefresh, foreign, sdk.AuthRefreshRequest{}, &credentials)
			if rejected.Error == nil || rejected.Error.Outcome != "not-started" || count != 1 {
				test.Fatal("foreign refresh dispatched")
			}
		})
	}
}

func TestCoreChatPreservesWireAndSparseUsage(test *testing.T) {
	for _, streaming := range []bool{false, true} {
		test.Run(fmt.Sprint(streaming), func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			scope, credentials := coreAccount(China)
			input := []byte(`{ "model":"MiniMax-M3", "messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"opaque","signature":"keep+exact=="},{"type":"tool_use","id":"tool-1","name":"fn","input":{"x":1}}]}], "max_tokens":42 }`)
			response := `{"id":"response-7","model":"MiniMax-M3","type":"message","content":[{"type":"text","text":"selected-access"}],"usage":{"input_tokens":0}}`
			contentType := "application/json"
			if streaming {
				contentType = "text/event-stream"
				response = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"response-7\",\"usage\":{\"input_tokens\":0}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"signature_delta\",\"signature\":\"keep+exact==\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
			}
			sessions := []string{}
			transport := coreDoer(func(request *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(request.Body)
				if string(body) != string(input) || request.Header.Get("Authorization") != "Bearer "+credentials.Access || request.Header.Get("Accept-Encoding") != "identity" {
					test.Error("chat wire changed")
				}
				sessions = append(sessions, request.Header.Get("X-Mavis-Session-Id"))
				return coreResponse(200, response, contentType), nil
			})
			fixture := newCoreHarness(test, NewModule(coreOptions(&now, transport)))
			for attempt := 0; attempt < 2; attempt++ {
				frame, output := fixture.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: input}, &credentials)
				var result sdk.ExecuteResult
				json.Unmarshal(frame.Result, &result)
				if frame.Error != nil || string(output) != response {
					test.Fatalf("chat failed: %+v %s", frame.Error, output)
				}
				if result.Usage == nil || result.Usage.Facts == nil || result.Usage.Facts.InputTokens == nil || *result.Usage.Facts.InputTokens != 0 || result.Usage.Facts.OutputTokens != nil {
					test.Fatal("zero and absent usage were conflated")
				}
			}
			if sessions[0] == "" || sessions[0] != sessions[1] {
				test.Fatal("missing session became random per request")
			}
		})
	}
}

func TestCoreChatEOFAndRefusalsNeverReplay(test *testing.T) {
	cases := []struct {
		name              string
		status            int
		body, contentType string
		wantStatus        int
		uncertain         bool
	}{
		{"incomplete", 200, "data: {\"type\":\"message_start\"}\n\n", "text/event-stream", 200, true},
		{"unframed-stop", 200, "data: {\"type\":\"message_stop\"}", "text/event-stream", 200, true},
		{"stream-error", 200, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\"}}\n\n", "text/event-stream", 200, true},
		{"credit", 403, `{"error":"insufficient credit"}`, "application/json", 429, false},
		{"permission", 403, `{"error":"content policy denied"}`, "application/json", 403, false},
		{"unauthorized", 401, `{"error":"invalid token"}`, "application/json", 401, false},
		{"business-credit", 200, `{"base_resp":{"status_code":1008,"status_msg":"insufficient balance"}}`, "application/json", 200, false},
		{"business-unknown", 200, `{"base_resp":{"status_code":2013,"status_msg":"invalid params"}}`, "application/json", 502, false},
	}
	for _, scenario := range cases {
		test.Run(scenario.name, func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			requests := 0
			fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(*http.Request) (*http.Response, error) {
				requests++
				return coreResponse(scenario.status, scenario.body, scenario.contentType), nil
			}))))
			scope, credentials := coreAccount(Global)
			frame, _ := fixture.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}, &credentials)
			if requests != 1 {
				test.Fatal("refusal was replayed")
			}
			if scenario.uncertain {
				if frame.Error == nil || frame.Error.Outcome != "uncertain" {
					test.Fatalf("incomplete stream marked successful: %+v", frame)
				}
			} else {
				var result sdk.ExecuteResult
				json.Unmarshal(frame.Result, &result)
				if frame.Error != nil || result.Status != scenario.wantStatus {
					test.Fatalf("refusal changed: %+v", frame)
				}
				if scenario.name == "business-credit" && (result.Refusal == nil || result.Outcome != "not-started") {
					test.Fatal("business refusal lost its original status or rejection evidence")
				}
			}
		})
	}
}

func TestCoreRedirectAndExpiryDoNotCrossAuthority(test *testing.T) {
	for _, destination := range []string{"https://agent.minimax.io/foreign", "http://agent.minimax.cn/downgrade"} {
		test.Run(destination, func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			requests := 0
			fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
				requests++
				if request.URL.Host != "agent.minimax.cn" {
					test.Error("credentials crossed origin")
				}
				response := coreResponse(307, "", "text/plain")
				response.Header.Set("Location", destination)
				return response, nil
			}))))
			scope, credentials := coreAccount(China)
			request := sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}
			frame, _ := fixture.invoke(sdk.MethodProviderExecute, scope, request, &credentials)
			if frame.Error == nil || requests != 1 {
				test.Fatal("foreign redirect followed")
			}
			credentials.ExpiresAt = now.Load() - 1
			expired, _ := fixture.invoke(sdk.MethodProviderExecute, scope, request, &credentials)
			if expired.Error == nil || expired.Error.Outcome != "not-started" || requests != 1 {
				test.Fatal("expired account dispatched")
			}
		})
	}
}

func TestCoreMediaURLPinsAndRejectsPrivate(test *testing.T) {
	for _, address := range []string{"https://127.0.0.1/file", "https://[::1]/file", "https://100.64.0.1/file", "https://203.0.113.77/file", "http://8.8.8.8/file", "https://name:secret@8.8.8.8/file"} {
		if _, err := publicURL(test.Context(), address); err == nil {
			test.Fatalf("unsafe media destination admitted: %s", address)
		}
	}
	destination, err := publicURL(test.Context(), "https://8.8.8.8:8443/file?signature=synthetic")
	if err != nil || destination.address != "8.8.8.8:8443" || destination.serverName != "8.8.8.8" {
		test.Fatalf("destination not pinned: %+v %v", destination, err)
	}
	encoded, err := url.Parse("https://8.8.8.8/file")
	if err != nil || encoded.Hostname() != destination.serverName {
		test.Fatal("origin mismatch")
	}
}
