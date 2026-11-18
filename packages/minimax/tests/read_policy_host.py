"""Named read recovery through ordinary host calls and actual MiniMax handlers."""

import argparse
from contextlib import ExitStack
import importlib.util
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import threading


specification = importlib.util.spec_from_file_location("minimax_policy_host", Path(__file__).with_name("policy_host.py"))
policy = importlib.util.module_from_spec(specification)
specification.loader.exec_module(policy)
fixture = policy.fixture
require = fixture.require
SEARCH = "/mavis/api/v1/mcp/web_search"
CATALOG = "/minimax-cloud/api/v1/connectors/connected-tools/list"
INVOKE = "/minimax-cloud/api/v1/connectors/tools/call"


class ReadEndpoint(policy.PolicyEndpoint):
    def dispatch(self, path, value, region):
        if path in (SEARCH, CATALOG):
            with self.server.lock:
                count = self.server.reads.get(path, 0) + 1
                self.server.reads[path] = count
            mode = self.server.mode
            failed = count % 2 == 1 if mode in ("paid", "budget") else count == 1
            if failed:
                if mode == "redirect":
                    self.send_response(307)
                    self.send_header("Location", "https://agent.minimax.io/credential-trap")
                    self.send_header("Content-Length", "0")
                    self.end_headers()
                    self.server.refused.set()
                    return
                body = {"base_resp": {"status_code": 503, "status_msg": "private-read-message " +
                                     self.headers["Authorization"].removeprefix("Bearer ")}}
                if mode == "malformed":
                    body["base_resp"]["status_code"] = 503.5
                encoded = json.dumps(body).encode()
                self.send_response(503)
                self.send_header("Content-Type", "application/json")
                self.send_header("Retry-After", "4" if mode in ("cancel", "config", "generation", "disable") else "0")
                self.send_header("X-Private-Fixture", "private-read-header")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)
                self.wfile.flush()
                self.server.refused.set()
                return
            if path == SEARCH:
                require(value.get("query") == "private-read-query", "named search changed its input")
                self.reply({"organic": [{"title": "Recovered", "url": "https://example.invalid/read"}]})
                return
        if path == INVOKE and self.server.mode == "invoke":
            self.reply({"base_resp": {"status_code": 503, "status_msg": "uncertain paid work"}}, 503)
            return
        super().dispatch(path, value, region)


fixture.EndpointHandler = ReadEndpoint


def stop_call(process):
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)


def invocation(host, mode, account):
    if mode in ("search", "denied", "malformed", "redirect", "cancel", "config", "generation", "disable"):
        method = "module.call"
        params = {"action": "search." + fixture.GLOBAL,
                  "input": {"source": fixture.GLOBAL, "query": "private-read-query", "count": 1}}
    elif mode in ("paid", "budget"):
        method = "provider.execute"
        params = {"operation": "images.generate", "protocol": "chat", "model": "MiniMax-Image",
                  "body": policy.base64.b64encode(json.dumps({"model": "MiniMax-Image",
                      "prompt": "private-read-prompt", "response_format": "url"}).encode()).decode()}
    else:
        method = "tool.call"
        name = {"web": "web_search", "catalog": "list_tools", "schema": "tool_schema",
                "invoke": "generate_image"}[mode]
        arguments = {"web": {"query": "private-read-query"}, "catalog": {},
                     "schema": {"tool_name": "connector__matrix__generate_image"},
                     "invoke": {"requests": [{"prompt": "private-read-prompt"}]}}[mode]
        params = {"id": name, "arguments": arguments}
    return [str(host.executable), "plugin", "sdk", "call", fixture.MODULE, method, json.dumps(params),
            "--scope", json.dumps({"provider": fixture.GLOBAL, "account": account, "session": "named-read-" + mode})]


