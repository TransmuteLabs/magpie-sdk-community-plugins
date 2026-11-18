# Rooted automatic CLI history evidence

2026-11-15. Independent module slice after local Lore commit
`83394a67e131310fa617c1a27b633f39e0053892`. Ordinary package 0.15.0,
public API9 / native ABI1; historical headless package remains 0.7.3 / API1.4.
Only this repository changed. No new dependencies, host/donor edits,
branches/worktrees/stashes, push, publication or live app/account calls.

The pre-edit extraction/regression plan is AUTOMATIC-HISTORY-PLAN.md.
AUTOMATIC-HISTORY.md describes the finite default CLI source and its
read-only transaction. Current remaining host contracts are in
NATIVE-CONTRACT-REQUEST.md; this slice does not implement native resume or V4.

## Authority and frozen inputs

Official source is `zai-org/ZCode@fec975905a2abd4c07e35c40d51fa097a2258ec9`,
desktop 3.18.2 / CLI 0.17.1. Exact default path, migrations, row codecs,
message/session repositories and contracts are hashed in source-lock.json.
The shared `0024_session_database_identity` migration and read-only
`sourceServerWorkspaceAliasRepo.ts` consumer additionally establish the
optional `scope='session'` generation. The module reads that generation;
it never initializes or migrates the source.

The adapted VFS/Unix lock/probe source is sibling OpenCode module
`ed9c811f9fa931e70da4c7e393ae4ba1b0844946`. Its consumed file bytes remained
identical when that other lane advanced HEAD. Exact consumed hashes, rather
than its moving checkout, define this module's provenance.

Current installed replay uses the owner's merged API9 base:

| Identity | Exact value |
| --- | --- |
| Supplied merged commit | `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55` |
| Committed and physical tree | `94ec4c8de4e5b366a6e3fb854df2176007587ab8` |
| Read-only usbox root | `/tmp/magpie-upstream-signing-closure.ofRrrBdz` |
| Source paths | 10849 |
| Source archive SHA256 | `131aeaa8fa9052dab47854ead8cb8b57f7f4dadca92715dbad307f4424c2de9e` |
| Source inventory SHA256 | `d97b5eca0017294ddccc457ee691411ede08c190e12a0d7857f27af6d49ec7c7` |
| Binary SHA256 | `a8e09770909f78ae89cc6175a45bb98e1dacdb305b62856f46956928cb8ecbe2` |

