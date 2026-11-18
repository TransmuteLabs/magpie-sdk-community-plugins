import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import sqlite3
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time


def main():
    source = Path(__file__).resolve().parents[1] / "src"
    host = Path(sys.argv[1]).resolve()
    china = len(sys.argv) == 3 and sys.argv[2] == "--china"
    if len(sys.argv) > 2 and not china:
        raise SystemExit("expected host binary and optional --china")
    site = "bigmodel" if china else "zai"
    login_provider = "zai_cn" if china else "zai"
    observed = {"init": 0, "poll": {}, "inferences": 0, "errors": []}
    expiry = int(time.time()) + 3600

    def token(flow):
        payload = base64.urlsafe_b64encode(json.dumps({"exp": expiry, "flow": flow}).encode()).decode().rstrip("=")
        return "e30." + payload + ".synthetic"

    class Vendor(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def answer(self, value):
            payload = json.dumps(value).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def do_GET(self):
            if self.path.startswith("/api/v1/oauth/cli/poll/"):
                flow = self.path.rsplit("/", 1)[1]
                observed["poll"][flow] = observed["poll"].get(flow, 0) + 1
                if observed["poll"][flow] == 1:
                    data = {"status": "pending"}
                else:
                    data = {"status": "ready", "token": token(flow), "user": {"user_id": "synthetic-start-user"}}
                    if china:
                        data.update(bigmodel=None, zai={"access_token": "synthetic-China-bootstrap-not-paid"})
                self.answer({"code": 0, "data": data})
            elif self.path.startswith("/api/v1/client/configs?"):
                self.answer({"code": 0, "data": {"configs": {
                    "builtin_provider_config_json": origin + "/public-config"
                }}})
            elif self.path == "/public-config":
                if self.headers.get("Authorization") or self.headers.get("X-Api-Key"):
                    observed["errors"].append("credentials reached public config")
                self.answer({"config": {
                    "providerConfigRules": {"providerRules": [{
                        "providerId": f"account:{site}-start-plan",
                        "config": {"builtinModelIds": ["GLM-5.3", "GLM-5.2"]},
                    }]},
                    "modelConfigRules": {"modelRules": [{
                        "modelMatch": ".*", "config": {"enabled": True, "properties": {"contextWindow": 256000}},
                    }]},
                }})
            elif self.path.startswith("/api/v1/zcode-plan/billing/balance?"):
                self.answer({"code": 0, "data": {
                    "plans": [{"plan_id": "start", "user_plan_id": "owned", "status": "active"}],
                    "balances": [{"plan_id": "start", "user_plan_id": "owned",
                                  "remaining_units": 7, "capabilities": ["model:GLM-5.3"]}],
                }})
            else:
                observed["errors"].append("unexpected GET " + self.path.split("?", 1)[0][:200])
                self.send_error(404)

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            if self.path == "/api/v1/oauth/cli/init":
                observed["init"] += 1
                if body != {"provider": login_provider} or not self.headers.get("Authorization", "").startswith("Bearer "):
                    observed["errors"].append("init credential/selection boundary")
                self.answer({"code": 0, "data": {
                    "flow_id": str(observed["init"]), "authorize_url": "https://chat.z.ai/login?state=synthetic",
                    "poll_interval_sec": 1, "expires_at": int(time.time()) + 300,
                }})
            elif self.path == "/start":
                observed["inferences"] += 1
                if (self.headers.get("Authorization") != "Bearer " + token("2")
                        or self.headers.get("X-Api-Key") or body["model"] != "GLM-5.3"):
                    observed["errors"].append("renewed session/family boundary")
                self.answer({"type": "message", "content": [{"type": "text", "text": "auth-fixture"}]})
            else:
                observed["errors"].append("unexpected POST " + self.path.split("?", 1)[0][:200])
                self.send_error(404)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Vendor)
    origin = f"http://127.0.0.1:{server.server_port}"
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="zcode-auth-e2e-") as scratch:
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
                    "CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME", "MAGPIE_UPDATE_FEED"
                }:
                    env.pop(key, None)
            declaration = json.loads(subprocess.check_output(["go", "run", "./cmd/manifest", "--headless"], cwd=source, env=env))
            fields = ",".join(name + ":" + json.dumps(origin + path) for name, path in {
                "Zai": "/zai", "BigModel": "/bigmodel", "Start": "/start",
                "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
            }.items())
            entry = root / "entry.go"
            entry.write_text(
                'package main\nimport (zcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
                '"github.com/yetone/magpie/pkg/extension/native")\n'
                'func init(){if err:=native.Register(zcode.NewModule(zcode.ModuleOptions{Endpoints:zcode.Endpoints{'
                + fields + '}}));err!=nil{panic(err)}}\nfunc main(){}\n'
            )
            overlay = root / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {str(source / "cmd/native/main.go"): str(entry)}}))
            distribution = root / "distribution"
            library = distribution / declaration["entry"]
            library.parent.mkdir(parents=True)
            subprocess.run(["go", "build", "-overlay", str(overlay), "-buildmode=c-shared",
                            "-o", str(library), "./cmd/native"], cwd=source, env=env, check=True)
            (distribution / "magpie-plugin.json").write_text(json.dumps(declaration))

            def cli(*args, succeeds=True):
                result = subprocess.run([str(host), "plugin", "sdk", *args], env=env, text=True,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
                if (result.returncode == 0) != succeeds:
                    raise AssertionError(f"SDK {args[0]} exit {result.returncode}: {result.stderr[-500:]}")
                return result.stdout

            cli("add", str(distribution))

            documents = root / "documents"
            documents.mkdir()
            history_source = b'{"type":"assistant","sessionId":"opaque-task","opaque":9007199254740993,"message":{"role":"assistant","content":[{"type":"thinking","signature":"synthetic-signature"}]}}\r\n'
            (documents / "source.jsonl").write_bytes(history_source)
            (documents / "bundle.json").write_bytes(b"previous-reviewed-target")
            conversion = {
                "id": "zcode-history", "from": "zcode-native-jsonl", "to": "zcode-history-bundle-v1",
                "files": [{"path": "source.jsonl", "format": "zcode-native-jsonl",
                           "body": base64.b64encode(history_source).decode()}],
                "options": {"outputPath": "bundle.json"},
            }
            preview = json.loads(cli("session", "zcode", json.dumps(conversion), "--root", str(documents),
                                     "--path", "bundle.json"))
            assert preview["revision"] and (documents / "bundle.json").read_bytes() == b"previous-reviewed-target"
            applied = json.loads(cli("session", "zcode", json.dumps(conversion), "--root", str(documents),
                                     "--path", "bundle.json", "--apply", "--backup-dir", str(root / "backups")))
            assert applied["applied"]
            backup = Path(applied["backup"])
            backup_manifest = json.loads((backup / "manifest.json").read_text())
            backed_up = next(item for item in backup_manifest["files"] if item["path"] == "bundle.json")
            assert (backup / backed_up["backup"]).read_bytes() == b"previous-reviewed-target"
            packed = (documents / "bundle.json").read_bytes()
            restoring = {
                "id": "zcode-history", "from": "zcode-history-bundle-v1", "to": "zcode-native-jsonl",
                "files": [{"path": "bundle.json", "format": "zcode-history-bundle-v1",
                           "body": base64.b64encode(packed).decode()}],
                "options": {"outputPath": "restored.jsonl"},
            }
            restored = json.loads(cli("session", "zcode", json.dumps(restoring), "--root", str(documents),
                                      "--path", "restored.jsonl", "--apply", "--backup-dir", str(root / "restore-backups")))
            assert restored["applied"] and (documents / "restored.jsonl").read_bytes() == history_source
            assert (documents / "source.jsonl").read_bytes() == history_source
            unsafe = dict(conversion, options={"outputPath": "../escape.json"})
            cli("session", "zcode", json.dumps(unsafe), "--root", str(documents), "--path", "safe.json", succeeds=False)
            assert not (root / "escape.json").exists()

            database_path = documents / "sqlite-source.db"
            with sqlite3.connect(database_path) as database:
                database.execute("CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)")
                database.execute("INSERT INTO tasks VALUES('synthetic-task','User title',9007199254740993,0)")
            database_source = database_path.read_bytes()
            (documents / "sqlite-bundle.json").write_bytes(b"previous-sqlite-target")
            sqlite_conversion = {
                "id": "zcode-sqlite-history", "from": "zcode-sqlite-db", "to": "zcode-sqlite-bundle-v1",
                "files": [{"path": "sqlite-source.db", "format": "zcode-sqlite-db",
                           "body": base64.b64encode(database_source).decode()}],
                "options": {"outputPath": "sqlite-bundle.json"},
            }
            sqlite_preview = json.loads(cli("session", "zcode", json.dumps(sqlite_conversion),
                                            "--root", str(documents), "--path", "sqlite-bundle.json"))
            assert sqlite_preview["revision"] and (documents / "sqlite-bundle.json").read_bytes() == b"previous-sqlite-target"
            sqlite_applied = json.loads(cli("session", "zcode", json.dumps(sqlite_conversion), "--root", str(documents),
                                           "--path", "sqlite-bundle.json", "--apply", "--backup-dir", str(root / "sqlite-backups")))
            assert sqlite_applied["applied"]
            sqlite_backup = Path(sqlite_applied["backup"])
            sqlite_backup_manifest = json.loads((sqlite_backup / "manifest.json").read_text())
            sqlite_before = next(item for item in sqlite_backup_manifest["files"] if item["path"] == "sqlite-bundle.json")
            assert (sqlite_backup / sqlite_before["backup"]).read_bytes() == b"previous-sqlite-target"
            sqlite_restore = {
                "id": "zcode-sqlite-history", "from": "zcode-sqlite-bundle-v1", "to": "zcode-sqlite-db",
                "files": [{"path": "sqlite-bundle.json", "format": "zcode-sqlite-bundle-v1",
                           "body": base64.b64encode((documents / "sqlite-bundle.json").read_bytes()).decode()}],
                "options": {"outputPath": "sqlite-restored.db"},
            }
            sqlite_restored = json.loads(cli("session", "zcode", json.dumps(sqlite_restore), "--root", str(documents),
                                            "--path", "sqlite-restored.db", "--apply",
                                            "--backup-dir", str(root / "sqlite-restore-backups")))
            assert sqlite_restored["applied"] and (documents / "sqlite-restored.db").read_bytes() == database_source
            assert database_path.read_bytes() == database_source
            generated_path = documents / "generated.db"
            with sqlite3.connect(generated_path) as database:
                database.execute("CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER,expanded TEXT GENERATED ALWAYS AS (printf('%2000s','x')) VIRTUAL)")
                database.execute("INSERT INTO tasks(task_id,title,updated_at,deleted)VALUES('safe','Generated fixture',1,0)")
            generated_source = generated_path.read_bytes()
            generated_request = dict(sqlite_conversion, files=[{
                "path": "generated.db", "format": "zcode-sqlite-db",
                "body": base64.b64encode(generated_source).decode(),
            }], options={"outputPath": "generated-bundle.json"})
            cli("session", "zcode", json.dumps(generated_request), "--root", str(documents),
                "--path", "generated-bundle.json", succeeds=False)
            assert generated_path.read_bytes() == generated_source and not (documents / "generated-bundle.json").exists()

            def authorize(selected=None):
                flags = ["--scope", json.dumps(selected)] if selected else []
                auth_input = {"site": site, "kind": "auto" if china else "start"}
                if china:
                    auth_input["loginProvider"] = login_provider
                started = json.loads(cli("auth", "start", "zcode", "zcode", json.dumps({
                    "method": "browser", "input": auth_input,
                }), *flags))
                assert set(started["continuation"]) == {"id"}
                assert started["pollAfterMs"] == 1000
                finish = json.dumps({"continuation": started["continuation"]})
                assert json.loads(cli("auth", "finish", "zcode", "zcode", finish, *flags))["pending"]
                assert json.loads(cli("auth", "finish", "zcode", "zcode", finish, *flags))["pending"]
                time.sleep(1.05)
                result = json.loads(cli("auth", "finish", "zcode", "zcode", finish, *flags))
                assert result["id"] and "credentials" not in result
                cli("auth", "finish", "zcode", "zcode", finish, *flags, succeeds=False)
                return result

            signed = authorize()
            identity = signed["id"]
            selected = {"provider": "zcode", "account": identity, "operation": "auth-discovery-proof"}
            before = json.loads(cli("accounts", "zcode", "zcode", "--json"))
            assert before[0]["generation"] == 1
            renewed = authorize(selected)
            assert renewed["id"] == identity
            after = json.loads(cli("accounts", "zcode", "zcode", "--json"))
            assert after[0]["generation"] == 2

            def rpc(method, value):
                lines = cli("call", "zcode", method, json.dumps(value), "--scope", json.dumps(selected)).splitlines()
                return next(json.loads(line)["result"] for line in lines if json.loads(line).get("kind") == "result")

            models = rpc("provider.models", {})["models"]
            assert len(models) == 1 and models[0]["id"] == "GLM-5.3" and models[0]["contextWindow"] == 256000
            result = rpc("provider.execute", {
                "protocol": "anthropic", "model": "GLM-5.3",
                "body": base64.b64encode(b'{"model":"GLM-5.3","messages":[]}').decode(),
                "options": {"method": "POST", "path": "/v1/messages", "query": ""},
            })
            assert result["outcome"] == "completed"
            assert observed["init"] == 2 and observed["poll"] == {"1": 2, "2": 2}
            assert observed["inferences"] == 1 and not observed["errors"], observed
            print(json.dumps({
                "passed": True, "hostSHA256": hashlib.sha256(host.read_bytes()).hexdigest(),
                "nativeABI": 1, "minimumHostAPI": "1.4", "headlessInitPollFinish": True,
                "privateContinuation": True, "pollIntervalHonoredAcrossCLIProcesses": True,
                "completedExchangeNotReplayed": True, "sameAccountStartReauthorization": True,
                "generationBefore": 1, "generationAfter": 2, "dynamicEntitlementModels": True,
                "chinaUnlinkedStart": china, "loginProvider": login_provider,
                "renewedSessionUsedByInference": True, "fixtureTargetsOnly": True,
                "registeredOfflineSessionAdapter": True, "sessionPreviewDidNotWrite": True,
                "sessionApplyBackedUpExistingTarget": True, "sessionRestorePreservedExactBytes": True,
                "sessionSourceUnchanged": True, "unsafeSessionPlanRejected": True,
                "realSQLiteBinaryPreviewApplyRestore": True, "sqliteSourceUnchanged": True,
                "sqliteBinaryPreimageBackupExact": True, "sqliteGeneratedSchemaRejectedBeforePlan": True,
            }))
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
