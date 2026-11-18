# Frozen native auth-broker consumer packet

2026-11-13. Two complete independent fixture runs are green:
`positive3` and `positive4`. They use separate HOME/Store directories,
new intent/flow identities and separate long-running web processes.
Neither process restarts after reauthorization.
Consumer-proof review is owned by the leader; final consumer commit
remains pending that review. This is not a published/installed SDK claim.

## Exact host and modules

Final host basis only:

- source tree `7f0f63b30c5859bce19cdca90f0e114ee979a212`;
  explicit main a721 + corrected Auth, all 30 Auth files identical to
  the corrected 5241 snapshot, including production fix 910b242;
- immutable source `usbox:/tmp/magpie-auth-main.1lBjaRys/source`;
- host binary SHA-256
  `253f76efb7ffbdf9553a79b74252897f5b988175ca5b6222b81e504d415ab586`;
- complete source inventory SHA-256
  `a278427150cc3dd39829c5768d490d4144d043987fab8703b1cad41baedb6d29`.

ZCode production checkpoint is `5b20682`, including the separately
verified ExecuteOptions correction. OpenCode source is read-only commit
`031d0931350c88e4875f11d89639110f5398dffc`.
The combined module-source inventory SHA-256 is:

`d1b16f00a239cd45d873e37eb0258697278cbb99683d965e4e13ea8a9d5811fe`

Host source is never edited or linked to permanent provider dependencies.
Temporary C-shared wrapper modules import only public SDK/domain packages.
They retain the actual vendor auth handlers, selected-account validation
and usage controllers. They add a fixture Window/AuthBindings declaration,
a sanitized selected-account probe and bounded test-only usage controls.
Production manifests/page assets receive no auth binding or auth button.
The committed default headless API-1.4 and UI-1.5 distributions remain
separate.

## Frozen six-file consumer inventory

Directory: `testdata/window-auth`.
Sorted JSON manifest SHA-256:

`d1290c9496929456b5a7f1b995260e97b9ca90a00718e502653edcb6711e1eb9`

| File | SHA-256 |
| --- | --- |
| driver.mjs | `031188a79ca292ac7cdef2ec6a2d28546c706c561472c9e9d03efb7efd740c1f` |
| realms.mjs | `9a9362d8fcbdd154ab177e647224a5c89722de8b506147caf1555c57ad600fa2` |
| realms.test.mjs | `90651082c0f6493d4f926a56eb3a928e8f18309be39a51cef013ae8a8c6b79ff` |
| run.py | `6722a57febb14186d3f5af0fa56afea3a8bc33c726d88675285035eb18b97150` |
| vendor.py | `74df997c5ae4b0571c3d0d55c4db86f8649d5fbbd4e4b440b27f826cbe3d5409` |
| wrappers.py | `1eb9a68f867fd67768c9d1ab55f4983ce9efe181727eb80ed07dfe6a584c9786` |

The runner compares the SDK inventory and module/harness sources before
and after execution. It also verifies the actually served pagebridge and
parent relay against the frozen source, plus the public browser SDK.

## Real execution path and authority

The actual public browser SDK and served PAGEBRIDGE execute in a frame VM
realm. Messages cross into a separate parent VM executing the actual
`answerPluginPage`. Its parent-only API transport performs real ordinary
web HTTP calls with the temporary page cookie and current immutable ref.
The real GUI auth route captures the host owner/lease/Store and calls the
actual native ZCode poll/link or OpenCode device controller.

There is no CLI auth shortcut, direct account JSON write, fake auth
rotation, injected auth result or duplicate credential store.
CLI commands are programmatically limited to setup `add`, `config` and
`off`. Only the fixture's own temporary Store is read to verify receipts;
no live app/account/session/host-secret directory is inspected.

Frame fetch is unavailable in the harness. Both iframe directions and
all parent HTTP auth responses are audited for credential/Continuation/
PKCE/device material and exact synthetic canaries, including failures.
A privacy violation remains recorded even if an expected rejection
would otherwise swallow its exception. Authorization URLs/public user
codes, opaque host flow handles and sanitized account summaries are
allowed. No PKCE/callback protocol support is inferred from these
poll/device consumers.

## Terminal assertion matrix

Each run completes all rows:

| Assertion | Observed terminal proof |
| --- | --- |
| Actual native ZCode poll/link | Initial account G1, same selected account reauthorized to G2. |
| Actual native OpenCode device | G1 device account, ordinary gateway host-owned refresh to G2, same selected account UI reauthorization to G3. |
| Atomic publication | Exactly two saved accounts, four complete UI auth receipts; IDs/expiry/current generations match the sanitized views. |
| Correct selected Frame.Auth | Real module credential identity validation and usage handler run with G1/G2 ZCode material and rotated/G3 OpenCode material; no auth digest returned to the iframe. |
| Dirty usage survives reauthorization | Public SDK invalidation occurs once before ZCode reauthorization; revision 1/ack 0 survives unchanged. Informational action usage reads do not acknowledge it. |
| Ordinary routing barrier | Gateway succeeds before invalidation, refuses both before/after reauthorization with a quota-barrier reason, then succeeds only after ordinary GUI quota refresh performs a real matching-revision read. |
| Completed refresh receipt survives | OpenCode's completed ordinary-refresh receipt fingerprint is byte-equivalent across selected G2 → G3 reauthorization. |
| Duplicate/changed start intent | Exact same start message returns its existing flow without a second vendor init; changed method or changed valid-method input is refused. |
| Duplicate poll/cancel | Replayed exact message IDs and further terminal calls return the bound receipt; no repeated vendor exchange/publication. |
| Root/parent/binding authority | Wrong module root, different frame-parent owner and undeclared binding cannot poll another flow. |
| Stale account generation | Prior selections fail after reauthorization; new selections use the exact new generation. |
| Cancellation during exchange | A held OpenCode selected-G3 exchange is canceled; its issued material cannot publish G4, and its terminal uncertain/canceled receipt cannot re-dispatch. |
| Module off | Old page refs cannot start, finish or cancel after setup disables the corresponding root. |
| No replay or paid fallback | Exactly two ZCode inference POSTs, one OpenCode inference POST and one ordinary refresh; blocked/canceled operations reach no inference endpoint. |
| Privacy | No private credential field/canary appears in iframe messages, auth web responses or ordinary web/driver logs. |

The fixture-only usage gate emits ReadDeferred for background
`provider.usage` reads while checking the barrier. It never repairs,
restores or re-invalidates account metadata after reauthorization.
On release the actual ZCode API-1.4 handler produces the fresh result.
OpenCode's existing API-1.3 handler has no revision echo; the fixture
does not invent a revision acknowledgment for it.

Each run records **160 iframe messages, 71 parent HTTP calls**, vendor
init counts ZCode 5/OpenCode 6 and poll counts ZCode 5/OpenCode 6.
All synthetic vendor errors are empty in the terminal successful runs.

## Terminal files and isolation

Artifact root: `usbox:/tmp/magpie-auth-final-consumers-OfzeV5lp`.

| File | SHA-256 |
| --- | --- |
| positive3/result.json | `b11a9eb529c1fbde362e31411b6de0c3dd27a9452e310f88510f743f28926e8f` |
| positive4/result.json | `b11a9eb529c1fbde362e31411b6de0c3dd27a9452e310f88510f743f28926e8f` |
| positive3/public-transcript.json | `06bdaba1023276c057bbab6a264760331db0de050c00c023ae20d76f6a06ad76` |
| positive4/public-transcript.json | `5c41d3aa069fff4e9f10fcd9d10bd0849db3775c2c10643ff785959ee7778453` |

Both terminal exits are 0 in `final-consumer-results.json`.
Same result metadata hashes do not imply reused flows: the transcripts
and temporary HOME/Store paths differ.
Execution uses usbox only, Go 1.26.8, offline pinned caches, UID/EUID 1000,
all five capability masks zero and loopback-only interfaces in a private
network namespace. No new npm/OS/browser dependency, adm/local-Mac
execution, push, installed activation or live vendor is involved.

## Historical red provenance

These remain separate red results, never relabeled by the final passes:

- `/tmp/magpie-auth-consumers-DmHmGRPY/run1` on revoked 6f292:
  fixture expected ZCode's read-only 503 polling to terminate, whereas the
  real controller correctly returns pending for transient poll failure.
  Terminal auth refusal is used for that error scenario instead.
- `run2` on revoked 6f292 reached both controllers but the fixture
  flagged an unhandled synthetic GET; later runs record its exact path.
- `run3-corrected` on pre-composition 5241 preserved the dirty epoch, but
  its synthetic balance lacked total capacity. The real module correctly
  marked usage unknown and did not acknowledge it. The fixture now
  supplies authoritative total_units 10/remaining_units 7.
- `run4-corrected` on 5241 exposed the actual owned-module escapedPath
  incompatibility. The public-SDK regression, real old-code HTTP 502 and
  separate fix commit `5b20682` are in EXECUTE-OPTIONS-EVIDENCE.md.
- Final-composition `positive1` completed the ZCode gateway/barrier
  assertions, then hit an incorrectly doubled synthetic OpenCode URI.
  HTTP 409 accepted-uncertainty/no-replay was retained; no admission or
  retry rule was relaxed. The next run used a new sandbox/intent.
- Final-composition `positive2` completed both native consumers but
  rejected the synthetic update-feed GET `/updates?…` at the fixture's
  final assertion. Only test path/query handling changed; the feed still
  returns 404. No broker code changed.

## Limits and review boundary

This proves the actual JS bridge/parent → ordinary web HTTP → captured
host Store/native-controller chain. VM documents are synthetic:
**not rendered-browser, visual, browser-origin enforcement or full-app
DOM acceptance**. Auth-broker callbacks, API-key imports, real vendor
accounts and a combined SDK release are outside this proof.
Final consumer commit awaits the leader's bounded independent review of
this six-file frozen harness and terminal evidence.
