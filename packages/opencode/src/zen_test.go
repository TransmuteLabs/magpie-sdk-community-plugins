package opencode

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func TestZenWindowDeclaresSeparatePersonalAndPublicAuthority(t *testing.T) {
	module := NewWindowModule(ModuleOptions{})
	if err := module.Validate(); err != nil {
		t.Fatal(err)
	}
	providers := map[string]bool{}
	for _, provider := range module.Manifest.Providers {
		providers[provider.ID] = true
	}
	if len(providers) != 3 || !providers[ProviderGo] || !providers[ProviderZen] || !providers["opencode-zen-free"] {
		t.Fatal("Zen personal and public identities are missing or merged")
	}
	methods := map[string]string{}
	for _, binding := range module.Manifest.AuthBindings {
		if binding.From != sdk.ProviderSignInPart {
			continue
		}
		for _, method := range binding.Methods {
			methods[binding.Provider+"/"+method.ID] = method.Mode
		}
	}
	for _, method := range []string{ProviderGo + "/device", ProviderZen + "/device", ProviderZen + "/key", "opencode-zen-free/public"} {
		if methods[method] != "poll" {
			t.Fatal("Zen method has no private host-owned authentication workflow:", method)
		}
	}
	if len(Manifest().Providers) != 1 || len(AgentManifest().Providers) != 1 {
		t.Fatal("Zen changed the accepted Go-only distributions")
	}
}

func TestZenKeyPublicFlowAndRefreshKeepSeparateAuthority(t *testing.T) {
	var exchanges atomic.Int32
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/console/auth/device/token" {
			t.Error("key/public auth attempted unexpected network")
			writer.WriteHeader(500)
			return
		}
		exchanges.Add(1)
		io.WriteString(writer, `{"access_token":"rotated","refresh_token":"rotated-refresh","expires_in":3600,"org_id":"org-b"}`)
	}))
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{}))
	for _, test := range []struct {
		Provider, Method string
		Input            any
	}{
		{ProviderZen, "key", map[string]string{"key": "PRIVATE-service-key"}},
		{ProviderZenPublic, "public", nil},
	} {
		scope := sdk.Scope{Provider: test.Provider}
		frame, _ := harness.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: test.Method, Input: raw(test.Input)}, nil)
		var start sdk.AuthStartResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &start) != nil || len(start.Continuation) == 0 {
			t.Fatal("private key/public flow did not start", frame.Error)
		}
		frame, _ = harness.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: start.Continuation}, nil)
		var result sdk.AuthResult
		var identity ZenCredentials
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || json.Unmarshal(result.Credentials, &identity) != nil ||
			identity.Validate(test.Provider) != nil || result.ID != identity.AccountID(test.Provider) {
			t.Fatal("flow published the wrong identity", frame.Error)
		}
		frame, _ = harness.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: start.Continuation}, nil)
		if frame.Error == nil {
			t.Fatal("private authentication continuation replayed")
		}
		selected := sdk.Call{Scope: zenScope(test.Provider, identity), Auth: result.Credentials}
		if _, err := selectedZen(selected); err != nil {
			t.Fatal(err)
		}
		for _, revision := range []string{"", "17"} {
			usage, _ := harness.invoke(sdk.MethodProviderUsage, selected.Scope, sdk.UsageRequest{Revision: revision}, selected.Auth)
			quota, fresh, err := sdk.ValidateUsageResult(WindowManifest(), usage.Result)
			if usage.Error != nil || err != nil || fresh || quota.Revision != revision || len(quota.Windows) != 0 {
				t.Fatalf("%s has no ordinary quota at revision %q: %+v, error %v, fresh %v, result %s",
					test.Provider, revision, usage.Error, err, fresh, usage.Result)
			}
			if revision == "" && bytesContains(usage.Result, `"revision"`) {
				t.Fatal("the omitted host usage revision was synthesized")
			}
		}
		selected.Scope.Provider = ProviderGo
		if _, err := selectedZen(selected); err == nil {
			t.Fatal("Zen credential was accepted as Go authority")
		}
	}
	if exchanges.Load() != 0 {
		t.Fatal("private key/public auth unexpectedly dispatched network")
	}
	for _, test := range []struct {
		Provider, Method string
		Input            any
	}{
		{ProviderZen, "key", map[string]string{"key": "public"}},
		{ProviderZenPublic, "public", map[string]string{"key": "PRIVATE"}},
		{ProviderZenPublic, "key", map[string]string{"key": "PRIVATE"}},
		{ProviderGo, "public", nil},
	} {
		frame, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: test.Provider},
			sdk.AuthStartRequest{Method: test.Method, Input: raw(test.Input)}, nil)
		if frame.Error == nil {
			t.Fatal("cross-provider or substituted authentication input was accepted")
		}
	}
	old := credential(time.Now(), "a", "org-a")
	identity := ZenCredentials{Kind: "oauth", OAuth: &old}
	frame, _ := harness.invoke(sdk.MethodAuthRefresh, zenScope(ProviderZen, identity), sdk.AuthRefreshRequest{}, raw(identity))
	var result sdk.AuthResult
	var next ZenCredentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || json.Unmarshal(result.Credentials, &next) != nil ||
		next.OAuth == nil || next.OAuth.Access != "rotated" || next.OAuth.InitialOrgID != "org-a" || next.OAuth.OrgID != "org-b" ||
		result.ID != identity.AccountID(ProviderZen) || exchanges.Load() != 1 {
		t.Fatal("Zen OAuth renewal changed its stable provider identity", frame.Error)
	}
	if old.AccountID(ProviderGo) == identity.AccountID(ProviderZen) {
		t.Fatal("Go and Zen OAuth accounts merged")
	}
}

