# ZCode 0.13: ordinary defaults, exact quota counts and retained observability

2026-11-14. Bounded continuation of module `5665859`, following the
parent's API9 field handoff. The assigned generic provider/GUI seams
remain in the shared uncommitted host tree for the parent to integrate.
This lane makes no host commit, broad host run, new branch/worktree,
dependency installation, publication or live-vendor call. SDK-only
deployment remains independent of module acceptance.

## One ordinary full package and exact identities

The default `build.py` produces **0.13.0 / API1.9 / native ABI1** with one
provider, controller, owned agent and static UI in one installation.
There is one native library and no community runtime or second module.
The explicit historical `--headless` build remains 0.7.1/API1.4.
Building either source variant now requires the API9 SDK types.

| Identity | Exact value |
| --- | --- |
| Ordinary package | `dist/zcode-0.13.0-api9-ae957/magpie-plugin_linux_amd64.zip` |
| ZIP SHA256 | `fac081f03c4275380de9196edebdbd1e1590e415e850fd1695affbc714145bf0` |
| ZIP inventory | 19 members; 9,816,700 bytes |
| Native library SHA256 | `2d0c84d931b29694a95138660cdbf9f5f7ebbf9819288731c87a9c5c12f39f8a` |
| Host binary SHA256 | `5ff7e035fc898c9c5fcdd5485caad2a6eddcc3b3b9a8a87ddeb9014b58756f24` |
| Shared SDK snapshot base commit | `ae957e5a8a77553a6cf6f484c8b58a69393f3887` |
| SDK content/mode snapshot SHA256 | `fe82cca0a913ea8a4307478050e241747c555524353221fdab81d43458f8ce16` |
| SDK source archive SHA256 | `efcd0cdb05b61399e818641b570479bccabbfc422bde5c5f8d442ece54bcd57c` |
| Final module source inventory | 183 files, matched locally and on usbox |
| Module inventory SHA256 | `24827d1774beb532dcd70810eeaaf88b73905d72647e1411a6413e5f5e52cb08` |
| Module source archive SHA256 | `278ba28d79b5aaf4d7acfedfc75808fa866fb4d4fac82fab353c4d89ef78b4fd` |
| Aggregated acceptance SHA256 | `5cde35c66f6119e004a3d093ab3304644ae4d5d18cd3a26828ca371a8f772419` |
| Preserved evidence archive SHA256 | `7937549777ac4c93bf74226fceeec097a18f42c2d62e44c2abb188519c5aae08` |

This SDK identity describes **10702 immutable source files in an
uncommitted union snapshot**, not a committed Git tree or final SDK
release. `source-lock.json` keeps this distinction and separately pins
19 relevant SDK files. The snapshot deliberately retains the independently
reported AccountStamp defect described below; the proposed fix is tested
only as an explicit overlay.

The local root is `dist/api9-lazx_9cj/`; remote sources/logs remain under
`usbox:/tmp/magpie-api9-zcode-OkPR9GHA`. `green-01/` contains the accepted
SDK/module source, actual host, full/headless artifacts and final receipts.
`module-final-source-inventory.json` and `module-final-source.tar.gz`
freeze all pre-evidence module source bytes and modes. This narrative is
the sole post-acceptance document excluded from that inventory.

`collect-handoff.py` independently checks log hashes, Go event counts,
isolation, source inventories, package members, native owner results and
the installed receipt. `evidence-final/green-01/api9-acceptance.json`
contains its aggregate and hashes of **120 preserved files**. The checked
evidence archive contains the corresponding reports/logs and inventories.
The earlier source archives, failed runs and 0.12 package are retained.

## Implemented scope and ordinary owners

The provider descriptor supplies optional `maxConcurrency`/`maxAccounts`
defaults; omitted values preserve existing host behavior. ZCode declares
the donor's **3 requests per account / 2 active accounts**. Existing user
account/provider settings win, including explicit zero. The adapter uses
the ordinary queue and account-selection owners, validates the current
package/descriptor and refuses stale account/config/package bindings.
API9 static providers participate in configuration revision fencing too;
queued preparation rechecks the revision before dispatch. There is no
second host semaphore or community scheduler.

Rich quota validation/projection preserves actual optional amount,
balance, display, unit and span. Exact zero and absent amount remain
distinct through native JSON and the ordinary quota view. Missing amount
is not formatted as zero. Existing Used/Unknown/Aside, model selectors,
observation/reset times and exhausted-window rules remain authoritative.
Balance metadata grants no wallet-routing authority and unit/display never
become execution currency, credits or measured cost.

