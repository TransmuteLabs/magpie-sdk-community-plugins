# OpenCode module migration

The starting point is local HEAD 031d093, provider manifest 0.2.0, API1.3.
Do not describe that checkpoint as agent/UI 0.4. Keep its provider constructor
and reviewed four-model catalog working unchanged.

## Coherent local PR scopes

1. Agent profile/config/launch/Library: new agent module and focused fixtures.
   Profile explicitly targets builtin `opencode`; it never claims that ID.
   Write only a declared private overlay, passed as `OPENCODE_CONFIG`.
   Reuse existing Library target paths/formats from the adopted agent.
   Host snapshots, CAS binding, backups, shell wrappers and recovery remain
   host-owned. Add ordinary real-host connect/set/launch/disconnect proof.
2. Auth window and distribution: extend the agent module with a real page,
   selected-account read binding and `AuthBindings` device poll declaration.
   Use the public SDK workflow through the actual bridge; credentials and
   continuations stay server-side. Package all browser imports with a tested
   compiler/bundler, never publish source-only dangling SDK imports.
3. Acceptance/docs: actual native provider+agent+window in isolated HOME,
   synthetic endpoints, one-use device flow, refresh and all four model paths;
   no replay, stale authority, cancel/off, original config and Library safety.
   Each slice has its own version/manifest/constructor; evidence identifies
   exactly which variant was built and exercised.

## Donor authority and bounded implementation choices

- `../magpie-devzeus-archive/internal/agent/agents.go` openCodeLike/opencode:
  fields model/small; explicit Magpie provider; model catalog/options.
- Current owner `../magpie/internal/agent/air.go`: OPENCODE_CONFIG is a supported
  merge overlay; retains user's original plugins/agents/MCP settings.
- Current donor `internal/agent/opencode_v2.go` and opencode_providers.go:
  v1 provider/options/npm versus v2 providers/settings/package;
  small_model versus agents.title.model; prompt-cache settings.
- Library `internal/library/targets.go` targetOf("opencode") uses actual agent
  config directory, AGENTS.md, same config MCP and skills, plus Claude fallback.
  Explicit profile with no replacement Library metadata reuses that owner.
- Public SDK API1.5 AgentLaunch/AgentProfile, checked snapshots and binding CAS;
  no private Magpie imports in this repository.

The overlay declares v2 format by default. An explicit module config choice
can select v1; the plugin does not probe a live installed CLI or infer version
from its filename. It retains user-supplied entries and original overlay bytes
under the host's checked before/previous snapshots. Unknown/corrupt settings
fail before a plan is returned.

The first implementation checkpoint was Go OAuth, agent/profile and window.
The 0.5.0 window extends it with provider-bound Zen OAuth/key/public, native
four-protocol transport and bounded Console HTTP inspection/watch primitives.
No live vendor/credential access, host edit,
dependency, publication or push. Build/test only on usbox. The shared development
and acceptance host is SDK1.6 at
freeze tree `f536d8202e40aae4b0fc36630aa3c1eb496d58d5`, whose SDK/runtime
equals `d07078c7926daf6f33af09c40297c941bd39da92`. Provider minimum API1.3 and
agent/window minimum API1.5 stay unchanged; these cuts do not declare Source1.6.

## One installed package

The ordinary no-flag build, root manifest and native entry point produce one
module ID, `opencode`, with every ready Go/Zen/agent/UI component. Provider-only
0.2.0 and headless-agent 0.3.0 constructors remain explicit compatibility/test
artifacts. They are historical alternatives, never additional installations.
The complete package bundles its Go library and compiled browser assets.
It must not install another SDK module,
community package, OpenCode auth plugin or Bun backend. The native OpenCode
application itself is the agent executable, not an auth companion.

The clean-HOME acceptance must record the single SDK catalog entry, absence of
legacy plugin-auth/migration/package state, native provider/auth execution in
the gateway PID, and no Bun/node backend child. History through the built-in
reader or a manual vendor UI is not plugin Source acceptance.

