package zcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/mcpwire"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const imageSearchEndpoint = "/api/v1/mcp/server/image_search"
const imageSearchRemoteTool = "search_image"

type remoteMCPSession struct {
	owner    *module
	endpoint string
	proxy    string
	headers  http.Header
	session  string
}

func (session *remoteMCPSession) post(ctx context.Context, body []byte, id string, initialize bool) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, session.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header = session.headers.Clone()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("User-Agent", "ZCode/"+ClientVersion)
	if !initialize {
		request.Header.Set("MCP-Protocol-Version", mcpwire.Version)
	}
	if session.session != "" {
		request.Header.Set("Mcp-Session-Id", session.session)
	}
	response, err := send(ctx, session.owner.options.HTTPFactory, session.proxy, request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if id == "" {
		if response.StatusCode != http.StatusAccepted {
			return nil, mcpwire.ErrProtocol
		}
		value, err := io.ReadAll(io.LimitReader(response.Body, 1))
		if err != nil || len(value) != 0 {
			return nil, mcpwire.ErrProtocol
		}
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP HTTP status %d", response.StatusCode)
	}
	next, err := mcpwire.Session(response.Header.Values("Mcp-Session-Id"))
	if err != nil {
		return nil, err
	}
	if initialize {
		session.session = next
	} else if next != "" && next != session.session {
		return nil, mcpwire.ErrProtocol
	}
	if versions := response.Header.Values("MCP-Protocol-Version"); len(versions) > 0 &&
		(len(versions) != 1 || versions[0] != mcpwire.Version) {
		return nil, mcpwire.ErrProtocol
	}
	if id == "" {
		return nil, nil
	}
	return mcpwire.Read(response.Body, response.Header.Get("Content-Type"), id)
}

func (session *remoteMCPSession) rpc(ctx context.Context, id, method string, params any, initialize bool) (json.RawMessage, error) {
	body, err := mcpwire.Request(id, method, params)
	if err != nil {
		return nil, err
	}
	return session.post(ctx, body, id, initialize)
}

func (session *remoteMCPSession) notify(ctx context.Context, method string, params any) error {
	body, err := mcpwire.Notification(method, params)
	if err != nil {
		return err
	}
	_, err = session.post(ctx, body, "", false)
	return err
}

func validateMCPInitialize(body []byte) error {
	value, err := protocol.ReadObject(body)
	capabilities := mapObject(value["capabilities"])
	info := mapObject(value["serverInfo"])
	name, nameValid := info["name"].(string)
	version, versionValid := info["version"].(string)
	if err != nil || value["protocolVersion"] != mcpwire.Version ||
		mapObject(capabilities["tools"]) == nil || !nameValid || !versionValid ||
		name == "" || len(name) > 256 || len(version) > 128 {
		return mcpwire.ErrProtocol
	}
	return nil
}

func validateSearchSchema(schema map[string]any) error {
	properties := mapObject(schema["properties"])
	query := mapObject(properties["query"])
	if schema["type"] != "object" || query["type"] != "string" {
		return unsupported("The fixed remote tool requires an unsupported query schema")
	}
	if required, present := schema["required"]; present {
		values, valid := required.([]any)
		if !valid {
			return mcpwire.ErrProtocol
		}
		for _, value := range values {
			if value != "query" {
				return unsupported("The remote tool now requires arguments outside this fixed declaration")
			}
		}
	}
	return nil
}

