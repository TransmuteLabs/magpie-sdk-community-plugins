# Paid cache task evidence

The current standard package is0.8.0, API1.9, ABI1. It includes the complete
agent/paired provider and one default-off paid task, `cache-keepalive`.
`keepAlive:true` is the sole opt-in; timing options do not authorize spending.
Historical paired0.5/API1.7 and inbound0.2/API1.1 remain explicit alternatives.

## Current committed API9 replay and strict sampler

Accepted host isd5b098d8/tree11c2; execution uses the parent's binaryc018
built from tree8fe. Only two test files differ; production is identical.
`DELIVERY-EVIDENCE.md` records full source, binary, library and receipt
hashes. The standard0.8/API9 library017ebccc is unchanged.
Current evidence:
`usbox:/tmp/magpie-devin-final-fixture-20261114.3miL8Zfd`.

The maintained hot fixture isc2c58b5a; its9 regressions are4405c0c4.
It uses the existing cold scanner for raw wire0/1/2/5, without changing
the legacy decoder. Native temperature/top_p are fixed64/wire1 according
to recovered source and the actual encoder. A separately named negative
proves the old decoder rejects a legitimate structural fixed32 chunk;
no captured native fixed32 field is claimed. Running the exact original
hot75cb parser on a valid native fixed64 template is RED at line117.
The9 tests on old cold511 are7PASS/2FAIL; the strict fixture is9PASS.
Exact zero sampling, stop/seed/defaults, named `Read` choice and disabled
parallel calls are required on foreground and paid requests. Only output
max changes to1; sampling chunk order/bytes and tool bytes are preserved.

Three complete installed hot-config-CAS runs are GREEN in44.680,46.240
and47.552s, each without restart or catalog warmup. Real30s cadence and
the held38 wait are unchanged. Each observes zero default-off intents,
fresh budget/revoked-model/readOnly refusals, both paid arrivals,
cancellation/join, completed/not-started/not-started/uncertain terminal
intents and exactly two once-only measured2.5ACU rows. Those rows preserve
the original caller, distinct operations and counts`[7,0,17,5]`.
No external requests, extra plugin, runtime overlay or policy workaround
is used. Identity/no-op/held-read, real transitions, genuine Gemini4 and
paired24 also pass on the same physical binary/library.

The subsequent frozen merged candidate treef7f26b2d/binary9ddc4e4a
passes one additional complete hot replay in43.414s with the same library
and fixture. Its public SDK and paid-admission production are unchanged.
All10773 source paths remain unchanged and the same eight assertions,
two rows, cancellation and zero external requests pass. The receipt
identifies that exact tree, not a future commit; it does not move any
of the three c018 results to the new binary.

The original c018 held38 timeout is still unexplained/unreproduced;
the new passes do not establish its cause or repair it. Godel's six
independent diagnostics reached both paid arrivals/cancel/two rows, then
failed the old parser; all original REDs, including467 and052, remain
separate. `DELIVERY-EVIDENCE.md` gives their paths and causal log hashes.
This is bounded synthetic-peer acceptance, not original-client, live
cache/charge or complete native-resume readiness.

## Historical committed467 API9 replay

Host46756a48/binary32fbdeab and full library017ebccc have fresh installed
evidence in `DELIVERY-EVIDENCE.md`. The unchanged cold-start paid fixture
passes real30s cadence, spent-budget/model-revocation/readOnly controls,
one completed ping, one held canceled ping and two once-only measured
ACU rows. Default-off still creates zero intents. Ordinary duplicate/
no-op/held-read, real transitions and genuine Gemini four-case gates pass.

The original hot-CAS fixture75cb45a5 remains an actual separate RED:
all five foregrounds return200, but no paid ping arrives within38s and
no paid usage is measured. Four not-started receipts were collected after
cleanup; these do not establish the exact admission/cancellation cause.
This differs from the earlier stale-catalog foreground failure, now
corrected on467. No module retry, paid guard workaround or new host code
was added. The later current-catalog correction and strict hot fixture
proof are recorded above without relabeling this old run.
The older052 paid-not-started result remains failed.

## Historical pre-compatibility full-package replay

