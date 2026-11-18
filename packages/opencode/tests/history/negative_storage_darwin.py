#!/usr/bin/env python3
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

packet = Path(sys.argv[1]).resolve()
settings = json.loads(Path("/tmp/magpie-opencode-account-rugrjcv2/test-env.json").read_text())
home = Path(tempfile.mkdtemp(prefix="darwin-negative-home-", dir=packet))
env = dict(os.environ, **settings)
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / "config"),
           XDG_CACHE_HOME=str(home / "cache"), XDG_DATA_HOME=str(home / "data"),
           XDG_STATE_HOME=str(home / "state"), GOFLAGS="-mod=readonly -p=4", GOMAXPROCS="4")
go = settings["PATH"].split(":")[0] + "/go"
cases = [
    ("metadata-physical-identity", "magpie", "internal/extension/storage_grant_unix.go",
     "./internal/extension", "TestStorageMetadataIdentityAndNoFollowSurvivePublication",
     lambda text: re.sub(r"func storageSameFile\(.*?\n}", """func storageSameFile(first, second fs.FileInfo) bool {
return os.SameFile(first, second)
}""", text, count=1, flags=re.S)),
    ("metadata-close", "magpie", "internal/extension/storage_grant_unix.go",
     "./internal/extension", "TestStorageMetadataRecheckDoesNotClosePOSIXLockedInode",
     lambda text: re.sub(r"func \(grant \*StorageGrant\) stat\(.*?\n}", """func (grant *StorageGrant) stat(name string) (fs.FileInfo, error) {
file, err := grant.open(name)
if err != nil { return nil, err }
defer grant.closeMember(file)
return file.Stat()
}""", text, count=1, flags=re.S)),
    ("metadata-symlink", "magpie", "internal/extension/storage_grant_unix.go",
     "./internal/extension", "TestStorageMetadataIdentityAndNoFollowSurvivePublication",
     lambda text: text.replace("&info.stat, unix.AT_SYMLINK_NOFOLLOW", "&info.stat, 0", 1)),
    ("native-symlink", "magpie", "pkg/extension/native/storage_reader_unix.go",
     "./pkg/extension/native", "TestStorageNativeRootIsFiniteReadonlyAndCancelable",
     lambda text: text.replace(" | unix.O_NOFOLLOW", "", 1)),
    ("posix-probe-close", "plugin", "history_lock_unix.go",
     ".", "TestHistoryPOSIXReadersAndProbesRetainExternalLocks",
     lambda text: text.replace("state != nil && (state.db > 0 || state.shm > 0)", "false", 1)),
    ("posix-overlap-unlock", "plugin", "history_lock_unix.go",
     ".", "TestHistoryPOSIXRealWALReadersSurviveProbeCloseAndWriterReset",
     lambda text: text.replace("!acquire && *readers != 1", "false", 1)),
    ("posix-failed-reader", "plugin", "history_lock_unix.go",
     ".", "TestHistoryPOSIXFailedAcquisitionLeavesNoReaderOrPendingLock",
     lambda text: text.replace("state = locks.inodes[inode]", """state = locks.inodes[inode]
if state == nil { state = &historyInodeLocks{}; locks.inodes[inode] = state }""", 1)),
    ("wire-parent-track", "magpie", "internal/extension/client.go",
     "./internal/extension", "TestStorageCanceledWireReleasesOnlyAfterValidTerminal",
     lambda text: text.replace("trackStorageCall(ctx)", "_ = ctx", 1)),
    ("wire-parent-terminal", "magpie", "internal/extension/client.go",
     "./internal/extension", "TestStorageCanceledWireReleasesOnlyAfterValidTerminal",
     lambda text: text.replace("endStorageCall(pending.ctx)", "_ = pending.ctx", 1)),
    ("cancel-lifetime", "magpie", "internal/extension/storage_grant.go",
     "./internal/extension", "TestStorageCanceledLifetimeWaitsForActualParentTerminal",
     lambda text: text.replace("grant.lifetime == nil || !grant.lifetime.native || !grant.lifetime.started || grant.lifetime.ended",
                               "true", 1)),
]
rows = []
for name, owner, filename, package, test, mutate in cases:
    directory = packet / owner
    source = directory / filename
    before = source.read_bytes()
    after = mutate(before.decode())
    assert after.encode() != before, "negative did not change its selected source"
    try:
        source.write_text(after)
        result = subprocess.run([go, "test", "-tags=nogui", "-json", "-count=1", "-timeout=45s",
                                 "-run", "^" + test + "$", package],
                                cwd=directory, env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    finally:
        source.write_bytes(before)
    (packet / ("darwin-negative-" + name + ".log")).write_text(result.stdout)
    failed = False
    for line in result.stdout.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        failed = failed or event.get("Action") == "fail" and event.get("Test") == test
    row = {"mutation": name, "exit": result.returncode, "namedBehaviorFailure": failed,
           "compileFailure": "[build failed]" in result.stdout or '"Action":"build-fail"' in result.stdout}
    rows.append(row)
    (packet / "darwin-negative-results.json").write_text(json.dumps(rows, indent=2) + "\n")
    print(json.dumps(row), flush=True)
    assert result.returncode != 0 and failed and not row["compileFailure"], "negative did not fail the named behavior"
