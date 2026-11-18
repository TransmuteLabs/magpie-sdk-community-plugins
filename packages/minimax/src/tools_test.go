package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type toolsRecordedCall struct {
	Path   string
	Body   map[string]any
	Budget time.Duration
}

type toolsFakeTransport struct {
	test     *testing.T
	calls    []toolsRecordedCall
	catalog  any
	respond  func(context.Context, string, map[string]any) (any, error)
	transfer func(context.Context, string, string, []byte, http.Header) (ByteResponse, error)
}

func (fixture *toolsFakeTransport) JSON(ctx context.Context, endpoint string, body any, limit int64) (json.RawMessage, error) {
	fixture.test.Helper()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var arguments map[string]any
	if err := json.Unmarshal(raw(body), &arguments); err != nil {
		fixture.test.Fatal(err)
	}
	deadline, bounded := ctx.Deadline()
	if !bounded || limit != toolResponseLimit {
		fixture.test.Fatal("request lost its deadline or response byte limit")
	}
	fixture.calls = append(fixture.calls, toolsRecordedCall{Path: endpoint, Body: arguments, Budget: time.Until(deadline)})
	if endpoint == matrixListPath {
		return raw(fixture.catalog), nil
	}
	if endpoint == matrixCallPath {
		if err := json.Unmarshal([]byte(arguments["arguments_json"].(string)), &arguments); err != nil {
			fixture.test.Fatal(err)
		}
	}
	name := endpoint
	if endpoint == matrixCallPath {
		name = strings.TrimPrefix(text(fixture.calls[len(fixture.calls)-1].Body["runtime_tool_name"]), matrixPrefix)
	}
	value, err := fixture.respond(ctx, name, arguments)
	if err != nil {
		return nil, err
	}
	return raw(value), nil
}

func (fixture *toolsFakeTransport) Transfer(ctx context.Context, method, address string, body []byte, headers http.Header, limit int64) (ByteResponse, error) {
	fixture.test.Helper()
	if fixture.transfer == nil {
		fixture.test.Fatal("unexpected media transfer")
	}
	return fixture.transfer(ctx, method, address, body, headers)
}

func toolsEntry(name, schema string) map[string]any {
	return map[string]any{"provider": "matrix", "provider_tool_name": "live_" + name, "runtime_tool_name": matrixPrefix + name, "input_schema_json": schema, "description": "Live " + name}
}

func toolsCatalog(entries ...map[string]any) map[string]any {
	return map[string]any{"tools": entries, "partial": false, "provider_failures": []any{}}
}

func toolsSuccess(value any) map[string]any {
	return map[string]any{"is_error": false, "result_json": string(raw(value))}
}

func toolsFixture(test *testing.T, names ...string) (*toolsFakeTransport, matrixEngine, *int) {
	test.Helper()
	entries := make([]map[string]any, 0, len(names))
	for _, name := range names {
		entries = append(entries, toolsEntry(name, `{"type":"object"}`))
	}
	fixture := &toolsFakeTransport{test: test, catalog: toolsCatalog(entries...)}
	fixture.respond = func(context.Context, string, map[string]any) (any, error) {
		return toolsSuccess(map[string]any{"ok": true}), nil
	}
	accepted := 0
	engine := matrixEngine{client: fixture, accept: func(context.Context) error {
		accepted++
		return nil
	}}
	return fixture, engine, &accepted
}

func TestToolsCatalogUsesLiveSchemaAndFreshRoute(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test, "generate_image")
	schema := `{"type":"object","required":["live"],"properties":{"live":{"type":"string"}},"additionalProperties":false}`
	entry := toolsEntry("generate_image", schema)
	fixture.catalog = toolsCatalog(entry)
	if _, err := engine.runTool(context.Background(), "generate_image", map[string]any{"live": "new parameter"}); err != nil {
		test.Fatal(err)
	}
	entry["provider_tool_name"] = "changed_live_route"
	if _, err := engine.runTool(context.Background(), "generate_image", map[string]any{"live": "new parameter"}); err != nil {
		test.Fatal(err)
	}
	if len(fixture.calls) != 4 || *accepted != 2 || fixture.calls[3].Body["provider_tool_name"] != "changed_live_route" {
		test.Fatalf("catalog cached or route ignored: %#v", fixture.calls)
	}
	if fixture.calls[0].Budget > 30*time.Second || fixture.calls[1].Budget < 9*time.Minute {
		test.Fatal("catalog/generation deadline parity lost")
	}
}

