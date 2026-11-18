#!/usr/bin/env python3
import collections
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

packet = Path(sys.argv[1]).resolve()
phase = sys.argv[2]
settings = json.loads(Path("/tmp/magpie-opencode-account-rugrjcv2/test-env.json").read_text())
home = Path(tempfile.mkdtemp(prefix="darwin-home-", dir=packet))
env = dict(os.environ, **settings)
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
           XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
           XDG_STATE_HOME=str(home / "state"), GOFLAGS="-mod=readonly -p=4", GOMAXPROCS="4")
env["PATH"] = settings["PATH"].split(":")[0] + ":/Users/maratkarimov/.local/bin:/usr/bin:/bin"
go = settings["PATH"].split(":")[0] + "/go"
plugin, host = packet / "plugin", packet / "magpie"
host_files = [
    "pkg/extension/native/storage_reader.go", "pkg/extension/native/storage_reader_unix.go",
    "pkg/extension/native/storage_reader_other.go", "pkg/extension/native/storage_reader_unix_test.go",
    "internal/extension/storage_grant.go", "internal/extension/storage_grant_unix.go",
    "internal/extension/storage_grant_other.go", "internal/extension/storage_grant_lifetime.go",
    "internal/extension/storage_grant_lifetime_linux_test.go", "internal/extension/client.go",
    "internal/sessions/extension_storage_root_unix.go", "internal/sessions/extension_storage_root_other.go",
]
packages = ["./pkg/extension/native", "./internal/extension", "./internal/sessions"]
if phase == "format":
    commands = [
        ("darwin-module-format", ["gofmt", "-w", *sorted(str(path.relative_to(plugin)) for path in plugin.glob("history*.go"))], plugin),
        ("darwin-host-format", ["gofmt", "-w", *host_files], host),
    ]
elif phase == "unit":
    commands = [
        ("darwin-root-siblings", [go, "test", "-tags=nogui", "-json", "-race", "-count=3", "-timeout=3m",
                                  "-run", "^TestStorage", "./pkg/extension/native", "./internal/extension"], host),
        ("darwin-vfs-siblings", [go, "test", "-json", "-race", "-count=3", "-timeout=3m",
                                 "-run", "^TestHistory|^TestStorage", "."], plugin),
    ]
elif phase == "module":
    commands = [
        ("darwin-module-all-race", [go, "test", "-json", "-race", "-count=3", "-timeout=5m", "./..."], plugin),
        ("darwin-zen-donor-oracle", ["node", "testdata/zen_tools_oracle.mjs", "--check"], plugin),
    ]
elif phase == "stress":
    commands = [
        ("darwin-broker-lifetime-race20", [go, "test", "-tags=nogui", "-json", "-race", "-count=20", "-timeout=5m",
                                          "-run", "^TestStorageMetadata|^TestStorageCanceled|^TestStorageSerial",
                                          "./internal/extension"], host),
        ("darwin-posix-wal-race20", [go, "test", "-json", "-race", "-count=20", "-timeout=5m",
                                    "-run", "^TestHistoryPOSIX", "."], plugin),
    ]
elif phase == "owner":
    commands = [
        ("darwin-ordinary-full-owner", [str(packet / "storage-loopback-guard"),
                                        str(packet / "darwin-full-owner-network.json"), "python3",
                                        "testdata/full_e2e.py", str(packet / "storage-host"),
                                        str(packet / "history-package-v8"),
                                        str(packet / ("darwin-full-owner-" + str(time.time_ns()))),
                                        "--zen-console"], plugin),
    ]
elif phase == "metadata":
    destination = Path(tempfile.mkdtemp(prefix="darwin-default-package-", dir=packet)) / "distribution"
    commands = [
        ("darwin-source-lock", [go, "test", "-json", "-race", "-count=1", "-timeout=2m",
                                "-run", "^TestSourceLock|^TestDonorFixture|^TestOrdinaryManifest", "."], plugin),
        ("darwin-final-format-check", ["gofmt", "-l", "."], plugin),
        ("darwin-final-python-static", ["python3", "-m", "py_compile", "testdata/verify_darwin.py",
                                        "testdata/negative_storage_darwin.py", "testdata/full_e2e.py"], plugin),
        ("darwin-default-build", ["python3", "build.py", str(destination)], plugin),
        ("darwin-default-package", ["python3", "testdata/check_distribution.py", str(destination)], plugin),
    ]
elif phase == "static":
    commands = [
        ("darwin-linux-host-vet", [go, "vet", "-tags=nogui", *packages], host),
        ("darwin-linux-module-vet", [go, "vet", "./..."], plugin),
        ("darwin-module-format-check", ["gofmt", "-l", "."], plugin),
        ("darwin-host-format-check", ["gofmt", "-l", *host_files], host),
        ("darwin-python-static", ["python3", "-m", "py_compile", "build.py",
                                   *sorted(str(path.relative_to(plugin)) for path in (plugin/"testdata").glob("*.py") if not path.name.startswith("."))], plugin),
    ]
elif phase == "platform":
    commands = []
    for architecture in ("arm64", "amd64"):
        cross = ["env", "GOOS=darwin", "GOARCH="+architecture, "CGO_ENABLED=0"]
        commands.extend([
            ("darwin-"+architecture+"-host-vet", [*cross, go, "vet", "-tags=nogui", *packages], host),
            ("darwin-"+architecture+"-module-vet", [*cross, go, "vet", "./..."], plugin),
            ("darwin-"+architecture+"-host-build", [*cross, go, "build", "-tags=nogui", *packages], host),
            ("darwin-"+architecture+"-module-build", [*cross, go, "build", "./..."], plugin),
        ])
    commands.append(("darwin-other-platform-refusal-vet",
                     ["env", "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", go, "vet", "."], plugin))
else:
    raise SystemExit("unknown Darwin verification phase")

rows = []
for name, arguments, directory in commands:
    started = time.monotonic()
    with (packet/(name+".log")).open("w") as output:
        result = subprocess.run(arguments, cwd=directory, env=env, stdout=output, stderr=subprocess.STDOUT)
    content = (packet/(name+".log")).read_text()
    counts, failures = collections.Counter(), []
    for line in content.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if event.get("Test"):
            counts[event.get("Action")] += 1
        if event.get("Action") == "fail":
            failures.append(event.get("Test", event.get("Package")))
    row = {"name": name, "exit": result.returncode, "seconds": round(time.monotonic()-started, 2),
           "tests": dict(counts), "failures": failures, "runnerPlatform": sys.platform}
    if name.endswith("format-check") and content:
        row.update(exit=1, findings=content.splitlines())
    rows.append(row)
    (packet/("darwin-"+phase+"-results.json")).write_text(json.dumps(rows, indent=2)+"\n")
    print(json.dumps(row), flush=True)
    if row["exit"]:
        print(content[-6000:], flush=True)
        raise SystemExit(row["exit"])
