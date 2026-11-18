package devin

import (
	"context"
	"errors"
	"net/http"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func (owner *providerOwner) accountOperation(ctx context.Context, call sdk.Call, stream *sdk.Stream) (result any, failure error) {
	started, phase, status := time.Now(), "preflight", 0
	defer func() {
		finishDiagnostic(ctx, stream, started, "devin.account", phase, status, failure)
	}()
	credentials, state, err := selectedCredentials(ctx, call)
	if err != nil {
		return nil, err
	}
	var request sdk.AccountOperationRequest
	if err := decodeParams(call.Params, &request); err != nil || request.Model == "" {
		return nil, agentFailure("Native account operation requires its selected model")
	}
	var kind codec.AccountKind
	switch call.Action {
	case "devin.account.user-status":
		kind = codec.UserStatus
	case "devin.account.team-settings":
		kind = codec.TeamSettings
	case "devin.account.managed-plugins":
		kind = codec.ManagedPlugins
	case "devin.account.model-assignment":
		kind = codec.ModelAssignment
	default:
		return nil, agentFailure("Unknown Devin account operation")
	}
	client, err := owner.client(state, stream)
	if err != nil {
		return nil, moduleFailure(err, "not-started")
	}
	if kind == codec.ModelAssignment && (call.Scope.Operation == "" || request.CallerID == "") {
		return nil, agentFailure("Native assignment requires its host receipt identity")
	}
	accept := func(ctx context.Context) error {
		return stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation})
	}
	phase = "dispatch"
	body, err := client.Account(ctx, credentials, kind, request.Body, request.Model, accept)
	if err != nil {
		var upstream *outbound.Error
		if errors.As(err, &upstream) && upstream.Status >= 100 && upstream.Status <= 599 {
			status = upstream.Status
		}
		return nil, moduleFailure(err, "not-started")
	}
	status = http.StatusOK
	if kind == codec.ModelAssignment {
		assignment, err := codec.DecodeAssignment(body)
		if err != nil || assignment.Model != request.Model {
			return nil, &sdk.Error{Code: "permission_denied", Message: "Native assignment changed the selected model", Outcome: "uncertain"}
		}
		cascade, err := codec.AssignmentCascade(request.Body)
		if err != nil {
			return nil, moduleFailure(err, "uncertain")
		}
		if err := persistOriginRecords(ctx, stream, []originReceipt{{Kind: "assignment",
			Digest: codec.SignatureDigest([]byte(assignment.Token)), Model: request.Model, ActualModel: assignment.Model,
			Format: cascade, Caller: request.CallerID, Generation: call.Scope.Generation}}); err != nil {
			return nil, err
		}
	}
	phase = "completed"
	return sdk.AccountOperationResult{Status: http.StatusOK, Headers: http.Header{"Content-Type": {"application/proto"}}, Body: body}, nil
}
