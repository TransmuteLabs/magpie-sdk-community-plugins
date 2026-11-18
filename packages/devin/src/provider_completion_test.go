package devin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDonor042CatalogFailureIsNotSnapshotOrEmptySuccess(t *testing.T) {
	var unavailable atomic.Bool
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sends.Add(1)
		if request.URL.Path != outbound.ModelsPath {
			t.Error("model listing reached a different endpoint")
		}
		if unavailable.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/proto")
		writer.Write(domainCatalog())
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	first, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	var good sdk.ModelsResult
	if first.Error != nil || json.Unmarshal(first.Result, &good) != nil || len(good.Models) == 0 {
		t.Fatal("current selected-account catalog unavailable", first)
	}
	unavailable.Store(true)
	failed, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	if failed.Error == nil || len(failed.Result) != 0 {
		t.Fatal("unavailable catalog became a fresh snapshot/empty success", failed)
	}
	unavailable.Store(false)
	recovered, _ := peer.call(t, sdk.MethodProviderModels, sdk.ModelsRequest{})
	if recovered.Error != nil || !bytes.Equal(first.Result, recovered.Result) || sends.Load() != 3 {
		t.Fatal("current catalog failed to recover or requests were repeated", recovered.Error, sends.Load())
	}
}

func TestProviderDimensionUsageFactsKeepSparsePresence(t *testing.T) {
	dimension := binary.LittleEndian.AppendUint32([]byte{2<<3 | 5}, math.Float32bits(0))
	metric := nativeFields(nativeText(4, string(dimension)), nativeText(5, "output_tokens"))
	group := nativeFields(nativeText(1, "Token Usage"), nativeText(2, string(metric)))
	summary, err := codec.DecodeNativeMetrics(nativeText(28, string(group)))
	if err != nil {
		t.Fatal(err)
	}
	usage := executeUsage(summary)
	if usage == nil || usage.Facts == nil || usage.Facts.OutputTokens == nil || *usage.Facts.OutputTokens != 0 ||
		usage.Facts.InputTokens != nil || usage.Facts.CachedTokens != nil || usage.Facts.CacheWriteTokens != nil {
		t.Fatal("dimension measurement was lost or absent siblings became measured zeros", usage)
	}
	chat := outbound.CanonicalUsage(summary)
	messages := outbound.MessagesUsage(summary)
	if len(chat) != 1 || chat["completion_tokens"] != uint64(0) || len(messages) != 1 || messages["output_tokens"] != 0 {
		t.Fatal("canonical usage invented unnamed counters/totals", chat, messages)
	}
	progress := cachePingProgress{}
	if progress.keep(cacheKeepalivePolicy{Enabled: true, MaxPings: 6}, 200, summary.Usage) {
		t.Fatal("incomplete dimension measurements authorized another paid ping")
	}
}

func TestProviderStreamRefusalBeforeContentKeepsFailureStatus(t *testing.T) {
	for _, protocol := range []string{"chat", "anthropic"} {
		for _, code := range []string{"resource_exhausted", "unauthenticated"} {
			t.Run(protocol+"/"+code, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Write(nativeFrame(0, nil))
					writer.Write(nativeFrame(2, []byte(`{"error":{"code":"`+code+`","message":"PRIVATE"}}`)))
				}))
				defer server.Close()
				peer, _ := providerPeer(t, server.URL)
				selectAccount(peer, server.URL)
				request := fixtureExecute([]byte(`{"model":"fixture-model","stream":true,"max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`), sdk.ExecuteOptions{})
				request.Protocol = protocol
				if protocol == "anthropic" {
					request.Options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "fixture-caller"})
				}
				result, events := peer.call(t, sdk.MethodProviderExecute, request)
				want := http.StatusTooManyRequests
				if code == "unauthenticated" {
					want = http.StatusUnauthorized
				}
				headers := 0
				for _, event := range events {
					if event.Event == "headers" {
						var header sdk.HeadersEvent
						if json.Unmarshal(event.Data, &header) != nil || header.Status != want {
							t.Fatal("metadata opened synthetic success before refusal", event)
						}
						headers++
					}
					if strings.Contains(string(event.Data), "PRIVATE") {
						t.Fatal("vendor message escaped")
					}
				}
				if result.Error == nil || result.Error.Code != code || result.Error.Outcome != "uncertain" || calls.Load() != 1 || headers != 1 {
					t.Fatal("refusal became success/replay or lost typed failure", result, calls.Load(), headers)
				}
			})
		}
	}
}

