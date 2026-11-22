package minimax

import (
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestProviderFirstAccountDeviceBindings(test *testing.T) {
	configured := NewModule(ModuleOptions{})
	manifest := configured.Manifest
	if manifest.Window != nil || len(manifest.Agents) != 0 || !manifest.Contract.Has(sdk.ProviderSignIn) {
		test.Fatal("MiniMax should provide ordinary subscription sign-in without a sidebar or agent")
	}
	if err := configured.Validate(); err != nil {
		test.Fatal(err)
	}
	if len(manifest.AuthBindings) != 2 {
		test.Fatal("both MiniMax regions require explicit GUI authentication bindings")
	}
	for _, provider := range []string{China, Global} {
		binding, found := manifest.ResolveAuthBinding("provider.signin." + provider)
		if !found || binding.Provider != provider || binding.From != sdk.ProviderSignInPart || !binding.Reauthorize {
			test.Fatal("GUI binding changed provider or host-owned authentication authority", provider)
		}
		if len(binding.Methods) != 1 || binding.Methods[0].ID != "device" || binding.Methods[0].Mode != "poll" ||
			binding.Methods[0].Label == "" || len(binding.Methods[0].Fields) != 0 {
			test.Fatal("MiniMax GUI must use its existing device poll workflow", provider)
		}
	}
}
