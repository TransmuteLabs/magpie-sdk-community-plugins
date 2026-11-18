import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from _workspace import packages
from release import package_release, payload_inventory


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="module-release-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.inputs = self.root / "built"
        self.before = {}
        for index, (name, spec) in enumerate(packages().items()):
            directory = self.inputs / name
            (directory / "bin").mkdir(parents=True)
            manifest = {
                "id": spec["id"], "version": f"{index + 1}.0.0", "runtime": "native",
                "entry": "bin/module.so", "contract": {"version": {"major": 1, "minor": 12}},
            }
            (directory / "magpie-plugin.json").write_text(json.dumps(manifest))
            (directory / "source-lock.json").write_text(json.dumps({"plugin": {
                "id": spec["id"], "version": manifest["version"], "sdkMinimum": "1.12",
            }}))
            (directory / "bin/module.so").write_bytes(b"synthetic module; never execute")
            (directory / "bin/module.so").chmod(0o755)
        self.before = self.inventory()
        for name in packages():
            self.receipt(name)

    def receipt(self, name, platform="darwin/arm64"):
        directory = self.inputs / name
        manifest = json.loads((directory / "magpie-plugin.json").read_text())
        path = self.root / (name + "-receipt.json")
        path.write_text(json.dumps({
            "schema": 2, "status": "built-release-candidate", "publishable": True,
            "id": manifest["id"], "version": manifest["version"],
            "tag": manifest["id"] + "-v" + manifest["version"],
            "platform": platform,
            "output": str(self.root), "outputs": payload_inventory(directory, name),
        }))
        return path

    def inventory(self):
        return {
            path.relative_to(self.inputs).as_posix(): (path.read_bytes(), stat.S_IMODE(path.stat().st_mode))
            for path in self.inputs.rglob("*") if path.is_file()
        }

    def test_independent_assets_checksums_and_determinism(self):
        first, second = self.root / "release-a", self.root / "release-b"
        result = []
        for name in packages():
            result.extend(package_release(name, self.inputs, first / name, "darwin_arm64", self.root / (name + "-receipt.json")))
        for path in self.inputs.rglob("*"):
            os.utime(path, (1800000000, 1800000000))
        repeated = []
        for name in packages():
            repeated.extend(package_release(name, self.inputs, second / name, "darwin_arm64", self.root / (name + "-receipt.json")))
        self.assertEqual(result, repeated)
        self.assertEqual(len(result), len(packages()))
        self.assertEqual(len({item["version"] for item in result}), len(packages()))
        for item in result:
            asset = first / item["package"] / item["asset"]
            self.assertEqual(asset.read_bytes(), (second / item["package"] / item["asset"]).read_bytes())
            self.assertEqual(hashlib.sha256(asset.read_bytes()).hexdigest(), item["sha256"])
            self.assertEqual(item["asset"], f'magpie-plugin_{item["id"]}__darwin_arm64.zip')
            self.assertEqual(item["tag"], item["id"] + "-v" + item["version"])
            with zipfile.ZipFile(asset) as archive:
                self.assertIsNone(archive.testzip())
                self.assertEqual(archive.namelist(), sorted(archive.namelist()))
                manifest = json.loads(archive.read("magpie-plugin.json"))
                self.assertEqual((manifest["id"], manifest["version"]), (item["id"], item["version"]))
                self.assertEqual(stat.S_IMODE(archive.getinfo("bin/module.so").external_attr >> 16), 0o755)
                for entry in archive.infolist():
                    self.assertEqual(entry.date_time, (1980, 1, 1, 0, 0, 0))
                    self.assertEqual(archive.read(entry), (self.inputs / item["package"] / entry.filename).read_bytes())
            expected = item["sha256"] + "  " + item["asset"] + "\n"
            self.assertEqual((first / item["package"] / "checksums.txt").read_text(), expected)
            self.assertEqual((first / item["package"] / "checksums.txt").read_bytes(),
                             (second / item["package"] / "checksums.txt").read_bytes())
        self.assertFalse((first / "checksums.txt").exists())
        self.assertEqual(self.inventory(), self.before)
        with self.assertRaisesRegex(ValueError, "exactly one"):
            package_release("all", self.inputs, self.root / "all", "linux_amd64")

    def test_scoped_selection_and_existing_output_refusal(self):
        output = self.root / "selected"
        receipt = self.receipt("minimax", "linux/amd64")
        result = package_release("minimax", self.inputs, output, "linux_amd64", receipt)
        self.assertEqual([item["id"] for item in result], ["minimax"])
        self.assertEqual(len(list(output.iterdir())), 2)
        before = {path.name: path.read_bytes() for path in output.iterdir()}
        with self.assertRaisesRegex(ValueError, "already exists"):
            package_release("minimax", self.inputs, output, "linux_amd64", receipt)
        self.assertEqual({path.name: path.read_bytes() for path in output.iterdir()}, before)
        self.assertEqual(self.inventory(), self.before)

    def test_identity_version_and_path_failures_publish_nothing(self):
        for case in ("id", "version", "symlink", "platform", "inside", "any-native"):
            with self.subTest(case=case):
                directory = self.inputs / "minimax"
                manifest_path = directory / "magpie-plugin.json"
                original = manifest_path.read_bytes()
                extra = None
                try:
                    if case in ("id", "version"):
                        manifest = json.loads(original)
                        manifest[case] = "other" if case == "id" else "99.0.0"
                        manifest_path.write_text(json.dumps(manifest))
                    elif case == "symlink":
                        extra = directory / "link"
                        extra.symlink_to(self.root)
                    output = self.inputs / "unsafe" if case == "inside" else self.root / ("bad-" + case)
                    platform = "../darwin_arm64" if case == "platform" else "any" if case == "any-native" else "darwin_arm64"
                    with self.assertRaises(ValueError):
                        package_release("minimax", self.inputs, output, platform, self.root / "minimax-receipt.json")
                    self.assertFalse(output.exists())
                finally:
                    manifest_path.write_bytes(original)
                    if extra is not None:
                        if extra.is_symlink():
                            extra.unlink()
                        else:
                            extra.rmdir()
        self.assertEqual(self.inventory(), self.before)

    def test_isolated_cli_and_universal_bun(self):
        directory = self.inputs / "minimax"
        path = directory / "magpie-plugin.json"
        manifest = json.loads(path.read_text())
        manifest["runtime"] = "bun"
        path.write_text(json.dumps(manifest))
        receipt = self.receipt("minimax")
        output = self.root / "bun"
        script = Path(__file__).resolve().parents[1] / "scripts/release.py"
        result = subprocess.run([
            sys.executable, "-I", str(script), "minimax", "--input", str(self.inputs),
            "--output", str(output), "--platform", "any", "--receipt", str(receipt),
        ], capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["asset"], "magpie-plugin_minimax__any.zip")
        manifest["services"] = [{"runtime": "exec"}]
        path.write_text(json.dumps(manifest))
        self.receipt("minimax")
        with self.assertRaisesRegex(ValueError, "every component"):
            package_release("minimax", self.inputs, self.root / "bad-service", "any", receipt)


if __name__ == "__main__":
    unittest.main()
