package codearts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func (st *accountState) isBenefitModel(model string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	var free bool
	if st.hasActiveBen {
		free = st.benefit[model]
	} else {
		free = slices.Contains(benefitModelFallback, model)
	}
	if !free || st.noBenefit[model] {
		return false
	}
	if st.hasRegular && st.regular[model] && st.aliasRefusals[model] {
		return false
	}
	return true
}

func (st *accountState) canSendBenefit(model, day string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.hasActiveBen && st.benefit[model] &&
		!st.noBenefit[model] && !st.aliasRefusals[model] &&
		st.claim != nil && st.claim.Day == day && st.claim.OK &&
		st.daily != nil && st.daily.Day == day && st.daily.Fresh &&
		st.daily.Window != nil && st.daily.Window.Used < 100
}

func (st *accountState) isRegularModel(model string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.hasRegular {
		return true
	}
	return st.regular[model]
}

func (st *accountState) isDailyExhaustedForAuto(model string, useBenefit bool, day string) (exhausted bool, allowedPaid bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if (useBenefit || (st.hasActiveBen && st.benefit[model])) &&
		st.daily != nil && st.daily.Day == day && st.daily.Window != nil && st.daily.Window.Used >= 100 {
		return true, st.hasRegular && st.regular[model]
	}
	return false, true
}

func (st *accountState) acquire(ctx context.Context) (func(), error) {
	select {
	case st.gate <- struct{}{}:
		released := false
		return func() {
			if !released {
				released = true
				<-st.gate
			}
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func emitJSONError(ctx context.Context, stream *sdk.Stream, opID string, status int, code, message string, extra map[string]string) (sdk.ExecuteResult, error) {
	hdr := http.Header{"Content-Type": {"application/json"}}
	for k, v := range extra {
		hdr.Set(k, v)
	}
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"type":    "provider_error",
		},
	})
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: hdr}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if err := stream.Emit(ctx, "data", payload); err != nil {
		return sdk.ExecuteResult{}, err
	}
	return sdk.ExecuteResult{Status: status, Headers: hdr, Outcome: "completed"}, nil
}

func emitBenefitUnavailable(ctx context.Context, stream *sdk.Stream, opID, code string) (sdk.ExecuteResult, error) {
	return emitJSONError(ctx, stream, opID, http.StatusNotFound, code, "CodeArts benefit model is not available for this account", nil)
}

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ExecuteRequest
	if decodeObject(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, invalid("Execution uses the selected host frame credentials")
	}
	if request.Protocol != "chat" || len(request.Body) == 0 || len(request.Body) > sdk.MaxNativeBodyBytes {
		return nil, invalid("CodeArts requires a bounded Chat Completions request")
	}
	provider := call.Scope.Provider
	if (provider != CodeArtsProvider && provider != OfficeAceProvider) || call.Scope.Account == "" || call.Scope.Generation == 0 {
		return nil, denied("An exact selected CodeArts or OfficeAce account is required")
	}
	var cred CodeArtsCredential
	if decodeObject(call.Auth, &cred) != nil {
		return nil, denied("Selected credentials do not match the call")
	}
	if provider == OfficeAceProvider {
		return owner.executeOfficeAce(ctx, call, stream, request, cred)
	}
	if !cred.ValidSTS() {
		return nil, denied("Selected CodeArts STS credentials are invalid")
	}
	return owner.executeCodeArts(ctx, call, stream, request, cred)
}

