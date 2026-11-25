import json
import os
import shlex
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from _workspace import build_environment, packages, validate_distribution


class WorkspaceTests(unittest.TestCase):
    def test_native_build_is_independent_of_unborn_git_metadata(self):
        with tempfile.TemporaryDirectory(prefix="native vcs fixture ") as temporary:
            root = Path(temporary)
            repository, sdk = root / "repository", root / "SDK with spaces"
            package = repository / "packages/native"
            source = package / "src"
            source.mkdir(parents=True)
            (sdk / "pkg/extension").mkdir(parents=True)
            (sdk / "pkg/extension/contract.go").write_text("package extension\nconst Value = 7\n")
            (sdk / "go.mod").write_text("module github.com/yetone/magpie\n\ngo 1.26.8\n")
            original = {
                "go.mod": "module example.com/nativefixture\n\ngo 1.26.8\n\n"
                          "require github.com/yetone/magpie v0.0.0\n"
                          "replace github.com/yetone/magpie => ../wrong-sdk\n",
                "go.sum": "",
                "main.go": 'package main\nimport "C"\n'
                           'import "github.com/yetone/magpie/pkg/extension"\n'
                           "var value = extension.Value\nfunc main() {}\n",
            }
            for name, contents in original.items():
                (source / name).write_text(contents)

            def run(arguments, cwd, env):
                process = subprocess.Popen(arguments, cwd=cwd, env=env, text=True,
                                           stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                           start_new_session=True)
                try:
                    stdout, stderr = process.communicate(timeout=60)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGTERM)
                    try:
                        process.communicate(timeout=10)
                    except subprocess.TimeoutExpired:
                        os.killpg(process.pid, signal.SIGKILL)
                        process.communicate()
                    raise
                return subprocess.CompletedProcess(arguments, process.returncode, stdout, stderr)

            environment = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
            initialized = run(["git", "init", "-q"], repository, environment)
            self.assertEqual(initialized.returncode, 0, initialized.stderr)
            exclude = repository / ".git/info/exclude"
            exclude.write_text("*\n")
            status = run(["git", "status", "--porcelain"], repository, environment)
            self.assertEqual(status.returncode, 0, status.stderr)
            self.assertEqual(status.stdout, "")
            with patch.object(tempfile, "tempdir", str(root)):
                with build_environment(package, sdk) as (env, selected):
                    env.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
                    flags = shlex.split(env["GOFLAGS"])
                    self.assertIn("-mod=readonly", flags)
                    modfile = Path(next(flag.removeprefix("-modfile=") for flag in flags
                                        if flag.startswith("-modfile=")))
                    self.assertTrue(modfile.is_file())
                    self.assertNotEqual(modfile, source / "go.mod")
                    resolved = run(["go", "list", "-m", "-f", "{{.Dir}}",
                                    "github.com/yetone/magpie"], source, env)
                    self.assertEqual(resolved.returncode, 0, resolved.stderr)
                    self.assertEqual(Path(resolved.stdout.strip()), selected)
                    output = root / "native.dylib"
                    command = ["go", "build", "-trimpath", "-buildmode=c-shared",
                               "-o", str(output), "."]
                    ignored = run(command, source, env)
                    self.assertEqual(ignored.returncode, 0, ignored.stderr)
                    library = output.read_bytes()
                    exclude.write_text("")
                    visible = run(command, source, env)
                    self.assertEqual(visible.returncode, 0, visible.stderr)
                    self.assertEqual(output.read_bytes(), library)
                    metadata = run(["go", "version", "-m", str(output)], source, env)
                    self.assertEqual(metadata.returncode, 0, metadata.stderr)
                    self.assertNotIn("\tbuild\tvcs", metadata.stdout)
                    modfile.write_text(modfile.read_text().replace(
                        "require github.com/yetone/magpie v0.0.0\n", ""))
                    missing = run(command, source, env)
                    self.assertNotEqual(missing.returncode, 0)
                    self.assertIn("replaced but not required", missing.stderr)
            self.assertFalse(modfile.exists())
            self.assertEqual({name: (source / name).read_text() for name in original}, original)

    def test_donor_fixtures_keep_exact_bytes_with_autocrlf_checkout(self):
        root = Path(__file__).resolve().parents[1]
        prefix = Path("packages/opencode/src/testdata/donors")
        names = ["zen-tools.json", "history-schema.sql", "history-legacy-roots.go.txt"]
        with tempfile.TemporaryDirectory(prefix="magpie-donor-checkout-") as temporary:
            checkout = Path(temporary)
            env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)

            def git(*args):
                return subprocess.check_output(["git", "-c", "core.autocrlf=true", *args],
                                               cwd=checkout, env=env, stderr=subprocess.STDOUT)

            git("init", "-q")
            if (root / ".gitattributes").is_file():
                (checkout / ".gitattributes").write_bytes((root / ".gitattributes").read_bytes())
            (checkout / prefix).mkdir(parents=True)
            for name in names:
                (checkout / prefix / name).write_bytes((root / prefix / name).read_bytes())
            git("add", ".")
            for name in names:
                (checkout / prefix / name).unlink()
            git("checkout-index", "--all", "--force")
            for name in names:
                with self.subTest(name=name):
                    self.assertEqual((checkout / prefix / name).read_bytes(), (root / prefix / name).read_bytes())
            attributes = git("check-attr", "whitespace", "--", str(prefix / names[1]), str(prefix / names[2]))
            self.assertIn(b"whitespace: -blank-at-eol,-blank-at-eof", attributes)
            self.assertIn(b"whitespace: -blank-at-eof", attributes)

    def test_author_commands_are_available_with_isolated_python(self):
        root = Path(__file__).resolve().parents[1]
        for script in ("build.py", "check.py"):
            result = subprocess.run([sys.executable, "-I", str(root / "scripts" / script), "--help"],
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("--sdk", result.stdout)

    def test_sdk_selection_with_spaces_is_real_go_input_without_source_edits(self):
        with tempfile.TemporaryDirectory(prefix="sdk workspace ") as temporary:
            root = Path(temporary)
            package, sdk = root / "package", root / "SDK with spaces"
            source = package / "src"
            source.mkdir(parents=True)
            (sdk / "pkg/extension").mkdir(parents=True)
            (sdk / "pkg/extension/contract.go").write_text("package extension\n")
            (sdk / "go.mod").write_text("module github.com/yetone/magpie\n\ngo 1.26.8\n")
            original = "module example.com/fixture\n\ngo 1.26.8\n\nrequire github.com/yetone/magpie v0.0.0\nreplace github.com/yetone/magpie => ../wrong-sdk\n"
            (source / "go.mod").write_text(original)
            (source / "go.sum").write_text("")
            # Both the SDK replacement and temporary modfile may contain spaces.
            with patch.object(tempfile, "tempdir", str(root)):
                with build_environment(package, sdk) as (env, selected):
                    value = subprocess.check_output(["go", "list", "-m", "-f", "{{.Dir}}", "github.com/yetone/magpie"],
                                                    env=env, cwd=source, text=True).strip()
                    self.assertEqual(Path(value), selected)
                    # go env GOMOD deliberately reports the source go.mod even
                    # with -modfile; the real module selection above proves use.
                    modfile = next(flag.removeprefix("-modfile=") for flag in shlex.split(env["GOFLAGS"])
                                   if flag.startswith("-modfile="))
                    self.assertTrue(Path(modfile).is_file())
                    self.assertNotEqual(Path(modfile), source / "go.mod")
            self.assertFalse(Path(modfile).exists())
            self.assertEqual((source / "go.mod").read_text(), original)
            self.assertEqual((source / "go.sum").read_text(), "")

    def test_temporary_modfile_aligns_go_directive_for_newer_sdk(self):
        with tempfile.TemporaryDirectory(prefix="sdk-go-upgrade-") as temporary:
            root = Path(temporary)
            sdk = root / "sdk"
            package = root / "package"
            source = package / "src"
            (sdk / "pkg/extension").mkdir(parents=True)
            source.mkdir(parents=True)
            (sdk / "pkg/extension/contract.go").write_text("package extension\n")
            (sdk / "go.mod").write_text("module github.com/yetone/magpie\n\ngo 1.27.4\n")
            original = "module example.com/fixture\n\ngo 1.26.8\n\nrequire github.com/yetone/magpie v0.0.0\nreplace github.com/yetone/magpie => ../wrong-sdk\n"
            (source / "go.mod").write_text(original)
            (source / "go.sum").write_text("")
            with build_environment(package, sdk) as (env, _):
                modfile = Path(next(flag.removeprefix("-modfile=") for flag in shlex.split(env["GOFLAGS"])
                                    if flag.startswith("-modfile=")))
                self.assertIn("\ngo 1.27.4\n", modfile.read_text())
            self.assertEqual((source / "go.mod").read_text(), original)

    def test_unknown_package_is_not_treated_as_all(self):
        with self.assertRaises(ValueError):
            packages("typo")
        self.assertEqual(list(packages("devin")), ["devin"])

    def test_package_artifact_identity_and_entry(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            manifest = {"id": "fixture", "version": "1.0.0", "entry": "plugin.so",
                        "contract": {"version": {"major": 1, "minor": 9}}}
            (root / "magpie-plugin.json").write_text(json.dumps(manifest))
            (root / "source-lock.json").write_text(json.dumps({"plugin": {
                "id": "fixture", "version": "1.0.0", "sdkMinimum": "1.9"}}))
            (root / "plugin.so").write_bytes(b"fixture")
            validate_distribution(root, "fixture")
            with self.assertRaisesRegex(ValueError, "identity"):
                validate_distribution(root, "other")
            manifest["entry"] = "../outside.so"
            (root / "magpie-plugin.json").write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, "entry"):
                validate_distribution(root, "fixture")
            manifest["entry"], manifest["version"] = "plugin.so", "2.0.0"
            (root / "magpie-plugin.json").write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, "versions"):
                validate_distribution(root, "fixture")


if __name__ == "__main__":
    unittest.main()
