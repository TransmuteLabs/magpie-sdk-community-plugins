package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const maxBoundedSystemCache = 256

type syntheticReply struct {
	Status  int
	Headers http.Header
	Body    []byte
}

func notFreePayload(site Site, model, why string) syntheticReply {
	label := model
	if label == "" {
		label = "(none)"
	}
	var msg, code string
	status := http.StatusNotFound
	if why != "" {
		status = http.StatusServiceUnavailable
		code = "free_only_unknown"
		msg = fmt.Sprintf(`%s's model list couldn't be read (%s), so whether model %s is free is unknown; with "Free models only" on it isn't sent`, site.Name, why, label)
	} else {
		code = "model_not_found"
		msg = fmt.Sprintf(`model %s is not available while "Free models only" is on: %s charges for it. Turn the switch off in %s's provider settings to use it`, label, site.Name, site.Name)
	}
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
	return syntheticReply{
		Status: status,
		Headers: http.Header{
			"Content-Type":     {"application/json"},
			"X-Magpie-Sign-In": {"kept"},
		},
		Body: body,
	}
}

func emitSyntheticReply(ctx context.Context, stream *sdk.Stream, opID string, reply syntheticReply) (sdk.ExecuteResult, error) {
	hdr := cloneHeaders(reply.Headers)
	if hdr.Get("Content-Type") == "" {
		hdr.Set("Content-Type", "application/json")
	}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: reply.Status, Headers: hdr}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if len(reply.Body) > 0 {
		if err := stream.Emit(ctx, "data", reply.Body); err != nil {
			return sdk.ExecuteResult{}, err
		}
	}
	return sdk.ExecuteResult{Status: reply.Status, Headers: hdr, Outcome: "completed"}, nil
}

func resolveExecuteURL(site Site, request sdk.ExecuteRequest, execOpts sdk.ExecuteOptions) (string, string, error) {
	method := firstNonEmpty(execOpts.Method, http.MethodPost)
	proto := strings.ToLower(strings.TrimSpace(request.Protocol))
	isMessages := proto == "messages" || proto == "anthropic"
	if isMessages && !site.Messages {
		return "", "", invalid(fmt.Sprintf("%s supports Chat Completions requests only", site.Name))
	}
	if proto != "" && proto != "chat" && !isMessages {
		return "", "", invalid(fmt.Sprintf("%s does not support protocol %q", site.Name, request.Protocol))
	}
	targetPath := strings.TrimSpace(execOpts.Path)
	if targetPath == "" {
		if isMessages {
			if request.Operation == "count_tokens" {
				targetPath = "/v1/messages/count_tokens"
			} else {
				targetPath = "/v1/messages"
			}
		} else {
			targetPath = "/v2/chat/completions"
		}
	} else {
		if !strings.HasPrefix(targetPath, "/") {
			targetPath = "/" + targetPath
		}
		if strings.HasPrefix(targetPath, "/messages") {
			targetPath = "/v1" + targetPath
		} else if strings.HasPrefix(targetPath, "/chat/") || strings.HasPrefix(targetPath, "/images/") {
			targetPath = "/v2" + targetPath
		}
	}
	rawURL := site.Endpoint + targetPath
	if strings.TrimSpace(execOpts.Query) != "" {
		rawURL += "?" + strings.TrimPrefix(strings.TrimSpace(execOpts.Query), "?")
	}
	return method, rawURL, nil
}

