# OMP JSONL Source reader

The ordinary SDK1.10 package includes the public SDK1.9 Source reader and all six
migration routes. No tag, partial library or second install is required.
`SourcePreviewModule()` is a compatibility alias for `Module()`; both return
the same full manifest. The filename retains the earlier preparation history.

The manifest declares `session-source`, `session.source.read` and this
`SessionSourceDescriptor`:

| Field | Value |
| --- | --- |
| ID / agent / codec / container | `omp-history` / `omp` / `omp-v3` / `jsonl` |
| Store | slot `main`, native root slot `omp.sessions` |
| Layout | `bucketed`, ID from `filename-after-first-underscore` |
| Transcript / resume | `true` / `native` |
| Package host API / native ABI | `^1.10` / `1` |

## Pure decode and display

`ReadSessionSource` accepts `SessionSourceReadRequest` and returns
`SessionSourceReadResult`. Both the request and serialized result are checked
through the actual public SDK. The handler opens no files, fetches no URLs,
starts no agent and changes no process/session state.

This finite reader accepts the recovered 256-byte UTF-8 v1 OMP title slot,
followed by one v3 session header confirming the host-extracted UUID. Pi
journals without that marker, unknown versions, malformed trees and unknown
message/block semantics fail explicitly. The host must supply a complete
JSONL prefix ending in a newline; the module does not silently discard a
truncated tail. Input is capped at the module's existing 8 MiB budget and
10,000 journal entries, within the host's independent binary/frame budgets.

Summary title/cwd/start/last come from recorded fields, with the native
first-prompt title fallback. Recorded assistant and auxiliary `model_usage`
usage becomes nonnegative typed model counters. Copied pre-fork usage is
excluded. All recorded branches contribute spent usage, while the transcript
follows the selected parent chain and explicit `activeLeafId`.
Provider-supplied costs and `details.usage` aggregates are not imported.
Missing/unattributable usage stays unavailable; a recorded zero is known.
The recovered four OMP counters do not establish one-hour cache-write usage,
so `cacheWrite1h` is zero. Host pricing remains authoritative.

Source transcript projection is deliberately separate from portable migration:
it retains unfinished tool calls, selected thinking text and execution
history instead of applying carry's completed-tool/context filters. Text,
thinking, tool call arguments, results, compaction summaries and image order
map to the frozen `SessionSourcePart` fields. Error outputs receive a plain
`[error]` display marker. Tool IDs, provider signatures and original binary
media have no structured field in this DTO and remain in the original journal.
System-only context is not projected. Host part/text limits produce explicit
`cut` results with UTF-8-safe truncation.

Image parts use `kind: image`, `text: "[image]"`, matching the recovered
display projection. They do not imply original-image rendering or lossless
media export. Inline base64 and blob-reference syntax are validated, but
external images are never acquired by the reader.

Additional host-admitted documents must be JSONL descendants of the selected
journal's artifact directory. Subagent/advisor journals may start directly
with a v3 header, as OMP writes them. Every header UUID must be unique;
unrelated paths and `btw-history` are rejected. Their recorded usage and last
activity aggregate into the main summary, retaining each model. Descendant
history does not become main conversation context. Unknown usage makes the
aggregate unavailable; aliases and overflowing aggregate counters fail. Every
admitted child contributes a typed `{path, sessionId}` summary identity. The
host projects these into `Session.Subs` and ordinary gateway deduplication.

## Host-owned boundaries

The host owns discovery, native root/profile resolution, explicit takeover of
the existing native reader, read consent/OwnOff, checked reads, selected
identity, opaque snapshot and store-reference values, cache publication,
last-good/revocation policy, pricing, resume and migration application.
Project filtering grants no permission. This module returns no agent,
filesystem path, root, shell command or monetary cost in a read result.

OMP's global blobs can sit outside the approved `omp.sessions` root. The
module cannot grant that sibling directory or request `../blobs`. The
Source input action therefore returns an empty version-1 acquisition
plan for `id: omp-history`, `from: omp-v3`, `to: omp-v3` only. Existing migration
requests still call the original attachment planner unchanged.

API 1.9 declares `descendants.kind: journal-stem-jsonl`, excluding the
`btw-history` directory basename. The host enumerates only the selected
journal's stem subtree, with the shared 128-document/16-MiB budget plus bounded
depth and entry count. It supplies captured child paths in `request.descendants`
and binds content, physical identities, directory membership and permissions
to the checked snapshot. New/removed children invalidate an unchanged main
journal too. Symlinks, malformed children and duplicate header IDs fail.
No module-authored root or arbitrary path grant is involved. Original binary
media, packed files, SQLite and remote history remain outside this contract.

The owner paths are `internal/sessions/extension_source_snapshot.go`
(`collectSourceDocuments`), `internal/sessions/extension_source_inputs.go`
(`sessionInputPlan`, `readSessionInputs`), and `pkg/extension/session_source.go`
(`SessionSourceSummary`) and `internal/sessions/extension_source_descendants.go`
in the host. The builtin's corresponding recursive enumeration remains
`internal/sessions/omp.go` (`ompFilesUnder`).

## One ordinary build

Build/test on `usbox` with private HOME/XDG and pinned offline caches:

```sh
python3 scripts/build.py --output /tmp/omp-dist
```

The builder obtains the manifest from the same `Module()` that the native
library registers. It rejects a partial factory or inputs changed during
compilation. An optional `--sdk SDK_SOURCE` uses a temporary modfile, without
changing `go.mod`. The SDK directory is a build-time dependency, not a root
grant. The old `scripts/build_source_preview.py --output ... --sdk ...` command forwards here.

