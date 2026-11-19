package devin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func domainCatalog() []byte {
	var result []byte
	for _, variant := range []struct {
		id, family, label string
		free              uint64
		images            uint64
	}{
		{"swe-2-medium-lightning", "swe-2", "SWE-2", 0, 0},
		{"swe-2-high", "swe-2", "SWE-2", 4, 0},
		{"swe-2-low", "swe-2", "SWE-2", 4, 0},
		{"swe-2-max", "swe-2", "SWE-2", 4, 0},
		{"claude-opus-5-medium", "claude-opus-5", "Claude Opus 5", 0, 1},
		{"claude-opus-5-xhigh", "claude-opus-5", "Claude Opus 5", 0, 0},
		{"claude-opus-5-low-fast", "claude-opus-5", "Claude Opus 5", 0, 1},
		{"claude-opus-5-high-fast", "claude-opus-5", "Claude Opus 5", 0, 1},
		{"claude-opus-5_MIN_PRIORITY", "claude-opus-5", "Claude Opus 5", 0, 1},
		{"claude-opus-5_HIGH_PRIORITY", "claude-opus-5", "Claude Opus 5", 0, 1},
		{"swe-1-6-fast", "swe-1-6", "SWE-1.6", 0, 0},
		{"account-only-low", "account-only", "Account model", 0, 0},
		{"account-only-high", "account-only", "Account model", 0, 0},
	} {
		info := nativeFields(nativeNumber(4, 32768), nativeNumber(13, 2048),
			nativeText(23, variant.family), nativeText(6, string(nativeFields(nativeNumber(12, 1), nativeNumber(15, 1)))))
		config := nativeFields(nativeText(1, variant.id), nativeText(22, variant.id),
			nativeNumber(5, variant.images), nativeNumber(18, 32768), nativeNumber(24, variant.free),
			nativeText(23, string(info)), nativeText(30, string(nativeText(1, variant.label))))
		result = append(result, nativeText(1, string(config))...)
	}
	return result
}

func TestProviderDomainCatalogProjectsFamiliesEffortsFastAliasesAndFreeScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != outbound.ModelsPath {
			t.Error("model discovery reached another operation")
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "application/proto")
		writer.Write(domainCatalog())
	}))
	t.Cleanup(server.Close)
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	frame, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	var result sdk.ModelsResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
		t.Fatal("selected-account catalog failed", frame)
	}
	models := map[string]sdk.Model{}
	for _, model := range result.Models {
		models[model.ID] = model
	}
	family, found := models["swe-2"]
	if !found || family.Name != "SWE-2" || !family.Free ||
		!slices.Equal(family.Efforts, []string{"low", "high", "max"}) || family.DefaultEffort != "high" {
		t.Fatalf("discovered free family/default efforts were lost: %+v", family)
	}
	for _, id := range []string{"swe", "opus", "opus-fast", "account-only", "swe-2-medium-lightning", "swe-2-high"} {
		if _, found := models[id]; !found {
			t.Errorf("discovered family, alias, tier or pinned concrete UID missing: %s", id)
		}
	}
	fast := models["claude-opus-5-fast"]
	if !slices.Equal(fast.Efforts, []string{"low", "high"}) || fast.DefaultEffort != "high" ||
		fast.Name != "Claude Opus 5 Fast" || fast.ContextWindow != 32768 || fast.MaxOutput != 2048 ||
		fast.Free || models["claude-opus-5"].Vision {
		t.Fatalf("fast tier or conservative family capabilities changed: %+v", fast)
	}
	if _, found := models["swe-1-6-fast-fast"]; found {
		t.Fatal("a fast word without an effort became a tier")
	}
	if _, found := models["gemini"]; found {
		t.Fatal("static aliases fabricated an undiscovered account family")
	}
}

