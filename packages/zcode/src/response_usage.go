package zcode

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"unicode"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type executionUsage struct {
	totals sdk.Usage
	known  bool
	rich   bool
}

func (usage *executionUsage) value() *sdk.Usage {
	if !usage.known {
		return nil
	}
	return usage.totals.Clone()
}

func (usage *executionUsage) observe(body []byte) bool {
	root, err := protocol.ReadObject(body)
	if err != nil {
		return false
	}
	if usage.rich {
		return usage.observeFacts(root)
	}
	switch root["type"] {
	case "message_start":
		root = mapObject(root["message"])
	case "message", "message_delta":
	default:
		return false
	}
	counters := mapObject(root["usage"])
	if counters == nil {
		return false
	}
	next, known := usage.totals, false
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"input_tokens", &next.InputTokens},
		{"output_tokens", &next.OutputTokens},
		{"cache_read_input_tokens", &next.CachedTokens},
		{"cache_creation_input_tokens", &next.CacheWriteTokens},
	} {
		raw, exists := counters[field.name]
		if !exists {
			continue
		}
		number, valid := raw.(json.Number)
		count, err := number.Int64()
		if !valid || err != nil || count < 0 {
			return false
		}
		*field.value, known = max(*field.value, count), true
	}
	if _, present := counters["cache_creation_input_tokens"]; !present && counters["cache_creation"] != nil {
		parts := mapObject(counters["cache_creation"])
		total := int64(0)
		for _, name := range []string{"ephemeral_5m_input_tokens", "ephemeral_1h_input_tokens"} {
			if raw, present := parts[name]; present {
				number, valid := raw.(json.Number)
				count, err := number.Int64()
				if !valid || err != nil || count < 0 || count > math.MaxInt64-total {
					return false
				}
				total, known = total+count, true
			}
		}
		next.CacheWriteTokens = max(next.CacheWriteTokens, total)
	}
	if !known {
		return false
	}
	usage.totals, usage.known = next, true
	return true
}

func (usage *executionUsage) observeHeaders(headers http.Header) {
	if !usage.rich {
		return
	}
	for _, name := range []string{"Request-Id", "X-Request-Id"} {
		if value := executionText(headers.Get(name)); value != nil && *value != "" {
			if usage.totals.Facts == nil {
				usage.totals.Facts = &sdk.ExecutionUsageFacts{}
			}
			usage.totals.Facts.RequestID, usage.known = value, true
		}
	}
}

func (usage *executionUsage) observeFacts(root map[string]any) bool {
	next := usage.totals.Clone()
	if next.Facts == nil {
		next.Facts = &sdk.ExecutionUsageFacts{}
	}
	facts, known := next.Facts, false
	if root["type"] == "message_start" {
		root = mapObject(root["message"])
	}
	switch root["type"] {
	case "message", "message_delta", "error":
	default:
		if root["error"] == nil && root["code"] == nil && root["success"] != false {
			return false
		}
	}
	if counters := mapObject(root["usage"]); counters != nil {
		for _, field := range []struct {
			name  string
			value **int64
		}{
			{"input_tokens", &facts.InputTokens}, {"output_tokens", &facts.OutputTokens},
			{"cache_read_input_tokens", &facts.CachedTokens},
			{"cache_creation_input_tokens", &facts.CacheWriteTokens},
		} {
			if raw, present := counters[field.name]; present {
				count, valid := executionCount(raw)
				if !valid {
					return false
				}
				*field.value, known = count, true
				if field.name == "cache_creation_input_tokens" {
					facts.CacheWrite1hTokens = nil
				}
			}
		}
		parts := mapObject(counters["cache_creation"])
		var short, long *int64
		for _, field := range []struct {
			name  string
			value **int64
		}{{"ephemeral_5m_input_tokens", &short}, {"ephemeral_1h_input_tokens", &long}} {
			if raw, present := parts[field.name]; present {
				count, valid := executionCount(raw)
				if !valid {
					return false
				}
				*field.value = count
			}
		}
		if _, present := counters["cache_creation_input_tokens"]; !present && short != nil && long != nil {
			total := *short + *long
			if total > 1<<53-1 {
				return false
			}
			facts.CacheWriteTokens, facts.CacheWrite1hTokens, known = &total, long, true
		} else if long != nil && facts.CacheWriteTokens != nil {
			if *long > *facts.CacheWriteTokens || short != nil && *short > *facts.CacheWriteTokens-*long {
				return false
			}
			facts.CacheWrite1hTokens, known = long, true
		}
		for _, source := range []map[string]any{counters, mapObject(counters["completion_tokens_details"]),
			mapObject(counters["output_tokens_details"])} {
			if raw, present := source["reasoning_tokens"]; present {
				count, valid := executionCount(raw)
				if !valid {
					return false
				}
				facts.ReasoningTokens, known = count, true
				break
			}
		}
		if tier := executionText(counters["service_tier"]); tier != nil {
			facts.Tier, known = tier, true
		}
	}
	for _, field := range []struct {
		raw   any
		value **string
	}{
		{root["model"], &facts.ServedModel},
		{root["stop_reason"], &facts.Stop}, {mapObject(root["delta"])["stop_reason"], &facts.Stop},
		{mapObject(root["error"])["type"], &facts.ErrorType},
	} {
		if value := executionText(field.raw); value != nil {
			*field.value, known = value, true
		}
	}
	if root["type"] == "message" && root["error"] == nil && root["success"] != false {
		if response := executionText(root["id"]); response != nil {
			facts.ResponseID, known = response, true
		}
	}
	var requestID *string
	for _, source := range []map[string]any{root, mapObject(root["error"])} {
		for _, name := range []string{"request_id", "requestId"} {
			if request := executionText(source[name]); request != nil {
				requestID = request
				break
			}
		}
		if requestID != nil {
			break
		}
	}
	if requestID == nil && facts.RequestID == nil && (root["type"] == "error" || root["error"] != nil) {
		for _, source := range []map[string]any{root, mapObject(root["error"])} {
			if requestID = executionText(source["id"]); requestID != nil {
				break
			}
		}
	}
	if requestID != nil {
		facts.RequestID, known = requestID, true
	}
	if !known {
		return false
	}
	usage.totals, usage.known = *next, true
	return true
}

func executionCount(raw any) (*int64, bool) {
	number, valid := raw.(json.Number)
	if !valid {
		return nil, false
	}
	count, err := number.Int64()
	if err != nil || count < 0 || count > 1<<53-1 {
		return nil, false
	}
	return &count, true
}

func executionText(raw any) *string {
	text, valid := raw.(string)
	if !valid || len(text) > 300 || strings.ContainsFunc(text, unicode.IsControl) {
		return nil
	}
	return &text
}
