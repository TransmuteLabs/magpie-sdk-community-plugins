"""First-account admission through the actual Providers poll broker."""

import argparse
import base64
from contextlib import ExitStack
import hashlib
import http.cookiejar
import importlib.util
import json
from pathlib import Path
import secrets
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request


specification = importlib.util.spec_from_file_location(
    "minimax_gui_policy", Path(__file__).with_name("policy_host.py"))
policy = importlib.util.module_from_spec(specification)
specification.loader.exec_module(policy)
fixture = policy.fixture
require = fixture.require


class GuiEndpoint(policy.PolicyEndpoint):
    def dispatch(self, path, value, region):
        region = fixture.HOSTS[self.authority]
        if path == "/oauth2/device/code":
            device = "device-" + region + "-" + str(len(self.server.devices) + 1)
            self.server.devices[device] = value["code_challenge"]
            self.reply({"device_code": device, "user_code": "GUI-" + str(len(self.server.devices)),
                        "verification_uri": "https://" + self.authority + "/device",
                        "expires_in": 300, "interval": 0.01})
            return
        if path == "/oauth2/token":
            if value["grant_type"] == "refresh_token":
                source, region_from_token = self.server.refreshes[value["refresh_token"]]
                require(region == region_from_token, "refresh crossed regions")
                access = source + "-renewed"
                refresh = "synthetic-gui-refresh-" + access
                self.server.tokens[access] = self.server.tokens[source]
                self.server.refreshes[refresh] = (access, region)
                self.reply({"access_token": access, "refresh_token": refresh, "expires_in": 3600})
                return
            device = value["device_code"]
            require(device.startswith("device-" + region + "-"), "device exchange crossed regions")
            challenge = base64.urlsafe_b64encode(hashlib.sha256(
                value["code_verifier"].encode()).digest()).decode().rstrip("=")
            require(challenge == self.server.devices[device], "GUI device verifier changed")
            if not self.server.authorized:
                self.reply({"error": "authorization_pending"}, 400)
                return
            if self.server.hold:
                self.server.arrived.set()
                require(self.server.release.wait(15), "held GUI exchange did not release")
            identity = device.removeprefix("device-")
            access = "synthetic-gui-access-" + identity
            refresh = "synthetic-gui-refresh-" + identity
            self.server.tokens[access] = (region, self.server.identity or region, self.server.name or region + "@example.invalid")
            self.server.refreshes[refresh] = (access, region)
            self.reply({"access_token": access, "refresh_token": refresh,
                        "token_type": "Bearer", "expires_in": 3600})
            return
        if not self.authority.startswith("account."):
            token = self.headers["Authorization"].removeprefix("Bearer ")
            expected_region, identity, name = self.server.tokens[token]
            require(region == expected_region, "selected account credential crossed regions")
            if path == "/v1/api/user/info":
                self.reply({"statusInfo": {"code": 0}, "data": {"userInfo": {"realUserID": identity, "userEmail": name}}})
                return
        super().dispatch(path, value, region)


fixture.EndpointHandler = GuiEndpoint


