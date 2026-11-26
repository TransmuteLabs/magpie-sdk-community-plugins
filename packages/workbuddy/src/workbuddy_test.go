package workbuddy

import (
	"bytes"
	"compress/gzip"
	"context"
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

func jsonResponse(status int, body any, extra http.Header) *http.Response {
	var text string
	switch typed := body.(type) {
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
		Status:     fmt.Sprintf("%d %s", status, statusText(status)),
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader(text)),
	}
}

func rawJSON(val any) json.RawMessage {
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

type wbHarness struct {
	t        *testing.T
	conn     net.Conn
	enc      *json.Encoder
	dec      *json.Decoder
	seq      int
	mu       sync.Mutex
	replaced []json.RawMessage
	events   []sdk.Frame
}

func newWBHarness(t *testing.T, mod sdk.Module, configValues json.RawMessage) *wbHarness {
	t.Helper()
	if err := mod.Validate(); err != nil {
		t.Fatalf("module validation failed: %v", err)
	}
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- sdk.Serve(ctx, worker, worker, mod) }()
	harness := &wbHarness{
		t:    t,
		conn: host,
		enc:  json.NewEncoder(host),
		dec:  json.NewDecoder(host),
	}
	t.Cleanup(func() {
		cancel()
		host.Close()
		worker.Close()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("WorkBuddy SDK fixture did not stop")
		}
	})
	if len(configValues) == 0 {
		configValues = json.RawMessage(`{}`)
	}
	frame, _ := harness.invoke(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent,
		Instance:  "workbuddy-test",
		Config:    sdk.ConfigSnapshot{Version: "1", Values: configValues},
	}, nil)
	if frame.Error != nil {
		t.Fatalf("configure failed: %v", frame.Error)
	}
	return harness
}

func (harness *wbHarness) invoke(method string, scope sdk.Scope, params any, cred *Credential) (sdk.Frame, []byte) {
	harness.t.Helper()
	harness.seq++
	id := fmt.Sprintf("call-%d", harness.seq)
	scope.Plugin = ModuleID
	scope.Operation = id
	req := sdk.Frame{Kind: "call", ID: id, Method: method, Scope: scope, Params: rawJSON(params)}
	if cred != nil {
		req.Auth = rawJSON(cred)
	}
	_ = harness.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := harness.enc.Encode(req); err != nil {
		harness.t.Fatal(err)
	}
	var data []byte
	for {
		var frame sdk.Frame
		if err := harness.dec.Decode(&frame); err != nil {
			harness.t.Fatalf("decode during %s: %v", method, err)
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
				harness.t.Fatal(err)
			}
		case sdk.KindHostCall:
			resFrame := sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: id}
			switch frame.Method {
			case sdk.MethodNetworkProxy:
				resFrame.Result = rawJSON(sdk.NetworkProxyResult{})
			case sdk.MethodAccountReplace:
				var repReq sdk.AccountReplaceRequest
				_ = json.Unmarshal(frame.Params, &repReq)
				harness.mu.Lock()
				harness.replaced = append(harness.replaced, repReq.Credentials)
				harness.mu.Unlock()
				resFrame.Result = rawJSON(sdk.AccountReplaceResult{})
			default:
				resFrame.Error = &sdk.Error{Code: "unsupported", Message: "unsupported host call " + frame.Method}
			}
			if err := harness.enc.Encode(resFrame); err != nil {
				harness.t.Fatal(err)
			}
		case sdk.KindResult:
			return frame, data
		default:
			harness.t.Fatalf("unexpected frame kind %q", frame.Kind)
		}
	}
}

func sampleOAuthCred(siteID string, expiresMs int64) Credential {
	domain := "copilot.tencent.com"
	if siteID == ProviderAI {
		domain = "www.workbuddy.ai"
	}
	return Credential{
		Type:             "oauth",
		Source:           "browser",
		Access:           "wb-access-1",
		Refresh:          "wb-refresh-1",
		Expires:          expiresMs,
		RefreshExpiresAt: expiresMs + 30*24*3600*1000,
		Domain:           domain,
		UID:              "uid-100",
		AccountID:        "acct-100",
		User:             "alice",
		Name:             "Alice",
	}
}

func gunzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("gunzip read: %v", err)
	}
	return out
}

func TestManifestAndOptions(t *testing.T) {
	manifest := Manifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Manifest().Validate() failed: %v", err)
	}
	if manifest.ID != ModuleID {
		t.Fatalf("manifest ID = %q, want %q", manifest.ID, ModuleID)
	}
	if len(manifest.Providers) != 2 {
		t.Fatalf("providers count = %d, want 2", len(manifest.Providers))
	}
	cn := manifest.Providers[0]
	ai := manifest.Providers[1]
	if cn.ID != ProviderWorkBuddy || ai.ID != ProviderAI {
		t.Fatalf("unexpected provider IDs: %q, %q", cn.ID, ai.ID)
	}
	if cn.Checkin == nil || cn.Checkin.Action != "account.checkin" {
		t.Fatalf("CN provider missing checkin action: %+v", cn.Checkin)
	}
	if ai.Checkin != nil {
		t.Fatalf("AI provider should not declare checkin: %+v", ai.Checkin)
	}
	if len(cn.Options) != 2 || cn.Options[0].Key != FreeOnlyKey || cn.Options[1].Key != HideDSHKey {
		t.Fatalf("unexpected CN options: %+v", cn.Options)
	}
	if len(ai.Options) != 2 || ai.Options[0].Key != AIFreeOnlyKey || ai.Options[1].Key != AIHideDSHKey {
		t.Fatalf("unexpected AI options: %+v", ai.Options)
	}
	if !cn.Options[0].FiltersModels || !ai.Options[0].FiltersModels {
		t.Fatalf("free_only options must set FiltersModels")
	}
	if !cn.Options[1].Default || !ai.Options[1].Default {
		t.Fatalf("hide_dsh options must default to true")
	}
	if len(manifest.AuthBindings) != 2 || len(manifest.AuthBindings[0].Methods) != 4 || len(manifest.AuthBindings[1].Methods) != 4 {
		t.Fatalf("unexpected AuthBindings: %+v", manifest.AuthBindings)
	}
}

