package zcode

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func finiteNumber(value any) (float64, bool) {
	number, valid := value.(json.Number)
	if !valid {
		return 0, false
	}
	parsed, err := number.Float64()
	return parsed, err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
}

func quotaFailure(message string) error {
	return &sdk.Error{Code: "upstream", Message: message}
}

func projectQuota(body []byte, observedAt int64) (sdk.UsageResult, error) {
	root, err := protocol.ReadObject(body)
	if err != nil || root["success"] == false {
		return sdk.UsageResult{}, quotaFailure("ZCode quota response is invalid")
	}
	code, hasCode := finiteNumber(root["code"])
	if !hasCode || code != 0 && code != 200 {
		return sdk.UsageResult{}, quotaFailure("ZCode quota response did not confirm success")
	}
	data, _ := root["data"].(map[string]any)
	limits, valid := data["limits"].([]any)
	if !valid || len(limits) > 128 {
		return sdk.UsageResult{}, quotaFailure("ZCode quota limits are unavailable")
	}
	result := sdk.UsageResult{AsOf: observedAt, Windows: []sdk.UsageWindow{}}
	names := map[string]bool{}
	for _, entry := range limits {
		limit, valid := entry.(map[string]any)
		if !valid {
			return sdk.UsageResult{}, quotaFailure("ZCode quota limit is invalid")
		}
		kind, _ := limit["type"].(string)
		if kind != "TOKENS_LIMIT" && kind != "CREDIT_LIMIT" && kind != "TIME_LIMIT" {
			continue
		}
		unit, unitValid := finiteNumber(limit["unit"])
		count, countValid := finiteNumber(limit["number"])
		name := ""
		if kind == "TIME_LIMIT" {
			name = "MCP"
		} else if unitValid && unit == 3 && (countValid && count == 5 || !countValid || count <= 0) {
			name = "5 hours"
		} else if unitValid && unit == 6 {
			name = "Weekly"
		}
		if name == "" {
			continue
		}
		if names[name] {
			return sdk.UsageResult{}, quotaFailure("ZCode quota windows are ambiguous")
		}
		names[name] = true
		window := sdk.UsageWindow{Name: name, AsOf: observedAt, Aside: kind == "TIME_LIMIT", Span: quotaSpan(unit, count)}
		used, known := finiteNumber(limit["percentage"])
		total, hasTotal := finiteNumber(limit["usage"])
		if hasTotal && total < 0 {
			return sdk.UsageResult{}, quotaFailure("ZCode quota total is invalid")
		}
		if hasTotal {
			window.Limit = total
		}
		if hasTotal && total == 0 {
			window.Unknown, window.Error = true, "Uncapped informational window is not an authoritative routing allowance"
			if !window.Aside {
				result.Error = "ZCode quota contains an informational window"
			}
		} else {
			if hasTotal && total > 0 {
				if remaining, hasRemaining := finiteNumber(limit["remaining"]); hasRemaining {
					if remaining < 0 || remaining > total {
						return sdk.UsageResult{}, quotaFailure("ZCode quota remaining value is invalid")
					}
					used, known = 100*(total-remaining)/total, true
					quotaCounts(&window, total-remaining, total)
				} else if current, hasCurrent := finiteNumber(limit["currentValue"]); hasCurrent {
					if current < 0 {
						return sdk.UsageResult{}, quotaFailure("ZCode quota used count is invalid")
					}
					quotaCounts(&window, current, total)
					if !known {
						used, known = 100*current/total, true
					}
				}
			}
			if !known || used < 0 || math.IsInf(used, 0) || math.IsNaN(used) {
				window.Unknown, window.Error = true, "ZCode did not report a valid used value"
				if !window.Aside {
					result.Error = "One or more ZCode quota windows are unavailable"
				}
			} else {
				window.Used = min(used, 100)
			}
		}
		if reset, valid := finiteNumber(limit["nextResetTime"]); valid && reset > 0 && reset < float64(math.MaxInt64) {
			window.ResetsAt = int64(reset)
		}
		result.Windows = append(result.Windows, window)
	}
	if !names["5 hours"] && !names["Weekly"] {
		return sdk.UsageResult{}, unsupported("ZCode did not supply an authoritative ordinary five-hour or weekly quota")
	}
	return result, nil
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	account, err := owner.selected(call)
	if err != nil {
		return nil, err
	}
	var request sdk.UsageRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if len(request.Credentials) != 0 {
		return nil, invalid("Usage credentials must come from the host account scope")
	}
	if request.Revision != "" {
		if _, err := sdk.ParseUsageRevision(request.Revision); err != nil {
			return nil, invalid("Usage revision must be a canonical unsigned integer string")
		}
	}
	read := func() (any, error) {
		return owner.readUsage(ctx, call, stream, account, request.Revision)
	}
	state, stateErr := currentInstance(ctx)
	if stateErr != nil {
		return nil, stateErr
	}
	if state.dataDir == "" {
		return read()
	}
	_, store, principal, _, err := owner.executionContext(ctx, call, stream)
	if err != nil {
		return nil, err
	}
	var result any
	err = store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		for _, claim := range transaction.Journal.Claims {
			if claim.State == "pending" || claim.State == "unknown" {
				result = sdk.UsageResult{Revision: request.Revision, ReadDeferred: true}
				return nil
			}
		}
		if transaction.Journal.Reset[call.Scope.Account].Pending != nil {
			result = sdk.UsageResult{Revision: request.Revision, ReadDeferred: true}
			return nil
		}
		var readErr error
		result, readErr = read()
		return readErr
	})
	return result, err
}

