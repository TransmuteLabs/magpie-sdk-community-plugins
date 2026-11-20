package commandcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestProviderExecuteGeminiNullableToolSchema(t *testing.T) {
	const original = `{
		"type":"object",
		"required":["maybe","items"],
		"properties":{
			"maybe":{"type":["string","null"],"default":null,"description":"keep"},
			"items":{"type":"array","items":{"type":["null","integer"],"minimum":1}},
			"variants":{"oneOf":[{"type":["boolean","null"],"enum":[true,null],"default":true},{"type":"string"}]},
			"declared":{"$ref":"#/$defs/Value"},
			"ordinary":{"type":"string","default":null},
			"union":{"type":["string","integer","null"]},
			"duplicate":{"type":["number","null","null"]},
			"malformed":{"type":["null",false]},
			"unknown":{"type":["unknown","null"]},
			"conflict":{"type":["string","null"],"nullable":false},
			"broken":{"not":[{"type":["string","null"]}],"oneOf":{"type":["string","null"]}}
		},
		"$defs":{"Value":{"type":["number","null"],"nullable":true}},
		"examples":[{"type":["string","null"],"default":null}],
		"x-data":{"type":["string","null"],"default":null}
	}`
	const normalized = `{
		"type":"object",
		"required":["maybe","items"],
		"properties":{
			"maybe":{"type":"string","nullable":true,"description":"keep"},
			"items":{"type":"array","items":{"type":"integer","nullable":true,"minimum":1}},
			"variants":{"oneOf":[{"type":"boolean","nullable":true,"enum":[true,null],"default":true},{"type":"string"}]},
			"declared":{"$ref":"#/$defs/Value"},
			"ordinary":{"type":"string","default":null},
			"union":{"type":["string","integer","null"]},
			"duplicate":{"type":["number","null","null"]},
			"malformed":{"type":["null",false]},
			"unknown":{"type":["unknown","null"]},
			"conflict":{"type":["string","null"],"nullable":false},
			"broken":{"not":[{"type":["string","null"]}],"oneOf":{"type":["string","null"]}}
		},
		"$defs":{"Value":{"type":"number","nullable":true}},
		"examples":[{"type":["string","null"],"default":null}],
		"x-data":{"type":["string","null"],"default":null}
	}`
	for _, sample := range []struct {
		name, plan, model, want string
	}{
		{"native-gemini", "go", "google/gemini-3-flash-preview", normalized},
		{"native-other-google", "go", "google/other", original},
		{"native-other-provider", "go", "anthropic/claude-sonnet-4-6", original},
		{"provider-gemini", "pro", "google/gemini-3-flash-preview", original},
	} {
		t.Run(sample.name, func(t *testing.T) {
			var want map[string]any
			if err := json.Unmarshal([]byte(sample.want), &want); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/alpha/billing/subscriptions" {
					fmt.Fprintf(writer, `{"data":{"planId":"individual-%s","status":"active"}}`, sample.plan)
					return
				}
				calls.Add(1)
				type tool struct {
					Schema   map[string]any `json:"input_schema"`
					Function struct {
						Parameters map[string]any `json:"parameters"`
					} `json:"function"`
				}
				var envelope struct {
					Tools  []tool `json:"tools"`
					Params struct {
						Tools []tool `json:"tools"`
					} `json:"params"`
				}
				if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
					t.Error(err)
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				endpoint := "/provider/v1/chat/completions"
				if sample.plan == "go" {
					endpoint = "/alpha/generate"
				}
				var actual map[string]any
				switch {
				case request.URL.Path == endpoint && sample.plan == "go" && len(envelope.Params.Tools) == 1:
					actual = envelope.Params.Tools[0].Schema
				case request.URL.Path == endpoint && sample.plan == "pro" && len(envelope.Tools) == 1:
					actual = envelope.Tools[0].Function.Parameters
				}
				if !reflect.DeepEqual(actual, want) {
					t.Errorf("unexpected %s tool schema: %v", request.URL.Path, actual)
					writer.WriteHeader(http.StatusBadRequest)
					io.WriteString(writer, `{"error":{"message":"invalid tool schema"}}`)
					return
				}
				if sample.plan == "go" {
					writer.Header().Set("Content-Type", "application/x-ndjson")
					io.WriteString(writer, "{\"type\":\"text-delta\",\"text\":\"ok\"}\n")
					io.WriteString(writer, "{\"type\":\"finish\",\"finishReason\":\"stop\",\"totalUsage\":{\"inputTokens\":5,\"outputTokens\":1}}\n")
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				io.WriteString(writer, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			worker := fixture(t, Config{APIURL: server.URL, WorkingDirectory: "/fixture"})
			raw := fmt.Appendf(nil, `{"model":%q,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"inspect","parameters":%s}}]}`, sample.model, original)
			body, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: sample.model, Body: raw}, testAuth())
			var result sdk.ExecuteResult
			status, _ := eventsBody(t, events)
			if err != nil || json.Unmarshal(body, &result) != nil || result.Outcome != "completed" || status != http.StatusOK || calls.Load() != 1 {
				t.Fatalf("expected one completed request: %s status=%d calls=%d error=%v", body, status, calls.Load(), err)
			}
		})
	}
}