func TestProviderDomainSelectionUsesCurrentAccountVariantsAndKeepsConcreteUIDs(t *testing.T) {
	for _, fixture := range []struct {
		name, model, controls, want string
		messages                    bool
	}{
		{"family-default", "swe-2", "", "swe-2-high", false},
		{"alias-nearest-tie-up", "swe", `,"reasoning_effort":"medium"`, "swe-2-high", false},
		{"ultra-alias", "swe", `,"reasoning_effort":"ultra"`, "swe-2-max", false},
		{"fast-nearest-tie-up", "opus-fast", `,"reasoning_effort":"medium"`, "claude-opus-5-high-fast", false},
		{"priority-underscore-min", "opus-priority", `,"reasoning_effort":"none"`, "claude-opus-5_MIN_PRIORITY", false},
		{"concrete-stays-pinned", "swe-2-low", `,"reasoning_effort":"max"`, "swe-2-low", false},
		{"messages-effort", "opus", `,"output_config":{"effort":"max"}`, "claude-opus-5-xhigh", true},
		{"messages-budget", "swe", `,"thinking":{"type":"enabled","budget_tokens":4096}`, "swe-2-low", true},
		{"messages-disabled", "swe", `,"thinking":{"type":"disabled"},"output_config":{"effort":"low"}`, "swe-2-high", true},
		{"discovered-dynamic", "account-only", "", "account-only-low", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var dispatches atomic.Int32
			actual := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case outbound.ModelsPath:
					writer.Header().Set("Content-Type", "application/proto")
					writer.Write(domainCatalog())
				case outbound.ChatPath:
					dispatches.Add(1)
					_, payload, err := codec.ReadConnectFrame(request.Body, false)
					if err != nil {
						t.Error(err)
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					fields, err := codec.RawFields(payload)
					if err != nil {
						t.Error(err)
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					select {
					case actual <- string(fields[21]):
					default:
						t.Error("duplicate inference dispatch")
					}
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Write(nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeNumber(5, 2))))
					writer.Write(nativeFrame(2, []byte(`{}`)))
				default:
					t.Error("selection reached another operation")
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			peer, _ := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			if fixture.name != "family-default" {
				discovered, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
				if discovered.Error != nil {
					t.Fatal(discovered)
				}
			}
			body := []byte(`{"model":"` + fixture.model + `","max_tokens":64,"messages":[{"role":"user","content":"hello"}]` + fixture.controls + `}`)
			request := fixtureExecute(body, sdk.ExecuteOptions{CallerID: "domain-caller"})
			request.Model = fixture.model
			if fixture.messages {
				request.Protocol = "anthropic"
				request.Options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "domain-caller"})
			}
			frame, events := peer.call(t, sdk.MethodProviderExecute, request)
			if frame.Error != nil || dispatches.Load() != 1 {
				t.Fatalf("source-proven family/effort selection did not execute once: %+v dispatches=%d", frame.Error, dispatches.Load())
			}
			select {
			case got := <-actual:
				if !bytes.Equal([]byte(got), nativeText(21, fixture.want)) {
					t.Fatalf("selected family went as the wrong native UID: got %x want %s", got, fixture.want)
				}
			default:
				t.Fatal("successful execution had no observed native model")
			}
			accepted := 0
			for _, event := range events {
				if event.Event == "accepted" {
					accepted++
				}
			}
			if accepted != 1 {
				t.Fatal("model resolution changed the accepted-write boundary", accepted)
			}
		})
	}
}

