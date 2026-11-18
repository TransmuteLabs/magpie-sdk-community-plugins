import hashlib
import json
from pathlib import Path
import shutil
import stat
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import release_build


class ReleaseBuildTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="independent-build-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.workspace = self.root / "workspace"
        self.package = self.workspace / "packages/minimax"
        (self.package / "docs").mkdir(parents=True)
        (self.package / "src").mkdir()
        (self.package / "src/main.go").write_text("package main\n")
        self.version("0.1.0")
        scripts = self.workspace / "scripts"
        scripts.mkdir()
        for name in ("build.py", "_workspace.py", "release.py", "release_build.py"):
            shutil.copy2(Path(release_build.__file__).parent / name, scripts / name)
        sibling = self.workspace / "packages/devin"
        sibling.mkdir()
        (sibling / "source.go").write_text("untouched sibling")
        self.sdk = self.root / "sdk"
        (self.sdk / "pkg/extension").mkdir(parents=True)
        (self.sdk / "pkg/extension/contract.go").write_text("package extension\n")
        (self.sdk / "go.mod").write_text("module github.com/yetone/magpie\n")
        self.index = self.root / "sdk-source.json"
        self.sdk_index()
        self.published = self.root / "published.json"
        self.inventory([])
        self.calls = []
        self.root_patch = patch.object(release_build, "ROOT", self.workspace)
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)

    def version(self, version):
        (self.package / "docs/source-lock.json").write_text(json.dumps({
            "plugin": {"id": "minimax", "version": version, "sdkMinimum": "1.12"},
        }))

    def sdk_index(self):
        files = {
            path.relative_to(self.sdk).as_posix(): {
                "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                "mode": stat.S_IMODE(path.stat().st_mode),
            }
            for path in self.sdk.rglob("*") if path.is_file()
        }
        identity = hashlib.sha256(json.dumps(files, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        self.index.write_text(json.dumps({"identity": identity, "files": files}))
        return identity

    def inventory(self, tags, complete=True):
        self.published.write_text(json.dumps({
            "schema": 1, "complete": complete, "repository": "owner/mono",
            "releases": [{"tag_name": tag, "draft": False, "prerelease": False} for tag in tags],
        }))

    def builder(self, command, check, env=None):
        self.assertTrue(check)
        self.calls.append(command)
        self.assertEqual(command[2], "minimax")
        self.assertNotIn("all", command)
        destination = Path(command[command.index("--output") + 1]) / "minimax"
        destination.mkdir(parents=True)
        provenance = json.loads((self.package / "docs/source-lock.json").read_text())
        manifest = {
            "id": "minimax", "version": provenance["plugin"]["version"],
            "runtime": "native", "entry": "module.so", "contract": {"version": {"major": 1, "minor": 12}},
        }
        (destination / "magpie-plugin.json").write_text(json.dumps(manifest))
        (destination / "source-lock.json").write_text(json.dumps(provenance))
        (destination / "module.so").write_bytes(b"synthetic local module; never execute")

    def build(self, output, version="0.1.0", previous=None, manual=False, selection="minimax"):
        return release_build.build_release(
            selection, version, self.root / output, self.sdk, self.index, self.published,
            previous=previous, manual_dev=manual, builder=self.builder,
        )

    def test_published_version_skips_before_builder_or_sdk(self):
        self.inventory(["devin-inbound-v9.0.0", "minimax-v0.1.0"])
        before = self.published.read_bytes()
        with patch.object(release_build, "sdk_source", side_effect=AssertionError("SDK accessed before published skip")):
            result = self.build("skip")
        self.assertEqual(result["status"], "skip-published")
        self.assertFalse(result["builderInvoked"])
        self.assertEqual(self.calls, [])
        self.assertFalse((self.root / "skip").exists())
        self.assertEqual(self.published.read_bytes(), before)

    def test_unsupported_release_target_refuses_before_inputs_candidate_or_builder(self):
        spec = {"minimax": {"id": "minimax", "platforms": ["darwin/arm64", "darwin/amd64"]}}
        with patch.object(release_build, "packages", return_value=spec), \
                patch.object(release_build.shutil, "which", return_value="/offline/go"), \
                patch.object(release_build, "effective_target", return_value=("linux/amd64", {})), \
                patch.object(release_build, "file_inputs", side_effect=AssertionError("unsupported target read build inputs")):
            with self.assertRaisesRegex(ValueError, r"minimax.*linux/amd64"):
                self.build("unsupported")
        self.assertEqual(self.calls, [])
        self.assertFalse((self.root / "unsupported").exists())

    def test_published_platform_restricted_module_still_skips_before_target_lookup(self):
        self.inventory(["minimax-v0.1.0"])
        spec = {"minimax": {"id": "minimax", "platforms": ["darwin/arm64"]}}
        with patch.object(release_build, "packages", return_value=spec), \
                patch.object(release_build, "effective_target", side_effect=AssertionError("published skip looked up a target")), \
                patch.object(release_build, "sdk_source", side_effect=AssertionError("published skip read the SDK")):
            result = self.build("skip-platform")
        self.assertEqual(result["status"], "skip-published")
        self.assertFalse(result["builderInvoked"])
        self.assertEqual(self.calls, [])
        self.assertFalse((self.root / "skip-platform").exists())

    def test_publication_inventory_distinguishes_valid_sibling_and_malformed_selected_tags(self):
        for sibling in ("minimax-v1-v9.0.0", "minimax-vnext-v9.0.0-rc.1"):
            with self.subTest(sibling=sibling):
                self.inventory([sibling, "minimax-v0.1.0"])
                with patch.object(release_build, "sdk_source", side_effect=AssertionError("published skip read SDK")):
                    self.assertEqual(self.build("skip-sibling")["status"], "skip-published")
        for malformed in ("minimax-vbroken", "minimax-vbad-vbroken", "minimax-vbad+alias-v9.0.0"):
            with self.subTest(malformed=malformed):
                self.inventory([malformed, "minimax-v0.1.0"])
                with self.assertRaises(ValueError):
                    self.build("malformed-selected")
        self.assertEqual(self.calls, [])

    def test_one_selected_module_and_unchanged_local_reuse(self):
        self.inventory(["devin-inbound-v99.0.0"])
        result = self.build("first")
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(result["tag"], "minimax-v0.1.0")
        self.assertTrue(result["publishable"])
        self.assertEqual(result["reference"], "github:owner/mono#minimax@minimax-v0.1.0")
        receipt = self.root / "first/build-receipt.json"
        self.calls.clear()
        (self.workspace / "packages/devin/source.go").write_text("changed sibling does not trigger MiniMax")
        repeated = self.build("unused", previous=receipt)
        self.assertEqual(repeated["status"], "reuse-local")
        self.assertEqual(self.calls, [])
        self.assertFalse((self.root / "unused").exists())
        self.assertFalse((self.root / "first/built/devin").exists())

    def test_sdk_input_change_requires_bump_or_local_dev(self):
        first = self.build("first")
        receipt = self.root / "first/build-receipt.json"
        old_receipt = receipt.read_bytes()
        self.calls.clear()
        (self.sdk / "pkg/extension/contract.go").write_text("package extension\nvar Changed = true\n")
        self.sdk_index()
        with self.assertRaisesRegex(ValueError, "Same-version inputs changed"):
            self.build("refused", previous=receipt)
        self.assertEqual(self.calls, [])
        self.assertFalse((self.root / "refused").exists())
        dev = self.build("local-dev", previous=receipt, manual=True)
        self.assertEqual(dev["status"], "built-local-dev")
        self.assertFalse(dev["publishable"])
        self.assertNotEqual(first["inputsSHA256"], dev["inputsSHA256"])
        self.assertEqual(receipt.read_bytes(), old_receipt)
        self.version("0.2.0")
        bumped = self.build("bumped", version="0.2.0", previous=receipt)
        self.assertEqual(bumped["tag"], "minimax-v0.2.0")
        self.assertTrue(bumped["publishable"])
        self.assertTrue(all(command[2] == "minimax" for command in self.calls))

    def test_published_manual_dev_does_not_authorize_republish_or_overwrite(self):
        self.inventory(["minimax-v0.1.0"])
        before = self.published.read_bytes()
        dev = self.build("dev", manual=True)
        self.assertFalse(dev["publishable"])
        self.assertEqual(dev["status"], "built-local-dev")
        self.assertEqual(len(self.calls), 1)
        with self.assertRaisesRegex(ValueError, "already exists"):
            self.build("dev", manual=True)
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(self.published.read_bytes(), before)
        self.calls.clear()
        skipped = self.build("release")
        self.assertEqual(skipped["status"], "skip-published")
        self.assertEqual(self.calls, [])

    def test_unknown_inventory_aliases_and_version_mismatch_never_build(self):
        for tags, complete in [
            ([], False), (["minimax-v0.1.0+alias"], True),
            (["minimax-v0.1.0", "minimax-v0.1.0"], True),
        ]:
            self.inventory(tags, complete)
            with self.assertRaises(ValueError):
                self.build("refused")
        self.published.write_text('{"schema":1,"complete":false,"complete":true}')
        with self.assertRaisesRegex(ValueError, "Duplicate"):
            self.build("duplicate")
        self.inventory([])
        with self.assertRaisesRegex(ValueError, "differs from source"):
            self.build("wrong-version", version="9.0.0")
        with self.assertRaisesRegex(ValueError, "exactly one"):
            self.build("all", selection="all")
        self.assertEqual(self.calls, [])

    def test_tampered_sdk_or_prior_artifact_never_silently_rebuilds(self):
        self.build("first")
        receipt = self.root / "first/build-receipt.json"
        self.calls.clear()
        (self.sdk / "pkg/extension/contract.go").write_text("unindexed SDK edit")
        with self.assertRaisesRegex(ValueError, "sealed input manifest"):
            self.build("bad-sdk", previous=receipt)
        (self.sdk / "pkg/extension/contract.go").write_text("package extension\n")
        extra = self.sdk / "pkg/extension/unindexed.go"
        extra.write_text("package extension\n")
        with self.assertRaisesRegex(ValueError, "unindexed"):
            self.build("bad-extra", previous=receipt)
        extra.unlink()
        (self.root / "first/built/minimax/module.so").write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "artifact changed"):
            self.build("bad-artifact", previous=receipt)
        self.assertEqual(self.calls, [])


if __name__ == "__main__":
    unittest.main()
