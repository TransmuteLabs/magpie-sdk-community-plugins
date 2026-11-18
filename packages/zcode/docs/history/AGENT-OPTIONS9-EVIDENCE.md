# Typed API9 agent options on the frozen shared host

2026-11-14. Bounded module continuation of `6e9e7a9`. This slice changes
the ZCode options decoder and its regressions, using the public SDK DTOs
and validators. No shared host source/index, dependency version, module
version, branch/worktree, remote publication or live account is changed.
SDK-only rollout remains independent of this module's acceptance.

## Exact inputs and ordinary package

| Identity | Value |
| --- | --- |
| Shared host commit | `46756a48c9c9f204a8369638dc63cbcb83d02031` |
| Frozen physical source tree | `db40ddbf8944b54f9f689d444644af4c3bc76881` |
| Host source archive SHA256 | `e188abd5d115c7da3167a6296e7d07f9bd31766a738ea4e6326ff093e4a74a61` |
| Host inventory SHA256 | `cf630604f3c719338a5c8c05ac6c035d1da435602b5b5008486ee27d7ea92b54` |
| Old module + unchanged regression archive | `b5e50740dd187d6781d14e20c0753f22935831793a3978905de273b0892b43bb` |
| Final 192-file module input archive | `6d7ca3d6644d3fdbfeaea6b8ceb56cd23293e150507dd32e478099aab8b6dfc2` |
| Final `agent.go` SHA256 | `f66d8c509e8defa7821d470a353fc701a1863eb81c9bd6f375a5e4b660dcf825` |
| Unchanged RED/GREEN fixture SHA256 | `055cbcec6f759b36c8c9941d0b8898bad8f486a036476d40bf08763b53f21489` |
| Ordinary 0.13.1/API9 ZIP SHA256 | `4e893dc37325beb39def0737140250ad5582adfb916fa3885861b4241df62527` |
| Packaged native library SHA256 | `1b408ffbb73a7db0874d9321777947c5eb89d25c66e2c089fa84b3f12c686bc3` |
| Reused parent host binary SHA256 | `32fbdeabd20ed3059f61a91f7614a51a2ac9b908b12dd637c204a82918a28256` |
| Collected receipt SHA256 | `bf576e0e438a7562a135c1dbf4a44716a8a2ad61adfde30db85af65bf3f7b9cc` |

`source-proof.json` checks all **10,751** frozen host file hashes against
the parent's inventory. Unpacking removes write bits; read/execute bits
match, with no unexpected mode change. The initial checker incorrectly
required all permission bits, then incorrectly expected every write bit
to be removed. Those preflight attempts started no Go test and their
checker versions are retained. A remote-shell quoting failure likewise
started no test. None counts as a behavior RED.

`source-lock.json` adds `sdk-agent-options9`, retaining the exact shared
commit, separately named physical tree, archive hash and 14 checked wire/
SDK source files. Older SDK acceptance snapshots remain historical.
Desktop **3.18.2 / CLI0.17.1** at `fec975905a2abd4c07e35c40d51fa097a2258ec9`
and community **0.1.66** at `87a8cf03c62fa55ca845c51648e8859ece580336`
remain unchanged. No additional donor behavior is ported here.

The ordinary `build.py` output remains one native provider/controller/
owned-agent/UI package, with one provider, one agent and the static page.
There is no community runtime or second module to install. Its embedded
source lock includes the current frozen host pin. The historical
`--headless` selection is unchanged. Development's relative Go replacement
is bound to the frozen host source during verification.

## Decoder behavior and regression

Previously, `readAgentOptions` used `json.Unmarshal` after its JSON object
check. It silently discarded the new decision list and accepted unknown
option fields and invalid decision facts.

The decoder now reuses the module's existing strict `decode` helper.
It retains `[]sdk.AgentDecisionModel` separately from chat options and
calls `sdk.ValidateAgentDecisionModels` when the field is present.
Legacy absence and an empty API9 array are accepted; null, malformed
identity/provider/group/endpoint data and unknown fields are refused with
`invalid_params` / `not-started` before a file plan.

The exact ordinary host presentation fields and opaque `priorModels`
snapshot remain accepted. Effective chat metadata uses the public
`sdk.AgentModelMetadata` DTO and validator. This compatibility change
does not add a new output/effort configuration port.

