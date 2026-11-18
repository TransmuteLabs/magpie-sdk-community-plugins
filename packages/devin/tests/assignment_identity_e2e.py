"""Diagnose native identity or accept a conservative host no-op sync fix."""

import argparse
import base64
from contextlib import ExitStack
from concurrent.futures import ThreadPoolExecutor
import hashlib
import io
import json
from pathlib import Path
import secrets
import shutil
import tempfile
import threading
import time

import agent_http_e2e as agent
import paired_http_e2e as paired
from conformance import fields, text_field
from http_e2e import DenyProxy, Gateway, SERVICE, digest, require, server_thread


def snapshot(gateway):
    records = json.loads((gateway.config / "sdk-agent-bindings.json").read_text())["records"]
    selected = [record for record in records.values()
                if record["plugin"] == "devin-inbound" and record["agent"] == "devin-native"]
    require(len(selected) == 1, "diagnosis requires one owned native binding")
    record = dict(selected[0])
    record.pop("home")
    private = gateway.config / "extensions/private/devin-inbound"
    operations = []
    for path in sorted((private / "operations").glob("*.json")):
        entry = json.loads(path.read_text())
        if entry["scope"].get("operation", "").startswith("native-account-"):
            operations.append(entry)
    callers = set()
    for path in sorted((private / "state").glob("*.json")):
        entry = json.loads(path.read_text())
        value = entry.get("value")
        if isinstance(value, dict):
            for receipt in value.get("receipts", []):
                if receipt.get("kind") == "assignment":
                    callers.add(receipt["caller"])
    return {"binding": record, "operations": operations, "assignmentCallers": sorted(callers)}


def decode(gateway, body, response_id):
    value = {"id": "devin.account.model-assignment", "responseId": response_id,
             "headers": {"Content-Type": ["application/proto"], "Connect-Protocol-Version": ["1"]},
             "body": base64.b64encode(body).decode()}
    lines = gateway.command("plugin", "sdk", "call", "devin-inbound", "inbound.decode", json.dumps(value))
    results = [json.loads(line)["result"] for line in lines.splitlines()
               if json.loads(line).get("kind") == "result"]
    require(len(results) == 1, "installed decode returned no unique result")
    return results[0]


def verify_bootstrap_fence(bootstrap, expect_stable_sync=False):
    require(bootstrap["before"] == bootstrap["atVendor"], "bootstrap changed before held reply")
    if expect_stable_sync:
        require(bootstrap["before"] == bootstrap["afterSync"] == bootstrap["after"],
                "no-op bootstrap sync changed durable authority or journal state")
        require(bootstrap["status"] == 200 and bootstrap["failure"] == "\x08\x01",
                "unchanged authority did not deliver the original native bootstrap reply")
        return
    before = dict(bootstrap["before"]["binding"])
    after = dict(bootstrap["afterSync"]["binding"])
    require(after.pop("revision") == before.pop("revision") + 1 and before == after,
            "held bootstrap sync changed more than the binding revision")
    require(bootstrap["afterSync"] == bootstrap["after"], "bootstrap state changed after releasing reply")
    require(bootstrap["status"] == 403 and
            json.loads(bootstrap["failure"]) ==
            {"code": "permission_denied", "message": "Native account operation failed"},
            "no-op sync did not reproduce the post-dispatch bootstrap binding fence")


class IdentityVendorHandler(agent.OwnedVendorHandler):
    def reply(self, content_type, body, status=200):
        if self.path.endswith("/GetCliTeamSettings") and self.server.hold_bootstrap:
            self.server.bootstrap_snapshot = snapshot(self.server.gateway)
            self.server.bootstrap_reached.set()
            require(self.server.bootstrap_release.wait(15), "held bootstrap reply was never released")
        super().reply(content_type, body, status)

    def do_POST(self):
        if self.path == SERVICE + "AssignModel":
            body = self.rfile.read(int(self.headers["Content-Length"]))
            decoded = fields(body)
            observed = {"wireSHA256": hashlib.sha256(body).hexdigest(),
                        "model": decoded[2][0].decode(), "session": decoded[3][0].decode(),
                        "snapshot": snapshot(self.server.gateway)}
            with self.server.lock:
                self.server.assignments.append(observed)
            self.rfile = io.BytesIO(body)
        super().do_POST()