func TestProviderDomainAPI9QuotaKeepsNumericAmountsBalanceSpansAndPlanEnd(t *testing.T) {
	for _, consumed := range []string{`"0"`, `"1.5"`, `null`} {
		t.Run(consumed, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case outbound.ModelsPath:
					writer.Header().Set("Content-Type", "application/proto")
					writer.Write(domainCatalog())
				case outbound.StatusPath:
					writer.Header().Set("Content-Type", "application/json")
					io.WriteString(writer, `{"userStatus":{"planStatus":{"acuConsumed":`+consumed+
						`,"acuLimit":"5","overageBalanceMicros":"0","planEnd":"2030-01-01T00:00:00Z",`+
						`"dailyQuotaRemainingPercent":75,"weeklyQuotaRemainingPercent":50}}}`)
				default:
					t.Error("quota read dispatched inference")
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			module, err := NewAgent(ModuleOptions{Server: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			peer := startSDKPeer(t, module)
			store := &stateFixture{rows: map[string]sdk.StateValue{}}
			peer.host = store.host
			selectAccount(peer, server.URL)
			frame, _ := peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "19"})
			var result sdk.UsageResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
				t.Fatal("rich quota read failed", frame)
			}
			if _, fresh, err := sdk.ValidateUsageResult(module.Manifest, frame.Result); err != nil || !fresh {
				t.Fatal("rich quota violated the public freshness/number contract", fresh, err)
			}
			if len(result.Windows) != 4 {
				t.Fatalf("numeric quota/balance windows disappeared: %+v", result.Windows)
			}
			day, week, acu, balance := result.Windows[0], result.Windows[1], result.Windows[2], result.Windows[3]
			if day.Span != 86400 || week.Span != 604800 ||
				!slices.Contains(day.NotModels, "swe-2") || !slices.Contains(day.NotModels, "swe") ||
				slices.Contains(day.NotModels, "swe-2-medium-lightning") {
				t.Fatal("quota spans or selectable free-family exclusions were lost", day, week)
			}
			unknown := consumed == `null`
			if acu.Unit != "ACUs" || acu.Limit != 5 || !acu.Aside || acu.Unknown != unknown ||
				acu.ResetsAt != 1893456000000 || acu.Display == "" || (acu.Amount == nil) != unknown {
				t.Fatalf("native numeric ACUs/absence/plan-end were lost: %+v", acu)
			}
			if !unknown {
				want := 0.0
				if consumed == `"1.5"` {
					want = 1.5
				}
				if *acu.Amount != want || acu.Used != want/5*100 {
					t.Fatal("numeric ACUs were rounded, guessed or confused with percent", acu)
				}
			}
			if balance.Balance == nil || *balance.Balance != 0 || balance.Unit != "micros" ||
				!balance.Aside || !balance.Unknown || balance.Amount != nil ||
				bytes.Contains(frame.Result, []byte("USD")) || bytes.Contains(frame.Result, []byte("$")) {
				t.Fatalf("explicit zero balance or source currency authority changed: %+v", balance)
			}
		})
	}
}