## Zen and identity boundaries

Read-only donors:

- `../magpie-community-plugins` HEAD `57d1cc6a9f49389d495ea9be8d986c70de7767bf`,
  `packages/zen-free` 0.1.27 (`provider.mjs`, `models.mjs`, stream and tool
  fixtures). This is evidence, not a runtime dependency.
- Archived Magpie `opencode_oauth.go:isOpenCodeOAuth` supports both
  `opencode-go` and `opencode-zen`; `opencode_endpoints.go` and
  `opencode_go_free.go` distinguish their gateways.
- Native OpenCode source reviewed at fetched dev
  `38bd73789a43bf043599d6196af96583228035d7` (2.3.0); checkout remains on
  `83960f8a56b2f2b8ad0b751f23baaf9d0eb65ad9`,
  `packages/core/src/plugin/provider/opencode.ts` enables the public identity
  only when no key/Console connection is selected; paid models are disabled
  without a key. Console supports device OAuth and service-account API keys.

The complete module needs explicit identities, not a single credential object
that changes its meaning when a model is unavailable:

| Provider domain | Account identity | Model authority / transport |
| --- | --- | --- |
| OpenCode Go | user + initial organization, selected current organization and generation | Go catalog, Go API paths, Go quota |
| OpenCode Zen authenticated | separate provider-bound OAuth or explicitly imported service key | Zen catalog and per-model Chat/Responses/Messages/Gemini protocol |
| OpenCode Zen public/free | one declared public identity, no personal credential | vendor-listed eligible free models only; no Go quota fabrication |

Keep all three inside the same packaged module. A Go credential must never be
forwarded to the public gateway, nor should a refusal silently grant another
provider/account. The archived Go-to-Zen retry is evidence of a real use case;
any cross-domain fallback needs an explicit account/plan policy, not an
unannounced identity switch inside one execution.

Completed bounded cuts in the 0.5.0 window:

1. Pure reviewed Zen catalog/protocol adapter, synthetic vendor responses and
   literal stream/tool fixtures. Preserve dynamic retirement/free filtering and
   native protocol routes. Do not advertise SystemOne as conversational chat.
2. Provider-bound Zen OAuth/public/key identity handlers with separate account
   hashes, G checks and credential-isolation regressions. Key input uses the
   SDK1.5 auth binding/receipt path, never ordinary business input.
3. Extend the one auth window with explicitly labeled provider/account plans;
   one-use flows/cancel/replay tests remain per selected provider. No second
   package installation.
4. Five-second Console config and independent one-minute instruction/skill
   caches, response-header presence gates, same-workspace last-good retention,
   sanitized policy inspection, safe versioned skill download and HTTP watch
   list/poll/receipt-bound ACK. These are explicit actions, not an automatic
   donor configuration/policy/session runtime.
5. Selected-public-only Chat/Responses/Messages tool normalization and reply
   guard from pinned zen-free0.1.27: stand-in schemas, collision-safe refused
   names in declarations/history/choice, caller-name restoration and
   unambiguous name folding. Fragmented Chat calls preserve IDs/arguments;
   nameless calls become notes and unknown named calls remain visible.
   Personal Go/Zen wire behavior is unchanged. Literal helper differentials
   do not establish live public access.

## Storage checkpoint and remaining module features

The parent ACKed the separate `session-storage-source` contract at API1.8.
Storage-owned host files provide root consent, finite family grants,
source-only logical list/read and publication revocation. The module now
owns the actual Unix SQLite/WAL transaction, V1/V2 migration precedence,
ordered `session_part`, instruction epochs and selected media hydration.
Literal donor schemas and separate-writer/native-host tests exercise this
reader; Python is fixture-only. The exact already-host-locked SQLite runtime
is recorded in `go.mod`, `go.sum` and `source-lock.json`.

