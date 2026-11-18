# Scoped Command Code verification — November 15, 2026

## Completion and receipt handoff

The module is version 0.1.3, minimum SDK API 1.9, native ABI 1. The merged
host base is `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`, tree
`94ec4c8de4e5b366a6e3fb854df2176007587ab8`. Its immutable source is
`usbox:/tmp/magpie-upstream-signing-closure.ofRrrBdz/source`, with 10,849
paths and archive SHA256
`131aeaa8fa9052dab47854ead8cb8b57f7f4dadca92715dbad307f4424c2de9e`.
The receipt's earlier capture head, `adcf4ce5`, remains separate from the
now committed physical tree. The supplied binary SHA256 is
`a8e09770909f78ae89cc6175a45bb98e1dacdb305b62856f46956928cb8ecbe2`.

The host completion correction remains a separate unstaged overlay, with ten
source/test hashes and its owning reference. It changes neither the base
packet nor its supplied binary. All completion evidence below is rooted at
`usbox:/tmp/magpie-commandcode-completion.QL5FOqka/`.

- `candidate2`: prior complete module race20, 2,840 PASS, zero FAIL/SKIP; empty
  gofmt output, vet, build, strict TypeScript, donor and official harness
  probes, pinned sources, default C-shared build and ZIP validation pass.
  Its runtime package was used by the earlier installed native replays.
- The actual old module produces behavioral failures in `red` (4 PASS,
  17 FAIL), `red2` (6/16), `agent-red` (22/1) and `usage-red` (20/5).
  These are retained independently of source-capture and fixture failures.
- `host4b9-provider-green`: 880 PASS under race20, zero FAIL/SKIP. Coverage
  includes prompt completed receipts after early close, parse failure and
  nonterminal replies; wrapper-context recovery; simultaneous repeated
  Close and cancellation; missing/late receipts; excessive tail racing a
  completed receipt; and real two-second expiry followed by a late receipt.
  The transport/test bytes are identical in the final overlay. The prior
  Close fails all seven prompt-authority negative test events in
  `host4b9-authority-red`.
- `closure94-native`: actual binary clean installation and ordinary native
  owner pass; installed Go-plan Chat/Responses/Messages whole and stream,
  plus search, pass 20 repetitions. Installed SystemOne/Decisions and agent
  lifecycle each pass 20 repetitions. The independently rebuilt syntax-guard
  overlay binary has SHA256
  `e329ba8de5d1d6cb673551ddabc22b425d9424515b6e790b62d484368400f46c`.
  This verifies successful replies, not the later typed-decoder correction.

An independent frozen-source review found that `json.Valid` was insufficient:
`{"choices":"invalid"}` is valid JSON but the actual Chat decoder rejects
its field type. The unmodified wrong-type event before a valid terminal and
a terminal with wrong-type fields both fail against that syntax-only guard:
`closure94-typed-guard-red` records 25 PASS and three FAIL, including the
parent test. Both failures report an unauthorized tail drain and completed
receipt after decoder rejection.

The correction observes sticky failure in the actual Chat, Responses,
Anthropic and Gemini/Code Assist decoders and retains their existing tolerant
event behavior. `closure94-typed-proposal-check1` has 49 PASS, zero FAIL/SKIP,
with fmt/vet/build passing. Final `closure94-typed-green20` has 980 PASS,
zero FAIL/SKIP, plus empty gofmt, vet and build. Its final source freeze is
`closure94-typed-input2/overlay`; patch SHA256 is
`8f699c58724a8612c87089775ca85bebf27477a34c94c423bee52e665f5d9440`.
The rebuilt typed-overlay binary SHA256 is
`8b1a102fae32aca85e54b75f478bd6585efe70d3a33900f7c6ed4c1fce041c4f`.
Raman's independent Astra review found no remaining HIGH/MEDIUM in the
receipt slice. Its unchanged predecessor probe has 66 passing test events
over three repetitions on proposal1, and a separate source-equivalence review
confirms input2 differs there only in struct spacing.

Astra also found a separate module defect: non-Go Provider API streams
returned completed on clean EOF with no protocol terminal. The original
reviewer probe is preserved byte-for-byte at
`verify/astra_provider_probe_test.go.txt`, SHA256
`ab3f7b6fe973549dc009cd959e9ea669ddd1b7b5b593022db0002845f3ca3d1a`.
Its independent RED fails Chat, Responses and Messages in all three
repetitions. Stronger SDK byte-preservation and outcome checks in
`provider-eof-red1` produce 9 PASS/16 FAIL, including the parent.

The correction observes bounded SSE framing and the pinned canonical terminal
predicates while forwarding the original bytes. EOF-only, wrong-protocol
terminals and malformed events or tails remain uncertain; opaque whole JSON
forwarding stays unchanged. `provider-eof-green1` passes complete module
race20 with 3,520 PASS, zero FAIL/SKIP, plus fmt/vet/build, strict TypeScript,
source checks, donor and isolated official-harness probes, C-shared packaging
and ZIP validation. Raman independently reran the unchanged EOF probe against
that frozen correction: 12 PASS over three repetitions, zero failures, skips
or race warnings. This closes the EOF-only defect separately from the actual
installed-package gates.

