# Independent Devin distribution acceptance

Date: 2026-11-14. These are actual libraries installed in the ordinary
host, separate from SDK-peer tests. One own `devin-inbound` distribution is
installed per fixture. No community plugin, runtime helper or built-in
experimental loader is required. Execution is on usbox with temporary
HOME/XDG/TMPDIR, pinned offline caches, local peers and bounded commands.

## Standard install: one full default package

`python3 build.py DISTRIBUTION` builds the complete0.8.0/API1.9 `NewAgent`
module by default. Its single native library and manifest contain inbound,
outbound Chat/Messages, auth, models, usage/Facts/diagnostics, the agent
profile, all four implemented finite bootstrap operations and the
default-off paid fresh-account cache task. `keepAlive` is false unless
explicitly enabled with the paid confirmation; timing alone never opts in.
`--agent` is an alias; `--paired-only`0.5.0 and `--inbound-only`0.2.0 are
explicit compatibility artifacts. They are alternative installs with the
same module ID, never dependencies of the standard package.

## Current committed host: strict sampler and hot activation

Accepted host commit:
`d5b098d8183e88ab9fee1da0170d1c90b9590822`, tree
`11c2b9dd3d514bcc76c338f9ae680552319494dc`.
The accepted physical source is
`usbox:/tmp/magpie-sdk-final-base-23ttp2l1/source`.
Execution uses the parent's existing binary at
`usbox:/tmp/magpie-sdk-final-base-tvn9tc_o/magpie`, built from tree
`8fe78ab9df13d733d61462eacce19bff523c1e58`.
The capture comparison proves exactly two differences, both tests:
`internal/cli/settings_parity_test.go` and
`internal/provider/extension_account_tasks_catalog_test.go`.
Production bytes are identical. The binary was not built from tree11c2.
No host overlay, rebuild or runtime exception is used.

| Current input/output | SHA256 |
| --- | --- |
| Accepted source capture receipt | `450ac8c1fb79389904c782d018e0612cafad9e14409873a3a21254e6e4d30324` |
| Accepted physical source inventory,10758 paths | `cdfb6bb3b27c2a618f33eb47d6e3e9b652d351dda13d24ba5dfaa1a8795d0583` |
| Binary-build source capture receipt | `7ce4753ff79f3493acb4f61c1826d3f52145f8b156537ac61f493b39c9a50cc6` |
| Binary-build physical source inventory | `a902d6428bba59b469e74fa466ff77f16ac65a66ef97e6863bcb0a109cd02c66` |
| Parent core-build receipt | `baf13fcc69133298f1b44a3d60e76744288782658ad43f561ddece1a6e6a9e77` |
| Actual runtime binary | `c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86` |
| Public SDK export, unchanged from467 | `36f73768a1335fbf1210fd7033528031f70f2de72b3b182f57af7b0a954969b5` |
| Tested implementation inventory, before current documentation/provenance | `925c2d4e053c593a81e6da19be409d591bfec2754470f8919d4114dd794241c9` |
| Tested implementation archive | `20e6cbe477e7b54d7a7126cdf96fd4c61cfc1148c91ed668bff8c42890d5d0f9` |
| Full default0.8.0/API1.9 library, unchanged | `017ebcccec33c936334853b6efbca9991e697d657bef620bbc296991040f3402` |
| Corrected maintained hot fixture | `c2c58b5a3d1e2dc2b87a66c83c85990776bb11ffe8655b60d33439aa7c2408c3` |
| Nine sampler/template regressions | `4405c0c4995183efa29ac5ae8b1945cba23b0f1b7bb69ed581c4da0bc91c638b` |

Evidence root:
`usbox:/tmp/magpie-devin-final-fixture-20261114.3miL8Zfd`.
RED and GREEN captures both use module baseda599, sourceCommit:null.
The new maintained fixture defaults to hot external config CAS without a
gateway restart or catalog warmup; `--cold-start` is the explicit cold
control. Its held38 wait, real30-second cadence, admission controls and
cancellation checks are unchanged.

The fixture uses the existing cold fixture's `sampling_chunks` scanner,
which preserves raw wire0/1/2/5 chunks. The legacy `conformance.fields`
decoder remains strict and unchanged. Recovered `chisel-api/src/wire.rs`
and the actual canonical encoder use fixed64/wire1 for native temperature
and top_p, fields5/8. The named regression
`test_legacy_parser_rejects_valid_fixed32_and_native_fixed64_chunks` also
proves the old decoder rejects a structurally valid fixed32/wire5 chunk,
while the scanner preserves it exactly. That field99 case is structural
coverage, not a claim of a captured native fixed32 control.

Strict assertions require the exact foreground sampling chunks:
n1/max_tokens64, zero fixed64 temperature/top_p, stop `cache-stop`, seed7,
max_newlines400 and top_k40. The paid request changes max_tokens to1
exactly once and preserves the other chunks and their order/bytes.
Both requests must retain the `Read` tool definition, named `Read` choice
and disabled parallel calls. Wrong sampling or an omitted choice on both
sides now fails, as do altered bytes/choice/parallel mode, duplicate
output limits and malformed/truncated wire chunks.

| Causal regression | Actual result | Log SHA256 |
| --- | --- | --- |
| Original hot75cb parser with the valid native fixed64 template oracle | RED, exit1 at unchanged line117 → `conformance.py:84` | `01840f15c79abbaa3a2c0b17ad5d334a4d889bba2a5b5ef0f410617cf49dd742` |
| Nine new tests against unchanged cold511 assertions | RED,7 pass/2 fail: wrong sampling and both missing choice were accepted | `2a0e21909b74bf23f4696290151fc8bd32f8fd8afde728158d9e00d1827b8e0b` |
| Same nine tests against maintained fixture | GREEN,9/9 | `1bd6daf5661d17cdde83357e0bfc2e376820baec559a299699c588f36b4267e3` |