func TestAuthMethodsAndRefresh(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	tempDir := t.TempDir()
	desktopCNPath := filepath.Join(tempDir, "workbuddy-desktop.info")
	desktopAIPath := filepath.Join(tempDir, "workbuddy-desktop-ai.info")

	desktopPayload := map[string]any{
		"auth": map[string]any{
			"accessToken":  "desk-tok-1",
			"refreshToken": "desk-ref-1",
			"expiresAt":    now.Add(2 * time.Hour).UnixMilli(),
			"domain":       "copilot.tencent.com",
		},
		"account": map[string]any{
			"uid":      "desk-uid",
			"nickname": "DeskUser",
		},
	}
	desktopBytes, _ := json.Marshal(desktopPayload)
	if err := os.WriteFile(desktopCNPath, desktopBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktopAIPath, []byte(`{"auth":{"accessToken":{"$wbEncrypted":true,"ciphertext":"abc"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var pollCount atomic.Int32
	var refreshCount atomic.Int32
	factory := func(string) (Doer, func(), error) {
		return testDoer(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/v2/plugin/auth/state":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"state":   "st-123",
						"authUrl": "https://copilot.tencent.com/login?state=st-123",
					},
				}, nil), nil
			case "/v2/plugin/auth/token":
				if pollCount.Add(1) == 1 {
					return jsonResponse(200, map[string]any{"code": 11217, "msg": "waiting"}, nil), nil
				}
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"accessToken":     "browser-access-1",
						"refreshToken":    "browser-refresh-1",
						"expiresIn":       3600,
						"refreshExpireIn": 86400,
						"domain":          "copilot.tencent.com",
					},
				}, nil), nil
			case "/v2/plugin/login/account":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"uid":      "u-browser",
						"nickname": "BrowserUser",
						"email":    "alice@example.com",
					},
				}, nil), nil
			case "/v2/plugin/auth/token/refresh":
				refreshCount.Add(1)
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"accessToken":  "wb-access-refreshed",
						"refreshToken": "wb-refresh-2",
						"expiresIn":    7200,
					},
				}, nil), nil
			case "/v3/config":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{"models": []any{}},
				}, nil), nil
			default:
				return jsonResponse(200, map[string]any{"code": 0, "data": map[string]any{}}, nil), nil
			}
		}), func() {}, nil
	}

	mod := NewModule(ModuleOptions{
		Endpoints:   Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:         func() time.Time { return now },
		HTTPFactory: factory,
		DesktopPath: func(site Site) string {
			if site.AI {
				return desktopAIPath
			}
			return desktopCNPath
		},
		PollInterval: 1 * time.Millisecond,
		SignInWindow: 2 * time.Second,
	})
	harness := newWBHarness(t, mod, nil)

	startFrame, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthStartRequest{
		Method: "browser",
	}, nil)
	if startFrame.Error != nil {
		t.Fatalf("browser authStart failed: %v", startFrame.Error)
	}
	var started sdk.AuthStartResult
	if err := json.Unmarshal(startFrame.Result, &started); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(started.URL, "version=5.9.3") {
		t.Fatalf("browser URL missing version=5.9.3: %q", started.URL)
	}
	var finished sdk.AuthResult
	for attempt := 0; attempt < 4; attempt++ {
		finishFrame, _ := harness.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthFinishRequest{
			Continuation: started.Continuation,
		}, nil)
		if finishFrame.Error != nil {
			t.Fatalf("browser authFinish failed: %v", finishFrame.Error)
		}
		finished = sdk.AuthResult{}
		if err := json.Unmarshal(finishFrame.Result, &finished); err != nil {
			t.Fatal(err)
		}
		if !finished.Pending {
			break
		}
	}
	if finished.Pending {
		t.Fatalf("browser authFinish stayed pending")
	}
	var gotCred Credential
	if err := json.Unmarshal(finished.Credentials, &gotCred); err != nil {
		t.Fatal(err)
	}
	if gotCred.Access != "browser-access-1" || gotCred.UID != "u-browser" {
		t.Fatalf("unexpected browser cred: %+v", gotCred)
	}

	deskStart, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthStartRequest{
		Method: "desktop",
	}, nil)
	var deskStarted sdk.AuthStartResult
	_ = json.Unmarshal(deskStart.Result, &deskStarted)
	deskFinish, _ := harness.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthFinishRequest{
		Continuation: deskStarted.Continuation,
	}, nil)
	if deskFinish.Error != nil {
		t.Fatalf("desktop CN finish failed: %v", deskFinish.Error)
	}
	var deskFinRes sdk.AuthResult
	_ = json.Unmarshal(deskFinish.Result, &deskFinRes)
	var deskCred Credential
	_ = json.Unmarshal(deskFinRes.Credentials, &deskCred)
	if deskCred.Source != "desktop" || deskCred.Access != "" || deskCred.UID != "desk-uid" || deskCred.Name != "DeskUser" {
		t.Fatalf("unexpected desktop cred: %+v", deskCred)
	}

	aiDeskStart, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderAI}, sdk.AuthStartRequest{
		Method: "desktop",
	}, nil)
	var aiDeskStarted sdk.AuthStartResult
	_ = json.Unmarshal(aiDeskStart.Result, &aiDeskStarted)
	aiDeskFinish, _ := harness.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderAI}, sdk.AuthFinishRequest{
		Continuation: aiDeskStarted.Continuation,
	}, nil)
	if aiDeskFinish.Error == nil || !strings.Contains(aiDeskFinish.Error.Message, "encrypted") {
		t.Fatalf("expected encrypted desktop error on AI, got: %+v", aiDeskFinish.Error)
	}

	keyStart, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthStartRequest{
		Method: "api-key",
		Input:  rawJSON(map[string]string{"apiKey": "ck_live_secret_123"}),
	}, nil)
	var keyStarted sdk.AuthStartResult
	_ = json.Unmarshal(keyStart.Result, &keyStarted)
	keyFinish, _ := harness.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthFinishRequest{
		Continuation: keyStarted.Continuation,
	}, nil)
	if keyFinish.Error != nil {
		t.Fatalf("api-key finish failed: %v", keyFinish.Error)
	}
	var keyFinRes sdk.AuthResult
	_ = json.Unmarshal(keyFinish.Result, &keyFinRes)
	var keyCred Credential
	_ = json.Unmarshal(keyFinRes.Credentials, &keyCred)
	if keyCred.Source != "key" || !remoteConfigKey(keyCred.Key) {
		t.Fatalf("unexpected api-key cred: %+v", keyCred)
	}

	savedCrossStart, _ := harness.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderAI}, sdk.AuthStartRequest{
		Method: sdk.SavedSignInMethod,
		Input:  rawJSON(map[string]string{"record": string(desktopBytes)}),
	}, nil)
	var savedCrossStarted sdk.AuthStartResult
	_ = json.Unmarshal(savedCrossStart.Result, &savedCrossStarted)
	savedCrossFinish, _ := harness.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderAI}, sdk.AuthFinishRequest{
		Continuation: savedCrossStarted.Continuation,
	}, nil)
	if savedCrossFinish.Error == nil || !strings.Contains(savedCrossFinish.Error.Message, "WorkBuddy") {
		t.Fatalf("expected cross-site import rejection, got: %+v", savedCrossFinish.Error)
	}

	expiredCred := sampleOAuthCred(ProviderWorkBuddy, now.Add(-5*time.Minute).UnixMilli())
	refFrame, _ := harness.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.AuthRefreshRequest{}, &expiredCred)
	if refFrame.Error != nil {
		t.Fatalf("authRefresh failed: %v", refFrame.Error)
	}
	var refRes sdk.AuthResult
	_ = json.Unmarshal(refFrame.Result, &refRes)
	var refreshedCred Credential
	_ = json.Unmarshal(refRes.Credentials, &refreshedCred)
	if refreshedCred.Access != "wb-access-refreshed" || refreshCount.Load() != 1 {
		t.Fatalf("unexpected refresh result: %+v count=%d", refreshedCred, refreshCount.Load())
	}
}

func TestModelsPromotionsAndFreeOnly(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	configPayload := map[string]any{
		"code": 0,
		"data": map[string]any{
			"agents": []any{
				map[string]any{
					"name":   "cli",
					"models": []any{"deepseek-v4.1-flash", "glm-5.0-night", "claude-4.5-sonnet"},
				},
			},
			"models": []any{
				map[string]any{
					"id":              "deepseek-v4.1-flash",
					"name":            "DeepSeek Flash",
					"credits":         "0",
					"maxInputTokens":  131072,
					"maxOutputTokens": 16384,
					"reasoning": map[string]any{
						"supportedEfforts": []any{"low", "medium", "high"},
						"defaultEffort":    "medium",
					},
				},
				map[string]any{
					"id":              "glm-5.0-night",
					"name":            "GLM Night",
					"credits":         "1.0",
					"maxInputTokens":  131072,
					"maxOutputTokens": 8192,
					"reasoning": map[string]any{
						"supportedEfforts": []any{"high"},
					},
				},
				map[string]any{
					"id":              "claude-4.5-sonnet",
					"name":            "Claude Sonnet",
					"credits":         "2.0",
					"maxInputTokens":  200000,
					"maxOutputTokens": 16384,
					"reasoning": map[string]any{
						"supportedEfforts": []any{"low", "high"},
					},
				},
			},
			"modelPromotions": []any{
				map[string]any{
					"kind":     "discount",
					"enabled":  true,
					"modelIds": []any{"glm-5.0-night"},
					"discount": map[string]any{"discountedCredits": "0"},
					"schedule": map[string]any{
						"validFrom":  now.Add(-time.Hour).Format(time.RFC3339),
						"validUntil": now.Add(time.Hour).Format(time.RFC3339),
						"daily":      []any{map[string]any{"start": "00:00", "end": "08:00"}},
					},
				},
			},
		},
	}
	var chatHits atomic.Int32
	factory := func(string) (Doer, func(), error) {
		return testDoer(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/v3/config":
				return jsonResponse(200, configPayload, nil), nil
			case "/v2/chat/completions":
				chatHits.Add(1)
				return jsonResponse(200, map[string]any{"id": "ok"}, nil), nil
			default:
				return jsonResponse(404, map[string]any{"error": "not found"}, nil), nil
			}
		}), func() {}, nil
	}

	mod := NewModule(ModuleOptions{
		Endpoints:   Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:         func() time.Time { return now },
		HTTPFactory: factory,
	})
	harness := newWBHarness(t, mod, json.RawMessage(`{"free_only":true}`))
	cred := sampleOAuthCred(ProviderWorkBuddy, now.Add(time.Hour).UnixMilli())

	modelsFrame, _ := harness.invoke(sdk.MethodProviderModels, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.ModelsRequest{}, &cred)
	if modelsFrame.Error != nil {
		t.Fatalf("models failed: %v", modelsFrame.Error)
	}
	var modelsRes sdk.ModelsResult
	if err := json.Unmarshal(modelsFrame.Result, &modelsRes); err != nil {
		t.Fatal(err)
	}
	if len(modelsRes.Models) != 1 || modelsRes.Models[0].ID != "deepseek-v4.1-flash" {
		t.Fatalf("free_only should list only always-free model (excluding night-free), got: %+v", modelsRes.Models)
	}

	execFrame, execData := harness.invoke(sdk.MethodProviderExecute, sdk.Scope{
		Provider:   ProviderWorkBuddy,
		Account:    "acct-100",
		Generation: 1,
	}, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "glm-5.0-night",
		Body:     []byte(`{"model":"glm-5.0-night","messages":[{"role":"user","content":"hi"}]}`),
	}, &cred)
	if execFrame.Error != nil {
		t.Fatalf("execute returned protocol error instead of 404 payload: %v", execFrame.Error)
	}
	var execRes sdk.ExecuteResult
	_ = json.Unmarshal(execFrame.Result, &execRes)
	if execRes.Status != 404 || execRes.Headers.Get("X-Magpie-Sign-In") != "kept" {
		t.Fatalf("expected 404 with X-Magpie-Sign-In: kept, got status=%d hdr=%v body=%s", execRes.Status, execRes.Headers, string(execData))
	}
	if chatHits.Load() != 0 {
		t.Fatalf("paid model request should never be sent when free_only is on; hits=%d", chatHits.Load())
	}
}

func TestRequestRewritesRetriesAndTelemetry(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	var seenChatBodies []map[string]any
	var seenReports [][]map[string]any
	var chatAttempt int

	factory := func(string) (Doer, func(), error) {
		return testDoer(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/v3/config":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"agents": []any{
							map[string]any{"name": "cli", "models": []any{"kimi-k3.1-pro"}},
						},
						"models": []any{
							map[string]any{
								"id":      "kimi-k3.1-pro",
								"name":    "Kimi Pro",
								"credits": "0",
								"reasoning": map[string]any{
									"supportedEfforts": []any{"low", "high"},
									"defaultEffort":    "high",
								},
							},
						},
					},
				}, nil), nil
			case "/v2/report":
				raw, _ := io.ReadAll(req.Body)
				var batch []map[string]any
				_ = json.Unmarshal(raw, &batch)
				mu.Lock()
				seenReports = append(seenReports, batch)
				mu.Unlock()
				return jsonResponse(200, map[string]any{"code": 0}, nil), nil
			case "/v2/chat/completions":
				if req.Header.Get("Content-Encoding") != "gzip" {
					t.Errorf("expected gzip Content-Encoding on chat completion")
				}
				rawGZ, _ := io.ReadAll(req.Body)
				raw := gunzipBytes(t, rawGZ)
				var decoded map[string]any
				_ = json.Unmarshal(raw, &decoded)
				mu.Lock()
				chatAttempt++
				attempt := chatAttempt
				seenChatBodies = append(seenChatBodies, decoded)
				mu.Unlock()

				if attempt == 1 {
					return jsonResponse(400, map[string]any{
						"error": map[string]any{"message": "Illegal API invocation from an unapproved channel"},
					}, nil), nil
				}
				if attempt == 2 {
					return jsonResponse(400, map[string]any{
						"error": map[string]any{"message": "messages must end with a user message"},
					}, nil), nil
				}
				sse := strings.Join([]string{
					`data: {"id":"cmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"tc-1","function":{"name":"bad_tool","arguments":"{}"}}]}}]}`,
					"",
					`data: {"id":"cmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150,"prompt_tokens_details":{"cached_tokens":40}}}`,
					"",
					"data: [DONE]",
					"",
				}, "\n")
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": {"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader(sse)),
				}, nil
			default:
				return jsonResponse(404, map[string]any{"error": "unexpected path " + req.URL.Path}, nil), nil
			}
		}), func() {}, nil
	}

	mod := NewModule(ModuleOptions{
		Endpoints:   Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:         func() time.Time { return now },
		HTTPFactory: factory,
		SyncReports: true,
	})
	harness := newWBHarness(t, mod, nil)
	cred := sampleOAuthCred(ProviderAI, now.Add(time.Hour).UnixMilli())

	chatReq := map[string]any{
		"model":  "kimi-k3.1-pro",
		"stream": true,
		"messages": []any{
			map[string]any{"role": "system", "content": "You are an assistant powered by CustomHarness. DeepSeek Harness is great at /tmp/DeepSeek Harness/app."},
			map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "c0", "type": "function", "function": map[string]any{"name": "bad.tool", "arguments": "{}"}}}},
			map[string]any{"role": "tool", "tool_call_id": "c0", "content": "ok"},
			map[string]any{"role": "user", "content": "continue"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "bad.tool",
					"description": "dotted tool",
					"parameters":  map[string]any{"type": "object", "properties": map[string]any{"val": map[string]any{"type": []any{"string", "null"}}}},
				},
			},
		},
	}
	chatBody, _ := json.Marshal(chatReq)
	execFrame, execData := harness.invoke(sdk.MethodProviderExecute, sdk.Scope{
		Provider:   ProviderAI,
		Account:    "acct-100",
		Generation: 1,
	}, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "kimi-k3.1-pro",
		Body:     chatBody,
	}, &cred)
	if execFrame.Error != nil {
		t.Fatalf("execute failed: %v", execFrame.Error)
	}
	var execRes sdk.ExecuteResult
	_ = json.Unmarshal(execFrame.Result, &execRes)
	if execRes.Status != 200 {
		t.Fatalf("expected 200 after comma+replyAfterTools cascade, got %d body=%s", execRes.Status, string(execData))
	}
	if execRes.Usage == nil || execRes.Usage.InputTokens != 120 || execRes.Usage.CachedTokens != 40 || execRes.Usage.OutputTokens != 30 {
		t.Fatalf("unexpected usage: %+v", execRes.Usage)
	}
	bodyText := string(execData)
	if !strings.Contains(bodyText, `"bad.tool"`) {
		t.Fatalf("expected tool name restored to bad.tool in SSE, got: %s", bodyText)
	}

	filter := newUnsaidFilter()
	filtered := filter.push(
		`data: {"id":"f1","choices":[{"index":0,"delta":{"content":"Sorry, I can't respond "}}]}`+"\n\n"+
			`data: {"id":"f1","choices":[{"index":0,"delta":{"content":"to this question."},"finish_reason":"content_filter"}]}`+"\n\n",
	) + filter.flush()
	if strings.Contains(filtered, "Sorry, I can't respond") || !strings.Contains(filtered, `"content_filter"`) {
		t.Fatalf("unsaidFilter should strip canned refusal on content_filter while preserving finish_reason: %s", filtered)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenChatBodies) != 3 {
		t.Fatalf("expected 3 chat attempts (initial + commaPaused + replyAfterTools), got %d", len(seenChatBodies))
	}
	firstMsgs := seenChatBodies[0]["messages"].([]any)
	if firstMsgs[1].(map[string]any)["content"] != CallTextSpace {
		t.Fatalf("kimi-k3.1-pro call-only assistant turn should carry space, got %#v", firstMsgs[1])
	}
	if seenChatBodies[0]["reasoning_effort"] != "high" {
		t.Fatalf("expected default reasoning_effort=high, got %#v", seenChatBodies[0]["reasoning_effort"])
	}
	if seenChatBodies[0]["extra_vars"] == nil {
		t.Fatalf("expected WorkBuddy AI growthEvent in extra_vars")
	}
	secondSys := seenChatBodies[1]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(secondSys, "assistant, powered by CustomHarness") {
		t.Fatalf("expected commaPaused retry in attempt 2, got %q", secondSys)
	}
	thirdMsgs := seenChatBodies[2]["messages"].([]any)
	if len(thirdMsgs) < 2 || thirdMsgs[len(thirdMsgs)-2].(map[string]any)["role"] != "assistant" || thirdMsgs[len(thirdMsgs)-1].(map[string]any)["role"] != "user" {
		t.Fatalf("expected replyAfterTools inserted assistant turn before trailing user turn in attempt 3, got %#v", thirdMsgs)
	}
	if len(seenReports) == 0 {
		t.Fatalf("expected WorkBuddy AI /v2/report activity telemetry on clean EOF")
	}
}

