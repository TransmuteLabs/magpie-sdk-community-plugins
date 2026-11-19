# Devin source map and update guide

The machine-readable source of truth is `source-lock.json`, schema 1.
It pins immutable commits and artifact SHA256s, not branch names or latest
releases. Sources are read-only evidence, not packages installed at runtime.

The November 19 comparison uses committed `Agents/devinapp`
`e977587dd6a76ed2125ea7e09ac4771035100d35` (reconstructed CLI `3000.11.228`).
`client/account_upload_wire.rs` and `client/plugins.rs`, present since
`3a4db5e2fa6170c97ef170616a295b43b532b31e`, define the lookup used by
`plugin_source.rs` for AccountUpload retrieval, preview and revision checks.
Version 0.8.1 serves that separate read through the selected account. Bundle
IDs follow the segment grammar in `plugin-host/src/discovery.rs`. The wire
regressions cover the three scopes, replacement of caller metadata,
malformed fields, and exact success/absence responses. The native client
handles the download. The reconstructed and shipped clients still need live
verification with an assigned cloud bundle.

Since `4d399385deadca25115b37b430a5c7cccb2c46c4`, the recovered
`provider-proxy` adds Connect framing, protobuf decoding and a transport state
model. GetChatMessage uses the same Connect media type, version and frame flags.
The specification still leaves the request/response fields of
`RecordTrajectorySegmentEvents` unresolved. Official community commit
`836f2e4e4cd899318c422d67cc1b2b30baaf1e06` has the same Devin 0.1.45
package tree `1c23cc5c881c36b027f8cef2feb70f7a49ada5e4` as `324acd6007`.

The local `Agents/devinapp` comparison at `dfd5a590a3d4c3a89e79fe90b3348015d8e6f1e1`
ties the response field numbers to the `3000.11.228` binary in
`reference/docs/provider-protocol.md`. The paired decoder already uses the
confirmed message ID, nested timestamp, output ID and request ID fields.
The paired route preserves opaque notifications and model changes. The
standard bridge rejects those controls explicitly. The codec regression
fixture exercises both paths with distinct message, output and request IDs.
In this recovered source, ACP `initialize` prefetches user status and team
settings when credentials are available. The separate
`GetAccountManagedPluginBundle` RPC belongs to the cloud-backed `AccountUpload`
source. Retrieval and preview consume the bundle, and revision checks can use
just the token from the response. The default bootstrap handler remains
unchanged. Startup with an assigned bundle in the shipped client still
requires live acceptance. See `reference/crates/chisel-agent-bin/src/acp.rs`,
`plugin_context.rs`, `plugin_source.rs` and
`reference/crates/windsurf-api-client/src/client/plugins.rs` at the pinned
commit.

The November 18 comparison reached `4d399385deadca25115b37b430a5c7cccb2c46c4`.
The four commits after `e9d40139` change eight reconstruction files for
extension permissions, skill imports, browser preview and telemetry.
The watched protocol and model inputs are byte-identical across this
interval. The `account-upload:` string handling formats a skill key.
Managed bundle retrieval uses `GetAccountManagedPluginBundle`.

The November17 bounded update adopts Devin0.1.45's per-read quiet watchdog
from exact community `324acd6007f2d513c5072fe2e9fe1db3524ae03d`,
behavior commit `e30f8ed0561f8deeca30964cfdefa0799fe704d3`.
One shared body reader covers decoded `Complete` and raw `Native`.
Each pending underlying read gets five minutes; heartbeat bytes renew that
window, and consumer callback time does not consume it. The existing
30-second response-header timeout remains unchanged. Expiry cancels only
the owned HTTP child, joins its timer callback and closes/disposes the body
once. It returns uncertain `504/deadline_exceeded`, preserving partial
output and measured usage, without complete refusal facts or replay.
The host caller/accounting context remains live on quiet expiry.

The external `sol-devin-quiet-read-20261117-01` packet retains the genuine
old decoded/silent assertion failure, all20 deterministic raw/decoded
leaves, focused race20 and vet receipts. Its public SDK input is root's
source7af6 on HEAD31dc through a tempmodfile; no host module definition was
changed. This proves ordinary client transport behavior, not live vendor
or newly built/installed native acceptance. Package0.8.0 and API1.12 floors
are unchanged; the earlier extraction and acceptance pins below retain
their original dates and meanings.