Three bounded full installed hot repetitions completed:

| Gate | Exit / seconds | Log and fixture-result SHA256 | Command-receipt SHA256 |
| --- | --- | --- | --- |
| hot1 | 0 /44.680 | `11ee3a9de14c30ad5bd97de5295060c1dc7a1cbfcb568449cd8fda5180978fcf` | `262aa1d270f2a47f23d87094ab7046880f4877023a45852560bd8d180ad14f2d` |
| hot2 | 0 /46.240 | `8f545f17d1e4722e2dbe0d9b91979998fd542c1f337d126e663375d5affbaa8b` | `f50ffc880669a4acd73b7a7e6e8a6edc5c35f7a3e747c9bab5405e2b4d527a2a` |
| hot3 | 0 /47.552 | `161acd9b93ef2aba45225943c780581904f08e64753c62571c8781aa11149aff` | `0ea2d06d5adb918dacd25cc6cf48e0c29a089c5c0d79aba4146a331cafad1700` |

Each repetition installs one own module and maps that exact library.
Default-off creates zero intents. Spent-budget, revoked-model and readOnly
controls refuse dispatch. Both completed and held paid requests arrive;
a new foreground cancels and joins the held request. Four intents end
completed/not-started/not-started/uncertain. Exactly two measured ledger
rows retain distinct operation IDs, the original caller and
`[in,out,read,write]=[7,0,17,5]`, each with2.5ACU and no inferred USD.
Direct paid-action replay is refused. Each run records eight local vendor
arrivals, zero external requests, no peer failures and a stopped gateway.

| Other current gate on the same binary/library | Result | Log SHA256 |
| --- | --- | --- |
| All module Go tests | GREEN | `d26581bd5d22a480672a77d7a793d8aefcad337519442c490146c327fd981672` |
| Go vet | GREEN, empty log | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Standard default native build | GREEN | `97adccb1588a220719b8fb20496d589b963a3063c15fca2dd7f94b15190d330c` |
| Native options ABI | GREEN,8 checks | `942b2f37bb0119496c86b3f55d7bf9940eaa0cc05d390a697349262105ee1bf1` |
| Ordinary duplicate/no-op/held read | GREEN | `71a8085501b7bb91257baebf916c42b0ecfeb6065c4dcfe9359773678a798191` |
| Real model transitions | GREEN; zero automatic retries | `e02a22926f7f386a6c4c91122de626fd4796d10e03d560d955d779b4693f52a0` |
| Genuine Gemini Chat/Messages JSON/SSE | GREEN, all4 cases,368-byte signature/type/tool roundtrip | `c2fad1cedcaff9e8c60e281b33a7df0f3ee25d9d3552d4c665c4bf5ee8ae3b57` |
| Paired catalogs/policy/cancellation | GREEN,24 checks | `bafcad486f17800cdd6dc87bc79e44a446b67dcabb9d041489e436ae4c511810` |

The transition fixture's historical `bootstrapAcceptance:"red"` marker
remains in its raw result; its real-transition gate succeeds. It is not a
blanket original-client bootstrap claim. Every command audits the complete
module/public-SDK and10758-path accepted-host inventories before/after,
verifies the binary hash, removes its sandbox HOME and uses the existing
network guard with an explicitly blocked external canary.
The initial green unit test log already reported9/9, but its wrapper
failed while hashing a library being built concurrently; the separate
harness-error receipt is preserved. The corrected sequential receipt
`fixture-unit-final` is the GREEN above.

All earlier REDs remain evidence of their exact runs. In particular,
`usbox:/tmp/magpie-devin-hot-cas-fix-20261114.kjo_hnjn/installed-original-hot-tvn*`
records a c018 run that reached a paid request/row but timed out waiting
for held work at38s. Its timing cause is unknown and was not reproduced
or proven fixed. Godel's six separate diagnostic/control runs reached
both paid requests, cancellation and two rows, then failed the legacy
sampling parser. His closure evidence
`usbox:/tmp/magpie-hot-cas-closure-20261114.sQVusLEq/EVIDENCE.json`
has SHA256
`44935bfcae6dc0e2cf69939e3b1b87540807c625c31c965e35ce3c727d1f0e63`.
No raw sampling chunk was retained there; its wire type is not inferred.
These observations and the corrected fixture's three passes do not
relabel any old run, including the467 no-paid and052 not-started results.
No additional host change is proposed by this fixture correction.

### Fresh merged candidate compatibility

One additional bounded installed replay uses frozen candidate tree
`f7f26b2d5ee5388a089ab07a72f7372ed0068971` at
`usbox:/tmp/magpie-sdk-final-base-p2tiu0uf/source` and its parent-built
sibling binary. This identifies a tree, not a future host commit.
Its public SDK matches36f73768 exactly and paid-admission production
remains9b04f613. The same fixturec2c58b5a and library017ebccc complete
GREEN in43.414s, exit0, with no production overlay.

| Fresh candidate input/output | SHA256 |
| --- | --- |
| Source capture receipt | `1c5ad12c8bf6774a73e8b89497250aae7e905fe790891384639113a58bc35009` |
| Parent successful core-build receipt | `9ae108a4016c42c7cc693d1fa5d1817c2580a8fed383b56e548fc1f4f97b3234` |
| Actual binary | `9ddc4e4a076dbcc119e71b5c3b140253e8c419f3278dfadc5d8cdadd59c20adb` |
| Installed command receipt | `8701d0fa5b67b24bf654276eb445a37ae9f2ac63d6aa1308630a580d4edcce9a` |
| Log and fixture result | `dd4fdf4a5e4f42509192447233c1d520c965b76117aace1a776f22ab55ca8b8e` |
| Network guard receipt | `1515f2bc66724aad5cfd4dbdffc70a369fcb5ef983a0283fb4dcc820bedc2ef9` |

