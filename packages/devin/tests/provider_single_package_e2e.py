"""One installed default provider: synthetic auth/catalog/HTTP/usage, no live data."""

import argparse
from contextlib import ExitStack
import gzip
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import struct
import tempfile
import threading
import time

from conformance import connect_frames, data_frame, fields, number_field, text_field
from http_e2e import CALLER_KEY, DenyProxy, SERVICE, chat, digest, emitted_text, require, server_thread, terminal
from paired_http_e2e import PairedGateway, SELECTED_KEY


MODEL = "provider-completion-model"
OTHER_KEYS = ("devin-session-token$completion-a", "cog_completion_b")
STATUS = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
MODELS = SERVICE + "GetCliModelConfigs"


class Vendor(ThreadingHTTPServer):
    def __init__(self):
        super().__init__(("127.0.0.1", 0), Handler)
        self.lock = threading.Lock()
        self.status_mode = self.catalog_mode = "ok"
        self.calls, self.requests, self.failures = [], [], []

    def modes(self, status="ok", catalog="ok"):
        with self.lock:
            self.status_mode, self.catalog_mode = status, catalog


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, content_type, body, status=200):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 < size <= 16 << 20, "invalid fixture body size")
            body = self.rfile.read(size)
            with self.server.lock:
                self.server.calls.append(self.path)
                mode = self.server.status_mode if self.path == STATUS else self.server.catalog_mode if self.path == MODELS else "ok"
            if mode == "unauthorized":
                self.reply("text/plain", b"PRIVATE rejected credential", 401)
                return
            if mode == "connect-auth":
                self.reply("application/json", b'{"error":{"code":"unauthenticated","message":"PRIVATE"}}', 502)
                return
            if mode == "unavailable":
                self.reply("text/plain", b"PRIVATE catalog unavailable", 503)
                return
            if self.path.endswith("/ExchangeDevinCLIPKCECode"):
                value = json.loads(body)
                require(set(value) == {"code", "code_verifier"} and value["code"] == "offline-code"
                        and 43 <= len(value["code_verifier"]) <= 128, "PKCE exchange changed")
                self.reply("application/json", json.dumps({"sessionToken": SELECTED_KEY}).encode())
                return
            if self.path == STATUS:
                key = json.loads(body)["metadata"]["apiKey"]
                require(key in (SELECTED_KEY, *OTHER_KEYS), "unselected status credential")
                value = {"planStatus": {"dailyQuotaRemainingPercent": 75}}
                if key == SELECTED_KEY:
                    value.update(email="named@example.invalid")
                    value["planStatus"]["planInfo"] = {"teamName": "Fixture organization"}
                self.reply("application/json", json.dumps({"userStatus": value}).encode())
                return
            if self.path == MODELS:
                key = fields(fields(body)[1][0])[3][0].decode()
                require(key in (SELECTED_KEY, *OTHER_KEYS), "unselected model credential")
                info = number_field(4, 65536) + number_field(13, 4096) + text_field(6, number_field(12, 1))
                models = b""
                for uid, free in ((MODEL, 0), ("provider-completion-free", 4)):
                    models += text_field(1, text_field(1, uid) + text_field(22, uid)
                                         + number_field(18, 65536) + number_field(24, free) + text_field(23, info))
                self.reply("application/proto", models)
                return
            require(self.path == SERVICE + "GetChatMessage", "unexpected vendor operation")
            require(self.headers.get("Authorization") == "Basic " + SELECTED_KEY,
                    "request did not use selected credential")
            parts = connect_frames(body)
            require(len(parts) == 1, "request did not contain one frame")
            flags, payload = parts[0]
            require(flags == 1 and self.headers.get("Connect-Content-Encoding") == "gzip",
                    "large request lost negotiated compression")
            payload = gzip.decompress(payload)
            request = fields(payload)
            require(request[21] == [MODEL.encode()] and fields(request[1][0])[3] == [SELECTED_KEY.encode()],
                    "model/credential projection changed")
            require(CALLER_KEY.encode() not in payload, "gateway credential escaped")
            require(b"completion-large-" in payload and len(payload) > 8192, "large request changed")
            with self.server.lock:
                source = "field7" if not self.server.requests else "field28"
                self.server.requests.append({"compressed": True, "bytes": len(payload), "usageSource": source})
            if source == "field7":
                usage = text_field(7, number_field(2, 7) + number_field(3, 3))
            else:
                group = text_field(1, "Token Usage")
                for key, value in (("input_tokens", 7), ("output_tokens", 3),
                                   ("cache_read_input_tokens", 0), ("cache_creation_input_tokens", 0)):
                    metric = text_field(4, b"\x15" + struct.pack("<f", value)) + text_field(5, key)
                    group += text_field(2, metric)
                usage = text_field(28, group)
            response = text_field(3, "self-contained answer") + number_field(5, 2) + usage
            self.reply("application/connect+proto", data_frame(response) + b"\2\0\0\0\2{}")
        except Exception as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.reply("application/json", b'{"code":"internal"}', 500)


