"""Actual Front MCP account-binding regression; isolated guarded usbox only."""

import argparse
from contextlib import ExitStack
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile


specification = importlib.util.spec_from_file_location(
    "minimax_tools_policy_fixture", Path(__file__).with_name("policy_host.py"))
policy = importlib.util.module_from_spec(specification)
specification.loader.exec_module(policy)
fixture = policy.fixture
require = fixture.require
CATALOG = "/minimax-cloud/api/v1/connectors/connected-tools/list"
SEARCH = "/mavis/api/v1/mcp/web_search"
PATH = "/mcp/magpie/tools"
RESTRICTED = "synthetic-minimax-restricted"
READONLY = "synthetic-minimax-readonly"


def validate(schema, value):
    """Validate the complete keyword subset used by list_tools/web_search."""
    require(set(schema) <= {"type", "properties", "required", "additionalProperties",
                            "minLength", "minimum", "enum", "description"},
            "published schema needs a fixture validator extension")
    kind = schema.get("type")
    require(kind in ("object", "string", "integer"), "unsupported fixture schema type")
    if kind == "object":
        require(isinstance(value, dict), "schema requires an object")
        properties = schema.get("properties", {})
        require(all(key in value for key in schema.get("required", [])),
                "schema required field missing")
        require(schema.get("additionalProperties") is not False or set(value) <= set(properties),
                "published schema rejects argument: " + ",".join(sorted(set(value) - set(properties))))
        for key in value:
            if key in properties:
                validate(properties[key], value[key])
    elif kind == "string":
        require(isinstance(value, str) and len(value) >= schema.get("minLength", 0),
                "schema string constraint failed")
    else:
        require(type(value) is int and value >= schema.get("minimum", value),
                "schema integer constraint failed")
    require("enum" not in schema or value in schema["enum"], "schema enum rejected argument")


class ToolsEndpoint(policy.PolicyEndpoint):
    def dispatch(self, path, value, region):
        if path == SEARCH:
            require(value == {"query": "synthetic front search", "count": 1},
                    "host metadata or routing assertion reached upstream search")
            self.reply({"organic": [{"title": "Synthetic", "url": "https://example.invalid/front"}]})
            return
        if path == CATALOG:
            require("_magpie" not in value and "account" not in value and "site" not in value,
                    "host metadata reached upstream catalog")
        super().dispatch(path, value, region)


fixture.EndpointHandler = ToolsEndpoint


def rpc(host, method, params=None, key=fixture.CALLER):
    status, _, body = host.request(PATH, {
        "jsonrpc": "2.0", "id": "synthetic-front", "method": method, "params": params or {},
    }, key=key)
    require(status in (200, 401, 403), "unexpected Front HTTP status")
    return status, json.loads(body)