Evidence is under the same root's `fresh-host` directory. All10773 host
source paths, module/public-SDK and distribution inventories stay
unchanged. All eight paid-fixture assertions pass: both paid arrivals,
the four terminal states, cancellation/join and exactly two measured
ACU rows, with zero external requests, no peer failures and a stopped
gateway. The temporary HOME is removed. No other installed fixture or
broad suite is attributed to this new candidate; the three c018 repeats
and their source23 distinction remain the earlier exact evidence.

The final metadata/package receipt is `FINAL-PACKAGE-RECEIPT.json`.
It records the later documentation/provenance capture and byte comparison
of its emitted library/manifest with this installed-tested artifact.
No runtime code, manifest version, paid policy or guard changes here.
Live vendor/cache lifetime/charges, the original Cognition client, complete
native resume, other platforms and whole-host broad acceptance remain
outside this proof.

## Historical committed467 API9 compatibility and physical-host replay

This earlier host is committed
`46756a48c9c9f204a8369638dc63cbcb83d02031`, tree
`db40ddbf8944b54f9f689d444644af4c3bc76881`.
Parent's core-build receipt is exit0. Acceptance uses its exact physical
source and already-built binary at
`usbox:/tmp/magpie-sdk-final-base-eg014lri`; no host source or binary is
changed or rebuilt by this module.

| Historical input/output | SHA256 |
| --- | --- |
| Parent host source archive | `e188abd5d115c7da3167a6296e7d07f9bd31766a738ea4e6326ff093e4a74a61` |
| Physical host source inventory | `8031506ab47325fe0c2c6db6af0db8ed3373e822440568f6ae98d57030c5ed45` |
| Parent-built host binary | `32fbdeabd20ed3059f61a91f7614a51a2ac9b908b12dd637c204a82918a28256` |
| Public SDK export for compatibility gates | `36f73768a1335fbf1210fd7033528031f70f2de72b3b182f57af7b0a954969b5` |
| Tested module implementation capture, before documentation reconciliation | `0eb3f976b3c50bd28c06f7ac0b3a959888e77b5c29090341f9c451df4dca72ea` |
| Full default0.8.0/API1.9 library | `017ebcccec33c936334853b6efbca9991e697d657bef620bbc296991040f3402` |

The unpublished API9 agent options gained typed `decisionModels`.
The module now decodes the public `AgentDecisionModel` DTO and calls
`ValidateAgentDecisionModels`, separately from conversation
`models`/`priorModels`. Omitted input remains compatible; explicit null,
invalid identity/endpoints and unknown root/nested fields remain refused.
Connect/set/sync/disconnect preserve conversation binding authority and
write no client files or decision-model values. No host guard, journal,
manifest version or compatibility floor is changed.

The tests-only RED is
`usbox:/tmp/magpie-devin-decision-red.jibyzwAP`, inventory
`350639a1643d31e4ddc5670b3c6a134effb5352ca6060b7163dfc59d112dee01`.
Both the Go regression and actual native ABI conformance fail on a valid
catalog; omitted-field controls pass and the native build succeeds.
Go assertion log40795df9 and ABI log5f7b1a12 are preserved as REDs.
GREEN is `usbox:/tmp/magpie-devin-decision-green.A17gyfzE`: empty gofmt,
the same regressions, all module tests, agent race/count20, vet, default
native build and all eight ABI checks exit0. The public SDK files are
byte-identical to the final committed physical source.

Installed replay:
`usbox:/tmp/magpie-devin-combined-accept-r3.46756a48`.
The default library rebuilt against that physical SDK is byte-identical
to the one above. Exact full file sets, bytes, physical modes and binary
hash remain unchanged before/after every command.

| Historical installed gate | Actual result | Log SHA256 |
| --- | --- | --- |
| Native API9 options ABI | GREEN, all eight checks; no client files | `942b2f37bb0119496c86b3f55d7bf9940eaa0cc05d390a697349262105ee1bf1` |
| Ordinary duplicate/no-op/held read | GREEN; one AssignModel dispatch, duplicates403 before/after sync, binding/journal stable, held read200 | `ef0369bbfd3fda3564c9a9ceb8fdfe545e15fe864c7204c819340621a3337c70` |
| Real model transitions | GREEN; stale model and held replies remain fenced, accepted write completes once without replay | `e02a22926f7f386a6c4c91122de626fd4796d10e03d560d955d779b4693f52a0` |
| Genuine Gemini Chat/Messages JSON/SSE | GREEN, all four cases with original donor signature/type/tool roundtrip | `406b903753156bc927fa3921890bccbb02f64662ed50b5f91930741f0064b270` |
| Original hot-CAS paid fixture75cb45a5 | Actual RED; all five foregrounds200 and readOnly403, but no paid arrival within38s | `dbb584f57214d2127b766b7281f074269194e13f2a0c8f23fb00d33072c3aeea` |
| Independent unchanged cold-start paid fixture511cc360 | GREEN; real30s cadence, budget/revocation, held cancellation and two once-only measured ACU rows | `d738798a2096a1dd66caedd1fb7a28a4b5dcbcdfc40f1b35f10bc3311e750fef` |

