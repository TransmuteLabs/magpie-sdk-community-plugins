package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type meterError struct {
	Message      string
	Status       int
	Code         int
	InvalidToken bool
	Unreadable   bool
	HasResponse  bool
}

func (err *meterError) Error() string { return err.Message }

func (owner *module) meterAsked(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, path string, body any) (*http.Response, error) {
	if body == nil {
		body = map[string]any{}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointOf(site, acct)+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+acct.Access)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WorkBuddy/"+site.App)
	req.Header.Set("X-Domain", domainOf(site, acct))
	req.Header.Set("X-Product", "SaaS")
	req.Header.Set("X-IDE-Type", "WorkBuddy")
	req.Header.Set("X-IDE-Name", "WorkBuddy")
	req.Header.Set("X-IDE-Version", site.App)
	if acct.UID != "" {
		req.Header.Set("X-User-Id", acct.UID)
	}
	for key, val := range enterpriseHeaders(acct) {
		req.Header.Set(key, val)
	}
	resp, _, err := owner.doHTTP(ctx, stream, req)
	return resp, err
}

func (owner *module) meter(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, path string, body any) (map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := owner.meterAsked(reqCtx, stream, site, acct, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	okStatus := resp.StatusCode >= 200 && resp.StatusCode < 300
	var env map[string]any
	parsed := false
	if json.Unmarshal(raw, &env) == nil {
		parsed = true
	}
	if okStatus && !parsed {
		unreadable := strings.TrimSpace(string(raw))
		if len(unreadable) > 120 {
			unreadable = unreadable[:120]
		}
		if unreadable == "" {
			unreadable = "(empty)"
		}
		return nil, &meterError{
			Message:     "unreadable reply " + unreadable,
			Status:      resp.StatusCode,
			Unreadable:  true,
			HasResponse: true,
		}
	}
	code := parseIntCode(env["code"])
	baseErr := meterError{
		Status:      resp.StatusCode,
		Code:        code,
		HasResponse: true,
	}
	if !okStatus {
		baseErr.InvalidToken = isInvalidToken(resp)
		if code != 0 {
			baseErr.Message = firstNonEmpty(strField(env, "msg"), fmt.Sprintf("error %d", code))
			return nil, &baseErr
		}
		if msg := strField(env, "msg"); msg != "" {
			baseErr.Message = msg
			return nil, &baseErr
		}
		baseErr.Message = statusText(resp.StatusCode)
		return nil, &baseErr
	}
	if code != 0 {
		baseErr.Message = firstNonEmpty(strField(env, "msg"), fmt.Sprintf("error %d", code))
		return nil, &baseErr
	}
	if env == nil {
		return nil, nil
	}
	data, _ := env["data"].(map[string]any)
	return data, nil
}

func (owner *module) checkinOf(ctx context.Context, stream *sdk.Stream, site Site, acct Credential) (sdk.AccountCheckinResult, error) {
	status, err := owner.meter(ctx, stream, site, acct, "/v2/billing/meter/checkin-activity-status", map[string]any{})
	if err != nil {
		var mErr *meterError
		if errors.As(err, &mErr) {
			if mErr.Code == 1001 {
				return sdk.AccountCheckinResult{Outcome: "done"}, nil
			}
			if mErr.HasResponse {
				return sdk.AccountCheckinResult{Outcome: "failed", Message: mErr.Message}, nil
			}
		}
		return sdk.AccountCheckinResult{}, err
	}
	if !truthy(status["active"]) {
		return sdk.AccountCheckinResult{Outcome: "inactive"}, nil
	}
	if truthy(firstPresent(status, "today_checked_in", "todayCheckedIn")) {
		res := sdk.AccountCheckinResult{Outcome: "done"}
		if val, ok := asFloat(firstPresent(status, "today_credit", "daily_credit")); ok && !math.IsNaN(val) && !math.IsInf(val, 0) && val > 0 {
			credit := val
			res.Credit = &credit
		}
		return res, nil
	}

	waits := owner.options.CheckinWaits
	for attempt := 0; ; attempt++ {
		got, claimErr := owner.meter(ctx, stream, site, acct, "/v2/billing/meter/daily-checkin", map[string]any{})
		if claimErr == nil {
			res := sdk.AccountCheckinResult{Outcome: "claimed"}
			if val, ok := asFloat(got["credit"]); ok && !math.IsNaN(val) && !math.IsInf(val, 0) && val > 0 {
				credit := val
				res.Credit = &credit
			}
			if val, ok := asInt64(got["streak_days"]); ok && val > 0 {
				streak := int(val)
				res.Streak = &streak
			}
			if msg := strField(got, "message"); msg != "" {
				res.Message = msg
			}
			return res, nil
		}
		var mErr *meterError
		if !errors.As(claimErr, &mErr) {
			return sdk.AccountCheckinResult{}, claimErr
		}
		if mErr.Code == 1001 {
			return sdk.AccountCheckinResult{Outcome: "done"}, nil
		}
		if mErr.Code == 1002 {
			return sdk.AccountCheckinResult{Outcome: "ineligible", Message: mErr.Message}, nil
		}
		if mErr.Code == 1003 {
			return sdk.AccountCheckinResult{Outcome: "inactive", Message: mErr.Message}, nil
		}
		if mErr.Status == http.StatusUnauthorized {
			if _, probeErr := owner.meter(ctx, stream, site, acct, "/billing/meter/get-user-resource-summary", map[string]any{}); probeErr != nil {
				return sdk.AccountCheckinResult{}, claimErr
			}
			return sdk.AccountCheckinResult{
				Outcome: "ineligible",
				Message: "WorkBuddy's check-in takes WorkBuddy's own app",
			}, nil
		}
		if mErr.Unreadable {
			return sdk.AccountCheckinResult{Outcome: "failed", Message: "check-in: " + mErr.Message}, nil
		}
		transient := mErr.Status >= 500 || mErr.Code == 10000
		if transient && attempt < len(waits) {
			if err := sleepCtx(ctx, waits[attempt]); err != nil {
				return sdk.AccountCheckinResult{}, err
			}
			continue
		}
		if mErr.HasResponse {
			return sdk.AccountCheckinResult{Outcome: "failed", Message: mErr.Message}, nil
		}
		return sdk.AccountCheckinResult{}, claimErr
	}
}

func (owner *module) checkin(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.AccountCheckinRequest
	if nonempty(call.Params) {
		if decodeObject(call.Params, &req) != nil {
			return nil, invalid("Invalid account check-in request")
		}
		if req.Day != "" {
			if err := req.Validate(); err != nil {
				return nil, invalid(err.Error())
			}
		}
	}
	provider := firstNonEmpty(call.Scope.Provider, ProviderWorkBuddy)
	site, ok := owner.siteFor(provider)
	if !ok {
		return nil, invalid("Unknown WorkBuddy provider")
	}
	if site.AI {
		return sdk.AccountCheckinResult{
			Outcome: "ineligible",
			Message: "WorkBuddy AI has no daily check-in",
		}, nil
	}
	var stored Credential
	if decodeObject(call.Auth, &stored) != nil {
		return nil, unauthenticated("Selected WorkBuddy credentials are invalid")
	}
	if stored.isAPIKey() {
		return sdk.AccountCheckinResult{
			Outcome: "ineligible",
			Message: "WorkBuddy's check-in requires a WorkBuddy account sign-in",
		}, nil
	}
	if !stored.isOAuth() {
		return nil, unauthenticated("WorkBuddy isn't signed in")
	}
	active, err := owner.current(ctx, stream, site, stored)
	if err != nil {
		return nil, err
	}
	return owner.checkinOf(ctx, stream, site, active)
}
