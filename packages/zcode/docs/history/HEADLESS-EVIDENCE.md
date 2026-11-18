# Headless owner tools and offline history evidence

2026-11-13. This continuation follows
`2bc79c91b79bde922035f83ff5f5747f2a6ecc96` and depends on unreleased SDK
1.4. All compilation/execution ran on **usbox**, Linux amd64, pinned Go
1.26.8, isolated HOME/XDG and caches, offline module resolution. No live
vendor, installed app, real account, host secret file, SQL dependency,
push or activation was used.

## Exact source and artifacts

Remote proof root: `/tmp/magpie-zcode-headless-fvslzaj6`.
All **57 executable-source/module files** hash-match the local source.
SHA-256 of compact key-sorted JSON `{relativePath: fileSHA256}`:

`3d9bbba5f4c19604800a00cede8ed680af3f40322af25c70cac803746783c9f4`

The SDK public-source copy is unchanged from the 1.4 checkpoint:
60 files, manifest SHA-256
`98eac7779cb9e2c024025c0b1a6d0ccf58d287381f307097e1544efad2347c23`.

Native Linux ZIP SHA-256:

`1f5157d168ea7ce7ec6985036fdd14fe3147d425949c4bca8969e4a0b2003235`

Actual native-host binary SHA-256:

`bf11f37b125b4a6b59ba3c8501b323cc98b9556bad55ed1af3b8fa477092efca`

It was copied read-only to this lane's proof root. The native E2Es overlay
only module registration destinations with loopback fixtures.

## Completed gates

- `go test -json -race -count=20 -timeout=4m ./...`: exit 0,
  **3040 passing events, 152 distinct tests/subtests**, no failures or
  skipped tests.
- `gofmt -l .`: exit 0 **and zero output bytes**; `go vet ./...`: exit 0.
- Linux C-shared ZIP/checksum builder and two concurrent native ABI sessions:
  exit 0, five calls/session in one process, no Bun/network callbacks.
- Darwin arm64 and Windows amd64 CGO-disabled vet/build: exit 0.
  These do not establish native execution on those platforms.
- Four compiling negative overlays fail the intended test:
  substitute a PAT for the MCP business JWT; skip authenticated selected
  user verification; ignore bundle SHA; expose mutable source ownership.
- The full earlier controller/auth/quota suite remains included: no replay,
  ambiguous paid exchange, cross-instance journals and usage barriers.

Logs/exit codes: `final-results.json`, `race20-final.log`, `*-final.log`,
`negative-results.json`, `negative-*.log`.

## Actual native-host tool proof

The extended `e2e.py` passed, exit 0, using actual SDK `tool.call` invocations
through the direct local-management CLI. The host resolves the saved
selected account/current generation and marks the context local-owner.
This is an executing native library, not a mocked handler registration.

- Account status reports the selected opaque ID and generation 2 without
  token material.
- Official MCP usage sends the selected ZCode session JWT and newly
  published business JWT, with explicit Personal target, no PAT/API-key
  header. The resulting quota is Aside/informational.
- Native history inspection recognizes opaque thinking fields.
  Conversion preserves original CRLF bytes, signature text and integer
  `9007199254740993` inside the validated bundle source.
- After reset uncertainty resolution, an informational `zcode_usage` tool
  observes quota. A separate inference is still blocked. Only a real
  matching-revision `provider.usage` call clears the durable host barrier.
- Refresh still publishes business/PAT/expiry/generation atomically,
  uncertain exchanges remain fenced, native ticket reuse and settlement
  idempotence remain intact, and inference is never replayed.

`auth_e2e.py` also passed against the same binary: init/poll/finish,
same-account Start reauthorization, dynamic models and replacement-session
inference.

Unit fixtures additionally cover Team header pairing, rejection of missing
paid material and mismatched authenticated user, caller-supplied `_magpie`
arguments, ticket/card suppression from execution tools, malformed quota,
unknown capacity, duplicate/bounded JSON and immutable history copies.

## Reader/converter scope

The standalone `history` package actually parses native JSONL records and
legacy meta/messages/checkpoint/file-change snapshots. It produces a
structural summary plus owned original bytes and record JSON (native line
whitespace is retained in the original source). Body/content references
remain explicit and are not fetched.

The module-owned bundle is a checked source-preserving container with an
exact restore operation. It is not an official ZCode file format, signature
verification, live execution admission or a lossy cross-agent transcript
converter. Unknown native fields survive because complete source bytes are
retained; unsupported target formats fail. No SessionMigration descriptor
is registered before an actual SDK adapter is implemented.

## Honest remaining coverage

- Full gateway HTTP/MCP `_magpie` delegation and caller-policy behavior are
  supplied by the existing host implementation/tests; this repository's
  actual-native proof uses the local management path with selected scope.
- Tools added here are read-only. Paid reset/claim/settlement writes remain
  their existing declared actions; no new financial retry path was added.
- Official remote MCP JSON-RPC execution/dynamic tool registration,
  selected-account UI/auth broker, registered session adapters, live native
  resume and reviewed archived SQLite import remain separate features.
- No live vendor OAuth/MCP/history parity or macOS/Windows native runtime
  acceptance is inferred from synthetic fixtures or compilation gates.
