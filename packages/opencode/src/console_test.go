package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func (harness *sdkHarness) action(name string, scope sdk.Scope, params any, auth json.RawMessage) (sdk.Frame, []byte) {
	return harness.invoke(sdk.MethodModuleCall, scope, sdk.ModuleCallRequest{Action: name, Input: raw(params)}, auth)
}

func newZenFixture(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	id, err := randomHex(16)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "/fixture-" + id
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, prefix+"/") {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		address := *request.URL
		address.Path = strings.TrimPrefix(address.Path, prefix)
		address.RawPath = strings.TrimPrefix(address.RawPath, prefix)
		request.URL = &address
		handler(writer, request)
	}))
	server.URL += prefix
	return server
}

type consoleFixture struct {
	Config       json.RawMessage `json:"config"`
	Instructions json.RawMessage `json:"instructions"`
	Skills       json.RawMessage `json:"skills"`
	Bundle       json.RawMessage `json:"bundle"`
	Poll         json.RawMessage `json:"poll"`
}

func loadConsole(t *testing.T) consoleFixture {
	t.Helper()
	body, err := os.ReadFile("testdata/donors/console.json")
	var fixture consoleFixture
	if err != nil || json.Unmarshal(body, &fixture) != nil {
		t.Fatal("invalid literal Console fixture", err)
	}
	return fixture
}

func zenScope(provider string, identity ZenCredentials) sdk.Scope {
	return sdk.Scope{Provider: provider, Account: identity.AccountID(provider), Generation: 1}
}

