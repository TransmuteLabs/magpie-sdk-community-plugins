# CLIProxyAPI behavior comparison

Date: 2026-11-15. This is a source-based regression inventory, not a live
provider acceptance report.

## Source authority

The maintained donor is
`../CLIProxyAPI-family/CLIProxyAPI-u2-331t-up42`, branch `main`, commit
`842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9`
(`v8.0.42-698-g842cedec8`). That commit consolidates work in progress. Its
presence does not establish that the whole donor passes its tests.

The checkout also contains an unresolved merge. The committed comparisons below
were read using `git show HEAD:<path>`. Staged, conflicted and untracked changes
are separate evidence and must not be attributed to that commit. For example,
the working-tree `docs/codex-refresh-recovery.md` is an added file rather than
a document shipped by this HEAD.

The older clean checkouts were identified separately:
`CLIProxyAPI` at `feacc8264bd637ece733049eab8f28f39d6a697f`, and
`CLIProxyAPI-up-u2` at `7638884b40a60c3fb534126b334b178f3b3bbcbb`.
They are not substitutes for the maintained donor.

## Shared behavior

Test paths in the second column are relative to the committed CLIProxyAPI
donor. Magpie paths are relative to the host repository. An existing test is a
coverage anchor; it does not claim a new run at the latest source.

| Behavior | Donor regression or implementation | Magpie owner and current disposition |
| --- | --- | --- |
| A future expiry does not prove the access token is still accepted | `sdk/cliproxy/auth/conductor_unauthorized_refresh_test.go`, `TestManager_Execute_UnauthorizedRefreshesCurrentAuthBeforeFallback` | Confirmed missing reactive recovery contract. A provider-local loop is not the fix. Shared design: host ADR 0028. |
| Concurrent 401 responses consume one refresh grant | Same file, `TestManager_ConcurrentUnauthorized_BarrierExecutionPreservesTerminalState` | Existing transactional owner: `internal/extension/refresh.go`; regression anchors in `refresh_test.go`. Reactive admission must join it. |
| Late success or 500 must not revive terminal unauthorized credentials | Same file, `TestManager_MarkResult_InFlightResultDoesNotReviveTerminalUnauthorizedAuth` | Generation and publication fences: `internal/provider/extension_generation_test.go`. Preserve account pause and replacement through recovery. |
| Delegated credentials have a different refresh owner | `sdk/cliproxy/auth/home_unauthorized_refresh_test.go`, `TestHomeUnauthorizedDoesNotRefreshOrReplay` | The selected host account owns renewal. A module cannot choose a second account or an independent token manager. |
| Refresh selects the effective provider executor | `sdk/cliproxy/auth/conductor_refresh_executor_key_test.go` | Module/provider/account remain explicit in `Scope`; `internal/extension/account_successor_test.go` rejects an unproved replacement. |
| Refresh transport uses credential policy, not an arbitrary execution override | `sdk/cliproxy/auth/request_proxy_refresh_test.go` | Check against Magpie's actual `ExtensionNetworkContext`/`Via` semantics. Do not transplant a different proxy precedence rule without evidence. |
| Unknown refresh outcome cannot safely spend a rotating grant again | Working-tree `docs/codex-refresh-recovery.md`, explicitly outside the committed authority | Already represented by host `refresh_store.go` and the malformed/crash/cross-process regressions in `refresh_test.go`. |
| Header-only EOF differs from partial model output | `test/codex_stream_disconnect_failover_test.go` | Existing transport/decoder terminal tests in `internal/provider/extension_transport_completion_test.go` and `internal/gateway/extension_transport_completion_test.go`. Magpie additionally requires accepted-write evidence before considering replay. |
| Partial output must not be replayed on another account | Same file, `TestCodexStreamDisconnectAfterOutputDoesNotFailOver` | Existing immutable execution receipt and `ExtensionAttempt` outcome owner. Provider adapters must preserve the terminal uncertainty. |
| An incomplete upstream stream is not invalid client input | `test/codex_incomplete_stream_error_type_test.go` | Check translated protocol errors and request diagnostics, not only module return values. |
| Caller cancellation is request-scoped and does not rest the account | `sdk/cliproxy/auth/conductor_claude_cancellation_test.go`, `TestManagerClaudeStreamTailCancellationIsAvailabilityNeutral` | Shared cancellation/attempt owner. MiniMax fixtures must cancel a real request and inspect resulting accounting. |
| An account-level quota and a model-level quota have different reach | `test/codex_quota_failover_test.go` | Shared quota applicability and selected-account projection: `internal/provider/extension_catalog_quota_test.go`, `extension_usage_barrier_test.go`. Vendor classification stays in the module. |
| A 401 followed by 429 cannot re-enable an operator-disabled model | `sdk/cliproxy/auth/conductor_cooldown_wave29_test.go` | Preserve manual account/model state independently from transient error and usage observations. Include the ordinary admission path. |
| A stale quota probe cannot erase a newer refusal | `sdk/cliproxy/auth/conductor_quota_probe_spent_freshness_test.go` | Usage revision barriers and generation checks, including foreign-process observations. |
| Only the admitting owner spends its rate budget | `sdk/cliproxy/auth/conductor_usage_sink_owner_test.go`, `TestIso1ARecordDebitsOnlyTheLimiterThatAdmittedIt` | `internal/provider/extension_attempt_usage_test.go` pins usage to module/provider/account/generation; `internal/usage/extension_calls_test.go` prevents ordinary provider executions being billed again as own calls. |
| Explicit successful zero usage is different from absent usage | `test/usage_logging_test.go`, `TestGeminiExecutorRecordsSuccessfulZeroUsageInQueue` | Sparse usage facts and normalized media presence tests. Preserve explicit zero rather than inventing an estimate or dropping the row. |
| Cancellation or a late transport failure still retains measured usage once | Donor usage owner and stream cancellation regressions | Existing `TestExtensionAudioLateFailureKeepsUsageAndFailureAccounting` covers ledger, request diagnostics and route trace together. |
| Request payload policy must survive final framing | `internal/runtime/executor/helps/payload_finalizer.go` and its tests | Follow the actual gateway middleware/conversion/executor path. Vendor framing must not silently restore fields removed by caller policy. |

