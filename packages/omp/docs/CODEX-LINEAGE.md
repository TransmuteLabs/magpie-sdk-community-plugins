# Captured Codex ancestor history boundary — 2026-11-15

The module refuses a non-null `session_meta.history_base` without the opt-in
API1.10 checked `codex-rollout-lineage` carrier: importing only the selected
child would silently lose inherited context. Core owns native resolution and
original captures; this module owns materialization. This document specifies
the additive carrier and preserves the original source evidence and v1
attachment boundary below. It grants no filesystem authority.

## API1.10 acquisition and module serialization

The named capability is `session-related-inputs`, requiring API major1,
minor>=10 and `session-migration`. `SessionRequest.Related` is optional.
When present it contains `version:1`, `kind:"codex-rollout-lineage"`,
`selected` provenance and `ancestors:[{body,source}]`, oldest first.
`Files[0].Body` remains the untouched selected decoded JSONL. Each ancestor
body is exactly its original decoded prefix including its header and ordinals.

Every provenance entry has `path`, `rolloutId`, `encoding`, `rawBytes`,
`rawSha256`, `decodedBytes`, `decodedSha256`, `byteStart`,
`byteEndExclusive`, `ordinalStart`, `ordinalEndExclusive`, `prefixSha256`.
Counts and ranges are canonical decimal uint64 **strings**, including values
above JavaScript's exact integer range. The encoding is `jsonl` or `jsonl.zst`.
Byte start is `"0"`; selected byte end equals full decoded length. Original
raw/decoded digests bind the full source, while prefix SHA binds supplied bytes.
Paths are host-generated managed relative provenance, never requests to
discover/open a file or authority to select a root.

Core resolves immutable rollout IDs only in the selected Codex home's
`sessions` and `archived_sessions` stores. It checks selected ownership,
managed path/physical identity, complete decoded record boundaries and exact
contiguous original ordinals; detects recursion/cycles, missing/truncated
segments, divergent copies and symlinks; and retains original source snapshots.
Bounds are 32 segments, 128 captured representations, independent 16-MiB
raw/decoded/supplied budgets, 10,000 enumerated entries and directory depth16.
Original plain/packed capture and directory identities, full raw bytes and
selected store membership remain checked until apply.

`codex_lineage.go` in this module validates every ordered child base against
the immediately preceding captured rollout ID and exact decoded byte/ordinal
end. The oldest segment must have no unresolved base. It preserves selected
metadata, omits ancestor headers, removes sparse-only base/subagent pointers
and renumbers wrappers only in memory before the existing Codex importer.
The existing compaction/rollback, paired tools, images and unfinished-call
semantics then apply. Materialized native history remains bounded to8MiB.
No normalized journal replaces an original source capture.

The full cloned request, including ordered provenance and options, enters
`ActionPlan` revision. The native adapter also binds installed package/config
identity and read consent. Its context-aware revalidation checks original
captures and target/options before write/remove, after a write-lock wait and
at completion; later drift uses the existing rollback/recovery owner. It checks
authority before and after source rereads and observes the apply caller's
cancellation. No background `Manifest` reread is used. This is bounded
prepublication validation, not a filesystem transaction locking a live vendor.

`session.inputs.v1` is unchanged. Missing capability or ancestry retains the
explicit sparse refusal. SDK1.9 receipts below predate this contract; physical
API1.10 test identities and causal negatives are recorded in `VERIFICATION.md`.

## Exact source authority

Official source: `openai/codex`
`81b995a6c9a4c978f399775cfa5ce50702368749`, tree
`eeef8486f0e835d4f5a9eda9ebfb0f3e7367ad85`. The workspace declares the placeholder
`0.0.0`; there is no established release/client version for this source check.
The previous source pin was `7c2ce90716335c889a5076ded9a630459f9c9899`.

All paths below are relative to that exact Codex tree:

