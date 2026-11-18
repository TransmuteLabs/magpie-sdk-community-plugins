# Packaged browser RED and withdrawn admission experiment

2026-11-14. The closed-socket admission experiment is **withdrawn**, not
accepted. Active `testdata/agent/ingress.py` is byte-identical to module
baseline `9bec47b033405414bfa761e639d3a77f19b84d2d`:
SHA256 `8c53c6b5a2f7e1ae00b66e54b4d933cc9e31b40510ac9d0064ac3a39680ecf12`.
Unknown or vanished ownership still becomes a vendor error before HTTP.
No cached tuple, JSONL record, header or process name grants admission.

The experimental ledger, guard, admission tests and browser runner are
preserved as inert archive members, outside active fixture source. Their
source is not a supported runner or an accepted security boundary.
The module stays 0.13.1/API1.9, with provider, controller, agent and UI in
one ordinary package. Production Go/TS, native libraries, package ZIP,
source-lock, dependencies and shared SDK source are unchanged.

## Preserved sources and independent review

The complete eight-file WIP candidate, both fixture patches, all three
remote source variants, baseline source, independent review, gate logs,
connect/ingress evidence and browser RED observations are in
`testdata/agent/history/browser-ingress-20261114.tar.gz`.
Its adjacent JSON manifest records every member SHA256 and the archive
digest. The113-member archive SHA256 is
`088e272aa6d8ad2727894fcacb897c54bb59bf87d6a8c48577ce023dd4b75269`.
It also preserves the later unused-reset regression draft,
separately named and explicitly **not executed**. No retained-FD proposal
was implemented or tested.

The private full downloaded packet remains in
`dist/ingress-admission-f7-QA/preserved-remote-packet.tar.gz`, SHA256
`06e03a22af741e10619e25722896d99718b76b922fab2a1030dc9ec2b339205e`.
Original remote files remain untouched at
`usbox:/tmp/magpie-zcode-ingress-admission-f7-rgsG8ph9/`.
HOME, extracted distribution and compiled binaries are not copied into
the committed archive. Private environment/configuration copies stay in
the local evidence packet. Synthetic source and reviewed logs are archived.

Godel's independent source review is preserved verbatim:
SHA256 `7243d5b2cb6d35cda1c45734cd40ddf31cd4bb8511a36f6fa6c1cec8ca6c969b`.
Its verdict is **CHANGES REQUESTED** for the experimental authority:
one authentic, unused old record can match a later foreign closed
connection reusing its tuple within five seconds. Old PID/startTicks,
credentials and single consumption authenticate the old event; they do
not bind the newly accepted TCP incarnation when no live inode remains.
The seven passing admission tests and consumed-record replay negatives
do not close that missing case. They remain their actual historical
results, without an admission or browser acceptance claim.

The second review finding is the archived runner's unhandled outer
180-second timeout. The completed browser case exited normally at its
failure and did not exercise that branch. A small separate owned-command
wrapper and synthetic cleanup tests are described below; the unsafe
archived browser runner is not executed by that wrapper.

## Three historical browser boundaries

The initial rendered case completed synthetic native sign-in and a
selected-account read, then failed the unchanged strict vendor assertion
on three connections with unavailable live owners. Its receipt remains
`5be4223d40e9dc3987163634c6c3fc6a02818243a7fbf2b692643034a087b018`
at `usbox:/tmp/magpie-zcode-browser-f7-Fw7yi7Oo/`.
Ports43908,43914,43988 remain historically **unattributed**.

A separate diagnostic case attributed its three new connections at
connect time to Devin PIDs1931132 and1931200, children of GUI
Magpie PID1930709 under fixture PID1930006. Its live-owner lookups
occurred after socket/process disappearance. That establishes the
reproduced attribution race, without assigning the first three senders.
This case also remains RED; its receipt is
`57795c716cc69f34dbf6abf2f56521589369ed08da04d9407cde74cb83b15786`
at `usbox:/tmp/magpie-zcode-browser-attribution-f7-ADrRLL7M/`.

The sole post-experiment browser case is **RED, exit1 after17.02s**.
No additional browser case, replay or native/host rebuild followed it.

## Read-only triage of the 17.02-second auth failure

The saved stack is:

```text
page.waitForResponse: Timeout 15000ms exceeded while waiting for event "response"
    at authResponse (.../source-browser/testdata/agent/browser.cjs:74:37)
    at main (.../source-browser/testdata/agent/browser.cjs:76:20)
```

The waiter watches `/api/plugin-pages/auth`. The initial ordinary backend
request `/api/plugin-pages/accounts` returned HTTP200 with an empty list.
No auth response was captured. No vendor OAuth init/poll request, saved
account file or native-account proof exists. The final iframe DOM still
has the initial auth status and the completed empty-account-list status.
This does **not** establish whether `auth.start` reached the parent bridge
or core handler. Request-start/failure events, message snapshots on failure
and core access/native-operation timing were not captured.

Observed page JavaScript errors are `[]`; browser console messages were
not instrumented. Three served assets returned200 and matched their pins.
The ordinary GUI parent and packaged iframe rendered; early sandbox,
parent-document/storage isolation and theme assertions passed. Auth,
account persistence/selection, selected-account action, final ledger,
vendor-error, CSP and secret-canary assertions were not reached.

The ingress report contains14 requests, zero records labelled
unattributed and one positively identified outside-process rejection
(moshi-hook PID24086). The candidate labelled two CONNECT requests
closed-socket admissions:

