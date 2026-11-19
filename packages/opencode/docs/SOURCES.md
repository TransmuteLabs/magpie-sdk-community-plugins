# OpenCode module source provenance

OpenCode **0.8.1** uses public SDK API **1.12** and native ABI **1**.
The source review advances OpenCode `2.3.0` from `d33b578333` to
`011f9043e8a90dc9a1ac42fb1181486dd4d13b71`. The 25 commits change
260 paths and leave the 17 extraction paths unchanged. The module applies
the exact HTTP refusals added in `bca1def93b` and `a8dbca1065` through
[host request policy](REQUEST-POLICY.md) and reports a matching Responses
stream refusal while preserving the accepted fence. The unattended Console
watch is an OpenCode session-wakeup policy and stays with the harness.

Community `836f2e4e4cd899318c422d67cc1b2b30baaf1e06` retains the exact
Zen Free `0.1.28` tree `84f1e08de8e6b72ea3ecfe1ccc324476280ab287`.
Magpie upstream `99cc8fa5d` was checked for OpenCode routes, free-model
payloads and agent wiring. Go Chat and Gemini already use their native
endpoints and keep the caller's body. The Zen module uses the public SDK's
proxy resolver and its own Go HTTP transport. The upstream built-in's Bun
fetch for Zen Free is a private host transport hook. Automatic model
discovery on OpenCode startup and separate agent `limit.input` are
upstream built-in behavior. The module's current config plan writes the
model snapshot on connect/sync and receives context/output from the SDK.
These differences and the exact source refs are in
`officialProviderFollowUp20261119` in `source-lock.json`.

The November 19 source review covers `d33b5783338b8b350aafc09f7dc2701eeba52a9a`
at CLI version `2.3.0`, 33 commits and 305 changed paths since `79bd2c6f`.
Of the 17 extraction paths, `packages/core/src/model-resolver.ts` changed
to pass configured timeouts and TLS to media and decision models. OpenCode
continues to own that behavior. The new model-list contract in
`services/site/src/docs/content/console/api/models.mdx` documents the
metadata used for Zen discovery. Zen removes GLM 5, 5.1 and 5.2 from
bootstrap and live lists, including stale responses. Models such as
Gemini 4 Neon and Tidepool Alpha Free can appear when the live list
provides a supported native endpoint and complete limits.
Go reads its selected-account inference list and admits Chat and Gemini
entries whose exact native routes match
`packages/core/src/models-dev/snapshot.txt` and the endpoint contract.
Messages and Responses Go entries stay out of the module's catalog. The
packaged Go fallback retains its original donor and model metadata.
`officialCatalogFollowUp20261119` records the exact source commits.
The documented rich response shape is exercised with loopback fixtures.

The November 18 follow-up advances the local OpenCode checkout from
`521ceac004e59fbbfca0929596107cd8430f2b26` to
`79bd2c6f11fd5ad011dbe5a1c31343c46e1e5294`. The three commits keep CLI
version `2.3.0` and leave all 17 pinned OpenCode paths and provider
protocol implementations unchanged. They change the harness's GUI plugin
icons, goal indicator and macOS file permission matching. The module
continues to let OpenCode handle these features. This comparison did not
execute the updated client.

On November 18, the local OpenCode checkout advanced to
`521ceac004e59fbbfca0929596107cd8430f2b26` at version `2.3.0`.
The 11 commits change 41 paths. All 17 pinned source paths and the
Responses protocol files keep their bytes. OpenCode removes
`glm-5.3-flash-free` from preferred models and records its retirement
in `services/site/src/docs/content/console/models.mdx`. Both Zen
catalogs now apply that retirement during bootstrap and discovery,
including a stale vendor listing. The Console artifact reads, video
jobs and MCP event changes run in the OpenCode harness.

