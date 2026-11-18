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

## Host permit and diagnostic ownership

The public SDK validates a positive recovery permit before another dispatch:
the step must equal the named request, the action must be `wait`, and the
permit must agree with complete response facts and explicit safe-replay
evidence. Terminal class, scope or denial contradictions fail closed.
The package uses that shared validator; it adds no private scheduler.

The host records recovery diagnostics when an invocation enters its recovery
owner, including revocation during a wait. An outer package/account/config
admission gate can reject before recovery starts; that gate's error is the
diagnostic, and no recovery row is promised for a callback never admitted.
Neither case authorizes a repeat after revocation.

These are current shared-host contracts. The earlier report's malformed
permit and missing recovery-diagnostic findings describe its captured SDK,
not the current implementation. The [unaltered historical report](history/named-read-policy-2026-11-15.md)
retains the original failed probes, receipts, source identities and limits.
Final acceptance must replay the relevant tests against the exact host being
released; a prose correction does not establish that acceptance.

## Verification entry points and limits

[`tests/read_policy_host.py`](../tests/read_policy_host.py) installs the
actual handlers through an exec fixture and establishes synthetic accounts
through ordinary auth. It exercises selected CLI search/catalog/schema calls,
denial, malformed responses, redirects, paid-call fences, root-budget
exhaustion, cancellation and configuration/account/package revocation.
It uses a loopback vendor and stops and joins its processes.

[`tests/permit_contract_test.go`](../tests/permit_contract_test.go) retains
the standalone public-helper repro. The host and public SDK own the permit
validation and recovery journal regressions. Package units, installed exec
fixtures and native loading are separate gates; a C-shared build alone is
not native-runtime acceptance.

See [facts projection](FACTS-PROJECTION.md), [verification](VERIFICATION.md)
and [source provenance](SOURCES.md) for the remaining representability limits,
exact donors and evidence. No live-vendor, current cross-platform native or
whole-application acceptance is established by this reference.