func TestToolsSchema955AndNoPaidCallOnInvalidArguments(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test, "generate_image")
	schema, err := os.ReadFile("testdata/tools_generate_image_955.json")
	if err != nil {
		test.Fatal(err)
	}
	fixture.catalog = toolsCatalog(toolsEntry("generate_image", string(schema)))
	for _, wrong := range []string{"resolution", "input_urls"} {
		arguments := map[string]any{"requests": []any{map[string]any{"prompt": "fixture", wrong: "invalid"}}}
		if _, err := engine.runTool(context.Background(), "generate_image", arguments); err == nil {
			test.Fatalf("accepted obsolete argument %s", wrong)
		}
	}
	if *accepted != 0 || len(fixture.calls) != 2 {
		test.Fatal("invalid arguments accepted or reached paid endpoint")
	}
	arguments := map[string]any{"requests": []any{map[string]any{"prompt": "fixture", "area": "2K", "n": float64(1), "output_format": "png", "reference_images": []any{map[string]any{"url": "https://example.test/image.png"}}}}}
	if _, err := engine.runTool(context.Background(), "generate_image", arguments); err != nil {
		test.Fatal(err)
	}
	if *accepted != 1 || len(fixture.calls) != 4 {
		test.Fatal("valid live schema955 input did not invoke once")
	}
}

func TestToolsRejectUnsupportedLiveConstraintsBeforePaidCall(test *testing.T) {
	for _, schema := range []string{
		`{"type":"object","$ref":"#/definitions/input"}`,
		`{"type":"object","properties":{"x":{"type":"string","pattern":"^x$"}}}`,
		`{"type":"object","properties":{"x":{"type":"array","uniqueItems":true}}}`,
		`{"type":"object","required":"x"}`,
		`{"type":"object","items":[]}`,
	} {
		test.Run(schema, func(test *testing.T) {
			fixture, engine, accepted := toolsFixture(test, "generate_image")
			fixture.catalog = toolsCatalog(toolsEntry("generate_image", schema))
			if _, err := engine.runTool(context.Background(), "generate_image", map[string]any{}); err == nil {
				test.Fatal("unsupported constraint was ignored")
			}
			if *accepted != 0 || len(fixture.calls) != 1 {
				test.Fatal("unvalidated request reached paid endpoint")
			}
		})
	}
}

func TestToolsBundledSchemasAndConditionalValidation(test *testing.T) {
	definitions := matrixDefinitions()
	if len(definitions) != 16 || len(toolDefinitions()) != 20 {
		test.Fatal("named tool inventory changed")
	}
	for name, definition := range definitions {
		if err := supportedSchema(definition.Schema, 0); err != nil {
			test.Fatalf("%s: %v", name, err)
		}
	}
	schema, err := parseToolSchema(`{"type":"object","properties":{"model":{"enum":["a","b"]},"n":{"type":"integer"}},"required":["model","n"],"allOf":[{"if":{"properties":{"model":{"const":"a"}}},"then":{"properties":{"n":{"minimum":4,"maximum":15}}},"else":{"properties":{"n":{"enum":[6,10]}}}}]}`)
	if err != nil {
		test.Fatal(err)
	}
	for _, entry := range []struct {
		model string
		count float64
		valid bool
	}{{"a", 4, true}, {"a", 3, false}, {"b", 6, true}, {"b", 7, false}, {"b", 10, true}} {
		err := validateToolArguments(schema, map[string]any{"model": entry.model, "n": entry.count})
		if (err == nil) != entry.valid {
			test.Fatalf("%+v: %v", entry, err)
		}
	}
	deep := map[string]any{"type": "object"}
	for depth := 0; depth < 70; depth++ {
		deep = map[string]any{"allOf": []any{deep}}
	}
	if supportedSchema(deep, 0) == nil {
		test.Fatal("unbounded schema recursion")
	}
}

