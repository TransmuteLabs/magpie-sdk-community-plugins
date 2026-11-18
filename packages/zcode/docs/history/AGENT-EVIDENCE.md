# Owned ZCode agent acceptance

2026-11-14. Continue from CN/Start Lore checkpoint `3d949ff`.
The native/UI distribution is now **0.9.0**, with AgentWiring in that
same native module. The headless distribution stays **0.6.2**.
This is bounded default-HOME agent acceptance; it does not establish
actual ZCode client startup or complete community-domain parity.

## Frozen identities and source provenance

| Item | Exact identity |
| --- | --- |
| Built/tested shared host | `d07078c7926daf6f33af09c40297c941bd39da92` |
| Built/tested host tree | `ed28dfd65eca68d15e9400b56660bbbf5e7bc319` |
| Host binary SHA-256 | `5d1f307c9164bc4954c7806d3658255cb497da71f3cf75c1afb63c6a3064b8b3` |
| Consolidated devzeus HEAD checked read-only | `b5e4494f6ae7b69b0333925e21e9eaf709a7acf9` |
| Consolidated host tree | `1880ea418af8c65318afbcc4c7e396aa9cb5dd12` |
| Vendor source | `fec975905a2abd4c07e35c40d51fa097a2258ec9`, desktop **3.18.2**, CLI **0.17.1** |
| Previous implemented vendor baseline | `b95ebaffeab641907b38652f45c1502bd762972d`, desktop **3.18.0**, CLI **0.17.1** |
| Frozen module source inventory | **140 files**, `agent-final-source.json` |
| Inventory SHA-256 | `dffbacfd6c3ac96a1ea2dcd5352d482cf4e32ca361f0dd894d1037b7f41abfc7` |

The read-only committed diff from `d07078c` to `b5e4494` contains only
`AGENTS.md`; runtime source is identical. Verification used an exact
archive of `d07078c`, not the host's unrelated uncommitted test changes.
The versioned [source-lock.json](source-lock.json) therefore keeps the
actual tested commit. No host/core files were edited for this slice.
SDK deployment does not depend on this plugin's acceptance or delivery.

Versions were read from the two vendor package files at the exact
commit, in `/Users/maratkarimov/work/SIB/Agents/ZCode`. The following
SHA-256 values identify the source used for the bounded config adapter:

| Vendor path at `fec975905a2abd4c07e35c40d51fa097a2258ec9` | SHA-256 |
| --- | --- |
| `package.json` (3.18.2) | `a94d4a04b3ba8e8f693c7a41597488c1884ba6d37b5e7b9800f0c979f20424df` |
| `apps/zcode-cli/package.json` (0.17.1) | `971f8f3e93e747643c37a42c3716d8f81cfc21e5abc031f09d93f82e98a3ebe7` |
| `packages/provider-node/src/provider-config-file-codec.ts` | `7a57fa943a527eeb901a6707326f143e2a17231fe6c1c07f850eff02065b8dfc` |
| `packages/provider-node/src/runtime-paths.ts` | `28a42710029e53876c374389664e0d0a43899ed121b97b9c77a46cf41518ee03` |
| `apps/zcode-cli/packages/cli/src/provider-runtime-env.ts` | `0209c59775378750de3c6eeb82ca1640f5dcb32e4dc6ed6725d5b4c9ddc43110` |
| `packages/provider/src/config/rule-data-schema.ts` | `bada2de66dd08a4fd98cb6aa2521da7faa69be6ef9bbe9dc2481d93748d974f8` |
| `packages/provider/src/config/provider-data-schema.ts` | `ee884a115201b65045191a1f82b31c3353a3fd0ae956e1623326223429f52c68` |
| `packages/shared/src/model-selection.ts` | `653a4671723d4a3302e84ba13f29d4d55a17e5e8082b44d54ba1f9087dbd3e44` |
| `apps/zcode-cli/docs/usage/03-config.md` | `fb9082049ad83ed29c38eb5feef5b81a71ab68db491329ab0c22b70cd6fe4eb6` |

This is source-shaped compatibility evidence. The executable exercised
below is a synthetic Python `zcode`, explicitly identified as such.
No actual ZCode executable was present in the usbox fixture.

## Implemented ownership boundary

`ControlManifest` alone adds AgentWiring and the `zcode-native` profile
of target `zcode`, launching bin `zcode`. Its sole setting file is
`.zcode/v2/provider_config.json`; the launch environment uses
`ZCODE_PERSONAL_PROVIDER_CONFIG_FILE: "{config}"`. Inherited ZCode Library
placement handles instructions, MCP and skills. The headless manifest
does not acquire AgentWiring.

