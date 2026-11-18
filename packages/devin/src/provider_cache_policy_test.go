package devin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func TestCacheKeepaliveDefaultOffOptionsAndSentTimeCadence(t *testing.T) {
	policy, err := cachePolicy(nil, nil, nil)
	if err != nil || policy.Enabled || policy.Interval != 285*time.Second || policy.MaxPings != 6 {
		t.Fatal("paid work is enabled or defaults changed", policy, err)
	}
	enabled, interval, maximum := true, 120.5, 2
	policy, err = cachePolicy(&enabled, &interval, &maximum)
	if err != nil || !policy.Enabled || policy.Interval != 120500*time.Millisecond || policy.MaxPings != 2 {
		t.Fatal("explicit bounded policy was not preserved", policy, err)
	}
	for _, invalid := range []float64{0, 29.9, 299.1, math.NaN(), math.Inf(1)} {
		if _, err := cachePolicy(&enabled, &invalid, nil); err == nil {
			t.Fatal("invalid paid cadence was admitted", invalid)
		}
	}
	for _, invalid := range []int{-1, 0, 51} {
		if _, err := cachePolicy(&enabled, nil, &invalid); err == nil {
			t.Fatal("invalid paid ping count was admitted", invalid)
		}
	}
	policy = defaultCacheKeepalivePolicy()
	sent := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		elapsed time.Duration
		wait    time.Duration
	}{
		{46800 * time.Millisecond, 238200 * time.Millisecond},
		{15 * time.Second, 270 * time.Second},
		{300 * time.Second, 0},
	} {
		now := sent.Add(test.elapsed)
		if got := policy.due(sent, now).Sub(now); got != test.wait {
			t.Fatal("cadence was timed from response completion", got, test.wait)
		}
	}
}

func TestCacheKeepaliveCheckedConfigurationRequiresExplicitBooleanConsent(t *testing.T) {
	for _, test := range []struct {
		values   string
		enabled  bool
		interval time.Duration
		maximum  int
	}{
		{`{}`, false, 285 * time.Second, 6},
		{`{"cacheKeepaliveInterval":30.5,"cacheKeepaliveMaxPings":1}`, false, 30500 * time.Millisecond, 1},
		{`{"keepAlive":false,"cacheKeepaliveInterval":299,"cacheKeepaliveMaxPings":50}`, false, 299 * time.Second, 50},
		{`{"keepAlive":true}`, true, 285 * time.Second, 6},
		{`{"keepAlive":true,"cacheKeepaliveInterval":30,"cacheKeepaliveMaxPings":1}`, true, 30 * time.Second, 1},
	} {
		var configuration cacheKeepaliveConfig
		if err := decodeParams(json.RawMessage(test.values), &configuration); err != nil {
			t.Fatal(err)
		}
		policy, err := configuration.policy()
		if err != nil || policy.Enabled != test.enabled || policy.Interval != test.interval ||
			policy.MaxPings != test.maximum {
			t.Fatal("checked configuration changed consent or cadence", test.values, policy, err)
		}
	}
	for _, values := range []string{
		`{"keepAlive":null}`, `{"keepAlive":1}`, `{"keepAlive":"true"}`,
		`{"cacheKeepaliveInterval":null}`, `{"cacheKeepaliveInterval":"285"}`,
		`{"cacheKeepaliveInterval":29.9}`, `{"cacheKeepaliveInterval":299.1}`,
		`{"cacheKeepaliveInterval":1e999}`, `{"cacheKeepaliveMaxPings":null}`,
		`{"cacheKeepaliveMaxPings":"6"}`, `{"cacheKeepaliveMaxPings":1.5}`,
		`{"cacheKeepaliveMaxPings":0}`, `{"cacheKeepaliveMaxPings":51}`,
		`{"keepAlive":true,"keepAlive":false}`, `{"cacheKeepalive":true}`,
		`{"KEEPALIVE":true}`, `{"KeepAlive":true}`, `{"cache_keepalive":true}`,
		`{"cachekeepaliveInterval":285}`, `{"cacheKeepalivemaxPings":6}`,
		`null`, `[]`, `{"keepAlive":true} {}`,
	} {
		var configuration cacheKeepaliveConfig
		err := decodeParams(json.RawMessage(values), &configuration)
		if err == nil {
			_, err = configuration.policy()
		}
		if err == nil {
			t.Fatal("invalid paid configuration was accepted", values)
		}
	}
}

func TestCacheKeepaliveMissBoundaryRefusalsAndMaximum(t *testing.T) {
	policy := defaultCacheKeepalivePolicy()
	policy.Enabled = true
	hit := &codec.NativeUsage{Input: 100, CacheWrite: 10, CacheRead: 110}
	miss := &codec.NativeUsage{Input: 100, CacheWrite: 10, CacheRead: 109}
	var progress cachePingProgress
	for position, usage := range []*codec.NativeUsage{miss, hit, miss, miss} {
		progress.Sent++
		if keep := progress.keep(policy, 200, usage); keep != (position != 3) {
			t.Fatal("two consecutive misses or intervening hit changed", position, progress)
		}
	}
	for _, status := range []int{401, 402, 429} {
		progress = cachePingProgress{Sent: 1}
		if progress.keep(policy, status, hit) {
			t.Fatal("refusal scheduled more paid work", status)
		}
	}
	for _, status := range []int{200, 502} {
		progress = cachePingProgress{Sent: 1}
		if progress.keep(policy, status, nil) {
			t.Fatal("missing measurements authorized another paid ping", status)
		}
	}
	progress = cachePingProgress{Sent: 6}
	if progress.keep(policy, 200, hit) {
		t.Fatal("maximum ping count was ignored")
	}
	progress = cachePingProgress{Sent: 1}
	policy.Enabled = false
	if progress.keep(policy, 200, hit) {
		t.Fatal("disabled paid policy continued")
	}
	policy.Enabled = true
	progress = cachePingProgress{Sent: 1}
	if progress.keep(policy, 200, &codec.NativeUsage{Input: math.MaxUint64, CacheWrite: math.MaxUint64,
		CacheRead: math.MaxUint64}) && progress.Misses != 1 {
		t.Fatal("overflow manufactured a cache hit")
	}
}