func TestProviderDomainFamilyResolutionFailsClosedForCurrentAccount(t *testing.T) {
	for _, mode := range []string{"missing-family", "unreadable-catalog", "absent-static-alias"} {
		t.Run(mode, func(t *testing.T) {
			var current, dispatches, reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case outbound.ModelsPath:
					reads.Add(1)
					body, _ := io.ReadAll(request.Body)
					if current.Load() == 0 {
						writer.Header().Set("Content-Type", "application/proto")
						writer.Write(domainCatalog())
						return
					}
					if !bytes.Contains(body, []byte("replacement-fixture-key")) {
						t.Error("catalog resolution reused another account's credential")
					}
					if mode == "unreadable-catalog" {
						writer.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					writer.Header().Set("Content-Type", "application/proto")
					writer.Write(nativeText(1, string(nativeText(22, "other-account-model"))))
				case outbound.ChatPath:
					dispatches.Add(1)
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Write(nativeFrame(0, nativeFields(nativeText(3, "unexpected answer"), nativeNumber(5, 2))))
					writer.Write(nativeFrame(2, []byte(`{}`)))
				default:
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			peer, _ := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			frame, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
			if frame.Error != nil {
				t.Fatal(frame)
			}
			credentials := outbound.Credentials{Key: "replacement-fixture-key"}
			peer.auth, _ = json.Marshal(credentials)
			peer.scope.Account, peer.scope.Generation = credentials.AccountID(server.URL), 4
			current.Store(1)
			model := "account-only"
			if mode == "absent-static-alias" {
				model = "gemini"
			}
			body, _ := json.Marshal(map[string]any{"model": model, "messages": []any{map[string]string{"role": "user", "content": "hello"}}})
			request := fixtureExecute(body, sdk.ExecuteOptions{})
			request.Model = model
			frame, events := peer.call(t, sdk.MethodProviderExecute, request)
			if frame.Error == nil || frame.Error.Outcome != "not-started" || dispatches.Load() != 0 || reads.Load() != 2 {
				t.Fatalf("stale/static family crossed current-account preflight: %+v dispatches=%d reads=%d", frame.Error, dispatches.Load(), reads.Load())
			}
			for _, event := range events {
				if event.Event == "accepted" {
					t.Fatal("unreadable or absent current family claimed the operation")
				}
			}
		})
	}
}

func TestProviderDomainResolvedFamilyKeepsGeminiSelectedModelOrigin(t *testing.T) {
	reply, signature := donorGeminiReply(t)
	const family = "gemini-4-hydrogen"
	arrivals := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == outbound.ModelsPath {
			info := nativeText(23, family)
			config := nativeFields(nativeText(22, donorGeminiModel), nativeText(23, string(info)))
			writer.Header().Set("Content-Type", "application/proto")
			writer.Write(nativeText(1, string(config)))
			return
		}
		if request.URL.Path != outbound.ChatPath {
			t.Error("family continuation reached another operation")
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		_, payload, err := codec.ReadConnectFrame(request.Body, request.Header.Get("Connect-Content-Encoding") == "gzip")
		if err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case arrivals <- payload:
		default:
			t.Error("duplicate Gemini family dispatch")
		}
		writer.Header().Set("Content-Type", "application/connect+proto")
		writer.Header().Set("Connect-Content-Encoding", "gzip")
		writer.Write(reply)
	}))
	t.Cleanup(server.Close)
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	frame, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	if frame.Error != nil {
		t.Fatal(frame)
	}
	first := map[string]any{"model": family, "messages": []any{map[string]string{"role": "user", "content": "cat calc.py"}}}
	frame, events := geminiExecute(t, peer, "chat", "family-caller", first)
	if frame.Error != nil {
		t.Fatal("family could not issue the genuine Gemini receipt", frame.Error)
	}
	next := geminiContinuation("chat")
	next["model"] = family
	frame, nextEvents := geminiExecute(t, peer, "chat", "family-caller", next)
	if frame.Error != nil {
		t.Fatal("resolved UID replaced selected-model receipt authority", frame.Error)
	}
	for _, callEvents := range [][]sdk.Frame{events, nextEvents} {
		var headers sdk.HeadersEvent
		for _, event := range callEvents {
			if event.Event == "headers" {
				if err := json.Unmarshal(event.Data, &headers); err != nil {
					t.Fatal(err)
				}
			}
		}
		if headers.SentModel != donorGeminiModel {
			t.Fatalf("Devin sent UID metadata = %q, want %q", headers.SentModel, donorGeminiModel)
		}
	}
	for index := 0; index < 2; index++ {
		select {
		case payload := <-arrivals:
			if oracleValue(t, payload, 21) != donorGeminiModel {
				t.Fatal("family was sent without its actual UID")
			}
			if index == 1 && !bytes.Contains(payload, []byte(signature)) {
				t.Fatal("resolved family lost its selected-model-bound thought signature")
			}
		default:
			t.Fatal("family continuation lacked an observed dispatch")
		}
	}
}