Final combined replay is `closure94-typed-native`, receipt SHA256
`e7fa4a6a5213c9d2da01d5b74c6d904ed8df48b2f368d66cd7c895e6d4c13847`.
It uses the exact merged tree94 base, frozen typed input2 overlay and corrected
`provider-eof-green1` package together. Actual binary clean install/list/inspect
passes. Installed Go-plan Chat/Responses/Messages whole and stream plus search,
SystemOne/Decisions and agent lifecycle each pass race20 with 20 PASS and zero
FAIL/SKIP. The ordinary owner also passes completed and truncated Provider API
SSE for all three protocols, preserving authoritative receipts, uncertainty and
no replay. The module library SHA256 is
`9d7f706ea0e7851c8054bad30459836d857952ee02a6fe0dc7c3bf2c7aea6074`.
This closes scoped F1/F4 compatibility together; host merge and broader
API 1.10 acceptance remain separate.

Final `f1-f4-final1` packaging passes the physical source check, provenance
regression, formatting, all pinned sources and overlay hashes, C-shared/strict
TypeScript build and ZIP inventory validation. All 18 production/build inputs
and the native library, manifest, package metadata and UI runtime files are
byte-identical to the tested package. The final ZIP SHA256 is
`4220d84f4f678d55db374d55b33f44e4583e58a71a24185c4a40d6162c5b7b80`.
Actual final-package clean install/list/inspect also passes in
`f1-f4-final-install`. Exact receipts and the complete changed-file lists are in
[verify/EVIDENCE-COMPLETION.json](verify/EVIDENCE-COMPLETION.json).
The containing module-only Lore commit records this accepted boundary.
No host commit or staging is claimed.

The source decision for the two-second budget lives in the host's
`docs/subsystems/extension-providers.md`. It matches the existing process
worker cancellation grace and applies only to the local tail/receipt handoff
after proven protocol completion. It bounds waiting before cancellation,
not an arbitrary native worker's final join. Completed SDK receipts remain
authoritative after protocol rejection; overflow and expiry retain
uncertainty, and `Outcome(true)` still forbids replay. No receipt is invented.

All runs use usbox, temporary HOME/XDG, pinned existing offline caches,
`GOMAXPROCS=2`, `-p=2` and the existing descendant network guard. No live
auth/vendor calls, installed-app changes, companion runtime, publication,
push, broad host suite or non-Linux execution is claimed. Earlier incorrect
inclusive-input assertions and wrapper mocks remain historical failures;
they are not final receipt or accounting evidence.

## Historical November 14 physical-host repin

The default module remains version 0.1.2, minimum SDK API 1.9, native ABI 1.
The exact tested physical host tree is
`8fe78ab9df13d733d61462eacce19bff523c1e58`, at
`usbox:/tmp/magpie-sdk-final-base-tvn9tc_o/source`. Its committed baseline is
`c11acbc2bebec1fb213ef562900d35188cdf55fe`, whose own tree differs; the
physical snapshot includes the parent's saved uncommitted inputs. No future
commit is claimed. The supplied binary SHA256 is
`c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86`.

All 10,758 physical files matched the supplied source receipt before and
after the gates. An independent Git object/tree calculation from those files
produced the exact physical tree above. The source archive SHA256 is
`01117ce6f6f8842d16fcd81224dad1795020b3b5c4d69b9145d562fd88171e37`;
all 50 SDK watchpoint hashes still match the prior accepted `46756a48` host.
That committed host and every earlier receipt remain historical evidence.

The bounded replay under
`usbox:/tmp/magpie-commandcode-final-repin.YJ06ocvO/` has these terminal results:

- `native-agent-race20`: 20 PASS, zero FAIL/SKIP, exit 0. Ordinary Connect,
  inspection, sync, model edits, effective limits/efforts and disconnect
  receive the separate public decision provider/group catalog while retaining
  only conversation models in the vendor configuration.
- `native-gateway-race20`: 20 PASS, zero FAIL/SKIP, exit 0. Installed native
  SystemOne/Decisions success, refusal and interrupted accepted execution
  preserve exact selected accounts, one ledger row, normalized usage,
  observer/recent/route projections and terminal receipts without replay.
- `native-owner`: exit 0. The existing installed owner probe passes Provider
  API/Go-plan controls, quota revisions 0/1/2, dirty/unknown inference fences,
  fresh ordinary quota acknowledgement, SystemOne, checked configuration,
  disable and account deletion. It reports zero vendor calls.
- The supplied binary performs ordinary `plugin sdk install`, JSON listing
  and inspection in a fresh HOME. Exactly one enabled default native module
  exposes its provider, SystemOne, agent and UI declarations.
- Provenance regression, all 61 pinned source hashes, default C-shared build,
  strict TypeScript and actual ZIP inventory validation pass.

Native agent/gateway/owner probes compile the exact physical host source with
test-only overlays. The supplied binary itself is exercised by installation,
listing and inspection. All runs use usbox, isolated HOME/XDG, existing pinned
offline caches, the initial user namespace and the inherited descendant
network guard. No shared-host write/index operation, live activation,
host/domain-wide suite, live vendor/client or non-Linux runtime is claimed.
Earlier causal negatives and full-domain results retain their original source
identities and were not rerun or relabelled.

The final receipt and artifact identities are in
[verify/EVIDENCE-SDK19-FINAL-REPIN.json](verify/EVIDENCE-SDK19-FINAL-REPIN.json).
The final package is checked against completed module status documents and
its runtime files are compared with the artifact used by the native replay.

