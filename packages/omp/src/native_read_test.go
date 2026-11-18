package ompsessions

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeClaudeUsesLatestRevisionAndActiveBranch(test *testing.T) {
	body := []byte("{\"type\":\"user\",\"uuid\":\"u\",\"parentUuid\":null,\"message\":{\"content\":\"original\"}}\n" +
		"{\"type\":\"assistant\",\"uuid\":\"a\",\"parentUuid\":\"u\",\"message\":{\"content\":\"partial\"}}\n" +
		"{\"type\":\"assistant\",\"uuid\":\"a\",\"parentUuid\":\"u\",\"message\":{\"content\":\"final answer\"}}\n" +
		"{\"type\":\"user\",\"uuid\":\"side\",\"parentUuid\":\"u\",\"isSidechain\":true,\"message\":{\"content\":\"private side branch\"}}\n")
	items, err := readClaudeNative(test.Context(), body)
	if err != nil || len(items) != 2 || items[1].Text != "final answer" {
		test.Fatalf("native Claude replay selected a stale revision or side branch: %+v %v", items, err)
	}
	cyclic := bytes.ReplaceAll(body, []byte(`"parentUuid":"u"`), []byte(`"parentUuid":"a"`))
	if _, err := readClaudeNative(test.Context(), cyclic); err == nil {
		test.Fatal("cyclic native history was accepted")
	}
}

func TestNativeCodexReplacementAndLocalSummaryFollowReplay(test *testing.T) {
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"old context"}]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"private"}}`,
		`{"type":"compacted","payload":{"replacement_history":[{"type":"message","role":"user","content":[{"type":"input_text","text":"selected summary"}]}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"retained answer"}]}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"retained answer"}}`,
	}
	items, err := readCodexNative(test.Context(), []byte(strings.Join(lines, "\n")))
	if err != nil || len(items) != 2 || items[0].Content[0].Text != "selected summary" || items[1].Text != "retained answer" {
		test.Fatalf("native Codex replay duplicated events or ignored compaction: %+v %v", items, err)
	}
	result, err := codexNativeItems(codexSourceItem{Type: "function_call_output", CallID: "call", Output: json.RawMessage(`{"unknown":"opaque metadata"}`)}, fixtureTime)
	if err != nil || len(result) != 1 || result[0].Text != `{"unknown":"opaque metadata"}` {
		test.Fatal("opaque native tool output was silently dropped")
	}
}

func TestNativeReadersNeverFetchExternalImages(test *testing.T) {
	claude := []byte(`{"type":"user","uuid":"u","message":{"content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/private"}}]}}`)
	if _, err := readClaudeNative(test.Context(), claude); err == nil {
		test.Fatal("native Claude URL image was accepted without explicit acquisition")
	}
	codex := []byte(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/private"}]}}`)
	if _, err := readCodexNative(test.Context(), codex); err == nil {
		test.Fatal("native Codex URL image was accepted without explicit acquisition")
	}
}

func TestNativeCodexPreservesUserMarkup(test *testing.T) {
	body := []byte(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<div>legitimate source code</div>"}]}}`)
	items, err := readCodexNative(test.Context(), body)
	if err != nil || len(items) != 1 || items[0].Content[0].Text != "<div>legitimate source code</div>" {
		test.Fatal("native migration discarded legitimate user markup")
	}
	injected := bytes.ReplaceAll(body, []byte("<div>legitimate source code</div>"), []byte("<environment_context>generated context"))
	items, err = readCodexNative(test.Context(), injected)
	if err != nil || len(items) != 0 {
		test.Fatal("native migration retained known client-injected context")
	}
}