func TestNativeMessagesBadSignatureAnd401ReissueAnd2MiBProbe(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	var msgAttempts atomic.Int32
	var refreshHits atomic.Int32
	var probeHits atomic.Int32
	var lastAuth atomic.Value

	factory := func(string) (Doer, func(), error) {
		return testDoer(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/v1/messages":
				lastAuth.Store(req.Header.Get("Authorization"))
				attempt := msgAttempts.Add(1)
				if attempt == 1 {
					return jsonResponse(401, map[string]any{"error": "expired"}, http.Header{
						"WWW-Authenticate": {`Bearer realm="copilot", error="invalid_token"`},
					}), nil
				}
				if attempt == 2 {
					return jsonResponse(400, map[string]any{
						"error": map[string]any{
							"code":          "400001",
							"internal_code": "11159",
							"message":       "Invalid `signature` in `thinking` block",
						},
					}, nil), nil
				}
				raw, _ := io.ReadAll(req.Body)
				if strings.Contains(string(raw), `"thinking"`) {
					t.Errorf("expected signed thinking block stripped on retry 3, got %s", string(raw))
				}
				return jsonResponse(200, map[string]any{
					"id":      "msg-1",
					"content": []any{map[string]any{"type": "text", "text": "hello"}},
					"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5},
				}, nil), nil
			case "/v2/plugin/auth/token/refresh":
				refreshHits.Add(1)
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"accessToken":  "wb-access-reissued",
						"refreshToken": "wb-refresh-reissued",
						"expiresIn":    3600,
					},
				}, nil), nil
			case "/v2/chat/completions":
				return nil, errors.New("connection reset by peer")
			case summaryURI:
				probeHits.Add(1)
				return jsonResponse(401, map[string]any{"code": 401, "msg": "unauthorized"}, http.Header{
					"WWW-Authenticate": {`Bearer realm="copilot", error="invalid_token"`},
				}), nil
			default:
				return jsonResponse(404, map[string]any{"error": req.URL.Path}, nil), nil
			}
		}), func() {}, nil
	}

	mod := NewModule(ModuleOptions{
		Endpoints:   Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:         func() time.Time { return now },
		HTTPFactory: factory,
	})
	harness := newWBHarness(t, mod, nil)
	cred := sampleOAuthCred(ProviderWorkBuddy, now.Add(time.Hour).UnixMilli())

	msgBody := `{"model":"claude-4.5-sonnet","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","signature":"other-vendor-sig"},{"type":"text","text":"prev"}]},{"role":"user","content":"go"}]}`
	frame, _ := harness.invoke(sdk.MethodProviderExecute, sdk.Scope{
		Provider:   ProviderWorkBuddy,
		Account:    "acct-100",
		Generation: 1,
	}, sdk.ExecuteRequest{
		Protocol: "messages",
		Model:    "claude-4.5-sonnet",
		Body:     []byte(msgBody),
	}, &cred)
	if frame.Error != nil {
		t.Fatalf("messages execute failed: %v", frame.Error)
	}
	var res sdk.ExecuteResult
	_ = json.Unmarshal(frame.Result, &res)
	if res.Status != 200 || res.Headers.Get("X-Magpie-Sign-In") != "renewed" {
		t.Fatalf("expected 200 renewed, got status=%d hdr=%v", res.Status, res.Headers)
	}
	if refreshHits.Load() != 1 || lastAuth.Load() != "Bearer wb-access-reissued" {
		t.Fatalf("expected one-shot refresh reissue, refreshHits=%d lastAuth=%v", refreshHits.Load(), lastAuth.Load())
	}

	owner := newOwner(ModuleOptions{
		Endpoints:   Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:         func() time.Time { return now },
		HTTPFactory: factory,
	})
	hugeBody := make([]byte, EdgeReadsBytes+128)
	synth, note, ok := owner.unansweredProbe(context.Background(), nil, owner.sites[ProviderWorkBuddy], cred, hugeBody, errors.New("ECONNRESET"))
	if !ok || synth == nil || synth.StatusCode != 401 || !strings.Contains(note, "over 2 MiB") || probeHits.Load() != 1 {
		t.Fatalf("expected 2 MiB ECONNRESET billing probe to yield synthetic 401, ok=%v note=%q hits=%d", ok, note, probeHits.Load())
	}
	_, _, smallOK := owner.unansweredProbe(context.Background(), nil, owner.sites[ProviderWorkBuddy], cred, make([]byte, EdgeReadsBytes), errors.New("ECONNRESET"))
	if smallOK {
		t.Fatalf("body <= 2 MiB must not trigger billing probe")
	}
}

