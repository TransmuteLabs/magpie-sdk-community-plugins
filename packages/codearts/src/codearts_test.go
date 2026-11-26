package codearts

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
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

func makeResponse(status int, body, contentType string, extra http.Header) *http.Response {
	hdr := http.Header{"Content-Type": {contentType}}
	for k, v := range extra {
		hdr[k] = append([]string(nil), v...)
	}
	return &http.Response{
		StatusCode: status,
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func makeJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".sig"
}

func sampleSTSCred(t *testing.T, station string) CodeArtsCredential {
	t.Helper()
	jwk, err := GenerateDPoPKey()
	if err != nil {
		t.Fatalf("GenerateDPoPKey: %v", err)
	}
	stsTok := makeJWT(map[string]any{
		"user_profile": map[string]any{
			"user_id":   "hw-user-42",
			"user_name": "alice",
			"domain_id": "dom-42",
		},
	})
	return CodeArtsCredential{
		AccessKeyID:       "AKIAEXAMPLE123",
		SecretAccessKey:   "SECRETEXAMPLE456",
		SecurityToken:     stsTok,
		RefreshToken:      "refresh-tok-1",
		CodeVerifier:      "verifier-42",
		SessionID:         "sess-42",
		DPoPPrivateKeyJWK: &jwk,
		Station:           station,
		DomainID:          "dom-42",
		UserID:            "hw-user-42",
		UserName:          "alice",
		ExpiresAt:         time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
}

func sampleOACred(t *testing.T) CodeArtsCredential {
	t.Helper()
	cred := sampleSTSCred(t, "mainland")
	cred.ProjectID = "proj-sw-2"
	cred.ModelAppKey = "oa-app-key-1"
	cred.ModelAppSecret = "oa-app-secret-1"
	cred.MaaSBaseURL = "https://maas.test"
	return cred
}

type storedEntry struct {
	version int
	body    json.RawMessage
}

type testHarness struct {
	t        *testing.T
	conn     net.Conn
	enc      *json.Encoder
	dec      *json.Decoder
	seq      int
	mu       sync.Mutex
	state    map[string]storedEntry
	replaced []json.RawMessage
	events   []sdk.Frame
}

func rawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw
	}
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func newTestHarness(t *testing.T, mod sdk.Module) *testHarness {
	t.Helper()
	if err := mod.Validate(); err != nil {
		t.Fatalf("module validation failed: %v", err)
	}
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- sdk.Serve(ctx, worker, worker, mod) }()
	fixture := &testHarness{
		t:     t,
		conn:  host,
		enc:   json.NewEncoder(host),
		dec:   json.NewDecoder(host),
		state: map[string]storedEntry{},
	}
	t.Cleanup(func() {
		cancel()
		host.Close()
		worker.Close()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("CodeArts SDK fixture did not stop")
		}
	})
	frame, _ := fixture.invoke(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent,
		Instance:  "codearts-test",
		Config:    sdk.ConfigSnapshot{Version: "1", Values: json.RawMessage(`{}`)},
	}, nil)
	if frame.Error != nil {
		t.Fatalf("configure failed: %v", frame.Error)
	}
	return fixture
}

func (fixture *testHarness) invoke(method string, scope sdk.Scope, params any, cred *CodeArtsCredential) (sdk.Frame, []byte) {
	fixture.t.Helper()
	fixture.seq++
	id := fmt.Sprintf("call-%d", fixture.seq)
	scope.Plugin = ModuleID
	scope.Operation = id
	req := sdk.Frame{Kind: "call", ID: id, Method: method, Scope: scope, Params: rawJSON(params)}
	if cred != nil {
		req.Auth = rawJSON(cred)
	}
	fixture.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := fixture.enc.Encode(req); err != nil {
		fixture.t.Fatal(err)
	}
	var data []byte
	for {
		var frame sdk.Frame
		if err := fixture.dec.Decode(&frame); err != nil {
			fixture.t.Fatalf("decode during %s: %v", method, err)
		}
		switch frame.Kind {
		case sdk.KindEvent:
			fixture.mu.Lock()
			fixture.events = append(fixture.events, frame)
			fixture.mu.Unlock()
			if frame.Event == "data" {
				var chunk []byte
				if json.Unmarshal(frame.Data, &chunk) == nil {
					data = append(data, chunk...)
				}
			}
			if err := fixture.enc.Encode(sdk.Frame{Kind: sdk.KindAck, ID: id, Seq: frame.Seq}); err != nil {
				fixture.t.Fatal(err)
			}
		case sdk.KindHostCall:
			resFrame := sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: id}
			switch frame.Method {
			case sdk.MethodStateGet:
				var getReq sdk.StateGetRequest
				_ = json.Unmarshal(frame.Params, &getReq)
				fixture.mu.Lock()
				ent, ok := fixture.state[getReq.Key]
				fixture.mu.Unlock()
				if ok {
					resFrame.Result = rawJSON(sdk.StateValue{Found: true, Version: fmt.Sprint(ent.version), Value: ent.body})
				} else {
					resFrame.Result = rawJSON(sdk.StateValue{Found: false, Version: "0"})
				}
			case sdk.MethodStateCAS:
				var casReq sdk.StateCASRequest
				_ = json.Unmarshal(frame.Params, &casReq)
				fixture.mu.Lock()
				ent := fixture.state[casReq.Key]
				expected := fmt.Sprint(ent.version)
				if casReq.Version == "" || casReq.Version == expected {
					ent.version++
					ent.body = append(json.RawMessage(nil), casReq.Value...)
					fixture.state[casReq.Key] = ent
					resFrame.Result = rawJSON(sdk.StateCASResult{Swapped: true, Version: fmt.Sprint(ent.version)})
				} else {
					resFrame.Result = rawJSON(sdk.StateCASResult{Swapped: false, Version: expected})
				}
				fixture.mu.Unlock()
			case sdk.MethodAccountReplace:
				var repReq sdk.AccountReplaceRequest
				_ = json.Unmarshal(frame.Params, &repReq)
				fixture.mu.Lock()
				fixture.replaced = append(fixture.replaced, append(json.RawMessage(nil), repReq.Credentials...))
				fixture.mu.Unlock()
				resFrame.Result = rawJSON(sdk.AccountReplaceResult{Generation: 2})
			case sdk.MethodNetworkProxy:
				resFrame.Result = rawJSON(sdk.NetworkProxyResult{})
			default:
				resFrame.Error = &sdk.Error{Code: "unsupported", Message: "unexpected host call " + frame.Method}
			}
			if err := fixture.enc.Encode(resFrame); err != nil {
				fixture.t.Fatal(err)
			}
		case sdk.KindResult:
			return frame, data
		default:
			fixture.t.Fatalf("unexpected frame kind %q", frame.Kind)
		}
	}
}

