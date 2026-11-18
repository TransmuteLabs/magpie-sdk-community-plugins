package ompsessions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func customMessageRequest(test *testing.T, target string) sdk.SessionRequest {
	test.Helper()
	journal, err := os.ReadFile("testdata/omp-custom-v3.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	image, err := os.ReadFile("testdata/image.bin")
	if err != nil {
		test.Fatal(err)
	}
	options := json.RawMessage(`{"cwd":"/work/omp"}`)
	if target == "codex" {
		options = json.RawMessage(`{"cwd":"/work/omp","provider":"offline"}`)
	}
	return sdk.SessionRequest{
		ID: "omp-to-" + target, From: "omp", To: target, Options: options,
		Files: []sdk.SessionDocument{
			{Path: "sessions/custom.jsonl", Format: "omp", Body: journal},
			{Path: "blobs/" + hashBytes(image), Format: "omp", Body: image},
		},
	}
}

func TestCustomMessageNativeExport(test *testing.T) {
	for _, target := range []string{"codex", "claude"} {
		test.Run(target, func(test *testing.T) {
			request := customMessageRequest(test, target)
			result, err := Convert(test.Context(), request)
			if err != nil {
				test.Fatal(err)
			}
			if len(result.Files) != 1 {
				test.Fatal("native export did not return one journal")
			}
			body := result.Files[0].Body
			for _, absent := range []string{"EXCLUDED_ROW", "AGENT_INJECTED_ROW", "DEVELOPER_CONTEXT_ROW", "OFF_BRANCH_ROW", "blob:sha256:"} {
				if bytes.Contains(body, []byte(absent)) {
					test.Fatalf("native export exposed %q", absent)
				}
			}
			encodedImage := base64.StdEncoding.EncodeToString(request.Files[1].Body)
			if !bytes.Contains(body, []byte(encodedImage)) {
				test.Error("native export omitted the peer's image")
			}
			if target == "codex" {
				checkCustomCodexHistory(test, body)
			} else {
				checkCustomClaudeHistory(test, body)
			}
		})
	}
}

func checkCustomCodexHistory(test *testing.T, body []byte) {
	test.Helper()
	var kinds, users []string
	turnByText := map[string]string{}
	var callID, resultID string
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte{'\n'}) {
		var record struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				TurnID  string `json:"turn_id"`
				CallID  string `json:"call_id"`
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				Item struct {
					Type    string `json:"type"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"item"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			test.Fatal(err)
		}
		switch record.Payload.Type {
		case "task_started", "task_complete", "turn_aborted":
			kinds = append(kinds, record.Payload.Type)
		case "item_completed":
			kinds = append(kinds, record.Payload.Item.Type)
			if record.Payload.Item.Type == "UserMessage" {
				turnByText[record.Payload.Item.Content[0].Text] = record.Payload.TurnID
			}
		case "message":
			if record.Payload.Role == "user" {
				users = append(users, record.Payload.Content[0].Text)
			}
		case "function_call":
			callID = record.Payload.CallID
		case "function_call_output":
			resultID = record.Payload.CallID
		}
	}
	want := []string{"task_started", "UserMessage", "CommandExecution", "UserMessage", "AgentMessage", "task_complete",
		"task_started", "UserMessage", "AgentMessage", "task_complete"}
	if !slices.Equal(kinds, want) {
		test.Errorf("Codex native turn/card order = %v, want %v", kinds, want)
	}
	if !slices.Equal(users, []string{"Start the work.", "Use the peer's correction.", "Explicit skill request."}) {
		test.Errorf("Codex model-facing user messages = %v", users)
	}
	if first := turnByText["Start the work."]; first == "" || turnByText["Use the peer's correction."] != first ||
		turnByText["Explicit skill request."] == first || turnByText["Explicit skill request."] == "" {
		test.Error("peer correction changed the active turn or the skill did not get its own turn")
	}
	if callID == "" || callID != resultID {
		test.Error("recorded tool result lost its call")
	}
}

func checkCustomClaudeHistory(test *testing.T, body []byte) {
	test.Helper()
	var messages []string
	var callID, resultID string
	for _, raw := range bytes.Split(bytes.TrimSpace(body), []byte{'\n'}) {
		var record struct {
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type      string `json:"type"`
					Text      string `json:"text"`
					ID        string `json:"id"`
					ToolUseID string `json:"tool_use_id"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			test.Fatal(err)
		}
		for _, block := range record.Message.Content {
			switch block.Type {
			case "text":
				messages = append(messages, record.Message.Role+": "+block.Text)
			case "tool_use":
				callID = block.ID
				messages = append(messages, "call")
			case "tool_result":
				resultID = block.ToolUseID
				messages = append(messages, "result")
			case "image":
				messages = append(messages, "image")
			}
		}
	}
	want := []string{"user: Start the work.", "call", "result", "user: Use the peer's correction.", "image",
		"assistant: Finished with the correction.", "user: Explicit skill request.", "assistant: Finished the requested skill."}
	if !slices.Equal(messages, want) || callID == "" || callID != resultID {
		test.Errorf("Claude message order or tool pairing changed: %v", messages)
	}
}