The first quiet-read candidate, source41dc9cf8, received REQUEST CHANGES
for `DEVIN-QUIET-CANCEL-1`; its review9c27b78f and all old receipts remain
preserved. The corrected shared publisher classifies an ended original
parent through `Err`, preserves its custom `Cause` through `errors.Join`
where needed, and keeps owned quiet expiry504 while the parent is live.
Actual held-Close timer-first cancellation and custom parent deadline
controls fail on the saved old runtime and pass unchanged on both raw and
decoded successor paths. The original20 quiet leaves remain unchanged.

Successor `sol-devin-quiet-cancel-20261117-02` pins tested SOURCE
`e577c0d5604de3e7f4932b15e2fac23029067d8474d7382e38cb3774ebc22f12`
and DELIVERY
`6d74705a67db8c20966de918406bec9b424ff4c5a76dcbbd8437e43969ef1a37`.
Its focused24 leaves and selected57-leaf race20 are green, with vet and
format exit0. Aristotle's bounded source/evidence closure is
`astra-devin-quiet-review-20261117-01/TERMINAL-CLOSURE-02.txt`, SHA256
`74a83e7e661e0b68d5875594ae5498a86a8c4536c2937a3ae202bda57025e2d4`.
It approves only that corrected successor and finds no remaining P1/P2
in the inspected scope. These current metadata pointers were added after
positive review; runtime/tests and the sealed tested source are unchanged.
No new native build/install, live vendor acceptance or broad suite is implied.

The November16 bounded donor update compares exact community
`b35d4b35b2446bd929896ddeb58f3cb299e9e2d1`, Devin0.1.44, including
`6c6677d0` (snapshot marker), `2f8f061a` (adjacent signed thoughts) and
`1260b2ba` (long-session normalization/refusals). The SDK package remains
0.8.0/API1.12. Inline image types follow recognizable PNG/JPEG/GIF/WebP bytes
through the common encoder; unknown bytes retain the validated declared type.
Unmarked whitespace-only user prompts are omitted without trimming meaningful
text or inventing a continuation. Whitespace-only cache boundaries fail
explicitly. Signed thinking goes on the next actual assistant text/tool
prompt; adjacent runs retain the last intact thought, while every supplied
signature still requires the existing scoped origin check.

The ASTRA-DEVIN-01 successor applies that same whitespace/cache rule again to
user content left after deferred tool-image association. Empty unmarked
residue is omitted; meaningful text and remaining user images retain their
ownership, and an unrepresentable residual cache boundary is refused.
The normalization order and implementation/reference pins are unchanged.
See [the focused correction evidence](history/ASTRA-DEVIN-01.md).

The SDK model handler already returns an error on failed discovery; it never
serves an authoritative snapshot/empty success and needs no Bun fellBack symbol.
The donor's automatic unsigned second dispatch and byte-estimate-to-too-long
conversion are excluded. Ambiguous invalid_argument retains original response
facts and the existing uncertainty/replay fences. Request length alone cannot
prove context exhaustion or safe replay. These boundaries have focused
catalog, long-refusal and codec regressions, not a full parity claim.

Recovered baseline `ed84daf0` was compared with requested committed
`fe0a9481cdce045597b04f57d5489aab4d7b21a9`. At capture the reference had
advanced to `1729055bb5c566903ce2adbcf97a7e1b78bd3dc1`: the two reported dirty
windsurf files were committed there, with no tracked dirty files left.
Committed, successor and working comparisons remain separate in
`/tmp/magpie-devin-donor-update-20261116/INPUT-IDENTITY.json` and its diffs.
The existing wire.rs and models-228.json hashes are unchanged. Reference
environment/auth/cache mechanisms and reconstructed agent tracing were not
imported. No reference checkout or host source was modified.

