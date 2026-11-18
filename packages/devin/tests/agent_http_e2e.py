"""SDK agent profile -> native CLI wire fixture -> ordinary Front -> native module."""

import argparse
from contextlib import ExitStack
import http.client
import io
import json
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time

import paired_http_e2e as paired
from conformance import fields, text_field, number_field
from http_e2e import DenyProxy, Provider, server_thread, require, digest, SERVICE, CALLER_KEY


class OwnedVendorHandler(paired.VendorHandler):
    def do_POST(self):
        account_paths = {
            "/exa.seat_management_pb.SeatManagementService/GetUserStatus": "status",
            "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings": "team",
            SERVICE + "GetAccountManagedPlugins": "plugins",
            SERVICE + "AssignModel": "assignment",
        }
        if self.path not in (SERVICE + "GetChatMessage", SERVICE + "GetCliModelConfigs", *account_paths):
            self.reply("application/json", b"{}", 404)
            return
        size = int(self.headers.get("Content-Length", "0"))
        if not 0 < size <= 16 << 20:
            self.reply("application/json", b"{}", 404)
            self.close_connection = True
            return
        body = self.rfile.read(size)
        if paired.SELECTED_KEY.encode() not in body:
            self.reply("application/json", b"{}", 404)
            return
        if b"magpie-devin-native" in body or b"magpie-devin-native" in self.headers.get("Authorization", "").encode():
            with self.server.lock:
                self.server.failures.append("local agent credential escaped to selected vendor")
            self.reply("application/json", b'{"error":"credential boundary"}', 500)
            return
        if self.path in account_paths and self.headers.get("Content-Type") == "application/proto":
            if self.headers.get("Authorization") != "Basic " + paired.SELECTED_KEY + "-" + paired.SELECTED_KEY:
                self.reply("application/json", b'{"code":"unauthenticated"}', 401)
                return
            with self.server.lock:
                self.server.calls.append(self.path)
            kind = account_paths[self.path]
            if kind == "status":
                native = text_field(1, text_field(3, "Fixture user") + text_field(7, "fixture@example.invalid"))
            elif kind == "team":
                native = number_field(1, 1)
            elif kind == "plugins":
                native = text_field(1, '{"plugins":[]}')
            else:
                if fields(body).get(2) != [paired.MODEL.encode()]:
                    self.reply("application/json", b'{"code":"invalid_argument"}', 400)
                    return
                native = text_field(1, text_field(1, "owned-assignment-fixture") + text_field(2, paired.MODEL))
            self.reply("application/proto", native)
            return
        self.rfile = io.BytesIO(body)
        super().do_POST()


