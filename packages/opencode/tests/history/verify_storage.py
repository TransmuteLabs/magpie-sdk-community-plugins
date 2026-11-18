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
home = Path(tempfile.mkdtemp(prefix="storage-home-", dir=packet))
env = dict(os.environ, **settings)
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
           XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
           XDG_STATE_HOME=str(home / "state"), GOFLAGS="-mod=readonly -p=4", GOMAXPROCS="4")
env["PATH"] = settings["PATH"].split(":")[0] + ":/Users/maratkarimov/.local/bin:/usr/bin:/bin"
go = settings["PATH"].split(":")[0] + "/go"
host = packet / "magpie"
plugin = packet / "plugin"
distribution = packet / "history-package-v8"
env.update(MAGPIE_STORAGE_TEST_DIST=str(distribution),
           MAGPIE_STORAGE_TEST_WRITER=str(plugin / "testdata/storage_writer.py"),
           MAGPIE_STORAGE_TEST_HOST=str(packet / "storage-host"))
if phase == "sdk":
    commands = [
        ("sdk-storage", [go, "test", "-json", "-race", "-count=1", "-timeout=2m", "-run",
                         "^TestStorage", "./pkg/extension", "./pkg/extension/native", "./internal/extension"], host),
    ]
elif phase == "compile":
    commands = [
        ("host-storage-compile", [go, "test", "-tags=nogui", "-json", "-run", "^$", "-timeout=3m",
                                 "./pkg/extension", "./pkg/extension/native", "./internal/extension",
                                 "./internal/sessions", "./internal/gui"], host),
    ]
elif phase == "runtime-lock":
    commands = [
        ("module-runtime-lock", [go, "list", "-mod=mod", "-deps", "."], plugin),
    ]
elif phase == "contracts":
    commands = [
        ("sdk-storage-shared-contracts", [go, "test", "-tags=nogui", "-json", "-race", "-count=1",
                                         "-timeout=3m", "./pkg/extension", "./pkg/extension/native"], host),
        ("js-storage-shared-contracts", ["node", "--test", "test/storage-source.test.js",
                                         "test/session-source.test.js", "test/search-sources.test.js",
                                         "test/composite.test.js", "test/diagnostics.test.js",
                                         "test/usage-facts.test.js", "test/catalog-projection.test.js",
                                         "test/own-calls.test.js", "test/gateway-hooks.test.js",
                                         "test/refresh-contract.test.js"], host / "sdk/js"),
        ("ts-storage-shared-contracts", ["tsc", "--noEmit", "--strict", "--skipLibCheck", "--target", "es2022",
                                         "--module", "nodenext", "--moduleResolution", "nodenext",
                                         "test/storage-source-types.ts", "test/search-source-types.ts",
                                         "test/contract-version-types.ts"], host / "sdk/js"),
        ("broker-storage-diagnostics", [go, "test", "-tags=nogui", "-json", "-race", "-count=1",
                                        "-timeout=3m", "-run", "^TestStorage|^TestModuleDiagnostics",
                                        "./internal/extension"], host),
    ]
elif phase == "authoring":
    commands = [
        ("js-storage-authoring", ["node", "--test", "test/storage-source.test.js"], host / "sdk/js"),
        ("ts-storage-authoring", ["tsc", "--noEmit", "--strict", "--skipLibCheck", "--target", "es2022",
                                  "--module", "nodenext", "--moduleResolution", "nodenext",
                                  "test/storage-source-types.ts"], host / "sdk/js"),
    ]
elif phase == "static":
    commands = [
        ("module-storage-vet", [go, "vet", "./..."], plugin),
        ("host-storage-vet", [go, "vet", "-tags=nogui", "./pkg/extension", "./pkg/extension/native",
                               "./internal/extension", "./internal/sessions", "./internal/gui"], host),
    ]
elif phase == "module":
    commands = [
        ("module-storage", [go, "test", "-json", "-race", "-count=1", "-timeout=3m", "-run",
                            "^TestHistory|^TestStorage", "."], plugin),
    ]
