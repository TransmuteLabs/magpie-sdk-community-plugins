# Decision-operation contract and remaining metadata boundary

The historical proposal below describes the SDK 1.6 boundary that led to
the host owner's generic API 1.7 implementation. The current module
uses that selected-account operation for real SystemOne requests and publishes
an explicit Jev decision fact after successful discovery. It never exposes
Jev as chat. Its legacy `systemone` action still refuses to bypass the
operation owner. Host scoped evidence and final merged-parent/module
acceptance are distinct; see [VERIFICATION.md](history/VERIFICATION.md).

Own-model metadata now has a separate negotiated API 1.9 implementation and
proof record in [OWN-MODEL-METADATA-API9.md](OWN-MODEL-METADATA-API9.md).
Final acceptance on the exact consolidated parent is recorded separately.
The independent first plugin-free deployment does not depend on module
acceptance. The remaining sections retain the original
proposal and its original SDK 1.6 observations, rather than relabeling those
historical fixtures as current host acceptance.

## Historical SDK 1.6 boundary

At the unified host `../magpie`, commit
`d07078c7926daf6f33af09c40297c941bd39da92`, tree
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`, SDK 1.6,
`pkg/extension/provider_operations.go` has a finite operation registry,
requires the conversational `chat` envelope for every operation, and rejects
model kind `decision`. Adding a module handler alone cannot make Jev usable
through ordinary routing. `Model` also has no operation association with
which a decision-only model can advertise a non-chat endpoint.

`NativeAccountOperations` is a separate native inbound contract. Its finite
declarations bind an action to a provider and a model-bound agent launch;
it requires inbound codec, agent wiring and same-module execution authority.
It does not add a provider decision operation, decision model kind or
ordinary `/v1/systemone` dispatch. Session Source supplies history readers,
not decision execution. The persistent catalog preserves validated model
facts but cannot accept an unsupported model kind.

## Recovered donor wire

The pinned community source sends JSON `{model, state, questions}` to
`POST /provider/v1/systemone` and preserves `{model, answers, usage}`.
The canonical model is `typesafe/jev`, with aliases `jev`, `jev-latest` and
`typesafe-ai/jev`; its recovered CLI header is 1.88.2. This is separate from
the native generate transport's 1.72.2 header. The exact source is
`magpie-community/plugins` commit
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, package 0.1.24:
`packages/commandcode/index.mjs`, `systemOne`, and `jev.test.mjs`.
Both file digests are in `source-lock.json`.

`tests/systemone_donor.mjs` loads that hash-checked donor unchanged and
replaces its fetch with an in-process recorder. The request and typed
`noul` answer come literally from the donor's Jev test. It checks all four
aliases, exact CLI headers, ZDR, answer bytes, malformed input before
dispatch, and the donor's account-preserving Go refusal. The donor maps
`MODEL_NOT_IN_PLAN` from upstream 400 to 403; the fixture records that
mapping explicitly. Authentication is Bearer only on this endpoint, with
CLI version 1.88.2, `user-agent: cli`, production environment, project slug,
session UUID and `x-taste-learning: false`. No vendor execution, official
CLI binary or live entitlement is part of this evidence.

## Exact minimum proposed wire change

Use the existing selected-account provider execution and result machinery
with these additions under proposed SDK API **1.7**. This number is a
proposal for the host owner after core acceptance, not a negotiated version
of this module. API 1.5 remains its minimum; the verified host is API 1.6.
No additional runtime, module-specific route or decision codec in core is
required.

| Surface | Required contract |
| --- | --- |
| Operation registry | Add `systemone` to `ProviderOperation`; declare it in `ProviderDescriptor.operations`. Manifest admission requires the module's negotiated minor to be at least 7 and `outbound-executor`. |
| Model metadata | Add `Model.operations`, accept `kind: "decision"` with `operations: ["systemone"]` and `protocols: []`. Require the association to be a subset of that provider's declared operations. |
| Protocol isolation | A decision-only model never inherits provider chat protocols, including when it appears beside chat models. Permit a provider with no conversational protocols only when it declares the decision operation alone; retain existing chat-envelope requirements for the existing operations. |
| Execution | Accept `ExecuteRequest.operation: "systemone"` only with `protocol: ""`, negotiated module minor at least 7, a current provider declaration and an exact selected-account model-operation association. Preserve raw JSON, selected model, sanitized headers and existing `ExecuteOptions`. |
| Ordinary route | Dispatch the existing `POST /v1/systemone` owner through the declared operation and selected account; expose `/v1/systemone` as the model's native endpoint. Reuse the host's existing `/v1/decisions` translation only where it already applies. |
| Admission | Validate declaration, model-operation association, key/model authorization, account generation, current usage, cancellation and journal acceptance before dispatch. |
| Responses | Preserve status, safe end-to-end headers and raw decision JSON via existing headers/data/result frames. Use the normalized usage and accounting rules below. |
| Compatibility | Gate both manifest/model admission and execute-time validation. A newer host must still reject `systemone` claimed by a module negotiating minor 5 or 6. An undeclared operation, chat envelope or mismatched model is a pre-dispatch refusal with `outcome: "not-started"`. |

The host performs no vendor transformation. Once negotiated, this module can
map that operation to the recovered vendor route, canonicalize only the
known model aliases, and apply the selected account, network policy and ZDR.
The host continues to own routing, key restrictions, usage revisions and
replay authority. A decision request naming another model must pass the
same model authorization as the primary request.

## Exact selected-account association

The provider operation declaration says what the adapter implements; it
does not grant every account every decision model. Derive `DecidesModel`
and dispatch authorization from that selected account's validated
`provider.models` result or its valid persistent catalog. Persist and clone
the model's operation list with the existing account/package/configuration
generation keys. Recheck the same association at dispatch; another account's
catalog or a provider-wide union is insufficient.

A successful known-empty model catalog stays empty, including after a cold
read. Unknown, unreadable and stale catalogs retain their distinct states
and grant no decision entitlement. Do not synthesize Jev in core from its
name, a preset, a static manifest fallback or a previous account's list.
The recovered donor inserts Jev even when its vendor model list omits it;
that donor policy is evidence about the donor, not host authorization.
Any future module-supplied Jev entry must be an explicit validated account
fact rather than an invention in the ordinary adapter.

## Fallback and typed outcome ownership

The read-only review found two distinct owners in
`internal/gateway/decide_group.go`: `serveDecideTries` retries accounts and
group members, while `askDecideTries` retries classifier accounts/members.
Both currently lack `provider.WithExtensionAttempt` and the accepted or
uncertain fence. Installing an adapter only in `postDecideSent` would leave
both replay paths open.

Create an extension attempt context for each complete fallback sequence
and pass it through every candidate call. After the transport finishes and
its body is closed, check that attempt before any retry, rest-driven
fallback or classifier continuation. Follow the existing
`serveRetrieval` / `extensionSideOutcome` pattern in `embeddings.go`.
Definite pre-acceptance failures may use the existing routing policy;
accepted, completed-but-failed or uncertain work must stop the sequence.
Ordinary HTTP errors then return the existing conflict/no-retry response
and operation receipt headers. A classifier must return the typed failure
to its caller instead of asking another candidate or silently selecting a
default. Apply this rule through the existing Decisions translation too.

`internal/gateway/decide.go:postDecideSent` currently wraps the `p.Do`
failure with `%v` and discards the response read error. Preserve the error
chain with `%w`, and preserve read/close failures and execution outcomes.
Read with an explicit size check rather than treating a truncated bounded
read as success. Drain/close before consulting final normalized usage and
the attempt fence, since the SDK result can arrive after body chunks.
A typed SDK/host outcome must remain discoverable with `errors.As` all
the way through `askJev` and classifier wrappers. A partial answer or
missing terminal result cannot become a successful decision.

## Headers and exactly one usage record

The request uses existing `ExecuteRequest.headers`, with the transport's
credential stripping and host-selected network options. Caller
Authorization, cookies and proxy credentials do not become upstream auth.
Working-directory and session context follows the existing generic
request context; the module constructs the recovered vendor headers,
Bearer credential, project slug, session UUID and ZDR. Core must not
invent the Command Code CLI headers or apply its chat signing codec.

Return the module's safe end-to-end response headers through both direct
and group decision paths: content type, request identifier, Retry-After
and account-retention signals. Reuse the ordinary reply-header policy;
strip hop-by-hop/credential headers and recompute framing. On an outcome
fence, retain the host's `X-Should-Retry: false`, `X-Magpie-Operation` and
`X-Magpie-Extension` receipt headers. Preserving only Content-Type and
retry headers loses part of this contract.

Use existing `sdk.Usage` fields (`inputTokens`, `outputTokens`,
`cachedTokens`, `cacheWriteTokens`, `cost`, `currency`) without changing the answer
JSON. A usage event and `ExecuteResult.usage` are cumulative snapshots of
the same execution, never additive increments. The terminal result is
authoritative when present; otherwise retain the latest validated event.
Do not erase an event with a nil result or turn missing usage into known
zero. Apply existing nonnegative/finite/bounded validation.

The decision owner consumes that normalized snapshot after completion;
only when absent does it parse the existing raw answer usage as its
fallback. In the fixture, raw 41/3 and normalized 41/3 mean one 41/3
record, not 82/6 or two records. Keep the SDK account-usage invalidation
barrier; it is not another billing append. `serveDecideTries` owns the
ordinary decision record; the classifier's existing `systemOne` owner
owns its routing record. Transport and Decisions translation must not
append a second copy. A cancelled/uncertain attempt retains any measured
usage and its operation outcome without replay.

## Ordinary adapter seams for the host owner

| Owner | Bounded change |
| --- | --- |
| `pkg/extension/types.go`, `provider_operations.go`, `manifest.go`, matching author SDK | Add the operation/model association, minor-7 admission and execute gates, cloning and validation; never inherit chat protocols for decision-only models. |
| `internal/provider/extension_provider.go`, `extension_models_cache.go`, `decide.go` | Project and persist exact selected-account decision facts. Preserve known-empty catalogs and generation checks; resolve through the ordinary extension transport without hard-coded Jev. |
| `internal/provider/extension_operations.go`, `extension_transport.go` | Recognize exact `POST /v1/systemone`, recheck negotiated minor and model-operation association, reuse execution lease/auth/network/usage/journal ownership and preserve the final usage snapshot. |
| `internal/gateway/decide_group.go` | Add attempt/fence handling to both `serveDecideTries` and `askDecideTries`, including typed classifier propagation and account/group fallback. |
| `internal/gateway/decide.go`, `decisions_systemone.go` | Preserve typed transport/read failures, bounded bodies, ordinary authorization and response headers; consume normalized usage once and reuse existing Decisions translation. |

The host owner can implement and accept these after the shared final
acceptance. This repository contains no host patch. The first host
deployment with zero plugins is independent of this module.

## Own-model metadata for checked agent configuration

SDK `Model` already carries `MaxOutput`, `Efforts` and `DefaultEffort`.
The loss occurs in ordinary agent options: `internal/agent/agent.go:Option`
has context and picker presentation fields, while
`internal/agent/extension_actions.go:extensionAgentOptions` forwards
`viaMagpie` model options without those capability facts. This module's
`agent.go:agentOptions` receives context only. Preserving existing BYOK
metadata does not supply capabilities for a newly selected own model.

The minimum generic addition is normalized model fields
`maxOutput`, `efforts` and `defaultEffort` in the checked options snapshot
sent to `agent.inspect` and `agent.plan`. Use the effective
`provider.Entry` already selected by `provider.CatalogFor(agent)`:

| Field | Exact source and wire rule |
| --- | --- |
| `context` | Effective `Entry.Context`, as today. |
| `maxOutput` | Positive `Entry.PublishedOutput()`, after provider/model output and context overrides; omit when unknown. Never publish raw `Entry.Output` above the effective context or look up a donor/static vendor table. |
| `efforts` | Effective `Entry.Efforts` after user overrides. Serialize unknown as `null`, explicit no-level support as `[]`, and a known list as its strings. An effective `Reasonless` entry explicitly has `[]`. Do not use `omitempty` on a bare slice or a length check that collapses nil and empty. |
| `defaultEffort` | Present only if the effective nonempty effort list still contains the default; otherwise omit. A removed/invalid default cannot be reintroduced from vendor metadata. |

Preserve nil versus empty through discovery, persistence, cloning, option
cache and final serialization. SDK `Model.Efforts` currently has
`omitempty`, so changing only `Option` would still lose an explicit empty
list before it reaches the catalog. The host owner must version/preserve
that representation as part of the metadata contract. Apply existing
numeric and effort validation; invalid input is not a fabricated default.
No credentials or vendor-shaped configuration belong in the snapshot.

The ordinary seams are `Option`, `route.go:buildVia` (including routing
group options), `sameOption` and existing option cache cloning/equality,
then `extensionAgentOptions` serialization. Every branch must carry the
same effective facts and invalidate on catalog/settings changes. The
module then owns mapping them to Command Code BYOK configuration while
retaining foreign metadata and subsequent user edits. This metadata
addition is independent of SystemOne and remains pending.

## Fixture status and acceptance handoff

`SYSTEMONE-SDK-FIXTURES.json` contains literal pinned donor wire and
authored, explicitly pending host/agent scenarios. The unchanged donor
probe consumes its wire section. Module regressions exercise the actual
SDK 1.6 refusal, precise `not-started` action outcome, no network/events
and all recovered Jev aliases outside conversational execution/discovery.
Those checks do not implement a hypothetical host.

Pending native acceptance must install a real API 1.7 fixture module and
exercise ordinary SystemOne, Decisions and classifier owners: both minor
gates; key/model/account denial; live/cold known-empty and stale catalogs;
no inherited chat protocol; accepted/uncertain/read-error fences with a
second candidate dispatch count of zero; headers; and raw plus normalized
usage counted once. Metadata acceptance must round-trip unknown and empty
efforts, effective output clamping/overrides and a removed default for a
custom model absent from donor tables, then connect/sync/disconnect.
These are blockers for the relevant module capabilities, not for the
independent first plugin-free core deployment.
