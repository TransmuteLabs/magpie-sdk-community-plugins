package opencode

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type zenDSH028Definition struct {
	Name, Description, ServedName, ServedDescription string
	Parameters                                       map[string]any
}

func zenDSH028Definitions(t *testing.T) []zenDSH028Definition {
	t.Helper()
	body, err := os.ReadFile("testdata/donors/zen-dsh028.json")
	var definitions []zenDSH028Definition
	if err != nil || json.Unmarshal(body, &definitions) != nil || len(definitions) != 3 {
		t.Fatal("missing literal Zen 0.1.28 DSH definitions", err)
	}
	return definitions
}

func zenDSH028Tools(protocol string, definitions []zenDSH028Definition) []any {
	tools := make([]any, 0, len(definitions))
	for _, definition := range definitions {
		tool := map[string]any{"name": definition.Name, "description": definition.Description}
		if protocol == "messages" {
			tool["input_schema"] = definition.Parameters
		} else {
			tool["parameters"] = definition.Parameters
		}
		if protocol == "chat" {
			tool = map[string]any{"type": "function", "function": tool}
		} else if protocol == "responses" {
			tool["type"] = "function"
		}
		tools = append(tools, tool)
	}
	return tools
}

func zenDSH028Request(protocol string, tools []any) map[string]any {
	request := map[string]any{"model": "fixture-free", "tools": tools, "seed": json.Number("9007199254740993")}
	if protocol == "responses" {
		request["input"] = []any{map[string]any{"role": "user", "content": "fixture"}}
	} else {
		request["messages"] = []any{map[string]any{"role": "user", "content": "fixture"}}
	}
	return request
}

func zenDSH028ShapedTools(t *testing.T, protocol string, body []byte) []map[string]any {
	t.Helper()
	value, err := zenToolJSON(body)
	if err != nil {
		t.Fatal("invalid shaped request", err)
	}
	tools, ok := value["tools"].([]any)
	if !ok {
		t.Fatal("shaped request lost its tools")
	}
	definitions := make([]map[string]any, 0, len(tools))
	for _, entry := range tools {
		definition := zenToolObject(entry)
		if protocol == "chat" {
			definition = zenToolObject(definition["function"])
		}
		definitions = append(definitions, definition)
	}
	return definitions
}

func TestZenDSH028NormalizesLiteralToolsBeforeCoreStandins(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			original := zenDSH028Definitions(t)
			request := raw(zenDSH028Request(protocol, zenDSH028Tools(protocol, original)))
			before := bytes.Clone(request)
			shaped, policy, err := prepareZenTools(protocol, request)
			if err != nil {
				t.Fatal(err)
			}
			definitions := zenDSH028ShapedTools(t, protocol, shaped)
			for index, expected := range original {
				actual := definitions[index]
				schema := "parameters"
				if protocol == "messages" {
					schema = "input_schema"
				}
				if actual["name"] != expected.ServedName || actual["description"] != expected.ServedDescription ||
					!sameToolJSON(raw(actual[schema]), raw(expected.Parameters)) {
					t.Errorf("DSH tool %s lost its exact alias, wording or schema: %s", expected.Name, raw(actual))
				}
				if policy.name(expected.ServedName) != expected.Name {
					t.Errorf("reply lost caller-owned name %s", expected.Name)
				}
			}
			core := map[string]int{}
			for _, definition := range definitions[len(original):] {
				if definition["description"] != "Not available. Never call this tool." {
					t.Fatal("a core stand-in gained callable authority")
				}
				core[zenToolString(definition["name"])]++
			}
			if !reflect.DeepEqual(core, map[string]int{"edit": 1, "glob": 1, "grep": 1, "read": 1, "shell": 1}) ||
				policy.declared["shell"] || !bytes.Equal(request, before) || !bytes.Contains(shaped, []byte("9007199254740993")) {
				t.Fatal("DSH normalization lost stand-ins, caller authority, input bytes or numeric precision")
			}
		})
	}
}

