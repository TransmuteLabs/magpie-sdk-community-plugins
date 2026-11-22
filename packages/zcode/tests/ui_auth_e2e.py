import contextlib
import hashlib
import http.cookiejar
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parents[3] / "scripts"))
from _workspace import build_environment


def main():
    source = Path(__file__).resolve().parents[1] / "src"
    host, sdk = Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve()
    bun = shutil.which("bun")
    assert bun, "the isolated Community fixture requires Bun"
    spec = importlib.util.spec_from_file_location("zcode_synthetic_vendor", source / "testdata/window-auth/vendor.py")
    vendor_module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vendor_module)
    marker = secrets.token_hex(32)
    digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
    with build_environment(source.parent, sdk) as (env, _), \
            tempfile.TemporaryDirectory(prefix="zcode-auth-ui-") as scratch, \
            vendor_module.Vendor(request_marker=marker) as vendor, contextlib.ExitStack() as cleanup:
        root = Path(scratch)
        home = root / "home"
        home.mkdir()
        env.update(json.loads(subprocess.check_output(["go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"], env=env)))
        for key in list(env):
            if key.lower().endswith("_proxy") or key in ("MAGPIE_UPDATE_FEED", "MAGPIE_ADDR", "MAGPIE_WEB_KEY"):
                env.pop(key, None)
        env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                   XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                   XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                   LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
                   MAGPIE_NO_ACCOUNTS="", TMPDIR=str(root), MAGPIE_BUN=bun)
        distribution = root / "distribution"
        with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
            archive.extractall(distribution)
        manifest = json.loads((distribution / "magpie-plugin.json").read_bytes())
        assert "provider-saved-signin" in manifest["contract"]["capabilities"]
        provenance = json.loads((distribution / "source-lock.json").read_bytes())
        assert provenance["plugin"] == {"id": manifest["id"], "version": manifest["version"],
                                        "sdkMinimum": "1.20", "nativeABI": 1}
        assert all((distribution / name).is_file() for name in
                   ("SOURCES.md", "VENDOR-DIFF-3.18.2.md", "COMMUNITY-PARITY.md",
                    "ONE-BUNDLE-AUDIT.md", "SQLITE-DEPENDENCIES.md"))
        binding, = manifest["authBindings"]
        assert (binding["from"], binding["provider"], binding["reauthorize"]) == ("provider:signin", "zcode", True)
        methods = {method["id"]: method for method in binding["methods"]}
        browser, importer = methods["browser"], methods["saved-signin"]
        assert (browser["id"], browser["mode"]) == ("browser", "poll")
        assert (importer["id"], importer["mode"]) == ("saved-signin", "poll")
        record_field, = importer["fields"]
        assert record_field["key"] == "record" and record_field["type"] == "secret" and not record_field.get("optional")
        fields = ",".join(key + ":" + json.dumps(vendor.origin + path) for key, path in {
            "Zai": "/zai", "BigModel": "/bigmodel", "Start": "/start",
            "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
        }.items())
        entry = root / "native.go"
        entry.write_text('package main\nimport(z "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
                         '"github.com/yetone/magpie/pkg/extension/native")\nfunc init(){'
                         'if e:=native.Register(z.NewControlModule(z.ModuleOptions{Endpoints:z.Endpoints{'
                         + fields + '},HTTPFactory:z.FixtureHTTPFactory(' + json.dumps(vendor.origin)
                         + ',' + json.dumps(marker) + ')}));e!=nil{panic(e)}}\nfunc main(){}\n')
        overlay = root / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {
            str(source / "cmd/native-ui/main.go"): str(entry),
            str(source / "fixture_http.go"): str(source / "fixture_http_test.go"),
        }}))
        subprocess.run(["go", "build", "-overlay", str(overlay), "-buildmode=c-shared", "-o",
                        str(distribution / manifest["entry"]), "./cmd/native-ui"], cwd=source, env=env, check=True)
        subprocess.run([str(host), "plugin", "sdk", "add", str(distribution)], env=env, check=True,
                       stdout=subprocess.DEVNULL, timeout=60)

        def free_port():
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                return listener.getsockname()[1]

        port = free_port()
        gateway_address = f"127.0.0.1:{free_port()}"
        env.update(MAGPIE_ADDR=gateway_address, MAGPIE_WEB_KEY="synthetic-auth-ui",
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
        try:
            opener.open(vendor.origin + "/", timeout=5).close()
            raise AssertionError("unmarked request reached the synthetic vendor")
        except urllib.error.HTTPError as error:
            with error:
                assert error.code == 404 and vendor.rejected_requests and not vendor.errors
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
        def request_json(url, payload=None, expected_status=200, headers=None):
            body = None if payload is None else json.dumps(payload).encode()
            request = urllib.request.Request(url, body, {"Content-Type": "application/json", **(headers or {})})
            try:
                response = opener.open(request, timeout=20)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                raw = response.read()
            assert all(secret.encode() not in raw for secret in vendor.secrets)
            assert response.status == expected_status, (request.selector, response.status, raw[:800])
            return json.loads(raw)

        def api(path, payload=None, expected_status=200):
            return request_json(origin + "/api/" + path, payload, expected_status, {"Origin": origin})

        def inference(provider_id, expected_status):
            return request_json("http://" + gateway_address + "/v1/messages", {
                "model": provider_id + "/GLM-5.3",
                "messages": [{"role": "user", "content": "saved sign-in fixture"}],
                "max_tokens": 16,
            }, expected_status, {"Anthropic-Version": "2023-06-01"})

        def subscription():
            return next(row for row in api("providers")["sdkSubscriptions"] if row["name"] == "ZCode native")

        start_name = "fixture@example.invalid · ZCode zai start"
        key_name = "Z.ai · GLM Coding Plan API key"

        def check_account(provider_id, identity, generation, enabled, name, active=None):
            listed, = [row for row in api("providers")["providers"] if row["id"] == provider_id]
            login, = [entry for entry in listed["account"]["logins"] if entry["user"] == identity]
            assert login["extensionGeneration"] == str(generation)
            assert login["on"] == enabled and (active is None or login["active"] == active), login
            assert listed["accountUsers"][identity] == name

        def check_surfaces(provider_id):
            installed, = [row for row in api("plugins")["plugins"] if row["spec"] == "sdk:zcode"]
            assert (installed["ownName"], installed["version"], installed["icon"]) == ("ZCode", manifest["version"], "zcode")
            assert installed["providers"] == ["ZCode native"] and installed["sdk"]["providerIDs"] == [provider_id]
            assert installed["isAgent"] and installed["sdk"]["agents"]
            assert not any(page["name"] == "zcode" for page in api("plugin-pages"))
            assert [(panel["name"], panel["slot"]) for panel in api("plugin-panels") if panel["name"] == "zcode"] == [
                ("zcode", "settings")]

        assert {method["id"] for method in subscription()["methods"]} == {"browser", "api-key"}
        check_surfaces(subscription()["id"])

        def authentication(options, owner_revision=None, expected_status=200):
            row = subscription()
            common = {"id": row["id"], "revision": row["revision"], "ownerRevision": row.get("ownerRevision", ""),
                      "parent": secrets.token_hex(16)}
            if owner_revision is not None:
                common["ownerRevision"] = owner_revision
            result = api("provider/sdk-signin", {**common, "request": "start", "operation": "start", "options": options},
                         expected_status)
            for _ in range(5):
                if result.get("state") not in ("waiting", "pending"):
                    break
                time.sleep(max(1, result.get("retryAfterMs", 0) / 1000))
                result = api("provider/sdk-signin", {**common, "operation": "finish", "flow": result["flow"]})
            return result

        selected = None
        for generation in (1, 2):
            grant = {"method": "browser", "input": {"site": "zai", "kind": "start"} if not selected else {}}
            if selected:
                grant["account"] = {"id": selected, "expectedGeneration": str(generation - 1)}
            result = authentication(grant)
            assert result["state"] == "complete"
            if selected:
                assert result["account"]["id"] == selected
            selected = result["account"]["id"]
            assert result["account"]["generation"] == str(generation)
            check_account(subscription()["id"], selected, generation, True, start_name, active=True)
        keyed = authentication({"method": "api-key", "input": {"key": vendor.zcode_key}})
        assert keyed["state"] == "complete", "Providers did not support sign-in with the coding-plan API key"
        api_account = keyed["account"]
        assert api_account["generation"] == "1"
        check_account(subscription()["id"], api_account["id"], 1, True, key_name)
        keyed = authentication({"method": "api-key", "input": {"key": vendor.zcode_key},
                                "account": {"id": api_account["id"], "expectedGeneration": "1"}})
        assert keyed["state"] == "complete" and keyed["account"] == {**api_account, "generation": "2"}
        check_account(subscription()["id"], api_account["id"], 2, True, key_name)
        assert vendor.zcode_quota_reads >= 2
        row = subscription()
        for identity in (selected, api_account["id"]):
            api("login/forget", {"agent": row["id"], "account": {"id": identity, "expectedGeneration": "2"}})
        token = vendor.issued["zcode"][-1]
        details = {"site": "zai", "base": "https://api.z.ai/api/anthropic", "plan": "Start", "device": "fixture-device", "jwt": token}
        saved = {"type": "oauth", "accountId": "fixture@example.invalid", "uid": "zcode-user", "access": token,
                 "expires": vendor.expiry * 1000}
        accounts_file, = list(home.rglob("accounts.json"))
        before = accounts_file.read_bytes()
        linked = {**saved, "expires": 0, "refresh": json.dumps({**details, "source": "zcode", "key": vendor.zcode_key})}
        start = {**saved, "refresh": json.dumps(details)}
        denied = api("provider/sdk-signin", {
            "id": row["id"], "revision": row["revision"], "parent": secrets.token_hex(16),
            "request": "import", "operation": "start",
            "options": {"method": "saved-signin", "input": {"record": json.dumps(start)}},
        }, expected_status=400)
        assert "error" in denied and "flow" not in denied
        legacy_auth = Path(env["XDG_CONFIG_HOME"]) / "magpie/plugin-auth.json"
        start_slot, key_slot = "zcode", "zcode#key"
        key_user, start_user = "fixture-api-account", start["accountId"]
        key = {"type": "api", "key": vendor.zcode_key, "accountId": key_user, "metadata": {"site": "zai"}}
        records = {start_slot: start, key_slot: key}

        def command(*arguments, refusal=None):
            result = subprocess.run([str(host), *arguments],
                                    env=env, capture_output=True, text=True, timeout=35)
            assert all(secret not in result.stdout + result.stderr for secret in vendor.secrets)
            if refusal:
                assert result.returncode != 0 and refusal in result.stderr, result.stderr
                return
            assert result.returncode == 0, result.stderr
            return result.stdout

        def saved_auth(*arguments, refusal=None):
            stored_source = legacy_auth.read_bytes()
            output = command("plugin", "sdk", "auth", *arguments, refusal=refusal)
            assert legacy_auth.read_bytes() == stored_source
            return json.loads(output) if output else None

        legacy_spec = str(source / "testdata/window-auth/community.mjs")
        legacy_auth.write_text(json.dumps({**records, start_slot: linked}))
        api("plugins/add", {"spec": legacy_spec})
        saved_auth("import", "zcode", "zcode", start_slot, refusal="requires interactive authorization")
        assert accounts_file.read_bytes() == before and not vendor.zcode_profiles
        legacy_auth.write_text(json.dumps(records))
        source_bytes = legacy_auth.read_bytes()
        command("provider", "refresh", "zcode")
        command("provider", "models", "zcode", "+GLM-5.3")
        command("provider", "pool", "zcode", "--accounts", start_user)

        def public_subscription(revision=""):
            timeout = time.monotonic() + 10
            while time.monotonic() < timeout:
                matching = [row for row in api("providers")["providers"] if row["id"] == "zcode"]
                if len(matching) == 1:
                    account = matching[0].get("account", {})
                    if account.get("ownerRevision", "") == revision:
                        return matching[0]
                time.sleep(0.1)
            raise AssertionError("Providers did not publish the expected public ZCode subscription")

        def policies(listed):
            return [(entry["user"], entry["on"]) for entry in listed["account"]["logins"]]

        before_policy = policies(public_subscription())
        assert before_policy == [(start_user, True), (key_user, False)]
        assert inference("zcode", 200)["content"] == [{"type": "text", "text": "community fixture"}]

        def tickets():
            imported = [saved_auth("import", "zcode", "zcode", slot) for slot in records]
            for item, identity, name in zip(imported, (selected, api_account["id"]), (start_name, key_name), strict=True):
                assert item["account"]["id"] == identity and item["account"]["generation"] == "4"
                assert item["account"]["name"] == name
            return [item["ticket"] for item in imported]

        prepared = tickets()
        assert accounts_file.read_bytes() == before
        assert vendor.zcode_profiles == [token]
        assert selected not in (api("login/usage?agent=" + row["id"]) or {})
        assert inference(row["id"], 404)["type"] == "error"
        assert vendor.inferences["zcode"] == 0
        saved_auth("activate", "zcode", "zcode", prepared[0], refusal="requires a subscription owner transfer")
        transfer = ("transfer", "zcode", "zcode")
        incomplete = "selection changed or is incomplete"
        saved_auth(*transfer, prepared[0], refusal=incomplete)
        legacy_auth.write_text(json.dumps({**records, start_slot: {**start, "accountId": "changed"}}))
        saved_auth(*transfer, *prepared, refusal=incomplete)
        assert accounts_file.read_bytes() == before
        assert not saved_auth("owners")
        legacy_auth.write_bytes(source_bytes)
        owner = saved_auth(*transfer, *prepared)
        assert owner["active"] and owner["publicID"] == "zcode" and owner["revision"]
        assert {item["key"]: (item["user"], item["id"]) for item in owner["accounts"]} == {
            start_slot: (start_user, selected), key_slot: (key_user, api_account["id"]),
        }
        revision = owner["revision"]
        assert policies(public_subscription(revision)) == before_policy
        check_account("zcode", start_user, 4, True, start_name, active=True)
        check_account("zcode", key_user, 4, False, key_name, active=False)
        check_surfaces("zcode")
        current = api("login/usage?agent=zcode")[start_user]
        assert current["provider"] == "zcode" and current["user"] == start_user and not current.get("error"), current
        assert any(window["used"] == 30 for window in current["windows"]), current
        routed = inference("zcode", 200)
        assert any(part.get("text") == "fixture" for part in routed["content"]) and vendor.inferences["zcode"] == 1

        committed = accounts_file.read_bytes()
        rollback = saved_auth("rollback", "zcode", revision)
        assert not rollback["active"] and rollback["revision"] != revision and rollback["accounts"] == owner["accounts"]
        assert accounts_file.read_bytes() == committed and policies(public_subscription()) == before_policy
        assert inference("zcode", 200)["content"] == [{"type": "text", "text": "community fixture"}]
        saved_auth(*transfer, *prepared, refusal=incomplete)

        owner = saved_auth(*transfer, *tickets())
        assert owner["active"] and owner["revision"] != revision
        assert policies(public_subscription(owner["revision"])) == before_policy
        api("login/on", {"agent": "zcode", "user": key_user})
        api("login/off", {"agent": "zcode", "user": start_user})
        paid = api("login/usage?agent=zcode")[key_user]
        assert any(window["used"] == 23 for window in paid["windows"]), paid
        assert inference("zcode", 200)["content"] == [{"type": "text", "text": "API key fixture"}]
        assert vendor.zcode_paid_inferences == 1
        api("login/on", {"agent": "zcode", "user": start_user})
        api("login/off", {"agent": "zcode", "user": key_user})
        again = {"method": "api-key", "input": {"key": vendor.zcode_key},
                 "account": {"id": key_user, "expectedGeneration": "4"}}
        assert "error" in authentication(again, revision, expected_status=400)
        assert accounts_file.read_bytes() == committed
        keyed = authentication(again)
        assert keyed["state"] == "complete" and keyed["account"]["id"] == key_user
        check_account("zcode", key_user, 5, False, key_name)
        api("login/forget", {"agent": "zcode", "ownerRevision": owner["revision"],
                             "account": {"id": key_user, "expectedGeneration": "4"}}, expected_status=400)
        check_account("zcode", key_user, 5, False, key_name)
        api("login/forget", {"agent": "zcode", "ownerRevision": owner["revision"],
                             "account": {"id": key_user, "expectedGeneration": "5"}})
        assert policies(public_subscription(owner["revision"])) == [(start_user, True)]
        api("plugins/remove", {"spec": legacy_spec})
        check_account("zcode", start_user, 4, True, start_name)
        assert any(part.get("text") == "fixture" for part in inference("zcode", 200)["content"])
        assert vendor.inferences["zcode"] == 2
        assert vendor.started["zcode"] == 2 and len(vendor.issued["zcode"]) == 2
        assert not vendor.errors, vendor.errors
        installed = []
        for catalog in home.rglob("catalog.json"):
            installed.extend(json.loads(catalog.read_bytes()))
        assert len(installed) == 1 and installed[0]["manifest"]["id"] == "zcode"
        assert installed[0]["manifest"]["runtime"] == "native" and not installed[0]["manifest"].get("services")
        for legacy in home.rglob("plugins.json"):
            assert not json.loads(legacy.read_bytes()).get("plugins")
        assert list(home.rglob("plugin-auth.json")) == [legacy_auth] and legacy_auth.read_bytes() == source_bytes
        assert not list(home.rglob("node_modules"))
        stop()
        assert all(secret not in (root / "web.log").read_text() for secret in vendor.secrets)
        print(json.dumps({"passed": True, "hostSHA256": digest(host), "vendorStarts": 2,
                          "storedKinds": ["start", "legacy-key"], "importedGeneration": 4,
                          "desktopFollowRefused": True, "partialAndChangedSourceTransfersRefused": True,
                          "publicID": "zcode", "preservedAliasOrderAndPause": True, "selectedUsage": True,
                          "legacySDKAndRestoredGatewayRoutes": True, "legacyAuthPreserved": True,
                          "legacyPluginRemovedAfterVerification": True, "selectedAPIKeyReauthorization": True,
                          "staleSelectionsRefused": True, "selectedLogout": True, "settingsPanelOnly": True,
                          "outsideVendorRequests": vendor.rejected_requests,
                          "renderedBrowser": False}))


if __name__ == "__main__":
    main()