func (owner *module) executeCodeArts(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.ExecuteRequest, cred CodeArtsCredential) (any, error) {
	var parsed map[string]any
	if json.Unmarshal(request.Body, &parsed) != nil || parsed == nil {
		return nil, invalid("CodeArts requires a JSON Chat Completions body")
	}
	rawModel, _ := parsed["model"].(string)
	if request.Model == "" || rawModel != request.Model {
		return nil, invalid("Chat model must match the selected model")
	}
	requested := UpstreamModelID(rawModel)
	benefitOnly := strings.HasSuffix(requested, BenefitModelSuffix)
	model := requested
	if benefitOnly {
		model = strings.TrimSuffix(requested, BenefitModelSuffix)
	}
	station := owner.stationOf(&cred)
	parsedForPrepare := make(map[string]any, len(parsed))
	for k, v := range parsed {
		parsedForPrepare[k] = v
	}
	if benefitOnly {
		parsedForPrepare["model"] = model
	}
	chat := PrepareChatBody(parsedForPrepare, owner.thinkingDefault(), station.ID)
	if chat == nil {
		return nil, invalid("CodeArts requires model and messages")
	}
	bodyBytes, err := encodeJSONNoEscape(chat.Body)
	if err != nil {
		return nil, err
	}
	if len(bodyBytes) > MaxSignedBodyBytes {
		mb := float64(len(bodyBytes)) / (1024 * 1024)
		return emitJSONError(ctx, stream, call.Scope.Operation, http.StatusRequestEntityTooLarge, "",
			fmt.Sprintf("CodeArts 网关只接受 12 MB 以内的请求，这次请求有 %.1f MB（通常是对话里的图片）。请新开会话，或去掉较大的图片后再试", mb), nil)
	}
	if benefitOnly && station.OpenGW == "" {
		return emitBenefitUnavailable(ctx, stream, call.Scope.Operation, "")
	}

	id, _ := AccountIdentityOf(cred)
	st := owner.stateFor(firstOf(call.Scope.Account, id, cred.AccessKeyID))
	release, err := st.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	sessionID, chatID := AccountSessionIDs(cred)
	targetURL := strings.TrimRight(station.Snap, "/") + "/api/v2/chat/completions"
	useBenefit := benefitOnly || st.isBenefitModel(model)
	active := cred
	renewed := false
	claimTried := false

	for attempt := 0; attempt < 6; attempt++ {
		day := beijingDay(owner.options.Now().UnixMilli())
		if benefitOnly && !st.canSendBenefit(model, day) {
			return emitBenefitUnavailable(ctx, stream, call.Scope.Operation, "")
		}
		exhausted, allowedPaid := false, true
		if !benefitOnly {
			exhausted, allowedPaid = st.isDailyExhaustedForAuto(model, useBenefit, day)
			if exhausted && !allowedPaid {
				return emitBenefitUnavailable(ctx, stream, call.Scope.Operation, "")
			}
		}
		signedBenefit := useBenefit && !exhausted
		var extraSigned map[string]string
		if signedBenefit {
			extraSigned = map[string]string{"maas_type": "benefit"}
		}
		signed, signErr := SignRequestHuawei(active.AccessKeyID, active.SecretAccessKey, active.SecurityToken,
			http.MethodPost, targetURL, bodyBytes, extraSigned, sdkDateStamp(owner.options.Now()))
		if signErr != nil {
			return nil, signErr
		}
		st.mu.Lock()
		sentClaim := st.claim
		st.mu.Unlock()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		for key, vals := range request.Headers {
			lower := strings.ToLower(key)
			if lower == "authorization" || lower == "x-api-key" || lower == "cookie" ||
				lower == "host" || lower == "content-length" || lower == "maas_type" ||
				strings.HasPrefix(lower, "x-magpie-") {
				continue
			}
			req.Header[key] = append([]string(nil), vals...)
		}
		for k, v := range signed {
			if !strings.EqualFold(k, "host") {
				req.Header.Set(k, v)
			}
		}
		req.Header.Set("Chat-Id", chatID)
		req.Header.Set("Session-Id", sessionID)
		req.Header.Set(SessionHeader, sessionID)
		req.Header.Set("user-session-id", sessionID)
		req.Header.Set("x-ot-parent-session-id", "")
		req.Header.Set("x-ot-trace-id", randomHex(16))
		req.Header.Set("x-ot-span-id", randomHex(8))
		req.Header.Set("x-snap-traceid", randomHex(16))
		req.Header.Set("lang", "en")

		resp, err := owner.doHTTP(ctx, stream, req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, uncertain("CodeArts request failed: " + err.Error())
		}

		if resp.StatusCode == http.StatusTooManyRequests && strings.EqualFold(resp.Header.Get("queue-status"), "queued") {
			retryAfter := firstOf(resp.Header.Get("retry-after"), "5")
			_ = resp.Body.Close()
			var retryMs *int64
			if sec, err := strconv.ParseInt(retryAfter, 10, 64); err == nil && sec > 0 {
				ms := sec * 1000
				retryMs = &ms
			}
			return sdk.RefusedExecution(sdk.RequestRecoveryRequest{
				Step:     "codearts-chat",
				Evidence: "rejected-before-effect",
				Facts: sdk.RequestFailureFacts{
					Status:       http.StatusServiceUnavailable,
					Code:         "codearts_queued",
					Message:      "CodeArts queued the request; try again in " + retryAfter + "s",
					Headers:      map[string]string{"Retry-After": retryAfter},
					RetryAfterMS: retryMs,
					Operation:    sdk.MethodProviderExecute,
					Phase:        "response",
					Complete:     true,
				},
				Rules: []sdk.RequestFailureRule{{
					ID:     "codearts-queued",
					Status: []int{http.StatusServiceUnavailable},
					Code:   "codearts_queued",
					Class:  "overload",
					Scope:  "account",
				}},
			}, nil), nil
		}

		var (
			errText string
			errCode string
		)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			peek, peekErr := PeekAnswer(resp.Body)
			if peekErr != nil {
				_ = resp.Body.Close()
				return nil, uncertain("CodeArts response interrupted: " + peekErr.Error())
			}
			if peek.Code == "" {
				return owner.deliverCodeArtsSuccess(ctx, stream, call.Scope.Operation, resp, peek, chat)
			}
			errText = string(peek.Head)
			errCode = peek.Code
			_ = resp.Body.Close()
		} else {
			rawErr, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			_ = resp.Body.Close()
			errText = string(rawErr)
			errCode = ErrorCodeOf(errText)
		}

		if IsBodyTooLarge(errText) {
			mb := float64(len(bodyBytes)) / (1024 * 1024)
			return emitJSONError(ctx, stream, call.Scope.Operation, http.StatusRequestEntityTooLarge, errCode,
				fmt.Sprintf("CodeArts 网关只接受 12 MB 以内的请求，这次请求有 %.1f MB（通常是对话里的图片）。请新开会话，或去掉较大的图片后再试", mb), nil)
		}
		if resp.StatusCode == http.StatusUnauthorized || IsStaleCredential(errText) {
			if !renewed {
				renewed = true
				next, rErr := owner.refreshCredential(ctx, stream, CodeArtsProvider, active)
				if rErr == nil {
					active = next
					if rawNext, mErr := json.Marshal(next); mErr == nil {
						var rep sdk.AccountReplaceResult
						_ = stream.HostCall(ctx, sdk.MethodAccountReplace, sdk.AccountReplaceRequest{Credentials: rawNext}, &rep)
					}
					continue
				}
				return nil, rErr
			}
			return nil, unauthenticated(firstOf(ErrorMessageOf(errText), "the sign-in expired; sign in again"))
		}
		if resp.StatusCode == http.StatusForbidden {
			return nil, unauthenticated(firstOf(ErrorMessageOf(errText), "CodeArts returned 403 Forbidden"))
		}
		if resp.StatusCode == http.StatusBadRequest && chat.Injected && strings.Contains(strings.ToLower(errText), "thinking") {
			chat.Injected = false
			delete(chat.Body, "thinking")
			bodyBytes, err = encodeJSONNoEscape(chat.Body)
			if err != nil {
				return nil, err
			}
			continue
		}
		noBenefitChannel := signedBenefit && IsNoBenefitChannelCode(firstOf(errCode, errText))
		noModelBenefit := signedBenefit && IsNoModelBenefitCode(firstOf(errCode, errText))
		if noBenefitChannel || noModelBenefit {
			if benefitOnly {
				st.mu.Lock()
				if st.claim == sentClaim {
					st.aliasRefusals[model] = true
				}
				st.mu.Unlock()
				return emitBenefitUnavailable(ctx, stream, call.Scope.Operation, errCode)
			}
			st.mu.Lock()
			claimedToday := st.claim != nil && st.claim.Day == day
			st.mu.Unlock()
			if !noBenefitChannel && !claimTried && !claimedToday {
				claimTried = true
				if owner.ensureBenefitClaimed(ctx, stream, active, st) {
					continue
				}
			}
			if noBenefitChannel || st.isRegularModel(model) {
				st.mu.Lock()
				if st.claim == sentClaim {
					st.noBenefit[model] = true
				}
				st.mu.Unlock()
				useBenefit = false
				continue
			}
		}
		if errCode == SessionLimitError || strings.Contains(errText, SessionLimitError) {
			return sdk.RefusedExecution(sdk.RequestRecoveryRequest{
				Step:     "codearts-chat",
				Evidence: "rejected-before-effect",
				Facts: sdk.RequestFailureFacts{
					Status:    http.StatusTooManyRequests,
					Code:      SessionLimitError,
					Message:   firstOf(ErrorMessageOf(errText), "并发会话数已达上限，请稍后再试"),
					Operation: sdk.MethodProviderExecute,
					Phase:     "response",
					Complete:  true,
				},
				Rules: []sdk.RequestFailureRule{{
					ID:     "codearts-session-limit",
					Status: []int{http.StatusTooManyRequests},
					Code:   SessionLimitError,
					Class:  "rate_limit",
					Scope:  "account",
				}},
			}, nil), nil
		}
		if IsNotMultimodal(firstOf(errCode, errText)) {
			return emitJSONError(ctx, stream, call.Scope.Operation, http.StatusBadRequest, errCode,
				firstOf(ErrorMessageOf(errText), "the model takes no images"), nil)
		}
		finalStatus := resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			finalStatus = EmbeddedErrorStatus(errCode)
		}
		if finalStatus == http.StatusTooManyRequests || IsRateLimitCode(errCode) {
			return sdk.RefusedExecution(sdk.RequestRecoveryRequest{
				Step:     "codearts-chat",
				Evidence: "rejected-before-effect",
				Facts: sdk.RequestFailureFacts{
					Status:    http.StatusTooManyRequests,
					Code:      firstOf(errCode, "rate_limited"),
					Message:   firstOf(ErrorMessageOf(errText), "CodeArts rate or quota limit reached"),
					Operation: sdk.MethodProviderExecute,
					Phase:     "response",
					Complete:  true,
				},
				Rules: []sdk.RequestFailureRule{{
					ID:     "codearts-rate-or-quota",
					Status: []int{http.StatusTooManyRequests},
					Class:  "quota",
					Scope:  "account",
				}},
			}, nil), nil
		}
		return emitJSONError(ctx, stream, call.Scope.Operation, finalStatus, errCode,
			firstOf(ErrorMessageOf(errText), "the gateway answered with an error"), nil)
	}
	return emitJSONError(ctx, stream, call.Scope.Operation, http.StatusBadGateway, "", "CodeArts retry budget exhausted", nil)
}

