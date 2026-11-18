package commandcode

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSystemOneNegotiatesDeclarationAndCanonicalDiscovery(t *testing.T) {
	manifest := Manifest()
	if manifest.Contract.Version.Minor != 12 || manifest.Compatibility.HostAPI != "^1.12" ||
		!slices.Contains(manifest.Providers[0].Operations, sdk.OperationSystemOne) {
		t.Fatalf("decision execution is not negotiated: %+v", manifest)
	}
	for _, goPlan := range []bool{false, true} {
		models, err := ParseModels([]byte(`{"data":[{"id":"fixture-chat"}]}`), Subscription{Plan: "Go"}, false, goPlan)
		if err != nil || len(models) != 2 {
			t.Fatalf("successful discovery omitted the explicit module Jev fact: %+v %v", models, err)
		}
		decision := models[1]
		if decision.ID != Jev || decision.Kind != "decision" || len(decision.Protocols) != 0 ||
			!slices.Equal(decision.Operations, []sdk.ProviderOperation{sdk.OperationSystemOne}) || decision.Tools {
			t.Fatalf("Jev borrowed conversational authority: %+v", decision)
		}
	}
	for _, body := range []string{`{`, `null`, `{"data":[]}`, `{"data":[{}]}`} {
		models, err := ParseModels([]byte(body), Subscription{}, false, false)
		if err == nil || len(models) != 0 {
			t.Fatalf("failed discovery invented a decision fact: %q %+v %v", body, models, err)
		}
	}
}

func TestSystemOneUsesRecoveredWire(t *testing.T) {
	spec := loadSystemOneFixtures(t)
	answer := []byte(`{"model":"typesafe/jev","answers":{"ok":{"type":"noul","noul":0.93}},"usage":{"input_tokens":41,"output_tokens":3}}`)
	for _, alias := range spec.Wire.Aliases {
		t.Run(alias, func(t *testing.T) {
			var requests atomic.Int64
			received := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Error(err)
				}
				var fields map[string]json.RawMessage
				if json.Unmarshal(body, &fields) != nil || string(fields["model"]) != `"typesafe/jev"` ||
					string(fields["vendor_extension"]) != `{"keep":true}` {
					t.Errorf("decision request fields changed: %s", body)
				}
				received <- request
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Retry-After", "7")
				writer.Header().Set("X-Request-Id", "systemone-fixture")
				writer.Header().Set("Set-Cookie", "must-not-return=secret")
				writer.Write(answer)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai",
				WorkingDirectory: "/synthetic/shop", ZDR: true})
			var body map[string]json.RawMessage
			if err := json.Unmarshal(spec.Wire.Request, &body); err != nil {
				t.Fatal(err)
			}
			body["model"], _ = json.Marshal(alias)
			body["vendor_extension"] = json.RawMessage(`{"keep":true}`)
			encoded, _ := json.Marshal(body)
			result, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{
				Operation: sdk.OperationSystemOne, Model: alias, Body: encoded,
				Headers: http.Header{"Authorization": {"Bearer caller-key"}, "Cookie": {"caller=secret"},
					"X-Api-Key": {"caller-key"}},
			}, testAuth())
			if err != nil {
				t.Fatalf("declared decision operation failed: %v", err)
			}
			status, returned := eventsBody(t, events)
			var execution sdk.ExecuteResult
			if json.Unmarshal(result, &execution) != nil || status != 200 || !bytes.Equal(returned, answer) ||
				execution.Outcome != "completed" || execution.Usage == nil ||
				execution.Usage.InputTokens != 41 || execution.Usage.OutputTokens != 3 || requests.Load() != 1 {
				t.Fatalf("decision answer or single usage snapshot changed: %d %s %s", status, returned, result)
			}
			request := <-received
			if request.Method != "POST" || request.URL.Path != "/provider/v1/systemone" ||
				request.Header.Get("Authorization") != "Bearer fixture-key" || request.Header.Get("X-Api-Key") != "" ||
				request.Header.Get("Cookie") != "" || request.Header.Get("User-Agent") != "cli" ||
				request.Header.Get("x-command-code-version") != "1.89.2" || request.Header.Get("x-cli-environment") != "production" ||
				request.Header.Get("x-project-slug") != "shop" || request.Header.Get("x-taste-learning") != "false" ||
				request.Header.Get("x-cmd-zdr") != "1" ||
				!regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(request.Header.Get("x-session-id")) {
				t.Fatalf("decision wire borrowed chat/native headers: %s %s %v", request.Method, request.URL, request.Header)
			}
			if execution.Headers.Get("X-Request-Id") != "systemone-fixture" ||
				execution.Headers.Get("Retry-After") != "7" || execution.Headers.Get("Set-Cookie") != "" ||
				execution.Headers.Get("X-Magpie-Sign-In") != "kept" {
				t.Fatalf("decision reply headers changed: %v", execution.Headers)
			}
		})
	}
}

