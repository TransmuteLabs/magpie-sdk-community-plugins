# ZCode readiness continuation

2026-11-14. Module baseline `6cb7873`, host
`f4232ea762bf881759eba37ac1c9ea7581b627d9`, clean when inspected.
No host edits, live vendors/accounts/apps, dependencies or publication.
Optional UI advances to 0.8.0; the headless 0.6.0 runtime floor stays 1.4.

## Confirmed defects and fixes

- Current public `ui.js` imports `account-selection.js`, which imports
  `errors.js`. The old builder copied only `ui.js`, so its actual UI
  package failed at Bun's unresolved import. Build the actual public
  browser entry as ESM first, then bundle the application as an IIFE.
- Failed account listing, selected read or selected write left the prior
  selection usable in the control client. Three old-code assertions fail
  on that retained generation. Failure now clears cached accounts,
  selection and result; a fresh list and explicit selection are required.
  List validation also enforces canonical uint64 strings.
- The accepted host auth broker was only consumed by temporary fixtures.
  The production optional variant now declares fixed `page.signin` →
  provider `zcode`, method `browser`, mode `poll`, with explicit
  reauthorization. The real page consumes the public SDK workflow.
  Editable Team fields apply only to new sign-in; reauthorization uses
  the exact existing generation and original vendor selection.

The page exposes explicit new sign-in, same-account reauthorization,
manual poll and cancel. It locks business actions during authentication
and refuses authentication during confirmation/write. Lost start replies
cannot become another exchange. Completion refreshes the account list
without selecting its result. Disposed pages cannot reload accounts,
render late results or unlock business controls.

## Exact source and artifacts

Proof root: `usbox:/tmp/magpie-zcode-readiness-azNr9B5g`.
All 91 executable/fixture/metadata files match local source.
`readiness-source-inventory.json` SHA-256:

`95d82678170941a1f904342af65d07534a23fae380bc82f56f940702a004fb79`

| Artifact | SHA-256 |
| --- | --- |
| Host source archive at f4232ea76 | `69280c15d505bb2aae2b2412abf77e4480d8251dedea75f6f7ecaee44ac26cff` |
| Actual host binary | `2c15325e88d437a293b4f603dc6eac720e9436d3ba669ad5199f08574f9c6cfa` |
| Linux headless ZIP | `14ba402d7193fa303fdf29092554a9b3acbd7978d890c5ab6f1eb4634c22a30c` |
| Linux UI ZIP | `3c18a94ef4273e42f61fab38bfbf9f37b30eeb5e3462d46e7a87e76e6d19eb9a` |

## Terminal verification

All gates ran on usbox with Go 1.26.8, temporary HOME/XDG and pinned
offline caches. Final gates used a UID-preserving loopback-only network
namespace with capabilities dropped.

- Module race count20: **3900 passing test/subtest events**, zero failures
  or skips. Formatter output empty, vet passed.
- Headless and UI native packages built; actual native ABI proof passed.
- Strict TypeScript compilation passed during the final UI build.
  Node and Bun each passed **28 tests**, with zero failures/skips.
- Darwin arm64 and Windows amd64 CGO-disabled vet/build passed.
  These are compile gates, not native runtime acceptance.
- Actual host controller, auth/discovery/history and selected control-page
  E2Es passed on host f4232ea76.
- The production packaged IIFE ran in a frame VM through the actual
  served bridge and persistent parent relay, ordinary HTTP, host Store
  and native vendor controller. Setup CLI used **add only**.
  Three vendor starts issued three tokens; only two generations were
  published. The third exchange was held, canceled twice and then
  released after page disposal. No G3, late account reload, renderer
  update or exchange replay occurred. **34 crossings, 16 HTTP calls**.
  Credential/Continuation/private canaries were absent from public
  messages and HTTP bodies. This is not rendered-browser acceptance.

Results: `final-results.json`, `ui-final-results.json`,
`native-ui-auth-final.log`, `node-ui-final.log`, `bun-ui-final.log`.

## Independent review and preserved red evidence

Leader's bounded read-only review found no confirmed blocking code
defect and requested held finish/cancel plus disposal regressions.
Those cases now pass in both engines and the actual native/UI proof.
The bounded review is not a blanket live-vendor approval.

Old-code failures remain `old-package.log`, `old-authgrant.log` and
`old-invalidations-corrected.log`. The initial regression harness omitted
TypeScript rootDir and failed compilation; only its corrected,
assertion-level three failures count as the control regression proof.
The first unrestricted-loopback race run received unrelated GET-root
requests in an older cancel fixture. Final isolated race20 is separate
green evidence, not a fabricated product fix for that observation.
The first new UI HTTP fixture recreated its parent owner per message,
so the host correctly refused the changed parent before vendor polling.
Its corrected persistent-owner fixture is the final passing proof.

## Remaining behavior

Paid inference, refresh, discovery, principal journals, reset/claim,
durable cancellation, fixed remote image search and manual lossless
history conversion retain their prior scoped proofs. Other recovered
MCP tools need their actual endpoint/schema inventories, not a fabricated
dynamic registry.

SQLite native history and archived financial journals are **not JSONL**.
Their DB/WAL snapshot, preservation and migration work is specified in
SQLITE-INTEGRATION-PLAN.md. Automatic Source6 integration awaits its
accepted snapshot contract. Live resume still needs recovered execution
control and selected journal admission. Rendered browsers, live vendor
parity and native macOS/Windows execution were not tested.

Read-only log audit inspected 4086 lines/571435 bytes from app.log;
944 contained ZCode, 634 matched auth-failure patterns and 13 cancellation
patterns. No payload, credential or raw message was printed. These are
historical aggregate matches, not evidence that the external module
caused those failures.
