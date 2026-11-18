package devin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestProviderMessagesKeepsCacheBlocksAndScopedSignatures(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(request.Body)
		if request.URL.Path != "/exa.api_server_pb.ApiServerService/GetChatMessage" ||
			request.Header.Get("Authorization") != "Basic selected-fixture-secret" ||
			!bytes.Contains(body, []byte("claude-fixture")) {
			t.Error("Messages lost native model or selected credentials")
		}
		writer.Header().Set("Content-Type", "application/connect+proto")
		writer.Write(nativeFrame(0, nativeFields(nativeText(9, "native thought"), nativeText(16, "thought-1"))))
		writer.Write(nativeFrame(0, nativeFields(nativeText(10, "synthetic-signed"), nativeText(16, "thought-1"),
			nativeText(21, "anthropic-bedrock"), nativeText(23, "claude-fixture"))))
		usage := nativeFields(nativeNumber(2, 7), nativeNumber(3, 3), nativeNumber(4, 5), nativeNumber(5, 11))
		writer.Write(nativeFrame(0, nativeFields(nativeText(3, "Messages answer"), nativeNumber(5, 2), nativeText(7, string(usage)))))
		writer.Write(nativeFrame(2, []byte(`{}`)))
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "caller-one"})
	execute := func(body string) (sdk.Frame, []sdk.Frame) {
		return peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "anthropic", Model: "claude-fixture",
			OperationID: "fixture-op", Options: options, Body: []byte(body)})
	}
	first := `{"model":"claude-fixture","max_tokens":64,"system":[{"type":"text","text":"instructions","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"prefix-A"},{"type":"text","text":"prefix-B","cache_control":{"type":"ephemeral"}}]}]}`
	result, events := execute(first)
	if result.Error != nil {
		t.Fatal("ordinary Messages request is unsupported", result.Error)
	}
	var content []byte
	for _, event := range events {
		if event.Event == "data" {
			var data []byte
			json.Unmarshal(event.Data, &data)
			content = append(content, data...)
		}
	}
	var reply struct {
		Type    string `json:"type"`
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Thinking  string `json:"thinking"`
			Signature string `json:"signature"`
		} `json:"content"`
		Stop  string `json:"stop_reason"`
		Usage struct {
			Input      int `json:"input_tokens"`
			Output     int `json:"output_tokens"`
			CacheRead  int `json:"cache_read_input_tokens"`
			CacheWrite int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(content, &reply) != nil || reply.Type != "message" || reply.Stop != "end_turn" ||
		len(reply.Content) != 2 || reply.Content[0].Thinking != "native thought" ||
		reply.Content[0].Signature != "synthetic-signed" || reply.Content[1].Text != "Messages answer" ||
		reply.Usage.Input != 7 || reply.Usage.Output != 3 || reply.Usage.CacheRead != 11 || reply.Usage.CacheWrite != 5 {
		t.Fatal("Messages response lost blocks, signatures or exclusive usage", string(content))
	}
	continued := `{"model":"claude-fixture","max_tokens":64,"messages":[{"role":"user","content":"hello"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"native thought","signature":"synthetic-signed"},
			{"type":"text","text":"Messages answer"}]},{"role":"user","content":"continue"}]}`
	result, _ = execute(continued)
	if result.Error != nil || requests.Load() != 2 {
		t.Fatal("known scoped signature could not resume", result, requests.Load())
	}
	for _, change := range []string{"generation", "caller", "model", "signature"} {
		peer.scope.Generation = 3
		options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "caller-one"})
		body := continued
		model := "claude-fixture"
		switch change {
		case "generation":
			peer.scope.Generation++
		case "caller":
			options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "another-caller"})
		case "model":
			model = "claude-other"
			body = string(bytes.ReplaceAll([]byte(body), []byte("claude-fixture"), []byte(model)))
		case "signature":
			body = string(bytes.ReplaceAll([]byte(body), []byte("synthetic-signed"), []byte("foreign-signed")))
		}
		result, events = peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "anthropic", Model: model,
			OperationID: "fixture-op", Options: options, Body: []byte(body)})
		if result.Error == nil || result.Error.Code != "permission_denied" || result.Error.Outcome != "not-started" ||
			requests.Load() != 2 {
			t.Fatal("foreign signature crossed acceptance", change, result, requests.Load())
		}
		for _, event := range events {
			if event.Event == "accepted" {
				t.Fatal("foreign signature was accepted", change)
			}
		}
		var headers sdk.HeadersEvent
		for _, event := range events {
			if event.Event == "headers" {
				json.Unmarshal(event.Data, &headers)
			}
		}
		if headers.Status != http.StatusForbidden {
			t.Fatal("origin refusal lost its nonretryable HTTP status", change, headers.Status)
		}
	}
}

func TestProviderMessagesFailuresDoNotIssueOriginReceipts(t *testing.T) {
	for name, end := range map[string][]byte{
		"truncated":                nil,
		"vendor refusal":           nativeFrame(2, []byte(`{"error":{"code":"permission_denied"}}`)),
		"unknown control":          nativeFrame(0, nativeNumber(24, 1)),
		"invalid stop":             nativeFields(nativeFrame(0, nativeNumber(5, 8)), nativeFrame(2, []byte(`{}`))),
		"missing signature format": nativeFields(nativeFrame(0, nativeNumber(5, 2)), nativeFrame(2, []byte(`{}`))),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/connect+proto")
				writer.Write(nativeFrame(0, nativeFields(nativeText(9, "native thought"), nativeText(16, "thought"))))
				signature := nativeFields(nativeText(10, "never-issued"), nativeText(16, "thought"))
				if name != "missing signature format" {
					signature = append(signature, nativeText(21, "anthropic-bedrock")...)
				}
				writer.Write(nativeFrame(0, signature))
				writer.Write(end)
			}))
			defer server.Close()
			peer, store := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "caller-one"})
			result, events := peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "anthropic", Model: "claude-fixture",
				OperationID: "fixture-op", Options: options, Body: []byte(`{"model":"claude-fixture","stream":true,"max_tokens":64,
					"messages":[{"role":"user","content":"hello"}]}`)})
			if result.Error == nil || store.writes != 0 {
				t.Fatal("failed stream issued an origin or successful outcome", result, store.writes)
			}
			var body []byte
			for _, event := range events {
				if event.Event == "data" {
					var data []byte
					json.Unmarshal(event.Data, &data)
					body = append(body, data...)
				}
			}
			if bytes.Contains(body, []byte("message_stop")) {
				t.Fatal("failed Messages stream fabricated success", string(body))
			}
		})
	}
}

func TestProviderMessagesNeedsHostCallerBeforeAcceptance(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.WriteHeader(500)
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	options, _ := json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages"})
	result, events := peer.call(t, sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "anthropic", Model: "claude-fixture",
		OperationID: "fixture-op", Options: options,
		Body: []byte(`{"model":"claude-fixture","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)})
	if result.Error == nil || result.Error.Code != "permission_denied" || result.Error.Outcome != "not-started" ||
		requests.Load() != 0 {
		t.Fatal("Messages without stable host caller crossed acceptance", result, requests.Load())
	}
	for _, event := range events {
		if event.Event == "accepted" {
			t.Fatal("unscoped canonical Messages was accepted")
		}
	}
	var headers sdk.HeadersEvent
	for _, event := range events {
		if event.Event == "headers" {
			json.Unmarshal(event.Data, &headers)
		}
	}
	if headers.Status != http.StatusForbidden {
		t.Fatal("missing caller refusal became a retryable provider error", headers.Status)
	}
}
