package zcode

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
	codec "github.com/yetone/magpie/pkg/protocolext"
)

type giftHint struct {
	until time.Time
	names []string
}

func (owner *module) missingTrial(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials) error {
	message := "ZCode has no confirmed gift quota for this Trial model"
	state, err := currentInstance(ctx)
	if err != nil || !giftSession(account, owner.options.Now()) {
		return unsupported(message)
	}
	key := controller.Principal(owner.zcodeOrigin(), strings.Join([]string{
		account.UserID, account.Device, account.SessionToken, call.Scope.Account, strconv.FormatUint(call.Scope.Generation, 10),
	}, "\x00"))
	now := owner.options.Now()
	state.mu.Lock()
	cached, found := state.giftHints[key]
	names := append([]string(nil), cached.names...)
	state.mu.Unlock()
	if !found || !now.Before(cached.until) {
		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		offers, _, readErr := owner.readOffers(readCtx, stream, account, 0)
		cancel()
		names = nil
		if readErr == nil {
			seen := map[string]bool{}
			for _, offer := range offers {
				if !seen[offer.Name] {
					seen[offer.Name] = true
					names = append(names, offer.Name)
				}
				if len(names) == 4 {
					break
				}
			}
			state.mu.Lock()
			for id, hint := range state.giftHints {
				if !now.Before(hint.until) {
					delete(state.giftHints, id)
				}
			}
			if len(state.giftHints) < 4096 {
				state.giftHints[key] = giftHint{until: now.Add(10 * time.Minute), names: append([]string(nil), names...)}
			}
			state.mu.Unlock()
		}
	}
	if len(names) > 0 {
		message += ". ZCode holds " + strings.Join(names, ", ") +
			" for this account unclaimed: claim it in the ZCode app (its claim needs the app's security check)"
	}
	return &sdk.Error{Code: "invalid_request_error", Message: message, Outcome: "not-started"}
}

func wireGiftModel(name string) string {
	for _, model := range Models() {
		if strings.EqualFold(name, model.ID) {
			return model.ID
		}
	}
	return name
}

func giftSession(account Credentials, now time.Time) bool {
	expiry := account.SessionExpiresAt
	if account.Kind == "start" {
		expiry = account.ExpiresAt
	}
	return account.SessionToken != "" && expiry > now.Add(30*time.Second).UnixMilli()
}

func (owner *module) giftRested(ctx context.Context, account Credentials, model string, note bool) bool {
	state, err := currentInstance(ctx)
	if err != nil {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	now := owner.options.Now()
	for key, until := range state.giftRest {
		if !now.Before(until) {
			delete(state.giftRest, key)
		}
	}
	key := controller.Principal(owner.zcodeOrigin(), account.UserID) + "/" + strings.ToLower(model)
	if note && len(state.giftRest) < 4096 {
		state.giftRest[key] = now.Add(time.Minute)
	}
	return now.Before(state.giftRest[key])
}

func appendGiftModels(result, catalog []sdk.Model, gift startAvailability, aliases bool) []sdk.Model {
	seen := map[string]bool{}
	for _, model := range result {
		seen[strings.ToLower(model.ID)] = true
	}
	names := make([]string, 0, len(gift.Declared))
	for name := range gift.Declared {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		id := gift.Declared[name]
		model := sdk.Model{ID: id, Name: id, Free: true, Protocols: []codec.Protocol{codec.Messages}}
		for _, candidate := range append(Models(), catalog...) {
			if strings.EqualFold(candidate.ID, id) {
				model = candidate
			}
		}
		model.Free = true
		if aliases {
			model.ID, model.Name = id+"-Trial", id+"-Trial"
		}
		key := strings.ToLower(model.ID)
		if !seen[key] {
			seen[key] = true
			result = append(result, model)
		}
	}
	return result
}

func (owner *module) giftRoute(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials, model string, trial, giftOnly bool) (string, Credentials, bool, error) {
	state, err := currentInstance(ctx)
	if err != nil {
		return "", account, false, err
	}
	if !trial && !giftOnly && account.Kind != "start" && !state.freeFirst {
		return model, account, false, nil
	}
	if !giftSession(account, owner.options.Now()) {
		if trial || giftOnly || account.Kind == "start" {
			return "", account, false, unsupported("Trial models require the selected account's unexpired ZCode session")
		}
		return model, account, false, nil
	}
	gift, err := owner.startEntitlement(ctx, stream, account, false)
	if err != nil {
		if trial {
			return "", account, false, owner.missingTrial(ctx, call, stream, account)
		}
		if giftOnly || account.Kind == "start" {
			return "", account, false, err
		}
		return model, account, false, nil
	}
	name := strings.ToLower(model)
	if trial {
		name = strings.TrimSuffix(name, "-trial")
	}
	raw, declared := gift.Declared[name]
	if !declared || !gift.Models[name] || owner.giftRested(ctx, account, raw, false) {
		if trial && !declared {
			return "", account, false, owner.missingTrial(ctx, call, stream, account)
		}
		if trial || giftOnly || account.Kind == "start" {
			return "", account, false, &sdk.Error{Code: "rate_limit_error",
				Message: "The selected Trial model has no confirmed spendable gift quota", Outcome: "not-started"}
		}
		return model, account, false, nil
	}
	if !trial && !giftOnly && account.Kind != "start" {
		held, err := owner.giftStartBlocked(ctx, stream, &account)
		if err != nil {
			return "", account, false, err
		}
		if held {
			return model, account, false, nil
		}
	}
	dispatch := Credentials{Site: account.Site, Kind: "start", UserID: account.UserID,
		Device: account.Device, SessionToken: account.SessionToken}
	return raw, dispatch, true, nil
}

func (owner *module) giftRouteHeld(ctx context.Context, stream *sdk.Stream) (bool, error) {
	return owner.giftStartBlocked(ctx, stream, nil)
}

func (owner *module) giftStartBlocked(ctx context.Context, stream *sdk.Stream, account *Credentials) (bool, error) {
	state, err := currentInstance(ctx)
	if err != nil {
		return false, err
	}
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: owner.options.Endpoints.Start}, &proxy); err != nil {
		return false, err
	}
	state.startGuard.mu.Lock()
	defer state.startGuard.mu.Unlock()
	route := controller.Principal(owner.zcodeOrigin(), proxy.URL)
	return state.startGuard.held(route) ||
		account != nil && (state.startGuard.full(controller.Principal(owner.zcodeOrigin(), account.UserID)) ||
			state.startGuard.full(startAddressKey(route))), nil
}

