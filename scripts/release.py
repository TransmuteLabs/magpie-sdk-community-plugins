#!/usr/bin/env python3
"""Package existing distributions as deterministic module-scoped release assets."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import stat
import sys
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _workspace import packages, read_json, validate_distribution


def module_tag(identifier, version):
    if re.fullmatch(r"[a-z0-9][a-z0-9_.-]{0,127}", identifier) is None:
        raise ValueError("Invalid module identity")
    match = re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?", version)
    if match is None:
        raise ValueError("Invalid module semantic version")
    for value, prerelease in ((match[4], True), (match[5], False)):
        if value is not None:
            for part in value.split("."):
                if not part or prerelease and part.isdigit() and len(part) > 1 and part.startswith("0"):
                    raise ValueError("Invalid module semantic version identifier")
    return identifier + "-v" + version


def module_release_version(identifier, tag):
    prefix = identifier + "-v"
    if not tag.startswith(prefix):
        return None
    version = tag[len(prefix):]
    try:
        module_tag(identifier, version)
    except ValueError as selected_error:
        remaining = tag
        while "-v" in remaining:
            sibling, _, _ = remaining.rpartition("-v")
            if len(sibling) <= len(identifier):
                break
            try:
                module_tag(sibling, tag[len(sibling) + 2:])
                return None
            except ValueError:
                remaining = sibling
        raise selected_error
    return version


def payload_files(directory):
    if directory.is_symlink() or not directory.is_dir():
        raise ValueError("Distribution must be a regular directory")
    files, spellings, total = [], {}, 0
    for path in sorted(directory.rglob("*")):
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode) or not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
            raise ValueError("Distribution contains a symlink or special file")
        name = path.relative_to(directory).as_posix()
        if len(name) > 1024 or any(character in name for character in "\\:") or any(ord(character) < 32 or ord(character) == 127 for character in name):
            raise ValueError("Distribution has an unsafe archive path")
        if name.lower() in spellings:
            raise ValueError("Distribution has case-colliding paths")
        spellings[name.lower()] = name
        if stat.S_ISREG(info.st_mode):
            files.append((name, path, info))
            total += info.st_size
            if info.st_size > 64 << 20 or total > 256 << 20:
                raise ValueError("Distribution exceeds installer size limits")
    if not files or len(files) > 4096:
        raise ValueError("Distribution exceeds installer file limits")
    return files


def payload_inventory(directory, selection):
    inventory = {}
    for name, path, info in payload_files(directory):
        with path.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
        inventory["built/" + selection + "/" + name] = {
            "sha256": digest, "mode": stat.S_IMODE(info.st_mode),
        }
    return inventory


def validate_build_receipt(receipt, selection, identifier, version, directory, platform=None):
    if not isinstance(receipt, dict) or receipt.get("schema") != 2:
        raise ValueError("Build receipt requires schema 2 with complete payload hashes and modes")
    if receipt.get("publishable") is not True or receipt.get("status") not in ("built-release-candidate", "reuse-local"):
        raise ValueError("Build receipt is not publishable; manual-dev artifacts cannot become release assets")
    target = receipt.get("platform")
    if not isinstance(target, str) or re.fullmatch(r"[a-z0-9]{1,32}/[a-z0-9]{1,32}", target) is None:
        raise ValueError("Build receipt requires its effective build platform")
    if platform not in (None, "any") and target != platform.replace("_", "/"):
        raise ValueError("Requested archive platform differs from the build receipt")
    if (receipt.get("id"), receipt.get("version"), receipt.get("tag")) != (
        identifier, version, module_tag(identifier, version)
    ):
        raise ValueError("Build receipt belongs to another module or version")
    if not isinstance(receipt.get("output"), str) or not Path(receipt["output"]).is_absolute():
        raise ValueError("Build receipt requires its original absolute output directory")
    expected = Path(receipt["output"]) / "built" / selection
    if expected.is_symlink() or expected.resolve() != directory.resolve():
        raise ValueError("Build receipt belongs to another distribution directory")
    if not isinstance(receipt.get("outputs"), dict) or not receipt["outputs"]:
        raise ValueError("Build receipt payload inventory is missing")
    if payload_inventory(directory, selection) != receipt["outputs"]:
        raise ValueError("Prior local artifact changed: complete payload hashes or modes differ")


def package_release(selection, inputs, output, platform, receipt=None):
    if selection == "all":
        raise ValueError("A release selects exactly one module")
    if platform != "any" and re.fullmatch(r"[a-z0-9]{1,32}_[a-z0-9]{1,32}", platform) is None:
        raise ValueError("Platform must be <os>_<arch> or any")
    inputs, output = Path(inputs).resolve(), Path(output).absolute()
    if output.exists() or output.is_symlink():
        raise ValueError("Release output already exists; choose a fresh directory")
    output = output.resolve()
    if output.is_relative_to(inputs):
        raise ValueError("Release output must be outside the input distributions")
    receipt_path = Path(receipt) if receipt is not None else inputs.parent / "build-receipt.json"
    if receipt_path.is_symlink() or not receipt_path.is_file():
        raise ValueError("Release packaging requires a publishable build receipt; plain build.py outputs are not release candidates")
    build_receipt = read_json(receipt_path)
    selected, identities = [], set()
    for name, spec in packages(selection).items():
        identifier = spec["id"]
        if re.fullmatch(r"[a-z0-9][a-z0-9_.-]{0,127}", identifier) is None or identifier in identities:
            raise ValueError("Registry has an invalid or duplicate module identity")
        identities.add(identifier)
        directory = inputs / name
        if directory.is_symlink() or not directory.is_dir():
            raise ValueError("Distribution must be a regular directory")
        files = payload_files(directory)
        manifest = validate_distribution(directory, identifier)
        tag = module_tag(identifier, manifest["version"])
        validate_build_receipt(build_receipt, name, identifier, manifest["version"], directory, platform)
        supported = manifest.get("compatibility", {}).get("platforms")
        if supported is not None and (not isinstance(supported, list) or platform.replace("_", "/") not in supported):
            raise ValueError("Distribution does not support the requested platform")
        if platform == "any" and (manifest["runtime"] != "bun" or any(
            service["runtime"] != "bun" for service in manifest.get("services", [])
        )):
            raise ValueError("Universal archives require Bun for every component")
        selected.append((name, identifier, manifest["version"], tag, files))
    output.mkdir(parents=True, exist_ok=False)
    checksums, result = [], []
    for name, identifier, version, tag, files in selected:
        asset = "magpie-plugin_" + identifier + "__" + platform + ".zip"
        archive = output / asset
        with zipfile.ZipFile(archive, "x", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as zipped:
            for relative, path, info in files:
                header = zipfile.ZipInfo(relative, date_time=(1980, 1, 1, 0, 0, 0))
                header.create_system = 3
                mode = 0o755 if info.st_mode & 0o111 else 0o644
                header.external_attr = (stat.S_IFREG | mode) << 16
                zipped.writestr(header, path.read_bytes(), compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
        if archive.stat().st_size > 256 << 20:
            raise ValueError("Release archive exceeds installer size limit")
        with archive.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
        checksums.append((asset, digest))
        result.append({"package": name, "id": identifier, "version": version, "tag": tag,
                       "asset": asset, "sha256": digest})
    with (output / "checksums.txt").open("x") as stream:
        stream.write("".join(digest + "  " + name + "\n" for name, digest in sorted(checksums)))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", choices=list(packages()))
    parser.add_argument("--input", required=True, type=Path, help="parent of existing built package distributions")
    parser.add_argument("--output", required=True, type=Path, help="fresh release asset directory outside input")
    parser.add_argument("--platform", required=True, help="<os>_<arch>, or any for Bun-only distributions")
    parser.add_argument("--receipt", type=Path, help="publishable build receipt; defaults to the parent of --input")
    args = parser.parse_args()
    for item in package_release(args.package, args.input, args.output, args.platform, args.receipt):
        print(json.dumps(item), flush=True)


if __name__ == "__main__":
    main()
