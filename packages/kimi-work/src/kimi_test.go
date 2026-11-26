package kimiwork

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

func makeResponse(status int, body string, contentType string, extra http.Header) *http.Response {
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

type storedEntry struct {
	version int
	body    json.RawMessage
}

type harness struct {
	t         *testing.T
	conn      net.Conn
	enc       *json.Encoder
	dec       *json.Decoder
	seq       int
	mu        sync.Mutex
	state     map[string]storedEntry
	events    []sdk.Frame
	callbacks []sdk.Frame
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

func newHarness(t *testing.T, mod sdk.Module) *harness {
	t.Helper()
	if err := mod.Validate(); err != nil {
		t.Fatalf("module validation failed: %v", err)
	}
	host, worker := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- sdk.Serve(ctx, worker, worker, mod) }()
	fixture := &harness{
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
			t.Error("Kimi Work SDK fixture did not stop")
		}
	})
	frame, _ := fixture.invoke(sdk.MethodComponentConfigure, sdk.Scope{}, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent,
		Instance:  "kimi-test",
		Config:    sdk.ConfigSnapshot{Version: "1", Values: json.RawMessage(`{}`)},
	}, nil)
	if frame.Error != nil {
		t.Fatalf("configure failed: %v", frame.Error)
	}
	return fixture
}

func (fixture *harness) invoke(method string, scope sdk.Scope, params any, creds *Credentials) (sdk.Frame, []byte) {
	fixture.t.Helper()
	fixture.seq++
	id := fmt.Sprintf("call-%d", fixture.seq)
	scope.Plugin = ModuleID
	scope.Operation = id
	req := sdk.Frame{Kind: "call", ID: id, Method: method, Scope: scope, Params: rawJSON(params)}
	if creds != nil {
		req.Auth = rawJSON(creds)
	}
	fixture.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := fixture.enc.Encode(req); err != nil {
		fixture.t.Fatal(err)
	}
	var data []byte
	for {
		var frame sdk.Frame
		if err := fixture.dec.Decode(&frame); err != nil {
			fixture.t.Fatal(err)
		}
		switch frame.Kind {
		case sdk.KindHostCall:
			if frame.Scope != scope || frame.Parent != id || len(frame.Auth) != 0 {
				fixture.t.Fatalf("host callback crossed scope: %+v", frame)
			}
			fixture.callbacks = append(fixture.callbacks, frame)
			var result any
			switch frame.Method {
			case sdk.MethodNetworkProxy:
				result = sdk.NetworkProxyResult{URL: "http://proxy.invalid:8080"}
			case sdk.MethodStateGet:
				var getReq sdk.StateGetRequest
				_ = json.Unmarshal(frame.Params, &getReq)
				fixture.mu.Lock()
				stored := fixture.state[scope.Provider+"\x00"+scope.Account+"\x00"+getReq.Key]
				fixture.mu.Unlock()
				result = sdk.StateValue{Found: stored.version != 0, Version: fmt.Sprint(stored.version), Value: stored.body}
			case sdk.MethodStateCAS:
				var casReq sdk.StateCASRequest
				_ = json.Unmarshal(frame.Params, &casReq)
				fixture.mu.Lock()
				key := scope.Provider + "\x00" + scope.Account + "\x00" + casReq.Key
				stored := fixture.state[key]
				swapped := casReq.Version == fmt.Sprint(stored.version) || (casReq.Version == "" && stored.version == 0)
				if swapped {
					stored.version++
					stored.body = append(json.RawMessage(nil), casReq.Value...)
					fixture.state[key] = stored
				}
				ver := fmt.Sprint(stored.version)
				fixture.mu.Unlock()
				result = sdk.StateCASResult{Swapped: swapped, Version: ver}
			default:
				fixture.t.Fatalf("unexpected host callback: %s", frame.Method)
			}
			if err := fixture.enc.Encode(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: id, Result: rawJSON(result)}); err != nil {
				fixture.t.Fatal(err)
			}
		case sdk.KindEvent:
			fixture.events = append(fixture.events, frame)
			if frame.Event == "data" {
				var chunk []byte
				if err := json.Unmarshal(frame.Data, &chunk); err != nil {
					fixture.t.Fatal(err)
				}
				data = append(data, chunk...)
			}
			if err := fixture.enc.Encode(sdk.Frame{Kind: sdk.KindAck, ID: id, Seq: frame.Seq}); err != nil {
				fixture.t.Fatal(err)
			}
		case sdk.KindResult, "error":
			return frame, data
		default:
			fixture.t.Fatalf("unexpected frame kind: %s", frame.Kind)
		}
	}
}

