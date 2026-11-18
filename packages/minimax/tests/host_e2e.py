"""Ordinary-host MiniMax integration with an unchanged production distribution.

Run only on isolated usbox under the external network guard. The CONNECT
fixture terminates TLS locally; it never forwards a connection to a vendor.
No native wrapper, endpoint override, other plugin or Bun is used.
"""

import argparse
import base64
from contextlib import ExitStack
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import signal
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import parse_qs, quote, urlsplit


MODULE = "minimax"
CN = "minimax-code"
GLOBAL = "minimax-code-global"
CALLER = "synthetic-minimax-caller"
TOKENS = {CN: "synthetic-minimax-cn-access", GLOBAL: "synthetic-minimax-global-access"}
HOSTS = {
    "account.minimax.cn": CN, "account.minimax.io": GLOBAL,
    "agent.minimax.cn": CN, "agent.minimaxi.com": CN, "agent.minimax.io": GLOBAL,
    "www.minimaxi.com": CN, "platform.minimax.io": GLOBAL,
    "93.184.216.34": "media",
}
LIMIT = 16 << 20
PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j4x8AAAAASUVORK5CYII=")
VIDEO = b"\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isomsynthetic-video-bytes"


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def copy_window_assets(distribution, package=None, sdk=None):
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    if not manifest.get("window"):
        return
    package = Path(package) if package is not None else Path(__file__).resolve().parents[1]
    selected_sdk = sdk or os.environ.get("MAGPIE_SDK_SOURCE")
    require(selected_sdk, "window fixture requires an explicit public SDK source")
    shutil.copytree(package / "web", distribution / "web")
    shutil.copy2(package / "web/package.json", distribution / "package.json")
    for name in ("ui.js", "account-selection.js", "errors.js"):
        shutil.copy2(Path(selected_sdk) / "sdk/js" / name,
                     distribution / "web" / ("sdk-ui.js" if name == "ui.js" else name))


def gateway_provider(provider):
    # Host ExtensionProviderID namespaces module/provider pairs in its catalog.
    digest = hashlib.sha256((MODULE + "\0" + provider).encode()).hexdigest()
    return "extension-" + digest[:24]


def wait_for(predicate, message, seconds=10):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(0.05)
    raise AssertionError(message)


def certificate(directory, env):
    config = directory / "openssl.cnf"
    config.write_text("[req]\ndistinguished_name=dn\nprompt=no\n[dn]\nCN=MiniMax synthetic fixture\n")
    ca, key = directory / "ca.pem", directory / "ca.key"
    server, server_key = directory / "server.pem", directory / "server.key"
    extensions = directory / "server.ext"
    names = ["IP:" + name if name[0].isdigit() else "DNS:" + name for name in HOSTS]
    extensions.write_text("basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\n"
                          "extendedKeyUsage=serverAuth\nsubjectAltName=" + ",".join(names) + "\n")
    commands = [
        ["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
         "-config", str(config), "-addext", "basicConstraints=critical,CA:TRUE",
         "-keyout", str(key), "-out", str(ca)],
        ["openssl", "req", "-new", "-newkey", "rsa:2048", "-nodes", "-config", str(config),
         "-keyout", str(server_key), "-out", str(directory / "server.csr")],
        ["openssl", "x509", "-req", "-in", str(directory / "server.csr"), "-CA", str(ca),
         "-CAkey", str(key), "-CAcreateserial", "-days", "1", "-extfile", str(extensions), "-out", str(server)],
    ]
    with (directory / "openssl.log").open("w") as log:
        for command in commands:
            subprocess.run(command, env=env, cwd=directory, stdout=log, stderr=subprocess.STDOUT,
                           check=True, timeout=15)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.minimum_version = ssl.TLSVersion.TLSv1_2
    context.load_cert_chain(server, server_key)
    return ca, context


class Fixture(ThreadingHTTPServer):
    daemon_threads = False
    block_on_close = True

    def __init__(self, context):
        super().__init__(("127.0.0.1", 0), ConnectHandler)
        self.context = context
        self.lock = threading.Lock()
        self.connections = set()
        self.records = []
        self.denied = []
        self.local_probes = []
        self.failures = []
        self.stop = threading.Event()
        self.started = threading.Event()
        self.cancelled = threading.Event()
        self.polls = {CN: 0, GLOBAL: 0}
        self.challenges = {}

    def record(self, value):
        with self.lock:
            self.records.append(value)

    def snapshot(self):
        with self.lock:
            return list(self.records)

    def handle_error(self, request, client_address):
        if not self.stop.is_set():
            with self.lock:
                self.failures.append("unhandled fixture connection error")

    def start(self, stack):
        thread = threading.Thread(target=self.serve_forever, name="minimax-connect")
        thread.start()

        def close():
            self.stop.set()
            self.shutdown()
            with self.lock:
                connections = list(self.connections)
            for connection in connections:
                try:
                    connection.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
                connection.close()
            self.server_close()
            thread.join(5)
            require(not thread.is_alive(), "CONNECT fixture did not join")

        stack.callback(close)
        return self


class ConnectHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def do_CONNECT(self):
        host, separator, port = self.path.rpartition(":")
        if not separator or port != "443" or host not in HOSTS:
            with self.server.lock:
                self.server.denied.append(self.path)
            self.send_error(502, "non-fixture destination denied")
            self.close_connection = True
            return
        self.send_response(200, "Connection established")
        self.end_headers()
        self.wfile.flush()
        self.connection.settimeout(15)
        secured = self.server.context.wrap_socket(self.connection, server_side=True)
        with self.server.lock:
            self.server.connections.add(secured)
        try:
            EndpointHandler(secured, self.client_address, self.server, host)
        finally:
            with self.server.lock:
                self.server.connections.discard(secured)
            secured.close()
            self.close_connection = True

    def do_GET(self):
        local_authority = "127.0.0.1:" + str(self.server.server_port)
        if (self.command == "GET" and self.path == "/" and self.headers.get("Host") == local_authority
                and not self.headers.get("Authorization") and not self.headers.get("Cookie")):
            # A request to this listener itself is not a forwarded vendor request.
            # Keep it visible and answer locally without opening any connection.
            with self.server.lock:
                self.server.local_probes.append({"method": self.command, "path": self.path,
                    "host": local_authority, "agent": self.headers.get("User-Agent"), "peer": self.client_address})
            self.send_error(404, "local fixture has no root resource")
            self.close_connection = True
            return
        with self.server.lock:
            self.server.denied.append({"method": self.command, "path": self.path,
                "host": self.headers.get("Host"), "agent": self.headers.get("User-Agent"),
                "peer": self.client_address})
        self.send_error(502, "fixture requires an allowlisted TLS tunnel")
        self.close_connection = True

    do_POST = do_GET


class EndpointHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def __init__(self, connection, address, server, authority):
        self.authority = authority
        super().__init__(connection, address, server)

    def log_message(self, *_):
        pass

    def reply(self, value, status=200, content_type="application/json"):
        body = value if isinstance(value, bytes) else json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)
        self.wfile.flush()
        self.close_connection = True

    def handle_request(self):
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 <= size <= LIMIT and not self.headers.get("Transfer-Encoding"), "unbounded fixture request")
            body = self.rfile.read(size)
            parsed = urlsplit(self.path)
            if body and "application/json" in self.headers.get("Content-Type", ""):
                value = json.loads(body)
            else:
                value = {key: values[0] for key, values in parse_qs(body.decode()).items()}
            region = HOSTS[self.authority]
            headers = {key.lower(): val for key, val in self.headers.items()}
            require(CALLER not in json.dumps(headers) + body.decode(errors="replace"),
                    "gateway caller credential escaped to the vendor")
            if region == "media":
                require(not headers.get("authorization") and not headers.get("cookie"),
                        "account credential escaped to media transfer")
            elif not self.authority.startswith("account."):
                require(headers.get("authorization") == "Bearer " + TOKENS[region],
                        "request used another region's account")
            self.server.record({"host": self.authority, "path": parsed.path, "method": self.command,
                                "region": region, "body": value, "headers": headers})
            self.dispatch(parsed.path, value, region)
        except (BrokenPipeError, ConnectionResetError):
            if not self.server.stop.is_set():
                self.server.failures.append("unexpected fixture disconnect")
        except Exception as error:
            self.server.failures.append(str(error))
            self.reply({"error": "fixture contract failed"}, 500)

    do_GET = handle_request
    do_POST = handle_request
    do_PUT = handle_request

    def dispatch(self, path, value, region):
        if region == "media" and path == "/fixture.png" and self.command == "GET":
            self.reply(PNG, content_type="image/png")
            return
        if region == "media" and path == "/fixture.mp4" and self.command == "GET":
            self.reply(VIDEO, content_type="video/mp4")
            return
        if path == "/oauth2/device/code":
            require(self.authority.startswith("account.") and value.get("client_id") == "mcode-public"
                    and value.get("scope") == "agent.default" and value.get("audience") == "agent-backend"
                    and value.get("code_challenge_method") == "S256" and value.get("code_challenge"),
                    "device authorization lost its donor PKCE request")
            self.server.challenges[region] = value["code_challenge"]
            self.reply({"device_code": "device-" + region, "user_code": "SYNTHETIC-" + region,
                        "verification_uri": "https://" + self.authority + "/device", "expires_in": 300,
                        "interval": 0.01})
            return
        if path == "/oauth2/token":
            require(value.get("grant_type") == "urn:ietf:params:oauth:grant-type:device_code"
                    and value.get("device_code") == "device-" + region and value.get("client_id") == "mcode-public",
                    "device exchange crossed a region or changed its grant")
            challenge = base64.urlsafe_b64encode(hashlib.sha256(value.get("code_verifier", "").encode()).digest()).decode().rstrip("=")
            require(challenge == self.server.challenges[region], "device verifier does not match its original challenge")
            self.server.polls[region] += 1
            if self.server.polls[region] == 1:
                self.reply({"error": "authorization_pending"}, 400)
            else:
                self.reply({"access_token": TOKENS[region], "refresh_token": "synthetic-refresh-" + region,
                            "token_type": "Bearer", "expires_in": 3600})
            return
        if path == "/v1/api/user/info":
            require(self.headers.get("Yy") and self.headers.get("X-Signature"), "account API request is unsigned")
            self.reply({"statusInfo": {"code": 0}, "data": {"userInfo": {
                "realUserID": "user-" + region, "userEmail": region + "@example.invalid", "name": "Synthetic"}}})
            return
        if path == "/mavis/api/v1/models":
            self.reply({"providers": [{"providerId": "minimax", "config": {"model_order": ["MiniMax-E2E"],
                "models": {"MiniMax-E2E": {"name": "Synthetic MiniMax", "limit": {"context": 32768, "output": 4096},
                    "reasoning": False, "thinking_config": {"mode": "forced_off"},
                    "modalities": {"input": ["text", "image"]}}}}}]})
            return
        if path == "/mavis/api/v1/llm/v1/messages":
            self.chat(value, region)
            return
        if path == "/matrix/api/v1/user/get_user_extra_info":
            self.reply({"workspaces": [{"workspace_type": 0, "workspace_id": "workspace-" + region,
                                        "has_token_plan": False, "opcredit_balance": "1000"}]})
            return
        if path == "/matrix/api/v1/commerce/get_membership_info":
            self.reply({"has_token_plan": False, "opcredit_balance": "1000"})
            return
        if path == "/minimax-cloud/api/v1/credit/details":
            self.reply({"details": [], "total_count": 0})
            return
        if path == "/mavis/api/v1/mcp/web_search":
            require(value.get("query") == "synthetic search", "search did not preserve its query")
            self.reply({"organic": [{"title": "Synthetic result", "url": "https://example.invalid/result",
                                     "snippet": "synthetic search answer"}]})
            return
        if path == "/minimax-cloud/api/v1/connectors/connected-tools/list":
            properties = {"generate_image": {"requests": {"type": "array", "items": {"type": "object",
                "properties": {"prompt": {"type": "string"}}, "required": ["prompt"]}}},
                "get_asset_url": {"node_id": {"type": "string"}},
                "submit_video_generation": {"model": {"type": "string"}, "prompt": {"type": "string"}},
                "query_video_generation": {"model": {"type": "string"}, "task_id": {"type": "string"}}}
            self.reply({"tools": [{"provider": "matrix", "provider_tool_name": "mcp_" + name,
                "runtime_tool_name": "connector__matrix__" + name,
                "input_schema_json": json.dumps({"type": "object", "properties": fields,
                    "required": list(fields)})} for name, fields in properties.items()],
                "partial": False, "provider_failures": []})
            return
        if path == "/minimax-cloud/api/v1/connectors/tools/call":
            require(value.get("provider") == "matrix", "connector provider changed")
            arguments = json.loads(value["arguments_json"])
            name = value["runtime_tool_name"]
            require(value["provider_tool_name"] == "mcp_" + name.removeprefix("connector__matrix__"),
                    "connector dispatch ignored its selected catalog route")
            if name == "connector__matrix__generate_image":
                require(arguments.get("requests") and arguments["requests"][0].get("prompt"),
                        "paid image call has no prompt")
                if arguments["requests"][0]["prompt"] == "mm-e2e-business":
                    self.reply({"base_resp": {"status_code": 2013,
                                "status_msg": "synthetic business refusal " + TOKENS[region]}})
                    return
                self.reply({"is_error": False, "result_json": json.dumps({"success_items": [{"node_id": "node-fixture"}]})})
            elif name == "connector__matrix__get_asset_url":
                require(arguments.get("node_id") in ("node-fixture", "video-node"), "asset lookup changed generated identity")
                suffix = "mp4" if arguments["node_id"] == "video-node" else "png"
                self.reply({"is_error": False, "result_json": json.dumps({"download_url": "https://93.184.216.34/fixture." + suffix})})
            elif name == "connector__matrix__submit_video_generation":
                require(arguments.get("model") == "MiniMax-H3" and arguments.get("prompt") == "synthetic video",
                        "video submission lost its selected model or prompt")
                self.reply({"is_error": False, "result_json": json.dumps({"task_id": "fixture-video"})})
            elif name == "connector__matrix__query_video_generation":
                require(arguments == {"model": "MiniMax-H3", "task_id": "fixture-video"},
                        "video content lookup lost its job identity")
                self.reply({"is_error": False, "result_json": json.dumps({"status": "succeeded", "node_id": "video-node"})})
            else:
                raise AssertionError("unexpected matrix route: " + name)
            return
        raise AssertionError("unmapped frozen API fixture: " + self.authority + path)

    def event(self, name, value):
        self.wfile.write(("event: " + name + "\ndata: " + json.dumps(value, separators=(",", ":")) + "\n\n").encode())
        self.wfile.flush()

    def chat(self, request, region):
        require(request.get("model") == "MiniMax-E2E" and self.headers.get("X-Api-Key") == "sk-xxx"
                and self.headers.get("X-Mavis-Agent-Id") == "main", "selected chat wire or model changed")
        text = json.dumps(request.get("messages", []))
        fractions = {"mm-control-fraction-credit": 1008.5, "mm-control-fraction-auth": 1000048.5,
                     "mm-control-string-fraction": "1008.5"}
        for marker, code in fractions.items():
            if marker in text:
                self.reply({"base_resp": {"status_code": code, "status_msg": "synthetic malformed code"}})
                return
        if "mm-control-malformed-json" in text:
            self.reply(b'{"message":"insufficient credit"', 403)
            return
        text = text.replace("mm-control-", "mm-e2e-")
        if "mm-e2e-permission" in text:
            self.reply({"type": "error", "error": {"type": "permission_error",
                        "message": "synthetic refusal " + TOKENS[region]}}, 403)
            return
        if "mm-e2e-quota" in text:
            self.reply({"type": "error", "error": {"type": "insufficient_quota", "code": 1008,
                        "message": "account credit exhausted " + TOKENS[region]}}, 403)
            return
        if "mm-e2e-chat-business" in text:
            code = 1008 if "mm-e2e-chat-business-credit" in text else 2013
            self.reply({"base_resp": {"status_code": code,
                        "status_msg": "synthetic business refusal " + TOKENS[region]}})
            return
        tool = "mm-e2e-tool" in text
        content = ([{"type": "tool_use", "id": "fixture-tool", "name": "fixture_echo", "input": {"value": "hello"}}]
                   if tool else [{"type": "text", "text": "answer-" + region}])
        message = {"id": "msg-synthetic-" + region, "type": "message", "role": "assistant",
                   "model": "MiniMax-E2E", "content": content, "stop_reason": "tool_use" if tool else "end_turn",
                   "stop_sequence": None, "usage": {"input_tokens": 7, "output_tokens": 3}}
        if not request.get("stream"):
            self.reply(message)
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        self.close_connection = True
        self.event("message_start", {"type": "message_start", "message": {
            **message, "content": [], "stop_reason": None, "usage": {"input_tokens": 7, "output_tokens": 0}}})
        self.event("content_block_start", {"type": "content_block_start", "index": 0,
            "content_block": {"type": "tool_use", "id": "fixture-tool", "name": "fixture_echo", "input": {}}
            if tool else {"type": "text", "text": ""}})
        if "mm-e2e-partial" in text:
            self.event("content_block_delta", {"type": "content_block_delta", "index": 0,
                                               "delta": {"type": "text_delta", "text": "partial-output"}})
            # Clean transport EOF deliberately lacks a successful message_stop.
            return
        if "mm-e2e-cancel" in text:
            self.event("content_block_delta", {"type": "content_block_delta", "index": 0,
                                               "delta": {"type": "text_delta", "text": "held"}})
            self.server.started.set()
            deadline = time.monotonic() + 12
            try:
                while not self.server.stop.wait(0.05) and time.monotonic() < deadline:
                    self.wfile.write(b": fixture-heartbeat\n\n")
                    self.wfile.flush()
            except (OSError, ssl.SSLError):
                self.server.cancelled.set()
                return
            if not self.server.stop.is_set():
                raise AssertionError("upstream stream did not stop after caller disconnect")
            return
        fragments = ['{"value":', '"hello"}'] if tool else ["answer-", region]
        for fragment in fragments:
            self.event("content_block_delta", {"type": "content_block_delta", "index": 0,
                "delta": {"type": "input_json_delta", "partial_json": fragment}
                if tool else {"type": "text_delta", "text": fragment}})
        self.event("content_block_stop", {"type": "content_block_stop", "index": 0})
        self.event("message_delta", {"type": "message_delta", "delta": {"stop_reason": message["stop_reason"]},
                                     "usage": {"output_tokens": 3}})
        self.event("message_stop", {"type": "message_stop"})


