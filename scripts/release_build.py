#!/usr/bin/env python3
"""Build one unpublished module version, or explicitly make a local dev artifact."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _workspace import ROOT, effective_target, packages, read_json, sdk_source, select_build_packages, validate_distribution
from release import module_release_version, module_tag, payload_inventory, validate_build_receipt


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def file_inputs(directory):
    result = {}
    for path in sorted(directory.rglob("*")):
        name = path.relative_to(directory)
        if name.parts[0] == "dist" or any(part in (".git", "__pycache__") for part in name.parts):
            continue
        if path.is_symlink():
            raise ValueError("Build inputs must not contain symlinks")
        if path.is_file():
            result[name.as_posix()] = {"sha256": digest(path), "mode": path.stat().st_mode & 0o777}
        elif not path.is_dir():
            raise ValueError("Build inputs must be regular files")
    return result


def sdk_inputs(directory, index_path):
    index = read_json(index_path)
    files = index["files"]
    identity = hashlib.sha256(json.dumps(files, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    if identity != index["identity"] or not files:
        raise ValueError("SDK input manifest identity differs from its files")
    actual = set()
    for path in directory.rglob("*"):
        relative = path.relative_to(directory)
        if ".git" in relative.parts:
            continue
        if path.is_symlink():
            raise ValueError("SDK input manifest cannot contain symlinks")
        if path.is_file():
            actual.add(relative.as_posix())
    if actual != set(files):
        raise ValueError("SDK source contains missing or unindexed files")
    for name, item in files.items():
        relative = PurePosixPath(name)
        if relative.is_absolute() or ".." in relative.parts or str(relative) != name or "\\" in name:
            raise ValueError("Invalid SDK input manifest path")
        path = directory / name
        if path.is_symlink() or not path.resolve().is_relative_to(directory) or digest(path) != item["sha256"]:
            raise ValueError("SDK source differs from its sealed input manifest")
        if path.stat().st_mode & 0o777 != item["mode"]:
            raise ValueError("SDK source mode differs from its sealed input manifest")
    return {"identity": identity, "manifestSHA256": digest(index_path), "files": len(files)}


def publication(inventory_path, identifier, version):
    inventory = read_json(inventory_path)
    if inventory.get("schema") != 1 or inventory.get("complete") is not True or not isinstance(inventory.get("releases"), list):
        raise ValueError("Publication inventory must be a complete versioned release listing")
    repository = inventory["repository"]
    if not isinstance(repository, str) or re.fullmatch(r"[A-Za-z0-9-]{1,100}/[A-Za-z0-9._-]{1,100}", repository) is None or repository.split("/")[1] in (".", ".."):
        raise ValueError("Publication inventory requires an explicit repository")
    target = module_tag(identifier, version)
    seen = set()
    found = False
    for release in inventory["releases"]:
        if not isinstance(release.get("tag_name"), str) or type(release.get("draft")) is not bool or type(release.get("prerelease")) is not bool:
            raise ValueError("Publication inventory has an unknown release state")
        tag = release["tag_name"]
        if release["draft"]:
            continue
        published_version = module_release_version(identifier, tag)
        if published_version is None:
            continue
        semantic = published_version.split("+")[0]
        if semantic in seen:
            raise ValueError("Publication inventory has ambiguous module version aliases")
        seen.add(semantic)
        if tag == target:
            found = True
        elif semantic == version.split("+")[0]:
            raise ValueError("An alias of this module version is already published; bump the version")
    return repository, found


def build_release(selection, version, output, sdk, sdk_index, published, previous=None, manual_dev=False, builder=None):
    if selection == "all":
        raise ValueError("A release builds exactly one selected module")
    spec = packages(selection)[selection]
    identifier = spec["id"]
    tag = module_tag(identifier, version)
    package = ROOT / "packages" / selection
    provenance = read_json(package / "docs/source-lock.json")["plugin"]
    if provenance["id"] != identifier or provenance["version"] != version:
        raise ValueError("Requested module/version differs from source provenance")
    repository, exists = publication(published, identifier, version)
    if exists and not manual_dev:
        return {"schema": 1, "status": "skip-published", "id": identifier, "version": version, "tag": tag,
                "builderInvoked": False, "publicationInventorySHA256": digest(published)}
    selected_sdk = sdk_source(sdk)
    executable = shutil.which("go")
    if executable is None:
        raise ValueError("An offline Go toolchain is required for an unpublished build")
    go = Path(executable)
    platform, environment = effective_target(go, package)
    select_build_packages({selection: spec}, selection, platform)
    inputs = {
        "package": file_inputs(package), "sdk": sdk_inputs(selected_sdk, sdk_index),
        "registryEntry": spec, "platform": platform,
        "tools": {name: digest(ROOT / "scripts" / name) for name in
                  ("build.py", "_workspace.py", "release.py", "release_build.py")},
        "goSHA256": digest(go),
        "buildEnvironment": {name: os.environ.get(name, "") for name in
                             ("CGO_ENABLED", "CC", "CXX", "SDKROOT", "DEVELOPER_DIR", "MACOSX_DEPLOYMENT_TARGET")},
    }
    input_digest = hashlib.sha256(json.dumps(inputs, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    source_inputs = {name: inputs[name] for name in ("package", "sdk", "registryEntry", "tools")}
    source_digest = hashlib.sha256(json.dumps(source_inputs, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    if previous is not None:
        prior = read_json(previous)
        if prior.get("schema") != 2:
            raise ValueError("Prior build receipt has an unknown schema")
        if prior["id"] != identifier:
            raise ValueError("Prior build receipt belongs to another module")
        if prior["version"] == version and not manual_dev:
            if not isinstance(prior.get("outputs"), dict) or not prior["outputs"]:
                raise ValueError("Prior local artifact inventory is missing")
            old_output = Path(prior["output"])
            validate_build_receipt(prior, selection, identifier, version, old_output / "built" / selection)
            if prior.get("sourceInputsSHA256") != source_digest:
                raise ValueError("Same-version inputs changed; bump the module version or use local-only --manual-dev")
            if prior["platform"] == platform:
                if prior["inputsSHA256"] != input_digest:
                    raise ValueError("Same-version inputs changed for this target; bump the version or use local-only --manual-dev")
                return {**prior, "status": "reuse-local", "builderInvoked": False}
    output = Path(output).absolute()
    if output.exists() or output.is_symlink():
        raise ValueError("Build output already exists; choose a fresh directory")
    output = output.resolve()
    if output.is_relative_to(package) or output.is_relative_to(selected_sdk):
        raise ValueError("Build output must be outside module and SDK inputs")
    output.mkdir(parents=True, exist_ok=False)
    (output / "build-inputs.json").write_text(json.dumps(inputs, indent=2) + "\n")
    command = [sys.executable, str(ROOT / "scripts/build.py"), selection,
               "--sdk", str(selected_sdk), "--output", str(output / "built")]
    run = builder or subprocess.run
    run(command, check=True, env=environment)
    manifest = validate_distribution(output / "built" / selection, identifier)
    if manifest["version"] != version:
        raise ValueError("Built distribution changed the requested module version")
    if file_inputs(package) != inputs["package"] or sdk_inputs(selected_sdk, sdk_index) != inputs["sdk"]:
        raise ValueError("Build inputs changed during compilation")
    result = {
        "schema": 2, "status": "built-local-dev" if manual_dev else "built-release-candidate", "id": identifier,
        "version": version, "tag": tag, "builderInvoked": True, "command": command,
        "output": str(output), "inputsSHA256": input_digest,
        "sourceInputsSHA256": source_digest, "platform": platform,
        "sdkIdentity": inputs["sdk"]["identity"], "publishable": not manual_dev,
        "reference": "github:" + repository + "#" + identifier + "@" + tag,
        "publicationInventorySHA256": digest(published),
        "outputs": payload_inventory(output / "built" / selection, selection),
    }
    (output / "build-receipt.json").write_text(json.dumps(result, indent=2) + "\n")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", choices=list(packages()))
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--sdk", required=True, type=Path)
    parser.add_argument("--sdk-source-manifest", required=True, type=Path)
    parser.add_argument("--published", required=True, type=Path, help="complete offline publication inventory")
    parser.add_argument("--previous", type=Path)
    parser.add_argument("--manual-dev", action="store_true", help="local/dev artifact only; never authorizes republishing")
    args = parser.parse_args()
    print(json.dumps(build_release(args.package, args.version, args.output, args.sdk,
                                  args.sdk_source_manifest, args.published, args.previous, args.manual_dev),
                     indent=2))


if __name__ == "__main__":
    main()
