package ompsessions

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestWatchedNativeProviderErrorReplay(test *testing.T) {
	const (
		refusal  = "PRIVATE_PROVIDER_REFUSAL"
		notRun   = "PRIVATE_TOOL_NOT_EXECUTED"
		real     = "REAL_TOOL_ERROR"
		partial  = "Visible partial commentary"
		finished = "Selected final answer"
		neutral  = "Tool call was not executed because the provider stream ended with an error before the tool could run."
	)
	text := func(value string) map[string]any {
		return map[string]any{"type": "text", "text": value}
	}
	call := func(id string) map[string]any {
		return map[string]any{"type": "toolCall", "id": id, "name": "bash",
			"arguments": map[string]any{"command": "printf " + id, "cwd": "/work/omp"}}
	}
	result := func(id, parent, callID, output string, details any) map[string]any {
		return message(id, parent, "toolResult", map[string]any{"toolCallId": callID, "toolName": "bash",
			"isError": true, "content": []any{text(output)}, "details": details})
	}
	for _, scenario := range []struct {
		name   string
		stop   string
		detail string
		drop   bool
	}{
		{"provider-refusal", "error", "refusal", true},
		{"provider-sensitive", "error", "sensitive", true},
		{"stream-error", "error", "network", false},
		{"ordinary-answer", "toolUse", "refusal", false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			marker := map[string]any{"__synthetic": true, "source": "assistant_stop_error", "upstreamError": notRun}
			comment := partial
			if scenario.drop {
				comment = refusal
			}
			body := sourceJournal(test,
				textMessage("request", "", "Check the selected history"),
				message("stream", "request", "assistant", map[string]any{
					"stopReason": scenario.stop, "stopDetails": map[string]any{"type": scenario.detail},
					"content": []any{text(comment), call("interrupted-call")},
				}),
				result("placeholder", "stream", "interrupted-call", notRun, marker),
				message("executed", "placeholder", "assistant", map[string]any{
					"stopReason": "toolUse", "content": []any{call("executed-call"), call("other-call")},
				}),
				result("real-error", "executed", "executed-call", real, map[string]any{"source": "assistant_stop_error"}),
				result("near-miss", "real-error", "other-call", "OTHER_TOOL_ERROR", map[string]any{"__synthetic": true, "source": "other"}),
				message("answer", "near-miss", "assistant", map[string]any{"stopReason": "stop", "content": []any{text(finished)}}),
			)
			original := bytes.Clone(body)
			document, err := Decode(test.Context(), body, nil)
			if err != nil || document.Archive == nil || !bytes.Equal(document.Archive.Journal, original) {
				test.Fatal("portable archive lost its original journal", err)
			}
			portable, err := json.Marshal(document.Items)
			if err != nil || !strings.Contains(string(portable), notRun) {
				test.Fatal("portable projection changed with native replay policy", err)
			}
			transcript, err := ReadSessionSource(test.Context(), sourceRequest(test, "transcript", body))
			wire, marshalErr := json.Marshal(transcript.Parts)
			if err != nil || marshalErr != nil || !strings.Contains(string(wire), notRun) || !strings.Contains(string(wire), comment) {
				test.Fatal("selected Source transcript lost recorded context", err, marshalErr, string(wire))
			}
			for _, target := range []struct{ name, options string }{
				{"codex", `{"cwd":"/work/omp","provider":"offline"}`},
				{"claude", `{"cwd":"/work/omp"}`},
			} {
				test.Run(target.name, func(test *testing.T) {
					converted, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-" + target.name,
						From: "omp", To: target.name, Options: json.RawMessage(target.options),
						Files: []sdk.SessionDocument{{Path: "bucket/source.jsonl", Format: "omp", Body: body}}})
					if err != nil || len(converted.Files) != 1 {
						test.Fatal("native migration could not replay the selected history", err)
					}
					journal := string(converted.Files[0].Body)
					for _, forbidden := range []string{refusal, notRun} {
						if strings.Contains(journal, forbidden) {
							test.Errorf("native history replayed provider error %s", forbidden)
						}
					}
					for _, preserved := range []string{real, "OTHER_TOOL_ERROR", finished} {
						if !strings.Contains(journal, preserved) {
							test.Errorf("native history lost %s", preserved)
						}
					}
					for _, detail := range []string{partial, "printf interrupted-call", neutral} {
						if strings.Contains(journal, detail) == scenario.drop {
							test.Errorf("native history has the wrong interrupted context for %q", detail)
						}
					}
					calls := strings.Count(journal, `"type":"tool_use"`)
					results := strings.Count(journal, `"type":"tool_result"`)
					if target.name == "codex" {
						calls = strings.Count(journal, `"type":"function_call"`)
						results = strings.Count(journal, `"type":"function_call_output"`)
					}
					expectedPairs := 3
					if scenario.drop {
						expectedPairs = 2
					}
					if calls != expectedPairs || results != expectedPairs {
						test.Errorf("expected %d native call/result pairs, got %d calls and %d results", expectedPairs, calls, results)
					}
				})
			}
			if !bytes.Equal(body, original) {
				test.Fatal("conversion rewrote the supplied journal")
			}
		})
	}
}

