# First OMP extraction

## Current official harness compatibility — 2026-11-15

1. Preserve donor checkout/index/untracked state. Fetch current public refs
   into isolated ignored bare stores, compare exact trees with the locked
   revisions, and record schema, storage and resume watchpoints.
2. Add behavioral regressions before implementation: OMP developer entries
   remain branch parents without becoming user turns; encrypted redacted
   thinking remains private; Codex rollback removes whole user turns, tools
   and images while retaining compaction context.
3. `source_preview_decode.go`, `source_preview_parts.go`, `codex_history.go`:
   admit the source-defined developer/private record shapes using the
   existing exclusion boundary. Keep unknown roles/blocks explicit errors.
4. `native_read.go`: apply source-defined `thread_rolled_back.num_turns`
   boundaries to supported portable history, validate rollback counts and
   compaction metadata alignment, and keep tool failure status scoped to
   the retained history. Preserve the native OMP→Codex serializer.
5. Run old-code negatives, full module race tests, focused race20, packaging,
   vet/typecheck/format and one full native build only on usbox, isolated
   HOME/XDG, pinned offline Go1.26.8 caches and GOMAXPROCS2/-p2. Record exact
   physical source inventories separately from commit bases.
6. Update source/package provenance and current watch records. Replay the
   ordinary installed package against the leader's final host pin when
   supplied; historical Codex0.160 acceptance remains historical.
7. Document the unresolved Codex lineage boundary with exact source fields,
   a synthetic ancestor/child pair, computed prefix bytes/digests and the
   existing v1 planner's authority limits. Lock refusal of suffix-only and
   unbound multi-document imports; implement no host acquisition behavior.

Ownership: this standalone repository only. The original
`magpie-extensions/modules/omp-sessions` is a read-only backup. Donor files stay
read-only.

1. `document.go`: define module-scoped `omp-conversation-v1`, ordered
   user content and tool history. Retain an optional original OMP journal and
   supplied asset bytes for lossless reverse conversion. Reject edits to a
   retained archive rather than silently ignoring changed conversation data.
2. `read.go`: extract the pure active-parent-chain, reset and portable-compaction
   selection from archived `carry_omp_read.go`. Resolve image blobs only from
   explicit request documents, without reading HOME, profiles or session roots.
3. `write.go`: extract OMP v3 journal construction from `carry_omp.go`. Produce
   bytes and planned blob documents; preserve the 256-byte title slot,
   timestamps, native shell call/result pairs, errors and image ordering.
   Generate UUIDs using the standard library, with dedicated version/variant
   tests. Unsupported portable items fail explicitly.
4. `convert.go`: validate bounded, explicit SDK `SessionRequest` routes and
   relative output paths. Return `SessionResult` only; the host's checked plans
   own every filesystem write, including image blobs.
5. `cmd/native/main.go`, `magpie-plugin.json`, `build.py`: register a native
   `session.convert` handler using public SDK imports and its native ABI
   plumbing. Build an installable platform distribution without C glue.
6. `*_test.go`, `testdata/`: synthetic fixtures for both directions, branch and
   compaction selection, roles/tool history, title slots, blob digests,
   malformed input, path rejection, cancellation and UUIDs. Run all compilation
   and tests only on usbox, with an isolated HOME and the immutable SDK sibling.
7. `README.md`: document schema, minimum API 1.1 / native ABI 1, explicit
   invocation and checked-plan application, donor semantics and remaining host
   adapters. No discovery, indexing, resume or live OMP commands in this slice.

The SDK's byte documents can carry blobs alongside a journal when the caller
selects a checked root containing both `sessions/` and `blobs/`. Asset inputs
must be supplied explicitly. No filesystem discovery or direct publication is
part of the codec.

## Round-trip review repairs

1. Add regressions for a valid 4 MiB journal whose archived document exceeds
   8 MiB, and for literal `blob:` strings outside attachment fields.
2. `convert.go`: distinguish the journal/asset budget from the larger archived
   document budget, bounded by host plans and public SDK wire frames. Reject
   outputs that the opposite route cannot admit.
3. `decode.go`: inspect OMP message image blocks and file-mention image fields
   across every branch. Keep text, tool arguments and metadata opaque.
4. `write.go`: bound fresh journal plus assets together. Preserve the existing
   serialized formats. Update README and run usbox regressions, race20 and vet.

## Ordinary source inputs

1. Expose the read-only, versioned `session.inputs.v1` action over the existing
   SDK ModuleRPC contract. Return only clean store-relative paths and SHA-256
   digests; retain filesystem authority in the host.
2. Share attachment-field traversal with decoding so planning does not treat
   text, tool arguments, metadata or inactive branches differently.
3. The session owner approves the selected native store, acquires bounded
   regular files without symlink parents, verifies digests, and snapshots
   identities, parent directories and bytes. Recheck those snapshots after
   conversion and immediately before ordinary application.