The bounded request-policy adoption uses public API1.12
and the supplied frozen host candidate. It preserves the accepted v2 baseline,
records its own current source/build/ordinary-host receipts, and introduces no
dependency or credential refresh. Earlier v2 acceptance is not acceptance of
this integration. Its detailed guide is `docs/REQUEST-POLICY.md` in the source
checkout; that guide is not bundled in the native distribution.
All three generated variants carry exact capability identities and matching
SDK/host floors. The request-policy builder floor is API1.12. The separate
owner-visible native acceptance is scoped in `requestPolicy.nativeOwnerAcceptance`;
Devin author integration receipts and parent final combined replay retain
their own evidence identities.

The current monorepo provider pass is recorded in
[PROVIDER-PARITY.md](PROVIDER-PARITY.md) and `limits.providerCompletion`.
It compares the same community0.1.41 source afresh, fixes authentication
classification/usage ordering and organization-label fallbacks, and verifies
the single native package against frozen source4. The subsequent requested
[CLIProxy comparison](CLIPROXY-PROVIDER-CHECK.md) adds sparse field28 token
accounting from exact committed842cedec8 source; its dirty consolidation is
excluded. It adds no dependency,
manifest/version change, credential refresh or replay. The source4 input is
a content snapshot with null tree/commit, distinct from every earlier host
below. Historical documents and their receipts remain unchanged.

## Earlier source and verification history

The 2026-11-15 domain completion uses committed public SDK
`7bc5ae211510d0b3fe436da8fa819b7e812729e5`, current community main57d1cc6a
(Devin0.1.41, latest package change191d94bf) and recovered authorityed84daf0.
The recovered checkout has no remote; relevant wire/model sources are
unchanged from97a4883. No dirty recovered files were read as authority.
`DOMAIN-COMPLETION-PLAN.md` and `DOMAIN-COMPLETION-EVIDENCE.md` separate
the module regressions/builds and ordinary installed replay on supplied
candidate physical tree36f8e629 from final successor acceptance. The
candidate SDK is byte-identical to the7bc5ae21 export. Prior installed
results below remain attributed to their original source and library.

