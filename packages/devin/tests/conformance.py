"""Synthetic SDK/native ABI and real local-install checks; no vendor traffic."""

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import ctypes
import hashlib
import json
import os
from pathlib import Path
import queue
import struct
import subprocess
import tempfile


MAX_FRAME = 24 << 20
CHAT = bytes.fromhex("00000000141a0910011a0568656c6c6f720767726f75702f6d")
CHAT_HEADERS = {"Content-Type": ["application/connect+proto"], "Connect-Protocol-Version": ["1"]}
CATALOG_HEADERS = {"Content-Type": ["application/proto"], "Connect-Protocol-Version": ["1"]}
EMIT = ctypes.CFUNCTYPE(ctypes.c_int32, ctypes.c_size_t, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t)


class Descriptor(ctypes.Structure):
    _fields_ = [
        ("abi_version", ctypes.c_uint32), ("struct_size", ctypes.c_uint32),
        ("host", ctypes.c_size_t), ("emit", EMIT),
    ]


def b64(body):
    return base64.b64encode(body).decode()


def varint(value):
    output = bytearray()
    while value > 127:
        output.append((value & 127) | 128)
        value >>= 7
    output.append(value)
    return bytes(output)


def text_field(number, value):
    value = value.encode() if isinstance(value, str) else value
    return varint(number << 3 | 2) + varint(len(value)) + value


def number_field(number, value):
    return varint(number << 3) + varint(value)


def data_frame(body):
    return b"\0" + struct.pack(">I", len(body)) + body


def read_varint(body, position):
    value = shift = 0
    while position < len(body) and shift < 70:
        byte = body[position]
        position += 1
        value |= (byte & 127) << shift
        if byte < 128:
            return value, position
        shift += 7
    raise AssertionError("invalid synthetic protobuf")


def fields(body):
    output = {}
    position = 0
    while position < len(body):
        tag, position = read_varint(body, position)
        number, wire = tag >> 3, tag & 7
        if wire == 0:
            value, position = read_varint(body, position)
        elif wire == 2:
            size, position = read_varint(body, position)
            value = body[position:position + size]
            position += size
            if len(value) != size:
                raise AssertionError("truncated synthetic protobuf")
        else:
            raise AssertionError("unexpected native fixture wire type")
        output.setdefault(number, []).append(value)
    return output


def connect_frames(body):
    output = []
    while body:
        if len(body) < 5:
            raise AssertionError("truncated native Connect header")
        flag, size = body[0], struct.unpack(">I", body[1:5])[0]
        value, body = body[5:5 + size], body[5 + size:]
        if len(value) != size:
            raise AssertionError("truncated native Connect data")
        output.append((flag, value))
    return output


class Session:
    def __init__(self, library, module_id):
        self.library, self.module_id = library, module_id
        self.messages = queue.Queue()
        self.callback_errors = []
        self.next = 0
        self.closed = False
        self.emit = EMIT(self.receive)
        descriptor = Descriptor(1, ctypes.sizeof(Descriptor), 17, self.emit)
        handle = ctypes.c_uint64()
        if library.magpie_extension_open_v1(ctypes.byref(descriptor), ctypes.byref(handle)):
            raise AssertionError("native open failed")
        self.handle = handle.value
        descriptor.host, descriptor.emit = 0, EMIT()

    def receive(self, opaque, data, length):
        try:
            if self.closed or opaque != 17 or length > MAX_FRAME + 1:
                return 1
            self.messages.put(json.loads(ctypes.string_at(data, length)))
            return 0
        except Exception as error:
            self.callback_errors.append(str(error))
            return 1

    def send(self, frame):
        body = json.dumps(frame, separators=(",", ":")).encode() + b"\n"
        buffer = ctypes.create_string_buffer(body)
        status = self.library.magpie_extension_send_v1(
            self.handle, ctypes.cast(buffer, ctypes.POINTER(ctypes.c_uint8)), len(body)
        )
        ctypes.memset(buffer, 0x21, len(body))
        if status:
            raise AssertionError("native send failed")

    def call(self, method, params, allow_error=False):
        self.next += 1
        identifier = str(self.next)
        self.send({
            "kind": "call", "id": identifier, "method": method, "params": params,
            "scope": {"plugin": self.module_id, "session": "synthetic"},
        })
        events = []
        while True:
            frame = self.messages.get(timeout=10)
            if frame["id"] != identifier:
                raise AssertionError("native call IDs crossed")
            if frame["kind"] == "event":
                events.append(frame)
                self.send({"kind": "ack", "id": identifier, "seq": frame["seq"]})
                continue
            if frame["kind"] != "result" or self.callback_errors:
                raise AssertionError("invalid native terminal")
            if frame.get("error") and not allow_error:
                raise AssertionError(frame["error"])
            return frame, events

    def close(self):
        if not self.closed:
            self.closed = True
            if self.library.magpie_extension_close_v1(self.handle):
                raise AssertionError("native close failed")


