package devin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const donorGeminiModel = "gemini-4-hydrogen-medium"
const donorGeminiCall = "call_11646388125050983982"

type oracleField struct {
	number int
	value  []byte
	count  uint64
}

func oracleFields(t *testing.T, body []byte) []oracleField {
	t.Helper()
	var fields []oracleField
	for len(body) > 0 {
		tag, size := binary.Uvarint(body)
		if size <= 0 {
			t.Fatal("invalid oracle tag")
		}
		body = body[size:]
		field := oracleField{number: int(tag >> 3)}
		switch tag & 7 {
		case 0:
			field.count, size = binary.Uvarint(body)
		case 1:
			size = 8
		case 5:
			size = 4
		case 2:
			length, used := binary.Uvarint(body)
			if used <= 0 || length > uint64(len(body)-used) {
				t.Fatal("invalid oracle bytes")
			}
			body = body[used:]
			size = int(length)
			field.value = bytes.Clone(body[:size])
		default:
			t.Fatal("unsupported oracle wire")
		}
		if size < 0 || size > len(body) {
			t.Fatal("invalid oracle size")
		}
		body = body[size:]
		fields = append(fields, field)
	}
	return fields
}

func oracleValue(t *testing.T, body []byte, number int) string {
	t.Helper()
	for _, field := range oracleFields(t, body) {
		if field.number == number {
			return string(field.value)
		}
	}
	return ""
}

func donorGeminiReply(t *testing.T) ([]byte, string) {
	t.Helper()
	body, err := os.ReadFile("testdata/gemini-tool-reply.bin")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != "e4ff6e3b5134fe64f3d9761a98d3ddd1af0945ea6f7946fdfe01a9389ebd4162" {
		t.Fatal("the genuine donor fixture changed")
	}
	reader := bytes.NewReader(body)
	signature := ""
	for reader.Len() > 0 {
		flags, payload, err := codec.ReadConnectFrame(reader, true)
		if err != nil {
			t.Fatal(err)
		}
		if flags&2 != 0 {
			continue
		}
		if oracleValue(t, payload, 9) != "" {
			t.Fatal("donor oracle unexpectedly contains visible thinking")
		}
		signature += oracleValue(t, payload, 10)
		if format := oracleValue(t, payload, 21); format != "" && format != "gemini" {
			t.Fatal("the actual donor signature type changed", format)
		}
	}
	if len(signature) != 368 || !strings.HasPrefix(signature, "AY89a1+u1EAPOiO7CmhB") {
		t.Fatal("the actual donor signature changed")
	}
	return body, signature
}

func geminiContinuation(protocol string) map[string]any {
	call := map[string]any{"id": donorGeminiCall, "type": "function",
		"function": map[string]string{"name": "exec", "arguments": `{"command":"cat calc.py"}`}}
	messages := []any{
		map[string]any{"role": "user", "content": "cat calc.py"},
		map[string]any{"role": "assistant", "content": "", "tool_calls": []any{call}},
		map[string]any{"role": "tool", "tool_call_id": donorGeminiCall, "content": "def add(a,b): return a+b"},
	}
	if protocol == "anthropic" {
		messages[1] = map[string]any{"role": "assistant", "content": []any{map[string]any{
			"type": "tool_use", "id": donorGeminiCall, "name": "exec", "input": map[string]string{"command": "cat calc.py"}}}}
		messages[2] = map[string]any{"role": "user", "content": []any{map[string]any{
			"type": "tool_result", "tool_use_id": donorGeminiCall, "content": "def add(a,b): return a+b"}}}
	}
	return map[string]any{"model": donorGeminiModel, "max_tokens": 64, "messages": messages}
}

func geminiExecute(t *testing.T, peer *sdkPeer, protocol, caller string, body map[string]any) (sdk.Frame, []sdk.Frame) {
	t.Helper()
	path := "/v1/chat/completions"
	if protocol == "anthropic" {
		path = "/v1/messages"
	}
	options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: path, CallerID: caller})
	encoded, _ := json.Marshal(body)
	return peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: protocol, Model: body["model"].(string),
		OperationID: "fixture-op", Options: options, Body: encoded})
}

