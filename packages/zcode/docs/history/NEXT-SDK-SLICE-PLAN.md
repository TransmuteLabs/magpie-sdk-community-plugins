# Pending source plan: concurrency defaults and rich quota windows

This historical planning checkpoint was released by the user after
52a8d85/ae957e5. The bounded API9 implementation, public field ownership
and actual acceptance limits are recorded in API9-EVIDENCE.md and
API9-PARENT-HANDOFF.md. The original proposal below remains historical.

2026-11-14. Planning only, after the ZCode 0.12 observability commit
`5f36e8a`. This does not release implementation work, change the frozen
host, add a manifest field or raise a capability floor. Start only after
the parent completes its union/upstream merge, designates an exact
committed SDK base and releases this bounded slice.

## Source evidence and present need

The read-only community donor is
`../magpie-community-plugins` at
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, ZCode 0.1.61.
The following identities are already recorded in COMMUNITY-PARITY.md:

| Source | SHA256 | Behavior to preserve |
| --- | --- | --- |
| `packages/zcode/package.json` | `c3ba49c27df561cec86def92fabccb1b48e2ab93a9401694e5bf54b728c57bc2` | `magpie.maxConcurrency: 3`, `magpie.maxAccounts: 2` |
| `packages/zcode/index.mjs` | `b29c486235227c339807755872c020af0b8df61a26812fa160230d74778ef277` | Auth-hook defaults; `limitWindows`, `giftOf`, `pendingLines`; Start gate remains a distinct domain guard |
| `packages/zcode/README.md` | `360146b1f5a2720d70b39689f57ca2d4edfb2506f99d4fce5939a16d0d1a233f` | User overrides; paid-plan defaults are not a fixed module cap |
| `packages/zcode/inflight.test.mjs` | `a734815f0319f5f8899cd19a5ea2a0d5e8813a57de150a6258bcb7ea72a6c6b9` | Per-account Start queue, cancellation and held slot across retries/body consumption |
| `packages/zcode/gift.test.mjs` | `1f66df99849f06e44b41a30781a33cba43ae95381cedc3fb3d7bc859b52df184` | Gift quota authority and source amounts |
| `packages/zcode/pending.test.mjs` | `dd9cd0adbae0809625c8e7f1ca31b55350b14bdb5242e79f175ca7bf7a756c36` | Future plans are informational and do not grant current routing allowance |

The ZCode module already has the Start-specific FIFO cap of three,
adaptive narrowing and route holds in `start_guard.go`. This does not
declare generic host defaults for ordinary paid accounts or the number
of simultaneously active provider accounts. The donor declares default
three per account and two active accounts; its Start gate remains useful
when a user increases the ordinary host default.

For quota cards, the donor reports exact amount/limit/display and window
span when the source supplies them. It also reports informational pending
plan/sign-in/claim text. `usage.go` and `discovery_start.go` preserve honest
Used/Unknown/Aside and model applicability, but cannot carry that complete
display metadata through the checked public SDK. They must not encode
amounts into Used or reinterpret quota amounts as execution credits/cost.

Desktop 3.18.2 / CLI 0.17.1 remain separately pinned at
`fec975905a2abd4c07e35c40d51fa097a2258ec9`. The community README's
3.17.2 runtime explanation does not establish actual 3.18.2 execution or
authorize a new paid-tier cap.

## Public contract ownership proposal for the parent

The parent owns public names, types, feature gating and schema compatibility
in `pkg/extension/contract.go`, `pkg/extension/types.go`, manifest cloning/
validation and the owning design reference. No parallel definition of
those fields will be introduced by this module. The parent also retains
`internal/provider/extension_catalog_projection.go` and its catalog tests.

The minimal candidate contract is:

| Public surface | Candidate semantics for parent review |
| --- | --- |
| Provider defaults, analogous to `MaxConcurrency` / `MaxAccounts` | Optional defaults for the host's existing queue/account selection owners. An omitted default preserves current behavior. Explicit user settings, including a supported unlimited setting, win. Values do not become hard vendor caps or module-managed host queues. |
| Quota `Amount`, existing `Limit`, `Unit`, `Display`, `Span` metadata | Optional finite source amounts, bounded display/unit text and a defined span unit, projected into existing native QuotaWindow fields. Preserve absent versus exact zero where meaningful. Agree the existing Limit compatibility semantics before reusing it as source capacity. |
| Existing Used/Unknown/Aside/model/time fields | Remain the routing authority. Amount/display text alone cannot make unknown allowance known, unblock a spent bucket or activate a future plan. |

These names are a coordination proposal, not a published API. Reuse the
native fields and settings precedence rather than introducing another
budget, limiter, quota owner or middleware activation path. Unit is
reported only when the source supplies an authoritative unit. Do not
interpret quota display currency as `ExecutionUsageFacts.Cost`.

At the checked 43b7 snapshot, ProviderDescriptor has no concurrency
defaults and UsageWindow has no amount/display/unit/span projection.
Before implementation, recheck the merged base for fields or adapters
the parent already supplied and record the resulting ownership handoff.
Keep this slice separate from parent catalog projection ownership.

## File-level implementation sequence after release

1. Pin the parent-designated committed SDK and donor files. Extend
   `source-lock.json` with checked source identities before porting.
   Read `docs/subsystems/extension-providers.md` and its linked owners.
2. Add behavior fixtures that fail on that unchanged base. Generic
   contract/serialization fixtures belong with the public SDK owner;
   ordinary provider/queue/quota fixtures belong at the adapter boundary.
   Save their compiled assertion RED and original source hashes.
3. Following the parent's public-field handoff, carry metadata through
   the existing extension provider registration and quota adapter in
   `internal/provider/extension_provider.go`. Reuse the host's existing
   defaults and native quota representation; do not edit parent catalog
   projection or the gateway's concurrency algorithm.
4. Port only the bounded source data in this module's `module.go`,
   `usage.go` and `discovery_start.go`, preserving `start_guard.go`
   behavior. Extend `usage_test.go`, `discovery_start_test.go`,
   `gift_test.go`, `start_guard_test.go` and a scoped ordinary
   installed-module fixture where the port requires it.
5. Verify against the exact new committed base on usbox with isolated
   HOME/offline caches/loopback and finite timeouts. Record causal
   negatives, actual artifacts and Lore decisions before declaring the
   slice complete.

The old-failing and causal fixtures must cover:

- Cold ordinary module registration receives defaults 3/2; account and
  provider user overrides survive refresh/restart; absent defaults and
  supported explicit unlimited settings remain distinguishable.
- Actual concurrent requests exercise the existing queue and active
  account selection, release on cancellation/body completion and keep
  another provider's settings independent. The Start gate still limits
  Start when a user raises the paid-account setting.
- A source count of zero is distinct from absence. Known amount/limit and
  span/display survive SDK serialization and ordinary native quota views.
  Percentage-only readings retain their present semantics.
- Unknown or invalid amounts, zero-cap informational windows, exhausted
  buckets and future plans never acquire routing allowance from display
  fields. Model/Trial selectors, observation times and reset times survive.
- Removing a defaults handoff or a metadata projection makes the ordinary
  owner assertion fail; changing a fixture alone is not closure.

No implementation or new acceptance is claimed by this plan. Budget/
masking/settings/keepalive expansion, financial import/resume, automatic
DB/WAL or log collection, actual Mac/client execution and live vendor
tests remain outside this slice.
