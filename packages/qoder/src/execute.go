package qoder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type queueEndError struct {
	Message string
}

func (err *queueEndError) Error() string { return err.Message }

type queueAskState struct {
	RequestSetID string
	RequestID    string
	SessionID    string
	ModelKey     string
	QueueType    string
	Stuck        bool
}

func (owner *module) queuePause(seconds *float64) time.Duration {
	dur := owner.queue.Unsaid
	if seconds != nil && !math.IsNaN(*seconds) && !math.IsInf(*seconds, 0) {
		dur = time.Duration(*seconds * float64(time.Second))
	}
	if dur < owner.queue.Shortest {
		dur = owner.queue.Shortest
	}
	if dur > owner.queue.Longest {
		dur = owner.queue.Longest
	}
	return dur
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func queueState(text string) *queueInfo {
	var root any
	if json.Unmarshal([]byte(text), &root) != nil {
		return nil
	}
	todo := []any{root}
	visited := map[string]bool{}
	nestedKeys := []string{"data", "result", "message", "body"}
	for len(todo) > 0 {
		cur := todo[0]
		todo = todo[1:]
		obj, ok := cur.(map[string]any)
		if !ok || obj == nil {
			continue
		}
		if extracted := extractQueueObj(obj); extracted != nil {
			return extracted
		}
		for _, key := range nestedKeys {
			val, exists := obj[key]
			if !exists || val == nil {
				continue
			}
			switch typed := val.(type) {
			case map[string]any:
				todo = append(todo, typed)
			case string:
				trimmed := strings.TrimSpace(typed)
				if trimmed == "" || visited[trimmed] {
					continue
				}
				visited[trimmed] = true
				var parsed any
				if json.Unmarshal([]byte(trimmed), &parsed) == nil && parsed != nil {
					todo = append(todo, parsed)
				}
			}
		}
	}
	return nil
}

func stuckKey(info *queueInfo) string {
	if info == nil {
		return `[null,null,null,null,null,null]`
	}
	if info.QueueCount != nil && !math.IsNaN(*info.QueueCount) && !math.IsInf(*info.QueueCount, 0) && *info.QueueCount > 1 {
		return ""
	}
	var isQ any
	if info.IsQueued != nil {
		isQ = *info.IsQueued
	}
	var avail any
	if info.ServiceAvailable != nil {
		avail = *info.ServiceAvailable
	}
	var qCount any
	if info.QueueCount != nil && !math.IsNaN(*info.QueueCount) && !math.IsInf(*info.QueueCount, 0) {
		qCount = *info.QueueCount
	}
	var qType any
	if info.QueueType != "" {
		qType = info.QueueType
	}
	var subCode any
	if info.SubCode != "" {
		subCode = info.SubCode
	}
	var retry any
	if info.RetryAfterSeconds != nil && !math.IsNaN(*info.RetryAfterSeconds) && !math.IsInf(*info.RetryAfterSeconds, 0) {
		retry = *info.RetryAfterSeconds
	}
	raw, _ := json.Marshal([]any{isQ, avail, qCount, qType, subCode, retry})
	return string(raw)
}

func (owner *module) queueStatus(ctx context.Context, stream *sdk.Stream, proxyOverride, apiBase string, cred Credential, ask queueAskState, deadline time.Time, site Site) (*queueInfo, error) {
	query := url.Values{}
	query.Set("requestSetId", ask.RequestSetID)
	query.Set("modelKey", ask.ModelKey)
	if ask.QueueType != "" {
		query.Set("queueType", ask.QueueType)
	}
	targetURL := apiBase + queueStatusPath + "?" + query.Encode()
	headers, err := owner.cosyHeaders(targetURL, cred, "", site, owner.now().Unix())
	if err != nil {
		return nil, err
	}
	headers.Del("Content-Type")
	headers.Set("X-Request-ID", ask.RequestID)
	headers.Set("X-Session-ID", ask.SessionID)

	remaining := deadline.Sub(owner.now())
	if remaining <= 0 {
		return nil, &queueEndError{Message: "the queue's wait ran out"}
	}
	timeout := owner.queue.Ask
	if remaining < timeout {
		timeout = remaining
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers

	resp, cleanup, err := owner.doHTTP(reqCtx, stream, proxyOverride, req)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer resp.Body.Close()
	raw, err := readBodyLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, &queueEndError{Message: "Qoder has no queue status here"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("queue status: HTTP %d", resp.StatusCode)
	}
	info := queueState(string(raw))
	if info == nil {
		return nil, &queueEndError{Message: "the queue status said nothing of the queue"}
	}
	return info, nil
}

func (owner *module) queueWait(ctx context.Context, stream *sdk.Stream, proxyOverride, apiBase string, cred Credential, ask *queueAskState, first *queueInfo, deadline time.Time, site Site) (bool, error) {
	cur := queueInfo{}
	if first != nil {
		cur = *first
	}
	asked := false
	failed := 0
	sameKey := stuckKey(&cur)
	since := owner.now()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !owner.now().Before(deadline) {
			return false, nil
		}
		if asked {
			delay := owner.queuePause(cur.RetryAfterSeconds)
			if left := deadline.Sub(owner.now()); left < delay {
				delay = left
			}
			if err := sleepContext(ctx, delay); err != nil {
				return false, err
			}
		}
		asked = true
		if !owner.now().Before(deadline) {
			return false, nil
		}
		pollAsk := *ask
		if cur.ModelKey != "" {
			pollAsk.ModelKey = cur.ModelKey
		}
		if cur.QueueType != "" {
			pollAsk.QueueType = cur.QueueType
		}
		statusInfo, err := owner.queueStatus(ctx, stream, proxyOverride, apiBase, cred, pollAsk, deadline, site)
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			var endErr *queueEndError
			if errors.As(err, &endErr) {
				return false, nil
			}
			failed++
			if failed >= owner.queue.Failures {
				return false, nil
			}
			cur.RetryAfterSeconds = nil
			continue
		}
		failed = 0
		merged := *statusInfo
		if merged.ModelKey == "" {
			merged.ModelKey = cur.ModelKey
		}
		if merged.QueueType == "" {
			merged.QueueType = cur.QueueType
		}
		cur = merged

		key := stuckKey(&cur)
		if key == "" || key != sameKey {
			sameKey = key
			since = owner.now()
		} else if owner.now().Sub(since) >= owner.queue.Stuck && (statusInfo.IsQueued == nil || *statusInfo.IsQueued) {
			ask.Stuck = true
			return false, nil
		}
		if statusInfo.IsQueued != nil && !*statusInfo.IsQueued {
			if statusInfo.RetryAfterSeconds != nil && *statusInfo.RetryAfterSeconds > 0 {
				delay := owner.queuePause(statusInfo.RetryAfterSeconds)
				if left := deadline.Sub(owner.now()); left < delay {
					delay = left
				}
				if err := sleepContext(ctx, delay); err != nil {
					return false, err
				}
			}
			return owner.now().Before(deadline), nil
		}
		if (statusInfo.ServiceAvailable != nil && !*statusInfo.ServiceAvailable) ||
			(statusInfo.IsQueued != nil && *statusInfo.IsQueued) {
			continue
		}
		return false, nil
	}
}

