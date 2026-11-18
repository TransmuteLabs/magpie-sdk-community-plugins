package ompsessions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func semanticCodexExport(test *testing.T) sdk.SessionResult {
	test.Helper()
	body, err := os.ReadFile("testdata/omp-codex-semantic-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	result, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
		Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline","model":"offline-model"}`),
		Files:   []sdk.SessionDocument{{Path: "bucket/time_" + sessionID + ".jsonl", Format: "omp", Body: body}}})
	if err != nil {
		test.Fatal(err)
	}
	if output := os.Getenv("OMP_CODEX_EXPORT_DIR"); output != "" {
		if err := os.MkdirAll(output, 0o700); err != nil {
			test.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "rollout.jsonl"), result.Files[0].Body, 0o600); err != nil {
			test.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "id.txt"), []byte(result.ID), 0o600); err != nil {
			test.Fatal(err)
		}
	}
	return result
}

func TestCodexExportRequiresNoInventedModel(test *testing.T) {
	request, _ := nativeSourceFixture(test)
	request.ID, request.To = "omp-to-codex", "codex"
	request.Options = json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`)
	result, err := Convert(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	if bytes.Contains(result.Files[0].Body, []byte(`"turn_context"`)) ||
		bytes.Contains(result.Files[0].Body, []byte(`"model":`)) {
		test.Fatal("legacy options caused a target model to be invented")
	}
}

func TestCodexExportPersistsDurableItemsAndOrdinals(test *testing.T) {
	result := semanticCodexExport(test)
	counts := map[string]int{}
	for ordinal, line := range bytes.Split(bytes.TrimSpace(result.Files[0].Body), []byte{'\n'}) {
		var row struct {
			Ordinal int    `json:"ordinal"`
			Type    string `json:"type"`
			Payload struct {
				Type         string          `json:"type"`
				HistoryMode  string          `json:"history_mode"`
				DynamicTools json.RawMessage `json:"dynamic_tools"`
				Item         struct {
					Type     string          `json:"type"`
					ExitCode *int            `json:"exit_code"`
					Duration json.RawMessage `json:"duration"`
				} `json:"item"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			test.Fatal(err)
		}
		if row.Ordinal != ordinal {
			test.Fatal("native persisted ordinals are not contiguous")
		}
		if row.Type == "session_meta" && (row.Payload.HistoryMode != "paginated" || len(row.Payload.DynamicTools) != 0) {
			test.Fatal("historical tools were registered as runnable tools or native history mode was omitted")
		}
		if row.Type == "event_msg" && row.Payload.Type == "item_completed" {
			counts[row.Payload.Item.Type]++
			if row.Payload.Item.Type == "CommandExecution" && (row.Payload.Item.ExitCode != nil || len(row.Payload.Item.Duration) != 0) {
				test.Fatal("unknown shell exit code/duration was fabricated")
			}
		}
	}
	for kind, count := range map[string]int{"ContextCompaction": 1, "UserMessage": 1, "AgentMessage": 2, "Reasoning": 1, "DynamicToolCall": 1, "CommandExecution": 1} {
		if counts[kind] != count {
			test.Errorf("durable native %s count=%d, want %d", kind, counts[kind], count)
		}
	}
}

func TestCodexExportUnfinishedTurnIsNotCompleted(test *testing.T) {
	request, _ := nativeSourceFixture(test)
	request.ID, request.To = "omp-to-codex", "codex"
	request.Options = json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`)
	body := request.Files[0].Body
	lines := bytes.Split(bytes.TrimSpace(body), []byte{'\n'})
	request.Files[0].Body = append(bytes.Join(lines[:len(lines)-2], []byte{'\n'}), '\n')
	result, err := Convert(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	encoded := string(result.Files[0].Body)
	if !strings.Contains(encoded, `"type":"turn_aborted"`) || strings.Contains(encoded, `"type":"task_complete"`) ||
		strings.Contains(encoded, `"type":"function_call_output"`) || !strings.Contains(encoded, `"status":"in_progress"`) {
		test.Fatal("an unfinished call/turn was completed or given an invented result")
	}
}

func TestCodexExportNativeReverseKeepsResultError(test *testing.T) {
	request, _ := nativeSourceFixture(test)
	request.ID, request.To = "omp-to-codex", "codex"
	request.Options = json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`)
	result, err := Convert(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	items, err := readCodexNative(test.Context(), result.Files[0].Body)
	if err != nil {
		test.Fatal(err)
	}
	for _, item := range items {
		if item.Kind == "result" {
			if !item.IsError {
				test.Fatal("native Codex result lost the recorded error status")
			}
			return
		}
	}
	test.Fatal("native Codex result is missing")
}

