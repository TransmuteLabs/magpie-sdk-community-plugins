import contextlib
import hashlib
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
import zipfile


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def main():
    root = Path(sys.argv[1]).resolve()
    source = Path(__file__).resolve().parents[2]
    host = root / "magpie-host"
    run = Path(tempfile.mkdtemp(prefix="installed-proof-", dir=root))
    home = run / "home"
    home.mkdir(mode=0o700)
    env = os.environ.copy()
    for name in list(env):
        if name.lower().endswith("_proxy") or name in {
            "MAGPIE_ADDR", "MAGPIE_WEB_KEY", "MAGPIE_UPDATE_FEED",
            "ZCODE_DATA_BASE_DIR", "ZCODE_PERSONAL_PROVIDER_CONFIG_FILE",
        }:
            env.pop(name)
    assert shutil.which("zcode", path=env["PATH"]) is None
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
               XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
               LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home))
    observed, errors = [], []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def send(self, body, media="application/json", status=200, identity=None):
            body = body.encode()
            self.send_response(status)
            self.send_header("Content-Type", media)
            self.send_header("Content-Length", str(len(body)))
            if identity:
                self.send_header("Request-Id", "header-fallback")
                self.send_header("X-Request-Id", "vendor-request-" + identity)
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if self.path == "/quota":
                self.send(json.dumps({"code": 0, "data": {"limits": [{
                    "type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 20,
                    "usage": 10.25, "remaining": 10.25, "nextResetTime": 2000000000000,
                }]}}))
            elif self.path == "/updates":
                self.send("{}", status=404)
            else:
                errors.append("unexpected GET " + self.path)
                self.send("{}", status=404)

        def do_POST(self):
            request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            mode = request["messages"][-1]["content"].removeprefix("private-input-canary:")
            observed.append(mode)
            if self.path != "/paid" or self.headers.get("Authorization") != "Bearer private-credential-canary":
                errors.append("destination or credential channel")
            message = {
                "type": "message", "id": "vendor-response-" + mode, "role": "assistant",
                "model": "GLM-5.3", "content": [{"type": "text", "text": "private-output-canary"}],
                "stop_reason": "end_turn",
                "usage": {"input_tokens": 0, "output_tokens": 5, "cache_read_input_tokens": 11,
                          "cache_creation_input_tokens": 3,
                          "cache_creation": {"ephemeral_5m_input_tokens": 1, "ephemeral_1h_input_tokens": 2},
                          "completion_tokens_details": {"reasoning_tokens": 0}, "service_tier": "standard"},
            }
            if mode.startswith("currency-"):
                message["usage"] = {"input_tokens": 0, "cost": 12, "cost_units": 3, "credits": 4,
                                    "currency": mode.removeprefix("currency-")}
            elif mode == "unknown":
                del message["usage"]
            elif mode == "rejected":
                message = {"type": "error", "request_id": "vendor-request-rejected",
                           "usage": {"input_tokens": 0},
                           "error": {"type": "overloaded_error", "message": "private-error-canary"}}
            if mode not in {"complete", "truncated", "error", "zero"}:
                return self.send(json.dumps(message), identity=mode)
            message.pop("stop_reason")
            message["usage"] = {"input_tokens": 7, "output_tokens": 0,
                                "cache_creation": {"ephemeral_5m_input_tokens": 1, "ephemeral_1h_input_tokens": 2}}
            frames = [("message_start", {"type": "message_start", "message": message}),
                      ("message_delta", {"type": "message_delta", "usage": {"output_tokens": 5, "reasoning_tokens": 2}})]
            if mode == "error":
                frames.append(("error", {"type": "error", "request_id": "vendor-request-error",
                                        "error": {"type": "overloaded_error", "message": "private-error-canary"}}))
            elif mode == "zero":
                frames.append(("message_delta", {"type": "message_delta", "usage": {"output_tokens": 0, "reasoning_tokens": 0}}))
            if mode in {"complete", "zero"}:
                frames.extend([("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn"}}),
                               ("message_stop", {"type": "message_stop"})])
            self.send("".join("event: " + name + "\ndata: " + json.dumps(value) + "\n\n"
                              for name, value in frames), "text/event-stream", identity=mode)

    with contextlib.ExitStack() as cleanup:
        vendor = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        vendor.daemon_threads = True
        thread = threading.Thread(target=vendor.serve_forever, daemon=True)
        thread.start()
        cleanup.callback(vendor.server_close)
        cleanup.callback(vendor.shutdown)
        origin = "http://127.0.0.1:" + str(vendor.server_port)
        env["MAGPIE_UPDATE_FEED"] = origin + "/updates"
        distribution = run / "distribution"
        asset = source.parent / "dist/magpie-plugin_linux_amd64.zip"
        with zipfile.ZipFile(asset) as archive:
            archive.extractall(distribution)
        manifest = json.loads((distribution / "magpie-plugin.json").read_bytes())
        assert manifest["contract"]["version"] == {"major": 1, "minor": 9}
        assert len(manifest["agents"]) == 1 and manifest["window"]
        assert manifest["providers"][0]["maxConcurrency"] == 3 and manifest["providers"][0]["maxAccounts"] == 2
        production_library = hashlib.sha256((distribution / manifest["entry"]).read_bytes()).hexdigest()
        entry = run / "entry.go"
        fields = ",".join(name + ":" + json.dumps(origin + path) for name, path in {
            "Zai": "/paid", "BigModel": "/paid", "Start": "/paid",
            "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
        }.items())
        entry.write_text(
            'package main\nimport(z "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
            '"github.com/yetone/magpie/pkg/extension/native")\n'
            'func init(){if err:=native.Register(z.NewControlModule(z.ModuleOptions{Endpoints:z.Endpoints{'
            + fields + '}}));err!=nil{panic(err)}}\nfunc main(){}\n')
        overlay = run / "endpoints-overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(source / "cmd/native-ui/main.go"): str(entry)}}))
        subprocess.run(["go", "build", "-trimpath", "-overlay", str(overlay), "-buildmode=c-shared",
                        "-o", str(distribution / manifest["entry"]), "./cmd/native-ui"],
                       cwd=source, env=env, check=True, timeout=180)
        fixture_library = hashlib.sha256((distribution / manifest["entry"]).read_bytes()).hexdigest()

        def cli(*arguments, target_env=env):
            result = subprocess.run([str(host), *arguments], cwd=run, env=target_env, text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=45)
            with (run / "cli.log").open("a") as output:
                output.write(result.stderr)
            assert result.returncode == 0, (arguments[:3], result.returncode, result.stderr[-1500:])
            return result.stdout.strip()

        canonical_home = run / "canonical-home"
        canonical_home.mkdir(mode=0o700)
        canonical_env = env.copy()
        canonical_env.update(HOME=str(canonical_home), USERPROFILE=str(canonical_home),
                             XDG_CONFIG_HOME=str(canonical_home / "config"), XDG_CACHE_HOME=str(canonical_home / "cache"),
                             XDG_DATA_HOME=str(canonical_home / "data"), XDG_STATE_HOME=str(canonical_home / "state"),
                             APPDATA=str(canonical_home / "appdata"), LOCALAPPDATA=str(canonical_home / "localappdata"),
                             MAGPIE_TEST_SANDBOX=str(canonical_home))
        cli("plugin", "sdk", "add", str(root / "full-artifact"), target_env=canonical_env)
        canonical_entries = json.loads(cli("plugin", "sdk", "list", "--json", target_env=canonical_env))
        canonical_entry = json.loads(cli("plugin", "sdk", "inspect", "zcode", target_env=canonical_env))
        assert len(canonical_entries) == 1 and canonical_entry["manifest"]["version"] == "0.13.1"
        canonical_native = json.loads(cli("plugin", "sdk", "call", "zcode", "sdk.describe", "{}", target_env=canonical_env).splitlines()[-1])
        assert canonical_native["result"]["version"] == "0.13.1" and not canonical_native.get("error"), canonical_native
        cli("plugin", "sdk", "off", "zcode", target_env=canonical_env)
        cli("plugin", "sdk", "add", str(distribution))
        config = json.loads(cli("plugin", "sdk", "config", "zcode"))
        cli("plugin", "sdk", "config", "zcode", json.dumps({"freeFirst": False}), "--expected", config["version"])
        account = json.loads(cli("plugin", "sdk", "auth", "finish", "zcode", "zcode",
                                 json.dumps({"input": {"site": "zai", "kind": "legacy-key",
                                                       "key": "private-credential-canary"}})))["id"]
        key = cli("gateway-key", "add", "observability-fixture")
        assert key and "\n" not in key
        provider = "extension-" + hashlib.sha256(b"zcode\0zcode").hexdigest()[:24]
        gateway_port, web_port = free_port(), free_port()
        env.update(MAGPIE_ADDR=f"127.0.0.1:{gateway_port}", MAGPIE_WEB_KEY="synthetic-observability-web")
        web_origin = f"http://127.0.0.1:{web_port}"
        log = cleanup.enter_context((run / "web.log").open("wb"))
        process = subprocess.Popen([str(host), "web", "--addr", f"127.0.0.1:{web_port}", "--no-open"],
                                   cwd=run, env=env, stdout=log, stderr=log)

        def stop():
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)

        cleanup.callback(stop)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                             urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        deadline = time.monotonic() + 40
        while True:
            try:
                opener.open(web_origin + "/?k=synthetic-observability-web", timeout=2).read()
                opener.open(f"http://127.0.0.1:{gateway_port}/v1/models",
                            timeout=2).close()
                break
            except urllib.error.HTTPError as failure:
                if failure.code == 401:
                    break
                raise
            except OSError:
                assert process.poll() is None and time.monotonic() < deadline, (run / "web.log").read_text()[-3000:]
                time.sleep(0.1)

        def records():
            return [json.loads(line) for path in home.rglob("usage.jsonl")
                    for line in path.read_text().splitlines() if line]

        cards = json.loads(opener.open(web_origin + "/api/usage/quotas", timeout=30).read())
        card = next(value for value in cards if value["provider"] == provider and value["user"] == account)
        window = next(value for value in card["windows"] if value["name"] == "5 hours")
        assert window["used"] == 0 and window["amount"] == 0 and window["limit"] == 10.25, window
        assert window["display"] == "0 / 10.25" and window["resetsAt"].startswith("2033-05-18T03:33:20"), window
        assert window["startsAt"].startswith("2033-05-17T22:33:20"), window
        assert not any(name in window for name in ["unit", "balance", "currency", "cost", "credits"]), window

        modes = ["json", "complete", "truncated", "error", "zero",
                 "currency-USD", "currency-EUR", "currency-CNY", "currency-credits", "unknown", "rejected"]
        results = []
        for index, mode in enumerate(modes):
            request = urllib.request.Request(
                f"http://127.0.0.1:{gateway_port}/v1/messages",
                data=json.dumps({"model": provider + "/GLM-5.3", "max_tokens": 16,
                                 "stream": mode in {"complete", "truncated", "error", "zero"},
                                 "messages": [{"role": "user", "content": "private-input-canary:" + mode}]}).encode(),
                headers={"Content-Type": "application/json", "X-Api-Key": key,
                         "X-Session-Id": "observability-" + mode})
            try:
                response = opener.open(request, timeout=30)
            except urllib.error.HTTPError as failure:
                response = failure
            with response:
                status, body = response.status, response.read()
            (run / (mode + "-response.bin")).write_bytes(body)
            deadline = time.monotonic() + 5
            while True:
                rows = [row for row in records() if row.get("provider") == provider]
                if len(rows) >= index + 1:
                    break
                assert time.monotonic() < deadline, ("missing ledger execution", mode, rows)
                time.sleep(0.05)
            assert len(rows) == index + 1, ("duplicate ordinary ledger execution", mode, rows)
            row = rows[-1]
            assert row["providerAccount"] == account and row["rid"] == "vendor-request-" + mode, (mode, row)
            assert not any(name in row for name in ["reported_cost", "credits", "cost", "upstream"]), (mode, row)
            if mode == "json":
                assert [row.get(name, 0) for name in ("in", "out", "cache_read", "cache_write",
                                                    "cache_write_1h", "reasoning")] == [0, 5, 11, 3, 2, 0], row
                assert row.get("tier_served") == "standard", row
            elif mode in {"complete", "truncated", "error", "zero"}:
                output, reasoning = (0, 0) if mode == "zero" else (5, 2)
                assert [row.get(name, 0) for name in ("in", "out", "cache_write",
                                                    "cache_write_1h", "reasoning")] == [7, output, 3, 2, reasoning], row
            else:
                assert [row.get(name, 0) for name in ("in", "out", "cache_read", "cache_write",
                                                    "cache_write_1h", "reasoning")] == [0, 0, 0, 0, 0, 0], row
            failed = mode in {"truncated", "error", "rejected"}
            assert bool(row.get("err")) == failed, (mode, row)
            if mode != "rejected":
                assert row.get("served") == "GLM-5.3" and row.get("upstream_response_id") == "vendor-response-" + mode, row
            if failed:
                assert row["status"] == 409 and row.get("err_type") == "extension_operation_terminal" and not row.get("stop"), row
                assert b"private-error-canary" not in body
            results.append({"mode": mode, "httpStatus": status, "ledger": row})
        diagnostics = [line for line in (run / "web.log").read_text().splitlines()
                       if "extension diagnostic:" in line and "phase=execute " in line]
        assert len(diagnostics) == len(modes), ("missing installed execution diagnostics", diagnostics)
        expected_codes = ["execution.completed", "execution.completed", "execute.uncertain", "execute.uncertain",
                          *["execution.completed"] * 6, "execution.rejected"]
        for line, code in zip(diagnostics, expected_codes, strict=True):
            assert 'module="zcode"' in line and 'provider="zcode"' in line and f'account="{account}"' in line, line
            assert re.search(r'operation="[^"]+"', line) and "code=" + code + " " in line, line
            assert "private-" not in line and "vendor-request-" not in line and key not in line, line
        assert observed == modes and not errors, (observed, errors)
        catalogs = [json.loads(path.read_bytes()) for path in home.rglob("catalog.json")]
        assert sum(len(value) for value in catalogs if isinstance(value, list)) == 1
        assert not list(home.rglob("node_modules"))
        cli("plugin", "sdk", "off", "zcode")
        source_identity = json.loads((root / "sdk-source.json").read_bytes())
        report = {
            "passed": True, "sourceSDKTree": source_identity.get("tree"),
            "sourceSDKSnapshotSHA256": source_identity.get("snapshotSHA256"),
            "hostSHA256": hashlib.sha256(host.read_bytes()).hexdigest(),
            "artifactSHA256": hashlib.sha256(asset.read_bytes()).hexdigest(),
            "productionLibrarySHA256": production_library, "endpointFixtureLibrarySHA256": fixture_library,
            "runDirectory": str(run), "ordinaryExecutions": len(results), "results": results,
            "executionDiagnostics": diagnostics, "actualClientPresent": False,
            "vendorIsLoopbackFixture": True, "overlayChangesDestinationsOnly": True,
            "unsupportedMonetaryFieldsRemainUnknown": True, "failedTerminalRetainsUsage": True,
            "finalMeasuredZeroReplacesEarlierCounters": True,
            "failedExecutionLedgerTypeIsHostOperationTerminal": True,
            "originalVendorErrorTypeVerifiedInModuleEventOnly": True,
            "unmodifiedCanonicalPackageInstallListInspectDescribeOff": True,
            "richQuotaCard": window, "providerDefaults": {"maxConcurrency": 3, "maxAccounts": 2},
        }
        (root / "installed-proof.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report, indent=2), flush=True)


if __name__ == "__main__":
    main()
