package zcode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestObservability7IncompleteJSONKeepsActualRequestID(t *testing.T) {
	var dispatched atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("unexpected discovery request before the inference: %s", request.Method)
			return
		}
		dispatched.Add(1)
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Request-Id", "incomplete-request")
		response.Header().Set("Content-Length", "100")
		response.Write([]byte(`{"type":`))
	}))
	defer server.Close()
	h := controlHarness(t, endpointOptions(server))
	account := Credentials{Site: "zai", Kind: "legacy-key", Key: "synthetic-incomplete-key"}
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if dispatched.Load() != 1 || !h.accepted.Load() || frame.Error == nil || frame.Error.Outcome != "uncertain" || len(h.usage) != 1 ||
		h.usage[0].Facts == nil || h.usage[0].Facts.RequestID == nil || *h.usage[0].Facts.RequestID != "incomplete-request" ||
		h.usage[0].Facts.InputTokens != nil {
		t.Fatal("an incomplete JSON body lost its actual response request id or invented counts", frame.Error, h.usage)
	}
}

func TestObservability7SparseSnapshotsKeepOwnershipAndRequestIdentity(t *testing.T) {
	usage := executionUsage{rich: true}
	usage.observeHeaders(http.Header{"Request-Id": {"fallback"}, "X-Request-Id": {"header-request"}})
	if !usage.observe([]byte(`{"type":"message","id":"response-id","model":"GLM-5.3",
		"usage":{"output_tokens":5,"cache_creation_input_tokens":3,"cache_creation":{"ephemeral_1h_input_tokens":2}}}`)) {
		t.Fatal("initial source snapshot missing")
	}
	first := usage.value()
	if !usage.observe([]byte(`{"type":"message_delta","usage":{"output_tokens":0,"cache_creation_input_tokens":0}}`)) {
		t.Fatal("measured final zero missing")
	}
	last := usage.value()
	if first.Facts.OutputTokens == nil || *first.Facts.OutputTokens != 5 ||
		first.Facts.CacheWrite1hTokens == nil || *first.Facts.CacheWrite1hTokens != 2 ||
		last.Facts.OutputTokens == nil || *last.Facts.OutputTokens != 0 ||
		last.Facts.CacheWriteTokens == nil || *last.Facts.CacheWriteTokens != 0 || last.Facts.CacheWrite1hTokens != nil {
		t.Fatal("sparse snapshots alias their pointers or keep an obsolete cache subset", first, last)
	}
	if !usage.observe([]byte(`{"type":"error","id":"ambiguous-error-id","error":{"type":"overloaded_error"}}`)) {
		t.Fatal("terminal error type missing")
	}
	last = usage.value()
	if last.Facts.ResponseID == nil || *last.Facts.ResponseID != "response-id" ||
		last.Facts.RequestID == nil || *last.Facts.RequestID != "header-request" {
		t.Fatal("an ambiguous error id overwrote the actual response/request identity", last.Facts)
	}
	for _, parts := range []string{`{"ephemeral_1h_input_tokens":2}`, `{"ephemeral_5m_input_tokens":1}`} {
		sparse := executionUsage{rich: true}
		sparse.observe([]byte(`{"type":"message","usage":{"cache_creation":` + parts + `}}`))
		if sparse.value() != nil {
			t.Fatal("an incomplete cache breakdown invented a total or an absent zero")
		}
	}
}

func controlHarness(t *testing.T, options ModuleOptions) *harness {
	t.Helper()
	return newHarnessModule(t, options, moduleConfig{FreeFirst: new(false)}, NewControlModule)
}

