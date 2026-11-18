"""Validate source provenance and project it onto a selected distribution."""

import copy
import json
from pathlib import Path
import re
import subprocess


def _object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate provenance field")
        result[key] = value
    return result


def _read(path):
    return json.loads(Path(path).read_text(), object_pairs_hook=_object)


def _identity(manifest):
    version = manifest["contract"]["version"]
    capabilities = manifest["contract"]["capabilities"]
    minimum = str(version["major"]) + "." + str(version["minor"])
    if "request-policy" in capabilities and (version["major"], version["minor"]) < (1, 12):
        raise ValueError("request-policy requires SDK API 1.12 or later")
    if manifest["compatibility"]["hostAPI"] != "^" + minimum:
        raise ValueError("manifest host API does not match its SDK minimum")
    return {"id": manifest["id"], "version": manifest["version"],
            "sdkMinimum": minimum, "capabilities": capabilities,
            "nativeABI": manifest["compatibility"]["nativeABI"]}


def source_lock(source, manifest_name="magpie-agent.json"):
    source = Path(source)
    lock = _read(source / "docs/source-lock.json")
    if lock.get("schema") != 1 or lock.get("plugin") != _identity(_read(source / "src/magpie-agent.json")):
        raise ValueError("source lock identity/version does not match the default manifest")
    if not (source / "docs/SOURCES.md").is_file():
        raise ValueError("source guide is missing")
    manifests = {"src/magpie-plugin.json", "src/magpie-inbound.json", "src/magpie-agent.json"}
    variants = lock.get("limits", {}).get("variants", [])
    if len(variants) != 3 or {item.get("manifest") for item in variants} != manifests:
        raise ValueError("source lock must describe every distribution variant")
    for variant in variants:
        expected = _identity(_read(source / variant["manifest"]))
        if any(variant.get(key) != expected[key] for key in ("version", "sdkMinimum", "nativeABI", "capabilities")):
            raise ValueError("source lock variant version/SDK/ABI/capabilities is stale")
    ids = set()
    for item in lock.get("sources", []):
        identifier = item.get("id")
        if not identifier or identifier in ids or not item.get("repository") or not item.get("role") or not item.get("paths"):
            raise ValueError("source declaration is missing or ambiguous")
        if not re.fullmatch(r"[0-9a-f]{40}", item.get("commit", "")):
            raise ValueError("source reference must be an immutable full commit")
        if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", item.get("checkedAt", "")):
            raise ValueError("source check date is missing")
        ids.add(identifier)
        for artifact in item.get("artifacts", []):
            if not artifact.get("path") or not re.fullmatch(r"[0-9a-f]{64}", artifact.get("sha256", "")):
                raise ValueError("source artifact needs an exact SHA256")
    if not {"sdk", "devinapp", "community-devin", "cliproxy-maintained", "codec-extraction"} <= ids:
        raise ValueError("required provenance source is missing")
    for watch in lock.get("watch", []):
        if watch.get("source") not in ids or not watch.get("paths") or not watch.get("behaviors") or not watch.get("tests"):
            raise ValueError("source watch lacks its behavior/test mapping")
        for name in watch["tests"]:
            path = Path(name)
            if path.is_absolute() or ".." in path.parts or not (source / path).is_file():
                raise ValueError("required watched regression/input is missing")
    if "src/" + manifest_name not in manifests:
        raise ValueError("unknown provenance distribution")
    result = copy.deepcopy(lock)
    result["plugin"] = _identity(_read(source / "src" / manifest_name))
    return result


def check_tracked_inputs(source):
    source = Path(source)
    lock = source_lock(source)
    required = {"docs/SOURCES.md", "docs/source-lock.json", "scripts/build.py",
                "scripts/provenance.py", "tests/provenance_test.py",
                "src/magpie-plugin.json", "src/magpie-inbound.json", "src/magpie-agent.json"}
    for watch in lock["watch"]:
        required.update(watch["tests"])
    result = subprocess.run(["git", "ls-files", "--error-unmatch", "--", *sorted(required)],
                            cwd=source, capture_output=True, text=True)
    if result.returncode:
        raise ValueError("required provenance/source regression inputs are not tracked")
    return sorted(required)
