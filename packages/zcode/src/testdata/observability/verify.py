import argparse
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import zipfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("stage", choices=["red", "unit", "host", "installed", "package", "provenance", "causal", "owners", "owners-remaining", "sqlite"])
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--label")
    args = parser.parse_args()
    root = args.root.resolve()
    source = Path(__file__).resolve().parents[2]
    sdk = root / "magpie"
    environment = {
        "PATH": str(args.go.parent) + ":/Users/maratkarimov/.bun/bin:/usr/local/bin:/usr/bin:/bin",
        "HOME": os.environ["HOME"],
        "LANG": "C.UTF-8", "TZ": "UTC",
    }
    environment.update(json.loads(subprocess.check_output(
        [str(args.go), "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"], env=environment)))
    isolation = runpy.run_path(str(source / "testdata/agent/verify.py"))["isolate"]()
    home = Path(tempfile.mkdtemp(prefix=args.stage + "-home-", dir=root))
    environment.update(
        HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
        XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
        XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
        LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
        GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
        CGO_ENABLED="1",
    )
    identity = json.loads((root / "sdk-source.json").read_text())
    report = {"stage": args.stage, "isolation": isolation,
              "sdkTree": identity.get("tree"), "sdkSnapshotSHA256": identity.get("snapshotSHA256"),
              "results": []}
    environment["PATH"] = str(root / "tools") + ":" + environment["PATH"]

    def audit():
        changed = []
        for name, expected in identity["files"].items():
            path = sdk / name
            if "symlink" in expected:
                good = path.is_symlink() and os.readlink(path) == expected["symlink"]
            else:
                good = path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == expected["sha256"]
            if good and "mode" in expected:
                good = path.lstat().st_mode & 0o777 == expected["mode"]
            if not good:
                changed.append(name)
        assert not changed, changed
        return len(identity["files"])

    report["sdkFilesBefore"] = audit()

    def run(label, command, cwd=source, timeout=300, expected=0):
        if args.label:
            label = args.label + "-" + label
        log = root / (label + ".log")
        with log.open("wb") as output:
            process = subprocess.run(command, cwd=cwd, env=environment,
                                     stdout=output, stderr=subprocess.STDOUT, timeout=timeout)
        counts = {action: 0 for action in ["pass", "fail", "skip"]}
        if "-json" in command:
            for line in log.read_text().splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if event.get("Test") and event.get("Action") in counts:
                    counts[event["Action"]] += 1
        result = {"label": label, "command": command, "exit": process.returncode,
                  "expectedExit": expected, "counts": counts,
                  "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        report["results"].append(result)
        (root / ((args.label or args.stage) + "-result.json")).write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(result), flush=True)
        if process.returncode != expected:
            print(log.read_text()[-6000:], flush=True)
            raise SystemExit(process.returncode or 1)
        if expected:
            assert counts["fail"] > 0, "RED must reach compiled behavior assertions"
            if args.stage == "red":
                assert counts["pass"] > 0, "old-source RED must include passing controls"
        if command[0] == str(args.go.parent / "gofmt"):
            assert not log.read_bytes(), "source is not formatted"

    go = str(args.go)
    if args.stage == "red":
        run("old-observability-red", [go, "test", "-json", "-race", "-count=1", "-timeout=3m",
                                     "-run", "^TestObservability7", "."], expected=1)
    elif args.stage == "unit":
        run("gofmt", [str(args.go.parent / "gofmt"), "-l", "module.go", "module_test.go",
                      "response.go", "response_usage.go", "response_observability_test.go",
                      "observability.go", "window.go", "window_test.go", "lanes.go",
                      "protocol/response.go", "sources_test.go", "quota_metadata.go", "quota_metadata_test.go",
                      "usage.go", "discovery_start.go"])
        run("module-vet", [go, "vet", "./..."])
        run("python-syntax", ["python3", "-I", "-c",
            "import ast,pathlib; [ast.parse(pathlib.Path(name).read_bytes()) for name in " +
            repr(["../tests/conformance.py", "testdata/observability/verify.py",
                  "testdata/observability/installed.py", "testdata/parity/default_package.py",
                  "../tests/ui_auth_e2e.py", "../tests/ui_e2e.py"]) + "]"])
        run("module-race", [go, "test", "-json", "-race", "-count=1", "-timeout=5m", "./..."], timeout=360)
        run("observability-race20", [go, "test", "-json", "-race", "-count=20", "-timeout=3m",
                                    "-run", "^TestObservability7", "."], timeout=240)
    elif args.stage == "host":
        run("host-build", [go, "build", "-tags", "nogui", "-trimpath", "-o",
                           str(root / "magpie-host"), "."], cwd=sdk, timeout=600)
    elif args.stage == "package":
        run("full-package", ["python3", "-I", "testdata/parity/default_package.py", str(root), "full-package", "--output", str(source.parent / "dist/distribution"), "--archive", str(source.parent / "dist")])
        with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
            archive.extractall(root / "full-artifact")
        run("native-abi", ["python3", "-I", "../tests/conformance.py", str(root / "full-artifact")])
        run("headless-package", ["python3", "-I", "../scripts/build.py", "--output", "../dist/compat-sdk14/distribution", "--archive", "../dist/compat-sdk14", "--headless"])
        with zipfile.ZipFile(source.parent / "dist/compat-sdk14/magpie-plugin_linux_amd64.zip") as archive:
            archive.extractall(root / "headless-artifact")
        run("headless-native-abi", ["python3", "-I", "../tests/conformance.py", str(root / "headless-artifact")])
        environment["ZCODE_PAGE_BUILD_DIR"] = str(root / "full-artifact/web")
        environment["ZCODE_UI15_BUILD_DIR"] = str(root / "full-artifact/web")
        tests = [str(path) for path in sorted((source.parent / "web").glob("*.test.mjs"))]
        tests.append(str(source.parent / "docs/history/prototype/ui-sdk15/control.test.mjs"))
        run("node-ui", ["node", "--test", *tests])
        run("bun-ui", ["bun", "test", *tests])
    elif args.stage == "installed":
        run("installed-host", ["python3", "-I", "testdata/observability/installed.py", str(root)],
            timeout=360)
    elif args.stage == "provenance":
        run("source-provenance", ["python3", "-I", "testdata/parity/provenance.py", str(root)])
        lock = json.loads((source.parent / "docs/source-lock.json").read_bytes())
        sdk_lock = next(item for item in lock["sources"] if item["id"] == identity.get("id", "sdk-observability7"))
        if "snapshotSHA256" in identity:
            assert sdk_lock["snapshotSHA256"] == identity["snapshotSHA256"]
            assert sdk_lock["archiveSHA256"] == identity["archiveSHA256"]
        else:
            assert sdk_lock["tree"] == identity["tree"]
        for name, expected in sdk_lock["checkedFiles"].items():
            assert hashlib.sha256((sdk / name).read_bytes()).hexdigest() == expected, name
        report["checkedSDKFiles"] = sdk_lock["checkedFiles"]
    elif args.stage in {"owners", "owners-remaining"}:
        owners = [
            ("native-lifecycle", ["testdata/agent/run.py"]),
            ("native-cn-auth", ["../tests/auth_e2e.py"]),
            ("native-controls", ["../tests/ui_e2e.py"]),
            ("native-controller-mcp-history", ["../tests/e2e.py"]),
            ("native-ui-auth", ["../tests/ui_auth_e2e.py"]),
        ]
        for label, command in owners[2:] if args.stage == "owners-remaining" else owners:
            arguments = ["python3", "-I", *command, str(root / "magpie-host")]
            if label == "native-cn-auth":
                arguments.append("--china")
            if label == "native-ui-auth":
                arguments.append(str(sdk))
            run(label, arguments, timeout=300)
    elif args.stage == "sqlite":
        run("native-sqlite", ["python3", "-I", "testdata/sqlite-rollback/run.py",
                             "--sdk", str(sdk), "--distribution", str(root / "full-artifact"),
                             "--output", str(root / ((args.label or "current") + "-sqlite-owner"))], timeout=600)
    elif args.stage == "causal":
        mutations = [
            ("no-facts", "response.go", "executionUsage{rich: observability}",
             "executionUsage{rich: false}", "^TestObservability7JSON"),
            ("ignore-zero", "response_usage.go", "*field.value, known = count, true",
             "if *count == 0 { continue }; *field.value, known = count, true",
             "^TestObservability7Stream.*"),
            ("no-diagnostic", "observability.go",
             "_ = stream.HostCall(observeCtx, sdk.MethodDiagnosticEmit, diagnostic, &receipt)",
             "if false { _ = stream.HostCall(observeCtx, sdk.MethodDiagnosticEmit, diagnostic, &receipt) }",
             "^TestObservability7Diagnostics"),
            ("lost-header", "response.go", 'if usage.value() != nil {',
             'if false && usage.value() != nil {', "^TestObservability7IncompleteJSON"),
            ("rewritten-facts", "protocol/response.go", "observe(body)",
             "func() error { restored, failure := Restore(body, renamed, responseModel...); "
             "if failure != nil { return failure }; return observe(restored) }()",
             "^TestObservability7Stream"),
        ]
        for label, name, before, after, test in mutations:
            original = (source / name).read_text()
            assert original.count(before) == 1, label
            target = root / ("mutation-" + label + ".go")
            target.write_text(original.replace(before, after, 1))
            overlay = root / ("mutation-" + label + "-overlay.json")
            overlay.write_text(json.dumps({"Replace": {str(source / name): str(target)}}))
            run("mutation-" + label, [go, "test", "-overlay", str(overlay), "-json", "-count=1",
                                     "-timeout=1m", "-run", test, "."], expected=1, timeout=100)
    report["sdkFilesAfter"] = audit()
    (root / ((args.label or args.stage) + "-result.json")).write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"stage": args.stage, "sdkTree": identity.get("tree"),
                      "sdkSnapshotSHA256": identity.get("snapshotSHA256"),
                      "sdkFilesUnchanged": report["sdkFilesAfter"], "isolation": isolation}), flush=True)


if __name__ == "__main__":
    main()