## Historical API 1.9 consolidated decision catalog acceptance

The current default module remains version 0.1.2, minimum API 1.9 and native
ABI 1. SystemOne itself remains an API 1.7 operation. The exact declared
tested host is commit `46756a48c9c9f204a8369638dc63cbcb83d02031`, tree
`db40ddbf8944b54f9f689d444644af4c3bc76881`. The parent's physical source is
`usbox:/tmp/magpie-sdk-final-base-eg014lri/source`; its 10,751 file hashes
have identity
`423ad62b2299f78b66c5970093acf7ddcd2d35ec70ea941fced2e0513b56f97a`.
The source lock also pins the parent's archive and receipt hashes.

Both module agent handlers decode `decisionModels` into the public SDK DTO
and reuse its validator. Provider/group ownership, names and finite relative
decision endpoints are checked; null, malformed and unknown DTO fields are
refused with `invalid_params`/`not-started` and no output events. Omitted
options retain the older module wire. Existing picker fields are unchanged.
A valid catalog produces the same conversational plan bytes as omission;
Jev cannot be selected as a conversation model or replace an empty
conversation catalog. No recovered vendor decision-config schema exists.

Evidence under `usbox:/tmp/magpie-agent-decisions.7tS2kyJ7/`:

- `proof-commandcode-decision-options-red3`: old production with the new
  regressions, 3 positive PASS and 43 behavioral FAIL, no compile error.
- `proof-commandcode-decision-options-green1`: module race20 2,340 PASS,
  zero FAIL/SKIP; formatting, vet/build and all 61 pinned source hashes pass.
  Its later ZIP build failed on extraction-harness timestamps before 1980;
  this complete run is not claimed as green.
- `proof-commandcode-decision-options-green2`: the same source bytes with
  preserved archive timestamps. Default C-shared/strict TS packaging and
  archive validation pass; installed native metadata and ordinary SystemOne
  race20 each have 20 PASS, zero FAIL/SKIP. The actual native agent receives
  a decision provider and group, while its config retains only conversation
  models. Connect, inspection, sync, model/effort edits, user overrides and
  disconnect remain covered.
- The same native owner probe passes Provider API, Go-plan, quota revisions
  0/1/2, dirty/unknown usage fences, ordinary quota acknowledgement,
  SystemOne, action-bypass refusal, checked config, disable and account
  deletion. It reports zero vendor calls.
- `proof-commandcode-decision-options-final1`: final status/package
  verification on the same runtime inputs. The native controller and other
  runtime package bytes are compared with the actually replayed package.

Runtime source identity is
`0bf1caf9b5cfcc1a8b97ea56c3204183f7c56bf9e7712a8cb2fe186b8cd99ef2`.
Later status documents and the new receipt are recorded separately in
[verify/EVIDENCE-SDK19-DECISION-OPTIONS.json](verify/EVIDENCE-SDK19-DECISION-OPTIONS.json).
The first RED also exposed a fixture revision error; that log remains
preserved and is superseded by the compiling RED3 with passing controls.
All checks use usbox, isolated HOME/XDG, pinned offline caches and the
initial user namespace with the existing descendant network guard.
No host writes/staging, broad suite, activation, live vendor/client or
non-Linux execution is claimed.

## API 1.9 post-fix actual shared acceptance

The parent applied the two Go effort-preservation hunks. The new actual
shared capture has 10,703 files, source identity
`e65763bfb9d461d3354effadd909cdcecd2f727e3461f0c07ba96c633508ab3b`,
archive SHA256
`1ea721335576005f5aff570f7b4ce165bf24dc0fd4042c9842f0a7ccf26f4b44`.
The committed base remains ae957e5/tree 958fdab6; the dirty SDK source is
pinned separately in `source-lock.json`. Only the two parent-fixed files
changed among the 45 pinned SDK inputs. This agent made no host edit.

`usbox:/tmp/magpie-model-metadata.INRSn51E/module-metadata-postfix1/`
records these terminal results:

- SDK metadata helper race20: 260 PASS, zero FAIL/SKIP.
- Ordinary metadata lifecycle, routing-group/cache and existing no-op sync
  race20: 120 PASS, zero FAIL/SKIP, including actual legacy metadata off.
- Installed native ordinary CommandCode metadata race20: 20 PASS, zero
  FAIL/SKIP, using the default C-shared package built on this actual source.
- Complete module race20: 1,420 PASS, zero FAIL/SKIP.
- Scoped vet, empty gofmt output, 56 source hashes, C-shared/strict TS/default
  package build and archive verification: exit 0.

Exact commands, log hashes, source inventories and distribution identity
are in
[verify/EVIDENCE-SDK19-POSTFIX.json](verify/EVIDENCE-SDK19-POSTFIX.json).
The runtime inputs match the prior module RED; all captured host/module
files matched after the checks. Native execution used the initial user
namespace, isolated HOME/XDG and pinned offline caches with the existing
trusted-fixture loopback guard.

The earlier actual SDK 12 PASS/1 FAIL, ordinary 100 PASS/20 FAIL and native
0 PASS/20 FAIL remain recorded below and in
[verify/EVIDENCE-SDK19.json](verify/EVIDENCE-SDK19.json). No old RED log or
artifact was overwritten. The effort-preservation blocker is closed for
this scoped actual-source replay. Exact final parent commit/tree release
provenance, live-client/vendor checks and non-Linux native execution remain
unclaimed. No common broad suite, host commit or activation was performed.