func TestGeminiDonorOracleToolRoundtrip(t *testing.T) {
	reply, signature := donorGeminiReply(t)
	for _, protocol := range []string{"chat", "anthropic"} {
		for _, streaming := range []bool{false, true} {
			t.Run(protocol+"/"+map[bool]string{false: "buffered", true: "stream"}[streaming], func(t *testing.T) {
				var mu sync.Mutex
				var arrivals [][]byte
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					raw, _ := io.ReadAll(request.Body)
					_, payload, err := codec.ReadConnectFrame(bytes.NewReader(raw), request.Header.Get("Connect-Content-Encoding") == "gzip")
					if err != nil {
						t.Error(err)
					}
					mu.Lock()
					arrivals = append(arrivals, payload)
					mu.Unlock()
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Header().Set("Connect-Content-Encoding", "gzip")
					writer.Write(reply)
				}))
				defer server.Close()
				peer, _ := providerPeer(t, server.URL)
				selectAccount(peer, server.URL)
				first := map[string]any{"model": donorGeminiModel, "max_tokens": 64, "stream": streaming,
					"messages": []any{map[string]string{"role": "user", "content": "cat calc.py"}}}
				result, events := geminiExecute(t, peer, protocol, "caller-one", first)
				if result.Error != nil {
					t.Fatal("the genuine Gemini reply was rejected", result.Error)
				}
				var response []byte
				for _, event := range events {
					if event.Event == "data" {
						var part []byte
						json.Unmarshal(event.Data, &part)
						response = append(response, part...)
					}
				}
				if !bytes.Contains(response, []byte(donorGeminiCall)) {
					t.Fatal("actual tool call was lost")
				}
				continued := geminiContinuation(protocol)
				result, _ = geminiExecute(t, peer, protocol, "caller-one", continued)
				if result.Error != nil {
					t.Fatal("ordinary tool continuation was rejected", result.Error)
				}
				mu.Lock()
				defer mu.Unlock()
				if len(arrivals) != 2 {
					t.Fatal("tool continuation did not dispatch exactly once", len(arrivals))
				}
				signed, realResult := 0, false
				for _, field := range oracleFields(t, arrivals[1]) {
					if field.number != 3 {
						continue
					}
					prompt := field.value
					if oracleValue(t, prompt, 7) == donorGeminiCall &&
						oracleValue(t, prompt, 3) == "def add(a,b): return a+b" {
						realResult = true
					}
					if oracleValue(t, prompt, 12) == "" {
						continue
					}
					signed++
					if oracleValue(t, prompt, 12) != signature || oracleValue(t, prompt, 18) != "gemini" ||
						oracleValue(t, prompt, 22) != donorGeminiModel || oracleValue(t, prompt, 11) != "" ||
						oracleValue(t, []byte(oracleValue(t, prompt, 6)), 1) != donorGeminiCall {
						t.Fatal("actual signature/type/model moved away from its tool or invented thinking")
					}
				}
				if signed != 1 || !realResult {
					t.Fatal("donor signature or real tool result was lost", signed, realResult)
				}
				want := []byte(`"finish_reason":"tool_calls"`)
				if protocol == "anthropic" {
					want = []byte(`"stop_reason":"tool_use"`)
				}
				if !bytes.Contains(response, want) {
					t.Fatal("actual native stop2 with a tool lost its canonical tool finish")
				}
			})
		}
	}
}

func geminiFixtureServer(t *testing.T, reply []byte) (*httptest.Server, <-chan []byte) {
	t.Helper()
	arrivals := make(chan []byte, 16)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != outbound.ChatPath {
			t.Error("Gemini fixture contacted an unexpected endpoint", request.URL.Path)
		}
		raw, _ := io.ReadAll(request.Body)
		_, payload, err := codec.ReadConnectFrame(bytes.NewReader(raw), request.Header.Get("Connect-Content-Encoding") == "gzip")
		if err != nil {
			t.Error(err)
		}
		arrivals <- payload
		writer.Header().Set("Content-Type", "application/connect+proto")
		writer.Header().Set("Connect-Content-Encoding", "gzip")
		writer.Write(reply)
	}))
	t.Cleanup(server.Close)
	return server, arrivals
}