func TestZenNativeProtocolsSignOnlySelectedCredentialsAndPreserveBodies(t *testing.T) {
	var replies map[string]struct {
		JSON string `json:"json"`
		SSE  string `json:"sse"`
	}
	body, err := os.ReadFile("testdata/donors/zen-replies.json")
	if err != nil || json.Unmarshal(body, &replies) != nil {
		t.Fatal("missing literal donor tool replies", err)
	}
	for _, protocol := range []string{"chat", "responses", "messages", "anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			wireProtocol := protocol
			if wireProtocol == "anthropic" {
				wireProtocol = "messages"
			}
			var model string
			for _, candidate := range zenBootstrap(false) {
				if slices.ContainsFunc(candidate.Protocols, func(value protocolext.Protocol) bool { return string(value) == wireProtocol }) {
					model = candidate.ID
					break
				}
			}
			if model == "" {
				t.Fatal("pinned native protocol metadata missing")
			}
			var dispatches atomic.Int32
			var cut atomic.Bool
			payload := []byte(`{"model":"` + model + `","stream":true,"messages":[{"role":"user","content":"fixture"}],"tools":[]}`)
			if protocol == "gemini" {
				payload = []byte(`{"contents":[{"parts":[{"text":"fixture"}]}]}`)
			}
			transcript := replies[wireProtocol].SSE
			if protocol == "gemini" {
				transcript = "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\",\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"Read\",\"args\":{\"path\":\"fixture\"}}}]}}]}\n\n"
			}
			server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/inference/v1/models" {
					io.WriteString(writer, `{"data":[{"id":"`+model+`"}]}`)
					return
				}
				dispatches.Add(1)
				actual, _ := io.ReadAll(request.Body)
				if !bytes.Equal(actual, payload) || request.Header.Get("Cookie") != "" || request.Header.Get("Proxy-Authorization") != "" ||
					request.Header.Get("x-org-id") != "" || request.Header.Get("x-opencode-org-id") != "" {
					t.Error("native request changed or foreign authority survived")
				}
				secretHeader := "Authorization"
				secret := "Bearer PRIVATE-native"
				prefix, suffix := "/inference/openai/v1", "/chat/completions"
				switch protocol {
				case "responses":
					suffix = "/responses"
				case "messages", "anthropic":
					prefix, suffix, secretHeader, secret = "/inference/anthropic/v1", "/messages", "x-api-key", "PRIVATE-native"
					if request.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("Anthropic version missing")
					}
				case "gemini":
					prefix, suffix, secretHeader, secret = "/inference/google/v1beta/models/", model+":streamGenerateContent", "x-goog-api-key", "PRIVATE-native"
				}
				if request.URL.Path != prefix+suffix || request.Header.Get(secretHeader) != secret || request.Header.Get("User-Agent") != "opencode/latest/2.3.0/cli" {
					t.Error("wrong native route or selected credential")
				}
				for _, key := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
					if key != secretHeader && request.Header.Get(key) != "" {
						t.Error("foreign protocol credential survived")
					}
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				if cut.Load() {
					partial := strings.SplitN(transcript, "\n\n", 2)[0] + "\n\n"
					if protocol == "gemini" {
						partial = strings.ReplaceAll(partial, `"finishReason":"STOP"`, `"finishReason":""`)
					}
					io.WriteString(writer, partial)
				} else {
					io.WriteString(writer, transcript)
				}
			}))
			defer server.Close()
			identity := ZenCredentials{Kind: "key", Key: "PRIVATE-native"}
			harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
			options := sdk.ExecuteOptions{}
			if protocol == "gemini" {
				options.Path, options.Query = "/v1beta/models/"+model+":streamGenerateContent", "alt=sse"
			}
			request := sdk.ExecuteRequest{Protocol: protocol, Model: model, Body: payload, Options: raw(options),
				Headers: http.Header{"Authorization": {"FOREIGN"}, "X-Api-Key": {"FOREIGN"}, "X-Goog-Api-Key": {"FOREIGN"},
					"Cookie": {"FOREIGN"}, "Proxy-Authorization": {"FOREIGN"}, "X-Org-Id": {"FOREIGN"}, "X-Opencode-Org-Id": {"FOREIGN"}}}
			frame, received := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZen, identity), request, raw(identity))
			if frame.Error != nil || string(received) != transcript || dispatches.Load() != 1 {
				t.Fatal("native tool transcript was not preserved", frame.Error, string(received))
			}
			request.Credentials = raw(map[string]string{"key": "foreign"})
			frame, _ = harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZen, identity), request, raw(identity))
			if frame.Error == nil || frame.Error.Outcome != "not-started" || dispatches.Load() != 1 {
				t.Fatal("business credentials gained dispatch authority")
			}
			request.Credentials = nil
			cut.Store(true)
			frame, _ = harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZen, identity), request, raw(identity))
			if frame.Error == nil || frame.Error.Code != "truncated" || frame.Error.Outcome != "uncertain" || dispatches.Load() != 2 {
				t.Fatal("truncated native stream was accepted as completed", frame.Error)
			}
		})
	}
}

