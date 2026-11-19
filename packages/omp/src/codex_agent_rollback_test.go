package ompsessions

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestCodexRollbackInterAgentInstructions(t *testing.T) {
	wire := func(value any) string { return string(mustJSON(t, value)) }
	entry := func(kind string, payload any) string {
		return wire(map[string]any{"type": kind, "payload": payload}) + "\n"
	}
	msg := func(role, kind, text string) map[string]any {
		return map[string]any{"type": "message", "role": role, "content": []any{map[string]any{"type": kind, "text": text}}}
	}
	comm := map[string]any{"author": "/root/a", "recipient": "/root", "content": "instruction", "trigger_turn": false}
	native := map[string]any{"type": "agent_message", "author": "/root/a", "recipient": "/root", "content": []any{map[string]string{"type": "encrypted_content", "encrypted_content": "opaque"}}}
	first := entry("response_item", msg("user", "input_text", "Earlier request")) + entry("response_item", msg("assistant", "output_text", "Earlier answer"))
	for _, tc := range []struct {
		name, boundary string
		keep           bool
	}{
		{"native", entry("response_item", native), true},
		{"record", entry("inter_agent_communication", comm), true},
		{"serialized", entry("response_item", msg("assistant", "output_text", wire(comm))), true},
		{"replacement", entry("compacted", map[string]any{"replacement_history": []any{msg("user", "input_text", "Earlier request"), msg("assistant", "output_text", "Earlier answer"), native}}), true},
		{"invalid-path", entry("response_item", msg("assistant", "output_text", strings.Replace(wire(comm), "/root/a", "/ROOT/a", 1))), false},
		{"duplicate", entry("response_item", msg("assistant", "output_text", strings.Replace(wire(comm), `"trigger_turn":false`, `"trigger_turn":true,"trigger_turn":false`, 1))), false},
		{"missing-trigger", entry("response_item", msg("assistant", "output_text", `{"author":"/root/a","recipient":"/root","content":"ordinary"}`)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := first + tc.boundary + entry("response_item", msg("assistant", "output_text", "Discarded answer")) + entry("event_msg", map[string]any{"type": "thread_rolled_back", "num_turns": 1}) + entry("response_item", msg("user", "input_text", "Fresh question"))
			out := convertCodexJournal(t, body)
			for _, expected := range []string{"Earlier request", "Earlier answer"} {
				if strings.Contains(out, expected) != tc.keep {
					t.Errorf("retained %q = %t, want %t", expected, !tc.keep, tc.keep)
				}
			}
			if strings.Contains(out, "Discarded answer") || strings.Contains(out, "opaque") || !strings.Contains(out, "Fresh question") {
				t.Fatal("conversion retained discarded input or dropped the fresh question")
			}
		})
	}
}

func TestCodexCompletionPastPrivateAgentMessages(t *testing.T) {
	const request = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Question"}]}}` + "\n"
	const answer = `{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Visible reply"}]}}` + "\n"
	const communication = `{"type":"inter_agent_communication","payload":{"author":"/root/a","recipient":"/root","content":"private instruction","trigger_turn":false}}` + "\n"
	const native = `{"type":"response_item","payload":{"type":"agent_message","author":"/root/a","recipient":"/root","content":[{"type":"encrypted_content","encrypted_content":"private ciphertext"}]}}` + "\n"
	for _, scenario := range []struct {
		name, intervening, event, stop string
	}{
		{"complete", "", "task_complete", "stop"},
		{"abort", "", "turn_aborted", "aborted"},
		{"visible-user-boundary", request + native, "task_complete", "toolUse"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			body := request + answer + communication + native + scenario.intervening +
				`{"type":"event_msg","payload":{"type":"` + scenario.event + `"}}` + "\n"
			out := convertCodexJournal(t, body)
			if !strings.Contains(out, "Visible reply") || !strings.Contains(out, `"stopReason":"`+scenario.stop+`"`) {
				t.Fatalf("converted reply must retain stop reason %q: %s", scenario.stop, out)
			}
			for _, private := range []string{"private instruction", "private ciphertext", codexAgentBoundary} {
				if strings.Contains(out, private) {
					t.Fatalf("converted journal contains private material %q", private)
				}
			}
		})
	}
}

