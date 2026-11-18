package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func nativeText(number int, value string) []byte {
	body := binary.AppendUvarint(nil, uint64(number<<3|2))
	body = binary.AppendUvarint(body, uint64(len(value)))
	return append(body, value...)
}
func nativeNumber(number int, value uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(number<<3)), value)
}
func nativeFrame(flags byte, payload []byte) []byte {
	out := []byte{flags, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[1:], uint32(len(payload)))
	return append(out, payload...)
}
func nativeFields(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

type stateFixture struct {
	mu     sync.Mutex
	rows   map[string]sdk.StateValue
	writes int
}

func (store *stateFixture) host(frame sdk.Frame) (any, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if frame.Auth != nil {
		return nil, fmt.Errorf("auth leaked to callback")
	}
	if frame.Method == sdk.MethodNetworkProxy {
		return sdk.NetworkProxyResult{}, nil
	}
	if frame.Method == sdk.MethodDiagnosticEmit {
		return sdk.DiagnosticResult{Recorded: true}, nil
	}
	key := fmt.Sprintf("%s/%s/%d/", frame.Scope.Provider, frame.Scope.Account, frame.Scope.Generation)
	switch frame.Method {
	case sdk.MethodStateGet:
		var request sdk.StateGetRequest
		if json.Unmarshal(frame.Params, &request) != nil {
			return nil, fmt.Errorf("bad state read")
		}
		return store.rows[key+request.Key], nil
	case sdk.MethodStateCAS:
		var request sdk.StateCASRequest
		if json.Unmarshal(frame.Params, &request) != nil {
			return nil, fmt.Errorf("bad state write")
		}
		key += request.Key
		old := store.rows[key]
		if old.Version != request.Version {
			return sdk.StateCASResult{Version: old.Version}, nil
		}
		generation, _ := strconv.Atoi(old.Version)
		version := strconv.Itoa(generation + 1)
		store.rows[key] = sdk.StateValue{Found: true, Version: version, Value: bytes.Clone(request.Value)}
		store.writes++
		return sdk.StateCASResult{Swapped: true, Version: version}, nil
	}
	return nil, fmt.Errorf("unknown callback")
}

func providerPeer(t *testing.T, server string) (*sdkPeer, *stateFixture) {
	t.Helper()
	module, err := NewModule(ModuleOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	for method, handler := range module.Handlers {
		module.Handlers[method] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
			defer func() {
				if cause := recover(); cause != nil {
					t.Errorf("%s panicked: %v", method, cause)
					panic(cause)
				}
			}()
			result, err := handler(ctx, call, stream)
			if err != nil {
				t.Logf("%s: %T %+v", method, err, err)
			}
			return result, err
		}
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	peer.host = store.host
	peer.scope = sdk.Scope{Provider: ProviderID}
	return peer, store
}

func selectAccount(peer *sdkPeer, server string) {
	credentials := outbound.Credentials{Key: "selected-fixture-secret"}
	peer.auth, _ = json.Marshal(credentials)
	peer.scope = sdk.Scope{Provider: ProviderID, Account: credentials.AccountID(server), Generation: 3, Operation: "fixture-op", Attempt: 1}
}

func fixtureExecute(body []byte, options sdk.ExecuteOptions) sdk.ExecuteRequest {
	options.Method = "POST"
	options.Path = "/v1/chat/completions"
	data, _ := json.Marshal(options)
	return sdk.ExecuteRequest{Protocol: "chat", Model: "fixture-model", OperationID: "fixture-op", Body: body, Options: data}
}

func TestProviderPKCEIsDurableOneUseAndDoesNotExposeVerifier(t *testing.T) {
	var exchanges, statuses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case outbound.ExchangePath:
			exchanges.Add(1)
			var value map[string]string
			if json.NewDecoder(request.Body).Decode(&value) != nil || len(value) != 2 || value["code"] != "synthetic-code" || len(value["code_verifier"]) < 43 {
				t.Error("invalid exchange")
			}
			io.WriteString(writer, `{"sessionToken":"selected-fixture-secret"}`)
		case outbound.StatusPath:
			statuses.Add(1)
			io.WriteString(writer, `{"userStatus":{"email":"fixture@example.invalid"}}`)
		default:
			t.Error("unexpected auth endpoint")
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	peer, store := providerPeer(t, server.URL)
	result, _ := peer.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "browser", RedirectURI: "http://127.0.0.1:34567/callback"})
	var started sdk.AuthStartResult
	if result.Error != nil || json.Unmarshal(result.Result, &started) != nil || bytes.Contains(result.Result, []byte("Verifier")) {
		t.Fatal("auth start", result)
	}
	address, _ := url.Parse(started.URL)
	if address.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("PKCE challenge missing")
	}
	bad := json.RawMessage(`{"callbackURL":"http://127.0.0.1:34567/callback?state=foreign&code=synthetic-code"}`)
	finish := sdk.AuthFinishRequest{Continuation: started.Continuation, Input: bad}
	result, _ = peer.call(t, sdk.MethodAuthFinish, finish)
	if result.Error == nil || exchanges.Load() != 0 || store.writes != 1 {
		t.Fatal("mismatched state consumed auth flow")
	}
	callback := "http://127.0.0.1:34567/callback?state=" + url.QueryEscape(address.Query().Get("state")) + "&code=synthetic-code"
	finish.Input, _ = json.Marshal(map[string]string{"callbackURL": callback})
	result, _ = peer.call(t, sdk.MethodAuthFinish, finish)
	var account sdk.AuthResult
	if result.Error != nil || json.Unmarshal(result.Result, &account) != nil || account.ID == "" || statuses.Load() != 1 || exchanges.Load() != 1 {
		t.Fatal("auth finish", result)
	}
	result, _ = peer.call(t, sdk.MethodAuthFinish, finish)
	if result.Error == nil || exchanges.Load() != 1 {
		t.Fatal("PKCE continuation replayed", result)
	}
	for _, row := range store.rows {
		var flow signInFlow
		if json.Unmarshal(row.Value, &flow) != nil || flow.State != "complete" || flow.Credentials.Key != "" || flow.PKCE.Verifier != "" {
			t.Fatal("completed flow retained secret")
		}
	}
}

