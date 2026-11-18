# Effective agent model metadata, API 1.9

Date: November 14, 2026.
Status: bounded metadata acceptance passes on actual shared source after
the parent applied both Go effort-preservation hunks. SDK helper,
ordinary-agent, native-agent and module race20 are green. Earlier actual
REDs remain preserved. The prior committed checkpoint is consolidated host
46756a48c9c9f204a8369638dc63cbcb83d02031, tree
db40ddbf8944b54f9f689d444644af4c3bc76881; its native replay also covers
separate API 1.9 decision discovery. The current module pins physical tree
8fe78ab9df13d733d61462eacce19bff523c1e58 and the supplied host binary in
source-lock.json; c11acbc2 identifies its committed baseline only. The bounded
native agent/decision replay is recorded in
[verify/EVIDENCE-SDK19-FINAL-REPIN.json](history/EVIDENCE-SDK19-FINAL-REPIN.json).
Overall release acceptance remains parent-owned.
Original metadata base: host ae957e5a8a77553a6cf6f484c8b58a69393f3887, tree
958fdab666e41dc35c41a8a061b76dba88c67aeb, including commonbase 52a8d85.

## Contract and ownership handoff

The separate public `options.decisionModels` DTO is now validated by both
CommandCode agent handlers without entering the conversation models or
vendor settings. Its provider/group facts and finite decision endpoints use
`ValidateAgentDecisionModels`; unknown DTO fields and null are refused.
No CommandCode decision-config field is invented. Current commands, hashes
and the ordinary native provider/group replay are recorded in
[the consolidated verification](history/VERIFICATION.md).

Keep `agent.inspect` and `agent.plan` options as their existing JSON envelope.
Its `models` entries retain the ordinary picker fields, adding `maxOutput`,
`efforts` and `defaultEffort` only for a module negotiating API minor 9 or
later. There is no additional capability or descriptor flag. Existing
API 1.0–1.8 modules receive their previous model-option wire, with all three
new fields absent.

The metadata owner adds these independent new files:

- `pkg/extension/agent_model_metadata.go` and its tests: public
  `AgentModelMetadata{MaxOutput int64, Efforts []string, DefaultEffort string}`,
  deep `Clone`, and `ValidateAgentModelMetadata(Manifest, AgentModelMetadata)`.
- `sdk/js/agent-model-metadata.js`, `agent-model-metadata.d.ts` and tests:
  `AgentModelMetadata` and `validateAgentModelMetadata(manifest, value)`.

The helper's wire is `maxOutput?: nonnegative safe integer` (zero means unknown), required
`efforts: string[] | null`, and optional `defaultEffort: string`.
Unknown efforts remain `null`; measured no-level support is `[]`.
The default must belong to a nonempty effective list. Efforts are bounded,
unique ordinary core effort levels; metadata cannot carry credentials,
provider configuration or vendor-specific fields. A metadata helper call
requires `agent-wiring` and negotiated minor >= 9.

Discovery `Model.efforts` has no new API 1.9 floor: missing and null are
unknown for every supported minor, matching Go's existing nil validation;
an explicit array remains known, including `[]`. The API 1.9 floor applies
to the new agent-option projection and metadata helper. The JS regression
checks null and empty discovery at minor 0, 8 and 9.

Required parent-owned public hunks, supplied before implementation:

```diff
--- a/pkg/extension/types.go
+++ b/pkg/extension/types.go
@@ type Model struct
-    Efforts []string `json:"efforts,omitempty"`
+    Efforts []string `json:"efforts"`
```

This preserves unknown versus explicitly empty discovery/manifest/cache
input through the SDK encoder. Missing and null both decode to unknown;
empty remains a non-nil empty slice. Existing model validation still
applies. No extra model field or vendor inference is required.

One provider owner condition must also preserve that distinction before
projection:

```diff
--- a/internal/provider/models.go
+++ b/internal/provider/models.go
@@ func effortsOf(m catalog.Model) []string
-    if len(m.Efforts) > 0 || m.Provider != "" {
+    if m.Efforts != nil || m.Provider != "" {
```

Without it a measured empty list may borrow vendor levels before the agent
sees the effective entry. This hunk belongs to the parent provider owner.

