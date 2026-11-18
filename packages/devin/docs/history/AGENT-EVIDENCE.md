# Native agent chat/session slice

Historical SDK 1.5 / agent 0.4.0 checkpoint. Its bootstrap 501 boundary
below was replaced by SDK 1.6 finite account operations. Current d070
results, including failed assignment no-replay acceptance, are in
`UNION-EVIDENCE.md`; this page's artifacts and results remain unchanged.

Date: 2026-11-14. Execution: usbox, Linux/amd64, Go 1.26.8, cgo enabled,
isolated HOME/XDG and pinned existing Go caches. No live vendor, real account,
original Devin executable, Bun backend, host changes or new dependencies.

## Implemented boundary

The optional `--agent` distribution is version 0.4.0, SDK API 1.5. It adds
an explicit `devin-native` profile of the existing `devin` agent, using
`WINDSURF_API_SERVER_URL`, `DEVIN_API_KEY` and `--model`. The ordinary host
owns binding revisions, profile adoption, shell functions, launch and offline
disconnect. The module plans no user configuration-file edits and never puts
credentials into binding values.

Chat, discovery and native session identifiers reuse the paired codecs and
configured raw bridge. Histories remain client-owned. No cloud session store,
ACP server, local tool runner, assignment token or entitlement is invented.

**This is not a complete startup adapter for the original Devin CLI.**
Native account/bootstrap calls currently return explicit HTTP 501 Connect
`unimplemented`. A synthetic recovered-wire client proves supported native
chat/session transport through actual SDK agent launch; it is not original
client or cloud-session parity.

The alternative distribution has the same module ID/native paths as paired
0.3.0; install one variant at a time. The default paired manifest stays SDK
1.4; the optional inbound-only distribution remains separate.

## Source authority

Read-only inputs under `Agents/devinapp`; no captured account/artifact bytes
are included:

| Source | SHA256 | Use |
| --- | --- | --- |
| `RUNBOOK.md` | `93480639f620b68cc16329321be515304fbcf68584f9f5e200cecb8eed3fd8a1` | Previously observed redirect; distinguishes codeium RPC from cloud ACP |
| `reference/crates/windsurf-api-client/src/client_slots.rs` | `978b16484d96b34081d0973186d3efa048a49dbb6db71208007642499276a8e9` | Endpoint env, UA product, native RPC names |
| `reference/crates/chisel-agent-bin/src/main.rs` | `be2c31a4b417e6241cdc5983e0c255dcadfba04e03119578cd4b027df72ec8a0` | DEVIN_API_KEY override |
| `reference/crates/chisel-agent-bin/src/help/main.txt` | `ca71efbb2efb8884b943cb45098e24eff33ac7647ff830731f57ab5e9a2c4ff1` | Model, resume, print arguments |
| `reference/crates/chisel-agent-bin/src/plugin_context.rs` | `3f5699397886c1fc896cee2ce6fb3bed29283615b5c9fa4f769a49e2acad70bf` | Literal Basic key-key transport and bootstrap RPC names |
| `reference/crates/chisel-api/src/wire.rs` | `b5a581afa68047f62a22710ab57f29c9f6c9c9dfa170d237e8c019a3bf8edcf3` | Secondary reconstruction of native transport/session fields |

Generated descriptors and the community encoder remain the wire authority
recorded in `codec/EVIDENCE.md` and `outbound/EVIDENCE.md`. Rust reconstruction
is supporting evidence. The initial SDK profile/binding authority is frozen
MAIN `9b8c60f477afd1d613f79618370e61bdf275c391`, including `5c0a8ef6c`.
The identical module inventory was subsequently rebuilt and verified against
MAIN `f4232ea762bf881759eba37ac1c9ea7581b627d9`.

## Frozen inputs

Inventory: every `.go`/`.py`, `go.mod`, and the three `magpie-*.json` manifests.
SHA256 is over the sorted compact path-to-SHA256 JSON map; docs and donor
metadata are excluded.

| Input | SHA256 |
| --- | --- |
| Runtime/test inventory, 48 files | `25d301f69f0a0b5af657d73f0369803e82dd0032e03efa0a8eeee9fa13a8b27a` |
| Ordinary MAIN 9b8c60f47 host | `0744fef885e23b897aea689e8dfa0916e38c27a0a2ac58b259c1855f4a9ee79d` |
| Agent library | `fa6aec7fd39b3c9cae09f50d132191a72db69dcb35045a114b8cfd8ff9475b28` |
| Paired library | `c71654e10cfd20aa0df852bcca91ed21f2be3c771a805f7dcc63fe41d21daa91` |
| Inbound-only library | `19811872adfca0292d25b1d50f357c57b7bb810daeec2b5595e049e51a5493fe` |
| Frozen SDK 1.4 host, 0dc09a544 | `ad7dc555d44bb5bb6900661011009365f0d83f67a54d110d0df0cdd9c64707d8` |