func TestProviderPKCEConcurrentCompletionExchangesOnlyOnce(t *testing.T) {
	var exchanges, statuses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case outbound.ExchangePath:
			exchanges.Add(1)
			io.WriteString(writer, `{"sessionToken":"fixture-key"}`)
		case outbound.StatusPath:
			statuses.Add(1)
			io.WriteString(writer, `{"userStatus":{"email":"fixture@example.invalid"}}`)
		default:
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	first, store := providerPeer(t, server.URL)
	second, _ := providerPeer(t, server.URL)
	second.host = store.host
	result, _ := first.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "browser", RedirectURI: "http://127.0.0.1:34567/callback"})
	var start sdk.AuthStartResult
	if result.Error != nil || json.Unmarshal(result.Result, &start) != nil {
		t.Fatal(result)
	}
	address, _ := url.Parse(start.URL)
	input, _ := json.Marshal(map[string]string{"callbackURL": "http://127.0.0.1:34567/callback?state=" + url.QueryEscape(address.Query().Get("state")) + "&code=fixture"})
	request := sdk.AuthFinishRequest{Continuation: start.Continuation, Input: input}
	results := make(chan sdk.Frame, 2)
	var wait sync.WaitGroup
	for _, peer := range []*sdkPeer{first, second} {
		wait.Add(1)
		go func() { defer wait.Done(); result, _ := peer.call(t, sdk.MethodAuthFinish, request); results <- result }()
	}
	wait.Wait()
	close(results)
	successes, failures := 0, 0
	for result := range results {
		if result.Error == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 || exchanges.Load() != 1 || statuses.Load() != 1 {
		t.Fatal("PKCE was not consumed atomically", successes, failures, exchanges.Load(), statuses.Load())
	}
}

func TestProviderAuthBoundsPendingFlowsAndRejectsExpiredHandles(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	module, err := NewModule(ModuleOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	peer.host = store.host
	peer.scope = sdk.Scope{Provider: ProviderID}
	var original sdk.AuthStartResult
	for index := 0; index < 16; index++ {
		result, _ := peer.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "api-key", Input: json.RawMessage(`{"key":"fixture"}`)})
		if result.Error != nil {
			t.Fatal("free sign-in slot refused", index, result.Error)
		}
		if index == 0 {
			json.Unmarshal(result.Result, &original)
		}
	}
	result, _ := peer.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "api-key", Input: json.RawMessage(`{"key":"fixture"}`)})
	if result.Error == nil || len(store.rows) != 16 {
		t.Fatal("sign-in storage was not bounded")
	}
	now = now.Add(11 * time.Minute)
	result, _ = peer.call(t, sdk.MethodAuthFinish, sdk.AuthFinishRequest{Continuation: original.Continuation})
	if result.Error == nil {
		t.Fatal("expired handle accepted")
	}
	result, _ = peer.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "api-key", Input: json.RawMessage(`{"key":"fixture"}`)})
	if result.Error != nil || len(store.rows) != 16 {
		t.Fatal("expired slot was not reused")
	}
}