The final full0.8/API9 library, including the Gemini0.1.41 port and strict
origin guard, is
`464489d54f5982f310821ebefb26704deb65f24ca20fe74161f75f28be1d6910`.
Its immutable module input inventory is
`dac520a0eb59fc7774cd6a540baea22ecc81c93c096a868f52340e42c453a88c`.
Final own Go/full/race/vet/build, nine provenance cases and the unchanged
donor7/7 tests all pass. Their newer frozen public SDK input is999f9b2f;
exact hashes are in `GEMINI-SIGNATURE-EVIDENCE.md`.

Actual installed replay:
`usbox:/tmp/magpie-devin-release-install.cyABtIA0`.
It reuses the complete hostca3/public148/binary6d4 pinned below, separately
from the public SDK export used for module-only gates. The default build
and all six selected installed fixtures exit0:

| Installed gate | Log SHA256 |
| --- | --- |
| Default-off/paid/canceled usage | `064767834532523a040506fa22906109059bd1c1f61ed09e2ad93980993dcec0` |
| Paired24 catalogs/policy/cancellation | `cd85cd2d11a48f2a4ed807045f9def1dc8e1fb16d023154f50c6e726cde75599` |
| Genuine Gemini Chat JSON/SSE | `687e4c7b4a59bf45c70ed7255893003f2c62e273482d76f62b655246d79f06da` |
| Claude Messages/signature/history/cancel | `e08b6485f70f94a2267d3313a6a53e094dcc4c138215cd22cb90f6d5bf54d508` |
| Ordinary synthetic agent/bootstrap | `0d67cf7b3899073cc30f23a1c6efb80991e3d56b5983c22a472cbe229b03919f` |
| Real model transitions | `e02a22926f7f386a6c4c91122de626fd4796d10e03d560d955d779b4693f52a0` |

The paid causal assertions below repeat unchanged with this final library:
real30-second cadence, zero intents by default, two once-only measured
ledger rows, original caller/operation, held cancellation/join and zero
external requests. Sources remain unchanged and isolated homes/processes
are removed/joined. The known no-op, hot-config and Gemini Messages REDs
are preserved independently; they are not turned into successful gates.
This is scoped acceptance on a dirty frozen host, not the parent's final
committed upstream+SDK9 pin or blanket bootstrap readiness.

## Earlier keepAlive SDK-peer acceptance

KeepAlive-only checkpoint after the priorModels correction:
`usbox:/tmp/magpie-devin-agent-prior-green.kX58rZet`.
Local input: `dist/2026-11-14-agent-prior-green`; compact results are preserved
in `dist/2026-11-14-final-compact-evidence.tar.gz`.
The later Gemini port and combined module gates have their separate exact
inventory in `GEMINI-SIGNATURE-EVIDENCE.md`.

| Input | SHA256 |
| --- | --- |
| Complete module inventory for these Go checks | `92bc3c357cb124ea95160d2db4db8cab284e4cdd8779bd40663fc319b0eb4dc0` |
| Module archive | `dc332a5682a9a3fe97698dcb73395b5b1a11100ecae171ec2fed00edcd4ed391` |
| Dirty public SDK9 export | `dc4d6772b33459b59e4058bbdc8b3586547efe7c1ff896f173e1f58b9bc64e9b` |

The module base is7f8fde6; the SDK export base is
`ae957e5a8a77553a6cf6f484c8b58a69393f3887`. Both captures have
sourceCommit:null. The public export is not a whole runtime. Subsequent
documentation, packaging and installed-fixture reconciliation does not
change the tested production Go.

The ordinary model-transition regression was initially RED on the same
public SDK input: host planning included `options.priorModels`, and the
module rejected the unknown field. The module now accepts that catalog as
opaque planning input; selected model and host Binding remain the authority.
`TestAgentModelTransitionAcceptsAPI9PriorCatalogWithoutGrantingAuthority`
passes without relaxing unknown-option rejection. The RED is preserved at
`usbox:/tmp/magpie-devin-agent-prior-red.4tK9vZr3` with log SHA256
`cadfc3f03fddd0c26ae799b9d73b33fa8dd435b6bd1e2c7ebfa269a6e3427ebf`.