The ordinary 0.6.0/API1.8 full constructor, native entry point and manifest
include the reader after parent acceptance of the rooted-storage source.
The no-flag build requires no history/window/agent selection or companion
package. Installation grants no root or account-derived storage rights;
ordinary host session-read consent and revision-checked root binding apply.
Linux has actual native/WAL runtime proof. Darwin rooted Openat and POSIX
lock/VFS source passes arm64/amd64 build/vet, with overlapping real WAL reader
and independent-writer sibling tests on Linux. This does not prove native
Mac loading or lock interoperability; other platforms remain unsupported.
Rendered consent/media UI, actual Mac runtime, custom database/channel
selection and complete donor replay remain separate readiness work.
Source1.6 JSONL and native account operations retain their existing contracts;
neither is used to impersonate a storage grant. Actual direct-call import is
parent-owned and is not established by normalized session summary usage.
Current implementation, proof and the archived proposal are in
[HISTORY_CONTRACT.md](HISTORY_CONTRACT.md).

The actual native OpenCode harness can retain its Console layers, policy,
skills and session watches without these Magpie exports. The generated
`magpie` inference provider preserves that native ownership, as checked by
`agent_console_test.go`. It supplies no native Console credential: shared
sign-in would need a separate selected-account handoff. The module's HTTP
inspection flag does not set the native experimental watch flag.

Other optional Magpie product boundaries are separate from storage:

- A connection/workspace-scoped remote config/policy owner with precedence,
  revocation and enforcement. `console.snapshot` inspects allowed fields and
  normalizes known policies; it does not apply them to routing or tools.
- Checked publication/retraction of remote instruction and versioned skill
  content into discovery/Library, with workspace ownership. Existing local
  Library launch/restore acceptance does not establish remote publication.
- Session lookup, synthetic deduplicated delivery, deleted-session cleanup
  and bounded resume/wake with prompt/window counters. Current account actions
  do not establish the donor's session authority. Poll/ACK receipts neither
  wake a session nor advance a durable cursor automatically.

These are optional Magpie exports, not prerequisites for native ownership
or claimed defects in an existing promised SDK contract.
This module imports public SDK packages only and makes no host workaround.
[HOST_GAPS.md](history/HOST_GAPS.md) gives the exact official source and public SDK
evidence and ranks the concrete browser owner gap and conditional shared
sign-in bridge. Session-bound create/delete, WebSocket-first watch lease
recovery and automatic ACK/delivery remain owned by the native client. A
separate Magpie implementation would need host session authority.
The new `console.watch.types` action independently provides bounded native
type inspection under the existing selected-account/header/flag gates.

The ordinary 0.7.0 package also completes public forced upstream streaming,
Chat usage, caller-selected JSON collapse or SSE and optional explicit
environment facts. Personal Go/Zen and Gemini preserve their native path.
63 literal donor cases supplement the existing 99 tool cases. JSON collapse
requires native completion and preserves exact numbers; no retry, account
fallback or cut-short success synthesis is introduced.
Skill validation now accepts native optional fields, quoted and literal/folded
scalars and scalar metadata while preserving original bundle bytes. Full
YAML remains a module limitation: no parser dependency is added, and unsupported
syntax is refused. Candidate physical-host verification passed on exact tree
`36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, with actual owner, stock storage
and API9/API8 agent proofs. Parent full acceptance/review and any future host
commit remain separate. The current source/package/binary receipts and all
earlier failed attempts are retained in EVIDENCE.md.

These unfinished integrations are not advertised by the ordinary production
manifest and do not block the standalone Magpie SDK's first release.
Storage is declared by the ordinary API1.8 package on every platform;
unsupported rooted access fails explicitly rather than disappearing.

Zen's stand-in tools, tool-name normalization, environment blocks, headers,
version updates and truncated-stream handling are behavioral donor evidence
to review individually, not copied as an unexplained fetch wrapper. Preserve
the calling agent's real tool identities and make unsupported semantics
explicit. No live free-tier or billing claim is established by offline tests.
