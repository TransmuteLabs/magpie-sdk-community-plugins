# Bounded ZCode automatic-history review

For the user-designated reviewer Raman,
`01a1dcb8-7d49-7813-9703-b2a32e78d66d`. Review this independent slice
against `83394a67e131310fa617c1a27b633f39e0053892` in this repository's
existing main. The previous model reviewer is not an approval authority
for this packet.

## Changed behavior and files

- `storage.go` and `window.go`: ordinary 0.15.0 native/UI package adopts
  existing API9 rooted storage for finite `.zcode/cli/db`, DB/WAL/SHM and
  rollback-journal presence. Lease checks surround acquisition/projection;
  cursor binds lease/catalog, locator binds root/family/session and DB
  generation/physical incarnation. Resume remains none.
- `storage_sqlite.go` and `storage_projection.go`: bounded read-only
  transaction, current schema/row ownership/order, fork-copy accounting,
  exact native counts, explicit unknowns and unavailable external bodies.
- `storage_vfs_{unix,other}.go` and `storage_lock_{unix,linux,darwin}.go`:
  module-local adaptation of pinned sibling VFS/locks. Source descriptors
  open read-only, private SHM memory under real shared locks, coordinated
  POSIX descriptor lifetime; no new dependency or path fallback.
- `storage_test.go`, `storage_sqlite_linux_test.go`,
  `storage_lock_linux_test.go` and `testdata/automatic-history/`: real
  separate SQLite writer, actual SDK callback owner, installed-package
  additional host-test overlay and isolated verification runner.
- `source-lock.json`, `sources_test.go`, `build.py`, `web/package.json`
  and documentation: exact source/proof identities, one ordinary package,
  current API9 floor and precise remaining resume/V4 boundaries.

Historical manual converters, financial state and UI logic are unchanged.
No host, donor, branch/worktree/stash or live app/account was changed.

Exact changed paths:

```text
AUTOMATIC-HISTORY-EVIDENCE.md
AUTOMATIC-HISTORY-HOST-HANDOFF.md
AUTOMATIC-HISTORY-PLAN.md
AUTOMATIC-HISTORY-REVIEW-PACKET.md
AUTOMATIC-HISTORY.md
LEADER-CONTRACTS.md
NATIVE-CONTRACT-REQUEST.md
README.md
SOURCES.md
SQLITE-INTEGRATION-PLAN.md
build.py
source-lock.json
sources_test.go
storage.go
storage_lock_darwin.go
storage_lock_linux.go
storage_lock_linux_test.go
storage_lock_unix.go
storage_projection.go
storage_sqlite.go
storage_sqlite_linux_test.go
storage_test.go
storage_vfs_other.go
storage_vfs_unix.go
testdata/automatic-history/host_test.go.fixture
testdata/automatic-history/verify.py
testdata/automatic-history/writer.py
web/package.json
window.go
```

## Evidence to review

AUTOMATIC-HISTORY-EVIDENCE.md has exact commands, artifacts and failures.
Usbox evidence: `/tmp/zcode-auto-history-20261115-APFz8QwX`.
Local retained evidence:
`dist/zcode-auto-history-20261115-APFz8QwX/complete-scoped-evidence.tar.gz`,
SHA256 `410adbfa2d7918d352021edf6e632c1102b0fa6372c537f93da81c25bc603770`.

- Focused per-invariant race20: 640 PASS, no FAIL/SKIP/race warning.
- Entire module race regression: 506 PASS, no FAIL/SKIP.
- Eleven compiled causal overlays: all assertion RED, including ignored
  WAL, absent SHM lock, premature POSIX close and both DB identity bindings.
- Actual installed rooted owner and rotation plus nine publication/
  sidecar negatives: 11 scenarios / 20 PASS events on merged API9
  `66ac2f7f`, physical/committed tree `94ec4c8d`, binary `a8e09770`.
- gofmt, module vet, Darwin/Windows static vet/build, ordinary native/UI
  build and ABI conformance pass. Native runtime proof is Linux only.
- Final handoff ZIP SHA256:
  `781c3bb4dca212f19ab5fdfc5b0899fc2731a48983cd9d9fd0b053c818f8a205`.
  Only the precise API10 resume sidecar changed after installed tests;
  all executable entries stayed exact and fresh isolated ABI passed.

## Known host RED and separate work

Ordinary list/transcript is not accepted read-only yet:
`TestZCodeAutomaticHistoryInstalledListTranscriptAndWAL` changes SHM;
`TestZCodeAutomaticHistoryDelegatedOwnerPreventsNativeEnumeration` calls
the native enumerator once instead of zero. The proposed leader-owned
existing-contract fix is `storageNativeFiles("zcode", zcode)` in
`allFilesWith`; see AUTOMATIC-HISTORY-HOST-HANDOFF.md. No host production
overlay was applied or passed off as acceptance.

NATIVE-CONTRACT-REQUEST.md gives the exact generic source-bound `--resume`
recipe/current SDK-owned agent/profile requirement for the host owner's
API10 slice. Duplex belongs to Linnaeus. Actual V4 and native resume are
not claimed implemented; desktop index/custom roots and financial import
remain distinct from this default CLI history source.

Please assess correctness, authority/resource lifetime and regression
sufficiency of this bounded module slice. If more verification is needed,
use usbox with the existing isolated HOME/XDG/offline guard and `-p=2`,
GOMAXPROCS2. Do not edit host code, run a host broad suite or test locally.
