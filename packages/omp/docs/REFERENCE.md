# magpie-sdk-plugin-session-omp

An unpublished native Magpie SDK module for OMP history reading and session conversion.
It imports public SDK packages only. The native SDK selects the already
host-locked `golang.org/x/sys`; this module records its existing checksums.
The workspace sibling `magpie` checkout supplies the SDK through
`src/go.mod`'s `../../../../magpie` development replacement; a release will
need a real SDK revision. Current package commands are in [README.md](../README.md).

Install one default package built by `scripts/build.py`. It contains all six conversion
routes and the Source 1.9 reader; no second plugin is required for OMP↔Claude,
OMP↔Codex, delegated history or lossless interchange. The installed native package needs neither this checkout nor Go,
Python, Bun/npm or the SDK source.

The earlier SDK1.9 bounded host replay uses physical tree
`36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, based on
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`,
and the exact supplied binary pinned in `source-lock.json` `hostAcceptance`.
It covers all four ordinary native carry directions, descendant Source,
the Source owner and actual CLI/package paths. This is a physical candidate;
the leader's merge/review and host broad completion remain separate. Earlier
physical/committed host replays, domain suites and vendor-client evidence
retain their original identities in [VERIFICATION.md](history/VERIFICATION.md).

Current official source comparison and compatibility changes are recorded in
[SOURCES.md](SOURCES.md): OMP 18.20.3 developer/private records, Codex rollback
and checkpoint metadata validation. SDK1.10 adds an opt-in checked
`codex-rollout-lineage` carrier for sparse `history_base`. The host acquires
original ancestor prefixes; this module alone materializes and imports them.
Missing capability or captured ancestry still fails explicitly.
[CODEX-LINEAGE.md](CODEX-LINEAGE.md) gives the exact fields, synthetic prefix
and unchanged attachment-planner boundary. New physical proof snapshots and
remaining integration limits are recorded in [VERIFICATION.md](history/VERIFICATION.md).
These source checks do not extend the
historical installed Codex0.160.0 acceptance.
The [2026-11-16 watch](SOURCE-WATCH-20261116.md) found no portable schema
change in the new donor revisions. Its native shell round-trip regression
identified an existing completion bug: assistant text immediately before a
tool call now shares that OMP assistant message, with `toolUse`, rather than
creating a separate completed answer. The final answer retains `stop`.
Foreign-tool reverse projection remains the labelled-context boundary below.

The [JSONL Source reader](SOURCE-PREVIEW.md) uses the public SDK 1.9 contract
in the ordinary build. It exposes summaries, usage, selected transcripts and
the host's native resume. Installation alone leaves the builtin reader in
charge: delegated reading requires read consent, an allowed module record and
an explicit checked host ownership binding.

The reader declares bounded automatic collection of subagent/advisor JSONL
under the selected journal's stem directory, excluding `btw-history`. The host
captures each file and its tree membership under the same checked store and
module lease. Child usage and header identities enter the ordinary summary
and gateway deduplication; only the selected main branch enters its transcript.
Images use the existing Source display markers;
original binary images remain available through migration/interchange.
These bounds grant no access to sibling stores or live data.

## Supported first slice

| Descriptor | From | To |
| --- | --- | --- |
| `omp-to-document` | `omp`, or its explicit alias `omp-v3` | `omp-conversation-v1` |
| `document-to-omp` | `omp-conversation-v1` | `omp`, or `omp-v3` |
| `omp-to-claude` | `omp`, or explicit `omp-v3` | native `claude` journal |
| `claude-to-omp` | native `claude` journal | native `omp` journal |
| `omp-to-codex` | `omp`, or explicit `omp-v3` | native `codex` rollout |
| `codex-to-omp` | native `codex` rollout | native `omp` journal |

`omp-conversation-v1` is this module's interchange format, not a global SDK
conversation standard. The module reads only bytes explicitly supplied in
`SessionRequest.Files` and the checked optional `SessionRequest.Related`
carrier. It performs no filesystem discovery, publication,
profile selection, indexing or resume command execution.

