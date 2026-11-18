# Rooted automatic SQLite history slice

2026-11-15. Independent continuation after local Lore commit
`83394a67e131310fa617c1a27b633f39e0053892`. Only this module repository is
writable. The existing public API9 includes rooted API8 storage; no new
host API is required for automatic SQLite list and transcript.

## Extraction and preservation before edits

- Keep the explicit checkpointed `historydb` converter and its binary
  round-trip, financial-journal refusal and resource regressions intact.
- Read the current official `fec9759` SQLite migrations, row codecs,
  message ordering and default path. Use SQL row ownership and IDs rather
  than trusting embedded JSON identities. Do not infer a desktop task-index
  association, read a financial database or acquire resume authority.
- Adapt the already authored sibling OpenCode rooted VFS and Unix lock/
  probe helpers into module-local `storage_vfs_*` and `storage_lock_*`
  files, keeping the finite probe methods in the VFS. Record exact donor revision and file hashes.
  Preserve read-only DB/WAL/SHM transactions, real SQLite locks, descriptor
  lifetime coordination and explicit unsupported-platform behavior.
  No new shared helper, external dependency or path-based fallback.
- Add `storage.go` for the finite default CLI source descriptor, handlers,
  lease-bound cursors and locators; add bounded schema/projection code in
  `storage_sqlite.go`. Wire only the ordinary API9 composite package.
  Until the separate resume recipe exists, declare `Resume: "none"`.

## Regression and implementation sequence

1. Add synthetic real DB/WAL/SHM fixtures and descriptor/handler tests.
   Establish a compiled RED for the missing ordinary automatic source
   before implementing registration.
2. Implement one read-only transaction per rooted invocation, fixed
   validated schema, bounded SQL/rows/JSON/text/parts/time and ordered
   session summaries/transcripts. Preserve current fork-copy usage,
   authoritative IDs, explicit zero versus unknown counts, reasoning,
   tool/media/body-reference boundaries and source bytes.
3. Reject foreign/stale cursors and locators, malformed/schema/resource
   failures and missing/incompatible WAL/SHM rather than returning empty
   or cached history. Check the exact lease before native acquisition and
   publication; rely on the existing SDK broker's final publication fence.
4. Verify a committed WAL row and an uncommitted concurrent writer using a
   separate synthetic process. Exercise rollback, missing SHM, rooted
   symlink/rotation, cancellation, revoked consent/off/config/source and
   late publication through the actual installed storage owner paths.
   Preserve coherent transaction results or explicit errors; never copy
   independently changing DB/WAL files or checkpoint/write the source.
5. Run focused race repetition, causal negative overlays and appropriate
   static/build/ABI checks only on usbox, isolated HOME/XDG, pinned offline
   caches, loopback-only guard, `GOMAXPROCS=2`, `-p=2`. No host broad suite.
   Keep physical host source/binary and future merged pin identities
   separate. Record exact commands, named failures and artifact hashes.
6. Update current provenance and capability claims, retaining historical
   evidence. Commit the green module slice with Lore trailers; no push.

## Current database incarnation check

Before final verification, cover official migration
`0024_session_database_identity` and the read-only consumer in
`sourceServerWorkspaceAliasRepo.ts`. The optional `scope='session'` identity
and the rooted primary file's device/inode must bind locators and therefore
cursors. A same-root replacement with identical session summaries must reject
an old selection. Older schemas without the identity row remain readable
under physical incarnation binding; never create or initialize that row.
First run the new rotation fixture against the pre-binding reader to retain
its actual assertion failure, then add only this selection binding.

## Separate minimal host plan after the current merge

Rank 1: the host owner adds a checked literal-argv recipe bound to the
selected source/session and current owned profile/file plan. Exercise
`--resume <sessionId>`, stale/foreign selection, hostile literal arguments
and zero-spawn rejection with the existing synthetic peer.

Rank 2: the host owner adds an owned process-incarnation duplex lease with
three pipes, bounded ordered chunks/credit, concurrent reverse replies and
stop, cancellation and joined revocation. Module adaptation then implements
the official vendor framing, V4 projection/query and financial uncertainty;
ACK/EOF never authorizes replay. Existing desktop processes require an
explicit owner handoff. No host edits are part of this SQLite slice.