func testEndpoints() Endpoints {
	return Endpoints{
		Stations: map[string]Station{
			"mainland": {
				ID:      "mainland",
				Snap:    "https://snap.test",
				OpenGW:  "https://opengw.test",
				Console: "https://console.test",
				Portal:  "https://portal.test",
				Locale:  "zh-cn",
			},
			"international": {
				ID:      "international",
				Snap:    "https://snap-intl.test",
				OpenGW:  "",
				Console: "https://console-intl.test",
				Portal:  "https://portal-intl.test",
				Locale:  "en-us",
				Login:   "https://login-intl.test",
			},
		},
		STSToken:           "https://sts.test/v1/oauth2/tokens",
		IAMHost:            "https://iam.test",
		OAClawBase:         "https://claw.test",
		OASubscriptionBase: "https://oa-sub.test",
		OAMaaSBase:         "https://maas.test",
	}
}

func TestSignersAndDPoP(t *testing.T) {
	stamp := "20261124T080000Z"
	body := []byte(`{"model":"deepseek-v4-flash-0731","messages":[]}`)
	signed, err := SignRequestHuawei("AK1", "SK1", "TOK1", http.MethodPost,
		"https://snap.test/api/v2/chat/completions?b=2&a=1", body, map[string]string{"maas_type": "benefit"}, stamp)
	if err != nil {
		t.Fatalf("SignRequestHuawei: %v", err)
	}
	auth := signed["Authorization"]
	if !strings.HasPrefix(auth, "SDK-HMAC-SHA256 Access=AK1,SignedHeaders=") || !strings.Contains(auth, "maas_type") {
		t.Fatalf("unexpected Huawei Authorization header: %s", auth)
	}
	if signed["x-sdk-date"] != stamp || signed["x-security-token"] != "TOK1" || signed["maas_type"] != "benefit" {
		t.Fatalf("missing expected Huawei headers: %+v", signed)
	}

	v11, err := SignOfficeAceV11("https://oa-sub.test/v1/subscription?x=1", "AK1", "SK1", "TOK1", "proj-1", http.MethodGet, nil, stamp)
	if err != nil {
		t.Fatalf("SignOfficeAceV11: %v", err)
	}
	if !strings.HasPrefix(v11["Authorization"], "V11-HMAC-SHA256 Credential=AK1/20261124/cn-southwest-2/apic, SignedHeaders=") {
		t.Fatalf("unexpected V11 Authorization header: %s", v11["Authorization"])
	}
	if v11["X-Project-ID"] != "proj-1" || v11["x-subscription-type"] != "v2" {
		t.Fatalf("unexpected V11 headers: %+v", v11)
	}

	pkce, err := GeneratePKCE()
	if err != nil || len(pkce.Verifier) < 40 || pkce.Challenge == "" {
		t.Fatalf("bad PKCE: %+v err=%v", pkce, err)
	}
	jwk, err := GenerateDPoPKey()
	if err != nil {
		t.Fatalf("GenerateDPoPKey: %v", err)
	}
	jws, err := SignDPoPJWS(jwk, http.MethodPost, "https://sts.test/v1/oauth2/tokens", time.Unix(1795500000, 0))
	if err != nil {
		t.Fatalf("SignDPoPJWS: %v", err)
	}
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWS parts, got %d", len(parts))
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sigBytes) != 64 {
		t.Fatalf("expected 64-byte raw ES256 signature, got len=%d err=%v", len(sigBytes), err)
	}
	priv, _, err := parseDPoPPrivateKey(jwk)
	if err != nil {
		t.Fatalf("parseDPoPPrivateKey: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	if !ecdsa.Verify(&priv.PublicKey, digest[:], r, s) {
		t.Fatal("DPoP ES256 signature failed verification")
	}
}

