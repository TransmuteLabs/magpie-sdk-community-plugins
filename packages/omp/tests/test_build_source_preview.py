import json
from pathlib import Path
import shlex
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

import build
import build_source_preview


class SourceDistributionTests(unittest.TestCase):
    def factory(self):
        return json.loads((Path(__file__).resolve().parents[1] / "src/magpie-plugin.json").read_text())

    def sdk(self, root):
        sdk = root / "sdk"
        (sdk / "pkg/extension").mkdir(parents=True)
        (sdk / "pkg/extension/contract.go").write_text("package extension\n")
        return sdk

    def responses(self, manifest):
        def output(command, **_):
            if command[1] == "env":
                return json.dumps({"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1"})
            self.assertEqual(command[1], "run")
            self.assertNotIn("-tags", command)
            self.assertEqual(command[-1], "./cmd/native")
            return json.dumps(manifest)
        return output

    def test_legacy_builder_is_the_same_complete_distribution(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            sdk = self.sdk(root)
            manifest = self.factory()

            def compile_native(command, **_):
                if command[1] == "build":
                    Path(command[command.index("-o") + 1]).write_bytes(b"synthetic native library")

            with patch("build.subprocess.check_output", side_effect=self.responses(manifest)), \
                    patch("build.snapshot_build_inputs", return_value={"files": {}, "modules": {}, "sha256": "fixture"}), \
                    patch("build.subprocess.run", side_effect=compile_native) as run:
                build_source_preview.build(root / "dist", sdk)
            self.assertEqual(json.loads((root / "dist/magpie-plugin.json").read_text()), manifest)
            lock = json.loads((root / "dist/source-lock.json").read_text())
            self.assertEqual(lock["plugin"]["sdkMinimum"], "1.10")
            command = run.call_args.args[0]
            self.assertNotIn("-tags", command)
            self.assertEqual(command[-1], "./cmd/native")
            flags = shlex.split(run.call_args.kwargs["env"]["GOFLAGS"])
            modfile = Path(next(flag.split("=", 1)[1] for flag in flags if flag.startswith("-modfile=")))
            self.assertFalse(modfile.exists(), "temporary SDK override was retained")

    def test_partial_factory_is_refused_before_publication(self):
        for missing in ("sessionSources", "sessions"):
            with self.subTest(missing=missing), tempfile.TemporaryDirectory() as temporary:
                destination = Path(temporary) / "dist"
                sdk = self.sdk(Path(temporary))
                manifest = self.factory()
                del manifest[missing]
                with patch("build.subprocess.check_output", side_effect=self.responses(manifest)), \
                        patch("build.snapshot_build_inputs", return_value={}), \
                        patch("build.subprocess.run") as run:
                    with self.assertRaisesRegex(ValueError, "full API 1.10"):
                        build.build(destination, sdk)
                self.assertFalse(destination.exists())
                run.assert_not_called()

    def test_wrong_sdk_floor_and_escaping_entry_are_refused(self):
        for invalid in ("floor", "entry"):
            with self.subTest(invalid=invalid), tempfile.TemporaryDirectory() as temporary:
                destination = Path(temporary) / "dist"
                sdk = self.sdk(Path(temporary))
                manifest = self.factory()
                if invalid == "floor":
                    manifest["contract"]["version"]["minor"] = 3
                else:
                    manifest["entry"] = "../plugin.so"
                with patch("build.subprocess.check_output", side_effect=self.responses(manifest)), \
                        patch("build.snapshot_build_inputs", return_value={}), \
                        patch("build.subprocess.run") as run:
                    with self.assertRaises(ValueError):
                        build.build(destination, sdk)
                self.assertFalse(destination.exists())
                run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