func TestZenDSH028AliasesRequireEveryActualSchemaProperty(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, original := range zenDSH028Definitions(t)[:2] {
			for _, property := range []string{"file_path", "old_string", "new_string", "offset", "limit"} {
				properties := zenToolObject(original.Parameters["properties"])
				if _, present := properties[property]; !present {
					continue
				}
				t.Run(protocol+"/"+original.Name+"/without-"+property, func(t *testing.T) {
					definition := zenDSH028Definitions(t)[0]
					if original.Name == "read" {
						definition = zenDSH028Definitions(t)[1]
					}
					delete(zenToolObject(definition.Parameters["properties"]), property)
					shaped, _, err := prepareZenTools(protocol, raw(zenDSH028Request(protocol, zenDSH028Tools(protocol, []zenDSH028Definition{definition}))))
					if err != nil {
						t.Fatal(err)
					}
					actual := zenDSH028ShapedTools(t, protocol, shaped)[0]
					if actual["name"] != original.Name || actual["description"] != original.ServedDescription {
						t.Fatal("incomplete schema renamed a tool or suppressed independent exact wording", string(shaped))
					}
				})
			}
		}
		t.Run(protocol+"/ordinary-and-whole-description", func(t *testing.T) {
			definitions := []zenDSH028Definition{
				{Name: "read", Description: "Reads a file", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filePath": nil, "offset": nil, "limit": nil}}},
				{Name: "edit", Description: "Edits a file", Parameters: map[string]any{"type": "object", "properties": map[string]any{"file_path": nil}}},
				{Name: "custom", Description: zenDSH028Definitions(t)[0].Description + " Extra instruction."},
				{Name: "another", Description: "\n " + zenDSH028Definitions(t)[0].Description + " \t"},
				{Name: "Read", Description: "Keep case", Parameters: zenDSH028Definitions(t)[1].Parameters},
			}
			shaped, _, err := prepareZenTools(protocol, raw(zenDSH028Request(protocol, zenDSH028Tools(protocol, definitions))))
			if err != nil {
				t.Fatal(err)
			}
			actual := zenDSH028ShapedTools(t, protocol, shaped)
			for index, expected := range definitions {
				description := expected.Description
				if index == 3 {
					description = zenDSH028Definitions(t)[0].ServedDescription
				}
				if actual[index]["name"] != expected.Name || actual[index]["description"] != description {
					t.Fatal("ordinary OpenCode tool or nonmatching description changed", string(shaped))
				}
			}
		})
		t.Run(protocol+"/javascript-description-whitespace", func(t *testing.T) {
			original := zenDSH028Definitions(t)[0]
			definitions := []zenDSH028Definition{
				{Name: "bom", Description: "\ufeff" + original.Description + "\ufeff"},
				{Name: "nel", Description: "\u0085" + original.Description + "\u0085"},
			}
			shaped, _, err := prepareZenTools(protocol, raw(zenDSH028Request(protocol, zenDSH028Tools(protocol, definitions))))
			if err != nil {
				t.Fatal(err)
			}
			actual := zenDSH028ShapedTools(t, protocol, shaped)
			if actual[0]["description"] != original.ServedDescription || actual[1]["description"] != definitions[1].Description {
				t.Fatal("whole-description matching differs from JavaScript trim", string(shaped))
			}
		})
	}
}

