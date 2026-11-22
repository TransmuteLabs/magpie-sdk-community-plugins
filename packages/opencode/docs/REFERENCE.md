> Relocated package reference. Commands below use the package root; new checks are recorded only in [RELOCATION.md](migration/RELOCATION.md). Prior acceptance identities remain historical.

# magpie-sdk-plugin-provider-opencode

The [request-policy contract](REQUEST-POLICY.md) describes the current
acceptance and refusal behavior. `REQUEST-POLICY-VERIFICATION.json` records
the original adoption. [SOURCES.md](SOURCES.md) records the current source review.

One native OpenCode package for Magpie. The ordinary build includes Go and
Zen OAuth/service-key/public providers, native Chat/Responses/Messages/Gemini,
Console HTTP actions, the checked agent/profile, its authentication UI and
the actual SQLite/WAL history reader. It uses public SDK imports and the exact
already-host-locked Go SQLite runtime.

The full package needs **SDK API 1.12, native ABI 1 and `hostAPI: ^1.12`**.
The SDK supplies request recovery policy, effective agent context/output/effort
metadata, and the prior model snapshot used to update generated config without
overwriting user edits. The provider-only compatibility artifact also uses
the SDK transaction that serializes refresh-token exchange and durable
credential publication across host processes.

There is one ordinary package with module ID `opencode`. It bundles provider, native
agent controller and compiled browser assets in a single package. It does
not install a community/OpenCode auth plugin or run a Bun backend.

| Distribution | Version | Minimum SDK | Contents |
| --- | --- | --- | --- |
| Ordinary full package | 0.8.4 | 1.12 | Providers sign-in, agent wiring, Go + Zen OAuth/key/public, Console HTTP bindings and a Sessions panel for SQLite/WAL history |
| Previous release | 0.8.2 | 1.12 | Providers sign-in plus the standalone OpenCode page |
| Full compatibility artifact | 0.8.1 | 1.12 | Explicit legacy `--compat-api8` flag with the previous window-only sign-in declarations |
| Provider compatibility artifact | 0.2.0 | 1.12 | Historical Go OAuth, four models, Chat/Gemini, quota |
| Agent compatibility artifact | 0.3.0 | 1.12 | Historical provider plus declared profile, config planner and launch metadata |

Update Magpie to a build that supports the `sessions` panel slot before
installing or updating to 0.8.4. The Add and Update paths validate panel
metadata before activating the package.

The panel supports rendered history through the optional
SDK UI1.10 `session-history-v1` feature. It requires a selected root and a
trusted parent confirmation for the independent root/read gates, and renders
only issued session/media selections. Account sign-in grants no history
rights. The interface shows an explicit unsupported state when the host
does not offer the optional history feature. Bun is a build tool for
the classic browser bundle, with no Bun backend or runtime dependency.
[HISTORY_UI.md](HISTORY_UI.md) records the exact contract and proof boundary.

The preserved 0.7.0 renewal adds public forced streaming and caller-shaped JSON collapse,
explicit environment facts, scalar skill frontmatter and watch type inspection.
It passed 377 module race tests/subtests, seven causal controls and actual
single-package native host/SQLite/WAL/media/agent replays on candidate physical
tree `36f8e629b1092b089d4d59e0f9d8473a1f602ff8`. Parent full acceptance and
review remain separate; no future host commit is substituted.
[EVIDENCE.md](history/EVIDENCE.md) records exact source/package/binary hashes and
preserved RED history. [HOST_GAPS.md](history/HOST_GAPS.md)
distinguishes native Console ownership from optional Magpie exports. Native
OpenCode retains organization config/policy, skills and watch delivery with
its own active Console connection and native watch flag. Magpie OAuth alone
does not create that native connection. The new browser history adapter has
its own source/overlay and installed-package receipt, separate from that
preserved 0.7.0 acceptance.

The preserved 0.6.1 metadata repair passed installed ordinary native connect,
sync and model edits on exact host tree
`013b3ab2d4cb863c3150af3110a377718402f38b`: default API9 with V1 and V2
config, and the explicit API8 V2 variant. Effective context and API9
limits/efforts survive, independent user output/variants remain intact,
and unchanged sync preserves bytes. [EVIDENCE.md](history/EVIDENCE.md) retains the
old-code assertion failures, fixture-only key-order correction and exact
synthetic receipts. This is not real vendor-client acceptance.

