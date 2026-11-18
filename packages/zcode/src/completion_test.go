package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type completionVendor struct {
	paidActive   atomic.Bool
	paidList     string
	startCalls   atomic.Int32
	paidCalls    atomic.Int32
	balanceReads atomic.Int32
	requests     chan *http.Request
	balance      string
	balanceAfter string
	answer       string
	media        string
	status       int
}

func newCompletionVendor(t *testing.T) (*completionVendor, *httptest.Server) {
	t.Helper()
	value := &completionVendor{requests: make(chan *http.Request, 32), status: 200, media: "application/json",
		answer: `{"type":"message","model":"glm-5.3","content":[{"type":"text","text":"GLM-5.3 and model are ordinary words"}]}`,
		balance: `{"code":0,"data":{"plans":[{"plan_id":"grant","user_plan_id":"owned","name":"Trust Build","status":"active"}],
			"balances":[{"plan_id":"grant","user_plan_id":"owned","show_name":"GLM-5.3","total_units":100,"remaining_units":75,"capabilities":["model:GLM-5.3"]}]}}`}
	value.paidActive.Store(true)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Date", fixtureNow.Format(http.TimeFormat))
		switch request.URL.Path {
		case "/api/v1/client/configs":
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"configs": map[string]string{"builtin_provider_config_json": server.URL + "/public"},
			}})
		case "/public":
			json.NewEncoder(writer).Encode(modelConfigFixture())
		case "/api/biz/subscription/list":
			if value.paidList != "" {
				io.WriteString(writer, value.paidList)
			} else if value.paidActive.Load() {
				io.WriteString(writer, `{"code":200,"data":[{"status":"VALID","inCurrentPeriod":true,"productName":"GLM Coding Pro"}]}`)
			} else {
				io.WriteString(writer, `{"code":200,"data":[]}`)
			}
		case "/api/v1/zcode-plan/billing/balance":
			if value.balanceReads.Add(1) > 1 && value.balanceAfter != "" {
				io.WriteString(writer, value.balanceAfter)
			} else {
				io.WriteString(writer, value.balance)
			}
		case "/quota/zai":
			io.WriteString(writer, `{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":40}]}}`)
		case "/start", "/zai":
			if request.URL.Path == "/start" {
				value.startCalls.Add(1)
			} else {
				value.paidCalls.Add(1)
			}
			copy := request.Clone(request.Context())
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
			}
			copy.Body = io.NopCloser(strings.NewReader(string(body)))
			value.requests <- copy
			writer.Header().Set("Content-Type", value.media)
			writer.WriteHeader(value.status)
			io.WriteString(writer, value.answer)
		default:
			t.Errorf("unexpected completion fixture path %s", request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return value, server
}