class AgentGateway(paired.PairedGateway):
    def binding(self):
        records = json.loads((self.config / "sdk-agent-bindings.json").read_text())["records"]
        return [{"revision": record["revision"], "phase": record["phase"], "values": record["values"]}
                for record in records.values()
                if record["plugin"] == "devin-inbound" and record["agent"] == "devin-native"]

    def require_single_bundle(self):
        entries = json.loads(self.command("plugin", "sdk", "list", "--json"))
        require(len(entries) == 1 and entries[0]["manifest"]["id"] == "devin-inbound" and
                not entries[0].get("disabled"), "fixture requires exactly one enabled own Devin bundle")
        legacy = self.config / "plugins.json"
        if legacy.exists():
            require(not json.loads(legacy.read_text()).get("plugins"),
                    "legacy/community provider plugin was installed")
        packages = self.config / "plugins/node_modules"
        require(not packages.exists() or not any(packages.iterdir()),
                "a legacy/helper package was installed")
        return [entry["manifest"]["id"] for entry in entries]

    def configure_agent(self, vendor):
        self.env["MAGPIE_NO_ACCOUNTS"] = "0"
        settings = json.loads((self.config / "settings.json").read_text())
        settings["noAccountDiscovery"] = False
        (self.config / "settings.json").write_text(json.dumps(settings))
        self.version = self.command("--version").strip()
        self.command("plugin", "sdk", "add", str(self.distribution))
        installed = json.loads(self.command("plugin", "sdk", "list", "--json"))
        contract = next(row["manifest"]["contract"] for row in installed
                        if row["manifest"]["id"] == "devin-inbound")
        require(contract["version"]["major"] == 1 and contract["version"]["minor"] >= 6 and
                "native-account-operations" in contract["capabilities"],
                "agent fixture requires an installed SDK1.6+ native account-operation contract")
        config = json.loads(self.command("plugin", "sdk", "config", "devin-inbound"))
        self.command("plugin", "sdk", "config", "devin-inbound",
                     json.dumps({"server": vendor}), "--expected", config["version"])
        self.sign_in()
        self.command("gateway-key", "add", "Fixture caller", "--key", CALLER_KEY)

    def start(self):
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            self.web_port = listener.getsockname()[1]
        self.env["MAGPIE_WEB_KEY"] = "owned-agent-web-" + secrets.token_hex(16)
        self.log = (self.artifacts / "gateway.log").open("w")
        self.process = subprocess.Popen([str(self.magpie), "web", "--no-open", "--gateway",
            "--addr", "127.0.0.1:" + str(self.web_port)], env=self.env, cwd=self.home,
            stdout=self.log, stderr=subprocess.STDOUT, start_new_session=True)
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            require(self.process.poll() is None, "ordinary web/gateway host exited")
            try:
                status, _, body = self.request("/", method="GET", key=None)
                listening = json.loads((self.config / "gateway.json").read_text())
                if status == 200 and listening.get("pid") == self.process.pid:
                    self.listener = listening
                    connection = http.client.HTTPConnection("127.0.0.1", self.web_port, timeout=5)
                    connection.request("GET", "/?k=" + self.env["MAGPIE_WEB_KEY"])
                    response = connection.getresponse()
                    response.read()
                    cookie = response.getheader("Set-Cookie", "")
                    connection.close()
                    require(response.status == 303 and cookie, "ordinary web authentication failed")
                    self.web_cookie = cookie.split(";", 1)[0]
                    return
            except (OSError, ValueError, http.client.HTTPException):
                pass
            time.sleep(0.05)
        raise AssertionError("ordinary web/gateway host did not start")

    def web_action(self, path, value):
        connection = http.client.HTTPConnection("127.0.0.1", self.web_port, timeout=30)
        connection.request("POST", path, json.dumps(value).encode(),
                           {"Content-Type": "application/json", "Cookie": self.web_cookie})
        response = connection.getresponse()
        body = response.read()
        status = response.status
        connection.close()
        require(status == 200, "ordinary GUI action failed: " + repr(body[:1500]))
        return json.loads(body)