The cold control is separately captured at
`usbox:/tmp/magpie-devin-cold-control.46756a48`, using the same binary,
library and guards. It proves default-off, fresh budget/model/readOnly
admission, canceled measured usage and rejection of direct paid-action
replay. It does not convert the hot-activation RED into a pass.
Hot-CAS foreground routing no longer reports the earlier stale-catalog
error, but its paid phase still has zero arrivals and measured rows;
four not-started receipts were collected after fixture cleanup. Those
receipts alone do not establish the exact admission/cancellation cause.
No paid workaround, retry or host edit was introduced in that checkpoint.
The later current-catalog correction and strict hot fixture acceptance
are recorded above; this original run remains RED.

Two earlier runner preflights stopped before building or testing: the
parent's source receipt records0664/0775 while its extracted physical tree
has0644/0755. All10,751 paths and hashes match. The successful runner
verifies this exact0022 extraction mask once, records the actual inventory,
then compares every byte/mode exactly after each command. The receipt's
separate pre-extraction inventory digest is066f893d; it is never confused
with physical inventory8031506a. These preflight failures are evidence
setup failures, not protocol REDs.

The current README/default command describes0.8/API9. Historical0.7,
API7/API8, dirty API9 and052 paid-not-started receipts remain unchanged.
This is scoped local-peer acceptance, not an original Cognition-client,
live vendor/cache lifetime/charges, complete native resume or broad host
readiness claim.

## Historical pre-compatibility0.8 checkpoint

Earlier tested library SHA256:
`464489d54f5982f310821ebefb26704deb65f24ca20fe74161f75f28be1d6910`.
Module production input inventorydac520a0 passes all own module, race3,
vet/build, nine provenance cases and the unmodified donor7/7 tests.
On complete frozen hostca3/binary6d4, the default build and six selected
installed gates pass: paid/default-off/canceled usage, paired24, genuine
Gemini Chat JSON/SSE, Claude Messages signatures/history/cancel, ordinary
synthetic agent/bootstrap and real model transitions.
`CACHE-KEEPALIVE-EVIDENCE.md` and `GEMINI-SIGNATURE-EVIDENCE.md` pin the full
inputs, library, logs and preserved REDs separately from the public SDK
export used for module-only gates.

Three host gaps at that checkpoint prevented blanket readiness: cosmetic ModelOptions
sync can reopen duplicate AssignModel (held-no-op read not reached),
cross-process hot module.config CAS leaves routing catalogs stale, and
legacy Anthropic shaping injects unsupported thinking-disabled into
Gemini Messages before SDK execution. No shared host source is changed
for these gaps. The committed API9 replay above remains separate from this
dirty frozen runtime. Earlier cold CLI and Rust
reconstruction startup results below are historical local-peer evidence,
not current original-client acceptance.

## Historical0.7 full-default packaging correction

Evidence root: `usbox:/tmp/magpie-devin-default.S2EoN3BM`.
The tests-only RED builder snapshot has inventory SHA256
`5037ad2fe8a5118046c3f9da5094702514ee68b19bdd6e311e1ebf04e9c74db1`:
the ordinary no-flag command selects0.5.0/`cmd/native`, not the expected
full0.7.0/`cmd/native-agent`, and `--paired-only` is unsupported.
The explicit existing compatibility controls pass on that baseline.
These are behavioral packaging failures, not a failed native compile.

The successful `green-r2` builder/fixture input inventory is
`98dc5cf69a7f4818045aa01e1ee03634723c33e46a24e44db7e95c7b9e0fcc51`.
It uses the unchanged frozen host sourcec434/binary2fe733 described below.
All eight provenance/distribution tests pass, including the ordinary
command, explicit alias, compatibility flags, packaged capabilities,
bootstrap routes, variant floors and source-lock identity.
All three actual native builds pass on linux/amd64 with Go1.26.8.

| Actual artifact | Library SHA256 |
| --- | --- |
| Standard default0.7.0 | `77717f491587f4f5fcb73ffa4f183b2fd1bb15e5fc8a8c95751c7d614cf6aab6` |
| Explicit paired-only0.5.0 | `82600601283eddd749a23df6e1e506029f6f5dd4de650b1bf290765b85fb633d` |
| Explicit inbound-only0.2.0 | `526f96304ca6c206fa004e55f09cacdbef9f9e7d9e958d9c23da6d0e18af714d` |

The same standard default artifact passes all nine installed fixtures:
paired24 groups, ordinary agent/bootstrap/cold CLI, Messages signatures/
stream/cancel, each of the three corrected Chat histories, execution
Facts/ledger/diagnostics, duplicate/no-op/held read and real transitions.
Every fixture installs exactly one own bundle; no community/helper plugin,
vendor request or real credential is used. All temporary HOMEs are removed
and fixture processes stop. Both full source inventories remain unchanged.
The standalone codec-only compatibility artifact is built here; its
earlier independent ABI proof retains its separate historical hash below.

`green-r2/checks.json` SHA256:
`b930dd694b83fefacb490468792faca4e2f3788635a30dd72069d2fcfe72dc0f`.
The40-entry raw receipt archive SHA256 is
`0f11ed1ac43f6510823a1e220a35e4f80b344ae7264afd71bdadc8a062f0547b`.
Local copy: `dist/2026-11-14-default/receipts.tar.gz`.
The initial `green` audit failure is retained: paired24 and agent7 passed,
but the child Python fixture created two `__pycache__` files in captured
module source. Its wrapper now sets `sys.dont_write_bytecode=True`.
`initial-audit.json` and the initial runner retain that observation;
the strict full-inventory check remains intact and passes in `green-r2`.
It is an evidence-isolation failure, not a protocol RED.