func TestZenDSH028CollisionHistoryChoiceAndReplyNames(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			definitions := append(zenDSH028Definitions(t),
				zenDSH028Definition{Name: "text_replace"}, zenDSH028Definition{Name: "text_replace_2"}, zenDSH028Definition{Name: "file_view"})
			request := zenDSH028Request(protocol, zenDSH028Tools(protocol, definitions))
			switch protocol {
			case "chat":
				request["messages"] = []any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c0", "type": "function", "function": map[string]any{"name": "edit", "arguments": `{"file_path":"fixture"}`}}}}}
				request["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": "read"}}
				request["function_call"] = map[string]any{"name": "edit"}
			case "messages":
				request["messages"] = []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "c0", "name": "edit", "input": map[string]any{"file_path": "fixture"}}}}}
				request["tool_choice"] = map[string]any{"type": "tool", "name": "read"}
			case "responses":
				request["input"] = []any{map[string]any{"type": "function_call", "call_id": "c0", "name": "edit", "arguments": `{"file_path":"fixture"}`}}
				request["tool_choice"] = map[string]any{"type": "function", "name": "read"}
			}
			shaped, policy, err := prepareZenTools(protocol, raw(request))
			if err != nil {
				t.Fatal(err)
			}
			actual := zenDSH028ShapedTools(t, protocol, shaped)
			if actual[0]["name"] != "text_replace_3" || actual[1]["name"] != "file_view_2" ||
				actual[3]["name"] != "text_replace" || actual[4]["name"] != "text_replace_2" || actual[5]["name"] != "file_view" {
				t.Fatal("DSH aliases collided with caller-declared tools", string(shaped))
			}
			value, _ := zenToolJSON(shaped)
			choice := zenToolObject(value["tool_choice"])
			past := ""
			switch protocol {
			case "chat":
				choice = zenToolObject(choice["function"])
				message := zenToolObject(value["messages"].([]any)[0])
				past = zenToolString(zenToolObject(zenToolObject(message["tool_calls"].([]any)[0])["function"])["name"])
				if zenToolObject(value["function_call"])["name"] != "text_replace_3" {
					t.Fatal("legacy selected function lost alias")
				}
			case "messages":
				past = zenToolString(zenToolObject(zenToolObject(value["messages"].([]any)[0])["content"].([]any)[0])["name"])
			case "responses":
				past = zenToolString(zenToolObject(value["input"].([]any)[0])["name"])
			}
			if past != "text_replace_3" || choice["name"] != "file_view_2" || !bytes.Contains(shaped, []byte("fixture")) {
				t.Fatal("history or selected tool lost collision-safe identity", string(shaped))
			}
			reply := `{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"text_replace_3","arguments":"{}"}}]}}]}`
			if protocol == "responses" {
				reply = `{"output":[{"type":"function_call","name":"text_replace_3","call_id":"c0","arguments":"{}"}]}`
			} else if protocol == "messages" {
				reply = `{"type":"message","content":[{"type":"tool_use","name":"text_replace_3","id":"c0","input":{}}],"stop_reason":"tool_use"}`
			}
			var output bytes.Buffer
			err = relayZenTools(t.Context(), strings.NewReader(reply), policy, protocol, false, func(body []byte) error { _, err := output.Write(body); return err })
			if err != nil || !bytes.Contains(output.Bytes(), []byte(`"name":"edit"`)) || bytes.Contains(output.Bytes(), []byte("text_replace_3")) ||
				policy.name("text_replace") != "text_replace" || policy.name("file_view_2") != "read" {
				t.Fatal("reply stole a declared alias or failed to restore caller identity", err, output.String())
			}
		})
		t.Run(protocol+"/properties-not-required-list", func(t *testing.T) {
			definitions := []zenDSH028Definition{
				{Name: "edit", Description: "neutral", Parameters: map[string]any{"properties": map[string]any{"file_path": nil, "old_string": nil, "new_string": nil}}},
				{Name: "read", Description: "neutral", Parameters: map[string]any{"required": []any{"file_path", "offset", "limit"}}},
			}
			shaped, _, err := prepareZenTools(protocol, raw(zenDSH028Request(protocol, zenDSH028Tools(protocol, definitions))))
			if err != nil {
				t.Fatal(err)
			}
			actual := zenDSH028ShapedTools(t, protocol, shaped)
			if actual[0]["name"] != "text_replace" || actual[1]["name"] != "read" {
				t.Fatal("DSH matching did not use actual property presence", string(shaped))
			}
		})
	}
}

