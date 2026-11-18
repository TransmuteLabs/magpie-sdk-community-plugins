# China login and unlinked Start checkpoint

2026-11-14. Continuation of SQLite checkpoint
`835a75efa31241ffd0e0dbc2593a51b05c6db291`, checked against local vendor
`fec975905a2abd4c07e35c40d51fa097a2258ec9` (desktop 3.18.2 / CLI 0.17.1).
Headless 0.6.2 and UI 0.8.2 retain native ABI 1 and their respective
SDK 1.4 / 1.5 floors.

## Behavior

Automatic sign-in may publish an independently confirmed Start identity
without linked paid material. Missing/null material never grants paid
access; malformed present material is refused. A different family's paid
material is not borrowed. Selected reauthorization retains its original
vendor user, account and plan, including when automatic selection would
otherwise discover Start.

Explicit `loginProvider: zai_cn` is a BigModel login channel. It survives
selected renewal without changing the stable account identity and cannot
authorize another family. The production control page offers that channel.
Wire version 3.18.2 accompanies these source-grounded semantic changes.

## Frozen evidence

Evidence root: `usbox:/tmp/magpie-zcode-sqlite-jF2jd6Ay`.
The sorted, indented JSON executable/test/package inventory
`cn-final-source.json` contains 100 files, SHA-256
`cdc3fb857678d4a4a6c117b6070d50d8e4175e7309431b53fc86afe578ebe35f`.
All 100 local files were checked byte-for-byte against that freeze before
this commit. Documentation added after the freeze does not alter it.

`cn-red-results.json` records compiled old-code assertion failures for
unlinked Start, entitlement/selected-plan fences and the China channel.
`cn-green-results.json` records:

- Formatting and `go vet ./...`: exit 0.
- Full `go test -race -count=20 ./...`: 4820 test pass events, no test
  failures or skips.
- Native headless and UI packages, including strict TypeScript: exit 0.
- Node and Bun control-page tests: 29 passes each.
- Actual native default auth, China/unlinked-Start auth, controller and
  one-bundle UI/HTTP/native fixtures against SDK host
  `831c3d74af9358fa8a45d77eed902fe7cd02a076`: exit 0.

Tested host binary SHA-256:
`45d10c98019d7a258c27d80d333ef0ae6ca7c538c76c9dd286b82da9590455b1`.
Headless ZIP SHA-256:
`2b4d52157f898759872986e9dc42f615796cbf90b0a0bc53d6abc4cd9408a4a7`.
UI ZIP SHA-256:
`9d3f0ecfb49d13cce022cb6690f98754dc41551fba73ba928c5315e815703994`.

Builds and tests used usbox, isolated HOME/XDG, existing pinned caches and
offline dependencies. Final full-suite acceptance used an isolated
loopback namespace. No native root-ownership check was weakened.

## Preserved diagnostic and acceptance limits

Two unexpected `/` requests previously reached host-visible billing
fixtures. Their source remains unattributed. The diagnostic red log is
retained as `cn-race.log.diagnostic-red`; strict unexpected-request
assertions remain in place. The isolated pass does not establish or fix
their cause.

Historical untracked `AUTH-WEB-EVIDENCE.md` is preserved byte-for-byte
outside this commit, SHA-256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
Its older multi-consumer proof is not the identity of this CN checkpoint.

No live vendor/account/application, rendered-browser, actual ZCode client
startup, new platform runtime or publication is claimed. Owned
AgentWiring and the concrete community gift/claim/dual-trial and automatic
capture gaps remain described in ONE-BUNDLE-AUDIT.md and
COMMUNITY-PARITY.md.