func TestCacheConversationKeysKeepLeadSubagentAndPermissionChecksIndependent(t *testing.T) {
	for _, session := range []bool{false, true} {
		t.Run(fmt.Sprint(session), func(t *testing.T) {
			var index cacheConversationIndex
			headers := make(http.Header)
			if session {
				headers.Set("X-Magpie-Session", "session")
			}
			ask := func(system, first string, extra ...map[string]any) []byte {
				parts := []map[string]any{{"type": "text", "text": first, "cache_control": map[string]string{"type": "ephemeral"}}}
				parts = append(parts, extra...)
				body, _ := json.Marshal(map[string]any{"model": "model", "system": system,
					"messages": []any{map[string]any{"role": "user", "content": parts}},
					"tools":    []any{map[string]any{"name": "Read"}}})
				return body
			}
			key := func(caller string, body []byte, headers http.Header) string {
				result, err := index.key(caller, "anthropic", body, headers)
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			lead := ask("You are the lead", "History A")
			check := ask("Permission classifier", "History A")
			checkKey := key("host-caller", check, headers)
			grown := ask("Permission classifier", "History A", map[string]any{"type": "text", "text": "History B"})
			if key("host-caller", grown, headers) != checkKey {
				t.Fatal("growing permission check created a second conversation")
			}
			leadKey := key("host-caller", lead, headers)
			child := headers.Clone()
			child.Set("X-Magpie-Subagent", "child")
			if leadKey == checkKey || key("host-caller", lead, child) == leadKey ||
				key("other-caller", lead, headers) == leadKey {
				t.Fatal("conversation keys mixed caller, lead, classifier or subagent")
			}
			if session {
				merged := ask("Permission classifier", "entirely changed middle and history")
				if key("host-caller", merged, headers) != checkKey {
					t.Fatal("session-bound classifier growth changed key")
				}
			}
			noMark := strings.ReplaceAll(string(lead), `,"cache_control":{"type":"ephemeral"}`, "")
			if key("host-caller", []byte(noMark), headers) != leadKey {
				t.Fatal("moving a cache boundary changed conversation identity")
			}
		})
	}
}

func TestCacheConversationNoSessionMatchesLongestGrowthWithoutRetainingText(t *testing.T) {
	var index cacheConversationIndex
	key := func(first string) string {
		body, _ := json.Marshal(map[string]any{"system": "S", "messages": []any{map[string]string{"role": "user", "content": first}}})
		value, err := index.key("caller", "anthropic", body, nil)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	original := key("private history 🐦")
	if key("private history 🐦, then more") != original || key("unrelated private history") == original {
		t.Fatal("first-message growth lost original conversation or mixed unrelated conversations")
	}
	encoded, _ := json.Marshal(index.first)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "history") {
		t.Fatal("no-session matching retained caller text", string(encoded))
	}
	for position := 0; position < 300; position++ {
		key(fmt.Sprintf("different-%d", position))
	}
	if len(index.first) != 256 {
		t.Fatal("fallback index is not bounded", len(index.first))
	}
}

func TestCacheConversationHeaderProjectionAndBillingLine(t *testing.T) {
	var index cacheConversationIndex
	first := []byte(`{"system":"x-anthropic-billing-header: cc_version=1;\nS","messages":[{"role":"user","content":"private"}]}`)
	second := []byte(`{"system":"S","messages":[{"role":"user","content":"other"}]}`)
	projected := http.Header{"X-Magpie-Session": {"s"}, "X-Magpie-Subagent": {"a"}}
	named := http.Header{"X-Claude-Code-Session-Id": {"s"}, "X-Claude-Code-Agent-Id": {"a"}}
	left, err := index.key("caller", "anthropic", first, projected)
	right, otherErr := index.key("caller", "anthropic", second, named)
	if err != nil || otherErr != nil || left != right {
		t.Fatal("projected grouping headers or billing noise changed identity")
	}
	for _, invalid := range []struct {
		caller   string
		protocol string
		body     string
	}{
		{"", "chat", `{"messages":[{"role":"user","content":"hi"}]}`},
		{"caller", "other", `{"messages":[{"role":"user","content":"hi"}]}`},
		{"caller", "chat", `{}`},
		{"caller", "anthropic", `{"messages":[{"role":"user","content":42}]}`},
	} {
		if _, err := index.key(invalid.caller, invalid.protocol, []byte(invalid.body), nil); err == nil {
			t.Fatal("invalid conversation was keyed", invalid)
		}
	}
}

func TestCacheConversationIndexConcurrentGrowth(t *testing.T) {
	var index cacheConversationIndex
	var wait sync.WaitGroup
	for position := 0; position < 32; position++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := index.key("caller", "chat", []byte(`{"messages":[{"role":"user","content":"same"}]}`), nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
	if len(index.first) != 1 {
		t.Fatal("concurrent matching duplicated a conversation", len(index.first))
	}
}
