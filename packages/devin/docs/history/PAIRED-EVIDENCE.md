# Native paired Devin verification

Date: 2026-11-13. Target: `usbox`, Linux/amd64, Go 1.26.8, cgo enabled.
Version 0.3.0 registers native ingress and the outbound/account provider in
one module. All builds/tests used isolated HOME/XDG, pinned existing Go
caches, `GOTOOLCHAIN=local`, `GOPROXY=off` and `GOSUMDB=off`.
No Bun, live vendor, developer credentials or production gateway was used.

This page retains the frozen paired 0.3.0 verification. The optional SDK 1.5
agent distribution and its additional compatibility runs are recorded in
`AGENT-EVIDENCE.md`. Its original-client bootstrap boundary is separate from
the resolved account generation bug. Current paired 0.3.1 and SDK 1.6 agent
checks on the consolidated d070 runtime are in `UNION-EVIDENCE.md`,
including their different acceptance results.

## Initial paired checkpoint inputs

The initial paired host is an exact archive of committed SDK checkpoint
`0dc09a544f85dbe378ebae11858245b916f00a43`, following core SDK 1.4 commit
`28a4403f9`. No dirty host overlay, SDK 1.5 UI/agent work or baseline app-test
fix is needed by this module. This is a local checkpoint, not a published
SDK/plugin release.

| Input | SHA256 |
| --- | --- |
| Committed host source archive | `bd8328e69e0dc046c0f3d8561a974f0be2c3a20eaea7d9382ab5451ded8654ff` |
| Compiled ordinary host | `ad7dc555d44bb5bb6900661011009365f0d83f67a54d110d0df0cdd9c64707d8` |
| Module runtime/test inventory, 42 files | `7b96d6cc1c6cc7802f68dc6017ccfc51d042978db86b6b065f0153b9d7094aac` |
| Full native module library | `c5eba27c120e89434e0708a85b29498d15b8f7cc50d735b3a36ec30f81bd9733` |
| Optional inbound-only library | `76f61512ef54541618ce366f0e07f2c655b374087d07dd2a6669b65acabb417e` |

The module inventory covers every `.go`/`.py` file, `go.mod` and both embedded
manifests. Its hash is SHA256 of the path-to-SHA256 JSON map serialized with
sorted keys and compact separators. Documentation is excluded to avoid
self-referential evidence hashes. Tested remote bytes and local source
must match that inventory before committing.

Vendor semantics use the generated sources and installed plugin described in
`outbound/EVIDENCE.md`. An additional check of `module-98224.js:12047`
confirmed native Metadata: force-team 22, tier impersonation 29, team 32 and
evaluation 33 are account/control selectors, not disposable client labels.
The recovered file SHA256 is
`ef66a64bdce5ec10bf1eb34d8fe9d7927527aab7d24cb742c3d658c2906aeb10`.
Active selectors and unknown metadata fields fail explicitly. Original
credential metadata is discarded; only the selected SDK credential is sent.

## Completed gates

| Gate | Terminal result |
| --- | --- |
| `go test -json ./...` | Exit 0, **429 PASS events** |
| Complete module `-race -count=3` | Exit 0, **1281 PASS events**, no race report |
| Auth/PKCE/bridge/origin/client/render/raw regressions `-race -count=20` | Exit 0, **423 PASS events** |
| `go vet ./...`, `gofmt -l` | Exit 0; no diagnostics or formatting differences |
| Full and inbound-only C-shared builds | Exit 0 |
| Inbound-only direct C ABI and actual SDK installation | Exit 0, two concurrent native sessions, all three ABI exports |
| Inbound-only HTTP regression against frozen host `01bcc7c7d`, preceding SDK snapshot | Exit 0, **11 checks**, six inference requests, no external traffic |
| Initial full paired HTTP fixture with restart-mode generation check, three fresh sandboxes | Exit 0, **54 checks**, **39 native inference requests**, no external traffic |

