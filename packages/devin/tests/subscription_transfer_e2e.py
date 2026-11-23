"""Exercise Devin subscription transfer, rollback and selected Providers sign-out offline."""

import argparse
from contextlib import ExitStack
import gzip
import http.cookiejar
from http.server import ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

from conformance import connect_frames, data_frame, fields, number_field, text_field
from http_e2e import CALLER_KEY, DenyProxy, Gateway, SERVICE, digest, require, server_thread
from provider_single_package_e2e import Handler, MODELS, STATUS


MODULE = "devin-inbound"
PROVIDER = "devin"
MODEL = "swe-2"
PUBLIC_MODEL = f"{PROVIDER}/{MODEL}"
ORGANIZATION = "Fixture organization"
WEB_KEY = "synthetic-devin-transfer-web-key"
SAVED_KEYS = {
    "devin": "cog_fixture_devin_transfer_first",
    "devin#secondary": "cog_fixture_devin_transfer_second",
    "devin#cli": "cog_fixture_devin_transfer_cli_saved",
}
USERS = {
    "devin": "saved-first",
    "devin#secondary": "saved-second",
    "devin#cli": "saved-cli",
}
SELECTED = "devin#cli"
ROTATED = "devin#secondary"
ACCOUNT_NAMES = {SELECTED: "Fixture CLI account", ROTATED: "Fixture second organization"}
TEAM_ID = "fixture-team"
SUBJECT_USERS = {**USERS, ROTATED: USERS[SELECTED]}
ORG_IDS = dict.fromkeys(SAVED_KEYS, "fixture-org")
ORG_IDS[ROTATED] = "fixture-second-org"
CLI_CURRENT = "cog_fixture_devin_transfer_cli_current"
ROTATED_KEY = "cog_fixture_devin_transfer_second_rotated"
GATEWAY_KEY_NAME = "Fixture transfer caller"
DENIED_KEY_NAME = "Fixture disjoint account caller"
DENIED_KEY = "synthetic-denied-devin-transfer-key"
CALLER_KEYS = {GATEWAY_KEY_NAME: CALLER_KEY, DENIED_KEY_NAME: DENIED_KEY}
PRIVATE = (*SAVED_KEYS.values(), CLI_CURRENT, ROTATED_KEY, WEB_KEY, *CALLER_KEYS.values())


def public_output(value):
    require(not any(secret in value for secret in PRIVATE), "public output contained a credential")


def account_label(key):
    return f"{SUBJECT_USERS[key]}@example.invalid · {ORGANIZATION}"


class Vendor(Handler):
    def do_POST(self):
        with self.server.lock:
            self.server.requests += 1
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 < size <= 64 << 10, "invalid fixture request size")
            body = self.rfile.read(size)
            if self.path == STATUS:
                key = json.loads(body)["metadata"]["apiKey"]
            elif self.path == MODELS:
                key = fields(fields(body)[1][0])[3][0].decode()
            else:
                key = self.headers.get("Authorization", "").removeprefix("Basic ")
            saved_key = SELECTED if key == CLI_CURRENT else next(
                (name for name, token in SAVED_KEYS.items() if token == key), None,
            )
            if key == ROTATED_KEY:
                saved_key = ROTATED
            require(saved_key, "vendor received an unexpected credential")
            if self.path == STATUS:
                user = SUBJECT_USERS[saved_key]
                status = {"userId": user, "email": f"{user}@example.invalid", "teamId": TEAM_ID,
                          "planStatus": {"dailyQuotaRemainingPercent": 90, "planInfo": {"devinInfo": {
                              "orgId": ORG_IDS[saved_key], "accountDisplayName": ORGANIZATION,
                          }}}}
                with self.server.lock:
                    self.server.status_accounts.append(saved_key)
                self.reply("application/json", json.dumps({"userStatus": status}).encode())
                return
            if self.path == MODELS:
                info = number_field(4, 65536) + number_field(13, 4096) + text_field(6, number_field(12, 1))
                model = text_field(1, MODEL) + text_field(22, MODEL) + number_field(18, 65536) + text_field(23, info)
                self.reply("application/proto", text_field(1, model))
                return
            require(self.path == SERVICE + "GetChatMessage", "unexpected vendor endpoint")
            (flags, payload), = connect_frames(body)
            require(flags in (0, 1), "invalid Connect message flags")
            if flags == 1:
                payload = gzip.decompress(payload)
            request = fields(payload)
            require(request.get(21) == [MODEL.encode()] and fields(request[1][0])[3] == [key.encode()],
                    "model or credential did not reach the ordinary Devin endpoint")
            require(saved_key == SELECTED and not any(token.encode() in payload for token in CALLER_KEYS.values()),
                    "wrong selected account or leaked caller key")
            messages = [fields(value) for value in request.get(3, [])]
            prompt = b"".join(message.get(3, [b""])[0] for message in messages).decode()
            require(prompt in ("before-transfer", "after-transfer", "after-rollback", "after-logout"),
                    "unexpected vendor prompt")
            expected_key = CLI_CURRENT if prompt in ("after-transfer", "after-logout") else SAVED_KEYS[SELECTED]
            require(key == expected_key, "the serving owner did not use its verified credential")
            with self.server.lock:
                self.server.calls.append({"prompt": prompt, "account": USERS[saved_key], "model": MODEL})
            usage = text_field(7, number_field(2, 7) + number_field(3, 3))
            reply = text_field(3, "fixture answer: " + prompt) + number_field(5, 2) + usage
            self.reply("application/connect+proto", data_frame(reply) + b"\2\0\0\0\2{}")
        except Exception as error:
            with self.server.lock:
                self.server.failures.append(f"{self.path}: {type(error).__name__}")
            self.reply("application/json", b'{"code":"internal"}', 500)


