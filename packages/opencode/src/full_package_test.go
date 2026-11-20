package opencode

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDefaultFullPackageDeclarationMatchesNativeFactory(t *testing.T) {
	declaration, err := os.ReadFile("magpie-plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest := WindowManifest()
	suffix := map[string]string{"darwin": "dylib", "windows": "dll"}[runtime.GOOS]
	if suffix == "" {
		suffix = "so"
	}
	if manifest.Entry != "lib/opencode."+suffix {
		t.Fatalf("native factory entry differs from platform: %s", manifest.Entry)
	}
	manifest.Entry = "lib/opencode.so"
	factory, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected any
	if err := json.Unmarshal(declaration, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(factory, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("ordinary package declaration differs from native factory wire metadata")
	}
}

func TestDefaultFullPackageDeclaresAndBindsHistory(t *testing.T) {
	module := NewWindowModule(ModuleOptions{})
	if err := module.Validate(); err != nil {
		t.Fatal("ordinary full package is invalid", err)
	}
	manifest := module.Manifest
	if manifest.Version != "0.8.2" || manifest.Contract.Version != (sdk.APIVersion{Major: 1, Minor: 12}) ||
		manifest.Compatibility.HostAPI != "^1.12" {
		t.Fatal("ordinary full package did not negotiate effective agent metadata")
	}
	for _, capability := range []sdk.Capability{sdk.AccountAuth, sdk.ProviderSignIn, sdk.ModelDiscovery, sdk.OutboundExecutor,
		sdk.UsageReporter, sdk.AgentWiring, sdk.ModuleRPC, sdk.StorageSource, sdk.RequestPolicy} {
		if !manifest.Contract.Has(capability) {
			t.Fatal("ordinary full package omitted a ready capability", capability)
		}
	}
	if !reflect.DeepEqual(manifest.StorageSources, []sdk.StorageSourceDescriptor{HistoryDescriptor()}) ||
		len(manifest.Providers) != 3 || len(manifest.Agents) != 1 || manifest.Window == nil ||
		len(manifest.AuthBindings) != 6 || len(manifest.Services) != 0 {
		t.Fatal("ordinary full package omitted a ready component or required a companion")
	}
	for _, method := range []string{sdk.MethodStorageList, sdk.MethodStorageRead} {
		if module.Handlers[method] == nil {
			t.Fatal("ordinary full package requires an opt-in history constructor", method)
		}
	}
	older := manifest
	older.Contract.Version.Minor = 7
	older.Compatibility = &sdk.Compatibility{HostAPI: "^1.7", NativeABI: 1}
	if sdk.ValidateStorageSources(older) == nil {
		t.Fatal("storage package was admitted under an older public contract")
	}
	for _, historical := range []sdk.Manifest{Manifest(), AgentManifest()} {
		if historical.Contract.Has(sdk.StorageSource) || len(historical.StorageSources) != 0 ||
			historical.Contract.Has(sdk.ProviderSignIn) || len(historical.AuthBindings) != 0 {
			t.Fatal("historical compatibility artifact silently changed its contract")
		}
	}
	historical := NewAPI8WindowModule(ModuleOptions{})
	if err := historical.Validate(); err != nil {
		t.Fatal("historical complete package is invalid", err)
	}
	if historical.Manifest.Version != "0.8.1" || historical.Manifest.Contract.Version.Minor != 12 ||
		historical.Manifest.Compatibility.HostAPI != "^1.12" || historical.Manifest.Window == nil ||
		len(historical.Manifest.StorageSources) != 1 || len(historical.Manifest.Providers) != 3 ||
		!reflect.DeepEqual(historical.Manifest.StorageSources, manifest.StorageSources) ||
		historical.Manifest.Contract.Has(sdk.ProviderSignIn) ||
		!historical.Manifest.Contract.Has(sdk.RequestPolicy) {
		t.Fatal("legacy flag lowered the actual request-policy runtime floor")
	}
}

func TestProvidersSignInMethodsMatchOpenCodeCredentialAuthority(t *testing.T) {
	manifest := WindowManifest()
	legacy := API8WindowManifest()
	if len(legacy.AuthBindings) != 3 || len(manifest.AuthBindings) < 3 ||
		!reflect.DeepEqual(manifest.AuthBindings[:3], legacy.AuthBindings) {
		t.Fatal("ordinary package changed the existing OpenCode page sign-in grants")
	}
	device := sdk.WindowAuthMethod{ID: "device", Mode: "poll", Label: "Sign in with OpenCode"}
	want := []sdk.WindowAuthBindingDescriptor{
		{ID: "provider.go", From: "provider:signin", Provider: "opencode-go", Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{device}},
		{ID: "provider.zen", From: "provider:signin", Provider: "opencode-zen", Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{device, {ID: "key", Mode: "poll", Label: "Use API key",
				Fields: []sdk.AuthInputField{{Key: "key", Label: "API key", Type: "secret"}}}}},
		{ID: "provider.zen-free", From: "provider:signin", Provider: "opencode-zen-free",
			Methods: []sdk.WindowAuthMethod{{ID: "public", Mode: "poll", Label: "Enable free models"}}},
	}
	if !reflect.DeepEqual(manifest.AuthBindings[3:], want) {
		t.Fatal("OpenCode Providers must offer only methods bound to each provider, with Zen key kept secret")
	}
}
