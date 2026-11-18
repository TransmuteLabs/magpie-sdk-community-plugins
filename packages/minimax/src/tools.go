package minimax

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const matrixPrefix = "connector__matrix__"
const matrixListPath = "/minimax-cloud/api/v1/connectors/connected-tools/list"
const matrixCallPath = "/minimax-cloud/api/v1/connectors/tools/call"
const toolResponseLimit = 4 << 20

//go:embed tools_matrix.json
var bundledMatrix []byte

type toolDefinition struct {
	Description string         `json:"description"`
	Schema      map[string]any `json:"input_schema"`
}

func matrixDefinitions() map[string]toolDefinition {
	var bundle struct {
		Tools map[string]toolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(bundledMatrix, &bundle); err != nil {
		panic(err)
	}
	return bundle.Tools
}

// All requests use the same exact-account client supplied by the core owner.
// The engine does not read account stores or refresh/replay any request.
type matrixTransport interface {
	JSON(context.Context, string, any, int64) (json.RawMessage, error)
	Transfer(context.Context, string, string, []byte, http.Header, int64) (ByteResponse, error)
}
type matrixEntry struct {
	Provider     string `json:"provider"`
	ProviderTool string `json:"provider_tool_name"`
	RuntimeTool  string `json:"runtime_tool_name"`
	Description  string `json:"description"`
	InputSchema  string `json:"input_schema_json"`
	OutputSchema any    `json:"output_schema_json,omitempty"`
}
type matrixCatalog struct {
	Tools    []matrixEntry
	Partial  bool
	Failures []map[string]any
}
type matrixEngine struct {
	client matrixTransport
	accept func(context.Context) error
}

func toolError(message string) error {
	return &sdk.Error{Code: "upstream", Message: message}
}
func toolBudget(name string) time.Duration {
	if contains([]string{"generate_image", "submit_video_generation", "synthesize_speech", "batch_text_to_audio", "batch_text_to_music", "enhance_image", "remove_image_background", "describe_images", "videos_understand", "audios_understand", "listen_audio", "search_images", "reverse_image_search"}, strings.TrimPrefix(name, matrixPrefix)) {
		return 10 * time.Minute
	}
	return 55 * time.Second
}

func (engine matrixEngine) catalog(ctx context.Context) (matrixCatalog, error) {
	var result matrixCatalog
	if err := ctx.Err(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := engine.client.JSON(ctx, matrixListPath, map[string]any{}, toolResponseLimit)
	if err != nil {
		return result, err
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return result, toolError("Malformed connector catalog")
	}
	entries, ok := payload["tools"].([]any)
	if !ok {
		return result, toolError("Connector catalog has no tools array")
	}
	failures, ok := payload["provider_failures"].([]any)
	if !ok {
		return result, toolError("Connector catalog has no provider failures array")
	}
	result.Partial, ok = payload["partial"].(bool)
	if !ok {
		return result, toolError("Connector catalog has invalid partial flag")
	}
	result.Tools = []matrixEntry{}
	result.Failures = []map[string]any{}
	for _, value := range failures {
		failure := object(value)
		if text(failure["provider"]) == "" || text(failure["code"]) == "" || text(failure["message"]) == "" {
			return result, toolError("Malformed connector provider failure")
		}
		result.Failures = append(result.Failures, failure)
	}
	seen := map[string]bool{}
	for _, value := range entries {
		entryFields := object(value)
		schema, ok := entryFields["input_schema_json"].(string)
		entry := matrixEntry{Provider: text(entryFields["provider"]), ProviderTool: text(entryFields["provider_tool_name"]), RuntimeTool: text(entryFields["runtime_tool_name"]), Description: text(entryFields["description"]), InputSchema: schema, OutputSchema: entryFields["output_schema_json"]}
		if !ok || entry.Provider == "" || entry.ProviderTool == "" || entry.RuntimeTool == "" || len(entry.RuntimeTool) > 300 || seen[entry.RuntimeTool] {
			return result, toolError("Malformed or duplicate connector route")
		}
		seen[entry.RuntimeTool] = true
		result.Tools = append(result.Tools, entry)
	}
	return result, nil
}

func (catalog matrixCatalog) find(name string) (matrixEntry, error) {
	for _, entry := range catalog.Tools {
		if entry.RuntimeTool == name {
			return entry, nil
		}
	}
	for _, failure := range catalog.Failures {
		if strings.HasPrefix(name, "connector__"+text(failure["provider"])+"__") {
			return matrixEntry{}, toolError("Connector provider is unavailable")
		}
	}
	message := "Connector tool is not connected on the selected account"
	if catalog.Partial {
		message += " (catalog is partial)"
	}
	return matrixEntry{}, toolError(message)
}

func (engine matrixEngine) invoke(ctx context.Context, entry matrixEntry, arguments map[string]any, budget time.Duration) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(arguments)
	if err != nil {
		return nil, invalid("Invalid connector arguments")
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	result, err := engine.client.JSON(ctx, matrixCallPath, map[string]any{"provider": entry.Provider, "provider_tool_name": entry.ProviderTool, "runtime_tool_name": entry.RuntimeTool, "arguments_json": string(data)}, toolResponseLimit)
	if err != nil {
		return nil, err
	}
	return connectorResult(ByteResponse{Body: result})
}

func (engine matrixEngine) matrix(ctx context.Context, name string, arguments map[string]any, budget time.Duration) (any, error) {
	catalog, err := engine.catalog(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := catalog.find(matrixPrefix + strings.TrimPrefix(name, matrixPrefix))
	if err != nil {
		return nil, err
	}
	if entry.InputSchema != "" {
		schema, err := parseToolSchema(entry.InputSchema)
		if err != nil {
			return nil, err
		}
		// Native media builds numeric fields as Go integers; validate the JSON
		// representation that the connector receives, as for ordinary tool calls.
		var wireArguments map[string]any
		encoded, err := json.Marshal(arguments)
		if err != nil || json.Unmarshal(encoded, &wireArguments) != nil {
			return nil, invalid("Invalid native media arguments")
		}
		if err := validateToolArguments(schema, wireArguments); err != nil {
			return nil, err
		}
	}
	if budget == 0 {
		budget = toolBudget(name)
	}
	return engine.invoke(ctx, entry, arguments, budget)
}

func toolSchema(properties map[string]any, required ...string) map[string]any {
	needs := make([]any, len(required))
	for index, field := range required {
		needs[index] = field
	}
	return map[string]any{"type": "object", "properties": properties, "required": needs, "additionalProperties": false}
}
func stringSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": float64(1)}
}
func toolDefinitions() map[string]toolDefinition {
	definitions := matrixDefinitions()
	definitions["list_tools"] = toolDefinition{"List the selected account's connected Matrix tools and partial catalog failures.", toolSchema(map[string]any{"keyword": map[string]any{"type": "string"}})}
	definitions["tool_schema"] = toolDefinition{"Read a connected Matrix tool's current input and output schemas.", toolSchema(map[string]any{"tool_name": stringSchema()}, "tool_name")}
	definitions["call_tool"] = toolDefinition{"Call a connected connector__matrix__ tool, validating its arguments against the selected account's current schema.", toolSchema(map[string]any{"tool_name": stringSchema(), "arguments": map[string]any{"type": "object"}}, "tool_name", "arguments")}
	definitions["web_search"] = toolDefinition{"Search MiniMax Code's live web sources, news, videos, places or shopping. Cite returned URLs.", webToolSchema()}
	return definitions
}

func installTools(owner *module, target *sdk.Module) {
	target.Manifest.Contract.Capabilities = append(target.Manifest.Contract.Capabilities, sdk.ToolProvider)
	definitions := toolDefinitions()
	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		definition := definitions[name]
		schema := definition.Schema
		props := object(schema["properties"])
		if props == nil {
			props = map[string]any{}
			schema["properties"] = props
		}
		props["_magpie"] = toolSchema(map[string]any{
			"provider": stringSchema(),
			"account":  stringSchema(),
		}, "provider", "account")
		props["site"] = map[string]any{"type": "string", "enum": []string{China, Global}, "description": "Optional assertion of the host-selected provider. Cannot change the selected account."}
		props["account"] = map[string]any{"type": "string", "minLength": 1, "description": "Optional assertion of the exact host-selected account ID. Cannot select another account."}
		target.Manifest.Tools = append(target.Manifest.Tools, sdk.ToolDescriptor{Name: name, Description: definition.Description, InputSchema: raw(schema)})
	}
	target.Handlers[sdk.MethodToolCall] = owner.toolCall
	installSearch(owner, target)
}

func selectedToolArguments(call sdk.Call, arguments map[string]any) (map[string]any, error) {
	cleaned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		if key == "site" || key == "account" {
			assertion, ok := value.(string)
			want := call.Scope.Provider
			if key == "account" {
				want = call.Scope.Account
			}
			if !ok || assertion == "" || assertion != want {
				return nil, denied("Tool routing must match the exact host-selected account")
			}
			continue
		}
		if key == "_magpie" {
			return nil, invalid("Host selection metadata must not reach tool arguments")
		}
		cleaned[key] = value
	}
	return cleaned, nil
}

