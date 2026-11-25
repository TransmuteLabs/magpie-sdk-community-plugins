package codearts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

var (
	dailyCheckinTitlePat = regexp.MustCompile(`签到|每日`)
	claimedStatuses      = []string{"CLAIMED", "CONFIRMED", "CONSUMED"}
)

type unconfirmedClaim struct {
	CampaignID any            `json:"campaignId"`
	Benefit    map[string]any `json:"benefit,omitempty"`
}

type unconfirmedRecord struct {
	Day    string             `json:"day"`
	Claims []unconfirmedClaim `json:"claims"`
}

func sameCampaign(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func isClaimedToday(item map[string]any) bool {
	if item == nil {
		return false
	}
	return slices.Contains(claimedStatuses, strings.ToUpper(strField(item, "status")))
}

func (owner *module) loadUnconfirmed(ctx context.Context, stream *sdk.Stream, key, day string) ([]unconfirmedClaim, string) {
	if stream == nil {
		return nil, "0"
	}
	var val sdk.StateValue
	if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: key}, &val); err != nil || !val.Found {
		return nil, "0"
	}
	var rec unconfirmedRecord
	if json.Unmarshal(val.Value, &rec) != nil || rec.Day != day {
		return nil, val.Version
	}
	return rec.Claims, val.Version
}

func (owner *module) saveUnconfirmed(ctx context.Context, stream *sdk.Stream, key, version, day string, claims []unconfirmedClaim) {
	if stream == nil {
		return
	}
	raw, err := json.Marshal(unconfirmedRecord{Day: day, Claims: claims})
	if err != nil {
		return
	}
	var res sdk.StateCASResult
	_ = stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{
		Key:     key,
		Version: version,
		Value:   raw,
	}, &res)
}

func (owner *module) listDailyCampaigns(ctx context.Context, stream *sdk.Stream, cred CodeArtsCredential, station Station) ([]map[string]any, string) {
	urlStr := strings.TrimRight(station.Snap, "/") + "/v1/ops/delivery?channel=IDE"
	ok, _, data, msg := owner.snapRequest(ctx, stream, http.MethodGet, urlStr, cred, nil)
	if !ok {
		return nil, msg
	}
	var rawItems []any
	switch typed := data.(type) {
	case []any:
		rawItems = typed
	case map[string]any:
		for _, k := range []string{"items", "list", "deliveries", "campaigns"} {
			if arr, ok := typed[k].([]any); ok {
				rawItems = arr
				break
			}
		}
	}
	var out []map[string]any
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.ToUpper(strField(item, "campaignType", "triggerType", "type")) != "USER_LOGIN" {
			continue
		}
		title := firstOf(strField(item, "campaignName", "title", "name"))
		if !dailyCheckinTitlePat.MatchString(title) {
			continue
		}
		out = append(out, item)
	}
	return out, ""
}

