package devin

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestBrowserCallbacksExpiryAndConcurrentDisposal(test *testing.T) {
	callbacks := newBrowserCallbacks()
	origin := time.Now()
	var reads atomic.Int32
	flow, _, err := callbacks.prepare(func() time.Time {
		if reads.Add(1) == 1 {
			return origin
		}
		return origin.Add(10*time.Minute - time.Millisecond)
	})
	if err != nil {
		test.Fatal("could not start bounded callback timer")
	}
	select {
	case <-flow.done:
	case <-time.After(5 * time.Second):
		test.Fatal("callback deadline did not close and join the receiver")
	}
	callbacks.mu.Lock()
	retained := len(callbacks.flows)
	callbacks.mu.Unlock()
	if retained != 0 || flow.callback != "" || flow.pkce.Verifier != "" {
		test.Fatal("deadline retained private callback state")
	}
	var workers sync.WaitGroup
	for index := 0; index < 8; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			callbacks.prepare(time.Now)
		}()
	}
	callbacks.close()
	workers.Wait()
	callbacks.close()
	if _, _, err := callbacks.prepare(time.Now); err == nil {
		test.Fatal("disposed callback registry admitted a new listener")
	}
}

func TestBrowserCallbacksDisposalJoinsPartialConnection(test *testing.T) {
	callbacks := newBrowserCallbacks()
	flow, authorize, err := callbacks.prepare(time.Now)
	if err != nil {
		test.Fatal(err)
	}
	defer callbacks.close()
	address, _ := url.Parse(authorize)
	redirect, _ := url.Parse(address.Query().Get("redirect_uri"))
	connection, err := net.DialTimeout("tcp", redirect.Host, 5*time.Second)
	if err != nil {
		test.Fatal("callback did not accept loopback TCP")
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(connection, "GET /callback HTTP/1.1\r\nHost: ")
	callbacks.close()
	buffer := make([]byte, 1)
	if _, err := connection.Read(buffer); err == nil {
		test.Fatal("disposed receiver retained a partial HTTP connection")
	}
	select {
	case <-flow.done:
	default:
		test.Fatal("receiver disposal returned before joining")
	}
}

func TestProviderWindowPKCEConcurrentFinish(test *testing.T) {
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == outbound.ExchangePath {
			exchanges.Add(1)
			io.WriteString(writer, `{"sessionToken":"synthetic-window-session"}`)
		} else if request.URL.Path == outbound.StatusPath {
			io.WriteString(writer, `{"userStatus":{"email":"window@example.invalid"}}`)
		} else {
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	peer, _ := providerPeer(test, server.URL)
	started, callback := startBrowserFixture(test, peer)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	callbackRequest(test, &http.Client{Transport: transport, Timeout: 5 * time.Second}, "GET",
		callback.String(), "", http.StatusOK)
	params, _ := json.Marshal(sdk.AuthFinishRequest{Continuation: started.Continuation})
	for _, identity := range []string{"concurrent-a", "concurrent-b"} {
		peer.send(test, sdk.Frame{Kind: sdk.KindCall, ID: identity, Method: sdk.MethodAuthFinish,
			Scope: peer.callScope(), Params: params})
	}
	successes, failures := 0, 0
	for successes+failures < 2 {
		select {
		case frame := <-peer.frames:
			if frame.Kind == sdk.KindHostCall {
				value, err := peer.host(frame)
				if err != nil {
					test.Fatal("synthetic state owner refused concurrent completion")
				}
				raw, _ := json.Marshal(value)
				peer.send(test, sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent, Result: raw})
				continue
			}
			if frame.Kind != sdk.KindResult || frame.ID != "concurrent-a" && frame.ID != "concurrent-b" {
				test.Fatal("concurrent callback returned an unexpected frame")
			}
			if frame.Error == nil {
				var account sdk.AuthResult
				if json.Unmarshal(frame.Result, &account) != nil || account.ID == "" || account.Pending {
					test.Fatal("concurrent completion returned a false success")
				}
				successes++
			} else {
				failures++
			}
		case <-time.After(10 * time.Second):
			test.Fatal("concurrent callback completion did not terminate")
		}
	}
	if successes != 1 || failures != 1 || exchanges.Load() != 1 {
		test.Fatal("module callback did not preserve the durable single-exchange owner")
	}
}
