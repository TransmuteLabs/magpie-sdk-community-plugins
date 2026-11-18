package devin

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestOutboundRequestPreservesCanonicalSemantics(t *testing.T) {
	key := "synthetic-outbound-key"
	body := []byte(`{"model":"swe-2-high","stream":true,"messages":[
		{"role":"system","content":"system","cache_control":{"type":"ephemeral"}},
		{"role":"user","content":"inspect"},
		{"role":"assistant","content":null,"reasoning_content":"thinking","tool_calls":[{"id":"call-1","type":"function","function":{"name":"read","arguments":"{\"path\":\"a\"}"}}]},
		{"role":"tool","tool_call_id":"call-1","content":[{"type":"text","text":"result"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="}}]}],
		"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"},"strict":true}}],
		"tool_choice":{"type":"function","function":{"name":"read"}},"parallel_tool_calls":false,
		"max_tokens":1234,"temperature":0,"top_p":0,"stop":["stop1","stop2"],"seed":42,"prompt_cache_key":"cache"}`)
	result, err := EncodeChatRequest(body, key)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stream || result.Model != "swe-2-high" {
		t.Fatal("canonical model or stream mode changed")
	}
	fields, err := readMessage(result.Payload, nil)
	if err != nil || fields.number(7) != 5 || fields.text(21) != "swe-2-high" || len(fields[3]) != 4 || fields.text(27) != "cache" {
		t.Fatal("native request fields differ:", err)
	}
	metadata, err := readMessage(fields.data(1), nil)
	if err != nil || metadata.text(3) != key || metadata.text(12) != "chisel" || metadata.text(28) != "chisel" {
		t.Fatal("credential metadata differed")
	}
	configuration, err := readMessage(fields.data(8), nil)
	if err != nil || configuration.number(2) != 1234 || configuration.number(3) != 400 || configuration.number(7) != 40 ||
		!bytes.Equal(configuration.data(5), make([]byte, 8)) || !bytes.Equal(configuration.data(8), make([]byte, 8)) {
		t.Fatal("caller zero sampling or native defaults changed")
	}
	for index, native := range fields[3] {
		prompt, err := decodePrompt(native.b)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 && prompt["role"] != "system" || index == 2 && prompt["reasoning_content"] != "thinking" ||
			index == 3 && prompt["tool_call_id"] != "call-1" {
			t.Fatal("history changed")
		}
	}
	if !bytes.Contains(fields[3][3].b, []byte("aW1hZ2U=")) || bytes.Contains(fields[3][3].b, []byte("no output")) {
		t.Fatal("tool-result image lost or output fabricated")
	}
}