func (owner *module) codeArtsCheckin(ctx context.Context, stream *sdk.Stream, opID string, cred CodeArtsCredential, accountKey string) sdk.AccountCheckinResult {
	station := owner.stationOf(&cred)
	items, listErr := owner.listDailyCampaigns(ctx, stream, cred, station)
	if listErr != "" {
		return sdk.AccountCheckinResult{Outcome: "failed", Message: listErr}
	}
	if len(items) == 0 {
		return sdk.AccountCheckinResult{Outcome: "inactive", Message: "当前没有每日领取活动"}
	}
	day := beijingDay(owner.options.Now().UnixMilli())
	stateKey := "codearts.unconfirmed." + accountKey
	left, stateVer := owner.loadUnconfirmed(ctx, stream, stateKey, day)
	var (
		kept       []unconfirmedClaim
		failedMsgs []string
		claimedSum float64
		claimedAny bool
		doneAny    bool
		ineligMsg  string
		accepted   bool
	)
	emitAccepted := func() {
		if !accepted && stream != nil {
			accepted = true
			_ = stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID})
		}
	}
	claimURL := strings.TrimRight(station.Snap, "/") + "/v1/ops/claim"
	confirmURL := strings.TrimRight(station.Snap, "/") + "/v1/ops/confirm"

	for _, item := range items {
		credit, _ := asFloat(item["benefitAmount"])
		campID := item["campaignId"]
		hasCamp := campID != nil && fmt.Sprint(campID) != ""
		claimable, _ := item["claimable"].(bool)
		var leftHere *unconfirmedClaim
		if hasCamp {
			for idx := range left {
				if sameCampaign(left[idx].CampaignID, campID) {
					leftHere = &left[idx]
					break
				}
			}
		}
		if !claimable && strings.EqualFold(strField(item, "status"), "CLAIMED") && hasCamp && leftHere != nil {
			emitAccepted()
			cOK, _, _, cMsg := owner.snapRequest(ctx, stream, http.MethodPost, confirmURL, cred, map[string]any{"campaignId": campID})
			if cOK {
				got := credit
				if bAmt, ok := asFloat(leftHere.Benefit["benefitAmount"]); ok && bAmt > 0 {
					got = bAmt
				}
				claimedSum += got
				claimedAny = true
				continue
			}
			again, _ := owner.listDailyCampaigns(ctx, stream, cred, station)
			var seen map[string]any
			for _, it := range again {
				if sameCampaign(it["campaignId"], campID) {
					seen = it
					break
				}
			}
			if isClaimedToday(seen) && !strings.EqualFold(strField(seen, "status"), "CLAIMED") {
				doneAny = true
				continue
			}
			kept = append(kept, *leftHere)
			failedMsgs = append(failedMsgs, "领取确认失败："+cMsg)
			continue
		}
		if isClaimedToday(item) {
			doneAny = true
			continue
		}
		if !claimable {
			if ineligMsg == "" {
				stName := firstOf(strField(item, "status"), "null")
				ineligMsg = fmt.Sprintf("当前不可领取（status=%s）", stName)
			}
			continue
		}
		if !hasCamp {
			failedMsgs = append(failedMsgs, "活动缺少 campaignId")
			continue
		}
		emitAccepted()
		ok, _, data, cMsg := owner.snapRequest(ctx, stream, http.MethodPost, claimURL, cred, map[string]any{
			"campaignId": campID,
			"channel":    "IDE",
		})
		if !ok {
			again, _ := owner.listDailyCampaigns(ctx, stream, cred, station)
			var seen map[string]any
			for _, it := range again {
				if sameCampaign(it["campaignId"], campID) {
					seen = it
					break
				}
			}
			if isClaimedToday(seen) {
				doneAny = true
				continue
			}
			failedMsgs = append(failedMsgs, "领取失败："+cMsg)
			continue
		}
		benefit, _ := data.(map[string]any)
		if benefit != nil && benefit["id"] != nil {
			cOK, _, _, confMsg := owner.snapRequest(ctx, stream, http.MethodPost, confirmURL, cred, map[string]any{"campaignId": campID})
			if !cOK {
				kept = append(kept, unconfirmedClaim{CampaignID: campID, Benefit: benefit})
				failedMsgs = append(failedMsgs, "领取确认失败："+confMsg)
				continue
			}
		}
		got := credit
		if benefit != nil {
			if bAmt, ok := asFloat(benefit["benefitAmount"]); ok && bAmt > 0 {
				got = bAmt
			}
		}
		claimedSum += got
		claimedAny = true
	}

	if len(kept) > 0 || len(left) > 0 {
		owner.saveUnconfirmed(ctx, stream, stateKey, stateVer, day, kept)
	}

	if len(failedMsgs) > 0 {
		msg := failedMsgs[0]
		if len(failedMsgs) > 1 {
			msg = fmt.Sprintf("%s（另有 %d 个活动失败）", failedMsgs[0], len(failedMsgs)-1)
		}
		return sdk.AccountCheckinResult{Outcome: "failed", Message: msg}
	}
	if claimedAny {
		res := sdk.AccountCheckinResult{Outcome: "claimed", Message: "已领取"}
		if claimedSum > 0 {
			val := claimedSum
			res.Credit = &val
			res.Message = fmt.Sprintf("+%s 积分", formatFloatCompact(claimedSum))
		}
		return res
	}
	if doneAny {
		return sdk.AccountCheckinResult{Outcome: "done", Message: "今天已领取"}
	}
	return sdk.AccountCheckinResult{Outcome: "ineligible", Message: ineligMsg}
}

