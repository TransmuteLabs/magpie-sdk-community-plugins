package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConsoleSkillScalarFrontmatter(t *testing.T) {
	for _, content := range []string{
		"---\nname: 'deploy''s helper'\ndescription: \"Do a thing # safely\"\n---\nBody",
		"---\nname: deploy\ndescription: |-\n  First line.\n  Second line.\n---\nBody",
		"---\nname: deploy\ndescription: >\n  First line\n  continues here.\n---\nBody",
		"---\ndescription: \"Named by its slug\"\n---\nBody",
		"---\nname: deploy\n---\nBody",
		"---\nname: deploy\ndescription: Do this: safely\nmetadata:\n  opencode/autoinvoke: false\ndisable-model-invocation: true\n---\nBody",
		"Body without frontmatter",
	} {
		t.Run(strings.ReplaceAll(content, "\n", "/"), func(t *testing.T) {
			if !validSkillFrontmatter(content) {
				t.Fatal("native optional/quoted/block scalar skill frontmatter rejected")
			}
		})
	}
}

func TestConsoleWatchTypesUseSelectedWorkspaceAndGates(t *testing.T) {
	var dispatched atomic.Int32
	server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer PRIVATE-types" {
			t.Error("watch types lost selected credentials")
		}
		switch request.URL.Path {
		case "/api/v2/config":
			writer.Header()["X-Opencode-Event-Watches"] = []string{""}
			io.WriteString(writer, `{}`)
		case "/api/event-watches/types":
			dispatched.Add(1)
			json.NewEncoder(writer).Encode(map[string]any{"types": []any{map[string]any{
				"server": "fixture", "name": "deploy.finished", "description": "  A\n\tdeploy " + strings.Repeat("x", 400),
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
			}}})
		default:
			t.Error("watch types called unrelated route", request.URL.Path)
			writer.WriteHeader(500)
		}
	})
	defer server.Close()
	identity := ZenCredentials{Kind: "key", Key: "PRIVATE-types"}
	for _, enabled := range []bool{false, true} {
		harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
			HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ConsoleWatches: enabled}))
		frame, _ := harness.action("console.watch.types", zenScope(ProviderZen, identity), map[string]any{}, raw(identity))
		if !enabled {
			if frame.Error == nil || dispatched.Load() != 0 {
				t.Fatal("disabled watches gained event-type access")
			}
			continue
		}
		var result struct {
			Types []struct {
				Server, Name, Description string
				InputSchema               map[string]any
			}
		}
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Types) != 1 {
			t.Fatal("selected workspace cannot inspect native watch types", frame.Error)
		}
		item := result.Types[0]
		if item.Server != "fixture" || item.Name != "deploy.finished" || len(item.Description) != 300 ||
			!strings.HasPrefix(item.Description, "A deploy ") || item.InputSchema["type"] != "object" {
			t.Fatal("watch type normalization lost the native schema or description bound")
		}
		frame, _ = harness.action("console.watch.types", zenScope(ProviderZen, identity), map[string]any{"sessionID": "foreign"}, raw(identity))
		if frame.Error == nil || dispatched.Load() != 1 {
			t.Fatal("watch type read accepted substituted session authority")
		}
	}
}

func TestConsoleWatchTypesRejectMalformedOrForeignAuthority(t *testing.T) {
	for _, fixture := range []struct {
		name, body string
		header     bool
	}{
		{"missing-header", `{"types":[]}`, false},
		{"missing-types", `{}`, true},
		{"null-types", `{"types":null}`, true},
		{"null-description", `{"types":[{"server":"fixture","name":"done","description":null,"inputSchema":{}}]}`, true},
		{"nonstring-description", `{"types":[{"server":"fixture","name":"done","description":42,"inputSchema":{}}]}`, true},
		{"array-schema", `{"types":[{"server":"fixture","name":"done","inputSchema":[]}]}`, true},
		{"missing-server", `{"types":[{"name":"done","inputSchema":{}}]}`, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var dispatched atomic.Int32
			server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v2/config":
					if fixture.header {
						writer.Header()["X-Opencode-Event-Watches"] = []string{""}
					}
					io.WriteString(writer, `{}`)
				case "/api/event-watches/types":
					dispatched.Add(1)
					io.WriteString(writer, fixture.body)
				default:
					t.Error("unexpected Console route", request.URL.Path)
					writer.WriteHeader(500)
				}
			})
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ConsoleWatches: true}))
			identity := ZenCredentials{Kind: "key", Key: "PRIVATE-types"}
			frame, _ := harness.action("console.watch.types", zenScope(ProviderZen, identity), map[string]any{}, raw(identity))
			if frame.Error == nil || !fixture.header && dispatched.Load() != 0 {
				t.Fatal("invalid workspace type response or missing header accepted")
			}
			previous := dispatched.Load()
			frame, _ = harness.action("console.watch.types", zenScope(ProviderZenPublic, identity), map[string]any{}, raw(identity))
			if frame.Error == nil || dispatched.Load() != previous {
				t.Fatal("public scope gained personal workspace access")
			}
		})
	}
}

func TestConsoleWatchTypeDescriptionKeepsUnicodeBoundary(t *testing.T) {
	server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/config" {
			writer.Header()["X-Opencode-Event-Watches"] = []string{""}
			io.WriteString(writer, `{}`)
			return
		}
		json.NewEncoder(writer).Encode(map[string]any{"types": []any{map[string]any{
			"server": "fixture", "name": "done", "inputSchema": map[string]any{},
			"description": strings.Repeat("a", 299) + "🦆 trailing",
		}}})
	})
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ConsoleWatches: true}))
	identity := ZenCredentials{Kind: "key", Key: "PRIVATE-types"}
	frame, _ := harness.action("console.watch.types", zenScope(ProviderZen, identity), map[string]any{}, raw(identity))
	var result struct {
		Types []ConsoleWatchType `json:"types"`
	}
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Types) != 1 ||
		result.Types[0].Description != strings.Repeat("a", 299) {
		t.Fatal("bounded description split a Unicode character", frame.Error)
	}
}

func TestConsoleInspectionIdentifiesItsActualOwner(t *testing.T) {
	server := newZenFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		agent := request.Header.Get("User-Agent")
		if !strings.HasPrefix(agent, "magpie-opencode/"+WindowManifest().Version+" ") ||
			!strings.Contains(agent, "console-inspection") || strings.Contains(agent, "opencode/latest/") {
			t.Error("inspection claimed native Console policy/session ownership", agent)
		}
		io.WriteString(writer, `{}`)
	})
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{}))
	identity := ZenCredentials{Kind: "key", Key: "PRIVATE-inspection"}
	frame, _ := harness.action("console.snapshot", zenScope(ProviderZen, identity), map[string]any{}, raw(identity))
	if frame.Error != nil {
		t.Fatal("selected-account inspection failed", frame.Error)
	}
}
