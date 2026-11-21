#!/usr/bin/env python3
"""Build the native payload in an empty distribution directory. Run on usbox."""
import json
import argparse
import os
import pathlib
import platform
import subprocess
import sys
import shutil

PACKAGE = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment

root = PACKAGE / "src"
parser = argparse.ArgumentParser()
parser.add_argument("--output", required=True, type=pathlib.Path, help="empty distribution directory")
parser.add_argument("--sdk", type=pathlib.Path, help="Magpie SDK checkout; defaults to MAGPIE_SDK_SOURCE or sibling magpie")
compat = parser.add_mutually_exclusive_group()
compat.add_argument("--compat-provider", action="store_true", help="historical provider-only compatibility/test artifact")
compat.add_argument("--compat-agent", "--agent", dest="compat_agent", action="store_true", help="historical headless-agent compatibility/test artifact")
compat.add_argument("--compat-api8", action="store_true", help="legacy full-package flag; current runtime requires API1.12")
compat.add_argument("--window", action="store_true", help=argparse.SUPPRESS)
args = parser.parse_args()
destination = args.output.resolve()
destination.mkdir(parents=True, exist_ok=True)
if any(destination.iterdir()):
    raise SystemExit("distribution directory must be empty")
target_os = os.environ.get("GOOS") or {"Darwin": "darwin", "Windows": "windows"}.get(platform.system(), "linux")
suffix = {"darwin": "dylib", "windows": "dll"}.get(target_os, "so")
entry = f"lib/opencode.{suffix}"
(destination / "lib").mkdir()
with build_environment(PACKAGE, args.sdk) as (env, sdk_root):
    variant = "./cmd/native-provider" if args.compat_provider else "./cmd/native-agent" if args.compat_agent else "./cmd/native-api8" if args.compat_api8 else "./cmd/native"
    manifest = json.loads(subprocess.check_output(["go", "run", "-mod=readonly", "./cmd/manifest",
                          *(["--compat-provider"] if args.compat_provider else ["--compat-agent"] if args.compat_agent else ["--compat-api8"] if args.compat_api8 else [])], cwd=root, env=env))
    subprocess.run(["go", "build", "-mod=readonly", "-buildmode=c-shared", "-o", str(destination / entry), variant], cwd=root, env=env, check=True)
    manifest["entry"] = entry
    (destination / "magpie-plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")
    provenance = json.loads((PACKAGE / "docs/source-lock.json").read_text())
    provenance["variant"] = "provider" if args.compat_provider else "agent" if args.compat_agent else "api8" if args.compat_api8 else "full"
    provenance["plugin"] = {
        "id": manifest["id"], "version": manifest["version"],
        "sdkMinimum": str(manifest["contract"]["version"]["major"]) + "." + str(manifest["contract"]["version"]["minor"]),
        "nativeABI": manifest["compatibility"]["nativeABI"],
    }
    (destination / "source-lock.json").write_text(json.dumps(provenance, indent=2) + "\n")
    shutil.copyfile(PACKAGE / "docs/SOURCES.md", destination / "SOURCES.md")
    shutil.copyfile(PACKAGE / "docs/REQUEST-POLICY.md", destination / "REQUEST-POLICY.md")
    shutil.copyfile(root / "testdata/donors/provenance.json", destination / "donor-provenance.json")
    if not args.compat_provider and not args.compat_agent:
        web = destination / "web"
        web.mkdir()
        config = {
            "compilerOptions": {
                "target": "ES2022", "module": "NodeNext", "moduleResolution": "NodeNext",
                "lib": ["ES2023", "DOM"], "strict": True, "rootDir": str(PACKAGE / "web"), "outDir": str(web),
                "paths": {"@magpie/extension-sdk/ui": [str(sdk_root / "sdk/js/ui.d.ts")]},
            },
            "include": [str(PACKAGE / "web" / "*.ts")],
        }
        project = destination / "tsconfig.json"
        project.write_text(json.dumps(config))
        subprocess.run(["tsc", "-p", str(project)], cwd=root, env=env, check=True)
        project.unlink()
        for source, target in (("ui.js", "sdk-ui.js"), ("account-selection.js", "account-selection.js"),
                               ("errors.js", "errors.js")):
            shutil.copyfile(sdk_root / "sdk/js" / source, web / target)
        bun = os.environ.get("MAGPIE_BUILD_BUN") or shutil.which("bun")
        if not bun:
            raise SystemExit("browser packaging requires the existing pinned Bun build tool")
        subprocess.run([bun, "build", str(web / "app.js"), "--target=browser", "--format=iife",
                        "--outfile=" + str(web / "app.browser.js")], cwd=root, env=env, check=True)
        html = "index.html" if args.compat_api8 else "history.html"
        shutil.copyfile(PACKAGE / "web" / html, web / "index.html")
        metadata = json.loads((PACKAGE / "web/package.json").read_text())
        metadata["version"] = manifest["version"]
        if args.compat_api8:
            metadata["magpie"] = {"page": {"title": "OpenCode", "file": "web/index.html"}, "permissions": []}
        (destination / "package.json").write_text(json.dumps(metadata, indent=2) + "\n")
print(destination)
