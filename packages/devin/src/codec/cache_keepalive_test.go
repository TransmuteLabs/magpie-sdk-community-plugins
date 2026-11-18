package devin

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestCacheKeepaliveTemplatePreservesInterleavedBytesAndFreshAuthority(t *testing.T) {
	requests := []struct {
		name    string
		message bool
		body    string
	}{
		{"chat", false, `{"model":"swe-2","max_tokens":400,"temperature":0.2,"messages":[{"role":"system","content":"instructions"},{"role":"user","content":"read a","cache_control":{"type":"ephemeral"}},{"role":"assistant","reasoning_content":"thought","content":null,"tool_calls":[{"id":"call","type":"function","function":{"name":"Read","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call","content":"A","cache_control":{"type":"ephemeral"}}],"tools":[{"type":"function","function":{"name":"Read","parameters":{"type":"object"}}}]}`},
		{"messages", true, `{"model":"claude-test","max_tokens":400,"system":[{"type":"text","text":"instructions","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"read a"},{"role":"assistant","content":[{"type":"thinking","thinking":"thought","signature":"opaque-signature"},{"type":"tool_use","id":"call","name":"Read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":"A","cache_control":{"type":"ephemeral"}}]}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`},
	}
	for _, test := range requests {
		t.Run(test.name, func(t *testing.T) {
			var encoded EncodedChat
			var err error
			if test.message {
				encoded, err = EncodeMessagesRequest([]byte(test.body), "original-secret", func(string) (NativeSignature, error) {
					return NativeSignature{Format: "anthropic-test", Model: "claude-test"}, nil
				})
			} else {
				encoded, err = EncodeChatRequest([]byte(test.body), "original-secret")
			}
			if err != nil {
				t.Fatal(err)
			}
			original, _ := orderedChunks(encoded.Payload)
			var interleaved []byte
			for _, chunk := range original {
				if chunk.number != 3 {
					interleaved = append(interleaved, chunk.raw...)
				}
			}
			for _, chunk := range original {
				if chunk.number == 3 {
					interleaved = append(interleaved, chunk.raw...)
					interleaved = append(interleaved, addText(nil, 10, string(join(fixtureText(1, "Unused"), fixtureText(3, `{"type":"object"}`))))...)
				}
			}
			before := bytes.Clone(interleaved)
			template, err := NewCacheKeepaliveTemplate(interleaved)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(template.payload, []byte("original-secret")) || !bytes.Equal(interleaved, before) {
				t.Fatal("retained account credentials or changed the source")
			}
			first, err := template.Payload("fresh-secret")
			if err != nil {
				t.Fatal(err)
			}
			second, err := template.Payload("fresh-secret")
			if err != nil || !bytes.Equal(first, second) {
				t.Fatal("each ping changed retained message IDs or added another continue")
			}
			was, _ := orderedChunks(interleaved)
			now, _ := orderedChunks(first)
			var sameBefore, sameAfter [][]byte
			beforePrompts, afterPrompts := 0, 0
			for _, chunk := range was {
				if chunk.number == 3 {
					beforePrompts++
				}
				if chunk.number != 1 && chunk.number != 8 {
					sameBefore = append(sameBefore, chunk.raw)
				}
			}
			for _, chunk := range now {
				if chunk.number == 3 {
					afterPrompts++
					prompt, _ := readMessage(chunk.value, nil)
					if prompt.text(3) == "continue" {
						if prompt.number(2) != 1 || prompt.text(1) == "" || len(prompt) != 3 {
							t.Fatal("continue gained a cache mark or lost its new user identity")
						}
						continue
					}
				}
				if chunk.number != 1 && chunk.number != 8 {
					sameAfter = append(sameAfter, chunk.raw)
				}
			}
			if !bytes.Equal(bytes.Join(sameBefore, nil), bytes.Join(sameAfter, nil)) || afterPrompts != beforePrompts+1 {
				t.Fatal("raw field order, prompt, signature, tools or cache bytes changed")
			}
			beforeFields, _ := readMessage(interleaved, nil)
			afterFields, _ := readMessage(first, nil)
			if bytes.Contains(first, []byte("original-secret")) || !bytes.Contains(afterFields.data(1), []byte("fresh-secret")) {
				t.Fatal("fresh account metadata missing")
			}
			configuration, _ := readMessage(afterFields.data(8), nil)
			if configuration.number(2) != 1 {
				t.Fatal("ping did not limit output to one token")
			}
			compareSampling := func(payload []byte) []byte {
				chunks, _ := orderedChunks(payload)
				var result []byte
				for _, chunk := range chunks {
					if chunk.number != 2 {
						result = append(result, chunk.raw...)
					}
				}
				return result
			}
			if !bytes.Equal(compareSampling(beforeFields.data(8)), compareSampling(afterFields.data(8))) {
				t.Fatal("sampling changed outside max_tokens")
			}
			first[0] ^= 1
			third, err := template.Payload("another-secret")
			if err != nil || bytes.Equal(first, third) || bytes.Contains(template.payload, []byte("fresh-secret")) {
				t.Fatal("payload mutation escaped into the immutable credential-free template")
			}
		})
	}
}

