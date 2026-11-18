//go:build !linux && !darwin

package opencode

import (
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestHistoryUnsupportedPlatformKeepsExplicitDefaultDeclaration(t *testing.T) {
	module := NewWindowModule(ModuleOptions{})
	if !module.Manifest.Contract.Has(sdk.StorageSource) || len(module.Manifest.StorageSources) != 1 ||
		module.Handlers[sdk.MethodStorageList] == nil || module.Handlers[sdk.MethodStorageRead] == nil {
		t.Fatal("unsupported platform silently omitted ordinary history")
	}
	if _, err := newHistoryVFS(nil); err == nil || !strings.Contains(err.Error(), "not supported on this platform") {
		t.Fatal("unsupported rooted history did not fail explicitly", err)
	}
}
