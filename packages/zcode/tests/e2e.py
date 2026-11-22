import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
import zipfile


def main():
    source = Path(__file__).resolve().parents[1] / "src"
    host = str(Path(sys.argv[1]).resolve())
    headless = "--headless" in sys.argv[2:]
    guard = threading.Lock()
    observed = {"mints": 0, "logins": 0, "inferences": 0, "tickets": 0, "native": 0, "settlements": 0,
                "mode": "success", "resets": 0, "mcpReads": 0,
                "remoteInit": 0, "remoteList": 0, "remoteCalls": 0, "remoteIDs": set(), "errors": []}
    expiry = int(time.time()) + 600

    class Vendor(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            if self.path == "/api/v1/coding-plan/reset/status":
                payload = json.dumps({"code": 0, "data": {
                    "available_five_hour_resets": [{"expire_at": expiry * 1000}],
                    "available_week_resets": [], "has_unread_history": False,
                    "latest_five_hour_reset_history": None, "latest_week_reset_history": None,
                }}).encode()
            elif self.path == "/quota":
                payload = json.dumps({"code": 0, "data": {"limits": [{
                    "type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 20,
                }]}}).encode()
            elif self.path == "/api/v1/mcp/usage":
                with guard:
                    observed["mcpReads"] += 1
                    if (self.headers.get("Authorization") != "Bearer synthetic-session"
                            or self.headers.get("X-Bigmodel-Authorization") != "Bearer synthetic-business-new"
                            or self.headers.get("Bigmodel-Target-Type") != "PERSONAL"
                            or self.headers.get("X-Coding-Plan-Api-Key") is not None
                            or self.headers.get("X-Api-Key") is not None):
                        observed["errors"].append("official MCP selected JWT channel")
                payload = json.dumps({"code": 0, "data": {
                    "server_time": int(time.time()),
                    "total_usage": {"used": 2, "limit": 10, "remaining": 8},
                }}).encode()
            elif self.path == "/api/v1/oauth/profile":
                if self.headers.get("Authorization") != "Bearer synthetic-session":
                    with guard:
                        observed["errors"].append("native profile credential role")
                payload = json.dumps({"code": 0, "data": {"user": {"user_id": "synthetic-user"}}}).encode()
            else:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def do_POST(self):
            body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
            if self.path == "/api/v1/mcp/server/image_search":
                self.remote_mcp(json.loads(body))
                return
            with guard:
                if self.path == "/api/auth/z/login":
                    observed["logins"] += 1
                    if json.loads(body) != {"token": "synthetic-bootstrap"} or self.headers.get("Authorization") is not None:
                        observed["errors"].append("business bootstrap boundary")
                    status = 200
                    answer = {"code": 0, "data": {
                        "access_token": "synthetic-business-new", "expires_in": 1200,
                    }}
                elif self.path.endswith("/access_tokens"):
                    observed["mints"] += 1
                    if (self.path != "/api/biz/v1/organization/org/projects/project/api_keys/key-id/access_tokens"
                            or self.headers.get("Authorization") != "Bearer synthetic-business-new"
                            or self.headers.get("X-Api-Key") is not None
                            or json.loads(body) != {"clientType": "zcode", "clientVersion": "3.18.2"}):
                        observed["errors"].append("issuer boundary")
                    status = 200 if observed["mode"] == "success" else 503
                    answer = {"code": 0, "data": {
                        "accessToken": "synthetic-replacement", "tokenType": "Bearer",
                        "expiresIn": 600, "expiresAt": expiry,
                    }}
                elif self.path == "/zai":
                    observed["inferences"] += 1
                    if (self.headers.get("Authorization") != "Bearer synthetic-replacement"
                            or self.headers.get("X-Api-Key") != "synthetic-replacement"):
                        observed["errors"].append("replacement credentials not used")
                    status = 200
                    answer = {"type": "message", "content": [{"type": "text", "text": "fixture-ok"}]}
                elif self.path == "/api/v1/off-peak/ticket":
                    observed["tickets"] += 1
                    if json.loads(body) != {"task_id": "native-session"}:
                        observed["errors"].append("alias-derived task")
                    status = 200
                    answer = {"ticket_id": "synthetic-ticket", "task_id": "native-session", "state": "ready"}
                elif self.path == "/api/v1/off-peak/anthropic/v1/messages":
                    observed["native"] += 1
                    if (self.headers.get("Authorization") != "Bearer synthetic-session"
                            or self.headers.get("X-Coding-Plan-Api-Key") != "synthetic-replacement"
                            or self.headers.get("X-Off-Peak-Ticket-ID") != "synthetic-ticket"
                            or json.loads(body)["model"] != "GLM-5.3"):
                        observed["errors"].append("native credential/model/ticket binding")
                    status = 200
                    answer = {"type": "message", "content": [{"type": "text", "text": "native-fixture"}]}
                elif self.path == "/api/v1/off-peak/ticket/synthetic-ticket/settle":
                    observed["settlements"] += 1
                    status = 200
                    answer = {"ticket_id": "synthetic-ticket", "task_id": "native-session", "state": "settled"}
                elif self.path == "/api/v1/coding-plan/reset/use":
                    observed["resets"] += 1
                    status, answer = 503, {"code": 503}
                else:
                    observed["errors"].append("unexpected target")
                    status, answer = 404, {}
            payload = json.dumps(answer).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def remote_mcp(self, message):
            method, identity = message.get("method"), message.get("id")
            with guard:
                if (message.get("jsonrpc") != "2.0"
                        or self.headers.get("Authorization") != "Bearer synthetic-session"
                        or self.headers.get("X-Bigmodel-Authorization") != "Bearer synthetic-business-new"
                        or self.headers.get("Bigmodel-Target-Type") != "PERSONAL"
                        or self.headers.get("X-Api-Key") is not None
                        or self.headers.get("X-Coding-Plan-Api-Key") is not None):
                    observed["errors"].append("remote MCP selected credential/protocol boundary")
                if identity is not None:
                    if identity in observed["remoteIDs"]:
                        observed["errors"].append("remote JSON-RPC ID replay")
                    observed["remoteIDs"].add(identity)
                if method != "initialize" and (
                        self.headers.get("Mcp-Session-Id") != "native-owned-session"
                        or self.headers.get("MCP-Protocol-Version") != "2025-06-18"):
                    observed["errors"].append("remote MCP session ownership")
                if method == "initialize":
                    observed["remoteInit"] += 1
                    if message["params"]["protocolVersion"] != "2025-06-18" or self.headers.get("Mcp-Session-Id"):
                        observed["errors"].append("remote initialize pin/session")
                    result = {"protocolVersion": "2025-06-18", "capabilities": {"tools": {}},
                              "serverInfo": {"name": "native-fixture", "version": "1"}}
                elif method == "notifications/initialized":
                    if identity is not None:
                        observed["errors"].append("remote notification acquired ID")
                    self.send_response(202)
                    self.send_header("Content-Length", "0")
                    self.end_headers()
                    return
                elif method == "tools/list":
                    observed["remoteList"] += 1
                    result = {"tools": [
                        {"name": "new_dynamic_tool", "inputSchema": {"type": "object"}},
                        {"name": "search_image", "inputSchema": {"type": "object",
                            "properties": {"query": {"type": "string"}}, "required": ["query"]}},
                    ]}
                elif method == "tools/call":
                    observed["remoteCalls"] += 1
                    if message["params"] != {"name": "search_image", "arguments": {"query": "synthetic image"}}:
                        observed["errors"].append("remote fixed tool/arguments")
                    result = {"content": [{"type": "text", "text": '{"remoteImage":"fixture"}'}],
                              "structuredContent": {"large": 9007199254740993},
                              "_meta": {"fixture": "retained"}}
                    payload = json.dumps({"jsonrpc": "2.0", "id": identity, "result": result}).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.end_headers()
                    self.wfile.write(b': heartbeat\n\ndata: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}\n\n')
                    self.wfile.write(b"event: message\ndata: " + payload + b"\n\n")
                    self.wfile.flush()
                    return
                else:
                    observed["errors"].append("remote unknown method")
                    self.send_error(404)
                    return
            payload = json.dumps({"jsonrpc": "2.0", "id": identity, "result": result}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            if method == "initialize":
                self.send_header("Mcp-Session-Id", "native-owned-session")
            self.end_headers()
            self.wfile.write(payload)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Vendor)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="zcode-e2e-") as scratch:
            root = Path(scratch)
            home = root / "home"
            home.mkdir()
            env = os.environ.copy()
            caches = json.loads(subprocess.check_output([
                "go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"
            ]))
            env.update(caches)
            env.update(HOME=str(home), USERPROFILE=str(home),
                       XDG_CONFIG_HOME=str(home / ".config"), XDG_CACHE_HOME=str(home / ".cache"),
                       XDG_DATA_HOME=str(home / ".local/share"), XDG_STATE_HOME=str(home / ".local/state"),
                       APPDATA=str(home / "appdata"), LOCALAPPDATA=str(home / "localappdata"),
                       MAGPIE_TEST_SANDBOX=str(home), GOTOOLCHAIN="local",
                       GOPROXY="off", GOSUMDB="off", CGO_ENABLED="1")
            for key in list(env):
                if key.lower().endswith("_proxy") or key in [
                    "CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME", "MAGPIE_UPDATE_FEED"
                ]:
                    env.pop(key, None)
            declaration = json.loads(subprocess.check_output(
                ["go", "run", "./cmd/manifest", *(["--headless"] if headless else [])], cwd=source, env=env
            ))
            origin = f"http://127.0.0.1:{server.server_port}"
            endpoints = {
                "Zai": origin + "/zai", "BigModel": origin + "/bigmodel",
                "Start": origin + "/start", "ZaiQuota": origin + "/quota",
                "BigModelQuota": origin + "/quota", "LegacyBigModelQuota": origin + "/quota",
            }
            fields = ",".join(key + ":" + json.dumps(value) for key, value in endpoints.items())
            entry = root / "entry.go"
            entry.write_text(
                'package main\nimport (zcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
                '"github.com/yetone/magpie/pkg/extension/native")\n'
                'func init(){if err:=native.Register(zcode.' +
                ('NewModule' if headless else 'NewControlModule') + '(zcode.ModuleOptions{Endpoints:zcode.Endpoints{'
                + fields + '}}));err!=nil{panic(err)}}\nfunc main(){}\n'
            )
            overlay = root / "overlay.json"
            command = "./cmd/native" if headless else "./cmd/native-ui"
            overlay.write_text(json.dumps({"Replace": {str(source / command / "main.go"): str(entry)}}))
            distribution = root / "distribution"
            if not headless:
                with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
                    archive.extractall(distribution)
            library = distribution / declaration["entry"]
            library.parent.mkdir(parents=True, exist_ok=True)
            subprocess.run([
                "go", "build", "-overlay", str(overlay), "-buildmode=c-shared",
                "-o", str(library), command
            ], cwd=source, env=env, check=True)
            (distribution / "magpie-plugin.json").write_text(json.dumps(declaration))

            def cli(*args, succeeds=True):
                result = subprocess.run([host, "plugin", "sdk", *args], env=env,
                                        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                        timeout=60)
                if (result.returncode == 0) != succeeds:
                    raise AssertionError(f"SDK {args[0]} unexpected exit {result.returncode}: {result.stderr[-800:]}")
                return result

            cli("add", str(distribution))
            now = int(time.time() * 1000)
            credentials = {
                "site": "zai", "kind": "individual", "userId": "synthetic-user",
                "device": "synthetic-device", "projectToken": "synthetic-old",
                "expiresAt": now + 3600000, "businessToken": "synthetic-business",
                "businessExpiresAt": now - 60000,
                "oauthToken": "synthetic-bootstrap", "oauthExpiresAt": now + 7200000,
                "sessionToken": "synthetic-session", "sessionExpiresAt": now + 3600000,
                "location": {"organizationId": "org", "projectId": "project", "apiKeyId": "key-id"},
            }
            imported = json.loads(cli("auth", "finish", "zcode", "zcode",
                                      json.dumps({"input": credentials})).stdout)
            identity = imported["id"]
            before = json.loads(cli("accounts", "zcode", "zcode", "--json").stdout)
            assert before[0]["generation"] == 1
            refreshed = json.loads(cli("auth", "refresh", "zcode", "zcode", identity).stdout)
            assert refreshed["expiresAt"] == expiry * 1000
            assert "credentials" not in refreshed
            after = json.loads(cli("accounts", "zcode", "zcode", "--json").stdout)
            assert after[0]["generation"] == 2

            def tool(name, arguments, operation):
                scope = json.dumps({"provider": "zcode", "account": identity, "operation": operation})
                invoked = cli("call", "zcode", "tool.call", json.dumps({
                    "id": name, "arguments": arguments,
                }), "--scope", scope)
                frames = [json.loads(line) for line in invoked.stdout.splitlines()]
                result = next(frame["result"] for frame in frames if frame.get("kind") == "result")
                assert not result.get("isError", False) and result["content"][0]["type"] == "text"
                return json.loads(result["content"][0]["text"])

            account_status = tool("zcode_account_status", {}, "tool-account-proof")
            assert account_status["account"] == identity and account_status["generation"] == 2
            mcp = tool("zcode_mcp_usage", {}, "tool-mcp-proof")
            assert mcp["informational"] and mcp["window"]["aside"] and mcp["window"]["used"] == 20
            searched = tool("zcode_image_search", {"query": "synthetic image"}, "tool-remote-image-proof")
            assert searched == {"remoteImage": "fixture"}
            remote_scope = json.dumps({"provider": "zcode", "account": identity, "operation": "tool-remote-image-proof"})
            cli("call", "zcode", "tool.call", json.dumps({"id": "zcode_image_search",
                "arguments": {"query": "synthetic image"}}), "--scope", remote_scope, succeeds=False)
            unknown_scope = json.dumps({"provider": "zcode", "account": identity, "operation": "tool-undeclared-proof"})
            cli("call", "zcode", "tool.call", json.dumps({"id": "new_dynamic_tool",
                "arguments": {}}), "--scope", unknown_scope, succeeds=False)
            history_source = b'{"type":"assistant","sessionId":"offline-fixture","opaque":9007199254740993,"message":{"role":"assistant","content":[{"type":"thinking","signature":"synthetic-signature"}]}}\r\n'
            encoded_history = base64.b64encode(history_source).decode()
            inspected = tool("zcode_history_inspect", {"format": "zcode-native-jsonl", "body": encoded_history},
                             "tool-history-inspect")
            assert inspected["summary"]["records"] == 1 and inspected["summary"]["hasThinking"]
            assert not inspected["resumeAuthorized"]
            converted = tool("zcode_history_convert", {"format": "zcode-native-jsonl",
                             "body": encoded_history, "to": "zcode-history-bundle-v1"},
                             "tool-history-convert")
            bundle = json.loads(base64.b64decode(converted["body"]))
            assert converted["sourcePreserved"] and not converted["resumeAuthorized"]
            assert base64.b64decode(bundle["source"]) == history_source
            diagnostic = b'{"timestamp":"2026-11-14T00:00:00Z","level":"info","sessionId":"offline-fixture","context":{"rawUsage":9007199254740993}}\n{"timestamp":'
            inspected_log = tool("zcode_history_inspect", {
                "format": "zcode-cli-log-jsonl", "body": base64.b64encode(diagnostic).decode(),
            }, "tool-diagnostic-inspect")
            assert inspected_log["summary"]["records"] == 1 and inspected_log["summary"]["incompleteTail"]
            assert not inspected_log["resumeAuthorized"]

            account_files = list(home.rglob("accounts.json"))
            assert len(account_files) == 1

            def record():
                records = json.loads(account_files[0].read_text())["accounts"]
                return next(value for value in records if value["id"] == identity)

            current = record()
            assert current["generation"] == 2 and current["expiresAt"] == expiry * 1000
            assert current["auth"]["projectToken"] == "synthetic-replacement"
            assert current["auth"]["businessToken"] == "synthetic-business-new"
            assert current["auth"]["expiresAt"] == current["expiresAt"]
            assert current["refresh"]["state"] == "completed"
            assert current["refresh"]["toGeneration"] == 2
            assert current["refresh"]["receipt"]["expiresAt"] == current["expiresAt"]
            assert "credentials" not in current["refresh"]["receipt"]
            request = {
                "protocol": "anthropic", "model": "GLM-5.3",
                "body": base64.b64encode(b'{"model":"GLM-5.3","messages":[]}').decode(),
                "options": {"method": "POST", "path": "/v1/messages", "query": ""},
            }
            selected = json.dumps({"provider": "zcode", "account": identity,
                                   "operation": "native-inference-proof", "session": "fixture-session"})
            executed = cli("call", "zcode", "provider.execute", json.dumps(request), "--scope", selected)
            assert "completed" in executed.stdout
            config = json.loads(cli("config", "zcode").stdout)
            data_dir = home / "zcode-controller"
            cli("config", "zcode", json.dumps({"dataDir": str(data_dir)}), "--expected", config["version"])
            assert not data_dir.exists()
            native_request = dict(request)
            native_request["model"] = "GLM-5.3-OffPeak"
            native_request["body"] = base64.b64encode(b'{"model":"GLM-5.3-OffPeak","messages":[]}').decode()
            native_scope = {"provider": "zcode", "account": identity,
                            "operation": "native-controller-1", "session": "native-session"}
            for operation in ["native-controller-1", "native-controller-2"]:
                native_scope["operation"] = operation
                invoked = cli("call", "zcode", "provider.execute", json.dumps(native_request),
                              "--scope", json.dumps(native_scope))
                assert "completed" in invoked.stdout
            listed = cli("call", "zcode", "module.call", json.dumps({
                "action": "execution.list", "input": {},
            }), "--scope", selected)
            frames = [json.loads(line) for line in listed.stdout.splitlines()]
            entries = next(frame["result"] for frame in frames if frame.get("kind") == "result")
            assert len(entries) == 1
            for operation in ["settlement-1", "settlement-2"]:
                settled_scope = {"provider": "zcode", "account": identity, "operation": operation}
                canceled = cli("call", "zcode", "operation.cancel", json.dumps({
                    "operationId": "native-controller-1", "reference": {"reference": entries[0]["reference"]},
                }), "--scope", json.dumps(settled_scope))
                canceled_frames = [json.loads(line) for line in canceled.stdout.splitlines()]
                cancel_result = next(frame["result"] for frame in canceled_frames if frame.get("kind") == "result")
                assert cancel_result["operationId"] == "native-controller-1"
            control_scope = dict(provider="zcode", account=identity, operation="reset-proof")
            cli("call", "zcode", "module.call", json.dumps({
                "action": "reset.use", "input": {"window": "FIVE_HOUR"},
            }), "--scope", json.dumps(control_scope), succeeds=False)
            control_scope["operation"] = "quota-blocked-inference"
            cli("call", "zcode", "provider.execute", json.dumps(request),
                "--scope", json.dumps(control_scope), succeeds=False)
            with guard:
                assert observed["inferences"] == 1 and observed["resets"] == 1
            deferred = cli("call", "zcode", "provider.usage", "{}",
                           "--scope", json.dumps(control_scope))
            usage_frames = [json.loads(line) for line in deferred.stdout.splitlines()]
            quota = next(frame["result"] for frame in usage_frames if frame.get("kind") == "result")
            assert quota["readDeferred"] and quota["revision"] == "1"
            control_scope["operation"] = "still-quota-blocked"
            cli("call", "zcode", "provider.execute", json.dumps(request),
                "--scope", json.dumps(control_scope), succeeds=False)
            control_scope["operation"] = "reset-resolution"
            cli("call", "zcode", "module.call", json.dumps({
                "action": "reset.resolve", "input": {"acknowledgment": "vendor-may-have-spent-reset"},
            }), "--scope", json.dumps(control_scope))
            observation = tool("zcode_usage", {}, "tool-informational-usage")
            assert observation["informational"] and observation["report"]["asOf"] > 0
            assert not observation["report"].get("revision")
            control_scope["operation"] = "tool-cannot-ack-quota"
            cli("call", "zcode", "provider.execute", json.dumps(request),
                "--scope", json.dumps(control_scope), succeeds=False)
            refreshed_usage = cli("call", "zcode", "provider.usage", "{}",
                                  "--scope", json.dumps(control_scope))
            usage_frames = [json.loads(line) for line in refreshed_usage.stdout.splitlines()]
            quota = next(frame["result"] for frame in usage_frames if frame.get("kind") == "result")
            assert quota["revision"] == "1" and not quota.get("readDeferred", False)
            assert len(quota["windows"][0]["notModels"]) == 4
            control_scope["operation"] = "post-quota-inference"
            cli("call", "zcode", "provider.execute", json.dumps(request),
                "--scope", json.dumps(control_scope))
            with guard:
                observed["mode"] = "uncertain"
            cli("auth", "refresh", "zcode", "zcode", identity, succeeds=False)
            failed = record()
            assert failed["generation"] == 2
            assert failed["auth"]["projectToken"] == "synthetic-replacement"
            assert failed["refresh"]["state"] == "uncertain"
            cli("auth", "refresh", "zcode", "zcode", identity, succeeds=False)
            with guard:
                assert observed["mints"] == 2 and observed["inferences"] == 2 and observed["logins"] == 1
                assert observed["tickets"] == 1 and observed["native"] == 2 and observed["settlements"] == 1
                assert observed["mcpReads"] == 1
                assert observed["remoteInit"] == 1 and observed["remoteList"] == 1 and observed["remoteCalls"] == 1
                assert not observed["errors"], observed["errors"]
            print(json.dumps({
                "passed": True, "hostSHA256": hashlib.sha256(Path(host).read_bytes()).hexdigest(),
                "nativeABI": 1, "minimumHostAPI": "1.20", "generationBefore": 1,
                "generationAfter": 2, "credentialsExpiryReceiptAtomic": True,
                "businessAndPATPublishedTogether": True,
                "candidateReadinessDidNotCreateDataDir": True,
                "aliasDerivedAdmissionReusedAcrossProcesses": True,
                "durableSettlementIdempotent": True,
                "businessCancelKeptOriginalTarget": True,
                "usageInvalidationBlockedSeparateCLIInference": True,
                "deferredReadDidNotClearBarrier": True,
                "freshMatchingReadReopenedInference": True,
                "localOwnerNativeToolsSelectedGeneration": True,
                "officialMCPAsideSelectedJWT": True,
                "informationalToolDidNotAcknowledgeUsageBarrier": True,
                "offlineHistoryToolPreservedOriginalBytes": True,
                "fixedRemoteMCPToolSelectedJWTAndSession": True,
                "fixedRemoteMCPToolBoundedSSE": True,
                "remoteMCPIntentReplayRefused": True,
                "dynamicallyUndeclaredRemoteToolRefused": True,
                "replacementUsedByNativeInference": True, "uncertainFencePreventedReplay": True,
                "issuerRequests": 2, "businessLoginRequests": 1,
                "inferenceRequests": 2, "fixtureTargetsOnly": True,
            }))
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
