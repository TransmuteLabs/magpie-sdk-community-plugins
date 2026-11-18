#!/usr/bin/env python3
"""Exercise the unchanged native distribution through an ordinary Magpie host.

Run with an already built macOS host and matching native distribution:

    python3 -I packages/claude-desktop/tests/host_e2e.py \
        --host /path/to/magpie --distribution /path/to/claude-desktop \
        --output /path/to/new-evidence-directory

An inherited OS loopback-only network supervisor is mandatory. Set
MAGPIE_CLAUDE_DESKTOP_LOOPBACK_GUARD=sandbox-exec in that supervisor; the test
also requires real EPERM/EACCES results for numeric external TCP/UDP probes.
The supervisor must deny execution of installed Claude.app bundles and access
to the real user's configurations and Keychain. This script does not create
that supervisor or treat a Python socket monkeypatch as process isolation.

No compiler, package manager, Desktop executable, OAuth browser, credential
store or live vendor is used. The test launches only the provided Magpie binary
and local HTTP fixtures. It proves host integration, not live Desktop Chat,
Code/Cowork, managed-policy acceptance or execution on another architecture.
"""

import argparse
import base64
import errno
import hashlib
import http.client
import http.server
import json
import os
from pathlib import Path
import platform
import secrets
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import urlsplit


MODULE = "agent-claude-desktop"
AGENT = "claude-desktop-sdk"
TARGET = "claude-desktop"
PROFILE = "a363652b-0f53-4cd6-aede-cd0000000001"
OTHER_PROFILE = "3a6b0adf-158a-4491-9b7e-2c12bc479a40"
BUILTIN_PROFILE = "00000000-0000-4000-8000-6d6167706965"
VENDOR_MODEL = "claude-sonnet-4-6"
GENERIC_MODEL = "sdk-gpt-desktop-fixture-model"
UPSTREAM_KEY = "synthetic-desktop-upstream-key"
GATEWAY_KEY = "magpie-" + AGENT
CONTROL_KEY = "magpie-claude"
WEB_KEY = "synthetic-desktop-web-entry"
APPLICATION_SUPPORT = Path("Library/Application Support")
DESKTOP_PATHS = (
    APPLICATION_SUPPORT / "Claude/claude_desktop_config.json",
    APPLICATION_SUPPORT / "Claude-3p/claude_desktop_config.json",
    APPLICATION_SUPPORT / "Claude-3p/configLibrary/_meta.json",
    APPLICATION_SUPPORT / ("Claude-3p/configLibrary/" + PROFILE + ".json"),
)
RECEIPT_PATH = APPLICATION_SUPPORT / "Magpie SDK/agents/claude-desktop.json"
PROFILE_FIELDS = {
    "inferenceProvider", "inferenceCredentialKind", "inferenceGatewayBaseUrl",
    "inferenceGatewayApiKey", "inferenceGatewayAuthScheme",
    "disableDeploymentModeChooser", "desktopHome", "isClaudeCodeForDesktopEnabled",
}
MAX_BODY = 1 << 20


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def encoded(value):
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def redact(value):
    text = value if isinstance(value, str) else json.dumps(value)
    for secret in (UPSTREAM_KEY, GATEWAY_KEY, CONTROL_KEY, WEB_KEY):
        text = text.replace(secret, "<synthetic-credential>")
    return text