func TestWatchedNativeShellRoundTripWithoutArtifacts(test *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "completed"
		if failed {
			name = "failed"
		}
		test.Run(name, func(test *testing.T) {
			body := sourceJournal(test,
				textMessage("request", "", "Inspect only the selected branch"),
				message("instructions", "request", "developer", map[string]any{"content": "PRIVATE_DEVELOPER"}),
				message("assistant", "instructions", "assistant", map[string]any{
					"stopReason": "toolUse", "content": []any{
						map[string]any{"type": "redactedThinking", "data": "PRIVATE_THINKING"},
						map[string]any{"type": "text", "text": "Inspecting selected files"},
						map[string]any{"type": "toolCall", "id": "shell-call", "name": "bash",
							"arguments": map[string]any{"command": "printf NEVER_EXECUTE_WATCH_FIXTURE", "cwd": "/work/omp"}},
					},
				}),
				message("output", "assistant", "toolResult", map[string]any{
					"toolCallId": "shell-call", "toolName": "bash", "isError": failed,
					"content": []any{map[string]any{"type": "text", "text": "Recorded shell bytes"}},
				}),
				message("answer", "output", "assistant", map[string]any{
					"stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": "Selected final answer"}},
				}),
				textMessage("abandoned", "request", "PRIVATE_ABANDONED"),
				node("selected", "abandoned", "title_change", map[string]any{"title": "Selected", "activeLeafId": "answer"}),
			)
			before := bytes.Clone(body)
			export := sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
				Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
				Files:   []sdk.SessionDocument{{Path: "bucket/source.jsonl", Format: "omp", Body: body}}}
			plan, err := PlanSourceInputs(test.Context(), export)
			if err != nil || len(plan.Files) != 0 {
				test.Fatal("text/shell history acquired an artifact dependency:", plan, err)
			}
			codex, err := Convert(test.Context(), export)
			if err != nil || len(codex.Files) != 1 {
				test.Fatal("native Codex export did not produce one self-contained journal:", codex, err)
			}
			restored, err := Convert(test.Context(), sdk.SessionRequest{ID: "codex-to-omp", From: "codex", To: "omp",
				Options: json.RawMessage(`{"cwd":"/work/omp","canonicalCwd":"/work/omp","home":"/home/fixture","temp":"/tmp"}`),
				Files:   codex.Files})
			if err != nil || len(restored.Files) != 1 {
				test.Fatal("native OMP reverse import required sidecars or lost the history:", restored, err)
			}
			export.Files = restored.Files
			reexported, err := Convert(test.Context(), export)
			if err != nil || len(reexported.Files) != 1 {
				test.Fatal("restored OMP history could not be exported again:", err)
			}
			for _, journal := range [][]byte{codex.Files[0].Body, restored.Files[0].Body, reexported.Files[0].Body} {
				for _, artifact := range [][]byte{[]byte("Recorded tool "), []byte("Recorded bash result"),
					[]byte("PRIVATE_"), []byte("blob:sha256:"), []byte(`"dynamic_tools"`)} {
					if bytes.Contains(journal, artifact) {
						test.Fatalf("native round trip leaked private context or fabricated history: %q", artifact)
					}
				}
			}
			for _, journal := range [][]byte{codex.Files[0].Body, reexported.Files[0].Body} {
				items, err := readCodexNative(test.Context(), journal)
				if err != nil {
					test.Fatal(err)
				}
				var kinds []string
				for _, item := range items {
					kinds = append(kinds, item.Kind)
				}
				if !reflect.DeepEqual(kinds, []string{"user", "assistant", "call", "result", "assistant"}) ||
					items[0].Content[0].Text != "Inspect only the selected branch" ||
					items[1].Text != "Inspecting selected files" || items[4].Text != "Selected final answer" ||
					items[2].Shell != "printf NEVER_EXECUTE_WATCH_FIXTURE" || items[2].Workdir != "/work/omp" ||
					items[2].ID == "" || items[2].ID != items[3].ID ||
					items[3].Text != "Recorded shell bytes" || items[3].IsError != failed {
					test.Fatalf("native roles, shell pair or recorded result changed: %+v", items)
				}
				if bytes.Count(journal, []byte(`"type":"task_started"`)) != 1 ||
					bytes.Count(journal, []byte(`"type":"task_complete"`)) != 1 ||
					bytes.Contains(journal, []byte(`"type":"turn_aborted"`)) {
					test.Fatal("completed native history acquired extra or unfinished turns")
				}
			}
			if !bytes.Equal(body, before) {
				test.Fatal("round trip changed the supplied journal")
			}
		})
	}
}