No Go production source, manifest schema, donor pin or host guard changed.
The subsequent documentation/provenance supplement is packaged separately
with a repeated provenance gate and byte-identical standard library;
its final archive receipt is retained beside the raw evidence as
`dist/2026-11-14-default/final-package.json`. The standard install archive is
`dist/2026-11-14-default/devin-inbound-0.7.0-linux-amd64.tar.gz`;
the receipt records its SHA256, exact final source inventory and byte
comparison with the installed-tested library and manifest.
The parent's later HandedOn/ResponseID correction in
`internal/gateway/extension_usage.go` is outside this frozen proof.
Final combined host commit/upstream replay remains parent-owned.

## Current canonical correction: actual RED to GREEN

Evidence root: `usbox:/tmp/magpie-devin-canonical.oWwRN68w`.
These are scoped dirty API8 snapshots above committed
`9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, not host commits.
The RED snapshot contains the new ordinary regressions with old production.
GREEN projects exactly three existing-file changes onto that same snapshot:
trusted caller projection, SDK Chat history preservation and the existing
native-metadata test's trusted-context setup. Both new regression files
are identical in RED and GREEN. Complete path/mode/hash/symlink inventories
remain unchanged after checks, builds and installed replays.

| Corrective input/output | SHA256 |
| --- | --- |
| RED complete source inventory | `e7f76ae2b81d8ce02de518040896cd050728cd750ff254180782fcfeb139a69d` |
| RED ordinary host binary | `4d1a10278620144adca17081690ed9e221c19ed291b1144cab564cf2961c75ab` |
| GREEN complete source inventory | `c43421772d425ffc60705d66c186b66c40173efee1583e7f864cce65f4e198aa` |
| GREEN ordinary host binary | `2fe733387f1f2ad54bfde1583b1b1d26216796cc706908f6decd5182eaffaeba` |
| Unchanged paired0.5.0 library | `8fbb1835ebcb69c326855349cee1af2103fd50d1dfc111d8ef6d18b6c833d566` |
| Unchanged agent0.7.0 library | `587521112655ff09cb35da04e5ccefbc5c28c096efac624ebf1552ba8f8b313f` |

Inventory digests use SHA256 of UTF-8 JSON with sorted keys and compact
separators. Capture receipts retain the full inventories and the explicit
RED-to-GREEN projection. Module code and installed fixtures are unchanged
from `a045760bf2c116e6ad943f28290b4b407224e73f`, under
`usbox:/tmp/magpie-devin-api8.vK1K15Oc/r2/module`.

| Same actual installed fixture | RED | GREEN |
| --- | --- | --- |
| `messages_host_e2e.py` | First request403, zero native dispatch | First request200, one native dispatch; complete signature/stream/cancel fixture passes, five total native requests |
| `split-calls-and-reasoning-alias` | 400, zero dispatch; host creates alias conflict | 200, one dispatch; original source aliases preserved |
| `missing-real-result-refused` | 200, one dispatch; host fabricates result | 400, zero dispatch; missing real result refused |
| `split-cache-boundary-refused` | 200, one dispatch; host merges marks | 400, zero dispatch; unrepresentable boundaries refused |

Ordinary host tests fail behaviorally before the fix, not at compilation:
`TestExtensionProviderCanonicalCallerIdentity` sees absent/overridden
trusted callers; `TestSDKCanonicalCallerThroughFront` sees empty ordinary
identities; `TestSDKCanonicalChatHistoryThroughFront` sees merged fragments,
manufactured results and altered cache marks. The non-SDK positive control,
`TestSDKCanonicalHistoryKeepsLegacyRepairForNonSDK`, already passes on RED
and remains GREEN. It still joins split calls and inserts the interrupted
result for legacy providers.

The generic production change initializes `ExecuteOptions.CallerID` only
from `extension.CallerIDFrom(ctx)` and bypasses `toolPairsOut` for SDK
providers. Private native metadata/request IDs still require the same
module owner. No Devin import, signature fallback, weakened account/origin
check or write-identity/journal change is involved. Tests cover Chat and
Messages callers, same-key stability, another key and loopback owner,
body/header/private-metadata spoofing, mixed/same aliases, real paired
results, missing results and separate cache boundaries.

GREEN ordinary checks on usbox: the two regression selections, scoped
provider/gateway neighbors under `-race -count=3`, scoped `go vet`,
`gofmt -l` for all five Go files and ordinary `nogui` host build.
`checks.json` records exact commands, timeouts, exit codes and source audits.
Installed GREEN controls additionally include same-alias/simple requests,
all24 paired native groups with catalog restrictions/aliases/budgets/
readOnly/cancellation, and unchanged identity/no-op and real-transition
scripts. The no-op still admits one assignment only, refuses duplicates
before/after sync, preserves complete binding/journal and returns the held
read200. Real changes still fence stale payloads and held read/accepted-write
replies; the original accepted write completes once without replay.

| Aggregate receipt | SHA256 |
| --- | --- |
| `red/checks.json` | `ac98620a296cf329712e49d9c635d4b9cc3ff8dedabfd9d2c2d1378ad7a4bd35` |
| `green/checks.json` | `75db83fd12e9b3e029c9629a38d83854befd2d089feb6e97e8c7c3761788d1fd` |
| `red/installed.json` | `208822c4f43521324e17ce87e63aa2441284ec7495f561aa3898a166e342d441` |
| `green/installed.json` | `62792fb01094dbac6cf66b2be80ce062b0b5082bff3507503c862eb3cf5029a9` |

All installed fixtures record zero external requests, no mock-vendor
assertion failures, removed temporary HOME and a stopped fixture process.
The local receipt copy is `dist/2026-11-14-canonical/receipts.tar.gz`,
SHA256 `c41e4426d9368160002617bcf6a9b5da76fb791fda4da26e2f1e46955618afab`.
That ignored handoff directory also contains `host-canonical.patch`,
SHA256 `89e0ab2a0987ead1447f4e731aefbd6c2d71a238943b232c2990f07a0132382e`,
including both new regression files; `host-handoff.json` audits its exact
tested files. Its preimage is RED with those two new files omitted.
The capture/projection scripts and raw logs are retained alongside it.

At that checkpoint shared host changes remained uncommitted for the parent.
The parent's later extension-activation hunk was absent from this tested
snapshot and outside this patch. No result here is attributed to a later
mutable tree or final upstream freeze. Final combined
freeze/replay and broad/cross-platform acceptance remain parent-owned.
The earlier full-package cold CLI, reconstructed startup, usage/Facts and
diagnostic results below retain their separate source identities.

## Package-build checkpoint before canonical correction

Evidence root: `usbox:/tmp/magpie-devin-api8.vK1K15Oc/r2`.
This is an own two-pass capture of the parent's dirty API1.8 union above
committed9e88, **not a new host commit**. Parent owns final union acceptance
and commit. The module's consumed runtime floors remain1.7/1.1.

| Final build input/output | SHA256 |
| --- | --- |
| API8 complete source inventory,10,528 entries | `277f939d2bf971728abef073db3593243f7370768ab2d2c549ec3977a9664fb0` |
| Ordinary host, built outside source | `a08f805ea7cf4665dfab70143eb2b2869b3ed547c7388fa91b96344217746de8` |
| Paired0.5.0/API1.7 `plugin.so` | `8fbb1835ebcb69c326855349cee1af2103fd50d1dfc111d8ef6d18b6c833d566` |
| Agent0.7.0/API1.7 `plugin.so` | `587521112655ff09cb35da04e5ccefbc5c28c096efac624ebf1552ba8f8b313f` |
| Inbound0.2.0/API1.1 `plugin.so` | `3491b167bee764e3c305d94b0b1761df132ec2322128d40f723ceaaa85220e07` |

`build-host.json` confirms source inventory equality after build.
`module-checks.json` records complete module unit/race count3/vet,
five provenance tests and all native builds passing against this captured
SDK. No Go production/test source changed after those gates.
`module-go-inputs.json` records the tested Go inputs.

The same installed fixtures and authority assertions produce these actual
results. Original identity/transition gates use unchanged agent0.5.1,
SHA256 `22eefa631bb913a2e7ba1b74dd74045c62f78988b622edc6f75be4958bf69397`.
Every HTTP fixture stops its host/removes its HOME; external requests and
vendor assertion failures are zero. ABI conformance separately uses a
temporary HOME and closes its two native sessions.

| API8 gate | Actual result | Receipt SHA256 |
| --- | --- | --- |
| paired-http | GREEN, all24 groups including six catalogs/policy/cancellation and live account-generation replacement | `1026c1105124e2ae62d906ffd573fa17c4c5e602b3b9ecec8f74b071d1a6a28a` |
| agent-http | GREEN, cold CLI/no vendor calls, ordinary launch, four native operations, synthetic session/resume/isolation and cancellation | `cf225cf3dd54d1ba2cc9fa1e7d17a7d936a99e891a90fa3cd49f0736cfc8bb23` |
| original-identity | GREEN, duplicate403/one dispatch, stable complete binding/journal, held read200/exact `08 01` | `f0e191a5e1d3d482ac5e176b7ce1da3dcd546e827b235ec601eaa1c154496ad2` |
| original-transition | GREEN, real model/revision fences intact | `92b8504e19d6ae9f7887ea736adf817acba213f8eb20a02cfff7f35a96366719` |
| current-identity | GREEN, identical no-op guarantees with final agent library | `d81260973bcd7e2b98b5e35fcc88d9182cc77a5759c3f5b2e39015fe94b8ba8a` |
| current-transition | GREEN, stale payload/held read/accepted write controls intact | `06c6cfb18895531b48d47bf0ba34f68eec4633563abc0e79d105c8d9d89d783c` |
| inbound-abi.json | GREEN, actual two-session C ABI and installed SDK CLI decode | `caa68d474d0f2310be4e56372b0d3c53a6dc8480ff0d9f9fa8bedd3e5069e4ba` |
| execution-facts-ledger | GREEN, measured counts/units/attribution and four scoped diagnostic records | `e72674dda873c1a2c6314cb217a75add411043add52a5c9fb2f22aded64994e6` |
| acu-status | GREEN,30%/explicit zero/unknown absence/reset, zero inference | `a56c4ac9832b64f801ecc2fe4338541f0956f8bd6dd4dd17f1442beb8d17010a` |
| canonical-simple-control | GREEN,200/one native dispatch | `d813fb4d8b0e61d1d6218b1467e5508ddf48edea7da44b3e8a2c109d684d302a` |
| split-calls-same-reasoning-field | GREEN, real calls/results preserved | `496316c8ff0f69d4ff76ea2348a6de5f971a6ea3f22d7f7e427b55eb68804d4f` |
| conflicting-reasoning-refused | GREEN,400/zero dispatch | `412c316bf9742ae4dc6fa83bc54d6ebf0ce46009e59a28f1d8a30498c07efdc9` |
| deferred-tool-images-preserve-user | GREEN, real image/text owners preserved | `811a10283365081196172a3028e42106329f77c2dd06c0654c2ed9880ec7c00a` |
| incomplete-deferred-images-refused | GREEN, zero dispatch | `7dffc79d821e20f0cc80508f859cb0789f48bc91529fe525040a94f3abb7d93a` |
| claude-unsigned-thinking-suppressed | GREEN, no fabricated signature | `a66aab96f3bdabd143e43ddcd0520180c05824918ea480310202e281627b9048` |
| reconstructed-startup | GREEN, ordinary launch/status/first native turn; Rust reconstruction only | `8de9df8ab1786b6fa73874b077c2d4b61add40ead211b1e420f42dd86bb8c6e0` |
| messages-red | Actual acceptance RED,403/zero dispatch; missing host CallerID | `d1c7d5d4faef4191d2b166d310ed8ed482c81a45dac102223ce98e1d487537a1` |
| split-calls-and-reasoning-alias | Actual acceptance RED,400/zero dispatch; host-created alias conflict | `8f06f93fef3f84b624f92088149b34260cf717b6e71b8028306f3e0cd9e0a93c` |
| missing-real-result-refused | Actual acceptance RED,200/one dispatch; host-manufactured result | `bdfd3f9a80ac74837293244ef2d38bd3df513e6d06183b438c820c16556f710e` |
| split-cache-boundary-refused | Actual acceptance RED,200/one dispatch; host-merged marks | `116ca88227c8a5662ad7bb0ad03dd5fa4d328d8f8c069250f69ea1ffbc12b40a` |
| execution-facts-old-red | Behavioral RED, older pre7 library on same host/fixture lacks rich measured metadata/units | `9dbb8a6c24f676be116e771ae9468f77fdb68eefa7c1bc0e768403e3524785e3` |

`installed-gates.json` preserves commands/bounds/exit codes. Expected failure
of a known acceptance case is still recorded as **RED**. It is never counted
as successful Messages/history acceptance.

The first API8 capture, source
`d6f34a6d73f7de2e868691d7afeddd00089a629455f9bf811367e963e42a0edc`,
is retained at the parent evidence directory. Its host build failed because
`outsideRead` was declared in the wrong scope in `internal/extension/manager.go`
(unused717, undefined927/928). Its module tests/builds passed. Parent
corrected shared source before the separate final capture above. No host
production file was edited by this executor at that checkpoint; a build
failure is not a protocol acceptance red.

## Earlier approved base and captured API7 runtime

Committed base: `9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, tree
`4f6525135603b84cdc552403923f39b44fa1dea4`.
The API7 source includes parent dirty files and has **no source commit**.
Its own immutable capture is distinct from that base and the parent's
mutable startup directory.