func (owner *module) deliverCodeArtsSuccess(ctx context.Context, stream *sdk.Stream, opID string, resp *http.Response, peek PeekResult, chat *PreparedChat) (sdk.ExecuteResult, error) {
	defer resp.Body.Close()
	if chat.Fold {
		var full bytes.Buffer
		full.Write(peek.Head)
		if !peek.EOF {
			if _, err := io.Copy(&full, io.LimitReader(resp.Body, sdk.MaxNativeBodyBytes)); err != nil {
				return sdk.ExecuteResult{}, uncertain("CodeArts stream ended while folding")
			}
		}
		events := ParseSSEEvents(full.String())
		completion := CompletionFromChunks(events, chat.Names)
		encoded, err := encodeJSONNoEscape(completion)
		if err != nil {
			return sdk.ExecuteResult{}, err
		}
		usage := extractChatUsage(encoded)
		hdr := http.Header{"Content-Type": {"application/json"}}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: http.StatusOK, Headers: hdr}); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if err := stream.Emit(ctx, "data", encoded); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if usage != nil {
			_ = stream.Emit(ctx, "usage", *usage)
		}
		return sdk.ExecuteResult{Status: http.StatusOK, Headers: hdr, Usage: usage, Outcome: "completed"}, nil
	}

	hdr := http.Header{"Content-Type": {firstOf(resp.Header.Get("Content-Type"), "text/event-stream; charset=utf-8")}}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: resp.StatusCode, Headers: hdr}); err != nil {
		return sdk.ExecuteResult{}, err
	}
	var reader io.Reader = bytes.NewReader(peek.Head)
	if !peek.EOF {
		reader = io.MultiReader(bytes.NewReader(peek.Head), resp.Body)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var usage *sdk.Usage
	for scanner.Scan() {
		line := scanner.Text()
		if len(chat.Names) > 0 {
			line = RestoreToolNamesInSSELine(line, chat.Names)
		}
		outLine := []byte(line + "\n")
		if err := stream.Emit(ctx, "data", outLine); err != nil {
			return sdk.ExecuteResult{}, err
		}
		if u := extractSSEUsage(line); u != nil {
			usage = u
			_ = stream.Emit(ctx, "usage", *usage)
		}
	}
	if err := scanner.Err(); err != nil {
		return sdk.ExecuteResult{}, uncertain("CodeArts SSE stream interrupted")
	}
	return sdk.ExecuteResult{Status: resp.StatusCode, Headers: hdr, Usage: usage, Outcome: "completed"}, nil
}

