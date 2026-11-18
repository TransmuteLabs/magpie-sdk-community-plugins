package ompsessions

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestCurrentOMPSourceKeepsDeveloperParentsPrivate(test *testing.T) {
	body := sourceJournal(test,
		textMessage("request", "", "Retained user request"),
		message("instructions", "request", "developer", map[string]any{"content": "PRIVATE developer instructions"}),
		message("answer", "instructions", "assistant", map[string]any{
			"content": []map[string]any{{"type": "text", "text": "Retained answer"}},
			"model":   "fixture-model", "usage": map[string]int{"input": 12, "output": 3},
		}),
	)
	before := bytes.Clone(body)
	for _, mode := range []string{"summary", "transcript"} {
		result, err := ReadSessionSource(test.Context(), sourceRequest(test, mode, body))
		if err != nil {
			test.Fatal("source-defined developer parent broke ordinary Source reading:", err)
		}
		if mode == "summary" {
			if len(result.Summary.Usage) != 1 || result.Summary.Usage[0].Input != 12 {
				test.Fatal("developer record changed recorded usage:", result.Summary)
			}
		} else if len(result.Parts) != 2 || result.Parts[0].Text != "Retained user request" ||
			result.Parts[1].Text != "Retained answer" {
			test.Fatal("developer instructions became conversation or broke the branch:", result.Parts)
		}
	}
	if !bytes.Equal(body, before) {
		test.Fatal("Source compatibility changed supplied journal bytes")
	}
}

func TestCurrentOMPRedactedThinkingStaysPrivate(test *testing.T) {
	body := sourceJournal(test,
		textMessage("request", "", "Retained request"),
		message("answer", "request", "assistant", map[string]any{
			"content": []map[string]any{
				{"type": "redactedThinking", "data": "PRIVATE encrypted reasoning"},
				{"type": "thinking", "thinking": "Visible narration", "narration": true},
				{"type": "text", "text": "Retained final answer"},
			},
			"stopReason": "stop",
		}),
	)
	test.Run("source", func(test *testing.T) {
		result, err := ReadSessionSource(test.Context(), sourceRequest(test, "transcript", body))
		if err != nil || len(result.Parts) != 3 || result.Parts[1].Text != "Visible narration" {
			test.Fatal("redacted thinking broke Source display or displaced visible narration:", result, err)
		}
	})
	test.Run("codex-export", func(test *testing.T) {
		exported, err := Convert(test.Context(), sdk.SessionRequest{
			ID: "omp-to-codex", From: "omp", To: "codex",
			Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
			Files:   []sdk.SessionDocument{{Path: "bucket/session.jsonl", Format: "omp", Body: body}},
		})
		if err != nil {
			test.Fatal("known private thinking broke native Codex export:", err)
		}
		output := string(exported.Files[0].Body)
		if strings.Contains(output, "PRIVATE") || !strings.Contains(output, "Visible narration") ||
			!strings.Contains(output, `"type":"Reasoning"`) || !strings.Contains(output, `"phase":"final_answer"`) {
			test.Fatal("private thinking escaped or native narration/final phase changed")
		}
	})
}

func TestCurrentCodexRollbackKeepsOnlySelectedHistory(test *testing.T) {
	prefix := `{"type":"compacted","payload":{"message":"","replacement_history":[{"type":"message","role":"user","content":[{"type":"input_text","text":` +
		string(mustJSON(test, codexSummaryPrefix+"\nRetained compact context")) + `}]}],"replacement_history_metadata":[{"history_sequence":1}]}}` + "\n"
	first := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"First request"}]}}` + "\n"
	second := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Discarded request"},{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}}` + "\n" +
		`{"type":"response_item","payload":{"type":"function_call","call_id":"discarded","name":"exec_command","arguments":"{\"cmd\":\"printf discarded\"}"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"discarded","output":"Discarded output"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"discarded","status":"failed"}}}` + "\n"
	for _, scenario := range []struct {
		name, count string
		remaining   int
	}{
		{"zero", "0", 5},
		{"one", "1", 2},
		{"all", "2", 1},
		{"beyond-history", "4294967295", 1},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			body := []byte(prefix + first + second +
				`{"type":"event_msg","payload":{"type":"thread_rolled_back","num_turns":` + scenario.count + `}}` + "\n")
			items, err := readCodexNative(test.Context(), body)
			if err != nil || len(items) != scenario.remaining || items[0].Content[0].Text != codexSummaryPrefix+"\nRetained compact context" {
				test.Fatalf("rollback retained discarded turns/tools/images or removed compaction context: %+v %v", items, err)
			}
		})
	}
	body := []byte(prefix + first + second +
		`{"type":"event_msg","payload":{"type":"thread_rolled_back","num_turns":1}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Replacement request"}]}}` + "\n" +
		`{"type":"response_item","payload":{"type":"function_call","call_id":"discarded","name":"exec_command","arguments":"{\"cmd\":\"printf retained\"}"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"discarded","output":"Retained output"}}` + "\n")
	items, err := readCodexNative(test.Context(), body)
	if err != nil || len(items) != 5 || items[4].IsError || items[4].Text != "Retained output" {
		test.Fatal("rolled-back tool status contaminated later history:", items, err)
	}
	result, err := Convert(test.Context(), sdk.SessionRequest{
		ID: "codex-to-omp", From: "codex", To: "omp",
		Options: json.RawMessage(`{"cwd":"/work/omp","canonicalCwd":"/work/omp","home":"/home/fixture","temp":"/tmp"}`),
		Files:   []sdk.SessionDocument{{Path: "rollout.jsonl", Format: "codex", Body: body}},
	})
	if err != nil {
		test.Fatal("ordinary Codex-to-OMP route rejected supported rollback:", err)
	}
	output := string(result.Files[0].Body)
	if strings.Contains(output, "Discarded") || strings.Contains(output, "printf discarded") ||
		strings.Contains(output, "AQID") || !strings.Contains(output, "printf retained") {
		test.Fatal("ordinary native migration resurrected discarded context")
	}
}

