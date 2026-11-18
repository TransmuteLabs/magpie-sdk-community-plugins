package minimax

import (
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestWindowFirstAccountDeviceBindings(test *testing.T) {
	configured := NewModule(ModuleOptions{})
	manifest := configured.Manifest
	if manifest.Window == nil || manifest.Window.Metadata != "package.json" {
		test.Fatal("first-account GUI entry is missing its declared package page")
	}
	if err := configured.Validate(); err != nil {
		test.Fatal(err)
	}
	if len(manifest.AuthBindings) != 2 {
		test.Fatal("both MiniMax regions require explicit GUI authentication bindings")
	}
	for _, provider := range []string{China, Global} {
		binding, found := manifest.ResolveAuthBinding("page.auth." + provider)
		if !found || binding.Provider != provider || binding.From != "window:page" || !binding.Reauthorize {
			test.Fatal("GUI binding changed provider or host-owned authentication authority", provider)
		}
		if len(binding.Methods) != 1 || binding.Methods[0] != (sdk.WindowAuthMethod{ID: "device", Mode: "poll"}) {
			test.Fatal("MiniMax GUI must use its existing device poll workflow", provider)
		}
	}
}
