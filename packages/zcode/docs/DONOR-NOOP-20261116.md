# ZCode donor comparison, 2026-11-16

This read-only comparison follows the completed API1.12 candidate-06 runtime
gates in [REQUEST-POLICY-EVIDENCE.md](REQUEST-POLICY-EVIDENCE.md).
Parent fetched the refs; SOL neither fetched nor changed a donor checkout.
Common docs and STATUS remain with parent.

| Source | Exact fetched ref | Compared baseline | Result |
| --- | --- | --- | --- |
| Community `origin/main` | `b35d4b35b2446bd929896ddeb58f3cb299e9e2d1` | audit `57d1cc6a9f49389d495ea9be8d986c70de7767bf` and ported `87a8cf03c62fa55ca845c51648e8859ece580336` | all three `packages/zcode` trees are `d1be84b45f19c7d51bf3bf1c428ddc7ae95dfbf7`; no package delta |
| Agents/ZCode `origin/main` | `fec975905a2abd4c07e35c40d51fa097a2258ec9` | existing `fec975905a2abd4c07e35c40d51fa097a2258ec9` | identical commit and full tree `5e90fec80706b8381f94e4eef48cf821b9868916`; full diff empty |

The community checkout remains at `57d1cc6a`; the fetched ref is inspected
through Git objects. `git rev-parse <ref>:packages/zcode` establishes the
complete subtree identity, and scoped `git diff --stat` is empty. The fetched
package JSON still names `@magpie-community/opencode-zcode-auth` **0.1.66**.
Its package JSON SHA256 is
`4573dc6bd65068959b9af616aa59e2c7746011cef0e79d8dc3782e0ef2022091`;
`index.mjs` remains
`cd81a4e3f59cadefa677d262a9969b284ccc6c0bdae4fe9a7fe456155bbd73a8`.

The vendor root package remains desktop **3.18.2**, with CLI **0.17.1**.
Root package JSON SHA256 is
`a94d4a04b3ba8e8f693c7a41597488c1884ba6d37b5e7b9800f0c979f20424df`.
The vendor worktree is clean. Its unchanged full commit covers queue, ticket,
claim/reset, native process and history surfaces as well as the watched policy
and request/auth paths.

There is no vendor behavior to port. Plugin versions remain full **0.15.0**
and provider-only **0.7.3**, native ABI **1**, with the separately implemented
SDK floor **1.12**. Wire client **3.18.2**, vendor CLI **0.17.1**, community
**0.1.66**, plugin versions and SDK floor are distinct identities.

The new source-lock entries record `checkedAt: 2026-11-16`, immutable refs,
tree/file hashes and `noBehaviorDelta: true`. Historical adoption/audit entries
remain intact. This is provenance maintenance only: no new retry, queue,
financial, auth, agent or session behavior and no fictitious version bump.