## API 1.9 pre-fix actual shared checkpoint, retained

The default module is version 0.1.2, minimum API 1.9 and native ABI 1.
SystemOne's operation floor remains API 1.7. Discovery null/missing efforts
mean unknown at every supported minor, and `[]` means known no levels;
only the new agent-option projection/helper requires API 1.9.

The actual shared worktree was captured without overlays or proposed hunks:
10,702 files, source identity
`0f712e7774eb73c09195e5f9be96c7cc8a0b7cc42f2f58f41278f822e5519356`,
archive SHA256
`65764b96458c7d3a15155a63c4ac826d33558dc9b5fbb08d2c1fcf24ab5ce7db`.
Its committed base remains ae957e5, tree 958fdab6; neither identifies the
dirty API 1.9 source by itself. `source-lock.json` now pins this actual
capture, replacing the review archive as the current SDK input.

[verify/EVIDENCE-SDK19.json](verify/EVIDENCE-SDK19.json) records the distinct
actual and review inputs. All evidence is under
`usbox:/tmp/magpie-model-metadata.INRSn51E/`:

- `module-metadata-shared2/evidence.json`: complete module race20 1,420 PASS,
  installed native ordinary SystemOne/Decisions race20 20 PASS, both with
  zero FAIL/SKIP. Native owner, Go vet/build, empty gofmt output, Python
  syntax, strict TS, C-shared/default ZIP, seven unchanged donor dispatches,
  56 source hashes and archive verification all pass.
- The same native gate has 20 FAIL in the ordinary agent regression on the
  unknown/default/no-level distinction. Its complete result is red.
- `proof-shared-api9-1/`: ordinary agent race20 100 PASS/20 FAIL, each failure
  is the explicit empty model's `efforts: null` instead of `[]`; the SDK
  helper has 12 PASS/1 FAIL on SDK `Model` JSON dropping that empty list.
  Both are behavioral failures. Scoped Linux vet, nogui main build and
  formatting pass.
- `proof-sdk-package-sdk-public2/`: actual shared JS helper/discovery-floor
  tests pass in Node 5 and Bun 5 cases, strict TS, offline npm pack, extracted
  runtime exports and extracted package types. Discovery covers minors
  0/8/9 without a new null/empty gate.

The two required parent-owned Go hunks are supplied exactly in
[OWN-MODEL-METADATA-API9.md](OWN-MODEL-METADATA-API9.md): preserve
`Model.Efforts` without `omitempty` and retain a non-nil empty list in
`effortsOf`. Shared host/public files were not edited or staged here.
These actual failures are not replaced by the green review results below.

The current actual-runtime execution snapshot is
`970806b130c556dbfaf24d0194b1a1123b9059cf1c66922a3666023345c0b7dd`.
It retains the host's already pinned `golang.org/x/sys v0.49.0` requirement,
checksums and license for a readonly offline native SDK build. The first
actual attempt had 1,420 runtime passes but a native-entry build failure
because these checksums were absent; that failed log is preserved.
Its 14-file ZIP is
`module-metadata-shared2/dist/protocol-commandcode-0.1.2-linux-amd64.zip`,
SHA256
`911912fc2335cfce8f6dc0fd639825ec55d5e2ff195ee110688c7297c85f949c`.
Later status documents and evidence are outside this execution snapshot
and ZIP. It is a blocked checkpoint, not final release acceptance.

The remaining bounded host gate is SDK/ordinary-agent/native-agent replay
after the two parent-owned fixes, followed by exact final parent commit/tree
provenance and package regeneration. No common broad run, host commit,
live activation, additional dependency version or new scope was started.

## API 1.9 bounded review checkpoint

The 0.1.2 default package requires API 1.9 for effective agent metadata;
SystemOne itself remains an API 1.7 operation. It includes all implemented
provider, Go-plan, decision, account/model/usage, checked agent and UI
capabilities without another runtime package or a capability variant flag.

The review source is based on committed commonbase
`ae957e5a8a77553a6cf6f484c8b58a69393f3887`, tree
`958fdab666e41dc35c41a8a061b76dba88c67aeb`. The parent-owned Go/JS
nil/empty preservation hunks were applied only inside the immutable review
archive. This is not actual consolidated-host acceptance. Its source
identity is
`85019ef2c2e119eb8137731112792c4acc61ddb8fe66e648409c98b9bdafc0a6`,
archive SHA-256
`b89a76621c918214ea08208a6c4e1c1aab0c1b88b5945d4970acd19d820887dc`.
The module's 51-file execution snapshot identity is
`c58d146f193a6cee4e1999829838d35a8e5dfc468460322cf08dfa70c698236f`.
Status documents and the generated evidence are updated after that capture;
they are not attributed to its execution fingerprint.

Evidence is under `usbox:/tmp/magpie-model-metadata.INRSn51E/`:

- `module-metadata-candidate2/evidence.json`: module race20 1,420 PASS;
  installed native ordinary agent race20 20 PASS; installed native ordinary
  SystemOne/Decisions gateway race20 20 PASS; all zero FAIL/SKIP.
- The same directory records empty gofmt output, Go vet/build, strict TS,
  C-shared/default ZIP build, Python syntax, seven unchanged donor
  dispatches and all 56 pinned donor/SDK source hashes.
