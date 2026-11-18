package minimax

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func chatRefusal(response ByteResponse, facts *FailureFacts) (sdk.ExecuteResult, bool) {
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if facts == nil || !facts.TrustedEndpoint || !facts.BodyComplete || facts.OutputStarted ||
		facts.CodePresent && !facts.CodeValid || err != nil || contentType != "application/json" ||
		response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" ||
		!chatRejectedBeforeEffect(response) {
		return sdk.ExecuteResult{}, false
	}
	projected := sdk.RequestFailureFacts{
		Status: facts.WireStatus, Code: facts.Code, Type: facts.Type, Message: facts.Message,
		Operation: sdk.MethodProviderExecute, Phase: "response", Complete: true,
	}
	if facts.Timing.State == "delay" || facts.Timing.State == "reset" {
		projected.Headers = map[string]string{"Retry-After": facts.Timing.Raw}
		projected.ResetAt = facts.Timing.ResetAt
		if facts.Timing.DelayMS != nil && *facts.Timing.DelayMS <= 30*24*60*60*1000 {
			projected.RetryAfterMS = facts.Timing.DelayMS
		}
	}
	failure := sdk.RequestRecoveryRequest{
		Step: "minimax-chat", Facts: projected, Evidence: "rejected-before-effect",
		Rules: []sdk.RequestFailureRule{
			{ID: "minimax-content", Type: "content_policy_error", Class: "content-refusal", Scope: "request"},
			{ID: "minimax-request", Type: "invalid_request_error", Class: "request-refusal", Scope: "request"},
			{ID: "minimax-credit-1008", Code: "1008", Class: "quota", Scope: "account"},
			{ID: "minimax-credit-2053", Code: "2053", Class: "quota", Scope: "account"},
			{ID: "minimax-auth", Type: "authentication_error", Class: "auth-refusal", Scope: "account"},
			{ID: "minimax-credit-cn", Type: "permission_error", Contains: "余额不足", Class: "quota", Scope: "account"},
			{ID: "minimax-credit-cn-points", Type: "permission_error", Contains: "积分不足", Class: "quota", Scope: "account"},
			{ID: "minimax-credit-insufficient", Type: "permission_error", Contains: "insufficient credit", Class: "quota", Scope: "account"},
			{ID: "minimax-balance-insufficient", Type: "permission_error", Contains: "insufficient balance", Class: "quota", Scope: "account"},
			{ID: "minimax-credit-exhausted", Type: "permission_error", Contains: "credit exhausted", Class: "quota", Scope: "account"},
			{ID: "minimax-permission", Type: "permission_error", Class: "request-refusal", Scope: "request"},
			{ID: "minimax-rate", Type: "rate_limit_error", Class: "transient-rate", Scope: "unknown"},
		},
	}
	return sdk.RefusedExecution(failure, nil), true
}

func chatRejectedBeforeEffect(response ByteResponse) bool {
	envelope, valid := refusalObject(response.Body, "type", "error", "base_resp", "request_id", "trace_id")
	if !valid {
		return false
	}
	for _, key := range []string{"request_id", "trace_id"} {
		if value, present := envelope[key]; present {
			var identifier string
			if json.Unmarshal(value, &identifier) != nil || bytes.Equal(value, []byte("null")) {
				return false
			}
		}
	}
	if base, present := envelope["base_resp"]; present {
		if len(envelope["error"]) != 0 || len(envelope["type"]) != 0 ||
			response.Status != http.StatusOK && response.Status != http.StatusPaymentRequired && response.Status != http.StatusForbidden {
			return false
		}
		status, valid := refusalObject(base, "status_code", "status_msg")
		if !valid {
			return false
		}
		var code any
		var message string
		if json.Unmarshal(status["status_code"], &code) != nil || json.Unmarshal(status["status_msg"], &message) != nil ||
			bytes.Equal(status["status_msg"], []byte("null")) || len(message) > 1024 {
			return false
		}
		number, valid := apiCode(code)
		return valid && (number == 1008 || number == 2053)
	}
	if response.Status < 400 || response.Status >= 500 || !bytes.Equal(envelope["type"], []byte(`"error"`)) {
		return false
	}
	refusal, valid := refusalObject(envelope["error"], "type", "code", "message")
	if !valid {
		return false
	}
	var kind, message string
	if json.Unmarshal(refusal["type"], &kind) != nil || json.Unmarshal(refusal["message"], &message) != nil ||
		bytes.Equal(refusal["message"], []byte("null")) || len(message) > 1024 {
		return false
	}
	return contains([]string{"permission_error", "authentication_error", "invalid_request_error", "content_policy_error", "rate_limit_error"}, kind)
}

func refusalObject(body []byte, allowed ...string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		name, valid := token.(string)
		if err != nil || !valid || !contains(allowed, name) || fields[name] != nil {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[name] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	return fields, decoder.Decode(new(any)) == io.EOF
}
