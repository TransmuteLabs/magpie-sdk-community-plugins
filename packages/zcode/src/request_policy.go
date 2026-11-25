package zcode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type policyTransport func(*http.Request) (*http.Response, error)

type policyReadFailure struct{ err error }

func (failure policyReadFailure) Read([]byte) (int, error) {
	return 0, failure.err
}

func (transport policyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

var quotaResetTime = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})`)

func policyRules() []sdk.RequestFailureRule {
	var rules []sdk.RequestFailureRule
	for _, entry := range []struct{ code, class, scope string }{
		{"3008", "transient-rate", "account"}, {"3009", "transient-rate", "unknown"},
		{"3010", "transient-rate", "model"},
		{"1005", "quota", "model"}, {"1113", "quota", "model"},
		{"1308", "quota", "model"}, {"1310", "quota", "model"},
		{"1313", "quota", "model"}, {"1316", "quota", "model"},
		{"1317", "quota", "model"}, {"1318", "quota", "model"},
		{"1319", "quota", "model"}, {"1320", "quota", "model"},
		{"1321", "quota", "model"}, {"2056", "quota", "model"},
		{"20097", "quota", "model"},
		{"1006", "auth-refusal", "account"}, {"3007", "request-refusal", "request"},
		{"3006", "request-refusal", "model"}, {"3001", "request-refusal", "request"},
	} {
		rules = append(rules, sdk.RequestFailureRule{ID: "zcode-" + entry.code,
			Status: []int{200, 400, 401, 403, 429}, Code: entry.code,
			Operation: sdk.MethodProviderExecute, Phase: "response", Class: entry.class, Scope: entry.scope})
	}
	return rules
}

func policyString(value any, limit int) (string, bool) {
	if value == nil {
		return "", true
	}
	text, valid := value.(string)
	return text, valid && len(text) <= limit
}

func policyCode(value any) (string, bool) {
	if number, valid := value.(json.Number); valid {
		value = string(number)
	}
	text, valid := policyString(value, 256)
	if !valid {
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", true
	}
	if !protocol.IsProviderCodeShaped(trimmed) {
		return "", false
	}
	return trimmed, true
}

func policyEnvelope(root map[string]any) bool {
	for name, value := range root {
		switch name {
		case "code", "msg", "message":
		case "request_id":
			text, valid := value.(string)
			if !valid || len(text) > 256 {
				return false
			}
		case "type":
			if value != "error" {
				return false
			}
		case "success":
			if value != false {
				return false
			}
		case "error":
			nested, valid := value.(map[string]any)
			if !valid {
				return false
			}
			for key := range nested {
				if key != "code" && key != "type" && key != "message" && key != "msg" {
					return false
				}
			}
		case "data":
			if value != nil {
				nested, valid := value.(map[string]any)
				if !valid || len(nested) != 0 {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

func (owner *module) requestFailure(response *http.Response, step string) (sdk.RequestRecoveryRequest, error) {
	failure := sdk.RequestRecoveryRequest{Step: step, Rules: policyRules(),
		Facts: sdk.RequestFailureFacts{Status: response.StatusCode, Operation: sdk.MethodProviderExecute, Phase: "response"}}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media != "application/json" {
		if response.StatusCode < 400 {
			return sdk.RequestRecoveryRequest{}, nil
		}
		return failure, nil
	}
	const limit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		response.Body.Close()
		response.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), policyReadFailure{err}))
		return failure, nil
	}
	if len(body) > limit {
		response.Body = &struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), response.Body), response.Body}
		if response.StatusCode < 400 {
			return sdk.RequestRecoveryRequest{}, nil
		}
		return failure, nil
	}
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	root, err := protocol.ReadObject(body)
	if err != nil {
		if response.StatusCode < 400 {
			return sdk.RequestRecoveryRequest{}, nil
		}
		return failure, nil
	}
	nested := mapObject(root["error"])
	code, validCode := policyCode(root["code"])
	nestedCode, validNestedCode := policyCode(nested["code"])
	if code == "" {
		code = nestedCode
	} else if nestedCode != "" && nestedCode != code {
		validCode = false
	}
	kind, validType := policyString(nested["type"], 128)
	message, validMessage := policyString(root["msg"], 1024)
	for _, value := range []any{root["message"], nested["message"], nested["msg"]} {
		text, valid := policyString(value, 1024)
		validMessage = validMessage && valid
		if message == "" {
			message = text
		}
	}
	if response.StatusCode < 400 && code == "" && root["type"] != "error" && root["success"] != false && root["error"] == nil ||
		response.StatusCode < 400 && (code == "0" || code == "200") && root["success"] != false && root["error"] == nil {
		return sdk.RequestRecoveryRequest{}, nil
	}
	failure.Facts.Complete = validCode && validNestedCode && validType && validMessage
	if !failure.Facts.Complete {
		return failure, nil
	}
	failure.Facts.Code, failure.Facts.Type, failure.Facts.Message = code, kind, message
	failure.Facts.Headers = map[string]string{}
	for _, name := range []string{"Retry-After", "X-RateLimit-Reset", "RateLimit-Reset"} {
		values := response.Header.Values(name)
		if len(values) == 0 {
			continue
		}
		if len(values) != 1 || len(values[0]) > 300 {
			failure.Facts.Complete = false
			return failure, nil
		}
		failure.Facts.Headers[name] = values[0]
	}
	if values := response.Header.Values("Retry-After-Ms"); len(values) > 0 {
		if len(values) != 1 {
			failure.Facts.Complete = false
			return failure, nil
		}
		delay, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || delay < 0 || delay > int64(30*24*time.Hour/time.Millisecond) {
			failure.Facts.Complete = false
			return failure, nil
		}
		failure.Facts.RetryAfterMS = &delay
	}
	for _, rule := range failure.Rules {
		if rule.Code != code {
			continue
		}
		statusMatches := false
		for _, status := range rule.Status {
			statusMatches = statusMatches || status == response.StatusCode
		}
		if statusMatches && policyEnvelope(root) {
			failure.Evidence = "rejected-before-effect"
		}
		if rule.Class == "quota" && (strings.Contains(strings.ToLower(message), "reset") || strings.Contains(message, "重置")) {
			if match := quotaResetTime.FindStringSubmatch(message); match != nil {
				reset, err := time.Parse(time.RFC3339, match[1]+"T"+match[2]+"+08:00")
				now := owner.options.Now()
				if err == nil && !reset.Before(now.Add(-24*time.Hour)) && !reset.After(now.Add(35*24*time.Hour)) {
					failure.Facts.ResetAt = reset.UTC().Format(time.RFC3339)
				}
			}
		}
		break
	}
	return failure, nil
}

func (owner *module) policyDispatch(ctx context.Context, stream *sdk.Stream, proxy string, outbound *http.Request, step string, before func() error, observe func(*http.Response, sdk.RequestRecoveryRequest) error) (*http.Response, error) {
	response, _, err := sdk.RequestWithPolicy(ctx, stream, step, outbound,
		policyTransport(func(request *http.Request) (*http.Response, error) {
			if before != nil {
				if err := before(); err != nil {
					return nil, err
				}
			}
			return send(ctx, owner.options.HTTPFactory, proxy, request)
		}), func(response *http.Response) (sdk.RequestRecoveryRequest, error) {
			failure, err := owner.requestFailure(response, step)
			if err == nil && observe != nil {
				err = observe(response, failure)
			}
			return failure, err
		})
	return response, err
}

func (owner *module) finalRefusal(response *http.Response, model string, free bool) (*sdk.ExecuteResult, error) {
	step := "paid-messages"
	if free {
		step = "start-messages"
	}
	failure, err := owner.requestFailure(response, step)
	if err != nil || failure.Evidence == "" {
		return nil, err
	}
	window := sdk.RequestFailureWindow{ID: step, Models: []string{model},
		ResetAt: failure.Facts.ResetAt, RetryAfterMS: failure.Facts.RetryAfterMS}
	result := sdk.RefusedExecution(failure, []sdk.RequestFailureWindow{window})
	return &result, nil
}
