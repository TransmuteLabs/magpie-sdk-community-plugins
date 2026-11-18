#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", type=Path, required=True)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--binary-sha256")
    parser.add_argument("--transport-overlay", type=Path)
    parser.add_argument("--decoder-overlay", type=Path)
    parser.add_argument("--host-overlay", type=Path)
    parser.add_argument("--distribution", type=Path, required=True)
    parser.add_argument("--proof", type=Path, required=True)
    parser.add_argument("--count", type=int, default=20)
    parser.add_argument("--only", nargs="+", choices=("binary-install", "go-protocols", "systemone", "agent", "owner"))
    args = parser.parse_args()
    source = Path(__file__).resolve().parent.parent
    proof = args.proof.resolve()
    proof.mkdir()
    pins = {
        "GOROOT": "/tmp/magpie-sdk-v11-usbox-niq306rt/go-toolchain-1.26.8",
        "GOPATH": "/Users/maratkarimov/go", "GOMODCACHE": "/Users/maratkarimov/go/pkg/mod",
        "GOCACHE": "/Users/maratkarimov/.cache/go-build",
    }
    environment = pins | {
        "PATH": pins["GOROOT"] + "/bin:/usr/bin:/usr/sbin:/bin:/sbin:/Users/maratkarimov/.local/bin",
        "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOENV": "off",
        "GOFLAGS": "-mod=readonly -p=2", "GOMAXPROCS": "2", "CGO_ENABLED": "1",
        "LANG": "C.UTF-8", "PYTHONDONTWRITEBYTECODE": "1", "MAGPIE_NO_ACCOUNTS": "",
    }
    guard = Path("/tmp/magpie-commandcode-final-repin.YJ06ocvO/magpie-verification-network.py")
    if hashlib.sha256(guard.read_bytes()).hexdigest() != "09f2a8d3311566d21dafc87c118ef8d5e07cfba88f78dba3114e4cf124b2b57e":
        raise SystemExit("Existing network guard changed")
    distribution = args.distribution.resolve()
    shared_overlay = {str(args.host / "internal/provider/extension_transport.go"):
        str(args.transport_overlay.resolve())} if args.transport_overlay else {}
    if args.decoder_overlay:
        shared_overlay[str(args.host / "internal/gateway/continuation.go")] = str(args.decoder_overlay.resolve())
    if args.host_overlay:
        for name in ("internal/provider/extension_transport.go", "internal/gateway/gateway.go",
                     "internal/gateway/continuation.go", "internal/gateway/search.go",
                     "internal/gateway/chat.go", "internal/gateway/responses.go",
                     "internal/gateway/anthropic.go", "internal/gateway/codeassist.go"):
            shared_overlay[str(args.host / name)] = str((args.host_overlay / name).resolve())
    commands = []
    if args.binary:
        expected = args.binary_sha256
        if not expected:
            lock = json.loads((source / "docs/source-lock.json").read_text())
            expected = next(entry for entry in lock["sources"] if entry["id"] == "sdk-tested-host")["binary"]["sha256"]
        commands.append(("binary-install", ["python3",
            str(source / "tests/install_package.py"),
            str(args.binary.resolve()), str(distribution), expected]))
    for name, host_file, fixture, package, test_name, flag in (
        ("go-protocols", "internal/gateway/commandcode_native_go_protocol_test.go",
            "native_go_plan_test.go.txt", "./internal/gateway", "TestCommandCodeNativeGoProtocols", "commandcode-go-distribution"),
        ("systemone", "internal/gateway/commandcode_native_owner_test.go",
            "native_gateway_test.go.txt", "./internal/gateway", "TestCommandCodeNativeSystemOneSDK17", "commandcode-sdk17-distribution"),
        ("agent", "internal/agent/commandcode_native_metadata_test.go",
            "native_agent_metadata_test.go.txt", "./internal/agent", "TestCommandCodeNativeAgentEffectiveMetadata", "commandcode-metadata-distribution"),
    ):
        overlay = proof / (name + "-overlay.json")
        overlay.write_text(json.dumps({"Replace": shared_overlay | {
            str(args.host / host_file): str(source / "tests" / fixture)
        }}, indent=2) + "\n")
        commands.append((name, ["go", "test", "-race", "-tags", "nogui", "-overlay", str(overlay),
            package, "-run", "^" + test_name + "$", "-count=" + str(args.count), "-json", "-timeout=900s",
            "-args", "-" + flag, str(distribution)]))
    owner = args.host / "internal/commandcodeowner/main.go"
    overlay = proof / "owner-overlay.json"
    overlay.write_text(json.dumps({"Replace": shared_overlay | {str(owner): str(source / "tests/native_owner.go.txt")}}, indent=2) + "\n")
    commands.append(("owner", ["go", "run", "-overlay", str(overlay), str(owner), str(distribution)]))
    if args.only:
        commands = [(name, command) for name, command in commands if name in args.only]
    results = []
    runtime = {str(path.relative_to(distribution)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted(distribution.rglob("*")) if path.is_file()}
    verification_files = {str(path.relative_to(source)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted((source / "tests").iterdir()) if path.is_file()}
    for name, command in commands:
        home = Path(tempfile.mkdtemp(prefix="commandcode-native-" + name + "-", dir=proof))
        current = environment | {
            "HOME": str(home), "USERPROFILE": str(home), "TMPDIR": str(home),
            "XDG_CONFIG_HOME": str(home / "config"), "XDG_CACHE_HOME": str(home / "cache"),
            "XDG_DATA_HOME": str(home / "data"), "XDG_STATE_HOME": str(home / "state"),
            "APPDATA": str(home / "appdata"), "LOCALAPPDATA": str(home / "localappdata"),
        }
        log = proof / (name + ".log")
        started = time.monotonic()
        try:
            with log.open("xb") as output:
                process = subprocess.run(["python3", str(guard), str(proof / (name + "-network.json")), *command],
                    cwd=args.host, env=current, stdout=output, stderr=subprocess.STDOUT)
        finally:
            shutil.rmtree(home)
        counts = {"pass": 0, "fail": 0, "skip": 0}
        for line in log.read_text(errors="replace").splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if "Test" in event and event.get("Action") in counts:
                counts[event["Action"]] += 1
        result = {"name": name, "command": command, "exit": process.returncode, "counts": counts,
            "seconds": round(time.monotonic() - started, 3),
            "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        results.append(result)
        (proof / "results.json").write_text(json.dumps({
            "hostSource": str(args.host), "hostBinary": str(args.binary) if args.binary else None,
            "hostBinarySHA256": hashlib.sha256(args.binary.read_bytes()).hexdigest() if args.binary else None,
            "transportOverlay": {"path": str(args.transport_overlay), "sha256":
                hashlib.sha256(args.transport_overlay.read_bytes()).hexdigest()} if args.transport_overlay else None,
            "decoderOverlay": {"path": str(args.decoder_overlay), "sha256":
                hashlib.sha256(args.decoder_overlay.read_bytes()).hexdigest()} if args.decoder_overlay else None,
            "hostOverlay": {name: hashlib.sha256(Path(path).read_bytes()).hexdigest()
                for name, path in shared_overlay.items()},
            "distribution": str(distribution), "distributionFiles": runtime,
            "verificationFiles": verification_files,
            "environment": environment, "results": results,
        }, indent=2) + "\n")
        print(json.dumps({key: result[key] for key in ("name", "exit", "counts", "seconds")}), flush=True)
        if process.returncode:
            break
    raise SystemExit(int(any(result["exit"] for result in results)))


if __name__ == "__main__":
    main()
