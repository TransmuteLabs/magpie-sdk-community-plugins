"""Public policy integration through an unchanged host and an exec test worker."""

import argparse
import base64
from contextlib import ExitStack
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
from urllib.parse import parse_qs, urlsplit


specification = importlib.util.spec_from_file_location("minimax_host_fixture", Path(__file__).with_name("host_e2e.py"))
fixture = importlib.util.module_from_spec(specification)
specification.loader.exec_module(fixture)
require = fixture.require
MODEL = "MiniMax-M3"


class PolicyEndpoint(fixture.EndpointHandler):
    def handle_request(self):
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 <= size <= fixture.LIMIT and not self.headers.get("Transfer-Encoding"), "unbounded fixture request")
            body = self.rfile.read(size)
            parsed = urlsplit(self.path)
            value = json.loads(body) if body and "application/json" in self.headers.get("Content-Type", "") else {
                key: values[0] for key, values in parse_qs(body.decode()).items()}
            headers = {key.lower(): value for key, value in self.headers.items()}
            require(fixture.CALLER not in json.dumps(headers) + body.decode(), "caller secret reached upstream")
            if not self.authority.startswith("account."):
                require(headers.get("authorization", "").removeprefix("Bearer ") in self.server.tokens,
                        "request used credentials outside the two selected accounts")
            self.server.record({"host": self.authority, "path": parsed.path, "method": self.command,
                                "headers": headers, "body": value})
            self.dispatch(parsed.path, value, fixture.GLOBAL)
        except (OSError, ValueError, AssertionError) as error:
            self.server.failures.append(repr(error))
            self.close_connection = True

    do_GET = handle_request
    do_POST = handle_request
    do_PUT = handle_request

    def dispatch(self, path, value, region):
        if path == "/mavis/api/v1/models":
            self.reply({"providers": [{"providerId": "minimax", "config": {"model_order": [MODEL],
                "models": {MODEL: {"name": "Synthetic MiniMax", "limit": {"context": 32768, "output": 4096},
                    "reasoning": False, "thinking_config": {"mode": "forced_off"},
                    "modalities": {"input": ["text"]}}}}}]})
            return
        if path == "/oauth2/device/code":
            identity = "alice" if not self.server.devices else "bob"
            device = "device-" + identity
            self.server.devices[device] = value["code_challenge"]
            self.reply({"device_code": device, "user_code": identity, "verification_uri": "https://account.minimax.io/device",
                        "expires_in": 300, "interval": 0.01})
            return
        if path == "/oauth2/token":
            device = value["device_code"]
            challenge = base64.urlsafe_b64encode(hashlib.sha256(value["code_verifier"].encode()).digest()).decode().rstrip("=")
            require(challenge == self.server.devices[device], "device exchange changed its verifier")
            identity = device.removeprefix("device-")
            access = "synthetic-policy-access-" + identity
            self.server.tokens[access] = identity
            self.reply({"access_token": access, "refresh_token": "synthetic-policy-refresh-" + identity,
                        "token_type": "Bearer", "expires_in": 3600})
            return
        if path == "/v1/api/user/info":
            identity = self.server.tokens[self.headers["Authorization"].removeprefix("Bearer ")]
            self.reply({"statusInfo": {"code": 0}, "data": {"userInfo": {
                "realUserID": identity, "userEmail": identity + "@example.invalid", "name": identity}}})
            return
        if path == "/minimax-cloud/api/v1/connectors/tools/call" and self.server.mode == "media":
            self.reply({"is_error": True, "error_code": 1008, "error_message": "credit exhausted after accepted work"})
            return
        super().dispatch(path, value, region)

    def chat(self, request, region):
        require(request.get("model") == MODEL and self.headers.get("X-Api-Key") == "sk-xxx"
                and self.headers.get("X-Mavis-Agent-Id") == "main", "selected chat wire changed")
        token = self.headers["Authorization"].removeprefix("Bearer ")
        with self.server.lock:
            if self.server.primary is None:
                self.server.primary = token
            primary = self.server.primary
        mode = self.server.mode
        if mode in ("fallback", "prose", "exhausted", "direct") and (token == primary or mode in ("exhausted", "direct")):
            body = {"base_resp": {"status_code": 1008, "status_msg": "credit exhausted " + token}}
            if mode == "prose":
                body = {"type": "error", "error": {"type": "permission_error", "message": "积分余额不足 " + token}}
            self.send_response(403 if mode == "prose" else 200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Retry-After", "7200")
            self.send_header("Connection", "close")
            encoded = json.dumps(body).encode()
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            self.wfile.flush()
            self.close_connection = True
            return
        if mode == "permission":
            self.reply({"type": "error", "error": {"type": "permission_error", "message": "insufficient permissions"}}, 403)
            return
        if mode == "content":
            self.reply({"type": "error", "error": {"type": "content_policy_error", "message": "quota exceeded"}}, 403)
            return
        if mode == "malformed":
            self.reply(b'{"base_resp":{"status_code":1008.5,"status_msg":"quota exceeded"}}')
            return
        if mode == "partial":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Connection", "close")
            self.end_headers()
            self.event("message_start", {"type": "message_start", "message": {"type": "message", "id": "paid",
                        "model": MODEL, "role": "assistant", "content": [], "usage": {"input_tokens": 1}}})
            self.event("content_block_start", {"type": "content_block_start", "index": 0,
                                             "content_block": {"type": "text", "text": ""}})
            self.event("content_block_delta", {"type": "content_block_delta", "index": 0,
                                             "delta": {"type": "text_delta", "text": "paid output"}})
            self.event("error", {"type": "error", "error": {"type": "rate_limit_error", "code": 1008,
                                                         "message": "credit exhausted"}})
            self.close_connection = True
            return
        answer = "answer-" + self.server.tokens[token]
        if request.get("stream") and self.server.success_format == "negotiated":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Connection", "close")
            self.end_headers()
            self.event("message_start", {"type": "message_start", "message": {"type": "message", "id": "msg-policy",
                        "model": MODEL, "role": "assistant", "content": [], "usage": {"input_tokens": 7, "output_tokens": 0}}})
            self.event("content_block_start", {"type": "content_block_start", "index": 0,
                                             "content_block": {"type": "text", "text": ""}})
            self.event("content_block_delta", {"type": "content_block_delta", "index": 0,
                                             "delta": {"type": "text_delta", "text": answer}})
            self.event("content_block_stop", {"type": "content_block_stop", "index": 0})
            self.event("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None},
                                        "usage": {"output_tokens": 3}})
            self.event("message_stop", {"type": "message_stop"})
            self.close_connection = True
            return
        self.reply({"id": "msg-policy", "type": "message", "role": "assistant", "model": MODEL,
                    "content": [{"type": "text", "text": answer}],
                    "stop_reason": "end_turn", "stop_sequence": None,
                    "usage": {"input_tokens": 7, "output_tokens": 3}})


