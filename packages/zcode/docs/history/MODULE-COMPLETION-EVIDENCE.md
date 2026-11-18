# Source-proven module completion evidence

2026-11-15. Writable repository:
`magpie-sdk-plugin-protocol-zcode`, existing main at
`687af12d52871e4d9995f9548639b005e68104e7` before this slice.
Provider/controller/owned agent/UI remain one ordinary 0.14.0 native
package, SDK 1.9 / ABI 1. The optional historical headless surface is 0.7.3.
No host, donor, installed app, live account or credential source was edited.
No new dependency, branch/worktree/stash, push or publication was used.

## Authority and changes

Fresh safe official fetch preserves the clean fec9759 checkout and resolves
`origin/main` to `fec975905a2abd4c07e35c40d51fa097a2258ec9`,
desktop 3.18.2 / CLI 0.17.1. Community authority
`57d1cc6a9f49389d495ea9be8d986c70de7767bf` has ZCode subtree
`d1be84b45f19c7d51bf3bf1c428ddc7ae95dfbf7`, identical to ported 0.1.66
at `87a8cf03c62fa55ca845c51648e8859ece580336`. Source hashes and historical
versus current audit roles are preserved in source-lock.json.

The pre-edit cleanup/regression plan is MODULE-COMPLETION-PLAN.md.

- `agent.go` deletes static suffix-based model inference. Native profile
  facts now come from effective API9 context/output/efforts/defaults,
  including routing groups and unknown versus confirmed reasonless.
  Previous typed catalog facts permit replacing/removing only matching
  generated fields. Manual rules, user changes and disconnect restoration
  stay protected. Unavailable explicit user effort fails before a plan.
- `reset.go` accepts only the official opportunity envelopes: code 0 plus
  granted true, or code 3301 plus granted false and positive integer
  next_try_at. Code 0/granted false remains uncertain. Official reset use
  still requires used true. No speculative confirmed-unused path was added.
- New selected-account actions expose a sanitized reset summary, explicit
  history reconciliation and a separately acknowledged opportunity
  uncertainty resolution. Reconciliation never reposts use/opportunity;
  unknown debt stays until explicit acknowledgment, across restart and
  generation changes. Opportunity resolution retains the cooldown.
- The ordinary page now has thirteen fixed bindings, with selected exact
  string generation, confirmation, single-flight writes, auth locking and
  disposal. Summary contains no journal key/operation/generation, private
  vendor fields or credentials. Actual native routes exercise two
  synthetic accounts, debt isolation and one global history acknowledgment.
- NATIVE-CONTRACT-REQUEST.md and `testdata/native-contract/` give the
  leader a ranked finite resume/process request with official examples and
  a reusable synthetic stdio peer. Its seven self-tests establish fixture
  behavior, not a working SDK V4 attachment.

## Initial isolated module verification

Remote evidence root:
`usbox:/tmp/zcode-completion-20261115-fwTTotBP`.
`verified/` contains successful results; `red-fixed/` contains corrected,
compiled old-implementation REDs; `ui-red-fixed/` contains the old UI RED.
Initial draft/infra failures remain in the earlier evidence directories.
The initial RED's indexing panic and initial ingress PATH failure are not
presented as causal failures. Corrected old-code assertion failures and the
four unchanged ingress boundary tests were run afterward.

All builds/tests ran on usbox, new HOME/XDG per stage, pinned offline caches,
GOPROXY/GOSUMDB off, GOTOOLCHAIN local, GOMAXPROCS 2 and `-p=2`.
`testdata/agent/verify.py:isolate()` gives a new user/network namespace,
loopback only, zero capabilities and no regain of privilege.
Go is `/tmp/magpie-sdk-v11-usbox-niq306rt/go-toolchain-1.26.8/bin/go`;
caches come from its pinned `go-env-external-toolchain.json`.

| Verification | Result |
| --- | --- |
| Focused root agent/reset/control/provenance `-race -count=20` | 1460 PASS, 0 FAIL, 0 SKIP, no race warning |
| Entire module root package, `go test -race -count=1 .` | 421 PASS, 0 FAIL, 0 SKIP; no host/broad suite |
| Strict TypeScript, production IIFE build | PASS |
| Node / Bun UI tests | 34 PASS each |
| gofmt, `go vet -p=2 ./...` | PASS |
| Darwin arm64 / Windows amd64 root vet and CGO0 build | PASS; compile/static support, not native runtime |
| Unchanged ingress ownership boundary tests | 4 PASS |
| Ordinary Linux native ZIP / ABI conformance | PASS; two sessions, five calls each, no outbound network callbacks |
| Actual host native reset page routes | PASS; synthetic loopback endpoints only |