The follow-up through `63bfe62c91fb069c97fcdd7a721aa8f7291cbf49` covers five
commits and 63 changed paths. The CLI remains `2.3.0`. OpenCode now puts
the system prompt in a leading developer message on every Responses route,
including standalone `/compact`. The harness drops echoed developer items
from the compacted replacement. The module's generated Magpie provider uses
`@ai-sdk/openai-compatible` for Chat Completions. The generic Magpie Responses
generation route reads developer items. Standalone `/v1/responses/compact`
requires a gateway route that returns one `response.compaction` JSON
object with an encrypted checkpoint. The existing Codex compaction
handler returns SSE, and the generated Chat profile uses neither
Responses path. All 17 pinned OpenCode source paths retain their bytes. See
`responsesCompactionFollowUp20261117` in the source lock.

The November 17 comparison through `7ca7b3feba47a4f43e6a9ee3fbe10067855ddb61`
covers one commit and 20 paths at version `2.3.0`. The harness sends the
Responses system prompt as a leading developer message during generation.
Explicit compaction keeps the prompt in `instructions`. Magpie's Responses
parser already accepts developer messages with string or text-part content.
Provider authentication, agent configuration and persisted history are
unchanged in this comparison. `responsesHarnessFollowUp20261117` records
the source check. Live client and cache verification remain pending.

The next November 17 comparison covers `ea79f69b` through
`9cfff5f24df3664f7785bb995cd62afafd878976`. The four commits change 22 paths
and keep version `2.3.0`. The pinned provider, authentication, configuration
and storage inputs are byte-identical. The delta affects native Code Mode
extension loading, `CLAUDE.md` discovery flags and the built-in `webfetch`
result. `officialHarnessFollowUp20261117` records the review. The plugin
keeps its existing runtime and extraction pins.

The November 17 follow-up compares `db1d7b9120fb899dc1d471a93e8bfad1e6dfbd98`
with `ea79f69bcf566aa385570814f8624a5c4fa72d7e`. The 53 commits change 560
paths and preserve CLI version `2.3.0`. Of the 17 pinned paths, only
`packages/core/src/plugin/provider/opencode.ts` changes. Its delta registers
the native Console artifact publisher when the Console connection advertises
`x-opencode-artifacts`. Native instruction discovery also gains a Claude
fallback. The SQLite schema, stored history readers, credential exchange
and generated provider config inputs retain their source bytes. The plugin
leaves those native features with OpenCode. `officialSourceFollowUp20261117`
records the comparison. Native client execution remains pending.

The [November 17 Zen 0.1.28 port](ZEN-028-PORT-20261117.md) records a
supplemental DSH tool-normalization change from community
`324acd6007f2d513c5072fe2e9fe1db3524ae03d`. Author old-red/new-green,
race20, affected interop, vet and formatting gates are terminal. Independent
bounded source-and-evidence closure is approved, with terminal receipt SHA-256
`04ee243cc376da83426e04f953272104a6b2bf1690ba5005fe2195b89d0a0ec6`;
the linked port record identifies the exact reviewed seal and report.
This is not fresh native-harness or aggregate acceptance.
This port's original 0.1.27 extraction and 0.8.0 package records remain historical.
The current minimum 1.12 is an API revision, not the public SDK release;
the public SDK release follows app `VERSION`, with the same Go module/tag.
Wire 1.12, codec 0.5 and native ABI 1 remain separate identifiers.

The separate official OpenCode source comparison on November 17 covers
reviewed `c5e741e787594e8c18fa70ab3a4705db1c927793` through fresh
`db1d7b9120fb899dc1d471a93e8bfad1e6dfbd98`: 73 commits/1118 paths,
still CLI version 2.3.0. The two changed literal watched paths are CLI package
metadata and `model-resolver.ts`'s native video resolver. Root's broader
auth/provider/config/history review identified no required module port.
Independent bounded source review `83cc7eaf21474b1de3edff88f292e6860bbcf9f4b38a0094868c67d34c74974f`
agrees on those reviewed inputs, bound to original `zen_tools.go`
`1c500ffdd1422ea4815eaa25336d8806e0fd651b93f5a8f25c97923744d43c04`.
That approval excludes the separately authored Zen 0.1.28 delta and does not
prove fresh native-harness execution. `officialSourceComparison20261117` in
the lock preserves this distinction without replacing the original pins.

