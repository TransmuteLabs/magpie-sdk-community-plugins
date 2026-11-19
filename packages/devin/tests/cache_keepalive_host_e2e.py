"""Installed full Devin package, selected-account paid tasks, synthetic loopback only."""

import argparse
from contextlib import ExitStack
import gzip
import hashlib
import json
from pathlib import Path
import secrets
import struct
import subprocess
import tempfile
import threading
import time

import agent_http_e2e as agent
import paired_http_e2e as paired
from conformance import connect_frames, data_frame, fields, number_field, read_varint, text_field
from http_e2e import CALLER_KEY, READONLY_KEY, DenyProxy, Provider, SERVICE, digest, require, server_thread


class CacheVendorHandler(paired.VendorHandler):
    def do_POST(self):
        if self.path != SERVICE + "GetChatMessage":
            super().do_POST()
            return
        try:
            body = self.rfile.read(int(self.headers["Content-Length"]))
            frames = connect_frames(body)
            require(len(frames) == 1 and frames[0][0] in (0, 1), "invalid paid request framing")
            payload = gzip.decompress(frames[0][1]) if frames[0][0] else frames[0][1]
            native = fields(payload)
            require(native[21] == [paired.MODEL.encode()] and
                    self.headers.get("Authorization") == "Basic " + paired.SELECTED_KEY and
                    paired.SELECTED_KEY.encode() in native[1][0] and CALLER_KEY.encode() not in payload,
                    "paid request lost selected-account authority")
            prompts = [fields(value) for value in native.get(3, [])]
            first = next((value.get(3, [b""])[0].decode() for value in prompts if value.get(2) == [1]), "")
            ping = len(prompts) > 1 and prompts[-1].get(3) == [b"continue"]
            with self.server.lock:
                self.server.chat.append(payload)
                self.server.calls.append(self.path)
                self.server.cache_arrivals.append({"marker": first, "ping": ping, "payload": payload,
                                                   "time": time.monotonic()})
            write, read = (0, 0) if first == "replacement-no-cache" else (5, 17)
            usage = (number_field(2, 7) + number_field(3, 0) + number_field(4, write) + number_field(5, read)
                     + number_field(6, 4))
            reply = (text_field(3, "paid fixture answer") + text_field(7, usage)
                     + text_field(17, "cache-" + first) + number_field(5, 2))
            if ping:
                reply += b"\xb1\x01" + struct.pack("<d", 2.5)
            if ping and first == "held-prefix":
                self.send_response(200)
                self.send_header("Content-Type", "application/connect+proto")
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(data_frame(reply))
                self.wfile.flush()
                self.server.paid_reached.set()
                self.connection.settimeout(12)
                try:
                    ended = self.connection.recv(1)
                    require(ended == b"", "held paid connection sent unexpected trailing bytes")
                    self.server.paid_canceled.set()
                except (OSError, TimeoutError):
                    if not self.server.stop.is_set():
                        raise AssertionError("held paid connection was not canceled")
                self.close_connection = True
                return
            self.reply("application/connect+proto", data_frame(reply) + b"\2\0\0\0\2{}")
        except BaseException as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.close_connection = True
            try:
                self.reply("application/json", b'{"code":"internal"}', 500)
            except OSError:
                pass


def wait_for(message, predicate, timeout=8):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(0.05)
    raise AssertionError(message)


def task_intents(gateway):
    private = gateway.config / "extensions/private/devin-inbound/account-tasks"
    return [json.loads(path.read_text())["receipt"] for path in sorted(private.glob("*.json"))]


def paid_rows(gateway):
    ledger = gateway.config / "usage.jsonl"
    if not ledger.exists():
        return []
    return [row for line in ledger.read_text().splitlines()
            if (row := json.loads(line)).get("kind") == "cache_keepalive"]


def safe_rows(rows):
    allowed = ("provider", "providerAccount", "model", "kind", "status", "in", "out", "cache_read", "cache_write",
               "rid", "upstream_response_id", "upstream", "reported_cost", "own_cost", "callerKeyId",
               "operation", "err", "error", "source")
    return [{key: row[key] for key in allowed if key in row} for row in rows]


def sampling_chunks(body):
    chunks, position = [], 0
    while position < len(body):
        start = position
        tag, position = read_varint(body, position)
        number, wire = tag >> 3, tag & 7
        require(number > 0 and wire in (0, 1, 2, 5), "invalid sampling field")
        if wire == 0:
            _, position = read_varint(body, position)
        elif wire == 2:
            size, position = read_varint(body, position)
            position += size
        else:
            position += 8 if wire == 1 else 4
        require(position <= len(body), "truncated sampling field")
        chunks.append((number, wire, body[start:position]))
    return chunks


