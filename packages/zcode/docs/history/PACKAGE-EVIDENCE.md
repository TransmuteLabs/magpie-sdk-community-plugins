# Ordinary full ZCode package

2026-11-14. Follows donor-parity Lore checkpoint
`f6c708fd6929d4fe8e36cd034953529c10b8233e`.
The requested ordinary package now contains the ready provider,
controller, owned agent and UI together. `build.py` needs no selection
flags and produces the SDK's normal install/update asset name.
No second module or community runtime is required.

## Artifact and source identity

| Identity | Value |
| --- | --- |
| Ordinary artifact | `dist/magpie-plugin_linux_amd64.zip` |
| Preserved versioned copy | `dist/zcode-0.10.0/magpie-plugin_linux_amd64.zip` |
| ZIP SHA-256 | `2803d64d786a4d08f5abb7ee283fba1e6ea300d8fb82b69abe09640492fde191` |
| Default version | **0.10.0** |
| Existing full runtime contract | SDK **1.5**, native ABI **1** |
| Full package | One provider `zcode`, one `zcode-native` agent, controller actions, window metadata and static browser assets |
| Frozen source files | **139**, excluding git/dist/cache and evidence documents |
| Plain inventory SHA-256 | `21e13fe9a297da8d003efc5de0f611c6c3a37d75e601454fcd2017e535c07a3b` |
| Host-tagged inventory SHA-256 | `b3bb4b94379122a3a563b9fa2cab6c219672aff36fd2f56a2a63647a818be2a4` |
| Built/tested SDK host | `d07078c7926daf6f33af09c40297c941bd39da92` |
| Host binary SHA-256 | `5d1f307c9164bc4954c7806d3658255cb497da71f3cf75c1afb63c6a3064b8b3` |
| Vendor source | `fec975905a2abd4c07e35c40d51fa097a2258ec9`, desktop **3.18.2**, CLI **0.17.1** |
| Separate domain donor | `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, community package **0.1.61** |

This selects the already-tested `ControlManifest` and full native entry.
Its SDK 1.5 contract is unchanged; no new SDK 1.6 capability/minimum was
introduced for packaging. The explicit `--headless` compatibility
artifact remains version 0.7.0 / runtime floor 1.4 under
`dist/compat-sdk14/`. It is not another module needed for normal use.
The source lock now describes the ordinary full artifact.

The ZIP contains one native library and its full manifest, page metadata,
bundled browser UI/SDK adapter and source provenance. There are no
node_modules, provider-worker packages or additional runtime directories.
Go/tsc/Bun are author tools; installing the ZIP does not start a Bun worker.
The actual ZCode application executable remains a separately supplied
client application, not a second Magpie module.

All provider/controller/domain implementation Go file hashes match the
f6c708f domain inventory exactly. The only changed production Go file
selects the existing full manifest for the manifest CLI. Domain RED,
scoped race and negative evidence remain in `PARITY-EVIDENCE.md`.

## Old-failing package behavior

`testdata/parity/default_package.py` first ran plain `build.py` before
the packaging change. The native build succeeded and the resulting
ordinary ZIP declared headless 0.7.0 with no window/agent. The artifact
assertion then failed with:

`default package omits the ready agent/UI composition`

`default-package-old-red.log` SHA-256:
`0f0789fc68ea0e3046d83a29c39cdb4920c0dd2d63ee21f40d054f737afd4505`.
This is a **built-artifact assertion**, not a Go test assertion;
the report's zero Go test-failure count does not conceal that distinction.

After the change, the same no-flags command passes exact checks for the
existing SDK 1.5 contract, ABI 1, one provider/agent/library, window
metadata, controller/claim/check-in actions, browser assets and matching
source-lock metadata. Headless-only fixtures request their compatibility
manifest explicitly, while ordinary native lifecycle/UI tests unpack
the default full ZIP.

Changed packaging/fixture files are `build.py`, `cmd/manifest/main.go`,
`sources_test.go`, `source-lock.json`, `auth_e2e.py`, `e2e.py`,
`ui_e2e.py`, `ui_auth_e2e.py`, `testdata/agent/run.py`,
`testdata/agent/verify.py`, `testdata/parity/default_package.py` and
`testdata/parity/verify.py`, with current build/provenance documentation.
The normal path is simpler: one command selects all ready components;
the former `--ui` build choice is removed.

## Final ordinary-package acceptance

Every final stage ran on usbox using the frozen source inventory,
Go 1.26.8, existing offline caches, fresh HOME/XDG and timeouts.
UID/EUID remain 1000, all capability masks are zero and only loopback
exists in the user/network namespace. No live credentials, vendor,
installed application or host/core source was changed.

| Gate | Result |
| --- | --- |
| Final module race | **317 passed**, zero failed |
| gofmt, module vet, Python syntax | Passed |
| Plain full build and artifact assertions | Passed |
| Explicit historical headless compatibility build | Passed |
| Full and headless native ABI | Passed, simultaneous sessions |
| Strict TypeScript and browser bundling | Passed |
| Node / Bun page/controller tests | **29 / 29 passed** |
| Native owned-agent lifecycle from ordinary ZIP | Passed |
| Native China sign-in/models/Start | Passed |
| Native selected-account controller/UI actions | Passed |
| Native production UI/auth bridge | Passed |
| Agent file/binding behavior-negative overlays | Three expected compiled assertion failures |
| Native header-guard-disabled overlay | Expected actual lifecycle assertion failure |

The native forged-header proof retains the existing strict boundary.
Default host filtering does not deliver arbitrary client headers to the
module; the explicit PassHeaders fixture delivers each forged ticket/card
header and requires refusal before vendor admission/inference. Disabling
the native guard must make that assertion fail. No guard or host
ownership requirement was weakened.

| Final log | SHA-256 |
| --- | --- |
| Module race | `e6e27a1b1824d86cc1f35ae13a4c73f028c1cc190c4e2f3f03cb9072110b1bfc` |
| Default full package assertion | `989b685ee2df19d5346e6438a176ebe1909cf8f00733bc8d9360a3567a511f3d` |
| Full ABI | `c5beaedb3e13a8558b5fa17524ebe213122aa34a82a70d1b8f3fc39176a66817` |
| Native owned-agent lifecycle | `451e850335685b165ca6a6d4824946e4c2c9e56a5c79dc4d07451249c352c08d` |
| Native controller/UI | `6fce15d865b3bdad85bbe0cf21f88f74b56718ff4419d99a2b19796ca7093afb` |
| Native forged-header negative | `eebf53604176cea8cc09450ad45f7a8c71d12295d4989dfa66d242b0e9373e1c` |

Local ignored `dist/zcode-0.10.0/evidence/` retains the frozen inventory,
old package RED/observation, final reports and logs. Earlier domain
artifacts/evidence remain in `dist/parity-domain-0.10.0/`; the 0.9.0
agent checkpoint remains separate. The historical untracked
`AUTH-WEB-EVIDENCE.md` is preserved outside commits/packages with SHA-256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.

## Remaining limits

The normal ZIP includes everything ready in this cut; it does not prove
complete donor or native-client parity. Gift response alias rewriting,
plain-model bypass of a held gift route, pending-plan/card presentation
and wider ToolSearch/turn attribution still need work. Dual model
discovery also still requires a confirmed current paid entitlement;
inactive-paid-to-gift switching and manual non-discovery import surfaces
are not claimed complete.

Check-in is an explicit module action, without a scheduler,
native-account-operation integration or page check-in control.
SDK 1.6 native account operations exist; automatic scheduling is a
separate module/contract decision, not an assumed missing SDK feature.

No actual ZCode client executable, live vendor/account, rendered-browser,
Darwin/Windows native runtime or custom data-root/Library acceptance is
claimed. Agent launch uses an explicit synthetic executable. Automatic
DB/WAL capture, financial import and actual-client resume remain separate
unimplemented work. Earlier platform evidence is compile-only.

The two historical unexpected `/` billing requests remain unattributed;
these passing namespace runs do not establish a cause or fix. Later
shared-host freeze acceptance is not inferred from d070 verification.
The first plugin-free SDK host rollout does not wait for this module.
