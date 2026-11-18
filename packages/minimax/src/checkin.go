package minimax

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type checkinDay struct {
	Number int     `json:"day_no"`
	Status int     `json:"status"`
	Today  bool    `json:"is_today"`
	Points float64 `json:"points"`
}

func checkinPanel(body []byte) ([]checkinDay, int, error) {
	var envelope struct {
		Data struct {
			Days []checkinDay `json:"days"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Data.Days) != 7 {
		return nil, -1, invalid("MiniMax check-in panel is malformed")
	}
	days := envelope.Data.Days
	sort.Slice(days, func(left, right int) bool { return days[left].Number < days[right].Number })
	today, claimable := -1, 0
	for index, day := range days {
		if day.Number != index+1 || day.Status < 1 || day.Status > 4 || day.Points < 0 || math.IsNaN(day.Points) || math.IsInf(day.Points, 0) {
			return nil, -1, invalid("MiniMax check-in panel has invalid days")
		}
		if day.Today {
			if today != -1 {
				return nil, -1, invalid("MiniMax check-in panel has multiple current days")
			}
			today = index
		}
		if day.Status == 2 {
			claimable++
		}
	}
	if claimable > 1 {
		return nil, -1, invalid("MiniMax check-in panel has multiple claimable days")
	}
	return days, today, nil
}

func checkinStreak(days []checkinDay, index int) int {
	streak := 0
	for index >= 0 && days[index].Status == 3 {
		streak++
		index--
	}
	return streak
}

func (owner *module) checkin(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.AccountCheckinRequest
	if decode(call.Params, &request) != nil || request.Validate() != nil {
		return nil, invalid("A canonical check-in day is required")
	}
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, err
	}
	if client.credentials.UserID == "" {
		account, err := client.identity(ctx)
		if err != nil {
			return nil, safeError(err)
		}
		client.credentials.UserID = account.UserID
	}
	panel, err := client.accountJSON(ctx, "/minimax-cloud/api/v1/signin/status", nil, checkinQuery(), "")
	if err != nil {
		return nil, safeError(err)
	}
	days, today, err := checkinPanel(panel)
	if err != nil {
		return nil, err
	}
	if today == -1 || days[today].Status == 1 {
		return sdk.AccountCheckinResult{Outcome: "inactive"}, nil
	}
	if days[today].Status == 4 {
		return sdk.AccountCheckinResult{Outcome: "ineligible"}, nil
	}
	if days[today].Status == 3 {
		streak := checkinStreak(days, today)
		result := sdk.AccountCheckinResult{Outcome: "done", Streak: &streak}
		if days[today].Points > 0 {
			result.Credit = &days[today].Points
		}
		return result, nil
	}
	key := "checkin." + request.Day
	var fence sdk.StateCASResult
	if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{
		Key: key, Version: "0", Value: raw(map[string]string{"state": "pending", "operation": call.Scope.Operation}),
	}, &fence); err != nil {
		return nil, err
	}
	if !fence.Swapped {
		return nil, uncertain("MiniMax check-in may already have been claimed; the write was not repeated")
	}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	var invalidated sdk.UsageInvalidateResult
	if err := stream.HostCall(ctx, sdk.MethodUsageInvalidate, sdk.UsageInvalidateRequest{}, &invalidated); err != nil {
		return nil, err
	}
	body, err := client.accountJSON(ctx, "/minimax-cloud/api/v1/signin/claim", map[string]any{}, checkinQuery(), "")
	if err != nil {
		return nil, uncertain("MiniMax check-in outcome is unknown; the claim was not repeated")
	}
	data := object(parseObject(body)["data"])
	outcome, valid := number(data["claim_result"])
	if !valid || outcome != 1 && outcome != 2 {
		return nil, uncertain("MiniMax returned an unknown check-in outcome")
	}
	streak := checkinStreak(days, today-1) + 1
	result := sdk.AccountCheckinResult{Outcome: "claimed", Streak: &streak}
	if outcome == 2 {
		result.Outcome = "done"
	}
	if points := positive(data["points"]); points > 0 {
		result.Credit = &points
	}
	var persisted sdk.StateCASResult
	if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{
		Key: key, Version: fence.Version, Value: raw(result),
	}, &persisted); err != nil {
		return nil, err
	}
	if !persisted.Swapped {
		return nil, uncertain("MiniMax check-in receipt changed before publication")
	}
	return result, nil
}
