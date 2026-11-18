import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

import build


class DefaultDistributionProvenanceTests(unittest.TestCase):
    def test_default_build_packages_the_declared_identity_and_preserves_sources(self):
        package = Path(__file__).resolve().parents[1]
        module = package / "src"
        declaration = json.loads((module / "magpie-plugin.json").read_text())
        repository_lock = (package / "docs/source-lock.json").read_bytes()
        original = json.loads(repository_lock)
        platform = json.dumps({"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1", "GOVERSION": "go1.26.8"})
        factory = dict(declaration)
        factory["compatibility"] = dict(declaration["compatibility"], platforms=["linux/amd64"])
        def output(command, **_):
            return platform if command[1] == "env" else json.dumps(factory)
        def compile_native(command, **_):
            Path(command[command.index("-o") + 1]).write_bytes(b"synthetic native library")
        with tempfile.TemporaryDirectory() as temporary:
            destination = Path(temporary) / "distribution"
            sdk = Path(temporary) / "sdk"
            (sdk / "pkg/extension").mkdir(parents=True)
            (sdk / "pkg/extension/contract.go").write_text("package extension\n")
            with patch("build.subprocess.check_output", side_effect=output), \
                    patch("build.snapshot_build_inputs", return_value={"files": {}, "modules": {}, "sha256": "fixture"}), \
                    patch("build.subprocess.run", side_effect=compile_native) as run:
                build.build(destination, sdk)
            command = run.call_args.args[0]
            self.assertEqual(command[-1], "./cmd/native")
            self.assertNotIn("magpie_source16_preview", command)
            manifest = json.loads((destination / "magpie-plugin.json").read_text())
            self.assertTrue((destination / "source-lock.json").is_file(), "native distribution omitted source provenance")
            lock = json.loads((destination / "source-lock.json").read_text())
            self.assertEqual(manifest["contract"], declaration["contract"])
            self.assertEqual(manifest["id"], "omp-sessions")
            self.assertEqual(manifest["version"], "0.1.0")
            self.assertEqual(manifest["compatibility"]["hostAPI"], "^1.10",
                             "ordinary package omitted the full Source reader")
            self.assertEqual(lock["plugin"], {
                "id": manifest["id"], "version": manifest["version"],
                "sdkMinimum": "1.10", "nativeABI": manifest["compatibility"]["nativeABI"],
            })
            self.assertIn("session-source", manifest["contract"]["capabilities"])
            self.assertEqual(manifest["sessionSources"][0]["id"], "omp-history")
            self.assertEqual(lock["sources"], original["sources"])
            self.assertEqual(lock["artifacts"], original["artifacts"])
            self.assertEqual(lock["acceptance"], original["acceptance"])
            self.assertEqual((destination / "SOURCES.md").read_bytes(), (package / "docs/SOURCES.md").read_bytes())
        self.assertEqual((package / "docs/source-lock.json").read_bytes(), repository_lock)


if __name__ == "__main__":
    unittest.main()
