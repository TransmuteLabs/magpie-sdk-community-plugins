from contextlib import redirect_stdout
import hashlib
import io
import json
from pathlib import Path
import runpy
import re
import shutil
import sys
import tempfile
import unittest
import subprocess
from unittest import mock

PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE / "scripts"))

from build import build
from provenance import source_lock, check_tracked_inputs


def assert_same_directory(case, actual, expected):
    actual = Path(actual).resolve(strict=True)
    expected = Path(expected).resolve(strict=True)
    case.assertTrue(actual.is_dir(), f"Not a directory: {actual}")
    case.assertTrue(expected.is_dir(), f"Not a directory: {expected}")
    case.assertTrue(actual.samefile(expected), f"{actual} is not the selected directory {expected}")


class DirectoryIdentityTests(unittest.TestCase):
    def test_existing_wrong_directory_with_same_name_is_refused(self):
        with tempfile.TemporaryDirectory(prefix="devin-wrong-directory-") as temporary:
            selected = Path(temporary) / "selected" / "sdk"
            foreign = Path(temporary) / "foreign" / "sdk"
            selected.mkdir(parents=True)
            foreign.mkdir(parents=True)
            for actual, expected in ((selected, foreign), (foreign, selected)):
                with self.subTest(actual=actual, expected=expected), self.assertRaises(AssertionError):
                    assert_same_directory(self, actual, expected)

    def test_files_and_missing_directories_are_refused(self):
        with tempfile.TemporaryDirectory(prefix="devin-invalid-directory-") as temporary:
            regular = Path(temporary) / "regular"
            regular.write_text("fixture")
            with self.assertRaises(AssertionError):
                assert_same_directory(self, regular, regular)
            missing = Path(temporary) / "missing"
            with self.assertRaises(FileNotFoundError):
                assert_same_directory(self, missing, missing)


