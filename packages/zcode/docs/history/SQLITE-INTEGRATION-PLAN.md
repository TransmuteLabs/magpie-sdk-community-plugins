# SQLite history and archived controller integration

Current status, 2026-11-15: the generic rooted StorageSource contract is
already supplied in API8 and available in API9. The capture proposal below
is historical design context. Automatic adoption now requires module
VFS/read-transaction, projection and lease/publication regressions, not a
new SDK snapshot promise. The separate 0.15 slice implements that default
CLI reader; current behavior and scoped evidence are in AUTOMATIC-HISTORY.md
and AUTOMATIC-HISTORY-EVIDENCE.md. The historical plan below is retained
for lineage, not as a current request for a new host capture API.

2026-11-14. Initial audit is read-only against archive and recovered
source. No installed DB, credentials, app state or host code was opened
or changed. JSONL conversion does not cover any database below.

## Three distinct data families

1. **CLI history**, `.zcode/cli/db/db.sqlite`. Archive
   `internal/sessions/zcode.go` uses session/message/part tables, with
   `title_source`, parent sessions, row IDs and JSON data. The recovered
   `apps/zcode-cli/packages/adapters/src/storage/session-store/sqlite-session-store.ts`
   uses Node's SQLite storage. Preserve unknown JSON, signatures, tool
   state, external body references and all original bytes.
   Archive `zcode_fork_test.go` requires forkOrigin copies to count zero
   when their original message exists; orphan copies still count.
   Cache input is included in input; reasoning is included in output.
   A copied message does not make every later message in that session
   a copy. Missing IDs inside JSON use the authoritative SQL row IDs.
2. **Desktop task index**, `.zcode/v2/tasks-index.sqlite`. Archive
   `zcode_tasks.go` reads nondeleted task_id/title/updated_at rows.
   User title takes precedence over a generated/default CLI title.
   The index has a separate chosen data root; a matching filename or
   workspace path is not proof that two DBs belong together.
3. **Archived Magpie financial controller**, explicit
   `native-account-state.sqlite`. Archive
   `zcode_account_journal.go` requires application_id **1296518721**,
   user_version **1 or 2**, native_journal and, for v2,
   legacy_zcode_reset tables. Rows contain scope/origin/version/raw
   payload; they are not session messages. Pending claims, generation,
   reset high-water values, acknowledgment debt, draw cooldowns, ticket/
   card bindings and unknown fields must survive. Fractional history
   milliseconds must not be silently rounded down. Legacy reset debt
   and tombstones remain meaningful even when no selected mapping exists.

The third family's old string generation is not the SDK's numeric
generation. Import may never treat it as a current admitted generation,
turn pending into ready, clear a host usage barrier or replay a claim.
Financial import requires a separate explicit mapping/reconciliation,
stopped destination, principal lock, backup and durable provenance.
Reading a historical DB never authorizes a vendor request.

## Smallest generic host capture contract

An automatic reader needs **a consistent SQLite snapshot**, not an array
of independently stamped DB/WAL files.

- A declared source identifies a host-authorized root and exact relative
  DB candidates, optional exact task-index companion and byte/row/time
  limits. The host retains consent, package/config generation, read
  authority and publish-time revocation/epoch checks.
- Preferred capture: SQLite online backup/read transaction into a private
  host-owned snapshot, with a bounded busy deadline. The writer's
  committed WAL contents belong to that read snapshot. No forced
  checkpoint, journal_mode change, migration or write to the original.
- A copying fallback is valid only under a proven stopped-source fence
  covering DB, WAL, SHM and rollback journal. Repeated stat/mtime/size or
  individually hashed files alone do not establish atomicity.
- Return immutable checkpointed DB bytes (existing
  `SessionDocument.Body` is binary/base64-capable), SHA-256, byte length,
  opaque source identity/capture epoch and relative logical name.
  Do not leak an unrestricted path, live handle or arbitrary SQL endpoint.
  A multi-DB capture declares consistency separately per DB; it cannot
  silently claim an atomic snapshot across CLI history and task index.
- Grant holds through materialization and rechecks before publication.
  Revocation/off/config replacement cancels reads and prevents row
  publication. A private temporary DB and its sidecars are cleaned up
  after the lease drains. Errors mean unknown, never empty or last-good
  authority.

The module owns schema/queries and vendor interpretation. The host only
captures and fences bytes; it gains no ZCode tables or financial rules.
Do not retrofit SQLite semantics into accepted finite JSONL capture.
Source6 automatic registration is a later consumer of this explicit
generic snapshot contract, not a fake reader descriptor today.

## Bounded module implementation sequence

1. Add a separate `historydb` package for checkpointed, explicitly supplied
   SQLite bytes. Reuse the SQLite driver version **already locked in the
   host dependency graph**, `modernc.org/sqlite v1.61.0`, or an existing
   platform SQLite tool. Do not add a new version/package, copy host
   internal imports, assume a new executable, or ship a JSONL substitute.
   Verify module graph equality when reusing a direct driver import.
2. Parse only private materialized immutable snapshots. Enforce SQLite
   header/schema/integrity, fixed known tables and bounded rows/JSON.
   Preserve raw DB bytes and row IDs; reject unsupported schemas.
   Standalone uncheckpointed WAL input must be refused explicitly.
3. Implement source-preserving SQLite bundle inspection/restoration and a
   **manual** SessionMigration binary file-plan adapter using existing
   SessionDocument bytes. It performs no automatic home discovery or
   original DB writes. Preserve source fingerprints and host backups.
4. Add task-index association and transcript/token projection only with
   explicit matching evidence. Test default/custom titles, real_user
   semantics, parent/fork chains, orphan originals, huge integers,
   malformed JSON, signatures and missing external bodies.
5. Add an independent financial-journal inspector and reviewed import
   preview. Unknown/pending/unmapped rows remain durable blocking debt.
   Actual import is an explicit write with scoped acceptance, source
   digest, principal lock, backup, crash recovery and no vendor effects.
6. Register automatic history reading only after the generic snapshot
   contract and Source6 authority tests are accepted. Live resume is
   separate from history reading and financial import.

## Required evidence

Use synthetic real SQLite files, not hand-written JSON exports. Include
committed/uncommitted WAL transactions, active writer/rotation and missing
sidecars; capture must return one coherent committed view or an error.
Test read-only/truncated/corrupt/encrypted/oversized DBs, unexpected
views/triggers/virtual tables, cancellation, privacy and source byte
preservation. A fork-copy mutation must fail the token regression.
An omitted pending reset/import debt must fail before any vendor dispatch.
Run actual native-host preview/apply/restore against explicit temp files
on usbox, with separate pure-reader and automatic-host-capture claims.

No runtime SQLite parity, automatic capture or financial migration is
claimed by this initial contract inventory.
