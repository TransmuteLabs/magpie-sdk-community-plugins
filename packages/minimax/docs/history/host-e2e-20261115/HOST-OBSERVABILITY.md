# MM-HOST-OBS-1 — accepted-operation fence loses upstream refusal facts (P2)

Confirmed on the frozen source4 ordinary host, executable SHA-256
dfc5c0ddcaae39339ef5d2008056d0b63ad1b7821c6f87bf6af472913e2f1e79,
using unchanged MiniMax dist-v2 df7c26c7b76c8d47705f7f6d9b670d91cb1e25382c58a0498f5822b4dc4c832f.
This is a generic host observability finding, not a request to remove its
accepted-operation no-replay fence. No host changes were made by this lane.

The synthetic ordinary gateway cases mm-e2e-permission, mm-e2e-quota,
mm-e2e-chat-business-unknown and mm-e2e-chat-business-credit each dispatch
exactly once. Their module response classifications are 403, 429, 502 and
429 respectively; ordinary SDK CLI controls against the same native artifact
independently observe these distinct headers. The gateway correctly returns
409, X-Should-Retry:false, and an operation ID for every accepted failure.

However, each corresponding ordinary trace route and try contains only 409
and the generic accepted-operation message. Each usage row likewise contains
status 409 and err_type extension_operation_terminal, with no original
status or refusal classification. The permission and credit refusals become
indistinguishable in these views. A user cannot tell whether to inspect an
account permission or its credit balance from the recorded request failure.

The frozen host source explains this at internal/gateway/gateway.go:4456:
call.Status/call.Error are replaced by extensionOperationFailure before the
trace try is updated. The branch overwrites usage.ErrType and breaks before
ordinary failure handling. internal/gateway/extension_outcome.go:86 builds
the generic 409. The existing internal/extension/store.go:44 JournalEntry
contains scope, state and update time only; it has no refusal detail field.
The frozen public SDK has no ExecutionReference type. No claim is made about
the new shared engine currently under development or optional body archives.

Named evidence on regular usbox:
/tmp/magpie-minimax-host-e2e.fhLVWFGt/production-v4/trace.json,
ledger.json, commands.json, fixture.json and result.json. This run completed
20 functional cases but ended with an additional rejected plain GET / at the
local proxy fixture, so it is not recorded as an overall green test. The
trace and ledger above are direct captured observations, not inferred from
test exit status. Network guard denied all nonlocal attempts; no vendor was
contacted. Source4 was read-only. Report sent to the parent for ownership and
scoping; separate client status and original upstream facts should be retained
without changing the no-replay fence.
