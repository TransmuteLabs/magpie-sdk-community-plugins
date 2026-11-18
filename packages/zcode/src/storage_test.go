package zcode

import (
	"reflect"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAutomaticHistoryIsRegisteredInTheOrdinaryNativePackage(t *testing.T) {
	module := NewControlModule(ModuleOptions{})
	if err := module.Validate(); err != nil {
		t.Fatal(err)
	}
	if !module.Manifest.Contract.Has(sdk.StorageSource) || len(module.Manifest.StorageSources) != 1 {
		t.Fatal("ordinary native package has no automatic rooted SQLite history source")
	}
	source := module.Manifest.StorageSources[0]
	if source.ID != "zcode-native-history" || source.Agent != "zcode" || source.Codec != "zcode-sqlite-v1" ||
		!source.Transcript || source.Resume != "agent" ||
		!reflect.DeepEqual(source.AgentResume, &sdk.SessionResumeRecipe{Agent: AgentID, Args: []string{"--resume", "{sessionId}"}}) ||
		!reflect.DeepEqual(source.Roots, []sdk.SessionSourceStore{{Slot: "cli", HomeRelativePath: ".zcode/cli/db"}}) ||
		!reflect.DeepEqual(source.Families, []sdk.StorageFamily{{ID: "cli", Primary: "db.sqlite",
			Sidecars: []string{"db.sqlite-wal", "db.sqlite-shm", "db.sqlite-journal"}}}) {
		t.Fatalf("automatic history changed its exact default root, finite family or owned resume recipe: %+v", source)
	}
	for _, method := range []string{sdk.MethodStorageList, sdk.MethodStorageRead} {
		if module.Handlers[method] == nil {
			t.Fatal("automatic history handler is absent:", method)
		}
	}
	if Manifest().Contract.Has(sdk.StorageSource) || len(Manifest().StorageSources) != 0 {
		t.Fatal("the historical API1.4 headless surface acquired an automatic reader")
	}
}
