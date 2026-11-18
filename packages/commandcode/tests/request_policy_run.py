"""Bounded offline usbox runner. Keeps snapshots, homes and receipts."""

import hashlib
import ast
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
sys.dont_write_bytecode = True
from request_policy_snapshot import frozen_sdk

root = Path(__file__).resolve().parents[2]
packet = Path(sys.argv[1]).resolve()
phase = sys.argv[2]
if len(sys.argv) == 3:
    os.execvp("unshare", ["unshare", "--user", "--map-current-user", "--keep-caps", "--net",
                         sys.executable, __file__, str(packet), phase, "namespace"])
if sys.argv[3] == "namespace":
    for arguments in (["link", "set", "lo", "up"], ["link", "add", "fixture0", "type", "dummy"],
                      ["address", "add", "192.0.2.1/32", "dev", "fixture0"], ["link", "set", "fixture0", "up"]):
        subprocess.run(["ip", *arguments], check=True)
    os.execvp("setpriv", ["setpriv", "--bounding-set=-all", "--inh-caps=-all", "--ambient-caps=-all",
                         sys.executable, "/tmp/magpie-upstream-signing-closure.ofRrrBdz/magpie-verification-network.py",
                         str(packet / (phase + "-network.json")), sys.executable, __file__, str(packet), phase, "guarded"])

sdk, sdk_freeze = frozen_sdk(packet)
pins = json.loads(Path("/tmp/magpie-obs-policy-fix-20261115/PINNED-GO-ENV.json").read_text())
home = Path(tempfile.mkdtemp(prefix=phase + "-home-", dir=packet))
environment = pins | dict(HOME=str(home), USERPROFILE=str(home), TMPDIR=str(home),
    PATH=pins["GOROOT"] + "/bin:/Users/maratkarimov/.bun/bin:/Users/maratkarimov/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
    XDG_CONFIG_HOME=str(home / "config"), XDG_CONFIG_DIRS=str(home / "config-dirs"),
    XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
    XDG_DATA_DIRS=str(home / "data-dirs"), XDG_STATE_HOME=str(home / "state"),
    APPDATA=str(home / "appdata"), LOCALAPPDATA=str(home / "localappdata"),
    GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOWORK="off", GOENV="off",
    GOTELEMETRY="off", CGO_ENABLED="1", GOMAXPROCS="2", GOFLAGS="-mod=readonly -p=2",
    PYTHONDONTWRITEBYTECODE="1")
os.environ.clear()
os.environ.update(environment)
records = []
for name in ("commandcode", "opencode"):
    package = root / name
    source = package / "src"
    modfile = packet / (phase + "-" + name + ".mod")
    modfile.write_text((source / "go.mod").read_text().split("replace github.com/yetone/magpie")[0]
                       + "replace github.com/yetone/magpie => " + str(sdk) + "\n")
    modfile.with_suffix(".sum").write_bytes((source / "go.sum").read_bytes())
    env = environment | {"GOFLAGS": environment["GOFLAGS"] + " -modfile=" + str(modfile)}
    for path in [*package.joinpath("scripts").glob("*.py"), *package.joinpath("tests").glob("request_policy*.py")]:
        ast.parse(path.read_text(), filename=str(path))
    if phase.startswith("red"):
        commands = [["go", "test", "-run", "^TestRequestPolicyOriginalFinalRefusal$", "-count=1", "-v", "."]]
    elif phase.startswith("format"):
        original = packet / "input/packages" / name / "src"
        changed = [str(path) for path in source.rglob("*.go")
                   if not (original / path.relative_to(source)).exists()
                   or path.read_bytes() != (original / path.relative_to(source)).read_bytes()]
        if name == "commandcode":
            changed.append(str(package / "tests/request_policy_host_test.go.txt"))
        commands = [["gofmt", "-w", *changed]]
    elif phase.startswith("distribution"):
        helper = "/tmp/magpie-commandcode-monorepo-20261115/workspace/scripts/_workspace.py"
        bootstrap = ("import importlib.util,runpy;"
                     "spec=importlib.util.spec_from_file_location('_workspace'," + repr(helper) + ");"
                     "module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);"
                     "import sys;sys.modules['_workspace']=module;sys.path.insert(0,'scripts');"
                     "sys.argv=['scripts/build.py',*sys.argv[1:]];"
                     "runpy.run_path('scripts/build.py',run_name='__main__')")
        destination = packet / ("distribution-" + phase + "-" + name)
        commands = [["python3", "-c", bootstrap, "--output", str(destination), "--sdk", str(sdk),
                     *(["--archive"] if name == "commandcode" else [])]]
        archive = destination.parent / "protocol-commandcode-0.1.3-linux-amd64.zip"
        commands.append(["python3", str(package / "tests" / ("check_package.py" if name == "commandcode" else "check_distribution.py")),
                         str(archive if name == "commandcode" else destination)])
    else:
        commands = [
            ["go", "test", "-race", "-json", "-count=1", "-timeout=180s", "-run",
             "TestRequestPolicy|TestProvider|TestGenerate|TestSystemOne|TestNative|TestZen|TestGemini|TestPublicSDK|TestModuleManifest|TestDefaultFull|TestManifest|TestPackageProvenance|TestSourceLock|TestDonorFixture|TestDonorDelta|TestWindow|TestAgentDistribution", "."],
            ["go", "vet", "./..."], ["go", "build", "./..."], ["gofmt", "-l", "."]]
    for index, command in enumerate(commands):
        log = packet / (phase + "-" + name + "-" + str(index) + ".log")
        started = time.monotonic()
        with log.open("xb") as output:
            completed = subprocess.run(command, cwd=package if phase.startswith("distribution") else source,
                                       env=env, stdout=output, stderr=subprocess.STDOUT, timeout=360)
        exit_code = completed.returncode
        if command[:2] == ["gofmt", "-l"] and log.read_text().strip():
            exit_code = 1
        record = dict(package=name, command=command, exit=exit_code, log=str(log),
                      logSHA256=hashlib.sha256(log.read_bytes()).hexdigest(), seconds=time.monotonic() - started)
        records.append(record)
        print(json.dumps(record), flush=True)
        (packet / (phase + "-receipt.json")).write_text(json.dumps(dict(sdk=str(sdk), sdkFreeze=sdk_freeze, environment=environment, records=records), indent=2) + "\n")

if phase.startswith("red"):
    assert all(record["exit"] and "--- FAIL: TestRequestPolicyOriginalFinalRefusal" in Path(record["log"]).read_text()
               and "[build failed]" not in Path(record["log"]).read_text() for record in records)
else:
    raise SystemExit(int(any(record["exit"] for record in records)))
