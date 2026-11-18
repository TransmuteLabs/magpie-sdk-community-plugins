#!/usr/bin/env python3
import argparse
import errno
import hashlib
import http.client
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import threading
import time
from urllib.parse import urlsplit


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("host", type=Path)
    parser.add_argument("distribution", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--zen-console", action="store_true")
    parser.add_argument("--agent-metadata-only", action="store_true")
    parser.add_argument("--agent-format", choices=("v1", "v2"), default="v2")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1] / "src"
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    process_status = {line.split(":")[0]: line.split(":", 1)[1].strip()
                      for line in Path("/proc/self/status").read_text().splitlines() if ":" in line}
    security = {name: value for name, value in process_status.items() if name.startswith("Cap")}
    assert all(int(security[name], 16) == 0 for name in ("CapInh", "CapPrm", "CapEff", "CapAmb")), "fixture retained capabilities"
    assert process_status["NoNewPrivs"] == "1" and process_status["Seccomp"] == "2", "fixture can gain privileges or lost its guard"
    assert os.environ.get("MAGPIE_OPENCODE_LOOPBACK_GUARD") == "seccomp-broker", "loopback supervisor is required"
    assert Path("/").stat().st_uid == 0, "fixture hid system dependency ownership"
    controls = []
    for family, kind, address in ((socket.AF_INET, socket.SOCK_STREAM, ("192.0.2.1", 443)),
                                  (socket.AF_INET, socket.SOCK_DGRAM, None),
                                  (socket.AF_INET6, socket.SOCK_STREAM, None)):
        try:
            with socket.socket(family, kind) as probe:
                probe.settimeout(1)
                if address:
                    probe.connect(address)
        except OSError as failure:
            assert failure.errno == errno.EPERM, "network guard failed for a different reason"
            controls.append({"family": family, "kind": kind, "errno": failure.errno})
        else:
            raise AssertionError("network guard allowed an external or datagram operation")
    (output / "isolation.json").write_text(json.dumps({"uid": os.getuid(), "euid": os.geteuid(), "caps": security,
                                                     "rootOwnerUID": Path("/").stat().st_uid,
                                                     "uidMap": Path("/proc/self/uid_map").read_text().strip(),
                                                     "noNewPrivs": True,
                                                     "networkGuard": "seccomp-broker", "negativeControls": controls}, indent=2) + "\n")
    home = output / "home"
    config = home / ".config/magpie"
    config.mkdir(parents=True)
    lock = threading.Lock()
    release, held = threading.Event(), threading.Event()
    release.set()
    state = {"starts": 0, "polls": 0, "chat": [], "gemini": 0, "quota": 0, "failures": [], "foreignRequests": 0}
    state.update(zenInference=[], consoleRequests=[], watchAcks=0, zenRefresh=0)
    donor_console = json.loads((root / "testdata/donors/console.json").read_text())
    donor_models = json.loads((root / "testdata/donors/zen-list.json").read_text())
    donor_catalog = json.loads((root / "testdata/donors/zen-catalog.json").read_text())["models"]
    devices = set()
    namespace = "/fixture-" + secrets.token_hex(16)

    class Vendor(http.server.BaseHTTPRequestHandler):
        def parse_request(self):
            if not super().parse_request():
                return False
            if not self.path.startswith(namespace + "/"):
                with lock:
                    state["foreignRequests"] += 1
                self.send_error(404)
                return False
            self.path = self.path[len(namespace):]
            return True

        def log_message(self, *unused):
            pass

        def reply(self, value, code=200, headers=None):
            body = json.dumps(value).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            for key, value in (headers or {}).items():
                self.send_header(key, value)
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            if self.path == "/__hold":
                held.clear()
                release.clear()
                return self.reply({})
            if self.path == "/__release":
                release.set()
                return self.reply({})
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            if self.path == "/console/auth/device/code":
                with lock:
                    state["starts"] += 1
                    device = "PRIVATE-device-" + str(state["starts"])
                    devices.add(device)
                return self.reply({"device_code": device, "user_code": "SAFE-CODE",
                                   "verification_uri_complete": vendor + "/console/verify", "expires_in": 120, "interval": 1})
            if self.path == "/console/auth/device/token":
                if args.zen_console and body.get("grant_type") == "refresh_token" and body.get("refresh_token") == "PRIVATE-refresh-4":
                    with lock:
                        state["zenRefresh"] += 1
                    return self.reply({"access_token": "ZEN-ROTATED", "refresh_token": "ZEN-ROTATED-REFRESH",
                                       "expires_in": 3600, "org_id": "changed-org"})
                with lock:
                    if body.get("device_code") not in devices:
                        state["failures"].append("device code was exchanged twice")
                        return self.reply({"error": "invalid_grant"}, 400)
                    devices.remove(body["device_code"])
                    state["polls"] += 1
                    index = state["polls"]
                if not release.is_set():
                    held.set()
                    assert release.wait(15), "held token request not joined"
                return self.reply({"access_token": "PRIVATE-access-" + str(index),
                                   "refresh_token": "PRIVATE-refresh-" + str(index),
                                   "expires_in": 3600, "org_id": "initial-org" if index == 1 else "changed-org"})
            authorization = self.headers.get("Authorization", "")
            if args.zen_console and self.path == "/console/api/event-watches/ack":
                assert authorization == "Bearer SYNTHETIC-ZEN-SERVICE" and body == {"ids": ["ev_1"]}
                with lock:
                    state["watchAcks"] += 1
                return self.reply({})
            if args.zen_console and self.path.startswith(("/inference/openai/v1/", "/inference/anthropic/v1/", "/inference/google/v1beta/")):
                selected = authorization.removeprefix("Bearer ") or self.headers.get("x-api-key") or self.headers.get("x-goog-api-key")
                assert selected in ("SYNTHETIC-ZEN-SERVICE", "PRIVATE-access-4", "ZEN-ROTATED", "public")
                assert not self.headers.get("Cookie") and not self.headers.get("x-org-id")
                with lock:
                    state["zenInference"].append({"path": self.path, "public": selected == "public",
                                                   "protocolHeader": "x-api-key" if self.headers.get("x-api-key") else "x-goog-api-key" if self.headers.get("x-goog-api-key") else "Authorization"})
                if "/anthropic/" in self.path:
                    return self.reply({"id": "fixture", "type": "message", "role": "assistant",
                                       "content": [{"type": "text", "text": "synthetic-zen"}], "stop_reason": "end_turn",
                                       "usage": {"input_tokens": 1, "output_tokens": 1}})
                if "/google/" in self.path:
                    return self.reply({"candidates": [{"content": {"role": "model", "parts": [{"text": "synthetic-zen"}]}, "finishReason": "STOP"}]})
                if self.path.endswith("/responses"):
                    return self.reply({"id": "r_fixture", "object": "response", "status": "completed",
                                       "output": [{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "synthetic-zen"}]}]})
                if selected == "public" and body.get("tools") and any(
                        item.get("function", {}).get("name") == "exec_program" for item in body["tools"]):
                    names = [item.get("function", {}).get("name") for item in body["tools"]]
                    assert "run_code" not in names and all(name in names for name in ("edit", "glob", "grep", "read", "shell"))
                    assert body["tool_choice"]["function"]["name"] == "exec_program"
                    assert body["messages"][-1]["function_call"]["name"] == "exec_program"
                    assert body["seed"] == 9007199254740993
                    assert not any("Working directory:" in item.get("content", "") for item in body["messages"])
                    assert body["stream"] is True and body["stream_options"]["include_usage"] is True
                    frames = [
                        {"choices": [{"index": 0, "delta": {"role": "assistant", "tool_calls": [{
                            "index": 0, "id": "call_fixture", "type": "function",
                            "function": {"name": "exec_program", "arguments": "{"}}]}}]},
                        {"choices": [{"index": 0, "delta": {"tool_calls": [{
                            "index": 0, "function": {"arguments": "}"}}]}, "finish_reason": "tool_calls"}],
                         "usage": {"prompt_tokens": 3, "completion_tokens": 2}},
                    ]
                    data = ("".join("data: " + json.dumps(frame) + "\n\n" for frame in frames) + "data: [DONE]\n\n").encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.send_header("Content-Length", str(len(data)))
                    self.end_headers()
                    self.wfile.write(data)
                    return
                return self.reply({"choices": [{"message": {"role": "assistant", "content": "synthetic-zen"}, "finish_reason": "stop"}],
                                   "usage": {"prompt_tokens": 1, "completion_tokens": 1}})
            if self.path == "/inference/go/openai/v1/chat/completions":
                if authorization != "Bearer PRIVATE-access-2":
                    with lock:
                        state["failures"].append("gateway did not use the current account")
                    return self.reply({}, 401)
                with lock:
                    state["chat"].append({"model": body.get("model"), "agent": "launched-agent" in json.dumps(body),
                                          "project": self.headers.get("x-opencode-project")})
                return self.reply({"choices": [{"message": {"role": "assistant", "content": "synthetic-go"}, "finish_reason": "stop"}],
                                   "usage": {"prompt_tokens": 5, "completion_tokens": 2}})
            if self.path in ("/inference/go/google/v1beta/models/gemini-4-argon:generateContent",
                             "/inference/go/google/v1beta/models/gemini-4-argon:streamGenerateContent?alt=sse"):
                with lock:
                    state["gemini"] += 1
                    if self.headers.get("x-goog-api-key") != "PRIVATE-access-2" or authorization:
                        state["failures"].append("Gemini selected-account signing changed")
                result = {"candidates": [{"index": 0, "content": {"role": "model", "parts": [{"text": "synthetic-go"}]},
                                          "finishReason": "STOP"}]}
                if ":streamGenerateContent" in self.path:
                    data = ("data: " + json.dumps(result) + "\n\n").encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.send_header("Content-Length", str(len(data)))
                    self.end_headers()
                    self.wfile.write(data)
                    return
                return self.reply(result)
            with lock:
                state["failures"].append("unexpected or Zen request: " + self.path)
            self.reply({}, 404)

        def do_GET(self):
            path = urlsplit(self.path).path
            if args.zen_console and path == "/inference/v1/models":
                return self.reply(donor_models)
            if args.zen_console and path.startswith("/console/api/v2/"):
                assert self.headers.get("Authorization") in ("Bearer SYNTHETIC-ZEN-SERVICE", "Bearer PRIVATE-access-4", "Bearer ZEN-ROTATED")
                with lock:
                    state["consoleRequests"].append(path)
                if path == "/console/api/v2/config":
                    return self.reply(donor_console["config"], headers={"x-opencode-instructions-skills": "", "x-opencode-event-watches": ""})
                if path == "/console/api/v2/instructions":
                    return self.reply(donor_console["instructions"])
                if path == "/console/api/v2/skills":
                    return self.reply(donor_console["skills"])
                if path == "/console/api/v2/skills/deploy":
                    return self.reply(donor_console["bundle"])
            if args.zen_console and path == "/console/api/event-watches/events":
                return self.reply(donor_console["poll"])
            if args.zen_console and path == "/console/api/event-watches/types":
                assert self.headers.get("Authorization") == "Bearer SYNTHETIC-ZEN-SERVICE"
                assert self.headers.get("User-Agent", "").startswith("magpie-opencode/")
                return self.reply({"types": [{"server": "fixture", "name": "deploy.finished",
                                             "description": "  A\n deploy  finished. ",
                                             "inputSchema": {"type": "object"}}]})
            if self.path == "/__state":
                return self.reply({"held": held.is_set()})
            if self.path == "/console/api/user":
                return self.reply({"id": "same-user", "email": "fixture@example.invalid"})
            if self.path == "/console/api/orgs":
                return self.reply([{"id": "initial-org", "name": "Initial"}, {"id": "changed-org", "name": "Changed"}])
            if self.path == "/zen/go/v1/usage":
                with lock:
                    state["quota"] += 1
                return self.reply({"usage": {"rolling": {"percent": 23}}})
            if urlsplit(self.path).path == "/updates":
                return self.reply({}, 404)
            with lock:
                state["failures"].append("unexpected GET: " + self.path)
            self.reply({}, 404)

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Vendor)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    vendor = "http://127.0.0.1:" + str(server.server_port) + namespace
    foreign_probes = []
    env = os.environ.copy()
    for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        env.pop(name, None)
    env.update(HOME=str(home), USERPROFILE=str(home), SHELL="/bin/bash",
               XDG_CONFIG_HOME=str(home / ".config"), XDG_CACHE_HOME=str(home / ".cache"),
               XDG_DATA_HOME=str(home / ".local/share"), XDG_STATE_HOME=str(home / ".local/state"),
               MAGPIE_ADDR="127.0.0.1:" + str(free_port()), MAGPIE_OPENCODE_FIXTURE_URL=vendor,
               MAGPIE_OPENCODE_FIXTURE_VARIANT="window", MAGPIE_NO_STATS="1",
               MAGPIE_TEST_SANDBOX=str(home), MAGPIE_NO_ACCOUNTS="0", MAGPIE_PLUGIN_CATALOG="off",
               MAGPIE_PLUGIN_MARKET="off", MAGPIE_PARTNERS="off", DO_NOT_TRACK="1",
               MAGPIE_BUN=str(home / "no-bun"), MAGPIE_UPDATE_FEED=vendor + "/updates")
    events = output / "native-events.jsonl"
    env["MAGPIE_OPENCODE_FIXTURE_EVENTS"] = str(events)
    if args.agent_metadata_only and json.loads((args.distribution / "magpie-plugin.json").read_text())["contract"]["version"]["minor"] == 8:
        env["MAGPIE_OPENCODE_FIXTURE_VARIANT"] = "window-api8"
    (config / "settings.json").write_text(json.dumps({"noStats": True, "noUsageStats": True, "noAutoUpdate": True,
                                                      "noAccountDiscovery": False, "agentTelemetryOff": True}))
    cache = home / ".cache/magpie"
    cache.mkdir(parents=True)
    (cache / "models.json").write_text("{}")
    binary_dir = home / "bin"
    binary_dir.mkdir()
    binary = binary_dir / "opencode"
    binary.write_text("#!/usr/bin/python3\nimport runpy\nrunpy.run_path(" + repr(str(root.parent / "tests/agent_cli.py")) + ",run_name='__main__')\n")
    binary.chmod(0o700)
    env["PATH"] = str(binary_dir) + ":" + env["PATH"]
    personal = home / ".config/opencode/opencode.json"
    personal.parent.mkdir(parents=True)
    original = b'{"model":"own/model","theme":"dark","mcp":{"mine":{"type":"local","command":["true"]}},"custom":9007199254740993}\n'
    personal.write_bytes(original)
    shell = "# synthetic personal shell\n"
    for name in (".bashrc", ".zshrc"):
        (home / name).write_text(shell)
    commands = []
    process = None
    logfile = None

    def command(*values, expected=0):
        result = subprocess.run([str(args.host.resolve()), *values], cwd=home, env=env, text=True, capture_output=True, timeout=40)
        for secret in ("PRIVATE-access-", "PRIVATE-refresh-", "PRIVATE-device-"):
            assert secret not in result.stdout + result.stderr, "CLI disclosed private credential material"
        commands.append({"args": list(values), "exit": result.returncode, "stdout": result.stdout, "stderr": result.stderr})
        assert result.returncode == expected, json.dumps(commands[-1])
        return result.stdout

    def request(path, body=None):
        connection = http.client.HTTPConnection("127.0.0.1", web_port, timeout=30)
        connection.request("POST" if body is not None else "GET", path, None if body is None else json.dumps(body),
                           {"Cookie": cookie, "Content-Type": "application/json"})
        response = connection.getresponse()
        raw = response.read()
        connection.close()
        assert response.status == 200, (response.status, raw[:1000])
        return json.loads(raw)

    def self_contained():
        catalog = json.loads((config / "extensions/catalog.json").read_text())
        assert [item["manifest"]["id"] for item in catalog] == ["opencode"], "companion SDK package installed"
        for forbidden in ("plugins.json", "plugin-providers.json", "migrations.json"):
            assert not (config / forbidden).exists(), "legacy provider/plugin path entered: " + forbidden
        assert not list(home.rglob("node_modules")), "runtime installed an auth/package companion"
        records = [json.loads(line) for line in events.read_text().splitlines()]
        host_records = [record for record in records if record["pid"] == process.pid]
        assert any(record["method"] == "provider.execute" for record in host_records), "gateway did not execute native provider"
        assert any(record["method"] == "auth.finish" for record in host_records), "gateway did not execute native auth"
        assert any(record["method"] == "agent.plan" for record in records), "native agent controller was not used"
        children = {}
        for status in Path("/proc").glob("[0-9]*/status"):
            try:
                values = dict(line.split(":", 1) for line in status.read_text().splitlines() if ":" in line)
                children[int(status.parent.name)] = (int(values["PPid"].strip()), values["Name"].strip())
            except (OSError, ValueError, KeyError):
                continue
        descendants = {process.pid}
        while True:
            found = {pid for pid, (parent, _) in children.items() if parent in descendants}
            if found <= descendants:
                break
            descendants |= found
        assert not any(children[pid][1] in ("bun", "node", "npm", "npx") for pid in descendants if pid in children), "legacy plugin worker started"
        return {"installedModules": ["opencode"], "legacyCompanions": False, "runtimeBun": False,
                "nativeGatewayPID": process.pid}

    def refused_page(ref):
        connection = http.client.HTTPConnection("127.0.0.1", web_port, timeout=10)
        connection.request("POST", "/api/plugin-pages/accounts", json.dumps({"ref": ref, "binding": "account.info"}),
                           {"Cookie": cookie, "Content-Type": "application/json"})
        response = connection.getresponse()
        response.read()
        connection.close()
        assert response.status != 200, "revoked package/config page retained authority"

    def reauthorize_launch():
        with lock:
            before = (len(state["chat"]), state["gemini"])
        command("opencode-sdk", "run", "synthetic", expected=1)
        assert "sync or reconnect" in commands[-1]["stderr"], "launch failed for a different reason"
        with lock:
            assert (len(state["chat"]), state["gemini"]) == before, "stale launch reached the vendor"
        request("/api/agents/reapply/opencode-sdk", {})

    def owner_observability():
        expected = {"extension-" + hashlib.sha256(("opencode\0" + provider).encode()).hexdigest()[:24]
                    for provider in ("opencode-go", "opencode-zen") if args.zen_console or provider == "opencode-go"}
        deadline = time.monotonic() + 15
        while True:
            ledger = request("/api/usage/requests?period=all&limit=200")
            rows = ledger.get("rows", [])
            observed = {row.get("provider") for row in rows}
            if expected <= observed or time.monotonic() >= deadline:
                break
            time.sleep(0.05)
        summary = request("/api/usage?period=all")
        trace = request("/api/gateway/trace")
        collected = {"usage": summary, "requests": ledger, "trace": trace}
        (output / "owner-observability.json").write_text(json.dumps(collected, indent=2) + "\n")
        assert expected <= observed, ("ordinary owner ledger omitted a native provider", sorted(expected), sorted(observed))
        assert trace.get("mine") and trace.get("routes"), "ordinary owner trace did not retain native routes"
        for provider in expected:
            selected = [row for row in rows if row.get("provider") == provider]
            assert any(row.get("in", 0) > 0 and row.get("out", 0) > 0 for row in selected), (
                "ordinary owner ledger lost native usage", provider, selected)
        for secret in ("PRIVATE-access-", "PRIVATE-refresh-", "PRIVATE-device-", "SYNTHETIC-ZEN-SERVICE"):
            assert secret not in json.dumps(collected), "ordinary owner telemetry disclosed private authentication"
            assert secret not in (output / "host.log").read_text(), "collected host log disclosed private authentication"
        return {"providers": sorted(expected), "requestRows": len(rows), "routeRows": len(trace["routes"]),
                "actualOwnerUsage": True, "actualOwnerTrace": True, "hostLogCollected": True,
                "hostLogSHA256": hashlib.sha256((output / "host.log").read_bytes()).hexdigest(),
                "credentialViolations": 0}

    try:
        for path in ("/", "/foreign-fixture/console/api/user"):
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=5)
            connection.request("GET", path)
            response = connection.getresponse()
            response.read()
            connection.close()
            assert response.status == 404, "foreign fixture path was accepted"
            foreign_probes.append({"path": path, "status": response.status})
        with lock:
            assert not state["failures"] and state["starts"] == 0 and state["polls"] == 0, (
                "foreign request entered vendor state", state)
        distribution = args.distribution.resolve()
        declaration = json.loads((distribution / "magpie-plugin.json").read_text())
        fixture = output / "fixture-dist"
        shutil.copytree(distribution, fixture)
        subprocess.run(["go", "build", "-mod=readonly", "-buildmode=c-shared", "-o", str(fixture / "lib/opencode.so"), "./testdata/native-fixture"],
                       cwd=root, env=env, check=True, timeout=180)
        command("plugin", "sdk", "add", str(fixture))
        module_config = {"projectId": "global"}
        if args.agent_metadata_only:
            module_config["agentFormat"] = args.agent_format
        command("plugin", "sdk", "config", "opencode", json.dumps(module_config), "--expected", "0")
        web_port = free_port()
        origin = "http://127.0.0.1:" + str(web_port)
        env["MAGPIE_WEB_KEY"] = "synthetic-web-entry"
        logfile = (output / "host.log").open("w")
        process = subprocess.Popen([str(args.host.resolve()), "web", "--no-open", "--gateway", "--addr", "127.0.0.1:" + str(web_port)],
                                   cwd=home, env=env, stdout=logfile, stderr=subprocess.STDOUT, start_new_session=True)
        cookie = ""
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            assert process.poll() is None, "ordinary host exited"
            try:
                connection = http.client.HTTPConnection("127.0.0.1", web_port, timeout=2)
                connection.request("GET", "/?k=synthetic-web-entry")
                response = connection.getresponse()
                response.read()
                if response.status == 303:
                    cookie = response.getheader("Set-Cookie").split(";", 1)[0]
                    connection.close()
                    break
                connection.close()
            except OSError:
                pass
            time.sleep(0.05)
        assert cookie, "web never became ready"
        pages = request("/api/plugin-pages")
        page = next(item for item in pages if item["name"] == "opencode")
        settings = output / "window-driver.json"
        settings.write_text(json.dumps({"origin": origin, "vendor": vendor, "cookie": cookie, "page": page,
                                       "output": str(output / "window-result.json")}))
        subprocess.run(["node", "--experimental-vm-modules", str(root.parent / "tests/window_e2e.mjs"), str(settings)],
                       cwd=root, env=dict(env, NODE_NO_WARNINGS="1"), check=True, timeout=90,
                       stdout=(output / "window.log").open("w"), stderr=subprocess.STDOUT)
        account_files = list(config.rglob("accounts.json"))
        assert len(account_files) == 1
        accounts = json.loads(account_files[0].read_text())["accounts"]
        assert len(accounts) == 1
        account = accounts[0]
        assert account["generation"] == 2 and account["auth"]["access"] == "PRIVATE-access-2"
        assert account["auth"]["initialOrgId"] == "initial-org" and account["auth"]["orgId"] == "changed-org"
        with lock:
            assert state["starts"] == 3 and state["polls"] == 3 and not state["failures"], (
                "auth replay or cancellation changed exchange count", state)
        provider = "extension-" + hashlib.sha256(b"opencode\0opencode-go").hexdigest()[:24]
        if args.agent_metadata_only:
            from agent_metadata_e2e import verify
            report = verify(command, request, output, home, events, provider, args.agent_format, declaration)
            assert personal.read_bytes() == original, "agent metadata changed personal config"
            report["hostBinarySHA256"] = hashlib.sha256(args.host.read_bytes()).hexdigest()
            report["stockLibrarySHA256"] = hashlib.sha256((distribution / declaration["entry"]).read_bytes()).hexdigest()
            report["fixtureLibrarySHA256"] = hashlib.sha256((fixture / declaration["entry"]).read_bytes()).hexdigest()
            report["actualOpenCodeBinary"] = False
            report["liveVendorAcceptance"] = False
            (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
            print(json.dumps(report))
            return
        for model in ("deepseek-v4.1-flash", "glm-5.2", "kimi-k3", "gemini-4-argon"):
            command("provider", "test", provider, model)
        request("/api/set", {"Agent": "opencode-sdk", "Field": "model", "Value": "magpie/" + provider + "/deepseek-v4.1-flash"})
        request("/api/set", {"Agent": "opencode-sdk", "Field": "small", "Value": "magpie/" + provider + "/glm-5.2"})
        assert personal.read_bytes() == original, "profile changed personal config"
        help_config = home / ".config/magpie/extensions/opencode/agent.json"
        connected = json.loads(help_config.read_text())
        gateway = urlsplit(connected["providers"]["magpie"]["settings"]["baseURL"])
        gateway_key = connected["providers"]["magpie"]["settings"]["apiKey"]
        for model in ("deepseek-v4.1-flash", "glm-5.2", "kimi-k3", "gemini-4-argon"):
            connection = http.client.HTTPConnection(gateway.hostname, gateway.port, timeout=15)
            connection.request("POST", gateway.path + "/chat/completions", json.dumps({
                "model": provider + "/" + model, "messages": [{"role": "user", "content": "ordinary-gateway"}],
            }), {"Authorization": "Bearer " + gateway_key, "Content-Type": "application/json"})
            response = connection.getresponse()
            result = json.loads(response.read())
            connection.close()
            assert response.status == 200 and result["choices"][0]["message"]["content"] == "synthetic-go", "ordinary gateway model failed"
        isolation = self_contained()
        with lock:
            assert state["starts"] == 3 and state["polls"] == 3 and not state["failures"], (
                "provider used an undeclared vendor path", state)
        (output / "provider-window-result.json").write_text(json.dumps({
            "actualNative": True, "models": 4, "ordinaryGatewayModels": 4, "accountGeneration": account["generation"],
            "selfContained": isolation, "ordinaryLaunch": False, "libraryAcceptance": False,
            "deviceStarts": 3, "deviceExchanges": 3, "completedStartReplayExchanged": False,
        }, indent=2) + "\n")
        initial_launch = json.loads(command("opencode-sdk", "run", "synthetic"))
        assert initial_launch["usedCheckedOverlay"] and initial_launch["ordinaryGateway"]
        (output / "pre-library-result.json").write_text(json.dumps({
            "actualNative": True, "models": 4, "accountGeneration": account["generation"],
            "ordinaryLaunch": initial_launch, "libraryAcceptance": False,
            "selfContained": isolation,
        }, indent=2) + "\n")
        instructions = home / "instructions.md"
        instructions.write_text("SDK OpenCode Library fixture\n")
        personal_instructions = personal.parent / "AGENTS.md"
        personal_instructions.write_text("Synthetic personal guidance\n")
        personal_skill = personal.parent / "skills/personal/SKILL.md"
        personal_skill.parent.mkdir(parents=True)
        personal_skill.write_text("---\nname: personal\ndescription: Personal fixture\n---\nPersonal skill text\n")
        command("library", "instructions", "set", str(instructions))
        command("library", "instructions", "agents", "opencode")
        command("library", "mcp", "router", "off")
        command("library", "mcp", "add", "sdk-fixture", "/usr/bin/true", "agents=opencode")
        request("/api/library/skills/create", {"name": "sdk-fixture", "description": "SDK OpenCode skill fixture", "agents": ["opencode"]})
        skill_file = personal.parent / "skills/sdk-fixture/SKILL.md"
        assert "SDK OpenCode skill fixture" in skill_file.read_text()
        assert "Personal skill text" in personal_skill.read_text()
        assert "Synthetic personal guidance" in personal_instructions.read_text()
        after_library = json.loads(personal.read_text())
        assert after_library["mcp"]["mine"]["command"] == ["true"] and "sdk-fixture" in after_library["mcp"]
        launch = json.loads(command("opencode-sdk", "run", "synthetic"))
        assert launch["usedCheckedOverlay"] and launch["libraryInstructions"] and launch["ordinaryGateway"]
        overlay = json.loads(help_config.read_text())
        assert overlay["model"].endswith("/deepseek-v4.1-flash")
        assert overlay["agents"]["title"]["model"].endswith("/glm-5.2")
        command("plugin", "sdk", "reload", "opencode")
        reauthorize_launch()
        assert json.loads(command("opencode-sdk", "run", "synthetic"))["ordinaryGateway"]
        overlay["userMarker"] = {"large": 9007199254740993}
        overlay["providers"]["magpie"]["models"][provider + "/deepseek-v4.1-flash"]["limit"]["output"] = 7777
        overlay["providers"]["magpie"]["compatibility"]["supportsPromptCacheKey"] = False
        help_config.write_text(json.dumps(overlay))
        request("/api/set", {"Agent": "opencode-sdk", "Field": "model", "Value": "magpie/" + provider + "/deepseek-v4.1-flash"})
        synced = json.loads(help_config.read_text())
        assert synced["userMarker"]["large"] == 9007199254740993
        assert synced["providers"]["magpie"]["models"][provider + "/deepseek-v4.1-flash"]["limit"]["output"] == 7777
        assert synced["providers"]["magpie"]["compatibility"]["supportsPromptCacheKey"] is False
        project = "a" * 40
        command("plugin", "sdk", "config", "opencode", json.dumps({"projectId": project}), "--expected", "1")
        refused_page(page["ref"])
        page = next(item for item in request("/api/plugin-pages") if item["name"] == "opencode")
        assert len(request("/api/plugin-pages/accounts", {"ref": page["ref"], "binding": "account.info"})) == 1
        reauthorize_launch()
        assert json.loads(command("opencode-sdk", "run", "synthetic"))["ordinaryGateway"]
        with lock:
            assert state["chat"][-1]["project"] == project, "long-running gateway retained old module config"
        replacement = output / "replacement-dist"
        shutil.copytree(fixture, replacement)
        with (replacement / "web/index.html").open("a") as changed:
            changed.write("\n<!-- synthetic package generation replacement -->\n")
        command("plugin", "sdk", "add", str(replacement))
        refused_page(page["ref"])
        page = next(item for item in request("/api/plugin-pages") if item["name"] == "opencode")
        assert len(request("/api/plugin-pages/accounts", {"ref": page["ref"], "binding": "account.info"})) == 1
        reauthorize_launch()
        assert json.loads(command("opencode-sdk", "run", "synthetic"))["ordinaryGateway"]
        command("plugin", "sdk", "off", "opencode")
        refused_page(page["ref"])
        command("opencode-sdk", "run", "synthetic", expected=1)
        command("plugin", "sdk", "on", "opencode")
        assert json.loads(command("opencode-sdk", "run", "synthetic"))["ordinaryGateway"]
        request("/api/agents/disconnect/opencode-sdk", {})
        restored = json.loads(help_config.read_text())
        assert restored["userMarker"]["large"] == 9007199254740993
        assert not restored.get("model") and not restored.get("agents", {}).get("title", {}).get("model")
        assert not restored.get("providers", {}).get("magpie", {}).get("settings", {}).get("apiKey")
        assert json.loads(personal.read_text()) == after_library, "disconnect changed independent personal/Library config"
        for name in (".bashrc", ".zshrc"):
            assert (home / name).read_text() == shell, "disconnect did not restore personal shell"
        command("library", "skill", "rm", "sdk-fixture")
        command("library", "mcp", "rm", "sdk-fixture")
        command("library", "instructions", "agents", "none")
        assert not skill_file.exists() and "Personal skill text" in personal_skill.read_text()
        assert "SDK OpenCode Library fixture" not in personal_instructions.read_text()
        assert "Synthetic personal guidance" in personal_instructions.read_text()
        assert json.loads(personal.read_text()) == json.loads(original), "Library removal changed personal config"
        for secret in ("PRIVATE-access-", "PRIVATE-refresh-", "PRIVATE-device-"):
            assert secret not in (output / "host.log").read_text(), "host log disclosed private credentials"
        with lock:
            assert state["starts"] == 3 and state["polls"] == 3 and not state["failures"], state
            assert state["gemini"] == 2 and len(state["chat"]) == 12
        report = {"hostSHA256": hashlib.sha256(args.host.read_bytes()).hexdigest(),
                  "librarySHA256": hashlib.sha256((fixture / "lib/opencode.so").read_bytes()).hexdigest(),
                  "actualNative": True, "models": 4, "ordinaryGatewayModels": 4,
                  "accountGeneration": 2, "initialOrganizationRetained": True,
                  "ordinaryLaunch": launch, "libraryPreservedUserMCP": True, "offDenied": True, "reload": True,
                  "librarySkills": True, "libraryRemovedOwnedOnly": True,
                  "configGeneration": True, "packageReplacement": True, "onResumed": True, "restored": True,
                  "staleLaunchRefusedBeforeDispatch": True, "explicitReapplyRequired": True,
                  "deviceStarts": 3, "deviceExchanges": 3, "vendorFailures": [], "liveVendors": False,
                  "foreignNamespaceProbes": foreign_probes, "rejectedForeignRequests": state["foreignRequests"],
                  "selfContained": self_contained()}
        if args.zen_console:
            page = next(item for item in request("/api/plugin-pages") if item["name"] == "opencode")
            command("plugin", "sdk", "config", "opencode", json.dumps({"projectId": project, "consoleWatches": True}), "--expected", "2")
            page = next(item for item in request("/api/plugin-pages") if item["name"] == "opencode")
            settings.write_text(json.dumps({"origin": origin, "vendor": vendor, "cookie": cookie, "page": page, "zenOnly": True,
                                           "output": str(output / "zen-window-result.json")}))
            subprocess.run(["node", "--experimental-vm-modules", str(root.parent / "tests/window_e2e.mjs"), str(settings)],
                           cwd=root, env=dict(env, NODE_NO_WARNINGS="1"), check=True, timeout=90,
                           stdout=(output / "zen-window.log").open("w"), stderr=subprocess.STDOUT)
            zen = json.loads((output / "zen-window-result.json").read_text())["result"]["zen"]
            command("plugin", "sdk", "auth", "refresh", "opencode", "opencode-zen",
                    zen["oauth"]["id"], "--scope", json.dumps({"generation": int(zen["oauth"]["generation"])}))
            listed = {item["id"] for item in donor_models["data"]}
            for protocol in ("chat-completions", "responses", "messages", "generate-content"):
                model = next(key for key, value in donor_catalog.items() if key in listed and protocol in value.get("api", {}))
                zen_provider = "extension-" + hashlib.sha256(b"opencode\0opencode-zen").hexdigest()[:24]
                command("provider", "test", zen_provider, model)
            public_model = next(key for key in sorted(listed) if key.endswith("-free") and
                                donor_catalog.get(key, {}).get("status") not in ("deprecated", "retired", "disabled") and
                                "chat-completions" in donor_catalog.get(key, {}).get("api", {}))
            public_provider = "extension-" + hashlib.sha256(b"opencode\0opencode-zen-free").hexdigest()[:24]
            command("provider", "test", public_provider, public_model)
            chat_model = next(key for key, value in donor_catalog.items() if key in listed and "chat-completions" in value.get("api", {}))
            connection = http.client.HTTPConnection(gateway.hostname, gateway.port, timeout=15)
            connection.request("POST", gateway.path + "/chat/completions", json.dumps({
                "model": zen_provider + "/" + chat_model, "messages": [{"role": "user", "content": "ordinary-zen-gateway"}],
            }), {"Authorization": "Bearer " + gateway_key, "Content-Type": "application/json"})
            response = connection.getresponse()
            value = json.loads(response.read())
            connection.close()
            assert response.status == 200 and value["choices"][0]["message"]["content"] == "synthetic-zen"
            connection = http.client.HTTPConnection(gateway.hostname, gateway.port, timeout=15)
            connection.request("POST", gateway.path + "/chat/completions", json.dumps({
                "model": public_provider + "/" + public_model,
                "messages": [{"role": "user", "content": "ordinary public tools"},
                             {"role": "assistant", "function_call": {"name": "run_code", "arguments": "{}"}}],
                "tools": [{"type": "function", "function": {"name": "run_code", "parameters": {"type": "object"}}}],
                "tool_choice": {"type": "function", "function": {"name": "run_code"}},
                "seed": 9007199254740993,
            }), {"Authorization": "Bearer " + gateway_key, "Content-Type": "application/json"})
            response = connection.getresponse()
            value = json.loads(response.read())
            connection.close()
            assert response.status == 200 and value["choices"][0]["message"]["tool_calls"][0]["function"]["name"] == "run_code", value
            assert "exec_program" not in json.dumps(value)
            assert value["choices"][0]["message"]["tool_calls"][0]["function"]["arguments"] == "{}"
            assert value["usage"] == {"prompt_tokens": 3, "completion_tokens": 2}
            native_records = [json.loads(line) for line in events.read_text().splitlines()]
            assert any(record["pid"] == process.pid and record["method"] == "provider.execute" and
                       record["scope"].get("provider") == "opencode-zen" for record in native_records)
            with lock:
                assert state["starts"] == 4 and state["polls"] == 4 and state["zenRefresh"] == 1
                assert state["watchAcks"] == 1 and len(state["zenInference"]) == 7 and not state["failures"]
                report["zen"] = dict(zen, nativeProtocols=state["zenInference"], refreshGeneration=2,
                                     ordinaryHTTPGateway=True, ordinaryPublicToolShaping=True, publicToolRetries=0,
                                     publicForcedStreamJSONCollapse=True,
                                     policyEnforcement=False, automaticSessionWake=False, actualOpenCodeBinary=False)
            accounts = json.loads(account_files[0].read_text())["accounts"]
            assert len(accounts) == 4
            assert next(item for item in accounts if item["id"] == zen["oauth"]["id"])["generation"] == 2
            assert "SYNTHETIC-ZEN-SERVICE" not in (output / "host.log").read_text()
        report["storageSourceDeclared"] = bool(declaration.get("storageSources"))
        report["ownerObservability"] = owner_observability()
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
        print(json.dumps(report))
    finally:
        (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
        with lock:
            (output / "vendor-state.json").write_text(json.dumps(state, indent=2) + "\n")
        release.set()
        if process is not None:
            os.killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
        if logfile is not None:
            logfile.close()
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


if __name__ == "__main__":
    main()
