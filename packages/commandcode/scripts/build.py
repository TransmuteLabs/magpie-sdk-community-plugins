#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import zipfile


PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment
from _web import compile_web


def main():
    parser = argparse.ArgumentParser(description="Build this native module with the already installed Go/TypeScript toolchains.")
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--sdk", type=Path, help="Magpie SDK checkout (or MAGPIE_SDK_SOURCE)")
    parser.add_argument("--archive", action="store_true", help="Also write a ZIP beside the installable directory")
    args = parser.parse_args()
    with build_environment(PACKAGE, args.sdk) as (env, selected_sdk):
        build(args.output.resolve(), env, selected_sdk, args.archive)


def build(output, env, selected_sdk, archive_requested=False):
    root = PACKAGE
    source = root / "src"
    if output.exists() and any(output.iterdir()):
        raise SystemExit("Use an empty output directory so stale artifacts cannot enter the package.")
    output.mkdir(parents=True, exist_ok=True)
    documents = {
        "package.json": "web/package.json", "README.md": "README.md",
        "CONTRACT-PLAN.md": "docs/history/CONTRACT-PLAN.md",
        "SOURCES.md": "docs/SOURCES.md", "source-lock.json": "docs/source-lock.json",
        "REQUEST-POLICY.md": "docs/REQUEST-POLICY.md",
        "SYSTEMONE-SDK-CONTRACT.md": "docs/SYSTEMONE-SDK-CONTRACT.md",
        "SYSTEMONE-SDK-FIXTURES.json": "docs/SYSTEMONE-SDK-FIXTURES.json",
        "OWN-MODEL-METADATA-API9.md": "docs/OWN-MODEL-METADATA-API9.md",
        "THIRD_PARTY_NOTICES": "THIRD_PARTY_NOTICES",
    }
    for name, relative in documents.items():
        shutil.copy2(root / relative, output / name)
    (output / "lib").mkdir(exist_ok=True)
    (output / "web").mkdir(exist_ok=True)
    manifest = json.loads(subprocess.check_output(["go", "run", "./cmd/manifest"], cwd=source, env=env))
    lock = json.loads((root / "docs/source-lock.json").read_text())
    if (manifest["version"] != lock["plugin"]["version"] or manifest["id"] != lock["plugin"]["id"]
        or manifest["compatibility"]["nativeABI"] != lock["plugin"]["nativeABI"]
        or manifest["compatibility"]["hostAPI"] != "^" + lock["plugin"]["sdkMinimum"]):
        raise SystemExit("Native manifest and provenance disagree")
    for fixture in lock["fixtures"]:
        if hashlib.sha256((root / fixture["path"]).read_bytes()).hexdigest() != fixture["sha256"]:
            raise SystemExit("Source fixture differs from its provenance: " + fixture["path"])
    subprocess.run(["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(output / manifest["entry"]), "./cmd/native"], cwd=source, env=env, check=True)
    compile_web(root, selected_sdk, output / "web", env)
    shutil.copy2(root / "web/index.html", output / "web/index.html")
    header = (output / manifest["entry"]).with_suffix(".h")
    header.unlink(missing_ok=True)
    (output / "magpie-plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")
    inventory = {}
    for file in sorted(output.rglob("*")):
        if file.is_file():
            inventory[file.relative_to(output).as_posix()] = hashlib.sha256(file.read_bytes()).hexdigest()
    (output / "inventory.json").write_text(json.dumps(inventory, indent=2) + "\n")
    result = {"directory": str(output), "files": len(inventory)}
    if archive_requested:
        result.update(write_archive(output, manifest, env))
    print(json.dumps(result))


def write_archive(output, manifest, env):
    platform = subprocess.check_output(["go", "env", "GOOS", "GOARCH"], cwd=PACKAGE / "src", env=env, text=True).split()
    archive = output.parent / f'{manifest["id"]}-{manifest["version"]}-{"-".join(platform)}.zip'
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, strict_timestamps=False) as package:
        for file in sorted(output.rglob("*")):
            if file.is_file():
                package.write(file, file.relative_to(output))
    return {"package": str(archive), "sha256": hashlib.sha256(archive.read_bytes()).hexdigest()}


if __name__ == "__main__":
    main()
