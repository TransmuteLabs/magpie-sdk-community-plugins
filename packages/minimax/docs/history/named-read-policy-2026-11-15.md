# Named read request policy

`SelectedClient.JSON` uses public API 1.12 `RequestWithPolicy` for exactly:

| Endpoint | Named step | Actual callers |
| --- | --- | --- |
| `/mavis/api/v1/mcp/web_search` | `minimax-web-search` | Selected `module.call` search action and `tool.call` web search |
| `/minimax-cloud/api/v1/connectors/connected-tools/list` | `minimax-tool-catalog` | Tool listing/schema, connector validation and media workflow catalog reads |

The request transport asks the existing selected host network/proxy owner for
each dispatch. It uses the production HTTP client with redirect forwarding
disabled and bypasses the general site-request redirect loop. A permit repeats
the same POST/body with the original account credentials and context. No
package counter, delay, account selector, credential refresh or final provider
refusal is added. Cancellation reaches the request and host callback through
the existing SDK stream context.

Each response is bounded and closed before classification. Complete bytes are
restored for the public helper so a denied permit preserves the original
terminal response. Success retains its bytes. Transport errors, redirects,
partial reads and malformed or unknown envelopes acquire no recovery callback.
The strict error-only subset has unique allowed keys, one nonzero canonical
numeric code and a string message: `base_resp`, `statusInfo`, top-level
`code`/`message` or `code`/`msg`, or an `error` object with numeric code/message
and optional bounded type. Optional top-level request/trace IDs are strings.
Success/progress/task/receipt fields, conflicting status envelopes, null or
missing required fields, duplicate keys, encoded bodies and ambiguous content
types fail closed. The existing JSON decoder still determines which responses
are failures; successful HTTP-200 error-shaped content is not reinterpreted.

The callback contains original HTTP status, bounded selected-secret-redacted
code/type/message, parent method, response phase and complete read evidence.
Only validated actual Retry-After reaches typed delay/reset and the permitted
header. Explicit zero survives. Durations above the typed thirty-day limit
retain their actual header; absent/invalid timing supplies no guessed reset.
Request query/prompt fields, arbitrary headers, raw bodies and request
identifiers are not copied into the DTO. Bounded vendor message facts retain
their classification role. Internal `FailureFacts` remain at the adapter.

Content/request/auth/credit/permission declarative rules precede request-scoped
HTTP 429 rate and 500/502/503/504 outage rules. Classification inputs remain
separate from exact-endpoint read evidence. Other valid facts may be classified
by operator policy; the package invents no fallback matcher.

Generic `/connectors/tools/call`, transfers, uploads and paid work retain their
existing acceptance and uncertainty fences. Catalog reads may retry after
paid acceptance, but never replay that earlier work. The ordinary-host image
fixture proves two separate catalog recoveries around one generation and one
asset lookup. A one-permit root budget stops the second recovery after three
catalog dispatches and exactly one paid invoke, returning uncertain.

## Evidence and ownership

New receipts are at
`usbox:/tmp/magpie-minimax-workhorse-20261115.VNyyfYMu`.
They retain commands, isolated HOME removal, descendant guard receipts, source
snapshots and artifact hashes. The pre-existing packet
`usbox:/tmp/magpie-minimax-readonly-20261115.dpRK5lUi` and its genuine
`read-red` failures are unchanged.

The immutable combined SDK source is
`/tmp/magpie-policy-combined-mbp6ictj/source`, identity
`e00716396d054335c8848b07ff7451756b9d6d13710f2837a83c00525fb0cb3a`.
Its host binary SHA-256 is
`1bffcd572127c30ebfe39a89be9a11561b5864901f1a1d1b218cb6d72607e5f7`.
The public helper hash remains
`be486205f9b4cc2e37dd003e20e0602dd27470788a1c3419ac045f3f1acd1c3d`.
API minimum 1.12, donor version 0.2.9/commit
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`, copied fixtures, final chat refusal
source and the corrected stream-aware Chat fixture remain preserved.

`tests/read_policy_host.py` installs the actual handlers through the existing
exec fixture, establishes two synthetic accounts through ordinary auth, and
uses ordinary selected CLI invocations. It checks search/web/catalog/schema,
denial, malformed response, redirect, paid invoke failure, successful accepted
media reads, shared-budget exhaustion, cancellation, config/account revocation
and disable. Diagnostic gaps are recorded separately from dispatch/effect
safety. Processes are stopped and joined; no native loading or live account
is used. Final combined-host/native distribution replay belongs to the parent.

`read-host-red` fails behaviorally with one search dispatch instead of two.
The first implementation's `read-admission` reaches the real callback but
classifies unknown because its vendor rules were absent; that package defect
and receipt are retained. `read-host2` then passes its first eleven cases and
exposes a fixture assertion expecting a failing CLI exit for revoked search.
The real result is a failed unary SearchResult, with one dispatch. Corrected
revocation probes retain that failure representation.

The package race suite, focused final named-read regressions, vet, scoped Go
and C-shared builds, formatting and static/provenance receipts are separate.
`SOURCE-FREEZE.json`, `RESULTS.json`, `ARTIFACTS.json` and the packet's source
archives identify the final bytes and which checks ran before later test-only
or documentation edits. No broad host suite is repeated for this delta.

| Receipt | Result and boundary |
| --- | --- |
| Prior `read-red`; new `read-host-red` | Behavioral failures on original production source; no setup/compile failure |
| `read-green2`, `read-green-final` | Named reads pass under race detection, including malformed facts, denied and structurally malformed permits, original business facts, timing, transport and paid-call fences |
| `suite` | One package `go test -race ./...` passes on the final runtime; later wire-fixture additions pass in `read-green-final` |
| `vet`, `build`, `native-build`, `manifest` | Scoped Go checks/builds pass; C-shared artifact is built, not loaded |
| `read-host2` | First eleven runtime cases pass; retained fixture exit-code assertion fails at config revocation |
| `read-revocation-final` | All three config/account/disable safety cases pass; disable diagnostic gap remains explicitly recorded |
| `format-check`, `static` | Empty gofmt listing; Python syntax, donor/SDK floors and copied fixture hashes pass |
| `permit-contract` | Three behavioral failures: contradictory, foreign-step and empty-action permits each authorize two dispatches |
| `read-host-diagnostics` | Behavioral failure: disable stops replay but emits no policy revocation diagnostic |

## Remaining contract findings

Disabling the selected package can stop a callback before its recovery
diagnostic is emitted. The retained revocation fixture observes one dispatch,
no replay and a failed SearchResult, but empty policy diagnostics. The strict
`--require-diagnostics` probe retains this host observability failure; runtime
safety and full diagnostic readiness are reported separately.

The public helper checks `RequestRecoveryResult.Retry` without validating that
its step matches the named request or its action is `wait`. Structural JSON
type errors fail closed in the actual SDK wire tests. However, a well-typed
contradictory or foreign-step permit can authorize a second dispatch in the
standalone `tests/permit_contract_test.go` repro. That probe is deliberately
outside the normal `src` package suite and reports an SDK contract gap.
Ordinary-host fixtures return coherent permits. The package adds no private
permit validator or replacement scheduler; this finding belongs to the public
SDK/host owner before complete malformed-permit acceptance is claimed.

Other diagnostic knownness limits remain in `FACTS-PROJECTION.md`. Independent
criticism/review is reserved for Astra. Current native loading, final host OBS
fix replay, non-Linux runtime acceptance and live vendor compatibility remain
unverified. The new native build is an artifact, not native runtime acceptance.
