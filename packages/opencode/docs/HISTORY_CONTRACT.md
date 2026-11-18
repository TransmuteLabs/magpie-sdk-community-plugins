# OpenCode own-storage reader and authority boundary

Current status, 2026-11-14: the parent ACKed the storage seam and the shared
SDK API1.8 admits `session-storage-source`. The module implements a real
Unix read-only SQLite/WAL reader in the ordinary 0.6.0/API1.8 package. Linux has
runtime evidence; Darwin arm64/amd64 has build/vet evidence and a separate
POSIX lock implementation exercised through Linux sibling tests. Actual Mac
native loading and lock interoperability remain unproven.
The parent accepted the exact rooted-storage owned hashes. Its default
constructor, native entry point and manifest include the same full reader
previously exercised by the explicit EnableHistory fixture. Installation
still grants no storage rights: ordinary session-read consent and selected
root binding remain required. This does not claim complete OpenCode parity.

The vendor schema authority remains OpenCode 2.3.0 at
`83960f8a56b2f2b8ad0b751f23baaf9d0eb65ad9`. The API1.8 host proof uses the
parent's frozen Git tree `43b7e290d9c3370881780b23ad49222f5d27f44b`
over commit `9e88f10982a1d66c1b402fba27caddda8ac4ac4e`.
That tree includes shared uncommitted work; the commit alone does not contain
the storage implementation. Exact gates and immutable evidence are in
EVIDENCE.md. Earlier accepted package evidence on f536d820/d070 stays separate.

## Implemented seam

The generic host declares finite roots and families, persists revision-checked
root consent, issues a source-bound read lease and invokes
`session.storage.list` or `session.storage.read`. Private `storage.native`,
`storage.list`, `storage.read` and `storage.check` callbacks require the exact
active parent. Provider/account/generation/auth authority is absent.

The native SDK atomically duplicates a borrowed rooted descriptor with
close-on-exec set. Linux and Darwin open every component read-only with
no-follow checks; foreign family members and
symlinks fail. Raw legacy snapshots are bounded, charged once in full and
rechecked for identity, membership and content before publication. The broker
and sessions adapter recheck consent, root, module/package/config/worker
identity, enabled state and cancellation before publishing summaries,
transcripts or selected media. Revocation hides delegated data.

The module declares source `opencode-history`, native owner agent `opencode`,
root slot `opencode.data`, and one finite family containing `opencode.db`,
its WAL/SHM and `storage/{session,message,part,media}`. The agent wiring profile
`opencode-sdk` is a different identity and does not own that native root.
The host has no vendor SQL, generic SQL RPC or arbitrary raw HTTP route.

Default ceilings are sixteen roots/families, 256 entries per page, 10,000
entries/parts, 64 MiB aggregate raw reads, 4 MiB normalized text, 8 MiB aggregate
media and ten seconds per ordinary invocation. Completed catalogs release
their snapshot slots; pending pages retain one bounded consistent catalog.

## Actual module backend

`history_sqlite.go` uses `database/sql`, the exact host-locked
`modernc.org/sqlite v1.61.0` and `modernc.org/libc v1.79.0`, `query_only` and
a read-only transaction. Fixed queries bind selected session values. The
Unix VFS opens the real DB/WAL, takes shared database and SHM read locks,
and copies the bounded SHM index into private anonymous memory. Writes,
checkpoint/recovery locks and unsupported roots fail explicitly.
Linux retains OFD locks; Darwin uses coordinated POSIX locks. There is no
immutable/main-only fallback. The VFS ABI uses the locked libc's
C-memory slices and typed accessors; Go buffers stay Go pointers.

Implemented vendor behavior includes V2 precedence over duplicate V1 IDs,
completed-migration suppression of V1, database precedence over leftover
legacy JSON, ordered `session_part` before retained inline content, bounded
stored metadata/aggregate parts, local compaction boundaries and instruction
epochs. Selected `blob_ref` membership, stored byte length, MIME and SHA256
are checked before blob hydration. Repeated media is charged once; missing
external media is represented explicitly. Legacy files are actual JSON
objects, never a generated JSONL substitute.

`testdata/storage_writer.py` is a fixture-only, separate SQLite writer using
literal reviewed donor DDL. `testdata/storage-native` enables the reader in
the otherwise full package. The ordinary native source tests install that
package and use normal consent, discovery, transcript/search/media and
refresh owners. The HTTP fixture builds/runs the actual host with private
HOME/XDG and a seccomp loopback-only network supervisor. Python is neither a
packaged dependency nor the history reader.

