package devin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func executionFactsFixture() []byte {
	usage := nativeFields(nativeNumber(2, 7), nativeNumber(3, 0), nativeNumber(4, 5), nativeNumber(5, 11),
		nativeNumber(6, 4), nativeText(7, "usage-message"), nativeText(9, "usage-model"))
	cost := binary.AppendUvarint(nil, 22<<3|1)
	cost = binary.LittleEndian.AppendUint64(cost, math.Float64bits(2.5))
	return nativeFrame(0, nativeFields(nativeText(1, "native-message"), nativeText(17, "native-request"),
		nativeText(23, "served-model"), nativeNumber(18, 0), cost, nativeText(7, string(usage)),
		nativeText(3, "answer"), nativeNumber(5, 2)))
}

func TestExecutionFactsAndCodeOnlyDiagnosticsSurviveTerminalFailure(t *testing.T) {
	for _, refused := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "refused"}[refused], func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				writer.Header().Set("Content-Type", "application/connect+proto")
				writer.Write(executionFactsFixture())
				terminal := []byte(`{}`)
				if refused {
					terminal = []byte(`{"error":{"code":"permission_denied","message":"private vendor details"}}`)
				}
				writer.Write(nativeFrame(2, terminal))
			}))
			defer server.Close()
			peer, store := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			var diagnostics []sdk.Diagnostic
			peer.host = func(frame sdk.Frame) (any, error) {
				if frame.Method != sdk.MethodDiagnosticEmit {
					return store.host(frame)
				}
				var value sdk.Diagnostic
				if json.Unmarshal(frame.Params, &value) != nil || frame.Auth != nil || frame.Scope != peer.callScope() {
					t.Fatal("diagnostic lost its host scope or exposed auth")
				}
				encoded, _ := json.Marshal(value)
				if bytes.Contains(encoded, []byte("selected-fixture-secret")) || bytes.Contains(encoded, []byte("private")) ||
					bytes.Contains(encoded, []byte("caller content")) {
					t.Fatal("diagnostic contains free text or credentials", string(encoded))
				}
				diagnostics = append(diagnostics, value)
				return nil, errors.New("synthetic diagnostic refusal")
			}
			result, events := peer.call(t, sdk.MethodProviderExecute, fixtureExecute(
				[]byte(`{"model":"fixture-model","stream":true,"messages":[{"role":"user","content":"caller content"}]}`),
				sdk.ExecuteOptions{}))
			if (result.Error != nil) != refused || requests.Load() != 1 || len(diagnostics) != 2 {
				t.Fatal("diagnostic denial changed execution or acceptance", result, requests.Load(), diagnostics)
			}
			accepted := 0
			var last sdk.Usage
			for _, event := range events {
				switch event.Event {
				case "accepted":
					accepted++
				case "usage":
					if json.Unmarshal(event.Data, &last) != nil {
						t.Fatal("malformed usage snapshot")
					}
				}
			}
			if accepted != 1 || last.Facts == nil || last.Facts.InputTokens == nil || *last.Facts.InputTokens != 7 ||
				last.Facts.OutputTokens == nil || *last.Facts.OutputTokens != 0 || last.Facts.Cost == nil ||
				last.Facts.Cost.Currency != "ACU" || last.Facts.Cost.Amount != 2.5 {
				t.Fatal("measured usage disappeared on terminal failure", last)
			}
			if refused && (last.Facts.ErrorType == nil || *last.Facts.ErrorType != "permission_denied") {
				t.Fatal("failed terminal lost its safe outcome type", last.Facts)
			}
			final := diagnostics[len(diagnostics)-1]
			suffix, level := ".completed", "info"
			if refused {
				suffix, level = ".failed", "warn"
			}
			if !strings.HasSuffix(final.Code, suffix) || final.Level != level || final.Status != 200 {
				t.Fatal("diagnostic lost actual response status and local phase", final)
			}
		})
	}
}

func TestExecutionFactsCannotLeakSelectedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/connect+proto")
		writer.Write(nativeFrame(0, nativeFields(nativeText(17, "selected-fixture-secret"), nativeText(3, "answer"), nativeNumber(5, 2))))
		writer.Write(nativeFrame(2, []byte(`{}`)))
	}))
	defer server.Close()
	peer, _ := providerPeer(t, server.URL)
	selectAccount(peer, server.URL)
	result, events := peer.call(t, sdk.MethodProviderExecute, fixtureExecute(
		[]byte(`{"model":"fixture-model","messages":[{"role":"user","content":"hello"}]}`), sdk.ExecuteOptions{}))
	encoded, _ := json.Marshal(events)
	if result.Error == nil || bytes.Contains(encoded, []byte("selected-fixture-secret")) ||
		bytes.Contains(encoded, []byte("c2VsZWN0ZWQtZml4dHVyZS1zZWNyZXQ")) {
		t.Fatal("credential-bearing execution metadata escaped the scoped boundary", result)
	}
}

func TestExecutionFactsPreserveMeasuredNativeUnitsAndExplicitZero(t *testing.T) {
	wire := append(executionFactsFixture(), nativeFrame(2, []byte(`{}`))...)
	for _, raw := range []bool{false, true} {
		var summary codec.NativeSummary
		var err error
		if raw {
			summary, err = codec.ReadNativeRawStream(t.Context(), bytes.NewReader(wire), false, func(codec.NativeFrame) error { return nil })
		} else {
			summary, err = codec.ReadNativeStream(t.Context(), bytes.NewReader(wire), false, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		usage := executeUsage(summary)
		encoded, _ := json.Marshal(usage)
		var value struct {
			Facts map[string]json.RawMessage `json:"facts"`
		}
		json.Unmarshal(encoded, &value)
		for key, expected := range map[string]string{
			"inputTokens": "7", "outputTokens": "0", "cachedTokens": "11", "cacheWriteTokens": "5",
			"servedModel": `"served-model"`, "upstreamProvider": `"API_PROVIDER_ANTHROPIC"`,
			"requestId": `"native-request"`, "responseId": `"native-message"`, "credits": "0",
			"stop": `"STOP_PATTERN"`, "cost": `{"amount":2.5,"currency":"ACU"}`,
		} {
			if !bytes.Equal(value.Facts[key], []byte(expected)) {
				t.Fatalf("raw=%v fact %s lost measured native value: %s", raw, key, encoded)
			}
		}
		for _, absent := range []string{"reasoningTokens", "cacheWrite1hTokens", "deferred", "tier", "errorType"} {
			if _, found := value.Facts[absent]; found {
				t.Fatalf("unmeasured %s was inferred: %s", absent, encoded)
			}
		}
		if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CachedTokens != 0 ||
			usage.CacheWriteTokens != 0 || usage.Cost != 0 || usage.Currency != "" {
			t.Fatal("facts mixed with legacy totals", string(encoded))
		}
	}
}

func TestExecutionFactsAbsentUsageDoesNotInventTokenCounts(t *testing.T) {
	wire := nativeFields(nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeNumber(5, 2))),
		nativeFrame(2, []byte(`{}`)))
	summary, err := codec.ReadNativeStream(t.Context(), bytes.NewReader(wire), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(executeUsage(summary))
	var value struct {
		Facts map[string]json.RawMessage `json:"facts"`
	}
	json.Unmarshal(encoded, &value)
	if string(value.Facts["stop"]) != `"STOP_PATTERN"` {
		t.Fatal("native stop disappeared with absent usage", string(encoded))
	}
	for _, field := range []string{"inputTokens", "outputTokens", "cachedTokens", "cacheWriteTokens", "credits", "cost"} {
		if _, found := value.Facts[field]; found {
			t.Fatal("missing native fact became zero", field, string(encoded))
		}
	}
}