func TestCustomMessageNativeRefusesAmbiguousRows(test *testing.T) {
	for _, scenario := range []struct {
		name, original, replacement, errorText string
	}{
		{"null-exclusion", `"excludeFromContext":false`, `"excludeFromContext":null`, "excludeFromContext"},
		{"string-exclusion", `"excludeFromContext":false`, `"excludeFromContext":"false"`, "excludeFromContext"},
		{"duplicate-exclusion", `"excludeFromContext":false`, `"excludeFromContext":true,"excludeFromContext":false`, "excludeFromContext"},
		{"case-variant-exclusion", `"excludeFromContext":false`, `"ExcludeFromContext":true,"excludeFromContext":false`, "excludeFromContext"},
		{"duplicate-attribution", `"attribution":"user"`, `"attribution":"agent","attribution":"user"`, "attribution"},
		{"unsupported-block", `"type":"text","text":"Use the peer's correction."`, `"type":"audio","data":"unsupported"`, "block"},
		{"duplicate-block-type", `"type":"text","text":"Use the peer's correction."`, `"type":"audio","type":"text","text":"Use the peer's correction."`, "type"},
		{"case-variant-block-type", `"type":"text","text":"Use the peer's correction."`, `"type":"audio","TYPE":"text","text":"Use the peer's correction."`, "type"},
		{"duplicate-block-text", `"text":"Use the peer's correction."`, `"text":"different","text":"Use the peer's correction."`, "text"},
		{"case-variant-image-data", `"data":"blob:sha256:`, `"Data":"AA==","data":"blob:sha256:`, "data"},
		{"null-content", `"content":"Explicit skill request."`, `"content":null`, "content"},
		{"null-text", `"text":"Use the peer's correction."`, `"text":null`, "text"},
	} {
		for _, target := range []string{"codex", "claude"} {
			test.Run(scenario.name+"/"+target, func(test *testing.T) {
				request := customMessageRequest(test, target)
				journal := request.Files[0].Body
				mutated := bytes.Replace(journal, []byte(scenario.original), []byte(scenario.replacement), 1)
				if bytes.Equal(journal, mutated) {
					test.Fatal("fixture mutation made no change")
				}
				request.Files[0].Body = mutated
				result, err := Convert(test.Context(), request)
				if err == nil || !strings.Contains(err.Error(), scenario.errorText) || len(result.Files) != 0 {
					test.Fatalf("expected closed refusal for %s, got err=%v and %d output files", scenario.name, err, len(result.Files))
				}
			})
		}
	}
}

