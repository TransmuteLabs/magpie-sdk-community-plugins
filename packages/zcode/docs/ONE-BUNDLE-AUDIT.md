# One-install ZCode behavior audit

2026-11-14. Hard requirement: our native module, UI and any packaged
helpers form **one installation**. No community provider, OpenCode auth
plugin, legacy mover, Bun provider host or second account store may be
required. The bounded owned-agent continuation uses frozen host
`d07078c7926daf6f33af09c40297c941bd39da92` for its historical proof.
Current 0.11.0 acceptance pins committed shared host `9e88f10`, excluding
unfinished SDK 1.7. No host or live account changes.

## Current runnable boundary

- Native primary: provider/auth/refresh/models/usage, controller,
  local-owner tools, fixed remote image search and manual history.
- Native/UI/agent 0.11.0: selected account actions, offers and explicitly
  confirmed check-in plus host-owned browser
  start/poll/cancel/reauthorization. The production-IIFE fixture installs
  only this native bundle into a clean HOME; its fake vendor is
  verification infrastructure, not a runtime plugin.
- SQLite continuation: in-process Go driver from the existing host lock
  graph. Python is fixture generation only.
- Owned `zcode-native` profile: AgentWiring and agent.inspect/agent.plan
  are in that same native module. Default-HOME connect/set/sync/disconnect,
  checked launch and inherited Library are exercised through the ordinary
  host routes with a synthetic executable. Actual ZCode startup is unverified.

The donor-parity continuation adds its gift routing, explicit feed/check-in
actions and Start admission inside this same module. No community runtime
or scheduler is installed. The 0.9.0 evidence remains an immutable earlier
checkpoint; the current package identity and bounded acceptance are in
`PARITY-EVIDENCE.md`. The ordinary `scripts/build.py --output DIRECTORY` output now selects this
full composition without flags; `PACKAGE-EVIDENCE.md` records its
separate artifact proof. The historical provider-only build is explicit
`--headless` compatibility, not another module required for installation.

## Existing generic contracts

SDK 1.5 already provides AgentDescriptor, AgentLaunch, AgentProfile,
AgentLibrary, exact AgentBindingSnapshot/Change, checked file plans and
host-owned backup/rollback. These suffice for the default-home agent
cut; adding vendor implementation to core is unnecessary.

The distinct owned agent ID is `zcode-native`, provider `zcode`
in the same manifest and the existing ZCode executable as the actual
agent application. This application is not another Magpie plugin.
Vendor tokens stay in the sole SDK account namespace. Config receives
only the host's gateway URL/key and published model IDs.

The accepted upstream ZCode Library target already knows its
AGENTS.md/skills/MCP layout. AgentProfile target `zcode` can inherit that
core target without installing another plugin. AgentLibrary.MCPFormat
does not currently accept `zcode`, even though the built-in fmtZCode
exists. Do not pretend OpenCode's MCP file shape is equivalent.
An explicit custom Library placement may need a generic structured-JSON
MCP layout contract; default-profile reuse avoids that host change.

## Source-grounded config and launch

Recovered CLI `docs/usage/03-config.md`, `13-automation.md`,
`packages/adapters/src/config/{file-config,env-config}.adapter.ts` and
`packages/provider-node/src/runtime-paths.ts` establish:

- Model rules: `.zcode/v2/provider_config.json`, schemaVersion 1,
  config.providerConfigRules.providerRules and modelConfigRules.
  Provider API uses `type: anthropic-messages`, `baseUrl` and api-key
  access. Keep providerModelRules/manualProviderModelRules distinct.
- Selection: config.defaultModelSelection with providerId/modelId and
  only the chosen model's declared options.
- Runtime/MCP: `.zcode/cli/config.json`, `features.mcp` and
  `mcp.servers.<name>.{type,command,args,enabled}`. This is not an
  OpenCode `mcp` mapping. Skills/instructions remain separately owned.
- `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE` and `ZCODE_DATA_BASE_DIR` are real
  path overrides. Old `ZCODE_MODEL`/`ZCODE_API_KEY` and ZCODE_BASE_URL
  are not interchangeable model-config overrides.
- The CLI supports explicit prompt/cwd/mode/output-format and resume.
  Preserve argv arrays, user settings and native process cancellation;
  never build a shell command from model text.

Current host agent/zcode.go already preserves provider formats, user
manual model rules, output/reasoning choices and custom data roots.
Those are donor invariants, not a reason to copy the kernel wholesale.

No `zcode` executable was found on usbox during this audit. A synthetic
argv/config fixture cannot be claimed as actual recovered CLI startup.
Obtaining/building a compatible recovered executable with already locked
artifacts is an agent-domain artifact task, not a new host auth contract.

## Owned-agent verification boundary

`testdata/agent/run.py` installs one native/UI/agent bundle in a clean HOME.
It uses production handlers and descriptors with only synthetic endpoint
registration substituted. Its Python executable is explicitly a stand-in
for the absent ZCode client. `AGENT-EVIDENCE.md` records terminal results,
source and artifact identities and preserved failures.

| Gate | Observed boundary |
| --- | --- |
| One installation | One native SDK catalog entry and account store; browser Start and imported paid fixtures share that owner. No legacy plugin/auth store, node_modules or community runtime is installed. |
| Configuration plans | Public SDK handlers cover connect/set/sync/disconnect, canonical binding revisions, exact baseline restoration or new-file removal, manual rules, API format and foreign edits. Ordinary preview is inert and a stale revision is refused. |
| Launch ownership | Actual host run/env commands carry literal argv and a config path containing spaces and `{key}`. File drift, changed module config and module off refuse launch. Vendor credentials stay out of the agent config and renderer. |
| Conversation and native aliases | Ordinary Messages requests reach Start, OffPeak twice and Highspeed once. Journals retain the exact host conversation/account/generation; the vendor receives matching derived session identity. One ticket and one card are admitted. |
| Forged headers | Default-filtered headers cannot grant native admission to Start. With the host's explicit PassHeaders setting, both forged ticket/card headers are refused before any vendor inference or admission. The guard-disabled compiled module must make that assertion fail. |
| Library | Instructions, MCP and skills roundtrip through the inherited default ZCode target while preserving personal entries. Custom data-root placement is unimplemented. |
| History and UI | The bundle retains its existing manual history adapters and control page. SQLite/manual history evidence remains in its own checkpoint; actual client resume and rendered-browser acceptance are unverified. |

The initial forged-header RED assumed that all incoming headers reached
ExecuteRequest. The host reconstructs outbound headers and passes only
explicitly configured client headers. The fixture now enables that
existing public setting before asserting strict module refusal. The
original RED remains evidence; no native guard or host contract was relaxed.
