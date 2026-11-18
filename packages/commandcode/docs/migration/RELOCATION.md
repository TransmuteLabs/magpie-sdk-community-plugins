# CommandCode relocation

Author terminal: 2026-11-15. Build, test and runtime source is frozen for the
parent's combined monorepo validation and independent review. This pass changes
workspace paths and packaging tools, preserving the audited plugin behavior.
Original repositories, host source and backup evidence remain untouched.

All78 original mapped files remain present:60 retain their exact bytes and18
have relocation changes. [PRESERVATION.json](PRESERVATION.json) records each
original/final path and hash. Runtime Go files, embedded model data and the
SystemOne fixture retain their bytes. The two browser-source changes only replace
the old SDK type-import location with the public SDK module name; the compiled
`web/app.js` is byte-identical to the audited UI artifact. No module identity,
version, capability, auth/transport/UI behavior or dependency was changed.

The changed scope is:

- `src/go.mod`: preserve the public module ID; point the development replacement
  at `../../../../magpie`. The shared builder overrides it with a temporary
  modfile when `--sdk` selects another checkout; source mod/sum files stay intact.
- `scripts/build.py`: use root `build_environment`, run Go from `src`, read
  relocated metadata/docs and bundle browser helpers from the selected SDK.
  Remove the obsolete workstation-specific runtime guard; workspace policy still
  requires regular usbox for all execution. Default output is the raw installable
  directory. `--archive` explicitly adds a sibling ZIP; normalized pre-1980 source
  timestamps are clamped by the ZIP writer without changing file contents.
- New `scripts/_web.py` and `tests/typecheck.py`: share one temporary TypeScript
  config resolving public browser types from the selected SDK. The two type imports
  in `web/app.ts` and `web/sdk-ui.d.ts` use that mapping.
- `src/provenance_test.go`, `src/systemone_contract_test.go`, the source/package/
  snapshot checkers, donor/harness scripts and native/completion replay consumers:
  update file roots, cwd and watch paths. `run_completion.py` now requires an
  explicit `--proof` directory and uses a temporary SDK modfile, avoiding a sibling
  symlink or source mutation.
- `docs/source-lock.json`: update only local fixture/watch/evidence locations;
  donor revisions, source/fixture hashes and historical claims are retained.
  Current documentation links follow the new layout. The short package README
  points to the detailed `docs/REFERENCE.md` and separates historical claims.

Ten historical receipts moved from `tests/` to `docs/history/` without content
changes. [SECONDARY-MOVES.json](SECONDARY-MOVES.json) records original map target,
final target and hash. Original receipt-internal paths describe the historical
layout; they were not rewritten or presented as new results.

Default commands from the workspace root, under the prescribed isolation:

```sh
python3 scripts/check.py commandcode --sdk /path/to/magpie
python3 scripts/build.py commandcode --sdk /path/to/magpie --output /tmp/plugin-output
```

The per-package API is
`python3 packages/commandcode/scripts/build.py --output /tmp/commandcode-package --sdk /path/to/magpie`.
The registry's extra check has package-relative cwd `.` and command
`{python} tests/typecheck.py --sdk {sdk}`. To check a ZIP, explicitly build with
`--archive`, then run `python3 packages/commandcode/tests/check_package.py ARCHIVE`.
The runtime layout remains root `magpie-plugin.json`, `source-lock.json`,
`package.json`, native `lib/` and bundled `web/`; no nested ZIP or runtime SDK
checkout is needed.

Fresh checks used regular usbox with final source4 at
`/tmp/magpie-astra-combined-20261115-v4/source`, private workspace/HOME/XDG,
allowlisted environment, pinned offline caches/toolchain and a network namespace
plus descendant guard. [VALIDATION.json](VALIDATION.json) records exact commands,
results, log hashes, selected SDK/helper identities and tested package hashes.
Raw logs remain at `usbox:/tmp/magpie-commandcode-monorepo-20261115`.

| Check | Result |
| --- | --- |
| Full default Go suite | 206 pass events including subtests;0 fail/skip |
| Full Go race suite | 206 pass events including subtests;0 fail/skip |
| Go vet and gofmt check | Pass; formatting performed only on usbox |
| Fixture-integrity negative | A changed fixture is rejected by the unchanged hash guarantee |
| Selected-SDK browser typecheck | Pass |
| Default complete native build | Installable directory; no automatically created archive |
| Root distribution validator | Manifest, source-lock identity/SDK/version and native entry pass |
| Explicit archive and package checker | 17 members; every inventory/content/provenance assertion passes |

The initial relocated type import and frozen-source ZIP timestamp failures are
retained in the logs and were corrected before freeze. They are not passing checks.

Optional historical runners (`tests/run_completion.py`, `run_host_completion.py`,
`replay_native.py`, `check_host_snapshot.py`, `check_sources.py`, `systemone_donor.mjs`
and `harness_probe.mjs`) require their explicitly pinned historical inputs. They
are not default monorepo gates and were not used to claim fresh domain acceptance.
The existing sandbox browser and native-owner runtime probes were not rerun in
this relocation pass. Parent owns ordinary install/list/inspect/remove and final
whole-monorepo validation on the combined snapshot. No live vendors, real accounts,
native macOS/Windows execution, installation into a live app, staging, refs,
commits, merges or pushes were used or claimed.