func TestCacheKeepaliveTemplateRefusesUnknownDuplicateAndAuthorityControls(t *testing.T) {
	base, err := EncodeChatRequest([]byte(`{"model":"test","messages":[{"role":"user","content":"hi"}]}`), "secret")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(base.Payload, nil)
	for name, payload := range map[string][]byte{
		"unknown top":        append(bytes.Clone(base.Payload), fixtureNumber(99, 1)...),
		"duplicate model":    append(bytes.Clone(base.Payload), fixtureText(21, "another")...),
		"duplicate metadata": append(bytes.Clone(base.Payload), fixtureText(1, "")...),
		"assignment token":   append(bytes.Clone(base.Payload), fixtureText(26, "credential-bearing-token")...),
		"truncated":          base.Payload[:len(base.Payload)-1],
		"unknown sampling":   join(fixtureText(21, "test"), fixtureText(3, string(fields.data(3))), fixtureText(8, string(join(fields.data(8), fixtureNumber(99, 1))))),
		"duplicate max":      join(fixtureText(21, "test"), fixtureText(3, string(fields.data(3))), fixtureText(8, string(join(fields.data(8), fixtureNumber(2, 7))))),
		"unknown prompt":     join(fixtureText(21, "test"), fixtureText(3, string(join(fields.data(3), fixtureNumber(99, 1)))), fixtureText(8, string(fields.data(8)))),
		"no max":             join(fixtureText(21, "test"), fixtureText(3, string(fields.data(3))), fixtureText(8, string(fixtureNumber(1, 1)))),
		"empty":              nil,
		"oversized":          make([]byte, maxBody+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewCacheKeepaliveTemplate(payload); err == nil {
				t.Fatal("unsupported keepalive semantics were admitted")
			}
		})
	}
	var empty CacheKeepaliveTemplate
	if _, err := empty.Payload("key"); err == nil {
		t.Fatal("empty template could dispatch")
	}
	template, err := NewCacheKeepaliveTemplate(base.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", " spaced ", "bad\nkey"} {
		if _, err := template.Payload(key); err == nil {
			t.Fatal("invalid fresh credentials were admitted")
		}
	}
}

func TestCacheKeepaliveTemplatePreservesNoncanonicalVarintChunks(t *testing.T) {
	sampling := join([]byte{8, 0x81, 0}, fixtureNumber(2, 500))
	prompt := join(fixtureText(1, "original"), []byte{16, 0x81, 0}, fixtureText(3, "hello"))
	payload := join(fixtureText(21, "test"), fixtureText(3, string(prompt)), fixtureText(8, string(sampling)))
	template, err := NewCacheKeepaliveTemplate(payload)
	if err != nil {
		t.Fatal(err)
	}
	ping, err := template.Payload("fresh")
	if err != nil || !bytes.Contains(ping, prompt) {
		t.Fatal("original prompt varint was re-encoded")
	}
	fields, _ := readMessage(ping, nil)
	if !bytes.Equal(fields.data(8), join([]byte{8, 0x81, 0}, fixtureNumber(2, 1))) {
		t.Fatal("unchanged sampling chunks were canonicalized")
	}
	wire, compressed, err := EncodeConnectRequest(ping)
	if err != nil || compressed || binary.BigEndian.Uint32(wire[1:5]) != uint32(len(ping)) {
		t.Fatal("ordinary bounded Connect framing failed")
	}
}

func TestCacheKeepaliveTemplateNeverRetainsCredentialsInsideContent(t *testing.T) {
	encoded, err := EncodeChatRequest([]byte(
		`{"model":"test","messages":[{"role":"user","content":"original-account-secret"}]}`), "original-account-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCacheKeepaliveTemplate(encoded.Payload); err == nil {
		t.Fatal("removing metadata retained account credentials in prompt content")
	}
	encoded, err = EncodeChatRequest([]byte(
		`{"model":"test","messages":[{"role":"user","content":"future-account-secret"}]}`), "original-account-secret")
	if err != nil {
		t.Fatal(err)
	}
	template, err := NewCacheKeepaliveTemplate(encoded.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := template.Payload("future-account-secret"); err == nil {
		t.Fatal("current account credentials were present in retained template content")
	}
}
