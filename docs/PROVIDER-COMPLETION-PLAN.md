# Self-contained Devin and MiniMax providers

Requested on 2026-11-15: complete the Devin provider in our existing SDK package
and add MiniMax, without requiring another installed plugin.

## Scope and source authority

The community repository is a source donor, not a runtime dependency. Its
current read-only checkout is commit
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`: Devin 0.1.41 and MiniMax 0.2.9.
Record exact source paths, versions and update watch points in each package's
provenance and include that provenance in its distribution.

The prior relocation is complete. This task explicitly adds provider behavior;
it does not reopen the directory migration or change the preserved originals.
The Magpie host and sibling original plugin repositories remain read-only unless
a missing generic SDK contract is demonstrated with a regression.

## Implementation

1. Compare Devin's existing native provider with the donor feature by feature.
   Preserve its native ingress, agent bootstrap and existing account fences.
   Port missing behavior with focused regressions instead of replacing working
   codecs or adding a second provider implementation.
2. Add `packages/minimax` using the existing public native SDK and package
   layout. Support China and Global accounts, device authorization with PKCE,
   transactional token refresh, chat, live models, usage and check-in.
3. Include MiniMax's selected-account tools, search and image/video operations
   in that same package. Preserve host account selection, proxy policy,
   cancellation, write receipts and usage/logging ownership.
4. Register MiniMax in the shared workspace/build/check commands. Keep vendor
   logic inside its package and avoid new dependencies.

The initial donor comparison identified one generic integration gap:
selected-account tools do not run the expiry preflight already used by inference
and search. Repair that shared host entry with the existing transactional refresh
owner before module acceptance. Do not add a vendor-specific refresh manager or
automatically repeat an accepted write after an unexpected HTTP 401.

## Parallel ownership

- Devin author: `packages/devin`.
- MiniMax core author: provider transport/auth/models/usage, module manifest,
  builder and package documentation.
- MiniMax tools author: tools/media/search code and their fixtures/tests, using
  the agreed core transport.
- Parent: root workspace/tooling, combined verification and install inspection.
- Independent reviewer: donor parity and final implementation review.
- Host author: bounded generic selected-account tool preflight and regressions.

## Acceptance

Build, format, vet and test only on regular `usbox`, with isolated HOME/XDG,
pinned offline Go caches and a descendant network guard. Use fixture servers;
never contact live vendors or read live credentials. Add regressions for
account isolation, rotating refresh, regional redirects, stream errors,
usage and tool/media cancellation. Existing Devin tests remain required.

Build both default distributions and exercise the ordinary host SDK
install/inspect/remove path in a clean sandbox with no other plugin installed.
Check installed manifests, provenance and file hashes. Record exact tested
source identities, test outcomes and untested live/platform behavior. Do not
commit, merge, publish or install into the user's running app before a positive
independent review.