func TestObservability7JSONKeepsOnlySourceBackedSparseFacts(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.answer = `{"type":"message","id":"observed-response","model":"GLM-5.3","content":[],"stop_reason":"end_turn",
		"usage":{"input_tokens":0,"output_tokens":5,"cache_read_input_tokens":11,"cache_creation_input_tokens":3,
		"cache_creation":{"ephemeral_5m_input_tokens":1,"ephemeral_1h_input_tokens":2},
		"completion_tokens_details":{"reasoning_tokens":0},"service_tier":"standard"}}`
	h := controlHarness(t, endpointOptions(server))
	account := giftAccount()
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	var result sdk.ExecuteResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Usage == nil || result.Usage.Facts == nil {
		t.Fatalf("source execution facts never reached the full module: error=%+v usage=%+v", frame.Error, result.Usage)
	}
	facts := result.Usage.Facts
	if facts.InputTokens == nil || *facts.InputTokens != 0 || facts.OutputTokens == nil || *facts.OutputTokens != 5 ||
		facts.CachedTokens == nil || *facts.CachedTokens != 11 || facts.CacheWriteTokens == nil || *facts.CacheWriteTokens != 3 ||
		facts.CacheWrite1hTokens == nil || *facts.CacheWrite1hTokens != 2 ||
		facts.ReasoningTokens == nil || *facts.ReasoningTokens != 0 || facts.ServedModel == nil || *facts.ServedModel != "GLM-5.3" ||
		facts.ResponseID == nil || *facts.ResponseID != "observed-response" ||
		facts.Stop == nil || *facts.Stop != "end_turn" || facts.Tier == nil || *facts.Tier != "standard" {
		t.Fatalf("sparse zero, cache TTL or vendor identity was lost: %+v", facts)
	}
	if result.Usage.InputTokens != 0 || result.Usage.OutputTokens != 0 || result.Usage.CachedTokens != 0 ||
		result.Usage.CacheWriteTokens != 0 || facts.Deferred != nil || facts.UpstreamProvider != nil ||
		facts.Cost != nil || facts.Credits != nil || facts.CacheUnknown != nil {
		t.Fatal("normalized facts invented unsupported vendor evidence or mixed legacy counts", result.Usage)
	}
}

func TestObservability7StreamRetainsFailedTerminalFactsAndMeasuredZero(t *testing.T) {
	for _, terminal := range []string{"complete", "truncated", "error", "zero"} {
		t.Run(terminal, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.media = "text/event-stream"
			vendor.answer = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"id\":\"stream-response\",\"model\":\"GLM-5.3\",\"content\":[],\"usage\":{\"input_tokens\":7,\"output_tokens\":0,\"cache_creation\":{\"ephemeral_5m_input_tokens\":1,\"ephemeral_1h_input_tokens\":2}}}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5,\"reasoning_tokens\":2}}\n\n"
			switch terminal {
			case "complete":
				vendor.answer += "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			case "error":
				vendor.answer += "event: error\ndata: {\"type\":\"error\",\"request_id\":\"failed-request\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"private-vendor-text\"}}\n\n"
			case "zero":
				vendor.answer += "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":0,\"reasoning_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			}
			h := controlHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, body := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
			failed := terminal == "truncated" || terminal == "error"
			if failed && (frame.Error == nil || frame.Error.Outcome != "uncertain") || !failed && frame.Error != nil {
				t.Fatalf("terminal uncertainty changed: %+v", frame.Error)
			}
			if len(h.usage) == 0 || h.usage[len(h.usage)-1].Facts == nil {
				t.Fatal("the full module lost its final observable usage snapshot")
			}
			last := h.usage[len(h.usage)-1]
			facts := last.Facts
			output, reasoning := int64(5), int64(2)
			if terminal == "zero" {
				output, reasoning = 0, 0
			}
			if facts.InputTokens == nil || *facts.InputTokens != 7 || facts.OutputTokens == nil || *facts.OutputTokens != output ||
				facts.ReasoningTokens == nil || *facts.ReasoningTokens != reasoning || facts.CachedTokens != nil ||
				facts.CacheWriteTokens == nil || *facts.CacheWriteTokens != 3 ||
				facts.CacheWrite1hTokens == nil || *facts.CacheWrite1hTokens != 2 ||
				facts.ServedModel == nil || *facts.ServedModel != "GLM-5.3" ||
				facts.ResponseID == nil || *facts.ResponseID != "stream-response" {
				t.Fatalf("last source snapshot lost measured facts or invented absent cache reads: %+v", facts)
			}
			if terminal == "error" && (facts.ErrorType == nil || *facts.ErrorType != "overloaded_error" ||
				facts.RequestID == nil || *facts.RequestID != "failed-request" || strings.Contains(string(body), "private-vendor-text")) {
				t.Fatal("terminal error lost bounded vendor evidence or leaked its text", facts)
			}
			if !failed {
				var result sdk.ExecuteResult
				if json.Unmarshal(frame.Result, &result) != nil || !reflect.DeepEqual(result.Usage, &last) {
					t.Fatal("terminal result differs from the final event snapshot")
				}
			}
		})
	}
}

