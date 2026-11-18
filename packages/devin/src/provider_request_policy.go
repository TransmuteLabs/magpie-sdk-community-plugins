package devin

import (
	"context"
	"errors"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func devinFailureRules() []sdk.RequestFailureRule {
	rules := []sdk.RequestFailureRule{
		{ID: "devin.free-limit", Code: "resource_exhausted", Contains: "reached free model rate limit", Class: "transient-rate", Scope: "model"},
		{ID: "devin.acu-quota", Code: "failed_precondition", Contains: "user monthly acu quota exhausted", Class: "quota", Scope: "model"},
		{ID: "devin.client-version", Code: "failed_precondition", Contains: "is no longer supported", Class: "request-refusal", Scope: "request"},
		{ID: "devin.credential", Code: "unauthenticated", Class: "auth-refusal", Scope: "account"},
		{ID: "devin.model-entitlement", Code: "permission_denied", Contains: "does not have access to this model", Class: "request-refusal", Scope: "model"},
		{ID: "devin.content", Code: "permission_denied", Contains: "content policy", Class: "content-refusal", Scope: "request"},
		{ID: "devin.demand", Code: "permission_denied", Contains: "high demand", Class: "transient-rate", Scope: "unknown"},
		{ID: "devin.invalid-internal", Code: "invalid_argument", Contains: "internal error", Class: "transient-outage", Scope: "unknown"},
		{ID: "devin.invalid-request", Code: "invalid_argument", Class: "request-refusal", Scope: "request"},
		{ID: "devin.permission", Code: "permission_denied", Class: "request-refusal", Scope: "request"},
	}
	for index := range rules {
		rules[index].Operation = sdk.MethodProviderExecute
		rules[index].Phase = "response"
	}
	return rules
}

func observeExecutionFailure(ctx context.Context, stream *sdk.Stream, err error, model, credential string, unpublished bool) (sdk.ExecuteResult, bool) {
	var failure *outbound.Error
	if ctx.Err() != nil || !errors.As(err, &failure) || failure.Facts == nil ||
		failure.Operation != "chat" && failure.Operation != "native" {
		return sdk.ExecuteResult{}, false
	}
	facts := *failure.Facts
	facts.Operation, facts.Phase = sdk.MethodProviderExecute, "response"
	for _, text := range []string{facts.Code, facts.Type, facts.Message} {
		if strings.Contains(text, credential) {
			return sdk.ExecuteResult{}, false
		}
	}
	for _, text := range facts.Headers {
		if strings.Contains(text, credential) {
			return sdk.ExecuteResult{}, false
		}
	}
	request := sdk.RequestRecoveryRequest{Step: "devin.execute." + failure.Operation, Facts: facts, Rules: devinFailureRules()}
	_, _ = sdk.DecideRequest(ctx, stream, request)
	if ctx.Err() != nil || !unpublished || !failure.NoEffect || !facts.Complete {
		return sdk.ExecuteResult{}, false
	}
	request.Evidence = "rejected-before-effect"
	var windows []sdk.RequestFailureWindow
	if facts.Code == "resource_exhausted" {
		windows = []sdk.RequestFailureWindow{{ID: "devin.free-model", Models: []string{model}, RetryAfterMS: facts.RetryAfterMS}}
	}
	return sdk.RefusedExecution(request, windows), true
}
