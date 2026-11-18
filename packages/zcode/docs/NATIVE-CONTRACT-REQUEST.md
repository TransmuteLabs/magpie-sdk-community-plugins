# Bounded native completion handoff

2026-11-16 status: the proposal below is retained as historical evidence.
The generic source-resume and foreground process seams now exist in public
API1.10. The full module consumes them; current ownership, exact entry points,
configuration and limits are in [NATIVE-FOREGROUND.md](NATIVE-FOREGROUND.md).
The original proposal and its old absence claims are not current SDK status.

2026-11-15. This is a request to the generic host owner, with a synthetic
regression peer, not a declaration of new SDK capabilities. ZCode module
0.15.0 remains incomplete for actual native resume/control. Official authority:
`zai-org/ZCode@fec975905a2abd4c07e35c40d51fa097a2258ec9`, desktop 3.18.2,
CLI 0.17.1. A fresh fetch still resolves to that commit. Community
`57d1cc6a9f49389d495ea9be8d986c70de7767bf` adds no ZCode tree delta beyond
the already ported 0.1.66. Historical proposed contracts are not current gaps.

## Ranked work and ownership

| Rank | Required behavior | Owner and finite completion gate |
| --- | --- | --- |
| 1 | Resume the selected native CLI history session through the current owned profile | Missing generic host argv recipe. Resolve a current source/session and owned profile at invocation, then launch once with literal argv. Stale/foreign/revoked source, profile or config launches nothing. |
| 2 | Actual V4 execution, reverse authorization and stop/reconciliation | Missing generic host-owned duplex process attachment. Module owns vendor schemas and financial admission; host owns the selected process/stdio and lease revocation. A bounded HTTP AccountOperation is not this transport. |
| 3 | Automatic SQLite history list/transcript from current authorized sources | Separate 0.15 module slice adopts existing API8 storage in API9 for the default CLI database. See AUTOMATIC-HISTORY.md and its independent evidence; no new SDK blocker. Desktop index/custom roots are outside that bounded source. |
| 4 | Optional scheduled account work | Existing API9 account tasks support foreground-bound successors. No native-parity blocker established. Explicit billing/reset controls already work. A perpetual sign-in/daily calendar timer is a different lifecycle, not implied by the foreground task API. |

Essential native control includes finite send/stop, correlated reverse
request/reply/cancel, authoritative V4 subscription/query and uncertainty
across process loss. A new arbitrary shell, generic socket listener,
standalone account scheduler, automatic diagnostic collector, dynamic MCP
registry and Magpie-only settings page are unnecessary to complete those
operations. Automatic CLI history uses checked rooted storage; its acceptance
is separate from native V4/resume acceptance.

## 1. Resume recipe as data

The owner completed API9 base `66ac2f7f` (tree `94ec4c8d`) and is
considering this as the bounded API10 addition. Automatic SQLite history
continues on API9. Duplex planning is assigned to Linnaeus separately.
These proposed recipe fields are not advertised by the current module.

Official `apps/zcode-cli/packages/cli/src/arguments.ts:47` declares
`resume: {type:"string"}`. `run.ts:418–425` constructs the resume request and
rejects simultaneous `--continue`. This is ordinary interactive CLI launch,
distinct from `app-server --stdio` and distinct from attaching an existing
execution. SDK `SessionSourceDescriptor.Resume` /
`StorageSourceDescriptor.Resume` only accept `none|native`.
`internal/sessions/sessions.go:resumeCommand` has no ZCode case.

Smallest proposal: an optional recipe on both owned source descriptors,
with typed substitutions limited to the selected source's native session ID
and approved cwd. The host resolves the installed agent executable and the
current profile's existing environment/file-plan binding. No executable path,
credentials, free-form env, shell text or alternate profile comes from a
session document. The provider module never executes a subprocess itself.
The source/session ref must be a host-checked locator, not an arbitrary ID.

Illustrative data below uses proposed names, not existing method names:

```json
{"recipe":{"agent":"zcode-native","argv":[{"literal":"--resume"},{"variable":"sessionId"}]},"selection":{"sourceRef":"source-7","sessionRef":"session-9","profileRef":"profile-4","sourceEpoch":"11","profileRevision":"6","configEpoch":"8","packageGeneration":"3"}}
```

The host resolves the selection, checks current ownership and cwd, then
constructs:

```json
{"executable":"<host-resolved zcode>","argv":["--resume","session-9"],"cwd":"<approved workspace>","profileRef":"profile-4","shell":false}
```

