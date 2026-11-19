package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func policyFixtureModule(options ModuleOptions, protocol, model string) sdk.Module {
	module := NewWindowModule(options)
	start := module.Start
	module.Start = func(ctx context.Context, configuration sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
		ready, close, err := start(ctx, configuration)
		if err == nil {
			identity := ZenCredentials{Kind: "public", Key: "public"}
			state := ready.State.(*instance)
			state.console.catalog.values[ProviderZenPublic+":"+identity.AccountID(ProviderZenPublic)+":1"] = modelListing{
				models: []sdk.Model{{ID: model, Protocols: []protocolext.Protocol{protocolext.Protocol(protocol)}, Free: true}},
				at:     time.Now(),
			}
		}
		return ready, close, err
	}
	return module
}

func TestZenPublicNativeStreamPolicy(t *testing.T) {
	fixture, err := os.ReadFile("testdata/donors/zen-replies.json")
	var replies map[string]struct{ SSE string }
	if err != nil || json.Unmarshal(fixture, &replies) != nil {
		t.Fatal("native stream fixtures unavailable", err)
	}
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, callerStream := range []bool{false, true} {
			name := protocol + "/json"
			if callerStream {
				name = protocol + "/sse"
			}
			t.Run(name, func(t *testing.T) {
				model := "fixture-" + protocol + "-free"
				server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/inference/v1/models" {
						json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]string{{"id": model}}})
						return
					}
					var body map[string]any
					json.NewDecoder(request.Body).Decode(&body)
					if body["stream"] != true {
						t.Error("public upstream must stream even for a JSON caller")
					}
					if protocol == "chat" {
						options, _ := body["stream_options"].(map[string]any)
						if options["include_usage"] != true || options["fixture"] != "kept" {
							t.Error("public Chat must request usage and preserve other stream options")
						}
					}
					writer.Header().Set("Content-Type", "text/event-stream")
					writer.Header().Set("X-Fixture", "kept")
					io.WriteString(writer, replies[protocol].SSE)
				})
				defer server.Close()
				harness := serveHarness(t, policyFixtureModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
					HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}, protocol, model), raw(moduleConfig{ProjectID: "global"}))
				identity := ZenCredentials{Kind: "public", Key: "public"}
				body := map[string]any{"model": model, "stream": callerStream, "messages": []any{},
					"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "Read", "parameters": map[string]any{"type": "object"}}}}}
				if protocol == "messages" {
					body["tools"] = []any{map[string]any{"name": "Read", "input_schema": map[string]any{"type": "object"}}}
				}
				if protocol == "chat" {
					body["stream_options"] = map[string]any{"fixture": "kept", "include_usage": false}
				}
				frame, received := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity),
					sdk.ExecuteRequest{Protocol: protocol, Model: model, Body: raw(body)}, raw(identity))
				if frame.Error != nil {
					t.Fatal("public native response failed", frame.Error)
				}
				if len(harness.headers) != 1 || harness.headers[0].Headers.Get("X-Fixture") != "kept" {
					t.Fatal("native response metadata lost")
				}
				if callerStream {
					if json.Valid(received) || !bytes.Contains(received, []byte("data:")) ||
						harness.headers[0].Headers.Get("Content-Type") != "text/event-stream" {
						t.Fatal("stream caller did not retain native SSE", string(received))
					}
				} else if !json.Valid(received) || !bytes.Contains(received, []byte(`"Read"`)) ||
					harness.headers[0].Headers.Get("Content-Type") != "application/json" ||
					harness.headers[0].Headers.Get("Content-Length") != "" {
					t.Fatal("JSON caller did not receive collapsed native tools and corrected headers", string(received))
				}
			})
		}
	}
}

func TestZenPublicExplicitEnvironment(t *testing.T) {
	server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/inference/v1/models" {
			io.WriteString(writer, `{"data":[{"id":"fixture-free"}]}`)
			return
		}
		sent, _ := io.ReadAll(request.Body)
		for _, fact := range []string{"Working directory: /fixture/work", "Workspace root folder: /fixture", "Is directory a git repo: yes",
			"Platform: linux", "Prefer /fixture/tmp/opencode"} {
			if !bytes.Contains(sent, []byte(fact)) {
				t.Error("configured public environment fact missing", fact)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"choices":[{"message":{"role":"assistant","content":"fixture"}}]}`)
	})
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(map[string]any{
		"projectId": "global", "publicEnvironment": map[string]any{
			"directory": "/fixture/work", "workspace": "/fixture", "git": true, "platform": "linux", "temporary": "/fixture/tmp/opencode",
		},
	}))
	identity := ZenCredentials{Kind: "public", Key: "public"}
	frame, _ := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), sdk.ExecuteRequest{
		Protocol: "chat", Model: "fixture-free", Body: []byte(`{"model":"fixture-free","messages":[{"role":"system","content":"Existing instruction."}]}`),
	}, raw(identity))
	if frame.Error != nil {
		t.Fatal("explicit public environment failed", frame.Error)
	}
}

func TestZenPublicJSONCollapseRejectsIncompleteNativeReply(t *testing.T) {
	for _, reply := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n",
		"event: error\ndata: {\"message\":\"PRIVATE-error\"}\n\n",
		"data: {\"error\":{\"message\":\"PRIVATE-error\"}}\n\ndata: [DONE]\n\n",
	} {
		t.Run(strings.ReplaceAll(reply, "\n", ""), func(t *testing.T) {
			server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/inference/v1/models" {
					io.WriteString(writer, `{"data":[{"id":"fixture-free"}]}`)
					return
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(writer, reply)
			})
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
			identity := ZenCredentials{Kind: "public", Key: "public"}
			frame, received := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), sdk.ExecuteRequest{
				Protocol: "chat", Model: "fixture-free", Body: []byte(`{"model":"fixture-free","messages":[]}`),
			}, raw(identity))
			if frame.Error == nil || frame.Error.Outcome != "uncertain" || len(received) != 0 ||
				strings.Contains(frame.Error.Message, "PRIVATE-error") {
				t.Fatal("incomplete public JSON leaked a partial reply, private error or completed outcome", frame.Error, string(received))
			}
		})
	}
}
