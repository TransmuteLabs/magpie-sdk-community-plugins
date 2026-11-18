import hashlib
import json
import os
from pathlib import Path
import shlex
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

import build
import build_source_preview


class BuildIdentityTests(unittest.TestCase):
    def fixture(self, root, source=False, mutate=False):
        module = Path(__file__).resolve().parents[1] / "src"
        sdk = root / "sdk"
        package = sdk / "pkg" / "extension"
        package.mkdir(parents=True)
        (package / "contract.go").write_text("package extension\n")
        (sdk / "go.mod").write_text("module github.com/yetone/magpie\n\ngo 1.26.8\n")
        sdk_input = package / "sdk.go"
        sdk_input.write_text('package extension\nconst Fixture = "actual SDK input"\n')
        manifest = json.loads((module / "magpie-plugin.json").read_text())
        manifest["version"] = "0.2.0"
        manifest["compatibility"]["platforms"] = ["linux/amd64"]
        if source:
            manifest["contract"]["version"]["minor"] = 10
            if "session-source" not in manifest["contract"]["capabilities"]:
                manifest["contract"]["capabilities"].append("session-source")
            manifest["compatibility"]["hostAPI"] = "^1.10"
            manifest["sessionSources"] = [{"id": "omp-history"}]
        packages = [
            {
                "ImportPath": "github.com/yetone/magpie/pkg/extension",
                "Dir": str(package), "GoFiles": ["sdk.go"],
                "Module": {
                    "Path": "github.com/yetone/magpie", "Version": "v0.0.0",
                    "Replace": {"Path": str(sdk), "Dir": str(sdk)},
                },
            },
            {
                "ImportPath": "github.com/TransmuteLabs/magpie-sdk-plugin-session-omp",
                "Dir": str(module), "GoFiles": ["manifest.go"],
                "EmbedFiles": ["magpie-plugin.json"],
                "Module": {
                    "Path": "github.com/TransmuteLabs/magpie-sdk-plugin-session-omp",
                    "Dir": str(module), "Main": True,
                },
            },
        ]

        def output(command, **_):
            if command[1] == "env":
                return json.dumps({
                    "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1",
                    "GOVERSION": "go1.26.8",
                })
            if command[1] == "list":
                return "\n".join(json.dumps(package) for package in packages)
            if command[1] == "run":
                return json.dumps(manifest)
            self.fail("unexpected compiler query: " + repr(command))

        def run(command, **_):
            if command[1] == "build":
                binary = Path(command[command.index("-o") + 1])
                binary.write_bytes(b"synthetic native library")
                if mutate:
                    sdk_input.write_text("package extension\nconst Fixture = \"changed\"\n")

        return sdk, sdk_input, manifest, output, run

    def test_default_distribution_uses_the_compiled_factory_identity(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            sdk, _, manifest, output, run = self.fixture(root)
            with patch.dict(os.environ, {"MAGPIE_SDK_SOURCE": str(sdk)}), \
                    patch("build.subprocess.check_output", side_effect=output), \
                    patch("build.subprocess.run", side_effect=run):
                build.build(root / "dist")
            actual = json.loads((root / "dist/magpie-plugin.json").read_text())
            self.assertEqual(actual["version"], manifest["version"],
                             "packaged identity came from a stale static manifest")

    def test_distribution_records_actual_sdk_inputs_and_native_artifact(self):
        for source in (False, True):
            with self.subTest(source=source), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                sdk, sdk_input, _, output, run = self.fixture(root, source=source)
                with patch("build.subprocess.check_output", side_effect=output), \
                        patch("build.subprocess.run", side_effect=run):
                    if source:
                        build_source_preview.build(root / "dist", sdk)
                    else:
                        build.build(root / "dist", sdk)
                destination = root / "dist"
                evidence_path = destination / "build-provenance.json"
                self.assertTrue(evidence_path.is_file(),
                                "distribution omitted the actual SDK/build identity")
                evidence = json.loads(evidence_path.read_text())
                sdk_key = "github.com/yetone/magpie/pkg/extension/sdk.go"
                self.assertEqual(evidence["inputs"]["files"][sdk_key],
                                 hashlib.sha256(sdk_input.read_bytes()).hexdigest())
                self.assertEqual(evidence["toolchain"]["GOVERSION"], "go1.26.8")
                self.assertEqual(evidence["entry"]["sha256"],
                                 hashlib.sha256((destination / "plugin.so").read_bytes()).hexdigest())
                self.assertEqual(evidence["manifestSHA256"],
                                 hashlib.sha256((destination / "magpie-plugin.json").read_bytes()).hexdigest())
                workspace_helper = Path(__file__).resolve().parents[3] / "scripts/_workspace.py"
                self.assertEqual(evidence["inputs"]["files"]["workspace/scripts/_workspace.py"],
                                 hashlib.sha256(workspace_helper.read_bytes()).hexdigest())
                self.assertNotIn(str(root), json.dumps(evidence),
                                 "build identity exposed a machine-specific source location")

    def test_changed_sdk_during_build_cannot_publish_a_manifest(self):
        for source in (False, True):
            with self.subTest(source=source), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                sdk, _, _, output, run = self.fixture(root, source=source, mutate=True)
                with patch("build.subprocess.check_output", side_effect=output), \
                        patch("build.subprocess.run", side_effect=run):
                    with self.assertRaisesRegex(ValueError, "build inputs changed"):
                        if source:
                            build_source_preview.build(root / "dist", sdk)
                        else:
                            build.build(root / "dist", sdk)
                self.assertFalse((root / "dist/magpie-plugin.json").exists(),
                                 "a mixed-source package became installable")

    def test_alternate_sdk_build_retains_locked_dependency_checksums(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            sdk, _, _, output, run = self.fixture(root)
            expected = (Path(__file__).resolve().parents[1] / "src/go.sum").read_bytes()

            def checked_inputs(go_command, module_directory, arguments, target, env=None):
                flags = shlex.split(env["GOFLAGS"])
                modfile = Path(next(flag.split("=", 1)[1] for flag in flags if flag.startswith("-modfile=")))
                checksums = modfile.with_suffix(".sum")
                self.assertTrue(checksums.is_file(),
                                "alternate SDK build omitted pinned dependency checksums")
                self.assertEqual(checksums.read_bytes(), expected,
                                 "alternate SDK build changed pinned dependency checksums")
                self.assertIn(str(sdk), modfile.read_text())
                self.assertEqual(Path(module_directory), Path(__file__).resolve().parents[1] / "src")
                return {"files": {}, "modules": {}, "sha256": "fixture"}

            with patch("build.subprocess.check_output", side_effect=output), \
                    patch("build.subprocess.run", side_effect=run), \
                    patch("build.snapshot_build_inputs", side_effect=checked_inputs):
                build.build(root / "dist", sdk)

    def test_existing_distribution_and_local_files_are_preserved(self):
        for source in (False, True):
            with self.subTest(source=source), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                sdk, _, _, output, run = self.fixture(root, source=source)
                destination = root / "dist"
                destination.mkdir()
                original = {"plugin.so": b"previous library",
                            "magpie-plugin.json": b"previous manifest",
                            "local-note.txt": b"untracked local work"}
                for name, body in original.items():
                    (destination / name).write_bytes(body)
                with patch("build.subprocess.check_output", side_effect=output), \
                        patch("build.subprocess.run", side_effect=run):
                    with self.assertRaisesRegex(ValueError, "destination must be empty"):
                        if source:
                            build_source_preview.build(destination, sdk)
                        else:
                            build.build(destination, sdk)
                self.assertEqual({path.name: path.read_bytes() for path in destination.iterdir()},
                                 original, "packaging overwrote an existing distribution or local work")


if __name__ == "__main__":
    unittest.main()