class Host:
    def __init__(self, executable, distribution, home, output, proxy, ca):
        self.executable, self.distribution = executable, distribution
        self.home, self.output = home, output
        self.config = home / ".config/magpie"
        self.config.mkdir(parents=True)
        (home / ".cache/magpie").mkdir(parents=True)
        self.process = None
        self.log = None
        self.commands = []
        self.env = dict(HOME=str(home), USERPROFILE=str(home), PATH="/usr/bin:/bin", SHELL="/bin/sh",
                        LANG="C.UTF-8", GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off", GOENV="off",
                        SSL_CERT_FILE=str(ca), SSL_CERT_DIR=str(home / "no-system-ca-directory"),
                        MAGPIE_TEST_SANDBOX=str(home), MAGPIE_NO_STATS="1", MAGPIE_NO_ACCOUNTS="0",
                        MAGPIE_PLUGIN_CATALOG="off", MAGPIE_PLUGIN_MARKET="off", MAGPIE_PARTNERS="off",
                        MAGPIE_BUN=str(home / "no-bun"), MAGPIE_UPDATE_FEED="http://127.0.0.1:1/disabled",
                        DO_NOT_TRACK="1", GSETTINGS_BACKEND="memory")
        for key, relative in {"XDG_CONFIG_HOME": ".config", "XDG_CACHE_HOME": ".cache",
                              "XDG_DATA_HOME": ".local/share", "XDG_STATE_HOME": ".local/state",
                              "XDG_CONFIG_DIRS": "config-dirs", "XDG_DATA_DIRS": "data-dirs",
                              "APPDATA": "appdata", "LOCALAPPDATA": "localappdata", "TMPDIR": "tmp"}.items():
            directory = home / relative
            directory.mkdir(parents=True, exist_ok=True)
            self.env[key] = str(directory)
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            self.port = listener.getsockname()[1]
        self.env["MAGPIE_ADDR"] = "127.0.0.1:" + str(self.port)
        settings = {"lan": True, "proxy": proxy, "noStats": True, "noUsageStats": True,
                    "noAccountDiscovery": False, "noAutoUpdate": True, "agentTelemetryOff": True}
        (self.config / "settings.json").write_text(json.dumps(settings))
        for name in ("models.json", "labs.json"):
            (home / ".cache/magpie" / name).write_text("{}")
        require(not (executable.parent / "data").exists() and not (executable.parent / ".portable").exists(),
                "host executable has a portable configuration marker")

    def command(self, *arguments, failure=False):
        process = subprocess.run([str(self.executable), *arguments], env=self.env, cwd=self.home,
                                 capture_output=True, text=True, timeout=40)
        self.commands.append({"args": list(arguments), "exit": process.returncode,
                              "stdout": process.stdout, "stderr": process.stderr})
        require((process.returncode != 0) == failure,
                "unexpected CLI outcome: " + repr(arguments[:5]) + " " + process.stderr[-1200:])
        return process.stdout

    def sdk(self, method, params, provider, account, failure=False, session=None):
        scope = {"provider": provider, "account": account}
        if session:
            scope["session"] = session
        result = self.command("plugin", "sdk", "call", MODULE, method, json.dumps(params), "--scope",
                              json.dumps(scope), failure=failure)
        if failure:
            return None
        rows = [json.loads(line) for line in result.splitlines()]
        require(rows and rows[-1].get("kind") == "result", "SDK CLI omitted its terminal result")
        return rows[-1]["result"]

    def install(self):
        self.command("plugin", "sdk", "add", str(self.distribution))
        installed = json.loads(self.command("plugin", "sdk", "list", "--json"))
        require(len(installed) == 1 and installed[0]["manifest"]["id"] == MODULE
                and not installed[0].get("disabled"), "expected one enabled MiniMax package")
        require(installed[0]["manifest"]["runtime"] == "native", "installed package is not native")
        legacy = self.config / "plugins.json"
        require(not legacy.exists() or not json.loads(legacy.read_text()).get("plugins"), "legacy plugin installed")
        require(not (self.config / "plugins/node_modules").exists(), "runtime dependency directory was created")
        self.command("gateway-key", "add", "MiniMax synthetic caller", "--key", CALLER)
        return installed[0]["manifest"]

    def start(self):
        self.log = (self.output / "host.log").open("w")
        self.process = subprocess.Popen([str(self.executable), "serve"], cwd=self.home, env=self.env,
                                        stdout=self.log, stderr=subprocess.STDOUT, start_new_session=True)

        def ready():
            require(self.process.poll() is None, "ordinary host exited during startup")
            try:
                status, _, _ = self.request("/", method="GET", key=None)
                state = json.loads((self.config / "gateway.json").read_text())
                return status == 200 and state.get("pid") == self.process.pid
            except (OSError, ValueError, http.client.HTTPException):
                return False

        wait_for(ready, "ordinary gateway did not start", 25)

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
        ledger = self.config / "usage.jsonl"
        if ledger.exists():
            (self.output / "usage.jsonl").write_bytes(ledger.read_bytes())

    def connect(self, path, value=None, *, method="POST", key=CALLER, session=None):
        connection = http.client.HTTPConnection("127.0.0.1", self.port, timeout=20)
        headers = {"Content-Type": "application/json"}
        if key is not None:
            headers["Authorization"] = "Bearer " + key
        if session:
            headers["X-Magpie-Session"] = session
        try:
            connection.request(method, path, body=None if value is None else json.dumps(value).encode(), headers=headers)
            transport = connection.sock
            return connection, connection.getresponse(), transport
        except BaseException:
            connection.close()
            raise

    def request(self, path, value=None, **kwargs):
        connection, response, _ = self.connect(path, value, **kwargs)
        try:
            body = response.read(LIMIT + 1)
            require(len(body) <= LIMIT, "unbounded host response")
            return response.status, dict(response.getheaders()), body
        finally:
            response.close()
            connection.close()