The [November16 closure](LOCAL-CLOSURE-20261116.md) supersedes the original
verification checkpoint below. `currentVerification` in the source lock names
the strict public helper used for the common-host closure. Original OBS and
donor extraction identities remain historical evidence.

The current unpublished request-policy adoption requires public API1.12 in all
rebuilt variants; existing package version strings and donor records stay
unchanged. [REQUEST-POLICY.md](REQUEST-POLICY.md) owns current proof, scope and
no-recovery boundaries. The lock's `requestPolicyAdoption` records original OBS
archive/manifest and stable public helper hashes plus literal official/donor
error sources. Verification uses an exact extraction of that original archive:
the shared candidate directory had a drifted transport file and was not patched.
Current receipts are in `REQUEST-POLICY-VERIFICATION.json` and
`usbox:/tmp/magpie-sol-two-policy-20261116`. API1.9/API1.8 readiness below is
preserved history, superseded only for the newly rebuilt policy runtime.

The November16 committed-donor review covers the full official
`38bd73789a43bf043599d6196af96583228035d7` →
`c5e741e787594e8c18fa70ab3a4705db1c927793` diff:10 commits/68 files,
931 insertions/400 deletions, still version2.3.0. All31 selected source,
watch, fixture-authority and refusal-source paths have identical Git blobs.
The changed unified generation statuses belong to native asynchronous media;
subagent outbox/recovery and shell byte paging retain their native owners.
Our stored synthetic transcript stays opaque and shell text truncation stays
UTF8-safe. `donor_delta_test.go` adapts the newly relevant native harness
inputs to those two existing projection boundaries; it does not implement
native recovery, shell execution or media polling. No runtime port is needed.
Community `origin/main` `b35d4b35b2446bd929896ddeb58f3cb299e9e2d1` still has
the identical Zen Free0.1.27 tree
`526c7f5defd59aa68270dcbe40291e905af48b8c`.
`DONOR-REVIEW-20261116.json` records the full changed-path inventory,
content hashes and per-group decisions. Extraction pins remain original;
latest reviewed refs are separate lock fields. No fetch/checkout occurred.

Workspace layout: the Go module/embedded manifests and donor fixtures are in `src/`; browser sources are in `web/`; current verification scripts are in `tests/`; the builder is `scripts/build.py`. Paths in `watch.tests` in this directory's source lock are package-relative. Donor provenance fixture paths remain relative to `src/` so their existing bytes/hashes remain unchanged. Earlier acceptance/work records retain original paths and identities; the exact previous lock is preserved in [history/source-lock.pre-relocation.json](history/source-lock.pre-relocation.json). New relocation evidence is recorded separately in [migration/RELOCATION.md](migration/RELOCATION.md).

The table below records the **2026-11-15 historical checkpoint**.
Current rebuilt variants all require API1.12 as stated above.
`source-lock.json` is the machine-readable companion.
It is documentation and build metadata, never runtime authority, a credential
source or an automatic instruction to install another package.