The preserved 0.6.0 full-owner/history replay uses physical tree
`8fe78ab9df13d733d61462eacce19bff523c1e58`, based on `c11acbc2`, with
the exact source archive and supplied binary in `source-lock.json`
`hostAcceptance`. That API1.8 package passes ordinary native owner
and SQLite/WAL HTTP routes on this API1.9 host. This physical tree is not
attributed to a future host commit. The previous committed467 replay and
storage proof over `ae957e5a8` remain separately inventoried in EVIDENCE.md,
SOURCES.md and source-lock.json.
Earlier SDK1.6 and opt-in storage evidence remains archived in EVIDENCE.md.

The ordinary build is `python3 scripts/build.py --output <empty-destination>`. Its full
constructor, native entry point, emitted/root manifest and source lock all
include history and agent metadata at API1.9. No history flag, `--window`, `--agent` or another
package is required. Installation does not grant history access: ordinary
host session-read consent and revision-checked root binding are required.
Darwin rooted access/VFS source passes arm64/amd64 CGO0 build/vet on usbox,
with actual POSIX WAL sibling tests on Linux. Actual Mac native loading and
lock interoperability remain unproven. Unsupported platforms retain the
declared source and explicitly refuse rooted access. Rendered storage
consent/media requires the negotiated browser feature. The standalone SDK
rollout is independent.

The historical 0.6.0 window distribution passed the complete ordinary native
lifecycle on the exact host source in [SOURCES.md](SOURCES.md), including launch, Library,
reload/config/package replacement, off/on and schema-aware disconnect.
Verification uses synthetic endpoints and an executable fixture, not a live
vendor binary. This bounded slice does not implement all OpenCode features.
See [MIGRATION.md](MIGRATION.md) for exact Console, public request/tool and history limits, and
[SOURCES.md](SOURCES.md) / [source-lock.json](source-lock.json) for exact
vendor, donor and tested-host revisions.
The implemented history grant, literal SQLite/WAL evidence and archived
proposal are in
[HISTORY_CONTRACT.md](HISTORY_CONTRACT.md). Ordinary gateway usage/requests/trace
and isolated host logs are collected separately from historical import.

The agent planner writes effective host context and output limits for own,
other-provider and group models in both OpenCode config formats. API9 effort
lists generate native variants; unknown lists remain unspecified and explicit
empty lists remain empty. A valid host default becomes the pinned schema's
model `settings.reasoningEffort` (V1 `options.reasoningEffort`). These are
configuration projections, not proof of a live OpenCode process's pinned
runtime effort. `priorModels` distinguishes previously generated values from
independent local output/settings/variant edits. Decision models are validated
as a separate public envelope and are not inserted into this chat catalog.

## Contracts

| Contract | Module responsibility |
| --- | --- |
| `account-auth` | Go/Zen device OAuth, selected OAuth refresh, explicit Zen key/public authentication and local logout |
| `model-discovery` | Go and Zen native catalogs with selected-account availability and packaged fallbacks |
| `outbound-executor` | Go Chat/Gemini and Zen Chat/Responses/Messages/Gemini, accepted before inference dispatch |
| `usage-reporter` | Go's rolling, weekly and monthly percentage windows; no fabricated Zen quota |
| `network.proxy` | Resolve the effective parent proxy for each vendor request |
| Private state CAS | Device-flow continuation and one CAS claim per poll |
| `session-storage-source` | Actual SQLite/WAL and legacy JSON projection through consented finite rooted grants; no arbitrary path/SQL or account-derived storage rights |

The SDK owns native registration, framing, account selection/generations,
operation receipts, refresh serialization, persistence, cancellation and
backpressure. The author calls `native.Register`; there is no author C/RPC
shim, vendor implementation in the host, module refresh lock, implicit token
retry or module-owned account file.

## OAuth and account boundaries

The host fixes provider/account/generation and supplies credentials in
`Call.Auth`. Refresh parameters cannot substitute credentials, even with a
copy of the selected credential object. Account ID derives from the product,
user and initial organization; rotation can change the current organization
without changing the account's identity.

The module makes one refresh exchange and returns replacement credentials
and absolute expiry to the host. It never publishes them itself. A definite
preflight rejection is `not-started`; a failed/malformed exchange is
`uncertain`. `invalid_grant` at HTTP 400 reports `auth_revoked` and asks for
sign-in. Vendor bodies, descriptions and token values are absent from errors.
The host retains the durable fence/receipt and decides whether another
attempt is permissible.

