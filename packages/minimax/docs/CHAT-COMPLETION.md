# Chat completion regression

The previously reported plain Chat 409 came from the policy fixture's response
format. It did not establish a failure of healthy MiniMax Chat or a host-core
defect. The fixture now honors the actual upstream `stream` request. Host and
production module bytes are unchanged.

## Exact cause and effect boundary

The caller asks `/v1/chat/completions` with `stream:false`. The ordinary host
translates that request to MiniMax's Messages protocol and sets upstream
`stream:true` even for a whole caller response. The host collects that stream
and renders the whole Chat completion.

In the frozen timefix host:

- `internal/gateway/gateway.go:9467` saves caller streaming mode and sets
  `request.Stream = true`.
- `internal/gateway/gateway.go:8181`, `eventStream`, tests the content type and
  actual initial bytes for SSE.
- `internal/gateway/gateway.go:9504` rejects a nonstreaming success response
  with `did not stream`/502.
- `internal/gateway/gateway.go:9540` marks protocol completion only after a
  valid terminal stream with no decoder rejection.
- `internal/provider/extension_outcome.go`, `ExtensionAttempt.Outcome(true)`,
  retains the accepted/completed replay barrier when a decoder fails. The
  outer gateway reports the terminal operation as 409, rather than repeating
  paid work.

Original fixture receipts explicitly show upstream `"stream":true`, followed
by fixture `application/json` and a whole Messages object. Its valid JSON,
text and usage did not satisfy the requested streaming protocol. Success of
the module's byte transport alone was insufficient for gateway completion.
The old-worker control reproduced the same fixture mistake.

The correction is in `tests/policy_host.py`: a successful upstream streaming
request receives complete Messages SSE: `message_start`, text block
start/delta/stop, `message_delta` with end_turn/output usage, and `message_stop`.
A nonstreaming Messages request still receives its whole JSON response.
`--success-format whole-json` explicitly retains the old invalid response as
a negative control. This option controls only synthetic vendor responses;
no production endpoint, CLI alias, account selector or effect contract changes.

## Causal checks

The old 409 controls remain in
`usbox:/tmp/magpie-minimax-policy.9CkOtx0J`. The continuation has its own
evidence root, `usbox:/tmp/magpie-minimax-chat-20261115.1r0t7eOp`.

The new tests reuse the exact same host and current/original exec worker
binaries as the old receipts. All 33 current files under `src` match the
preceding public-policy source freeze. No host common-policy changes are
required to test this correction.

| Gate | Response/input | Observed result |
| --- | --- | --- |
| Original `host-control` | Current worker, whole JSON despite upstream stream:true | Behavioral red: 409/one account instead of expected 200. |
| `chat` success | Current worker, complete requested SSE, whole Chat caller | 200/one account; answer, stop finish reason and usage 7/3 preserved. |
| `chat` fallback | Current worker, actual HTTP200 code1008 refusal, then complete requested SSE | 200/two distinct accounts; original refusal facts and timing retained. |
| `chat-old` | Original facts worker, complete requested SSE, whole Chat caller | 200/one account. This isolates the fixture correction from policy source changes. |
| `chat-whole-negative` | Current worker, deliberately retained whole JSON for upstream stream:true | Terminal 409/one account; no paid replay. |
| `chat-stream` | Current worker, complete requested SSE, streaming Chat caller | 200/one account; answer, stop and exactly one DONE terminator. |
| `host-messages` | Ordinary direct Messages success and quota fallback | 200/one and 200/two distinct accounts. |

The original eight-case Messages fence run is retained. The continuation
does not rerun a broad host suite or change accepted media, partial SSE,
transport uncertainty, malformed-facts or final-refusal authority.
All six final guarded stages (`chat`, `chat-old`, `chat-whole-negative`,
`chat-stream`, `host-messages`, `static`) exit zero. The five host stages
cover seven cases. The final assertions check upstream stream:true and, for
whole Chat success/fallback, exact answer/stop and 7/3 usage. Streaming Chat
has exactly one DONE terminator. An initial file-permission setup failure
occurred before worker execution and is retained separately; executable
metadata was then preserved from the original binaries.

## Native boundary

The owner-visible report
`usbox:/tmp/magpie-native-acceptance-20261115.D2KiOWCp/REPORT.txt`
establishes bounded Linux native runtime acceptance on its exact frozen
`candidate-closure` baseline. It explains the hidden-root-ownership namespace
negative and the successful regular-usbox owner-visible harness.
That evidence supplies a valid verification environment, not acceptance of the
current MiniMax API 1.12 module. Current-module native verification remains with
the parent/Dirac owner; no native loader guard is altered here.
