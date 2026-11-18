# Community-domain parity plan

The November16 bounded update compares community0.1.44 at exact
`b35d4b35b2446bd929896ddeb58f3cb299e9e2d1`. Its current source map and
committed-versus-reported-dirty reference comparison are in
[SOURCES.md](SOURCES.md) and `limits.donor044Comparison`.
The earlier0.1.41 checkpoints below remain historical.

| New donor behavior | Bounded SDK treatment |
| --- | --- |
| 0.1.42 snapshot catalog marker | Existing selected-account listing returns an error on unavailable discovery, preserving the host's last-good-list authority; regression forbids a snapshot/empty success. No Bun marker is added. |
| 0.1.43 adjacent signed thoughts | Every signature retains scoped origin validation; only the last thought in a run is attached intact to the next real assistant text/tool prompt. Standalone signed-only turns remain refused. |
| 0.1.44 image label and whitespace normalization | Common inline encoding recognizes supported image magic bytes without altering bytes; unknown signatures retain their declared supported MIME. Unmarked whitespace-only user blocks are omitted; meaningful whitespace is preserved, and unrepresentable cache boundaries fail explicitly. |
| 0.1.44 unsigned retry and long-refusal heuristic | Not adopted: ambiguous invalid_argument cannot authorize another dispatch, strip origin authority or turn approximate bytes into known context exhaustion. Small/large Chat/Messages controls retain one dispatch, original facts and uncertainty. No invented continuation prompt. |

Current monorepo provider comparison and new source4 verification are in
[PROVIDER-PARITY.md](PROVIDER-PARITY.md). This earlier matrix retains its
original donor/domain checkpoints; those results do not certify a later
host or package build.

Read-only donor checked 2026-11-15: fetched main
57d1cc6a9f49389d495ea9be8d986c70de7767bf, packages/devin 0.1.41.
This is source evidence, never a runtime package or install dependency.
The latest Devin-specific change is
`191d94bf6ed95ccc4bc62d77e1dd9201a17f5918`, package0.1.41, without
changing the donor checkout. The earlier0.1.40 pin remains historical.
Recovered authority is `ed84daf06cb9eb961a0d6cd7703e39b7b7f90654`;
relevant wire/model sources are unchanged from97a4883 and that checkout
has no configured remote. See `DOMAIN-COMPLETION-EVIDENCE.md` for this
completion and `GEMINI-SIGNATURE-EVIDENCE.md` for the earlier delta.

The history/image/price ports have independent ordinary installed-host
evidence, including actual token/cache ledger rows.
`DONOR-HOST-EVIDENCE.md` retains the initial timeout and subsequent
independent greens/reds. `DELIVERY-EVIDENCE.md` records actual installed
RED-to-GREEN correction of host caller projection and three Chat history
defects on scoped sourcec434, including unchanged Messages signature/stream/
cancel acceptance and non-SDK repair controls. Current committed46756a48
replay additionally covers API9 typed options, no-op/held read, real
transitions, genuine Gemini four cases and cold paid execution. The
original hot-activation paid phase remains a separate RED. No installed
plugin donor or runtime companion is used.

