import json
import os
from pathlib import Path
import subprocess
import sys
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from release import package_release
import release_build
import test_release_build


class ReleasePlatformTests(test_release_build.ReleaseBuildTests):
    def setUp(self):
        super().setUp()
        self.build_environments = []
        self.compatibility = None

    def builder(self, command, check, env=None):
        self.build_environments.append(env)
        super().builder(command, check)
        if self.compatibility is not None:
            path = Path(command[command.index("--output") + 1]) / "minimax/magpie-plugin.json"
            manifest = json.loads(path.read_text())
            manifest["compatibility"] = {"platforms": self.compatibility}
            path.write_text(json.dumps(manifest))

    def test_empty_target_environment_records_effective_go_host_and_pins_builder(self):
        with patch.dict(os.environ, {"GOOS": "", "GOARCH": ""}):
            host = json.loads(subprocess.check_output(
                ["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH"], text=True,
            ))
            result = self.build("effective")
        expected = host["GOHOSTOS"] + "/" + host["GOHOSTARCH"]
        self.assertEqual(result.get("platform"), expected, "receipt omitted effective target")
        inputs = json.loads((self.root / "effective/build-inputs.json").read_text())
        self.assertEqual(inputs["platform"], expected)
        environment = self.build_environments[0]
        self.assertIsNotNone(environment, "builder was not pinned to the effective target")
        self.assertEqual((environment["GOOS"], environment["GOARCH"]), (host["GOHOSTOS"], host["GOHOSTARCH"]))
        self.calls.clear()
        with patch.dict(os.environ, {"GOOS": host["GOHOSTOS"], "GOARCH": host["GOHOSTARCH"]}):
            repeated = self.build("explicit-equivalent", previous=self.root / "effective/build-receipt.json")
        self.assertEqual(repeated["status"], "reuse-local")
        self.assertEqual(self.calls, [])

    def test_native_payload_cannot_be_relabeled_for_another_supported_platform(self):
        for case in ("omitted", "multiple"):
            with self.subTest(case=case):
                self.compatibility = None if case == "omitted" else ["darwin/arm64", "linux/amd64"]
                with patch.dict(os.environ, {"GOOS": "darwin", "GOARCH": "arm64"}):
                    self.build(case)
                inputs = self.root / case / "built"
                output = self.root / (case + "-wrong")
                with self.assertRaises(ValueError, msg="Darwin payload was repackaged as Linux"):
                    package_release("minimax", inputs, output, "linux_amd64")
                self.assertFalse(output.exists())
                self.assertTrue(package_release("minimax", inputs, self.root / (case + "-right"), "darwin_arm64"))

    def test_same_version_independent_targets_build_and_reuse_without_source_bump(self):
        with patch.dict(os.environ, {"GOOS": "darwin", "GOARCH": "arm64"}):
            first = self.build("darwin")
        darwin_receipt = self.root / "darwin/build-receipt.json"
        original = darwin_receipt.read_bytes()
        with patch.dict(os.environ, {"GOOS": "linux", "GOARCH": "amd64"}):
            try:
                second = self.build("linux", previous=darwin_receipt)
            except ValueError as error:
                self.fail("same-version new target incorrectly requires a bump: " + str(error))
            linux_receipt = self.root / "linux/build-receipt.json"
            self.calls.clear()
            repeated = self.build("unused-linux", previous=linux_receipt)
        self.assertEqual((first["version"], second["version"]), ("0.1.0", "0.1.0"))
        self.assertEqual((first.get("platform"), second.get("platform")), ("darwin/arm64", "linux/amd64"))
        self.assertEqual(first.get("sourceInputsSHA256"), second.get("sourceInputsSHA256"))
        self.assertEqual(len(first.get("sourceInputsSHA256", "")), 64)
        self.assertEqual(repeated["status"], "reuse-local")
        self.assertEqual(self.calls, [])
        with patch.dict(os.environ, {"GOOS": "darwin", "GOARCH": "arm64"}):
            self.assertEqual(self.build("unused-darwin", previous=darwin_receipt)["status"], "reuse-local")
        self.assertEqual(darwin_receipt.read_bytes(), original)
        self.assertFalse((self.root / "unused-linux").exists())
        self.assertFalse((self.root / "unused-darwin").exists())
        (self.sdk / "pkg/extension/contract.go").write_text("package extension\nvar Changed = true\n")
        self.sdk_index()
        with patch.dict(os.environ, {"GOOS": "linux", "GOARCH": "amd64"}):
            with self.assertRaisesRegex(ValueError, "Same-version inputs changed"):
                self.build("changed-sdk-target", previous=darwin_receipt)
        self.assertEqual(self.calls, [])

    def test_missing_or_malformed_receipt_target_never_produces_archive(self):
        self.build("first")
        path = self.root / "first/build-receipt.json"
        original = path.read_bytes()
        for index, value in enumerate((None, "", "/", "../amd64")):
            with self.subTest(value=value):
                record = json.loads(original)
                if value is None:
                    record.pop("platform", None)
                else:
                    record["platform"] = value
                path.write_text(json.dumps(record))
                output = self.root / ("unknown-target-" + str(index))
                try:
                    with self.assertRaises(ValueError):
                        package_release("minimax", self.root / "first/built", output, "darwin_arm64")
                    self.assertFalse(output.exists())
                finally:
                    path.write_bytes(original)

    def test_published_skip_does_not_query_go_or_call_builder(self):
        self.inventory(["minimax-v0.1.0"])
        with patch.object(release_build.subprocess, "check_output", side_effect=AssertionError("published skip queried go")):
            result = self.build("skip-no-go")
        self.assertEqual(result["status"], "skip-published")
        self.assertEqual(self.calls, [])
        self.assertFalse((self.root / "skip-no-go").exists())