type heldQueueStream struct {
	headers http.Header
	emitMu  sync.Mutex
	stopCh  chan struct{}
	wg      sync.WaitGroup
	stopped bool
}

func sanitizeSSEComment(text string) string {
	replacer := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")
	return replacer.Replace(text)
}

func (owner *module) startHeldQueue(ctx context.Context, stream *sdk.Stream, opID, servedName string, fail *qoderFailure, renewed bool) (*heldQueueStream, error) {
	hdr := http.Header{
		"Content-Type":  {"text/event-stream"},
		"Cache-Control": {"no-cache"},
	}
	applySignInHeader(hdr, http.StatusOK, renewed)
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
		return nil, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{
		Status:    http.StatusOK,
		Headers:   hdr,
		SentModel: servedName,
	}); err != nil {
		return nil, err
	}
	firstComment := fmt.Sprintf(": %s\n\n", sanitizeSSEComment(fail.Message))
	if err := stream.Emit(ctx, "data", []byte(firstComment)); err != nil {
		return nil, err
	}
	held := &heldQueueStream{
		headers: hdr,
		stopCh:  make(chan struct{}),
	}
	beat := owner.queue.Beat
	if beat <= 0 {
		beat = 10 * time.Second
	}
	held.wg.Add(1)
	go func() {
		defer held.wg.Done()
		ticker := time.NewTicker(beat)
		defer ticker.Stop()
		for {
			select {
			case <-held.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				held.emitMu.Lock()
				if !held.stopped {
					_ = stream.Emit(ctx, "data", []byte(": queued\n\n"))
				}
				held.emitMu.Unlock()
			}
		}
	}()
	return held, nil
}

func (held *heldQueueStream) stop() {
	if held == nil {
		return
	}
	held.emitMu.Lock()
	already := held.stopped
	held.stopped = true
	held.emitMu.Unlock()
	if !already {
		close(held.stopCh)
	}
	held.wg.Wait()
}

