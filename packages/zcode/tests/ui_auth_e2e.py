import contextlib
import hashlib
import http.cookiejar
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import zipfile


def main():
    source = Path(__file__).resolve().parents[1] / "src"
    host, sdk = Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve()
    spec = importlib.util.spec_from_file_location("zcode_synthetic_vendor", source / "testdata/window-auth/vendor.py")
    vendor_module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vendor_module)
    digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
    with tempfile.TemporaryDirectory(prefix="zcode-auth-ui-") as scratch, vendor_module.Vendor() as vendor, contextlib.ExitStack() as cleanup:
        root = Path(scratch)
        home = root / "home"
        home.mkdir()
        env = os.environ.copy()
        env.update(json.loads(subprocess.check_output(["go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"])))
        for key in list(env):
            if key.lower().endswith("_proxy") or key in ("MAGPIE_UPDATE_FEED", "MAGPIE_ADDR", "MAGPIE_WEB_KEY"):
                env.pop(key, None)
        env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                   XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                   XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                   LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
                   GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", CGO_ENABLED="1")
        distribution = root / "distribution"
        with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
            archive.extractall(distribution)
        manifest = json.loads((distribution / "magpie-plugin.json").read_bytes())
        provenance = json.loads((distribution / "source-lock.json").read_bytes())
        assert provenance["plugin"] == {"id": manifest["id"], "version": manifest["version"],
                                        "sdkMinimum": "1.12", "nativeABI": 1}
        assert all((distribution / name).is_file() for name in
                   ("SOURCES.md", "VENDOR-DIFF-3.18.2.md", "COMMUNITY-PARITY.md",
                    "ONE-BUNDLE-AUDIT.md", "SQLITE-DEPENDENCIES.md"))
        assert manifest["authBindings"] == [{"id": "page.signin", "from": "window:page", "provider": "zcode",
                                             "methods": [{"id": "browser", "mode": "poll"}], "reauthorize": True}]
        fields = ",".join(key + ":" + json.dumps(vendor.origin + path) for key, path in {
            "Zai": "/zai", "BigModel": "/bigmodel", "Start": "/start",
            "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
        }.items())
        entry = root / "native.go"
        entry.write_text('package main\nimport(z "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
                         '"github.com/yetone/magpie/pkg/extension/native")\nfunc init(){'
                         'if e:=native.Register(z.NewControlModule(z.ModuleOptions{Endpoints:z.Endpoints{'
                         + fields + '}}));e!=nil{panic(e)}}\nfunc main(){}\n')
        overlay = root / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(source / "cmd/native-ui/main.go"): str(entry)}}))
        subprocess.run(["go", "build", "-overlay", str(overlay), "-buildmode=c-shared", "-o",
                        str(distribution / manifest["entry"]), "./cmd/native-ui"], cwd=source, env=env, check=True)
        subprocess.run([str(host), "plugin", "sdk", "add", str(distribution)], env=env, check=True,
                       stdout=subprocess.DEVNULL, timeout=60)

        def free_port():
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                return listener.getsockname()[1]

        port = free_port()
        env.update(MAGPIE_ADDR=f"127.0.0.1:{free_port()}", MAGPIE_WEB_KEY="synthetic-auth-ui",
                   MAGPIE_UPDATE_FEED=vendor.origin + "/updates", HTTP_PROXY=vendor.origin,
                   HTTPS_PROXY=vendor.origin, NO_PROXY="127.0.0.1,localhost")
        log = cleanup.enter_context((root / "web.log").open("wb"))
        process = subprocess.Popen([str(host), "web", "--addr", f"127.0.0.1:{port}", "--no-open"],
                                   env=env, stdout=log, stderr=log)

        def stop():
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)

        cleanup.callback(stop)
        jar = http.cookiejar.CookieJar()
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
        origin = f"http://127.0.0.1:{port}"
        deadline = time.monotonic() + 30
        while True:
            try:
                opener.open(origin + "/?k=synthetic-auth-ui", timeout=2).read()
                break
            except OSError:
                if process.poll() is not None or time.monotonic() > deadline:
                    raise AssertionError("isolated native UI host failed to start")
                time.sleep(0.1)
        pages = json.loads(opener.open(origin + "/api/plugin-pages", timeout=10).read())
        config = root / "driver.json"
        config.write_text(json.dumps({
            "origin": origin, "page": next(page for page in pages if page["name"] == "zcode"),
            "vendor": vendor.origin,
            "cookie": "; ".join(f"{item.name}={item.value}" for item in jar),
            "secrets": vendor.secrets, "bridgeSHA256": digest(sdk / "internal/gui/pagebridge.js"),
            "parentSHA256": digest(sdk / "internal/gui/assets/extensionwindows.js"),
            "bundleSHA256": digest(distribution / "web/app.bundle.js"),
        }))
        driver = subprocess.run(["node", "--experimental-vm-modules", str(source.parent / "web/auth-http.mjs"), str(config)],
                                cwd=source, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=90)
        if driver.returncode:
            raise AssertionError("production UI auth consumer failed: " + driver.stderr.decode()[-3500:]
                                 + json.dumps({"starts": vendor.started, "polls": vendor.polled, "errors": vendor.errors}))
        for secret in vendor.secrets:
            assert secret not in driver.stdout.decode()
        accounts = []
        for account_file in home.rglob("accounts.json"):
            accounts.extend(json.loads(account_file.read_bytes())["accounts"])
        assert len(accounts) == 1 and accounts[0]["generation"] == 2
        assert accounts[0]["auth"]["sessionToken"] == vendor.issued["zcode"][1]
        assert vendor.started["zcode"] == 3 and len(vendor.issued["zcode"]) == 3
        assert not vendor.errors, vendor.errors
        installed = []
        for catalog in home.rglob("catalog.json"):
            installed.extend(json.loads(catalog.read_bytes()))
        assert len(installed) == 1 and installed[0]["manifest"]["id"] == "zcode"
        assert installed[0]["manifest"]["runtime"] == "native" and not installed[0]["manifest"].get("services")
        for legacy in home.rglob("plugins.json"):
            assert not json.loads(legacy.read_bytes()).get("plugins")
        for legacy_auth in home.rglob("plugin-auth.json"):
            assert not json.loads(legacy_auth.read_bytes())
        assert not list(home.rglob("node_modules"))
        descendants = []
        pending = [process.pid]
        while pending:
            parent = pending.pop()
            for child_file in Path(f"/proc/{parent}/task").glob("*/children"):
                try:
                    children = child_file.read_text().split()
                except FileNotFoundError:
                    continue
                for child in children:
                    identity = int(child)
                    if identity in descendants:
                        continue
                    descendants.append(identity)
                    pending.append(identity)
        workers = []
        for child in descendants:
            try:
                workers.append(Path(f"/proc/{child}/comm").read_text().strip())
            except FileNotFoundError:
                continue
        assert not any(name in ("bun", "node", "nodejs") for name in workers)
        stop()
        assert all(secret not in (root / "web.log").read_text() for secret in vendor.secrets)
        print(json.dumps({"passed": True, "hostSHA256": digest(host), "setupCLI": ["add"],
                          "productionManifestAndPackagedApp": True, "accountGeneration": 2,
                          "vendorStarts": 3, "vendorIssuedTokens": 3, "hostPublishedGenerations": 2,
                          "oneNativeBundleOnly": True, "legacyPluginSpecs": 0, "legacyAuthStores": 0,
                          "runtimeBunNodeWorkers": 0,
                          "driver": json.loads(driver.stdout), "renderedBrowser": False}))


if __name__ == "__main__":
    main()
