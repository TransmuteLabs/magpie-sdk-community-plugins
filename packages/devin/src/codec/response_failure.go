package devin

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ResponseFailure struct {
	Code     string          `json:"code"`
	Type     string          `json:"type,omitempty"`
	Message  string          `json:"message,omitempty"`
	Details  json.RawMessage `json:"details,omitempty"`
	Complete bool            `json:"-"`
}

func ParseResponseFailure(payload []byte, connect bool) *ResponseFailure {
	var envelope struct {
		Code     string              `json:"code"`
		Type     string              `json:"type"`
		Message  string              `json:"message"`
		Details  json.RawMessage     `json:"details"`
		Error    *ResponseFailure    `json:"error"`
		Metadata map[string][]string `json:"metadata"`
	}
	valid := len(payload) <= 64<<10 && strictJSON(payload, &envelope) == nil
	if valid {
		root, err := catalogObject(payload)
		valid = err == nil
		objects := []map[string]json.RawMessage{root}
		if raw, found := root["error"]; found {
			nested, err := catalogObject(raw)
			valid = valid && err == nil
			objects = append(objects, nested)
		}
		for index, object := range objects {
			for name := range object {
				switch name {
				case "code", "type", "message":
					_, err := catalogString(object, name)
					valid = valid && err == nil
				case "details":
				case "error", "metadata":
					valid = valid && index == 0
				default:
					valid = false
				}
			}
		}
		if raw, found := root["metadata"]; found {
			metadata, err := catalogObject(raw)
			valid = valid && err == nil
			for _, rawValues := range metadata {
				var values []*string
				if json.Unmarshal(rawValues, &values) != nil || values == nil {
					valid = false
				}
				for _, value := range values {
					valid = valid && value != nil
				}
			}
		}
	}
	failure := envelope.Error
	if failure == nil && !connect {
		failure = &ResponseFailure{Code: envelope.Code, Type: envelope.Type, Message: envelope.Message, Details: envelope.Details}
	}
	if failure == nil {
		return nil
	}
	failure.Complete = valid && (!connect || envelope.Code == "" && envelope.Type == "" && envelope.Message == "" && len(envelope.Details) == 0) &&
		(envelope.Error == nil || envelope.Code == "" && envelope.Type == "" && envelope.Message == "" && len(envelope.Details) == 0)
	for _, field := range []struct {
		value *string
		max   int
	}{{&failure.Code, 128}, {&failure.Type, 128}, {&failure.Message, 1024}} {
		if len(*field.value) > field.max || !utf8.ValidString(*field.value) ||
			strings.IndexFunc(*field.value, unicode.IsControl) >= 0 {
			failure.Complete = false
			*field.value = ""
		}
	}
	failure.Complete = failure.Complete && failure.Code != "" && strings.TrimSpace(failure.Code) == failure.Code
	return failure
}

func (failure *ResponseFailure) RejectsBeforeEffect() bool {
	if failure == nil || !failure.Complete || len(failure.Details) != 0 && string(failure.Details) != "[]" && string(failure.Details) != "null" {
		return false
	}
	message := strings.ToLower(failure.Message)
	switch failure.Code {
	case "unauthenticated":
		return true
	case "resource_exhausted":
		return strings.HasPrefix(message, "reached free model rate limit")
	case "failed_precondition":
		return strings.HasPrefix(message, "user monthly acu quota exhausted") ||
			strings.HasPrefix(message, "client version ") && strings.Contains(message, "is no longer supported")
	case "permission_denied":
		return strings.Contains(message, "does not have access to this model") &&
			(strings.Contains(message, "account") || strings.Contains(message, "organization"))
	default:
		return false
	}
}
