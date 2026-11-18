# Extraction plan

The user released the migration hold after the upstream merge at host
`01bcc7c7d`. Only this repository is writable for this lane.

1. Inventory archived codecs, account isolation, admission and bug fixtures.
2. Lock ordinary inference behavior with synthetic fixtures before adapting
   it to the public native SDK.
3. Separate `protocol/` codecs, account selection, HTTP transport and native
   `cmd/` registration. Do not copy Magpie's internal provider/gateway code.
4. Implement selected-account ordinary Messages inference without automatic
   fallback, replay, credential borrowing or live-app discovery.
5. Retain explicit unsupported errors for operations whose host guarantees
   have not been exported. A caller-supplied options object is not admission.
6. Verify SDK framing, native build, cancellation, errors, scope and tool
   behavior on usbox under isolated HOME/XDG; no vendor traffic.

This is an implementation slice, not a claim of complete ZCode parity.
Outstanding host contracts are recorded in CONTRACT-BLOCKERS.md.

## Native controller continuation

Vendor journals move into this repository. API 1.3 already owns credential
publication, routing, budgets and lifecycle. `Start` provides a surviving
runtime context and disposer. Use an explicit private `dataDir` configuration
until the host supplies its default module directory.

1. Add a bounded, atomically replaced journal with an OS cross-process lock.
2. Derive a stable vendor-user principal from the session-authenticated profile.
3. Derive lane admission from declared model aliases and selected Frame.Auth.
4. Persist pending admission before vendor I/O; preserve unknown outcomes
   across operation IDs and credential generations. Do not auto-retake.
5. Keep ticket/card/reset/claim codecs and reconciliation vendor-specific.
6. Expose explicit settlement through a declared write action if generic
   `operation.cancel` is not yet journaled by the host.

## Headless auth and discovery lane

The first controller checkpoint is committed locally as `c8acfa2`.
Follow AUTH-DISCOVERY.md. Keep SDK14-ADOPTION.md staged independently until
the generic host contract is green. UI/MCP and legacy SQLite import remain
separate from this authentication/catalog lane.
