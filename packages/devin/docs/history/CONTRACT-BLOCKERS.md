# Remaining native Devin semantics and their owners

## Domain completion, 2026-11-15

The current public API9 SDK7bc5ae21 already supplies model efforts and
numeric quota metadata. This module now uses them for discovered families,
ordered effort/Fast/Priority variants, source aliases, numeric ACU
amount/limit, signed overage micros, quota spans and plan-end resets.
It retains concrete UID and selected-model receipt identity while changing
only the native dispatch UID. Every virtual selection re-reads the selected
account's catalog; the name index stores no account/variant authority.
`DOMAIN-COMPLETION-EVIDENCE.md` records module verification and ordinary
installed replay on supplied candidate physical tree36f8e629/binary0bd335b7
separately from final successor and parent whole-host acceptance.

The precise remaining generic DTO limit is `pkg/extension/types.go`:
UsageResult has windows/revision/freshness but no top-level plan/expiry
fields. A plan-end informational window/reset represents the known value.
UsageWindow supplies amount/balance/unit/span/display, so these fields
require no host work. Overages remain signed micros because the recovered
native field does not establish a currency.

Another exact generic bound is the model-ID grammar:
`pkg/extension/manifest.go`, `usage_contract.go` and `account_tasks.go`
reject whitespace. Community `index.mjs:433` onwards maps a config without
a family UID to its label, including MODEL_PRIVATE_11 → `Claude Haiku 4.5`.
Such label-only virtual IDs cannot be exposed through this contract.
The module preserves the real concrete UID, display name and Free scope;
it does not invent a normalized alias or invalidate the entire catalog.
`TestProviderDomainLabelOnlyFamiliesKeepConcreteSDKIdentities` has a real
candidate2 RED and candidate3 GREEN. No host change is needed to use the
concrete model; exposing the label itself as an ID would need a deliberate
generic contract change.

Remaining cache limits are vendor wire limits, not missing host transport:
`module-39768.js:2337` PromptCacheOptions contains only its type enum;
`module-39768.js:2366` ChatToolDefinition has no cache field. A5m EPHEMERAL
mark and a mark at one real tool result's final boundary are representable.
A1h TTL, a tool-definition mark and an intermediate mark inside that
single result remain explicit refusals. No prompt substitution, synthetic
tool result, new host vendor logic or runtime companion is introduced.

## Current full module, API1.9 on committedd5b098d8

Default0.8 is one self-contained native package. It consumes the final
unpublished API9 typed decision-model catalog and selected-account task
contract; explicit paired0.5/API1.7 and inbound0.2/API1.1 are compatibility
artifacts. The accepted tree11c2 and parent binaryc018 built from tree8fe
differ only in two test files. Exact source/binary/library identities,
the historical typed compatibility RED-to-GREEN and current installed
receipts are pinned in `DELIVERY-EVIDENCE.md`.
One additional hot paid replay passes on frozen merged treef7f26b2d/
binary9ddc4e4a, with unchanged public SDK/admission code. That tree is
recorded separately rather than attributed to a future commit.

Actual installed duplicate/no-op/held read, real model transitions and
genuine Gemini Chat/Messages JSON/SSE pass on the parent-built physical
host. Finite bootstrap, caller identity, signature receipts, binding
revision and accepted-write journals retain their authority checks.
API9 decision models are validated separately from conversation models
and never written into client configuration.

The old finished-Stream cache boundary is resolved by public API9
selected-account tasks. The module retains bounded credential-free
templates and default-off paid consent. Three full strict hot-config-CAS
fixtures pass real cadence, budget/model/readOnly controls, both paid
arrivals, cancellation/join and two once-only measured ACU rows without
restart or catalog warmup. Nine sampler regressions and the exact old
parser's causal failure preserve the wire distinction: native sampling
uses fixed64, while valid fixed32 scanning has structural coverage.
All earlier REDs remain separate, including467 no-paid and the original
c018 held38 timeout. That timing symptom is unexplained/unreproduced;
the new fixture's passes do not prove its cause or repair. No further
generic host change is proposed on this evidence.