## Remaining readiness boundary

- Native source access and the WAL VFS are implemented for Linux and Darwin.
  Other platforms refuse it; actual Mac native loading, rooted reads and
  concurrent lock interoperability still require runtime acceptance.
- Ordinary 0.6.0/API1.8 packaging includes history without an opt-in flag or
  companion package. Older hosts reject the declared storage minimum.
- Owner APIs and CLI binding exist. Rendered root-consent controls and media
  display have not been implemented or tested.
- Custom `OPENCODE_DB`, channel-specific filenames and `:memory:` are not
  supported by this finite default-root declaration.
- Replay/legacy/part projections are bounded subsets. Full native provider
  checkpoints, every donor part type and concurrent SHM capture stress have
  not been established as equivalent to the vendor.
- Native call/trace accounting is a separate parent-owned adapter; source
  summary usage is not proof of direct vendor-call import.
- No actual OpenCode binary, live account, user database or vendor was used.

The standalone SDK release remains independent. Global API/version ownership,
catalog/accounting/search/ledger work and fresh upstream integration stay
with the parent. This module checkpoint does not commit mixed host WIP.

## Implemented Darwin descriptor and lock ownership

The bounded Darwin source work is over shared host
`ae957e5a8a77553a6cf6f484c8b58a69393f3887`, with its concurrent shared WIP
captured by source inventory. The base commit alone does not contain these
changes. Host commits, public types and versions remain parent-owned.

The three rooted host layers share Linux/Darwin Unix siblings. The SDK owns
one `F_DUPFD_CLOEXEC` directory duplicate, performs component `Openat` calls
with `O_RDONLY|O_NOFOLLOW|O_NONBLOCK|O_CLOEXEC`, and requires intermediate
directories and final regular files/directories. It no longer owns an unused
second `os.Root` or reopens through proc-fd. Root identity remains device/inode.
Other platforms remain explicitly unsupported.

The broker checks metadata through rooted `Fstatat(AT_SYMLINK_NOFOLLOW)`,
without opening and closing the selected DB/SHM inode. Physical comparisons
use device/inode for both OS and metadata-only file information. A replaced
file with identical size/mode/mtime still invalidates publication. Darwin
grant lifetimes serialize across native libraries. After native admission,
broker member closes are deferred within a finite 256-descriptor budget.
The only shared client hooks track a registered parent and observe a
validated terminal frame. Cancellation and worker retirement do not prove
that a native handler stopped. An uncertain handler without a terminal keeps
that admission closed; no fallback/retry is introduced.

The common `history_vfs_unix.go` keeps the locked libc/SQLite ABI. Linux
locking uses named OFD commands in `history_lock_linux.go`; Darwin locking
uses `F_SETLK/F_GETLK` in `history_lock_darwin.go`. `history_lock_unix.go`
coordinates device/inode reader counts and pending closes, including initial
existence and VFS access probes. Closing one reader cannot unlock another
reader's shared DB or SHM ranges. A failed lock acquisition retains neither
a reader entry nor the temporary pending lock.

The actual POSIX sibling tests use two real read-only VFS transactions and
an independent Python SQLite writer with donor DDL. They verify committed
WAL-only rows, retained transaction view across an append, reset BUSY while
the second reader remains, successful reset after its close and a fresh view
after reset. Separate fcntl subprocesses demonstrate both retained locks and
the positive control in which an ordinary same-inode close drops POSIX locks.
Real worker wire tests preserve canceled admission until a valid terminal
and reject a structurally invalid terminal. Causal mutations and the precise
Linux/runtime versus Darwin/static gates are recorded in EVIDENCE.md.

The graph adds only Darwin transitives already selected and checksummed in
the host lock (`go-isatty0.0.24`, `go-strftime1.1.0`); no version was fetched
or newly selected. The ordinary package now includes this implementation at
API1.8 against the inventoried accepted host source. This source implementation
does not establish actual Mac runtime acceptance.

## Historical macOS proposal before implementation

This section records the pre-implementation checkpoint. macOS history was
explicitly unsupported then. All four
non-Linux refusals are real: SDK native descriptor access, broker rooted open,
session root identity and module WAL VFS. The ordinary auth/agent/UI package
does not thereby lose its existing platform declaration, but it gains no Mac
history acceptance.

