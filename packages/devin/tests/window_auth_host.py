"""Installed Devin through Providers and saved sign-in preparation, synthetic accounts only."""

import argparse
from contextlib import ExitStack
import hashlib
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

from conformance import fields, number_field, text_field
from http_e2e import Gateway, DenyProxy, SERVICE, digest, require, server_thread

MODULE = "devin-inbound"
PROVIDER = "devin"
PROVIDER_ROW = "extension-" + hashlib.sha256(f"{MODULE}\0{PROVIDER}".encode()).hexdigest()[:24]
FIRST_NAME = "same@example.invalid · Fixture organization"
UPDATED_NAME = "updated@example.invalid · Fixture organization"
FIRST_SAVED_KEY = "synthetic-saved-entry-a"
ROTATED_SAVED_KEY = "synthetic-saved-entry-a-rotated"
SECOND_SAVED_KEY = "synthetic-saved-entry-b"
UNVERIFIED_SAVED_KEY = "synthetic-saved-entry-without-uid"
SAVED_KEYS = (FIRST_SAVED_KEY, ROTATED_SAVED_KEY, SECOND_SAVED_KEY)
CLI_SAVED_KEY = "synthetic-cli-saved"
CLI_CURRENT_KEY = "synthetic-cli-current"
CLI_FOREIGN_KEY = "synthetic-cli-foreign"
CLI_RENAME = "cli-current@example.invalid"
CLI_KEYS = (CLI_SAVED_KEY, CLI_CURRENT_KEY)
PRIVATE_KEYS = (*SAVED_KEYS, UNVERIFIED_SAVED_KEY, *CLI_KEYS, CLI_FOREIGN_KEY)


def recognized_key(key):
    return key.startswith("synthetic-window-session-") or key in PRIVATE_KEYS


