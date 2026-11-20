package opencode

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestWindowGrantKeepsCredentialsAndContinuationsOutOfUIResult(t *testing.T) {
	module := NewWindowModule(ModuleOptions{})
	if err := module.Validate(); err != nil {
		t.Fatal(err)
	}
	grant, found := module.Manifest.ResolveAuthBinding("signin")
	if module.Manifest.Contract.Version.Minor != 12 || !found || grant.From != "window:page" ||
		len(grant.Methods) != 1 || grant.Methods[0].ID != "device" || module.Manifest.Bindings[0].Provider != ProviderGo ||
		AgentManifest().Window != nil || Manifest().Window != nil {
		t.Fatal("window negotiation changed headless/agent variants")
	}
	credentials := Credentials{Access: "PRIVATE-ACCESS", Refresh: "PRIVATE-REFRESH", UserID: "user-a", OrgID: "org-a",
		InitialOrgID: "org-a", Email: "user@example.invalid", Expires: time.Now().Add(time.Hour)}
	call := sdk.Call{Scope: sdk.Scope{Plugin: "opencode", Provider: ProviderGo, Account: credentials.AccountID(ProviderGo), Generation: 7},
		Auth: raw(credentials), Params: json.RawMessage(`null`)}
	result, err := module.Actions["account.info"](t.Context(), call, nil)
	body, marshalErr := json.Marshal(result)
	if err != nil || marshalErr != nil || strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "credentials") ||
		strings.Contains(string(body), "example.invalid") {
		t.Fatal("account action leaked private auth material:", err)
	}
	var summary sdk.AccountSummary
	if json.Unmarshal(body, &summary) != nil || summary.Generation != "7" || summary.ID != call.Scope.Account {
		t.Fatal("account action changed host authority")
	}
	for _, mutate := range []func(*sdk.Call){
		func(call *sdk.Call) { call.Scope.Account = "foreign" },
		func(call *sdk.Call) { call.Scope.Generation = 0 },
		func(call *sdk.Call) { call.Scope.Provider = "foreign" },
		func(call *sdk.Call) { call.Auth = nil },
		func(call *sdk.Call) { call.Params = raw(map[string]string{"account": "foreign"}) },
	} {
		invalid := call
		mutate(&invalid)
		if _, err := module.Actions["account.info"](context.Background(), invalid, nil); err == nil {
			t.Fatal("window action accepted supplied business authority")
		}
	}
}