After this checkpoint, the bounded Darwin source changes are:

1. Share Linux/Darwin `Openat`, `O_NOFOLLOW` and duplicated-descriptor access
   using the already locked `golang.org/x/sys/unix`. In the SDK native
   `storage_reader` files, remove the redundant `StorageRoot.root *os.Root`
   and `/proc/self/fd` reopen: the field is only used by `Close`. Keep the
   independently owned duplicated descriptor and finite-family checks.
2. Add Darwin broker and session-identity siblings beside
   `internal/extension/storage_grant_linux.go` and
   `internal/sessions/extension_storage_root_linux.go`, using native
   device/inode identity. Other platforms retain explicit refusal.
3. Add a Darwin module VFS beside `history_vfs_linux.go`. Establish real
   SQLite-compatible DB/WAL/SHM locking and descriptor-close ownership before
   admission. The pinned Darwin Unix constants do not expose Linux's OFD
   operations; directly reusing Linux fcntl numbers or substituting unrelated
   locks is not a valid port.
4. Run Darwin arm64/amd64 CGO0 build/vet and unsupported-platform checks only
   on usbox, plus Linux race regressions. Those are source/static evidence.
   Actual Darwin native loading, concurrent writer/WAL reset, media and
   revocation acceptance require a later isolated Mac runtime run.

No path fallback, user-database access, new dependency or global version
change is authorized by this plan. Default history activation waits for the
platform implementation and coordinated readiness decision.

### Historical read-only implementation review while the host was frozen

This review prepares the next bounded implementation; it changes no host
source and establishes no Darwin acceptance. The pinned `x/sys v0.49.0`
Darwin bindings expose `Openat`, `Fstatat`, `Fstat`, `Dup`, `FcntlFlock`,
`F_DUPFD_CLOEXEC`, `F_GETLK` and `F_SETLK`. They do not expose Linux OFD lock
commands. The pinned SQLite1.61.0 shared generated source
`lib/sqlite_g_000000000001ffff.go` includes both Darwin architectures.
Its `_unixClose` deliberately defers descriptor closure while inode locks
remain; `_closePendingFds` releases those descriptors afterwards. That is
concrete evidence that replacing Linux command37 with Darwin `F_SETLK`
without coordinating all closes would be incorrect.

| Proposed owned source/hook | Required change after the merged-host gate |
| --- | --- |
| SDK `pkg/extension/native/storage_reader.go`, new `storage_reader_unix.go`, existing platform siblings | Remove redundant `root *os.Root`; own one atomic close-on-exec duplicate of the borrowed directory fd. Share component-by-component `unix.Openat` with `O_RDONLY`, `O_NOFOLLOW`, `O_NONBLOCK`, `O_CLOEXEC`, intermediate `O_DIRECTORY`, and final regular-file/directory checks. No `/proc`, `/dev/fd` or path fallback. Narrow the unsupported build tag to exclude Darwin. |
| Host `internal/extension/storage_grant_linux.go` and proposed Unix sibling, plus minimal storage-owned hooks in `storage_grant.go` | Share finite rooted opens and add rooted metadata-only `Fstatat` checks for native primary/sidecars. Audit `storage.native`, `recheck`, raw-byte selectors and all close paths: a broker probe must not open/close a DB or SHM inode while a native transaction holds process-owned Darwin locks. Preserve root/config/consent/module/cancellation checks. |
| Host `internal/sessions/extension_storage_root_linux.go` and proposed Unix sibling | Share directory device/inode identity using native `Stat_t`; retain exact root-reference/revision/publication checks. Preserve explicit refusal elsewhere. |
| Module `history_vfs_linux.go`, proposed common Unix VFS and `history_vfs_darwin.go` | Share the locked SQLite/libc ABI and readonly operations. Keep Linux OFD locking in its platform sibling. Implement Darwin POSIX DB/SHM byte-range locks with a device/inode registry, local conflict/refcounts and deferred close ownership for every VFS probe/handle. Never substitute `flock`, numeric Linux operations or immutable/main-only reads. |
| Module `history_sqlite.go` | Audit its initial existence probe and transaction/VFS cleanup against the same Darwin inode ownership. Keep the real readonly transaction, `query_only`, selected-root budget and close-error publication behavior. |

