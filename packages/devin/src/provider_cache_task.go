package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const cacheTaskID = "cache-keepalive"
const cacheTaskAction = "devin.cache-keepalive"

type cacheForeground struct {
	state     *providerState
	authority cacheTaskAuthority
	id        string
	key       string
	prepared  bool
	committed bool
}

func (owner *providerOwner) beginCacheForeground(ctx context.Context, call sdk.Call, stream *sdk.Stream,
	state *providerState, request sdk.ExecuteRequest, options sdk.ExecuteOptions) (*cacheForeground, error) {
	if !state.cacheTasks || !state.cachePolicy.Enabled || options.CallerID == "" {
		return nil, nil
	}
	key, err := state.cacheIndex.key(options.CallerID, request.Protocol, request.Body, request.Headers)
	if err != nil {
		return nil, agentFailure("Cannot identify the Devin cache conversation")
	}
	identity, removed, err := state.cacheTemplates.begin(key)
	if err != nil {
		return nil, agentFailure("Cannot reserve the Devin cache conversation")
	}
	turn := &cacheForeground{state: state, id: identity, key: key,
		authority: cacheTaskAuthority{Account: call.Scope.Account, Generation: call.Scope.Generation, Model: request.Model}}
	if _, err := stream.CancelAccountTask(ctx, sdk.AccountTaskCancelRequest{Task: cacheTaskID, Key: key}); err != nil {
		turn.close()
		return nil, moduleFailure(err, "not-started")
	}
	if err := turn.cancelRemoved(ctx, stream, removed); err != nil {
		turn.close()
		return nil, err
	}
	return turn, nil
}

func (turn *cacheForeground) close() {
	if turn != nil && !turn.committed {
		turn.state.cacheTemplates.remove(turn.id)
	}
}

func (turn *cacheForeground) cancelRemoved(ctx context.Context, stream *sdk.Stream, removed []cacheTaskReference) error {
	for _, reference := range removed {
		if reference.Key != turn.key {
			if _, err := stream.CancelAccountTask(ctx, sdk.AccountTaskCancelRequest{
				Task: cacheTaskID, Key: reference.Key,
			}); err != nil {
				return moduleFailure(err, "not-started")
			}
		}
		if reference.Done != nil {
			select {
			case <-reference.Done:
			case <-ctx.Done():
				return moduleFailure(ctx.Err(), "not-started")
			}
		}
	}
	return nil
}

func (turn *cacheForeground) prepare(ctx context.Context, stream *sdk.Stream, payload []byte) error {
	if turn == nil {
		return nil
	}
	template, err := codec.NewCacheKeepaliveTemplate(payload)
	if err != nil {
		emitDiagnostic(ctx, stream, sdk.Diagnostic{Level: "info", Code: "devin.cache.unsupported", Phase: "preflight"})
		return nil
	}
	removed, armed := turn.state.cacheTemplates.arm(turn.id, template, turn.authority, turn.state.cachePolicy)
	if err := turn.cancelRemoved(ctx, stream, removed); err != nil {
		return err
	}
	turn.prepared = armed
	return nil
}

func cacheJob(identity string, tick uint64) string {
	return identity + "." + strconv.FormatUint(tick, 10)
}

func scheduleCacheTask(ctx context.Context, stream *sdk.Stream, state *providerState,
	identity, key string, tick uint64, sent, now time.Time) bool {
	dueAt := state.cachePolicy.due(sent, now).UnixMilli()
	if !state.cacheTemplates.plan(identity, tick, dueAt) {
		return false
	}
	_, err := stream.ScheduleAccountTask(ctx, sdk.AccountTaskScheduleRequest{
		Task: cacheTaskID, Key: key, Job: cacheJob(identity, tick), DueAt: dueAt,
	})
	if err != nil {
		emitDiagnostic(ctx, stream, sdk.Diagnostic{Level: "warn", Code: "devin.cache.schedule-refused", Phase: "schedule"})
		return false
	}
	return true
}

func (turn *cacheForeground) finish(ctx context.Context, stream *sdk.Stream, summary codec.NativeSummary, sent, now time.Time) {
	if turn == nil || !turn.prepared || ctx.Err() != nil || sent.IsZero() ||
		!summary.Usage.Measured(codec.InputCount|codec.CacheReadCount|codec.CacheWriteCount) ||
		summary.Usage.CacheRead == 0 && summary.Usage.CacheWrite == 0 {
		return
	}
	turn.committed = scheduleCacheTask(ctx, stream, turn.state, turn.id, turn.key, 1, sent, now)
}

func cacheMeasuredCall(summary codec.NativeSummary, model string, status int, started time.Time, failure error) sdk.OwnCall {
	usage := executeUsage(summary)
	if usage == nil {
		usage = &sdk.Usage{}
	}
	call := sdk.OwnCall{Kind: "cache_keepalive", Model: model, Status: status,
		Millis: min(time.Since(started).Milliseconds(), 24*60*60*1000), Usage: *usage}
	if failure != nil {
		code := "upstream"
		var scoped *sdk.Error
		if errors.Is(failure, context.Canceled) {
			code = "canceled"
		} else if errors.Is(failure, context.DeadlineExceeded) {
			code = "deadline_exceeded"
		} else if errors.As(failure, &scoped) {
			switch scoped.Code {
			case "unauthenticated", "permission_denied", "resource_exhausted", "data_loss", "unimplemented", "canceled", "deadline_exceeded":
				code = scoped.Code
			}
		}
		call.Error = code
		if call.Usage.Facts != nil {
			call.Usage.Facts.ErrorType = &code
		}
	}
	return call
}

