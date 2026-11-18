from contextlib import redirect_stderr, redirect_stdout
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import _workspace
import build as workspace_build
import check as workspace_check


class BuildSelectionTests(unittest.TestCase):
    def setUp(self):
        self.entries = {
            "claude-desktop": {"id": "agent-claude-desktop", "platforms": ["darwin/arm64", "darwin/amd64"], "checks": []},
            "minimax": {"id": "minimax", "checks": []},
            "devin": {"id": "devin-inbound", "checks": []},
        }

    def test_all_linux_skips_only_platform_restricted_package_without_mutating_registry(self):
        before = json.dumps(self.entries, sort_keys=True)
        selected, skipped = _workspace.select_build_packages(self.entries, "all", "linux/amd64")
        self.assertEqual(list(selected), ["minimax", "devin"])
        self.assertEqual(list(skipped), ["claude-desktop"])
        self.assertIs(selected["minimax"], self.entries["minimax"])
        self.assertEqual(json.dumps(self.entries, sort_keys=True), before)

    def test_both_declared_mac_targets_keep_the_existing_registry_order(self):
        for target in ("darwin/arm64", "darwin/amd64"):
            with self.subTest(target=target):
                selected, skipped = _workspace.select_build_packages(self.entries, "all", target)
                self.assertEqual(list(selected), list(self.entries))
                self.assertEqual(skipped, {})

    def test_explicit_unsupported_package_is_an_error_not_an_empty_success(self):
        with self.assertRaisesRegex(ValueError, r"claude-desktop.*linux/amd64"):
            _workspace.select_build_packages(self.entries, "claude-desktop", "linux/amd64")
        selected, skipped = _workspace.select_build_packages(self.entries, "minimax", "linux/amd64")
        self.assertEqual(list(selected), ["minimax"])
        self.assertEqual(skipped, {})

    def test_missing_platform_restriction_keeps_previous_target_behavior(self):
        for target in ("linux/arm64", "windows/amd64", "freebsd/amd64"):
            with self.subTest(target=target):
                selected, skipped = _workspace.select_build_packages(self.entries, "minimax", target)
                self.assertEqual(list(selected), ["minimax"])
                self.assertEqual(skipped, {})

    def test_unknown_selection_and_invalid_platform_metadata_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "Unknown package"):
            _workspace.select_build_packages(self.entries, "typo", "linux/amd64")
        for invalid in (None, [], "darwin/arm64", ["darwin_arm64"], [1], ["darwin/arm64", "darwin/arm64"]):
            with self.subTest(invalid=invalid):
                entries = {"restricted": {"id": "fixture", "platforms": invalid}}
                with self.assertRaisesRegex(ValueError, "platform"):
                    _workspace.select_build_packages(entries, "all", "linux/amd64")
        for target in ("linux_amd64", "../linux/amd64", "", "linux/AMD64"):
            with self.subTest(target=target):
                with self.assertRaisesRegex(ValueError, "target"):
                    _workspace.select_build_packages(self.entries, "all", target)

    def test_registered_platforms_do_not_filter_package_or_check_discovery(self):
        entries = _workspace.packages()
        self.assertEqual(entries["claude-desktop"]["platforms"], ["darwin/arm64", "darwin/amd64"])
        self.assertEqual(list(_workspace.packages("claude-desktop")), ["claude-desktop"])
        self.assertIn("claude-desktop", entries)

    def test_effective_target_uses_offline_go_environment_and_pins_the_builder(self):
        with patch.dict(os.environ, {"GOOS": "", "GOARCH": "", "GOTOOLCHAIN": "auto"}):
            with patch.object(_workspace.subprocess, "check_output", return_value='{"GOOS":"linux","GOARCH":"amd64"}') as query:
                target, environment = _workspace.effective_target(Path("/offline/go"), Path("/fixture/package"))
        self.assertEqual(target, "linux/amd64")
        self.assertEqual((environment["GOOS"], environment["GOARCH"]), ("linux", "amd64"))
        self.assertEqual(environment["GOWORK"], "off")
        self.assertEqual(environment["GOTOOLCHAIN"], "local")
        self.assertEqual(environment["GOPROXY"], "off")
        self.assertEqual(environment["GOSUMDB"], "off")
        self.assertEqual(query.call_args.args[0], ["/offline/go", "env", "-json", "GOOS", "GOARCH"])
        self.assertEqual(query.call_args.kwargs["cwd"], Path("/fixture/package/src"))

    def test_effective_target_rejects_unknown_or_malformed_go_output(self):
        for output in ('{}', '{"GOOS":"linux"}', '{"GOOS":"../linux","GOARCH":"amd64"}',
                       '{"GOOS":"linux","GOARCH":1}', '{"GOOS":"linux","GOARCH":""}'):
            with self.subTest(output=output):
                with patch.object(_workspace.subprocess, "check_output", return_value=output):
                    with self.assertRaisesRegex(ValueError, "effective build target"):
                        _workspace.effective_target(Path("/offline/go"), Path("/fixture/package"))


class BuildCommandSelectionTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="build-selection-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        self.entries = {
            "claude-desktop": {"id": "agent-claude-desktop", "platforms": ["darwin/arm64", "darwin/amd64"], "checks": []},
            "minimax": {"id": "minimax", "checks": []},
        }
        self.target = ("linux/amd64", {"GOOS": "linux", "GOARCH": "amd64"})
        for attribute, value in (("ROOT", self.root), ("packages", None), ("sdk_source", None),
                                 ("effective_target", None), ("validate_distribution", None)):
            replacement = patch.object(workspace_build, attribute, value) if attribute == "ROOT" else patch.object(workspace_build, attribute)
            mocked = replacement.start()
            self.addCleanup(replacement.stop)
            if attribute != "ROOT":
                setattr(self, attribute, mocked)
        self.packages.return_value = self.entries
        self.sdk_source.return_value = self.root / "sdk"
        self.effective_target.return_value = self.target
        self.validate_distribution.return_value = {"version": "0.1.0"}
        runner = patch.object(workspace_build.subprocess, "run")
        self.runner = runner.start()
        self.addCleanup(runner.stop)

    def run_command(self, selection):
        stdout, stderr = io.StringIO(), io.StringIO()
        arguments = ["build.py", selection, "--sdk", str(self.root / "sdk"), "--output", str(self.root / "output")]
        with patch.object(sys, "argv", arguments), redirect_stdout(stdout), redirect_stderr(stderr):
            workspace_build.main()
        return stdout.getvalue(), stderr.getvalue()

    def test_all_skips_unsupported_before_output_collision_and_invokes_only_supported_builder(self):
        stale = self.root / "output/claude-desktop"
        stale.mkdir(parents=True)
        (stale / "preserved").write_text("untouched")
        stdout, stderr = self.run_command("all")
        self.assertIn("Skipping claude-desktop", stderr)
        self.assertIn("linux/amd64", stderr)
        self.assertIn('"package": "minimax"', stdout)
        self.assertEqual(self.runner.call_count, 1)
        self.assertEqual(self.runner.call_args.args[0][1], str(self.root / "packages/minimax/scripts/build.py"))
        self.assertEqual(self.runner.call_args.kwargs["env"], self.target[1])
        self.assertEqual((stale / "preserved").read_text(), "untouched")
        self.validate_distribution.assert_called_once_with(self.root / "output/minimax", "minimax")

    def test_explicit_unsupported_refuses_before_sdk_output_or_builder(self):
        with self.assertRaises(SystemExit) as failure:
            self.run_command("claude-desktop")
        self.assertEqual(failure.exception.code, 2)
        self.sdk_source.assert_not_called()
        self.runner.assert_not_called()
        self.validate_distribution.assert_not_called()
        self.assertFalse((self.root / "output").exists())

    def test_all_with_no_supported_package_does_not_read_sdk_or_create_output(self):
        self.entries.pop("minimax")
        stdout, stderr = self.run_command("all")
        self.assertEqual(stdout, "")
        self.assertIn("Skipping claude-desktop", stderr)
        self.sdk_source.assert_not_called()
        self.runner.assert_not_called()
        self.assertFalse((self.root / "output").exists())

    def test_explicit_supported_package_receives_the_effective_target(self):
        self.effective_target.return_value = ("darwin/arm64", {"GOOS": "darwin", "GOARCH": "arm64"})
        stdout, stderr = self.run_command("claude-desktop")
        self.assertEqual(stderr, "")
        self.assertIn('"package": "claude-desktop"', stdout)
        self.assertEqual(self.runner.call_count, 1)
        self.assertEqual(self.runner.call_args.kwargs["env"], {"GOOS": "darwin", "GOARCH": "arm64"})

    def test_linux_planner_unit_checks_are_not_filtered_by_distribution_platforms(self):
        entry = dict(self.entries["claude-desktop"], checks=[{"command": ["{python}", "-I", "tests/provenance_test.py"]}])
        stdout = io.StringIO()
        with patch.object(workspace_check, "packages", return_value={"claude-desktop": entry}), \
                patch.object(workspace_check, "ROOT", self.root), \
                patch.object(workspace_check, "sdk_source", return_value=self.root / "sdk"), \
                patch.object(workspace_check, "build_environment") as environment, \
                patch.object(workspace_check.subprocess, "check_output", return_value="") as query, \
                patch.object(workspace_check.subprocess, "run") as runner, \
                patch.object(sys, "argv", ["check.py", "claude-desktop", "--sdk", str(self.root / "sdk")]), \
                patch.dict(os.environ, {"GOOS": "linux", "GOARCH": "amd64"}), redirect_stdout(stdout):
            environment.return_value.__enter__.return_value = ({"GOOS": "linux", "GOARCH": "amd64"}, self.root / "sdk")
            workspace_check.main()
        commands = [call.args[0] for call in runner.call_args_list]
        self.assertIn(["go", "vet", "./..."], commands)
        self.assertTrue(any(command[:2] == ["go", "test"] for command in commands))
        self.assertIn([sys.executable, "-I", "tests/provenance_test.py"], commands)
        self.assertIn("Checking claude-desktop", stdout.getvalue())
        self.assertTrue(query.called)


if __name__ == "__main__":
    unittest.main()