`agent_options_test.go` checks the actual module plan handler and typed
decoder. Provider/group decision facts survive decoding, while the
generated chat file and binding stay byte/struct identical to the same
request without those facts. Selecting a decision-only model as chat
still returns `unsupported` / `not-started`. Unknown top-level, model,
previous-file and decision fields, invalid public facts, duplicate JSON,
trailing JSON and null entries remain refused.

## usbox verification and causal negatives

All builds/tests use Go1.26.8, existing offline caches, temporary HOME/XDG
and bounded commands on usbox. The parent seccomp guard denies external
destinations and permits loopback and exact assigned local addresses;
fixtures use loopback. UID is **1000**, no user namespace is introduced,
and `/` remains owner **0**. No native ownership guarantee is weakened.

| Gate | Result |
| --- | --- |
| Old `6e9e7a9` implementation + new fixture, race once | **18 FAIL events**, genuine assertion failures; no build failure/race |
| Agent/options/provenance scope, race20 | **860 PASS**, zero fail/skip/race |
| Whole module, race once | **456 PASS**, zero fail/skip/race |
| Changed Go formatting | Empty output |
| Module `go vet -p=2 ./...` | Exit0 |
| Default native package build, strict TS and Bun bundles | Exit0 |
| Restore permissive JSON decode | Genuine RED, **5 FAIL events** |
| Remove public decision validation | Genuine RED, **10 FAIL events** |
| Append decisions to chat catalog | Genuine RED, **3 FAIL events** |
| Remove effective chat metadata validation | Genuine RED, **2 FAIL events** |

FAIL event counts include parent tests. All four causal controls compile,
reach their intended behavior assertions and have zero race warnings.
The regression bytes are identical in the original RED and final GREEN.

## Installed-host gate remains RED

The existing `testdata/agent/run.py` fixture uses the parent's already
built Linux/nogui host from this frozen tree and the new ordinary ZIP.
Only the native library's destinations are overlaid to its loopback
vendor. Its executable is synthetic; the actual ZCode client is absent.
The wrapper retains its private fixture folder and logs.

The run reaches the final vendor assertion after the connect/set/inspect/
launch, Library, native aliases, forged-header refusal, ledger and stale/
off checks. It then **fails** on:

```text
AssertionError: ['unexpected vendor GET /', 'unexpected vendor GET /']
```

Receipt: `installed-host-result.json`, exit1, **83.49s**. Log SHA256:
`7d500ab7dff6b0b8c749038a77dafe72bf9cd2a52027b4ec6ac689f398904810`.
The strict vendor assertion is unchanged. This is **not** an installed
acceptance pass. Assertions after that failure did not run.

This original run did not record the sender. The subsequent exact-host,
same-library repeats in
[AGENT-INGRESS-EVIDENCE.md](AGENT-INGRESS-EVIDENCE.md) attribute their two
root requests to an unrelated local service and close the reproduced
fixture failure with an incoming process-ownership boundary. This
original RED and its missing PID evidence are retained. Historical billing
root requests and the separate empty-catalog RED remain unattributed;
the controlled retired-listener regression does not identify their sender.

Nonempty decision facts are proven through the module handlers and public
SDK wire types, without an actual ZCode decision client. Actual vendor/
account/client use, native Darwin/Windows runtime, automatic DB/WAL capture
and financial import/resume remain unverified or outside this slice.
This compatibility result does not declare the whole plugin ready.

## Preserved evidence

Local packet: `dist/agent-decisionmodels9-rrdZDxUK/`.
Remote packet: `usbox:/tmp/magpie-zcode-decisionmodels9-LeI0cmVk/`.
It retains both module archives/inventories, all causal overlays and
logs, isolation reports, exact ZIP and `acceptance.json`. `collect.py`
independently verifies hashes, counts, pins and the explicit installed RED.
This narrative is added after the 192-file executable/package input
archive; those input bytes remain unchanged.

The installed fixture remains under
`installed-fixtures/zcode-owned-agent-uuq8iz_2/`, including `web.log` and
synthetic configuration. Earlier empty-catalog evidence remains untouched
at `usbox:/tmp/magpie-empty-core-attribution-F1VvoK5N/`.
`AUTH-WEB-EVIDENCE.md` remains unchanged at SHA256
`b5965a592facddb31002788cbe0a2002c7ee9e738d087ee279877807263bd8e3`.