func (owner *module) readUsage(ctx context.Context, call sdk.Call, stream *sdk.Stream, account Credentials, revision string) (any, error) {
	if account.Kind == "start" {
		available, err := owner.startEntitlement(ctx, stream, account, true)
		if err != nil {
			return nil, err
		}
		if len(available.Windows) == 0 && len(available.Upcoming) == 0 {
			return nil, unsupported("Start balance supplied no authoritative model-specific quota windows")
		}
		for index := range available.Windows {
			window := &available.Windows[index]
			if !window.Aside && owner.giftRested(ctx, account, window.Models[0], false) {
				window.Used = 100
			}
		}
		return sdk.UsageResult{Revision: revision, Windows: append(available.Windows, available.Upcoming...)}, nil
	}
	report, err := owner.readPaidUsage(ctx, stream, account, revision)
	if err != nil {
		return nil, err
	}
	if giftSession(account, owner.options.Now()) {
		if gift, err := owner.startEntitlement(ctx, stream, account, true); err == nil {
			state, err := currentInstance(ctx)
			if err != nil {
				return nil, err
			}
			held, holdErr := owner.giftRouteHeld(ctx, stream)
			freeFirst := state.freeFirst && !held && holdErr == nil
			giftRest := make(map[string]bool, len(gift.Declared))
			for name, model := range gift.Declared {
				giftRest[name] = owner.giftRested(ctx, account, model, false)
			}
			for index := range report.Windows {
				window := &report.Windows[index]
				if window.Aside {
					continue
				}
				for name, model := range gift.Declared {
					window.NotModels = append(window.NotModels, model+"-Trial")
					if freeFirst && gift.Models[name] && !giftRest[name] {
						window.NotModels = append(window.NotModels, model)
					}
				}
			}
			for _, window := range gift.Windows {
				if window.Aside {
					models := make([]string, 0, len(window.Models))
					for _, name := range window.Models {
						model := gift.Declared[name]
						models = append(models, model+"-Trial")
						if freeFirst && gift.Models[name] && !giftRest[name] {
							models = append(models, model)
						}
					}
					window.Models = models
					report.Windows = append(report.Windows, window)
					continue
				}
				plain := window.Models[0]
				window.Models = []string{gift.Declared[plain] + "-Trial"}
				window.Name = gift.Declared[plain] + "-Trial · " + window.Name
				if giftRest[plain] {
					window.Used = 100
				}
				if freeFirst && gift.Models[plain] && !giftRest[plain] {
					window.Models = append(window.Models, gift.Declared[plain])
				}
				report.Windows = append(report.Windows, window)
			}
			report.Windows = append(report.Windows, gift.Upcoming...)
		}
	}
	return report, nil
}

func (owner *module) readPaidUsage(ctx context.Context, stream *sdk.Stream, account Credentials, revision string) (sdk.UsageResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	address, _ := url.Parse(owner.options.Endpoints.quotaForAccount(account))
	if account.Kind == "team" || account.Organization != "" {
		query := address.Query()
		query.Set("type", "2")
		address.RawQuery = query.Encode()
	}
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: address.String()}, &proxy); err != nil {
		return sdk.UsageResult{}, err
	}
	outbound, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return sdk.UsageResult{}, invalid("Invalid ZCode quota endpoint")
	}
	outbound.Header.Set("Authorization", account.token())
	if account.Organization != "" {
		outbound.Header.Set("Bigmodel-Organization", account.Organization)
		outbound.Header.Set("Bigmodel-Project", account.Project)
	}
	response, err := send(ctx, owner.options.HTTPFactory, proxy.URL, outbound)
	if err != nil {
		if ctx.Err() != nil {
			return sdk.UsageResult{}, ctx.Err()
		}
		return sdk.UsageResult{}, &sdk.Error{Code: "upstream", Message: "ZCode quota transport failed"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.UsageResult{}, &sdk.Error{Code: "upstream", Message: "ZCode quota endpoint refused the read"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		if ctx.Err() != nil {
			return sdk.UsageResult{}, ctx.Err()
		}
		return sdk.UsageResult{}, &sdk.Error{Code: "upstream", Message: "ZCode quota response is incomplete or too large"}
	}
	report, err := projectQuota(body, owner.options.Now().UnixMilli())
	if err != nil {
		return sdk.UsageResult{}, err
	}
	report.Revision, report.AsOf = revision, 0
	for index := range report.Windows {
		window := &report.Windows[index]
		if !owner.richQuota {
			window.Amount, window.Balance = nil, nil
			window.Display, window.Unit, window.Span, window.Limit = "", "", 0, 100
		}
		if window.Aside {
			continue
		}
		if !window.Unknown {
			window.AsOf = 0
		}
		for _, alias := range aliasModels() {
			window.NotModels = append(window.NotModels, alias.ID)
		}
	}
	return report, nil
}

func invalidateUsage(ctx context.Context, stream *sdk.Stream) error {
	var result sdk.UsageInvalidateResult
	if err := stream.HostCall(ctx, sdk.MethodUsageInvalidate, sdk.UsageInvalidateRequest{}, &result); err != nil {
		return uncertain("Quota invalidation was not confirmed; the vendor mutation was not dispatched")
	}
	if _, err := sdk.ParseUsageRevision(result.Revision); err != nil {
		return uncertain("Quota invalidation returned an invalid revision; the vendor mutation was not dispatched")
	}
	return nil
}