Public SDK `agent.inspect` and `agent.plan` handlers return checked
snapshots, file plans and canonical binding changes. They perform no
filesystem writes. Connect/set/sync/disconnect preserve foreign/manual
rules, API format, user names, output limits and compatible selected
options. Disconnect restores owned baseline rules and selection while
keeping foreign edits; an unchanged document restores exact baseline
bytes or removes a newly created file. Unknown models/actions, malformed
or duplicate rules, missing/foreign/noncanonical bindings and canceled
requests fail closed.

Host gateway URL/key and published model identities enter the config;
vendor credentials remain in the single SDK account store. Existing
native/auth/history/controller owners remain in the same bundle.
Host file plans, backup/rollback, binding revisions, launch and Library
contracts are reused. No community runtime, second credential owner,
new dependency or generic host edit was introduced. `go.mod` and
`go.sum` remain unchanged.

## Forged-header RED: cause and behavior-negative proof

The initial lifecycle assertion expected an ordinary authorized Start
request to fail after the synthetic client attached a forged native
ticket/card header. It returned exit 0. That test did not deliver the
header to the native guard: the host reconstructs outbound requests.
`Provider.Sign` in `internal/provider/account.go` calls
`passClientHeaders` in `requestheaders.go`, which copies only configured
`PassHeaders`. `extension_transport.go` then serializes those sanitized
headers into `ExecuteRequest.Headers`.

The corrected fixture retains strict refusal assertions:

1. With default filtering and only a Start account, attempts to route to
   OffPeak/Highspeed using forged headers fail. Ticket mints, card draws
   and native inference all remain zero.
2. The isolated fixture calls the existing public `/api/provider/save`
   setting with `passHeaders` containing `X-Off-Peak-Ticket-ID` and
   `X-Highspeed-Card-ID`. Each forged header now reaches the existing
   native guard and is refused before vendor inference or admission.
3. A compiled Go overlay disables the production header guard. The same
   lifecycle assertion fails with exit 0 where refusal was required.
   The causal stage expects that assertion failure, not merely a build
   error or an arbitrary nonzero result.

Neither the production native header guard nor a host contract was
changed to make the regression green.

| Preserved evidence | SHA-256 |
| --- | --- |
| `forged-header-red-agent-native-lifecycle.log` | `612a401b3c99fbdf8e1ab017ecd417af6391278fbd8c851200e311a7cb7b52d8` |
| `forged-header-red-agent-integration-results.json` | `fefcb82e9082d656bc8cb57015efb19c9bd546b2039565e2709140f0452f3cf0` |
| Guard-disabled `agent-mutation-header-guard.log` | `a08b33006b2e6578f2bbc274c65304e8529e4ee883959890bf8e36f84f447ff4` |
| Final `agent-native-lifecycle.log` | `c8d43b272d8d2e4109fffa7bd63ead38250085b62294d47c08209c23911322e9` |

Before implementation, the new agent assertions compiled against the
old module and failed for missing AgentWiring/handlers. The preserved
`agent-old-red.json` SHA-256 is
`53b38835c047e74222fb68c95fded0db7edad6701d436b1ad2d120f86c5b45d6`;
its log is
`e65c0b83d8d423b7eb45e65ebfdaa6521803b894244719332a7895cf045ae246`.
Three other compiled mutations independently fail the assertions for
manual-rule preservation, foreign-edit restoration and exact binding
validation. All four expected-negative results are recorded in
`agent-causal-results.json`.

## Verification and ordinary lifecycle

All builds/tests ran on `ssh usbox`, under
`/tmp/magpie-zcode-agent-ieXpZg5H`, using SDK Go **1.26.8** and existing
offline pinned caches. `testdata/agent/verify.py` uses isolated HOME/XDG,
`GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`,
`GOFLAGS=-mod=readonly`, bounded subprocess timeouts and a user/network
namespace with loopback only. UID/EUID remain **1000** and all five
capability masks are **zero**. Native root-ownership checks were retained.
The temporary two-CPU cap was lifted. No live credentials, vendor
endpoints, app settings or installed community runtime were used.

| Gate | Result |
| --- | --- |
| Python AST, gofmt and module `go vet ./...` | Pass; gofmt has no output |
| Scoped `OwnedAgent\|Control\|SourceLock`, `-race -count=20` | **500 pass events**, zero fail/skip |
| Entire plugin `go test -race -count=1 ./...` | **262 pass events**, zero fail/skip |
| Headless and native/UI package builds | Pass, including strict TypeScript compilation |
| Native ABI conformance of both packages | Pass; full described contract equals packaged manifest |
| Node / Bun UI assertions | **29 / 29 pass**, zero failures |
| Native agent lifecycle, China auth, native UI fixtures | All exit 0 |
| Four behavior mutations | All reach their expected failing assertions |
| Darwin arm64 / Windows amd64 | Root vet and CGO-disabled entry builds pass; compile evidence only |

