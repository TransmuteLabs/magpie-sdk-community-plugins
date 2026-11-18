# First-account browser authorization

The complete0.8.0 and paired0.5.0 distributions declare a root window and
`page.auth.devin` browser/poll binding. The codec-only distribution stays
unchanged. With zero accounts the page starts browser authorization through
the existing public host broker, displays a selectable safe authorization URL,
and offers check/cancel. It accepts no credential, callback URL or code input.
Private continuation, token exchange and host publication stay outside the page.

For browser start without an explicit redirect,
[`browserCallbacks`](../src/provider_auth_callback.go) reserves at most sixteen
receivers per configured component instance and binds only `127.0.0.1:0`.
The exact redirect is `http://127.0.0.1:<port>/callback`, matching the already
pinned devinapp1729055 `DevinLoginFlow`. Existing
[`outbound.PKCE`](../src/outbound/auth.go) retains S256/state/code/deadline checks.
The callback handler additionally checks GET, exact Host/path, bounded request
and no body; invalid/duplicate callbacks are refused without exchange. Its
static response has no callback data, disables caching and external resources,
and instructs the user to return to Magpie.

The callback is retained only in private instance memory, never written into
the sign-in flow or public page view. Existing host-private sign-in state
contains PKCE and an opaque receiver/instance identity. Before callback,
[`authFinish`](../src/provider_auth.go) returns Pending. After callback it uses
the same durable exchanging CAS, clears verifier/credentials before the single
exchange, and closes/joins the receiver before dispatch. Concurrent finish
cannot exchange twice. Uncertain exchange never replays. Explicit CLI
RedirectURI/manual callback and API-key paths keep their previous behavior.

The receiver joins server/connection goroutines on completion, deadline and
component disposal. Reconfiguration creates a new immutable component instance;
a receiver cannot be adopted across instances or recovered after process loss.
Start a new flow after that loss. Host cancellation prevents further poll,
exchange and account publication. The public SDK has no module-cancel callback,
so a canceled host flow's receiver remains bounded until its ten-minute deadline
or component disposal; immediate module notification is not claimed. Host
revocation still owns stale page/configuration, discovery policy, selected
authority and final account publication, including exchange already in flight.

The builder includes metadata, HTML/JS/CSS and the existing public SDK URL
presentation helper. Page disposal cancels pending/late host flows. No new
SDK API, dependency, account selector or private account-store reader was added.

## Bounded verification

The author packet is `/tmp/magpie-devin-first-account-20261116`. Named missing
page and browser-start-without-redirect regressions fail on the predecessor;
the same probes pass on the successor. The unchanged actual GUI HTTP probe
starts with zero accounts and covers pending/early polls, invalid callback,
callback without exchange, first account/single finish, foreign parent/cancel,
stale configuration, discovery off before start and during held exchange, and
package disable before completion. Focused Go controls include invalid
method/host/path/query/code, callback input injection, duplicates, sixteen-flow
bound, expiry, uncertainty, concurrent finish, stale instance, immutable
configuration, component stop, real receiver timer and partial TCP teardown.
Existing manual PKCE/API-key, scoped execution and donor/history controls are
retained. Exact commands, race20 counts and terminal results are in packet
receipts; setup failures are separate from behavioral reds.

All local runtime uses the parent Mac guard, isolated HOME/XDG/TMPDIR, pinned
offline Go1.26.8 and no live data. This proves actual GUI HTTP/broker and module
handlers in the exec fixture plus a native Darwin distribution build. It does
not prove a rendered browser, a new native load or live vendor sign-in.
Independent Astra review and parent combined-source acceptance remain separate.
The earlier sealed `.44`/bounded92503f96 and whitespace P2 packets remain intact.
