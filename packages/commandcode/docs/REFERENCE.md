# CommandCode detailed reference

The current runtime requires API 1.12 with `request-policy` and
`provider-signin`.
[REQUEST-POLICY.md](REQUEST-POLICY.md) supersedes the prior API1.9 floor and
acceptance description below. Remaining detailed parity notes are preserved
pre-adoption context, not current request-policy verification.

This reference preserves prior implementation notes and checkpoint claims.
Use [the package README](../README.md) for the current workspace entry point and
[migration verification](migration/RELOCATION.md) for this relocation. Historical
checkpoint statements below do not assert current host, vendor or platform acceptance.

Self-contained Command Code integration for the Magpie SDK.
The native package targets SDK API 1.9 and native ABI 1 because checked agent
configuration consumes effective output and reasoning metadata. The separate
`systemone` provider operation requires API 1.7. Current verification uses the
leader's merged API 1.9 base with a separately pinned completion overlay;
its exact tree and scoped package results are recorded in
[VERIFICATION.md](history/VERIFICATION.md). The completion fix's host review and merge
remain separate. Earlier SDK 1.5/1.6
packages and their evidence remain historical checkpoints.

One package contains the native Go controller, Provider API Chat, Responses
and Messages forwarding, Go-plan `/alpha/generate` conversion, API-key and
loopback browser authorization, explicit CLI-key import, plan-aware model
discovery, usage, ZDR, Jev/SystemOne decisions, checked agent configuration
and a TS Usage panel. All implemented capabilities ship in the default package.
It has no additional provider/auth plugin or npm runtime dependency.

The Command Code subscription declares API key, browser approval and CLI import
in the ordinary Providers sign-in broker. Reauthorization must confirm the
same vendor user and publish the selected SDK account ID with a new generation.
For older credentials without a vendor user ID, only the already saved key
can establish identity. Successful verification records the available name.
An explicit 401 or 403 from `/alpha/whoami` requires sign-in again while temporary verification failures
remain unavailable. The Usage panel retains plan, period, balances, ZDR and
System One details for the chosen account and generation.

Messages token-count requests preserve `/messages/count_tokens` on the
Provider API. The Go-plan lane refuses token counting before acceptance;
it never turns counting into generation. Conversational execution and model
discovery stop when subscription lookup fails or returns malformed data.
An explicit no-plan response remains a known Provider API state.
Usage reports the rolling resets and plan period end as Unix milliseconds.
Monthly usage includes amounts and limits in the plan's unit. Purchased and
free dollar credits appear separately for plans denominated in credits and
as a balance for accounts with only a known dollar balance, including zero.
Sign-in admission reclaims expired flows for all three auth methods before
enforcing the limit; live flows remain bounded, and removed browser listeners
are closed and joined outside the controller lock.

Checked agent Connect, model edits and catalog sync map the host's effective
context, maximum output, effort levels and valid default into CommandCode's
BYOK configuration. Unknown limits stay unknown; an empty effort list disables
reasoning. Existing user output limits, valid per-model effort choices and Auto
survive repeated sync. Disconnect restores generated metadata keys individually
and preserves edited user limits. Canonical SDK `none` uses CommandCode's `off`
spelling in vendor settings and metadata, and inspection maps it back. See
[the API 1.9 contract and proof plan](OWN-MODEL-METADATA-API9.md).

The separate API 1.9 `decisionModels` discovery option accepts public provider
and group DTOs in both inspection and planning. The SDK validates their
identity, ownership and relative decision endpoints; unknown DTO fields and
supplied null catalogs are refused. There is no recovered CommandCode decision
configuration schema, so discovery leaves the existing conversational config
unchanged. Jev remains available through the ordinary gateway decision routes.

Fresh quota reads echo the selected host revision and leave historical
`AsOf` unset. Unknown subscription or allowance reads cannot acknowledge
dirty usage or reopen inference. The installed native-owner probe exercises
these barriers through the actual host and ordinary provider quota path.

Jev/SystemOne uses ordinary `/v1/systemone` and `/v1/decisions` through the
selected-account operation. The canonical model is `typesafe/jev`; recovered
aliases never gain chat authority. Successful model discovery publishes an
explicit module-owned decision fact; malformed or empty discovery fails
without inventing a catalog. The legacy `systemone` module action refuses to
bypass ordinary decision authorization. Native history has no recovered
implementation and is not advertised.