func TestToolsCatalogMalformedPartialAndMatrixBoundary(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test, "get_asset_url")
	for _, malformed := range []any{nil, map[string]any{}, map[string]any{"tools": []any{}, "partial": "false", "provider_failures": []any{}}, toolsCatalog(toolsEntry("x", "{}"), toolsEntry("x", "{}"))} {
		fixture.catalog = malformed
		if _, err := engine.runTool(context.Background(), "list_tools", map[string]any{}); err == nil {
			test.Fatal("malformed catalog became an empty success")
		}
	}
	fixture.calls = nil
	for _, name := range []string{"tool_schema", "call_tool"} {
		arguments := map[string]any{"tool_name": "connector__email__send"}
		if name == "call_tool" {
			arguments["arguments"] = map[string]any{}
		}
		if _, err := engine.runTool(context.Background(), name, arguments); err == nil {
			test.Fatal("non-Matrix integration accessible")
		}
	}
	if len(fixture.calls) != 0 || *accepted != 0 {
		test.Fatal("non-Matrix request reached transport")
	}
	catalog := toolsCatalog(toolsEntry("get_asset_url", "{}"))
	catalog["partial"] = true
	catalog["provider_failures"] = []any{map[string]any{"provider": "matrix", "code": "offline", "message": "offline"}}
	fixture.catalog = catalog
	value, err := engine.runTool(context.Background(), "list_tools", map[string]any{"keyword": "ASSET"})
	if err != nil || object(value)["partial"] != true || len(object(value)["tools"].([]map[string]any)) != 1 {
		test.Fatalf("partial catalog not preserved: %v %v", value, err)
	}
}

func TestToolsResultFailuresAndTransportAreNeverReplayed(test *testing.T) {
	for _, result := range []any{map[string]any{"is_error": true, "result_json": "{}"}, map[string]any{"is_error": "false", "result_json": "{}"}, map[string]any{}, map[string]any{"result_json": "{"}, toolsSuccess(map[string]any{"code": 17})} {
		fixture, engine, accepted := toolsFixture(test, "generate_image")
		fixture.respond = func(context.Context, string, map[string]any) (any, error) { return result, nil }
		if _, err := engine.runTool(context.Background(), "generate_image", map[string]any{}); err == nil {
			test.Fatal("business/malformed result reported success")
		}
		if len(fixture.calls) != 2 || *accepted != 1 {
			test.Fatal("paid operation replayed or not accepted")
		}
	}
	fixture, engine, accepted := toolsFixture(test, "generate_image")
	fixture.respond = func(context.Context, string, map[string]any) (any, error) {
		return nil, &APIError{Status: 401, Code: "http"}
	}
	if _, err := engine.runTool(context.Background(), "generate_image", map[string]any{}); err == nil || len(fixture.calls) != 2 || *accepted != 1 {
		test.Fatal("401 was hidden, refreshed, or replayed")
	}
}