class Endpoint(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 < size < 65536, "invalid synthetic auth request size")
            body = self.rfile.read(size)
            if self.path == SERVICE + "GetCliModelConfigs":
                key = fields(fields(body)[1][0])[3][0].decode()
                require(recognized_key(key), "catalog used an unknown credential")
                with self.server.lock:
                    self.server.catalog_keys.append(key)
                model = "synthetic-devin-signin-model"
                data = text_field(1, text_field(1, model) + text_field(22, model) + number_field(18, 65536))
                self.reply("application/proto", data)
                return
            value = json.loads(body)
            if self.path.endswith("/ExchangeDevinCLIPKCECode"):
                require(set(value) == {"code", "code_verifier"} and value["code"] == "synthetic-gui-code"
                        and 43 <= len(value["code_verifier"]) <= 128, "GUI PKCE exchange changed")
                with self.server.lock:
                    self.server.exchanges += 1
                    identity = self.server.exchanges
                if self.server.hold:
                    self.server.arrived.set()
                    require(self.server.release.wait(15), "held PKCE exchange did not release")
                result = {"sessionToken": "synthetic-window-session-" + str(identity)}
            elif self.path.endswith("/GetUserStatus"):
                key = value["metadata"]["apiKey"]
                require(recognized_key(key), "unexpected credential")
                email = CLI_RENAME if key == CLI_CURRENT_KEY else "same@example.invalid"
                if key == "synthetic-window-session-2" and self.server.renamed:
                    email = "updated@example.invalid"
                if key in CLI_KEYS:
                    subject = "linked"
                elif key in (SECOND_SAVED_KEY, CLI_FOREIGN_KEY):
                    subject = "saved-b"
                else:
                    subject = "saved-a" if key in SAVED_KEYS else "browser"
                with self.server.lock:
                    self.server.status_keys.append(key)
                result = {"userStatus": {"userId": "synthetic-user-" + subject, "teamId": "fixture-team-" + subject, "email": email,
                          "planStatus": {"planInfo": {"devinInfo": {
                              "orgId": "fixture-organization",
                              "accountDisplayName": "Fixture organization",
                          }}}}}
                if key == UNVERIFIED_SAVED_KEY:
                    result = {"userStatus": {"email": email}}
            else:
                raise AssertionError("unexpected synthetic auth endpoint")
            self.reply("application/json", json.dumps(result).encode())
        except Exception as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.send_error(500, "synthetic endpoint assertion failed")

    def reply(self, media_type, data):
        self.send_response(200)
        self.send_header("Content-Type", media_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def run(host_binary, distribution, output, *, paired_only=False):
    require(not output.exists(), "evidence output must be new")
    output.mkdir()
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    report = {"test": "TestInstalledDevinProvidersSignIn", "cases": [],
              "hostSHA256": digest(host_binary), "librarySHA256": digest(distribution / manifest["entry"]),
              "runnerSHA256": digest(__file__),
              "moduleVersion": manifest["version"], "variant": "paired" if paired_only else "complete",
              "runtime": "actual Providers HTTP broker, saved sign-in CLI, native module and loopback receiver; no rendered browser"}
    try:
        with ExitStack() as stack:
            vendor = ThreadingHTTPServer(("127.0.0.1", 0), Endpoint)
            vendor.lock, vendor.exchanges, vendor.failures = threading.Lock(), 0, []
            vendor.status_keys, vendor.catalog_keys = [], []
            vendor.hold, vendor.arrived, vendor.release = False, threading.Event(), threading.Event()
            vendor.renamed = False
            server_thread(stack, vendor)
            stack.callback(vendor.release.set)
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-gui-")))
            host = Gateway(host_binary, distribution, home, output, "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(host.close)
            host.env.update(MAGPIE_NO_ACCOUNTS="0", MAGPIE_ADDR="127.0.0.1:0",
                            MAGPIE_WEB_KEY="synthetic-devin-gui-entry")
            report["hostVersion"] = host.command("--version").strip()
            settings = json.loads((host.config / "settings.json").read_text())
            settings["noAccountDiscovery"] = False
            (host.config / "settings.json").write_text(json.dumps(settings))
            for name in ("models.json", "labs.json"):
                (host.cache / name).write_text("{}")
            host.command("plugin", "sdk", "add", str(distribution))
            snapshot = json.loads(host.command("plugin", "sdk", "config", MODULE))
            server = "http://127.0.0.1:" + str(vendor.server_port)
            host.command("plugin", "sdk", "config", MODULE, json.dumps({"server": server}),
                         "--expected", snapshot["version"])
            host.log = (output / "host.log").open("w")
            host.process = subprocess.Popen(
                [str(host_binary), "web", "--addr", "127.0.0.1:" + str(host.port), "--no-open"],
                cwd=home, env=host.env, stdout=host.log, stderr=subprocess.STDOUT, start_new_session=True)
            origin = "http://127.0.0.1:" + str(host.port)
            opener = urllib.request.build_opener(
                urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            deadline = time.monotonic() + 20
            while True:
                require(host.process.poll() is None, "GUI host exited")
                try:
                    with opener.open(origin + "/?k=" + host.env["MAGPIE_WEB_KEY"], timeout=2) as response:
                        if response.status == 200:
                            break
                except OSError:
                    require(time.monotonic() < deadline, "GUI host failed to start")
                    time.sleep(0.05)

            def request(path, value=None, success=True, refusal=None):
                body = None if value is None else json.dumps(value).encode()
                message = urllib.request.Request(origin + path, data=body,
                    headers={"Content-Type": "application/json", "Origin": origin})
                try:
                    with opener.open(message, timeout=20) as response:
                        public = response.read(1 << 20).decode()
                    require_public(public)
                    require(success is not False, "revoked GUI request succeeded")
                    return json.loads(public)
                except urllib.error.HTTPError as denial:
                    with denial:
                        public = denial.read(1 << 20).decode()
                    require_public(public)
                    require(success is not True, f"{path} returned HTTP {denial.code}: {public[:1200]}")
                    if refusal is not None:
                        require(denial.code == 400 and refusal in json.loads(public).get("error", ""),
                                f"{path} refused for the wrong reason: {public[:1200]}")
                    return None

            def accounts():
                public = host.command("plugin", "sdk", "accounts", MODULE, PROVIDER, "--json")
                require_public(public)
                return json.loads(public)

            def account_generations():
                return {item["id"]: item["generation"] for item in accounts()}

            def vendor_keys():
                with vendor.lock:
                    return ([key for key in vendor.status_keys if key in PRIVATE_KEYS],
                            [key for key in vendor.catalog_keys if key in PRIVATE_KEYS])

            def require_public(text):
                require(not any(secret in text for secret in (
                    "synthetic-window-session-", "code_verifier", "continuation", "synthetic-gui-code", *PRIVATE_KEYS)),
                    "authentication output exposed private material")

            def cli_auth(operation, value, *, error=None):
                completed = subprocess.run(
                    [str(host.magpie), "plugin", "sdk", "auth", operation, MODULE, PROVIDER, value],
                    env=host.env, cwd=home, capture_output=True, text=True, timeout=45)
                require_public(completed.stdout + completed.stderr)
                if error is not None:
                    require(completed.returncode != 0 and error in completed.stderr and not completed.stdout.strip(),
                            f"auth {operation} did not give the expected refusal: {error}")
                    return None
                require(completed.returncode == 0, f"auth {operation} failed")
                return json.loads(completed.stdout)

            def subscription():
                rows = request("/api/providers").get("sdkSubscriptions", [])
                return next(item for item in rows if item["id"] == PROVIDER_ROW)

            def provider():
                return next(item for item in request("/api/providers")["providers"] if item["id"] == PROVIDER_ROW)

            require(not accounts(), "fixture must begin with zero accounts")
            row = subscription()
            require(row["name"] == "Devin" and row["reauthorize"] and not row.get("page"), "provider did not declare native sign-in")
            require({method["id"]: [field["type"] for field in method.get("fields", [])] for method in row["methods"]}
                    == {"browser": [], "api-key": ["secret"]}, "provider methods changed")
            require(not any(item["name"] == MODULE for item in request("/api/plugin-pages")), "a duplicate page was exposed")
            installed, = json.loads(host.command("plugin", "sdk", "list", "--json"))
            descriptor = installed["manifest"]
            capabilities = set(descriptor["contract"]["capabilities"])
            require({"provider-signin", "provider-saved-signin", "outbound-executor"} <= capabilities,
                    "provider sign-in roles changed")
            agents = descriptor.get("agents", [])
            if paired_only:
                require("agent-wiring" not in capabilities and not agents, "paired package acquired agent wiring")
            else:
                require("agent-wiring" in capabilities and len(agents) == 1 and
                        agents[0]["profile"]["target"] == PROVIDER, "complete package lost agent wiring")
            report["cases"].append("zero-accounts-ordinary-provider-tile-and-declared-roles")
            parent, sequence = "a" * 64, 0

            def auth(operation, flow="", selected_parent=parent, revision=None, options=None, success=True):
                nonlocal sequence
                sequence += 1
                value = {"id": PROVIDER_ROW, "revision": revision or row["revision"], "parent": selected_parent,
                         "request": format(sequence, "064x"), "operation": operation}
                if operation == "start":
                    value["options"] = options or {"method": "browser"}
                else:
                    value["flow"] = flow
                return request("/api/provider/sdk-signin", value, success)

            def callback(view, wrong=False):
                address = urllib.parse.urlsplit(view["url"])
                query = urllib.parse.parse_qs(address.query)
                redirect = query["redirect_uri"][0]
                require(redirect.startswith("http://127.0.0.1:") and redirect.endswith("/callback"),
                        "browser redirect changed donor loopback authority")
                parameters = urllib.parse.urlencode({
                    "state": "foreign" if wrong else query["state"][0], "code": "synthetic-gui-code"})
                try:
                    with opener.open(redirect + "?" + parameters, timeout=5) as response:
                        body = response.read()
                        require(not wrong and response.status == 200 and b"synthetic-gui-code" not in body,
                                "callback accepted foreign state or returned private data")
                except urllib.error.HTTPError as error:
                    require(wrong and error.code == 400, "valid module callback was refused")

            def complete_auth(view, refused=False):
                time.sleep(view["retryAfterMs"] / 1000 + 0.05)
                result = auth("finish", view["flow"])
                if refused:
                    require(result["state"] == "uncertain" and "account" not in result,
                            "refused authorization published its account")
                    return
                require(result["state"] == "complete", "authorization did not publish its account")
                return result["account"]

            def api_key_account(key):
                return complete_auth(auth("start", options={"method": "api-key", "input": {"key": key}}))

            def configure_server(server):
                snapshot = json.loads(host.command("plugin", "sdk", "config", MODULE))
                host.command("plugin", "sdk", "config", MODULE,
                             json.dumps({"server": server, "keepAlive": False}), "--expected", snapshot["version"])

            def selection(account):
                return {"id": account["id"], "expectedGeneration": str(account["generation"])}

            def saved_record(key, **metadata):
                return {"type": "api", "key": key, "metadata": {"server": server, **metadata}}

            saved_metadata = {"email": "unverified label", "orgId": "legacy-org",
                              "teamId": "legacy-team", "plan": "Legacy plan"}
            records = {
                "devin": saved_record(FIRST_SAVED_KEY, **saved_metadata),
                "devin#secondary": {**saved_record(SECOND_SAVED_KEY, email="unverified label"),
                                    "accountId": "legacy-unnamespaced-id"},
                "devin#rotated": saved_record(ROTATED_SAVED_KEY, **saved_metadata),
                "devin#unverified": saved_record(UNVERIFIED_SAVED_KEY, **saved_metadata),
                "devin#linked": saved_record(CLI_SAVED_KEY, cli="true", teamId="fixture-team-linked",
                                             orgId="fixture-organization"),
            }
            source = host.config / "plugin-auth.json"
            source_bytes = (json.dumps(records) + "\n").encode()
            source.write_bytes(source_bytes)
            source.chmod(0o600)
            before = vendor_keys()
            auth("start", options={"method": "saved-signin", "input": {"record": json.dumps(records["devin"])}},
                 success=False)
            require(vendor_keys() == before and not accounts(), "HTTP import dispatched or published credentials")
            require(source.read_bytes() == source_bytes, "rejected HTTP import changed the source")
            report["cases"].append("saved-signin-hidden-and-rejected-over-http")

            view = auth("start")
            require(view["state"] == "waiting" and vendor.exchanges == 0, "browser start exchanged credentials")
            early = auth("finish", view["flow"])
            require(early["state"] == "waiting" and vendor.exchanges == 0, "early browser poll dispatched")
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            pending = auth("finish", view["flow"])
            require(pending["state"] == "waiting" and not accounts() and vendor.exchanges == 0,
                    "poll without callback did not remain pending")
            callback(view, wrong=True)
            require(vendor.exchanges == 0, "foreign callback exchanged credentials")
            callback(view)
            require(vendor.exchanges == 0, "callback handler exchanged outside auth.finish")
            time.sleep(pending["retryAfterMs"] / 1000 + 0.05)
            complete = auth("finish", view["flow"])
            require(complete["state"] == "complete" and len(accounts()) == 1 and vendor.exchanges == 1,
                    "ordinary browser poll did not publish its first account once")
            first = complete["account"]
            require(provider()["accountUsers"][first["id"]] == FIRST_NAME, "Providers did not show the verified name")
            repeated = auth("finish", view["flow"])
            require(repeated["state"] == "complete" and vendor.exchanges == 1, "GUI completion exchanged again")
            report["cases"].append("loopback-pending-first-account-single-exchange")

            view = auth("start", options={"method": "browser", "account": selection(first)})
            callback(view)
            rotated = complete_auth(view)
            require(rotated["id"] == first["id"] and rotated["generation"] == "2" and len(accounts()) == 1,
                    "key rotation changed or duplicated the selected account")
            view = auth("start", options={"method": "api-key", "input": {"key": FIRST_SAVED_KEY},
                                          "account": selection(rotated)})
            complete_auth(view, refused=True)
            require(len(accounts()) == 1 and accounts()[0]["generation"] == 2,
                    "equally named credential for another subject replaced the selected account")
            report["cases"].append("browser-key-rotation-and-selected-account-mismatch")

            status_before, catalog_before = vendor_keys()
            cli_auth("import", "devin#unverified", error="requires interactive authorization")
            require(vendor_keys() == (status_before + [UNVERIFIED_SAVED_KEY], catalog_before) and
                    account_generations() == {first["id"]: 2} and source.read_bytes() == source_bytes,
                    "unverified status published an account or changed the legacy source")
            report["cases"].append("static-import-requires-verified-subject")

            status_before, catalog_before = vendor_keys()
            prepared = cli_auth("import", "devin")
            candidate = prepared["account"]
            require(set(prepared) == {"ticket", "account"} and prepared["ticket"] and
                    candidate["generation"] == "1" and candidate["name"] == FIRST_NAME,
                    "import did not return a prepared public account")
            require(account_generations() == {first["id"]: 2} and candidate["id"] not in provider()["accountUsers"],
                    "prepared sign-in appeared in an active account list")
            cli_auth("refresh", candidate["id"], error="Extension account is unavailable")
            require(vendor_keys() == (status_before + [FIRST_SAVED_KEY], catalog_before),
                    "preparation used a different source or allowed the candidate to make provider requests")
            require(source.read_bytes() == source_bytes, "preparation changed the legacy sign-ins")
            transfer_required = "requires a subscription owner transfer"
            cli_auth("activate", prepared["ticket"], error=transfer_required)
            require(account_generations() == {first["id"]: 2}, "activation published an account without a transfer")
            saved_first = api_key_account(FIRST_SAVED_KEY)
            require(saved_first["id"] == candidate["id"] and saved_first["generation"] == "1",
                    "interactive sign-in did not use the verified account identity")
            report["cases"].append("exact-source-preparation-and-transfer-required-for-activation")

            before_import = account_generations()
            prepared = cli_auth("import", "devin#secondary")
            records["devin#secondary"]["metadata"]["email"] = "changed-after-prepare"
            changed_bytes = (json.dumps(records) + "\n").encode()
            source.write_bytes(changed_bytes)
            cli_auth("activate", prepared["ticket"], error=transfer_required)
            require(account_generations() == before_import and source.read_bytes() == changed_bytes,
                    "refused activation published an account or rewrote the legacy source")
            source.write_bytes(source_bytes)
            prepared = cli_auth("import", "devin#secondary")
            saved_second = api_key_account(SECOND_SAVED_KEY)
            require(saved_second["id"] == prepared["account"]["id"], "prepared and interactive accounts differed")
            cli_auth("import", "devin", error="cannot replace an existing account")
            cli_auth("import", "devin#rotated", error="cannot replace an existing account")
            labels = provider()["accountUsers"]
            require(len({first["id"], saved_first["id"], saved_second["id"]}) == 3 and
                    account_generations() == {first["id"]: 2, saved_first["id"]: 1, saved_second["id"]: 1} and
                    all(labels[account["id"]] == FIRST_NAME for account in (first, saved_first, saved_second)),
                    "static sign-ins collided, accepted a donor label or replaced an active account")
            require(source.read_bytes() == source_bytes, "static sign-in activation changed the legacy source")
            report["cases"].append("same-label-distinct-subjects-and-repeat-import-refused")

            cli_file = Path(host.env["XDG_DATA_HOME"]) / "devin" / "credentials.toml"
            cli_file.parent.mkdir(parents=True, mode=0o700, exist_ok=True)

            def write_cli(key, endpoint=server):
                cli_file.write_text(f"windsurf_api_key = {json.dumps(key)}\napi_server_url = {json.dumps(endpoint)}\n")
                cli_file.chmod(0o600)

            before_import = account_generations()
            cli_auth("import", "devin#linked", error="could not verify the saved sign-in")
            write_cli(CLI_FOREIGN_KEY)
            cli_auth("import", "devin#linked", error="could not verify the saved sign-in")
            write_cli(CLI_CURRENT_KEY, "https://foreign.invalid")
            cli_auth("import", "devin#linked", error="could not verify the saved sign-in")
            require(account_generations() == before_import,
                    "missing, unrelated or foreign CLI sign-in published an account")
            write_cli(CLI_CURRENT_KEY)
            cli_bytes = cli_file.read_bytes()
            status_before, _ = vendor_keys()
            prepared = cli_auth("import", "devin#linked")
            verified = [key for key in vendor_keys()[0][len(status_before):] if key in CLI_KEYS]
            require(verified == [CLI_SAVED_KEY, CLI_CURRENT_KEY] and
                    account_generations() == before_import and source.read_bytes() == source_bytes and
                    cli_file.read_bytes() == cli_bytes,
                    "linked preparation did not preserve the source files or verify both keys")
            linked = prepared["account"]
            require(linked["id"] not in before_import and linked["generation"] == "1" and
                    linked["name"] == f"{CLI_RENAME} · Fixture organization",
                    "linked preparation lost the verified CLI identity")
            cli_auth("activate", prepared["ticket"], error=transfer_required)
            require(account_generations() == before_import, "activation published the CLI account without a transfer")
            report["cases"].append("linked-import-proves-saved-subject-origin-and-current-cli")

            vendor.renamed = True
            refreshed = cli_auth("refresh", first["id"])
            require(refreshed["name"] == UPDATED_NAME and provider()["accountUsers"][first["id"]] == UPDATED_NAME,
                    "credential refresh did not publish the updated account label")
            replacement = server_thread(stack, DenyProxy())
            configure_server(f"http://127.0.0.1:{replacement.server_port}")
            request("/api/login/forget", {"agent": PROVIDER_ROW, "account": selection(rotated)}, success=False,
                    refusal="extension account generation changed")
            require(account_generations() == {first["id"]: 3, saved_first["id"]: 1, saved_second["id"]: 1},
                    "stale sign-out changed the account store")
            request("/api/login/forget", {"agent": PROVIDER_ROW, "account": selection(saved_second)})
            require(account_generations() == {first["id"]: 3, saved_first["id"]: 1} and
                    not replacement.requests and not replacement.probes,
                    "local logout changed another account or contacted the replacement server")
            require(provider()["accountUsers"] == {first["id"]: UPDATED_NAME, saved_first["id"]: FIRST_NAME},
                    "Providers kept the removed account or lost a surviving account's label")
            configure_server(server)
            row = subscription()
            require(source.read_bytes() == source_bytes and cli_file.read_bytes() == cli_bytes,
                    "SDK logout changed a source sign-in")
            report["cases"].append("refresh-updated-label-stale-generation-local-logout-after-server-change")

            view = auth("start")
            callback(view)
            before = vendor.exchanges
            auth("finish", view["flow"], selected_parent="b" * 64, success=False)
            require(auth("cancel", view["flow"])["state"] == "canceled", "cancel failed")
            require(auth("finish", view["flow"])["state"] == "canceled" and vendor.exchanges == before,
                    "canceled/foreign flow exchanged")
            report["cases"].append("cancel-foreign-parent-zero-exchange")
            view = auth("start")
            old_revision = row["revision"]
            snapshot = json.loads(host.command("plugin", "sdk", "config", MODULE))
            host.command("plugin", "sdk", "config", MODULE, json.dumps({"server": server}),
                         "--expected", snapshot["version"])
            before = vendor.exchanges
            auth("finish", view["flow"], revision=old_revision, success=False)
            row = subscription()
            require(row["revision"] != old_revision and vendor.exchanges == before, "stale selection dispatched")
            report["cases"].append("stale-config-selection")
            host.command("accounts", "discovery", "off")
            auth("start", success=False)
            require(vendor.exchanges == before, "discovery-off start dispatched")
            host.command("accounts", "discovery", "on")
            report["cases"].append("discovery-off-start")
            view = auth("start")
            callback(view)
            vendor.hold = True
            time.sleep(view["retryAfterMs"] / 1000 + 0.05)
            results, failures = [], []

            def held_finish():
                try:
                    results.append(auth("finish", view["flow"], success=None))
                except Exception as error:
                    failures.append(str(error))

            thread = threading.Thread(target=held_finish, name="devin-gui-held-finish")
            thread.start()
            try:
                require(vendor.arrived.wait(10), "browser completion did not reach held exchange")
                host.command("accounts", "discovery", "off")
            finally:
                vendor.release.set()
                thread.join(15)
            require(not thread.is_alive() and not failures, "held browser completion did not join")
            require(results and (results[0] is None or results[0]["state"] != "complete") and
                    account_generations() == {first["id"]: 3, saved_first["id"]: 1},
                    "revoked browser completion published an account")
            host.command("accounts", "discovery", "on")
            report["cases"].append("discovery-off-held-exchange")
            view = auth("start")
            callback(view)
            before = vendor.exchanges
            host.command("plugin", "sdk", "off", MODULE)
            auth("finish", view["flow"], success=False)
            require(vendor.exchanges == before and
                    not any(item["id"] == PROVIDER_ROW for item in request("/api/providers").get("sdkSubscriptions", [])),
                    "disabled package retained a subscription or exchanged")
            report["cases"].append("disabled-before-finish")
            report["vendorFailures"] = len(vendor.failures)
            report["externalRequests"] = len(proxy.requests)
            require(not vendor.failures and not proxy.requests, "fixture endpoint/proxy assertions failed")
        require(not home.exists() and host.process.poll() is not None, "Providers fixture retained its HOME or process")
        report["status"] = "passed"
    except Exception as error:
        report["status"], report["failure"] = "failed", str(error)
        raise
    finally:
        report["homeRemoved"] = "home" in locals() and not home.exists()
        report["processStopped"] = "host" in locals() and host.process is not None and host.process.poll() is not None
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ("host", "distribution", "output"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--paired-only", action="store_true")
    options = parser.parse_args()
    run(options.host.resolve(), options.distribution.resolve(), options.output.resolve(),
        paired_only=options.paired_only)