func TestCompletionStartShapeKeepsCallerTextAndSourceCacheBoundaries(t *testing.T) {
	for _, start := range []bool{true, false} {
		t.Run(map[bool]string{true: "start", false: "paid"}[start], func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{FreeFirst: new(false)})
			account := giftAccount()
			model := "GLM-5.3"
			if start {
				model += "-Trial"
			}
			request := giftRequest(model)
			request.Body = raw(map[string]any{"model": model, "system": "  caller's exact instructions\n",
				"messages": []any{
					map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "first", "cache_control": map[string]any{"type": "ephemeral"}}}},
					map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "thinking": "opaque", "signature": "unchanged-signature"}}},
					map[string]any{"role": "user", "content": "next"},
				},
				"tools": []any{map[string]any{"name": "Read", "input_schema": map[string]any{"type": "object"}, "cache_control": map[string]any{"type": "ephemeral"}}},
			})
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
			if frame.Error != nil {
				t.Fatalf("Start shape request: %+v", frame.Error)
			}
			sent := <-vendor.requests
			var body map[string]any
			if json.NewDecoder(sent.Body).Decode(&body) != nil {
				t.Fatal("fixture lost the native body")
			}
			if !start {
				if body["system"] != "  caller's exact instructions\n" || len(body["messages"].([]any)) != 3 {
					t.Fatal("paid request acquired the Start envelope")
				}
				return
			}
			system, valid := body["system"].([]any)
			if !valid || len(system) != 3 || mapObject(system[0])["text"] != "You are ZCode, an interactive agent" ||
				!strings.Contains(mapObject(system[1])["text"].(string), "# Harness") ||
				!strings.HasSuffix(mapObject(system[2])["text"].(string), "  caller's exact instructions\n") {
				t.Fatalf("Start lacks the source-shaped three blocks or loses caller text: %+v", body["system"])
			}
			for _, block := range system {
				if mapObject(mapObject(block)["cache_control"])["type"] != "ephemeral" {
					t.Fatal("Start system cache boundary changed")
				}
			}
			messages := body["messages"].([]any)
			if len(messages) != 4 || !strings.Contains(string(raw(messages[0])), "Today's date is") ||
				!strings.Contains(string(raw(messages[2])), "unchanged-signature") {
				t.Fatal("Start meta-user envelope lost date or opaque signature")
			}
			marked := 0
			for _, message := range messages {
				if blocks, valid := mapObject(message)["content"].([]any); valid {
					for _, block := range blocks {
						if mapObject(block)["cache_control"] != nil {
							marked++
						}
					}
				}
			}
			if marked != 1 || mapObject(body["tools"].([]any)[0])["cache_control"] != nil {
				t.Fatal("Start exceeded its one conversation cache boundary or cached a tool schema")
			}
		})
	}
}

func TestCompletionPaidFallbackRestoresOriginalSystemEnvelope(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.answer = `{"code":1005}`
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	request := giftRequest("GLM-5.3")
	request.Body = raw(map[string]any{"model": request.Model, "system": "original paid instructions",
		"messages": []any{map[string]any{"role": "user", "content": "question"}}})
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error != nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 1 {
		t.Fatalf("confirmed unused gift fallback did not reach paid: %+v", frame.Error)
	}
	<-vendor.requests
	paid := <-vendor.requests
	var body map[string]any
	if json.NewDecoder(paid.Body).Decode(&body) != nil || body["system"] != "original paid instructions" ||
		len(body["messages"].([]any)) != 1 {
		t.Fatalf("paid fallback retained the gift's system/cache/date envelope: %+v", body)
	}
}

func TestCompletionGiftRepliesNameActualTrialPoolWithoutChangingText(t *testing.T) {
	for _, streamed := range []bool{false, true} {
		for _, model := range []string{"GLM-5.3", "GLM-5.3-Trial"} {
			t.Run(model+map[bool]string{false: "/json", true: "/sse"}[streamed], func(t *testing.T) {
				vendor, server := newCompletionVendor(t)
				if streamed {
					vendor.media = "text/event-stream"
					vendor.answer = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"model\":\"glm-5.3\",\"content\":[]}}\n\n" +
						"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"GLM-5.3 and model are ordinary words\"}}\n\n" +
						"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
				}
				h := newHarness(t, endpointOptions(server))
				frame, body := h.call(sdk.MethodProviderExecute, scope(giftAccount()), giftRequest(model), raw(giftAccount()))
				if frame.Error != nil || !strings.Contains(string(body), `"model":"GLM-5.3-Trial"`) ||
					!strings.Contains(string(body), "GLM-5.3 and model are ordinary words") ||
					vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 0 {
					t.Fatalf("actual gift pool disappeared from response/log attribution: %+v %s", frame.Error, body)
				}
			})
		}
	}
}

func TestCompletionHeldFreeFirstBypassesGiftBeforeAcceptanceButTrialStaysStrict(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.answer, vendor.status, vendor.media = "<html>errors.aliyun.com</html>", 405, "text/html"
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	first, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
	if first.Error != nil || vendor.startCalls.Load() != 1 {
		t.Fatal("fixture did not establish its explicit gift firewall refusal")
	}
	next, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if next.Error != nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 1 {
		t.Fatalf("known held free-first route blocked the paid request before dispatch: %+v", next.Error)
	}
	trial, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3-Trial"), raw(account))
	if trial.Error == nil || trial.Error.Outcome != "not-started" || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 1 {
		t.Fatal("held Trial was dispatched or used the paid route")
	}
}