Reusing that request after profile revision 7 yields
`{"outcome":"not-started","reason":"stale-profile"}` and zero launches.
Switching source, package, profile owner, config or selected session must also
fail before launch. A session ID containing spaces, quotes, `$()`, backticks
or `;` stays one literal argument. NUL/over-limit values fail before spawn.
Do not append `--continue`, parse a shell string, infer current execution
authority from imported history, or silently choose a new source/profile.
The actual ABI names and bound sizes belong to the host owner.

The declaration belongs to the same installed source owner as the
`zcode-native` AgentDescriptor/profile, not to a hardcoded vendor switch in
core. Both SessionSource and StorageSource selections need the same generic
resolution rule. For this module the finite substitutions are the native
session ID and already approved workspace cwd; argv is exactly
`["--resume", sessionId]`. The existing profile/file-plan environment remains
host-owned, including its `{config}` substitution. History cannot override
the executable, cwd policy, environment, account or profile. A newly
resolved profile revision must not silently replace the selected revision.

For this module `source.Agent == "zcode"` and the same package declares
`AgentDescriptor.ID == "zcode-native"`, `Profile.Target == "zcode"`.
The recipe references that descriptor ID. Resolve the current SDK-owned
profile with the same module owner and matching target; do not require the
source agent string to equal the descriptor ID, or accept a foreign owner
merely because its executable has the same name. ZCode requires only the
`sessionId` argv variable. Use the selected
`StorageSessionLocator.SessionID` or `SessionSourceLocator.SessionID`,
not a UI/display ID, storage path or locator key. Cwd is a checked launch
directory outside argv; no ZCode path/env/executable variable is required.
Unknown substitutions, NUL and over-limit values reject before launch.

The exact ordinary gap is
`internal/sessions/extension_source_resume.go:sourceResume`: it checks
`stale` and `Resume=="native"`, then calls `resumeFor` using the source's
agent/native ID/cwd/path. `sessions.go:resumeCommand` implements the legacy
shell-string switch and has no ZCode case. Resolve the generic recipe from
the selected record before that legacy path. Command display/copy text is
not a launch grant: actual execution must use the checked SDK-owned argv/
cwd/profile plan with a final current-selection/owner check. Missing or
stale ownership must not cause an implicit sync/adopt/config write or a
fallback to a different profile.

Acceptance: run `testdata/native-contract/peer.py --resume <literal>` as the
synthetic configured executable and compare its exact argv/cwd response.
Replace the file plan/profile/config/account while launch admission waits:
no child may start. Invoke source A's session under source B/profile B:
zero starts. Test removed package, revoked source and disabled agent. These
are future host acceptance cases; peer self-tests do not establish them.

## 2. Owned V4 duplex attachment

Exact official launch/transport evidence:

| Official source at fec9759 | Contract fact |
| --- | --- |
| `packages/shared/src/zcode-agent-runtime.ts:26–37` | Native executable receives `["app-server","--stdio"]`; Node bundle is `zcode.cjs`. The descriptor's runtime version 0.13.3 is not the CLI package version. |
| `packages/services/src/zcode-agent/zcodeAgentProcessManager.ts:399–476,526,1289` | Node entrypoint prepends its entry path; packaged binary uses those args; desktop appends `--surface desktop`; child stdio is three separate pipes. Workspace identity is distinct from cwd. |
| `apps/zcode-cli/packages/cli/src/run.ts:252–293,608–617` | `agent-server` and `app-server` enter the same protocol command. Input is the protocol stdin; stdout is protocol output; errors go to stderr. |
| `packages/services/src/zcode-agent/zcodeStdioTransport.ts:58–101,149–170` | LF-delimited JSON, UTF-8 decoder, independent stderr. Graceful close ends stdin before bounded owned process-tree termination and stderr drain. |
| `apps/zcode-cli/packages/bootstrap/src/zcode-protocol/transport.ts:19–43,220–270` | Responses/errors and V4 lifecycle/command requests cannot wait behind a long handler. Control must arrive after the target handler starts, without waiting for its completion. |
| `packages/shared/src/zcode-protocol/index.ts:457–503` | Messages are `{id,method,params?}`, `{method,params?}`, `{id,result}` or `{id,error:{code,message,data?}}`. They do not carry `jsonrpc:"2.0"`. |

