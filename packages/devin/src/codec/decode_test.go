package devin

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/pkg/protocolext"
)

// All fixtures in this package are handcrafted from the documented schema.
// They do not establish parity with a live Devin client or server.
func fixtureHeaders() http.Header {
	return http.Header{"Content-Type": {"application/connect+proto"}, "Connect-Protocol-Version": {"1"}}
}

func fixtureFrame(flags byte, b []byte) []byte {
	out := []byte{flags, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[1:], uint32(len(b)))
	return append(out, b...)
}

func fixtureText(n int, s string) []byte {
	b := binary.AppendUvarint(nil, uint64(n<<3|2))
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

func fixtureNumber(n int, value uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(n<<3)), value)
}

func fixtureDouble(n int, value float64) []byte {
	b := binary.AppendUvarint(nil, uint64(n<<3|1))
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(value))
}

func join(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
func fixturePrompt(role uint64, text string, more ...[]byte) []byte {
	return join(fixtureNumber(2, role), fixtureText(3, text), join(more...))
}

func fixtureRequest(more ...[]byte) []byte {
	return join(fixtureText(14, "group/test"), fixtureText(3, string(fixturePrompt(1, "hello"))), join(more...))
}

func TestDecodeLiteralSchemaFixture(t *testing.T) {
	b, err := hex.DecodeString("00000000141a0910011a0568656c6c6f720767726f75702f6d")
	if err != nil {
		t.Fatal(err)
	}
	a := Adapter{}
	r, err := a.Decode(context.Background(), fixtureHeaders(), b)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != "devin" || a.Path() != Path || r.Protocol != protocolext.Chat {
		t.Fatalf("wrong adapter contract: %+v", r)
	}
	if err := protocolext.InboundContract().Check(a.Contract()); err != nil {
		t.Fatalf("incompatible module contract: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(r.Body, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "group/m" || got["stream"] != true || got["messages"].([]any)[0].(map[string]any)["content"] != "hello" {
		t.Fatalf("wrong Chat request: %s", r.Body)
	}
}

func TestDecodeConversation(t *testing.T) {
	tool := join(fixtureText(1, "read"), fixtureText(2, "Read a file"), fixtureText(3, `{"type":"object","properties":{"path":{"type":"string"}}}`), fixtureNumber(12, 1))
	call := join(fixtureText(1, "call-1"), fixtureText(2, "read"), fixtureText(3, `{"path":"a"}`))
	cache := fixtureNumber(1, 1)
	image := join(fixtureText(1, "aGk="), fixtureText(2, "image/png"))
	body := join(
		fixtureText(1, string(fixtureText(3, "credential-sentinel"))),
		fixtureText(2, "system"), fixtureText(13, string(cache)),
		fixtureText(3, string(fixturePrompt(1, "look", fixtureText(10, string(image)), fixtureText(8, string(cache))))),
		fixtureText(3, string(fixturePrompt(2, "reading", fixtureText(6, string(call)), fixtureText(11, "think")))),
		fixtureText(3, string(fixturePrompt(4, "result", fixtureText(7, "call-1")))),
		fixtureText(3, string(fixturePrompt(1, "continue"))),
		fixtureNumber(7, 5), fixtureText(8, string(join(fixtureNumber(2, 128), fixtureDouble(5, 0), fixtureDouble(8, 0.9)))),
		fixtureText(10, string(tool)), fixtureNumber(11, 1), fixtureText(12, string(fixtureText(2, "read"))),
		fixtureText(14, "legacy-name"), fixtureText(21, "group/explicit"), fixtureNumber(6, 12345),
		fixtureText(16, "cascade"), fixtureText(17, "prompt"), fixtureText(22, "execution"), fixtureText(27, "cache-key"))
	h := fixtureHeaders()
	h.Set("Authorization", "Bearer header-sentinel")
	before := h.Clone()
	r, err := (Adapter{}).Decode(context.Background(), h, fixtureFrame(0, body))
	if err != nil {
		t.Fatal(err)
	}
	if r.Session != "cascade" || strings.Contains(string(r.Body), "sentinel") || h.Get("Authorization") != before.Get("Authorization") {
		t.Fatalf("session or credentials mishandled: %s", r.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(r.Body, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "group/explicit" || got["temperature"] != float64(0) || got["top_p"] != 0.9 || got["max_tokens"] != float64(128) || got["parallel_tool_calls"] != false || got["prompt_cache_key"] != "cache-key" {
		t.Fatalf("request semantics lost: %s", r.Body)
	}
	if got["metadata"].(map[string]any)["devin_cascade_id"] != "cascade" {
		t.Fatal("missing cascade metadata")
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 5 || msgs[0].(map[string]any)["cache_control"].(map[string]any)["type"] != "ephemeral" {
		t.Fatal("system/cache lost")
	}
	user := msgs[1].(map[string]any)["content"].([]any)
	if user[1].(map[string]any)["image_url"].(map[string]any)["url"] != "data:image/png;base64,aGk=" {
		t.Fatal("image lost")
	}
	assistant := msgs[2].(map[string]any)
	if assistant["reasoning_content"] != "think" || assistant["reasoning_details"] != nil {
		t.Fatal("plain thinking lost or signed history manufactured")
	}
	if msgs[3].(map[string]any)["tool_call_id"] != "call-1" || msgs[3].(map[string]any)["content"] != "result" {
		t.Fatal("tool result lost")
	}
	if got["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["strict"] != true {
		t.Fatal("strict lost")
	}
}

func TestDecodeRefusesSignedThinkingHistory(t *testing.T) {
	for name, fields := range map[string][]byte{
		"signature":          fixtureText(12, "signature-sentinel"),
		"signature type":     fixtureText(18, "type-sentinel"),
		"signature and type": join(fixtureText(12, "signature-sentinel"), fixtureText(18, "type-sentinel")),
	} {
		t.Run(name, func(t *testing.T) {
			body := fixtureFrame(0, fixtureRequest(fixtureText(3, string(fixturePrompt(2, "answer", fixtureText(11, "thought"), fields)))))
			r, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), body)
			var public *protocolext.Error
			if !errors.As(err, &public) || public.Status != 400 || !strings.Contains(public.Message, "unsupported") || strings.Contains(public.Message, "sentinel") || len(r.Body) != 0 {
				t.Fatalf("expected safe signed-history refusal before canonical dispatch: body=%s err=%v", r.Body, err)
			}
		})
	}
	// Empty proto3 defaults carry no signed state and need no preservation.
	body := fixtureFrame(0, fixtureRequest(fixtureText(3, string(fixturePrompt(2, "answer", fixtureText(11, "thought"), fixtureText(12, ""), fixtureText(18, ""))))))
	r, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), body)
	if err != nil {
		t.Fatalf("empty signature defaults refused: %v", err)
	}
	if !strings.Contains(string(r.Body), `"reasoning_content":"thought"`) || strings.Contains(string(r.Body), "reasoning_details") {
		t.Fatal("plain thinking not retained")
	}
}

func TestDecodeRefusesUnsupportedAndMalformed(t *testing.T) {
	cases := map[string][]byte{
		"unknown request field":      fixtureRequest(fixtureNumber(99, 0)),
		"trajectory":                 fixtureRequest(fixtureText(15, "trajectory")),
		"enum only":                  join(fixtureNumber(6, 123), fixtureText(3, string(fixturePrompt(1, "hi")))),
		"unknown role":               join(fixtureText(14, "m"), fixtureText(3, string(fixturePrompt(3, "hi")))),
		"unknown prompt content":     fixtureRequest(fixtureText(3, string(fixturePrompt(1, "hi", fixtureText(99, "must-not-drop"))))),
		"tool result image":          fixtureRequest(fixtureText(3, string(fixturePrompt(4, "image", fixtureText(7, "orphan"), fixtureText(10, "image"))))),
		"orphan tool result":         fixtureRequest(fixtureText(3, string(fixturePrompt(4, "result", fixtureText(7, "orphan"))))),
		"unanswered call":            fixtureRequest(fixtureText(3, string(fixturePrompt(2, "", fixtureText(6, string(join(fixtureText(1, "c"), fixtureText(2, "f"), fixtureText(3, `{}`)))))))),
		"signature without thinking": fixtureRequest(fixtureText(3, string(fixturePrompt(2, "hi", fixtureText(12, "sig"))))),
		"oneof conflict":             fixtureRequest(fixtureText(12, string(join(fixtureText(1, "auto"), fixtureText(2, "f"))))),
		"unknown choice":             fixtureRequest(fixtureText(12, string(fixtureText(1, "invented")))),
		"undefined named choice":     fixtureRequest(fixtureText(12, string(fixtureText(2, "f")))),
		"opaque tuning":              fixtureRequest(fixtureText(8, string(fixtureNumber(7, 40)))),
		"nan temperature":            fixtureRequest(fixtureText(8, string(fixtureDouble(5, math.NaN())))),
		"large top p":                fixtureRequest(fixtureText(8, string(fixtureDouble(8, 1.1)))),
		"token overflow":             fixtureRequest(fixtureText(8, string(fixtureNumber(2, 1<<40)))),
		"unknown tool semantic":      fixtureRequest(fixtureText(10, string(join(fixtureText(1, "f"), fixtureText(3, `{}`), fixtureNumber(9, 1))))),
		"non-object tool schema":     fixtureRequest(fixtureText(10, string(join(fixtureText(1, "f"), fixtureText(3, `[]`))))),
		"duplicate scalar":           fixtureRequest(fixtureText(14, "other")),
		"wrong wire":                 fixtureRequest(fixtureNumber(21, 1)),
		"invalid utf8":               fixtureRequest(fixtureText(21, string([]byte{0xff}))),
		"malformed varint":           append(fixtureRequest(), 0x80),
		"zero tag":                   append(fixtureRequest(), 0),
		"truncated nested":           fixtureRequest(fixtureText(3, string([]byte{0x1a, 0x05, 'a'}))),
		"malformed metadata":         fixtureRequest(fixtureText(1, "credential-sentinel")),
		"invalid boolean":            fixtureRequest(fixtureNumber(11, 2)),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), fixtureFrame(0, body))
			var public *protocolext.Error
			if !errors.As(err, &public) || public.Status != 400 || strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), "must-not-drop") {
				t.Fatalf("expected safe refusal, got %v", err)
			}
		})
	}
}

