# Devin on the consolidated SDK host

This file preserves the historical **d070** acceptance, including actual
reds. Its blockers are not the current no-op status. The original bundle's
identity/held-read and real-transition gates now pass on committed
`9e88f1098` and a separately captured API7 host. See `NOOP-ACCEPTANCE.md`
for exact inputs; current package/domain results belong in
`DELIVERY-EVIDENCE.md`. Broad host acceptance remains leader-owned.

Date: 2026-11-14. **Historical d070 acceptance is partial, not green.** Installed
paired/native catalog checks, cold CLI selection, inbound conformance and
reconstructed-client startup pass. Duplicate native assignment dispatch
and unstable bootstrap block agent acceptance.

## Frozen inputs and ownership

The host was built from a fresh git archive of
`d07078c7926daf6f33af09c40297c941bd39da92`, tree
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`, exported read-only from
`../magpie`. Source6, native account operations and persistent modelcache
already share this runtime. The earlier devzeus consolidation
`b5e4494f6ae7b69b0333925e21e9eaf709a7acf9`, tree
`1880ea418af8c65318afbcc4c7e396aa9cb5dd12`, differs only in AGENTS.md.
The leader's subsequent SDK-alone freeze is git tree
`f536d8202e40aae4b0fc36630aa3c1eb496d58d5`; its complete diff from d070
contains documentation/tests only. The tested executable is attributed to
d070, not to an unbuilt b5/f536 artifact.
Other sessions' dirty host tests were not exported or changed.

Module Go runtime remains committed
`7a621943095fb7f4a1e05857ea380a6788fb161a`. This follow-up changes
fixtures, documentation and provenance, preserving the six prior WIP files.
No host edits, branches, worktrees, built-in experimental loader, vendor
imports into core, dependencies or pushes were added.

Source authority remains:

- Recovered app/wire: `97a4883009bc988fedcdf0416a50a6c62f8d9c9b`.
- Community Devin: `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`,
  packages/devin 0.1.40, read-only domain comparison.
- Maintained CLIProxy: `842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9`,
  v8.0.42-698-g842cedec8, committed source only.

`SOURCES.md` and `source-lock.json` pin exact artifacts, descriptor hashes,
variant floors and update watchpoints. The afdb5d8 checkpoint's 63-file executable input
inventory SHA256 is
`440d6bbf53841cd91ad11ec2b5a1dba167ac8d208da85c86659f82174fb8f597`.
It hashes the sorted compact path-to-SHA256 JSON map of Go/Python,
go.mod and manifests; prose and source lock are excluded. That checkpoint
replaced the earlier inventory because three Python fixtures changed.
The later identity diagnosis adds one Go regression and one Python
diagnostic; its raw receipts and unchanged runtime identities are recorded
in `ASSIGNMENT-DIAGNOSIS.md`. The subsequent real-model negative-control
fixture and donor-domain evidence add no production Go changes or runtime
rebuild; their receipt/input hashes are recorded separately.

## Environment and commands

All builds/tests ran through ssh usbox, Go 1.26.8 Linux/amd64, existing
pinned caches, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off` and
`CGO_ENABLED=1`. Each invocation uses temporary HOME/XDG and bounded
subprocess timeouts. User removed the earlier CPU cap; no new
GOMAXPROCS/-p restriction was applied. No adm, live app, vendor server,
credential import or dependency download was used.

The usbox evidence root is `/tmp/magpie-devin-union.vlj9mmty`.
`gates.py` retains exact commands and separate per-gate exit codes:

```sh
go test -mod=readonly -json -count=1 -timeout=3m ./...
go test -mod=readonly -race -json -count=3 -timeout=5m ./...
go vet -mod=readonly ./...
go build -tags nogui -trimpath \
  -ldflags=-X\ main.version=0.0.0-devzeus-sdk16-sdk15-sdk14-d07078c79 \
  -o ../magpie-host .
python3 build.py ../paired-dist
python3 build.py ../agent-dist --agent
python3 build.py ../inbound-dist --inbound-only
python3 paired_http_e2e.py ../magpie-host ../paired-dist ../paired-http \
  --generation-mode live
python3 agent_http_e2e.py --magpie ../magpie-host \
  --distribution ../agent-dist --artifacts ../agent-http
python3 agent_startup_e2e.py --magpie ../magpie-host \
  --distribution ../agent-dist \
  --agent /tmp/magpie-nativeops.F9jg4B/recovered-target/debug/chisel-agent-bin \
  --network-guard ../network-guard.so --artifacts ../recovered-startup
python3 conformance.py ../inbound-dist --magpie ../magpie-host \
  --result ../inbound-abi.json
```

Host build runs in the exported `magpie/`; module commands run in `module/`.
The runner's top-level status reflects only its last selected invocation.
It says passed after startup/conformance, but individual `agent-http` and
`agent-repeats` gates have exit 1. They remain failures in this record.
No broad host/union suite was run by this module executor.

