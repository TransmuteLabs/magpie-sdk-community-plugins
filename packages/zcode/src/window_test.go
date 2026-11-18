package zcode

import (
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestControlVariantRequiresScopedUIWithoutChangingHeadless(t *testing.T) {
	headless := Manifest()
	if headless.Contract.Version.Minor != 12 || headless.Window != nil || len(headless.Bindings) != 0 {
		t.Fatal("provider-only policy surface acquired the UI variant")
	}
	control := NewControlModule(ModuleOptions{})
	if err := control.Manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if control.Manifest.Contract.Version.Minor != 12 || control.Manifest.Compatibility.HostAPI != "^1.12" ||
		control.Manifest.Window == nil || control.Manifest.Window.Metadata != "package.json" ||
		len(control.Manifest.Bindings) != 15 || !control.Manifest.Contract.Has(sdk.AgentProcess) ||
		len(control.Manifest.Processes) != 1 || len(headless.Processes) != 0 || headless.Contract.Has(sdk.AgentProcess) {
		t.Fatal("control page has no explicit scoped UI floor or metadata")
	}
	for _, binding := range control.Manifest.Bindings {
		action, found := control.Manifest.ResolveAction(binding.Action)
		if binding.Provider != ProviderID || binding.From != "window:page" || !found ||
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
	if len(headless.AuthBindings) != 0 {
		t.Fatal("headless API4 acquired a browser authentication grant")
	}
	manifest := ControlManifest()
	if len(manifest.AuthBindings) != 1 {
		t.Fatal("control page has no host-owned authentication binding")
	}
	binding := manifest.AuthBindings[0]
	if binding.ID != "page.signin" || binding.From != "window:page" || binding.Provider != ProviderID ||
		!binding.Reauthorize || len(binding.Methods) != 1 || binding.Methods[0].ID != "browser" ||
		binding.Methods[0].Mode != "poll" {
		t.Fatal("authentication escaped its fixed provider/method/current-generation grant")
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}
