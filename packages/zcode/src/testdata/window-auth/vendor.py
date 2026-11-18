import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
import time


class Vendor:
    def __init__(self):
        self.lock = threading.Lock()
        self.expiry = int(time.time()) + 3600
        self.issued = {"zcode": [], "opencode": []}
        self.observed_usage = {"zcode": [], "opencode": []}
        self.current_access = ""
        self.refreshes = 0
        self.inferences = {"zcode": 0, "opencode": 0}
        self.flows = {}
        self.modes = {"zcode": "success", "opencode": "success"}
        self.errors = []
        self.started = {"zcode": 0, "opencode": 0}
        self.polled = {"zcode": 0, "opencode": 0}
        self.release = threading.Event()
        self.on_disable = None
        self.on_account = None
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def answer(self, value, status=200):
                raw = json.dumps(value).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def do_CONNECT(self):
                self.send_error(403)

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
                if self.path == "/__fixture__/mode":
                    with owner.lock:
                        owner.modes[body["kind"]] = body["mode"]
                    return self.answer({"ok": True})
                if self.path == "/__fixture__/release":
                    owner.release.set()
                    return self.answer({"ok": True})
                if self.path == "/__fixture__/disable":
                    if owner.on_disable is None:
                        return self.answer({}, 503)
                    owner.on_disable(body["kind"])
                    return self.answer({"ok": True})
                if self.path == "/__fixture__/account":
                    return self.answer(owner.on_account(body["kind"], body["id"]))
                if self.path in ("/api/v1/oauth/cli/init", "/console/auth/device/code"):
                    kind = "zcode" if self.path.endswith("/init") else "opencode"
                    with owner.lock:
                        owner.started[kind] += 1
                        number = owner.started[kind]
                        identity = f"private-{kind}-flow-{number}"
                        owner.flows[identity] = {
                            "kind": kind, "mode": owner.modes[kind], "polls": 0,
                            "authorization": self.headers.get("Authorization"), "held": False,
                        }
                    if kind == "zcode":
                        if body != {"provider": "zai"} or not (self.headers.get("Authorization") or "").startswith("Bearer "):
                            owner.errors.append("ZCode init boundary")
                        return self.answer({"code": 0, "data": {
                            "flow_id": identity, "authorize_url": "https://auth.example.invalid/login?state=public-link",
                            "poll_interval_sec": 1, "expires_at": int(time.time()) + 300,
                            "code_verifier": "private-pkce-canary",
                        }})
                    if body != {"client_id": "opencode-cli", "supports_org_scope": True}:
                        owner.errors.append("OpenCode device init boundary")
                    return self.answer({
                        "device_code": identity, "user_code": "PUBLIC-CODE",
                        "verification_uri_complete": "https://auth.example.invalid/device",
                        "interval": 1, "expires_in": 300, "code_verifier": "private-pkce-canary",
                    })
                if self.path == "/console/auth/device/token":
                    if body.get("grant_type") == "refresh_token":
                        if body.get("refresh_token") != "private-opencode-refresh-private-opencode-flow-1":
                            owner.errors.append("unexpected refresh credential")
                        owner.refreshes += 1
                        owner.current_access = "private-opencode-rotated-access"
                        return self.answer({
                            "access_token": owner.current_access, "refresh_token": "private-opencode-rotated-refresh",
                            "expires_in": 3600, "org_id": "organization",
                        })
                    if body.get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" or body.get("client_id") != "opencode-cli":
                        owner.errors.append("OpenCode device grant boundary")
                    return self.poll(body.get("device_code"), "opencode")
                if self.path == "/start":
                    owner.inferences["zcode"] += 1
                    if self.headers.get("Authorization") != "Bearer " + owner.issued["zcode"][-1]:
                        owner.errors.append("stale ZCode inference snapshot")
                    return self.answer({"type": "message", "id": "fixture", "role": "assistant",
                                        "model": "GLM-5.3", "content": [{"type": "text", "text": "fixture"}],
                                        "stop_reason": "end_turn", "usage": {"input_tokens": 1, "output_tokens": 1}})
                if self.path == "/inference/go/openai/v1/chat/completions":
                    owner.inferences["opencode"] += 1
                    if self.headers.get("Authorization") != "Bearer " + owner.current_access:
                        owner.errors.append("stale OpenCode inference snapshot")
                    return self.answer({"choices": [{"index": 0, "message": {"role": "assistant", "content": "fixture"},
                                                     "finish_reason": "stop"}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}})
                owner.errors.append("unexpected vendor POST " + self.path.split("?", 1)[0][:200])
                self.answer({}, 404)

            def poll(self, identity, kind):
                with owner.lock:
                    flow = owner.flows.get(identity)
                    if not flow or flow["kind"] != kind:
                        owner.errors.append("foreign vendor flow")
                        return self.answer({}, 400)
                    if kind == "zcode" and self.headers.get("Authorization") != flow["authorization"]:
                        owner.errors.append("ZCode private polling channel")
                    flow["polls"] += 1
                    owner.polled[kind] += 1
                    mode, count = flow["mode"], flow["polls"]
                    if mode == "hold":
                        flow["held"] = True
                if mode == "hold" and not owner.release.wait(15):
                    return self.answer({"private": "private-error-canary"}, 504)
                if mode == "error":
                    status = 401 if kind == "zcode" else 503
                    return self.answer({"error": "private-error-canary", "credentials": "private-credential-canary"}, status)
                if mode == "success" and count == 1:
                    if kind == "zcode":
                        return self.answer({"code": 0, "data": {"status": "pending"}})
                    return self.answer({"error": "authorization_pending"}, 400)
                if kind == "zcode":
                    token = owner.zcode_token(identity)
                    with owner.lock:
                        owner.issued[kind].append(token)
                    return self.answer({"code": 0, "data": {
                        "status": "ready", "token": token,
                        "user": {"user_id": "zcode-user"},
                    }})
                with owner.lock:
                    owner.issued[kind].append("private-opencode-access-" + identity)
                    owner.current_access = owner.issued[kind][-1]
                return self.answer({
                    "access_token": "private-opencode-access-" + identity, "refresh_token": "private-opencode-refresh-" + identity,
                    "expires_in": 10 if len(owner.issued[kind]) == 1 else 3600, "org_id": "organization",
                })

            def do_GET(self):
                if self.path == "/__fixture__/state":
                    with owner.lock:
                        return self.answer({
                            "started": dict(owner.started), "polled": dict(owner.polled),
                            "held": sum(int(value["held"]) for value in owner.flows.values()),
                            "refreshes": owner.refreshes, "inferences": dict(owner.inferences),
                            "errors": list(owner.errors),
                        })
                if self.path.startswith("/api/v1/oauth/cli/poll/"):
                    return self.poll(self.path.rsplit("/", 1)[1], "zcode")
                if self.path.startswith("/api/v1/zcode-plan/billing/balance?"):
                    latest = owner.issued["zcode"][-1] if owner.issued["zcode"] else ""
                    if self.headers.get("Authorization") != "Bearer " + latest:
                        owner.errors.append("ZCode entitlement credential channel")
                    owner.observed_usage["zcode"].append(self.headers.get("Authorization"))
                    return self.answer({"code": 0, "data": {
                        "plans": [{"plan_id": "start", "user_plan_id": "owned", "status": "active"}],
                        "balances": [{"plan_id": "start", "user_plan_id": "owned",
                                      "remaining_units": 7, "total_units": 10, "capabilities": ["model:GLM-5.3"]}],
                    }})
                if self.path.startswith("/api/v1/client/configs?"):
                    return self.answer({"code": 0, "data": {"configs": {"builtin_provider_config_json": owner.origin + "/public-config"}}})
                if self.path == "/public-config":
                    return self.answer({"config": {
                        "providerConfigRules": {"providerRules": [{
                            "providerId": "account:zai-start-plan", "config": {"builtinModelIds": ["GLM-5.3"]},
                        }]},
                        "modelConfigRules": {"modelRules": [{
                            "modelMatch": ".*", "config": {"enabled": True, "properties": {"contextWindow": 256000}},
                        }]},
                    }})
                if self.path in ("/console/api/user", "/console/api/orgs"):
                    latest = owner.current_access
                    if self.headers.get("Authorization") != "Bearer " + latest:
                        owner.errors.append("OpenCode identity credential channel")
                    if self.path.endswith("/user"):
                        return self.answer({"id": "opencode-user", "email": "user@example.invalid"})
                    return self.answer([{"id": "organization", "name": "Synthetic organization"}])
                if self.path == "/zen/go/v1/usage":
                    latest = owner.current_access
                    if self.headers.get("Authorization") != "Bearer " + latest:
                        owner.errors.append("OpenCode selected usage credential channel")
                    owner.observed_usage["opencode"].append(self.headers.get("Authorization"))
                    return self.answer({"usage": {"rolling": {"percent": 23}}})
                if self.path.split("?", 1)[0] != "/updates":
                    owner.errors.append("unexpected vendor GET " + self.path.split("?", 1)[0][:200])
                self.answer({}, 404)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        self.thread = threading.Thread(target=self.server.serve_forever)

    @property
    def origin(self):
        return f"http://127.0.0.1:{self.server.server_port}"

    @property
    def secrets(self):
        return [*[self.zcode_token(f"private-zcode-flow-{number}") for number in range(1, 21)],
                "private-zcode-", "private-opencode-", "private-pkce-canary",
                "private-error-canary", "private-credential-canary"]

    def zcode_token(self, identity):
        payload = base64.urlsafe_b64encode(json.dumps({
            "exp": self.expiry, "private": "private-zcode-session-canary", "flow": identity,
        }).encode()).decode().rstrip("=")
        return "e30." + payload + ".synthetic"

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *args):
        self.release.set()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
