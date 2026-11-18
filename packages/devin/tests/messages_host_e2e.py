"""Prove the own Messages domain path in an ordinary isolated installed host."""

import argparse
from contextlib import ExitStack
import gzip
import json
from pathlib import Path
import secrets
import tempfile
import time

import agent_http_e2e as agent
import donor_host_e2e as donor
import paired_http_e2e as paired
from conformance import connect_frames, data_frame, fields, number_field, text_field
from http_e2e import CALLER_KEY, DenyProxy, Provider, SERVICE, digest, require, server_thread


class MessagesVendorHandler(donor.DomainVendorHandler):
    def do_POST(self):
        if self.path != SERVICE + "GetChatMessage":
            super().do_POST()
            return
        try:
            size = int(self.headers.get("Content-Length", "0"))
            require(0 < size <= 16 << 20, "invalid Messages native request size")
            frames = connect_frames(self.rfile.read(size))
            require(len(frames) == 1 and frames[0][0] in (0, 1), "invalid Messages native framing")
            payload = gzip.decompress(frames[0][1]) if frames[0][0] else frames[0][1]
            request = fields(payload)
            require(request.get(21) == [b"claude-fixture"] and
                    self.headers.get("Authorization") == "Basic " + paired.SELECTED_KEY and
                    paired.SELECTED_KEY.encode() in request[1][0] and CALLER_KEY.encode() not in payload,
                    "Messages lost selected account/model authority")
            with self.server.lock:
                self.server.calls.append(self.path)
                self.server.chat.append(payload)
            prompts = [fields(value) for value in request.get(3, [])]
            text = b"".join(prompt.get(3, [b""])[0] for prompt in prompts)
            thought = text_field(9, "native thought") + text_field(16, "thought-1")
            signature = (text_field(10, "messages-signed") + text_field(16, "thought-1")
                         + text_field(21, "anthropic-bedrock") + text_field(23, "claude-fixture"))
            if b"messages-cancel" in text:
                self.send_response(200)
                self.send_header("Content-Type", "application/connect+proto")
                self.send_header("Connection", "close")
                self.end_headers()
                signature = (text_field(10, "cancel-only-sig") + text_field(16, "thought-1")
                             + text_field(21, "anthropic-bedrock") + text_field(23, "claude-fixture"))
                try:
                    self.wfile.write(data_frame(thought) + data_frame(signature))
                    self.wfile.flush()
                    while not self.server.stop.wait(0.05):
                        self.wfile.write(data_frame(text_field(3, ".")))
                        self.wfile.flush()
                except (OSError, ConnectionError):
                    self.server.cancelled["disconnect"].set()
                finally:
                    self.close_connection = True
                return
            if b"messages-unknown-output" in text:
                self.reply("application/connect+proto",
                           data_frame(thought) + data_frame(signature) + data_frame(number_field(24, 1))
                           + b"\2\0\0\0\2{}")
                return
            usage = b"".join(number_field(index, value) for index, value in ((2, 7), (3, 3), (4, 5), (5, 11)))
            end = text_field(3, "Messages answer") + number_field(5, 2) + text_field(7, usage)
            self.reply("application/connect+proto",
                       data_frame(thought) + data_frame(signature) + data_frame(end) + b"\2\0\0\0\2{}")
        except BaseException as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.reply("application/json", b'{"code":"internal"}', 500)