func TestProviderDomainLabelOnlyFamiliesKeepConcreteSDKIdentities(t *testing.T) {
	catalog := nativeText(1, string(nativeFields(nativeText(1, "Claude Haiku 4.5"),
		nativeText(22, "MODEL_PRIVATE_11"), nativeNumber(18, 32768))))
	catalog = append(catalog, nativeText(1, string(nativeFields(nativeText(1, "Offline free"),
		nativeText(22, "fixture-free"), nativeNumber(18, 32768), nativeNumber(24, 4))))...)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case outbound.ModelsPath:
			writer.Header().Set("Content-Type", "application/proto")
			writer.Write(catalog)
		case outbound.StatusPath:
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, `{"userStatus":{"planStatus":{"dailyQuotaRemainingPercent":75}}}`)
		default:
			t.Error("label-only model discovery dispatched inference")
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	module, err := NewAgent(ModuleOptions{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	peer.host = store.host
	selectAccount(peer, server.URL)
	frame, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	var result sdk.ModelsResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil {
		t.Fatal("label-only catalog failed", frame)
	}
	manifest := module.Manifest
	manifest.Providers = slices.Clone(manifest.Providers)
	manifest.Providers[0].Models = result.Models
	if err := manifest.Validate(); err != nil {
		t.Fatalf("display-only family label became an invalid SDK identity: %v", err)
	}
	if len(result.Models) != 2 || result.Models[0].ID != "MODEL_PRIVATE_11" ||
		result.Models[0].Name != "Claude Haiku 4.5" || result.Models[1].ID != "fixture-free" ||
		result.Models[1].Name != "Offline free" || !result.Models[1].Free {
		t.Fatal("label-only families lost concrete UID, display name or Free scope", result.Models)
	}
	frame, _ = peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "19"})
	var usage sdk.UsageResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &usage) != nil {
		t.Fatal("label-only quota failed", frame)
	}
	if _, fresh, err := sdk.ValidateUsageResult(module.Manifest, frame.Result); err != nil || !fresh {
		t.Fatal("label-only quota violated SDK model identity grammar", fresh, err)
	}
	if len(usage.Windows) != 1 || !slices.Equal(usage.Windows[0].NotModels, []string{"fixture-free"}) {
		t.Fatal("label-only Free model gained a fabricated quota alias", usage.Windows)
	}
}

func TestProviderDomainAPI9QuotaDoesNotInventAbsentLimit(t *testing.T) {
	for _, fixture := range []struct {
		name, limit, display string
	}{
		{"absent", "", "1.5 / unknown ACUs"},
		{"null", `,"acuLimit":null`, "1.5 / unknown ACUs"},
		{"zero", `,"acuLimit":0`, "1.5 / 0 ACUs"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case outbound.ModelsPath:
					writer.Header().Set("Content-Type", "application/proto")
					writer.Write(domainCatalog())
				case outbound.StatusPath:
					writer.Header().Set("Content-Type", "application/json")
					io.WriteString(writer, `{"userStatus":{"planStatus":{"acuConsumed":1.5`+fixture.limit+`}}}`)
				default:
					t.Error("quota read dispatched inference")
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			module, err := NewAgent(ModuleOptions{Server: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			peer := startSDKPeer(t, module)
			store := &stateFixture{rows: map[string]sdk.StateValue{}}
			peer.host = store.host
			selectAccount(peer, server.URL)
			frame, _ := peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "19"})
			var result sdk.UsageResult
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || len(result.Windows) != 1 {
				t.Fatal("partial native quota read failed", frame)
			}
			window := result.Windows[0]
			if window.Display != fixture.display || !window.Unknown || window.Amount == nil ||
				*window.Amount != 1.5 || window.Unit != "ACUs" || window.Limit != 0 || !window.Aside {
				t.Fatalf("absent native limit became a claimed zero limit: %+v", window)
			}
			if _, _, err := sdk.ValidateUsageResult(module.Manifest, frame.Result); err != nil {
				t.Fatal("partial native quota violated the SDK contract", err)
			}
		})
	}
}
