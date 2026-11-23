package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	extensionhost "github.com/yetone/magpie/internal/extension"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/requestpolicy"
	"github.com/yetone/magpie/internal/settings"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestZCodeInstalledTrialGroupAccountIsolation(t *testing.T) {
	worker := os.Getenv("MAGPIE_ZCODE_GROUP_FIXTURE_WORKER")
	if !filepath.IsAbs(worker) {
		t.Fatal("set MAGPIE_ZCODE_GROUP_FIXTURE_WORKER to the exact fixture worker")
	}
	fresh(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("MAGPIE_BUN", filepath.Join(t.TempDir(), "no-bun"))
	t.Setenv("MAGPIE_NO_ACCOUNTS", "")
	t.Setenv("MAGPIE_ADDR", "")
	oldAllowances := allowances
	allowances = provider.Allowances
	t.Cleanup(func() { allowances = oldAllowances })
	opts := settings.Load()
	zero := 0
	opts.QuotaReads = "asked"
	opts.RequestPolicy = &requestpolicy.Config{Defaults: requestpolicy.Options{MaxTotalRetries: &zero}}
	if err := settings.Save(opts); err != nil {
		t.Fatal(err)
	}

	vendor := &zcodeAccountGroupVendor{ends: time.Now().Add(time.Hour), reads: map[string]int{}}
	upstream := httptest.NewServer(vendor)
	t.Cleanup(upstream.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	providerID, users := installZCodeAccountGroup(t, ctx, worker, upstream.URL)
	installed, err := provider.ExtensionProviders()
	if err != nil {
		t.Fatal(err)
	}
	models := []struct {
		native, alias string
		free          bool
	}{
		{"GLM-5.3", "paid-alias", false},
		{"GLM-5.3-Trial", "trial-alias", true},
		{"glm-limited-preview-Trial", "preview-alias", true},
	}
	members := map[string]map[string]string{}
	for _, label := range []string{"a", "b"} {
		user := users[label]
		index := slices.IndexFunc(installed, func(entry provider.Provider) bool {
			return entry.ID == providerID && entry.Account != nil && entry.Account.User == user
		})
		if index < 0 {
			t.Fatalf("no installed account %s", label)
		}
		discovered, err := installed[index].Fetch(ctx)
		if err != nil {
			t.Fatalf("fetch account %s: %v", label, err)
		}
		for _, model := range models {
			if !slices.ContainsFunc(discovered, func(entry catalog.Model) bool {
				return entry.ID == model.native && entry.Free == model.free
			}) {
				t.Fatalf("account %s does not list %s (free=%v)", label, model.native, model.free)
			}
		}
		accountRef, err := provider.ResolveAccount(providerID, user)
		if err != nil {
			t.Fatal(err)
		}
		accountID := strings.TrimPrefix(accountRef, providerID+"/")
		members[label] = map[string]string{}
		for _, model := range models {
			members[label][model.alias] = provider.WithMemberAccount(providerID+"/"+model.alias, accountID)
		}
		if err := provider.SetWindowCaps(providerID, user, []string{"5 hours", "Reserve · Start Plan"}, 90); err != nil {
			t.Fatal(err)
		}
	}
	for _, model := range models {
		if _, changed, err := provider.SetModelID(providerID+"/"+model.native, model.alias); err != nil || !changed {
			t.Fatalf("rename %s: changed=%v err=%v", model.native, changed, err)
		}
	}
	vendor.mu.Lock()
	clear(vendor.reads)
	vendor.mu.Unlock()
	if !provider.ReadUsageNow(ctx, providerID, users["a"], users["b"]) {
		t.Fatal("could not refresh installed ZCode allowances")
	}
	readings := provider.Allowances(providerID)
	for _, label := range []string{"a", "b"} {
		vendor.mu.Lock()
		paidReads, reserveReads := vendor.reads["paid/"+label], vendor.reads["reserve/"+label]
		vendor.mu.Unlock()
		if paidReads < 1 || reserveReads < 1 {
			t.Fatalf("account %s quota request counts: paid=%d reserve=%d", label, paidReads, reserveReads)
		}
		allowance := readings[strings.ToLower(users[label])]
		for _, model := range models {
			want := float64(25)
			if !model.free {
				want = 20
				if label == "b" {
					want = 100
				}
			}
			if used, _ := allowance.For(model.native, time.Now()); used != want {
				t.Fatalf("account %s model %s ordinary allowance=%v, want %v", label, model.native, used, want)
			}
		}
	}

	server := New()
	t.Cleanup(server.idle.stop)
	handler := server.Front()
	first, second := members["a"], members["b"]
	for _, scenario := range []struct {
		name    string
		members []string
		status  int
		want    []zcodeAccountGroupAttempt
	}{
		{"a-trial-held", []string{first["trial-alias"]}, 429, nil},
		{"trial-falls-through-to-b", []string{first["trial-alias"], second["trial-alias"]}, 200,
			[]zcodeAccountGroupAttempt{{"/start", "GLM-5.3", "Bearer fixture-trial-b", ""}}},
		{"a-preview-beside-held-trial", []string{first["trial-alias"], first["preview-alias"], second["trial-alias"]}, 200,
			[]zcodeAccountGroupAttempt{{"/start", "glm-limited-preview", "Bearer fixture-trial-a", ""}}},
		{"paid-falls-through-to-a", []string{second["paid-alias"], first["paid-alias"], second["trial-alias"]}, 200,
			[]zcodeAccountGroupAttempt{{"/zai", "GLM-5.3", "Bearer fixture-paid-a", "fixture-paid-a"}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if err := provider.SaveGroup(provider.Group{ID: scenario.name, Name: scenario.name,
				Routing: provider.Ordered, Members: scenario.members}); err != nil {
				t.Fatal(err)
			}
			vendor.mu.Lock()
			before := len(vendor.attempts)
			vendor.mu.Unlock()
			payload := `{"model":"group/` + scenario.name + `","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`
			request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/messages", strings.NewReader(payload))
			request.RemoteAddr = "127.0.0.1:12345"
			request.Header.Set("Content-Type", "application/json")
			reply := httptest.NewRecorder()
			handler.ServeHTTP(reply, request)
			vendor.mu.Lock()
			attempts := slices.Clone(vendor.attempts[before:])
			vendor.mu.Unlock()
			if reply.Code != scenario.status || !slices.Equal(attempts, scenario.want) {
				t.Fatalf("group wire: status=%d attempts=%v, want status=%d attempts=%v; body=%s",
					reply.Code, attempts, scenario.status, scenario.want, reply.Body.String())
			}
			if (scenario.status == 200 && !strings.Contains(reply.Body.String(), "zcode account fixture output")) ||
				(scenario.status == 429 && !strings.Contains(reply.Body.String(), "usage cap reached")) {
				t.Fatalf("ordinary group reply: %d %s", reply.Code, reply.Body.String())
			}
			if len(attempts) == 0 {
				t.Logf("group status=%d vendor sends=0", reply.Code)
			} else {
				t.Logf("group status=%d vendor sends=1 path=%s wire model=%s credentials matched",
					reply.Code, attempts[0].path, attempts[0].model)
			}
		})
	}
	vendor.mu.Lock()
	defer vendor.mu.Unlock()
	if len(vendor.unexpected) != 0 {
		t.Fatalf("unexpected local vendor calls: %v", vendor.unexpected)
	}
	if len(vendor.attempts) != 3 {
		t.Fatalf("all groups sent %d inference requests, want 3", len(vendor.attempts))
	}
}

func installZCodeAccountGroup(t *testing.T, ctx context.Context, worker, origin string) (string, map[string]string) {
	t.Helper()
	raw, err := exec.CommandContext(ctx, worker, origin, "fixture").Output()
	if err != nil {
		t.Fatalf("prepare ZCode fixture manifest: %v", err)
	}
	var fixture struct {
		Manifest sdk.Manifest
		Accounts []struct {
			Label string
			ID    string
			Auth  json.RawMessage
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Manifest.ID != "zcode" || fixture.Manifest.Runtime != "exec" || fixture.Manifest.Entry != "worker" ||
		!slices.Equal(fixture.Manifest.Args, []string{origin}) || len(fixture.Accounts) != 2 {
		t.Fatal("unexpected ZCode fixture identity")
	}
	directory := t.TempDir()
	binary, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, fixture.Manifest.Entry), binary, 0700); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(fixture.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, sdk.ManifestFile), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	manager := extensionhost.Default()
	t.Cleanup(func() { closeSDKGatewayHook(t, manager) })
	if _, err := manager.Install(ctx, directory); err != nil {
		t.Fatal(err)
	}
	config, err := manager.ModuleConfig(ctx, fixture.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CompareAndSwapModuleConfig(ctx, fixture.Manifest.ID, config.Version, json.RawMessage(`{"freeFirst":false}`)); err != nil {
		t.Fatal(err)
	}
	users := make(map[string]string, len(fixture.Accounts))
	for _, account := range fixture.Accounts {
		if account.ID == "" || len(account.Auth) == 0 || (account.Label != "a" && account.Label != "b") || users[account.Label] != "" {
			t.Fatal("invalid ZCode fixture account")
		}
		if _, err := manager.Store().SaveAccount(extensionhost.Account{Plugin: fixture.Manifest.ID,
			Provider: "zcode", ID: account.ID, Name: "ZCode group fixture " + account.Label, Auth: account.Auth}, 0); err != nil {
			t.Fatal(err)
		}
		users[account.Label] = account.ID
	}
	if users["a"] == users["b"] {
		t.Fatal("fixture must use distinct stable account IDs")
	}
	provider.Changed()
	return provider.ExtensionProviderID(fixture.Manifest.ID, "zcode"), users
}

type zcodeAccountGroupAttempt struct {
	path          string
	model         string
	authorization string
	apiKey        string
}

type zcodeAccountGroupVendor struct {
	mu         sync.Mutex
	ends       time.Time
	reads      map[string]int
	attempts   []zcodeAccountGroupAttempt
	unexpected []string
}

func (vendor *zcodeAccountGroupVendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vendor.mu.Lock()
	defer vendor.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	auth := r.Header.Get("Authorization")
	paid, isPaid := strings.CutPrefix(auth, "fixture-paid-")
	trial, isTrial := strings.CutPrefix(auth, "Bearer fixture-trial-")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/client/configs":
		io.WriteString(w, `{"code":0,"data":{"configs":{"builtin_provider_config_json":"http://`+r.Host+`/public-config"}}}`)
	case r.Method == http.MethodGet && r.URL.Path == "/public-config":
		io.WriteString(w, `{"config":{"providerConfigRules":{"providerRules":[
			{"providerId":"account:zai-individual-coding-plan","config":{"builtinModelIds":["GLM-5.3"]}},
			{"providerId":"account:zai-start-plan","config":{"builtinModelIds":["GLM-5.3","glm-limited-preview"]}}
			]},"modelConfigRules":{"modelRules":[{"modelMatch":".*","config":{"enabled":true,"properties":{"contextWindow":256000}}}]}}}`)
	case r.Method == http.MethodGet && r.URL.Path == "/api/biz/subscription/list" && isPaid && (paid == "a" || paid == "b"):
		io.WriteString(w, `{"code":200,"data":[{"status":"VALID","inCurrentPeriod":true,"productName":"GLM Coding Pro"}]}`)
	case r.Method == http.MethodGet && r.URL.Path == "/quota/zai" && isPaid && (paid == "a" || paid == "b"):
		vendor.reads["paid/"+paid]++
		used := 20
		if paid == "b" {
			used = 100
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"limits": []any{
			map[string]any{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": used, "nextResetTime": vendor.ends.UnixMilli()},
		}}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/zcode-plan/billing/balance" &&
		isTrial && (trial == "a" || trial == "b") && r.Header.Get("X-Device-Mid") == "fixture-device-"+trial:
		vendor.reads["reserve/"+trial]++
		remaining := 0
		if trial == "b" {
			remaining = 10
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"plans": []any{map[string]any{"plan_id": "gift", "user_plan_id": "owned", "status": "active", "ends_at": vendor.ends.Unix()}},
			"balances": []any{
				map[string]any{"plan_id": "gift", "user_plan_id": "owned", "total_units": 100, "remaining_units": 75,
					"capabilities": []string{"model:GLM-5.3", "model:glm-limited-preview"}},
				map[string]any{"plan_id": "gift", "user_plan_id": "owned", "show_name": "Reserve", "total_units": 25, "remaining_units": remaining,
					"capabilities": []string{"model:GLM-5.3"}},
			},
		}})
	case r.Method == http.MethodPost && (r.URL.Path == "/start" || r.URL.Path == "/zai"):
		var request struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			vendor.unexpected = append(vendor.unexpected, "invalid inference body")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		vendor.attempts = append(vendor.attempts, zcodeAccountGroupAttempt{r.URL.Path, request.Model, auth, r.Header.Get("X-Api-Key")})
		json.NewEncoder(w).Encode(map[string]any{"type": "message", "id": "zcode-account-fixture", "role": "assistant",
			"model": request.Model, "content": []any{map[string]string{"type": "text", "text": "zcode account fixture output"}},
			"stop_reason": "end_turn", "usage": map[string]int{"input_tokens": 1, "output_tokens": 1}})
	default:
		vendor.unexpected = append(vendor.unexpected, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}