func TestObservability7UnsupportedCurrencyCreditsAndMalformedCountsStayUnknown(t *testing.T) {
	for _, currency := range []string{"USD", "EUR", "CNY", "credits"} {
		t.Run(currency, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.answer = `{"type":"message","content":[],"usage":{"input_tokens":0,
				"cost":12,"cost_units":3,"credits":4,"currency":"` + currency + `"}}`
			h := controlHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, body := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			var result sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Usage == nil ||
				result.Usage.Facts == nil || result.Usage.Facts.InputTokens == nil || *result.Usage.Facts.InputTokens != 0 {
				t.Fatal("measured zero disappeared", result.Usage, frame.Error)
			}
			facts := result.Usage.Facts
			if facts.Credits != nil || facts.Cost != nil || facts.OutputTokens != nil || result.Usage.Cost != 0 ||
				result.Usage.Currency != "" || !strings.Contains(string(body), `"currency":"`+currency+`"`) {
				t.Fatal("unsupported monetary fields became authoritative accounting or changed vendor bytes", result.Usage)
			}
		})
	}
	for _, counters := range []string{`{"input_tokens":-1}`, `{"input_tokens":1.5}`,
		`{"input_tokens":9007199254740992}`, `{"cache_creation_input_tokens":2,"cache_creation":{"ephemeral_1h_input_tokens":3}}`} {
		t.Run(counters, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.answer = `{"type":"message","content":[],"usage":` + counters + `}`
			h := controlHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			var result sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Usage != nil || len(h.usage) != 0 {
				t.Fatal("malformed source counts became measured zero", result.Usage, frame.Error)
			}
		})
	}
}

func TestObservability7DiagnosticsAreScopedCodesAndNeverChangeExecution(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "recorded", true: "sink-failed"}[reject], func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.answer = `{"type":"message","content":[{"type":"text","text":"private-output-canary"}],"usage":{"input_tokens":0}}`
			h := controlHarness(t, endpointOptions(server))
			h.rejectDiagnostics = reject
			account := giftAccount()
			request := giftRequest("GLM-5.3")
			request.Body = []byte(`{"model":"GLM-5.3","messages":[{"role":"user","content":"private-input-canary"}]}`)
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
			if frame.Error != nil || len(h.diagnostics) != 1 {
				t.Fatal("scoped execution diagnostic is missing or changed its outcome", frame.Error, h.diagnostics)
			}
			diagnostic := h.diagnostics[0]
			if diagnostic.Code != "execution.completed" || diagnostic.Phase != "execute" ||
				diagnostic.Level != "info" || diagnostic.Status != 200 {
				t.Fatal("execution diagnostic has no finite outcome/status", diagnostic)
			}
			for _, body := range h.diagnosticParams {
				if strings.Contains(string(body), "private-") || strings.Contains(string(body), "synthetic-") {
					t.Fatal("diagnostic copied vendor, request or credential text", string(body))
				}
				var fields map[string]json.RawMessage
				json.Unmarshal(body, &fields)
				for name := range fields {
					if !map[string]bool{"level": true, "code": true, "phase": true, "status": true,
						"millis": true, "count": true, "retryable": true}[name] {
						t.Fatal("diagnostic invented caller identity or unstructured fields", name)
					}
				}
			}
		})
	}
}

func TestObservability7FailedDiagnosticsKeepOnlyFiniteOutcomes(t *testing.T) {
	for _, terminal := range []string{"rejected", "uncertain"} {
		t.Run(terminal, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.answer = `{"type":"error","error":{"type":"overloaded_error","message":"private-error-canary"}}`
			code, status := "execution.rejected", 503
			if terminal == "uncertain" {
				vendor.media = "text/event-stream"
				vendor.answer = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"content\":[],\"usage\":{\"input_tokens\":0}}}\n\n"
				code, status = "execute.uncertain", 0
			}
			h := controlHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			if terminal == "uncertain" && (frame.Error == nil || frame.Error.Outcome != "uncertain") ||
				terminal == "rejected" && frame.Error != nil {
				t.Fatal("diagnostics changed terminal outcome", frame.Error)
			}
			if len(h.diagnostics) != 1 || h.diagnostics[0].Level != "warn" ||
				h.diagnostics[0].Code != code || h.diagnostics[0].Status != status ||
				h.diagnostics[0].Retryable != nil {
				t.Fatal("failed execution has no bounded, source-backed diagnostic", h.diagnostics)
			}
			if strings.Contains(string(h.diagnosticParams[0]), "private-") {
				t.Fatal("diagnostic copied vendor error text")
			}
		})
	}
}
