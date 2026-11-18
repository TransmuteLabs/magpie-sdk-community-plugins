package commandcode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const triggerSchema = `{"type":"object","properties":{"func":{"type":"object","properties":{"triggers":{"type":"array","items":{"type":"object","properties":{"on":{"type":"string","enum":["push","pull"]}},"required":["on"]}}}},"extra":{"type":"array","items":{"$ref":"#/properties/func/properties/triggers/items","description":"another trigger"}}},"required":["func"]}`

func schemaRequest(model, schema string) []byte {
	return []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"schedule"}],"tools":[{"type":"function","function":{"name":"schedule","parameters":%s}}]}`, model, schema))
}

func TestGoogleSchemaPreservesNestedReferencesAndData(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, model := range []string{"google/gemini-3.8-flash", "gemini-3.8-flash", "anthropic/claude-sonnet-4-6"} {
			t.Run(fmt.Sprintf("%s/native=%t", model, native), func(t *testing.T) {
				raw := schemaRequest(model, triggerSchema)
				var body []byte
				var err error
				if native {
					body, err = GenerateBody(raw, Place{Directory: "/fixture"})
				} else {
					body, err = providerBody(raw, model)
				}
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]any
				if err := decode(body, &fields); err != nil {
					t.Fatal(err)
				}
				if native {
					fields = fields["params"].(map[string]any)
				}
				tool := fields["tools"].([]any)[0].(map[string]any)
				schema := tool["input_schema"]
				if !native {
					schema = tool["function"].(map[string]any)["parameters"]
				}
				object := schema.(map[string]any)
				properties := object["properties"].(map[string]any)
				items := properties["extra"].(map[string]any)["items"].(map[string]any)
				if strings.Contains(model, "gemini") {
					if items["$ref"] != nil || items["type"] != "object" || items["description"] != "another trigger" || items["required"].([]any)[0] != "on" {
						t.Fatalf("Google lost the referenced constraint: %s", body)
					}
				} else if items["$ref"] != "#/properties/func/properties/triggers/items" {
					t.Fatalf("another provider's schema changed: %s", body)
				}
			})
		}
	}
	for _, schema := range []string{
		`{"type":"object","properties":{"a":{"$ref":"#/$defs/A"},"b":{"$ref":"#/definitions/B"}},"$defs":{"A":{"type":"string"}},"definitions":{"B":{"type":"number"}}}`,
		`{"type":"object","properties":{"data":{"default":{"$ref":"#/properties/x"},"enum":[{"$ref":"#/properties/x"}]},"lost":{"$ref":"#/missing"},"external":{"$ref":"https://example.invalid/schema"}}}`,
		`{"type":"object","properties":{"a/b~c":{"type":"integer","minimum":9007199254740993},"copy":{"$ref":"#/properties/a~1b~0c"}}}`,
	} {
		body, err := providerBody(schemaRequest("google/gemini", schema), "google/gemini")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(schema, "9007199254740993") {
			if bytes.Count(body, []byte("9007199254740993")) != 2 {
				t.Fatalf("escaped pointer or integer precision lost: %s", body)
			}
		} else {
			var fields struct {
				Tools []struct {
					Function struct{ Parameters json.RawMessage }
				}
			}
			if decode(body, &fields) != nil {
				t.Fatal("invalid transformed schema")
			}
			var original, transformed any
			decode([]byte(schema), &original)
			decode(fields.Tools[0].Function.Parameters, &transformed)
			left, _ := json.Marshal(original)
			right, _ := json.Marshal(transformed)
			if !bytes.Equal(left, right) {
				t.Fatalf("supported definitions or ordinary data changed: %s", body)
			}
		}
	}
}

func TestGoogleSchemaRefusesCyclesAndExpansionLimits(t *testing.T) {
	schemas := []string{
		`{"type":"object","properties":{"children":{"items":{"$ref":"#"}}}}`,
		`{"type":"object","properties":{"a":{"$ref":"#/properties/b"},"b":{"$ref":"#/properties/a"}}}`,
	}
	deep := `{"type":"string"}`
	for range 70 {
		deep = `{"type":"array","items":` + deep + `}`
	}
	schemas = append(schemas, deep, `{"type":"string","description":"`+strings.Repeat("x", (1<<20)+1)+`"}`)
	for _, schema := range schemas {
		for _, native := range []bool{false, true} {
			raw := schemaRequest("google/gemini", schema)
			var err error
			if native {
				_, err = GenerateBody(raw, Place{Directory: "/fixture"})
			} else {
				_, err = providerBody(raw, "google/gemini")
			}
			if err == nil || !strings.Contains(err.Error(), "schema") {
				t.Fatalf("cyclic schema silently weakened or dispatched: native=%t %v", native, err)
			}
		}
	}
}

func TestGenerateFitsRecoveredModelEfforts(t *testing.T) {
	for _, item := range []struct{ model, want, expected string }{
		{"deepseek/deepseek-v4-pro", "low", "high"},
		{"deepseek/deepseek-v4.1-flash", "medium", "high"},
		{"deepseek/deepseek-v4.1-flash", "ultra", "max"},
		{"deepseek/deepseek-v4.1-flash", "minimal", "low"},
		{"unknown-model", "high", "high"},
		{"unknown-model", "ultra", "max"},
	} {
		raw := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"reasoning_effort":%q}`, item.model, item.want))
		body, err := GenerateBody(raw, Place{Directory: "/fixture"})
		var wire struct {
			Params struct {
				Effort string `json:"reasoning_effort"`
			}
		}
		if err != nil || decode(body, &wire) != nil || wire.Params.Effort != item.expected {
			t.Errorf("%s/%s: effort=%s expected=%s error=%v", item.model, item.want, wire.Params.Effort, item.expected, err)
		}
	}
}