| Donor family | Existing own behavior | Remaining work |
| --- | --- | --- |
| keepalive | Standard0.8/API9 includes default-off `keepAlive`, paid confirmation, credential-free raw-byte templates, send-time cadence, conversation/replay/memory fences and fresh-account callbacks; peer tests cover selected authority, one-use acceptance, independent cancellation and cumulative measured ACU usage | Installed evidence is recorded separately in CACHE-KEEPALIVE-EVIDENCE.md. Transport/protocol/uncertain failures stop conservatively rather than reproducing the donor's initial502 retry. Vendor TTL/charges and original-client behavior remain live-only checks |
| cache-marks | Raw paired bytes retain marks; Chat message and ordinary content-block marks retain prompt boundaries; Messages preserves each system/user/assistant block, growing prefixes and deferred notices with real results; a final nested result mark becomes the same single result's end mark; explicit5m maps to native EPHEMERAL | ChatToolDefinition has no cache field; an intermediate mark within one result cannot become its end mark; PromptCacheOptions has no1h TTL field. Mixed Chat assistant content/signature/call boundaries without an ordered mapping remain explicit refusals |
| reasoning-history | Scoped installed mixed-alias and same-alias histories pass without host-created conflicts; conflicting source input remains refused before vendor; earlier Claude unsigned-history control passes | Preserve source-message ownership and strict conflict refusal when replaying the final upstream union |
| thinking-signature | Native receipt-bound signatures remain; Messages resolves native format/actual model from persisted caller/account-generation/selected-model receipts; scoped ordinary signed continuation passes | Imported/ambiguous origins, malformed/error/canceled streams never issue receipts; original Cognition/live-client parity is untested |
| Gemini0.1.41 thought-signature | Genuine donor-captured no-thinking tool reply has four SDK-peer Chat/Messages buffered/streamed RED-to-GREEN regressions, original donor7/7 tests and current installed Chat/Messages JSON/SSE roundtrips on committed467; exact signature/type/generation-model and real tool results survive; scoped bounded invisible cache plus existing origin receipts cover clients that omit signatures | Earlier frozen-host Messages400/zero dispatch and thinking-disabled probe remain historical in GEMINI-SIGNATURE-EVIDENCE.md. No unscoped global cache, guessed type, whitespace-colliding text key or retry. Restart/eviction/unknown invisible history stays unsigned; explicit foreign signed history fails before acceptance. Not a proven fix for live empty replies |
| tool-images | Independent installed seeLater image association and incomplete-image refusal pass without consuming unrelated user text; Messages inline blocks retain ordering | Messages result content cannot reorder text after images within one native result; unsupported shapes fail explicitly |
| split-calls | Module coalescing keeps real calls/results; scoped installed missing-result and split-cache refusals pass with zero dispatch, alongside same-alias/paired positives and non-SDK repair | Preserve historical host-manufactured-result/cache-merge REDs; final combined host requires separate replay |
| model families / effort / tiers / aliases | Account-discovered families and source-defined aliases advertise ordered efforts/defaults, Fast/Priority selections, conservative image capabilities and known common prices; explicit concrete UIDs remain listed/pinned. Chat effort and Messages output effort/thinking budget choose the exact or nearest variant, ties upward. Every virtual selection reads the current selected account's catalog before acceptance; no static variant inventory grants missing models | Source family names classify lookups only. Newly named families need discovery; unknown explicit IDs retain native UID semantics. SDK IDs forbid whitespace, so label-only families retain real concrete UIDs/display names. Ordinary candidate-tree replay passes; original/live account availability and final successor remain unverified |
| free / usage | Free families cover selectable effort/default variants and exclude paid unselectable Lightning siblings; quota exclusions include concrete/family/alias IDs. API9 emits exact present ACU amounts/limits, signed overage micros, units, day/week spans and plan-end resets; zero/absence and percentage routing remain distinct. API7 keeps its compatible percentage metadata; measured execution/cache usage stays separate | UsageResult has no top-level plan/expiry fields; plan end uses an informational window/reset. Native balance currency is unproven, so micros remain micros. Unknown catalog scope is deferred rather than guessed; whole-host replay is parent-owned |
| prices | Typed float32 field32 labels/units and per-UID SDK prices have unit coverage; installed SDK verifies rounding and distinct paid/Free/unknown models | Installed malformed/duplicate quote coverage can extend the existing unit refusals |
| schema / Gemini schema | Canonical tool schemas are validated and sent unchanged; raw paired schemas stay native | Identify provably safe normalization; do not silently weaken oneOf/required constraints as a blind compatibility transformation |
| empty replies / gzip / tool IDs | Bounded framing, gzip and reversible IDs have tests; paired raw completion now requires content and a valid successful stop before issuing origin receipts | Keep strict malformed/error/unknown-control and cancellation regressions alongside donor-shaped inputs; native-only bytes are not an excuse to bless incomplete output |
| Messages tools and controls | Ordinary custom function definitions, exact schemas, tool choice, real results, text/images and supported sampling have a direct native encoder; bounded typed JSON/SSE renderer validates tools/stops before success. Source output effort/thinking budget selects the native variant; disabled thinking preserves default selection | Exact per-request native thinking-budget execution is not inferred from effort selection. Server tools, redacted blocks, tool error flags and unknown controls remain explicit unsupported cases; no fabricated fallback user turn or result |
| Adaptive / Fusion | Exact trimmed case-insensitive router UID/family filtering matches pinned donor and upstream `c8290df25991865d0b9eed60937aeb73ab7fa255`; unrelated fusion-research remains selectable. Router exclusion precedes family metadata validation | Router-only catalogs fail instead of presenting an empty usable catalog |
| Execution accounting / diagnostics | API7 sparse facts keep measured token/cache counts, native request ID, actual client response ID, served model/upstream, stop, committed credits and ACU-valued reported cost; partial measurements precede failed terminals; bounded code-only diagnostics cannot change the result | Native descriptors do not establish reasoning/cache1h/deferred/tier facts; quota display fields and currency-unspecified overage are separate limitations. Exact mapping and absence rules are in EXECUTION-FACTS.md |

Port into codec/outbound/provider domain code with tests, not legacy
OpenCode/Bun host plumbing. Keep the four bootstrap operations and the
single-native-bundle startup checkpoint independently reviewable.

Do not copy donor prompt substitutions, missing-tool-result manufacture,
credential-file fallback, a second community-plugin installation, or
timers that spend retained credentials without current host authority.
Do not label the broad remaining matrix as implemented or full native parity.
