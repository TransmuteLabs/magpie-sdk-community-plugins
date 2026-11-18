package zcode

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestCompletionUsageJSONPreservesUncachedInputAndCacheBreakdown(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.answer = `{"type":"message","model":"GLM-5.3","content":[],"usage":{"input_tokens":7,"output_tokens":5,"cache_read_input_tokens":11,
		"cache_creation_input_tokens":3,"cache_creation":{"ephemeral_5m_input_tokens":1,"ephemeral_1h_input_tokens":2}}}`
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	frame, body := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
	var result sdk.ExecuteResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Usage == nil ||
		result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 5 || result.Usage.CachedTokens != 11 ||
		result.Usage.CacheWriteTokens != 3 || !strings.Contains(string(body), `"ephemeral_1h_input_tokens":2`) {
		t.Fatalf("normalized execution usage lost raw cache fidelity or invented inclusive input: %+v %s", result.Usage, body)
	}
}

func TestCompletionUsageStreamUsesCumulativeTotalsAndRetainsPartialObservation(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "truncated"}[complete], func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.media = "text/event-stream"
			vendor.answer = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"model\":\"GLM-5.3\",\"content\":[],\"usage\":{\"input_tokens\":7,\"output_tokens\":0,\"cache_read_input_tokens\":11,\"cache_creation_input_tokens\":3}}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n\n"
			if complete {
				vendor.answer += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			}
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
			if complete && frame.Error != nil || !complete && (frame.Error == nil || frame.Error.Outcome != "uncertain") {
				t.Fatalf("stream completion/uncertainty changed: %+v", frame.Error)
			}
			if len(h.usage) == 0 {
				t.Fatal("stream emitted no SDK usage observation for the ordinary ledger")
			}
			last := h.usage[len(h.usage)-1]
			if last.InputTokens != 7 || last.OutputTokens != 5 || last.CachedTokens != 11 || last.CacheWriteTokens != 3 {
				t.Fatalf("stream totals were added twice or lost at truncation: %+v", last)
			}
			if complete {
				var result sdk.ExecuteResult
				if json.Unmarshal(frame.Result, &result) != nil || result.Usage == nil || *result.Usage != last {
					t.Fatal("terminal receipt differs from the last stream usage observation")
				}
			}
		})
	}
}

func TestCompletionUsageMissingAndMalformedNeverBecomeMeasuredZero(t *testing.T) {
	for _, usage := range []string{`null`, `{}`, `{"input_tokens":-1}`, `{"input_tokens":1.5}`,
		`{"input_tokens":"7"}`, `{"input_tokens":9223372036854775808}`} {
		t.Run(usage, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.answer = `{"type":"message","content":[],"usage":` + usage + `}`
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
			var result sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Usage != nil || len(h.usage) != 0 {
				t.Fatalf("unknown/malformed counters were fabricated as a measurement: %+v %+v", frame.Error, result.Usage)
			}
		})
	}
}
