package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type toolsProtocolDoer func(*http.Request) (*http.Response, error)

func (do toolsProtocolDoer) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func toolsProtocolCall(test *testing.T, module sdk.Module, call sdk.Frame, onFrame func(sdk.Frame), policy ...func(sdk.Frame) sdk.RequestRecoveryResult) sdk.Frame {
	test.Helper()
	var reply func(sdk.Frame) json.RawMessage
	if len(policy) != 0 {
		reply = func(frame sdk.Frame) json.RawMessage { return raw(policy[0](frame)) }
	}
	return toolsProtocolReplyCall(test, module, call, onFrame, reply)
}

func toolsProtocolReplyCall(test *testing.T, module sdk.Module, call sdk.Frame, onFrame func(sdk.Frame), policy func(sdk.Frame) json.RawMessage) sdk.Frame {
	test.Helper()
	if err := module.Validate(); err != nil {
		test.Fatal(err)
	}
	input, send := io.Pipe()
	receive, output := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	frames := make(chan sdk.Frame)
	readerDone := make(chan struct{})
	go func() {
		done <- sdk.Serve(ctx, input, output, module)
	}()
	go func() {
		defer close(readerDone)
		defer close(frames)
		decoder := json.NewDecoder(receive)
		for {
			var frame sdk.Frame
			if decoder.Decode(&frame) != nil {
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	test.Cleanup(func() {
		cancel()
		send.Close()
		receive.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				test.Error(err)
			}
		case <-time.After(5 * time.Second):
			test.Error("SDK worker did not stop")
		}
		select {
		case <-readerDone:
		case <-time.After(5 * time.Second):
			test.Error("SDK fixture reader did not stop")
		}
	})
	encoder := json.NewEncoder(send)
	write := func(frame sdk.Frame) {
		if err := encoder.Encode(frame); err != nil {
			test.Fatal(err)
		}
	}
	write(sdk.Frame{Kind: sdk.KindCall, ID: "configure", Method: sdk.MethodComponentConfigure, Params: raw(sdk.ComponentConfigureRequest{Component: sdk.PrimaryComponent, Instance: "fixture-instance", Config: sdk.ConfigSnapshot{Version: "1", Values: json.RawMessage(`{}`)}})})
	select {
	case configured := <-frames:
		if configured.ID != "configure" || configured.Error != nil {
			test.Fatalf("SDK configure failed: %+v", configured)
		}
	case <-time.After(5 * time.Second):
		test.Fatal("SDK configure timed out")
	}
	write(call)
	for {
		var frame sdk.Frame
		select {
		case value, open := <-frames:
			if !open {
				test.Fatal("SDK closed before a terminal result")
			}
			frame = value
		case <-time.After(5 * time.Second):
			test.Fatal("SDK call timed out")
		}
		onFrame(frame)
		switch frame.Kind {
		case sdk.KindEvent:
			write(sdk.Frame{Kind: sdk.KindAck, ID: frame.ID, Seq: frame.Seq})
		case sdk.KindHostCall:
			result := json.RawMessage(`null`)
			switch frame.Method {
			case sdk.MethodNetworkProxy:
				result = raw(sdk.NetworkProxyResult{URL: "http://selected.proxy.invalid:8080"})
			case sdk.MethodUsageAppend:
				result = raw(sdk.OwnCallResult{Accepted: true})
			case sdk.MethodRequestRecover:
				result = raw(sdk.RequestRecoveryResult{Action: "terminal", Reason: "fixture-denied"})
				if policy != nil {
					result = policy(frame)
				}
			default:
				test.Fatalf("unexpected host callback %s", frame.Method)
			}
			write(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent, Result: result})
		case sdk.KindResult:
			return frame
		}
	}
}

func toolsProtocolFrame(method string, parameters any) sdk.Frame {
	return sdk.Frame{
		Kind:   sdk.KindCall,
		ID:     "fixture-call",
		Method: method,
		Params: raw(parameters),
		Scope:  sdk.Scope{Plugin: ModuleID, Provider: Global, Account: "exact-account", Generation: 7, Operation: "operation-1", Attempt: 1},
		Auth:   raw(Credentials{Site: Global, ID: "exact-account", Access: "fixture-access", Refresh: "fixture-refresh", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}),
	}
}