def run(host, distribution, artifacts):
    require(not artifacts.exists(), "Messages proof requires a fresh destination")
    artifacts.mkdir(parents=True)
    result = {"status": "running", "checks": [], "hostSHA256": digest(host),
              "librarySHA256": digest(distribution / "plugin.so"),
              "runnerSHA256": digest(Path(__file__)), "hostSourceEdits": 0,
              "fixtureRetries": 0, "bootstrapAcceptance": "not-exercised"}
    prior_handler = paired.VendorHandler
    paired.VendorHandler = MessagesVendorHandler
    paired.SELECTED_KEY = "messages-fixture-" + secrets.token_hex(16)
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, paired.Vendor())
            stack.callback(vendor.stop.set)
            foreign = server_thread(stack, Provider())
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-messages-home-")))
            gateway = paired.PairedGateway(host, distribution, home, artifacts,
                                           "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)
            gateway.configure_paired("http://127.0.0.1:" + str(vendor.server_port),
                                     "http://127.0.0.1:" + str(foreign.server_port))
            result["installedModules"] = agent.AgentGateway.require_single_bundle(gateway)
            gateway.start()
            deadline = time.monotonic() + 15
            model = None
            while time.monotonic() < deadline:
                status, _, body = gateway.request("/v1/models", method="GET")
                require(status == 200, "Messages catalog unavailable")
                model = next((row["id"] for row in json.loads(body)["data"]
                              if row["id"].endswith("/claude-fixture")), None)
                if model:
                    break
                time.sleep(0.05)
            require(model, "Messages model was not discovered")
            mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
            libraries = {line.split()[-1] for line in mappings.splitlines()
                         if line.split()[-1].startswith(str(home)) and ".so" in line.split()[-1]}
            require(any(Path(path).is_file() and digest(Path(path)) == result["librarySHA256"]
                        for path in libraries), "Messages library not mapped in actual host")
            result["libraryMappedInHost"] = True

            def request(body, key=CALLER_KEY, marker="messages-proof"):
                return gateway.request("/v1/messages", json.dumps({"model": model, "max_tokens": 64, **body}).encode(),
                                       key=key, headers={"Content-Type": "application/json",
                                                         "X-Magpie-Session": marker})

            blocks = [{"type": "text", "text": "prefix-A"},
                      {"type": "text", "text": "messages-cache-prefix", "cache_control": {"type": "ephemeral"}}]
            status, _, body = request({"system": [{"type": "text", "text": "system-A"},
                                                  {"type": "text", "text": "system-B",
                                                   "cache_control": {"type": "ephemeral"}}],
                                      "messages": [{"role": "user", "content": blocks}]})
            result["firstStatus"] = status
            result["firstReply"] = json.loads(body)
            require(status == 200 and vendor.count() == 1, "Messages failed before its first native completion")
            reply = json.loads(body)
            require(reply["type"] == "message" and reply["stop_reason"] == "end_turn" and
                    reply["content"] == [{"type": "thinking", "thinking": "native thought", "signature": "messages-signed"},
                                          {"type": "text", "text": "Messages answer"}] and
                    reply["usage"] == {"input_tokens": 7, "output_tokens": 3,
                                       "cache_creation_input_tokens": 5, "cache_read_input_tokens": 11},
                    "Messages response lost signatures or exclusive usage")
            prompts = [fields(value) for value in fields(vendor.chat[-1]).get(3, [])]
            result["firstPrompts"] = [{"role": prompt.get(2), "text": prompt.get(3),
                                       "cache": bool(prompt.get(8))} for prompt in prompts]
            require([prompt.get(3, [b""])[0] for prompt in prompts] ==
                    [b"system-A", b"system-B", b"prefix-A", b"messages-cache-prefix"] and
                    [bool(prompt.get(8)) for prompt in prompts] == [False, True, False, True],
                    "host/provider flattened Messages blocks or moved cache boundaries")
            result["checks"].append("ordinary JSON preserves system/user cache blocks, thinking/signature and exclusive usage")

            history = [{"role": "user", "content": "hello"},
                       {"role": "assistant", "content": reply["content"]}, {"role": "user", "content": "continue"}]
            status, _, _ = request({"messages": history})
            require(status == 200 and vendor.count() == 2, "scoped signed Messages history did not resume")
            prompts = [fields(value) for value in fields(vendor.chat[-1]).get(3, [])]
            signed = [prompt for prompt in prompts if prompt.get(12)]
            require(len(signed) == 1 and signed[0].get(12) == [b"messages-signed"] and
                    signed[0].get(18) == [b"anthropic-bedrock"] and signed[0].get(22) == [b"claude-fixture"],
                    "Messages signature format/actual model was guessed or discarded")
            result["checks"].append("signed history reuses persisted native format/actual model under the same caller/account")
            before = vendor.count()
            status, _, _ = request({"messages": history}, key="other-fixture-caller")
            require(status >= 400 and vendor.count() == before, "another caller replayed signed Messages history")
            forged = [{"role": "assistant", "content": [{"type": "thinking", "thinking": "foreign",
                                                        "signature": "foreign-signature"}]},
                      {"role": "user", "content": "continue"}]
            status, _, _ = request({"messages": forged})
            require(status >= 400 and vendor.count() == before, "unissued Messages signature reached the vendor")
            result["checks"].append("foreign caller/unissued signatures refused without native dispatch")

            status, _, body = request({"stream": True, "messages": [{"role": "user", "content": "messages-stream"}]})
            require(status == 200 and b"event: message_start\n" in body and
                    b"event: content_block_delta\n" in body and b"event: message_stop\n" in body and
                    b'"signature":"messages-signed"' in body and b"[DONE]" not in body,
                    "Messages stream did not preserve typed events")
            result["checks"].append("ordinary streaming uses typed Messages events and real native terminal")

            status, _, body = request({"stream": True, "messages": [{"role": "user", "content": "messages-unknown-output"}]})
            require(status == 200 and b"event: error\n" in body and b"event: message_stop\n" not in body,
                    "unknown native output fabricated a successful Messages terminal")
            result["checks"].append("unknown native output remains a failed stream")

            connection, response, transport = gateway.connect("/v1/messages",
                json.dumps({"model": model, "max_tokens": 64, "stream": True,
                            "messages": [{"role": "user", "content": "messages-cancel"}]}).encode(),
                headers={"Content-Type": "application/json"})
            try:
                require(response.status == 200 and response.readline().startswith(b"event: message_start"),
                        "Messages cancellation fixture did not start")
                transport.shutdown(2)
            finally:
                response.close()
                connection.close()
            require(vendor.cancelled["disconnect"].wait(5), "Messages disconnect did not cancel native HTTP")
            before = vendor.count()
            cancelled = [{"role": "assistant", "content": [{"type": "thinking", "thinking": "native thought",
                                                           "signature": "cancel-only-sig"}]},
                         {"role": "user", "content": "continue"}]
            status, _, _ = request({"messages": cancelled})
            require(status >= 400 and vendor.count() == before, "cancelled Messages stream issued a signature receipt")
            result["checks"].append("caller disconnect cancels native HTTP and issues no resumable signature")
            require(not proxy.requests and not foreign.snapshot() and not vendor.failures,
                    "Messages fixture crossed a peer boundary or failed vendor assertions")
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
        if "proxy" in locals():
            result["externalRequests"] = len(proxy.requests)
        if result["status"] == "running":
            result["status"] = "failed"
        (artifacts / "result.json").write_text(json.dumps(result, indent=2, default=lambda value: value.decode()) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "Messages fixture retained HOME/process")
    print(json.dumps({key: result[key] for key in ("status", "nativeRequests", "externalRequests", "homeRemoved", "processStopped")}))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--magpie", type=Path, required=True)
    parser.add_argument("--distribution", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    options = parser.parse_args()
    run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve())
