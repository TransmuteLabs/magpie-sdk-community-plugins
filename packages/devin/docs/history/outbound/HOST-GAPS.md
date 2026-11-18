# Minimum host requirements for the paired Devin module

The current full default is0.8.0/API1.9, including agent/native operations,
Facts/diagnostics and default-off paid account tasks. Final own module and
selected installed proofs are in `../CACHE-KEEPALIVE-EVIDENCE.md` and
`../GEMINI-SIGNATURE-EVIDENCE.md`. They retain three actual frozen-host REDs:
cosmetic no-op duplicate AssignModel (held-no-op not reached), hot-config
stale catalog and legacy thinking-disabled injection into Gemini Messages.
The parent's final committed upstream+SDK9 pin/replay remains separate.

## Paired compatibility contracts and SDK 1.7 floor

The historical standard0.7.0 and explicit paired-only0.5.0 require SDK1.7 for sparse
execution facts and diagnostics. The paired routing contracts were
introduced in SDK 1.4 and remain consumed together. `pairedAdapter` returns `ExecutorContract()`;
its decoded native request requires `SameModuleExecutor`. The host derives
the owner from the pinned inbound lease and rejects foreign candidates before
execution. No caller-supplied module id is used.

The usage handler publishes `NotModels` and echoes the host's canonical usage
revision. Auth continuation state uses the generic declared-AccountAuth
start/finish CAS exception. The actual compiled-host fixture covers those
contracts, including browser PKCE, without a host-side vendor adapter.

The module resolves `ModuleContext.State` in `Decode`'s RPC context. `Bind`
retains only lifetime, so no assumption that it receives configured state is
needed. No missing generic contract remains for this paired
Chat/account-provider slice. The live separate-process account replacement
defect in older host `0dc09a544` is resolved by host
`a721fd647c016f857c965f3b61c72a1b5cdb6084` or an equivalent implementation
fix. It does not require an SDK API bump or a module-side workaround.
Use that host fix when the CLI may reauthenticate an account while its
gateway keeps running. See `../PAIRED-EVIDENCE.md` for the exact source
equivalence, binary hashes and strict-live proof.

## Consumed contracts

| Contract | Module use and verification |
| --- | --- |
| Same-module executor, SDK 1.4 | Required on paired native requests; real Front denies a foreign HTTP executor before its network request |
| Pinned decode/execute/response generation | Module-private request handles exist only in the configured instance; raw replies use that instance's bounded queue |
| Selected account generation | `Call.Auth` is checked against its explicit provider/account scope; receipts remain generation-bound |
| Live account discovery and revoked selection, host `a721fd647` | Three ordinary native-host runs refuse old signed history promptly, execute new unsigned work without restart and preserve 11 old journal entries each; a separate diagnostic run proves actual G3/new Auth frames |
| Scoped quota selectors and epochs | Native Free-tier UIDs populate `NotModels`; the report echoes the revision and satisfies `ValidateUsageResult` freshness |
| Declared auth lifecycle state CAS | Actual SDK CLI key and browser start/finish persist and consume continuation state; no host bypass |
| Configured module state | Read in Decode RPC context; Bind needs only its lifetime, not ModuleContext |
| Host network proxy | Each vendor request resolves its proxy through the scoped public host service |
| Transactional refresh and ordinary expiry, SDK 1.3 | Available; no recovered Devin renewal RPC exists, so this module returns an explicit sign-in requirement |
| Sparse execution facts and scoped diagnostics, SDK 1.7 | Current provider variants use the separately captured parent overlay; measured counts/units/metadata remain distinct from legacy totals, and code-only diagnostics cannot change execution. See `../EXECUTION-FACTS.md`. |

These are implemented, consumed contracts, not requests for future SDK
features. Browser PKCE and origin receipts are module-owned logic and are
registered in the paired distribution. The optional 0.2.0 distribution retains the old
inbound-only behavior.

The earlier pure-client checkpoint and source authority remain in
`EVIDENCE.md`; `../PAIRED-EVIDENCE.md` is the earlier paired checkpoint.
Final installed-package proof is in `../DELIVERY-EVIDENCE.md`.
The standard agent profile and finite bootstrap add their own contracts;
they do not require a built-in experimental loader or another plugin.

## Verification boundaries

The historical live separate-process account replacement defect was
reproduced on both source inventory `9b42e406` and committed `0dc09a544`.
Its resolution is verified independently of restart mode: three live
production-library runs, old-history refusal in 0.260-0.331 seconds,
new unsigned execution and unchanged old journals. The tested eight-file
patch inventory
`cecbab80c4f442a69b9d789790529d13111f0d4f30b7ad645a54aa5d35e0c8bb`
matches all eight files in host commit `a721fd647`. Authoritative generation
checks still reject manual successors; they were not relaxed into refresh
receipts. The host's full provider-race suite remains separately non-green,
so this is bounded paired-module evidence, not whole-application acceptance.

No missing executor/quota/auth-CAS API blocks the bounded raw paired slice.
Ordinary canonical Messages caller identity and strict Chat history fidelity
now have scoped installed RED-to-GREEN proof in `../DELIVERY-EVIDENCE.md`;
`../HOST-CANONICAL-GAPS.md` retains their historical generic seams.
Full native CLI/vendor parity, other native platforms, automatic browser
callback listeners and additional native management semantics
are not claimed. Unsupported semantics remain explicit refusals; no refresh,
revoke or assignment endpoint is invented.