| Source | Exact reference | Responsibility |
| --- | --- | --- |
| Domain candidate host | physical tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, binary SHA256 `0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509` | Read-only parent packet with10846 paths; source archive SHA256 `5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc`; candidate ordinary installed proof, sourceCommit null, not parentadcf4ce or a future merge commit |
| Committed final API9 acceptance source | `d5b098d8183e88ab9fee1da0170d1c90b9590822`, tree `11c2b9dd3d514bcc76c338f9ae680552319494dc` | Current public SDK and checked current-account catalog admission; runtimec018 was built from tree8fe78, differing only in two test files |
| Frozen merged compatibility candidate | tree `f7f26b2d5ee5388a089ab07a72f7372ed0068971`, source receipt SHA256 `1c5ad12c8bf6774a73e8b89497250aae7e905fe790891384639113a58bc35009` | One additional complete hot replay on parent binary9ddc4e4a; SDK/admission bytes unchanged; no future commit inferred |
| Historical API9 compatibility host | `46756a48c9c9f204a8369638dc63cbcb83d02031`, tree `db40ddbf8944b54f9f689d444644af4c3bc76881` | Preserved typed compatibility/no-op/Gemini results and original hot/cold paid observations |
| Current public API9 SDK for compatibility gates | source SHA256 `36f73768a1335fbf1210fd7033528031f70f2de72b3b182f57af7b0a954969b5` | Public export byte-identical in committedd5b098d8 and46756a48; strict typed `decisionModels` Go/ABI regressions |
| Historical captured API9 task host | source SHA256 `ca3c6c8ad9be659d5d507f91c39c198b69a5a5180fc8db86647943b7a4c2fda6`, base `ae957e5a8a77553a6cf6f484c8b58a69393f3887` | Complete immutable capture of the earlier parent/Volta dirty runtime; sourceCommit is null |
| Historical public API9 SDK for Gemini gates | source SHA256 `999f9b2f2a2575d89c1a1a6dae7d7f284f950f9f89fa9e05450fdafb182d0816`, base ae957 | Earlier combined module/race/donor gates; sourceCommit is null, not a complete runtime |
| Earlier public API9 SDK for module gates | source SHA256 `dc4d6772b33459b59e4058bbdc8b3586547efe7c1ff896f173e1f58b9bc64e9b`, base ae957 | Historical priorModels and first Gemini/cache module gates |
| Public SDK inside the complete task host | source SHA256 `1480770ac41c06f3f06051fd58e0b45d51bfc7f698d3a68d001249e8632794bd`, base ae957 | Distinct earlier export included in complete hostca3; installed proofs use that runtime |
| Historical cache-policy public SDK input | `52a8d85d7956fd027d76b0baea96ea5cf002c34a` | Public SDK1.8 export for earlier independent template/options/policy tests |
| Historical SDK/domain host | `7fbda87dc281e807e121a185760406363d100494` | Committed SDK 1.6 export for earlier module domain proof; codec 0.5 |
| Committed upstream/no-op host | `9e88f10982a1d66c1b402fba27caddda8ac4ac4e` | Upstream/startup plus approved controller fix; original-library identity and real-transition replay |
| Captured API7 overlay | source SHA256 `4c17a1a74eec69ab020c39661c0d02244452a8fdae049e9d4a24453b69337c63`, base 9e88 | Prior complete installed checkpoint, separate from API8 package hashes; not a commit or mutable startup tree |
| Historical captured API8 union | source SHA256 `277f939d2bf971728abef073db3593243f7370768ab2d2c549ec3977a9664fb0`, base 9e88 | Own two-pass capture after parent corrected outsideRead scope; earlier0.7 authoring base |
| Scoped canonical correction | source SHA256 `c43421772d425ffc60705d66c186b66c40173efee1583e7f864cce65f4e198aa`, base 9e88 | Exact projection from tests-only REDe7f76; actual unchanged-library RED-to-GREEN, non-SDK and no-op/transition controls; not a host commit |
| Historical union host | `d07078c7926daf6f33af09c40297c941bd39da92` | Preserved paired/catalog/cold CLI proof and original no-op/bootstrap reds |
| Earlier shared leader checkpoint | `87359a70169661ff9b5b4f71e0831e2c606b293f` | Historical upstream/startup after merge 75fa28b1, before no-op fix |
| Current module authoring SDK | `7bc5ae211510d0b3fe436da8fa819b7e812729e5` | Read-only public SDK export; model efforts and API9 quota DTOs, no host changes |
| Recovered Devin app and wire | `ed84daf06cb9eb961a0d6cd7703e39b7b7f90654` | Current generated contracts/model catalog; historical reconstructed binary remains pinned to97a4883 |
| Current community Devin | `57d1cc6a9f49389d495ea9be8d986c70de7767bf`, package0.1.41 | Current fetched main; latest Devin change191d94bf; source and regression comparison only |
| Historical community Devin | `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, package0.1.40 | Preserved earlier extraction/proof input |
| Community Gemini delta | `191d94bf6ed95ccc4bc62d77e1dd9201a17f5918`, package0.1.41 | Genuine Hydrogen signature/tool oracle and continuation behavior; read with git show/diff, without checkout |
| Upstream router exclusion | `c8290df25991865d0b9eed60937aeb73ab7fa255` | Confirms Adaptive/Fusion API router filtering; read-only comparison, no built-in restored |
| Maintained CLIProxy donor | `842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9`, v8.0.42-698-g842cedec8 | Secondary executor/stream/journal comparison from committed CLIProxyAPI-u2-331t-up42 source |
| Original codec extraction | `61dc6c58de9740345d0b9462041444b40ffa9dbb` | Historical extraction recorded in donor-files.json; current files intentionally differ |

The integration target is the leader's single `devzeus` runtime.
Source/nativeops/persistent model discovery are already consolidated;
the separate nativeops worktree is retired. Read/export committed inputs
from `../magpie` or explicitly capture the dirty overlay with a complete
file/mode/hash inventory. Current domain authoring uses committed7bc5ae21
and the identical public export from the candidate packet; d5b098d8 and
API8/API7 pin prior installed checkpoints;
earlier domain proof uses exact 7fb. A base commit never claims the dirty
overlay's code. The historical d070 tree is
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`; its documentation/tests-only
freeze tree f536d820 and earlier b5e4494/2d95d9cf are historical.

