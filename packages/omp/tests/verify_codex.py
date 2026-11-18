"""Check a semantic-fixture export with an installed Codex, offline on usbox.

Run inside a private user/network namespace with only loopback enabled.
The optional single continuation uses a loopback mock, never a vendor.
"""

import argparse
import gzip
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import tempfile
import threading
import time


def require(condition, name):
    if not condition:
        raise AssertionError(name)


def verify(arguments):
    root = arguments.output.resolve()
    root.mkdir(parents=True, exist_ok=True)
    routes = json.loads(subprocess.check_output(
        ["/usr/sbin/ip", "-j", "route", "show", "default"], text=True))
    routes += json.loads(subprocess.check_output(
        ["/usr/sbin/ip", "-6", "-j", "route", "show", "default"], text=True))
    require(not routes, "preflight: network namespace must have no default route")
    parent_namespace = os.environ.get("OMP_CODEX_PARENT_NETNS")
    require(parent_namespace and os.readlink("/proc/self/ns/net") != parent_namespace,
            "preflight: capture OMP_CODEX_PARENT_NETNS before entering the network namespace")
    launcher = arguments.codex.resolve(strict=True)
    command = [str(launcher)]
    if launcher.suffix == ".js":
        node = shutil.which("node")
        require(node is not None, "preflight: installed JavaScript launcher needs existing node")
        command.insert(0, node)
    home = Path(tempfile.mkdtemp(prefix="codex-home-", dir=root))
    codex = home / "codex"
    codex.mkdir()
    environment = dict(HOME=str(home), CODEX_HOME=str(codex), PATH="/usr/bin:/bin",
                       LANG="C.UTF-8", NO_PROXY="127.0.0.1,localhost",
                       RUST_LOG="codex_rollout=trace")
    version = subprocess.check_output(command + ["--version"], env=environment,
                                      cwd=home, text=True, timeout=15).strip()
    require(version == "codex-cli " + arguments.expected_version,
            "preflight: unreviewed Codex version: " + version)
    identifier = (arguments.export / "id.txt").read_text().strip()
    session_root = codex / "sessions/2026/11/14"
    session_root.mkdir(parents=True)
    rollout = session_root / ("rollout-2026-11-14T12-00-00-" + identifier + ".jsonl")
    shutil.copyfile(arguments.export / "rollout.jsonl", rollout)
    requests = []

    class Mock(BaseHTTPRequestHandler):
        def log_message(self, *values):
            pass

        def do_POST(self):
            body = self.rfile.read(int(self.headers["Content-Length"]))
            encoding = self.headers.get("Content-Encoding")
            if encoding == "gzip":
                body = gzip.decompress(body)
            elif encoding == "zstd":
                body = subprocess.run(["/usr/bin/zstd", "-d", "-c"], input=body,
                                      capture_output=True, check=True).stdout
            else:
                require(encoding in (None, "identity"), "mock: unknown body compression")
            request = json.loads(body)
            requests.append(request)
            (root / ("request-" + str(len(requests)) + ".json")).write_text(json.dumps(request, indent=2))
            events = [
                {"type": "response.created", "response": {"id": "offline-final"}},
                {"type": "response.output_item.done", "item": {
                    "type": "message", "id": "offline-message", "role": "assistant",
                    "phase": "final_answer", "content": [
                        {"type": "output_text", "text": "OFFLINE_RESUME_ACCEPTED"}]}},
                {"type": "response.completed", "response": {
                    "id": "offline-final", "usage": {
                        "input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
            ]
            payload = "".join("event: " + event["type"] + "\ndata: " +
                              json.dumps(event) + "\n\n" for event in events).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Mock)
    serving = threading.Thread(target=server.serve_forever, daemon=True)
    serving.start()
    (codex / "config.toml").write_text(
        'model = "offline-model"\nmodel_provider = "offline"\n'
        '[features]\nresponses_websockets_v2 = false\n'
        '[model_providers.offline]\nname = "offline"\n'
        'base_url = "http://127.0.0.1:' + str(server.server_port) + '/v1"\n'
        'wire_api = "responses"\nrequires_openai_auth = false\nsupports_websockets = false\n')
    stderr = (root / "app-server-stderr.log").open("w")
    process = subprocess.Popen(command + ["app-server", "--listen", "stdio://"],
                               cwd=home, env=environment, stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, stderr=stderr, bufsize=0)
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ)
    buffer = b""
    observations = []

    def send(value):
        process.stdin.write(json.dumps(value).encode() + b"\n")
        process.stdin.flush()

    def receive():
        nonlocal buffer
        deadline = time.monotonic() + 45
        while b"\n" not in buffer:
            require(time.monotonic() < deadline, "app-server: bounded response timeout")
            if not selector.select(1):
                continue
            part = os.read(process.stdout.fileno(), 65536)
            require(bool(part), "app-server: unexpected exit")
            buffer += part
        line, buffer = buffer.split(b"\n", 1)
        value = json.loads(line)
        observations.append(value)
        require(not (value.get("method") and "id" in value),
                "replay: historical tool requested execution")
        return value

    def call(number, method, parameters):
        send(dict(id=number, method=method, params=parameters))
        while True:
            value = receive()
            if value.get("id") == number:
                require("error" not in value, method + ": " + str(value.get("error")))
                (root / (str(number) + "-" + method.replace("/", "-") + ".json")).write_text(
                    json.dumps(value, indent=2))
                return value["result"]

    def check_history(thread):
        turns = thread["turns"]
        items = [item for turn in turns for item in turn["items"]]
        users = [item for item in items if item["type"] == "userMessage"]
        require(len(users) == 1, "artifact: tool results/compaction must not become user turns")
        require(len(turns) == 1 and turns[0]["status"] == "completed",
                "turns: one explicit finished turn required")
        require("Recorded tool" not in json.dumps(items), "artifact: fabricated tool prose")
        require([item["phase"] for item in items if item["type"] == "agentMessage"] ==
                ["commentary", "final_answer"], "phases: commentary/final_answer lost")
        tools = [item for item in items if item["type"] in ("dynamicToolCall", "commandExecution")]
        require(len(tools) == 2, "tools: native durable tool cards missing")
        require(tools[0]["type"] == "dynamicToolCall" and tools[0]["tool"] == "read" and
                tools[0]["status"] == "completed", "tools: foreign history identity changed")
        require(tools[1]["type"] == "commandExecution" and tools[1]["status"] == "failed" and
                tools[1]["exitCode"] is None, "tools: failure status or unknown exit code changed")
        require(any(part["type"] == "image" for part in users[0]["content"]),
                "images: user image missing")
        require(any(part["type"] == "inputImage" for part in tools[0]["contentItems"]),
                "images: tool result image was not retained inside its result")
        require(any(item["type"] == "contextCompaction" for item in items),
                "compaction: native boundary missing")

    try:
        initialized = call(1, "initialize", dict(
            clientInfo=dict(name="omp-offline-acceptance", version="1"),
            capabilities=dict(experimentalApi=True)))
        require("/" + arguments.expected_version in initialized["userAgent"],
                "preflight: app-server runtime version differs")
        send(dict(method="initialized", params={}))
        initial = call(2, "thread/read", dict(threadId=identifier, includeTurns=True))["thread"]
        resumed = call(3, "thread/resume", dict(
            threadId=identifier, path=str(rollout), cwd=str(home), model="offline-model",
            modelProvider="offline", approvalPolicy="never", sandbox="read-only"))["thread"]
        check_history(resumed)
        materialized = call(4, "thread/read", dict(threadId=identifier, includeTurns=True))["thread"]
        check_history(materialized)
        require(initial["historyMode"] == "paginated" and initial["turns"] == [],
                "materialization: expected empty pre-resume paginated view")
        if arguments.continuation:
            continuation_start = len(observations)
            call(5, "turn/start", dict(threadId=identifier, input=[
                dict(type="text", text="Continue offline fixture only.", text_elements=[])]))
            while True:
                value = receive()
                if value.get("method") == "turn/completed":
                    require(value["params"]["turn"]["status"] == "completed",
                            "continuation: mock turn failed")
                    break
            require(not any(
                value.get("method") in ("item/started", "item/completed") and
                value.get("params", {}).get("item", {}).get("type") in
                ("commandExecution", "dynamicToolCall")
                for value in observations[continuation_start:]),
                "replay: historical command/tool executed during continuation")
            require(len(requests) == 1, "continuation: expected exactly one mock request")
            history = requests[0]["input"]
            calls = [item for item in history if item["type"] == "function_call"]
            outputs = [item for item in history if item["type"] == "function_call_output"]
            require([item["name"] for item in calls] == ["read", "exec_command"],
                    "continuation: historical tool order changed")
            require([item["call_id"] for item in calls] == [item["call_id"] for item in outputs],
                    "continuation: result identities changed")
            encoded = json.dumps(history)
            for required in ("Retained compact context", "source file contents", "shell failed",
                             "Selected final answer."):
                require(required in encoded, "continuation: lost " + required)
            for private in ("Recorded tool", "PRIVATE_ABANDONED_BRANCH", "Old answer must not return",
                            "private signed reasoning"):
                require(private not in encoded, "continuation: leaked " + private)
            require(not any(tool.get("name") == "read" for tool in requests[0].get("tools", [])),
                    "replay: historical foreign tool registered for execution")
        else:
            require(not requests, "resume-only: unexpected model request")
        result = dict(version=version, exportPath=arguments.export.resolve().as_posix(),
                      preResumeTurns=0, resumedTurns=1, materializedReadTurns=1,
                      nativeToolCards=2, mockContinuation=arguments.continuation,
                      mockRequests=len(requests), historicalToolsNotReplayed=True)
        (root / "acceptance.json").write_text(json.dumps(result, indent=2))
        print(json.dumps(result, indent=2))
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        selector.close()
        process.stdin.close()
        process.stdout.close()
        server.shutdown()
        server.server_close()
        serving.join(timeout=5)
        stderr.close()
        (root / "messages.json").write_text(json.dumps(observations, indent=2))
        (root / "home.txt").write_text(str(home))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--codex", type=Path, required=True)
    parser.add_argument("--expected-version", default="0.160.0")
    parser.add_argument("--export", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--continuation", action="store_true")
    verify(parser.parse_args())
