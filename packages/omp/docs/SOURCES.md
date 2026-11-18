# OMP module source provenance

Checked **2026-11-16** against the parent's fetched exact public refs using
read-only `git show`/`git diff`, without changing donor checkouts or running a
donor CLI. `source-lock.json` is the machine-readable companion. Local file references in the lock are relative to the package root; donor
paths remain relative to their original repositories. Relocation changes only
these local paths, not historical receipts or acceptance claims. Both files
are documentation and package provenance, never runtime authority, a credential
source or an instruction to install or update another module.

| Identity | Exact revision/version | Role |
| --- | --- | --- |
| Full distribution | `omp-sessions` 0.1.0, minimum SDK API 1.10, native ABI 1 | All six routes, module-owned Codex lineage materialization, unchanged input planning and Source1.9 descendant acquisition |
| Previous physical SDK1.9 candidate acceptance | Tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, based on `adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`, SDK 1.9 | Earlier bounded native carry, descendant Source, Source owner and CLI/package replay; exact binary/archive in `source-lock.json` `hostAcceptance`, no API1.10 or final merge/review/broad claim |
| Previous physical host/SDK acceptance | Tree `8fe78ab9df13d733d61462eacce19bff523c1e58`, based on `c11acbc2bebec1fb213ef562900d35188cdf55fe`, SDK 1.9 | Earlier Source/Codex CarryOver and CLI/package replay, preserved in `hostAcceptanceHistory` |
| Previous committed host/SDK acceptance | `46756a48c9c9f204a8369638dc63cbcb83d02031`, tree `db40ddbf8944b54f9f689d444644af4c3bc76881`, SDK 1.9 | Native/CLI/Source/Codex CarryOver and descendant replay; preserved in `hostAcceptanceHistory` and VERIFICATION.md |
| OMP target | `can1357/oh-my-pi` `90935366da0c368a437da4343ae748b03a2ffe97`, `packages/coding-agent` 18.20.3 | Native v3 journals, title slot, branch/reset/compaction, developer/private blocks and attachments |
| Codex protocol donor | `lee101/codex-infinity` `2ecc8a92fe18fd43552529856078999d3f6888c9` | Read-only comparison of protocol/item/tool schemas; workspace declares 0.159.3-infinity.1, not installed-client acceptance |
| Official Codex authority | `openai/codex` `aea4c6ee4b652d84b416731b11c2b8299d767209` | Response schemas, persistence, rollback, compaction and sparse lineage; workspace placeholder 0.0.0 is not a release, so lock version is null |
| Archived Magpie donor | `TransmuteLabs/magpie` `893f560cccdeb8fa2e98eadb60427990c10eedcc` | Native OMP/Claude carry extraction; no exact release version, recorded as null |
| Historical host/fixture donor | `TransmuteLabs/magpie` `831c3d74af9358fa8a45d77eed902fe7cd02a076`, SDK API 1.5 | Host source used by the recorded Codex repair and its sanitized fixture; not current aggregate acceptance |

There is one ordinary complete distribution. The repository lock describes
`src/magpie-plugin.json`. `scripts/build.py` copies this record and the lock, deriving the
packaged lock's `plugin` block from the registered typed factory manifest.
The historical preview builder name forwards to the same builder; no tagged
partial library or second installation exists. The manifest remains the runtime
authority; editing these provenance files cannot grant a capability.

Each package's `build-provenance.json` is separate from historical donors. It
records the actual Go-selected module/SDK inputs, including embedded files
and module definitions, their content fingerprint, Go version, target
platform and native library/manifest hashes. Source fingerprints are checked
before and after compilation. Machine-specific source locations are omitted.
This identifies the supplied build source; it does not turn a development
`v0.0.0` replacement into a downloadable SDK release or certify an untested
platform/client. Build into a new or empty directory; existing packages and
local files are never overwritten.

The minimum **1.10** is the declared compatibility floor. It is separate
from the SDK versions used by historical acceptance gates. See
[VERIFICATION.md](history/VERIFICATION.md), [CODEX-EXPORT.md](CODEX-EXPORT.md) and
[SOURCE-PREVIEW.md](SOURCE-PREVIEW.md) for those exact checkpoints and limits.
Packaging metadata tests do not repeat or extend native client acceptance.