func TestFullModuleDeclaresBothDirectionsAndRejectsAmbiguousParameters(t *testing.T) {
	module, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if module.Manifest.Contract.Version != (sdk.APIVersion{Major: 1, Minor: 12}) || module.Manifest.Compatibility.HostAPI != "^1.12" || !module.Manifest.Contract.Has(sdk.RequestPolicy) ||
		!module.Manifest.Contract.Has(sdk.OutboundExecutor) || !module.Manifest.Contract.Has(sdk.AccountAuth) ||
		len(module.Manifest.Providers) != 1 || module.Manifest.Providers[0].ID != ProviderID ||
		module.Manifest.Version != "0.5.0" {
		t.Fatal("paired module contract drifted", module.Manifest)
	}
	for _, input := range []string{
		`{"method":"api-key","method":"browser"}`,
		`{"method":"api-key","input":{"key":"first","key":"second"}}`,
		`{"method":"api-key","unknown":true}`,
		`null`, `{} {}`,
	} {
		var request sdk.AuthStartRequest
		if err := decodeParams(json.RawMessage(input), &request); err == nil {
			t.Fatal("ambiguous parameters accepted", input)
		}
	}
	failure := moduleFailure(&sdk.Error{Code: "permission_denied", Message: "safe", Outcome: "not-started"}, "uncertain")
	public, ok := failure.(*sdk.Error)
	if !ok || public.Outcome != "uncertain" {
		t.Fatal("post-acceptance failure became replayable", failure)
	}
}

func TestNativeBusinessIDsAreNotAssumedToBeTransportIdempotencyKeys(t *testing.T) {
	peer, _ := providerPeer(t, "http://127.0.0.1:1")
	var requests []sdk.DecodedRequest
	for index, text := range []string{"first turn", "second turn"} {
		responseID := strconv.Itoa(index)
		body := nativeFields(nativeText(21, "model"), nativeText(22, "shared-execution"),
			nativeText(17, "shared-prompt"), nativeText(3, string(nativeFields(nativeNumber(2, 1), nativeText(3, text)))))
		wire := nativeFrame(0, body)
		result, _ := peer.call(t, sdk.MethodInboundDecode, sdk.NativeRequest{ID: "devin", ResponseID: responseID, Headers: chatHeaders(), Body: wire})
		var decoded sdk.DecodedRequest
		if result.Error != nil || json.Unmarshal(result.Result, &decoded) != nil || decoded.RequestID == "" || decoded.RequestID == "shared-execution" {
			t.Fatal("native ID was used as a transport write identity", result)
		}
		requests = append(requests, decoded)
		peer.call(t, sdk.MethodInboundResponse, sdk.ResponseEvent{ID: "devin", ResponseID: responseID, Action: "finish"})
	}
	if requests[0].RequestID == requests[1].RequestID {
		t.Fatal("separate native calls collided on reused business IDs")
	}
}