def run(host, distribution, output, classification_only=False):
    require(not output.exists(), "E2E output must be a fresh directory")
    output.mkdir(parents=True)
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    library = distribution / manifest["entry"]
    require(manifest["id"] == MODULE and library.is_file(), "expected complete production MiniMax distribution")
    result = {"status": "running", "hostSHA256": sha256(host), "librarySHA256": sha256(library),
              "manifestSHA256": sha256(distribution / "magpie-plugin.json"), "runnerSHA256": sha256(__file__),
              "productionPackageModified": False, "cases": [], "limits": "synthetic Linux host integration; no live vendor or platform readiness"}
    gateway = proxy = None
    try:
        with ExitStack() as stack:
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="mm-e2e-")))
            certs = output / "certificates"
            certs.mkdir()
            cert_env = {"HOME": str(home), "PATH": "/usr/bin:/bin", "LANG": "C.UTF-8"}
            ca, context = certificate(certs, cert_env)
            proxy = Fixture(context).start(stack)
            gateway = Host(host, distribution, home, output, "http://127.0.0.1:" + str(proxy.server_port), ca)
            stack.callback(gateway.close)
            result["installedManifest"] = gateway.install()
            exercise(gateway, proxy, result, classification_only)
            require(sha256(library) == result["librarySHA256"], "production library changed during E2E")
        require(not proxy.failures, "fixture assertions failed: " + repr(proxy.failures))
        require(not proxy.denied, "host attempted a non-fixture destination: " + repr(proxy.denied))
        require(not proxy.connections, "fixture connections survived shutdown")
        result["status"] = "pass"
    except BaseException as error:
        result.update(status="fail", error=repr(error))
        raise
    finally:
        if gateway:
            (output / "commands.json").write_text(json.dumps(gateway.commands, indent=2) + "\n")
        if proxy:
            (output / "fixture.json").write_text(json.dumps({"requests": proxy.snapshot(), "failures": proxy.failures,
                "denied": proxy.denied, "localProbes": proxy.local_probes}, indent=2) + "\n")
        (output / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps({"status": result["status"], "cases": len(result["cases"]), "output": str(output)}))


