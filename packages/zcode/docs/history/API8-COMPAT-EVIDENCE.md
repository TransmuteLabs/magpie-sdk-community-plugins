# ZCode full package: bounded API8 compatibility checkpoint

2026-11-14. The 52-file donor slice is committed as
`1eff0f09128c1f0f5223b70d15241f1ac4e1df9a`. The preceding native
log/SQLite fidelity and full-package commits are `371b06c` and `6470897`.
This continuation changes documentation only. No host, other module,
dependency, branch, worktree, installed client or live account was changed.
No publication or push occurred. SDK-only deployment remains independent.

## Package and exact tested sources

The ordinary package remains full **0.11.0**, provider/controller/owned
agent/UI together, one native library and static assets. Its existing SDK
floor is 1.5, ABI 1. The test installs exactly one module; no community
runtime, second install or alternative agent/window build is required.

Canonical package:
`dist/zcode-0.11.0-devzeus9e88/magpie-plugin_linux_amd64.zip`.

| Identity | Exact value |
| --- | --- |
| Canonical ZIP SHA-256 | `bb2f42924cb5a2ee6f3edb38fbda7fa6794b9d9e41c7a6674b9244aaf1918cd6` |
| Canonical native library SHA-256 | `a0f998da32228c7cf6060f9b1fcd3b05d0263cbdbd52db8928bc35a4f4d688d8` |
| Committed release verification host | `9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, SDK 1.6 |
| API8 snapshot base tree | `4f6525135603b84cdc552403923f39b44fa1dea4` |
| API8 working-source inventory | 10530 files; SHA-256 `5bcb3c646fab311f323dd5b545655732e05dd5ca1094266a66c032533efba03b` |
| API8 source archive SHA-256 | `ce9f8b8d57d8ed78cee727cdcd3dd4345ee007bfa68472d7f50044594166ec9b` |
| API8 host binary SHA-256 | `cb976081ea4f862da8591ea9d0350c62aea04f16b9e52ca2074441176e90f4f0` |
| Tested module source inventory | 153 files; SHA-256 `b7745fa604849fd1545ec31a1b761c050a48c1e2f3cd7f97b8e8b44f8faea829` |
| Aggregated API8 evidence SHA-256 | `dc3526d905ea67f57d43a5f4dc0a8b8c2a1d33cca5f04cb6d820c6c6316f8c6b` |

The API8 source was still an uncommitted working tree on that base.
Two complete local inventory passes matched before transfer, and all
remote host bytes/modes and module source bytes matched after acceptance.
This identifies immutable tested bytes without declaring a committed API8
release freeze. `source-lock.json` and the canonical ZIP retain the
committed 9e88 release baseline. Nested verifier `hostCommit` fields come
from that release lock; `api8-evidence.json.hostSnapshot` identifies the
actual API8 compatibility source.

Verification root: `usbox:/tmp/magpie-zcode-api8-jZhLsR1i`.
Local companion evidence: `dist/api8-compat-pchrad3k/`, including
`host-snapshot.json`, `host-inventory.json`, `api8-module-source.json`,
`api8-summary.json`, `api8-evidence.json` and `verify-package.py`.
The aggregate contains 10 reports and hashes of 27 preserved logs.

The unchanged lock and fresh provenance gate verify 30 vendor files at
`fec975905a2abd4c07e35c40d51fa097a2258ec9`: desktop **3.18.2**,
CLI **0.17.1**. Five community donor files and package **0.1.61** are
checked at `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`. These are
source/provenance checks; the actual client executable remains absent.

## Acceptance and preserved failures

All builds/tests ran on usbox, Go 1.26.8, private HOME/XDG, pinned offline
caches, finite timeouts and loopback-only namespaces. UID/EUID were 1000
and all five capability masks zero. No broad host `./...` suite ran.

| API8 compatibility gate | Result |
| --- | --- |
| Scoped host `nogui` build | Pass |
| Full module race regression, once | 385 pass events, zero failures/skips |
| Module gofmt, vet and Python syntax | Pass |
| Five native scenarios: lifecycle, China auth, controls, controller/MCP/history, packaged production UI | All exit 0 |
| Unchanged canonical ZIP: ABI, add/list/inspect, synthetic legacy import/models, off | Pass; one installed module, ZIP/library unchanged |
| Strict TypeScript against captured API8 declarations | Pass |
| Packaged Node/Bun UI | 31/31 each |
| Agent ownership/config and actual forwarded-header causal negatives | Four compiled assertion failures as expected |
| Native SQLite rollback | 60 fresh race processes, 120 pass events, zero failures/skips |
| Exact desktop/CLI/community provenance | Pass |

Endpoint-only native fixture overlays compile against the captured API8
SDK and dispatch to loopback vendors. Separately, the original packaged
binary passes host installation, synthetic legacy import and model reads.
The actual client launch remains synthetic. The full 9e88 donor acceptance,
21 domain/data causal mutations and page-confirmation negative remain
recorded in [COMPLETION-EVIDENCE.md](COMPLETION-EVIDENCE.md); this
compatibility run does not relabel them as fresh API8 runs.

The lifecycle's initial forged-header RED was intercepted by the host's
default header filter. The fixture now enables the existing PassHeaders
setting before requiring strict module refusal. Removing the module
guard fails that real lifecycle assertion on API8 as well. Native root
ownership, admission checks and refusal assertions were preserved.

SQLite cancellation restores the exact binary preimage; stale preimages
prevent writes; foreign rollback-time data remains with ErrActionChanged
and the prepared journal. Each scenario ran in 20 separate processes.
The fixture SHA-256 remains
`2700c35785974f55f847830e7f86aeafa33b25415d72d8f767143e3d78392d5b`.
The scoped owner fixture verified all 10530 copied host files unchanged,
content inventory `89ebc4d25a1c3b69e31dc3f2ca26c2f3d9ee6957e6647ae3a2df1958d3dff3db`.

An earlier API8 snapshot is preserved at
`usbox:/tmp/magpie-zcode-api8-1NqMlrOO` and locally in
`dist/api8-compat-qlfhecr7/`. Its inventory was
`1672e1551cebda497ad0da687d9f0a9a554f12b046c03721f3028a7b2f97c432`.
The module race suite passed 385 events, but the host failed compilation:
`outsideRead` was declared in Manager.Call and consumed in Lease.Call.
The shared checkout subsequently placed the declaration/decoding in
Lease.Call; a newly captured snapshot built successfully without a module
or host edit by this executor. That compiler failure is preserved and is
not behavior-negative evidence. Older SQLite loader refusal, fixture
failures and the two unattributed `/` billing requests remain recorded;
no claim is made that the latter's cause has been fixed.

## Final module review and pending gates

Review found no additional module code fix required by these compatibility
gates. The package reuses the existing native SDK owners, codecs and
locked SQLite dependencies. No cleanup removed evidence or changed runtime
behavior. Historical untracked `AUTH-WEB-EVIDENCE.md` remains unchanged:
SHA-256 `b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.