func applySignInHeader(hdr http.Header, status int, renewed bool) {
	if renewed {
		hdr.Set("X-Magpie-Sign-In", "renewed")
		return
	}
	if status >= 200 && status < 300 {
		hdr.Set("X-Magpie-Sign-In", "kept")
	}
}

func errorHeadersForFailure(fail *qoderFailure, renewed bool) http.Header {
	hdr := http.Header{"Content-Type": {"application/json"}}
	signIn := ""
	if fail.HasSignIn {
		signIn = fail.SignIn
	} else if fail.Status == http.StatusUnauthorized {
		signIn = "kept"
	}
	if signIn != "" {
		hdr.Set("X-Magpie-Sign-In", signIn)
	}
	if fail.RetryAfter > 0 {
		hdr.Set("Retry-After", strconv.Itoa(fail.RetryAfter))
	}
	applySignInHeader(hdr, fail.Status, renewed)
	return hdr
}

func emitFailureResponse(ctx context.Context, stream *sdk.Stream, opID, servedName string, fail *qoderFailure, renewed bool, held *heldQueueStream) (sdk.ExecuteResult, error) {
	if fail == nil {
		fail = &qoderFailure{Status: http.StatusBadGateway, Message: "Qoder request failed"}
	}
	if held != nil {
		held.stop()
		errFrame, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": fail.Message,
				"code":    fail.Status,
			},
		})
		_ = stream.Emit(ctx, "data", append(append([]byte("data: "), errFrame...), '\n', '\n'))
		return sdk.ExecuteResult{
			Status:    http.StatusOK,
			Headers:   held.headers,
			SentModel: servedName,
			Outcome:   "completed",
		}, nil
	}
	hdr := errorHeadersForFailure(fail, renewed)
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": fail.Message,
			"type":    "qoder_error",
			"code":    fail.Status,
		},
	})
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{
		Status:    fail.Status,
		Headers:   hdr,
		SentModel: servedName,
	}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if err := stream.Emit(ctx, "data", payload); err != nil {
		return sdk.ExecuteResult{}, err
	}
	return sdk.ExecuteResult{
		Status:    fail.Status,
		Headers:   hdr,
		SentModel: servedName,
		Outcome:   "completed",
	}, nil
}

func failureFromSignInError(site Site, err error) *qoderFailure {
	status := http.StatusBadGateway
	expired := false
	var gone *signInGoneError
	if errors.As(err, &gone) {
		status = http.StatusUnauthorized
		expired = gone.Expired
	}
	msg := strings.TrimPrefix(err.Error(), site.Name+": ")
	fail := &qoderFailure{
		Status:  status,
		Message: msg,
	}
	if expired {
		fail.SignIn = "expired"
		fail.HasSignIn = true
	}
	return fail
}

func toSDKUsage(usage *streamUsage) *sdk.Usage {
	if usage == nil {
		return nil
	}
	return &sdk.Usage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		CachedTokens: usage.CachedTokens,
	}
}