def append_events(result, events):
    for event in events:
        if event["event"] == "headers":
            result["status"] = event["data"]["status"]
            result["headers"] = event["data"]["headers"]
        elif event["event"] == "data":
            result["body"] += base64.b64decode(event["data"])
        elif event["event"] != "flush":
            raise AssertionError("unexpected codec event")


def native_response(session, adapter, response_id, canonical, content_type):
    result = {"status": 0, "headers": {}, "body": b""}
    operations = [{"action": "start", "status": 200, "headers": {"Content-Type": [content_type]}}]
    operations += [{"action": "write", "body": b64(canonical[index:index + 23])} for index in range(0, len(canonical), 23)]
    operations.append({"action": "finish"})
    for operation in operations:
        _, events = session.call("inbound.response", {"id": adapter, "responseId": response_id, **operation})
        append_events(result, events)
    return result


def native_checks(distribution, manifest):
    library = ctypes.CDLL(str(distribution / manifest["entry"]))
    library.magpie_extension_open_v1.argtypes = [ctypes.POINTER(Descriptor), ctypes.POINTER(ctypes.c_uint64)]
    library.magpie_extension_send_v1.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    library.magpie_extension_close_v1.argtypes = [ctypes.c_uint64]
    for name in ("open", "send", "close"):
        getattr(library, "magpie_extension_" + name + "_v1").restype = ctypes.c_int32
    session = Session(library, manifest["id"])
    checks = {}
    try:
        described, _ = session.call("sdk.describe", None)
        expected = {key: value for key, value in manifest.items() if key not in {"runtime", "entry", "args", "compatibility"}}
        if described["result"] != expected:
            raise AssertionError("public native handshake differs from package")
        session.call("component.configure", {"component": "primary", "contract": {}, "instance": "conformance",
                                           "config": {"version": "0", "values": {}}})
        decoded, _ = session.call("inbound.decode", {"id": "devin", "responseId": "chat", "headers": CHAT_HEADERS, "body": b64(CHAT)})
        canonical = json.loads(base64.b64decode(decoded["result"]["body"]))
        if decoded["result"]["protocol"] != "chat" or canonical["model"] != "group/m" or canonical["messages"][0]["content"] != "hello":
            raise AssertionError("native chat fixture changed during decoding")
        system = number_field(2, 5) + text_field(3, "fixture system") + number_field(4, 12) + number_field(5, 1)
        user = number_field(2, 1) + text_field(3, "inspect")
        call = text_field(1, "screen-1") + text_field(2, "screenshot") + text_field(3, "{}")
        assistant = number_field(2, 2) + text_field(6, call)
        image = text_field(1, "aW1hZ2U=") + text_field(2, "image/png") + text_field(3, "")
        tool = number_field(2, 4) + text_field(3, "screen") + text_field(7, "screen-1") + text_field(10, image)
        payload = text_field(14, "group/m") + b"".join(text_field(3, prompt) for prompt in (system, user, assistant, tool))
        decoded, _ = session.call("inbound.decode", {
            "id": "devin", "responseId": "recovered-prompts", "headers": CHAT_HEADERS, "body": b64(data_frame(payload)),
        })
        prompts = json.loads(base64.b64decode(decoded["result"]["body"]))["messages"]
        if [prompt["role"] for prompt in prompts] != ["system", "user", "assistant", "tool"]:
            raise AssertionError("native system prompt or tool-result association changed")
        if prompts[0]["content"] != "fixture system" or prompts[3]["tool_call_id"] != "screen-1":
            raise AssertionError("native recovered prompt content changed")
        if prompts[3]["content"][1]["image_url"]["url"] != "data:image/png;base64,aW1hZ2U=":
            raise AssertionError("native tool-result image was dropped")
        checks["recovered_system_and_tool_image"] = True
        body = (
            b'data: {"id":"fixture","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}\n\n'
            b'data: {"id":"fixture","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\n'
            b'data: [DONE]\n\n'
        )
        response = native_response(session, "devin", "chat", body, "text/event-stream")
        frames = connect_frames(response["body"])
        messages = [fields(value) for flag, value in frames if flag == 0]
        if response["status"] != 200 or b"".join(part for message in messages for part in message.get(3, [])) != b"hello" or not any(2 in message.get(5, []) for message in messages):
            raise AssertionError("native Connect text or stop-reason fixture changed")
        if frames[-1][0] != 2 or "error" in json.loads(frames[-1][1]):
            raise AssertionError("native Connect success did not terminate correctly")
        checks["connect_chat"] = True
        models = b'{"object":"list","data":[{"object":"model","id":"group/visible","display_name":"Visible"}]}'
        for kind in ("cli-models", "command-models", "cascade-models",
                     "cli-models-site", "command-models-site", "cascade-models-site", "model-statuses"):
            adapter = "devin." + kind
            decoded, _ = session.call("inbound.decode", {"id": adapter, "responseId": kind, "headers": CATALOG_HEADERS})
            if decoded["result"]["protocol"] != "models" or decoded["result"].get("body"):
                raise AssertionError("native discovery bypassed the Models operation")
            response = native_response(session, adapter, kind, models, "application/json")
            if response["status"] != 200 or response["headers"].get("Content-Type") != ["application/proto"]:
                raise AssertionError("native discovery was not unary protobuf")
            if kind == "model-statuses":
                if response["body"]:
                    raise AssertionError("native statuses invented availability")
            else:
                entries = fields(response["body"]).get(1, [])
                if [fields(entry).get(22, [b""])[0] for entry in entries] != [b"group/visible"]:
                    raise AssertionError("native catalog fixture changed model identity")
            checks[kind] = True
        for kind, team_field in (("cli-models-site", 3), ("command-models-site", 2), ("cascade-models-site", 7)):
            rejected, _ = session.call("inbound.decode", {
                "id": "devin." + kind, "responseId": "team-" + kind, "headers": CATALOG_HEADERS,
                "body": b64(text_field(team_field, "private-team-sentinel")),
            }, allow_error=True)
            error = rejected.get("error", {})
            if error.get("code") != "invalid_params" or "sentinel" in error.get("message", ""):
                raise AssertionError("native site team was accepted or leaked")
        checks["refuses_native_team_selection"] = True
        for number, name, value in ((3, "max_newlines", 400), (7, "top_k", 40)):
            prompt = number_field(2, 1) + text_field(3, "hello")
            payload = text_field(14, "group/m") + text_field(3, prompt) + text_field(8, number_field(number, value))
            refused, _ = session.call("inbound.decode", {"id": "devin", "responseId": name, "headers": CHAT_HEADERS, "body": b64(data_frame(payload))}, allow_error=True)
            if name not in refused.get("error", {}).get("message", ""):
                raise AssertionError("unsupported native constraint was silently dropped: " + name)
            checks["refuses_" + name] = True
        expired, _ = session.call("inbound.decode", {"id": "devin", "responseId": "deadline",
            "headers": {**CHAT_HEADERS, "Connect-Timeout-Ms": ["0"]}, "body": b64(CHAT)}, allow_error=True)
        if not expired.get("error"):
            raise AssertionError("expired native request was accepted")
        checks["deadline_error"] = expired["error"]
        checks["deadline_status_preserved"] = expired["error"]["code"] == "http.504"
        session.call("component.stop", None)
    finally:
        session.close()
    return checks