func TestDecodeHeadersFramingAndGzip(t *testing.T) {
	plain := fixtureRequest()
	var zip bytes.Buffer
	z := gzip.NewWriter(&zip)
	if _, err := z.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	h := fixtureHeaders()
	h.Set("Connect-Content-Encoding", "gzip")
	if _, err := (Adapter{}).Decode(context.Background(), h, fixtureFrame(1, zip.Bytes())); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		h http.Header
		b []byte
	}{
		"no mime":                   {http.Header{"Connect-Protocol-Version": {"1"}}, fixtureFrame(0, plain)},
		"no version":                {http.Header{"Content-Type": {"application/connect+proto"}}, fixtureFrame(0, plain)},
		"no gzip declaration":       {fixtureHeaders(), fixtureFrame(1, zip.Bytes())},
		"unknown flags":             {fixtureHeaders(), fixtureFrame(4, plain)},
		"request trailer":           {fixtureHeaders(), fixtureFrame(2, []byte(`{}`))},
		"extra frame":               {fixtureHeaders(), join(fixtureFrame(0, plain), fixtureFrame(0, plain))},
		"truncated frame":           {fixtureHeaders(), fixtureFrame(0, plain)[:8]},
		"truncated gzip":            {h, fixtureFrame(1, zip.Bytes()[:zip.Len()-1])},
		"gzip trailing bytes":       {h, fixtureFrame(1, append(bytes.Clone(zip.Bytes()), 0))},
		"gzip concatenated members": {h, fixtureFrame(1, join(zip.Bytes(), zip.Bytes()))},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (Adapter{}).Decode(context.Background(), c.h, c.b); err == nil {
				t.Fatal("accepted invalid transport")
			}
		})
	}
	for _, e := range []string{"br", "gzip, identity"} {
		h := fixtureHeaders()
		h.Set("Connect-Content-Encoding", e)
		if _, err := (Adapter{}).Decode(context.Background(), h, fixtureFrame(0, plain)); err == nil {
			t.Fatal("accepted unsupported compression")
		}
	}
	zip.Reset()
	z = gzip.NewWriter(&zip)
	if _, err := z.Write(bytes.Repeat([]byte{'x'}, maxBody+1)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (Adapter{}).Decode(context.Background(), h, fixtureFrame(1, zip.Bytes())); err == nil {
		t.Fatal("accepted gzip expansion past bound")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Adapter{}).Decode(ctx, fixtureHeaders(), fixtureFrame(0, plain)); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}