func (owner *module) emitAnswer(
	ctx context.Context,
	stream *sdk.Stream,
	opID string,
	reader *sseEventReader,
	firstEvent streamEvent,
	hasFirst bool,
	streamMode bool,
	servedName string,
	renewed bool,
	held *heldQueueStream,
) (sdk.ExecuteResult, error) {
	if firstEvent.Error != nil {
		return emitFailureResponse(ctx, stream, opID, servedName, firstEvent.Error, renewed, held)
	}
	if !hasFirst {
		return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
			Status:  http.StatusBadGateway,
			Message: "Qoder ended without an answer",
		}, renewed, held)
	}

	completionID := "chatcmpl-" + hexID()
	created := owner.now().Unix()

	if !streamMode {
		content := strings.Builder{}
		reasoning := strings.Builder{}
		type accTool struct {
			id   string
			name string
			args strings.Builder
		}
		var calls []*accTool
		finishReason := "stop"
		var finalUsage *streamUsage

		event := firstEvent
		ok := true
		for ok {
			if event.Error != nil {
				return emitFailureResponse(ctx, stream, opID, servedName, event.Error, renewed, held)
			}
			if event.Text != "" {
				content.WriteString(event.Text)
			}
			if event.Reasoning != "" {
				reasoning.WriteString(event.Reasoning)
			}
			if event.Tool != nil {
				entry := &accTool{id: event.Tool.ID, name: event.Tool.Name}
				entry.args.WriteString(event.Tool.Args)
				calls = append(calls, entry)
			}
			if event.Args != nil && event.Args.Index >= 0 && event.Args.Index < len(calls) {
				calls[event.Args.Index].args.WriteString(event.Args.Text)
			}
			if event.Stop != "" {
				finishReason = event.Stop
				finalUsage = event.Usage
			}
			var readErr error
			event, ok, readErr = reader.Next()
			if readErr != nil {
				return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
					Status:  http.StatusBadGateway,
					Message: readErr.Error(),
				}, renewed, held)
			}
		}
		msgObj := map[string]any{
			"role":    "assistant",
			"content": content.String(),
		}
		if reasoning.Len() > 0 {
			msgObj["reasoning_content"] = reasoning.String()
		}
		if len(calls) > 0 {
			outCalls := make([]map[string]any, 0, len(calls))
			for _, item := range calls {
				outCalls = append(outCalls, map[string]any{
					"id":   item.id,
					"type": "function",
					"function": map[string]any{
						"name":      item.name,
						"arguments": item.args.String(),
					},
				})
			}
			msgObj["tool_calls"] = outCalls
		}
		respBody := map[string]any{
			"id":      completionID,
			"object":  "chat.completion",
			"created": created,
			"model":   servedName,
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       msgObj,
					"finish_reason": finishReason,
				},
			},
		}
		if finalUsage != nil {
			respBody["usage"] = finalUsage.toOpenAI()
		}
		encoded, err := json.Marshal(respBody)
		if err != nil {
			return sdk.ExecuteResult{}, err
		}
		hdr := http.Header{"Content-Type": {"application/json"}}
		applySignInHeader(hdr, http.StatusOK, renewed)
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{
			Status:    http.StatusOK,
			Headers:   hdr,
			SentModel: servedName,
		}); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if err := stream.Emit(ctx, "data", encoded); err != nil {
			return sdk.ExecuteResult{}, err
		}
		sdkUsage := toSDKUsage(finalUsage)
		if sdkUsage != nil {
			_ = stream.Emit(ctx, "usage", *sdkUsage)
		}
		return sdk.ExecuteResult{
			Status:    http.StatusOK,
			Headers:   hdr,
			SentModel: servedName,
			Usage:     sdkUsage,
			Outcome:   "completed",
		}, nil
	}

	var sseHeaders http.Header
	if held != nil {
		held.stop()
		sseHeaders = held.headers
	} else {
		sseHeaders = http.Header{
			"Content-Type":  {"text/event-stream"},
			"Cache-Control": {"no-cache"},
		}
		applySignInHeader(sseHeaders, http.StatusOK, renewed)
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{
			Status:    http.StatusOK,
			Headers:   sseHeaders,
			SentModel: servedName,
		}); err != nil {
			return sdk.ExecuteResult{}, err
		}
	}

	emitChunk := func(delta map[string]any, finishReason any, usage *streamUsage) error {
		frame := map[string]any{
			"id":      completionID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   servedName,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         delta,
					"finish_reason": finishReason,
				},
			},
		}
		if usage != nil {
			frame["usage"] = usage.toOpenAI()
		}
		encoded, err := json.Marshal(frame)
		if err != nil {
			return err
		}
		return stream.Emit(ctx, "data", append(append([]byte("data: "), encoded...), '\n', '\n'))
	}

	if err := emitChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil); err != nil {
		return sdk.ExecuteResult{}, err
	}

	var finalUsage *streamUsage
	event := firstEvent
	ok := true
	for ok {
		if event.Text != "" {
			if err := emitChunk(map[string]any{"content": event.Text}, nil, nil); err != nil {
				return sdk.ExecuteResult{}, err
			}
		} else if event.Reasoning != "" {
			if err := emitChunk(map[string]any{"reasoning_content": event.Reasoning}, nil, nil); err != nil {
				return sdk.ExecuteResult{}, err
			}
		} else if event.Tool != nil {
			if err := emitChunk(map[string]any{
				"tool_calls": []map[string]any{
					{
						"index": event.Tool.Index,
						"id":    event.Tool.ID,
						"type":  "function",
						"function": map[string]any{
							"name":      event.Tool.Name,
							"arguments": event.Tool.Args,
						},
					},
				},
			}, nil, nil); err != nil {
				return sdk.ExecuteResult{}, err
			}
		} else if event.Args != nil {
			if err := emitChunk(map[string]any{
				"tool_calls": []map[string]any{
					{
						"index": event.Args.Index,
						"function": map[string]any{
							"arguments": event.Args.Text,
						},
					},
				},
			}, nil, nil); err != nil {
				return sdk.ExecuteResult{}, err
			}
		} else if event.Stop != "" {
			finalUsage = event.Usage
			if err := emitChunk(map[string]any{}, event.Stop, event.Usage); err != nil {
				return sdk.ExecuteResult{}, err
			}
		} else if event.Error != nil {
			errFrame, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": event.Error.Message,
					"code":    event.Error.Status,
				},
			})
			_ = stream.Emit(ctx, "data", append(append([]byte("data: "), errFrame...), '\n', '\n'))
			sdkUsage := toSDKUsage(finalUsage)
			if sdkUsage != nil {
				_ = stream.Emit(ctx, "usage", *sdkUsage)
			}
			return sdk.ExecuteResult{
				Status:    http.StatusOK,
				Headers:   sseHeaders,
				SentModel: servedName,
				Usage:     sdkUsage,
				Outcome:   "completed",
			}, nil
		}
		var readErr error
		event, ok, readErr = reader.Next()
		if readErr != nil {
			errFrame, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": readErr.Error(),
					"code":    http.StatusBadGateway,
				},
			})
			_ = stream.Emit(ctx, "data", append(append([]byte("data: "), errFrame...), '\n', '\n'))
			return sdk.ExecuteResult{
				Status:    http.StatusOK,
				Headers:   sseHeaders,
				SentModel: servedName,
				Usage:     toSDKUsage(finalUsage),
				Outcome:   "completed",
			}, nil
		}
	}

	if err := stream.Emit(ctx, "data", []byte("data: [DONE]\n\n")); err != nil {
		return sdk.ExecuteResult{}, err
	}
	sdkUsage := toSDKUsage(finalUsage)
	if sdkUsage != nil {
		_ = stream.Emit(ctx, "usage", *sdkUsage)
	}
	return sdk.ExecuteResult{
		Status:    http.StatusOK,
		Headers:   sseHeaders,
		SentModel: servedName,
		Usage:     sdkUsage,
		Outcome:   "completed",
	}, nil
}