All seven gates exit0: gofmt-l is empty, TestCache SDK-peer tests,
TestCache root+codec race/count3, complete module tests, go vet, go build,
and eight Python provenance/distribution tests. Execution used usbox,
Go1.26.8, pinned offline caches, bounded commands and isolated HOME/XDG/TMPDIR.
Every before/after source inventory matched, and every sandbox home was
removed. Module tests include byte preservation, fresh proxy resolution,
default-off, duplicate/max, wrong account/generation/model/key/job/due,
independent conversations, two completed misses, refusal/malformed stop,
held cancellation/join and partial measured ACU facts.

The first API9 run preserved a real module RED:
`TestCacheTaskRefusesChangedAuthorityAndUnplannedInputsBeforeAcceptance/expired`
observed an accountTask.usage event before acceptance. The fix returns
not-started without usage when no accepted dispatch occurred. Both the
ordinary assertion and its failing log remain in
`dist/2026-11-14-cache-adapter` /
`usbox:/tmp/magpie-devin-cache-api9.lzIUYgqu`.
The second run's missing x/sys go.sum entry was build setup, not behavioral
evidence; the third run records the SDK's already-existing v0.49.0 pin and
passes all gates offline.

## Complete host identity used for installed replays

The read-only complete capture is
`dist/2026-11-14-cache-host-input`. It separates the parent's base ae957
from all dirty parent/Volta API9 files: sourceCommit:null.

| Input | SHA256 |
| --- | --- |
| Complete source inventory | `ca3c6c8ad9be659d5d507f91c39c198b69a5a5180fc8db86647943b7a4c2fda6` |
| Complete source archive | `5f6c8d17cad52fbf974091b716fd8731a885bb716afb4f9cac908ff95bad80ce` |
| Actual built host binary | `6d4ee4edd42f5e03f00b43190f8a7ed5592d6d550a0b56a2a96573240eb4531c` |
| PriorModels-corrected0.8 library, before the Gemini port | `231584de44c9e047053cdd84b7d00a9aa177b0ac02341f0d78d13c7c49816d8e` |

Build root: `usbox:/tmp/magpie-devin-cache-installed.1XbPmdbS`.
The actual gateway maps this library and reports exactly one installed
module. Ordinary default-off completion passes with zero task intents.
Paid installed acceptance and its exact causal observations are reconciled
below independently of the successful SDK-peer gates.

## PriorModels-corrected installed paid and transition acceptance

The corrected library above was built and loaded in
`usbox:/tmp/magpie-devin-cache-installed-r8.qH9cX2Mk`. The host maps exactly
one own full0.8/API9 library; no Bun or companion plugin is installed.

| Unchanged installed fixture | Result / log SHA256 |
| --- | --- |
| `cache_keepalive_host_e2e.py` | Exit0; `52d08a2151f6a66bfe78a069e3c1eec20533cd0f6e99300d0389e86acc1a195f` |
| `paired_http_e2e.py` | Exit0,24 checks; `92bc5942eba52ed1acc96f886e5d562b3969950b76e2291be3ac30c2c6bd327d` |
| `assignment_transition_e2e.py` | Exit0 in `usbox:/tmp/magpie-devin-cache-transition-r9.kT84eSDh`; `e02a22926f7f386a6c4c91122de626fd4796d10e03d560d955d779b4693f52a0` |
| `assignment_identity_e2e.py --expect-stable-sync` | Actual exit1; `f4eaeb67ff2ffe0267c36376f30cb6f220fea378bda1f903f6ae78079483992e` |

Paid acceptance starts with default-off: ordinary completion creates zero
task intents. Enabling the saved option followed by an ordinary cold
startup exercises a real30-second cadence. Four intents terminate
completed/not-started/not-started/uncertain. Spent budget, revoked model and
readOnly403 refuse before paid dispatch. Exactly the completed-prefix and
held-prefix pings reach the synthetic vendor; a new real turn cancels and
joins the held request. Two ledger rows retain the original caller and
distinct operation identities, each with measured `[in,out,read,write]`
`[7,0,17,5]` and2.5ACU. Canceled usage appears once without inferred USD or
own_cost. Raw prompt/cache/tool/sampling bytes match the foreground request;
only one unmarked continue prompt and max_tokens1 are added. Ordinary
module.call cannot replay the task. Eight total synthetic vendor arrivals,
zero external requests and joined processes/removed homes are recorded.