```diff
--- a/sdk/js/index.d.ts
+++ b/sdk/js/index.d.ts
@@ interface Model
-    efforts?: string[];
+    efforts?: string[] | null;
@@ exports
+export type { AgentModelMetadata } from "./agent-model-metadata.js";
+export { validateAgentModelMetadata } from "./agent-model-metadata.js";
--- a/sdk/js/index.js
+++ b/sdk/js/index.js
@@ exports
+export { validateAgentModelMetadata } from "./agent-model-metadata.js";
--- a/sdk/js/manifest.js
+++ b/sdk/js/manifest.js
@@ function modelPolicy(model)
-  if (model.efforts !== undefined) {
+  if (model.efforts !== undefined && model.efforts !== null) {
--- a/sdk/js/package.json
+++ b/sdk/js/package.json
@@ files
+    "agent-model-metadata.js",
+    "agent-model-metadata.d.ts",
```

Parent coordinates API minor 9, `^1.9` compatibility and package version in
the existing shared version/manifest/index owners. This owner does not
edit or stage those files. The helper declaration imports `Manifest` from
the existing index and exports its own type; it needs no shared DTO edit.

## Effective projection and user configuration

`internal/agent/agent.go:Option`, `route.go:buildVia`, `sameOption` and the
returned-copy path carry the same effective facts for individual models
and routing groups. Limits come from `Entry.PublishedOutput()`, including
ordinary model/provider overrides and context clamping. Levels come from
effective `Entry.Efforts`; `Reasonless` means explicit `[]`. A removed
default is omitted. Caller mutation must not alter a cached effort list.

`extension_actions.go` and `extension_controller.go` serialize the metadata
using the actual installed manifest, including controlled inspection.
The legacy wire stays unchanged below API 1.9. Catalog/settings changes use
the existing stamps, checked plan and ownership ledger.

The controlled owner retains the last requested model-options snapshot
in its existing internal binding and supplies it as `priorModels`.
`previous` remains the original file preimage. Comparing generated limits
with the prior catalog, rather than with a preserved user value in the
last applied file, keeps an edit through multiple later syncs. This also
distinguishes a user-removed default (Auto). Pre-1.9 options omit
`priorModels`; a module cannot supply that host evidence itself. No second
copy of applied settings, shared DTO, persisted authority or new capability
is required.

CommandCode maps the effective snapshot to BYOK `maxOutput`,
`reasoningEfforts` and a supported default without replacing the user's
selected per-model effort. Explicit Auto and valid user defaults remain
the user's choice. Foreign providers, unknown fields, unrelated models,
settings and disconnect preimages retain their existing ownership rules.
Unknown output is never filled from a static vendor table. Only supported
CommandCode effort levels are advertised.

## Ordered implementation and proof

1. Add an exec fixture and permanent ordinary discovery/Connect/sync/model
   edit regressions. Exercise effective output overrides/clamping, valid
   and removed defaults, nil/empty levels, user edits and legacy metadata
   off. Capture a compiling behavioral red on the immutable commonbase.
2. Add bounded Go/JS helper validation and wire/clone regressions, then
   implement the existing agent projection/controller seams. Preserve
   ordinary no-op sync and checked action execution.
3. Update CommandCode's existing default package and config planner,
   recording exact native config donor revision/hashes. Add module
   preservation/default/refusal tests and an actual installed native
   ordinary Connect/sync/set/disconnect probe.
4. On usbox, run scoped race20, Go/JS/TS static checks, Node and Bun helper
   parity, and a causal old-projection negative with the same tests. Replay
   the existing check-in/activation and full exec/native SystemOne suites
   on this commonbase using isolated state and existing offline caches.
5. Record exact source inventories, commands, logs and package provenance.
   Host staging/commits remain the parent owner's work. Commit only this
   module with Lore after its actual package gates pass; final parent
   commit/tree provenance requires a replay against that exact release.

Live accounts, vendor requests, installed apps, broad host acceptance,
startup/signing changes and additional SDK capabilities are outside this
slice. Earlier source captures, failures and accepted module distributions
remain preserved.

## Post-fix actual shared acceptance