class TransferGateway(Gateway):
    def command(self, *arguments, refusal=None):
        completed = subprocess.run([str(self.magpie), *arguments], cwd=self.home, env=self.env,
                                   capture_output=True, text=True, timeout=45)
        output = completed.stdout + completed.stderr
        if arguments not in [("gateway-key", "add", name, "--key", key) for name, key in CALLER_KEYS.items()]:
            public_output(output)
        operation = " ".join(arguments[:3])
        if refusal:
            require(completed.returncode != 0 and refusal in output,
                    f"{operation} did not refuse as expected: {output[-1200:]}")
        else:
            require(completed.returncode == 0, f"{operation} failed: {completed.stderr[-1200:]}")
        return completed.stdout

    def start_web(self):
        self.log = (self.artifacts / "host.log").open("w")
        address = f"127.0.0.1:{self.port}"
        self.process = subprocess.Popen([str(self.magpie), "web", "--addr", address, "--no-open"],
                                        cwd=self.home, env=self.env, stdout=self.log,
                                        stderr=subprocess.STDOUT, start_new_session=True)
        self.origin = "http://" + address
        self.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            require(self.process.poll() is None, "sandbox web host exited during startup")
            try:
                with self.opener.open(self.origin + "/?k=" + WEB_KEY, timeout=2) as response:
                    if response.status == 200:
                        return
            except OSError:
                time.sleep(0.05)
        raise AssertionError("sandbox web host did not become ready")

    def subscription(self, revision=""):
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            with self.opener.open(self.origin + "/api/providers", timeout=15) as response:
                public = response.read(16 << 20).decode()
            public_output(public)
            rows = [row for row in json.loads(public)["providers"] if row["id"] == PROVIDER]
            if len(rows) == 1 and rows[0].get("account", {}).get("ownerRevision", "") == revision:
                return rows[0]
            time.sleep(0.15)
        raise AssertionError("ordinary Providers did not publish the expected subscription owner")

    def account_acl_bindings(self, expected, stage):
        with self.opener.open(self.origin + "/api/caller-keys", timeout=15) as response:
            public = response.read(16 << 20).decode()
        public_output(public)
        names = json.loads(public)["accountNames"]
        bindings = {}
        for label, account in expected.items():
            matches = [ref for ref, shown in names.items() if shown == label]
            require(matches == [account], f"{stage}: {label} did not identify the expected canonical grant")
            bindings[label] = matches[0]
        return bindings

    def configure_server(self, server):
        config = json.loads(self.command("plugin", "sdk", "config", MODULE))
        self.command("plugin", "sdk", "config", MODULE, json.dumps({"server": server}),
                     "--expected", config["version"])

    def forget_login(self, revision, login, refusal=None):
        body = json.dumps({"agent": PROVIDER, "ownerRevision": revision,
                           "account": {"id": login["user"], "expectedGeneration": login["extensionGeneration"]}})
        request = urllib.request.Request(self.origin + "/api/login/forget", data=body.encode(),
                                         headers={"Content-Type": "application/json", "Origin": self.origin})
        try:
            with self.opener.open(request, timeout=35) as response:
                status, public = response.status, response.read(16 << 20).decode()
        except urllib.error.HTTPError as denial:
            with denial:
                status, public = denial.code, denial.read(16 << 20).decode()
        public_output(public)
        result = json.loads(public)
        if refusal:
            require(status == 400 and refusal in result.get("error", ""),
                    f"Providers sign-out did not give the expected refusal: {refusal}")
        else:
            require(status == 200, f"Providers sign-out returned HTTP {status}: {public[:1200]}")
        return result

    def infer(self, stage):
        status, _, body = self.request("/v1/models", method="GET")
        require(status == 200 and sum(row["id"] == PUBLIC_MODEL for row in json.loads(body)["data"]) == 1,
                "the public Devin model was absent or duplicated")
        value = {"model": PUBLIC_MODEL, "messages": [{"role": "user", "content": stage}], "max_tokens": 32}
        status, _, body = self.request("/v1/chat/completions", json.dumps(value).encode(),
                                       headers={"Content-Type": "application/json"})
        output = body.decode()
        public_output(output)
        require(status == 200, f"{stage}: ordinary Chat request returned {status}: {output[-1200:]}")
        require(json.loads(output)["choices"][0]["message"]["content"] == "fixture answer: " + stage,
                stage + ": ordinary Chat response changed")

    def refuse_disjoint_account(self, vendor, stage):
        with vendor.lock:
            previous = vendor.requests
        value = {"model": PUBLIC_MODEL, "messages": [{"role": "user", "content": "acl-denied-" + stage}],
                 "max_tokens": 32}
        status, _, body = self.request("/v1/chat/completions", json.dumps(value).encode(), key=DENIED_KEY,
                                       headers={"Content-Type": "application/json"})
        output = body.decode()
        public_output(output)
        with vendor.lock:
            require(vendor.requests == previous, f"{stage}: denied caller sent an upstream request")
        require(status == 403 and DENIED_KEY_NAME in output and
                f"not allowed to use the accounts behind {PUBLIC_MODEL}" in output,
                f"{stage}: authenticated caller did not get the account ACL refusal: HTTP {status}")
        return {"stage": stage, "status": status, "upstreamRequests": 0}


