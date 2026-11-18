#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import zipfile


def require(condition, message):
    if not condition:
        raise SystemExit(message)


def main():
    parser = argparse.ArgumentParser(description="Validate the actual standalone native package.")
    parser.add_argument("archive", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    with zipfile.ZipFile(args.archive) as archive:
        names = archive.namelist()
        require(len(names) == len(set(names)), "Duplicate archive entries")
        for name in names:
            path = PurePosixPath(name)
            require(not path.is_absolute() and ".." not in path.parts, "Nonlocal archive entry")
        inventory = json.loads(archive.read("inventory.json"))
        require(set(inventory) | {"inventory.json"} == set(names), "Uninventoried or missing artifact")
        for name, digest in inventory.items():
            require(hashlib.sha256(archive.read(name)).hexdigest() == digest, "Artifact digest differs: " + name)
        manifest = json.loads(archive.read("magpie-plugin.json"))
        metadata = json.loads(archive.read("package.json"))
        lock = json.loads(archive.read("source-lock.json"))
        require(archive.read("source-lock.json") == (root / "docs/source-lock.json").read_bytes(), "Packaged source lock differs")
        require(manifest["id"] == lock["plugin"]["id"], "Module identity differs")
        require(manifest["version"] == metadata["version"] == lock["plugin"]["version"], "Module versions differ")
        require(manifest["compatibility"]["nativeABI"] == lock["plugin"]["nativeABI"] == 1, "Native ABI differs")
        require(manifest["compatibility"]["hostAPI"] == "^" + lock["plugin"]["sdkMinimum"], "SDK floor differs")
        signatures = {
            ".so": (b"\x7fELF",),
            ".dylib": (b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf"),
            ".dll": (b"MZ",),
        }
        native = archive.read(manifest["entry"])
        require(manifest["runtime"] == "native"
                and any(native.startswith(signature) for signature in signatures.get(PurePosixPath(manifest["entry"]).suffix, ())),
                "Missing native controller for declared platform")
        require(set(manifest["contract"]["capabilities"]) == {
            "outbound-executor", "model-discovery", "usage-reporter", "account-auth", "module-rpc", "agent-wiring", "request-policy"
        }, "Default package omits an implemented capability")
        providers = manifest.get("providers", [])
        require(len(providers) == 1 and providers[0]["id"] == "commandcode-native"
                and set(providers[0]["protocols"]) == {"chat", "responses", "anthropic"}
                and providers[0].get("operations") == ["systemone"], "Default package omits ordinary provider or decision execution")
        options = providers[0].get("options", [])
        require(len(options) == 1 and options[0]["key"] == "zdr"
                and "cost more" in options[0].get("confirm", ""),
                "Default package omits confirmed ZDR option")
        require(len(manifest.get("agents", [])) == 1 and manifest["agents"][0]["id"] == "commandcode-sdk"
                and manifest["agents"][0].get("profile", {}).get("target") == "commandcode",
                "Default package omits the checked agent adapter")
        require(manifest.get("window", {}).get("metadata") == "package.json"
                and metadata.get("magpie", {}).get("page", {}).get("file") == "web/index.html"
                and "web/app.js" in inventory, "Default package omits its implemented UI")
        require(not metadata.get("dependencies") and not metadata.get("optionalDependencies"), "Additional runtime package dependency")
        declared = metadata["files"] + ["package.json", "inventory.json"]
        for name in names:
            require(any(name == path or name.startswith(path + "/") for path in declared), "Undeclared packaged file: " + name)
        for path in declared:
            require(any(name == path or name.startswith(path + "/") for name in names), "Declared file absent: " + path)
        documents = {
            "package.json": "web/package.json", "README.md": "README.md",
            "SOURCES.md": "docs/SOURCES.md", "CONTRACT-PLAN.md": "docs/history/CONTRACT-PLAN.md",
            "REQUEST-POLICY.md": "docs/REQUEST-POLICY.md",
            "SYSTEMONE-SDK-CONTRACT.md": "docs/SYSTEMONE-SDK-CONTRACT.md",
            "SYSTEMONE-SDK-FIXTURES.json": "docs/SYSTEMONE-SDK-FIXTURES.json",
            "OWN-MODEL-METADATA-API9.md": "docs/OWN-MODEL-METADATA-API9.md",
            "THIRD_PARTY_NOTICES": "THIRD_PARTY_NOTICES",
        }
        for name, relative in documents.items():
            require(archive.read(name) == (root / relative).read_bytes(), "Packaged documentation differs: " + name)
        require(b"../../magpie" not in archive.read("web/app.js"), "UI retained a build-time SDK import")
    print(json.dumps({"standalonePackage": True, "files": len(names), "sources": len(lock["sources"]),
                      "sourceLockSHA256": hashlib.sha256((root / "docs/source-lock.json").read_bytes()).hexdigest(),
                      "packageSHA256": hashlib.sha256(args.archive.read_bytes()).hexdigest()}))


if __name__ == "__main__":
    main()
