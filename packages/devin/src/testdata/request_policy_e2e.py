"""Current installed native distribution and ordinary compiled host, offline."""

import argparse
from contextlib import ExitStack
import gzip
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import struct
import sys
import tempfile
import threading
import time

PACKAGE = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(PACKAGE / "tests"))
from conformance import connect_frames, data_frame, fields, number_field, text_field
from http_e2e import CALLER_KEY, DenyProxy, SERVICE, chat, digest, emitted_text, require, server_thread, terminal
from provider_single_package_e2e import SingleGateway

KEYS = ("policy-account-A", "policy-account-B", "policy-account-C")
PROVIDER = "extension-" + hashlib.sha256(b"devin-inbound\0devin").hexdigest()[:24]
FREE = "policy-free"
OTHER_FREE = "policy-free-sibling"
PAID = "policy-paid"
FREE_MESSAGE = "Reached free model rate limit. Please wait before retrying. Your limit will reset in "


def end_error(code, message):
    payload = json.dumps({"error": {"code": code, "message": message}}).encode()
    return b"\2" + struct.pack(">I", len(payload)) + payload


class Vendor(ThreadingHTTPServer):
    def __init__(self, scenario):
        super().__init__(("127.0.0.1", 0), Handler)
        self.scenario = scenario
        self.lock = threading.Lock()
        self.sends = []
        self.failures = []
        self.resets = (int(time.time()) + 86400, int(time.time()) + 604800)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, media, body, status=200):
        self.send_response(status)
        self.send_header("Content-Type", media)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            require(0 < length <= 16 << 20, "invalid request length")
            body = self.rfile.read(length)
            if self.path.endswith("/GetUserStatus"):
                key = json.loads(body)["metadata"]["apiKey"]
                require(key in KEYS, "status selected an unknown key")
                self.reply("application/json", json.dumps({"userStatus": {
                    "email": key + "@fixture.invalid",
                    "planStatus": {"dailyQuotaRemainingPercent": 100, "weeklyQuotaRemainingPercent": 100,
                                   "dailyQuotaResetAtUnix": self.server.resets[0],
                                   "weeklyQuotaResetAtUnix": self.server.resets[1]},
                }}).encode())
                return
            if self.path.endswith("/GetCliModelConfigs"):
                require(any(key.encode() in body for key in KEYS), "catalog selected an unknown key")
                models = b""
                for model, tier in ((FREE, 4), (OTHER_FREE, 4), (PAID, 0)):
                    models += text_field(1, text_field(1, model) + text_field(22, model) +
                                         number_field(18, 32768) + number_field(24, tier))
                self.reply("application/proto", models)
                return
            require(self.path == SERVICE + "GetChatMessage", "unexpected vendor operation")
            key = self.headers.get("Authorization", "").removeprefix("Basic ")
            require(key in KEYS, "inference selected an unknown key")
            framed = connect_frames(body)
            require(len(framed) == 1, "inference repeated native payload")
            payload = gzip.decompress(framed[0][1]) if framed[0][0] & 1 else framed[0][1]
            decoded = fields(payload)
            model = decoded[21][0].decode()
            require(key.encode() in decoded[1][0], "metadata and selected account disagree")
            with self.server.lock:
                count = sum(item["account"] == key and item["model"] == model for item in self.server.sends) + 1
                self.server.sends.append({"account": key, "model": model, "at": time.monotonic()})
            scenario = self.server.scenario
            refusal = None
            if scenario == "short-wait":
                if count == 1:
                    refusal = end_error("resource_exhausted", FREE_MESSAGE + "1 seconds")
            elif scenario in ("request-terminal", "static-auth"):
                if scenario == "request-terminal":
                    refusal = end_error("failed_precondition", "Client version 3000.1.0 is no longer supported")
                elif key != KEYS[2]:
                    refusal = end_error("unauthenticated", "The selected credential was rejected")
            elif model == FREE and key != KEYS[2] or scenario in ("all-long", "zero-budget", "zero-short-wait"):
                duration = {KEYS[0]: "42 seconds", KEYS[1]: "2 minutes", KEYS[2]: "2 HOURS"}[key]
                refusal = end_error("resource_exhausted", FREE_MESSAGE + duration)
                if scenario == "no-reset":
                    refusal = end_error("resource_exhausted", "Reached free model rate limit for devin/swe-2.")
            if scenario in ("prior-zero", "prior-output", "trailing", "unknown"):
                refusal = end_error("resource_exhausted", FREE_MESSAGE + "42 seconds")
                if scenario == "prior-zero":
                    refusal = data_frame(text_field(7, number_field(2, 0))) + refusal
                elif scenario == "prior-output":
                    refusal = data_frame(text_field(3, "previous paid work")) + refusal
                elif scenario == "trailing":
                    refusal += b"x"
                else:
                    refusal = end_error("resource_exhausted", "unknown failure")
            if scenario in ("duplicate-error", "null-error", "duplicate-metadata"):
                payload = {
                    "duplicate-error": b'{"error":{"code":"unavailable","code":"resource_exhausted","message":"Reached free model rate limit"}}',
                    "null-error": b'{"error":{"code":"unauthenticated","message":null}}',
                    "duplicate-metadata": b'{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"},"metadata":{"trace":["first"],"trace":["second"]}}',
                }[scenario]
                refusal = b"\2" + struct.pack(">I", len(payload)) + payload
            if refusal is not None:
                self.reply("application/connect+proto", refusal)
                return
            response = text_field(3, "current policy answer") + number_field(5, 2)
            self.reply("application/connect+proto", data_frame(response) + b"\2\0\0\0\2{}")
        except Exception as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.reply("application/json", b'{"code":"internal"}', 500)


