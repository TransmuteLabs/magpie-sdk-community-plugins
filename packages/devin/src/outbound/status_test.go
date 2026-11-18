package outbound

import "testing"

func TestStatusPreservesExplicitZeroAndAbsentACUAndBalance(t *testing.T) {
	zero, err := ParseStatus([]byte(`{"userStatus":{"planStatus":{"acuConsumed":0,"acuLimit":5,"overageBalanceMicros":0}}}`), nil)
	if err != nil || !zero.ACUUsedPresent || !zero.BalancePresent || zero.ACUUsed != 0 || zero.BalanceMicros != 0 {
		t.Fatal("explicit native zero was lost", zero, err)
	}
	missing, err := ParseStatus([]byte(`{"userStatus":{"planStatus":{"acuLimit":5}}}`), nil)
	if err != nil || missing.ACUUsedPresent || missing.BalancePresent {
		t.Fatal("missing native counts became explicit zero", missing, err)
	}
}
