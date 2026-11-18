# Generic contracts and bounded module adoption

The historical ranked handoff is
[NATIVE-CONTRACT-REQUEST.md](NATIVE-CONTRACT-REQUEST.md), with exact official
launch/wire examples, a reusable synthetic stdio peer and owner acceptance
cases. Public API1.10 now provides checked source resume and owned process
callbacks. The full 0.15 module adopts them as documented in
[NATIVE-FOREGROUND.md](NATIVE-FOREGROUND.md). The separate rooted CLI SQLite
history and foreground account tasks retain their existing host owners.
Final combined-host acceptance remains parent-owned.

2026-11-15. Module 0.14 consumes the shared public API9 contracts. The
current source-proven native profile and reset-control completion is recorded
in MODULE-COMPLETION-EVIDENCE.md. Final merged-host acceptance is separate.
Its previous
bounded concurrency/quota adoption and remaining parent gate are recorded
in API9-EVIDENCE.md. Historical module 0.12 consumed public API7 against the owner's immutable
API8 source snapshot `43b7e290d9c3370881780b23ad49222f5d27f44b`,
based on committed shared host `9e88f10982a1d66c1b402fba27caddda8ac4ac4e`.
The snapshot remains uncommitted; final committed-host acceptance is a
separate gate. SDK-only deployment remains independent.

## Exact execution usage projection

The host owner supplied optional API 7 `Usage.Facts` in
`pkg/extension/usage_facts.go`: pointer counts preserve exact zero versus
absence, including cache-write 1h and reasoning; deferred/cache-unknown,
served/upstream identity, tier, credits and currency-bearing cost are
structured facts. Final/event snapshots are deep-cloned. This addresses
the projection contract and is adopted by the full module.

The ordinary package retains the SDK 1.7 telemetry layer and reports only source-backed
Facts. The historical provider-only API1.4 compatibility manifest retains
its four legacy counters. One-hour cache-write and reasoning remain
subsets, not additional totals. Missing execution cost/currency/credits,
deferred counts and upstream-provider facts are not derived from account
quota or raw lookalike fields. OBSERVABILITY7-EVIDENCE.md records the
source, negative and installed-host proofs.

## Structured operation diagnostics

The host owner has supplied API 7 `diagnostics.emit` in
`pkg/extension/diagnostics.go`. Only level/code/phase/status/millis/count/
retryable are accepted; the host derives module/account/operation identity
and limits emission to 60 events per module per minute. Full-module
execution, auth, quota, models and named actions use that callback for
finite terminal codes. Callback failure or budget refusal does not alter
the original result. A canceled parent emits no late diagnostic.

This package does not substitute stderr, free text, request/response
bodies, vendor headers or credentials for that gated callback. Manual
source-preserving native-log inspection is a separate explicit operation;
it neither grants automatic collection nor forwards raw logs to telemetry.

## Rich quota card information

The shared API9 UsageWindow adds optional amount/balance/display/unit/span.
The bounded module port adopts actual donor count/display/period data,
and the assigned host adapter preserves it through validation, native
JSON and the ordinary quota view. Display, amount and wallet metadata
create no missing Used share, freshness, cost or financial balance
authority. Pending grants stay informational. Historical API4 output
retains its previous percentage representation.

Provider descriptors also carry optional API9 concurrency/fanout defaults.
The full package declares 3/2 through ordinary host owners with user limits
winning, including explicit zero. Stale/config/account/package fences,
cancellation and response-body release are exercised through Front.
The adjacent cold config/account-stamp finding is handed to the parent
with an unchanged independent probe in testdata/api9; the lane does not
edit the shared stamp owner.

## Consistent SQLite and diagnostic sources

The old snapshot proposal in SQLITE-INTEGRATION-PLAN.md is historical.
The separate 0.15 module slice adopts existing rooted StorageSource for
default CLI history list/transcript. AUTOMATIC-HISTORY.md describes the
read-only DB/WAL/SHM transaction, source/lease and publication boundaries;
AUTOMATIC-HISTORY-EVIDENCE.md records its scoped acceptance. No new generic
snapshot API, forced checkpoint, source write or independent DB/WAL copy is
used. Manual binary conversion remains a distinct supported path. Desktop
task-index association, custom native roots and financial import remain
separate module work rather than accepted automatic-history behavior.

CLI log JSONL is a diagnostic stream, not a session record source.
If automatic diagnostics are requested, they need a separate explicitly authorized bounded
source kind with rotation/tail semantics, redaction/collection policy,
capture provenance, epoch/revocation and disposal. The native writer's
seven-day retention is source behavior, not permission to delete logs
or silently collect installed app data.

## Scheduling and actual native wire

SDK 1.6 NativeAccountOperations exists. The module's current launch is
ordinary configured Messages, without a native model-bound operation
wire. Adapting recovered stdio/control is module work; a synthetic
endpoint is not evidence that the actual client calls it.

Official fec9759 `arguments.ts` and `run.ts` accept `--resume <sessionId>`.
The rooted source now declares the existing API1.10 owned-agent recipe.
The full module also declares `app-server --stdio` through `AgentProcess`;
the host owns executable/config/lifetime and callback bounds. The adapter
owns bounded V4 framing, reverse correlation, command query and a separate
durable native intent. The ordinary process cwd is profile HOME, while V4
workspace authority is explicit module configuration. Ordinary CLI resume
and foreground process launch do not authorize attaching a desktop child.

API9 supplies opt-in foreground-bound successor account tasks, with fresh
selected-account admission, finite declared writes, durable intent and
off/config/reauth revocation. This is an available adoption path for
foreground-triggered work; it does not establish a perpetual daily timer
from sign-in or module startup. No official requirement for a new paid
keepalive was established. Module Start has no per-account Auth and
read-only usage must not become a financial mutation scheduler. Explicit
page/owner daily check-in already works. Any requested calendar-only
lifecycle needs its own finite evidence and contract.

Archived financial import remains a separate reviewed module migration:
explicit source/principal mapping, stopped destination, backup and
durable pending/unknown/reset debt. Old string generations cannot become
current SDK numeric generations. Actual-client resume and live financial
acceptance remain distinct gates.