The required ownership invariant spans the broker and module: closing any
descriptor for a locked Darwin inode must not release another live reader's
locks. A module-only mutex does not cover the broker's current reopen/stat/
close loop or another invocation. Before implementing, choose and test one
generic coordination strategy for the finite native family: metadata checks
that do not open the locked inode, rejection of raw native-member byte
selectors, and bounded serialization/ownership of overlapping native leases,
or a shared descriptor owner that defers every affected close. This is
descriptor lifetime coordination within the existing storage seam, not SQL,
account authority or an arbitrary filesystem route. Merely adding Darwin
Openat siblings does not resolve it.

Required causal gates are: two readers of the same DB/SHM inode; closing one
or a broker metadata probe while an external SQLite writer attempts reset;
writer appends/checkpoint/reset while a real transaction retains its view;
root/file replacement and symlink refusal; unchanged DB/WAL/SHM hashes;
selected media/dedup; and consent/config/off/cancellation immediately before
publication. Breaking deferred close or releasing the shared SHM lock must
fail those tests. Static Darwin arm64/amd64 CGO0 build/vet can run on usbox;
native Darwin lock interoperability remains an isolated Mac runtime gate.
The final ordinary full-history package remains disabled until this root/
VFS contract and the coordinated platform implementation are accepted.

## Historical proposal at c9f8058

The remainder records the pre-ACK design and evidence. Statements such as
“proposed”, “not implemented” and “future” below describe that historical
checkpoint, not the implemented API1.8 seam above.

## Concrete donor and contract evidence

| Evidence | Consequence |
| --- | --- |
| `packages/util/src/global-roots.ts` | Default data root is XDG data home or HOME `.local/share`, then `opencode`; configuration/state/cache roots are separate |
| `packages/cli/src/database-path.ts` | Default release/dev/beta/next/prod database is `opencode.db`; other channels choose a sanitized suffix. `OPENCODE_DB` can select another file or `:memory:` |
| `packages/core/src/database/database.ts` | File-backed native stores use WAL, normal synchronization and a busy timeout; a main-file copy can omit committed data |
| `packages/core/src/session/sql.ts` and migration DDL | Sessions/messages, ordered separate `session_part`, instruction state and media blobs are distinct tables; a message's inline JSON is insufficient |
| `packages/core/src/session/history.ts` | Parts precede retained legacy inline content; replay applies compaction/model boundaries and instruction baseline epochs |
| `packages/core/src/session/blob.ts` | `media:` references need blob hydration, or the donor's explicit missing-media representation |
| `packages/core/src/database/v1-migration.bun.ts` | V1/V2 stores and migration state need schema-aware selection/deduplication; file presence alone is insufficient |
| Archived fork `893f560cccdeb8fa2e98eadb60427990c10eedcc`, `internal/sessions/opencode.go` | Legacy `storage/session/<project>/<id>.json`, `message/<session>/<id>.json` and `part/<message>/<id>.json` are JSON objects, and can remain after database migration |
| Public `pkg/extension/session_source.go` | `Container` must be `jsonl`; only flat, bucketed and directory/log.jsonl layouts can derive a session ID |
| Host `internal/sessions/extension_source_snapshot.go` | The host enumerates those physical JSONL layouts and snapshots documents before calling the module. It cannot list logical sessions inside this SQLite file |

`testdata/donors/provenance.json` pins literal roots, history excerpts and a
reviewed seven-table DDL selection. `history-schema.sql` decodes only JS
backtick escapes and retains the literal staging-message rename. It is a
fixture schema selection, not a complete migrator or application database.
No donor code is executed locally.

`history_contract_test.go` uses public SDK validation. Its positive control
admits the existing finite JSONL declaration at API1.6; changing only the
container to actual SQLite or JSON fails the named descriptor check. Every
existing layout rejects `.db`, `.db-wal` and actual legacy JSON locators.
The ordinary package still advertises no history reader.

`testdata/history_contract.py`, run only on usbox, creates synthetic data with
the literal DDL and Python's standard-library SQLite. It proves:

- A read-only connection sees a session committed only in WAL while a copied
  main database contains zero sessions.
- One read transaction retains its view across a later writer commit; another
  uncommitted message remains invisible.
- Separate assistant parts sort by ordinal and precede legacy inline content.
- An instruction baseline retires an earlier instruction notice; media bytes
  come from the separate blob table.
- The test reader refuses a DELETE.

