"""Ordinary compiled Magpie + native Devin executor, using local fixtures only."""

import argparse
from contextlib import ExitStack
import gzip
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import socket
import struct
import subprocess
import tempfile
import threading
import time

from conformance import fields, text_field, number_field, data_frame, connect_frames
from http_e2e import (
    Gateway, DenyProxy, Provider, server_thread, require, digest, chat, prompt,
    terminal, emitted_text, CALLER_KEY, READONLY_KEY, SERVICE, NATIVE_SECRET,
    IMAGE,
)


SELECTED_KEY = "synthetic-selected-devin-key"
MODEL = "fixture-model"


def invalid_native_streams():
    content = text_field(3, "rejected completion")
    complete = content + number_field(5, 2)
    signed = (text_field(9, "unearned thinking") + text_field(10, "unearned-signature")
              + text_field(21, "synthetic-native-format") + text_field(23, MODEL))
    return {
        "invalid-native-empty": ([], "data_loss"),
        "invalid-native-usage": ([text_field(7, number_field(2, 17)), number_field(5, 2)], "data_loss"),
        "invalid-native-incomplete": ([signed, content + number_field(5, 1)], "data_loss"),
        "invalid-native-error": ([content + number_field(5, 13)], "internal"),
        "invalid-native-unknown-stop": ([content + number_field(5, 14)], "unimplemented"),
        "invalid-native-changed-stop": ([complete, number_field(5, 3)], "data_loss"),
        "invalid-native-late-text": ([complete, content], "data_loss"),
        "invalid-native-unknown-control": ([complete + number_field(31, 1)], "unimplemented"),
        "invalid-native-unknown-usage": ([complete + text_field(7, number_field(14, 1))], "data_loss"),
        "invalid-native-provider-refusal": ([complete + text_field(7, number_field(12, 1))], "unimplemented"),
    }