func TestConsoleHeadersCachesAndWorkspaceAuthority(t *testing.T) {
	fixture := loadConsole(t)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	var mu sync.Mutex
	config, instructions, skills := fixture.Config, fixture.Instructions, fixture.Skills
	configStatus, instructionStatus := 200, 200
	gate := true
	counts := map[string]int{}
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		counts[request.URL.Path]++
		if request.Header.Get("Authorization") != "Bearer PRIVATE-console" ||
			request.Header.Get("x-org-id") != "org-a" || request.Header.Get("x-opencode-org-id") != "" {
			t.Error("Console did not use its selected account and Console organization header")
		}
		switch request.URL.Path {
		case "/console/api/v2/config":
			if gate {
				writer.Header()["X-Opencode-Instructions-Skills"] = []string{""}
				writer.Header()["X-Opencode-Event-Watches"] = []string{""}
			}
			writer.WriteHeader(configStatus)
			writer.Write(config)
		case "/console/api/v2/instructions":
			writer.WriteHeader(instructionStatus)
			writer.Write(instructions)
		case "/console/api/v2/skills":
			writer.Write(skills)
		default:
			t.Error("unexpected Console route", request.URL.Path)
			writer.WriteHeader(500)
		}
	}))
	defer server.Close()
	account := credential(now(), "a", "org-a")
	account.Access = "PRIVATE-console"
	identity := ZenCredentials{Kind: "oauth", OAuth: &account}
	scope := zenScope(ProviderZen, identity)
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Now: now, Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global", ConsoleWatches: true}))
	snapshot := func(selected sdk.Scope) ConsoleSnapshot {
		t.Helper()
		frame, _ := harness.action("console.snapshot", selected, map[string]any{}, raw(identity))
		if frame.Error != nil {
			t.Fatalf("snapshot failed: %+v", frame.Error)
		}
		var value ConsoleSnapshot
		if json.Unmarshal(frame.Result, &value) != nil {
			t.Fatal("invalid snapshot")
		}
		return value
	}
	value := snapshot(scope)
	if !value.Lists || !value.Watches || len(value.Instructions) != 1 || len(value.Skills) != 1 || value.Stale {
		t.Fatalf("present empty-value headers failed to enable independent lists: %+v", value)
	}
	snapshot(scope)
	mu.Lock()
	if counts["/console/api/v2/config"] != 1 || counts["/console/api/v2/instructions"] != 1 || counts["/console/api/v2/skills"] != 1 {
		t.Error("fresh Console cache was refetched")
	}
	instructions, skills = json.RawMessage(`{"instructions":[{"id":"broken"}]}`), json.RawMessage(`{"skills":[{"slug":"deploy","version":"2"}]}`)
	mu.Unlock()
	clock.Add(int64(time.Minute))
	value = snapshot(scope)
	if !value.Stale || value.Instructions[0].Content != "Use tabs." || value.Skills[0].Version != "2" {
		t.Fatal("malformed instruction erased its last copy or blocked independent skill cache", value)
	}
	mu.Lock()
	instructions = json.RawMessage(`{"instructions":null}`)
	mu.Unlock()
	value = snapshot(scope)
	if !value.Stale || len(value.Instructions) != 1 {
		t.Fatal("null list was treated as an empty successful list")
	}
	mu.Lock()
	instructionStatus, instructions = 403, json.RawMessage(`{"_tag":"SsoRequired","orgId":"org-a","connectionId":"sso"}`)
	mu.Unlock()
	value = snapshot(scope)
	if !value.Stale || len(value.Instructions) != 1 {
		t.Fatal("SSO list failure erased the previous workspace list")
	}
	mu.Lock()
	instructionStatus, instructions = 403, json.RawMessage(`{"error":"forbidden"}`)
	mu.Unlock()
	value = snapshot(scope)
	if value.Stale || len(value.Instructions) != 0 {
		t.Fatal("non-SSO 403 did not clear its independent list")
	}
	mu.Lock()
	configStatus, config = 403, json.RawMessage(`{"_tag":"SsoRequired","orgId":"org-a","connectionId":"sso"}`)
	mu.Unlock()
	clock.Add(int64(time.Minute))
	value = snapshot(scope)
	if !value.Stale || value.Status != "needs_sso" || len(value.Policies) == 0 {
		t.Fatal("valid SSO did not preserve the same-workspace document")
	}
	scope.Generation++
	value = snapshot(scope)
	if value.Lists || len(value.Policies) != 0 || len(value.Skills) != 0 {
		t.Fatal("a new account generation inherited the previous workspace cache")
	}
	mu.Lock()
	configStatus, config = 200, fixture.Config
	gate = false
	mu.Unlock()
	value = snapshot(scope)
	if value.Lists || value.Watches || len(value.Skills) != 0 {
		t.Fatal("missing response header did not disable remote lists/watches")
	}
	mu.Lock()
	configStatus = 404
	mu.Unlock()
	clock.Add(int64(time.Minute))
	value = snapshot(scope)
	if len(value.Policies) != 0 || value.Stale {
		t.Fatal("config 404 did not clear the previous document")
	}
	public := ZenCredentials{Kind: "public", Key: "public"}
	frame, _ := harness.action("console.snapshot", zenScope(ProviderZenPublic, public), map[string]any{}, raw(public))
	if frame.Error == nil || frame.Error.Code != "permission_denied" {
		t.Fatal("public identity gained personal Console authority")
	}
}

