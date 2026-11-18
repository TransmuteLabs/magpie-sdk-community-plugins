# Devin domain completion evidence, 2026-11-15

This records independent module verification and ordinary installed-package
replay on the leader's supplied physical candidate. Host signing/UI, the
final upstream merge and whole-host acceptance remain leader-owned.
Earlier installed documents retain their exact source/library attribution.

## Inputs and authority

- Module baseline: `3bb742022225484e3573d0954df21682566831c1`.
- Public SDK: `7bc5ae211510d0b3fe436da8fa819b7e812729e5`, exported with
  `git archive` from committed `go.mod`, `go.sum` and `pkg`. Archive SHA256:
  `77eec90faf2d0bb3b2218bf29ea6e91516c5e3952c5439e26651473cc8e8fb09`.
- Candidate host: `usbox:/tmp/magpie-upstream-signing-final.QhgGhCd7`.
  Physical tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`,10846 paths;
  source archive SHA256
  `5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc`;
  compiled binary SHA256
  `0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509`.
  Every source file hash and both artifact hashes were verified. A private
  public-SDK copy is byte/mode-identical to the7bc export. The physical tree
  is not parentadcf4ce or a future commit; sourceCommit is null.
- Community main: `57d1cc6a9f49389d495ea9be8d986c70de7767bf`;
  Devin0.1.41, latest package-specific commit
  `191d94bf6ed95ccc4bc62d77e1dd9201a17f5918`.
  Domain source differs from earlier0.1.40 only by the already incorporated
  Gemini delta. The new work closes remaining earlier domain gaps.
- Recovered authority: `ed84daf06cb9eb961a0d6cd7703e39b7b7f90654`.
  No remote is configured. Scoped `git diff` from97a4883 is empty for
  the native wire/client crates, model catalog and relevant descriptors.
  Dirty recovered files and private cassettes are excluded.
- Version evidence remains reconstructed CLI3000.11.228 (053434014dbd)
  and recovered Desktop3.10.48 descriptors. No original Cognition binary
  or live compatibility is claimed.

Read-only source paths that decide behavior:

- Community `index.mjs:140` onwards: suffix/tier recognition, first
  variant at each effort, default selection excluding paid Lightning,
  nearest effort with upward ties, aliases and conservative Free/prices.
- Community `index.mjs:2006`: Messages effort/budget/disabled semantics.
- Community `index.mjs:3070`: quota shares, spans, ACU display/plan end.
- Recovered `models-228.json`: current family/alias names and ordered
  concrete variants. Names classify lookups; they never grant a static
  variant missing from the selected account.
- Recovered `module-39768.js:2337`/`:2366`: enum-only prompt cache options
  and tool definition without a cache field.
- Recovered `module-98224.js:14436` onwards: signed int64 overage micros
  and optional double ACU consumed/limit.

## Actual old-source RED

Evidence root: `usbox:/tmp/magpie-devin-domain.zMciImM8`.
The baseline production inventory is `baseline-production.json`.
New tests run through Go overlays; the baseline production files remain
unchanged. `red-v6.log`, `red-v6-overlay.json` and `red-v6-receipt.json`
contain terminal assertion failures on that exact baseline using the final
test bytes, including:

- `TestProviderDomainCatalogProjectsFamiliesEffortsFastAliasesAndFreeScope`:
  the discovered family/default efforts are absent.
- `TestProviderDomainSelectionUsesCurrentAccountVariantsAndKeepsConcreteUIDs`:
  the family is sent as bare `swe-2`/`account-only`; effort controls are
  refused instead of choosing the account variant.
- `TestProviderDomainAPI9QuotaKeepsNumericAmountsBalanceSpansAndPlanEnd`:
  numeric quota/balance windows disappear.
- `TestProviderDomainFamilyResolutionFailsClosedForCurrentAccount`:
  missing/unreadable current families still dispatch once without a fresh
  selected-account read.
- `TestProviderDomainResolvedFamilyKeepsGeminiSelectedModelOrigin`:
  the family goes as a bare family ID instead of the real UID.
- `TestDomainChatBlockCacheMarksPreserveGrowingPrefixes`: a representable
  block mark is refused.
- `TestDomainMessagesFinalResultCacheMarkKeepsOneRealResult`: the mark
  at the real result's final boundary is refused.
- `TestDomainStatusKeepsSignedMicrosWithoutInventingCurrency` and
  `TestDomainStatusRefusesInexactQuotaNumbers`: negative micros are refused,
  while unrepresentable ACU counts are accepted.

The final baseline RED log SHA256 is
`37eca74199cd25fb29200c617f99b19bbe7fd312b6a2e9589fb7a044a268435f`.
Its receipt SHA256 is
`69daea97ebd13b5d610a4c4812106c7f19ec30a3d001732188b4cc7b5b3635df`;
`productionUnchanged` is true.

| Regression input | SHA256 |
| --- | --- |
| `provider_domain_test.go` | `155e9643edba04892de08b36fa0f66ceb1178e49951309456639c169e551b528` |
| `codec/domain_cache_test.go` | `aea22a3b94fb6b828b4f7037bc81666a363b30afc768c737327583458095690b` |
| `outbound/domain_status_test.go` | `9614c23099ea2440f8e68d3659fe2758e1473127c4e3041e88f32f7319e98071` |

The first attempted runner lacked `ip` on its restricted PATH and did not
run tests. The first quota fixture also used an invalid nonnumeric SDK
revision. Those initial logs are retained, not counted as causal quota
RED. The corrected overlay uses canonical revision19 and reaches the
missing numeric-window assertion.

## Label-only identity RED and correction

The first ordinary installed replay on candidate2 exposed an additional
module regression: upstream `parseConfigFamilies` falls back from family
UID to metadata label or display name, including MODEL_PRIVATE_11's
`Claude Haiku 4.5`. SDK Manifest model IDs, quota NotModels and account tasks
prohibit whitespace, so projecting such a virtual ID invalidated an
otherwise usable catalog and the initial quota read.

`TestProviderDomainLabelOnlyFamiliesKeepConcreteSDKIdentities` ran as an
overlay on immutable `candidate2/module`. It failed at the actual SDK
validation assertion, `display-only family label became an invalid SDK
identity`. `candidate2/label-red.log` SHA256 is
`51a06523334edc65e50fb0df17bc6aacefcacde59edf86682f16fe1eacd6f0d9`;
its receipt is
`204b9b73d5fce62519e1948898218f03b8fe5dc034a3d1dd21496dc1b890b72f`,
with `sourceUnchanged=true`.

The correction keeps the real concrete UID/display name and Free scope
when the family ID cannot satisfy the SDK grammar. It neither invents a
normalized alias nor discards the entire catalog. Candidate3 passes this
regression and the previously failed ordinary replay. No host file changed.

## Absent-limit display RED and correction

`TestProviderDomainAPI9QuotaDoesNotInventAbsentLimit` ran on immutable
candidate3 with a new test overlay. The absent/null cases failed the
actual assertion `absent native limit became a claimed zero limit`;
the explicit-zero control passed. The corrected RED is
`candidate3/quota-limit-red2.log`, SHA256
`89851d5f37c8d4f9b6e0d8eaff043f22c8240e289f4068f39c81c2c5d3713d65`,
receipt SHA256
`c076af3381576fc1f5eb3595e72cb511a47c5ff38bcbd16e019b7c1c16d37a2e`.
The first overlay also incorrectly required a freshness flag for an
all-Unknown window; it is retained separately and is not the zero-case RED.

The final code uses the parsed presence flag to display `unknown` for a
missing/null limit and `0` for an explicit zero. Present Amount and Unknown
percentage semantics stay intact. Candidate4 passes all three cases.

## Implementation and scope

`codec/model_selection.go` implements source family/tier/effort/alias
selection. Concrete UIDs remain available. Family price/Free status covers
the variants selection can actually send; image support remains
conservative across the family. A bounded name index identifies discovered
virtual selections, but stores no variants, credentials or account
authority. Each virtual request reads the current selected catalog before
acceptance. Source-defined names also work before discovery; unknown
explicit IDs retain literal native UID semantics.

The native payload changes dispatch UID without changing selected model,
caller, account generation, receipts, measured usage or rendering identity.
The genuine Gemini continuation regression holds that distinction.

API9 emits exact present counts/limits, signed micros, units/spans and
plan-end resets. Explicit zero differs from absence, and count metadata
does not supply a missing percentage. API7 omits API9 fields. Balance
currency remains unknown; no USD conversion is invented.
The maintained installed quota fixture now asserts actual API9 limit,
Amount, Unit, Display and absence, while retaining its API7 limit100
percentage contract. Both distributions pass through the ordinary host.

Chat ordinary block marks and5m cache controls map to native prompt
boundaries. A final nested result mark becomes the same one result's end
mark. No result is split or fabricated. Tool-definition marks, intermediate
result marks,1h TTL and unordered mixed assistant boundaries still fail
explicitly because their native mapping is unproven.

## Verification record

All commands execute only on usbox with Go1.26.8, `GOMAXPROCS=2`,
`-p=2`, isolated HOME/XDG and pinned offline caches. A per-descendant
seccomp guard denies external connections; its canary and connection
events are retained beside each log.

`green2.log` records the initial focused GREEN. `module2.log` records
the broader suite finding a router-filter ordering regression; the
existing router test required filtering trimmed Adaptive/Fusion before
validating family metadata. That order is restored. Final focused,
module/race/static/build and installed results use `candidate4`, whose
runtime inputs are pinned separately from later documentation and fixture
metadata changes.

Evidence index:
`usbox:/tmp/magpie-devin-domain.zMciImM8/candidate4/domain-evidence.json`,
SHA256 `1b8392da15c07339f33de54d1b4708c8125f5bdd18fedfa1a9e5c049121c172d`.
It records every gate receipt/log/network hash, immutable RED identities,
runtime input hashes and exact libraries. Runtime input SHA256 is
`688c6f1a939b7826328de724f37526a96b34cd41b6ad33a6f200972e62515c81`.
Candidate4 snapshot identity is
`d740a42c14365a6c36897e60c9644ea927eee91fae2320c1fd306f0161761ce6`;
the archive SHA256 is
`725f82c680360b8f8ccd84daac5729e3ba0317f460df32e56329ad22351d1aa9`.
The SDK snapshot identity is
`da8861971bc4f266c6e091b1b5754a97492321eed2513ea836515569cdb0a95f`.

| Gate | Terminal result |
| --- | --- |
| `format`, `module` | All Go sources formatted; full `go test -mod=readonly -p=2 -count=1 -timeout=120s ./...` passes |
| `race`, `vet` | Full race suite count3 and `go vet -mod=readonly -p=2 ./...` pass |
| `python` |18 provenance/distribution and sampling-fixture regressions pass |
| `build`, `build-paired`, `build-inbound` | Full0.8/API9, paired0.5/API7 and inbound0.2/API1.1 native libraries build offline |
| `candidate2/donor2` |46 unmodified donor usage/Free/prices/images/cache-mark tests pass; no donor installed or dependencies added |
| `native-options` | Typed decision catalog and default artifact's API9 native options pass |
| `paired` |24 real native requests; catalog persistence/restrictions, origins, caller/generation, raw/canonical Chat, invalid completion and cancellation controls pass |
| `agent` | Ordinary one-package adoption/launch, finite bootstrap/journal, cold catalog selection, native session/resume and disable controls pass |
| `gemini` | Genuine donor-captured signature/tool continuation passes Chat/Messages JSON/SSE |
| `identity`, `transitions` | Duplicate/no-op/held reads retain authority; real model changes fence stale reads/writes without retry, with accepted-write journal completion preserved |
| `messages` | Ordinary caller-derived signature continuation, streaming and cancellation pass |
| `history`, `facts` | Existing history/image/price/usage controls plus API9 numeric quota and measured token/cache/Facts/diagnostic attribution pass |
| `quota-api7` | Paired compatibility artifact retains percentage-only ACU Aside with explicit zero/absence and no inference dispatch |
| `conformance` | Codec-only ABI in two concurrent native sessions, installed decode and native HTTP504 deadline pass |
| `paid-hot` | Strict hot-config-CAS paid task: both real arrivals, preserved template, budget/model/readOnly controls, cancel/join and exactly two measured ACU rows pass |

The full library SHA256 is
`af90f1d3fdff103afdf8ef46740aac6c7e0b85f134e1754e8e6c14730a5623e0`;
paired is
`8ff0dcad3a68bca59a22ef6f8fd1089602e62604f8d71f3018b118cd4905b4d1`;
inbound is
`83eb0321a05e988e8ace320edc50719e2d53d0bf136830b56d687b12f9b91ac6`.
Installed fixtures verify the actual mapped library. Each successful
network report contains only the deliberate external-denial canary; all
test traffic stays synthetic. Fixture homes are removed and owned
processes stopped.

Earlier failed preparations remain available: candidate1 included
AppleDouble metadata, the first donor invocation violated its HOME/TMPDIR
sandbox precondition, and the old codec-only conformance runner was
misapplied to the full artifact. The latter checks canonical-only
max_newlines/top_k refusal, while the full artifact preserves native bytes;
the unchanged runner passes on its inbound-only distribution. A temporary
describe-default normalization probe is retained outside the repository.
The initial full history runner also expected the old API7 limit100; its
API9-aware quota assertions pass in `history` with a separate API7
control. These failed trials are not counted as acceptance or causal domain
RED. No retries or host production edits were used to turn them green.

The final successor host, parent broad gates, original Cognition client,
live model availability, cache lifetime/charges and non-Linux distributions
remain outside this proof. Donor tests also cover static fallback and
manufactured results that this module deliberately refuses; passing the
donor source oracle does not imply those behaviors were copied.
