"""Copy reviewed provenance with the selected distribution's public identity."""

import hashlib
import json
from pathlib import Path
import shutil
import subprocess


def snapshot_build_inputs(go, module, arguments, target, env=None):
    output = subprocess.check_output(
        [go, "list", *arguments, "-deps", "-json", target],
        cwd=module, env=env, text=True,
    )
    decoder = json.JSONDecoder()
    files, modules = {}, {}
    while output.strip():
        package, end = decoder.raw_decode(output.lstrip())
        output = output.lstrip()[end:]
        dependency = package.get("Module")
        if not dependency:
            continue
        identity = dependency["Path"]
        resolved = dependency.get("Replace", dependency)
        directory = Path(resolved["Dir"]).resolve()
        modules[identity] = {
            "version": dependency.get("Version", "(devel)"),
            "replacement": "local" if "Replace" in dependency else None,
        }
        paths = []
        for field in ("GoFiles", "CgoFiles", "CFiles", "CXXFiles", "HFiles",
                      "FFiles", "SFiles", "SysoFiles", "EmbedFiles"):
            paths.extend(Path(package["Dir"]) / name for name in package.get(field, []))
        paths.extend(path for path in (directory / "go.mod", directory / "go.sum") if path.is_file())
        for path in paths:
            relative = path.resolve().relative_to(directory).as_posix()
            files[identity + "/" + relative] = hashlib.sha256(path.read_bytes()).hexdigest()
    sdk_identity = "github.com/yetone/magpie"
    module_identity = "github.com/TransmuteLabs/magpie-sdk-plugin-session-omp"
    if sdk_identity not in modules or module_identity not in modules:
        raise ValueError("build did not resolve both module and SDK inputs")
    package = Path(module).parent
    for name in ("scripts/build.py", "scripts/build_source_preview.py", "scripts/provenance.py",
                 "docs/source-lock.json", "docs/SOURCES.md"):
        files[module_identity + "/" + name] = hashlib.sha256((package / name).read_bytes()).hexdigest()
    workspace_helper = package.parents[1] / "scripts/_workspace.py"
    files["workspace/scripts/_workspace.py"] = hashlib.sha256(workspace_helper.read_bytes()).hexdigest()
    inputs = {"modules": modules, "files": files}
    inputs["sha256"] = hashlib.sha256(
        json.dumps(inputs, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    return inputs


def write_provenance(module, destination, manifest, inputs, toolchain):
    module, destination = Path(module), Path(destination)
    documentation = module.parent / "docs"
    lock = json.loads((documentation / "source-lock.json").read_text())
    version = manifest["contract"]["version"]
    lock["plugin"] = {
        "id": manifest["id"],
        "version": manifest["version"],
        "sdkMinimum": f"{version['major']}.{version['minor']}",
        "nativeABI": manifest["compatibility"]["nativeABI"],
    }
    (destination / "source-lock.json").write_text(json.dumps(lock, indent=2) + "\n")
    shutil.copyfile(documentation / "SOURCES.md", destination / "SOURCES.md")
    entry = destination / manifest["entry"]
    evidence = {
        "schema": 1,
        "plugin": lock["plugin"],
        "toolchain": toolchain,
        "inputs": inputs,
        "entry": {
            "path": entry.name,
            "sha256": hashlib.sha256(entry.read_bytes()).hexdigest(),
        },
        "manifestSHA256": hashlib.sha256((destination / "magpie-plugin.json").read_bytes()).hexdigest(),
    }
    (destination / "build-provenance.json").write_text(json.dumps(evidence, indent=2) + "\n")
