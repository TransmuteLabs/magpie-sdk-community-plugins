package zcode

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceLockMatchesActualModuleAndNamesImmutableBehaviorSources(t *testing.T) {
	var lock struct {
		Schema int `json:"schema"`
		Plugin struct {
			ID         string `json:"id"`
			Version    string `json:"version"`
			SDKMinimum string `json:"sdkMinimum"`
			NativeABI  uint32 `json:"nativeABI"`
		} `json:"plugin"`
		Sources []struct {
			ID             string            `json:"id"`
			Role           string            `json:"role"`
			Repository     string            `json:"repository"`
			Commit         string            `json:"commit"`
			Tree           string            `json:"tree"`
			PhysicalTree   string            `json:"physicalTree"`
			Version        string            `json:"version"`
			Paths          []string          `json:"paths"`
			CheckedAt      string            `json:"checkedAt"`
			CheckedFiles   map[string]string `json:"checkedFiles"`
			ArchiveSHA256  string            `json:"archiveSHA256"`
			SnapshotSHA256 string            `json:"snapshotSHA256"`
			BinarySHA256   string            `json:"binarySHA256"`
			TestedModule   string            `json:"testedModuleCommit"`
			NativeLibrary  string            `json:"nativeLibrarySHA256"`
			FixtureLibrary string            `json:"fixtureLibrarySHA256"`
			CheckoutCommit string            `json:"checkoutCommit"`
			OriginHEAD     string            `json:"observedOriginHEAD"`
			ZcodeTree      string            `json:"observedZcodeTree"`
			Updates        []struct {
				Commit   string `json:"commit"`
				Version  string `json:"version"`
				Behavior string `json:"behavior"`
			} `json:"updates"`
			NoBehaviorDelta bool     `json:"noBehaviorDelta"`
			ComparedCommits []string `json:"comparedCommits"`
		} `json:"sources"`
		Watch []struct {
			Source    string   `json:"source"`
			Paths     []string `json:"paths"`
			Behaviors []string `json:"behaviors"`
			Tests     []string `json:"tests"`
		} `json:"watch"`
		Limits []string `json:"limits"`
	}
	body, err := os.ReadFile("../docs/source-lock.json")
	if err != nil || json.Unmarshal(body, &lock) != nil {
		t.Fatal("source-lock.json missing or unreadable:", err)
	}
	manifest := ControlManifest()
	if lock.Schema != 1 || lock.Plugin.ID != manifest.ID || lock.Plugin.Version != manifest.Version ||
		lock.Plugin.SDKMinimum != fmt.Sprintf("%d.%d", manifest.Contract.Version.Major, manifest.Contract.Version.Minor) ||
		lock.Plugin.NativeABI != manifest.Compatibility.NativeABI || len(lock.Limits) == 0 {
		t.Fatal("source lock conflated module, SDK or ABI versions")
	}
	seen := map[string]bool{}
	for _, source := range lock.Sources {
		if source.ID == "" || seen[source.ID] || source.Role == "" || source.Version == "" ||
			!strings.HasPrefix(source.Repository, "https://") || len(source.Commit) != 40 || len(source.Paths) == 0 {
			t.Fatal("source needs a unique identity, role, version, paths and exact commit")
		}
		if _, err := hex.DecodeString(source.Commit); err != nil {
			t.Fatal("source commit is not immutable hexadecimal")
		}
		if source.ID == "sdk-host" {
			if decoded, err := hex.DecodeString(source.Tree); err != nil || len(decoded) != 20 {
				t.Fatal("verification host needs an exact committed tree, excluding dirty SDK work")
			}
		}
		if source.ID == "sdk-base-history9" {
			if source.Commit != "66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55" ||
				source.Tree != "94ec4c8de4e5b366a6e3fb854df2176007587ab8" || source.PhysicalTree != source.Tree ||
				source.SnapshotSHA256 != "d97b5eca0017294ddccc457ee691411ede08c190e12a0d7857f27af6d49ec7c7" ||
				source.BinarySHA256 != "a8e09770909f78ae89cc6175a45bb98e1dacdb305b62856f46956928cb8ecbe2" {
				t.Fatal("automatic-history verification lost its exact merged base and physical source identity")
			}
		}
		if source.ID == "sdk-observability7" {
			if source.Commit != "9e88f10982a1d66c1b402fba27caddda8ac4ac4e" ||
				source.Tree != "43b7e290d9c3370881780b23ad49222f5d27f44b" ||
				source.ArchiveSHA256 != "dd5bf036acdbb39bf1d93a415816b4ccea5700c40c49f2f430eea3dacd411454" ||
				!strings.Contains(source.Version, "Uncommitted") {
				t.Fatal("API7 verification snapshot was conflated with a committed host release")
			}
			for _, path := range []string{"pkg/extension/usage_facts.go", "pkg/extension/diagnostics.go",
				"pkg/extension/stream.go", "pkg/extension/types.go", "internal/extension/diagnostics.go"} {
				if source.CheckedFiles[path] == "" {
					t.Fatal("observability adoption has no checked SDK bytes:", path)
				}
			}
		}
		if source.ID == "sdk-concurrency-quota9" {
			if source.Commit != "ae957e5a8a77553a6cf6f484c8b58a69393f3887" ||
				source.Tree != "" || !strings.Contains(source.Version, "Uncommitted") {
				t.Fatal("API9 source snapshot was conflated with a committed host tree")
			}
			for _, digest := range []string{source.ArchiveSHA256, source.SnapshotSHA256} {
				if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != 32 {
					t.Fatal("API9 source has no exact archive and content/mode inventory")
				}
			}
			for _, path := range []string{"pkg/extension/provider_defaults.go", "pkg/extension/rich_quota.go",
				"pkg/extension/manifest.go", "pkg/extension/types.go", "internal/provider/extension_defaults.go",
				"internal/provider/extension_provider.go", "internal/provider/quota_amount.go", "internal/gui/assets/app.js"} {
				if source.CheckedFiles[path] == "" {
					t.Fatal("API9 adoption has no checked SDK bytes:", path)
				}
			}
		}
		if source.ID == "sdk-agent-options9" {
			if source.Commit != "46756a48c9c9f204a8369638dc63cbcb83d02031" ||
				source.PhysicalTree != "db40ddbf8944b54f9f689d444644af4c3bc76881" ||
				source.ArchiveSHA256 != "e188abd5d115c7da3167a6296e7d07f9bd31766a738ea4e6326ff093e4a74a61" ||
				source.Tree != "" {
				t.Fatal("current agent compatibility lost its exact shared commit and separate physical source identity")
			}
			for _, path := range []string{"pkg/extension/agent_decision_models.go", "pkg/extension/agent_model_metadata.go",
				"internal/agent/extension_actions.go", "internal/agent/extension_decision_models.go", "internal/agent/agent.go"} {
				if source.CheckedFiles[path] == "" {
					t.Fatal("agent-options compatibility has no checked source bytes:", path)
				}
			}
		}
		if source.ID == "sdk-final-native9" {
			if source.Commit != "c11acbc2bebec1fb213ef562900d35188cdf55fe" ||
				source.PhysicalTree != "8fe78ab9df13d733d61462eacce19bff523c1e58" ||
				source.ArchiveSHA256 != "01117ce6f6f8842d16fcd81224dad1795020b3b5c4d69b9145d562fd88171e37" ||
				source.SnapshotSHA256 != "7ce4753ff79f3493acb4f61c1826d3f52145f8b156537ac61f493b39c9a50cc6" ||
				source.BinarySHA256 != "c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86" ||
				source.TestedModule != "c8281b6c97d751207f429bcbc1861daa2c465abb" ||
				source.NativeLibrary != "1b408ffbb73a7db0874d9321777947c5eb89d25c66e2c089fa84b3f12c686bc3" ||
				source.FixtureLibrary != "57c6c01915dd6b46cde2c031394f6ee5a7972023f0887021930e8a6183122327" ||
				source.Tree != "" {
				t.Fatal("native lifecycle repin lost its exact source, binary or unchanged module/library identity")
			}
			for _, path := range []string{"pkg/extension/manifest.go", "pkg/extension/agent_decision_models.go",
				"internal/agent/extension_actions.go", "internal/provider/extension_catalog_projection.go",
				"internal/extension/account_stamp.go"} {
				if source.CheckedFiles[path] == "" {
					t.Fatal("native lifecycle repin has no checked source bytes:", path)
				}
			}
		}
		for path, digest := range source.CheckedFiles {
			if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != 32 || !fs.ValidPath(path) {
				t.Fatal("checked source file lost its repository-relative hash")
			}
		}
		if source.ID == "vendor-current-audit" {
			if source.Version != "desktop "+ClientVersion+" / CLI 0.17.1" ||
				source.Commit != "fec975905a2abd4c07e35c40d51fa097a2258ec9" {
				t.Fatal("wire version is not backed by the audited desktop and CLI source")
			}
			for _, path := range []string{
				"package.json", "apps/zcode-cli/package.json",
				"apps/zcode-cli/packages/core/src/context/builder.ts",
				"apps/zcode-cli/packages/adapters/src/model/runner-attribution.ts",
				"apps/zcode-cli/packages/adapters/src/storage/session-store/repositories/usage.ts",
				"apps/zcode-cli/packages/adapters/src/logging/serialize.ts",
				"apps/zcode-cli/tools/provider-regression/src/usage.ts",
				"apps/zcode-cli/packages/adapters/src/model/runner-diagnostics.ts",
				"apps/zcode-cli/packages/adapters/src/model/model-execution.ts",
				"packages/desktop/test/e2e/helpers/upstream-capture.ts",
				"packages/desktop/test/e2e/helpers/upstream-replay-server.ts",
			} {
				if source.CheckedFiles[path] == "" {
					t.Fatal("ported native behavior has no checked source bytes:", path)
				}
			}
		}
		if source.ID == "community-zcode-0168" {
			if source.Commit != "324acd6007f2d513c5072fe2e9fe1db3524ae03d" || source.Version != "0.1.68" ||
				source.ZcodeTree != "91595e647e66e6b45105ce45fcea2291b85d0634" ||
				source.CheckoutCommit != "57d1cc6a9f49389d495ea9be8d986c70de7767bf" ||
				source.CheckedAt != "2026-11-17" || len(source.Updates) != 2 {
				t.Fatal("current Start donor lost its independent exact provenance")
			}
			for _, path := range []string{"package.json", "index.mjs", "README.md",
				"concurrency.test.mjs", "inflight.test.mjs", "own.test.mjs", "start.test.mjs"} {
				if source.CheckedFiles["packages/zcode/"+path] == "" {
					t.Fatal("current Start port has no checked donor file:", path)
				}
			}
		}
		if source.ID == "community-zcode-0169" {
			if source.Commit != "fdbf261b7d01d1c908220029473d671cc420cab7" || source.Version != "0.1.69" ||
				source.ZcodeTree != "ee4de91f09b88382e6b2d55236b1f1e3e93dc6ec" ||
				source.CheckoutCommit != "57d1cc6a9f49389d495ea9be8d986c70de7767bf" ||
				source.CheckedAt != "2026-11-22" {
				t.Fatal("check-in donor lost its exact provenance")
			}
			for _, path := range []string{"package.json", "index.mjs", "README.md", "checkin.test.mjs"} {
				if source.CheckedFiles["packages/zcode/"+path] == "" {
					t.Fatal("check-in port has no checked donor file:", path)
				}
			}
		}
		if source.ID == "community-zcode-0166" {
			if source.Commit != "87a8cf03c62fa55ca845c51648e8859ece580336" || source.Version != "0.1.66" ||
				source.CheckoutCommit != "26ea8faf7793ca66dd8e7a8b90128a5235552a1f" ||
				source.OriginHEAD != "a60667b57040f62080da60f6018cc7351fa1953f" || len(source.Updates) != 4 {
				t.Fatal("bounded donor source confused checkout, observed upstream or ported version")
			}
			for index, commit := range []string{
				"3f37b858db8074959447c16ed06ac51d444c7fcf", "8e19b6a0e712e4ea844ccd5e3fe95522f570ae05",
				"1dbbc1fb489d7ac40b0620d802301075e25332b6", "87a8cf03c62fa55ca845c51648e8859ece580336",
			} {
				update := source.Updates[index]
				if update.Commit != commit || update.Version != fmt.Sprintf("0.1.%d", 63+index) || update.Behavior == "" {
					t.Fatal("bounded donor update lost its exact version or behavior")
				}
			}
			for _, path := range []string{"package.json", "index.mjs", "prompt.mjs", "README.md",
				"own.test.mjs", "gift.test.mjs", "inflight.test.mjs"} {
				if source.CheckedFiles["packages/zcode/"+path] == "" {
					t.Fatal("bounded donor behavior has no checked file bytes:", path)
				}
			}
		}
		if source.ID == "community-zcode-current-audit" {
			if source.Commit != "57d1cc6a9f49389d495ea9be8d986c70de7767bf" ||
				source.ZcodeTree != "d1be84b45f19c7d51bf3bf1c428ddc7ae95dfbf7" ||
				source.CheckedFiles["packages/zcode/index.mjs"] != "cd81a4e3f59cadefa677d262a9969b284ccc6c0bdae4fe9a7fe456155bbd73a8" {
				t.Fatal("fresh community audit lost its exact ref and unchanged ported subtree")
			}
		}
		if source.ID == "community-zcode-20261116-noop-audit" {
			if source.Commit != "b35d4b35b2446bd929896ddeb58f3cb299e9e2d1" ||
				source.ZcodeTree != "d1be84b45f19c7d51bf3bf1c428ddc7ae95dfbf7" ||
				source.CheckoutCommit != "57d1cc6a9f49389d495ea9be8d986c70de7767bf" ||
				source.OriginHEAD != source.Commit || source.Version != "0.1.66" ||
				source.CheckedAt != "2026-11-16" || !source.NoBehaviorDelta || len(source.ComparedCommits) != 2 ||
				source.ComparedCommits[0] != "57d1cc6a9f49389d495ea9be8d986c70de7767bf" ||
				source.ComparedCommits[1] != "87a8cf03c62fa55ca845c51648e8859ece580336" {
				t.Fatal("no-op donor audit invented a version, port or checkout change")
			}
		}
		if source.ID == "vendor-zcode-20261116-noop-audit" {
			if source.Commit != "fec975905a2abd4c07e35c40d51fa097a2258ec9" ||
				source.Tree != "5e90fec80706b8381f94e4eef48cf821b9868916" ||
				source.OriginHEAD != source.Commit || source.CheckedAt != "2026-11-16" ||
				source.Version != "desktop 3.18.2 / CLI 0.17.1" || !source.NoBehaviorDelta {
				t.Fatal("no-op vendor audit changed the established desktop/CLI baseline")
			}
		}
		if source.ID == "vendor-control-completion" {
			if source.Commit != "fec975905a2abd4c07e35c40d51fa097a2258ec9" {
				t.Fatal("native/control continuation has no exact official authority")
			}
			for _, path := range []string{"packages/shared/src/model-config.ts",
				"packages/services/src/usage-stats/providers/bigmodelUsageQuotaProvider.ts",
				"apps/zcode-cli/packages/cli/src/arguments.ts",
				"apps/zcode-cli/packages/bootstrap/src/zcode-protocol/provider-runtime-headers.ts"} {
				if source.CheckedFiles[path] == "" {
					t.Fatal("native/control semantics have no checked official bytes:", path)
				}
			}
		}
		if _, err := time.Parse("2006-01-02", source.CheckedAt); err != nil {
			t.Fatal("source has no checked date")
		}
		for _, path := range source.Paths {
			if !fs.ValidPath(path) || strings.Contains(path, "\\") {
				t.Fatal("source path is not repository-relative")
			}
		}
		seen[source.ID] = true
	}
	for _, id := range []string{"sdk-host", "sdk-observability7", "sdk-concurrency-quota9", "sdk-agent-options9", "sdk-final-native9",
		"sdk-base-history9", "vendor-sqlite-history", "rooted-vfs-donor", "sdk-request-policy12", "vendor-request-policy",
		"community-zcode-20261116-noop-audit", "vendor-zcode-20261116-noop-audit",
		"vendor-baseline", "vendor-current-audit", "vendor-control-completion", "archived-controller",
		"community-zcode", "community-zcode-0166", "community-zcode-0168", "community-zcode-0169", "community-zcode-current-audit"} {
		if !seen[id] {
			t.Fatal("required source lineage omitted:", id)
		}
	}
	for _, watch := range lock.Watch {
		if !seen[watch.Source] || len(watch.Paths) == 0 || len(watch.Behaviors) == 0 || len(watch.Tests) == 0 {
			t.Fatal("watch has no known source/paths/behaviors/regression surface")
		}
		for _, path := range watch.Tests {
			if !fs.ValidPath(path) {
				t.Fatal("regression path escaped the package")
			}
			if _, err := os.Stat(filepath.Join("..", path)); err != nil {
				t.Fatal("watch points at a missing regression:", path)
			}
		}
	}
}
