package commandcode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestProviderSignInMethods(t *testing.T) {
	module := Module()
	if err := module.Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := module.Manifest
	grant, found := manifest.ResolveAuthBinding("provider.signin")
	if !found || !manifest.Contract.Has(sdk.ProviderSignIn) || grant.From != sdk.ProviderSignInPart || grant.Provider != ProviderID || !grant.Reauthorize {
		t.Fatal("Command Code is missing its ordinary Providers sign-in and reauthorization")
	}
	if len(manifest.AuthBindings) != 1 || len(grant.Methods) != 3 {
		t.Fatal("Command Code must expose all three methods through one ordinary entry")
	}
	for _, id := range []string{"api-key", "browser", "cli-import"} {
		index := slices.IndexFunc(grant.Methods, func(method sdk.WindowAuthMethod) bool { return method.ID == id })
		if index < 0 {
			t.Fatalf("missing provider method %q", id)
		}
		method := grant.Methods[index]
		if method.Label == "" || method.Mode != "poll" {
			t.Fatalf("missing provider method %q", id)
		}
		if id == "api-key" {
			if len(method.Fields) != 1 || method.Fields[0].Key != "apiKey" || method.Fields[0].Type != "secret" || method.Fields[0].Optional {
				t.Fatal("ordinary API-key sign-in must request exactly the required secret")
			}
		} else if len(method.Fields) != 0 {
			t.Fatalf("unexpected inputs for method %q", id)
		}
	}
}

func TestSelectedAPIKeyReauthenticationAndRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/alpha/whoami" {
			t.Errorf("unexpected sign-in request: %s", request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		switch request.Header.Get("x-api-key") {
		case "original-key":
			io.WriteString(writer, `{"user":{"id":"same-user","userName":"Original name"}}`)
		case "replacement-key":
			io.WriteString(writer, `{"user":{"id":"same-user","email":"new-name@example.invalid"}}`)
		case "other-user-key":
			io.WriteString(writer, `{"user":{"id":"other-user","userName":"Original name"}}`)
		case "unnamed-key":
			io.WriteString(writer, `{"name":"  ","user":{"id":"same-user"}}`)
		case "revoked-key":
			writer.WriteHeader(http.StatusUnauthorized)
		case "forbidden-key":
			writer.WriteHeader(http.StatusForbidden)
		default:
			writer.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	worker := fixture(t, Config{APIURL: server.URL})
	finish := func(scope sdk.Scope, inputKey string, previous json.RawMessage) (sdk.AuthResult, error) {
		t.Helper()
		input, _ := json.Marshal(map[string]string{"apiKey": inputKey})
		raw, _, err := worker.callScoped(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "api-key", Input: input}, previous)
		var started sdk.AuthStartResult
		if err != nil || json.Unmarshal(raw, &started) != nil || len(started.Continuation) == 0 {
			t.Fatalf("start: %s %v", raw, err)
		}
		raw, _, err = worker.callScoped(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: started.Continuation}, previous)
		var result sdk.AuthResult
		if err == nil && json.Unmarshal(raw, &result) != nil {
			t.Fatalf("unreadable auth result: %s", raw)
		}
		return result, err
	}
	fresh := sdk.Scope{Plugin: ModuleID, Provider: ProviderID}
	first, err := finish(fresh, "original-key", nil)
	if err != nil || first.ID == "" || first.Name != "Original name" {
		t.Fatalf("fresh account did not receive its published name: %q %v", first.Name, err)
	}
	fallback, err := finish(fresh, "unnamed-key", nil)
	if err != nil || fallback.Name != "Command Code [3eaa2bf9bb5f]" {
		t.Fatalf("unnamed profile did not receive a recognizable account label: %q %v", fallback.Name, err)
	}
	legacyScope := fresh
	legacyScope.Account, legacyScope.Generation = fallback.ID, 1
	legacy, _ := json.Marshal(credentials{Key: "unnamed-key", UserID: "same-user", Name: "same-user"})
	legacyRaw, _, err := worker.callScoped(sdk.MethodAuthRefresh, legacyScope, sdk.AuthRefreshRequest{}, legacy)
	var legacyResult sdk.AuthResult
	if err != nil || json.Unmarshal(legacyRaw, &legacyResult) != nil || legacyResult.ID != fallback.ID || legacyResult.Name != fallback.Name {
		t.Fatalf("refresh did not repair the old raw label: %q %v", legacyResult.Name, err)
	}
	selected := fresh
	selected.Account, selected.Generation = first.ID, 1
	replacement, err := finish(selected, "replacement-key", first.Credentials)
	if err != nil || replacement.ID != first.ID || replacement.Name != "new-name@example.invalid" {
		t.Fatalf("same user could not renew selected account %q: %q %v", first.ID, replacement.ID, err)
	}
	if _, err := finish(selected, "other-user-key", first.Credentials); err == nil {
		t.Fatal("another account with the same display name replaced the selected user")
	}
	fallback, err = finish(selected, "unnamed-key", first.Credentials)
	if err != nil || fallback.ID != first.ID || fallback.Name != first.Name {
		t.Fatalf("reauthorization erased an existing name when the vendor omitted it: %q %v", fallback.Name, err)
	}
	selected.Generation++
	raw, _, err := worker.callScoped(sdk.MethodAuthRefresh, selected, sdk.AuthRefreshRequest{}, replacement.Credentials)
	var refreshed sdk.AuthResult
	if err != nil || json.Unmarshal(raw, &refreshed) != nil || refreshed.Name != replacement.Name || refreshed.ID != first.ID {
		t.Fatalf("refresh lost the selected ID or published name: %q %q %v", refreshed.ID, refreshed.Name, err)
	}
	for _, item := range []struct{ key, code string }{
		{"revoked-key", "unauthenticated"},
		{"forbidden-key", "unauthenticated"},
		{"busy-key", "unavailable"},
	} {
		previous, _ := json.Marshal(credentials{Key: item.key, UserID: "same-user"})
		_, _, err := worker.callScoped(sdk.MethodAuthRefresh, selected, sdk.AuthRefreshRequest{}, previous)
		var refusal *sdk.Error
		if !errors.As(err, &refusal) || refusal.Code != item.code {
			t.Fatalf("refresh must distinguish %s: %v", item.code, err)
		}
	}
}