| Sequence | Client → synthetic proxy | Captured caller | Target |
| --- | --- | --- | --- |
| 21 | `127.0.0.1:37864 → 127.0.0.1:34361` | Devin PID2156716 | `unleash.codeium.com:443` |
| 30 | `127.0.0.1:37934 → 127.0.0.1:34361` | Devin PID2156782 | `unleash.codeium.com:443` |

These observations do not remove the independent incarnation blocker.
Other recorded requests used the live-owner path. Synthetic CONNECT
responses were403; two already-closed peers produced BrokenPipe traces.
GET `/updates` was observed. There was no live external connection.

Relative to scenario start, GUI PID2156425 spawned at737.765ms,
the browser driver at874.556ms, the two CONNECT records at1708.630ms
and2024.999ms, GET `/updates` at5860.330ms and the outside rejection
at10212.812ms. The driver exited1 at16710.570ms; the GUI was then
terminated and joined with exit-15. The context/browser finally and
guardian completed normally for this RED; no observed orphan is claimed.

The guard report has33 connects,33 proofs,5 binds,1012 denied operations,
zero Unix connects,one Unix bind and childExit1. All33 captured connects
returned EINPROGRESS; the largest connect-to-metadata stamp interval was
0.244249ms. The last connect was at5810.266ms. There is no per-denial
operation/timing trace. These aggregate values neither identify an auth
denial nor exclude a guard-related delay.

The auth cause remains **unknown**. No product/SDK defect, admission
publication failure or fixture coordination cause is inferred. A future
explicitly authorized diagnostic would need the missing bridge/request
boundary observations; a timed browser rerun is not evidence of causality.

## Exact browser pins and immutable RED receipts

Only this earlier f7 host was used, not the later SDK milestone:

| Input | SHA256 or tree |
| --- | --- |
| Physical source tree | `f7f26b2d5ee5388a089ab07a72f7372ed0068971` |
| Host binary | `9ddc4e4a076dbcc119e71b5c3b140253e8c419f3278dfadc5d8cdadd59c20adb` |
| Full ZIP | `45d1e73c30379b5911bc1c47c64928828f12988692c66b7611be70e7f681125b` |
| Production native library | `1b408ffbb73a7db0874d9321777947c5eb89d25c66e2c089fa84b3f12c686bc3` |
| Synthetic-endpoint library | `57c6c01915dd6b46cde2c031394f6ee5a7972023f0887021930e8a6183122327` |
| Unchanged browser driver | `cdc65960416c14bdb13da8279e0e7c4dbff49b67f3e0757d5f22b84789bc3e36` |

Chromium151.0.7922.34 and Playwright1.62.1 were pinned. All197 original
module source files and the package/runtime pins matched before and after.

Under the remote root's `browser/`:

| Artifact | SHA256 |
| --- | --- |
| `result.json` | `340640747500b4897c7ee2aca459614bc48b3559f2779ede1a9b7226e953dd0e` |
| `browser-error.json` | `8913e5d6ae7dae658b3e47bb85486b14ce940cc0b83e7236aa3683ff0a889801` |
| `ingress.jsonl` | `f15e74f4e1c691cbab188b3879e8ca7daffc3053a40393d66404b7cfc1b41a0e` |
| `network.json` | `c7e58e0fb7c26da9ddf661dbd7371a02c356acee46251b1956a79913f81a2275` |
| Identical before/after pins | `538ed190a0e4397ce85fb31258ea53190094cd85b703bf5c7426c87ddeee5c03` |

## Bounded cleanup and active-boundary checks

`testdata/agent/owned_run.py` accepts an explicit command, starts a new
process session and retains its leader unreaped with `waitid(WNOWAIT)`.
Before signalling it checks PID/startTicks and the separate session/group.
It sends TERM then bounded KILL to that owned group and joins its direct
child. The unreaped leader prevents PID/group-ID recycling during cleanup.
It does not scan, signal or join unrelated processes.

Timeout remains exit124, and an ordinary command's failure remains RED.
The wrapper does not launch the archived experiment automatically and
does not add any socket admission authority. It covers inherited process
groups; moving to another process group, including within the same
session, or another session is outside its guarantee.

`cleanup_test.py` uses only synthetic guard/child processes and an unrelated
control. Tests exercise timeout escalation, a successful leader with a
lingering child, identity mismatch, shared-session refusal and preservation
of a command error. The same behavior tests against the old bare waiter
must fail because owned processes remain running. Exact final results,
source hashes and logs accompany the closure receipt.

| Scoped usbox gate | Terminal result |
| --- | --- |
| Owned-group cleanup, identity/session refusal, failure preservation | 5 PASS,1.105s |
| Same two lifecycle tests against the old bare waiter | RED,2 behavioral failures,2.905s |
| Unchanged baseline ingress tests | 4 PASS,2.284s |
| Python AST checks | PASS |

The two negative failures assert that actual owned processes remain
running; their test finalizers then kill and join only those synthetic
children. The unrelated control survives. An earlier ingress-only
attempt omitted `/usr/sbin` from its isolated PATH and failed to find
the already-installed `ss`; that RED is preserved. The successful attempt
uses the same source with `/usr/sbin` included. No dependency was installed.

The original four ingress regressions and the baseline ingress digest
remain the active acceptance boundary. A closed client whose ownership
has vanished may still make a browser fixture RED; it is never silently
admitted. No browser pass or repaired auth result is claimed.

Actual installed ZCode, live accounts/vendors, macOS/Windows execution,
automatic DB/WAL capture and financial resume remain unverified. The
independent SDK-only rollout and its committed acceptance are unaffected.