func TestGenerateRecoveredDisabledReasoning(t *testing.T) {
	for _, effort := range []string{"none", "off"} {
		for _, model := range []string{"deepseek/deepseek-v4.1-flash", "moonshotai/Kimi-K2.7-Code", "unknown-model"} {
			raw := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"reasoning_effort":%q}`, model, effort))
			body, err := GenerateBody(raw, Place{Directory: "/fixture"})
			if model == "deepseek/deepseek-v4.1-flash" {
				if err != nil || !bytes.Contains(body, []byte(`"reasoning_effort":"off"`)) {
					t.Errorf("recovered native disabled reasoning was refused or dropped: %s %s %v", effort, body, err)
				}
			} else if err == nil {
				t.Errorf("unrecovered model gained disabled reasoning: %s", body)
			}
		}
	}
	for _, model := range []string{"moonshotai/Kimi-K2.7-Code", "unknown-model"} {
		effort := "high"
		if model == "unknown-model" {
			effort = "minimal"
		}
		raw := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"reasoning_effort":%q}`, model, effort))
		if body, err := GenerateBody(raw, Place{Directory: "/fixture"}); err == nil {
			t.Errorf("unrecovered reasoning wire silently accepted: %s", body)
		}
	}
	models, err := ParseModels([]byte(`{"data":[{"id":"deepseek/deepseek-v4.1-flash"},{"id":"moonshotai/Kimi-K2.7-Code"}]}`), Subscription{Plan: "Go"}, false, true)
	if err != nil || len(models) != 3 || len(models[0].Efforts) == 0 || models[0].Efforts[0] != "none" || len(models[1].Efforts) != 0 {
		t.Errorf("Go catalog does not project the recovered thinking-only off level: %+v %v", models, err)
	}
}

func TestGenerateRefusesUnknownNestedControls(t *testing.T) {
	for _, raw := range []string{
		`{"model":"m","messages":[{"role":"user","content":"hi","vendor_control":true}]}`,
		`{"model":"m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f","strict":true,"parameters":{}}}]}`,
		`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}]}`,
		`{"model":"m","messages":[{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"f","arguments":"{}","vendor_control":1}}]}]}`,
	} {
		if _, err := GenerateBody([]byte(raw), Place{Directory: "/fixture"}); err == nil {
			t.Errorf("silently discarded a nested control: %s", raw)
		}
	}
}

