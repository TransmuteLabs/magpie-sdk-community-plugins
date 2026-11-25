package devin

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"testing"
)

func TestNativeStreamParsesEndStreamErrorTrailerAfterGzipDataFrame(t *testing.T) {
	dataPayload := join(fixtureText(3, "partial output"))
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(dataPayload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	dataFrame := make([]byte, 5+compressed.Len())
	dataFrame[0] = 1
	binary.BigEndian.PutUint32(dataFrame[1:5], uint32(compressed.Len()))
	copy(dataFrame[5:], compressed.Bytes())

	trailerJSON := []byte(`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"}}`)
	trailerFrame := make([]byte, 5+len(trailerJSON))
	trailerFrame[0] = 2
	binary.BigEndian.PutUint32(trailerFrame[1:5], uint32(len(trailerJSON)))
	copy(trailerFrame[5:], trailerJSON)

	var deltas []NativeDelta
	_, err := ReadNativeStream(t.Context(), bytes.NewReader(join(dataFrame, trailerFrame)), true, func(delta NativeDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	var nativeErr *NativeError
	if !errors.As(err, &nativeErr) || nativeErr.Code != "resource_exhausted" ||
		nativeErr.Failure == nil || !nativeErr.Failure.Complete || nativeErr.BeforeEffect {
		t.Fatalf("expected post-data resource_exhausted trailer error with BeforeEffect=false, got err=%v (%+v)", err, nativeErr)
	}
	if len(deltas) != 1 || deltas[0].Text != "partial output" {
		t.Fatalf("expected gzip data delta before trailer error, got %+v", deltas)
	}
	flat := ParseResponseFailure([]byte(`{"code":"unauthenticated","message":"bad token"}`), false)
	if flat == nil || !flat.Complete || flat.Code != "unauthenticated" {
		t.Fatalf("flat JSON error failed to parse: %+v", flat)
	}
}