func (owner *providerOwner) cacheTask(ctx context.Context, call sdk.Call, stream *sdk.Stream) (result any, failure error) {
	started, phase, status := time.Now(), "preflight", 0
	defer func() { finishDiagnostic(ctx, stream, started, "devin.cache", phase, status, failure) }()
	credentials, state, err := selectedCredentials(ctx, call)
	if err != nil {
		return nil, err
	}
	request, err := sdk.DecodeAccountTaskRequest(call.Params)
	if err != nil || request.Task != cacheTaskID || !state.cacheTasks || !state.cachePolicy.Enabled ||
		call.Scope.Operation == "" || call.Scope.Attempt != 1 {
		return nil, agentFailure("Invalid Devin cache task")
	}
	identity, count, found := strings.Cut(request.Job, ".")
	tick, err := strconv.ParseUint(count, 10, 64)
	if !found || err != nil || tick == 0 || request.Job != cacheJob(identity, tick) {
		return nil, agentFailure("Invalid Devin cache job")
	}
	entry, err := state.cacheTemplates.claim(identity, tick, cacheTaskAuthority{
		Account: call.Scope.Account, Generation: call.Scope.Generation, Model: request.Model,
	})
	if err != nil {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Stale Devin cache task", Outcome: "not-started"}
	}
	if entry.Key != request.Key || entry.DueAt != request.DueAt {
		state.cacheTemplates.remove(identity)
		return nil, agentFailure("Devin cache task differs from its planned conversation")
	}
	lifetime, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	defer func() {
		cancel()
		state.cacheTemplates.detach(identity, tick)
		close(done)
	}()
	if !state.cacheTemplates.attach(identity, tick, request.DueAt, cancel, done) {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Superseded Devin cache task", Outcome: "not-started"}
	}
	payload, err := entry.Template.Payload(credentials.Key)
	if err != nil {
		state.cacheTemplates.remove(identity)
		return nil, agentFailure("Devin cache task cannot use its selected credentials")
	}
	client, err := owner.client(state, stream)
	if err != nil {
		state.cacheTemplates.remove(identity)
		return nil, moduleFailure(err, "not-started")
	}
	var observed codec.NativeSummary
	var last []byte
	accepted := false
	accept := func(ctx context.Context) error {
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return err
		}
		accepted = true
		phase = "dispatch"
		return nil
	}
	note := func(delta codec.NativeDelta) error {
		phase, status = "stream", 200
		observed.Merge(codec.NativeSummary{Usage: delta.Usage, Stop: delta.Stop, Facts: delta.Facts})
		usage := executeUsage(observed)
		if usage == nil {
			return nil
		}
		encoded, err := json.Marshal(usage)
		if err != nil || bytes.Contains(encoded, []byte(credentials.Key)) {
			return errors.New("cache execution facts contain account credentials")
		}
		if bytes.Equal(last, encoded) {
			return nil
		}
		last = encoded
		return stream.ReportAccountTaskUsage(lifetime, cacheMeasuredCall(observed, request.Model, status, started, nil))
	}
	sent := owner.options.Now()
	summary, err := client.Complete(lifetime, credentials, codec.EncodedChat{Payload: payload, Model: request.Model}, accept, note)
	observed.Merge(summary)
	if err != nil {
		canceled := lifetime.Err()
		state.cacheTemplates.detach(identity, tick)
		state.cacheTemplates.remove(identity)
		if !accepted {
			return nil, moduleFailure(err, "not-started")
		}
		if canceled != nil {
			return nil, moduleFailure(canceled, "uncertain")
		}
		failure := moduleFailure(err, "uncertain")
		var response *outbound.Error
		if errors.As(err, &response) && response.Status >= 400 {
			status = response.Status
		}
		measured := cacheMeasuredCall(observed, request.Model, status, started, failure)
		if encoded, marshalErr := json.Marshal(measured); marshalErr == nil && !bytes.Contains(encoded, []byte(credentials.Key)) {
			_ = stream.ReportAccountTaskUsage(lifetime, measured)
		}
		return nil, failure
	}
	status, phase = 200, "completed"
	measured := cacheMeasuredCall(observed, request.Model, status, started, nil)
	if encoded, marshalErr := json.Marshal(measured); marshalErr != nil || bytes.Contains(encoded, []byte(credentials.Key)) {
		state.cacheTemplates.remove(identity)
		return nil, agentFailure("Invalid Devin cache usage facts")
	}
	if err := stream.ReportAccountTaskUsage(lifetime, measured); err != nil {
		state.cacheTemplates.remove(identity)
		return nil, moduleFailure(err, "uncertain")
	}
	if state.cacheTemplates.complete(identity, tick, status, observed.Usage) &&
		!scheduleCacheTask(lifetime, stream, state, identity, entry.Key, tick+1, sent, owner.options.Now()) {
		state.cacheTemplates.remove(identity)
	}
	return sdk.AccountTaskResult{Outcome: "completed", Call: &measured}, nil
}