func TestZenNativeTerminalsRejectErrorsAndTruncation(t *testing.T) {
	for _, test := range []struct {
		Protocol, Reply   string
		Complete, Invalid bool
	}{
		{"chat", "data: {\"choices\":[{\"delta\":{\"content\":\"[DONE]\"}}]}\n\n", false, false},
		{"chat", "data: [DONE]\n\n", true, false},
		{"chat", "data: {\"error\":{\"message\":\"private\"}}\n\n", false, true},
		{"responses", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", true, false},
		{"responses", "data: {\"type\":\"response.incomplete\"}\n\n", false, true},
		{"responses", "data: {\"type\":\"response.output_item.done\"}\n\n", false, false},
		{"messages", "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n", false, false},
		{"messages", "data: {\"type\":\"message_stop\"}\n\n", true, false},
		{"messages", "data: {\"type\":\"error\",\"error\":{\"message\":\"private\"}}\n\n", false, true},
	} {
		terminal := zenTerminal{protocol: test.Protocol}
		var failure error
		for _, character := range []byte(test.Reply) {
			if failure = terminal.write([]byte{character}); failure != nil {
				break
			}
		}
		if (failure != nil) != test.Invalid || terminal.complete() != test.Complete {
			t.Errorf("terminal result mismatch for %s: %v", test.Protocol, failure)
		}
	}
}

func TestZenCatalogFollowsPublishedRetirement(t *testing.T) {
	retired := []string{"glm-5.3-flash-free", "glm-5", "glm-5.1", "glm-5.2"}
	const neighboringFree = "deepseek-v4.1-flash-free"
	const neighboringPaid = "glm-5.3-flash"
	for _, descriptor := range WindowManifest().Providers {
		if descriptor.ID != ProviderZen && descriptor.ID != ProviderZenPublic {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			public := descriptor.ID == ProviderZenPublic
			listed := append(slices.Clone(retired), neighboringFree, neighboringPaid)
			discovered := zenBuildModels(listed, public)
			for source, models := range map[string][]sdk.Model{"manifest": descriptor.Models, "discovery": discovered} {
				contains := func(id string) bool {
					return slices.ContainsFunc(models, func(model sdk.Model) bool { return model.ID == id })
				}
				if slices.ContainsFunc(retired, contains) || !contains(neighboringFree) || contains(neighboringPaid) == public {
					t.Errorf("%s has unexpected eligibility for the retired model or its neighbors", source)
				}
			}
		})
	}
}

