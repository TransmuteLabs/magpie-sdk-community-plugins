package ompsessions

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func capturedCodexFixture(test *testing.T) sdk.SessionRequest {
	test.Helper()
	parent, err := os.ReadFile("testdata/codex-sparse-ancestor.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	child, err := os.ReadFile("testdata/codex-sparse-child.jsonl")
	if err != nil {
		test.Fatal(err)
	}
	provenance := func(id string, raw, prefix []byte, first, last string) sdk.SessionInputProvenance {
		return sdk.SessionInputProvenance{
			Path:      "sessions/2026/11/15/rollout-2026-11-15T00-00-00-" + id + ".jsonl",
			RolloutID: id, Encoding: "jsonl", RawBytes: strconv.Itoa(len(raw)), RawSHA256: hashBytes(raw),
			DecodedBytes: strconv.Itoa(len(raw)), DecodedSHA256: hashBytes(raw), PrefixSHA256: hashBytes(prefix),
			ByteStart: "0", ByteEndExclusive: strconv.Itoa(len(prefix)), OrdinalStart: first, OrdinalEndExclusive: last,
		}
	}
	return sdk.SessionRequest{ID: "codex-to-omp", From: "codex", To: "omp",
		Options: json.RawMessage(`{"cwd":"/work/fixture","canonicalCwd":"/work/fixture","home":"/home/fixture","temp":"/tmp"}`),
		Files:   []sdk.SessionDocument{{Path: "selected.jsonl", Format: "codex", Body: child}},
		Related: &sdk.SessionRelatedInputSet{Version: 1, Kind: sdk.CodexRolloutLineage,
			Selected: provenance("22222222-2222-4222-8222-222222222222", child, child, "3", "5"),
			Ancestors: []sdk.SessionRelatedInput{{Body: bytes.Clone(parent[:756]),
				Source: provenance("11111111-1111-4111-8111-111111111111", parent, parent[:756], "0", "3")}}}}
}

func TestCodexLineageModuleFromCapturedFixture(test *testing.T) {
	request := capturedCodexFixture(test)
	before, _ := json.Marshal(request)
	result, err := Convert(test.Context(), request)
	if err != nil {
		test.Fatal("captured official-schema sparse fixture did not convert through the OMP module", err)
	}
	document, err := Decode(test.Context(), result.Files[0].Body, nil)
	if err != nil {
		test.Fatal(err)
	}
	var messages []string
	for _, item := range document.Items {
		if item.Text != "" {
			messages = append(messages, item.Text)
		}
		for _, content := range item.Content {
			if content.Type == "text" {
				messages = append(messages, content.Text)
			}
		}
	}
	if strings.Join(messages, "|") != "Inherited request|Inherited answer|Only the child suffix" {
		test.Fatal("lineage lost inherited context, included the excluded suffix or invented a turn", messages)
	}
	after, _ := json.Marshal(request)
	if !bytes.Equal(before, after) {
		test.Fatal("module serialization changed an original capture or its provenance")
	}
	request.Related = nil
	if _, err := Convert(test.Context(), request); err == nil ||
		!strings.Contains(err.Error(), "requires captured ancestor history") {
		test.Fatal("uncaptured sparse history did not retain its explicit refusal", err)
	}
}

func TestCodexLineageModuleKeepsSelectedMetadata(test *testing.T) {
	request := capturedCodexFixture(test)
	body, err := materializeCodexLineage(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	var first struct {
		Ordinal uint64                     `json:"ordinal"`
		Payload map[string]json.RawMessage `json:"payload"`
	}
	header, _, _ := bytes.Cut(body, []byte{'\n'})
	if json.Unmarshal(header, &first) != nil || first.Ordinal != 0 ||
		string(first.Payload["id"]) != `"22222222-2222-4222-8222-222222222222"` ||
		string(first.Payload["cwd"]) != `"/work/fixture"` ||
		string(first.Payload["session_id"]) != `"11111111-1111-4111-8111-111111111111"` ||
		first.Payload["history_base"] != nil || first.Payload["subagent_history_start_ordinal"] != nil {
		test.Fatal("module normalization lost selected-child metadata or retained a sparse pointer")
	}
	for index, line := range bytes.Split(bytes.TrimSpace(body), []byte{'\n'}) {
		var record struct {
			Type    string `json:"type"`
			Ordinal uint64 `json:"ordinal"`
		}
		if json.Unmarshal(line, &record) != nil || record.Ordinal != uint64(index) ||
			index != 0 && record.Type == "session_meta" {
			test.Fatal("module normalization did not preserve one selected header and contiguous wrapper ordinals")
		}
	}
}

func TestCodexLineageModuleRejectsForgedRangesAndBodies(test *testing.T) {
	for _, problem := range []string{"digest", "range", "parent-id", "selected-id", "ordinal", "missing-oldest",
		"empty-base", "late-header", "partial", "reordered", "other-route"} {
		test.Run(problem, func(test *testing.T) {
			request := capturedCodexFixture(test)
			ancestor := &request.Related.Ancestors[0]
			switch problem {
			case "digest":
				ancestor.Body[0] = 'x'
			case "range":
				ancestor.Source.OrdinalEndExclusive = "4"
			case "parent-id":
				request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body,
					[]byte(`"thread_id":"11111111-1111-4111-8111-111111111111"`),
					[]byte(`"thread_id":"33333333-3333-4333-8333-333333333333"`))
			case "selected-id":
				request.Files[0].Body = bytes.ReplaceAll(request.Files[0].Body,
					[]byte(`"id":"22222222-2222-4222-8222-222222222222"`),
					[]byte(`"id":"33333333-3333-4333-8333-333333333333"`))
			case "ordinal":
				ancestor.Body = bytes.ReplaceAll(ancestor.Body, []byte(`"ordinal":1`), []byte(`"ordinal":9`))
			case "missing-oldest":
				ancestor.Body = bytes.Replace(ancestor.Body, []byte(`"history_mode":"paginated"`),
					[]byte(`"history_mode":"paginated","history_base":{"thread_id":"33333333-3333-4333-8333-333333333333","end_ordinal_exclusive":1,"end_byte_offset":1}`), 1)
				ancestor.Source.OrdinalStart = "1"
			case "empty-base":
				request.Files[0].Body = bytes.Replace(request.Files[0].Body,
					[]byte(`"history_base":{"thread_id":"11111111-1111-4111-8111-111111111111","end_ordinal_exclusive":3,"end_byte_offset":756}`),
					[]byte(`"history_base":{}`), 1)
			case "late-header":
				ancestor.Body = bytes.Replace(ancestor.Body, []byte(`"type":"response_item"`), []byte(`"type":"session_meta"`), 1)
			case "partial":
				ancestor.Body = ancestor.Body[:len(ancestor.Body)-1]
			case "reordered":
				request.Related.Selected = ancestor.Source
				request.Related.Ancestors[0] = sdk.SessionRelatedInput{Body: request.Files[0].Body,
					Source: capturedCodexFixture(test).Related.Selected}
				request.Files[0].Body = ancestor.Body
			case "other-route":
				request.ID = "claude-to-omp"
			}
			if problem != "digest" && problem != "reordered" {
				refreshCodexCapture(&ancestor.Source, ancestor.Body)
				refreshCodexCapture(&request.Related.Selected, request.Files[0].Body)
			}
			if _, err := Convert(test.Context(), request); err == nil {
				test.Fatal("forged related history reached native OMP serialization", problem)
			}
		})
	}
}

func refreshCodexCapture(source *sdk.SessionInputProvenance, body []byte) {
	source.RawBytes, source.DecodedBytes, source.ByteEndExclusive = strconv.Itoa(len(body)), strconv.Itoa(len(body)), strconv.Itoa(len(body))
	source.RawSHA256, source.DecodedSHA256, source.PrefixSHA256 = hashBytes(body), hashBytes(body), hashBytes(body)
}

func TestCodexLineageModulePairsAcrossCapturedBoundary(test *testing.T) {
	for _, completed := range []bool{false, true} {
		test.Run(strconv.FormatBool(completed), func(test *testing.T) {
			request := capturedCodexFixture(test)
			parentHeader, _, _ := bytes.Cut(request.Related.Ancestors[0].Body, []byte{'\n'})
			parent := append(bytes.Clone(parentHeader), '\n')
			parent = append(parent, []byte(`{"ordinal":1,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Selected user"},{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}}`+"\n")...)
			parent = append(parent, []byte(`{"ordinal":2,"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"inherited-call","arguments":"{\"cmd\":\"printf lineage\",\"workdir\":\"/work/fixture\"}"}}`+"\n")...)
			first, _, _ := bytes.Cut(request.Files[0].Body, []byte{'\n'})
			var header map[string]any
			if err := json.Unmarshal(first, &header); err != nil {
				test.Fatal(err)
			}
			header["payload"].(map[string]any)["history_base"].(map[string]any)["end_byte_offset"] = len(parent)
			first, _ = json.Marshal(header)
			child := append(first, '\n')
			last := uint64(4)
			if completed {
				child = append(child, []byte(`{"ordinal":4,"type":"response_item","payload":{"type":"function_call_output","call_id":"inherited-call","output":[{"type":"text","text":"Captured result"},{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}}`+"\n")...)
				last++
			}
			record, _ := json.Marshal(map[string]any{"ordinal": last, "type": "response_item",
				"payload": map[string]any{"type": "message", "role": "assistant", "phase": "final_answer",
					"content": []map[string]any{{"type": "output_text", "text": "Selected completion"}}}})
			child = append(child, record...)
			child = append(child, '\n')
			request.Related.Ancestors[0].Body = parent
			refreshCodexCapture(&request.Related.Ancestors[0].Source, parent)
			request.Files[0].Body = child
			refreshCodexCapture(&request.Related.Selected, child)
			request.Related.Selected.OrdinalEndExclusive = strconv.FormatUint(last+1, 10)
			result, err := Convert(test.Context(), request)
			if err != nil {
				test.Fatal("captured tool boundary did not convert", err)
			}
			document, err := Decode(test.Context(), result.Files[0].Body, nil)
			if err != nil {
				test.Fatal(err)
			}
			counts, images := map[string]int{}, 0
			for _, item := range document.Items {
				counts[item.Kind]++
				for _, content := range item.Content {
					if content.Type == "image" {
						images++
					}
				}
			}
			resultImages := 0
			for _, line := range bytes.Split(bytes.TrimSpace(result.Files[0].Body), []byte{'\n'}) {
				var record struct {
					Message struct {
						Role       string    `json:"role"`
						ToolCallID string    `json:"toolCallId"`
						Content    []Content `json:"content"`
					} `json:"message"`
				}
				if err := json.Unmarshal(line, &record); err != nil {
					test.Fatal(err)
				}
				if record.Message.Role == "toolResult" {
					if record.Message.ToolCallID != "inherited-call" {
						test.Fatal("captured result changed its inherited native call identity")
					}
					for _, content := range record.Message.Content {
						if content.Type == "image" && content.Data == "AQID" && content.MimeType == "image/png" {
							resultImages++
						}
					}
				}
			}
			if counts["user"] != 1 || counts["assistant"] == 0 {
				test.Fatal("lineage boundary fabricated user turns or lost the selected completion", counts)
			}
			if completed && (counts["call"] != 1 || counts["result"] != 1 || images != 1 || resultImages != 1) {
				test.Fatal("inherited call was not paired with its selected-child result and images", counts, images, resultImages)
			}
			if !completed && (counts["call"] != 0 || counts["result"] != 0 || images != 1 ||
				resultImages != 0 ||
				!bytes.Contains(result.Files[0].Body, []byte("no later output was recorded"))) {
				test.Fatal("incomplete inherited call fabricated a tool result", counts, images)
			}
		})
	}
}