PASS counts include tests, subtests, fuzz seeds and package results. No timed
fuzz campaign or non-Linux native execution is claimed. Dependencies were
only the standard library, this repository and the public Magpie extension,
native and protocol SDK packages.

The paired fixture starts the ordinary compiled `magpie serve` and reaches
its normal Front guards. It installs the actual `.so`; `/proc/<gateway
pid>/maps` and the installed library hash prove it is mapped in that host
process. It does not run a special gateway or a Go/Bun backend worker.

The eighteen checks in each paired run cover:

- API-key auth and persisted browser PKCE through actual host state CAS;
  wrong callback state is refused and a consumed continuation cannot exchange
  again. Explicit scoped reauthentication keeps account identity.
- Selected-account model discovery, native caller-authorized discovery,
  unauthenticated/read-only denial, actual SDK usage revision and native
  Free-tier quota exclusion.
- Raw native text, exact zero output usage, fragmented signed thinking and
  opaque Gemini bytes; native tools, inline image and zero sampling controls.
- Fragmented native tool replies and historical tool-call/result IDs survive
  the ordinary host translation. Tools and their associations also remain in
  the canonical admission projection; that projection is not vendor input.
- Ordinary Chat JSON/SSE through the same native outbound executor, including
  explicit zero output usage.
- Hash-only origin receipts issued before successful terminal delivery;
  known replay succeeds, unknown/cross-caller replay makes no vendor request.
  Rejected origins do not disable an otherwise usable account.
- Account generation revocation across a fixture gateway restart, while a
  new unsigned request still succeeds.
- Original native vendor error terminal, malformed input rejection, deadline
  terminal and actual upstream cancellation on deadline/TCP disconnect.
- Same-module execution denial for a foreign HTTP provider and module-off
  denial. Temporary homes, gateway process groups and fixture threads close.

The private canonical wakeup stream drives the ordinary host's response
pump. It is not vendor input or a substitute response: native clients receive
only the queued raw native frames. The fixture checks that wakeup bytes do
not appear in native output.

Native signature receipts establish local provenance, not cryptographic
validity. Vendor verification remains vendor-owned. Receipt eviction, unknown
origin, missing caller identity or unrepresentable controls cause refusal.
No foreign encrypted history is silently stripped or relabeled.

## Regression sensitivity

Two independent temporary Go overlays deliberately restored unsafe behavior:

- Setting a historical `AsOf` on the live quota result makes the epoch
  freshness assertion fail. The real handler omits it so SDK 1.4 can confirm
  the captured revision; deferred reads cannot clear a dirty epoch.
- Removing the caller match from origin receipts makes the cross-caller
  rejection assertion fail.

Both negative commands exited 1 at the intended assertion. They did not
change the tested source tree and are not counted as passing gates.

## Resolved: live separate-process account replacement

**The strict default `generationMode=live` fixture is now green.** Host
commit `a721fd647c016f857c965f3b61c72a1b5cdb6084` fixes generic discovery and
pre-dispatch handling of a manually replaced SDK account. The module runtime
remains the accepted `cd9992c7fa0bb167ea38d3cf99909c848156335f` source; it does
not read private host account files or bypass generation fencing.

The verified host was built from main
`541410c164e0b45a2311658d2c1f19570045dbce` plus the eight-file fix.
Every one of those eight source hashes matches committed `a721fd647`.
The intervening `163ffd02e` changes only a validation ADR; this is a
source-equivalence check, not a claim that an additional binary was built
from the later Git archive.

| Strict-live input | SHA256 |
| --- | --- |
| Eight-file host patch inventory | `cecbab80c4f442a69b9d789790529d13111f0d4f30b7ad645a54aa5d35e0c8bb` |
| Compiled ordinary host | `b957e5132676b38b155638c727b5b3375d1e16657a0c25abf73084641deb72e8` |
| Production native module library | `2bc4db657e447a3bbd78d588ed8014e49c8ddc8889266fab6355a7578a877bb9` |
| Diagnostic native library, separate Auth-frame proof only | `4557d06d233fe1010b32c1888489d28683eca3ec46b7ff0ac047f7b400d73c1b` |