func TestGeminiDonorOracleRejectsExplicitForeignOriginsBeforeAcceptance(t *testing.T) {
	reply, signature := donorGeminiReply(t)
	server, arrivals := geminiFixtureServer(t, reply)
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	first := map[string]any{"model": donorGeminiModel, "messages": []any{map[string]string{
		"role": "user", "content": "cat calc.py"}}}
	result, events := geminiExecute(t, peer, "chat", "caller-one", first)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	<-arrivals
	var content []byte
	for _, event := range events {
		if event.Event == "data" {
			var part []byte
			json.Unmarshal(event.Data, &part)
			content = append(content, part...)
		}
	}
	var response struct {
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(content, &response) != nil || len(response.Choices) != 1 {
		t.Fatal("invalid donor Chat response")
	}
	continued := geminiContinuation("chat")
	continued["messages"].([]any)[1] = response.Choices[0].Message
	result, _ = geminiExecute(t, peer, "chat", "caller-one", continued)
	if result.Error != nil || !bytes.Contains(<-arrivals, []byte(signature)) {
		t.Fatal("the actual returned Chat message could not continue with its scoped signature", result.Error)
	}
	for _, change := range []string{"caller", "generation", "model", "signature", "format"} {
		t.Run(change, func(t *testing.T) {
			peer.scope.Generation = 3
			next := geminiContinuation("chat")
			caller := "caller-one"
			detailSignature, format := signature, "gemini"
			switch change {
			case "caller":
				caller = "caller-two"
			case "generation":
				peer.scope.Generation++
			case "model":
				next["model"] = "gemini-another-model"
			case "signature":
				detailSignature = "unissued-signature"
			case "format":
				format = "anthropic"
			}
			assistant := next["messages"].([]any)[1].(map[string]any)
			assistant["reasoning_details"] = []any{map[string]any{"type": "reasoning.text", "text": "",
				"signature": detailSignature, "format": format, "index": 0}}
			result, events := geminiExecute(t, peer, "chat", caller, next)
			if result.Error == nil || result.Error.Outcome != "not-started" {
				t.Fatal("explicit foreign signature crossed its scope", result.Error)
			}
			for _, event := range events {
				if event.Event == "accepted" {
					t.Fatal("explicit foreign signature crossed acceptance")
				}
			}
			select {
			case <-arrivals:
				t.Fatal("explicit foreign signature dispatched")
			default:
			}
		})
	}
}

func TestGeminiDonorOracleInvisibleStateDoesNotCrossIdentityOrEditedHistory(t *testing.T) {
	reply, signature := donorGeminiReply(t)
	for _, change := range []string{"unknown", "caller", "generation", "account", "model", "arguments", "text", "restart"} {
		t.Run(change, func(t *testing.T) {
			server, arrivals := geminiFixtureServer(t, reply)
			peer, store := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			first := map[string]any{"model": donorGeminiModel, "messages": []any{map[string]string{
				"role": "user", "content": "cat calc.py"}}}
			if change != "unknown" {
				result, _ := geminiExecute(t, peer, "chat", "caller-one", first)
				if result.Error != nil {
					t.Fatal(result.Error)
				}
				<-arrivals
			}
			next := geminiContinuation("chat")
			caller := "caller-one"
			switch change {
			case "caller":
				caller = "caller-two"
			case "generation":
				peer.scope.Generation++
			case "account":
				credentials := outbound.Credentials{Key: "another-fixture-secret"}
				peer.auth, _ = json.Marshal(credentials)
				peer.scope.Account = credentials.AccountID(server.URL)
			case "model":
				next["model"] = "gemini-another-model"
			case "arguments":
				assistant := next["messages"].([]any)[1].(map[string]any)
				assistant["tool_calls"].([]any)[0].(map[string]any)["function"] =
					map[string]string{"name": "exec", "arguments": `{"command":"another command"}`}
			case "text":
				next["messages"].([]any)[1].(map[string]any)["content"] = "changed reply"
			case "restart":
				peer, _ = providerPeer(t, server.URL)
				peer.host = store.host
				selectAccount(peer, server.URL)
			}
			result, _ := geminiExecute(t, peer, "chat", caller, next)
			if result.Error != nil {
				t.Fatal("unknown unsigned history changed its ordinary behavior", result.Error)
			}
			if bytes.Contains(<-arrivals, []byte(signature)) {
				t.Fatal("invisible signature escaped its exact reply/identity or survived a restart", change)
			}
		})
	}
}

func TestGeminiThoughtStoreBoundsScopeAndClose(t *testing.T) {
	store := &geminiThoughtStore{}
	for index := range 300 {
		key := geminiThoughtKey{account: "account", generation: 1, caller: "caller", model: donorGeminiModel,
			reply: string(rune(index))}
		store.put(key, codec.GeminiThought{Signature: "signature", Format: "gemini", Model: donorGeminiModel})
	}
	if len(store.entries) != 256 || len(store.order) != 256 || store.bytes != 256*len("signature") {
		t.Fatal("Gemini state exceeded its entry bound")
	}
	key := geminiThoughtKey{account: "account", generation: 1, caller: "caller", model: donorGeminiModel,
		reply: string(rune(299))}
	if _, found := store.get(key); !found {
		t.Fatal("latest thought disappeared")
	}
	other := key
	other.conversation = "other-session"
	if _, found := store.get(other); found {
		t.Fatal("thought crossed a host session")
	}
	for index := range 9 {
		key.reply = string(rune(index + 500))
		store.put(key, codec.GeminiThought{Signature: strings.Repeat("s", 2<<20), Format: "gemini", Model: donorGeminiModel})
	}
	if store.bytes > 8<<20 || len(store.entries) > 4 {
		t.Fatal("Gemini signatures exceeded their aggregate byte bound")
	}
	store.close()
	store.put(key, codec.GeminiThought{Signature: "late"})
	if len(store.entries) != 0 || store.bytes != 0 {
		t.Fatal("a finished module retained or resurrected a thought")
	}
}

func TestGeminiDonorCanceledReplyNeverIssuesOrigins(t *testing.T) {
	reply, _ := donorGeminiReply(t)
	held := make(chan struct{})
	joined := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/connect+proto")
		writer.Header().Set("Connect-Content-Encoding", "gzip")
		writer.Write(reply[:len(reply)-7])
		writer.(http.Flusher).Flush()
		close(held)
		<-request.Context().Done()
		close(joined)
	}))
	t.Cleanup(server.Close)
	module, err := NewModule(ModuleOptions{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var state *providerState
	delegate := module.Handlers[sdk.MethodProviderExecute]
	module.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		state, _ = currentProviderState(ctx)
		return delegate(ctx, call, stream)
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	canceled := false
	peer.host = func(frame sdk.Frame) (any, error) {
		if frame.Method == sdk.MethodDiagnosticEmit && !canceled {
			var diagnostic sdk.Diagnostic
			if json.Unmarshal(frame.Params, &diagnostic) == nil && diagnostic.Code == "devin.execute.accepted" {
				go func() {
					select {
					case <-held:
						peer.send(t, sdk.Frame{Kind: sdk.KindCancel, ID: frame.Parent})
					case <-time.After(5 * time.Second):
					}
				}()
				canceled = true
			}
		}
		return store.host(frame)
	}
	selectAccount(peer, server.URL)
	result, _ := geminiExecute(t, peer, "chat", "caller-one", map[string]any{"model": donorGeminiModel,
		"messages": []any{map[string]string{"role": "user", "content": "cat calc.py"}}})
	if result.Error == nil || store.writes != 0 || state == nil || len(state.geminiThoughts.entries) != 0 {
		t.Fatal("canceled Gemini reply issued a continuation origin", result.Error, store.writes)
	}
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled Gemini HTTP request was not joined")
	}
}

