# Experimental Devin protocol module

File plan: `wire.go` bounds and validates protobuf/Connect framing;
`decode.go` translates the supported request subset to canonical Chat;
`generation.go` maps supported generation controls;
`response.go` translates Chat SSE or whole JSON to Connect;
`catalog.go` projects the host's authorized catalog into native discovery;
`*_test.go` exercise handcrafted schema fixtures and refusal boundaries.

The module serves `GetChatMessage`, `GetCliModelConfigs`,
`GetCommandModelConfigs`, `GetCascadeModelConfigs`, their three `*ForSite`
variants, and `GetModelStatuses` under
`/exa.api_server_pb.ApiServerService/`. It does not implement vendor sign-in,
session-control RPCs or an outbound subscription executor. It performs no
network or config IO. Chat requires SDK 0.1 `inbound-codec`; discovery
requires SDK 0.2 with both `inbound-codec` and `model-discovery`.
Registry compatibility and route admission belong to the host.

Schema evidence: `PROFILE-devin-vendor-schema-2026-09-23.md` in the adjacent
CLIProxyAPI-program briefs, and the installed
`@magpie-community/opencode-devin-auth/index.mjs` (`encodeRequest`, `encodeMsg`,
`events`). Field 21 is the model UID used by that encoder, preferred over
field 14's model name. Explicit names including `group/foo` pass through;
enum-only selections are refused. Metadata field 1 is discarded in full,
including credentials. Unknown semantic fields are refused. Tests are
handcrafted from this evidence, not proof of live native CLI parity.
The recovered `Agents/devinapp` generated descriptors and observed cassette
shapes supply additional field/type evidence: see [the evidence inventory](EVIDENCE.md).

Supported requests contain a system prompt (2), USER/ASSISTANT/TOOL prompts
(source 1/2/4), leading SYSTEM_PROMPT prompts (source 5), inline user and
tool-result images, ordinary function definitions/calls/text
results, tool choice, max tokens (configuration 2), temperature (5), top-p
(8), stop patterns (9), seed (10), service tier (17), and plain thinking
(prompt 11). One native completion (configuration 1) is supported.
Cache type 1 becomes
`cache_control: {type: "ephemeral"}`; cascade/prompt/execution ids become
allow-listed Chat metadata; native session id (28) becomes SDK
`Request.Session`, with cascade (16) as the fallback. Trajectory correlation
id (15.1) is retained as metadata, while force-billable trajectories are
refused. Default planner mode (20=1) is accepted; no-tool mode (20=3) enforces
`tool_choice: "none"` and refuses conflicting choices.
Field 27 becomes `prompt_cache_key`. Plain thinking uses
`reasoning_content`. Nonempty native history signature/type fields 12/18
are refused with a safe unsupported-request error: the current gateway
Chat parser reconstructs only thinking text, losing signatures when routing
across protocols. Empty proto3 defaults are permitted. Signed native history
is therefore outside this decoder's supported subset.
Whether a selected upstream accepts the remaining Chat extensions is its
contract. Response encoding of signed `reasoning.text` remains supported;
this does not imply that signed native history can be replayed through the
decoder. Response `reasoning.encrypted` is refused as
`unimplemented`: a sealed Codex or other foreign reasoning item has no
established equivalence to a native Devin thinking signature. Its opaque
data is never copied to native signature field 10.

Native source 2 is the model's reply despite the enum spelling `SYSTEM`;
source 5 is `SYSTEM_PROMPT`. Leading source-5 messages become canonical
system messages and retain their cache options. A source-5 message after the
conversation has begun is refused because the host's Chat parser collects
system messages before the conversation. Native `num_tokens` (uint32, field 4)
and `safe_for_code_telemetry` (bool, field 5) are validated advisory data;
neither overrides token estimation or privacy policy. Known empty proto3
defaults for provenance, phase, redacted-thinking and tool-error fields are
accepted. Active values remain refused. ImageData's empty caption default
is accepted; active captions have no established mapping and remain refused.