func TestModuleGoPlanContinuesOnlyCompletedPausedBodies(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, mode := range []string{"complete", "late-error", "invalid-cache-split", "truncated", "exhausted", "second-refused"} {
			t.Run(fmt.Sprintf("%s/stream=%t", mode, streaming), func(t *testing.T) {
				var mutex sync.Mutex
				var bodies [][]byte
				var sessions []string
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/alpha/billing/subscriptions" {
						io.WriteString(writer, `{"data":{"planId":"individual-go","status":"active"}}`)
						return
					}
					if request.URL.Path != "/alpha/generate" {
						t.Errorf("unexpected request: %s", request.URL.Path)
						writer.WriteHeader(500)
						return
					}
					body, _ := io.ReadAll(request.Body)
					mutex.Lock()
					bodies = append(bodies, body)
					sessions = append(sessions, request.Header.Get("x-session-id"))
					attempt := len(bodies)
					mutex.Unlock()
					if attempt > 1 && mode == "second-refused" {
						writer.WriteHeader(429)
						io.WriteString(writer, `{"error":{"message":"window_limit"}}`)
						return
					}
					writer.Header().Set("Content-Type", "application/x-ndjson")
					if mode == "truncated" {
						writer.Header().Set("Content-Length", "8192")
					}
					if attempt == 1 || mode == "exhausted" {
						io.WriteString(writer, "{\"type\":\"text-delta\",\"text\":\"first\"}\n")
						io.WriteString(writer, "{\"type\":\"finish\",\"finishReason\":\"other\",\"rawFinishReason\":\"pause_turn\",\"totalUsage\":{\"inputTokens\":10,\"outputTokens\":2,\"inputTokenDetails\":{\"cacheReadTokens\":3}}}\n")
						io.WriteString(writer, "{\"type\":\"cache-write-tokens\",\"cacheWriteTokens\":2}\n")
						if mode == "late-error" {
							io.WriteString(writer, "{\"type\":\"error\",\"error\":\"late failure\"}\n")
						} else if mode == "invalid-cache-split" {
							io.WriteString(writer, "{\"type\":\"cache-write-tokens\",\"cacheWriteTokens\":9}\n")
						}
						return
					}
					io.WriteString(writer, "{\"type\":\"text-delta\",\"text\":\"second\"}\n{\"type\":\"tool-call\",\"toolCallId\":\"t1\",\"toolName\":\"read\",\"input\":{\"path\":\"x\"}}\n")
					io.WriteString(writer, "{\"type\":\"finish\",\"finishReason\":\"tool-calls\",\"totalUsage\":{\"inputTokens\":7,\"outputTokens\":4,\"inputTokenDetails\":{\"cacheReadTokens\":1,\"cacheWriteTokens\":1}}}\n")
				}))
				defer server.Close()
				worker := fixture(t, Config{APIURL: server.URL, WorkingDirectory: "/fixture"})
				raw := []byte(fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":%t}`, streaming))
				result, events, err := worker.call(sdk.MethodProviderExecute, sdk.ExecuteRequest{Protocol: "chat", Model: "m", Body: raw}, testAuth())
				mutex.Lock()
				defer mutex.Unlock()
				_, body := eventsBody(t, events)
				if mode == "complete" {
					var execution sdk.ExecuteResult
					if err != nil || decode(result, &execution) != nil || execution.Outcome != "completed" || execution.Usage == nil {
						t.Fatalf("paused answer was not continued: %s %v", result, err)
					}
					counts := execution.Usage
					if counts.InputTokens != 10 || counts.OutputTokens != 6 || counts.CachedTokens != 4 || counts.CacheWriteTokens != 3 {
						t.Fatalf("continuations lost accounting: %+v", counts)
					}
					if !bytes.Contains(body, []byte(`"prompt_tokens":17`)) {
						t.Fatalf("SDK cache normalization changed the inclusive Chat usage: %s", body)
					}
					if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) || sessions[0] == "" || sessions[0] != sessions[1] {
						t.Fatalf("continuation identity/body changed: calls=%d sessions=%v", len(bodies), sessions)
					}
					for _, text := range []string{"first", "second", `"id":"t1"`} {
						if bytes.Count(body, []byte(text)) != 1 {
							t.Fatalf("lost or repeated output %s: %s", text, body)
						}
					}
				} else {
					failure, ok := err.(*sdk.Error)
					if !ok || failure.Outcome != "uncertain" || bytes.Contains(body, []byte("[DONE]")) || bytes.Contains(body, []byte(`"finish_reason":"`)) {
						t.Fatalf("unsafe successful terminal: %s %v", body, err)
					}
					expected := 1
					if mode == "exhausted" {
						expected = 6
					} else if mode == "second-refused" {
						expected = 2
					}
					if len(bodies) != expected {
						t.Fatalf("unexpected continuation/retry: %d expected=%d", len(bodies), expected)
					}
				}
				accepted := 0
				for _, event := range events {
					if event.Event == "accepted" {
						accepted++
					}
				}
				if accepted != 1 {
					t.Fatalf("one host operation gained %d admissions", accepted)
				}
			})
		}
	}
}
