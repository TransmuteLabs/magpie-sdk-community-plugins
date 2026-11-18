"""Build the self-contained macOS Claude Desktop agent module offline."""

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys

PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment


def snapshot():
    files = {
        path.relative_to(PACKAGE).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
        for part in ("src", "docs", "scripts")
        for path in sorted((PACKAGE / part).rglob("*"))
        if path.is_file() and "__pycache__" not in path.parts
    }
    files["README.md"] = hashlib.sha256((PACKAGE / "README.md").read_bytes()).hexdigest()
    return files


def build(destination, sdk=None):
    destination = Path(destination).resolve()
    if destination.is_relative_to(PACKAGE):
        raise ValueError("distribution must be outside the package source")
    if destination.exists() and any(destination.iterdir()):
        raise ValueError("distribution directory must be empty")
    source = PACKAGE / "src"
    with build_environment(PACKAGE, sdk) as (environment, _):
        target = json.loads(subprocess.check_output(
            ["go", "env", "-json", "GOOS", "GOARCH", "CGO_ENABLED", "GOVERSION"],
            cwd=source, env=environment, text=True,
        ))
        if target["GOOS"] != "darwin" or target["GOARCH"] not in ("arm64", "amd64") or target["CGO_ENABLED"] != "1":
            raise ValueError("this package requires a native macOS cgo build")
        inputs = snapshot()
        manifest = json.loads(subprocess.check_output(
            ["go", "run", "./cmd/native"], cwd=source, env=environment, text=True,
        ))
        lock = json.loads((PACKAGE / "docs/source-lock.json").read_text())
        identity = {
            "id": manifest["id"], "version": manifest["version"],
            "sdkMinimum": ".".join(str(manifest["contract"]["version"][key]) for key in ("major", "minor")),
            "nativeABI": manifest["compatibility"]["nativeABI"],
        }
        if lock["plugin"] != identity or manifest["runtime"] != "native" or manifest["entry"] != "lib/claude-desktop.dylib":
            raise ValueError("source provenance and typed manifest disagree")
        destination.mkdir(parents=True, exist_ok=True)
        entry = destination / manifest["entry"]
        entry.parent.mkdir()
        subprocess.run(["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(entry), "./cmd/native"],
                       cwd=source, env=environment, check=True)
        entry.with_suffix(".h").unlink(missing_ok=True)
        if snapshot() != inputs:
            raise ValueError("package inputs changed during compilation; no manifest published")
        shutil.copyfile(PACKAGE / "docs/source-lock.json", destination / "source-lock.json")
        shutil.copyfile(PACKAGE / "docs/SOURCES.md", destination / "SOURCES.md")
        shutil.copyfile(PACKAGE / "README.md", destination / "README.md")
        shutil.copytree(PACKAGE / "docs", destination / "docs", ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
        (destination / "build-provenance.json").write_text(json.dumps({
            "schema": 1, "plugin": identity, "toolchain": target,
            "packageInputs": inputs, "entrySHA256": hashlib.sha256(entry.read_bytes()).hexdigest(),
            "liveDesktopAcceptance": False,
        }, indent=2) + "\n")
        (destination / "magpie-plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")
    return manifest


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--sdk", type=Path)
    arguments = parser.parse_args()
    result = build(arguments.output, arguments.sdk)
    print(json.dumps({"id": result["id"], "version": result["version"], "output": str(arguments.output)}))