func TestZenLiveMetadataMatchesDiscoveryAndExecution(t *testing.T) {
	const live = `{"object":"list","data":[
		{"id":"gemini-4-neon","object":"model","name":"Gemini 4 Neon","type":"chat",
			"limit":{"context":1000000,"input":900000,"output":64000},
			"modalities":{"input":["text","image/png"],"output":["text"]},
			"capabilities":{"tools":{"supported":true},"reasoning":{"supported":true,"effort":["low","high"]}},
			"cost":{"text":{"input":{"price":3},"output":{"price":15}}},
			"api":{"generate-content":{"base_url":"https://opencode.ai/inference/google/v1beta"}}},
		{"id":"tidepool-alpha-free","object":"model","name":"Tidepool Alpha Free","type":"chat",
			"limit":{"context":96000,"output":8000},
			"cost":{"text":{"input":{"price":0},"output":{"price":0}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"paid-looking-free","type":"chat","limit":{"context":96000,"output":8000},
			"cost":{"text":{"input":{"price":1},"output":{"price":1}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"alien-free","type":"chat","limit":{"context":96000,"output":8000},
			"cost":{"text":{"input":{"price":0},"output":{"price":0}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/custom/conn_bad"}}},
		{"id":"bad-limits-free","type":"chat","limit":{"context":96000,"output":-1},
			"cost":{"text":{"input":{"price":0},"output":{"price":0}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"wrong-protocol-free","type":"chat","limit":{"context":96000,"output":8000},
			"api":{"systemone":{"base_url":"https://opencode.ai/inference/systemone/v1"}}},
		{"id":"glm-5.2","type":"chat","limit":{"context":96000,"output":8000},
			"cost":{"text":{"input":{"price":0},"output":{"price":0}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"legacy-free","object":"model"}
	]}`
	var dispatches atomic.Int32
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/inference/v1/models" {
			io.WriteString(writer, live)
			return
		}
		dispatches.Add(1)
		switch request.URL.Path {
		case "/inference/google/v1beta/models/gemini-4-neon:generateContent":
			if request.Header.Get("X-Goog-Api-Key") != "fixture-personal" {
				t.Error("paid Gemini used the wrong authority")
			}
			io.WriteString(writer, `{"candidates":[{"finishReason":"STOP"}]}`)
		case "/inference/openai/v1/chat/completions":
			if request.Header.Get("Authorization") != "Bearer public" {
				t.Error("free Chat used the wrong authority")
			}
			io.WriteString(writer, `{"choices":[]}`)
		default:
			t.Errorf("unexpected native request: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	options := ModuleOptions{
		Endpoints:   Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil },
	}
	harness := serveHarness(t, NewWindowModule(options), raw(moduleConfig{ProjectID: "global"}))
	personal := ZenCredentials{Kind: "key", Key: "fixture-personal"}
	public := ZenCredentials{Kind: "public", Key: "public"}
	for _, test := range []struct {
		provider string
		identity ZenCredentials
		want     []string
	}{
		{ProviderZen, personal, []string{"gemini-4-neon", "legacy-free", "paid-looking-free", "tidepool-alpha-free"}},
		{ProviderZenPublic, public, []string{"legacy-free", "tidepool-alpha-free"}},
	} {
		frame, _ := harness.invoke(sdk.MethodProviderModels, zenScope(test.provider, test.identity), sdk.ModelsRequest{}, raw(test.identity))
		var result sdk.ModelsResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
			t.Fatalf("discovery failed: %+v", frame.Error)
		}
		var listed []string
		for _, model := range result.Models {
			listed = append(listed, model.ID)
			switch model.ID {
			case "gemini-4-neon":
				if model.Name != "Gemini 4 Neon" || model.ContextWindow != 900000 || model.MaxOutput != 64000 ||
					!model.Tools || !model.Vision || !model.Reasoning || model.Free ||
					!slices.Equal(model.Protocols, []protocolext.Protocol{"gemini"}) {
					t.Errorf("Gemini lost native protocol or live metadata: %+v", model)
				}
			case "tidepool-alpha-free":
				if model.ContextWindow != 96000 || model.MaxOutput != 8000 || !model.Free ||
					!slices.Equal(model.Protocols, []protocolext.Protocol{"chat"}) {
					t.Errorf("Chat lost native protocol or live metadata: %+v", model)
				}
			}
		}
		if !slices.Equal(listed, test.want) {
			t.Errorf("%s models = %q, want %q", test.provider, listed, test.want)
		}
	}
	gemini := sdk.ExecuteRequest{Protocol: "gemini", Model: "gemini-4-neon", Body: json.RawMessage(`{"contents":[{}]}`)}
	chat := sdk.ExecuteRequest{Protocol: "chat", Model: "tidepool-alpha-free", Body: json.RawMessage(`{"model":"tidepool-alpha-free","messages":[]}`)}
	for _, test := range []struct {
		provider string
		identity ZenCredentials
		request  sdk.ExecuteRequest
	}{
		{ProviderZen, personal, gemini}, {ProviderZenPublic, public, chat},
	} {
		frame, _ := harness.invoke(sdk.MethodProviderExecute, zenScope(test.provider, test.identity), test.request, raw(test.identity))
		if frame.Error != nil || !harness.accepted[frame.ID] {
			t.Errorf("discovered native model did not execute: %+v", frame.Error)
		}
	}
	frame, _ := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, public), gemini, raw(public))
	if frame.Error == nil || frame.Error.Outcome != "not-started" || dispatches.Load() != 2 {
		t.Fatal("public execution used a model outside its discovered authorization", frame.Error, dispatches.Load())
	}
}

