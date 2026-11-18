"""Run inside the existing usbox network guard with pinned offline caches."""

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile


root = Path(sys.argv[1]).resolve()
stage = sys.argv[2]
receipt = os.environ.get("MINIMAX_POLICY_RECEIPT", stage)
if any((root / (receipt + suffix)).exists() for suffix in (".log", "-result.json")):
    raise RuntimeError("refusing to overwrite existing receipt: " + receipt)
sdk = Path(os.environ.get("MINIMAX_POLICY_SDK", "/tmp/magpie-policy-combined-mbp6ictj/source"))
pins = json.loads(Path("/tmp/magpie-sdk-v11-usbox-niq306rt/go-env-external-toolchain.json").read_text())
workspace = root / ("red" if stage in ("red", "red-worker-build", "read-red", "read-host-red") else "green")
package = workspace / "packages/minimax"
home = Path(tempfile.mkdtemp(prefix=receipt + "-home-", dir=root))
environment = dict(pins, PATH=pins["GOROOT"] + "/bin:/usr/bin:/bin", HOME=str(home),
                   USERPROFILE=str(home), GOENV="off", GOTELEMETRY="off", GOPROXY="off",
                   GOSUMDB="off", GOTOOLCHAIN="local", GOWORK="off", CGO_ENABLED="1",
                   GOOS="linux", GOARCH="amd64", GOMAXPROCS="2", LC_ALL="C.UTF-8")
for key in ("XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
            "APPDATA", "LOCALAPPDATA", "TMPDIR"):
    directory = home / key.lower()
    directory.mkdir()
    environment[key] = str(directory)
modfile = home / "plugin.mod"
modfile.write_text(re.sub(r"(?m)^replace github\.com/yetone/magpie => .*?$",
                         lambda _: "replace github.com/yetone/magpie => " + str(sdk),
                         (package / "src/go.mod").read_text()))
