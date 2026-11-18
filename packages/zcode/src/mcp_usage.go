package zcode

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type mcpObservation struct {
	Informational  bool            `json:"informational"`
	ProviderFamily string          `json:"providerFamily"`
	TargetType     string          `json:"targetType"`
	Window         sdk.UsageWindow `json:"window"`
	ServerTime     int64           `json:"serverTime"`
}

func (owner *module) officialMCPHeaders(account Credentials) (http.Header, error) {
	deadline := owner.options.Now().Add(30 * time.Second).UnixMilli()
	if account.Kind != "individual" && account.Kind != "team" ||
		account.SessionToken == "" || account.SessionExpiresAt <= deadline ||
		account.BusinessToken == "" || account.BusinessExpiresAt <= deadline {
		return nil, &sdk.Error{Code: "credentials_required", Message: "Official MCP calls require the selected paid account's current session and business/MaaS JWT; no free fallback or token exchange was performed", Outcome: "not-started"}
	}
	headers := http.Header{
		"Authorization":            {"Bearer " + account.SessionToken},
		"X-Bigmodel-Authorization": {"Bearer " + account.BusinessToken},
		"Bigmodel-Target-Type":     {"PERSONAL"},
	}
	if account.Kind == "team" {
		headers.Set("Bigmodel-Target-Type", "TEAM")
		headers.Set("Bigmodel-Organization", account.Organization)
		headers.Set("Bigmodel-Project", account.Project)
	}
	return headers, nil
}

func mcpMillis(value any) (int64, bool) {
	seconds, valid := finiteNumber(value)
	if !valid || seconds < 0 || seconds*1000 >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(seconds * 1000), true
}

func projectMCPUsage(body []byte, observedAt int64) (sdk.UsageWindow, int64, error) {
	root, err := protocol.ReadObject(body)
	code, valid := finiteNumber(root["code"])
	data := mapObject(root["data"])
	if err != nil || root["success"] == false || !valid || code != 0 || data == nil {
		return sdk.UsageWindow{}, 0, quotaFailure("Official MCP usage did not confirm its envelope")
	}
	serverTime, valid := mcpMillis(data["server_time"])
	total := mapObject(data["total_usage"])
	used, usedValid := finiteNumber(total["used"])
	limit, limitValid := finiteNumber(total["limit"])
	remaining, remainingValid := finiteNumber(total["remaining"])
	if !valid || !usedValid || !limitValid || !remainingValid ||
		used < 0 || limit < 0 || remaining < 0 || remaining > limit {
		return sdk.UsageWindow{}, 0, quotaFailure("Official MCP usage aggregate is missing or invalid")
	}
	window := sdk.UsageWindow{Name: "Official MCP", Limit: 100, Aside: true, AsOf: observedAt}
	if limit == 0 {
		window.Unknown, window.Error = true, "Official MCP has no authoritative positive quota capacity"
	} else {
		window.Used = min(100, max(0, 100*(limit-remaining)/limit))
	}
	if next, present := data["next_refresh_at"]; present {
		value, valid := mcpMillis(next)
		if !valid {
			return sdk.UsageWindow{}, 0, quotaFailure("Official MCP refresh timestamp is invalid")
		}
		window.ResetsAt = value
	}
	return window, serverTime, nil
}

func (owner *module) officialMCPUsage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	headers, err := owner.officialMCPHeaders(account)
	if err != nil {
		return nil, err
	}
	if _, _, err := owner.principal(ctx, stream, account, ""); err != nil {
		return nil, err
	}
	response, err := owner.vendorRequest(ctx, stream, http.MethodGet,
		owner.zcodeOrigin()+"/api/v1/mcp/usage", headers, nil)
	if err != nil || response.Status < 200 || response.Status >= 300 {
		return nil, quotaFailure("Official MCP usage endpoint could not supply an observation")
	}
	body, err := json.Marshal(response.Data)
	if err != nil {
		return nil, err
	}
	window, serverTime, err := projectMCPUsage(body, owner.options.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	target := "PERSONAL"
	if account.Kind == "team" {
		target = "TEAM"
	}
	return mcpObservation{Informational: true, ProviderFamily: account.Site,
		TargetType: target, Window: window, ServerTime: serverTime}, nil
}