func TestCurrentCodexRejectsInvalidReplayBoundaries(test *testing.T) {
	for _, count := range []string{"null", "-1", "1.5", "4294967296", `"1"`} {
		body := []byte(`{"type":"event_msg","payload":{"type":"thread_rolled_back","num_turns":` + count + `}}`)
		if _, err := readCodexNative(test.Context(), body); err == nil {
			test.Errorf("invalid rollback count was silently ignored: %s", count)
		}
	}
	if _, err := readCodexNative(test.Context(), []byte(`{"type":"event_msg","payload":{"type":"thread_rolled_back"}}`)); err == nil {
		test.Error("absent rollback count was silently ignored")
	}
	for _, payload := range []string{
		`{"replacement_history":[],"replacement_history_metadata":[{}]}`,
		`{"replacement_history_metadata":[]}`,
		`{"replacement_history":[{"type":"message","role":"user","content":[{"type":"input_text","text":"context"}]}],"replacement_history_metadata":[]}`,
		`{"replacement_history":[{"type":"message","role":"user","content":[]}],"replacement_history_metadata":[null]}`,
	} {
		if _, err := readCodexNative(test.Context(), []byte(`{"type":"compacted","payload":`+payload+`}`)); err == nil {
			test.Error("invalid source-defined replacement metadata alignment was accepted:", payload)
		}
	}
}

func TestCurrentCodexSparseLineageRequiresCapturedHistory(test *testing.T) {
	ancestor := sourceFixture(test, "codex-sparse-ancestor.jsonl")
	child := sourceFixture(test, "codex-sparse-child.jsonl")
	prefix := bytes.Join(bytes.SplitAfter(ancestor, []byte{'\n'})[:3], nil)
	if len(prefix) != 756 || fmt.Sprintf("%x", sha256.Sum256(prefix)) != "dfed2bfd17573fb388783a6c8b394fa8b40a0d44d0276d2610069cb3db1b2879" ||
		bytes.Contains(prefix, []byte("Parent continuation outside inherited prefix")) {
		test.Fatal("synthetic captured ancestor prefix no longer matches its documented range and digest")
	}
	suffix := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Only the child suffix"}]}}` + "\n"
	for _, body := range [][]byte{
		child,
		[]byte(`{"type":"session_meta","payload":{"history_mode":"paginated","history_base":{}}}` + "\n" + suffix),
	} {
		if _, err := readCodexNative(test.Context(), body); err == nil || !strings.Contains(err.Error(), "captured ancestor history") {
			test.Fatal("sparse native lineage was silently imported as a complete conversation:", err)
		}
	}
	request := sdk.SessionRequest{
		ID: "codex-to-omp", From: "codex", To: "omp",
		Options: json.RawMessage(`{"cwd":"/work/fixture","canonicalCwd":"/work/fixture","home":"/home/fixture","temp":"/tmp"}`),
		Files:   []sdk.SessionDocument{{Path: "rollout-child.jsonl", Format: "codex", Body: child}},
	}
	plan, err := PlanSourceInputs(test.Context(), request)
	if err != nil || plan.Version != 1 || len(plan.Files) != 0 {
		test.Fatal("attachment planner acquired unbound Codex ancestor authority:", plan, err)
	}
	if _, err := Convert(test.Context(), request); err == nil || !strings.Contains(err.Error(), "captured ancestor history") {
		test.Fatal("ordinary conversion imported an incomplete native lineage:", err)
	}
	request.Files = append(request.Files, sdk.SessionDocument{Path: "rollout-ancestor.jsonl", Format: "codex", Body: ancestor})
	if _, err := Convert(test.Context(), request); err == nil || !strings.Contains(err.Error(), "one selected source journal") {
		test.Fatal("unbound attachment bytes became native lineage selection authority:", err)
	}
	body := []byte(`{"type":"session_meta","payload":{"history_mode":"paginated","history_base":null}}` + "\n" + suffix)
	items, err := readCodexNative(test.Context(), body)
	if err != nil || len(items) != 1 || items[0].Content[0].Text != "Only the child suffix" {
		test.Fatal("self-contained native history was rejected:", items, err)
	}
}

func mustJSON(test *testing.T, value any) []byte {
	test.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		test.Fatal(err)
	}
	return body
}
