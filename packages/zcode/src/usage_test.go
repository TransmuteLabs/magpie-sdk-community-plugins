package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestQuotaKnownZeroUnknownAndRemainingAreDistinct(t *testing.T) {
	report, err := projectQuota([]byte(`{"code":0,"data":{"limits":[
		{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":99,"usage":100,"remaining":100,"nextResetTime":1794565000000},
		{"type":"CREDIT_LIMIT","unit":6,"number":7},
		{"type":"TIME_LIMIT","unit":5,"number":1,"percentage":100}
	]}}`), fixtureNow.UnixMilli())
	if err != nil || len(report.Windows) != 3 || !report.Windows[2].Aside {
		t.Fatalf("quota: %+v %v", report, err)
	}
	if report.Windows[0].Unknown || report.Windows[0].Used != 0 || report.Windows[0].ResetsAt != 1794565000000 {
		t.Fatal("valid remaining value did not override stale percentage")
	}
	if !report.Windows[1].Unknown || report.Windows[1].Error == "" || report.Error == "" {
		t.Fatal("missing used value converted to a known zero")
	}
}

func TestQuotaFailureAndUncappedCannotInventAllowance(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"code":0,"success":false,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":0}]}}`,
		`{"code":0,"data":{"limits":[]}}`,
		`{"code":401,"data":{"limits":[]}}`,
		`{"code":0,"data":{"limits":[{"type":"TIME_LIMIT","unit":5,"number":1,"percentage":0}]}}`,
		`{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"usage":10,"remaining":20}]}}`,
		`{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":0},{"type":"CREDIT_LIMIT","unit":3,"number":5,"percentage":0}]}}`,
	} {
		if _, err := projectQuota([]byte(body), fixtureNow.UnixMilli()); err == nil {
			t.Fatal("accepted invalid or ambiguous quota")
		}
	}
	report, err := projectQuota([]byte(`{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"usage":0,"percentage":0}]}}`), fixtureNow.UnixMilli())
	if err != nil || len(report.Windows) != 1 || !report.Windows[0].Unknown || report.Error == "" {
		t.Fatal("uncapped informational window became a fresh routing allowance")
	}
}

func TestScopedQuotaReadUsesSeparateBusinessTarget(t *testing.T) {
	type observed struct {
		path    string
		query   string
		headers http.Header
	}
	seen := make(chan observed, 1)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		seen <- observed{request.URL.Path, request.URL.RawQuery, request.Header.Clone()}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":25}]}}`)
	}))
	defer server.Close()
	for _, kind := range []string{"individual", "team", "legacy-key"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t, endpointOptions(server))
			account := fixtureCredentials(kind)
			account.Site = "bigmodel"
			frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
			var result sdk.UsageResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Windows) != 1 ||
				result.Windows[0].Used != 25 || result.AsOf != 0 || result.Windows[0].AsOf != 0 || h.accepted.Load() {
				t.Fatalf("usage: %+v", frame.Error)
			}
			actual := <-seen
			expected := "/quota/bigmodel"
			if kind == "legacy-key" {
				expected = "/quota/legacy-bigmodel"
			}
			if actual.path != expected || actual.headers.Get("Authorization") != account.token() || actual.headers.Get("X-Api-Key") != "" {
				t.Fatal("quota mixed credential roles or family targets")
			}
			if kind == "team" && (actual.query != "type=2" || actual.headers.Get("Bigmodel-Project") != "fixture-project") {
				t.Fatal("Team quota lost selected product scope")
			}
		})
	}
	if requests.Load() != 3 {
		t.Fatal("quota unexpectedly added another vendor request")
	}
}

func TestOrdinaryQuotaCannotLimitNativeExecutionAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/oauth/profile" {
			profile(writer, request, "fixture-user")
			return
		}
		io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":100}]}}`)
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{Revision: "7"}, raw(account))
	var result sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Windows) != 1 ||
		result.Windows[0].Used != 100 || result.Windows[0].AsOf != 0 || result.Error != "" ||
		result.Revision != "7" || len(result.Windows[0].NotModels) != 4 {
		t.Fatal("ordinary 100% quota became an authoritative account-wide limit for native ticket/card aliases")
	}
	for _, alias := range aliasModels() {
		if !containsString(result.Windows[0].NotModels, alias.ID) {
			t.Fatal("ordinary quota omitted a native alias exclusion")
		}
	}
}