func TestManifestAndDesktopPaths(t *testing.T) {
	mod := NewModule(ModuleOptions{})
	if err := mod.Validate(); err != nil {
		t.Fatalf("manifest validate: %v", err)
	}
	if mod.Manifest.ID != ModuleID || len(mod.Manifest.Providers) != 1 || mod.Manifest.Providers[0].ID != ProviderID {
		t.Fatalf("unexpected manifest: %+v", mod.Manifest)
	}

	if got := UserDataDir(DesktopPathOptions{Home: "/Users/alice", Platform: "darwin"}); got != "/Users/alice/Library/Application Support/kimi-desktop" {
		t.Fatalf("darwin userDataDir = %q", got)
	}
	if got := UserDataDir(DesktopPathOptions{Home: "/home/alice", Platform: "linux"}); got != "/home/alice/.config/kimi-desktop" {
		t.Fatalf("linux userDataDir = %q", got)
	}
	if got := UserDataDir(DesktopPathOptions{Home: `C:\Users\alice`, Platform: "win32", AppData: `C:\Users\alice\AppData\Roaming`}); got != `C:\Users\alice\AppData\Roaming\kimi-desktop` {
		t.Fatalf("win32 userDataDir = %q", got)
	}

	bom := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"version":1,"shareDir":"/custom/daimon"}`)...)
	gotShare := ShareDir(DesktopPathOptions{
		Home:     "/home/alice",
		Platform: "linux",
		Read:     func(string) ([]byte, error) { return bom, nil },
	})
	if gotShare != "/custom/daimon" {
		t.Fatalf("BOM shareDir = %q", gotShare)
	}
	relShare := ShareDir(DesktopPathOptions{
		Home:     "/home/alice",
		Platform: "linux",
		Read:     func(string) ([]byte, error) { return []byte(`{"version":1,"shareDir":"relative/dir"}`), nil },
	})
	if relShare != "/home/alice/.config/kimi-desktop/daimon-share" {
		t.Fatalf("relative pointer should fall back, got %q", relShare)
	}
}

func TestReadDesktopKeysAndAwayHandling(t *testing.T) {
	dir := t.TempDir()
	share := filepath.Join(dir, "daimon-share")
	cfgDir := filepath.Join(share, "daimon")
	devDir := filepath.Join(cfgDir, "runtime", "kimi-code", "home")
	if err := os.MkdirAll(devDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "device_id"), []byte("dev-abc-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uidA := AccountUID("user-a")
	uidB := AccountUID("user-b")
	keysJSON := `{"v":2,"keys":[{"userId":"user-a","apiKey":"work-key-a"},{"userId":"user-b","apiKey":"work-key-b"}]}`
	if err := os.WriteFile(filepath.Join(cfgDir, "kimi-code-key.json"), []byte(keysJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	webToken := makeJWT(map[string]any{"sub": "user-a", "exp": time.Now().Add(time.Hour).Unix()})
	cfgContent := fmt.Sprintf(`{"credentials":{"kimiCode":{"baseUrl":"https://agent-gw.kimi.com/coding/v1","apiKey":"work-key-a"},"kimiWeb":{"userId":"user-a","accessToken":%q,"refreshToken":"ref-a"}}}`, webToken)
	cfgPath := filepath.Join(cfgDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := readDesktop(cfgPath, DefaultBase)
	if err != nil {
		t.Fatalf("readDesktop: %v", err)
	}
	if session.UID != uidA || session.WorkToken != "work-key-a" || session.DeviceID != "dev-abc-123" || session.Keys[uidB] != "work-key-b" {
		t.Fatalf("unexpected desktop session: %+v", session)
	}

	if err := os.RemoveAll(share); err != nil {
		t.Fatal(err)
	}
	_, err = readDesktop(cfgPath, DefaultBase)
	deskErr, ok := err.(*DesktopError)
	if !ok || !deskErr.ShareMissing || deskErr.SignIn != "kept" {
		t.Fatalf("expected shareMissing kept error, got %#v", err)
	}
}

func TestMoonshotToolsAndFastSpeed(t *testing.T) {
	clean := []byte(`{"model":"k3-agent","messages":[{"role":"user","content":"hi"}],"thinking":{"type":"enabled","effort":"max"},"tools":[{"type":"function","function":{"name":"ping","parameters":{"type":"object","properties":{"x":{"type":"string"}}}}}]}`)
	out := MoonshotTools(clean)
	if !bytes.Equal(out, clean) {
		t.Fatalf("clean payload should stay byte-identical:\ngot:  %s\nwant: %s", out, clean)
	}

	dirty := []byte(`{"model":"k3-agent","thinking":{"type":"enabled","effort":"max"},"messages":[{"role":"assistant","content":"step","reasoning_content":"trace"}],"tools":[{"type":"function","function":{"name":"run","parameters":{"type":"object","$defs":{"Mode":{"enum":["a","b"]},"Obj":{"properties":{"x":{"type":"integer"}}}},"properties":{"mode":{"type":"string","$ref":"#/$defs/Mode"},"target":{"$ref":"#/$defs/Obj"},"choice":{"type":"string","anyOf":[{"type":"string"},{"type":"null"}]}}}}}]}`)
	rewritten := MoonshotTools(dirty)
	var parsed map[string]any
	if err := json.Unmarshal(rewritten, &parsed); err != nil {
		t.Fatal(err)
	}
	thinking := parsed["thinking"].(map[string]any)
	if thinking["effort"] != "max" {
		t.Fatalf("thinking lost: %+v", thinking)
	}
	msgs := parsed["messages"].([]any)
	if msgs[0].(map[string]any)["reasoning_content"] != "trace" {
		t.Fatalf("reasoning_content lost: %+v", msgs)
	}
	params := parsed["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)
	if _, hasRef := props["mode"].(map[string]any)["$ref"]; hasRef || props["mode"].(map[string]any)["type"] != "string" {
		t.Fatalf("expected inlined $ref beside type, got %+v", props["mode"])
	}
	if _, hasType := props["choice"].(map[string]any)["type"]; hasType {
		t.Fatalf("expected type stripped beside anyOf, got %+v", props["choice"])
	}
	defs := params["$defs"].(map[string]any)
	if defs["Mode"].(map[string]any)["type"] != "string" {
		t.Fatalf("expected enum target inferred type=string, got %+v", defs["Mode"])
	}
	if defs["Obj"].(map[string]any)["type"] != "object" {
		t.Fatalf("expected $ref target inferred type=object, got %+v", defs["Obj"])
	}

	offered := map[string]string{"k3-agent": "k3-agent", "k3-agent-fast": "k3-agent"}
	fastBody, isFast := FastSpeed([]byte(`{"model":"k3-agent-fast","messages":[]}`), offered)
	if !isFast {
		t.Fatal("expected synthetic -fast to activate fast speed")
	}
	var fb map[string]any
	_ = json.Unmarshal(fastBody, &fb)
	if fb["model"] != "k3-agent" {
		t.Fatalf("expected stripped model k3-agent, got %v", fb["model"])
	}
	tierBody, isTierFast := FastSpeed([]byte(`{"model":"k3-agent","service_tier":"priority","messages":[]}`), offered)
	if !isTierFast {
		t.Fatal("expected priority tier to activate fast speed")
	}
	_ = json.Unmarshal(tierBody, &fb)
	if _, hasTier := fb["service_tier"]; hasTier {
		t.Fatalf("expected service_tier stripped, got %+v", fb)
	}
	unknownBody, isUnknownFast := FastSpeed([]byte(`{"model":"custom-fast","messages":[]}`), offered)
	if isUnknownFast || string(unknownBody) != `{"model":"custom-fast","messages":[]}` {
		t.Fatalf("unknown -fast model should remain untouched")
	}
}

func TestAuthRefreshSingleflightAndSavedSignIn(t *testing.T) {
	now := atomic.Int64{}
	now.Store(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC).UnixMilli())
	uid := AccountUID("user-1")
	expSoon := time.UnixMilli(now.Load()).Add(30 * time.Second).Unix()
	expLater := time.UnixMilli(now.Load()).Add(2 * time.Hour).Unix()
	oldAccess := makeJWT(map[string]any{"sub": "user-1", "exp": expSoon})
	newAccess := makeJWT(map[string]any{"sub": "user-1", "exp": expLater})
	wrongSubAccess := makeJWT(map[string]any{"sub": "other-user", "exp": expLater})

	var refreshCalls atomic.Int32
	var returnStatus atomic.Int32
	returnStatus.Store(http.StatusOK)
	var returnWrongSub atomic.Bool

	mod := NewModule(ModuleOptions{
		Now: func() time.Time { return time.UnixMilli(now.Load()) },
		ReadSession: func() (DesktopSession, error) {
			return DesktopSession{
				WorkToken:  "work-1",
				WebToken:   oldAccess,
				WebRefresh: "ref-1",
				DeviceID:   "dev-1",
				UID:        uid,
				Keys:       map[string]string{uid: "work-1"},
			}, nil
		},
		HTTPFactory: func(proxy string) (Doer, func(), error) {
			if proxy != "http://proxy.invalid:8080" {
				return nil, nil, fmt.Errorf("missing proxy")
			}
			return testDoer(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == RefreshURL {
					refreshCalls.Add(1)
					if st := int(returnStatus.Load()); st != http.StatusOK {
						return makeResponse(st, `{"error":"denied"}`, "application/json", nil), nil
					}
					tok := newAccess
					if returnWrongSub.Load() {
						tok = wrongSubAccess
					}
					return makeResponse(http.StatusOK, fmt.Sprintf(`{"access_token":%q,"refresh_token":"ref-2"}`, tok), "application/json", nil), nil
				}
				if req.URL.String() == DefaultBase+"/models" {
					return makeResponse(http.StatusOK, `{"data":[{"id":"k3-agent","display_name":"K3"}]}`, "application/json", nil), nil
				}
				return makeResponse(http.StatusNotFound, `{}`, "application/json", nil), nil
			}), func() {}, nil
		},
	})
	fixture := newHarness(t, mod)

	startRes, _ := fixture.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderID}, sdk.AuthStartRequest{Method: "desktop"}, nil)
	if startRes.Error != nil {
		t.Fatalf("auth start error: %v", startRes.Error)
	}
	var started sdk.AuthStartResult
	_ = json.Unmarshal(startRes.Result, &started)
	finishRes, _ := fixture.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderID}, sdk.AuthFinishRequest{Continuation: started.Continuation}, nil)
	if finishRes.Error != nil {
		t.Fatalf("auth finish error: %v", finishRes.Error)
	}
	var authRes sdk.AuthResult
	_ = json.Unmarshal(finishRes.Result, &authRes)
	if authRes.ID != uid {
		t.Fatalf("unexpected account id: %s", authRes.ID)
	}
	var creds Credentials
	_ = json.Unmarshal(authRes.Credentials, &creds)

	// Refresh rotates token and remembers replacement for same input pair.
	refRes1, _ := fixture.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderID, Account: uid}, sdk.AuthRefreshRequest{}, &creds)
	if refRes1.Error != nil {
		t.Fatalf("refresh 1 failed: %v", refRes1.Error)
	}
	refRes2, _ := fixture.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderID, Account: uid}, sdk.AuthRefreshRequest{}, &creds)
	if refRes2.Error != nil {
		t.Fatalf("refresh 2 failed: %v", refRes2.Error)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("expected 1 upstream refresh call for repeated stale pair, got %d", refreshCalls.Load())
	}

	// Subject mismatch rejected on fresh pair.
	returnWrongSub.Store(true)
	credsMismatch := creds
	credsMismatch.Refresh = "ref-mismatch"
	badRes, _ := fixture.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderID, Account: uid}, sdk.AuthRefreshRequest{}, &credsMismatch)
	if badRes.Error == nil {
		t.Fatal("expected sub mismatch rejection")
	}

	// 401 returns kimi_work_membership_expired (kept semantics).
	returnWrongSub.Store(false)
	returnStatus.Store(http.StatusUnauthorized)
	creds401 := creds
	creds401.Refresh = "ref-401"
	expRes, _ := fixture.invoke(sdk.MethodAuthRefresh, sdk.Scope{Provider: ProviderID, Account: uid}, sdk.AuthRefreshRequest{}, &creds401)
	if expRes.Error == nil || expRes.Error.Code != "kimi_work_membership_expired" {
		t.Fatalf("expected kimi_work_membership_expired, got %+v", expRes.Error)
	}

	// Saved sign-in import flow.
	savedRecord, _ := json.Marshal(map[string]any{"source": SourceTag, "uid": uid, "accountId": "Kimi Work · saved"})
	sStart, _ := fixture.invoke(sdk.MethodAuthStart, sdk.Scope{Provider: ProviderID}, sdk.AuthStartRequest{
		Method: sdk.SavedSignInMethod,
		Input:  rawJSON(map[string]string{"record": string(savedRecord)}),
	}, nil)
	if sStart.Error != nil {
		t.Fatalf("saved start: %v", sStart.Error)
	}
	var savedStarted sdk.AuthStartResult
	_ = json.Unmarshal(sStart.Result, &savedStarted)
	sFinish, _ := fixture.invoke(sdk.MethodAuthFinish, sdk.Scope{Provider: ProviderID}, sdk.AuthFinishRequest{Continuation: savedStarted.Continuation}, nil)
	if sFinish.Error != nil {
		t.Fatalf("saved finish: %v", sFinish.Error)
	}
}

func TestModelsExecuteRefusalAndUsage(t *testing.T) {
	uid := AccountUID("user-exec")
	otherUID := AccountUID("user-away")
	creds := Credentials{Source: SourceTag, UID: uid, AccountID: "Kimi Work · exec"}
	awayCreds := Credentials{Source: SourceTag, UID: otherUID, AccountID: "Kimi Work · away"}

	var lastSpeed string
	var lastAuth string
	var lastMagpie string
	var executeStatus atomic.Int32
	executeStatus.Store(http.StatusOK)
	var executeBody atomic.Value
	executeBody.Store("data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"id\":\"1\",\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\ndata: [DONE]\n\n")
	var executeContentType atomic.Value
	executeContentType.Store("text/event-stream")
	var usageBody atomic.Value
	usageBody.Store(`{"subscriptionBalance":{"kimiCodeUsedRatio":0.125}}`)
	var usageStatus atomic.Int32
	usageStatus.Store(http.StatusOK)

	mod := NewModule(ModuleOptions{
		ReadSession: func() (DesktopSession, error) {
			return DesktopSession{
				WorkToken: "work-tok",
				WebToken:  "web-tok",
				DeviceID:  "dev-99",
				UID:       uid,
				Keys:      map[string]string{uid: "work-tok", otherUID: "away-work-tok"},
			}, nil
		},
		HTTPFactory: func(proxy string) (Doer, func(), error) {
			return testDoer(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.URL.String() == ConfigURL:
					return makeResponse(http.StatusOK, `{"models":[{"requestParams":{"modelId":"k3-agent"},"display":{"displayName":"K3 Desktop"},"daimonModelConfig":{"capabilities":["thinking","image_in"]},"speedOptions":[{"speedValue":"fast","available":true}]}]}`, "application/json", nil), nil
				case req.URL.String() == DefaultBase+"/models":
					return makeResponse(http.StatusOK, `{"data":[{"id":"k3-agent","display_name":"K3 GW","context_length":200000,"supports_reasoning":true,"supports_image_in":true,"reasoningEffortOptions":[{"effort":"REASONING_EFFORT_HIGH"}]}]}`, "application/json", nil), nil
				case req.URL.String() == DefaultBase+"/chat/completions":
					lastSpeed = req.Header.Get(SpeedHeader)
					lastAuth = req.Header.Get("Authorization")
					lastMagpie = req.Header.Get("X-Magpie-Secret")
					extra := http.Header{}
					if executeStatus.Load() == http.StatusForbidden {
						extra.Set("Retry-After", "45")
					}
					return makeResponse(int(executeStatus.Load()), executeBody.Load().(string), executeContentType.Load().(string), extra), nil
				case strings.HasSuffix(req.URL.String(), "/GetSubscriptionStats"):
					return makeResponse(int(usageStatus.Load()), usageBody.Load().(string), "application/json", nil), nil
				default:
					return makeResponse(http.StatusNotFound, `{}`, "application/json", nil), nil
				}
			}), func() {}, nil
		},
	})
	fixture := newHarness(t, mod)

	// Models merge: desktop name + priority tier + synthetic -fast + gateway context/efforts.
	modelsRes, _ := fixture.invoke(sdk.MethodProviderModels, sdk.Scope{Provider: ProviderID, Account: uid}, sdk.ModelsRequest{}, &creds)
	if modelsRes.Error != nil {
		t.Fatalf("models failed: %v", modelsRes.Error)
	}
	var listed sdk.ModelsResult
	_ = json.Unmarshal(modelsRes.Result, &listed)
	if len(listed.Models) != 2 {
		t.Fatalf("expected 2 models (base + -fast), got %+v", listed.Models)
	}
	if listed.Models[0].Name != "K3 Desktop" || listed.Models[0].ContextWindow != 200000 || !slices.Contains(listed.Models[0].ServiceTiers, "priority") || !slices.Contains(listed.Models[0].Efforts, "high") {
		t.Fatalf("unexpected merged base model: %+v", listed.Models[0])
	}
	if listed.Models[1].ID != "k3-agent-fast" {
		t.Fatalf("expected synthetic k3-agent-fast, got %+v", listed.Models[1])
	}

	// Execute fast SSE with header stripping and usage reporting.
	execScope := sdk.Scope{Provider: ProviderID, Account: uid, Generation: 1}
	execRes, rawStream := fixture.invoke(sdk.MethodProviderExecute, execScope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "k3-agent-fast",
		Headers:  http.Header{"Authorization": {"Bearer evil"}, "X-Magpie-Secret": {"drop"}},
		Body:     []byte(`{"model":"k3-agent-fast","stream":true,"messages":[{"role":"user","content":"hi"}]}`),
	}, &creds)
	if execRes.Error != nil {
		t.Fatalf("execute failed: %v", execRes.Error)
	}
	if lastSpeed != "fast" || lastAuth != "Bearer work-tok" || lastMagpie != "" {
		t.Fatalf("unexpected outbound headers: speed=%q auth=%q magpie=%q", lastSpeed, lastAuth, lastMagpie)
	}
	if !strings.Contains(string(rawStream), "[DONE]") {
		t.Fatalf("expected SSE body, got %q", string(rawStream))
	}
	var executed sdk.ExecuteResult
	_ = json.Unmarshal(execRes.Result, &executed)
	if executed.Usage == nil || executed.Usage.InputTokens != 10 || executed.Usage.OutputTokens != 5 || executed.Usage.CachedTokens != 3 {
		t.Fatalf("unexpected usage: %+v", executed.Usage)
	}

	// Execute 403 spentQuota converts to RefusedExecution 429 with Retry-After.
	executeStatus.Store(http.StatusForbidden)
	executeContentType.Store("application/json")
	executeBody.Store(`{"error":{"type":"resource_exhausted","message":"Monthly quota exceeded"}}`)
	refusedRes, _ := fixture.invoke(sdk.MethodProviderExecute, execScope, sdk.ExecuteRequest{
		Protocol: "chat",
		Model:    "k3-agent",
		Body:     []byte(`{"model":"k3-agent","messages":[{"role":"user","content":"hi"}]}`),
	}, &creds)
	if refusedRes.Error != nil {
		t.Fatalf("expected RefusedExecution result, got rpc error: %v", refusedRes.Error)
	}
	var refused sdk.ExecuteResult
	_ = json.Unmarshal(refusedRes.Result, &refused)
	if refused.Outcome != "not-started" || refused.Status != http.StatusTooManyRequests || refused.Refusal == nil || refused.Refusal.Facts.Code != "kimi_work_quota_exhausted" || refused.Refusal.Facts.Headers["Retry-After"] != "45" {
		t.Fatalf("unexpected refusal: %+v", refused)
	}

	// Usage: missing amountUsedRatio defaults to 0%, kimiCodeUsedRatio displayed.
	usageRes, _ := fixture.invoke(sdk.MethodProviderUsage, sdk.Scope{Provider: ProviderID, Account: uid}, sdk.UsageRequest{Revision: "17"}, &creds)
	if usageRes.Error != nil {
		t.Fatalf("usage rpc error: %v", usageRes.Error)
	}
	var usage sdk.UsageResult
	_ = json.Unmarshal(usageRes.Result, &usage)
	if usage.Revision != "17" {
		t.Fatalf("Kimi Work did not acknowledge usage revision 17: %q", usage.Revision)
	}
	if len(usage.Windows) != 1 || usage.Windows[0].Used != 0 || usage.Windows[0].Display != "Code 已用 12.5%（占总额）" {
		t.Fatalf("unexpected usage result: %+v", usage)
	}

	// Usage away account returns non-fatal error without losing Work key.
	awayUsageRes, _ := fixture.invoke(sdk.MethodProviderUsage, sdk.Scope{Provider: ProviderID, Account: otherUID}, sdk.UsageRequest{Revision: "18"}, &awayCreds)
	if awayUsageRes.Error != nil {
		t.Fatalf("away usage rpc error: %v", awayUsageRes.Error)
	}
	var awayUsage sdk.UsageResult
	_ = json.Unmarshal(awayUsageRes.Result, &awayUsage)
	if awayUsage.Revision != "18" {
		t.Fatalf("away account did not acknowledge usage revision 18: %q", awayUsage.Revision)
	}
	if awayUsage.Error == "" || len(awayUsage.Windows) != 0 {
		t.Fatalf("expected away error, got %+v", awayUsage)
	}
}