func TestProviderModelsQuotaAndCanonicalExecution(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case outbound.ModelsPath:
			writer.Header().Set("Content-Type", "application/proto")
			for _, model := range []struct {
				id   string
				free uint64
			}{{"fixture-model", 0}, {"free-model", 4}} {
				writer.Write(nativeText(1, string(nativeFields(nativeText(1, model.id), nativeText(22, model.id), nativeNumber(18, 32768), nativeNumber(24, model.free)))))
			}
		case outbound.StatusPath:
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, `{"userStatus":{"planStatus":{"dailyQuotaRemainingPercent":75}}}`)
		case outbound.ChatPath:
			calls.Add(1)
			if request.Header.Get("Authorization") != "Basic selected-fixture-secret" {
				t.Error("selected auth missing")
			}
			writer.Header().Set("Content-Type", "application/connect+proto")
			writer.Write(nativeFrame(0, nativeFields(nativeText(3, "canonical answer"), nativeNumber(5, 2), nativeText(7, string(nativeFields(nativeNumber(2, 9), nativeNumber(3, 0)))))))
			writer.Write(nativeFrame(2, []byte(`{}`)))
		default:
			t.Error("unexpected path")
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	result, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	var models sdk.ModelsResult
	if result.Error != nil || json.Unmarshal(result.Result, &models) != nil || len(models.Models) != 2 {
		t.Fatal("models", result)
	}
	result, _ = peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "7"})
	var usage sdk.UsageResult
	if result.Error != nil || json.Unmarshal(result.Result, &usage) != nil || usage.Revision != "7" || len(usage.Windows) != 1 ||
		usage.Windows[0].Used != 25 || len(usage.Windows[0].NotModels) != 1 || usage.Windows[0].NotModels[0] != "free-model" {
		t.Fatal("usage", result)
	}
	module, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, fresh, err := sdk.ValidateUsageResult(module.Manifest, result.Result); err != nil || !fresh {
		t.Fatal("live quota did not satisfy the epoch freshness contract", fresh, err)
	}
	for _, streaming := range []bool{false, true} {
		body, _ := json.Marshal(map[string]any{"model": "fixture-model", "stream": streaming, "messages": []any{map[string]string{"role": "user", "content": "hello"}}})
		result, events := peer.call(t, sdk.MethodProviderExecute, fixtureExecute(body, sdk.ExecuteOptions{}))
		var content []byte
		accepted := 0
		for _, event := range events {
			if event.Event == "accepted" {
				accepted++
			}
			if event.Event == "data" {
				var value []byte
				json.Unmarshal(event.Data, &value)
				content = append(content, value...)
			}
		}
		if result.Error != nil || accepted != 1 || !bytes.Contains(content, []byte("canonical answer")) {
			t.Fatal("execute", result, string(content))
		}
		if streaming && !bytes.Contains(content, []byte("[DONE]")) {
			t.Fatal("no canonical terminal")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected dispatch count")
	}
	peer.scope.Generation = 0
	result, _ = peer.call(t, sdk.MethodProviderExecute, fixtureExecute([]byte(`{"model":"fixture-model"}`), sdk.ExecuteOptions{}))
	if result.Error == nil || calls.Load() != 2 {
		t.Fatal("invalid scope dispatched")
	}
}

func TestProviderInvalidHistoryReturns400BeforeAcceptance(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	for _, body := range []string{
		`{"model":"fixture-model","messages":[{"role":"assistant","content":"answer","reasoning":"one","reasoning_content":"two"}]}`,
		`{"model":"fixture-model","messages":[{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]}]}`,
		`{"model":"fixture-model","messages":[{"role":"user","content":"hello"}],"unsupported_control":true}`,
	} {
		result, events := peer.call(t, sdk.MethodProviderExecute, fixtureExecute([]byte(body), sdk.ExecuteOptions{}))
		if result.Error == nil || result.Error.Code != "invalid_params" || result.Error.Outcome != "not-started" {
			t.Fatal("invalid history lost its unstarted outcome", result)
		}
		status := 0
		for _, event := range events {
			switch event.Event {
			case "accepted", "usage":
				t.Fatal("invalid history crossed acceptance or reported usage", event)
			case "headers":
				var headers sdk.HeadersEvent
				if json.Unmarshal(event.Data, &headers) != nil {
					t.Fatal("invalid headers event")
				}
				status = headers.Status
			}
		}
		if status != http.StatusBadRequest || requests.Load() != 0 {
			t.Fatalf("invalid history became a retryable upstream failure: status=%d dispatches=%d", status, requests.Load())
		}
	}
}

