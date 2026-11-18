#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys
import time


def audit(value):
    path = os.environ.get("ZCODE_CONTRACT_LOG")
    if path:
        with Path(path).open("a") as output:
            output.write(json.dumps(value, separators=(",", ":")) + "\n")


def adoption(mode):
    session = workspace = command = model = None
    replied = stopped = False
    reverse_id = 42 if mode == "adoption-numeric-reverse" else "rpc-reverse-1"
    request_id = "session-9:provider-runtime-headers:00000000-0000-4000-8000-000000000001"
    control = {"phase": "completedSuccess", "sessionEnded": True, "canStop": False,
               "stopState": "idle", "stopTargetKind": "unknown", "activeWorks": [],
               "lastError": None, "apiRetry": None}
    sys.stderr.write("synthetic native diagnostic stays on stderr\n")
    sys.stderr.flush()
    for raw in sys.stdin.buffer:
        if len(raw) > 2 * 1024 * 1024:
            raise ValueError("fixture input bound")
        message = json.loads(raw)
        method = message.get("method")
        audit({"kind": "request", "method": method, "id": message.get("id")})
        if method == "v4/conversation/subscribe":
            params = message["params"]
            session = params["topic"].removeprefix("conversation/")
            workspace = params["workspace"]
            assert params["clientMode"] == "desktop-continuous"
            send({"id": message["id"], "result": {"ack": {
                "subscriptionId": "subscription-1", "mode": "snapshot", "logEpoch": "epoch-1",
            }}})
            snapshot = {
                "protocolVersion": 1, "sessionId": session, "logEpoch": "epoch-1",
                "seq": 1, "revision": 1, "control": control,
            }
            if mode == "adoption-large":
                snapshot["meta"] = {"title": "中\u2028\u2029" + "x" * (15 * 1024 * 1024),
                                    "titleSource": "user"}
            send({"method": "v4/conversation/frame", "params": {
                "topic": "conversation/" + session, "subscriptionId": "subscription-1",
                "fromSeq": 0, "toSeq": 1, "sentAt": int(time.time() * 1000),
                "payload": {"kind": "snapshot", "snapshot": snapshot},
            }})
        elif method == "v4/command":
            params = message["params"]
            assert params["workspace"] == workspace and params["sessionId"] == session
            if params["type"] == "sendText":
                assert command is None, "native input was replayed"
                command = params["commandId"]
                model = params["payload"]["modelSelection"]
                assert model["providerId"] == "magpie" and params["payload"]["text"] == "fixture"
                audit({"kind": "send", "session": session, "command": command})
                send({"id": message["id"], "result": {"commandId": command, "status": "accepted",
                      "revisionAtDecision": 1, "result": {"type": "inputAccepted",
                      "delivery": "startNow", "inputId": "input-1"}}})
                if mode == "adoption-loss":
                    return
                control = dict(control, phase="running", sessionEnded=False, canStop=True,
                               stopState="stoppable", activeWorks=[{
                                   "kind": "primaryTurn", "foregroundExecutionId": "execution-1",
                                   "sourceCommandId": command, "startedAt": int(time.time() * 1000),
                               }])
                send({"method": "v4/conversation/frame", "params": {
                    "topic": "conversation/" + session, "subscriptionId": "subscription-1",
                    "fromSeq": 1, "toSeq": 2, "sentAt": int(time.time() * 1000),
                    "payload": {"kind": "deltas", "deltas": [
                        {"op": "state.updated", "patch": {"control": control}},
                    ]},
                }})
                selected_workspace = dict(workspace)
                if mode == "adoption-foreign-reverse":
                    selected_workspace["workspaceKey"] = "foreign-workspace"
                send({"id": reverse_id, "method": "interaction/requestProviderRuntimeHeaders",
                      "params": {"requestId": request_id, "sessionId": session, "turnId": "turn-1",
                                 "workspace": selected_workspace, "modelSelection": model,
                                 "providerId": "magpie", "reason": "model-request"}})
            elif params["type"] == "stop":
                assert command is not None and replied and not stopped
                assert params["payload"] == {"expectedForegroundExecutionId": "execution-1"}
                stopped = True
                audit({"kind": "stop", "command": params["commandId"]})
                send({"method": "interaction/providerRuntimeHeadersCancelled", "params": {
                    "requestId": request_id, "sessionId": session, "workspace": workspace,
                }})
                send({"id": message["id"], "result": {"commandId": params["commandId"],
                      "status": "accepted", "revisionAtDecision": 2}})
            else:
                raise ValueError("unsupported fixture command")
        elif method == "v4/commands/query":
            assert message["params"] == {"commands": [{"sessionId": session, "commandId": command}]}
            result = {"commandId": command, "status": "accepted", "revisionAtDecision": 1}
            if mode == "adoption-unknown-query":
                result = "unknown"
            state = "completedInterrupted" if stopped else (
                "running" if mode == "adoption-running" or not replied else "completedSuccess")
            identity = "foreign-reply" if mode == "adoption-foreign-reply" else message["id"]
            send({"id": identity, "result": {"results": [{
                "key": {"sessionId": session, "commandId": command}, "result": result,
                "turn": {"turnId": "turn-1", "state": state},
            }]}})
        elif method is None and message.get("id") == reverse_id:
            assert command is not None and not replied and message.get("result") == {"headersApplied": False}
            replied = True
            audit({"kind": "reverseReply", "id": message["id"], "headersApplied": False})
            if mode == "adoption-foreign-cancel":
                send({"method": "interaction/providerRuntimeHeadersCancelled", "params": {
                    "requestId": "foreign", "sessionId": session, "workspace": workspace,
                }})
        else:
            raise ValueError("unexpected or uncorrelated native frame")