func TestOutboundRefusesLossyCanonicalInputs(t *testing.T) {
	for name, body := range map[string]string{
		"unknown control":     `{"model":"swe","messages":[{"role":"user","content":"x"}],"response_format":{"type":"json_object"}}`,
		"signed history":      `{"model":"swe","messages":[{"role":"assistant","content":"x","reasoning_details":[{"type":"reasoning.text","text":"x","signature":"secret"}]}]}`,
		"missing tool result": `{"model":"swe","messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}}]}]}`,
		"orphan result":       `{"model":"swe","messages":[{"role":"tool","tool_call_id":"c","content":"x"}]}`,
		"remote image":        `{"model":"swe","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.invalid/a.png"}}]}]}`,
		"ordered content":     `{"model":"swe","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="}},{"type":"text","text":"after"}]}]}`,
		"late system":         `{"model":"swe","messages":[{"role":"user","content":"x"},{"role":"system","content":"late"}]}`,
		"sampling":            `{"model":"swe","messages":[{"role":"user","content":"x"}],"top_p":2}`,
		"two completions":     `{"model":"swe","messages":[{"role":"user","content":"x"}],"n":2}`,
		"conflict":            `{"model":"swe","messages":[{"role":"user","content":"x"}],"max_tokens":1,"max_completion_tokens":2}`,
		"duplicate":           `{"model":"a","model":"swe","messages":[{"role":"user","content":"x"}]}`,
		"negative seed":       `{"model":"swe","messages":[{"role":"user","content":"x"}],"seed":-1}`,
		"unknown metadata":    `{"model":"swe","messages":[{"role":"user","content":"x"}],"metadata":{"credential":"private"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			value, err := EncodeChatRequest([]byte(body), "private-credential")
			if err == nil || len(value.Payload) != 0 || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("lossy request accepted or private data disclosed: %v", err)
			}
		})
	}
	for _, key := range []string{"", "key\nprivate", " key", "ключ"} {
		if _, err := NativeMetadata(key); err == nil {
			t.Fatal("invalid credential accepted")
		}
	}
}

func TestOutboundToolIDReversibleWithoutCollision(t *testing.T) {
	for _, value := range []string{"plain-id", "Bash:0#abc", "dv_YmFzZQ", "utf8-имя"} {
		encoded := CanonicalToolID(value)
		if NativeToolID(encoded) != value {
			t.Fatal("native tool ID roundtrip failed")
		}
	}
	if NativeToolID("dv_%%%") != "dv_%%%" {
		t.Fatal("malformed external tool id was rewritten")
	}
}

func nativeStreamFixture(parts ...[]byte) []byte {
	var body []byte
	for _, part := range parts {
		body = append(body, fixtureFrame(0, part)...)
	}
	return append(body, fixtureFrame(2, []byte(`{"metadata":{}}`))...)
}

func TestOutboundNativeStreamToolsSignaturesUsage(t *testing.T) {
	first := join(fixtureText(3, "answer"), fixtureText(9, "thinking"),
		fixtureText(10, "signature"), fixtureText(21, "anthropic"),
		fixtureText(23, "actual-model"), fixtureText(25, "analysis"),
		fixtureText(6, string(join(fixtureText(1, "Bash:0#id"), fixtureText(2, "read"), fixtureText(3, `{"path":`)))))
	last := join(fixtureText(6, string(fixtureText(3, `"a"}`))), fixtureNumber(5, 10),
		fixtureText(7, string(join(fixtureNumber(2, 7), fixtureNumber(3, 0), fixtureNumber(4, 2), fixtureNumber(5, 3)))))
	var events []NativeDelta
	summary, err := ReadNativeStream(context.Background(), bytes.NewReader(nativeStreamFixture(first, last)), false,
		func(delta NativeDelta) error { events = append(events, delta); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Signature != "signature" || events[0].SignatureType != "anthropic" ||
		events[0].ActualModelUID != "actual-model" || events[0].Phase != "analysis" || !bytes.Equal(events[0].Raw, first) {
		t.Fatal("native provenance or content lost")
	}
	if summary.Stop != 10 || summary.Usage == nil || *summary.Usage != (NativeUsage{Input: 7, Output: 0, CacheWrite: 2, CacheRead: 3}) {
		t.Fatalf("native zero tokens were estimated or usage changed: %+v", summary)
	}
}

func TestOutboundConnectGzipAndBoundaries(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), 8193)
	framed, compressed, err := EncodeConnectRequest(payload)
	if err != nil || !compressed {
		t.Fatal("large native request not compressed")
	}
	_, decoded, err := ReadConnectFrame(bytes.NewReader(framed), true)
	if err != nil || !bytes.Equal(payload, decoded) {
		t.Fatal("native gzip roundtrip failed")
	}
	if _, _, err := ReadConnectFrame(bytes.NewReader(framed), false); err == nil {
		t.Fatal("gzip accepted without negotiation")
	}
	var zip bytes.Buffer
	writer := gzip.NewWriter(&zip)
	_, _ = writer.Write([]byte("x"))
	_ = writer.Close()
	for name, body := range map[string][]byte{
		"short header": {0, 0},
		"truncated":    fixtureFrame(0, []byte("x"))[:5],
		"flags":        fixtureFrame(4, nil),
		"gzip members": fixtureFrame(1, append(bytes.Clone(zip.Bytes()), zip.Bytes()...)),
		"gzip tail":    fixtureFrame(1, append(bytes.Clone(zip.Bytes()), 0)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ReadConnectFrame(bytes.NewReader(body), true); err == nil {
				t.Fatal("malformed Connect frame accepted")
			}
		})
	}
}

func TestOutboundNativeStreamRefusesTruncationAndControlLoss(t *testing.T) {
	text := join(fixtureText(3, "text"), fixtureNumber(5, 2))
	for name, body := range map[string][]byte{
		"missing terminal":        fixtureFrame(0, text),
		"missing stop":            nativeStreamFixture(fixtureText(3, "text")),
		"incomplete":              nativeStreamFixture(join(fixtureText(3, "text"), fixtureNumber(5, 1))),
		"trailing frame":          append(nativeStreamFixture(text), fixtureFrame(0, text)...),
		"content filter":          nativeStreamFixture(join(fixtureText(3, "text"), fixtureNumber(5, 11))),
		"redacted":                nativeStreamFixture(join(fixtureText(3, "text"), fixtureNumber(8, 1), fixtureNumber(5, 2))),
		"opaque Gemini signature": nativeStreamFixture(join(fixtureText(3, "text"), fixtureText(20, "\x00opaque"), fixtureNumber(5, 2))),
		"model change":            nativeStreamFixture(join(fixtureText(3, "text"), fixtureText(30, ""), fixtureNumber(5, 2))),
		"unknown response":        nativeStreamFixture(join(fixtureText(3, "text"), fixtureNumber(31, 1), fixtureNumber(5, 2))),
		"empty tools":             nativeStreamFixture(join(fixtureText(6, ""), fixtureNumber(5, 10))),
		"incomplete tool json":    nativeStreamFixture(join(fixtureText(6, string(join(fixtureText(1, "c"), fixtureText(2, "f"), fixtureText(3, "{")))), fixtureNumber(5, 10))),
		"data after stop":         nativeStreamFixture(text, fixtureText(3, "later")),
		"vendor secret error":     fixtureFrame(2, []byte(`{"error":{"code":"PRIVATE-secret","message":"PRIVATE-secret"}}`)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ReadNativeStream(context.Background(), bytes.NewReader(body), false, nil)
			if err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("native failure was hidden or disclosed vendor text: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadNativeStream(ctx, bytes.NewReader(nil), false, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("native cancellation identity lost")
	}
	if _, err := ReadNativeStream(context.Background(), bytes.NewReader(nativeStreamFixture(text)), false,
		func(NativeDelta) error { return io.ErrClosedPipe }); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("native consumer failure ignored")
	}
}

func TestOutboundModelsUseGeneratedFeaturesAndExplicitAvailability(t *testing.T) {
	features := join(fixtureNumber(12, 1), fixtureNumber(15, 1))
	info := join(fixtureNumber(4, 65536), fixtureNumber(13, 8192), fixtureText(23, "swe"),
		fixtureText(6, string(features)))
	model := join(fixtureText(22, "swe-2-high"), fixtureText(1, "SWE high"),
		fixtureNumber(5, 1), fixtureNumber(24, 4), fixtureText(23, string(info)))
	disabled := join(fixtureText(22, "disabled"), fixtureNumber(4, 1))
	router := join(fixtureText(22, "router"), fixtureText(23, string(fixtureNumber(25, 1))))
	catalog := join(fixtureText(1, string(model)), fixtureText(1, string(disabled)), fixtureText(1, string(router)))
	models, err := DecodeNativeModels(catalog)
	if err != nil || len(models) != 1 || !models[0].Tools || !models[0].Thinking || !models[0].Images ||
		!models[0].Free || models[0].Context != 65536 || models[0].MaxOutput != 8192 {
		t.Fatalf("native account model features changed: %+v %v", models, err)
	}
	for _, malformed := range [][]byte{nil, fixtureText(1, ""), join(fixtureText(1, string(model)), fixtureText(1, string(model)))} {
		if _, err := DecodeNativeModels(malformed); err == nil {
			t.Fatal("empty or ambiguous native model catalog accepted")
		}
	}
}

func TestOutboundRoundtripRetainsMetadataButNeverUsesItAsCredential(t *testing.T) {
	body := []byte(`{"model":"swe","messages":[{"role":"user","content":"x"}],"metadata":{"devin_cascade_id":"cascade","devin_session_id":"session"}}`)
	result, err := EncodeChatRequest(body, "selected-key")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(result.Payload, nil)
	metadata, _ := readMessage(fields.data(1), nil)
	if fields.text(16) != "cascade" || fields.text(28) != "session" || metadata.text(3) != "selected-key" {
		t.Fatal("canonical correlation became authority")
	}
	var parsed map[string]any
	if json.Unmarshal(body, &parsed) != nil || parsed["metadata"] == nil {
		t.Fatal("input bytes mutated")
	}
}
