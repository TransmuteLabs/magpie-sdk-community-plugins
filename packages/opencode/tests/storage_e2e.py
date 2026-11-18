#!/usr/bin/env python3
import argparse
import errno
import hashlib
import http.client
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
from urllib.parse import urlencode

parser = argparse.ArgumentParser()
parser.add_argument("host", type=Path)
parser.add_argument("distribution", type=Path)
parser.add_argument("output", type=Path)
parser.add_argument("--history-browser", action="store_true")
parser.add_argument("--history-unsupported", action="store_true")
args = parser.parse_args()
root = Path(__file__).resolve().parents[1] / "src"
declaration = json.loads((args.distribution / "magpie-plugin.json").read_text())
output = args.output.resolve()
output.mkdir(parents=True, exist_ok=False)
home = output / "home"
config = home / ".config/magpie"
config.mkdir(parents=True)
if os.environ.get("MAGPIE_OPENCODE_LOOPBACK_GUARD") != "seccomp-broker":
    raise SystemExit("ordinary storage proof requires its loopback supervisor")
with socket.socket() as probe:
    probe.settimeout(1)
    try:
        probe.connect(("192.0.2.1", 443))
    except OSError as failure:
        assert failure.errno == errno.EPERM, "external network was not denied by the fixture"
    else:
        raise AssertionError("storage fixture reached external networking")
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    port = listener.getsockname()[1]
origin = "http://127.0.0.1:" + str(port)
env = os.environ.copy()
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / ".config"),
           XDG_CACHE_HOME=str(home / ".cache"), XDG_DATA_HOME=str(home / ".local/share"),
           XDG_STATE_HOME=str(home / ".local/state"), MAGPIE_TEST_SANDBOX=str(home),
           MAGPIE_WEB_KEY="synthetic-storage-key", MAGPIE_ADDR="127.0.0.1:0",
           MAGPIE_NO_STATS="1", MAGPIE_NO_ACCOUNTS="1", MAGPIE_PLUGIN_CATALOG="off",
           MAGPIE_PLUGIN_MARKET="off", MAGPIE_PARTNERS="off", DO_NOT_TRACK="1",
           MAGPIE_BUN=str(home / "no-bun"), MAGPIE_UPDATE_FEED=origin + "/offline-updates",
           OPENCODE_DB="")
(config / "settings.json").write_text(json.dumps({
    "noStats": True, "noUsageStats": True, "noAutoUpdate": True, "noAccountDiscovery": True,
    "agentTelemetryOff": True, "gatewayMode": "off", "sessionReads": {"asked": True, "all": False},
}))
(home / ".cache/magpie").mkdir(parents=True)
(home / ".cache/magpie/models.json").write_text("{}")
commands, calls = [], []
cookie = ""
process = None
writer = None
host_log = (output / "host.log").open("w")
writer_log = (output / "sqlite-writer.log").open("w")

def command(*arguments):
    result = subprocess.run([str(args.host.resolve()), *arguments], cwd=home, env=env,
                            text=True, capture_output=True, timeout=40)
    commands.append({"args": list(arguments), "exit": result.returncode,
                     "stdout": result.stdout, "stderr": result.stderr})
    assert result.returncode == 0, commands[-1]
    return result.stdout

def request(path, body=None, expected=200, authenticated=True, headers=None, raw=None):
    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=20)
    fields = {"Content-Type": "application/json", "Origin": origin}
    if authenticated:
        fields["Cookie"] = cookie
    fields.update(headers or {})
    payload = raw if raw is not None else None if body is None else json.dumps(body)
    connection.request("POST" if payload is not None else "GET", path, payload, fields)
    response = connection.getresponse()
    content = response.read()
    response_headers = dict(response.getheaders())
    connection.close()
    record = {"path": path, "status": response.status, "bytes": len(content)}
    if "application/json" in response_headers.get("Content-Type", ""):
        record["json"] = json.loads(content)
    calls.append(record)
    assert response.status == expected, (path, response.status, content[:1500])
    return content, response_headers

def ask_writer(value):
    writer.stdin.write(json.dumps(value) + "\n")
    writer.stdin.flush()
    line = writer.stdout.readline()
    assert line, "separate SQLite writer exited"
    return json.loads(line)