func TestExplainErrorBodyCodes(t *testing.T) {
	body11140, hdr := explainErrorBody(403, http.Header{}, []byte(`{"code":11140,"msg":"request illegal"}`))
	if !strings.Contains(string(body11140), RestrictedHint) || hdr.Get("X-Magpie-Sign-In") != "kept" {
		t.Fatalf("11140 should include RestrictedHint: %s", string(body11140))
	}
	body111400, _ := explainErrorBody(403, http.Header{}, []byte(`{"code":111400,"msg":"other"}`))
	if strings.Contains(string(body111400), RestrictedHint) {
		t.Fatalf("111400 must not match whole code 11140: %s", string(body111400))
	}
	body14018, _ := explainErrorBody(403, http.Header{}, []byte(`{"code":"14018","msg":"no package"}`))
	if !strings.Contains(string(body14018), ExhaustedHint) {
		t.Fatalf("14018 should include ExhaustedHint: %s", string(body14018))
	}
	body140185, _ := explainErrorBody(403, http.Header{}, []byte(`{"code":"140185","msg":"other"}`))
	if strings.Contains(string(body140185), ExhaustedHint) {
		t.Fatalf("140185 must not match whole code 14018: %s", string(body140185))
	}
	bodyRefused, _ := explainErrorBody(400, http.Header{}, []byte(`{"error":{"message":"Illegal API invocation from an unapproved channel"}}`))
	if !strings.Contains(string(bodyRefused), RefusedDoc) || !strings.HasSuffix(extractErrMsg(t, bodyRefused), RefusedHint) {
		t.Fatalf("refused error should contain RefusedDoc before RefusedHint: %s", string(bodyRefused))
	}
}

