# Native Devin agent slice

Base: paired 0.3.0 runtime commit cd9992c7, documentation commit 29f49cd.
Host baseline: MAIN 9b8c60f47, including SDK agent launch/profile contract
5c0a8ef6c and generation replacement fix a721fd647.

## Scope and ownership

Add an optional SDK 1.5 agent-enabled distribution of this module. Preserve
the default SDK 1.4 paired manifest and execution behavior, and the optional
0.2.0 inbound-only distribution. The agent distribution uses the same module
ID/native paths and is an alternative installation, not a second concurrent
owner of those paths. All implementation stays in this repository.

The agent side means the local native Devin CLI communicating with Magpie
through recovered Connect RPC and native session identifiers. It does not
mean claiming to reimplement Devin cloud ACP, its hosted session service,
VM orchestration, local tool execution, assignment JWT issuance or native
account entitlements. Unsupported operations must fail explicitly.

## Source authority

Read-only recovered source under Agents/devinapp:

- RUNBOOK.md:33,55,203 and windsurf-api-client/src/client_slots.rs:76:
  WINDSURF_API_SERVER_URL redirects server.codeium.com RPC.
- chisel-agent-bin/src/main.rs:71: DEVIN_API_KEY overrides saved credentials.
- chisel-agent-bin/src/help/main.txt: native devin launch, --model, --resume,
  --continue and acp command; no shell expansion or invented launch flag.
- chisel-agent-bin/src/plugin_context.rs:1697-1733: native literal Basic
  key-key framing for unary RPC, not OAuth bearer normalization.
- Existing codec/EVIDENCE.md and outbound/EVIDENCE.md: generated native
  request/session field layouts and community plugin encoder/response source.

Capture exact source hashes in AGENT-EVIDENCE.md. Source artifacts and
captured credentials remain outside the module distribution.

Implementation checkpoint: the SDK 1.5 profile, handlers, native library and
synthetic ordinary-host chat/session proof are implemented. The proof binds
through the normal GUI action, then uses ordinary CLI run/env; cold CLI model
selection has the separately recorded process-local catalog limitation.
Full original CLI startup remains blocked by generic selected-account native
operation dispatch. See AGENT-EVIDENCE.md; no bootstrap success is fabricated.

## Implementation sequence

1. Add an independent agent manifest and native entrypoint. Reuse NewModule,
   its configured provider state, codecs and bounded raw bridge. Register
   SDK AgentWiring handlers only in the new slice.
2. Declare an explicit profile of the existing Devin agent. Use host-owned
   binding revisions for inspect/connect/set/sync/disconnect. Keep credentials
   out of binding values and user config files; declare environment-only
   launch metadata for the recovered endpoint/key and model flag.
3. Normalize host model-picker references into native model IDs. Refuse
   unknown actions/fields, absent host binding, stale revisions, malformed
   values and unsafe/non-loopback endpoint combinations. Do not silently
   select a foreign executor or manufacture an unspecified default model.
4. Serve native catalog/chat/session traffic using the existing bridge.
   Add only explicit refusals for unsupported agent bootstrap/control RPCs;
   never synthesize successful authentication, entitlement or session state.
5. Add unit/regression coverage for slice isolation, metadata negotiation,
   host binding CAS payloads, model translation and refusal behavior.
6. Add an actual ordinary-host fixture on usbox: install the real native
   library; discover/adopt the SDK profile using ordinary agent commands;
   launch a synthetic Devin wire client through the declared argv/env;
   discover native models, send/resume separate native session IDs and inspect
   real fake-vendor traffic. Verify no local key forwarding, cancellation,
   disabled-owner refusal and restoration/disconnect. No Bun backend.
7. Re-run paired behavior with the unchanged 1.4 manifest and inbound-only
   regression. Prove a new agent slice cannot lower its metadata floor to1.4.
   Keep original-client parity distinct from synthetic wire conformance.

## Gates and completion

Build/test only on usbox, with isolated HOME/XDG, pinned existing Go caches,
GOTOOLCHAIN=local, GOPROXY=off and GOSUMDB=off. No new dependencies, live vendor,
real account/config access, host writes, push or SDKNext/Source changes.

Record exact host/module source inventories, library/executable hashes,
commands and terminal results. Use negative variants for meaningful new
guards. If recovered native startup needs a missing generic SDK contract,
document its exact host seam and retain a fail-closed behavior rather than
injecting vendor logic or weakening host caller/account policy.

Completion checkpoint: the optional slice, regressions, three distributions,
native C ABI and actual agent/paired host fixtures are verified against
unchanged MAIN f4232ea76 on usbox. The module's executable inventory matched
the frozen tested input. Original CLI bootstrap, cold CLI dynamic-model
selection and remote literal-Basic policy remain explicit documented host
boundaries; no full native protocol or original-client parity is claimed.
