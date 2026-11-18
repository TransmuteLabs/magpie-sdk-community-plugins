# Plugin monorepo reorganization

Requested: organize our five SDK plugins in one repository with the predictable packages/scripts layout used by magpie-community/plugins. Preserve reviewed runtime behavior and all dirty source work. No host changes, dependency additions, commits, merges, pushes, publication or live installation in this pass.

## Layout and boundaries

- packages/{devin,zcode,opencode,commandcode,omp}/README.md: short current entry point.
- src/: existing Go module, source, commands and source-local testdata. Module import identities stay unchanged; this is a multi-module workspace, not a domain rewrite.
- web/: existing browser sources; tests/: author verification scripts; scripts/build.py: relocated per-package builder behind one root build command.
- src/magpie-*.json: source manifest variants beside Go code where go:embed requires them; full versus compatibility variants are explicit. The published runtime filename remains magpie-plugin.json.
- docs/: donor provenance, detailed reference and prior verification/design records; history/: historical plans/checkpoints when identified.
- Root registry.json, README, AGENTS, go.work and scripts/: navigation, common checks/build/package commands. No historical reports or build outputs at package roots.

## Preservation before changes

Inventory each original repository's HEAD, remotes, staged/unstaged changes, untracked source and relevant ignored paths separately. Capture all current tracked/nonignored source bytes and hashes, plus original Git patches, before edits. Never use HEAD alone as migration input. Keep the five original repositories intact; no deletion or force-removal. Generated ignored dist/cache files remain with their original repositories and are recorded separately.

## Behavior and checks

Existing Go/domain/native/UI regression suites already protect the audited behavior. Copy them unchanged before relocation. Add meaningful checks for package selection, preserved-source mapping, manifest/provenance, SDK resolution and package output contracts. Fix only relocation-derived paths/imports/docs. Keep literal historical evidence identities as history, not claims about this new layout. Every author records old/new path mapping and changed files.

All target tests/builds/formatting run on regular usbox with private HOME/XDG and existing pinned offline caches, no live account/vendor/app/keychain. Build each default package and run Go tests/vet, affected browser/provenance checks and at least one ordinary native install/describe path per package as available. Independent review checks preservation, layout consistency and relocated consumers before any commit can be considered. Root documentation must distinguish completed verification from platform/live limits.

## Parallel scopes

Each package has one author, exclusive to its packages/<name> subtree. Parent owns root tooling, migration inventory and verification orchestration. Independent reviewer does not author the package changes. Original repositories and the Magpie host remain read-only. No subagent-created branches/worktrees are needed.