func TestWatchedRemovedSeanceHeaderGrantsNoInputs(test *testing.T) {
	for _, legacy := range []bool{false, true} {
		body := sourceJournal(test,
			textMessage("request", "", "Selected local journal"),
			message("answer", "request", "assistant", map[string]any{
				"stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": "Selected local answer"}},
			}),
		)
		if legacy {
			body = bytes.Replace(body, []byte(`"version":3`),
				[]byte(`"version":3,"seanceSource":"/PRIVATE_UNSUPPLIED/source.jsonl"`), 1)
		}
		request := sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
			Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
			Files:   []sdk.SessionDocument{{Path: "bucket/source.jsonl", Format: "omp", Body: body}}}
		plan, err := PlanSourceInputs(test.Context(), request)
		if err != nil || len(plan.Files) != 0 {
			test.Fatal("removed donor metadata acquired a foreign session:", plan, err)
		}
		result, err := Convert(test.Context(), request)
		if err != nil || len(result.Files) != 1 || bytes.Contains(result.Files[0].Body, []byte("PRIVATE_UNSUPPLIED")) {
			test.Fatal("removed header metadata escaped into native history:", err)
		}
		transcript, err := ReadSessionSource(test.Context(), sourceRequest(test, "transcript", body))
		if err != nil || len(transcript.Parts) != 2 ||
			transcript.Parts[0].Text != "Selected local journal" || transcript.Parts[1].Text != "Selected local answer" {
			test.Fatal("removed header metadata changed the selected Source transcript:", transcript, err)
		}
	}
}

