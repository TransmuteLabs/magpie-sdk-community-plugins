package devin

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestNativeFactsKeepActualModelAndMessageAuthorityAcrossUsageFrames(t *testing.T) {
	first, err := DecodeNativeMetrics(addText(addText(addText(nil, 23, "actual-model"), 1, "native-message"), 17, "native-request"))
	if err != nil {
		t.Fatal(err)
	}
	usage := addText(addText(addNumber(nil, 6, 999), 9, "billing-fallback"), 7, "usage-message")
	next, err := DecodeNativeMetrics(addBytes(nil, 7, usage))
	if err != nil {
		t.Fatal(err)
	}
	first.Merge(next)
	if first.Facts.ServedModel != "actual-model" || first.Facts.ResponseID != "native-message" ||
		first.Facts.RequestID != "native-request" || first.Facts.UpstreamProvider != "API_PROVIDER_999" ||
		first.Usage == nil || *first.Usage != (NativeUsage{}) {
		t.Fatal("later usage replaced explicit native response authority", first)
	}
}

func TestNativeFactsRejectInvalidMeasuredMetadataAndCosts(t *testing.T) {
	cost := func(value float64) []byte {
		wire := binary.AppendUvarint(nil, 22<<3|1)
		return binary.LittleEndian.AppendUint64(wire, math.Float64bits(value))
	}
	for _, payload := range [][]byte{cost(math.NaN()), cost(math.Inf(1)), cost(-1),
		addNumber(nil, 18, math.MaxUint64), addText(nil, 17, "private\nrequest"),
		addBytes(nil, 23, bytes.Repeat([]byte("x"), 301))} {
		if _, err := DecodeNativeMetrics(payload); err == nil {
			t.Fatal("invalid execution fact became trusted")
		}
	}
	for _, payload := range [][]byte{cost(0), addNumber(nil, 18, 0)} {
		facts, err := DecodeNativeMetrics(payload)
		if err != nil || facts.Facts.ACUCost == nil && facts.Facts.Credits == nil {
			t.Fatal("explicit committed zero cost or credit was lost", facts, err)
		}
	}
}
