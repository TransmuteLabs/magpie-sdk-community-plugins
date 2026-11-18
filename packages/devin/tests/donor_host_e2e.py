"""Exercise existing donor-domain ports through an unchanged installed host."""

import argparse
from contextlib import ExitStack
import gzip
import http.client
import json
from pathlib import Path
import secrets
import signal
import struct
import tempfile
import threading
import time

import agent_http_e2e as agent
import paired_http_e2e as paired
from conformance import connect_frames, data_frame, fields, number_field, text_field
from http_e2e import CALLER_KEY, DenyProxy, Provider, SERVICE, digest, require, server_thread


def price(label, value):
    return text_field(32, text_field(1, label) + b"\x15" + struct.pack("<f", value)
                      + text_field(3, "1M tokens"))


class DomainVendorHandler(paired.VendorHandler):
    def reply(self, content_type, body, status=200):
        if self.path.endswith("/GetUserStatus") and status == 200 and hasattr(self.server, "acu_fields"):
            value = json.loads(body)
            value["userStatus"]["planStatus"].update(self.server.acu_fields)
            body = json.dumps(value).encode()
        if self.path == SERVICE + "GetCliModelConfigs" and status == 200:
            models = fields(body)[1]
            paid = models[0] + price("Input", 1.4) + price("Output", 4.4) + price("Cached input", 0.26)
            info = number_field(4, 32768) + number_field(13, 4096)
            claude = text_field(1, "Synthetic Claude") + text_field(22, "claude-fixture")
            claude += number_field(18, 32768) + text_field(23, info)
            body = text_field(1, paid) + text_field(1, models[1]) + text_field(1, claude)
        super().reply(content_type, body, status)

    def do_POST(self):
        if self.path != SERVICE + "GetChatMessage":
            super().do_POST()
            return
        try:
            with self.server.lock:
                self.server.domain_arrivals.append({"path": self.path, "length": self.headers.get("Content-Length")})
            size = int(self.headers["Content-Length"])
            require(0 < size <= 16 << 20, "unbounded native domain request")
            body = self.rfile.read(size)
            frames = connect_frames(body)
            require(len(frames) == 1 and frames[0][0] in (0, 1), "invalid native domain frame")
            payload = gzip.decompress(frames[0][1]) if frames[0][0] else frames[0][1]
            value = fields(payload)
            require(self.headers.get("Authorization") == "Basic " + paired.SELECTED_KEY and
                    paired.SELECTED_KEY.encode() in value[1][0] and
                    CALLER_KEY.encode() not in payload, "domain request lost selected-account authority")
            require(value[21][0].decode() in (paired.MODEL, "claude-fixture"), "unknown domain model")
            with self.server.lock:
                self.server.calls.append(self.path)
                self.server.chat.append(payload)
            prompts = [fields(item) for item in value.get(3, [])]
            text = b"".join(item.get(3, [b""])[0] for item in prompts)
            reply = text_field(3, "domain answer") + number_field(5, 2)
            if b"usage-counted" in text or b"usage-zero" in text:
                counters = (7, 3, 5, 11) if b"usage-counted" in text else (0, 0, 0, 0)
                usage = b"".join(number_field(index, count) for index, count in zip((2, 3, 4, 5), counters))
                if getattr(self.server, "execution_facts", False):
                    marker = "usage-counted" if b"usage-counted" in text else "usage-zero"
                    usage += number_field(6, 4) + text_field(9, "usage-fallback-model")
                    reply += text_field(1, "native-" + marker) + text_field(17, "upstream-" + marker)
                    reply += text_field(23, paired.MODEL + "-served") + number_field(18, 2 if counters[0] else 0)
                reply += text_field(7, usage)
                reply += b"\xb1\x01" + struct.pack("<d", 2.5)
            self.reply("application/connect+proto",
                       data_frame(reply) + b"\2\0\0\0\2{}")
        except BaseException as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.reply("application/json", b'{"code":"internal"}', 500)


def call(identity):
    return {"id": identity, "type": "function", "function": {"name": "shot", "arguments": "{}"}}


