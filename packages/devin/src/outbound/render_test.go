package outbound

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func TestCanonicalRendererKeepsToolsThinkingSignatureAndExactUsage(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		writer := httptest.NewRecorder()
		renderer := NewChatRenderer(writer, "model", streaming)
		for _, delta := range []codec.NativeDelta{
			{Text: "answer", Thinking: "thinking", Signature: "sig", SignatureType: "format"},
			{Tools: []codec.NativeToolDelta{{ID: "native id", Name: "read_file", Arguments: `{"file":`}}},
			{Tools: []codec.NativeToolDelta{{ID: "native id", Arguments: `"a"}`}}},
		} {
			if err := renderer.Delta(delta); err != nil {
				t.Fatal(err)
			}
		}
		if err := renderer.Finish(codec.NativeSummary{Stop: 10, Usage: &codec.NativeUsage{Input: 3, Output: 0, CacheRead: 4, CacheWrite: 5}}); err != nil {
			t.Fatal(err)
		}
		body := writer.Body.String()
		if !strings.Contains(body, "answer") || !strings.Contains(body, "thinking") || !strings.Contains(body, "signature") ||
			!strings.Contains(body, `"completion_tokens":0`) || !strings.Contains(body, `"prompt_tokens":12`) {
			t.Fatal(body)
		}
		if streaming && !strings.Contains(body, "[DONE]") {
			t.Fatal("missing stream terminal")
		}
		if !streaming {
			var value struct {
				ID      string `json:"id"`
				Choices []struct {
					Message struct {
						Calls []struct {
							ID       string `json:"id"`
							Function struct {
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"message"`
				} `json:"choices"`
			}
			if json.Unmarshal(writer.Body.Bytes(), &value) != nil || len(value.Choices) != 1 || len(value.Choices[0].Message.Calls) != 1 ||
				value.Choices[0].Message.Calls[0].ID != codec.CanonicalToolID("native id") ||
				value.Choices[0].Message.Calls[0].Function.Arguments != `{"file":"a"}` {
				t.Fatal(body)
			}
			if value.ID == NewChatRenderer(httptest.NewRecorder(), "model", false).id {
				t.Fatal("response IDs are reused")
			}
		}
	}
}

func TestCanonicalRendererNeverFabricatesCompletionForUnrepresentableNativeState(t *testing.T) {
	for _, streaming := range []bool{true, false} {
		writer := httptest.NewRecorder()
		renderer := NewChatRenderer(writer, "model", streaming)
		if err := renderer.Delta(codec.NativeDelta{Phase: "native-phase"}); err == nil {
			t.Fatal("native phase was dropped")
		}
		if err := renderer.Finish(codec.NativeSummary{Stop: 11}); err == nil {
			t.Fatal("native stop was relabeled")
		}
		if writer.Body.Len() != 0 {
			t.Fatal("failure emitted a success")
		}
		if err := renderer.Fail(429); err != nil {
			t.Fatal(err)
		}
		if writer.Code != 429 || strings.Contains(writer.Body.String(), "[DONE]") {
			t.Fatal("failure became a successful completion")
		}
	}
}
