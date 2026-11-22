package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type eventWriter struct {
	ctx       context.Context
	stream    *sdk.Stream
	header    http.Header
	status    int
	err       error
	sentModel string
}

func (writer *eventWriter) Header() http.Header { return writer.header }
func (writer *eventWriter) WriteHeader(status int) {
	if writer.status != 0 || writer.err != nil {
		return
	}
	writer.status = status
	writer.err = writer.stream.Emit(writer.ctx, "headers", sdk.HeadersEvent{
		Status: status, Headers: writer.header.Clone(), SentModel: writer.sentModel,
	})
}
func (writer *eventWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.err != nil {
		return 0, writer.err
	}
	written := 0
	for len(body) > 0 {
		count := min(len(body), sdk.MaxChunkBytes)
		if err := writer.stream.Emit(writer.ctx, "data", body[:count]); err != nil {
			writer.err = err
			return written, err
		}
		written += count
		body = body[count:]
	}
	return written, nil
}
func (writer *eventWriter) Flush() {
	if writer.err == nil && writer.status != 0 {
		writer.err = writer.stream.Emit(writer.ctx, "flush", nil)
	}
}

func executeUsage(summary codec.NativeSummary) *sdk.Usage {
	facts := &sdk.ExecutionUsageFacts{}
	if value := summary.Usage; value != nil {
		for _, field := range []struct {
			bit    uint8
			value  uint64
			target **int64
		}{
			{codec.InputCount, value.Input, &facts.InputTokens}, {codec.OutputCount, value.Output, &facts.OutputTokens},
			{codec.CacheReadCount, value.CacheRead, &facts.CachedTokens}, {codec.CacheWriteCount, value.CacheWrite, &facts.CacheWriteTokens},
		} {
			if value.Measured(field.bit) {
				count := int64(field.value)
				*field.target = &count
			}
		}
	}
	for _, field := range []struct {
		value  string
		target **string
	}{
		{summary.Facts.ServedModel, &facts.ServedModel}, {summary.Facts.UpstreamProvider, &facts.UpstreamProvider},
		{summary.Facts.RequestID, &facts.RequestID}, {summary.Facts.ResponseID, &facts.ResponseID},
		{codec.NativeStopName(summary.Stop), &facts.Stop},
	} {
		if field.value != "" {
			copy := field.value
			*field.target = &copy
		}
	}
	if summary.Facts.Credits != nil {
		copy := *summary.Facts.Credits
		facts.Credits = &copy
	}
	if summary.Facts.ACUCost != nil {
		facts.Cost = &sdk.ExecutionCost{Amount: *summary.Facts.ACUCost, Currency: "ACU"}
	}
	if summary.Usage == nil && facts.ServedModel == nil && facts.UpstreamProvider == nil && facts.RequestID == nil &&
		facts.ResponseID == nil && facts.Stop == nil && facts.Credits == nil && facts.Cost == nil {
		return nil
	}
	return &sdk.Usage{Facts: facts}
}

