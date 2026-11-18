package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func sourceRequest(test *testing.T, mode string, body []byte) sdk.SessionSourceReadRequest {
	test.Helper()
	return sdk.SessionSourceReadRequest{
		SourceID: "omp-history", Mode: mode, Snapshot: strings.Repeat("a", 64),
		Locator: sdk.SessionSourceLocator{StoreRef: "host-selected",
			Path: "bucket/2026-10-25T03-50-25-123Z_" + sessionID + ".jsonl", SessionID: sessionID},
		Documents: []sdk.SessionDocument{{Path: "bucket/2026-10-25T03-50-25-123Z_" + sessionID + ".jsonl", Format: OMPFormat, Body: body}},
		Limits:    sdk.SessionSourceLimits{MaxParts: 1000, MaxTextBytes: 400000},
	}
}

func sourceFixture(test *testing.T, name string) []byte {
	test.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		test.Fatal(err)
	}
	return body
}

func sourceJournal(test *testing.T, records ...map[string]any) []byte {
	test.Helper()
	title, _, _ := bytes.Cut(sourceFixture(test, "omp-v3.jsonl"), []byte{'\n'})
	return append(append(bytes.Clone(title), '\n'), fixtureJournal(test, records...)...)
}

func TestSourcePreviewKeepsDefaultModule(test *testing.T) {
	defaultModule, preview := Module(), SourcePreviewModule()
	if preview.Manifest.Contract.Version.Minor != 10 || len(preview.Manifest.SessionSources) != 1 ||
		!reflect.DeepEqual(defaultModule.Manifest, preview.Manifest) {
		test.Fatal("legacy Source factory differs from the ordinary full distribution")
	}
	if err := preview.Manifest.Validate(); err != nil {
		test.Fatal(err)
	}
	body := sourceFixture(test, "omp-v3.jsonl")
	request := sdk.SessionRequest{ID: "omp-history", From: OMPFormat, To: OMPFormat, Files: sourceRequest(test, "summary", body).Documents}
	raw, _ := json.Marshal(request)
	peer := newSourcePeer(test, preview, allowedSourceConfiguration, "planner-instance", "1")
	frame := peer.call(test, sdk.MethodModuleCall, sdk.ModuleCallRequest{Action: SourceInputsAction, Input: raw})
	var plan SourceInputs
	if frame.Error != nil || json.Unmarshal(frame.Result, &plan) != nil || plan.Version != 1 || len(plan.Files) != 0 {
		test.Fatal("source-read planner attempted acquisition", frame.Error)
	}
	request.ID, request.From, request.To = "omp-to-document", "omp", DocumentFormat
	request.Files[0].Format = "omp"
	raw, _ = json.Marshal(request)
	result, err := preview.Actions[SourceInputsAction](test.Context(), sdk.Call{Params: raw}, nil)
	want, expectedErr := PlanSourceInputs(test.Context(), request)
	if err != nil || expectedErr != nil || !reflect.DeepEqual(result, want) || len(want.Files) != 1 {
		test.Fatal("preview changed ordinary migration attachment planning", result, err)
	}
}

func TestSourceReadRecoveredOMP(test *testing.T) {
	body := sourceFixture(test, "omp-v3.jsonl")
	before := bytes.Clone(body)
	for _, mode := range []string{"summary", "transcript"} {
		request := sourceRequest(test, mode, body)
		result, err := ReadSessionSource(test.Context(), request)
		if err != nil {
			test.Fatal(mode, err)
		}
		wire, err := json.Marshal(result)
		if err != nil {
			test.Fatal(err)
		}
		if _, err := sdk.DecodeSessionSourceResult(request, wire); err != nil {
			test.Fatal("result violates the actual frozen SDK DTO", err)
		}
		if mode == "summary" {
			if result.Summary.Title != "Renamed OMP fixture" || result.Summary.Cwd != "/work/omp" ||
				result.Summary.Start != fixtureTime.Format("2006-01-02T15:04:05.999999999Z07:00") || result.Summary.Usage != nil {
				test.Fatal("summary invented missing model usage or lost recovered fields", result.Summary)
			}
			continue
		}
		var images int
		for _, part := range result.Parts {
			if part.Kind == "image" {
				images++
				if part.Text != "[image]" {
					test.Fatal("uncontracted image representation", part)
				}
			}
		}
		text := string(wire)
		for _, want := range []string{"human request", "private reasoning", "visible narration", "tool_use", "printf fixture", "tool_result", "[error]", "standalone output"} {
			if !strings.Contains(text, want) {
				test.Fatal("transcript lost recovered content", want)
			}
		}
		if images != 2 || strings.Contains(text, "abandoned sibling") || strings.Contains(text, "preserve-signature") {
			test.Fatal("wrong branch/image/signature projection", images, text)
		}
	}
	if !bytes.Equal(before, body) {
		test.Fatal("pure Source decode changed caller bytes")
	}
}