4. Keep existing explicit conversion requests compatible. Verify the actual
   native module through both checked preview/apply directions on usbox, with
   neither Bun nor vendor processes, plus attachment mutation regressions.

## Native ordinary carry route

Extract the archive's Claude journal schema into a pure `claude.go` serializer,
registered as `omp-to-claude` with the existing ordinary SessionDescriptor
fields. Use the selected host `cwd` and the default approved Claude project
root. Keep an unsupported/custom target root out of ordinary routing rather
than writing a session the target cannot discover.

Keep the original explicit interchange codec byte-compatible. A separate
projection mode retains selected tool-result images for the native route,
while signed/private reasoning and off-branch records stay outside portable
native context. Shell calls become native Bash history; other tool calls are
recorded as context and cannot become executable foreign tools. Verify the
actual native module through OMP discovery, preview, CarryOver, native Claude
discovery and safe resume text, without starting either vendor agent.

## Complete recovered native route set

The archive offered Claude→OMP, Codex→OMP, OMP→Claude and OMP→Codex;
upstream still owns Claude↔Codex. Keep that composition rather than replacing
it with private interchange targets.

1. `native_read.go`: extract Claude active-parent replay and Codex
   response-item/compaction replay into pure bounded byte readers. Preserve
   user/tool-result image order, shell IDs/results/errors and full text.
   Signed reasoning and injected agent context follow the recovered portable
   migration boundary. Unknown nonportable formats fail explicitly.
2. `native_omp.go`: encode either reader's selected context as a real OMP v3
   journal. Use host-supplied canonical cwd/home/temp for the recovered bucket
   algorithm. Add an internal inline-image encoder mode; ordinary migrations
   return one create-only journal, avoiding unsafe global-blob rewrites.
   Keep explicit document-to-OMP encoding and its blob behavior unchanged.
3. `codex.go`: emit the recovered native Codex rollout schema, with native
   exec_command history and host-supplied target model provider. Preserve
   images using native content blocks and context where no executable
   counterpart exists.
4. Register the three missing ordinary descriptors, each with an exact default
   target prefix. Extend only the reserved session-owner adapter files for
   authoritative target options, packed Codex document reads and root guards.
5. Add real native Find/preview/CarryOver/reader/resume tests for all four OMP
   routes, image/tool history and unchanged sources. Test bodies and format
   boundaries before implementation; run verification only on usbox.

## Current-base distribution closure

This historical checkpoint kept API 1.3 as the default and Source 1.6 as a
developer alternative. The unified distribution below supersedes that policy;
its original bytes, dirty/index patches and untracked work are preserved in the
verification packet.

1. `test_build_identity.py`: first reproduce absent actual-build provenance,
   stale default manifest selection and SDK source mutation during compilation
   using synthetic compiler/filesystem fixtures on usbox.
2. `provenance.py`: reuse the existing packaging helper to fingerprint the
   selected Go source and embedded inputs, dependency module files, toolchain,
   actual native library and active manifest. Keep historical donors distinct.
3. `cmd/native/main.go`, `build.py`, `build_source_preview.py`: derive packaging
   from the actual typed factory and reject changing build inputs. Preserve
   existing manifests, capabilities, minimum versions and converter bytes.
4. `verify_host.py`, README and source documentation: report the installed
   contract rather than a hardcoded label; document one default installation
   and the remaining delegated-source boundary.
5. Freeze module and committed shared-SDK bytes into an isolated usbox packet.
   Preserve old-code assertion failures, then run scoped packaging checks,
   module race/vet, new provenance checks under race20 where applicable and
   actual relocated native loading. Reuse unchanged long Codex-client evidence;
   do not call it new current-host acceptance.

## Unified Source and migration distribution

1. `source_preview_test.go`, packaging tests: first require Source 1.6 in the
   ordinary factory/package, all six unchanged migration routes and exact
   compiled-input provenance. Preserve named old-code assertion failures.
2. `manifest.go`, `magpie-plugin.json`, `source_preview*.go`, `cmd/`, `build*.py`:
   remove the tagged partial distribution. Register the reader, immutable read
   veto and existing migration handlers in one ordinary API 1.6 factory.
   Keep the old builder name only as a forwarding compatibility command.
3. Source reader/tests: support bounded explicitly supplied related OMP JSONL
   journals, validate descendant paths/IDs and aggregate recorded usage without
   flattening child transcripts or borrowing filesystem authority. The current
   host input planner accepts exact path/digest pairs only; automatic recursive
   artifact discovery cannot be claimed from that interface.
4. `source-lock.json`, README, source/provenance docs: declare one full native
   package and explicit host ownership. Record the precise checked-input
   boundary, source snapshot and unchanged historical client evidence.
5. Isolated usbox verification: module race suite, focused race20, packaging,
   vet/format and relocated native loading; actual ordinary host
   List/read/resume/permissions and CarryOver→Codex against this one package.
   Check source immutability and private output modes. Do not edit or commit
   shared host source, change native ownership checks or run a duplicate broad.
