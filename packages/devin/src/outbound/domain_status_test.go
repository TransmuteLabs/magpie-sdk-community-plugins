package outbound

import "testing"

func TestDomainStatusKeepsSignedMicrosWithoutInventingCurrency(t *testing.T) {
	for _, fixture := range []struct {
		raw     string
		want    int64
		present bool
	}{
		{`"-2500000"`, -2500000, true},
		{`"0"`, 0, true},
		{`"9007199254740991"`, 9007199254740991, true},
		{`null`, 0, false},
	} {
		t.Run(fixture.raw, func(t *testing.T) {
			status, err := ParseStatus([]byte(`{"userStatus":{"planStatus":{"overageBalanceMicros":`+fixture.raw+`}}}`), nil)
			if err != nil || status.BalanceMicros != fixture.want || status.BalancePresent != fixture.present {
				t.Fatalf("signed int64 micros or explicit absence changed: %+v err=%v", status, err)
			}
		})
	}
}

func TestDomainStatusRefusesInexactQuotaNumbers(t *testing.T) {
	for _, fields := range []string{
		`"acuConsumed":9007199254740992`,
		`"acuLimit":"1e100"`,
		`"acuConsumed":-1`,
		`"overageBalanceMicros":"9007199254740992"`,
		`"overageBalanceMicros":"-9007199254740992"`,
		`"overageBalanceMicros":1.5`,
	} {
		t.Run(fields, func(t *testing.T) {
			if _, err := ParseStatus([]byte(`{"userStatus":{"planStatus":{`+fields+`}}}`), nil); err == nil {
				t.Fatal("unrepresentable native quota number became a trusted reading")
			}
		})
	}
}