The `./...` gates above cover only this module. No broad host suite was
run; union verification belongs to the leader. The host was built from
the frozen archive. Every final module stage used the 140-file inventory
and checked its exact file hashes and tested host commit before running.
This evidence document and the historical `AUTH-WEB-EVIDENCE.md` are
deliberately outside that frozen inventory.

The ordinary host lifecycle observed one native catalog entry and one
SDK account store. Browser Start and an explicit synthetic paid snapshot
produce two accounts, each generation 1. Preview is inert and a stale
revision is refused. Connect/set/inspect/sync/disconnect and real host
run/env paths preserve literal argv, a HOME/config path containing
spaces and literal `{key}`, foreign config, personal instructions,
MCP entries and skills. File drift, changed module config and a disabled
module refuse launch.

The synthetic client sends one Start inference and three native-alias
inferences: OffPeak twice for the same conversation and Highspeed once.
Exactly one ticket is minted and one card drawn. Native journals retain
the exact host conversation/account/generation, and the vendor fixture
receives matching derived UUID session identity in headers and metadata.
No community plugin, legacy auth store or runtime/node_modules is installed.

The lifecycle exercises production handlers with only endpoint
registration substituted for the loopback vendor. It does not exercise
the unmodified production library against real endpoints. The two native
library hashes are recorded separately below.

## Deliverable and preserved artifacts

Local package:
`dist/agent-0.9.0/magpie-plugin_linux_amd64.zip`.
It contains 17 members: the native library, manifest, own UI and source
provenance documents. Its runtime requires no community package.
It was not installed into the user's live Magpie or published.

| Artifact | SHA-256 |
| --- | --- |
| Native/UI/agent Linux amd64 ZIP | `70b40ad08e73f02ce42b2f98ee133e5018039b56caa22b7bd280d9460cb4d3ea` |
| Production native/UI library | `79d69aa5592914a0d94ffea0d548cc4da0a4736e8f68375c6ddd9e75a931f052` |
| Lifecycle endpoint-fixture library | `5c363ab084cc778ad8cf2e3c319c33a92e7c18c2733ff2c84f6f7e93491ef47b` |
| Headless compatibility ZIP, retained on usbox | `bd67aae14ff541c7687751592ca2740cad547b32dd403ab19c53de02d186ffd5` |
| Headless production library | `5b3c4c21cc64afbf828b5535d0bb384903d94e1889bce30efed2117e6fc7d00c` |

`dist/agent-0.9.0/evidence/` holds local copies of the frozen inventory,
stage reports, final lifecycle/auth/UI logs, all four mutation logs,
old-code RED and the initial forged-header RED. Full terminal logs remain
in the usbox fixture directory. Generated artifacts are ignored by git;
this document records their immutable identities.

The historical untracked `AUTH-WEB-EVIDENCE.md` remains untouched,
outside the commit and package. Its SHA-256 remains
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
The separate CN/Start checkpoint retains its earlier 100-file inventory
and race20/Node/Bun/native evidence in [AUTH-CN-EVIDENCE.md](AUTH-CN-EVIDENCE.md).

## Remaining real-runtime and domain gaps

- Actual desktop/CLI startup, live vendor/account/app verification,
  rendered-browser behavior and native Darwin/Windows runtime remain
  unverified. Synthetic launch and cross-compilation do not close them.
- Custom data-root provider files and custom Library placement are not
  implemented by this bounded default-HOME profile.
- Gift-only models and dual paid+gift `-Trial`, free-first and strict-trial
  routing remain unimplemented.
- Claim code 0 without `data.plan` and code 1003 remain uncertain.
  Definitive 1004/3002/3007 handling and feed discovery are not ported.
- No ZCode daily check-in action, claim workflow or schedule is
  implemented. SDK 1.6 already has generic native account operations;
  this is module work, not a missing host contract.
- Adaptive Start concurrency and shared-IP/network holds are not ported.
- Automatic SQLite DB/WAL capture, financial import and actual client
  resume remain unimplemented. Existing manual snapshot/SQLite evidence
  remains its own checkpoint.
- Two historical unexpected `/` requests in host-visible billing
  fixtures remain **unattributed**. Strict unexpected-request assertions
  are retained. Passing isolated namespace runs does not identify their
  cause or establish that it was fixed.

[COMMUNITY-PARITY.md](COMMUNITY-PARITY.md) and
[VENDOR-DIFF-3.18.2.md](VENDOR-DIFF-3.18.2.md) retain the wider semantic
comparison. No complete donor parity, live acceptance or publication is
claimed.