- `module-metadata-candidate2/native-owner.log`: exactly one native module,
  three Provider API protocols, Go-plan, quota revisions 0/1/2, dirty and
  unknown quota fences, fresh ordinary quota acknowledgement, SystemOne,
  action-bypass refusal, synthetic CLI import, checked config, disable
  and account deletion. It reports zero vendor calls.
- `proof-recovery1/agent.json`: ordinary metadata and existing no-op sync
  race20, 120 PASS. `proof-candidate2/sdk.log`: Go helper validation,
  wire/clone and floor/refusal race20, 260 PASS. Candidate2 contains the
  same production Go bytes as candidate3.
- `proof-candidate2/evidence.json`: scoped vet on Linux/macOS/Windows and
  Linux nogui main build pass. Its original agent entry failed on
  filesystem exhaustion; the recovery result is separate.
- `proof-candidate3/evidence.json`: five Node and five Bun cases, strict TS,
  compiling projection mutant rejected on missing effective `maxOutput`,
  restored ordinary Connect passes. The later discovery-floor regression
  additionally covers minor 0/8/9 on actual shared JS source.
- `module-metadata-candidate2/negatives/evidence.json`: removing output
  mapping, removing the accepted receipt and granting Jev chat authority
  each compiled and failed the behavioral assertion. Removing the default
  operation from a rehashed ZIP failed semantic package validation.
  Restoration passed; positive source inventories remained unchanged.
- `proof-sdk-package`: an actual offline npm archive includes the metadata
  helper and declaration; runtime exports and strict TS from the extracted
  package passed.

The reviewed 14-file ZIP is
`module-metadata-candidate2/dist/protocol-commandcode-0.1.2-linux-amd64.zip`,
SHA-256
`5a607fd2330f1d87b12e1709c0a6deb5bc5254eddb61d3db4c487465f0023a09`.
It is a review artifact with explicit pending-parent provenance.

The exact committed commonbase was replayed independently:
`proof-replay/` records 580 passing race20 events for SDK check-in,
activation leases, ordinary provider check-in and gateway/empty-core.
After disk cleanup, `proof-recovery1/decisions.json` records the full
exec/native decision/classifier/read-error race20: 660 PASS, zero FAIL/SKIP.
Committed source identity
`817ddef9dda9d45bbec4f409393cb6c303af072a8e6992ed2f2b691840559166`
matched before and after. The earlier decisions run had 634 PASS/4 FAIL
with `no space left on device`; it is retained as failed evidence.

All executions used usbox, isolated HOME/XDG, pinned existing offline caches
and the initial user namespace for native positives under the existing
trusted-fixture descendant loopback guard. No tests or builds ran locally.
Exact final-parent pin/replay, live vendor/accounts/browser/CLI continuation,
GUI rendering, non-Linux native execution and deferred Google nested schema
normalization remain outside this checkpoint.

## Historical API 1.7 candidate

The current default package is version 0.1.1, minimum SDK 1.7, native ABI 1.
It includes provider Chat/Responses/Messages, native Go-plan conversion,
Jev/SystemOne, checked agent configuration and UI. SystemOne is an ordinary
selected-account operation with no chat authority or additional package.

The final scoped candidate4 passed module count1 (66 events), module
race20 (1,320 events) and actual installed native gateway race20 (20
iterations), all with zero failures or skips on usbox. Its immutable module
source identity is
`97d19d1b28783a5cb0caadc61d5f62afe97edbbe49bf84631a730584927669f2`;
the shared host snapshot identity is
`88f103b2355e90e6cbae2cf2289dad0cab0d4ffeb79e329da93092f4653ae88f`,
based on `9e88f10982a1d66c1b402fba27caddda8ac4ac4e` plus explicitly
inventoried dirty SDK 1.8 additions. The base commit alone does not contain
the tested contract. All 10,589 host source files and 47 captured module
files matched after verification. These status documents and the generated
evidence were written afterward and are not part of that test fingerprint.

Evidence is [verify/EVIDENCE-SDK17.json](verify/EVIDENCE-SDK17.json) and
`usbox:/tmp/magpie-checkin-activation.1roJJ6i5/module-candidate4/`.
The module race log SHA-256 is
`51f489d3b73083165c0c421cb95f522f2e99b1945a3bb8684aeb22f1a7f65acb`;
the native gateway race log SHA-256 is
`2a42dddfdd074d3287119c5a555432afd0a3a95734a2f9dfbbba399c76bacc1c`.
All commands, log digests, source file hashes and network guard receipts
are in the evidence. Tests use isolated HOME/XDG, existing pinned offline
caches and the initial user namespace under the existing per-descendant
loopback guard. Its address inspection is for trusted fixtures, not an
adversarial native-code sandbox.

Vet/build, empty gofmt output, Python syntax, strict TypeScript, the
C-shared controller, seven unchanged donor dispatches, all 37 pinned
donor/SDK source hashes, and actual default ZIP validation passed.
Tool versions were Go 1.27.3 linux/amd64, Node 22.23.4 and TypeScript 7.0.2.
The installed native owner proved three Provider API protocols, Go-plan,
selected usage revisions 0/1/2, dirty/unknown/fresh quota fences, ordinary
quota acknowledgement, actual SystemOne, legacy action refusal, synthetic
CLI import, checked agent connect/disconnect, disable and account removal.
The ordinary gateway proved models/endpoint projection and four requests:
direct SystemOne, Decisions translation, plan refusal and interrupted
accepted work, with exact call counts and single ledger/observer/recent/
trace projections.