func extractErrMsg(t *testing.T, raw []byte) string {
	t.Helper()
	var doc struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Error.Message
}

func TestCheckinCNOutcomes(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	var mode atomic.Int32
	var claimAttempts atomic.Int32

	factory := func(string) (Doer, func(), error) {
		return testDoer(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/v2/billing/meter/checkin-activity-status":
				switch mode.Load() {
				case 1:
					return jsonResponse(200, map[string]any{
						"code": 0,
						"data": map[string]any{"active": true, "today_checked_in": false},
					}, nil), nil
				case 2:
					return jsonResponse(200, map[string]any{
						"code": 0,
						"data": map[string]any{"active": true, "today_checked_in": true, "today_credit": 25},
					}, nil), nil
				case 3:
					return jsonResponse(200, map[string]any{
						"code": 0,
						"data": map[string]any{"active": true, "today_checked_in": false},
					}, nil), nil
				default:
					return jsonResponse(200, map[string]any{"code": 1001, "msg": "already"}, nil), nil
				}
			case "/v2/billing/meter/daily-checkin":
				if mode.Load() == 1 {
					if claimAttempts.Add(1) == 1 {
						return jsonResponse(503, map[string]any{"code": 10000, "msg": "busy"}, nil), nil
					}
					return jsonResponse(200, map[string]any{
						"code": 0,
						"data": map[string]any{"credit": 50, "streak_days": 4, "message": "claimed ok"},
					}, nil), nil
				}
				if mode.Load() == 3 {
					return jsonResponse(401, map[string]any{"code": 401, "msg": "own app required"}, nil), nil
				}
				return jsonResponse(200, map[string]any{"code": 1002, "msg": "not eligible"}, nil), nil
			case summaryURI:
				return jsonResponse(200, map[string]any{"code": 0, "data": map[string]any{"IsPaidUser": false}}, nil), nil
			default:
				return jsonResponse(404, map[string]any{"error": req.URL.Path}, nil), nil
			}
		}), func() {}, nil
	}

	mod := NewModule(ModuleOptions{
		Endpoints:    Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:          func() time.Time { return now },
		HTTPFactory:  factory,
		CheckinWaits: []time.Duration{time.Millisecond, time.Millisecond},
	})
	harness := newWBHarness(t, mod, nil)
	cred := sampleOAuthCred(ProviderWorkBuddy, now.Add(time.Hour).UnixMilli())

	invokeCheckin := func(provider string) sdk.Frame {
		fr, _ := harness.invoke(sdk.MethodModuleCall, sdk.Scope{
			Provider:   provider,
			Account:    "acct-100",
			Generation: 1,
		}, sdk.ModuleCallRequest{
			Action: "account.checkin",
			Input:  rawJSON(sdk.AccountCheckinRequest{Day: "2026-11-24"}),
		}, &cred)
		return fr
	}

	mode.Store(1)
	frame1 := invokeCheckin(ProviderWorkBuddy)
	if frame1.Error != nil {
		t.Fatalf("checkin mode 1 failed: %v", frame1.Error)
	}
	var res1 sdk.AccountCheckinResult
	_ = json.Unmarshal(frame1.Result, &res1)
	if res1.Outcome != "claimed" || res1.Credit == nil || *res1.Credit != 50 || res1.Streak == nil || *res1.Streak != 4 {
		t.Fatalf("unexpected claimed result: %+v", res1)
	}

	mode.Store(2)
	frame2 := invokeCheckin(ProviderWorkBuddy)
	var res2 sdk.AccountCheckinResult
	_ = json.Unmarshal(frame2.Result, &res2)
	if res2.Outcome != "done" || res2.Credit == nil || *res2.Credit != 25 {
		t.Fatalf("unexpected already-checked result: %+v", res2)
	}

	mode.Store(3)
	frame3 := invokeCheckin(ProviderWorkBuddy)
	var res3 sdk.AccountCheckinResult
	_ = json.Unmarshal(frame3.Result, &res3)
	if res3.Outcome != "ineligible" || !strings.Contains(res3.Message, "own app") {
		t.Fatalf("unexpected 401 own-app result: %+v", res3)
	}

	aiFrame := invokeCheckin(ProviderAI)
	var aiRes sdk.AccountCheckinResult
	_ = json.Unmarshal(aiFrame.Result, &aiRes)
	if aiRes.Outcome != "ineligible" {
		t.Fatalf("workbuddy-ai checkin must be ineligible: %+v", aiRes)
	}
}