def run(host_binary, distribution, output):
    require(not output.exists(), "GUI evidence must use a new directory")
    output.mkdir()
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    metadata = json.loads((distribution / "package.json").read_text())
    require(manifest["runtime"] == "native" and not manifest.get("window") and not manifest.get("agents")
            and not (distribution / "web").exists(), "MiniMax must supply providers without a duplicate window or agent")
    require(manifest["version"] == metadata["version"] and metadata["magpie"]["name"] == "MiniMax",
            "MiniMax distribution metadata does not match the native module")
    report = {"test": "TestInstalledMiniMaxProviderSignIn", "cases": [],
              "hostSHA256": fixture.sha256(host_binary),
              "librarySHA256": fixture.sha256(distribution / manifest["entry"]),
              "runtime": "actual Providers routes and packaged native handlers; no external vendor"}
    try:
        with ExitStack() as stack:
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="mm-gui-")))
            ca, context = fixture.certificate(output, {"HOME": str(home), "PATH": "/usr/bin:/bin"})
            proxy = fixture.Fixture(context)
            proxy.mode, proxy.primary, proxy.devices, proxy.tokens = "gui", None, {}, {}
            proxy.refreshes, proxy.identity, proxy.name = {}, "", ""
            proxy.authorized, proxy.success_format = False, "negotiated"
            proxy.hold, proxy.arrived, proxy.release = False, threading.Event(), threading.Event()
            proxy.start(stack)
            stack.callback(proxy.release.set)
            host = fixture.Host(host_binary, distribution, home, output,
                                "http://127.0.0.1:" + str(proxy.server_port), ca)
            stack.callback(host.close)
            host.env["MAGPIE_WEB_KEY"] = "synthetic-minimax-gui-entry-key"
            host.env["MAGPIE_ADDR"] = "127.0.0.1:0"
            host.log = (output / "host.log").open("w")
            host.process = subprocess.Popen(
                [str(host_binary), "web", "--addr", "127.0.0.1:" + str(host.port), "--no-open"],
                cwd=home, env=host.env, stdout=host.log, stderr=subprocess.STDOUT, start_new_session=True)
            origin = "http://127.0.0.1:" + str(host.port)
            opener = urllib.request.build_opener(
                urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

            def ready():
                require(host.process.poll() is None, "GUI host exited during startup")
                try:
                    return opener.open(origin + "/?k=" + host.env["MAGPIE_WEB_KEY"], timeout=2).status == 200
                except OSError:
                    return False

            fixture.wait_for(ready, "GUI host did not start", 20)

            def request(path, value=None, success=True):
                body = None if value is None else json.dumps(value).encode()
                message = urllib.request.Request(origin + path, data=body,
                    headers={"Content-Type": "application/json", "Origin": origin})
                try:
                    with opener.open(message, timeout=20) as response:
                        data = response.read(fixture.LIMIT)
                    require(success is not False, "revoked GUI request unexpectedly succeeded")
                    return json.loads(data)
                except urllib.error.HTTPError:
                    require(success is not True, "ordinary GUI request failed")
                    return None

            def accounts(provider):
                saved = json.loads(host.command("plugin", "sdk", "accounts", fixture.MODULE, provider, "--json"))
                return [{**item, "generation": str(item["generation"])} for item in saved or []]

            installed = request("/api/plugins/add", {"spec": str(distribution)})
            require(len(installed["plugins"]) == 1 and installed["plugins"][0]["version"] == manifest["version"],
                    "ordinary Plugins Add did not install the MiniMax distribution")
            require(not accounts(fixture.GLOBAL) and not accounts(fixture.CN), "fixture must start with zero accounts")
            regions = (fixture.GLOBAL, fixture.CN)
            ids = {provider: fixture.gateway_provider(provider) for provider in regions}

            def subscriptions():
                listed = request("/api/providers")
                return {provider: row for provider, identifier in ids.items()
                        for row in listed.get("sdkSubscriptions", []) if row["id"] == identifier}

            rows = subscriptions()
            require(set(rows) == set(regions) and all(row["reauthorize"] and row["revision"] and not row.get("page")
                        and len(row["methods"]) == 1 and row["methods"][0]["id"] == "device"
                        and row["methods"][0]["mode"] == "poll" and not row["methods"][0].get("fields") for row in rows.values()),
                    "both empty regions must offer first-class device authorization")
            require(not any(row["name"] == fixture.MODULE for row in request("/api/plugin-pages")),
                    "MiniMax appeared as a duplicate sidebar page")
            report["cases"].append({"case": "zero-account-subscriptions", "passed": True})
            parent = secrets.token_hex(16)

            def auth(operation, provider, flow="", selected_parent=parent, revision=None, selected=None, success=True):
                value = {"id": ids[provider], "revision": revision or rows[provider]["revision"],
                         "parent": selected_parent, "request": secrets.token_hex(16), "operation": operation}
                if operation == "start":
                    value["options"] = {"method": "device"}
                    if selected is not None:
                        value["options"]["account"] = {"id": selected["id"],
                                                       "expectedGeneration": selected["generation"]}
                else:
                    value["flow"] = flow
                result = request("/api/provider/sdk-signin", value, success)
                if result is not None:
                    public = json.dumps(result)
                    require(not any(secret in public for secret in ("synthetic-gui-access-", "synthetic-gui-refresh-",
                            "code_verifier", "device_code", "continuation")), "GUI exposed private authentication data")
                return result

            def exchanges():
                return len([row for row in proxy.snapshot() if row["path"] == "/oauth2/token"])

            signed_in = {}
            for provider in regions:
                proxy.authorized = False
                view = auth("start", provider)
                require(view["state"] == "waiting" and view["userCode"], "device challenge not projected")
                before = exchanges()
                early = auth("finish", provider, view["flow"])
                require(early["state"] == "waiting" and exchanges() == before, "early poll exchanged a device code")
                time.sleep(view["retryAfterMs"] / 1000 + 0.05)
                pending = auth("finish", provider, view["flow"])
                require(pending["state"] in ("pending", "waiting") and not accounts(provider),
                        "pending authorization published an account")
                proxy.authorized = True
                time.sleep(pending.get("retryAfterMs", 1000) / 1000 + 0.05)
                complete = auth("finish", provider, view["flow"])
                require(complete["state"] == "complete" and len(accounts(provider)) == 1,
                        "GUI completion did not publish exactly one account")
                before = exchanges()
                repeated = auth("finish", provider, view["flow"])
                require(repeated["state"] == "complete" and exchanges() == before,
                        "GUI completion exchanged a token again")
                signed_in[provider] = complete["account"]
                require(complete["account"]["generation"] == "1", "first sign-in must start at generation 1")
                report["cases"].append({"case": "first-account-" + provider, "passed": True})

            def account_name(provider, account, name):
                listed = request("/api/providers")
                row, = [row for row in listed["providers"] if row["id"] == ids[provider]]
                visible = row.get("accountUsers", {}).get(account["id"])
                require(visible == name, f"Providers showed {visible!r} for MiniMax account named {name!r}")
                require(any(login.get("extensionGeneration") == account["generation"]
                            for login in row["account"]["logins"]), "Providers did not show the selected generation")

            for provider, first in signed_in.items():
                account_name(provider, first, provider + "@example.invalid")
                proxy.name = "renewed-" + provider + "@example.invalid"
                view = auth("start", provider, selected=first)
                time.sleep(view["retryAfterMs"] / 1000 + 0.05)
                changed = auth("finish", provider, view["flow"])["account"]
                require(changed["id"] == first["id"] and changed["generation"] == "2" and len(accounts(provider)) == 1,
                        "reauthorization must update the selected MiniMax account")
                account_name(provider, changed, proxy.name)
                report["cases"].append({"case": "named-reauthorization-" + provider, "passed": True})
                refreshed = json.loads(host.command("plugin", "sdk", "auth", "refresh", fixture.MODULE, provider, first["id"]))
                require(refreshed["name"] == proxy.name and refreshed["id"] == first["id"],
                        "rotating refresh lost the selected account identity or published name")
                changed["generation"] = accounts(provider)[0]["generation"]
                require(changed["generation"] == "3", "rotating refresh did not advance exactly one generation")
                account_name(provider, changed, proxy.name)
                signed_in[provider] = changed
                report["cases"].append({"case": "named-refresh-" + provider, "passed": True})

            proxy.identity = "wrong-subject"
            view = auth("start", fixture.GLOBAL, selected=signed_in[fixture.GLOBAL])
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            rejected = auth("finish", fixture.GLOBAL, view["flow"], success=None)
            require(rejected is None or rejected["state"] != "complete", "foreign identity replaced the selected account")
            require(accounts(fixture.GLOBAL)[0]["generation"] == signed_in[fixture.GLOBAL]["generation"],
                    "foreign identity changed the selected account generation")
            report["cases"].append({"case": "conflicting-reauthorization", "passed": True})
            proxy.identity = ""

            proxy.authorized = False
            view = auth("start", fixture.GLOBAL)
            before = exchanges()
            auth("finish", fixture.GLOBAL, view["flow"], selected_parent="b" * 64, success=False)
            auth("finish", fixture.CN, view["flow"], success=False)
            canceled = auth("cancel", fixture.GLOBAL, view["flow"])
            require(canceled["state"] == "canceled" and exchanges() == before, "cancel or foreign flow dispatched")
            ended = auth("finish", fixture.GLOBAL, view["flow"])
            require(ended["state"] == "canceled" and exchanges() == before, "canceled flow resumed")
            report["cases"].append({"case": "cancel-foreign-parent-region", "passed": True})
            view = auth("start", fixture.GLOBAL)
            old_revision = rows[fixture.GLOBAL]["revision"]
            snapshot = json.loads(host.command("plugin", "sdk", "config", fixture.MODULE))
            host.command("plugin", "sdk", "config", fixture.MODULE, "{}",
                         "--expected", snapshot["version"])
            before = exchanges()
            auth("finish", fixture.GLOBAL, view["flow"], revision=old_revision, success=False)
            rows = subscriptions()
            require(rows[fixture.GLOBAL]["revision"] != old_revision and exchanges() == before,
                    "stale configuration reused a subscription grant or dispatched")
            report["cases"].append({"case": "stale-config-revision", "passed": True})
            host.command("accounts", "discovery", "off")
            before = exchanges()
            auth("start", fixture.GLOBAL, success=False)
            require(exchanges() == before, "discovery-off GUI start exchanged credentials")
            host.command("accounts", "discovery", "on")
            report["cases"].append({"case": "discovery-off-start", "passed": True})
            view = auth("start", fixture.GLOBAL)
            proxy.authorized, proxy.hold = True, True
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            completed, failures = [], []

            def finish_held():
                try:
                    completed.append(auth("finish", fixture.GLOBAL, view["flow"], success=None))
                except Exception as error:
                    failures.append(str(error))

            call = threading.Thread(target=finish_held, name="minimax-gui-held-finish")
            call.start()
            try:
                require(proxy.arrived.wait(10), "GUI completion never reached its held token exchange")
                host.command("accounts", "discovery", "off")
            finally:
                proxy.release.set()
                call.join(15)
            require(not call.is_alive() and not failures, "GUI held completion failed or did not join")
            require(completed and (completed[0] is None or completed[0]["state"] != "complete"),
                    "revoked GUI exchange published successful completion")
            host.command("accounts", "discovery", "on")
            require(len(accounts(fixture.GLOBAL)) == 1, "revoked GUI exchange published a new account")
            report["cases"].append({"case": "discovery-off-held-exchange", "passed": True})

            def logout(provider, account, success=True):
                return request("/api/login/forget", {"agent": ids[provider], "account": {
                    "id": account["id"], "expectedGeneration": account["generation"],
                }}, success)

            expired = {**signed_in[fixture.GLOBAL], "generation": "1"}
            logout(fixture.GLOBAL, expired, success=False)
            require(len(accounts(fixture.GLOBAL)) == 1, "stale logout removed the current account")
            for provider, account in signed_in.items():
                logout(provider, account)
                require(not accounts(provider) and provider in subscriptions(),
                        "ordinary sign-out did not restore the empty subscription")
            report["cases"].append({"case": "generation-bound-sign-out", "passed": True})
            host.command("plugin", "sdk", "off", fixture.MODULE)
            auth("start", fixture.GLOBAL, success=False)
            require(not subscriptions(), "disabled module's subscriptions remained published")
            report["cases"].append({"case": "disabled-stale-subscription", "passed": True})
            require(not proxy.failures, "synthetic GUI endpoint assertions failed")
            require(not (host.config / "plugins.json").exists() and not (host.config / "plugins/node_modules").exists(),
                    "MiniMax depended on an additional plugin or runtime install")
        report["status"] = "passed"
    except Exception as error:
        report["status"], report["failure"] = "failed", str(error)
        raise
    finally:
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for option in ("host", "distribution", "output"):
        parser.add_argument("--" + option, type=Path, required=True)
    options = parser.parse_args()
    run(*(getattr(options, name).resolve() for name in ("host", "distribution", "output")))
