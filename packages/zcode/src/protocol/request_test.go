package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestReasoningFromNativeFixtures(t *testing.T) {
	for _, sample := range []struct {
		body string
		want string
	}{
		{`{"model":"GLM-5.2","thinking":{"type":"disabled","extra":"kept"},"output_config":{"effort":"none","format":"keep"},"messages":[]}`, `{"thinking":{"type":"disabled","extra":"kept"},"output_config":{"format":"keep"}}`},
		{`{"model":"GLM-5.3","thinking":{"type":"disabled"},"messages":[]}`, `{"thinking":{"type":"enabled"},"output_config":{"effort":"low"}}`},
		{`{"model":"GLM-5.3","thinking":{"type":"adaptive","budget_tokens":8192,"display":"summarized"},"messages":[]}`, `{"thinking":{"type":"enabled"},"output_config":{"effort":"high"}}`},
		{`{"model":"GLM-5.2","output_config":{"effort":"low"},"messages":[]}`, `{"thinking":{"type":"enabled"},"output_config":{"effort":"high"}}`},
		{`{"model":"GLM-5.3","output_config":{"effort":"xhigh"},"messages":[]}`, `{"thinking":{"type":"enabled"},"output_config":{"effort":"max"}}`},
	} {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(sample.body), &raw)
		var model string
		json.Unmarshal(raw["model"], &model)
		result, _, err := Prepare([]byte(sample.body), model)
		if err != nil {
			t.Fatal(err)
		}
		var actual, expected map[string]json.RawMessage
		json.Unmarshal(result, &actual)
		json.Unmarshal([]byte(sample.want), &expected)
		for key, value := range expected {
			var got, want any
			json.Unmarshal(actual[key], &got)
			json.Unmarshal(value, &want)
			if !equalJSON(got, want) {
				t.Errorf("%s: %s = %s, want %s", model, key, actual[key], value)
			}
		}
	}
}

func equalJSON(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return bytes.Equal(leftJSON, rightJSON)
}

func TestSearchToolRoundtripAndOpaqueArguments(t *testing.T) {
	body := []byte(`{"model":"GLM-5.3","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"x","name":"ToolSearch","input":{"name":"ToolSearch","integer":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"ToolSearchTool is plain text"}]}],"tools":[{"name":"ToolSearch","input_schema":{}},{"name":"Read","defer_loading":true,"input_schema":{}},{"name":"DeferredToolPlaceholder","input_schema":{}}],"tool_choice":{"type":"tool","name":"ToolSearch"}}`)
	got, renamed, err := Prepare(body, "GLM-5.3")
	if err != nil || !renamed {
		t.Fatalf("prepare: %v, %v", renamed, err)
	}
	if !bytes.Contains(got, []byte(`9007199254740993`)) || !bytes.Contains(got, []byte(`"name":"ToolSearch"`)) ||
		bytes.Contains(got, []byte(`defer_loading`)) || bytes.Contains(got, []byte(`DeferredToolPlaceholder`)) {
		t.Fatalf("opaque arguments or tool schema changed: %s", got)
	}
	restored, err := Restore([]byte(`{"type":"message","content":[{"type":"tool_use","name":"ToolSearchTool","input":{"name":"ToolSearchTool"}}]}`), true)
	if err != nil || !bytes.Contains(restored, []byte(`"name":"ToolSearch","type":"tool_use"`)) ||
		!bytes.Contains(restored, []byte(`"input":{"name":"ToolSearchTool"}`)) {
		t.Fatalf("restore: %s, %v", restored, err)
	}
}

func TestAliasCollisionRefusedRatherThanHidingTools(t *testing.T) {
	_, _, err := Prepare([]byte(`{"model":"GLM-5.3","messages":[],"tools":[{"name":"ToolSearch"},{"name":"ToolSearchTool"}]}`), "GLM-5.3")
	if err == nil {
		t.Fatal("ToolSearch alias collision accepted")
	}
}

func TestMalformedAndMismatchedBodiesFail(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{"model":"other","messages":[]}`, `{"model":"GLM-5.3","messages":{}}`,
		`{"model":"GLM-5.3","messages":[],"thinking":[]}`, `{"model":"GLM-5.3","messages":[],"output_config":false}`,
		`{"model":"GLM-5.3","messages":[]} {}`, `{"model":"GLM-5.3","model":"GLM-5.2","messages":[]}`,
	} {
		if _, _, err := Prepare([]byte(body), "GLM-5.3"); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestOpaqueHistorySignaturesAndUnknownNativeFieldsArePreserved(t *testing.T) {
	body := []byte(`{"model":"GLM-5.3","system":[{"type":"text","text":"fixture system","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"fixture","signature":"synthetic-signature"},{"type":"redacted_thinking","data":"synthetic-sealed-block"},{"type":"tool_use","id":"call","name":"Read","input":{"counter":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":[{"type":"text","text":"fixture"}]},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"c3ludGhldGlj"}}]}],"native_future":{"version":17,"value":"preserve"}}`)
	prepared, renamed, err := Prepare(body, "GLM-5.3")
	if err != nil || renamed {
		t.Fatalf("prepare: renamed=%v err=%v", renamed, err)
	}
	before, _ := ReadObject(body)
	after, _ := ReadObject(prepared)
	for _, key := range []string{"system", "messages", "native_future"} {
		if !equalJSON(before[key], after[key]) {
			t.Fatalf("native history field %s changed", key)
		}
	}
}
