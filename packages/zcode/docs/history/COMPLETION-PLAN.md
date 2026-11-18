# Source-backed ZCode continuation

2026-11-14. Continue accepted `6470897` in this module only. Preserve
`AUTH-WEB-EVIDENCE.md` and every prior failure/evidence record. No branches,
worktrees, dependencies, live vendors/configuration, client installation,
host edits, publication or push.

Initial RED verification source is the committed shared host
`7fbda87dc281e807e121a185760406363d100494`, tree
`86263d8ccf0320edb81d5e70f79ddcfa692c9b06`. Its public runtime sources
match the previously verified d070 SDK 1.6 runtime. Concurrent uncommitted
SDK 1.7 decision work is excluded. Vendor and community remain clean at
`fec975905a2abd4c07e35c40d51fa097a2258ec9` (desktop 3.18.2, CLI 0.17.1)
and `26ea8faf7793ca66dd8e7a8b90128a5235552a1f` (community 0.1.61).
Intermediate acceptance advances to committed shared host
`87359a70169661ff9b5b4f71e0831e2c606b293f`, tree
`35ba40b44153416955226ffc922bcb98c64eec9f`: upstream merge plus
SDK-only startup correction. Public SDK/runtime owner paths are unchanged.
Uncommitted SDK 1.7 remains excluded; no host tree was edited.
Final acceptance uses the user's subsequent committed base
`9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, tree
`4f6525135603b84cdc552403923f39b44fa1dea4`. Its only intervening
runtime change is the host owner's effect-free agent sync authority fix.
Public SDK contracts are unchanged. API 7 facts/diagnostics adoption still
awaits the separate telemetry freeze.

File-level extraction sequence; compiled behavior RED precedes each port:

1. `gift.go`, `discovery_models.go`, `module.go`, `response.go`,
   `protocol/response.go`: exact requested response model aliases in JSON/SSE,
   pre-dispatch bypass of a locally held free-first route, gift discovery
   for manual imports, and confirmed inactive-paid gift selection. Keep
   Trial strict and unknown/partial results non-replayable.
2. `identity.go`, `client.go`, `protocol/request.go`, `response.go`,
   `protocol/response.go`: turn-stable query identity, source-grounded Start
   request shape, ToolSearch restoration in all message envelopes, and
   normalized token/cache usage for the SDK ledger. Preserve original
   signatures, tool payloads and caller text. Compare exact vendor/community
   fixtures rather than copying the community loader.
3. `discovery_start.go`, `usage.go`, `checkin.go`: bounded pending-plan,
   per-bucket plan grouping and redundant-bucket applicability, offer/session
   hints and honest unknown capacity. Keep display metadata separate from
   routing authority. No speculative feed polling during usage reads.
4. `window.go`, `web/control.ts`, `web/page.ts`, `web/index.html`,
   native codec/account actions and fixtures: explicit selected-account
   offers/check-in controls, and finite native account operations if the
   actual recovered client wire matches the existing SDK 1.6 contract.
   Advance the full package contract only with implemented, tested use.
   Automatic daily scheduling needs a verified owner/lifecycle; the finite
   account-operation contract alone does not provide one.
5. Audit auth selection, reset/execution receipts, agent config/Library,
   session/SQLite, usage/log/data collection against their exact writers.
   Port recoverable domain behavior with fixtures. Record a precise minimal
   generic contract for host-owned operations the SDK cannot express.
   SQLite/WAL snapshots cannot be presented as SDK 1.6 JSONL sources.
6. Freeze source inventories; run scoped race/causal negatives, module
   regression/static checks, ordinary full ZIP, native ABI/auth/agent/UI
   owner fixtures and platform compile checks on usbox with pinned offline
   caches, isolated HOME/XDG, loopback namespaces and timeouts. Commit
   coherent module slices with Lore and immutable evidence.

The ordinary package stays one provider/controller/agent/UI installation.
Synthetic client fixtures do not prove actual client execution. Native
Darwin/Windows, rendered-browser, live financial/vendor acceptance and
actual-client resume remain separately identified until exercised.
The two historical unexpected `/` billing requests remain unattributed.
Module completion does not gate the first plugin-free host rollout.