| Identity | Exact revision/version | Role |
| --- | --- | --- |
| Our ordinary full package | `opencode` 0.8.0, SDK minimum 1.9, native ABI 1 | Default no-flag build: effective agent metadata, Go + Zen OAuth/key/public, Console HTTP, native agent, auth/history window and actual SQLite/WAL history |
| Explicit current API8 variant | `opencode` 0.8.0, SDK minimum 1.8, native ABI 1 | `--compat-api8`; same ready components, without API9 metadata delivery; historical 0.6.0/0.6.1/0.7.0 are not rebuilt |
| Browser history implementation base | Accepted HEAD `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`, physical tree `94ec4c8de4e5b366a6e3fb854df2176007587ab8`, native API9 | Supplied immutable closure source plus separately inventoried history owner/GUI/SDK overlay; optional browser feature does not change the native floor or publish global SDK1.10 |
| Preserved0.7.0 candidate-host replay | Physical tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, captured over `adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`, build SDK1.9 | Passed ordinary native owner, public forced-stream JSON collapse, watch types, stock SQLite/WAL/media and API9 V1/V2 plus API8 V2 agent proofs; parent full acceptance/review was separate |
| Earlier development SDK | Tree `013b3ab2d4cb863c3150af3110a377718402f38b`, build SDK1.9 | Immutable development gates only; superseded for current installed replay by the supplied candidate physical source/binary |
| Preserved 0.6.1 metadata acceptance | Tree `013b3ab2d4cb863c3150af3110a377718402f38b`, based on `6508c079b84b2a21acad5fd415eab142eaaf7b0c`, build SDK1.9 | Module race/vet/package checks and installed ordinary native connect/sync/model edits: API9 V1/V2 and explicit API8 V2; exact archive/binary/fixtures in `agentMetadataAcceptance` |
| Previous physical-host acceptance | Tree `8fe78ab9df13d733d61462eacce19bff523c1e58`, based on `c11acbc2bebec1fb213ef562900d35188cdf55fe`, build SDK1.9 | Preserved 0.6.0 full ordinary native owner and default-package WAL/HTTP replay; exact source/binary and fixture distinction in `source-lock.json` `hostAcceptance`, no future commit claim |
| Previous committed-host acceptance | `46756a48c9c9f204a8369638dc63cbcb83d02031`, tree `db40ddbf8944b54f9f689d444644af4c3bc76881`, build SDK1.9 | Preserved full-owner, package and domain evidence in `hostAcceptanceHistory` and EVIDENCE.md |
| Earlier gated history proof | Same full module with explicit `EnableHistory`, API1.8 | Archived precursor promoted into the ordinary constructor; no separate installation |
| Darwin rooted storage implementation | Shared dirty source over `ae957e5a8a77553a6cf6f484c8b58a69393f3887`, packet inventory in `source-lock.json` `darwinWork` | Actual Linux ordinary WAL/HTTP proof; Darwin arm64/amd64 build/vet and POSIX sibling tests, actual Mac runtime unproven |
| Historical headless agent artifact | 0.3.0, SDK minimum 1.5 | Explicit `--compat-agent` build for compatibility/tests; not a second installation |
| Historical provider-only artifact | 0.2.0, SDK minimum 1.3 | Archived `testdata/compat/provider-0.2.0.json`, explicit `--compat-provider`; keeps accepted four-model coverage |
| Native vendor target | `anomalyco/opencode` `38bd73789a43bf043599d6196af96583228035d7`, package 2.3.0 | Fetched dev reference; native application schema, Console auth, history storage and literal skill/watch authority |
| Latest stable official release | v2.3.0, `a6b15d33daf6ab7d35569f6d711fdb630b9f04e7` | Release tag is distinct from the existing post-release baseline checkout |
| Community behavioral donor | `magpie-community/plugins` `57d1cc6a9f49389d495ea9be8d986c70de7767bf`, `packages/zen-free` 0.1.27 | Leader-fetched reference, read-only Zen/tool/stream/environment helpers and fixtures; no runtime dependency |
| Storage host proof source | Freeze tree `43b7e290d9c3370881780b23ad49222f5d27f44b` over `9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, SDK 1.8 | Shared dirty source captured by the parent, not a storage implementation claimed at that commit |
| Storage CLI follow-up | Four-file uncommitted overlay listed in `source-lock.json` `storageWork.hostFollowUp` | Actual root list/bind/unbind parity, scoped race4/4 and vet; separate from the original WAL/HTTP freeze and any final parent host pin |
| Earlier ordinary package acceptance source | Freeze tree `f536d8202e40aae4b0fc36630aa3c1eb496d58d5`, runtime commit `d07078c7926daf6f33af09c40297c941bd39da92`, SDK 1.6 | Historical launch/Library/auth/gateway/refresh proof; gates in EVIDENCE.md |
| Previous host acceptance source | `TransmuteLabs/magpie` `dedc332cb67cfd49c4b5105ce6fa4f34d999220f`, SDK 1.5 | Historical accepted Go/agent/window gates; superseded as the common development base |
| Archived fork baseline | `TransmuteLabs/magpie` `893f560cccdeb8fa2e98eadb60427990c10eedcc` | Provider/config/launch/Library donor; no exact release tag, version explicitly null |

Version 2.3.0 is the **vendor application target**, not this module's version
and not the SDK version. A tested host revision identifies an actual source
snapshot; it does not claim that every behavior works on that host.
The historical native workflow on f423 failed environment `{config}`
expansion and Library profile canonicalization. Those failed artifacts remain
in EVIDENCE.md. The hidden-root userns reload diagnostic is a separate expected
strict authority refusal, never a reason to weaken or bypass the native guard.

The official checkout stays clean on `dev` at the prior
`83960f8a56b2f2b8ad0b751f23baaf9d0eb65ad9` baseline. Fetch used explicit
dev/version refs, no checkout/reset and no FETCH_HEAD replacement. The fetched
dev changes repair text boundaries and glob errors and change app/UI behavior.
Actual provider/auth, Console, config/schema/database and stored-history
sources used here are unchanged. The entire community `packages/zen-free`
subtree is byte-identical to the previous
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f` donor. The donor checkout remains
clean at the leader's `57d1cc6a` pin. `sourceReview20261115` records both
comparisons; no installed client/config or account was inspected.