## Terminal gates

| Gate | Result |
| --- | --- |
| Agent regressions, race count20 | Exit 0; 360 test-run events, 363 PASS events including packages |
| Complete module, race count3 | Exit 0; 1332 test-run events, 1335 PASS events including packages |
| gofmt, go vet | Exit 0, no formatting differences or diagnostics |
| Agent, paired, inbound C-shared builds | Exit 0 |
| Four temporary negative overlays | Intended assertion FAIL for field/origin/model-translation/bootstrap guards |
| Ordinary native-host agent proof, three fresh sandboxes | 18 grouped checks, 12 native inference requests, zero external requests |
| Paired on current SDK 1.5 host | 18 checks, 13 native requests, live generation replacement, zero external requests |
| Paired on frozen SDK 1.4 host | 18 checks, 13 native requests, explicit restart-mode check, zero external requests |
| Agent installed into SDK 1.4 host | Exit 1 before activation: `json: unknown field "profile"`; no library installed or external request |

Three command packages contain no tests and have package-level skips. No
agent regression was skipped. No non-Linux native execution, timed fuzzing,
full host-suite acceptance or published release is claimed.

## Actual host path

Each run starts ordinary `magpie web --no-open --gateway` in a fresh HOME,
authenticates its normal web cookie and warms `/v1/models`. Normal
`POST /api/set` adopts the profile; no ledger or test-only gateway is used.

Ordinary `magpie devin-native env/run` uses the durable binding. A temporary
executable named `devin` speaks recovered wire: literal Basic key-key, unary
catalog, framed chat, native session ID and signed thinking. It is a
synthetic client, not the original agent.

Initial history, same-session resume and an independent session preserve
native IDs and message associations at the selected provider. A fourth
request proves Connect deadline and actual upstream cancellation. The fake
vendor accepts owned methods/paths and a unique selected-key nonce. Local
agent keys never reach it; ambient GETs consume no responses or inference.

The exact installed library hash appears in `/proc/<gateway>/maps`. Disable
refuses ordinary launch before client execution. Normal offline disconnect
restores shell preimages and leaves personal Devin config bytes unchanged.
Every process stops and every sandbox HOME is removed.

## Remaining host boundaries

1. **Original CLI startup is blocked.** GetUserStatus, GetCliTeamSettings,
   GetAccountManagedPlugins and AssignModel do not select the gateway model
   as chat does. `pkg/protocolext/inbound.go:29` has no native account-operation
   request. The dispatch switch is
   `internal/gateway/protocol_extensions.go:184`; the operation whitelist at
   `pkg/extension/provider_operations.go:28` supports media, not declared
   native reads. Agent binding snapshots reach agent handlers, not native
   decode/account dispatch.

   Minimal generic support: resolve an inbound operation against the current
   owned agent binding, select its provider/account under caller visibility
   and generation checks, pin the same module lease, and execute a declared
   bounded native read with its own response. Domain codecs stay here.
   Assignment writes need separate declared effect/receipt rules. No private
   credential-store reads, caller-chosen accounts, dummy chat or manufactured
   entitlement substitutes for that contract.

2. **Cold CLI model selection is separately limited.** A new CLI process
   refuses `magpie/<dynamic-SDK-model>` at `internal/agent/agent.go:763`, and
   the raw form through `agent.go:803`, although `/v1/models` lists it in the
   running host. The cache at `internal/provider/extension_provider.go:40` is
   process-local. The proof binds via the ordinary GUI action in that warmed
   process, then launches via ordinary CLI. It does not claim cold selection
   succeeded. A generic refreshed/persisted generation-bound SDK catalog
   before spelling would remove this limitation.

The profile therefore requires an explicit model and loopback gateway. The
host's caller-key parser strips Bearer, not literal Basic key-key; remote
named-key support is not claimed.

## Evidence locations

On usbox: `/tmp/magpie-devin-agent.PmLC1o/`.

```text
gates-result.json
agent-race.log
module-race.log
negative-result.json
proof-result.json
agent-proof-{1,2,3}/{result.json,gateway.log}
paired-proof-current/{result.json,observations.json,gateway.log}
paired-proof-sdk14/{result.json,observations.json,gateway.log}
agent-floor-sdk14.log
agent-first-prefixed-selection-failure.log
agent-live-raw.log
```

Earlier fixture-only failures had a mismatched cancellation trigger, an
invalid DenyProxy attribute and a premature async catalog read. The final
results above use corrected, frozen source, not partial runs.

## Current-host checkpoint

