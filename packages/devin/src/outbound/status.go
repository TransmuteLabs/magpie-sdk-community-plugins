package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type Window struct {
	Name      string
	Used      float64
	ResetsAt  int64
	NotModels []string
	Span      int64
}

type Status struct {
	Email           string
	Name            string
	UserID          string
	TeamID          string
	OrgID           string
	OrgName         string
	Plan            string
	Until           time.Time
	Windows         []Window
	ACUUsed         float64
	ACUUsedPresent  bool
	ACULimit        float64
	ACULimitPresent bool
	BalanceMicros   int64
	BalancePresent  bool
}

func numeric(raw json.RawMessage) (float64, bool, error) {
	return quotaNumeric(raw, false)
}

func quotaNumeric(raw json.RawMessage, signed bool) (float64, bool, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, false, nil
	}
	value := string(raw)
	if len(value) > 0 && value[0] == '"' {
		if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) != value || value == "" {
			return 0, true, errors.New("invalid native quota number")
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || !signed && number < 0 || math.Abs(number) > 9007199254740991 {
		return 0, true, errors.New("invalid native quota number")
	}
	return number, true, nil
}

func ParseStatus(payload []byte, freeIDs []string) (Status, error) {
	var reply struct {
		Status *struct {
			Email     string `json:"email"`
			Name      string `json:"name"`
			UserID    string `json:"userId"`
			TeamID    string `json:"teamId"`
			TeamName  string `json:"teamName"`
			TeamsTier string `json:"teamsTier"`
			Plan      struct {
				End         string          `json:"planEnd"`
				Daily       json.RawMessage `json:"dailyQuotaRemainingPercent"`
				DailyReset  json.RawMessage `json:"dailyQuotaResetAtUnix"`
				Weekly      json.RawMessage `json:"weeklyQuotaRemainingPercent"`
				WeeklyReset json.RawMessage `json:"weeklyQuotaResetAtUnix"`
				ACUUsed     json.RawMessage `json:"acuConsumed"`
				ACULimit    json.RawMessage `json:"acuLimit"`
				Balance     json.RawMessage `json:"overageBalanceMicros"`
				Info        struct {
					Name       string `json:"planName"`
					TeamName   string `json:"teamName"`
					Billing    string `json:"billingStrategy"`
					HideDaily  bool   `json:"hideDailyQuota"`
					HideWeekly bool   `json:"hideWeeklyQuota"`
					Devin      struct {
						OrgID string `json:"orgId"`
						Name  string `json:"accountDisplayName"`
					} `json:"devinInfo"`
				} `json:"planInfo"`
			} `json:"planStatus"`
		} `json:"userStatus"`
	}
	if decodeReply(payload, &reply) != nil || reply.Status == nil {
		return Status{}, errors.New("invalid native account status")
	}
	source := reply.Status
	out := Status{Email: source.Email, Name: source.Name, UserID: source.UserID, TeamID: source.TeamID, OrgID: source.Plan.Info.Devin.OrgID,
		OrgName: source.Plan.Info.Devin.Name, Plan: source.Plan.Info.Name}
	if out.OrgName == "" {
		out.OrgName = source.Plan.Info.TeamName
	}
	if out.OrgName == "" {
		out.OrgName = source.TeamName
	}
	if out.Plan == "" {
		out.Plan = source.TeamsTier
	}
	if source.Plan.End != "" {
		value, err := time.Parse(time.RFC3339Nano, source.Plan.End)
		if err != nil {
			return Status{}, errors.New("invalid native plan end")
		}
		out.Until = value
	}
	for _, entry := range []struct {
		name        string
		span        int64
		left, reset json.RawMessage
		hidden      bool
	}{{"1 day", 86400, source.Plan.Daily, source.Plan.DailyReset, source.Plan.Info.HideDaily},
		{"7 days", 604800, source.Plan.Weekly, source.Plan.WeeklyReset, source.Plan.Info.HideWeekly}} {
		left, present, err := numeric(entry.left)
		reset, _, resetErr := numeric(entry.reset)
		if err != nil || resetErr != nil || left > 100 || reset >= 1<<53 || math.Trunc(reset) != reset {
			return Status{}, errors.New("invalid native quota window")
		}
		if entry.hidden || !present && !(source.Plan.Info.Billing == "BILLING_STRATEGY_QUOTA" && reset > 0) {
			continue
		}
		out.Windows = append(out.Windows, Window{Name: entry.name, Used: 100 - left, ResetsAt: int64(reset), Span: entry.span,
			NotModels: append([]string(nil), freeIDs...)})
	}
	var err error
	out.ACUUsed, out.ACUUsedPresent, err = numeric(source.Plan.ACUUsed)
	if err != nil {
		return Status{}, err
	}
	out.ACULimit, out.ACULimitPresent, err = numeric(source.Plan.ACULimit)
	if err != nil {
		return Status{}, err
	}
	balance, balancePresent, err := quotaNumeric(source.Plan.Balance, true)
	if err != nil || math.Trunc(balance) != balance {
		return Status{}, errors.New("invalid native balance")
	}
	out.BalanceMicros = int64(balance)
	out.BalancePresent = balancePresent
	return out, nil
}

func (client *Client) Status(ctx context.Context, credentials Credentials, freeIDs []string) (Status, error) {
	if err := client.selected(credentials); err != nil {
		return Status{}, err
	}
	body, _ := json.Marshal(map[string]any{"metadata": map[string]string{
		"ideName": "devin-cli", "ideVersion": codec.NativeClientVersion,
		"extensionName": "devin-cli", "extensionVersion": codec.NativeClientVersion,
		"apiKey": credentials.Key, "locale": "en",
		"os": runtime.GOOS,
	}})
	payload, err := client.unary(ctx, "status", StatusPath, "application/json", body)
	if err != nil {
		return Status{}, err
	}
	status, err := ParseStatus(payload, freeIDs)
	if err != nil {
		return Status{}, &Error{Operation: "status", Code: "data_loss"}
	}
	return status, nil
}