The module ports source-backed paid and gift counts/display and quota
periods, including the donor's omitted team count meaning five hours and
Start entitlement periods or a bounded period range. It never invents a
textual unit, balance or monetary label. API4 compatibility omits all API9
fields and retains the historical percentage limit.

Public fields/cloning/manifest packaging and shared TS declarations were
integrated by the parent. Exact ownership, paths and the historical
additive packet are in API9-PARENT-HANDOFF.md. Do not reapply
`testdata/api9/parent-public-fields.patch`. `app.js` and the provider design
reference also contain parent edits; their whole diff is not attributed
to this lane.

The API7 observability implementation is retained: sparse exact-zero
token/cache/cache1h/reasoning facts, original served model, request and
response IDs, stop/tier, and final failed-stream snapshots. Observation
precedes Trial model/error rewriting. Finite code-only diagnostics use
the host's scoped budgeted callback, without bodies, vendor text, auth
or caller-supplied identities. Consent, collection, retention and export
are not expanded.

## Scoped usbox acceptance

All builds and runtime tests used Go1.26.8, existing offline caches,
isolated HOME/XDG, finite timeouts and a loopback-only network namespace.
UID/EUID were 1000 and all five capability masks zero. Native root
ownership checks and forged-header refusal remain intact.

| Gate | Result and receipt |
| --- | --- |
| SDK validators, race20 | **100 pass**, zero fail/skip; `green-owners-static-config-race20-result.json` |
| Provider defaults/quota owners, race20 | **180 pass**, zero fail/skip; same receipt |
| Actual Front installed-worker owners, race20 | **140 pass**, zero fail/skip; `green-front-race20-result.json` |
| Node/Bun public validators and strict TS | **4/4** each; positive declaration fixtures compile |
| Actual GUI formatter/pace source in Node VM | **2 pass**; `green-presentation-result.json`; no DOM claim |
| Scoped host vet and GUI guards | Exit0; `green-static-result.json` |
| Module formatting/vet/Python syntax | Exit0; formatting output independently empty |
| Whole module, race once | **415 pass**, zero fail/skip; `api9-final-unit-result.json` |
| Observability, race20 | **460 pass**, zero fail/skip; same receipt |
| Ordinary ZIP, strict TS/bundles, full/headless native ABI | Exit0; `api9-final-package-result.json` |
| Packaged Node/Bun UI | **31/31** each, zero failures |
| Actual installed host | **11 executions / 11 scoped execute diagnostics**; `api9-installed-02-result.json` |
| Five native owner suites | Exit0; first two results in `api9-native-owners-result.json`, all three in `api9-native-owners-retry03-result.json` |
| Native SQLite rollback | **120 pass / 60 fresh processes**, zero fail/skip; `api9-native-sqlite-sqlite-owner/result.json` |
| Source provenance | **35 vendor + 6 community + 19 SDK hashes**; `api9-final-provenance-result.json` |

Front covers held requests through body EOF, cancellation release, two
active accounts with a third reserved, user zero for concurrency/fanout,
and queued off/config/account/package revocation. Adjacent historical
quota/queue coverage is separately conditional on the parent stamp fix.

The canonical unmodified native package passes add/list/inspect/
sdk.describe/off. Actual inference uses a destination-only loopback
fixture library, SHA256
`83d403c49c81271e010e996516f576d9cbe1a15789fe6ef71c39afa17dca6d8e`.
Each of 11 executions produces one ordinary ledger row. Complete,
truncated, error and rejected operations retain the correct known usage;
measured final zero supersedes earlier counts. USD/EUR/CNY/credits-shaped
lookalikes remain unknown normalized monetary facts.

The ordinary `/api/usage/quotas` response carries amount 0, limit 10.25,
display `0 / 10.25`, reset `2033-05-18T03:33:20Z` and derived start
`2033-05-17T22:33:20Z`, preserving the five-hour period. No unit, balance,
cost, credits or currency is invented. This is actual host/card data
projection, not rendered-browser or actual-vendor evidence.

Native lifecycle includes strict forwarded forged-header refusal after
PassHeaders deliberately allows those headers to reach admission.
Controls retain selection/generation and non-replay proofs. Controller
acceptance includes remote MCP session/JWT ownership and offline history.
SQLite exercises cancellation after write, foreign rollback-time bytes
and stale preimages in 20 fresh processes each, preserving exact bytes or
the unresolved journal and refusing unsafe rollback.

## Behavior-negative controls and parent gate