func TestUsageSplitsEnterpriseAndC33FreeWindow(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	workDir := t.TempDir()
	routingDir := filepath.Join(workDir, "routing")
	if err := os.MkdirAll(routingDir, 0o700); err != nil {
		t.Fatal(err)
	}

	localDay := now.In(time.Local).Format("2006-01-02")
	refusalReset := "2026-11-25 18:30:00 UTC+8"
	routeEvent := map[string]any{
		"time": now.Add(-10 * time.Minute).Format(time.RFC3339),
		"order": []any{
			map[string]any{"provider": ProviderWorkBuddy, "id": "slot-1", "who": "acct-100", "kind": "account"},
		},
		"tries": []any{
			map[string]any{
				"id":     "slot-1",
				"model":  "deepseek-v4.1-flash-sg",
				"status": 429,
				"start":  now.Add(-10 * time.Minute).Format(time.RFC3339),
				"error":  "WorkBuddy: 您的使用量已超出频率限制，将在 " + refusalReset + " 重置，您也可以切换其他模型继续使用。",
				"rest": map[string]any{
					"key": "workbuddy@acct-100/deepseek-v4.1-flash-sg",
					"by":  "resets",
				},
			},
		},
	}
	routeLine, _ := json.Marshal(routeEvent)
	gzPath := filepath.Join(routingDir, localDay+".1.jsonl.gz")
	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	_, _ = gzWriter.Write(append(routeLine, '\n'))
	_ = gzWriter.Close()
	if err := os.WriteFile(gzPath, gzBuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	var enterpriseMode atomic.Bool
	factory := func(string) (Doer, func(), error) {
		return testDoer(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case summaryURI:
				if enterpriseMode.Load() {
					return jsonResponse(200, map[string]any{
						"code": 0,
						"data": map[string]any{"Packages": []any{}},
					}, nil), nil
				}
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"IsPaidUser":              true,
						"SubscriptionPackageCode": "TCACA_code_002_AkiJS3ZHF5",
						"CreditGroups": []any{
							map[string]any{"Kind": "plan_base", "CycleTotalCapacity": "500", "CycleUsedCapacity": "520"},
							map[string]any{"Kind": "addon", "CycleTotalCapacity": "200", "CycleUsedCapacity": "50"},
						},
						"Packages": []any{
							map[string]any{"PackageCode": "TCACA_code_002_AkiJS3ZHF5", "CycleTotalCapacity": "500", "CycleUsedCapacity": "520"},
							map[string]any{"PackageCode": "TCACA_code_009_0XmEQc2xOf", "CycleTotalCapacity": "200", "CycleUsedCapacity": "50"},
							map[string]any{"PackageCode": "prepaid_wallet", "CycleRemainCapacity": "42.5"},
						},
					},
				}, nil), nil
			case "/billing/meter/get-user-resource-paid-packages":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"TotalCount": 1,
						"Accounts": []any{
							map[string]any{
								"PackageCode":    "TCACA_code_002_AkiJS3ZHF5",
								"CycleStartTime": "2026-11-01 00:00:00",
								"CycleEndTime":   "2026-12-01 00:00:00",
							},
						},
					},
				}, nil), nil
			case "/billing/meter/get-user-resource-free-packages":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{"TotalCount": 0, "Accounts": []any{}},
				}, nil), nil
			case "/billing/meter/get-enterprise-user-usage":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"limitNum":       "1000",
						"credit":         "250",
						"cycleStartTime": "2026-11-01 00:00:00",
						"cycleResetTime": "2026-12-01 00:00:00",
					},
				}, nil), nil
			case "/v3/config":
				return jsonResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"agents": []any{
							map[string]any{"name": "cli", "models": []any{"deepseek-v4.1-flash"}},
						},
						"models": []any{
							map[string]any{"id": "deepseek-v4.1-flash", "credits": "0"},
						},
					},
				}, nil), nil
			default:
				return jsonResponse(404, map[string]any{"error": req.URL.Path}, nil), nil
			}
		}), func() {}, nil
	}

	mod := NewModule(ModuleOptions{
		Endpoints:   Endpoints{CN: "https://cn.test", AI: "https://ai.test"},
		Now:         func() time.Time { return now },
		HTTPFactory: factory,
		WorkDir:     func() string { return workDir },
		HomeDir:     func() string { return workDir },
		Env:         func(string) string { return "" },
	})
	harness := newWBHarness(t, mod, nil)
	cred := sampleOAuthCred(ProviderWorkBuddy, now.Add(time.Hour).UnixMilli())

	usageFrame, _ := harness.invoke(sdk.MethodProviderUsage, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.UsageRequest{Revision: "17"}, &cred)
	if usageFrame.Error != nil {
		t.Fatalf("usage failed: %v", usageFrame.Error)
	}
	validated, _, err := sdk.ValidateUsageResult(Manifest(), usageFrame.Result)
	if err != nil {
		t.Fatalf("sdk.ValidateUsageResult rejected usage result: %v\nraw=%s", err, string(usageFrame.Result))
	}
	if validated.Revision != "17" {
		t.Fatalf("WorkBuddy did not acknowledge usage revision 17: %q", validated.Revision)
	}
	byName := map[string]sdk.UsageWindow{}
	for _, win := range validated.Windows {
		byName[win.Name] = win
	}
	if !byName["Credits"].Aside || !byName["Extra credits"].Aside || byName["Total credits"].Aside {
		t.Fatalf("expected Credits/Extra aside and Total non-aside: %+v", validated.Windows)
	}
	if byName["Credits"].Used != 100 {
		t.Fatalf("expected clamped Used=100 for 520/500, got %v", byName["Credits"].Used)
	}
	if byName["Total credits"].ResetsAt == 0 || byName["Total credits"].Span <= 0 {
		t.Fatalf("expected cycle resetsAt/span on Total credits: %+v", byName["Total credits"])
	}
	if !slices.Contains(byName["Total credits"].NotModels, "deepseek-v4.1-flash") {
		t.Fatalf("expected spareFree NotModels on credits window: %+v", byName["Total credits"])
	}
	if byName["Prepaid credits"].Balance == nil || *byName["Prepaid credits"].Balance != 42.5 {
		t.Fatalf("expected Prepaid credits balance 42.5: %+v", byName["Prepaid credits"])
	}
	refusedWin, hasRefused := byName["deepseek-v4.1-flash tokens"]
	if !hasRefused {
		t.Fatalf("expected C33 alias refusal card even with empty/absent ledger; got windows: %+v", validated.Windows)
	}
	if refusedWin.Used != 100 || refusedWin.Amount != nil || refusedWin.Limit != 0 || refusedWin.Display != "" || refusedWin.Aside {
		t.Fatalf("unexpected C33 refused card fields: %+v", refusedWin)
	}
	if !slices.Equal(refusedWin.Models, []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-sg"}) {
		t.Fatalf("expected both primary and alias models on refused card: %+v", refusedWin.Models)
	}

	enterpriseMode.Store(true)
	entFrame, _ := harness.invoke(sdk.MethodProviderUsage, sdk.Scope{Provider: ProviderWorkBuddy}, sdk.UsageRequest{Revision: "18"}, &cred)
	entVal, _, err := sdk.ValidateUsageResult(Manifest(), entFrame.Result)
	if err != nil {
		t.Fatalf("enterprise usage validation failed: %v", err)
	}
	if entVal.Revision != "18" {
		t.Fatalf("enterprise Usage did not acknowledge revision 18: %q", entVal.Revision)
	}
	if len(entVal.Windows) != 1 || entVal.Windows[0].Display != "250 / 1000" {
		t.Fatalf("unexpected enterprise usage windows: %+v", entVal.Windows)
	}
}

