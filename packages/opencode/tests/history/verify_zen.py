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
plugin = packet / "plugin"
settings = json.loads(Path("/tmp/magpie-opencode-account-rugrjcv2/test-env.json").read_text())
home = Path(tempfile.mkdtemp(prefix="home-", dir=packet))
env = dict(os.environ, **settings)
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
           XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
           XDG_STATE_HOME=str(home / "state"), GOFLAGS="-mod=readonly -p=4", GOMAXPROCS="4")
env["PATH"] = settings["PATH"].split(":")[0] + ":/Users/maratkarimov/.local/bin:/usr/bin:/bin"
go = settings["PATH"].split(":")[0] + "/go"
phase = sys.argv[2] if len(sys.argv) > 2 else "unit"
if phase == "tools":
    commands = [
        ("tool-oracle", ["node", "testdata/zen_tools_oracle.mjs", "--check"], plugin),
        ("tool-format", ["gofmt", "-w", "zen_client.go", "zen_tools.go", "zen_tool_stream.go", "zen_tools_test.go"], plugin),
        ("tool-race", [go, "test", "-json", "-race", "-count=1", "-timeout=2m", "-run", "^TestZen|^TestConsole|^TestWindow", "./..."], plugin),
    ]
elif phase == "unit":
    commands = [
        ("donor-replies", ["node", "testdata/donor_replies.mjs", "testdata/donors/zen-replies.json"], plugin),
        ("format", ["gofmt", "-w", *sorted(str(path.relative_to(plugin)) for path in plugin.rglob("*.go"))], plugin),
        ("zen-race", [go, "test", "-json", "-race", "-count=1", "-timeout=2m", "-run", "^TestZen|^TestConsole|^TestWindow", "./..."], plugin),
    ]
elif phase in ("gates", "tools-package"):
    builds = Path(tempfile.mkdtemp(prefix=phase+"-packages-", dir=packet))
    (packet/"tool-package-path.json").write_text(json.dumps({"directory": str(builds)})+"\n")
    commands = ([
        ("race20", [go, "test", "-json", "-race", "-count=20", "-timeout=5m", "-run", "^TestZen|^TestConsole|^TestWindow", "./..."], plugin),
        ("all-race3", [go, "test", "-json", "-race", "-count=3", "-timeout=5m", "./..."], plugin),
        ("vet", [go, "vet", "./..."], plugin),
        ("linux-vet", [go, "vet", "-tags", "nogui", "./..."], plugin),
    ] if phase == "gates" else []) + [
        ("format-check", ["gofmt", "-l", "."], plugin),
        ("python-static", ["python3", "-m", "py_compile", "build.py", *sorted(str(path.relative_to(plugin)) for path in (plugin / "testdata").glob("*.py") if not path.name.startswith("."))], plugin),
        ("provider-build", ["python3", "build.py", str(builds / "provider-dist"), "--compat-provider"], plugin),
        ("agent-build", ["python3", "build.py", str(builds / "agent-dist"), "--compat-agent"], plugin),
        ("window-build", ["python3", "build.py", str(builds / "window-dist")], plugin),
        *[(name+"-package", ["python3", "testdata/check_distribution.py", str(builds/(name+"-dist"))], plugin)
          for name in ("provider", "agent", "window")],
    ]
elif phase == "platform":
    commands = [
        (target+"-tool-vet", ["env", "GOOS="+system, "GOARCH="+architecture, "CGO_ENABLED=0", go, "vet", "."], plugin)
        for target, system, architecture in (("darwin-arm64", "darwin", "arm64"),
                                             ("darwin-amd64", "darwin", "amd64"),
                                             ("windows", "windows", "amd64"))
    ]
elif phase == "ready":
    commands = [
        ("ready-format", ["gofmt", "-w", *sorted(str(path.relative_to(plugin)) for path in plugin.glob("*.go"))], plugin),
        ("ready-scoped", [go, "test", "-json", "-race", "-count=1", "-timeout=2m", "-run", "^TestZen|^TestConsole|^TestWindow", "./..."], plugin),
        ("ready-build", ["python3", "build.py", str(packet/"ready-dist"), "--window"], plugin),
        ("ready-package", ["python3", "testdata/check_distribution.py", str(packet/"ready-dist")], plugin),
        ("native-readiness", [str(packet/"loopback-guard"), str(packet/"ready-network-guard.json"), "python3", "testdata/full_e2e.py",
                              str(packet/"magpie-host"), str(packet/"ready-dist"), str(packet/"native-readiness"), "--zen-console"], plugin),
    ]
elif phase == "history":
    commands = [
        ("history-format", ["gofmt", "-w", "history_contract_test.go"], plugin),
        ("history-boundary", [go, "test", "-json", "-race", "-count=20", "-timeout=2m",
                              "-run", "^TestHistory|^TestDonorFixture", "."], plugin),
        ("history-storage", ["python3", "testdata/history_contract.py", str(packet/"history-contract")], plugin),
    ]
elif phase == "owner":
    builds = Path(json.loads((packet/"tool-package-path.json").read_text())["directory"])
    commands = [
        ("owner-e2e", [str(packet/"loopback-guard"), str(packet/"owner-network-guard.json"), "python3", "testdata/full_e2e.py",
                       str(packet/"magpie-host"), str(builds/"window-dist"),
                       str(packet/("native-owner-"+str(time.time_ns()))), "--zen-console"], plugin),
    ]
elif phase == "native":
    commands = [
        ("host-build", [go, "build", "-tags", "nogui", "-ldflags", "-X main.version=0.0.0-opencode-fixture", "-o", str(packet/"magpie-host"), "."], packet/"magpie"),
        ("guard-build", ["gcc", "-std=c11", "-Wall", "-Wextra", "-Werror", "-O2", "-o", str(packet/"loopback-guard"), "testdata/loopback_guard.c"], plugin),
        ("native-e2e", [str(packet/"loopback-guard"), str(packet/"network-guard.json"), "python3", "testdata/full_e2e.py",
                        str(packet/"magpie-host"), str(packet/"window-dist"), str(packet/"native-e2e"), "--zen-console"], plugin),
        ("native-renewal", [str(packet/"loopback-guard"), str(packet/"renewal-network-guard.json"), "python3", "testdata/native_refresh.py",
                            str(packet/"magpie-host"), str(packet/"native-renewal"), "--ordinary-renewal", "--all-models"], plugin),
    ]
else:
    raise SystemExit("unknown phase")
rows = []
for name, arguments, directory in commands:
    started = time.monotonic()
    with (packet/(name+".log")).open("w") as log:
        result = subprocess.run(arguments, cwd=directory, env=env, stdout=log, stderr=subprocess.STDOUT)
    content = (packet/(name+".log")).read_text()
    row = {"name":name,"exit":result.returncode,"seconds":round(time.monotonic()-started,2)}
    if name == "format-check" and content:
        row.update(exit=1, findings=content.splitlines())
    if "-json" in arguments:
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
        row.update(tests=dict(counts),failures=failures)
    rows.append(row)
    (packet/(phase+"-results.json")).write_text(json.dumps(rows,indent=2)+"\n")
    print(json.dumps(row),flush=True)
    if row["exit"] and phase != "platform":
        raise SystemExit(row["exit"])
if any(row["exit"] for row in rows):
    raise SystemExit(1)