`Connect-Timeout-Ms`, when present, must contain 1–10 ASCII digits in one
header value. Zero is refused with a public 504 deadline error before
dispatch. A positive duration is returned as SDK `Request.Timeout`; the
core applies it from bridge entry. An omitted header adds no codec timeout.

Unsupported content is refused, including custom tools,
experiments, nondefault tool-policy planner modes, and active generation
controls with no equivalent in this Chat bridge. Configuration field 3 is
`max_newlines` and 7 is `top_k`, as the recovered descriptors establish.
Their nonzero values are explicitly refused rather than discarded. The
installed outbound plugin and observed GUI captures use 400/40 respectively,
so their complete native requests still require a native executor or an
explicitly negotiated translation for those constraints.
The schema does not enumerate `tool_choice.option_name`; `auto`, `none`,
and `required` are accepted using secondary implementation evidence only.

Requests are limited to one 16 MiB protobuf data frame (uncompressed or
gzip, with the same expansion limit). Reply processing is synchronous;
text/thinking/signatures stream immediately. Fragmented/interleaved tool
arguments accumulate up to 1 MiB total and are emitted as complete native
calls at finish. Each SSE event is limited to 1 MiB; gateway error JSON to
64 KiB. SSE success requires a finish reason and `[DONE]`; a whole JSON
completion requires one assistant choice and an explicit valid finish reason.
Whole JSON is bounded at 16 MiB and uses the same native event validation.
No output tokens are estimated. The installed plugin's `events()` sums
native fields 2/4/5 (input/cache-write/cache-read) into Chat `prompt_tokens`
and exposes the cache counts in `prompt_tokens_details`; its `v2Usage()`
independently subtracts both to recover uncached input. Isolating and running
that installed `events()` on native input/output/write/read = 7/4/2/3 gave
Chat prompt/output = 12/4 and cache read/write = 3/2. `encodeUsage` reverses
this exact mapping, refuses inconsistent counters, and does not clamp them.
This is implementation evidence, not live vendor accounting validation.
The recovered `StopReason` descriptor establishes ordinary stop=2,
length=3 and tool calls=10. Filtering becomes a terminal error.

Discovery uses unary unframed protobuf, not Connect streaming frames.
Config entries come only from the caller-filtered `/v1/models` result.
`GetCascadeModelConfigs` shares the ClientModelConfig projection. Optional
sorts, default overrides, native router hints and assignment tokens are not
fabricated. Site requests have different generated schemas: cascade uses
fields 1/2/3/4/7 (API key, client information and team); command uses team
field 2; CLI uses team field 3. Credentials/client information are discarded.
Nonempty native team selection is refused because the host has no authority
to resolve that vendor scope. Empty/default site requests use the same
caller-authorized catalog; they do not create a second authorization path.
Native status discovery returns an empty advisory list; it invents no
healthy status, quota, price, router or assignment JWT. Unary errors preserve
their non-200 status and use the Connect `{code,message}` JSON shape.

Only `Retry-After` and `X-Magpie-Session` canonical headers pass through;
native content headers are set by the codec. Every framed native response
uses HTTP 200, including invalid-request refusals: the canonical status is
kept internally and mapped to the terminal Connect error code, as required
by the [Connect streaming response contract](https://connectrpc.com/docs/protocol/#streaming-response).
Authentication/read-only failures handled before this codec retain the
core's ordinary HTTP behavior. `Unwrap` supports response controllers.

`Finish(cause)` maps an unfinished stream's deadline/cancellation to
`deadline_exceeded`/`canceled`; another cause becomes a safe `internal`
error. It preserves earlier codec errors and never exposes raw cause text.
A complete finish plus explicit `[DONE]` is not invalidated by cancellation
that arrives just after completion. Without a cause, truncation remains
`data_loss`.

Chat and discovery both reject duplicate Content-Type,
Connect-Protocol-Version and compression header values. Header ambiguity
cannot select a different decoder from the one admitted by the transport.