The following additional terminal verification used clean host MAIN
`f4232ea762bf881759eba37ac1c9ea7581b627d9`, Git tree
`16cdbfcc57320c5bb1ec8b5e7af58fbba53a7620`. Host source was exported by
`git archive`; no host source changes were made. The 48-file module
inventory is still
`25d301f69f0a0b5af657d73f0369803e82dd0032e03efa0a8eeee9fa13a8b27a`;
all files matched before and after verification.

Host binary SHA256:
`2a6098a48123ec0376fad9cde0c2e2cc971ae2a30b8ba51d0da1adcdd6457d44`.
All three rebuilt library hashes are identical to the frozen-input table.

| Gate on current MAIN | Terminal result |
| --- | --- |
| Agent regressions, race count20 | Exit 0; 360 test runs, 363 PASS events |
| Complete module, race count3 | Exit 0; 1332 test runs, 1335 PASS events |
| gofmt, go vet, all Python files compile | Exit 0; no formatting or vet diagnostics |
| Ordinary host and three C-shared distributions | All builds exit 0 |
| Inbound-only C ABI and ordinary CLI conformance | Exit 0; two concurrent native sessions |
| Four negative Go overlays | All fail at their intended field/origin/model/refusal assertion |
| Agent ordinary native-host proof, three sandboxes | 18 grouped checks, 12 native inference requests, zero external requests |
| Paired ordinary native-host proof, live replacement | 18 checks, 13 native inference requests, zero external requests |

The three no-test command packages account for package-level skips; no
regression was skipped. All four ordinary-host E2Es stopped their processes
and removed their sandbox HOME. Two unrelated local GET probes were recorded
in one agent run; they did not consume scripted responses or inference.
Actual source hashes, artifacts, commands and terminal results are in
`/tmp/magpie-devin-current.orl5O8/{source,result,negative-result}.json`
on usbox. Detailed outputs are `agent-race.log`, `module-race.log`,
`inbound-conformance.json`, `agent-proof-{1,2,3}/result.json` and
`paired-live/result.json` in that directory.

This adds current-host evidence; the older SDK 1.4 floor/compatibility checks
above retain their original frozen binaries and scope.

## Exact unsupported native surface

These are implementation limits, not a claim that the recovered schemas
are missing. The recovered names are documented in
`windsurf-api-client/src/client_slots.rs:90-128` and the generated descriptors.

| Surface | Current behavior |
| --- | --- |
| `GetUserStatus`, `GetCliTeamSettings`, `GetAccountManagedPlugins`, `AssignModel` inbound | Registered explicit HTTP 501 Connect `unimplemented`; no selected-provider dispatch |
| `GetImageCaption`, `GetDevstralStream` | No inbound registration; ordinary host unknown-route refusal |
| `AddUserManagedPlugin`, `RemoveUserManagedPlugin` | No inbound registration or write receipt contract |
| `EnablePartnerProvider`, `DisablePartnerProvider`, `Attribution` | No inbound registration or account-policy/telemetry emulation |
| Hosted/cloud sessions, ACP and agent-local tool execution | Not implemented; native session IDs and client-owned history are the covered transport |
| Refresh/revoke | No recovered renewal/revocation RPC implemented; expiry requires sign-in, logout removes host state |

Paired request checks are in `codec/paired.go:130-151,279-299`:

- Envelope field 5 cannot enable internal-model selection. Enum-only
  field 6 cannot replace an explicit model UID/name in fields 21/14.
- Request type field 7 accepts only 0/5; nonempty experiment config 9,
  active arena count 24, arena assignment JWT 25 and model assignment JWT
  26 are refused. Envelope fields 4/23 and fields above 28 are refused.
- Credential metadata fields 22/29/31/32 must be empty and 33 must be zero;
  unsupported metadata fields 18/19/23/30 and fields above 33 are refused.
  Incoming credentials are discarded, not forwarded.
- Prompt role field 2 accepts 1/2/4/5. Configuration completion count field
  1 permits at most one completion; output limit field 2 is bounded.
  Redacted prompt state without a recognized native signature is refused.
- Nonempty team/site selectors are not resolved against vendor accounts.
  Imported, unknown or cross-caller/account/generation/model signatures
  cannot be replayed; only this module's proven origin receipts qualify.

The optional inbound-only Chat converter has stricter limits than paired:
signed history, active top-k/max-newlines and native-only controls cannot be
translated there. They are not missing paired features: paired preserves
their native bytes for the same module executor. See `codec/README.md`.

Removing the bootstrap blocker requires a generic host contract before
module codecs can safely dispatch those account operations. It is not
resolved by reading private account files, forwarding the local CLI key,
issuing a dummy chat or returning fabricated entitlements. The separate
cold CLI catalog and remote literal-Basic-key limits above also remain.
