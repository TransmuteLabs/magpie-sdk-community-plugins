#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess


def inventory(source):
    return {
        str(path.relative_to(source)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted(source.rglob("*")) if path.is_file()
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    sdk = (root.parent / "magpie").resolve(strict=True)
    before = inventory(root)
    environment = dict(os.environ, GOMAXPROCS="2", GOFLAGS="-mod=readonly -p=2",
                       GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOWORK="off",
                       PYTHONDONTWRITEBYTECODE="1")
    controls = [
        ("forced-stream", "zen_policy.go", "TestZenPublicNativeStreamPolicy",
         'value["stream"] = true', 'value["stream"] = false'),
        ("json-collapse", "zen_client.go", "TestZenPublicNativeStreamPolicy",
         "if streaming && !requestedStream {", "if streaming && false {"),
        ("explicit-environment", "zen_policy.go", "TestZenPublicExplicitEnvironment",
         "if facts == nil {", "if true {"),
        ("quoted-skill", "skill_frontmatter.go", "TestConsoleSkillScalarFrontmatter",
         "if value[0] == '\\'' {", "if value[0] == '\\'' {\nreturn false, false"),
        ("watch-gate", "console_watch.go", "TestConsoleWatchTypesUseSelectedWorkspaceAndGates",
         'if !value.(ConsoleSnapshot).Watches || value.(ConsoleSnapshot).Status != "" {',
         'if false && (!value.(ConsoleSnapshot).Watches || value.(ConsoleSnapshot).Status != "") {'),
        ("native-console-owner", "agent.go", "TestAgentPreservesNativeConsoleOwnership",
         'values["model"], _ = json.Marshal(change.Values["model"])',
         'values["model"], _ = json.Marshal(change.Values["model"])\ndelete(values, "policies")'),
        ("inspection-attribution", "console.go", "TestConsoleInspectionIdentifiesItsActualOwner",
         'req.Header.Set("User-Agent", "magpie-opencode/"+windowVersion+" (console-inspection; opencode-schema/"+ClientVersion+")")',
         'req.Header.Set("User-Agent", "opencode/latest/"+ClientVersion+"/cli")'),
    ]
    results = []
    for name, filename, test, original, replacement in controls:
        lane = output / name
        source = lane / "plugin"
        shutil.copytree(root, source)
        (lane / "magpie").symlink_to(sdk, target_is_directory=True)
        target = source / filename
        text = target.read_text()
        assert text.count(original) == 1, "mutation target is ambiguous or absent"
        target.write_text(text.replace(original, replacement, 1))
        command = ["go", "test", "-json", "-count=1", "-timeout=45s", "-run", "^" + test + "$", "."]
        process = subprocess.run(command, cwd=source, env=environment, text=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
        log = output / (name + ".log")
        log.write_text(process.stdout)
        failures = []
        for line in process.stdout.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if isinstance(event, dict) and event.get("Action") == "fail" and event.get("Test"):
                failures.append(event["Test"])
        row = {
            "mutation": name, "file": filename, "before": original, "after": replacement,
            "command": command, "cwd": str(source), "exit": process.returncode,
            "namedBehaviorFailure": test in failures, "failedTestsAndSubtests": failures,
            "compileFailure": "[build failed]" in process.stdout or "build-fail" in process.stdout,
            "log": str(log), "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest(),
            "mutatedSourceSHA256": hashlib.sha256(target.read_bytes()).hexdigest(),
        }
        results.append(row)
        (output / "results.json").write_text(json.dumps(results, indent=2) + "\n")
        print(json.dumps(row), flush=True)
        assert row["exit"] != 0 and row["namedBehaviorFailure"] and not row["compileFailure"], \
            "control must fail the named behavior rather than compilation"
    unchanged = before == inventory(root)
    (output / "source-audit.json").write_text(json.dumps({
        "source": str(root), "unchanged": unchanged, "files": len(before), "inventory": before,
    }, indent=2) + "\n")
    assert unchanged, "controls changed the original source"


if __name__ == "__main__":
    main()
