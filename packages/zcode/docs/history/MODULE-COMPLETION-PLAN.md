# Source-proven module continuation

2026-11-15. Writable scope is this repository on main, starting at
`687af12d52871e4d9995f9548639b005e68104e7`. No host, donor, installed app
or account files are writable. Verification uses usbox only, fresh HOME/XDG,
pinned offline caches, loopback-only network isolation and two Go workers.

## Authority and cleanup plan

Fresh official `origin/main` remains `fec975905a2abd4c07e35c40d51fa097a2258ec9`
(desktop 3.18.2, CLI 0.17.1). Community authority
`57d1cc6a9f49389d495ea9be8d986c70de7767bf` has the same `packages/zcode`
tree as the ported 0.1.66 commit `87a8cf03c62fa55ca845c51648e8859ece580336`.
Neither a historical proposal nor a version label establishes missing SDK
functionality. API9 model metadata, account tasks and API8 storage already exist.

1. `agent_options_test.go`, `agent_test.go`: protect effective context/output,
   reasoning and default metadata, including routing groups, unknown versus
   reasonless models, catalog changes, manual rules and user edits.
2. `agent.go`: delete the suffix-based static model lookup. Translate public
   API9 facts into the official schema-1 properties/optionSpecs. Use the
   host's previous catalog only to replace matching generated fields; keep
   unrelated/manual/user values and exact disconnect restoration.
3. `reset_test.go`, `reset.go`: lock exact official opportunity envelopes and
   durable unknown-result behavior. Official use accepts only `used:true`;
   opportunity success is code 0/granted true, denial is code 3301/granted
   false/positive integer next_try_at. A malformed response grants no retry.
   Expose bounded journal status and explicitly acknowledged uncertainty
   recovery without issuing another vendor mutation.
4. `window.go`, `web/control.ts`, `web/page.ts`, `web/index.html`, `ui_e2e.py` and their
   tests: finish selected-generation reset opportunity/history/recovery
   controls, preserving explicit confirmation, single-flight writes,
   cancellation/disposal and no automatic mutation replay.
5. Source/evidence documents and lock: record exact fresh refs and bounded
   negative/positive evidence. Preserve historical artifacts and socket REDs.

Run the new tests against old implementation bytes before changing those
bytes. Then run focused repeated race tests, UI tests/typecheck, vet, native
package build/ABI conformance, and relevant existing regressions. Go uses
`GOMAXPROCS=2` and `-p=2`; no host or competing broad suite is launched.
Final merged-host acceptance remains separate until the leader supplies a pin.

## Native wire and storage boundary

At official fec9759, actual native chat uses V4 topic snapshots/deltas and
`v4/command`, with command acknowledgment distinct from terminal execution.
`provider-runtime-headers.ts` requires correlated reverse requests and
cancellation; `zcodeAgentProcessManager.ts` owns process/workspace identity.
The module's current ordinary provider-file launch remains ordinary Messages.
No offline history ID, vendor ticket, PID name, cached socket tuple or JSONL
observation authorizes attachment to an existing execution.

SDK `pkg/extension/agent_metadata.go:AgentLaunch` declares static env/args,
while `account_operations.go` and `pkg/protocolext/inbound.go` describe bounded
HTTP operations. They do not expose the bound process's duplex stdio or a
checked live-resume owner. The smallest generic requirement handed to the
leader is a host-owned process attachment lease bound to the current profile,
workspace/session and package/config generation, bounded duplex framing and
reply/cancel delivery, with revocation through shutdown/publication. Vendor
V4 schemas, projections and financial admission remain module-owned.
Adding an arbitrary command launcher or loopback service here would not
establish that authority. No host edit is requested for the fixes above.
The smaller ordinary CLI resume seam is also missing: the official CLI's
`arguments.ts` and `run.ts` accept `--resume <sessionId>`, but the host's
`internal/sessions/sessions.go:resumeCommand` has no ZCode case and the SDK
source's `resume` value only selects an existing native recipe. A generic
checked recipe for a current source identity, cwd and owned profile could
enable ordinary resume without vendor code in the host. It would not supply
the live duplex V4 control lifecycle.

API8 rooted storage is an available adoption path, not a missing SDK promise.
Automatic SQLite requires a real read transaction through rooted descriptors,
including WAL/SHM locks and publication revocation. Independent DB/WAL copies,
`immutable=1` on a live DB, or treating diagnostic JSONL as history are invalid.
The current manual snapshot reader stays explicit; automatic capture and
live resume are not claimed by these changes.

## Leader handoff and candidate replay

Keep the two essential host requests finite in NATIVE-CONTRACT-REQUEST.md:
source/profile-bound argv resume, then an owned duplex process attachment.
Add a synthetic stdio peer and reusable wire examples in
`testdata/native-contract/`; self-test the peer on usbox before handing it to
the host owner. A peer self-test proves the fixture, not a missing host adapter.
Keep SDK-supported SQLite adoption and optional account tasks in separate rows,
with exact available request/reply types and bounded consumer acceptance cases.
No generic launcher or socket authority enters module production code.

Freeze a separate module copy beside the read-only candidate host tree
`36f8e629b1092b089d4d59e0f9d8473a1f602ff8`. Check its archive/binary identities,
build the ordinary package against that source, then replay existing agent,
auth, controller/MCP/history and page owner fixtures sequentially. Preserve
earlier REDs, mutants and the previous 09cfa results. Record candidate acceptance
separately from `adcf4ce5` ancestry and any future merge commit.

The first candidate UI-auth VM replay failed `initial account enumeration`:
its synthetic DOM omitted the five new controls that `mountControlPage`
requires. Preserve that replay log, add the same five IDs already supplied
by the page/app unit fixtures, then rerun this actual native HTTP consumer.
Keep production UI/account/auth behavior unchanged by the fixture correction.