def installed_checks(magpie, distribution, manifest):
    with tempfile.TemporaryDirectory(prefix="magpie-devin-install-") as temporary:
        home = Path(temporary)
        env = {key: os.environ[key] for key in ("PATH", "TMPDIR", "LANG", "LC_ALL", "TZ", "LD_LIBRARY_PATH") if key in os.environ}
        env.update(HOME=str(home), XDG_CONFIG_HOME=str(home / ".config"), XDG_CACHE_HOME=str(home / ".cache"),
                   XDG_DATA_HOME=str(home / ".local/share"), XDG_STATE_HOME=str(home / ".local/state"),
                   MAGPIE_BUN=str(home / "bun-does-not-exist"), GOPROXY="off", GOSUMDB="off")

        def run(*arguments):
            result = subprocess.run([str(magpie), "plugin", "sdk", *arguments], env=env, capture_output=True, text=True, timeout=45)
            if result.returncode:
                raise AssertionError({"command": arguments[:2], "exit": result.returncode, "stderr": result.stderr[-2000:]})
            return result.stdout

        run("add", str(distribution))
        installed = json.loads(run("inspect", manifest["id"]))
        if installed["manifest"]["runtime"] != "native" or installed["manifest"]["id"] != manifest["id"]:
            raise AssertionError("CLI did not install the native package")
        params = {"id": "devin", "responseId": "installed", "headers": CHAT_HEADERS, "body": b64(CHAT)}
        output = [json.loads(line) for line in run("call", manifest["id"], "inbound.decode", json.dumps(params)).splitlines()]
        terminal = next(frame["result"] for frame in output if frame["kind"] == "result")
        if terminal["protocol"] != "chat" or json.loads(base64.b64decode(terminal["body"]))["model"] != "group/m":
            raise AssertionError("installed native codec changed the fixture")
        return {"installed": manifest["id"], "runtime": "native", "decode_fixture": True, "bun_present": False}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("distribution", type=Path)
    parser.add_argument("--magpie", required=True, type=Path)
    parser.add_argument("--result", type=Path)
    args = parser.parse_args()
    distribution = args.distribution.resolve()
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    with ThreadPoolExecutor(max_workers=2) as pool:
        sessions = [pool.submit(native_checks, distribution, manifest) for _ in range(2)]
        native = [session.result() for session in sessions]
    if native[0] != native[1]:
        raise AssertionError("concurrent native sessions returned different fixture results")
    result = {
        "native": native[0],
        "parallel_native_sessions": len(native),
        "cli": installed_checks(args.magpie.resolve(), distribution, manifest),
        "library_sha256": hashlib.sha256((distribution / manifest["entry"]).read_bytes()).hexdigest(),
    }
    if args.result:
        args.result.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
    if not result["native"]["deadline_status_preserved"]:
        raise SystemExit("SDK lost native HTTP 504: expected http.504")


if __name__ == "__main__":
    main()