The `--negative-main-only` control must fail `history read lost committed WAL
state`. This is evidence of the required storage semantics. It is **not**
execution of a Go module history reader, the actual OpenCode binary, its full
migrations or its full replay/media decoder.

## Minimum generic contract requested

Keep consent, host-selected roots, cancellation, source limits and
package/config/publication revocation from Source1.6. Add the following
capabilities; these names are sketches for review, not promised wire methods.

1. **Host-selected storage read grant.** The host resolves an explicitly
   declared own data root/database under approved HOME/native slots, records
   consent, and supplies an opaque grant bound to module instance, package
   generation, configuration version, source and root identity. Ordinary
   action parameters, model credentials or an account-operation receipt cannot
   select a root or expand the grant. Custom XDG/channel/`OPENCODE_DB` locations
   need explicit host resolution/consent; `:memory:` is not a readable store.
2. **Module-owned logical list.** Invoke a bounded, cancellable source list
   against that grant, with page/entry/byte/time limits. The module enumerates
   logical session metadata and returns opaque, source/store-bound locators.
   The host retains ownership of pagination validity and publication checks.
   It must not infer a session from the filename or copy vendor SQL/schema
   knowledge into the host.
3. **Selected logical read.** Invoke summary/transcript read with the selected
   locator, source/store grant, snapshot/read lease and limits. Existing
   normalized summary/part DTOs can remain. The module owns schema selection,
   V1/V2 deduplication, ordering, compaction/epoch semantics, media hydration
   and truncation. The host rejects foreign locators and revoked roots before
   work and rechecks authority before publishing results.

The storage grant must provide a usable read-only root capability, including
bounded relative listing and random reads of approved database/sidecar files
or an equivalent native access handle. Symlink escape, inode/root replacement,
foreign roots, writes, hidden cache/index creation and authority inferred from
business input must fail. A path string or unrestricted HOME access is not a
grant.

For SQLite, **independent file list/read calls alone are insufficient**.
The granted access must let the module establish a genuine read transaction
over a coherent main/WAL view, including required SQLite read-lock/shared-memory
semantics, or provide an equivalent stable raw snapshot/read lease with that
guarantee. A checkpoint or concurrent WAL reset must not produce mixed versions.
Absent/busy/changed storage fails explicitly; no fallback to an older copied
main database, exported JSONL or the built-in reader.

The proposal does not require a vendor-specific SQLite query/decoder in the
host, a new host branch, or changing Source1.6's established JSONL contract.
The leader must choose the native/brokered access shape before implementation.
Generic grant availability is necessary, but does not itself implement the
module's SQLite reader.

## Concrete post-1.7 contract proposal for the parent

These are proposed names and limits for review, not registered SDK methods.
Reserve a separate `session-storage-source` capability after API1.7. Keep
Source1.6's JSONL descriptor/decoder unchanged. Do not raise the package's
minimum or advertise history until the complete ordinary native path passes.

The source declaration contains `id`, `agent`, `codec`, `transcript`, `resume`,
finite `roots` and finite `families`. A root uses either an exact HOME-relative
directory or an existing native root slot. A family declares its primary
relative filename, allowed sidecars and bounded relative subtrees. For this
module the declaration covers the selected `opencode.db` family including
`-wal`/`-shm`, and the legacy `storage/session`, `storage/message` and
`storage/part` trees. Additional channel/custom database locations require
separately selected consent; no recursive HOME grant or caller-supplied root.
The host interprets paths and finite sets, not vendor tables or migrations.

The minimum DTO shape for review is:

```text
StorageSource { id, agent, codec, roots[], families[], transcript, resume }
StorageRoot { slot, homeRelativePath | nativeRootSlot }
StorageFamily { id, primary, sidecars[], trees[] }
StorageReadLease { sourceId, rootRef, snapshotRef, familyId, limits }
StorageSessionLocator { rootRef, sessionId, key }
StorageListRequest { lease, cursor?, limit }
StorageListResult { snapshotRef, entries[{ locator, summary }], nextCursor?, cut }
StorageSessionReadRequest { lease, locator, mode, limits }
StorageSessionReadResult { snapshotRef, sessionId, mode, summary? | parts?, cut }
StorageDirectoryPage { snapshotRef, entries[{ fileRef, relativeName, kind, size }], nextCursor? }
StorageReadChunk { snapshotRef, fileRef, offset, bytes, eof }
```