func (engine matrixEngine) runTool(ctx context.Context, name string, arguments map[string]any) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	definitions := toolDefinitions()
	definition, ok := definitions[name]
	if !ok {
		return nil, invalid("Unknown MiniMax tool")
	}
	if name == "web_search" {
		if err := validateToolArguments(definition.Schema, arguments); err != nil {
			return nil, err
		}
		if strings.TrimSpace(text(arguments["query"])) == "" {
			return nil, invalid("Search query is required")
		}
		if err := engine.accept(ctx); err != nil {
			return nil, err
		}
		return engine.webSearch(ctx, arguments)
	}
	generic := name == "list_tools" || name == "tool_schema" || name == "call_tool"
	if generic {
		if err := validateToolArguments(definition.Schema, arguments); err != nil {
			return nil, err
		}
	}
	runtimeName := matrixPrefix + name
	if name == "tool_schema" || name == "call_tool" {
		runtimeName = text(arguments["tool_name"])
		if !strings.HasPrefix(runtimeName, matrixPrefix) || runtimeName == matrixPrefix {
			return nil, invalid("Only connector__matrix__ tools are accessible")
		}
	}
	catalog, err := engine.catalog(ctx)
	if err != nil {
		return nil, err
	}
	if name == "list_tools" {
		tools := []map[string]any{}
		keyword := strings.ToLower(text(arguments["keyword"]))
		for _, entry := range catalog.Tools {
			if strings.HasPrefix(entry.RuntimeTool, matrixPrefix) && (keyword == "" || strings.Contains(strings.ToLower(entry.RuntimeTool+" "+entry.Description), keyword)) {
				tools = append(tools, map[string]any{"tool_name": entry.RuntimeTool, "description": entry.Description})
			}
		}
		return map[string]any{"tools": tools, "partial": catalog.Partial, "provider_failures": catalog.Failures}, nil
	}
	entry, err := catalog.find(runtimeName)
	if err != nil {
		return nil, err
	}
	if name == "tool_schema" && entry.InputSchema == "" {
		return nil, toolError("The connector supplied no input schema")
	}
	var schema map[string]any
	if entry.InputSchema != "" {
		schema, err = parseToolSchema(entry.InputSchema)
		if err != nil {
			return nil, err
		}
	} else {
		schema = definition.Schema
		if generic {
			schema = map[string]any{"type": "object"}
		}
	}
	if name == "tool_schema" {
		var output any
		if entry.OutputSchema != nil && entry.OutputSchema != "" {
			data, isString := entry.OutputSchema.(string)
			if !isString || json.Unmarshal([]byte(data), &output) != nil {
				output = map[string]any{"malformed": entry.OutputSchema}
			}
		}
		return map[string]any{"tool_name": runtimeName, "provider": entry.Provider, "description": entry.Description, "input_schema": schema, "output_schema": output, "partial": catalog.Partial}, nil
	}
	budget := toolBudget(name)
	if name == "call_tool" {
		arguments = object(arguments["arguments"])
		budget = 10 * time.Minute
	}
	if err := validateToolArguments(schema, arguments); err != nil {
		return nil, err
	}
	if err := engine.accept(ctx); err != nil {
		return nil, err
	}
	return engine.invoke(ctx, entry, arguments, budget)
}