def verify_template(original, paid):
    before, after = fields(original), fields(paid)
    require(after[3][:-1] == before[3], "paid task changed original prompt bytes")
    extra = fields(after[3][-1])
    require(set(extra) == {1, 2, 3} and extra[2] == [1] and extra[3] == [b"continue"] and extra[1][0],
            "paid task continue gained cache controls or lost its new user ID")
    original_sampling, ping_sampling = sampling_chunks(before[8][0]), sampling_chunks(after[8][0])
    expected_sampling = (number_field(1, 1) + number_field(2, 64)
                         + b"\x29" + struct.pack("<d", 0.0) + b"\x41" + struct.pack("<d", 0.0)
                         + text_field(9, "cache-stop") + number_field(10, 7)
                         + number_field(3, 400) + number_field(7, 40))
    require(original_sampling == sampling_chunks(expected_sampling), "foreground sampling differs from its exact fixture")
    require([chunk for chunk in ping_sampling if chunk[0] == 2] == [(2, 0, number_field(2, 1))],
            "paid task output exceeds one token")
    require([chunk for chunk in original_sampling if chunk[0] != 2] ==
            [chunk for chunk in ping_sampling if chunk[0] != 2], "paid task changed other sampling bytes")
    require(len(before.get(10, [])) == 1 and after.get(10) == before[10] and
            fields(before[10][0]).get(1) == [b"Read"] and
            before.get(12) == [text_field(2, "Read")] and after.get(12) == before[12] and
            before.get(11) == [1] and after.get(11) == [1],
            "paid task changed its named tool choice, definition or parallel mode")
    for number in set(before) | set(after):
        if number not in (1, 3, 8):
            require(before.get(number) == after.get(number), "paid task changed preserved field " + str(number))