func TestFreeWindowLedgerEstimateAndAmbiguousRouting(t *testing.T) {
	now := time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()
	dir := t.TempDir()
	ledgerPath := filepath.Join(dir, "usage.jsonl")
	lines := []string{
		fmt.Sprintf(`{"t":%q,"providerAccount":"acct-ai","provider":"workbuddy-ai","model":"deepseek-v4.1-flash@high","status":200,"in":1000000,"cache_read":19000000,"out":500000}`, now.Add(-2*time.Hour).Format(time.RFC3339)),
		fmt.Sprintf(`{"t":%q,"source":"reported","provider":"workbuddy-ai","model":"deepseek-v4.1-flash","status":200,"in":99999999}`, now.Add(-48*time.Hour).Format(time.RFC3339)),
		`{"t":"2026-11-24T14:59:00Z","providerAccount":"acct-ai","torn_json_without_newline`,
	}
	if err := os.WriteFile(ledgerPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	owner := newOwner(ModuleOptions{Now: func() time.Time { return now }})
	rows := owner.freeWindowRows("acct-ai", nowMs, dir, func(string) string { return "" }, "", freeWindowAI)
	win := freeWindowOf(rows, "acct-ai", nowMs, freeWindowAI)
	if win == nil {
		t.Fatalf("expected estimated AI free window")
	}
	if !win.Aside || win.Display != "20.5M / 100M" || win.Amount == nil || *win.Amount != 20500000 {
		t.Fatalf("unexpected AI free window estimate: %+v", win)
	}

	ambiguousTrace := `{"time":"2026-11-24T14:00:00Z","order":[{"provider":"workbuddy","id":"s1","who":"acct-a"},{"provider":"workbuddy","id":"s1","who":"acct-b"}],"tries":[{"id":"s1","model":"deepseek-v4.1-flash","status":429,"error":"will reset at 2026-11-25 14:00:00 UTC+8"}]}`
	if got := routingRefusals(cnEstimatedWindows[0], ambiguousTrace); len(got) != 0 {
		t.Fatalf("ambiguous order id must be skipped, got %+v", got)
	}

	mismatchTrace := `{"time":"2026-11-24T14:00:00Z","order":[{"provider":"workbuddy","id":"s1","who":"acct-a"}],"tries":[{"id":"s1","model":"deepseek-v4.1-flash","status":429,"error":"will reset at 2026-11-25 14:00:00 UTC+8","rest":{"key":"workbuddy@acct-b/deepseek-v4.1-flash","by":"resets"}}]}`
	if got := routingRefusals(cnEstimatedWindows[0], mismatchTrace); len(got) != 0 {
		t.Fatalf("order vs rest account mismatch must be skipped, got %+v", got)
	}
}