func findModelEntry(models []ModelEntry, modelID string) (ModelEntry, bool) {
	for _, item := range models {
		if item.Key == modelID {
			return item, true
		}
	}
	return ModelEntry{}, false
}

func (owner *module) ask(
	ctx context.Context,
	stream *sdk.Stream,
	opID string,
	proxyOverride string,
	site Site,
	state *providerRuntimeState,
	stored Credential,
	cred Credential,
	chat map[string]any,
	named string,
	fallback bool,
	initialRenewed bool,
) (sdk.ExecuteResult, error) {
	modelID := rawStrField(chat, "model")
	streamMode := boolVal(chat["stream"])
	renewed := initialRenewed
	relogged := false
	renamed := false
	reclamped := false

	var entry ModelEntry
	var found bool

	cached, err := owner.loadModels(ctx, stream, proxyOverride, site, state, cred, false)
	if err == nil && cached != nil {
		entry, found = findModelEntry(cached.Models, modelID)
	}
	if !found {
		cached, err = owner.loadModels(ctx, stream, proxyOverride, site, state, cred, true)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, modelID, failureFromSignInError(site, err), renewed, nil)
		}
		entry, found = findModelEntry(cached.Models, modelID)
	}
	if !found && state.knowsModel(modelID) {
		if cached != nil && cached.Off[modelID] {
			if spent := owner.creditsSpent(ctx, stream, proxyOverride, site, state, stored); spent != "" {
				return emitFailureResponse(ctx, stream, opID, modelID, &qoderFailure{
					Status:    http.StatusTooManyRequests,
					Message:   fmt.Sprintf(`usage limit reached: the account's credits are used up (%s), and Qoder lists "%s" off for it until they refresh`, spent, modelID),
					SignIn:    "",
					HasSignIn: true,
				}, renewed, nil)
			}
		}
		relogged = true
		nextCred, didRenew, reloginErr := owner.relogin(ctx, stream, proxyOverride, site, state, stored, cred.Access)
		if reloginErr != nil {
			return emitFailureResponse(ctx, stream, opID, modelID, failureFromSignInError(site, reloginErr), renewed, nil)
		}
		if nextCred.Access != "" {
			cred = nextCred
			if didRenew || state.consumeUnsaid(cred) {
				renewed = true
			}
			reloaded, reloadErr := owner.loadModels(ctx, stream, proxyOverride, site, state, cred, true)
			if reloadErr != nil {
				return emitFailureResponse(ctx, stream, opID, modelID, failureFromSignInError(site, reloadErr), renewed, nil)
			}
			entry, found = findModelEntry(reloaded.Models, modelID)
		}
	}
	if !found {
		return emitFailureResponse(ctx, stream, opID, modelID, &qoderFailure{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf(`unknown or disabled model "%s"`, modelID),
		}, renewed, nil)
	}

	servedName := modelID
	if trimmed := strings.TrimSpace(entry.Name); trimmed != "" && trimmed != entry.Key {
		servedName = trimmed
	}

	resolved, err := siteOf(site, cred)
	if err != nil {
		return emitFailureResponse(ctx, stream, opID, servedName, failureFromSignInError(site, err), renewed, nil)
	}
	task := state.taskOf(sessionOf(chat, named), chat, owner.nowMs())
	body, err := owner.qoderBody(state, chat, entry, named, task, resolved)
	if err != nil {
		return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		}, renewed, nil)
	}
	tools := extractFunctionTools(chat)

	began := owner.now()
	deadline := began.Add(owner.queue.Budget)
	askState := queueAskState{
		RequestSetID: rawStrField(body, "request_set_id"),
		RequestID:    rawStrField(body, "request_id"),
		SessionID:    rawStrField(body, "session_id"),
		ModelKey:     entry.Key,
	}

	var held *heldQueueStream
	defer func() {
		if held != nil {
			held.stop()
		}
	}()

	waited := 0
	for sent := 0; ; sent++ {
		if sent > 0 {
			nextID := randomUUID()
			body["request_id"] = nextID
			body["chat_record_id"] = nextID
			askState.RequestID = nextID
		}
		resolved, err = siteOf(site, cred)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, servedName, failureFromSignInError(site, err), renewed, held)
		}
		chatURL := resolved.API + chatPath
		plainBytes, err := json.Marshal(body)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
				Status:  http.StatusBadRequest,
				Message: err.Error(),
			}, renewed, held)
		}
		wire := encodeBody(string(plainBytes))
		identity := owner.chatIdentity(site, state, cred.UID)
		headers, err := owner.chatHeaders(chatURL, cred, wire, resolved, entry, identity)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: err.Error(),
			}, renewed, held)
		}

		callCtx, cancelCall := context.WithCancel(ctx)
		var silent atomic.Bool
		hush := time.AfterFunc(owner.queue.Silent, func() {
			silent.Store(true)
			cancelCall()
		})

		req, err := http.NewRequestWithContext(callCtx, http.MethodPost, chatURL, strings.NewReader(wire))
		if err != nil {
			hush.Stop()
			cancelCall()
			return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: err.Error(),
			}, renewed, held)
		}
		req.Header = headers

		resp, cleanup, httpErr := owner.doHTTP(callCtx, stream, proxyOverride, req)
		var reader *sseEventReader
		var firstEvent streamEvent
		var hasFirst bool
		var openErr error
		if httpErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			reader = newSSEEventReader(resp.Body, tools)
			firstEvent, hasFirst, openErr = reader.Next()
		}
		hush.Stop()

		if httpErr != nil || openErr != nil {
			if resp != nil {
				_ = resp.Body.Close()
				cleanup()
			}
			cancelCall()
			if silent.Load() {
				phase := "no reply headers"
				if resp != nil {
					phase = "no first event"
				}
				who := firstNonEmpty(resolved.Name, "Qoder")
				secs := int(math.Round(owner.queue.Silent.Seconds()))
				return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
					Status:    http.StatusGatewayTimeout,
					Message:   fmt.Sprintf("%s sent nothing for %ds after the request (%s); the call was dropped", who, secs, phase),
					SignIn:    "",
					HasSignIn: true,
				}, renewed, held)
			}
			msg := ""
			if httpErr != nil {
				msg = httpErr.Error()
			} else {
				msg = openErr.Error()
			}
			return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: msg,
			}, renewed, held)
		}

		var fail *qoderFailure
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			rawErr, _ := readBodyLimited(resp.Body, 1<<20)
			fail = classifyFailure(resp.StatusCode, string(rawErr))
		} else if hasFirst && firstEvent.Error != nil {
			fail = firstEvent.Error
		}
		if fail != nil {
			_ = resp.Body.Close()
			cleanup()
			cancelCall()
		}

		if fail != nil && fail.Lapsed && !relogged {
			relogged = true
			nextCred, didRenew, reloginErr := owner.relogin(ctx, stream, proxyOverride, site, state, stored, cred.Access)
			if reloginErr != nil {
				return emitFailureResponse(ctx, stream, opID, servedName, failureFromSignInError(site, reloginErr), renewed, held)
			}
			if nextCred.Access != "" {
				cred = nextCred
				if didRenew || state.consumeUnsaid(cred) {
					renewed = true
				}
				continue
			}
		}
		if fail != nil && fail.Nameless && !renamed {
			renamed = true
			continue
		}
		if fail != nil && !reclamped && state.reclamp(fail, body, entry) {
			reclamped = true
			continue
		}
		if fail == nil {
			state.markTaskAnswered(task)
			res, emitErr := owner.emitAnswer(ctx, stream, opID, reader, firstEvent, hasFirst, streamMode, servedName, renewed, held)
			_ = resp.Body.Close()
			cleanup()
			cancelCall()
			return res, emitErr
		}
		if fail.Queue == nil || waited >= owner.queue.Recoveries || fallback {
			return emitFailureResponse(ctx, stream, opID, servedName, fail, renewed, held)
		}
		if streamMode && held == nil {
			started, holdErr := owner.startHeldQueue(ctx, stream, opID, servedName, fail, renewed)
			if holdErr != nil {
				return sdk.ExecuteResult{}, holdErr
			}
			held = started
			deadline = began.Add(owner.queue.Held)
		}
		admitted, waitErr := owner.queueWait(ctx, stream, proxyOverride, resolved.API, cred, &askState, fail.Queue, deadline, resolved)
		if waitErr != nil {
			return emitFailureResponse(ctx, stream, opID, servedName, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: waitErr.Error(),
			}, renewed, held)
		}
		if !admitted {
			outFail := *fail
			if askState.Stuck {
				mins := int(math.Round(owner.queue.Stuck.Minutes()))
				outFail.Message = fmt.Sprintf("%s (the queue hasn't moved in %d minutes with nothing ahead)", fail.Message, mins)
			}
			return emitFailureResponse(ctx, stream, opID, servedName, &outFail, renewed, held)
		}
		waited++
	}
}

