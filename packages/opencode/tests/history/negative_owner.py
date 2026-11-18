#!/usr/bin/env python3
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

packet = Path(sys.argv[1]).resolve()
lane = Path(tempfile.mkdtemp(prefix="negative-owner-", dir=packet))
shutil.copytree(packet / "plugin", lane / "plugin", ignore=shutil.ignore_patterns("__pycache__"))
(lane / "magpie").symlink_to(packet / "magpie", target_is_directory=True)
source = lane / "plugin/testdata/full_e2e.py"
before = source.read_text()
target = '"usage": {"prompt_tokens": 5, "completion_tokens": 2}'
assert before.count(target) == 1
source.write_text(before.replace(target, '"usage": {"prompt_tokens": 0, "completion_tokens": 0}', 1))
settings = json.loads(Path("/tmp/magpie-opencode-account-rugrjcv2/test-env.json").read_text())
home = lane / "isolated-home"
home.mkdir()
env = dict(os.environ, **settings)
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
           XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
           XDG_STATE_HOME=str(home / "state"), GOFLAGS="-mod=readonly -p=4", GOMAXPROCS="4")
arguments = [str(packet / "loopback-guard"), str(lane / "network-guard.json"), "python3",
             str(source), str(packet / "magpie-host"), str(packet / "window-dist"), str(lane / "native")]
result = subprocess.run(arguments, cwd=lane / "plugin", env=env, text=True,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(lane / "terminal.log").write_text(result.stdout)
proof = {"mutation": "remove-native-go-usage", "exit": result.returncode,
         "namedBehaviorFailure": "ordinary owner ledger lost native usage" in result.stdout,
         "compileFailure": "[build failed]" in result.stdout, "lane": str(lane)}
(packet / "negative-owner-results.json").write_text(json.dumps(proof, indent=2) + "\n")
print(json.dumps(proof))
assert proof["exit"] != 0 and proof["namedBehaviorFailure"] and not proof["compileFailure"], "owner usage control failed for another reason"
