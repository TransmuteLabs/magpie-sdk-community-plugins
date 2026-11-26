package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestPublicSDKGoUsageExcludesOnlyConfirmedWorkspaceModels(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	reset := start.Add(3 * time.Hour)
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	var consoleReads, inferenceReads atomic.Int32
	var consoleDown, inferenceDown, duplicateMetadata atomic.Bool
	const freeID = "hy4-flash-free"
	consoleModels := map[string]map[string]any{}
	for _, id := range []string{freeID, "name-only-free", "zero-rate", "paid"} {
		consoleModels[id] = map[string]any{
			"id": id, "type": "chat",
			"limit": map[string]int{"context": 32000, "output": 4096},
			"api":   map[string]any{"chat-completions": map[string]string{"base_url": "https://opencode.ai/inference/go/openai/v1"}},
		}
	}
	consoleModels["zero-rate"]["cost"] = map[string]any{"text": map[string]any{
		"input": map[string]int{"price": 0}, "output": map[string]int{"price": 0},
	}}
	accounts := map[string]string{"access-first": "org-first", "access-peer": "org-first", "access-rotated": "org-first", "access-other-workspace": "org-second"}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		credential := request.Header.Get("Authorization")
		var org string
		for access, workspace := range accounts {
			if credential == "Bearer "+access {
				org = workspace
				break
			}
		}
		header := "x-opencode-org-id"
		if request.URL.Path == "/console/api/v1/models" {
			header = "x-org-id"
		}
		if request.Method != http.MethodGet || org == "" || request.Header.Get(header) != org {
			t.Errorf("wrong account, workspace or verb on %s", request.URL.Path)
			http.Error(writer, "wrong scope", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/zen/go/v1/usage":
			_, _ = io.WriteString(writer, `{"usage":{"rolling":{"percent":100,"resetsAt":"`+reset.Format(time.RFC3339)+`"},"weekly":{"percent":80},"monthly":{"percent":40}}}`)
		case "/console/api/v1/models":
			consoleReads.Add(1)
			if consoleDown.Load() || credential == "Bearer access-rotated" {
				http.Error(writer, "unavailable", http.StatusServiceUnavailable)
				return
			}
			models := consoleModels
			if credential == "Bearer access-peer" {
				models = map[string]map[string]any{"paid": consoleModels["paid"]}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{ProviderGo: map[string]any{"id": ProviderGo, "models": models}})
		case "/inference/go/v1/models":
			inferenceReads.Add(1)
			if inferenceDown.Load() {
				http.Error(writer, "unavailable", http.StatusServiceUnavailable)
				return
			}
			freeMetadata := `{"monthly_usage":null}`
			if duplicateMetadata.Load() {
				freeMetadata = `{"monthly_usage":{"plus":100},"monthly_usage":null}`
			}
			if org == "org-second" {
				freeMetadata = `{"monthly_usage":{"plus":100}}`
			}
			_, _ = io.WriteString(writer, `{"data":[{"id":"`+freeID+`","go":`+freeMetadata+`},{"id":"outside-workspace","go":{"monthly_usage":null}},{"id":"name-only-free"},{"id":"zero-rate","go":{}},{"id":"paid","go":{"monthly_usage":0}}]}`)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	options := ModuleOptions{
		Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL}, Now: now,
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil },
	}
	harness := serveHarness(t, NewWindowModule(options), raw(moduleConfig{ProjectID: "global"}))
	first := credential(start, "first", "org-first")
	checkUsage := func(credentials Credentials, generation uint64, excluded []string) {
		t.Helper()
		scope := sdk.Scope{Provider: ProviderGo, Account: credentials.AccountID(ProviderGo), Generation: generation}
		frame, _ := harness.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{Revision: "27"}, raw(credentials))
		result, fresh, err := sdk.ValidateUsageResult(Manifest(), frame.Result)
		if frame.Error != nil || err != nil || !fresh || result.Revision != "27" || len(result.Windows) != 3 {
			t.Fatalf("invalid Usage for generation %d: result=%s error=%v validation=%v", generation, frame.Result, frame.Error, err)
		}
		for _, window := range result.Windows {
			if !slices.Equal(window.NotModels, excluded) || len(window.Models) != 0 {
				t.Errorf("%s exclusion %v, want %v", window.Name, window.NotModels, excluded)
			}
		}
		firstWindow := result.Windows[0]
		if firstWindow.Name != "5 hours" || firstWindow.Used != 100 || firstWindow.Limit != 100 || firstWindow.ResetsAt != reset.UnixMilli() {
			t.Fatalf("paid quota reading changed: %+v", firstWindow)
		}
	}

	checkUsage(first, 1, []string{freeID})
	if t.Failed() {
		return
	}
	checkUsage(first, 1, []string{freeID})
	if consoleReads.Load() != 1 || inferenceReads.Load() != 1 {
		t.Fatalf("warm usage repeated discovery: Console=%d inference=%d", consoleReads.Load(), inferenceReads.Load())
	}

	checkUsage(credential(start, "peer", "org-first"), 1, nil)
	otherWorkspace := first
	otherWorkspace.Access = "access-other-workspace"
	otherWorkspace.OrgID = "org-second"
	checkUsage(otherWorkspace, 1, nil)
	rotated := first
	rotated.Access = "access-rotated"
	checkUsage(rotated, 2, nil)
	if consoleReads.Load() != 4 || inferenceReads.Load() != 3 {
		t.Fatalf("discovery crossed scope or reached inference with no workspace: Console=%d inference=%d", consoleReads.Load(), inferenceReads.Load())
	}

	clock.Add(int64(time.Minute + time.Second))
	consoleDown.Store(true)
	checkUsage(first, 1, nil)
	consoleDown.Store(false)
	inferenceDown.Store(true)
	checkUsage(first, 1, nil)
	inferenceDown.Store(false)
	duplicateMetadata.Store(true)
	checkUsage(first, 1, nil)
	duplicateMetadata.Store(false)
	checkUsage(first, 1, []string{freeID})
	if consoleReads.Load() != 6 || inferenceReads.Load() != 6 {
		t.Fatalf("discovery did not reuse the repaired workspace: Console=%d inference=%d", consoleReads.Load(), inferenceReads.Load())
	}
}