func (owner *module) executeOfficeAce(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.ExecuteRequest, cred CodeArtsCredential) (any, error) {
	if cred.ModelAppKey == "" || cred.ModelAppSecret == "" {
		return nil, unauthenticated("OfficeAce MaaS appKey is missing; reauthorize or import saved credentials")
	}
	bodyBytes := officeAceStripPrefix(request.Body)
	doOnce := func(c CodeArtsCredential) (*http.Response, error) {
		target := owner.officeAceMaaSBase(c) + OAChatPath
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", c.BasicAuth())
		return owner.doHTTP(ctx, stream, req)
	}

	resp, err := doOnce(cred)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, uncertain("OfficeAce request failed: " + err.Error())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawErr, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		_ = resp.Body.Close()
		errText := string(rawErr)
		status := resp.StatusCode
		if status == http.StatusUnauthorized || IsOABasicRejection(status, errText) {
			before := cred.BasicFingerprint()
			if next, rErr := owner.refreshCredential(ctx, stream, OfficeAceProvider, cred); rErr == nil && next.BasicFingerprint() != before {
				if rawNext, mErr := json.Marshal(next); mErr == nil {
					var rep sdk.AccountReplaceResult
					_ = stream.HostCall(ctx, sdk.MethodAccountReplace, sdk.AccountReplaceRequest{Credentials: rawNext}, &rep)
				}
				resp2, err2 := doOnce(next)
				if err2 == nil {
					if resp2.StatusCode >= 200 && resp2.StatusCode < 300 {
						resp = resp2
						goto deliverSuccess
					}
					rawErr2, _ := io.ReadAll(io.LimitReader(resp2.Body, 512*1024))
					_ = resp2.Body.Close()
					status = resp2.StatusCode
					errText = string(rawErr2)
				}
			}
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				return nil, unauthenticated("OfficeAce（华为 MaaS）凭据被拒，请重新登录")
			}
		}
		if status == http.StatusTooManyRequests {
			return sdk.RefusedExecution(sdk.RequestRecoveryRequest{
				Step:     "officeace-chat",
				Evidence: "rejected-before-effect",
				Facts: sdk.RequestFailureFacts{
					Status:    http.StatusTooManyRequests,
					Code:      "officeace_rate_limited",
					Message:   "OfficeAce 被限流，稍后重试或换账号",
					Operation: sdk.MethodProviderExecute,
					Phase:     "response",
					Complete:  true,
				},
				Rules: []sdk.RequestFailureRule{{
					ID:     "officeace-rate-limit",
					Status: []int{http.StatusTooManyRequests},
					Class:  "rate_limit",
					Scope:  "account",
				}},
			}, nil), nil
		}
		if len(errText) > 200 {
			errText = errText[:200]
		}
		return emitJSONError(ctx, stream, call.Scope.Operation, status, "", fmt.Sprintf("OfficeAce（华为 MaaS）返回 HTTP %d：%s", status, errText), nil)
	}

