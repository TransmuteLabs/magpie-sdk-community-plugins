package zcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/history"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func toolValue(t *testing.T, frame sdk.Frame, target any) {
	t.Helper()
	var result sdk.ToolResult
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.IsError ||
		json.Unmarshal(result.Content, &blocks) != nil || len(blocks) != 1 || blocks[0].Type != "text" ||
		json.Unmarshal([]byte(blocks[0].Text), target) != nil {
		t.Fatalf("invalid native tool result: %+v", frame.Error)
	}
}

func TestAllHeadlessToolsDeclareOwnerExposureAndHostSelection(t *testing.T) {
	manifest := Manifest()
	if len(manifest.Tools) != 9 || !manifest.Contract.Has(sdk.ToolProvider) {
		t.Fatal("headless tools were not registered through the real SDK contract")
	}
	if len(manifest.Sessions) != 2 || !manifest.Contract.Has(sdk.SessionMigration) ||
		manifest.Sessions[0].ID != historyAdapterID || manifest.Sessions[0].From != "" || manifest.Sessions[0].To != "" {
		t.Fatal("offline converter falsely advertised an automatic agent reader/resume route")
	}
	if manifest.Sessions[1].ID != sqliteAdapterID || manifest.Sessions[1].From != "" || manifest.Sessions[1].To != "" {
		t.Fatal("SQLite converter falsely advertised an automatic agent reader/resume route")
	}
	for _, tool := range manifest.Tools {
		var schema map[string]any
		if tool.Exposure != "local-owner" || json.Unmarshal(tool.InputSchema, &schema) != nil ||
			mapObject(schema["properties"])["_magpie"] == nil {
			t.Fatal("a tool omitted owner-only exposure or the host-consumed selection declaration")
		}
	}
}

func TestOfficialMCPToolUsesSeparateSelectedJWTChannel(t *testing.T) {
	for _, kind := range []string{"individual", "team"} {
		t.Run(kind, func(t *testing.T) {
			account := nativeCredentials()
			if kind == "team" {
				account.Kind, account.Site, account.Organization, account.Project, account.Product =
					"team", "bigmodel", "selected-org", "selected-project", "selected-product"
				account.Location = &ProjectLocation{OrganizationID: account.Organization, ProjectID: account.Project, APIKeyID: "selected-key"}
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/oauth/profile" {
					profile(writer, request, account.UserID)
					return
				}
				if request.URL.Path != "/api/v1/mcp/usage" || request.Method != http.MethodGet ||
					request.Header.Get("Authorization") != "Bearer "+account.SessionToken ||
					request.Header.Get("X-Bigmodel-Authorization") != "Bearer "+account.BusinessToken ||
					request.Header.Get("X-Coding-Plan-Api-Key") != "" || request.Header.Get("X-Api-Key") != "" {
					t.Error("MCP tool mixed session/business/PAT credential channels")
				}
				target := "PERSONAL"
				if kind == "team" {
					target = "TEAM"
					if request.Header.Get("Bigmodel-Organization") != account.Organization ||
						request.Header.Get("Bigmodel-Project") != account.Project {
						t.Error("MCP Team credential scope lost its atomic org/project pair")
					}
				}
				if request.Header.Get("Bigmodel-Target-Type") != target {
					t.Error("MCP selected target type changed")
				}
				fmt.Fprintf(writer, `{"code":0,"data":{"server_time":%d,"next_refresh_at":%d,"total_usage":{"used":9,"limit":10,"remaining":8}}}`,
					fixtureNow.Unix(), fixtureNow.Unix()+60)
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			frame, _ := h.call(sdk.MethodToolCall, scope(account),
				sdk.ToolRequest{ID: "zcode_mcp_usage", Arguments: raw(struct{}{})}, raw(account))
			var observation mcpObservation
			toolValue(t, frame, &observation)
			if !observation.Informational || !observation.Window.Aside || observation.Window.AsOf == 0 ||
				observation.Window.Used != 20 || observation.ServerTime != fixtureNow.UnixMilli() ||
				observation.ProviderFamily != account.Site || h.invalidations != 0 {
				t.Fatal("MCP quota became ordinary inference allowance or lost selected family")
			}
			for _, secret := range []string{account.SessionToken, account.ProjectToken, account.BusinessToken, account.UserID} {
				if secret != "" && strings.Contains(string(frame.Result), secret) {
					t.Fatal("tool result exposed selected credential material or vendor identity")
				}
			}
		})
	}
}

