# Retained facts and the public contract

Public API 1.12 does not serialize the complete internal `FailureFacts` struct.
That alone is not a routing defect. The supported final chat refusal preserves
all facts needed for its current classification, timing and routing decision.
Other fields are validation inputs, local normalization or optional diagnostic
context. Neither a raw response body nor credentials belong in this DTO.

| Internal field | Public representation and actual limit | Consequence |
| --- | --- | --- |
| `Status`, `WireStatus` | `facts.status` and result status preserve the original HTTP status. `Status` is a legacy alias, not another observed status. | No routing loss; HTTP200 stays 200. |
| `EffectiveStatus` | No separate field. It is the package normalizer's derived status; the host derives its routing status from classification. | Intentionally not routing authority. Code1008/quota maps to route429 without replacing wire200. Logging both derived statuses would be optional diagnostics. |
| `Code`, `Type`, `Message` | Bounded selected-secret-redacted strings. | Represented for supported complete refusals. No body serialization is needed. |
| `CodePresent`, `CodeValid` | No independent flags. A valid code is represented as a string; malformed present codes fail the local complete-refusal gate. | No routing gap in the admitted subset. Absent-versus-malformed code is a safe diagnostic distinction on excluded paths, not evidence for replay. |
| `Timing.DelayMS` | `retryAfterMs` pointer preserves a valid explicit zero and positive duration up to 30 days. | Represented. A longer valid actual Retry-After remains in the real header; no shorter typed delay is invented. |
| `Timing.ResetAt` | `resetAt` carries the validated UTC instant. | Represented. |
| `Timing.State` | Delay/reset is represented by typed hints and the actual validated header. Absent and invalid both have no valid public timing. | No routing distinction: both mean unknown reset; neither proves an immediate reset. Distinguishing absent/invalid is a diagnostic gap only. |
| `Timing.Source` | No separate field; this extractor uses only the actual Retry-After header, whose name is preserved for valid values. | Local extraction provenance; no need for a new scheduling authority. |
| `Timing.Raw`, `RetryAfter` | Only validated, redacted actual Retry-After is projected. Duplicate or malformed raw values are not copied. | Intentionally private invalid input. Safe future diagnostics could use a reason enum, not arbitrary headers. |
| `Origin` | No exact internal origin enum; `step`, `operation` and `phase` identify the admitted public observation. | Internal HTTP/business/SSE/connector provenance validates which facts may be projected. An origin enum could help diagnostics but would not prove effects. |
| `RequestID` | No dedicated correlation field in the refusal DTO. Host operation/account/generation already identify the selected call; ordinary response wire diagnostics have their own owner. | Optional upstream correlation gap, not routing input. Do not put an unchecked vendor identifier or secret in host policy logs. |
| `TrustedEndpoint` | No boolean supplied to the host. Exact selected endpoint/redirect validation remains the adapter's obligation. | Intentionally private proof input; a module-supplied trust flag would not add authority. |
| `BodyComplete` | Public `complete` is true only after bounded complete parsing and stricter error-only validation. | Represented for admitted facts. False alone never authorizes replay. |
| `OutputStarted` | No advisory field; the host validates actual accepted/output publication and durable effect state independently. The adapter also excludes it before final refusal. | Intentionally private local input plus a stronger host-owned fence. A DTO flag must not override published output. |

There is no demonstrated required routing/knownness field missing for the
implemented complete final chat refusal. In particular, zero is not lost,
HTTP200 is not rewritten, malformed code does not become absent, and an unknown
reset does not become a guessed cooldown. Optional diagnostic losses are code
validity/presence on excluded observations, timing invalid-versus-absent,
detailed extraction origin and upstream request correlation. Their absence must
not be presented as a reason to add credentials, body text, arbitrary headers
or a second local routing classifier.

For the stronger requirement that the host itself retain full extraction
knownness, there is a real diagnostic contract gap: it cannot distinguish
absent from malformed machine code, absent from invalid timing, or the exact
extraction origin. Those bounded states are
required to claim complete host-side facts diagnostics and are not currently
preserved by the DTO. Basic policy decision logging still has status,
class/scope/rule, selected operation/account/generation and a known reset.
Upstream request-ID correlation is optional; raw malformed header values,
endpoint authority flags, body bytes and credentials are not required.
The package does not add undeclared fields or encode these states in message,
code or fabricated headers. A future typed diagnostic carrier belongs to the
public SDK/host owner. This diagnostic gap does not justify changing routing
or relaxing a completeness/effect check.

## Search and tool observations

The terminal public `Error`, `SearchResult` and tool result shapes do not carry
`ExecuteResult.refusal`. That is a limit on final result serialization and on
whole-provider fallback authority. It does not make all shared policy use by
search/tools impossible.

The public negotiated `request.recover` callback is available within the
original selected `provider.execute`, `tool.call` or `module.call` invocation.
MiniMax search actions are declared read actions and run as `module.call`.
A separately named, actually read-only HTTP step can use `DecideRequest` or
`RequestWithPolicy` with bounded, redacted extracted facts and `read-only`
evidence. Classification can also be requested without recovery evidence.
Any repeat needs a host permit, keeps the selected account and model, and
repeats only that request. Redirect forwarding must be disabled for the public
helper; the existing general site-request function is not silently wrapped.

The exact web-search and connector-catalog endpoints now have named
observation/recovery boundaries in `src/read_policy.go`. Original wire status,
bounded redacted code/type/message and validated timing reach the callback.
Successful bodies and terminal adapter facts retain their existing owners.
A separately proved download would need another named boundary before it
could use this integration. A connector's generic tool call cannot assume
read-only behavior from its name. Accepted media generation, upload, a write
action or a completed
earlier paid step cannot acquire business replay permission from those reads.
Neither a callback diagnostic nor a search/tool result can become the final
provider account-fallback carrier.

This delta implements final chat projection and the two exact named reads.
It does not extend final-refusal serialization to search/tools. The parent's
common model-budget, accepted-replay and effect checks retain their own source
and acceptance; this package does not compensate for them.
`READ-POLICY.md` records current source, ordinary-host evidence and the
remaining public-helper and host observability findings.
