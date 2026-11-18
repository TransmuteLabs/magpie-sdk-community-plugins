package devin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestWindowFirstAccountBrowserBinding(test *testing.T) {
	for _, constructor := range []func(ModuleOptions) (sdk.Module, error){NewModule, NewAgent} {
		module, err := constructor(ModuleOptions{})
		if err != nil {
			test.Fatal(err)
		}
		if module.Manifest.Window == nil || module.Manifest.Window.Metadata != "package.json" {
			test.Fatal("Devin first-account GUI has no declared package page")
		}
		binding, found := module.Manifest.ResolveAuthBinding("page.auth.devin")
		if !found || binding.Provider != ProviderID || binding.From != "window:page" || !binding.Reauthorize ||
			len(binding.Methods) != 1 || binding.Methods[0] != (sdk.WindowAuthMethod{ID: "browser", Mode: "poll"}) {
			test.Fatal("Devin first-account GUI must use its existing browser poll owner")
		}
	}
	inbound, err := NewInbound()
	if err != nil || inbound.Manifest.Window != nil || len(inbound.Manifest.AuthBindings) != 0 {
		test.Fatal("codec-only package acquired an authorization surface")
	}
}

func startBrowserFixture(test *testing.T, peer *sdkPeer) (sdk.AuthStartResult, *url.URL) {
	test.Helper()
	result, _ := peer.call(test, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "browser"})
	var started sdk.AuthStartResult
	if result.Error != nil || json.Unmarshal(result.Result, &started) != nil {
		test.Fatal("first-account browser start without a caller callback was refused")
	}
	address, err := url.Parse(started.URL)
	if err != nil || address.Query().Get("code_challenge_method") != "S256" || address.Query().Get("state") == "" ||
		bytes.Contains(result.Result, []byte("Verifier")) || bytes.Contains(result.Result, []byte("callbackURL")) {
		test.Fatal("first-account browser start leaked private material or lost PKCE")
	}
	redirect, err := url.Parse(address.Query().Get("redirect_uri"))
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" || redirect.Port() == "" || redirect.Path != "/callback" {
		test.Fatal("module-owned callback did not use the exact donor loopback origin")
	}
	redirect.RawQuery = url.Values{"state": {address.Query().Get("state")}, "code": {"synthetic-gui-code"}}.Encode()
	return started, redirect
}

