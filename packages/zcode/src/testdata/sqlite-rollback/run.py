import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import tempfile


def inventory(root):
    return {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in sorted(root.rglob("*")) if path.is_file() and ".git" not in path.relative_to(root).parts}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sdk", required=True, type=Path)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    original, distribution, output = args.sdk.resolve(), args.distribution.resolve(), args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    before = inventory(original)
    env = os.environ.copy()
    env.update(json.loads(subprocess.check_output(["go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE"])))
    with tempfile.TemporaryDirectory(prefix="zcode-sdk-rollback-") as scratch:
        root = Path(scratch)
        sdk = root / "sdk"
        shutil.copytree(original, sdk)
        fixture = Path(__file__).with_name("host_test.go.fixture")
        injected = sdk / "internal/extension/zcode_sqlite_rollback_external_test.go"
        shutil.copyfile(fixture, injected)
        source = root / "source.db"
        with sqlite3.connect(source) as database:
            database.execute("CREATE TABLE tasks(task_id TEXT,title TEXT,updated_at INTEGER,deleted INTEGER)")
            database.execute("INSERT INTO tasks VALUES('synthetic','Rollback',9007199254740993,0)")
        home = root / "home"
        home.mkdir()
        env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
                   XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                   XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                   LOCALAPPDATA=str(home / "localappdata"),
                   GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
                   GOWORK="off",
                   MAGPIE_ZCODE_SQLITE_SOURCE=str(source), MAGPIE_ZCODE_SQLITE_DISTRIBUTION=str(distribution))
        env.pop("MAGPIE_TEST_SANDBOX", None)
        scenarios = ["cancel-after-write", "foreign-during-rollback", "stale-preimage"]
        commands = [["go", "test", "-json", "-tags", "nogui", "-race", "-count=1", "-timeout=2m",
                     "-run", "^TestZcodeNativeSQLiteRollback/" + scenario + "$", "./internal/extension"]
                    for scenario in scenarios]
        processes = {scenario: 0 for scenario in scenarios}
        with (output / "race20.log").open("wb") as log:
            for _ in range(20):
                for scenario, command in zip(scenarios, commands):
                    result = subprocess.run(command, cwd=sdk, env=env, stdout=log, stderr=subprocess.STDOUT,
                                            timeout=150)
                    processes[scenario] += 1
                    if result.returncode:
                        break
                if result.returncode:
                    break
        after = inventory(sdk)
        after.pop(str(injected.relative_to(sdk)))
        if after != before or inventory(original) != before:
            raise AssertionError("verification changed SDK production bytes")
        events = [json.loads(line) for line in (output / "race20.log").read_text().splitlines() if line.startswith("{")]
        evidence = {
            "exit": result.returncode, "commands": commands, "sdkProductionFiles": len(before),
            "freshProcessIterations": sum(processes.values()), "processesPerScenario": processes,
            "sdkInventorySHA256": hashlib.sha256(json.dumps(before, sort_keys=True).encode()).hexdigest(),
            "fixtureSHA256": hashlib.sha256(fixture.read_bytes()).hexdigest(),
            "passes": sum(event.get("Action") == "pass" and "Test" in event for event in events),
            "failures": [event.get("Test", event["Package"]) for event in events if event.get("Action") == "fail"],
            "skips": sum(event.get("Action") == "skip" for event in events),
            "productionBytesUnchanged": True, "pythonOnlyGeneratedSyntheticInput": True,
        }
        (output / "result.json").write_text(json.dumps(evidence, indent=2))
        print(json.dumps(evidence))
        if result.returncode:
            raise SystemExit(result.returncode)


if __name__ == "__main__":
    main()