func refusalCode(body []byte) (float64, bool) {
	root, err := protocol.ReadObject(body)
	if err != nil || root["type"] != nil && root["type"] != "error" {
		return 0, false
	}
	for _, value := range []map[string]any{root, mapObject(root["data"])} {
		for _, name := range []string{"content", "usage", "output", "choices", "completion", "stop_reason"} {
			if _, exists := value[name]; exists {
				return 0, false
			}
		}
	}
	code, valid := claimCode(root["code"])
	if !valid {
		code, valid = claimCode(mapObject(root["error"])["code"])
	}
	return code, valid
}

func giftSpentReply(status int, body []byte) bool {
	code, valid := refusalCode(body)
	return (status >= 200 && status < 300 || status == 429) && valid && (code == 1005 || code == 1113)
}

func (owner *module) paidFallbackUsage(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials) (sdk.UsageResult, error) {
	state, err := currentInstance(ctx)
	if err != nil {
		return sdk.UsageResult{}, err
	}
	if state.dataDir == "" {
		return owner.readPaidUsage(ctx, stream, account, "")
	}
	store, err := state.controller()
	if err != nil {
		return sdk.UsageResult{}, err
	}
	var report sdk.UsageResult
	err = store.Locked(ctx, controller.Principal(owner.zcodeOrigin(), account.UserID), func(transaction *controller.Transaction) error {
		for _, claim := range transaction.Journal.Claims {
			if claim.State == "pending" || claim.State == "unknown" {
				return unsupported("Unsettled billing prevents confirmation of paid fallback capacity")
			}
		}
		if transaction.Journal.Reset[call.Scope.Account].Pending != nil {
			return unsupported("Unsettled reset prevents confirmation of paid fallback capacity")
		}
		var readErr error
		report, readErr = owner.readPaidUsage(ctx, stream, account, "")
		return readErr
	})
	return report, err
}

func (owner *module) giftFallback(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials, model string, allowPaid bool, source http.Header, plainBody []byte, outbound *http.Request, response *http.Response) (*http.Response, bool, error) {
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media == "text/event-stream" || media != "application/json" {
		return response, false, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, protocol.MaxBody+1))
	response.Body.Close()
	if err != nil || len(body) > protocol.MaxBody {
		return nil, false, uncertain("Gift response is incomplete; no paid replay was authorized")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	if !giftSpentReply(response.StatusCode, body) {
		return response, false, nil
	}
	owner.giftRested(ctx, account, model, true)
	if !allowPaid {
		return response, false, nil
	}
	report, err := owner.paidFallbackUsage(ctx, call, stream, account)
	if err != nil || report.Error != "" {
		return response, false, nil
	}
	confirmed := false
	for _, window := range report.Windows {
		if window.Aside {
			continue
		}
		// Used is the authoritative percentage; Limit retains raw capacity in
		// the full module and can be absent for percentage-only observations.
		if window.Unknown || window.Error != "" || window.Used >= 100 {
			return response, false, nil
		}
		confirmed = true
	}
	if !confirmed {
		return response, false, nil
	}
	endpoint := owner.options.Endpoints.forAccount(account)
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return nil, false, err
	}
	paid, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(plainBody))
	if err != nil {
		return nil, false, err
	}
	paid.Header = requestHeaders(source, account, outbound.Header.Get("X-Request-Id"))
	paid.Header.Set("X-Session-Id", outbound.Header.Get("X-Session-Id"))
	for _, header := range []string{"X-Query-Id", "X-ZCode-Trace-Id"} {
		if value := outbound.Header.Get(header); value != "" {
			paid.Header.Set(header, value)
		}
	}
	replay, err := owner.paidDispatch(ctx, stream, proxy.URL, paid)
	if err != nil {
		return nil, false, uncertain("Paid dispatch after a definitive unused gift refusal is unconfirmed; no further replay was made")
	}
	return replay, true, nil
}
