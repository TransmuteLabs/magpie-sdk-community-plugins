# magpie-sdk-plugin-protocol-devin

> Historical detailed reference. Source paths and author commands below were
> relocated; acceptance statements, hashes and linked receipts describe their
> original snapshots. See [relocation verification](migration/RELOCATION.md) for
> the new workspace checks and [current entry point](../README.md).

The standard `python3 scripts/build.py --output DISTRIBUTION` builds the complete **0.8.1**
native module through `NewAgent`: inbound, outbound Chat/Messages, auth,
models, usage/Facts/diagnostics, the `devin-native` agent, four finite
bootstrap operations, managed bundle lookup and the default-off paid cache task.
Install that one distribution;
no companion plugin is required. `--agent` remains an explicit alias.
`--paired-only` selects the **0.5.0** compatibility artifact without agent
wiring/bootstrap; `--inbound-only` selects the **0.2.0** codec-only artifact.
The full and paired distributions require **SDK API 1.12, native ABI 1**.
Prior installed acceptance is pinned to committed host
`d5b098d8183e88ab9fee1da0170d1c90b9590822`, including the public API9
decision-model DTO/validator and selected-account task contract.
The accepted tree is `11c2b9dd3d514bcc76c338f9ae680552319494dc`.
The parent-built `c018c443...` runtime was built from tree8fe78; its only
differences from the accepted source are two test files. Exact build,
source and installed-fixture identities remain separate in the source lock.
The later frozen merged tree `f7f26b2d5ee5388a089ab07a72f7372ed0068971`
also passes one bounded hot replay on its own binary; this is a separate
candidate-tree receipt, not an inferred future commit.
Current domain authoring uses public SDK7bc5ae21, current community
Devin0.1.41 at main57d1cc6a and recovered wire/model authorityed84daf0,
checked 2026-11-15. Discovered family/effort/Fast/Priority/alias selection,
API9 numeric quota and representable cache boundaries are now implemented.
[Completion evidence](history/DOMAIN-COMPLETION-EVIDENCE.md) records the old-source
regressions, bounded module gates and ordinary installed-package replay on
candidate physical tree36f8e629/binary0bd335b7. Its public SDK is byte-identical
to the7bc5ae21 export. This candidate is separate from parentadcf4ce, a
future successor commit and parent whole-host acceptance.
Earlier unpublished API9 draft exports do not contain that typed field.
Source6, native account operations and modelcache share the same runtime.
[Standard-package acceptance](history/DELIVERY-EVIDENCE.md#standard-install-one-full-default-package)
uses that one default artifact for paired, agent/bootstrap, canonical,
usage/diagnostic and no-op/transition fixtures on an exact frozen host.
[The source guide](SOURCES.md) pins exact inputs and
keeps historical 7fb/d070 proofs separate.
[Current installed-package evidence](history/DELIVERY-EVIDENCE.md) separates the
committed API9 host's three strict hot paid repetitions and the
identity/transition/Gemini/paired gates from historical API7/API8 and dirty
API9 captures. Historical RED receipts remain intact, including an
unexplained original held38 timeout; whole-host acceptance belongs to
the parent.

**Full original Devin CLI startup parity is not claimed.** Native bootstrap
replaces the old unconditional refusals for GetUserStatus,
GetCliTeamSettings, GetAccountManagedPlugins and AssignModel with finite,
selected-account native operations. These require the matching generic
SDK 1.6 host dispatch. [The bootstrap checkpoint](history/BOOTSTRAP-EVIDENCE.md)
records actual ordinary-host launch, account operations and native turns
with a synthetic wire client, plus a separately built, unmodified Rust
reconstruction's startup and first turn. Neither is the original Cognition
binary. Only this native bundle is installed; no community plugin or
runtime helper package is required.

[Historical consolidated-host evidence](history/UNION-EVIDENCE.md) is partial: paired
native execution, installed native catalog policy, cold CLI selection and
the reconstruction's startup/first turn pass. Agent/bootstrap acceptance
was red on d070: one identical AssignModel request reached the local mock twice,
and a held bootstrap read returned 403 across a no-op binding sync.
[The identity diagnosis](ASSIGNMENT-DIAGNOSIS.md) proves unchanged module
RequestID/Session and exact host hash preimages differing only by binding
revision. [Scoped no-op acceptance](NOOP-ACCEPTANCE.md) now passes with the
original installed agent library: duplicates remain refused after ordinary
sync, a held read succeeds, and real model transitions retain their fences.
The same original-library gates also pass on committed upstream/no-op
`9e88f1098` and captured API7/API8 hosts. Historical API8 proof also uses
agent0.7.0. This does not establish all native
resume paths, original-client/live acceptance or the leader's broad suite.
Remaining generic seams are in [CONTRACT-BLOCKERS.md](history/CONTRACT-BLOCKERS.md).
SDK-only host rollout is independent of this optional Devin acceptance.
This module uses the installed public SDK, with no built-in experimental
core loader.

Experimental native Devin module for Magpie, serving both directions:
native Devin ingress, canonical Chat and scoped Messages requests execute
against the selected Devin account. Standard version **0.8.1** requires
**SDK API 1.12, native ABI 1** and
declares `inbound-codec`, `outbound-executor`, `account-auth`,
`model-discovery`, `usage-reporter`, `agent-wiring`, `module-rpc` and
`native-account-operations` and `request-policy`.

Native ingress requires the same module's executor through
`ExecutorContract()` and `SameModuleExecutor`. The canonical projection is
for host admission, budgeting and policy; actual vendor requests retain the
native protobuf. Raw native response frames return through the pinned module
generation. An incompatible route or foreign signed history fails explicitly.
See [paired verification](history/PAIRED-EVIDENCE.md) for the actual host and library
hashes; this is fixture evidence, not a live vendor compatibility claim.

The **0.2.0 inbound-only** contract remains available as `NewInbound()` and
`magpie-inbound.json`. It requires SDK 1.1 plus public codec HTTP-status
fidelity, performs no vendor requests and retains its original refusal rules.

| Adapter | Path below `/exa.api_server_pb.ApiServerService/` |
| --- | --- |
| `devin` | `GetChatMessage` |
| `devin.cli-models` | `GetCliModelConfigs` |
| `devin.command-models` | `GetCommandModelConfigs` |
| `devin.cascade-models` | `GetCascadeModelConfigs` |
| `devin.cli-models-site` | `GetCliModelConfigsForSite` |
| `devin.command-models-site` | `GetCommandModelConfigsForSite` |
| `devin.cascade-models-site` | `GetCascadeModelConfigsForSite` |
| `devin.model-statuses` | `GetModelStatuses` |

Chat replies use Connect framing; discovery replies use unary protobuf.
Discovery projects the host's caller-authorized Models list. Status discovery
returns an empty advisory list without inventing health, quota or assignment
tokens. The site variants use their individual generated request schemas,
discard credentials and client information, and refuse nonempty vendor team
selection. Host authentication, route admission, model aliases, authorization and
upstream routing remain the host's responsibility.

`module.go` registers the eight codecs through `Module.Inbounds`.
`cmd/native/main.go` calls `native.Register` from `init`; authors supply no C
exports or RPC transport. [The extraction plan](history/EXTRACTION.md) and
[digest inventory](donor-files.json) record the original eleven donor files at
commit `d6841b1`. [The continuation plan](history/CONTINUATION.md) records subsequent
generated-schema additions; current files intentionally differ from those
historical extraction hashes.

## Build and verify

Run builds and tests on `usbox` with Go **1.26.8**, a target C compiler and
`CGO_ENABLED=1`. Use an isolated HOME/XDG, pinned Go caches, `GOTOOLCHAIN=local`,
`GOPROXY=off` and `GOSUMDB=off`. The `src` module's local replacement expects a
`magpie` checkout beside this monorepo; the builder accepts an explicit `--sdk`. No dependency download is needed.

From `packages/devin` (or use the root dispatcher in the current README):

```sh
(cd src && GOWORK=off go test -mod=readonly -count=1 ./...)
(cd src && GOWORK=off go test -mod=readonly -race -count=3 ./...)
(cd src && GOWORK=off go vet -mod=readonly ./...)
python3 scripts/build.py --output /tmp/devin-distribution
python3 tests/agent_options_conformance.py /tmp/devin-distribution \
  --result /tmp/devin-agent-options.json
python3 tests/paired_http_e2e.py /path/to/frozen-test-magpie \
  /tmp/devin-distribution /tmp/devin-paired-proof
python3 tests/agent_http_e2e.py --magpie /path/to/frozen-test-magpie \
  --distribution /tmp/devin-distribution --artifacts /tmp/devin-agent-proof
python3 scripts/build.py --output /tmp/devin-inbound-distribution --inbound-only
python3 tests/conformance.py /tmp/devin-inbound-distribution \
  --magpie /path/to/test-magpie --result /tmp/devin-inbound-proof.json
```

The builder writes `magpie-plugin.json`, a platform-specific
`plugin.so`/`plugin.dylib`/`plugin.dll`, `LICENSE`, `SOURCES.md`,
`EXECUTION-FACTS.md`, `DONOR-MATRIX.md`, `DELIVERY-EVIDENCE.md` and the
variant-specific `source-lock.json`. It narrows package
compatibility to the actual build platform. Only Linux/amd64 execution has
been verified here; the other declared platforms require their own native
compiler and execution checks.

The API9 agent options conformance gate loads the actual full library.
It accepts the typed `decisionModels` catalog separately from conversation
models, validates it with the public SDK and proves that planning writes
no client files or decision-model binding values. Unknown options remain
strict errors. [Paid cache policy](CACHE-KEEPALIVE.md) documents the
`keepAlive` switch, explicit spending confirmation and bounded timing.

The conformance driver loads the actual library through all three C ABI
exports, exercises two concurrent native sessions with synthetic fixtures and
checks Connect framing, discovery and explicit refusals. It then installs
the distribution with the real `magpie plugin sdk add`, inspects it and calls
`inbound.decode`, all in a temporary HOME with Bun deliberately unavailable.
It makes no live vendor calls. Its native callback code is fixture host
plumbing, not code required from a module author.

For a manual local installation, use an explicitly selected test HOME:

```sh
magpie plugin sdk add /tmp/devin-distribution
magpie plugin sdk inspect devin-inbound
```

## Account setup

Installation does not import credentials or read another application's
session files. Select an isolated test HOME before running these commands.

```sh
magpie plugin sdk auth start devin-inbound devin \
  '{"method":"api-key","input":{"key":"YOUR_KEY"}}'
magpie plugin sdk auth finish devin-inbound devin \
  '{"continuation":{"id":"HANDLE_FROM_START"}}'
```

CLI browser sign-in uses `method: "browser"` and an explicit
`redirectUri: "http://127.0.0.1:PORT/callback"`. Open the returned URL, capture
the callback locally and submit its complete URL in
`auth finish`'s `input.callbackURL`. The module does not launch a browser.
The installed package page instead omits the explicit redirect and uses the
module-owned loopback receiver plus host poll completion described in
[onboarding](ONBOARDING.md). The verifier stays in host-private state; a bounded
CAS consumes the flow before exchange and erases retained secrets even when
the exchange becomes uncertain. At most sixteen flows are retained; expired
slots are reused. Reauthentication of an existing credential fingerprint
requires `--scope '{"account":"ACCOUNT_ID"}'` on both calls.

Models and quota come from the selected account. The SDK publishes native
Free-tier exclusions and echoes the host's usage revision. No refresh or
revoke endpoint was recovered for keys/session tokens: expiry requests a new
sign-in and logout removes the host account without inventing a vendor RPC.

`plugin sdk config devin-inbound` reads the config version. A CAS replacement
can set `{"server":"https://SERVER"}` with `--expected VERSION`.
Only HTTPS origins or explicit loopback HTTP fixtures are accepted; redirects
and returned API-host hints never move credentials to another origin.

## Supported subset and limits

[The codec reference](history/codec/README.md) lists the complete translation and
refusal rules. Ordinary text, supported thinking, images, function calls and
generation controls have synthetic fixture coverage. This is not proof of
full native Devin CLI or live account compatibility.

- Native paired requests preserve max-newlines/top-k, image captions, native
  tool attributes, cache/session references and supported signed history as
  native bytes. The old inbound-only converter still refuses controls it
  cannot translate.
- Signed replay requires receipts issued by this module after a successful
  native terminal, bound to account generation, host-derived caller identity,
  selected model, actual-model evidence when present, signature type and hash.
  Unknown, imported or cross-origin encrypted history fails before dispatch.
  Use an enabled gateway key for a stable caller identity; the module does not
  invent one when the host supplies none.
- Enum-only model selection, active native experiments/arena
  tokens, metadata team/tier/evaluation selectors and unsupported
  role/completion-count semantics remain explicit
  errors. Canonical Chat refuses signed input and native response controls
  it cannot represent; native paired replies retain those raw fields.
- The standard distribution supports the four finite bootstrap
  operations and their cascade-bound assignment receipts.
- `GetAccountManagedPluginBundle` handles Account, Org and User bundle lookup
  through the same local-owner, selected-account contract. Magpie replaces
  caller metadata with the selected account's credentials. The native client
  receives the vendor's signed URL, headers and revision, or its absent-bundle
  response. The native client owns the download.
- The SDK's 16 MiB native body budget includes the five-byte Connect envelope.
  The direct codec permits 16 MiB of protobuf data. The installed path thus
  has the smaller boundary; the codec has not been weakened to hide it.
- Native modules share the host's address space and failure domain. Closing
  a session is logical cancellation, not library unloading.

No recovered cassette, credential or account bytes are included. Donor
schema evidence remains in [codec/EVIDENCE.md](history/codec/EVIDENCE.md); current
standalone verification is recorded separately in [EVIDENCE.md](history/EVIDENCE.md).

## Inbound-only HTTP fixture

`http_e2e.py` runs the actual compiled merged host and native distribution
with a synthetic HTTP provider. The host's ordinary `serve` command reaches
`Server.Front`; this runner does not import private host packages or replace
guards with a fixture server. All application configuration, providers and
caller keys are created under a temporary HOME/XDG, Bun is unavailable,
and the gateway is pinned to its own loopback port.

On `usbox`, with the frozen build provenance recorded in `EVIDENCE.md`:

```sh
python3 tests/http_e2e.py \
  --magpie /tmp/magpie-devin-http-e2e-jqlc_i6m/magpie-01bcc7c7d \
  --host-build /tmp/magpie-devin-http-e2e-jqlc_i6m/host-build.json \
  --distribution /tmp/magpie-devin-continuation-jfmi7m1r/distribution \
  --artifacts /tmp/devin-http-e2e-fresh
```

The provenance file must name frozen merge commit
`01bcc7c7d155b029769327f909372b2ff72252e2` and the actual executable SHA256.
The runner checks it before launch. It covers guards, caller-filtered
discovery, text/usage, fragmented tool replies, tool-result images, malformed
requests, native deadlines, client disconnect and module off/on. Catalog
and lab metadata are synthetic and preseeded; partner discovery is off.
A local deny-proxy catches any unexpected external HTTP attempt and fails
the run. Only fixture-owned processes and threads are stopped, and the
temporary HOME is removed.

This is a native HTTP-to-synthetic-provider proof, not live Devin parity.
[The remaining owner/contract analysis](history/CONTRACT-BLOCKERS.md) distinguishes
ordinary Chat limitations from the existing same-module metadata/outbound
route.

## Paired native fixture

`paired_http_e2e.py` uses the ordinary compiled `magpie serve`, actual native
library and a synthetic native vendor. It exercises real SDK account
start/finish, one-use browser PKCE, models/quota, canonical JSON/SSE, raw
Connect replies and signed-origin receipts. All HOME/XDG, providers and keys
are fixture-owned; Bun is unavailable and a deny-proxy catches external
traffic. Only fixture processes are stopped.

The module-private response queue has one frame per request, a global
32 MiB response retention budget, sixteen active handles and a separate
32 MiB payload/projection budget. Private canonical wakeup chunks make the
ordinary host pump the pinned raw response; they never reach native clients
and do not replace vendor usage. Signed receipt storage is bounded to sixteen
buckets of 256 hash/identity entries; evicted history fails explicitly.

Native execution/prompt identifiers remain in the vendor protobuf. They are
not assumed to be globally unique transport idempotency keys: each incoming
HTTP call receives its own module transport identity, while the host's
accepted-attempt journal prevents automatic replay within that call.

Raw completion requires native content and an established successful stop
before a successful Connect terminal can persist origin receipts. Empty,
usage-only, incomplete, error and contradictory streams fail explicitly.
Unknown response/tool/usage controls also fail; an interrupted read retains
its cancellation cause. Known native-only response bytes and advisory
usage after the stop remain raw. Ordinary Chat still refuses stop reasons
without a canonical mapping. The generated committed ACU cost is a fixed64
double in response field 22, not a varint.
The strict-completion implementation is recorded in
[the stream checkpoint](history/STREAM-EVIDENCE.md). Historical exact d070 artifacts
and installed-host results are in [UNION-EVIDENCE.md](history/UNION-EVIDENCE.md).
The paired fixture covers all six native catalogs with key model
restrictions and aliases, denies read-only discovery/status/inference,
checks the discovered alias's scoped token budget, and cancels an expired
unary catalog request without dispatch or damage to later discovery.

[`outbound/`](history/outbound/README.md) documents the vendor client and canonical
renderer. [`outbound/HOST-GAPS.md`](history/outbound/HOST-GAPS.md) records the former
contract gaps and their SDK 1.4 resolution. The host owns routing, policy,
account generations, acceptance journaling and native library lifecycle.

## Canonical Messages and domain evidence

The provider implements strict Anthropic `/v1/messages` JSON/SSE conversion
with per-block ephemeral cache marks, real tools/results, inline images,
supported sampling and typed text/thinking/signature/tool events. Signed
continuation resolves native signature format and actual model from the
existing caller/account-generation/selected-model origin receipt. It never
defaults an imported signature to a guessed native type. Growing histories
keep their marked prefix blocks; late notices wait for real pending results.
Unknown controls, budgets/server tools, redacted blocks, unrepresentable
tool error/cache shapes and incomplete results fail explicitly.

Messages requires a nonempty **host-derived CallerID**. Earlier ordinary
7fb and captured API7/API8 hosts omit it outside matching native metadata;
their installed REDs are preserved. Scoped corrected sourcec434 projects
the trusted context and passes the unchanged ordinary `messages_host_e2e.py`,
including signature continuation, streaming and cancellation. The module
still refuses absent identity before acceptance/vendor. On that same
corrected host, mixed Chat aliases are preserved, missing real results and
unrepresentable cache boundaries are refused before dispatch, and the
non-SDK legacy repair control passes. Exact snapshots, receipts and generic
changes are in [DELIVERY-EVIDENCE.md](history/DELIVERY-EVIDENCE.md);
[HOST-CANONICAL-GAPS.md](HOST-CANONICAL-GAPS.md) retains the earlier diagnosis.
Current committed467 replay separately passes genuine Gemini
Chat/Messages JSON/SSE; its exact binary/library and other scoped gates
are recorded there without extending the earlier canonical proof.

Independent installed Chat/image/price/usage cases and raw reds are in
[DONOR-HOST-EVIDENCE.md](history/DONOR-HOST-EVIDENCE.md). SDK usage reports fresh
input separately from cache read/write; canonical Chat prompt usage remains
inclusive. Native committed ACUs never become USD. ACU percentages are
nonblocking Aside windows with explicit absence/zero handling. API9 also
projects present numeric amounts/limits, signed balance micros, units,
quota spans and plan-end resets. API7 keeps its percentage-only metadata.
API7 sparse execution facts retain measured counts, actual model/upstream,
native request ID, the actual canonical/native response ID, native stop,
committed credits and explicitly ACU-valued reported cost. Failed native
terminals retain earlier measured snapshots. Scoped code-only diagnostics
record local phase and actual status; rejection cannot change dispatch or
acceptance. [Execution facts](EXECUTION-FACTS.md) documents absent fields,
units and cancellation limits.

Adaptive/Fusion agent routers are excluded from effective API picks.
Discovered families, effort/Fast/Priority variants and source aliases resolve
against the selected account. Label-only families whose IDs violate the SDK
grammar retain concrete UIDs and display names. Remaining wire/schema and
live-client boundaries are explicit in
[DONOR-MATRIX.md](DONOR-MATRIX.md); full donor parity is not claimed.