The standard0.8 package registers the default-off `cache-keepalive` task
through API9. `CACHE-KEEPALIVE.md` describes explicit `keepAlive` consent,
paid confirmation and bounded timing. `CACHE-KEEPALIVE-PLAN.md` preserves
the extraction decisions and final Task/Key/Job/DueAt adapter contract.
`CACHE-KEEPALIVE-EVIDENCE.md` separates historical primitives, actual
SDK-peer RED-to-GREEN and installed-host observations. Measured cumulative
usage snapshots preserve canceled paid work; no finished Stream, Auth,
client, credential or module-owned timer is retained. The old draft's
DelayMs, account-wide cancellation and missing-snapshot blockers have
been replaced by absolute DueAt, conversation Key and task-only snapshots.

The0.1.41 Gemini delta is incorporated in this same full package.
`GEMINI-SIGNATURE-PLAN.md` records extraction boundaries and
`GEMINI-SIGNATURE-EVIDENCE.md` pins the genuine1451-byte donor fixture,
actual four-case RED-to-GREEN and exact public-SDK input. Signatures/type
and generation model survive Chat/Messages tools without visible thinking;
restoration still requires account/generation/caller/model/conversation
matching and existing origin receipts. The donor's unscoped global cache,
missing-type fallback and empty-reply speculation are not adopted.
The earlier production inputdac520a0 and library464489d5 retain their
module/race/vet/build, nine provenance cases, unmodified donor7/7 and six
selected installed results. Their cosmetic no-op, hot-config and Gemini
Messages REDs remain historical observations of the exact earlier host,
including the separate052 paid-not-started failure. They are not erased
by new acceptance on the committed runtime. `DELIVERY-EVIDENCE.md` records
the current compatibility RED-to-GREEN and physical-host replays separately.

The maintained paid fixture now uses hot external config CAS by default,
with an explicit `--cold-start` control. Its nine regressions require
exact sampling/tool choice preservation and demonstrate the old parser's
failure on valid fixed64/native and fixed32/structural chunks.
Recovered `reference/crates/chisel-api/src/wire.rs` documents native
temperature/top_p as fixed64; fixed32 coverage does not invent a native
field or captured wire type. Three complete hot repetitions pass on
binaryc018 against accepted source11c2; no production or timing changed.
The original c018 held38 timeout remains unexplained, and every old RED
is retained. `limits.finalInstalledSampler` pins those distinct inputs.

Current API9 planning accepts `decisionModels` through the public
`AgentDecisionModel` DTO and `ValidateAgentDecisionModels`. It is separate
from conversation `models`/`priorModels`; no decision model is written to
client configuration or binding values. Missing optional input remains
compatible, explicit null/invalid catalogs and unknown root/nested fields
remain refused. This typed field was added within unpublished SDK9;
earlier draft API9 exports cannot build the current consumer. No new host
contract, manifest floor or fallback decoder is introduced.

`UNION-EVIDENCE.md` preserves paired/catalog/cold CLI/reconstruction greens
and original bootstrap reds. `ASSIGNMENT-DIAGNOSIS.md` proves stable module
RequestID/Session and exact write preimages differing only by host revision.
`NOOP-ACCEPTANCE.md` preserves proof of the scoped conservative controller candidate
with the original installed agent bundle: duplicate before/after no-op
403/one dispatch, complete binding/journal stable, held read 200/exact payload,
unchanged real-transition script green. Candidate complete source SHA256
is `7c6cf2e03dad22d88cc04cbefe15183dd589ce0f4dc7566fd2e4e1e597b86022`;
its source has no git metadata. The own frozen copy was hashed before/after
build. No commit is inferred from the parent's mutable remote path.
`limits.scopedNoopCandidate` records exact helper/binary/original-library
hashes. The same original-library gates now pass on committed 9e88 and
captured API7 source `ca0d9878...`, with separate source/binary identities in
`NOOP-ACCEPTANCE.md`. That first API7 capture preceded the parent's
compatibility-validation fix; the later `4c17a1a7...` capture is the module
build input for the earlier API7 checkpoint. Earlier0.7 builds and full
cold/startup/Facts proof use API8 source277f/binarya08f, with complete
inventories and library hashes in `DELIVERY-EVIDENCE.md` and
`limits.sdk18Snapshot`. Both original and final agent libraries pass
duplicate/held-read/real-transition gates there. The later sourcec434/
binary2fe733 correction reuses the same unchanged paired/agent libraries
and repeats installed canonical, paired-policy and no-op/transition gates.
`limits.scopedCanonicalCandidate` pins its distinct RED/GREEN inventories,
binaries and receipts. No module production rebuild/version bump is implied.
Those prior snapshots do not establish the current committed-host replay;
its exact receipts are recorded separately. Broad host acceptance remains
leader-owned.