Removing the accepted receipt and granting Jev chat protocols each
compiled and failed on the behavioral assertion. Removing the packaged
default decision operation also failed the archive validator after its
inventory digests were recomputed; this was not a checksum-only refusal.
Restoration passed, and the immutable positive source stayed unchanged.

The 13-file intermediate default ZIP is
`dist/sdk17-snapshot-88f103b2/protocol-commandcode-0.1.1-linux-amd64.zip`,
SHA-256
`6f65b64a38580a6ec1b51cbba621f6e1c3045bf1d27d8ba5c3375717afeb66f2`.
It contains all six declared capabilities and the implemented provider,
Go-plan, decision, account, agent and UI adapters. No variant flag or
additional runtime plugin enables these capabilities.

Prior candidate directories and failures are retained. Candidate1 stopped
on two formatting lines; candidate2's gateway overlay had an unused SDK
import. Candidate3 passed 19 native gateway iterations and failed one on
two root requests. Its diagnostic overlay reproduced anonymous `GET /`
with `Go-http-client/1.1` and no Authorization; their initiating process is
unknown. The final fixture rejects only that exact anonymous probe shape
with 401 and directly verifies that it grants no dispatch authority.
Other unknown routes, including credentialed or CLI-identified requests,
still fail. Host runtime and shared source were not changed for this repair.

The exact parent commit/tree after consolidation and upstream merge
remains pending. Final packaging must replace the explicitly labelled dirty
snapshot pin and repeat the module/native/package gates on that parent.
Own-model effective output/effort metadata, deferred Google nested-reference
normalization, live vendor/accounts/browser/CLI continuation, GUI rendering
and non-Linux runtime acceptance remain outside this scoped evidence.

The remaining sections preserve historical acceptance unchanged.

