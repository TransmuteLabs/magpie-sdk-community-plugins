package commandcode

import (
	"encoding/json"
	"testing"
)

func TestSubscriptionUnknownIsNotNoPlan(t *testing.T) {
	for _, raw := range []string{`{}`, `{"success":false,"data":null}`, `{"data":{"planId":42}}`} {
		if _, err := ParseSubscription([]byte(raw)); err == nil {
			t.Fatalf("unknown subscription accepted: %s", raw)
		}
	}
	sub, err := ParseSubscription([]byte(`{"success":true,"data":null}`))
	if err != nil || sub.Plan != "No plan" {
		t.Fatalf("explicit empty plan: %+v %v", sub, err)
	}
	sub, err = ParseSubscription([]byte(`{"data":{"planId":"individual-pro-v1-monthly","status":"active"}}`))
	if err != nil || sub.Plan != "Pro" || sub.Monthly != 80 || sub.Unit != "credits" {
		t.Fatalf("longest prefix: %+v %v", sub, err)
	}
}

func TestCreditUnitsDoNotAbsorbPurchasedDollars(t *testing.T) {
	sub := Subscription{Plan: "GOAT", Monthly: 70, Unit: "credits"}
	value, err := ParseCredits([]byte(`{"credits":{"monthlyCredits":"60","purchasedCredits":"12","freeCredits":3},"windowLimits":{"fiveHour":{"used":2,"cap":14,"resetAt":1790000000}}}`), sub)
	if err != nil || value.Monthly == nil || *value.Monthly != 60 || value.ExtraUSD == nil || *value.ExtraUSD != 15 {
		t.Fatalf("units: %+v %v", value, err)
	}
	if len(value.Windows) != 2 || value.Windows[1].Used < 14.28 || value.Windows[1].Used > 14.29 {
		t.Fatalf("monthly used should be 10/70: %+v", value.Windows)
	}
	for _, raw := range []string{`{}`, `{"credits":{"monthlyCredits":"NaN"}}`, `{"credits":{"monthlyCredits":-1}}`, `{"credits":{"monthlyCredits":null}}`} {
		if _, err := ParseCredits([]byte(raw), sub); err == nil {
			t.Fatalf("malformed/unknown quota accepted: %s", raw)
		}
	}
}

func TestKnownZeroBalanceAndUnknownWindowsRemainDistinct(t *testing.T) {
	value, err := ParseCredits([]byte(`{"credits":{"purchasedCredits":0}}`), Subscription{})
	if err != nil || value.ExtraUSD == nil || *value.ExtraUSD != 0 || len(value.Windows) != 0 {
		t.Fatalf("known zero: %+v %v", value, err)
	}
	raw, _ := json.Marshal(value)
	if string(raw) == "{}" {
		t.Fatal("known zero was omitted")
	}
}