func callbackRequest(test *testing.T, client *http.Client, method, address, host string, want int) {
	test.Helper()
	request, err := http.NewRequest(method, address, nil)
	if err != nil {
		test.Fatal(err)
	}
	if host != "" {
		request.Host = host
	}
	response, err := client.Do(request)
	if err != nil {
		test.Fatal("callback receiver was unavailable")
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != want {
		test.Fatal("callback admission returned the wrong status", response.StatusCode, want)
	}
	if bytes.Contains(body, []byte("synthetic-gui-code")) || bytes.Contains(body, []byte("state=")) ||
		response.Header.Get("Cache-Control") != "no-store" {
		test.Fatal("callback response exposed private callback data or permitted caching")
	}
}

func TestProviderWindowPKCECallbackAndSingleExchange(test *testing.T) {
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case outbound.ExchangePath:
			exchanges.Add(1)
			var body map[string]string
			if json.NewDecoder(request.Body).Decode(&body) != nil || body["code"] != "synthetic-gui-code" || len(body["code_verifier"]) < 43 {
				test.Error("callback exchange lost its private PKCE binding")
			}
			io.WriteString(writer, `{"sessionToken":"synthetic-window-session"}`)
		case outbound.StatusPath:
			io.WriteString(writer, `{"userStatus":{"email":"window@example.invalid"}}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	peer, store := providerPeer(test, server.URL)
	started, callback := startBrowserFixture(test, peer)
	finish := sdk.AuthFinishRequest{Continuation: started.Continuation}
	result, _ := peer.call(test, sdk.MethodAuthFinish, finish)
	var pending sdk.AuthResult
	if result.Error != nil || json.Unmarshal(result.Result, &pending) != nil || !pending.Pending || exchanges.Load() != 0 {
		test.Fatal("browser poll before callback did not remain pending without dispatch")
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	callbackRequest(test, client, "POST", callback.String(), "", http.StatusBadRequest)
	callbackRequest(test, client, "GET", callback.String(), "foreign.invalid", http.StatusBadRequest)
	wrong := *callback
	wrong.Path = "/foreign"
	callbackRequest(test, client, "GET", wrong.String(), "", http.StatusBadRequest)
	wrong = *callback
	query := wrong.Query()
	query.Set("state", "foreign")
	wrong.RawQuery = query.Encode()
	callbackRequest(test, client, "GET", wrong.String(), "", http.StatusBadRequest)
	wrong = *callback
	wrong.RawQuery += "&code=duplicate"
	callbackRequest(test, client, "GET", wrong.String(), "", http.StatusBadRequest)
	wrong = *callback
	wrong.RawQuery += "&state=duplicate"
	callbackRequest(test, client, "GET", wrong.String(), "", http.StatusBadRequest)
	wrong = *callback
	query = wrong.Query()
	query.Set("code", string(bytes.Repeat([]byte("x"), 4097)))
	wrong.RawQuery = query.Encode()
	callbackRequest(test, client, "GET", wrong.String(), "", http.StatusBadRequest)
	injected := finish
	injected.Input = json.RawMessage(`{"callbackURL":"http://127.0.0.1:1/callback"}`)
	result, _ = peer.call(test, sdk.MethodAuthFinish, injected)
	if result.Error == nil {
		test.Fatal("GUI completion accepted a replacement callback")
	}
	if exchanges.Load() != 0 {
		test.Fatal("invalid callback dispatched an exchange")
	}
	callbackRequest(test, client, "GET", callback.String(), "", http.StatusOK)
	callbackRequest(test, client, "GET", callback.String(), "", http.StatusConflict)
	if exchanges.Load() != 0 {
		test.Fatal("callback handler exchanged outside the existing auth.finish owner")
	}
	result, _ = peer.call(test, sdk.MethodAuthFinish, finish)
	var account sdk.AuthResult
	if result.Error != nil || json.Unmarshal(result.Result, &account) != nil || account.ID == "" || account.Pending || exchanges.Load() != 1 {
		test.Fatal("host poll did not complete the single callback exchange")
	}
	result, _ = peer.call(test, sdk.MethodAuthFinish, finish)
	if result.Error == nil || exchanges.Load() != 1 {
		test.Fatal("callback finish replayed an exchange")
	}
	for _, row := range store.rows {
		if bytes.Contains(row.Value, []byte("synthetic-gui-code")) || bytes.Contains(row.Value, []byte("synthetic-window-session")) {
			test.Fatal("callback code or exchanged session persisted in sign-in state")
		}
	}
	response, err := client.Get(callback.String())
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		test.Fatal("completed callback listener survived auth.finish")
	}
}

func TestProviderWindowPKCEStaleInstanceAndDisposal(test *testing.T) {
	first, store := providerPeer(test, "http://127.0.0.1:1")
	second, _ := providerPeer(test, "http://127.0.0.1:1")
	second.host = store.host
	started, callback := startBrowserFixture(test, first)
	result, _ := second.call(test, sdk.MethodAuthFinish, sdk.AuthFinishRequest{Continuation: started.Continuation})
	if result.Error == nil {
		test.Fatal("another component instance adopted a private callback")
	}
	result, _ = first.call(test, sdk.MethodComponentConfigure, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent, Instance: "replacement",
		Config: sdk.ConfigSnapshot{Version: "1", Values: json.RawMessage(`{}`)},
	})
	if result.Error == nil || result.Error.Code != "conflict" {
		test.Fatal("active component accepted a mutable instance configuration")
	}
	result, _ = first.call(test, sdk.MethodComponentStop, struct{}{})
	if result.Error != nil {
		test.Fatal("component disposal failed")
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(callback.String())
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		test.Fatal("disposed instance retained its callback listener")
	}
	result, _ = first.call(test, sdk.MethodAuthFinish, sdk.AuthFinishRequest{Continuation: started.Continuation})
	if result.Error == nil {
		test.Fatal("replacement replayed an old private callback")
	}
}

func TestProviderWindowPKCEBoundExpiryAndUncertainty(test *testing.T) {
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		exchanges.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	var elapsed atomic.Int64
	origin := time.Now()
	module, err := NewModule(ModuleOptions{Server: server.URL, Now: func() time.Time {
		return origin.Add(time.Duration(elapsed.Load()))
	}})
	if err != nil {
		test.Fatal(err)
	}
	peer := startSDKPeer(test, module)
	store := &stateFixture{rows: make(map[string]sdk.StateValue)}
	peer.host, peer.scope = store.host, sdk.Scope{Provider: ProviderID}
	var first sdk.AuthStartResult
	var callback *url.URL
	for index := 0; index < 16; index++ {
		started, address := startBrowserFixture(test, peer)
		if index == 0 {
			first, callback = started, address
		}
	}
	result, _ := peer.call(test, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "browser"})
	if result.Error == nil || len(store.rows) != 16 {
		test.Fatal("browser callback storage exceeded sixteen pending flows")
	}
	elapsed.Store(int64(11 * time.Minute))
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	callbackRequest(test, client, "GET", callback.String(), "", http.StatusBadRequest)
	result, _ = peer.call(test, sdk.MethodAuthFinish, sdk.AuthFinishRequest{Continuation: first.Continuation})
	if result.Error == nil || exchanges.Load() != 0 {
		test.Fatal("expired browser callback dispatched an exchange")
	}
	started, callback := startBrowserFixture(test, peer)
	callbackRequest(test, client, "GET", callback.String(), "", http.StatusOK)
	finish := sdk.AuthFinishRequest{Continuation: started.Continuation}
	result, _ = peer.call(test, sdk.MethodAuthFinish, finish)
	if result.Error == nil || result.Error.Outcome != "uncertain" || exchanges.Load() != 1 {
		test.Fatal("failed exchange lost its uncertainty fence")
	}
	result, _ = peer.call(test, sdk.MethodAuthFinish, finish)
	if result.Error == nil || exchanges.Load() != 1 {
		test.Fatal("uncertain callback exchange replayed")
	}
}