func TestZenDSH028FragmentedReplyRestoresCallerName(t *testing.T) {
	_, policy, err := prepareZenTools("chat", raw(zenDSH028Request("chat", zenDSH028Tools("chat", zenDSH028Definitions(t)))))
	if err != nil {
		t.Fatal(err)
	}
	reply := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c0","type":"function","function":{"name":"text_","arguments":"{"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"replace","arguments":"}"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	var output bytes.Buffer
	err = relayZenTools(t.Context(), strings.NewReader(reply), policy, "chat", true, func(body []byte) error { _, err := output.Write(body); return err })
	if err != nil || !bytes.Contains(output.Bytes(), []byte(`"name":"edit"`)) || bytes.Contains(output.Bytes(), []byte("text_")) ||
		!bytes.Contains(output.Bytes(), []byte("[DONE]")) || !bytes.Contains(output.Bytes(), []byte(`"arguments":"{}"`)) {
		t.Fatal("fragmented DSH reply leaked its served name or lost arguments/completion", err, output.String())
	}
}

func TestZenDSH028LegacyFunctionsDoNotSupplyToolSchemas(t *testing.T) {
	definitions := zenDSH028Definitions(t)
	request := zenDSH028Request("chat", nil)
	request["tools"] = []any{}
	request["functions"] = []any{map[string]any{"name": definitions[0].Name,
		"description": definitions[0].Description, "parameters": definitions[0].Parameters}}
	shaped, _, err := prepareZenTools("chat", raw(request))
	if err != nil {
		t.Fatal(err)
	}
	value, _ := zenToolJSON(shaped)
	function := zenToolObject(value["functions"].([]any)[0])
	if function["name"] != "edit" || function["description"] != definitions[0].Description {
		t.Fatal("legacy functions invented a tool-schema or description match", string(shaped))
	}
}

func TestZenDSH028ActualSelectedExecutionAndPrivateBoundary(t *testing.T) {
	for _, kind := range []string{"public", "key"} {
		t.Run(kind, func(t *testing.T) {
			var dispatches atomic.Int32
			original := raw(zenDSH028Request("chat", zenDSH028Tools("chat", zenDSH028Definitions(t))))
			server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/inference/v1/models" {
					io.WriteString(writer, `{"data":[{"id":"fixture-free"}]}`)
					return
				}
				dispatches.Add(1)
				sent, _ := io.ReadAll(request.Body)
				if request.Header.Get("Authorization") != "Bearer "+kind || request.Header.Get("x-api-key") != "" ||
					request.Header.Get("Cookie") != "" || request.URL.Path != "/inference/openai/v1/chat/completions" {
					t.Error("DSH compatibility changed selected authority or route")
				}
				name := "edit"
				if kind == "public" {
					tools := zenDSH028ShapedTools(t, "chat", sent)
					if tools[0]["name"] != "text_replace" || tools[1]["name"] != "file_view" ||
						tools[2]["description"] != zenDSH028Definitions(t)[2].ServedDescription {
						t.Error("ordinary public execution did not use the common DSH policy")
					}
					name = "text_replace"
				} else if !bytes.Equal(sent, original) {
					t.Error("public DSH compatibility rewrote a private request")
				}
				writer.Header().Set("Content-Type", "application/json")
				io.WriteString(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c0","type":"function","function":{"name":"`+name+`","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
			}))
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
			identity := ZenCredentials{Kind: kind, Key: kind}
			provider := ProviderZenPublic
			if kind == "key" {
				provider = ProviderZen
			}
			request := sdk.ExecuteRequest{Protocol: "chat", Model: "fixture-free", Body: original,
				Headers: http.Header{"Authorization": {"foreign"}, "X-Api-Key": {"foreign"}, "Cookie": {"foreign"}}}
			frame, reply := harness.invoke(sdk.MethodProviderExecute, zenScope(provider, identity), request, raw(identity))
			if frame.Error != nil || dispatches.Load() != 1 || !bytes.Contains(reply, []byte(`"name":"edit"`)) || bytes.Contains(reply, []byte("text_replace")) {
				t.Fatal("ordinary selected execution lost tool identity or sent again", frame.Error, dispatches.Load(), string(reply))
			}
		})
	}
}
