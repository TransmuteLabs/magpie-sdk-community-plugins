package zcode

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestPolicyHostDenialStopsTransportAndReturnsFreshWholeRefusal(t *testing.T) {
	for _, kind := range []string{"individual", "start"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path == "/api/v1/zcode-plan/billing/balance" {
					io.WriteString(writer, confirmedStartGLM52)
					return
				}
				calls.Add(1)
				writer.Header().Set("Retry-After", "7200")
				writer.WriteHeader(429)
				io.WriteString(writer, `{"code":3008,"msg":"user concurrency limit exceeded"}`)
			}))
			t.Cleanup(server.Close)
			harness := newHarness(t, endpointOptions(server))
			account := fixtureCredentials(kind)
			model := "GLM-5.3"
			if kind == "start" {
				model = "GLM-5.2"
			}
			frame, body := harness.call(sdk.MethodProviderExecute, scope(account), giftRequest(model), raw(account))
			var result sdk.ExecuteResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || calls.Load() != 1 {
				t.Fatalf("host denial did not stop transport: calls=%d error=%+v result=%s", calls.Load(), frame.Error, frame.Result)
			}
			if harness.accepted.Load() || len(body) != 0 || len(harness.headers) != 0 ||
				result.Outcome != "not-started" || result.Refusal == nil {
				t.Fatalf("proved rejection published effects or lost final facts: %+v accepted=%t", result, harness.accepted.Load())
			}
			if len(harness.recoveries) != 1 || result.Refusal.Facts.Status != 429 ||
				result.Refusal.Facts.Code != "3008" || result.Refusal.Facts.Headers["Retry-After"] != "7200" ||
				result.Refusal.Evidence != "rejected-before-effect" {
				t.Fatal("final refusal lacks original bounded facts and separate evidence")
			}
		})
	}
}

func TestPolicyBothManifestsNegotiatePublicRecovery(t *testing.T) {
	for _, manifest := range []sdk.Manifest{Manifest(), ControlManifest()} {
		if manifest.Contract.Version.Minor != 12 || manifest.Compatibility.HostAPI != "^1.12" ||
			!manifest.Contract.Has(sdk.RequestPolicy) {
			t.Fatalf("runtime uses policy without its SDK floor: %+v", manifest.Contract)
		}
	}
}

func TestPolicyEvidenceRequiresAnUnambiguousOutputFreeVendorRefusal(t *testing.T) {
	owner := &module{options: ModuleOptions{Now: func() time.Time { return fixtureNow }}}
	for _, body := range []string{
		`{"code":3008,"usage":{"input_tokens":0}}`,
		`{"code":3008,"data":{"output":[]}}`,
		`{"code":3008,"error":{"code":3008,"usage":{}}}`,
		`{"code":3008,"success":true}`,
		`{"code":3008,"task_id":"accepted-task"}`,
		`{"code":3008,"request_id":{"output":[]}}`,
		`{"code":3008,"error":{"code":3010}}`,
		`{"code":3008,"code":3008}`,
		`{"code":3008,"msg":false}`,
		`{"code":3008,"msg":"` + string(bytes.Repeat([]byte("x"), 1025)) + `"}`,
		`{"code":3008`,
		`{"code":500,"msg":"request-id 3008"}`,
		`{"error":null}`,
		`{}`,
	} {
		t.Run(body[:min(len(body), 70)], func(t *testing.T) {
			response := &http.Response{StatusCode: 429, Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(bytes.NewBufferString(body))}
			failure, err := owner.requestFailure(response, "paid-messages")
			if err != nil || failure.Evidence != "" {
				t.Fatalf("ambiguous or output-bearing response authorized replay: %+v %v", failure, err)
			}
			restored, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || string(restored) != body {
				t.Fatal("fact extraction changed the original response")
			}
		})
	}
}

func TestPolicyUnrepresentableTimingNeverAuthorizesShorterRetry(t *testing.T) {
	owner := &module{options: ModuleOptions{Now: func() time.Time { return fixtureNow }}}
	for _, header := range []http.Header{
		{"Retry-After": {"1", "7200"}},
		{"Retry-After": {string(bytes.Repeat([]byte("9"), 301))}},
		{"Retry-After-Ms": {"1", "7200000"}},
		{"Retry-After-Ms": {"2592000001"}},
		{"Retry-After-Ms": {"not-a-duration"}},
	} {
		header.Set("Content-Type", "application/json")
		response := &http.Response{StatusCode: 429, Header: header,
			Body: io.NopCloser(bytes.NewBufferString(`{"code":3008}`))}
		failure, err := owner.requestFailure(response, "paid-messages")
		response.Body.Close()
		if err != nil || failure.Facts.Complete || failure.Evidence != "" {
			t.Fatalf("a lost timing bound acquired retry authority: %+v %v", failure, err)
		}
	}
}

func TestPolicyResetUsesDocumentedBeijingMessageAndExactModelWindow(t *testing.T) {
	owner := &module{options: ModuleOptions{Now: func() time.Time { return fixtureNow }}}
	reset := fixtureNow.Add(2 * time.Hour)
	beijing := reset.In(time.FixedZone("Beijing", 8*60*60)).Format("2006-01-02 15:04:05")
	for _, scenario := range []struct {
		code    int
		message string
		want    string
	}{
		{1308, "您的限额将在 " + beijing + " 重置。", reset.UTC().Format(time.RFC3339)},
		{1310, "Limit will reset at " + beijing, reset.UTC().Format(time.RFC3339)},
		{3008, "Limit will reset at " + beijing, ""},
		{1308, "request-id " + beijing, ""},
		{1308, "reset at 2099-01-01 01:00:00", ""},
		{1308, "reset at 2026-02-31 01:00:00", ""},
	} {
		body := raw(map[string]any{"code": scenario.code, "msg": scenario.message})
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(bytes.NewReader(body))}
		failure, err := owner.requestFailure(response, "start-messages")
		if err != nil || failure.Facts.ResetAt != scenario.want || failure.Facts.Status != 200 {
			t.Fatalf("typed reset lost provenance or original status: %+v %v", failure, err)
		}
		final, err := owner.finalRefusal(response, "GLM-5.3-Trial", true)
		response.Body.Close()
		if err != nil || final == nil || final.Refusal == nil || len(final.Refusal.Windows) != 1 ||
			len(final.Refusal.Windows[0].Models) != 1 || final.Refusal.Windows[0].Models[0] != "GLM-5.3-Trial" ||
			final.Refusal.Windows[0].ResetAt != scenario.want {
			t.Fatal("a free reset became paid allowance or final facts were stale")
		}
	}
}
