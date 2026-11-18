#!/usr/bin/env python3
import argparse
import hashlib
import json
import sys
from pathlib import Path
import shutil
import subprocess
import tempfile
import time


PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("phase", choices=("red", "green", "package", "final"))
    parser.add_argument("--host", type=Path, required=True)
    parser.add_argument("--sources", type=Path, required=True)
    parser.add_argument("--host-overlay", type=Path)
    parser.add_argument("--test-pattern")
    parser.add_argument("--proof", type=Path, required=True, help="New directory for isolated historical replay evidence")
    args = parser.parse_args()
    with build_environment(PACKAGE, args.host) as (sdk_env, _):
        run(args, sdk_env)


def run(args, sdk_env):
    source = PACKAGE
    proof = args.proof.resolve()
    proof.mkdir()
    pins = {
        "GOROOT": "/tmp/magpie-sdk-v11-usbox-niq306rt/go-toolchain-1.26.8",
        "GOPATH": "/Users/maratkarimov/go",
        "GOMODCACHE": "/Users/maratkarimov/go/pkg/mod",
        "GOCACHE": "/Users/maratkarimov/.cache/go-build",
    }
    for value in pins.values():
        if not Path(value).is_dir():
            raise SystemExit("Pinned offline toolchain/cache is absent: " + value)
    environment = pins | {
        "PATH": pins["GOROOT"] + "/bin:/usr/bin:/usr/sbin:/bin:/sbin:/Users/maratkarimov/.local/bin",
        "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOENV": "off",
        "GOFLAGS": sdk_env["GOFLAGS"] + " -p=2", "GOMAXPROCS": "2", "CGO_ENABLED": "1",
        "LANG": "C.UTF-8", "PYTHONDONTWRITEBYTECODE": "1", "MAGPIE_BUILD_ON_USBOX": "1",
    }
    guard = Path("/tmp/magpie-commandcode-final-repin.YJ06ocvO/magpie-verification-network.py")
    if hashlib.sha256(guard.read_bytes()).hexdigest() != "09f2a8d3311566d21dafc87c118ef8d5e07cfba88f78dba3114e4cf124b2b57e":
        raise SystemExit("Existing network guard changed")
    if args.phase == "red":
        commands = [("old-code-negative", ["go", "test", "-json", "-count=1", "-run",
            args.test_pattern or "^Test(GoogleSchema|GenerateFitsRecovered|GenerateRecoveredDisabled|GenerateRefusesUnknown|ModuleGoPlanContinues|AgentOffUses)", "./..."])]
    else:
        commands = [
            ("host-snapshot", ["python3", "tests/check_host_snapshot.py", "--root", str(args.host.parent),
                "--output", str(proof / "host-snapshot.json")]),
            ("format", ["gofmt", "-l", *sorted(str(path) for path in (source / "src").rglob("*.go"))]),
            ("race", ["go", "test", "-race", "-count=20", "-json", "-timeout=900s", "./..."]),
            ("vet", ["go", "vet", "./..."]),
            ("build", ["go", "build", "./..."]),
            ("types", ["python3", "tests/typecheck.py", "--sdk", str(args.host.resolve())]),
            ("donor", ["node", "tests/systemone_donor.mjs", str(args.sources / "community")]),
            ("harness", ["node", "tests/harness_probe.mjs", str(args.sources / "harness/package")]),
            ("sources", ["python3", "tests/check_sources.py",
                "--source", "community-commandcode=" + str(args.sources / "community"),
                "--source", "cliproxyapi-maintained=" + str(args.sources / "cliproxy"),
                "--source", "official-commandcode-harness=" + str(args.sources / "harness/package"),
                "--source", "sdk-tested-host=" + str(args.host)]),
            ("package", ["python3", "scripts/build.py", "--archive", "--output", str(proof / "dist/package"), "--sdk", str(args.host.resolve())]),
            ("package-verify", ["python3", "tests/check_package.py",
                str(proof / "dist/protocol-commandcode-0.1.3-linux-amd64.zip")]),
        ]
        if args.phase == "package":
            commands = commands[-3:]
        elif args.phase == "final":
            commands = commands[:2] + [("provenance-test", ["go", "test", "-json", "-count=1",
                "-run", "^TestPackageProvenanceAndVersionsAgree$", "./..."])] + commands[-3:]
        if args.host_overlay:
            for name, command in commands:
                if name == "sources":
                    command += ["--host-overlay", str(args.host_overlay)]
    results = []
    for name, command in commands:
        home = Path(tempfile.mkdtemp(prefix="commandcode-" + name + "-", dir=proof))
        current = environment | {
            "HOME": str(home), "USERPROFILE": str(home), "TMPDIR": str(home),
            "XDG_CONFIG_HOME": str(home / "config"), "XDG_CACHE_HOME": str(home / "cache"),
            "XDG_DATA_HOME": str(home / "data"), "XDG_STATE_HOME": str(home / "state"),
            "APPDATA": str(home / "appdata"), "LOCALAPPDATA": str(home / "localappdata"),
        }
        log = proof / (name + ".log")
        network = proof / (name + "-network.json")
        started = time.monotonic()
        try:
            with log.open("xb") as output:
                process = subprocess.run(["python3", str(guard), str(network), *command],
                    cwd=source / "src" if command[0] == "go" else source, env=current, stdout=output, stderr=subprocess.STDOUT)
        finally:
            shutil.rmtree(home)
        counts = {"pass": 0, "fail": 0, "skip": 0}
        failures = []
        for line in log.read_text(errors="replace").splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if "Test" in event and event.get("Action") in counts:
                counts[event["Action"]] += 1
                if event["Action"] == "fail":
                    failures.append(event["Test"])
        result = {"name": name, "command": command, "exit": process.returncode, "counts": counts,
            "failures": failures, "seconds": round(time.monotonic() - started, 3),
            "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        if name == "format" and log.read_text().strip():
            result["formatOutput"] = log.read_text()
            result["exit"] = 1
        results.append(result)
        (proof / "results.json").write_text(json.dumps({
            "phase": args.phase, "pins": pins, "environment": environment, "host": str(args.host),
            "sourceFiles": {str(path.relative_to(source)): hashlib.sha256(path.read_bytes()).hexdigest()
                for path in sorted(source.rglob("*")) if path.is_file() and ".git" not in path.parts},
            "networkGuardSHA256": hashlib.sha256(guard.read_bytes()).hexdigest(),
            "results": results,
        }, indent=2) + "\n")
        print(json.dumps(result), flush=True)
        if result["exit"] and args.phase != "red":
            break
    if args.phase == "red":
        raise SystemExit(0 if results[0]["counts"]["fail"] else 1)
    raise SystemExit(int(any(result["exit"] for result in results)))


if __name__ == "__main__":
    main()
