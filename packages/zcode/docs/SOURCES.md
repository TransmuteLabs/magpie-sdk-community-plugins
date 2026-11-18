# Source provenance and compatibility

Machine-readable provenance is [source-lock.json](source-lock.json).
Refs name exact commits; the verification host additionally pins its
committed tree. Source repositories remain
read-only; no dependency installation or vendor account was used.

| Identity | Exact baseline |
| --- | --- |
| Our committed readiness module | `8143212`, headless 0.6.0 / optional UI 0.8.0 |
| SQLite checkpoint | `835a75e`, headless 0.6.1 / optional UI 0.8.1, native ABI 1 |
| Confirmed CN/Start checkpoint | `3d949ff`, headless 0.6.2 / optional UI 0.8.2, wire client 3.18.2 |
| Accepted owned-agent checkpoint | `ffa65a5`, headless 0.6.2 / native UI and agent 0.9.0 |
| Bounded donor-parity checkpoint | `f6c708f`, headless compatibility 0.7.0 / native UI and agent 0.10.0 |
| Ordinary full package | 0.15.0, provider/controller/agent/UI and rooted CLI history together; `scripts/build.py --output DIRECTORY` with no selection flags |
| Current provider-only package | 0.7.3, explicit `--headless`, same domain fixes and public policy |
| Runtime SDK floor | both current compositions 1.12; historical receipts retain their original API4/API9 floors |
| 2026-11-16 baseline regression repair host | parent immutable local candidate-05, 11001-file source SHA256 `0db2f2e148b7263e196db2e1e4f40e2fd3a4bfba31fddf842a9417e6b57f4926`; separate from earlier native/policy freezes and final combined acceptance |
| Current public policy SDK source | `sdk-request-policy12`, 10987-file physical snapshot SHA256 `e2ca42c6fa8529b781a61aec04862355a5b28183646cc57ee028f6133fb74c34`; capture base `705d67efc78f72cdc63973318ecaaaa661c1e04e` is not the physical tree |
| Previous native lifecycle base | `c11acbc2bebec1fb213ef562900d35188cdf55fe` |
| Previous frozen physical source tree | `8fe78ab9df13d733d61462eacce19bff523c1e58`; later test-only corrections and future HEAD are separate |
| Previous frozen host binary SHA256 | `c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86` |
| Previous unchanged lifecycle module | `c8281b6c97d751207f429bcbc1861daa2c465abb`; its production and endpoint-overlaid libraries were reused byte-for-byte |
| 0.14 candidate replay source | physical tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, 10846 paths; `adcf4ce5` is parent at capture, not merge/final acceptance |
| Candidate archive SHA256 | `5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc` |
| Candidate binary SHA256 | `0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509` |
| Automatic-history API9 merged base | `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`, committed and physical tree `94ec4c8de4e5b366a6e3fb854df2176007587ab8`, 10849 paths |
| Base archive / binary SHA256 | `131aeaa8fa9052dab47854ead8cb8b57f7f4dadca92715dbad307f4424c2de9e` / `a8e09770909f78ae89cc6175a45bb98e1dacdb305b62856f46956928cb8ecbe2` |
| Previous API9 agent-options compatibility host | `46756a48c9c9f204a8369638dc63cbcb83d02031`, physical tree `db40ddbf8944b54f9f689d444644af4c3bc76881` |
| CN/SQLite compatibility host | `831c3d74af9358fa8a45d77eed902fe7cd02a076` |
| Owned-agent frozen SDK 1.6 host | `d07078c7926daf6f33af09c40297c941bd39da92` |
| Previous committed verification host | `9e88f10982a1d66c1b402fba27caddda8ac4ac4e`, upstream merge `75fa28b10`, SDK-only startup and effect-free agent sync corrections |
| Previous committed host tree | `4f6525135603b84cdc552403923f39b44fa1dea4`; 0.11 package evidence |
| API7 adoption verification source | owner's immutable, uncommitted API8 snapshot `43b7e290d9c3370881780b23ad49222f5d27f44b`, based on `9e88f10` |
| API7 verification archive SHA256 | `dd5bf036acdbb39bf1d93a415816b4ccea5700c40c49f2f430eea3dacd411454`; final committed host acceptance remains separate |
| Previous actual vendor baseline | ZCode desktop 3.18.0, CLI 0.17.1, `b95ebaffeab641907b38652f45c1502bd762972d` |
| Current local vendor diff target | ZCode desktop 3.18.2, CLI 0.17.1, `fec975905a2abd4c07e35c40d51fa097a2258ec9` |
| Controller/archive donor | `893f560cccdeb8fa2e98eadb60427990c10eedcc` |
| Community comparison | package 0.1.61, `26ea8faf7793ca66dd8e7a8b90128a5235552a1f` |
| Bounded donor update | package 0.1.66, `87a8cf03c62fa55ca845c51648e8859ece580336`; exact 0.1.63/64/65/66 update commits in source-lock |
| Previous community authority | `57d1cc6a9f49389d495ea9be8d986c70de7767bf`; ZCode tree `d1be84b45f19c7d51bf3bf1c428ddc7ae95dfbf7` equals the ported 0.1.66 tree; donor unchanged |
| 2026-11-16 community no-op audit | parent-fetched `b35d4b35b2446bd929896ddeb58f3cb299e9e2d1`; the same ZCode tree and package 0.1.66; no runtime delta |
| 2026-11-16 vendor no-op audit | parent-fetched `fec975905a2abd4c07e35c40d51fa097a2258ec9`; unchanged full tree, desktop 3.18.2 / CLI 0.17.1 |
| 2026-11-17 bounded community donor | `324acd6007f2d513c5072fe2e9fe1db3524ae03d`, package0.1.68, ZCode subtree `91595e647e66e6b45105ce45fcea2291b85d0634`; original extraction pins remain unchanged |

