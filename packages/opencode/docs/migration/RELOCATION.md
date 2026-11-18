# OpenCode relocation

Status: **package frozen; scoped relocation checks passed**. Parent owns independent review and combined monorepo check/build/install. No commits, staging, merges, pushes or dependency changes.

## Plan recorded before implementation

Preserve the audited Go module identity, full0.8.0 default, explicit compatibility entrypoints, donor bytes, auth/history behavior and historical evidence. Adapt only layout-derived build/provenance/test/docs paths. Use the public shared build environment, run Go from `src/`, compile `web/`, package provenance from `docs/`. Protect behavior with the existing audited regressions, reproduce broken relocation paths in an isolated baseline, verify only the package and ordinary native path, then freeze it. Original repositories and the Magpie host stay read-only.

## Layout and touched paths

- `scripts/build.py` now accepts `--output DIRECTORY [--sdk SDK_PATH]`, imports root `scripts/_workspace.py:build_environment`, uses its selected SDK and temporary modfile, and builds in `src/`. Repeated per-package Go environment/SDK lookup is removed. The ordinary output remains the complete0.8.0/SDK1.9 native+web package. Explicit provider/agent/API8 variants and all Go import/module identities remain unchanged.
- `src/go.mod` changes only the local SDK replacement to `../../../../magpie`. `src/source_version_test.go` resolves current docs and package-relative watch locators; `src/history_lock_linux_test.go` resolves the relocated synthetic writer. No runtime Go or browser source changed. Embedded source manifests and Go fixtures remain in `src/`.
- Verification scripts move from `src/testdata/` to `tests/`; donor extraction moves to `scripts/extract_donors.py`. Donor data and provenance remain under `src/testdata/donors/`, byte-identical. Current script roots, sibling helpers and oracle imports now resolve this layout. [secondary-moves.json](secondary-moves.json) records every additional move with before/after hashes for the root source-map owner.
- Seven old packet-bound runners stay byte-identical under `tests/history/`, explicitly labeled historical and excluded from current workspace commands. Prior plans/receipts under `docs/history/` remain byte-identical. These old runners require their original frozen packets and are not monorepo validation.
- `docs/source-lock.json` updates only current `watch.tests` locators and adds the layout explanation. Donor versions, original acceptance identities and hash guarantees remain unchanged; the exact previous lock is retained in `docs/history/source-lock.pre-relocation.json`. `docs/SOURCES.md`, the detailed reference and applicable documentation links explain current paths. The old full reference is also retained under history. The short package README gives current commands.

All140 original source-map records remain present:120 retain literal bytes,20 have path/documentation edits. [preservation.json](preservation.json) lists those20 and records protected-byte checks for runtime Go, web auth/history, donors and original history/runner evidence. It does not substitute a new domain audit. Original repositories and the central backup remain untouched. Transient source tarballs live only in the private usbox evidence directory, not in this package.

## Validation and provenance

All execution was on regular usbox, with fresh HOME/USERPROFILE/XDG/APPDATA/TMPDIR, pinned offline Go1.26.8 caches, GOWORK/GOPROXY/GOSUMDB/GOENV/GOTELEMETRY disabled as applicable, and network guarding. SDK source: `/tmp/magpie-astra-combined-20261115-v4/source`, content `609a7b5a1bfd8da599639900e19a2635296b5f5d1ce85bcbd8df0df702d5b493`. No local build/test/gofmt, live credentials, vendor calls, installed application or adm access.

The frozen pre-edit copy failed the new build API with unrecognized `--output/--sdk`, and its unchanged source-lock test failed because the old `source-lock.json` path no longer existed. The first setup attempt separately exposed the root helper's GOFLAGS quoting bug; parent corrected that shared helper. The package only consumes its public API. The successful baseline provenance failure and final checks use the corrected helper.

| Final check | Result |
| --- | --- |
| `go test -json -race -count=1 -timeout=5m ./...`, cwd `src` |377 test/subtest pass events,0 failures/skips |
| `go vet ./...` and `gofmt -l .`, cwd `src` | Exit0; no format output |
| Full default `scripts/build.py --output … --sdk …` | Native library, full manifest and browser bundle produced |
| `python3 tests/check_distribution.py DIST` | Complete declaration, version/minimum, provenance and self-contained browser imports verified |
| `node tests/zen_tools_oracle.mjs --check` |99 exact donor cases |
| `node tests/zen_policy_oracle.mjs --check` |63 exact donor cases |
| Python syntax |18 current/historical script files parsed without running historical commands |
| `node --test tests/history_ui.test.mjs` with built distribution |6 pass, including visibility/disposal and stale reply suppression |
| Unchanged `tests/auth_ui.test.cjs`, Chromium |1 pass against the built package and actual source4 page bridge; existing sandbox unchanged |
| `tests/storage_e2e.py` through its strict loopback supervisor | Actual ordinary host installs default package and reads synthetic history through its native SQLite reader |

The native check reuses the successful v2 nogui host executable; Go production is unchanged through source4. It is an installed-native synthetic integration, not a new host build or live OpenCode run. Chromium uses an unprivileged network namespace with loopback only/no external route; other gates use the existing per-descendant guard, and the native fixture uses its stricter loopback supervisor.

Setup failures are retained: the initial browser launch failed before assertions, subsequent launches diagnosed an overlong isolated Chromium socket path, and an initial native attempt could not nest seccomp-notification listeners. The final browser run uses a short private `/tmp` directory; the final native run uses the fixture's single strict listener. No product/test assertion was relaxed, no retry was hidden, and no production source was changed for these harness fixes.

[verification.json](verification.json) records all initial and final exits, commands, log/receipt hashes, output hashes and scope limits. Raw logs, initial/candidate source archives and the runner remain at `usbox:/tmp/magpie-opencode-relocation.AaLNo9c6`. They are new relocation evidence; historical receipts are not relabeled as new runs.

## Commands and freeze

From this package directory on isolated usbox:

```sh
python3 scripts/build.py --output /tmp/opencode-dist --sdk /path/to/magpie
python3 tests/check_distribution.py /tmp/opencode-dist
node tests/zen_tools_oracle.mjs --check
node tests/zen_policy_oracle.mjs --check
```

The last two commands are the extra root-registry checks and need no built app/live state. Root tooling owns Go test/vet/format and environment selection. Browser checks additionally require the built distribution, pinned Playwright/browser tools and `MAGPIE_HISTORY_DIST`; auth also requires `MAGPIE_HOST_SOURCE` and `BROWSER=chromium`.

[Frozen source identity](FREEZE.json):142 package files outside `docs/migration/`, content SHA256 `999118bae0efaa8f50a1a3a507bce830cc25f84dcb475c66f1cebd8196b0b646`. Local bytes match the tested remote source; only README navigation was finalized after executable gates. Receipt files are excluded to avoid self-reference. No further package source changes are planned.

Limits: Linux/amd64 synthetic coverage only; no live OAuth/vendor/native OpenCode binary, Mac/Windows/WebKit readiness, all-locales UI or new full domain audit. Historical packet runners and compatibility distributions were not rerun; their unchanged constructors/entrypoints remain covered by package tests. Whole-monorepo packaging/installation and independent acceptance remain parent-owned.