func TestZenStaleListingCannotRestoreRetirementWithoutMetadata(t *testing.T) {
	const retired = "glm-5.3-flash-free"
	const future = "future-free"
	var metadata struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(zenCatalog, &metadata); err != nil {
		t.Fatal(err)
	}
	delete(metadata.Models, retired)
	snapshot, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	original := zenCatalog
	t.Cleanup(func() { zenCatalog = original })
	zenCatalog = snapshot

	for _, test := range []struct {
		provider string
		identity ZenCredentials
	}{
		{ProviderZen, ZenCredentials{Kind: "key", Key: "fixture-personal"}},
		{ProviderZenPublic, ZenCredentials{Kind: "public", Key: "public"}},
	} {
		t.Run(test.provider, func(t *testing.T) {
			var discoveries atomic.Int32
			server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/inference/v1/models" || request.Header.Get("Authorization") != "Bearer "+test.identity.Key {
					t.Error("discovery used the wrong endpoint or selected identity")
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				discoveries.Add(1)
				io.WriteString(writer, `{"data":[{"id":"`+retired+`"},{"id":"`+future+`"}]}`)
			}))
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{}))
			frame, _ := harness.invoke(sdk.MethodProviderModels, zenScope(test.provider, test.identity), sdk.ModelsRequest{}, raw(test.identity))
			var result sdk.ModelsResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || discoveries.Load() != 1 {
				t.Fatal("stale listing discovery did not complete", frame.Error, discoveries.Load())
			}
			if slices.ContainsFunc(result.Models, func(model sdk.Model) bool { return model.ID == retired }) {
				t.Error("stale listing restored the retired model after metadata removal")
			}
			if !slices.ContainsFunc(result.Models, func(model sdk.Model) bool {
				return model.ID == future && slices.Equal(model.Protocols, []protocolext.Protocol{"chat"})
			}) {
				t.Error("stale listing filter discarded the unrelated future free model")
			}
		})
	}
}