## Current harness compatibility

The November 17 comparison checks OMP `18.22.0`
(`1bdbf6d242d99ca62bdf2feeff4519ff992f647d`), OpenAI Codex
`59ac39addbbcd3499319c805a916846ca8ac5fef`, and Codex Infinity
`2ecc8a92fe18fd43552529856078999d3f6888c9`. Codex's workspace
`0.0.0` is not a published release version. The exact comparison is recorded
in `latestCommittedDonorReview`; the original extraction pins stay in
`sources` and the earlier acceptance records remain unchanged.

OMP added 449 commits and changed 665 paths since the extraction pin.
The nine journal, compaction, message and storage owner objects are
byte-identical, and the persisted session version remains 3. The new
`agent_pause` concerns native execution; `loop-end-wire.ts` maps it to the
existing flagged `agent_end`. The additional `getGhidraDir` path does not
change session discovery. This package converts captured journals and does
not implement OMP's live agent loop.

OpenAI Codex added 93 commits and changed 827 paths. Nine persisted
declarations, including `ResponseItem`, `ContentItem`, `SessionMeta`,
`RolloutItem` and `CompactedItem`, are byte-identical. Attachment preparation
moved into `user-input`, while lazy `LineageWalk` changes native database
paging. This package continues to validate explicitly captured related-input
prefixes; it does not query the native history pager. No required portable
format change was identified. Codex Infinity's compared commit is unchanged.

The source-only audit is `sol-harness-refresh-20261117-01`, report SHA-256
`61591206301a5667cdf6f3bc8a1fd2f81206f2b414e384536c81f8752c74f383`.
Neither the comparison nor unchanged declarations prove a run with the
current installed harness. Earlier Codex `0.160.0` client evidence retains
its original scope.

### OMP 18.22.1 provider replay

The OMP `18.22.1` follow-up checks `can1357/oh-my-pi` at
`9cb7eaee6128ecf6b5b084700e342062218bee75`. Commit
`9bed2a4d8ffa6526ddd81d2dab0033d9266c2174` defines the model-facing
filter in `packages/agent/src/replay-policy.ts`. Native Codex and Claude
exports now omit assistant messages with `stopReason: "error"` and
`stopDetails.type` of `refusal` or `sensitive`. A tool result with
`details.__synthetic: true` and `details.source: "assistant_stop_error"`
uses the donor's generic not-executed text when its call survives in the
selected history. The export omits that placeholder when its call is absent.
Ordinary tool errors keep their recorded output. Source transcripts and
portable exports retain the original data, including the byte-exact archive.

Commit `56ba77ff8408f37b96d0eb2cbb1b77411b478ba7` stores `partial_answer`
in individual text blocks' versioned `textSignature`. The Codex `0.160.0`
phase enum lacks the value. Preserving the phase in native exports requires
target capability evidence and must keep turn completion independent of
the text phase. This replay change keeps the existing phase behavior.

The next seven commits through `7a53f0a2d36e160f9255d80626ed757f41280b76`
keep version `18.22.1` and change 41 paths. The session-directory delta
changes prompt-history SQLite write timing, search and shutdown.
The converter reads captured session journals and assets. The journal
serialization and replay policy are unchanged. `promptHistoryFollowUp`
records the source comparison.

### OMP 18.22.2 custom messages

The native replay update checks `can1357/oh-my-pi` version `18.22.2` at
`04a4c101ff5b56b6aa6647ffc7ca1019c1998759`, 41 commits after
`7a53f0a2d36e160f9255d80626ed757f41280b76`. Commit
`205dde09d756c0869da42e39fdc59351f485b678` persists the optional boolean
`excludeFromContext` on top-level `custom_message` rows. The donor's
`session-entries.ts`, `session-manager.ts`, `messages.ts` and
`packages/agent/src/compaction/messages.ts` define its persisted and
model-context meanings.

Native Codex and Claude exports carry user-attributed `skill-prompt` and
`collab-prompt` rows with their original text and images. The donor's
`collab/host.ts` queues a peer prompt as steering while busy. Codex keeps a
peer correction inside the open turn, and Claude retains its position
after the native tool results. A peer prompt after a completed turn has
its own turn because the persisted custom row carries no busy/idle marker.
Excluded rows, other extension context and agent-authored prompts stay
in the byte-exact archive. Native conversion acquires custom images from
the retained prompts. Archive conversion requires all original custom images.
Malformed flags, ambiguous block fields and unsupported prompt content
in the selected context stop native export. `display` affects the OMP interface
only. No OMP steering template or peer metadata enters the target history.

