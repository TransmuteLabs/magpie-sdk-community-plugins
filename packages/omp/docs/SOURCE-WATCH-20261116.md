# OMP source watch — 2026-11-16

This check follows the existing `source-lock.json` watch paths. Parent-fetched
refs were read with `git show` and `git diff`; donor HEADs, indexes, working
files and foreign untracked files were preserved. No donor checkout or
installed Codex/OMP command ran.

| Donor | Previous pin | Checked pin | Declared version |
| --- | --- | --- | --- |
| OMP | `7c49c11aa5bbaf3f418406583096168ad5b8c6dc` | `90935366da0c368a437da4343ae748b03a2ffe97` | `18.20.3`, previously `18.20.2` |
| Official Codex | `81b995a6c9a4c978f399775cfa5ce50702368749` | `aea4c6ee4b652d84b416731b11c2b8299d767209` | release unknown; workspace placeholder `0.0.0` |
| Codex-infinity | `2ecc8a92fe18fd43552529856078999d3f6888c9` | unchanged | `0.159.3-infinity.1` |

The independent packet is
`usbox:/tmp/magpie-omp-watch-sol-20261116.rj2fdtm6`.
`source-comparison.json` has SHA256
`b05f731e79716f788430bd84cc72f738bdb01f0d1cf432a58463be87d7f8ebfb`.
It records exact trees, per-path old/new SHA256 values and changed flags.
The previous comparison is preserved in the lock's `previousComparison`;
historical native/client acceptance receipts retain their original pins.

## Format and runtime classification

OMP changed 250 files overall. Within the ten locked watch paths:

- `session/session-entries.ts` removes optional `SessionHeader.seanceSource`.
  A journal without it already works; an older supplied header still grants
  no additional input or foreign filesystem lookup.
- `session/session-storage.ts` changes deletion/purge ordering. The plugin
  converts supplied bytes and owns neither deletion nor history/stat stores.
- Session context, title slot, listing/index, blob references, message types,
  canonical directories and compaction entries are unchanged.

Adjacent OMP checks include `docs/session.md`, `session-paths.ts`,
`session-history-format.ts`, artifact/history URL handlers and the canonical
AI/TUI/compaction message definitions. The skill-prompt change is a bounded
Markdown preview change; seance artifact/URL removal is donor runtime behavior.
Neither introduces a portable native schema change or plugin filesystem grant.
The persisted session remains v3 and the fixed-width title slot remains v1.

Official Codex changed 206 files overall. Among the seventeen locked watch
paths, only `core/src/compact.rs` changed: sampling hook/usage plumbing,
interruption/deadline handling and deferred compaction-start events. These
control live compaction execution, not the persisted `compacted` replacement,
response-item schema or reconstruction algorithm. Canonical protocol
models/items/protocol, history payloads, rollout ordinal/persistence readers,
rollback and sparse lineage paths are unchanged. Additional dynamic-tool and
app-server v2 schema comparisons are unchanged. The four Codex-infinity tool,
item and protocol watch paths are byte-identical; its whole revision is
unchanged.

There is consequently **no new portable donor format to port**. No seance
lookup, deletion scheduler, sampling hook, tool registration, account access
or donor runtime subsystem was added.

## Existing native round-trip defect found by the regression

`src/source_watch_regression_test.go` exercises OMP→Codex→OMP→Codex using
completed and failed native shell pairs, one selected user turn, commentary,
a final answer, private/developer records and an abandoned branch. It needs
no supplied assets and produces exactly one self-contained journal per leg.
It checks role/order, matching tool IDs, command/cwd, original result bytes and
error state, one completed turn, no private context, and no fabricated
`Recorded tool …` prose or dynamic-tool registration.

On the existing encoder this goes behaviorally red: the reverse OMP writer
emits commentary as a standalone assistant `stop`, followed by its separate
tool call. Exporting that journal back into Codex completes an extra turn.
This is a pre-existing writer defect, not a new donor schema change.

The focused correction keeps assistant text immediately preceding a call
inside that call's OMP assistant content array. That record has `toolUse`;
the actual final answer retains its separate `stop`. There is no new public
DTO, SDK capability, tool execution or invented tool result. Existing labelled
context for foreign tools without a native OMP counterpart remains unchanged;
the new artifact-free round-trip proof concerns native shell pairs.

The second regression checks current headers without `seanceSource` and
legacy headers containing an unsupplied path. Both produce an empty extra-input
plan, preserve the selected transcript and omit that private path from export.

## Evidence boundary

The packet preserves the pre-fix behavioral red, subsequent green receipts,
scoped package race/vet/format and build provenance, source inventories and
donor preservation records. All tests/build/format run on regular usbox with
isolated HOME/XDG, pinned offline Go caches and the existing network guard.
Its SDK is a private extraction of stable manifest
`e2ca42c6fa8529b781a61aec04862355a5b28183646cc57ee028f6133fb74c34`,
without the separately completed public-permit overlay. SDK permit receipts
are not reused as OMP acceptance.

Module identity remains `omp-sessions` `0.1.0`, minimum API `1.10`, ABI `1`.
This source-derived compatibility check does not certify a current installed
Codex, OMP or codex-infinity client. No live session, vendor, account, installed
application or model request is used. Parent owns aggregate `docs/STATUS.md`,
combined validation and release readiness.
