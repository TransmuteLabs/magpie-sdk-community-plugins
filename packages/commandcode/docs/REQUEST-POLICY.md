# Shared request policy

CommandCode adopted `request-policy` in version `0.1.3`. Its runtime floor
remains public SDK API1.12/native ABI1. The adoption receipts apply to `0.1.3`.

`request_policy.go` extracts the original HTTP status, actual code/type/message,
and allowlisted timing headers before `refusalStatus` or an error codec can
normalize them. The bounds are 64KiB JSON, depth16, code256, type128,
message1024 bytes and four timing headers of at most256 bytes each. Duplicate
keys, invalid UTF8, malformed/wrong/null fields, truncation, excess size,
credential reflection and conflicting envelope fields do not establish proof.
Optional OpenAI `code:null` means absent code. Numeric codes stay their actual
JSON number spelling; request/message digits are never promoted into a code.
There are no invented vendor body delay/reset fields. Standard HTTP timing is
passed unchanged to the host and never turned into a module timer.

Only a complete initial 4xx error-only envelope with a recovered preflight
credit/plan/window message can become `RefusedExecution`. The donor's exact
message labels (`PREMIUM_CREDITS_EXHAUSTED:`, `MODEL_NOT_IN_PLAN:`,
`WINDOW_LIMIT:`) and credit/usage prose supply the provider meaning.
`payment` is the donor's observed error type. Unknown types/codes or extra
effect fields remain fenced. Extraction and that envelope/phase proof are
separate from the host's classification. No callback classification authorizes
the final response. Fresh facts/rules/evidence go on the final result.

These refusals apply to the selected account/model pair, including Jev's
original selected-account decision execution. A premium credit reset is not
proof about free allowance or every other model. The module supplies no
cross-model window or guessed persistent cooldown. Operator rules retain
priority over the bounded donor message rules. The host chooses retry,
eligible sibling, terminal disposition and timing in its existing smart/order
path; the module supplies no account selector, scheduling loop or timer.

Acceptance is delayed only while inspecting an initial error response. The
host journals the execution as dispatched before invoking the module
(`internal/extension/manager.go`); missing receipts, process loss and thrown
transport errors retain uncertainty. Success publishes accepted immediately
after its response headers, before reading/forwarding paid output. Every
unproved failure publishes accepted before preserving its prior forwarding
path. Interrupted bodies retain uncertainty. Initial proof cannot undo
acceptance, prior output, native session progress or usage. This is the reason
for changing the old pre-dispatch accepted event; it is not permission to
replay an unknown transport outcome.

The sole named transport repeat is `commandcode.subscription`: the GET needed
to establish the plan before ordinary inference. It uses the public
`RequestWithPolicy` helper with `read-only` evidence. Only a host permit can
repeat that GET. It never repeats inference or changes credentials. Reads from
models/usage/auth owners do not acquire an execution callback implicitly.
Jev does not perform this subscription preflight and keeps its own wire,
selected account, operation, usage and signing identity.

Intentionally not automatically recovered:

| Flow | Reason |
| --- | --- |
| Transport errors, incomplete/ambiguous/unknown errors, 5xx and redirects | No whole-execution rejection proof; vendor acceptance stays uncertain |
| Go-plan native events, even an early error before text | Absence of output is not proof of no vendor/session/tool/cache effects |
| `pause_turn` and native tool/cache continuation failures | Existing same-session continuation and accounting owner remains authoritative; no whole-turn restart |
| Paid successful HTTP/SSE, late error or published output | Accepted/output fences forbid a business replay |
| 401/auth refresh and sign-in | Existing transactional owner and `keyRefused` behavior remain authoritative; API1.12 supplies no credential recovery |
| Raw `RATE_LIMITED` or unknown window scope | No proved account bucket, egress identity or before-effect guarantee |
| Subscription/model/usage responses outside provider.execute | Public recovery callback authority is the original invocation, not arbitrary reads |

The plan precedes runtime edits in [REQUEST-POLICY-PLAN.md](REQUEST-POLICY-PLAN.md).
Original source/receipts and limits are recorded in `REQUEST-POLICY-VERIFICATION.json`
after guarded regular-usbox verification. [The current closure](LOCAL-CLOSURE-20261116.md)
adds locally authorized macOS strict-SDK/common-host checks and confirmed ZDR
adoption while preserving that history. Parent SDK fixes and Astra review
remain separate. No live vendor, account, installed app or Windows acceptance
is claimed.
