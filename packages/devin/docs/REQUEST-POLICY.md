# Devin request policy

The default 0.8.0 and paired 0.5.0 manifests now require public SDK API 1.12
and declare request-policy. The inbound-only 0.2.0 converter is unchanged.
The accepted 166-record v2 source is preserved separately; its earlier green
receipts do not establish acceptance of this integration.

Source-lock identities include exact capability lists for all three variants.
The builder refuses request-policy below SDK1.12 even if a downgraded manifest
and source lock agree, and requires compatibility.hostAPI to match the declared
SDK floor. Generated distribution provenance is projected onto the selected
manifest; paired/default API1.12 and inbound-only API1.1 remain distinct.

## Boundaries

Outbound errors retain original bounded response code, type, message, selected
timing headers and parsed duration in `Error.Facts`, before safeCode or native
status normalization. `Error.Status` and `Error.Code` retain the legacy public
error projection; policy uses Facts. HTTP 200 with a Connect error remains
Facts.Status 200. RefusedExecution also returns Status 200 in that case; the
host classifies it and emits its own 429/401/400 response. No vendor HTTP status
or Retry-After header is fabricated.

A callback at `devin.execute.chat` or `devin.execute.native` provides intermediate
classification with empty evidence. It never authorizes transport replay or
supplies final routing authority. Final facts are freshly projected with
operation provider.execute and phase response. Rules have stable devin.* IDs,
explicit code predicates and bounded provider-specific prose. Operator policy
continues to precede these adapter rules. Neither a rule match nor a zero usage
counter proves absence of effects.

Only complete, strictly parsed, error-only envelopes with recognized
pre-execution rejection semantics qualify for final refusal. A Connect error
must be the first frame and end at EOF. Any preceding frame, including empty
metadata or measured-zero usage, prevents this proof. Unknown reasons, details
that may describe work, malformed/duplicate fields, trailing bytes, transport
errors, cancellation and published output remain uncertain. Unrepresentable or
malformed stated timing makes the facts incomplete instead of silently reducing
the minimum delay. Nested error/metadata keys are checked for duplicates;
explicit null strings and malformed metadata arrays cannot prove rejection.
Selected credentials cannot enter policy callbacks/results.

The host journals dispatched before calling the module. The foreground
predispatch callback and dispatch diagnostic still run at that point, including
the historical diagnostic code devin.execute.accepted. That diagnostic is a
dispatch observation, not an accepted event or replay authority. Foreground
accepted publication now precedes the first response frame/usage/output, or an
uncertain postdispatch failure. Proved refusals publish none of those events.
Cache-task acceptance is unchanged. The host's durable dispatched/uncertain
fence protects cancellation, crashes and lost callback acknowledgements.

For native passthrough, the complete first error terminal is held until this
decision. Only a valid final refusal releases that invocation's bridge claim
for the host's next attempt; prior frames/output never release it. The next
attempt still resolves selected credentials and checks signed-history origins.
No successful origin receipt, paid cache successor, new tool/media operation or
module-owned retry loop is created by a refusal.

## Scope and timing

Free refusals use an exact selected-account/model window `devin.free-model`.
No shared bucket is inferred from costTier4, SWE family membership or prose.
Another model or another account remains independent. Monthly ACU refusal is
also conservatively scoped to the selected model because this adapter has no
proved shared paid-bucket identity. Credential refusal names the selected
account and never requests reactive 401 refresh.

The exact pinned donor example says the free limit resets in 42 seconds.
It supplies retryAfterMs 42000. Minutes and HOURS examples are synthetic parser
controls, not captured vendor replies. Missing reset remains absent. Unsupported
or overflowing stated durations are incomplete facts. The existing paid daily
and weekly status windows read epoch seconds and project milliseconds while
excluding the live catalog's costTier4 free IDs. They never supply a free reset.

The host owns account order, final availability, shortWait, cancellation and the
logical-request budget. Callback classifications and final decisions are
separate observations. No adapter timer, account selector, credential manager,
new dependency or generic replay of earlier paid work is introduced.

## Sources and verification

Read-only host design:
`magpie/docs/subsystems/request-policy.md`,
`pkg/extension/request_policy.go`,
`internal/provider/extension_request_policy.go`,
`extension_outcome.go`, `extension_transport.go`.
API1.12 authority is the frozen candidate-closure and parent's minimal-timefix
candidate, copied into the isolated verification packet. Exact input and binary
hashes, causal failures, final freeze and remaining gaps belong in the packet's
TERMINAL.md and JSON manifests.

Inventory SHA256:
`9041b23f81b4269bf7126a463f045f925f3d660c1dc4ab759a8ac690b810fb8e`.
Fixture JSON SHA256:
`971402ad43ab862b1ff0da8c2458e2de6497c057b2386a695d31f800634c2601`.
Both are the supplied November 15 adapter-inventory artifacts.

CLIProxy committed donor: `842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9`.

| Source | SHA256 |
| --- | --- |
| devin_journal_test.go, exact 42-second donor fixture | `12cf43d5e5568a55f9df7a0ad8278dc72aa3ef46c6161d725012bd99b9bcdf5c` |
| devin_stream_status_345_test.go, absent reset | `58ddb5bf9bbd2b953aa5c7eabbdc929fe5e4acfc51c95816ce55836065ff6152` |
| helps/devin_wire.go, code/message distinctions | `d139a0c296a0a99c837e248cb70482734cec1cef6ef76c337e020173deb38a14` |
| helps/devin_wire_test.go, quota versus client-version examples | `abca709730bc2e202c4b01eda9653597e270a1643273170184b7f169bb97510d` |
| helps/devin_quota.go, separate paid-window timing | `385dc94fe3f26570c8993059c966762a55387959c3a537856936cace9535e92c` |

Community donor commit: `57d1cc6a9f49389d495ea9be8d986c70de7767bf`.
Its packages/devin/free.test.mjs hash is
`591d98af060a9cfe35c13a8ca4416384bd795780796a931ea6c30b49cb6a521f`;
index.mjs hash is
`4cc31bf5291b169f72aaf2275207737060dbe34e6be6c4f9e7924704e275c0b2`.
No fresh vendor capture or dirty donor merge was used.

`provider_request_policy_test.go` exercises actual SDK events and final results.
`outbound/response_facts_test.go` exercises both native readers, original facts,
typed timing and incomplete controls. `src/testdata/request_policy_e2e.py`
installs the built native distribution into an isolated real compiled host,
selects three distinct accounts, checks sends/order/budgets/shortWait and
independent model windows, and joins every server/process. All executable
checks run on regular usbox with isolated HOME, offline caches and the
unmodified network guard. Native positives use a fresh ordinary host process;
cross-generation native-equivalence approval is not inferred from them.

The separate owner-visible native report is
`usbox:/tmp/magpie-native-acceptance-20261115.D2KiOWCp/REPORT.txt`,
SHA256 `86c96162f080a5c6bfc853b0a01146b655fdf7b386eedacd0b425c00a2c61371`.
Its terminal ACCEPTANCE.json SHA256 is
`f6eab2ba4d1e39629697221de156425a68874af887c9f7c8a46e1908f9067e47`.
That report establishes the named Linux native lifecycle/authority paths with
visible system ownership and preserves namespace rejection evidence unchanged.
It does not approve Devin request-policy integration or the complete SDK.
The cited independent loader review concerns its named earlier native source;
candidate5 Devin policy receipts are author evidence. Exact successor module
hashes and terminal provenance checks are in the candidate6 packet, while
parent final combined replay remains a separate result.