## Actual results

| Gate | Result |
| --- | --- |
| Complete module unit suite | Exit 0; 501 runs / 504 PASS events |
| Complete module race count3 | Exit 0; 1503 runs / 1506 PASS events |
| gofmt -d / go vet | Exit 0; no diagnostics |
| Python static compilation / provenance | Nine Python files compile; five provenance regressions pass |
| Ordinary host, three C-shared variants and loopback guard | Build exit 0 |
| Installed paired HTTP, live account replacement | Exit 0; 24 grouped checks / 24 inference requests |
| Cold CLI regression against historical 9c | Expected assertion failure: model is not in the fresh CLI catalog |
| Cold CLI selection on d070 | Pass in complete agent trials: fresh model/env processes with the gateway stopped and no added vendor calls/probes |
| First ordinary agent HTTP | Exit 1; duplicate assignment returned HTTP 200; dispatch count not recorded in this first run |
| Single agent diagnostic | Exit 0; seven groups / four inference requests; one assignment dispatch |
| Bounded three-trial diagnostic | Two pass; third fails a bootstrap-read assertion before AssignModel, with zero assignment dispatches |
| Dispatch-count observation | Exit 1; three bootstrap reads HTTP 200, duplicate assignment HTTP 200 and **two actual mock assignment dispatches** |
| Binding-snapshot observation | Exit 0; seven groups / four inference requests; one assignment dispatch, duplicate HTTP 403; same-model revision 1 → 2 during team-settings read |
| Controlled identity diagnosis | Diagnostic exit 0 / acceptance red; duplicate refused at revision 1, then actual second dispatch after a revision-only 1 → 2 no-op sync; installed decode and exact hash inputs otherwise match |
| Held team-settings reply across no-op sync | HTTP 403 after revision-only 2 → 3; raw diagnostic exit 1 because its first assertion incorrectly required an external retry header; corrected assertion checked against the saved receipt |
| Real-model negative control | Exit 0; actual CLI model/revision changes fence a held read and accepted-write reply with 403; a separate stale-model payload never dispatches; the accepted write completes under its original journal with exactly one dispatch |
| Unmodified reconstructed-client startup/first turn | Exit 0; native status/model preflight and one inference request |
| Inbound C ABI and ordinary SDK CLI | Exit 0; concurrent native sessions, all catalog variants, decode and explicit refusal checks |

Successful full HTTP/client fixtures report zero external requests and
removed fixture HOME/stopped host processes. Agent fixtures assert exactly
one installed SDK bundle, `devin-inbound`, and zero legacy/community
provider plugins or helper packages; Bun is unavailable. Deny-proxy and
the reconstructed client's loopback-only libc guard remain active.

### Installed native catalog policy and cancellation

The actual loaded paired library projects all six native catalog endpoints
through a key restricted to the selected model. Every response contains
only that key's alias `assistant`; GetModelStatuses remains an established
empty advisory response. Read-only keys receive HTTP 403 for every catalog,
status and inference route.

The discovered alias executes exactly once against the selected native
model. A second call hits its one-token scoped budget without another
inference request. A forbidden model is refused before dispatch. An expired
unary catalog deadline returns HTTP 504 / `deadline_exceeded`; later
discovery still works. Existing native inference timeout and TCP disconnect
tests cancel the actual mock upstream request.

The paired run also retains signed-origin/generation revocation, malformed
stop/unknown-control refusal, failed-completion receipt refusal and foreign
executor denial. No stop, cancellation or unknown-control guard was relaxed.

`http_e2e.py` now puts TMPDIR inside fixture HOME. The host's native-image
cache uses verified temporary library snapshots. This keeps those snapshots
isolated while preserving the assertion that the **actually mapped**
library SHA256 matches the distribution; the assertion was not weakened.

### Failed native write and generic host handoff

`agent-observed-run.json` records this failed acceptance observation:

```json
{"bootstrapReadStatuses":[200,200,200],"duplicateStatus":200,"assignmentDispatches":2}
```

Both HTTP requests carry the same native model/cascade request. They are
deliberate duplicate client requests, not a retry introduced by the fixture.
One accepted assignment should reach the mock. The second dispatch is a
confirmed no-replay defect. The original failure had no dispatch counter;
the later observation supplies that missing evidence.

The final instrumented fixture records bootstrap errors, binding snapshots
before/after reads and assignments, and dispatch count even on a failed
duplicate assertion. Its passing observation shows owned revision 1 → 2
with unchanged model during GetCliTeamSettings, then stable revision 2
and duplicate HTTP 403. Earlier three-trial evidence also records a
same-model revision change and one bootstrap failure whose exact response
status was not retained. These passes do not supersede the failed dispatch
observation. No binding snapshots were captured in the two-dispatch run,
so its exact interleaving is not established.

