# Native lifecycle on the final frozen host

2026-11-14. One bounded repin of the accepted `c8281b6` module and fixture.
The host binary, production native library and existing endpoint-overlaid
fixture library are reused. No native library is rebuilt, no full domain
suite is rerun and no shared host source or index is changed.

## Exact inputs and ordinary package

| Identity | Value |
| --- | --- |
| Host base commit | `c11acbc2bebec1fb213ef562900d35188cdf55fe` |
| Frozen physical source tree | `8fe78ab9df13d733d61462eacce19bff523c1e58` |
| Host binary SHA256 | `c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86` |
| Host source archive SHA256 | `01117ce6f6f8842d16fcd81224dad1795020b3b5c4d69b9145d562fd88171e37` |
| Host inventory SHA256 | `7ce4753ff79f3493acb4f61c1826d3f52145f8b156537ac61f493b39c9a50cc6` |
| Reused module commit | `c8281b6c97d751207f429bcbc1861daa2c465abb` |
| Unchanged production library SHA256 | `1b408ffbb73a7db0874d9321777947c5eb89d25c66e2c089fa84b3f12c686bc3` |
| Unchanged endpoint-overlaid fixture library SHA256 | `57c6c01915dd6b46cde2c031394f6ee5a7972023f0887021930e8a6183122327` |
| Ordinary 0.13.1/API1.9 ZIP SHA256 | `45d1e73c30379b5911bc1c47c64928828f12988692c66b7611be70e7f681125b` |
| Native lifecycle log SHA256 | `6d40ab7c7ed08b60e9afd604556484ff7e4874cfb89fb7346bde8e975fef1cda` |
| Collected acceptance receipt SHA256 | `63170e1192de0dd575fe8a6c10d41ae0d36b0d1a8d8e596aabbc75a892302a38` |

`source-proof.json` verifies all **10,758** frozen source file hashes and
their read/execute mode bits against the parent's inventory. All 14
checked source files from the previous `sdk-agent-options9` pin remain
byte-identical. The new `sdk-final-native9` entry records 15 exact source
hashes, the physical tree and the binary separately from the base commit.
Later test-only corrections and future shared-host HEAD are not substituted
for these tested bytes.

The ordinary distribution remains one native provider/controller/agent/UI
package, with one provider, one agent and its static control page. No
community runtime or additional module is required. Only `source-lock.json`
and `SOURCES.md` differ inside the ZIP from the previously accepted
`4e893dc37325beb39def0737140250ad5582adfb916fa3885861b4241df62527`
package. Its manifest, native library and UI bytes are unchanged. ABI1,
module0.13.1 and the ordinary SDK1.9 floor are unchanged.

The README identifies the current exact pin and calls the old 0.8.0
readiness section a historical checkpoint. These documentation and source
lock changes add no production behavior.

## One lifecycle and scoped checks

All commands run on usbox with temporary HOME/XDG, offline pinned caches,
bounded timeouts and the parent's existing outbound guard. External
connections are denied; fixtures use loopback. The guard receipts retain
UID1000, `/` owner0 and no user namespace. Native ownership checks are
unchanged.

| Gate | Result |
| --- | --- |
| Ordinary installed native lifecycle, exact host and same libraries | **GREEN**, exit0, **95.64s** |
| Owned vendor requests | **93**, all attributed to the fixture lineage |
| Unknown or unresolved incoming ownership | **0** |
| Verified foreign connections rejected before HTTP | **1**, PID24086 |
| Start gateway inferences | **1** |
| Native alias inferences | **3** |
| OffPeak ticket mints / Highspeed card draws | **1 / 1** |
| Provenance test `TestSourceLockMatchesActualModuleAndNamesImmutableBehaviorSources` | **PASS**, once, no fail/skip |
| `gofmt -l sources_test.go` | Empty output |
| Native library builds in this packet | **0** |

