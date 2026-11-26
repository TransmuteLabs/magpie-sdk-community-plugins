package qoder

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type testDoer func(*http.Request) (*http.Response, error)

func (fn testDoer) Do(req *http.Request) (*http.Response, error) { return fn(req) }

func testHTTPFactory(handler func(*http.Request) (*http.Response, error)) HTTPFactory {
	return func(proxyURL string) (Doer, func(), error) {
		return testDoer(handler), func() {}, nil
	}
}

func testJSONResp(status int, payload any, extra http.Header) *http.Response {
	var text string
	switch typed := payload.(type) {
	case string:
		text = typed
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			panic(err)
		}
		text = string(encoded)
	}
	hdr := http.Header{"Content-Type": {"application/json"}}
	for key, vals := range extra {
		hdr[key] = slices.Clone(vals)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader(text)),
	}
}

func testSSEResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func qoderSSELines(statusCode int, innerBodies ...string) string {
	var out strings.Builder
	for _, inner := range innerBodies {
		frame, _ := json.Marshal(map[string]any{
			"statusCodeValue": statusCode,
			"body":            inner,
		})
		out.WriteString("data: ")
		out.Write(frame)
		out.WriteString("\n\n")
	}
	return out.String()
}

func testRawJSON(val any) json.RawMessage {
	if val == nil {
		return nil
	}
	if raw, ok := val.(json.RawMessage); ok {
		return raw
	}
	encoded, err := json.Marshal(val)
	if err != nil {
		panic(err)
	}
	return encoded
}

type qoderHarness struct {
	test     *testing.T
	conn     net.Conn
	enc      *json.Encoder
	dec      *json.Decoder
	seq      int
	mu       sync.Mutex
	replaced []json.RawMessage
	events   []sdk.Frame
}

func newQoderHarness(test *testing.T, mod sdk.Module) *qoderHarness {
	test.Helper()
	if err := mod.Validate(); err != nil {
		test.Fatalf("module validation failed: %v", err)
	}
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(test.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- sdk.Serve(ctx, worker, worker, mod) }()
	harness := &qoderHarness{
		test: test,
		conn: host,
		enc:  json.NewEncoder(host),
		dec:  json.NewDecoder(host),
	}
	test.Cleanup(func() {
		cancel()
		_ = host.Close()
		_ = worker.Close()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			test.Error("Qoder SDK fixture did not stop cleanly")
		}
	})
	frame, _ := harness.invoke(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent,
		Instance:  "qoder-test",
		Config:    sdk.ConfigSnapshot{Version: "1", Values: json.RawMessage(`{}`)},
	}, nil)
	if frame.Error != nil {
		test.Fatalf("configure failed: %v", frame.Error)
	}
	return harness
}

func (harness *qoderHarness) invoke(method string, scope sdk.Scope, params any, cred *Credential) (sdk.Frame, []byte) {
	harness.test.Helper()
	harness.seq++
	id := fmt.Sprintf("call-%d", harness.seq)
	scope.Plugin = ModuleID
	scope.Operation = id
	req := sdk.Frame{Kind: sdk.KindCall, ID: id, Method: method, Scope: scope, Params: testRawJSON(params)}
	if cred != nil {
		req.Auth = testRawJSON(cred)
	}
	_ = harness.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := harness.enc.Encode(req); err != nil {
		harness.test.Fatal(err)
	}
	var data []byte
	for {
		var frame sdk.Frame
		if err := harness.dec.Decode(&frame); err != nil {
			harness.test.Fatalf("decode during %s: %v", method, err)
		}
		switch frame.Kind {
		case sdk.KindEvent:
			harness.mu.Lock()
			harness.events = append(harness.events, frame)
			harness.mu.Unlock()
			if frame.Event == "data" {
				var chunk []byte
				if json.Unmarshal(frame.Data, &chunk) == nil {
					data = append(data, chunk...)
				}
			}
			if err := harness.enc.Encode(sdk.Frame{Kind: sdk.KindAck, ID: id, Seq: frame.Seq}); err != nil {
				harness.test.Fatal(err)
			}
		case sdk.KindHostCall:
			switch frame.Method {
			case sdk.MethodAccountReplace:
				var repReq sdk.AccountReplaceRequest
				_ = json.Unmarshal(frame.Params, &repReq)
				harness.mu.Lock()
				harness.replaced = append(harness.replaced, repReq.Credentials)
				harness.mu.Unlock()
				_ = harness.enc.Encode(sdk.Frame{
					Kind:   sdk.KindHostResult,
					ID:     frame.ID,
					Parent: frame.Parent,
					Result: testRawJSON(sdk.AccountReplaceResult{ID: "acct", Generation: 2}),
				})
			case sdk.MethodNetworkProxy:
				_ = harness.enc.Encode(sdk.Frame{
					Kind:   sdk.KindHostResult,
					ID:     frame.ID,
					Parent: frame.Parent,
					Result: testRawJSON(sdk.NetworkProxyResult{}),
				})
			default:
				_ = harness.enc.Encode(sdk.Frame{
					Kind:   sdk.KindHostResult,
					ID:     frame.ID,
					Parent: frame.Parent,
					Error:  &sdk.RPCError{Code: "method_not_found", Message: "unsupported in test"},
				})
			}
		case sdk.KindResult:
			return frame, data
		default:
			harness.test.Fatalf("unexpected frame kind: %+v", frame)
		}
	}
}

func sampleCredential(now time.Time) Credential {
	return Credential{
		Type:          "oauth",
		Access:        "jt-live-token",
		Refresh:       "rt-live-refresh",
		Expires:       now.Add(2 * time.Hour).UnixMilli(),
		AccountID:     "acct-1",
		UID:           "uid-1001",
		Email:         "user@example.com",
		CardName:      "user@example.com",
		MachineID:     "machine-fixed-01",
		DeviceToken:   "dt-live-token",
		DeviceRefresh: "dr-live-refresh",
	}
}