func TestChatPrepToolShorteningAndSSEFold(t *testing.T) {
	longTool := "mcp__very_long_namespace_prefix__some_extremely_verbose_tool_function_name_exceeding_64_chars"
	raw := map[string]any{
		"model":            "codearts/deepseek-v4-pro-0813",
		"stream":           false,
		"reasoning_effort": "high",
		"stop":             []any{"A", "B", "C", "D", "E"},
		"messages": []any{
			map[string]any{"role": "user", "content": "run tool"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":       longTool,
					"parameters": map[string]any{"type": "object"},
				},
			},
		},
	}
	prep := PrepareChatBody(raw, true, "mainland")
	if prep == nil {
		t.Fatal("PrepareChatBody returned nil")
	}
	if !prep.Fold {
		t.Fatal("expected stream:false to enable SSE fold")
	}
	if prep.Body["stream"] != true {
		t.Fatalf("expected outbound stream=true, got %v", prep.Body["stream"])
	}
	if len(prep.Names) != 1 {
		t.Fatalf("expected 1 shortened tool name mapping, got %+v", prep.Names)
	}
	var shortName string
	for k, v := range prep.Names {
		shortName = k
		if len(k) > 64 || v != longTool {
			t.Fatalf("bad shortened tool name %q -> %q", k, v)
		}
	}
	if _, exists := prep.Body["stop"]; exists {
		t.Fatalf("mainland should strip stop, got %#v", prep.Body["stop"])
	}
	prepIntl := PrepareChatBody(map[string]any{
		"model":    "codearts/deepseek-v4-pro-0813",
		"stop":     []any{"A", "B"},
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}, true, "international")
	if stopIntl, ok := prepIntl.Body["stop"].([]string); !ok || len(stopIntl) != 2 {
		t.Fatalf("international should preserve stop strings, got %#v", prepIntl.Body["stop"])
	}
	sse := strings.Join([]string{
		fmt.Sprintf(`data: {"id":"cmpl-1","model":"deepseek-v4-pro-0813","choices":[{"index":0,"delta":{"reasoning_content":"plan","tool_calls":[{"index":0,"id":"tc1","type":"function","function":{"name":%q,"arguments":"{\"a\":"}}]}}]}`, shortName),
		`data: {"id":"cmpl-1","model":"deepseek-v4-pro-0813","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	events := ParseSSEEvents(sse)
	comp := CompletionFromChunks(events, prep.Names)
	choices, _ := comp["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("expected 1 choice, got %#v", comp)
	}
	choice0, _ := choices[0].(map[string]any)
	msg, _ := choice0["message"].(map[string]any)
	if msg["reasoning_content"] != "plan" {
		t.Fatalf("expected reasoning_content=plan, got %#v", msg)
	}
	tcs, _ := msg["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", msg["tool_calls"])
	}
	tc0, _ := tcs[0].(map[string]any)
	fnObj, _ := tc0["function"].(map[string]any)
	if fnObj["name"] != longTool || fnObj["arguments"] != `{"a":1}` {
		t.Fatalf("restored tool call mismatch: %#v", fnObj)
	}
}

func TestSavedSignInAndRefresh(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	var stsCalls atomic.Int32
	var clawCalls atomic.Int32
	mod := NewModule(ModuleOptions{
		Endpoints: testEndpoints(),
		Now:       func() time.Time { return now },
		HTTPFactory: func(string) (Doer, func(), error) {
			return testDoer(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasPrefix(req.URL.String(), "https://sts.test/v1/oauth2/tokens"):
					stsCalls.Add(1)
					if req.Header.Get("DPoP") == "" {
						t.Errorf("expected DPoP header on STS token refresh")
					}
					raw, _ := io.ReadAll(req.Body)
					if strings.Contains(string(raw), "dead-refresh") {
						return makeResponse(400, `{"error":"invalid_grant","error_description":"refresh token expired"}`, "application/json", nil), nil
					}
					return makeResponse(200, `{"credentials":{"access_key_id":"AK-NEW","secret_access_key":"SK-NEW","security_token":"TOK-NEW","expiration":"2026-11-25T12:00:00Z"},"refresh_token":"refresh-tok-2"}`, "application/json", nil), nil
				case strings.HasPrefix(req.URL.String(), "https://claw.test"+OAPermissionPath):
					clawCalls.Add(1)
					return makeResponse(200, `{"model_auth_info":{"app_key":"oa-k2","app_secret":"oa-s2","base_url":"https://maas.test"}}`, "application/json", nil), nil
				default:
					return makeResponse(404, `{}`, "application/json", nil), nil
				}
			}), func() {}, nil
		},
	})
	fx := newTestHarness(t, mod)
	cred := sampleSTSCred(t, "mainland")
	savedBytes, _ := json.Marshal(cred)

	startFrame, _ := fx.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: CodeArtsProvider}, sdk.AuthStartRequest{
		Method: sdk.SavedSignInMethod,
		Input:  rawJSON(map[string]string{"record": string(savedBytes)}),
	}, nil)
	if startFrame.Error != nil {
		t.Fatalf("saved-signin start failed: %v", startFrame.Error)
	}
	var startRes sdk.AuthStartResult
	_ = json.Unmarshal(startFrame.Result, &startRes)

	finFrame, _ := fx.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: CodeArtsProvider}, sdk.AuthFinishRequest{
		Continuation: startRes.Continuation,
	}, nil)
	if finFrame.Error != nil {
		t.Fatalf("saved-signin finish failed: %v", finFrame.Error)
	}
	var finRes sdk.AuthResult
	_ = json.Unmarshal(finFrame.Result, &finRes)
	if finRes.Pending || finRes.ID != "hw-user-42" {
		t.Fatalf("unexpected finish result: %+v", finRes)
	}

	refFrame, _ := fx.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: CodeArtsProvider}, sdk.AuthRefreshRequest{
		Credentials: finRes.Credentials,
	}, nil)
	if refFrame.Error != nil {
		t.Fatalf("codearts refresh failed: %v", refFrame.Error)
	}
	var refRes sdk.AuthResult
	_ = json.Unmarshal(refFrame.Result, &refRes)
	var refreshed CodeArtsCredential
	_ = json.Unmarshal(refRes.Credentials, &refreshed)
	if refreshed.AccessKeyID != "AK-NEW" || refreshed.RefreshToken != "refresh-tok-2" || stsCalls.Load() != 1 {
		t.Fatalf("unexpected refreshed cred: %+v stsCalls=%d", refreshed, stsCalls.Load())
	}

	dead := cred
	dead.RefreshToken = "dead-refresh"
	deadFrame, _ := fx.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: CodeArtsProvider}, sdk.AuthRefreshRequest{
		Credentials: rawJSON(dead),
	}, nil)
	if deadFrame.Error == nil || deadFrame.Error.Code != "unauthenticated" {
		t.Fatalf("expected unauthenticated on dead refresh token, got %+v", deadFrame.Error)
	}

	oaCred := sampleOACred(t)
	oaRefFrame, _ := fx.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: OfficeAceProvider}, sdk.AuthRefreshRequest{
		Credentials: rawJSON(oaCred),
	}, nil)
	if oaRefFrame.Error != nil {
		t.Fatalf("officeace refresh failed: %v", oaRefFrame.Error)
	}
	var oaRefRes sdk.AuthResult
	_ = json.Unmarshal(oaRefFrame.Result, &oaRefRes)
	var oaNext CodeArtsCredential
	_ = json.Unmarshal(oaRefRes.Credentials, &oaNext)
	if oaNext.ModelAppKey != "oa-k2" || oaNext.ModelAppSecret != "oa-s2" || clawCalls.Load() != 1 {
		t.Fatalf("unexpected refreshed officeace cred: %+v clawCalls=%d", oaNext, clawCalls.Load())
	}
}

func TestCodeArtsBenefitIsolationC33(t *testing.T) {
	now := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC) // 11:00 Beijing
	var (
		mu                sync.Mutex
		balanceFail       bool
		balanceUsed       int64 = 100
		balanceLimit      int64 = 1000
		refuseBenefitCode string
		chatCalls         []string
	)
	mod := NewModule(ModuleOptions{
		Endpoints: testEndpoints(),
		Now:       func() time.Time { return now },
		HTTPFactory: func(string) (Doer, func(), error) {
			return testDoer(func(req *http.Request) (*http.Response, error) {
				u := req.URL.String()
				switch {
				case strings.Contains(u, "/api/v1/gateway/config"):
					return makeResponse(200, `{"result":{"models":[{"model_id":"deepseek-v4-flash-0731","model_name":"DeepSeek V4 Flash"},{"model_id":"benefit-only-model","model_name":"Benefit Only"}]}}`, "application/json", nil), nil
				case strings.Contains(u, "/api/v1/benefit/claim"):
					return makeResponse(200, `{"error_code":"0000","error_msg":"ok"}`, "application/json", nil), nil
				case strings.Contains(u, "/v1/model/builtin"):
					return makeResponse(200, `{"builtinModels":[{"model_id":"deepseek-v4-flash-0731","model_name":"DeepSeek V4 Flash"},{"model_id":"paid-only-model","model_name":"Paid Only"}]}`, "application/json", nil), nil
				case strings.Contains(u, "/v1/agents"):
					return makeResponse(200, `{"agents":[]}`, "application/json", nil), nil
				case strings.Contains(u, "/snap-manager/v1/statistics/plugin"):
					return makeResponse(200, `{"code":0,"data":{"package":{"name":"Pro"},"metrics":[{"name":"usageTotalPackageCredit","package_credit_amount":1000,"package_credit_remain":800},{"name":"usageBasicPackageCredit","package_credit_amount":500,"package_credit_remain":400}]}}`, "application/json", nil), nil
				case strings.Contains(u, "/api/v1/user/tokens/balance"):
					mu.Lock()
					fail := balanceFail
					used := balanceUsed
					lim := balanceLimit
					mu.Unlock()
					if fail {
						return makeResponse(502, `bad gateway`, "text/plain", nil), nil
					}
					return makeResponse(200, fmt.Sprintf(`{"error_code":"0000","result":{"daily_tokens_used":%d,"daily_token_limit":%d}}`, used, lim), "application/json", nil), nil
				case strings.Contains(u, "/api/v2/chat/completions"):
					maasType := req.Header.Get("maas_type")
					mu.Lock()
					chatCalls = append(chatCalls, maasType)
					refCode := refuseBenefitCode
					mu.Unlock()
					if maasType == "benefit" && refCode != "" {
						return makeResponse(200, fmt.Sprintf("data: {\"error_code\":%q,\"error_msg\":\"no benefit\"}\n\n", refCode), "text/event-stream", nil), nil
					}
					return makeResponse(200, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "text/event-stream", nil), nil
				default:
					return makeResponse(404, `{}`, "application/json", nil), nil
				}
			}), func() {}, nil
		},
	})
	fx := newTestHarness(t, mod)
	cred := sampleSTSCred(t, "mainland")
	scope := sdk.Scope{Provider: CodeArtsProvider, Account: "acc-1", Generation: 1}

	// 1. Models discovery lists regular models + :benefit aliases.
	mFrame, _ := fx.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, &cred)
	if mFrame.Error != nil {
		t.Fatalf("models failed: %v", mFrame.Error)
	}
	var mRes sdk.ModelsResult
	_ = json.Unmarshal(mFrame.Result, &mRes)
	var ids []string
	aliasFree := false
	for _, m := range mRes.Models {
		ids = append(ids, m.ID)
		if m.ID == "deepseek-v4-flash-0731:benefit" {
			aliasFree = m.Free
		}
	}
	if !slices.Contains(ids, "deepseek-v4-flash-0731:benefit") || !slices.Contains(ids, "benefit-only-model:benefit") || !aliasFree {
		t.Fatalf("expected free :benefit aliases in models: %+v", ids)
	}

	// 2. Usage reads fresh balance and scopes windows cleanly (notModels on credits, models on 免费额度).
	uFrame, _ := fx.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{Revision: "17"}, &cred)
	if uFrame.Error != nil {
		t.Fatalf("usage failed: %v", uFrame.Error)
	}
	var uRes sdk.UsageResult
	_ = json.Unmarshal(uFrame.Result, &uRes)
	if uRes.Revision != "17" {
		t.Fatalf("CodeArts did not acknowledge usage revision 17: %q", uRes.Revision)
	}
	var foundCredit, foundBenefit bool
	for _, w := range uRes.Windows {
		if w.Name == "总积分" {
			foundCredit = true
			if !slices.Contains(w.NotModels, "deepseek-v4-flash-0731:benefit") {
				t.Fatalf("credit window missing alias in NotModels: %+v", w)
			}
		}
		if w.Name == BenefitWindowLabel {
			foundBenefit = true
			if w.Used != 10 || !slices.Contains(w.Models, "deepseek-v4-flash-0731:benefit") || strings.HasPrefix(w.Display, "上次已知：") {
				t.Fatalf("unexpected fresh benefit window: %+v", w)
			}
		}
	}
	if !foundCredit || !foundBenefit {
		t.Fatalf("missing credit or benefit window: %+v", uRes.Windows)
	}

	// 3. :benefit alias sends signed maas_type=benefit when fresh & under 100%.
	execReq := func(model string) (sdk.Frame, []byte) {
		body := rawJSON(map[string]any{
			"model":    model,
			"stream":   true,
			"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
		return fx.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
			Protocol: "chat",
			Model:    model,
			Body:     body,
		}, &cred)
	}
	eFrame, _ := execReq("deepseek-v4-flash-0731:benefit")
	if eFrame.Error != nil {
		t.Fatalf("benefit execute failed: %v", eFrame.Error)
	}
	var eRes sdk.ExecuteResult
	_ = json.Unmarshal(eFrame.Result, &eRes)
	if eRes.Status != 200 || len(chatCalls) != 1 || chatCalls[0] != "benefit" {
		t.Fatalf("expected single signed benefit POST, status=%d calls=%v", eRes.Status, chatCalls)
	}

	// 4. Stale same-day balance preservation: failing balance keeps previous window with 上次已知：
	// and blocks :benefit alias with 404 and zero upstream POSTs.
	mu.Lock()
	balanceFail = true
	chatCalls = nil
	mu.Unlock()
	uFrame2, _ := fx.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{}, &cred)
	var uRes2 sdk.UsageResult
	_ = json.Unmarshal(uFrame2.Result, &uRes2)
	staleSeen := false
	for _, w := range uRes2.Windows {
		if w.Name == BenefitWindowLabel && strings.HasPrefix(w.Display, "上次已知：") {
			staleSeen = true
		}
	}
	if !staleSeen {
		t.Fatalf("expected stale benefit window prefixed with 上次已知：, got %+v", uRes2.Windows)
	}
	eFrameStale, _ := execReq("deepseek-v4-flash-0731:benefit")
	var eResStale sdk.ExecuteResult
	_ = json.Unmarshal(eFrameStale.Result, &eResStale)
	if eResStale.Status != 404 || len(chatCalls) != 0 {
		t.Fatalf("stale balance must reject :benefit with 404 and zero POSTs, got status=%d calls=%v", eResStale.Status, chatCalls)
	}

	// 5. Refresh balance back to fresh, then verify upstream InferHub.4004.200 on :benefit alias
	// returns 404 with ZERO paid retry (only 1 POST with maas_type=benefit).
	mu.Lock()
	balanceFail = false
	balanceUsed = 200
	refuseBenefitCode = "InferHub.4004.200"
	chatCalls = nil
	mu.Unlock()
	_, _ = fx.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{}, &cred)
	eFrameRefuse, _ := execReq("deepseek-v4-flash-0731:benefit")
	var eResRefuse sdk.ExecuteResult
	_ = json.Unmarshal(eFrameRefuse.Result, &eResRefuse)
	if eResRefuse.Status != 404 || !slices.Equal(chatCalls, []string{"benefit"}) {
		t.Fatalf(":benefit refusal must NOT retry paid; status=%d calls=%v", eResRefuse.Status, chatCalls)
	}

	// 6. Auto (non-alias) request on regular model whose alias was refused skips benefit and uses paid directly.
	mu.Lock()
	chatCalls = nil
	mu.Unlock()
	eFrameAuto, _ := execReq("deepseek-v4-flash-0731")
	var eResAuto sdk.ExecuteResult
	_ = json.Unmarshal(eFrameAuto.Result, &eResAuto)
	if eResAuto.Status != 200 || !slices.Equal(chatCalls, []string{""}) {
		t.Fatalf("expected auto request on regular model to route without benefit header, status=%d calls=%v", eResAuto.Status, chatCalls)
	}

	// 7. When daily balance is 100% exhausted:
	// - benefit-only model (not in regular) returns 404 with zero POSTs.
	// - regular model auto request succeeds without maas_type=benefit.
	mu.Lock()
	balanceUsed = 1000
	refuseBenefitCode = ""
	chatCalls = nil
	mu.Unlock()
	_, _ = fx.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{}, &cred)
	eFrameBenOnly, _ := execReq("benefit-only-model")
	var eResBenOnly sdk.ExecuteResult
	_ = json.Unmarshal(eFrameBenOnly.Result, &eResBenOnly)
	if eResBenOnly.Status != 404 || len(chatCalls) != 0 {
		t.Fatalf("exhausted benefit-only model must return 404 with zero POSTs, status=%d calls=%v", eResBenOnly.Status, chatCalls)
	}
	eFrameReg, _ := execReq("deepseek-v4-flash-0731")
	var eResReg sdk.ExecuteResult
	_ = json.Unmarshal(eFrameReg.Result, &eResReg)
	if eResReg.Status != 200 || !slices.Equal(chatCalls, []string{""}) {
		t.Fatalf("exhausted regular model should proceed paid, status=%d calls=%v", eResReg.Status, chatCalls)
	}
}

func TestCodeArtsExecuteErrorsRefreshAndFolding(t *testing.T) {
	now := time.Date(2026, 11, 24, 4, 0, 0, 0, time.UTC)
	var mode atomic.Int32
	var attempt atomic.Int32
	mod := NewModule(ModuleOptions{
		Endpoints: testEndpoints(),
		Now:       func() time.Time { return now },
		HTTPFactory: func(string) (Doer, func(), error) {
			return testDoer(func(req *http.Request) (*http.Response, error) {
				if strings.HasPrefix(req.URL.String(), "https://sts.test/v1/oauth2/tokens") {
					return makeResponse(200, `{"credentials":{"access_key_id":"AK-REF","secret_access_key":"SK-REF","security_token":"TOK-REF","expiration":"2026-11-25T12:00:00Z"},"refresh_token":"ref-2"}`, "application/json", nil), nil
				}
				switch mode.Load() {
				case 1:
					return makeResponse(429, `queued`, "text/plain", http.Header{
						"Queue-Status": {"queued"},
						"Retry-After":  {"7"},
					}), nil
				case 2:
					return makeResponse(200, "data: {\"error_code\":\"TM.00001041\",\"error_msg\":\"session limit\"}\n\n", "text/event-stream", nil), nil
				case 3:
					n := attempt.Add(1)
					if n == 1 {
						return makeResponse(401, `{"error_code":"APIG.0301","error_msg":"expired"}`, "application/json", nil), nil
					}
					if !strings.Contains(req.Header.Get("Authorization"), "Access=AK-REF") {
						t.Errorf("expected refreshed AK on retry, got %s", req.Header.Get("Authorization"))
					}
					raw, _ := io.ReadAll(req.Body)
					if n == 2 && bytes.Contains(raw, []byte(`"thinking"`)) {
						return makeResponse(400, `{"error":"unsupported field thinking"}`, "application/json", nil), nil
					}
					return makeResponse(200, "data: {\"id\":\"c2\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7}}\n\ndata: [DONE]\n\n", "text/event-stream", nil), nil
				default:
					return makeResponse(500, `{}`, "application/json", nil), nil
				}
			}), func() {}, nil
		},
	})
	fx := newTestHarness(t, mod)
	cred := sampleSTSCred(t, "mainland")
	scope := sdk.Scope{Provider: CodeArtsProvider, Account: "acc-2", Generation: 1}
	callExec := func(stream bool) (sdk.Frame, []byte) {
		return fx.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
			Protocol: "chat",
			Model:    "deepseek-v4-pro-0813",
			Body: rawJSON(map[string]any{
				"model":    "deepseek-v4-pro-0813",
				"stream":   stream,
				"messages": []any{map[string]any{"role": "user", "content": "hi"}},
			}),
		}, &cred)
	}

	mode.Store(1)
	fQueued, _ := callExec(true)
	var rQueued sdk.ExecuteResult
	_ = json.Unmarshal(fQueued.Result, &rQueued)
	if rQueued.Outcome != "not-started" || rQueued.Refusal == nil || rQueued.Refusal.Facts.Status != 503 || rQueued.Refusal.Facts.RetryAfterMS == nil || *rQueued.Refusal.Facts.RetryAfterMS != 7000 {
		t.Fatalf("expected queued 503 refusal with 7000ms retry, got %+v", rQueued)
	}

	mode.Store(2)
	fLimit, _ := callExec(true)
	var rLimit sdk.ExecuteResult
	_ = json.Unmarshal(fLimit.Result, &rLimit)
	if rLimit.Outcome != "not-started" || rLimit.Refusal == nil || rLimit.Refusal.Facts.Code != SessionLimitError {
		t.Fatalf("expected TM.00001041 refusal, got %+v", rLimit)
	}

	mode.Store(3)
	fFold, dataFold := callExec(false)
	if fFold.Error != nil {
		t.Fatalf("refresh+thinking retry fold failed: %v", fFold.Error)
	}
	var rFold sdk.ExecuteResult
	_ = json.Unmarshal(fFold.Result, &rFold)
	if rFold.Status != 200 || !strings.Contains(string(dataFold), `"content":"done"`) || len(fx.replaced) != 1 {
		t.Fatalf("expected folded 200 + account replace, status=%d replaced=%d body=%s", rFold.Status, len(fx.replaced), string(dataFold))
	}
}

func TestCodeArtsDailyCheckinAndRecovery(t *testing.T) {
	now := time.Date(2026, 11, 24, 5, 0, 0, 0, time.UTC)
	var (
		mu          sync.Mutex
		claimable   = true
		status      = "AVAILABLE"
		failConfirm = true
	)
	mod := NewModule(ModuleOptions{
		Endpoints: testEndpoints(),
		Now:       func() time.Time { return now },
		HTTPFactory: func(string) (Doer, func(), error) {
			return testDoer(func(req *http.Request) (*http.Response, error) {
				u := req.URL.String()
				mu.Lock()
				defer mu.Unlock()
				switch {
				case strings.Contains(u, "/v1/ops/delivery"):
					return makeResponse(200, fmt.Sprintf(`{"code":"0000","data":[{"campaignId":"camp-1","campaignType":"USER_LOGIN","campaignName":"每日登录领取积分","claimable":%v,"status":%q,"benefitAmount":100}]}`, claimable, status), "application/json", nil), nil
				case strings.Contains(u, "/v1/ops/claim"):
					claimable = false
					status = "CLAIMED"
					return makeResponse(200, `{"code":"0000","data":{"id":"ben-1","benefitAmount":100}}`, "application/json", nil), nil
				case strings.Contains(u, "/v1/ops/confirm"):
					if failConfirm {
						failConfirm = false
						return makeResponse(500, `{"error_msg":"confirm transient"}`, "application/json", nil), nil
					}
					status = "CONFIRMED"
					return makeResponse(200, `{"code":"0000"}`, "application/json", nil), nil
				default:
					return makeResponse(404, `{}`, "application/json", nil), nil
				}
			}), func() {}, nil
		},
	})
	fx := newTestHarness(t, mod)
	cred := sampleSTSCred(t, "mainland")
	scope := sdk.Scope{Provider: CodeArtsProvider, Account: "acc-checkin", Generation: 1}

	// First checkin claims, fails confirm, persists unconfirmed claim via StateCAS.
	checkinCall := func() sdk.Frame {
		fr, _ := fx.invoke(sdk.MethodModuleCall, scope, sdk.ModuleCallRequest{
			Action: "account.checkin",
			Input:  rawJSON(sdk.AccountCheckinRequest{Day: "2026-11-24"}),
		}, &cred)
		return fr
	}
	f1 := checkinCall()
	if f1.Error != nil {
		t.Fatalf("checkin 1 error: %v", f1.Error)
	}
	var r1 sdk.AccountCheckinResult
	_ = json.Unmarshal(f1.Result, &r1)
	if r1.Outcome != "failed" {
		t.Fatalf("expected failed outcome when confirm fails, got %+v", r1)
	}
	if len(fx.state) != 1 {
		t.Fatalf("expected unconfirmed claim saved in host state, got %+v", fx.state)
	}

	// Second checkin recovers unconfirmed claim and confirms it cleanly.
	f2 := checkinCall()
	var r2 sdk.AccountCheckinResult
	_ = json.Unmarshal(f2.Result, &r2)
	if r2.Outcome != "claimed" || !strings.Contains(r2.Message, "100") {
		t.Fatalf("expected recovered claimed outcome with 100 credits, got %+v", r2)
	}

	// Third checkin sees CONFIRMED and returns done.
	f3 := checkinCall()
	var r3 sdk.AccountCheckinResult
	_ = json.Unmarshal(f3.Result, &r3)
	if r3.Outcome != "done" {
		t.Fatalf("expected done outcome on third checkin, got %+v", r3)
	}
}

func TestOfficeAceEndToEnd(t *testing.T) {
	now := time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC)
	var chatCalls atomic.Int32
	mod := NewModule(ModuleOptions{
		Endpoints: testEndpoints(),
		Now:       func() time.Time { return now },
		HTTPFactory: func(string) (Doer, func(), error) {
			return testDoer(func(req *http.Request) (*http.Response, error) {
				u := req.URL.String()
				switch {
				case strings.HasPrefix(u, "https://maas.test"+OAModelsPath):
					return makeResponse(200, `{"data":[{"id":"deepseek-v3.2"},{"id":"glm-5"},{"id":"deepseek-v4-pro"}]}`, "application/json", nil), nil
				case strings.HasPrefix(u, "https://oa-sub.test"+OATokenRatesPath):
					if !strings.HasPrefix(req.Header.Get("Authorization"), "V11-HMAC-SHA256 ") {
						t.Errorf("expected V11 auth on token rates, got %s", req.Header.Get("Authorization"))
					}
					return makeResponse(200, `{"models":[{"model_name":"deepseek-v3.2","cost_coefficient":1.5},{"model_name":"glm-5","cost_coefficient":2.0},{"model_name":"deepseek-v4-pro","cost_coefficient":1.0}]}`, "application/json", nil), nil
				case strings.HasPrefix(u, "https://maas.test"+OAChatPath):
					n := chatCalls.Add(1)
					if n == 1 {
						return makeResponse(401, `{"error":"invalid basic auth"}`, "application/json", nil), nil
					}
					raw, _ := io.ReadAll(req.Body)
					if strings.Contains(string(raw), "officeace/") {
						t.Errorf("expected officeace/ prefix stripped, got %s", string(raw))
					}
					return makeResponse(200, `{"id":"oa-1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`, "application/json", nil), nil
				case strings.HasPrefix(u, "https://sts.test/v1/oauth2/tokens"):
					return makeResponse(200, `{"credentials":{"access_key_id":"AK-OA2","secret_access_key":"SK-OA2","security_token":"TOK-OA2","expiration":"2026-11-25T12:00:00Z"},"refresh_token":"ref-oa2"}`, "application/json", nil), nil
				case strings.HasPrefix(u, "https://claw.test"+OAPermissionPath):
					return makeResponse(200, `{"model_auth_info":{"app_key":"oa-key-2","app_secret":"oa-sec-2","base_url":"https://maas.test"}}`, "application/json", nil), nil
				case strings.HasPrefix(u, "https://oa-sub.test"+OASubscriptionPath) && !strings.Contains(u, "bonus"):
					return makeResponse(200, `{"skus":[{"sku_code":"pro.monthly","sku_name":"Pro","expired_time":"2026-12-24T00:00:00Z","quotas":[{"sku_attr_code":"officeace_points","sku_value":1000,"current_value":250}]}],"bonus_skus":[{"points":100,"current_value":20,"expired_time":"2026-12-01T00:00:00Z"}]}`, "application/json", nil), nil
				case strings.HasPrefix(u, "https://oa-sub.test"+OABonusClaimPath):
					return makeResponse(200, `{"bonus_skus":[{"activity_name":"每日签到","points":50}]}`, "application/json", nil), nil
				default:
					return makeResponse(404, `{}`, "application/json", nil), nil
				}
			}), func() {}, nil
		},
	})
	fx := newTestHarness(t, mod)
	cred := sampleOACred(t)
	scope := sdk.Scope{Provider: OfficeAceProvider, Account: "oa-1", Generation: 1}

	mFrame, _ := fx.invoke(sdk.MethodProviderModels, scope, sdk.ModelsRequest{}, &cred)
	if mFrame.Error != nil {
		t.Fatalf("officeace models failed: %v", mFrame.Error)
	}
	var mRes sdk.ModelsResult
	_ = json.Unmarshal(mFrame.Result, &mRes)
	var ids []string
	for _, m := range mRes.Models {
		ids = append(ids, m.ID)
	}
	if !slices.Contains(ids, "deepseek-v3.2") || !slices.Contains(ids, "glm-5") {
		t.Fatalf("expected live officeace models, got %v", ids)
	}
	if slices.Contains(ids, "deepseek-v4-pro") {
		t.Fatalf("unavailable OA model should have been filtered out: %v", ids)
	}

	eFrame, data := fx.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "officeace/deepseek-v3.2",
		Body:     rawJSON(map[string]any{"model": "officeace/deepseek-v3.2", "messages": []any{map[string]any{"role": "user", "content": "hi"}}}),
	}, &cred)
	if eFrame.Error != nil {
		t.Fatalf("officeace execute failed: %v", eFrame.Error)
	}
	var eRes sdk.ExecuteResult
	_ = json.Unmarshal(eFrame.Result, &eRes)
	if eRes.Status != 200 || chatCalls.Load() != 2 || len(fx.replaced) != 1 || !strings.Contains(string(data), `"hi"`) {
		t.Fatalf("expected OA 401 refresh+retry success: status=%d calls=%d replaced=%d body=%s", eRes.Status, chatCalls.Load(), len(fx.replaced), string(data))
	}

	uFrame, _ := fx.invoke(sdk.MethodProviderUsage, scope, sdk.UsageRequest{Revision: "18"}, &cred)
	if uFrame.Error != nil {
		t.Fatalf("officeace usage failed: %v", uFrame.Error)
	}
	var uRes sdk.UsageResult
	_ = json.Unmarshal(uFrame.Result, &uRes)
	if uRes.Revision != "18" {
		t.Fatalf("OfficeAce did not acknowledge usage revision 18: %q", uRes.Revision)
	}
	if len(uRes.Windows) < 2 {
		t.Fatalf("expected OA usage windows + aggregate, got %+v", uRes.Windows)
	}

	cFrame, _ := fx.invoke(sdk.MethodModuleCall, scope, sdk.ModuleCallRequest{
		Action: "account.checkin",
		Input:  rawJSON(sdk.AccountCheckinRequest{Day: "2026-11-24"}),
	}, &cred)
	if cFrame.Error != nil {
		t.Fatalf("officeace checkin failed: %v", cFrame.Error)
	}
	var cRes sdk.AccountCheckinResult
	_ = json.Unmarshal(cFrame.Result, &cRes)
	if cRes.Outcome != "claimed" || cRes.Credit == nil || *cRes.Credit != 50 {
		t.Fatalf("expected OA checkin claimed, got %+v", cRes)
	}
}
