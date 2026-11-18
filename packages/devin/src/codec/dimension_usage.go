package devin

import (
	"encoding/binary"
	"math"
	"strings"
)

const (
	InputCount uint8 = 1 << iota
	OutputCount
	CacheReadCount
	CacheWriteCount
	AllCounts = InputCount | OutputCount | CacheReadCount | CacheWriteCount
)

func (usage *NativeUsage) Measured(counts uint8) bool {
	return usage != nil && (usage.dimensions == 0 || usage.dimensions&counts == counts)
}

// Field28 ResponseDimensionGroups are informational except for the exact
// Token Usage keys below. Field7 retains its existing complete-snapshot
// contract; these sparse values never replace a field7 observation.
func dimensionUsage(groups []field) (*NativeUsage, error) {
	result := &NativeUsage{}
	failure := &NativeError{Code: "data_loss"}
	for _, raw := range groups {
		group, err := projectedMessage(raw.b, map[int]int{1: textField, 2: 2 | repeated})
		if err != nil {
			return nil, failure
		}
		if !strings.EqualFold(group.text(1), "Token Usage") {
			continue
		}
		for _, rawMetric := range group[2] {
			metric, err := projectedMessage(rawMetric.b, map[int]int{4: 2, 5: textField})
			if err != nil {
				return nil, failure
			}
			var target *uint64
			var bit uint8
			switch strings.ToLower(metric.text(5)) {
			case "input_tokens":
				target, bit = &result.Input, InputCount
			case "output_tokens":
				target, bit = &result.Output, OutputCount
			case "cached_input_tokens", "cache_read_input_tokens":
				target, bit = &result.CacheRead, CacheReadCount
			case "cache_creation_input_tokens":
				target, bit = &result.CacheWrite, CacheWriteCount
			default:
				continue
			}
			dimension, err := projectedMessage(metric.data(4), map[int]int{2: 5})
			if err != nil || len(dimension[2]) != 1 {
				return nil, failure
			}
			value := float64(math.Float32frombits(binary.LittleEndian.Uint32(dimension.data(2))))
			// Retain the package's native token range and exact integral counts.
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > math.MaxInt32 || math.Trunc(value) != value {
				return nil, failure
			}
			*target = uint64(value)
			result.dimensions |= bit
		}
	}
	if result.dimensions == 0 {
		return nil, nil
	}
	return result, nil
}

func mergeUsage(previous, update *NativeUsage) *NativeUsage {
	if update == nil {
		return previous
	}
	if previous == nil || update.dimensions == 0 {
		copy := *update
		return &copy
	}
	if previous.dimensions == 0 {
		return previous
	}
	copy := *previous
	for _, field := range []struct {
		bit    uint8
		value  uint64
		target *uint64
	}{
		{InputCount, update.Input, &copy.Input}, {OutputCount, update.Output, &copy.Output},
		{CacheReadCount, update.CacheRead, &copy.CacheRead}, {CacheWriteCount, update.CacheWrite, &copy.CacheWrite},
	} {
		if update.dimensions&field.bit != 0 {
			*field.target = field.value
		}
	}
	copy.dimensions |= update.dimensions
	return &copy
}
