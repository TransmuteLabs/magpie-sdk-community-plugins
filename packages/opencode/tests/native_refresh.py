#!/usr/bin/env python3
"""Exercise a rebuilt SDK host and native fixture against synthetic OAuth."""
import argparse
import base64
import hashlib
import http.server
import json
import os
import pathlib
import subprocess
import threading
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("host", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    parser.add_argument("--ordinary-renewal", action="store_true")
    parser.add_argument("--all-models", action="store_true")
    args = parser.parse_args()
    if args.all_models and not args.ordinary_renewal:
        parser.error("--all-models requires --ordinary-renewal")
    root = pathlib.Path(__file__).resolve().parents[1] / "src"
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    home = output / "home"
    home.mkdir()
    lock = threading.Lock()
    state = {"devices": {}, "refreshes": [], "chat": [], "gemini": [], "usage": [], "errors": []}

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *values):
            pass

        def reply(self, value, status=200):
            body = json.dumps(value).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            with lock:
                if self.path == "/console/auth/device/code":
                    user = chr(ord("a") + len(state["devices"]))
                    device = "device-" + user
                    state["devices"][device] = user
                    return self.reply({"device_code": device, "user_code": "CODE-" + user,
                                       "verification_uri_complete": "/console/verify",
                                       "expires_in": 60, "interval": 1})
                if self.path == "/console/auth/device/token":
                    if self.headers.get("Authorization"):
                        state["errors"].append("token endpoint received inference authority")
                    if body.get("client_id") != "opencode-cli":
                        return self.reply({"error": "invalid_client"}, 400)
                    if body.get("grant_type") == "refresh_token":
                        token = body.get("refresh_token", "")
                        state["refreshes"].append(token)
                        if token not in ["refresh-a", "refresh-b"] or state["refreshes"].count(token) != 1:
                            return self.reply({"error": "invalid_grant"}, 400)
                        user = token.removeprefix("refresh-")
                        return self.reply({"access_token": "rotated-access-" + user,
                                           "refresh_token": "rotated-refresh-" + user,
                                           "expires_in": 3600, "org_id": "org-" + user})
                    user = state["devices"].get(body.get("device_code"))
                    if not user:
                        return self.reply({"error": "invalid_grant"}, 400)
                    return self.reply({"access_token": "access-" + user, "refresh_token": "refresh-" + user,
                                       "expires_in": 10, "org_id": "org-" + user})
                if self.path == "/inference/go/openai/v1/chat/completions":
                    state["chat"].append(dict(self.headers))
                    return self.reply({"choices": [{"message": {"role": "assistant", "content": "fixture"}}]})
                if self.path == "/inference/go/google/v1beta/models/gemini-4-argon:generateContent":
                    state["gemini"].append(dict(self.headers))
                    if not body.get("contents") or "model" in body:
                        state["errors"].append("native Gemini body changed")
                    return self.reply({"candidates": [{"content": {"role": "model", "parts": [{"text": "fixture"}]},
                                                       "finishReason": "STOP"}]})
            self.reply({"error": "unexpected_path"}, 404)

        def do_GET(self):
            access = self.headers.get("Authorization", "").removeprefix("Bearer ")
            with lock:
                if self.path == "/console/api/user":
                    user = access.removeprefix("access-")
                    return self.reply({"id": user, "email": user + "@example.invalid"})
                if self.path == "/console/api/orgs":
                    user = access.removeprefix("access-")
                    return self.reply([{"id": "org-" + user, "name": "Organization " + user}])
                if self.path == "/zen/go/v1/usage":
                    state["usage"].append(dict(self.headers))
                    return self.reply({"usage": {"rolling": {"percent": 23}}})
            self.reply({"error": "unexpected_path"}, 404)

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    server.daemon_threads = True
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    env = os.environ.copy()
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"), XDG_STATE_HOME=str(home / "state"),
               MAGPIE_OPENCODE_FIXTURE_URL="http://127.0.0.1:" + str(server.server_port),
               MAGPIE_OPENCODE_FIXTURE_EVENTS=str(output / "module-events.jsonl"),
               GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOWORK="off", CGO_ENABLED="1")
    env.pop("MAGPIE_UPDATE_FEED", None)
    commands = []

    def run(values, expected=0, sdk=True):
        prefix = ["plugin", "sdk"] if sdk else []
        result = subprocess.run([str(args.host.resolve())] + prefix + values, env=env,
                                cwd=root, text=True, capture_output=True, timeout=30)
        record = {"args": values, "exit": result.returncode,
                  "stdout": result.stdout, "stderr": result.stderr}
        commands.append(record)
        assert result.returncode == expected, json.dumps(record)
        for secret in ["rotated-access-", "rotated-refresh-", "refresh-a", "refresh-b"]:
            assert secret not in result.stdout + result.stderr, "CLI disclosed a synthetic credential"
        if not result.stdout.strip():
            return None
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError:
            return result.stdout.strip()

    def accounts():
        files = list(home.rglob("accounts.json"))
        assert len(files) == 1, "fixture did not locate a unique account document"
        return json.loads(files[0].read_text())["accounts"]

    try:
        dist = output / "dist"
        (dist / "lib").mkdir(parents=True)
        subprocess.run(["go", "build", "-mod=readonly", "-buildmode=c-shared",
                        "-o", str(dist / "lib/opencode.so"), "./testdata/native-fixture"],
                       env=env, cwd=root, check=True, timeout=120)
        (dist / "magpie-plugin.json").write_bytes((root / "testdata/compat/provider-0.2.0.json").read_bytes())
        run(["add", str(dist)])
        run(["config", "opencode", '{"projectId":"global"}', "--expected", "0"])
        identities = {}
        for user in ["a", "b"]:
            start = run(["auth", "start", "opencode", "opencode-go", '{"method":"device"}'])
            time.sleep(start["pollAfterMs"] / 1000 + 0.05)
            result = run(["auth", "finish", "opencode", "opencode-go",
                          json.dumps({"continuation": start["continuation"]})])
            assert not result.get("credentials"), "host returned credentials to CLI"
            identities[user] = result["id"]
            if user == "a" and args.ordinary_renewal:
                provider = "extension-" + hashlib.sha256(b"opencode\0opencode-go").hexdigest()[:24]
                run(["provider", "test", provider, "deepseek-v4.1-flash"], sdk=False)
                renewed = {row["id"]: row for row in accounts()}[identities[user]]
                assert renewed["generation"] == 2, "ordinary provider did not renew an expiring account"
                assert renewed["auth"]["access"] == "rotated-access-a"
                if args.all_models:
                    for model in ["glm-5.2", "kimi-k3", "gemini-4-argon"]:
                        run(["provider", "test", provider, model], sdk=False)
        before = {row["id"]: row for row in accounts()}
        assert len(before) == 2
        assert before[identities["a"]]["generation"] == (2 if args.ordinary_renewal else 1)
        assert before[identities["b"]]["generation"] == 1
        for user in ["a", "b"]:
            account = identities[user]
            if not (user == "a" and args.ordinary_renewal):
                result = run(["auth", "refresh", "opencode", "opencode-go", account,
                              "--scope", '{"generation":1}'])
                assert result["id"] == account and not result.get("credentials")
            after = {row["id"]: row for row in accounts()}
            assert after[account]["generation"] == 2
            auth = after[account]["auth"]
            assert auth["access"] == "rotated-access-" + user and auth["refresh"] == "rotated-refresh-" + user
            other = identities["b" if user == "a" else "a"]
            if user == "a":
                assert after[other] == before[other], "refresh modified another account"
            stale = subprocess.run([str(args.host.resolve()), "plugin", "sdk", "auth", "refresh",
                                    "opencode", "opencode-go", account, "--scope", '{"generation":1}'],
                                   cwd=root, env=env, text=True, capture_output=True, timeout=30)
            commands.append({"args": ["stale-refresh", user], "exit": stale.returncode,
                             "stdout": stale.stdout, "stderr": stale.stderr})
            assert stale.returncode != 0, "stale generation was refreshed"
            request = {"protocol": "chat", "model": "deepseek-v4.1-flash",
                       "body": base64.b64encode(b'{"model":"deepseek-v4.1-flash","messages":[]}').decode()}
            executed = subprocess.run([str(args.host.resolve()), "plugin", "sdk", "call", "opencode",
                                       "provider.execute", json.dumps(request), "--scope",
                                       json.dumps({"provider": "opencode-go", "account": account})],
                                      cwd=root, env=env, text=True, capture_output=True, timeout=30)
            commands.append({"args": ["chat", user], "exit": executed.returncode,
                             "stdout": executed.stdout, "stderr": executed.stderr})
            assert executed.returncode == 0, json.dumps(commands[-1])
            run(["call", "opencode", "provider.usage", "{}", "--scope",
                 json.dumps({"provider": "opencode-go", "account": account})])
        with lock:
            assert state["refreshes"] == ["refresh-a", "refresh-b"], "rotation was repeated"
            expected_chat = (3 if args.ordinary_renewal else 2) + (2 if args.all_models else 0)
            assert len(state["chat"]) == expected_chat and len(state["usage"]) == 2
            assert len(state["gemini"]) == (1 if args.all_models else 0)
            for rows in [state["chat"], state["usage"]]:
                expected_users = ["a"] * (expected_chat - 1) + ["b"] if rows is state["chat"] else ["a", "b"]
                for user, headers in zip(expected_users, rows):
                    lowered = {key.lower(): value for key, value in headers.items()}
                    assert lowered.get("authorization") == "Bearer rotated-access-" + user
                    assert lowered.get("x-opencode-org-id") == "org-" + user
                    assert not lowered.get("cookie") and not lowered.get("x-api-key")
            for headers in state["gemini"]:
                lowered = {key.lower(): value for key, value in headers.items()}
                assert lowered.get("x-goog-api-key") == "rotated-access-a"
                assert lowered.get("x-opencode-org-id") == "org-a" and not lowered.get("authorization")
            assert not state["errors"], state["errors"]
        report = {"host": str(args.host.resolve()), "commands": commands,
                  "accounts": len(identities), "refresh_exchanges": 2,
                  "stale_retries_rejected": 2, "chat_requests": len(state["chat"]), "usage_requests": 2,
                  "ordinary_predispatch_renewals": 1 if args.ordinary_renewal else 0,
                  "ordinary_model_tests": 4 if args.all_models else (1 if args.ordinary_renewal else 0),
                  "native_gemini_requests": len(state["gemini"]),
                  "host_sha256": hashlib.sha256(args.host.resolve().read_bytes()).hexdigest(),
                  "module_sha256": hashlib.sha256((dist / "lib/opencode.so").read_bytes()).hexdigest(),
                  "live_vendor_requests": 0}
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({key: value for key, value in report.items() if key != "commands"}))
    finally:
        (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
        with lock:
            (output / "fixture-counts.json").write_text(json.dumps({
                "refreshes": len(state["refreshes"]), "chat": len(state["chat"]), "gemini": len(state["gemini"]),
                "usage": len(state["usage"]), "errors": state["errors"]}, indent=2) + "\n")
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)


if __name__ == "__main__":
    main()
