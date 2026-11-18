package commandcode

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func commandFailureRules() []sdk.RequestFailureRule {
	return []sdk.RequestFailureRule{
		{ID: "commandcode.credits", Contains: "premium_credits_exhausted", Class: "quota", Scope: "model"},
		{ID: "commandcode.balance", Contains: "insufficient credits", Class: "quota", Scope: "model"},
		{ID: "commandcode.plan", Contains: "model_not_in_plan", Class: "request-refusal", Scope: "model"},
		{ID: "commandcode.window", Contains: "window_limit", Class: "quota", Scope: "model"},
		{ID: "commandcode.usage", Contains: "usage limit", Class: "quota", Scope: "model"},
		{ID: "commandcode.pace", Contains: "rate_limited", Class: "transient-rate", Scope: "unknown"},
	}
}

func responseFailure(status int, headers http.Header, body []byte, secret string) (sdk.RequestFailureFacts, bool) {
	facts := sdk.RequestFailureFacts{Status: status, Operation: sdk.MethodProviderExecute, Phase: "response"}
	timing := make(map[string]string)
	for _, name := range []string{"Retry-After", "X-Ratelimit-Reset", "X-Ratelimit-Reset-Requests", "X-Ratelimit-Reset-Tokens"} {
		values := headers.Values(name)
		if len(values) > 1 || len(values) == 1 && (len(values[0]) > 256 || strings.ContainsFunc(values[0], unicode.IsControl) || secret != "" && strings.Contains(values[0], secret)) {
			return facts, false
		}
		if len(values) == 1 {
			timing[name] = values[0]
		}
	}
	facts.Headers = timing
	if len(body) > 64<<10 || !utf8.Valid(body) || !uniquePolicyJSON(body) {
		return facts, false
	}
	var envelope map[string]json.RawMessage
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil ||
		json.Unmarshal(envelope["error"], &fields) != nil || fields == nil {
		return facts, false
	}
	for name := range envelope {
		if name != "error" && name != "type" {
			return facts, false
		}
	}
	if kind, found := envelope["type"]; found && string(kind) != `"error"` {
		return facts, false
	}
	for name := range fields {
		if name != "code" && name != "type" && name != "message" && name != "param" {
			return facts, false
		}
	}
	if param, found := fields["param"]; found && !bytes.Equal(bytes.TrimSpace(param), []byte("null")) {
		return facts, false
	}
	parsed := facts
	for name, target := range map[string]*string{"code": &parsed.Code, "type": &parsed.Type, "message": &parsed.Message} {
		value, found := fields[name]
		if !found || name == "code" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return facts, false
		}
		if json.Unmarshal(value, target) != nil {
			var number json.Number
			if name != "code" || json.Unmarshal(value, &number) != nil {
				return facts, false
			}
			*target = number.String()
		}
		limit := 256
		if name == "type" {
			limit = 128
		}
		if name == "message" {
			limit = 1024
		}
		if len(*target) > limit || secret != "" && strings.Contains(*target, secret) {
			return facts, false
		}
	}
	parsed.Complete = parsed.Code != "" || parsed.Type != "" || parsed.Message != ""
	return parsed, parsed.Complete
}

func uniquePolicyJSON(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var read func(int) bool
	read = func(depth int) bool {
		if depth > 16 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, container := token.(json.Delim)
		if !container {
			return true
		}
		if delim != '{' && delim != '[' {
			return false
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				name, valid := key.(string)
				if err != nil || !valid || seen[name] {
					return false
				}
				seen[name] = true
			}
			if !read(depth + 1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && (delim == '{' && end == json.Delim('}') || delim == '[' && end == json.Delim(']'))
	}
	if !read(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func commandRefusal(status int, headers http.Header, body []byte, secret, step string) (sdk.ExecuteResult, bool) {
	facts, complete := responseFailure(status, headers, body, secret)
	if !complete || status < 400 || status >= 500 {
		return sdk.ExecuteResult{}, false
	}
	if facts.Type != "" && facts.Type != "payment" {
		return sdk.ExecuteResult{}, false
	}
	message := strings.ToLower(facts.Message)
	if facts.Code != "" && !strings.HasPrefix(message, strings.ToLower(facts.Code)+":") {
		return sdk.ExecuteResult{}, false
	}
	noEffect := strings.HasPrefix(message, "premium_credits_exhausted:") ||
		strings.HasPrefix(message, "model_not_in_plan:") ||
		message == "insufficient credits" ||
		strings.HasPrefix(message, "window_limit:") || strings.HasPrefix(message, "usage limit reached")
	if !noEffect {
		return sdk.ExecuteResult{}, false
	}
	request := sdk.RequestRecoveryRequest{Step: step, Facts: facts, Rules: commandFailureRules(), Evidence: "rejected-before-effect"}
	return sdk.RefusedExecution(request, nil), true
}
