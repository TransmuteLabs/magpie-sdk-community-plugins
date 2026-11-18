package commandcode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestProviderStreamCompletionPreservesBytesAndUncertainty(test *testing.T) {
	for _, protocol := range []struct {
		name     string
		start    string
		terminal string
		invalid  string
	}{
		{"chat", `{"choices":[{"delta":{"content":"partial"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`, `{"choices":"invalid"}`},
		{"responses", `{"type":"response.output_text.delta","delta":"partial"}`,
			`{"type":"response.completed","response":{"status":"completed"}}`, `{"type":123}`},
		{"anthropic", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
			`{"type":"message_stop"}`, `{"delta":{"stop_reason":{}}}`},
	} {
		start := "data: " + protocol.start + "\r\n\r\n"
		terminal := "data: " + protocol.terminal + "\r\n\r\n"
		for _, scenario := range []struct {
			name      string
			body      string
			streaming bool
			sse       bool
			completed bool
		}{
			{"terminal", ": keepalive\r\nid: 7\r\n" + start + terminal + ": tail\r\n\r\n", true, true, true},
			{"terminal-at-eof", start + "data: " + protocol.terminal, true, true, true},
			{"multiline-terminal", start + "data: " + strings.Replace(protocol.terminal, "{", "{\r\ndata: ", 1) + "\r\n\r\n", true, true, true},
			{"done-marker", start + "data: [DONE]\r\n\r\n", true, true, protocol.name == "chat"},
			{"eof-only", start, true, true, false},
			{"unsolicited-sse-eof", start, false, true, false},
			{"wrong-protocol-terminal", start + "data: {\"type\":\"unrelated.completed\"}\r\n\r\n", true, true, false},
			{"malformed-before-terminal", "data: {INVALID}\r\n\r\n" + start + terminal, true, true, false},
			{"typed-malformed-before-terminal", "data: " + protocol.invalid + "\r\n\r\n" + start + terminal, true, true, false},
			{"malformed-after-terminal", start + terminal + "data: {INVALID}\r\n\r\n", true, true, false},
			{"opaque-whole", `{"vendor_extension":{"keep":true}}`, false, false, true},
		} {
			test.Run(protocol.name+"/"+scenario.name, func(test *testing.T) {
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/alpha/billing/subscriptions" {
						io.WriteString(writer, `{"data":{"planId":"individual-pro","status":"active"}}`)
						return
					}
					calls.Add(1)
					if scenario.sse {
						writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
					} else {
						writer.Header().Set("Content-Type", "application/json")
					}
					for offset := 0; offset < len(scenario.body); offset += 7 {
						io.WriteString(writer, scenario.body[offset:min(offset+7, len(scenario.body))])
						writer.(http.Flusher).Flush()
					}
				}))
				defer upstream.Close()
				worker := fixture(test, Config{APIURL: upstream.URL, StudioURL: "https://commandcode.ai"})
				body := []byte(fmt.Sprintf(`{"model":"fixture","stream":%t}`, scenario.streaming))
				raw, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{
					Protocol: protocol.name, Model: "fixture", Body: body,
				}, testAuth())
				status, forwarded := eventsBody(test, events)
				if calls.Load() != 1 || status != http.StatusOK || !bytes.Equal(forwarded, []byte(scenario.body)) {
					test.Fatalf("provider forwarding changed or replayed: calls=%d status=%d body=%s", calls.Load(), status, forwarded)
				}
				if scenario.completed {
					var result sdk.ExecuteResult
					if err != nil || json.Unmarshal(raw, &result) != nil || result.Outcome != "completed" {
						test.Fatalf("explicit completed response lost its receipt: %s %v", raw, err)
					}
				} else {
					failure, ok := err.(*sdk.Error)
					if !ok || failure.Outcome != "uncertain" {
						test.Fatalf("unfinished provider stream published completion: result=%s error=%v", raw, err)
					}
				}
			})
		}
	}
}
