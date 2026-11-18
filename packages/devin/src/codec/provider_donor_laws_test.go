package devin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
)

// Fixture shapes come from CLIProxyAPI 842cedec8, not its dirty working tree.
func donorDimension(key string, value float32) []byte {
	dimension := binary.AppendUvarint(nil, 2<<3|5)
	dimension = binary.LittleEndian.AppendUint32(dimension, math.Float32bits(value))
	return addText(addBytes(nil, 4, dimension), 5, key)
}

func donorDimensionGroup(metrics ...[]byte) []byte {
	group := addText(nil, 1, "Token Usage")
	for _, metric := range metrics {
		group = addBytes(group, 2, metric)
	}
	return addBytes(nil, 28, group)
}

func TestProviderDonorDimensionUsageDoesNotDisappear(t *testing.T) {
	metrics := donorDimensionGroup(donorDimension("input_tokens", 7), donorDimension("output_tokens", 4),
		donorDimension("cached_input_tokens", 2), donorDimension("cache_creation_input_tokens", 3))
	for _, raw := range []bool{false, true} {
		wire := nativeStreamFixture(append(addNumber(addText(nil, 3, "answer"), 5, 2), metrics...))
		var summary NativeSummary
		var err error
		if raw {
			summary, err = ReadNativeRawStream(t.Context(), bytes.NewReader(wire), false, func(NativeFrame) error { return nil })
		} else {
			summary, err = ReadNativeStream(t.Context(), bytes.NewReader(wire), false, nil)
		}
		if err != nil || summary.Usage == nil || summary.Usage.Input != 7 || summary.Usage.Output != 4 ||
			summary.Usage.CacheRead != 2 || summary.Usage.CacheWrite != 3 {
			t.Fatalf("raw=%t: field28 measured counters lost: %+v error=%v", raw, summary.Usage, err)
		}
	}
}

func TestProviderDonorOverflowRefusesWithoutErasingPriorSnapshot(t *testing.T) {
	for _, count := range []uint64{math.MaxInt64, 1 << 63} {
		valid := addBytes(nil, 7, addNumber(nil, 3, 5))
		invalid := addBytes(nil, 7, addNumber(nil, 3, count))
		for _, raw := range []bool{false, true} {
			wire := nativeStreamFixture(valid, invalid)
			var summary NativeSummary
			var err error
			if raw {
				summary, err = ReadNativeRawStream(t.Context(), bytes.NewReader(wire), false, func(NativeFrame) error { return nil })
			} else {
				summary, err = ReadNativeStream(t.Context(), bytes.NewReader(wire), false, nil)
			}
			if err == nil || summary.Usage == nil || summary.Usage.Output != 5 {
				t.Fatalf("raw=%t: invalid count became success or erased measurement: %+v %v", raw, summary.Usage, err)
			}
		}
	}
}

func TestProviderDonorDimensionScopeAndSnapshotPriority(t *testing.T) {
	first, err := DecodeNativeMetrics(donorDimensionGroup(donorDimension("input_tokens", 7)))
	if err != nil || first.Usage == nil || first.Usage.Input != 7 {
		t.Fatal("dimension input was lost", first, err)
	}
	next, err := DecodeNativeMetrics(donorDimensionGroup(donorDimension("output_tokens", 0)))
	if err != nil || next.Usage == nil {
		t.Fatal("named zero was lost", next, err)
	}
	first.Merge(next)
	if first.Usage.Input != 7 || first.Usage.Output != 0 {
		t.Fatal("partial dimension update erased earlier measurement", first)
	}
	field7, err := DecodeNativeMetrics(addBytes(nil, 7, addNumber(nil, 2, 3)))
	if err != nil {
		t.Fatal(err)
	}
	first.Merge(field7)
	first.Merge(next)
	if first.Usage == nil || *first.Usage != (NativeUsage{Input: 3}) {
		t.Fatal("dimension fallback overrode authoritative proto3 snapshot", first)
	}
	unknown := []byte{}
	unknown = append(unknown, donorDimensionGroup(donorDimension("uncached_input_tokens", 9))...)
	unknown = append(unknown, addBytes(nil, 28, addBytes(addText(nil, 1, "Latency Metrics"), 2, donorDimension("input_tokens", 9)))...)
	ignored, err := DecodeNativeMetrics(unknown)
	if err != nil || ignored.Usage != nil {
		t.Fatal("unknown metric/group invented token counts", ignored, err)
	}
	alias, err := DecodeNativeMetrics(donorDimensionGroup(donorDimension("CACHE_READ_INPUT_TOKENS", 2)))
	if err != nil || alias.Usage == nil || alias.Usage.CacheRead != 2 {
		t.Fatal("explicit cache-read alias was lost", alias, err)
	}
}