The inventory's `head_at_capture` remains
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`. That capture metadata is separate
from the supplied merged commit; its physical tree matches the committed
tree. Earlier candidate `36f8e629` / binary `0bd335b7` remains historical,
not substituted for the merged-base replay.

Evidence root is `usbox:/tmp/zcode-auto-history-20261115-APFz8QwX`.
The executable/test freeze has 228 paths, archive `coherent-source.tar.gz`
SHA256 `c299c7698307a51c3b047009f137dd87bc26ede8a1409bea69efc7ffbf58f8e6`,
inventory SHA256
`396a76034e8930d77f915498d9707a9d60605329f7d56e87702246fad0a7c3c7`.
All frozen file hashes were checked before and after the complete pipeline.
Subsequent changes are documentation only: README, the plan, host handoff,
the precise API10 resume handoff, this evidence and the bounded review
packet. Production, fixtures and source-lock remain the tested bytes.
The API10 handoff sidecar was repacked separately with every executable
payload byte retained and fresh isolated ABI verification. The repack
compared all 21 ZIP entries: only NATIVE-CONTRACT-REQUEST.md changed.
All 228 frozen module paths remained exact before and after that check.

## Isolation and commands

Every build/test runs on usbox through
`testdata/automatic-history/verify.py`, reusing the existing
`testdata/agent/verify.py:isolate` network guard: a user/network namespace,
loopback only, zero effective/permitted/inheritable/bounding/ambient
capabilities. Each stage gets a new HOME/XDG/APPDATA and test sandbox.
GOCACHE/GOMODCACHE/GOPATH are explicitly pinned in `cache-env.json`;
GOPROXY/GOSUMDB are off, GOTOOLCHAIN is local, GOFLAGS is
`-mod=readonly -p=2`, GOMAXPROCS is 2.

Go is `/tmp/magpie-sdk-v11-usbox-niq306rt/go-toolchain-1.26.8/bin/go`.
The runner records each exact argv, exit, named failure, count and log hash.
Its `unit`, `causal`, `static`, `package` and `installed` stages have
`coherent-<stage>` output directories. Installed replay uses only the
named tests in `./internal/sessions`, with a module-owned additional Go
test supplied by overlay; no host broad suite or whole sessions suite.
All 10849 host source hashes were checked before and after replay.

| Gate | Result |
| --- | --- |
| Nine focused automatic-history/provenance tests, each `-race -count=20` | 640 PASS events, 0 FAIL/SKIP, no race warning |
| Entire module `go test -p=2 -race -json -count=1 -timeout=3m ./...` | 506 PASS events, 0 FAIL/SKIP |
| Eleven independent compiled causal overlays | All reach the required assertions: 17 FAIL events, no build failure |
| gofmt / module `go vet -p=2 ./...` | PASS |
| Darwin arm64 / Windows amd64 root vet and CGO0 build | PASS; static/compile evidence only |
| Ordinary native/UI package, strict TypeScript/IIFE, ABI conformance | PASS |
| Installed rooted owner, database rotation and nine revocation/sidecar scenarios | 11 scenarios PASS, 20 PASS events |
| Installed ordinary list/transcript and counted native enumerator | Two real host FAILs, described below |

The earlier combined focused command exhausted its four-minute aggregate
test deadline: `panic: test timed out after 4m0s`, with 492 PASS events and
no named assertion failure before exit. Its log remains in `final-unit`,
SHA256 `f86dd2fad5276798f5fab3784868f4eb2cc3b7864ba14f24ff8ed31f5667a26d`.
The successful runner gives each invariant its own process and three-minute
test deadline, retaining every count20 repetition. No production timing or
resource limit, assertion or skip policy changed.

## Real DB/WAL/SHM behavior

The stdlib Python fixture is a separate SQLite writer, not a JSON export.
It holds the database open with real official-shaped session/message/part
DDL and a stored write trigger. Its checkpointed main-only comparison has
zero sessions; the live committed WAL has five visible sessions plus a
soft-deleted one. `immutable=1` therefore cannot satisfy the test.

The rooted transaction retains exact DB/WAL/SHM hashes across reads.
A commit after transaction acquisition stays outside that transaction;
a fresh reader sees it. Uncommitted text never publishes. A real external
TRUNCATE checkpoint reports busy while the reader holds shared SHM locks
and succeeds after Close. Separate-process POSIX probes establish that
closing one probe/reader does not release another reader's locks.

Projection checks exact integers above 2^53, cache/reasoning subsets,
fork-copy/orphan accounting, real-user titles, nullable sequence and complete
assistant blockIndex ordering, unknown versus explicit-zero counters,
tool input/output and unavailable media/body references. Opaque signatures
and file URLs never appear in normalized output.

The optional official database identity and primary file device/inode bind
locators. Identity changes, same-root replacement with identical metadata,
and replacement of an older schema without the identity table reject old
locators and cursors. Fresh selections work. Malformed JSON, missing WAL/SHM,
unresolved rollback journal, generated schema, invalid identity table/column,
oversized identity and read-budget failure remain errors.

The actual SDK callback fixture checks admission before root acquisition,
after SQLite close and at final SDK publication. Actual installed owner
tests cover immediate reads/consent/config/off/root/cancel and foreign-process
off revocation, plus symlink and missing-SHM rejection.

## Actual old-code and causal REDs

The old ordinary package compiles with the registration regression:

```text
TestAutomaticHistoryIsRegisteredInTheOrdinaryNativePackage
storage_test.go:16: ordinary native package has no automatic rooted SQLite history source
```

Log SHA256:
`9a2646f5713c1165315c8aa5a9cbe00bc560a307a5d170a768ff90183776ef3e`.
`source-red.tar.gz` preserves the old bytes.

Before incarnation binding, the new real rotation fixture compiles and fails
`TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection/{identity,replace,legacy-replace}`:

```text
storage_sqlite_linux_test.go:295: stale database cursor survived same-root database rotation: <case>
storage_sqlite_linux_test.go:300: stale database locator survived same-root database rotation: <case>
```

Log SHA256:
`e9e1d6ffada4d23748ef0fb54106e4731e7444893351de5154f885c068e2cff2`.
`bounded-candidate.tar.gz` plus `identity-red-fixtures.tar.gz` preserve
the exact pre-binding source and new fixture.

Each independent mutation is a Go overlay, never a production source edit:

| Removed guarantee | Named failing regression |
| --- | --- |
| Automatic registration | `TestAutomaticHistoryIsRegisteredInTheOrdinaryNativePackage` |
| Fork-copy exclusion | `TestAutomaticHistoryRealWALProjectionAndSourcePreservation` |
| Unknown usage remains nil | `TestAutomaticHistoryRealWALProjectionAndSourcePreservation` |
| Complete block ordering | `TestAutomaticHistoryRealWALProjectionAndSourcePreservation` |
| WAL participation (`immutable=1`) | `TestAutomaticHistoryRealWALProjectionAndSourcePreservation` |
| Real SHM lock acquisition | `TestAutomaticHistoryTransactionExcludesConcurrentAndUncommittedWrites` |
| Deferred POSIX probe close | `TestAutomaticHistoryPOSIXProbesAndOtherReadersRetainLocks/{false,true}` |
| Cursor lease binding | `TestAutomaticHistoryPagesAndLocatorsRejectForeignAndChangedViews/snapshot` |
| Official database generation | `TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection/identity` |
| Primary physical incarnation | `TestAutomaticHistoryDatabaseIncarnationRejectsStaleSelection/replace` |
| Pre-acquisition lease check | `TestAutomaticHistorySDKChecksLeaseBeforeAndAfterProjection/1` |

## Installed ordinary-owner blocker

The exact base replay still fails:

```text
TestZCodeAutomaticHistoryInstalledListTranscriptAndWAL
zcode_automatic_history_test.go:147: installed reader wrote source family: shm