func sampleListingPayload() map[string]any {
	return map[string]any{
		"app": []map[string]any{
			{
				"key":                    "qoder-pro",
				"display_name":           "Qoder Pro",
				"enable":                 true,
				"is_vl":                  true,
				"is_reasoning":           true,
				"max_input_tokens":       200000,
				"max_output_tokens":      16384,
				"supported_effort_level": []string{"low", "medium", "high"},
				"default_effort_level":   "medium",
				"price_factor":           1.0,
				"model_config":           map[string]any{"provider": "qoder", "model": "qoder-pro-v1"},
			},
			{
				"key":              "qwen-free",
				"display_name":     "Qwen Free",
				"enable":           true,
				"is_vl":            false,
				"is_reasoning":     false,
				"max_input_tokens": 131072,
				"price_factor":     0.0,
				"model_config":     map[string]any{"provider": "qoder", "model": "qwen-free-v1"},
			},
			{
				"key":              "dfmodel",
				"display_name":     "Disabled Model",
				"enable":           false,
				"max_input_tokens": 131072,
				"price_factor":     2.0,
			},
		},
	}
}

func TestManifestContractAndProviders(test *testing.T) {
	manifest := Manifest()
	if manifest.ID != ModuleID || manifest.Contract.Version.Major != 1 || manifest.Contract.Version.Minor != 12 {
		test.Fatalf("unexpected manifest identity/contract: %+v", manifest)
	}
	if manifest.Compatibility == nil || manifest.Compatibility.NativeABI != 1 {
		test.Fatalf("unexpected compatibility: %+v", manifest.Compatibility)
	}
	if len(manifest.Providers) != 4 {
		test.Fatalf("expected 4 providers, got %d", len(manifest.Providers))
	}
	providerMap := map[string]sdk.ProviderDescriptor{}
	for _, desc := range manifest.Providers {
		providerMap[desc.ID] = desc
	}
	if providerMap[ProviderQoder].Checkin == nil || providerMap[ProviderQoderCN].Checkin == nil {
		test.Fatalf("expected checkin descriptor on qoder and qoder-cn")
	}
	if providerMap[ProviderQwenWork].Checkin != nil || providerMap[ProviderQwenWorkEnterprise].Checkin != nil {
		test.Fatalf("did not expect checkin descriptor on qwenwork providers")
	}

	presetMod := NewModule(ModuleOptions{
		Env: func(key string) string {
			if key == "QWENWORK_ENT_DOMAIN" {
				return "https://acme.vpc.qwenwork.cn/"
			}
			return ""
		},
	})
	if err := presetMod.Validate(); err != nil {
		test.Fatalf("preset enterprise module failed validate: %v", err)
	}
	foundPreset := false
	for _, binding := range presetMod.Manifest.AuthBindings {
		if binding.Provider == ProviderQwenWorkEnterprise && len(binding.Methods) == 1 &&
			strings.Contains(binding.Methods[0].Label, "acme.vpc.qwenwork.cn") &&
			len(binding.Methods[0].Fields) == 1 && binding.Methods[0].Fields[0].Optional {
			foundPreset = true
		}
	}
	if !foundPreset {
		test.Fatalf("expected preset enterprise domain binding in manifest")
	}
}

func TestCosyHeadersAndBodyCodec(test *testing.T) {
	samples := []string{
		"",
		"a",
		"ab",
		"hello world",
		`{"model":"qoder-pro","text":"你好世界 🚀 — test payload with 1234567890"}`,
	}
	for _, plain := range samples {
		wire := encodeBody(plain)
		decoded := decodeBody(wire)
		if decoded != plain {
			test.Fatalf("codec roundtrip mismatch for %q: got %q", plain, decoded)
		}
	}

	asciiHost := machineHostname("研发主机.local")
	if !isHostSafe(asciiHost) || !strings.Contains(asciiHost, "xn--") {
		test.Fatalf("expected punycode-safe hostname, got %q", asciiHost)
	}
	if capped := machineHostname(strings.Repeat("a", 120)); len(capped) > 96 {
		test.Fatalf("expected hostname capped to <=96 chars, got len=%d (%q)", len(capped), capped)
	}

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	owner := newOwner(ModuleOptions{
		Now:      func() time.Time { return now },
		Platform: "darwin",
		Arch:     "arm64",
		Hostname: func() string { return "dev-box" },
	})
	cred := sampleCredential(now)
	site := owner.sites[ProviderQoder]
	wire := encodeBody(`{"ping":true}`)
	hdr, err := owner.chatHeaders(site.API+chatPath, cred, wire, site, ModelEntry{Key: "qoder-pro"}, map[string]string{
		"Cosy-MachineToken": "mt-123",
		"Cosy-MachineType":  "3",
	})
	if err != nil {
		test.Fatalf("chatHeaders failed: %v", err)
	}
	for _, required := range []string{"Authorization", "Cosy-Key", "Cosy-User", "Cosy-Date", "X-Model-Key", "Cosy-MachineToken", "Cosy-MachineType"} {
		if hdr.Get(required) == "" {
			test.Fatalf("missing required header %s: %+v", required, hdr)
		}
	}
	if !strings.HasPrefix(hdr.Get("Authorization"), "Bearer COSY.") {
		test.Fatalf("unexpected Authorization header: %q", hdr.Get("Authorization"))
	}
	if hdr.Get("X-Model-Key") != "qoder-pro" || hdr.Get("Cosy-MachineToken") != "mt-123" {
		test.Fatalf("unexpected header values: model=%q token=%q", hdr.Get("X-Model-Key"), hdr.Get("Cosy-MachineToken"))
	}
}