def image(value):
    return {"type": "image_url", "image_url": {"url": "data:image/png;base64," + value}}


def run(host, distribution, artifacts, selected_case=None, sample_stall=False):
    require(not artifacts.exists(), "domain proof requires a fresh destination")
    artifacts.mkdir(parents=True)
    result = {"status": "running", "bootstrapAcceptance": "not-exercised", "cases": [], "failures": [],
              "hostSHA256": digest(host), "librarySHA256": digest(distribution / "plugin.so"),
              "runnerSHA256": digest(Path(__file__)), "retries": 0, "hostSourceEdits": 0}
    numeric_quota = json.loads((distribution / "magpie-plugin.json").read_text())["contract"]["version"]["minor"] >= 9
    result.update(selectedCase=selected_case, sampledBeforeCancellation=sample_stall)
    prior_handler = paired.VendorHandler
    paired.VendorHandler = DomainVendorHandler
    paired.SELECTED_KEY = "domain-fixture-" + secrets.token_hex(16)
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, paired.Vendor())
            vendor.domain_arrivals = []
            vendor.execution_facts = selected_case == "execution-facts-ledger"
            stack.callback(vendor.stop.set)
            foreign = server_thread(stack, Provider())
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-domain-home-")))
            gateway = paired.PairedGateway(host, distribution, home, artifacts,
                                            "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)
            gateway.configure_paired("http://127.0.0.1:" + str(vendor.server_port),
                                     "http://127.0.0.1:" + str(foreign.server_port))
            result["installedModules"] = agent.AgentGateway.require_single_bundle(gateway)
            gateway.start()
            catalog = []
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                status, _, body = gateway.request("/v1/models", method="GET")
                require(status == 200, "domain catalog unavailable")
                catalog = json.loads(body)["data"]
                if any(row["id"].endswith("/claude-fixture") for row in catalog):
                    break
                time.sleep(0.05)
            selected = {name: next(row["id"] for row in catalog if row["id"].endswith("/" + name))
                        for name in (paired.MODEL, "claude-fixture")}
            mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
            libraries = {line.split()[-1] for line in mappings.splitlines()
                         if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
            require(any(Path(path).is_file() and digest(Path(path)) == result["librarySHA256"]
                        for path in libraries), "domain library is not mapped in the actual host")
            result["libraryMappedInHost"] = True
            accounts = json.loads(gateway.command("plugin", "sdk", "accounts", "devin-inbound", "devin", "--json"))
            scope = json.dumps({"provider": "devin", "account": accounts[0]["id"]})
            models = json.loads(gateway.command("plugin", "sdk", "call", "devin-inbound",
                                                "provider.models", "{}", "--scope", scope))["result"]["models"]
            result["models"] = [{"id": row["id"], "free": row.get("free", False), "price": row.get("price")}
                                for row in models]
            indexed = {row["id"]: row for row in result["models"]}
            require(indexed[paired.MODEL]["price"] ==
                    {"currency": "USD", "input": 1.4, "output": 4.4, "cacheRead": 0.26} and
                    indexed["fixture-free"]["free"] and indexed["fixture-free"]["price"] == {"currency": "USD"} and
                    indexed["claude-fixture"]["price"] is None, "installed SDK conflated native prices/free/unknown")
            result["priceProjectionPassed"] = True

            def execute(name, messages, expected=None, model=paired.MODEL):
                if selected_case is not None and name != selected_case:
                    return
                entry = {"name": name}
                before = vendor.count()
                sampler = None
                if sample_stall:
                    def sample():
                        if gateway.process.poll() is None:
                            gateway.process.send_signal(signal.SIGQUIT)
                    sampler = threading.Timer(3, sample)
                    sampler.start()
                try:
                    status, _, body = gateway.request("/v1/chat/completions",
                        json.dumps({"model": selected[model], "stream": False, "messages": messages}).encode(),
                        headers={"Content-Type": "application/json"})
                except (OSError, http.client.HTTPException) as error:
                    entry.update(passed=False, error=type(error).__name__, dispatches=vendor.count() - before)
                    result["cases"].append(entry)
                    result["failures"].append(name + ": " + type(error).__name__)
                    if gateway.process.poll() is None:
                        gateway.process.send_signal(signal.SIGQUIT)
                        gateway.process.wait(timeout=5)
                    raise
                finally:
                    if sampler is not None:
                        sampler.cancel()
                        sampler.join(timeout=5)
                after = vendor.count()
                entry.update(status=status, dispatches=after - before)
                if after == before + 1:
                    native = fields(vendor.chat[-1])
                    prompts = [fields(value) for value in native.get(3, [])]
                    entry["nativeMessages"] = [{str(key): [
                        value.decode(errors="replace") if isinstance(value, bytes) else value for value in values]
                        for key, values in prompt.items()} for prompt in prompts]
                try:
                    if expected is None:
                        require(status >= 400 and after == before, "invalid domain history reached vendor")
                    else:
                        require(status == 200 and after == before + 1 and
                                json.loads(body)["choices"][0]["message"]["content"] == "domain answer",
                                "valid domain history failed before native completion")
                        expected(prompts)
                    entry["passed"] = True
                except (AssertionError, KeyError, ValueError, IndexError) as error:
                    entry.update(passed=False, error=str(error))
                    result["failures"].append(name + ": " + str(error))
                result["cases"].append(entry)
                (artifacts / "observations.json").write_text(json.dumps(result, indent=2) + "\n")

            execute("canonical-simple-control", [{"role": "user", "content": "ordinary-canonical"}],
                    lambda prompts: require(len(prompts) == 1, "simple control history changed"))
            split = [{"role": "user", "content": "read both"},
                     {"role": "assistant", "content": None, "reasoning": "First a.",
                      "tool_calls": [call("a")]},
                     {"role": "assistant", "content": "reading", "reasoning_content": "Then b.",
                      "tool_calls": [call("b")]},
                     {"role": "tool", "tool_call_id": "a", "content": "real A"},
                     {"role": "tool", "tool_call_id": "b", "content": "real B"}]

            def check_split(prompts):
                require(len(prompts) == 4 and prompts[1].get(3) == [b"reading"] and
                        prompts[1].get(11) == [b"First a.\n\nThen b."] and len(prompts[1].get(6, [])) == 2 and
                        prompts[2].get(7) == [b"a"] and prompts[2].get(3) == [b"real A"] and
                        prompts[3].get(7) == [b"b"] and prompts[3].get(3) == [b"real B"],
                        "split calls/reasoning or real result association changed")

            execute("split-calls-and-reasoning-alias", split, check_split)
            same_alias = json.loads(json.dumps(split))
            same_alias[1]["reasoning_content"] = same_alias[1].pop("reasoning")
            execute("split-calls-same-reasoning-field", same_alias, check_split)
            execute("missing-real-result-refused", same_alias[:-1])
            conflicting = [{"role": "assistant", "content": "answer",
                            "reasoning": "one", "reasoning_content": "two"}]
            execute("conflicting-reasoning-refused", conflicting)
            cached = json.loads(json.dumps(same_alias))
            cached[1]["cache_control"] = {"type": "ephemeral"}
            execute("split-cache-boundary-refused", cached)
            deferred = [{"role": "user", "content": "look"},
                        {"role": "assistant", "tool_calls": [call("c1")]},
                        {"role": "tool", "tool_call_id": "c1",
                         "content": "done\n\n[The tool returned an image; it follows in the next message.]"},
                        {"role": "user", "content": [
                            {"type": "text", "text": "[From the result of shot (tool call c1):]"},
                            image("eA=="), {"type": "text", "text": "user words"}, image("eQ==")]}]

            def check_images(prompts):
                require(len(prompts) == 4 and prompts[2].get(3) == [b"done"] and
                        prompts[2].get(7) == [b"c1"] and
                        fields(prompts[2][10][0]).get(1) == [b"eA=="] and len(prompts[2][10]) == 1 and
                        prompts[3].get(3) == [b"user words"] and len(prompts[3][10]) == 1 and
                        fields(prompts[3][10][0]).get(1) == [b"eQ=="],
                        "tool image or user-owned image/text crossed owners")

            execute("deferred-tool-images-preserve-user", deferred, check_images)
            incomplete = json.loads(json.dumps(deferred))
            incomplete[2]["content"] = "[The tool returned 2 images; they follow in the next message.]"
            execute("incomplete-deferred-images-refused", incomplete)

            def check_claude(prompts):
                require(len(prompts) == 3 and not any(key in prompts[1] for key in (11, 12, 18)),
                        "Claude received unsigned reasoning or an invented signature")

            execute("claude-unsigned-thinking-suppressed",
                    [{"role": "user", "content": "hello"},
                     {"role": "assistant", "content": "answer", "reasoning_content": "unsigned secret"},
                     {"role": "user", "content": "next"}], check_claude, "claude-fixture")
            if selected_case in (None, "usage-ledger-counted-and-zero", "execution-facts-ledger"):
                rich = selected_case == "execution-facts-ledger"
                entry = {"name": selected_case if rich else "usage-ledger-counted-and-zero", "ledger": []}
                response_ids = {}
                before = vendor.count()
                for marker, prompt, output in (("usage-counted", 23, 3), ("usage-zero", 0, 0)):
                    status, _, body = gateway.request("/v1/chat/completions",
                        json.dumps({"model": selected[paired.MODEL], "stream": False,
                                    "messages": [{"role": "user", "content": marker}]}).encode(),
                        headers={"Content-Type": "application/json", "X-Magpie-Session": marker})
                    require(status == 200, "usage execution failed")
                    usage = json.loads(body)["usage"]
                    response_ids[marker] = json.loads(body)["id"]
                    require(usage["prompt_tokens"] == prompt and usage["completion_tokens"] == output,
                            "canonical Chat usage lost its inclusive prompt count")
                ledger = gateway.config / "usage.jsonl"
                deadline = time.monotonic() + 5
                rows = []
                while time.monotonic() < deadline:
                    rows = [json.loads(line) for line in ledger.read_text().splitlines()] if ledger.exists() else []
                    rows = [row for row in rows if row.get("session") in ("usage-counted", "usage-zero")]
                    if len(rows) == 2:
                        break
                    time.sleep(0.05)
                entry["ledger"] = [{key: row[key] for key in
                    ("provider", "model", "in", "out", "cache_read", "cache_write", "status", "ep",
                     "session", "callerKeyId", "response_id", "rid", "served", "upstream", "credits",
                     "reported_cost", "stop", "source", "cost", "own_cost", "tries")
                    if key in row} for row in rows]
                entry["dispatches"] = vendor.count() - before
                result["cases"].append(entry)
                (artifacts / "observations.json").write_text(json.dumps(result, indent=2) + "\n")
                require(len(rows) == 2 and entry["dispatches"] == 2, "usage log or actual dispatch count differs")
                counted = next(row for row in rows if row["session"] == "usage-counted")
                zero = next(row for row in rows if row["session"] == "usage-zero")
                require(all(row.get("status") == 200 and row.get("ep") == "/v1/chat/completions"
                            and row.get("callerKeyId") and row.get("response_id") for row in rows),
                        "request log lost endpoint, caller or response attribution")
                require([counted.get(key) for key in ("in", "out", "cache_read", "cache_write")] == [7, 3, 11, 5],
                        "usage log double-counts native cached prompt tokens")
                require([zero.get(key) for key in ("in", "out", "cache_read", "cache_write")] == [0, 0, 0, 0],
                        "usage log dropped explicit zero token/cache counts")
                require(all("own_cost" not in row for row in rows), "native ACU cost became USD")
                if rich:
                    for row in rows:
                        require(row.get("served") == paired.MODEL + "-served" and
                                row.get("upstream") == "API_PROVIDER_ANTHROPIC" and
                                row.get("rid") == "upstream-" + row["session"] and
                                row.get("response_id") == response_ids[row["session"]] and
                                row.get("credits") == (2 if row["session"] == "usage-counted" else 0) and
                                row.get("reported_cost") == {"amount": 2.5, "currency": "ACU"} and
                                row.get("stop") == "STOP_PATTERN",
                                "execution facts lost measured identity, credits, stop or ACU units")
                    log = (artifacts / "gateway.log").read_text()
                    require(paired.SELECTED_KEY not in log and CALLER_KEY not in log,
                            "host diagnostic log contains fixture credentials")
                    diagnostics = []
                    for line in log.splitlines():
                        if "extension diagnostic:" in line and "code=devin.execute." in line:
                            diagnostics.append({key: value for key, value in
                                (part.split("=", 1) for part in line.split() if "=" in part)
                                if key in ("level", "code", "phase", "status", "retryable")})
                    entry["diagnostics"] = diagnostics
                    require(sum(row.get("code") == "devin.execute.accepted" for row in diagnostics) == 2 and
                            sum(row.get("code") == "devin.execute.completed" and row.get("status") == "200"
                                for row in diagnostics) == 2,
                            "installed host lost scoped execution phase/status diagnostics")
                entry["passed"] = True
            if selected_case in (None, "acu-aside-status"):
                entry = {"name": "acu-aside-status", "windows": []}
                before = vendor.count()
                for counters, expected, unknown in (({"acuConsumed": 1.5}, 30, False),
                                                     ({"acuConsumed": 0}, 0, False), ({}, 0, True)):
                    vendor.acu_fields = {"acuLimit": 5, "planEnd": "2030-01-01T00:00:00Z", **counters}
                    usage = json.loads(gateway.command("plugin", "sdk", "call", "devin-inbound",
                                                       "provider.usage", "{}", "--scope", scope))["result"]
                    acu = next(window for window in usage["windows"] if window["name"] == "ACUs")
                    entry["windows"].append(acu)
                    require(acu["used"] == expected and acu.get("unknown", False) == unknown and
                            acu["aside"] and acu["limit"] == (5 if numeric_quota else 100) and
                            acu["resetsAt"] == 1893456000000 and
                            not acu.get("notModels") and not acu.get("models"),
                            "installed ACU Aside lost absent/zero distinction, reset or nonblocking scope")
                    if numeric_quota:
                        require(acu["unit"] == "ACUs" and acu.get("amount") == counters.get("acuConsumed") and
                                ("amount" in acu) == ("acuConsumed" in counters) and
                                acu["display"] == str(counters.get("acuConsumed", "unknown")) + " / 5 ACUs",
                                "installed API9 quota lost exact count, absence, units or native limit")
                require(vendor.count() == before, "quota read dispatched inference")
                entry.update(passed=True, dispatches=0)
                result["cases"].append(entry)
            require(not proxy.requests and not foreign.snapshot() and not vendor.failures,
                    "domain fixture crossed a peer boundary or vendor assertions failed")
            require(result["cases"], "selected domain case did not run")
            require(not result["failures"], "installed domain cases failed: " + repr(result["failures"]))
            result["status"] = "passed"
    finally:
        paired.VendorHandler = prior_handler
        if "home" in locals():
            result["homeRemoved"] = not home.exists()
        if "gateway" in locals():
            result["processStopped"] = gateway.process is None or gateway.process.poll() is not None
        if "vendor" in locals():
            result["nativeRequests"] = vendor.count()
            result["vendorFailures"] = list(vendor.failures)
            result["domainArrivals"] = list(vendor.domain_arrivals)
            result["nativeCalls"] = list(vendor.calls)
        if "proxy" in locals():
            result["externalRequests"] = len(proxy.requests)
        if result["status"] == "running":
            result["status"] = "failed"
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "domain fixture retained a process or HOME")
    print(json.dumps({key: result[key] for key in
                      ("status", "nativeRequests", "priceProjectionPassed", "externalRequests",
                       "homeRemoved", "processStopped")}))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--magpie", type=Path, required=True)
    parser.add_argument("--distribution", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    parser.add_argument("--case")
    parser.add_argument("--sample-stall", action="store_true")
    options = parser.parse_args()
    run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve(),
        options.case, options.sample_stall)
