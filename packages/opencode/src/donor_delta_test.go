package opencode

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDonorDeltaPreservesDeliveredSubagentText(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		state string
		body  string
	}{
		{"legacy-report-with-marker", "completed", "Example from the docs:\n\nBackground work still running:\n- shell (sh_1): bun test"},
		{"outstanding-snapshot", "running", "Started tests\n\nBackground work still running:\n- shell (sh_1): bun test"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			text := `<subagent sessionID="ses_child" state="` + scenario.state + `" description="Outbox inspection">` +
				"\n" + scenario.body + "\n</subagent>"
			message := historyMessage{kind: "synthetic", data: map[string]json.RawMessage{
				"text": historyJSON(text),
				"metadata": historyJSON(map[string]any{
					"source": "subagent", "childID": "ses_child", "agent": "explore", "state": "completed",
				}),
			}}
			projection := historyProjection{request: sdk.StorageRequest{
				Lease: sdk.StorageReadLease{Limits: sdk.DefaultStorageLimits()},
			}}
			if err := projection.message(t.Context(), nil, message); err != nil {
				t.Fatal(err)
			}
			if len(projection.result.Parts) != 1 || projection.result.Parts[0].Text != text ||
				projection.result.Parts[0].Kind != "context" || projection.result.Cut {
				t.Fatal("delivered report was reinterpreted or reformatted", projection.result)
			}
			recent, err := historyRecentText(message)
			if err != nil || recent != "[Synthetic context]: "+text {
				t.Fatal("local compaction changed the delivered report", recent, err)
			}
		})
	}
}

func TestDonorDeltaPreservesUTF8ShellTranscript(t *testing.T) {
	for _, character := range []string{"é", "€", "😀"} {
		t.Run(character, func(t *testing.T) {
			output := strings.Repeat("a", 65535) + character + "\uFEFFz"
			text := "pages\n" + output
			message := historyMessage{kind: "shell", data: map[string]json.RawMessage{
				"command": historyJSON("pages"),
				"output":  historyJSON(map[string]string{"output": output}),
			}}
			for _, limit := range []int{len("pages\n") + 65536, len(text)} {
				limits := sdk.DefaultStorageLimits()
				limits.MaxTextBytes = limit
				projection := historyProjection{request: sdk.StorageRequest{Lease: sdk.StorageReadLease{Limits: limits}}}
				if err := projection.message(t.Context(), nil, message); err != nil {
					t.Fatal(err)
				}
				if len(projection.result.Parts) != 1 {
					t.Fatal("shell output disappeared", projection.result)
				}
				part := projection.result.Parts[0]
				expected := text
				if limit < len(text) {
					expected = "pages\n" + strings.Repeat("a", 65535)
				}
				if part.Text != expected || !utf8.ValidString(part.Text) ||
					part.Cut != len(text)-len(expected) || projection.result.Cut != (limit < len(text)) {
					t.Fatal("stored shell text split a character or lost bytes", len(part.Text), part.Cut, projection.result.Cut)
				}
			}
		})
	}
}