func TestAuthFlowsDomainValidationAndLingmaCache(test *testing.T) {
	if got := enterpriseDomain("https://corp.vpc.qwenwork.cn/"); got != "corp.vpc.qwenwork.cn" {
		test.Fatalf("unexpected enterpriseDomain: %q", got)
	}
	for _, bad := range []string{"http://corp.example.com", "https://127.0.0.1", "localhost", "foo.local", "https://corp.example.com/path", "192.168.1.1"} {
		if got := enterpriseDomain(bad); got != "" {
			test.Fatalf("expected invalid enterpriseDomain for %q, got %q", bad, got)
		}
	}
	if got := maskPhone("+86 13800138000"); got != "138****8000" {
		test.Fatalf("unexpected masked phone: %q", got)
	}

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	var pollCalls atomic.Int32
	var refreshCalls atomic.Int32
	mod := NewModule(ModuleOptions{
		Now: func() time.Time { return now },
		HTTPFactory: testHTTPFactory(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/v1/deviceToken/poll":
				if pollCalls.Add(1) == 1 {
					return testJSONResp(http.StatusOK, map[string]any{"status": "pending"}, nil), nil
				}
				return testJSONResp(http.StatusOK, map[string]any{
					"token":         "dt-new",
					"refresh_token": "dr-new",
					"expires_in":    7200,
					"user_id":       "uid-42",
					"user_name":     "alice",
				}, nil), nil
			case "/api/v1/jobToken/exchange":
				return testJSONResp(http.StatusOK, map[string]any{
					"token":         "jt-exchanged",
					"refresh_token": "rt-exchanged",
					"expires_in":    3600 * 1000,
				}, nil), nil
			case "/api/v1/userinfo":
				return testJSONResp(http.StatusOK, map[string]any{
					"id":              "uid-42",
					"email":           "phone_1111@phone.local",
					"name":            "8000",
					"security_mobile": "13800138000",
				}, nil), nil
			case "/api/v1/jobToken/refresh":
				refreshCalls.Add(1)
				return testJSONResp(http.StatusOK, map[string]any{
					"token":         "jt-refreshed-2",
					"refresh_token": "rt-refreshed-2",
					"expires_in":    7200 * 1000,
				}, nil), nil
			default:
				return testJSONResp(http.StatusNotFound, map[string]any{"error": req.URL.Path}, nil), nil
			}
		}),
	})
	harness := newQoderHarness(test, mod)
	scope := sdk.Scope{Provider: ProviderQoderCN}

	startFrame, _ := harness.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "browser"}, nil)
	if startFrame.Error != nil {
		test.Fatalf("auth.start failed: %v", startFrame.Error)
	}
	var startRes sdk.AuthStartResult
	_ = json.Unmarshal(startFrame.Result, &startRes)
	if !strings.Contains(startRes.URL, "challenge=") || len(startRes.Continuation) == 0 {
		test.Fatalf("unexpected auth.start result: %+v", startRes)
	}

	fin1, _ := harness.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: startRes.Continuation}, nil)
	if fin1.Error != nil {
		test.Fatalf("first auth.finish failed: %v", fin1.Error)
	}
	var authRes1 sdk.AuthResult
	_ = json.Unmarshal(fin1.Result, &authRes1)
	if !authRes1.Pending {
		test.Fatalf("expected pending on first poll: %+v", authRes1)
	}

	fin2, _ := harness.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: startRes.Continuation}, nil)
	if fin2.Error != nil {
		test.Fatalf("second auth.finish failed: %v", fin2.Error)
	}
	var authRes2 sdk.AuthResult
	_ = json.Unmarshal(fin2.Result, &authRes2)
	if authRes2.Pending || len(authRes2.Credentials) == 0 {
		test.Fatalf("expected completed credential on second poll: %+v", authRes2)
	}
	var savedCred Credential
	_ = json.Unmarshal(authRes2.Credentials, &savedCred)
	if savedCred.Access != "dt-new" || !savedCred.DeviceChat || savedCred.PhoneLabel != "138****8000" {
		test.Fatalf("unexpected qoder-cn credential: %+v", savedCred)
	}

	// PAT sign-in + refresh de-duplication via spent map
	patStart, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderQoder}, sdk.AuthStartRequest{
		Method: "pat",
		Input:  testRawJSON(map[string]string{"pat": "pt-secret-token-1"}),
	}, nil)
	if patStart.Error != nil {
		test.Fatalf("pat auth.start failed: %v", patStart.Error)
	}
	var patStartRes sdk.AuthStartResult
	_ = json.Unmarshal(patStart.Result, &patStartRes)
	patFin, _ := harness.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderQoder}, sdk.AuthFinishRequest{Continuation: patStartRes.Continuation}, nil)
	if patFin.Error != nil {
		test.Fatalf("pat auth.finish failed: %v", patFin.Error)
	}
	var patAuthRes sdk.AuthResult
	_ = json.Unmarshal(patFin.Result, &patAuthRes)
	var patCred Credential
	_ = json.Unmarshal(patAuthRes.Credentials, &patCred)
	if !patCred.PATAuth || patCred.Access != "jt-exchanged" {
		test.Fatalf("unexpected PAT cred: %+v", patCred)
	}

	// Refresh twice with the same stale stored credential; second must follow spent without re-calling upstream.
	ref1, _ := harness.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderQoder}, sdk.AuthRefreshRequest{Credentials: patAuthRes.Credentials}, nil)
	ref2, _ := harness.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderQoder}, sdk.AuthRefreshRequest{Credentials: patAuthRes.Credentials}, nil)
	if ref1.Error != nil || ref2.Error != nil {
		test.Fatalf("auth.refresh failed: ref1=%v ref2=%v", ref1.Error, ref2.Error)
	}
	if refreshCalls.Load() != 1 {
		test.Fatalf("expected exactly 1 upstream jobToken refresh due to spent cache, got %d", refreshCalls.Load())
	}

	// Verify Lingma 1.0 AES-128-CBC cache file decode in an isolated temp dir.
	tmpRoot := test.TempDir()
	cacheDir := filepath.Join(tmpRoot, "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		test.Fatal(err)
	}
	machineID := "0123456789abcdef-lingma-host"
	keyBytes := []byte(machineID[:16])
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		test.Fatal(err)
	}
	lingmaJSON := fmt.Sprintf(`{"security_oauth_token":"lingma-tok-1","uid":"lingma-uid-1","name":"bob","email":"bob@corp.example","expire_time":%d}`, now.Add(3*time.Hour).UnixMilli())
	padded := pkcs7Pad([]byte(lingmaJSON), aes.BlockSize)
	cipherBytes := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, keyBytes).CryptBlocks(cipherBytes, padded)
	wire := base64.StdEncoding.EncodeToString(cipherBytes)
	if err := os.WriteFile(filepath.Join(cacheDir, "id"), []byte(machineID), 0o600); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "user"), []byte(wire), 0o600); err != nil {
		test.Fatal(err)
	}
	decoded := lingmaDecode(tmpRoot)
	if decoded.Access != "lingma-tok-1" || decoded.UID != "lingma-uid-1" {
		test.Fatalf("unexpected decoded lingma cache: %+v", decoded)
	}
}

