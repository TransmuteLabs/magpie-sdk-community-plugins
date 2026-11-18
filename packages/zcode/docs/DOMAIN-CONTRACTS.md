# Remaining ZCode domain work and scoped UI contracts

2026-11-13. Local module checkpoint:
`68e916efcc6c48eee4fb4739d121897dcffcaf95`.
It depends on unreleased SDK 1.4. Its 52 executable-source files, 2680 race
events and actual native auth/controller E2Es are recorded in
SDK14-EVIDENCE.md. This document proposes the next contract cut; none of
the proposed UI APIs is claimed to exist.
The subsequent headless continuation implements owner-only tools, the
official MCP quota credential codec and standalone offline readers/bundle
conversion; see HEADLESS-EVIDENCE.md. A later real manual SessionMigration
adapter and local typed UI 1.5 prototype are recorded in SESSION-EVIDENCE.md.
The fixed remote image-search continuation is recorded separately in
REMOTE-MCP-EVIDENCE.md. This does not implement a host auth broker,
installed UI or live resume.

The UI gap analysis below is historical: the selected-account slice has
since been reviewed and merged into host MAIN at `c9dd41e12`. The module
can now build its separate SDK 1.5 control-page variant using fixed
Binding.provider, backend.accounts and backend.call selection. Auth-broker
buttons still require the separate host-owned authentication contract.
That module variant is now implemented and verified separately in
UI-EVIDENCE.md; it preserves the default API 1.4 artifact. The historical
gap/proposal tables below describe the earlier contract boundary, not a
remaining blocker for the five scoped control-page bindings.

## Definite current UI gap

Current host source, inspected read-only:

| Source | Observed behavior |
| --- | --- |
| `magpie/internal/extension/module_window.go:206` | `CallWindowBinding` resolves the declared window binding and current window generation. |
| `module_window.go:227` | It constructs only `sdk.Scope{Plugin: id}`. No selected provider/account/generation reaches the native action. |
| `module_window.go:231` | A write intent is derived from module/window/binding/request identity, without an account selection. |
| `magpie/internal/gui/extension_window.go:150` | The iframe deliberately cannot supply module/revision/component/account/state/endpoint authority. Its request contains ref, binding, request and input. |
| `magpie/pkg/extension/composite.go:74` | `BindingDescriptor` names an action or service endpoint, with no account-scope declaration. |
| `magpie/internal/extension/manager.go:966` | `authFor` returns no credentials for an unbound scope; otherwise it checks the saved selected account/generation. |

Consequently existing `reset.*`, `billing.*`, execution controls and
account-scoped discovery cannot safely work from the present window
binding. Putting an account ID in action input does not grant Frame.Auth
or generation authority. This is a missing generic UI selection bridge,
not missing ZCode credential logic.

The module will not shell out to `magpie`, read the host account files,
copy tokens into config/HTML, or recreate the host authentication store.

## Smallest generic SDK 1.5 UI slice

The following are required behavior, not prescribed names or a final ABI.

### Sanitized module account discovery

A host-owned UI method lists accounts belonging to providers declared by
the current installed module. It returns only opaque account ID, provider
ID, display name, generation, expiry and status needed for selection.
No credential JSON, business/OAuth/session/PAT token, native journal,
vendor-user identity or arbitrary host state key is returned.

The query must validate current window/module/config generation and local
UI authority. A page cannot enumerate another module's accounts. Listing
does not start the module, sign in, refresh an account, or change selection.
Disabled/deleted accounts cannot become selectable write targets.

### Account-scoped declared actions

Add an optional binding scope policy with a default preserving current
plugin-only behavior. An account-scoped binding declares its allowed
provider(s), requires AccountAuth and still resolves exactly one declared
action/effect/component.

The host parent owns the selected account. It may issue an opaque selection
reference bound to the current window and saved account generation; the
iframe supplies that reference, never credentials or a free-form Scope.
Equivalent host-validated designs are acceptable if they preserve this
authority boundary.

For each invocation the host must:

1. Validate window/module/config/binding ownership and current selection.
2. Resolve the declared provider and saved account under this module.
3. Reject deleted, disabled, foreign or stale-generation selection before
   module execution. Do not silently upgrade the selected generation.
4. Construct Provider/Account/Generation in Scope and obtain Frame.Auth
   through the existing host owner.
5. Apply the declared read/write effect and existing operation journal.
6. Preserve request cancellation, component lease and configuration proof.

A write's identity must bind the selected provider/account/generation as
well as the existing window/binding/request identity. Retries retain the
same selection and intent; reusing a request against another selection
must fail rather than retargeting a financial action. Module reload,
configuration replacement, account refresh/logout and window replacement
must invalidate stale selection references. Read calls gain no write,
usage-invalidation or credential-replacement authority.

No vendor principal/profile/card/reset types are needed in the SDK.
The native controller continues verifying those from selected Frame.Auth.

### Host-owned UI authentication broker

The page also needs bounded start/finish/refresh/logout operations using
the existing host account owner and declared provider. Merely forwarding
`auth.finish` through a module action is insufficient: its credential
result must be published by the host, never returned to the renderer.