| Path / definition | Relevant semantics |
| --- | --- |
| `codex-rs/protocol/src/protocol.rs`, `HistoryPosition`, `SessionMeta` | `history_mode = "paginated"`; optional `history_base` identifies an exclusive inherited rollout prefix |
| `codex-rs/rollout/src/ordinal.rs`, `RolloutOrdinalState::for_new_rollout` | A standalone header starts at ordinal 0. A sparse child's header starts at `history_base.end_ordinal_exclusive`; its first local record follows at that value plus one |
| `codex-rs/thread-store/src/local/rollout_lineage.rs`, `resolve_rollout_lineage_with_mode`, `resolve_ancestor_candidate`, `RolloutBounds::read` | Follow physical ancestor IDs, detect cycles, validate metadata and assemble oldest ancestor through selected child; managed current/archived storage and immutable rollout filenames matter |
| `codex-rs/thread-store/src/local/thread_rollout_resolver.rs` | Current thread identity and immutable rollout identity are distinct after revert |
| `codex-rs/thread-store/src/local/rollout_lineage/ancestor_reads.rs`, `AncestorReads::open` | Open the actual ancestor representation, verify decoded length covers the cutoff, track failed reads and permit bounded alternative-copy resolution |
| `codex-rs/rollout/src/seekable_reader.rs` | Offsets address original decoded JSONL, including when durable storage is `.jsonl.zst`; decoded snapshots do not become new persistent ancestor files |
| `codex-rs/rollout/src/reverse_jsonl_scanner.rs`, `ReverseJsonlScanner::new_at` | Start a reverse scan at the exclusive decoded byte cutoff |
| `codex-rs/thread-store/src/local/model_context.rs`, `scan_model_context_from_lineage_blocking` | Consume ancestor byte prefixes plus the child delta, omit segment headers and restore the selected child's metadata; a replacement compaction with a window number can terminate the reverse scan |

The exact `history_base` fields are:

```json
{
  "thread_id": "11111111-1111-4111-8111-111111111111",
  "end_ordinal_exclusive": 3,
  "end_byte_offset": 756
}
```

- `thread_id` is the **immutable rollout ID**, despite the historical field
  name. After revert it need not equal that rollout header's stable thread
  `id`. Resolving only the current SQLite row for a thread can select the wrong
  file.
- `end_ordinal_exclusive` is a `u64`: the first ordinal excluded from that
  ancestor. Source resolution rejects zero because the inherited prefix must
  include session metadata; a child's first local ordinal is checked
  `end_ordinal_exclusive + 1`.
- `end_byte_offset` is a `u64`: the position immediately after the last included
  JSONL record, measured in decoded bytes. It is neither a compressed-file
  offset nor a line count.

The selected header's `id` is the owning thread and `session_id` is the root
session. `forked_from_id`, `attached_thread` and
`forked_from_ordinal_exclusive` describe the logical fork. They do not replace
the physical `history_base`: revert can change the latter independently.
`subagent_history_start_ordinal` and attached-child history-page rules can hide
inherited context from UI projection while retaining it as model context.

## Minimal reviewed synthetic pair

`src/testdata/codex-sparse-ancestor.jsonl` has four records:

| Ordinal | Record | Included in the child? |
| --- | --- | --- |
| 0 | Standalone paginated session header | Capture for identity/lineage validation; not a conversation item |
| 1 | User: `Inherited request` | Yes |
| 2 | Assistant: `Inherited answer` | Yes |
| 3 | User: `Parent continuation outside inherited prefix` | No |

`src/testdata/codex-sparse-child.jsonl` has two records: its own header at ordinal 3
with the exact base above, and a user message `Only the child suffix` at ordinal
4. Its logical `forked_from_ordinal_exclusive` is also 3 in this simple case.
These fixtures are synthetic schema examples; `cli_version = "0.0.0"` is not
evidence that a binary wrote or accepted them.

The required ancestor capture is exactly bytes **`[0,756)`**, including the
three newline terminators and excluding the fourth record. Its SHA-256 is
`dfed2bfd17573fb388783a6c8b394fa8b40a0d44d0276d2610069cb3db1b2879`.
The entire ancestor file has a different SHA-256:
`cc55378b032f2e5086fdec7ad8241d855282765d0db3824e4cd18af30742391b`.
The child file SHA-256 is
`25087c8378be863a7427a946cf6d04adc6e7d4eb1289281feace3b45f0c28d2f`.
The portable conversation would contain the two inherited messages followed by
the child request, never the ancestor continuation.

A realistic placement puts the ancestor under `sessions/2026/11/14/` and the
selected child under `sessions/2026/11/15/`. Revert, archival or compression can
change the selected representation; neither a sibling filename guess nor a
logical parent thread lookup establishes the required rollout identity.

## Required selection, range and digest authority

The native resolver and checked carrier follow these acquisition requirements:

1. Anchor acquisition to the host-selected child's captured bytes, native
   identity, read consent and selected store. Resolve each `history_base` by
   immutable rollout ID inside authorized managed storage. Verify the selected
   file/header relationship without assuming rollout ID equals owner thread ID
   after revert. Detect cycles, missing/truncated segments and budget overflow.