elif phase == "module-all":
    commands = [
        ("module-all", [go, "test", "-json", "-race", "-count=1", "-timeout=3m", "./..."], plugin),
    ]
elif phase in ("ordinary-build", "reader-build"):
    commands = [
        *([("storage-full-build", [sys.executable, "build.py", str(distribution)], plugin)] if phase == "ordinary-build" else []),
        ("storage-native-build", [go, "build", "-buildmode=c-shared", "-o",
                                   str(distribution / "lib/opencode.so"), "./testdata/storage-native"], plugin),
        ("storage-manifest", [go, "run", "./testdata/storage-manifest"], plugin),
        *([("storage-host-build", [go, "build", "-tags=nogui", "-o", str(packet / "storage-host"), "."], host)] if phase == "ordinary-build" else []),
    ]
elif phase == "ordinary":
    commands = [
        ("storage-ordinary-sessions", [go, "test", "-tags=nogui", "-json", "-race", "-count=1",
                                       "-timeout=4m", "-run", "^TestStorageNative", "./internal/sessions"], host),
        ("storage-ordinary-http", [go, "test", "-tags=nogui", "-json", "-race", "-count=1",
                                   "-timeout=4m", "-run", "^TestStorage", "./internal/gui"], host),
    ]
elif phase == "cli":
    commands = [
        ("storage-cli-format", [str(Path(go).with_name("gofmt")), "-w", "internal/cli/main.go",
                                "internal/cli/session_sources_cli.go", "internal/cli/settings_parity_test.go",
                                "internal/cli/storage_sources_cli_test.go"], host),
        ("storage-cli", [go, "test", "-tags=nogui", "-json", "-race", "-count=1", "-timeout=3m",
                         "-run", "^TestEveryRouteHasACommandOrIsNamed$|^TestStorageSourceCLI|^TestSessionSourceCLIReadAndHelpCreateNoState$",
                         "./internal/cli"], host),
        ("storage-cli-vet", [go, "vet", "-tags=nogui", "./internal/cli"], host),
    ]
elif phase == "http":
    commands = [
        ("storage-loopback-build", ["gcc", "-std=c11", "-Wall", "-Wextra", "-Werror", "-O2", "-o",
                                    str(packet / "storage-loopback-guard"), "testdata/loopback_guard.c"], plugin),
        ("storage-http-e2e", [str(packet / "storage-loopback-guard"), str(packet / "storage-network.json"),
                              sys.executable, "testdata/storage_e2e.py", str(packet / "storage-host"),
                              str(distribution), str(packet / ("storage-http-" + str(time.time_ns())))], plugin),
    ]
else:
    raise SystemExit("unknown storage phase")
rows = []
for name, arguments, directory in commands:
    started = time.monotonic()
    with (packet / (name + ".log")).open("w") as log:
        result = subprocess.run(arguments, cwd=directory, env=env, stdout=log, stderr=subprocess.STDOUT)
    content = (packet / (name + ".log")).read_text()
    if result.returncode == 0 and name == "storage-manifest":
        manifest = json.loads(content)
        manifest["entry"] = "lib/opencode.so"
        (distribution / "magpie-plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")
        lock = json.loads((plugin / "source-lock.json").read_text())
        lock["plugin"]["sdkMinimum"] = str(manifest["contract"]["version"]["major"]) + "." + str(manifest["contract"]["version"]["minor"])
        (distribution / "source-lock.json").write_text(json.dumps(lock, indent=2) + "\n")
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
    row = {"name": name, "exit": result.returncode, "seconds": round(time.monotonic() - started, 2),
           "tests": dict(counts), "failures": failures, "home": str(home)}
    rows.append(row)
    (packet / (phase + "-results.json")).write_text(json.dumps(rows, indent=2) + "\n")
    print(json.dumps(row), flush=True)
    if row["exit"]:
        print(content[-8000:], flush=True)
        raise SystemExit(row["exit"])
