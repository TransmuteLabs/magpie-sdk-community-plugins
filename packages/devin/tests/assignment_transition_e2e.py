"""Keep real model transitions fenced while diagnosing false no-op revocation."""

import argparse
from concurrent.futures import ThreadPoolExecutor
from contextlib import ExitStack
import json
from pathlib import Path
import secrets
import tempfile
import threading
import time

import agent_http_e2e as agent
import paired_http_e2e as paired
from assignment_identity_e2e import IdentityVendorHandler, decode, snapshot
from conformance import text_field
from http_e2e import DenyProxy, Gateway, SERVICE, digest, require, server_thread


class TransitionVendorHandler(IdentityVendorHandler):
    def reply(self, content_type, body, status=200):
        if self.path == SERVICE + "AssignModel" and self.server.hold_assignment:
            self.server.assignment_snapshot = snapshot(self.server.gateway)
            self.server.assignment_reached.set()
            require(self.server.assignment_release.wait(15), "held assignment reply was never released")
        super().reply(content_type, body, status)


def require_transition(before, after, old_model, new_model):
    left, right = before["binding"], after["binding"]
    require(left["phase"] == right["phase"] == "owned" and
            right["revision"] == left["revision"] + 1, "real selection did not advance owned revision once")
    require(left["values"]["model"] == old_model and right["values"]["model"] == new_model and
            old_model != new_model, "real selection did not change its binding model")
    for name in ("plugin", "agent", "descriptor", "bin", "packageRevision", "configVersion"):
        require(left.get(name) == right.get(name), "model transition changed unrelated authority: " + name)


def require_fenced(entry):
    require(entry["status"] == 403 and json.loads(entry["failure"]) ==
            {"code": "permission_denied", "message": "Native account operation failed"},
            "real binding transition did not refuse the original operation result")
    require(entry["afterChange"] == entry["afterRelease"],
            "releasing a fenced reply changed the binding or accepted-write journal")