def wait_for(predicate, message, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = predicate()
        if result:
            return result
        time.sleep(0.05)
    raise AssertionError(message)


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def check_network_guard():
    marker = os.environ.get("MAGPIE_CLAUDE_DESKTOP_LOOPBACK_GUARD")
    require(sys.platform == "darwin", "this unchanged distribution declares macOS only")
    require(marker == "sandbox-exec", "an inherited macOS OS-level loopback supervisor is required")
    controls = []
    for family, kind, address in (
        (socket.AF_INET, socket.SOCK_STREAM, ("192.0.2.1", 443)),
        (socket.AF_INET, socket.SOCK_DGRAM, ("192.0.2.1", 443)),
        (socket.AF_INET6, socket.SOCK_STREAM, ("2001:db8::1", 443)),
    ):
        try:
            with socket.socket(family, kind) as probe:
                probe.settimeout(0.5)
                probe.connect(address)
        except OSError as failure:
            require(failure.errno in (errno.EPERM, errno.EACCES),
                    "network probe failed without proving OS policy denial: " + str(failure))
            controls.append({"family": family, "kind": kind, "errno": failure.errno})
        else:
            raise AssertionError("the network supervisor permitted an external operation")
    return {"supervisor": marker, "negativeControls": controls,
            "pythonSocketPatch": False, "inheritedByHostProcesses": True}


class Vendor(http.server.ThreadingHTTPServer):
    daemon_threads = False
    block_on_close = True

    def __init__(self, namespace):
        super().__init__(("127.0.0.1", 0), VendorHandler)
        self.namespace = namespace
        self.lock = threading.Lock()
        self.records = []
        self.failures = []

    def record(self, value):
        with self.lock:
            self.records.append(value)

    def snapshot(self):
        with self.lock:
            return list(self.records), list(self.failures)

    def handle_error(self, request, client_address):
        with self.lock:
            self.failures.append("unhandled local fixture request failure")


class VendorHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def setup(self):
        super().setup()
        self.connection.settimeout(5)

    def log_message(self, *_):
        pass

    def reply(self, value, status=200, content_type="application/json"):
        body = value if isinstance(value, bytes) else encoded(value)
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)
        self.wfile.flush()
        self.close_connection = True

    def do_GET(self):
        path = urlsplit(self.path).path
        if path == self.server.namespace + "/updates":
            self.reply({}, 404)
            return
        if path != self.server.namespace + "/v1/models":
            self.reply({}, 404)
            return
        self.server.record({"method": "GET", "path": "/v1/models"})
        self.reply({"data": [{"id": model, "type": "model", "display_name": model,
                              "created_at": "2026-11-17T00:00:00Z"}
                             for model in (VENDOR_MODEL, GENERIC_MODEL)], "has_more": False})

    def do_POST(self):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            require(0 < length <= MAX_BODY and not self.headers.get("Transfer-Encoding"),
                    "fixture received an unbounded request")
            body = self.rfile.read(length)
            require(len(body) == length, "fixture received a truncated request")
            payload = json.loads(body)
            path = urlsplit(self.path).path
            require(path in (self.server.namespace + "/v1/messages",
                             self.server.namespace + "/v1/messages/count_tokens"),
                    "unexpected fixture endpoint: " + path)
            credential = self.headers.get("x-api-key") or self.headers.get("Authorization", "").removeprefix("Bearer ")
            require(credential == UPSTREAM_KEY, "the ordinary provider used a different account credential")
            wire = json.dumps(dict(self.headers.items())) + body.decode()
            require(not self.headers.get("Cookie"), "owner UI cookie escaped to the upstream")
            require(GATEWAY_KEY not in wire and CONTROL_KEY not in wire and WEB_KEY not in wire,
                    "a gateway or owner credential escaped to the upstream")
            require(payload.get("model") in (VENDOR_MODEL, GENERIC_MODEL),
                    "Desktop alias was not resolved to the provider model")
            marker = payload.get("messages", [{}])[-1].get("content", "")
            require(isinstance(marker, str), "unexpected synthetic prompt shape")
            self.server.record({"method": "POST", "path": path[len(self.server.namespace):],
                                "model": payload["model"], "stream": bool(payload.get("stream")),
                                "marker": marker, "credentialVerified": True})
            if path.endswith("/count_tokens"):
                self.reply({"input_tokens": 37})
                return
            text = "synthetic-desktop:" + marker
            if not payload.get("stream"):
                self.reply({"id": "msg_desktop_fixture", "type": "message", "role": "assistant",
                            "model": payload["model"], "content": [{"type": "text", "text": text}],
                            "stop_reason": "end_turn", "stop_sequence": None,
                            "usage": {"input_tokens": 7, "output_tokens": 3}})
                return
            events = [
                ("message_start", {"type": "message_start", "message": {
                    "id": "msg_desktop_stream", "type": "message", "role": "assistant",
                    "model": payload["model"], "content": [], "stop_reason": None,
                    "stop_sequence": None, "usage": {"input_tokens": 7, "output_tokens": 0}}}),
                ("content_block_start", {"type": "content_block_start", "index": 0,
                                         "content_block": {"type": "text", "text": ""}}),
                ("content_block_delta", {"type": "content_block_delta", "index": 0,
                                         "delta": {"type": "text_delta", "text": text}}),
                ("content_block_stop", {"type": "content_block_stop", "index": 0}),
                ("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn",
                                                                          "stop_sequence": None},
                                   "usage": {"output_tokens": 3}}),
                ("message_stop", {"type": "message_stop"}),
            ]
            frames = b"".join(b"event: " + event.encode() + b"\ndata: " + encoded(value) + b"\n\n"
                              for event, value in events)
            self.reply(frames, content_type="text/event-stream")
        except (AssertionError, ValueError, KeyError, TypeError) as failure:
            with self.server.lock:
                self.server.failures.append(redact(str(failure)))
            self.reply({"error": {"type": "fixture_error", "message": redact(str(failure))}}, 500)


