import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("label")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--sdk", type=Path)
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    output = args.output.resolve()
    archives = (args.archive or args.root / (args.label + "-archives")).resolve()
    sdk_args = ["--sdk", str(args.sdk.resolve())] if args.sdk else []
    subprocess.run(["python3", "-I", str(source.parent / "scripts/build.py"),
                    "--output", str(output), "--archive", str(archives), *sdk_args], cwd=source, check=True, timeout=180)
    asset = archives / "magpie-plugin_linux_amd64.zip"
    with zipfile.ZipFile(asset) as archive:
        manifest = json.loads(archive.read("magpie-plugin.json"))
        names = archive.namelist()
        assert json.loads((output / "magpie-plugin.json").read_bytes()) == manifest
        assert {str(path.relative_to(output)) for path in output.rglob("*") if path.is_file()} == set(names)
        for name in names:
            assert (output / name).read_bytes() == archive.read(name), name
        report = {"version": manifest["version"], "providers": manifest.get("providers", []),
                  "agents": manifest.get("agents", []), "window": manifest.get("window"),
                  "contract": manifest["contract"],
                  "artifactSHA256": hashlib.sha256(asset.read_bytes()).hexdigest()}
        (args.root / (args.label + "-observed.json")).write_text(json.dumps(report, indent=2) + "\n")
        assert manifest.get("window") and len(manifest.get("agents", [])) == 1, \
            "default package omits the ready agent/UI composition"
        assert manifest["agents"][0]["id"] == "zcode-native"
        assert [value["id"] for value in manifest["providers"]] == ["zcode"]
        assert manifest["contract"]["version"] == {"major": 1, "minor": 20}
        assert "request-policy" in manifest["contract"]["capabilities"]
        assert manifest["providers"][0]["maxConcurrency"] == 3
        assert manifest["providers"][0]["maxAccounts"] == 2
        assert manifest["compatibility"]["nativeABI"] == 1
        actions = {value["id"] for value in manifest["actions"]}
        assert {"execution.list", "execution.settle", "billing.claim", "billing.checkin", "billing.offers"} <= actions
        bindings = {value["id"]: value["action"] for value in manifest["bindings"]}
        assert bindings["page.billing-offers"] == "billing.offers"
        assert bindings["page.billing-checkin"] == "billing.checkin"
        assert json.loads(archive.read("package.json"))["version"] == manifest["version"]
        assert {"web/app.bundle.js", "web/sdk-ui.js", "web/index.html", "package.json",
                "source-lock.json", manifest["entry"]} <= set(names)
        assert len([name for name in names if name.startswith("lib/")]) == 1
        assert not any(name.startswith(("node_modules/", "packages/", "runtime/")) for name in names)
        provenance = json.loads(archive.read("source-lock.json"))
        assert provenance["plugin"] == {"id": "zcode", "version": manifest["version"],
                                        "sdkMinimum": "1.20", "nativeABI": 1}
    print("default full ZCode package passed", report["artifactSHA256"], flush=True)


if __name__ == "__main__":
    main()
