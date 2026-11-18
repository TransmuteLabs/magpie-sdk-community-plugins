package ompsessions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestOMPToClaudeProducesNativeHistoryWithAttachments(t *testing.T) {
	request := sdkFixture(t, "omp")
	request.ID, request.To, request.Options = "omp-to-claude", "claude", json.RawMessage(`{"cwd":"/work/omp"}`)
	result, err := Convert(t.Context(), request)
	if err != nil {
		t.Fatal("native OMP-to-Claude conversion unavailable:", err)
	}
	if !validUUID(result.ID) || len(result.Files) != 1 ||
		result.Files[0].Path != "-work-omp/"+result.ID+".jsonl" || result.Files[0].Format != "claude" {
		t.Fatalf("not a native Claude journal: %+v", result)
	}
	var lines []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(result.Files[0].Body), []byte{'\n'}) {
		var line map[string]any
		if err := json.Unmarshal(raw, &line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	var previous any
	images, calls, results := 0, 0, 0
	pending := map[string]bool{}
	assistantID := ""
	encodedImage := base64.StdEncoding.EncodeToString(request.Files[1].Body)
	for _, line := range lines {
		if line["sessionId"] != result.ID || line["cwd"] != "/work/omp" || line["parentUuid"] != previous {
			t.Fatalf("broken native parent/session chain: %+v", line)
		}
		previous = line["uuid"]
		message := line["message"].(map[string]any)
		if message["role"] == "assistant" {
			id := message["id"].(string)
			if assistantID != id && len(pending) != 0 {
				t.Fatal("native assistant group changed before its tool results")
			}
			assistantID = id
		}
		for _, value := range message["content"].([]any) {
			block := value.(map[string]any)
			switch block["type"] {
			case "image":
				if block["source"].(map[string]any)["data"] != encodedImage {
					t.Fatal("user image bytes changed")
				}
				images++
			case "tool_use":
				if block["name"] != "Bash" {
					t.Fatal("foreign tool name exposed as a native executable")
				}
				calls++
				pending[block["id"].(string)] = true
			case "tool_result":
				id := block["tool_use_id"].(string)
				if !pending[id] {
					t.Fatal("native tool result does not belong to its assistant group")
				}
				delete(pending, id)
				results++
				if blocks, ok := block["content"].([]any); ok {
					for _, value := range blocks {
						image := value.(map[string]any)
						if image["type"] == "image" {
							if image["source"].(map[string]any)["data"] != encodedImage {
								t.Fatal("tool result image bytes changed")
							}
							images++
						}
					}
				}
			}
		}
	}
	if images != 2 || calls != 2 || results != 2 || len(pending) != 0 {
		t.Fatalf("native image/tool history incomplete: images=%d calls=%d results=%d", images, calls, results)
	}
	if bytes.Contains(result.Files[0].Body, []byte("private reasoning")) ||
		bytes.Contains(result.Files[0].Body, []byte("abandoned sibling")) ||
		!bytes.Contains(result.Files[0].Body, []byte("file contents")) ||
		!bytes.Contains(result.Files[0].Body, []byte("is_error\":true")) {
		t.Fatal("native context contains private/off-branch history or lost selected tool results")
	}
}

func TestOMPToClaudeRequiresSelectedCwdAndBoundedProjectKey(t *testing.T) {
	for _, cwd := range []string{"", "relative/path", "/" + strings.Repeat("a", 201)} {
		request := sdkFixture(t, "omp")
		request.ID, request.To = "omp-to-claude", "claude"
		request.Options, _ = json.Marshal(map[string]string{"cwd": cwd})
		if _, err := Convert(t.Context(), request); err == nil {
			t.Fatalf("unsafe native target cwd accepted: %q", cwd)
		}
	}
}

func TestOMPToClaudeDoesNotRequireInterchangeExpansion(t *testing.T) {
	request := sdkFixture(t, "omp")
	text := bytes.Repeat([]byte("x"), 7<<20)
	request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body, []byte("human request"), text)
	if _, err := Convert(t.Context(), request); err == nil {
		t.Fatal("fixture did not exceed the interchange document expansion budget")
	}
	request.ID, request.To, request.Options = "omp-to-claude", "claude", json.RawMessage(`{"cwd":"/work/omp"}`)
	result, err := Convert(t.Context(), request)
	if err != nil {
		t.Fatal("bounded native input was incorrectly forced through an oversized interchange document:", err)
	}
	if len(result.Files) != 1 || !bytes.Contains(result.Files[0].Body, text) {
		t.Fatal("large native input lost its user text")
	}
}

func TestOMPToClaudeRejectsUnrepresentableAssistantImage(t *testing.T) {
	request := sdkFixture(t, "omp")
	request.ID, request.To, request.Options = "omp-to-claude", "claude", json.RawMessage(`{"cwd":"/work/omp"}`)
	request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body,
		[]byte(`{"type":"text","text":"Checking."}`),
		[]byte(`{"type":"image","mimeType":"image/png","data":"`+base64.StdEncoding.EncodeToString(request.Files[1].Body)+`"}`))
	if _, err := Convert(t.Context(), request); err == nil {
		t.Fatal("assistant image was silently discarded by native migration")
	}
}