The lifecycle proves ordinary connect/set/inspect/disconnect, exact
preview and stale revision refusal, literal config path and argv handling,
preservation of foreign config/shell content, Library instructions/MCP/
skills roundtrip, stale file/module/off refusal and disconnect restoration.
It also checks exact conversation/task identity, native turn traces,
selected session/device/version for manual gift Discovery and ordinary
gateway ledger token/cache values. Forwarded and default-filtered forged
headers grant no native admission. Final catalog/account-store assertions
pass: one native catalog entry and one SDK account store with two accounts.
The executable is synthetic; `actualClientPresent` is false.

The parent's SDK race gate of1404 GREEN is separate parent evidence.
This packet does not rerun it or make a blanket claim about every parent
fixture. The retained parent runtime receipt includes a separate failed
task-catalog fixture; the parent reports subsequent corrections as
test-only. Those later bytes are outside this exact acceptance pin.

## Strict ingress and preserved negatives

The three fixture sources are unchanged from the accepted c828 lineage:

| Source | SHA256 |
| --- | --- |
| `testdata/agent/run.py` | `2e3002a00d6a485ba96d93ec1bb9eb79c1515caef7fb8a5dd1e5070f39faf19e` |
| `testdata/agent/ingress.py` | `8c53c6b5a2f7e1ae00b66e54b4d933cc9e31b40510ac9d0064ac3a39680ecf12` |
| `testdata/agent/ingress_test.py` | `94d4255f011190d032d45894dd1a6e7f3ccd865a7807349ae5c91c31d69e10fd` |

The boundary accepts verified fixture descendants, rejects verified
foreign socket owners before HTTP and fails on unresolved or mixed
ownership. Method, path, process name and marked headers grant no bypass.
Every unexpected owned GET/POST still reaches the strict vendor assertion.
The rejected PID24086 is the same outside process attributed in
[AGENT-INGRESS-EVIDENCE.md](AGENT-INGRESS-EVIDENCE.md); no foreign service
is changed, stopped or traced here.

The earlier four real-process ingress tests and three compiling causal
mutations remain preserved evidence: admitting foreign processes gives
one failed assertion, ignoring owned root GET gives two, and accepting
unknown ownership gives one. They are not rerun or counted as new gates
in this repin. The unchanged original installed RED remains at SHA256
`7d500ab7dff6b0b8c749038a77dafe72bf9cd2a52027b4ec6ac689f398904810`.
Its sender was not recorded. Later reproduced probes do not supply
missing historical PID evidence; older billing and c575 empty-catalog
senders remain historically unknown.

## Artifacts and limits

Remote packet: `usbox:/tmp/magpie-zcode-final-native-75dm4l0j/`.
Local packet: `dist/final-native-repin-5vUiLoPn/`.
The exact package and checksum are under `module/dist/` in both packets.
The host binary remains at
`usbox:/tmp/magpie-sdk-final-base-tvn9tc_o/magpie`.
`prepare.py`, `run.py`, `attribution.py` and `collect.py`, source proof,
network receipts, logs, exact module inputs and collected acceptance are
preserved. Raw ingress headers and synthetic credentials remain private
artifacts outside Git. This narrative is recorded after the sole lifecycle;
the tested provenance sidecars, fixture and library bytes remain unchanged.

`AUTH-WEB-EVIDENCE.md` is unchanged at SHA256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
Desktop3.18.2 / CLI0.17.1 source remains pinned at
`fec975905a2abd4c07e35c40d51fa097a2258ec9`; community0.1.66 remains pinned
at `87a8cf03c62fa55ca845c51648e8859ece580336`.

Actual installed ZCode client, live vendors/accounts, native
macOS/Windows runtime, automatic DB/WAL capture and financial import/resume
remain outside this synthetic proof. Nonempty decision facts retain their
earlier typed-handler/validator evidence, without an actual decision
client. This repin closes the bounded final-host native lifecycle gate;
it does not declare whole-plugin readiness or gate the independent
SDK-only rollout.