func TestCustomMessagePeerAfterCompletedTurnStartsItsOwnTurn(test *testing.T) {
	request := customMessageRequest(test, "codex")
	original := []byte(`"customType":"skill-prompt","content":"Explicit skill request."`)
	replacement := []byte(`"customType":"collab-prompt","content":"Explicit skill request."`)
	journal := bytes.Replace(request.Files[0].Body, original, replacement, 1)
	if bytes.Equal(journal, request.Files[0].Body) {
		test.Fatal("fixture lacks the completed-turn boundary")
	}
	request.Files[0].Body = journal
	result, err := Convert(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	checkCustomCodexHistory(test, result.Files[0].Body)
}

func TestCustomMessageClaudeRefusesSteeringBeforeToolResults(test *testing.T) {
	request := customMessageRequest(test, "claude")
	journal := bytes.Replace(request.Files[0].Body, []byte(`"id":"result","parentId":"call"`),
		[]byte(`"id":"result","parentId":"peer"`), 1)
	journal = bytes.Replace(journal, []byte(`"id":"peer","parentId":"result"`),
		[]byte(`"id":"peer","parentId":"call"`), 1)
	request.Files[0].Body = bytes.Replace(journal, []byte(`"id":"excluded","parentId":"peer"`),
		[]byte(`"id":"excluded","parentId":"result"`), 1)
	result, err := Convert(test.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "steering before pending tool results") || len(result.Files) != 0 {
		test.Fatalf("expected refusal at the Claude tool boundary, got err=%v and %d output files", err, len(result.Files))
	}
}

func TestCustomMessageSourceInputsAndArchive(test *testing.T) {
	request := customMessageRequest(test, "codex")
	planRequest := request
	planRequest.Files = request.Files[:1]
	plan, err := PlanSourceInputs(test.Context(), planRequest)
	if err != nil || len(plan.Files) != 1 || plan.Files[0].Path != request.Files[1].Path {
		test.Errorf("peer image was not planned for host acquisition: %+v, %v", plan, err)
	}
	document, err := Decode(test.Context(), request.Files[0].Body, []Asset{{
		SHA256: hashBytes(request.Files[1].Body), Body: request.Files[1].Body,
	}})
	if err != nil {
		test.Fatal(err)
	}
	if !bytes.Equal(document.Archive.Journal, request.Files[0].Body) ||
		bytes.Contains(mustJSON(test, document.Items), []byte("Explicit skill request.")) {
		test.Fatal("the exact journal or the earlier interchange projection changed")
	}
	if _, err := Convert(test.Context(), planRequest); err == nil || !strings.Contains(err.Error(), "unresolved omp image blob") {
		test.Fatalf("expected missing image refusal, got %v", err)
	}
}

func TestCustomMessageIgnoredBlobsDoNotBlockNativeExport(test *testing.T) {
	ignoredImage := []byte("archive-only image")
	ignoredHash := hashBytes(ignoredImage)
	rawImage := `[{"type":"image","mimeType":"image/png","data":"blob:sha256:` + ignoredHash + `"}]`
	for _, scenario := range []struct {
		name, original string
	}{
		{"excluded", `"EXCLUDED_ROW"`},
		{"agent-authored", `"AGENT_INJECTED_ROW"`},
		{"generic-extension", `"DEVELOPER_CONTEXT_ROW"`},
		{"off-branch", `{"invalid":"OFF_BRANCH_ROW"}`},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			for _, target := range []string{"codex", "claude"} {
				request := customMessageRequest(test, target)
				original := request.Files[0].Body
				request.Files[0].Body = bytes.Replace(original, []byte(scenario.original), []byte(rawImage), 1)
				if bytes.Equal(original, request.Files[0].Body) {
					test.Fatal("fixture did not put the image on the ignored row")
				}
				planRequest := request
				planRequest.Files = request.Files[:1]
				plan, err := PlanSourceInputs(test.Context(), planRequest)
				want := []SourceInput{{Path: request.Files[1].Path, SHA256: hashBytes(request.Files[1].Body)}}
				if err != nil || !slices.Equal(plan.Files, want) {
					test.Errorf("%s tried to read the archive-only image: %+v %v", target, plan, err)
				}
				result, err := Convert(test.Context(), request)
				if err != nil || len(result.Files) != 1 {
					test.Errorf("%s export failed without the archive-only image: %v", target, err)
				}
				archiveRequest := planRequest
				archiveRequest.ID, archiveRequest.To = "omp-to-document", DocumentFormat
				archivePlan, err := PlanSourceInputs(test.Context(), archiveRequest)
				ignored := SourceInput{Path: "blobs/" + ignoredHash, SHA256: ignoredHash}
				if err != nil || !slices.Contains(archivePlan.Files, ignored) {
					test.Error("archive planner omitted the original custom image")
				}
				body := request.Files[0].Body
				active := Asset{SHA256: want[0].SHA256, Body: request.Files[1].Body}
				if _, err := Decode(test.Context(), body, []Asset{active}); err == nil {
					test.Error("archive silently lost the referenced image")
				}
				document, err := Decode(test.Context(), body, []Asset{active, {SHA256: ignoredHash, Body: ignoredImage}})
				if err != nil || !bytes.Equal(body, document.Archive.Journal) || len(document.Archive.Assets) != 2 {
					test.Fatalf("complete archive changed original data: %v", err)
				}
			}
		})
	}
}
