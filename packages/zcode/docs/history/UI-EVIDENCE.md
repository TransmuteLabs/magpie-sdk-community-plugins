# Selected-account control-page evidence

2026-11-13. Separate native module 0.7.0, SDK API 1.5 UI consumer,
ABI 1. Default headless module 0.6.0 remains SDK API 1.4.
Neither SDK variant is claimed to be published or installed.
The MCP/default-4 checkpoint is commit `7410acd`.

## Implementation

`window.go` declares five fixed-provider page bindings and the native
`execution.summary` read action. Summary results remove ticket/card
values before crossing into the page, without changing headless
`execution.list`. The host selects Frame.Scope/Auth/current generation.
The client cannot choose a provider, supply credentials or retarget a
stored financial intent.

`web/control.ts` promotes the original typed prototype; the prototype
entry re-exports it. The real HTML/CSS/TS page explicitly selects saved
accounts, reads execution/plan/reset status and offers reset/claim.
Generation remains a string, stale results are discarded, writes are
single-flight and uncertainty never triggers automatic retry.

Confirmation uses an HTML dialog with explicit button handlers, not
`window.confirm` or a sandbox-blocked form. `build.py --ui` uses installed
tsc and Bun as author tools to produce a classic IIFE. The existing host
page bridge and script are inlined by the actual revision-checked server.
There is no ES-module/CORS load, extra server or Bun runtime worker.

The page contains no auth buttons or copied auth store. Native controls
still require the separately configured private dataDir. The auth broker
remains separate host work.

## Frozen provenance

Proof directory on usbox:
`/tmp/magpie-zcode-ui15-UmC0ES4r`.

80 Go/Python/module/browser/declaration files were individually compared
against the local source. `source-hashes.json` SHA-256:

`39b1a177a5aee52cb30d56727b8537c8f9f192f0fbf5e525a752c44036f27b7f`

Host source is the immutable git archive of approved selected-account UI
commit `c9dd41e129fc0d1dc6e7ca7fa10d299ef0a97cef`, built on usbox.
No host source was edited by this module lane.

| Artifact | SHA-256 |
| --- | --- |
| SDK 1.5 host binary | `449af6f902bd33b1a9c898061286174e4b2c7eca0d0f57fd51819377421c7b86` |
| UI Linux amd64 ZIP | `9d89d5e5e943b633737dd9a5cf43b7874aab72d9ac210688f32a62772841940b` |
| Default headless Linux amd64 ZIP | `6016eef8296829164bcfeb8eae033fd0f841541ca6cd702fb55776a0e2a53880` |
| Actual public ui.js | `081d0ed40e2cccaf57c1440ee6a2e9c756c006793d10c97ca7171b9436c44604` |
| Actual public ui.d.ts | `1fa03ca694b04b3d3e6116201b24df52d1faf476c4d347741078626845454905` |

The native fixture overlays only the entry point's vendor destinations.
Auth publication, account scope, window grants, operation journal and
the native module's controller remain real.

## Constrained verification

All builds/tests ran on **usbox**, with temporary HOME/XDG and pinned
Go 1.26.8 caches, GOPROXY/GOSUMDB off. No live vendors, accounts, app
changes, new dependencies, push or installed activation.

The final behavior gates ran in a new loopback-only network namespace:

```sh
unshare --user --map-current-user --keep-caps --net sh -c \
  'ip link set lo up && exec setpriv --bounding-set=-all --inh-caps=-all --ambient-caps=-all python3 -u -'
```

UID/EUID/GID stayed 1000. CapInh/CapPrm/CapEff/CapBnd/CapAmb were all zero;
a mode-000 fixture read raised PermissionError. The initial root-mapped
namespace experiment is not counted as permission evidence.

- `go test -json -race -count=20 -timeout=15m ./...` passed:
  **3860 passing events, 193 distinct tests/subtests**, no failed or
  skipped tests.
- Actual isolated native **web** host passed checked HTML/classic
  bundle/bridge/CSS serving, safe account enumeration, two selected
  accounts reaching the correct native profile, plan/reset reads,
  one claim and one uncertain reset.
- Same financial intent with duplicate/changed account/changed input was
  refused. Unbound, forged, foreign, stale-generation and undeclared
  bindings were denied. Refresh required explicit generation reselection;
  disabling the module revoked its old page ref.
- Page responses contained no fixture session/business/PAT credentials.
  Native summary leakage has its own regression.
- Headless controller/MCP E2E and five consecutive headless
  auth/discovery/manual-history E2Es passed in the constrained namespace.
- The current source's headless controller/auth E2Es also passed against
  the earlier real SDK 1.4 host
  `bf11f37b125b4a6b59ba3c8501b323cc98b9556bad55ed1af3b8fa477092efca`.
- Strict TS compile and **17/17** state/DOM/actual-compiled-IIFE tests
  passed. They cover exact generations, stale enumeration/read behavior,
  late-read/write ordering, dialog cancellation, disposal during
  confirmation, fixed arguments and no uncertain retry.
- Empty-output formatting, vet, native packaging and default headless
  ABI proof passed. The default ZIP contains only its API-1.4 manifest
  and native library. Darwin arm64/Windows amd64 CGO-disabled vet/build
  passed.
- Three separately compiling/launching behavioral negative mutations
  failed the intended tests: native ticket/card leakage, stale-account
  result acceptance and numeric generation precision loss.

Logs/results: `usernetns-results.json`, `usernetns-*.log`,
`frozen-final-results.json`, `ui-tests-frozen.log`,
`ui-negative-results.json`, `oldhost-results.json`,
`final-proof-summary.json`, `negative-*.log`.

## Independent bounded review

Mencius closed the frozen consumer review with **COMMENT, no blocking
code findings**; no established critical/high/medium/low defect.
The independent copy matched all 80 hashes before and after its probes.
Its private usbox namespace retained UID/EUID 1000 and all five capability
masks zero.

Independent Go variant/summary race tests passed, strict TS passed,
Node and Bun each passed all 17 compiled state/page/app tests. Removing
only native ticket/card cleanup in a private overlay caused the named
summary regression to fail at its assertion, not compilation. The
reviewer also inspected the author's actual native web-route evidence.

Report and machine-readable results are locally in
`/tmp/magpie-zcode-ui15-independent-review/REVIEW.md` and
`review-results.json`; its private remote artifact root is
`usbox:/tmp/magpie-zcode-ui15-review-0o86oysj`.
No LSP diagnostic tool was exposed and gopls was unavailable; compiled Go,
race, vet and strict TS checks are recorded explicitly instead of a
formal LSP-gated approval.

## Historical red and limits

An earlier shared-loopback headless auth run received an unexpected
`GET /`; one of five diagnostic repeats reproduced it. Those runs remain
red in `headless-auth-final.log` / `auth-repeat-0.log`. The source contains
no vendor GET-root operation. No sender attribution or production fix is
claimed. Network isolation and the final five constrained passes are
separate evidence, not a rewrite of those historical results.

DOM/IIFE tests and real HTTP/native route tests do **not** establish
rendered-browser acceptance. No browser executable was available on
usbox; no new browser dependency was installed. macOS/Windows native UI
runtime, live vendor parity, auth-broker buttons, dynamic MCP publication,
SQLite import, automatic agent-reader integration and live resume remain
outside this slice. This is a consumer-module proof of the approved
selected-account UI cut, not a combined SDK 1.5 release claim.
