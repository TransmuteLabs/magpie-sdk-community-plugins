package zcode

import (
	"slices"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestControlVariantRequiresScopedUIWithoutChangingHeadless(t *testing.T) {
	headless := Manifest()
	if headless.Contract.Version.Minor != 20 || headless.Window != nil || len(headless.Bindings) != 0 {
		t.Fatal("provider-only composition lost its SDK floor or acquired UI bindings")
	}
	control := NewControlModule(ModuleOptions{})
	if err := control.Manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if control.Manifest.Contract.Version.Minor != 20 || control.Manifest.Compatibility.HostAPI != "^1.20" ||
		control.Manifest.Window == nil || control.Manifest.Window.Metadata != "package.json" ||
		len(control.Manifest.Bindings) != 15 || !control.Manifest.Contract.Has(sdk.AgentProcess) ||
		len(control.Manifest.Processes) != 1 || len(headless.Processes) != 0 || headless.Contract.Has(sdk.AgentProcess) {
		t.Fatal("control page has no explicit scoped UI floor or metadata")
	}
	for _, binding := range control.Manifest.Bindings {
		action, found := control.Manifest.ResolveAction(binding.Action)
		if binding.Provider != ProviderID || binding.From != "window:panel:0" || !found ||
			control.Actions[binding.Action] == nil || binding.Service != "" {
			t.Fatal("page binding escaped its fixed provider/action owner")
		}
		write := binding.Action == "billing.claim" || binding.Action == "billing.checkin" ||
			binding.Action == "reset.use" || binding.Action == "reset.resolve" || binding.Action == "reset.reconcile" ||
			binding.Action == "reset.opportunity" || binding.Action == "reset.opportunity.resolve" || binding.Action == "reset.ack" ||
			binding.Action == "native.run" || binding.Action == "native.stop"
		if (action.Effect == "write") != write {
			t.Fatal("financial action effect was weakened")
		}
	}
	control.Manifest.Contract.Version = sdk.APIVersion{Major: 1, Minor: 4}
	if control.Manifest.Validate() == nil {
		t.Fatal("selected-account page admitted on SDK 1.4")
	}
}

func TestControlExecutionSummaryCannotExposeTicketCardOrMutateSource(t *testing.T) {
	source := []executionView{{Reference: "ref", State: "ready", Ticket: "private-ticket", Card: "private-card"}}
	summary := summarizeExecutions(source)
	if len(summary) != 1 || summary[0].Reference != "ref" || summary[0].Ticket != "" || summary[0].Card != "" ||
		source[0].Ticket != "private-ticket" || source[0].Card != "private-card" {
		t.Fatal("control page leaked ticket/card authority or mutated controller views")
	}
}

func TestControlAuthenticationUsesHostOwnedFixedProviderPolling(t *testing.T) {
	headless := Manifest()
	if len(headless.AuthBindings) != 0 || headless.Contract.Has(sdk.ProviderSavedSignIn) {
		t.Fatal("headless composition acquired the full package's sign-in contract")
	}
	manifest := ControlManifest()
	if len(manifest.AuthBindings) != 1 || !manifest.Contract.Has(sdk.ProviderSavedSignIn) {
		t.Fatal("control composition has no host-owned saved sign-in contract")
	}
	binding := manifest.AuthBindings[0]
	if binding.ID != "provider.signin" || binding.From != sdk.ProviderSignInPart || binding.Provider != ProviderID ||
		!binding.Reauthorize || len(binding.Methods) != 3 || binding.Methods[0].ID != "browser" ||
		binding.Methods[0].Mode != "poll" || len(binding.Methods[0].Fields) != 6 {
		t.Fatal("authentication escaped its fixed provider/method/current-generation grant")
	}
	keyMethod := binding.Methods[1]
	if keyMethod.ID != apiKeySignIn || keyMethod.Mode != "poll" || len(keyMethod.Fields) != 2 ||
		keyMethod.Fields[1].Key != "key" || keyMethod.Fields[1].Type != "secret" || keyMethod.Fields[1].Optional {
		t.Fatal("GLM Coding Plan API key sign-in does not use a required secret field")
	}
	importMethod := binding.Methods[2]
	if importMethod.ID != sdk.SavedSignInMethod || importMethod.Mode != "poll" || len(importMethod.Fields) != 1 ||
		importMethod.Fields[0].Key != "record" || importMethod.Fields[0].Type != "secret" || importMethod.Fields[0].Optional {
		t.Fatal("saved sign-in does not use the ordinary provider import contract")
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	manifest.Contract.Capabilities = slices.DeleteFunc(manifest.Contract.Capabilities, func(capability sdk.Capability) bool {
		return capability == sdk.ProviderSavedSignIn
	})
	if manifest.Validate() == nil {
		t.Fatal("saved sign-in was admitted without host support for preparation and activation")
	}
}

func TestAPIKeySignInKeepsSelectedKeyAndSite(t *testing.T) {
	h := newHarness(t, ModuleOptions{})
	selected := fixtureCredentials("legacy-key")
	for _, input := range []map[string]string{
		{"key": "different-key"},
		{"key": selected.Key, "site": "bigmodel"},
	} {
		frame, _ := h.call(sdk.MethodAuthStart, scope(selected),
			sdk.AuthStartRequest{Method: apiKeySignIn, Input: raw(input)}, raw(selected))
		if frame.Error == nil || frame.Error.Outcome != "not-started" || len(frame.Result) != 0 ||
			h.proxies != 0 || len(h.state) != 0 {
			t.Fatal("reauthorization crossed the selected key or site")
		}
	}
}