The Go-plan module wire accepts canonical Chat. Go models declare that lane
so the host can translate Responses and Messages clients through its generic
protocol adapters. Direct non-Chat native module calls remain refused.
The base candidate has a generic host completion bug: a protocol decoder can
close after its terminal and cancel the final SDK receipt. The separately
recorded compatibility overlay lets only successful explicit terminal decoding
grant bounded tail draining and receipt waiting. Actual typed decoder rejection
is sticky across later valid events; JSON syntax validity alone is insufficient.
Early caller close never drains;
completed receipts retain authority even after client protocol rejection.
Overflow, timeout, late errors and missing receipts preserve uncertainty and
no-replay. A native worker ignoring cancellation can still block its join.
The final typed overlay and corrected module pass combined installed parity
for all three protocols, decisions and agent lifecycle. That scoped proof
remains distinct from the completion fix's host merge.
Non-auto `tool_choice`, `parallel_tool_calls=false`,
`response_format`, tool-result images, unknown nested controls and unsupported
stream options are refused before native dispatch. `include_usage=true` is
accepted. The recovered model table fits reasoning effort to the nearest
supported level, choosing the higher level on a tie.
For models with recovered thinking support, canonical `none` and native `off`
send the current official harness's explicit `reasoning_effort:"off"` wire.
Models without that evidence refuse disabled reasoning; known models without
thinking refuse effort controls. Unknown models cannot gain a guessed
`minimal` wire value.

A complete native `pause_turn` response continues the identical request with
the same session, account and original deadline, up to five continuations,
as the pinned official 1.89.1 harness does. Text, reasoning and tool identities
remain in one answer and per-segment usage is added once. Truncation, late
errors, rejected continuations and exhausted pauses stay uncertain without a
successful terminal or automatic retry.
Chat usage retains inclusive prompt totals. The completed SDK receipt reports
uncached input, cache reads and cache writes separately for the host ledger;
late cache metadata that exceeds the native input total is refused.

Google Provider Chat and native function tools expand supported nested local
schema references without dropping their constraints or sibling annotations.
Root definitions, unresolved/external references and data stay intact.
Cyclic and excessive expansions are refused instead of weakening the schema.
For native Go-plan generation with `google/gemini-*`, a schema type array
containing exactly one supported type and `null` is sent as that type with
`nullable: true`. A null default on the converted node is omitted. Required
lists, constraints, literal data and other transport/model schemas retain
their existing handling.
Other Provider API protocols preserve their own request and response bytes.
Provider API SSE forwarding observes the canonical protocol terminal before
publishing a completed SDK receipt. EOF alone or malformed events leave the
operation uncertain while preserving every forwarded byte and the replay fence.
Opaque non-streaming JSON responses retain their existing forwarding behavior.

See [SOURCES.md](SOURCES.md) and [source-lock.json](source-lock.json) for exact
source versions, fixture hashes, watchpoints and remaining limits.
[PLAN.md](history/PLAN.md) and [CONTRACT-PLAN.md](history/CONTRACT-PLAN.md) retain the bounded
extraction and verification plan.

Build only on usbox with temporary HOME/XDG, the existing pinned Go caches,
`GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local` and no inherited account
environment. Use `python3 scripts/build.py --archive --output <empty-dir> --sdk <sdk-checkout>`
with the installed Go and TypeScript tools. Validate the resulting ZIP with
`python3 tests/check_package.py <archive>` before installation. The host
installs the extracted native package without rebuilding itself.

The local Go replacement and TS type import are development inputs for the
SDK source revision in the lock; they do not pin a published SDK release.
The [contract record](SYSTEMONE-SDK-CONTRACT.md) retains the historical
decision proposal. The API 1.9 record describes the negotiated metadata
projection, checked configuration and remaining final-parent acceptance.
The independent first deployment with zero plugins remains separate.
Run `tests/provider_signin_host.py --host HOST --distribution DIST --output NEW_DIR`
with the bounded usbox network guard to exercise the packaged native module
through Plugins Add, Providers, and Usage. The verification uses local HTTP
fixtures and synthetic account data.
Live vendor approval, actual CLI continuation and native macOS or Windows
execution remain open. `tests/account_details_ui.test.cjs` checks the packaged
Usage panel with synthetic bridge responses under the host's sandbox.