func TestWatchedNativeShellMessageBoundaries(test *testing.T) {
	for _, scenario := range []struct {
		name       string
		before     int
		after      int
		calls      int
		finalTexts int
		turns      int
	}{
		{"single-commentary", 1, 0, 1, 1, 1},
		{"separated-commentary", 2, 0, 1, 1, 1},
		{"trailing-commentary", 1, 1, 1, 1, 1},
		{"multiple-trailing-commentary", 2, 2, 1, 1, 1},
		{"multiple-calls", 2, 2, 2, 1, 1},
		{"separated-final-text", 2, 1, 1, 2, 1},
		{"distinct-completed-turns", 2, 1, 1, 2, 2},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			var blocks []any
			for index := 0; index < scenario.before; index++ {
				blocks = append(blocks,
					map[string]any{"type": "text", "text": "Recorded leading commentary"},
					map[string]any{"type": "thinking", "thinking": "PRIVATE_LEADING_THINKING"})
			}
			for index := 0; index < scenario.calls; index++ {
				blocks = append(blocks, map[string]any{"type": "toolCall", "id": []string{"shell-a", "shell-b"}[index],
					"name": "bash", "arguments": map[string]any{
						"command": "printf NEVER_EXECUTE_BOUNDARY_FIXTURE", "cwd": "/work/omp"}})
			}
			for index := 0; index < scenario.after; index++ {
				blocks = append(blocks,
					map[string]any{"type": "text", "text": "Recorded trailing commentary"},
					map[string]any{"type": "thinking", "thinking": "PRIVATE_TRAILING_THINKING"})
			}
			entries := []map[string]any{
				textMessage("user", "", "Recorded user request"),
				message("tools", "user", "assistant", map[string]any{"stopReason": "toolUse", "content": blocks}),
			}
			parent := "tools"
			for index := 0; index < scenario.calls; index++ {
				id := []string{"shell-a", "shell-b"}[index]
				entries = append(entries, message(id, parent, "toolResult", map[string]any{
					"toolCallId": id, "toolName": "bash", "isError": false,
					"content": []any{map[string]any{"type": "text", "text": "Recorded shell output"}},
				}))
				parent = id
			}
			var final []any
			for index := 0; index < scenario.finalTexts; index++ {
				final = append(final,
					map[string]any{"type": "text", "text": "Recorded final answer"},
					map[string]any{"type": "thinking", "thinking": "PRIVATE_FINAL_THINKING"})
			}
			entries = append(entries, message("answer", parent, "assistant",
				map[string]any{"stopReason": "stop", "content": final}))
			if scenario.turns == 2 {
				entries = append(entries, textMessage("next-user", "answer", "Recorded second request"),
					message("next-answer", "next-user", "assistant", map[string]any{"stopReason": "stop",
						"content": []any{map[string]any{"type": "text", "text": "Recorded second answer"}}}))
			}
			body := sourceJournal(test, entries...)
			original := bytes.Clone(body)
			forward := sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
				Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
				Files:   []sdk.SessionDocument{{Path: "source.jsonl", Format: "omp", Body: body}}}
			first, err := Convert(test.Context(), forward)
			if err != nil || len(first.Files) != 1 {
				test.Fatal("original completed history did not export:", err)
			}
			current := first
			for cycle := 0; cycle < 2; cycle++ {
				journal := current.Files[0].Body
				if bytes.Count(journal, []byte(`"type":"task_started"`)) != scenario.turns ||
					bytes.Count(journal, []byte(`"type":"task_complete"`)) != scenario.turns ||
					bytes.Contains(journal, []byte(`"type":"turn_aborted"`)) {
					test.Fatalf("cycle %d changed recorded completion boundaries", cycle)
				}
				restored, err := Convert(test.Context(), sdk.SessionRequest{ID: "codex-to-omp", From: "codex", To: "omp",
					Options: json.RawMessage(`{"cwd":"/work/omp","canonicalCwd":"/work/omp","home":"/home/fixture","temp":"/tmp"}`),
					Files:   current.Files})
				if err != nil || len(restored.Files) != 1 {
					test.Fatal("native reverse import failed:", err)
				}
				forward.Files = restored.Files
				current, err = Convert(test.Context(), forward)
				if err != nil || len(current.Files) != 1 {
					test.Fatalf("cycle %d could not reexport recorded calls/results: %v", cycle, err)
				}
				journal = current.Files[0].Body
				for _, private := range []string{"PRIVATE_", "Recorded tool ", "blob:sha256:", `"dynamic_tools"`} {
					if bytes.Contains(journal, []byte(private)) {
						test.Fatalf("cycle %d leaked reasoning or fabricated a history artifact: %s", cycle, private)
					}
				}
				if bytes.Count(journal, []byte(`"type":"task_started"`)) != scenario.turns ||
					bytes.Count(journal, []byte(`"type":"task_complete"`)) != scenario.turns ||
					bytes.Contains(journal, []byte(`"type":"turn_aborted"`)) {
					test.Fatalf("cycle %d invented or lost a completed turn", cycle)
				}
				items, err := readCodexNative(test.Context(), journal)
				if err != nil {
					test.Fatal(err)
				}
				calls, results := 0, 0
				var leading, trailing, answers int
				for _, item := range items {
					switch item.Kind {
					case "call":
						calls++
						if item.Shell != "printf NEVER_EXECUTE_BOUNDARY_FIXTURE" || item.Workdir != "/work/omp" {
							test.Fatalf("recorded shell changed: %+v", item)
						}
					case "result":
						results++
						if item.Text != "Recorded shell output" || item.IsError {
							test.Fatalf("recorded shell result changed: %+v", item)
						}
					case "assistant":
						leading += bytes.Count([]byte(item.Text), []byte("Recorded leading commentary"))
						trailing += bytes.Count([]byte(item.Text), []byte("Recorded trailing commentary"))
						answers += bytes.Count([]byte(item.Text), []byte("Recorded final answer"))
					}
				}
				if calls != scenario.calls || results != scenario.calls ||
					leading != scenario.before || trailing != scenario.after || answers != scenario.finalTexts {
					test.Fatalf("cycle %d lost ordered visible history: calls=%d results=%d leading=%d trailing=%d final=%d",
						cycle, calls, results, leading, trailing, answers)
				}
			}
			if !bytes.Equal(body, original) {
				test.Fatal("supplied source journal changed")
			}
		})
	}
}