Smallest proposal: host-owned attachment to a process launched through the
current checked agent/profile, with opaque lease/process incarnation and
workspace/session binding. Lease covers package/config/profile revision,
current selected account generation when credentials are used, cancellation
and publication. The host owns all three pipes, flow control, stdin EOF,
process-tree drain and revocation. Module receives bounded ordered byte
chunks and can send replies while an earlier command is pending. Module
owns protocol parsing/correlation, vendor headers, V4 projection/CAS and the
existing ticket/card/reset journals. No copied token or scope in JSON grants
authority. An already running desktop child requires an explicit owner
handoff; PID/name/socket tuple/cache/history evidence cannot substitute.

Finite official-shaped exchanges are in `testdata/native-contract/wire.json`.
For example, subscribe ACK precedes a separate owned initial frame:

```json
{"id":"subscribe-1","method":"v4/conversation/subscribe","params":{"topic":"conversation/session-9","connectionId":"connection-1","clientMode":"desktop-continuous"}}
{"id":"subscribe-1","result":{"ack":{"subscriptionId":"subscription-1","mode":"snapshot","logEpoch":"epoch-1"}}}
```

`transport.ts:482–490` wraps the ACK in `result.ack`; it contains no snapshot.
The subsequent `v4/conversation/frame` uses the `wire.ts` physical envelope.
The peer deliberately stops at the ACK rather than inventing a conversation
snapshot schema. Full initial snapshot projection remains module work.

`command.ts:435–482,558–632` requires stable command/client/session identity;
CAS and row-targeting commands add the documented baseRevision/baseLogEpoch.
`sendText` carries explicit modelSelection when modelExecution is supplied;
runtime secrets/tickets are only valid for idle immediate execution, never
durable ordinary queued input. `stop` can bind
`expectedForegroundExecutionId` to reject a late stop targeting a newer turn.

```json
{"id":"send-1","method":"v4/command","params":{"commandId":"cmd-send-1","clientId":"client-1","sessionId":"session-9","type":"sendText","payload":{"text":"fixture"},"issuedAt":1800000000000}}
{"id":"send-1","result":{"commandId":"cmd-send-1","status":"accepted","revisionAtDecision":1,"result":{"type":"inputAccepted","delivery":"startNow","inputId":"input-1"}}}
```

Accepted is admission, not terminal success. `v4/commands/query` can return
`"unknown"`; it cannot authorize replay or clearing debt. Process EOF after
acceptance remains uncertain until authoritative reconciliation; a new PID
must not resend an admitted command. Command ACK/query and sourceCommandId
projection must agree before settling an execution.

The reverse request is
`interaction/requestProviderRuntimeHeaders`, with **two distinct IDs**:
outer RPC id and params.requestId. Its params bind workspace, session, turn,
provider and model selection. Official
`provider-runtime-headers.ts:30–116` waits for its corresponding reply and
emits `interaction/providerRuntimeHeadersCancelled` on abort/timeout.
Successful result is `{headersApplied:true,requestAuth:{...}}`; false is
failure. Reply/requestAuth is process-private, never a UI/log payload.
The synthetic fixture deliberately replies `headersApplied:false` without
credentials. Foreign/stale/canceled requests must never receive auth.

Framing has two different bounds. `zcodeStdioTransport.ts` accepts split UTF-8,
CRLF, blank lines and a final nonempty JSON frame at EOF; U+2028/U+2029 inside
JSON text are not delimiters. No hard stdio line cap was found there.
`core.ts:65–100` specifies V4 physical frames **1 MiB**, logical assembly
**16 MiB**, **1024** fragments, **32** assemblies, **32 MiB** staged bytes and
**30 s** assembly expiry. Wire version is **3** (`core.ts:8`) despite the V4
API name. `wire.ts`/`wire-codec.ts` use UTF-8 byte fragments, base64 and CRC32.
These are vendor codec limits, not permission to impose a 1 MiB stdio line
cap: other RPC responses can exceed it. The generic host should expose
bounded byte chunks/credit; module enforces distinct per-envelope limits
with explicit fault/revocation, never truncation/drop. Any proposed global
line cap needs a separate source-backed compatibility decision.

Acceptance matrix for the generic owner:

| Case | Required observable result |
| --- | --- |
| `peer.py app-server --stdio` | Three separate pipes, no PTY/shell; exact literal argv, process incarnation owned by the lease. |
| Reverse request while send is pending | A reply and bound stop arrive before send completion; no serial-queue deadlock. |
| Fragmented UTF-8 / CRLF / U+2028+U+2029 / EOF tail | Exact ordered bytes and one decoded JSON envelope per LF; stderr never parsed as a frame. |
| Multiple frames per read / >1 MiB generic RPC line | No boundary assumption, corruption, truncation or silent drop; explicit negotiated bounds. |
| V4 malformed/oversized/reordered/checksum-bad fragments | Module-owned typed error, bounded assembly cleanup, no seq advance/publication. |
| ACK then process loss / unknown command query | One admitted command, uncertain outcome, no automatic restart replay/retake. |
| Off/config/package/profile/account revocation | Cancel in-flight callbacks, close owned pipes/tree, join, reject late replies/publication. |
| Foreign/cached tuple or process incarnation | Zero attachment and zero credential replies, including when old ownership has vanished. |
| Stop for old foreground ID | Does not stop a newer execution; ACK alone does not settle either execution. |