def run(magpie, distribution, artifacts, scenario, protocol):
    artifacts.mkdir(parents=True, exist_ok=True)
    result = {"status": "running", "scenario": scenario, "protocol": protocol,
              "hostSHA256": digest(magpie), "librarySHA256": digest(distribution / "plugin.so"),
              "manifestSHA256": digest(distribution / "magpie-plugin.json"), "runnerSHA256": digest(__file__)}
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, Vendor(scenario))
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-policy-installed-")))
            gateway = SingleGateway(magpie, distribution, home, artifacts, "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)
            gateway.env.update(MAGPIE_NO_ACCOUNTS="0", APPDATA=str(home / "appdata"), LOCALAPPDATA=str(home / "localappdata"))
            settings = json.loads((gateway.config / "settings.json").read_text())
            policy = {"maxRetries": 2, "maxTotalRetries": 2, "baseDelayMs": 0,
                      "maxDelayMs": 7200000, "shortWaitMs": 0, "maxElapsedMs": 10000, "jitterPercent": 0}
            if scenario == "budget":
                policy["maxTotalRetries"] = 1
            if scenario == "zero-budget":
                policy["maxTotalRetries"] = 0
            if scenario == "short-wait":
                policy.update(shortWaitMs=2000, maxDelayMs=2000)
            if scenario == "no-reset":
                policy["maxRetries"] = 0
            settings.update(noAccountDiscovery=False, requestPolicy={"defaults": policy})
            (gateway.config / "settings.json").write_text(json.dumps(settings))
            for name in ("models.json", "labs.json"):
                (gateway.cache / name).write_text("{}")
            gateway.version = gateway.command("--version").strip()
            gateway.command("plugin", "sdk", "add", str(distribution))
            snapshot = json.loads(gateway.command("plugin", "sdk", "config", "devin-inbound"))
            origin = "http://127.0.0.1:" + str(vendor.server_port)
            gateway.command("plugin", "sdk", "config", "devin-inbound", json.dumps({"server": origin}),
                            "--expected", snapshot["version"])
            accounts = [gateway.sign_in_key(key) for key in KEYS]
            order = accounts[:1] if scenario in ("short-wait", "zero-short-wait") else accounts
            gateway.command("provider", "set", PROVIDER, "routing=order")
            gateway.command("provider", "pool", PROVIDER, "--accounts", ",".join(item["id"] for item in order))
            usage = gateway.usage(accounts[0]["id"])
            windows = usage["windows"]
            require(len(windows) == 2 and all(set(window["notModels"]) == {FREE, OTHER_FREE} for window in windows),
                    "paid quota observations lost costTier4 exclusions")
            require([window["resetsAt"] for window in windows] == [reset * 1000 for reset in vendor.resets],
                    "paid epoch seconds were not converted once to milliseconds")
            gateway.command("gateway-key", "add", "Policy fixture", "--key", CALLER_KEY)
            gateway.start()

            def ask(model=FREE):
                full = PROVIDER + "/" + model
                if protocol == "native":
                    return gateway.request(SERVICE + "GetChatMessage", chat("policy current request", full))
                path = "/v1/messages" if protocol == "anthropic" else "/v1/chat/completions"
                body = json.dumps({"model": full, "max_tokens": 16, "messages": [{"role": "user", "content": "policy current request"}]}).encode()
                return gateway.request(path, body, headers={"Content-Type": "application/json"})

            started = time.monotonic()
            status, headers, body = ask()
            elapsed = time.monotonic() - started
            with vendor.lock:
                sends = list(vendor.sends)
            labels = [item["account"] for item in sends]
            expected = list(KEYS)
            want = 200
            if scenario in ("budget",):
                expected, want = list(KEYS[:2]), 429
            elif scenario in ("zero-budget", "zero-short-wait"):
                expected, want = list(KEYS[:1]), 429
            elif scenario == "all-long":
                want = 429
            elif scenario == "request-terminal":
                expected, want = list(KEYS[:1]), 400
            elif scenario in ("prior-zero", "prior-output", "trailing", "unknown",
                              "duplicate-error", "null-error", "duplicate-metadata"):
                expected, want = list(KEYS[:1]), 409
            elif scenario == "short-wait":
                expected = [KEYS[0], KEYS[0]]
                require(len(sends) == 2 and sends[1]["at"] - sends[0]["at"] >= 0.95, "retry preceded the typed one-second reset")
            if protocol == "native" and want != 200:
                require(status in (200, want) and b"current policy answer" not in body, "native refusal became successful output")
            else:
                require(status == want, "ordinary host status mismatch: " + repr((status, want, body.decode(errors="replace"))))
            require(labels == expected, "ordinary account order/root budget/replay fence mismatch: " + repr(labels))
            require(elapsed < 8, "long reset started a blocking sleep")
            if scenario == "all-long":
                require(int(headers.get("Retry-After", "0")) >= 7190, "two-hour reset was shortened in the final refusal")
            if want == 200:
                if protocol == "native":
                    require(emitted_text(terminal(body)) == b"current policy answer", "native answer missing")
                else:
                    require(b"current policy answer" in body, "current successful answer missing")
            if scenario == "three-accounts":
                require(ask()[0] == 200, "known free-window observations did not permit C")
                require(ask(PAID)[0] == 200 and ask(OTHER_FREE)[0] == 200, "free limit parked independent paid/free models")
                with vendor.lock:
                    require([item["account"] for item in vendor.sends[3:]] == [KEYS[2], KEYS[0], KEYS[0]],
                            "free observation leaked across accounts or models")
            gateway.close()
            log = (artifacts / "gateway.log").read_text()
            require(not any(key in log for key in KEYS), "diagnostics leaked selected credentials")
            result.update(status="passed", httpStatus=status, elapsedSeconds=elapsed, headers=headers,
                          sends=vendor.sends, paidWindows=windows,
                          finalDecisions=[line for line in log.splitlines() if "request policy final:" in line],
                          intermediateDecisions=[line for line in log.splitlines() if "request policy:" in line])
            require(not vendor.failures and not proxy.requests, "fixture or network boundary failed")
    finally:
        if "home" in locals():
            result["homeRemoved"] = not home.exists()
        if "gateway" in locals():
            result["processStopped"] = gateway.process is None or gateway.process.poll() is not None
        if "vendor" in locals():
            result["vendorFailures"] = vendor.failures
            result["sends"] = vendor.sends
        if "proxy" in locals():
            result["externalRequests"] = len(proxy.requests)
        if result["status"] == "running":
            result["status"] = "failed"
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "fixture retained its home/process")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--magpie", required=True, type=Path)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--artifacts", required=True, type=Path)
    parser.add_argument("--scenario", required=True)
    parser.add_argument("--protocol", default="chat")
    args = parser.parse_args()
    run(args.magpie.resolve(), args.distribution.resolve(), args.artifacts.resolve(), args.scenario, args.protocol)
