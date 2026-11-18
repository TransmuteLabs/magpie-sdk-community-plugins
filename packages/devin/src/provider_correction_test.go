package devin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func correctionData(t *testing.T, events []sdk.Frame) []byte {
	t.Helper()
	var body []byte
	for _, event := range events {
		if event.Event == "data" {
			var part []byte
			if err := json.Unmarshal(event.Data, &part); err != nil {
				t.Fatal(err)
			}
			body = append(body, part...)
		}
	}
	return body
}

func TestClaudeChatReturnedSignatureContinuesWithinScope(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			reply := nativeFields(
				nativeFrame(0, nativeFields(nativeText(9, "native thought"), nativeText(16, "thought-1"))),
				nativeFrame(0, nativeFields(nativeText(10, "synthetic-signed"), nativeText(16, "thought-1"), nativeText(21, "anthropic"), nativeText(23, "claude-fixture"))),
				nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeNumber(5, 2))), nativeFrame(2, []byte(`{}`)))
			server, arrivals := geminiFixtureServer(t, reply)
			peer, _ := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			execute := func(caller, model string, messages []any, stream bool) (sdk.Frame, []sdk.Frame) {
				body, _ := json.Marshal(map[string]any{"model": model, "messages": messages, "stream": stream})
				options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/chat/completions", CallerID: caller})
				return peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: model, OperationID: "fixture-op", Body: body, Options: options})
			}
			user := map[string]any{"role": "user", "content": "hello"}
			result, events := execute("caller-one", "claude-fixture", []any{user}, streaming)
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			<-arrivals
			body := correctionData(t, events)
			assistant := map[string]any{}
			if !streaming {
				var value struct {
					Choices []struct {
						Message map[string]any `json:"message"`
					} `json:"choices"`
				}
				if json.Unmarshal(body, &value) != nil || len(value.Choices) != 1 {
					t.Fatal(string(body))
				}
				assistant = value.Choices[0].Message
			} else {
				text, thinking, signature, format, detailText := "", "", "", "", ""
				for _, line := range bytes.Split(body, []byte{'\n'}) {
					if !bytes.HasPrefix(line, []byte("data: {")) {
						continue
					}
					var chunk struct {
						Choices []struct {
							Delta struct {
								Content  string `json:"content"`
								Thinking string `json:"reasoning_content"`
								Details  []struct {
									Text      string `json:"text"`
									Signature string `json:"signature"`
									Format    string `json:"format"`
								} `json:"reasoning_details"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if err := json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &chunk); err != nil {
						t.Fatal(err)
					}
					for _, choice := range chunk.Choices {
						text += choice.Delta.Content
						thinking += choice.Delta.Thinking
						for _, detail := range choice.Delta.Details {
							detailText += detail.Text
							signature += detail.Signature
							format += detail.Format
						}
					}
				}
				assistant = map[string]any{"role": "assistant", "content": text, "reasoning_content": thinking,
					"reasoning_details": []any{map[string]any{"type": "reasoning.text", "text": detailText, "signature": signature, "format": format, "index": 0}}}
			}
			if assistant["reasoning_content"] != "native thought" || assistant["reasoning_details"] == nil {
				t.Fatal("missing returned signed thought", assistant)
			}
			history := []any{user, assistant, map[string]any{"role": "user", "content": "continue"}}
			result, _ = execute("caller-one", "claude-fixture", history, false)
			if result.Error != nil {
				t.Fatal("own returned Chat message cannot continue", result.Error)
			}
			raw := <-arrivals
			if !bytes.Contains(raw, []byte("synthetic-signed")) || !bytes.Contains(raw, []byte("native thought")) {
				t.Fatal("signed history was dropped")
			}
			original, _ := json.Marshal(assistant)
			for _, change := range []string{"caller", "generation", "account", "model", "signature", "format", "text"} {
				selectAccount(peer, server.URL)
				json.Unmarshal(original, &assistant)
				history[1] = assistant
				caller, model := "caller-one", "claude-fixture"
				switch change {
				case "caller":
					caller = "caller-two"
				case "generation":
					peer.scope.Generation++
				case "account":
					credentials := outbound.Credentials{Key: "other-fixture-key"}
					peer.auth, _ = json.Marshal(credentials)
					peer.scope.Account = credentials.AccountID(server.URL)
				case "model":
					model = "claude-other"
				case "signature", "format", "text":
					assistant["reasoning_details"].([]any)[0].(map[string]any)[change] = "unissued"
				}
				result, events = execute(caller, model, history, false)
				if result.Error == nil || result.Error.Outcome != "not-started" {
					t.Fatal("foreign origin accepted", change, result.Error)
				}
				for _, event := range events {
					if event.Event == "accepted" {
						t.Fatal("foreign origin crossed acceptance", change)
					}
				}
				select {
				case <-arrivals:
					t.Fatal("foreign origin dispatched", change)
				default:
				}
			}
		})
	}
}

func TestNativeBridgeUsesHostOutputLimit(t *testing.T) {
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		t.Run(field, func(t *testing.T) {
			// One terminal refusal keeps this unit peer independent of the host's concurrent response drain.
			reply := nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted"}}`))
			server, arrivals := geminiFixtureServer(t, reply)
			peer, _ := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			payload := nativeFields(nativeText(21, "fixture-model"), nativeText(27, "opaque-cache"),
				nativeText(3, string(nativeFields(nativeNumber(2, 1), nativeText(3, "hello")))),
				nativeText(8, string(nativeFields(nativeNumber(2, 64), nativeNumber(7, 19)))))
			result, _ := peer.call(t, sdk.MethodInboundDecode, sdk.NativeRequest{ID: "devin", ResponseID: "limit", Headers: chatHeaders(), Body: nativeFrame(0, payload)})
			var decoded sdk.DecodedRequest
			if result.Error != nil || json.Unmarshal(result.Result, &decoded) != nil {
				t.Fatal(result.Error)
			}
			defer peer.call(t, sdk.MethodInboundResponse, sdk.ResponseEvent{ID: "devin", ResponseID: "limit", Action: "finish"})
			var canonical map[string]any
			json.Unmarshal(decoded.Body, &canonical)
			delete(canonical, "max_tokens")
			canonical[field] = 7
			body, _ := json.Marshal(canonical)
			request := fixtureExecute(body, sdk.ExecuteOptions{CallerID: "caller-one", Native: decoded.Metadata})
			result, _ = peer.call(t, sdk.MethodProviderExecute, request)
			if result.Error == nil || result.Error.Outcome != "uncertain" {
				t.Fatal("fixture refusal lost", result.Error)
			}
			raw := <-arrivals
			envelope, err := codec.ParseNativeEnvelope(t.Context(), chatHeaders(), nativeFrame(0, raw))
			if err != nil {
				t.Fatal(err)
			}
			var projection map[string]any
			json.Unmarshal(envelope.Projection, &projection)
			if projection["max_tokens"] != float64(7) {
				t.Fatal("native payload ignored host output limit", projection["max_tokens"])
			}
			before, _ := codec.RawFields(payload)
			after, _ := codec.RawFields(raw)
			for _, number := range []int{3, 27} {
				if !bytes.Equal(before[number], after[number]) {
					t.Fatal("opaque field changed", number)
				}
			}
			if !bytes.Contains(after[8], nativeNumber(7, 19)) {
				t.Fatal("unrelated sampling control lost")
			}
		})
	}
}