func (owner *module) officeAceCheckin(ctx context.Context, stream *sdk.Stream, opID string, cred CodeArtsCredential) sdk.AccountCheckinResult {
	if !cred.ValidSTS() {
		return sdk.AccountCheckinResult{Outcome: "failed", Message: "凭据缺失（access 或 securityToken），未发起签到"}
	}
	if stream != nil {
		_ = stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: opID})
	}
	target := strings.TrimRight(owner.options.Endpoints.OASubscriptionBase, "/") + OABonusClaimPath
	status, text, ok := owner.officeAceSigned(ctx, stream, cred, target, http.MethodPost, nil)
	if !ok || status < 200 || status >= 300 {
		detail := strings.TrimSpace(text)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		statusDesc := fmt.Sprintf("%d", status)
		if status == 0 {
			statusDesc = "（未发出/无响应）"
		}
		msg := "HTTP " + statusDesc
		if detail != "" {
			msg += "：" + detail
		}
		return sdk.AccountCheckinResult{Outcome: "failed", Message: msg}
	}
	if strings.TrimSpace(text) == "" {
		return sdk.AccountCheckinResult{Outcome: "claimed", Message: "已领取（响应为空）"}
	}
	var doc map[string]any
	if json.Unmarshal([]byte(text), &doc) != nil || doc == nil {
		return sdk.AccountCheckinResult{Outcome: "done", Message: "已领取（响应无法解析）"}
	}
	rawSKUs, _ := doc["bonus_skus"].([]any)
	if len(rawSKUs) == 0 {
		return sdk.AccountCheckinResult{Outcome: "done", Message: "今日已无新增可领"}
	}
	var total float64
	var parts []string
	for _, rawSKU := range rawSKUs {
		sku, ok := rawSKU.(map[string]any)
		if !ok {
			continue
		}
		name := firstOf(strField(sku, "activity_name"), "活动奖励")
		pts, _ := asFloat(sku["points"])
		if pts < 0 {
			pts = 0
		}
		total += pts
		parts = append(parts, fmt.Sprintf("%s +%s", name, formatFloatCompact(pts)))
	}
	res := sdk.AccountCheckinResult{
		Outcome: "claimed",
		Message: strings.Join(parts, "；"),
	}
	if total > 0 {
		val := total
		res.Credit = &val
	}
	return res
}

func (owner *module) checkin(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.AccountCheckinRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &req) != nil {
		return nil, invalid("Invalid account check-in request")
	}
	if req.Day != "" {
		if err := req.Validate(); err != nil {
			return nil, invalid(err.Error())
		}
	}
	var cred CodeArtsCredential
	if decodeObject(call.Auth, &cred) != nil {
		return nil, unauthenticated("Selected check-in credentials are invalid")
	}
	provider := firstOf(call.Scope.Provider, CodeArtsProvider)
	if provider == OfficeAceProvider {
		return owner.officeAceCheckin(ctx, stream, call.Scope.Operation, cred), nil
	}
	id, _ := AccountIdentityOf(cred)
	accountKey := firstOf(call.Scope.Account, id, cred.AccessKeyID)
	return owner.codeArtsCheckin(ctx, stream, call.Scope.Operation, cred, accountKey), nil
}