Three production-library runs kept the gateway alive across a separate SDK
CLI reauthentication: **54 checks, 39 native inference requests, zero
external requests**. Old signed history was refused in **0.265, 0.260 and
0.331 seconds**; new unsigned work succeeded without restarting the gateway.
Each run preserved all **11** captured old-generation journal entries
unchanged. A fourth, explicitly instrumented run confirmed the first unsigned
request returns HTTP 200 and six execution frames contain actual generation
**G3** and the expected newly stored synthetic Auth. Its old signed-history
refusal took 0.360 seconds. No vendor credentials were logged or used.

The runner owns only matching methods, native RPC paths and per-run
credential/PKCE nonces. Two ambient GET `/` probes in the second production
run were excluded from effects and counts; no external daemon was changed.
Every fixture gateway stopped and its sandbox HOME was removed.

Primary cause was captured before the fix on main `240aab8e9`: a stale
locally selected generation became repeated core HTTP 502 errors and entered
`gateway.pauseUntilReaddressed`. Ordinary canonical Chat reproduced the
same wait. A paired-codec-only canonical 409 test with no executor dispatch
passed race x20, excluding raw-frame pumping as the primary cause.

The host fix observes account metadata and portable file identities outside
the global provider-cache mutex. Ordered observations prevent delayed old
metadata publication. An undetected in-place rewrite remains an optimistic
stamp miss, but authoritative preflight returns terminal not-started and
invalidates discovery. The fallback, atomic replacement preserving metadata,
accepted/uncertain/completed/replay fences and prior journal identity have
regressions. Manual successors still cannot be adopted as proven refreshes.

Scoped host race x20 passed **780 test-run events**, with no skips or
failures; four deliberately broken overlays failed at their intended
identity/publication/fencing assertions. Linux native builds and Linux,
Windows and Darwin vet passed; cross vet is not native platform execution.
The full extension package race passed. **The full provider package race
remains non-green** and is tracked separately; no whole-application green
or SDK release claim follows from this bounded fix.

The earlier strict failure on `0dc09a544` remains historical evidence:
exit 1, nine preceding native inference requests and zero external traffic.
Its `strict-live-generation-result.json` and logs are retained. The earlier
`--generation-mode restart` result proves persistence only and is not used
as the live-fix proof above.

## Evidence locations

Initial paired checkpoint remote root:
`/tmp/magpie-devin-sdk14-committed-nk497w7p/` on **usbox**.

```text
source.json
tests.log
race.log
repeat.log
vet.log
format.log
abi.json
http-results.json
http-{0,1,2}/{result,observations}.json
live-result.json
live/{observations.json,gateway.log}
negative-results.json
quota-freshness-red.log
caller-receipt-red.log
```

Earlier source inventory `9b42e406` and its inbound-only HTTP regression
remain at `/tmp/magpie-devin-paired-4_50y5qa/` on usbox. The same live
replacement defect was present there; that snapshot is not substituted for
the committed checkpoint above.

The source checks and artifact hashes identify this local implementation,
not a published SDK/plugin release. Native app boot, every Devin management
RPC, live CLI/vendor parity, browser auto-listener integration, a rotating
refresh/revoke endpoint and other native platforms remain unverified or
outside this Chat/account-provider slice.

Strict-live closure evidence is on **usbox** at
`/tmp/magpie-generation-reviewed.ekx2dQ/`:

```text
source.json
scoped-race.log
gates-result.json
sensitivity-result.json
static-result.json
live-result.json
paired-*/{account-transition,old-operation-fence,replacement-response}.json
paired-*/{expected-frame-auth,first-new-unsigned}.json
baseline{,-plugin,-second}-race.log
```

The eight-file inventory uses the same sorted compact path-to-SHA256 JSON
map convention. Local checkpoint:
`/tmp/magpie-account-generation-review-checkpoint.json`.