def run(host, distribution, artifacts, expect_stable_sync=False):
    require(not artifacts.exists(), "diagnostic artifacts must use a fresh destination")
    artifacts.mkdir(parents=True)
    result = {"diagnosisStatus": "running", "acceptanceStatus": "red", "events": [],
              "hostSHA256": digest(host), "librarySHA256": digest(distribution / "plugin.so"),
              "runnerSHA256": digest(Path(__file__)), "expectStableSync": expect_stable_sync,
              "retries": 0, "hostSourceEdits": 0}
    prior_handler = paired.VendorHandler
    paired.VendorHandler = IdentityVendorHandler
    paired.SELECTED_KEY = "identity-fixture-" + secrets.token_hex(16)
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, paired.Vendor())
            stack.callback(vendor.stop.set)
            vendor.assignments = []
            vendor.hold_bootstrap = False
            vendor.bootstrap_reached = threading.Event()
            vendor.bootstrap_release = threading.Event()
            stack.callback(vendor.bootstrap_release.set)
            proxy = server_thread(stack, DenyProxy())
            home = Path(tempfile.mkdtemp(prefix="devin-write-identity-"))
            stack.callback(shutil.rmtree, home)
            gateway = agent.AgentGateway(host, distribution, home, artifacts,
                                         "http://127.0.0.1:" + str(proxy.server_port))
            vendor.gateway = gateway
            stack.callback(gateway.close)
            gateway.env["SHELL"] = "/bin/bash"
            executable = home / "bin/devin"
            executable.parent.mkdir()
            executable.write_text("#!/bin/sh\nexit 99\n")
            executable.chmod(0o700)
            gateway.env["PATH"] = str(executable.parent) + ":/usr/bin:/bin"
            for name in (".bashrc", ".zshrc"):
                (home / name).write_text("# isolated identity fixture\n")
            gateway.configure_agent("http://127.0.0.1:" + str(vendor.server_port))
            result["installedModules"] = gateway.require_single_bundle()
            Gateway.start(gateway)
            model = None
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                status, _, body = gateway.request("/v1/models", method="GET")
                require(status == 200, "diagnostic models unavailable")
                model = next((entry["id"] for entry in json.loads(body)["data"]
                              if entry["id"].endswith("/" + paired.MODEL)), None)
                if model:
                    break
                time.sleep(0.05)
            require(model is not None, "diagnostic native model never discovered")
            gateway.command("devin-native", "model", "magpie/" + model)
            request = (text_field(1, text_field(3, "magpie-devin-native"))
                       + text_field(2, paired.MODEL) + text_field(3, "identity-fixed-cascade"))
            decoded = decode(gateway, request, "identity-before")
            result["decodedBefore"] = decoded
            result["requestSHA256"] = hashlib.sha256(request).hexdigest()
            headers = {"Content-Type": "application/proto", "Connect-Protocol-Version": "1",
                       "Authorization": "Basic magpie-devin-native-magpie-devin-native",
                       "User-Agent": "devin-cli/0.0.0-synthetic"}

            def assignment(label):
                entry = {"label": label, "before": snapshot(gateway)}
                status, _, body = gateway.request(SERVICE + "AssignModel", request, key=None, headers=headers)
                entry.update(status=status, after=snapshot(gateway),
                             failure=body.decode(errors="replace") if status != 200 else "")
                with vendor.lock:
                    entry["dispatches"] = len(vendor.assignments)
                result["events"].append(entry)
                (artifacts / "observations.json").write_text(json.dumps(result, indent=2) + "\n")
                return entry

            first = assignment("first")
            require(first["status"] == 200 and first["dispatches"] == 1, "first assignment did not dispatch once")
            mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
            libraries = {line.split()[-1] for line in mappings.splitlines()
                         if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
            require(any(Path(path).is_file() and digest(Path(path)) == result["librarySHA256"]
                        for path in libraries), "actual host did not map the expected agent library")
            result["libraryMappedInHost"] = True
            control = assignment("duplicate-without-explicit-sync")
            require(control["status"] == 403 and control["dispatches"] == 1,
                    "control duplicate changed before explicit sync; retain observations")
            before_sync = snapshot(gateway)
            plan = {"id": "devin-native", "action": "sync",
                    "binding": {"target": "devin", "revision": str(before_sync["binding"]["revision"]),
                                "connected": True, "values": before_sync["binding"]["values"]},
                    "options": {"gateway": "http://127.0.0.1:" + str(gateway.port), "models": []}}
            result["moduleSyncPlan"] = json.loads(gateway.command(
                "plugin", "sdk", "call", "devin-inbound", "agent.plan", json.dumps(plan)))["result"]
            require(result["moduleSyncPlan"].get("files") == [], "sync does not declare a zero-file action")
            gateway.command("model", "name", model, "Identity fixture label")
            after_sync = snapshot(gateway)
            result["sync"] = {"trigger": ["model", "name", model, "Identity fixture label"],
                              "before": before_sync, "after": after_sync}
            decoded_after = decode(gateway, request, "identity-after")
            result["decodedAfter"] = decoded_after
            repeated = assignment("duplicate-after-catalog-sync")
            with vendor.lock:
                result["vendorAssignments"] = list(vendor.assignments)
            require(decoded == decoded_after, "module decode identity changed across catalog sync")
            if expect_stable_sync:
                require(before_sync == after_sync, "no-op catalog sync rewrote authority or completed journals")
                require(repeated["status"] == 403 and repeated["dispatches"] == 1 and
                        repeated["before"] == repeated["after"],
                        "duplicate after no-op sync dispatched or changed its original journal")
            else:
                require(before_sync["binding"]["revision"] != after_sync["binding"]["revision"],
                        "catalog mutation did not apply native binding sync")
                require(repeated["status"] == 200 and repeated["dispatches"] == 2,
                        "controlled no-op sync did not reproduce duplicate dispatch")
            left, right = dict(before_sync["binding"]), dict(after_sync["binding"])
            left.pop("revision")
            right.pop("revision")
            require(left == right, "catalog sync changed semantic binding inputs")
            callers = repeated["after"]["assignmentCallers"]
            require(len(callers) == 1, "caller identity changed or assignment receipt is absent")
            result["identityPreimages"] = []
            for entry in (first, repeated):
                binding = entry["before"]["binding"]
                operations = entry["after"]["operations"]
                prior = {row["scope"]["operation"] for row in entry["before"]["operations"]}
                added = [row for row in operations if row["scope"]["operation"] not in prior]
                if expect_stable_sync and entry is repeated:
                    require(not added and len(operations) == 1, "duplicate allocated another native write identity")
                    added = operations
                require(len(added) == 1 and added[0]["state"] == "completed", "no unique completed write journal")
                scope = added[0]["scope"]
                identity = ["devin-inbound", "model-assignment", str(binding["revision"]),
                            scope["provider"], scope["account"], callers[0], decoded["requestId"]]
                encoded = json.dumps(identity, separators=(",", ":")).encode()
                expected = "native-account-" + hashlib.sha256(encoded).hexdigest()
                require(scope["operation"] == expected, "observed operation does not match exact host preimage")
                require(scope["session"] == decoded["session"], "host scope changed module session")
                result["identityPreimages"].append({"inputs": identity, "operation": expected, "scope": scope})
            vendor.hold_bootstrap = True
            bootstrap = {"before": snapshot(gateway)}
            with ThreadPoolExecutor(max_workers=1) as pending:
                future = pending.submit(gateway.request,
                    "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings",
                    text_field(1, text_field(3, "magpie-devin-native")), key=None, headers=headers)
                try:
                    require(vendor.bootstrap_reached.wait(10), "native bootstrap read never reached local vendor")
                    bootstrap["atVendor"] = vendor.bootstrap_snapshot
                    gateway.command("model", "name", model, "Identity fixture second label")
                    bootstrap["afterSync"] = snapshot(gateway)
                finally:
                    vendor.bootstrap_release.set()
                status, response_headers, body = future.result(timeout=15)
            bootstrap.update(status=status, headers=response_headers, failure=body.decode(errors="replace"),
                             after=snapshot(gateway))
            result["heldBootstrap"] = bootstrap
            verify_bootstrap_fence(bootstrap, expect_stable_sync)
            require(not proxy.requests and not vendor.failures, "diagnostic crossed local vendor boundaries")
            result["teamReadDispatches"] = vendor.calls.count(
                "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings")
            require(result["teamReadDispatches"] == 1, "held native read was retried")
            result["diagnosisStatus"] = "stable-no-op-fenced" if expect_stable_sync else "duplicate-reproduced"
            if expect_stable_sync:
                result["acceptanceStatus"] = "no-op-regression-passed"
    finally:
        paired.VendorHandler = prior_handler
        if "home" in locals():
            result["homeRemoved"] = not home.exists()
        if "gateway" in locals():
            result["processStopped"] = gateway.process is None or gateway.process.poll() is not None
        if "proxy" in locals():
            result["externalRequests"] = len(proxy.requests)
        if "vendor" in locals():
            result["vendorFailures"] = list(vendor.failures)
        if result["diagnosisStatus"] == "running":
            result["diagnosisStatus"] = "unexpected"
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "diagnostic retained a process or HOME")
    print(json.dumps({key: value for key, value in result.items()
                      if key in ("diagnosisStatus", "acceptanceStatus", "identityPreimages", "sync",
                                 "homeRemoved", "processStopped", "externalRequests")}, indent=2))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--magpie", type=Path, required=True)
    parser.add_argument("--distribution", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    parser.add_argument("--expect-stable-sync", action="store_true")
    options = parser.parse_args()
    run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve(),
        options.expect_stable_sync)
