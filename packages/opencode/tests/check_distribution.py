#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import re


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("distribution", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1] / "src"
    package = args.distribution.resolve()
    manifest = json.loads((package / "magpie-plugin.json").read_text())
    lock = json.loads((package / "source-lock.json").read_text())
    source = json.loads((root.parent / "docs/source-lock.json").read_text())
    minimum = manifest["contract"]["version"]
    assert lock["plugin"] == {
        "id": manifest["id"], "version": manifest["version"],
        "sdkMinimum": str(minimum["major"]) + "." + str(minimum["minor"]),
        "nativeABI": manifest["compatibility"]["nativeABI"],
    }
    assert lock["schema"] == source["schema"] == 1
    assert lock["sources"] == source["sources"] and lock["watch"] == source["watch"]
    assert lock["compatibilityArtifacts"] == source["compatibilityArtifacts"]
    assert lock["validation"] == source["validation"]
    assert (package / "donor-provenance.json").read_bytes() == (root / "testdata/donors/provenance.json").read_bytes()
    assert (package / "SOURCES.md").read_bytes() == (root.parent / "docs/SOURCES.md").read_bytes()
    assert manifest["id"] == "opencode" and manifest["runtime"] == "native"
    assert not manifest.get("services"), "distribution unexpectedly needs a sidecar"
    assert not list(package.rglob("node_modules")), "distribution needs another package install"
    assert (package / manifest["entry"]).is_file()
    if manifest.get("window") or manifest["version"] == source["plugin"]["version"]:
        assert manifest.get("window"), "ordinary package omitted its history UI"
        expected = json.loads((root / "magpie-plugin.json").read_text())
        expected["entry"] = manifest["entry"]
        metadata = json.loads((package / manifest["window"]["metadata"]).read_text())
        html = (package / "web/index.html").read_text()
        if lock["variant"] == "api8":
            artifact = next(item for item in source["compatibilityArtifacts"] if item["variant"] == "api8")
            assert artifact == {"id": "opencode", "version": manifest["version"], "sdkMinimum": "1.12",
                                "nativeABI": 1, "variant": "api8", "buildFlag": "--compat-api8"}
            expected["version"] = artifact["version"]
            expected["contract"]["capabilities"].remove("provider-signin")
            legacy = {
                "signin": ("opencode-go", ("device",)),
                "zen.signin": ("opencode-zen", ("device", "key")),
                "public.signin": ("opencode-zen-free", ("public",)),
            }
            expected["authBindings"] = [
                {"id": grant, "from": "window:page", "provider": provider, "reauthorize": True,
                 "methods": [{"id": method, "mode": "poll"} for method in methods]}
                for grant, (provider, methods) in legacy.items()
            ]
            for binding in expected["bindings"]:
                binding["from"] = "window:page"
            assert metadata["magpie"]["page"]["file"] == "web/index.html"
            assert "panels" not in metadata["magpie"] and 'id="signin"' in html
        else:
            assert lock["variant"] == "full"
            assert manifest["version"] == json.loads((root.parent / "package.json").read_text())["version"]
            assert lock["windowPlacement"] == source["windowPlacement"]
            assert manifest["version"] == lock["windowPlacement"]["version"]
            assert metadata["magpie"]["name"] == "OpenCode" and metadata["magpie"]["icon"] == "opencode"
            panel, = metadata["magpie"]["panels"]
            assert panel["slot"] == "sessions" and panel["file"] == "web/index.html"
            assert "page" not in metadata["magpie"] and 'id="signin"' not in html
        assert manifest == expected, "full package drifted from its complete declaration"
        assert minimum == {"major": 1, "minor": 12}
        assert "request-policy" in manifest["contract"]["capabilities"]
        assert manifest["compatibility"]["hostAPI"] == "^1." + str(minimum["minor"])
        assert "session-storage-source" in manifest["contract"]["capabilities"]
        assert [source["id"] for source in manifest["storageSources"]] == ["opencode-history"]
        assert len(manifest["providers"]) == 3 and len(manifest["agents"]) == 1
        assert len(manifest["authBindings"]) == 3
        assert not metadata.get("dependencies") and not metadata.get("devDependencies")
        assert metadata["version"] == manifest["version"]
        assert 'id="history-panel"' in html and "app.browser.js" in html
        browser = (package / "web/app.browser.js").read_text()
        assert not re.search(r"\b(?:from\s*|import\s*)[\"']", browser), "iframe bootstrap still loads relative module files"
        for script in (package / "web").glob("*.js"):
            text = script.read_text()
            for target in re.findall(r"\b(?:from\s*|import\s*)[\"']([^\"']+)[\"']", text):
                assert target.startswith("./"), "privileged or bare browser import: " + target
                dependency = (script.parent / target).resolve()
                assert dependency.is_relative_to(package / "web") and dependency.is_file(), "unpackaged browser dependency: " + target
            assert not re.search(r"\b(?:require|import)\s*\(", text), "runtime loader in browser package"
    files = {str(path.relative_to(package)): hashlib.sha256(path.read_bytes()).hexdigest()
             for path in sorted(package.rglob("*")) if path.is_file()}
    print(json.dumps({"package": lock["plugin"], "files": files}, indent=2))


if __name__ == "__main__":
    main()