Real model transitions still fence an in-flight read and accepted write.
The stable duplicate is403 with one dispatch, stale-model requests do not
dispatch, the accepted write keeps its original journal/scope, and two
actual model assignments are recorded. That fixture's historical
`bootstrapAcceptance:"red"` is a broad readiness marker, separate from its
successful transition gate.

## Historical generic host gaps before committed467

The following three REDs describe the earlier captured API9 runtime.
Current no-op, hot-config foreground and Gemini controls pass on the
committed host, with paid hot acceptance and its remaining timing limit
recorded above.

The identity RED above is not a module RequestID/Session change. First
AssignModel200 dispatches once, its exact duplicate before sync403 dispatches
zero more, but an ordinary model-name edit changes only
`binding.modelOptions[0].label` and `binding.revision`1→2. The exact duplicate
then returns200 and actually dispatches a second AssignModel. Frozen
`internal/agent/extension_controller_sync.go` compares ModelOptions bytes,
so cosmetic catalog labels defeat its no-op path. The fixture stops before
the held-no-op read; current API9 held-no-op acceptance is unproven.
Keep binding revision, journal and Check fences. Parent needs a generic
authority-preserving distinction between cosmetic catalog display and real
model/capability/effort/output transitions, with the existing checked plan
and file/shell rechecks retained.

The earlier r7 identity GREEN used library8146, which rejected priorModels
and therefore masked the real sync transition. It is superseded, not
current acceptance. Older0.7 no-op proofs remain historical.

Separately, checked external module.config CAS from off to on leaves a
held model selection stale: the next ordinary request502 reports
`extension model catalog selection changed`, with no module or paid
dispatch. Actual evidence is
`usbox:/tmp/magpie-devin-cache-installed-r4.cGTF5MIO/installed-paid`.
Catalog identity includes config.Version while AccountStamp observes
account files rather than cross-process module.config. Parent needs generic
catalog/binding invalidation on the config revision; do not weaken the old
selection fence. Cold saved configuration passes.

The third actual host RED affects Gemini canonical Messages: this frozen
host adds `"thinking":{"type":"disabled"}` before strict SDK execution,
although the caller omitted it. Actual installed status is400 with zero
native dispatch. The exact original RED and code-only delegating module
probe are in `GEMINI-SIGNATURE-EVIDENCE.md`. Parent's generic seam is SDK
Anthropic caller-body preservation before legacy shaping, with non-SDK
controls retained. No shared host fix or guard weakening was made in this
module scope; final combined-host replay belongs to the parent.

## Historical prepared domain code

At this earlier checkpoint the standard package was0.7.0 with minimum
SDK1.7, declared no account task and started no paid background work. It added
independently tested template, option, conversation and storage primitives
for a later API9 adapter. It is not installed-host paid-task acceptance.

## Exact inputs

Own immutable usbox fixture:
`/tmp/magpie-devin-cache.kIfWRbeX/policy-r4`.
Local receipt mirror: `dist/2026-11-14-cache-policy-r4`.

| Input | Identity |
| --- | --- |
| Module base | `7f8fde6cc4de4bb5b3ebc4f4c9a3570312acc077` plus the completely inventoried preparation files |
| Module source inventory SHA256 | `04f24fffe5f85467fe48e0c29dfbd2dd7ac4aa811ad66f7c9dfac5ae887e3d25` |
| Source archive SHA256 | `27bee4dcaad46d6a3bfa6a1479f2ceb65d9d5af5761aac1639c2f77bccba992f` |
| Public SDK export | `52a8d85d7956fd027d76b0baea96ea5cf002c34a`: go.mod/go.sum/pkg/extension/pkg/protocolext only |
| SDK compressed archive SHA256 | `c0ede4e5d0de1c90d3ba099b3f43043b2a3d77d578778360c6abf4c18782960e` |
| Runner SHA256 | `62e7861ef3a61a2323704a1a1948abdf0eaee0fb5948f5337ecb7cf6e549b045` |