func TestSourceUsageUnknownAndMeasuredZero(test *testing.T) {
	request := sourceRequest(test, "summary", sourceFixture(test, "omp-source-usage-v3.jsonl"))
	result, err := ReadSessionSource(test.Context(), request)
	if err != nil || len(result.Summary.Usage) != 3 || result.Summary.Title != "Recovered summary" {
		test.Fatal("recovered model/usage fields lost", result.Summary, err)
	}
	var input, output uint64
	for _, point := range result.Summary.Usage {
		input += point.Input
		output += point.Output
	}
	if input != 130 || output != 60 || result.Summary.Usage[1].Model != "claude-haiku-5" {
		test.Fatal("model_usage was dropped or counted twice", result.Summary.Usage)
	}
	zero := sourceJournal(test, message("a", "", "assistant", map[string]any{
		"model": "model", "usage": map[string]int{"input": 0}, "content": "zero"}))
	result, err = ReadSessionSource(test.Context(), sourceRequest(test, "summary", zero))
	if err != nil || len(result.Summary.Usage) != 1 || result.Summary.Usage[0].Input != 0 {
		test.Fatal("measured zero became unavailable", result.Summary, err)
	}
	unknown := sourceJournal(test, message("a", "", "assistant", map[string]any{"model": "model", "content": "unknown"}))
	result, err = ReadSessionSource(test.Context(), sourceRequest(test, "summary", unknown))
	if err != nil || result.Summary.Usage != nil {
		test.Fatal("absent usage was reported as measured zero", result.Summary, err)
	}
	unnamed := bytes.Replace(sourceJournal(test, textMessage("a", "", "First user prompt")),
		[]byte(`"title":"Fixture title"`), []byte(`"title":""`), 1)
	result, err = ReadSessionSource(test.Context(), sourceRequest(test, "summary", unnamed))
	if err != nil || result.Summary.Title != "First user prompt" {
		test.Fatal("unnamed history lost the native first-prompt title fallback", result.Summary, err)
	}
}

func TestSourceReadPreservesUnfinishedToolsAndBounds(test *testing.T) {
	body := sourceJournal(test, message("a", "", "assistant", map[string]any{"content": []map[string]any{
		{"type": "text", "text": strings.Repeat("я", 20)},
		{"type": "toolCall", "id": "pending", "name": "read", "arguments": map[string]string{"path": "file.go"}},
	}}))
	request := sourceRequest(test, "transcript", body)
	result, err := ReadSessionSource(test.Context(), request)
	if err != nil || len(result.Parts) != 2 || result.Parts[1].Kind != "tool_use" {
		test.Fatal("Source reused carry filtering and discarded unfinished tool history", result, err)
	}
	request.Limits.MaxParts, request.Limits.MaxTextBytes = 1, 7
	result, err = ReadSessionSource(test.Context(), request)
	if err != nil || !result.Cut || len(result.Parts) != 1 || len(result.Parts[0].Text) > 7 || !utf8.ValidString(result.Parts[0].Text) {
		test.Fatal("transcript did not honor host byte/part limits", result, err)
	}
}

func TestSourceReadRejectsUnknownOrUnsafeInput(test *testing.T) {
	valid := sourceFixture(test, "omp-v3.jsonl")
	_, noSlot, _ := bytes.Cut(valid, []byte{'\n'})
	for name, body := range map[string][]byte{
		"unknown-format":      []byte("{\"type\":\"unknown\"}\n"),
		"pi-without-omp-slot": noSlot,
		"unknown-version":     bytes.Replace(valid, []byte(`"version":3`), []byte(`"version":4`), 1),
		"bad-json":            append(bytes.Clone(valid), []byte("{bad}\n")...),
		"incomplete-prefix":   bytes.TrimSuffix(valid, []byte{'\n'}),
		"oversized":           sourceJournal(test, textMessage("a", "", strings.Repeat("x", MaxBytes))),
		"cycle":               sourceJournal(test, textMessage("a", "a", "cyclic")),
		"missing-parent":      sourceJournal(test, textMessage("a", "missing", "orphan")),
		"invalid-leaf":        sourceJournal(test, node("a", "", "model_change", map[string]any{"activeLeafId": 17})),
		"negative-usage": sourceJournal(test, message("a", "", "assistant", map[string]any{
			"content": "bad", "model": "model", "usage": map[string]int{"input": -1}})),
		"overflow-usage": sourceJournal(test, message("a", "", "assistant", map[string]any{
			"content": "bad", "model": "model", "usage": map[string]uint64{"input": 1 << 63}})),
		"unknown-role":  sourceJournal(test, message("a", "", "unknown", map[string]any{"content": "bad"})),
		"unknown-block": sourceJournal(test, message("a", "", "user", map[string]any{"content": []map[string]string{{"type": "video"}}})),
	} {
		test.Run(name, func(test *testing.T) {
			if _, err := ReadSessionSource(test.Context(), sourceRequest(test, "transcript", body)); err == nil {
				test.Fatal("unsupported or unsafe Source input was accepted")
			}
		})
	}
	request := sourceRequest(test, "summary", valid)
	request.Locator.SessionID = "019a0000-0000-7000-8000-00000000000b"
	request.Locator.Path = strings.ReplaceAll(request.Locator.Path, sessionID, request.Locator.SessionID)
	request.Documents[0].Path = request.Locator.Path
	if _, err := ReadSessionSource(test.Context(), request); err == nil {
		test.Fatal("module did not confirm the host-extracted ID")
	}
	cancelled, cancel := context.WithCancel(test.Context())
	cancel()
	if _, err := ReadSessionSource(cancelled, sourceRequest(test, "summary", valid)); err == nil {
		test.Fatal("canceled Source call succeeded")
	}
}