## Native agent in the standard distribution

The committed 0.4.0 agent slice used SDK 1.5 and explicit bootstrap refusals.
That historical checkpoint remains in `../AGENT-EVIDENCE.md`.

The historical0.7.0 agent slice consumes finite native account operations
introduced in SDK 1.6, plus API7 facts/diagnostics. Historical d070 and 7fb
proof is retained. Its builds use the own captured API8 union on
approved upstream/no-op base 9e88; source and binary hashes remain distinct.
It is now included in the standard no-flag build, together with all ready
paired/auth/models/usage features. `--paired-only` and `--inbound-only`
explicitly select compatibility artifacts; no second plugin is installed.
`../DELIVERY-EVIDENCE.md` records the one full default artifact passing
paired, agent/bootstrap, canonical and no-op/transition installed gates.
GetUserStatus, GetCliTeamSettings, GetAccountManagedPlugins and AssignModel
now use selected-account native dispatch, not unconditional 501 placeholders.
The original inbound lease owns decode/action/response; current binding,
package/config and account generation are fenced. Assignments use declared
write acceptance/no-replay and cascade-bound token receipts. Core has no
vendor endpoint, credential or entitlement implementation.

The installed paired fixture passes 24 groups, including all six native
catalogs with restricted model/alias policy, scoped budgets, read-only
denial and unary cancellation. Cold CLI selection with the gateway stopped
passes without vendor calls; Source6 union and durable catalog persistence
are no longer blockers. The reconstruction's startup/first-turn proof also
passes on usbox with only the own native bundle installed.

Historical d070 agent/bootstrap acceptance remains partial: a duplicate AssignModel
returned HTTP 200 with two actual mock dispatches; an earlier bounded trial
failed a bootstrap read. Binding revisions also change under catalog sync
without a model change. The conservative scoped no-op candidate passed
the original installed-library identity/held-read and real-transition
gates (`../NOOP-ACCEPTANCE.md`) on those historical runtimes.
The original-library gates also pass on committed9e88 and captured API7/API8.
Historical API8 proof repeats them with0.7.0 and confirms ordinary startup,
cold CLI, catalog/cancellation and Facts/diagnostics. Parent's host commit
and broad acceptance remain separate. No module retry or relaxed
generation check is introduced.

See `../DELIVERY-EVIDENCE.md` and `../source-lock.json` for final results and
artifacts; `../BOOTSTRAP-EVIDENCE.md` records the earlier 9c checkpoint.
Remote/restricted/group delegation and native Basic ingress recognition
remain generic boundaries. Paired routing dates from SDK1.4; the current
paired compatibility distribution requires API1.7 for Facts/diagnostics;
the full default0.8 requires API1.9 for account tasks. SDK-only host
rollout is independent of optional Devin acceptance; neither checkpoint
proves original Cognition/live-server parity.

## Canonical Messages caller and history boundaries

Messages conversion/rendering and receipt-resolved signature continuation
use the existing public SDK. Historical committed7fb/captured API7/API8
REDs omit `ExecuteOptions.CallerID` outside matching private metadata.
Scoped corrected sourcec434 projects only the trusted host caller and
passes the unchanged ordinary signature/stream/cancel fixture. Private
native metadata/request IDs still require the same module owner; missing
identity still refuses before acceptance with no header/session fallback.

The same correction preserves SDK source Chat histories before legacy
`toolPairsOut`: mixed aliases pass, missing real results and unrepresentable
cache boundaries fail before dispatch. The ordinary non-SDK repair control
still passes. Actual RED/GREEN snapshots and unchanged-module receipts are
in `../DELIVERY-EVIDENCE.md`; earlier reds remain in
`../HOST-CANONICAL-GAPS.md`. Those scoped dirty inputs are historical;
the parent's final freeze/upstream replay remains separate.

Usage projection adds representable nonblocking ACU percentages. The consumed
quota DTO still lacks numeric display/unit/balance/until fields for full donor status
display. Native ACU cost is not USD; no currency conversion is introduced.

## Background cache keepalive

The full0.8 module now consumes the public API9 account-task contract.
Default-off `keepAlive` requires explicit paid confirmation; each admitted
Task/Key/Job/DueAt callback receives fresh selected-account authority and
reports cumulative measured usage. The actual installed proof includes
real30-second cadence, budget/model/readOnly refusal, next-turn held
cancellation/join and once-only ACU accounting. No finished Stream, Auth,
client, credential or module-owned timer is retained.

Domain ping bytes, bounded templates, conversation identity, interval/
maximum/miss policy and paid metadata stay in this one module.
`../CACHE-KEEPALIVE.md`, `../CACHE-KEEPALIVE-EVIDENCE.md` and
`../DONOR-MATRIX.md` document the consumed contract and remaining boundaries.
The earlier missing-scheduler gap is closed for the exact captured API9
runtime; hot activation still exposes the separate stale routing-catalog
RED described above. Live cache TTL/charges are unverified.
