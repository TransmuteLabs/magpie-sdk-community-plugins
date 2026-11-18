package minimax

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type FailureFacts struct {
	Status          int
	WireStatus      int
	EffectiveStatus int
	Origin          string
	Code            string
	CodePresent     bool
	CodeValid       bool
	Type            string
	Message         string
	RetryAfter      string
	RequestID       string
	Timing          failureTiming
	TrustedEndpoint bool
	BodyComplete    bool
	OutputStarted   bool
}

var creditRefusal = regexp.MustCompile(`(?i)(insufficient|not enough|exhausted|depleted)[ _-]*(account[ _-]*)?(credit|balance|quota)|(credit|balance|quota)[^\n]{0,40}(insufficient|exhausted|depleted|exceeded|run out|used up)|(?:account|plan|daily|weekly)[ _-]+(?:usage[ _-]+)?limit[ _-]+(?:reached|exceeded)|余额不足|积分不足|额度不足|额度用完|积分用完|额度已耗尽|积分已耗尽`)

func factText(value any, limit int) string {
	literal, valid := value.(string)
	if !valid || len(literal) > limit {
		return ""
	}
	return strings.TrimSpace(strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, literal))
}

func refusalFacts(status int, headers http.Header, body []byte, trusted bool) FailureFacts {
	facts := FailureFacts{
		Status: status, WireStatus: status, Origin: "http-body", TrustedEndpoint: trusted, BodyComplete: true,
		RetryAfter: factText(headers.Get("Retry-After"), 128),
		RequestID:  factText(headers.Get("X-Request-Id"), 300),
		Timing:     responseTiming(headers),
	}
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		contentType := strings.ToLower(headers.Get("Content-Type"))
		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(contentType, "text/plain") && !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
			facts.Message = factText(trimmed, 1024)
		} else {
			facts.BodyComplete = false
		}
		return facts
	}
	errorObject := object(envelope["error"])
	for _, status := range []struct {
		fields map[string]any
		key    string
	}{
		{object(envelope["base_resp"]), "status_code"},
		{object(envelope["statusInfo"]), "code"},
	} {
		if candidate, present := status.fields[status.key]; present {
			if _, valid := apiCode(candidate); !valid {
				facts.BodyComplete = false
			}
		}
	}
	for _, candidate := range []any{errorObject["type"], envelope["type"]} {
		if literal := factText(candidate, 128); literal != "" {
			facts.Type = literal
			break
		}
	}
	for _, field := range []struct {
		fields map[string]any
		key    string
	}{
		{errorObject, "code"},
		{envelope, "code"},
		{object(envelope["base_resp"]), "status_code"},
		{object(envelope["statusInfo"]), "code"},
	} {
		candidate, present := field.fields[field.key]
		if !present {
			continue
		}
		facts.CodePresent = true
		if code, valid := apiCode(candidate); valid {
			facts.Code = strconv.Itoa(code)
			facts.CodeValid = true
			break
		}
		if literal := factText(candidate, 256); literal != "" {
			if literal[0] >= '0' && literal[0] <= '9' || literal[0] == '-' || literal[0] == '+' {
				facts.BodyComplete = false
				break
			}
			facts.Code = literal
			facts.CodeValid = true
			break
		}
		facts.BodyComplete = false
		break
	}
	for _, candidate := range []any{errorObject["message"], envelope["message"], envelope["msg"], envelope["error"], object(envelope["base_resp"])["status_msg"], object(envelope["statusInfo"])["message"], object(envelope["statusInfo"])["msg"]} {
		if literal := factText(candidate, 1024); literal != "" {
			facts.Message = literal
			break
		}
	}
	return facts
}

func (facts FailureFacts) accountQuota() bool {
	if !facts.TrustedEndpoint || !facts.BodyComplete || facts.OutputStarted || facts.Status != 402 && facts.Status != 403 {
		return false
	}
	if contains([]string{"content_policy_error", "invalid_request_error"}, facts.Type) {
		return false
	}
	for _, classification := range []string{facts.Code, facts.Type} {
		if contains([]string{"1008", "2053", "insufficient_balance", "insufficient_credit", "insufficient_quota", "quota_exceeded", "credit_exhausted"}, classification) {
			return true
		}
	}
	return creditRefusal.MatchString(facts.Message)
}
