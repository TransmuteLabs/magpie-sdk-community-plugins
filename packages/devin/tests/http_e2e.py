"""Real Magpie Front/native ingress checks against a synthetic HTTP provider."""

import argparse
import base64
from contextlib import ExitStack
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import select
import signal
import socket
import subprocess
import tempfile
import threading
import time

from conformance import b64, connect_frames, data_frame, fields, number_field, text_field


HOST_COMMIT = "01bcc7c7d155b029769327f909372b2ff72252e2"
SERVICE = "/exa.api_server_pb.ApiServerService/"
MODEL = "fixture/visible"
UPSTREAM_KEY = "synthetic-upstream-key"
CALLER_KEY = "synthetic-caller-key"
READONLY_KEY = "synthetic-readonly-key"
NATIVE_SECRET = "synthetic-native-secret"
IMAGE = base64.b64encode(b"synthetic-inline-image").decode()
LIMIT = 16 << 20


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def prompt(source, content, *extra):
    return number_field(2, source) + text_field(3, content) + b"".join(extra)


def chat(content, model=MODEL, prompts=None, extra=b""):
    metadata = text_field(3, NATIVE_SECRET)
    messages = prompts if prompts is not None else [prompt(1, content)]
    return data_frame(text_field(1, metadata) + text_field(21, model)
                      + b"".join(text_field(3, item) for item in messages) + extra)


def terminal(body, code=None):
    frames = connect_frames(body)
    require(frames and frames[-1][0] == 2, "native HTTP reply lacks a terminal Connect frame")
    require(sum(flag == 2 for flag, _ in frames) == 1, "native HTTP reply has multiple terminal frames")
    require(all(flag == 0 for flag, _ in frames[:-1]), "native HTTP reply has unsupported frame flags")
    end = json.loads(frames[-1][1])
    if code is None:
        require("error" not in end, "native HTTP success ended with an error: " + str(end))
    else:
        require(end.get("error", {}).get("code") == code,
                "native HTTP terminal code differs: " + str(end))
    return [fields(value) for flag, value in frames if flag == 0]


def emitted_text(messages):
    return b"".join(value for message in messages for value in message.get(3, []))