The0.1.68 adoption reuses the existing Start transport admission owner for
account3 plus6 per exact origin/proxy route and omits the provider's former
two-account default. Explicit host/user limits still win. The same single
wait deadline, front requeue and idempotent release apply to both reservations;
fully consumed or closed Start replies release before a confirmed paid
fallback continues. Plain free-first chooses paid when current account/route
Start capacity is full; forced Trial and manual Start do not acquire paid
authority. Host retry admission, logical budgets and durable outcome fences
are unchanged.

This is per-module-instance admission for known transports, not a global
physical-IP limit across overlapping workers, processes or other apps.
Donor0.1.67's enc:v2 desktop-store diagnostic was inspected but does not
apply to this module's explicit polling/snapshot authentication; no automatic
desktop store or Keychain reader was added. The new source-lock entry names
this donor separately from extraction and historical acceptance.

Bounded author verification passed on package identity
`0ebae38a4bca56c1ae75d2afa7dc21237636072871d245023103c1e5094dee7f`:
selected race20 has1620 named pass events; the full Go package tree has530
named test pass events and six package passes, with three command packages
having no test files. The single body-release mutant produces the actual
named route-capacity assertion; vet passes and gofmt-list is empty.
Independent Aristotle review is bounded source/terminal-evidence positive,
receipt `TERMINAL-CLOSURE.txt` SHA256
`e2364854061c13dcc069726acbbc1d8c2e210aa96bc63482adb0d67f9ddb6ba7`.
The exact author delivery/seal and review location are recorded in
`community-zcode-0168.verification` in source-lock.json. Original business
reds and the separate repaired fixture failures remain in that sealed
evidence. This current metadata reconciliation preserves the reviewed
runtime/tests; it does not relabel the tested identity or establish
installed-host, live client/vendor, distribution/release or full GUI
acceptance.

Our module version, vendor desktop version, vendor CLI version, SDK
contract floor and built ZIP identity are different values.
[DONOR-NOOP-20261116.md](DONOR-NOOP-20261116.md) records the post-API1.12
read-only comparison; its checked refs introduce no behavior or version bump.
`credentials.go` and EXTRACTION.md prove the implemented vendor baseline
was 3.18.0 before the explicit unlinked-Start port; 3.17.2 (`a9fe401e...`) is older history, not the correct
starting point for the current vendor diff.

[VENDOR-DIFF-3.18.2.md](VENDOR-DIFF-3.18.2.md) records the bounded domain
comparison and required semantic ports. A checked source revision does
not mean every changed behavior is already implemented.
[COMMUNITY-PARITY.md](COMMUNITY-PARITY.md) separately records the provider
plugin fixture comparison; installing that plugin is not an integration
step. [ONE-BUNDLE-AUDIT.md](ONE-BUNDLE-AUDIT.md) describes the one-install
requirement and the bounded owned-agent cut. Its source-shaped plans use
the recovered provider codec and CLI path preparation; the executable
used for launch verification is explicitly synthetic because the actual
ZCode client is absent on usbox.