`key`, root/file refs and cursors are opaque and source-bound. Directory/file
DTOs are private source callbacks, not browser/HTTP results. A snapshot lease
can span bounded pages only while its deadline and read view remain valid;
expiry restarts enumeration with a new lease rather than combining pages
from unrelated database snapshots. Summary/parts reuse the parent's existing
validated session DTOs. Rich usage/media accounting and diagnostic schemas
stay parent-owned; this proposal adds no competing usage/media protocol.

| Proposed boundary | Minimum private data / behavior |
| --- | --- |
| Consent/bind | Host validates installed revision + config version + source descriptor, selects the root and persists consent; returns opaque `rootRef`, never a browser-readable path |
| Read lease | Host issues opaque `snapshotRef`, tied to root inode/identity, source, package revision, module instance, config version, parent call and deadline; grants no account authority |
| `session.storage.list` | Host invokes module with `sourceId`, `rootRef`, `snapshotRef`, opaque cursor and limits; module returns bounded logical `{sessionId, locator, summary}` records, next cursor and cut flag |
| `session.storage.read` | Host invokes module with that selected root-bound locator, fresh or retained read lease, mode and limits; module returns normalized summary/transcript/media results |
| Private `storage.list` callback | Only inside that source call: `snapshotRef`, selected directory ref, opaque cursor, entry limit; returns typed file refs/relative names/lengths, no absolute paths |
| Private `storage.read` callback | Only inside that source call: `snapshotRef`, file ref, offset and length; returns bounded bytes and file identity/version, never an arbitrary URL or path |
| Native storage access | SDK-managed rooted read handle or equivalent VFS access under the same lease; supports a real coherent main/WAL read transaction. A JSON path or independently copied files is insufficient |
| Publish | Host rechecks consent, binding, root identity, package/config/module generation, cancellation and snapshot validity immediately before cache/catalog/transcript/media publication |

Initial hard ceilings proposed for the parent: 16 declared roots/families,
256 list entries per page, 10,000 enumerated entries per invocation,
256 KiB per raw read, 64 MiB aggregate raw reads, 10,000 normalized parts,
4 MiB normalized text, 8 MiB per media object and a 10-second invocation
deadline. The host can lower any ceiling; the module must respect the
selected limits, return explicit truncation for complete safe prefixes, and
fail rather than publish an inconsistent database view. An oversized store
is not silently treated as empty. Media bytes participate in the same budget
and publication recheck; an expired file/snapshot reference is unusable.

`Call.Auth` is empty and provider/account/generation are absent for both
logical methods and their callbacks. For a selected read, session scope
must equal the selected logical session; a list call has source scope only.
Raw callbacks inherit the parent source scope and cannot be invoked through
`module.call`, HTTP business input or another component's receipt. Locators
and cursors are opaque module data validated/bound by the host; replay across
root/source/package/config/lease or into another module fails.

For SQLite the module opens the already locked driver, starts a read-only
transaction and uses hard-coded vendor queries with bound session values.
The native storage adapter must make that transaction operate on the granted
root/file family, including required read-lock/shared-memory semantics.
It must not open an unrestricted pathname behind the broker. A coherent
immutable raw snapshot is an alternative only after DB/WAL checkpoint/reset
tests prove equivalence; no main-only or fingerprint-before/after shortcut.
There is no generic SQL RPC, vendor decoder in the host, writable grant,
arbitrary raw filesystem HTTP route or browser database access.

The current shared host's `go.mod` pins `modernc.org/sqlite v1.61.0` and
`modernc.org/libc v1.79.0`. Reuse the target host's exact locked driver and
checksums with offline caches; do not fetch a new SQLite version. This module
has not yet imported that driver. Lock availability and a working bounded
native/VFS read transaction must be demonstrated on usbox before claiming
the backend solved.

### Proposed ownership and exact integration files

I can own the following additive SDK/storage and module slice after the
current Zen/Console checkpoint, with parent coordination before any host
edit. These paths are proposed new files, not files already delivered:

| Owner / new files | Responsibility |
| --- | --- |
| SDK `pkg/extension/storage_source.go`, `storage_source_test.go` | Finite descriptor, limits, logical list/read DTOs, strict request/result validation and no-account scope rules |
| SDK `pkg/extension/storage_reader.go`, `storage_reader_test.go` | Parent-scoped root/file references, bounded raw read/list helpers and snapshot lifecycle |
| Native SDK `pkg/extension/native/storage_reader.go`, `storage_reader_test.go` | Native rooted access bridge; coherent DB/WAL read-transaction and cancellation tests with the locked driver |
| Host `internal/extension/storage_grant.go`, `storage_grant_test.go` | Private grant broker, child callback admission, root/file identity, budgets and revocation; no SQL |
| Host `internal/sessions/extension_storage_source.go`, `extension_storage_source_test.go` | Logical enumeration/read adapter, native owner binding, checked snapshots, cache publication and media recheck |
| Host `internal/gui/extension_storage_sources.go`, `extension_storage_sources_test.go` | Owner consent/status and selected session read through existing session APIs; no raw file route |
| Module `history_sqlite.go`, `history_legacy.go`, `history_replay.go`, `history_media.go` and corresponding `_test.go` files | Actual V1/V2 SQLite/legacy selection, transactions, ordered parts, epochs/compactions, usage and bounded media decoding |

Existing SDK integration points are `contract.go` (declaration/clone),
`wire.go` and `routing.go` (negotiated methods/capability), `sdk.go`
(handler/scope/result validation) and `stream.go` (only the new parent-bound
callbacks). Host callback admission is in `internal/extension/manager.go`;
current `internal/extension/session_source.go` shows the scope validation
boundary. Native bridge changes must preserve the existing native ABI or
explicitly negotiate a version change; a borrowed pointer/path is not a
portable wire contract.

Existing session hooks to reuse are `extension_source_registry.go`
(consent/native binding and epoch invalidation), `extension_source_adapter.go`
(`PrepareSessionSources`, summary/cache publication), and
`extension_source_resume.go` (`checkSelectedSource`, transcript/carry and
post-call recheck). Storage enumeration is a new path beside
`listSourceCandidates`/`captureSourceRequest`; do not disguise a database as
one JSONL document. GUI status is currently registered by
`extensionSessionSourceRoutes`. The parent remains responsible for approving
the final generic shape/version and integrating shared host hooks; no
private host branch or worktree is needed.

Ownership confirmation requested by the parent, 2026-11-14: the proposed
disjoint slice covers the generic storage-source contract/broker/native
adapter plus this module's actual reader. Parent owns media accounting,
rich usage and diagnostics; Curie owns decision1.7. Only narrow registration
hooks in shared SDK/manager/session files need coordinated edits. Do not edit
those owners' implementation files or start host/storage implementation
before the parent's acknowledgement of this seam. No host/storage change is
authorized or claimed by this proposal commit.

Closure evidence must distinguish: authored DTOs; real rooted broker; actual
module reader; and ordinary host-selected acceptance. Required negatives
include absent/revoked consent, foreign root/file/cursor/locator, path/symlink
escape, root replacement, package/config/off revocation during a read and
immediately before publication, time/row/byte/media limits, cancellation,
concurrent commits/checkpoint/WAL reset, uncommitted data and a main-only
negative. Include donor-schema fixtures for V1/V2 coexistence, split parts,
epochs, native/local compactions and present/missing media. Unit tests over
copied files or a green descriptor validation alone cannot close history.

## Additional boundaries and remaining module work

The database can also hold credentials imported by native bootstrap. A raw
storage grant is private material, not account-auth authority or public
business data. History code must read only its reviewed data and publish only
bounded normalized history; it must never turn a stored credential into
`Call.Auth`, a browser result or a log entry. The consent/access review must
account for secrets sharing the physical database.

A production read-only Go SQLite/VFS implementation, complete migration/
legacy selection, replay, media handling and conformance fixtures remain
module work. This repository adds no SQLite dependency and no Python/runtime
companion; the Python probe is a test. A driver/backend decision cannot be
claimed solved by the public root grant or by this fixture.

Console automatic delivery is a separate contract: host-selected session
lookup, deduplicated synthetic insertion, deleted-session cleanup and bounded
wake counters. Remote config/policy enforcement and instruction/skill Library
publication likewise need workspace ownership. None follows from a history
read grant or from the existing account-bound Console HTTP actions.

Ordinary native gateway usage/log collection is independent of history.
`testdata/full_e2e.py` now collects real owner usage/requests/trace endpoints
and the isolated host log after Go/Zen requests. That proves accounting for
requests the gateway served, without pretending to import historical direct
vendor calls from OpenCode's store.