def run(host, distribution, artifacts):
    require(not artifacts.exists(), "transition proof requires a fresh destination")
    artifacts.mkdir(parents=True)
    result = {"status": "running", "bootstrapAcceptance": "red", "events": [],
              "hostSHA256": digest(host), "librarySHA256": digest(distribution / "plugin.so"),
              "runnerSHA256": digest(Path(__file__)), "automaticRetries": 0, "hostSourceEdits": 0}
    prior_handler = paired.VendorHandler
    paired.VendorHandler = TransitionVendorHandler
    paired.SELECTED_KEY = "transition-fixture-" + secrets.token_hex(16)
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, paired.Vendor())
            stack.callback(vendor.stop.set)
            vendor.assignments = []
            vendor.hold_bootstrap = False
            vendor.bootstrap_reached = threading.Event()
            vendor.bootstrap_release = threading.Event()
            vendor.hold_assignment = False
            vendor.assignment_reached = threading.Event()
            vendor.assignment_release = threading.Event()
            stack.callback(vendor.bootstrap_release.set)
            stack.callback(vendor.assignment_release.set)
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-transition-home-")))
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
                (home / name).write_text("# isolated real-transition fixture\n")
            gateway.configure_agent("http://127.0.0.1:" + str(vendor.server_port))
            result["installedModules"] = gateway.require_single_bundle()
            Gateway.start(gateway)
            models = {}
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                status, _, body = gateway.request("/v1/models", method="GET")
                require(status == 200, "transition models unavailable")
                models = {name: entry["id"] for name in (paired.MODEL, "fixture-free")
                          for entry in json.loads(body)["data"] if entry["id"].endswith("/" + name)}
                if len(models) == 2:
                    break
                time.sleep(0.05)
            require(len(models) == 2, "both native models were not discovered")
            selected = {name: "magpie/" + value for name, value in models.items()}
            gateway.command("devin-native", "model", selected[paired.MODEL])
            headers = {"Content-Type": "application/proto", "Connect-Protocol-Version": "1",
                       "Authorization": "Basic magpie-devin-native-magpie-devin-native",
                       "User-Agent": "devin-cli/0.0.0-synthetic"}
            metadata = text_field(1, text_field(3, "magpie-devin-native"))

            def event(label):
                result["events"].append({"label": label, "monotonicNS": time.monotonic_ns()})

            def count():
                with vendor.lock:
                    return len(vendor.assignments)

            def request(body):
                return gateway.request(SERVICE + "AssignModel", body, key=None, headers=headers)

            def assignment(cascade):
                return metadata + text_field(2, paired.MODEL) + text_field(3, cascade)

            control_body = assignment("transition-stable-control")
            control = {"before": snapshot(gateway), "decoded": decode(gateway, control_body, "transition-control")}
            result["stableControl"] = control
            status, _, body = request(control_body)
            control.update(status=status, afterFirst=snapshot(gateway), firstDispatches=count())
            require(status == 200 and count() == 1, "initial transition control did not dispatch once")
            status, _, body = request(control_body)
            control.update(duplicateStatus=status, afterDuplicate=snapshot(gateway), duplicateDispatches=count(),
                           duplicateFailure=body.decode(errors="replace"))
            require(status == 403 and count() == 1 and control["afterFirst"] == control["afterDuplicate"],
                    "stable duplicate was not refused with the original completed journal intact")
            mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
            libraries = {line.split()[-1] for line in mappings.splitlines()
                         if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
            require(any(Path(path).is_file() and digest(Path(path)) == result["librarySHA256"]
                        for path in libraries), "actual host did not map the frozen agent library")
            result["libraryMappedInHost"] = True
            event("stable-control-completed-and-duplicate-refused")

            vendor.hold_bootstrap = True
            read = {"before": snapshot(gateway)}
            result["heldRead"] = read
            with ThreadPoolExecutor(max_workers=1) as pending:
                future = pending.submit(gateway.request,
                    "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings",
                    metadata, key=None, headers=headers)
                try:
                    require(vendor.bootstrap_reached.wait(8), "held real-transition read never reached local peer")
                    read["atVendor"] = vendor.bootstrap_snapshot
                    event("read-at-vendor")
                    read["command"] = ["devin-native", "model", selected["fixture-free"]]
                    gateway.command(*read["command"])
                    read["afterChange"] = snapshot(gateway)
                    event("read-model-transition-completed")
                finally:
                    event("read-reply-released")
                    vendor.bootstrap_release.set()
                status, response_headers, body = future.result(timeout=12)
            read.update(status=status, headers=response_headers, failure=body.decode(errors="replace"),
                        afterRelease=snapshot(gateway))
            event("read-result-received")
            require(read["before"] == read["atVendor"], "read admission changed fixture state")
            require_transition(read["before"], read["afterChange"], models[paired.MODEL], models["fixture-free"])
            require_fenced(read)
            vendor.hold_bootstrap = False

            stale_body = assignment("transition-distinct-old-model")
            stale = {"before": snapshot(gateway), "dispatchesBefore": count(),
                     "decoded": decode(gateway, stale_body, "transition-stale")}
            result["staleModel"] = stale
            status, _, body = request(stale_body)
            stale.update(status=status, failure=body.decode(errors="replace"), after=snapshot(gateway),
                         dispatchesAfter=count())
            require(status == 403 and stale["dispatchesBefore"] == stale["dispatchesAfter"],
                    "a separately submitted old-model assignment reached the vendor")
            previous = {row["scope"]["operation"] for row in stale["before"]["operations"]}
            rejected = [row for row in stale["after"]["operations"] if row["scope"]["operation"] not in previous]
            require(len(rejected) == 1 and rejected[0]["state"] == "not-started",
                    "old-model payload refusal was not journaled as not-started")
            event("distinct-old-model-assignment-refused-before-vendor")

            gateway.command("devin-native", "model", selected[paired.MODEL])
            held_body = assignment("transition-distinct-held-write")
            write = {"before": snapshot(gateway), "dispatchesBefore": count(),
                     "decodedBefore": decode(gateway, held_body, "transition-write-before")}
            result["heldWrite"] = write
            vendor.hold_assignment = True
            with ThreadPoolExecutor(max_workers=1) as pending:
                future = pending.submit(request, held_body)
                try:
                    require(vendor.assignment_reached.wait(8), "new accepted assignment never reached local peer")
                    write["atVendor"] = vendor.assignment_snapshot
                    event("write-accepted-at-vendor")
                    write["command"] = ["devin-native", "model", selected["fixture-free"]]
                    gateway.command(*write["command"])
                    write["afterChange"] = snapshot(gateway)
                    write["dispatchesAfterChange"] = count()
                    event("write-model-transition-completed")
                finally:
                    event("write-reply-released")
                    vendor.assignment_release.set()
                status, response_headers, body = future.result(timeout=12)
            write.update(status=status, headers=response_headers, failure=body.decode(errors="replace"),
                         afterRelease=snapshot(gateway), dispatchesAfterRelease=count(),
                         decodedAfter=decode(gateway, held_body, "transition-write-after"))
            event("write-result-received")
            require(write["before"]["binding"] == write["atVendor"]["binding"] and
                    write["atVendor"]["operations"] == write["afterChange"]["operations"],
                    "held write changed authority or accepted journal before its reply")
            require_transition(write["atVendor"], write["afterChange"], models[paired.MODEL], models["fixture-free"])
            require(write["decodedBefore"] == write["decodedAfter"], "held write changed module request identity")
            previous = {row["scope"]["operation"] for row in write["before"]["operations"]}
            accepted = [row for row in write["atVendor"]["operations"] if row["scope"]["operation"] not in previous]
            require(len(accepted) == 1 and accepted[0]["state"] == "accepted", "held write had no accepted journal")
            operation = accepted[0]["scope"]["operation"]
            completed = [row for row in write["afterRelease"]["operations"] if row["scope"]["operation"] == operation]
            require(len(completed) == 1 and completed[0]["state"] == "completed" and
                    completed[0]["scope"] == accepted[0]["scope"], "fenced write lost its original accepted scope")
            require(write["status"] == 403 and json.loads(write["failure"]) ==
                    {"code": "permission_denied", "message": "Native account operation failed"},
                    "real model transition admitted an old in-flight write response")
            require(write["afterChange"]["binding"] == write["afterRelease"]["binding"] and
                    write["dispatchesAfterChange"] == write["dispatchesAfterRelease"] == write["dispatchesBefore"] + 1,
                    "fenced accepted write mutated selection or dispatched again")
            with vendor.lock:
                result["vendorAssignments"] = list(vendor.assignments)
                result["teamReadDispatches"] = vendor.calls.count(
                    "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings")
            require(result["teamReadDispatches"] == 1 and count() == 2, "transition fixture replayed an operation")
            require(not proxy.requests and not vendor.failures, "transition fixture crossed a native peer boundary")
            result["status"] = "real-transitions-fenced"
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
        if result["status"] == "running":
            result["status"] = "unexpected"
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "transition fixture retained its process or HOME")
    print(json.dumps({key: result[key] for key in
                      ("status", "bootstrapAcceptance", "teamReadDispatches", "externalRequests",
                       "homeRemoved", "processStopped")}))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--magpie", type=Path, required=True)
    parser.add_argument("--distribution", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    options = parser.parse_args()
    run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve())
