import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("stage", choices=["red", "red-front", "red-provider", "red-gui", "red-presentation", "red-stamp", "stamp", "stamp-proposed", "owners", "front", "gui", "presentation", "module-red", "module", "adjacent", "static", "causal"])
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--label")
    parser.add_argument("--node-path")
    parser.add_argument("--browser-path")
    parser.add_argument("--source", type=Path)
    parser.add_argument("--module-source", type=Path)
    parser.add_argument("--count", type=int, default=1)
    parser.add_argument("--race", action="store_true")
    parser.add_argument("--bun", type=Path)
    parser.add_argument("--tsc", type=Path)
    parser.add_argument("--overlay", type=Path)
    args = parser.parse_args()
    root = args.root.resolve()
    source = (args.source or root / "magpie").resolve()
    module = Path(__file__).resolve().parents[2]
    spec = importlib.util.spec_from_file_location("isolation", module / "testdata/agent/verify.py")
    isolation = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(isolation)
    env = {"PATH": str(args.go.parent) + ":/usr/local/bin:/usr/bin:/bin",
           "HOME": os.environ["HOME"], "LANG": "C.UTF-8", "TZ": "UTC"}
    env.update(json.loads(subprocess.check_output(
        [str(args.go), "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"], env=env)))
    proof = isolation.isolate()
    home = Path(tempfile.mkdtemp(prefix=args.stage + "-home-", dir=root))
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
               XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
               LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
               GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
               PATH=str(args.go.parent) + ":" + env["PATH"])
    if args.node_path:
        env["NODE_PATH"] = args.node_path
    if args.browser_path:
        env["PLAYWRIGHT_BROWSERS_PATH"] = args.browser_path
    if args.bun:
        env["PATH"] = str(args.bun.parent) + ":" + env["PATH"]
    report = {"isolation": proof, "stage": args.stage, "results": []}
    if args.overlay:
        report["overlay"] = str(args.overlay)
        report["overlaySHA256"] = hashlib.sha256(args.overlay.read_bytes()).hexdigest()
    prefix = args.label or args.stage

    def run(label, command, cwd=source, expected=0, timeout=240):
        log = root / (label + ".log")
        with log.open("wb") as output:
            process = subprocess.run(command, cwd=cwd, env=env, stdout=output,
                                     stderr=subprocess.STDOUT, timeout=timeout)
        text = log.read_text()
        counts = dict.fromkeys(["pass", "fail", "skip"], 0)
        for line in text.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if event.get("Test") and event.get("Action") in counts:
                counts[event["Action"]] += 1
        if "node" in command[0]:
            for action in counts:
                match = re.search(r"^# " + action + r" (\d+)$", text, re.MULTILINE)
                if match:
                    counts[action] = int(match[1])
        if Path(command[0]).name == "bun":
            for action in counts:
                match = re.search(r"^\s*(\d+) " + action + r"\s*$", text, re.MULTILINE)
                if match:
                    counts[action] = int(match[1])
        record = {"label": label, "command": command, "exit": process.returncode,
                  "expectedExit": expected, "counts": counts,
                  "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        report["results"].append(record)
        (root / (prefix + "-result.json")).write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(record), flush=True)
        assert process.returncode == expected, text[-7000:]
        if expected and "-json" in command:
            assert counts["fail"] > 0 and "[build failed]" not in text, text[-7000:]
        elif expected:
            assert "ERR_ASSERTION" in text and re.search(r"^# fail [1-9]", text, re.MULTILINE), text[-7000:]
        assert "WARNING: DATA RACE" not in text, text[-7000:]

    go = str(args.go)
    expected = 1 if args.stage.startswith("red") or args.stage == "module-red" else 0
    test_flags = ["-count=" + str(args.count)]
    if args.race:
        test_flags.append("-race")
    if args.overlay:
        test_flags.extend(["-overlay", str(args.overlay)])
    if args.stage in {"red-front", "front"}:
        pattern = "^TestExtensionDefaultsFront"
        if args.stage == "red-front":
            pattern += "(Holds|Fanout)"
        run(prefix + "-front", [go, "test", "-tags", "nogui", "-json", *test_flags, "-timeout=8m", "-run",
            pattern, "./internal/gateway"], expected=expected, timeout=500)
        return
    if args.stage in {"red-gui", "gui"}:
        run(prefix + "-gui", ["node", "--test", "internal/gui/tests/extension-rich-quota.test.cjs"],
            expected=expected, timeout=240)
        return
    if args.stage in {"red-presentation", "presentation"}:
        run(prefix + "-presentation", ["node", "--test", "internal/gui/tests/extension-rich-quota-functions.test.cjs"],
            expected=expected)
        return
    if args.stage in {"module-red", "module"}:
        run(prefix + "-module", [go, "test", "-json", *test_flags, "-timeout=5m", "-run",
            "^TestQuotaMetadata" if args.stage == "module-red" else ".", "." if args.stage == "module-red" else "./..."],
            cwd=(args.module_source or module).resolve(), expected=expected, timeout=330)
        return
    if args.stage == "red-provider":
        run(prefix + "-provider", [go, "test", "-tags", "nogui", "-json", *test_flags, "-timeout=3m", "-run",
            "^TestExtensionRichQuota", "./internal/provider"], expected=1)
        return
    if args.stage in {"red-stamp", "stamp", "stamp-proposed"}:
        replacements = {str(source / "internal/extension/api9_account_stamp_probe_test.go"):
                        str(Path(__file__).parent / "account_stamp_probe_test.go.fixture")}
        if args.stage == "stamp-proposed":
            original = (source / "internal/extension/account_stamp.go").read_text()
            before = '\t\thash.Write([]byte(entry.Name()))\n\t\thash.Write([]byte{0})\n'
            assert original.count(before) == 1
            proposed = original.replace(before, "", 1).replace('\t\t\thash.Write([]byte{0})\n', "", 1)
            proposed = proposed.replace('\t\tfiles = append(files, accountInfo)\n',
                before + '\t\tfiles = append(files, accountInfo)\n', 1)
            target = root / (prefix + "-account-stamp.go")
            target.write_text(proposed)
            replacements[str(source / "internal/extension/account_stamp.go")] = str(target)
        overlay = root / (prefix + "-overlay.json")
        overlay.write_text(json.dumps({"Replace": replacements}))
        run(prefix + "-probe", [go, "test", "-tags", "nogui", "-overlay", str(overlay), "-json", *test_flags,
            "-timeout=3m", "-run", "^TestAccountStampProbe" if expected else "^TestAccountStamp",
            "./internal/extension"], expected=expected)
        if not expected:
            run(prefix + "-hold", [go, "test", "-tags", "nogui", "-overlay", str(overlay), "-json", *test_flags,
                "-timeout=3m", "-run", "^TestExtensionProviderCatalogChangesInvalidateHeldSnapshots$",
                "./internal/provider"])
        return
    if args.stage == "causal":
        mutations = [
            ("defaults", "internal/provider/extension_defaults.go",
             "return binding.descriptor.MaxConcurrency, binding.descriptor.MaxAccounts", "return nil, nil",
             "^TestExtensionDefaultsReachOrdinaryLimitsAndUserOverrides$", "./internal/provider"),
            ("config", "internal/provider/extension_models_cache.go",
             "entry.Manifest.Contract.Version.Minor >= 9", "entry.Manifest.Contract.Version.Minor >= 999",
             "^TestExtensionDefaultsRejectStaleSelection/config$", "./internal/provider"),
            ("queued-config", "internal/provider/extension_refresh.go",
             "binding.manifest.Contract.Version.Minor >= 9", "binding.manifest.Contract.Version.Minor >= 999",
             "^TestExtensionDefaultsFrontQueuedSelectionRevoked/config$", "./internal/gateway"),
            ("amount-projection", "internal/provider/extension_provider.go",
             "if window.Amount != nil {", "if false && window.Amount != nil {",
             "^TestExtensionRichQuotaProjectsWithoutInventingAllowance$", "./internal/provider"),
            ("amount-presence", "internal/provider/quota_amount.go",
             "presence == quotaAmountAbsent ||", "false ||",
             "^TestExtensionRichQuotaCountPresenceSurvivesSerialization$", "./internal/provider"),
            ("wallet-authority", "internal/provider/routing.go",
             "w.Balance != nil && !w.balanceInformational", "w.Balance != nil",
             "^TestExtensionRichQuotaProjectsWithoutInventingAllowance$", "./internal/provider"),
        ]
        for name, path, before, after, pattern, package in mutations:
            original = (source / path).read_text()
            assert original.count(before) == 1, (path, before)
            replacement = root / (prefix + "-" + name + ".go")
            replacement.write_text(original.replace(before, after, 1))
            overlay = root / (prefix + "-" + name + "-overlay.json")
            overlay.write_text(json.dumps({"Replace": {str(source / path): str(replacement)}}))
            run(prefix + "-" + name, [go, "test", "-tags", "nogui", "-overlay", str(overlay), "-json",
                "-count=1", "-timeout=3m", "-run", pattern, package], expected=1)
        return
    if args.stage == "adjacent":
        run(prefix + "-provider", [go, "test", "-tags", "nogui", "-json", *test_flags, "-timeout=5m", "-run",
            "^Test(Quota|KeptQuotaReport|NamedRemoteCards|WindowPace|WithWindowStarts|Wallet|PluginWallet|Allowance|Limit|Extension)",
            "./internal/provider"], timeout=330)
        run(prefix + "-gateway", [go, "test", "-tags", "nogui", "-json", *test_flags, "-timeout=5m", "-run",
            "^Test(MaxConcurrency|Lanes|ProviderConcurrency|LaneReleased|AccountConcurrency|ExtensionDefaults)",
            "./internal/gateway"], timeout=330)
        return
    if args.stage == "static":
        run(prefix + "-vet", [go, "vet", "-tags", "nogui", "./pkg/extension", "./internal/provider", "./internal/gateway"],
            timeout=330)
        run(prefix + "-gui-guards", [go, "test", "-tags", "nogui", "-json", "-timeout=3m", "-run",
            "^Test(CoreScript|AppJS|QuotaReset)", "./internal/gui"], timeout=210)
        return
    run(prefix + "-sdk", [go, "test", "-json", *test_flags, "-timeout=3m", "-run",
        "^Test(ProviderDefaultsRequireAPI9|RichQuota)", "./pkg/extension"], expected=expected)
    run(prefix + "-provider", [go, "test", "-tags", "nogui", "-json", *test_flags, "-timeout=3m", "-run",
        "^TestExtension(Defaults|RichQuota)", "./internal/provider"], expected=expected)
    run(prefix + "-js", ["node", "--test", "test/provider-defaults.test.js", "test/rich-quota.test.js"],
        cwd=source / "sdk/js", expected=expected)
    if args.bun:
        run(prefix + "-bun", [str(args.bun), "test", "test/provider-defaults.test.js", "test/rich-quota.test.js"],
            cwd=source / "sdk/js", expected=expected)
    if args.tsc:
        run(prefix + "-types", ["node", str(args.tsc), "--noEmit", "--strict", "--target", "ES2022",
            "--module", "NodeNext", "--moduleResolution", "NodeNext",
            "sdk/js/test/provider-defaults-types.ts", "sdk/js/test/rich-quota-types.ts"])


if __name__ == "__main__":
    main()