func (owner *providerOwner) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (result any, failure error) {
	started, phase := time.Now(), "preflight"
	var writer *eventWriter
	var observed codec.NativeSummary
	responseID := ""
	executionUsage := func(summary codec.NativeSummary) *sdk.Usage {
		usage := executeUsage(summary)
		if usage != nil && responseID != "" {
			copy := responseID
			usage.Facts.ResponseID = &copy
		}
		return usage
	}
	defer func() {
		if failure != nil && ctx.Err() == nil {
			if usage := executionUsage(observed); usage != nil {
				code := "upstream"
				var scoped *sdk.Error
				if errors.As(failure, &scoped) {
					switch scoped.Code {
					case "permission_denied", "resource_exhausted", "unauthenticated", "invalid_params",
						"data_loss", "unimplemented", "deadline_exceeded", "canceled", "failed_precondition":
						code = scoped.Code
					}
				}
				usage.Facts.ErrorType = &code
				_ = stream.Emit(ctx, "usage", usage)
			}
		}
		status := 0
		if writer != nil {
			status = writer.status
		}
		finishDiagnostic(ctx, stream, started, "devin.execute", phase, status, failure)
	}()
	credentials, state, err := selectedCredentials(ctx, call)
	if err != nil {
		return nil, err
	}
	var request sdk.ExecuteRequest
	if err := decodeParams(call.Params, &request); err != nil {
		return nil, err
	}
	var options sdk.ExecuteOptions
	if err := decodeParams(request.Options, &options); err != nil {
		return nil, err
	}
	chat := request.Protocol == "chat" && (options.Path == "/v1/chat/completions" || options.Path == "/chat/completions")
	messages := request.Protocol == "anthropic" && (options.Path == "/v1/messages" || options.Path == "/messages")
	if !chat && !messages || request.Operation != "" ||
		request.Model == "" || request.OperationID == "" || request.OperationID != call.Scope.Operation ||
		options.Method != "POST" ||
		options.Query != "" || options.EscapedPath != "" && options.EscapedPath != options.Path || len(request.Credentials) > 0 {
		return nil, &sdk.Error{Code: "unsupported", Message: "Unsupported Devin provider operation", Outcome: "not-started"}
	}
	var body struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(request.Body, &body) != nil || body.Model != request.Model {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Selected Devin model does not match the request", Outcome: "not-started"}
	}
	cacheTurn, err := owner.beginCacheForeground(ctx, call, stream, state, request, options)
	if err != nil {
		return nil, err
	}
	defer cacheTurn.close()
	var sent time.Time
	writer = &eventWriter{ctx: ctx, stream: stream, header: make(http.Header)}
	if messages && options.CallerID == "" {
		_ = outbound.NewMessagesRenderer(writer, request.Model, body.Stream).Fail(http.StatusForbidden)
		return nil, originDenied()
	}
	client, err := owner.client(state, stream)
	if err != nil {
		return nil, moduleFailure(err, "not-started")
	}
	client.HoldRefusals = true
	accepted := false
	dispatched := false
	var sentModel string
	publishAccepted := func(ctx context.Context) error {
		if accepted {
			return nil
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return err
		}
		accepted = true
		return nil
	}
	accept := func(ctx context.Context) error {
		dispatched = true
		if !strings.Contains(sentModel, credentials.Key) {
			writer.sentModel = sentModel
		}
		phase = "dispatch"
		retryable := false
		emitDiagnostic(ctx, stream, sdk.Diagnostic{Level: "info", Code: "devin.execute.accepted",
			Phase: phase, Retryable: &retryable})
		sent = owner.options.Now()
		return nil
	}
	var lastUsage []byte
	note := func(update codec.NativeSummary) error {
		if err := publishAccepted(ctx); err != nil {
			return err
		}
		phase = "stream"
		candidate := observed
		candidate.Merge(update)
		usage := executionUsage(candidate)
		if usage == nil {
			return nil
		}
		encoded, err := json.Marshal(usage)
		if err != nil || bytes.Contains(encoded, []byte(credentials.Key)) {
			return errors.New("native execution facts contain account credentials")
		}
		observed = candidate
		if bytes.Equal(lastUsage, encoded) {
			return nil
		}
		lastUsage = encoded
		return stream.Emit(ctx, "usage", usage)
	}
	refused := func(err error) (sdk.ExecuteResult, bool) {
		result, final := observeExecutionFailure(ctx, stream, err, request.Model, credentials.Key, !accepted && writer.status == 0)
		if final {
			result.SentModel = writer.sentModel
		}
		if !final && dispatched {
			_ = publishAccepted(ctx)
		}
		return result, final
	}
	var summary codec.NativeSummary
	if len(options.Native) > 0 && string(options.Native) != "null" {
		if !chat {
			return nil, &sdk.Error{Code: "unsupported", Message: "Native Devin bridges require Chat transport", Outcome: "not-started"}
		}
		var reference struct {
			Handle string `json:"handle"`
		}
		if decodeParams(options.Native, &reference) != nil || reference.Handle == "" {
			return denyNativeOrigin(writer, nil)
		}
		bridge, err := state.bridges.get(reference.Handle)
		if err != nil {
			return denyNativeOrigin(writer, nil)
		}
		if err := verifyOrigins(ctx, stream, call, options, request.Model, bridge.envelope.Claims); err != nil {
			return denyNativeOrigin(writer, bridge)
		}
		payload, err := codec.ApplyNativeOutputLimit(bridge.envelope.Payload, request.Body)
		if err != nil {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Invalid native output limit", Outcome: "not-started"}
		}
		nativeModel, err := owner.resolveModel(ctx, client, credentials, state, request.Model, "")
		if err != nil {
			return nil, moduleFailure(err, "not-started")
		}
		if cacheTurn != nil {
			payload, encodeErr := codec.ReplaceNativeAuthority(payload, nativeModel, credentials.Key)
			if encodeErr != nil {
				return nil, moduleFailure(encodeErr, "not-started")
			}
			if err := cacheTurn.prepare(ctx, stream, payload); err != nil {
				return nil, err
			}
		}
		sentModel = nativeModel
		summary, err = owner.executeNative(ctx, call, stream, client, credentials, request, options, bridge, payload, nativeModel, writer, accept, note)
		if err != nil {
			if result, final := refused(err); final {
				bridge.mu.Lock()
				bridge.claimed = false
				bridge.mu.Unlock()
				return result, nil
			}
			return nil, moduleFailure(err, "uncertain")
		}
	} else if messages {
		renderer := outbound.NewMessagesRenderer(writer, request.Model, body.Stream)
		responseID = renderer.ResponseID()
		encoded, err := codec.EncodeMessagesRequest(request.Body, credentials.Key, func(signature string) (codec.NativeSignature, error) {
			return resolveMessagesSignature(ctx, stream, call, options, request.Model, signature)
		})
		if err == nil {
			encoded, err = owner.resolveEncodedModel(ctx, client, credentials, state, encoded)
		}
		if err == nil {
			encoded, err = restoreGeminiThoughts(ctx, stream, call, options, state, request, encoded)
		}
		if err != nil {
			var failure *sdk.Error
			if errors.As(err, &failure) {
				_ = renderer.Fail(http.StatusForbidden)
				return nil, failure
			}
			_ = renderer.Fail(http.StatusBadRequest)
			return nil, &sdk.Error{Code: "invalid_params", Message: "Unsupported or inconsistent Devin Messages request", Outcome: "not-started"}
		}
		if err := cacheTurn.prepare(ctx, stream, encoded.Payload); err != nil {
			return nil, err
		}
		collector := originCollector{}
		geminiReply := codec.GeminiReply{}
		sentModel = encoded.SentModel
		summary, err = client.Messages(ctx, credentials, encoded, accept, func(delta codec.NativeDelta) error {
			if codec.IsGeminiModel(request.Model) && bytes.Contains(delta.Raw, []byte(credentials.Key)) {
				return errors.New("native Gemini reply contains account credentials")
			}
			if err := note(codec.NativeSummary{Usage: delta.Usage, Stop: delta.Stop, Facts: delta.Facts}); err != nil {
				return err
			}
			if err := renderer.Delta(delta); err != nil {
				return err
			}
			if codec.IsGeminiModel(request.Model) {
				if err := geminiReply.Add(delta); err != nil {
					return err
				}
			}
			return collector.add(delta.Raw)
		})
		if err == nil {
			err = renderer.Validate(summary)
		}
		if err == nil && !codec.IsGeminiModel(request.Model) {
			err = collector.persist(ctx, stream, call, options, request.Model)
		}
		if err == nil && codec.IsGeminiModel(request.Model) {
			err = persistGeminiThought(ctx, stream, call, options, state, request, &geminiReply, &collector)
		}
		if err == nil {
			err = renderer.Finish(summary)
		}
		if err != nil {
			if result, final := refused(err); final {
				return result, nil
			}
			status := http.StatusBadGateway
			var failure *outbound.Error
			if errors.As(err, &failure) && failure.Status >= 400 {
				status = failure.Status
			}
			_ = renderer.Fail(status)
			return nil, moduleFailure(err, "uncertain")
		}
	} else {
		renderer := outbound.NewChatRenderer(writer, request.Model, body.Stream)
		responseID = renderer.ResponseID()
		encoded, encodeErr := codec.EncodeChatRequest(request.Body, credentials.Key, func(signature string) (codec.NativeSignature, error) {
			return resolveMessagesSignature(ctx, stream, call, options, request.Model, signature)
		})
		if encodeErr == nil {
			encoded, encodeErr = owner.resolveEncodedModel(ctx, client, credentials, state, encoded)
		}
		if encodeErr == nil {
			encoded, encodeErr = restoreGeminiThoughts(ctx, stream, call, options, state, request, encoded)
		}
		if encodeErr != nil {
			var failure *sdk.Error
			if errors.As(encodeErr, &failure) {
				_ = renderer.Fail(http.StatusForbidden)
				return nil, failure
			}
			_ = renderer.Fail(http.StatusBadRequest)
			return nil, &sdk.Error{Code: "invalid_params", Message: "Unsupported Devin Chat request", Outcome: "not-started"}
		}
		if err := cacheTurn.prepare(ctx, stream, encoded.Payload); err != nil {
			return nil, err
		}
		geminiReply := codec.GeminiReply{}
		collector := originCollector{}
		sentModel = encoded.SentModel
		summary, err = client.Complete(ctx, credentials, encoded, accept, func(delta codec.NativeDelta) error {
			if codec.IsGeminiModel(request.Model) && bytes.Contains(delta.Raw, []byte(credentials.Key)) {
				return errors.New("native Gemini reply contains account credentials")
			}
			if err := note(codec.NativeSummary{Usage: delta.Usage, Stop: delta.Stop, Facts: delta.Facts}); err != nil {
				return err
			}
			if err := renderer.Delta(delta); err != nil {
				return err
			}
			if codec.IsGeminiModel(request.Model) {
				if err := geminiReply.Add(delta); err != nil {
					return err
				}
			}
			return collector.add(delta.Raw)
		})
		if err == nil {
			err = renderer.Validate(summary)
		}
		if err == nil && codec.IsGeminiModel(request.Model) {
			err = persistGeminiThought(ctx, stream, call, options, state, request, &geminiReply, &collector)
		}
		if err == nil && !codec.IsGeminiModel(request.Model) {
			err = collector.persist(ctx, stream, call, options, request.Model)
		}
		if err == nil {
			err = renderer.Finish(summary)
		}
		if err != nil {
			if result, final := refused(err); final {
				return result, nil
			}
			status := http.StatusBadGateway
			var failure *outbound.Error
			if errors.As(err, &failure) && failure.Status >= 400 {
				status = failure.Status
			}
			_ = renderer.Fail(status)
			return nil, moduleFailure(err, "uncertain")
		}
	}
	if writer.err != nil {
		return nil, moduleFailure(writer.err, "uncertain")
	}
	usage := executionUsage(summary)
	if usage != nil {
		if err := stream.Emit(ctx, "usage", usage); err != nil {
			return nil, moduleFailure(err, "uncertain")
		}
	}
	phase = "completed"
	cacheTurn.finish(ctx, stream, summary, sent, owner.options.Now())
	return sdk.ExecuteResult{Status: http.StatusOK, Usage: usage, Outcome: "completed"}, nil
}

