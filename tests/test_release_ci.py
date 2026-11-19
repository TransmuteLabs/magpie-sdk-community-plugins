import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import release_ci
from release_build import sdk_inputs


class ReleaseCITests(unittest.TestCase):
    def setUp(self):
        scratch = tempfile.TemporaryDirectory()
        self.addCleanup(scratch.cleanup)
        self.root = Path(scratch.name)
        self.inventory = self.root / "publications.json"
        self.entries = {name: {"id": name} for name in ("minimax", "opencode", "zcode")}
        for name in self.entries:
            (self.root / "packages" / name / "docs").mkdir(parents=True)
            self.write_version(name, "3.2.1")
        for patcher in (
            patch.object(release_ci, "ROOT", self.root),
            patch.object(release_ci, "packages", side_effect=self.packages),
        ):
            patcher.start()
            self.addCleanup(patcher.stop)

    def packages(self, name="all"):
        return self.entries if name == "all" else {name: self.entries[name]}

    def write_version(self, package, version, **metadata):
        path = self.root / "packages" / package / "package.json"
        path.write_text(json.dumps({"name": package, "version": version, **metadata}))

    def write_inventory(self, *tags, draft=False):
        self.inventory.write_text(json.dumps({
            "schema": 1, "repository": release_ci.REPOSITORY, "complete": True,
            "releases": [{"tag_name": tag, "draft": draft, "prerelease": False} for tag in tags],
        }))

    def commit(self, repository, *paths):
        git = ["git", "-C", str(repository)]
        subprocess.run([*git, "init", "-q"], check=True)
        subprocess.run([*git, "add", *paths], check=True)
        subprocess.run([*git, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                        "-c", "commit.gpgsign=false", "commit", "-qm", "Release fixture"], check=True)
        return subprocess.check_output([*git, "rev-parse", "HEAD"], text=True).strip()

    def test_zcode_bump_only_selects_zcode_even_without_sibling_releases(self):
        before = self.commit(self.root, "packages")
        self.write_version("zcode", "3.3.0")
        self.write_version("opencode", "3.2.1", description="updated")
        self.commit(self.root, "packages")
        self.write_inventory()
        candidates = release_ci.release_candidates(before=before)
        self.assertEqual(release_ci.unpublished(candidates, self.inventory), [
            {"package": "zcode", "id": "zcode", "version": "3.3.0", "tag": "zcode-v3.3.0"},
        ])
        self.write_inventory("zcode-v3.3.0")
        self.assertEqual(release_ci.unpublished(candidates, self.inventory), [])

    def test_changes_without_version_bumps_do_not_plan_a_build(self):
        before = self.commit(self.root, "packages")
        self.write_version("zcode", "3.2.1", description="updated")
        self.commit(self.root, "packages")
        self.assertEqual(release_ci.release_candidates(before=before), [])

    def test_adding_manifests_does_not_release_existing_versions(self):
        for name in self.entries:
            package = self.root / "packages" / name
            (package / "package.json").unlink()
            (package / "docs/source-lock.json").write_text(json.dumps({
                "plugin": {"id": name, "version": "3.2.1"},
            }))
        before = self.commit(self.root, "packages")
        for name in self.entries:
            self.write_version(name, "3.2.1")
        self.commit(self.root, "packages")
        self.assertEqual(release_ci.release_candidates(before=before), [])

    def test_two_bumps_and_explicit_retry_are_independent(self):
        before = self.commit(self.root, "packages")
        self.write_version("minimax", "3.2.2")
        self.write_version("opencode", "4.0.0")
        self.commit(self.root, "packages")
        self.write_inventory("minimax-v3.2.2", "zcode-v3.2.1")
        selected = release_ci.unpublished(release_ci.release_candidates(before=before), self.inventory)
        self.assertEqual([item["tag"] for item in selected], ["opencode-v4.0.0"])
        selected = release_ci.release_candidates(selection="minimax")
        self.assertEqual([item["tag"] for item in selected], ["minimax-v3.2.2"])

    def test_published_version_skips_before_go_or_sdk(self):
        self.write_inventory("minimax-v3.2.1")
        with patch.object(release_ci, "effective_target", side_effect=AssertionError("ran Go")):
            self.assertFalse(release_ci.build_candidate(
                "minimax", "3.2.1", self.root / "missing", "a" * 40, self.inventory, self.root / "release",
            ))

    def test_draft_fails_before_go(self):
        self.write_inventory("minimax-v3.2.1", draft=True)
        with patch.object(release_ci, "effective_target", side_effect=AssertionError("ran Go")):
            with self.assertRaisesRegex(ValueError, "draft already exists"):
                release_ci.build_candidate(
                    "minimax", "3.2.1", self.root / "missing", "a" * 40, self.inventory, self.root / "release",
                )

    def test_decreased_version_fails(self):
        before = self.commit(self.root, "packages")
        self.write_version("zcode", "3.2.0")
        with self.assertRaisesRegex(ValueError, "zcode version decreased"):
            release_ci.release_candidates(before=before)

    def test_sealed_sdk_uses_committed_bytes(self):
        sdk = self.root / "sdk"
        sdk.mkdir()
        contract = sdk / "contract.go"
        contract.write_text("committed SDK\n")
        expected = self.commit(sdk, "contract.go")
        contract.write_text("dirty SDK\n")
        (sdk / "untracked").touch()
        sealed = self.root / "sealed"
        index = self.root / "sdk.json"
        self.assertEqual(release_ci.seal_sdk(sdk, sealed, index), expected)
        self.assertEqual(json.loads(index.read_text())["commit"], expected)
        self.assertEqual((sealed / "contract.go").read_text(), "committed SDK\n")
        self.assertEqual(sdk_inputs(sealed, index)["files"], 1)