The maintained CLIProxy evidence uses only its pinned committed source.
Uncommitted files are not attributed to that commit and are not copied.
Older CLIProxyAPI-up-u2 7638884b is historical evidence, not the active donor.

The unmodified reconstructed application built on usbox reports
`devin 3000.11.228 (053434014dbd)`. Its actual executable SHA256 is
`8b5438632942381272d20974f7ee59fd9af7674d452dce7ea448077b4c075327`.
It is a Rust reconstruction, not the original Cognition binary. Generated
descriptors came from recovered Desktop 3.10.48. Binary, generated source and
commit identity are separate facts; no private captures or credentials are
shipped. `BOOTSTRAP-EVIDENCE.md` and `STREAM-EVIDENCE.md` retain historical
9c startup and strict completion/receipt proofs. `UNION-EVIDENCE.md` records
the consolidated host. The lock separates historical/runtime/candidate
host identities and pins exact committed descriptor hashes.

## Variants and prerequisites

| Distribution | Version / minimum SDK | Implemented surface |
| --- | --- | --- |
| Default / `--agent`, source `magpie-agent.json` | 0.8.1 / 1.12 | Paired ingress/outbound/auth/models/usage/Facts/diagnostics, agent/bootstrap, managed bundle lookup and default-off paid account task |
| `--paired-only`, source `magpie-plugin.json` | 0.5.0 / 1.12 | Paired ingress, Chat/Messages, auth/models/usage/Facts/diagnostics and request policy |
| `--inbound-only` | 0.2.0 / 1.1 | Canonical Chat/model converter; no provider or account sign-in |

All variants use native ABI 1 and are alternative installs of the same
module ID, not simultaneous owners. The default builder registers `NewAgent`
from `cmd/native-agent`; `New`/`NewModule` and the repository
`magpie-plugin.json` retain their paired compatibility contract for explicit
`--paired-only` builds and historical fixtures. The emitted standard artifact
is always named `magpie-plugin.json` and contains the full0.8 declaration.
Source needs the final API9 decision-model DTO/validator and account-task
methods plus the earlier Facts and diagnostics public packages.
The native SDK's existing `golang.org/x/sys v0.49.0`
transitive requirement and its exact host go.sum checksums are recorded
locally for `-mod=readonly`; no package is fetched during verification.
The unified host includes the Source and native account
contracts; unchanged SDK 1.5 source cannot compile the new bootstrap DTOs.
The repository URL identifies our fork, not evidence of a push. Historical
API7/API8/API9 dirty captures remain distinct from committed acceptance pins;
neither source publication nor public availability is implied.

Auth is explicit API key or one-use browser PKCE. Explicit CLI redirects remain
caller-managed. The first-account window uses a private module-owned callback
receiver at `127.0.0.1:0` and the existing host poll broker. Its redirect shape
comes from `reference/crates/windsurf-api-client/src/oauth.rs` at the unchanged
implementation pin1729055; no later donor capture or native CLI login is imported.
The earlier bounded92503f96 comparison remains unchanged. See
[onboarding ownership and evidence](ONBOARDING.md).
A recovered refresh/revoke endpoint is not known. Known
bootstrap methods are implemented, not 501 placeholders. Other native
methods, cloud hosting, remote delegation and donor domain gaps remain
listed in the lock and `DONOR-MATRIX.md`; full original native parity is
not claimed.