New sign-in is explicitly unbound; renewal/logout is bound to one selected
generation. The host returns only authorization URL, opaque continuation,
pending status or sanitized saved-account result. The existing transactional
refresh owner remains authoritative. Cancellation or a lost UI result must
not replay a consumed vendor exchange. A page may request opening the
returned authorization URL through the normal host UI policy.

Required regressions include foreign/stale refs, read escalation, logout
during lazy startup, refresh during an in-flight page call, account switch
while a write retries, duplicate request IDs, expired continuations and
absence of credentials in page responses/events/logs.

## Bounded remaining domain work

| Feature | Can proceed with current contracts? | Bounded module work and evidence |
| --- | --- | --- |
| Account/control UI | Root config/display can use existing window/config contracts. Selected reset/claim/usage/actions and sign-in UI need the slice above. | Native controller plus TS page/panels; no duplicated auth backend. Actions are already implemented and exercised headlessly. |
| Official MCP credential codec | Implemented for selected paid quota observations. | Separate JWT channels, verified profile and exact Team scope follow recovered `official-mcp/officialMcpCredentials.ts:299,413` and shared `official-mcp-auth.ts:18`; no PAT/free fallback. |
| Headless diagnostic/history tools | Implemented as eight local-owner diagnostic/history declarations plus fixed image search. | Actual native-host selected-account calls and informational usage-barrier behavior are verified. |
| Fixed official MCP tools/transport | Implemented for the recovered image_search/search_image endpoint/tool and protocol 2025-06-18. | Bounded JSON-RPC/HTTP/SSE codec, cancellation, exact endpoint and protocol/session IDs, sanitized errors and preserved result fields. No live endpoint invocation or dynamic tool publication in tests. |
| Live MCP tool discovery | Static packaged tools are supported; dynamic registry publication is a separate generic contract if required. | Current SDK `Manifest.Tools` and `tool.call` admit only the executing generation's declared tools (`tool_scope.go:12`). A model's `DynamicTools` metadata is not a module tool-registration API. |
| External-agent MCP connection | Account-scoped binding/grant is needed if an external client enters a standalone local service without a host-selected call. | Reuse host-authorized account scope/grant, not headers carrying copied vendor tokens. No per-vendor host resolver. Do not claim this transport is already supplied by window-service generation proof. |
| Offline native history readers | Standalone functions/tools and a real manual SessionMigration file-plan adapter implemented. | Native preview/apply/host-backup/exact-restore/source-preservation are verified; no automatic agent-reader or live resume is declared. |
| Native live resume/control | Offline file conversion does not authorize a running vendor execution. | Bind resume to verified selected identity and the module's admission journal; implement only recovered control exchanges, with cancellation/reconciliation and no automatic retake. |
| Archived SQLite journal import | A host-selected immutable export/file grant suffices; parser/dependency choice remains separate. | Synthetic schema fixtures, preview, source fingerprint, stopped-state destination lock, rollback and preservation of unknown/pending/high-water records. No direct host-secret or live-app DB access. |

### Official MCP boundaries

The recovered official MCP credential channel deliberately differs from
Off-Peak:

- ZCode session JWT is the user identity in Authorization.
- Coding-plan identity uses the business/MaaS JWT in
  X-Bigmodel-Authorization, not X-Coding-Plan-Api-Key.
- Personal versus Team target is explicit; Team organization/project form
  one validated pair.
- The recovered reserved-header set also includes MCP session/protocol
  headers. Static caller headers cannot override them.

Recovered sources explicitly warn that sending the old API-key channel
alongside the business JWT can cause stale-key rejection even when the JWT
is valid. These belong in a separate MCP codec with synthetic tests, not a
shared Off-Peak header helper or host authentication rewrite.
The existing host-derived ToolScope can carry selected authority; ordinary
tool arguments cannot choose credentials. Missing/expired paid material
must not silently downgrade a selected paid account to a free MCP identity.

### Native history boundaries

Recovered `session/claude-native/sessionHistoryJsonl.ts` reads JSONL;
`session/legacyTaskSessionFile.ts` preserves the optional legacy mode field.
Shared `validation.ts:2150` defines the legacy snapshot with meta/messages,
file changes and turn checkpoints. Its metadata also carries task/trace,
workspace identity, runtime epoch and Off-Peak linkage.

SDK `SessionRequest.Files` already transports explicit document bytes with
path/format, and SessionResult returns documents. Start with that bounded
offline adapter. Preserve signatures/opaque blocks and unknown fields;
refuse an unsupported lossy target rather than silently dropping history.
Do not interpret imported task/card/ticket IDs or a workspace path as
execution admission. Recovered `offPeakResidentExecutions.ts` tracks a live
control/completion lifecycle that is distinct from exporting a transcript.

## Sequence

1. Agree and test the generic UI selection/auth broker as SDK 1.5.
2. In parallel, implement pure MCP codecs and offline history parsers using
   synthetic fixtures; they do not wait for UI contracts.
3. Connect headless declared MCP calls and SessionMigration with native
   host fixtures under isolated HOME on usbox.
4. Add the selected-account UI when the generic bridge is stable.
5. Handle reviewed SQLite migration and live resume as separate explicit
   operations with their own uncertainty/rollback fixtures.

These are remaining features, not already implemented claims. No proposed
contract justifies moving vendor controller rules back into Magpie core.
