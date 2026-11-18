import argparse
from contextlib import ExitStack
from datetime import datetime, timedelta, timezone
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import threading
import time
import urllib.parse


PROVIDER = "extension-" + hashlib.sha256(b"zcode\0zcode").hexdigest()[:24]
KEYS = ["synthetic-policy-A", "synthetic-policy-B"]


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def run(args):
    root = args.root.resolve()
    source = (args.package or Path(__file__).resolve().parents[1]) / "src"
    artifacts = root / args.label
    artifacts.mkdir()
    observed, failures, balance_reads = [], [], set()
    lock = threading.Lock()
    reached = threading.Event()
    scenario = args.scenario

    class Vendor(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def answer(self, value, status=200, media="application/json", retry=None):
            body = value.encode() if isinstance(value, str) else json.dumps(value).encode()
            self.send_response(status)
            self.send_header("Content-Type", media)
            self.send_header("Content-Length", str(len(body)))
            if retry is not None:
                self.send_header("Retry-After", retry)
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if self.path.startswith("/api/v1/zcode-plan/billing/balance"):
                token = self.headers.get("Authorization", "").removeprefix("Bearer ")
                if token in KEYS:
                    with lock:
                        balance_reads.add(KEYS.index(token))
                self.answer({"code": 0, "data": {
                    "plans": [{"plan_id": "grant", "status": "active"}],
                    "balances": [{"plan_id": "grant", "remaining_units": 10,
                                  "total_units": 10, "capabilities": ["model:GLM-5.3"]}],
                }})
            elif self.path == "/quota":
                self.answer({"code": 0, "data": {"limits": [{
                    "type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 20,
                }]}})
            else:
                failures.append("unexpected GET " + urllib.parse.urlsplit(self.path).path)
                self.answer({}, 404)

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            token = self.headers.get("Authorization", "").removeprefix("Bearer ")
            if token not in KEYS or self.path not in ("/paid", "/start"):
                failures.append("inference destination or selected credential")
                return self.answer({}, 400)
            selected = KEYS.index(token)
            with lock:
                count = sum(item["account"] == selected for item in observed) + 1
                observed.append(dict(account=selected, model=body["model"], path=self.path,
                                     at=time.monotonic()))
            reached.set()
            if scenario == "transport":
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
                return
            if scenario in ("partial", "output"):
                frames = 'event: message_start\ndata: {"type":"message_start","message":{"type":"message","content":[]}}\n\n'
                if scenario == "output":
                    frames += 'event: content_block_delta\ndata: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}\n\n'
                return self.answer(frames, media="text/event-stream")
            if scenario == "unknown":
                return self.answer({"code": 500, "msg": "private vendor message"}, 429)
            if scenario in ("short", "cancel", "revoke-account", "revoke-config", "combined-budget", "fresh-final"):
                if count == 1 and (scenario != "fresh-final" or selected == 0):
                    return self.answer({"code": 3008, "msg": "user concurrency limit exceeded"}, 429, retry="1")
                if scenario == "combined-budget":
                    return self.answer({"code": 3008, "msg": "user concurrency limit exceeded"}, 429, retry="7200")
                if scenario == "fresh-final" and selected == 0:
                    reset = (datetime.now(timezone(timedelta(hours=8))) + timedelta(hours=2)).strftime("%Y-%m-%d %H:%M:%S")
                    return self.answer({"code": 1308, "msg": "您的限额将在 " + reset + " 重置。"})
            elif scenario in ("budget", "all-long") or selected == 0 and body["model"] == "GLM-5.3" and (scenario != "windows" or self.path == "/start"):
                return self.answer({"code": 3008, "msg": "user concurrency limit exceeded"}, 429, retry="7200")
            self.answer({"type": "message", "role": "assistant", "id": "synthetic-response",
                         "model": body["model"], "content": [{"type": "text", "text": "policy answer"}],
                         "stop_reason": "end_turn"})

    report = {"scenario": scenario, "hostSHA256": digest(args.magpie), "status": "running"}
    with ExitStack() as cleanup:
        vendor = ThreadingHTTPServer(("127.0.0.1", 0), Vendor)
        thread = threading.Thread(target=vendor.serve_forever)
        thread.start()

        def stop_vendor():
            vendor.shutdown()
            vendor.server_close()
            thread.join(timeout=5)
            assert not thread.is_alive()

        cleanup.callback(stop_vendor)
        origin = f"http://127.0.0.1:{vendor.server_port}"
        distribution = artifacts / "distribution"
        shutil.copytree(args.distribution, distribution)
        manifest = json.loads((distribution / "magpie-plugin.json").read_text())
        report["productionLibrarySHA256"] = digest(distribution / manifest["entry"])
        entry = artifacts / "entry.go"
        fields = ",".join(name + ":" + json.dumps(origin + path) for name, path in {
            "Zai": "/paid", "BigModel": "/paid", "Start": "/start",
            "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
        }.items())
        entry.write_text(
            'package main\nimport(z "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
            '"github.com/yetone/magpie/pkg/extension/native")\n'
            'func init(){if err:=native.Register(z.NewControlModule(z.ModuleOptions{Endpoints:z.Endpoints{'
            + fields + '}}));err!=nil{panic(err)}}\nfunc main(){}\n')
        overlay = artifacts / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(source / "cmd/native-ui/main.go"): str(entry)}}))
        subprocess.run(["go", "build", "-trimpath", "-overlay", str(overlay), "-buildmode=c-shared",
                        "-o", str(distribution / manifest["entry"]), "./cmd/native-ui"],
                       cwd=source, check=True, timeout=180)
        report.update(fixtureLibrarySHA256=digest(distribution / manifest["entry"]),
                      fixtureEntrySHA256=digest(entry), manifestSHA256=digest(distribution / "magpie-plugin.json"))
        home = artifacts / "home"
        home.mkdir(mode=0o700)
        environment = os.environ.copy()
        environment.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                           XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                           XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                           LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
                           MAGPIE_NO_STATS="1", MAGPIE_PLUGIN_CATALOG="off",
                           MAGPIE_PLUGIN_MARKET="off", MAGPIE_PARTNERS="off", DO_NOT_TRACK="1")
        config = home / "config/magpie"
        cache = home / "cache/magpie"
        config.mkdir(parents=True)
        cache.mkdir(parents=True)
        (config / "settings.json").write_text(json.dumps({
            "noStats": True, "noAutoUpdate": True, "noAccountDiscovery": False, "agentTelemetryOff": True,
        }))
        for name in ("models.json", "labs.json"):
            (cache / name).write_text("{}")

        def cli(*arguments):
            completed = subprocess.run([str(args.magpie), *arguments], env=environment,
                                       text=True, capture_output=True, timeout=40)
            with (artifacts / "cli.log").open("a") as output:
                output.write(completed.stderr)
            assert completed.returncode == 0, (arguments[:3], completed.stderr[-1200:])
            return completed.stdout.strip()

        cli("plugin", "sdk", "add", str(distribution))
        snapshot = json.loads(cli("plugin", "sdk", "config", "zcode"))
        cli("plugin", "sdk", "config", "zcode", json.dumps({"freeFirst": scenario == "accepted"}),
            "--expected", snapshot["version"])
        policy = dict(maxRetries=2, maxTotalRetries=2, baseDelayMs=0, maxDelayMs=7200000,
                      shortWaitMs=0, maxElapsedMs=10000, jitterPercent=0)
        if scenario in ("short", "cancel", "revoke-account", "revoke-config", "combined-budget", "fresh-final"):
            policy["shortWaitMs"] = 2000
        if scenario == "combined-budget":
            policy["maxTotalRetries"] = 1
        if scenario == "budget":
            policy["maxTotalRetries"] = 0
        cli("settings", "set", "requestPolicy", json.dumps({"defaults": policy}))
        accounts = []
        for token in KEYS:
            expiry = int(time.time() * 1000) + 3600000
            credentials = dict(site="zai", kind="individual", userId=token, device="synthetic-device",
                               projectToken=token, expiresAt=expiry, sessionToken=token, sessionExpiresAt=expiry)
            accounts.append(json.loads(cli("plugin", "sdk", "auth", "finish", "zcode", "zcode",
                                            json.dumps({"input": credentials})))["id"])
        order = accounts[:1] if scenario in ("short", "cancel", "revoke-account", "revoke-config") else accounts
        cli("provider", "set", PROVIDER, "routing=order")
        cli("provider", "pool", PROVIDER, "--accounts", ",".join(order))
        if scenario == "windows":
            for account in accounts:
                ranked = [account, *(other for other in accounts if other != account)]
                cli("provider", "pool", PROVIDER, "--accounts", ",".join(ranked))
                cli("provider", "refresh", PROVIDER)
            assert balance_reads == {0, 1}, "both Trial catalogs must be read before testing cached eligibility"
        cli("provider", "pool", PROVIDER, "--accounts", ",".join(order))
        caller = cli("gateway-key", "add", "Policy synthetic caller")
        port = free_port()
        environment["MAGPIE_ADDR"] = f"127.0.0.1:{port}"
        log = cleanup.enter_context((artifacts / "gateway.log").open("w"))
        process = subprocess.Popen([str(args.magpie), "serve"], env=environment,
                                   cwd=home, stdout=log, stderr=subprocess.STDOUT)

        def stop_host():
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)

        cleanup.callback(stop_host)
        deadline = time.monotonic() + 30
        while True:
            try:
                connection = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
                connection.request("GET", "/")
                response = connection.getresponse()
                response.read()
                connection.close()
                break
            except OSError:
                assert process.poll() is None and time.monotonic() < deadline
                time.sleep(0.05)

        def connect(model="GLM-5.3", stream=False):
            connection = http.client.HTTPConnection("127.0.0.1", port, timeout=15)
            connection.request("POST", "/v1/messages", json.dumps({
                "model": PROVIDER + "/" + model, "max_tokens": 16, "stream": stream,
                "messages": [{"role": "user", "content": "policy fixture"}],
            }), {"Content-Type": "application/json", "X-Api-Key": caller})
            return connection

        started = time.monotonic()
        connection = connect("GLM-5.3-Trial" if scenario == "windows" else "GLM-5.3",
                             scenario == "output")
        if scenario in ("cancel", "revoke-account", "revoke-config"):
            assert reached.wait(timeout=5)
            if scenario == "cancel":
                connection.sock.shutdown(socket.SHUT_RDWR)
                connection.close()
            elif scenario == "revoke-account":
                cli("plugin", "sdk", "auth", "logout", "zcode", "zcode", accounts[0])
            else:
                snapshot = json.loads(cli("plugin", "sdk", "config", "zcode"))
                cli("plugin", "sdk", "config", "zcode", '{"freeFirst":true}', "--expected", snapshot["version"])
        status, response_headers = None, {}
        if scenario != "cancel":
            response = connection.getresponse()
            status, response_headers = response.status, dict(response.getheaders())
            (artifacts / "response.bin").write_bytes(response.read())
            connection.close()
        elapsed = time.monotonic() - started
        if scenario == "windows" and status == 200:
            for model in ("GLM-5.3-Trial", "GLM-5.3", "GLM-5.2"):
                followup = connect(model)
                reply = followup.getresponse()
                assert reply.status == 200, ("independent model or cached sibling", model, reply.status)
                reply.read()
                followup.close()
        if scenario == "cancel":
            time.sleep(1.2)
        stop_host()
        labels = [item["account"] for item in observed]
        report.update(status="observed", httpStatus=status, elapsedSeconds=elapsed,
                      sends=list(observed), responseHeaders=response_headers,
                      hostStopped=process.poll() is not None, failures=failures)
        (artifacts / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        expected = [0, 1]
        want = 200
        if scenario == "short":
            expected = [0, 0]
            assert observed[1]["at"] - observed[0]["at"] >= 0.95
        elif scenario == "all-long":
            want = 429
            assert int(response_headers.get("Retry-After", "0")) >= 7190
        elif scenario == "budget":
            expected, want = [0], 429
        elif scenario == "combined-budget":
            expected, want = [0, 0], 429
        elif scenario == "fresh-final":
            expected = [0, 0, 1]
        elif scenario == "windows":
            expected = [0, 1, 1, 0, 0]
        elif scenario in ("accepted", "partial", "output", "unknown", "transport"):
            expected, want = [0], 409 if scenario != "output" else 200
        elif scenario in ("cancel", "revoke-account", "revoke-config"):
            expected = [0]
            want = status
            assert scenario == "cancel" or status >= 400
        assert labels == expected, ("dispatch/root-budget/no-replay boundary", labels, expected)
        assert status == want and elapsed < 6, (status, want, elapsed)
        assert not failures and process.poll() is not None
        diagnostics = [line for line in (artifacts / "gateway.log").read_text().splitlines()
                       if "request policy" in line]
        assert not any(token in "\n".join(diagnostics) for token in KEYS)
        report["policyDiagnostics"] = diagnostics
        report["status"] = "passed"
        (artifacts / "result.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", required=True, type=Path)
    parser.add_argument("--magpie", required=True, type=Path)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--package", type=Path)
    parser.add_argument("--label", required=True)
    parser.add_argument("--scenario", default="long")
    run(parser.parse_args())