def run(host_binary, worker, output, modes, require_diagnostics):
    require(not output.exists(), "evidence directory must be new")
    output.mkdir()
    distribution = output / "exec-test-package"
    (distribution / "bin").mkdir(parents=True)
    shutil.copy2(worker, distribution / "bin/minimax")
    declaration = subprocess.check_output([str(worker), "--manifest"], timeout=10)
    (distribution / "magpie-plugin.json").write_bytes(declaration)
    fixture.copy_window_assets(distribution)
    report = {"runtime": "ordinary selected CLI calls; actual handlers in exec harness",
              "hostSHA256": fixture.sha256(host_binary), "workerSHA256": fixture.sha256(worker),
              "requireDiagnostics": require_diagnostics, "diagnosticGaps": [], "cases": []}
    try:
        for mode in modes:
            case = output / mode
            case.mkdir()
            proxy = host = process = None
            try:
                with ExitStack() as stack:
                    home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="minimax-read-home-")))
                    ca, context = fixture.certificate(case, {"HOME": str(home), "PATH": "/usr/bin:/bin"})
                    shutil.copy2(ca, distribution / "bin/fixture-ca.pem")
                    proxy = fixture.Fixture(context)
                    proxy.mode, proxy.primary, proxy.devices, proxy.tokens = mode, None, {}, {}
                    proxy.reads, proxy.refused = {}, threading.Event()
                    proxy.start(stack)
                    host = policy.PolicyHost(host_binary, distribution, home, case,
                        "http://127.0.0.1:" + str(proxy.server_port), ca)
                    stack.callback(host.close)
                    settings_file = host.config / "settings.json"
                    settings = json.loads(settings_file.read_text())
                    settings["requestPolicy"] = {"defaults": {
                        "maxRetries": 0 if mode == "denied" else 4,
                        "maxTotalRetries": 1 if mode == "budget" else 3,
                        "shortWaitMs": 10000, "maxDelayMs": 10000, "maxElapsedMs": 30000, "baseDelayMs": 0}}
                    settings_file.write_text(json.dumps(settings))
                    host.install()
                    accounts = policy.sign_in(host)
                    before = len(proxy.snapshot())
                    command = invocation(host, mode, accounts[0])
                    with (case / "stdout.jsonl").open("w") as stdout, (case / "stderr.log").open("w") as stderr:
                        process = subprocess.Popen(command, env=host.env, cwd=home,
                                                   stdout=stdout, stderr=stderr, start_new_session=True)
                        stack.callback(stop_call, process)
                        if mode in ("cancel", "config", "generation", "disable"):
                            require(proxy.refused.wait(10), "selected read did not reach its response")
                            if mode == "cancel":
                                process.send_signal(signal.SIGINT)
                            elif mode == "config":
                                snapshot = json.loads(host.command("plugin", "sdk", "config", fixture.MODULE))
                                host.command("plugin", "sdk", "config", fixture.MODULE, "{}",
                                             "--expected", snapshot["version"])
                            elif mode == "generation":
                                host.command("plugin", "sdk", "auth", "logout", fixture.MODULE, fixture.GLOBAL, accounts[0])
                            else:
                                host.command("plugin", "sdk", "off", fixture.MODULE)
                        exit_code = process.wait(timeout=20)
                    text = (case / "stdout.jsonl").read_text()
                    diagnostics = (case / "stderr.log").read_text()
                    rows = [json.loads(line) for line in text.splitlines() if line.startswith("{")]
                    calls = proxy.snapshot()[before:]
                    reads = [row for row in calls if row["path"] in (SEARCH, CATALOG)]
                    invokes = [row for row in calls if row["path"] == INVOKE]
                    observed = {"case": mode, "exit": exit_code, "reads": len(reads), "invokes": len(invokes),
                                "accepted": sum(row.get("event") == "accepted" for row in rows)}
                    report["cases"].append(observed)
                    expected_reads = 4 if mode == "paid" else 3 if mode == "budget" else (
                        1 if mode in ("denied", "malformed", "redirect", "cancel", "config", "generation", "disable") else 2)
                    require(len(reads) == expected_reads, "named read dispatch bound failed: " + json.dumps(observed))
                    expected_invokes = 2 if mode == "paid" else 1 if mode in ("invoke", "budget") else 0
                    require(len(invokes) == expected_invokes, "paid work was replayed or lost: " + json.dumps(observed))
                    require(len({row["headers"]["authorization"] for row in reads + invokes}) == 1,
                            "named read switched selected accounts")
                    require(all(proxy.tokens[row["headers"]["authorization"].removeprefix("Bearer ")] == "alice"
                                for row in reads + invokes), "named read changed the explicitly selected account")
                    for path in (SEARCH, CATALOG):
                        require(len({json.dumps(row["body"], sort_keys=True) for row in reads
                                     if row["path"] == path}) <= 1, "host permit changed the named request body")
                    require(not any(row["path"] == "/credential-trap" for row in calls), "named read followed a redirect")
                    require(all(secret not in text + diagnostics for secret in (
                        *proxy.tokens, "private-read-query", "private-read-header", "private-read-message")),
                        "private facts reached host output/diagnostics")
                    success = mode in ("search", "web", "catalog", "schema", "paid")
                    revoked = mode in ("config", "generation", "disable")
                    if not revoked:
                        require((exit_code == 0) == (success or mode in ("denied", "malformed", "redirect")),
                                "ordinary host terminal outcome changed: " + json.dumps(observed))
                    if mode in ("denied", "malformed", "redirect") or revoked and exit_code == 0:
                        result = rows[-1].get("result", {}) if rows else {}
                        require(result.get("status", 0) >= 400 and result.get("pages") == [],
                                "refused/revoked search published success")
                    if mode in ("search", "web"):
                        require("Recovered" in text, "recovered search lost its result")
                    if mode in ("paid", "budget", "invoke"):
                        require(observed["accepted"] == 1, "paid acceptance fence changed")
                    if mode in ("budget", "invoke"):
                        require('"uncertain"' in text or "uncertain" in diagnostics,
                                "accepted failure lost uncertainty")
                    if mode == "budget":
                        require("budget-exhausted" in diagnostics, "shared root budget was not the stopping cause")
                    if revoked:
                        recorded = "revoked" in diagnostics or "changed" in diagnostics
                        observed["revocationDiagnostic"] = recorded
                        if not recorded:
                            report["diagnosticGaps"].append(mode + ": no policy revocation diagnostic")
                        if require_diagnostics:
                            require(recorded, "revocation was not recorded by the host")
                require(not proxy.failures and not proxy.denied and not proxy.connections,
                        "fixture failed or retained connections")
            finally:
                if process is not None and process.poll() is None:
                    stop_call(process)
                if host:
                    (case / "commands.json").write_text(json.dumps(host.commands, indent=2))
                if proxy:
                    (case / "fixture.json").write_text(json.dumps(proxy.snapshot(), indent=2))
                    (case / "fixture-state.json").write_text(json.dumps(
                        {"failures": proxy.failures, "denied": proxy.denied,
                         "openConnections": len(proxy.connections)}, indent=2))
    finally:
        (output / "RESULTS.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", type=Path, required=True)
    parser.add_argument("--worker", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--require-diagnostics", action="store_true")
    parser.add_argument("--modes", nargs="+", default=[
        "search", "web", "catalog", "schema", "denied", "malformed", "redirect", "invoke",
        "paid", "budget", "cancel", "config", "generation", "disable"])
    arguments = parser.parse_args()
    run(arguments.host, arguments.worker, arguments.output, arguments.modes, arguments.require_diagnostics)
