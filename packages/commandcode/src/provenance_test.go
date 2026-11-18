package commandcode

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPackageProvenanceAndVersionsAgree(t *testing.T) {
	var lock struct {
		Schema int `json:"schema"`
		Plugin struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			SDK     string `json:"sdkMinimum"`
			ABI     uint32 `json:"nativeABI"`
		} `json:"plugin"`
		Sources []struct {
			ID      string `json:"id"`
			Commit  string `json:"commit"`
			Version string `json:"version"`
			Kind    string `json:"revisionKind"`
			Tree    string `json:"tree"`
			Overlay *struct {
				BaseTree string            `json:"basePhysicalTree"`
				Files    map[string]string `json:"files"`
			} `json:"compatibilityOverlay"`
			Distribution struct {
				SHA256    string `json:"sha256"`
				Integrity string `json:"integrity"`
			} `json:"distribution"`
			Paths []string          `json:"paths"`
			Files map[string]string `json:"files"`
		} `json:"sources"`
		Fixtures []struct {
			Path           string `json:"path"`
			Source         string `json:"source"`
			SourcePath     string `json:"sourcePath"`
			SHA256         string `json:"sha256"`
			Transformation string `json:"transformation"`
		} `json:"fixtures"`
		Watch []struct {
			Source    string   `json:"source"`
			Paths     []string `json:"paths"`
			Behaviors []string `json:"behaviors"`
			Tests     []string `json:"tests"`
		} `json:"watch"`
	}
	body, err := os.ReadFile("../docs/source-lock.json")
	if err != nil || json.Unmarshal(body, &lock) != nil {
		t.Fatal("provenance is unreadable", err)
	}
	manifest := Manifest()
	if lock.Schema != 1 || lock.Plugin.ID != manifest.ID || lock.Plugin.Version != Version || lock.Plugin.SDK != "1.12" || lock.Plugin.ABI != manifest.Compatibility.NativeABI {
		t.Fatal("provenance identity/version differs from native module")
	}
	sources := make(map[string]map[string]string)
	for _, source := range lock.Sources {
		pinned := len(source.Commit) == 40 && strings.Trim(source.Commit, "0123456789abcdef") == ""
		if source.Kind == "npm-tarball" {
			pinned = source.Commit == "" && len(source.Distribution.SHA256) == 64 && strings.Trim(source.Distribution.SHA256, "0123456789abcdef") == "" && strings.HasPrefix(source.Distribution.Integrity, "sha512-")
		}
		if !pinned || source.Version == "" || len(source.Paths) == 0 || len(source.Files) == 0 || sources[source.ID] != nil {
			t.Fatalf("unfrozen or duplicate source: %+v", source)
		}
		if source.Kind == "frozen-physical-tree" {
			overlay := source.Overlay
			if len(source.Tree) != 40 || overlay == nil || overlay.BaseTree != source.Tree || len(overlay.Files) != 10 {
				t.Fatal("physical host and compatibility overlay are not separately frozen")
			}
			for path, digest := range overlay.Files {
				if !filepath.IsLocal(path) || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
					t.Fatal("host compatibility overlay has no immutable hash")
				}
			}
		}
		sources[source.ID] = source.Files
		for path, digest := range source.Files {
			if !filepath.IsLocal(path) || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
				t.Fatalf("invalid source file digest: %s %s", path, digest)
			}
		}
	}
	if len(sources) != 4 || len(lock.Fixtures) != 5 || len(lock.Watch) != 4 {
		t.Fatal("source, fixture or watch inventory is incomplete")
	}
	for _, fixture := range lock.Fixtures {
		if !filepath.IsLocal(fixture.Path) || fixture.Transformation == "" || sources[fixture.Source][fixture.SourcePath] == "" {
			t.Fatalf("fixture lost its source: %+v", fixture)
		}
		body, err := os.ReadFile(filepath.Join("..", fixture.Path))
		if err != nil {
			t.Fatal(err)
		}
		if actual := fmt.Sprintf("%x", sha256.Sum256(body)); actual != fixture.SHA256 {
			t.Fatalf("fixture differs from its source lock: %s %s", fixture.Path, actual)
		}
	}
	testFiles, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var testBodies strings.Builder
	for _, path := range testFiles {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		testBodies.Write(body)
	}
	for _, watch := range lock.Watch {
		if sources[watch.Source] == nil || len(watch.Paths) == 0 || len(watch.Behaviors) == 0 || len(watch.Tests) == 0 {
			t.Fatalf("watchpoint lost its source or coverage: %+v", watch)
		}
		for _, name := range watch.Tests {
			if strings.Contains(name, "/") {
				if _, err := os.Stat(filepath.Join("..", name)); err != nil {
					t.Fatal(err)
				}
			} else if !strings.Contains(testBodies.String(), "func "+name+"(") {
				t.Fatalf("watchpoint names no existing regression: %s", name)
			}
		}
	}
	var metadata struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
	}
	body, err = os.ReadFile("../web/package.json")
	if err != nil || json.Unmarshal(body, &metadata) != nil || metadata.Version != Version {
		t.Fatal("package version differs")
	}
	for name, source := range map[string]string{
		"SOURCES.md": "docs/SOURCES.md", "source-lock.json": "docs/source-lock.json",
		"README.md": "README.md", "web": "web", "CONTRACT-PLAN.md": "docs/history/CONTRACT-PLAN.md",
		"SYSTEMONE-SDK-CONTRACT.md":   "docs/SYSTEMONE-SDK-CONTRACT.md",
		"SYSTEMONE-SDK-FIXTURES.json": "docs/SYSTEMONE-SDK-FIXTURES.json",
		"OWN-MODEL-METADATA-API9.md":  "docs/OWN-MODEL-METADATA-API9.md", "THIRD_PARTY_NOTICES": "THIRD_PARTY_NOTICES",
	} {
		if !slices.Contains(metadata.Files, name) {
			t.Fatalf("package excludes %s", name)
		}
		if _, err := os.Stat(filepath.Join("..", source)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPinnedGoMetadataTableIsNotSlimmed(t *testing.T) {
	if len(knownGoModels) != 57 {
		t.Fatalf("pinned whole Go table changed: %d", len(knownGoModels))
	}
}