The0.8.0 browser packet is `usbox:/tmp/magpie-history-v1.FwJugSor`.
It distinguishes the supplied closure tree from the exact owned overlay and
its compiled binary. `HISTORY_UI.md` records actual negotiation, owner/ref
schemas, package build and proof limits. Native OpenCode and community source
reviews above are unchanged; no vendor source or dependency was added.

Preserved0.7.0 replay packet: `usbox:/tmp/magpie-opencode-current.2PzAwkkH`.
The candidate source archive SHA256 is
`5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc`;
the supplied host binary SHA256 is
`0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509`.
All 10,846 source bytes/executable bits and the binary matched capture and
were unchanged before/after replay. Archive extraction stripped group write
from all captured modes; actual extracted modes are inventoried separately.
The physical tree is not attributed to its parent or a future accepted commit.
`publicPolicyAcceptance` and EVIDENCE.md distinguish the stock default/API8
libraries, endpoint/logging fixture and final metadata-only packaging.
Module race377, scoped race5/130, seven causal controls, vet/format and both
strict package gates passed. Both literal Node oracles retain 99+63 cases.
All execution used isolated offline usbox with GOMAXPROCS2/-p2.

Native OpenCode's Console integration remains separate from the generated
`magpie` inference provider. `agent_console_test.go` preserves its provider,
policies, instructions, skills and watch flag across the checked lifecycle.
Magpie OAuth alone does not create a native Console connection. Native remote
policy/discovery/session wake therefore needs no duplicate Magpie contract;
the implemented browser adapter and conditional shared-sign-in handoff are
ranked with minimal fixtures in HOST_GAPS.md.

