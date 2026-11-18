"""Unmodified recovered Rust application startup through ordinary native SDK agent wiring."""

import argparse
from contextlib import ExitStack
import json
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time

from agent_http_e2e import AgentGateway, OwnedVendorHandler
import paired_http_e2e as paired
from http_e2e import DenyProxy, digest, require, server_thread


def run(host, distribution, agent, network_guard, artifacts):
    artifacts.mkdir(parents=True, exist_ok=True)
    paired.SELECTED_KEY = "owned-startup-vendor-" + secrets.token_hex(16)
    paired.VendorHandler = OwnedVendorHandler
    with ExitStack() as stack:
        vendor = server_thread(stack, paired.Vendor())
        stack.callback(vendor.stop.set)
        proxy = server_thread(stack, DenyProxy())
        home = Path(tempfile.mkdtemp(prefix="recovered-devin-home-"))
        stack.callback(shutil.rmtree, home)
        gateway = AgentGateway(host, distribution, home, artifacts,
                               "http://127.0.0.1:" + str(proxy.server_port))
        stack.callback(gateway.close)
        binary = home / "bin/devin"
        binary.parent.mkdir()
        guarded = home / "bin/recovered-devin"
        shutil.copyfile(agent, guarded)
        guarded.chmod(0o700)
        guard_log = artifacts / "network-guard.log"
        binary.write_text('#!/bin/sh\nexport LD_PRELOAD="' + str(network_guard) +
            '"\nexport DEVIN_TEST_NET_GUARD_LOG="' + str(guard_log) +
            '"\nexec "' + str(guarded) + '" "$@"\n')
        binary.chmod(0o700)
        gateway.env["PATH"] = str(binary.parent) + ":/usr/bin:/bin"
        gateway.env["SHELL"] = "/bin/bash"
        for name in (".bashrc", ".zshrc"):
            (home / name).write_text("# isolated reconstructed app fixture\n")
        credentials = home / ".local/share/devin/credentials.toml"
        credentials.parent.mkdir(parents=True)
        credentials.write_text('windsurf_api_key = "magpie-devin-native"\n')
        trust = credentials.parent / "cli/trusted_workspaces.json"
        trust.parent.mkdir()
        trust.write_text(json.dumps({"trusted_paths": [str(home)]}))
        gateway.configure_agent("http://127.0.0.1:" + str(vendor.server_port))
        gateway.require_single_bundle()
        gateway.start()
        model = None
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            status, _, body = gateway.request("/v1/models", method="GET")
            require(status == 200, "host model discovery failed")
            model = next((item["id"] for item in json.loads(body)["data"]
                          if item["id"].endswith("/" + paired.MODEL)), None)
            if model:
                break
            time.sleep(0.1)
        require(model is not None, "selected SDK model did not become ready")
        gateway.web_action("/api/set", {"Agent": "devin-native", "Field": "model", "Value": "magpie/" + model})
        gateway.env["DEVIN_SERVER_URL"] = "http://127.0.0.1:" + str(gateway.port)
        process = subprocess.run([str(host), "devin-native", "run", "--permission-mode", "dangerous",
            "--respect-workspace-trust", "false", "-p", "owned native startup fixture"],
            env=gateway.env, cwd=home, capture_output=True, text=True, timeout=45)
        (artifacts / "application.stdout").write_text(process.stdout)
        (artifacts / "application.stderr").write_text(process.stderr)
        require(process.returncode == 0, "recovered application startup failed: " + process.stderr[-2000:])
        require("native outbound answer" in process.stdout,
                "recovered application never completed its first native turn")
        require(not guard_log.exists() or not guard_log.read_text(),
                "recovered application attempted a non-loopback connection")
        require(not proxy.requests and not vendor.failures, "startup escaped fixture ownership")
        require(credentials.read_text() == 'windsurf_api_key = "magpie-devin-native"\n',
                "startup changed synthetic account credentials")
        installed_modules = gateway.require_single_bundle()
        with vendor.lock:
            methods = list(vendor.calls)
        require("/exa.seat_management_pb.SeatManagementService/GetUserStatus" in methods and vendor.count() >= 1,
                "startup did not traverse native preflight and inference")
        result = {"status": "passed", "client": "unmodified recovered Rust reconstruction, not original Cognition binary",
                  "agentSHA256": digest(agent), "hostSHA256": digest(host),
                  "librarySHA256": digest(distribution / "plugin.so"),
                  "networkGuardSHA256": digest(network_guard), "nativeRequests": vendor.count(),
                  "methods": methods, "externalRequests": len(proxy.requests),
                  "sourceRedirect": "DEVIN_SERVER_URL is the reconstruction's existing test redirect",
                  "installedModules": installed_modules, "legacyProviderPlugins": [], "helperPackages": [],
                  "preseeded": "synthetic local credentials and sandbox-only workspace trust; no cached account status or session"}
    result["homeRemoved"] = not home.exists()
    result["processStopped"] = gateway.process.poll() is not None
    require(result["homeRemoved"] and result["processStopped"], "startup fixture leaked its host or home")
    (artifacts / "result.json").write_text(json.dumps(result, indent=2))
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ("magpie", "distribution", "agent", "network-guard", "artifacts"):
        parser.add_argument("--" + name, required=True, type=Path)
    options = parser.parse_args()
    run(options.magpie.resolve(), options.distribution.resolve(), options.agent.resolve(),
        options.network_guard.resolve(), options.artifacts.resolve())
