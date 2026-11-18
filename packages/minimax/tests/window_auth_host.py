"""First-account admission through the actual GUI page and poll broker."""

import argparse
import base64
from contextlib import ExitStack
import hashlib
import http.cookiejar
import importlib.util
import json
from pathlib import Path
import shutil
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
        if path == "/oauth2/device/code":
            device = "device-" + region + "-" + str(len(self.server.devices) + 1)
            self.server.devices[device] = value["code_challenge"]
            self.reply({"device_code": device, "user_code": "GUI-" + str(len(self.server.devices)),
                        "verification_uri": "https://account.minimax.io/device",
                        "expires_in": 300, "interval": 0.01})
            return
        if path == "/oauth2/token":
            device = value["device_code"]
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
            self.server.tokens[access] = identity
            self.reply({"access_token": access, "refresh_token": "synthetic-gui-refresh-" + identity,
                        "token_type": "Bearer", "expires_in": 3600})
            return
        super().dispatch(path, value, region)


fixture.EndpointHandler = GuiEndpoint


def run(host_binary, worker, package, sdk, output):
    require(not output.exists(), "GUI evidence must use a new directory")
    output.mkdir()
    distribution = output / "exec-test-package"
    (distribution / "bin").mkdir(parents=True)
    shutil.copy2(worker, distribution / "bin/minimax")
    declaration = subprocess.check_output([str(worker), "--manifest"], timeout=10)
    (distribution / "magpie-plugin.json").write_bytes(declaration)
    fixture.copy_window_assets(distribution, package, sdk)
    report = {"test": "TestInstalledMiniMaxGUIFirstAccount", "cases": [],
              "hostSHA256": fixture.sha256(host_binary), "workerSHA256": fixture.sha256(worker),
              "runtime": "actual GUI HTTP routes and actual handlers in exec harness; no rendered browser"}
    try:
        with ExitStack() as stack:
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="mm-gui-")))
            ca, context = fixture.certificate(output, {"HOME": str(home), "PATH": "/usr/bin:/bin"})
            shutil.copy2(ca, distribution / "bin/fixture-ca.pem")
            proxy = fixture.Fixture(context)
            proxy.mode, proxy.primary, proxy.devices, proxy.tokens = "gui", None, {}, {}
            proxy.authorized, proxy.success_format = False, "negotiated"
            proxy.hold, proxy.arrived, proxy.release = False, threading.Event(), threading.Event()
            proxy.start(stack)
            stack.callback(proxy.release.set)
            host = policy.PolicyHost(host_binary, distribution, home, output,
                                     "http://127.0.0.1:" + str(proxy.server_port), ca)
            stack.callback(host.close)
            host.install()
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
                return json.loads(host.command("plugin", "sdk", "accounts", fixture.MODULE, provider, "--json"))

            require(not accounts(fixture.GLOBAL) and not accounts(fixture.CN), "fixture must start with zero accounts")
            pages = request("/api/plugin-pages")
            matches = [page for page in pages if page["name"] == fixture.MODULE]
            require(len(matches) == 1, "FIRST-ACCOUNT: installed MiniMax has no ordinary GUI sign-in page")
            page = matches[0]
            for asset in ("index.html", "app.js", "style.css", "sdk-ui.js"):
                address = page["url"].rsplit("/", 1)[0] + "/" + asset
                with opener.open(origin + address, timeout=10) as response:
                    require(response.status == 200 and response.read(), "packaged GUI asset missing: " + asset)
            report["cases"].append({"case": "zero-account-page", "passed": True})
            parent = "a" * 64
            sequence = 0

            def auth(operation, provider, flow="", selected_parent=parent, ref=None, success=True):
                nonlocal sequence
                sequence += 1
                value = {"ref": ref or page["ref"], "binding": "page.auth." + provider,
                         "parent": selected_parent, "request": format(sequence, "064x"), "operation": operation}
                if operation == "start":
                    value["options"] = {"method": "device"}
                else:
                    value["flow"] = flow
                result = request("/api/plugin-pages/auth", value, success)
                if result is not None:
                    public = json.dumps(result)
                    require(not any(secret in public for secret in ("synthetic-gui-access-", "synthetic-gui-refresh-",
                            "code_verifier", "device_code", "continuation")), "GUI exposed private authentication data")
                return result

            def exchanges():
                return len([row for row in proxy.snapshot() if row["path"] == "/oauth2/token"])

            for provider in (fixture.GLOBAL, fixture.CN):
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
                report["cases"].append({"case": "first-account-" + provider, "passed": True})
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
            old_ref = page["ref"]
            snapshot = json.loads(host.command("plugin", "sdk", "config", fixture.MODULE))
            host.command("plugin", "sdk", "config", fixture.MODULE, "{}",
                         "--expected", snapshot["version"])
            before = exchanges()
            auth("finish", fixture.GLOBAL, view["flow"], ref=old_ref, success=False)
            page = next(item for item in request("/api/plugin-pages") if item["name"] == fixture.MODULE)
            require(page["ref"] != old_ref and exchanges() == before,
                    "stale configuration reused a GUI page or dispatched")
            report["cases"].append({"case": "stale-config-page", "passed": True})
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
            host.command("plugin", "sdk", "off", fixture.MODULE)
            auth("start", fixture.GLOBAL, success=False)
            require(not [item for item in request("/api/plugin-pages") if item["name"] == fixture.MODULE],
                    "disabled GUI page remained published")
            report["cases"].append({"case": "disabled-stale-page", "passed": True})
            require(not proxy.failures, "synthetic GUI endpoint assertions failed")
        report["status"] = "passed"
    except Exception as error:
        report["status"], report["failure"] = "failed", str(error)
        raise
    finally:
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for option in ("host", "worker", "package", "sdk", "output"):
        parser.add_argument("--" + option, type=Path, required=True)
    options = parser.parse_args()
    run(*(getattr(options, name).resolve() for name in ("host", "worker", "package", "sdk", "output")))