## Earlier source comparisons

The November 16 source-watch packet is
`usbox:/tmp/magpie-omp-watch-sol-20261116.rj2fdtm6`.
Its `source-comparison.json` SHA-256 is
`b05f731e79716f788430bd84cc72f738bdb01f0d1cf432a58463be87d7f8ebfb`.
It records exact old/new commits, trees and SHA-256 values for all locked
watch paths and adjacent canonical tool/message schemas. The OMP whole-tree
delta is 250 files; the official Codex delta is 206. Only two OMP locked watch
paths and Codex's `compact.rs` changed. None changes the plugin's portable
JSONL/item schema: removed seance metadata and session deletion remain
harness-owned, while compaction sampling/control changes remain runtime-owned.
Codex-infinity is unchanged at `2ecc8a92fe18fd43552529856078999d3f6888c9`,
workspace version `0.159.3-infinity.1`. No unrelated donor runtime code was
ported. [The watch record](SOURCE-WATCH-20261116.md) gives the path-level
classification and the separate pre-existing native shell completion fix.

The previous comparison and its implementation details follow as historical
context. Its pins remain in `sourceWatch.previous`; its older baseline pins
and exact receipt are retained in `sourceWatch.previousComparison`.

The source comparison packet is
`usbox:/tmp/magpie-omp-current-20261115.RKcxeFiQ`. Its
`source-comparison.json` records current/previous full SHAs, current trees,
declared versions, watched file SHA-256 values and donor checkout state.
Its SHA-256 is
`dfec4a56c7bd91884aea80f63eaf92701732f1e6638a7faa547407237db63467`.
The previous pins remain in `sourceWatch.previous` and in historical
acceptance below. OMP's original untracked `packages/coding-agent/undefined/`
and both original donor HEADs remain untouched. The earlier official Codex
pin was obtained in the isolated bare repository; an existing checkout is
not required for that tree comparison.

OMP 18.20.2 still defines `CURRENT_SESSION_VERSION = 3` and the same
fixed-width v1 title slot. `session-entries.ts`, `messages.ts` and
`docs/session.md` define persisted developer messages and private
`redactedThinking` assistant blocks. Source now accepts developer entries as
parents and omits their instructions, matching its existing system-context
boundary and the public DTO's conversation roles. Known encrypted redacted
thinking is omitted from Source and native Codex export; visible narration
retains its reasoning representation. Unknown blocks still fail explicitly.

OMP's default authoritative file remains bucketed JSONL; its session index
and SQLite-backed index storage do not replace that journal. Blob staging,
sidecars and GC do not change the `blob:sha256:` reference contract. The
current bucket/root functions live in `session-paths.ts` and utils
`dirs.ts`; host-supplied canonical paths remain the only path authority.
`continuationOf` is now distinct from a deliberate fork: OMP's own resume
logic follows the newest continuation. This plugin reads the selected
captured file and returns the host's native resume recipe; it does not
discover or follow another file itself.

Official Codex keeps response items flat on the wire, with optional
`id_allocation` and separate harness metadata. Compaction still stores
`replacement_history` as response items; parallel
`replacement_history_metadata`, when present, must contain exactly one
metadata object per item. The importer now checks that alignment instead of
accepting malformed checkpoints. The current persistence policy continues
to admit the existing export's durable native items and response pairs.
New optional fields and dynamic-tool namespaces require no registration of
historical foreign tools.

`core/src/context_manager/history.rs` and
`core/src/session/rollout_reconstruction.rs` define
`thread_rolled_back.num_turns`: remove complete user turns, retain any
non-turn prefix and never count the synthetic compaction summary as a user
turn. The importer now applies that boundary to supported portable history,
including the removed tool/image suffix. Removed tool failure status cannot
contaminate a later call. Absent, null, negative, fractional and overflowing
counts fail explicitly.

