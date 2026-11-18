import contextlib
import hashlib
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
import zipfile


def main():
    source = Path(__file__).resolve().parents[1] / "src"
    host = str(Path(sys.argv[1]).resolve())
    guard = threading.Lock()
    observed = {"profiles": [], "resets": 0, "claims": 0, "feeds": 0, "mints": 0,
                "opportunities": 0, "acks": 0, "unread": True, "errors": []}
    expiry = int(time.time()) + 3600

    class Vendor(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_CONNECT(self):
            self.send_error(403)

        def reply(self, value, status=200):
            body = json.dumps(value).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def selected_user(self):
            token = self.headers.get("Authorization")
            return {"Bearer session-a": "user-a", "Bearer session-b": "user-b"}.get(token)

        def do_GET(self):
            user = self.selected_user()
            if self.path == "/api/v1/oauth/profile":
                with guard:
                    observed["profiles"].append(user)
                if user is None:
                    self.reply({}, 401)
                else:
                    self.reply({"code": 0, "data": {"user": {"user_id": user}}})
            elif self.path.startswith("/api/v1/zcode-plan/billing/balance?"):
                self.reply({"code": 0, "data": {"plans": []}})
            elif self.path.startswith("/api/v1/client/configs?"):
                self.reply({"code": 0, "data": {"configs": {"captcha": {"enabled": False}}}})
            elif self.path.startswith("/api/v1/marketing/touch?seq="):
                with guard:
                    observed["feeds"] += 1
                    if user != "user-a" or self.headers.get("X-ZCode-App-Version") != "3.18.2":
                        observed["errors"].append("feed selected/version channel")
                self.reply({"code": 0, "data": {"deliveries": [{
                    "action": {"type": "claim_zcode_plan", "args": {"plan_id": "checkin-plan"}},
                }]}})
            elif self.path == "/api/v1/coding-plan/reset/status":
                if self.headers.get("X-Bigmodel-Authorization") != "business-a":
                    with guard:
                        observed["errors"].append("reset selected business channel")
                self.reply({"code": 0, "data": {
                    "available_five_hour_resets": [{"expire_at": expiry * 1000}],
                    "available_week_resets": [], "has_unread_history": observed["unread"],
                    "latest_five_hour_reset_history": None, "latest_week_reset_history": None,
                }})
            else:
                self.reply({}, 404)

        def do_POST(self):
            payload = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))) or b"{}")
            with guard:
                if self.path == "/api/v1/zcode-plan/billing/claim":
                    observed["claims"] += 1
                    plan_id = payload.get("plan_id")
                    if self.selected_user() != "user-a" or payload != {"plan_id": plan_id} or plan_id not in {"synthetic-plan", "checkin-plan"}:
                        observed["errors"].append("claim selection/input")
                    value = {"code": 0, "data": {"plan": {
                        "plan_id": plan_id, "user_plan_id": "claimed-" + plan_id,
                        "status": "active", "ends_at": expiry,
                    }}}
                    status = 200
                elif self.path == "/api/v1/coding-plan/reset/use":
                    observed["resets"] += 1
                    if (self.selected_user() != "user-a" or payload.get("reset_type") != "FIVE_HOUR"
                            or set(payload) != {"reset_type", "idempotency_key"} or not payload["idempotency_key"]):
                        observed["errors"].append("reset selection/input")
                    value, status = {"code": 503}, 503
                elif self.path == "/api/v1/coding-plan/reset/opportunity":
                    observed["opportunities"] += 1
                    if (self.selected_user() != "user-a" or set(payload) != {"idempotency_key"}
                            or not payload["idempotency_key"]):
                        observed["errors"].append("opportunity selection/input")
                    value, status = {"code": 503}, 503
                elif self.path == "/api/v1/coding-plan/reset/history/read":
                    observed["acks"] += 1
                    observed["unread"] = False
                    if (self.selected_user() != "user-a" or self.headers.get("Bigmodel-Target-Type")
                            or self.headers.get("Bigmodel-Organization")):
                        observed["errors"].append("history global user scope")
                    value, status = {"code": 0}, 200
                elif self.path.endswith("/access_tokens"):
                    observed["mints"] += 1
                    if self.headers.get("Authorization") != "Bearer business-a":
                        observed["errors"].append("refresh selected business channel")
                    value, status = {"code": 0, "data": {
                        "accessToken": "pat-a-new", "tokenType": "Bearer", "expiresIn": 600, "expiresAt": expiry,
                    }}, 200
                else:
                    value, status = {}, 404
            self.reply(value, status)

    vendor = ThreadingHTTPServer(("127.0.0.1", 0), Vendor)
    thread = threading.Thread(target=vendor.serve_forever)
    thread.start()
    web = None
    try:
        with tempfile.TemporaryDirectory(prefix="zcode-ui-e2e-") as scratch, contextlib.ExitStack() as cleanup:
            root = Path(scratch)
            home = root / "home"
            home.mkdir()
            env = os.environ.copy()
            env.update(json.loads(subprocess.check_output(["go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"])))
            env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                       XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                       XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                       LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
                       GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off", CGO_ENABLED="1")
            for key in list(env):
                if key.lower().endswith("_proxy") or key in {
                    "CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME",
                    "MAGPIE_UPDATE_FEED", "MAGPIE_ADDR", "MAGPIE_WEB_KEY",
                }:
                    env.pop(key, None)
            origin = f"http://127.0.0.1:{vendor.server_port}"
            env.update(HTTP_PROXY=origin, HTTPS_PROXY=origin, ALL_PROXY=origin,
                       NO_PROXY="127.0.0.1,localhost", MAGPIE_UPDATE_FEED=origin + "/updates",
                       MAGPIE_WEB_KEY="synthetic-zcode-page-key")
            distribution = root / "distribution"
            with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
                archive.extractall(distribution)
            manifest = json.loads((distribution / "magpie-plugin.json").read_bytes())
            assert manifest["contract"]["version"]["minor"] == 12
            fields = ",".join(name + ":" + json.dumps(origin + path) for name, path in {
                "Zai": "/zai", "BigModel": "/bigmodel", "Start": "/start",
                "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
            }.items())
            entry = root / "entry.go"
            entry.write_text(
                'package main\nimport (zcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
                '"github.com/yetone/magpie/pkg/extension/native")\n'
                'func init(){if err:=native.Register(zcode.NewControlModule(zcode.ModuleOptions{Endpoints:zcode.Endpoints{'
                + fields + '}}));err!=nil{panic(err)}}\nfunc main(){}\n'
            )
            overlay = root / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {str(source / "cmd/native-ui/main.go"): str(entry)}}))
            subprocess.run(["go", "build", "-overlay", str(overlay), "-buildmode=c-shared",
                            "-o", str(distribution / manifest["entry"]), "./cmd/native-ui"],
                           cwd=source, env=env, check=True)

            def cli(*args, succeeds=True):
                result = subprocess.run([host, "plugin", "sdk", *args], env=env, text=True,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
                if (result.returncode == 0) != succeeds:
                    raise AssertionError(f"SDK {args[0]} exit {result.returncode}: {result.stderr[-600:]}")
                return result.stdout

            cli("add", str(distribution))
            config = json.loads(cli("config", "zcode"))
            data_dir = root / "private-controller"
            cli("config", "zcode", json.dumps({"dataDir": str(data_dir)}),
                "--expected", config["version"])
            identities = []
            for suffix in ("a", "b"):
                credentials = {
                    "site": "zai", "kind": "individual", "userId": "user-" + suffix,
                    "device": "device-" + suffix, "projectToken": "pat-" + suffix,
                    "expiresAt": expiry * 1000, "businessToken": "business-" + suffix,
                    "businessExpiresAt": expiry * 1000, "sessionToken": "session-" + suffix,
                    "sessionExpiresAt": expiry * 1000,
                    "location": {"organizationId": "org", "projectId": "project", "apiKeyId": "key"},
                }
                imported = json.loads(cli("auth", "finish", "zcode", "zcode", json.dumps({"input": credentials})))
                identities.append(imported["id"])
            assert not data_dir.exists(), "readiness mutated shared vendor storage"
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                env["MAGPIE_ADDR"] = f"127.0.0.1:{listener.getsockname()[1]}"
            web_origin = f"http://127.0.0.1:{port}"
            web_log = cleanup.enter_context((root / "web.log").open("wb"))
            web = subprocess.Popen([host, "web", "--addr", f"127.0.0.1:{port}", "--no-open"],
                                   env=env, stdout=web_log, stderr=subprocess.STDOUT)

            def stop_web(process):
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)

            cleanup.callback(stop_web, web)
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                                 urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

            def request(path, value=None, succeeds=True):
                headers = {"Origin": web_origin, "Sec-Fetch-Site": "same-origin"}
                body = None
                if value is not None:
                    headers["Content-Type"] = "application/json"
                    body = json.dumps(value).encode()
                try:
                    response = opener.open(urllib.request.Request(web_origin + path, body, headers), timeout=30)
                except urllib.error.HTTPError as failure:
                    response = failure
                content = response.read()
                if (response.status == 200) != succeeds:
                    raise AssertionError(f"page route {path}: {response.status}: {content[:500]!r}")
                for secret in (b"session-a", b"session-b", b"business-a", b"business-b", b"pat-a", b"pat-b"):
                    assert secret not in content, "credential reached page response"
                return content, response.headers

            deadline = time.monotonic() + 30
            while True:
                try:
                    request("/?k=synthetic-zcode-page-key")
                    break
                except (OSError, urllib.error.URLError):
                    if web.poll() is not None or time.monotonic() > deadline:
                        raise AssertionError("isolated web host failed to start")
                    time.sleep(0.1)
            pages = json.loads(request("/api/plugin-pages")[0])
            page = next(value for value in pages if value["name"] == "zcode")
            base = page["url"].removesuffix("index.html")
            for relative in ("index.html", "app.bundle.js", "app.js", "page.js", "control.js", "sdk-ui.js", "style.css"):
                content, headers = request(base + relative)
                if relative != "index.html":
                    assert content == (distribution / "web" / relative).read_bytes()
                else:
                    assert "Управление счётом".encode() in content
                    assert b'<script src="./app.bundle.js"' not in content
                    assert b"<script>globalThis.__magpieInlined=1;" in content
                    assert b'type="module"' not in content
                assert "sandbox allow-scripts" in headers.get("Content-Security-Policy", "")
            summaries = json.loads(request("/api/plugin-pages/accounts", {
                "ref": page["ref"], "binding": "page.execution-list",
            })[0])
            assert {value["id"] for value in summaries} == set(identities)
            assert all(value["generation"] == "1" and set(value) <= {"id", "generation", "expiresAt"} for value in summaries)
            assert not data_dir.exists(), "safe account discovery started vendor state"

            def call(binding, identity=identities[0], generation="1", input=None, intent="", succeeds=True):
                return request("/api/plugin-pages/call", {
                    "ref": page["ref"], "binding": binding, "request": intent, "input": input or {},
                    "account": {"id": identity, "expectedGeneration": generation},
                }, succeeds=succeeds)[0]

            for identity in identities:
                assert json.loads(call("page.execution-list", identity=identity)) == []
            assert set(observed["profiles"]) == {"user-a", "user-b"}
            call("page.billing-status")
            call("page.reset-status")
            call("page.reset-use", input={"window": "FIVE_HOUR"}, intent="frame:reset-one", succeeds=False)
            for identity, input in ((identities[0], {"window": "FIVE_HOUR"}),
                                    (identities[1], {"window": "FIVE_HOUR"}),
                                    (identities[0], {"window": "WEEK"})):
                call("page.reset-use", identity=identity, input=input, intent="frame:reset-one", succeeds=False)
            assert observed["resets"] == 1
            summary = json.loads(call("page.reset-summary"))
            assert summary["pending"] is True and summary["opportunityPending"] is False, summary
            reconciled = json.loads(call("page.reset-reconcile", intent="frame:reconcile-one"))
            assert reconciled["outcome"] == "unconfirmed" and observed["resets"] == 1, reconciled
            call("page.reset-resolve", input={"acknowledgment": "retry"}, intent="frame:bad-reset-resolve", succeeds=False)
            resolved = json.loads(call("page.reset-resolve", input={
                "acknowledgment": "vendor-may-have-spent-reset",
            }, intent="frame:reset-resolve"))
            assert resolved["replayed"] is False and observed["resets"] == 1, resolved
            call("page.reset-opportunity", intent="frame:opportunity-one", succeeds=False)
            call("page.reset-opportunity", intent="frame:opportunity-two", succeeds=False)
            summary = json.loads(call("page.reset-summary"))
            assert summary["pending"] is False and summary["opportunityPending"] is True, summary
            call("page.reset-opportunity-resolve", identity=identities[1], input={
                "acknowledgment": "vendor-may-have-granted-reset",
            }, intent="frame:foreign-opportunity-resolve", succeeds=False)
            resolved = json.loads(call("page.reset-opportunity-resolve", input={
                "acknowledgment": "vendor-may-have-granted-reset",
            }, intent="frame:opportunity-resolve"))
            assert resolved["replayed"] is False and observed["opportunities"] == 1, resolved
            summary = json.loads(call("page.reset-summary"))
            assert summary["pending"] is False and summary["opportunityPending"] is False and summary["nextTryAt"] > 0, summary
            acknowledged = json.loads(call("page.reset-ack", intent="frame:ack-one"))
            assert acknowledged["outcome"] == "acknowledged", acknowledged
            acknowledged = json.loads(call("page.reset-ack", intent="frame:ack-two"))
            assert acknowledged["outcome"] == "already-acknowledged" and observed["acks"] == 1, acknowledged
            result = json.loads(call("page.billing-claim", input={"planId": "synthetic-plan"}, intent="frame:claim-one"))
            assert result["outcome"] == "confirmed"
            call("page.billing-claim", input={"planId": "synthetic-plan"}, intent="frame:claim-one", succeeds=False)
            assert observed["claims"] == 1
            offers = json.loads(call("page.billing-offers"))
            assert offers["offers"] == [{"planId": "checkin-plan", "name": "checkin-plan"}]
            call("page.billing-checkin", succeeds=False)
            assert observed["feeds"] == 1 and observed["claims"] == 1
            checked = json.loads(call("page.billing-checkin", intent="frame:checkin-one"))
            assert checked["outcome"] == "claimed"
            call("page.billing-checkin", intent="frame:checkin-one", succeeds=False)
            assert observed["feeds"] == 2 and observed["claims"] == 2
            for bad in (
                {"binding": "page.execution-list", "identity": "foreign"},
                {"binding": "page.execution-list", "generation": "2"},
                {"binding": "page.execution-list", "input": {"account": identities[1], "auth": "forged"}},
                {"binding": "page.reset-use", "input": {"window": "FIVE_HOUR"}},
                {"binding": "not-a-binding"},
            ):
                call(**bad, succeeds=False)
            cli("auth", "refresh", "zcode", "zcode", identities[0])
            call("page.execution-list", succeeds=False)
            summaries = json.loads(request("/api/plugin-pages/accounts", {
                "ref": page["ref"], "binding": "page.execution-list",
            })[0])
            assert next(value for value in summaries if value["id"] == identities[0])["generation"] == "2"
            call("page.execution-list", generation="2")
            cli("off", "zcode")
            request("/api/plugin-pages/accounts", {"ref": page["ref"], "binding": "page.execution-list"}, succeeds=False)
            assert not observed["errors"], observed["errors"]
            assert observed["mints"] == 1
            web.terminate()
            web.wait(timeout=15)
            web = None
            web_log.close()
            print(json.dumps({
                "passed": True, "hostSHA256": hashlib.sha256(Path(host).read_bytes()).hexdigest(),
                "minimumHostAPI": "1.12", "actualNativePageRoutes": True,
                "checkedStaticAssets": True, "sanitizedAccountEnumeration": True,
                "twoSelectedAccountsReachedNativeProfile": True,
                "uncertainResetNotReplayedOrRetargeted": True, "claimDispatchedOnce": True,
                "resetSummaryAndHistoryReconciliation": True, "explicitOpportunityDebtRecovery": True,
                "foreignOpportunityDebtCannotBeCleared": True, "historyAcknowledgedOnce": True,
                "selectedOffersAndExplicitCheckin": True, "checkinIntentNeverReplayed": True,
                "unboundForgedForeignStaleCallsDenied": True,
                "refreshGenerationRequiresReselection": True, "disabledPageRefDenied": True,
                "noCredentialInPageResponses": True, "fixtureTargetsOnly": True,
            }))
    finally:
        if web is not None:
            web.terminate()
            try:
                web.wait(timeout=15)
            except subprocess.TimeoutExpired:
                web.kill()
                web.wait(timeout=5)
        vendor.shutdown()
        vendor.server_close()
        thread.join()


if __name__ == "__main__":
    main()