func (owner *module) toolCall(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ToolRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	var arguments map[string]any
	if err := decode(request.Arguments, &arguments); err != nil {
		return nil, err
	}
	arguments, err := selectedToolArguments(call, arguments)
	if err != nil {
		return nil, err
	}
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, err
	}
	started := time.Now()
	accepted := false
	engine := matrixEngine{client: client, accept: func(ctx context.Context) error {
		if stream == nil {
			return denied("A host acceptance receipt is required")
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return err
		}
		accepted = true
		return nil
	}}
	result, err := engine.runTool(ctx, request.ID, arguments)
	if accepted {
		status := 200
		message := ""
		if err != nil {
			status = toolHTTPStatus(err)
			message = "MiniMax tool failed"
		}
		_, logErr := stream.AppendOwnCall(ctx, sdk.OwnCall{Kind: "tool", Model: request.ID, Status: status, Millis: time.Since(started).Milliseconds(), Error: message, Usage: sdk.Usage{Facts: &sdk.ExecutionUsageFacts{}}})
		if err == nil && logErr != nil {
			return nil, uncertain("Tool completed but its host usage receipt was not confirmed")
		}
	}
	if err != nil {
		if accepted {
			return nil, keepFailureDetails(uncertain("MiniMax tool did not complete; its accepted request must not be replayed"), err)
		}
		return nil, err
	}
	return sdk.ToolResult{Content: raw(result)}, nil
}

func toolHTTPStatus(err error) int {
	var status interface{ HTTPStatus() int }
	if errors.As(err, &status) {
		statusCode := status.HTTPStatus()
		if statusCode >= 400 && statusCode <= 599 {
			return statusCode
		}
	}
	return 502
}
