import argparse
import contextlib
import hashlib
import http.cookiejar
import importlib.util
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import zipfile


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def observe_vendor(vendor):
    observed = {"tickets": 0, "draws": 0, "native": 0, "manualGiftReads": 0, "requests": []}
    base = vendor.server.RequestHandlerClass

    class Handler(base):
        def do_GET(self):
            if (self.path.startswith("/api/v1/zcode-plan/billing/balance?")
                    and self.headers.get("Authorization") == "Bearer synthetic-paid-session"):
                if (self.path != "/api/v1/zcode-plan/billing/balance?app_version=3.18.2"
                        or self.headers.get("X-Device-Mid") != "synthetic-paid-device"):
                    vendor.errors.append("manual gift selected device/version channel")
                with vendor.lock:
                    observed["manualGiftReads"] += 1
                return self.answer({"code": 0, "data": {"plans": [], "balances": []}})
            if self.path == "/api/v1/oauth/profile":
                allowed = ["Bearer " + token for token in vendor.issued["zcode"]]
                allowed.append("Bearer synthetic-paid-session")
                if self.headers.get("Authorization") not in allowed:
                    vendor.errors.append("profile credential channel")
                return self.answer({"code": 0, "data": {"user": {"user_id": "zcode-user"}}})
            if self.path == "/quota":
                return self.answer({"code": 0, "data": {"limits": [{
                    "type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 20,
                }]}})
            return super().do_GET()

        def do_POST(self):
            if self.path != "/start" and not self.path.startswith(("/api/v1/off-peak/", "/api/v1/highspeed/")):
                return super().do_POST()
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            if self.path == "/start":
                with vendor.lock:
                    vendor.inferences["zcode"] += 1
                    if self.headers.get("Authorization") != "Bearer " + vendor.issued["zcode"][-1]:
                        vendor.errors.append("Start selected credential channel")
                    observed["requests"].append({"path": self.path, "session": self.headers.get("X-Session-Id"),
                                                 "query": self.headers.get("X-Query-Id"),
                                                 "trace": self.headers.get("X-ZCode-Trace-Id"),
                                                 "request": self.headers.get("X-Request-Id"),
                                                 "identity": json.loads(body["metadata"]["user_id"])})
            else:
                if self.headers.get("Authorization") != "Bearer synthetic-paid-session":
                    vendor.errors.append("native selected session channel")
                if "/off-peak/" in self.path and self.headers.get("X-Coding-Plan-Api-Key") != "synthetic-paid-project":
                    vendor.errors.append("OffPeak selected project channel")
                if "/highspeed/" in self.path and self.headers.get("X-Bigmodel-Authorization") != "Bearer synthetic-paid-business":
                    vendor.errors.append("Highspeed selected business channel")
                if self.path == "/api/v1/off-peak/ticket":
                    observed["tickets"] += 1
                    if body != {"task_id": "offpeak-conversation"}:
                        vendor.errors.append("OffPeak exact host conversation")
                    return self.answer({"ticket_id": "owned-ticket", "task_id": body["task_id"], "state": "ready"})
                if self.path == "/api/v1/highspeed/draw":
                    observed["draws"] += 1
                    if body != {"task_id": "highspeed-conversation", "provider": "builtin:zai-coding-plan", "model": "GLM-5.3"}:
                        vendor.errors.append("Highspeed exact host conversation/model")
                    return self.answer({"code": 0, "data": {"next_draw_at": 0, "card": {
                        "card_id": "owned-card", "task_id": body["task_id"], "model": body["model"],
                        "provider": body["provider"], "expires_at": int(time.time() * 1000) + 3600000,
                    }}})
                if self.path not in {"/api/v1/off-peak/anthropic/v1/messages", "/api/v1/highspeed/anthropic/v1/messages"}:
                    vendor.errors.append("unexpected native POST " + self.path)
                    return self.answer({}, 404)
                observed["native"] += 1
                ticket, card = self.headers.get("X-Off-Peak-Ticket-ID"), self.headers.get("X-Highspeed-Card-ID")
                expected = ("owned-ticket", None) if "/off-peak/" in self.path else (None, "owned-card")
                if (ticket, card) != expected or body["model"] != "GLM-5.3":
                    vendor.errors.append("native alias credential/model ownership")
                observed["requests"].append({"path": self.path, "session": self.headers.get("X-Session-Id"),
                                             "query": self.headers.get("X-Query-Id"),
                                             "trace": self.headers.get("X-ZCode-Trace-Id"),
                                             "request": self.headers.get("X-Request-Id"),
                                             "identity": json.loads(body["metadata"]["user_id"])})
            return self.answer({"type": "message", "id": "fixture", "role": "assistant",
                                "model": "GLM-5.3", "content": [{"type": "text", "text": "fixture"}],
                                "stop_reason": "end_turn", "usage": {
                                    "input_tokens": 7, "output_tokens": 5,
                                    "cache_read_input_tokens": 11, "cache_creation_input_tokens": 3,
                                }})

    vendor.server.RequestHandlerClass = Handler
    return observed


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("host", type=Path)
    parser.add_argument("--overlay", type=Path)
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    host = args.host.resolve()
    spec = importlib.util.spec_from_file_location("zcode_fixture_ingress", Path(__file__).with_name("ingress.py"))
    ingress = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(ingress)
    with tempfile.TemporaryDirectory(prefix="zcode-owned-agent-") as scratch, ingress.owned_vendor(source, Path(scratch)) as vendor, contextlib.ExitStack() as cleanup:
        observed = observe_vendor(vendor)
        root = Path(scratch)
        home = root / "home with spaces {key}"
        home.mkdir(mode=0o700)
        env = os.environ.copy()
        env.update(json.loads(subprocess.check_output(["go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"])))
        for name in list(env):
            if name.lower().endswith("_proxy") or name in {
                "MAGPIE_ADDR", "MAGPIE_WEB_KEY", "MAGPIE_UPDATE_FEED",
                "ZCODE_DATA_BASE_DIR", "ZCODE_PERSONAL_PROVIDER_CONFIG_FILE",
            }:
                env.pop(name, None)
        original_client = shutil.which("zcode", path=env["PATH"])
        assert original_client is None, "fixture expects the previously audited absent actual ZCode client"
        env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                   XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                   XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                   LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
                   GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
                   CGO_ENABLED="1", SHELL="/bin/bash")
        env["MAGPIE_ADDR"] = f"127.0.0.1:{free_port()}"
        env["MAGPIE_UPDATE_FEED"] = vendor.origin + "/updates"
        gateway = "http://" + env["MAGPIE_ADDR"]
        distribution = root / "distribution"
        with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
            archive.extractall(distribution)
        manifest = json.loads((distribution / "magpie-plugin.json").read_bytes())
        assert manifest["runtime"] == "native" and len(manifest["agents"]) == 1
        assert manifest["agents"][0]["id"] == "zcode-native" and manifest["agents"][0]["profile"] == {"target": "zcode"}
        production_library = hashlib.sha256((distribution / manifest["entry"]).read_bytes()).hexdigest()
        fields = ",".join(name + ":" + json.dumps(vendor.origin + path) for name, path in {
            "Zai": "/paid", "BigModel": "/paid", "Start": "/start",
            "ZaiQuota": "/quota", "BigModelQuota": "/quota", "LegacyBigModelQuota": "/quota",
        }.items())
        entry = root / "entry.go"
        entry.write_text(
            'package main\nimport(z "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode";'
            '"github.com/yetone/magpie/pkg/extension/native")\n'
            'func init(){if err:=native.Register(z.NewControlModule(z.ModuleOptions{Endpoints:z.Endpoints{'
            + fields + '}}));err!=nil{panic(err)}}\nfunc main(){}\n')
        replacements = {}
        if args.overlay:
            replacements.update(json.loads(args.overlay.read_bytes())["Replace"])
        assert str(source / "cmd/native-ui/main.go") not in replacements
        replacements[str(source / "cmd/native-ui/main.go")] = str(entry)
        overlay = root / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}))
        subprocess.run(["go", "build", "-overlay", str(overlay), "-buildmode=c-shared",
                        "-o", str(distribution / manifest["entry"]), "./cmd/native-ui"],
                       cwd=source, env=env, check=True, timeout=120)
        fixture_library = hashlib.sha256((distribution / manifest["entry"]).read_bytes()).hexdigest()

        def cli(*arguments, succeeds=True):
            result = subprocess.run([str(host), *arguments], cwd=root, env=env, text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=45)
            assert (result.returncode == 0) == succeeds, (
                f"isolated host command {arguments[:3]} exit {result.returncode}: {result.stderr[-2000:]}")
            return result.stdout

        cli("plugin", "sdk", "add", str(distribution))
        config = json.loads(cli("plugin", "sdk", "config", "zcode"))
        settings = {"dataDir": str(home / "controller")}
        cli("plugin", "sdk", "config", "zcode", json.dumps(settings), "--expected", config["version"])
        started = json.loads(cli("plugin", "sdk", "auth", "start", "zcode", "zcode",
                                 json.dumps({"method": "browser", "input": {"site": "zai", "kind": "start"}})))
        authorization = None
        for _ in range(6):
            time.sleep(max(started.get("pollAfterMs", 1000), 1000) / 1000 + 0.02)
            authorization = json.loads(cli("plugin", "sdk", "auth", "finish", "zcode", "zcode",
                                           json.dumps({"continuation": started["continuation"]})))
            if authorization.get("id"):
                break
        assert authorization and authorization.get("id"), "native auth did not complete"
        account_id = authorization["id"]
        provider_id = "extension-" + hashlib.sha256(b"zcode\0zcode").hexdigest()[:24]
        target_model = provider_id + "/GLM-5.3"
        agents_dir = home / ".zcode"
        provider_file = agents_dir / "v2/provider_config.json"
        provider_file.parent.mkdir(parents=True)
        original = {
            "schemaVersion": 1, "config": {
                "providerConfigRules": {"providerRules": [{"providerId": "personal", "providerName": "Personal",
                                                          "config": {"group": "standard-personal"}}]},
                "modelConfigRules": {"providerModelRules": [], "manualProviderModelRules": []},
                "defaultModelSelection": {"providerId": "personal", "modelId": "personal-model"},
            },
        }
        provider_file.write_text(json.dumps(original))
        provider_file.chmod(0o600)
        personal_instructions = agents_dir / "AGENTS.md"
        personal_instructions.write_text("Personal instructions survive\n")
        runtime = agents_dir / "cli/config.json"
        runtime.parent.mkdir()
        runtime.write_text(json.dumps({"ui": {"locale": "zh-CN"}, "mcp": {"servers": {
            "personal": {"type": "stdio", "command": "personal-command", "args": [], "enabled": True},
        }}}))
        (home / ".bashrc").write_text("# Personal shell text\n")
        binary_dir = home / "bin"
        binary_dir.mkdir()
        client = binary_dir / "zcode"
        client.write_text(
            "#!" + sys.executable + "\n"
            "import json,os,sys,urllib.request\n"
            "from pathlib import Path\n"
            "args=sys.argv[1:]\n"
            "path=Path(os.environ['ZCODE_PERSONAL_PROVIDER_CONFIG_FILE'])\n"
            "config=json.loads(path.read_text())['config']\n"
            "selection=config['defaultModelSelection']\n"
            "provider=next(p for p in config['providerConfigRules']['providerRules'] if p['providerId']==selection['providerId'])['config']\n"
            "assert selection['providerId']=='magpie' and provider['access']['type']=='api-key'\n"
            "session=args[args.index('--fixture-session')+1]\n"
            "model=args[args.index('--fixture-model')+1] if '--fixture-model' in args else selection['modelId']\n"
            "headers={'Content-Type':'application/json','X-Api-Key':provider['access']['apiKey'],'X-Session-ID':session,'User-Agent':'zcode/synthetic-argv'}\n"
            "if '--fixture-forged' in args: headers[args[args.index('--fixture-forged')+1]]='forged-not-owned'\n"
            "request=urllib.request.Request(provider['api']['baseUrl'].rstrip('/')+'/v1/messages',"
            "data=json.dumps({'model':model,'max_tokens':32,'messages':[{'role':'user','content':args[-1]}]}).encode(),"
            "headers=headers)\n"
            "with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request,timeout=15) as response:\n"
            " result=json.loads(response.read())\n"
            "print(json.dumps({'syntheticClient':True,'args':args,'config':str(path),'model':selection['modelId'],'result':result}))\n")
        client.chmod(0o700)
        env["PATH"] = str(binary_dir) + ":" + env["PATH"]
        web_port = free_port()
        origin = f"http://127.0.0.1:{web_port}"
        env["MAGPIE_WEB_KEY"] = "synthetic-owned-agent-web"
        log = cleanup.enter_context((root / "web.log").open("wb"))
        process = subprocess.Popen([str(host), "web", "--addr", f"127.0.0.1:{web_port}", "--no-open"],
                                   cwd=root, env=env, stdout=log, stderr=log)

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
        deadline = time.monotonic() + 35
        while True:
            try:
                opener.open(origin + "/?k=synthetic-owned-agent-web", timeout=2).read()
                break
            except OSError:
                assert process.poll() is None and time.monotonic() < deadline, (root / "web.log").read_text()[-3000:]
                time.sleep(0.1)
        calls = []

        def api(path, body=None, succeeds=True):
            request = urllib.request.Request(origin + path, data=None if body is None else json.dumps(body).encode(),
                                             headers={"Content-Type": "application/json", "Origin": origin,
                                                      "Sec-Fetch-Site": "same-origin"})
            try:
                with opener.open(request, timeout=45) as response:
                    status, result = response.status, response.read()
            except urllib.error.HTTPError as error:
                status, result = error.code, error.read()
            calls.append({"path": path, "status": status})
            assert (status == 200) == succeeds, f"{path}: {status} {result[:1200]!r}"
            for secret in vendor.secrets:
                assert secret.encode() not in result, "HTTP page response disclosed private auth"
            return json.loads(result)

        def row(state):
            return next(agent for agent in state["agents"] if agent["id"] == "zcode-native")

        before = api("/api/state")
        assert not row(before).get("wired", False) and any(agent["id"] == "zcode" for agent in before["agents"])
        baseline_hash = hashlib.sha256(provider_file.read_bytes()).hexdigest()
        preview_request = {"id": "zcode-native", "action": "connect",
                           "options": {"gateway": gateway, "key": "preview-only-gateway-key",
                                       "models": [{"value": "magpie/" + target_model}]}}
        cli("plugin", "sdk", "agent", "zcode", json.dumps(preview_request),
            "--root", str(home), "--path", ".zcode/v2/provider_config.json", succeeds=False)
        assert hashlib.sha256(provider_file.read_bytes()).hexdigest() == baseline_hash
        connected = api("/api/agents/connect/zcode-native", {})
        assert row(connected)["wired"] and not any(agent["id"] == "zcode" for agent in connected["agents"])
        cli("zcode-native", "model", target_model)
        launch_environment = cli("zcode-native", "env")
        assert "ZCODE_PERSONAL_PROVIDER_CONFIG_FILE=" + str(provider_file) in launch_environment
        assert "{key}" in str(provider_file) and "ZCODE_PERSONAL_PROVIDER_CONFIG_FILE={config}" not in launch_environment
        prompt = "literal spaces; $(touch should-not-exist) `no-shell`"
        launched = json.loads(cli("zcode-native", "run", "--fixture-session", "owned-conversation", prompt))
        assert launched["syntheticClient"] and launched["config"] == str(provider_file)
        assert launched["args"] == ["--fixture-session", "owned-conversation", prompt]
        assert launched["model"] == target_model and launched["result"]["content"][0]["text"] == "fixture"
        assert not (root / "should-not-exist").exists() and vendor.inferences["zcode"] == 1
        for suffix, header in (("OffPeak", "X-Off-Peak-Ticket-ID"), ("Highspeed", "X-Highspeed-Card-ID")):
            cli("zcode-native", "run", "--fixture-session", "unpaid-forged",
                "--fixture-model", target_model + "-" + suffix, "--fixture-forged", header, prompt, succeeds=False)
        assert vendor.inferences["zcode"] == 1 and observed["tickets"] == observed["draws"] == observed["native"] == 0
        api("/api/provider/save", {"id": provider_id, "passHeaders": [
            "X-Off-Peak-Ticket-ID", "X-Highspeed-Card-ID",
        ]})
        for header in ("X-Off-Peak-Ticket-ID", "X-Highspeed-Card-ID"):
            cli("zcode-native", "run", "--fixture-session", "forged", "--fixture-forged", header, prompt, succeeds=False)
        assert vendor.inferences["zcode"] == 1 and observed["tickets"] == observed["draws"] == observed["native"] == 0
        for secret in vendor.secrets:
            assert secret not in json.dumps(launched) and secret not in provider_file.read_text()
        shared = root / "shared.txt"
        shared.write_text("Owned profile guidance\n")
        cli("library", "instructions", "set", str(shared))
        cli("library", "instructions", "agents", "zcode-native")
        assert "Owned profile guidance" in personal_instructions.read_text()
        assert "Personal instructions survive" in personal_instructions.read_text()
        cli("library", "mcp", "router", "off")
        cli("library", "mcp", "add", "owned-fixture", "fixture-mcp", "literal argument", "agents=zcode-native")
        runtime_config = json.loads(runtime.read_bytes())
        assert runtime_config["mcp"]["servers"]["personal"]["command"] == "personal-command"
        assert runtime_config["mcp"]["servers"]["owned-fixture"]["args"] == ["literal argument"]
        assert runtime_config["ui"]["locale"] == "zh-CN"
        api("/api/library/skills/create", {"name": "owned-agent-skill", "description": "Synthetic profile skill",
                                          "agents": ["zcode"]})
        cli("library", "skill", "agents", "owned-agent-skill", "zcode-native")
        skill = agents_dir / "skills/owned-agent-skill/SKILL.md"
        assert skill.is_file() and "owned-agent-skill" in skill.read_text()
        cli("library", "instructions", "agents", "none")
        cli("library", "mcp", "agents", "owned-fixture", "none")
        cli("library", "skill", "agents", "owned-agent-skill", "none")
        assert "Owned profile guidance" not in personal_instructions.read_text()
        assert "Personal instructions survive" in personal_instructions.read_text()
        assert "owned-fixture" not in json.loads(runtime.read_bytes())["mcp"]["servers"]
        assert not skill.exists()
        credentials = {
            "site": "zai", "kind": "individual", "userId": "zcode-user", "device": "synthetic-paid-device",
            "projectToken": "synthetic-paid-project", "expiresAt": int(time.time() * 1000) + 3600000,
            "sessionToken": "synthetic-paid-session", "sessionExpiresAt": int(time.time() * 1000) + 3600000,
            "businessToken": "synthetic-paid-business", "businessExpiresAt": int(time.time() * 1000) + 3600000,
        }
        paid = json.loads(cli("plugin", "sdk", "auth", "finish", "zcode", "zcode", json.dumps({"input": credentials})))
        paid_id = paid["id"]
        for suffix, session, repeats in [("OffPeak", "offpeak-conversation", 2), ("Highspeed", "highspeed-conversation", 1)]:
            model = target_model + "-" + suffix
            api("/api/set", {"agent": "zcode-native", "field": "model", "value": model})
            for _ in range(repeats):
                result = json.loads(cli("zcode-native", "run", "--fixture-session", session, prompt))
                assert result["model"] == model and result["result"]["content"][0]["text"] == "fixture"
        assert observed["tickets"] == observed["draws"] == 1 and observed["native"] == 3
        journal_files = list((home / "controller").glob("*.json"))
        journals = [json.loads(path.read_bytes()) for path in journal_files]
        entries = [entry for journal in journals for entry in journal.get("executions", {}).values()]
        assert {entry["task"] for entry in entries} == {"offpeak-conversation", "highspeed-conversation"}
        assert all(entry["account"] == paid_id and entry["generation"] == 1 for entry in entries)
        assert sorted(len(entry["outcomes"]) for entry in entries) == [1, 2]
        for request, session in zip(observed["requests"], ["owned-conversation", "offpeak-conversation", "offpeak-conversation", "highspeed-conversation"], strict=True):
            digest = bytearray(hashlib.sha256(("zcode-session\0" + session).encode()).digest())
            digest[6], digest[8] = digest[6] & 15 | 64, digest[8] & 63 | 128
            text = digest[:16].hex()
            expected = "-".join([text[:8], text[8:12], text[12:16], text[16:20], text[20:32]])
            assert request["session"] == request["identity"]["session_id"] == expected
            assert all(isinstance(request[key], str) and len(request[key]) == 36
                       for key in ("query", "trace", "request")), "native attribution is missing"
        assert observed["requests"][1]["query"] == observed["requests"][2]["query"]
        assert len({request["request"] for request in observed["requests"]}) == 4
        records = [json.loads(line) for path in home.rglob("usage.jsonl")
                   for line in path.read_text().splitlines() if line]
        measured = [record for record in records if record.get("provider") == provider_id and
                    any(record.get(key, 0) > 0 for key in ("in", "out", "cache_read", "cache_write"))]
        assert len(measured) == 4 and all(
            [record.get(key) for key in ("in", "out", "cache_read", "cache_write")] == [7, 5, 11, 3]
            for record in measured), "ordinary gateway ledger lost native token/cache usage"
        api("/api/set", {"agent": "zcode-native", "field": "model", "value": target_model})
        document = json.loads(provider_file.read_bytes())
        document["config"]["providerConfigRules"]["providerRules"][0]["providerName"] = "User changed personal provider"
        provider_file.write_text(json.dumps(document))
        cli("zcode-native", "run", "--fixture-session", "stale-config", prompt, succeeds=False)
        api("/api/set", {"agent": "zcode-native", "field": "model", "value": target_model})
        before_disconnect = provider_file.read_bytes()
        preview = api("/api/agents/preview/zcode-native")
        assert preview.get("revision") and preview.get("changes")
        assert provider_file.read_bytes() == before_disconnect
        api("/api/agents/disconnect-offline/zcode-native", {"revision": "stale"}, succeeds=False)
        assert provider_file.read_bytes() == before_disconnect
        api("/api/agents/disconnect/zcode-native", {})
        restored = json.loads(provider_file.read_bytes())
        assert restored["config"]["defaultModelSelection"] == original["config"]["defaultModelSelection"]
        assert restored["config"]["providerConfigRules"]["providerRules"] == [{
            "providerId": "personal", "providerName": "User changed personal provider", "config": {"group": "standard-personal"},
        }]
        assert "# Personal shell text" in (home / ".bashrc").read_text()
        assert "sdk-agent" not in (home / ".bashrc").read_text()
        api("/api/agents/connect/zcode-native", {})
        config = json.loads(cli("plugin", "sdk", "config", "zcode"))
        cli("plugin", "sdk", "config", "zcode", json.dumps({"dataDir": str(home / "controller-next")}),
            "--expected", config["version"])
        cli("zcode-native", "run", "--fixture-session", "stale-module-config", prompt, succeeds=False)
        api("/api/set", {"agent": "zcode-native", "field": "model", "value": target_model})
        cli("plugin", "sdk", "off", "zcode")
        cli("zcode-native", "run", "--fixture-session", "disabled", prompt, succeeds=False)
        assert vendor.inferences["zcode"] == 1 and observed["manualGiftReads"] > 0 and not vendor.errors, vendor.errors
        catalogs = [json.loads(path.read_bytes()) for path in home.rglob("catalog.json")]
        installed = [item for catalog in catalogs if isinstance(catalog, list) for item in catalog]
        assert len(installed) == 1 and installed[0]["manifest"]["id"] == "zcode"
        accounts = [account for path in home.rglob("accounts.json") for account in json.loads(path.read_bytes())["accounts"]]
        assert len(accounts) == 2 and {account["id"] for account in accounts} == {account_id, paid_id}
        assert all(account["generation"] == 1 for account in accounts)
        for path in home.rglob("plugins.json"):
            assert not json.loads(path.read_bytes()).get("plugins")
        for path in home.rglob("plugin-auth.json"):
            assert not json.loads(path.read_bytes())
        assert not list(home.rglob("node_modules"))
        result = {
            "passed": True, "hostSHA256": hashlib.sha256(host.read_bytes()).hexdigest(),
            "productionLibrarySHA256": production_library, "syntheticEndpointLibrarySHA256": fixture_library,
            "singleNativeCatalogEntry": True, "singleSDKAccountStore": True,
            "ordinaryConnectSetInspectDisconnect": True, "exactPreviewAndStaleRevisionRefusal": True,
            "literalConfigPathAndArgv": True, "foreignConfigAndShellPreserved": True,
            "libraryInstructionsMCPAndSkillRoundtrip": True, "staleFileConfigOffLaunchRefused": True,
            "ordinaryGatewayInferences": vendor.inferences["zcode"], "httpCalls": calls,
            "ordinaryNativeAliasInferences": observed["native"],
            "offPeakTicketMints": observed["tickets"], "highspeedCardDraws": observed["draws"],
            "exactConversationInJournalAndVendorIdentity": True, "forwardedForgedLaneHeadersRefused": True,
            "ordinaryGatewayLedgerPreservesTokenAndCacheUsage": True,
            "nativeTurnTraceAndRequestAttribution": True,
            "manualGiftDiscoveryUsesSelectedSessionDeviceAndVersion": True,
            "defaultFilteredForgedHeadersGrantNoNativeAdmission": True,
            "actualClientPresent": False, "syntheticClientOnly": True,
            "strictVendorIngress": True, "rejectedForeignConnections": vendor.ingress.rejected,
        }
        print(json.dumps(result))


if __name__ == "__main__":
    main()
