# Full ZCode donor and observability checkpoint

2026-11-14. Module-only continuation of `6470897`, with the native
log/SQLite fidelity slice already committed as `371b06c`. No host,
other module, branch, worktree, dependency, installed account or client
was changed. There was no push, publication, live vendor call or client
installation. SDK-only deployment remains independent.

## Frozen identities

| Item | Exact identity |
| --- | --- |
| Ordinary package | Full 0.11.0: provider/controller/owned agent/UI, native ABI 1, SDK floor 1.5 |
| Historical compatibility | Explicit headless 0.7.1, SDK floor 1.4; no second install is needed |
| Shared host | `9e88f10982a1d66c1b402fba27caddda8ac4ac4e` |
| Shared host tree | `4f6525135603b84cdc552403923f39b44fa1dea4` |
| Host binary SHA-256 | `ba7da01a14f1959f29cddd6a9ab342c7cc67587e9439f60d40114384a396a549` |
| Vendor source | `fec975905a2abd4c07e35c40d51fa097a2258ec9`, desktop 3.18.2 / CLI 0.17.1 |
| Community donor | `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, package 0.1.61; separate 3.17.2 wire lineage |
| Plain 153-file source inventory SHA-256 | `b7745fa604849fd1545ec31a1b761c050a48c1e2f3cd7f97b8e8b44f8faea829` |
| Tagged inventory, including host commit/tree | `0b41077ae631269cedda8fd1f3fe7a16c22afc66f8c24e878ba2895a7d9c4e64` |
| Ordinary Linux amd64 ZIP SHA-256 | `bb2f42924cb5a2ee6f3edb38fbda7fa6794b9d9e41c7a6674b9244aaf1918cd6` |
| Packaged native library SHA-256 | `a0f998da32228c7cf6060f9b1fcd3b05d0263cbdbd52db8928bc35a4f4d688d8` |
| Headless compatibility ZIP SHA-256 | `890722ae6829f885d97823b38a77ef41e3ec83f5f8641dcae7a012c249cc3fee` |
| Aggregated release evidence SHA-256 | `d71d2946a3a5fb25b3c0ffaf35e39bb0343fd4912b77ae5c7d9f4f261c53ceae` |

Final verification root:
`usbox:/tmp/magpie-zcode-devzeus9e88-vtaIPu9A`.
Local ordinary artifact:
`dist/zcode-0.11.0-devzeus9e88/magpie-plugin_linux_amd64.zip`.
That directory also holds checksums, artifact metadata, the tagged
inventory and `release-evidence.json` with 20 reports and 50 log hashes.
All 153 inventoried local source files matched the remote bytes.
Evidence narratives, ignored distribution files and Git metadata are
excluded from that source inventory. ZIP hashes identify tested archives;
rebuilding timestamps need not reproduce the ZIP.

The host is a read-only archive of the exact committed tree. Its public
SDK remains 1.6. The intervening 9e88 change preserves native authority
across effect-free agent sync; dirty API 7 telemetry is excluded.
Provenance checks verified 30 pinned vendor files, five checked community
files and the separately archived community package version.

## Implemented behavior

`auth_complete.go`, `discovery_selection.go` and `auth_auto_test.go`
preserve confirmed Personal→Team→Start selection. A Personal absence
cannot skip a valid Team seat. Read-only entitlement envelopes accept the
donor's optional success code while validating every present field.
Heterogeneous Personal records and an earlier malformed Coding sibling
cannot hide a later valid Coding entitlement. Malformed Coding data,
unknown Team enums, ambiguous selection and a changed selected product
remain errors, without minting a new identity or falling through to Start.

`gift.go`, `discovery_models.go`, `discovery_start.go`, `module.go` and
`usage.go` retain manual-import gift discovery, actual Trial response
aliases, held-route bypass before acceptance and strict gift-only
authority. A second gift read losing confirmation cannot become paid.
Named pending grants and redundant buckets are informational; only the
best applicable bucket controls each model. Existing claim outcomes,
principal debt, dual Trial and adaptive Start holds stay enforced.

`identity.go`, `protocol/start.go`, the request/response codecs,
`paid_dispatch.go`, `response.go` and `response_usage.go` add source-shaped
3.18.2 Start blocks/cache boundaries, user-turn query identity, stable
session/trace identity and unique requests. Nested ToolSearch, multiline
SSE and caller text/tool/signature data survive. Paid fallback receives
the original identity-stamped Messages envelope. Only complete output-free
JSON HTTP 429 codes 3008/3009/3010 permit bounded paid retries; native
ticket/card inference and uncertain or partial responses never retry.

The legacy SDK ledger receives uncached input/output/cache-read/cache-write
cumulative snapshots. The ordinary gateway fixture verifies four successful
Start/native calls each record exactly `[7,5,11,3]`; no cache tokens are
added into uncached input. Richer reasoning/cache-TTL/credit/cost details
remain in source bytes where present. `history/`, `historydb/` and
`tools.go` preserve CLI diagnostic JSONL, incomplete tails and optional
SQLite model/turn/tool attempts, nullable metadata and raw usage without
fabricating conversation records or counting their usage twice.

`window.go` and `web/{control,page,index.html}` expose selected-generation
offers and explicitly confirmed check-in with single-flight/auth locking
and no automatic write replay. `build.py` keeps one ordinary full package,
one native library and static UI. The community runtime is not installed.
`source-lock.json`, `sources_test.go`, provenance fixtures and the source/
contract documents distinguish package, SDK, desktop, CLI and donor versions.

This reuses existing SDK owners, donor codecs and locked SQLite libraries.
It adds no scheduler, duplicate account store or transport runtime.

## Acceptance

All builds and behavior checks ran on usbox with Go 1.26.8, existing
offline caches, private HOME/XDG, timeouts and loopback-only namespaces.
UID/EUID remained 1000 and all five capability masks were zero. Native
root ownership and strict forwarded-header assertions were unchanged.
No broad host `./...` suite was run.

| Gate on exact 9e88 | Result |
| --- | --- |
| Full module race regression | 385 pass events, zero failures/skips |
| Scoped race repeat, count 20 | 1220 pass events, zero failures/skips |
| Domain/data causal overlays | 21 compiled behavior-negative assertions reached |
| Agent config/ownership and real forwarded-header causal checks | 4 expected assertion failures |
| Compiled page confirmation causal check | 1 expected assertion failure |
| gofmt, module vet, Python syntax | Pass |
| Ordinary full and explicit compatibility build/ABI | Pass |
| Strict TypeScript and packaged Node/Bun UI | 31/31 each |
| Native lifecycle, China auth, selected controls, controller/remote-MCP/history, production-IIFE UI | All five exit 0 |
| Native SQLite rollback | 60 fresh race processes, 120 pass events, zero failures/skips |
| Darwin arm64 / Windows amd64 | Vet and CGO-disabled compile pass; no native runtime claim |
| Exact donor bytes and desktop/CLI/package versions | Pass |

Ordinary native lifecycle covers connect/set/preview/disconnect,
inert/stale plans, literal argv and paths, preserved foreign config,
Library instructions/MCP/skills, Start and OffPeak/Highspeed routing,
one ticket/one card, selected journals, gateway ledger and refusal after
file/config/off changes. The actual ZCode executable was absent; the
launch executable is explicitly synthetic.

The forwarded-header regression originally stopped at the host's default
header filter. Its existing PassHeaders setting now forwards the exact
forged ticket/card headers before strict refusal is asserted. Disabling
the module guard makes the actual lifecycle assertion fail. This is a
causal module test; no native admission or ownership guarantee was relaxed.

SQLite cancellation after a real write restores the exact binary preimage;
a stale preimage prevents writes; foreign rollback-time data survives with
the prepared journal and ErrActionChanged. Each scenario ran in 20 fresh
processes against the actual full native module. All 10447 SDK production
files were unchanged, inventory
`30b7d4f84899b19533e0d72df9dfe1b442d223fc44dee72eae41578c1ba52bd2`.
The injected test fixture remains
`2700c35785974f55f847830e7f86aeafa33b25415d72d8f767143e3d78392d5b`.

## Preserved failures and earlier evidence

Initial compiled donor REDs remain in
`usbox:/tmp/magpie-zcode-completion-T8c1l5jW`: completion, usage,
Start shape, auto auth, paid-envelope, transport, log and data assertions.
`data-old-red` failed compilation; only `data-old-red-compiled` is a
behavior RED. The budget fixture eventually passed the old code and is
not reported as a newly demonstrated defect.

Intermediate acceptance and the strengthened entitlement REDs remain in
`usbox:/tmp/magpie-zcode-final-nnOu7qk7`. That root retains
`pre-acceptance-vvdgdyd3`, `pre-agent-fixture-_c46zldm` and `sqlite-owner`.
The first paid-envelope mutation failed compilation, and the first
auto-team mutation exposed a fixture blind spot; neither is counted as a
successful negative proof. Corrected compiled fixtures subsequently failed.

The original SQLite owner run passed cancellation, then the guarded loader
refused a second independent native image in the same namespace process:
`native_restart_required`. The driver now gives each scenario its own
process, preserving every rollback assertion and the native root guard.
This is verification isolation, not a loader fix.

An intermediate lifecycle fixture rejected the new manual-import gift read
because its shared donor handler accepted only browser-issued sessions.
Its explicit manual-session branch now validates the selected device,
3.18.2 query and session before returning a synthetic empty gift balance.
The failure remains preserved. It does not explain the two historical
unexpected `/` billing requests, which remain unattributed.

The historical root 0.10 ZIP remains untouched with SHA-256
`2803d64d786a4d08f5abb7ee283fba1e6ea300d8fb82b69abe09640492fde191`.
Historical untracked AUTH-WEB-EVIDENCE.md remains unchanged with SHA-256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.

## Pending gates and explicit limits

This is a tested bounded donor slice, not a whole-plugin/live-parity claim.
Actual recovered client startup/resume and the model-bound stdio/control
adapter remain module work; SDK 1.6 NativeAccountOperations already exists.
The synthetic launch does not establish that wire.

Automatic consistent DB/WAL capture, authorized diagnostic collection/
rotation/revocation, reviewed archived financial resume/import, custom
data-root Library placement and automatic daily scheduling remain separate.
Manual immutable DB conversion and explicit check-in do not implement them.
Rich quota amount/display/span projection still needs a distinct public
card contract. Live vendor/captcha, rendered-browser and native
Darwin/Windows runtime acceptance have not run.

The host owner's optional API 7 Usage.Facts and diagnostics.emit contracts
are supplied but uncommitted in this base. Exact frozen-base adoption,
sparse-zero/richer fact projection and structured operation diagnostics
are pending; LEADER-CONTRACTS.md records the bounded contract. No raw text,
body, auth or credential logging to stderr substitutes for that callback.
Native log preservation is an explicit supplied-data tool, not telemetry.

The requested independent owner adapters for sync middleware, budget,
masking, sanitized ledger and settings form a subsequent separately agreed
scope. This checkpoint makes no host edits or claim to implement that work.