def terminate(process):
    if process is None or process.poll() is not None:
        return
    os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=10)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--host", required=True, type=Path)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    isolation = check_network_guard()
    host = args.host.resolve(strict=True)
    distribution = args.distribution.resolve(strict=True)
    require(host.is_file() and os.access(host, os.X_OK), "--host must be an already built executable")
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    for name in ("DESIGN.md", "SOURCES.md", "VERIFICATION.md", "source-lock.json"):
        require((distribution / "docs" / name).is_file(), "distribution omitted linked documentation: " + name)
    require((distribution / "docs/source-lock.json").read_bytes() == (distribution / "source-lock.json").read_bytes(),
            "packaged source provenance differs between root and documentation")
    require(platform.machine() in ("arm64", "x86_64", "amd64"), "unrecognized native macOS architecture")
    architecture = "arm64" if platform.machine() == "arm64" else "amd64"
    require(manifest["id"] == MODULE and manifest["runtime"] == "native", "wrong or non-native package")
    require("darwin/" + architecture in manifest["compatibility"]["platforms"], "distribution does not support this host")
    require(not manifest.get("providers") and not manifest.get("services") and not manifest.get("window"),
            "the agent distribution unexpectedly registers a provider, backend or UI")
    require(len(manifest["agents"]) == 1 and manifest["agents"][0]["id"] == AGENT,
            "the distribution must contain only the Desktop SDK agent")
    descriptor = manifest["agents"][0]
    require(descriptor.get("profile", {}).get("target") == TARGET, "the profile has another canonical target")
    require(set(descriptor["configFiles"]) == {str(path) for path in (*DESKTOP_PATHS, RECEIPT_PATH)},
            "expected exactly four Desktop files and one host-owned hash receipt")
    require(not descriptor.get("launch") and not descriptor.get("bin"), "configuration-only package gained a launcher")
    if args.output:
        output = args.output.resolve()
        output.mkdir(parents=True, exist_ok=False)
    else:
        output = Path(tempfile.mkdtemp(prefix="magpie-claude-desktop-host-e2e."))
    home = output / "home"
    config = home / ".config/magpie"
    cache = home / ".cache/magpie"
    tools = home / "bin"
    for directory in (config, cache, tools, home / "tmp"):
        directory.mkdir(parents=True, exist_ok=True)
    isolated_host = tools / "magpie"
    shutil.copyfile(host, isolated_host)
    isolated_host.chmod(0o700)
    tool_attempts = output / "blocked-tools.jsonl"
    for name in ("open", "osascript", "security", "claude", "codex", "opencode", "bun", "node", "npm", "npx"):
        path = tools / name
        path.write_text("#!/bin/sh\nprintf '%s\\n' " + repr(name) + " >> \"$MAGPIE_CLAUDE_DESKTOP_BLOCKED_TOOLS\"\nexit 127\n")
        path.chmod(0o700)
    namespace = "/fixture-" + secrets.token_hex(16)
    server = Vendor(namespace)
    vendor_thread = threading.Thread(target=server.serve_forever, name="claude-desktop-local-anthropic")
    vendor = "http://127.0.0.1:" + str(server.server_port) + namespace
    gateway_port, web_port = free_port(), free_port()
    while gateway_port == web_port:
        web_port = free_port()
    gateway = "http://127.0.0.1:" + str(gateway_port)
    environment = {name: os.environ[name] for name in ("LANG", "LC_ALL", "LC_CTYPE", "TZ", "TERM")
                   if name in os.environ}
    environment.update(HOME=str(home), USERPROFILE=str(home), SHELL="/bin/sh", TMPDIR=str(home / "tmp"),
                       PATH=str(tools) + ":/usr/bin:/bin:/usr/sbin:/sbin", XDG_CONFIG_HOME=str(home / ".config"),
                       XDG_CACHE_HOME=str(home / ".cache"), XDG_DATA_HOME=str(home / ".local/share"),
                       XDG_STATE_HOME=str(home / ".local/state"), MAGPIE_ADDR="127.0.0.1:" + str(gateway_port),
                       MAGPIE_WEB_KEY=WEB_KEY, MAGPIE_NO_ACCOUNTS="1", MAGPIE_NO_STATS="1",
                       MAGPIE_PLUGIN_CATALOG="off", MAGPIE_PLUGIN_MARKET="off", MAGPIE_PARTNERS="off",
                       MAGPIE_BUN=str(tools / "no-bun"), MAGPIE_TEST_SANDBOX=str(home), DO_NOT_TRACK="1",
                       MAGPIE_UPDATE_FEED=vendor + "/updates", MAGPIE_CLAUDE_DESKTOP_BLOCKED_TOOLS=str(tool_attempts),
                       MAGPIE_CLAUDE_DESKTOP_LOOPBACK_GUARD="sandbox-exec")
    write_json(config / "settings.json", {"noStats": True, "noUsageStats": True, "noAutoUpdate": True,
                                           "noAccountDiscovery": True, "agentTelemetryOff": True})
    write_json(cache / "models.json", {})
    initial = {
        DESKTOP_PATHS[0]: {"deploymentMode": "1p", "custom": 9007199254740993},
        DESKTOP_PATHS[1]: {"deploymentMode": "1p", "custom3p": "keep"},
        DESKTOP_PATHS[2]: {"appliedId": OTHER_PROFILE,
                           "entries": [{"id": OTHER_PROFILE, "name": "Personal profile"}], "customMeta": True},
    }
    for path, value in initial.items():
        write_json(home / path, value)
    foreign = home / APPLICATION_SUPPORT / ("Claude-3p/configLibrary/" + OTHER_PROFILE + ".json")
    write_json(foreign, {"inferenceProvider": "anthropic", "personal": "leave unchanged"})
    foreign_bytes = foreign.read_bytes()
    protected = {}
    for path in (APPLICATION_SUPPORT / "Claude/config.json", APPLICATION_SUPPORT / "Claude-3p/config.json"):
        write_json(home / path, {"personalAppSettings": str(path)})
        protected[path] = (home / path).read_bytes()
    original = {path: (home / path).read_bytes() if (home / path).exists() else None
                for path in (*DESKTOP_PATHS, RECEIPT_PATH)}
    commands, requests = [], []
    process = None
    log = None
    cookie = ""
    body_complete = False
    result = {"passed": False, "hostSHA256": digest(host),
              "manifestSHA256": digest(distribution / "magpie-plugin.json"),
              "entrySHA256": digest(distribution / manifest["entry"]), "platform": "darwin/" + architecture,
              "liveDesktopAcceptance": False, "desktopLaunched": False, "liveVendors": False,
              "compilerInvoked": False, "keychainAccessRequested": False}

    def command(*arguments, succeeds=True):
        child = subprocess.Popen([str(isolated_host), *arguments], cwd=home, env=environment,
                                 text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        try:
            stdout, stderr = child.communicate(timeout=40)
        except BaseException as failure:
            try:
                terminate(child)
            except BaseException as cleanup_failure:
                failure.add_note("CLI cleanup failed: " + redact(str(cleanup_failure)))
            if isinstance(failure, subprocess.TimeoutExpired):
                raise AssertionError("ordinary CLI did not finish: " + redact(arguments)) from failure
            raise
        finally:
            child.stdout.close()
            child.stderr.close()
        commands.append({"args": json.loads(redact(arguments)), "exit": child.returncode,
                         "stdout": redact(stdout), "stderr": redact(stderr)})
        require((child.returncode == 0) == succeeds, "ordinary CLI failed: " + json.dumps(commands[-1]))
        require(UPSTREAM_KEY not in stdout + stderr and GATEWAY_KEY not in stdout + stderr,
                "ordinary CLI disclosed a provider or agent credential")
        return stdout

    def http_request(port, path, body=None, key=None, admin=False, succeeds=True):
        connection = http.client.HTTPConnection("127.0.0.1", port, timeout=20)
        headers = {"Content-Type": "application/json", "anthropic-version": "2023-06-01"}
        if key:
            headers["Authorization"] = "Bearer " + key
        if admin:
            headers["Cookie"] = cookie
        connection.request("POST" if body is not None else "GET", path,
                           None if body is None else encoded(body), headers)
        response = connection.getresponse()
        raw = response.read(MAX_BODY + 1)
        status, content_type = response.status, response.getheader("Content-Type", "")
        connection.close()
        requests.append({"path": path, "status": status, "owner": admin})
        require(len(raw) <= MAX_BODY, "ordinary host response exceeded the fixture bound")
        require((status == 200) if succeeds else (400 <= status < 600),
                "ordinary HTTP returned an unexpected status: "
                + redact((path, status, raw[:1000].decode(errors="replace"))))
        return raw if "text/event-stream" in content_type else json.loads(raw)

    def admin(path, body=None, succeeds=True):
        return http_request(web_port, path, body, admin=True, succeeds=succeeds)

    def row():
        matches = [value for value in admin("/api/state")["agents"] if value["id"] == AGENT]
        require(len(matches) == 1, "ordinary host did not expose exactly one Desktop SDK agent")
        return matches[0]

    def binding():
        path = config / "sdk-agent-bindings.json"
        if not path.exists():
            return None
        records = json.loads(path.read_text()).get("records", {})
        matches = [record for record in records.values() if record.get("plugin") == MODULE and record.get("agent") == AGENT]
        require(len(matches) <= 1, "the host published duplicate profile ownership")
        return matches[0] if matches else None

    def unchanged_before_connect():
        for path, before in original.items():
            current = home / path
            require((current.read_bytes() if current.exists() else None) == before,
                    "installation or inspection took over Desktop before explicit connect: " + str(path))
        record = binding()
        require(record is None or record["phase"] == "disconnected", "installation claimed agent ownership")

    def unchanged_foreign():
        require(foreign.read_bytes() == foreign_bytes, "another Desktop profile was modified")
        for path, body in protected.items():
            require((home / path).read_bytes() == body, "unowned app configuration was modified: " + str(path))
        builtin = foreign.parent / (BUILTIN_PROFILE + ".json")
        require(not builtin.exists(), "the SDK invoked the competing built-in configuration writer")

    def check_catalog():
        catalog = json.loads((config / "extensions/catalog.json").read_text())
        require([item["manifest"]["id"] for item in catalog] == [MODULE], "a companion SDK module was installed")
        for name in ("plugins.json", "plugin-providers.json", "migrations.json"):
            require(not (config / name).exists(), "legacy plugin/provider migration state was created: " + name)
        require(not list(home.rglob("node_modules")), "the agent installed an external authentication dependency")

    def trigger_sync(name, previous_revision):
        admin("/api/provider/name", {"id": "anthropic", "model": GENERIC_MODEL, "modelName": name})
        return wait_for(lambda: (current if (current := binding()) and current["revision"] > previous_revision else None),
                        "the ordinary catalog change did not complete agent synchronization")

    def message(path, model, marker, key, stream=False):
        response = http_request(gateway_port, path, {"model": model, "max_tokens": 64, "stream": stream,
                                "messages": [{"role": "user", "content": marker}]}, key=key)
        if stream:
            require(isinstance(response, bytes) and b"event: message_stop" in response,
                    "ordinary Desktop streaming did not finish")
            frames = [json.loads(line[6:]) for line in response.splitlines() if line.startswith(b"data: ")]
            text = "".join(frame.get("delta", {}).get("text", "") for frame in frames)
        else:
            text = "".join(part.get("text", "") for part in response["content"])
        require(text == "synthetic-desktop:" + marker, "ordinary Anthropic/alias response was corrupted")

    try:
        vendor_thread.start()
        command("provider", "add", "anthropic", UPSTREAM_KEY, "id=anthropic", "anthropic=" + vendor,
                "models=" + VENDOR_MODEL + "," + GENERIC_MODEL, "list=off")
        providers_before = (config / "providers.json").read_bytes()
        before_cli = command(TARGET)
        require("Claude Desktop SDK" not in before_cli, "the absent module already owned the built-in agent")
        command("plugin", "sdk", "add", str(distribution))
        command("plugin", "sdk", "inspect", MODULE)
        command(AGENT)
        unchanged_before_connect()
        check_catalog()
        log = (output / "host.log").open("w")
        process = subprocess.Popen([str(isolated_host), "web", "--no-open", "--addr", "127.0.0.1:" + str(web_port)],
                                   cwd=home, env=environment, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)

        def ready():
            nonlocal cookie
            require(process.poll() is None, "ordinary web host exited before readiness")
            try:
                connection = http.client.HTTPConnection("127.0.0.1", web_port, timeout=1)
                connection.request("GET", "/?k=" + WEB_KEY)
                response = connection.getresponse()
                response.read(MAX_BODY)
                header = response.getheader("Set-Cookie", "")
                connection.close()
                if response.status == 303 and header:
                    cookie = header.split(";", 1)[0]
                    return True
            except OSError:
                pass
            return False

        wait_for(ready, "ordinary owner web server did not become ready", timeout=30)
        require(not row().get("wired") and row().get("native", {}).get("provider") == "disconnected",
                "an installed but unconnected profile took control")
        unchanged_before_connect()
        message("/v1/messages", "anthropic/" + VENDOR_MODEL, "control-before-connect", CONTROL_KEY)
        admin("/api/agents/connect/" + AGENT, {})
        connected = row()
        require(connected.get("wired") and connected.get("native", {}).get("provider") == "connected",
                "explicit connect did not establish the ordinary SDK ownership")
        record = binding()
        require(record and record["phase"] == "owned" and record["descriptor"]["profile"]["target"] == TARGET,
                "the host did not retain the canonical target ownership")
        require("Claude Desktop SDK" in command(TARGET), "ordinary canonical lookup did not select the connected profile")
        for path in (*DESKTOP_PATHS, RECEIPT_PATH):
            require((home / path).is_file() and stat.S_IMODE((home / path).stat().st_mode) == 0o600,
                    "owned configuration or receipt missing/private mode lost: " + str(path))
        for path in DESKTOP_PATHS[:2]:
            require(json.loads((home / path).read_text())["deploymentMode"] == "3p", "deployment mode was not connected")
        metadata = json.loads((home / DESKTOP_PATHS[2]).read_text())
        require(metadata["appliedId"] == PROFILE and len(metadata["entries"]) == 2,
                "the owned profile was not selected alongside the existing one")
        profile = json.loads((home / DESKTOP_PATHS[3]).read_text())
        expected = {"inferenceProvider": "gateway", "inferenceCredentialKind": "static",
                    "inferenceGatewayBaseUrl": gateway + "/agent/" + TARGET,
                    "inferenceGatewayApiKey": GATEWAY_KEY, "inferenceGatewayAuthScheme": "bearer",
                    "disableDeploymentModeChooser": True, "desktopHome": "standard",
                    "isClaudeCodeForDesktopEnabled": True}
        require(all(profile.get(key) == value for key, value in expected.items()), "the connected profile changed its gateway contract")
        receipt = json.loads((home / RECEIPT_PATH).read_text())
        require(receipt.get("schema") == 1 and set(receipt.get("fields", {})) == PROFILE_FIELDS,
                "applied field receipt has the wrong shape")
        for key, value in expected.items():
            require(receipt["fields"][key] == hashlib.sha256(encoded(value)).hexdigest(),
                    "applied field receipt does not describe the value actually written")
        require(GATEWAY_KEY not in json.dumps(receipt) and gateway not in json.dumps(receipt),
                "the hash receipt retained an endpoint or credential in plaintext")
        unchanged_foreign()
        desktop_models = http_request(gateway_port, "/agent/" + TARGET + "/v1/models", key=GATEWAY_KEY)
        write_json(output / "desktop-catalog.json", desktop_models)
        aliases = [item["id"] for item in desktop_models["data"] if item["id"].startswith("anthropic/magpie-")]
        require(len(aliases) == 1 and aliases[0] != "anthropic/" + GENERIC_MODEL,
                "the ordinary Desktop catalog did not retain its non-Claude model alias")
        count = http_request(gateway_port, "/agent/" + TARGET + "/v1/messages/count_tokens",
                             {"model": aliases[0],
                              "messages": [{"role": "user", "content": "desktop-count"}]}, key=GATEWAY_KEY)
        require(count == {"input_tokens": 37}, "Desktop token counting did not reach the ordinary Anthropic provider")
        message("/agent/" + TARGET + "/v1/messages", aliases[0], "desktop-once", GATEWAY_KEY)
        message("/agent/" + TARGET + "/v1/messages", aliases[0], "desktop-stream", GATEWAY_KEY, stream=True)
        user_config = json.loads((home / DESKTOP_PATHS[0]).read_text())
        user_config["fixtureUserEdit"] = "preserve-after-connect"
        del user_config["deploymentMode"]
        write_json(home / DESKTOP_PATHS[0], user_config)
        normalized_root = (home / DESKTOP_PATHS[0]).read_bytes()
        (home / DESKTOP_PATHS[1]).unlink()
        require(row().get("native", {}).get("provider") == "connected",
                "inspection treated root normalization as a profile override")
        trigger_sync("Desktop fixture renamed once", record["revision"])
        require((home / DESKTOP_PATHS[0]).read_bytes() == normalized_root
                and not (home / DESKTOP_PATHS[1]).exists(),
                "ordinary synchronization reclaimed normalized or deleted root mode configuration")
        command(AGENT, "default")
        require(not row().get("wired") and binding()["phase"] == "disconnected", "ordinary CLI did not disconnect the profile")
        expected_original = dict(initial[DESKTOP_PATHS[0]], fixtureUserEdit="preserve-after-connect")
        del expected_original["deploymentMode"]
        require(json.loads((home / DESKTOP_PATHS[0]).read_text()) == expected_original,
                "disconnect reclaimed the removed mode or discarded the user edit")
        require(not (home / DESKTOP_PATHS[1]).exists(), "disconnect recreated a root file deleted after connect")
        require((home / DESKTOP_PATHS[2]).read_bytes() == original[DESKTOP_PATHS[2]],
                "disconnect failed exact metadata restoration")
        require(not (home / DESKTOP_PATHS[3]).exists() and not (home / RECEIPT_PATH).exists(),
                "disconnect retained newly created owned profile/receipt files")
        require("Claude Desktop SDK" not in command(TARGET), "canonical lookup retained a disconnected SDK owner")
        unchanged_foreign()
        admin("/api/agents/connect/" + AGENT, {})
        second = binding()
        for mode in ("1p", "unsupported"):
            profile = json.loads((home / DESKTOP_PATHS[3]).read_text())
            profile["deploymentMode"] = mode
            write_json(home / DESKTOP_PATHS[3], profile)
            refused_before = {path: (home / path).read_bytes() if (home / path).exists() else None
                              for path in (*DESKTOP_PATHS, RECEIPT_PATH)}
            inspected = row()
            require(inspected.get("native", {}).get("provider") == "invalid"
                    and "different local profile or gateway" in inspected.get("native", {}).get("detail", ""),
                    "ordinary inspection accepted a first-party or invalid profile override")
            admin("/api/agents/connect/" + AGENT, {}, succeeds=False)
            for path, before in refused_before.items():
                require(((home / path).read_bytes() if (home / path).exists() else None) == before,
                        "refused profile override changed configuration: " + str(path))
            del profile["deploymentMode"]
            write_json(home / DESKTOP_PATHS[3], profile)
            require(row().get("native", {}).get("provider") == "connected",
                    "removing the conflicting optional profile mode did not restore inspection")
        sealed = "__sealed__:v1:" + base64.b64encode(b"synthetic-opaque-desktop-credential").decode()
        profile = json.loads((home / DESKTOP_PATHS[3]).read_text())
        profile["inferenceGatewayApiKey"] = sealed
        profile["fixtureProfileEdit"] = {"large": 9007199254740993}
        write_json(home / DESKTOP_PATHS[3], profile)
        user_config = json.loads((home / DESKTOP_PATHS[0]).read_text())
        user_config["deploymentMode"] = "1p"
        write_json(home / DESKTOP_PATHS[0], user_config)
        trigger_sync("Desktop fixture renamed twice", second["revision"])
        inspected = row()
        require(inspected.get("native", {}).get("provider") == "connected"
                and "sealed" in inspected.get("native", {}).get("detail", "").lower(),
                "ordinary inspection treated an opaque sealed credential as a verified plaintext key")
        require(json.loads((home / DESKTOP_PATHS[3]).read_text())["inferenceGatewayApiKey"] == sealed,
                "ordinary synchronization rewrote the user's opaque sealed credential")
        require(json.loads((home / DESKTOP_PATHS[0]).read_text())["deploymentMode"] == "1p",
                "ordinary synchronization changed an irrelevant persisted first-party selection")
        command(AGENT)
        admin("/api/agents/disconnect/" + AGENT, {})
        retained = json.loads((home / DESKTOP_PATHS[3]).read_text())
        require(retained == {"inferenceGatewayApiKey": sealed, "fixtureProfileEdit": {"large": 9007199254740993}},
                "disconnect discarded a post-connect credential/edit or left applied gateway fields behind")
        require(not (home / RECEIPT_PATH).exists(), "hash receipt was not restored/removed on the second disconnect")
        require(json.loads((home / DESKTOP_PATHS[0]).read_text())["deploymentMode"] == "1p"
                and not (home / DESKTOP_PATHS[1]).exists(),
                "disconnect changed a later persisted-mode edit or recreated the original missing root")
        unchanged_foreign()
        message("/v1/messages", "anthropic/" + VENDOR_MODEL, "control-after-disconnect", CONTROL_KEY)
        require((config / "providers.json").read_bytes() == providers_before,
                "install/connect/sync/disconnect modified the ordinary Anthropic provider")
        check_catalog()

        def observed():
            ledger = admin("/api/usage/requests?period=all&limit=100")
            rows = [item for item in ledger.get("rows", []) if item.get("provider") == "anthropic" and item.get("status") == 200]
            desktop = [item for item in rows if item.get("agent") == TARGET]
            control = [item for item in rows if item.get("agent") == "claude"]
            return ledger if len(desktop) >= 2 and len(control) >= 2 else None

        ledger = wait_for(observed, "ordinary owner ledger lost canonical Desktop or Anthropic control attribution")
        trace = admin("/api/gateway/trace")
        summary = admin("/api/usage?period=all")
        desktop_rows = [item for item in ledger["rows"] if item.get("provider") == "anthropic" and item.get("agent") == TARGET]
        require(any(item.get("model") == GENERIC_MODEL and item.get("in") == 7 and item.get("out") == 3
                    for item in desktop_rows), "ordinary owner ledger lost alias resolution or provider token usage")
        require(trace.get("mine") and any(item.get("agent") == TARGET for item in trace.get("routes", [])),
                "ordinary owner routing trace omitted the canonical Desktop agent")
        telemetry = {"usage": summary, "requests": ledger, "trace": trace}
        for secret in (UPSTREAM_KEY, GATEWAY_KEY, CONTROL_KEY, WEB_KEY, sealed):
            require(secret not in json.dumps(telemetry), "ordinary telemetry disclosed credential material")
        for secret in (UPSTREAM_KEY, GATEWAY_KEY, CONTROL_KEY, sealed):
            require(secret not in (output / "host.log").read_text(), "host log disclosed credential material")
        records, failures = server.snapshot()
        require(not failures, "local upstream fixture failed: " + json.dumps(failures))
        markers = {item.get("marker") for item in records}
        require({"control-before-connect", "desktop-count", "desktop-once", "desktop-stream", "control-after-disconnect"} <= markers,
                "the ordinary paths did not all reach the local upstream fixture")
        blocked = tool_attempts.read_text().splitlines() if tool_attempts.exists() else []
        require(not {"open", "osascript", "security"} & set(blocked), "the configuration-only path attempted app/Keychain activation")
        write_json(output / "observability.json", telemetry)
        result.update(actualNativeHostIntegration=True, explicitConnectRequired=True,
                      canonicalTargetTakeover=True, vendorConfigFiles=4, hashReceipt=True,
                      providerUnchanged=True, companionPlugins=False, opaqueCredentialPreserved=True,
                      userEditsPreserved=True, ordinaryCliDisconnect=True, ordinaryHttpDisconnect=True,
                      persistedModeNormalizationPreserved=True, deletedRootNotRecreated=True,
                      profileFirstPartyOverrideRefused=True, profileInvalidModeRefused=True,
                      persistedFirstPartyRootPreserved=True, disconnectNormalizedModesPreserved=True,
                      ordinaryCatalogSync=True, desktopModelAlias=aliases[0], countTokens=37,
                      messages=True, streaming=True, canonicalUsageAgent=TARGET,
                      ownerLedgerRows=len(ledger["rows"]), ownerTraceRows=len(trace["routes"]),
                      syntheticUpstreamRequests=records, blockedToolDiscovery=blocked)
        body_complete = True
    except BaseException as failure:
        result["error"] = redact(type(failure).__name__ + ": " + str(failure))
        raise
    finally:
        cleanup_errors = []
        cleanups = [
            ("host process", lambda: terminate(process)),
            ("host log", lambda: log.close() if log is not None else None),
            ("fixture server", lambda: server.shutdown() if vendor_thread.ident is not None else None),
            ("fixture connections", server.server_close),
            ("fixture thread", lambda: vendor_thread.join(timeout=5) if vendor_thread.ident is not None else None),
        ]
        for label, cleanup in cleanups:
            try:
                cleanup()
            except BaseException as failure:
                cleanup_errors.append(label + ": " + redact(str(failure)))
        result["fixtureThreadJoined"] = not vendor_thread.is_alive()
        result["hostProcessJoined"] = process is None or process.poll() is not None
        if not result["fixtureThreadJoined"] or not result["hostProcessJoined"]:
            cleanup_errors.append("an owned direct process or fixture thread did not join")
        result["cleanupErrors"] = cleanup_errors
        result["passed"] = body_complete and not cleanup_errors
        if (output / "host.log").exists():
            result["hostLogSHA256"] = digest(output / "host.log")
        write_json(output / "isolation.json", isolation)
        write_json(output / "commands.json", commands)
        write_json(output / "http-requests.json", requests)
        write_json(output / "result.json", result)
        if cleanup_errors:
            pending = sys.exception()
            if pending is not None:
                pending.add_note("Fixture cleanup failed: " + json.dumps(cleanup_errors))
            else:
                raise AssertionError("Fixture cleanup failed: " + json.dumps(cleanup_errors))
    print(json.dumps({"passed": result["passed"], "evidence": str(output),
                      "liveDesktopAcceptance": False, "platform": result["platform"]}))


if __name__ == "__main__":
    main()
