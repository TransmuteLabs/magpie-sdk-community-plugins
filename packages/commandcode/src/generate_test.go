package commandcode

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeneratePreservesReasoningToolsImagesAndRoles(t *testing.T) {
	raw := []byte(`{"model":"m","messages":[{"role":"system","content":"system"},{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]},{"role":"assistant","reasoning_content":"thought","content":null,"tool_calls":[{"id":"call1","type":"function","function":{"name":"read","arguments":"{\"path\":\"x\"}"}}]},{"role":"tool","tool_call_id":"call1","content":"value"}],"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}],"max_tokens":300000}`)
	body, err := GenerateBody(raw, Place{Directory: "/workspace", Date: "2026-11-14", Environment: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Params struct {
			Messages []struct {
				Role    string
				Content []map[string]any
			}
			Max    int `json:"max_tokens"`
			System string
		}
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Params.System != "system" || wire.Params.Max != 200000 || len(wire.Params.Messages) != 3 {
		t.Fatalf("wire: %s", body)
	}
	if wire.Params.Messages[0].Content[1]["image"] != "data:image/png;base64,AA==" ||
		wire.Params.Messages[1].Content[0]["type"] != "reasoning" ||
		wire.Params.Messages[2].Role != "tool" ||
		wire.Params.Messages[2].Content[0]["toolName"] != "read" {
		t.Fatalf("lost semantic part: %s", body)
	}
}

func TestGenerateRefusesUnsupportedOrMalformedSemantics(t *testing.T) {
	for _, raw := range []string{
		`{"model":"m","messages":[{"role":"alien","content":"x"}]}`,
		`{"model":"m","messages":[{"role":"user","content":[{"type":"input_audio","data":"x"}]}]}`,
		`{"model":"m","messages":[{"role":"tool","tool_call_id":"lost","content":"x"}]}`,
		`{"model":"m","messages":[{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"f","arguments":"broken"}}]}]}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"response_format":{"type":"json_object"}}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":-1}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"tool_choice":"required"}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"parallel_tool_calls":false}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"reasoning_effort":"off"}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"reasoning_effort":"none"}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"stream_options":{"include_usage":false}}`,
		`{"model":"m","messages":[{"role":"user","content":"x"}],"stream_options":{"vendor_option":true}}`,
	} {
		if _, err := GenerateBody([]byte(raw), Place{Directory: "/workspace"}); err == nil {
			t.Fatalf("silently accepted: %s", raw)
		}
	}
}

func TestGenerateStreamRequiresFinalFinishAndRejectsLateError(t *testing.T) {
	for name, raw := range map[string]string{
		"truncated":         "{\"type\":\"text-delta\",\"text\":\"partial\"}\n",
		"step-is-not-final": "{\"type\":\"finish-step\",\"finishReason\":\"stop\"}\n",
		"malformed":         "{\"type\":\"text-delta\",\"text\":\"partial\"}\nnot-json\n",
		"late-error":        "{\"type\":\"finish\",\"finishReason\":\"stop\"}\n{\"type\":\"error\",\"error\":\"late failure\"}\n",
		"pause":             "{\"type\":\"finish\",\"rawFinishReason\":\"pause_turn\",\"finishReason\":\"stop\"}\n",
	} {
		t.Run(name, func(t *testing.T) {
			var chunks []byte
			_, err := TranslateGenerate(strings.NewReader(raw), "model", true, func(body []byte) error {
				chunks = append(chunks, body...)
				return nil
			})
			if err == nil || strings.Contains(string(chunks), "[DONE]") || strings.Contains(string(chunks), `"finish_reason":"stop"`) {
				t.Fatalf("false successful terminal: %s %v", chunks, err)
			}
		})
	}
}

func TestGenerateStreamToolIdentityUsageAndNoDuplicate(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"start"}`,
		`{"type":"reasoning-delta","text":"think"}`,
		`{"type":"tool-input-start","id":"a","toolName":"read"}`,
		`{"type":"tool-input-delta","id":"a","delta":"{\"x\":1}"}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"read","input":{"x":1}}`,
		`{"type":"tool-call","toolCallId":"server","toolName":"private","providerExecuted":true,"input":{}}`,
		`{"type":"finish","finishReason":"tool-calls","totalUsage":{"inputTokens":3124,"outputTokens":7,"inputTokenDetails":{"noCacheTokens":52,"cacheReadTokens":3072}}}`,
	}, "\n")
	var chunks []byte
	result, err := TranslateGenerate(strings.NewReader(raw), "model", false, func(body []byte) error {
		chunks = append(chunks, body...)
		return nil
	})
	if err != nil || result.InputTokens != 3124 || result.CachedTokens != 3072 || result.OutputTokens != 7 {
		t.Fatalf("usage: %+v %v", result, err)
	}
	if strings.Count(string(chunks), `"id":"a"`) != 1 || strings.Contains(string(chunks), `"id":"server"`) || !strings.Contains(string(chunks), `"finish_reason":"tool_calls"`) {
		t.Fatalf("tool replay/duplication: %s", chunks)
	}
}

func TestNativeStructuredRefusalPreservesZDRTypeAndStatus(t *testing.T) {
	raw := `{"type":"error","error":{"statusCode":422,"type":"cmd_zdr_no_providers","code":"CMD_ZDR_NO_PROVIDERS","message":"No ZDR providers"}}`
	_, err := TranslateGenerate(strings.NewReader(raw), "m", true, func([]byte) error { t.Fatal("refusal emitted success"); return nil })
	failure, ok := err.(*NativeFailure)
	if !ok || failure.Status != 422 || failure.Type != "cmd_zdr_no_providers" || !failure.NotStarted {
		t.Fatalf("lost refusal: %#v", err)
	}
}
