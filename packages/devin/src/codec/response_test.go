package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/pkg/protocolext"
)

type nativeFrame struct {
	flags byte
	body  []byte
}

func nativeFrames(t *testing.T, b []byte) []nativeFrame {
	t.Helper()
	var frames []nativeFrame
	for len(b) > 0 {
		if len(b) < 5 {
			t.Fatal("partial native header")
		}
		n := int(binary.BigEndian.Uint32(b[1:5]))
		if n > len(b)-5 {
			t.Fatal("partial native data")
		}
		frames = append(frames, nativeFrame{b[0], b[5 : 5+n]})
		b = b[5+n:]
	}
	return frames
}

func terminal(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	frames := nativeFrames(t, w.Body.Bytes())
	if len(frames) == 0 || frames[len(frames)-1].flags != 2 {
		t.Fatal("missing Connect terminal frame")
	}
	var got map[string]any
	if err := json.Unmarshal(frames[len(frames)-1].body, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func sse(data string) string { return "data: " + data + "\n\n" }
func chunk(delta string, finish string) string {
	end := "null"
	if finish != "" {
		b, _ := json.Marshal(finish)
		end = string(b)
	}
	return sse(`{"id":"reply","choices":[{"index":0,"delta":` + delta + `,"finish_reason":` + end + `}]}`)
}

func wholeChat(message string, finish string) string {
	end := "null"
	if finish != "" {
		value, _ := json.Marshal(finish)
		end = string(value)
	}
	return `{"id":"reply","object":"chat.completion","created":1,"model":"test","choices":[{"index":0,"message":` + message + `,"finish_reason":` + end + `}]}`
}

func TestResponseStopMatchesRecoveredGeneratedDescriptor(t *testing.T) {
	const source = "Agents/devinapp/gui-src/restored/chat-client/src/stores/nxi.ts:34700-34768 exa.codeium_common_pb.StopReason"
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	if _, err := io.WriteString(r, chunk(`{"content":"done"}`, "stop")+sse("[DONE]")); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	got, err := readMessage(nativeFrames(t, w.Body.Bytes())[0].body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[5]) != 1 || got.number(5) != 2 {
		t.Fatalf("ordinary stop must be STOP_REASON_STOP_PATTERN=2 from %s; got %d", source, got.number(5))
	}
}

func TestResponseWholeJSONTextAndThinking(t *testing.T) {
	for name, thinking := range map[string]string{
		"plain":   `"reasoning_content":"thought"`,
		"signed":  `"reasoning":"thought","reasoning_signature":"sig","reasoning_signature_type":"anthropic"`,
		"details": `"reasoning_content":"thought","reasoning_details":[{"type":"reasoning.text","text":"thought","signature":"sig","format":"anthropic","index":0}]`,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := (Adapter{}).Reply(w)
			r.Header().Set("Content-Type", "application/json; charset=utf-8")
			body := wholeChat(`{"role":"assistant","content":"héllo","refusal":null,`+thinking+`}`, "stop")
			for _, value := range []byte(body) {
				if written, err := r.Write([]byte{value}); err != nil || written != 1 {
					t.Fatalf("whole JSON write: %d %v", written, err)
				}
			}
			if w.Body.Len() != 0 {
				t.Fatal("whole reply emitted before complete validation")
			}
			if err := r.Finish(nil); err != nil {
				t.Fatal(err)
			}
			frames := nativeFrames(t, w.Body.Bytes())
			if len(frames) != 2 || frames[0].flags != 0 || terminal(t, w)["error"] != nil {
				t.Fatal("whole reply did not produce one data frame and successful terminal")
			}
			got, err := readMessage(frames[0].body, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.text(1) != "reply" || got.text(3) != "héllo" || got.text(9) != "thought" || len(got[9]) != 1 || got.number(5) != 2 {
				t.Fatal("whole reply lost text, thinking or finish")
			}
			if name != "plain" && (got.text(10) != "sig" || got.text(21) != "anthropic") {
				t.Fatal("whole reply lost established signature provenance")
			}
			size := w.Body.Len()
			if err := r.Finish(nil); err != nil || w.Body.Len() != size {
				t.Fatal("whole reply duplicated its terminal")
			}
		})
	}
}

func TestResponseWholeJSONToolsAndCacheUsage(t *testing.T) {
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	r.Header().Set("Content-Type", "application/json")
	body := wholeChat(`{"role":"assistant","content":null,"tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{\"p\":1}"}},{"id":"b","type":"function","function":{"name":"stat","arguments":"{}"}}]}`, "tool_calls")
	body = strings.TrimSuffix(body, "}") + `,"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":2}}}`
	if _, err := io.WriteString(r, body); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if terminal(t, w)["error"] != nil {
		t.Fatal("whole tool reply failed")
	}
	got, err := readMessage(nativeFrames(t, w.Body.Bytes())[0].body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.number(5) != 10 || len(got[6]) != 2 {
		t.Fatal("whole reply lost function calls or finish")
	}
	for index, expected := range []struct{ id, name, args string }{{"a", "read", `{"p":1}`}, {"b", "stat", `{}`}} {
		call, err := readMessage(got[6][index].b, nil)
		if err != nil {
			t.Fatal(err)
		}
		if call.text(1) != expected.id || call.text(2) != expected.name || call.text(3) != expected.args {
			t.Fatal("whole reply mixed function calls")
		}
	}
	stats, err := readMessage(got.data(7), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.number(2) != 7 || stats.number(3) != 4 || stats.number(4) != 2 || stats.number(5) != 3 {
		t.Fatal("whole reply lost cache usage accounting")
	}
}

func TestResponseWholeJSONRejectsInvalidOrUnsupportedReplies(t *testing.T) {
	valid := wholeChat(`{"role":"assistant","content":"text"}`, "stop")
	cases := map[string]struct{ body, code string }{
		"empty":               {"", "data_loss"},
		"malformed":           {`{"choices":`, "data_loss"},
		"truncated":           {strings.TrimSuffix(valid, "}"), "data_loss"},
		"trailing JSON":       {valid + `{}`, "data_loss"},
		"invalid UTF8":        {strings.Replace(valid, "text", "\xff", 1), "data_loss"},
		"not object":          {`[]`, "data_loss"},
		"no finish":           {wholeChat(`{"role":"assistant","content":"partial"}`, ""), "data_loss"},
		"missing finish":      {strings.ReplaceAll(valid, `,"finish_reason":"stop"`, ""), "data_loss"},
		"empty finish":        {strings.ReplaceAll(valid, `"finish_reason":"stop"`, `"finish_reason":""`), "unimplemented"},
		"unknown finish":      {wholeChat(`{"role":"assistant"}`, "new_reason"), "unimplemented"},
		"content filter":      {wholeChat(`{"role":"assistant"}`, "content_filter"), "permission_denied"},
		"error envelope":      {`{"error":{"message":"public failure"}}`, "internal"},
		"missing choice":      {`{"choices":[]}`, "data_loss"},
		"multiple choices":    {`{"choices":[{"index":0,"message":{"role":"assistant"},"finish_reason":"stop"},{"index":1,"message":{"role":"assistant"},"finish_reason":"stop"}]}`, "unimplemented"},
		"missing index":       {strings.Replace(valid, `"index":0,`, "", 1), "data_loss"},
		"nonzero index":       {strings.Replace(valid, `"index":0`, `"index":1`, 1), "data_loss"},
		"null message":        {wholeChat(`null`, "stop"), "data_loss"},
		"missing role":        {wholeChat(`{"content":"text"}`, "stop"), "data_loss"},
		"wrong role":          {wholeChat(`{"role":"user","content":"text"}`, "stop"), "data_loss"},
		"delta for message":   {strings.Replace(valid, `"message":`, `"delta":`, 1), "data_loss"},
		"unknown top field":   {strings.TrimSuffix(valid, "}") + `,"audio":"lost"}`, "data_loss"},
		"unknown choice":      {strings.Replace(valid, `"index":0`, `"index":0,"audio":"lost"`, 1), "data_loss"},
		"unknown message":     {wholeChat(`{"role":"assistant","audio":{"data":"lost"}}`, "stop"), "unimplemented"},
		"refusal":             {wholeChat(`{"role":"assistant","refusal":"cannot"}`, "stop"), "unimplemented"},
		"nonstring content":   {wholeChat(`{"role":"assistant","content":[{"type":"text","text":"lost"}]}`, "stop"), "data_loss"},
		"logprobs":            {strings.Replace(valid, `"index":0`, `"index":0,"logprobs":{}`, 1), "unimplemented"},
		"encrypted thinking":  {wholeChat(`{"role":"assistant","reasoning_details":[{"type":"reasoning.encrypted","data":"foreign-seal-sentinel","format":"openai-responses-v1"}]}`, "stop"), "unimplemented"},
		"ambiguous thinking":  {wholeChat(`{"role":"assistant","reasoning":"a","reasoning_content":"b"}`, "stop"), "data_loss"},
		"ambiguous signature": {wholeChat(`{"role":"assistant","signature":"a","reasoning_details":[{"type":"reasoning.text","signature":"b"}]}`, "stop"), "data_loss"},
		"missing tool":        {wholeChat(`{"role":"assistant"}`, "tool_calls"), "data_loss"},
		"tool without finish": {wholeChat(`{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]}`, "stop"), "data_loss"},
		"invalid arguments":   {wholeChat(`{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{"}}]}`, "tool_calls"), "data_loss"},
		"duplicate tool ids":  {wholeChat(`{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}},{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]}`, "tool_calls"), "data_loss"},
		"custom tool":         {wholeChat(`{"role":"assistant","tool_calls":[{"id":"a","type":"custom","custom":{"name":"read","input":"text"}}]}`, "tool_calls"), "data_loss"},
		"unknown tool field":  {wholeChat(`{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}","audio":"lost"}}]}`, "tool_calls"), "data_loss"},
		"invalid usage":       {strings.TrimSuffix(valid, "}") + `,"usage":{"prompt_tokens":1,"prompt_tokens_details":{"cached_tokens":2}}}`, "data_loss"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := (Adapter{}).Reply(w)
			r.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(r, test.body); err != nil {
				t.Fatal(err)
			}
			if err := r.Finish(nil); err == nil {
				t.Fatal("invalid whole reply succeeded")
			}
			frames := nativeFrames(t, w.Body.Bytes())
			if len(frames) != 1 || frames[0].flags != 2 {
				t.Fatal("invalid whole reply emitted partial data or multiple terminal frames")
			}
			if terminal(t, w)["error"].(map[string]any)["code"] != test.code {
				t.Fatalf("wrong terminal code, expected %s", test.code)
			}
			if strings.Contains(w.Body.String(), "foreign-seal-sentinel") {
				t.Fatal("foreign sealed reasoning leaked")
			}
		})
	}
}

func TestResponseWholeJSONBounds(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(map[int]string{0: "at limit", 1: "over limit"}[extra], func(t *testing.T) {
			body := wholeChat(`{"role":"assistant","content":""}`, "stop")
			body = strings.Replace(body, `"content":""`, `"content":"`+strings.Repeat("x", maxBody-len(body))+`"`, 1)
			body += strings.Repeat(" ", extra)
			w := httptest.NewRecorder()
			r := (Adapter{}).Reply(w)
			r.Header().Set("Content-Type", "application/json")
			for len(body) > 0 {
				size := min(len(body), maxEvent/2)
				written, err := io.WriteString(r, body[:size])
				if err != nil {
					if extra == 0 || len(body) != 1 || written != 0 {
						t.Fatalf("whole body bound: %d %v", written, err)
					}
					break
				}
				body = body[size:]
			}
			if len(r.(*response).data) > maxBody {
				t.Fatal("whole body accumulated beyond bound")
			}
			err := r.Finish(nil)
			if extra == 0 {
				if err != nil || terminal(t, w)["error"] != nil {
					t.Fatalf("whole body at limit refused: %v", err)
				}
			} else if err == nil || terminal(t, w)["error"].(map[string]any)["code"] != "resource_exhausted" {
				t.Fatal("whole body overflow succeeded or lost exhaustion code")
			}
		})
	}
	for _, extra := range []int{0, 1} {
		t.Run(map[int]string{0: "tools at limit", 1: "tools over limit"}[extra], func(t *testing.T) {
			args := `{"x":"` + strings.Repeat("x", maxEvent-10+extra) + `"}`
			message, err := json.Marshal(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a", "type": "function", "function": map[string]string{"name": "f", "arguments": args}}}})
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			r := (Adapter{}).Reply(w)
			r.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(r, wholeChat(string(message), "tool_calls")); err != nil {
				t.Fatal(err)
			}
			err = r.Finish(nil)
			if extra == 0 {
				if err != nil || terminal(t, w)["error"] != nil {
					t.Fatalf("whole tools at limit refused: %v", err)
				}
			} else if err == nil || terminal(t, w)["error"].(map[string]any)["code"] != "resource_exhausted" {
				t.Fatal("whole tool overflow succeeded or lost exhaustion code")
			}
		})
	}
}

func TestResponseWholeJSONCancellationCause(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		code  string
	}{
		{"deadline", context.DeadlineExceeded, "deadline_exceeded"},
		{"canceled", context.Canceled, "canceled"},
		{"unknown", errors.New("secret-cause-sentinel"), "internal"},
	} {
		for _, body := range []string{"", `{"choices":[{"index":0,"message":`} {
			t.Run(test.name+map[bool]string{false: "/empty", true: "/partial"}[body != ""], func(t *testing.T) {
				w := httptest.NewRecorder()
				r := (Adapter{}).Reply(w)
				r.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(r, body); err != nil {
					t.Fatal(err)
				}
				if err := r.Finish(test.cause); err == nil {
					t.Fatal("interrupted whole JSON succeeded")
				}
				if terminal(t, w)["error"].(map[string]any)["code"] != test.code || strings.Contains(w.Body.String(), "secret-cause-sentinel") {
					t.Fatal("whole JSON lost safe cancellation semantics")
				}
			})
		}
		w := httptest.NewRecorder()
		r := (Adapter{}).Reply(w)
		r.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(r, wholeChat(`{"role":"assistant","content":"complete"}`, "stop")); err != nil {
			t.Fatal(err)
		}
		if err := r.Finish(test.cause); err != nil || terminal(t, w)["error"] != nil {
			t.Fatalf("late cancellation changed completed whole JSON: %v", err)
		}
	}
}

func TestResponseStreamsSynchronouslyAcrossEveryByte(t *testing.T) {
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	r.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	r.Header().Set("Content-Length", "999")
	r.Header().Set("Retry-After", "3")
	r.Header().Set("X-Magpie-Session", "scoped-session")
	r.Header().Set("X-Private", "secret")
	stream := chunk(`{"role":"assistant","content":"héllo","reasoning_content":"think","signature":"sig","signature_type":"anthropic"}`, "")
	stream = strings.ReplaceAll(stream, "\n", "\r\n")
	for _, b := range []byte(stream) {
		if n, err := r.Write([]byte{b}); err != nil || n != 1 {
			t.Fatalf("write: %d %v", n, err)
		}
	}
	if w.Body.Len() == 0 || !w.Flushed {
		t.Fatal("text did not stream before Finish")
	}
	if r.(interface{ Unwrap() http.ResponseWriter }).Unwrap() != w {
		t.Fatal("Unwrap lost destination")
	}
	usage := sse(`{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":2}}}`)
	if _, err := io.WriteString(r, chunk(`{}`, "stop")+usage+sse("[DONE]")); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if terminal(t, w)["error"] != nil {
		t.Fatalf("unexpected native error: %s", w.Body.Bytes())
	}
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/connect+proto" || w.Header().Get("Connect-Protocol-Version") != "1" || w.Header().Get("Content-Length") != "" || w.Header().Get("X-Private") != "" || w.Header().Get("Retry-After") != "3" || w.Header().Get("X-Magpie-Session") != "scoped-session" {
		t.Fatalf("incorrect native headers: %+v", w.Header())
	}
	frames := nativeFrames(t, w.Body.Bytes())
	m, err := readMessage(frames[0].body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.text(3) != "héllo" || m.text(9) != "think" || m.text(10) != "sig" || m.text(21) != "anthropic" {
		t.Fatalf("lost native deltas: %+v", m)
	}
	u, err := readMessage(frames[len(frames)-2].body, nil)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := readMessage(u.data(7), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.number(2) != 7 || stats.number(3) != 4 || stats.number(4) != 2 || stats.number(5) != 3 {
		t.Fatalf("wrong native token accounting: %+v", stats)
	}
	n := w.Body.Len()
	if err := r.Finish(nil); err != nil || w.Body.Len() != n {
		t.Fatal("Finish duplicated terminal")
	}
	if _, err := r.Write([]byte("later")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("write after Finish accepted")
	}
}

func TestResponseInterleavedTools(t *testing.T) {
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	stream := chunk(`{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"rea","arguments":"{\"p\":"}}]}`, "") +
		chunk(`{"tool_calls":[{"index":1,"id":"b","type":"function","function":{"name":"stat","arguments":"{}"}}]}`, "") +
		chunk(`{"tool_calls":[{"index":0,"function":{"name":"d","arguments":"1}"}}]}`, "") +
		chunk(`{}`, "tool_calls") + sse("[DONE]")
	if _, err := io.WriteString(r, stream); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	var calls []message
	var stop uint64
	for _, f := range nativeFrames(t, w.Body.Bytes()) {
		if f.flags != 0 {
			continue
		}
		m, err := readMessage(f.body, nil)
		if err != nil {
			t.Fatal(err)
		}
		if m.number(5) != 0 {
			stop = m.number(5)
		}
		for _, c := range m[6] {
			v, err := readMessage(c.b, nil)
			if err != nil {
				t.Fatal(err)
			}
			calls = append(calls, v)
		}
	}
	if stop != 10 || len(calls) != 2 || calls[0].text(1) != "a" || calls[0].text(2) != "read" || calls[0].text(3) != `{"p":1}` || calls[1].text(1) != "b" || calls[1].text(3) != `{}` {
		t.Fatalf("tool deltas mixed: stop=%d, calls=%+v", stop, calls)
	}
}

func TestResponseLengthAndReasoningDetails(t *testing.T) {
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	if _, err := io.WriteString(r, chunk(`{"reasoning_content":"thought","reasoning_details":[{"type":"reasoning.text","text":"thought","signature":"sig","format":"anthropic","index":0}]}`, "length")+sse("[DONE]")); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(nil); err != nil {
		t.Fatal(err)
	}
	m, err := readMessage(nativeFrames(t, w.Body.Bytes())[0].body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.text(9) != "thought" || len(m[9]) != 1 || m.text(10) != "sig" || m.number(5) != 3 {
		t.Fatal("thinking/length semantics lost")
	}
}

func TestResponseFailuresHaveNativeTerminalErrors(t *testing.T) {
	cases := map[string]string{
		"empty":                "",
		"no done":              chunk(`{"content":"partial"}`, "stop"),
		"no finish":            chunk(`{"content":"partial"}`, "") + sse("[DONE]"),
		"truncated event":      chunk(`{"content":"partial"}`, "") + `data: {"choices":`,
		"unterminated done":    chunk(`{}`, "stop") + "data: [DONE]\n",
		"bad json":             sse(`{"choices":`),
		"not object":           sse(`[]`),
		"after done":           chunk(`{}`, "stop") + sse("[DONE]") + chunk(`{"content":"late"}`, ""),
		"after finish":         chunk(`{}`, "stop") + chunk(`{"content":"late"}`, "") + sse("[DONE]"),
		"unknown delta":        chunk(`{"audio":{"data":"lost"}}`, "stop") + sse("[DONE]"),
		"nonstring content":    chunk(`{"content":[{"type":"text","text":"lost"}]}`, "stop") + sse("[DONE]"),
		"filter":               chunk(`{}`, "content_filter") + sse("[DONE]"),
		"unknown finish":       chunk(`{}`, "new_reason") + sse("[DONE]"),
		"gateway SSE error":    sse(`{"error":{"message":"upstream failed"}}`),
		"multiple choices":     sse(`{"choices":[{"index":0,"delta":{}},{"index":1,"delta":{}}]}`),
		"nonstreaming choice":  sse(`{"choices":[{"index":0,"message":{"content":"must-not-drop"},"finish_reason":"stop"}]}`) + sse("[DONE]"),
		"missing choice index": sse(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`) + sse("[DONE]"),
		"null delta":           sse(`{"choices":[{"index":0,"delta":null,"finish_reason":"stop"}]}`) + sse("[DONE]"),
		"ambiguous signatures": chunk(`{"signature":"a","reasoning_details":[{"type":"reasoning.text","text":"thought","signature":"b"}]}`, "stop") + sse("[DONE]"),
		"incomplete tool":      chunk(`{"tool_calls":[{"index":0,"id":"a","function":{"name":"f","arguments":"{"}}]}`, "tool_calls") + sse("[DONE]"),
		"missing tool index":   chunk(`{"tool_calls":[{"id":"a","function":{"name":"f","arguments":"{}"}}]}`, "tool_calls") + sse("[DONE]"),
		"invalid tool usage":   chunk(`{}`, "tool_calls") + sse("[DONE]"),
		"invalid usage":        chunk(`{}`, "stop") + sse(`{"choices":[],"usage":{"prompt_tokens":1,"prompt_tokens_details":{"cached_tokens":2}}}`) + sse("[DONE]"),
		"oversized event":      "data: " + strings.Repeat("x", maxEvent),
		"oversized comments":   strings.Repeat(":"+strings.Repeat("x", 1024)+"\n", 1024),
	}
	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := (Adapter{}).Reply(w)
			io.WriteString(r, stream)
			if err := r.Finish(nil); err == nil {
				t.Fatal("false success on invalid stream")
			}
			trailer := terminal(t, w)
			if trailer["error"] == nil {
				t.Fatal("missing terminal error")
			}
			frames := nativeFrames(t, w.Body.Bytes())
			count := 0
			for _, f := range frames {
				if f.flags == 2 {
					count++
				}
			}
			if count != 1 {
				t.Fatal("multiple terminal frames")
			}
		})
	}
}

func TestResponseGatewayJSONErrorsUseStreamingStatus(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 413, 415, 429, 500, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			r := (Adapter{}).Reply(w)
			r.Header().Set("Content-Type", "application/json")
			r.Header().Set("Retry-After", "9")
			r.WriteHeader(status)
			if _, err := io.WriteString(r, `{"error":{"message":"public failure"}}`); err != nil {
				t.Fatal(err)
			}
			if err := r.Finish(nil); err == nil {
				t.Fatal("failed gateway request succeeded")
			}
			e := terminal(t, w)["error"].(map[string]any)
			if w.Code != http.StatusOK || r.(*response).status != status || e["code"] != statusCode(status) || e["message"] != "public failure" || w.Header().Get("Retry-After") != "9" {
				t.Fatalf("incorrect streaming HTTP status or lost canonical status/error: %d %+v", w.Code, e)
			}
		})
	}
}

func TestResponseDecodeRefusalUsesStreamingStatus(t *testing.T) {
	_, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), []byte{0})
	var public *protocolext.Error
	if !errors.As(err, &public) || public.Status != 400 {
		t.Fatalf("expected public invalid-frame refusal: %v", err)
	}
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	r.WriteHeader(public.Status)
	body, err := json.Marshal(map[string]any{"error": map[string]string{"message": public.Message}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(nil); err == nil {
		t.Fatal("decode refusal became success")
	}
	if w.Code != http.StatusOK || terminal(t, w)["error"].(map[string]any)["code"] != "invalid_argument" {
		t.Fatal("invalid-request terminal violated streaming status")
	}
}

func TestResponseRefusesEncryptedReasoningProvenance(t *testing.T) {
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	_, err := io.WriteString(r, chunk(`{"reasoning_details":[{"type":"reasoning.encrypted","data":"codex-seal-sentinel","format":"openai-responses-v1"}]}`, "stop")+sse("[DONE]"))
	if err == nil {
		t.Fatal("sealed foreign reasoning accepted as Devin signature")
	}
	if err := r.Finish(nil); err == nil {
		t.Fatal("unsupported reasoning succeeded")
	}
	if terminal(t, w)["error"].(map[string]any)["code"] != "unimplemented" {
		t.Fatal("wrong provenance refusal code")
	}
	if strings.Contains(w.Body.String(), "codex-seal-sentinel") {
		t.Fatal("sealed reasoning leaked into native response")
	}
	for _, f := range nativeFrames(t, w.Body.Bytes()) {
		if f.flags == 0 {
			m, err := readMessage(f.body, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(m[10]) > 0 {
				t.Fatal("foreign seal emitted as native signature")
			}
		}
	}
}

func TestResponseFinishCancellationCause(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		code  string
	}{
		{"deadline", context.DeadlineExceeded, "deadline_exceeded"},
		{"canceled", context.Canceled, "canceled"},
		{"wrapped deadline", errors.Join(errors.New("secret-sentinel"), context.DeadlineExceeded), "deadline_exceeded"},
		{"unknown cause", errors.New("secret-sentinel"), "internal"},
	} {
		for _, partial := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/empty", true: "/partial"}[partial], func(t *testing.T) {
				w := httptest.NewRecorder()
				r := (Adapter{}).Reply(w)
				if partial {
					if _, err := io.WriteString(r, chunk(`{"content":"partial"}`, "")); err != nil {
						t.Fatal(err)
					}
				}
				if err := r.Finish(tc.cause); err == nil {
					t.Fatal("interrupted stream succeeded")
				}
				if w.Code != http.StatusOK || terminal(t, w)["error"].(map[string]any)["code"] != tc.code || strings.Contains(w.Body.String(), "secret-sentinel") {
					t.Fatal("cancellation mapping lost or exposed raw cause")
				}
			})
		}
		w := httptest.NewRecorder()
		r := (Adapter{}).Reply(w)
		if _, err := io.WriteString(r, chunk(`{"content":"complete"}`, "stop")+sse("[DONE]")); err != nil {
			t.Fatal(err)
		}
		if err := r.Finish(tc.cause); err != nil || terminal(t, w)["error"] != nil {
			t.Fatalf("late cancellation changed completed stream: %v", err)
		}
	}
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	r.Write([]byte("data: invalid\n\n"))
	r.Finish(context.Canceled)
	if terminal(t, w)["error"].(map[string]any)["code"] != "data_loss" {
		t.Fatal("cancellation overwrote earlier codec error")
	}
}

func TestUsageReversesInstalledPluginAccounting(t *testing.T) {
	// The installed events() was independently exercised with native
	// input/output/cacheWrite/cacheRead=7/4/2/3. It emits exactly this Chat
	// usage; v2Usage() also reports noCache=12-3-2. Pin that reverse mapping.
	b, err := encodeUsage([]byte(`{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{0x10, 7, 0x18, 4, 0x20, 2, 0x28, 3}) {
		t.Fatalf("wrong native usage counters: %x", b)
	}
	for _, raw := range []string{
		`{"prompt_tokens":4,"completion_tokens":0,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":2}}`,
		`{"prompt_tokens":1,"completion_tokens":-1}`,
	} {
		if _, err := encodeUsage([]byte(raw)); err == nil {
			t.Fatal("inconsistent usage silently clamped")
		}
	}
}

func TestResponseBoundsAggregateToolsAndGatewayErrors(t *testing.T) {
	w := httptest.NewRecorder()
	r := (Adapter{}).Reply(w)
	// Every event is within its limit; the accumulated arguments are not.
	part := strings.Repeat("x", maxEvent/2)
	data, err := json.Marshal(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "a", "function": map[string]string{"name": "f", "arguments": part}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(r, chunk(string(data), "")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(r, chunk(string(data), "")); err == nil {
		t.Fatal("aggregate tool bound not enforced")
	}
	if err := r.Finish(nil); err == nil || terminal(t, w)["error"] == nil {
		t.Fatal("tool overflow succeeded")
	}
	w = httptest.NewRecorder()
	r = (Adapter{}).Reply(w)
	r.WriteHeader(429)
	if _, err := io.WriteString(r, strings.Repeat("x", 64<<10+1)); err == nil {
		t.Fatal("gateway error bound not enforced")
	}
	if err := r.Finish(nil); err == nil || terminal(t, w)["error"] == nil || w.Code != http.StatusOK || r.(*response).status != 429 {
		t.Fatal("error overflow lost failure or canonical status")
	}
}

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header     { return w.header }
func (*failingWriter) WriteHeader(int)           {}
func (*failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type deadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineWriter) SetWriteDeadline(v time.Time) error { w.deadline = v; return nil }

func TestResponseDestinationErrorsAndController(t *testing.T) {
	r := (Adapter{}).Reply(&failingWriter{make(http.Header)})
	if _, err := io.WriteString(r, chunk(`{"content":"x"}`, "stop")); err == nil {
		t.Fatal("destination failure lost")
	}
	if err := r.Finish(nil); err == nil {
		t.Fatal("destination Finish failure lost")
	}
	w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	r = (Adapter{}).Reply(w)
	want := time.Unix(123, 0)
	if err := http.NewResponseController(r).SetWriteDeadline(want); err != nil || w.deadline != want {
		t.Fatal("controller did not unwrap")
	}
}

func FuzzResponse(f *testing.F) {
	f.Add([]byte(chunk(`{"content":"hello"}`, "stop") + sse("[DONE]")))
	f.Add([]byte("data: {\r\n\r\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > maxEvent+1 {
			return
		}
		w := httptest.NewRecorder()
		r := (Adapter{}).Reply(w)
		r.Write(b)
		r.Finish(nil)
		terminal(t, w)
	})
}

func FuzzResponseWholeJSON(f *testing.F) {
	f.Add([]byte(wholeChat(`{"role":"assistant","content":"hello"}`, "stop")))
	f.Add([]byte(wholeChat(`{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]}`, "tool_calls")))
	f.Add([]byte(`{"choices":[`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > maxEvent+1 {
			return
		}
		w := httptest.NewRecorder()
		r := (Adapter{}).Reply(w)
		r.Header().Set("Content-Type", "application/json")
		r.Write(body)
		r.Finish(nil)
		terminal(t, w)
	})
}