func TestModelsFreeViewGuardAndQwenWorkFallback(test *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	var listCalls atomic.Int32
	var refreshCalls atomic.Int32
	mod := NewModule(ModuleOptions{
		Now: func() time.Time { return now },
		HTTPFactory: testHTTPFactory(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/algo/api/v2/model/list":
				if req.URL.Host == "ent.vpc.qwenwork.cn" {
					return testJSONResp(http.StatusServiceUnavailable, map[string]any{"error": "no catalog"}, nil), nil
				}
				callNum := listCalls.Add(1)
				if callNum == 1 {
					return testJSONResp(http.StatusOK, map[string]any{
						"app": []map[string]any{
							{"key": "qwen-free", "display_name": "Qwen Free", "enable": true, "price_factor": 0.0, "max_input_tokens": 131072},
							{"key": "qoder-pro", "display_name": "Qoder Pro", "enable": false, "price_factor": 1.0, "max_input_tokens": 200000},
						},
					}, nil), nil
				}
				return testJSONResp(http.StatusOK, sampleListingPayload(), nil), nil
			case "/api/v1/jobToken/refresh":
				refreshCalls.Add(1)
				return testJSONResp(http.StatusOK, map[string]any{
					"token":         "jt-upgraded",
					"refresh_token": "rt-upgraded",
					"expires_in":    7200,
				}, nil), nil
			default:
				return testJSONResp(http.StatusNotFound, map[string]any{}, nil), nil
			}
		}),
	})
	harness := newQoderHarness(test, mod)
	cred := sampleCredential(now)

	// Free-only listing triggers one-shot relogin and retrieves full model list.
	frame, _ := harness.invoke(sdk.MethodProviderModels, sdk.Scope{Provider: ProviderQoder}, sdk.ModelsRequest{}, &cred)
	if frame.Error != nil {
		test.Fatalf("models failed: %v", frame.Error)
	}
	var res sdk.ModelsResult
	_ = json.Unmarshal(frame.Result, &res)
	if refreshCalls.Load() != 1 || len(res.Models) != 2 {
		test.Fatalf("expected 1 relogin and 2 enabled models, got refreshes=%d models=%+v", refreshCalls.Load(), res.Models)
	}

	// QwenWork enterprise falls back cleanly when model list returns 503.
	entCred := sampleCredential(now)
	entCred.VPC = "ent.vpc.qwenwork.cn"
	entCred.EnterpriseHost = "ent.vpc.qwenwork.cn"
	entFrame, _ := harness.invoke(sdk.MethodProviderModels, sdk.Scope{Provider: ProviderQwenWorkEnterprise}, sdk.ModelsRequest{}, &entCred)
	if entFrame.Error != nil {
		test.Fatalf("enterprise models failed: %v", entFrame.Error)
	}
	var entRes sdk.ModelsResult
	_ = json.Unmarshal(entFrame.Result, &entRes)
	if len(entRes.Models) != 3 {
		test.Fatalf("expected 3 fallback enterprise models, got %+v", entRes.Models)
	}
}

func TestRequestRewritesUnionsAndReclamp(test *testing.T) {
	if got := asAgentSDK("You are Claude Code, Anthropic's official CLI for Claude."); !strings.Contains(got, "Claude Agent SDK") {
		test.Fatalf("asAgentSDK did not rewrite Claude Code intro: %q", got)
	}
	if got := harnessComma("You are an interactive coding agent powered by DeepSeek"); !strings.Contains(got, "coding agent, powered by") {
		test.Fatalf("harnessComma did not rewrite comma: %q", got)
	}

	rawUnion := map[string]any{
		"$defs": map[string]any{
			"Item": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}},
		},
		"oneOf": []any{
			map[string]any{"$ref": "#/$defs/Item"},
			map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "integer"}}},
		},
	}
	normalized, ok := typedUnions(rawUnion).(map[string]any)
	if !ok || normalized["type"] != "object" {
		test.Fatalf("typedUnions did not resolve untyped oneOf + $ref: %#v", normalized)
	}

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	owner := newOwner(ModuleOptions{Now: func() time.Time { return now }})
	state := owner.states[ProviderQoder]
	kimiEntry := ModelEntry{
		Key:     "kmodel",
		Name:    "Kimi K2.5",
		Context: 131072,
		Images:  true,
		Thinks:  true,
		Efforts: []string{"low", "high"},
		Config:  map[string]any{"provider": "qoder", "model": "kimi-k2.5"},
	}
	chat := map[string]any{
		"model":            "kmodel",
		"reasoning_effort": "high",
		"max_tokens":       120000,
		"messages": []any{
			map[string]any{"role": "system", "content": "You are Claude Code, Anthropic's official CLI for Claude."},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "describe this"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}},
				},
			},
			map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []any{
					map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "lookup", "arguments": `{"q":"x"}`}},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "ok"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":       "lookup",
					"parameters": rawUnion,
				},
			},
		},
	}
	task := state.taskOf(sessionOf(chat, "sess-1"), chat, now.UnixMilli())
	body, err := owner.qoderBody(state, chat, kimiEntry, "sess-1", task, owner.sites[ProviderQoder])
	if err != nil {
		test.Fatalf("qoderBody failed: %v", err)
	}
	msgs, _ := body["messages"].([]map[string]any)
	foundSpaceContent := false
	for _, msg := range msgs {
		if msg["role"] == "assistant" && msg["content"] == " " {
			foundSpaceContent = true
		}
	}
	if !foundSpaceContent {
		test.Fatalf("expected kimiModel assistant tool call content to be a single space: %#v", msgs)
	}
	fail := classifyFailure(http.StatusBadRequest, `Range of max_tokens should be [1, 32768]`)
	if fail == nil || fail.MaxTokens != 32768 {
		test.Fatalf("expected max_tokens failure to parse 32768, got %+v", fail)
	}
	if !state.reclamp(fail, body, kimiEntry) {
		test.Fatalf("expected reclamp to clamp body max_tokens")
	}
	params := body["parameters"].(map[string]any)
	if params["max_tokens"] != 32768 || state.outputCap(kimiEntry) != 32768 {
		test.Fatalf("unexpected reclamped max_tokens/cap: %v / %d", params["max_tokens"], state.outputCap(kimiEntry))
	}
}

