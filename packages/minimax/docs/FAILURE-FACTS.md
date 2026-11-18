# Internal failure facts delta

This delta follows the accepted v4 package. It preserves bounded vendor facts
inside the Go adapter before generic errors or quota responses discard them.
It does not implement public API 1.12, a request policy matcher, account
selection, waiting, reactive refresh or business replay. The manifest remains
API 1.11/native ABI 1. The v4 archives, distribution and acceptance receipts
remain separate, immutable evidence.

`src/failure.go` retains the existing MiniMax quota classifier and adds original
wire status, code presence/validity, envelope origin and timing knownness.
`src/failure_details.go` carries those facts through errors without putting raw
messages into `Error()`. Selected credentials are redacted from the facts,
including inner error carriers. `safeError` and accepted tool/media wrappers
retain the facts while preserving their existing public error and uncertain
outcome.

| Boundary | Internal preservation | Remaining public boundary |
| --- | --- | --- |
| Nonstream chat | `prepareChatResponse` captures original status/code/type/message before the existing business/quota normalization; `EffectiveStatus` is separate | ExecuteResult still returns the existing status/body; original facts are not serialized |
| Selected JSON, including web search and account APIs | `checkedJSON` errors carry original response facts; `SelectedClient` establishes selected-site trust and redacts credentials | SearchResult, SDK errors and account result DTOs retain their existing schema |
| Matrix tools and media calls | Connector `error_code`/`error_message`, or nested `result_json` failures, retain bounded facts before generic tool errors | Accepted work remains uncertain; no new host refusal DTO is emitted |
| Nested upload form | Nested business code/message are retained with origin `upload-envelope` | Transport metadata already absent from the parsed value stays unknown: wire status 0, untrusted |
| Media upload/download | Refusal bodies and headers retain `media-transfer` facts; transfers remain untrusted account endpoints | Foreign 401 never authorizes selected-account refresh |
| SSE error event | Framed error facts are captured with `OutputStarted=true`; transport failure after the event marks completeness false | The existing uncertain outcome remains; emitted bytes are not replayed |
| Partial/oversized bounded reads | Available wire status and headers survive with completeness false | No inference of no-effect or terminal auth from an incomplete response |

`BodyComplete` means the bounded error payload was available and recognized;
it is not evidence that an operation was rejected before execution. Likewise,
selected-site trust is endpoint provenance, not permission to refresh or retry.
Accepted receipts and already-emitted output remain independent fences. Video
job status and asset visibility polling retain their existing workflow semantics.
This delta does not reinterpret a failed job as a transport refusal.

Timing currently recognizes only the actual HTTP `Retry-After` header. One
nonnegative integral seconds value becomes milliseconds, including explicit
zero; an HTTP date becomes an absolute UTC reset time. Absent, malformed,
negative, duplicate, oversized and overflowing values remain distinguishable.
No duration is inferred from vendor prose. Credit expiry, usage windows and
device `slow_down` are not substituted for a refusal's retry time. A 7200-second
fact causes no sleep here. The host must decide whether to use a short wait,
another eligible account or a terminal response under its shared policy.

Source provenance remains MiniMax 0.2.9 at community commit
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`, recorded in `source-lock.json`.
The connector outer error fields and their precedence come from
`packages/minimax/connector-tools.mjs:175-190`; nested upload decoding comes
from `packages/minimax/native.mjs:546-554`. Numeric MiniMax envelope handling
uses the accepted native `apiCode`/`envelopeError` mapping. Malformed numeric
codes and explicit null codes remain unknown rather than becoming auth/quota
evidence. Synthetic tests exercise these shapes; they are not fresh vendor
captures or evidence for undocumented retry/reset fields.

Verification and exact source/SDK hashes are recorded in
`/tmp/magpie-minimax-facts-20261115/TERMINAL.md` and `FREEZE.json`.
The old-code red overlay contains only the new regression file over accepted
v4 source. Package race tests, vet, formatting and a C ABI shared-library build
run on regular usbox with isolated HOME/XDG, pinned offline caches and the
descendant network guard. No live account/vendor check or new ordinary-host
installation acceptance is claimed for this delta.

Public projection is a dependency on the separately owned, frozen shared
request-policy DTO and its acceptance rules. It still needs an independent
integration review across ExecuteResult, search/tool errors and host selection.
This internal extraction result is not full routing readiness.