The equivalent manual build is:

```sh
(cd src && go test -mod=readonly -race ./...)
python3 scripts/build.py --output /tmp/omp-dist --sdk /path/to/selected-sdk
```

`src/cmd/native` emits the full factory manifest and registers the same module in
library mode. Building never installs it or takes over native ownership.

## Exact host owner integration

Native binding is a host-owner operation, not a plugin RPC. This
verification uses the Go owner API; checked CLI/UI exposure belongs to the
host build. Installation alone leaves the native OMP reader in charge.
After granting OMP read consent and selecting an installed package revision,
the host owner configures the module record and performs:

```go
entry, err := manager.Install(ctx, distribution)
if err != nil {
    return err
}
config, err := manager.ModuleConfig(ctx, entry.Manifest.ID)
if err != nil {
    return err
}
_, err = manager.CompareAndSwapModuleConfig(ctx, entry.Manifest.ID,
    config.Version, json.RawMessage(`{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":true,"enabled":true}]}}`))
if err != nil {
    return err
}
if err := sessions.BindSessionSourceNative(ctx, entry.Manifest.ID, "omp-history", entry.Revision); err != nil {
    return err
}
if err := sessions.PrepareSessionSources(ctx); err != nil {
    return err
}
row, found := sessions.Find("omp", selectedID)
if !found {
    return errors.New("selected Source session is not available")
}
transcript, err := sessions.TranscriptOf(row)
```

This is host-owner code, not an internal-package import allowed in the plugin.
Binding rejects a stale revision. The host resolves `omp.sessions`, enumerates
`<bucket>/<prefix>_<uuid>.jsonl`, reads a checked complete JSONL prefix and
extracts the UUID. There is no invented `session.discover` module method:
discovery is host-owned, guided by the manifest descriptor.

For each selected journal, the host invokes `session.source.read` with a
`SessionSourceReadRequest`: `sourceId: omp-history`, an opaque `storeRef`,
relative locator path and selected UUID, opaque snapshot, `summary` or
`transcript` mode, one `omp-v3` document and host part/text limits. Its root
lease carries the same session/operation and no borrowed provider/account.
The module confirms the header UUID and returns only the typed result.
If the optional planner is called first, its ModuleRPC input remains
`SessionRequest{id: omp-history, from: omp-v3, to: omp-v3, files: [journal]}`;
the version-1 empty acquisition plan requests no blobs.

## Module-owned read veto

Source reading requires this private module config:

```json
{
  "sessionSource": {
    "ownOff": false,
    "records": [
      {"sourceId": "omp-history", "owned": true, "enabled": true}
    ]
  }
}
```

The host-owner example above commits it through the module's checked config
compare-and-swap before binding/preparation.

`Module.Start` decodes an immutable record into instance-local
`ModuleContext.State`. The actual `session.source.read` RPC and only the
Source-specific input planner check that state before and after decoding.
Missing, malformed, ambiguous, duplicated or unknown record fields fail
closed. `ownOff`, an unowned record or a disabled record returns
`permission_denied`, including when the host accepts a fresh native binding.
The ordinary migration planner and converters remain usable without this veto.
The pure `ReadSessionSource` decoder still accepts already-supplied bytes;
it is not the public RPC authority boundary.

Records contain no filesystem root or `storeRef`. They grant no root access
and do not replace host read consent or exact package/config selection.
State is bound to the component instance and config version. Disposing an
instance atomically revokes its record, including callers that detached
cancellation with `context.WithoutCancel`; another allowed instance does not
revive it. Host off/removal/consent/binding revocation remains independently
authoritative.

An ordinary row with nonempty `SourceStatus` proves delegation; a successful
native Find alone does not. Consent/off/config/package/root changes remain
host lifecycle decisions and invalidate delegated access, including held
transcript/resume actions. Disabling a bound module must not silently expose
the old native parser's rows.

`verify_source_owner.py OWNER_PROBE DISTRIBUTION --repeat N` runs a separately
compiled host-owner probe in private HOME/XDG stores with a fixture-only OMP
executable and no inherited vendor environment. The probe is external host
verification code, not plugin implementation. The recorded E2E uses the
unchanged Source preview owner APIs and real native module; no host source
patch or production overlay is required. Its exact source/hash/build command
is in the verification packet referenced by `VERIFICATION.md`.

## Historical module permission acceptance

The separate permission closure uses the accepted integration working-tree
snapshot `ad234e70764a9837dbdb28084a224335865a7f5c`, based on
`831c3d74af9358fa8a45d77eed902fe7cd02a076`. Builds and tests ran only on
usbox with private HOME/XDG, pinned offline Go caches and no vendor access.
No host production code or SDK DTO was edited for this module change.

Evidence and exact hashes are retained at
`usbox:/tmp/magpie-omp-source-policy.1OGJWal8/completion.json`.
The final preview suite passes under race, alongside the unchanged default
suite. Policy/lifecycle and default-factory regressions pass at race count 20.
Three compile-valid mutations fail their named assertions when the module
record veto, post-decode check or instance/config identity check is removed.

The real native owner probe passes in 20 fresh processes. Each run first
obtains a fresh exact host binding for missing/unowned/disabled/own-off
records, then requires the direct native read to return both
`permission_denied` and the module's exact denial message. This proves the
module veto independently of stale host binding rejection. The allowed
record then serves ordinary List/Find/transcript/search/stats/native resume;
host consent/off and held-object denials remain effective. Journals are
unchanged and no sibling/global blob store is acquired.

This historical acceptance covered the Source module, not a whole-SDK, GUI
or live vendor verdict. In the unified package, migration routes remain
independent of the Source read record.
