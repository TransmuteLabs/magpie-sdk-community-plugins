import concurrent.futures
import ctypes
import json
import os
from pathlib import Path
import queue
import sys
import threading


EMIT = ctypes.CFUNCTYPE(
    ctypes.c_int32, ctypes.c_size_t,
    ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t
)


class Descriptor(ctypes.Structure):
    _fields_ = [
        ("abi_version", ctypes.c_uint32),
        ("struct_size", ctypes.c_uint32),
        ("host", ctypes.c_size_t),
        ("emit", EMIT),
    ]


class Session:
    def __init__(self, library, identity, contract, observability):
        self.library = library
        self.identity = identity
        self.contract = contract
        self.observability = observability
        self.messages = queue.Queue()
        self.errors = []
        self.lock = threading.Lock()
        self.next = 0
        self.diagnostics = []

        def emit(host, data, length):
            try:
                assert host == identity
                assert 0 < length <= 24 * 1024 * 1024
                self.messages.put(json.loads(ctypes.string_at(data, length)))
                return 0
            except BaseException as error:
                self.errors.append(str(error))
                return 1

        self.callback = EMIT(emit)
        descriptor = Descriptor(1, ctypes.sizeof(Descriptor), identity, self.callback)
        self.handle = ctypes.c_uint64()
        assert library.magpie_extension_open_v1(
            ctypes.byref(descriptor), ctypes.byref(self.handle)
        ) == 0
        assert self.handle.value != 0

    def rpc(self, method, params, account=None, credentials=None):
        with self.lock:
            self.next += 1
            identity = f"native-{self.identity}-{self.next}"
            scope = {"plugin": "zcode", "provider": "zcode", "operation": identity}
            if account:
                scope.update(account=account, generation=1, session="native-fixture")
            frame = {"kind": "call", "id": identity, "method": method,
                     "scope": scope, "params": params}
            if credentials:
                frame["auth"] = credentials
            body = json.dumps(frame).encode() + b"\n"
            buffer = ctypes.create_string_buffer(body)
            assert self.library.magpie_extension_send_v1(
                self.handle, ctypes.cast(buffer, ctypes.POINTER(ctypes.c_uint8)),
                len(body)
            ) == 0
            ctypes.memset(buffer, 0x21, len(body))
            result = self.messages.get(timeout=10)
            while result["kind"] == "host.call":
                assert self.observability
                assert result["method"] == "diagnostics.emit" and result["parent"] == identity, result
                assert result["scope"] == scope, result
                diagnostic = result["params"]
                assert set(diagnostic) <= {"level", "code", "phase", "status", "millis", "count", "retryable"}
                assert diagnostic["level"] in {"debug", "info", "warn", "error"}
                assert diagnostic["code"] and not any(
                    text in json.dumps(diagnostic) for text in ["synthetic-key", "credentials", "body"])
                self.diagnostics.append(diagnostic)
                reply = json.dumps({"kind": "host.result", "id": result["id"], "parent": identity,
                                    "result": {"recorded": True}}).encode() + b"\n"
                response_buffer = ctypes.create_string_buffer(reply)
                assert self.library.magpie_extension_send_v1(
                    self.handle, ctypes.cast(response_buffer, ctypes.POINTER(ctypes.c_uint8)), len(reply)
                ) == 0
                ctypes.memset(response_buffer, 0x21, len(reply))
                result = self.messages.get(timeout=10)
            assert result["kind"] == "result", result
            assert result["id"] == identity, result
            assert not self.errors, self.errors
            return result

    def close(self):
        if self.handle.value:
            assert self.library.magpie_extension_close_v1(self.handle) == 0
            self.handle.value = 0


def exercise(session):
    described = session.rpc("sdk.describe", {})
    assert "error" not in described, described
    assert described["result"]["id"] == "zcode"
    assert described["result"]["contract"] == session.contract
    assert "runtime" not in described["result"]
    configured = session.rpc("component.configure", {
        "component": "primary", "instance": f"native-instance-{session.identity}",
        "config": {"version": "1", "values": {}},
    })
    assert "error" not in configured, configured
    credentials = {"site": "zai", "kind": "legacy-key", "key": "synthetic-key"}
    imported = session.rpc("auth.finish", {"input": credentials})
    assert "error" not in imported, imported
    account = imported["result"]["id"]
    listed = session.rpc("provider.models", {}, account, credentials)
    assert "error" not in listed, listed
    assert len(listed["result"]["models"]) == 4
    refused = session.rpc("provider.execute", {
        "protocol": "messages", "model": "GLM-5.3-OffPeak",
        "body": "e30=",
    }, account, credentials)
    assert refused["error"]["code"] == "not_ready", refused
    assert refused["error"]["outcome"] == "not-started", refused
    session.close()
    expected_diagnostics = 3 if session.observability else 0
    assert len(session.diagnostics) == expected_diagnostics
    return {"calls": 5, "pid": os.getpid(), "nativeSession": session.identity,
            "missingAdmissionRefused": True, "noNetworkCallbacks": True,
            "scopedCodeOnlyDiagnostics": len(session.diagnostics)}


def main():
    distribution = Path(sys.argv[1]).resolve()
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    library = ctypes.CDLL(str(distribution / manifest["entry"]))
    library.magpie_extension_open_v1.argtypes = [
        ctypes.POINTER(Descriptor), ctypes.POINTER(ctypes.c_uint64)
    ]
    library.magpie_extension_send_v1.argtypes = [
        ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t
    ]
    library.magpie_extension_close_v1.argtypes = [ctypes.c_uint64]
    for name in ["open", "send", "close"]:
        getattr(library, f"magpie_extension_{name}_v1").restype = ctypes.c_int32
    sessions = [Session(library, identity, manifest["contract"], bool(manifest.get("window"))) for identity in [17, 23]]
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(exercise, sessions))
    finally:
        for session in sessions:
            session.close()
    print(json.dumps({"passed": True, "sessions": results, "hostPID": os.getpid()}))


if __name__ == "__main__":
    main()