Minimum contracts for this package are **SDK API 1.10**, **session-migration**,
**session-related-inputs**, **module-rpc**, **session-source**, and **native ABI 1**.
Source retains its API1.9 descendant contract. The earlier codec-only extraction used
SDK 1.1; the ordinary attachment-aware route requires the new host adapter.
The distribution is a Go C-shared library registered through
`pkg/extension/native`, with no author-written C transport or `plugin.Open`.
Its manifest names the exact platform for which it was built.
The builder includes [SOURCES.md](SOURCES.md) and `source-lock.json`, with
package metadata matching the active distribution. Provenance records and
declared SDK floors are separate from the historical acceptance checkpoints.
`build-provenance.json` additionally records the actual compiled module/SDK
input hashes, toolchain, native library hash and generated manifest hash.
Historical donor pins do not stand in for the SDK used by a new build.
Builders require a new or empty output directory and reject changing source
inputs rather than publishing a mixed-source manifest.
`scripts/build.py --output OUTPUT --sdk SDK_SOURCE` selects an optional build-time SDK source.
The temporary module override retains the same locked dependency checksums.
The old `scripts/build_source_preview.py` command forwards to that same builder; it
produces the identical full package and has no separate tagged entry point.

The optional source-input planner also uses **module-rpc**. Existing explicit
`session.convert` requests remain compatible; ordinary attachment acquisition
requires the host's `session.inputs.v1` adapter.

## Source-input planning contract

The manifest declares the read-only ModuleRPC action `session.inputs.v1`.
Its input is a `SessionRequest` containing the one journal selected by the
host, with a path relative to the approved native store. Its result is:

```json
{
  "version": 1,
  "files": [
    {"path": "blobs/<lowercase-sha256>", "sha256": "<lowercase-sha256>"}
  ]
}
```

The planner opens no files. It deduplicates and sorts the references from
actual image/file-mention fields across all branches. Ordinary text and opaque
metadata remain untouched. Invalid blob references fail before acquisition.
The host chooses the input root, validates this version and each relative
path, limits the aggregate input budget, checks the digest and captures file
and parent-directory identities. No other profile or default store is searched
for a missing blob. Conversion receives the journal and acquired blob bytes
together, under one module generation. The ordinary carry path rechecks source
snapshots after conversion and before applying its output.
The plan result is limited to 64 KiB and 127 attachment references in addition
to the journal. Root requests remain subject to the ordinary SDK frame limit;
host callbacks retain their smaller independent budget.

This action plans inputs only. The native route below reuses the host's
existing OMP/Claude discovery and safe resume command. The module-owned
conversation document is still an explicit interchange format.
Explicit byte-only callers remain responsible for supplying their own source
authority. The native session adapter retains original captures in its
context-aware action guard until apply. It does not lock another application's
running session across validation and publication.

## Checked Codex lineage

The optional API1.10 `related` carrier has `version: 1`,
`kind: "codex-rollout-lineage"`, selected-child provenance and ordered
`ancestors: [{body, source}]`. `Files[0].Body` remains the selected original
decoded JSONL. Every ancestor body is its exact original decoded prefix,
including its own header and wrapper ordinals. Provenance binds immutable
rollout ID, host-generated managed relative path, raw representation
size/digest, full decoded size/digest, decoded prefix range/digest and exclusive
ordinal range. All counts/ranges are canonical decimal uint64 strings.

The host resolves only inside the selected native Codex home, including
managed current/archived plain or packed journals. It owns read consent,
bounded acquisition, original file/directory snapshots, installed package and
config identity, action revision and revalidation under the apply caller's
context immediately before publication. Paths in the carrier grant no
filesystem authority. `session.inputs.v1` keeps its existing whole-file
attachment schema and root.