func TestToolsCancellationAndAcceptanceFailurePreventSubmission(test *testing.T) {
	fixture, engine, _ := toolsFixture(test, "generate_image")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.runTool(cancelled, "generate_image", map[string]any{}); !errors.Is(err, context.Canceled) || len(fixture.calls) != 0 {
		test.Fatal("pre-cancelled operation reached transport")
	}
	engine.accept = func(context.Context) error { return errors.New("receipt refused") }
	if _, err := engine.runTool(context.Background(), "generate_image", map[string]any{}); err == nil || len(fixture.calls) != 1 {
		test.Fatal("operation sent without acceptance receipt")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.accept = func(context.Context) error { cancel(); return nil }
	if _, err := engine.runTool(ctx, "generate_image", map[string]any{}); !errors.Is(err, context.Canceled) || len(fixture.calls) != 2 {
		test.Fatal("cancel after catalog still submitted a paid request")
	}
}

func TestToolsRoutingAssertionsCannotSwitchAccount(test *testing.T) {
	call := sdk.Call{Scope: sdk.Scope{Provider: Global, Account: "exact-id"}}
	input := map[string]any{"site": Global, "account": "exact-id", "query": "hello"}
	cleaned, err := selectedToolArguments(call, input)
	if err != nil || !reflect.DeepEqual(cleaned, map[string]any{"query": "hello"}) || len(input) != 3 {
		test.Fatal("routing not removed without mutating caller arguments")
	}
	for _, arguments := range []map[string]any{{"site": China}, {"account": "another"}, {"account": ""}, {"site": 7}, {"_magpie": map[string]any{}}} {
		if _, err := selectedToolArguments(call, arguments); err == nil {
			test.Fatal("routing override accepted")
		}
	}
}

func TestToolsSchemaInspectionAndGenericInvocation(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test, "new_tool")
	entry := toolsEntry("new_tool", `{"type":"object","properties":{"prompt":{"type":"string","pattern":"^x$"}}}`)
	entry["output_schema_json"] = "not JSON"
	fixture.catalog = toolsCatalog(entry)
	value, err := engine.runTool(context.Background(), "tool_schema", map[string]any{"tool_name": matrixPrefix + "new_tool"})
	if err != nil || object(object(value)["output_schema"])["malformed"] != "not JSON" || *accepted != 0 {
		test.Fatalf("schema introspection unavailable: %v %v", value, err)
	}
	arguments := map[string]any{"tool_name": matrixPrefix + "new_tool", "arguments": map[string]any{"prompt": "x"}}
	if _, err := engine.runTool(context.Background(), "call_tool", arguments); err == nil || *accepted != 0 {
		test.Fatal("unsupported live pattern silently accepted")
	}
	entry["input_schema_json"] = `{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"]}`
	if _, err := engine.runTool(context.Background(), "call_tool", arguments); err != nil || *accepted != 1 {
		test.Fatalf("generic Matrix call failed: %v", err)
	}
	entry["input_schema_json"] = ""
	if _, err := engine.runTool(context.Background(), "tool_schema", map[string]any{"tool_name": matrixPrefix + "new_tool"}); err == nil {
		test.Fatal("absent account schema was fabricated by inspection")
	}
}

func TestToolsSchemaCombinationAndNestedBounds(test *testing.T) {
	cases := []struct {
		schema string
		value  string
		valid  bool
	}{
		{`{"type":"object","properties":{"x":{"anyOf":[{"type":"string"},{"type":"integer"}]}}}`, `{"x":4}`, true},
		{`{"type":"object","properties":{"x":{"oneOf":[{"type":"number"},{"type":"integer"}]}}}`, `{"x":4}`, false},
		{`{"type":"object","not":{"required":["x","y"]}}`, `{"x":1,"y":2}`, false},
		{`{"type":"object","additionalProperties":{"type":"string","minLength":2,"maxLength":2}}`, `{"x":"😀"}`, true},
		{`{"type":"object","additionalProperties":false}`, `{"x":1}`, false},
		{`{"type":"object","properties":{"x":{"type":"array","minItems":1,"maxItems":2,"items":{"type":"integer","minimum":1,"maximum":3}}}}`, `{"x":[1,3]}`, true},
		{`{"type":"object","properties":{"x":{"type":"array","minItems":1,"maxItems":2,"items":{"type":"integer","minimum":1,"maximum":3}}}}`, `{"x":[0]}`, false},
		{`{"type":"object","required":["x"]}`, `{"x":null}`, false},
		{`{"type":"object","properties":{"x":{"type":["null","string"]}}}`, `{"x":null}`, true},
	}
	for _, entry := range cases {
		schema, err := parseToolSchema(entry.schema)
		if err != nil {
			test.Fatal(err)
		}
		var value any
		if err := json.Unmarshal([]byte(entry.value), &value); err != nil {
			test.Fatal(err)
		}
		if err := validateToolArguments(schema, value); (err == nil) != entry.valid {
			test.Fatalf("%s on %s: %v", entry.schema, entry.value, err)
		}
	}
}
