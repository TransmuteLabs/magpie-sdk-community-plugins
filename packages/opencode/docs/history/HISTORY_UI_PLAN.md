# Rendered history: proposed browser contract

Status: **proposal, not an implemented SDK API**, 2026-11-15.
The accepted runtime remains `ed9c811f9fa931e70da4c7e393ae4ba1b0844946`,
0.7.0/API1.9, with its explicit full API1.8 variant. No host edits are
authorized while the leader finishes the merge. No native Console
connection or shared sign-in change belongs to this plan.

This review uses the immutable supplied host at
`usbox:/tmp/magpie-upstream-signing-final.QhgGhCd7/source`, physical tree
`36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, source archive SHA256
`5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc`.
It does not identify the leader's current mutable checkout or final accepted
commit. Recheck these seams against that final pin before implementation.
`testdata/history-ui-plan.json` records the source anchors and future
acceptance cases. Its `planned` cases are not passing runtime evidence.

## Existing owners and the exact missing adapter

Paths below are relative to the pinned host, not this module.

| Existing seam | Reuse and required narrow addition |
| --- | --- |
| `sdk/js/ui.d.ts:ModuleUI`, `sdk/js/ui.js` | Add an optional negotiated browser history facet after contract approval. Existing account/auth/config methods are unrelated to storage consent. |
| `internal/gui/pagebridge.js:call`, dispose handling; `assets/pluginpages.js` message dispatch | Keep the existing iframe-to-parent channel, exact `contentWindow` matching, pending-call rejection and frame AbortController. Do not introduce iframe fetch or another transport. |
| `internal/gui/assets/extensionwindows.js:answerPluginPage` | The trusted parent supplies `entry.ref`, request identity and abort signal. A history access request must always enter a parent-owned confirmation, never forward an iframe's `confirmed` boolean. |
| `internal/gui/extension_window.go:extensionWindowParts`, `resolveExtensionWindow` | Derive module, component, current package and config from the current opaque page ref. Retain the matching `WindowModule.PackageRevision` alongside its config version for source admission. |
| `internal/extension/module_window.go:windowRevision`, `windowEntry` | Already binds package bytes and configuration. Inspection must remain metadata-only; do not acquire `windowLease` merely to show consent controls. |
| `internal/sessions/access.go:Reads`, `SetReads`; `internal/settings/settings.go:SetSessionReads` | Existing ordinary agent-read gate and merge-under-file-lock writer. A trusted prompt may change only the declared source's native agent; preserve `All` and every other agent. `OwnOff` remains an overriding denial. |
| `internal/sessions/extension_reuse.go:sessionMigrationIdentity` | Already hashes manager namespace, module, package revision and config version. Reuse this identity instead of a second module-generation scheme. |
| `internal/sessions/extension_storage_source.go:selectedStorageRoot`, `BindStorageSource`, `storageConsented`, `storageSourceRecord.check` | Existing finite-root discovery, opaque root ref, package/config/root identity, checked binding and publication rechecks. Factor a source-scoped inspection path; do not inspect every module and filter afterwards. Existing bind compares root/package, not a previous consent revision: the window variant must also compare the inspected consent precondition under the same consent-file lock. |
| Same file, `UnbindStorageSource` | Existing release matches **agent + identity**, potentially several roots. Add an exact-selection owner helper under the existing consent-file lock. A module window must release only its module/source/slot/root/identity, preserving all other rows. Keep the existing CLI/HTTP convenience operation compatible. |
| `internal/sessions/extension_source_adapter.go:sessionSourceView`, `prepareStorageSources` in the storage owner, `extension_source_resume.go:checkSelectedSource` | Add a scoped query/projection of the existing immutable storage records. Select by module/source/root and current identity before any read. Preserve preparation admission, ambiguity rejection and publication checks. |
| `internal/sessions/extension_storage_source.go:storageSourceRead`, `StorageSourceMedia` | Reuse the actual native reader and its exact parent grant. The new window adapter must not use the broad `Find(agent,id)` selector in `StorageSourceMedia`; carry the selected storage record through to the read. |
| Same file, `storageSourceTranscript`; `pkg/extension/storage_source.go:StorageResult` | The current transcript projection retains image IDs in `Part.Name` but drops `result.Media` descriptors. Share the existing part normalization/budget and add selected media metadata; do not expose the raw storage result or bytes in a list/transcript response. |
| `internal/extension/storage_grant.go:NewStorageGrant`, `Recheck`; `pkg/extension/storage_reader.go:storageParent` | Keep descriptor/family/member authority, active parent ownership and before/after publication checks. No new SQL, filesystem, raw `storage.*` callback or native ABI contract is needed. |
| `internal/gui/extension_storage_sources.go:extensionStorageSourceRoutes` | Reuse strict JSON selection decoding, authenticated owner routes and safe MIME/no-store/nosniff rules. Existing global owner endpoints are not iframe endpoints. |
| `internal/gui/assets/app.js:confirmAction` | Reuse a trusted host dialog with host-derived module/source/root labels, explicit effects and cancel as default. Abort or a changed view while it is open must cancel admission. |
| `internal/gui/pluginpage.go:pagePolicy` | Existing CSP permits `data:` images, forbids iframe network, and does not permit `blob:` or inline audio/video. First preview can use bounded raster data URLs and text content without weakening CSP. |

The current `ui.read` is not a hidden history API:
`pluginpage.go:pageRead` serves provider names and aggregate usage only.
An account action cannot manufacture `StorageReadLease`; `storageParent`
requires the exact active `session.storage.list/read` parent.

## Authority carried by every operation

The parent/server constructs the authority tuple:

```
current page ref + window component
module ID + installed package revision + module config version
declared storage source ID + its native agent
declared root slot + inspected root ref + current binding identity/revision
ordinary Reads(agent) decision + OwnOff + current immutable source view
```

For this module the source is `opencode-history`, native agent `opencode`,
root slot `opencode.data`. Agent wiring profile `opencode-sdk` is a
different identity. Provider/account/generation selection contributes no
storage authority.

`windowRevision` already binds package/config. `sessionMigrationIdentity`
already includes package/config/namespace. `selectedStorageRoot` binds that
identity to source, root slot, resolved native root and physical directory
identity. Reuse these facts inside an opaque expected `viewRef`; do not ask
the iframe to assert package/config values or reconstruct their hashes.

An inspected root ref identifies metadata, not consent. An issued session,
cursor or media reference selects an already admitted object, not an
independent grant. Every operation resolves and rechecks the tuple before
dispatch and again before publication, including after a trusted prompt.
Unknown fields, trailing JSON, paths, caller-supplied module/agent/account,
credentials, callback names and endpoint URLs must be rejected.

The exact-binding revision must also change on revoke/rebind. The present
`Identity` is a package/config identity, not a consent-event counter. The
new owner must not claim that it detects that ABA using `Identity` alone:
derive the checked revision from the current persisted consent-file
precondition and exact row under its existing lock, together with the local
source epoch. Reuse checked file identity/content preconditions; do not add
a browser-writable revision or silently rewrite the stored consent schema.
Require a fresh inspection when that precondition changes, even if another
root's change conservatively invalidates the old view.

The existing root bind can also replace another source's delegation for the
same native agent. The trusted prompt must name that existing owner and
explain the replacement; deny it if inspection cannot identify it. Compare
that same precondition after the prompt rather than silently replacing a
newer owner. A stale/missing inspection never becomes implicit consent.

## Minimal logical contract for approval

These operation names describe proposed semantics, **not callable SDK
methods**. No corresponding imports, ambient SDK declarations or production
calls are added by this planning slice.

| Operation | Frame input and result | Owner rule |
| --- | --- | --- |
| Inspect | Declared `source`; return opaque `viewRef`, read/gateway status, finite root choices and this source's binding status | Metadata only: no DB/WAL read, worker start, global `Found()` enumeration or other module's roots/consents. |
| Request access | Issued `viewRef` and selected `rootRef`; return cancellation or fresh inspection | Trusted parent confirms agent-read permission and selected-root delegation as separate effects. It may call `SetReads(nil,{declaredAgent:true})` only after explicit host confirmation. It binds through the existing checked root owner. No iframe `allow`, `all`, `confirmed` or account field. |
| Release | Issued `viewRef` and exact `consentRef`; return fresh inspection | Revoke only the selected persisted row using the narrow helper. Do not turn off agent-wide reads, affect another source/root or fall through to a native reader in this window. |
| List | Current `viewRef`, selected root and optional issued cursor/limit; return bounded summaries and issued `sessionRef`s | Only current admitted storage records for the tuple. Do not forward to global `sessions.List/Find`, gateway history or another parser. A missing source returns missing/pending/denied explicitly. |
| Read | Current `viewRef` and issued `sessionRef`; return normalized bounded transcript, `cut`, read revision and media descriptors with issued `mediaRef`s | Reuse the selected record and `storageSourceRead`. Descriptors contain safe name/MIME/size, never paths, raw locators, bytes or remote URLs. Image part `Name` links only to a returned descriptor. |
| Media | Current `viewRef`, issued session/read/media references; return validated MIME and bounded binary payload | Re-read the same selected record, check membership and the read revision before publication. Changed transcript/media requires reload rather than silently substituting content. Parent fetches the authenticated owner response and transfers bounded binary data; no API URL/cookie reaches the iframe. |

The read revision must describe validated projected content, including
selected media digests/MIME/length, together with the authority tuple.
The broker's per-invocation `SnapshotRef` is not a stable content revision;
do not compare a fresh invocation's random lease token to a previous read.
If the selected content changes, reject old media selection and reload.

The two persistent gates are not one atomic transaction today. For the
access action, validate the whole selection, obtain the explicit trusted
decision, bind the exact root first, revalidate, then apply an approved
read-gate delta if needed. A failure returns actual fresh status, never
`ready`; root binding alone cannot enable reads. Do not roll back a newer
independent settings/consent choice with an old snapshot. Cancellation or
generation/root change forbids continuing a pending second step.
Read-gate comparison belongs inside its settings write lock as well; add a
narrow checked wrapper around the existing writer rather than trusting only
a frontend precheck. If another writer invalidates either persisted
precondition, return changed and require a new trusted decision. This
proposal does not claim a new cross-file atomic transaction.

`Request access` does not accept a global permission switch. It must explain
that ordinary agent-read permission applies to that native agent in Magpie,
while delegation is package/config/root-specific. Release revokes delegation
only. The trusted host UI/CLI retains release/recovery for a disabled module;
an already retired iframe does not acquire a lifecycle exception.

A fresh successful inspection replaces all previous view/session/media
selections in the module UI. Frame disposal and visibility transitions use
the existing parent lifetime. While visible, use one bounded metadata
revalidation at a time, at the existing catalog watch cadence; a changed
read/root/consent view cancels pending reads and clears transcript/media.
Do not rely solely on `watchExtensionWindows`: its current catalog/config
version does not report independent storage consent changes.
Revocation prevents subsequent publication; already delivered bytes cannot
be made unread retroactively.

## Bounds, rendering and compatibility

Keep existing storage limits: 256 entries/page, 10,000 entries/parts,
64 MiB raw reads, 4 MiB storage text, 8 MiB aggregate media and ten-second
ordinary invocations. Browser list responses must also fit a bounded JSON
response; issue a cursor rather than allocating an unlimited DOM.
Reuse the ordinary normalized transcript ceiling (currently 400,000 text
bytes) and its `cut` semantics. Binary media does not travel as base64 through
the account/module-call JSON RPC or inherit its input allowance.

Render stored text/name/model as text, never HTML. A selected validated
raster image may be encoded to a bounded `data:image/...` URL under the
existing CSP. Text attachments use text nodes. SVG/HTML are not active
previews; other MIME types get an explicit unsupported-preview state.
Inline audio/video and general download/export are separate extensions to
this first contract, not permission to widen CSP or fetch a stored URL.
No vendor URL, raw `file:`, `http:`, `blob:` or filesystem path from a
transcript is accepted as an image source.

Negotiate one optional versioned browser feature in the existing hello
exchange, scoped to current SDK windows declaring storage sources. Old
bridges report no feature; new SDK/module code must show unsupported and
make no calls. Existing account/auth/config/read APIs, CLI/global owner
routes, storage descriptors and native ABI remain compatible. No new
manifest capability, companion package or account binding is required by
this proposal; global SDK publication/version decisions remain leader-owned.

After approval, module work is `web/history.ts` plus its view/controller
fixtures, a small mount in `web/app.ts` and history markup in `web/index.html`.
It must be independent of `createAccountWorkflow`, account selection and
sign-in state. Do not patch `web/sdk-ui.d.ts` with imagined methods. Bind
only to the approved published SDK. Do not republish changed assets as the
already accepted 0.7.0 artifact.

## Bounded implementation and acceptance sequence

1. After the merge, repin source anchors to the accepted physical host and
   approve the logical contract/feature name. Retain this proposal unchanged
   as the pre-implementation source record.
2. Extend SDK/page/parent routing and the small scoped sessions owner.
   Factor existing root selection and transcript projection; leave SQL,
   native VFS, account paths and ordinary owner behavior with their owners.
   Add exact-row release rather than exposing agent-wide release to a page.
3. Implement the module view against the approved SDK; keep old-host and
   unsupported-media states explicit. Reuse the existing separate SQLite
   writer and stock native package in the acceptance fixture.
4. Run old-host/old-source behavioral RED, exact-scope/CAS/prompt/disposal
   negatives and passing account/config/CLI neighbors before claiming a fix.
5. On usbox only, one Chromium driver, sandbox HOME/XDG, pinned offline
   caches, existing loopback guard, GOMAXPROCS2/-p2: replay a real installed
   host/package with actual fixture DB/WAL and selected PNG. Preserve RED,
   command/result/network logs and before/after source/DB/WAL/SHM hashes.

The JSON matrix specifies the concrete cases, including cross-module/source/
root/session media, package/config/root replacement during a held response,
revoke/rebind ABA, exact release and no-account operation. Existing
`testdata/storage_writer.py` and `storage_e2e.py` provide actual synthetic
WAL/media bytes; `window_e2e.mjs` provides the real bridge/parent fixture.
A real rendered Chromium flow is additional to that VM fixture.
No real user history, live account/vendor, official OpenCode process,
Mac runtime, full GUI suite or WebKit retry is required or claimed here.
