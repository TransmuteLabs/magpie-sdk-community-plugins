# SDK 1.6 native account/bootstrap checkpoint

Historical pre-union checkpoint. Source6 and cold CLI catalog persistence
are now combined in d070. `UNION-EVIDENCE.md` records its passing paired/
cold/startup checks and failed agent no-replay acceptance. The worktree
and artifact references below describe the earlier run only.

Date: 2026-11-14. Executor-owned host worktree:
`../magpie-native-account-ops`, detached from
`f4232ea762bf881759eba37ac1c9ea7581b627d9`. The new capability requires
SDK 1.6; codec minor is 0.5. This does not extend accepted SDK 1.5.
MAIN/Source6 integration and the durable cold CLI catalog are leader-owned.

All builds/tests ran on usbox with isolated HOME/XDG, offline locked
toolchains/caches and local synthetic peers. No live account/vendor,
installed application, Bun backend, new dependency or push was used.

## Implemented and exercised

The optional 0.5.0 agent bundle declares native GetUserStatus,
GetCliTeamSettings and GetAccountManagedPlugins reads, and an AssignModel
write. There are no unconditional 501 placeholders for these four methods.
The host selects the current owned agent model/account, pins the original
inbound module lease and invokes its declared module.call action.

Incoming local credentials, supported_model_displays (metadata 30) and
opaque fingerprint f (31) are validated and discarded. The module builds
new selected-account metadata. Active team/tier/evaluation selection cannot
choose another account. Both account RPC and native agent inference accept
the recovered informational metadata; default paired behavior is unchanged.

Assignment uses its recovered cascade/model request, durable acceptance and
no-replay. Successful opaque tokens get account-generation/caller/model/
cascade receipts. Unknown/imported tokens cannot cross that origin boundary.
Private host errors become safe native errors. Native vendor bytes and
entitlements are never manufactured by the host or module.

## Terminal gates

| Gate | Result |
| --- | --- |
| Complete public Go SDK and codec race count3 | 1233 test runs, 1235 PASS events, exit0 |
| Gateway/agent/provider/extension scoped race count3 | 96 test runs, 100 PASS events, exit0 |
| Complete module race count3 after recovered metadata fix | 1374 test runs, 1377 PASS events, exit0 |
| Node / Bun | 130 passed each, no skipped tests |
| Strict public TS fixtures | Exit0 |
| Host/module vet | Exit0 |
| Ordinary host, native agent/paired libraries and network guard | Build exit0 |
| Native agent HTTP/assignment/continuation proof | Six grouped checks; 4 inference requests; only our bundle |
| Ordinary paired live-generation proof | 18 checks; 13 inference requests |
| Unmodified recovered application's startup and first turn | Exit0; native preflight and 1 inference request |

Three module command packages contain no tests; their package-level skips
are not missing regressions. These are bounded SDK/module/ordinary-host
gates, not a full application suite or non-Linux native acceptance.

## Actual application and only-own-bundle proof

The reconstructed Rust application was built offline from the committed
`Agents/devinapp` reference subtree at
`97a4883009bc988fedcdf0416a50a6c62f8d9c9b`. Its foreign dirty source files
were excluded. This is not the original Cognition binary.

A new HOME contains the single installed SDK bundle `devin-inbound`.
Assertions verify zero legacy/community provider plugins and zero helper
packages. Bun is deliberately unavailable. The original native client is
not imported as a package or companion; the fixture supplies the compiled
reconstruction as its executable.

Ordinary GUI adoption writes the host-owned agent binding; ordinary SDK
`devin-native run` launches the reconstruction. The fixture supplies only a
synthetic local credential and sandbox-only workspace trust, not a cached
account status/model response/session. The reconstruction's existing
DEVIN_SERVER_URL test redirect routes its native preflight/inference to the
ordinary gateway. A loopback-only libc guard prevents external DNS/connect
from the reconstructed client. The owned vendor receives selected credentials,
not the local client credential. Its reply produces the first native turn.
The fixture terminates all its processes and removes HOME.

The companion recovered-wire client additionally proves all four account
operations, assignment replay refusal, token-bound native continuation,
session resume/isolation, actual cancellation, disabled-owner refusal and
offline disconnect. That synthetic client is not substituted for the
application startup proof.

## Artifact SHA256

| Artifact | SHA256 |
| --- | --- |
| Ordinary patched host | `bc9550602452ad79ae3c214f1ab62a781d1ba6c4814efd98894b0c69bbaed9fb` |
| Native agent library | `8d7ba5a07dcbc68afaca66ec249883764a2487e9f244d3110e24933d5f4db2f1` |
| Default paired library | `c3022909068d5098ae3b5fa44fe97eb722918609ae3cc3149af131a170face70` |
| Reconstructed application | `8b5438632942381272d20974f7ee59fd9af7674d452dce7ea448077b4c075327` |
| Offline network guard | `c7012e29410d0585e675caef74ceef84216ac0e4e2e6210ea3d8ae863e98279b` |

Evidence on usbox: `/tmp/magpie-nativeops.F9jg4B/`:
`proof-gates-result.json`, `sdk-race.log`, `host-scoped.log`,
`module-race.log`, `node.log`, `bun.log`, `types.log`,
`agent-http/result.json`, `paired-live/result.json`,
`actual-app-startup/{result.json,application.stdout,application.stderr}`,
and `recovered-build-result.json`. The result contains final per-file hashes;
tested formatter output was checked as whitespace-only and applied locally.
All recorded host/module hash entries matched the local sources afterwards.

Earlier fixture failures were the old future-version assertions, C fixture
placement, a fake vendor counter name and reconstructed argv/trust setup.
The metadata 30/31 refusal was a real codec gap and now has a regression.
No failed/interrupted stage is counted as a successful proof.

## Still not full native parity

There is no original Cognition client/live-server proof. Cloud ACP/session
hosting, GetImageCaption/GetDevstralStream, managed-plugin mutations,
partner-provider toggles, attribution and arbitrary group/account delegation
are not implemented by this checkpoint. Source6 integration needs a fresh
fixture run on the union source. The cold CLI model catalog remains separate;
these fixtures explicitly warm the ordinary running host's catalog.

Further community-domain parity work is separate from this frozen bootstrap
checkpoint and must not rewrite its artifact hashes as proof of new code.