func TestDecodeConnectTimeout(t *testing.T) {
	body := fixtureFrame(0, fixtureRequest())
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"1", time.Millisecond}, {"1250", 1250 * time.Millisecond}, {"00001", time.Millisecond}, {"9999999999", 9999999999 * time.Millisecond},
	} {
		t.Run(tc.value, func(t *testing.T) {
			h := fixtureHeaders()
			h.Set("Connect-Timeout-Ms", tc.value)
			r, err := (Adapter{}).Decode(context.Background(), h, body)
			if err != nil || r.Timeout != tc.want {
				t.Fatalf("timeout: got %v, %v; want %v", r.Timeout, err, tc.want)
			}
		})
	}
	r, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), body)
	if err != nil || r.Timeout != 0 {
		t.Fatalf("omitted timeout: %v %v", r.Timeout, err)
	}
	for _, s := range []string{"", "-1", "+1", "1.0", "1ms", " 1", "1 ", "1,2", "10000000000", "00000000000", "١", "credential-sentinel"} {
		t.Run("malformed/"+s, func(t *testing.T) {
			h := fixtureHeaders()
			h.Set("Connect-Timeout-Ms", s)
			_, err := (Adapter{}).Decode(context.Background(), h, body)
			var public *protocolext.Error
			if !errors.As(err, &public) || public.Status != 400 || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("expected safe timeout refusal: %v", err)
			}
		})
	}
	for _, s := range []string{"0", "0000000000"} {
		h := fixtureHeaders()
		h.Set("Connect-Timeout-Ms", s)
		_, err := (Adapter{}).Decode(context.Background(), h, body)
		var public *protocolext.Error
		if !errors.As(err, &public) || public.Status != 504 {
			t.Fatalf("zero timeout must refuse before dispatch: %v", err)
		}
	}
	h := fixtureHeaders()
	h["Connect-Timeout-Ms"] = []string{"1", "2"}
	if _, err := (Adapter{}).Decode(context.Background(), h, body); err == nil {
		t.Fatal("duplicate timeout accepted")
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(fixtureFrame(0, fixtureRequest()))
	f.Add([]byte{0, 0, 0, 0, 1, 0x80})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > maxBody+5 {
			return
		}
		(Adapter{}).Decode(context.Background(), fixtureHeaders(), b)
	})
}