shutil.copyfile(package / "src/go.sum", modfile.with_suffix(".sum"))
environment["GOFLAGS"] = "-mod=readonly -modfile=" + str(modfile)
go = pins["GOROOT"] + "/bin/go"
commands = {
    "red": [go, "test", "-count=1", "-timeout=2m", "-run", "^TestRequestPolicy", "."],
    "redirect-red": [go, "test", "-count=1", "-timeout=2m", "-run", "^TestRequestPolicyRedirect", "."],
    "green": [go, "test", "-race", "-count=1", "-timeout=2m", "-run", "^TestRequestPolicy", "."],
    "read-red": [go, "test", "-count=1", "-timeout=2m", "-run", "^TestReadPolicy", "."],
    "read-green": [go, "test", "-race", "-count=1", "-timeout=2m", "-run", "^TestReadPolicy", "."],
    "suite": [go, "test", "-race", "-count=1", "-timeout=5m", "./..."],
    "vet": [go, "vet", "./..."],
    "static": ["/usr/bin/python3", "-I", "-c",
               "import ast,hashlib,json,pathlib; "
               "files=[pathlib.Path('../tests')/name for name in "
               "('policy_host.py','read_policy_host.py','verify_policy.py','host_e2e.py')]; "
               "[ast.parse(source.read_text(),filename=str(source)) for source in files]; "
               "lock=json.loads(pathlib.Path('../docs/source-lock.json').read_text()); "
               "assert lock['plugin']['sdkMinimum']=='1.12'; "
               "assert lock['sources'][0]['version']=='0.2.9'; "
               "assert lock['sources'][0]['commit']=='57d1cc6a9f49389d495ea9be8d986c70de7767bf'; "
               "assert all(hashlib.sha256((pathlib.Path('..')/name).read_bytes()).hexdigest()==entry['sha256'] "
               "for name,entry in lock['copiedFixtures'].items()); "
               "print('Python AST, SDK/donor floors and copied fixture hashes: PASS')"],
    "format": [pins["GOROOT"] + "/bin/gofmt", "-w",
               *map(str, sorted((package / "src").rglob("*.go"))),
               *map(str, sorted((package / "tests").glob("*.go")))],
    "format-check": [pins["GOROOT"] + "/bin/gofmt", "-l",
                     *map(str, sorted((package / "src").rglob("*.go"))),
                     *map(str, sorted((package / "tests").glob("*.go")))],
    "worker-build": [go, "build", "-trimpath", "-o", str(root / "worker"), "../tests/policy_worker.go"],
    "red-worker-build": [go, "build", "-trimpath", "-o", str(root / "red-worker"), "../tests/policy_worker.go"],
    "build": [go, "build", "-trimpath", "./..."],
    "native-build": [go, "build", "-trimpath", "-buildmode=c-shared", "-o", str(root / "minimax.so"), "./cmd/native"],
    "manifest": [go, "run", "./cmd/manifest"],
    "permit-contract": [go, "test", "-count=1", "-timeout=30s", "../tests/permit_contract_test.go"],
    "read-host": ["/usr/bin/python3", "-I", str(package / "tests/read_policy_host.py"),
                  "--host", str(root / "host"), "--worker", str(root / "worker"),
                  "--output", str(root / (receipt + "-evidence")),
                  *([] if not os.environ.get("MINIMAX_READ_MODES") else
                    ["--modes", *os.environ["MINIMAX_READ_MODES"].split(",")])],
    "read-host-diagnostics": ["/usr/bin/python3", "-I", str(package / "tests/read_policy_host.py"),
                              "--host", str(root / "host"), "--worker", str(root / "worker"),
                              "--output", str(root / (receipt + "-evidence")),
                              "--require-diagnostics", "--modes", "disable"],
    "read-host-red": ["/usr/bin/python3", "-I", str(package / "tests/read_policy_host.py"),
                      "--host", str(root / "host"), "--worker", str(root / "red-worker"),
                      "--output", str(root / "read-host-red-evidence"), "--modes", "search"],
    "host-build": [go, "build", "-tags", "nogui", "-trimpath", "-o", str(root / "host"), "."],
    "host": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
             "--host", str(root / "host"), "--worker", str(root / "worker"),
             "--output", str(root / "host-evidence")],
    "host-red": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                 "--host", str(root / "host"), "--worker", str(root / "red-worker"),
                 "--output", str(root / "host-red-evidence"), "--modes", "fallback"],
    "host-control": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                     "--host", str(root / "host"), "--worker", str(root / "worker"),
                     "--output", str(root / "host-control-evidence"), "--protocol", "chat", "--modes", "success"],
    "host-old-control": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                         "--host", str(root / "host"), "--worker", str(root / "red-worker"),
                         "--output", str(root / "host-old-control-evidence"), "--protocol", "chat", "--modes", "success"],
    "host-messages": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                      "--host", str(root / "host"), "--worker", str(root / "worker"),
                      "--output", str(root / "host-messages-evidence"), "--protocol", "messages",
                      "--modes", "success", "fallback"],
    "chat": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
             "--host", str(root / "host"), "--worker", str(root / "worker"),
             "--output", str(root / "chat-evidence"), "--protocol", "chat",
             "--modes", "success", "fallback"],
    "chat-old": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                 "--host", str(root / "host"), "--worker", str(root / "red-worker"),
                 "--output", str(root / "chat-old-evidence"), "--protocol", "chat", "--modes", "success"],
    "chat-whole-negative": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                            "--host", str(root / "host"), "--worker", str(root / "worker"),
                            "--output", str(root / "chat-whole-negative-evidence"), "--protocol", "chat",
                            "--success-format", "whole-json", "--modes", "success"],
    "chat-stream": ["/usr/bin/python3", "-I", str(package / "tests/policy_host.py"),
                    "--host", str(root / "host"), "--worker", str(root / "worker"),
                    "--output", str(root / "chat-stream-evidence"), "--protocol", "chat",
                    "--caller-stream", "--modes", "success"],
}
command = commands[stage]
cwd = package / "src"
if stage == "host-build":
    cwd = sdk
    environment["GOFLAGS"] = "-mod=readonly"
record = {"command": command, "cwd": str(cwd), "sdk": str(sdk), "home": str(home)}
print(json.dumps(record), flush=True)
try:
    with (root / (receipt + ".log")).open("x") as log:
        result = subprocess.run(command, cwd=cwd, env=environment, stdout=log,
                                stderr=subprocess.STDOUT, timeout=600)
    record["exit"] = result.returncode
finally:
    shutil.rmtree(home)
    record["homeRemoved"] = not home.exists()
    with (root / (receipt + "-result.json")).open("x") as output:
        output.write(json.dumps(record, indent=2) + "\n")
print((root / (receipt + ".log")).read_text()[-8000:], flush=True)
sys.exit(result.returncode)
