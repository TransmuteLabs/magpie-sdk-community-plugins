package mcpwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestJSONAndSSEPreserveMatchingTerminalResult(t *testing.T) {
	for _, fixture := range []struct{ kind, body string }{
		{"application/json; charset=utf-8", `{"jsonrpc":"2.0","id":"call","result":{"large":9007199254740993,"content":[]}}`},
		{"text/event-stream", ": keepalive\r\n\r\nevent: message\r\nid: event-one\r\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progress\":1}}\r\n\r\nretry: 1000\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\ndata: \"id\":\"call\",\"result\":{\"large\":9007199254740993,\"content\":[]}}\n\n"},
	} {
		result, err := Read(strings.NewReader(fixture.body), fixture.kind, "call")
		if err != nil || !bytes.Contains(result, []byte("9007199254740993")) || !json.Valid(result) {
			t.Fatalf("terminal result lost framing/identity/precision: %v", err)
		}
	}
}

func TestRPCIdentityAndEnvelopeCannotBeLoosened(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":"other","result":{}}`,
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
		`{"jsonrpc":"2.0","id":null,"result":{}}`,
		`{"jsonrpc":"1.0","id":"call","result":{}}`,
		`{"jsonrpc":"2.0","id":"call","result":{},"error":null}`,
		`{"jsonrpc":"2.0","id":"call","result":null}`,
		`{"jsonrpc":"2.0","id":"call","id":"call","result":{}}`,
		`[{"jsonrpc":"2.0","id":"call","result":{}}]`,
		`{"jsonrpc":"2.0","method":"notifications/progress"}`,
		`{"jsonrpc":"2.0","id":"call","error":{"code":"-1","message":"not-a-number"}}`,
	} {
		if _, err := Read(strings.NewReader(body), "application/json", "call"); err == nil {
			t.Fatal("accepted an unrelated or ambiguous JSON-RPC response")
		}
	}
	_, err := Read(strings.NewReader(`{"jsonrpc":"2.0","id":"call","error":{"code":-32602,"message":"sensitive-upstream-detail","data":{"private":"opaque"}}}`),
		"application/json", "call")
	var failure *RPCError
	if !errors.As(err, &failure) || failure.Code != -32602 || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("JSON-RPC error lost its code or exposed private upstream payload")
	}
}

func TestSSEBoundsTruncationAndServerRequestsFailExplicitly(t *testing.T) {
	for _, body := range []string{
		"data: {\"jsonrpc\":\"2.0\",\"id\":\"call\",\"result\":{}}\n",
		"event: endpoint\ndata: https://another-endpoint.example\n\n",
		"data: {\"jsonrpc\":\"2.0\",\"id\":\"other\",\"result\":{}}\n\n",
		"data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"result\":null}\n\n",
		"data: " + strings.Repeat("x", 256<<10) + "\n\n",
		strings.Repeat("data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n", MaxEvents+1),
	} {
		if _, err := Read(strings.NewReader(body), "text/event-stream", "call"); err == nil {
			t.Fatal("unsupported/truncated/unbounded SSE was presented as a result")
		}
	}
	_, err := Read(strings.NewReader("data: {\"jsonrpc\":\"2.0\",\"id\":\"server-request\",\"method\":\"roots/list\"}\n\n"),
		"text/event-stream", "call")
	if !errors.Is(err, ErrServerRequest) {
		t.Fatal("server-initiated request was silently discarded")
	}
	if _, err := Read(strings.NewReader(strings.Repeat(" ", MaxBytes+1)), "application/json", "call"); !errors.Is(err, ErrLimit) {
		t.Fatal("JSON body limit was not enforced")
	}
	if _, err := Read(strings.NewReader("{}"), "text/html", "call"); err == nil {
		t.Fatal("HTML error page was interpreted as MCP")
	}
}

func TestSessionAndRequestBuildersKeepExactProtocolOwnership(t *testing.T) {
	value, err := Session(nil)
	if err != nil || value != "" {
		t.Fatal("stateless MCP was rejected")
	}
	for _, headers := range [][]string{{""}, {"one", "two"}, {"contains space"}, {"unsafe\nid"}, {"非ascii"}} {
		if _, err := Session(headers); err == nil {
			t.Fatal("malformed/ambiguous session header accepted")
		}
	}
	body, err := Request("private-call-id", "tools/call", map[string]any{"name": "search_image", "arguments": map[string]string{"query": "fixture"}})
	var request map[string]any
	if err != nil || json.Unmarshal(body, &request) != nil || request["id"] != "private-call-id" || request["jsonrpc"] != "2.0" {
		t.Fatal("request ID/protocol changed")
	}
	notice, err := Notification("notifications/cancelled", map[string]string{"requestId": "private-call-id"})
	if err != nil || bytes.Contains(notice, []byte(`"id":`)) {
		t.Fatal("notification acquired a JSON-RPC request ID")
	}
	if _, err := Request("", "tools/call", nil); err == nil {
		t.Fatal("empty request ID accepted")
	}
	if _, err := Request("id", "arbitrary/method", nil); err == nil {
		t.Fatal("unsupported request method accepted")
	}
}
