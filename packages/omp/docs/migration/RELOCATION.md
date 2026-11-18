# OMP package relocation

Plan recorded before implementation, 2026-11-15. This is a layout/build-tool
change within `packages/omp`; original repositories and the parent's backups
remain read-only. The root `docs/migration/source-map.json` records the copied
tracked and untracked inputs.

1. Preserve all native/session codecs, embedded manifest and `src/testdata`
   bytes. Keep the public Go module identity and all six conversion routes,
   Source reader, lineage checks and immutable input guarantees.
2. Run the Go module from `src/`. Update its development SDK replacement to
   `../../../../magpie`; select an explicit/environment SDK through the root
   `scripts/_workspace.py` context manager and a temporary modfile.
3. Make `scripts/build.py --output DIRECTORY [--sdk SDK_PATH]` the ordinary
   complete native distribution builder. Keep the historical preview-named
   wrapper forwarding to this same full typed factory.
4. Resolve Python helpers directly from `scripts/`, without installation.
   Retain the provenance and immutable-input regression assertions; update
   their explicit paths, temporary-modfile checks and fixture SDK layout.
5. Keep provenance documents under `docs/` and history under `docs/history/`.
   Rewrite only relocation-derived local references, retaining donor hashes,
   historical acceptance identities and limits. Hash the shared author build
   helper as an actual build input as well as package scripts/docs.
6. Add a short package README and update active documentation commands/links.
   Run package tests, vet, affected Python checks, native build and an ordinary
   install/inspect/checked-roundtrip fixture only on isolated offline usbox.
   Parent owns final whole-monorepo verification.

## Completed relocation and freeze

The package is frozen after the checks below. No additional file moves were
made after the parent's copy: every old source-map target remains present.
`docs/history/ORIGINAL-AGENTS.md`, `docs/history/PLAN.md` and
`docs/history/VERIFICATION.md` retain their original bytes. The old full README
is `docs/REFERENCE.md`; only its current paths/links/commands were adjusted.
Donor hashes and historical acceptance identities are unchanged. Local
references in `docs/source-lock.json` now name paths from this package root;
upstream donor paths still name their own repositories.

| Touched package paths | Relocation change |
| --- | --- |
| `scripts/build.py` | Public shared `build_environment`, Go cwd `src/`, `--output`/optional `--sdk`; removed the duplicate local modfile implementation |
| `scripts/provenance.py` | Hash scripts/docs at their real locations and the shared workspace helper; retain actual Go-selected SDK/source, artifact and manifest hashes |
| `scripts/build_source_preview.py` | Compatibility name forwards to the same full builder and accepts the same flags |
| `src/go.mod` | Development SDK replacement `../../../../magpie`; module identity/dependencies unchanged |
| `src/source_version_test.go` | Resolve relocated lock and its package-relative watchpoint/receipt paths |
| `tests/test_build_identity.py`, `tests/test_build_provenance.py`, `tests/test_build_source_preview.py` | Explicit source/helper imports and relocated fixtures; retain mutation, checksum, identity, no-overwrite and complete-manifest checks |
| `tests/verify_host.py`, `tests/verify_source_owner.py` | Default fixture root is `src/`, retaining fixture contents and host-owned verification behavior |
| `docs/source-lock.json` | Local reference paths only; original donor/acceptance hashes retained |
| `docs/REFERENCE.md`, `docs/SOURCES.md`, `docs/SOURCE-PREVIEW.md`, `docs/CODEX-EXPORT.md`, `docs/CODEX-LINEAGE.md` | Active relative links, fixture names and build/check commands |
| `README.md`, `docs/migration/RELOCATION.md` | Short current entry point and this plan/result record |

All other Go source, embedded `src/magpie-plugin.json`, `src/go.sum`, and all
`src/testdata` files remain byte-identical to the copied originals. No runtime
or codec logic was edited. Default output still comes from the typed native
factory and includes all six conversion routes and `omp-history` Source.

## Verification

All execution was on regular usbox using the source4 SDK at
`/tmp/magpie-astra-combined-20261115-v4/source`, content SHA256
`609a7b5a1bfd8da599639900e19a2635296b5f5d1ce85bcbd8df0df702d5b493`.
The private verification workspace was
`/tmp/magpie-monorepo-omp.bPLg7zM0/workspace`. Its copied shared helper includes
the parent's complete-token GOFLAGS quoting fix, SHA256
`1f386b59297faeddfa9b1c1f0b8d0dd25954dbe7322de161b62f32c99060442e`.

| Check | Result |
| --- | --- |
| `python3 -m unittest discover -s tests -p 'test_*.py' -v`, cwd package | PASS:9 tests; uses standard-library mocks and local fixtures, no external SDK/app/client required |
| `gofmt -l` on all `src/**/*.go` | Clean |
| `go vet ./...`, cwd `src`, temporary SDK modfile | PASS |
| `go test -race -p=2 -timeout=5m -count=1 -json ./...`, same cwd/modfile | PASS:62 top-level tests +93 subtests; no skips/failures |
| `python3 scripts/build.py --output ABS --sdk ABS`, cwd package | PASS:full default native `omp-sessions`0.1.0, API1.10/nativeABI1, six routes and one Source |
| Active Markdown relative links | All resolve; immutable historical receipt contents were excluded |
| `python3 tests/verify_host.py FIXTURE_BINARY DISTRIBUTION` | PASS:ordinary native add/inspect, checked preview/apply, exact journal/blob round trip, unauthorized blob path rejected, blob reuse grace and private permissions |

The fixture host binary was built from frozen source4 into the private result
directory. No installed/live app was used. The runner used a fresh explicit
HOME/USERPROFILE/XDG/APPDATA/TMPDIR, Go1.26.8 and pinned existing offline caches,
GOENV/GOTELEMETRY/GOPROXY/GOSUMDB off, GOTOOLCHAIN=local and GOWORK=off. Temporary
modfile paths deliberately contained spaces. Every stage ran in an unprivileged
network namespace with capability drop and the established descendant network
guard; external denied/local allowed probes passed. All temporary homes were
removed. Tested source bytes were rechecked after execution.

Build input fingerprint:
`04a0d44d12a1d7103503921febf1bdad4ed902a01b9401cbdbfc3e0fd4ae8c1c`.
Native library SHA256:
`072a91c32da495139905884bda60ae8d7ff6ff9a0e1e7b335bb9e9c233428a25`.
Fresh logs, commands, guards and provenance are retained locally under
`/tmp/magpie-monorepo-omp-results` and on usbox under
`/tmp/magpie-monorepo-omp.bPLg7zM0`; they are distinct from historical receipts.

Root registry extra check: `{python} -m unittest discover -s tests -p test_*.py`,
cwd `packages/omp`. It needs the checked-in package and shared workspace helper,
no installed Python package or external fixture. There are no browser tests.
The root builder can invoke `scripts/build.py --output ABS --sdk ABS` directly.

Remaining limits: Linux/amd64 synthetic verification only. Optional
`tests/verify_codex.py` and `tests/verify_source_owner.py` require explicit,
reviewed external fixtures; they were not rerun and do not block the current
default build/unit entry points. No live client/vendor/auth/platform readiness
claim. Parent owns final combined-monorepo checks/build/install and independent
review. No further package relocation issue is known. No commits, staging,
merges, publication, new dependencies, original/host edits or shared-root
tooling edits were made by this lane.
