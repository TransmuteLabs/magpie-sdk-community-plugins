# ZCode 0.12: source-backed execution facts and scoped diagnostics

2026-11-14. Module-only continuation of `4a71bd5`, `96e30b8` and
`fc839ee`. No parent, other module, dependency, branch, worktree, live
account or installed client was changed. No push or publication occurred.
The SDK-only deployment does not wait for this module.

## Ordinary full package and exact source identity

The ordinary **0.12.0** ZIP includes provider, controller, owned agent
and static UI together, with **API 1.7 / native ABI 1**. It contains one
native library, one provider and one agent. It needs no community runtime,
second module or alternative agent/window installation. The explicit
historical headless build remains 0.7.1/API1.4 with legacy counters;
compiling these sources requires the API7 SDK types.

| Identity | Exact value |
| --- | --- |
| Full package | `dist/zcode-0.12.0-api7-43b7/magpie-plugin_linux_amd64.zip` |
| ZIP SHA256 | `0cfd2087dfd276f60d1f482a984cb8745f145af3ffcb0e29356f25d90b949433` |
| Native library SHA256 | `4610989d862b83a7174152405e3440c6bda938c140496b8a240893e30a2bf857` |
| Package inventory | 19 members; 9,711,323 ZIP bytes |
| Tested SDK source tree | `43b7e290d9c3370881780b23ad49222f5d27f44b` |
| SDK snapshot base commit | `9e88f10982a1d66c1b402fba27caddda8ac4ac4e` |
| SDK source archive SHA256 | `dd5bf036acdbb39bf1d93a415816b4ccea5700c40c49f2f430eea3dacd411454` |
| Host binary SHA256 | `ecfd82059e962ddf153f497977849ba3125ab52743771b862a0c09b28fe754ec` |
| Module source inventory | 174 files, content and modes matched locally and on usbox |
| Module inventory SHA256 | `54384a7dae27daf0026733de123a01608a52e812be1c965fbc71e22adca5c3a8` |
| Module source archive SHA256 | `a1aab3cee9a50d734d7dd1f5cc73f464787dc9449606477e5b323f0ddff4e598` |
| Aggregated evidence SHA256 | `df6bd51ccf26b4efe7fe010f7f8c9250b8048f7edb26ebfe8c6729cda1b86b08` |

This is the owner's immutable **uncommitted API8 source snapshot**, not a
committed SDK release. The module consumes public API7 Facts and
diagnostics, without adopting API8 StorageSource. Acceptance against the
final consolidated committed host, including the requested upstream
`07a62b3`, remains separate. No parent catalog file was edited.

The local evidence root is `dist/observability7-01_ze11w/`; the remote
root is `usbox:/tmp/magpie-zcode-observability7-rQFCPRwz`.
`module-freeze-inventory.json` records all 174 pre-evidence source hashes
and modes. `module-freeze-source.tar.gz` preserves those bytes.
`freeze-evidence/freeze-source-audit.json` confirms those module files and
all **10557** SDK source files unchanged after the final package and
installed-host runs. This narrative and the subsequent source-only SDK
plan are post-acceptance documents, excluded from that tested inventory.

`observability7-acceptance.json` contains the checked reports, package
member hashes, installed-host receipt, provenance and hashes of **61**
preserved logs. `collect-freeze.py` records the independent aggregation
and rejects mismatched logs, counts, source bytes or package identity.
The preceding 0.11 package at `dist/zcode-0.11.0-devzeus9e88/` is retained
unchanged. Earlier 0.12 ZIPs, logs and source archives remain separate.

## What the module observes

`response_usage.go` carries sparse pointer Facts for uncached input,
output, cache reads/writes, one-hour cache-write subsets, reasoning,
original served model, response/request IDs, stop reason and service
tier. Explicit zero replaces an earlier count. Missing fields stay
unknown. Each event/result owns a deep clone; consecutive executions
share no snapshot pointers.

Both complete cache TTL buckets are required to derive a missing total.
A fresh aggregate cache-write count invalidates an older one-hour subset
unless that response also reports the subset. Invalid, negative,
fractional or over-safe-integer counts never become measured zero.
One-hour cache writes and reasoning are subsets, not extra token totals.

Observation precedes Trial response-model rewriting and error
sanitization. Header request IDs survive incomplete JSON bodies;
X-Request-Id takes precedence over Request-Id. An ambiguous error `id`
never replaces a known request ID or becomes a response ID. Failed SSE
retains the last actual event snapshot and remains uncertain.

The pinned Messages transport supplies no authoritative per-execution
credits, cost/currency, deferred count or upstream-provider fact.
Quota balances and client-local monetary history grant no such authority.
Unsupported USD/EUR/CNY/credits-shaped usage remains raw response data,
without normalized cost, credit or currency claims.

`observability.go` wraps the finite execution, models, usage, auth,
operation, tool and named action handlers with terminal code-only
diagnostics. Measured millis and actual execution status are bounded;
there is no body, vendor error text, URL, credential or caller-supplied
scope. The host derives account/module/operation identity and enforces
its 60-per-module-per-minute budget. Callback errors or budget refusal
do not change the operation result. The callback has a 250ms child
timeout; a canceled parent starts no late diagnostic. No ad-hoc stderr
collection, retention, export or consent expansion is introduced.

## Usbox acceptance and behavior-negative proof

All product builds/tests ran on usbox with Go **1.26.8**, pinned offline
caches, isolated HOME/XDG, finite timeouts and a loopback-only network
namespace. UID/EUID were **1000**, all five capability masks zero.
No broad parent `./...` suite ran.