class Provider(ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False

    def __init__(self):
        super().__init__(("127.0.0.1", 0), ProviderHandler)
        self.lock = threading.Lock()
        self.requests = []
        self.probes = []
        self.failures = []
        self.cancelled = {"deadline": threading.Event(), "disconnect": threading.Event()}
        self.blocked = {"deadline": threading.Event(), "disconnect": threading.Event()}
        self.stop = threading.Event()

    def snapshot(self):
        with self.lock:
            return list(self.requests)


class ProviderHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def fail(self, message):
        with self.server.lock:
            self.server.failures.append(message)
        self.send_error(500, message)

    def do_GET(self):
        if self.path != "/":
            self.fail("unexpected provider GET: " + self.path)
            return
        header_values = json.dumps(dict(self.headers.items()))
        if any(secret in header_values for secret in (CALLER_KEY, READONLY_KEY, NATIVE_SECRET)):
            self.fail("caller credential escaped in a provider root probe")
            return
        with self.server.lock:
            self.server.probes.append({"path": self.path, "user_agent": self.headers.get("User-Agent", "")})
        self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_CONNECT(self):
        self.fail("unexpected external connection: " + self.path)

    def send_event(self, value):
        data = value if isinstance(value, str) else json.dumps(value, separators=(",", ":"))
        self.wfile.write(("data: " + data + "\n\n").encode())
        self.wfile.flush()

    def chunk(self, delta, finish=None):
        return {"id": "synthetic-response", "object": "chat.completion.chunk", "model": "visible",
                "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        if self.path != "/v1/chat/completions" or length <= 0 or length > LIMIT:
            self.fail("unexpected provider request shape")
            return
        body = self.rfile.read(length)
        request = json.loads(body)
        record = {"path": self.path, "body": request, "headers": dict(self.headers.items())}
        with self.server.lock:
            self.server.requests.append(record)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        self.close_connection = True
        text = json.dumps(request.get("messages", []))
        mode = next((name for name in ("deadline", "disconnect") if "e2e-" + name in text), None)
        try:
            if mode:
                self.send_event(self.chunk({"role": "assistant", "content": "started"}))
                self.server.blocked[mode].set()
                end = time.monotonic() + 12
                while not self.server.stop.is_set() and time.monotonic() < end:
                    if select.select([self.connection], [], [], 0.1)[0]:
                        if self.connection.recv(1, socket.MSG_PEEK) == b"":
                            self.server.cancelled[mode].set()
                            return
                with self.server.lock:
                    self.server.failures.append("provider stream did not observe cancellation: " + mode)
                return
            if "e2e-tools" in text:
                self.send_event(self.chunk({"role": "assistant", "tool_calls": [{
                    "index": 0, "id": "tool-native-1", "type": "function",
                    "function": {"name": "read_file", "arguments": '{"path":'},
                }]}))
                self.send_event(self.chunk({"tool_calls": [{
                    "index": 0, "function": {"arguments": '"file.txt"}'},
                }]}))
                self.send_event(self.chunk({}, "tool_calls"))
            else:
                self.send_event(self.chunk({"role": "assistant", "content": "http-"}))
                self.send_event(self.chunk({"content": "roundtrip"}))
                self.send_event(self.chunk({}, "stop"))
            self.send_event({"id": "synthetic-response", "choices": [],
                             "usage": {"prompt_tokens": 7, "completion_tokens": 4}})
            self.send_event("[DONE]")
        except (BrokenPipeError, ConnectionResetError):
            if mode:
                self.server.cancelled[mode].set()
            else:
                with self.server.lock:
                    self.server.failures.append("unexpected normal response disconnect")


class DenyProxy(ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False

    def __init__(self):
        super().__init__(("127.0.0.1", 0), DenyHandler)
        self.requests = []
        self.probes = []
        self.lock = threading.Lock()


class DenyHandler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def deny(self):
        with self.server.lock:
            self.server.requests.append(self.path)
        self.send_error(502, "fixture forbids external traffic")

    def do_GET(self):
        if self.path == "/":
            with self.server.lock:
                self.server.probes.append({"path": self.path, "user_agent": self.headers.get("User-Agent", "")})
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        self.deny()

    do_POST = deny
    do_CONNECT = deny


def server_thread(stack, server):
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    def stop():
        if isinstance(server, Provider):
            server.stop.set()
        server.shutdown()
        server.server_close()
        thread.join(5)
        require(not thread.is_alive(), "fixture server thread remained running")

    stack.callback(stop)
    return server


class Gateway:
    def __init__(self, magpie, distribution, home, artifacts, proxy):
        self.magpie = magpie
        self.distribution = distribution
        self.home = home
        self.artifacts = artifacts
        self.config = home / ".config/magpie"
        self.cache = home / ".cache/magpie"
        self.process = None
        self.log = None
        self.version = ""
        self.listener = None
        self.config.mkdir(parents=True)
        self.cache.mkdir(parents=True)
        temporary = home / "tmp"
        temporary.mkdir()
        self.env = {name: os.environ[name] for name in ("LANG", "LC_ALL", "TZ") if name in os.environ}
        self.env.update(
            HOME=str(home), USERPROFILE=str(home), SHELL="/bin/sh", PATH="/usr/bin:/bin",
            XDG_CONFIG_HOME=str(home / ".config"), XDG_CACHE_HOME=str(home / ".cache"),
            XDG_DATA_HOME=str(home / ".local/share"), XDG_STATE_HOME=str(home / ".local/state"),
            TMPDIR=str(temporary),
            MAGPIE_TEST_SANDBOX=str(home), MAGPIE_NO_ACCOUNTS="1", MAGPIE_NO_STATS="1",
            MAGPIE_PLUGIN_CATALOG="off", MAGPIE_PLUGIN_MARKET="off", MAGPIE_PARTNERS="off",
            DO_NOT_TRACK="1",
            MAGPIE_BUN=str(home / "bun-does-not-exist"), GOTOOLCHAIN="local",
            GOPROXY="off", GOSUMDB="off", HTTP_PROXY=proxy, HTTPS_PROXY=proxy, ALL_PROXY=proxy,
            http_proxy=proxy, https_proxy=proxy, all_proxy=proxy,
            NO_PROXY="127.0.0.1,localhost", no_proxy="127.0.0.1,localhost",
        )
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 0))
            self.port = probe.getsockname()[1]
        self.env["MAGPIE_ADDR"] = "127.0.0.1:" + str(self.port)
        settings = {"lan": True, "noStats": True, "noUsageStats": True,
                    "noAccountDiscovery": True, "noAutoUpdate": True, "agentTelemetryOff": True}
        (self.config / "settings.json").write_text(json.dumps(settings))
        catalog = {"fixture": {"id": "fixture", "name": "Synthetic", "models": {
            "visible": {"id": "visible", "name": "Visible", "attachment": True,
                        "modalities": {"input": ["text", "image"], "output": ["text"]},
                        "limit": {"context": 32768, "output": 4096},
                        "cost": {"input": 0, "output": 0}},
            "hidden": {"id": "hidden", "name": "Hidden",
                       "limit": {"context": 32768, "output": 4096},
                       "cost": {"input": 0, "output": 0}},
        }}}
        (self.cache / "models.json").write_text(json.dumps(catalog))
        labs = {"fixture/visible": catalog["fixture"]["models"]["visible"]}
        (self.cache / "labs.json").write_text(json.dumps(labs))
        require(not (magpie.parent / "data").exists() and not (magpie.parent / ".portable").exists(),
                "test host path has a portable-data marker")

    def command(self, *arguments):
        completed = subprocess.run([str(self.magpie), *arguments], env=self.env,
                                   cwd=self.home, capture_output=True, text=True, timeout=45)
        require(completed.returncode == 0,
                "sandbox CLI failed: " + repr(arguments[:3]) + ": " + completed.stderr[-3000:])
        return completed.stdout

    def configure(self, upstream):
        self.version = self.command("--version").strip()
        require("01bcc7c7d" in self.version, "fixture requires the frozen merged host build")
        self.command("provider", "add", "Fixture", "id=fixture",
                     "url=" + upstream + "/v1", "key=" + UPSTREAM_KEY,
                     "models=visible,hidden", "list=off")
        self.command("model", "spec", MODEL, "--context", "32768", "--output", "4096", "--vision", "on")
        self.command("plugin", "sdk", "add", str(self.distribution))
        self.command("gateway-key", "add", "Fixture caller", "--key", CALLER_KEY)
        self.command("gateway-key", "add", "Fixture readonly", "--key", READONLY_KEY, "--read-only")
        keys = json.loads((self.config / "caller-keys.json").read_text())
        key = next(row["id"] for row in keys if row["name"] == "Fixture caller")
        self.command("gateway-key", "models", key, MODEL)

    def start(self):
        self.log = (self.artifacts / "gateway.log").open("w")
        self.process = subprocess.Popen([str(self.magpie), "serve"], env=self.env,
                                        cwd=self.home, stdout=self.log, stderr=subprocess.STDOUT,
                                        start_new_session=True)
        end = time.monotonic() + 30
        while time.monotonic() < end:
            require(self.process.poll() is None, "sandbox gateway exited during startup")
            try:
                status, _, body = self.request("/", method="GET", key=None)
                listening = json.loads((self.config / "gateway.json").read_text())
                if status == 200 and json.loads(body).get("version") == self.version.removeprefix("magpie ") and \
                        listening.get("pid") == self.process.pid and listening.get("addr") == self.env["MAGPIE_ADDR"]:
                    self.listener = listening
                    return
            except (OSError, ValueError, TypeError, http.client.HTTPException):
                pass
            time.sleep(0.05)
        raise AssertionError("sandbox gateway did not become ready")

    def close(self):
        if self.process and self.process.poll() is None:
            os.killpg(self.process.pid, signal.SIGTERM)
            try:
                self.process.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(self.process.pid, signal.SIGKILL)
                self.process.wait(timeout=5)
        if self.log:
            self.log.close()

    def connect(self, path, body, *, key=CALLER_KEY, headers=None, method="POST", catalog=False):
        connection = http.client.HTTPConnection("127.0.0.1", self.port, timeout=10)
        request_headers = {"Content-Type": "application/proto" if catalog else "application/connect+proto",
                           "Connect-Protocol-Version": "1", "User-Agent": "magpie-devin-http-e2e"}
        if key is not None:
            request_headers["Authorization"] = "Bearer " + key
        request_headers.update(headers or {})
        try:
            connection.request(method, path, body=body, headers=request_headers)
            transport = connection.sock
            return connection, connection.getresponse(), transport
        except BaseException:
            connection.close()
            raise

    def request(self, path, body=None, **options):
        connection, response, _ = self.connect(path, body, **options)
        try:
            result = response.read(LIMIT + 1)
            require(len(result) <= LIMIT, "native HTTP response exceeded fixture bound")
            return response.status, dict(response.getheaders()), result
        finally:
            response.close()
            connection.close()


def assert_provider_credentials(records):
    require(records, "HTTP fixture did not reach the provider")
    for record in records:
        headers = {name.lower(): value for name, value in record["headers"].items()}
        require(headers.get("authorization") == "Bearer " + UPSTREAM_KEY,
                "provider did not receive its own configured credential")
        serialized = json.dumps(record)
        for secret in (NATIVE_SECRET, CALLER_KEY, READONLY_KEY):
            require(secret not in serialized, "native/caller credential escaped to the provider")
        require("x-native-secret" not in headers and "x-api-key" not in headers,
                "native credential headers escaped to the provider")
        require(record["path"] == "/v1/chat/completions" and record["body"]["model"] == "visible",
                "provider received the wrong canonical endpoint/model")


def check_fixture_root(server):
    connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=5)
    try:
        connection.request("GET", "/", headers={"User-Agent": "devin-e2e-root-contract"})
        response = connection.getresponse()
        require(response.status == 404 and response.read() == b"", "synthetic HTTP root did not answer 404")
    finally:
        connection.close()


