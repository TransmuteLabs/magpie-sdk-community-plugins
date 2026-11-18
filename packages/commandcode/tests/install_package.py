#!/usr/bin/env python3
import hashlib
import json
from pathlib import Path
import subprocess
import sys


binary, distribution, expected = sys.argv[1:4]
assert hashlib.sha256(Path(binary).read_bytes()).hexdigest() == expected
wanted = json.loads((Path(distribution) / "magpie-plugin.json").read_text())
for arguments in (
    ["plugin", "sdk", "install", distribution],
    ["plugin", "sdk", "list", "--json"],
    ["plugin", "sdk", "inspect", wanted["id"]],
):
    result = subprocess.run([binary, *arguments], capture_output=True, text=True, timeout=90)
    if result.returncode:
        raise SystemExit(result.stderr)
    if "--json" in arguments:
        entries = json.loads(result.stdout)
        assert len(entries) == 1 and not entries[0]["disabled"]
        assert entries[0]["manifest"] == wanted
    if "inspect" in arguments:
        entry = json.loads(result.stdout)
        assert entry["manifest"] == wanted and not entry["disabled"]
        assert wanted["runtime"] == "native"
        for name, digest in entry["files"].items():
            assert hashlib.sha256((Path(distribution) / name).read_bytes()).hexdigest() == digest
print(json.dumps({"binarySHA256": expected, "version": wanted["version"],
    "singleInstalledNativeModule": True, "installedFilesMatched": True, "liveActivation": False}))
