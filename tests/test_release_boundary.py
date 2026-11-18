import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
from unittest.mock import patch

import test_release_build


class ReleaseBoundaryTests(test_release_build.ReleaseBuildTests):
    def setUp(self):
        super().setUp()
        target = patch.dict(os.environ, {"GOOS": "darwin", "GOARCH": "arm64"})
        target.start()
        self.addCleanup(target.stop)

    def package_result(self, name, inputs=None):
        output = self.root / name
        result = subprocess.run([
            sys.executable, "-B", "-I",
            str(Path(__file__).resolve().parents[1] / "scripts/release.py"),
            "minimax", "--input", str(inputs or self.root / "first/built"),
            "--output", str(output), "--platform", "darwin_arm64",
        ], capture_output=True, text=True, timeout=15)
        return result, output

    def test_release_candidate_packages_and_reuses_exact_payload(self):
        self.build("first")
        receipt = self.root / "first/build-receipt.json"
        before = receipt.read_bytes()
        first, output = self.package_result("release")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertTrue((output / "checksums.txt").is_file())
        repeated, second = self.package_result("release-again")
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(
            {path.name: path.read_bytes() for path in output.iterdir()},
            {path.name: path.read_bytes() for path in second.iterdir()},
        )
        self.calls.clear()
        self.assertEqual(self.build("unused", previous=receipt)["status"], "reuse-local")
        self.assertEqual(self.calls, [])
        self.assertEqual(receipt.read_bytes(), before)

    def test_receipt_deletion_corruption_and_plain_build_fail_closed(self):
        self.build("first")
        receipt = self.root / "first/build-receipt.json"
        original = receipt.read_bytes()
        cases = ("missing", "truncated", "duplicate", "old-schema", "unknown-policy")
        for case in cases:
            with self.subTest(case=case):
                if case == "missing":
                    receipt.unlink()
                elif case == "truncated":
                    receipt.write_text('{"schema":')
                elif case == "duplicate":
                    receipt.write_text('{"schema":2,"publishable":false,"publishable":true}')
                else:
                    record = json.loads(original)
                    if case == "old-schema":
                        record["schema"] = 1
                    else:
                        record.pop("publishable")
                    receipt.write_text(json.dumps(record))
                result, output = self.package_result(case)
                self.assertNotEqual(result.returncode, 0, "unknown receipt promoted to release")
                self.assertFalse(output.exists())
                receipt.write_bytes(original)
        plain = self.root / "plain/minimax"
        shutil.copytree(self.root / "first/built/minimax", plain)
        result, output = self.package_result("plain-output", plain.parent)
        self.assertNotEqual(result.returncode, 0, "plain build.py output has no publishable provenance")
        self.assertFalse(output.exists())

    def test_packager_rechecks_complete_payload_bytes_and_modes(self):
        self.build("first")
        directory = self.root / "first/built/minimax"
        binary = directory / "module.so"
        original, mode = binary.read_bytes(), binary.stat().st_mode & 0o777
        for case in ("extra", "mode", "bytes", "missing", "symlink"):
            with self.subTest(case=case):
                extra = directory / "unindexed"
                if case == "extra":
                    extra.write_bytes(b"unindexed payload")
                elif case == "mode":
                    binary.chmod(mode ^ 0o111)
                elif case == "bytes":
                    binary.write_bytes(b"changed payload")
                elif case == "missing":
                    binary.unlink()
                else:
                    extra.symlink_to(binary)
                try:
                    result, output = self.package_result("changed-" + case)
                    self.assertNotEqual(result.returncode, 0, "changed payload promoted to release")
                    self.assertFalse(output.exists())
                finally:
                    if extra.exists() or extra.is_symlink():
                        extra.unlink()
                    binary.write_bytes(original)
                    binary.chmod(mode)

    def test_manual_dev_cannot_reuse_or_package_after_relocation(self):
        self.build("first", manual=True)
        receipt = self.root / "first/build-receipt.json"
        self.calls.clear()
        with self.assertRaises(ValueError):
            self.build("promoted", previous=receipt)
        self.assertEqual(self.calls, [])
        moved = self.root / "moved"
        shutil.copytree(self.root / "first", moved)
        result, output = self.package_result("moved-release", moved / "built")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(output.exists())
        (moved / "build-receipt.json").unlink()
        result, output = self.package_result("unmarked-dev", moved / "built")
        self.assertNotEqual(result.returncode, 0, "removing provenance promoted manual-dev")
        self.assertFalse(output.exists())

    def test_reuse_rejects_missing_mode_inventory_and_removed_payload(self):
        self.build("first")
        receipt = self.root / "first/build-receipt.json"
        original = receipt.read_bytes()
        self.calls.clear()
        record = json.loads(original)
        record["schema"] = 1
        receipt.write_text(json.dumps(record))
        with self.assertRaises(ValueError):
            self.build("old-schema-reused", previous=receipt)
        receipt.write_bytes(original)
        (self.root / "first/built/minimax/module.so").unlink()
        with self.assertRaises(ValueError):
            self.build("missing-reused", previous=receipt)
        self.assertEqual(self.calls, [])

    def test_receipt_binding_checks_identity_version_output_and_inventory(self):
        self.build("first")
        receipt = self.root / "first/build-receipt.json"
        original = receipt.read_bytes()
        self.calls.clear()
        for case in ("id", "version", "tag", "output", "outputs"):
            with self.subTest(case=case):
                record = json.loads(original)
                record[case] = {} if case == "outputs" else str(self.root / "other") if case == "output" else "other"
                receipt.write_text(json.dumps(record))
                try:
                    result, output = self.package_result("bad-binding-" + case)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse(output.exists())
                    if case != "version":
                        with self.assertRaises(ValueError):
                            self.build("bad-reuse-" + case, previous=receipt)
                    self.assertEqual(self.calls, [])
                finally:
                    receipt.write_bytes(original)