Evidence root: `usbox:/tmp/magpie-devin-final.sRO8tXBQ/v2`.

| Input | SHA256 |
| --- | --- |
| Complete API7 source inventory | `4c17a1a74eec69ab020c39661c0d02244452a8fdae049e9d4a24453b69337c63` |
| Ordinary `magpie-host` | `42f77c642342f8b24d68d8b8fec89972037c1691d0b10392ce74031931d8a203` |
| Paired 0.5.0 / minimum API1.7 | `f890a89b8e09443733a2c2f1e3cbcadbc717e07160c72a002c5321f72551fa4f` |
| Agent 0.7.0 / minimum API1.7 | `a8feafe1f3fd78b4a3110081753697a9ba73c3603b8a5347462d82046c6e1a18` |
| Inbound-only 0.2.0 / minimum API1.1 | `ba2a26681e844b0713cc9d042d2b4030acc683763691ecf6f05faa1bff1a4a40` |
| Original unchanged agent 0.5.1 | `22eefa631bb913a2e7ba1b74dd74045c62f78988b622edc6f75be4958bf69397` |

`host-capture.json` contains all 10,498 file/symlink entries.
`host-postbuild-audit.json` confirms complete path/mode/hash equality after
build and the same source/binary digests. Source hashing uses sorted regular
relative paths, UTF-8 path + NUL + decimal permission bits + NUL + binary
SHA256; symlinks are checked separately. Outputs are outside source.

