#!/usr/bin/env python3
"""Build and package one unpublished plugin version on a Mac."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tarfile
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _workspace import ROOT, effective_target, packages, read_json, sdk_source
from release import module_tag, package_release
from release_build import build_release, file_inputs, publication


REPOSITORY = "TransmuteLabs/magpie-sdk-community-plugins"
PUBLICATION_FIELDS = ("tag_name", "draft", "prerelease")
BUILD_SOURCES = ("registry.json", "scripts/build.py", "scripts/_workspace.py",
                 "scripts/release.py", "scripts/release_build.py", "scripts/release_local.py")


def published_releases():
    pages = json.loads(subprocess.check_output(
        ["gh", "api", "--hostname", "github.com", "--paginate", "--slurp",
         f"repos/{REPOSITORY}/releases?per_page=100"], text=True,
    ))
    if not isinstance(pages, list) or not pages or any(not isinstance(page, list) for page in pages):
        raise ValueError("GitHub returned an incomplete release listing")
    return [{field: release[field] for field in PUBLICATION_FIELDS} for page in pages for release in page]


def seal_sdk(source, destination, index):
    git = ["git", "-C", str(source)]
    commit = subprocess.check_output([*git, "rev-parse", "--verify", "HEAD^{commit}"], text=True).strip()
    destination.mkdir()
    with tempfile.TemporaryFile() as archive:
        subprocess.run([*git, "archive", commit], stdout=archive, check=True)
        archive.seek(0)
        with tarfile.open(fileobj=archive) as snapshot:
            snapshot.extractall(destination, filter="data")
    files = file_inputs(destination)
    identity = hashlib.sha256(json.dumps(files, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    index.write_text(json.dumps({"commit": commit, "identity": identity, "files": files}, sort_keys=True) + "\n")
    return commit


def plugin_commit(selection):
    git = ["git", "-C", str(ROOT)]
    commit = subprocess.check_output([*git, "rev-parse", "--verify", "HEAD^{commit}"], text=True).strip()
    paths = [f"packages/{selection}", *BUILD_SOURCES]
    status = subprocess.check_output(
        [*git, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching", "--", *paths],
    )
    for record in filter(None, status.split(b"\0")):
        path = Path(os.fsdecode(record[3:]))
        if record.startswith(b"!! ") and "__pycache__" in path.parts:
            continue
        raise ValueError(f"Commit or move aside release input {path} before building")
    return commit


def prepare_release(selection, output=None, sdk=None, previous=None):
    spec = packages(selection)[selection]
    package = ROOT / "packages" / selection
    lock = read_json(package / "docs/source-lock.json")["plugin"]
    identifier, version = spec["id"], lock["version"]
    if lock["id"] != identifier:
        raise ValueError("Selected package differs from its source provenance")
    tag = module_tag(identifier, version)
    with tempfile.TemporaryDirectory(prefix="magpie-release-") as temporary:
        scratch = Path(temporary)
        inventory = scratch / "publications.json"
        releases = published_releases()
        inventory.write_text(json.dumps({
            "schema": 1, "repository": REPOSITORY, "complete": True, "releases": releases,
        }, sort_keys=True) + "\n")
        _, published = publication(inventory, identifier, version)
        if published:
            return {"status": "skip-published", "tag": tag}
        if any(item["tag_name"] == tag for item in releases):
            raise ValueError(f"A draft already exists for {tag}")
        source_commit = plugin_commit(selection)
        arch = {"arm64": "arm64", "x86_64": "amd64"}.get(platform.machine())
        if sys.platform != "darwin" or arch is None:
            raise ValueError("Release assets must be built on a supported Mac")
        target, _ = effective_target("go", package)
        if target != "darwin/" + arch:
            raise ValueError(f"Go target {target} differs from the local Mac")
        output = Path(output or ROOT / "dist/releases" / tag).absolute()
        if output.exists() or output.is_symlink():
            raise ValueError("Release output already exists; choose a fresh directory")
        sdk_checkout = sdk_source(sdk)
        if output.resolve().is_relative_to(sdk_checkout) or output.resolve().is_relative_to(package):
            raise ValueError("Release output must be outside the selected plugin and SDK sources")
        sdk_snapshot = scratch / "sdk"
        index = scratch / "sdk-source.json"
        commit = seal_sdk(sdk_checkout, sdk_snapshot, index)
        receipt = build_release(selection, version, output / "candidate", sdk_snapshot, index,
                                inventory, previous=previous)
        if plugin_commit(selection) != source_commit:
            raise ValueError("Plugin HEAD changed during the release build")
        assets = package_release(selection, Path(receipt["output"]) / "built", output / "assets",
                                 target.replace("/", "_"))
        result = {"status": receipt["status"], "pluginCommit": source_commit, "sdkCommit": commit,
                  "tag": tag, "output": str(output / "assets"), "assets": assets}
        (output / "release.json").write_text(json.dumps(result, indent=2) + "\n")
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", choices=list(packages()))
    parser.add_argument("--sdk", type=Path, help="use the committed HEAD of this Magpie checkout")
    parser.add_argument("--output", type=Path, help="fresh directory (default: dist/releases/<module tag>)")
    parser.add_argument("--previous", type=Path, help="optional prior local build receipt")
    args = parser.parse_args()
    try:
        print(json.dumps(prepare_release(args.package, args.output, args.sdk, args.previous), indent=2), flush=True)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError, tarfile.TarError) as error:
        parser.exit(1, f"{error}{os.linesep}")


if __name__ == "__main__":
    main()
