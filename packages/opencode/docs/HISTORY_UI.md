# Rendered history, 0.8.0

The accepted pre-implementation design remains in `HISTORY_UI_PLAN.md` at
commit `5cf5084432d9a365f04316186c944ca77a42f7f8`. This implementation uses
the actual host `sdk/js/ui.js` and `ui.d.ts`, never ambient invented methods.
The ordinary/native minimum stays API1.9; browser UI1.10's optional
`session-history-v1` feature is negotiated separately. API8 compatibility
keeps its native minimum. An older host displays unsupported and makes no
history request.

`web/history.ts` is independent of account workflows. It inspects
`opencode-history` owned by native agent `opencode` (declared root slot `data`,
native root selector `opencode.data`), presents only issued
finite root choices, asks the trusted parent for access, lists bounded
session pages, reads normalized transcripts and displays selected raster or
plain-text media. Release revokes exactly the selected delegation and keeps
ordinary OpenCode read permission. The wiring profile `opencode-sdk` is not
the storage owner.

The six SDK methods are `inspect`, `requestAccess`, `release`, `list`,
`read`, and `media`. Each selection carries the issued current `viewRef`.
Root selection uses `rootRef`, exact release uses `consentRef`, a session
uses `sessionRef`, and media additionally uses `readRef` and `mediaRef`.
The host owns package/config/root and persisted-consent CAS; an account,
caller path, URL or iframe confirmation cannot replace that authority.
Partial access failure displays the actual returned independent gates.

Stored text/model/names are text nodes. The first preview accepts raster
data images and inert text; SVG, HTML, audio, video and other types explicitly
have no active preview. No stored remote URL is fetched. Visibility,
selection changes and disposal clear retained transcript/media and suppress
late replies. One metadata revalidation runs at a time while visible at a
two-second cadence, independently of catalog notifications.

`scripts/build.py` type-checks against the actual selected SDK source, copies its
browser runtime files, and uses the existing pinned Bun build tool to create
a classic `app.browser.js`. This lets the existing checked host inliner
deliver the UI behind the cookie sandbox without relative module fetches or
CSP changes. Standalone compiled modules remain available for unit fixtures.
There is no community install, companion process or new dependency.

Verification packet: `usbox:/tmp/magpie-history-v1.FwJugSor`.
Development first used physical source4b9
`4b97b537667e680da22f9418eb35daa49f9be56f`; the current base is the supplied
closure source `94ec4c8de4e5b366a6e3fb854df2176007587ab8`.
Host feature overlays and their binaries are inventoried separately from
that immutable supplied base and from a future shared negotiation commit.
Preserved RED includes the old host's missing history route, unavailable
GTK default build, a projection fixture correction, and browser environment
failures before rendering. Command/result/log/network and source audits
remain in the packet. Final gate receipts distinguish synthetic DOM,
actual installed native SQLite/WAL/media and Chromium rendering from actual
OpenCode execution, live accounts/vendors and Mac runtime acceptance.

The Chromium fixture loads the installed package, actual page bridge and
actual parent bindings through authenticated host HTTP. Its small synthetic
parent reuses the actual cancel-default `confirmAction` function; it is not
a full Magpie application GUI audit. Native Console connection ownership
remains client-owned. No shared single-sign-in or remote policy publication
is introduced.

The final isolated feature source is candidate8, physical tree
`2c3ea47814211d7d0b818eeb074168fe8f83ec16`; the16-path overlay SHA256 is
`73d6c72fb5b7b9608660a88bc6550a310e7c619e417b3a72a15e85c74cb54287`.
It excludes concurrent API10 `AgentResume` helper authoring, which remains
in the shared working source. Both Astra findings are corrected: the actual
settings-backed OwnOff lock path, and external settings changes during CAS
owner validation. Their old-source failures are retained.

The bounded acceptance covers38 Go race test/subtest events, five parent
fixtures, four SDK neighbors, six module UI fixtures,377 module race events,
vet/format, default/API8 strict packages, seven causal controls and installed
Chromium with actual PNG decode. Older supplied API9 has a separately passed
installed unsupported-state replay. `history-receipt.json` links exact
sources, commands, failures and refreshed final distribution artifacts.
`history-contract.json` provides the negotiation/schema packet for the
leader's shared SDK1.10 integration. That final combined source still needs
its own acceptance; no unfinished feature implementation is deferred to it.