func TestToolsSDKAcceptedWriteProxyUsageAndUncertainFailure(test *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "business-failure"
		}
		test.Run(name, func(test *testing.T) {
			var mutex sync.Mutex
			requests := []string{}
			module := NewModule(ModuleOptions{HTTPFactory: func(proxy string) (Doer, func(), error) {
				if proxy != "http://selected.proxy.invalid:8080" {
					test.Error("host-selected proxy was not used")
				}
				return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
					mutex.Lock()
					defer mutex.Unlock()
					requests = append(requests, request.URL.Path)
					if request.URL.Host != "agent.minimax.io" || request.Header.Get("Authorization") != "Bearer fixture-access" || request.Header.Get("User-Agent") != "MiniMaxAgent" {
						test.Error("selected provider/account tool headers changed")
					}
					response := any(toolsCatalog(toolsEntry("generate_image", `{"type":"object"}`)))
					if request.URL.Path == matrixCallPath {
						if fail {
							response = map[string]any{"is_error": true, "error_message": "fixture-access fixture-refresh", "result_json": "{}"}
						} else {
							response = toolsSuccess(map[string]any{"node_id": "node"})
						}
					}
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw(response)))}, nil
				}), func() {}, nil
			}})
			accepted, proxies, receipts := 0, 0, 0
			call := toolsProtocolFrame(sdk.MethodToolCall, sdk.ToolRequest{ID: "generate_image", Arguments: raw(map[string]any{"requests": []any{map[string]any{"prompt": "fixture"}}})})
			terminal := toolsProtocolCall(test, module, call, func(frame sdk.Frame) {
				if frame.Event == "accepted" {
					accepted++
				}
				if frame.Kind != sdk.KindHostCall {
					return
				}
				if frame.Scope.Account != call.Scope.Account || frame.Scope.Provider != call.Scope.Provider || frame.Scope.Generation != 7 {
					test.Fatal("host callback lost frozen selection")
				}
				if frame.Method == sdk.MethodNetworkProxy {
					proxies++
				}
				if frame.Method == sdk.MethodUsageAppend {
					receipts++
					var receipt sdk.OwnCallRequest
					if json.Unmarshal(frame.Params, &receipt) != nil || receipt.Call.Model != "generate_image" || receipt.Call.Kind != "tool" || receipt.Call.Usage.Facts == nil || receipt.Call.Usage.Facts.Cost != nil {
						test.Fatal("own-call receipt fabricated usage or lost identity")
					}
					if accepted != 1 {
						test.Fatal("usage receipt before write acceptance")
					}
				}
			})
			if fail {
				if terminal.Error == nil || terminal.Error.Outcome != "uncertain" || strings.Contains(terminal.Error.Message, "fixture-access") || strings.Contains(terminal.Error.Message, "fixture-refresh") {
					test.Fatalf("accepted failure lost uncertainty or exposed credentials: %+v", terminal.Error)
				}
			} else if terminal.Error != nil {
				test.Fatal(terminal.Error)
			}
			mutex.Lock()
			defer mutex.Unlock()
			if len(requests) != 2 || accepted != 1 || proxies != 2 || receipts != 1 {
				test.Fatalf("paid call replayed or host receipt lost: requests=%v accepted=%d proxy=%d receipts=%d", requests, accepted, proxies, receipts)
			}
		})
	}
}

func TestToolsSDKAccountMismatchAndExpiredCredentialsStopBeforeNetwork(test *testing.T) {
	for _, credentials := range []Credentials{{Site: China, ID: "exact-account", Access: "token"}, {Site: Global, ID: "other-account", Access: "token"}, {Site: Global, ID: "exact-account", Access: "token", ExpiresAt: 1}} {
		module := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
			test.Error("rejected account reached network factory")
			return nil, nil, errors.New("unexpected network")
		}})
		call := toolsProtocolFrame(sdk.MethodToolCall, sdk.ToolRequest{ID: "list_tools", Arguments: raw(map[string]any{})})
		call.Auth = raw(credentials)
		terminal := toolsProtocolCall(test, module, call, func(frame sdk.Frame) {
			if frame.Kind == sdk.KindHostCall || frame.Kind == sdk.KindEvent {
				test.Fatal("rejected selection reached host service or acceptance")
			}
		})
		if terminal.Error == nil {
			test.Fatal("expired or mismatched account accepted")
		}
		if credentials.ExpiresAt == 1 && (terminal.Error.Code != "auth_refresh_required" || terminal.Error.Outcome != "not-started") {
			test.Fatalf("expired token was not classified before dispatch: %+v", terminal.Error)
		}
		if credentials.ExpiresAt != 1 && terminal.Error.Code != "permission_denied" {
			test.Fatalf("mismatched identity did not reach selection validation: %+v", terminal.Error)
		}
	}
}