def run(host, distribution, artifacts):
    artifacts.mkdir(parents=True, exist_ok=True)
    paired.SELECTED_KEY = "owned-agent-vendor-" + secrets.token_hex(16)
    paired.VendorHandler = OwnedVendorHandler
    checks = []
    with ExitStack() as stack:
        vendor = server_thread(stack, paired.Vendor())
        stack.callback(vendor.stop.set)
        proxy = server_thread(stack, DenyProxy())
        foreign = server_thread(stack, Provider())
        home = Path(tempfile.mkdtemp(prefix="devin-agent-home-"))
        stack.callback(shutil.rmtree, home)
        gateway = AgentGateway(host, distribution, home, artifacts,
                               "http://127.0.0.1:" + str(proxy.server_port))
        stack.callback(gateway.close)
        gateway.env["SHELL"] = "/bin/bash"
        executable = home / "bin/devin"
        executable.parent.mkdir()
        fixture = Path(__file__).with_name("agent_wire_fixture.py").resolve()
        executable.write_text("#!" + sys.executable + "\nimport runpy,sys\nsys.dont_write_bytecode=True\nsys.path.insert(0," +
                              repr(str(fixture.parent)) + ")\nrunpy.run_path(" + repr(str(fixture)) + ",run_name='__main__')\n")
        executable.chmod(0o700)
        gateway.env["PATH"] = str(executable.parent) + ":/usr/bin:/bin"
        original_shell = "# private synthetic shell\n"
        for name in (".bashrc", ".zshrc"):
            (home / name).write_text(original_shell)
        original_config = b'{"agent":{"model":"own-before-adoption"},"note":"synthetic"}\n'
        config = home / ".config/devin/config.json"
        config.parent.mkdir()
        config.write_bytes(original_config)
        gateway.configure_agent("http://127.0.0.1:" + str(vendor.server_port))
        gateway.require_single_bundle()
        gateway.start()
        model = None
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            status, _, body = gateway.request("/v1/models", method="GET")
            require(status == 200, "host models unavailable")
            model = next((item["id"] for item in json.loads(body)["data"]
                          if item["id"].endswith("/" + paired.MODEL)), None)
            if model:
                break
            time.sleep(0.1)
        require(model is not None, "SDK account model discovery never became ready")
        gateway.close()
        (artifacts / "gateway.log").rename(artifacts / "discovery-gateway.log")
        with vendor.lock:
            discovered_calls, discovered_probes = list(vendor.calls), list(vendor.probes)
        gateway.command("devin-native", "model", "magpie/" + model)
        environment = gateway.command("devin-native", "env")
        require("active=1" in environment and "WINDSURF_API_SERVER_URL=" in environment and
                "DEVIN_API_KEY=magpie-devin-native" in environment and "model=" + model in environment,
                "ordinary agent env did not use the declared profile")
        with vendor.lock:
            require(vendor.calls == discovered_calls and vendor.probes == discovered_probes,
                    "fresh CLI model selection refetched the vendor catalog")
        require(config.read_bytes() == original_config, "environment-only profile edited personal Devin config")
        checks.append("persisted dynamic model selection by fresh CLI with gateway stopped and no vendor calls")
        gateway.start()
        checks.append("ordinary SDK profile adoption, endpoint/key/model launch and untouched personal config")
        before = vendor.count()
        account_headers = {"Content-Type": "application/proto", "Connect-Protocol-Version": "1",
                           "Authorization": "Basic magpie-devin-native-magpie-devin-native",
                           "User-Agent": "devin-cli/0.0.0-synthetic"}
        bootstrap = {"reads": []}
        for path in ("/exa.seat_management_pb.SeatManagementService/GetUserStatus",
                     "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings",
                     SERVICE + "GetAccountManagedPlugins"):
            binding_before = gateway.binding()
            status, _, body = gateway.request(path, text_field(1, text_field(3, "magpie-devin-native")),
                                              key=None, headers=account_headers)
            bootstrap["reads"].append({"path": path, "status": status,
                                       "bindingBefore": binding_before, "bindingAfter": gateway.binding(),
                                       "failure": body.decode(errors="replace") if status != 200 else ""})
            (artifacts / "bootstrap-observations.json").write_text(json.dumps(bootstrap, indent=2))
            require(status == 200 and fields(body) and vendor.count() == before,
                    "recovered native bootstrap did not use selected account dispatch")
        assignment_request = text_field(1, text_field(3, "magpie-devin-native"))
        assignment_request += text_field(2, paired.MODEL) + text_field(3, "owned-assignment-session")
        with vendor.lock:
            assignments_before = vendor.calls.count(SERVICE + "AssignModel")
        bootstrap["assignment"] = {"bindingBefore": gateway.binding()}
        status, _, body = gateway.request(SERVICE + "AssignModel", assignment_request,
                                          key=None, headers=account_headers)
        bootstrap["assignment"].update(status=status, bindingAfter=gateway.binding())
        (artifacts / "bootstrap-observations.json").write_text(json.dumps(bootstrap, indent=2))
        require(status == 200, "native assignment did not use its declared write action: " + repr(body))
        assignment = fields(fields(body)[1][0])
        require(assignment[2] == [paired.MODEL.encode()], "native assignment changed its selected model")
        gateway.env["MAGPIE_FIXTURE_ASSIGNMENT"] = assignment[1][0].decode()
        bootstrap["duplicate"] = {"bindingBefore": gateway.binding()}
        status, _, body = gateway.request(SERVICE + "AssignModel", assignment_request,
                                          key=None, headers=account_headers)
        bootstrap["duplicate"].update(status=status, bindingAfter=gateway.binding())
        with vendor.lock:
            assignment_dispatches = vendor.calls.count(SERVICE + "AssignModel") - assignments_before
        bootstrap.update(duplicateStatus=status, assignmentDispatches=assignment_dispatches)
        (artifacts / "bootstrap-observations.json").write_text(json.dumps(bootstrap, indent=2))
        require(status >= 400 and assignment_dispatches == 1,
                "native assignment duplicate bypassed its durable write journal: " + repr(bootstrap))
        checks.append("selected-account native status/team/plugin reads and journaled assignment with no-replay")
        first = json.loads(gateway.command("devin-native", "run", "-p", "--", "native-signed"))
        require(first["signed"], "native session did not receive signed thinking")
        second = json.loads(gateway.command("devin-native", "run", "--resume", first["session"], "-p", "--", "session-resume"))
        require(second["resumed"] and second["session"] == first["session"] and second["historyMessages"] == 4,
                "resumed native session lost its history")
        third = json.loads(gateway.command("devin-native", "run", "-p", "--", "session-independent"))
        require(third["session"] != first["session"] and third["historyMessages"] == 2,
                "separate native session inherited another session's history")
        with vendor.lock:
            bodies = list(vendor.chat)
        require(len(bodies) == 3, "native launched sessions did not reach exactly three vendor requests")
        sessions = [fields(body).get(28) for body in bodies]
        require(sessions == [[first["session"].encode()], [first["session"].encode()], [third["session"].encode()]],
                "native session IDs changed between agent and selected provider")
        require(len(fields(bodies[1])[3]) == 3 and len(fields(bodies[2])[3]) == 1,
                "resumed and independent native message histories changed")
        checks.append("actual ordinary launch, native catalog, native Connect/session IDs, signed-history resume and session isolation")
        cancelled = json.loads(gateway.command("devin-native", "run", "-p", "--", "native-deadline"))
        require(cancelled["terminal"] == "deadline_exceeded" and vendor.cancelled["deadline"].wait(5),
                "agent deadline did not cancel actual selected-provider request")
        checks.append("launched native deadline reaches actual upstream cancellation")
        installed_modules = gateway.require_single_bundle()
        mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
        libraries = {line.split()[-1] for line in mappings.splitlines()
                     if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
        require(any(Path(path).is_file() and digest(path) == digest(distribution / "plugin.so")
                    for path in libraries), "native module hash was not mapped into ordinary gateway")
        launch_file = home / ".fixture-devin/launches.jsonl"
        launch_count = len(launch_file.read_text().splitlines())
        gateway.command("plugin", "sdk", "off", "devin-inbound")
        stopped = subprocess.run([str(host), "devin-native", "run", "-p", "--", "disabled"],
                                 env=gateway.env, cwd=home, capture_output=True, text=True, timeout=15)
        require(stopped.returncode != 0 and len(launch_file.read_text().splitlines()) == launch_count,
                "disabled owner launched a client on an own account")
        checks.append("disabled native module refuses ordinary agent launch before process execution")
        gateway.web_action("/api/agents/disconnect/devin-native", {})
        require(config.read_bytes() == original_config, "disconnect changed original Devin settings")
        for name in (".bashrc", ".zshrc"):
            require((home / name).read_text() == original_shell, "disconnect did not restore original shell")
        checks.append("disabled-owner offline disconnect restores shell and original config")
        require(not vendor.failures and not proxy.requests and not foreign.requests,
                "fixture vendor, proxy or foreign executor was used incorrectly")
        result = {"checks": checks, "nativeRequests": vendor.count(), "externalRequests": len(proxy.requests),
                  "hostSHA256": digest(host), "librarySHA256": digest(distribution / "plugin.so"),
                  "sessionIDs": [first["session"], third["session"]], "runtime": "native",
                  "client": "synthetic recovered-wire fixture, not original Devin CLI",
                  "installedModules": installed_modules, "legacyProviderPlugins": [], "helperPackages": [],
                  "generationMode": "live", "vendorProbes": vendor.probes, "status": "passed"}
    result["homeRemoved"] = not home.exists()
    result["processStopped"] = gateway.process.poll() is not None
    require(result["homeRemoved"] and result["processStopped"], "agent fixture left its host/home running")
    (artifacts / "result.json").write_text(json.dumps(result, indent=2))
    print(json.dumps(result, indent=2))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--magpie", required=True, type=Path)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--artifacts", required=True, type=Path)
    options = parser.parse_args()
    run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve())