fixture.EndpointHandler = PolicyEndpoint


class PolicyHost(fixture.Host):
    def install(self):
        self.command("plugin", "sdk", "add", str(self.distribution))
        installed = json.loads(self.command("plugin", "sdk", "list", "--json"))
        require(len(installed) == 1 and installed[0]["manifest"]["runtime"] == "exec", "exec test harness was not installed")
        self.command("gateway-key", "add", "MiniMax policy fixture", "--key", fixture.CALLER)
        return installed[0]["manifest"]


def sign_in(host):
    accounts = []
    for _ in range(2):
        start = json.loads(host.command("plugin", "sdk", "auth", "start", fixture.MODULE, fixture.GLOBAL,
                                       json.dumps({"method": "device"})))
        time.sleep(max(start.get("pollAfterMs", 0) / 1000, 0.02))
        saved = json.loads(host.command("plugin", "sdk", "auth", "finish", fixture.MODULE, fixture.GLOBAL,
                                       json.dumps({"continuation": start["continuation"]})))
        require(saved.get("id") and not saved.get("credentials"), "account was not saved by the ordinary auth owner")
        accounts.append(saved["id"])
    require(len(set(accounts)) == 2, "fixture did not establish two distinct accounts")
    return accounts


def run(host_binary, worker, output, modes, protocol, success_format, caller_stream):
    require(not output.exists(), "evidence directory must be new")
    output.mkdir()
    distribution = output / "exec-test-package"
    (distribution / "bin").mkdir(parents=True)
    shutil.copy2(worker, distribution / "bin/minimax")
    manifest_bytes = subprocess.check_output([str(worker), "--manifest"], timeout=10)
    manifest = json.loads(manifest_bytes)
    (distribution / "magpie-plugin.json").write_bytes(manifest_bytes)
    fixture.copy_window_assets(distribution)
    report = {"runtime": "exec test harness; native loading is not verified", "protocol": protocol,
              "successFormat": success_format, "callerStream": caller_stream, "hostSHA256": fixture.sha256(host_binary),
              "workerSHA256": fixture.sha256(worker), "manifest": manifest, "cases": []}
    try:
        for mode in modes:
            case_output = output / mode
            case_output.mkdir()
            proxy = host = None
            try:
                with ExitStack() as stack:
                    home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="minimax-policy-home-")))
                    ca, context = fixture.certificate(case_output, {"HOME": str(home), "PATH": "/usr/bin:/bin"})
                    shutil.copy2(ca, distribution / "bin/fixture-ca.pem")
                    proxy = fixture.Fixture(context)
                    proxy.mode, proxy.primary, proxy.devices, proxy.tokens = mode, None, {}, {}
                    proxy.success_format = success_format
                    proxy.start(stack)
                    host = PolicyHost(host_binary, distribution, home, case_output,
                                      "http://127.0.0.1:" + str(proxy.server_port), ca)
                    stack.callback(host.close)
                    settings_file = host.config / "settings.json"
                    settings = json.loads(settings_file.read_text())
                    settings["requestPolicy"] = {"defaults": {"maxRetries": 0, "maxTotalRetries": 3,
                                                             "shortWaitMs": 0, "baseDelayMs": 0}}
                    settings_file.write_text(json.dumps(settings))
                    host.install()
                    accounts = sign_in(host)
                    direct = None
                    if mode == "fallback":
                        proxy.mode = "direct"
                        payload = {"protocol": "messages", "model": MODEL, "body": base64.b64encode(
                            json.dumps({"model": MODEL, "messages": []}).encode()).decode()}
                        direct = host.sdk("provider.execute", payload, fixture.GLOBAL, accounts[0], session="policy-direct")
                        proxy.mode, proxy.primary = mode, None
                    host.start()
                    model = fixture.gateway_provider(fixture.GLOBAL) + "/" + MODEL
                    fixture.wait_for(lambda: model.encode() in host.request("/v1/models", method="GET")[2],
                                     "two-account provider was not published")
                    request = {"model": model, "messages": [{"role": "user", "content": "policy-" + mode}],
                               "max_tokens": 64, "stream": mode == "partial" or caller_stream}
                    endpoint = "/v1/messages" if protocol == "messages" else "/v1/chat/completions"
                    if mode == "media":
                        endpoint = "/v1/images/generations"
                        request = {"model": fixture.gateway_provider(fixture.GLOBAL) + "/MiniMax-Image",
                                   "prompt": "credit refusal must not regenerate paid work"}
                    before = len(proxy.snapshot())
                    status, headers, body = host.request(endpoint, request, session="policy-" + mode)
                    calls = [row for row in proxy.snapshot()[before:] if row["path"].endswith("/llm/v1/messages")
                             or row["path"].endswith("/connectors/tools/call")]
                    selected = [row["headers"]["authorization"] for row in calls]
                    observed = {"case": mode, "status": status, "dispatches": len(calls),
                                "distinctAccounts": len(set(selected)), "accounts": accounts,
                                "upstreamStream": [row["body"].get("stream") for row in calls]}
                    report["cases"].append(observed)
                    (case_output / "response-body.txt").write_bytes(body)
                    (case_output / "response-headers.json").write_text(json.dumps(headers, indent=2))
                    expected_status, expected_calls = {"success": (200, 1), "fallback": (200, 2), "prose": (200, 2), "exhausted": (429, 2),
                        "permission": (403, 1), "content": (403, 1), "malformed": (409, 1),
                        "partial": (200, 1), "media": (409, 1)}[mode]
                    if success_format == "whole-json" and protocol == "chat" and mode == "success":
                        expected_status = 409
                    require(status == expected_status and len(calls) == expected_calls,
                            "ordinary routing violated the refusal fence: " + json.dumps(observed))
                    require(len(set(selected)) == expected_calls, "one account was regenerated by a hidden retry")
                    if protocol == "chat" and mode != "media":
                        require(all(row["body"].get("stream") is True for row in calls),
                                "Chat translation did not exercise the requested upstream stream")
                    require(all(secret not in body.decode(errors="replace") for secret in proxy.tokens),
                            "selected secret reached the caller")
                    if mode in ("success", "fallback", "prose") and success_format == "negotiated":
                        require(b"answer-" in body, "successful completion lost its answer")
                        if not caller_stream and protocol == "chat":
                            completed = json.loads(body)
                            require(completed["choices"][0]["finish_reason"] == "stop"
                                    and completed["choices"][0]["message"]["content"].startswith("answer-")
                                    and completed["usage"]["prompt_tokens"] == 7
                                    and completed["usage"]["completion_tokens"] == 3,
                                    "Chat completion lost content, finish reason or usage")
                        if caller_stream and protocol == "chat":
                            require(body.count(b"[DONE]") == 1 and b'"finish_reason":"stop"' in body,
                                    "Chat stream did not complete exactly once")
                    if mode == "fallback":
                        require(direct["status"] == 200 and direct["outcome"] == "not-started"
                                and direct["refusal"]["facts"]["status"] == 200
                                and direct["refusal"]["facts"]["code"] == "1008"
                                and direct["refusal"]["facts"]["retryAfterMs"] == 7200000,
                                "SDK business refusal lost its original facts")
                        (case_output / "direct-refusal.json").write_text(json.dumps(direct, indent=2))
                    if mode == "exhausted":
                        require(b"quota" in body and "minimax-credit-1008" in (case_output / "host.log").read_text(),
                                "host classification did not produce route 429 and bounded rule diagnostics")
                    if mode == "partial":
                        require(b"paid output" in body and b"[DONE]" not in body, "uncertain SSE became a successful completion")
                    require("Retry-After" not in headers or mode == "exhausted", "adapter synthesized a timing header")
                require(not proxy.failures and not proxy.denied and not proxy.connections,
                        "fixture failed or retained connections: " + repr(proxy.failures))
            finally:
                if host:
                    (case_output / "commands.json").write_text(json.dumps(host.commands, indent=2))
                if proxy:
                    (case_output / "fixture.json").write_text(json.dumps(proxy.snapshot(), indent=2))
                    (case_output / "fixture-state.json").write_text(json.dumps(
                        {"failures": proxy.failures, "denied": proxy.denied, "openConnections": len(proxy.connections)}, indent=2))
    finally:
        (output / "RESULTS.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", type=Path, required=True)
    parser.add_argument("--worker", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--protocol", choices=["messages", "chat"], default="messages")
    parser.add_argument("--success-format", choices=["negotiated", "whole-json"], default="negotiated")
    parser.add_argument("--caller-stream", action="store_true")
    parser.add_argument("--modes", nargs="+", default=["fallback", "prose", "exhausted", "permission", "content", "malformed", "partial", "media"])
    arguments = parser.parse_args()
    run(arguments.host, arguments.worker, arguments.output, arguments.modes, arguments.protocol,
        arguments.success_format, arguments.caller_stream)