class SingleGateway(PairedGateway):
    def sign_in_key(self, key, account=None):
        scope = ("--scope", json.dumps({"account": account})) if account else ()
        started = json.loads(self.command("plugin", "sdk", "auth", "start", "devin-inbound", "devin",
                                          json.dumps({"method": "api-key", "input": {"key": key}}), *scope))
        result = json.loads(self.command("plugin", "sdk", "auth", "finish", "devin-inbound", "devin",
                                         json.dumps({"continuation": started["continuation"]}), *scope))
        require(result.get("id") and not result.get("credentials"), "auth did not save privately")
        return result

    def usage(self, account, refused=False, typed=False):
        command = [str(self.magpie), "plugin", "sdk", "call", "devin-inbound", "provider.usage", "{}",
                   "--scope", json.dumps({"provider": "devin", "account": account})]
        result = subprocess.run(command, cwd=self.home, env=self.env, capture_output=True, text=True, timeout=45)
        if refused:
            message = "sign in again" if typed else "HTTP 401"
            require(result.returncode != 0 and message in result.stdout + result.stderr
                    and "PRIVATE" not in result.stdout + result.stderr,
                    "authentication refusal became deferred/opaque usage: " + result.stdout + result.stderr)
            return None
        require(result.returncode == 0, "usage command failed: " + result.stderr)
        frames = [json.loads(line) for line in result.stdout.splitlines()]
        return next(frame["result"] for frame in frames if frame["kind"] == "result")