func denyNativeOrigin(writer *eventWriter, bridge *nativeBridge) (any, error) {
	if bridge != nil {
		writer.Header().Set("Content-Type", "text/event-stream")
		payload := []byte(`{"error":{"code":"permission_denied","message":"Native signed history has no matching Devin origin"}}`)
		if err := bridge.push(writer.ctx, codec.NativeFrame{Flags: 2, Payload: payload}); err != nil {
			return nil, moduleFailure(err, "not-started")
		}
		if _, err := writer.Write([]byte(nativeWakeup)); err != nil {
			return nil, moduleFailure(err, "not-started")
		}
		if _, err := writer.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")); err != nil {
			return nil, moduleFailure(err, "not-started")
		}
		writer.Flush()
		if writer.err != nil {
			return nil, moduleFailure(writer.err, "not-started")
		}
		return sdk.ExecuteResult{Status: http.StatusOK, Outcome: "not-started"}, nil
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Should-Retry", "false")
	writer.WriteHeader(http.StatusBadRequest)
	if _, err := writer.Write([]byte(`{"error":{"message":"Native signed history has no matching Devin origin","type":"permission_error","code":"permission_denied"}}`)); err != nil {
		return nil, moduleFailure(err, "not-started")
	}
	return sdk.ExecuteResult{Status: http.StatusBadRequest, Outcome: "not-started"}, nil
}

