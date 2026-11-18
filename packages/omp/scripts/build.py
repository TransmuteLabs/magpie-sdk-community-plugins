"""Build this native module's distribution with the public SDK C ABI."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys

PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))

from _workspace import build_environment

from provenance import snapshot_build_inputs, write_provenance


def build(destination, sdk_root=None):
    module = PACKAGE / "src"
    repository = module
    destination = Path(destination).resolve()
    if destination.exists() and any(destination.iterdir()):
        raise ValueError("distribution destination must be empty")
    go = os.environ.get("GO", "go")
    with build_environment(PACKAGE, sdk_root) as (environment, _):
        platform = json.loads(subprocess.check_output(
            [go, "env", "-json", "GOOS", "GOARCH", "CGO_ENABLED", "GOVERSION"],
            cwd=repository, env=environment, text=True,
        ))
        if platform["CGO_ENABLED"] != "1":
            raise SystemExit("native build requires CGO_ENABLED=1 and a C compiler")
        if platform["GOOS"] not in ("linux", "darwin", "windows", "freebsd"):
            raise SystemExit("unsupported native host platform")
        arguments, target = ["-mod=readonly"], "./cmd/native"
        inputs = snapshot_build_inputs(go, module, arguments, target, env=environment)
        manifest = json.loads(subprocess.check_output(
            [go, "run", *arguments, target], cwd=repository, env=environment, text=True,
        ))
        if (manifest.get("contract", {}).get("version") != {"major": 1, "minor": 10} or
                not {"session-source", "session-migration", "module-rpc", "session-related-inputs"}.issubset(
                    manifest.get("contract", {}).get("capabilities", [])) or
                not manifest.get("sessionSources") or len(manifest.get("sessions", [])) != 6):
            raise ValueError("the typed factory did not describe the full API 1.10 module")
        entry = manifest.get("entry", "")
        if manifest.get("runtime") != "native" or not entry or Path(entry).name != entry or entry in (".", ".."):
            raise ValueError("the typed factory returned an invalid native manifest")
        destination.mkdir(parents=True, exist_ok=True)
        subprocess.run([
            go, "build", *arguments, "-trimpath", "-buildmode=c-shared",
            "-o", str(destination / entry), target,
        ], cwd=repository, env=environment, check=True)
        if snapshot_build_inputs(go, module, arguments, target, env=environment) != inputs:
            raise ValueError("build inputs changed during compilation; no manifest was published")
        (destination / "magpie-plugin.json").write_text(
            json.dumps(manifest, indent=2) + "\n"
        )
        write_provenance(module, destination, manifest, inputs, platform)
        (destination / "plugin.h").unlink(missing_ok=True)
    print(f"Built omp-sessions {platform['GOOS']}/{platform['GOARCH']}: {destination}")
    return manifest


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, help="new or empty distribution directory")
    parser.add_argument("--sdk", help="optional build-time SDK source directory")
    options = parser.parse_args()
    build(options.output, options.sdk)
