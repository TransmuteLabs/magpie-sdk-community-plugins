package ompsessions

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOMPAdditiveV19JournalEntriesDecodeAndExcludePlanModeContext(t *testing.T) {
	title, _, _ := bytes.Cut(sourceFixture(t, "omp-v3.jsonl"), []byte{'\n'})
	header, err := json.Marshal(map[string]any{
		"type":      "session",
		"version":   3,
		"id":        sessionID,
		"cwd":       "/work/omp",
		"timestamp": fixtureTime.Format(time.RFC3339Nano),
		"title":     "Chrome side panel session",
		"origin":    "chrome",
	})
	if err != nil {
		t.Fatal(err)
	}
	records := []map[string]any{
		node("mc-temp", "", "model_change", map[string]any{
			"model":                   "openai/gpt-6.2-sol",
			"role":                    "temporary",
			"resolvedModelIsFallback": true,
			"fallback": map[string]any{
				"from":    "anthropic/claude-sonnet-4-6:high",
				"trigger": "usage-preflight",
				"reason":  "window exhausted",
			},
		}),
		node("mu-judge", "mc-temp", "model_usage", map[string]any{
			"model":    "gpt-6-luna",
			"duration": 420,
			"ttft":     95,
			"usage":    map[string]any{"input": 100, "output": 20, "cacheRead": 10, "cacheWrite": 0},
			"judgment": map[string]any{
				"state":   strings.Repeat("b", 64),
				"answers": map[string]any{"safe": 0.98},
				"cached":  1,
			},
			"completion": map[string]any{"prompt": "title?", "reply": "Chrome side panel session"},
		}),
		node("tan-boundary", "mu-judge", "custom", map[string]any{
			"customType": "tan-fork-boundary",
			"data":       sessionID,
		}),
		node("plan-ctx", "tan-boundary", "custom_message", map[string]any{
			"customType":  "plan-mode-context",
			"attribution": "user",
			"content":     "stale read-only plan directive",
		}),
		textMessage("u1", "plan-ctx", "keep real user turn"),
		message("a1", "u1", "assistant", map[string]any{
			"model":              "gpt-6.2-sol",
			"errorFromAgentLoop": true,
			"content":            []map[string]any{{"type": "text", "text": "visible reply"}},
			"usage":              map[string]any{"input": 50, "output": 15, "cacheRead": 0, "cacheWrite": 0},
		}),
	}
	var body bytes.Buffer
	body.Write(title)
	body.WriteByte('\n')
	body.Write(header)
	body.WriteByte('\n')
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		body.Write(encoded)
		body.WriteByte('\n')
	}

	document, err := decodeOMPJournal(t.Context(), body.Bytes(), map[string][]byte{}, false)
	if err != nil {
		t.Fatalf("decodeOMPJournal rejected additive v19 entries: %v", err)
	}
	rendered := itemWords(document.Items)
	if !strings.Contains(rendered, "keep real user turn") || !strings.Contains(rendered, "visible reply") {
		t.Fatalf("real user/assistant turns lost: %s", rendered)
	}
	if strings.Contains(rendered, "stale read-only plan directive") {
		t.Fatalf("plan-mode-context leaked into portable items: %s", rendered)
	}
	summary, branch, err := decodeSourceJournal(t.Context(), body.Bytes(), sessionID)
	if err != nil {
		t.Fatalf("decodeSourceJournal rejected additive v19 entries: %v", err)
	}
	if len(branch) != len(records) || len(summary.Usage) != 2 {
		t.Fatalf("unexpected branch or usage count: branch=%d usage=%+v", len(branch), summary.Usage)
	}
}