func TestProviderDonorInvalidDimensionsRefuseWithoutInventingZero(t *testing.T) {
	for _, value := range []float32{-1, 1.5, float32(math.NaN()), float32(math.Inf(1)), 1 << 31} {
		if _, err := DecodeNativeMetrics(donorDimensionGroup(donorDimension("input_tokens", value))); err == nil {
			t.Fatalf("invalid dimension %v became a measured count", value)
		}
	}
	for _, metric := range [][]byte{
		addText(nil, 5, "input_tokens"),
		addText(addBytes(nil, 4, addNumber(nil, 2, 7)), 5, "input_tokens"),
		addText(append(donorDimension("input_tokens", 7), addText(nil, 5, "output_tokens")...), 5, "input_tokens"),
	} {
		if _, err := DecodeNativeMetrics(donorDimensionGroup(metric)); err == nil {
			t.Fatal("missing/malformed/duplicate dimension became usable usage")
		}
	}
}

func TestProviderDonorSamplingKeepsNullAbsenceAndExplicitZero(t *testing.T) {
	for _, protocol := range []string{"chat", "messages"} {
		for _, raw := range []string{"absent", "null", "0", "0.5", `"0.5"`, "true", "[0.5]", "1e400"} {
			t.Run(protocol+"/"+raw, func(t *testing.T) {
				body := `{"model":"swe-2-high","max_tokens":10,"messages":[{"role":"user","content":"hello"}]`
				if raw != "absent" {
					body += `,"temperature":` + raw
				}
				body += "}"
				var encoded EncodedChat
				var err error
				if protocol == "chat" {
					encoded, err = EncodeChatRequest([]byte(body), "fixture-key")
				} else {
					encoded, err = EncodeMessagesRequest([]byte(body), "fixture-key", nil)
				}
				valid := raw == "absent" || raw == "null" || raw == "0" || raw == "0.5"
				if !valid {
					if err == nil || len(encoded.Payload) != 0 {
						t.Fatal("malformed sampling was coerced or sent")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				fields, _ := readMessage(encoded.Payload, nil)
				config, _ := readMessage(fields.data(8), nil)
				want := float64(1)
				if raw == "0" {
					want = 0
				} else if raw == "0.5" {
					want = 0.5
				}
				if len(config.data(5)) != 8 || binary.LittleEndian.Uint64(config.data(5)) != math.Float64bits(want) {
					t.Fatal("sampling bits changed")
				}
			})
		}
	}
}

func TestProviderDonorSchemaAndDescriptionsStayData(t *testing.T) {
	const schema = `{"properties":{"q":{"$ref":"#/dependencies/a/1"}},"dependencies":{"a":["b",{"description":"tree tree tree tree","properties":{"ghost":{}}}]}}`
	const description = "Reads a file from the local filesystem. You can access any file directly by using this tool."
	body, _ := json.Marshal(map[string]any{"model": "swe-2-high",
		"messages": []any{map[string]string{"role": "system", "content": "You are a careful engineer."}, map[string]string{"role": "user", "content": "hello"}},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": "Read", "description": description, "parameters": json.RawMessage(schema)}}}})
	encoded, err := EncodeChatRequest(body, "fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(encoded.Payload, nil)
	tool, _ := readMessage(fields.data(10), nil)
	if tool.text(2) != description || tool.text(3) != schema || !bytes.Contains(encoded.Payload, []byte("You are a careful engineer.")) {
		t.Fatal("schema or source descriptions were rewritten")
	}
}