The current gift/claim/Start-risk port additionally checks
`zaiStartPlanBilling.ts`, `marketingTouchService.ts`, the shared marketing
action/protocol schema and the BigModel subscription provider at the
same desktop 3.18.2 revision. `source-lock.json` records hashes of those
exact `git show` bytes and both desktop/CLI package versions. The
community source supplies gift aliases, acknowledgment/refusal behavior,
feed-only check-in and adaptive Start risk controls as a separate donor
lineage. [PARITY-EVIDENCE.md](history/PARITY-EVIDENCE.md) records compiled RED,
green and behavior-negative evidence; the historical agent evidence is
unchanged. Later shared-host freeze acceptance is owned separately and
is not inferred from the pinned d070 runtime runs.

The 0.11 continuation also ports Personal→Team→Start selection, manual
gift discovery/routing, actual gift response aliases, held-route bypass,
source-shaped Start requests, turn/trace attribution and execution usage.
Per-plan best-bucket and pending gift windows preserve model applicability.
Native CLI logs and optional SQLite model/turn/tool usage rows retain
source bytes, raw usage, attempts, nulls and large integers. The ordinary
page adds offers and explicitly confirmed check-in.
The added source-lock hashes name the exact context builder, attribution,
normalization, usage schema/repository and log writer/serializer/retention.
Static prompt-section hashes are pinned independently in protocol tests.
None of these source checks establish actual-client execution.

The previous host archive is committed `9e88f10`, including upstream
`aba6aeac22c1fb943c27308a531f36a5fe1a2e4e` via `75fa28b10`.
Its committed public SDK matches the initial `7fbda87` RED verification
base; `9e88f10` adds the host owner's effect-free agent sync authority fix.
The 0.11 acceptance uses that exact archive. The subsequent 0.12 API7
adoption uses the owner's separately inventoried `43b7e290` source
snapshot. Neither the earlier d070 acceptance nor this uncommitted source
snapshot proves final committed SDK release acceptance.

The normal package selects the full SDK 1.12 manifest for public policy, provider defaults
and rich quota metadata, retaining optional API7 execution Facts and diagnostics. The separate
`--headless` option exists for historical compatibility fixtures.
[PACKAGE-EVIDENCE.md](history/PACKAGE-EVIDENCE.md) records the default-package
RED and the final ordinary ZIP's inventory and native acceptance.
The subsequent 0.11 donor, log/usage fidelity and native rollback gates
are recorded in COMPLETION-EVIDENCE.md.

When advancing a source, update its exact lock entry, inspect each watch
surface, add or rerun its behavior regressions and record the resulting
source/artifact hashes. Do not replace refs with a branch name or a
floating latest version. Existing SQLite runtime libraries remain pinned
to the host lock; uncached dependency-test metadata is not a reason to
silently change library versions or substitute another runtime.

[API8-COMPAT-EVIDENCE.md](history/API8-COMPAT-EVIDENCE.md) records a subsequent
compatibility run of unchanged module `1eff0f0` and the ordinary full ZIP
against an immutable snapshot of the shared API8 working tree. It names
that snapshot's complete byte/mode inventory independently of the
committed release lock. This documentation-only continuation does not
advance the historical 0.11 package provenance or adopt optional
telemetry/storage capabilities.

The 0.12 observability port checks the exact CLI raw usage observer,
model execution/request-ID handling and bounded diagnostic summaries,
alongside the desktop capture/replay source. The lock names each checked
file hash. Raw execution counters, cache TTL, reasoning and identifiers
are distinct from account quota units and client-local monetary history.
The available Messages source does not establish per-execution credits,
cost/currency, deferred usage or an upstream provider, so the module does
not manufacture them. Source, installed-host and causal evidence is in
[OBSERVABILITY7-EVIDENCE.md](history/OBSERVABILITY7-EVIDENCE.md).

The 0.13 port keeps the same desktop 3.18.2 / CLI 0.17.1 and community
0.1.61 donor commits. The community `spanOf`, `limitWindows`, `giftOf`,
`periodOf` and provider defaults supply count/display/period semantics and
the three-request/two-account defaults. Numeric quota period codes do not
name a currency or credit unit. The full package emits exact reported
counts and periods; the explicit historical API4 surface omits API9 fields.
The separately inventoried API9 source snapshot and scoped parent seam
proofs are recorded in [API9-EVIDENCE.md](history/API9-EVIDENCE.md). They do not
replace historical API7 acceptance or final committed-host acceptance.