func TestConsoleSkillRedirectAndStrictBundleBoundary(t *testing.T) {
	fixture := loadConsole(t)
	var original, remote *httptest.Server
	var crossed atomic.Int32
	remote = newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		crossed.Add(1)
		if request.URL.RawQuery != "sig=a%2Bb&x=%2F&x=1" || request.Header.Get("Authorization") != "" ||
			request.Header.Get("Cookie") != "" || request.Header.Get("x-org-id") != "" {
			t.Error("redirect changed the signed query or retained personal credentials")
		}
		http.Redirect(writer, request, original.URL+"/return", 307)
	}))
	defer remote.Close()
	original = newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/return" {
			if request.Header.Get("Authorization") != "" || request.Header.Get("x-org-id") != "" {
				t.Error("credentials were regained after returning to the original origin")
			}
			writer.Write(fixture.Bundle)
			return
		}
		if request.Header.Get("Authorization") != "Bearer PRIVATE-key" {
			t.Error("initial skill download lost selected credentials")
		}
		http.Redirect(writer, request, remote.URL+"/blob?sig=a%2Bb&x=%2F&x=1", 302)
	}))
	defer original.Close()
	owner := &module{options: ModuleOptions{Endpoints: Endpoints{Console: original.URL, Inference: original.URL}, Now: time.Now,
		HTTPFactory: func(string) (Doer, func(), error) {
			return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil, nil
		}}}
	identity := ZenCredentials{Kind: "key", Key: "PRIVATE-key"}
	module := NewWindowModule(owner.options)
	module.Actions["console.skill"] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		return owner.downloadSkill(ctx, stream, identity, "deploy")
	}
	harness := serveHarness(t, module, raw(moduleConfig{}))
	frame, _ := harness.action("console.skill", zenScope(ProviderZen, identity), map[string]any{"slug": "deploy"}, raw(identity))
	if frame.Error != nil || crossed.Load() != 1 || !strings.Contains(string(frame.Result), "SKILL.md") {
		t.Fatal("literal skill failed guarded cross-origin redirects", frame.Error)
	}
	for _, path := range []string{"../escape", "%2e%2e/escape", "/absolute", "sub\\escape", "file?x=1", "a//b", "a:b", "a/%252e%252e/b"} {
		if safeRelative(path) {
			t.Error("unsafe skill path accepted", path)
		}
	}
	for _, content := range []string{"---\nname: okay\ndescription: !unsafe\n---\n",
		"---\nname: true\ndescription: text\n---\n", "---\nname: 42\ndescription: text\n---\n",
		"---\nname: &alias okay\ndescription: text\n---\n", "---\nname: [array]\n---\n", "---\nname: 'unterminated\n---\n"} {
		if validSkillFrontmatter(content) {
			t.Error("unsupported or incomplete frontmatter accepted")
		}
	}
}

func TestConsoleWatchReceiptsDoNotGrantForeignOrRepeatedAcknowledgement(t *testing.T) {
	fixture := loadConsole(t)
	var acknowledgements atomic.Int32
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/console/api/v2/config":
			writer.Header().Set("x-opencode-event-watches", "")
			writer.Write(fixture.Config)
		case "/console/api/event-watches/events":
			if request.URL.Query().Get("wait") != "25" || request.URL.Query().Get("cursor") != "opaque+/== &" {
				t.Error("watch cursor or wait was changed")
			}
			writer.Write(fixture.Poll)
		case "/console/api/event-watches/ack":
			var payload struct {
				IDs []string `json:"ids"`
			}
			json.NewDecoder(request.Body).Decode(&payload)
			if len(payload.IDs) != 1 || payload.IDs[0] != "ev_1" {
				t.Error("acknowledged another event")
			}
			acknowledgements.Add(1)
			io.WriteString(writer, `{}`)
		default:
			t.Error("unknown watch route", request.URL.Path)
			writer.WriteHeader(500)
		}
	}))
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ConsoleWatches: true}))
	identity := ZenCredentials{Kind: "key", Key: "PRIVATE-watch"}
	scope := zenScope(ProviderZen, identity)
	frame, _ := harness.action("console.watch.poll", scope, map[string]any{"cursor": "opaque+/== &"}, raw(identity))
	var page ConsoleWatchPage
	if frame.Error != nil || json.Unmarshal(frame.Result, &page) != nil || len(page.Receipt) != 32 || len(page.Events) != 1 {
		t.Fatal("literal watch poll failed", frame.Error)
	}
	params := map[string]any{"receipt": page.Receipt, "ids": []string{"ev_1"}}
	foreign := scope
	foreign.Generation++
	frame, _ = harness.action("console.watch.ack", foreign, params, raw(identity))
	if frame.Error == nil || acknowledgements.Load() != 0 {
		t.Fatal("foreign generation consumed an acknowledgement receipt")
	}
	frame, _ = harness.action("console.watch.ack", scope, map[string]any{"receipt": page.Receipt, "ids": []string{"unseen"}}, raw(identity))
	if frame.Error == nil || acknowledgements.Load() != 0 {
		t.Fatal("unseen event consumed an acknowledgement receipt")
	}
	frame, _ = harness.action("console.watch.ack", scope, params, raw(identity))
	if frame.Error != nil || acknowledgements.Load() != 1 {
		t.Fatal("delivered event did not acknowledge once", frame.Error)
	}
	frame, _ = harness.action("console.watch.ack", scope, params, raw(identity))
	if frame.Error == nil || acknowledgements.Load() != 1 {
		t.Fatal("replayed receipt acknowledged twice")
	}
}

