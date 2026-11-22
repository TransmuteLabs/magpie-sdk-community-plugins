#!/usr/bin/env python3
"""Select and package plugin versions for the macOS release workflow."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _workspace import ROOT, build_environment, effective_target, packages, read_json, sdk_source
from release import module_tag, package_release
from release_build import build_release, file_inputs, publication


REPOSITORY = "TransmuteLabs/magpie-sdk-community-plugins"
PUBLICATION_FIELDS = ("tag_name", "draft", "prerelease")


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


def write_inventory(path):
    releases = published_releases()
    path.write_text(json.dumps({
        "schema": 1, "repository": REPOSITORY, "complete": True, "releases": releases,
    }) + "\n")


def package_version(manifest, identifier):
    version = manifest["version"]
    module_tag(identifier, version)
    if manifest["name"] != identifier or re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) is None:
        raise ValueError(f"Expected a stable version for {identifier}")
    return version


def previous_version(git, before, name, identifier):
    path = f"packages/{name}/package.json"
    if subprocess.check_output([*git, "ls-tree", "--name-only", before, "--", path]):
        data = subprocess.check_output([*git, "show", f"{before}:{path}"])
        return package_version(json.loads(data), identifier)
    path = f"packages/{name}/docs/source-lock.json"
    if subprocess.check_output([*git, "ls-tree", "--name-only", before, "--", path]):
        data = subprocess.check_output([*git, "show", f"{before}:{path}"])
        previous = json.loads(data)["plugin"]
        return package_version({"name": previous["id"], "version": previous["version"]}, identifier)
    return None


def release_candidates(before=None, selection=None):
    candidates = []
    git = ["git", "-C", str(ROOT)]
    if before is not None and re.fullmatch(r"[0-9a-f]{40}", before) is None:
        raise ValueError("Expected the full revision before the push")
    if before == "0" * 40:
        before = None
    if before is not None:
        subprocess.run([*git, "cat-file", "-e", f"{before}^{{commit}}"], check=True)
    for name, spec in packages(selection or "all").items():
        path = f"packages/{name}/package.json"
        identifier = spec["id"]
        current = package_version(read_json(ROOT / path), identifier)
        if before is not None:
            previous = previous_version(git, before, name, identifier)
            if previous is not None:
                old = tuple(map(int, previous.split(".")))
                new = tuple(map(int, current.split(".")))
                if old == new:
                    continue
                if old > new:
                    raise ValueError(f"Package {name} version decreased from {previous} to {current}")
        candidates.append({"package": name, "id": identifier, "version": current, "tag": module_tag(identifier, current)})
    return candidates


def unchanged_release_inputs(before):
    if re.fullmatch(r"[0-9a-f]{40}", before) is None:
        raise ValueError("Expected the full revision before the change")
    if before == "0" * 40:
        return []
    git = ["git", "-C", str(ROOT)]
    problems = []
    for name, spec in packages().items():
        path = f"packages/{name}"
        metadata = f"{path}/package.json"
        if not subprocess.check_output([*git, "ls-tree", "--name-only", before, "--", metadata]):
            continue
        previous = package_version(json.loads(subprocess.check_output([*git, "show", f"{before}:{metadata}"])), spec["id"])
        current = package_version(read_json(ROOT / metadata), spec["id"])
        if tuple(map(int, current.split("."))) > tuple(map(int, previous.split("."))):
            continue
        if current != previous:
            raise ValueError(f"Package {name} version decreased from {previous} to {current}")
        inputs = [metadata, f"{path}/src", f"{path}/web", f"{path}/docs",
                  f"{path}/scripts", f"{path}/LICENSE", f"{path}/THIRD_PARTY_NOTICES"]
        if name == "commandcode":
            inputs.append(f"{path}/README.md")
        changed = subprocess.check_output([
            *git, "diff", "--no-renames", "--name-only", "-z", before, "HEAD", "--",
            *inputs,
        ]).decode().split("\0")
        shipped = [
            item for item in changed if item
            and not item.endswith(("_test.go", "_test.py", ".test.mjs", ".test.cjs"))
            and not (item.endswith(".py") and Path(item).name.startswith("test_"))
        ]
        if shipped:
            problems.append(f"{name}@{current}: {', '.join(shipped)}")
    return problems


def unpublished(candidates, inventory):
    tags = {release["tag_name"] for release in read_json(inventory)["releases"]}
    pending = []
    for item in candidates:
        _, exists = publication(inventory, item["id"], item["version"])
        if not exists:
            if item["tag"] in tags:
                raise ValueError(f"A draft already exists for {item['tag']}")
            pending.append(item)
    return pending


def build_candidate(selection, version, sdk, sdk_commit, inventory, output):
    candidates = release_candidates(selection=selection)
    if candidates[0]["version"] != version:
        raise ValueError("The requested version differs from the checked out package version")
    if not unpublished(candidates, inventory):
        return False
    package = ROOT / "packages" / selection
    provenance = read_json(package / "docs/source-lock.json")["plugin"]
    if provenance["id"] != candidates[0]["id"] or provenance["version"] != version:
        raise ValueError("Package version and source provenance disagree")
    target, _ = effective_target("go", package)
    if target != "darwin/arm64":
        raise ValueError(f"Expected macOS arm64, got {target}")
    sdk_checkout = sdk_source(sdk)
    with tempfile.TemporaryDirectory(prefix="magpie-release-") as temporary:
        scratch = Path(temporary)
        sdk_snapshot = scratch / "sdk"
        index = scratch / "sdk-source.json"
        build = scratch / "candidate"
        if seal_sdk(sdk_checkout, sdk_snapshot, index) != sdk_commit:
            raise ValueError("The SDK checkout differs from the pinned revision")
        with build_environment(package, sdk_snapshot) as (env, _):
            env.update(GOPROXY="https://proxy.golang.org", GOSUMDB="sum.golang.org")
            subprocess.run(["go", "list", "-deps", "-test", "./..."], cwd=package / "src",
                           stdout=subprocess.DEVNULL, env=env, check=True)
        subprocess.run([sys.executable, str(ROOT / "scripts/check.py"), selection,
                        "--sdk", str(sdk_snapshot), "--race"], check=True)
        build_release(selection, version, build, sdk_snapshot, index, inventory)
        distribution = build / "built" / selection
        entry = read_json(distribution / "magpie-plugin.json")["entry"]
        subprocess.run(["lipo", str(distribution / entry), "-verify_arch", "arm64"], check=True)
        package_release(selection, distribution.parent, output, "darwin_arm64")
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    plan = commands.add_parser("plan")
    source = plan.add_mutually_exclusive_group(required=True)
    source.add_argument("--before")
    source.add_argument("--package", choices=list(packages()))
    plan.add_argument("--inventory", type=Path)
    inventory = commands.add_parser("inventory")
    inventory.add_argument("output", type=Path)
    bumped = commands.add_parser("bumped")
    bumped.add_argument("before")
    build = commands.add_parser("build")
    build.add_argument("package", choices=list(packages()))
    build.add_argument("--version", required=True)
    build.add_argument("--sdk", type=Path, required=True)
    build.add_argument("--sdk-commit", required=True)
    build.add_argument("--inventory", type=Path, required=True)
    build.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.command == "bumped":
            problems = unchanged_release_inputs(args.before)
            if problems:
                raise ValueError("Package inputs changed without a version bump:\n" + "\n".join(problems))
            print("Every changed release input has a new package version")
            return
        if args.command == "inventory":
            write_inventory(args.output)
            return
        if args.command == "plan":
            pending = release_candidates(before=args.before, selection=args.package)
            if pending and args.inventory is not None:
                pending = unpublished(pending, args.inventory)
            elif pending:
                with tempfile.TemporaryDirectory(prefix="magpie-release-list-") as temporary:
                    inventory = Path(temporary) / "publications.json"
                    write_inventory(inventory)
                    pending = unpublished(pending, inventory)
            outputs = {
                "matrix": json.dumps({"include": pending}, separators=(",", ":")),
                "has-releases": str(bool(pending)).lower(),
            }
        else:
            built = build_candidate(args.package, args.version, args.sdk, args.sdk_commit, args.inventory, args.output)
            outputs = {"built": str(built).lower()}
        with Path(os.environ["GITHUB_OUTPUT"]).open("a") as stream:
            for key, value in outputs.items():
                print(f"{key}={value}", file=stream)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError, tarfile.TarError) as error:
        parser.exit(1, f"{error}{os.linesep}")


if __name__ == "__main__":
    main()
