package zcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/mcpwire"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type mcpFixture struct {
	server          *httptest.Server
	calls           atomic.Int32
	initializations atomic.Int32
	cancelled       chan string
	started         chan struct{}
	requestsMu      sync.Mutex
	ids             map[string]bool
}

func remoteFixture(t *testing.T, mode string, stateless bool) *mcpFixture {
	t.Helper()
	value := &mcpFixture{cancelled: make(chan string, 1), started: make(chan struct{}), ids: map[string]bool{}}
	value.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/oauth/profile" {
			profile(writer, request, "fixture-user")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != imageSearchEndpoint {
			t.Errorf("remote MCP used another endpoint/method: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer fixture-zcode-session" ||
			request.Header.Get("X-Bigmodel-Authorization") != "Bearer fixture-business-token" ||
			request.Header.Get("Bigmodel-Target-Type") != "PERSONAL" ||
			request.Header.Get("X-Coding-Plan-Api-Key") != "" || request.Header.Get("X-Api-Key") != "" {
			t.Error("remote MCP mixed selected credential channels")
		}
		if request.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Error("MCP did not advertise both bounded response transports")
		}
		var message struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      string         `json:"id"`
			Method  string         `json:"method"`
			Params  map[string]any `json:"params"`
		}
		if json.NewDecoder(request.Body).Decode(&message) != nil || message.JSONRPC != "2.0" {
			t.Error("invalid MCP request framing")
		}
		if message.Method != "initialize" {
			expectedSession := "owned-server-session"
			if stateless {
				expectedSession = ""
			}
			if request.Header.Get("Mcp-Session-Id") != expectedSession ||
				request.Header.Get("MCP-Protocol-Version") != mcpwire.Version {
				t.Error("MCP session/protocol headers changed or were borrowed")
			}
		}
		if message.ID != "" {
			value.requestsMu.Lock()
			if value.ids[message.ID] {
				t.Error("a JSON-RPC request ID was repeated")
			}
			value.ids[message.ID] = true
			value.requestsMu.Unlock()
		}
		write := func(result any) {
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
		}
		switch message.Method {
		case "initialize":
			value.initializations.Add(1)
			if request.Header.Get("Mcp-Session-Id") != "" || message.Params["protocolVersion"] != mcpwire.Version {
				t.Error("initialize reused a session or changed its pinned protocol")
			}
			if mode == "redirect" {
				writer.Header().Set("Location", value.server.URL+"/untrusted")
				writer.WriteHeader(307)
				return
			}
			if !stateless {
				writer.Header().Set("Mcp-Session-Id", "owned-server-session")
			}
			version := mcpwire.Version
			if mode == "version" {
				version = "2026-07-28"
			}
			write(map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]string{"name": "fixture", "version": "1"}})
		case "notifications/initialized":
			if message.ID != "" {
				t.Error("initialized notification had an ID")
			}
			writer.WriteHeader(202)
		case "tools/list":
			name := imageSearchRemoteTool
			if mode == "unknown-tool" {
				name = "new_dynamic_tool"
			}
			if strings.HasPrefix(mode, "cursor-") {
				cursor, _ := message.Params["cursor"].(string)
				if cursor == "" || mode == "cursor-loop" {
					write(map[string]any{"tools": []any{}, "nextCursor": "next-page"})
					return
				}
				if cursor != "next-page" {
					t.Error("MCP pagination changed the server cursor")
				}
			}
			schema := map[string]any{"type": "object", "properties": map[string]any{"query": map[string]string{"type": "string"}},
				"required": []string{"query"}}
			if mode == "new-required" {
				schema["required"] = []string{"query", "newArgument"}
			}
			if mode == "bad-query-type" {
				schema["properties"] = map[string]any{"query": map[string]string{"type": "array"}}
			}
			if mode == "too-many-tools" {
				tools := make([]any, 129)
				for index := range tools {
					tools[index] = map[string]any{"name": fmt.Sprintf("other-%d", index)}
				}
				write(map[string]any{"tools": tools})
				return
			}
			write(map[string]any{"tools": []any{map[string]any{"name": name,
				"inputSchema": schema}}})
		case "tools/call":
			value.calls.Add(1)
			if message.Params["name"] != imageSearchRemoteTool {
				t.Error("caller selected an undeclared remote tool")
			}
			arguments := mapObject(message.Params["arguments"])
			if len(arguments) != 1 || arguments["query"] != "synthetic query" {
				t.Error("MCP request changed or augmented the declared arguments")
			}
			if mode == "cancel" {
				close(value.started)
				writer.Header().Set("Content-Type", "text/event-stream")
				writer.WriteHeader(200)
				writer.(http.Flusher).Flush()
				<-request.Context().Done()
				return
			}
			if mode == "http-error" {
				writer.WriteHeader(503)
				return
			}
			if mode == "bad-id" {
				message.ID = "another-call"
			}
			if mode == "changed-session" {
				writer.Header().Set("Mcp-Session-Id", "foreign-session")
			}
			if mode == "rpc-error" {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%q,"error":{"code":-32602,"message":"private-not-for-log","data":{"token":"private"}}}`, message.ID)
				return
			}
			result := map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"image":"synthetic"}`}},
				"structuredContent": map[string]any{"large": json.Number("9007199254740993")}, "_meta": map[string]string{"fixture": "retained"}}
			if mode == "tool-error" {
				result["isError"] = true
			}
			if mode == "sse" {
				writer.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(writer, ": heartbeat\n\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progress\":1}}\n\n")
				body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
				fmt.Fprintf(writer, "event: message\ndata: %s\n\n", body)
			} else {
				write(result)
			}
		case "notifications/cancelled":
			id, _ := message.Params["requestId"].(string)
			if message.ID != "" || id == "" {
				t.Error("cancellation notification lost the original request ID")
			}
			value.cancelled <- id
			writer.WriteHeader(202)
		default:
			t.Error("an undeclared MCP method was sent")
			writer.WriteHeader(404)
		}
	}))
	t.Cleanup(value.server.Close)
	return value
}

