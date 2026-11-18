"""Actual GUI broker and module-owned loopback PKCE, synthetic accounts only."""

import argparse
from contextlib import ExitStack
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

from http_e2e import Gateway, DenyProxy, digest, require, server_thread

MODULE = "devin-inbound"
PROVIDER = "devin"


class Endpoint(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 < size < 65536, "invalid synthetic auth request size")
            value = json.loads(self.rfile.read(size))
            if self.path.endswith("/ExchangeDevinCLIPKCECode"):
                require(set(value) == {"code", "code_verifier"} and value["code"] == "synthetic-gui-code"
                        and 43 <= len(value["code_verifier"]) <= 128, "GUI PKCE exchange changed")
                with self.server.lock:
                    self.server.exchanges += 1
                    identity = self.server.exchanges
                if self.server.hold:
                    self.server.arrived.set()
                    require(self.server.release.wait(15), "held PKCE exchange did not release")
                result = {"sessionToken": "synthetic-window-session-" + str(identity)}
            elif self.path.endswith("/GetUserStatus"):
                require(value["metadata"]["apiKey"].startswith("synthetic-window-session-"),
                        "status used an unexpected credential")
                result = {"userStatus": {"email": "window@example.invalid"}}
            else:
                raise AssertionError("unexpected synthetic auth endpoint")
            data = json.dumps(result).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except Exception as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.send_error(500, "synthetic endpoint assertion failed")


def copy_assets(distribution, package, sdk):
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    if not manifest.get("window"):
        return
    shutil.copytree(package / "web", distribution / "web")
    shutil.copy2(package / "web/package.json", distribution / "package.json")
    for name in ("ui.js", "account-selection.js", "errors.js"):
        shutil.copy2(sdk / "sdk/js" / name,
                     distribution / "web" / ("sdk-ui.js" if name == "ui.js" else name))


