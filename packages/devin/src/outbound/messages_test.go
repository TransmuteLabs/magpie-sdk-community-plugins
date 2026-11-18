package outbound

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func TestMessagesRenderersKeepBlockOrderToolFragmentsAndExclusiveUsage(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[streaming], func(t *testing.T) {
			writer := httptest.NewRecorder()
			renderer := NewMessagesRenderer(writer, "claude-fixture", streaming)
			for _, delta := range []codec.NativeDelta{
				{Thinking: "first", ThinkingID: "one"},
				{Signature: "sig-", SignatureType: "anthropic-bedrock", ThinkingID: "one"},
				{Signature: "one", ThinkingID: "one"},
				{Text: "answer"},
				{Thinking: "second", ThinkingID: "two"},
				{Signature: "sig-two", SignatureType: "anthropic-bedrock", ThinkingID: "two"},
				{Tools: []codec.NativeToolDelta{{ID: "native tool", Name: "read", Arguments: `{"path":`}}},
				{Tools: []codec.NativeToolDelta{{Arguments: `"a"}`}}},
			} {
				if err := renderer.Delta(delta); err != nil {
					t.Fatal(err)
				}
			}
			summary := codec.NativeSummary{Stop: 10, Usage: &codec.NativeUsage{Input: 7, Output: 3, CacheRead: 11, CacheWrite: 5}}
			if err := renderer.Finish(summary); err != nil {
				t.Fatal(err)
			}
			body := writer.Body.String()
			if streaming {
				var kinds []string
				starts, stops := 0, 0
				for _, line := range strings.Split(body, "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var value map[string]json.RawMessage
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &value) != nil {
						t.Fatal(line)
					}
					var kind string
					json.Unmarshal(value["type"], &kind)
					kinds = append(kinds, kind)
					if kind == "content_block_start" {
						starts++
					}
					if kind == "content_block_stop" {
						stops++
					}
					if kind == "message_delta" && (!strings.Contains(line, `"input_tokens":7`) ||
						!strings.Contains(line, `"cache_read_input_tokens":11`) ||
						!strings.Contains(line, `"stop_reason":"tool_use"`)) {
						t.Fatal("Messages stream lost usage or terminal", line)
					}
				}
				if len(kinds) == 0 || kinds[0] != "message_start" || kinds[len(kinds)-1] != "message_stop" ||
					starts != 4 || stops != 4 || strings.Contains(body, "[DONE]") {
					t.Fatal("Messages events lost block pairing", body)
				}
			} else {
				var value struct {
					Content []struct {
						Type      string            `json:"type"`
						Signature string            `json:"signature"`
						Input     map[string]string `json:"input"`
					} `json:"content"`
					Usage map[string]uint64 `json:"usage"`
				}
				if json.Unmarshal(writer.Body.Bytes(), &value) != nil || len(value.Content) != 4 ||
					value.Content[0].Signature != "sig-one" || value.Content[2].Signature != "sig-two" ||
					value.Content[3].Input["path"] != "a" || value.Usage["input_tokens"] != 7 ||
					value.Usage["cache_creation_input_tokens"] != 5 {
					t.Fatal("Messages JSON reordered or weakened blocks", body)
				}
			}
		})
	}
}

func TestMessagesRendererRefusesUnrepresentableAndIncompleteOutput(t *testing.T) {
	for name, deltas := range map[string][]codec.NativeDelta{
		"orphan signature":       {{Signature: "foreign"}},
		"ambiguous order":        {{Thinking: "thought", Text: "answer"}},
		"phase":                  {{Text: "answer", Phase: "analysis"}},
		"reopened thinking":      {{Thinking: "one", ThinkingID: "a"}, {Text: "middle"}, {Thinking: "again", ThinkingID: "a"}},
		"missing signature type": {{Thinking: "one"}, {Signature: "sig"}},
		"invalid tool JSON":      {{Tools: []codec.NativeToolDelta{{ID: "a", Name: "read", Arguments: `{"path":`}}}},
	} {
		t.Run(name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			renderer := NewMessagesRenderer(writer, "claude-fixture", true)
			var err error
			for _, delta := range deltas {
				if err = renderer.Delta(delta); err != nil {
					break
				}
			}
			if err == nil {
				err = renderer.Finish(codec.NativeSummary{Stop: 2})
			}
			if err == nil {
				t.Fatal("unsupported native output became success")
			}
			if err := renderer.Fail(502); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(writer.Body.String(), "message_stop") {
				t.Fatal("failed stream fabricated a successful terminal")
			}
		})
	}
}
