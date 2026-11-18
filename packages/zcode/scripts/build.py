#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import shutil
import sys
import tempfile
import zipfile


PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment


def build(args, env, sdk_root):
    root = PACKAGE / "src"
    target = subprocess.check_output(
        ["go", "env", "-json", "GOOS", "GOARCH", "CGO_ENABLED"], cwd=root, env=env
    )
    platform = json.loads(target)
    if platform["CGO_ENABLED"] != "1":
        raise SystemExit("native ZCode requires CGO_ENABLED=1")
    declaration = subprocess.check_output(
        ["go", "run", "./cmd/manifest", *(["--headless"] if args.headless else [])], cwd=root, env=env
    )
    manifest = json.loads(declaration)
    output = args.output.expanduser().resolve()
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        raise SystemExit("output must be a new or empty directory")
    archives = args.archive.expanduser().resolve() if args.archive else None
    if archives is not None and archives.is_relative_to(output):
        raise SystemExit("archive directory must be outside the installable output")
    output.mkdir(exist_ok=True, parents=True)
    with tempfile.TemporaryDirectory(prefix="zcode-package-", dir=output.parent) as scratch:
        stage = Path(scratch)
        library = stage / manifest["entry"]
        library.parent.mkdir(parents=True)
        subprocess.run(
            ["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(library),
             "./cmd/native" if args.headless else "./cmd/native-ui"],
            cwd=root, env=env, check=True
        )
        library.chmod(0o755)
        (stage / "magpie-plugin.json").write_bytes(declaration)
        provenance = json.loads((PACKAGE / "docs/source-lock.json").read_text())
        if provenance["schema"] != 1 or provenance["plugin"]["id"] != manifest["id"]:
            raise SystemExit("source provenance does not match the native module")
        provenance["plugin"].update(version=manifest["version"],
                                    sdkMinimum=f'{manifest["contract"]["version"]["major"]}.{manifest["contract"]["version"]["minor"]}',
                                    nativeABI=manifest["compatibility"]["nativeABI"])
        (stage / "source-lock.json").write_text(json.dumps(provenance, indent=2) + "\n")
        provenance_docs = ["SOURCES.md", "VENDOR-DIFF-3.18.2.md", "COMMUNITY-PARITY.md",
                           "ONE-BUNDLE-AUDIT.md", "SQLITE-DEPENDENCIES.md",
                           "DATA-FIDELITY.md", "LEADER-CONTRACTS.md", "NATIVE-CONTRACT-REQUEST.md",
                           "AUTOMATIC-HISTORY.md", "NATIVE-FOREGROUND.md"]
        for name in provenance_docs:
            shutil.copyfile(PACKAGE / "docs" / name, stage / name)
        members = ["magpie-plugin.json", manifest["entry"], "source-lock.json", *provenance_docs]
        if not args.headless:
            config = {
                "compilerOptions": {
                    "strict": True, "target": "ES2022", "module": "NodeNext",
                    "moduleResolution": "NodeNext", "lib": ["ES2022", "DOM"],
                    "rootDir": str(PACKAGE / "web"), "outDir": str(stage / "web"),
                    "paths": {"@magpie/extension-sdk/ui": [str(sdk_root / "sdk/js/ui.d.ts")]},
                },
                "include": [str(PACKAGE / "web" / "*.ts")],
            }
            tsconfig = stage / "tsconfig.json"
            tsconfig.write_text(json.dumps(config))
            subprocess.run(["tsc", "--project", str(tsconfig)], cwd=root, env=env, check=True)
            for name in ("index.html", "style.css"):
                shutil.copyfile(PACKAGE / "web" / name, stage / "web" / name)
            subprocess.run([
                "bun", "build", str(sdk_root / "sdk/js/ui.js"), "--target", "browser",
                "--format", "esm", "--outfile", str(stage / "web/sdk-ui.js"),
            ], cwd=root, env=env, check=True)
            subprocess.run([
                "bun", "build", str(stage / "web/app.js"), "--target", "browser",
                "--format", "iife", "--outfile", str(stage / "web/app.bundle.js"),
            ], cwd=root, env=env, check=True)
            shutil.copyfile(PACKAGE / "web/package.json", stage / "package.json")
            members.append("package.json")
            members.extend(str(path.relative_to(stage)) for path in sorted((stage / "web").iterdir()))
        for member in members:
            destination = output / member
            destination.parent.mkdir(exist_ok=True, parents=True)
            shutil.copy2(stage / member, destination)
        result = {"distribution": str(output), "nativeABI": 1, "manifest": manifest["id"]}
        if archives is not None:
            archives.mkdir(exist_ok=True, parents=True)
            name = f"magpie-plugin_{platform['GOOS']}_{platform['GOARCH']}.zip"
            target = archives / name
            with zipfile.ZipFile(target, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                for member in members:
                    archive.write(stage / member, member)
            digest = hashlib.sha256(target.read_bytes()).hexdigest()
            (archives / "checksums.txt").write_text(f"{digest}  {name}\n")
            result.update(asset=str(target), sha256=digest)
        print(json.dumps(result))


def main():
    parser = argparse.ArgumentParser(description="Build the complete native ZCode package.")
    parser.add_argument("--output", required=True, type=Path, help="new or empty installable distribution directory")
    parser.add_argument("--archive", type=Path, help="also write ZIP/checksums outside the installable output")
    parser.add_argument("--sdk", type=Path, help="Magpie SDK checkout (or MAGPIE_SDK_SOURCE)")
    parser.add_argument("--headless", action="store_true", help="build the explicit provider-only composition (SDK 1.12)")
    args = parser.parse_args()
    with build_environment(PACKAGE, args.sdk) as (env, sdk_root):
        build(args, env, sdk_root)


if __name__ == "__main__":
    main()