The historical checkpoint contains one native module, version 0.1.0, verified
against unified SDK 1.6 host `../magpie` at
`d07078c7926daf6f33af09c40297c941bd39da92`, tree
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`. Its minimum remains API 1.5 /
native ABI 1. This is scoped fixture acceptance, not live-vendor or
whole-host acceptance.

The consolidated successor `b5e4494f6ae7b69b0333925e21e9eaf709a7acf9`
changes only `AGENTS.md` from that execution baseline. The complete Git
path diff confirms identical runtime/build/test inputs; `host-successor.json`
records the comparison. This is compatibility evidence for the successor,
not a claim that the checks ran from a different source tree.

## Historical read-only SystemOne review checkpoint

The proposal now specifies API 1.7 gates at admission and execution, both
ordinary/classifier decision fallback fences, typed transport/read errors,
exact selected-account model-operation facts, known-empty catalogs, header
ownership and one normalized-usage record. Own-model options specify
effective `Entry.PublishedOutput()`, effort overrides, valid defaults and
nil/empty wire preservation. At that checkpoint all host/agent
implementation remained pending.

`SYSTEMONE-SDK-FIXTURES.json` is packaged and hash-locked. It retains the
literal donor request/answer and 21 authored pending acceptance entries:
10 host, five usage and six agent metadata cases. These proposed cases have
not run. Two new module regressions use the actual SDK to prove precise
pre-network refusal and all four Jev aliases outside the three supported
conversational protocols; they do not supply a hypothetical host adapter.
Runtime Go code and advertised API 1.5 capabilities are unchanged.

The module-only run used the leader's immutable SDK 1.6 source at
`usbox:/tmp/magpie-sdk-final-base-6bb0rob6/source`, frozen tree
`f536d8202e40aae4b0fc36630aa3c1eb496d58d5`, head
`31984c880acbf341b213be3ffab2fa680fbf8658`. All 29 pinned donor/SDK source
files matched, including the 18 SDK files from the declared d07078c79
contract baseline. Each execution used `unshare -Urn` with loopback only,
temporary HOME/XDG and the existing offline caches.

`go test -race -count=20 -timeout=3m -json ./...` produced 1,140 PASS,
zero FAIL and zero SKIP: 26 top-level tests, 57 including subtests. Vet,
build, Go formatting, strict TypeScript, C-shared packaging, source hashes
and actual ZIP validation passed. Changing the action's `not-started` to
`uncertain` caused a behavioral failure; removing the decision-model
transport guard caused behavioral failures for all aliases/protocols.
Both mutants used module-source overlays, and restoration passed all 21
events in the two new test families. The unchanged pinned donor passed
seven dispatches, including synthetic response-header preservation.

Evidence: `verify/EVIDENCE-SYSTEMONE-REVIEW.json` and
`usbox:/tmp/magpie-commandcode-systemone.8S8u2P1e/`. The 37 selected inputs
matched fingerprint
`4ae1dcfd7ee2e17ccf04d86c8b63f3f9f34eb04c5c358e6456cb2c1d16d6cb01`.
The new 13-file ZIP is
`dist/systemone-contract-sdk16/protocol-commandcode-0.1.0-linux-amd64.zip`,
SHA-256 `93bdf1a57048108b9e43c0747d16075e867a89bba7cf8b1cc3b0676726a77a49`.
Prior distributions and failed logs are retained. No native installed-owner
probe, whole-host suite or live vendor/client/UI acceptance was repeated
for this proposal-only checkpoint.

## Historical unified SDK 1.6 checkpoint

Both paused WIP files are retained and finished: `PLAN.md` records the
bounded checkpoint and `verify/systemone_donor.mjs` proves the recovered
decision wire. Runtime Go code and the module's advertised capabilities
remain unchanged. `source-lock.json` now pins the exact shared host and
23 inspected source files, including the donor's Jev regression. The
community package remains 0.1.24 at `26ea8faf`; CLIProxyAPI remains pinned
at `7638884b`.

All checks ran on usbox with fresh HOME/XDG and the existing offline caches:
`go test -race -count=20 -timeout=3m -json ./...` produced 720 passing
test events and zero failures, from 24 top-level tests / 36 with subtests.
Vet, Go build, formatting, strict TypeScript, C-shared packaging, source
hashes, Python syntax and actual standalone ZIP validation all passed.
Tool versions were Go 1.27.3, TypeScript 7.0.2 and Node 22.23.4.

The existing read-only usbox host source matched all 10,420 files streamed
from the exact `../magpie` commit, with no extra source files. The module
retained its `../magpie` development replacement through a symlink to that
verified source. A Go overlay supplied the native fixture entrypoint;
no shared host file, branch, worktree or additional host copy was created.

The real native installed-owner probe passed Provider API Chat/Responses/
Messages, Go-plan Chat, synthetic CLI import and checked agent actions.
It verified revisions 0/1/2, dirty inference denial, unknown quota remaining
fenced, fresh native acknowledgement and ordinary `provider.LoginUsage`
acknowledgement. It also proved that status explicitly names blocked Jev,
the module action returns `unnegotiated_capability` / `not-started`, the
host rejects decision model metadata and the ordinary decision operation
cannot dispatch. It installed this package alone and made no vendor call.

The hash-checked unchanged donor passed seven recorded SystemOne fixture
dispatches: all four aliases, exact 1.88.2 CLI headers, ZDR, answer bytes
and account-preserving refusals. Invalid JSON stopped before dispatch.
This is donor-source evidence, not an official CLI or live vendor capture.

Current evidence is [verify/EVIDENCE-SDK16.json](verify/EVIDENCE-SDK16.json)
and `usbox:/tmp/magpie-commandcode.ogjQFhdz/v4-sdk16/`.
The package is `dist/sdk16-d07078c79/protocol-commandcode-0.1.0-linux-amd64.zip`,
SHA-256 `92c29ff97c484aedde222da6fb536e37780e923183f7c44da07896832ad603c4`.
Its 12 inventoried files need no additional runtime plugin.
The 35 selected inputs have fingerprint
`75cc1d4c9b703051b294260aeffe22c2f341c3483805b8e201da152b4859d513`,
using the same algorithm described in the historical record.

SystemOne decision execution and own-model effort/output metadata remain
SDK gaps on this exact host. The minimal contract and ordinary adapter
seams are in [SYSTEMONE-SDK-CONTRACT.md](SYSTEMONE-SDK-CONTRACT.md).
Cold persistent model-catalog reads, live accounts/vendor calls, actual
CLI continuation/history, GUI rendering and non-Linux execution were not
verified. Whole-host acceptance and the independent first deployment with
zero plugins belong to the host owner.

## Historical SDK 1.5 checkpoint

The remaining sections retain the original two-host SDK 1.5 evidence and
its limitations. `verify/EVIDENCE.json`, original failed logs and the
original `dist/` ZIP remain unchanged.

## Changes and simplifications

- `module.go`, `module_test.go`: echo canonical selected-account usage
  revisions, reject invalid revision/credential authority before networking,
  and remove the historical `AsOf` stamp from live reads. Unknown account
  readings retain errors and cannot reopen dirty usage.
- `generate.go`, `generate_test.go`: refuse native disabled reasoning,
  `parallel_tool_calls=false` and unsupported stream options explicitly.
  Existing refusals still cover unsupported protocols, content and terminal
  states. The request decoder reuses the existing bounded JSON utility.
- `provenance_test.go`, `source-lock.json`, `SOURCES.md`: freeze source-file
  and fixture hashes, transformations, versions and real regression
  watchpoints. The complete 57-entry static Go model table is retained.
- `build.py`, `package.json`, `verify/check_sources.py`,
  `verify/check_package.py`: refuse stale output and mismatched provenance;
  validate the actual ZIP and its declared inventory. Retain the donor
  license and package the separate proposed decision contract.
- `verify/native_owner.go.txt`: use the real native loader, account store,
  usage barrier, ordinary provider/quota owners and checked agent actions.
  Require an explicit backup directory for agent writes.
- Existing `auth.go`, `billing.go`, `models.go`, `transport.go`, `agent.go`,
  their tests, `cmd/manifest`, `cmd/native`, `web/`, `go.mod`, fixture data,
  `AGENTS.md` and bounded plans form the rest of the initial coherent module.
  Go cleanup is standard formatting only. `.gitignore` excludes the
  downloadable build artifact under `dist/`.

No third-party dependency, companion community runtime, new repository,
branch, worktree, vendor account, live credential or publication was added.

## Exact inputs and execution

All builds, tests, native execution and artifact/source validation ran over
SSH on usbox, with a temporary HOME/XDG, existing pinned Go caches,
`GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOWORK=off`,
`GOMAXPROCS=4`, `GOFLAGS="-mod=readonly -p=4"` and `CGO_ENABLED=1`.
The installed tools were Go 1.27.3 linux/amd64 and TypeScript 7.0.2.

The SDK input was an exact `git archive` of host
`84ae51afd40953280d768d7498990defb9a37595`, tree
`5a9e2397db331f55220f88b8b8be94f989ebea02`.
The same final native distribution was also exercised by a host probe built
from `e2c799fe8f3c35f2cd8d89e6ca306223a5dc06ec`, tree
`bdfece9519af20b48c9c64585c5fbe0e84000634`. Neither checkpoint claims to be
the leader's combined SDK 1.6.

The 34 verified source/build/test inputs are byte-identical locally and on
usbox. Their fingerprint is
`349f8c9e29b478ecc490a3c25155f13c4765e712068141fb0c629863e0a0334c`:
SHA-256 of sorted relative UTF-8 paths, each followed by a NUL and its binary
SHA-256 file digest. The selection comprises root/command Go files, tests,
fixtures, web files, `go.mod`, package/source metadata, build and verification
scripts, the native probe and packaged documentation. Plan/status/evidence
files and ignored distribution output are outside that input selection.

## Passed checks

| Check | Actual result |
| --- | --- |
| `go test -race -count=20 -json ./...` in this module | 24 top-level tests, 36 with subtests; 720 passing test events, zero failures. |
| `go vet ./...`, `go build ./...`, `gofmt -l` | Passed; no diagnostic output. |
| C-shared native controller build | Passed with the SDK-owned native registration/exports. |
| Strict TypeScript compilation | Passed with the pinned SDK's UI declarations; generated JS contains no development SDK import. |
| Python syntax checks | Build/source/package scripts parsed successfully. |
| Pinned source validation | All 16 selected source files across the three immutable source records matched their SHA-256 digests. |
| Static table derivation | The pinned donor's `_internal.GO_MODELS` was evaluated on usbox and matched all 57 fixture entries; no vendor request. |
| ZIP verification | Twelve inventoried files; manifest/package/source-lock identity, version, SDK floor, native ABI, documentation and hashes agree. No additional npm runtime dependency. |
| Native installed-owner probe, both host checkpoints | One installed module; three Provider API protocols, native Go-plan Chat, quota, synthetic CLI import, checked agent connect/disconnect, disabled-module denial and deleted-account denial passed. |

The native probe also verified usage revisions `0`, `1`, `2`. The host-side
fixture invalidated the actual account store; ordinary inference was refused
before reaching its local server. An unknown billing read echoed its
revision while leaving admission closed. A fresh native read acknowledged
the revision and cleared the barrier. A second invalidation was acknowledged
through ordinary `provider.LoginUsage`. Invalidation in this probe is host
test control, not a newly granted module write capability.

## Causal negatives and retained evidence

Evidence remains under `usbox:/tmp/magpie-commandcode.ogjQFhdz/`.
The original `race20-v2.jsonl` remains 580 pass / 20 fail and the original
`native-owner.log` still records the missing-revision failure. They were
not overwritten.

Under `v3/`, `usage-red-v3.jsonl` records failure of live freshness and
precise invalid-revision rejection before the handler fix.
`controls-red-v3.jsonl` records silent acceptance of native reasoning off
before its explicit refusal. Go source overlays independently remove the
revision echo or insert historical `AsOf: 1`; both make
`TestUsageAcknowledgesTheHostRevision` fail with a behavioral assertion.
Neither negative is a build failure or a skipped test.

The final clean run is `v3/race20-clean-final.jsonl`. Native proofs are
`v3/native-owner-final-magpie.log` and
`v3/native-owner-final-host-e2c799fe.log`. Package/source proofs and log hashes
are recorded in [verify/EVIDENCE.json](verify/EVIDENCE.json).

The locally available artifact is
`dist/protocol-commandcode-0.1.0-linux-amd64.zip`, SHA-256
`0743c81724401a6d18d7c7d902cf011050e43940e7fb47afcf31b7c7c12a0e2d`.
It is identical to the artifact validated and installed on usbox.

## Host blockers and remaining limits

At the verified SDK 1.5 floor, SystemOne has no negotiated decision model,
model-operation association or non-chat execution adapter. The exact minimum
proposal is [SYSTEMONE-SDK-CONTRACT.md](SYSTEMONE-SDK-CONTRACT.md). Status
reports that gap; the `systemone` action refuses it explicitly and Jev is
excluded from conversational discovery/execution. Whether the combined
SDK 1.6 resolves this is unverified until its exact commit is supplied.

The host's current agent model snapshots provide context windows, but do
not supply max-output or effort capability metadata. The module preserves
existing BYOK metadata and invents no missing limits. This is an integration
boundary to recheck at the combined SDK pin.

No live vendor, actual Command Code CLI continuation, real browser approval,
real CLI credential import, native history, GUI page rendering or non-Linux
execution was tested. The donor's Google nested-reference normalization is
not ported, so those Provider API tool schemas can still be refused by the
vendor. Unsupported native controls are listed in the README/source lock;
paused or truncated work remains uncertain and is not replayed. Host-wide
and combined SDK 1.6 suites remain owned by the leader.
