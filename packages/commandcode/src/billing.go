package commandcode

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type Subscription struct {
	Plan    string  `json:"plan"`
	Monthly float64 `json:"monthly"`
	Unit    string  `json:"unit"`
	Until   int64   `json:"until,omitempty"`
}

type Credits struct {
	Monthly  *float64          `json:"monthly,omitempty"`
	ExtraUSD *float64          `json:"extraUSD,omitempty"`
	Windows  []sdk.UsageWindow `json:"windows,omitempty"`
}

func ParseSubscription(body []byte) (Subscription, error) {
	var envelope struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if decode(body, &envelope) != nil || envelope.Success != nil && !*envelope.Success || len(envelope.Data) == 0 {
		return Subscription{}, errors.New("subscription is unknown")
	}
	if bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return Subscription{Plan: "No plan"}, nil
	}
	var row struct {
		PlanID string          `json:"planId"`
		Status string          `json:"status"`
		Until  json.RawMessage `json:"currentPeriodEnd"`
	}
	if decode(envelope.Data, &row) != nil || row.PlanID == "" || row.Status == "" {
		return Subscription{}, errors.New("subscription is malformed")
	}
	if row.Status == "canceled" || row.Status == "incomplete_expired" {
		return Subscription{Plan: "No plan"}, nil
	}
	value := subscriptionFor(row.PlanID)
	value.Until = resetTime(row.Until)
	return value, nil
}

func ParseCredits(body []byte, subscription Subscription) (Credits, error) {
	var envelope struct {
		Credits      map[string]json.RawMessage `json:"credits"`
		WindowLimits map[string]json.RawMessage `json:"windowLimits"`
	}
	if decode(body, &envelope) != nil {
		return Credits{}, errors.New("credits are malformed")
	}
	value := Credits{}
	var extra float64
	extraKnown := false
	for _, field := range []string{"monthlyCredits", "purchasedCredits", "freeCredits"} {
		raw, found := envelope.Credits[field]
		if !found {
			continue
		}
		number, err := amount(raw)
		if err != nil {
			return Credits{}, errors.New("credit amount is unknown or invalid")
		}
		if field == "monthlyCredits" {
			value.Monthly = &number
		} else {
			extra += number
			extraKnown = true
		}
	}
	if extraKnown {
		if math.IsInf(extra, 0) {
			return Credits{}, errors.New("credit balance overflow")
		}
		value.ExtraUSD = &extra
	}
	for _, field := range []string{"fiveHour", "weekly"} {
		raw, found := envelope.WindowLimits[field]
		if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var window struct {
			Used     json.RawMessage `json:"used"`
			Cap      json.RawMessage `json:"cap"`
			Reset    json.RawMessage `json:"resetAt"`
			Exceeded bool            `json:"exceeded"`
		}
		if decode(raw, &window) != nil {
			return Credits{}, errors.New("credit window is malformed")
		}
		used, usedErr := amount(window.Used)
		capacity, capErr := amount(window.Cap)
		if usedErr != nil || capErr != nil || capacity <= 0 {
			return Credits{}, errors.New("credit window is unknown or invalid")
		}
		name := "5 hours"
		if field == "weekly" {
			name = "Weekly"
		}
		percent := math.Min(100, 100*(used/capacity))
		if window.Exceeded {
			percent = 100
		}
		value.Windows = append(value.Windows, sdk.UsageWindow{Name: name, Used: percent, Limit: 100, ResetsAt: resetTime(window.Reset)})
	}
	if value.Monthly != nil && subscription.Monthly > 0 {
		pool := math.Max(subscription.Monthly, *value.Monthly)
		left := *value.Monthly
		if subscription.Unit != "credits" {
			pool += extra
			left += extra
		}
		if math.IsInf(pool, 0) {
			return Credits{}, errors.New("credit pool overflow")
		}
		value.Windows = append(value.Windows, sdk.UsageWindow{Name: "Credits", Used: math.Min(100, 100*((pool-left)/pool)), Limit: 100, ResetsAt: subscription.Until})
	}
	if value.Monthly == nil && value.ExtraUSD == nil && len(value.Windows) == 0 {
		return Credits{}, errors.New("credits are unknown")
	}
	return value, nil
}

func subscriptionFor(id string) Subscription {
	normalized := strings.ReplaceAll(strings.ToLower(id), "_", "-")
	for _, value := range []struct {
		prefix, name, unit string
		monthly            float64
	}{
		{"individual-provider", "Provider", "USD", 0},
		{"individual-pro-v1", "Pro", "credits", 80},
		{"individual-goat", "GOAT", "credits", 70},
		{"individual-ultra", "Ultra", "USD", 300},
		{"individual-max", "Max", "USD", 150},
		{"individual-pro", "Pro", "credits", 30},
		{"individual-go", "Go", "credits", 10},
		{"teams-pro", "Teams Pro", "USD", 40},
	} {
		if strings.HasPrefix(normalized, value.prefix) {
			return Subscription{Plan: value.name, Unit: value.unit, Monthly: value.monthly}
		}
	}
	return Subscription{Plan: id}
}

func decode(body []byte, out any) error {
	if len(body) == 0 || len(body) > 16<<20 {
		return errors.New("JSON size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}

func amount(raw json.RawMessage) (float64, error) {
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, err
		}
		text = strings.TrimSpace(text)
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || number < 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, errors.New("invalid nonnegative amount")
	}
	return number, nil
}

func resetTime(raw json.RawMessage) int64 {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if value, err := time.Parse(time.RFC3339Nano, text); err == nil && value.Unix() > 0 {
			return value.Unix()
		}
	}
	number, err := amount(raw)
	if err != nil || number <= 0 {
		return 0
	}
	if number >= 1e12 {
		number /= 1000
	}
	if number > float64(math.MaxInt64/1000) {
		return 0
	}
	return int64(number)
}
