# ZCode 0.13.1: bounded community 0.1.63–0.1.66 closure

2026-11-14. Module-only continuation of `3c0f4870c7b14e0ffc5f19754f222dc787751752`
(full 0.13.0/API9). No host edits/commit, donor checkout, branch/worktree,
new dependency, push, client install or live vendor/account access.
The ordinary distribution retains provider, controller, owned agent and
UI together in one native package. This is local package acceptance,
not complete actual-client parity or a published release.

## Exact source comparison

The donor checkout remains clean at
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f` (0.1.61).
All changes were read with Git objects rather than checkout:

| Commit | Package | Behavior and result |
| --- | --- | --- |
| `3f37b858db8074959447c16ed06ac51d444c7fcf` | 0.1.63 | Active CN identity. Our explicit selected-account ownership already rejects a different user with the same display name before entitlement/network mutation. Same-user CN renewal retains `zai_cn`, UserID and account ID. Both oracles pass on unchanged 0.13. Automatic cached-app-profile import is still absent. |
| `8e19b6a0e712e4ea844ccd5e3fe95522f570ae05` | 0.1.64 | Start beside inflight. Old code narrowed the gate but returned the refused request after its retries. Sustained known unused caps now release the slot and requeue at the head, sharing one two-minute wait budget and at most four requeues. |
| `1dbbc1fb489d7ac40b0620d802301075e25332b6` | 0.1.65 | Missing Trial/weekend explanation. The existing marketing feed supplies up to four distinct unclaimed names, with a three-second read bound and ten-minute selected-session/account/generation cache. Failed reads are not cached; spent declared buckets do not read the feed. The explanation claims nothing and creates no quota. |
| `87a8cf03c62fa55ca845c51648e8859ece580336` | 0.1.66 | Gift-only exclusion and failed-plan billing. Discovery already requires current paid/gift authority and fails closed on failed plan reads. New old-source oracles prove both. A real manual-Start bypass did exist: it accepted known models without checking the gift balance. Models and dispatch now require current named gift authority. Unknown balance refuses dispatch. |

Observed donor `origin/HEAD` is
`a60667b57040f62080da60f6018cc7351fa1953f`; its `packages/zcode` subtree
is identical to the bounded 0.1.66 target (empty Git diff). The additional
`community-zcode-0166` source-lock entry retains all four exact commits and
versions, nine checked production/test hashes and regression watch paths.
The historical 0.1.61 lock remains intact.

Vendor source stays desktop **3.18.2 / CLI 0.17.1** at
`fec975905a2abd4c07e35c40d51fa097a2258ec9`. Checked source bytes,
package versions and the new donor bytes pass the provenance reader.
The community source is a behavior donor, never a runtime dependency.

## Financial and replay boundaries

Manual Start previously bypassed `startEntitlement` before ordinary
inference. It could send GLM-5.2 even when the gift named only
GLM-5.3-Flash. This concrete billing exposure is closed: current balance
applicability gates model discovery and dispatch, with no accepted
inference for excluded, spent or unreadable gift quota. A newly confirmed
gift model remains usable. Confirmed Discovery gift-only selection and
failed paid-plan reads cannot create paid authority.

The module deliberately retains a stronger financial fence than the
donor's unknown-balance send or failed-plan paid fallback. Trial never
spends paid quota. Ordinary free-first fallback still requires a fully
read output-free 1005/1113 refusal, settled principal state and freshly
confirmed paid capacity. These conditions were not widened.

Only complete output-free HTTP 429 codes 3008/3009/3010 can trigger
Start requeue. Lone caps retain the old bounded retries and route-hold
accounting. Output/usage/choices markers, including measured zero,
partial SSE, uncertain transport and unknown codes never gain replay
authority. Queue-front order, cancellation, exact release and an expired
budget on an otherwise empty lane are asserted. Accepted is emitted once;
the final response retains its slot until the caller finishes reading it.

Review of the first green freeze found an additional entry-path gap:
Discovery Start/inactive-paid selection with no gift returned from
catalog/paid admission before reaching the Trial explanation. A second
compiled RED covers both account kinds. Explicit Trial selection now
uses its own current gift authority, without requiring paid/catalog
admission. It still refuses the request before acceptance when no gift
exists. The ordinary Discovery paid-plan fence is unchanged.

## Frozen source, package and verification host

| Identity | Value |
| --- | --- |
| Ordinary full package | `dist/zcode-0.13.1-donor0166/magpie-plugin_linux_amd64.zip`, 0.13.1/API1.9/native ABI1 |
| ZIP SHA256 | `8faa4ef30fa4087193ab4f1a5b1d6614b92fe6a8b46d9904390c00c3d286305a` |
| ZIP inventory | 19 members; 9,825,778 bytes; one native library, static UI, owned agent; no community runtime |
| Native library SHA256 | `68a7a52c326f82f0bbb3c1ca772b92189c3b757898c16235676118f2e5b6c727` |
| Explicit historical compatibility ZIP | `dist/zcode-0.7.2-donor0166-headless/magpie-plugin_linux_amd64.zip`, 0.7.2/API1.4 |
| Compatibility ZIP SHA256 | `a3edf1237d11518da81080e2992bcd9147e91e356465d8c5d871715d6aabaa64` |
| Final module input | 189 frozen source files, before this narrative and the evidence collector |
| Input inventory SHA256 | `5959a69dc44ad86004cf82bac29879119d126dc9cba04218dfa2490408f90f8c` |
| Input archive SHA256 | `1d0d2014cc448661026ec2e475bcc5a9a57c5fccc636a325d10ee6b7e653f118` |
| Source-lock SHA256 | `23f7a880f930827419c8f36edbb89f1feeef626dfe86103d02912e7ed50f9b9c` |
| SDK source base commit | `ae957e5a8a77553a6cf6f484c8b58a69393f3887` |
| SDK content/mode snapshot SHA256 | `fe82cca0a913ea8a4307478050e241747c555524353221fdab81d43458f8ce16` |
| SDK source archive SHA256 | `efcd0cdb05b61399e818641b570479bccabbfc422bde5c5f8d442ece54bcd57c` |
| Actual host binary SHA256 | `5ff7e035fc898c9c5fcdd5485caad2a6eddcc3b3b9a8a87ddeb9014b58756f24` |
| Aggregated acceptance SHA256 | `7132002867b709fa8f4eb1eb8e4506b8ef98a51aaf2491b4da35d54e39cf588b` |
| Preserved evidence archive | `dist/donor0166-3c0f487/donor0166-evidence.tar.gz`, SHA256 `10c9cfb70f13f5e8fe62de9c4604ad0b488dd8c9a1349121be59ac7fbdedf88d` |

This is the same **uncommitted immutable API9 union snapshot** used for
0.13.0, not a final committed SDK release. Its 10702 source bytes/modes
remain unchanged before and after every verification stage. It retains
the independently reported cold AccountStamp finding; parent integration
of the accepted hunk/unchanged probe is owned separately. This module
run does not claim that integrated closure or alter shared public fields.

All builds/tests ran on `ssh usbox`, Go 1.26.8, existing offline caches,
isolated HOME/XDG, UID/EUID1000, all five capability masks zero and only
loopback in the network namespace. Native root ownership was not relaxed.
No host broad suite or host rebuild ran.

## RED, GREEN and causal controls

| Gate | Actual result |
| --- | --- |
| Unchanged 0.13 production plus original new regressions | Compiled race RED: 5 PASS / 6 FAIL / 0 SKIP. CN and Discovery controls pass; manual Start, missing Trial hint/cache and beside-inflight behavior fail. |
| First port fixture regression | 50 PASS / 11 FAIL. Older synthetic Start handlers omitted the newly required balance GET. Fixtures now answer that exact GET explicitly and retain strict inference-path/counter assertions. |
| First frozen green | Full module race 432 PASS, package/UI, focused race and six module negatives pass; retained separately when the Discovery-Trial entry gap was found. |
| Discovery-Trial RED before its production fix | Compiled race RED: 12 PASS / 3 FAIL / 0 SKIP, including both Start and inactive-paid Discovery cases. |
| Final full module race | **435 PASS / 0 FAIL / 0 SKIP**, `go test -json -race -count=1 -timeout=4m -run . ./...` in this module only |
| Final bounded donor regressions | **200 PASS / 0 FAIL / 0 SKIP**, `-race -count=10 -run ^TestDonor0166 .` |
| Exact donor own/gift/inflight oracles | **78 PASS / 0 FAIL**, Bun against the archived 0.1.66 source |
| Donor causal controls | Five expected assertion failures: old CN profile order, requeue disabled, hint removed, gift exclusion removed, failed-plan paid routing restored |
| Final module causal controls | Seven compiled expected assertion failures: manual Start bypass, no requeue, no hint, no hint cache, no front order, expired budget admitted, Trial behind paid/catalog admission |
| Static/type/build | gofmt clean, `go vet ./...`, Python syntax, strict TS and browser bundles exit0 |
| Full/headless native ABI | Both exit0 |
| UI | Node **31/31**, Bun **31/31** |
| Actual installed-host proof | Canonical unmodified ZIP add/list/inspect/native describe/off; endpoint-only fixture overlay serves **11 ordinary executions / 11 scoped diagnostics** |
| Native owners | Five exit0: owned-agent lifecycle/rollback, CN auth, controller controls, MCP/history, UI auth |
| Source provenance | Exact desktop/CLI versions, earlier community lineage, new nine donor hashes, 19 SDK checked files and complete immutable SDK inventory verified |

Installed-host assertions retain failed-terminal last usage, final
measured zero, source model/request IDs, unknown execution currency/
credits/cost, rich quota exact zero and ordinary defaults3/2. Diagnostics
remain scoped code-only and cannot alter execution. No collection,
consent, retention or raw credential/error logging was expanded.

The real two-session loopback fixture holds the first inference beyond
the old one/two-second retries. Old code returns `beside:429`; current
code returns both200 with exactly two unused caps and four inference
POSTs. Its first-dispatch rendezvous makes the ordering explicit.

## Reproduction and retained evidence

Remote baseline/oracles/intermediate failures:
`usbox:/tmp/magpie-api9-zcode-OkPR9GHA/donor0166-3c0f487`.
Accepted frozen module, SDK links, package, receipts and logs:
`usbox:/tmp/magpie-api9-zcode-OkPR9GHA/donor0166-final2`.
Local copies and source archives:
`dist/donor0166-3c0f487/`, including `before/`, `accepted/`,
`module-final2-input.json` and `module-final2-input.tar.gz`.

`testdata/donor0166/verify.py` runs RED, covered/focused/module race,
donor oracles, donor/module causal controls, static and provenance stages.
`testdata/observability/verify.py` retains the ordinary package,
installed-host and native-owner paths; only its exact version assertion
consumer was advanced to 0.13.1.
`testdata/donor0166/collect.py` independently rechecks frozen source,
log hashes and Go counts, namespace identity, SDK files, both ZIPs,
native library, UI counts and installed receipts. The collector and this
narrative are post-acceptance evidence files; all 189 frozen inputs still
match locally and remotely.

Original RED archive SHA256:
`d9b9b72d42e02e878b4cee5fdac73df20d0896d5ad749bb983b1000593c28d07`.
Exact 0.1.66 donor archive SHA256:
`a1d059cd0f524b74617a8f97ae3134471782ab98725fcfc653b46ac6d78ef154`.
Original RED log SHA256:
`4782f6d04dbaed6664d7411b57057195b4a4cbd8399406529c2b78ca76348172`.
Discovery-Trial RED log SHA256:
`a88e57845299bc4fde03e4c96bd515a381a17f6a66e18cb93872f2481ed40d43`.
Final module race log SHA256:
`7de75394e5c91dfb5286898d4c81a09dfac452b2009ea44ff0a1c8afad2f0530`.

Previous 0.12/0.13 packages and every intermediate source/log remain
preserved. `AUTH-WEB-EVIDENCE.md` is unchanged at SHA256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
The two historical unexpected `/` billing requests remain unattributed;
this slice does not label their cause fixed.

## Remaining gates and deliberate differences

- Actual ZCode client/vendor, native macOS/Windows and rendered-browser
  acceptance were not run. The owned-agent executable is synthetic.
- Automatic cached-app-profile import/following remains absent. CN
  coverage proves selected-user ownership, not that donor integration.
- The Trial hint reads the existing marketing feed; it does not merge
  arbitrary legacy feeds, claim captcha-gated plans, schedule check-in
  or bridge a native client account-operation wire.
- Adaptive caps, queue state, hints and route holds are per module
  instance. Persistent cross-process admission and hold/card UI adapters
  are separate from this bounded port.
- Automatic DB/WAL capture, arbitrary log collection, financial journal
  import and actual client/session/financial resume remain unimplemented.
  Existing manual SQLite/history conversion is retained.
- Execution monetary/credit/deferred/upstream facts without pinned
  source authority remain unknown. No financial cost was invented.
- Final committed shared-host compatibility and AccountStamp closure
  belong to the parent acceptance. SDK-only deployment remains independent.

No known gift-only excluded/unknown-balance or failed-plan billing
exposure identified by this comparison is deferred as future work.
