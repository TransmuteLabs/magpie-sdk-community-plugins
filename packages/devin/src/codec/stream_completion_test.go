package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"testing"
)

func TestNativeRawStreamRequiresSuccessfulCompletion(t *testing.T) {
	content := fixtureText(3, "answer")
	stop := func(reason uint64) []byte {
		return join(content, fixtureNumber(5, reason))
	}
	tool := fixtureText(6, string(join(fixtureText(1, "call"), fixtureText(2, "read"), fixtureText(3, "{}"))))
	for name, parts := range map[string][][]byte{
		"empty":                  nil,
		"usage only":             {fixtureText(7, string(fixtureNumber(2, 1))), fixtureNumber(5, 2)},
		"signature only":         {fixtureText(10, "unearned-signature"), fixtureNumber(5, 2)},
		"missing stop":           {content},
		"unspecified stop":       {stop(0)},
		"incomplete stop":        {stop(1)},
		"nonfinite stop":         {stop(7)},
		"partial stop":           {stop(9)},
		"tool stop without tool": {stop(10)},
		"empty tool delta":       {fixtureText(6, ""), fixtureNumber(5, 10)},
		"content filter":         {stop(11)},
		"non insertion":          {stop(12)},
		"error stop":             {stop(13)},
		"unknown stop":           {stop(14)},
		"changed stop":           {stop(2), fixtureNumber(5, 3)},
		"late text":              {stop(2), content},
		"late thinking":          {stop(2), fixtureText(9, "late")},
		"late tool":              {stop(2), tool},
		"unknown response":       {join(stop(2), fixtureNumber(31, 1))},
		"unknown tool control":   {join(tool, fixtureText(6, string(fixtureNumber(7, 1))), fixtureNumber(5, 10))},
		"unknown usage control":  {join(stop(2), fixtureText(7, string(fixtureNumber(14, 1))))},
		"provider refusal":       {join(stop(2), fixtureText(7, string(fixtureNumber(12, 1))))},
	} {
		t.Run(name, func(t *testing.T) {
			success := false
			_, err := ReadNativeRawStream(t.Context(), bytes.NewReader(nativeStreamFixture(parts...)), false,
				func(frame NativeFrame) error {
					if frame.Flags&2 != 0 {
						success = true
					}
					return nil
				})
			if err == nil || success {
				t.Fatal("invalid native completion emitted success or could persist origin receipts", err)
			}
		})
	}
}

func TestNativeRawStreamPreservesKnownStopsAndNativeOnlyBytes(t *testing.T) {
	for _, reason := range []uint64{2, 3, 4, 5, 6, 8, 10} {
		t.Run(strconv.FormatUint(reason, 10), func(t *testing.T) {
			payload := join(fixtureText(3, "answer"), fixtureText(10, "signature"),
				fixtureText(20, "opaque-gemini"), fixtureText(25, "analysis"))
			if reason == 10 {
				payload = append(payload, fixtureText(6, string(join(fixtureText(1, "call"),
					fixtureText(2, "custom"), fixtureText(3, "native grammar"), fixtureNumber(6, 1))))...)
			}
			payload = append(payload, fixtureNumber(5, reason)...)
			payload = binary.AppendUvarint(payload, uint64(22<<3|1))
			payload = binary.LittleEndian.AppendUint64(payload, 0x3ff0000000000000)
			usage := fixtureText(7, string(join(fixtureNumber(2, 1), fixtureNumber(3, 0),
				fixtureNumber(6, 403), fixtureNumber(13, 1))))
			var frames []NativeFrame
			summary, err := ReadNativeRawStream(t.Context(), bytes.NewReader(nativeStreamFixture(payload, usage)), false,
				func(frame NativeFrame) error { frames = append(frames, frame); return nil })
			if err != nil || summary.Stop != reason || summary.Usage == nil || summary.Usage.Output != 0 ||
				len(frames) != 3 || !bytes.Equal(frames[0].Payload, payload) || !bytes.Equal(frames[1].Payload, usage) {
				t.Fatal("known native stop, opaque payload or explicit zero usage changed", err)
			}
		})
	}
}

type canceledFrameReader struct {
	cancel context.CancelFunc
	err    error
}

func (reader canceledFrameReader) Read([]byte) (int, error) {
	reader.cancel()
	return 0, reader.err
}

func TestNativeRawStreamKeepsCancellationAfterReadFailure(t *testing.T) {
	for _, readErr := range []error{io.EOF, io.ErrClosedPipe} {
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		_, err := ReadNativeRawStream(ctx, canceledFrameReader{cancel: cancel, err: readErr}, false,
			func(NativeFrame) error { t.Fatal("canceled stream emitted a frame"); return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatal("canceled native read lost context identity", err)
		}
	}
}

func TestCanonicalNativeResponseUsesGeneratedCostWire(t *testing.T) {
	payload := join(fixtureText(3, "answer"), fixtureNumber(5, 2))
	payload = binary.AppendUvarint(payload, uint64(22<<3|1))
	payload = binary.LittleEndian.AppendUint64(payload, 0x3ff0000000000000)
	if _, err := DecodeNativeDelta(payload); err != nil {
		t.Fatal("generated committed_acu_cost double was refused", err)
	}
}

func TestCanonicalNativeResponseRefusesUnknownControls(t *testing.T) {
	for name, extra := range map[string][]byte{
		"unknown tool": fixtureText(6, string(join(fixtureText(1, "call"), fixtureText(2, "read"),
			fixtureText(3, "{}"), fixtureNumber(7, 1)))),
		"unknown usage":    fixtureText(7, string(fixtureNumber(14, 1))),
		"provider refusal": fixtureText(7, string(fixtureNumber(12, 1))),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeNativeDelta(join(fixtureText(3, "answer"), extra)); err == nil {
				t.Fatal("canonical reply silently lost native control")
			}
		})
	}
}