def classification_controls(gateway, proxy, result, accounts, malformed=False):
    cases = [("permission", 403), ("quota", 429), ("chat-business-unknown", 502),
             ("chat-business-credit", 429)]
    if malformed:
        cases = [("fraction-credit", None), ("fraction-auth", None),
                 ("string-fraction", None), ("malformed-json", None)]
    failures = []
    for suffix, expected in cases:
        marker = "mm-control-" + suffix
        request = {"model": "MiniMax-E2E", "max_tokens": 64,
                   "messages": [{"role": "user", "content": marker}]}
        payload = {"protocol": "messages", "model": "MiniMax-E2E",
                   "body": base64.b64encode(json.dumps(request).encode()).decode()}
        before = len(proxy.snapshot())
        gateway.sdk("provider.execute", payload, GLOBAL, accounts[GLOBAL], session=marker)
        events = [json.loads(line) for line in gateway.commands[-1]["stdout"].splitlines()]
        headers = [row["data"] for row in events if row.get("event") == "headers"]
        require(len(headers) == 1, "SDK control omitted its single response headers")
        status = headers[0]["status"]
        passed = status == expected if expected is not None else status >= 400 and status not in (401, 402, 429)
        record = {"case": marker, "moduleStatus": status, "expected": expected or "unknown refusal, not auth/quota",
                  "dispatches": len(proxy.snapshot()) - before, "passed": passed}
        result["cases"].append(record)
        require(record["dispatches"] == 1, "classification control was replayed")
        if not passed:
            failures.append(record)
    require(not failures, "malformed vendor facts acquired auth/quota authority: " + json.dumps(failures))