Run `python3 -I testdata/native-contract/peer_test.py` under the existing
usbox isolation runner. Seven self-tests exercise the peer's argv, pending
execution/reverse/stop exchange, duplicate/foreign rejection, loss after ACK,
15 MiB UTF-8/EOF output and malformed output. The peer's large response is a
generic JSON envelope, not a claimed V4 snapshot. It is reusable as the
configured synthetic executable for host tests. It does not implement
source/profile authority, frame reassembly or revocation; those owner
acceptance rows remain required, not green claims from fixture self-tests.

## 3. Existing storage and task contracts

Candidate SDK tree `36f8e629` already has
`pkg/extension/storage_source.go`, `storage_reader.go`,
`native/storage_reader_unix.go`: rooted finite family leases, native
descriptors, `storage.check/list/read`, byte/entry limits and revocation.
The 0.15 module descriptor declares default `.zcode/cli/db`, `db.sqlite`,
`db.sqlite-wal`, `db.sqlite-shm` and a rollback-journal presence check,
`Transcript:true`, `Resume:"none"` until the recipe is accepted. Its
module-local rooted reader is described in AUTOMATIC-HISTORY.md. Custom
native data roots require verified root selection, not filename association.

Existing request/reply data:

```json
{"lease":{"sourceId":"zcode-history","rootRef":"root-1","snapshotRef":"snapshot-1","familyId":"cli","limits":{"maxEntries":10000,"maxReadBytes":67108864,"maxParts":10000,"maxTextBytes":4194304,"maxMediaBytes":8388608}},"limit":100}
{"snapshotRef":"snapshot-1","entries":[],"cut":false}
```

That empty result is valid only after a successful real read proving empty.
Root failure, busy transaction, malformed schema or revoked lease returns
an error. ZCode module must adopt rooted descriptors and a read-only SQLite
VFS/transaction covering DB/WAL/SHM locks. The sibling OpenCode module's
`history_sqlite.go`, `history_vfs_unix.go`, lock/probe helpers and
`testdata/history_contract.py` demonstrate current public SDK use; they are
read-only references, not a proposed new dependency or implicit import.
Current `historydb.Read` is for checkpointed explicit bytes and must not use
`immutable=1` on a live DB or independently copy changing DB/WAL files.

Finite module acceptance: synthetic committed and uncommitted WAL, concurrent
writer, rollback/recovery and missing SHM; one coherent committed transaction
or error; schema/row/byte/time bounds; source-root symlinks/rotation;
list cursor from a different lease; revoked publication; row IDs/fork copies,
opaque signed blocks and zero-versus-absent counts; no writes/checkpoint to
source. Actual native installed StorageSource replay must precede claiming
automatic history. Archived financial DB adoption is a separate explicit
principal mapping/import with pending/unknown debt and rollback, not history.

`pkg/extension/account_tasks.go` already supplies:

```json
{"task":"declared-task","key":"conversation-1","job":"module-private-job-1","dueAt":1800000000000}
{"operationId":"host-owned-operation-1"}
```

These are `accountTasks.schedule` callback params/result; host-selected
provider/account/model/auth are not supplied by the caller. Descriptor names
a declared write action. Existing cancel callback is `{task,key}`; result
is `{canceled:true|false}`. Tick is
`{task,key,job,dueAt,model,proxy?}` and returns
`{outcome:"completed"|"not-started"|"uncertain",call?:OwnCall}`.
`docs/subsystems/extension-providers.md:181–259` binds scheduling to a
successful measured foreground, fresh tick admission and no restart replay.
Optional paid keepalive would need opt-in/module templates/cadence and
durable uncertainty, not another SDK timer. No official requirement was
found to turn ordinary ZCode Messages into a new perpetual paid task.
Explicit daily gift claim and reset opportunity work already have module
actions. Startup has no selected Frame.Auth; read-only quota must not start
mutating background tasks. A true calendar-only task requirement would need
a separate bounded contract and evidence, not misuse of this one.