At that historical checkpoint the ordinary minimum was API1.9 and the full
compatibility variant used API1.8. Every current rebuilt constructor now
requires API1.12; historical receipts and archived provider JSON stay unchanged.
The parent-accepted storage seam is additive to Source1.6's JSONL
contract and native account operations. The storage capability and handlers use no
provider/account credential authority and grants no consent at installation.
API1.9 is required separately for effective agent model metadata. At pinned
OpenCode `83960f8a56b2f2b8ad0b751f23baaf9d0eb65ad9`,
`packages/schema/src/config/provider.ts` declares `limit`, model `settings`
and ordered variants; `packages/core/src/config/plugin/provider.ts` applies
those overlays and distinguishes an omitted variants array from `[]`.
`packages/core/src/v1/config/migrate.ts` and `provider-options.ts` map V1
model `options.reasoningEffort` to V2 settings. `model-resolver.ts` overlays a
selected variant over those model settings. This supports configuration
projection only; it does not establish the selected effort in a running
vendor executable. The new host modalities contract remains out of scope.
The exact013b host source/binary for this bounded metadata repair is recorded
separately under `agentMetadataAcceptance`, not relabeled as a newer host.
Old-code assertion failures and the V1 fixture-only object-order correction
are preserved in the same packet. The reviewed production source and the
native-proof source differ only in that assertion; final documentation and
package checks have a separate source inventory. Ordinary API9 V1/V2 and
explicit API8 V2 native proofs all exited0. These use an endpoint/logging
wrapper, not an actual OpenCode executable.
This packaging slice edits no host files. The preserved 0.6.0 acceptance uses the immutable
physical tree and supplied binary in `hostAcceptance`; its 10,758 inventoried
paths matched before and after execution. The previous committed-host record
is preserved in `hostAcceptanceHistory`; the older `validation`, `storageWork` and `darwinWork`
inventories retain their historical source identities. HISTORY_CONTRACT.md separates
implementation, runtime proof and remaining UI/platform boundaries.

The previous API9-host replay kept 0.6.0's API1.8 floor. The current SDK
serializes unknown model efforts explicitly as `null`; the checked root
declaration now matches the native factory for all 21 affected model records.
`TestDefaultFullPackageDeclarationMatchesNativeFactory` compares actual JSON
metadata and fails against the previous declaration. No model, effort value,
provider behavior or historical compatibility artifact was changed.
EVIDENCE.md separates the stock-library storage proof from the full-owner
test entry point that substitutes synthetic loopback endpoints. Neither is
real OpenCode-client or live vendor acceptance.

The runtime graph reuses exactly the host-locked SQLite/libc versions.
The original eight Linux module versions/checksums and two Darwin transitives
are in `go.mod`, `go.sum`, `storageWork.runtime` and `darwinWork.runtimeAdditions`.
Both Darwin transitives were already selected by the host. No module version
was newly selected or fetched. SQLite and
the rooted VFS run in the native library; there is no Python, Bun, npm or
companion-module runtime dependency.

## Reviewed vendor delta: 2.2.5 to 2.3.0

Compared `2fdf7b4df5984eddb46fa7ff4cfbb84c2d39487f` to the target above
in the clean local vendor checkout, without executing that checkout:

- Device start/poll/refresh JSON, client ID, organization selection and
  `console-access.ts` remain compatible. Only the pinned request client
  version changes in this slice; explicit request assertions cover 2.3.0.
- The vendor moved refresh reread/publication into `Credential.refresh`,
  added stale checks and a 30-second exchange timeout. Magpie's SDK retains
  ownership of its own refresh transaction; no second coordinator is copied.
- V2 `providers/settings/package`, model variants and title-agent model stay
  compatible. `freeform` is a new optional capability, and V1 migration now
  respects an explicit baseURL and partial capabilities. The overlay keeps
  user fields and does not invent missing capabilities.
- Console instruction/skill lists now require a response header, have
  independent caches and bounded loads; watches require `console_watches`.
  The window distribution now implements bounded remote config/list/skill
  inspection and HTTP watch list/poll/ack actions. Policy enforcement,
  remote config/Library publication and synthetic wake are separate host
  boundaries, described in MIGRATION.md.
- SQLite paths, `session_part` and media references remain authoritative.
  History replay additionally filters instruction notices by the baseline
  epoch in `instruction-state.ts`. The gated reader implements that epoch,
  split parts and selected blob hydration. Full replay equivalence remains
  unclaimed; JSONL and inline-message-only stand-ins remain invalid.

