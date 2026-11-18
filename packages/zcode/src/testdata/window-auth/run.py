import argparse
import contextlib
import hashlib
import http.cookiejar
import importlib.util
import json
import os
from pathlib import Path, PurePosixPath
import re
import socket
import subprocess
import time
import urllib.request


def fixture_module(name):
    spec = importlib.util.spec_from_file_location("zcode_auth_fixture_" + name, Path(__file__).with_name(name + ".py"))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


Vendor = fixture_module("vendor").Vendor
create_wrapper = fixture_module("wrappers").create_wrapper


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def verify_sdk(root, inventory, expected):
    if digest(inventory) != expected:
        raise AssertionError("SDK inventory does not match its frozen digest")
    entries = json.loads(inventory.read_bytes())
    if not isinstance(entries, dict) or not entries:
        raise AssertionError("frozen SDK inventory is empty")
    required = {
        "pkg/extension/window_auth.go", "internal/extension/window_auth.go",
        "internal/extension/window_auth_store.go", "internal/extension/store.go",
        "internal/gui/extension_window_auth.go", "internal/gui/pagebridge.js",
        "internal/gui/assets/extensionwindows.js", "sdk/js/ui.js",
    }
    if not required <= entries.keys():
        raise AssertionError("SDK inventory omits a required broker/Store/browser source")
    for name, value in entries.items():
        relative = PurePosixPath(name)
        if relative.is_absolute() or ".." in relative.parts or "\\" in name or not re.fullmatch("[0-9a-f]{64}", value):
            raise AssertionError("invalid frozen SDK inventory entry")
        if digest(root / name) != value:
            raise AssertionError("SDK source changed from its frozen inventory")


def port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def module_sources(root):
    return {
        str(path.relative_to(root)): digest(path)
        for path in sorted(root.rglob("*"))
        if path.is_file() and (path.suffix == ".go" or path.name in ("go.mod", "magpie-plugin.json"))
        and not any(part in (".git", "dist") for part in path.relative_to(root).parts)
    }


def stop(process):
    process.terminate()
    try:
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=5)


