package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

type originalFailure interface {
	OriginalFailure() FailureFacts
}

func TestFailureFactsConnectorOuterRefusal(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test, "generate_image")
	fixture.respond = func(context.Context, string, map[string]any) (any, error) {
		return map[string]any{
			"is_error":      true,
			"error_code":    "connector_tool_error",
			"error_message": "selected connector denied this operation",
			"result_json":   `{"code":1008,"message":"unrelated nested result"}`,
		}, nil
	}
	_, err := engine.runTool(context.Background(), "generate_image", map[string]any{"requests": []any{map[string]any{"prompt": "fixture"}}})
	facts := requireOriginalFailure(test, err)
	if facts.Code != "connector_tool_error" || facts.Message != "selected connector denied this operation" || facts.Status != 0 || facts.TrustedEndpoint || len(fixture.calls) != 2 || *accepted != 1 {
		test.Fatalf("outer connector facts lost or fabricated transport: %+v calls=%d accepted=%d", facts, len(fixture.calls), *accepted)
	}
}

func requireOriginalFailure(test *testing.T, err error) FailureFacts {
	test.Helper()
	var carrier originalFailure
	if err == nil || !errors.As(err, &carrier) {
		test.Fatalf("original vendor failure facts were discarded: %v", err)
	}
	return carrier.OriginalFailure()
}

func TestFailureFactsCheckedJSON(test *testing.T) {
	for _, fixture := range []struct {
		name     string
		status   int
		body     string
		code     string
		message  string
		complete bool
	}{
		{"permission", 403, `{"error":{"type":"permission_error","code":"denied","message":"insufficient permissions"}}`, "denied", "insufficient permissions", true},
		{"credit", 200, `{"base_resp":{"status_code":1008,"status_msg":"积分余额不足"}}`, "1008", "积分余额不足", true},
		{"account", 200, `{"statusInfo":{"code":1000048,"message":"selected token refused"}}`, "1000048", "selected token refused", true},
		{"unknown", 200, `{"code":2013,"message":"vendor business refusal"}`, "2013", "vendor business refusal", true},
		{"fractional", 200, `{"base_resp":{"status_code":1008.5,"status_msg":"quota exhausted"}}`, "", "quota exhausted", false},
		{"invalid_json", 403, `{"error":{"message":"quota exhausted"}`, "", "", false},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			_, err := checkedJSON(ByteResponse{Status: fixture.status, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"7200"}, "X-Request-Id": {"fixture-request"}}, Body: []byte(fixture.body)})
			facts := requireOriginalFailure(test, err)
			if facts.Status != fixture.status || facts.Code != fixture.code || facts.Message != fixture.message || facts.BodyComplete != fixture.complete || facts.RetryAfter != "7200" || facts.RequestID != "fixture-request" {
				test.Fatalf("original facts changed: %+v", facts)
			}
			kept := requireOriginalFailure(test, safeError(err))
			if kept != facts {
				test.Fatal("safe error conversion discarded original facts")
			}
		})
	}
}

func TestFailureFactsTimingKnownness(test *testing.T) {
	for _, fixture := range []struct {
		name   string
		header []string
		state  string
		delay  float64
		reset  string
	}{
		{"absent", nil, "absent", 0, ""},
		{"seconds", []string{"42"}, "delay", 42000, ""},
		{"hours", []string{"7200"}, "delay", 7200000, ""},
		{"zero", []string{"0"}, "delay", 0, ""},
		{"date", []string{"Sun, 15 Nov 2026 10:00:00 GMT"}, "reset", 0, "2026-11-15T10:00:00Z"},
		{"negative", []string{"-1"}, "invalid", 0, ""},
		{"overflow", []string{"9999999999999999999999"}, "invalid", 0, ""},
		{"duplicate", []string{"42", "90"}, "invalid", 0, ""},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			facts := refusalFacts(429, http.Header{"Retry-After": fixture.header}, []byte(`{"message":"limited"}`), true)
			encoded, err := json.Marshal(facts)
			if err != nil {
				test.Fatal(err)
			}
			var fields map[string]any
			if json.Unmarshal(encoded, &fields) != nil {
				test.Fatal("facts cannot be inspected")
			}
			timing, ok := fields["Timing"].(map[string]any)
			if !ok || timing["State"] != fixture.state {
				test.Fatalf("timing knownness lost: %s", encoded)
			}
			if fixture.state == "delay" && timing["DelayMS"] != fixture.delay || fixture.state == "reset" && timing["ResetAt"] != fixture.reset {
				test.Fatalf("vendor timing changed: %s", encoded)
			}
		})
	}
}