func TestProviderAccountIdentityUsesSelectedStatusLabels(t *testing.T) {
	for _, test := range []struct {
		name, status, want string
	}{
		{"Devin organization wins", `{"email":"person@example.invalid","teamName":"User team","planStatus":{"planInfo":{"teamName":"Plan team","devinInfo":{"accountDisplayName":"Devin org"}}}}`, "person@example.invalid · Devin org"},
		{"plan team fallback", `{"email":"person@example.invalid","teamName":"User team","planStatus":{"planInfo":{"teamName":"Plan team"}}}`, "person@example.invalid · Plan team"},
		{"user team fallback", `{"name":"handle","teamName":"User team"}`, "handle · User team"},
		{"trim display", `{"email":" person@example.invalid ","planStatus":{"planInfo":{"devinInfo":{"accountDisplayName":" Devin org "}}}}`, "person@example.invalid · Devin org"},
		{"do not repeat identity", `{"name":"Team","planStatus":{"planInfo":{"devinInfo":{"accountDisplayName":"TEAM"}}}}`, "Team"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != outbound.StatusPath {
					t.Error("sign-in reached a non-status endpoint")
					writer.WriteHeader(500)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				io.WriteString(writer, `{"userStatus":`+test.status+`}`)
			}))
			defer server.Close()
			module, err := NewAgent(ModuleOptions{Server: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			peer := startSDKPeer(t, module)
			store := &stateFixture{rows: map[string]sdk.StateValue{}}
			peer.host = store.host
			peer.scope = sdk.Scope{Provider: ProviderID}
			credentials := outbound.Credentials{Key: "fixture-key"}
			input, _ := json.Marshal(credentials)
			result, _ := peer.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: "api-key", Input: input})
			var started sdk.AuthStartResult
			if result.Error != nil || json.Unmarshal(result.Result, &started) != nil {
				t.Fatal("sign-in did not start", result)
			}
			result, _ = peer.call(t, sdk.MethodAuthFinish, sdk.AuthFinishRequest{Continuation: started.Continuation})
			var account sdk.AuthResult
			if result.Error != nil || json.Unmarshal(result.Result, &account) != nil {
				t.Fatal("sign-in did not finish", result)
			}
			if account.Name != test.want || account.ID != credentials.AccountID(server.URL) {
				t.Errorf("selected status label/fingerprint: got %q/%q, want %q/%q", account.Name, account.ID, test.want, credentials.AccountID(server.URL))
			}
		})
	}
}

func TestProviderUsageAuthenticationPrecedesCatalogAndRecovers(t *testing.T) {
	for _, test := range []struct {
		name                  string
		status, models        int
		statusBody, modelBody string
		auth, deferred        bool
	}{
		{"status bare 401", 401, 503, "PRIVATE expired", "PRIVATE outage", true, false},
		{"status typed 401", 401, 200, `{"code":"unauthenticated","message":"PRIVATE"}`, "", true, false},
		{"status Connect refusal", 502, 200, `{"error":{"code":"unauthenticated","message":"PRIVATE"}}`, "", true, false},
		{"catalog bare 401", 200, 401, "", "PRIVATE refused", true, false},
		{"catalog typed 401", 200, 401, "", `{"code":"unauthenticated","message":"PRIVATE"}`, true, false},
		{"catalog transient", 200, 503, "", "PRIVATE unavailable", false, true},
		{"catalog malformed", 200, 200, "", "malformed", false, true},
		{"current authority", 200, 200, "", "", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			recovered := false
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				paths = append(paths, request.URL.Path)
				switch request.URL.Path {
				case outbound.StatusPath:
					writer.Header().Set("Content-Type", "application/json")
					if !recovered && test.status != 200 {
						writer.WriteHeader(test.status)
						io.WriteString(writer, test.statusBody)
						return
					}
					io.WriteString(writer, `{"userStatus":{"planStatus":{"dailyQuotaRemainingPercent":75}}}`)
				case outbound.ModelsPath:
					writer.Header().Set("Content-Type", "application/proto")
					if !recovered && (test.models != 200 || test.modelBody != "") {
						writer.WriteHeader(test.models)
						io.WriteString(writer, test.modelBody)
						return
					}
					writer.Write(nativeText(1, string(nativeFields(nativeText(22, "free-fixture"), nativeNumber(24, 4)))))
				default:
					t.Error("usage reached inference or a companion endpoint")
					writer.WriteHeader(500)
				}
			}))
			defer server.Close()
			module, err := NewAgent(ModuleOptions{Server: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			peer := startSDKPeer(t, module)
			store := &stateFixture{rows: map[string]sdk.StateValue{}}
			peer.host = store.host
			selectAccount(peer, server.URL)
			result, _ := peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "7"})
			mu.Lock()
			gotPaths := slices.Clone(paths)
			mu.Unlock()
			wantPaths := []string{outbound.StatusPath}
			if test.status == 200 {
				wantPaths = append(wantPaths, outbound.ModelsPath)
			}
			if !slices.Equal(gotPaths, wantPaths) {
				t.Errorf("authentication must be checked before free scope: got %v want %v", gotPaths, wantPaths)
			}
			if test.auth {
				code, message := "unauthenticated", "sign in again"
				if strings.Contains(test.name, "bare") {
					code, message = "upstream", "HTTP 401"
				}
				if result.Error == nil || result.Error.Code != code || !strings.Contains(result.Error.Message, message) {
					t.Errorf("refused sign-in became deferred or opaque usage: %+v", result)
				}
			} else {
				var usage sdk.UsageResult
				if result.Error != nil || json.Unmarshal(result.Result, &usage) != nil || usage.ReadDeferred != test.deferred || usage.Revision != "7" {
					t.Fatalf("quota authority changed: %+v", result)
				}
				if test.deferred && (usage.Error == "" || len(usage.Windows) != 0) {
					t.Fatal("unreadable free scope invented usable quota", usage)
				}
			}
			if strings.Contains(string(result.Result), "PRIVATE") || result.Error != nil && strings.Contains(result.Error.Message, "PRIVATE") {
				t.Fatal("upstream error text leaked")
			}
			mu.Lock()
			paths, recovered = nil, true
			mu.Unlock()
			result, _ = peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "8"})
			var usage sdk.UsageResult
			if result.Error != nil || json.Unmarshal(result.Result, &usage) != nil || usage.Error != "" || usage.ReadDeferred ||
				usage.Revision != "8" || len(usage.Windows) != 1 || usage.Windows[0].Used != 25 ||
				!slices.Equal(usage.Windows[0].NotModels, []string{"free-fixture"}) {
				t.Fatal("fresh successful read did not recover current quota/free scope", result)
			}
		})
	}
}