func TestCompletionManualPaidSnapshotKeepsGiftCatalogAndUsage(t *testing.T) {
	_, server := newCompletionVendor(t)
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	account.Discovery = false
	frame, _ := h.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
	var catalog sdk.ModelsResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &catalog) != nil {
		t.Fatalf("manual catalog: %+v", frame.Error)
	}
	found := false
	for _, model := range catalog.Models {
		found = found || model.ID == "GLM-5.3-Trial" && model.Free
	}
	frame, _ = h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
	var usage sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &usage) != nil {
		t.Fatalf("manual usage: %+v", frame.Error)
	}
	gift := false
	for _, window := range usage.Windows {
		gift = gift || containsString(window.Models, "GLM-5.3-Trial")
	}
	if !found || !gift {
		t.Fatalf("manual snapshot lost its selected session's gifts: catalog=%+v usage=%+v", catalog.Models, usage.Windows)
	}
}

func TestCompletionManualPaidFreeFirstUsesConfirmedGift(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	account.Discovery = false
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if frame.Error != nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 0 {
		t.Fatalf("manual selected session ignored confirmed free-first capacity: %+v", frame.Error)
	}
}

func TestCompletionPersonalEntitlementKeepsValidSiblingAndSourceEnvelope(t *testing.T) {
	for _, response := range []string{
		`{"code":0,"data":[{"productName":"Coding Broken","status":false},{"productName":"Coding Plan","status":"VALID","inCurrentPeriod":true}]}`,
		`{"code":0,"data":[null,{"productName":"Unrelated Product"},{"productName":"Coding Plan","status":"VALID","inCurrentPeriod":true}]}`,
		`{"data":[{"productName":"Coding Plan","status":"VALID","inCurrentPeriod":true}]}`,
	} {
		t.Run(response, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.paidList = response
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
			var catalog sdk.ModelsResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &catalog) != nil {
				t.Fatalf("source-valid paid entitlement was hidden by a sibling/envelope: %+v", frame.Error)
			}
			for _, model := range catalog.Models {
				if model.ID == "GLM-5.3" && !model.Free {
					return
				}
			}
			t.Fatal("confirmed paid model became gift-only")
		})
	}
}

func TestCompletionMalformedCodingFieldsRemainUnknownWithoutGiftFallback(t *testing.T) {
	for _, fields := range []string{`"productId":null`, `"autoRenew":{}`, `"nextRenewTime":false`} {
		t.Run(fields, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.paidList = `{"code":0,"data":[{"productName":"Coding Plan","status":"VALID","inCurrentPeriod":true,` + fields + `}]}`
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
			if frame.Error == nil || vendor.balanceReads.Load() != 0 {
				t.Fatal("malformed Coding schema became paid authority or an inactive-gift fallback")
			}
		})
	}
}

