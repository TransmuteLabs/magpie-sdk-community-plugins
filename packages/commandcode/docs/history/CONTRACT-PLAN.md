# Bounded implementation contract

The package owns vendor formats and account state. The host retains account
selection/generation, routing, configuration snapshots, cancellation and retry
authority. The user separately authorized a generic bounded completion/close
fix in `internal/provider/extension_transport.go`, its causal regression and
minimal terminal-confirmation hooks in the three translated readers
(`gateway.go`, `continuation.go`, `search.go`) with a narrow SDK regression.
The accepted typed-decoder review correction additionally observes parse
rejection in Chat, Responses, Anthropic and Gemini/Code Assist without
changing their tolerant handling of unknown events.
That exact source overlay is recorded separately from the frozen candidate;
the leader owns staging, committing and merging those host files. The owning
extension-provider reference documents the shared two-second receipt/tail
budget and the unbounded native-worker cancellation constraint.

## File boundaries

1. `billing.go`, `models.go`: pure billing/catalog decoders. Tests pin unknown
   versus empty allowances, credit units, supported endpoints and decision-model
   exclusion from conversational model discovery. Recover the entire fallback
   catalog from the pinned community source, never a hand-selected subset.
2. `generate.go`, `generate_stream.go`, `google_schema.go`: canonical
   Chat-to-native conversion, bounded NDJSON-to-Chat output and shared Google
   function-schema normalization. Preserve tools, user images, reasoning and
   schema constraints. Unknown controls fail before dispatch. Only a complete
   pause terminal permits the owning transport's identical-body continuation,
   at most five; usage is additive. Truncation and trailing errors cannot
   authorize continuation or a successful terminal.
3. `module.go`, `transport.go`, `provider_stream.go`: public SDK provider handlers and scoped network
   callbacks. Provider API bodies stay in their original protocol. Go-plan
   models advertise Chat so generic host adapters translate other client
   protocols into that implemented native lane. Never retry accepted/uncertain
   requests; a proven completed pause is a bounded continuation of one turn.
   Provider SSE bytes remain opaque forwarding data, while an explicit
   canonical terminal gates the completed SDK receipt. EOF-only or malformed
   streaming replies retain uncertainty.
4. `auth.go`: API key, explicit fixed-path CLI import, loopback browser flow.
   Credentials enter only the authentication contract; selected account auth is
   never replaced by implicit CLI credential following.
5. `cmd/native`, package builder and provenance: one native module, no external
   auth plugin. Validate package identity and source revisions.
6. Agent configuration and optional UI follow the working provider slice.
   Their readiness is reported separately; native history requires recovered
   evidence and is not invented.

## Negotiation

The default package is 0.1.3, minimum API 1.9 / native ABI 1. SystemOne requires
the selected-account decision operation introduced in API 1.7; the legacy
module action refuses to bypass it, and Jev never has conversational protocols.
Checked agent configuration consumes API 1.9 effective output/effort metadata.
The separate validated decision catalog grants no conversational or guessed
vendor-configuration authority. The negotiated host identity is recorded in
`source-lock.json`; completion-overlay review remains distinct from the merged
API 1.9 base and earlier
API 1.5/1.6/1.7 checkpoint receipts retained in `VERIFICATION.md`.

## Verification

All compilation and execution happens on usbox with temporary HOME/XDG,
offline existing Go caches and local HTTP fixtures. First run meaningful
regressions against absent/incomplete behavior. Then run unit/race/static
checks, actual C-shared registration and ordinary host provider calls against
local mock servers. Record exact tested source identity. Live vendor, browser
approval and real CLI import are deliberately outside fixture evidence.
