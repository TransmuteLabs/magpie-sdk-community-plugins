"""Synthetic Devin CLI wire client, not an implementation of the real agent."""

import base64
import http.client
import json
import os
from pathlib import Path
import sys
from urllib.parse import urlsplit
import uuid

from conformance import fields, text_field, data_frame
from http_e2e import SERVICE, prompt, require, terminal, emitted_text


def main(arguments):
    if any(value in arguments for value in ("--help", "-h", "--version", "-V", "help", "version", "auth")):
        print("synthetic-devin-wire-fixture")
        return
    if arguments[:2] == ["models", "list"]:
        print("[]")
        return
    require("--model" in arguments, "ordinary SDK launch omitted --model")
    model = arguments[arguments.index("--model") + 1]
    require(not model.startswith("magpie/"), "host picker prefix leaked into native model ID")
    server = os.environ.get("WINDSURF_API_SERVER_URL", "")
    key = os.environ.get("DEVIN_API_KEY", "")
    require(server and key, "ordinary SDK launch omitted native endpoint/key environment")
    address = urlsplit(server)
    require(address.hostname in ("127.0.0.1", "localhost", "::1"), "fixture attempted a non-loopback gateway")
    session = arguments[arguments.index("--resume") + 1] if "--resume" in arguments else "fixture-" + uuid.uuid4().hex
    require(session.startswith("fixture-") and all(char.isalnum() or char == "-" for char in session),
            "invalid synthetic session ID")
    content = arguments[arguments.index("--") + 1] if "--" in arguments else "native-signed"
    store = Path(os.environ["HOME"]) / ".fixture-devin"
    store.mkdir(exist_ok=True)
    with (store / "launches.jsonl").open("a") as output:
        output.write(json.dumps({"model": model, "session": session, "content": content}) + "\n")
    state_file = store / (session + ".json")
    history = []
    if "--resume" in arguments:
        previous = json.loads(state_file.read_text())
        require(previous["model"] == model, "resumed fixture session changed model")
        history = [base64.b64decode(value) for value in previous["history"]]
    connection = http.client.HTTPConnection(address.hostname, address.port, timeout=10)
    headers = {"Authorization": "Basic " + key + "-" + key, "Connect-Protocol-Version": "1",
               "User-Agent": "devin-cli/0.0.0-synthetic"}
    metadata = text_field(1, "chisel") + text_field(3, key) + text_field(12, "chisel")
    connection.request("POST", SERVICE + "GetCliModelConfigs", text_field(1, metadata),
                       headers | {"Content-Type": "application/proto"})
    response = connection.getresponse()
    catalog = response.read()
    offered = [fields(item).get(22) for item in fields(catalog).get(1, [])] if response.status == 200 else []
    require([model.encode()] in offered, "native model discovery did not offer launch model")
    request_messages = history + [prompt(1, content)]
    payload = text_field(1, metadata) + text_field(21, model) + text_field(28, session)
    assignment = os.environ.get("MAGPIE_FIXTURE_ASSIGNMENT")
    if assignment:
        payload += text_field(26, assignment) + text_field(16, "owned-assignment-session")
    payload += b"".join(text_field(3, item) for item in request_messages)
    chat_headers = headers | {"Content-Type": "application/connect+proto"}
    if content == "native-deadline":
        chat_headers["Connect-Timeout-Ms"] = "750"
    connection.request("POST", SERVICE + "GetChatMessage", data_frame(payload), chat_headers)
    response = connection.getresponse()
    body = response.read()
    status = response.status
    connection.close()
    if content == "native-deadline":
        terminal(body, "deadline_exceeded")
        print(json.dumps({"session": session, "terminal": "deadline_exceeded"}))
        return
    require(status == 200, "native session failed: " + repr(body[:512]))
    messages = terminal(body)
    text = emitted_text(messages)
    require(text == b"native outbound answer", "native response text changed")
    thinking = b"".join(value for item in messages for value in item.get(9, []))
    signature = b"".join(value for item in messages for value in item.get(10, []))
    formats = [value for item in messages for value in item.get(21, [])]
    actual = [value for item in messages for value in item.get(23, [])]
    gemini = b"".join(value for item in messages for value in item.get(20, []))
    extra = b""
    if signature:
        require(thinking and formats and actual, "fixture signature lacks origin fields")
        extra = text_field(11, thinking) + text_field(12, signature)
        extra += text_field(18, formats[-1]) + text_field(22, actual[-1])
        if gemini:
            extra += text_field(17, gemini)
    request_messages.append(prompt(2, text.decode(), extra))
    state_file.write_text(json.dumps({"model": model,
        "history": [base64.b64encode(item).decode() for item in request_messages]}))
    print(json.dumps({"session": session, "model": model, "text": text.decode(),
                      "resumed": "--resume" in arguments, "signed": bool(signature),
                      "historyMessages": len(request_messages)}))


if __name__ == "__main__":
    main(sys.argv[1:])