Chat and usage reject credentials at or within the donor's 30-second expiry
margin. API 1.3's ordinary provider adapter can renew before dispatch using
the persisted expiry and then select the published generation. Direct raw
SDK execution must supply the current generation; the module does not
refresh inside execution or quota reads.

Device continuations expose an opaque nonce; the device code remains in
private state. Pending and slow-down results permit a later poll. A successful
exchange is one-shot. A crash between device exchange and host publication
still requires a new sign-in: the refresh transaction does not make initial
device authorization crash-resumable.

In link-only mode during device sign-in, copy the complete HTTPS address
and open it in a separate browser tab to confirm the displayed code. The auth
form clears the address when the flow ends, is canceled or is dismissed.
The compatibility artifact's sign-in page runs in a scripts-only sandbox.

## Models and native protocols

Go loads `/inference/go/v1/models` for the selected OAuth account. Its
Chat and Gemini routes use the same snapshot to decide which models can
execute. The snapshot belongs to the account, organization, and credential
generation. A successful empty list disables inference for that selection
until discovery returns models again. Failed refreshes retain the last
successful list. A cold worker can fetch the list before executing a
dynamic model. Go discovery admits the native Chat and Gemini endpoints
with verified context and output limits. It leaves Messages and Responses
models out until their Go transports are supported.

Before a successful discovery, the following four models provide a
packaged fallback from the reviewed `opencode-go-models-2026-10-31.json`
snapshot. When a live list contains only model IDs, it supplies availability
for these verified entries.

| Model | Protocol | Input modalities |
| --- | --- | --- |
| DeepSeek V4.1 Flash | `chat` | text, image |
| GLM-5.2 | `chat` | text |
| Kimi K3 | `chat` | text, image, video |
| Gemini 4 Argon | `gemini` | text, image, audio, video, PDF |

The Go transport preserves original native JSON and streamed response bytes.
Chat uses `/inference/go/openai/v1/chat/completions`; Gemini uses
`/inference/go/google/v1beta/models/<model>:generateContent` or
`:streamGenerateContent?alt=sse`. Gemini signs with the selected token in
`x-goog-api-key`; chat uses Bearer auth. Foreign credentials, cookies and
organization headers are removed. Attribution headers are replaced across
all header-name casings. The HTTP transport owns gzip negotiation.

Chat requires its terminal `[DONE]` event and closes a held stream once it
arrives. For OpenCode Go, an explicit Chat streaming request recognizes
SSE sent under a different MIME type and forwards the original bytes with
the SSE media type. Both Go and Zen Gemini SSE require terminal reasons for every
requested candidate, or a prompt block, followed by EOF at an event boundary.
The relay validates each event before forwarding it and preserves valid
preceding events regardless of HTTP read sizes. It keeps usage events after
the last finish reason. In-stream
error envelopes, malformed events and truncated streams return `uncertain`,
including after a finish reason. An individual Gemini event is bounded to
1 MiB and the module tracks at most 1,024 candidates.
Non-success HTTP statuses preserve their response status/body.

Zen accepts both the old ID-only inference list and the richer models.dev
records. Live metadata requires the official native URL for a supported
conversational protocol and positive context and output limits. A reported
input cap can reduce the published context. A rich public entry needs
explicit zero input and output prices. GLM 5, 5.1, 5.2 and GLM-5.3-Flash
Free are excluded from the Zen bootstrap and every subsequent listing.

Configure an explicit `projectId` (an OpenCode project hash or `global`), or
supply an explicit project header. No home directory or Git configuration is
inspected. The reviewed client target is pinned to OpenCode 2.3.0 at the exact
revision in `source-lock.json`; the 2.2.5→2.3.0 protocol review is in
[SOURCES.md](SOURCES.md).

## Explicit limits

The provider-only and headless agent distributions serve OpenCode Go.
The window distribution adds distinct `opencode-zen` personal OAuth/key and
`opencode-zen-free` public identities. Native Chat, Responses, Messages and
Gemini routes come from the pinned v2 catalog; the remote model list controls
availability with a one-minute account-generation cache. A valid empty list
retires models; a failed read retains the last good list or pinned bootstrap.
No refusal switches account/provider. Zen usage returns no fabricated Go quota.
Zen balance, `countTokens` and remote operation reconciliation remain absent.