def check_roundtrip(gateway, provider):
    checks = {}
    baseline = len(provider.snapshot())
    path = SERVICE + "GetChatMessage"
    status, _, _ = gateway.request(path, chat("guard"), key=None,
                                  headers={"X-Forwarded-For": "203.0.113.10"})
    require(status == 401 and len(provider.snapshot()) == baseline, "Front LAN/auth guard was bypassed")
    status, _, _ = gateway.request(path, chat("guard"), key=READONLY_KEY)
    require(status == 403 and len(provider.snapshot()) == baseline, "Front readonly guard was bypassed")
    checks["front_auth_and_readonly"] = True

    status, headers, body = gateway.request(path, chat("e2e-text"), headers={
        "X-Native-Secret": NATIVE_SECRET, "X-Api-Key": NATIVE_SECRET,
    })
    require(status == 200 and headers.get("Content-Type") == "application/connect+proto",
            "native chat did not return framed HTTP")
    messages = terminal(body)
    require(emitted_text(messages) == b"http-roundtrip", "provider text did not traverse the native HTTP path")
    require(any(2 in message.get(5, []) for message in messages), "native stop reason was lost")
    require(any(7 in message for message in messages), "provider usage did not reach native reply")
    checks["text_usage_and_terminal"] = True
    assert_provider_credentials(provider.snapshot())
    checks["credential_isolation"] = True

    tool = text_field(1, "read_file") + text_field(2, "Read a file") + text_field(3, '{"type":"object"}')
    status, _, body = gateway.request(path, chat("e2e-tools", extra=text_field(10, tool)))
    require(status == 200, "native tool request was refused")
    messages = terminal(body)
    calls = [fields(value) for message in messages for value in message.get(6, [])]
    require(len(calls) == 1 and calls[0].get(1) == [b"tool-native-1"] and calls[0].get(2) == [b"read_file"] and
            json.loads(calls[0][3][0]) == {"path": "file.txt"}, "fragmented HTTP tool call was changed")
    require(any(10 in message.get(5, []) for message in messages), "native tool-call stop reason was lost")
    checks["fragmented_tool_response"] = True

    call = text_field(1, "screen-1") + text_field(2, "screenshot") + text_field(3, "{}")
    image = text_field(1, IMAGE) + text_field(2, "image/png")
    history = [prompt(5, "fixture system"), prompt(1, "e2e-image"),
               prompt(2, "", text_field(6, call)),
               prompt(4, "screenshot", text_field(7, "screen-1"), text_field(10, image))]
    status, _, body = gateway.request(path, chat("", prompts=history))
    require(status == 200 and emitted_text(terminal(body)) == b"http-roundtrip", "native image conversation failed")
    received = provider.snapshot()[-1]["body"]["messages"]
    require(any(item.get("role") == "tool" and item.get("tool_call_id") == "screen-1" for item in received),
            "tool-result association was lost on the provider wire")
    require("data:image/png;base64," + IMAGE in json.dumps(received),
            "tool-result image was lost on the provider wire")
    require(any(item.get("role") == "system" and item.get("content") == "fixture system" for item in received),
            "native system prompt was lost on the provider wire")
    checks["system_and_tool_image"] = True

    methods = ("GetCliModelConfigs", "GetCommandModelConfigs", "GetCascadeModelConfigs",
               "GetCliModelConfigsForSite", "GetCommandModelConfigsForSite", "GetCascadeModelConfigsForSite")
    for method in methods:
        status, headers, body = gateway.request(SERVICE + method, b"", catalog=True)
        require(status == 200 and headers.get("Content-Type") == "application/proto",
                "native model discovery was not unary protobuf: " + method)
        rows = fields(body).get(1, [])
        names = [fields(row).get(22, [b""])[0].decode() for row in rows]
        require(names == [MODEL], "native model discovery escaped the caller's model allowlist: " + repr(names))
    status, _, body = gateway.request(SERVICE + "GetModelStatuses", b"", catalog=True)
    require(status == 200 and body == b"", "native statuses fabricated availability")
    checks["authorized_model_discovery"] = True
    count = len(provider.snapshot())
    status, _, body = gateway.request(path, chat("forbidden", model="fixture/hidden"))
    require(status == 200, "decoded model rejection lost Connect streaming status")
    terminal(body, "permission_denied")
    require(len(provider.snapshot()) == count, "forbidden native model reached provider")
    checks["model_authorization"] = True

    status, _, body = gateway.request(path, b"\x00")
    require(status == 200, "malformed native chat lost Connect streaming status")
    terminal(body, "invalid_argument")
    status, headers, body = gateway.request(SERVICE + "GetCliModelConfigs", b"\x00", catalog=True)
    require(status == 400 and headers.get("Content-Type") == "application/json" and
            json.loads(body).get("code") == "invalid_argument", "malformed unary discovery lost HTTP 400")
    require(len(provider.snapshot()) == count, "malformed native input reached provider")
    checks["malformed_status_and_terminal"] = True

    status, _, body = gateway.request(path, chat("e2e-deadline"), headers={"Connect-Timeout-Ms": "1200"})
    require(status == 200 and emitted_text(terminal(body, "deadline_exceeded")) == b"started",
            "native deadline did not retain partial output and terminal status")
    require(provider.cancelled["deadline"].wait(5), "native deadline did not cancel the HTTP provider")
    checks["deadline_terminal_and_provider_cancellation"] = True

    connection, response, transport = gateway.connect(path, chat("e2e-disconnect"))
    try:
        require(response.status == 200, "disconnect fixture did not start a native response")
        head = response.read(5)
        require(len(head) == 5 and head[0] == 0, "disconnect fixture did not receive a native data frame")
        size = int.from_bytes(head[1:], "big")
        require(size <= LIMIT and len(response.read(size)) == size, "disconnect fixture received a truncated first frame")
        require(provider.blocked["disconnect"].wait(5), "disconnect fixture did not reach provider")
        transport.shutdown(socket.SHUT_RDWR)
    finally:
        response.close()
        connection.close()
    require(provider.cancelled["disconnect"].wait(5), "client disconnect did not cancel the HTTP provider")
    checks["client_disconnect_cancels_provider"] = True

    count = len(provider.snapshot())
    gateway.command("plugin", "sdk", "off", "devin-inbound")
    status, _, _ = gateway.request(path, chat("off"))
    require(status == 404 and len(provider.snapshot()) == count, "disabled native module remained callable")
    gateway.command("plugin", "sdk", "on", "devin-inbound")
    status, _, body = gateway.request(path, chat("on"))
    require(status == 200 and emitted_text(terminal(body)) == b"http-roundtrip", "reenabled native module did not recover")
    checks["off_denial_and_reenable"] = True
    assert_provider_credentials(provider.snapshot())
    require(not provider.failures, "synthetic provider failed: " + str(provider.failures))
    return checks


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--magpie", type=Path, required=True)
    parser.add_argument("--distribution", type=Path, required=True)
    parser.add_argument("--host-build", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    args = parser.parse_args()
    magpie, distribution = args.magpie.resolve(), args.distribution.resolve()
    provenance = json.loads(args.host_build.read_text())
    require(provenance["source_commit"] == HOST_COMMIT and provenance["exit"] == 0,
            "host provenance is not the frozen successful merge build")
    require(provenance["binary_sha256"] == digest(magpie), "host binary differs from its build provenance")
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    require(manifest["runtime"] == "native", "fixture requires a real native library")
    artifacts = args.artifacts.resolve()
    artifacts.mkdir(parents=True, exist_ok=True)
    result = {"host_source_commit": HOST_COMMIT, "host_source_tree": provenance["source_tree"],
              "host_sha256": digest(magpie), "library_sha256": digest(distribution / manifest["entry"]),
              "runner_sha256": digest(__file__), "checks": {}, "complete": False}
    try:
        with ExitStack() as stack:
            temporary = stack.enter_context(tempfile.TemporaryDirectory(prefix="magpie-devin-http-home-"))
            provider = server_thread(stack, Provider())
            proxy = server_thread(stack, DenyProxy())
            check_fixture_root(provider)
            check_fixture_root(proxy)
            gateway = Gateway(magpie, distribution, Path(temporary), artifacts,
                              "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)
            gateway.configure("http://127.0.0.1:" + str(provider.server_port))
            gateway.start()
            result["gateway_version"] = gateway.version
            result["gateway_pid"] = gateway.process.pid
            result["listener"] = gateway.listener
            result["bun_executable_exists"] = Path(gateway.env["MAGPIE_BUN"]).exists()
            result["checks"] = check_roundtrip(gateway, provider)
            result["provider_requests"] = len(provider.snapshot())
            result["provider_root_probes"] = list(provider.probes)
            result["blocked_external_requests"] = list(proxy.requests)
            result["proxy_root_probes"] = list(proxy.probes)
            gateway.close()
            require(not proxy.requests, "fixture attempted nonlocal background HTTP traffic")
        result["sandbox_removed"] = not Path(temporary).exists()
        result["gateway_exit"] = gateway.process.returncode
        result["complete"] = True
    finally:
        (artifacts / "http-e2e-result.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