deliverSuccess:
	defer resp.Body.Close()
	outHdr := http.Header{"Content-Type": {firstOf(resp.Header.Get("Content-Type"), "application/json")}}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: resp.StatusCode, Headers: outHdr}); err != nil {
		return nil, err
	}
	isSSE := strings.Contains(strings.ToLower(outHdr.Get("Content-Type")), "text/event-stream")
	var usage *sdk.Usage
	var jsonBuf bytes.Buffer
	var sseBuf bytes.Buffer
	buf := make([]byte, 32*1024)
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if err := stream.Emit(ctx, "data", chunk); err != nil {
				return nil, err
			}
			if isSSE {
				sseBuf.Write(chunk)
				for {
					data := sseBuf.Bytes()
					idx := bytes.IndexByte(data, '\n')
					if idx < 0 {
						break
					}
					line := string(data[:idx])
					sseBuf.Next(idx + 1)
					if u := extractSSEUsage(line); u != nil {
						usage = u
						_ = stream.Emit(ctx, "usage", *usage)
					}
				}
			} else if jsonBuf.Len()+n <= 4*1024*1024 {
				jsonBuf.Write(chunk)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return nil, uncertain("OfficeAce stream ended unexpectedly")
		}
	}
	if !isSSE && jsonBuf.Len() > 0 {
		if u := extractChatUsage(jsonBuf.Bytes()); u != nil {
			usage = u
			_ = stream.Emit(ctx, "usage", *usage)
		}
	}
	return sdk.ExecuteResult{Status: resp.StatusCode, Headers: outHdr, Usage: usage, Outcome: "completed"}, nil
}