Service keys enter the authentication broker through the declared `key`
method in Providers. The compatibility artifact uses its OpenCode page.
Both forms use a secret input.
Key identity is derived from that key; replacing it creates another account,
and reauthorization with a different key is refused. OAuth identity remains
user + initial organization within its provider domain. Public auth accepts
no personal credential. Zen Free can be activated from Providers without reauthorization.

Personal Zen has selected-account actions `console.snapshot`, `console.skill`,
`console.watch.list`, `console.watch.types`, `console.watch.poll` and
`console.watch.ack`. Config/list
loads are bounded to five seconds; instruction and skill lists have independent
last-good caches. Header presence, including an empty header value, enables
lists; watches additionally require `consoleWatches: true`. A watch page's
ten-minute receipt binds ACK to that exact account generation and the entire
delivered page, once. Cursor advancement/delivery remains the caller's job.
Skill downloads preserve signed redirect queries, strip credentials permanently
after crossing origin and accept safe paths, optional name/description, quoted
and literal/folded scalar frontmatter and scalar metadata. The original skill
bytes are retained. Event types retain their object input schemas; descriptions
are normalized and bounded to 300 UTF16 units without splitting a character.
Full YAML syntax, automatic session wake/WebSocket transport,
policy enforcement and remote config/Library publication are not implemented.
The snapshot is an inspection result, not an enforced organization policy.
Inspection uses a module-specific User-Agent. The actual native OpenCode
client keeps its Console config/policy/discovery and session-watch owners;
the Magpie overlay preserves their configuration. Native Console sign-in and
its experimental watch flag are separate from this module's HTTP actions.

Personal Go/Zen native bodies and replies pass through unchanged. The selected
public Zen identity applies the pinned community tool compatibility rules for
Chat, Responses and Messages: five explicitly unavailable stand-ins, refused-name
aliases in declarations/history/choice, caller-name restoration and unambiguous
case/separator matching. Fragmented Chat calls retain separate IDs and arguments;
nameless calls become explicit notes. Unknown named calls remain visible.
Buffers and positions are bounded, and native errors/completion remain strict.
Public requests force upstream streaming and Chat usage while preserving the
caller's JSON or SSE shape. JSON collapse follows tool restoration and requires
native completion; errors and cancellation publish no partial JSON. Buffers
retain the 32 MiB reply/1 MiB event limits and exact JSON integer lexemes.
No retry or account fallback is added. Public FreeTierError/limit responses
remain visible.

Optional module config `publicEnvironment` supplies the public request's real
facts explicitly. All five fields are required when configured:

```json
{
  "publicEnvironment": {
    "directory": "/fixture/work",
    "workspace": "/fixture",
    "git": true,
    "platform": "linux",
    "temporary": "/fixture/tmp/opencode"
  }
}
```

The module never probes home, Git or filesystem state for these facts. With
no configuration it adds none, and an existing native environment block is
preserved. Temporary-directory preference grants no tool or filesystem
permission. Personal Go/Zen and Gemini remain outside this public policy.
Offline fixtures do not establish live public access or vendor billing acceptance.
The SDK1.5 variants implement an explicit `opencode-sdk` profile targeting
the existing OpenCode application, checked private `OPENCODE_CONFIG` overlay
and model/small fields. They preserve user settings and request host-owned
launch/Library behavior. The window uses the actual parent bridge and
selected-account auth workflow; tokens and continuations stay server-side.
The installed OpenCode binary remains the agent.

SDK agent plans preserve custom model entries in the private overlay for both
OpenCode config formats. When a host model leaves the catalog, the next plan
removes its previously generated fields and retains local overrides. Sync
updates unchanged generated fields and keeps manually edited fields within
each model. An empty catalog clears stale owned model selections. An
independent selection and local model entries remain in the configuration.
Disconnect restores saved owned values and refuses a malformed model catalog.

OpenCode uses the default model from the checked overlay. The launch descriptor
sets `OPENCODE_CONFIG`, leaving the client's explicit `--model` arguments
available. A binding written by an earlier development package with the
`--model` launch descriptor must be disconnected using that package's
descriptor before adopting this one.

This profile still writes its model list during sync. Dynamic discovery in
Magpie's built-in OpenCode wiring remains a separate path.