func TestSplitterAndSSEEvents(test *testing.T) {
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name": "read_file",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":  map[string]any{"type": "string"},
						"limit": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}

	// 1. XML tool call inside <tool_call> with typed parameter conversion + quoted fence ignored.
	sp := newSplitter(tools)
	pieces := sp.Feed("Look:\n```\n<tool_call>{\"name\":\"read_file\"}</tool_call>\n```\nNow call:\n<tool_call><function=read_file><parameter=path>/tmp/a</parameter><parameter=limit>42</parameter></function></tool_call>")
	pieces = append(pieces, sp.Flush()...)
	var foundCall *parsedToolCall
	for _, piece := range pieces {
		if piece.Call != nil {
			foundCall = piece.Call
		}
	}
	if foundCall == nil || foundCall.Name != "read_file" || !strings.Contains(foundCall.Args, `"limit":42`) {
		test.Fatalf("expected typed XML tool call, got %+v", pieces)
	}

	// 2. Bare JSON {"tool_calls":[...]} (0.2.68)
	spJSON := newSplitter(tools)
	jPieces := append(spJSON.Feed(`{"tool_calls":[{"name":"read_file","arguments":{"path":"x.go","limit":5}}]}`), spJSON.Flush()...)
	if len(jPieces) != 1 || jPieces[0].Call == nil || jPieces[0].Call.Name != "read_file" {
		test.Fatalf("expected bare JSON tool call, got %+v", jPieces)
	}

	// 3. Dual-ID same-index native tool calls (0.2.65)
	sseBody := qoderSSELines(http.StatusOK,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_b","function":{"name":"read_file","arguments":"{\"path\":\"b\"}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":8}}`,
		`[DONE]`,
	)
	reader := newSSEEventReader(strings.NewReader(sseBody), tools)
	var toolIDs []string
	var toolIndices []int
	for {
		ev, ok, err := reader.Next()
		if err != nil {
			test.Fatalf("reader.Next failed: %v", err)
		}
		if !ok {
			break
		}
		if ev.Tool != nil {
			toolIDs = append(toolIDs, ev.Tool.ID)
			toolIndices = append(toolIndices, ev.Tool.Index)
		}
	}
	if !slices.Equal(toolIDs, []string{"call_a", "call_b"}) || !slices.Equal(toolIndices, []int{0, 1}) {
		test.Fatalf("expected dual-ID same-index calls to split onto indices 0 and 1: ids=%v idx=%v", toolIDs, toolIndices)
	}

	// 4. Nameless native tool call sets Nameless=true (#579)
	namelessSSE := qoderSSELines(http.StatusOK,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_empty","function":{"arguments":"{\"x\":1}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	)
	readerNameless := newSSEEventReader(strings.NewReader(namelessSSE), tools)
	ev, ok, err := readerNameless.Next()
	if err != nil || !ok || ev.Error == nil || !ev.Error.Nameless {
		test.Fatalf("expected nameless tool call failure, got ev=%+v ok=%v err=%v", ev, ok, err)
	}
}

func TestExecuteQueueKeepaliveFallbackStuckSilentAndCreditsSpent(test *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	var curTimeMu sync.Mutex
	curTime := now
	getNow := func() time.Time {
		curTimeMu.Lock()
		defer curTimeMu.Unlock()
		return curTime
	}
	advance := func(delta time.Duration) {
		curTimeMu.Lock()
		curTime = curTime.Add(delta)
		curTimeMu.Unlock()
	}

	var mode atomic.Int32
	var chatAttempt atomic.Int32
	var queuePolls atomic.Int32
	mod := NewModule(ModuleOptions{
		Now: getNow,
		Queue: &QueueTimings{
			Budget:   2 * time.Second,
			Held:     5 * time.Second,
			Beat:     15 * time.Millisecond,
			Shortest: 5 * time.Millisecond,
			Longest:  25 * time.Millisecond,
			Unsaid:   10 * time.Millisecond,
			Ask:      200 * time.Millisecond,
			Silent:   40 * time.Millisecond,
			Stuck:    30 * time.Millisecond,
		},
		HTTPFactory: testHTTPFactory(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/algo/api/v2/model/list":
				return testJSONResp(http.StatusOK, sampleListingPayload(), nil), nil
			case usagePath:
				return testJSONResp(http.StatusOK, map[string]any{
					"displayMode": "qoder",
					"qoderUsage": map[string]any{
						"userType":  "personal_professional_trial",
						"userQuota": map[string]any{"total": 800, "used": 800},
					},
				}, nil), nil
			case queueStatusPath:
				poll := queuePolls.Add(1)
				if mode.Load() == 2 {
					advance(50 * time.Millisecond)
					return testJSONResp(http.StatusOK, map[string]any{
						"data": map[string]any{
							"isQueued":          true,
							"serviceAvailable":  false,
							"queueCount":        1,
							"queueType":         "p4",
							"subCode":           "RESOURCE_LIMIT",
							"retryAfterSeconds": 0.005,
						},
					}, nil), nil
				}
				if poll == 1 {
					return testJSONResp(http.StatusOK, map[string]any{
						"data": map[string]any{
							"isQueued":          true,
							"serviceAvailable":  false,
							"queueCount":        3,
							"retryAfterSeconds": 0.025,
						},
					}, nil), nil
				}
				return testJSONResp(http.StatusOK, map[string]any{
					"data": map[string]any{"isQueued": false, "serviceAvailable": true},
				}, nil), nil
			case "/algo/api/v2/service/pro/sse/agent_chat_generation":
				switch mode.Load() {
				case 1:
					// Streaming queued then admitted
					if chatAttempt.Add(1) == 1 {
						return testSSEResp(http.StatusOK, qoderSSELines(http.StatusOK, `{"code":"10605","message":"queued","data":{"isQueued":true,"queueType":"p3","queueCount":2,"retryAfterSeconds":0.01}}`)), nil
					}
					return testSSEResp(http.StatusOK, qoderSSELines(http.StatusOK,
						`{"choices":[{"delta":{"content":"hello after queue"}}]}`,
						`{"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
						`[DONE]`,
					)), nil
				case 2:
					// Stuck empty queue (queueCount=1 unchanged)
					return testSSEResp(http.StatusOK, qoderSSELines(http.StatusOK, `{"code":"10605","message":"busy","data":{"isQueued":true,"serviceAvailable":false,"queueCount":1,"queueType":"p4","subCode":"RESOURCE_LIMIT","retryAfterSeconds":0.005}}`)), nil
				case 3:
					// Silent upstream exceeding Queue.Silent -> 504
					<-req.Context().Done()
					return nil, req.Context().Err()
				default:
					return testSSEResp(http.StatusOK, qoderSSELines(http.StatusOK,
						`{"choices":[{"delta":{"content":"ok"}}]}`,
						`{"choices":[{"finish_reason":"stop"}]}`,
						`[DONE]`,
					)), nil
				}
			default:
				return testJSONResp(http.StatusNotFound, map[string]any{}, nil), nil
			}
		}),
	})
	harness := newQoderHarness(test, mod)
	cred := sampleCredential(now)
	scope := sdk.Scope{Provider: ProviderQoder}

	// 1. Streaming queue wait emits initial comment + keepalive + final answer + [DONE]
	mode.Store(1)
	frame1, body1 := harness.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "qoder-pro",
		Body:     testRawJSON(map[string]any{"model": "qoder-pro", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}),
	}, &cred)
	if frame1.Error != nil {
		test.Fatalf("queued stream failed: %v", frame1.Error)
	}
	var exec1 sdk.ExecuteResult
	_ = json.Unmarshal(frame1.Result, &exec1)
	if exec1.Status != http.StatusOK || exec1.Usage == nil || exec1.Usage.InputTokens != 10 {
		test.Fatalf("unexpected queued stream result: %+v", exec1)
	}
	body1Text := string(body1)
	if !strings.Contains(body1Text, ": ") || !strings.Contains(body1Text, "hello after queue") || !strings.Contains(body1Text, "data: [DONE]") {
		test.Fatalf("unexpected queued SSE body: %s", body1Text)
	}

	// 2. X-Magpie-Fallback: 1 returns 503 immediately without polling queueStatus
	chatAttempt.Store(0)
	queuePolls.Store(0)
	frameFB, _ := harness.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "qoder-pro",
		Headers:  http.Header{"X-Magpie-Fallback": {"1"}},
		Body:     testRawJSON(map[string]any{"model": "qoder-pro", "stream": false, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}),
	}, &cred)
	var execFB sdk.ExecuteResult
	_ = json.Unmarshal(frameFB.Result, &execFB)
	if execFB.Status != http.StatusServiceUnavailable || queuePolls.Load() != 0 {
		test.Fatalf("expected immediate 503 with 0 queue polls on fallback, got status=%d polls=%d", execFB.Status, queuePolls.Load())
	}

	// 3. Stuck queue (0.2.71) returns 503 with stuck explanation
	mode.Store(2)
	frameStuck, bodyStuck := harness.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "qoder-pro",
		Body:     testRawJSON(map[string]any{"model": "qoder-pro", "stream": false, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}),
	}, &cred)
	var execStuck sdk.ExecuteResult
	_ = json.Unmarshal(frameStuck.Result, &execStuck)
	if execStuck.Status != http.StatusServiceUnavailable || !strings.Contains(string(bodyStuck), "queue hasn't moved") {
		test.Fatalf("expected stuck 503, got status=%d body=%s", execStuck.Status, string(bodyStuck))
	}

	// 4. Silent upstream timeout (0.2.70) returns 504
	mode.Store(3)
	frameSilent, bodySilent := harness.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "qoder-pro",
		Body:     testRawJSON(map[string]any{"model": "qoder-pro", "stream": false, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}),
	}, &cred)
	var execSilent sdk.ExecuteResult
	_ = json.Unmarshal(frameSilent.Result, &execSilent)
	if execSilent.Status != http.StatusGatewayTimeout || !strings.Contains(string(bodySilent), "sent nothing") {
		test.Fatalf("expected silent 504, got status=%d body=%s", execSilent.Status, string(bodySilent))
	}

	// 5. Disabled model with spent credits (0.2.64) returns 429 usage limit reached
	frameOff, bodyOff := harness.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "dfmodel",
		Body:     testRawJSON(map[string]any{"model": "dfmodel", "stream": false, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}),
	}, &cred)
	var execOff sdk.ExecuteResult
	_ = json.Unmarshal(frameOff.Result, &execOff)
	if execOff.Status != http.StatusTooManyRequests || !strings.Contains(string(bodyOff), "800 / 800 credits") {
		test.Fatalf("expected 429 credits spent, got status=%d body=%s", execOff.Status, string(bodyOff))
	}
}