func TestGeminiMalformedReplyCannotPersistAnUntypedOrigin(t *testing.T) {
	for _, protocol := range []string{"chat", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			reply := nativeFields(nativeFrame(0, nativeFields(nativeText(3, "answer"),
				nativeText(10, "untyped-signature"), nativeNumber(5, 2))), nativeFrame(2, []byte(`{}`)))
			server, _ := geminiFixtureServer(t, reply)
			peer, store := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			result, _ := geminiExecute(t, peer, protocol, "caller-one", map[string]any{
				"model": donorGeminiModel, "max_tokens": 64, "messages": []any{
					map[string]string{"role": "user", "content": "hello"}}})
			if result.Error == nil || store.writes != 0 {
				t.Fatal("a malformed Gemini reply issued a reusable untyped origin", result.Error, store.writes)
			}
		})
	}
}

func TestGeminiReplyNeverRetainsOrRendersEchoedAccountCredentials(t *testing.T) {
	for _, protocol := range []string{"chat", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			reply := nativeFields(nativeFrame(0, nativeFields(nativeText(3, "answer"),
				nativeText(10, "selected-fixture-secret"), nativeText(21, "gemini"), nativeNumber(5, 2))),
				nativeFrame(2, []byte(`{}`)))
			server, _ := geminiFixtureServer(t, reply)
			peer, store := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			result, events := geminiExecute(t, peer, protocol, "caller-one", map[string]any{
				"model": donorGeminiModel, "max_tokens": 64, "messages": []any{
					map[string]string{"role": "user", "content": "hello"}}})
			if result.Error == nil || store.writes != 0 {
				t.Fatal("Gemini retained echoed credentials", result.Error, store.writes)
			}
			for _, event := range events {
				if event.Event != "data" {
					continue
				}
				var data []byte
				json.Unmarshal(event.Data, &data)
				if bytes.Contains(data, []byte("selected-fixture-secret")) {
					t.Fatal("Gemini rendered echoed credentials")
				}
			}
		})
	}
}