func TestCodexExportSelectionAndRefusals(test *testing.T) {
	body, err := os.ReadFile("testdata/omp-codex-semantic-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	for _, scenario := range []struct {
		name, old, replacement, present, absent string
		refuse                                  bool
	}{
		{name: "reset", old: `"type":"compaction"`, replacement: `"type":"reset_boundary"`,
			present: "Selected final answer.", absent: "Retained compact context"},
		{name: "private-compact", old: `"summary":"Retained compact context"`,
			replacement: `"summary":"¶think: private compact"`, present: "Old answer must not return", absent: "¶think:"},
		{name: "duplicate-call", old: `"id":"shell-call"`, replacement: `"id":"read-call"`, refuse: true},
		{name: "orphan-output", old: `"toolCallId":"read-call"`, replacement: `"toolCallId":"unknown"`, refuse: true},
		{name: "unknown-block", old: `"type":"text","text":"Inspecting files."`,
			replacement: `"type":"unknown","text":"Inspecting files."`, refuse: true},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			mutated := bytes.Replace(body, []byte(scenario.old), []byte(scenario.replacement), 1)
			if bytes.Equal(mutated, body) {
				test.Fatal("mutation did not change the fixture")
			}
			result, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
				Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
				Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: mutated}}})
			if scenario.refuse {
				if err == nil {
					test.Fatal("ambiguous or unsupported native history was accepted")
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			output := string(result.Files[0].Body)
			if !strings.Contains(output, scenario.present) || strings.Contains(output, scenario.absent) {
				test.Fatal("reset/private-compaction selected the wrong native history")
			}
		})
	}
}

func TestCodexExportCompactionDoesNotPromoteHistoryToInstructions(test *testing.T) {
	body := semanticCodexExport(test).Files[0].Body
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte{'\n'}) {
		var row struct {
			Type    string `json:"type"`
			Payload struct {
				Replacement []struct {
					Role string `json:"role"`
				} `json:"replacement_history"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			test.Fatal(err)
		}
		if row.Type == "compacted" {
			if len(row.Payload.Replacement) != 1 || row.Payload.Replacement[0].Role != "user" {
				test.Fatal("conversation summary was promoted to developer/system instructions")
			}
			return
		}
	}
	test.Fatal("native compaction missing")
}

func TestCodexExportUnfinishedCallsKeepOrderWithoutInventedOutputs(test *testing.T) {
	body, err := os.ReadFile("testdata/omp-codex-semantic-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(body), []byte{'\n'})
	body = append(bytes.Join(lines[:7], []byte{'\n'}), '\n')
	result, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
		Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
		Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: body}}})
	if err != nil {
		test.Fatal(err)
	}
	var calls, cards []string
	for _, line := range bytes.Split(bytes.TrimSpace(result.Files[0].Body), []byte{'\n'}) {
		var row struct {
			Type    string `json:"type"`
			Payload struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Item   struct {
					Type   string `json:"type"`
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"item"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			test.Fatal(err)
		}
		if row.Type == "response_item" && row.Payload.Type == "function_call" {
			calls = append(calls, row.Payload.CallID)
		}
		if row.Type == "response_item" && row.Payload.Type == "function_call_output" {
			test.Fatal("unfinished history received an invented output")
		}
		if row.Type == "event_msg" && row.Payload.Type == "item_completed" &&
			(row.Payload.Item.Type == "DynamicToolCall" || row.Payload.Item.Type == "CommandExecution") {
			cards = append(cards, row.Payload.Item.ID)
			if row.Payload.Item.Status != "in_progress" {
				test.Fatal("unfinished historical tool marked complete")
			}
		}
	}
	if len(calls) != 2 || strings.Join(calls, ",") != strings.Join(cards, ",") {
		test.Fatal("unfinished native tools changed their original call order/identity")
	}
}

func TestCodexExportBranchSummaryDoesNotReplaceActiveHistory(test *testing.T) {
	body, err := os.ReadFile("testdata/omp-codex-semantic-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	body = bytes.Replace(body, []byte(`{"type":"message","id":"assistant","parentId":"user"`),
		[]byte("{\"type\":\"branch_summary\",\"id\":\"branch-context\",\"parentId\":\"user\",\"summary\":\"Additional branch context\"}\n"+
			`{"type":"message","id":"assistant","parentId":"branch-context"`), 1)
	result, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
		Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
		Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: body}}})
	if err != nil {
		test.Fatal(err)
	}
	if bytes.Count(result.Files[0].Body, []byte(`"type":"compacted"`)) != 1 {
		test.Fatal("branch summary replaced active user history with a second compaction")
	}
	items, err := readCodexNative(test.Context(), result.Files[0].Body)
	if err != nil {
		test.Fatal(err)
	}
	encoded, _ := json.Marshal(items)
	if !bytes.Contains(encoded, []byte("Inspect the selected branch")) ||
		!bytes.Contains(encoded, []byte("Additional branch context")) {
		test.Fatal("native replay lost active user or appended branch context")
	}
}

func TestCodexExportTerminalBranchSummaryDoesNotInventTurn(test *testing.T) {
	body, err := os.ReadFile("testdata/omp-codex-semantic-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	body = append(body, []byte("{\"type\":\"branch_summary\",\"id\":\"branch-context\",\"parentId\":\"final\",\"summary\":\"Additional branch context\"}\n")...)
	result, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
		Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
		Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: body}}})
	if err != nil {
		test.Fatal(err)
	}
	if bytes.Count(result.Files[0].Body, []byte(`"type":"task_started"`)) != 1 ||
		bytes.Contains(result.Files[0].Body, []byte(`"type":"turn_aborted"`)) {
		test.Fatal("terminal branch summary invented an unfinished user turn")
	}
}

func TestCodexExportOmitsRecordedSystemAndDeveloperContext(test *testing.T) {
	body, err := os.ReadFile("testdata/omp-codex-semantic-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	for _, role := range []string{"system", "developer"} {
		test.Run(role, func(test *testing.T) {
			prefix := fmt.Sprintf("{\"type\":\"message\",\"id\":\"injected\",\"parentId\":\"user\",\"message\":{\"role\":%q,\"content\":\"DO_NOT_COPY_INJECTED_CONTEXT\"}}\n", role)
			selected := bytes.Replace(body, []byte(`{"type":"message","id":"assistant","parentId":"user"`),
				[]byte(prefix+`{"type":"message","id":"assistant","parentId":"injected"`), 1)
			result, err := Convert(test.Context(), sdk.SessionRequest{ID: "omp-to-codex", From: "omp", To: "codex",
				Options: json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`),
				Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: selected}}})
			if err != nil {
				test.Fatal(err)
			}
			if bytes.Contains(result.Files[0].Body, []byte("DO_NOT_COPY_INJECTED_CONTEXT")) {
				test.Fatal("source-injected instructions crossed vendor boundary")
			}
		})
	}
}

