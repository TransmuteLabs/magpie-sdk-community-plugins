# Native execution facts and scoped diagnostics

Paired0.5 and standard0.8 now require SDK API1.12 for the bounded
request-policy integration (source checkout: `docs/REQUEST-POLICY.md`).
Selected-account cache tasks
and provider options still use their original API1.9 contract. Both use the public
`Usage.Facts` envelope without mixing it with the four legacy SDK totals.
API1.8 union results are historical; the task adapter's exact API9
inputs and results are separate in `CACHE-KEEPALIVE-EVIDENCE.md`.
The inbound-only 0.2.0 distribution does not execute a provider or emit
these diagnostics.

Generated descriptors at the commits and hashes in source-lock.json are
the source authority. Informational facts do not grant model/account
authority or establish successful completion.

| Native evidence | SDK fact |
| --- | --- |
| ModelUsageStats 2/3/4/5, with a usage message present | Fresh input, output, cache writes, cache reads; explicit zero pointers retained |
| Response28 exact Token Usage dimensions when field7 has not supplied a snapshot | Sparse named input/output/cache counts; no inferred absent siblings or totals |
| Response 23, otherwise usage 9 | ServedModel; a later usage fallback cannot replace an explicit actual model |
| Usage 6 | UpstreamProvider as the exact generated API_PROVIDER enum name; unknown informational values retain their numeric code |
| Response 17 | Native RequestID |
| Response 1, otherwise usage 7 | Native ResponseID for raw paired execution |
| Canonical renderer's actual response object ID | ResponseID for Chat/Messages; matches the client response and usage ledger |
| Response 18 present | Committed native credits, including explicit zero |
| Response 22 present, finite nonnegative fixed64 double | Reported cost with the explicit native unit `ACU`; never USD or an inferred exchange rate |
| Validated native stop enum | Stop as the exact generated enum name |
| Failed SDK/native terminal | Bounded local ErrorType; earlier measured facts remain available |

Usage messages use protobuf scalar defaults for zero counts. A completely
absent usage message never becomes four measured zeros. Per-frame updates
merge into snapshots; snapshots replace rather than add to earlier usage.
Field28 dimension fallback merges only named counters. A field7 snapshot
takes precedence over it, including its protobuf-default zeros. Known
dimensions use finite nonnegative integral fixed32 values within the same
int32 count limit; malformed measurements fail. See
[the maintained donor comparison](CLIPROXY-PROVIDER-CHECK.md) for exact
keys, source provenance and intentionally different field7 presence semantics.
Native raw bytes remain unchanged. Metadata is bounded and rejects control
characters; selected credentials cannot enter execution facts.

The recovered descriptors do not establish reasoning token counts,
one-hour cache write counts, deferred counts, service tier or cache lifetime.
These optional SDK fields stay absent. Thinking text is not a token counter.
Preliminary credit cost is not committed credit cost. Quota basis points,
an unspecified overage currency, billing/requested-model distinctions and
arbitrary response headers do not have equivalent facts here; no currency,
charge or authority is guessed. Quota-window display/balance/plan-end
limitations are separate from this execution envelope.

Measured snapshots are emitted while the current execution lease is live,
including before a later malformed/refused terminal. Failed terminals do
not issue origin receipts or successful client terminals. Cancellation
does not acquire a detached context or retain credentials to report later.
A canceled/revoked lease may prevent a final callback; already admitted
snapshots remain the host's evidence.

Diagnostics use only `diagnostics.emit`: fixed `devin.execute.accepted`,
`devin.execute.completed`, `devin.execute.failed`,
`devin.execute.cancelled` and corresponding account completion codes.
Phases are module-owned preflight/dispatch/stream/completed states. HTTP
status is the actual emitted response status or an observed account HTTP
status, not an enum or a guessed vendor status. A streamed failure can
therefore retain HTTP 200 with a failed diagnostic code.

No raw error message, vendor phase text, credential, header, request body,
path or session identifier is sent in diagnostic parameters. The host adds
its own scope. Each callback is bounded to 200 ms inside the current
context. Rejection, throttling, cancellation or an unrecorded diagnostic
does not change acceptance, dispatch count, receipts, retry policy or the
execution outcome. Host rate limits mean diagnostics are best effort.

The paid task additionally emits `devin.cache.completed`,
`devin.cache.failed` or `devin.cache.cancelled`. Optional preparation and
scheduling refusals use `devin.cache.unsupported` and
`devin.cache.schedule-refused`. Cache-task usage snapshots precede a held
terminal and the host flushes its last admitted snapshot once on cancellation.
Preaccept credential failures emit no task usage. Background output creates
no signed-history receipt.

`provider_execution_facts_test.go` and `codec/execution_facts_test.go`
cover the mapping, absent/zero distinction, raw/canonical summaries,
actual-model precedence, malformed costs, terminal failure and diagnostic
refusal. `donor_host_e2e.py --case execution-facts-ledger` checks the actual
installed host's ledger, canonical response ID, native request ID,
credits/ACU units and structured diagnostic log. Exact input/result hashes
belong in DELIVERY-EVIDENCE.md; SDK-peer tests alone are not host acceptance.
