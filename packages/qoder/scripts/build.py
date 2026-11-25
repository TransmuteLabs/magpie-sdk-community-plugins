#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment, validate_distribution


def build(arguments, environment, sdk):
    output = arguments.output.expanduser().resolve()
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        raise SystemExit("output must be a new or empty installable directory")
    source = PACKAGE / "src"
    declaration = subprocess.check_output(
        ["go", "run", "./cmd/manifest"], cwd=source, env=environment,
    )
    manifest = json.loads(declaration)
    provenance = json.loads((PACKAGE / "docs/source-lock.json").read_text())
    expected = provenance["plugin"]
    version = manifest["contract"]["version"]
    if (expected["id"] != manifest["id"] or expected["version"] != manifest["version"]
            or expected["sdkMinimum"] != f'{version["major"]}.{version["minor"]}'
            or expected["nativeABI"] != manifest["compatibility"]["nativeABI"]):
        raise SystemExit("manifest and source provenance disagree")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="qoder-build-", dir=output.parent) as temporary:
        stage = Path(temporary)
        library = stage / manifest["entry"]
        if not library.resolve().is_relative_to(stage):
            raise SystemExit("native entry escapes the distribution")
        library.parent.mkdir(parents=True)
        subprocess.run(
            ["go", "build", "-trimpath", "-buildmode=c-shared", "-o", str(library), "./cmd/native"],
            cwd=source, env=environment, check=True,
        )
        library.chmod(0o755)
        library.with_suffix(".h").unlink(missing_ok=True)
        (stage / "magpie-plugin.json").write_bytes(declaration)
        for name in ("source-lock.json", "SOURCES.md", "DONOR-LICENSE.txt"):
            shutil.copy2(PACKAGE / "docs" / name, stage / name)
        shutil.copy2(PACKAGE / "package.json", stage / "package.json")
        build_info = {
            "schema": 1,
            "module": manifest["id"],
            "version": manifest["version"],
            "librarySHA256": hashlib.sha256(library.read_bytes()).hexdigest(),
            "sdkFiles": {
                name: hashlib.sha256((sdk / name).read_bytes()).hexdigest()
                for name in ("go.mod", "pkg/extension/contract.go", "pkg/extension/native/native.go")
            },
        }
        (stage / "build-info.json").write_text(json.dumps(build_info, indent=2) + "\n")
        validate_distribution(stage, "qoder")
        shutil.copytree(stage, output, dirs_exist_ok=True)
    print(json.dumps({"distribution": str(output), "manifest": "qoder",
                      "nativeABI": 1, "sha256": build_info["librarySHA256"]}))


def main():
    parser = argparse.ArgumentParser(description="Build the standalone native Qoder package.")
    parser.add_argument("--output", required=True, type=Path,
                        help="new or empty installable directory")
    parser.add_argument("--sdk", type=Path, help="public Magpie SDK checkout")
    arguments = parser.parse_args()
    with build_environment(PACKAGE, arguments.sdk) as (environment, sdk):
        build(arguments, environment, sdk)


if __name__ == "__main__":
    main()
