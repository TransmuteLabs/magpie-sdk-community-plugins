package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func opencodeFailureRules() []sdk.RequestFailureRule {
	return []sdk.RequestFailureRule{
		{ID: "opencode.go-limit", Status: []int{429}, Type: "GoUsageLimitError", Class: "quota", Scope: "account"},
		{ID: "opencode.credit", Status: []int{402}, Type: "CreditLimitExceeded", Class: "quota", Scope: "account"},
		{ID: "opencode.free-limit", Status: []int{429}, Type: "FreeUsageLimitError", Class: "quota", Scope: "unknown"},
		{ID: "opencode.free-client", Status: []int{403}, Type: "FreeTierError", Class: "request-refusal", Scope: "request"},
	}
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

func responseFailure(status int, headers http.Header, body []byte, secret string) (sdk.RequestFailureFacts, bool) {
	facts := sdk.RequestFailureFacts{Status: status, Operation: sdk.MethodProviderExecute, Phase: "response"}
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
		if name != "error" && name != "type" && name != "metadata" {
			return facts, false
		}
	}
	if kind, found := envelope["type"]; found && string(kind) != `"error"` {
		return facts, false
	}
	if metadata, found := envelope["metadata"]; found {
		var values map[string]string
		if json.Unmarshal(metadata, &values) != nil || values == nil {
			return facts, false
		}
		for name := range values {
			if name != "workspace" && name != "limitName" && name != "coverage" && name != "resetsAt" {
				return facts, false
			}
		}
	}
	for name := range fields {
		if name != "code" && name != "type" && name != "message" && name != "param" {
			return facts, false
		}
	}
	if param, found := fields["param"]; found && !bytes.Equal(bytes.TrimSpace(param), []byte("null")) {
		return facts, false
	}
	for name, target := range map[string]*string{"code": &facts.Code, "type": &facts.Type, "message": &facts.Message} {
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
	facts.Headers = make(map[string]string)
	for _, name := range []string{"Retry-After", "X-Ratelimit-Reset", "X-Ratelimit-Reset-Requests", "X-Ratelimit-Reset-Tokens"} {
		values := headers.Values(name)
		if len(values) > 1 || len(values) == 1 && (len(values[0]) > 256 || strings.ContainsFunc(values[0], unicode.IsControl) || secret != "" && strings.Contains(values[0], secret)) {
			return facts, false
		}
		if len(values) == 1 {
			facts.Headers[name] = values[0]
		}
	}
	facts.Complete = facts.Code != "" || facts.Type != "" || facts.Message != ""
	return facts, facts.Complete
}

func executionRefusal(ctx context.Context, stream *sdk.Stream, scope sdk.Scope, response *http.Response, secret string) (sdk.ExecuteResult, bool, error) {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return sdk.ExecuteResult{}, false, stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: scope.Operation})
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil {
		if emitErr := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: scope.Operation}); emitErr != nil {
			return sdk.ExecuteResult{}, false, emitErr
		}
		return sdk.ExecuteResult{}, false, failure(err, "uncertain")
	}
	response.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(body), response.Body), response.Body}
	facts, complete := responseFailure(response.StatusCode, response.Header, body, secret)
	known := scope.Provider == ProviderGo && facts.Type == "GoUsageLimitError" && facts.Status == 429 ||
		scope.Provider == ProviderZen && facts.Type == "CreditLimitExceeded" && facts.Status == 402 ||
		scope.Provider == ProviderZenPublic && (facts.Type == "FreeTierError" && facts.Status == 403 ||
			facts.Type == "FreeUsageLimitError" && facts.Status == 429)
	if complete && known && facts.Code == "" {
		request := sdk.RequestRecoveryRequest{Step: "opencode.execute", Facts: facts, Rules: opencodeFailureRules(), Evidence: "rejected-before-effect"}
		return sdk.RefusedExecution(request, nil), true, nil
	}
	return sdk.ExecuteResult{}, false, stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: scope.Operation})
}
