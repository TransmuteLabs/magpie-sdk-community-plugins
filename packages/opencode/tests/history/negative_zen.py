#!/usr/bin/env python3
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

packet = Path(sys.argv[1]).resolve()
settings = json.loads(Path("/tmp/magpie-opencode-account-rugrjcv2/test-env.json").read_text())
home = Path(tempfile.mkdtemp(prefix="negative-home-", dir=packet))
env = dict(os.environ, **settings)
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home/"config"),
           XDG_CACHE_HOME=str(home/"cache"), XDG_DATA_HOME=str(home/"data"),
           XDG_STATE_HOME=str(home/"state"), GOFLAGS="-mod=readonly -p=4", GOMAXPROCS="4")
go = settings["PATH"].split(":")[0]+"/go"
cases = [
    ("public-tool-request", "zen_client.go", "TestZenPublicToolCompatibilityUsesSelectedNativeExecution",
     lambda text: text.replace('identity.Kind == "public" && request.Protocol != "gemini"', "false", 1)),
    ("public-tool-reply", "zen_tools.go", "TestZenPublicToolCompatibilityUsesSelectedNativeExecution",
     lambda text: text.replace("policy.renamed[name]", 'policy.renamed["no alias "+name]', 1)),
    ("public-tool-fragments", "zen_tool_stream.go", "TestZenToolNativePublicStreamRestoresCallerNames",
     lambda text: text.replace('target[key] = zenToolString(target[key]) + text', 'target[key] = text', 1)),
    ("empty-header", "console.go", "TestConsoleHeadersCachesAndWorkspaceAuthority",
     lambda text: re.sub(r"func headerPresent\(.*?\n}", 'func headerPresent(headers http.Header, name string) bool {\nreturn headers.Get(name) != ""\n}', text, count=1, flags=re.S)),
    ("truncated-success", "zen_client.go", "TestZenNativeProtocolsSignOnlySelectedCredentialsAndPreserveBodies",
     lambda text: text.replace("streaming && !terminal.complete()", "streaming && false", 1)),
    ("foreign-receipt", "console_watch.go", "TestConsoleWatchReceiptsDoNotGrantForeignOrRepeatedAcknowledgement",
     lambda text: text.replace("receipt.owner == consoleKey(call, identity)", "true", 1)),
    ("empty-catalog", "zen_models.go", "TestZenCatalogLiteralEligibilityAndSuccessfulEmptyRetirement",
     lambda text: text.replace("listing.Data != nil", "len(listing.Data) > 0", 1)),
    ("foreign-fixture-path", "console_test.go", "TestZenFixtureRejectsForeignPathsBeforeVendorState",
     lambda text: text.replace('if !strings.HasPrefix(request.URL.Path, prefix+"/")', "if false", 1)),
]
rows = []
for name, filename, test, mutate in cases:
    lane = Path(tempfile.mkdtemp(prefix="negative-"+name+"-", dir=packet))
    plugin = lane/"plugin"
    shutil.copytree(packet/"plugin", plugin, ignore=shutil.ignore_patterns("__pycache__"))
    (lane/"magpie").symlink_to(packet/"magpie", target_is_directory=True)
    source = plugin/filename
    before = source.read_text()
    after = mutate(before)
    assert after != before, "negative control did not change its target"
    source.write_text(after)
    result = subprocess.run([go,"test","-json","-count=1","-timeout=45s","-run","^"+test+"$","."],
                            cwd=plugin,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    (packet/("negative-"+name+".log")).write_text(result.stdout)
    failed = False
    for line in result.stdout.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        failed = failed or event.get("Action") == "fail" and event.get("Test") == test
    row = {"mutation":name,"exit":result.returncode,"namedBehaviorFailure":failed,
           "compileFailure": "[build failed]" in result.stdout}
    rows.append(row)
    print(json.dumps(row),flush=True)
    (packet/"negative-results.json").write_text(json.dumps(rows,indent=2)+"\n")
    assert result.returncode != 0 and failed and not row["compileFailure"], "negative did not fail the named behavior"
