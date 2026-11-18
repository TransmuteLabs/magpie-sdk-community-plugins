# SDK 1.4 adoption

The scoped 1.4 contracts and independent host auth/usage/executor review
passed. The leader authorized this local module checkpoint after its
source-hash comparison, 2680 race events and actual native E2Es.
It depends on unreleased SDK 1.4 source; it is not a combined SDK release,
installed activation or remote publication.

Implemented behavior and regression plan:

- `usage.go`: echo canonical `UsageRequest.Revision` unchanged in
  `UsageResult.Revision`; fresh ordinary windows gain exact model
  applicability, ticket/card aliases are excluded, MCP metrics remain Aside.
- `reset.go`, `billing.go`: after the durable pending fence and accepted
  receipt, call selected-account `usage.invalidate` before any entitlement
  mutation. Failure to invalidate prevents vendor dispatch. Keeping the
  account dirty after an interrupted mutation is intentional; no late
  pre-mutation read may reopen routing. Vendor completion and module
  high-water bookkeeping remain separate facts; a lost reply or merely
  observed unrelated entitlement is never a confirmed mutation.
- Usage reads started before invalidation cannot acknowledge the new epoch.
  Cached AsOf/error/deferred observations remain informational, never a
  fresh same-revision acknowledgement. While the module journal records an
  unsettled accepted entitlement mutation, concurrent usage must remain
  deferred/error/AsOf/ordinary-unknown even when echoing the current revision.
  A fresh acknowledgment asserts that vendor data causally follows the
  settled mutation, rather than merely copying a revision string.
- `operations.go`: public `operation.cancel` uses a new `Scope.Operation`
  write intent; its `params.operationId`/reference still identifies the
  original execution target. Reuse the plugin's existing settlement fence.
- Keep old unknown outcomes and same-target settlement idempotent across
  generation/process changes. Never use credential rotation as cache
  invalidation.

No DataDir dependency is introduced. The explicit configured private
directory and pure startup continue working independently.

Required regressions: stale/cross-process usage read cannot reopen routing,
error/deferred cannot acknowledge, applicable model sets stay exact, and
durable cancel neither replays the target inference nor posts settlement
twice.