`codex_lineage.go` checks ordered physical base linkage and exact byte/ordinal
boundaries, then normalizes in memory for the existing Codex importer. It
retains selected-child metadata, removes sparse-only pointers, omits ancestor
headers and renumbers wrapper ordinals. The existing compaction, rollback,
tool/result, image and incomplete-call rules then apply. At most 32 segments
fit the SDK's independent 16-MiB raw/decoded/supplied budgets and this module's
8-MiB native history budget. Unknown or incomplete lineage fails explicitly.
Capture digests fence later mutation; Codex stores no historical fork digest.

## Ordinary native routes

`omp-to-claude` declares `from: omp`, `to: claude` and
`outputDir: .claude/projects`. The host offers it from an ordinary discovered
OMP session when Claude is available and its selected store is the default
HOME-relative store or a configured native Claude root. The host binds this
conventional descriptor prefix to the current `CLAUDE_CONFIG_DIR`, retaining
only relative output paths from the module.

The host supplies the selected session's `options.cwd`, acquires every
declared blob from that source's store and applies a create-only output plan.
The module returns `<Claude-project-key>/<new-uuid>.jsonl`. It uses the
archive's native Claude schema, UUID parent chains, synthetic assistant
messages with zero usage and UTF-16 project keys. Long hashed project keys
remain explicitly unsupported. The host returns its existing
`claude --resume <id>` command and does not run the agent.

This native migration carries the selected portable context, not the
lossless OMP archive. User images and selected tool-result images retain
their bytes. Shell calls become native Bash history with matching results,
errors and working directory. Other tool calls/results become context and
cannot expose a foreign executable tool. Parallel assistant tool fragments
retain one message identity; unsupported interleaved incomplete native tool
groups and assistant image messages fail. Private/signed thinking, injected
system context and off-branch history stay outside the portable native
conversation. Use the explicit interchange document for a lossless archive.

`claude-to-omp` and `codex-to-omp` declare `.omp/agent/sessions`;
`omp-to-codex` declares `.codex/sessions`. The four routes restore the
archive's complete OMP composition. Claude↔Codex remains upstream-owned.

For reverse imports the host supplies `cwd`, `canonicalCwd`, canonical `home`
and canonical `temp`. The module follows the recovered OMP bucket algorithm,
with temp preceding home. It writes a real v3 journal with a new UUID, parent
chain, title slot and synthetic zero-usage assistant records. Images of every
admitted size stay inline in native OMP image blocks. Only one new journal is
published, so existing global blobs are neither overwritten nor granted new
write authority. Explicit document-to-OMP retains its original blob behavior.

Claude input follows the selected parent chain and last stored UUID revision,
excluding sidechains, injected context and signed/private thinking.
Codex input follows response items and compaction replacement/summary replay;
UI event duplicates and encrypted vendor reasoning are not copied.
Opaque tool outputs remain text, shell calls retain native history, and tools
without a native counterpart remain labelled context.
Codex rollback drops complete user turns and their tools/images while
retaining non-turn compaction context. Malformed rollback counts and
misaligned replacement-history metadata are refused. A sparse rollout that
names `history_base` requires the checked API1.10 carrier; importing only its
child suffix remains explicitly refused.

Codex output uses the host-selected `provider` from the target config/profile.
The native paginated rollout contains contiguous ordinals, model-facing
response items and durable `item_completed` UI items. Historical foreign tools
keep native call/result identities; they are not registered as executable
dynamic tools. Result images stay inside structured tool output. Tool calls,
results and compaction are never converted into extra visible user/assistant
prose. Visible narration becomes reasoning; signed/private thinking is omitted.
Recorded `toolUse`/`stop` distinguish commentary/final answers and unfinished
turns remain interrupted. Unknown exit codes/durations are not invented.
The ordinary host payload remains `cwd`/`provider`; it sends no guessed model.
An explicit caller may supply `model`, enabling a native `turn_context`.
See [Codex export verification](CODEX-EXPORT.md) for the actual installed
Codex acceptance runner, compatibility boundaries and evidence.
This remains portable history migration, not a lossless cross-vendor archive.

Packed Codex input is decoded by the session owner from its captured bytes,
with both compressed and decoded budgets and source snapshots. The module
still receives logical JSONL bytes and opens no files.

