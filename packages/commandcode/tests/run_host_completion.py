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
    parser = argparse.ArgumentParser(description="Replay the narrow generic completion regression on an immutable host overlay.")
    parser.add_argument("phase", choices=("red", "green", "binary"))
    parser.add_argument("--host", type=Path, required=True)
    parser.add_argument("--inputs", type=Path, required=True)
    parser.add_argument("--proof", type=Path, required=True)
    parser.add_argument("--prior-transport", type=Path)
    parser.add_argument("--decoder-overlay", type=Path)
    parser.add_argument("--test-pattern")
    parser.add_argument("--count", type=int, default=20)
    parser.add_argument("--only", nargs="+", choices=("race", "gateway-race", "format", "vet", "build"))
    args = parser.parse_args()
    if args.phase == "red" and not (args.prior_transport or args.decoder_overlay):
        parser.error("red requires an explicit prior transport or decoder with the current hook declarations; original baseline inputs remain in their frozen receipts")
    proof = args.proof.resolve()
    proof.mkdir()
    paths = ("internal/provider/extension_transport.go", "internal/provider/extension_transport_completion_test.go",
        "internal/gateway/gateway.go", "internal/gateway/continuation.go", "internal/gateway/search.go",
        "internal/gateway/chat.go", "internal/gateway/responses.go",
        "internal/gateway/anthropic.go", "internal/gateway/codeassist.go",
        "internal/gateway/extension_transport_completion_test.go")
    replacements = {str(args.host / name): str(args.inputs / name) for name in paths
        if (args.inputs / name).exists()}
    if args.prior_transport:
        if args.phase != "red":
            parser.error("--prior-transport is only for causal negatives")
        replacements[str(args.host / paths[0])] = str(args.prior_transport.resolve())
    if args.decoder_overlay:
        replacements[str(args.host / "internal/gateway/continuation.go")] = str(args.decoder_overlay.resolve())
    overlay = proof / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replacements}, indent=2) + "\n")
    pins = {
        "GOROOT": "/tmp/magpie-sdk-v11-usbox-niq306rt/go-toolchain-1.26.8",
        "GOPATH": "/Users/maratkarimov/go", "GOMODCACHE": "/Users/maratkarimov/go/pkg/mod",
        "GOCACHE": "/Users/maratkarimov/.cache/go-build",
    }
    environment = pins | {
        "PATH": pins["GOROOT"] + "/bin:/usr/bin:/usr/sbin:/bin:/sbin:/Users/maratkarimov/.local/bin",
        "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOENV": "off",
        "GOFLAGS": "-mod=readonly -p=2", "GOMAXPROCS": "2", "CGO_ENABLED": "1",
        "LANG": "C.UTF-8", "PYTHONDONTWRITEBYTECODE": "1",
    }
    guard = Path("/tmp/magpie-commandcode-final-repin.YJ06ocvO/magpie-verification-network.py")
    if hashlib.sha256(guard.read_bytes()).hexdigest() != "09f2a8d3311566d21dafc87c118ef8d5e07cfba88f78dba3114e4cf124b2b57e":
        raise SystemExit("Existing network guard changed")
    tests = "^TestExtensionProviderTerminalClose"
    if args.phase == "green":
        tests += "|^TestExtensionProvider(StreamAndAcceptedReplayFence|TerminalOutcomePreservesTypedError|NativeReplayAfterReauthKeepsOriginalOperation)$"
    if args.test_pattern:
        tests = args.test_pattern
    commands = [("race", ["go", "test", "-race", "-tags", "nogui", "-overlay", str(overlay),
        "./internal/provider", "-run", tests, "-count=" + ("1" if args.phase == "red" else str(args.count)),
        "-json", "-timeout=900s"])]
    if (args.inputs / paths[-1]).exists():
        commands.append(("gateway-race", ["go", "test", "-race", "-tags", "nogui", "-overlay", str(overlay),
            "./internal/gateway", "-run", "^TestExtension(TranslatedReadersConfirmOnlyDecodedTerminal|ProtocolDecoderRecordsRejection)$",
            "-count=" + ("1" if args.phase == "red" else str(args.count)), "-json", "-timeout=900s"]))
    if args.phase == "binary":
        commands = [("binary-build", ["go", "build", "-tags", "nogui", "-overlay", str(overlay),
            "-o", str(proof / "magpie"), "."])]
    if args.phase == "green":
        commands += [
            ("format", ["gofmt", "-l", *[str(args.inputs / name) for name in paths if (args.inputs / name).exists()]]),
            ("vet", ["go", "vet", "-tags", "nogui", "-overlay", str(overlay), "./internal/provider", "./internal/gateway"]),
            ("build", ["go", "build", "-tags", "nogui", "-overlay", str(overlay), "./internal/provider", "./internal/gateway"]),
        ]
    if args.only:
        commands = [(name, command) for name, command in commands if name in args.only]
    results = []
    for name, command in commands:
        home = Path(tempfile.mkdtemp(prefix="commandcode-host-" + name + "-", dir=proof))
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
            result["exit"] = 1
        results.append(result)
        (proof / "results.json").write_text(json.dumps({
            "phase": args.phase, "host": str(args.host), "overlay": replacements,
            "inputFiles": {name: hashlib.sha256((args.inputs / name).read_bytes()).hexdigest() for name in paths
                if (args.inputs / name).exists()},
            "baseTransportSHA256": hashlib.sha256((args.host / paths[0]).read_bytes()).hexdigest(),
            "priorTransportSHA256": hashlib.sha256(args.prior_transport.read_bytes()).hexdigest() if args.prior_transport else None,
            "decoderOverlay": {"path": str(args.decoder_overlay), "sha256":
                hashlib.sha256(args.decoder_overlay.read_bytes()).hexdigest()} if args.decoder_overlay else None,
            "environment": environment, "results": results,
        }, indent=2) + "\n")
        print(json.dumps({key: result[key] for key in ("name", "exit", "counts", "seconds")}), flush=True)
        if result["exit"] and args.phase != "red":
            break
    if args.phase == "red":
        raise SystemExit(0 if results[0]["counts"]["fail"] else 1)
    raise SystemExit(int(any(result["exit"] for result in results)))


if __name__ == "__main__":
    main()