This initial module run uses the earlier immutable SDK source
`09cfa50eda8d2dbf4c5003bfeff6bf865319bc0a`, archive
`afc87d8eb72e49995a0606f44ffa7f578ca97aaffbda54ec9115efb99d6b4689`,
parent `a61777e3fb491c843142947dfddd0db9fd0584b6`.
Host binary SHA256 is
`a5f0f67317f090b2299d7ceca741e712ac3bc0f64e50a4256344bc2702f4f403`.
The earlier built 0.14 ZIP SHA256 is
`8190592415eaf43efb27d5ddd5e93f48f33c27858fdf1a22eecc6b7822b101c9`;
later provenance/contract packaging has a separate identity.

## Negative and causal evidence

Old production bytes were preserved in `baseline/`:
agent.go SHA256 `f66d8c509e8defa7821d470a353fc701a1863eb81c9bd6f375a5e4b660dcf825`;
reset.go SHA256 `bce16ec711a3bec002373835c433560fa6e880dbdd2c8ab9da083b651740e3b1`.
New tests against those bytes compiled and produced 12 FAIL events
(including parents), 6 PASS, no skips or build error:

- `TestAgentMetadataUsesEffectiveGroupLimitsAndDefault`: effective group
  output 2048 became static 128000 and low/disabled became low/high/max.
- `TestAgentMetadataDistinguishesUnknownAndReasonless/{unknown,reasonless}`:
  unknown effective output inherited a static vendor limit.
- `TestAgentMetadataSyncReplacesGeneratedFieldsAcrossCatalogs`:
  changed effective catalog did not update generated metadata.
- `TestAgentMetadataRejectsMalformedPreviousCatalog`:
  null previous catalog granted generated-field authority.
- `TestResetOpportunityAcceptsOnlyOfficialEnvelopes/zero-false`:
  unproven envelope cleared durable uncertainty.
- `TestResetControlsReconcileAcceptedUseAfterRestartWithoutReposting` and
  `TestResetControlsKeepUnknownDebtUntilSeparateExplicitResolution/{use,opportunity}`:
  new named actions were unowned, so debt reconciliation/recovery was absent.

Old UI produced 31 PASS / 3 assertion FAIL:
`reset summary and explicitly confirmed recovery use exact selected generation`;
`reset recovery confirmation blocks concurrent writes and disposal prevents dispatch`;
`declined reset recovery and stale selected generations never dispatch`.

Four independently compiled Go overlays all fail named assertions:

| Removed guarantee | Named failing regression |
| --- | --- |
| Effective metadata replaced with previous-only facts | `TestAgentMetadataUsesEffectiveGroupLimitsAndDefault` |
| Accept code 0/granted false | `TestResetOpportunityAcceptsOnlyOfficialEnvelopes/zero-false` |
| Drop pending.Accepted history fence | `TestResetControlsKeepUnknownDebtUntilSeparateExplicitResolution/use` |
| Return raw reset status in summary | `TestResetControlsReconcileAcceptedUseAfterRestartWithoutReposting` |

A compiled-JS confirmation bypass produces 9 PASS / 3 assertion FAIL.
No mutant modifies production source; Go uses overlays and JS mutates only
temporary compiled output.

| Evidence log | SHA256 |
| --- | --- |
| Old compiled Go RED | `726c6e14ce1bf81db76a81212a57cb41103d43a474578a25299975052fe6255a` |
| Focused race20 GREEN | `9d85751faa21957238a978c95bee5b99d4f7048787dea98b625d4d694510b5ed` |
| Root package GREEN | `84db4adbc1b4ca1573748595215180d6eb2daf3762105f0c1b131a32c42157f7` |
| Causal effective metadata | `4c188e697f682751de882b8fbc8f6e462d1111efb86668f39a213484a85be9ca` |
| Causal opportunity envelope | `86fcf472bf2f1eee27a9332b428523db74745b3557c6c041635566f9c463c48e` |
| Causal unknown debt | `94a273a388cc1baecc1edfa76e98534a163be958bbae8e2a37bee00c1cf5de37` |
| Causal private reset summary | `8db89c6a53bb039222cf7c5d57682e9c608502e17ca6ae2b475980f5c7c7f789` |
| Causal UI confirmation | `a942d9ae9cc7209d9c13c7412b7045ff9bd2f4109224661b00cf2c6cfeee12c2` |

