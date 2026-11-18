package minimax

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type coreCancelBody struct {
	context context.Context
	sent    bool
	closed  atomic.Bool
}

func (body *coreCancelBody) Read(buffer []byte) (int, error) {
	if !body.sent {
		body.sent = true
		return copy(buffer, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0}}}\n\n"), nil
	}
	<-body.context.Done()
	return 0, body.context.Err()
}

func (body *coreCancelBody) Close() error {
	body.closed.Store(true)
	return nil
}

func TestCoreCancellationStopsOutputWithoutRestingAnotherAccount(test *testing.T) {
	var now atomic.Int64
	now.Store(1790000000000)
	var body *coreCancelBody
	requests := 0
	fixture := newCoreHarness(test, NewModule(coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			body = &coreCancelBody{context: request.Context()}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, nil
		}
		if request.Header.Get("Authorization") != "Bearer another-access" {
			test.Error("another account inherited canceled credentials")
		}
		return coreResponse(200, `{"type":"message","usage":{"output_tokens":7}}`, "application/json"), nil
	}))))
	scope, credentials := coreAccount(Global)
	request := sdk.ExecuteRequest{Model: "MiniMax-M3", Protocol: "anthropic", Body: []byte(`{"model":"MiniMax-M3","messages":[]}`)}
	fixture.cancelAfterData = true
	canceled, output := fixture.invoke(sdk.MethodProviderExecute, scope, request, &credentials)
	if canceled.Error == nil || len(output) == 0 || body == nil || !body.closed.Load() {
		test.Fatal("stream was buffered or did not close on cancellation")
	}
	for _, event := range fixture.events {
		if event.Event == "usage" {
			test.Fatal("usage published after cancellation")
		}
	}
	credentials.ID, credentials.Access = accountID(Global, "another-user"), "another-access"
	scope.Account, scope.Generation = credentials.ID, 8
	completed, _ := fixture.invoke(sdk.MethodProviderExecute, scope, request, &credentials)
	if completed.Error != nil || requests != 2 {
		test.Fatalf("cancellation leaked to another selected account: %+v", completed.Error)
	}
}

func TestCoreAccountFallbackRequiresProvedNoDispatch(test *testing.T) {
	for _, mode := range []string{"dial", "written", "timeout", "http401", "http500", "business", "redirect"} {
		test.Run(mode, func(test *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			requests := 0
			options := coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
				requests++
				if requests > 1 {
					if request.URL.Host != "agent.minimax.cn" {
						test.Error("wrong CN fallback origin")
					}
					return coreResponse(200, `{}`, "application/json"), nil
				}
				switch mode {
				case "dial":
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
				case "written":
					httptrace.ContextClientTrace(request.Context()).WroteRequest(httptrace.WroteRequestInfo{})
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("late failure")}
				case "timeout":
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}
				case "http401":
					return coreResponse(401, `{}`, "application/json"), nil
				case "http500":
					return coreResponse(500, `{}`, "application/json"), nil
				case "business":
					return coreResponse(200, `{"base_resp":{"status_code":1000048}}`, "application/json"), nil
				default:
					response := coreResponse(307, "", "text/plain")
					response.Header.Set("Location", "https://agent.minimax.io/foreign")
					return response, nil
				}
			}))
			options.Sites = DefaultSites()
			owner := &module{options: options}
			configured := NewModule(options)
			configured.Handlers[sdk.MethodProviderModels] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
				client, err := owner.SelectedClient(ctx, call, stream, "")
				if err != nil {
					return nil, err
				}
				_, err = client.accountJSON(ctx, "/fixture", map[string]any{"same": "body"}, nil, "")
				return sdk.ModelsResult{}, err
			}
			fixture := newCoreHarness(test, configured)
			scope, credentials := coreAccount(China)
			result, _ := fixture.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, &credentials)
			if mode == "dial" {
				if requests != 2 || result.Error != nil {
					test.Fatalf("proved dial fallback failed: %d %+v", requests, result.Error)
				}
			} else if requests != 1 || result.Error == nil {
				test.Fatalf("unsafe request repeated: %s %d", mode, requests)
			}
		})
	}
}

func TestCoreTransferUsesOriginalPolicyAndNeverAccountHeaders(test *testing.T) {
	var now atomic.Int64
	now.Store(1790000000000)
	options := coreOptions(&now, coreDoer(func(request *http.Request) (*http.Response, error) {
		destination, pinned := request.Context().Value(destinationKey{}).(pinnedDestination)
		if !pinned || destination.address != "8.8.8.8:443" || request.Header.Get("Authorization") != "" || request.Header.Get("Accept-Encoding") != "identity" {
			test.Error("media destination/header isolation lost")
		}
		return coreResponse(200, "media-bytes", "video/mp4"), nil
	}))
	options.Sites = DefaultSites()
	owner := &module{options: options}
	configured := NewModule(options)
	configured.Handlers[sdk.MethodProviderModels] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		client, err := owner.SelectedClient(ctx, call, stream, "")
		if err != nil {
			return nil, err
		}
		response, err := client.Transfer(ctx, "GET", "https://8.8.8.8/video?grant=synthetic", nil, nil, 100)
		if err != nil {
			return nil, err
		}
		if string(response.Body) != "media-bytes" {
			return nil, fmt.Errorf("wrong transfer bytes")
		}
		return sdk.ModelsResult{}, nil
	}
	fixture := newCoreHarness(test, configured)
	scope, credentials := coreAccount(Global)
	result, _ := fixture.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, &credentials)
	if result.Error != nil {
		test.Fatal(result.Error)
	}
	if len(fixture.callbacks) != 1 || !strings.Contains(string(fixture.callbacks[0].Params), "https://8.8.8.8/video?grant=synthetic") {
		test.Fatal("proxy policy saw a rewritten or foreign URL")
	}
}

var _ io.ReadCloser = (*coreCancelBody)(nil)

func TestCorePinnedMediaHTTPSProxyHasIndependentTLSIdentity(test *testing.T) {
	var helloName atomic.Value
	var connectHost atomic.Value
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connectHost.Store(request.Method + " " + request.Host)
		writer.WriteHeader(http.StatusBadGateway)
	}))
	proxy.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		helloName.Store(hello.ServerName)
		return nil, nil
	}}
	proxy.StartTLS()
	defer proxy.Close()
	transport, cleanup, err := defaultHTTP(proxy.URL)
	if err != nil {
		test.Fatal(err)
	}
	defer cleanup()
	client := transport.(*policyClient)
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	client.transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	ctx := context.WithValue(test.Context(), destinationKey{}, pinnedDestination{address: "8.8.8.8:443", serverName: "asset.invalid"})
	request, err := http.NewRequestWithContext(ctx, "GET", "https://asset.invalid/clip", nil)
	if err != nil {
		test.Fatal(err)
	}
	_, err = client.Do(request)
	if helloName.Load() != "" || connectHost.Load() != "CONNECT 8.8.8.8:443" {
		test.Fatalf("media TLS identity leaked into HTTPS proxy: hello=%v connect=%v error=%v", helloName.Load(), connectHost.Load(), err)
	}
	if err == nil || !strings.Contains(err.Error(), "Bad Gateway") {
		test.Fatalf("fixture refusal lost: %v", err)
	}
}
