# Bounded provider-domain continuation

2026-11-14. Accepted checkpoint `ffa65a5`; preserve the untracked
`AUTH-WEB-EVIDENCE.md` byte for byte. Write only this module, commit
locally using Lore, and keep the first plugin-free host rollout independent.
Runtime authority remains host `d07078c7926daf6f33af09c40297c941bd39da92`.
No live vendor/client install, new dependencies, host changes or new git
branches/worktrees.

Source authority:

- Vendor `fec975905a2abd4c07e35c40d51fa097a2258ec9`, desktop 3.18.2 /
  CLI 0.17.1: Start billing/model normalization, marketing-touch service
  and shared claim action schema.
- Community `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, package 0.1.61:
  `index.mjs`, `gift.test.mjs`, `dual.test.mjs`, `team-gift.test.mjs`,
  `checkin.test.mjs`, `inflight.test.mjs`, `wall.test.mjs`.
  This is a separate behavior lineage, not vendor desktop 3.18.2.

Port sequence, with compiled old-failing behavior fixtures before each:

1. `gift_test.go`, `discovery_start.go`, `discovery_models.go`, `gift.go`,
   `module.go`, `instance.go`, `usage.go`, `protocol/response.go`:
   grant-derived model discovery, strict Trial routes, free metadata,
   quota isolation and configured/default free-first. Preserve native
   ticket/card guards and no replay of uncertain or partially emitted
   inference. Only a confirmed non-executing refusal can authorize paid
   fallback; donor blanket fallback is not acceptable.
2. `billing_outcomes_test.go`, `billing.go`, `checkin.go`,
   `controller/store.go`, manifest actions: typed claim acknowledgments,
   explicit ineligible/captcha/rate outcomes, one feed request per check,
   exact identity, durable no-replay and bounded account holds.
   Preserve baseline reconciliation and usage barriers. Expose an
   explicit module check-in action; do not invent an automatic host
   scheduler or equate a module action with native scheduling.
3. `start_guard_test.go`, `start_guard.go`, `instance.go`, ordinary
   dispatch: FIFO per-account slots, cancellation, bounded known-busy
   retries, adaptive narrowing and shared outbound-route holds.
   Never retry transport uncertainty, partial output or unknown errors.
4. Update provenance/watch surfaces, parity matrix and immutable evidence
   for each coherent slice. Tests/builds run only on usbox with isolated
   HOME/XDG, existing offline caches, loopback namespace and timeouts.
   Run scoped RED/green/race and behavior-negative mutations, then module
   regression, static checks and the relevant one-bundle fixtures.

Actual client startup, rendered UI, custom Library/data roots, automatic
DB/WAL capture and financial/live resume remain separate acceptance work.
Two historical unexpected `/` billing requests remain unattributed.

After the domain proofs and Lore checkpoint `f6c708f`, fulfill the
explicit normal-full-package instruction:

5. Lock plain `build.py` output with an old-failing artifact assertion.
   In `build.py` and `cmd/manifest/main.go`, make the already-tested
   provider/controller/agent/UI composition the ordinary package.
   Keep the provider-only SDK 1.4 build under an explicit historical
   `--headless` compatibility option. Reuse the existing full SDK 1.5
   manifest and native entry; do not introduce a new runtime contract.
   Update runnable fixtures to inspect the ordinary full ZIP, preserve
   headless-only fixture declarations explicitly, then rerun artifact,
   ABI, native auth/UI/agent and static gates on usbox. Record the new
   default package inventory/artifact identity separately from the
   immutable domain checkpoint and commit locally with Lore.