| Gate | Result and preserved receipt |
| --- | --- |
| Module gofmt/vet/Python syntax | Pass; formatting log independently checked empty |
| Whole module, race once | **408 pass**, zero fail/skip; `accepted-unit-result.json` |
| Observability, race20 | **460 pass**, zero fail/skip; same receipt |
| Final ordinary ZIP, strict TS, full and historical headless native ABI | Pass; two native sessions each, scoped diagnostics 3/full session and 0/headless session; `freeze-package-result.json` |
| Packaged Node/Bun UI | **31/31** each, zero failures |
| Final ZIP installed on the real frozen host | **11 ordinary gateway executions / 11 scoped diagnostics**, one ledger row each; `freeze-installed-result.json` and `installed-proof.json` |
| Native lifecycle and China auth | Exit 0; first two results in `final-owners-result.json` |
| Native controls, controller/MCP/history, production-IIFE auth | Exit 0; `accepted-owners-result.json` |
| Native SQLite rollback | **120 pass / 60 fresh processes**, zero fail/skip; `final-sqlite-sqlite-owner/result.json` |
| Vendor/community/SDK source provenance | 35 vendor files, 6 community files and 5 checked SDK files; `final-provenance-result.json` |

The installed-host run covers rich JSON, complete/truncated/error SSE,
final measured zero, all four unsupported currency-shaped variants,
the following unknown-usage execution, and vendor rejection. Account,
request/response identity, cache1h and reasoning reach the ordinary
ledger. Failed operations retain usage, remain failed/uncertain and are
not automatically replayed. Diagnostic lines contain none of the input,
output, error-text or credential canaries.

The original canonical native library separately passes add/list/inspect/
sdk.describe/off in an isolated HOME. Inference uses an endpoint-only
native fixture compiled from the same module, against the same SDK.
Its library hash is
`60913d83b22ba4ef48024f4a0ecf9b9370e2d809e6d29ad774bc022ecda5157d`.
It changes destinations to a loopback synthetic vendor; it does not
establish live-vendor or actual-client execution.

The host deliberately classifies accepted failed/uncertain operations in
the gateway/ledger as **409 / extension_operation_terminal**. Usage and
request/response IDs survive, but the original vendor error type is not
preserved as the ledger error type. The module event assertion separately
verifies `overloaded_error`; diagnostics record `execution.rejected`
with actual 503 or `execute.uncertain` without an invented HTTP status.
This is the existing host policy, not a module fix to that policy.

The initial old-source RED compiled: **2 pass / 16 assertion failures**;
log SHA256
`e1d3b438dd8c29c5af0f0df6acc4bbebf08166151f95e0f06dbc6542a975b632`.
Five final compiled causal mutations each reach the intended behavior
assertion (`final-causal-result.json`):

| Removed guarantee | Expected failing test/subtest events |
| --- | --- |
| Rich Facts observation | 1 |
| Measured zero replacement | 2 |
| Diagnostic callback | 3 |
| Header fact before incomplete body read | 1 |
| Observation before model/error rewriting | 5 |

Native lifecycle retains the strict forwarded forged-header refusal
after the host's PassHeaders setting allows the header to reach it.
The production guard, admission checks and native root ownership were
not weakened. SQLite cancellation restores the exact preimage; stale
preimages prevent writes; foreign rollback-time bytes retain their
journal and ErrActionChanged. Each rollback scenario uses 20 fresh
processes, preserving the root-owned native dependency guard.

## Preserved failures and remaining limits

The first ABI fixture did not answer diagnostics.emit; only its callback
driver was corrected. Provenance initially omitted the community
package.json; its pinned hash is now checked. Two installed fixtures
incorrectly expected the original vendor error type in terminal host
ledger rows; the host policy above is verified explicitly.

The first incomplete-JSON fixture accidentally enabled discovery and
failed before inference. Its correction requires exactly one accepted
POST; only the final header-removal mutation is causal evidence for the
incomplete-body guarantee. The owner-controls fixture initially retained
an API5 assertion; the updated API7 assertion passes. Original logs,
including `sparse-identity-red.log`, remain preserved.

An early parallel unit/package invocation shared `final-result.json`;
that file ends with a failed unit result and is not a green aggregate.
The final package and installed stages use unique labels and complete
receipts. No prior failure was erased or relabeled. The historical two
unattributed `/` billing requests remain unattributed; private-namespace
green evidence does not identify or fix their cause.

Desktop **3.18.2** and CLI **0.17.1** are source-verified at
`fec975905a2abd4c07e35c40d51fa097a2258ec9`; community **0.1.61** at
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`. Versioned
`source-lock.json` pins the checked files. The actual client is absent.

Final committed-host acceptance, actual-client launch/resume and
model-bound stdio/control, automatic coherent DB/WAL capture, financial
import/resume, log-collection consent/rotation/revocation, custom-root
Library placement, automatic scheduling and richer quota-card projection
remain open. No live vendor/captcha, rendered-browser or actual native
Darwin/Windows runtime acceptance is claimed. Manual native data
conversion is not automatic collection or financial resume.

The independent parent catalog closure remains in
PARENT-CATALOG-REVIEW.md and commit `4a71bd5`: 480 pass including 80
unchanged-probe passes. It is not new final-host acceptance.
AUTH-WEB-EVIDENCE.md remains byte-for-byte historical, SHA256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