def run(host, distribution, artifacts, cold_start=False):
    require(not artifacts.exists(), "paid-task evidence needs a fresh directory")
    artifacts.mkdir(parents=True)
    result = {"status": "running", "hostSHA256": digest(host), "librarySHA256": digest(distribution / "plugin.so"),
              "runnerSHA256": digest(Path(__file__)), "hostSourceEdits": 0,
              "activation": "cold-start" if cold_start else "hot-config-cas", "checks": []}
    prior_handler = paired.VendorHandler
    paired.VendorHandler = CacheVendorHandler
    paired.SELECTED_KEY = "paid-fixture-" + secrets.token_hex(16)
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, paired.Vendor())
            vendor.cache_arrivals = []
            vendor.paid_reached, vendor.paid_canceled = threading.Event(), threading.Event()
            stack.callback(vendor.stop.set)
            foreign = server_thread(stack, Provider())
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-paid-home-")))
            gateway = paired.PairedGateway(host, distribution, home, artifacts,
                                            "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)

            def capture_before_cleanup():
                if gateway.process and gateway.process.poll() is None:
                    status, _, trace = gateway.request("/v1/magpie/trace", method="GET", key=None)
                    if status == 200:
                        result["liveRouting"] = [
                            {key: row[key] for key in ("status", "error", "fail", "tries", "done") if key in row}
                            for row in json.loads(trace).get("routes", [])
                        ]
                gateway.close()
                with vendor.lock:
                    result["arrivals"] = [{"marker": row["marker"], "ping": row["ping"]}
                                          for row in vendor.cache_arrivals]
                    result["peerFailures"] = list(vendor.failures)
                result["terminalIntents"] = [{"operation": row["scope"]["operation"], "state": row["state"]}
                                             for row in task_intents(gateway)]
                result["measuredRows"] = safe_rows(paid_rows(gateway))
                ledger = gateway.config / "usage.jsonl"
                result["foregroundRows"] = safe_rows(
                    [json.loads(line) for line in ledger.read_text().splitlines()] if ledger.exists() else [])
                result["routingFailures"] = [
                    {key: row[key] for key in ("status", "error", "fail", "tries") if key in row}
                    for path in sorted((gateway.config / "routing").glob("*.jsonl"))
                    for line in path.read_text().splitlines()
                    if (row := json.loads(line)).get("error")
                ]

            stack.callback(capture_before_cleanup)
            gateway.configure_paired("http://127.0.0.1:" + str(vendor.server_port),
                                     "http://127.0.0.1:" + str(foreign.server_port))
            result["installedModules"] = agent.AgentGateway.require_single_bundle(gateway)
            installed = json.loads(gateway.command("plugin", "sdk", "list", "--json"))[0]["manifest"]
            result["packageVersion"] = installed["version"]
            packaged = json.loads((distribution / "magpie-plugin.json").read_text())
            contract = installed["contract"]["version"]
            require(installed["version"] == packaged["version"] and contract["major"] == 1 and contract["minor"] >= 9 and
                    installed["providers"][0]["accountTasks"] ==
                    [{"id": "cache-keepalive", "action": "devin.cache-keepalive"}],
                    "default artifact lacks its full task contract")
            switch = installed["providers"][0]["options"][0]
            require(switch["key"] == "keepAlive" and switch.get("default", False) is False and
                    "paid" in switch["confirm"], "paid switch lost explicit default-off consent")
            gateway.start()

            def catalog_model():
                status, _, raw = gateway.request("/v1/models", method="GET")
                require(status == 200, "paid fixture catalog unavailable")
                return next((row["id"] for row in json.loads(raw)["data"]
                             if row["id"].endswith("/" + paired.MODEL)), None)

            model = wait_for("paid fixture model was not discovered", catalog_model)
            mapped = Path("/proc", str(gateway.process.pid), "maps").read_text()
            libraries = {line.split()[-1] for line in mapped.splitlines()
                         if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
            require(any(Path(path).is_file() and digest(Path(path)) == result["librarySHA256"]
                        for path in libraries), "default library is not mapped in the actual host")
            result["libraryMappedInHost"] = True

            def execute(marker, session, key=CALLER_KEY):
                body = json.dumps({"model": model, "max_tokens": 64, "stream": False,
                                   "temperature": 0, "top_p": 0, "seed": 7, "stop": ["cache-stop"],
                                   "tools": [{"type": "function", "function": {
                                       "name": "Read", "parameters": {"type": "object"}}}],
                                   "tool_choice": {"type": "function", "function": {"name": "Read"}},
                                   "parallel_tool_calls": False,
                                   "messages": [{"role": "user", "content": marker}]}).encode()
                return gateway.request("/v1/chat/completions", body, key=key,
                                       headers={"Content-Type": "application/json", "X-Magpie-Session": session})

            require(execute("off-prefix", "off-proof")[0] == 200 and not task_intents(gateway),
                    "default-off created a paid task")
            result["checks"].append("ordinary default-off completion creates no task intent")
            if cold_start:
                gateway.close()
                (artifacts / "default-off-gateway.log").write_bytes((artifacts / "gateway.log").read_bytes())
            config = json.loads(gateway.command("plugin", "sdk", "config", "devin-inbound"))
            enabled = dict(config["values"], keepAlive=True, cacheKeepaliveInterval=30, cacheKeepaliveMaxPings=1)
            gateway.command("plugin", "sdk", "config", "devin-inbound", json.dumps(enabled),
                            "--expected", config["version"])
            for name, key in (("Paid budget", "owned-paid-budget"), ("Paid revoked", "owned-paid-revoked")):
                gateway.command("gateway-key", "add", name, "--key", key)
            keys = json.loads((gateway.config / "caller-keys.json").read_text())
            identities = {row["name"]: row["id"] for row in keys}
            if cold_start:
                gateway.start()
                model = wait_for("paid cold-start model was not discovered", catalog_model)
                result["checks"].append("paid policy is loaded at ordinary cold startup")
            else:
                result["checks"].append("paid policy changes through hot external config CAS without restart or catalog warmup")
            require(execute("budget-prefix", "paid-budget", "owned-paid-budget")[0] == 200,
                    "budget control could not make its one foreground request")
            gateway.command("gateway-key", "limit", identities["Paid budget"], "day", "--tokens", "1", "--model", model)
            require(execute("revoked-prefix", "paid-revoked", "owned-paid-revoked")[0] == 200,
                    "revocation control foreground failed")
            gateway.command("gateway-key", "models", identities["Paid revoked"], "fixture/visible")
            before = vendor.count()
            require(execute("readonly-prefix", "paid-readonly", READONLY_KEY)[0] == 403 and vendor.count() == before,
                    "read-only caller created paid or foreground execution")
            require(execute("completed-prefix", "paid-completed")[0] == 200 and
                    execute("held-prefix", "paid-held")[0] == 200 and len(task_intents(gateway)) == 4,
                    "independent paid foregrounds did not reserve four task intents")
            require(vendor.paid_reached.wait(38), "admitted paid task did not reach the local peer")
            wait_for("completed paid task was not accounted", lambda: paid_rows(gateway))
            require(execute("replacement-no-cache", "paid-held")[0] == 200,
                    "new foreground failed to cancel and join held paid work")
            require(vendor.paid_canceled.wait(5), "held HTTP ping survived the next real turn")
            intents = wait_for("paid tasks did not reach terminal receipts",
                               lambda: (rows if len(rows := task_intents(gateway)) >= 4 and
                                        all(row["state"] != "reserved" for row in rows[:4]) else None))
            rows = wait_for("canceled measured usage was lost", lambda: (rows if len(rows := paid_rows(gateway)) == 2 else None))
            require(sorted(row["state"] for row in intents[:4]) == ["completed", "not-started", "not-started", "uncertain"],
                    "paid admission/cancellation terminal states differ")
            require(all(row.get("callerKeyId") == identities["Fixture caller"] for row in rows),
                    "paid usage lost original caller ownership")
            require(all([row.get(key) for key in ("in", "out", "cache_read", "cache_write")] == [7, 0, 17, 5]
                        and row.get("reported_cost") == {"amount": 2.5, "currency": "ACU"} and
                        "own_cost" not in row for row in rows), "paid measured usage or ACU units changed")
            require(len({row.get("operation") for row in rows}) == 2, "paid snapshots duplicated ledger rows")
            with vendor.lock:
                arrivals = list(vendor.cache_arrivals)
            paid = [row for row in arrivals if row["ping"]]
            require(sorted(row["marker"] for row in paid) == ["completed-prefix", "held-prefix"],
                    "budget or revoked task dispatched, or a paid tick duplicated")
            for row in paid:
                original = next(item for item in arrivals if not item["ping"] and item["marker"] == row["marker"])
                verify_template(original["payload"], row["payload"])
                require(row["time"] - original["time"] >= 29.5, "paid dispatch ran before its configured cadence")
            result["checks"] += [
                "one installed full module schedules independent conversation tasks at the real 30s cadence",
                "fresh spent-budget and revoked-model controls refuse paid dispatch with not-started receipts",
                "one completed ping and one held canceled ping each preserve exactly one measured ACU usage row",
                "new foreground cancels and joins held paid HTTP before completing",
                "paid native bytes preserve original prompts/tools/cache/sampling with one new continue and max_tokens1",
            ]
            terminal = next(row for row in intents if row["state"] == "completed")
            before = vendor.count()
            denied = subprocess.run(
                [str(host), "plugin", "sdk", "call", "devin-inbound", "module.call",
                 json.dumps({"action": "devin.cache-keepalive", "input": {}}),
                 "--scope", json.dumps(terminal["scope"])],
                env=gateway.env, cwd=home, capture_output=True, timeout=15)
            require(denied.returncode != 0 and vendor.count() == before, "ordinary RPC replay paid twice")
            result["checks"].append("ordinary module.call cannot replay a completed paid task")
            require(not proxy.requests and not foreign.snapshot() and not vendor.failures,
                    "paid fixture escaped local peers or produced a peer failure")
            log = (artifacts / "gateway.log").read_text()
            require(paired.SELECTED_KEY not in log and CALLER_KEY not in log, "diagnostics leaked credentials")
            result["ledger"] = safe_rows(rows)
            result["intents"] = [{"operation": row["scope"]["operation"], "state": row["state"]} for row in intents]
            result["paidRequests"] = [{"marker": row["marker"], "payloadSHA256": hashlib.sha256(row["payload"]).hexdigest()}
                                      for row in paid]
            result["vendorRequests"] = len(arrivals)
            result["externalRequests"] = len(proxy.requests)
            gateway.close()
            result["processStopped"] = gateway.process.poll() is not None
        result["homeRemoved"] = not home.exists()
        require(result["homeRemoved"] and result["processStopped"], "paid fixture leaked a home or process")
        result["status"] = "pass"
    except BaseException as error:
        result["status"], result["failure"] = "fail", str(error)
        raise
    finally:
        paired.VendorHandler = prior_handler
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("host", type=Path)
    parser.add_argument("distribution", type=Path)
    parser.add_argument("artifacts", type=Path)
    parser.add_argument("--cold-start", action="store_true")
    arguments = parser.parse_args()
    run(arguments.host.resolve(), arguments.distribution.resolve(), arguments.artifacts.resolve(), arguments.cold_start)