def run(magpie, distribution, legacy, bun, artifacts):
    artifacts.mkdir()
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    report = {"status": "failed", "hostSHA256": digest(magpie), "sdkSHA256": digest(distribution / manifest["entry"]),
              "legacySHA256": digest(legacy), "runnerSHA256": digest(__file__),
              "moduleVersion": manifest["version"], "checks": [], "accountACLRefusals": []}
    try:
        require((manifest["id"], manifest["version"]) == (MODULE, "0.8.3"), "wrong Devin release")
        with ExitStack() as stack:
            vendor = ThreadingHTTPServer(("127.0.0.1", 0), Vendor)
            vendor.lock, vendor.calls, vendor.failures = threading.Lock(), [], []
            vendor.requests, vendor.status_accounts = 0, []
            server_thread(stack, vendor)
            proxy = server_thread(stack, DenyProxy())
            replacement = server_thread(stack, DenyProxy())
            replacement_origin = f"http://127.0.0.1:{replacement.server_port}"
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-transfer-")))
            gateway = TransferGateway(magpie, distribution, home, artifacts, f"http://127.0.0.1:{proxy.server_port}")
            stack.callback(gateway.close)
            gateway.env.update(MAGPIE_NO_ACCOUNTS="0", MAGPIE_WEB_KEY=WEB_KEY, MAGPIE_BUN=str(bun),
                               MAGPIE_ADDR="127.0.0.1:0")
            settings_path = gateway.config / "settings.json"
            settings = json.loads(settings_path.read_text())
            settings["noAccountDiscovery"] = False
            settings_path.write_text(json.dumps(settings))
            for name in ("models.json", "labs.json"):
                (gateway.cache / name).write_text("{}")
            report["hostVersion"] = gateway.command("--version").strip()
            origin = f"http://127.0.0.1:{vendor.server_port}"
            cli = Path(gateway.env["XDG_DATA_HOME"]) / "devin/credentials.toml"
            cli.parent.mkdir(parents=True)

            def write_cli(token):
                cli.write_text(f'api_server_url = {json.dumps(origin)}\nwindsurf_api_key = {json.dumps(token)}\n')
                cli.chmod(0o600)

            write_cli(SAVED_KEYS[SELECTED])
            source = gateway.config / "plugin-auth.json"
            records = {saved_key: {"type": "api", "key": token, "accountId": USERS[saved_key],
                                   "metadata": {"server": origin, "email": account_label(saved_key),
                                                "orgId": ORG_IDS[saved_key], "teamId": TEAM_ID}}
                       for saved_key, token in SAVED_KEYS.items()}
            records[SELECTED]["metadata"]["cli"] = "true"
            source.write_text(json.dumps(records) + "\n")
            source.chmod(0o600)
            gateway.command("plugin", "add", str(legacy))
            gateway.command("provider", "refresh", PROVIDER)
            gateway.command("provider", "models", PROVIDER, "+" + MODEL)
            gateway.command("provider", "pool", PROVIDER, "--accounts", USERS[SELECTED])
            for login, label in ACCOUNT_NAMES.items():
                gateway.command("accounts", "alias", PROVIDER, USERS[login], label)
            gateway.command("gateway-key", "add", GATEWAY_KEY_NAME, "--key", CALLER_KEY)
            gateway.command("gateway-key", "add", DENIED_KEY_NAME, "--key", DENIED_KEY)
            acl = gateway.config / "caller-keys.json"
            caller_ids = {item["name"]: item["id"] for item in json.loads(acl.read_text())}
            for name, login in ((GATEWAY_KEY_NAME, SELECTED), (DENIED_KEY_NAME, ROTATED)):
                gateway.command("gateway-key", "models", caller_ids[name], PUBLIC_MODEL)
                gateway.command("gateway-key", "accounts", caller_ids[name], f"{PROVIDER}/{USERS[login]}")
            original_acl = acl.read_bytes()
            caller_keys = {item["name"]: item for item in json.loads(original_acl)}
            caller_key = caller_keys[GATEWAY_KEY_NAME]
            denied_key = caller_keys[DENIED_KEY_NAME]
            require(caller_key["models"] == denied_key["models"] == [PUBLIC_MODEL] and
                    len(caller_key["accounts"]) == len(denied_key["accounts"]) == 1 and
                    caller_key["accounts"] != denied_key["accounts"],
                    "caller keys do not share the model with disjoint account ACLs")
            allowed, = caller_key["accounts"]
            disjoint, = denied_key["accounts"]
            expected_bindings = {f"{PROVIDER}/{ACCOUNT_NAMES[SELECTED]}": allowed,
                                 f"{PROVIDER}/{ACCOUNT_NAMES[ROTATED]}": disjoint}
            gateway.start_web()
            pid = gateway.process.pid
            before = gateway.subscription()
            expected_logins = [(USERS[SELECTED], True), (USERS["devin"], False), (USERS["devin#secondary"], False)]

            def check_logins(row):
                return [(login["user"], login["on"]) for login in row["account"]["logins"]]

            require(check_logins(before) == expected_logins, "legacy account policy was not applied")
            report["accountACLBindings"] = {
                "before-transfer": gateway.account_acl_bindings(expected_bindings, "before-transfer")
            }
            gateway.infer("before-transfer")
            report["accountACLRefusals"].append(gateway.refuse_disjoint_account(vendor, "before-transfer"))
            original = source.read_bytes()
            report["checks"].append("three community accounts, stable model ACL and successful CLI-linked request")

            gateway.command("plugin", "sdk", "add", str(distribution))
            gateway.configure_server(replacement_origin)
            gateway.command("plugin", "sdk", "auth", "import", MODULE, PROVIDER, ROTATED,
                            refusal="could not verify the saved sign-in")
            require(not replacement.requests and not replacement.probes,
                    "a foreign-server import sent a request to the configured replacement server")
            report["foreignServerImportRefused"] = True
            gateway.configure_server(origin)
            write_cli(SAVED_KEYS["devin"])
            gateway.command("plugin", "sdk", "auth", "import", MODULE, PROVIDER, SELECTED,
                            refusal="could not verify the saved sign-in")
            write_cli(SAVED_KEYS[ROTATED])
            with vendor.lock:
                status_start = len(vendor.status_accounts)
            gateway.command("plugin", "sdk", "auth", "import", MODULE, PROVIDER, SELECTED,
                            refusal="could not verify the saved sign-in")
            with vendor.lock:
                status_accounts = vendor.status_accounts[status_start:]
            require(SELECTED in status_accounts and ROTATED in status_accounts,
                    "same-user foreign organization refusal did not verify both keys at the vendor")
            report["wrongOrganizationStatusAccounts"] = [SELECTED, ROTATED]
            write_cli(CLI_CURRENT)
            require(not json.loads(gateway.command("plugin", "sdk", "auth", "owners")) and
                    not json.loads(gateway.command("plugin", "sdk", "accounts", MODULE, PROVIDER, "--json")) and
                    source.read_bytes() == original and acl.read_bytes() == original_acl and
                    check_logins(gateway.subscription()) == expected_logins,
                    "rejected identity checks changed an owner, credential, ACL or public alias")
            report["wrongOrganizationFollowCLIRefused"] = True

            def prepare_signins(rotated=""):
                prepared = {}
                for key in SAVED_KEYS:
                    item = json.loads(gateway.command("plugin", "sdk", "auth", "import", MODULE, PROVIDER, key))
                    require(item["account"]["name"] == account_label(key) and
                            item["account"]["generation"] == ("2" if key == rotated else "1"),
                            "import lost the verified account label or generation")
                    prepared[key] = item
                cli_account, other_org = [prepared[key]["account"] for key in (SELECTED, ROTATED)]
                require(cli_account["name"] == other_org["name"] and cli_account["id"] != other_org["id"],
                        "same-user Devin accounts with the same label and different organizations collided")
                require(len({item["account"]["id"] for item in prepared.values()}) == len(SAVED_KEYS),
                        "verified Devin accounts collided")
                return prepared

            prepared = prepare_signins()
            report["sharedUserTenantAccountsDistinct"] = True
            tickets = [item["ticket"] for item in prepared.values()]
            gateway.command("plugin", "sdk", "auth", "transfer", MODULE, PROVIDER, tickets[0],
                            refusal="selection changed or is incomplete")
            require(json.loads(gateway.command("plugin", "sdk", "auth", "owners")) == [],
                    "partial transfer published a subscription owner")
            require(source.read_bytes() == original, "preparation or partial transfer rewrote the legacy sign-ins")
            owner = json.loads(gateway.command("plugin", "sdk", "auth", "transfer", MODULE, PROVIDER, *tickets))
            require(owner["active"] and owner["publicID"] == PROVIDER and owner["revision"],
                    "SDK changed the public Devin provider ID")
            expected_accounts = {key: (USERS[key], item["account"]["id"]) for key, item in prepared.items()}
            require({item["key"]: (item["user"], item["id"]) for item in owner["accounts"]} == expected_accounts,
                    "transfer changed a public account alias or crossed credentials")
            selected, = [item for item in owner["accounts"] if item["key"] == SELECTED]
            other_org, = [item for item in owner["accounts"] if item["key"] == ROTATED]
            require(allowed == f"{PROVIDER}/{selected['loginID']}" and
                    disjoint == f"{PROVIDER}/{other_org['loginID']}" and acl.read_bytes() == original_acl,
                    "transfer changed the stored gateway ACL")
            active = gateway.subscription(owner["revision"])
            require(check_logins(active) == expected_logins and
                    active["accountUsers"] == {USERS[key]: account_label(key) for key in SAVED_KEYS},
                    "SDK Providers lost account policy or verified labels")
            require(all(login["extensionGeneration"] == "1" for login in active["account"]["logins"]),
                    "SDK Providers lost the account generation")
            report["accountACLBindings"]["after-transfer"] = gateway.account_acl_bindings(
                expected_bindings, "after-transfer")
            write_cli(SAVED_KEYS[SELECTED])
            gateway.infer("after-transfer")
            report["accountACLRefusals"].append(gateway.refuse_disjoint_account(vendor, "after-transfer"))
            report["checks"].append("foreign CLI identity and partial transfer refused, SDK preserved all three identities and routing")

            gateway.command("plugin", "logout", PROVIDER, refusal="managed by an SDK plugin")
            require(source.read_bytes() == original, "legacy sign-out changed rollback credentials")
            cli_before_rollback = cli.read_bytes()
            rollback = json.loads(gateway.command("plugin", "sdk", "auth", "rollback", PROVIDER, owner["revision"]))
            require(not rollback["active"] and rollback["revision"] != owner["revision"] and
                    rollback["publicID"] == owner["publicID"] and rollback["accounts"] == owner["accounts"],
                    "rollback changed the public subscription or account identities")
            require(check_logins(gateway.subscription()) == expected_logins, "rollback lost the legacy account policy")
            gateway.infer("after-rollback")
            report["accountACLRefusals"].append(gateway.refuse_disjoint_account(vendor, "after-rollback"))
            require(source.read_bytes() == original and cli.read_bytes() == cli_before_rollback and
                    gateway.process.pid == pid and gateway.process.poll() is None,
                    "rollback rewrote a legacy sign-in or restarted Magpie")
            require(acl.read_bytes() == original_acl, "rollback changed the stored gateway ACL")
            report["checks"].append("legacy logout refused, original credentials intact, same host served restored legacy route")

            write_cli(CLI_CURRENT)
            records = json.loads(original)
            records[ROTATED]["key"] = ROTATED_KEY
            rotated_source = (json.dumps(records) + "\n").encode()
            source.write_bytes(rotated_source)
            tickets = [item["ticket"] for item in prepare_signins(rotated=ROTATED).values()]
            current_owner = json.loads(gateway.command("plugin", "sdk", "auth", "transfer", MODULE, PROVIDER, *tickets))
            require(current_owner["active"] and current_owner["accounts"] == owner["accounts"] and
                    current_owner["revision"] != owner["revision"], "repeat transfer changed the public identities")
            fresh = gateway.subscription(current_owner["revision"])
            signed_out_user = USERS[ROTATED]
            old_login = next(login for login in active["account"]["logins"] if login["user"] == signed_out_user)
            login = next(login for login in fresh["account"]["logins"] if login["user"] == signed_out_user)
            require(login["extensionGeneration"] == "2", "new owner did not publish the new credential generation")
            cli_before_logout = cli.read_bytes()
            gateway.configure_server(replacement_origin)
            revision = fresh["account"]["ownerRevision"]
            gateway.forget_login(owner["revision"], login, refusal="subscription owner changed")
            gateway.forget_login(revision, old_login, refusal="extension account generation changed")
            require(check_logins(gateway.subscription(revision)) == expected_logins,
                    "a stale sign-out removed a transferred account")
            result = gateway.forget_login(revision, login)
            signed_out = next(row for row in result["providers"] if row["id"] == PROVIDER)
            surviving = [(user, on) for user, on in expected_logins if user != signed_out_user]
            require(check_logins(signed_out) == surviving and
                    signed_out["accountUsers"] == {user: label for user, label in fresh["accountUsers"].items()
                                                  if user != signed_out_user},
                    "ordinary sign-out changed another account or kept the signed-out account in Providers")
            remaining = json.loads(gateway.command("plugin", "sdk", "accounts", MODULE, PROVIDER, "--json"))
            require({item["id"]: item["generation"] for item in remaining} ==
                    {item["id"]: 1 for item in current_owner["accounts"] if item["user"] != signed_out_user},
                    "ordinary sign-out did not delete exactly the selected SDK credentials")
            require(source.read_bytes() == rotated_source and cli.read_bytes() == cli_before_logout and
                    acl.read_bytes() == original_acl and not replacement.requests,
                    "SDK sign-out changed a source sign-in or ACL, or sent a request to the replacement server")
            gateway.configure_server(origin)
            gateway.infer("after-logout")
            require([call["prompt"] for call in vendor.calls] ==
                    ["before-transfer", "after-transfer", "after-rollback", "after-logout"],
                    "unexpected inference attempts crossed the vendor boundary")
            report["replacementServerRequests"] = len(replacement.requests) + len(replacement.probes)
            require(not vendor.failures and not proxy.requests and report["replacementServerRequests"] == 0,
                    "offline fixture boundary failed")
            report["checks"].append("Providers refused stale owner and generation, signed out only the chosen account "
                                    "after a server change, and kept the surviving route")
            report["publicProvider"] = PROVIDER
            report["approvedAccount"] = allowed
            report["disjointAccount"] = disjoint
            report["vendorCalls"] = vendor.calls
        require(gateway.process.poll() is not None and not home.exists(), "fixture retained its host or home")
        report["status"] = "passed"
    except Exception as error:
        report["failure"] = str(error)
        raise
    finally:
        report["hostStopped"] = "gateway" in locals() and gateway.process is not None and gateway.process.poll() is not None
        report["homeRemoved"] = "home" in locals() and not home.exists()
        (artifacts / "result.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("magpie", "distribution", "legacy", "bun", "artifacts"):
        parser.add_argument("--" + name, required=True, type=Path)
    args = parser.parse_args()
    run(args.magpie.resolve(), args.distribution.resolve(), args.legacy.resolve(), args.bun.resolve(),
        args.artifacts.resolve())