Canonical Messages additionally requires the existing host-derived caller
field on ordinary SDK outbound calls. Earlier 7fb/captured API7/API8 REDs
omit it outside matching private metadata. Corrective sourcec434 initializes
it only from trusted host context and preserves SDK Chat history before
legacy tool repair. Four actual installed RED-to-GREEN cases, non-SDK
repair and private-metadata/spoof controls are in `DELIVERY-EVIDENCE.md`.
`HOST-CANONICAL-GAPS.md` retains the earlier diagnosis. Missing identity
still fails before acceptance; no signature-origin fallback is introduced.
API9 now projects present ACU amount/limit, signed balance micros, units,
quota spans and plan-end resets. API7 retains percentage-only metadata.
The SDK has no top-level plan/expiry DTO and the recovered balance field
establishes micros without proving a currency; neither is guessed.
SDK model IDs, quota scopes and account tasks prohibit whitespace. A
source label-only family such as `Claude Haiku 4.5` therefore retains its
concrete UID/display name instead of gaining a normalized or invalid alias.
API7 execution Facts are a separate envelope. `EXECUTION-FACTS.md` maps
native counts/metadata/committed credits and ACU-valued reported cost,
retains explicit zero and absent distinctions, and documents code-only
diagnostics, best-effort cancellation limits and unsupported facts.

## What to diff next time

Native response watchpoints include StopReason and ModelUsageStats in
module-98224.js and GetChatMessageResponse in module-432.js. Response
committed_acu_cost field 22 is a double (fixed64 wire 1). Keep native
failure stops, provider refusal, unknown controls and cancellation distinct
from a successful Connect terminal. Only a validated successful terminal
permits this module's origin receipts. Native-only output remains raw;
the canonical renderer still refuses stop reasons it cannot represent.

1. Diff each lock commit against the exact candidate commit, restricted to
   that source's `watch.paths`. Do not reinterpret a dirty working tree as
   the pinned commit.
2. For wire/app changes, compare generated message fields, native startup
   routes, metadata, assignment and CLI arguments. Update codec fixtures
   and run the actual reconstructed-client startup separately from the
   synthetic recovered-wire client.
3. For community changes, compare the named keepalive/cache/reasoning/
   signature/tool/image/free/usage/price/schema test families. Update the
   capability matrix before claiming a port. Preserve original schema and
   account-origin authority; do not blindly copy synthetic missing results
   or prompt substitutions. `DONOR-HOST-EVIDENCE.md` preserves independent
   history/image/usage successes and historical host normalization/caller
   REDs; `DELIVERY-EVIDENCE.md` records the same installed fixtures GREEN
   after generic correction, with non-SDK behavior retained. Recheck that
   boundary on the final frozen host. Messages growing-prefix,
   typed events, tool JSON and signature-origin tests are required before
   changing that conversion. Preserve ACU/cache units and explicit zeros;
   never project native committed ACUs as USD.
4. For SDK/CLIProxy changes, recheck lease lifetime, acceptance, cancellation,
   no-replay, account generation, persistent catalog authority and empty
   replies. Exercise all native catalogs with model restrictions, aliases,
   scoped budgets and read-only keys. Keep failed bootstrap/assignment
   observations and actual mock dispatch counts; successful retries are
   not acceptance. Watch no-op catalog sync, binding phase/revision and
   stable accepted-write identity together. Re-run
   `assignment_transition_e2e.py` for real model changes: stale payloads
   must fail before vendor dispatch, in-flight replies must stay fenced and
   an accepted write must keep its original journal without replay.
   Keep binding revision in write identity and retain the exact Check.
   Keep the raw failed diagnostic
   when a fixture assertion is corrected; replay saved observations before
   issuing another request. Native account-operation IDs use
   the SDK's lowercase ASCII module ID grammar; the four declared Devin IDs
   already conform. Keep vendor logic in this bundle and generic
   lifecycle/policy in the SDK.
5. Keep every manifest's actual version/floor and the lock's variant record
   consistent; explicit compatibility artifacts retain their own contracts.
   Run provenance/package regressions and check required tracked
   files. Rebuild on usbox and record new exact source/artifact hashes;
   historical checkpoint hashes are not proof of the updated code.

Every distribution includes this guide, execution-fact/donor/evidence
references and the source lock. Its packaged
`plugin` identity/version/SDK floor is projected to the selected manifest;
the repository lock's top-level plugin describes the complete default0.8
distribution. Donor pins and the earlier paired/inbound hashes remain
historical evidence, not hashes of a newly selected default artifact.