The export is SDK1.8 source for module tests, not a complete host/runtime
snapshot. Source/archive/runner identities are separate from a git commit.
The runner checks complete module and export byte/mode inventories before
and after every gate. Later reconciliation of this evidence and the source
guide does not change any tested Go source or manifest.

`source-audit.json` independently compares six reviewed community and native
descriptor files with their locked git blobs and SHA256s. All working bytes
match those immutable source versions. Source boundaries and extraction
decisions are in `CACHE-KEEPALIVE-PLAN.md`.

## Actual gates

All execution ran through ssh usbox with the existing Go1.26.8 toolchain,
pinned caches, `GOPROXY=off`, `GOSUMDB=off`, `GOWORK=off` and
`-mod=readonly`. Each command has its own temporary HOME/XDG/TMPDIR, a
bounded timeout and joined owned processes. The receipt confirms the homes
were removed and source inventories remained equal.

| Gate | Actual result |
| --- | --- |
| gofmt -l, six new Go files | Exit0, empty output |
| go test -count=1 -timeout120s -v ./... | Exit0 |
| go test -race -count=3 -timeout120s -run TestCache . ./codec | Exit0 |
| go vet ./... | Exit0 |
| go build ./... | Exit0 |
| python3 -B -m unittest -v provenance_test | Exit0 |
| Null-consent mutation, checked-configuration regression | Expected exit1 with the named test FAIL |
| Credential-retention mutation, content-secret regression | Expected exit1 with the named test FAIL |

These last two gates are intentional mutants of this new implementation,
not REDs observed in an earlier shipped plugin. They run in separately
inventoried derived fixtures while the green module and SDK remain unchanged.
Removing the explicit null refusal accepts `{"cache_keepalive":null}`.
Removing the content-secret guard retains the original account key after
metadata removal. Both produce ordinary assertion failures, not compile
errors, skips or timeouts.

The preserved `policy-r2` run had a format-only RED in two tests and green
policy/race3/vet. `policy-r3` added typed/null consent and content-secret
checks and passed all eight gates. `policy-r4` additionally refuses differently
cased option names; it is the final tested Go input for this checkpoint.

## Protected behavior

The template removes account metadata, preserves raw interleaved original
chunks including prompt IDs, cache marks, signatures, tools and unchanged
sampling, appends one unmarked USER `continue`, and limits only max_tokens
to1. Reusing a template adds no further history. Every payload reinserts
current metadata and refuses credential bytes already present in content.
Unknown/duplicate fields and assignment-bearing controls fail explicitly.

Configuration defaults to off/285 seconds/6 pings. Only literal boolean
`cache_keepalive:true` is consent. Nulls, incorrect types, alternate spellings
and out-of-range numbers are refused. The policy uses actual send time,
two consecutive misses, explicit401/402/429 stops and the configured maximum.
Caller/session/subagent/instruction/tool grouping preserves independent
conversations; the no-session growing-prefix index stores hashes and lengths.

Memory holds at most32 conversations,32MiB aggregate templates and256 hashed
fallback identities. New foreground tickets invalidate earlier completions;
only one concurrent callback can claim a tick. Generation/model mismatches,
duplicate deliveries, stale held completions and closed stores cannot rearm.
There is no timer, retained Stream/Auth/client or persisted prompt template.

## Historical API9 draft watchpoints

The earlier read-only accountTasks draft was discussed directly with Volta.
These three concrete issues were subsequently resolved by the final public
DueAt/Key/snapshot DTOs consumed by the0.8 adapter:

1. `DelayMs` must represent an overdue immediate tick; the inspected draft
   minimum1000ms changes donor timing.
2. Foreground cancellation is currently per plugin/provider/account lane.
   Canceling all same-account jobs would merge independent caller/session/
   subagent conversations despite their separate opaque task keys.
3. The inspected task call passes a nil event consumer, ordinary module RPC
   rejects non-accepted events, and terminal OwnCall accounting runs only
   after a successful result. A canceled paid tick needs a task-only,
   host-owned measured-usage snapshot/finalization path that records once
   even when cancellation discards its terminal.

The current adapter consumes that contract; current evidence is recorded
separately from this historical preparation. No shared host source was
edited for this module task. Native original-client/live-server/cache-TTL
behavior remains untested, and the recovered reconstruction is still distinct
from the original Cognition client.