func TestClaudeChatFailureDoesNotIssueSignature(t *testing.T) {
	for name, end := range map[string][]byte{
		"truncated":      nil,
		"vendor refusal": nativeFrame(2, []byte(`{"error":{"code":"permission_denied"}}`)),
		"invalid stop":   nativeFields(nativeFrame(0, nativeNumber(5, 8)), nativeFrame(2, []byte(`{}`))),
	} {
		t.Run(name, func(t *testing.T) {
			reply := nativeFields(nativeFrame(0, nativeFields(nativeText(9, "thought"), nativeText(16, "thought-1"))),
				nativeFrame(0, nativeFields(nativeText(10, "never-issued"), nativeText(16, "thought-1"), nativeText(21, "anthropic"))), end)
			server, arrivals := geminiFixtureServer(t, reply)
			peer, store := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			body := []byte(`{"model":"claude-fixture","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
			request := fixtureExecute(body, sdk.ExecuteOptions{CallerID: "caller-one"})
			request.Model = "claude-fixture"
			result, events := peer.call(t, sdk.MethodProviderExecute, request)
			<-arrivals
			if result.Error == nil || store.writes != 0 {
				t.Fatal("failed stream issued an origin", result.Error, store.writes)
			}
			if bytes.Contains(correctionData(t, events), []byte("[DONE]")) {
				t.Fatal("failed stream fabricated success")
			}
		})
	}
}

func TestClaudeChatMultipleThoughtsContinue(t *testing.T) {
	reply := nativeFields(
		nativeFrame(0, nativeFields(nativeText(9, "first thought"), nativeText(16, "thought-1"))),
		nativeFrame(0, nativeFields(nativeText(10, "signature-one"), nativeText(16, "thought-1"), nativeText(21, "anthropic"), nativeText(23, "claude-fixture"))),
		nativeFrame(0, nativeFields(nativeText(9, "second thought"), nativeText(16, "thought-2"))),
		nativeFrame(0, nativeFields(nativeText(10, "signature-two"), nativeText(16, "thought-2"))),
		nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeNumber(5, 2))), nativeFrame(2, []byte(`{}`)))
	server, arrivals := geminiFixtureServer(t, reply)
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	execute := func(messages []any) (sdk.Frame, []sdk.Frame) {
		body, _ := json.Marshal(map[string]any{"model": "claude-fixture", "messages": messages})
		options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/chat/completions", CallerID: "caller-one"})
		return peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "claude-fixture", OperationID: "fixture-op", Body: body, Options: options})
	}
	user := map[string]any{"role": "user", "content": "hello"}
	result, events := execute([]any{user})
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	<-arrivals
	body := correctionData(t, events)
	var value struct {
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &value); err != nil || len(value.Choices) != 1 {
		t.Fatalf("response: %s", body)
	}
	t.Logf("returned assistant=%s", body)
	result, _ = execute([]any{user, value.Choices[0].Message, map[string]any{"role": "user", "content": "continue"}})
	if result.Error != nil {
		t.Fatalf("ordinary continuation of own two-thought reply rejected: %v", result.Error)
	}
	<-arrivals
}

func TestClaudeChatMultipleThoughtsStreamingTools(t *testing.T) {
	for _, metadata := range []string{"inline", "leading", "trailing"} {
		for _, streaming := range []bool{false, true} {
			for _, tools := range []bool{false, true} {
				t.Run(fmt.Sprintf("metadata=%s/stream=%t/tools=%t", metadata, streaming, tools), func(t *testing.T) {
					meta := nativeFields(nativeText(21, "anthropic"), nativeText(23, "claude-fixture"))
					signature := nativeFields(nativeText(10, "signature-"), nativeText(16, "one"))
					if metadata == "inline" {
						signature = append(signature, meta...)
					}
					reply := nativeFields(
						nativeFrame(0, nativeFields(nativeText(9, "first thought"), nativeText(16, "one"))),
						nativeFrame(0, signature),
						nativeFrame(0, nativeFields(nativeText(10, "one"), nativeText(16, "one"))),
						nativeFrame(0, nativeFields(nativeText(9, "second thought"), nativeText(16, "two"))),
						nativeFrame(0, nativeFields(nativeText(10, "signature-"), nativeText(16, "two"))),
						nativeFrame(0, nativeFields(nativeText(10, "two"), nativeText(16, "two"))),
						nativeFrame(0, nativeText(3, "answer")))
					if metadata == "leading" {
						reply = append(nativeFrame(0, meta), reply...)
					}
					if metadata == "trailing" {
						reply = append(reply, nativeFrame(0, meta)...)
					}
					stop := uint64(2)
					if tools {
						stop = 10
						reply = append(reply, nativeFrame(0, nativeText(6, string(nativeFields(nativeText(1, "tool-one"), nativeText(2, "read"), nativeText(3, `{"path":`)))))...)
						reply = append(reply, nativeFrame(0, nativeText(6, string(nativeText(3, `"a"}`))))...)
					}
					reply = append(reply, nativeFrame(0, nativeNumber(5, stop))...)
					reply = append(reply, nativeFrame(2, []byte(`{}`))...)
					server, arrivals := geminiFixtureServer(t, reply)
					peer, _ := providerPeer(t, server.URL)
					selectAccount(peer, server.URL)
					model := "claude-fixture"
					execute := func(caller string, messages []any, streaming bool) (sdk.Frame, []sdk.Frame) {
						request := map[string]any{"model": model, "messages": messages, "stream": streaming}
						if tools {
							request["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}}}
						}
						body, _ := json.Marshal(request)
						options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/chat/completions", CallerID: caller})
						return peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: model, OperationID: "fixture-op", Body: body, Options: options})
					}
					user := map[string]any{"role": "user", "content": "hello"}
					result, events := execute("caller-one", []any{user}, streaming)
					if result.Error != nil {
						t.Fatal(result.Error)
					}
					<-arrivals
					assistant := multipleThoughtAssistant(t, correctionData(t, events), streaming)
					details, ok := assistant["reasoning_details"].([]any)
					if !ok || len(details) != 2 {
						t.Fatalf("lost thought identities: %+v", assistant)
					}
					for i, word := range []string{"one", "two"} {
						d := details[i].(map[string]any)
						if d["signature"] != "signature-"+word || d["format"] != "anthropic" {
							t.Fatalf("block %d: %+v", i, d)
						}
					}
					history := []any{user, assistant}
					if tools {
						history = append(history, map[string]any{"role": "tool", "tool_call_id": "tool-one", "content": "done"})
					}
					history = append(history, map[string]any{"role": "user", "content": "continue"})
					result, _ = execute("caller-one", history, false)
					if result.Error != nil {
						t.Fatal("own returned message refused", result.Error)
					}
					raw := <-arrivals
					fields, err := codec.RawFields(raw)
					if err != nil {
						t.Fatal(err)
					}
					prompts := fields[3]
					seen := 0
					for len(prompts) > 0 {
						_, n := binary.Uvarint(prompts)
						prompts = prompts[n:]
						size, n := binary.Uvarint(prompts)
						prompts = prompts[n:]
						block := prompts[:int(size)]
						prompts = prompts[int(size):]
						for i, word := range []string{"one", "two"} {
							if bytes.Contains(block, nativeText(12, "signature-"+word)) {
								text := []string{"first thought", "second thought"}[i]
								if !bytes.Contains(block, nativeText(11, text)) || !bytes.Contains(block, nativeText(22, "claude-fixture")) {
									t.Fatal("signature detached from its thought", word)
								}
								seen++
							}
						}
					}
					if seen != 2 {
						t.Fatalf("native thought prompts=%d", seen)
					}
					for _, foreign := range []string{"caller", "account", "generation", "model", "second-signature"} {
						selectAccount(peer, server.URL)
						caller := "caller-one"
						model = "claude-fixture"
						switch foreign {
						case "caller":
							caller = "caller-two"
						case "generation":
							peer.scope.Generation++
						case "account":
							credentials := outbound.Credentials{Key: "other-fixture-key"}
							peer.auth, _ = json.Marshal(credentials)
							peer.scope.Account = credentials.AccountID(server.URL)
						case "model":
							model = "claude-other"
						case "second-signature":
							details[1].(map[string]any)["signature"] = "unissued"
						}
						result, events = execute(caller, history, false)
						if result.Error == nil || result.Error.Outcome != "not-started" {
							t.Fatal("foreign multi-block origin dispatched", foreign)
						}
						for _, ev := range events {
							if ev.Event == "accepted" {
								t.Fatal("foreign multi-block origin accepted")
							}
						}
						select {
						case <-arrivals:
							t.Fatal("foreign origin reached upstream")
						default:
						}
					}
				})
			}
		}
	}
}

// Reassemble the actual SSE by its public indexes as a Chat client does.
func multipleThoughtAssistant(t *testing.T, body []byte, streaming bool) map[string]any {
	t.Helper()
	if !streaming {
		var value struct {
			Choices []struct {
				Message map[string]any `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(body, &value) != nil || len(value.Choices) != 1 {
			t.Fatal(string(body))
		}
		return value.Choices[0].Message
	}
	assistant := map[string]any{"role": "assistant", "content": "", "reasoning_content": ""}
	details := []any{}
	calls := []any{}
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if !bytes.HasPrefix(line, []byte("data: {")) {
			continue
		}
		var value struct {
			Choices []struct {
				Delta map[string]any `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &value); err != nil {
			t.Fatal(err)
		}
		for _, choice := range value.Choices {
			for _, key := range []string{"content", "reasoning_content"} {
				if v, ok := choice.Delta[key].(string); ok {
					assistant[key] = assistant[key].(string) + v
				}
			}
			if ds, ok := choice.Delta["reasoning_details"].([]any); ok {
				for _, raw := range ds {
					d := raw.(map[string]any)
					i := int(d["index"].(float64))
					for len(details) <= i {
						details = append(details, map[string]any{"type": "reasoning.text", "text": "", "signature": "", "format": "", "index": len(details)})
					}
					target := details[i].(map[string]any)
					for _, key := range []string{"text", "signature", "format"} {
						if v, ok := d[key].(string); ok {
							target[key] = target[key].(string) + v
						}
					}
				}
			}
			if ts, ok := choice.Delta["tool_calls"].([]any); ok {
				for _, raw := range ts {
					d := raw.(map[string]any)
					i := int(d["index"].(float64))
					for len(calls) <= i {
						calls = append(calls, map[string]any{"id": "", "type": "function", "function": map[string]any{"name": "", "arguments": ""}})
					}
					target := calls[i].(map[string]any)
					if id, ok := d["id"].(string); ok {
						target["id"] = id
					}
					if f, ok := d["function"].(map[string]any); ok {
						tf := target["function"].(map[string]any)
						for _, key := range []string{"name", "arguments"} {
							if v, ok := f[key].(string); ok {
								tf[key] = tf[key].(string) + v
							}
						}
					}
				}
			}
		}
	}
	assistant["reasoning_details"] = details
	if len(calls) > 0 {
		assistant["tool_calls"] = calls
	}
	return assistant
}

func TestClaudeChatSeparateSignatureMetadata(t *testing.T) {
	reply := nativeFields(
		nativeFrame(0, nativeFields(nativeText(21, "anthropic"), nativeText(23, "claude-fixture"))),
		nativeFrame(0, nativeFields(nativeText(9, "first thought"), nativeText(16, "thought-1"))),
		nativeFrame(0, nativeFields(nativeText(10, "signature-one"), nativeText(16, "thought-1"))),
		nativeFrame(0, nativeFields(nativeText(9, "second thought"), nativeText(16, "thought-2"))),
		nativeFrame(0, nativeFields(nativeText(10, "signature-two"), nativeText(16, "thought-2"))),
		nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeNumber(5, 2))), nativeFrame(2, []byte(`{}`)))
	server, arrivals := geminiFixtureServer(t, reply)
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	execute := func(messages []any) (sdk.Frame, []sdk.Frame) {
		body, _ := json.Marshal(map[string]any{"model": "claude-fixture", "messages": messages})
		options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/chat/completions", CallerID: "caller-one"})
		return peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "claude-fixture", OperationID: "fixture-op", Body: body, Options: options})
	}
	user := map[string]any{"role": "user", "content": "hello"}
	result, events := execute([]any{user})
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	<-arrivals
	body := correctionData(t, events)
	var value struct {
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &value); err != nil || len(value.Choices) != 1 {
		t.Fatalf("response: %s", body)
	}
	t.Logf("returned assistant=%s", body)
	result, _ = execute([]any{user, value.Choices[0].Message, map[string]any{"role": "user", "content": "continue"}})
	if result.Error != nil {
		t.Fatalf("ordinary continuation of own two-thought reply rejected: %v", result.Error)
	}
	<-arrivals
}