func TestCompletionPaidBusyRetriesOnlyConfirmedUnusedCaps(t *testing.T) {
	for _, test := range []struct {
		name   string
		media  string
		status int
		body   string
		retry  bool
	}{
		{"3008", "application/json", 429, `{"code":3008}`, true},
		{"3009", "application/json", 429, `{"error":{"code":"3009"}}`, true},
		{"3010", "application/json", 429, `{"code":3010}`, true},
		{"unknown", "application/json", 429, `{"code":500}`, false},
		{"output", "application/json", 429, `{"code":3008,"usage":{"input_tokens":1}}`, false},
		{"malformed", "application/json", 429, `{"code":3008`, false},
		{"sse", "text/event-stream", 429, "data: {\"code\":3008}\n\n", false},
		{"wrong-status", "application/json", 503, `{"code":3008}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/zai" {
					t.Errorf("paid retry escaped inference: %s", request.URL.Path)
				}
				attempt := calls.Add(1)
				writer.Header().Set("Content-Type", test.media)
				if attempt > 2 {
					io.WriteString(writer, `{"type":"message","content":[]}`)
					return
				}
				writer.WriteHeader(test.status)
				io.WriteString(writer, test.body)
			}))
			defer server.Close()
			h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{FreeFirst: new(false)})
			h.recoverRequest = func(failure sdk.RequestRecoveryRequest) sdk.RequestRecoveryResult {
				decision := sdk.RequestRecoveryResult{Step: failure.Step, Action: "terminal"}
				if test.retry && failure.Evidence != "" && len(h.recoveries) < 3 {
					decision.Action, decision.Retry = "wait", true
				}
				return decision
			}
			account := renewableCredentials("individual")
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			expected := int32(1)
			if test.retry {
				expected = 3
				var result sdk.ExecuteResult
				if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Status != 200 {
					t.Errorf("confirmed unused paid cap never recovered: %+v", frame.Error)
				}
			}
			if calls.Load() != expected {
				t.Fatalf("paid cap attempts = %d, want %d", calls.Load(), expected)
			}
		})
	}
}

func TestCompletionConfirmedInactivePaidStillOffersAndServesOnlyGift(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.paidActive.Store(false)
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{FreeFirst: new(false)})
	account := giftAccount()
	frame, _ := h.call(sdk.MethodProviderModels, scope(account), sdk.ModelsRequest{}, raw(account))
	var catalog sdk.ModelsResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &catalog) != nil || len(catalog.Models) != 2 {
		t.Fatalf("confirmed inactive personal entitlement hid its live gift: %+v %+v", frame.Error, catalog.Models)
	}
	for _, model := range catalog.Models {
		if !model.Free || model.ID != "GLM-5.3" && model.ID != "GLM-5.3-Trial" {
			t.Fatalf("inactive paid catalog advertised paid authority: %+v", model)
		}
	}
	frame, _ = h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
	if frame.Error != nil || vendor.startCalls.Load() != 1 || vendor.paidCalls.Load() != 0 {
		t.Fatalf("inactive paid route did not select its confirmed gift: %+v", frame.Error)
	}
}

func TestCompletionGiftOnlyCannotBecomePaidWhenConfirmationChanges(t *testing.T) {
	for _, balance := range []string{
		`{"code":500}`,
		`{"code":0,"data":{"plans":[{"plan_id":"grant","status":"active"}],"balances":[]}}`,
	} {
		t.Run(balance, func(t *testing.T) {
			vendor, server := newCompletionVendor(t)
			vendor.paidActive.Store(false)
			vendor.balanceAfter = balance
			h := newHarness(t, endpointOptions(server))
			account := giftAccount()
			frame, _ := h.call(sdk.MethodProviderExecute, scope(account), giftRequest("GLM-5.3"), raw(account))
			if frame.Error == nil || vendor.paidCalls.Load() != 0 || vendor.startCalls.Load() != 0 {
				t.Fatalf("gift-only catalog became paid authority after a changed/unknown balance: %+v", frame.Error)
			}
		})
	}
}

func TestCompletionGiftWindowsCannotExceedPublicUsageBudget(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	capabilities := []string{}
	for index := range 1024 {
		capabilities = append(capabilities, "model:gift-"+strconv.Itoa(index))
	}
	vendor.balance = string(raw(map[string]any{"code": 0, "data": map[string]any{
		"plans": []any{
			map[string]any{"plan_id": "active", "status": "active"},
			map[string]any{"plan_id": "future", "status": "pending", "starts_at": fixtureNow.Add(time.Hour).Unix()},
		},
		"balances": []any{map[string]any{"plan_id": "active", "remaining_units": 1,
			"total_units": 1, "capabilities": capabilities}},
	}}))
	h := newHarness(t, endpointOptions(server))
	account := Credentials{Site: "zai", Kind: "start", UserID: "fixture-user", Device: "fixture-device",
		SessionToken: "fixture-zcode-session", ExpiresAt: fixtureNow.Add(time.Hour).UnixMilli()}
	if err := account.Validate(fixtureNow); err != nil {
		t.Fatal(err)
	}
	frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
	if frame.Error == nil || vendor.balanceReads.Load() != 1 {
		t.Fatal("model projection plus pending plans exceeded the public 1024-window budget")
	}
}
func TestCompletionStartQueryTracksUserTurnsAndIgnoresToolResults(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	messages := []any{map[string]any{"role": "user", "content": "first"}}
	var first string
	for turn := 0; turn < 3; turn++ {
		request := giftRequest("GLM-5.3-Trial")
		request.Body = raw(map[string]any{"model": request.Model, "messages": messages})
		frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
		if frame.Error != nil {
			t.Fatalf("query fixture failed: %+v", frame.Error)
		}
		sent := <-vendor.requests
		query := sent.Header.Get("X-Query-Id")
		if len(query) != 36 || strings.Contains(query, "query_") {
			t.Fatalf("Start request omitted its native turn attribution: %q", query)
		}
		if turn == 0 {
			first = query
			messages = append(messages, map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "tool", "name": "Read", "input": map[string]any{}},
			}}, map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tool", "content": "data"},
			}})
		} else if turn == 1 {
			if query != first {
				t.Fatal("tool result started a different user turn")
			}
			messages = append(messages, map[string]any{"role": "user", "content": "next"})
		} else if query == first {
			t.Fatal("next user turn reused the previous query identity")
		}
	}
}

func TestCompletionPendingGiftIsVisibleWithoutRoutingAuthority(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.balance = string(raw(map[string]any{"code": 0, "data": map[string]any{
		"plans": []any{map[string]any{"plan_id": "future", "name": "Tomorrow grant", "status": "unknown_pending",
			"starts_at": fixtureNow.Add(5 * time.Hour).Unix(), "ends_at": fixtureNow.Add(48 * time.Hour).Unix()}},
		"balances": []any{},
	}}))
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
	var usage sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &usage) != nil {
		t.Fatalf("pending usage: %+v", frame.Error)
	}
	found := false
	for _, window := range usage.Windows {
		if strings.Contains(window.Name, "Tomorrow grant") {
			found = window.Aside && len(window.Models) == 0 && len(window.NotModels) == 0 &&
				window.ResetsAt == fixtureNow.Add(5*time.Hour).UnixMilli()
		}
	}
	if !found {
		t.Fatalf("pending gift disappeared or became spendable quota: %+v", usage.Windows)
	}
}

func TestCompletionGiftBucketsKeepPlanIdentityAndOnlyBestBucketControlsModel(t *testing.T) {
	vendor, server := newCompletionVendor(t)
	vendor.balance = `{"code":0,"data":{"plans":[
		{"plan_id":"old","user_plan_id":"old-user-plan","name":"Start Plan","status":"active"},
		{"plan_id":"new","user_plan_id":"new-user-plan","name":"Trust Build","status":"active"}],
		"balances":[
		{"plan_id":"old","user_plan_id":"old-user-plan","show_name":"GLM-5.3","total_units":100,"remaining_units":0,"capabilities":["model:GLM-5.3"]},
		{"plan_id":"new","user_plan_id":"new-user-plan","show_name":"GLM-5.3","total_units":100,"remaining_units":75,"capabilities":["model:GLM-5.3"]}]}}`
	h := newHarness(t, endpointOptions(server))
	account := giftAccount()
	frame, _ := h.call(sdk.MethodProviderUsage, scope(account), sdk.UsageRequest{}, raw(account))
	var usage sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &usage) != nil {
		t.Fatalf("per-plan gift usage: %+v", frame.Error)
	}
	live, spent := false, false
	for _, window := range usage.Windows {
		if strings.Contains(window.Name, "Trust Build") {
			live = !window.Aside && window.Used == 25 && containsString(window.Models, "GLM-5.3-Trial")
		}
		if strings.Contains(window.Name, "Start Plan") {
			spent = window.Aside && window.Used == 100 && len(window.Models) == 0
		}
	}
	if !live || !spent {
		t.Fatalf("spent sibling hid the live gift or two plans collapsed into one invented capacity: %+v", usage.Windows)
	}
}
