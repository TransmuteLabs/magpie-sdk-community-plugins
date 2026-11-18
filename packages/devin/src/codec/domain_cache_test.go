package devin

import (
	"bytes"
	"testing"
)

func TestDomainChatBlockCacheMarksPreserveGrowingPrefixes(t *testing.T) {
	for _, tail := range []string{"", `,{"type":"text","text":"later"}`} {
		body := []byte(`{"model":"swe-2-high","messages":[{"role":"system","content":[
			{"type":"text","text":"system-A"},{"type":"text","text":"system-B","cache_control":{"type":"ephemeral","ttl":"5m"}}]},
			{"role":"user","content":[{"type":"text","text":"user-A","cache_control":{"type":"ephemeral"}},{"type":"text","text":"user-B"}` + tail + `]}]}`)
		before := bytes.Clone(body)
		encoded, err := EncodeChatRequest(body, "synthetic-key")
		if err != nil {
			t.Fatalf("representable Chat block cache mark was refused: %v", err)
		}
		fields, err := readMessage(encoded.Payload, nil)
		wantPrompts := 4
		if tail != "" {
			wantPrompts++
		}
		if err != nil || len(fields[3]) != wantPrompts {
			t.Fatal("Chat blocks were collapsed or added", err, len(fields[3]))
		}
		for index, text := range []string{"system-A", "system-B", "user-A", "user-B"} {
			prompt, _ := readMessage(fields[3][index].b, nil)
			if prompt.text(3) != text || (len(prompt[8]) != 0) != (index == 1 || index == 2) {
				t.Fatalf("cache prefix boundary moved at %d: %q", index, prompt.text(3))
			}
		}
		if !bytes.Equal(body, before) {
			t.Fatal("cache normalization mutated caller input")
		}
	}
}

func TestDomainMessagesFinalResultCacheMarkKeepsOneRealResult(t *testing.T) {
	body := []byte(`{"model":"swe-2-high","max_tokens":64,"messages":[
		{"role":"assistant","content":[{"type":"tool_use","id":"call-a","name":"read","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-a","content":[
			{"type":"text","text":"first "},{"type":"text","text":"last","cache_control":{"type":"ephemeral","ttl":"5m"}}]},
			{"type":"text","text":"continue"}]}]}`)
	encoded, err := EncodeMessagesRequest(body, "synthetic-key", nil)
	if err != nil {
		t.Fatalf("cache at the real result's final boundary was refused: %v", err)
	}
	fields, _ := readMessage(encoded.Payload, nil)
	if len(fields[3]) != 3 {
		t.Fatal("one tool result was split or manufactured", len(fields[3]))
	}
	result, _ := readMessage(fields[3][1].b, nil)
	if result.number(2) != 4 || result.text(7) != "call-a" || result.text(3) != "first last" || len(result[8]) != 1 {
		t.Fatal("cache mark or actual result association changed", result)
	}
}

func TestDomainCacheControlsRefuseUnrepresentableBoundaries(t *testing.T) {
	for name, body := range map[string]string{
		"ttl":             `{"messages":[{"role":"user","content":[{"type":"text","text":"x","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
		"tool-definition": `{"tools":[{"name":"read","input_schema":{},"cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"x"}]}`,
		"inside-result":   `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":[{"type":"text","text":"first","cache_control":{"type":"ephemeral"}},{"type":"text","text":"last"}]}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			body = `{"model":"swe-2-high","max_tokens":64,` + body[1:]
			if result, err := EncodeMessagesRequest([]byte(body), "synthetic-key", nil); err == nil || len(result.Payload) != 0 {
				t.Fatal("unrepresentable cache control was discarded or moved")
			}
		})
	}
}