func TestProviderUsageKeepsFreshAndCachedTokensDisjoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		usage := nativeFields(nativeNumber(2, 7), nativeNumber(3, 3), nativeNumber(4, 5), nativeNumber(5, 11))
		writer.Header().Set("Content-Type", "application/connect+proto")
		writer.Write(nativeFrame(0, nativeFields(nativeText(3, "counted answer"), nativeNumber(5, 2), nativeText(7, string(usage)))))
		writer.Write(nativeFrame(2, []byte(`{}`)))
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	body := []byte(`{"model":"fixture-model","stream":false,"messages":[{"role":"user","content":"count this"}]}`)
	result, events := peer.call(t, sdk.MethodProviderExecute, fixtureExecute(body, sdk.ExecuteOptions{}))
	var execution sdk.ExecuteResult
	if result.Error != nil || json.Unmarshal(result.Result, &execution) != nil || execution.Usage == nil {
		t.Fatal("usage-bearing execution failed", result)
	}
	if execution.Usage.Facts == nil || execution.Usage.Facts.ResponseID == nil {
		t.Fatal("canonical execution lost its response identity")
	}
	expected := fmt.Sprintf(`{"facts":{"inputTokens":7,"outputTokens":3,"cachedTokens":11,"cacheWriteTokens":5,"responseId":%q,"stop":"STOP_PATTERN"}}`,
		*execution.Usage.Facts.ResponseID)
	actual, _ := json.Marshal(execution.Usage)
	if string(actual) != expected {
		t.Fatalf("SDK usage double-counts native cached prompt tokens: got %s, want %s", actual, expected)
	}
	var response []byte
	foundUsage := false
	for _, event := range events {
		if event.Event == "usage" {
			var usage sdk.Usage
			if json.Unmarshal(event.Data, &usage) != nil {
				t.Fatal("usage event disagrees with completed execution", string(event.Data))
			}
			encoded, _ := json.Marshal(usage)
			if string(encoded) != expected {
				t.Fatal("usage event disagrees with completed execution", string(event.Data))
			}
			foundUsage = true
		}
		if event.Event == "data" {
			var data []byte
			json.Unmarshal(event.Data, &data)
			response = append(response, data...)
		}
	}
	var chat struct {
		ID    string `json:"id"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if !foundUsage || json.Unmarshal(response, &chat) != nil || chat.ID != *execution.Usage.Facts.ResponseID ||
		chat.Usage.Prompt != 23 || chat.Usage.Completion != 3 || chat.Usage.Total != 26 {
		t.Fatal("canonical Chat usage lost its inclusive prompt count", string(response))
	}
}

func TestBridgeClosedPushReleasesEveryReservation(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		store := newBridgeStore()
		bridge, err := store.open(codec.NativeEnvelope{Payload: []byte("request")})
		if err != nil {
			t.Fatal(err)
		}
		var wait sync.WaitGroup
		wait.Add(2)
		go func() { defer wait.Done(); _ = bridge.push(t.Context(), codec.NativeFrame{Payload: []byte("data")}) }()
		go func() { defer wait.Done(); store.remove(bridge) }()
		wait.Wait()
		store.mu.Lock()
		retained := store.responseBytes + store.bytes
		store.mu.Unlock()
		if retained != 0 {
			t.Fatal("bridge leaked retained bytes", retained)
		}
	}
}

func TestOriginsAreAccountGenerationCallerAndModelBound(t *testing.T) {
	module, err := NewModule(ModuleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	collector := &originCollector{}
	delta := nativeFields(nativeText(10, "signed"), nativeText(16, "thinking"), nativeText(21, "format"), nativeText(23, "actual-model"))
	if err := collector.add(delta); err != nil {
		t.Fatal(err)
	}
	module.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		var request struct {
			Save   bool
			Caller string
			Model  string
			Claim  codec.NativeClaim
		}
		if err := json.Unmarshal(call.Params, &request); err != nil {
			return nil, err
		}
		options := sdk.ExecuteOptions{CallerID: request.Caller}
		if request.Save {
			return nil, collector.persist(ctx, stream, call, options, request.Model)
		}
		return nil, verifyOrigins(ctx, stream, call, options, request.Model, []codec.NativeClaim{request.Claim})
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	peer.host = store.host
	peer.scope = sdk.Scope{Provider: ProviderID, Account: "account", Generation: 3, Operation: "fixture-op", Attempt: 1}
	result, _ := peer.call(t, sdk.MethodProviderExecute, map[string]any{"Save": true, "Caller": "caller", "Model": "fixture-model"})
	if result.Error != nil {
		t.Fatal(result)
	}
	claim := codec.NativeClaim{Kind: "signature", Digest: codec.SignatureDigest([]byte("signed")), Format: "format", Model: "actual-model"}
	request := map[string]any{"Caller": "caller", "Model": "fixture-model", "Claim": claim}
	result, _ = peer.call(t, sdk.MethodProviderExecute, request)
	if result.Error != nil {
		t.Fatal("known origin refused", result)
	}
	for _, change := range []string{"caller", "model", "generation", "account", "signature", "format"} {
		peer.scope.Generation = 3
		peer.scope.Account = "account"
		request["Caller"] = "caller"
		request["Model"] = "fixture-model"
		request["Claim"] = claim
		switch change {
		case "caller":
			request["Caller"] = "foreign"
		case "model":
			request["Model"] = "foreign"
		case "generation":
			peer.scope.Generation = 4
		case "account":
			peer.scope.Account = "foreign"
		case "signature":
			value := claim
			value.Digest = codec.SignatureDigest([]byte("foreign"))
			request["Claim"] = value
		case "format":
			value := claim
			value.Format = "foreign"
			request["Claim"] = value
		}
		result, _ = peer.call(t, sdk.MethodProviderExecute, request)
		if result.Error == nil {
			t.Fatal("foreign origin accepted:", change)
		}
	}
}