class SourceLockTests(unittest.TestCase):
    def test_donor044_records_exact_sources_and_bounded_port(self):
        lock = source_lock(self.source)
        donor = next(item for item in lock["sources"] if item["id"] == "community-devin")
        self.assertEqual(donor["commit"], "b35d4b35b2446bd929896ddeb58f3cb299e9e2d1")
        self.assertEqual(donor["version"], "packages/devin 0.1.44")
        self.assertEqual(next(item["sha256"] for item in donor["artifacts"]
                              if item["path"] == "packages/devin/index.mjs"),
                         "ef90d9e1d117a1015e52ed8417af0ef6b918c8f4d1ffa2a4983225eac38ed059")
        comparison = lock["limits"]["donor044Comparison"]
        self.assertEqual(comparison["referenceBaseline"], "ed84daf06cb9eb961a0d6cd7703e39b7b7f90654")
        self.assertEqual(comparison["referenceRequested"], "fe0a9481cdce045597b04f57d5489aab4d7b21a9")
        self.assertEqual(comparison["referenceObserved"], "1729055bb5c566903ce2adbcf97a7e1b78bd3dc1")
        self.assertTrue(comparison["reportedDirtyNowCommitted"])
        self.assertEqual(comparison["trackedDirtyAtCapture"], [])
        self.assertIn("adapter-owned unsigned retry after invalid_argument", comparison["excluded"])
        self.assertIn("turning a byte-size estimate into known context exhaustion", comparison["excluded"])

    def setUp(self):
        self.home = tempfile.TemporaryDirectory(prefix="devin-provenance-")
        self.source = Path(self.home.name)
        repository = PACKAGE
        for name in ("docs/source-lock.json", "docs/SOURCES.md", "src/magpie-plugin.json",
                     "src/magpie-inbound.json", "src/magpie-agent.json", "scripts/build.py",
                     "scripts/provenance.py", "tests/provenance_test.py"):
            target = self.source / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(repository / name, target)
        lock = json.loads((repository / "docs/source-lock.json").read_text())
        for watch in lock["watch"]:
            for name in watch["tests"]:
                target = self.source / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(repository / name, target)
        self.addCleanup(self.home.cleanup)

    def update(self, change):
        path = self.source / "docs/source-lock.json"
        value = json.loads(path.read_text())
        change(value)
        path.write_text(json.dumps(value))

    def test_every_variant_has_its_exact_packaged_identity(self):
        for name in ("magpie-plugin.json", "magpie-inbound.json", "magpie-agent.json"):
            with self.subTest(name=name):
                lock = source_lock(self.source, name)
                manifest = json.loads((self.source / "src" / name).read_text())
                self.assertEqual(lock["plugin"]["version"], manifest["version"])
                self.assertEqual(lock["plugin"]["sdkMinimum"],
                    str(manifest["contract"]["version"]["major"]) + "." + str(manifest["contract"]["version"]["minor"]))
                self.assertEqual(lock["plugin"]["nativeABI"], manifest["compatibility"]["nativeABI"])
                self.assertEqual(lock["plugin"]["capabilities"], manifest["contract"]["capabilities"])

    def test_request_policy_requires_api12_even_when_manifest_and_lock_agree(self):
        for name in ("magpie-agent.json", "magpie-plugin.json", "magpie-inbound.json"):
            with self.subTest(manifest=name):
                self.setUp()
                path = self.source / "src" / name
                manifest = json.loads(path.read_text())
                manifest["contract"]["version"]["minor"] = 11
                manifest["compatibility"]["hostAPI"] = "^1.11"
                if "request-policy" not in manifest["contract"]["capabilities"]:
                    manifest["contract"]["capabilities"].append("request-policy")
                path.write_text(json.dumps(manifest))

                def downgrade(lock):
                    for variant in lock["limits"]["variants"]:
                        if variant["manifest"] == "src/" + name:
                            variant.update(sdkMinimum="1.11", capabilities=manifest["contract"]["capabilities"])
                    if name == "magpie-agent.json":
                        lock["plugin"]["sdkMinimum"] = "1.11"

                self.update(downgrade)
                with self.assertRaisesRegex(ValueError, "request-policy.*1.12"):
                    source_lock(self.source, name)

    def test_variant_capability_drift_is_refused(self):
        self.update(lambda value: value["limits"]["variants"][0].update(capabilities=["inbound-codec"]))
        with self.assertRaises(ValueError):
            source_lock(self.source)

    def test_manifest_host_api_cannot_understate_its_sdk_floor(self):
        path = self.source / "src/magpie-plugin.json"
        manifest = json.loads(path.read_text())
        manifest["compatibility"]["hostAPI"] = "^1.11"
        path.write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "host API"):
            source_lock(self.source)

    def test_stale_module_or_variant_version_is_refused(self):
        self.update(lambda value: value["plugin"].update(version="0.0.0"))
        with self.assertRaises(ValueError):
            source_lock(self.source)
        self.setUp()
        self.update(lambda value: value["limits"]["variants"][2].update(sdkMinimum="1.5"))
        with self.assertRaises(ValueError):
            source_lock(self.source)

    def test_branch_names_and_missing_required_regressions_are_refused(self):
        self.update(lambda value: value["sources"][0].update(commit="main"))
        with self.assertRaises(ValueError):
            source_lock(self.source)
        self.setUp()
        (self.source / "src/agent_account_test.go").unlink()
        with self.assertRaises(ValueError):
            source_lock(self.source)

    def test_source_guide_is_a_required_packaged_input(self):
        (self.source / "docs/SOURCES.md").unlink()
        with self.assertRaises(ValueError):
            source_lock(self.source)

    def test_required_provenance_regressions_must_be_tracked(self):
        # Exercise only Git's read API: no repository/index mutation in tests.
        with mock.patch("subprocess.run", return_value=subprocess.CompletedProcess([], 0)) as git:
            required = check_tracked_inputs(self.source)
        self.assertTrue({"docs/source-lock.json", "docs/SOURCES.md", "scripts/build.py",
                         "scripts/provenance.py", "tests/provenance_test.py",
                         "src/magpie-agent.json", "src/agent_account_test.go"}.issubset(required))
        self.assertEqual(git.call_args.args[0],
                         ["git", "ls-files", "--error-unmatch", "--", *required])
        self.assertEqual(git.call_args.kwargs["cwd"], self.source)
        with mock.patch("subprocess.run", return_value=subprocess.CompletedProcess([], 1)):
            with self.assertRaises(ValueError):
                check_tracked_inputs(self.source)

    def test_genuine_gemini_fixture_is_locked_with_its_oracle_regressions(self):
        lock = source_lock(self.source)
        donor = next(item for item in lock["sources"] if item["id"] == "community-gemini-041")
        self.assertEqual(donor["commit"], "191d94bf6ed95ccc4bc62d77e1dd9201a17f5918")
        fixture = self.source / "src/testdata/gemini-tool-reply.bin"
        digest = hashlib.sha256(fixture.read_bytes()).hexdigest()
        self.assertEqual(digest, "e4ff6e3b5134fe64f3d9761a98d3ddd1af0945ea6f7946fdfe01a9389ebd4162")
        recorded = next(item for item in donor["artifacts"] if item["path"].endswith("gemini-tool-reply.bin"))
        self.assertEqual(recorded["sha256"], digest)
        watch = next(item for item in lock["watch"] if item["source"] == donor["id"])
        self.assertTrue({"src/provider_gemini_test.go", "src/codec/gemini_thought_test.go", "tests/gemini_host_e2e.py"}.issubset(watch["tests"]))