Remaining boundaries and proof limits:

- Native account operations require the local owner and an explicit
  provider model. Named remote/restricted-key delegation and ambiguous
  group-account selection need an explicit generic authority policy.
- Literal native `Basic key-key` is not decoded as a supported remote
  gateway credential. Loopback wire fixtures do not prove remote access.
- Original Cognition-client startup, live vendor behavior, cache lifetime/
  charges and full native/cloud resume are untested. Earlier reconstructed
  Rust startup and cold CLI proofs remain separately identified.
- Refresh/revoke endpoints and additional native management methods are
  not recovered or implemented. Expiry requires sign-in; logout removes
  host state. Donor model/schema variants remain in `DONOR-MATRIX.md`.
- This earlier checkpoint projected quota percentages. The domain
  completion above adds API9 numeric metadata; execution Facts remain a
  separate measured envelope and never infer unavailable currency/counts.

No further host implementation scope is taken here. Broad host acceptance,
other platforms and SDK-only rollout remain independent parent concerns.

## Historical selected-account API1.7/API1.8 checkpoint

The following section predates the committed API9 runtime. Its missing
cache-task seam and uncommitted-host statements describe that checkpoint,
not current availability. Current results and limitations are above.

The historical union runtime is d070; earlier domain proof uses committed
7fbda87. The approved upstream/no-op base is
`9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, after merge 75fa28b1 and startup
87359a7. Final API8 authoring uses a separately hashed own capture of parent
dirty files; the earlier API7 checkpoint remains separately recorded.
Those files are never
attributed to the base commit or mutable startup source. Source6, native account
operations and persistent model discovery already share one runtime;
no separate nativeops host or built-in experimental loader is needed.

Paired installed-host acceptance passes, including native catalog key
restrictions, aliases, token budgets, read-only policy and cancellation.
Fresh ordinary CLI model selection with the gateway stopped also passes,
without another vendor call. Cold catalog persistence and a missing union
commit are **resolved**, not current blockers.

GetUserStatus, GetCliTeamSettings, GetAccountManagedPlugins and AssignModel
have implemented SDK 1.6 adapters. Historical d070 agent acceptance is **red**:
an observed duplicate AssignModel returned HTTP 200 and reached the local
mock twice. Another bounded trial failed a bootstrap read before assignment.
Successful agent/resume and reconstructed-client startup runs do not erase
these failures. The controlled installed-host reproduction now proves
module RequestID/Session stay stable while no-op sync changes only host
binding revision. That change admits a second write journal key and
rejects an already dispatched read. Exact preimages, event order and the
minimal common host proposal are in `ASSIGNMENT-DIAGNOSIS.md`.
Broader results and proof limits are in `UNION-EVIDENCE.md`;
earlier 9c evidence remains historical.

The leader's conservative no-op candidate now passes both original
installed-module gates: duplicate identity remains refused across no-op
sync with one dispatch, a held bootstrap read returns its exact payload,
and the unchanged real-transition script preserves revocation. Exact
source/helper/binary/original-library hashes are in `NOOP-ACCEPTANCE.md`.
The candidate was built from an own frozen copy, hashed before/after build.
The parent's mutable remote source may subsequently change. Both gates
also pass on committed 9e88 and the first captured API7 overlay, with the
original unchanged module. The refreshed API7 capture also passes both
gates with original and current libraries, and independent installed
Facts/diagnostics/cancellation gates pass (`DELIVERY-EVIDENCE.md`).
Final API8 source277f/binarya08f repeats those gates with both original and
final libraries, plus cold CLI, ordinary reconstruction startup and all
native catalog policy controls.
No additional no-op host scope is requested;
the final committed API7 union and broad suite remain leader-owned.

Canonical caller projection and three history-repair defects now pass
actual installed RED-to-GREEN gates on scoped sourcec434/binary2fe733.
`DELIVERY-EVIDENCE.md` records the unchanged libraries, non-SDK positive,
same-owner private metadata and no-op/real-transition controls.
The generic fix is present in the shared working tree, uncommitted for the
parent. Final freeze/upstream replay and broad acceptance remain separate.

Remaining generic host boundaries belong to the leader:

| Boundary | Existing seam and required guarantee |
| --- | --- |
| Delegated native account operations | `internal/gateway/protocol_account_operation.go` `nativeAccountCaller` permits only the local owner; `protocolAccountOperation` refuses group references. Supporting named remote/restricted keys or groups needs an explicit delegation/selection policy preserving limits, caller visibility and the owned binding/account generation. This module must not guess an account or bypass those checks. |
| Native caller credentials outside loopback | `internal/gateway/lan.go` `callerKeys` strips Bearer but treats literal `Basic key-key` as an opaque candidate. A supported credential decoder needs declared ingress scope and ordinary caller/limit checks. Loopback success cannot establish named remote-key support, and recognizing the key alone would not grant native account-operation delegation. |
| Cache keepalive after a reply | `pkg/extension/stream.go` `Stream.HostCall` rejects a finished/canceled parent; `pkg/extension/composite.go` `ModuleContext` supplies no account-bearing background invoker. A scheduled ping requires a fresh host-admitted accepted/journaled operation, current account/model/proxy/quota authority and lifecycle cancellation. Domain bytes and scheduling policy remain here. |

The scoped candidate already executes a checked empty ActionPlan while
retaining stable no-op authority and all tested real-transition fences.
Its helper additionally refuses `applyShell != nil`. Do not remove binding
checks or add module-side retries. This module supplies the ordinary
installed-host regression; common fixes remain in `../magpie`, with broad
host/union acceptance owned separately.
Initial SDK-only host rollout is independent of optional Devin acceptance.

The actual pre-fix real-model negative control now passes:
ordinary CLI selection changes the owned model/revision and refuses both
a held team-settings read and a previously accepted assignment reply.
A distinct old-model assignment is refused before vendor dispatch with a
not-started journal. The held accepted write keeps its original scope and
completes once despite the outward 403; it is never retried.
`ASSIGNMENT-DIAGNOSIS.md` records the exact chronology and raw failed fixture
assertions separately. This supports the conservative no-op fix while
retaining revision in write identity and the exact Check. Other real
revocation types still need the leader's generic regression coverage.

Messages encoding/rendering/scoped-origin continuation have SDK peer
coverage and unchanged ordinary installed acceptance on the corrective
snapshot. Independent installed history/image/usage controls pass;
`DONOR-HOST-EVIDENCE.md` preserves historical greens and reds, with current
canonical receipts in `DELIVERY-EVIDENCE.md`. Remaining
community schema/model variants and additional native methods are
module-domain work, not reasons to add vendor adapters to core. That
earlier quota DTO lacked numeric display/unit/balance fields. Current API9
projects the representable numeric metadata described above without
inventing currency or a top-level plan/expiry DTO.
Refresh/revoke endpoints have
not been recovered. Cloud ACP/hosting and the original Cognition client are
outside the current proof; no claim of their compatibility follows from the
reconstruction or local wire fixtures.

## Historical codec-only analysis

The remainder records host `01bcc7c7d155b029769327f909372b2ff72252e2`
before the paired executor and SDK 1.6 bootstrap were implemented. Its
statements about missing implementations describe that checkpoint only.

At that checkpoint the module was an inbound codec and model-discovery projection.
Generated descriptors establish field identities and wire types; they do not
establish equivalence to another provider's features. Remaining failures are
explicit, before canonical dispatch.

## Existing same-module route

The frozen merged host already has a path for native fields that an
ordinary Chat provider cannot understand. These features do not all require
a new SDK transport:

1. `protocolext.Request.Metadata` and RequestID reach the host-owned request
   context in `internal/gateway/protocol_extensions.go:235`.
2. `internal/provider/extension_transport.go:122` passes that metadata as
   `ExecuteRequest.Options.native` only when the selected outbound executor
   has the same module id. Other modules and ordinary HTTP providers do not
   receive it. Caller identity comes from the host.
3. `internal/extension/manager.go:677` reuses the pinned inbound lease for
   same-module execution. Its account scope and credentials still pass the
   host's generation checks; inbound metadata cannot choose account authority.

A combined inbound/outbound Devin module can therefore preserve native
generation constraints, signed history and native tool definitions in its
own representation, validate origin/model/account policy and execute the
real native protocol. The missing piece is that executor and its vendor
state/auth logic, not a generic metadata channel. The current package
declares no outbound executor and must retain its refusals on ordinary Chat
routes.

Native-only requests also need a fail-closed destination policy: a group,
alias or fallback must not send their canonical projection to an ordinary
provider after dropping private metadata. The paired executor must validate
the actual selected model/account against native provenance before dispatch.
The mere presence of a metadata channel is not proof of this route policy or
of compatibility with another provider.

Metadata is bounded to 64 KiB and is not an authentication assertion. It is
unsuitable for copying an entire maximum-sized history or credential
envelope. Larger state needs a bounded, request-scoped module-owned handle,
bound to the pinned generation and disposed on native response completion
or cancellation; that mechanism is not implemented here.

The HTTP E2E separately proves the ordinary HTTP-provider route strips the
original native credential envelope and credential headers and enforces the
caller's model allowlist. The scoped SDK metadata channel described above
was checked in the frozen host source, not exercised by this codec-only
fixture. The same-module outbound path is not implemented in this package.

| Native feature | Evidence | Missing end-to-end guarantee |
| --- | --- | --- |
| Signed history: prompt 12/18, Gemini signature 17, provenance 15/16/22 | `module-39768.js`, ChatMessagePrompt | Same-module native execution can preserve this through existing metadata, with validated origin/model/account generation. The ordinary host Chat parser reconstructs thinking text without those properties. A generic cross-provider route still needs a typed preservation policy. |
| Redacted thinking: prompt 13 | Same descriptor | A typed opaque/redacted reasoning item with explicit origin and replay policy. It cannot become plain thinking text or a native signature. |
| Tool failure: prompt 9 | Same descriptor | A same-module native executor can retain this flag in existing metadata. The ordinary host IR has IsError but Chat decoding does not read the native flag; cross-provider routes need a supported canonical representation. |
| Generation constraints: max_newlines/top_k, configuration 3/7 | `module-98224.js`, CompletionConfiguration | A same-module native executor can consume the existing metadata channel without new core transport. Ordinary provider routes still need explicit capability negotiation and rejection. Returning success after dropping 400/40 changes a real observed native request. |
| Structured output and verbosity, configuration 18/19/20 | Same descriptor | The descriptor only declares strings. Establish actual native emitter values, JSON-schema envelope and destination support before claiming an equivalent mapping. |
| Custom/computer-use tool definitions, fields 5–11 | `module-39768.js`, ChatToolDefinition | A same-module native executor can retain these in its representation. Cross-provider execution needs typed destination capability and tool-policy semantics, including custom grammar and native tool-call shape. Do not recast a custom tool as a function. |
| Assignment and arena tokens, request 24–26 | `module-432.js`, GetChatMessageRequest | An assignment owner with authentic vendor state, scoped account generation and cancellation semantics. No generated or reused JWTs. |
| Native session-control, billing and account RPCs | `module-16806.js`, ApiServerService | Separate host service contracts or an authorized external vendor executor. These are not aliases of Chat/Models, and fabricated success would lose state. |
| Vendor team-scoped model discovery | `module-432.js`, three ForSite requests | A declared team/account authority. The current Models projection is scoped by the host caller, so nonempty native team ids remain refused. |

Tool-result images are not a blocker: the current host's
`internal/gateway/ir.go` toolOutput and `chat.go` tool-message parser preserve
inline image parts and call association. This module now uses that existing
public Chat shape. Native image captions remain refused without emitter
evidence for their model-facing meaning.

These findings were checked against host commit
`01bcc7c7d155b029769327f909372b2ff72252e2`. Existing generic contracts should be
used first; future vendor auth/transport belongs in an external module. No
host source is modified by this repository.