## Actual installed gates on that runtime

`final-checks.json` records unit tests, race count3, vet, five provenance
tests and three native builds passing. Go is 1.26.8, CGO is enabled,
dependency resolution is readonly/offline, and wrapper HOMEs are removed.
`installed-gates.json` records exact commands, bounds and receipts.

| Gate | Outcome | Receipt SHA256 |
| --- | --- | --- |
| paired-http | GREEN,24 groups; mapped library, account replacement, six native catalogs with restrictions/aliases/budgets/readOnly, JSON/SSE/raw native, signatures, strict controls/cancellation | `0ca6f039237675a6103cd2ac7bdd53c96a475501fe86b5233256668a7bd3d5fd` |
| agent-http | GREEN; cold CLI with gateway stopped/no vendor calls, ordinary launch, four account operations, synthetic session/resume/isolation, cancellation/offline restore | `9a4eee1a539f8083a7a2b013339a98a992b687605b80415c148fc9c29a988b0c` |
| original-identity | GREEN, original agent library | `bd6003e73dfdf5c84f2d33b7fcb59c0ac206977b5e3ecac1cff8feaffcf11b6e` |
| original-transition | GREEN, original agent library | `16add160761974cf67730d2bb95378ef237fa056ca9fc3c16544e729126576cc` |
| current-identity | GREEN, current agent library | `13ebee294f25d0f4c4deb627925299a72ebcd9aea1886313fdb8f56efcc91531` |
| current-transition | GREEN, current agent library | `6608571a6b69bf37211f7cf8d0dcb7ddd731bac65baed015ad57a4366f973062` |
| inbound-abi.json | GREEN; actual C ABI/two sessions and SDK CLI install/decode, no Bun | `c88f48faa260727869f905d5729f70c1abf7ea6b49f74e8ace8681a2ea9adb17` |
| execution-facts-ledger | GREEN; actual ledger and scoped diagnostics | `3fbaaff44504827cca6975a1bae9a2cd6d029cb0f49edc850b1b943e6b5560b1` |
| acu-status | GREEN; Aside/reset/explicit zero/unknown absence, no inference | `a51554af96f6988bf5a76210d477a7f6cac2571ca01a35fbfecf55f8e051cfae` |
| messages-red | Actual acceptance RED,403/zero dispatch; omitted canonical CallerID | `5e09f3b0c8f1aacbbee334d9dbe1df9fe1a32e323afc70d42f5c98920e1fada7` |
| execution-facts-old-red | Behavioral RED, same host/fixture with older pre7 module missing metadata/units | `a30bb0e50f7859323fc02f3269ee253f43fc8c61b8503c703738fc1d8372f9a9` |

