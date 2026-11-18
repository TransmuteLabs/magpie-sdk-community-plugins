# SDK 1.5 selected-account state regressions

The original local prototype was promoted into `web/control.ts` for the
ordinary full native distribution. This directory retains its public
re-export, fixed binding fragment and state regressions. It imports only browser SDK types from
`@magpie/extension-sdk/ui`, never host auth JSON or a native secret store.

`ZcodeControl` supplies the account/control state layer for the TS page.
It calls `backend.accounts(binding)` and
`backend.call(binding, input, {account: {id, expectedGeneration}})`.
Generation stays an exact string. Provider is fixed by the staged binding
declaration, never a client argument.

The state layer rejects unlisted selection, discards stale enumeration/read
results, clears selection when the saved generation changes, serializes
writes and never retries an uncertain reset/claim. No sign-in/refresh/logout
UI is implemented: the host-owned auth broker is a later contract.

`bindings.json` mirrors the selected bindings for the SDK 1.5
composition; `window.go` owns the actual manifest. Ordinary `build.py`
packages provider/controller/agent/UI together. The explicit
`--headless` SDK 1.4 build is a historical compatibility fixture.
No installed activation or publication is implied.

Verification must run on usbox: typecheck/compile against the copied actual
SDK 1.5 `ui.d.ts`, then run Node's built-in test runner on
`control.test.mjs` with `ZCODE_UI15_BUILD_DIR` pointing at the compiled ES
module directory. The fake bridge tests exercise client state behavior.
Actual native web routes and constrained verification are recorded
separately in ../../UI-EVIDENCE.md; state tests alone do not establish
browser rendering or a combined SDK 1.5 release.