func TestZenCatalogLiteralEligibilityAndSuccessfulEmptyRetirement(t *testing.T) {
	body, err := os.ReadFile("testdata/donors/zen-list.json")
	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal(body, &listing) != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range listing.Data {
		ids = append(ids, entry.ID)
	}
	models := zenBuildModels(ids, true)
	if len(models) == 0 {
		t.Fatal("literal donor free catalog became empty")
	}
	for _, model := range models {
		if !model.Free || model.ID == "gpt-5" {
			t.Fatal("paid model gained public eligibility")
		}
	}
	if len(zenBuildModels([]string{"unknown-paid"}, false)) != 0 || len(zenBuildModels([]string{"future-free"}, true)) != 1 {
		t.Fatal("unknown paid model inferred or new free model lost Chat bootstrap")
	}
	if len(zenBuildModels([]string{"cat-jam-free", "jev-1.13-free"}, true)) != 0 {
		t.Fatal("retired or SystemOne-only listed free model became executable")
	}
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	var fail atomic.Bool
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer public" {
			t.Error("public catalog used personal authority")
		}
		if fail.Load() {
			writer.WriteHeader(503)
			return
		}
		io.WriteString(writer, `{"data":[]}`)
	}))
	defer server.Close()
	identity := ZenCredentials{Kind: "public", Key: "public"}
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Now: func() time.Time { return time.Unix(0, clock.Load()) }, Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{}))
	frame, _ := harness.invoke(sdk.MethodProviderModels, zenScope(ProviderZenPublic, identity), sdk.ModelsRequest{}, raw(identity))
	var result sdk.ModelsResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Models) != 0 {
		t.Fatal("successful empty catalog did not retire models")
	}
	fail.Store(true)
	clock.Add(int64(time.Minute))
	frame, _ = harness.invoke(sdk.MethodProviderModels, zenScope(ProviderZenPublic, identity), sdk.ModelsRequest{}, raw(identity))
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Models) != 0 {
		t.Fatal("transient error resurrected retired catalog")
	}
	if strings.Contains(string(frame.Result), "PRIVATE") {
		t.Fatal("catalog leaked credentials")
	}
}

func TestZenPublicRefusalNeverSwitchesIdentityOrRetries(t *testing.T) {
	var requests atomic.Int32
	var accepted atomic.Bool
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("Authorization") != "Bearer public" || request.Header.Get("x-opencode-org-id") != "" {
			t.Error("public inference gained personal credentials")
		}
		if request.URL.Path == "/inference/v1/models" {
			io.WriteString(writer, `{"data":[{"id":"future-free"}]}`)
			return
		}
		writer.WriteHeader(http.StatusForbidden)
		io.WriteString(writer, `{"error":{"type":"FreeTierError","message":"Native request required"}}`)
	}))
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
	harness.acceptedSignal = &accepted
	identity := ZenCredentials{Kind: "public", Key: "public"}
	request := sdk.ExecuteRequest{Protocol: "chat", Model: "future-free", Body: []byte(`{"model":"future-free","messages":[]}`)}
	frame, response := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), request, raw(identity))
	var result sdk.ExecuteResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || result.Status != 403 || requests.Load() != 2 ||
		result.Refusal == nil || result.Refusal.Facts.Type != "FreeTierError" || len(response) != 0 || accepted.Load() {
		t.Fatal("public refusal was hidden, retried or crossed to a personal account", frame.Error)
	}
}

func TestZenRetirementAliasesAndPolicyEvolution(t *testing.T) {
	for _, body := range []string{`{"deprecated":true}`, `{"status":"retired"}`, `{"lifecycle":"disabled"}`,
		`{"deprecated_at":"2026-10-15"}`, `{"retirement_date":"2026-10-15"}`} {
		var model inferenceModel
		if json.Unmarshal([]byte(body), &model) != nil || !model.retired() {
			t.Fatal("donor retirement signal was ignored", body)
		}
	}
	document := map[string]json.RawMessage{"providers": raw(map[string]any{}),
		"policies": json.RawMessage(`[{"action":"tool.use","resource":"shell:sudo *","effect":"deny"},{"action":"future.use","future":true},{"action":"tool.use","effect":"future-effect"},{"action":"integration.use","resource":"skill:deploy","effect":"allow"}]`)}
	if !validConsoleDocument(document) || strings.Contains(string(document["policies"]), "future") ||
		!strings.Contains(string(document["policies"]), `"action":"skill.use"`) {
		t.Fatal("future policy values or legacy resource normalization diverged")
	}
	for _, invalid := range []string{`[null]`, `[{}]`, `[{"action":"tool.use","resource":42,"effect":"deny"}]`, `[{"action":"tool.use","effect":"deny"}]`} {
		document["policies"] = json.RawMessage(invalid)
		if validConsoleDocument(document) {
			t.Fatal("malformed policy document was accepted", invalid)
		}
	}
}

func TestConsoleUsesDeclaredSelectedAccountActions(t *testing.T) {
	module := NewWindowModule(ModuleOptions{})
	for _, name := range []string{"console.snapshot", "console.skill", "console.watch.list", "console.watch.poll", "console.watch.ack"} {
		if module.Actions[name] == nil {
			t.Fatal("Console selected-account action is missing:", name)
		}
	}
	body, err := json.Marshal(module.Manifest)
	if err != nil || !json.Valid(body) {
		t.Fatal("invalid extended module manifest")
	}
}