def send(value, tail=False):
    body = json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()
    if not tail:
        body += b"\r\n"
    split = body.find("中".encode()) + 1
    offsets = sorted({0, 1, 7, max(8, split), len(body)})
    for start, end in zip(offsets, offsets[1:]):
        for position in range(start, end, 65536):
            sys.stdout.buffer.write(body[position:min(position + 65536, end)])
            sys.stdout.buffer.flush()


def main():
    audit({"kind": "spawn", "argv": sys.argv[1:], "cwd": os.getcwd(), "pid": os.getpid(),
           "config": os.environ.get("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE")})
    if len(sys.argv) == 3 and sys.argv[1] == "--resume":
        send({"argv": sys.argv[1:], "cwd": os.getcwd(), "pid": os.getpid(),
              "config": os.environ.get("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE")})
        return
    if sys.argv[1:] not in (["app-server", "--stdio"], ["app-server", "--stdio", "--surface", "desktop"]):
        raise ValueError("expected literal official launch mode")
    wire = json.loads(Path(__file__).with_name("wire.json").read_bytes())
    mode = os.environ.get("ZCODE_CONTRACT_CASE", "duplex")
    if mode.startswith("adoption"):
        adoption(mode)
        return
    if mode == "framing":
        sys.stderr.write("synthetic diagnostic is not a protocol frame\n")
        sys.stderr.flush()
        sys.stdout.buffer.write(b"\n \t\r\n")
        send({"id": "framing-1", "result": {"text": "中\u2028\u2029" + "x" * (15 * 1024 * 1024)}}, tail=True)
        return
    if mode == "malformed":
        sys.stdout.buffer.write(b'{"id":\n')
        sys.stdout.buffer.flush()
        return
    sent = False
    replied = False
    stopped = False
    for raw in sys.stdin.buffer:
        if len(raw) > 2 * 1024 * 1024:
            raise ValueError("fixture input bound")
        message = json.loads(raw)
        if message == wire["subscribe"]:
            send(wire["subscribeAck"])
        elif message == wire["send"]:
            if sent:
                raise ValueError("admitted input was replayed")
            sent = True
            send(wire["sendAck"])
            if mode == "loss-after-ack":
                return
            send(wire["reverse"])
        elif message == wire["reverseReply"]:
            if not sent or replied:
                raise ValueError("uncorrelated or duplicate reverse reply")
            replied = True
        elif message == wire["stop"]:
            if not sent or not replied or stopped:
                raise ValueError("reply/stop did not bypass pending execution")
            stopped = True
            send(wire["reverseCancelled"])
            send(wire["stopAck"])
        elif message == wire["query"]:
            result = wire["queryResult"]
            result["result"]["results"][0]["turn"]["state"] = "completedInterrupted" if stopped else "running"
            send(result)
        else:
            raise ValueError("unexpected request or foreign correlation")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(3)