func (st *siteRuntimeState) hasRefusedSystem(sysKey string) bool {
	if sysKey == "" {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.refusedSet[sysKey]
}

func (st *siteRuntimeState) addRefusedSystem(sysKey string) {
	if sysKey == "" {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.refusedSet[sysKey] {
		return
	}
	st.refusedSet[sysKey] = true
	st.refusedSystems = append(st.refusedSystems, sysKey)
	if len(st.refusedSystems) > maxBoundedSystemCache {
		oldest := st.refusedSystems[0]
		st.refusedSystems = st.refusedSystems[1:]
		delete(st.refusedSet, oldest)
	}
}

func (st *siteRuntimeState) hasCommaSystem(sysKey string) bool {
	if sysKey == "" {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.commaSet[sysKey]
}

func (st *siteRuntimeState) addCommaSystem(sysKey string) {
	if sysKey == "" {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.commaSet[sysKey] {
		return
	}
	st.commaSet[sysKey] = true
	st.commaSystems = append(st.commaSystems, sysKey)
	if len(st.commaSystems) > maxBoundedSystemCache {
		oldest := st.commaSystems[0]
		st.commaSystems = st.commaSystems[1:]
		delete(st.commaSet, oldest)
	}
}

func (st *siteRuntimeState) hasEndsAfterTools(modelID string) bool {
	if modelID == "" {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.endsAfterTools[modelID]
}

func (st *siteRuntimeState) addEndsAfterTools(modelID string) {
	if modelID == "" {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.endsAfterTools[modelID] = true
}

func (owner *module) unansweredProbe(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, wireBody []byte, sendErr error) (*http.Response, string, bool) {
	if ctx.Err() != nil || errors.Is(sendErr, context.Canceled) || errors.Is(sendErr, context.DeadlineExceeded) {
		return nil, "", false
	}
	size := len(wireBody)
	if size <= EdgeReadsBytes || acct.Source == "key" {
		return nil, "", false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	probeResp, err := owner.meterAsked(probeCtx, stream, site, acct, "/billing/meter/get-user-resource-summary", map[string]any{})
	if err != nil {
		return nil, "", false
	}
	status := probeResp.StatusCode
	challenge := probeResp.Header.Get("WWW-Authenticate")
	_, _ = io.Copy(io.Discard, io.LimitReader(probeResp.Body, 4096))
	_ = probeResp.Body.Close()
	if status != http.StatusUnauthorized {
		return nil, "", false
	}
	mb := fmt.Sprintf("%.1f", float64(size)/(1024*1024))
	note := fmt.Sprintf("It closes the connection on a refused request over 2 MiB (this one was %s MiB) instead of answering, which is why it read as ECONNRESET.", mb)
	tail := fmt.Sprintf("Sign in to %s again in magpie.", site.Name)
	if acct.Source == "desktop" {
		tail = fmt.Sprintf("Sign in to %s desktop again.", site.Name)
	}
	msg := fmt.Sprintf("%s refused this account's sign-in (401). %s %s", site.Name, note, tail)
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "authentication_error",
			"code":    401,
		},
	})
	hdr := http.Header{"Content-Type": {"application/json"}}
	if challenge != "" {
		hdr.Set("WWW-Authenticate", challenge)
	}
	synth := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
		Header:     hdr,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}
	return synth, note, true
}

func (owner *module) sendReports(site Site, acct Credential, proxyURL string, batches [][]map[string]any) {
	if len(batches) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, cleanup, err := owner.options.HTTPFactory(proxyURL)
	if err != nil {
		return
	}
	defer cleanup()
	reportURL := endpointOf(site, acct) + "/v2/report"
	for _, batch := range batches {
		encoded, err := json.Marshal(batch)
		if err != nil {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reportURL, bytes.NewReader(encoded))
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", cliAgent(site))
		req.Header.Set("X-Request-ID", randomHex(16))
		req.Header.Set("X-Product", "SaaS")
		req.Header.Set("Authorization", "Bearer "+acct.Access)
		req.Header.Set("X-User-Id", acct.UID)
		req.Header.Set("X-Domain", domainOf(site, acct))
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		for key, val := range enterpriseHeaders(acct) {
			req.Header.Set(key, val)
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}
}

func appendTail(tail []byte, chunk []byte) []byte {
	if len(chunk) == 0 {
		return tail
	}
	if len(chunk) >= TailBytes {
		return append([]byte(nil), chunk[len(chunk)-TailBytes:]...)
	}
	combined := append(tail, chunk...)
	if len(combined) > TailBytes {
		combined = combined[len(combined)-TailBytes:]
	}
	return combined
}

func extractChatUsageObj(obj map[string]any) *sdk.Usage {
	if obj == nil {
		return nil
	}
	usageMap, ok := obj["usage"].(map[string]any)
	if !ok || usageMap == nil {
		return nil
	}
	promptTok, hasPrompt := asInt64(firstPresent(usageMap, "prompt_tokens", "input_tokens"))
	compTok, hasComp := asInt64(firstPresent(usageMap, "completion_tokens", "output_tokens"))
	if !hasPrompt && !hasComp {
		return nil
	}
	var cachedTok int64
	if details, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
		if val, ok := asInt64(details["cached_tokens"]); ok {
			cachedTok = val
		}
	} else if val, ok := asInt64(usageMap["cache_read_input_tokens"]); ok {
		cachedTok = val
	}
	var cacheWrite int64
	if val, ok := asInt64(usageMap["cache_creation_input_tokens"]); ok {
		cacheWrite = val
	}
	return &sdk.Usage{
		InputTokens:      promptTok,
		OutputTokens:     compTok,
		CachedTokens:     cachedTok,
		CacheWriteTokens: cacheWrite,
	}
}

func extractUsageFromSSELine(line string, native bool, running *sdk.Usage) *sdk.Usage {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return nil
	}
	var doc map[string]any
	if json.Unmarshal([]byte(payload), &doc) != nil || doc == nil {
		return nil
	}
	if !native {
		return extractChatUsageObj(doc)
	}
	eventType := strField(doc, "type")
	if eventType == "message_start" {
		if msg, ok := doc["message"].(map[string]any); ok {
			return extractChatUsageObj(msg)
		}
	}
	if eventType == "message_delta" {
		deltaUsage := extractChatUsageObj(doc)
		if deltaUsage == nil {
			return nil
		}
		if running != nil {
			merged := *running
			if deltaUsage.InputTokens > 0 {
				merged.InputTokens = deltaUsage.InputTokens
			}
			if deltaUsage.OutputTokens > 0 {
				merged.OutputTokens = deltaUsage.OutputTokens
			}
			if deltaUsage.CachedTokens > 0 {
				merged.CachedTokens = deltaUsage.CachedTokens
			}
			if deltaUsage.CacheWriteTokens > 0 {
				merged.CacheWriteTokens = deltaUsage.CacheWriteTokens
			}
			return &merged
		}
		return deltaUsage
	}
	return extractChatUsageObj(doc)
}

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ExecuteRequest
	if decodeObject(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, invalid("Execution uses the selected host frame credentials")
	}
	if len(request.Body) > sdk.MaxNativeBodyBytes {
		return nil, invalid("Request body exceeds bounded native limit")
	}
	site, ok := owner.siteFor(call.Scope.Provider)
	if !ok {
		return nil, invalid("Unknown WorkBuddy provider")
	}
	if call.Scope.Account == "" || call.Scope.Generation == 0 {
		return nil, denied("An exact selected WorkBuddy account is required")
	}
	var execOpts sdk.ExecuteOptions
	if nonempty(request.Options) && decodeObject(request.Options, &execOpts) != nil {
		return nil, invalid("Invalid execution options")
	}
	method, rawURL, err := resolveExecuteURL(site, request, execOpts)
	if err != nil {
		return nil, err
	}

	var stored Credential
	if decodeObject(call.Auth, &stored) != nil {
		return nil, unauthenticated(fmt.Sprintf("%s isn't signed in", site.Name))
	}
	var active Credential
	if stored.isAPIKey() {
		keyed, valid := keyAccount(firstNonEmpty(stored.Key, stored.Access))
		if !valid {
			return nil, unauthenticated(fmt.Sprintf("%s isn't signed in", site.Name))
		}
		active = keyed
	} else if stored.isOAuth() {
		active, err = owner.current(ctx, stream, site, stored)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, unauthenticated(fmt.Sprintf("%s isn't signed in", site.Name))
	}

	headers := cloneHeaders(request.Headers)
	headers.Del("Authorization")
	headers.Del("X-Api-Key")
	headers.Del("Content-Length")
	headers.Del("Content-Encoding")
	headers.Del("Host")
	session := sessionOf(headers, "")
	signHeaders(site, active, headers)

	bodyBytes := slices.Clone(request.Body)
	var chat map[string]any
	if len(bytes.TrimSpace(bodyBytes)) > 0 {
		var decoded map[string]any
		if json.Unmarshal(bodyBytes, &decoded) == nil && decoded != nil {
			chat = decoded
		}
	}
	native := isMessagesRequest(request.Protocol, rawURL, chat)
	changed := false
	var names map[string]string
	if chat != nil {
		if native {
			if unsignedOn(chat) {
				changed = true
			}
		} else {
			if systemOn(chat) {
				changed = true
			}
			fitChanged, fitNames := fitTools(chat)
			if fitChanged {
				changed = true
			}
			names = fitNames
			if toolChoiceOn(chat) {
				changed = true
			}
		}
		if owner.hideDSHEnabled(ctx, site) && hideDSH(chat) {
			changed = true
		}
	}

	modelID := firstNonEmpty(strField(chat, "model"), request.Model)
	if owner.freeOnly(ctx, site) {
		list, listErr := owner.listedModels(ctx, stream, site, active, false)
		if listErr != nil {
			return emitSyntheticReply(ctx, stream, call.Scope.Operation, notFreePayload(site, modelID, listErr.Error()))
		}
		isFree := false
		for _, item := range list {
			if item.ID == modelID && item.Free {
				isFree = true
				break
			}
		}
		if !isFree {
			return emitSyntheticReply(ctx, stream, call.Scope.Operation, notFreePayload(site, modelID, ""))
		}
	}

	completion := !native && isCompletionRequest(rawURL, chat)
	if completion && chat != nil && !namesEffort(chat) {
		if withDefaultEffort(chat, owner.effortList(ctx, stream, site, active)) {
			changed = true
		}
	}
	owner.chatHeaders(site, headers)
	who := scopeOf(active)
	ids := owner.attendChat(headers, chat, session, who)
	grownFor := ""
	if completion && withGrowth(site, chat, headers.Get("X-Conversation-ID"), "") {
		changed = true
		grownFor = headers.Get("X-Conversation-ID")
	}
	var step *stepInfo
	if completion && site.AI && chat != nil {
		if _, hasMsgs := chat["messages"].([]any); hasMsgs {
			step = stepOf(chat)
		}
	}

	state := owner.stateFor(site)
	sysKey := ""
	if !native {
		sysKey = systemKeyOf(chat)
	}
	if sysKey != "" && state.hasRefusedSystem(sysKey) && systemAsUserOn(chat) {
		changed = true
	}
	if sysKey != "" && state.hasCommaSystem(sysKey) && commaPausedOn(chat, nil) {
		changed = true
	}
	if !native && state.hasEndsAfterTools(modelID) && replyAfterToolsOn(chat) {
		changed = true
	}
	if changed && chat != nil {
		encoded, err := json.Marshal(chat)
		if err != nil {
			return nil, invalid("Failed to serialize WorkBuddy request")
		}
		bodyBytes = encoded
	}
	chat = nil

	shaped := func(raw []byte) []byte {
		if !native && state.hasEndsAfterTools(modelID) {
			if adjusted, ok := replyAfterTools(raw); ok {
				return adjusted
			}
		}
		return raw
	}

	resolvedProxyURL := ""
	postOnce := func(raw []byte) (*http.Response, string, error) {
		toSend := raw
		conv := headers.Get("X-Conversation-ID")
		if grownFor != "" && conv != "" && conv != grownFor {
			if adjusted, ok := regrown(site, toSend, grownFor, conv); ok {
				toSend = adjusted
			}
		}
		reqHeaders := cloneHeaders(headers)
		wire := toSend
		if gz, ok := gzipBody(completion, toSend); ok {
			wire = gz
			reqHeaders.Set("Content-Encoding", "gzip")
		} else {
			reqHeaders.Del("Content-Encoding")
		}
		if reqHeaders.Get("Content-Type") == "" && len(wire) > 0 {
			reqHeaders.Set("Content-Type", "application/json")
		}
		httpReq, err := http.NewRequestWithContext(ctx, method, movedURL(site, active, rawURL), bytes.NewReader(wire))
		if err != nil {
			return nil, "", err
		}
		httpReq.Header = reqHeaders
		resp, proxyURL, err := owner.doHTTP(ctx, stream, httpReq)
		if proxyURL != "" {
			resolvedProxyURL = proxyURL
		}
		if err != nil {
			if synth, note, ok := owner.unansweredProbe(ctx, stream, site, active, wire, err); ok {
				return synth, note, nil
			}
			return nil, "", err
		}
		return resp, "", nil
	}

	renewed := false
	sendAttempt := func(raw []byte) (*http.Response, *syntheticReply, error) {
		resp, note, err := postOnce(raw)
		if err != nil {
			return nil, nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized {
			return resp, nil, nil
		}
		invalidTok := isInvalidToken(resp)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if renewed {
			body, hdr := signedOutPayload(site, active, "it refused the token renewed for it too", invalidTok, note)
			return nil, &syntheticReply{Status: http.StatusUnauthorized, Headers: hdr, Body: body}, nil
		}
		if active.Source == "key" {
			body, hdr := keyRefusedPayload(site, invalidTok, note)
			return nil, &syntheticReply{Status: http.StatusUnauthorized, Headers: hdr, Body: body}, nil
		}
		reissue := owner.reissued(ctx, stream, site, active)
		if !reissue.HasCred {
			body, hdr := signedOutPayload(site, active, reissue.Why, isGone(active, reissue, invalidTok), note)
			return nil, &syntheticReply{Status: http.StatusUnauthorized, Headers: hdr, Body: body}, nil
		}
		renewed = true
		active = reissue.Cred
		signHeaders(site, active, headers)
		nextWho := scopeOf(active)
		ids = owner.reattend(headers, ids, who, nextWho)
		who = nextWho
		resp2, note2, err2 := postOnce(raw)
		if err2 != nil {
			return nil, nil, err2
		}
		if resp2.StatusCode != http.StatusUnauthorized {
			return resp2, nil, nil
		}
		invalidTok2 := isInvalidToken(resp2)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp2.Body, 4096))
		_ = resp2.Body.Close()
		body, hdr := signedOutPayload(site, active, "it refused the token renewed for it too", invalidTok2, note2)
		return nil, &syntheticReply{Status: http.StatusUnauthorized, Headers: hdr, Body: body}, nil
	}

	startMs := owner.nowMs()
	resp, synth, err := sendAttempt(bodyBytes)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &sdk.Error{Code: "upstream", Message: err.Error(), Outcome: "uncertain"}
	}
	if synth != nil {
		return emitSyntheticReply(ctx, stream, call.Scope.Operation, *synth)
	}

	var bufferedErr []byte
	if native && resp.StatusCode >= 400 {
		rawErr, _ := io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
		_ = resp.Body.Close()
		if badSignatureRe.Match(rawErr) {
			if stripped, ok := withoutSignedThinking(bodyBytes); ok {
				resp2, synth2, err2 := sendAttempt(stripped)
				if err2 != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return nil, &sdk.Error{Code: "upstream", Message: err2.Error(), Outcome: "uncertain"}
				}
				if synth2 != nil {
					return emitSyntheticReply(ctx, stream, call.Scope.Operation, *synth2)
				}
				resp = resp2
				if resp.StatusCode >= 400 {
					bufferedErr, _ = io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
					_ = resp.Body.Close()
				}
			} else {
				bufferedErr = rawErr
			}
		} else {
			bufferedErr = rawErr
		}
	} else if !native && resp.StatusCode == http.StatusBadRequest {
		rawErr, _ := io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
		_ = resp.Body.Close()
		errText := string(rawErr)
		bufferedErr = rawErr

		if refusedRe.MatchString(errText) {
			roles := map[string]bool{}
			if paused, ok := commaPaused(bodyBytes, roles); ok {
				bodyBytes = paused
				resp2, synth2, err2 := sendAttempt(shaped(bodyBytes))
				if err2 != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return nil, &sdk.Error{Code: "upstream", Message: err2.Error(), Outcome: "uncertain"}
				}
				if synth2 != nil {
					return emitSyntheticReply(ctx, stream, call.Scope.Operation, *synth2)
				}
				resp = resp2
				if resp.StatusCode == http.StatusBadRequest {
					bufferedErr, _ = io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
					_ = resp.Body.Close()
					errText = string(bufferedErr)
				} else {
					bufferedErr = nil
					if sysKey != "" && roles["system"] {
						state.addCommaSystem(sysKey)
					}
				}
			}
		}
		if resp.StatusCode == http.StatusBadRequest && refusedRe.MatchString(errText) && sysKey != "" && !state.hasRefusedSystem(sysKey) {
			if moved, ok := systemAsUser(bodyBytes); ok {
				state.addRefusedSystem(sysKey)
				bodyBytes = moved
				resp2, synth2, err2 := sendAttempt(shaped(bodyBytes))
				if err2 != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return nil, &sdk.Error{Code: "upstream", Message: err2.Error(), Outcome: "uncertain"}
				}
				if synth2 != nil {
					return emitSyntheticReply(ctx, stream, call.Scope.Operation, *synth2)
				}
				resp = resp2
				if resp.StatusCode == http.StatusBadRequest {
					bufferedErr, _ = io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
					_ = resp.Body.Close()
					errText = string(bufferedErr)
				} else {
					bufferedErr = nil
				}
			}
		}
		if resp.StatusCode == http.StatusBadRequest && !state.hasEndsAfterTools(modelID) && isAfterToolsRefused(errText) {
			if again, ok := replyAfterTools(bodyBytes); ok {
				state.addEndsAfterTools(modelID)
				resp2, synth2, err2 := sendAttempt(again)
				if err2 != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return nil, &sdk.Error{Code: "upstream", Message: err2.Error(), Outcome: "uncertain"}
				}
				if synth2 != nil {
					return emitSyntheticReply(ctx, stream, call.Scope.Operation, *synth2)
				}
				resp = resp2
				if resp.StatusCode >= 400 {
					bufferedErr, _ = io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
					_ = resp.Body.Close()
				} else {
					bufferedErr = nil
				}
			}
		}
	}

	if resp.StatusCode >= 400 {
		if bufferedErr == nil {
			bufferedErr, _ = io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
			_ = resp.Body.Close()
		}
		explainedBody, explainedHdr := explainErrorBody(resp.StatusCode, resp.Header, bufferedErr)
		return emitSyntheticReply(ctx, stream, call.Scope.Operation, syntheticReply{
			Status:  resp.StatusCode,
			Headers: explainedHdr,
			Body:    explainedBody,
		})
	}

	defer resp.Body.Close()
	signInWord := "kept"
	if renewed {
		signInWord = "renewed"
	}
	outHdr := cloneHeaders(resp.Header)
	outHdr.Del("Content-Length")
	outHdr.Set("X-Magpie-Sign-In", signInWord)
	if outHdr.Get("Content-Type") == "" {
		outHdr.Set("Content-Type", "application/json")
	}
	isSSE := strings.Contains(strings.ToLower(outHdr.Get("Content-Type")), "text/event-stream")

	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: resp.StatusCode, Headers: outHdr}); err != nil {
		return nil, err
	}

	var usage *sdk.Usage
	var tailBuf []byte
	if !isSSE {
		rawBody, readErr := io.ReadAll(io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes))
		if readErr != nil {
			return nil, &sdk.Error{Code: "upstream", Message: readErr.Error(), Outcome: "uncertain"}
		}
		if !native && len(names) > 0 {
			rawBody = renameJSONBody(rawBody, names)
		}
		if len(rawBody) > 0 {
			if err := stream.Emit(ctx, "data", rawBody); err != nil {
				return nil, err
			}
			tailBuf = appendTail(tailBuf, rawBody)
			var doc map[string]any
			if json.Unmarshal(rawBody, &doc) == nil && doc != nil {
				if extracted := extractChatUsageObj(doc); extracted != nil {
					usage = extracted
					_ = stream.Emit(ctx, "usage", *usage)
				}
			}
		}
	} else {
		var linePending bytes.Buffer
		var filter *unsaidFilter
		if !native {
			filter = newUnsaidFilter()
		}
		emitChunk := func(payload []byte) error {
			if len(payload) == 0 {
				return nil
			}
			tailBuf = appendTail(tailBuf, payload)
			return stream.Emit(ctx, "data", payload)
		}
		processLine := func(line string, withNewline bool) error {
			outLine := line
			if !native && len(names) > 0 {
				outLine = renameSSELine(outLine, names)
			}
			if extracted := extractUsageFromSSELine(outLine, native, usage); extracted != nil {
				usage = extracted
				_ = stream.Emit(ctx, "usage", *usage)
			}
			piece := outLine
			if withNewline {
				piece += "\n"
			}
			if filter != nil {
				filtered := filter.push(piece)
				if filtered != "" {
					return emitChunk([]byte(filtered))
				}
				return nil
			}
			return emitChunk([]byte(piece))
		}
		buf := make([]byte, 32*1024)
		for {
			count, readErr := resp.Body.Read(buf)
			if count > 0 {
				linePending.Write(buf[:count])
				for {
					data := linePending.Bytes()
					idx := bytes.IndexByte(data, '\n')
					if idx < 0 {
						break
					}
					line := string(data[:idx])
					linePending.Next(idx + 1)
					if err := processLine(line, true); err != nil {
						return nil, err
					}
				}
			}
			if readErr == io.EOF {
				if linePending.Len() > 0 {
					if err := processLine(linePending.String(), false); err != nil {
						return nil, err
					}
				}
				if filter != nil {
					if rest := filter.flush(); rest != "" {
						if err := emitChunk([]byte(rest)); err != nil {
							return nil, err
						}
					}
				}
				break
			}
			if readErr != nil {
				return nil, &sdk.Error{Code: "upstream", Message: readErr.Error(), Outcome: "uncertain"}
			}
		}
	}

	conv := headers.Get("X-Conversation-ID")
	if step != nil && conv != "" {
		stepCopy := *step
		stepCopy.Conv = conv
		stepCopy.Turn = headers.Get("X-Conversation-Request-ID")
		stepCopy.Message = headers.Get("X-Conversation-Message-ID")
		facts := replyFacts(string(tailBuf))
		batches := owner.reportEvents(site, active, &stepCopy, facts, startMs, owner.nowMs())
		if len(batches) > 0 {
			if owner.options.SyncReports {
				owner.sendReports(site, active, resolvedProxyURL, batches)
			} else {
				activeSnap := active
				proxySnap := resolvedProxyURL
				owner.reportWG.Add(1)
				go func() {
					defer owner.reportWG.Done()
					owner.sendReports(site, activeSnap, proxySnap, batches)
				}()
			}
		}
	}

	return sdk.ExecuteResult{
		Status:  resp.StatusCode,
		Headers: outHdr,
		Usage:   usage,
		Outcome: "completed",
	}, nil
}