`scripts/build.py` copies both provenance files into every distribution and writes the
selected manifest's version, minimum SDK and ABI into the packaged lock.
The repository lock and `src/magpie-plugin.json` declare the ordinary full
package, 0.8.0/1.9. The no-flag native/manifest entry points and `scripts/build.py`
include every ready component. Compatibility artifact builds rewrite only
their packaged lock's version/minimum; they are never extra installations.
The library manifest remains the host's authority.

The lock's `watch` section maps exact donor paths to behaviors and local
fixtures. Entries pointing only at `MIGRATION.md` are explicit **missing
adapter/fixture scopes**, not passing coverage. In particular:

- Zen OAuth, key and public identities are provider-bound inside this package.
  The exact native v2 catalog is embedded; the remote list controls availability.
  A failed list retains its last good copy (or the pinned bootstrap); a valid
  empty list retires all models. No dynamic models.dev/updater request occurs.
- `src/testdata/donors/provenance.json` records the exact commit, source selection
  and SHA256 of each fixture. Catalog/list/tool helper and TS excerpts are
  literal bytes; `console.json` is a reviewed JSON adaptation, and tool replies
  are generated from the unchanged helper with Node only on usbox.
- Personal Go/Zen bodies and replies retain their native pass-through path.
  Only the selected public Zen identity applies the donor's stand-in schemas,
  collision-safe aliases, declaration/history/choice rewrites and reply name
  guard for Chat/Responses/Messages. Literal request helpers and the complete
  response helper are test-only; Node evaluates 99 reviewed cases on usbox.
  The Go port preserves JSON number lexemes independently of Node's number
  representation. Nameless calls become notes; unknown named calls remain
  visible. Fragmented Chat calls retain arguments and distinct colliding IDs.
  The additional unchanged collapse/environment helpers generate 63 cases:
  54 collapse and nine explicit-environment cases. Public upstream streaming
  and Chat usage are forced; the caller chooses JSON collapse or SSE after
  tool restoration. Responses uses its authoritative completed response.
  Optional explicit environment facts are never probed or invented.
  Buffers/positions are bounded. Cut-short success synthesis remains absent;
  native refusals remain visible, with no retry or personal-account fallback.
- Official literal `skill-file.ts`, `markdown.ts` and watch type excerpts
  establish optional frontmatter fields, sanitize-compatible scalar text and
  event type shape. The dependency-free scalar subset now covers quoted and
  literal/folded strings plus scalar metadata. Full YAML, managed remote
  config/policy/Library publication and automatic session wake are optional
  Magpie exports, distinct from the native client's existing owners.
  `HISTORY_UI.md` records the implemented consent/media adapter and its exact
  gates. [HOST_GAPS.md](history/HOST_GAPS.md) separates these from the conditional
  native Console shared-sign-in boundary.
- OpenCode history is legacy JSON or SQLite, not JSONL. The default Unix module
  reader consumes actual DB/WAL, `session_part`, instruction epochs and
  selected `media:` blobs. Linux retains OFD locks; Darwin uses device/inode
  reader counts and deferred closes for POSIX locks. The host uses no-follow
  Openat and metadata-only probes, with bounded Darwin lifetime serialization
  across libraries. Linux native-host and POSIX sibling evidence is distinct
  from Darwin static gates, actual Mac/OpenCode binary acceptance, complete
  replay. Rendered storage proof is recorded separately. Unsupported platforms retain the default
  declaration and explicitly refuse rooted access.
- All reported vendor interactions are synthetic loopback fixtures with
  isolated HOME/XDG on `usbox`; no live credentials/history or vendor billing
  behavior were exercised.
- Browser SDK code is copied from the actual selected `ui.js`,
  `account-selection.js` and `errors.js` sources. TypeScript checks their
  actual types; existing pinned Bun builds a classic self-contained
  `app.browser.js` for the checked host inliner. Standalone ES modules remain
  for fixtures and the package check validates every relative dependency.
  TypeScript and Bun are build tools only; neither is a runtime dependency.

When changing the client target, donor extraction or SDK minimum, update the
lock, this record and the affected behavioral fixtures together.