The host binds `.codex/sessions` to the selected `CODEX_HOME`, and
`.omp/agent/sessions` to OMP's selected default/config root, absolute agent
directory override, or named profile. OMP's XDG selection follows the
recovered application behavior: named profiles use their own migrated XDG
directory, rather than the presence of the base XDG app directory. Native
Find/read/resume sees these stores; named OMP resumes include `--profile`.
The module receives no arbitrary filesystem root option.
An ambiguous relative default agent override is refused; named profiles do
not inherit that default override.

When Pi and OMP share a store, the host recognizes only the recovered
256-byte v1 OMP title slot while assigning native ownership. Genuine Pi
histories keep their Pi owner. Unknown or unreadable shared prefixes are not
claimed by OMP, and each owner's independent read consent still applies.
This also covers Pi's separate flat session directory. Legacy OMP ownership
outside a shared store keeps its existing behavior.

Binding and preview create no target directories. The host snapshots the
selected destination, directory identities, permissions, missing components
and Codex provider config, then rechecks before apply. A retargeted symlink,
replaced directory or changed target config invalidates the plan. Publication
remains create-only.

Ordinary repeated migrations use a bounded host-owned in-memory receipt.
Reuse checks the source journal, each declared attachment, target selection,
published files and parent directories, module package and config generation.
Changed files are preserved and a fresh migration is created; invalid input or
revoked converter authority fails. This is not persistent reuse across host
restarts or a transaction against another running application.

## Conversation document

The JSON document contains `schema: 1`, `id`, `cwd`, `title`, `timestamp`,
ordered `items`, and an optional `ompArchive`.

- `items` uses the extracted carry vocabulary: `user`, `assistant`, `call`,
  `result`. User content preserves ordered text/image blocks. Calls retain
  IDs, names, arguments, commands and working directories; results retain
  errors and full output without the donor's 64 KiB truncation.
- An OMP import always includes `ompArchive`: original journal bytes, SHA-256,
  explicit asset bytes and a hash of the portable projection. JSON byte fields
  use base64, as Go's SDK DTOs do.
- The portable projection follows the donor's selected parent chain, active
  leaf, reset boundaries and portable compaction summaries. It omits private
  reasoning, injected/system/developer messages and unfinished tool calls.
  **The archive retains those records, provider fields, tool-result media,
  off-branch history, title records and unknown metadata byte-for-byte.**
  An imported document therefore remains sensitive session material.
- Reverse conversion restores an intact archive exactly. An edited archive or
  a portable projection that disagrees with it is rejected. It is not silently
  reconstructed from a reduced transcript.
- A newly authored document without `ompArchive` builds a fresh v3 journal.
  Unsupported roles/content, malformed arguments, unmatched results and
  unfinished calls fail explicitly. Generic tool names stay native tool calls.
  No tool is executed by conversion.

Fresh journals have a 256-byte blank title slot, a v3 session header, a linked
message branch, matching timestamps and synthetic assistant usage/model
metadata. Caller-supplied UUIDv4/v7 IDs are accepted. An omitted ID generates a
standard-library UUIDv7; entry IDs are UUIDv4. For repeatable CLI output paths,
provide the document's `id` and `timestamp` explicitly.

## Explicit inputs and checked outputs

The module implements `session.convert`. The public request shape is:

```json
{
  "id": "omp-to-document",
  "from": "omp",
  "to": "omp-conversation-v1",
  "files": [
    {"path": "sessions/source.jsonl", "format": "omp", "body": "<base64 journal>"},
    {"path": "blobs/<sha256>", "format": "omp", "body": "<base64 blob bytes>"}
  ],
  "options": {"output": "conversation.json"}
}
```

Supply one journal and each referenced blob as `blobs/<lowercase-sha256>`.
Every supplied digest is verified. Blob references in OMP image blocks directly
under user/assistant/tool-result `message.content`, and in
`message.files[].image.data` for role `fileMention`, must resolve, including off-branch
records. Text, titles, tool arguments and opaque metadata are not interpreted
as attachment references. The module never searches another profile or HOME
as a fallback. Additional subagent journals require a separate explicit
conversion instead of being silently dropped.