Identity gates retain exact RequestID/Session/body/account inputs.
First AssignModel is200/one dispatch. Duplicates before/after ordinary
no-op sync are403/one total dispatch with complete binding/journal unchanged.
Held GetCliTeamSettings returns200/exact `08 01`. Real model/revision
transitions still refuse stale payloads before vendor and fence held
read/accepted-write replies. The accepted write completes once in its
original journal despite outward403. No retry, removed revision input or
weakened Check is involved. `NOOP-ACCEPTANCE.md` preserves earlier proofs.

## Execution usage and diagnostic fidelity

The installed counted call records fresh input7/output3/cache read11/cache
write5; Chat keeps inclusive prompt23. Its zero control records all four
explicit zeros. Both ledger rows keep endpoint, selected model, caller key,
session, native request ID, actual canonical response ID, served model,
`API_PROVIDER_ANTHROPIC`, native `STOP_PATTERN`, committed credits2/0 and
reported cost2.5 in `ACU`. There is no USD OwnCost override or conversion.

Two calls produce four actual scoped application diagnostic records:
accepted/dispatch and completed/status200 for each. They contain fixed
local codes, not raw stderr, errors, bodies or credentials. SDK-peer tests
separately prove measured snapshots survive failed terminals and diagnostic
refusal cannot change dispatch/outcome. Cancellation/revocation can prevent
a final callback; no detached reporter retains credentials after a lease.
`EXECUTION-FACTS.md` maps supported facts and absent native evidence.

## Independent current donor and startup replay

`domain-current-gates.json` repeats independent cases on the same exact
runtime/libraries, with one own bundle and zero external requests.

| Case | Outcome | Receipt SHA256 |
| --- | --- | --- |
| canonical-simple-control | GREEN,200/one dispatch | `00069a48cc66b4217112f64eace8673d15477c65b8a36cc06f7332eefddca4ab` |
| split-calls-same-reasoning-field | GREEN, real call/results preserved | `a36909bfeda3b0d4c444fe5b35c27a1062858b4e447d24952d1b4fd2df451a92` |
| conflicting-reasoning-refused | GREEN,400/zero dispatch | `60731e24873b535648e819def6e2cea4d428f33a34095101408e72dbbfdaedad` |
| deferred-tool-images-preserve-user | GREEN, result/user ownership preserved | `6c6247aced6394edaa89d328a5dab0ef8439618cca1f9328beb0f889f40b393d` |
| incomplete-deferred-images-refused | GREEN, refused before dispatch | `fe136d8c0db7ad799ae84c85e90e4b5b90c72f675c4a0a05f926d6310120d780` |
| claude-unsigned-thinking-suppressed | GREEN, no invented signature | `2274dfc66ab304b3ba4df75681901a81b93fc582f9f00460add83af6346585b1` |
| split-calls-and-reasoning-alias | Actual RED,400/zero dispatch; host creates conflict | `6beeec2093f3ddf0cdaa80bee742bf62ac31c56173eee51ec68200031bdfec23` |
| missing-real-result-refused | Actual RED,200/one dispatch; host manufactures result | `65461111bfcc0775e342741a3e7376c531a4db141f01ec5c84677ee42aab4273` |
| split-cache-boundary-refused | Actual RED,200/one dispatch; host merges marks | `5c8e4569fe4e0b069f2f90fd04252a2979b073af268c726b55130601f50ea669` |
| reconstructed-startup | GREEN, ordinary native launch/status/first turn | `a24bfb6359a55cbfecfd62e7c2ae3793a2f6c77f8a91f2322b174b9d18503de4` |

The client is the unmodified **Rust reconstruction**, executable SHA256
`8b5438632942381272d20974f7ee59fd9af7674d452dce7ea448077b4c075327`.
Loopback guard:
`c7012e29410d0585e675caef74ceef84216ac0e4e2e6210ea3d8ae863e98279b`.
Synthetic credentials/trust exist only in the fixture. This is not original
Cognition client or live-server compatibility.

## Historical0.7 acceptance boundaries

The standard0.8/API9 continuation implements default-off paid account tasks.
`CACHE-KEEPALIVE-EVIDENCE.md` records its distinct module/host inputs,
measured canceled-usage acceptance and three current host gaps.
`GEMINI-SIGNATURE-EVIDENCE.md` adds the exact0.1.41 port and its installed
Messages limitation.

The four CallerID/Chat-history defects are resolved on scoped sourcec434
above; `HOST-CANONICAL-GAPS.md` retains their historical diagnosis.
Shared host commit/final upstream replay and broad acceptance belong to the
leader; SDK-only rollout is independent of optional Devin acceptance.

Remaining donor work is not all live-only: family/effort/fast virtual model
projection, safe schema normalization, unsupported Messages controls,
numeric quota display, remote/group
delegation and additional native endpoints remain incomplete or refused.
No refresh/revoke endpoint or full native/cloud resume contract is invented.
Signature receipts cover this module's successful scoped executions only;
imported origins remain refused.

Original Cognition/live account/server acceptance, other operating systems
and the leader's final committed union/broad suite remain unverified here.
Historical raw reds remain retained. Passing fixtures do not establish
blanket bootstrap, donor or native parity.