The old committed module `5665859` with the final quota tests compiles
and reaches **7 assertion failures**; log SHA256
`4055c7492e577a2b12fa543997d6a8de906e78d10c566f63e14b260e66179e18`.
The old host ae957 with the final Front fixture reaches **2 assertion
failures**: the fourth request bypasses the declared default and the
third account bypasses reserve. Log SHA256
`feced71c8557024f4f302f564f8f668567913430cdabbf74e3707560e5a6899e`.
Old SDK/provider behavior also reaches 5/8 assertion failures and the
actual formatter fails when missing amount becomes zero.

Six compiled generic causal overlays each exit1 at the intended owner
assertions: remove defaults, static configuration fencing, queued
configuration fencing, amount projection, absence preservation, or
informational wallet protection. Receipt SHA256
`3373d9d77dbbe0d59fab4470eb13e1acdc81c17526420015bbc20f99caa4e21f`.
Five fresh observability causal overlays also exit1 against this API9
snapshot: remove Facts, final-zero replacement, diagnostic callback,
incomplete-body header observation, or observation before rewriting.
Their failing-test event counts are 1/2/3/1/5, with no build errors;
`api9-final-causal-result.json` preserves the complete receipts.

The adjacent scope uncovered a real AccountStamp defect:
configuration reads create a private empty namespace, and
`accountStampAt` hashes its name without an `accounts.json`. An ordinary
Hold then sees an invented account change. The adapter's configuration
fence is retained. The parent owns the stamp file; the minimal proposal
skips absent account files before hashing namespace names while retaining
all directory, symlink, identity and unreadable-state checks.

The unchanged independent probe is
`testdata/api9/account_stamp_probe_test.go.fixture`, SHA256
`7b5379a8bd4f0f5e11d53f32976156eb2218c5175470bbe9ae67f50dac7464cf`.
It reaches one real RED. The proposed parent overlay passes
**160 AccountStamp + 20 unchanged catalog Hold tests under race20**;
the adjacent scoped provider/gateway suite passes **240 + 12**, zero
fail/skip/race. These are **conditional proposed-fix proofs**.
Integrated parent closure and final committed-host acceptance remain
pending. The exact hunk and receipts are in
`testdata/api9/ACCOUNT-STAMP-PARENT-HANDOFF.md`.

## Preserved failures and limits

The initial provider RED fixture referenced the wrong Allowance fields;
the first isolation attempt inherited BUN_INSTALL and was refused.
The initial Front fixture declared AccountAuth without its mandatory
handlers and failed its handshake. Those infrastructure failures are
preserved; only the corrected, compiled assertion runs count as RED.
An initial config-removal causal attempt passed because its fixture still
declared ModelDiscovery. The final static-provider fixture and causal
removal reach the intended failure.

The first installed run completed operations but its report writer
expected a Git tree for an uncommitted snapshot. The corrected report
records the content/mode snapshot and adds the ordinary quota-card proof.
Native controls and UI-auth fixtures initially retained API7 version
assertions. Only those exact expected-version/provenance checks and the
reported floor were advanced to API9; final owners pass with admission
and non-replay guarantees unchanged. Earlier failures remain in the
evidence archive and are not a green aggregate.

Source diff whitespace checks pass with the historical
`parent-public-fields.patch` excluded: its literal unified-diff context
retains original blank lines and space-before-tab prefixes. That review
packet remains unchanged and is not an instruction to reapply it.

Desktop **3.18.2 / CLI0.17.1** are source-verified at
`fec975905a2abd4c07e35c40d51fa097a2258ec9`, community **0.1.61** at
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`. These versions and checked
files remain in the versioned source lock. Actual ZCode client/vendor,
native Darwin/Windows runtime and rendered Chromium/WebKit acceptance
have not run. Browser binaries are absent; none were downloaded.

Automatic consistent DB/WAL capture, financial import/resume, actual
client-bound native operation wire/resume, custom-root Library placement,
automatic check-in adoption and adaptive Start hold/cap card hints remain
separate module work. The new generic account-task contract is not adopted
by this package. The Start guard remains per instance.

Per-execution credits/cost/currency/deferred/upstream authority remains
absent in the pinned Messages source; quota metadata supplies none.
The historical two unexpected `/` billing requests remain unattributed;
isolated GREEN is not a causal fix. Budget/masking/settings/keepalive
census is not expanded.

Historical catalog closure remains in `4a71bd5` and
PARENT-CATALOG-REVIEW.md: 480 passes including the unchanged independent
probe. AUTH-WEB-EVIDENCE.md is unchanged, SHA256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
The earlier 0.12 ordinary ZIP remains unchanged at SHA256
`0cfd2087dfd276f60d1f482a984cb8745f145af3ffcb0e29356f25d90b949433`.
