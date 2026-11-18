# Bounded maintained CLIProxy comparison

Read-only input: committed HEAD
`842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9`, tree
`8444ec2d8c9c34215b103ac4d29c3037873106cf`, from
`CLIProxyAPI-family/CLIProxyAPI-u2-331t-up42`. The working tree has an
unresolved consolidation; **no working-tree bytes were used**. Reads used
`git show`/`git grep` at the commit. No donor tests/build were run and no
global donor readiness is claimed.

The ten requested test files were exported unchanged under
`/tmp/magpie-devin-provider-completion/cliproxy-head`. Their paths, line
counts and SHA256s are in `manifest.json`, SHA256
`b1a6b9bd40edccfd5064baa77795adc858029794cd390d178c5128fd57bedc42`.
This is source evidence, not a runtime dependency.

| Tests under internal/runtime/executor | Applicability and result |
| --- | --- |
| devin_a3_law_test.go | Our recovered field7 counts are bounded to int32 before sums, so donor MaxInt64 fixtures fail decoding. New regression preserves the earlier valid snapshot and fails the request. Responses/Gemini canonical projection is outside this package's advertised protocols. |
| devin_a4_law_test.go | Native readers already retain earlier summary plus an error on later invalid counts. Duplicate singular field7 is rejected. Donor drift-log recovery is not copied; malformed measurements do not become success. |
| devin_b4b_wire_test.go | Applicable absence/null/zero sampling is checked through actual Chat/Messages encoders and fixed64 wire bytes. Interactions/Gemini/codex alias containers are not native package inputs. |
| devin_b4c_alias_test.go | Additional aliases, wrapper unwrapping, namespace tools and empty-result placeholders are not copied. Strict supported fields, real results and conflict refusal remain required. |
| devin_b4d_named_value_test.go | Boolean/number/array coercions into function names/arguments/schemas are not lossless ports to our stricter function schema. Existing refusals remain. |
| devin_stream_status_345_test.go | New SDK-peer Chat/Messages checks prove metadata followed by refusal emits failure status before synthetic success, with one dispatch and uncertain outcome. Empty successful EOS remains refused despite donor R7 accepting it. |
| helps/devin_request_validation_test.go | Numeric validation applies. Donor T304 null-refusal differs from B4 null-as-absence; our established optional null-as-absence behavior is preserved. No arbitrary selection between incompatible donor tests. |
| helps/devin_ref_roles_test.go | Our encoder does not resolve/rewrite schema refs. New wire regression preserves the dependency-list/ref example and descriptions exactly. |
| helps/devin_usage_test.go | Confirmed/fixed gap: field28 Token Usage was ignored when it was the only measured source. Decoder/SDK-fact regressions failed before correction. Exact named dimensions now supply sparse fallback; unknown keys/titles do not invent counters. |
| helps/devin_wire_fingerprint_test.go | Prompt/tool-description perturbation is intentionally excluded. New regression asserts original schema, system content and description stay unchanged. Payload bodies are not added to diagnostic logs. |

## Field28 correction

Donor `helps/devin_wire.go` defines group title field1, repeated metric
field2, metric key field5 and nested dimension field4/value field2 fixed32.
Only exact case-insensitive `Token Usage`, `input_tokens`, `output_tokens`,
`cached_input_tokens`/`cache_read_input_tokens` and
`cache_creation_input_tokens` apply. Unknown keys such as
`uncached_input_tokens` are ignored. A known measurement must contain one
readable finite nonnegative integer within the existing int32 token bound.
Missing, duplicate or wrong-wire values fail instead of becoming zero.

Field7 retains the existing complete protobuf-snapshot contract, including
scalar defaults. It wins over dimension fallback regardless of frame order.
Dimensions merge named counters, preserve explicit zero and do not invent
unnamed siblings. This differs from CLIProxy's per-counter field7 presence
interpretation; that broader change is not imported. Missing operands do
not produce canonical totals or measured SDK zeros. Incomplete cache
measurements cannot authorize another paid task. Raw paired bytes and
origin rules are preserved. Earlier accepted snapshots remain available
when a later frame fails.

Coverage: [codec regressions](../src/codec/provider_donor_laws_test.go),
[provider regressions](../src/provider_completion_test.go), and
[installed fixture](../tests/provider_single_package_e2e.py), which uses
field7 for Chat and field28 for Messages/native before checking the ledger.
Old `donor-red` and `donor-expanded-red` receipts remain distinct from
corrected runs under the provider-completion evidence directory.

## Shared error-policy handoff

Targeted additional source reads, not a whole-executor audit:

| Source | Committed SHA256 |
| --- | --- |
| helps/devin_wire.go | d139a0c296a0a99c837e248cb70482734cec1cef6ef76c337e020173deb38a14 |
| helps/devin_wire_test.go | abca709730bc2e202c4b01eda9653597e270a1643273170184b7f169bb97510d |
| devin_executor_test.go | 8cf94fd668c8a67c9ca0daacf3da43ce164ed4fcdb6f616a130df4639a7fedf6 |

`ParseDevinTrailerError` distinguishes capacity, request refusal, credential
refusal and account/model entitlement despite shared status/code values.
`permission_denied` plus high demand means capacity; model access not allowed
for an account means account/model entitlement; content-policy/MCP validation
means request refusal. `failed_precondition` plus monthly ACU exhaustion is
an account budget signal, while unsupported client version is a request
problem. `invalid_argument` naming an internal error is upstream failure.
Exact cases/scopes were sent to the shared owner and saved in
`/tmp/magpie-devin-provider-completion/error-policy-handoff.md`.

Bare HTTP401 does not overwrite explicit non-auth code. Explicit Connect
`unauthenticated` remains a credential refusal; static tokens have no
autonomous refresh/replay. The donor itself calls its transport401/trailer
split open issue270. Current errors remain bounded/code-only; message-specific
canonical classes/scopes need the shared public contract. That pending
integration is separate from this package freeze and is not claimed complete.