func TestUsageSpareFreeRotationAndQwenWorkWallets(test *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	expiresISO := "2026-12-01T00:00:00Z"
	var usageCalls atomic.Int32
	var rotated atomic.Int32
	mod := NewModule(ModuleOptions{
		Now: func() time.Time { return now },
		HTTPFactory: testHTTPFactory(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/algo/api/v2/model/list":
				return testJSONResp(http.StatusOK, sampleListingPayload(), nil), nil
			case usagePath:
				if usageCalls.Add(1) == 1 {
					return testJSONResp(http.StatusUnauthorized, map[string]any{"error": "expired"}, nil), nil
				}
				return testJSONResp(http.StatusOK, map[string]any{
					"displayMode": "qoder",
					"qoderUsage": map[string]any{
						"userType":   "pro",
						"expiresAt":  expiresISO,
						"userQuota":  map[string]any{"total": 2000, "used": 500},
						"addOnQuota": map[string]any{"total": 1000, "used": 250},
					},
				}, nil), nil
			case "/api/v1/deviceToken/refresh":
				rotated.Add(1)
				return testJSONResp(http.StatusOK, map[string]any{
					"token":         "dt-rotated",
					"refresh_token": "dr-rotated",
					"expires_in":    7200,
				}, nil), nil
			case "/api/v1/adapter/user/account-context":
				return testJSONResp(http.StatusOK, map[string]any{
					"data": map[string]any{
						"user":  map[string]any{"is_biz": false},
						"plan":  map[string]any{"name": "Personal"},
						"quota": map[string]any{"remaining": 150},
					},
				}, nil), nil
			case "/api/v1/adapter/user/wallets":
				return testJSONResp(http.StatusOK, map[string]any{
					"data": map[string]any{
						"daily_credits":   map[string]any{"total_balance": 50},
						"monthly_credits": map[string]any{"total_balance": 100},
					},
				}, nil), nil
			default:
				return testJSONResp(http.StatusNotFound, map[string]any{}, nil), nil
			}
		}),
	})
	harness := newQoderHarness(test, mod)
	cred := sampleCredential(now)

	// Qoder usage: 401 rotates device token once, synthesizes Total credits with Aside on pools, spares qwen-free via NotModels.
	frame, _ := harness.invoke(sdk.MethodProviderUsage, sdk.Scope{Provider: ProviderQoder}, sdk.UsageRequest{Revision: "17"}, &cred)
	if frame.Error != nil {
		test.Fatalf("usage failed: %v", frame.Error)
	}
	var uRes sdk.UsageResult
	_ = json.Unmarshal(frame.Result, &uRes)
	if uRes.Revision != "17" {
		test.Fatalf("Qoder did not acknowledge usage revision 17: %q", uRes.Revision)
	}
	if rotated.Load() != 1 || len(uRes.Windows) != 3 {
		test.Fatalf("expected 1 device rotation and 3 windows (Total + 2 aside), got rot=%d windows=%+v", rotated.Load(), uRes.Windows)
	}
	if uRes.Windows[0].Name != "Total credits" || uRes.Windows[0].Display != "750 / 3000 credits" || uRes.Windows[0].ResetsAt == 0 {
		test.Fatalf("unexpected Total credits window: %+v", uRes.Windows[0])
	}
	if !uRes.Windows[1].Aside || !uRes.Windows[2].Aside {
		test.Fatalf("expected constituent pools marked Aside: %+v", uRes.Windows)
	}
	if !slices.Contains(uRes.Windows[0].NotModels, "qwen-free") {
		test.Fatalf("expected free model spared via NotModels: %+v", uRes.Windows[0].NotModels)
	}

	// Zero-pool free account parses as 0 credits left (100% used).
	zeroParsed := parseUsage(map[string]any{
		"displayMode": "qoder",
		"qoderUsage": map[string]any{
			"userType":  "personal_standard",
			"userQuota": map[string]any{"total": 0, "used": 0},
		},
	})
	if len(zeroParsed.Windows) != 1 || zeroParsed.Windows[0].Used != 100 || zeroParsed.Windows[0].Display != "0 credits left" {
		test.Fatalf("unexpected zero-pool usage: %+v", zeroParsed)
	}

	// QwenWork personal account context + wallets breakdown.
	qwFrame, _ := harness.invoke(sdk.MethodProviderUsage, sdk.Scope{Provider: ProviderQwenWork}, sdk.UsageRequest{Revision: "18"}, &cred)
	if qwFrame.Error != nil {
		test.Fatalf("qwenwork usage failed: %v", qwFrame.Error)
	}
	var qwRes sdk.UsageResult
	_ = json.Unmarshal(qwFrame.Result, &qwRes)
	if qwRes.Revision != "18" {
		test.Fatalf("Qwen Work did not acknowledge usage revision 18: %q", qwRes.Revision)
	}
	if len(qwRes.Windows) != 1 || !strings.Contains(qwRes.Windows[0].Display, "150 credits (daily 50 · monthly 100)") {
		test.Fatalf("unexpected qwenwork personal balance windows: %+v", qwRes.Windows)
	}
}

