# Bounded donor-parity checkpoint

2026-11-14. Continuation from accepted
`ffa65a5bb2a6bd492b04deae08650347c4cb5181`, inside this module only.
No host/core edits, branches/worktrees, dependency additions, installed
client, live vendors/accounts or push. The SDK-alone rollout is independent.
This is a domain checkpoint before the requested full-package default.

## Immutable identities

| Identity | Value |
| --- | --- |
| Vendor source | `fec975905a2abd4c07e35c40d51fa097a2258ec9`, desktop **3.18.2**, CLI **0.17.1** |
| Separate community donor | `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, package **0.1.61** |
| Built/tested SDK host | `d07078c7926daf6f33af09c40297c941bd39da92` |
| Host committed tree | `ed28dfd65eca68d15e9400b56660bbbf5e7bc319` |
| Reused frozen host binary SHA-256 | `5d1f307c9164bc4954c7806d3658255cb497da71f3cf75c1afb63c6a3064b8b3` |
| Domain source files | **138**, excluding git/dist/cache and evidence documents |
| Plain inventory SHA-256 | `50065d24b2a1d4ba6afdcbf91badd9b46a546381d92c9a9c3c37b3e79ef52401` |
| Host-tagged inventory SHA-256 | `8ecc08b399cb2d40f2bdc0444d5f120d5f2cded3b242cbd0124e78c74e514edc` |
| Headless compatibility artifact | **0.7.0**, SDK runtime floor **1.4**, native ABI **1** |
| Native/UI/owned-agent artifact | **0.10.0**, SDK runtime floor **1.5**, native ABI **1** |

The later shared-host freeze is tested separately by the leader. No
acceptance on `f536d820...` is inferred here. The source-lock pins the
actual tested host, rather than substituting a current branch name.

The donor files and both vendor package versions were read locally and
compared byte for byte to `git show` at the exact commits. Their hashes
are in `source-lock.json`. Start billing, the marketing action schema,
subscription behavior and all five supported locales come from the
3.18.2 source. Community claim acknowledgment and Start risk behavior
remain a separate lineage based on its 3.17.2 wire client.

## Implemented boundary

- Gift discovery keeps spent aliases and grant-only names, with free
  metadata. Discovery-enabled paid accounts with a valid selected ZCode
  session gain strict `-Trial` routes and separate paid/gift usage.
- Trial never falls back to paid. Plain free-first defaults on and is
  configurable. A single paid fallback requires a completely read
  1005/1113 refusal without output markers, no unsettled claim/reset
  debt and a fresh authoritative paid quota read proving capacity.
  Unknown, partial, streamed and transport failures do not authorize it.
  Confirmed spent models rest for one minute within the instance.
- Claims distinguish bare code 0 acknowledgment from a returned exact
  plan, 1003 done, 1004 ineligible, 3002 rate hold and 3007 captcha.
  Malformed/foreign plan data and lost replies remain durably unknown.
  Existing baseline reconciliation and usage barriers remain.
- Explicit `billing.offers` / `billing.checkin` use only the marketing
  feed and exact `claim_zcode_plan` IDs. They require private dataDir
  and a verified selected principal, retain durable leases/claim fences
  and share principal rate holds with manual claims across restarts.
  Explicit Retry-After wins, up to six hours; absent headers use bounded
  backoff. Cached checks suppress feed/claim, while identity validation
  still runs. No preview, arbitrary URL/action or captcha substitute.
- Start uses per-account FIFO cap three, cancellable two-minute queue
  waits, one-/two-second retries only for unused HTTP 429
  3008/3009/3010 refusals, five-minute adaptive narrowing and
  three-minute origin/proxy holds after sustained lone caps or firewall
  pages. State is bounded and per module instance. Paid requests are
  outside this guard.

Changed runtime files are `gift.go`, `start_guard.go`, `checkin.go`,
`module.go`, `instance.go`, `discovery_models.go`, `discovery_start.go`,
`usage.go`, `billing.go`, `control_client.go`, `controller/store.go` and
`protocol/response.go`. Version/provenance and comparison documents were
updated with the fixtures. Paid quota parsing and existing claim fencing
are reused; there is one feed path and no second auth/runtime owner.

## RED before each port

The gift, claim and Start fixtures compiled against the accepted old
implementation before the corresponding semantic port. Additional
counterexamples captured defects found during the continuation.

| Preserved run | Compiled assertion failures | Meaning |
| --- | ---: | --- |
| `gift-old-red-corrected` | 17 | Missing grant-only/dual aliases, free-first and isolated gift quota |
| `claim-old-red` | 10 | Missing acknowledgment/refusal outcomes and explicit check-in |
| `start-guard-old-red` | 6 | Peak nine instead of at most three, missing known-busy retries and shared hold |
| `gift-rest-old-red` | 1 | A definitively spent model reached gift inference repeatedly |
| `claim-hold-old-red` | 3 | Short Retry-After was extended and offers bypassed the shared hold |
| `ambiguity-old-red` | 5 | Mixed output/refusal could replay; malformed claim data could be acknowledged |
| `fallback-debt-old-red` | 3 | Fresh quota alone bypassed unsettled financial debt |

The initial `gift-old-red` used an incomplete paid-subscription fixture.
It is preserved as a fixture defect, **not** semantic RED evidence.
The corrected fixture supplies valid in-period paid entitlement and was
run on an ordinary archived source snapshot, not a branch/worktree.

The first extra native header mutation started before its package
dependency existed. Its FileNotFoundError is retained as
`agent-mutation-header-guard-package-missing.log`; it is **not** negative
behavior evidence. Repeating the unchanged assertion after packaging
reached the intended forged-header behavior failure.

## Frozen verification

All builds/tests used `ssh usbox`, Go **1.26.8**, existing offline caches,
fresh HOME/XDG, GOPROXY/GOSUMDB off and timeouts. User/network namespaces
preserve UID/EUID **1000**, expose **lo only** and drop all five capability
masks to zero. No native root ownership guard was weakened for userns.

| Gate | Result |
| --- | --- |
| Full module race | **317 passed**, zero failed |
| Gift/claim/check-in/Start race, count 5 | **275 passed**, zero failed |
| Owned-agent/control/provenance race, count 20 | **500 passed**, zero failed |
| gofmt, module vet, Python syntax | Passed |
| Headless and native/UI package ABI | Passed, two simultaneous ABI sessions |
| Strict TypeScript / browser bundling | Passed with existing tools |
| Node / Bun UI/controller tests | **29 / 29 passed** |
| Native owned-agent lifecycle | Passed on the frozen host |
| Native China/auth/models/Start | Passed on the frozen host |
| Native production UI bridge/auth | Passed on the frozen host |
| Darwin arm64 / Windows amd64 | Vet and compile only, no native runtime acceptance |

Behavior-negative overlays independently disabled strict Trial
protection, unknown-gift no-replay, financial-debt fencing, unknown-claim
no-replay, bare claim acknowledgments, Start cap and known-code retry
boundaries. **All seven** reached expected compiled assertion failures.
Three owned-agent file/binding mutations also failed their assertions.
The native header-guard-disabled module additionally failed the real
PassHeaders forged-ticket/card assertion. The default-filtered header
case alone is not used to claim this guard was tested.

| Frozen log | SHA-256 |
| --- | --- |
| Full module race | `8f56183a8b42f9b58f44ff8779325742703cfa90863761b21f8070a2c4cc8c1b` |
| Scoped race, count 5 | `66a53e3694c93ca05ce669b83d6273c517210ad8687cb5ff4cdc934c99d9605e` |
| Agent/control/provenance race, count 20 | `7c2ee1872af8b2862235bda69b3207e8cc54330019215c759755aedf0a88d028` |
| Native header mutation | `eebf53604176cea8cc09450ad45f7a8c71d12295d4989dfa66d242b0e9373e1c` |

`dist/parity-domain-0.10.0/evidence/` retains local reports, inventories,
RED logs, green logs and behavior-negative logs. It is an ignored
verification artifact directory, not a runtime dependency.

| Domain checkpoint artifact | SHA-256 |
| --- | --- |
| `headless-compatibility.zip` | `fd5489cd48e82f1b8d277d05e955d2b8fae6ff8712cc570d89e9da621b8531c5` |
| `native-ui-agent.zip` | `4e5bdff7c3ef4e4ba3702fcbb75b4f88c49f0ba2ebae02c1ab329dce5f23b566` |

These are immutable checkpoint artifacts. A subsequent normal full
package may have a new ZIP identity even while retaining module version
0.10.0; its packaging provenance must be recorded separately.

The historical untracked `AUTH-WEB-EVIDENCE.md` stays outside the commit
and package, byte for byte:
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.

## Remaining evidence and behavior gaps

`COMMUNITY-PARITY.md` remains the detailed matrix. Gift response alias
rewriting, automatic plain-model bypass of a locally held gift route,
future-plan/card grouping, offer hints in usage, cap display hints and
the wider ToolSearch/turn-attribution comparison are incomplete. Manual
non-discovery paid imports retain their earlier gift discovery surface.
Safety restrictions on fallback are intentional, not donor parity claims.

Check-in has no scheduler, native-account-operation integration or page
control. These are module tasks; SDK 1.6 native operations already exist
and are not a daily scheduling contract.

Actual ZCode executable startup is still absent: lifecycle uses an
explicit synthetic executable. No live vendor/account, rendered browser,
Darwin/Windows runtime or custom data-root/Library acceptance is claimed.
Automatic DB/WAL capture, financial import and actual-client resume
remain unimplemented and separate.

The two historical unexpected `/` requests in host-visible billing
fixtures remain **unattributed**. Passing isolated runs did not establish
their cause or fix. This checkpoint does not declare the whole plugin ready.