func TestConsoleSkillVersionFailureKeepsExplicitStaleCopy(t *testing.T) {
	fixture := loadConsole(t)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	var revision atomic.Int32
	revision.Store(1)
	var broken atomic.Bool
	var downloads atomic.Int32
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/console/api/v2/config":
			writer.Header().Set("x-opencode-instructions-skills", "")
			writer.Write(fixture.Config)
		case "/console/api/v2/instructions":
			writer.Write(fixture.Instructions)
		case "/console/api/v2/skills":
			if revision.Load() == 0 {
				writer.WriteHeader(404)
				return
			}
			json.NewEncoder(writer).Encode(map[string]any{"skills": []map[string]string{{"slug": "deploy", "version": strconv.Itoa(int(revision.Load()))}}})
		case "/console/api/v2/skills/deploy":
			downloads.Add(1)
			if broken.Load() {
				io.WriteString(writer, `{"slug":"deploy","files":[{"path":"../escape","content":"bad"}]}`)
			} else {
				writer.Write(fixture.Bundle)
			}
		default:
			t.Error("unexpected skill route", request.URL.Path)
			writer.WriteHeader(500)
		}
	}))
	defer server.Close()
	identity := ZenCredentials{Kind: "key", Key: "PRIVATE-bundle"}
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Now: func() time.Time { return time.Unix(0, clock.Load()) },
		Endpoints: Endpoints{Console: server.URL + "/console", Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) {
			return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil, nil
		}}), raw(moduleConfig{}))
	read := func() SkillBundle {
		t.Helper()
		frame, _ := harness.action("console.skill", zenScope(ProviderZen, identity), map[string]string{"slug": "deploy"}, raw(identity))
		var bundle SkillBundle
		if frame.Error != nil || json.Unmarshal(frame.Result, &bundle) != nil {
			t.Fatal("skill read failed", frame.Error)
		}
		return bundle
	}
	if read().Version != "1" || read().Stale || downloads.Load() != 1 {
		t.Fatal("skill version cache failed")
	}
	revision.Store(2)
	broken.Store(true)
	clock.Add(int64(time.Minute))
	bundle := read()
	if !bundle.Stale || bundle.Version != "1" || downloads.Load() != 2 {
		t.Fatal("failed skill update silently replaced the last good version")
	}
	broken.Store(false)
	bundle = read()
	if bundle.Stale || bundle.Version != "2" || downloads.Load() != 3 {
		t.Fatal("skill update did not retry the changed version")
	}
	revision.Store(0)
	clock.Add(int64(time.Minute))
	frame, _ := harness.action("console.skill", zenScope(ProviderZen, identity), map[string]string{"slug": "deploy"}, raw(identity))
	if frame.Error == nil || downloads.Load() != 3 {
		t.Fatal("unlisted skill remained available from its previous bundle")
	}
}

func TestZenFixtureRejectsForeignPathsBeforeVendorState(t *testing.T) {
	var routed atomic.Int32
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		routed.Add(1)
		if request.URL.Path == "/declared" {
			writer.WriteHeader(200)
			return
		}
		writer.WriteHeader(501)
	}))
	defer server.Close()
	root := strings.Split(server.URL, "/fixture-")[0]
	for _, path := range []string{"/", "/foreign-fixture/console/api/user"} {
		response, err := http.Get(root + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 || routed.Load() != 0 {
			t.Fatal("foreign request entered vendor state")
		}
	}
	for _, endpoint := range []struct {
		Path   string
		Status int
	}{{"/declared", 200}, {"/undeclared", 501}} {
		response, err := http.Get(server.URL + endpoint.Path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != endpoint.Status {
			t.Fatal("owned undeclared path was hidden by the fixture gate")
		}
	}
	if routed.Load() != 2 {
		t.Fatal("owned fixture requests were lost")
	}
}
