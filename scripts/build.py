#!/usr/bin/env python3
"""Build one or all independently installable plugin distributions."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _workspace import ROOT, effective_target, packages, read_json, sdk_source, select_build_packages, validate_distribution


def write_package_description(package, distribution, manifest):
    source = read_json(package / "package.json")
    if (source["name"], source["version"]) != (manifest["id"], manifest["version"]):
        raise ValueError("Package metadata and SDK manifest differ in identity or version")
    description = source["description"]
    if not isinstance(description, str) or not description.strip():
        raise ValueError("Package description must be a non-empty string")

    path = distribution / "package.json"
    metadata = read_json(path) if path.exists() else {
        "name": source["name"], "version": source["version"],
    }
    if metadata["version"] != source["version"]:
        raise ValueError("Packaged window metadata has a different version")
    metadata["description"] = description
    data = (json.dumps(metadata, indent=2) + "\n").encode()
    path.write_bytes(data)

    inventory_path = distribution / "inventory.json"
    if inventory_path.is_file():
        inventory = read_json(inventory_path)
        inventory["package.json"] = hashlib.sha256(data).hexdigest()
        inventory_path.write_text(json.dumps(inventory, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", choices=["all", *packages()])
    parser.add_argument("--sdk", type=Path)
    parser.add_argument("--output", type=Path, default=ROOT / "dist", help="parent directory for package outputs")
    args = parser.parse_args()
    entries = packages()
    try:
        target, environment = effective_target("go", ROOT / "packages" / next(iter(entries)))
        selected, skipped = select_build_packages(entries, args.package, target)
    except (OSError, ValueError) as error:
        parser.error(str(error))
    for name, spec in skipped.items():
        print(f"Skipping {name}: build target {target} is not supported ({', '.join(spec['platforms'])})",
              file=sys.stderr, flush=True)
    if not selected:
        return
    sdk = sdk_source(args.sdk)
    output = args.output.resolve()
    for name in selected:
        if (output / name).exists():
            parser.error(f"Output already exists: {output / name}; choose a fresh output directory")
    for name, spec in selected.items():
        package = ROOT / "packages" / name
        subprocess.run([sys.executable, str(package / "scripts/build.py"),
                        "--output", str(output / name), "--sdk", str(sdk)], check=True, env=environment)
        manifest = validate_distribution(output / name, spec["id"])
        write_package_description(package, output / name, manifest)
        print(json.dumps({"package": name, "version": manifest["version"],
                          "directory": str(output / name)}), flush=True)


if __name__ == "__main__":
    main()