The 0.13.1 bugfix compares four exact community updates without checking
out the donor: CN identity `3f37b858db8074959447c16ed06ac51d444c7fcf`
(0.1.63), Start requeue `8e19b6a0e712e4ea844ccd5e3fe95522f570ae05`
(0.1.64), unclaimed Trial explanation
`1dbbc1fb489d7ac40b0620d802301075e25332b6` (0.1.65) and gift billing fences
`87a8cf03c62fa55ca845c51648e8859ece580336` (0.1.66).
`community-zcode-0166` retains checked production/test file hashes,
versions and watch surfaces alongside the earlier 0.1.61 source lock.
Selected-account CN identity and failed-plan Discovery fences were already
protected on unchanged 0.13; new oracles demonstrate them. The bounded
port adds current manual-Start gift applicability, front requeue within
one deadline and cached read-only unclaimed-plan hints.
Unknown gift balance still fails closed, stricter than the donor's
permissive send. Desktop 3.18.2 / CLI 0.17.1 and the immutable API9 SDK
source remain unchanged. [DONOR-0166-EVIDENCE.md](history/DONOR-0166-EVIDENCE.md)
records failures, controls and package identities without implying actual
client, vendor, financial-resume or final committed-host acceptance.

The earlier API9 agent-options compatibility slice uses the frozen
`46756a48` source. `sdk-agent-options9` pins the public decision-model
DTO/validator, effective chat metadata and exact ordinary host wire fields.
The strict module decoder accepts these known fields while rejecting
unknown or malformed options. Decision facts remain separate from chat
model configuration and selection. [AGENT-OPTIONS9-EVIDENCE.md](history/AGENT-OPTIONS9-EVIDENCE.md)
records the old failing fixtures and bounded verification; no host edit or
actual-client decision integration is implied.

`sdk-final-native9` records the historical native lifecycle repin on the
exact `c11acbc2` base, `8fe78ab9` physical source inventory and `c018c443`
host binary. It retains the `c8281b6` lineage fixture, both existing native
libraries and the earlier RED evidence. Only package provenance sidecars
are refreshed; this does not rebuild native code or rerun the full domain.
That module was one ordinary 0.13.1/API1.9 provider/controller/agent/UI
package. This historical pin does not follow future shared-host HEAD or
establish acceptance of 0.14.0.

The 0.14.0 continuation and candidate replay are recorded separately in
MODULE-COMPLETION-EVIDENCE.md. NATIVE-CONTRACT-REQUEST.md retains the
historical generic resume/process proposal. Current module adoption of the
existing API1.10 source recipe and AgentProcess callbacks is documented in
NATIVE-FOREGROUND.md. Its frozen host identity is separate from the earlier
API1.12 transport-policy source and from parent's final combined acceptance.
API8 rooted SQLite and API9 foreground account tasks retain their owners.

The independent 0.15 automatic CLI history slice consumes those existing
rooted storage contracts. `vendor-sqlite-history` pins the current official
path, migrations, codecs, ordering and message semantics; `rooted-vfs-donor`
pins the read-only sibling VFS/Unix lock files adapted into this repository.
The sibling is not a runtime dependency. Scope, independent evidence and
the existing-owner scanner handoff are in AUTOMATIC-HISTORY.md,
AUTOMATIC-HISTORY-EVIDENCE.md and AUTOMATIC-HISTORY-HOST-HANDOFF.md.

The 2026-11-16 foreground adoption keeps those same donor versions and uses
the existing API1.10 process/source-resume contracts in frozen host source
`9c69eb56fd9d5f71a571139de8aa1912453109df6deaa532b1684d820092a226`.
`sdk-native-foreground-20261116` records that dirty snapshot separately from
its base commit; `vendor-native-foreground-20261116` records the exact V4
schemas read locally. NATIVE-FOREGROUND.md and NATIVE-ADOPTION-EVIDENCE.md
describe synthetic installed acceptance, callback-join ordering and limits.
This is module adoption, with no donor delta or version bump.

The subsequent package baseline repair is recorded in
BASELINE-REPAIR-20261116.md. It corrects UTC HTTP-date fixtures and atomic
first-use durable lock creation under the same public API1.12 contract.
`sdk-baseline-repair-20261116` pins the supplied parent host05 independently;
earlier policy/native receipts retain their original exact source identities.
Package, vendor, SDK and ABI versions do not change.
