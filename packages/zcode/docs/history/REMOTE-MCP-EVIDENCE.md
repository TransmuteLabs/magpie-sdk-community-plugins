# Fixed official MCP execution evidence

2026-11-13. Module 0.6.0 remains a native SDK API 1.4 checkpoint, ABI 1.
All execution below ran on **usbox**, with isolated HOME/XDG, pinned
Go 1.26.8 caches, GOPROXY/GOSUMDB off and synthetic loopback endpoints.
No live vendors, accounts, installed apps, new dependencies or publication.

## Evidence and implemented boundary

Recovered read-only source in `Agents/ZCode`:

- `docs/coding-plan/zcode-official-server-mcp-client-phase-1-spec.md:867`
  fixes the `POST /api/v1/mcp/server/image_search` route and normally
  stateless transport.
- `apps/zcode-cli/packages/adapters/tests/mcp-official-auth.test.ts:610`
  supplies `search_image` with `query`.
- `scripts/dev-official-mcp-server.mjs:66` pins protocol `2025-06-18`.

The implementation declares only local-owner `zcode_image_search`.
It verifies the selected profile and immutable session/business JWT
channels, initializes, notifies, checks bounded tool-list pages and
dispatches exactly one fixed call. Unknown advertised tools cannot become
host tool declarations. There is no free/PAT fallback, redirect,
reconnect or tool replay. Auth material is not accepted in arguments.

Synthetic tests cover JSON/SSE, progress/heartbeat frames, exact IDs,
stateful/stateless sessions, cursor pagination/loops, schema changes,
list bounds, transport/RPC failures, cancellation and sanitized errors.
Acceptance precedes dispatch; failure after dispatch remains uncertain.
Cancellation targets the original JSON-RPC ID without repeating the tool.
Result content/isError are retained; extra fields become the documented
`zcodeMCPResultFields` text block, preserving large JSON integers.

## Exact source and artifacts

Proof directory: `/tmp/magpie-zcode-remote-mcp-kwgcdikv` on usbox.

63 Go/Python/module files were individually compared against the local
source and the remote source used by verification. The sorted JSON
SHA-256 manifest is `source-hashes.json`:

`ea7b3d36263246b3e03778dedde64735d545564af14da4c9af27cc27b7068dab`

Linux amd64 native release ZIP:

`dea443788b0daca4c7ebd0db7ec5b0240ff4298b4295adf5724a571196fa2b6a`

Actual immutable SDK 1.4 host binary:

`bf11f37b125b4a6b59ba3c8501b323cc98b9556bad55ed1af3b8fa477092efca`

The native-host endpoint overlay changes only destinations in the
fixture entry point. It does not replace auth, the manager, journals or
protocol handlers.

## Gates

- Empty-output `gofmt -d .`, `go vet ./...` passed.
- `go test -json -race -count=20 -timeout=15m ./...` passed:
  **3820 passing test/subtest events, 191 distinct tests/subtests**.
  No failed or skipped tests. The two entry-point packages report
  `[no test files]`.
- Linux native C-shared ZIP build and actual ABI conformance passed;
  two ABI sessions execute in one host process.
- Actual native host controller/auth E2Es passed, including selected
  generation, atomic refresh, reset barrier, no-replay and history tools.
- New native-host MCP proof verified exact JWT channels, endpoint,
  version/session/IDs, bounded SSE, one remote call, host intent replay
  rejection and dynamically undeclared tool rejection.
- CGO-disabled Darwin arm64 and Windows amd64 vet/build passed. These are
  compilation gates, not native runtime tests on those systems.

Three separately compiling negative overlays failed the intended
regressions: relaxed JSON-RPC ID matching, removed session binding and
substituted PAT for the business JWT. No build failure is counted as a
behavioral negative proof.

Logs: `final-results.json`, `negative-results.json`, `*-final.log`,
`native-controller-final.log`, `native-auth-final.log`,
`negative-{id,session,auth-channel}.log`.

## Limits

This is a recovered, fixed protocol cut, not a live vendor compatibility
claim or complete modern MCP client. Other negotiated versions,
server-initiated requests and dynamically published tools fail explicitly.
SDK 1.4 lacks native structured-content projection. No external standalone
MCP service, selected-account UI, auth broker, SQLite import, automatic
history reader or live resume is implied by these gates.

The earlier generic fixture-target intermittency recorded in
SESSION-EVIDENCE.md is not assigned a fabricated production fix. It did
not reproduce in this final 20-run gate.
