"""Verify standalone distributions through the ordinary Magpie SDK CLI."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def canonical_manifest(manifest):
    value = json.loads(json.dumps(manifest))
    for provider in value.get("providers", []):
        for option in provider.get("options", []):
            if option.get("default") is False:
                option.pop("default")
    return value


def verify(binary, distribution):
    manifest = json.loads((distribution / "magpie-plugin.json").read_text())
    identifier = manifest["id"]
    with tempfile.TemporaryDirectory(prefix="magpie-standalone-install-") as temporary:
        home = Path(temporary)
        environment = {
            "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
            "LANG": "C.UTF-8",
            "HOME": temporary,
            "USERPROFILE": temporary,
            "XDG_CONFIG_HOME": str(home / "config"),
            "XDG_CACHE_HOME": str(home / "cache"),
            "XDG_DATA_HOME": str(home / "data"),
            "XDG_STATE_HOME": str(home / "state"),
            "APPDATA": str(home / "appdata"),
            "LOCALAPPDATA": str(home / "localappdata"),
        }

        def cli(*arguments):
            result = subprocess.run(
                [str(binary), "plugin", "sdk", *arguments],
                env=environment,
                capture_output=True,
                text=True,
                timeout=120,
            )
            require(
                result.returncode == 0,
                f"SDK command {arguments} failed:\n{result.stdout}{result.stderr}",
            )
            return result.stdout

        require(json.loads(cli("list", "--json")) in ([], None), "Catalog was not empty")
        cli("install", str(distribution))
        entries = json.loads(cli("list", "--json"))
        require(len(entries) == 1, "Standalone installation added another SDK module")
        entry = json.loads(cli("inspect", identifier))
        require(entry["manifest"] == canonical_manifest(manifest), "Installed manifest changed")
        require(not entry["disabled"], "Installed module is disabled")
        require(manifest["runtime"] == "native", "Provider is not an in-process native module")
        installed = [
            item.parent
            for item in home.rglob("magpie-plugin.json")
            if json.loads(item.read_text()).get("id") == identifier
        ]
        require(len(installed) == 1, "Cannot identify the standalone installed package")
        inventory = {
            item.relative_to(distribution).as_posix(): hashlib.sha256(item.read_bytes()).hexdigest()
            for item in distribution.rglob("*")
            if item.is_file()
        }
        require(entry["files"] == inventory, "Host inventory differs from the distribution")
        for name, digest in inventory.items():
            require(
                hashlib.sha256((installed[0] / name).read_bytes()).hexdigest() == digest,
                f"Installed bytes changed: {name}",
            )
        lock = json.loads((installed[0] / "source-lock.json").read_text())
        require(lock["plugin"]["id"] == identifier, "Installed provenance names another module")
        require(lock["plugin"]["version"] == manifest["version"], "Installed provenance version changed")
        for provider in manifest.get("providers", []):
            accounts = json.loads(cli("accounts", identifier, provider["id"], "--json"))
            require(accounts in ([], None), "Installation fabricated a provider account")
        cli("off", identifier)
        require(json.loads(cli("inspect", identifier))["disabled"], "Module did not switch off")
        cli("on", identifier)
        require(not json.loads(cli("inspect", identifier))["disabled"], "Module did not switch on")
        cli("reload", identifier)
        require(not list(home.rglob("node_modules")), "Installation required a JS dependency tree")
        for store in home.rglob("plugins.json"):
            require(json.loads(store.read_text()) in ({}, [], None), "Installation added a legacy plugin")
        cli("remove", identifier)
        require(json.loads(cli("list", "--json")) in ([], None), "Removal left another SDK module")
        return {
            "package": distribution.name,
            "id": identifier,
            "version": manifest["version"],
            "runtime": manifest["runtime"],
            "files": len(inventory),
            "providers": [provider["id"] for provider in manifest.get("providers", [])],
            "singlePackage": True,
            "installedBytesVerified": True,
            "accountCreated": False,
        }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--magpie", required=True, type=Path)
    parser.add_argument("--dist", required=True, type=Path)
    parser.add_argument("packages", nargs="+")
    arguments = parser.parse_args()
    binary = arguments.magpie.resolve(strict=True)
    directory = arguments.dist.resolve(strict=True)
    for name in arguments.packages:
        distribution = (directory / name).resolve(strict=True)
        require(distribution.parent == directory, "Package must be a direct distribution child")
        print(json.dumps(verify(binary, distribution)), flush=True)


if __name__ == "__main__":
    main()
