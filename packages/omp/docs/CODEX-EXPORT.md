# Native OMP → Codex export

The old serializer fabricated assistant `Recorded tool … call` messages,
user tool-result/attachment turns and user compaction turns. Its output
parsed successfully but the installed Codex displayed three completed turns
instead of one, lost tool cards and promoted visible narration to an answer.
The repair keeps native history and its UI projection separate.

## Format and authority

`src/codex_history.go` selects the active OMP parent chain, latest reset and
portable compaction boundary. It preserves visible narration as reasoning,
tool call/result order, selected images and recorded completion phases.
Private/signed thinking and abandoned branches do not cross vendors.
Unsupported assistant blocks and message roles, duplicate call identities
and orphan outputs fail explicitly.

`codex.go` writes native paginated Codex JSONL: contiguous `ordinal`,
`task_started`, typed durable `item_completed` and `task_complete` or
`turn_aborted`. Model-facing `response_item` call/output pairs remain native.
Foreign calls are historical `DynamicToolCall` items, without a
`session_meta.dynamic_tools` registration. Known shell calls normalize to
`exec_command`; their command display uses a `/bin/sh -lc` envelope, not a
claim about the original process's actual executable. Original shell text,
working directory, result body and recorded success/failure survive.
Unknown exit codes and durations stay absent/null.

Tool-result images use native structured `function_call_output` arrays and
native dynamic-result image items. No image is invented as a later user
message. The reverse Codex decoder correlates durable native item IDs to
restore result failure status without changing result text.

Native Codex import also retains assistant message phase and recorded
completion/abort boundaries while rebuilding OMP. The encoder groups the
ordered commentary and calls into nonterminal assistant messages, including
text before and after a call. It groups final text through its recorded
completion boundary rather than treating each text fragment as a completed
answer. This transit metadata does not change the archived conversation
projection or expose private thinking. No command is executed during conversion.

Compaction uses Codex's own user-role summary framing inside replacement
history, without a visible user item. It never promotes a conversation
summary to developer/system authority. A branch summary appends context
rather than discarding active history. Missing tool results remain missing;
unfinished tools preserve their order and the owning turn is interrupted.
No fabricated tool outputs are emitted.

The export repair originally shipped at SDK API 1.3. The unified default
package now declares API 1.10 and includes Source and all six routes in
version 0.1.0. Current source compatibility is recorded in [SOURCES.md](SOURCES.md):
developer/private OMP records, Codex rollback, compaction metadata alignment
and checked module-owned sparse-lineage materialization with explicit refusal
when captured ancestors are absent. The historical acceptance below is
unchanged. Legacy host
options are exactly `cwd` and `provider`. Optional explicit `model` enables
`turn_context`; ordinary host migration omits it and Codex chooses its own
configured model when resuming.

An eventual generic API 1.6 option negotiation should be per
`SessionDescriptor`, for example `optionMetadata: ["targetModel"]`.
An absent declaration must preserve the legacy payload. The host would
obtain any opted-in model from the same checked config/profile snapshot as
the provider and recheck it before apply. This is a proposal, not an
implemented host feature or a plugin-name/version exception.

## Historical repair source evidence

Read-only donor repositories:

- `Agents/codex-infinity` commit
  `9c474c076aa0910932cfd3c8735dbfbf8c7c9966`: protocol models, items,
  dynamic tool schemas and app-server protocol.
- `Proxy/reference/openai-codex` commit
  `7c2ce90716335c889a5076ded9a630459f9c9899`: rollout persistence policy,
  paginated history reducer and compaction framing.
- Magpie commit `831c3d74af9358fa8a45d77eed902fe7cd02a076`:
  sanitized `internal/sessions/testdata/carry/codex-tools.jsonl`
  records real Codex 0.162.1 `exec_command`, paginated ordinals,
  `CommandExecution` items and message phases. Its SHA-256 is
  `c2ca94642e17ac78278e51b07ec2ce0a8777f141e39fdc8b774ee45761b13dde`.

These sources are not a claim that their versions match the installed
binary. Actual acceptance below uses **codex-cli 0.160.0** on usbox,
including that binary's generated schema and its native app-server.

## Repeatable installed-client acceptance

