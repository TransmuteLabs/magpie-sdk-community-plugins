package devin

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDonorSplitAssistantCallsKeepRealResultsAndUnsignedReasoning(t *testing.T) {
	body := []byte(`{"model":"swe-2-high","messages":[
		{"role":"user","content":"read both"},
		{"role":"assistant","content":null,"reasoning":"First a.","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]},
		{"role":"assistant","content":"reading","reasoning_content":"Then b.","tool_calls":[{"id":"b","type":"function","function":{"name":"read","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"a","content":"real A"},
		{"role":"tool","tool_call_id":"b","content":"real B"}]}`)
	before := bytes.Clone(body)
	request, err := EncodeChatRequest(body, "selected-key")
	if err != nil {
		t.Fatal("valid split tool turn was falsely refused", err)
	}
	fields, _ := readMessage(request.Payload, nil)
	if len(fields[3]) != 4 {
		t.Fatal("split assistant call fragments were not one native turn")
	}
	assistant, _ := readMessage(fields[3][1].b, nil)
	if assistant.text(3) != "reading" || assistant.text(11) != "First a.\n\nThen b." || len(assistant[6]) != 2 {
		t.Fatal("split calls or their reasoning were lost")
	}
	first, _ := readMessage(fields[3][2].b, nil)
	second, _ := readMessage(fields[3][3].b, nil)
	if first.text(7) != "a" || first.text(3) != "real A" || second.text(7) != "b" || second.text(3) != "real B" ||
		!bytes.Equal(before, body) {
		t.Fatal("real tool results changed or caller input was mutated")
	}
	var incomplete map[string]json.RawMessage
	_ = json.Unmarshal(body, &incomplete)
	var messages []json.RawMessage
	_ = json.Unmarshal(incomplete["messages"], &messages)
	incomplete["messages"], _ = json.Marshal(messages[:len(messages)-1])
	missing, _ := json.Marshal(incomplete)
	if _, err := EncodeChatRequest(missing, "selected-key"); err == nil {
		t.Fatal("missing tool result was manufactured instead of refused")
	}
}

func TestDonorClaudeUnsignedReasoningAndConflictingAliases(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hello"},
		{"role":"assistant","content":"answer","reasoning_content":"unsigned secret"},
		{"role":"user","content":"next"}]}`)
	result, err := EncodeChatRequest(body, "selected-key")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(result.Payload, nil)
	assistant, _ := readMessage(fields[3][1].b, nil)
	if assistant.text(11) != "" || assistant.text(12) != "" || assistant.text(18) != "" {
		t.Fatal("Claude received unsigned thinking or a fabricated signature")
	}
	conflict := []byte(`{"model":"swe","messages":[{"role":"assistant","content":"answer","reasoning":"one","reasoning_content":"two"}]}`)
	if _, err := EncodeChatRequest(conflict, "selected-key"); err == nil {
		t.Fatal("conflicting reasoning aliases were silently selected")
	}
}
