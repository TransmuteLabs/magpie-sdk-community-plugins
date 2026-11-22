"""Exercise the built native Command Code package through the ordinary web API."""

import argparse
from contextlib import ExitStack
import hashlib
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import threading
import time
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlsplit
import urllib.request


MODULE = "protocol-commandcode"
PROVIDER = "commandcode-native"
PROVIDER_ID = "extension-" + hashlib.sha256((MODULE + "\0" + PROVIDER).encode()).hexdigest()[:24]


class Vendor(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        key = self.headers.get("Authorization", "").removeprefix("Bearer ")
        identity = self.server.identities.get(key)
        if identity is None:
            self.server.failures.append(f"unexpected credential or path: {self.path}")
            self.send_error(401)
            return
        if self.path == "/alpha/whoami":
            value = {"user": {"id": identity[0], "userName": identity[1]}}
        elif self.path == "/alpha/billing/subscriptions":
            value = {"data": {"planId": "individual-pro", "status": "active", "currentPeriodEnd": 1900000000000}}
        elif self.path == "/alpha/billing/credits":
            value = {"credits": {"monthlyCredits": 100, "purchasedCredits": identity[2]}}
        elif self.path == "/provider/v1/models":
            value = {"data": [{"id": "fixture-model", "supported_endpoints": ["/chat/completions"]}]}
        else:
            self.server.failures.append(f"unexpected request path: {self.path}")
            self.send_error(404)
            return
        body = json.dumps(value).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def verify(host, distribution, output):
    output.mkdir()
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    assert manifest["runtime"] == "native" and len(manifest["agents"]) == 1
    identities = {
        "SYNTHETIC-FIRST": ("first-user", "Initial name", 7),
        "SYNTHETIC-RENEWED": ("first-user", "Renewed name", 12),
        "SYNTHETIC-CLI": ("cli-user", "Local CLI name", 2),
        "SYNTHETIC-FOREIGN": ("other-user", "Renewed name", 9),
    }
    with ExitStack() as stack:
        vendor = stack.enter_context(ThreadingHTTPServer(("127.0.0.1", 0), Vendor))
        vendor.identities, vendor.failures = identities, []
        serving = threading.Thread(target=vendor.serve_forever)
        serving.start()
        stack.callback(serving.join, 5)
        stack.callback(vendor.shutdown)
        vendor_url = f"http://127.0.0.1:{vendor.server_port}"
        home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="commandcode-signin-")))
        (home / "tmp").mkdir()
        (home / ".commandcode").mkdir()
        (home / ".commandcode/auth.json").write_text(json.dumps({"apiKey": "SYNTHETIC-CLI"}))
        config = home / ".config/magpie"
        config.mkdir(parents=True)
        (config / "settings.json").write_text(json.dumps({"noStats": True, "noAutoUpdate": True}))
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        origin = f"http://127.0.0.1:{port}"
        env = {"HOME": str(home), "USERPROFILE": str(home), "PATH": "/usr/bin:/bin", "TMPDIR": str(home / "tmp"),
               "XDG_CONFIG_HOME": str(home / ".config"), "XDG_CACHE_HOME": str(home / ".cache"),
               "XDG_DATA_HOME": str(home / ".local/share"), "XDG_STATE_HOME": str(home / ".local/state"),
               "MAGPIE_NO_STATS": "1", "MAGPIE_BUN": str(home / "no-bun"),
               "MAGPIE_PLUGIN_CATALOG": "off", "MAGPIE_PLUGIN_MARKET": "off", "MAGPIE_PARTNERS": "off",
               "MAGPIE_WEB_KEY": "synthetic-commandcode-web", "MAGPIE_UPDATE_FEED": "http://127.0.0.1:1/disabled"}
        log = stack.enter_context((output / "host.log").open("w"))
        process = subprocess.Popen([str(host), "web", "--addr", f"127.0.0.1:{port}", "--no-open"],
                                   cwd=home, env=env, stdout=log, stderr=subprocess.STDOUT)

        def close():
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(5)

        stack.callback(close)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                             urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        deadline = time.monotonic() + 20
        while True:
            assert process.poll() is None and time.monotonic() < deadline, "Magpie web failed to start"
            try:
                with opener.open(origin + "/?k=" + env["MAGPIE_WEB_KEY"], timeout=2):
                    break
            except OSError:
                time.sleep(0.05)

        def request(path, payload=None, expected=200):
            post = None if payload is None else json.dumps(payload).encode()
            query = urllib.request.Request(origin + path, data=post,
                                           headers={"Content-Type": "application/json", "Origin": origin})
            try:
                response = opener.open(query, timeout=20)
            except HTTPError as error:
                response = error
            with response:
                data = json.loads(response.read(1 << 20))
                assert response.status == expected, (path, response.status, data)
            encoded = json.dumps(data)
            assert not any(key in encoded for key in identities), "public API returned a private credential"
            return data

        def cli(*arguments):
            return subprocess.run([str(host), *arguments], cwd=home, env=env,
                                  capture_output=True, text=True, check=True, timeout=40).stdout

        installed = request("/api/plugins/add", {"spec": str(distribution)})
        assert len(installed["plugins"]) == 1 and installed["plugins"][0]["version"] == manifest["version"]
        cli("plugin", "sdk", "config", MODULE, json.dumps({"apiURL": vendor_url, "studioURL": vendor_url, "zdr": True}),
            "--expected", "0")

        def subscription():
            row, = [row for row in request("/api/providers")["sdkSubscriptions"] if row["id"] == PROVIDER_ID]
            return row

        row = subscription()
        assert row["name"] == "Command Code" and row["reauthorize"] and not row.get("page")
        assert {method["id"]: method.get("fields", []) for method in row["methods"]} == {
            "api-key": [{"key": "apiKey", "label": "API key", "type": "secret"}], "browser": [], "cli-import": [],
        }
        assert not any(page["name"] == MODULE for page in request("/api/plugin-pages"))
        panel, = [item for item in request("/api/plugin-panels") if item["name"] == MODULE]
        assert panel["slot"] == "usage"
        panel_binding = {"ref": panel["ref"], "binding": "account.status"}

        def authorize(method, key="", selected=None):
            parent = secrets.token_hex(16)
            options = {"method": method}
            if method == "api-key":
                options["input"] = {"apiKey": key}
            if selected:
                options["account"] = {"id": selected["id"], "expectedGeneration": selected["generation"]}

            def call(operation, **fields):
                value = {"id": PROVIDER_ID, "revision": row["revision"], "parent": parent,
                         "request": secrets.token_hex(16), "operation": operation, **fields}
                result = request("/api/provider/sdk-signin", value)
                assert "continuation" not in result
                return result

            view = call("start", options=options)
            if method == "browser":
                launch = urlsplit(view["url"])
                assert launch.path == "/studio/auth/cli"
                params = parse_qs(launch.query)
                callback = params["callback"][0]
                parsed = urlsplit(callback)
                assert parsed.scheme == "http" and parsed.hostname == "127.0.0.1" and parsed.path == "/callback"
                approved = urllib.request.Request(callback, data=json.dumps({"state": params["state"][0], "apiKey": key}).encode(),
                                                  headers={"Content-Type": "application/json", "Origin": "https://commandcode.ai"})
                with opener.open(approved, timeout=5) as response:
                    assert json.loads(response.read()) == {"success": True}
            else:
                assert not view.get("url")
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            return call("finish", flow=view["flow"])

        def selected(account, name):
            listed, = [item for item in request("/api/providers")["providers"] if item["id"] == PROVIDER_ID]
            assert listed["accountUsers"][account["id"]] == name, listed.get("accountUsers")
            assert any(login.get("extensionGeneration") == account["generation"] for login in listed["account"]["logins"])

        def details(account, expected=200):
            return request("/api/plugin-pages/call", {**panel_binding, "input": {},
                           "account": {"id": account["id"], "expectedGeneration": account["generation"]}}, expected)

        first = authorize("api-key", "SYNTHETIC-FIRST")["account"]
        selected(first, "Initial name")
        imported = authorize("cli-import")["account"]
        selected(imported, "Local CLI name")
        renewed = authorize("browser", "SYNTHETIC-RENEWED", first)["account"]
        assert renewed["id"] == first["id"] and renewed["generation"] == "2"
        selected(renewed, "Renewed name")
        assert authorize("api-key", "SYNTHETIC-FOREIGN", renewed)["state"] != "complete"
        selected(renewed, "Renewed name")

        identity = identities["SYNTHETIC-RENEWED"]
        identities["SYNTHETIC-RENEWED"] = (identity[0], "Refreshed name", identity[2])
        refreshed = json.loads(cli("plugin", "sdk", "auth", "refresh", MODULE, PROVIDER, renewed["id"]))
        assert refreshed["id"] == renewed["id"] and refreshed["name"] == "Refreshed name"
        summaries = request("/api/plugin-pages/accounts", panel_binding)
        refreshed = next(account for account in summaries if account["id"] == first["id"])
        listed_imported = next(account for account in summaries if account["id"] == imported["id"])
        assert listed_imported["generation"] == imported["generation"] and listed_imported["name"] == "Local CLI name"
        selected(refreshed, "Refreshed name")
        details(renewed, 400)
        plan = details(refreshed)
        assert plan["subscriptionKnown"] and plan["subscription"]["until"] == 1900000000000
        assert plan["creditsKnown"] and plan["credits"]["extraUSD"] == 12 and plan["zdr"]
        assert plan["systemone"]["model"] == "typesafe/jev"
        assert details(imported)["credits"]["extraUSD"] == 2

        def logout(account, expected=200):
            return request("/api/login/forget", {"agent": PROVIDER_ID, "account": {
                "id": account["id"], "expectedGeneration": account["generation"],
            }}, expected)

        logout(first, 400)
        logout(refreshed)
        remaining = request("/api/plugin-pages/accounts", panel_binding)
        assert remaining == [listed_imported], remaining
        logout(imported)
        assert subscription()["id"] == PROVIDER_ID
        assert not any(item["id"] == PROVIDER_ID for item in request("/api/providers")["providers"])
        assert not vendor.failures, vendor.failures
        assert not (config / "plugins.json").exists() and not (config / "plugins/node_modules").exists()
    return {"version": manifest["version"], "signInMethods": 3, "selectedReauthorization": True,
            "foreignReauthorizationRefused": True, "refreshName": True, "selectedUsagePanel": True,
            "staleSignOutRefused": True, "selectedSignOut": True, "liveVendors": False}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for option in ("host", "distribution", "output"):
        parser.add_argument("--" + option, type=Path, required=True)
    options = parser.parse_args()
    result = verify(options.host.resolve(), options.distribution.resolve(), options.output.resolve())
    print(json.dumps(result))
