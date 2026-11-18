# Frozen generic-dispatcher installed replay

2026-11-15. Bounded packet for the host owner and designated independent
reviewer Raman `01a1dcb8-7d49-7813-9703-b2a32e78d66d`.
Both original installed ZCode regressions now pass on the leader's generic
dispatcher candidate. This does not close the reviewer's other operation
ownership findings or establish a merged host commit.

The module remains the tested `56002ef4e178d94bbcdb23d382398025a8dd343d`
production slice: ordinary 0.15.0, API1.9, ABI1, Resume none. No module
production, fixture, native/UI payload, host checkout, index or commit was
changed for this replay. Only this plan and evidence document are new tracked
module files. The pre-replay plan is F2-HISTORY-CLOSURE-PLAN.md.

## Exact combined source

The leader supplied the generic dispatcher at
`usbox:/tmp/magpie-native-history-dispatch.OhIcINe3`. Its
`dispatch-overlay.json` supplies exactly seven files over immutable base94.
The replay copied that base and those exact files into an independent
read-only source directory, without unrelated resume/process changes.
The test execution overlay adds only the original module-owned test fixture.
It replaces no host production file.

| Identity | Exact value |
| --- | --- |
| Supplied base commit | `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55` |
| Base committed/physical tree | `94ec4c8de4e5b366a6e3fb854df2176007587ab8` |
| Original base inventory capture parent | `adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d` |
| Combined physical tree | `b3199e9ed194130e5bf7b7eb60ab01a55c20f723` |
| Combined source paths | 10852 |
| Combined source archive SHA256 | `a32f6ce5fb3dfec95451bdb793c527ea572fae78bcbdf5e5498ef88115ef3fbf` |
| Combined source inventory SHA256 | `874c021af417e337d36d8da342db1335f19ea30f520aab5f691dd2fd6491d19c` |
| Leader dispatcher overlay SHA256 | `0fc86852ffd36841e7b84ebddf50deede4f3276298a6a20a879b4687f31c764b` |
| Replay fixture-only overlay SHA256 | `d212dd6e539d923d4643e0260117357bd3022659e52b7cf4f49e8d1e74163a41` |
| Unchanged original fixture SHA256 | `2f3f6779d842806c34b511abfdaf20d4bc2f98e08590881ec542be5d8c51ed73` |
| Unchanged synthetic SQLite writer SHA256 | `9109721964ce72368086ab9543ff08bffc56da39a831d230c1fe6101562bcc44` |

These are source identities, not a new host binary or merged commit claim.
The separate Magpie CLI binary is not invoked by these two tests: the
compiled host sessions tests use the actual extension Manager to install
and execute the frozen native/UI package.

The seven frozen host inputs, relative to the source root:

| Path | SHA256 |
| --- | --- |
| `internal/sessions/native_history.go` | `b7c63ad5506b96037ed2a99a5b43425c9c92dd559a63dcf50201cd226218bff0` |
| `internal/sessions/native_history_test.go` | `1595e93c70b893ea9288e700ce28335210f158a6f70eead7751c92f1af3ba433` |
| `internal/sessions/sessions.go` | `fb3b88b2b06ebf35f109714ecb2e66435e82e5257abeb3e5157377541f1f1189` |
| `internal/sessions/calls.go` | `9b879cba91f5323c128696073a9ddd99d9d0994d2b54e89ea68a4958866dc716` |
| `internal/sessions/extension_source_registry.go` | `e129918261d8e8a9ca4dc90cf81d0daa37affb45b3814ebe36c0bfdcdbbd9e8d` |
| `internal/sessions/extension_source_adapter.go` | `1ad73d6423eafd022e9ee28d1b696aabb25a322e5a6032f415817807623995c7` |
| `internal/sessions/extension_storage_enumeration_test.go` | `0b7724073b45a84fad5835e64b5a4a6d2d61ca95c97d9bbd6ec9443bd5a6d5a5` |

## Registry inventory and callers

The frozen `native_history.go` declares these 25 local readers:
claude, claude-desktop, codex, opencode, pi, raft, cindy, zcode, dsh,
cline, qoder, qoder-cn, grok, workbuddy, workbuddy-ai, codebuddy, droid,
omp, cursor, hermes, alma, openmausbot, kimi, agy and antigravity.
Its two grouped WSL selections are `claude,codex` and
`pi,raft,omp,kimi,antigravity,agy`.

`nativeHistoryLists` checks the selected view's ownership and Reads before
calling a local reader or passing a filtered group to `wslFiles`.
`allFiles` at frozen `sessions.go:1001` and `allFilesForTitles` at
`:1006` both enter `allFilesWith`, which dispatches through that registry
at `:1012`. The permanent host regression covers native, delegated and
stale delegated ownership through both normal and title slots.
`nativeSourceOwner` in `extension_source_registry.go:181` derives owner
membership from the same registry; the binding-error path in
`extension_source_adapter.go:59` uses that inventory too.

This documents the dispatcher surface, not proof that its downstream
grouped WSL implementation performs no unrelated I/O. The reviewer's
separate WSL receipt remains material below.

Accounting is intentionally separate: frozen `calls.go:309–320` keeps
`callFiles` / `callFilesWith` / `callFileLists` on their own Claude/Desktop/
Codex/WSL accounting readers. This packet makes no claim that history
ownership suppresses those call/trace readers.

## Unchanged original assertions and results

The original base94 failures remain in AUTOMATIC-HISTORY-EVIDENCE.md:

```text
TestZCodeAutomaticHistoryInstalledListTranscriptAndWAL
zcode_automatic_history_test.go:147: installed reader wrote source family: shm

TestZCodeAutomaticHistoryDelegatedOwnerPreventsNativeEnumeration
zcode_automatic_history_test.go:277: delegated ZCode storage owner still invokes the native database enumerator: 1
```

The original fixture is copied byte for byte, with no assertion, logging,
timing or skip changes. Each named test runs in a fresh process.

| Own usbox replay | Result | Log SHA256 |
| --- | --- | --- |
| `TestZCodeAutomaticHistoryInstalledListTranscriptAndWAL` | PASS, exit0 | `04b415de01ab9b2cabaabe22d16405e2b8e645290553c2a39cc0d0e29dd4ca0e` |
| `TestZCodeAutomaticHistoryDelegatedOwnerPreventsNativeEnumeration` | PASS, exit0 | `63a15403f3dcea452eedfb0adca25a0217b93427aa33378f85cc2339c3b9369c` |

There are two named PASS events, no named FAIL or SKIP, and no race warning.
The first test retains its exact DB/WAL/SHM before/after hash assertions,
ordinary Find/ListAgent/TranscriptOf/SearchSession checks, exact counters
above 2^53, committed-WAL visibility, uncommitted exclusion, old-selection
rejection, external-body boundaries and post-read checkpoint completion.
The second retains the exact zero native-enumerator-call assertion.
No numeric source-family hashes were added to or logged by the unchanged
fixture; the existing equality assertions executed and passed.

Both commands use the pinned Go 1.26.8 and these exact flags:

```text
go test -p=2 -tags=nogui -overlay <replay/host-overlay.json> -race -json -count=1 -timeout=2m -run ^<exact-test-name>$ ./internal/sessions
```

The retained runner records each complete argv. It uses the existing
`testdata/agent/verify.py:isolate` loopback-only namespace guard, zero
capabilities, a fresh HOME/XDG/sandbox for each command, explicit pinned
GOCACHE/GOMODCACHE/GOPATH, GOPROXY/GOSUMDB off, local toolchain,
GOMAXPROCS2 and GOFLAGS `-mod=readonly -p=2`.
TMPDIR is unset, matching the established verifier.
All 10852 combined host paths, 228 module freeze paths and 21 package
entries were checked before and after the two tests and remained exact.
No broad or whole sessions suite ran in this lane.

The leader's separate registry race20 receipt, retained as
`leader-dispatch-result.json`, reports 640 PASS and no failures/race warnings.
That is the leader's run; it is not counted as another module-run gate.

## Frozen evidence and unchanged package

Remote evidence directory:
`usbox:/tmp/zcode-auto-history-20261115-APFz8QwX/f2-dispatcher-closure`.
Local retained directory:
`dist/zcode-auto-history-20261115-APFz8QwX/f2-dispatcher-closure`.

The complete `f2-dispatcher-closure-evidence.tar.gz` is alongside those
directories. SHA256:
`9d642f022ec2a19f37a9c23d3b6542ef834e00764d6db76305356a443ce0706f`.
It retains the combined source archive and inventory, exact seven-file
diff, original leader overlay/result, unchanged fixture and its overlay,
both logs, `freeze-f2-dispatcher.py` and `replay-f2-combined.py`.
The replay result inventory SHA256 is
`a481bcbb960f4dab1b203cc23a9d414c4acf0fec9f3ecd14093e8b075d4c8d81`.

The installed distribution remains
`usbox:/tmp/zcode-auto-history-20261115-APFz8QwX/coherent-package/distribution`.
Its native SO SHA256 is
`c78916bffd07e3faa8fb94acd260f183c19f1a4106b94e424732fa810d36a886`;
IIFE SHA256 is
`657759b0ed175fa0b6e6f4024ca0a314ca1d3bdf081b29e548b381babf0f2658`.
The final handoff ZIP SHA256 remains
`781c3bb4dca212f19ab5fdfc5b0899fc2731a48983cd9d9fd0b053c818f8a205`.
No native rebuild or documentation-sidecar repack was needed.

The earlier tree `3f760340` is not this candidate. Its two attempted tests
stopped at the testenv TMPDIR/sandbox guard before either named test ran.
That harness refusal is preserved separately as
`f2-combined-gtwr41t0/intermediate-harness-refusal.tar.gz`, SHA256
`7bd263018e454fb085e0cdc1b4b4f8a6f0b1c3bc7eb5862555010feb07e11948`.
It is not counted as an F2 assertion RED or PASS.

## Remaining independent findings

Raman reported three independent boundaries with `-race -count=3` failures:
held-native SearchSession after delegation; WSL scan/cache of delegated Pi
behind a filtered grouped selection; and saved rooted-storage consent with
a missing/corrupt catalog publishing an empty owners map. His receipt roots
are `/tmp/magpie-astra-owner-boundary2.bYaotDUU` and
`/tmp/magpie-astra-storage-unavailable.ryNIvuko`.
These are reviewer-run results, relayed to the leader, not rerun here.
Their source identities remain those of his read-only architecture report
at the host's
`.omx/evidence/astra-comprehensive-review/owner-boundary-audit/review.md`;
they are not substituted for this packet's `b3199e9e` combined tree.
The two green module tests do not exercise those failure cases or settle
the common operation-ownership contract. General F2 remains open pending
the designated independent review.

API1.10 source-bound agentResume adoption and live V4/duplex work remain
separate. No actual vendor, installed native application, live account,
credential or rendered browser was used.