def run(host_binary, worker, output):
    require(not output.exists(), "evidence directory must be new")
    output.mkdir()
    distribution = output / "exec-test-package"
    (distribution / "bin").mkdir(parents=True)
    shutil.copy2(worker, distribution / "bin/minimax")
    declaration = subprocess.check_output([str(worker), "--manifest"], timeout=10)
    (distribution / "magpie-plugin.json").write_bytes(declaration)
    fixture.copy_window_assets(distribution)
    report = {
        "test": "TestInstalledMiniMaxMCPAccountSelectionMatchesPublishedSchema",
        "runtime": "actual Front HTTP MCP, installed actual handlers in exec harness",
        "hostSHA256": fixture.sha256(host_binary), "workerSHA256": fixture.sha256(worker),
        "runnerSHA256": fixture.sha256(__file__), "cases": [],
    }
    try:
        with ExitStack() as stack:
            home = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="mm-front-")))
            ca, context = fixture.certificate(output, {"HOME": str(home), "PATH": "/usr/bin:/bin"})
            shutil.copy2(ca, distribution / "bin/fixture-ca.pem")
            proxy = fixture.Fixture(context)
            proxy.mode, proxy.primary, proxy.devices, proxy.tokens = "front", None, {}, {}
            proxy.success_format = "negotiated"
            proxy.start(stack)
            host = policy.PolicyHost(host_binary, distribution, home, output,
                "http://127.0.0.1:" + str(proxy.server_port), ca)
            stack.callback(host.close)
            host.install()
            accounts = policy.sign_in(host)
            host.command("gateway-key", "add", "SurfaceRestricted", "--key", RESTRICTED)
            keys = host.command("gateway-key", "list").splitlines()
            restricted_id = next(row.split()[0] for row in keys
                                 if len(row.split()) > 1 and row.split()[1] == "SurfaceRestricted")
            host.command("gateway-key", "models", restricted_id, "synthetic/model")
            host.command("gateway-key", "add", "Read-only Front fixture", "--key", READONLY, "--read-only")
            host.start()
            status, listing = rpc(host, "tools/list")
            require(status == 200 and "error" not in listing, "Front tools/list failed")
            tools = listing["result"]["tools"]
            selected = {}
            for name in ("list_tools", "web_search"):
                matches = [tool for tool in tools if tool["name"].endswith("_" + name)]
                require(len(matches) == 1, "Front did not publish exactly one MiniMax " + name)
                selected[name] = matches[0]
            (output / "published-tools.json").write_text(json.dumps(tools, indent=2) + "\n")

            def call(case, name, arguments, success, sends, identity=None, key=fixture.CALLER):
                descriptor = selected[name]
                validate(descriptor["inputSchema"], arguments)
                before = len(proxy.snapshot())
                status, response = rpc(host, "tools/call", {
                    "name": descriptor["name"], "arguments": arguments}, key)
                records = proxy.snapshot()[before:]
                dispatches = [row for row in records if row["path"] in (CATALOG, SEARCH)]
                expected_token = "Bearer synthetic-policy-access-" + identity if identity else None
                selected_only = all(row["headers"].get("authorization") == expected_token for row in dispatches)
                result = response.get("result", {})
                accepted = status == 200 and "error" not in response and not result.get("isError", False)
                observed = {"case": case, "httpStatus": status, "success": accepted,
                            "sends": len(dispatches), "selectedOnly": selected_only,
                            "schemaValid": True}
                report["cases"].append(observed)
                require(accepted == success, "Front outcome changed: " + case)
                require(len(dispatches) == sends and selected_only,
                        "Front selected-account dispatch fence failed: " + case)
                return response

            call("unbound-no-auto-first", "list_tools", {}, False, 0)
            call("assertions-do-not-select", "list_tools",
                 {"site": fixture.GLOBAL, "account": accounts[0]}, False, 0)
            selection = {"provider": fixture.GLOBAL, "account": accounts[0]}
            arguments = {"_magpie": selection}
            try:
                validate(selected["list_tools"]["inputSchema"], arguments)
            except AssertionError as error:
                raise AssertionError("SURFACE-1: schema-valid Front calls cannot select a saved account: " + str(error))
            call("selected-A-catalog-B-untouched", "list_tools", arguments, True, 1, "alice")
            search = {"query": "synthetic front search", "count": 1, "_magpie": selection,
                      "site": fixture.GLOBAL, "account": accounts[0]}
            call("selected-A-search-B-untouched", "web_search", search, True, 1, "alice")
            call("mismatched-assertion", "list_tools",
                 {**arguments, "account": accounts[1]}, False, 0)
            call("restricted-key", "list_tools", arguments, False, 0, key=RESTRICTED)
            call("readonly-key", "list_tools", arguments, False, 0, key=READONLY)
            call("unknown-stale-account", "list_tools",
                 {"_magpie": {"provider": fixture.GLOBAL, "account": "stale-synthetic-account"}}, False, 0)
            host.command("plugin", "sdk", "off", fixture.MODULE)
            call("disabled-old-published-name", "list_tools", arguments, False, 0)
            status, disabled = rpc(host, "tools/list")
            require(status == 200 and not disabled["result"]["tools"], "disabled package still publishes tools")
            host.command("plugin", "sdk", "on", fixture.MODULE)
            host.command("plugin", "sdk", "auth", "logout", fixture.MODULE, fixture.GLOBAL, accounts[0])
            call("logged-out-stale-selection", "list_tools", arguments, False, 0)
            call("unbound-with-only-B", "list_tools", {}, False, 0)
            call("explicit-B-control", "list_tools",
                 {"_magpie": {"provider": fixture.GLOBAL, "account": accounts[1]}}, True, 1, "bob")
            require(not proxy.failures, "synthetic endpoint assertions failed")
        require(not proxy.failures, "synthetic endpoint teardown failed")
        report["status"] = "passed"
    except Exception as error:
        report["status"], report["failure"] = "failed", str(error)
        raise
    finally:
        (output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report), flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", type=Path, required=True)
    parser.add_argument("--worker", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    options = parser.parse_args()
    run(options.host.resolve(), options.worker.resolve(), options.output.resolve())
