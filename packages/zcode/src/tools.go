package zcode

import (
	"context"
	"encoding/json"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/history"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func toolDescriptors() []sdk.ToolDescriptor {
	result := []sdk.ToolDescriptor{}
	for _, value := range []struct{ name, description string }{
		{"zcode_account_status", "Inspect sanitized host-selected ZCode credential status without tokens or refresh."},
		{"zcode_usage", "Read informational selected-account quota; this tool cannot acknowledge the host quota barrier."},
		{"zcode_mcp_usage", "Read official MCP quota with selected session/business JWT scope; observational Aside data only."},
		{"zcode_image_search", "Call the fixed official search_image MCP tool using the host-selected paid account; one dispatch, no fallback or retry."},
		{"zcode_execution_list", "List selected-account native execution references without replay or mutation."},
		{"zcode_billing_status", "Read the selected vendor user's billing plan baseline; never claim a plan."},
		{"zcode_reset_status", "Read selected paid reset availability/history; never spend or acknowledge a reset."},
		{"zcode_history_inspect", "Inspect supplied offline native history or CLI diagnostic JSONL; no filesystem access or live resume."},
		{"zcode_history_convert", "Convert supplied history to a source-preserving bundle or restore its original format; no lossy cross-agent conversion."},
	} {
		properties := map[string]any{"_magpie": map[string]any{
			"type": "object", "properties": map[string]any{
				"provider": map[string]string{"type": "string"}, "account": map[string]string{"type": "string"},
			}, "required": []string{"provider", "account"}, "additionalProperties": false,
		}}
		required := []string{}
		if value.name == "zcode_image_search" {
			properties["query"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}
			required = []string{"query"}
		}
		if value.name == "zcode_history_inspect" || value.name == "zcode_history_convert" {
			properties["format"] = map[string]any{"type": "string", "enum": []string{history.NativeJSONL, history.LegacySession, history.CLILogJSONL, history.Bundle}}
			properties["body"] = map[string]any{"type": "string", "contentEncoding": "base64", "maxLength": 1400000}
			required = []string{"format", "body"}
			if value.name == "zcode_history_convert" {
				properties["to"] = map[string]any{"type": "string", "enum": []string{history.NativeJSONL, history.LegacySession, history.CLILogJSONL, history.Bundle}}
				required = append(required, "to")
			}
		}
		schema, _ := json.Marshal(map[string]any{"type": "object", "properties": properties,
			"required": required, "additionalProperties": false})
		result = append(result, sdk.ToolDescriptor{Name: value.name, Description: value.description,
			InputSchema: schema, Exposure: "local-owner"})
	}
	return result
}

func toolContent(value any) (sdk.ToolResult, error) {
	text, err := json.Marshal(value)
	if err != nil {
		return sdk.ToolResult{}, err
	}
	content, err := json.Marshal([]map[string]string{{"type": "text", "text": string(text)}})
	return sdk.ToolResult{Content: content}, err
}

func (owner *module) tool(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ToolRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if len(request.Arguments) > 2<<20 {
		return nil, invalid("Tool arguments exceed their offline bounds")
	}
	if request.ID == "zcode_image_search" {
		var input struct {
			Query string `json:"query"`
		}
		if err := decode(request.Arguments, &input); err != nil {
			return nil, err
		}
		return owner.imageSearch(ctx, call, stream, input.Query)
	}
	if request.ID == "zcode_history_inspect" || request.ID == "zcode_history_convert" {
		var input struct {
			Format string `json:"format"`
			Body   []byte `json:"body"`
			To     string `json:"to,omitempty"`
		}
		var inputErr error
		if request.ID == "zcode_history_inspect" {
			var inspection struct {
				Format string `json:"format"`
				Body   []byte `json:"body"`
			}
			inputErr = decode(request.Arguments, &inspection)
			input.Format, input.Body = inspection.Format, inspection.Body
		} else {
			inputErr = decode(request.Arguments, &input)
		}
		if inputErr != nil || len(input.Body) > 1<<20 {
			return nil, invalid("History tools require bounded explicit document bytes and supported formats")
		}
		if request.ID == "zcode_history_convert" {
			body, err := history.Convert(input.Format, input.To, input.Body)
			if err != nil {
				return nil, invalid(err.Error())
			}
			return toolContent(map[string]any{"format": input.To, "body": body, "sourcePreserved": true, "resumeAuthorized": false})
		}
		document, err := history.Read(input.Format, input.Body)
		if err != nil {
			return nil, invalid(err.Error())
		}
		return toolContent(map[string]any{"summary": document.Summary(), "resumeAuthorized": false})
	}
	var empty struct{}
	if err := decode(request.Arguments, &empty); err != nil {
		return nil, invalid("Account tool arguments cannot supply scope, credentials, headers or a host selection")
	}
	account, err := owner.identity(call)
	if err != nil {
		return nil, err
	}
	call.Params = json.RawMessage(`{}`)
	var value any
	switch request.ID {
	case "zcode_account_status":
		value = map[string]any{"account": call.Scope.Account, "generation": call.Scope.Generation,
			"site": account.Site, "kind": account.Kind, "expiresAt": account.ExpiresAt,
			"sessionExpiresAt": account.SessionExpiresAt, "businessExpiresAt": account.BusinessExpiresAt,
			"refreshMaterialPresent": account.BusinessToken != "" || account.OAuthToken != "",
			"hasSavedLocation":       account.Location != nil}
	case "zcode_mcp_usage":
		value, err = owner.officialMCPUsage(ctx, call, stream)
	case "zcode_usage":
		var observation any
		observation, err = owner.usage(ctx, call, stream)
		if err == nil {
			report := observation.(sdk.UsageResult)
			report.Revision, report.AsOf = "", owner.options.Now().UnixMilli()
			for index := range report.Windows {
				report.Windows[index].AsOf = report.AsOf
			}
			value = map[string]any{"informational": true, "report": report}
		}
	case "zcode_execution_list":
		value, err = owner.executionList(ctx, call, stream)
		if err == nil {
			entries := value.([]executionView)
			for index := range entries {
				entries[index].Ticket, entries[index].Card = "", ""
			}
			value = entries
		}
	case "zcode_billing_status":
		value, err = owner.billingStatus(ctx, call, stream)
	case "zcode_reset_status":
		value, err = owner.resetStatus(ctx, call, stream)
	default:
		return nil, unsupported("The requested ZCode tool is not declared")
	}
	if err != nil {
		return nil, err
	}
	return toolContent(value)
}
