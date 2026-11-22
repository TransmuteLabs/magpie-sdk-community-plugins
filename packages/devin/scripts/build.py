"""Build the Devin native distribution. Run only in the usbox workspace."""

import argparse
import json
from pathlib import Path
import subprocess
import sys

PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))

from _workspace import build_environment
from provenance import source_lock


def build(destination, inbound_only=False, agent=True, sdk=None):
    source = PACKAGE / "src"
    manifest_name = "magpie-inbound.json" if inbound_only else "magpie-agent.json" if agent else "magpie-plugin.json"
    provenance = source_lock(PACKAGE, manifest_name)
    with build_environment(PACKAGE, sdk) as (env, _):
        target = json.loads(subprocess.check_output(
            ["go", "env", "-json", "GOOS", "GOARCH", "CGO_ENABLED"],
            cwd=source, env=env, text=True,
        ))
        if target["CGO_ENABLED"] != "1":
            raise SystemExit("native authoring requires CGO_ENABLED=1 and a target C compiler")
        platform = target["GOOS"] + "/" + target["GOARCH"]
        manifest = json.loads((source / manifest_name).read_text())
        if platform not in manifest["compatibility"]["platforms"]:
            raise SystemExit("unsupported native distribution platform: " + platform)
        suffix = {"darwin": ".dylib", "windows": ".dll"}.get(target["GOOS"], ".so")
        destination = Path(destination).resolve()
        destination.mkdir(parents=True, exist_ok=True)
        entry = "plugin" + suffix
        subprocess.run([
            "go", "build", "-mod=readonly", "-trimpath", "-buildmode=c-shared",
            "-o", str(destination / entry),
            "./cmd/native-inbound" if inbound_only else "./cmd/native-agent" if agent else "./cmd/native",
        ], cwd=source, env=env, check=True)
    manifest["entry"] = entry
    manifest["compatibility"]["platforms"] = [platform]
    (destination / "magpie-plugin.json").write_text(json.dumps(manifest, indent=2) + "\n")
    metadata = json.loads((PACKAGE / "package.json").read_text())
    metadata["version"] = manifest["version"]
    (destination / "package.json").write_text(json.dumps(metadata, indent=2) + "\n")
    for name in ("LICENSE", "docs/SOURCES.md", "docs/ONBOARDING.md", "docs/EXECUTION-FACTS.md", "docs/DONOR-MATRIX.md",
                 "docs/PROVIDER-PARITY.md", "docs/CLIPROXY-PROVIDER-CHECK.md",
                 "docs/history/DELIVERY-EVIDENCE.md", "docs/CACHE-KEEPALIVE.md",
                 "docs/history/CACHE-KEEPALIVE-EVIDENCE.md", "docs/history/GEMINI-SIGNATURE-EVIDENCE.md",
                 "docs/history/DOMAIN-COMPLETION-EVIDENCE.md"):
        (destination / Path(name).name).write_bytes((PACKAGE / name).read_bytes())
    (destination / "source-lock.json").write_text(json.dumps(provenance, indent=2) + "\n")
    (destination / "plugin.h").unlink(missing_ok=True)
    print(json.dumps({"directory": str(destination), "platform": platform, "entry": entry}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--sdk", type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--inbound-only", action="store_true")
    mode.add_argument("--paired-only", action="store_true")
    mode.add_argument("--agent", action="store_true", help="explicit alias for the complete default package")
    args = parser.parse_args()
    build(args.output, inbound_only=args.inbound_only, agent=not args.paired_only, sdk=args.sdk)