const nativeWakeup = "data: {\"id\":\"devin-native\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"\\u2063\"},\"finish_reason\":null}]}\n\n"

func (owner *providerOwner) executeNative(ctx context.Context, call sdk.Call, stream *sdk.Stream, client *outbound.Client,
	credentials outbound.Credentials, request sdk.ExecuteRequest, options sdk.ExecuteOptions, bridge *nativeBridge,
	payload []byte, nativeModel string, writer *eventWriter, accept func(context.Context) error, note func(codec.NativeSummary) error) (codec.NativeSummary, error) {
	collector := originCollector{}
	terminal := false
	marker := func() error {
		_, err := writer.Write([]byte(nativeWakeup))
		writer.Flush()
		if err == nil {
			err = writer.err
		}
		return err
	}
	emit := func(frame codec.NativeFrame) error {
		if bytes.Contains(frame.Payload, []byte(credentials.Key)) {
			return errors.New("native response contains account credentials")
		}
		if frame.Flags&2 == 0 {
			metrics, err := codec.DecodeNativeMetrics(frame.Payload)
			if err != nil {
				return err
			}
			if err := note(metrics); err != nil {
				return err
			}
			if err := collector.add(frame.Payload); err != nil {
				return err
			}
		} else {
			if err := note(codec.NativeSummary{}); err != nil {
				return err
			}
			var end struct {
				Error json.RawMessage `json:"error"`
			}
			if json.Unmarshal(frame.Payload, &end) != nil {
				return errors.New("malformed native terminal")
			}
			if len(end.Error) == 0 || string(end.Error) == "null" {
				if err := collector.persist(ctx, stream, call, options, request.Model); err != nil {
					return err
				}
			}
		}
		if err := bridge.push(ctx, frame); err != nil {
			return err
		}
		if frame.Flags&2 != 0 {
			terminal = true
		}
		return marker()
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	summary, err := client.Native(ctx, credentials, payload, nativeModel, accept, emit)
	var refusal *outbound.Error
	if err != nil && errors.As(err, &refusal) && refusal.NoEffect && refusal.Facts != nil && refusal.Facts.Complete && writer.status == 0 {
		return summary, err
	}
	if err != nil && !terminal && ctx.Err() == nil {
		code := "internal"
		var failure *outbound.Error
		if errors.As(err, &failure) && failure.Code != "" {
			code = failure.Code
		}
		payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": "Devin native request failed"}})
		_ = emit(codec.NativeFrame{Flags: 2, Payload: payload})
	}
	if err != nil {
		return summary, err
	}
	finish := "stop"
	if summary.Stop == 10 {
		finish = "tool_calls"
	} else if summary.Stop == 3 {
		finish = "length"
	}
	value := map[string]any{"id": "devin-native", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}}
	if usage := outbound.CanonicalUsage(summary); usage != nil {
		value["usage"] = usage
	}
	data, _ := json.Marshal(value)
	if _, err := writer.Write(append(append([]byte("data: "), data...), '\n', '\n')); err != nil {
		return summary, err
	}
	if _, err := io.WriteString(writer, "data: [DONE]\n\n"); err != nil {
		return summary, err
	}
	writer.Flush()
	return summary, writer.err
}