class LayoutTests(unittest.TestCase):
    def test_current_entry_and_reference_links_resolve(self):
        for name in ("README.md", "docs/REFERENCE.md", "docs/PROVIDER-PARITY.md", "docs/CLIPROXY-PROVIDER-CHECK.md"):
            document = PACKAGE / name
            for target in re.findall(r"\]\(([^)]+)\)", document.read_text()):
                if target.startswith(("https:", "http:", "#", "mailto:")):
                    continue
                with self.subTest(document=name, target=target):
                    self.assertTrue((document.parent / target.split("#")[0]).is_file())

    def test_relocated_gemini_oracle_retains_its_hash_and_wire_assertions(self):
        from gemini_host_e2e import oracle
        oracle()


class DistributionTests(unittest.TestCase):
    def test_distribution_fixture_accepts_same_directory_alias(self):
        with tempfile.TemporaryDirectory(prefix="devin-directory-alias-") as temporary:
            physical = Path(temporary) / "physical"
            physical.mkdir()
            alias = Path(temporary) / "alias"
            alias.symlink_to(physical, target_is_directory=True)
            with mock.patch.object(tempfile, "tempdir", str(alias)):
                self.package()

    def package(self, mode=None, **options):
        repository = PACKAGE
        with tempfile.TemporaryDirectory(prefix="devin-package-") as temporary:
            destination = Path(temporary) / "distribution"
            sdk = Path(temporary) / "selected sdk"
            (sdk / "pkg/extension").mkdir(parents=True)
            (sdk / "pkg/extension/contract.go").write_text("package extension\n")
            (sdk / "sdk/js").mkdir(parents=True)
            for name in ("ui.js", "account-selection.js", "errors.js"):
                (sdk / "sdk/js" / name).write_text("export {};\n")
            before = [(repository / "src" / name).read_bytes() for name in ("go.mod", "go.sum")]
            with mock.patch("subprocess.check_output", return_value=json.dumps({
                "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1",
            })), mock.patch("subprocess.run") as compile_native, redirect_stdout(io.StringIO()):
                if mode is None:
                    build(destination, sdk=sdk, **options)
                else:
                    arguments = ["build.py", "--output", str(destination), "--sdk", str(sdk)] + ([mode] if mode else [])
                    with mock.patch.object(sys, "argv", arguments):
                        runpy.run_path(str(repository / "scripts/build.py"), run_name="__main__")
            self.assertEqual(compile_native.call_count, 1)
            assert_same_directory(self, compile_native.call_args.kwargs["cwd"], repository / "src")
            environment = compile_native.call_args.kwargs["env"]
            assert_same_directory(self, environment["MAGPIE_SDK_SOURCE"], sdk)
            self.assertEqual(environment["GOWORK"], "off")
            self.assertEqual(environment["GOPROXY"], "off")
            self.assertIn("-modfile=", environment["GOFLAGS"])
            self.assertEqual(before, [(repository / "src" / name).read_bytes() for name in ("go.mod", "go.sum")])
            manifest = json.loads((destination / "magpie-plugin.json").read_text())
            lock = json.loads((destination / "source-lock.json").read_text())
            self.assertEqual(lock["plugin"]["version"], manifest["version"])
            self.assertEqual(lock["plugin"]["capabilities"], manifest["contract"]["capabilities"])
            floor = lock["plugin"]["sdkMinimum"]
            version = manifest["contract"]["version"]
            self.assertEqual(floor, str(version["major"]) + "." + str(version["minor"]))
            self.assertEqual(manifest["compatibility"]["hostAPI"], "^" + floor)
            self.assertEqual(manifest["entry"], "plugin.so")
            self.assertEqual(manifest["compatibility"]["platforms"], ["linux/amd64"])
            self.assertNotIn("window", manifest)
            self.assertFalse((destination / "web").exists())
            presentation = json.loads((destination / "package.json").read_text())
            self.assertEqual((presentation["name"], presentation["version"]),
                             (manifest["id"], manifest["version"]))
            self.assertEqual(presentation["magpie"], {"name": "Devin", "icon": "devin"})
            if "providers" in manifest:
                binding, = manifest["authBindings"]
                self.assertEqual((binding["provider"], binding["from"], binding["reauthorize"]),
                                 ("devin", "provider:signin", True))
                self.assertEqual([(method["id"], method["mode"]) for method in binding["methods"]],
                                 [("browser", "poll"), ("api-key", "poll"), ("saved-signin", "poll")])
                self.assertIn("provider-saved-signin", manifest["contract"]["capabilities"])
                saved = binding["methods"][-1]["fields"]
                self.assertEqual([(field["key"], field.get("optional", False)) for field in saved],
                                 [("record", False)])
                self.assertTrue(all(field["type"] == "secret"
                                    for method in binding["methods"] for field in method.get("fields", [])))
            else:
                self.assertNotIn("authBindings", manifest)
            for name in ("SOURCES.md", "ONBOARDING.md", "EXECUTION-FACTS.md", "DONOR-MATRIX.md", "PROVIDER-PARITY.md", "CLIPROXY-PROVIDER-CHECK.md", "DELIVERY-EVIDENCE.md",
                         "CACHE-KEEPALIVE.md", "CACHE-KEEPALIVE-EVIDENCE.md", "GEMINI-SIGNATURE-EVIDENCE.md",
                         "DOMAIN-COMPLETION-EVIDENCE.md"):
                self.assertEqual((destination / name).read_bytes(), (repository / ("docs/history" if name.endswith("-EVIDENCE.md") else "docs") / name).read_bytes())
            return manifest, lock, compile_native.call_args.args[0][-1]

    def test_default_distribution_contains_all_ready_surfaces(self):
        manifest, lock, entry = self.package()
        package = json.loads((PACKAGE / "package.json").read_text())
        self.assertEqual(manifest["version"], package["version"])
        self.assertEqual(entry, "./cmd/native-agent")
        self.assertEqual(manifest["contract"]["capabilities"], [
            "inbound-codec", "outbound-executor", "account-auth", "model-discovery",
            "usage-reporter", "agent-wiring", "module-rpc", "native-account-operations", "request-policy",
            "provider-signin", "provider-saved-signin",
        ])
        self.assertEqual(manifest["agents"][0]["id"], "devin-native")
        self.assertEqual(manifest["providers"][0]["protocols"], ["chat", "anthropic"])
        self.assertEqual(len(manifest["inbounds"]), 13)
        self.assertEqual(len(manifest["accountOperations"]), 5)
        self.assertEqual(len(manifest["actions"]), 6)
        bundle = next(row for row in manifest["accountOperations"] if row["id"] == "managed-plugin-bundle")
        self.assertEqual(bundle, {"id": "managed-plugin-bundle", "agent": "devin-native",
                                  "provider": "devin", "action": "devin.account.managed-plugin-bundle"})
        self.assertEqual(lock["plugin"]["sdkMinimum"], "1.12")
        self.assertEqual(manifest["providers"][0]["accountTasks"],
                         [{"id": "cache-keepalive", "action": "devin.cache-keepalive"}])
        paid = manifest["providers"][0]["options"][0]
        self.assertEqual(paid["key"], "keepAlive")
        self.assertIs(paid["default"], False)
        self.assertIn("paid", paid["confirm"])
        self.assertEqual(source_lock(PACKAGE)["plugin"], lock["plugin"])
        selection = lock["limits"]["standardDistribution"]
        self.assertEqual(selection["manifest"], "src/magpie-agent.json")
        self.assertEqual(selection["entrypoint"], entry)
        self.assertEqual(selection["compatibility"], {
            "--paired-only": "src/magpie-plugin.json", "--inbound-only": "src/magpie-inbound.json",
        })

    def test_explicit_compatibility_distributions_keep_their_floors(self):
        for options, version, floor, entry, capabilities in [
            ({"agent": False}, "0.5.1", "1.12", "./cmd/native",
             ["inbound-codec", "outbound-executor", "account-auth", "model-discovery", "usage-reporter", "request-policy",
              "provider-signin", "provider-saved-signin"]),
            ({"inbound_only": True}, "0.2.0", "1.1", "./cmd/native-inbound", ["inbound-codec", "model-discovery"]),
        ]:
            with self.subTest(version=version):
                manifest, lock, compiled = self.package(**options)
                self.assertEqual((manifest["version"], lock["plugin"]["sdkMinimum"], compiled),
                                 (version, floor, entry))
                self.assertEqual(manifest["contract"]["capabilities"], capabilities)
                self.assertNotIn("agent-wiring", manifest["contract"]["capabilities"])
                self.assertNotIn("agents", manifest)
                self.assertNotIn("accountOperations", manifest)
                if version == "0.2.0":
                    self.assertNotIn("providers", manifest)

    def test_cli_defaults_full_with_explicit_compatibility_modes(self):
        default_version = json.loads((PACKAGE / "package.json").read_text())["version"]
        for mode, version, entry in [
            ("", default_version, "./cmd/native-agent"),
            ("--agent", default_version, "./cmd/native-agent"),
            ("--paired-only", "0.5.1", "./cmd/native"),
            ("--inbound-only", "0.2.0", "./cmd/native-inbound"),
        ]:
            with self.subTest(mode=mode):
                manifest, _, compiled = self.package(mode=mode)
                self.assertEqual((manifest["version"], compiled), (version, entry))


if __name__ == "__main__":
    unittest.main()
