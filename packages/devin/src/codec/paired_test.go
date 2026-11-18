package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
)

func TestPairedNativeEnvelopeKeepsOpaqueControlsAndDropsIncomingAuthority(t *testing.T) {
	config := append(fixtureNumber(1, 1), fixtureNumber(2, 100)...)
	config = append(config, 41)
	config = binary.LittleEndian.AppendUint64(config, 0)
	prompt := join(fixtureNumber(2, 2), fixtureText(3, "answer"), fixtureText(11, "thinking"), fixtureText(12, "signed"),
		fixtureText(18, "format"), fixtureText(22, "actual"))
	payload := join(fixtureText(1, string(fixtureText(3, "LOCAL-CREDENTIAL"))), fixtureText(21, "group/alias"),
		fixtureText(3, string(prompt)), fixtureText(8, string(config)), fixtureText(27, "cache"), fixtureText(15, string(fixtureText(1, "trajectory"))))
	wire, _, err := EncodeConnectRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := ParseNativeEnvelope(t.Context(), fixtureHeaders(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(envelope.Payload, []byte("LOCAL-CREDENTIAL")) || bytes.Contains(envelope.Projection, []byte("signed")) ||
		len(envelope.Claims) != 1 || envelope.Claims[0].Digest != SignatureDigest([]byte("signed")) {
		t.Fatal("authority or opaque signature entered projection")
	}
	selected, err := ReplaceNativeAuthority(envelope.Payload, "selected-model", "SDK-CREDENTIAL")
	if err != nil {
		t.Fatal(err)
	}
	originalFields, _ := RawFields(envelope.Payload)
	selectedFields, _ := RawFields(selected)
	for _, number := range []int{3, 8, 15, 27} {
		if !bytes.Equal(originalFields[number], selectedFields[number]) {
			t.Fatal("raw native field changed", number)
		}
	}
	if !bytes.Contains(selectedFields[1], []byte("SDK-CREDENTIAL")) || bytes.Contains(selected, []byte("group/alias")) {
		t.Fatal("selected authority missing")
	}
}

func TestNativeRawStreamKeepsTerminalErrorsAndRejectsTruncationOrTrailingFrames(t *testing.T) {
	data := join(fixtureText(3, "native"), fixtureNumber(5, 2), fixtureText(10, "signature"))
	wire, _, err := EncodeConnectRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	terminal := []byte{2, 0, 0, 0, 2, '{', '}'}
	for _, test := range []struct {
		name  string
		wire  []byte
		valid bool
	}{
		{"complete", join(wire, terminal), true},
		{"truncated", wire[:len(wire)-1], false},
		{"missing terminal", wire, false},
		{"trailing data", join(wire, terminal, []byte{0}), false},
		{"missing error code", fixtureFrame(2, []byte(`{"error":{}}`)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var frames []NativeFrame
			summary, err := ReadNativeRawStream(t.Context(), bytes.NewReader(test.wire), false, func(frame NativeFrame) error { frames = append(frames, frame); return nil })
			if (err == nil) != test.valid {
				t.Fatal("invalid raw-stream result", err)
			}
			if test.valid && (summary.Stop != 2 || len(frames) != 2 || !bytes.Equal(frames[0].Payload, data) || frames[1].Flags != 2) {
				t.Fatal("native payload changed")
			}
		})
	}
	failure := []byte(`{"error":{"code":"resource_exhausted","message":"native refusal"},"metadata":{"key":["value"]}}`)
	end := []byte{2, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(end[1:], uint32(len(failure)))
	end = append(end, failure...)
	captured := []byte(nil)
	_, err = ReadNativeRawStream(t.Context(), bytes.NewReader(end), false, func(frame NativeFrame) error { captured = frame.Payload; return nil })
	var native *NativeError
	if !errors.As(err, &native) || native.Code != "resource_exhausted" || !bytes.Equal(captured, failure) {
		t.Fatal("native refusal was hidden or relabeled", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ReadNativeRawStream(ctx, bytes.NewReader(end), false, func(NativeFrame) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}

func TestNativeResponseControlsStayOnPairedRoute(t *testing.T) {
	control := string(fixtureText(1, "opaque"))
	payload := join(
		fixtureText(1, "message-current"), fixtureText(2, string(fixtureNumber(1, 1794957491))),
		fixtureText(3, "answer"), fixtureNumber(5, 2), fixtureText(15, "output-distinct"),
		fixtureText(17, "request-distinct"), fixtureText(29, control), fixtureText(30, control),
	)
	var frames []NativeFrame
	summary, err := ReadNativeRawStream(t.Context(), bytes.NewReader(nativeStreamFixture(payload)), false, func(frame NativeFrame) error {
		frames = append(frames, frame)
		return nil
	})
	if err != nil || len(frames) != 2 || !bytes.Equal(frames[0].Payload, payload) {
		t.Fatalf("paired route lost a recovered response field: %v", err)
	}
	if summary.Facts.ResponseID != "message-current" || summary.Facts.RequestID != "request-distinct" {
		t.Fatalf("execution facts do not distinguish message and request IDs: %+v", summary.Facts)
	}
	for _, field := range []int{29, 30} {
		translated := join(fixtureText(3, "answer"), fixtureNumber(5, 2), fixtureText(field, control))
		var failure *NativeError
		if _, err := DecodeNativeDelta(translated); !errors.As(err, &failure) || failure.Code != "unimplemented" {
			t.Fatalf("standard bridge silently dropped native response field %d: %v", field, err)
		}
	}
}

func TestPairedNativeMetadataNeverSilentlyDropsAccountSelection(t *testing.T) {
	for _, metadata := range [][]byte{
		fixtureText(22, "force-team"), fixtureText(32, "team"), fixtureText(29, "tier"),
		fixtureNumber(33, 1), fixtureText(31, "opaque"), fixtureNumber(34, 1),
	} {
		request := fixtureRequest(fixtureText(1, string(metadata)))
		wire, _, err := EncodeConnectRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseNativeEnvelope(t.Context(), fixtureHeaders(), wire); err == nil {
			t.Fatal("native metadata account control was silently ignored")
		}
	}
}

func TestPairedProjectionRetainsToolAdmissionAndHistoryAssociation(t *testing.T) {
	definition := join(fixtureText(1, "read_file"), fixtureText(3, `{"type":"object"}`), fixtureNumber(7, 1))
	call := join(fixtureText(1, "native id"), fixtureText(2, "read_file"), fixtureText(3, `{"path":"a"}`))
	assistant := fixturePrompt(2, "", fixtureText(6, string(call)))
	result := fixturePrompt(4, "contents", fixtureText(7, "native id"))
	payload := join(fixtureText(21, "model"), fixtureText(10, string(definition)), fixtureText(3, string(assistant)), fixtureText(3, string(result)))
	wire, _, err := EncodeConnectRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := ParseNativeEnvelope(t.Context(), fixtureHeaders(), wire)
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Tools    []json.RawMessage `json:"tools"`
		Messages []struct {
			Calls []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
			CallID string `json:"tool_call_id"`
		} `json:"messages"`
	}
	if json.Unmarshal(envelope.Projection, &projection) != nil || len(projection.Tools) != 1 || len(projection.Messages) != 2 ||
		len(projection.Messages[0].Calls) != 1 || projection.Messages[0].Calls[0].ID != CanonicalToolID("native id") ||
		projection.Messages[1].CallID != projection.Messages[0].Calls[0].ID {
		t.Fatal("native tool capability or history association disappeared from admission projection")
	}
	fields, _ := RawFields(envelope.Payload)
	if !bytes.Equal(fields[10], fixtureText(10, string(definition))) {
		t.Fatal("native readonly tool definition changed")
	}
}

func TestNativeOutputLimitPreservesOpaqueFieldsAndValidatesAuthority(t *testing.T) {
	sampling := join(fixtureNumber(1, 1), fixtureNumber(2, 64), fixtureNumber(7, 19), fixtureText(17, "priority"))
	payload := join(fixtureText(21, "model"), fixtureText(3, string(fixturePrompt(1, "hello"))),
		fixtureText(8, string(sampling)), fixtureText(27, "opaque-cache"))
	before := bytes.Clone(payload)
	for _, test := range []struct {
		body  string
		want  uint64
		valid bool
	}{
		{`{}`, 64, true}, {`{"max_tokens":null}`, 64, true},
		{`{"max_tokens":7}`, 7, true}, {`{"max_completion_tokens":7}`, 7, true},
		{`{"max_tokens":7,"max_completion_tokens":7}`, 7, true},
		{`{"max_tokens":7,"max_completion_tokens":8}`, 0, false},
		{`{"max_tokens":0}`, 0, false}, {`{"max_tokens":-1}`, 0, false},
		{`{"max_tokens":2147483648}`, 0, false}, {`{"max_tokens":1.5}`, 0, false},
		{`{"max_tokens":"7"}`, 0, false}, {`{"max_tokens":7,"max_tokens":8}`, 0, false},
	} {
		t.Run(test.body, func(t *testing.T) {
			changed, err := ApplyNativeOutputLimit(payload, []byte(test.body))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
			if !bytes.Equal(payload, before) {
				t.Fatal("retained bridge payload mutated")
			}
			if !test.valid {
				return
			}
			fields, _ := readMessage(changed, nil)
			config, _ := readMessage(fields.data(8), nil)
			if config.number(2) != test.want || config.number(7) != 19 || config.text(17) != "priority" {
				t.Fatal("native sampling changed", config)
			}
			old, _ := RawFields(payload)
			next, _ := RawFields(changed)
			for number, value := range old {
				if number != 8 && !bytes.Equal(value, next[number]) {
					t.Fatal("opaque envelope field changed", number)
				}
			}
		})
	}
	without := join(fixtureText(21, "model"), fixtureText(3, string(fixturePrompt(1, "hello"))))
	added, err := ApplyNativeOutputLimit(without, []byte(`{"max_tokens":7}`))
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(added, nil)
	config, _ := readMessage(fields.data(8), nil)
	if config.number(2) != 7 || len(config) != 1 {
		t.Fatal("missing sampling limit was not set precisely")
	}
}
