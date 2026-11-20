package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDonorFixtureProvenanceMatchesReviewedBytes(t *testing.T) {
	body, err := os.ReadFile("testdata/donors/provenance.json")
	var records []struct{ Fixture, Commit, Path, Selection, SHA256 string }
	if err != nil || json.Unmarshal(body, &records) != nil || len(records) < 8 {
		t.Fatal("donor fixture provenance missing", err)
	}
	for _, record := range records {
		if !strings.HasPrefix(record.Fixture, "testdata/donors/") || strings.Contains(record.Fixture, "..") ||
			len(record.Commit) != 40 || record.Path == "" || record.Selection == "" {
			t.Fatal("invalid donor extraction record", record.Fixture)
		}
		fixture, err := os.ReadFile(record.Fixture)
		digest := sha256.Sum256(fixture)
		if err != nil || hex.EncodeToString(digest[:]) != record.SHA256 {
			t.Fatal("donor fixture changed without reviewed provenance", record.Fixture)
		}
	}
}
func TestSourceLockMatchesModuleVendorAndContractVersions(t *testing.T) {
	body, err := os.ReadFile("../docs/source-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Schema int `json:"schema"`
		Plugin struct {
			ID         string `json:"id"`
			Version    string `json:"version"`
			SDKMinimum string `json:"sdkMinimum"`
			NativeABI  uint32 `json:"nativeABI"`
		} `json:"plugin"`
		Sources []struct {
			ID         string   `json:"id"`
			Role       string   `json:"role"`
			Repository string   `json:"repository"`
			Commit     string   `json:"commit"`
			Version    *string  `json:"version"`
			Paths      []string `json:"paths"`
			CheckedAt  string   `json:"checkedAt"`
		} `json:"sources"`
		Watch []struct {
			Source    string   `json:"source"`
			Paths     []string `json:"paths"`
			Behaviors []string `json:"behaviors"`
			Tests     []string `json:"tests"`
		} `json:"watch"`
		Limits []string `json:"limits"`
	}
	if json.Unmarshal(body, &lock) != nil {
		t.Fatal("invalid source lock")
	}
	manifest := WindowManifest()
	minimum := strconv.Itoa(manifest.Contract.Version.Major) + "." + strconv.Itoa(manifest.Contract.Version.Minor)
	if lock.Schema != 1 || lock.Plugin.ID != manifest.ID || lock.Plugin.Version != manifest.Version ||
		lock.Plugin.SDKMinimum != minimum || manifest.Compatibility.HostAPI != "^"+minimum ||
		lock.Plugin.NativeABI != manifest.Compatibility.NativeABI || len(lock.Limits) == 0 {
		t.Fatal("source lock drifted from repository module declaration")
	}
	metadata, err := os.ReadFile("../package.json")
	var release struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err != nil || json.Unmarshal(metadata, &release) != nil || release.Name != manifest.ID || release.Version != manifest.Version {
		t.Fatal("release metadata drifted from the native manifest", err)
	}
	documentation, err := os.ReadFile("../docs/SOURCES.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{Manifest().Version, AgentManifest().Version, WindowManifest().Version} {
		if !strings.Contains(string(documentation), variant) {
			t.Fatal("distribution version missing from source record:", variant)
		}
	}
	sources := map[string]bool{}
	for _, source := range lock.Sources {
		commit, err := hex.DecodeString(source.Commit)
		_, dateErr := time.Parse("2006-01-02", source.CheckedAt)
		if err != nil || len(commit) != 20 || dateErr != nil ||
			!strings.HasPrefix(source.Repository, "https://github.com/") || source.Role == "" ||
			source.ID == "" || sources[source.ID] || len(source.Paths) == 0 {
			t.Fatal("incomplete or duplicate source provenance:", source.ID)
		}
		if source.ID == "opencode" && (source.Version == nil || *source.Version != ClientVersion) {
			t.Fatal("source lock vendor target drifted from request client version")
		}
		for _, path := range source.Paths {
			if filepath.IsAbs(path) || strings.Contains(path, "..") {
				t.Fatal("source path escapes its repository")
			}
		}
		sources[source.ID] = true
	}
	for _, id := range []string{"opencode", "community-zen-free", "host-sdk", "archive-baseline"} {
		if !sources[id] {
			t.Fatal("required donor provenance missing:", id)
		}
	}
	for _, watch := range lock.Watch {
		if !sources[watch.Source] || len(watch.Paths) == 0 || len(watch.Behaviors) == 0 || len(watch.Tests) == 0 {
			t.Fatal("source watch has no behavioral mapping")
		}
		for _, path := range watch.Tests {
			if filepath.IsAbs(path) || strings.Contains(path, "..") {
				t.Fatal("watch fixture escapes this repository")
			}
			if _, err := os.Stat(filepath.Join("..", path)); err != nil {
				t.Fatal("watch fixture is missing:", path)
			}
		}
	}
}

func TestOrdinaryManifestIncludesEveryReadyComponent(t *testing.T) {
	body, err := os.ReadFile("magpie-plugin.json")
	var manifest sdk.Manifest
	if err != nil || json.Unmarshal(body, &manifest) != nil {
		t.Fatal("ordinary full-package declaration is missing", err)
	}
	expected := WindowManifest()
	if manifest.ID != expected.ID || manifest.Version != expected.Version || manifest.Contract.Version != expected.Contract.Version ||
		manifest.Window == nil || len(manifest.Providers) != 3 || len(manifest.Agents) != len(expected.Agents) ||
		!reflect.DeepEqual(manifest.AuthBindings, expected.AuthBindings) || len(manifest.Bindings) != len(expected.Bindings) ||
		!reflect.DeepEqual(manifest.StorageSources, expected.StorageSources) || !manifest.Contract.Has(sdk.StorageSource) {
		t.Fatal("ordinary manifest omitted a ready provider, agent or auth/UI component")
	}
	if Manifest().Version != "0.2.0" || Manifest().Contract.Version.Minor != 12 || AgentManifest().Version != "0.3.0" {
		t.Fatal("historical compatibility declarations changed")
	}
}

func TestSourceLockDeclaresCurrentAPI8VariantWithoutRepublishingHistory(t *testing.T) {
	body, err := os.ReadFile("../docs/source-lock.json")
	var lock struct {
		Variant                string `json:"variant"`
		CompatibilityArtifacts []struct {
			ID         string `json:"id"`
			Version    string `json:"version"`
			SDKMinimum string `json:"sdkMinimum"`
			NativeABI  uint32 `json:"nativeABI"`
			Variant    string `json:"variant"`
			BuildFlag  string `json:"buildFlag"`
		} `json:"compatibilityArtifacts"`
	}
	if err != nil || json.Unmarshal(body, &lock) != nil || lock.Variant != "full" || len(lock.CompatibilityArtifacts) != 1 {
		t.Fatal("full/API8 variants have no exact source lock")
	}
	artifact := lock.CompatibilityArtifacts[0]
	manifest := API8WindowManifest()
	if artifact.ID != manifest.ID || artifact.Version != manifest.Version || artifact.Version != "0.8.1" ||
		artifact.SDKMinimum != "1.12" || manifest.Contract.Has(sdk.ProviderSignIn) ||
		artifact.NativeABI != manifest.Compatibility.NativeABI || artifact.Variant != "api8" || artifact.BuildFlag != "--compat-api8" {
		t.Fatal("API8 compatibility would republish immutable history or drift from its declared version")
	}
}