func TestCodexOmitsSerializedAgentMessages(t *testing.T) {
	const hidden = `{"author":"/root/a","recipient":"/root","content":"private instruction","encrypted_content":"private ciphertext","trigger_turn":false}`
	const ordinary = `{"author":"/root/a","recipient":"/root","content":"ordinary assistant JSON"}`
	item := func(text string) string {
		return `{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":` + string(mustJSON(t, text)) + `}]}`
	}
	for _, scenario := range []struct {
		name, body string
	}{
		{"response-items", `{"type":"response_item","payload":` + item(hidden) + "}\n" +
			`{"type":"response_item","payload":` + item(ordinary) + "}\n"},
		{"replacement-history", `{"type":"compacted","payload":{"replacement_history":[` + item(hidden) + "," + item(ordinary) + "]}}\n"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			out := convertCodexJournal(t, scenario.body)
			for _, private := range []string{"private instruction", "private ciphertext", codexAgentBoundary} {
				if strings.Contains(out, private) {
					t.Errorf("converted journal contains private material %q", private)
				}
			}
			if !strings.Contains(out, "ordinary assistant JSON") || !strings.Contains(out, `"role":"assistant"`) {
				t.Errorf("converted journal lost ordinary assistant JSON: %s", out)
			}
		})
	}
}

func TestCodexCompletionStaysWithinItsTurn(t *testing.T) {
	const start = `{"type":"event_msg","payload":{"type":"task_started","turn_id":"first"}}` + "\n"
	const answer = `{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Visible reply"}]}}` + "\n"
	const complete = `{"type":"event_msg","payload":{"type":"task_complete","turn_id":"first"}}` + "\n"
	const laterStart = `{"type":"event_msg","payload":{"type":"turn_started","turn_id":"later"}}` + "\n"
	const laterAbort = `{"type":"event_msg","payload":{"type":"turn_aborted","turn_id":"later","reason":"interrupted"}}` + "\n"
	const anonymousAbort = `{"type":"event_msg","payload":{"type":"turn_aborted","turn_id":null,"reason":"interrupted"}}` + "\n"
	const staleAbort = `{"type":"event_msg","payload":{"type":"turn_aborted","turn_id":"first","reason":"interrupted"}}` + "\n"
	const aliasComplete = `{"type":"event_msg","payload":{"type":"turn_complete","turn_id":"later"}}` + "\n"
	for _, scenario := range []struct {
		name, body, stop string
	}{
		{"later-turn-without-reply", start + answer + complete + laterStart + laterAbort, "stop"},
		{"anonymous-abort-after-next-start", start + answer + laterStart + anonymousAbort, "toolUse"},
		{"late-event-from-earlier-turn", laterStart + answer + staleAbort, "toolUse"},
		{"v2-completion", laterStart + answer + aliasComplete, "stop"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			out := convertCodexJournal(t, scenario.body)
			if !strings.Contains(out, "Visible reply") || !strings.Contains(out, `"stopReason":"`+scenario.stop+`"`) {
				t.Fatalf("converted reply must retain stop reason %q: %s", scenario.stop, out)
			}
		})
	}
}

func convertCodexJournal(t *testing.T, body string) string {
	t.Helper()
	result, err := Convert(t.Context(), sdk.SessionRequest{
		ID: "codex-to-omp", From: "codex", To: "omp",
		Options: json.RawMessage(`{"cwd":"/work/omp","canonicalCwd":"/work/omp","home":"/home/fixture","temp":"/tmp"}`),
		Files:   []sdk.SessionDocument{{Path: "rollout.jsonl", Format: "codex", Body: []byte(body)}},
	})
	if err != nil || len(result.Files) != 1 {
		t.Fatalf("expected one converted journal, got %d files, error %v", len(result.Files), err)
	}
	return string(result.Files[0].Body)
}
