# Shared request policy

The full OpenCode 0.8.1 package uses public SDK API1.12 and native ABI1.
Every constructor that executes
request-policy code declares that capability and floor: full, the legacy
`--compat-api8` flag, provider0.2.0 and agent0.3.0. The provider and agent
versions identify compatibility and test artifacts. The archived provider JSON keeps its historical
API1.3 bytes; `Manifest()` raises the actual runtime floor. `--compat-api8` no
longer claims an API1.8 runtime and inherits the full constructor's metadata
capability. Historical receipts remain historical.

Go Chat/Gemini and personal/public Zen pass initial error responses through
`executionRefusal` before any codec/tool rewrite or output. Bounded facts
preserve original status and actual code/type/message plus standard timing
headers. JSON is limited to64KiB/depth16, code256, type128, message1024 bytes
and four headers of at most256 bytes. Unknown, duplicated, null/wrong typed,
truncated, oversized, invalid UTF8, credential-reflecting or contradictory
envelopes do not establish proof. `code:null` is absent code; an actual numeric
code retains its original number spelling. No digits are mined from prose.

The provider proof is narrower than host classification: a complete initial
HTTP error-only envelope, the appropriate selected provider, and its recovered
preflight error type/status. Only Go429 `GoUsageLimitError`, personal Zen402
`CreditLimitExceeded`, public Zen403 `FreeTierError` and public Zen429
`FreeUsageLimitError` are admitted as quota or free-client refusals. HTTP
400 `invalid_request_error` with `cyber_policy` and the documented Gemini
403 terms-of-use error are request-scoped `content-refusal`. The native
Gemini envelope must have numeric `error.code: 403`, status
`PERMISSION_DENIED` and the complete terms-of-use sentence. The Zen
`server_error` envelope must have that sentence immediately after
`Upstream request failed: [403] `. Unknown extra fields, codes and effect
metadata defeat proof. `RefusedExecution` sends the matched facts, rule and
evidence to host policy.

Go quota and personal credit are scoped to the selected account within that
provider. Public `FreeTierError` is terminal request scope. Public
`FreeUsageLimitError` has unknown scope: donor evidence includes source-address
and client-shape restrictions, so it never becomes an account fallback or
guessed shared bucket. The host owns sibling selection, retry budget and
waiting in its existing smart/order route. No timers or account selection are
implemented in the module. Standard HTTP timing stays original; body reset
fields and model/window applicability are not guessed.

The host's dispatched journal protects the in-flight call before its final
receipt. Initial HTTP refusal inspection precedes accepted; every success
publishes accepted before reading paid output, and every unproved refusal or
transport error retains the accepted/uncertain barrier. The buffered error
prefix is restored exactly for the existing opaque path; its original closer
and read failure remain authoritative. This is a limited change to the prior
pre-dispatch acceptance boundary, justified by explicit initial rejection
inspection and the dispatched journal. It does not make silence or EOF proof.

Zen Responses streams report exact `type: "error"` and `code: "cyber_policy"`
fields as `content_refusal` with a fixed message and an `uncertain` outcome.
Duplicate keys and case variants of `type` or `code` defeat classification. This
includes public Responses streams shaped for a non-streaming caller. The
accepted fence remains in force, and previously emitted data stays in place.

Native session headers, tool rewriting, public forced streaming/collapse,
Go/Zen credentials, cache usage and successful forwarding are unchanged.
Discovery's existing cache/fallback owner and usage/auth/Console operations do
not acquire an execution recovery callback: their transport facts do not prove
a whole paid execution refusal, and the public callback does not grant those
methods authority. The paired CommandCode adapter uses a named read-only
subscription GET when justified by its execution preflight.

Intentionally not automatically recovered:

| Flow | Reason |
| --- | --- |
| Public FreeUsageLimitError | Unknown network/client bucket; no account or egress authority |
| Personal paid upstream `server_error`, generic429/5xx, redirects and unknown types | Vendor acceptance is not proved absent |
| SSE errors, late failures, successful native output, tool/cache/session progress | Accepted/output fence; no whole-execution restart |
| Transport or body interruption, malformed/ambiguous/oversized envelopes | Incomplete effect evidence; fail closed |
| Google ErrorInfo/details and BudgetLimitError coverage/reset metadata | Exact applicability requires a dedicated reviewed adapter; no invented model/window semantics |
| Auth refresh, device exchanges, Console watch acknowledgement | Separate transactional/effect owner; no credential replay contract in API1.12 |
| Catalog/usage fallback outside execution | Existing read owner, no implicit request.recover authority |

See [the plan](REQUEST-POLICY-PLAN.md), `source-lock.json` and
`REQUEST-POLICY-VERIFICATION.json` for exact source, old-runtime failures,
guarded regular-usbox receipts and verification limits. All vendor fixtures
are synthetic. [The current closure](LOCAL-CLOSURE-20261116.md) adds locally
authorized macOS strict-SDK/common-host checks while preserving that history.
Live vendor/app/credentials, Windows behavior and independent Astra review
remain unverified here.