class Vendor(ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False

    def __init__(self):
        super().__init__(("127.0.0.1", 0), VendorHandler)
        self.lock = threading.Lock()
        self.chat = []
        self.calls = []
        self.failures = []
        self.probes = []
        self.stop = threading.Event()
        self.cancelled = {"deadline": threading.Event(), "disconnect": threading.Event()}

    def count(self):
        with self.lock:
            return len(self.chat)


class VendorHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def do_GET(self):
        with self.server.lock:
            self.server.probes.append(self.path)
        self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def reply(self, content_type, body, status=200):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            require(0 < length <= 16 << 20, "invalid fixture request size")
            body = self.rfile.read(length)
            require(NATIVE_SECRET.encode() not in body and CALLER_KEY.encode() not in body,
                    "local caller credential crossed vendor boundary")
            with self.server.lock:
                self.server.calls.append(self.path)
            if self.path.endswith("/ExchangeDevinCLIPKCECode"):
                value = json.loads(body)
                require(set(value) == {"code", "code_verifier"} and value["code"] == "offline-code"
                        and 43 <= len(value["code_verifier"]) <= 128, "PKCE exchange shape changed")
                self.reply("application/json", json.dumps({"sessionToken": SELECTED_KEY}).encode())
                return
            if self.path.endswith("/GetUserStatus"):
                value = json.loads(body)
                require(value["metadata"]["apiKey"] == SELECTED_KEY, "status used wrong account")
                self.reply("application/json", json.dumps({
                    "userStatus": {"email": "offline@example.invalid",
                                   "planStatus": {"dailyQuotaRemainingPercent": 75}},
                }).encode())
                return
            if self.path.endswith("/GetCliModelConfigs"):
                require(SELECTED_KEY.encode() in body, "models used wrong account")
                features = number_field(12, 1) + number_field(15, 1)
                info = number_field(4, 32768) + number_field(13, 4096) + text_field(6, features)
                model = text_field(1, "Offline Devin") + text_field(22, MODEL)
                model += number_field(5, 1) + number_field(18, 32768) + text_field(23, info)
                free = (text_field(1, "Offline free") + text_field(22, "fixture-free")
                        + number_field(18, 32768) + number_field(24, 4) + text_field(23, info))
                self.reply("application/proto", text_field(1, model) + text_field(1, free))
                return
            require(self.path == SERVICE + "GetChatMessage", "unknown native endpoint")
            require(self.headers.get("Authorization") == "Basic " + SELECTED_KEY,
                    "native request used wrong account header")
            framed = connect_frames(body)
            require(len(framed) == 1 and framed[0][0] in (0, 1), "invalid native request framing")
            payload = gzip.decompress(framed[0][1]) if framed[0][0] == 1 else framed[0][1]
            value = fields(payload)
            require(value.get(21) == [MODEL.encode()], "selected model was not applied")
            require(SELECTED_KEY.encode() in value[1][0], "selected metadata missing")
            with self.server.lock:
                self.server.chat.append(payload)
            messages = [fields(item) for item in value.get(3, [])]
            text = b"".join(message.get(3, [b""])[0] for message in messages)
            for marker, (parts, _) in invalid_native_streams().items():
                if marker.encode() in text:
                    self.reply("application/connect+proto",
                               b"".join(data_frame(part) for part in parts) + b"\2\0\0\0\2{}")
                    return
            if b"native-tools-reply" in text:
                first = text_field(6, text_field(1, "native id") + text_field(2, "read_file")
                                   + text_field(3, '{"path":'))
                second = text_field(6, text_field(1, "native id") + text_field(3, '"a"}'))
                second += number_field(5, 10) + text_field(7, number_field(2, 17) + number_field(3, 0))
                self.reply("application/connect+proto",
                           data_frame(first) + data_frame(second) + b"\2\0\0\0\2{}")
                return
            if b"native-tools-history" in text:
                require(any(fields(call).get(1) == [b"native id"]
                            for message in messages for call in message.get(6, [])),
                        "native historical tool call ID changed")
                require(any(message.get(2) == [4] and message.get(7) == [b"native id"]
                            for message in messages), "native tool result association changed")
            if b"native-tools-images" in text:
                require(10 in value and value[10][0] == TOOL, "native tool declaration was changed")
                require(any(message.get(10) == [INLINE_IMAGE] for message in messages),
                        "native inline image was changed")
                require(value.get(8) == [CONFIG], "native generation config was changed")
            for kind in ("deadline", "disconnect"):
                if ("native-" + kind).encode() in text:
                    self.send_response(200)
                    self.send_header("Content-Type", "application/connect+proto")
                    self.send_header("Connection", "close")
                    self.end_headers()
                    try:
                        self.wfile.write(data_frame(text_field(3, "started")))
                        self.wfile.flush()
                        while not self.server.stop.wait(0.05):
                            self.wfile.write(data_frame(text_field(3, ".")))
                            self.wfile.flush()
                    except (OSError, ConnectionError):
                        self.server.cancelled[kind].set()
                    finally:
                        self.close_connection = True
                    return
            if b"vendor-error" in text:
                payload = b'{"error":{"code":"resource_exhausted","message":"synthetic"}}'
                self.reply("application/connect+proto", b"\2" + struct.pack(">I", len(payload)) + payload)
                return
            if b"native-signed" in text:
                first = text_field(9, "private thinking") + text_field(10, "signed-")
                first += text_field(16, "thinking-id") + text_field(23, MODEL)
                second = text_field(10, "fixture") + text_field(16, "thinking-id")
                second += text_field(21, "synthetic-native-format")
                second += text_field(20, b"\0opaque-gemini")
                payloads = [first, second]
            else:
                payloads = []
            reply = text_field(3, "native outbound answer") + number_field(5, 2)
            reply += text_field(7, number_field(2, 17) + number_field(3, 0))
            payloads.append(reply)
            framed_reply = b"".join(data_frame(item) for item in payloads)
            framed_reply += b"\2\0\0\0\2{}"
            self.reply("application/connect+proto", framed_reply)
        except BaseException as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.reply("application/json", b'{"code":"internal"}', 500)


class PairedGateway(Gateway):
    def configure_paired(self, vendor, foreign):
        self.env["MAGPIE_NO_ACCOUNTS"] = "0"
        settings = json.loads((self.config / "settings.json").read_text())
        settings["noAccountDiscovery"] = False
        (self.config / "settings.json").write_text(json.dumps(settings))
        self.version = self.command("--version").strip()
        self.command("plugin", "sdk", "add", str(self.distribution))
        installed = json.loads(self.command("plugin", "sdk", "list", "--json"))
        contract = next(row["manifest"]["contract"] for row in installed
                        if row["manifest"]["id"] == "devin-inbound")
        require(contract["version"]["major"] == 1 and contract["version"]["minor"] >= 4 and
                "outbound-executor" in contract["capabilities"],
                "paired fixture requires an installed SDK1.4+ outbound contract")
        config = json.loads(self.command("plugin", "sdk", "config", "devin-inbound"))
        self.command("plugin", "sdk", "config", "devin-inbound",
                     json.dumps({"server": vendor}), "--expected", config["version"])
        account = self.sign_in()
        self.browser_sign_in(account["id"])
        scope = json.dumps({"provider": "devin", "account": account["id"]})
        usage = json.loads(self.command("plugin", "sdk", "call", "devin-inbound", "provider.usage",
                                      "{}", "--scope", scope))["result"]
        require(usage.get("revision", "").isdigit() and len(usage.get("windows", [])) == 1
                and usage["windows"][0].get("notModels") == ["fixture-free"]
                and usage["windows"][0]["used"] == 25, "SDK quota exclusion/epoch changed")
        self.command("provider", "add", "Foreign fixture", "id=fixture",
                     "url=" + foreign + "/v1", "key=synthetic-upstream-key",
                     "models=visible,hidden", "list=off")
        self.command("gateway-key", "add", "Fixture caller", "--key", CALLER_KEY)
        self.command("gateway-key", "add", "Fixture readonly", "--key", READONLY_KEY, "--read-only")
        self.command("gateway-key", "add", "Other fixture caller", "--key", "other-fixture-caller")

    def sign_in(self, account=None):
        scope = ("--scope", json.dumps({"account": account})) if account else ()
        start = json.loads(self.command(
            "plugin", "sdk", "auth", "start", "devin-inbound", "devin",
            json.dumps({"method": "api-key", "input": {"key": SELECTED_KEY}}), *scope))
        finish = json.dumps({"continuation": start["continuation"]})
        result = json.loads(self.command("plugin", "sdk", "auth", "finish",
                                        "devin-inbound", "devin", finish, *scope))
        require(result.get("id") and not result.get("credentials"), "saved auth leaked credential")
        return result

    def denied_command(self, *arguments):
        result = subprocess.run([str(self.magpie), *arguments], env=self.env,
                                cwd=self.home, capture_output=True, text=True, timeout=45)
        require(result.returncode != 0, "command unexpectedly succeeded: " + repr(arguments[:5]))

    def browser_sign_in(self, account):
        from urllib.parse import urlparse, parse_qs, quote
        scope = json.dumps({"account": account})
        start = json.loads(self.command(
            "plugin", "sdk", "auth", "start", "devin-inbound", "devin",
            json.dumps({"method": "browser", "redirectUri": "http://127.0.0.1:34567/callback"}),
            "--scope", scope))
        query = parse_qs(urlparse(start["url"]).query)
        require(query["code_challenge_method"] == ["S256"], "browser PKCE challenge missing")
        finish = {"continuation": start["continuation"], "input": {
            "callbackURL": "http://127.0.0.1:34567/callback?state=foreign&code=offline-code"}}
        self.denied_command("plugin", "sdk", "auth", "finish", "devin-inbound", "devin", json.dumps(finish),
                            "--scope", scope)
        finish["input"]["callbackURL"] = (
            "http://127.0.0.1:34567/callback?state=" + quote(query["state"][0]) + "&code=offline-code")
        result = json.loads(self.command(
            "plugin", "sdk", "auth", "finish", "devin-inbound", "devin", json.dumps(finish),
            "--scope", scope))
        require(result.get("id") and not result.get("credentials"), "browser auth did not save account safely")
        self.denied_command("plugin", "sdk", "auth", "finish", "devin-inbound", "devin", json.dumps(finish),
                            "--scope", scope)


INLINE_IMAGE = text_field(1, IMAGE) + text_field(2, "image/png")
TOOL = (text_field(1, "read_file") + text_field(2, "Read file")
        + text_field(3, '{"type":"object"}') + number_field(7, 1))
CONFIG = number_field(1, 1) + number_field(2, 1024) + b"\x29" + struct.pack("<d", 0.0)


def catalog_policies(gateway, vendor, model, checks):
    methods = ("GetCliModelConfigs", "GetCommandModelConfigs", "GetCascadeModelConfigs",
               "GetCliModelConfigsForSite", "GetCommandModelConfigsForSite", "GetCascadeModelConfigsForSite")
    key = "synthetic-native-catalog-policy"
    gateway.command("gateway-key", "add", "Native catalog policy", "--key", key)
    keys = json.loads((gateway.config / "caller-keys.json").read_text())
    caller = next(row["id"] for row in keys if row["name"] == "Native catalog policy")
    gateway.command("gateway-key", "models", caller, model)
    gateway.command("gateway-key", "aliases", caller, "assistant=" + model)
    gateway.command("gateway-key", "limit", caller, "day", "--tokens", "1", "--model", model)
    before = vendor.count()
    for method in methods:
        status, headers, body = gateway.request(SERVICE + method, b"", catalog=True, key=key)
        require(status == 200 and headers.get("Content-Type") == "application/proto",
                "native catalog projection failed: " + method)
        names = [fields(row).get(22, [b""])[0].decode() for row in fields(body).get(1, [])]
        require(names == ["assistant"], "native catalog escaped key model/alias policy: " + repr(names))
    status, _, body = gateway.request(SERVICE + "GetModelStatuses", b"", catalog=True, key=key)
    require(status == 200 and body == b"", "native statuses fabricated model availability")
    require(vendor.count() == before, "native catalog dispatched inference")
    checks.append("all six installed native catalogs preserve per-key model restrictions and aliases")
    for method in (*methods, "GetModelStatuses"):
        status, _, _ = gateway.request(SERVICE + method, b"", catalog=True, key=READONLY_KEY)
        require(status == 403, "native catalog bypassed read-only route policy: " + method)
    status, _, _ = gateway.request(SERVICE + "GetChatMessage", chat("readonly-catalog", "assistant"),
                                   key=READONLY_KEY)
    require(status == 403 and vendor.count() == before, "read-only key dispatched native inference")
    checks.append("read-only keys cannot use native catalog, status or inference routes")
    status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("restricted", "fixture/visible"), key=key)
    terminal(body, "permission_denied")
    require(status == 200 and vendor.count() == before, "model-restricted native request dispatched")
    status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("native-catalog-alias", "assistant"), key=key)
    require(status == 200 and emitted_text(terminal(body)) == b"native outbound answer"
            and vendor.count() == before + 1, "discovered native alias did not reach its selected model")
    status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("native-catalog-budget", "assistant"), key=key)
    terminal(body, "resource_exhausted")
    require(status == 200 and vendor.count() == before + 1, "native alias bypassed its scoped token budget")
    checks.append("discovered native alias executes once and cannot bypass model restriction or scoped budget")
    request = text_field(1, text_field(3, "synthetic-catalog-metadata"))
    connection = http.client.HTTPConnection("127.0.0.1", gateway.port, timeout=5)
    try:
        connection.putrequest("POST", SERVICE + "GetCliModelConfigs")
        for name, value in {"Content-Type": "application/proto", "Connect-Protocol-Version": "1",
                            "Authorization": "Bearer " + key, "Connect-Timeout-Ms": "1",
                            "Expect": "100-continue", "Content-Length": str(len(request))}.items():
            connection.putheader(name, value)
        connection.endheaders()
        interim = connection.sock.makefile("rb")
        try:
            require(interim.readline() == b"HTTP/1.1 100 Continue\r\n" and interim.readline() == b"\r\n",
                    "catalog deadline fixture never entered bounded request-body reading")
        finally:
            interim.close()
        time.sleep(0.05)
        connection.send(request)
        response = connection.getresponse()
        body = response.read()
        require(response.status == 504 and json.loads(body).get("code") == "deadline_exceeded",
                "expired native catalog lost its unary deadline error: " + repr(body))
    finally:
        connection.close()
    status, _, body = gateway.request(SERVICE + "GetCliModelConfigs", b"", catalog=True)
    require(status == 200 and model.encode() in body and vendor.count() == before + 1,
            "catalog deadline damaged subsequent discovery or dispatched inference")
    checks.append("expired native unary catalog deadline refuses before canonical dispatch and later discovery works")