func TestCodexExportKeepsNativeRolesAndToolHistory(test *testing.T) {
	result := semanticCodexExport(test)
	body := result.Files[0].Body
	for _, artifact := range [][]byte{[]byte("Recorded tool "), []byte("Recorded tool result"), []byte("Recorded tool result attachments:")} {
		if bytes.Contains(body, artifact) {
			test.Errorf("native history contains fabricated role/prose artifact %q", artifact)
		}
	}
	for _, hidden := range [][]byte{[]byte("PRIVATE_ABANDONED_BRANCH"), []byte("Old answer must not return"), []byte("private signed reasoning")} {
		if bytes.Contains(body, hidden) {
			test.Errorf("native export leaked unselected/private history %q", hidden)
		}
	}
	users, calls, outputs, contexts, compacted := 0, map[string]bool{}, map[string]bool{}, 0, 0
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte{'\n'}) {
		var row struct {
			Type    string `json:"type"`
			Payload struct {
				Type   string `json:"type"`
				Role   string `json:"role"`
				Name   string `json:"name"`
				CallID string `json:"call_id"`
				Phase  string `json:"phase"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			test.Fatal(err)
		}
		switch row.Type {
		case "turn_context":
			contexts++
		case "compacted":
			compacted++
		case "response_item":
			switch row.Payload.Type {
			case "message":
				if row.Payload.Role == "user" {
					users++
				}
			case "function_call":
				calls[row.Payload.CallID] = true
			case "function_call_output":
				outputs[row.Payload.CallID] = true
			}
		}
	}
	if users != 1 {
		test.Errorf("tool results or compaction were fabricated as extra user messages: %d", users)
	}
	if len(calls) != 2 || len(outputs) != 2 {
		test.Errorf("arbitrary tool pairs lost native identity: calls=%v outputs=%v", calls, outputs)
	}
	for id := range calls {
		if !outputs[id] {
			test.Errorf("native call %s lacks its output", id)
		}
	}
	if contexts == 0 || compacted != 1 {
		test.Errorf("native turn/compaction boundaries missing: context=%d compacted=%d", contexts, compacted)
	}
}