func TestSystemOneInvalidRequestsNeverReachNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "invalid decision reached the network", 500)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	for _, input := range []sdk.ExecuteRequest{
		{Protocol: "chat", Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`{"model":"typesafe/jev"}`)},
		{Operation: sdk.OperationSystemOne, Model: "fixture-chat", Body: []byte(`{"model":"fixture-chat"}`)},
		{Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`{"model":"another-model"}`)},
		{Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`"a prompt"`)},
		{Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`null`)},
		{Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`{"model":"typesafe/jev","stream":true}`)},
		{Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`{"model":"typesafe/jev"}`), Options: json.RawMessage(`{"method":"GET"}`)},
		{Operation: sdk.OperationSystemOne, Model: Jev, Body: []byte(`{"model":"typesafe/jev"}`), Options: json.RawMessage(`{"path":"/v1/chat/completions"}`)},
	} {
		_, events, err := worker.call(sdk.MethodProviderExecute, input, testAuth())
		var failure *sdk.Error
		if !errors.As(err, &failure) || failure.Outcome != "not-started" || len(events) != 0 || requests.Load() != 0 {
			t.Fatalf("invalid decision reached dispatch: %+v %v events=%d requests=%d", input, err, len(events), requests.Load())
		}
	}
}

func TestSystemOnePlanRefusalKeepsSelectedAccount(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.WriteHeader(400)
		io.WriteString(writer, `{"error":{"message":"MODEL_NOT_IN_PLAN: Jev available in Provider and above plans"}}`)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	result, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{
		Operation: sdk.OperationSystemOne, Model: Jev, Body: loadSystemOneFixtures(t).Wire.Request,
	}, testAuth())
	var execution sdk.ExecuteResult
	if err != nil || json.Unmarshal(result, &execution) != nil || execution.Status != 400 ||
		execution.Refusal == nil || execution.Outcome != "not-started" || len(events) != 0 ||
		!strings.Contains(execution.Refusal.Facts.Message, "MODEL_NOT_IN_PLAN") || requests.Load() != 1 {
		t.Fatalf("plan refusal lost its selected account or original facts: %s %v", result, err)
	}
}

func TestSystemOneInterruptedAnswerIsUncertainWithoutReplay(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Length", "4096")
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"answers":`)
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL, StudioURL: "https://commandcode.ai"})
	_, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{
		Operation: sdk.OperationSystemOne, Model: Jev, Body: loadSystemOneFixtures(t).Wire.Request,
	}, testAuth())
	var failure *sdk.Error
	accepted := 0
	for _, event := range events {
		if event.Event == "accepted" {
			accepted++
		}
	}
	if !errors.As(err, &failure) || failure.Outcome != "uncertain" || accepted != 1 || requests.Load() != 1 {
		t.Fatalf("interrupted decision succeeded or replayed: %v accepted=%d requests=%d", err, accepted, requests.Load())
	}
}

func TestSystemOneUsagePreservesMissingAndZero(t *testing.T) {
	for _, body := range []string{`{}`, `{"usage":null}`, `{"usage":{}}`} {
		totals, err := systemOneUsage([]byte(body))
		if err != nil || totals != nil {
			t.Fatalf("unknown usage became a zero snapshot: %s %+v %v", body, totals, err)
		}
	}
	totals, err := systemOneUsage([]byte(`{"usage":{"input_tokens":0,"output_tokens":0}}`))
	if err != nil || totals == nil || totals.Facts != nil || totals.InputTokens != 0 || totals.OutputTokens != 0 {
		t.Fatalf("explicit zero snapshot became unknown: %+v %v", totals, err)
	}
	totals, err = systemOneUsage([]byte(`{"usage":{"input_tokens":0}}`))
	if err != nil || totals == nil || totals.Facts == nil || totals.Facts.InputTokens == nil ||
		*totals.Facts.InputTokens != 0 || totals.Facts.OutputTokens != nil {
		t.Fatalf("partial usage fabricated another counter: %+v %v", totals, err)
	}
	for _, body := range []string{`{"usage":{"input_tokens":-1,"output_tokens":3}}`,
		`{"usage":{"input_tokens":1.5,"output_tokens":3}}`, `{"usage":{"output_tokens":9007199254740992}}`} {
		if _, err := systemOneUsage([]byte(body)); err == nil {
			t.Fatalf("invalid usage was accepted: %s", body)
		}
	}
}