For the reverse descriptor, supply one `omp-conversation-v1` document and
`options.output` as a session bucket directory, for example
`sessions/--work-omp--`. The result names
`<bucket>/<timestamp>_<session-id>.jsonl` and any `blobs/<sha256>` files.
All output documents use the requested `to` format, including binary assets,
as the host's SessionMigration contract requires.

Choose one checked output root covering **both `sessions/` and `blobs/`**.
The module never writes those paths. Magpie's host action layer previews,
checks, backs up and atomically publishes the returned files. The explicit
CLI requires each output path to be approved:

```text
magpie plugin sdk add /absolute/path/to/distribution
magpie plugin sdk session omp-sessions '<SessionRequest JSON>' \
  --root /absolute/checked/root --path conversation.json
magpie plugin sdk session omp-sessions '<SessionRequest JSON>' \
  --root /absolute/checked/root --path conversation.json \
  --apply --backup-dir /absolute/backup/directory
```

Preview writes no output files. Reverse conversion must approve each returned
blob path as well as the journal path. Reusing a blob still returns its bytes
in the checked plan, so application refreshes its write-grace period before a
journal refers to it. Journal and blob inputs and outputs together are bounded
to 8 MiB. Archived interchange documents can grow beyond that because they keep
both the journal and its projection, so their input and output limit is the
host's 16 MiB checked-plan payload budget. Both routes check the public SDK's
frame limit with envelope headroom and the 128-file limit. A conversion rejects
output that the opposite route cannot admit.

The ordinary session adapter protects the journal and every acquired blob
after conversion and before execution. A create-only native target route
that externalized output images into OMP's global store would need an explicit
checked reuse policy. The native reverse routes embed image bytes, avoiding
that collision while retaining valid OMP records.

## Build and verification

Run builds and tests **only on usbox**, with a temporary HOME/XDG,
Go 1.26.8 from pinned `GOROOT`, `GOTOOLCHAIN=local`, and pinned
`GOPATH`, `GOMODCACHE`, `GOCACHE`.
Verification uses an immutable SDK sibling and `GOPROXY=off`, `GOSUMDB=off`.
Native builds require cgo and the target platform's C toolchain.
The earlier committed-host replay is pinned to `46756a4` / tree `db40ddbf`
in [VERIFICATION.md](history/VERIFICATION.md). It includes ordinary delegated Source,
descendant accounting, native CarryOver and checked CLI routes. Recorded
Codex-client acceptance remains a separate historical checkpoint.

```text
(cd src && go test -mod=readonly -race ./... -count=20)
(cd src && go vet -mod=readonly ./...)
python3 scripts/build.py --output /absolute/output/distribution
python3 tests/verify_host.py /absolute/fixture/magpie /absolute/output/distribution
```

`tests/verify_host.py` creates its own temporary HOME and output root, sets Bun to a
nonexistent path, and uses only synthetic fixtures. It exercises real native
installation and CLI checked-plan preview/apply in both directions, rejected
asset paths, blob reuse grace, lossless restoration and private output modes.
No live OMP installation, session, account, app command or vendor API is used.
Linux native proof does not establish execution on macOS, Windows or FreeBSD.

The extraction source is read-only
`../magpie-devzeus-archive/internal/sessions/carry_omp{,_read}.go`, with its
canonical carry helpers and OMP tests. Fixtures here are newly constructed
synthetic data and a tiny image, not recovered sessions. The file-level plan is
in [PLAN.md](history/PLAN.md). The OMP-to-Claude serializer also follows the archived
`carry_convert.go` Claude schema, with separate pure-byte publication.
The native readers and Codex serializer follow the same recovered carry code.
Destination binding and checked migration reuse remain host-owned behavior;
the external module owns only format conversion and attachment references.
Current attachment and ordinary-route evidence is in [VERIFICATION.md](history/VERIFICATION.md).
