# Contract boundaries and remaining work

Ordinary inference uses the public selected account/generation, the host
operation journal, cancellation and backpressure. Paid PAT renewal uses
the API 1.3 host-owned refresh owner and atomic Auth/expiry/generation/receipt
publication. That contract is now sufficient for one vendor PAT exchange;
this module requires API 1.3 so an older host cannot admit it.

Vendor reset/card/claim/session rules belong in the native plugin, not the
host kernel. The plugin now owns durable cross-process journals and
reconciliation under explicit private `dataDir` configuration. A module
data directory is a useful generic host facility, not a hard migration
blocker. Startup is pure; initialization happens in an authorized call.

The session-authenticated vendor profile supplies the billing principal.
No vendor-user identity field is needed in core. SDK component lifetime and
disposal already suffice. `state.cas` includes account generation and is
therefore not used for vendor-user journals that must survive refresh.

SDK 1.4 adoption passed module gates and independent scoped host review.
The local checkpoint depends on unreleased SDK 1.4. Native E2E proves durable business cancellation and
the cross-process invalidation/deferred/fresh-read barrier. A generic policy
regression denying AccountAuth continuation CAS was reported and fixed by
the host owner. Both native auth and controller E2Es passed against the
corrected immutable preview; see SDK14-EVIDENCE.md. No scoped host blocker
remains for these flows. Combined SDK acceptance is a separate host gate.

Generic host integration status:

| Primitive | Evidence and safe current behavior |
| --- | --- |
| Default module DataDir | ModuleContext does not yet supply one; explicit private config works. Future opt-in must not mutate live state during candidate validation. |
| Window model applicability | 1.4 Models/NotModels/Aside consumed. Named best spendable Start buckets control each exact model; redundant/pending windows are informational and unknown totals remain unknown. |
| Selected-account cache invalidation/read barrier | 1.4 usage.invalidate runs before reset/claim POST. The module holds its principal journal lock across usage reads and defers while mutations remain unsettled. Native preview proves dirty state survives separate CLI processes. |
| Durable operation.cancel | 1.4 cancel uses a new write intent and the original target/reference; native preview proves one settlement across two intents without target replay. |
| AccountAuth continuation CAS | Narrow generic host policy fix verified by actual native init/poll/finish/reauthorization. Read-only handlers keep their state-write prohibition; no vendor implementation entered core. |

This module never follows the installed ZCode app's account, reads its
secrets, chooses another account, retries a vendor mutation or
falls back from an accepted lane to ordinary paid inference.

Headless init/poll sign-in, selected-account Start reauthorization and
dynamic entitlement/config discovery now use the existing 1.3 contracts.
The 1.3 flow has no generic host blocker. Recovered adapters do not
implement silent Start JWT refresh; explicit same-account reauthorization
is available instead of an invented token exchange. Selected paid renewal
binds the original project/key before any PAT issuance.

Standalone history readers/conversion, a real manual SessionMigration
file-plan adapter and headless owner MCP tools are implemented. Native
preview/apply/host-backup/restore verifies the adapter; no automatic agent
reader/resume route is declared. Remaining vendor/product work is separate
from those generic primitives: actual-client resume/automatic consistent DB capture,
native MCP UI and reviewed import
of archived SQLite journals. A lost Highspeed draw without its card ID
cannot acquire a new card/task binding from the recovered health endpoint;
the module preserves uncertainty or requires explicit abandonment.

Caller lane headers and arbitrary native metadata are not authority.
Admission derives from the declared selected alias and verified Frame.Auth,
then the plugin's journal and authenticated vendor reply. Generic host
routing, budgets, account publication and lifecycle remain authoritative.

## Historical UI scope gap (closed in the 0.8.0 variant)

The current window binding constructs Scope with Plugin alone. It cannot
pass selected Frame.Auth/generation to the existing account-scoped
controller. SDK 1.5 needs sanitized own-module account discovery, an
explicit host-validated account-scoped binding and a host-owned UI auth
broker. This gap does not block headless MCP codecs or offline history
parsers. Exact source evidence, security regressions and the bounded
remaining domain sequence are in DOMAIN-CONTRACTS.md. No CLI/host-secret
workaround is implemented. The isolated next SDK now has an early selected
binding/account-summary contract, and an unbundled TS control prototype is
typechecked against its actual public browser definitions. Its eight client
tests do not establish final SDK 1.5 acceptance or an auth broker.

That paragraph records the earlier SDK boundary. Host `f4232ea76` now
provides selected-account actions, the poll/link auth broker and workflow;
the optional 0.8.0 page consumes them. See READINESS-EVIDENCE.md for actual
production-IIFE/native proof and SQLITE-INTEGRATION-PLAN.md for the
remaining DB/WAL capture and financial-import work. No host blocker remains
for the implemented page flow.

## Current full-package boundary

The ordinary 0.11.0 package includes provider/controller/agent/UI and fixed
official remote MCP execution; a second runtime or plugin is unnecessary.
Selected-generation offers and explicitly confirmed check-in are page
actions. Native logs and SQLite usage rows can be inspected and bundled
losslessly from explicitly supplied bytes. These implemented surfaces do
not require a core change or unfinished SDK 1.7 telemetry.

Actual ZCode execution is still unverified. Native account-operation
descriptors already exist in committed SDK 1.6; the module does not yet
claim the actual client's stdio/control wire or model-bound launch.
Automatic scheduling, SQLite/WAL-consistent capture and rich quota cards
have distinct generic needs described in LEADER-CONTRACTS.md and
SQLITE-INTEGRATION-PLAN.md. The host owner's optional API 7 Usage.Facts
and structured diagnostics contracts address execution telemetry; exact
frozen-base adoption and its module acceptance remain pending. No host
patch is made or requested as a condition of plugin-free SDK deployment.
