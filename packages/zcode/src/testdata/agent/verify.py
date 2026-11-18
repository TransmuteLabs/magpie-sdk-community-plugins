import argparse
import ctypes
import fcntl
import hashlib
import json
import os
from pathlib import Path
import socket
import struct
import subprocess
import tempfile
import zipfile


def isolate():
    uid, gid = os.getuid(), os.getgid()
    libc = ctypes.CDLL(None, use_errno=True)

    def checked(result):
        if result != 0:
            raise OSError(ctypes.get_errno(), os.strerror(ctypes.get_errno()))

    checked(libc.unshare(0x10000000))
    Path("/proc/self/setgroups").write_text("deny")
    Path("/proc/self/uid_map").write_text(f"{uid} {uid} 1\n")
    Path("/proc/self/gid_map").write_text(f"{gid} {gid} 1\n")
    checked(libc.unshare(0x40000000))
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as control:
        current = fcntl.ioctl(control, 0x8913, struct.pack("16sH14x", b"lo", 0))
        flags = struct.unpack("16sH14x", current)[1]
        fcntl.ioctl(control, 0x8914, struct.pack("16sH14x", b"lo", flags | 1))
    last = int(Path("/proc/sys/kernel/cap_last_cap").read_text())
    for capability in range(last + 1):
        checked(libc.prctl(24, capability, 0, 0, 0))
    checked(libc.prctl(47, 4, 0, 0, 0))

    class Header(ctypes.Structure):
        _fields_ = [("version", ctypes.c_uint32), ("pid", ctypes.c_int)]

    class Data(ctypes.Structure):
        _fields_ = [("effective", ctypes.c_uint32), ("permitted", ctypes.c_uint32),
                    ("inheritable", ctypes.c_uint32)]

    checked(libc.capset(ctypes.byref(Header(0x20080522, 0)), (Data * 2)()))
    masks = {}
    for line in Path("/proc/self/status").read_text().splitlines():
        if line.split()[0] in {"CapInh:", "CapPrm:", "CapEff:", "CapBnd:", "CapAmb:"}:
            name, value = line.split()
            masks[name.rstrip(":")] = int(value, 16)
    assert os.getuid() == uid and os.geteuid() == uid and len(masks) == 5 and not any(masks.values())
    assert list(socket.if_nameindex()) == [(1, "lo")]
    return {"uid": uid, "euid": os.geteuid(), "capabilities": masks, "interfaces": ["lo"]}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("stage", choices=["host", "unit", "causal", "package", "integration", "platform", "sqlite", "provenance"])
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--inventory", type=Path)
    args = parser.parse_args()
    root, source = args.root.resolve(), Path(__file__).resolve().parents[2]
    go = str(args.go.resolve())
    assert root != source and (args.stage == "host" or (root / "magpie-host").is_file())
    env = {"PATH": str(args.go.parent) + ":" + os.environ["PATH"], "HOME": os.environ["HOME"],
           "LANG": "C.UTF-8", "TZ": "UTC"}
    env.update(json.loads(subprocess.check_output([go, "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"], env=env)))
    isolation = isolate()
    home = Path(tempfile.mkdtemp(prefix=args.stage + "-home-", dir=root))
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
               XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
               LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
               GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
               CGO_ENABLED="1")
    report = {"isolation": isolation, "stage": args.stage, "results": []}
    if args.inventory:
        inventory_body = args.inventory.read_bytes()
        inventory = json.loads(inventory_body)
        for name, digest in inventory["files"].items():
            location = (source / name).resolve()
            assert location.is_relative_to(source)
            assert hashlib.sha256(location.read_bytes()).hexdigest() == digest, name
        host_source = next(item for item in json.loads((source.parent / "docs/source-lock.json").read_bytes())["sources"]
                           if item["id"] == "sdk-host")
        assert inventory["hostCommit"] == host_source["commit"]
        report.update(sourceInventorySHA256=hashlib.sha256(inventory_body).hexdigest(),
                      sourceFiles=len(inventory["files"]), hostCommit=host_source["commit"])

    def run(name, command, timeout=300, cwd=source, expected=0, assertion=None):
        log = root / (name + ".log")
        with log.open("wb") as output:
            process = subprocess.run(command, cwd=cwd, env=env, stdout=output,
                                     stderr=subprocess.STDOUT, timeout=timeout)
        result = {"name": name, "exit": process.returncode, "expectedExit": expected,
                  "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        if "-json" in command:
            counts = {"pass": 0, "fail": 0, "skip": 0}
            for line in log.read_text().splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if event.get("Test") and event.get("Action") in counts:
                    counts[event["Action"]] += 1
            result.update(counts)
        report["results"].append(result)
        (root / ("agent-" + args.stage + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(result), flush=True)
        if process.returncode != expected:
            print(log.read_text()[-7000:], flush=True)
            raise SystemExit(process.returncode or 1)
        if expected:
            if assertion:
                assert "AssertionError:" in log.read_text() and assertion in log.read_text(), "mutation missed its behavior assertion"
            else:
                assert result.get("fail", 0) > 0, "mutation did not reach a compiled test assertion"
        if name == "agent-format":
            assert not log.read_bytes(), "gofmt reported unformatted source"

    if args.stage == "host":
        run("agent-host-build", [go, "build", "-tags", "nogui", "-trimpath",
                                "-o", str(root / "magpie-host"), "."],
            timeout=600, cwd=root / "magpie")
    elif args.stage == "unit":
        run("agent-python-syntax", ["python3", "-I", "-c",
                                    "import ast,pathlib; [ast.parse(pathlib.Path(name).read_bytes()) for name in "
                                    "['testdata/agent/run.py','testdata/agent/verify.py','conformance.py']]"])
        run("agent-format", [str(args.go.parent / "gofmt"), "-l", "agent.go", "agent_test.go", "window.go"])
        run("agent-vet", [go, "vet", "./..."])
        run("agent-race", [go, "test", "-json", "-race", "-count=20", "-timeout=3m",
                           "-run", "^Test(OwnedAgent|Control|SourceLock)", "."])
        run("agent-module-regression", [go, "test", "-json", "-race", "-count=1",
                                        "-timeout=4m", "./..."], timeout=300)
    elif args.stage == "causal":
        original = (source / "agent.go").read_text()
        mutations = [
            ("manual-rules", "if manual[id] {", "if false && manual[id] {",
             "^TestOwnedAgentSyncKeepsUserFormatManualRulesAndSelectedOptions$"),
            ("foreign-restoration", 'if mapObject(entry)["providerId"] == "magpie" {', "if true {",
             "^TestOwnedAgentDisconnectRestoresOriginalAndPreservesForeignEdits$"),
            ("foreign-binding",
             "binding == nil || sdk.ValidateAgentBindingSnapshot(ControlManifest(), agentDescriptor(), binding) != nil",
             "binding == nil",
             "^TestOwnedAgentRefusesUnknownFilesMalformedSnapshotsAndForeignBinding$"),
        ]
        for name, before, after, test in mutations:
            assert before in original
            mutated = root / (name + ".go")
            mutated.write_text(original.replace(before, after, 1))
            overlay = root / (name + "-overlay.json")
            overlay.write_text(json.dumps({"Replace": {str(source / "agent.go"): str(mutated)}}))
            run("agent-mutation-" + name, [go, "test", "-overlay", str(overlay), "-json",
                                           "-count=1", "-timeout=1m", "-run", test, "."], expected=1)
        original = (source / "module.go").read_text()
        before = 'case "x-off-peak-ticket-id", "x-highspeed-card-id", "x-coding-plan-api-key", "x-bigmodel-authorization":'
        assert original.count(before) == 1
        mutated = root / "header-guard-disabled.go"
        mutated.write_text(original.replace(before, 'case "mutated-header-guard-disabled":'))
        overlay = root / "header-guard-disabled-overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(source / "module.go"): str(mutated)}}))
        run("agent-mutation-header-guard", ["python3", "-I", "testdata/agent/run.py",
                                           str(root / "magpie-host"), "--overlay", str(overlay)],
            timeout=180, expected=1, assertion="exit 0:")
    elif args.stage == "package":
        run("agent-full-package", ["python3", "-I", "testdata/parity/default_package.py", str(root), "agent-full-package", "--output", str(source.parent / "dist/distribution"), "--archive", str(source.parent / "dist")])
        run("agent-headless-package", ["python3", "-I", "../scripts/build.py", "--output", "../dist/compat-sdk14/distribution", "--archive", "../dist/compat-sdk14", "--headless"])
        for name, path in [("headless", source.parent / "dist/compat-sdk14/magpie-plugin_linux_amd64.zip"),
                           ("full", source.parent / "dist/magpie-plugin_linux_amd64.zip")]:
            directory = root / (name + "-artifact")
            directory.mkdir(exist_ok=True)
            with zipfile.ZipFile(path) as archive:
                archive.extractall(directory)
            run("agent-" + name + "-abi", ["python3", "-I", "../tests/conformance.py", str(directory)])
        env["ZCODE_PAGE_BUILD_DIR"] = str(root / "full-artifact/web")
        env["ZCODE_UI15_BUILD_DIR"] = str(root / "full-artifact/web")
        tests = [str(path) for path in sorted((source.parent / "web").glob("*.test.mjs"))]
        tests.append(str(source.parent / "docs/history/prototype/ui-sdk15/control.test.mjs"))
        run("agent-node-ui", ["node", "--test", *tests])
        run("agent-bun-ui", ["bun", "test", *tests])
    elif args.stage == "provenance":
        run("agent-source-provenance", ["python3", "-I", "testdata/parity/provenance.py", str(root)])
    elif args.stage == "sqlite":
        run("agent-native-sqlite", ["python3", "-I", "testdata/sqlite-rollback/run.py",
                                  "--sdk", str(root / "magpie"),
                                  "--distribution", str(root / "full-artifact"),
                                  "--output", str(root / "sqlite-owner-isolated")], timeout=300)
    elif args.stage == "platform":
        for platform, architecture in [("darwin", "arm64"), ("windows", "amd64")]:
            env.update(CGO_ENABLED="0", GOOS=platform, GOARCH=architecture)
            run("agent-" + platform + "-vet", [go, "vet", "."])
            run("agent-" + platform + "-build", [go, "build", "-o", str(root / ("zcode-" + platform)), "./cmd/native-ui"])
    else:
        run("agent-native-lifecycle", ["python3", "-I", "testdata/agent/run.py",
                                       str(root / "magpie-host")], timeout=180)
        run("agent-native-auth", ["python3", "-I", "../tests/auth_e2e.py", str(root / "magpie-host"), "--china"])
        run("agent-native-controls", ["python3", "-I", "../tests/ui_e2e.py", str(root / "magpie-host")])
        run("agent-native-controller-mcp-history", ["python3", "-I", "../tests/e2e.py",
                                                   str(root / "magpie-host")])
        run("agent-native-ui", ["python3", "-I", "../tests/ui_auth_e2e.py",
                                str(root / "magpie-host"), str(root / "magpie")])


if __name__ == "__main__":
    main()
