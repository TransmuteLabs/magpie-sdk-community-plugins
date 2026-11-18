import argparse
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("label")
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--run", default=".")
    parser.add_argument("--count", type=int, default=1)
    parser.add_argument("--red", action="store_true")
    parser.add_argument("--all", action="store_true")
    parser.add_argument("--stage", choices=["tests", "static", "causal", "default-package"], default="tests")
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    root = args.root.resolve()
    assert root != source and root.is_dir()
    go = str(args.go.resolve())
    env = {"PATH": str(args.go.parent) + ":" + os.environ["PATH"],
           "HOME": os.environ["HOME"], "LANG": "C.UTF-8", "TZ": "UTC"}
    env.update(json.loads(subprocess.check_output(
        [go, "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"], env=env)))
    isolation = runpy.run_path(str(source / "testdata/agent/verify.py"))["isolate"]()
    home = Path(tempfile.mkdtemp(prefix=args.label + "-home-", dir=root))
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
               XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
               XDG_STATE_HOME=str(home / "state"), MAGPIE_TEST_SANDBOX=str(home),
               GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local",
               GOFLAGS="-mod=readonly", CGO_ENABLED="1")
    inventory = {}
    for location in sorted(source.rglob("*")):
        relative = location.relative_to(source)
        if not location.is_file() or set(relative.parts) & {".git", "dist", "node_modules", "__pycache__"}:
            continue
        if location.name.endswith("-EVIDENCE.md"):
            continue
        inventory[str(relative)] = hashlib.sha256(location.read_bytes()).hexdigest()
    inventory_body = (json.dumps(inventory, indent=2, sort_keys=True) + "\n").encode()
    (root / (args.label + "-source.json")).write_bytes(inventory_body)
    host = next(item for item in json.loads((source.parent / "docs/source-lock.json").read_bytes())["sources"]
                if item["id"] == "sdk-host")
    report = {"label": args.label, "isolation": isolation,
              "sourceFiles": len(inventory),
              "sourceInventorySHA256": hashlib.sha256(inventory_body).hexdigest(),
              "hostCommit": host["commit"], "hostTree": host["tree"]}

    def run(label, command, expected=0, assertion=None):
        log = root / (label + ".log")
        with log.open("wb") as output:
            process = subprocess.run(command, cwd=source, env=env, stdout=output,
                                     stderr=subprocess.STDOUT, timeout=300)
        counts = {"pass": 0, "fail": 0, "skip": 0}
        for line in log.read_text().splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if event.get("Test") and event.get("Action") in counts:
                counts[event["Action"]] += 1
        result = {"label": label, "command": command, "expectedExit": expected,
                  "exit": process.returncode, **counts,
                  "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
        if "results" in report:
            report["results"].append(result)
        else:
            report.update(result)
        (root / (args.label + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
        print(label, "exit", process.returncode, "pass", counts["pass"], "fail", counts["fail"], flush=True)
        if process.returncode != expected:
            print(log.read_text()[-10000:], flush=True)
            raise SystemExit(process.returncode or 1)
        if expected:
            if assertion:
                assert "AssertionError:" in log.read_text() and assertion in log.read_text(), \
                    "RED did not reach its built-artifact assertion"
            else:
                assert counts["fail"] > 0, "RED did not reach a compiled behavior assertion"
        return log

    if args.stage == "tests":
        run(args.label, [go, "test", "-json", "-race", "-count=" + str(args.count),
                         "-timeout=4m", "-run", args.run, "./..." if args.all else "."],
            expected=1 if args.red else 0)
    elif args.stage == "static":
        report["results"] = []
        files = [name for name in inventory if name.endswith(".go")]
        log = run(args.label + "-format", [str(args.go.parent / "gofmt"), "-l", *files])
        assert not log.read_bytes(), "gofmt reported unformatted source"
        run(args.label + "-vet", [go, "vet", "./..."])
        run(args.label + "-python", ["python3", "-I", "-c",
            "import ast,pathlib; [ast.parse(pathlib.Path(name).read_bytes()) for name in " +
            repr([name for name in inventory if name.endswith(".py")]) + "]"])
    elif args.stage == "default-package":
        run(args.label, ["python3", "-I", "testdata/parity/default_package.py", str(root), args.label, "--output", str(source.parent / "dist/distribution"), "--archive", str(source.parent / "dist")],
            expected=1 if args.red else 0,
            assertion="default package omits the ready agent/UI composition" if args.red else None)
    else:
        report["results"] = []
        mutations = [
            ("strict-trial", "module.go", '!trial && !giftOnly && credentials.Kind != "start"',
             'credentials.Kind != "start"', "^TestGiftTrialRefusalNeverSpendsPaidAndQuotaInside200Is429$"),
            ("uncertain-gift", "gift.go", "(code == 1005 || code == 1113)",
             "(code == 1005 || code == 1113 || code == 500)", "^TestGiftFreeFirstUnknownReplyNeverReplaysOntoPaid$"),
            ("fallback-debt", "gift.go", "owner.paidFallbackUsage(ctx, call, stream, account)",
             'owner.readPaidUsage(ctx, stream, account, "")', "^TestGiftFallbackKeepsUnsettledPrincipalDebt$"),
            ("claim-replay", "billing.go",
             'if exists && (old.State == "pending" || old.State == "unknown") {\n\t\t\treturn uncertain("A previous billing claim may have succeeded',
             'if false && exists && (old.State == "pending" || old.State == "unknown") {\n\t\t\treturn uncertain("A previous billing claim may have succeeded',
             "^TestCheckinUnknownFeedNeverClaimsAndUnknownClaimNeverReplays/claim$"),
            ("claim-ack", "billing.go", "code == 0 && (status < 200 || status >= 300)",
             "code == 0 && (!valid || status < 200 || status >= 300)",
             "^TestClaimAcknowledgmentsAndDefinitiveRefusalsSurviveRestart$"),
            ("start-cap", "start_guard.go", "return 3", "return 30",
             "^TestStartGuardLimitsConcurrentRequestsForOneAccount$"),
            ("start-unknown-retry", "start_guard.go",
             "(code == 3008 || code == 3009 || code == 3010)", "(code >= 0)",
             "^TestStartGuardRetriesOnlyKnownUnusedConcurrencyCaps/500$"),
            ("gift-reply", "module.go", 'replyModel = wireGiftModel(target) + "-Trial"',
             'replyModel = ""', "^TestCompletionGiftRepliesNameActualTrialPool"),
            ("gift-held", "gift.go", "if held {\n\t\t\treturn model, account, false, nil",
             "if false && held {\n\t\t\treturn model, account, false, nil",
             "^TestCompletionHeldFreeFirstBypassesGift"),
            ("gift-only", "gift.go",
             'if trial || giftOnly {\n\t\t\treturn "", account, false, &sdk.Error{Code: "rate_limit_error",',
             'if trial {\n\t\t\treturn "", account, false, &sdk.Error{Code: "rate_limit_error",',
             "^TestCompletionGiftOnlyCannotBecomePaid"),
            ("paid-envelope", "module.go", "request.Headers, plainBody, outbound, response)",
             "request.Headers, append(plainBody[:0:0], body...), outbound, response)",
             "^TestCompletionPaidFallbackRestoresOriginalSystemEnvelope$"),
            ("query-turn", "identity.go", "strconv.Itoa(turns)", "strconv.Itoa(min(turns, 1))",
             "^TestCompletionStartQueryTracksUserTurns"),
            ("usage-known", "response_usage.go", "usage.totals, usage.known = next, true",
             "usage.totals, usage.known = next, false", "^TestCompletionUsage"),
            ("gift-best", "discovery_start.go",
             "if !found || !buckets[old].spendable && bucket.spendable",
             "if !found || false && !buckets[old].spendable && bucket.spendable",
             "^TestCompletionGiftBucketsKeepPlanIdentity"),
            ("sqlite-observability", "historydb/sqlite.go", 'if document.summary.Kind == "cli-history" {',
             'if false && document.summary.Kind == "cli-history" {',
             "^TestSQLiteObservabilityRetainsAttempts"),
            ("log-tail", "history/history.go", "document.summary.IncompleteTail = true",
             "document.summary.IncompleteTail = false", "^TestCLILogIncompleteTail"),
            ("auto-team", "auth_complete.go", 'if selection.Kind == "auto" && account.Kind != "team" {',
             'if false && selection.Kind == "auto" && account.Kind != "team" {',
             "^TestCompletionAutoSignIn"),
            ("team-unknown", "discovery_selection.go",
             'if !validStatus || !validGrant || status != "EFFECTIVE" || grant != "VALID" {\n\t\treturn false, protocol.ErrInvalid',
             'if !validStatus || !validGrant || status != "EFFECTIVE" || grant != "VALID" {\n\t\treturn false, nil',
             "^TestCompletionAutoSignIn.*/new-"),
            ("paid-busy", "paid_dispatch.go", "if !startBusy(response.StatusCode, data) || attempt >= 2 {",
             "if attempt >= 0 {", "^TestCompletionPaidBusyRetriesOnlyConfirmedUnusedCaps/3008$"),
            ("start-envelope", "module.go", "body, err = protocol.PrepareStart(body, owner.options.Now())",
             "body, err = protocol.PrepareStart(body, owner.options.Now()); body = plainBody",
             "^TestCompletionStartShapeKeepsCallerTextAndSourceCacheBoundaries/start$"),
            ("nested-toolsearch", "protocol/request.go", 'if response["type"] == "message_start" {',
             'if false && response["type"] == "message_start" {',
             "^TestNestedMessageStartRestoresOnlyToolAuthority$"),
        ]
        for name, target, before, after, test in mutations:
            original = (source / target).read_text()
            assert original.count(before) == 1, name
            mutated = root / (args.label + "-" + name + ".go")
            mutated.write_text(original.replace(before, after, 1))
            overlay = root / (args.label + "-" + name + "-overlay.json")
            overlay.write_text(json.dumps({"Replace": {str(source / target): str(mutated)}}))
            run(args.label + "-" + name, [go, "test", "-overlay", str(overlay), "-json",
                                         "-count=1", "-timeout=1m", "-run", test, "./..."], expected=1)


if __name__ == "__main__":
    main()