func TestExecutionListToolReturnsReferencesWithoutVendorTicketCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			io.WriteString(writer, `{"ticket_id":"sensitive-vendor-ticket","task_id":"fixture-session","state":"ready"}`)
		case "/api/v1/off-peak/anthropic/v1/messages":
			io.WriteString(writer, `{"type":"message","content":[]}`)
		default:
			t.Errorf("unexpected execution tool fixture: owner=%s method=%s host=%q path=%q remote=%q agent=%q authorization=%t",
				t.Name(), request.Method, request.Host, request.URL.Path, request.RemoteAddr, request.UserAgent(), request.Header.Get("Authorization") != "")
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "controller")})
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), sdk.ExecuteRequest{
		Protocol: "anthropic", Model: "GLM-5.3-OffPeak", Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`),
	}, raw(account))
	if frame.Error != nil {
		t.Fatalf("execution tool fixture: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodToolCall, scope(account),
		sdk.ToolRequest{ID: "zcode_execution_list", Arguments: raw(struct{}{})}, raw(account))
	var entries []executionView
	toolValue(t, frame, &entries)
	if len(entries) != 1 || entries[0].Reference == "" || entries[0].Ticket != "" || entries[0].Card != "" ||
		strings.Contains(string(frame.Result), "sensitive-vendor-ticket") {
		t.Fatal("execution tool exposed vendor admission credentials instead of a controller reference")
	}
}

func TestOfficialMCPQuotaRejectsMismatchedAuthenticatedUser(t *testing.T) {
	var quotas atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/oauth/profile" {
			profile(writer, request, "another-user")
			return
		}
		quotas.Add(1)
		t.Error("another vendor principal reached the selected MCP observation")
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodToolCall, scope(account),
		sdk.ToolRequest{ID: "zcode_mcp_usage", Arguments: raw(struct{}{})}, raw(account))
	if frame.Error == nil || frame.Error.Code != "permission_denied" || quotas.Load() != 0 {
		t.Fatal("MCP quota accepted an unverified selected-user identity")
	}
}

func TestMCPMissingPaidMaterialAndArgumentScopeNeverFallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		t.Error("unauthorized MCP tool reached a vendor endpoint")
		writer.WriteHeader(403)
	}))
	defer server.Close()
	for _, kind := range []string{"start", "legacy-key", "individual"} {
		h := newHarness(t, endpointOptions(server))
		account := fixtureCredentials(kind)
		frame, _ := h.call(sdk.MethodToolCall, scope(account),
			sdk.ToolRequest{ID: "zcode_mcp_usage", Arguments: raw(struct{}{})}, raw(account))
		if frame.Error == nil || h.proxies != 0 || requests.Load() != 0 {
			t.Fatal("MCP tool borrowed credentials, refreshed or silently fell back to free scope")
		}
	}
	h := newHarness(t, endpointOptions(server))
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodToolCall, scope(account), sdk.ToolRequest{ID: "zcode_mcp_usage",
		Arguments: raw(map[string]any{"_magpie": map[string]string{"provider": "zcode", "account": "other"}})}, raw(account))
	if frame.Error == nil || h.proxies != 0 {
		t.Fatal("module accepted caller arguments as host credential authority")
	}
}

func TestAccountStatusAndOrdinaryUsageToolsAreInformational(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":25}]}}`)
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := fixtureCredentials("individual")
	frame, _ := h.call(sdk.MethodToolCall, scope(account),
		sdk.ToolRequest{ID: "zcode_account_status", Arguments: raw(struct{}{})}, raw(account))
	var status map[string]any
	toolValue(t, frame, &status)
	if status["account"] != account.AccountID() || status["kind"] != "individual" || h.proxies != 0 ||
		strings.Contains(string(frame.Result), account.ProjectToken) {
		t.Fatal("sanitized account tool dispatched or exposed a credential")
	}
	frame, _ = h.call(sdk.MethodToolCall, scope(account),
		sdk.ToolRequest{ID: "zcode_usage", Arguments: raw(struct{}{})}, raw(account))
	var result struct {
		Informational bool            `json:"informational"`
		Report        sdk.UsageResult `json:"report"`
	}
	toolValue(t, frame, &result)
	if !result.Informational || result.Report.Revision != "" || result.Report.AsOf == 0 ||
		len(result.Report.Windows) != 1 || result.Report.Windows[0].AsOf == 0 ||
		h.invalidations != 0 || h.accepted.Load() {
		t.Fatal("ordinary quota tool claimed the provider.usage acknowledgment protocol")
	}
}

