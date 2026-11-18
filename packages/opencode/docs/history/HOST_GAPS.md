# Native ownership and remaining integration boundaries

Checked 2026-11-15 against official OpenCode
`38bd73789a43bf043599d6196af96583228035d7`, native SDK API1.9 and the
separately frozen optional browser history implementation.
Source-backed native ownership is distinct from exporting the same capability
through Magpie. The approved history adapter has a separately inventoried,
unstaged host overlay; shared SDK version authoring remains leader-owned.

## What the configured native client already owns

`agent.go:agentCatalog` adds only the `magpie` inference provider with the
host gateway URL/key. `planAgent` owns the selected model/title and its checked
private overlay. `AgentManifest` launches the actual `opencode` executable
with `OPENCODE_CONFIG`; it sets no Console credential, disable-plugin or
experimental-flag environment variable.

Official `packages/core/src/plugin/provider.ts` registers `OpencodePlugin`.
`packages/core/src/plugin/internal.ts:314` puts it and the policy plugin in the
guarded set. The provider selection is not a condition on that plugin's load:

- `plugin/provider/opencode.ts:225-307` asks
  `integration.connection.active("opencode")`, resolves the native credential,
  and fetches Console config, instructions and skills.
- At lines307-360, it publishes a `ConfigIntegration.Document`, checks whether
  the policy/config owner accepted the document, and enables watches only
  with the native `console_watches` flag and workspace header.
- `config/integration.ts` restricts the remote layer to providers, remote MCP,
  policies and websearch. Native config owns its precedence and enforcement.
- `plugin/provider/opencode-watches.ts:114-204` owns native session lookup,
  deduplicated synthetic messages, deleted-session cleanup and wake counters.
  Its transport later owns WebSocket lease/poll fallback and delivery-before-ACK.
- `flag.ts:26-71` resolves `OPENCODE_FLAGS` and
  `experimental.console_watches`; stable release builds leave flags off
  unless explicitly enabled.

Thus remote organization layers, skills and watch delivery can stay entirely
inside the actual native OpenCode harness while inference uses Magpie.
They need no Magpie session mutation or remote policy/Library contract for that
route. This module neither enables them implicitly nor disables a native
connection. `agent_console_test.go` protects the native provider, policies,
instructions, skills and both native watch flag values across connect, sync
and disconnect, independently of the module's HTTP watch-inspection flag.

There is an essential identity qualification: Magpie sign-in alone does not
create the native Console connection. `integration.ts:694-714` chooses native
stored credentials or declared environment connections.
`ConsoleAccess.server/headers` uses that credential and its organization/server
metadata. The generated gateway API key is only an inference credential; it is
not a Console credential. A separately connected native client can own these
features without another package or auth sidecar. A clean native client using
only this module's Magpie OAuth will not have them activated.

The module's `console.snapshot/skill/watch.*` remain explicit selected-account
inspection actions. They do not constitute native connection setup or policy
enforcement. Console inspection now identifies itself as `magpie-opencode`
instead of advertising the official policy-evaluating client's User-Agent.
Inference/auth interoperability attribution is unchanged.

## Ranked remaining requirements

