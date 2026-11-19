import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import release_local
from release_build import sdk_inputs


class LocalReleaseTests(unittest.TestCase):
    def setUp(self):
        scratch = tempfile.TemporaryDirectory()
        self.addCleanup(scratch.cleanup)
        self.root = Path(scratch.name)
        self.output = self.root / "release"
        self.package = self.root / "packages/minimax"
        (self.package / "docs").mkdir(parents=True)
        (self.package / "docs/source-lock.json").write_text(json.dumps({
            "plugin": {"id": "minimax", "version": "3.2.1"},
        }))
        patcher = patch.object(release_local, "ROOT", self.root)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_published_package_skips_before_go_or_sdk(self):
        records = [{"tag_name": "minimax-v3.2.1", "draft": False, "prerelease": False}]
        with patch.object(release_local, "published_releases", return_value=records), \
                patch.object(release_local, "sdk_source", side_effect=AssertionError("opened SDK")), \
                patch.object(release_local, "effective_target", side_effect=AssertionError("ran Go")):
            result = release_local.prepare_release("minimax", self.output)
        self.assertEqual(result, {"status": "skip-published", "tag": "minimax-v3.2.1"})
        self.assertFalse(self.output.exists())

    def test_sibling_release_does_not_drive_selected_package(self):
        records = [{"tag_name": "zcode-v9.0.0", "draft": False, "prerelease": False}]
        sdk = self.root / "sdk"

        def package_fixture(selection, inputs, output, platform):
            output.mkdir(parents=True)
            return [{"id": selection}]

        with patch.object(release_local, "published_releases", return_value=records), \
                patch.object(release_local, "plugin_commit", return_value="b" * 40), \
                patch.object(release_local.sys, "platform", "darwin"), \
                patch.object(release_local.platform, "machine", return_value="arm64"), \
                patch.object(release_local, "effective_target", return_value=("darwin/arm64", {})), \
                patch.object(release_local, "sdk_source", return_value=sdk), \
                patch.object(release_local, "seal_sdk", return_value="a" * 40), \
                patch.object(release_local, "build_release", return_value={
                    "status": "built-release-candidate", "output": self.output / "candidate",
                }) as build, \
                patch.object(release_local, "package_release", side_effect=package_fixture) as package:
            result = release_local.prepare_release("minimax", self.output, sdk)
        self.assertEqual(build.call_args.args[:3], ("minimax", "3.2.1", self.output / "candidate"))
        package.assert_called_once_with("minimax", self.output / "candidate/built",
                                        self.output / "assets", "darwin_arm64")
        self.assertEqual(result["tag"], "minimax-v3.2.1")
        self.assertEqual(json.loads((self.output / "release.json").read_text())["pluginCommit"], "b" * 40)

    def test_exact_draft_does_not_build(self):
        records = [{"tag_name": "minimax-v3.2.1", "draft": True, "prerelease": False}]
        with patch.object(release_local, "published_releases", return_value=records), \
                patch.object(release_local, "build_release", side_effect=AssertionError("built draft")):
            with self.assertRaisesRegex(ValueError, "draft already exists"):
                release_local.prepare_release("minimax", self.output)
        self.assertFalse(self.output.exists())

    def commit_files(self, repo, *paths):
        git = ["git", "-C", str(repo)]
        subprocess.run([*git, "init", "-q"], check=True)
        subprocess.run([*git, "add", *paths], check=True)
        subprocess.run([*git, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                        "-c", "commit.gpgsign=false", "commit", "-qm", "SDK fixture"], check=True)
        return subprocess.check_output([*git, "rev-parse", "HEAD"], text=True).strip()

    def test_dirty_neighbor_does_not_block_but_dirty_selected_source_does(self):
        commit = self.commit_files(self.root, "packages/minimax")
        sibling = self.root / "packages/zcode"
        sibling.mkdir()
        (sibling / "new.go").touch()
        self.assertEqual(release_local.plugin_commit("minimax"), commit)
        (self.package / "docs/source-lock.json").write_text("{}")
        with self.assertRaisesRegex(ValueError, "packages/minimax/docs/source-lock.json"):
            release_local.plugin_commit("minimax")

    def test_sealed_sdk_uses_committed_bytes(self):
        sdk = self.root / "sdk"
        sdk.mkdir()
        contract = sdk / "contract.go"
        contract.write_text("committed SDK\n")
        expected_commit = self.commit_files(sdk, "contract.go")
        contract.write_text("dirty SDK\n")
        (sdk / "untracked").touch()
        sealed = self.root / "sealed"
        index = self.root / "sdk.json"
        commit = release_local.seal_sdk(sdk, sealed, index)
        self.assertEqual(commit, expected_commit)
        self.assertEqual(json.loads(index.read_text())["commit"], commit)
        self.assertEqual((sealed / "contract.go").read_text(), "committed SDK\n")
        self.assertEqual(sdk_inputs(sealed, index)["files"], 1)