func (owner *module) askLingma(
	ctx context.Context,
	stream *sdk.Stream,
	opID string,
	proxyOverride string,
	site Site,
	state *providerRuntimeState,
	stored Credential,
	cred Credential,
	chat map[string]any,
	named string,
	initialRenewed bool,
) (sdk.ExecuteResult, error) {
	modelID := rawStrField(chat, "model")
	streamMode := boolVal(chat["stream"])
	renewed := initialRenewed

	list := lingmaModelEntries()
	want, found := findModelEntry(list, modelID)
	if !found {
		keys := make([]string, 0, len(list))
		for _, item := range list {
			keys = append(keys, item.Key)
		}
		return emitFailureResponse(ctx, stream, opID, modelID, &qoderFailure{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf(`unknown model "%s"; this dedicated domain serves %s`, modelID, strings.Join(keys, ", ")),
		}, renewed, nil)
	}

	order := make([]ModelEntry, 0, len(list))
	order = append(order, want)
	for _, item := range list {
		if item.Key != want.Key {
			order = append(order, item)
		}
	}
	task := state.taskOf(sessionOf(chat, named), chat, owner.nowMs())
	tools := extractFunctionTools(chat)

	relogged := false
	reclamped := false
	for idx := 0; idx < len(order); {
		entry := order[idx]
		resolved, err := siteOf(site, cred)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, entry.Key, failureFromSignInError(site, err), renewed, nil)
		}
		body, err := owner.qoderBody(state, chat, entry, named, task, resolved)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, entry.Key, &qoderFailure{
				Status:  http.StatusBadRequest,
				Message: err.Error(),
			}, renewed, nil)
		}
		plainBytes, err := json.Marshal(body)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, entry.Key, &qoderFailure{
				Status:  http.StatusBadRequest,
				Message: err.Error(),
			}, renewed, nil)
		}
		wire := encodeBody(string(plainBytes))
		chatURL := resolved.API + chatPath
		headers, err := owner.lingmaHeaders(chatURL, cred, wire, resolved)
		if err != nil {
			return emitFailureResponse(ctx, stream, opID, entry.Key, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: err.Error(),
			}, renewed, nil)
		}

		callCtx, cancelCall := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(callCtx, http.MethodPost, chatURL, strings.NewReader(wire))
		if err != nil {
			cancelCall()
			return emitFailureResponse(ctx, stream, opID, entry.Key, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: err.Error(),
			}, renewed, nil)
		}
		req.Header = headers

		resp, cleanup, httpErr := owner.doHTTP(callCtx, stream, proxyOverride, req)
		var reader *sseEventReader
		var firstEvent streamEvent
		var hasFirst bool
		var openErr error
		if httpErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			reader = newSSEEventReader(resp.Body, tools)
			firstEvent, hasFirst, openErr = reader.Next()
		}
		if httpErr != nil || openErr != nil {
			if resp != nil {
				_ = resp.Body.Close()
				cleanup()
			}
			cancelCall()
			msg := ""
			if httpErr != nil {
				msg = httpErr.Error()
			} else {
				msg = openErr.Error()
			}
			return emitFailureResponse(ctx, stream, opID, entry.Key, &qoderFailure{
				Status:  http.StatusBadGateway,
				Message: msg,
			}, renewed, nil)
		}

		var fail *qoderFailure
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			rawErr, _ := readBodyLimited(resp.Body, 1<<20)
			fail = classifyFailure(resp.StatusCode, string(rawErr))
		} else if hasFirst && firstEvent.Error != nil {
			fail = firstEvent.Error
		}
		if fail == nil {
			state.markTaskAnswered(task)
			res, emitErr := owner.emitAnswer(ctx, stream, opID, reader, firstEvent, hasFirst, streamMode, entry.Key, renewed, nil)
			_ = resp.Body.Close()
			cleanup()
			cancelCall()
			return res, emitErr
		}
		_ = resp.Body.Close()
		cleanup()
		cancelCall()
		if fail.Lapsed || lingmaLapsedRe.MatchString(fail.Message) {
			if !relogged {
				relogged = true
				nextCred, didRenew, reloginErr := owner.relogin(ctx, stream, proxyOverride, site, state, stored, cred.Access)
				if reloginErr != nil {
					return emitFailureResponse(ctx, stream, opID, entry.Key, failureFromSignInError(site, reloginErr), renewed, nil)
				}
				if nextCred.Access != "" {
					cred = nextCred
					if didRenew || state.consumeUnsaid(cred) {
						renewed = true
					}
					continue
				}
			}
			return emitFailureResponse(ctx, stream, opID, entry.Key, &qoderFailure{
				Status:  http.StatusUnauthorized,
				Message: fmt.Sprintf("the sign-in lapsed — open Qoder CN IDE so it signs in again (%s)", truncateText(fail.Message, 300)),
			}, renewed, nil)
		}
		if !reclamped && state.reclamp(fail, body, entry) {
			reclamped = true
			continue
		}
		if lingmaNotHereRe.MatchString(fail.Message) && idx+1 < len(order) {
			idx++
			continue
		}
		return emitFailureResponse(ctx, stream, opID, entry.Key, fail, renewed, nil)
	}
	return emitFailureResponse(ctx, stream, opID, modelID, &qoderFailure{
		Status:  http.StatusBadGateway,
		Message: "no Lingma model accepted the request",
	}, renewed, nil)
}

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, state, err := owner.siteFor(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var req sdk.ExecuteRequest
	if err := json.Unmarshal(call.Params, &req); err != nil {
		return nil, invalid("Qoder requires a valid execute request")
	}
	if nonemptyRaw(req.Credentials) {
		return nil, invalid("Execution uses the selected host frame credentials")
	}
	if req.Protocol != "chat" || len(req.Body) == 0 || len(req.Body) > sdk.MaxNativeBodyBytes {
		return nil, invalid("Qoder requires a bounded Chat Completions request")
	}
	var execOpts sdk.ExecuteOptions
	if nonemptyRaw(req.Options) {
		_ = json.Unmarshal(req.Options, &execOpts)
	}
	var chat map[string]any
	if err := json.Unmarshal(req.Body, &chat); err != nil || chat == nil {
		return nil, invalid("Qoder requires a JSON Chat Completions body")
	}
	if rawStrField(chat, "model") == "" && req.Model != "" {
		chat["model"] = req.Model
	}
	stored, hasCred := decodeCredential(call.Auth)
	if !hasCred || stored.Type != "oauth" || stored.Access == "" || stored.UID == "" {
		return emitFailureResponse(ctx, stream, call.Scope.Operation, rawStrField(chat, "model"), &qoderFailure{
			Status:  http.StatusUnauthorized,
			Message: "not signed in",
		}, false, nil)
	}
	cred, didRenew, err := owner.fresh(ctx, stream, execOpts.Proxy, site, state, stored)
	if err != nil {
		return emitFailureResponse(ctx, stream, call.Scope.Operation, rawStrField(chat, "model"), failureFromSignInError(site, err), false, nil)
	}
	renewed := didRenew || state.consumeUnsaid(cred)

	named := namedSession(firstNonEmpty(
		req.Headers.Get(sessionHeader),
		req.Headers.Get("X-DeepSeek-Harness-Session-Id"),
		call.Scope.Session,
	))
	fallback := req.Headers.Get(fallbackHeader) == "1"
	if cred.Lingma {
		return owner.askLingma(ctx, stream, call.Scope.Operation, execOpts.Proxy, site, state, stored, cred, chat, named, renewed)
	}
	return owner.ask(ctx, stream, call.Scope.Operation, execOpts.Proxy, site, state, stored, cred, chat, named, fallback, renewed)
}