The parent applied both Go hunks before the actual `shared-api9-2` capture.
Its 10,703 files have source identity
`e65763bfb9d461d3354effadd909cdcecd2f727e3461f0c07ba96c633508ab3b`
and archive SHA256
`1ea721335576005f5aff570f7b4ce165bf24dc0fd4042c9842f0a7ccf26f4b44`.
No proposed production overlay was used. Only the two parent-fixed files
changed among the 45 pinned SDK inputs since the earlier actual capture;
all 16 metadata-owned host files remain unchanged.

On usbox with the same isolated offline/native environment, SDK helper
race20 has 260 PASS, ordinary agent lifecycle/no-op race20 has 120 PASS,
installed native ordinary metadata race20 has 20 PASS and the full module
race20 has 1,420 PASS, all zero FAIL/SKIP. Scoped vet, empty gofmt output,
56 source hashes, C-shared/strict TS/default package build and package
verification pass. The legacy ordinary test still proves metadata off
below API 1.9. Module runtime inputs are unchanged from the earlier RED.

The current proof is
[verify/EVIDENCE-SDK19-POSTFIX.json](history/EVIDENCE-SDK19-POSTFIX.json)
and `usbox:/tmp/magpie-model-metadata.INRSn51E/module-metadata-postfix1/`.
The base commit still names only committed ae957e5; the snapshot pins the
dirty API 1.9 source separately. Final release provenance requires the
coordinating owner's exact parent commit/tree.

## Initial behavioral red

The old agent production code with only API 1.9 admission retained ran the
permanent ordinary tests on usbox. Result: 1 PASS (legacy metadata off),
2 FAIL, zero skips; no compile error. The failures are missing effective
output on ordinary Connect and missing context on the ordinary routing
group branch. Source identity:
`9d2286f714b0134ba7065450f820e83fc0fdc22feee9661c6068105ad5f1c11a`.
Evidence: `usbox:/tmp/magpie-model-metadata.INRSn51E/proof-red/metadata.log`,
SHA256 `40fcd4bafbfdaf819aeaad9dc3053b14a9cd6c3f69a7fe6f29f300e8e4c7f784`.

## Pre-fix actual shared source handoff, retained

The read-only actual shared capture `shared-api9-1` has source identity
`0f712e7774eb73c09195e5f9be96c7cc8a0b7cc42f2f58f41278f822e5519356`
and archive SHA256
`65764b96458c7d3a15155a63c4ac826d33558dc9b5fbb08d2c1fcf24ab5ce7db`.
No proposed hunk was added to that capture. Its 10,702 files matched before
and after verification.

The actual ordinary agent race20 run has 100 PASS and 20 FAIL, all failures
on `magpie/metadata/empty`: effective `efforts` is `null`, expected `[]`.
The SDK helper has 12 PASS and one FAIL because SDK `Model` JSON erases
the explicitly empty list. Both failures are behavioral; Linux vet, main
nogui build and formatting pass. The same production projection with the
two proposed parent hunks passed 120 ordinary agent and 260 helper race20
events in the separately labelled immutable review archive. This comparison
does not make the actual shared red green.

Actual shared JS source `sdk-public2` passed Node 5, Bun 5, strict TypeScript,
offline npm packing, runtime exports and types from the extracted package.
It tests discovery null and empty at minor 0, 8 and 9. Its source identity is
`d8d06a36b775230a80dbb519d3603a4c0fba35d889125fad20230bacdfcd657b`.
The helper and option projection require API 1.9; discovery null/empty has
no new floor.

Evidence is under `usbox:/tmp/magpie-model-metadata.INRSn51E/` in
`proof-shared-api9-1/`, `proof-actual-sdk-floor1/`,
`proof-sdk-package-sdk-public2/` and the preserved review directories.
Host shared/public files remain owned by the parent. No additional public
field, capability or contract is requested.

Against those actual shared bytes, the module's complete `./...` race20 has
1,420 PASS and its installed native ordinary decision race20 has 20 PASS,
zero FAIL/SKIP. Native owner, source provenance, Go vet/build, strict TS,
C-shared/default ZIP and archive validation pass. The installed native agent
race20 has 20 FAIL on the same default/no-level knowledge distinction.
The module's `go.mod`/`go.sum` retain the host's existing `x/sys v0.49.0`
transitive pin, required by the current native SDK; no package was downloaded.
The full gate remains red. Exact records are
`module-metadata-shared2/evidence.json` and
[verify/EVIDENCE-SDK19.json](history/EVIDENCE-SDK19.json).
