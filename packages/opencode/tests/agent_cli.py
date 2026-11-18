#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys
import urllib.request

if any(arg in sys.argv for arg in ("--version", "-v")):
    print("2.3.0")
    raise SystemExit(0)
if any(arg in sys.argv for arg in ("--help", "-h")):
    print("Synthetic OpenCode CLI fixture")
    raise SystemExit(0)
config = json.loads(Path(os.environ["OPENCODE_CONFIG"]).read_text())
args = sys.argv[1:]
model = args[args.index("--model") + 1]
assert model == config["model"] and model.startswith("magpie/")
provider = config["providers"]["magpie"]
endpoint = provider["settings"]["baseURL"] + "/chat/completions"
body = json.dumps({"model": model.removeprefix("magpie/"), "messages": [{"role": "user", "content": "launched-agent"}]}).encode()
request = urllib.request.Request(endpoint, body, {"Content-Type": "application/json", "Authorization": "Bearer " + provider["settings"]["apiKey"]})
with urllib.request.urlopen(request, timeout=15) as response:
    result = json.loads(response.read())
assert result["choices"][0]["message"]["content"] == "synthetic-go"
instructions = Path(os.environ["XDG_CONFIG_HOME"]) / "opencode" / "AGENTS.md"
print(json.dumps({"usedCheckedOverlay": True, "model": model,
                  "libraryInstructions": instructions.exists() and "SDK OpenCode Library fixture" in instructions.read_text(),
                  "ordinaryGateway": True}))
