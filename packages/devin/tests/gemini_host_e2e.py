"""Installed full module, genuine community 0.1.41 reply, loopback-only peers."""

import argparse
from contextlib import ExitStack
import gzip
import json
from pathlib import Path
import tempfile
import time

import agent_http_e2e as agent
import paired_http_e2e as paired
from cache_keepalive_host_e2e import sampling_chunks
from conformance import connect_frames, fields
from http_e2e import CALLER_KEY, DenyProxy, Provider, SERVICE, digest, require, server_thread


UID = "gemini-4-hydrogen-medium"
CALL = "call_11646388125050983982"
FIXTURE_SHA256 = "e4ff6e3b5134fe64f3d9761a98d3ddd1af0945ea6f7946fdfe01a9389ebd4162"


def oracle():
    path = Path(__file__).resolve().parents[1] / "src/testdata/gemini-tool-reply.bin"
    require(digest(path) == FIXTURE_SHA256, "genuine donor fixture changed")
    signature, formats, calls = [], [], []
    for flags, payload in connect_frames(path.read_bytes()):
        if flags & 1:
            payload = gzip.decompress(payload)
        if flags & 2:
            require(payload == b"{}", "donor terminal changed")
            continue
        for number, _, chunk in sampling_chunks(payload):
            if number not in (6, 9, 10, 21):
                continue
            value = fields(chunk)[number][0]
            if number == 6:
                calls.append(fields(value))
            elif number == 9:
                require(not value, "genuine fixture now contains visible thinking")
            elif number == 10:
                signature.append(value)
            else:
                formats.append(value)
    signature = b"".join(signature)
    require(len(signature) == 368 and signature.startswith(b"AY89a1+u1EAPOiO7CmhB") and
            formats == [b"gemini"] and calls == [{1: [CALL.encode()], 2: [b"exec"],
                                                 3: [b'{"command":"cat calc.py"}']}],
            "actual donor signature/type/tool changed")
    return path.read_bytes(), signature


class GeminiVendorHandler(paired.VendorHandler):
    def do_POST(self):
        if self.path != SERVICE + "GetChatMessage":
            super().do_POST()
            return
        try:
            raw = self.rfile.read(int(self.headers["Content-Length"]))
            framed = connect_frames(raw)
            require(len(framed) == 1 and framed[0][0] in (0, 1), "invalid Gemini request framing")
            payload = gzip.decompress(framed[0][1]) if framed[0][0] else framed[0][1]
            request = fields(payload)
            require(request.get(21) == [UID.encode()] and
                    self.headers.get("Authorization") == "Basic " + paired.SELECTED_KEY and
                    paired.SELECTED_KEY.encode() in request[1][0] and CALLER_KEY.encode() not in payload,
                    "Gemini lost selected-account authority")
            with self.server.lock:
                self.server.chat.append(payload)
                self.server.calls.append(self.path)
            self.send_response(200)
            self.send_header("Content-Type", "application/connect+proto")
            self.send_header("Connect-Content-Encoding", "gzip")
            self.send_header("Content-Length", str(len(self.server.reply)))
            self.end_headers()
            self.wfile.write(self.server.reply)
        except BaseException as error:
            with self.server.lock:
                self.server.failures.append(str(error))
            self.reply("application/json", b'{"code":"internal"}', 500)


def assistant_from_reply(protocol, streaming, body):
    if not streaming:
        reply = json.loads(body)
        if protocol == "chat":
            require(reply["choices"][0]["finish_reason"] == "tool_calls", "Chat lost actual stop2 tool finish")
            return reply["choices"][0]["message"]
        require(reply["stop_reason"] == "tool_use", "Messages lost actual stop2 tool finish")
        return {"role": "assistant", "content": reply["content"]}
    events = [json.loads(line[6:]) for line in body.splitlines()
              if line.startswith(b"data: {")]
    if protocol == "chat":
        call = {"type": "function", "function": {"name": "", "arguments": ""}}
        finished = False
        for event in events:
            for choice in event.get("choices", []):
                finished |= choice.get("finish_reason") == "tool_calls"
                for piece in choice.get("delta", {}).get("tool_calls", []):
                    if piece.get("id"):
                        call["id"] = piece["id"]
                    for name in ("name", "arguments"):
                        call["function"][name] += piece.get("function", {}).get(name, "")
        require(finished, "streaming Chat lost tool finish")
        return {"role": "assistant", "content": "", "tool_calls": [call]}
    block, arguments, finished = None, "", False
    for event in events:
        if event["type"] == "content_block_start":
            require(event["content_block"]["type"] == "tool_use", "Gemini invented a visible Messages thinking block")
            block = event["content_block"]
        if event["type"] == "content_block_delta":
            arguments += event["delta"].get("partial_json", "")
        if event["type"] == "message_delta":
            finished |= event["delta"]["stop_reason"] == "tool_use"
    require(block and finished, "streaming Messages lost actual tool")
    block["input"] = json.loads(arguments)
    return {"role": "assistant", "content": [block]}


