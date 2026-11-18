"""Native ABI agent options conformance; no client configuration or vendor calls."""

import argparse
import ctypes
import hashlib
import json
from pathlib import Path

from conformance import Descriptor, Session


def check(distribution):
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    library_path = distribution / manifest["entry"]
    library = ctypes.CDLL(str(library_path))
    library.magpie_extension_open_v1.argtypes = [ctypes.POINTER(Descriptor), ctypes.POINTER(ctypes.c_uint64)]
    library.magpie_extension_send_v1.argtypes = [ctypes.c_uint64, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t]
    library.magpie_extension_close_v1.argtypes = [ctypes.c_uint64]
    for name in ("open", "send", "close"):
        getattr(library, "magpie_extension_" + name + "_v1").restype = ctypes.c_int32
    session = Session(library, manifest["id"])
    checks = []
    try:
        session.call("component.configure", {"component": "primary", "contract": {}, "instance": "agent-options",
                                           "config": {"version": "0", "values": {}}})
        binding = {"target": "devin", "revision": "11", "connected": True,
                   "values": {"model": "group/conversation"}}
        catalog = [{"id": "judge/native", "name": "Native judge", "provider": "judge",
                    "endpoints": ["/v1/systemone", "/v1/decisions"]},
                   {"id": "group/judges", "name": "Judges", "group": True, "endpoints": ["/v1/systemone"]}]
        for action in ("connect", "set", "sync", "disconnect"):
            options = {"gateway": "http://127.0.0.1:3425", "key": "fixture-key",
                       "models": [{"value": "magpie/group/conversation", "maxOutput": 4096,
                                   "efforts": ["low", "high"], "defaultEffort": "low"}],
                       "field": "model", "value": "magpie/group/conversation", "decisionModels": catalog}
            request = {"id": "devin-native", "action": action, "binding": binding, "options": options}
            frame, events = session.call("agent.plan", request)
            plan = frame["result"]
            expected = {"revision": binding["revision"], "values": binding["values"],
                        "connected": action != "disconnect"}
            if plan.get("files") or plan.get("binding") != expected or events:
                raise AssertionError("decision catalog wrote client config or changed conversation binding")
            checks.append(action + ": decision catalog separated from conversation binding; no files")
        for name, patch in (
            ("null decision catalog", {"decisionModels": None}),
            ("conversation endpoint", {"decisionModels": [dict(catalog[0], endpoints=["/v1/chat/completions"])]}),
            ("unknown nested field", {"decisionModels": [dict(catalog[0], overrideAccount="foreign")]}),
            ("unknown root option", {"overrideAccount": "foreign"}),
        ):
            request = {"id": "devin-native", "action": "sync", "binding": binding,
                       "options": dict(options, **patch)}
            frame, events = session.call("agent.plan", request, allow_error=True)
            if frame.get("error", {}).get("code") != "invalid_params" or events or "result" in frame:
                raise AssertionError(name + " was not rejected strictly")
            checks.append(name + ": rejected")
    finally:
        session.close()
    return {"status": "passed", "librarySHA256": hashlib.sha256(library_path.read_bytes()).hexdigest(),
            "packageVersion": manifest["version"], "sdkMinimum": manifest["contract"]["version"],
            "checks": checks, "externalRequests": 0}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("distribution", type=Path)
    parser.add_argument("--result", type=Path, required=True)
    arguments = parser.parse_args()
    result = check(arguments.distribution.resolve())
    arguments.result.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
