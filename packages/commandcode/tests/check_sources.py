#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Check extracted pinned source files against the packaged lock.")
    parser.add_argument("--source", action="append", required=True, metavar="ID=DIRECTORY")
    parser.add_argument("--host-overlay", type=Path)
    args = parser.parse_args()
    directories = dict(value.split("=", 1) for value in args.source)
    root = Path(__file__).resolve().parent.parent
    lock = json.loads((root / "docs/source-lock.json").read_bytes())
    checked = []
    for source in lock["sources"]:
        if source["id"] not in directories:
            raise SystemExit("Missing source: " + source["id"])
        directory = Path(directories[source["id"]])
        for name, expected in source["files"].items():
            actual = hashlib.sha256((directory / name).read_bytes()).hexdigest()
            if actual != expected:
                raise SystemExit("Pinned source differs: " + source["id"] + ":" + name)
        overlay = source.get("compatibilityOverlay")
        if overlay:
            if not args.host_overlay or overlay["basePhysicalTree"] != source["tree"]:
                raise SystemExit("The tested host compatibility overlay is required")
            for name, expected in overlay["files"].items():
                if hashlib.sha256((args.host_overlay / name).read_bytes()).hexdigest() != expected:
                    raise SystemExit("Host compatibility overlay differs: " + name)
        checked.append({"id": source["id"], "commit": source.get("commit"),
                        "distributionSHA256": source.get("distribution", {}).get("sha256"),
                        "files": len(source["files"]), "compatibilityOverlay": overlay})
    print(json.dumps({"pinnedSources": checked, "liveVendorEvidence": False}))


if __name__ == "__main__":
    main()