`tests/verify_codex.py` requires an existing installed client, an exact reviewed
version and a separate network namespace without default routes. Missing
capabilities or another version fail preflight; nothing silently skips.
It creates its own HOME/CODEX_HOME and only reads the supplied exported
fixture. Its own app-server and loopback mock are stopped before exit.

Run on **usbox**, with the pinned Go toolchain/cache environment and offline
dependency resolution:

```sh
(cd src && OMP_CODEX_EXPORT_DIR=/tmp/omp-export \
  go test -run '^TestCodexExportKeepsNativeRolesAndToolHistory$' .)

export OMP_CODEX_PARENT_NETNS="$(readlink /proc/self/ns/net)"
unshare --user --map-root-user --net sh -c '
  /usr/sbin/ip link set lo up &&
  exec python3 tests/verify_codex.py \
    --codex /Users/maratkarimov/.bun/bin/codex \
    --expected-version 0.160.0 \
    --export /tmp/omp-export --output /tmp/omp-codex-proof \
    --continuation
'
```

Omit `--continuation` to prove read/resume without any model turn.
With it, exactly one loopback mock request returns a final answer.
The runner checks the actual reconstructed request, native tool/result
order and IDs, selected branch/context, images, phases and lack of tool
replay/foreign-tool registration.

An unloaded paginated rollout can return an **empty pre-resume**
`thread/read` view. This is not evidence that tools survived. The runner
requires both `thread/resume` and a subsequent materialized `thread/read`
to contain one completed turn, native compaction, user image, reasoning,
commentary/final answer and two native tool cards with result media/error.

For the ordinary path the export directory must contain the actual
`CarryOver` result, not another direct conversion. The scoped host-owner
harness installs the real native distribution, performs ordinary OMP
`Find`, checked no-write preview, ordinary `CarryOver`, checked reuse,
Codex `Find`/transcript/resume and confirms unchanged source bytes.
The installed-client runner then reads that exact retained rollout.

## Verification boundary

Historical red/green evidence and exact source inventories are retained at
`usbox:/tmp/magpie-omp-codex-repair.km6e63ZT/`. Final gate results and
inventory are in `completion.json`.

Final-source gates:

| Gate | Result |
| --- | --- |
| Entire default module, `-race -count=1 ./...` | 83 test/subtest passes; no failed or skipped tests |
| Export regressions and native route tests, `-race -count=20` | 440 test/subtest passes |
| Unmodified host ordinary native OMP↔Claude/OMP↔Codex routes, `-race -count=20` | 40 test passes; no skips |
| Real native module build plus ordinary Find/preview/CarryOver/reuse/transcript/resume | Exit 0; source unchanged, no model option invented |
| Installed Codex app-server on that ordinary result | Exit 0; pre-resume 0 turns, resume 1, materialized read 1, two native tool cards |
| One offline continuation in the final installed-client gate | Exactly one loopback mock request; original calls not replayed |
| Linux vet/build and Windows/Darwin no-cgo vet | Exit 0; cross-platform vet is not native runtime evidence |

`cmd/native` contains no Go test functions; the executable was built and its
actual C-shared module was loaded by the native host gates. The 20-fresh-process
ordinary checkpoint is also retained separately. All 10,340 committed host
files matched the `831c3d74a` archive; the added verification harness changes
no production source. Installed Codex's native executable SHA-256:
`12eb3e81114588aca3b7998f4f19e8997b056aca08e57a7ca7c8a3ec8c652aad`.

Executed causal negatives include the original serializer accepted by
Codex but rejected for extra user turns, reverse error-status loss,
summary authority promotion and branch-summary history replacement.
Unit/race coverage includes both OMP↔Codex and OMP↔Claude native routes,
images, shell/arbitrary tool IDs, errors, selected forks/reset/compaction,
missing output/unfinished turns and unknown/ambiguous input refusals.

This proves the scoped native format on the installed Linux app-server,
not an interactive TUI session, vendor model behavior, every past/future
Codex release or Windows/macOS native runtime. At that repair checkpoint,
Source 1.6 remained a separate preview. The current ordinary 0.1.0/SDK1.10
package includes Source; that later distribution and current source-derived
tests do not relabel this old client receipt. The Magpie host source and
live application were not changed by the repair.