The subsequent controlled diagnosis in `ASSIGNMENT-DIAGNOSIS.md` supplies
the missing causal evidence without claiming that earlier unsampled
interleaving. Module RequestID/Session/body, caller/account/generation,
package/config and full semantic binding remain stable. A display-name
catalog update changes only revision. The actual host formula then gives
the identical write a new journal key, and a held native read fails its
post-call exact revision fence. The old completed journal is preserved.
The separate installed real-model negative control confirms the current
guards reject stale/in-flight work on genuine selection changes. Its held
write completes under the original accepted scope before the outward 403;
no replay or guard relaxation is introduced. Failed fixture assertions and
their raw receipts remain distinct from the final passing control.

Generic seams for the leader:

1. `internal/agent/sync.go` calls controlled-agent Sync.
   `internal/agent/extension_controller.go` prepares/applies it, publishes a
   pending record and increments revision even with unchanged model values.
2. `internal/agent/extension_account_operation.go` requires owned phase
   and exact binding revision throughout a finite native call.
3. `internal/gateway/protocol_account_operation.go` includes that revision
   in durable write identity.

Preserve semantic ownership across a proven no-op in
`extensionController.apply` before publishing pending/revision+1, retaining
checked zero-file ActionPlan execution and all authority/preconditions.
Keep package/config, account generation, caller/model and real ownership
revocation checks. A host regression should hold sync across read/write
admission and require one assignment dispatch, with explicit refusal after
genuine revocation. Do not remove revision checks, retry accepted writes
or add vendor policy to core. The shared host is read-only during the
leader's freeze.
`CONTRACT-BLOCKERS.md` is the current boundary reference.

## Artifact SHA256

| Artifact | SHA256 |
| --- | --- |
| Ordinary d070 host | `bc366fa3e321a7e7c0f5a6735a76c77c1488145b47892cf8551f08a8f444d024` |
| Agent 0.5.1 / SDK 1.6 | `22eefa631bb913a2e7ba1b74dd74045c62f78988b622edc6f75be4958bf69397` |
| Paired 0.3.1 / SDK 1.4 | `0ee6b873e64e49879df0b5c14fafa307b8277e3110e751ccb49939659b8445bb` |
| Inbound 0.2.0 / SDK 1.1 | `d689f178c11b3da789ceeb65600f1d6e2c40631ba04acfa7c80079d2e994e8e2` |
| Reconstructed application | `8b5438632942381272d20974f7ee59fd9af7674d452dce7ea448077b4c075327` |
| Loopback network guard | `c7012e29410d0585e675caef74ceef84216ac0e4e2e6210ea3d8ae863e98279b` |

Receipts are under the evidence root: per-gate logs,
`paired-http/result.json`, `agent-diagnostic{.json,/result.json}`,
`agent-repeats.json`, `agent-observed{.log,-run.json}`,
`agent-binding-observed{.log,-run.json}`, `recovered-startup/result.json`
and `inbound-abi.json`. `pre-observed-inputs.json` preserves the earlier
input hashes; `agent-http-prior.py` and `agent-http-observed.py` preserve
fixture versions used before binding snapshots were added.

The final metadata pass reran nine-file Python compilation, all five
provenance regressions and all three distribution builds. It matched 93
source inputs to the uploaded snapshot and confirmed 54 compiled inputs
unchanged from the previously tested union snapshot. All library hashes
above remained identical, and each packaged source lock exactly matched
its selected manifest and current guide. `final-metadata-result.json`
records metadata passed separately from acceptance partial; it retains
the failed agent gates and confirmed two-dispatch observation.
`final-handoff-result.json` records the final source match after the
documentation-only historical annotations. No unchanged Go/host suite
or runtime acceptance was repeated for those annotations.

## Proof limits and remaining work

The four finite bootstrap operations require a local owner and explicit
selected model/account. Restricted/remote caller delegation and ambiguous
group account selection are not supported. Literal `Basic key-key`
recognition at remote ingress and host-admitted post-reply scheduling
remain separate generic boundaries. SDK-only host rollout is independent
of the optional Devin agent acceptance.

Signatures require same caller/account/model/generation receipts; assignment
adds cascade scope. A failed completion cannot mint new receipts. Previously
saved receipts are not re-attested or purged. Native resume/history isolation
is proven with the synthetic recovered-wire client; the reconstructed app
proof covers startup and first turn only.

The unmodified Rust reconstruction reports
`devin 3000.11.228 (053434014dbd)`; it is **not the original Cognition
binary**. Original-client resume, live vendor interoperability, cloud
ACP/session hosting and non-Linux native execution remain unproven.
Additional native methods, renewal/revoke, Anthropic Messages conversion
and community domain gaps are recorded in `DONOR-MATRIX.md` and the source
lock. No native parity claim follows from reconstructed local peers.