def exercise(gateway, proxy, result, classification_only=False):
    accounts = {}
    for provider in (CN, GLOBAL):
        start = json.loads(gateway.command("plugin", "sdk", "auth", "start", MODULE, provider,
                                          json.dumps({"method": "device"})))
        require(start.get("userCode") == "SYNTHETIC-" + provider and start.get("continuation"),
                "device start omitted its usable challenge")
        require("verifier" not in json.dumps(start).lower() and "device_code" not in json.dumps(start),
                "private PKCE/device state escaped through the host")
        finish = json.dumps({"continuation": start["continuation"]})
        wrong = GLOBAL if provider == CN else CN
        before = len(proxy.snapshot())
        gateway.command("plugin", "sdk", "auth", "finish", MODULE, wrong, finish, failure=True)
        require(len(proxy.snapshot()) == before, "foreign-provider continuation made a vendor request")
        time.sleep(max(start.get("pollAfterMs", 0) / 1000, 0.02))
        pending = json.loads(gateway.command("plugin", "sdk", "auth", "finish", MODULE, provider, finish))
        require(pending.get("pending") is True, "pending device authorization became an account")
        time.sleep(max(start.get("pollAfterMs", 0) / 1000, 0.02))
        saved = json.loads(gateway.command("plugin", "sdk", "auth", "finish", MODULE, provider, finish))
        require(saved.get("id") and not saved.get("credentials") and not saved.get("pending"),
                "device authorization did not save the account safely")
        accounts[provider] = saved["id"]
        gateway.command("plugin", "sdk", "auth", "finish", MODULE, provider, finish, failure=True)
        require(proxy.polls[provider] == 2, "consumed device continuation was replayed")
        result["cases"].append({"case": "device-auth", "provider": provider, "account": saved["id"],
                                "pending": True, "wrongProviderRefused": True, "replayRefused": True})
    require(accounts[CN] != accounts[GLOBAL], "regional account identities collided")
    if classification_only:
        classification_controls(gateway, proxy, result, accounts, malformed=True)
        return
    for provider, account in accounts.items():
        models = gateway.sdk("provider.models", {}, provider, account)["models"]
        require(any(row["id"] == "MiniMax-E2E" for row in models), "selected-account catalog not published")
    search = gateway.sdk("module.call", {"action": "search." + CN,
        "input": {"source": CN, "query": "synthetic search", "count": 3}}, CN, accounts[CN])
    require(search.get("pages") == [{"title": "Synthetic result", "url": "https://example.invalid/result",
                                    "text": "synthetic search answer"}], "selected search result changed")
    generated = gateway.sdk("tool.call", {"id": "generate_image",
        "arguments": {"requests": [{"prompt": "synthetic image"}]}}, GLOBAL, accounts[GLOBAL])
    require(generated.get("content", {}).get("success_items") == [{"node_id": "node-fixture"}],
            "ordinary scoped tool did not return its selected connector result")
    before = len(proxy.snapshot())
    gateway.sdk("tool.call", {"id": "generate_image", "arguments": {"site": CN,
        "requests": [{"prompt": "must not dispatch"}]}}, GLOBAL, accounts[GLOBAL], failure=True)
    require(len(proxy.snapshot()) == before, "tool arguments redirected a selected account before rejection")
    result["cases"].append({"case": "selected-search-and-tool", "searchProvider": CN,
                            "toolProvider": GLOBAL, "foreignSiteRefusedBeforeNetwork": True})
    gateway.sdk("tool.call", {"id": "generate_image",
        "arguments": {"requests": [{"prompt": "mm-e2e-business"}]}}, GLOBAL, accounts[GLOBAL],
        failure=True, session="mm-e2e-business")
    business_calls = [row for row in proxy.snapshot() if row["path"].endswith("/connectors/tools/call")
                      and "mm-e2e-business" in row["body"].get("arguments_json", "")]
    require(len(business_calls) == 1, "accepted HTTP200 business refusal was replayed")
    last_command = gateway.commands[-1]
    require(TOKENS[GLOBAL] not in last_command["stdout"] + last_command["stderr"],
            "business refusal exposed selected credentials")
    result["cases"].append({"case": "tool-http200-business-refusal", "dispatches": len(business_calls),
                            "exit": last_command["exit"]})
    gateway.start()
    selected = {provider: gateway_provider(provider) + "/MiniMax-E2E" for provider in accounts}

    def discovered_catalog():
        status, _, body = gateway.request("/v1/models", method="GET")
        require(status == 200, "gateway model catalog unavailable")
        listed = {row["id"] for row in json.loads(body)["data"]}
        return body if set(selected.values()) <= listed else None

    # The ordinary host publishes live model discovery asynchronously at startup.
    body = wait_for(discovered_catalog, "ordinary catalog lost a selected regional provider")
    (gateway.output / "catalog.json").write_bytes(body)
    mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
    libraries = {line.split()[-1] for line in mappings.splitlines() if line.split()[-1].startswith(str(gateway.home))
                 and ".so" in line.split()[-1]}
    require(any(Path(path).is_file() and sha256(path) == result["librarySHA256"] for path in libraries),
            "installed production library is not mapped in the ordinary host")
    result["libraryMappedInHost"] = True
    listed = {row["id"] for row in json.loads(body)["data"]}
    require(all(model in listed for model in selected.values()), "ordinary catalog lost a selected regional provider")

    def chat(provider, marker, streaming=False, tools=False):
        request = {"model": selected[provider], "stream": streaming,
                   "messages": [{"role": "user", "content": marker}], "max_tokens": 64}
        if tools:
            request["tools"] = [{"type": "function", "function": {"name": "fixture_echo",
                "parameters": {"type": "object", "properties": {"value": {"type": "string"}}, "required": ["value"]}}}]
        return gateway.request("/v1/chat/completions", request, session=marker)

    status, _, body = chat(CN, "mm-e2e-buffered")
    value = json.loads(body)
    require(status == 200 and value["choices"][0]["message"]["content"] == "answer-" + CN,
            "buffered chat did not traverse the selected CN account")
    require(value["usage"]["prompt_tokens"] == 7 and value["usage"]["completion_tokens"] == 3,
            "buffered usage changed across Messages/Chat conversion")
    result["cases"].append({"case": "buffered-chat", "provider": CN, "usage": value["usage"]})

    for provider, marker, tools in ((GLOBAL, "mm-e2e-stream", False), (CN, "mm-e2e-tool", True)):
        status, headers, body = chat(provider, marker, True, tools)
        require(status == 200 and "text/event-stream" in headers.get("Content-Type", ""), "SSE response missing")
        payloads = [line[6:] for line in body.decode().splitlines() if line.startswith("data: ")]
        require(payloads and payloads[-1] == "[DONE]", "canonical SSE terminal missing")
        events = [json.loads(line) for line in payloads[:-1]]
        deltas = [choice["delta"] for event in events for choice in event.get("choices", [])]
        if tools:
            calls = [call for delta in deltas for call in delta.get("tool_calls", [])]
            require(any(call.get("id") == "fixture-tool" for call in calls)
                    and any(call.get("function", {}).get("name") == "fixture_echo" for call in calls)
                    and json.loads("".join(call.get("function", {}).get("arguments", "") for call in calls)) == {"value": "hello"},
                    "fragmented tool association/arguments changed")
        else:
            require("".join(delta.get("content", "") for delta in deltas) == "answer-" + provider,
                    "SSE chat crossed a regional account or lost text")
        result["cases"].append({"case": "tool-sse" if tools else "text-sse", "provider": provider})

    image_model = gateway_provider(GLOBAL) + "/MiniMax-Image"
    require(image_model in listed, "ordinary catalog omitted the declared image operation")
    status, _, body = gateway.request("/v1/images/generations", {
        "model": image_model, "prompt": "synthetic image", "response_format": "url", "n": 1},
        session="mm-e2e-image")
    generated = json.loads(body)
    require(status == 200 and generated.get("data") == [{"url": "https://93.184.216.34/fixture.png"}],
            "ordinary image operation lost its generated asset URL")
    result["cases"].append({"case": "image-generation", "provider": GLOBAL, "format": "url"})
    status, _, body = gateway.request("/v1/videos", {"model": gateway_provider(GLOBAL) + "/MiniMax-H3", "prompt": "synthetic video"},
                                      session="mm-e2e-video")
    created = json.loads(body)
    require(status == 200 and created.get("id") and created.get("status") == "queued",
            "ordinary video submission omitted its continuing job identity")
    status, headers, body = gateway.request("/v1/videos/" + quote(created["id"], safe="") + "/content", method="GET",
                                            session="mm-e2e-video-content")
    require(status == 200 and headers.get("Content-Type") == "video/mp4" and body == VIDEO,
            "ordinary video content did not preserve the actual TLS transfer bytes")
    media_requests = [row for row in proxy.snapshot() if row["region"] == "media"]
    require(len(media_requests) == 1 and media_requests[0]["path"] == "/fixture.mp4", "unexpected media transfer")
    result["cases"].append({"case": "video-create-content", "provider": GLOBAL,
                            "sha256": hashlib.sha256(VIDEO).hexdigest(), "credentialLeak": False})

    def dispatches(marker):
        return len([row for row in proxy.snapshot() if row["path"].endswith("/llm/v1/messages")
                    and marker in json.dumps(row["body"].get("messages", []))])

    failure_statuses = {}

    def refusal(provider, marker, expected=None):
        status, headers, body = chat(provider, marker)
        result["cases"].append({"case": marker, "status": status, "moduleExpectedStatus": expected,
                                "headers": headers, "dispatches": dispatches(marker),
                                "body": body.decode(errors="replace")})
        require(dispatches(marker) == 1 and TOKENS[provider].encode() not in body,
                "failed paid request replayed or exposed selected credentials: " + marker)
        require(status == 409 and headers.get("X-Should-Retry") == "false" and headers.get("X-Magpie-Operation"),
                "accepted refusal lost the host no-replay fence: " + marker)
        failure_statuses[marker] = status

    refusal(CN, "mm-e2e-permission", 403)
    status, _, body = chat(GLOBAL, "mm-e2e-partial", True)
    payloads = [line[6:] for line in body.decode().splitlines() if line.startswith("data: ")]
    events = [json.loads(line) for line in payloads if line != "[DONE]"]
    choices = [choice for event in events for choice in event.get("choices", [])]
    result["cases"].append({"case": "partial-output-eof", "status": status,
                            "dispatches": dispatches("mm-e2e-partial"), "body": body.decode()})
    require(status == 200 and dispatches("mm-e2e-partial") == 1
            and "partial-output" in "".join(choice.get("delta", {}).get("content", "") for choice in choices),
            "partial stream output was lost or the accepted request was replayed")
    require(not any(choice.get("finish_reason") in ("stop", "tool_calls", "length") for choice in choices),
            "incomplete EOF acquired a successful completion reason")

    connection, response, transport = gateway.connect("/v1/chat/completions", {
        "model": selected[GLOBAL], "stream": True, "messages": [{"role": "user", "content": "mm-e2e-cancel"}]},
        session="mm-e2e-cancel")
    try:
        require(response.status == 200 and proxy.started.wait(5), "held stream never started")
        transport.shutdown(socket.SHUT_RDWR)
    finally:
        response.close()
        connection.close()
    require(proxy.cancelled.wait(8), "caller disconnect did not cancel the actual TLS upstream")
    result["cases"].append({"case": "caller-cancellation", "upstreamClosed": True})

    # Genuine account quota may affect future selection; exercise it last.
    refusal(CN, "mm-e2e-quota", 429)
    refusal(GLOBAL, "mm-e2e-chat-business-unknown", 502)
    refusal(GLOBAL, "mm-e2e-chat-business-credit", 429)

    trace_status, _, trace_body = gateway.request("/v1/magpie/trace", method="GET")
    require(trace_status == 200, "ordinary host trace unavailable")
    (gateway.output / "trace.json").write_bytes(trace_body)
    classification_controls(gateway, proxy, result, accounts)

    required_sessions = {"mm-e2e-buffered", "mm-e2e-stream", "mm-e2e-tool", "mm-e2e-partial",
                         "mm-e2e-business", *failure_statuses}

    def ledger():
        path = gateway.config / "usage.jsonl"
        rows = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        own = [row for row in rows if row.get("session", "").startswith("mm-e2e-")]
        return own if required_sessions <= {row.get("session") for row in own} else None

    rows = wait_for(ledger, "ordinary request/usage ledger did not settle")
    for marker, provider in (("mm-e2e-buffered", CN), ("mm-e2e-stream", GLOBAL), ("mm-e2e-tool", CN)):
        matched = [row for row in rows if row.get("session") == marker]
        require(len(matched) == 1 and matched[0]["status"] == 200 and matched[0]["provider"] == gateway_provider(provider)
                and matched[0]["in"] == 7 and matched[0]["out"] == 3 and matched[0].get("callerKeyId"),
                "host request/usage ledger lost selected owner, exactly-once row or token totals: " + marker)
    for marker, status in failure_statuses.items():
        matched = [row for row in rows if row.get("session") == marker]
        require(len(matched) == 1 and matched[0]["status"] == status and matched[0].get("callerKeyId"),
                "failed request accounting is missing, duplicated or reports success: " + marker)
    partial = [row for row in rows if row.get("session") == "mm-e2e-partial"]
    require(len(partial) == 1 and partial[0].get("err") and partial[0]["in"] == 7,
            "partial output lacks one error receipt with observed input usage")
    business = [row for row in rows if row.get("session") == "mm-e2e-business"]
    require(len(business) == 1 and business[0]["status"] == 502 and business[0]["provider"] == gateway_provider(GLOBAL)
            and business[0].get("kind") == "tool",
            "accepted HTTP200 business refusal lacks exactly one selected-account tool failure receipt")
    result["ledger"] = rows
    (gateway.output / "ledger.json").write_text(json.dumps(rows, indent=2) + "\n")
    log = (gateway.output / "host.log").read_text()
    require(all(secret not in log for secret in TOKENS.values()), "account credential escaped into ordinary host log")
    result["cases"].append({"case": "request-usage-logging", "rows": len(rows), "credentialLeak": False})


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True, type=Path)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--classification-only", action="store_true",
                        help="probe malformed business envelopes through the ordinary host SDK CLI")
    arguments = parser.parse_args()
    require(sys.platform == "linux", "run only on isolated regular usbox under the external network guard")

    def stop_run(signum, _frame):
        # Unwind ExitStack so the ordinary host and CONNECT workers stop/join.
        signal.signal(signum, signal.SIG_IGN)
        raise TimeoutError("E2E interrupted by signal " + str(signum))

    signal.signal(signal.SIGTERM, stop_run)
    signal.signal(signal.SIGINT, stop_run)
    signal.signal(signal.SIGALRM, stop_run)
    signal.alarm(150)
    try:
        run(arguments.host.resolve(), arguments.distribution.resolve(), arguments.output.resolve(), arguments.classification_only)
    finally:
        signal.alarm(0)
