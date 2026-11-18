import argparse
import hashlib
import json
from pathlib import Path
import runpy
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    parser.add_argument("stage", choices=["red", "identity-red", "unit", "static", "package", "installed", "causal"])
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--cache-env", required=True, type=Path)
    parser.add_argument("--host", type=Path)
    parser.add_argument("--host-source", type=Path)
    parser.add_argument("--host-inventory", type=Path)
    parser.add_argument("--count", type=int, default=20)
    parser.add_argument("--expect-native-scanner-blocker", action="store_true")
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    cache = json.loads(args.cache_env.read_bytes())
    env = {
        "PATH": str(args.go.parent) + ":/Users/maratkarimov/.local/bin:/Users/maratkarimov/.bun/bin:/usr/local/bin:/usr/bin:/usr/sbin:/bin:/sbin",
        "LANG": "C.UTF-8", "TZ": "UTC", "GOMAXPROCS": "2", "GOFLAGS": "-mod=readonly -p=2",
        "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "CGO_ENABLED": "1",
    }
    for name in ["GOPATH", "GOMODCACHE", "GOCACHE"]:
        env[name] = cache[name]
        assert Path(env[name]).is_dir(), name
    isolation = runpy.run_path(str(source / "testdata/agent/verify.py"))["isolate"]()
    report = {"stage": args.stage, "isolation": isolation, "results": []}
    with tempfile.TemporaryDirectory(prefix=args.stage + "-home-", dir=output) as scratch:
        home = Path(scratch)
        env.update(HOME=scratch, USERPROFILE=scratch, XDG_CONFIG_HOME=str(home / "config"),
                   XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
                   XDG_STATE_HOME=str(home / "state"), APPDATA=str(home / "appdata"),
                   LOCALAPPDATA=str(home / "localappdata"), MAGPIE_TEST_SANDBOX=scratch)

        def run(name, command, expected=0, cwd=source, timeout=300):
            log = output / (name + ".log")
            with log.open("wb") as stream:
                result = subprocess.run(command, cwd=cwd, env=env, stdout=stream,
                                        stderr=subprocess.STDOUT, timeout=timeout)
            text = log.read_text()
            counts = dict.fromkeys(["pass", "fail", "skip"], 0)
            failures = []
            for line in text.splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if event.get("Test") and event.get("Action") in counts:
                    counts[event["Action"]] += 1
                    if event["Action"] == "fail":
                        failures.append(event["Test"])
            record = {"name": name, "command": command, "exit": result.returncode,
                      "expectedExit": expected, "counts": counts, "failures": failures,
                      "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
            report["results"].append(record)
            (output / (args.stage + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
            print(json.dumps(record), flush=True)
            assert result.returncode == expected, text[-10000:]
            assert "WARNING: DATA RACE" not in text
            if expected:
                assert counts["fail"] > 0 and "[build failed]" not in text, text[-10000:]
            if name.endswith("format"):
                assert not text, text

        go = str(args.go)
        if args.stage == "red":
            run("old-registration", [go, "test", "-p=2", "-json", "-count=1", "-timeout=1m",
                                     "-run", "^TestAutomaticHistoryIsRegistered", "."], expected=1)
        elif args.stage == "identity-red":
            run("old-database-selection", [go, "test", "-p=2", "-json", "-count=1", "-timeout=1m",
                                           "-run", "^TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection$", "."], expected=1)
        elif args.stage == "unit":
            for test in [
                "AutomaticHistoryRealWALProjectionAndSourcePreservation",
                "AutomaticHistoryTransactionExcludesConcurrentAndUncommittedWrites",
                "AutomaticHistoryPagesAndLocatorsRejectForeignAndChangedViews",
                "AutomaticHistoryDatabaseIncarnationRejectsStaleSelection",
                "AutomaticHistoryUnreadableMalformedAndMissingSHMStayUnknown",
                "AutomaticHistorySDKChecksLeaseBeforeAndAfterProjection",
                "AutomaticHistoryPOSIXProbesAndOtherReadersRetainLocks",
                "AutomaticHistoryIsRegisteredInTheOrdinaryNativePackage",
                "SourceLockMatchesActualModuleAndNamesImmutableBehaviorSources",
            ]:
                run("race-" + test, [go, "test", "-p=2", "-race", "-json",
                                     "-count=" + str(args.count), "-timeout=3m",
                                     "-run", "^Test" + test + "$", "."])
            run("module-regression", [go, "test", "-p=2", "-race", "-json", "-count=1",
                                      "-timeout=3m", "./..."])
        elif args.stage == "static":
            files = sorted(path.name for path in source.glob("storage*.go"))
            run("automatic-history-format", [str(args.go.parent / "gofmt"), "-l", *files, "window.go"])
            run("automatic-history-vet", [go, "vet", "-p=2", "./..."])
            for platform, architecture in [("darwin", "arm64"), ("windows", "amd64")]:
                env.update(GOOS=platform, GOARCH=architecture, CGO_ENABLED="0")
                run(platform + "-vet", [go, "vet", "-p=2", "."])
                run(platform + "-build", [go, "build", "-p=2", "-o", str(home / platform), "./cmd/native-ui"])
        elif args.stage == "package":
            run("automatic-history-package", ["python3", "-I", "../scripts/build.py", "--output", "../dist/distribution", "--archive", "../dist"])
            import zipfile
            distribution = output / "distribution"
            with zipfile.ZipFile(source.parent / "dist/magpie-plugin_linux_amd64.zip") as archive:
                archive.extractall(distribution)
            run("automatic-history-abi", ["python3", "-I", "../tests/conformance.py", str(distribution)])
            report["packageSHA256"] = hashlib.sha256((source.parent / "dist/magpie-plugin_linux_amd64.zip").read_bytes()).hexdigest()
            (output / (args.stage + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
        elif args.stage == "installed":
            assert args.host and args.host_source and args.host_inventory
            inventory = json.loads(args.host_inventory.read_bytes())
            report["host"] = {
                "physicalTree": inventory["tree"], "parentAtCapture": inventory["head_at_capture"],
                "archiveSHA256": inventory["archive_sha256"],
                "inventorySHA256": hashlib.sha256(args.host_inventory.read_bytes()).hexdigest(),
                "binarySHA256": hashlib.sha256(args.host.read_bytes()).hexdigest(),
                "paths": len(inventory["files"]),
            }
            pins = json.loads((source.parent / "docs/source-lock.json").read_bytes())["sources"]
            pin = next(item for item in pins if item.get("physicalTree") == inventory["tree"])
            assert report["host"]["binarySHA256"] == pin["binarySHA256"]
            assert report["host"]["archiveSHA256"] == pin["archiveSHA256"]
            report["host"]["sourceLockID"] = pin["id"]
            report["host"]["commit"] = pin["commit"]
            for name, entry in inventory["files"].items():
                assert hashlib.sha256((args.host_source / name).read_bytes()).hexdigest() == entry["sha256"], name
            distribution = output.parent / "package/distribution"
            assert (distribution / "magpie-plugin.json").is_file()
            fixture = output / "zcode_automatic_history_test.go"
            fixture.write_bytes((source / "testdata/automatic-history/host_test.go.fixture").read_bytes())
            run("installed-fixture-format", [str(args.go.parent / "gofmt"), "-l", str(fixture)])
            overlay = output / "host-overlay.json"
            overlay.write_text(json.dumps({"Replace": {
                str(args.host_source / "internal/sessions/zcode_automatic_history_test.go"): str(fixture)
            }}, indent=2) + "\n")
            report["overlaySHA256"] = hashlib.sha256(overlay.read_bytes()).hexdigest()
            report["fixtureSHA256"] = hashlib.sha256(fixture.read_bytes()).hexdigest()
            env.update(ZCODE_STORAGE_DIST=str(distribution),
                       ZCODE_STORAGE_WRITER=str(source / "testdata/automatic-history/writer.py"),
                       ZCODE_STORAGE_HOST=str(args.host))
            scenarios = [
                ("ordinary-list-transcript", "^TestZCodeAutomaticHistoryInstalledListTranscriptAndWAL$", args.expect_native_scanner_blocker),
                ("native-owner-boundary", "^TestZCodeAutomaticHistoryDelegatedOwnerPreventsNativeEnumeration$", args.expect_native_scanner_blocker),
                ("rooted-owner", "^TestZCodeAutomaticHistoryInstalledRootedOwnerPreservesFamily$", False),
                ("database-rotation", "^TestZCodeAutomaticHistoryInstalledDatabaseRotationRejectsOldSelection$", False),
                *[(name, "^TestZCodeAutomaticHistoryInstalledPublicationRejectsChangedOwners$/^" + name + "$", False)
                  for name in ["reads", "consent", "config", "off", "root", "cancel", "foreign-off"]],
                *[(name, "^TestZCodeAutomaticHistoryInstalledRejectsSidecarSymlinkAndMissingSHM$/^" + name + "$", False)
                  for name in ["symlink", "missing-shm"]],
            ]
            for iteration in range(args.count):
                for name, pattern, blocked in scenarios:
                    run("installed-" + str(iteration + 1) + "-" + name,
                        [go, "test", "-p=2", "-tags=nogui", "-overlay", str(overlay), "-race", "-json",
                         "-count=1", "-timeout=2m", "-run", pattern, "./internal/sessions"],
                        expected=1 if blocked else 0, cwd=args.host_source, timeout=150)
                    if blocked:
                        text = (output / ("installed-" + str(iteration + 1) + "-" + name + ".log")).read_text()
                        required = "installed reader wrote source family: shm" if name == "ordinary-list-transcript" else \
                            "delegated ZCode storage owner still invokes the native database enumerator"
                        assert required in text, text[-5000:]
            for name, entry in inventory["files"].items():
                assert hashlib.sha256((args.host_source / name).read_bytes()).hexdigest() == entry["sha256"], name
        elif args.stage == "causal":
            mutations = [
                ("automatic-registration", "window.go",
                 'manifest.StorageSources = []sdk.StorageSourceDescriptor{historyDescriptor()}',
                 'manifest.StorageSources = nil',
                 "^TestAutomaticHistoryIsRegistered"),
                ("fork-copy", "storage_projection.go", "if copied {", "if false && copied {",
                 "^TestAutomaticHistoryRealWALProjectionAndSourcePreservation$"),
                ("unknown-usage", "storage_projection.go", "summary.Usage = nil", "summary.Usage = []sdk.SessionUsagePoint{}",
                 "^TestAutomaticHistoryRealWALProjectionAndSourcePreservation$"),
                ("block-order", "storage_projection.go", 'if message.data["role"] != "assistant" {\n\t\treturn parts',
                 'if true {\n\t\treturn parts', "^TestAutomaticHistoryRealWALProjectionAndSourcePreservation$"),
                ("wal-transaction", "storage_sqlite.go", '"?mode=ro&vfs="+vfs.name',
                 '"?mode=ro&immutable=1&vfs="+vfs.name', "^TestAutomaticHistoryRealWALProjectionAndSourcePreservation$"),
                ("shm-lock", "storage_vfs_unix.go", "if file.vfs.locks.shm(shm, true) != nil {", "if false {",
                 "^TestAutomaticHistoryTransactionExcludesConcurrentAndUncommittedWrites$"),
                ("posix-probe-close", "storage_lock_unix.go",
                 "if state := locks.inodes[inode]; state != nil && (state.db > 0 || state.shm > 0) {",
                 "if state := locks.inodes[inode]; false && state != nil && (state.db > 0 || state.shm > 0) {",
                 "^TestAutomaticHistoryPOSIXProbesAndOtherReadersRetainLocks$"),
                ("cursor-lease", "storage.go", "}{request.Lease, entries, cut}", "}{sdk.StorageReadLease{}, entries, cut}",
                 "^TestAutomaticHistoryPagesAndLocatorsRejectForeignAndChangedViews/snapshot$"),
                ("database-generation", "storage_sqlite.go",
                 "reader.incarnation = historyDigest([]string{vfs.identity, identity})",
                 "_ = identity\n\treader.incarnation = historyDigest([]string{vfs.identity, \"\"})",
                 "^TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection$/^identity$"),
                ("database-file", "storage_sqlite.go",
                 "reader.incarnation = historyDigest([]string{vfs.identity, identity})",
                 "reader.incarnation = historyDigest([]string{\"\", identity})",
                 "^TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection$/^replace$"),
                ("before-admission", "storage.go", "if err := stream.CheckStorage(ctx, request.Lease); err != nil {",
                 "if err := ctx.Err(); err != nil {", "^TestAutomaticHistorySDKChecksLeaseBeforeAndAfterProjection/1$"),
            ]
            for name, path, before, after, test in mutations:
                original = (source / path).read_text()
                assert before in original, (name, before)
                changed = output / (name + ".go")
                if name == "automatic-registration":
                    original = original.replace("sdk.AgentWiring, sdk.StorageSource", "sdk.AgentWiring")
                    original = original.replace("module.Handlers[sdk.MethodStorageList] = automaticHistory", "")
                    original = original.replace("module.Handlers[sdk.MethodStorageRead] = automaticHistory", "")
                changed.write_text(original.replace(before, after, 1))
                overlay = output / (name + "-overlay.json")
                overlay.write_text(json.dumps({"Replace": {str(source / path): str(changed)}}))
                run("causal-" + name, [go, "test", "-p=2", "-overlay", str(overlay), "-json",
                                      "-count=1", "-timeout=1m", "-run", test, "."], expected=1)
        else:
            raise SystemExit("stage is not implemented")


if __name__ == "__main__":
    main()