func TestOfflineHistoryToolsUseOnlyExplicitDocumentBytes(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	body := []byte("{\"type\":\"assistant\",\"sessionId\":\"fixture\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"signature\":\"opaque\"}]}}\n")
	frame, _ := h.call(sdk.MethodToolCall, sdk.Scope{}, sdk.ToolRequest{ID: "zcode_history_inspect",
		Arguments: raw(map[string]any{"format": history.NativeJSONL, "body": body})}, nil)
	var inspected struct {
		Summary          history.Summary `json:"summary"`
		ResumeAuthorized bool            `json:"resumeAuthorized"`
	}
	toolValue(t, frame, &inspected)
	if inspected.Summary.Records != 1 || !inspected.Summary.HasThinking || inspected.ResumeAuthorized || h.proxies != 0 {
		t.Fatal("offline inspection invented executable admission or read a live source")
	}
	frame, _ = h.call(sdk.MethodToolCall, sdk.Scope{}, sdk.ToolRequest{ID: "zcode_history_convert",
		Arguments: raw(map[string]any{"format": history.NativeJSONL, "to": history.Bundle, "body": body})}, nil)
	var converted struct {
		Body            []byte `json:"body"`
		SourcePreserved bool   `json:"sourcePreserved"`
	}
	toolValue(t, frame, &converted)
	document, err := history.Read(history.Bundle, converted.Body)
	if err != nil || !converted.SourcePreserved || string(document.Source()) != string(body) || h.proxies != 0 {
		t.Fatal("offline conversion changed signed source or fetched external state")
	}
}

func TestMalformedMCPUsageCannotInventQuota(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"code":403,"data":{}}`,
		`{"code":0,"data":{"server_time":1}}`,
		`{"code":0,"data":{"server_time":1,"total_usage":{"used":0,"limit":10,"remaining":11}}}`,
		`{"code":0,"data":{"server_time":1,"total_usage":{"used":"0","limit":10,"remaining":1}}}`,
		`{"code":0,"data":{"server_time":1,"next_refresh_at":1e300,"total_usage":{"used":0,"limit":10,"remaining":1}}}`,
	} {
		if _, _, err := projectMCPUsage([]byte(body), fixtureNow.UnixMilli()); err == nil {
			t.Fatal("malformed MCP aggregate became an allowance")
		}
	}
	window, _, err := projectMCPUsage([]byte(`{"code":0,"data":{"server_time":1,"total_usage":{"used":0,"limit":0,"remaining":0}}}`), fixtureNow.UnixMilli())
	if err != nil || !window.Unknown || !window.Aside {
		t.Fatal("uncapped MCP data became a known inference allowance")
	}
}
