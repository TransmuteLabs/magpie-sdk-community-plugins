# Existing storage owner correction, no new SDK

2026-11-15. Module code is confined to this repository. No host source was
edited. Initial read-only acceptance candidate:
`usbox:/tmp/magpie-upstream-signing-final.QhgGhCd7/source`,
physical tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`.
The binary SHA256 is
`0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509`.
Parent-at-capture `adcf4ce5` is not its merge/final acceptance identity.

The owner subsequently supplied merged API9 base
`66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`, tree
`94ec4c8de4e5b366a6e3fb854df2176007587ab8`, at
`usbox:/tmp/magpie-upstream-signing-closure.ofRrrBdz/source`.
Binary SHA256:
`a8e09770909f78ae89cc6175a45bb98e1dacdb305b62856f46956928cb8ecbe2`.
Its `allFilesWith` still calls `zcode()` directly, with identical checked
file bytes. Its inventory's capture-parent field remains `adcf4ce5`;
that metadata is kept separate from the supplied merged commit and
matching physical/committed tree. Scoped replay results for each pin
belong in AUTOMATIC-HISTORY-EVIDENCE.md.

## Finite source-backed blocker for the host owner

`internal/sessions/sessions.go:allFilesWith` already calls
`storageNativeFiles("opencode", openCodeFiles)`, but invokes the passed
ZCode lister directly with `zcode()`. Both `allFiles` and
`allFilesForTitles` use that function. `extensionSourceFiles` filters native
rows only after enumeration. The native ZCode lister opens SQLite before
that filter, so module delegation does not prevent the other reader's
source access. The synthetic installed ordinary list/transcript test
actually changes the SHM hash while DB and WAL hashes remain exact.

Smallest proposed existing-contract correction:
call `storageNativeFiles("zcode", zcode)` instead of `zcode()` in
`allFilesWith`. The generic helper already checks the selected source view's
`owners` map, including disabled/revoked ownership. It does not grant
storage consent or add a new SDK method. Apply the same principle to any
other native database owner when that owner adopts StorageSource.

The module-owned overlay fixture
`testdata/automatic-history/host_test.go.fixture` gives two finite negatives:

- `TestZCodeAutomaticHistoryDelegatedOwnerPreventsNativeEnumeration` binds
  the actual installed ZCode source, prepares the ordinary source view and
  injects a counted native lister into `allFilesWith`. Current candidate
  invokes it once; a delegated owner must invoke it zero times.
- `TestZCodeAutomaticHistoryInstalledListTranscriptAndWAL` installs the
  actual one-package native module and uses ordinary `Find`, `ListAgent`,
  `TranscriptOf` and `SearchSession` with a separate real SQLite WAL writer.
  It requires exact DB/WAL/SHM bytes after those reads, fresh WAL commit
  visibility and old selected-source rejection.

The same fixture separately exercises the actual installed storage owner
without native enumeration, plus immediate reads/consent/config/off/root/
cancel/foreign-process-off publication revocation and missing/symlink
sidecars. Same-root database identity/file rotation also invalidates a
selected source whose visible summaries remain identical, and a fresh
selection remains readable without source writes. Tests run in fresh host processes: the candidate's native image
cache deliberately refuses repeated unknown dependency-equivalence images
in one process. No cache bypass or production workaround is proposed.

Ordinary-list storage acceptance needs the owner's correction and its new pin.
Module tests, package/ABI and rooted storage acceptance are reported
separately from the known ordinary-list candidate RED. The minimal future
resume recipe and owned duplex lease remain ranks 1 and 2 in
NATIVE-CONTRACT-REQUEST.md; no host edits for those belong to this slice.