## Candidate installed-package replay

The leader's read-only candidate is
`usbox:/tmp/magpie-upstream-signing-final.QhgGhCd7/source`,
physical tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, 10846 paths.
Archive SHA256:
`5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc`.
Inventory SHA256:
`6f58e53b6d383dc78018b9ed1437663d042186d07d971b40459aaede1b6d3ee8`.
Candidate binary SHA256:
`0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509`.
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d` is only the parent at capture,
not this physical tree's merge commit or a completed parent acceptance gate.

The isolated module freeze/replay root is
`usbox:/tmp/zcode-installed-candidate-20261115-nxL5k0HT`.
Its 210-path module archive SHA256 is
`d5cb3053f0eb848ae506bd647a88477d8950141170ce8700d1cbf44ced4c2923`.
This freeze precedes this evidence-only document; executable source identities
are separately inventoried. `proof/` records the candidate verification.

All 10846 candidate source file hashes were checked against the supplied
inventory before replay. No host source or binary was rebuilt or modified.

| Existing ordinary owner path | Candidate result and log SHA256 |
| --- | --- |
| Installed agent/profile/Library/gateway/native aliases | PASS, `4fa4807d7730f2e0f54a2021c281e17666552d513fb0c01b8a463927d019e24b` |
| Headless auth/China Start/manual JSONL and binary SQLite backup/restore | PASS, `8591b102c594d1a23c8daee011630d3686655eafc9be459ed69e9e1744fec8cd` |
| Actual selected-account reset/check-in page routes | PASS, `482c13b1a0715d1391560faa90111110494052298be00390ed298a1ad5fe94d5` |
| Controller/refresh/barrier/tools/MCP/history | PASS, `e30f16343db65cb524934dde981f34fb82b58ecdeedcd6d2bde110d84872a676` |
| Production UI-auth IIFE/bridge/parent HTTP/native store | PASS after fixture correction, `5c9c85f8e40b1d39d88092257cad1a18d116858c90a35a520212eceb6ecbfcd8` |

The first UI-auth replay failed `Error: initial account enumeration`, with
zero vendor starts/polls. Its preserved log SHA256 is
`f8d1f4979a4045affb485448a6a11f67a4fc7316719cf3a30671a097a0de5961`.
The VM fixture in `web/auth-http.mjs` omitted the five newly required reset
control elements. The actual HTML and page/app unit fixtures already had
them. Supplying those same five fixture IDs restores enumeration; no
production behavior or host code was changed for this correction.

The corrected immutable module freeze is
`usbox:/tmp/zcode-installed-candidate-fixed-20261115-ALGswE9x`,
211 paths, archive SHA256
`069131e95ccd94f610a782cfc60b9a22ddf6d2ec2dc46507df8967f5d3f0fbe4`.
On that candidate source, the same focused race20 gate passes all **1460**
events, no FAIL/SKIP/race warning; log SHA256
`119d4ca6a06c443e6c81de00324501cb8f9c452fe83d0ce54f94a8187fd62b41`.
gofmt, module vet, Darwin/Windows root vet/build and the four unchanged
ingress boundary tests pass again. Only the affected UI-auth owner replay
was repeated after the fixture change; the earlier four green owner paths
were preserved. No host or competing broad suite was launched.

The corrected ordinary ZIP SHA256 is
`966c3742b3a55c059078161e8fe75718f6cb5c5c1df2cfd5737ec378cd4192d3`.
Its production library is byte-identical to the first candidate package:
`ef3a166cee22a58e7df18160f1241b76514131bdbac5d944d850ec55e32c532c`.
The actual production IIFE tested in the successful UI-auth replay has
SHA256 `b3ff19421c995b49e488f293b40afb2028cc02247eee26203c432affe92477b0`.
Build-generated bundle comments can change with the temporary build path;
separate ZIP/bundle identities are retained instead of claiming identical
bytes between independent builds. The corrected ZIP's ABI conformance passes.

The final reviewable handoff ZIP is
`usbox:/tmp/zcode-installed-candidate-fixed-20261115-ALGswE9x/handoff/magpie-plugin_linux_amd64.zip`,
SHA256 `502cb8c7109668ea947e374eb7e8bb68d299fe60e33ef2d20719938724780b97`.
It reuses every tested production payload byte and changes only the two
documentation sidecars `LEADER-CONTRACTS.md` and
`NATIVE-CONTRACT-REQUEST.md`, for the precise task lifecycle distinction and
checked source line references. Native library and IIFE hashes remain those
above. A fresh isolated ABI conformance run passes; log SHA256
`7e4e95045e8b881b8fd3e2164aea9449a7c31f3e59e86c61011a7964c2c34b90`.
This is a documentation repack, not a claim that the new host contracts exist.

Local preserved artifacts are under
`dist/completion-20261115-mngiygzz/`; `handoff/` holds that final ZIP and
its payload/hash comparison. `lineage-evidence.tar.gz` preserves 99 source,
overlay, inventory and log files from the initial RED/GREEN/mutant and both
candidate freezes. Archive SHA256:
`73c84c9addd477585b63c8385b99722f35d0ad2488f2e832e81b9ee64806e02d`.
The original usbox evidence and previous local distribution files remain.
After the corrected test freeze, only LEADER-CONTRACTS.md,
NATIVE-CONTRACT-REQUEST.md, SQLITE-INTEGRATION-PLAN.md and this evidence
document change; executable/test source remains the verified bytes.

Seven synthetic stdio-peer self-tests pass under the same guard:
`contract-peer.log` SHA256
`e75c617a702bf00f6ef0db433b460b5e362952203b959e74acfff833c6bbbc7c`.
They cover literal argv/cwd, pending send/reverse reply/stop/query, admitted
input duplication, foreign reply, loss after ACK, split UTF-8/15 MiB final
EOF frame, and malformed output. They do not exercise a real V4 SDK host
adapter or claim initial snapshot/financial execution projection.

## Exact changed files

| Area | Files |
| --- | --- |
| Native profile and regressions | `agent.go`, `agent_test.go`, `agent_metadata_test.go` |
| Reset/actions and regressions | `reset.go`, `reset_control_test.go`, `module.go` |
| Owned package/page bindings and native route fixture | `window.go`, `window_test.go`, `ui_e2e.py`, `build.py` |
| Page and UI fixtures | `web/control.ts`, `web/page.ts`, `web/index.html`, `web/package.json`, `web/page.test.mjs`, `web/app.test.mjs`, `web/auth-http.mjs` |
| Provenance | `source-lock.json`, `sources_test.go`, `SOURCES.md`, `README.md` |
| Plan/evidence/current leader handoff | `MODULE-COMPLETION-PLAN.md`, `MODULE-COMPLETION-EVIDENCE.md`, `NATIVE-CONTRACT-REQUEST.md`, `LEADER-CONTRACTS.md`, `SQLITE-INTEGRATION-PLAN.md` |
| Isolated runner and reusable protocol peer | `testdata/completion/verify.py`, `testdata/native-contract/peer.py`, `testdata/native-contract/peer_test.py`, `testdata/native-contract/wire.json` |

## Limits and remaining essential work

NATIVE-CONTRACT-REQUEST.md ranks the missing generic argv resume recipe and
owned duplex process lease, with official launch/stdio/frame/operation
semantics and bounded owner acceptance cases. Existing public
NativeAccountOperations is finite HTTP, not the actual V4 process path.
No actual V4 host attachment or live financial resume is claimed.
Automatic SQLite source adoption is module work using available storage
contracts, not a new SDK blocker. Current manual conversion is not automatic
history. Foreground-bound account tasks already exist; perpetual calendar
work and automatic diagnostics are optional separate surfaces.

Withdrawn socket authority remains withdrawn. `testdata/agent/ingress.py`,
its tests and the archived browser RED cassette remain unchanged from
687af12. Unknown/vanished ownership fails closed. No cause is assigned to
historical extra TCP arrivals. A cached tuple, name, JSONL, PID/startTicks or
offline history does not authorize a new process/TCP incarnation.

No live vendor/account/native app, rendered-browser acceptance, Darwin/Windows
runtime, automatic source adoption, financial import or final merged-host
acceptance is implied. The leader's SDK-only first deployment is independent.