Captured Codex ancestor history uses the additive opt-in API1.10 capability
`session-related-inputs`. Current `thread-store/src/local/rollout_lineage.rs` resolves
`session_meta.history_base` through bounded ancestor prefixes, including
fork/revert and compressed segments. One supplied child JSONL document
cannot prove that prefix and remains explicitly refused. The host owns
native ID resolution, original raw/decoded capture provenance and source
snapshot revalidation. The module validates the finite ordered carrier and
alone materializes the history in memory for its existing Codex importer.
The plugin opens no ancestor
path, database or account. OMP descendants are usage artifacts and cannot
serve as this Codex ancestor adapter.

`CODEX-LINEAGE.md` records the exact `HistoryPosition` fields, physical versus
logical fork semantics and eight source paths. Its synthetic ancestor/child
pair fixes the inherited range at bytes `[0,756)`, exclusive ordinal 3, with a
separate prefix digest. Unchanged `session.inputs.v1` cannot resolve that
lineage: it knows exact relative whole-file paths/digests under the selected
Codex day directory, without ID lookup or ranges. Its capture/snapshot helpers
support the separate host-owned resolver without granting plugin filesystem
access. The versioned carrier contains exact decoded prefix bytes and metadata,
not plugin-directed lookup, paths or ranges to acquire. All count/range values
are canonical decimal uint64 strings. Original selected bytes remain separate;
raw capture identity, decoded ranges and prefix digests enter the action
revision and are checked again under the apply caller's context.

`current_harness_test.go` contains causal regressions for these boundaries.
The unchanged Codex export regressions still check native tools/results,
images, pairing, phases, unfinished turns and summary authority. Current
source-derived tests are not a new run of a current vendor client.

The previous SDK1.9 candidate replay used the immutable physical source and existing
binary identified by `hostAcceptance`, with initial-user-namespace native
ownership, private HOME/XDG and the unchanged network verifier. All 10,846
recorded paths were checked before/after execution. The full module race suite,
packaging, ordinary native/descendant regressions, three fresh Source-owner
processes and actual CLI passed. Final package inventories are recorded
separately after this provenance update. The earlier physical-host repin and
its 10,758 source paths remain historical, not a claim about the candidate.
The prior dependency correction recorded the SDK's already selected
`golang.org/x/sys v0.49.0`, using
the host's existing checksums. The alternate `--sdk` builder copies those
checksums into its temporary module file; it selects no new dependency version.
This replay covers actual host owners and synthetic histories, not a fresh
installed Codex/OMP vendor-client run. Historical client evidence stays below.
The API1.10 physical carrier snapshots and their exact tests are recorded
separately in `VERIFICATION.md`. The leader's final integrated host pin, broad
review and a current installed vendor client remain separate requirements.

The sanitized host fixture
`internal/sessions/testdata/carry/codex-tools.jsonl` at `831c3d74a` has SHA-256
`c2ca94642e17ac78278e51b07ec2ce0a8777f141e39fdc8b774ee45761b13dde`
and records client **0.162.1**. Historical installed-client acceptance in
`CODEX-EXPORT.md` used **codex-cli 0.160.0** on Linux. Neither number is the
module version, its SDK minimum or the Codex protocol donor's source version.
The locked fixture is evidence only; packaging does not copy a user history.

The lock's `watch` section maps donor paths to behaviors and local regressions:

- OMP journal/title/blob and config changes require branch/reset/compaction,
  attachment, bucket and Source permission checks.
- Codex protocol/item changes require native tool/result identity and media
  checks; rollout/reducer/compaction changes require ordinal, interruption and
  summary-authority checks. `verify_codex.py` is a separate explicit acceptance
  runner, not an automatic build step or a claim that every Codex version works.
- Archive carry changes require the ordinary native OMP/Claude route and
  lossless interchange regressions; private reasoning remains excluded from
  portable native context.
- Host SDK changes require checking full package metadata, explicit input
  grants and selected-source revocation. The Source 1.9 contract ships in the
  ordinary package; native ownership still requires an explicit checked bind.

No live vendor, credential, account or session is needed for these metadata
checks. Pure conversion still receives supplied bytes and returns checked
output plans; host-owned discovery, grants, publication and resume remain
outside provenance. Linux fixture evidence does not establish interactive TUI
behavior or native macOS/Windows acceptance.

When a donor revision, module version or SDK minimum changes, update the lock,
this record and the affected behavioral fixtures together.
