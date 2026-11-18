"""Build and install real native modules in bounded ordinary Front fixtures."""

import hashlib
import json
import os
import socket
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import shutil
sys.dont_write_bytecode = True
from request_policy_snapshot import frozen_sdk

packages = Path(__file__).resolve().parents[2]
packet = Path(sys.argv[1]).resolve()
phase = sys.argv[2]
guard = Path("/tmp/magpie-upstream-signing-closure.ofRrrBdz/magpie-verification-network.py")
if len(sys.argv) == 3:
    os.execv(sys.executable, [sys.executable, str(guard), str(packet / (phase + "-network.json")),
                            sys.executable, __file__, str(packet), phase, "guarded"])
sdk, sdk_freeze = frozen_sdk(packet)
pins = json.loads(Path("/tmp/magpie-obs-policy-fix-20261115/PINNED-GO-ENV.json").read_text())
home = Path(tempfile.mkdtemp(prefix=phase + "-home-", dir=packet))
env = pins | dict(HOME=str(home), USERPROFILE=str(home), TMPDIR=str(home),
    PATH=pins["GOROOT"] + "/bin:/usr/bin:/bin:/usr/sbin:/sbin",
    XDG_CONFIG_HOME=str(home / "config"), XDG_CONFIG_DIRS=str(home / "config-dirs"),
    XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
    XDG_DATA_DIRS=str(home / "data-dirs"), XDG_STATE_HOME=str(home / "state"),
    APPDATA=str(home / "appdata"), LOCALAPPDATA=str(home / "localappdata"),
    GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOWORK="off", GOENV="off",
    GOTELEMETRY="off", CGO_ENABLED="1", GOMAXPROCS="2", GOFLAGS="-mod=readonly -p=2",
    PYTHONDONTWRITEBYTECODE="1")
os.environ.clear()
os.environ.update(env)
baseline = phase.startswith("native-red")
records = []
overlay = packet / (phase + "-overlay.json")
overlay.write_text(json.dumps({"Replace": {
    str(sdk / "internal/gateway/sol_request_policy_test.go"):
        str(packages / "commandcode/tests/request_policy_host_test.go.txt")
}}))

def run(name, command, cwd, environment=env):
    log = packet / (phase + "-" + name + ".log")
    started = time.monotonic()
    with log.open("xb") as output:
        result = subprocess.run(command, cwd=cwd, env=environment, stdout=output, stderr=subprocess.STDOUT, timeout=300)
    record = dict(name=name, command=command, cwd=str(cwd), exit=result.returncode,
                  log=str(log), logSHA256=hashlib.sha256(log.read_bytes()).hexdigest(),
                  seconds=time.monotonic() - started)
    records.append(record)
    (packet / (phase + "-receipt.json")).write_text(json.dumps(dict(environment=env, sdk=str(sdk), sdkFreeze=sdk_freeze,
        guardSHA256=hashlib.sha256(guard.read_bytes()).hexdigest(), records=records), indent=2) + "\n")
    print(json.dumps(record), flush=True)
    return result.returncode

for name in ("commandcode", "opencode"):
    source = packages / name / "src"
    modfile = packet / (phase + "-" + name + ".mod")
    modfile.write_text((source / "go.mod").read_text().split("replace github.com/yetone/magpie")[0]
                       + "replace github.com/yetone/magpie => " + str(sdk) + "\n")
    modfile.with_suffix(".sum").write_bytes((source / "go.sum").read_bytes())
    goenv = env | {"GOFLAGS": env["GOFLAGS"] + " -modfile=" + str(modfile)}
    ordinary = packet / ("distribution-distribution-final04-" + name)
    distribution = packet / (phase + "-" + name + "-distribution")
    if not baseline and ordinary.exists():
        if name == "commandcode":
            distribution = ordinary
        else:
            shutil.copytree(ordinary, distribution)
        manifest = (distribution / "magpie-plugin.json").read_bytes()
    else:
        (distribution / "lib").mkdir(parents=True)
        (distribution / "web").mkdir()
        (distribution / "web/index.html").write_bytes((packages / name / "web/index.html").read_bytes())
        (distribution / "package.json").write_bytes((packages / name / "web/package.json").read_bytes())
        manifest = subprocess.check_output(["go", "run", "./cmd/manifest"], cwd=source, env=goenv)
        (distribution / "magpie-plugin.json").write_bytes(manifest)
    declaration = json.loads(manifest)
    endpoint = ""
    if name == "opencode":
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            endpoint = "http://127.0.0.1:" + str(listener.getsockname()[1])
    target = "./cmd/native" if name == "commandcode" else "./testdata/request-policy-native"
    if (name != "commandcode" or baseline or not ordinary.exists()) and run(name + "-build", ["go", "build", "-buildmode=c-shared",
                            *(["-ldflags", "-X main.endpoint=" + endpoint] if endpoint else []), "-o",
                            str(distribution / declaration["entry"]), target], source, goenv):
        raise SystemExit(1)
    scenarios = ["sibling"] if baseline else ["sibling", "smart", "long-delay", "request-terminal",
        "content-terminal", "budget", "model-budget", "other-model-budget", "transport", "uncertain", "output", "ambiguous"]
    if not baseline and name == "commandcode":
        scenarios.extend(["cancel-read", "jev-systemone", "jev-decisions", "jev-uncertain",
                          "jev-budget", "jev-request-terminal", "jev-content-terminal"])
    for scenario in scenarios:
        run(name + "-" + scenario, ["go", "test", "-tags", "nogui", "-overlay", str(overlay),
            "-run", "^TestSOLRequestPolicyOrdinaryFront$", "-count=1", "-timeout=90s", "-v",
            "./internal/gateway", "-args", "-sol-distribution", str(distribution),
            "-sol-package", name, "-sol-scenario", scenario, "-sol-endpoint", endpoint,
            "-sol-baseline=" + str(baseline).lower()], sdk)

if baseline:
    failures = [record for record in records if record["exit"]]
    assert len(failures) == 2
    assert all("eligible sibling was not selected" in Path(record["log"]).read_text() for record in failures)
else:
    raise SystemExit(int(any(record["exit"] for record in records)))
