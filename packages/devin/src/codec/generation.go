package devin

import (
	"encoding/binary"
	"fmt"
	"math"
)

func decodeSampling(payload []byte, output map[string]any) error {
	configuration, err := readMessage(payload, map[int]int{
		1: 0, 2: 0, 3: 0, 4: 1, 5: 1, 6: 1, 7: 0, 8: 1,
		9: textField | repeated, 10: 0, 11: 1, 12: 0, 13: 0,
		14: 0, 15: 0, 16: 0, 17: textField, 18: textField,
		19: textField, 20: textField,
	})
	if err != nil {
		return err
	}
	if configuration.number(1) > 1 {
		return fmt.Errorf("multiple native completions are unsupported")
	}
	if configuration.number(1) == 1 {
		output["n"] = 1
	}
	if len(configuration[2]) > 0 {
		tokens := configuration.number(2)
		if tokens == 0 || tokens > math.MaxInt32 {
			return fmt.Errorf("invalid max tokens")
		}
		output["max_tokens"] = tokens
	}
	for _, control := range []struct {
		field int
		name  string
		upper float64
	}{{5, "temperature", 2}, {8, "top_p", 1}} {
		if len(configuration[control.field]) == 0 {
			continue
		}
		value := math.Float64frombits(binary.LittleEndian.Uint64(configuration.data(control.field)))
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > control.upper {
			return fmt.Errorf("invalid sampling value")
		}
		output[control.name] = value
	}
	if len(configuration[9]) > 256 {
		return fmt.Errorf("too many native stop patterns")
	}
	if len(configuration[9]) > 0 {
		patterns := make([]string, 0, len(configuration[9]))
		for _, pattern := range configuration[9] {
			patterns = append(patterns, string(pattern.b))
		}
		output["stop"] = patterns
	}
	if len(configuration[10]) > 0 {
		seed := configuration.number(10)
		if seed > math.MaxInt64 {
			return fmt.Errorf("native seed exceeds the canonical integer range")
		}
		output["seed"] = int64(seed)
	}
	if tier := configuration.text(17); tier != "" {
		output["service_tier"] = tier
	}
	for _, control := range []struct {
		field int
		name  string
	}{
		{3, "max_newlines"}, {7, "top_k"}, {12, "use_fim_eot_threshold"},
		{13, "do_not_score_stop_tokens"}, {14, "sqrt_len_normalized_log_prob_score"},
		{15, "last_message_is_partial"}, {16, "return_logprob"},
	} {
		if configuration.number(control.field) != 0 {
			return fmt.Errorf("unsupported native completion control: %s", control.name)
		}
	}
	for _, control := range []struct {
		field int
		name  string
	}{{4, "min_log_probability"}, {6, "first_temperature"}, {11, "fim_eot_prob_threshold"}} {
		if len(configuration[control.field]) > 0 {
			value := math.Float64frombits(binary.LittleEndian.Uint64(configuration.data(control.field)))
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("invalid native completion control: %s", control.name)
			}
			if value != 0 {
				return fmt.Errorf("unsupported native completion control: %s", control.name)
			}
		}
	}
	for _, control := range []struct {
		field int
		name  string
	}{{18, "response_format_type"}, {19, "response_format_json_schema"}, {20, "text_verbosity"}} {
		if configuration.text(control.field) != "" {
			return fmt.Errorf("unsupported native completion control: %s", control.name)
		}
	}
	return nil
}