func (session *remoteMCPSession) verifyTool(ctx context.Context, prefix string) error {
	cursor := ""
	seen := map[string]bool{}
	found, count := false, 0
	for page := 0; page < 4; page++ {
		params := map[string]string{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		body, err := session.rpc(ctx, fmt.Sprintf("%s-list-%d", prefix, page), "tools/list", params, false)
		if err != nil {
			return err
		}
		value, err := protocol.ReadObject(body)
		tools, valid := value["tools"].([]any)
		if err != nil || !valid {
			return mcpwire.ErrProtocol
		}
		count += len(tools)
		if count > 128 {
			return mcpwire.ErrLimit
		}
		for _, value := range tools {
			tool := mapObject(value)
			name, valid := tool["name"].(string)
			if !valid || name == "" || len(name) > 256 {
				return mcpwire.ErrProtocol
			}
			if name == imageSearchRemoteTool {
				if found {
					return mcpwire.ErrProtocol
				}
				found = true
				if err := validateSearchSchema(mapObject(tool["inputSchema"])); err != nil {
					return err
				}
			}
		}
		next, present := value["nextCursor"]
		if !present {
			if !found {
				return unsupported("The fixed search_image tool is not advertised by this endpoint")
			}
			return nil
		}
		cursor, valid = next.(string)
		if !valid || !safeText(cursor) || len(cursor) > 256 || seen[cursor] {
			return mcpwire.ErrProtocol
		}
		seen[cursor] = true
	}
	return mcpwire.ErrLimit
}

func projectMCPToolResult(body []byte) (sdk.ToolResult, error) {
	value, err := protocol.ReadObject(body)
	if err != nil {
		return sdk.ToolResult{}, err
	}
	contents, valid := value["content"].([]any)
	if !valid || len(contents) > 1024 {
		return sdk.ToolResult{}, mcpwire.ErrProtocol
	}
	isError := false
	if flag, present := value["isError"]; present {
		isError, valid = flag.(bool)
		if !valid {
			return sdk.ToolResult{}, mcpwire.ErrProtocol
		}
	}
	for _, value := range contents {
		content := mapObject(value)
		if name, valid := content["type"].(string); !valid || name == "" {
			return sdk.ToolResult{}, mcpwire.ErrProtocol
		}
	}
	var raw map[string]json.RawMessage
	json.Unmarshal(body, &raw)
	var blocks []json.RawMessage
	json.Unmarshal(raw["content"], &blocks)
	delete(raw, "content")
	delete(raw, "isError")
	if len(raw) > 0 {
		extras, err := json.Marshal(map[string]any{"zcodeMCPResultFields": raw})
		if err != nil {
			return sdk.ToolResult{}, err
		}
		block, _ := json.Marshal(map[string]string{"type": "text", "text": string(extras)})
		blocks = append(blocks, block)
	}
	content, err := json.Marshal(blocks)
	return sdk.ToolResult{Content: content, IsError: isError}, err
}

func remoteMCPFailure(err error, dispatched bool) error {
	outcome := "not-started"
	if dispatched {
		outcome = "uncertain"
	}
	var rpc *mcpwire.RPCError
	if errors.As(err, &rpc) {
		return &sdk.Error{Code: "mcp_rpc_error", Message: fmt.Sprintf("Official MCP returned JSON-RPC code %d; no retry was performed", rpc.Code), Outcome: outcome}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &sdk.Error{Code: "cancelled", Message: "Official MCP operation was cancelled; no tool request was repeated", Outcome: outcome}
	}
	return &sdk.Error{Code: "mcp_protocol", Message: "Official MCP did not confirm the fixed protocol operation; no redirect, fallback or retry was performed", Outcome: outcome}
}

func (owner *module) imageSearch(ctx context.Context, call sdk.Call, stream *sdk.Stream, query string) (any, error) {
	if strings.TrimSpace(query) == "" || len(query) > 4096 || !utf8.ValidString(query) ||
		strings.ContainsRune(query, 0) || call.Scope.Operation == "" {
		return nil, invalid("Fixed image search requires a bounded query and a host-owned operation intent")
	}
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	headers, err := owner.officialMCPHeaders(account)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if _, _, err := owner.principal(ctx, stream, account, ""); err != nil {
		return nil, err
	}
	endpoint := owner.zcodeOrigin() + imageSearchEndpoint
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return nil, err
	}
	session := remoteMCPSession{owner: owner, endpoint: endpoint, proxy: proxy.URL, headers: headers}
	prefix, err := nonce()
	if err != nil {
		return nil, err
	}
	initialized, err := session.rpc(ctx, prefix+"-initialize", "initialize", map[string]any{
		"protocolVersion": mcpwire.Version, "capabilities": map[string]any{},
		"clientInfo": map[string]string{"name": "magpie-zcode", "version": Version},
	}, true)
	if err == nil {
		err = validateMCPInitialize(initialized)
	}
	if err == nil {
		err = session.notify(ctx, "notifications/initialized", nil)
	}
	if err == nil {
		err = session.verifyTool(ctx, prefix)
	}
	if err != nil {
		return nil, remoteMCPFailure(err, false)
	}
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, uncertain("MCP tool acceptance was not acknowledged; no tool request was dispatched")
	}
	id := prefix + "-call"
	result, err := session.rpc(ctx, id, "tools/call", map[string]any{
		"name": imageSearchRemoteTool, "arguments": map[string]string{"query": query},
	}, false)
	if err != nil {
		if ctx.Err() != nil {
			cancelCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			session.notify(cancelCtx, "notifications/cancelled", map[string]string{
				"requestId": id, "reason": "Client cancelled this operation",
			})
			stop()
			err = ctx.Err()
		}
		return nil, remoteMCPFailure(err, true)
	}
	projected, err := projectMCPToolResult(result)
	if err != nil {
		return nil, remoteMCPFailure(err, true)
	}
	return projected, nil
}