def run(host_binary, worker, package, sdk, output):
    require(not output.exists(), "evidence output must be new")
    output.mkdir()
    distribution = output / "exec-test-package"
    (distribution / "bin").mkdir(parents=True)
    shutil.copy2(worker, distribution / "bin/devin")
    (distribution / "magpie-plugin.json").write_bytes(
        subprocess.check_output([str(worker), "--manifest"], timeout=10))
    copy_assets(distribution, package, sdk)
    report = {"test": "TestInstalledDevinGUIFirstAccount", "cases": [],
              "hostSHA256": digest(host_binary), "workerSHA256": digest(worker),
              "runnerSHA256": digest(__file__),
              "runtime": "actual GUI HTTP/broker, module loopback receiver and exec handlers; no rendered browser"}
    try:
        with ExitStack() as stack:
            vendor = ThreadingHTTPServer(("127.0.0.1", 0), Endpoint)
            vendor.lock, vendor.exchanges, vendor.failures = threading.Lock(), 0, []
            vendor.hold, vendor.arrived, vendor.release = False, threading.Event(), threading.Event()
            server_thread(stack, vendor)
            stack.callback(vendor.release.set)
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-gui-")))
            host = Gateway(host_binary, distribution, home, output, "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(host.close)
            host.env.update(MAGPIE_NO_ACCOUNTS="0", MAGPIE_ADDR="127.0.0.1:0",
                            MAGPIE_WEB_KEY="synthetic-devin-gui-entry")
            settings = json.loads((host.config / "settings.json").read_text())
            settings["noAccountDiscovery"] = False
            (host.config / "settings.json").write_text(json.dumps(settings))
            for name in ("models.json", "labs.json"):
                (host.cache / name).write_text("{}")
            host.command("plugin", "sdk", "add", str(distribution))
            snapshot = json.loads(host.command("plugin", "sdk", "config", MODULE))
            server = "http://127.0.0.1:" + str(vendor.server_port)
            host.command("plugin", "sdk", "config", MODULE, json.dumps({"server": server}),
                         "--expected", snapshot["version"])
            host.log = (output / "host.log").open("w")
            host.process = subprocess.Popen(
                [str(host_binary), "web", "--addr", "127.0.0.1:" + str(host.port), "--no-open"],
                cwd=home, env=host.env, stdout=host.log, stderr=subprocess.STDOUT, start_new_session=True)
            origin = "http://127.0.0.1:" + str(host.port)
            opener = urllib.request.build_opener(
                urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            deadline = time.monotonic() + 20
            while True:
                require(host.process.poll() is None, "GUI host exited")
                try:
                    with opener.open(origin + "/?k=" + host.env["MAGPIE_WEB_KEY"], timeout=2) as response:
                        if response.status == 200:
                            break
                except OSError:
                    require(time.monotonic() < deadline, "GUI host failed to start")
                    time.sleep(0.05)

            def request(path, value=None, success=True):
                body = None if value is None else json.dumps(value).encode()
                message = urllib.request.Request(origin + path, data=body,
                    headers={"Content-Type": "application/json", "Origin": origin})
                try:
                    with opener.open(message, timeout=20) as response:
                        raw = response.read(1 << 20)
                    require(success is not False, "revoked GUI request succeeded")
                    return json.loads(raw)
                except urllib.error.HTTPError:
                    require(success is not True, "ordinary GUI request failed")
                    return None

            def accounts():
                return json.loads(host.command("plugin", "sdk", "accounts", MODULE, PROVIDER, "--json"))

            require(not accounts(), "fixture must begin with zero accounts")
            pages = [item for item in request("/api/plugin-pages") if item["name"] == MODULE]
            require(len(pages) == 1, "FIRST-ACCOUNT: installed Devin has no ordinary GUI sign-in page")
            page = pages[0]
            for asset in ("index.html", "auth.js", "app.js", "style.css", "sdk-ui.js"):
                with opener.open(origin + page["url"].rsplit("/", 1)[0] + "/" + asset, timeout=10) as response:
                    require(response.status == 200 and response.read(), "packaged GUI asset missing")
            report["cases"].append("zero-account-page")
            parent, sequence = "a" * 64, 0

            def auth(operation, flow="", selected_parent=parent, ref=None, success=True):
                nonlocal sequence
                sequence += 1
                value = {"ref": ref or page["ref"], "binding": "page.auth.devin", "parent": selected_parent,
                         "request": format(sequence, "064x"), "operation": operation}
                if operation == "start":
                    value["options"] = {"method": "browser"}
                else:
                    value["flow"] = flow
                result = request("/api/plugin-pages/auth", value, success)
                if result is not None:
                    public = json.dumps(result)
                    require(not any(secret in public for secret in (
                        "synthetic-window-session-", "code_verifier", "continuation", "synthetic-gui-code")),
                        "GUI exposed private authentication material")
                return result

            def callback(view, wrong=False):
                address = urllib.parse.urlsplit(view["url"])
                query = urllib.parse.parse_qs(address.query)
                redirect = query["redirect_uri"][0]
                require(redirect.startswith("http://127.0.0.1:") and redirect.endswith("/callback"),
                        "browser redirect changed donor loopback authority")
                parameters = urllib.parse.urlencode({
                    "state": "foreign" if wrong else query["state"][0], "code": "synthetic-gui-code"})
                try:
                    with opener.open(redirect + "?" + parameters, timeout=5) as response:
                        body = response.read()
                        require(not wrong and response.status == 200 and b"synthetic-gui-code" not in body,
                                "callback accepted foreign state or returned private data")
                except urllib.error.HTTPError as error:
                    require(wrong and error.code == 400, "valid module callback was refused")

            view = auth("start")
            require(view["state"] == "waiting" and vendor.exchanges == 0, "browser start exchanged credentials")
            early = auth("finish", view["flow"])
            require(early["state"] == "waiting" and vendor.exchanges == 0, "early browser poll dispatched")
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            pending = auth("finish", view["flow"])
            require(pending["state"] == "waiting" and not accounts() and vendor.exchanges == 0,
                    "poll without callback did not remain pending")
            callback(view, wrong=True)
            require(vendor.exchanges == 0, "foreign callback exchanged credentials")
            callback(view)
            require(vendor.exchanges == 0, "callback handler exchanged outside auth.finish")
            time.sleep(pending["retryAfterMs"] / 1000 + 0.05)
            complete = auth("finish", view["flow"])
            require(complete["state"] == "complete" and len(accounts()) == 1 and vendor.exchanges == 1,
                    "ordinary browser poll did not publish its first account once")
            repeated = auth("finish", view["flow"])
            require(repeated["state"] == "complete" and vendor.exchanges == 1, "GUI completion exchanged again")
            report["cases"].append("loopback-pending-first-account-single-exchange")
            view = auth("start")
            callback(view)
            before = vendor.exchanges
            auth("finish", view["flow"], selected_parent="b" * 64, success=False)
            require(auth("cancel", view["flow"])["state"] == "canceled", "cancel failed")
            require(auth("finish", view["flow"])["state"] == "canceled" and vendor.exchanges == before,
                    "canceled/foreign flow exchanged")
            report["cases"].append("cancel-foreign-parent-zero-exchange")
            view = auth("start")
            old_ref = page["ref"]
            snapshot = json.loads(host.command("plugin", "sdk", "config", MODULE))
            host.command("plugin", "sdk", "config", MODULE,
                         json.dumps({"server": server, "keepAlive": False}),
                         "--expected", snapshot["version"])
            before = vendor.exchanges
            auth("finish", view["flow"], ref=old_ref, success=False)
            page = next(item for item in request("/api/plugin-pages") if item["name"] == MODULE)
            require(page["ref"] != old_ref and vendor.exchanges == before, "stale page dispatched")
            report["cases"].append("stale-config-page")
            host.command("accounts", "discovery", "off")
            auth("start", success=False)
            require(vendor.exchanges == before, "discovery-off start dispatched")
            host.command("accounts", "discovery", "on")
            report["cases"].append("discovery-off-start")
            view = auth("start")
            callback(view)
            vendor.hold = True
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            results, failures = [], []

            def held_finish():
                try:
                    results.append(auth("finish", view["flow"], success=None))
                except Exception as error:
                    failures.append(str(error))

            thread = threading.Thread(target=held_finish, name="devin-gui-held-finish")
            thread.start()
            try:
                require(vendor.arrived.wait(10), "browser completion did not reach held exchange")
                host.command("accounts", "discovery", "off")
            finally:
                vendor.release.set()
                thread.join(15)
            require(not thread.is_alive() and not failures, "held browser completion did not join")
            require(results and (results[0] is None or results[0]["state"] != "complete") and len(accounts()) == 1,
                    "revoked browser completion published an account")
            host.command("accounts", "discovery", "on")
            report["cases"].append("discovery-off-held-exchange")
            view = auth("start")
            callback(view)
            before = vendor.exchanges
            host.command("plugin", "sdk", "off", MODULE)
            auth("finish", view["flow"], success=False)
            require(vendor.exchanges == before and not [item for item in request("/api/plugin-pages") if item["name"] == MODULE],
                    "disabled package retained a page or exchanged")
            report["cases"].append("disabled-before-finish")
            require(not vendor.failures and not proxy.requests, "fixture endpoint/proxy assertions failed")
        report["status"] = "passed"
    except Exception as error:
        report["status"], report["failure"] = "failed", str(error)
        raise
    finally:
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ("host", "worker", "package", "sdk", "output"):
        parser.add_argument("--" + name, type=Path, required=True)
    options = parser.parse_args()
    run(*(getattr(options, name).resolve() for name in ("host", "worker", "package", "sdk", "output")))
