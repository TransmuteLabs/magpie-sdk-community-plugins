package devin

import (
	"bytes"
	"strings"
	"testing"
)

func TestGeminiThoughtTextAndSplitToolsKeepActualTypeAndModel(t *testing.T) {
	for _, wire := range []string{"chat", "anthropic"} {
		for _, tools := range []bool{false, true} {
			t.Run(wire+map[bool]string{false: "/text", true: "/tools"}[tools], func(t *testing.T) {
				reply := GeminiReply{}
				deltas := []NativeDelta{{Text: "Hi there,"}, {Text: " friend!", Signature: "exact-signature",
					SignatureType: "gemini", ActualModelUID: "gemini-actual"}}
				if tools {
					deltas[1].Tools = []NativeToolDelta{{ID: "call-a", Name: "read", Arguments: `{"path":`}}
					deltas = append(deltas, NativeDelta{Tools: []NativeToolDelta{{Arguments: `"a"}`},
						{ID: "call-b", Name: "read", Arguments: `{"path":"b"}`}}})
				}
				for _, delta := range deltas {
					if err := reply.Add(delta); err != nil {
						t.Fatal(err)
					}
				}
				key, thought, err := reply.Thought("gemini-selected")
				if err != nil || thought.Format != "gemini" || thought.Model != "gemini-actual" {
					t.Fatal("actual signature origin was inferred or lost", thought, err)
				}
				history := `[{"role":"user","content":"hello"},{"role":"assistant","content":"Hi there, friend!"},{"role":"user","content":"again"}]`
				if tools {
					history = `[{"role":"user","content":"hello"},{"role":"assistant","content":"Hi there, friend!",
						"tool_calls":[{"id":"call-a","type":"function","function":{"name":"read","arguments":"{\"path\":\"a\"}"}},
						{"id":"call-b","type":"function","function":{"name":"read","arguments":"{\"path\":\"b\"}"}}]},
						{"role":"tool","tool_call_id":"call-a","content":"actual-a"},
						{"role":"tool","tool_call_id":"call-b","content":"actual-b"}]`
				}
				if wire == "anthropic" {
					history = `[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"Hi there, friend!"}]},
						{"role":"user","content":"again"}]`
					if tools {
						history = `[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"Hi there, friend!"},
							{"type":"tool_use","id":"call-a","name":"read","input":{"path":"a"}},
							{"type":"tool_use","id":"call-b","name":"read","input":{"path":"b"}}]},
							{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-a","content":"actual-a"},
							{"type":"tool_result","tool_use_id":"call-b","content":"actual-b"}]}]`
					}
				}
				body := []byte(`{"model":"gemini-selected","max_tokens":64,"messages":` + history + `}`)
				var encoded EncodedChat
				if wire == "chat" {
					encoded, err = EncodeChatRequest(body, "synthetic-key")
				} else {
					encoded, err = EncodeMessagesRequest(body, "synthetic-key", nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				before := bytes.Clone(encoded.Payload)
				restored, claims, err := RestoreGeminiThoughts(encoded, func(candidate string) (GeminiThought, bool) {
					return thought, key == candidate
				})
				if err != nil || len(claims) != 1 || claims[0].Format != "gemini" || claims[0].Model != "gemini-actual" ||
					claims[0].Digest != SignatureDigest([]byte("exact-signature")) {
					t.Fatal("text/split-tool continuation lost its exact origin", claims, err)
				}
				if !bytes.Equal(before, encoded.Payload) {
					t.Fatal("caller-owned encoded bytes were mutated")
				}
				fields, _ := readMessage(restored.Payload, nil)
				signed := 0
				for _, item := range fields[3] {
					prompt, _ := readMessage(item.b, nil)
					if prompt.text(12) == "" {
						continue
					}
					signed++
					if prompt.text(12) != "exact-signature" || prompt.text(18) != "gemini" ||
						prompt.text(22) != "gemini-actual" || prompt.text(11) != "" ||
						tools && len(prompt[6]) == 0 {
						t.Fatal("signature moved to a different block or invented thinking")
					}
				}
				if signed != 1 {
					t.Fatal("one reply produced multiple signatures", signed)
				}
				changed := bytes.ReplaceAll(body, []byte("Hi there, friend!"), []byte("Hithere,friend!"))
				if wire == "chat" {
					encoded, err = EncodeChatRequest(changed, "synthetic-key")
				} else {
					encoded, err = EncodeMessagesRequest(changed, "synthetic-key", nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				_, claims, err = RestoreGeminiThoughts(encoded, func(candidate string) (GeminiThought, bool) {
					return thought, candidate == key
				})
				if err != nil || len(claims) != 0 {
					t.Fatal("a whitespace-edited reply reused the original signature")
				}
			})
		}
	}
}

func TestGeminiThoughtRefusesGuessedTypeChangedOriginAndUnboundedState(t *testing.T) {
	for name, deltas := range map[string][]NativeDelta{
		"missing type":   {{Text: "answer", Signature: "signed"}},
		"foreign type":   {{Text: "answer", Signature: "signed", SignatureType: "anthropic"}},
		"changed type":   {{SignatureType: "gemini"}, {SignatureType: "other"}},
		"changed model":  {{ActualModelUID: "gemini-a"}, {ActualModelUID: "gemini-b"}},
		"model limit":    {{ActualModelUID: strings.Repeat("g", 513)}},
		"missing bytes":  {{Text: "answer", SignatureType: "gemini"}},
		"over retention": {{Text: strings.Repeat("a", 2<<20+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			reply := GeminiReply{}
			var err error
			for _, delta := range deltas {
				if err = reply.Add(delta); err != nil {
					break
				}
			}
			if err == nil {
				_, _, err = reply.Thought("gemini-selected")
			}
			if err == nil {
				t.Fatal("ambiguous signature state was accepted")
			}
		})
	}
}