def run(magpie, distribution, artifacts):
    artifacts.mkdir(parents=True, exist_ok=True)
    result = {"status": "running", "checks": [], "hostSHA256": digest(magpie),
              "librarySHA256": digest(distribution / "plugin.so"), "runnerSHA256": digest(__file__)}
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, Vendor())
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-one-provider-")))
            gateway = SingleGateway(magpie, distribution, home, artifacts, "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)
            gateway.env.update(MAGPIE_NO_ACCOUNTS="0", APPDATA=str(home / "appdata"), LOCALAPPDATA=str(home / "localappdata"))
            settings = json.loads((gateway.config / "settings.json").read_text())
            settings["noAccountDiscovery"] = False
            (gateway.config / "settings.json").write_text(json.dumps(settings))
            # No fake built-in/provider catalog is required by this native provider.
            for name in ("models.json", "labs.json"):
                (gateway.cache / name).write_text("{}")
            gateway.version = gateway.command("--version").strip()
            gateway.command("plugin", "sdk", "add", str(distribution))
            snapshot = json.loads(gateway.command("plugin", "sdk", "config", "devin-inbound"))
            origin = "http://127.0.0.1:" + str(vendor.server_port)
            gateway.command("plugin", "sdk", "config", "devin-inbound", json.dumps({"server": origin}),
                            "--expected", snapshot["version"])
            account = gateway.sign_in_key(SELECTED_KEY)
            require(account["name"] == "named@example.invalid · Fixture organization", "status identity was not projected")
            anonymous = [gateway.sign_in_key(key) for key in OTHER_KEYS]
            require(len({item["id"] for item in [account, *anonymous]}) == 3, "unknown identities collided")
            for key, item in zip(OTHER_KEYS, anonymous):
                expected = hashlib.sha256((origin + "\0" + key).encode()).hexdigest()[:32]
                require(item["id"] == expected and item["name"] == "Devin [" + expected[:12] + "]",
                        "fallback identity was not stable and private")
                require(gateway.sign_in_key(key, item["id"])["id"] == item["id"], "reauthentication changed identity")
            replacement = server_thread(stack, DenyProxy())
            snapshot = json.loads(gateway.command("plugin", "sdk", "config", "devin-inbound"))
            gateway.command("plugin", "sdk", "config", "devin-inbound",
                            json.dumps({"server": f"http://127.0.0.1:{replacement.server_port}"}),
                            "--expected", snapshot["version"])
            accounts_command = ("plugin", "sdk", "accounts", "devin-inbound", "devin", "--json")
            remaining = {item["id"]: item["generation"] for item in json.loads(gateway.command(*accounts_command))}
            account_id = anonymous[0]["id"]
            stale = subprocess.run([str(gateway.magpie), "plugin", "sdk", "auth", "logout", "devin-inbound",
                                    "devin", account_id, "--scope", json.dumps({"generation": remaining[account_id] - 1})],
                                   cwd=home, env=gateway.env, capture_output=True, text=True, timeout=45)
            require(stale.returncode != 0 and "SDK account generation changed" in stale.stderr,
                    "sign-out admitted a stale account generation")
            for item in anonymous:
                gateway.command("plugin", "sdk", "auth", "logout", "devin-inbound", "devin", item["id"])
                del remaining[item["id"]]
                require({item["id"]: item["generation"] for item in json.loads(gateway.command(*accounts_command))}
                        == remaining, "sign-out changed another account")
            require(not replacement.requests, "sign-out contacted the replacement server")
            snapshot = json.loads(gateway.command("plugin", "sdk", "config", "devin-inbound"))
            gateway.command("plugin", "sdk", "config", "devin-inbound", json.dumps({"server": origin}),
                            "--expected", snapshot["version"])
            result["checks"].append("selected local sign-out after changing the vendor server")
            gateway.browser_sign_in(account["id"])
            result["checks"].append("native API-key/session identity, scoped reauth/logout, PKCE mismatch/replay controls")
            usage = gateway.usage(account["id"])
            require(usage["windows"][0]["used"] == 25 and
                    usage["windows"][0]["notModels"] == ["provider-completion-free"], "quota/free scope changed")
            for status_mode, catalog_mode in (("unauthorized", "unavailable"), ("connect-auth", "ok"), ("ok", "unauthorized")):
                vendor.modes(status_mode, catalog_mode)
                gateway.usage(account["id"], refused=True, typed=status_mode == "connect-auth")
            vendor.modes(catalog="unavailable")
            deferred = gateway.usage(account["id"])
            require(deferred.get("readDeferred") and deferred.get("error") and not deferred.get("windows"),
                    "unavailable free scope became fresh quota")
            vendor.modes()
            require(gateway.usage(account["id"])["windows"] == usage["windows"], "fresh authority did not recover")
            result["checks"].append("status/catalog auth refusal, transient deferral and quota recovery through ordinary SDK calls")
            gateway.command("gateway-key", "add", "One provider caller", "--key", CALLER_KEY)
            gateway.start()
            deadline = time.monotonic() + 15
            model = None
            while time.monotonic() < deadline:
                status, _, body = gateway.request("/v1/models", method="GET")
                require(status == 200, "ordinary catalog failed")
                model = next((row["id"] for row in json.loads(body)["data"] if row["id"].endswith("/" + MODEL)), None)
                if model:
                    break
                time.sleep(0.1)
            require(model, "single provider model was not discoverable")
            prompt = "completion-large-" + "x" * 9000
            for protocol in ("chat", "anthropic", "native"):
                if protocol == "native":
                    status, _, body = gateway.request(SERVICE + "GetChatMessage", chat(prompt, model))
                    require(status == 200 and emitted_text(terminal(body)) == b"self-contained answer", "native path failed")
                else:
                    request = {"model": model, "messages": [{"role": "user", "content": prompt}], "max_tokens": 32}
                    path = "/v1/chat/completions" if protocol == "chat" else "/v1/messages"
                    status, _, body = gateway.request(path, json.dumps(request).encode(), headers={"Content-Type": "application/json"})
                    require(status == 200 and "self-contained answer" in body.decode(), protocol + " path failed: " + body.decode())
            require(len(vendor.requests) == 3 and all(item["compressed"] for item in vendor.requests),
                    "missing compression, extra inference or automatic keepalive")
            require([item["usageSource"] for item in vendor.requests] == ["field7", "field28", "field28"],
                    "native dimension usage did not exercise both ordinary entry paths")
            result["checks"].append("ordinary catalog + Chat/Messages/native inference, selected authority and >8KiB gzip")
            installed = json.loads(gateway.command("plugin", "sdk", "list", "--json"))
            require(len(installed) == 1 and installed[0]["manifest"]["id"] == "devin-inbound", "companion SDK module appeared")
            require(not (gateway.config / "plugins.json").exists() and not list(home.rglob("node_modules"))
                    and not Path(gateway.env["MAGPIE_BUN"]).exists(), "legacy companion runtime appeared")
            require(not proxy.requests and not vendor.failures, "fixture boundary failed")
            gateway.close()
            rows = [json.loads(line) for line in (gateway.config / "usage.jsonl").read_text().splitlines()]
            require(len(rows) == 3 and all(row.get("status") == 200 and row.get("kind") != "cache_keepalive"
                    and row.get("providerAccount") == account["id"] and row.get("in") == 7 and row.get("out") == 3
                    and row.get("cache_read", 0) == 0 and row.get("cache_write", 0) == 0 for row in rows),
                    "ordinary usage ledger lost measured counts/account or added background spending")
            require(all(key not in json.dumps(rows) for key in (SELECTED_KEY, CALLER_KEY, *OTHER_KEYS)),
                    "usage ledger exposed credentials")
            result["ledger"] = rows
            result["checks"].append("one package, no Bun/node_modules/legacy plugins; three successful ordinary usage rows")
            result["status"] = "passed"
    finally:
        if "home" in locals():
            result["homeRemoved"] = not home.exists()
        if "gateway" in locals():
            result["processStopped"] = gateway.process is None or gateway.process.poll() is not None
        if "vendor" in locals():
            result.update(vendorCalls=vendor.calls, requests=vendor.requests, vendorFailures=vendor.failures)
        if "proxy" in locals():
            result["externalRequests"] = len(proxy.requests)
        if result["status"] == "running":
            result["status"] = "failed"
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "fixture retained home/process")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("magpie", "distribution", "artifacts"):
        parser.add_argument("--" + name, required=True, type=Path)
    args = parser.parse_args()
    run(args.magpie.resolve(), args.distribution.resolve(), args.artifacts.resolve())