def run(magpie, distribution, artifacts, generation_mode="live"):
    artifacts.mkdir(parents=True, exist_ok=True)
    checks = []
    with ExitStack() as stack:
        vendor = server_thread(stack, Vendor())
        stack.callback(vendor.stop.set)
        foreign = server_thread(stack, Provider())
        proxy = server_thread(stack, DenyProxy())
        home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-paired-home-")))
        gateway = PairedGateway(magpie, distribution, home, artifacts,
                                "http://127.0.0.1:" + str(proxy.server_port))
        def observations():
            value = {"nativeRequests": vendor.count(), "vendorMethods": list(vendor.calls),
                     "vendorFailures": list(vendor.failures), "externalRequests": list(proxy.requests),
                     "hostSHA256": digest(magpie), "librarySHA256": digest(distribution / "plugin.so"),
                     "generationMode": generation_mode}
            (artifacts / "observations.json").write_text(json.dumps(value, indent=2) + "\n")
        stack.callback(observations)
        stack.callback(gateway.close)
        gateway.configure_paired("http://127.0.0.1:" + str(vendor.server_port),
                                 "http://127.0.0.1:" + str(foreign.server_port))
        gateway.start()
        model = None
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            status, _, body = gateway.request("/v1/models", method="GET", headers={}, key=CALLER_KEY)
            require(status == 200, "host model discovery failed")
            catalog = json.loads(body)["data"]
            model = next((row["id"] for row in catalog if row["id"].endswith("/" + MODEL)), None)
            if model:
                break
            time.sleep(0.1)
        require(model, "native SDK account model absent: " + json.dumps(catalog)[:2000]
                + " vendorMethods=" + repr(vendor.calls) + " failures=" + repr(vendor.failures))
        mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
        libraries = {line.split()[-1] for line in mappings.splitlines()
                     if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
        require(any(Path(path).is_file() and digest(path) == digest(distribution / "plugin.so")
                    for path in libraries), "native library was not mapped in the actual gateway process")
        checks.append("actual Devin shared library executes in the ordinary host process without Bun")
        checks.append("real SDK auth lifecycle CAS and selected-account model discovery")
        require(sum(path.endswith("/ExchangeDevinCLIPKCECode") for path in vendor.calls) == 1,
                "browser exchange was repeated")
        checks.append("persisted browser PKCE, callback-state rejection and one-use continuation")
        checks.append("actual SDK usage epoch and paid quota exclusion for native Free-tier model")
        status, _, body = gateway.request(SERVICE + "GetCliModelConfigs", b"", catalog=True)
        require(status == 200 and model.encode() in body, "native authorized catalog failed")
        status, _, _ = gateway.request(SERVICE + "GetCliModelConfigs", b"", catalog=True, key=None,
                                       headers={"X-Forwarded-For": "203.0.113.10"})
        require(status == 401, "native catalog bypassed key guard")
        checks.append("authorized native catalog and unauthenticated denial")
        keys = json.loads((gateway.config / "caller-keys.json").read_text())
        caller = next(row["id"] for row in keys if row["name"] == "Fixture caller")
        gateway.command("gateway-key", "models", caller, model)
        catalog_policies(gateway, vendor, model, checks)
        status, _, _ = gateway.request(SERVICE + "GetChatMessage", chat("hello", model), key=None,
                                       headers={"X-Forwarded-For": "203.0.113.10"})
        require(status == 401, "native execution bypassed authentication")
        status, _, _ = gateway.request(SERVICE + "GetChatMessage", chat("hello", model), key=READONLY_KEY)
        require(status == 403, "native execution bypassed readonly policy")
        checks.append("ordinary Front authentication and readonly policy")
        for content in ("hello", "native-signed"):
            status, _, body = gateway.request(SERVICE + "GetChatMessage", chat(content, model))
            require(status == 200, "paired HTTP status failed")
            messages = terminal(body)
            require(emitted_text(messages) == b"native outbound answer", "raw native reply changed")
            require(b"\\u2063" not in body and b"\xe2\x81\xa3" not in body,
                    "internal wakeup marker escaped native response")
            if content == "native-signed":
                require(b"".join(value for message in messages for value in message.get(10, []))
                        == b"signed-fixture", "signed native deltas changed")
                require(any(message.get(20) == [b"\0opaque-gemini"] for message in messages),
                        "opaque Gemini signature changed")
        checks.append("raw native streaming text, zero output usage and fragmented signed thinking")
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat(
            "", model, prompts=[prompt(1, "native-tools-images", text_field(10, INLINE_IMAGE))],
            extra=text_field(10, TOOL) + text_field(8, CONFIG)))
        require(status == 200 and emitted_text(terminal(body)) == b"native outbound answer",
                "native tools/images/control request failed")
        checks.append("native tool configuration, inline image and exact zero sampling preservation")
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat(
            "native-tools-reply", model, extra=text_field(10, TOOL)))
        messages = terminal(body)
        calls = [fields(call) for message in messages for call in message.get(6, [])]
        require(status == 200 and len(calls) == 2
                and b"".join(call.get(3, [b""])[0] for call in calls) == b'{"path":"a"}'
                and all(call.get(1) == [b"native id"] for call in calls)
                and any(message.get(5) == [10] for message in messages),
                "raw native tool deltas, IDs or stop changed")
        native_call = (text_field(1, "native id") + text_field(2, "read_file")
                       + text_field(3, '{"path":"a"}'))
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat(
            "", model, prompts=[prompt(2, "", text_field(6, native_call)),
                                prompt(4, "contents", text_field(7, "native id")),
                                prompt(1, "native-tools-history")], extra=text_field(10, TOOL)))
        require(status == 200 and emitted_text(terminal(body)) == b"native outbound answer",
                "native tool history did not execute")
        checks.append("raw native tool fragments and historical call/result IDs survive host translation")
        for marker, (_, code) in invalid_native_streams().items():
            before = vendor.count()
            status, _, body = gateway.request(SERVICE + "GetChatMessage", chat(marker, model))
            require(status == 200, "native failure lost its framed HTTP response")
            terminal(body, code)
            require(vendor.count() == before + 1, "invalid native completion was retried")
        checks.append("empty/incomplete/error/unknown/late native completions refuse success without replay")
        unearned = prompt(2, "rejected completion", text_field(11, "unearned thinking"),
                          text_field(12, "unearned-signature"),
                          text_field(18, "synthetic-native-format"), text_field(22, MODEL))
        before = vendor.count()
        status, _, body = gateway.request(SERVICE + "GetChatMessage",
                                         chat("", model, prompts=[unearned, prompt(1, "continue")]))
        terminal(body, "permission_denied")
        require(vendor.count() == before, "failed native completion minted a signed-history receipt")
        checks.append("failed native completion cannot authorize signed-history replay")
        for streaming in (False, True):
            request = {"model": model, "stream": streaming,
                       "messages": [{"role": "user", "content": "ordinary-canonical"}]}
            if streaming:
                request["stream_options"] = {"include_usage": True}
            canonical = json.dumps(request).encode()
            status, _, body = gateway.request("/v1/chat/completions", canonical,
                                             headers={"Content-Type": "application/json"})
            require(status == 200 and b"native outbound answer" in body,
                    "ordinary Chat did not use native Devin outbound executor: " + repr(body[:1000]))
            if streaming:
                require(b"[DONE]" in body, "ordinary Chat streaming terminal missing")
                chunks = [json.loads(line[6:]) for line in body.splitlines()
                          if line.startswith(b"data: ") and line[6:] != b"[DONE]"]
                require(any(chunk.get("usage", {}).get("completion_tokens") == 0 for chunk in chunks),
                        "ordinary SSE invented output usage")
            else:
                value = json.loads(body)
                require(value["choices"][0]["message"]["content"] == "native outbound answer",
                        "ordinary Chat JSON content changed")
                require(value.get("usage", {}).get("completion_tokens") == 0,
                        "ordinary JSON invented output usage")
        checks.append("ordinary Chat JSON and SSE use the same native outbound module")
        signed = prompt(2, "prior answer", text_field(11, "private thinking"),
                        text_field(12, "signed-fixture"),
                        text_field(17, b"\0opaque-gemini"),
                        text_field(18, "synthetic-native-format"), text_field(22, MODEL))
        replay = chat("", model, prompts=[signed, prompt(1, "continue")])
        before = vendor.count()
        status, _, body = gateway.request(SERVICE + "GetChatMessage", replay)
        require(status == 200 and emitted_text(terminal(body)) == b"native outbound answer"
                and vendor.count() == before + 1, "known signed history failed")
        checks.append("persisted same-caller/account/model signed-history receipt")
        before = vendor.count()
        status, _, body = gateway.request(SERVICE + "GetChatMessage", replay, key="other-fixture-caller")
        require(vendor.count() == before, "foreign caller signed history reached vendor")
        require(status >= 400 or "error" in json.loads(connect_frames(body)[-1][1]),
                "foreign caller receipt was accepted")
        forged = prompt(2, "prior", text_field(12, "unknown-signature"))
        status, _, body = gateway.request(SERVICE + "GetChatMessage",
                                         chat("", model, prompts=[forged, prompt(1, "continue")]))
        require(vendor.count() == before, "unknown signature reached vendor")
        checks.append("unknown and cross-caller signed history rejected before dispatch")
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("after-origin-denial", model))
        require(status == 200 and emitted_text(terminal(body)) == b"native outbound answer",
                "origin refusal incorrectly disabled the usable upstream account")
        accounts = json.loads(gateway.command("plugin", "sdk", "accounts", "devin-inbound", "devin", "--json"))
        require(len(accounts) == 1, "unexpected saved account count")
        gateway.sign_in(accounts[0]["id"])
        if generation_mode == "restart":
            gateway.close()
            (artifacts / "gateway.log").rename(artifacts / "gateway-before-generation.log")
            gateway.start()
        before = vendor.count()
        status, _, body = gateway.request(SERVICE + "GetChatMessage", replay)
        require(vendor.count() == before, "old-generation signed history reached vendor")
        require(status >= 400 or "error" in json.loads(connect_frames(body)[-1][1]),
                "old-generation signed history was accepted")
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("new-generation", model))
        require(status == 200 and emitted_text(terminal(body)) == b"native outbound answer",
                "new account generation could not make an unsigned request")
        checks.append(generation_mode + " reauthentication revokes old signature generation while unsigned execution continues")
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("vendor-error", model))
        terminal(body, "resource_exhausted")
        checks.append("native vendor error terminal preserved without retry")
        before = vendor.count()
        status, _, body = gateway.request(SERVICE + "GetChatMessage", b"\0")
        terminal(body, "invalid_argument")
        require(vendor.count() == before, "malformed native input dispatched")
        checks.append("malformed native input rejected before acceptance")
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("native-deadline", model),
                                         headers={"Connect-Timeout-Ms": "500"})
        require(status == 200 and emitted_text(terminal(body, "deadline_exceeded")).startswith(b"started"),
                "native deadline lost partial raw response or terminal")
        require(vendor.cancelled["deadline"].wait(5), "native deadline did not cancel vendor HTTP")
        connection, response, transport = gateway.connect(SERVICE + "GetChatMessage",
                                                         chat("native-disconnect", model))
        try:
            require(response.status == 200 and response.read(5)[0] == 0, "native disconnect did not start")
            transport.shutdown(socket.SHUT_RDWR)
        finally:
            response.close()
            connection.close()
        require(vendor.cancelled["disconnect"].wait(5), "client disconnect did not cancel vendor HTTP")
        checks.append("native deadline/terminal and TCP disconnect cancel the actual outbound request")
        before_foreign = len(foreign.snapshot())
        status, _, body = gateway.request(SERVICE + "GetChatMessage", chat("foreign", "fixture/visible"),
                                         key="other-fixture-caller")
        require(len(foreign.snapshot()) == before_foreign, "same-module route reached a foreign executor")
        require(status >= 400 or "error" in json.loads(connect_frames(body)[-1][1]),
                "foreign executor route was accepted")
        checks.append("native-only same-module executor binding")
        gateway.command("plugin", "sdk", "off", "devin-inbound")
        before = vendor.count()
        status, _, _ = gateway.request(SERVICE + "GetChatMessage", chat("off", model))
        require(status >= 400 and vendor.count() == before, "disabled module executed")
        checks.append("module off denies ingress and execution")
        require(not vendor.failures, "synthetic vendor assertion failed: " + repr(vendor.failures))
        require(not proxy.requests, "unexpected external traffic: " + repr(proxy.requests))
        require(not foreign.failures, "foreign fixture failed")
        result = {"checks": checks, "nativeRequests": vendor.count(),
                  "vendorMethods": list(vendor.calls), "externalRequests": len(proxy.requests),
                  "hostSHA256": digest(magpie), "librarySHA256": digest(distribution / "plugin.so"),
                  "runnerSHA256": digest(Path(__file__)), "version": gateway.version,
                  "generationMode": generation_mode, "libraryMappedInHost": True}
    require(not home.exists(), "sandbox HOME remained")
    require(gateway.process.poll() is not None, "sandbox gateway remained")
    result["homeRemoved"] = True
    result["processStopped"] = True
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("magpie", type=Path)
    parser.add_argument("distribution", type=Path)
    parser.add_argument("artifacts", type=Path)
    parser.add_argument("--generation-mode", choices=("live", "restart"), default="live")
    options = parser.parse_args()
    proof = run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve(),
                options.generation_mode)
    (options.artifacts / "result.json").write_text(json.dumps(proof, indent=2) + "\n")
    print(json.dumps(proof, indent=2))