## Provider-specific comparison

Devin's secondary donor already pins this commit in its `source-lock.json`.
The following committed tests are additional audit inputs:

- `internal/runtime/executor/devin_a3_law_test.go`,
  `devin_a4_law_test.go`: native execution and lifecycle invariants.
- `devin_b4b_wire_test.go`, `devin_b4c_alias_test.go`,
  `devin_b4d_named_value_test.go`: concrete model/alias and wire choices.
- `devin_stream_status_345_test.go`, `devin_stream_content_test.go`:
  status, partial stream and terminal behavior.
- `helps/devin_request_validation_test.go`, `devin_ref_roles_test.go`,
  `devin_usage_test.go`, `devin_wire_fingerprint_test.go`: request semantics,
  roles, usage and transport identity.

The comparison does not authorize copying prompt substitutions, fabricated tool
results, unscoped caches or unsupported wire fields. The current Devin
community 0.1.41 and recovered native protocol remain distinct primary inputs.

No MiniMax executor was found in the committed donor executor tree. MiniMax's
vendor protocol authority remains community 0.2.9 at
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`. Its shared failure handling must
nevertheless obey the same host policy.

## Retry and backoff ownership

Global host defaults and provider overrides must resolve through one reusable
engine. The engine owns attempt and elapsed limits, backoff, jitter,
Retry-After and cancellation. A module supplies its vendor classifier and
no-effect evidence. It does not own another rotating-token manager or a
business retry loop.

The common classifier must support provider-specific combinations of status,
vendor code/type, message, selected headers and request phase. Specified
conditions are conjunctive. Request-scoped rules take precedence over broad
authentication or outage handling: the committed donor's
`conductor_refresh.go:tryRefreshAfterUnauthorized` explicitly checks those
rules before rejecting a token or attempting refresh. Treating every 401 or
429 alike would lose that behavior.

The donor's `internal/ratelimit/throttle.go` also distinguishes rate from
concurrency and budget. Its vendor-specific 13xx rules are scoped to z.ai;
matching those digits inside another vendor's request ID would be wrong.
Exact machine-code rules precede generic prose. Vendor error envelopes and
stream trailers can describe a refusal even when HTTP is successful.

Current MiniMax device authorization is a continuation workflow. Media asset
visibility checks are bounded reads of an existing result. Neither grants
permission to resubmit generation. Core and tools authors are keeping these
separate while the shared request-policy contract is implemented.

The selected-tool expiry preflight now uses the same transactional owner on
ordinary MCP and Library paths. Its separately frozen correction also checks
terminal successful publication, including held responses revoked after
dispatch. Public tools, unknown expiry, holds and manual revocation retain
neighboring regressions. Bounded independent closure is recorded below;
the current combined shared-policy implementation still needs its own gate.

## Findings from the independent implementation probes

These findings are distinct from the donor inventory. The named artifacts
record actual ordinary-host or SDK-worker behavior on immutable source
captures; a later correction requires its own closure receipt.

| Finding | Observed behavior and owning boundary | Evidence and disposition |
| --- | --- | --- |
| Revoked unary result publication | A held tool returned successful MCP content after account Off, manual credential replacement, configuration change or module Off. Pre-dispatch and event checks did not cover the terminal result. Shared `Lease.Call` now rechecks selected authority before successful publication and completion. Accepted work remains uncertain after revocation. | Closed on host content `c4944cb7f6b7dce49c6ea039f1c3f3da514026c244741938cf8c85fcdffa5598`. The unchanged independent probe `c397ffaebaa7cc5fc330facfa649852c83a479142172bedac88de9a9b3f6edaf` passes all four original failing cases under race; all 10,955 source files remain unchanged. Original red: `/tmp/magpie-tool-publication-review-20261115`; bounded closure: `/tmp/magpie-tool-publication-closure-20261115/CLOSURE.md`. |
| Missing measurements authorizing another paid ping | Devin's successful paid task with absent usage or only an unknown metric scheduled a second paid task. Sparse known usage already stopped, but nil usage bypassed that predicate. Missing measurement cannot prove that another paid cache operation is permitted. | Closed by v2 archive `c83b9ad897242b7ea93cb40f50c12c2e2b6542890757a947a2f2325819f7a165`. The unchanged independent probe passes, including measured-hit continuation controls. Original red, causal overlay and bounded acceptance: `/tmp/magpie-devin-independent-20261115`. |
| Malformed codes becoming known refusals | MiniMax truncated fractional machine codes: `1008.5` became quota and `1000048.5` became authentication refusal. Malformed JSON could also acquire quota meaning from fallback text. A malformed envelope is unknown, rather than a source of a different valid code. | Closed on native v4 library `4ae1687285445c4570e1c08db7c6be229ad775969e57e3344f66cf0e2aedcc38` by the same four independent malformed-response probes plus valid authentication controls. Exact evidence and bounded acceptance: `packages/minimax/docs/history/host-e2e-20261115/INDEPENDENT-REVIEW.md`. |
| Terminal no-replay response losing the underlying refusal | The gateway correctly returns terminal 409 after accepted work, but overwrites the vendor status/error before request, route and ledger publication. Permission, quota and business errors then look identical in diagnostics. Preserve the no-replay boundary and record vendor facts separately from client status. | Ordinary-host traces and ledger under `usbox:/tmp/magpie-minimax-host-e2e.fhLVWFGt/production-v4`; shared host owner, not a MiniMax-specific status workaround. Open. |
| Capturing refusal facts after internal repair | The first observability candidate captures at the error writers. An accepted structured-format refusal is consumed before those writers, losing the facts; a body-size repair replaces an adapter 400 with a host 413, which is then misattributed. Both retain client 409, one paid handler execution, one ledger row and zero fallback. | Independently reproduced on owned archive `e19336d4fee268cc91ace25f81a089dd00cf2702da8bb61cfa52b3c42bf4a778`. Original actual-Front probes and causal controls: `/tmp/magpie-host-observability-independent-20261115/REVIEW.md`. A shared earlier response observer is under repair; no corrected-source approval yet. |

The first review run for the unary-publication probe failed during network
guard setup before Go execution. Its corrected run produced the four real
behavioral failures. These outcomes are not interchangeable. All probes use
synthetic credentials and fixture endpoints on regular usbox; none is live
vendor acceptance.

The observability review also hardened its privacy regression: JSON encoding a
usage record omits its observer-only body fields, so that assertion could not
detect leaked text. Direct observer-body assertions pass on the candidate and
fail when the privacy guards are removed. This is evidence for that bounded
privacy behavior, separate from the unresolved capture-placement defect.

## Quota windows, timing and account selection

The shared-policy requirements now include configurable counts and waits,
server-provided retry timing, and a choice between waiting, another eligible
account or a timed refusal. Classification in a callback is insufficient:
ordinary routing must honor the final refusal's scope, preserve the logical
request's budget across account changes, and retain accepted/uncertain fences.
These are implementation requirements under host ADR 0028, not newly accepted
runtime behavior.

Devin supplies a concrete regression. The pinned CLIProxyAPI donor's
`internal/runtime/executor/devin_journal_test.go` contains a free-model refusal
whose message says its limit resets in 42 seconds.
`devin_stream_status_345_test.go` contains a free-model refusal without a reset
time. The first has a usable provider-specific relative hint; the second
remains unknown. Minutes and hours belong in explicitly synthetic policy
tests until separately sourced examples are available.

The community donor's `packages/devin/free.test.mjs` identifies represented
free SWE variants through catalog cost tier 4 and excludes them from the
paid daily/weekly windows using `notModels`. Those paid epoch-second resets
cannot become free-model retry times. The inspected sources do not prove that
every free variant shares one rate bucket. Scope a refusal to the selected
account/model unless vendor evidence or explicit operator mapping supplies a
shared bucket.

The remaining migration also distinguishes transport recovery from existing
workflows. ZCode's immediate busy-response retries are candidates for the
shared scheduler; its financial holds and OffPeak ticket polling keep their
workflow owners. CommandCode's paused-turn continuation keeps the same turn
and account. MiniMax asset readiness rereads an existing result and must not
regenerate it. OMP declares session operations and needs no inference retry
adapter.

The source-only six-package inventory and 49 proposed source/synthetic cases
are retained at `/tmp/magpie-provider-policy-adapter-inventory.md` and
`/tmp/magpie-provider-policy-adapter-fixtures.json`. Fourteen proposed
two-account scenarios check actual sends, account/model state and timing.
None of those proposals is recorded as executed acceptance.