def run(host, distribution, artifacts, protocols=("chat", "anthropic")):
    require(not artifacts.exists(), "Gemini proof needs a fresh destination")
    artifacts.mkdir(parents=True)
    reply, signature = oracle()
    result = {"status": "running", "hostSHA256": digest(host), "librarySHA256": digest(distribution / "plugin.so"),
              "runnerSHA256": digest(Path(__file__)), "donorCommit": "191d94bf6ed95ccc4bc62d77e1dd9201a17f5918",
              "fixtureSHA256": FIXTURE_SHA256, "cases": [], "attempts": [],
              "hostSourceEdits": 0, "fixtureRetries": 0}
    prior_handler, prior_model = paired.VendorHandler, paired.MODEL
    paired.VendorHandler, paired.MODEL = GeminiVendorHandler, UID
    try:
        with ExitStack() as stack:
            vendor = server_thread(stack, paired.Vendor())
            vendor.reply = reply
            stack.callback(vendor.stop.set)
            foreign = server_thread(stack, Provider())
            proxy = server_thread(stack, DenyProxy())
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="devin-gemini-home-")))
            gateway = paired.PairedGateway(host, distribution, home, artifacts,
                                           "http://127.0.0.1:" + str(proxy.server_port))
            stack.callback(gateway.close)
            gateway.configure_paired("http://127.0.0.1:" + str(vendor.server_port),
                                     "http://127.0.0.1:" + str(foreign.server_port))
            result["installedModules"] = agent.AgentGateway.require_single_bundle(gateway)
            gateway.start()
            model = None
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                status, _, raw = gateway.request("/v1/models", method="GET")
                require(status == 200, "Gemini model catalog failed")
                model = next((item["id"] for item in json.loads(raw)["data"] if item["id"].endswith("/" + UID)), None)
                if model:
                    break
                time.sleep(0.05)
            require(model, "actual Gemini UID was not discovered")
            mappings = Path("/proc", str(gateway.process.pid), "maps").read_text()
            require(any(Path(line.split()[-1]).is_file() and digest(Path(line.split()[-1])) == result["librarySHA256"]
                        for line in mappings.splitlines() if line.split()[-1].startswith(str(home)) and
                        ".so" in line.split()[-1]), "actual host did not map the full module library")
            result["libraryMappedInHost"] = True
            for protocol in protocols:
                for streaming in (False, True):
                    path = "/v1/chat/completions" if protocol == "chat" else "/v1/messages"
                    session = "gemini-" + protocol + "-" + str(streaming)

                    def request(messages, stream=False, key=CALLER_KEY):
                        return gateway.request(path, json.dumps({"model": model, "max_tokens": 64,
                            "stream": stream, "messages": messages}).encode(), key=key,
                            headers={"Content-Type": "application/json", "X-Magpie-Session": session})

                    history = [{"role": "user", "content": "cat calc.py"}]
                    before = vendor.count()
                    status, _, body = request(history, streaming)
                    result["attempts"].append({"protocol": protocol, "stream": streaming, "status": status,
                        "dispatches": vendor.count() - before,
                        "error": body.decode(errors="replace")[:2048] if status != 200 else None})
                    require(status == 200 and vendor.count() == before + 1, "actual donor reply was rejected")
                    assistant = assistant_from_reply(protocol, streaming, body)
                    call = assistant["tool_calls"][0] if protocol == "chat" else assistant["content"][0]
                    require(call["id"] == CALL, "actual returned tool ID changed")
                    if protocol == "chat":
                        require(call["function"]["name"] == "exec" and json.loads(call["function"]["arguments"]) ==
                                {"command": "cat calc.py"}, "actual returned tool changed")
                        history += [assistant, {"role": "tool", "tool_call_id": CALL, "content": "def add(a,b): return a+b"}]
                    else:
                        require(call["name"] == "exec" and call["input"] == {"command": "cat calc.py"},
                                "actual returned Messages tool changed")
                        history += [assistant, {"role": "user", "content": [{"type": "tool_result",
                            "tool_use_id": CALL, "content": "def add(a,b): return a+b"}]}]
                    status, _, _ = request(history)
                    require(status == 200 and vendor.count() == before + 2, "actual tool result failed to continue")
                    prompts = [fields(item) for item in fields(vendor.chat[-1]).get(3, [])]
                    signed = [prompt for prompt in prompts if prompt.get(12)]
                    require(len(signed) == 1 and signed[0].get(12) == [signature] and signed[0].get(18) == [b"gemini"]
                            and signed[0].get(22) == [UID.encode()] and not signed[0].get(11) and
                            fields(signed[0][6][0]).get(1) == [CALL.encode()], "exact signature/type/model lost its tool")
                    require(any(prompt.get(2) == [4] and prompt.get(7) == [CALL.encode()] and
                                prompt.get(3) == [b"def add(a,b): return a+b"] for prompt in prompts),
                            "host lost or manufactured the real tool result")
                    result["cases"].append({"protocol": protocol, "stream": streaming,
                                            "signatureBytes": len(signature), "toolRoundtrip": True})
            require(not vendor.failures and not foreign.snapshot() and not proxy.requests, "Gemini crossed a peer boundary")
            result["status"] = "passed"
    finally:
        paired.VendorHandler, paired.MODEL = prior_handler, prior_model
        if result["status"] == "running":
            result["status"] = "failed"
        if "home" in locals():
            result["homeRemoved"] = not home.exists()
        if "gateway" in locals():
            result["processStopped"] = gateway.process is None or gateway.process.poll() is not None
        if "vendor" in locals():
            result["nativeRequests"], result["vendorFailures"] = vendor.count(), list(vendor.failures)
        if "proxy" in locals():
            result["externalRequests"] = len(proxy.requests)
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    require(result["homeRemoved"] and result["processStopped"], "Gemini proof retained HOME/process")
    print(json.dumps(result))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ("magpie", "distribution", "artifacts"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--protocol", choices=("chat", "anthropic", "both"), default="both")
    options = parser.parse_args()
    protocols = ("chat", "anthropic") if options.protocol == "both" else (options.protocol,)
    run(options.magpie.resolve(), options.distribution.resolve(), options.artifacts.resolve(), protocols)
