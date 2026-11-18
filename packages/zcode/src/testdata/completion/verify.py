import argparse
import hashlib
import json
from pathlib import Path
import re
import runpy
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("stage", choices=["red", "unit", "regression", "ui-red", "ui-causal", "ui", "static", "package", "native", "causal", "contracts", "replay"])
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--cache-env", required=True, type=Path)
    parser.add_argument("--host", type=Path)
    parser.add_argument("--host-source", type=Path)
    parser.add_argument("--host-inventory", type=Path)
    parser.add_argument("--replay-only", choices=["agent", "auth", "controls", "controller-mcp-history", "ui-auth"])
    parser.add_argument("--count", type=int, default=20)
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    sdk = Path(os.environ.get("MAGPIE_SDK_SOURCE", source.parents[3] / "magpie")).resolve()
    root = args.root.resolve()
    root.mkdir(exist_ok=True)
    cache = json.loads(args.cache_env.read_bytes())
    env = {
        "PATH": str(args.go.parent) + ":/Users/maratkarimov/.local/bin:/Users/maratkarimov/.bun/bin:/usr/local/bin:/usr/bin:/usr/sbin:/bin:/sbin",
        "LANG": "C.UTF-8", "TZ": "UTC", "GOMAXPROCS": "2", "GOFLAGS": "-mod=readonly -p=2",
        "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "CGO_ENABLED": "1",
    }
    for name in ["GOPATH", "GOMODCACHE", "GOCACHE"]:
        env[name] = cache[name]
        assert Path(env[name]).is_dir(), name
    isolation = runpy.run_path(str(source / "testdata/agent/verify.py"))["isolate"]()
    report = {"stage": args.stage, "isolation": isolation, "results": []}
    with tempfile.TemporaryDirectory(prefix=args.stage + "-home-", dir=root) as scratch:
        home = Path(scratch)
        env.update(
            HOME=scratch, USERPROFILE=scratch, XDG_CONFIG_HOME=str(home / "config"),
            XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
            XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
            LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=scratch,
        )

        def run(name, command, expected=0, timeout=240):
            log = root / (name + ".log")
            with log.open("wb") as output:
                process = subprocess.run(command, cwd=source, env=env, stdout=output,
                                         stderr=subprocess.STDOUT, timeout=timeout)
            text = log.read_text()
            counts = dict.fromkeys(["pass", "fail", "skip"], 0)
            failures = []
            for line in text.splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if event.get("Test") and event.get("Action") in counts:
                    counts[event["Action"]] += 1
                    if event["Action"] == "fail":
                        failures.append(event["Test"])
            if command[0] == "node":
                for action in counts:
                    match = re.search(r"^# " + action + r" (\d+)$", text, re.MULTILINE)
                    if match:
                        counts[action] = int(match[1])
            elif command[0] == "bun":
                for action in counts:
                    match = re.search(r"^\s*(\d+) " + action + r"\s*$", text, re.MULTILINE)
                    if match:
                        counts[action] = int(match[1])
            record = {
                "name": name, "command": command, "exit": process.returncode, "expectedExit": expected,
                "counts": counts, "failures": failures,
                "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest(),
            }
            report["results"].append(record)
            (root / (args.stage + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
            print(json.dumps(record), flush=True)
            assert process.returncode == expected, text[-8000:]
            assert "WARNING: DATA RACE" not in text
            if expected and "-json" in command:
                assert counts["fail"] > 0 and "[build failed]" not in text, text[-8000:]
            elif expected:
                assert "ERR_ASSERTION" in text and "# fail 0" not in text, text[-8000:]
            if name.endswith("format"):
                assert not text, text

        go = str(args.go)
        pattern = "^Test(AgentMetadata|ResetControls|ResetOpportunityAcceptsOnlyOfficial)"
        if args.stage in {"red", "unit", "regression"}:
            if args.stage == "unit":
                pattern = "^Test(Agent|OwnedAgent|Reset|UnknownReset|Control|SourceLock|UnsettledReset)"
            elif args.stage == "regression":
                pattern = "."
            run(args.stage + "-regressions",
                [go, "test", "-p=2", "-race", "-json", "-count=" + str(args.count), "-timeout=3m", "-run", pattern, "."],
                expected=1 if args.stage == "red" else 0)
        elif args.stage in {"ui", "ui-red", "ui-causal"}:
            output = home / "web"
            config = home / "tsconfig.json"
            config.write_text(json.dumps({
                "compilerOptions": {
                    "strict": True, "target": "ES2022", "module": "NodeNext", "moduleResolution": "NodeNext",
                    "lib": ["ES2022", "DOM"], "rootDir": str(source.parent / "web"), "outDir": str(output),
                    "paths": {"@magpie/extension-sdk/ui": [str(sdk / "sdk/js/ui.d.ts")]},
                },
                "include": [str(source.parent / "web/*.ts")],
            }))
            run("ui-typecheck", ["tsc", "--project", str(config)])
            (output / "package.json").write_text('{"type":"module"}\n')
            run("ui-sdk-bundle", ["bun", "build", str(sdk / "sdk/js/ui.js"),
                                  "--target", "browser", "--format", "esm", "--outfile", str(output / "sdk-ui.js")])
            run("ui-bundle", ["bun", "build", str(output / "app.js"), "--target", "browser",
                              "--format", "iife", "--outfile", str(output / "app.bundle.js")])
            (output / "index.html").write_bytes((source.parent / "web/index.html").read_bytes())
            env.update(ZCODE_PAGE_BUILD_DIR=str(output), ZCODE_UI15_BUILD_DIR=str(output))
            tests = [str(path) for path in sorted((source.parent / "web").glob("*.test.mjs"))]
            tests.append(str(source.parent / "docs/history/prototype/ui-sdk15/control.test.mjs"))
            if args.stage == "ui-causal":
                page = output / "page.js"
                original = page.read_text()
                before = "if (!await confirmWrite(`${entry.confirmation} для счёта ${selected.id}?`) || disposed)"
                assert original.count(before) == 1
                page.write_text(original.replace(before, "if (disposed)", 1))
                tests = [str(source.parent / "web/page.test.mjs")]
            run("ui-node", ["node", "--test", *tests], expected=1 if args.stage in {"ui-red", "ui-causal"} else 0)
            if args.stage == "ui":
                run("ui-bun", ["bun", "test", *tests])
        elif args.stage == "static":
            run("static-python", ["python3", "-I", "-c",
                                 "import ast,pathlib; [ast.parse(pathlib.Path(name).read_bytes()) for name in "
                                 "['testdata/completion/verify.py','ui_e2e.py']]"])
            run("static-format", [str(args.go.parent / "gofmt"), "-l",
                                  "agent.go", "agent_metadata_test.go", "agent_test.go",
                                  "reset.go", "reset_control_test.go", "module.go", "window.go", "window_test.go", "sources_test.go"])
            run("static-ingress-boundary", ["python3", "-I", "testdata/agent/ingress_test.py"])
            run("static-vet", [go, "vet", "-p=2", "./..."])
            for platform, architecture in [("darwin", "arm64"), ("windows", "amd64")]:
                env.update(GOOS=platform, GOARCH=architecture, CGO_ENABLED="0")
                run("static-" + platform + "-vet", [go, "vet", "-p=2", "."])
                run("static-" + platform + "-build", [go, "build", "-p=2", "-o", str(home / platform), "./cmd/native-ui"])
        elif args.stage == "package":
            run("package-full", ["python3", "-I", "../scripts/build.py", "--output", "../dist/distribution", "--archive", "../dist"], timeout=300)
            import zipfile
            output = home / "package"
            with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
                archive.extractall(output)
            run("package-abi", ["python3", "-I", "../tests/conformance.py", str(output)])
            report["packageSHA256"] = hashlib.sha256((source.parent / "dist/magpie-plugin_linux_amd64.zip").read_bytes()).hexdigest()
            (root / (args.stage + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
        elif args.stage == "contracts":
            run("contract-peer", ["python3", "-I", "testdata/native-contract/peer_test.py"])
        elif args.stage == "replay":
            assert args.host and args.host_source and args.host_inventory, "replay requires exact candidate identities"
            inventory = json.loads(args.host_inventory.read_bytes())
            report["host"] = {
                "sourceTree": inventory["tree"], "parentAtCapture": inventory["head_at_capture"],
                "sourceArchiveSHA256": inventory["archive_sha256"], "sourcePaths": len(inventory["files"]),
                "inventorySHA256": hashlib.sha256(args.host_inventory.read_bytes()).hexdigest(),
                "binarySHA256": hashlib.sha256(args.host.read_bytes()).hexdigest(),
            }
            for name, entry in inventory["files"].items():
                assert hashlib.sha256((args.host_source / name).read_bytes()).hexdigest() == entry["sha256"], name
            assert report["host"]["sourceTree"] == "36f8e629b1092b089d4d59e0f9d8473a1f602ff8"
            assert report["host"]["sourceArchiveSHA256"] == "5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc"
            assert report["host"]["binarySHA256"] == "0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509"
            assert report["host"]["sourcePaths"] == 10846
            commands = {
                "agent": ["python3", "-I", "testdata/agent/run.py", str(args.host)],
                "auth": ["python3", "-I", "../tests/auth_e2e.py", str(args.host), "--china"],
                "controls": ["python3", "-I", "../tests/ui_e2e.py", str(args.host)],
                "controller-mcp-history": ["python3", "-I", "../tests/e2e.py", str(args.host)],
                "ui-auth": ["python3", "-I", "../tests/ui_auth_e2e.py", str(args.host), str(args.host_source)],
            }
            for name, command in commands.items():
                if not args.replay_only or args.replay_only == name:
                    run("replay-" + name, command, timeout=300)
        elif args.stage == "native":
            assert args.host and args.host.is_file(), "native verification needs an explicitly pinned host binary"
            report["hostSHA256"] = hashlib.sha256(args.host.read_bytes()).hexdigest()
            run("native-reset-controls", ["python3", "-I", "../tests/ui_e2e.py", str(args.host)], timeout=300)
        else:
            mutations = [
                ("metadata", "agent.go", 'configureAgentModel(mapObject(rule["config"]), model, prior)',
                 'configureAgentModel(mapObject(rule["config"]), prior, prior)',
                 "^TestAgentMetadataUsesEffectiveGroupLimitsAndDefault$"),
                ("opportunity-envelope", "reset.go", 'code == 0 && granted || code == 3301 && !granted',
                 'code == 0 || code == 3301 && !granted', "^TestResetOpportunityAcceptsOnlyOfficialEnvelopes/zero-false$"),
                ("unknown-reset", "reset.go", 'pending.Accepted && pending.ObservedAt > 0',
                 'pending.ObservedAt > 0', "^TestResetControlsKeepUnknownDebtUntilSeparateExplicitResolution/use$"),
                ("reset-summary", "reset.go", 'summarizeResetStatus(snapshot)}', 'snapshot.Data}',
                 "^TestResetControlsReconcileAcceptedUseAfterRestartWithoutReposting$"),
            ]
            for name, path, before, after, test in mutations:
                original = (source / path).read_text()
                assert original.count(before) == 1, (name, before)
                changed = root / (name + ".go")
                changed.write_text(original.replace(before, after, 1))
                overlay = root / (name + "-overlay.json")
                overlay.write_text(json.dumps({"Replace": {str(source / path): str(changed)}}))
                run("causal-" + name, [go, "test", "-p=2", "-overlay", str(overlay), "-json",
                                      "-count=1", "-timeout=1m", "-run", test, "."], expected=1)


if __name__ == "__main__":
    main()
