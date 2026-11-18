# Automatic CLI SQLite history

The ordinary native/UI package 0.15.0 declares public API9
`session-storage-source`, using the rooted storage contract introduced in
API8. The host inspects and explicitly binds the finite HOME-relative
`.zcode/cli/db` root and retains ordinary `Reads("zcode")` consent.
The declared family contains only `db.sqlite`, `db.sqlite-wal`,
`db.sqlite-shm` and the rollback-journal presence check. No additional
installation, account, credential, SQLite executable or new dependency is
required. The historical headless API1.4 package is unchanged.

The module uses the host's public native root descriptor and a module-local
read-only SQLite VFS. One bounded read transaction covers schema, session,
message, part and fork-origin lookups. The VFS participates in real SQLite
DB/SHM locks, reads a private SHM index under the shared reader locks and
charges actual reads against the parent lease. It never changes, checkpoints
or independently copies the source DB/WAL/SHM. An unresolved rollback
journal, unavailable sidecar, malformed/unsupported schema, resource failure
or revoked lease produces an error rather than empty or last-good history.
Root opening retains the SDK's no-follow and identity checks.

`session.storage.list` projects bounded session summaries and paginates with
a cursor bound to the exact lease and normalized catalog. A changed lease
or changed catalog invalidates that cursor. `session.storage.read` validates
the selected root/session locator and reopens a fresh transaction for a
summary or transcript. Locators also bind the primary file's device/inode
and, when present, the official `zcode_database_identity` session generation.
Same-root file replacement or generation rotation invalidates an old
selection even when session summaries are identical. Older schemas remain
readable under physical file binding; the reader never initializes identity
tables or rows. Each handler checks the lease before acquisition and
after closing SQLite; the public SDK and ordinary session owner additionally
recheck result publication. No native execution authority comes from a
locator, imported row or history read.

The official fec9759 migrations/codecs/repositories define the default
path, SQL row ownership, soft deletion, nullable sequence ordering and
assistant blockIndex ordering. Embedded JSON IDs never override SQL
session/message/part ownership. A default title comes from a real user
prompt, without using runtime-injected reminders. A stored custom/generated
title survives. Copied assistant usage is excluded only while its declared
original row still exists; orphan copies and later original work count.
Native input already includes cache and output already includes reasoning,
so the reader does not add either again. SQL/JSON integer counters remain
exact. An explicit zero remains known; incomplete counters leave usage
unknown. Attempt/turn/tool-usage tables are not counted again as messages.

Transcript projection preserves native message order and, when every
participating assistant block has a valid blockIndex, reorders only those
blocks in their original slots. Text, reasoning and finite tool input/output
reach normalized parts. Unknown native parts, external bodies and media
produce explicit unavailable markers and `cut`; the reader never follows a
file URL or another root, exposes opaque signatures or invents hydration.
The original SQLite bytes remain untouched.

Default SDK limits remain 10,000 entries/parts, 64 MiB physical reads and
4 MiB text. SQLite engine allocation/schema limits are installed before the
first schema query; generated columns on the consumed tables are refused.
The module owns an eight-second ceiling within the ordinary host call
budget. Unknown views/virtual tables cannot substitute for the three
required tables. Official stored write triggers are permitted because the
transaction and VFS are read-only.

The VFS/Unix lock implementation is adapted into this repository from
`magpie-sdk-plugin-provider-opencode@ed9c811f9fa931e70da4c7e393ae4ba1b0844946`.
Exact consumed file hashes and the official source hashes are in
source-lock.json. No runtime import of that sibling or host internals exists.
Linux uses OFD record locks; Darwin uses coordinated process-owned POSIX
locks, retaining sibling descriptors until the final reader unlocks.
Other platforms explicitly refuse rooted WAL access.

The current slice covers the default CLI history database only. It does
not infer an association with a separately rooted desktop task index, select
custom native data roots or import financial journals. Checked source resume
and foreground V4 process control are separate API1.10 adoptions in
NATIVE-FOREGROUND.md; desktop attachment is not claimed.
Manual checkpointed binary conversion remains
available alongside the automatic source. Build/test/source evidence is
recorded separately in AUTOMATIC-HISTORY-EVIDENCE.md.
