package devin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDonor043AdjacentThoughtsKeepLastOnRealAssistantPrompt(t *testing.T) {
	for _, following := range []string{
		`{"type":"text","text":"Found it."}`,
		`{"type":"text","text":""},{"type":"text","text":"Found it."}`,
		`{"type":"tool_use","id":"read-1","name":"read","input":{}}`,
	} {
		t.Run(following, func(t *testing.T) {
			tail := `{"role":"user","content":"Continue."}`
			if strings.Contains(following, "tool_use") {
				tail = `{"role":"user","content":[{"type":"tool_result","tool_use_id":"read-1","content":"real result"}]}`
			}
			body := []byte(`{"model":"claude-fixture","max_tokens":64,"messages":[{"role":"user","content":"Look it up."},
				{"role":"assistant","content":[{"type":"thinking","thinking":"Search first.","signature":"s1"},
				{"type":"thinking","thinking":"Use the result.","signature":"s2"},` + following + `]},` + tail + `]}`)
			var checked []string
			encoded, err := EncodeMessagesRequest(body, "selected-key", func(signature string) (NativeSignature, error) {
				checked = append(checked, signature)
				return NativeSignature{Format: "anthropic", Model: "claude-fixture"}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			fields, _ := readMessage(encoded.Payload, nil)
			assistants := 0
			for _, field := range fields[3] {
				prompt, _ := readMessage(field.b, nil)
				if prompt.number(2) != 2 {
					continue
				}
				assistants++
				if prompt.text(3) == "" && len(prompt[6]) == 0 {
					t.Fatal("signed thinking created an empty assistant prompt")
				}
				if prompt.text(11) != "Use the result." || prompt.text(12) != "s2" ||
					prompt.text(18) != "anthropic" || prompt.text(22) != "claude-fixture" {
					t.Fatal("last signed thought lost its text/signature/model", prompt)
				}
			}
			if assistants != 1 || strings.Join(checked, ",") != "s1,s2" {
				t.Fatal("adjacent thoughts changed prompt count or bypassed an origin check", assistants, checked)
			}
			denied := errors.New("foreign earlier signature")
			if _, err := EncodeMessagesRequest(body, "selected-key", func(signature string) (NativeSignature, error) {
				if signature == "s1" {
					return NativeSignature{}, denied
				}
				return NativeSignature{Format: "anthropic", Model: "claude-fixture"}, nil
			}); !errors.Is(err, denied) {
				t.Fatal("discarded earlier thought bypassed origin authority", err)
			}
		})
	}
}

func TestDonor044InlineImageTypesAndWhitespaceUseCommonEncoding(t *testing.T) {
	for _, fixture := range []struct {
		name string
		data []byte
		want string
	}{
		{"PNG", []byte{137, 80, 78, 71, 13, 10, 26, 10}, "image/png"},
		{"JPEG", []byte{255, 216, 255, 224}, "image/jpeg"},
		{"GIF", []byte("GIF89a"), "image/gif"},
		{"WebP", []byte("RIFF0000WEBP"), "image/webp"},
		{"unknown", []byte{1, 2, 3}, "image/jpeg"},
	} {
		for _, protocol := range []string{"chat", "messages"} {
			t.Run(fixture.name+"/"+protocol, func(t *testing.T) {
				data := base64.StdEncoding.EncodeToString(fixture.data)
				content := any([]any{map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + data}}})
				if protocol == "messages" {
					content = []any{map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": "image/jpeg", "data": data}}}
				}
				body, _ := json.Marshal(map[string]any{"model": "swe-fixture", "max_tokens": 64,
					"messages": []any{map[string]any{"role": "user", "content": content}}})
				encoded, err := EncodeChatRequest(body, "selected-key")
				if protocol == "messages" {
					encoded, err = EncodeMessagesRequest(body, "selected-key", nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				fields, _ := readMessage(encoded.Payload, nil)
				prompt, _ := readMessage(fields[3][0].b, nil)
				image, _ := readMessage(prompt[10][0].b, nil)
				if image.text(2) != fixture.want || image.text(1) != data {
					t.Fatal("inline image changed bytes or trusted the incorrect label", image.text(2), fixture.want)
				}
			})
		}
	}
	for _, protocol := range []string{"chat", "messages"} {
		t.Run("whitespace/"+protocol, func(t *testing.T) {
			body := []byte(`{"model":"swe-fixture","max_tokens":64,"messages":[{"role":"user","content":" hi "},
				{"role":"assistant","content":"hello"},{"role":"user","content":" \n\t"}]}`)
			encoded, err := EncodeChatRequest(body, "selected-key")
			if protocol == "messages" {
				encoded, err = EncodeMessagesRequest(body, "selected-key", nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			fields, _ := readMessage(encoded.Payload, nil)
			if len(fields[3]) != 2 {
				t.Fatal("whitespace-only user prompt was sent", len(fields[3]))
			}
			first, _ := readMessage(fields[3][0].b, nil)
			if first.text(3) != " hi " {
				t.Fatal("meaningful text was trimmed")
			}
		})
	}
}

func TestDonor044WhitespaceMarksAndUnattachedThinkingFailClosed(test *testing.T) {
	for _, body := range []string{
		`{"model":"claude-fixture","max_tokens":64,"messages":[{"role":"user","content":[{"type":"text","text":" \n","cache_control":{"type":"ephemeral"}}]}]}`,
		`{"model":"claude-fixture","max_tokens":64,"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"thinking","thinking":"alone","signature":"signed"}]}]}`,
	} {
		if _, err := EncodeMessagesRequest([]byte(body), "selected-key", func(string) (NativeSignature, error) {
			return NativeSignature{Format: "anthropic", Model: "claude-fixture"}, nil
		}); err == nil {
			test.Fatal("unsupported history silently lost a cache mark or invented an empty assistant prompt")
		}
	}
	chat := []byte(`{"model":"swe-fixture","messages":[{"role":"user","content":"hi"},{"role":"user","content":" ","cache_control":{"type":"ephemeral"}}]}`)
	if _, err := EncodeChatRequest(chat, "selected-key"); err == nil {
		test.Fatal("whitespace cache boundary was silently dropped")
	}
}

func TestDonor044ToolImageUsesCommonTypeCorrection(test *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	body := []byte(`{"model":"swe-fixture","max_tokens":64,"messages":[{"role":"user","content":"read"},
		{"role":"assistant","content":[{"type":"tool_use","id":"read-1","name":"read","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"read-1","content":[
		{"type":"image","source":{"type":"base64","media_type":"image/webp","data":"` + data + `"}}]}]}]}`)
	encoded, err := EncodeMessagesRequest(body, "selected-key", nil)
	if err != nil {
		test.Fatal(err)
	}
	fields, _ := readMessage(encoded.Payload, nil)
	result, _ := readMessage(fields[3][2].b, nil)
	image, _ := readMessage(result[10][0].b, nil)
	if result.number(2) != 4 || result.text(7) != "read-1" || image.text(1) != data || image.text(2) != "image/png" {
		test.Fatal("tool image lost association/bytes or retained its incorrect type")
	}
}

func TestMessagesBlocksKeepGrowingCachePrefixesAndRealResults(t *testing.T) {
	body := []byte(`{"model":"claude-fixture","max_tokens":128,"system":[
		{"type":"text","text":"system-A"},{"type":"text","text":"system-B","cache_control":{"type":"ephemeral"}}],
		"tools":[{"name":"read","input_schema":{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"read","disable_parallel_tool_use":true},
		"messages":[{"role":"user","content":[{"type":"text","text":"prefix-A"},{"type":"text","text":"prefix-B","cache_control":{"type":"ephemeral"}}]},
		{"role":"assistant","content":[{"type":"thinking","thinking":"signed thought","signature":"fixture-signature"},
			{"type":"text","text":"reading","cache_control":{"type":"ephemeral"}},
			{"type":"tool_use","id":"call-a","name":"read","input":{"path":"a"}}]},
		{"role":"system","content":[{"type":"text","text":"late notice","cache_control":{"type":"ephemeral"}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-a","content":"real result","cache_control":{"type":"ephemeral"}},
			{"type":"text","text":"continue"}]}]}`)
	before := bytes.Clone(body)
	result, err := EncodeMessagesRequest(body, "selected-key", func(signature string) (NativeSignature, error) {
		if signature != "fixture-signature" {
			t.Fatal(signature)
		}
		return NativeSignature{Format: "anthropic-bedrock", Model: "claude-actual"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(result.Payload, nil)
	if len(fields[3]) != 9 || fields.number(11) != 1 {
		t.Fatal("Messages blocks or parallel policy were lost", len(fields[3]))
	}
	wantText := []string{"system-A", "system-B", "prefix-A", "prefix-B", "reading", "", "real result", "late notice", "continue"}
	for index, part := range fields[3] {
		prompt, _ := readMessage(part.b, nil)
		if prompt.text(3) != wantText[index] {
			t.Fatalf("block %d changed order: %q", index, prompt.text(3))
		}
		marked := index == 1 || index == 3 || index == 4 || index == 6 || index == 7
		if (len(prompt[8]) != 0) != marked {
			t.Fatalf("cache mark moved at block %d", index)
		}
		if index == 4 && (prompt.text(11) != "signed thought" || prompt.text(12) != "fixture-signature" ||
			prompt.text(18) != "anthropic-bedrock" || prompt.text(22) != "claude-actual") {
			t.Fatal("signature format was guessed or discarded")
		}
		if index == 6 && (prompt.number(2) != 4 || prompt.text(7) != "call-a") {
			t.Fatal("real result was reassigned")
		}
	}
	tool, _ := readMessage(fields[10][0].b, nil)
	if !bytes.Contains(tool.data(3), []byte(`"required":["path"]`)) || !bytes.Equal(before, body) {
		t.Fatal("schema or original caller request changed")
	}
	var growing map[string]json.RawMessage
	_ = json.Unmarshal(body, &growing)
	var turns []json.RawMessage
	_ = json.Unmarshal(growing["messages"], &turns)
	turns = append(turns, json.RawMessage(`{"role":"assistant","content":"next"}`), json.RawMessage(`{"role":"user","content":"last"}`))
	growing["messages"], _ = json.Marshal(turns)
	next, _ := json.Marshal(growing)
	expanded, err := EncodeMessagesRequest(next, "selected-key", func(string) (NativeSignature, error) {
		return NativeSignature{Format: "anthropic-bedrock", Model: "claude-actual"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := readMessage(expanded.Payload, nil)
	for index := range fields[3] {
		firstFields, _ := RawFields(fields[3][index].b)
		nextFields, _ := RawFields(second[3][index].b)
		delete(firstFields, 1)
		delete(nextFields, 1)
		firstJSON, _ := json.Marshal(firstFields)
		nextJSON, _ := json.Marshal(nextFields)
		if !bytes.Equal(firstJSON, nextJSON) {
			t.Fatalf("growing history changed prefix block %d", index)
		}
	}
}

func TestMessagesRefuseUnknownControlsMissingResultsAndForeignSignatures(t *testing.T) {
	for name, body := range map[string]string{
		"unknown control": `{"thinking":{"type":"enabled","budget_tokens":100,"unknown":true},"messages":[{"role":"user","content":"hi"}]}`,
		"block fields":    `{"messages":[{"role":"user","content":[{"type":"text","text":"hi","signature":"foreign"}]}]}`,
		"null content":    `{"messages":[{"role":"user","content":null}]}`,
		"null system":     `{"system":null,"messages":[{"role":"user","content":"hi"}]}`,
		"cache lifetime":  `{"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
		"missing result":  `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"read","input":{}}]},{"role":"user","content":"next"}]}`,
		"orphan result":   `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"invented"}]}]}`,
		"server tool":     `{"tools":[{"type":"web_search_20250305","name":"search","input_schema":{}}],"messages":[{"role":"user","content":"hi"}]}`,
		"unsigned type":   `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"secret","signature":"foreign"}]}]}`,
		"remote image":    `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/private"}}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var request map[string]json.RawMessage
			if json.Unmarshal([]byte(body), &request) != nil {
				t.Fatal("invalid test fixture")
			}
			request["model"], request["max_tokens"] = json.RawMessage(`"claude-fixture"`), json.RawMessage(`64`)
			data, _ := json.Marshal(request)
			if _, err := EncodeMessagesRequest(data, "selected-key", nil); err == nil {
				t.Fatal("lossy Messages request was accepted")
			}
		})
	}
	signed := []byte(`{"model":"claude-fixture","max_tokens":64,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"secret","signature":"foreign"}]}]}`)
	sentinel := errors.New("no scoped origin")
	if _, err := EncodeMessagesRequest(signed, "selected-key", func(string) (NativeSignature, error) { return NativeSignature{}, sentinel }); !errors.Is(err, sentinel) {
		t.Fatal("signature origin refusal was swallowed", err)
	}
}

func TestMessagesImagesPreserveTextImageTextOrder(t *testing.T) {
	body := []byte(`{"model":"swe-fixture","max_tokens":64,"messages":[{"role":"user","content":[
		{"type":"text","text":"before"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"},"cache_control":{"type":"ephemeral"}},
		{"type":"text","text":"after"}]}]}`)
	result, err := EncodeMessagesRequest(body, "selected-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(result.Payload, nil)
	if len(fields[3]) != 3 {
		t.Fatal("Messages text/image boundary was flattened")
	}
	middle, _ := readMessage(fields[3][1].b, nil)
	if len(middle[10]) != 1 || len(middle[8]) == 0 {
		t.Fatal("image bytes or cache boundary were lost")
	}
}
