# Community ZCode domain comparison

Historical read-only donor:
`../magpie-community-plugins`, clean commit
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`,
package `@magpie-community/opencode-zcode-auth` **0.1.61**.
No dependency install, hook/transport loading or donor edits.

The bounded 0.13.1 continuation also reads exact Git objects through
`87a8cf03c62fa55ca845c51648e8859ece580336` (0.1.66), including 0.1.63 CN
identity, 0.1.64 Start requeue and 0.1.65 unclaimed weekend Trial behavior.
Observed origin/HEAD `a60667b57040f62080da60f6018cc7351fa1953f` has the
same ZCode subtree; the checkout stays on `26ea8faf`. Exact update
commits, versions and nine production/test hashes are retained in the
additional `community-zcode-0166` source-lock entry.

The donor's TS/Bun/OpenCode auth ownership is not reused. Domain behavior
and fixtures are evidence for implementations inside our one native
module and SDK account owner.

Current bounded donor adoption uses0.1.68 at
`324acd6007f2d513c5072fe2e9fe1db3524ae03d`. Its cap changes are implemented
in the existing Start transport owner: account3 plus6 per exact
origin/proxy route, with no provider maxAccounts default. This is an
instance-local adaptation of donor's process-local ADDRESS, not enforcement
across worker generations, other processes/apps or a discovered physical IP.
Host budgets, recovery permissions and durable accounting remain separate.
The donor's enc:v2 automatic desktop-store diagnostic does not describe our
explicit polling/snapshot sign-in and was not copied. Bounded author gates
and independent source/terminal-evidence closure are positive on the sealed
`0ebae38a4bca56c1ae75d2afa7dc21237636072871d245023103c1e5094dee7f`
source. Exact gate counts and reviewer/author hashes are recorded in
[SOURCES.md](SOURCES.md) and `community-zcode-0168.verification` in
source-lock.json. This metadata update preserves the original execution
identity and all historical reds; installed-host, live vendor/client,
distribution/release and full GUI acceptance remain separate.

| Domain behavior | Our current implementation | Confirmed remaining delta |
| --- | --- | --- |
| Personal/Team/Start sign-in | Explicit headless/browser flow, PAT refresh, selected same-account renewal and confirmed Personal→Team→Start auto selection | Unknown/malformed/no-unique-Team selection never mints or falls through to Start. App-linked live credentials require an explicit owner/grant. |
| Dynamic coding models | Recovered config rules, authoritative entitlement, gift-only plain names, spent Trial aliases and manual paid imports with a valid selected session | Unknown paid entitlement does not become inactive. No live config-fetch compatibility claim. |
| Per-conversation cache identity | Stable session/device metadata, user-turn query identity, trace and unique request IDs for ordinary/native inference; pinned CLI source-shaped Start envelope/cache boundaries | Runtime environment, skills and cwd context come from the actual client, never invented by the plugin. Actual client absent. |
| Gift/Trial model routing | Strict dual Trial, configured free-first including manual imports, gift response names, one-minute quota rest, held-route paid bypass and current model applicability for manual Start | Paid fallback remains narrower than donor: fully read output-free 1005/1113, settled principal debt and fresh paid room. Gift-only, manual Start and Trial never acquire paid authority when gift confirmation changes. Unknown balance refuses dispatch rather than copying the donor's permissive send. |
| Busy/network risk control | Per-account FIFO cap 3, cancellable 120-second queue, adaptive cap/shared route hold; a sustained unused cap beside inflight requeues at the head within one shared wait budget and four turns | Memory state is bounded per instance. Lone caps keep bounded one/two-second retries. Donor free-text busy heuristics are not replay authority. Native ticket/card inference never retries. Paid execution is unaffected by Start holds. API9 metadata exists; adaptive hold/cap card hints remain a separate module adapter. |
| Provider concurrency defaults | Full package retains3 requests per account and omits the former two-account default; existing Start transport admission adds6 per exact origin/proxy route and releases at body completion/close | Explicit user values, including zero, remain host-owned. This Start-only protection is local to one module instance, not a host-global/address scheduler. |
| Gift and paid usage | Dual windows, Trial applicability, paid exclusions, per-model exhaustion, unknown totals, best bucket and pending informational windows; full API9 carries actual amount/limit/display and donor periods | Pending windows use Aside and starts-at without routing selectors. Missing Trial may read the existing feed for unclaimed names: a ten-minute selected-session cache and three-second read bound; no claim or entitlement. Numeric period codes never become unit/currency/cost labels; historical API4 output omits API9 metadata. |
| Gift claim | Principal-fenced `billing.claim`: bare code 0 acknowledgment, exact returned-plan confirmation, 1003 done, 1004 ineligible, 3002 rate hold, 3007 captcha; unknown results never repost | Bare acknowledgments do not prove entitlement. Explicit malformed/foreign plan data stays unknown. No captcha attestation or live-vendor acceptance. Acknowledged claims retain conservative baseline reconciliation before any later repeat. |
| Daily check-in | Explicit actions and selected-generation page controls, feed-only action extraction, confirmation/single-flight UI, one check-in lease, durable claim fences and shared principal rate holds | No automatic scheduler or actual native-operation wire integration. SDK 1.6 native account operations exist; recovered client wire/model launch still needs a real adapter. |
| ToolSearch/session/streams | Bounded codecs, nested message_start tool restoration, multiline SSE, signature/tool payload retention, sparse API7 token/cache/reasoning snapshots and partial-output uncertainty | Execution credits/cost/currency, deferred counts and upstream-provider identity lack source authority. They remain unknown; no wholesale loader copy. |
| Agent startup/Library | Same native/UI bundle owns zcode-native; checked provider-file plans, synthetic ordinary launch, OffPeak/Highspeed gateway routing and inherited Library are exercised | Actual recovered ZCode executable startup, custom data-root Library placement and rendered-browser acceptance remain unverified; see ONE-BUNDLE-AUDIT.md and AGENT-EVIDENCE.md. |
| Native history/diagnostics | JSONL/legacy bundles, native SQLite with optional model/turn/tool usage rows, exact raw usage/attempts/nulls and CLI diagnostic JSONL including incomplete tails | Automatic DB/WAL capture, arbitrary log-source collection, actual client resume and financial journal import remain separate. Logs are not fabricated as session messages. |

## Historical 0.1.61 evidence hashes

| Donor file | SHA-256 |
| --- | --- |
| package.json | `c3ba49c27df561cec86def92fabccb1b48e2ab93a9401694e5bf54b728c57bc2` |
| README.md | `360146b1f5a2720d70b39689f57ca2d4edfb2506f99d4fce5939a16d0d1a233f` |
| index.mjs | `b29c486235227c339807755872c020af0b8df61a26812fa160230d74778ef277` |
| prompt.mjs | `d0032db8a3560bfba3ecce34ed8513268dc6d3ad9192f50775285da1fc26b527` |
| checkin.test.mjs | `508fc1edcee95fe1770eeaa927dfaf94f35180899002dd2c1a2226c6fc7d6ed8` |
| gift.test.mjs | `1f66df99849f06e44b41a30781a33cba43ae95381cedc3fb3d7bc859b52df184` |
| team-gift.test.mjs | `09c241d6f4752d0cd642589c3e79ce3c4ad00dbb2a6030157961b902881a805e` |
| dual.test.mjs | `a12701020b6240ddc576a31e4130ae21e4ad04d1509d849a9301ad72cae6eb57` |
| inflight.test.mjs | `a734815f0319f5f8899cd19a5ea2a0d5e8813a57de150a6258bcb7ea72a6c6b9` |
| pending.test.mjs | `dd9cd0adbae0809625c8e7f1ca31b55350b14bdb5242e79f175ca7bf7a756c36` |
| session.test.mjs | `da54e05db8aa9b7abe6c0a008a4f3a64e68f9b327ac74e7d007cd1cbff54cadc` |
| wall.test.mjs | `42c311e35c116fe0643a44c73abde09a07f7b2400f2b09f7d8d8b284ff4e1bef` |
| toolsearch.test.mjs | `89ba01eb697d2865d595d750a850695abd479d659b8335f8597357dca0a54e7c` |

This matrix records actual gaps. It introduces no community-plugin
dependency and does not establish complete donor or actual-client parity.

The bounded 0.7.0 / native-UI-agent 0.10.0 continuation is recorded in
[PARITY-EVIDENCE.md](history/PARITY-EVIDENCE.md). Desktop 3.18.2 / CLI 0.17.1
source at `fec975905a2abd4c07e35c40d51fa097a2258ec9` separately grounds
Start billing, the marketing action schema and supported locale set
(`zh-CN`, `en-US`, `ja-JP`, `ru-RU`, `ko-KR`). The community donor still
uses 3.17.2 wire semantics; it is not evidence of actual 3.18.2 client
execution. Exact checked-file hashes are retained in `source-lock.json`.

The historical 0.11.0 continuation is module-only; its file-level
extraction plan is COMPLETION-PLAN.md. The ordinary 0.12.0 full package
adopts public API7 Usage.Facts and diagnostics.emit against the owner's
immutable API8 source snapshot `43b7e290d9c3370881780b23ad49222f5d27f44b`.
OBSERVABILITY7-EVIDENCE.md records its exact source and acceptance.
The explicit historical headless build retains its legacy usage
projection. Final committed-host acceptance and remaining generic
proposals in LEADER-CONTRACTS.md stay separate.

The ordinary 0.13.0 package adopts the bounded API9 concurrency/count/period
slice. [API9-EVIDENCE.md](history/API9-EVIDENCE.md) records scoped ordinary-owner
proofs and the separately handed-off cold configuration/account-stamp
finding. No actual client, vendor or financial-resume acceptance is inferred.

[DONOR-0166-EVIDENCE.md](history/DONOR-0166-EVIDENCE.md) records the subsequent
bounded comparison. CN oracle and Discovery gift/failed-plan fences pass
on unchanged 0.13; automatic app cached-profile import is still absent.
The compiled old-source regressions expose manual Start model bypass,
missing unclaimed Trial hint/cache and failed in-flight requeue. The
current module closes those concrete gaps while preserving stricter
unknown-entitlement and output/transport replay guarantees.
