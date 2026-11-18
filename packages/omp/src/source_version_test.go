package ompsessions

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSourceLockMatchesDeclaredModuleAndRecordedEvidence(t *testing.T) {
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
			Repository string   `json:"repository"`
			Commit     string   `json:"commit"`
			Paths      []string `json:"paths"`
		} `json:"sources"`
		Artifacts []struct {
			Source                string `json:"source"`
			Path                  string `json:"path"`
			SHA256                string `json:"sha256"`
			DeclaredClientVersion string `json:"declaredClientVersion"`
		} `json:"artifacts"`
		Acceptance struct {
			Version string `json:"version"`
			Record  string `json:"record"`
			Runner  string `json:"runner"`
		} `json:"acceptance"`
		Watch []struct {
			Source    string   `json:"source"`
			Paths     []string `json:"paths"`
			Behaviors []string `json:"behaviors"`
			Tests     []string `json:"tests"`
		} `json:"watch"`
		Limits []string `json:"limits"`
	}
	if err := json.Unmarshal(body, &lock); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest()
	minimum := strconv.Itoa(manifest.Contract.Version.Major) + "." + strconv.Itoa(manifest.Contract.Version.Minor)
	if lock.Schema != 1 || lock.Plugin.ID != manifest.ID || lock.Plugin.Version != manifest.Version ||
		lock.Plugin.SDKMinimum != minimum || manifest.Compatibility.HostAPI != "^"+minimum ||
		lock.Plugin.NativeABI != manifest.Compatibility.NativeABI || len(lock.Limits) == 0 {
		t.Fatal("repository provenance differs from the declared default distribution")
	}
	sources := map[string]bool{}
	for _, source := range lock.Sources {
		commit, err := hex.DecodeString(source.Commit)
		if source.ID == "" || sources[source.ID] || err != nil || len(commit) != 20 ||
			!strings.HasPrefix(source.Repository, "https://github.com/") || len(source.Paths) == 0 {
			t.Fatal("incomplete or duplicate source provenance:", source.ID)
		}
		for _, name := range source.Paths {
			if filepath.IsAbs(name) || strings.Contains(name, "..") {
				t.Fatal("donor path escapes its repository:", name)
			}
		}
		sources[source.ID] = true
	}
	for _, id := range []string{"omp", "codex-infinity", "openai-codex", "archive-baseline", "host-sdk"} {
		if !sources[id] {
			t.Fatal("missing donor provenance:", id)
		}
	}
	for _, watch := range lock.Watch {
		if !sources[watch.Source] || len(watch.Paths) == 0 || len(watch.Behaviors) == 0 || len(watch.Tests) == 0 {
			t.Fatal("watchpoint has no donor or behavioral coverage mapping")
		}
		for _, name := range watch.Tests {
			if filepath.IsAbs(name) || strings.Contains(name, "..") {
				t.Fatal("watchpoint escapes this module:", name)
			}
			if _, err := os.Stat(filepath.Join("..", name)); err != nil {
				t.Fatal("watchpoint references a missing fixture:", name)
			}
		}
	}
	if len(lock.Artifacts) != 1 || lock.Artifacts[0].Source != "host-sdk" ||
		lock.Artifacts[0].Path != "internal/sessions/testdata/carry/codex-tools.jsonl" ||
		lock.Artifacts[0].SHA256 != "c2ca94642e17ac78278e51b07ec2ce0a8777f141e39fdc8b774ee45761b13dde" ||
		lock.Artifacts[0].DeclaredClientVersion != "0.162.1" || lock.Acceptance.Version != "0.160.0" {
		t.Fatal("sanitized fixture evidence was conflated with installed-client acceptance")
	}
	for _, name := range []string{"docs/SOURCES.md", lock.Acceptance.Record, lock.Acceptance.Runner} {
		if name == "" {
			t.Fatal("missing provenance or historical acceptance reference")
		}
		if _, err := os.Stat(filepath.Join("..", name)); err != nil {
			t.Fatal(err)
		}
	}
}