def prepare_sources():
    deadline = time.monotonic() + 20
    while True:
        content, _ = request("/api/sessions/sources")
        state = json.loads(content)
        assert state["errors"] == [], state
        if not state["pending"]:
            return
        assert time.monotonic() < deadline, "ordinary source preparation stayed pending"
        time.sleep(0.05)

try:
    writer = subprocess.Popen([sys.executable, str(root.parent / "tests/storage_writer.py"),
                               str(home / ".local/share/opencode")], cwd=home, env=env,
                              text=True, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=writer_log)
    ready = json.loads(writer.stdout.readline())
    assert ready["pid"] == writer.pid and ready["mainOnly"] == 0
    command("plugin", "sdk", "add", str(args.distribution.resolve()))
    process = subprocess.Popen([str(args.host.resolve()), "web", "--no-open",
                                "--addr", "127.0.0.1:" + str(port)],
                               cwd=home, env=env, stdout=host_log, stderr=subprocess.STDOUT)
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        assert process.poll() is None, "ordinary host exited before becoming ready"
        try:
            connection = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
            connection.request("GET", "/?k=synthetic-storage-key")
            response = connection.getresponse()
            response.read()
            if response.status == 303:
                cookie = response.getheader("Set-Cookie").split(";", 1)[0]
                connection.close()
                break
            connection.close()
        except OSError:
            pass
        time.sleep(0.05)
    assert cookie, "ordinary web never became ready"
    if args.history_browser or args.history_unsupported:
        pages, _ = request("/api/plugin-pages")
        page = next(page for page in json.loads(pages) if page["name"] == "opencode")
        browser_config = output / "history-browser-config.json"
        browser_config.write_text(json.dumps({
            "origin": origin, "cookie": cookie, "page": page,
            "unsupported": args.history_unsupported, "output": str(output),
        }))
        browser_result = subprocess.run(
            ["/usr/bin/node-22", str(root.parent / "tests/history_browser.cjs"), str(browser_config)],
            cwd=home, env=env, text=True, capture_output=True, timeout=90,
        )
        (output / "history-browser.log").write_text(browser_result.stdout + browser_result.stderr)
        assert browser_result.returncode == 0, (browser_result.returncode, browser_result.stdout, browser_result.stderr)
    request("/api/sessions/storage-sources", expected=401, authenticated=False)
    request("/api/sessions/storage-sources", expected=403, headers={"Sec-Fetch-Site": "cross-site"})
    body, _ = request("/api/sessions/storage-sources")
    inspection = json.loads(body)
    assert len(inspection["roots"]) == 1 and inspection["consents"] == []
    selected = inspection["roots"][0]
    selection = {"plugin": selected["plugin"], "source": selected["source"],
                 "rootSlot": selected["rootSlot"], "rootRef": selected["rootRef"],
                 "expectedRevision": selected["revision"]}
    request("/api/sessions/storage-sources/bind", raw=json.dumps(selection) + " {}", expected=400)
    request("/api/sessions/storage-sources/bind", dict(selection, rootRef="foreign"), expected=409)
    request("/api/sessions/storage-sources/bind", dict(selection, expectedRevision="stale"), expected=409)
    request("/api/sessions/storage-sources/bind", selection)
    request("/api/sessions/reads", {"all": False, "agents": {"opencode": True}})
    prepare_sources()
    body, _ = request("/api/sessions/storage-sources")
    bound = json.loads(body)
    assert bound["roots"][0]["consented"] and len(bound["consents"]) == 1
    listing, _ = request("/api/sessions?limit=20")
    listed = json.loads(listing)["sessions"]
    assert sum(row["id"] == "ses_wal" for row in listed) == 1 and all(row["id"] != "ses_leftover" for row in listed), listed
    one, _ = request("/api/sessions/one?key=opencode:ses_wal")
    assert b"Committed WAL session" in one and json.loads(one)["source_status"] == "ready"
    transcript, _ = request("/api/sessions/transcript?agent=opencode&id=ses_wal")
    assert transcript.index(b"first split part") < transcript.index(b"second split part") < transcript.index(b"inline tail")
    assert b"retired instruction" not in transcript and b"current instruction" in transcript
    assert b"media unavailable: external" in transcript
    media_path = "/api/sessions/storage-media?" + urlencode({"agent": "opencode", "session": "ses_wal", "media": ready["mediaId"]})
    media, headers = request(media_path)
    assert hashlib.sha256(media).hexdigest() == ready["mediaId"].removeprefix("media:")
    assert headers["Content-Type"] == "image/png" and headers["Cache-Control"] == "no-store"
    assert headers["X-Content-Type-Options"] == "nosniff" and "sandbox" in headers["Content-Security-Policy"]
    request("/api/sessions/storage-media?" + urlencode({"agent": "opencode", "session": "ses_v1", "media": ready["mediaId"]}), expected=409)
    request("/api/sessions/storage-media?" + urlencode({"agent": "opencode", "session": "ses_wal", "media": "../main-only.db"}), expected=409)
    after = ask_writer("state")
    assert all(ready[key] == after[key] for key in ("db", "wal", "shm")), "ordinary read wrote donor files"
    ask_writer("append")
    prepare_sources()
    one, _ = request("/api/sessions/one?key=opencode:ses_wal")
    assert b"Updated WAL session" in one
    transcript, _ = request("/api/sessions/transcript?agent=opencode&id=ses_wal")
    assert b"later committed message" in transcript
    assert ask_writer("checkpoint")["busy"] == 0, "ordinary HTTP reader retained SQLite locks"
    request("/api/sessions/transcript?agent=opencode&id=ses_wal")
    pages, _ = request("/api/plugin-pages")
    assert any(page["name"] == "opencode" for page in json.loads(pages)), "full package omitted its UI"
    catalog = json.loads((config / "extensions/catalog.json").read_text())
    assert [entry["manifest"]["id"] for entry in catalog] == ["opencode"]
    assert not list(home.rglob("node_modules"))
    assert all(not (config / name).exists() for name in ("plugins.json", "plugin-providers.json", "migrations.json"))
    consent = bound["consents"][0]
    request("/api/sessions/storage-sources/unbind", {"agent": "opencode", "expectedIdentity": "foreign"}, expected=409)
    request("/api/sessions/storage-sources/unbind", {"agent": "opencode", "expectedIdentity": consent["identity"]})
    request("/api/sessions/storage-sources/bind", selection)
    command("plugin", "sdk", "off", "opencode")
    request(media_path, expected=409)
    listing, _ = request("/api/sessions?limit=20")
    assert b"ses_wal" not in listing, "foreign CLI off retained a storage row or selected the built-in"
    report = {
        "ordinaryHost": True, "nativeFullPackage": True, "actualModuleSQLiteReader": True,
        "actualOpenCodeBinary": False, "renderedStorageConsentUI": args.history_browser,
        "oldHostHistoryUnsupported": args.history_unsupported,
        "installedModules": ["opencode"], "moduleVersion": declaration["version"],
        "sdkMinimum": ".".join(str(declaration["contract"]["version"][key]) for key in ("major", "minor")),
        "stockLibrarySHA256": hashlib.sha256((args.distribution / declaration["entry"]).read_bytes()).hexdigest(),
        "separateWriterPID": writer.pid,
        "hostPID": process.pid, "committedWALVisible": True, "mainOnlySessionCount": ready["mainOnly"],
        "splitPartsBeforeInline": True, "instructionEpochApplied": True, "selectedBlobHydrated": True,
        "donorDBWALSHMUnchanged": True, "foreignBlobAndRawPathDenied": True,
        "ownerCookieAndOriginChecked": True, "consentRevisionAndRootChecked": True,
        "laterWALCommitObserved": True, "checkpointLocksReleased": True, "foreignOffPublicationDenied": True,
        "liveAccountsOrVendors": False, "network": "seccomp loopback only",
        "output": str(output),
    }
    (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report), flush=True)
finally:
    if process is not None and process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
    if writer is not None and writer.poll() is None:
        writer.stdin.close()
        writer.wait(timeout=5)
    host_log.close()
    writer_log.close()
    (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
    (output / "http.json").write_text(json.dumps(calls, indent=2) + "\n")