func TestCheckinOutcomesAndRuntimeDiscovery(test *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	tmpHome := test.TempDir()
	cliBinDir := filepath.Join(tmpHome, ".qoder", ".bin")
	if err := os.MkdirAll(cliBinDir, 0o700); err != nil {
		test.Fatal(err)
	}
	olderBin := filepath.Join(cliBinDir, "runtime-info-linux-x64-old")
	newerBin := filepath.Join(cliBinDir, "runtime-info-linux-x64-new")
	if err := os.WriteFile(olderBin, []byte("old"), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(newerBin, []byte("new"), 0o700); err != nil {
		test.Fatal(err)
	}
	_ = os.Chtimes(olderBin, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	_ = os.Chtimes(newerBin, now, now)

	ownerCheck := newOwner(ModuleOptions{
		Platform: "linux",
		Arch:     "x64",
		HomeDir:  func() string { return tmpHome },
		OptDir:   filepath.Join(tmpHome, "opt"),
		Env:      func(string) string { return "" },
	})
	discovered, err := ownerCheck.campaignRuntime(runtimeSpecs[ProviderQoder])
	if err != nil || discovered != newerBin {
		test.Fatalf("expected newest CLI runtime-info %q, got %q (err=%v)", newerBin, discovered, err)
	}

	// Check-in outcomes across claimable, already claimed, missing runtime, and Lingma.
	var scenario atomic.Int32
	var gotMachineHeader atomic.Bool
	var runtimeRuns atomic.Int32
	startAt := now.Add(-2 * time.Hour).UnixMilli()
	endAt := now.Add(22 * time.Hour).UnixMilli()

	mod := NewModule(ModuleOptions{
		Now:      func() time.Time { return now },
		Platform: "darwin",
		Arch:     "arm64",
		HomeDir:  func() string { return tmpHome },
		Env: func(key string) string {
			if key == "QODER_RUNTIME_INFO" && scenario.Load() != 3 {
				return newerBin
			}
			return ""
		},
		RunRuntimeInfo: func(ctx context.Context, binaryPath string, args []string, stdinData []byte) ([]byte, error) {
			runtimeRuns.Add(1)
			if scenario.Load() == 3 {
				return nil, errors.New("not installed")
			}
			if len(args) != 2 || args[0] != "3" || args[1] != "--account-stdin" || !strings.Contains(string(stdinData), "uid-1001") {
				return nil, fmt.Errorf("unexpected runtime-info invocation: args=%v stdin=%q", args, string(stdinData))
			}
			return []byte(`{"machineToken":"signed-mt-99","machineType":"3"}` + "\n"), nil
		},
		HTTPFactory: testHTTPFactory(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Cosy-MachineToken") == "signed-mt-99" {
				gotMachineHeader.Store(true)
			}
			switch {
			case req.Method == http.MethodGet && req.URL.Path == campaignsPath:
				switch scenario.Load() {
				case 1:
					return testJSONResp(http.StatusOK, map[string]any{
						"campaigns": []map[string]any{
							{
								"campaignId":  "camp-1",
								"actionType":  "CLAIM_BENEFIT",
								"claimStatus": "CLAIMABLE",
								"startAt":     startAt,
								"endAt":       endAt,
								"benefit":     map[string]any{"amount": 50},
							},
						},
					}, nil), nil
				case 2:
					return testJSONResp(http.StatusOK, map[string]any{
						"campaigns": []map[string]any{
							{
								"campaignId":  "camp-1",
								"actionType":  "CLAIM_BENEFIT",
								"claimStatus": "CLAIMED",
								"startAt":     startAt,
								"endAt":       endAt,
								"benefit":     map[string]any{"amount": 50},
							},
						},
					}, nil), nil
				default:
					return testJSONResp(http.StatusOK, map[string]any{"campaigns": []any{}}, nil), nil
				}
			case req.Method == http.MethodPost && req.URL.Path == "/sash/api/v1/me/campaigns/camp-1/claim":
				return testJSONResp(http.StatusOK, map[string]any{
					"status":  "CLAIMED",
					"benefit": map[string]any{"amount": 50},
				}, nil), nil
			default:
				return testJSONResp(http.StatusNotFound, map[string]any{}, nil), nil
			}
		}),
	})
	harness := newQoderHarness(test, mod)
	cred := sampleCredential(now)

	invokeCheckin := func(provider string, acct *Credential) sdk.Frame {
		fr, _ := harness.invoke(sdk.MethodModuleCall, sdk.Scope{
			Provider:   provider,
			Account:    acct.AccountID,
			Generation: 1,
		}, sdk.ModuleCallRequest{
			Action: "account.checkin",
			Input:  testRawJSON(sdk.AccountCheckinRequest{Day: "2026-11-24"}),
		}, acct)
		return fr
	}

	// 1. Claimable -> claimed with credit=50 and Until set
	scenario.Store(1)
	frameClaim := invokeCheckin(ProviderQoder, &cred)
	if frameClaim.Error != nil {
		test.Fatalf("checkin claim failed: %v", frameClaim.Error)
	}
	var resClaim sdk.AccountCheckinResult
	_ = json.Unmarshal(frameClaim.Result, &resClaim)
	if resClaim.Outcome != "claimed" || resClaim.Credit == nil || *resClaim.Credit != 50 || resClaim.Until == "" || !gotMachineHeader.Load() {
		test.Fatalf("unexpected claimed result: %+v gotHeader=%v", resClaim, gotMachineHeader.Load())
	}

	// 2. Already claimed -> done (and reuses cached runtime identity without running binary a second time)
	scenario.Store(2)
	frameDone := invokeCheckin(ProviderQoder, &cred)
	var resDone sdk.AccountCheckinResult
	_ = json.Unmarshal(frameDone.Result, &resDone)
	if resDone.Outcome != "done" || resDone.Credit == nil || *resDone.Credit != 50 || runtimeRuns.Load() != 1 {
		test.Fatalf("unexpected done result or cache miss: %+v runs=%d", resDone, runtimeRuns.Load())
	}

	// 3. Missing runtime on international Qoder -> ineligible
	scenario.Store(3)
	credOther := sampleCredential(now)
	credOther.UID = "uid-missing"
	frameMiss := invokeCheckin(ProviderQoder, &credOther)
	var resMiss sdk.AccountCheckinResult
	_ = json.Unmarshal(frameMiss.Result, &resMiss)
	if resMiss.Outcome != "ineligible" || !strings.Contains(resMiss.Message, "QODER_RUNTIME_INFO") {
		test.Fatalf("expected ineligible when runtime missing: %+v", resMiss)
	}

	// 4. Lingma dedicated domain -> inactive
	lingmaCred := sampleCredential(now)
	lingmaCred.Lingma = true
	frameLingma := invokeCheckin(ProviderQoderCN, &lingmaCred)
	var resLingma sdk.AccountCheckinResult
	_ = json.Unmarshal(frameLingma.Result, &resLingma)
	if resLingma.Outcome != "inactive" {
		test.Fatalf("expected inactive for Lingma check-in, got %+v", resLingma)
	}
}