TestZCodeAutomaticHistoryDelegatedOwnerPreventsNativeEnumeration
zcode_automatic_history_test.go:277: delegated ZCode storage owner still invokes the native database enumerator: 1
```

Their log SHA256 values are respectively
`125140d8fe629b321e98a1d9ae5157fc8fcefb3e3084e6251a8000ad3481d0f8`
and `227697434bf0ae7c5726fe0ae69274d9ba2778692d3743b5907440f74a5caffd`.
The actual installed rooted owner independently preserves all three source
hashes and passes fresh WAL/uncommitted/selection checks. This separates
module reader acceptance from the ordinary host enumeration defect.

`allFilesWith` calls `zcode()` before delegated rows are filtered; the
existing `storageNativeFiles("opencode", openCodeFiles)` already shows the
generic owner guard. The smallest proposed host correction is
`storageNativeFiles("zcode", zcode)`. It was sent to the leader, with
AUTOMATIC-HISTORY-HOST-HANDOFF.md and this fixture. No host production overlay
or fix was applied, and ordinary read-only acceptance remains RED until the
owner supplies the correction and its pin.

Overlay SHA256:
`70d62da159f94ec76451861f0a9cafb937b0b6554cf82732da7d87e355fec4d6`.
Additional fixture SHA256:
`2f3f6779d842806c34b511abfdaf20d4bc2f98e08590881ec542be5d8c51ed73`.
Each installed scenario uses a fresh host process. Initial multi-install
fixture attempts hit native-image dependency-equivalence refusal; an early
unanchored `off$` selector also matched `foreign-off`. Those harness failures
remain in earlier remote directories; the final selectors are anchored at
both subtest ends. Neither failure is counted as a causal product negative.

## Artifacts and evidence

| Artifact | SHA256 |
| --- | --- |
| Installed-test Linux ZIP | `85c633ca9f5583409cf04be1b2e4df8b829add4e9e4bcf5d7b7ff4ed07b6be56` |
| Final handoff ZIP, documentation-only repack | `781c3bb4dca212f19ab5fdfc5b0899fc2731a48983cd9d9fd0b053c818f8a205` |
| Installed production `lib/zcode.so` | `c78916bffd07e3faa8fb94acd260f183c19f1a4106b94e424732fa810d36a886` |
| Production IIFE | `657759b0ed175fa0b6e6f4024ca0a314ca1d3bdf081b29e548b381babf0f2658` |
| Repacked API10 handoff sidecar | `9686b5305b58973bf99b564b8b3ee17606763bf46757dac104ab3a440f538445` |
| Final handoff isolated ABI log | `e89de04825caa6c4a238e21e4723f20d9a8ecbaa03b4a4c8b2fcc3d7160d7615` |
| Supplemental handoff evidence archive | `063f4d8858ff393be2ca47408876433fe7f969a23df5b0712fa76b092201dabb` |
| Focused/full-module result inventory | `4a5962765f26dd05e8fbb3b21f87fff94e11a705942c5c324e9eca3644f4865e` |
| Causal result inventory | `6b3f1a9a8cb0c8908eedc76fcf65f19f36f882995038d2d2a5dbac44a05b7caa` |
| Static result inventory | `e67131b93b1325a1e46b50d63810aa6364d3c5c9e3f4717fba9c8b2f9d0f24bf` |
| Package/ABI result inventory | `7c7420dcd2ed973bd8e70516a9fff08ff27daaaea5493e8eded8a9e5a45be50e` |
| Installed result inventory | `6fa71a2f4e4e9bcf2d13514b76ba704977834f821fe8203a771187febb7753a5` |

Local copies are under `dist/zcode-auto-history-20261115-APFz8QwX`.
The complete evidence archive retains old sources, RED fixtures, aggregate
timeout, final exact commands/logs, all mutated Go files/overlays, the
additional installed fixture, and the tested source freeze/inventory.
Its `complete-scoped-evidence.tar.gz` SHA256 is
`410adbfa2d7918d352021edf6e632c1102b0fa6372c537f93da81c25bc603770`.
Both ZIPs retain the exact installed executable payload. The final handoff
ZIP and its separate repack evidence are under the local `handoff/`
directory and remote evidence root's `handoff/` and `handoff-input/`.
`handoff/handoff-evidence.tar.gz` retains the repack script, sidecar, result
and ABI log. `handoff-results.json` records every entry hash, the exact conformance argv,
zero capabilities, loopback-only isolation, pinned offline caches, and the
before/after source checks. `repack-handoff.py` retains the entire operation;
its only subprocess is the existing `conformance.py`, which returned
`passed:true`. No host suite or rebuild was part of that final check.

## Remaining scope

Rank1 checked source-bound argv resume is the requested generic API10
addition. Duplex planning belongs to Linnaeus; actual V4 snapshot/patch/query/
CAS, reverse authorization/cancellation and process-loss financial
reconciliation remain unimplemented/unverified native adapter work.
Default CLI history does not claim separate desktop task-index/custom-root
association, automatic financial DB import, external body hydration or
additional model/turn/tool-usage dashboards. The manual reader retains its
raw source-preserving behavior. These are distinct from the implemented
message history projection and are not requests for another SQLite core API.

No actual vendor/native client, rendered browser or Darwin/Windows native
runtime was exercised. Unsupported rooted WAL platforms explicitly refuse
access. Full native operation and ordinary merged-host read-only history
are not declared complete by the green module slice.
