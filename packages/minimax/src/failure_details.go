package minimax

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type failureTiming struct {
	State   string
	Source  string
	Raw     string
	DelayMS *int64
	ResetAt string
}

func responseTiming(headers http.Header) failureTiming {
	values := headers.Values("Retry-After")
	if len(values) == 0 {
		return failureTiming{State: "absent"}
	}
	result := failureTiming{State: "invalid", Source: "http-header:Retry-After"}
	if len(values) != 1 || len(values[0]) > 128 {
		return result
	}
	result.Raw = factText(values[0], 128)
	value := strings.TrimSpace(values[0])
	if value != "" && strings.Trim(value, "0123456789") == "" {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err == nil && seconds <= math.MaxInt64/1000 {
			delay := int64(seconds) * 1000
			result.State, result.DelayMS = "delay", &delay
		}
		return result
	}
	if reset, err := http.ParseTime(value); err == nil {
		result.State, result.ResetAt = "reset", reset.UTC().Format(time.RFC3339)
	}
	return result
}

type failureDetails struct {
	cause error
	facts FailureFacts
}

func (failure *failureDetails) Error() string {
	return failure.cause.Error()
}

func (failure *failureDetails) Unwrap() error {
	return failure.cause
}

func (failure *failureDetails) OriginalFailure() FailureFacts {
	return failure.facts
}

func (failure *failureDetails) HTTPStatus() int {
	var status interface{ HTTPStatus() int }
	if errors.As(failure.cause, &status) {
		return status.HTTPStatus()
	}
	return 0
}

func withFailureDetails(err error, facts FailureFacts) error {
	if err == nil {
		return nil
	}
	for {
		previous, wrapped := err.(*failureDetails)
		if !wrapped {
			break
		}
		err = previous.cause
	}
	return &failureDetails{cause: err, facts: facts}
}

func responseFailure(err error, response ByteResponse, origin string) error {
	facts := refusalFacts(response.Status, response.Header, response.Body, false)
	facts.Origin = origin
	return withFailureDetails(err, facts)
}

func keepFailureDetails(replacement, original error) error {
	var carrier interface{ OriginalFailure() FailureFacts }
	if errors.As(original, &carrier) {
		return withFailureDetails(replacement, carrier.OriginalFailure())
	}
	return replacement
}

func (client *SelectedClient) selectedFailure(err error) error {
	var carrier interface{ OriginalFailure() FailureFacts }
	if !errors.As(err, &carrier) {
		return err
	}
	facts := carrier.OriginalFailure()
	facts.TrustedEndpoint = true
	return withFailureDetails(err, redactFailureFacts(facts, client.credentials))
}

func (client *SelectedClient) redactedFailure(err error) error {
	var carrier interface{ OriginalFailure() FailureFacts }
	if !errors.As(err, &carrier) {
		return err
	}
	return withFailureDetails(err, redactFailureFacts(carrier.OriginalFailure(), client.credentials))
}

func redactFailureFacts(facts FailureFacts, credentials Credentials) FailureFacts {
	for _, secret := range []string{credentials.Access, credentials.Refresh} {
		if secret == "" {
			continue
		}
		for _, target := range []*string{&facts.Code, &facts.Type, &facts.Message, &facts.RequestID, &facts.RetryAfter, &facts.Timing.Raw} {
			*target = strings.ReplaceAll(*target, secret, "<redacted>")
		}
	}
	return facts
}

func transferFailure(err error, response ByteResponse) error {
	if response.Failure != nil {
		return withFailureDetails(err, *response.Failure)
	}
	return responseFailure(err, response, "media-transfer")
}

func prepareChatResponse(response ByteResponse, headers http.Header, credentials Credentials) ByteResponse {
	result := ByteResponse{Status: response.Status, Header: headers.Clone(), Body: bytes.Clone(response.Body)}
	businessRefusal := false
	if result.Status >= 200 && result.Status < 300 {
		if _, err := checkedJSON(response); err != nil {
			var refusal *APIError
			result.Status = 502
			if errors.As(err, &refusal) && refusal.Status >= 400 && refusal.Status <= 599 {
				result.Status = refusal.Status
			}
			businessRefusal = true
		}
	}
	if result.Status < 400 {
		return result
	}
	facts := redactFailureFacts(refusalFacts(response.Status, response.Header, response.Body, true), credentials)
	if businessRefusal {
		facts.Origin = "business-envelope"
	}
	for _, secret := range []string{credentials.Access, credentials.Refresh} {
		if secret != "" {
			result.Body = bytes.ReplaceAll(result.Body, []byte(secret), []byte("<redacted>"))
		}
	}
	classification := facts
	classification.Status = result.Status
	if classification.accountQuota() {
		result.Status = 429
		result.Body = raw(map[string]any{"type": "error", "error": map[string]string{"type": "rate_limit_error", "message": "MiniMax usage limit reached"}})
		result.Header.Set("Content-Type", "application/json")
		result.Header.Del("Content-Encoding")
	} else if businessRefusal {
		message := facts.Message
		if message == "" {
			message = "MiniMax returned an unsuccessful business response"
		}
		result.Body = raw(map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": message}})
		result.Header.Set("Content-Type", "application/json")
		result.Header.Del("Content-Encoding")
	}
	facts.EffectiveStatus = result.Status
	result.Failure = &facts
	return result
}

func connectorResult(response ByteResponse) (any, error) {
	var envelope map[string]any
	if json.Unmarshal(response.Body, &envelope) != nil || envelope == nil {
		return nil, responseFailure(toolError("Malformed connector call result"), response, "connector-envelope")
	}
	if value, exists := envelope["is_error"]; exists {
		flag, valid := value.(bool)
		if !valid {
			return nil, responseFailure(toolError("Malformed connector error flag"), response, "connector-envelope")
		}
		if flag {
			details := map[string]any{}
			if code, present := envelope["error_code"]; present {
				details["code"] = code
			}
			if message, present := envelope["error_message"]; present {
				details["message"] = message
			}
			if len(details) != 0 {
				response.Body = raw(details)
				return nil, responseFailure(toolError("Connector tool reported a business failure"), response, "connector-envelope")
			}
			if encoded, valid := envelope["result_json"].(string); valid && strings.TrimSpace(encoded) != "" {
				response.Body = []byte(encoded)
				return nil, responseFailure(toolError("Connector tool reported a business failure"), response, "connector-result")
			}
			return nil, responseFailure(toolError("Connector tool reported a business failure"), response, "connector-envelope")
		}
	}
	encoded, valid := envelope["result_json"].(string)
	if !valid || strings.TrimSpace(encoded) == "" {
		return nil, responseFailure(toolError("Connector call has no result_json"), response, "connector-envelope")
	}
	var result any
	if json.Unmarshal([]byte(encoded), &result) != nil {
		response.Body = []byte(encoded)
		return nil, responseFailure(toolError("Connector result_json is invalid JSON"), response, "connector-result")
	}
	if fields := object(result); fields != nil {
		if code, valid := number(fields["code"]); valid && code != 0 {
			response.Body = []byte(encoded)
			return nil, responseFailure(toolError("Matrix tool reported a business failure"), response, "connector-result")
		}
	}
	return result, nil
}