| Rank / requested outcome | Actual boundary | Minimum contract and acceptance fixture |
| --- | --- | --- |
| Base: one installed native agent + provider package | No newly identified host contract is needed. API1.9 already owns checked config/launch, selected provider/account execution and rooted history. | Actual single-package install/auth/gateway/agent/Library/lifecycle replay and stock-package WAL/media proof on the leader's exact physical host/binary. The executable fixture is distinct from real OpenCode execution. |
| 1: complete the module's rendered history workflow | Implemented as the approved optional `session-history-v1` SDK/owner overlay and module0.8.0 UI; final shared SDK1.10 integration/replay remains separate. | Actual methods, source bindings and frozen gates are in `HISTORY_UI.md`. Fixtures cover metadata without consent, trusted cancel/default prompt, independent read/root gates, exact release/CAS/ABA, issued scoped catalogs, selected WAL transcript/media, changed/off/revoked publication and old-host unsupported behavior. Count a gate as passed only when its receipt says so. No account grants history authority. |
| 2: one Magpie sign-in must also activate native Console features | No selected-account native-connection handoff exists. Current agent options carry gateway/key/model metadata, not a Console account lease. This is conditional on requiring shared sign-in; a native-owned connection already works conceptually without it. | Explicitly select module/provider/account/generation and agent binding revision, obtain a consented native-connection lease/handoff, and renew/revoke it with account/module/package/config/launch lifetime. Consume it through the supported native connection API, never by copying SQLite credential rows or refresh tokens into model config. Fixture: isolated actual native client and synthetic Console, activate the selected connection, observe remote layer/skill discovery and one bounded watch delivery, rotate/revoke/change workspace, then reject stale/foreign handoffs and withhold old content/ACK. |
| Optional: expose remote policies/Library or writable sessions in Magpie itself | These are new Magpie product integrations, not prerequisites for native OpenCode to own its features. Static `AgentLibrary` paths and read-only `StorageSource` do not grant these rights. | Only if that product surface is requested: a checked remote layer/publication owner, or a separately selected session mutation/dedup/wake owner. Do not add either merely to duplicate native OpenCode. |

The rank1 browser adapter reuses generic existing owners. For rank2, the native consumer
already requires a real non-external Console credential: `opencode.ts:236-240`
skips external credential sources. Its OAuth method accepts a selected server
and stores server/org metadata; a gateway key placed in provider settings is
insufficient. The bridge's transport/export policy must be chosen with the
host owner before promising shared sign-in. This document does not invent an
SDK method or claim that adding one env variable solves the native protocol.

The pre-implementation rendered-history proposal and exact source seams remain
unchanged in [HISTORY_UI_PLAN.md](HISTORY_UI_PLAN.md). The accepted implementation
uses actual `ui.history` source in the separate host overlay, documented in
[HISTORY_UI.md](HISTORY_UI.md); this does not publish a shared SDK version or
claim acceptance of a future merged host.

## Exact public SDK evidence

`pkg/extension/agent_metadata.go` exposes launch metadata, a profile target and
static `AgentLibrary` paths. Checked agent options and config plans provide
the gateway/model projection, not a selected Console connection.
`StorageReadLease` in `pkg/extension/storage_reader.go` must belong to the
exact parent `storage.list/read` call; an account action cannot mint it.
The base API9 `sdk/js/ui.d.ts` exposes declared account reads/actions, auth and
config CAS. The approved overlay adds `ui.history.ready` and six bounded
operations, selected by the existing manifest storage source ID. The parent
supplies the page ref; the server derives current module/package/config/
component ownership. Root slot `data` selects native root `opencode.data`;
neither the `opencode-sdk` profile nor an account is storage authority.

A future Magpie remote-config owner would need revision/precedence/revocation
and atomic instruction/skill publication. A future Magpie watch owner would
need selected-session existence/deletion, idempotent delivery, bounded wake
and cancellation/revision checks before ACK. Neither is being requested as
an essential native agent/provider contract.

## Module work and proof limits

The selected public request policy now forces upstream streaming/Chat usage,
restores caller JSON or SSE after tool normalization, and accepts only
explicit environment facts. No home/Git/filesystem inspection or tool approval
is invented. JSON collapse retains native strict completion/error handling
and exact numbers. 63 donor differential cases supplement the 99 tool cases.

The module's skill download validator now accepts optional name/description,
quoted/literal/folded scalar text and scalar metadata while preserving bundle
bytes. Anchors, aliases and arbitrary nested/non-JSON flow YAML remain a
dependency-free module limitation. Native OpenCode already uses gray-matter
and its own full parser/discovery; this module limitation does not disable
native skill parsing. There is no new parser dependency.

The installed-host proof uses synthetic HTTP and an executable fixture.
It does not prove real OpenCode Console authentication, policy enforcement,
native watch delivery, live vendor/free-tier access or actual Mac loading.
The candidate physical host tree/binary is recorded separately from its
parent and any future accepted commit in `source-lock.json` and EVIDENCE.md.