Own plugin history uses the default API1.8 host-selected storage grant described
in [MIGRATION.md](MIGRATION.md): the Unix reader consumes actual JSON/SQLite
with WAL, ordered `session_part`, instruction epochs and media blobs.
Linux runtime evidence and Darwin source/static evidence remain distinct.
Actual Mac runtime remains unproven.
Source6 JSONL, the built-in reader and manual vendor UI do not satisfy this
contract.
The host may translate a client protocol into a model's declared native
protocol before calling this module.

Multiple organizations without an explicitly selected token organization
remain unsupported: reproducing the donor's locale collation would require
an additional dependency. Unknown models, protocol/model mismatches and
ambiguous endpoint/query variants fail before dispatch. Peak pricing is
omitted where the SDK price DTO cannot express donor multipliers.

## Build and verification

Build and test only on `usbox`, with isolated HOME/XDG, pinned caches, Go
1.26.8 and offline module resolution. The distribution directory must be empty:

```sh
python3 scripts/build.py --output /tmp/opencode-dist --sdk /path/to/magpie
/path/to/sdk-host plugin sdk add /tmp/opencode-dist
/path/to/sdk-host plugin sdk config opencode '{"projectId":"global"}' --expected 0
```

`--expected` is the current config version; `0` is for the first write.
After enabling the host's ordinary session-read consent, inspect and bind
the selected root through the normal CLI. Listing grants no read authority:

```sh
/path/to/sdk-host sessions sources list --json
/path/to/sdk-host sessions sources bind opencode opencode-history \
  --root data --root-ref <inspected-root-reference> --expected <package-revision>
```

Use the exact reference/revision returned by inspection; consent, root,
package, module/config changes and cancellation are rechecked before
publication. No live installation or consent change is performed by these docs.
The ordinary no-flag build includes every ready component; there is no second
package to install. Historical regression artifacts use explicit
`--compat-provider` or `--compat-agent`; their versions/minima remain archived
in the table and evidence. Full builds use the installed TypeScript
compiler and copy the public browser SDK's relative ES modules. The pinned Bun
build tool bundles the iframe entry point; no Bun process is needed at runtime. The package has no npm dependency, sidecar or bare
browser SDK import. Every distribution
includes its active manifest metadata in `source-lock.json`, `SOURCES.md` and
the exact reviewed fixture records in `donor-provenance.json`.

See [PLAN.md](history/PLAN.md) for the historical provider extraction and
[MIGRATION.md](MIGRATION.md) for current work. `tests/native_refresh.py`
builds a test-only native entrypoint and exercises an actual SDK host against
a synthetic loopback OAuth server. It never uses live vendor accounts.
`--ordinary-renewal` additionally drives the ordinary provider test command
with an expiring account before any explicit refresh.
`--ordinary-renewal --all-models` also tests all four packaged models through
the host's ordinary provider path, including Gemini's native endpoint/key.

See [EVIDENCE.md](history/EVIDENCE.md) for the completed gates, source inventory and
the regression exposed by the ordinary host path. Native CLI and package
fixtures are recorded separately; a
cross-build is not live vendor parity or native execution on that platform.

`tests/full_e2e.py` exercises the window distribution through the actual
native host, served parent/page bridges and ordinary HTTP routes. On Linux
amd64 it requires `tests/loopback_guard.c`, a fixture-only seccomp supervisor:

```sh
gcc -std=c11 -Wall -Wextra -Werror -O2 -o /tmp/opencode-loopback tests/loopback_guard.c
/tmp/opencode-loopback /tmp/opencode-network.json \
  python3 tests/full_e2e.py /path/to/sdk-host /tmp/opencode-dist /tmp/opencode-proof
```

Use `--provider-signin-only` to install the native fixture through Plugins Add
and exercise the four declared sign-in methods through Providers. The check
refreshes models for the selected accounts and reads Go's quota after
reauthorization. Zen and Zen Free return no quota windows.

Run these commands only on `usbox`, under the isolated build environment above.
The supervisor executes approved IPv4 loopback socket operations itself and
denies external TCP, UDP, IPv6 and io_uring. Active capabilities are zero and
`no_new_privs` is set; the ordinary nonzero bounding mask is recorded. It uses
no user namespace, so system library ownership remains truthfully visible.
It is a network guard for trusted fixtures, not a sandbox for untrusted code.
The hidden-root userns reload refusal remains an expected strict native
authority negative; no loader fallback, restart or retry is used to pass it.
