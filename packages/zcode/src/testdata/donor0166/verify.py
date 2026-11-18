import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("stage", choices=["red", "covered", "focused", "unit", "all", "donor", "static",
                                          "causal", "donor-causal", "provenance"])
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--label", required=True)
    parser.add_argument("--count", type=int, default=1)
    args = parser.parse_args()
    root, source = args.root.resolve(), Path(__file__).resolve().parents[2]
    environment = {"HOME": os.environ["HOME"],
                   "PATH": str(root / "tools") + ":" + str(args.go.parent) +
                   ":/Users/maratkarimov/.bun/bin:/usr/local/bin:/usr/bin:/bin",
                   "LANG": "C.UTF-8", "TZ": "UTC"}
    environment.update(json.loads(subprocess.check_output(
        [str(args.go), "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"], env=environment)))
    isolation = runpy.run_path(str(source / "testdata/agent/verify.py"))["isolate"]()
    home = Path(tempfile.mkdtemp(prefix=args.label + "-home-", dir=root))
    environment.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                       XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                       XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                       LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=str(home),
                       GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
                       CGO_ENABLED="1")
    identity = json.loads((root / "sdk-source.json").read_bytes())

    def audit():
        for name, expected in identity["files"].items():
            path = root / "magpie" / name
            if "symlink" in expected:
                assert path.is_symlink() and os.readlink(path) == expected["symlink"], name
            else:
                assert hashlib.sha256(path.read_bytes()).hexdigest() == expected["sha256"], name
            if "mode" in expected:
                assert path.lstat().st_mode & 0o777 == expected["mode"], name
        return len(identity["files"])

    report = {"stage": args.stage, "isolation": isolation, "sdkSnapshotSHA256": identity["snapshotSHA256"],
              "sdkFilesBefore": audit(), "results": []}

    def run(label, command, cwd=source, expected=0, assertion=None):
        label = args.label + "-" + label
        log = root / (label + ".log")
        with log.open("wb") as output:
            process = subprocess.run(command, cwd=cwd, env=environment,
                                     stdout=output, stderr=subprocess.STDOUT, timeout=300)
        text = log.read_text()
        counts = dict.fromkeys(["pass", "fail", "skip"], 0)
        for line in text.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if event.get("Test") and event.get("Action") in counts:
                counts[event["Action"]] += 1
        if Path(command[0]).name == "bun":
            for action in counts:
                found = re.search(r"^\s*(\d+) " + action + r"\s*$", text, re.MULTILINE)
                if found:
                    counts[action] = int(found[1])
        result = {"label": label, "command": command, "cwd": str(cwd), "exit": process.returncode,
                  "expectedExit": expected, "counts": counts,
                  "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        report["results"].append(result)
        (root / (args.label + "-result.json")).write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(result), flush=True)
        assert process.returncode == expected, text[-10000:]
        assert "WARNING: DATA RACE" not in text, text[-10000:]
        if expected:
            assert counts["fail"] > 0 and "[build failed]" not in text and "SyntaxError" not in text, text[-10000:]
            if assertion:
                assert assertion in text, text[-10000:]
        if command[0] == str(args.go.parent / "gofmt"):
            assert not text, text

    if args.stage == "donor":
        run("oracles", ["bun", "test", "own.test.mjs", "gift.test.mjs", "inflight.test.mjs"],
            cwd=root / "donor/packages/zcode")
    elif args.stage == "provenance":
        run("source-lock", ["python3", "-I", "testdata/donor0166/provenance.py", str(root)])
    elif args.stage == "causal":
        mutations = [
            ("start-bypass", "gift.go",
             'if !trial && !giftOnly && account.Kind != "start" && !state.freeFirst {',
             'if account.Kind == "start" { return model, account, true, nil }; '
             'if !trial && !giftOnly && account.Kind != "start" && !state.freeFirst {',
             "^TestDonor0166ManualStart", "manual gift-only Start sent"),
            ("no-requeue", "start_guard.go", "if !requeue || turn >= 4 {",
             "if true || !requeue || turn >= 4 {", "^TestDonor0166StartBeside",
             "escaped instead of waiting"),
            ("no-hint", "gift.go", "if len(names) > 0 {", "if false && len(names) > 0 {",
             "^TestDonor0166MissingTrial", "wrong unclaimed Trial explanation"),
            ("no-hint-cache", "gift.go", "if !found || !now.Before(cached.until) {",
             "if true || !found || !now.Before(cached.until) {",
             "^TestDonor0166MissingTrial", "hint repeated cached reads"),
            ("no-front", "start_guard.go", "if front {", "if false && front {",
             "^TestDonor0166RequeueFront", "lost its place"),
            ("no-deadline", "start_guard.go", "if !time.Now().Before(until) {",
             "if false && !time.Now().Before(until) {",
             "^TestDonor0166RequeueFront", "expired requeue budget admitted"),
            ("trial-paid-admission", "module.go", "if credentials.Discovery && !trial {",
             "if credentials.Discovery {", "^TestDonor0166DiscoveredTrial",
             "lost the weekend explanation"),
        ]
        for label, name, before, after, pattern, assertion in mutations:
            original = (source / name).read_text()
            count = 2 if label == "no-deadline" else 1
            assert original.count(before) == count, label
            target = root / (label + ".go")
            target.write_text(original.replace(before, after, count))
            overlay = root / (label + "-overlay.json")
            overlay.write_text(json.dumps({"Replace": {str(source / name): str(target)}}))
            run(label, [str(args.go), "test", "-overlay", str(overlay), "-json", "-race",
                        "-count=1", "-timeout=1m", "-run", pattern, "."],
                expected=1, assertion=assertion)
    elif args.stage == "donor-causal":
        mutations = [
            ("cn-profile-order",
             'const families = [...new Set([active, "zai", "zai_cn", "bigmodel"].filter((x) => ["zai", "zai_cn", "bigmodel"].includes(x)))]',
             'const families = ["zai", "bigmodel"]',
             "own.test.mjs", "China sign-in|active .* identity"),
            ("no-requeue", "const REQUEUE_MAX = 4", "const REQUEUE_MAX = 0",
             "inflight.test.mjs", "two sessions on an account|requeued request whose"),
            ("no-hint", "const held = raw ? [] : await unclaimed(s)", "const held = []",
             "gift.test.mjs", "pick with no gift names the weekend"),
            ("no-exclusion", "if (names.length && !named(names)) {",
             "if (false && names.length && !named(names)) {",
             "gift.test.mjs", "model its balance doesn't name is not sent"),
            ("failed-plan-paid", "if (r?.start) start = true",
             "if (false && r?.start) start = true",
             "gift.test.mjs", "stays on the Start Plan when its plan list stops"),
        ]
        original = (root / "donor/packages/zcode/index.mjs").read_text()
        for label, before, after, test, pattern in mutations:
            assert original.count(before) == 1, label
            target = root / ("donor-causal-" + label)
            shutil.copytree(root / "donor/packages/zcode", target)
            (target / "index.mjs").write_text(original.replace(before, after, 1))
            run(label, ["bun", "test", test, "--test-name-pattern", pattern],
                cwd=target, expected=1, assertion="expect(")
    elif args.stage == "static":
        files = sorted(str(file.relative_to(source)) for file in source.rglob("*.go")
                       if not {".git", "dist"} & set(file.relative_to(source).parts))
        run("format", [str(args.go.parent / "gofmt"), "-l", *files])
        run("vet", [str(args.go), "vet", "./..."])
        run("python-syntax", ["python3", "-I", "-c",
                             "import ast,pathlib; [ast.parse(p.read_bytes()) for p in pathlib.Path('testdata/donor0166').glob('*.py')]"])
    else:
        pattern = "^TestDonor0166"
        if args.stage == "covered":
            pattern += "(China|Discovery)"
        elif args.stage == "unit":
            pattern = "^(TestDonor0166|TestStartGuard|TestGift|TestCompletionGiftOnly|TestCompletionConfirmedInactive|TestChina|TestSelectedStartRenewal)"
        run("module", [str(args.go), "test", "-json", "-race", "-count=" + str(args.count),
                       "-timeout=4m", "-run", "." if args.stage == "all" else pattern,
                       "./..." if args.stage == "all" else "."], expected=1 if args.stage == "red" else 0)
    report["sdkFilesAfter"] = audit()
    (root / (args.label + "-result.json")).write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