def main():
    parser = argparse.ArgumentParser(description="Unreleased auth-broker consumer proof; frozen host prerequisites required")
    parser.add_argument("--host", required=True, type=Path)
    parser.add_argument("--host-sha256", required=True)
    parser.add_argument("--sdk", required=True, type=Path)
    parser.add_argument("--sdk-files", required=True, type=Path)
    parser.add_argument("--sdk-files-sha256", required=True)
    parser.add_argument("--opencode", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    args.host, args.sdk, args.opencode = args.host.resolve(), args.sdk.resolve(), args.opencode.resolve()
    if digest(args.host) != args.host_sha256:
        raise AssertionError("host binary differs from frozen broker proof")
    verify_sdk(args.sdk, args.sdk_files, args.sdk_files_sha256)
    if os.getuid() != 1000 or os.geteuid() != 1000:
        raise AssertionError("proof requires the preserved usbox UID")
    status = {}
    for line in Path("/proc/self/status").read_text().splitlines():
        key, _, value = line.partition(":")
        if key in ("CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"):
            status[key] = value.strip()
    if len(status) != 5 or any(int(value, 16) for value in status.values()):
        raise AssertionError("proof requires all five capability masks zero")
    interfaces = {line.split(":", 1)[0].strip() for line in Path("/proc/net/dev").read_text().splitlines() if ":" in line}
    if interfaces != {"lo"}:
        raise AssertionError("proof requires a loopback-only network namespace")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    own = Path(__file__).resolve().parents[2]
    sources = {"zcode": module_sources(own), "opencode": module_sources(args.opencode)}
    (output / "module-source-hashes.json").write_text(json.dumps(sources, sort_keys=True, indent=2) + "\n")
    harness = {path.name: digest(path) for path in sorted(Path(__file__).parent.iterdir()) if path.suffix in (".py", ".mjs")}
    (output / "consumer-source-hashes.json").write_text(json.dumps(harness, sort_keys=True, indent=2) + "\n")
    home = output / "home"
    home.mkdir()
    env = os.environ.copy()
    env.update(json.loads(subprocess.check_output(["go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"])))
    for key in list(env):
        if key.lower().endswith("_proxy") or key in ("MAGPIE_ADDR", "MAGPIE_WEB_KEY", "MAGPIE_UPDATE_FEED", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "GEMINI_CLI_HOME"):
            env.pop(key, None)
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
               XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
               LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
               GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOWORK="off", CGO_ENABLED="1")
    setup = []

    def cli(*values):
        if values[0] not in ("add", "off", "config"):
            raise AssertionError("auth consumer proof permits setup-only CLI commands")
        result = subprocess.run([str(args.host), "plugin", "sdk", *values], env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
        setup.append({"command": values[0], "exit": result.returncode})
        if result.returncode:
            raise AssertionError("fixture module setup failed")

    with Vendor() as vendor, contextlib.ExitStack() as cleanup:
        for kind, module in (("zcode", own), ("opencode", args.opencode)):
            wrapper = output / ("wrapper-" + kind)
            create_wrapper(wrapper, args.sdk, module, kind, vendor.origin)
            with (output / ("wrapper-" + kind + ".log")).open("wb") as log:
                declaration = subprocess.check_output(["go", "run", "-mod=mod", ".", "manifest"],
                                                       cwd=wrapper, env=env, stderr=log, timeout=120)
                manifest = json.loads(declaration)
                distribution = output / ("distribution-" + kind)
                library = distribution / manifest["entry"]
                library.parent.mkdir(parents=True)
                subprocess.run(["go", "build", "-mod=readonly", "-buildmode=c-shared",
                                "-o", str(library), "."], cwd=wrapper, env=env, stdout=log, stderr=log, check=True, timeout=120)
            (distribution / "magpie-plugin.json").write_bytes(declaration)
            (distribution / "page").mkdir()
            (distribution / "page/index.html").write_text(
                '<!doctype html><title>Native auth fixture</title><script src="_magpie/bridge.js"></script>'
            )
            (distribution / "package.json").write_text(json.dumps({
                "name": "native-auth-consumer-" + kind, "private": True,
                "magpie": {"page": {"title": kind, "file": "page/index.html"}, "permissions": []},
            }))
            cli("add", str(distribution))
            if kind == "opencode":
                cli("config", "opencode", '{"projectId":"global"}', "--expected", "0")
        vendor.on_disable = lambda kind: cli("off", kind)

        def account_snapshot(kind, identity):
            for path in home.rglob("accounts.json"):
                records = json.loads(path.read_bytes())
                for account in records["accounts"]:
                    if account["plugin"] == kind and account["id"] == identity:
                        refresh = account.get("refresh")
                        return {
                            "generation": str(account["generation"]), "usage": account.get("usage"),
                            "refreshState": refresh.get("state") if refresh else None,
                            "refreshSHA256": hashlib.sha256(json.dumps(refresh, sort_keys=True).encode()).hexdigest() if refresh else None,
                        }
            raise AssertionError("synthetic selected account record was not found")

        vendor.on_account = account_snapshot
        web_port = port()
        env.update(MAGPIE_ADDR=f"127.0.0.1:{port()}", MAGPIE_WEB_KEY="synthetic-auth-consumer-key",
                   MAGPIE_UPDATE_FEED=vendor.origin + "/updates", HTTP_PROXY=vendor.origin,
                   HTTPS_PROXY=vendor.origin, NO_PROXY="127.0.0.1,localhost")
        origin = f"http://127.0.0.1:{web_port}"
        log = cleanup.enter_context((output / "web.log").open("wb"))
        process = subprocess.Popen([str(args.host), "web", "--addr", f"127.0.0.1:{web_port}", "--no-open"],
                                   env=env, stdout=log, stderr=log)
        cleanup.callback(stop, process)
        jar = http.cookiejar.CookieJar()
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
        deadline = time.monotonic() + 30
        while True:
            try:
                opener.open(origin + "/?k=synthetic-auth-consumer-key", timeout=2).read()
                break
            except OSError:
                if process.poll() is not None or time.monotonic() > deadline:
                    raise AssertionError("isolated frozen host failed to start")
                time.sleep(0.1)
        pages = json.loads(opener.open(origin + "/api/plugin-pages", timeout=10).read())
        pages = {value["name"]: value for value in pages if value["name"] in ("zcode", "opencode")}
        if set(pages) != {"zcode", "opencode"}:
            raise AssertionError("native consumer windows were not admitted")
        config = output / "driver.json"
        config.write_text(json.dumps({
            "origin": origin, "vendor": vendor.origin, "cookie": "; ".join(f"{item.name}={item.value}" for item in jar),
            "gateway": "http://" + env["MAGPIE_ADDR"],
            "pages": pages, "sdkUI": str(args.sdk / "sdk/js/ui.js"), "secrets": vendor.secrets,
            "bridgeSHA256": digest(args.sdk / "internal/gui/pagebridge.js"),
            "parentSHA256": digest(args.sdk / "internal/gui/assets/extensionwindows.js"),
            "sdkUISHA256": digest(args.sdk / "sdk/js/ui.js"),
            "output": str(output / "public-transcript.json"),
        }))
        config.chmod(0o600)
        with (output / "driver.log").open("wb") as log:
            result = subprocess.run(["node", "--experimental-vm-modules", str(Path(__file__).with_name("driver.mjs")), str(config)],
                                    env=env, cwd=own, stdout=log, stderr=log, timeout=120)
        if result.returncode:
            raise AssertionError("auth consumer JS/native chain failed; inspect bounded driver log")
        transcript = json.loads((output / "public-transcript.json").read_bytes())
        if not transcript["passed"]:
            raise AssertionError("consumer did not report its terminal proof")
        accounts = []
        complete = {}
        for path in home.rglob("accounts.json"):
            records = json.loads(path.read_bytes())
            if records.get("schema") != 4:
                raise AssertionError("auth owner did not retain schema-4 receipts")
            accounts.extend(records["accounts"])
            flows = list(records["authFlows"])
            for archived in (path.parent / "auth-receipts").glob("flow-*.json"):
                flows.append(json.loads(archived.read_bytes()))
            for record in flows:
                if record["phase"] == "complete":
                    complete[record["flow"]] = record
        if len(accounts) != 2 or len(complete) != 4:
            raise AssertionError("duplicate, canceled or uncertain exchange changed account publication count")
        for kind, provider in (("zcode", "zcode"), ("opencode", "opencode-go")):
            saved = next(item for item in accounts if item["plugin"] == kind and item["provider"] == provider)
            public = transcript["receipts"][kind]
            generation = 2 if kind == "zcode" else 3
            if saved["id"] != public["id"] or saved["generation"] != generation or public["generation"] != str(generation) or saved["expiresAt"] != public["expiresAt"]:
                raise AssertionError("public receipt/account generation/expiry publication was not atomic")
            receipts = [item["receipt"] for item in complete.values() if item["scope"]["plugin"] == kind]
            if {item["generation"] for item in receipts} != {"1", str(generation)} or any(item["id"] != saved["id"] for item in receipts):
                raise AssertionError("completed exchange receipt changed its captured account")
            if kind == "zcode":
                if saved["auth"]["sessionToken"] != vendor.issued["zcode"][1]:
                    raise AssertionError("ZCode vendor credentials were not published through the host owner")
            elif (saved["auth"]["access"] != vendor.issued["opencode"][1] or
                  saved["auth"]["refresh"] != vendor.issued["opencode"][1].replace("private-opencode-access-", "private-opencode-refresh-", 1)):
                raise AssertionError("OpenCode device credentials were not published through the host owner")
            usage_material = vendor.issued[kind][:2] if kind == "zcode" else ["private-opencode-rotated-access", vendor.issued[kind][1]]
            for issued in usage_material:
                if "Bearer " + issued not in vendor.observed_usage[kind]:
                    raise AssertionError("native selected generation did not reach its real vendor usage controller")
        verify_sdk(args.sdk, args.sdk_files, args.sdk_files_sha256)
        if sources != {"zcode": module_sources(own), "opencode": module_sources(args.opencode)}:
            raise AssertionError("native module source changed during consumer proof")
        if harness != {path.name: digest(path) for path in sorted(Path(__file__).parent.iterdir()) if path.suffix in (".py", ".mjs")}:
            raise AssertionError("consumer harness changed during proof")
        for log_path in (output / "web.log", output / "driver.log"):
            text = log_path.read_text()
            if any(secret in text for secret in vendor.secrets):
                raise AssertionError("private fixture material entered an ordinary web/driver log")
        evidence = {
            "passed": True, "hostSHA256": args.host_sha256, "sdkFilesSHA256": args.sdk_files_sha256,
            "actualPageBridgeParentHTTPNativeStore": True, "schema": 4, "publishedAccounts": 2,
            "completeReceipts": 4, "setupCLIOnly": setup, "iframeCredentialsAbsent": True,
            "duplicateChangedOffCancelGGuarded": True, "renderedBrowser": False,
            "reauthorizationKeptDirtyUsageUntilRealAck": True, "reauthorizationKeptCompletedRefreshReceipt": True,
            "uid": os.getuid(), "euid": os.geteuid(), "capabilities": status,
            "networkNamespace": os.readlink("/proc/self/ns/net"), "interfaces": sorted(interfaces),
            "moduleSourcesSHA256": digest(output / "module-source-hashes.json"),
            "consumerSourcesSHA256": digest(output / "consumer-source-hashes.json"),
        }
        (output / "result.json").write_text(json.dumps(evidence, indent=2))
        print(json.dumps(evidence))


if __name__ == "__main__":
    main()
