"""Shared author-tool paths and temporary SDK selection; no runtime dependency."""
from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def read_json(path):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise ValueError("Duplicate JSON key")
            value[key] = item
        return value
    return json.loads(Path(path).read_text(), object_pairs_hook=unique)


def packages(selection="all"):
    items = json.loads((ROOT / "registry.json").read_text())["packages"]
    if selection == "all":
        return items
    if selection not in items:
        raise ValueError(f"Unknown package: {selection}")
    return {selection: items[selection]}


def select_build_packages(items, selection, target):
    if not isinstance(target, str) or re.fullmatch(r"[a-z0-9]{1,32}/[a-z0-9]{1,32}", target) is None:
        raise ValueError("Invalid effective build target")
    if selection != "all" and selection not in items:
        raise ValueError(f"Unknown package: {selection}")
    selected, skipped = {}, {}
    for name, spec in items.items():
        if selection != "all" and selection != name:
            continue
        if "platforms" in spec:
            targets = spec["platforms"]
            if (not isinstance(targets, list) or not targets or
                    any(not isinstance(value, str) or re.fullmatch(r"[a-z0-9]{1,32}/[a-z0-9]{1,32}", value) is None
                        for value in targets) or len(set(targets)) != len(targets)):
                raise ValueError(f"Invalid build platforms for package {name}")
            if target not in targets:
                if selection != "all":
                    raise ValueError(f"Package {name} does not support build target {target}; supported targets: {', '.join(targets)}")
                skipped[name] = spec
                continue
        selected[name] = spec
    return selected, skipped


def effective_target(go, package):
    environment = os.environ.copy()
    environment.update(GOWORK="off", GOTOOLCHAIN="local", GOPROXY="off",
                       GOSUMDB="off", GOFLAGS="-mod=readonly")
    target = json.loads(subprocess.check_output(
        [str(go), "env", "-json", "GOOS", "GOARCH"],
        cwd=package / "src", env=environment, text=True,
    ))
    for name in ("GOOS", "GOARCH"):
        if not isinstance(target.get(name), str) or re.fullmatch(r"[a-z0-9]{1,32}", target[name]) is None:
            raise ValueError("Go did not identify an effective build target")
    environment.update(GOOS=target["GOOS"], GOARCH=target["GOARCH"])
    return target["GOOS"] + "/" + target["GOARCH"], environment


def validate_distribution(directory, expected_id):
    directory = Path(directory).resolve()
    manifest = json.loads((directory / "magpie-plugin.json").read_text())
    lock = json.loads((directory / "source-lock.json").read_text())["plugin"]
    if manifest["id"] != expected_id or lock["id"] != expected_id:
        raise ValueError("Distribution has the wrong plugin identity")
    if manifest["version"] != lock["version"]:
        raise ValueError("Distribution and provenance versions differ")
    version = manifest["contract"]["version"]
    if lock["sdkMinimum"] != f'{version["major"]}.{version["minor"]}':
        raise ValueError("Distribution and provenance SDK requirements differ")
    entry = (directory / manifest["entry"]).resolve()
    if not entry.is_relative_to(directory) or not entry.is_file():
        raise ValueError("Distribution entry is missing or outside its package")
    return manifest


def sdk_source(explicit=None):
    value = explicit or os.environ.get("MAGPIE_SDK_SOURCE") or ROOT.parent / "magpie"
    sdk = Path(value).expanduser().resolve()
    if not (sdk / "pkg/extension/contract.go").is_file():
        raise ValueError(f"Not a Magpie SDK checkout: {sdk}")
    return sdk


@contextmanager
def build_environment(package, sdk=None):
    """Select an SDK without writing go.mod/go.sum or downloading dependencies."""
    package = Path(package).resolve()
    selected = sdk_source(sdk)
    source = package / "src"
    data = (source / "go.mod").read_text()
    replacement = "replace github.com/yetone/magpie => " + json.dumps(str(selected))
    data, count = re.subn(r"(?m)^replace github\.com/yetone/magpie => .*?$", lambda _: replacement, data)
    if count != 1:
        raise ValueError("Expected exactly one explicit Magpie SDK replacement")
    with tempfile.TemporaryDirectory(prefix="magpie-plugin-build-") as directory:
        modfile = Path(directory) / "plugin.mod"
        modfile.write_text(data)
        shutil.copyfile(source / "go.sum", modfile.with_suffix(".sum"))
        env = os.environ.copy()
        env.update(GOWORK="off", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local",
                   CGO_ENABLED="1", MAGPIE_SDK_SOURCE=str(selected),
                   GOFLAGS="-mod=readonly -buildvcs=false " + json.dumps("-modfile=" + str(modfile)))
        yield env, selected
