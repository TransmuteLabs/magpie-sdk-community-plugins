package devin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
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

func canonicalFixtureRequest(protocol string, streaming bool) sdk.ExecuteRequest {
	body := fmt.Appendf(nil, `{"model":"fixture-model","stream":%t,"max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`, streaming)
	request := fixtureExecute(body, sdk.ExecuteOptions{CallerID: "fixture-caller"})
	if protocol == "anthropic" {
		request.Protocol = protocol
		request.Options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "fixture-caller"})
	}
	return request
}

func canonicalFixtureBody(t *testing.T, events []sdk.Frame) string {
	t.Helper()
	var body strings.Builder
	for _, event := range events {
		if event.Event == "data" {
			var data []byte
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatal(err)
			}
			body.Write(data)
		}
	}
	return body.String()
}

func TestProviderMinLogProbabilityCompletesCanonicalReply(t *testing.T) {
	usage := nativeFields(nativeNumber(2, 7), nativeNumber(3, 3), nativeNumber(4, 5), nativeNumber(5, 11))
	reply := nativeFields(nativeFrame(0, nativeText(3, "fixture answer")), nativeFrame(0, nativeNumber(5, 4)),
		nativeFrame(0, nativeText(7, string(usage))))
	terminal := nativeFrame(2, []byte(`{}`))
	for _, protocol := range []struct {
		name, stop, usage, streamEnd string
	}{
		{"chat", `"finish_reason":"stop"`, `"completion_tokens":3`, "data: [DONE]\n\n"},
		{"anthropic", `"stop_reason":"end_turn"`, `"output_tokens":3`, "data: {\"type\":\"message_stop\"}\n\n"},
	} {
		for _, completion := range []struct {
			name      string
			streaming bool
			tail      []byte
			wantOK    bool
		}{
			{"json", false, terminal, true},
			{"sse", true, terminal, true},
			{"missing-terminal", true, nil, false},
			{"conflicting-stop", true, nativeFields(nativeFrame(0, nativeNumber(5, 2)), terminal), false},
		} {
			t.Run(protocol.name+"/"+completion.name, func(t *testing.T) {
				var sends atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					sends.Add(1)
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Write(reply)
					writer.Write(completion.tail)
				}))
				defer server.Close()
				peer, _ := providerPeer(t, server.URL)
				selectAccount(peer, server.URL)
				result, events := peer.call(t, sdk.MethodProviderExecute, canonicalFixtureRequest(protocol.name, completion.streaming))
				body := canonicalFixtureBody(t, events)
				if sends.Load() != 1 {
					t.Fatal("canonical reply was replayed", sends.Load())
				}
				if !completion.wantOK {
					if result.Error == nil || result.Error.Code != "data_loss" || result.Error.Outcome != "uncertain" ||
						strings.Contains(body, protocol.stop) || strings.Contains(body, protocol.streamEnd) {
						t.Fatal("invalid Connect completion acquired a successful finish", result.Error, body)
					}
					return
				}
				var final sdk.ExecuteResult
				if result.Error != nil || json.Unmarshal(result.Result, &final) != nil || final.Outcome != "completed" ||
					final.Status != http.StatusOK || final.Usage == nil || final.Usage.Facts == nil ||
					final.Usage.Facts.Stop == nil || *final.Usage.Facts.Stop != "MIN_LOG_PROB" {
					t.Fatal("MIN_LOG_PROB did not complete or lost the measured stop", result.Error, string(result.Result))
				}
				if !strings.Contains(body, "fixture answer") || !strings.Contains(body, protocol.stop) || !strings.Contains(body, protocol.usage) ||
					completion.streaming && !strings.HasSuffix(body, protocol.streamEnd) ||
					!completion.streaming && !json.Valid([]byte(body)) {
					t.Fatal("canonical body lost the reply, late usage or terminator", body)
				}
			})
		}
	}
}

func TestProviderConnectPreconditionRetainsCanonicalFailure(t *testing.T) {
	const message = "Fixture private vendor precondition"
	for _, protocol := range []string{"chat", "anthropic"} {
		for _, content := range []struct {
			name  string
			delta []byte
		}{
			{"before-delta", nil},
			{"after-delta", nativeFrame(0, nativeFields(nativeText(3, "partial reply"), nativeText(7, string(nativeNumber(2, 7)))))},
		} {
			t.Run(protocol+"/"+content.name, func(t *testing.T) {
				var sends atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					sends.Add(1)
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Write(content.delta)
					writer.Write(nativeFrame(2, []byte(`{"error":{"code":"failed_precondition","type":"fixture_type","message":"`+message+`"}}`)))
				}))
				defer server.Close()
				peer, store := providerPeer(t, server.URL)
				selectAccount(peer, server.URL)
				var classified []sdk.RequestRecoveryRequest
				peer.host = func(frame sdk.Frame) (any, error) {
					if frame.Method != sdk.MethodRequestRecover {
						return store.host(frame)
					}
					var request sdk.RequestRecoveryRequest
					if err := json.Unmarshal(frame.Params, &request); err != nil {
						t.Fatal(err)
					}
					classified = append(classified, request)
					return sdk.RequestRecoveryResult{Step: request.Step, Action: "stop"}, nil
				}
				result, events := peer.call(t, sdk.MethodProviderExecute, canonicalFixtureRequest(protocol, true))
				if result.Error == nil || result.Error.Code != "failed_precondition" || result.Error.Outcome != "uncertain" ||
					strings.Contains(result.Error.Message, message) || sends.Load() != 1 {
					t.Fatal("canonical terminal lost its public type, leaked details or replayed", result.Error, sends.Load())
				}
				if len(classified) != 1 {
					t.Fatal("terminal did not classify its observed response once", classified)
				}
				facts := classified[0].Facts
				if classified[0].Evidence != "" || facts.Code != "failed_precondition" || facts.Type != "fixture_type" ||
					facts.Message != message || facts.Status != http.StatusOK || !facts.Complete ||
					facts.Operation != sdk.MethodProviderExecute || facts.Phase != "response" {
					t.Fatal("terminal altered observed failure facts or invented replay evidence", classified[0])
				}
				body := canonicalFixtureBody(t, events)
				if strings.Contains(body, message) || strings.Contains(body, "[DONE]") || strings.Contains(body, "message_stop") ||
					strings.Contains(body, "partial reply") != (content.delta != nil) {
					t.Fatal("canonical body lost partial output, leaked details or fabricated a finish", body)
				}
				if content.delta != nil {
					var last sdk.Usage
					for _, event := range events {
						if event.Event == "usage" {
							if err := json.Unmarshal(event.Data, &last); err != nil {
								t.Fatal(err)
							}
						}
					}
					if last.Facts == nil || last.Facts.InputTokens == nil || *last.Facts.InputTokens != 7 ||
						last.Facts.ErrorType == nil || *last.Facts.ErrorType != "failed_precondition" {
						t.Fatal("partial response lost measured usage or its typed terminal", last.Facts)
					}
				}
			})
		}
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