SQLite/history/log support accepts explicitly supplied immutable data.
Start 3.18.2 shaping, gifts/free-first/dual Trial, claim/check-in/feed
outcomes, remote MCP and four legacy execution-usage counters are covered
by the bounded donor slice. This is not whole-plugin/live parity.

- Actual-client startup/resume and a model-bound stdio/control adapter
  remain module work. SDK NativeAccountOperations already exists.
- Automatic coherent DB/WAL capture, diagnostic collection/consent/
  rotation/revocation, financial import/resume and custom-root Library
  placement remain open. Supplied API8 StorageSource/read-lease contracts
  are not yet consumed and do not prove those behaviors.
- API7 sparse Usage.Facts and diagnostics.emit are supplied in the API8
  source but not adopted by this package. Richer reasoning/cache-1h/
  cost/credit fact projection and structured operation diagnostics remain
  pending. Native log preservation is not automatic telemetry.
- Explicit check-in works; automatic daily scheduling and richer quota-card
  projection remain separate gates. The scheduling ADR is a proposal.
- Live vendor/captcha, a rendered browser, actual ZCode and native
  Darwin/Windows runtime acceptance have not run.

Independent parent catalog/ledger/usage review is a subsequent read-only
scope against the owner's identified snapshot. This checkpoint introduces
no parent change or new ZCode feature scope.