2. Capture each ancestor's header and exact decoded prefix ending at its
   recorded offset; recursively resolve its own base. Verify complete-record
   boundaries and consistency of ordinals/cutoffs. The model-context reader
   uses byte cutoffs while history projection also uses ordinal ranges, so
   neither value alone proves the other's correctness.
3. Preserve ordered segments, selected-child metadata and supported
   compaction/rollback semantics. A newer authoritative replacement compaction
   may make older model items unnecessary; a child `history_base` alone does
   not establish that optimization.
4. The host computes digests from bytes it actually captures, binds segment
   ID/path/representation/range and ordered lineage to its source snapshot, then
   rechecks applicable source identities and bytes before apply.
   `HistoryPosition` carries **no SHA-256**, expected filename or discovery
   root. A capture digest fences later mutation; it cannot authenticate what
   the file contained when a historical fork originally happened.

For a plain whole-file capture, existing v1 full-file digest checks can be
reused after resolution. They hash the entire file, not the 756-byte prefix:
even an append outside the inherited range invalidates that snapshot. A
range-aware capture must explicitly define that different mutation boundary.
For compressed ancestors, distinguish durable representation identity from the
decoded prefix digest and enforce decoded-byte/decompression budgets.

## Can unchanged `session.inputs.v1` cover this?

**No, for ordinary native sparse lineage as currently defined.** Its strict
v1 result is `{"version":1,"files":[{"path":"…","sha256":"…"}]}`. It accepts
whole files at clean relative paths under one root. For Codex that root is
`filepath.Dir(selected.Path)`, ordinarily the selected day directory, not the
whole `sessions/` plus `archived_sessions/` namespace. The planner receives one
captured child and is invoked once. It has no ID-to-path lookup, enumeration,
recursive acquisition, range field or way to learn an ancestor's digest.
The header supplies neither a timestamped path nor a content hash.

`..` paths, absolute paths, an unknown-range field, a guessed digest or an
empty digest do not satisfy that contract. Many valid ancestors are outside
the root. The module's reverse-route planner intentionally returns no
attachments, and native conversion requires one selected source journal.
Supplying an arbitrary second document therefore cannot authorize lineage.

Contract evidence: `internal/sessions/extension_source_inputs.go`, functions
`collectSessionInputsIn`, `sessionSourceRoot`, `readSessionInputs` and
`sessionInputSnapshot.check`. The inspected immutable host physical tree is
`8fe78ab9df13d733d61462eacce19bff523c1e58`, based on
`c11acbc2bebec1fb213ef562900d35188cdf55fe`; this is not a claim about the base
commit's bytes or the leader's unresolved merged candidate. The file SHA-256 is
`ad739b3574e5e7a1b0fb847b6c382b6eebde733f50fef6af708d29e4d18bdd8f`.
The same exact contract-file hash was checked in physical host candidate
`36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, captured with parent
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`. That candidate adds no lineage
fields to v1; it is not attributed to its parent or a future merged commit.

The existing regular-file, bounded acquisition, digest, parent/root identity
and before-apply snapshot machinery is reused by the separate native resolver.
An earlier **core materializer** prototype was rejected because it moved vendor
conversion into core and obscured the selected original behind derived bytes.
Its complete physical proof archive is preserved separately, SHA-256
`42c8daef26ce84f346d087463ec473a1fe089c2bcf29e0b912f3e2dadb704840`;
it is not the implementation selected for integration. The API1.10 carrier
keeps original selected bytes, acquired exact prefixes and their provenance
separate, with serialization only in this module. Native consent and selected
managed-store authority are checked by core, never inferred from attachment
planning.

If a future plugin is to request acquisition by rollout ID/range instead, the
strict v1 attachment schema cannot express that request: a documented versioned
contract is required. Merely broadening v1's root or interpreting ordinary file
attachments as ancestry changes authority without specifying it.

Evidence packet:
`usbox:/tmp/magpie-omp-current-20261115.RKcxeFiQ/lineage-evidence.json`, SHA-256
`f6bac0ef65fa60dc38c3246c861c8e70bc6e584526ef242fd92d1d43c3e5102b`.
It records the full source SHA, eight source file blob IDs/SHA-256 values, the
immutable host contract identity and both fixture hashes. The separate original
`source-comparison.json` remains unchanged.