func TestSearchSDKSelectedAccountSuccessAndRateLimit(test *testing.T) {
	for _, refused := range []bool{false, true} {
		status := http.StatusOK
		if refused {
			status = http.StatusTooManyRequests
		}
		module := NewModule(ModuleOptions{HTTPFactory: func(proxy string) (Doer, func(), error) {
			return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
				if proxy != "http://selected.proxy.invalid:8080" || request.URL.Path != webSearchPath || request.URL.Host != "agent.minimax.io" || request.Header.Get("Authorization") != "Bearer fixture-access" {
					test.Error("native search escaped selected account/proxy")
				}
				var body map[string]any
				if json.NewDecoder(request.Body).Decode(&body) != nil || body["query"] != "fixture" || body["count"] != float64(2) || len(body) != 2 {
					test.Error("native search did not trim query/preserve count")
				}
				payload := map[string]any{"organic": []any{map[string]any{"title": "Fixture", "url": "https://example.test/", "snippet": "Text"}}}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw(payload)))}, nil
			}), func() {}, nil
		}})
		request := sdk.ModuleCallRequest{Action: "search." + Global, Input: raw(sdk.SearchRequest{Source: Global, Query: " fixture ", Count: 2})}
		call := toolsProtocolFrame(sdk.MethodModuleCall, request)
		proxies := 0
		terminal := toolsProtocolCall(test, module, call, func(frame sdk.Frame) {
			if frame.Kind == sdk.KindEvent || frame.Method == sdk.MethodUsageAppend {
				test.Fatal("read-only search emitted write acceptance or own-call receipt")
			}
			if frame.Method == sdk.MethodNetworkProxy {
				proxies++
			}
		})
		var result sdk.SearchResult
		if terminal.Error != nil || json.Unmarshal(terminal.Result, &result) != nil || result.Validate(2) != nil || proxies != 1 {
			test.Fatalf("invalid native search result: %s %v", terminal.Result, terminal.Error)
		}
		if refused && (result.Status != 429 || len(result.Pages) != 0) || !refused && (len(result.Pages) != 1 || result.Pages[0].Text != "Text") {
			test.Fatalf("search success/refusal mapping lost: %+v", result)
		}
	}
}

func TestMediaSDKAcceptedAssetFailureIsUncertainAndNeverRegenerates(test *testing.T) {
	var mutex sync.Mutex
	generated, assets := 0, 0
	module := NewModule(ModuleOptions{HTTPFactory: func(string) (Doer, func(), error) {
		return toolsProtocolDoer(func(request *http.Request) (*http.Response, error) {
			mutex.Lock()
			defer mutex.Unlock()
			status := 200
			payload := any(toolsCatalog(toolsEntry("generate_image", "{}"), toolsEntry("get_asset_url", "{}")))
			if request.URL.Path == matrixCallPath {
				var body map[string]any
				if json.NewDecoder(request.Body).Decode(&body) != nil {
					return nil, errors.New("invalid fixture request")
				}
				if body["runtime_tool_name"] == matrixPrefix+"generate_image" {
					generated++
					payload = toolsSuccess(map[string]any{"success_items": []any{map[string]any{"node_id": "generated-node"}}})
				} else {
					assets++
					status = 401
					payload = map[string]any{"error": "fixture expired token"}
				}
			}
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw(payload)))}, nil
		}), func() {}, nil
	}})
	accepted := 0
	call := toolsProtocolFrame(sdk.MethodProviderExecute, mediaRequest(sdk.OperationImageGenerate, imageModel, map[string]any{"prompt": "fixture"}))
	terminal := toolsProtocolCall(test, module, call, func(frame sdk.Frame) {
		if frame.Event == "accepted" {
			accepted++
		}
	})
	mutex.Lock()
	defer mutex.Unlock()
	if terminal.Error == nil || terminal.Error.Outcome != "uncertain" || generated != 1 || assets != 1 || accepted != 1 {
		test.Fatalf("paid media failure replayed or lost uncertainty: %v generate=%d asset=%d accepted=%d", terminal.Error, generated, assets, accepted)
	}
}
