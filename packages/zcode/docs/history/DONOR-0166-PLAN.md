# Bounded comparison of community ZCode 0.1.63–0.1.66

2026-11-14. Start from our committed 0.13 package `3c0f487`. Read donor
objects with git show/diff only; the donor checkout remains
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`. Its observed origin/HEAD is
`a60667b57040f62080da60f6018cc7351fa1953f`, whose ZCode subtree is identical
to the bounded 0.1.66 target below. No host feature, dependency, live
credential/client, branch/worktree or publication belongs to this slice.

| Exact donor commit | Version | Comparison and extraction files |
| --- | --- | --- |
| `3f37b858db8074959447c16ed06ac51d444c7fcf` | 0.1.63 | `ownSignIn` and own.test: active CN identity must not follow another user with the same display name. Our explicit selected-account auth never reads app profile caches; add same-name CN renewal oracle in donor0166_test.go before changing any auth code. |
| `8e19b6a0e712e4ea844ccd5e3fe95522f570ae05` | 0.1.64 | Start narrowing currently returns the refused request. Add old-failing real in-flight/queue fixtures, then port bounded front requeue into start_guard.go; preserve body-held slots, cancellation, one wait budget, four requeues and no replay of partial output. |
| `1dbbc1fb489d7ac40b0620d802301075e25332b6` | 0.1.65 | A missing Trial quota lacks the weekend-plan explanation. Add feed/no-feed/failure/expiry/isolation oracles, then a bounded read-only hint cache in gift.go/instance.go using the existing marketing reader. No claim, captcha bypass, model dispatch or entitlement is created by a hint. |
| `87a8cf03c62fa55ca845c51648e8859ece580336` | 0.1.66 | Discovery already requires current paid/gift authority and fails closed on failed plan reads. Prove no paid dispatch after absent→failed plan reads. Manual Start currently bypasses balance applicability: add an old-failing exclusion fixture, then require current gift authority in gift.go for that path too. |

Use fresh synthetic objects rather than real donor account captures.
Materialize the donor's own selected tests from the exact Git object into
an isolated usbox source directory. Run its own/gift/inflight oracles and
causal removals without loading the community runtime into our package.
Run module assertions against unchanged `3c0f487` first; preserve compiled
RED separately from all fixture/infrastructure failures.

The module keeps its stricter financial policy: unknown/malformed balance
or plan reads do not authorize a billed request, Trial never spends paid
quota, and output/transport uncertainty is never a requeue trigger.
Do not copy the donor's permissive unknown-balance send or failed-plan
paid fallback. Existing explicitly selected paid imports remain distinct
from automatic discovery; confirmed gift-only selection cannot become paid.

The first green freeze exposed one additional entry-path gap during
review: Discovery Start/inactive-paid accounts with no gift returned from
catalog/paid admission before reaching the new Trial explanation. Preserve
that green freeze and compile a second RED first, then make explicit Trial
selection use its own current gift authority in module.go. This removes
the irrelevant paid/catalog prerequisite without admitting any paid
dispatch or changing ordinary Discovery financial fences.

After ports, update versioned source-lock/SOURCES/watch entries with exact
commit/file hashes while retaining the earlier 0.1.61 lineage. Verify
source bytes, scoped race/static/causal and ordinary full packaging on
usbox against the existing immutable API9 SDK snapshot. Keep the accepted
0.13 ZIP and previous evidence unchanged. Final committed-host, actual
client/vendor, native Mac/Windows, financial resume and DB/WAL gates stay
separate. Local module Lore commits only; no host commit or broad suite.