func TestFixedRemoteMCPJSONSSEAndStatelessSessions(t *testing.T) {
	for _, scenario := range []struct {
		mode      string
		stateless bool
	}{{"json", false}, {"sse", false}, {"json", true}, {"tool-error", true}, {"cursor-next", true}} {
		t.Run(fmt.Sprintf("%s/stateless=%t", scenario.mode, scenario.stateless), func(t *testing.T) {
			fixture := remoteFixture(t, scenario.mode, scenario.stateless)
			h := newHarness(t, endpointOptions(fixture.server))
			account := nativeCredentials()
			frame, _ := h.call(sdk.MethodToolCall, scope(account), sdk.ToolRequest{ID: "zcode_image_search",
				Arguments: raw(map[string]string{"query": "synthetic query"})}, raw(account))
			var result sdk.ToolResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || !h.accepted.Load() ||
				fixture.calls.Load() != 1 || fixture.initializations.Load() != 1 ||
				result.IsError != (scenario.mode == "tool-error") ||
				!strings.Contains(string(result.Content), "9007199254740993") ||
				!strings.Contains(string(result.Content), "zcodeMCPResultFields") {
				t.Fatalf("fixed remote tool lost protocol/result semantics: %+v", frame.Error)
			}
			if h.invalidations != 0 {
				t.Fatal("MCP execution acknowledged or invalidated ordinary quota")
			}
		})
	}
}

func TestRemoteMCPFailuresNeverRetryOrInventDynamicTools(t *testing.T) {
	for _, mode := range []string{"version", "redirect", "unknown-tool", "bad-id", "changed-session", "rpc-error", "http-error",
		"cursor-loop", "new-required", "bad-query-type", "too-many-tools"} {
		t.Run(mode, func(t *testing.T) {
			fixture := remoteFixture(t, mode, false)
			h := newHarness(t, endpointOptions(fixture.server))
			account := nativeCredentials()
			frame, _ := h.call(sdk.MethodToolCall, scope(account), sdk.ToolRequest{ID: "zcode_image_search",
				Arguments: raw(map[string]string{"query": "synthetic query"})}, raw(account))
			if frame.Error == nil || fixture.initializations.Load() != 1 || fixture.calls.Load() > 1 ||
				strings.Contains(frame.Error.Message, "private") {
				t.Fatal("remote failure was retried, downgraded or exposed an upstream payload")
			}
			dispatched := mode == "bad-id" || mode == "changed-session" || mode == "rpc-error" || mode == "http-error"
			if dispatched && (frame.Error.Outcome != "uncertain" || !h.accepted.Load()) ||
				!dispatched && (fixture.calls.Load() != 0 || h.accepted.Load()) {
				t.Fatal("remote preflight/accepted uncertainty boundary changed")
			}
		})
	}
}

func TestFixedRemoteMCPRejectsCallerProtocolAuthorityBeforeNetwork(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	account := nativeCredentials()
	for _, input := range []map[string]any{
		{"query": "synthetic query", "url": "https://elsewhere.example"},
		{"query": "synthetic query", "tool": "new_dynamic_tool"},
		{"query": "synthetic query", "headers": map[string]string{"Authorization": "not-authority"}},
		{"query": "synthetic query", "session": "not-authority"},
		{"query": "synthetic query", "protocolVersion": "2026-07-28"},
		{"query": " "},
	} {
		frame, _ := h.call(sdk.MethodToolCall, scope(account),
			sdk.ToolRequest{ID: "zcode_image_search", Arguments: raw(input)}, raw(account))
		if frame.Error == nil || h.proxies != 0 || h.accepted.Load() {
			t.Fatal("caller arguments selected transport/credential/session authority")
		}
	}
}

func TestRemoteMCPCancellationClosesCallAndNotifiesWithoutReplay(t *testing.T) {
	fixture := remoteFixture(t, "cancel", false)
	h := newHarness(t, endpointOptions(fixture.server))
	account := nativeCredentials()
	go func() {
		select {
		case <-fixture.started:
			h.send(sdk.Frame{Kind: sdk.KindCancel, ID: "fixture-2"})
		case <-t.Context().Done():
		}
	}()
	frame, _ := h.call(sdk.MethodToolCall, scope(account), sdk.ToolRequest{ID: "zcode_image_search",
		Arguments: raw(map[string]string{"query": "synthetic query"})}, raw(account))
	if frame.Error == nil || fixture.calls.Load() != 1 {
		t.Fatal("cancelled MCP tool returned success or replayed")
	}
	select {
	case id := <-fixture.cancelled:
		fixture.requestsMu.Lock()
		known := fixture.ids[id]
		fixture.requestsMu.Unlock()
		if !known || !strings.HasSuffix(id, "-call") {
			t.Fatal("cancellation targeted another JSON-RPC request")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("bounded MCP cancellation notification was not delivered")
	}
}
