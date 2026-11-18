# ZCode relocation

## Plan (before edits)

Preserve all copied source and original evidence. Keep runtime Go/browser bytes and full 0.15.0 behavior, including the audited quota/poll corrections. Change only layout-derived paths, build API, provenance regression locations and documentation navigation.

1. Use the shared build environment with Go cwd `src`, explicit output and optional SDK selection; retain explicit `--headless`.
2. Repair verification consumers for `src`, `web`, `tests`, `scripts` and `docs`; keep Go testdata under `src/testdata`.
3. Preserve donor hashes/versions and historical receipts; update local regression paths separately.
4. Add a short current README and run isolated offline package checks/builds on regular usbox. Record exact checks and limitations below.

Original repositories and backups remain untouched. No dependencies, commits, index changes, merges or publication.

## Results

Code, build scripts and tests are frozen in [CODE-FREEZE.json](CODE-FREEZE.json):
170 paths, content SHA256 `91eccaf23e5355cfeaea9c31babefcda99a56d0d9789908e4bbc67984da95ac6`.
This is relocation acceptance, with the existing retry regression failure
recorded below; it is not a claim that the whole domain test suite is green.

### Preserved source

All 233 mapped original files remain represented. 208 are byte-identical;
25 have the path/build/documentation changes listed below. All 55 Go files
not ending in `_test.go` are byte-identical to the captured originals. Among
Go tests only `src/sources_test.go` changed, to load the relocated lock and
resolve package-relative regression paths. No SQLite/runtime change, module
identity change, dependency addition or manifest/capability change was made.

The complete `sources` and `plugin` objects in `docs/source-lock.json` equal
the original objects; only local `watch[].tests` locations changed. Donor
commit/tree/content hashes and historical verification identities remain
unchanged. All 51 historical non-prototype documents retain their exact bytes.
Four additional historical records moved from `docs/` to `docs/history/`:
`AUTOMATIC-HISTORY-REVIEW-PACKET.md`, `PARENT-CATALOG-REVIEW.md`,
`F2-HISTORY-CLOSURE.md`, `SDK14-ADOPTION.md`. Their original hashes and secondary
moves are recorded in [secondary-moves.json](secondary-moves.json) and the
parent-owned source map has been reconciled.

The audited quota/poll corrections and their regression remain unchanged:

| Source | SHA256 |
| --- | --- |
| `src/gift.go` | `68cf2aebdacbc7c162ff6d8904d3539ecd6b942f7c0fe62be5f0ee1890090044` |
| `src/lanes.go` | `f28a7d61f11e59c9f4291b3b1318d2e03ef33aa76be1fb301b84a280c9c28af4` |
| `src/gift_lane_regression_test.go` | `df478784f19153518bffdfaab9ee2b9d83b71db2e1c25c68f1204e4a7454fbde` |

### Changed paths and purpose

- `scripts/build.py`: shared `build_environment`, explicit optional SDK,
  Go cwd `src/`, docs/web inputs, raw installable output and optional separate
  `--archive` directory. Nonempty output and nested archive destinations are
  refused. Full 0.15.0 stays default; `--headless` stays explicit 0.7.3.
- `src/go.mod`: only local replacement becomes `../../../../magpie`.
- `src/sources_test.go`, `docs/source-lock.json`: relocated lock and local
  regression paths, preserving valid relative paths and immutable donor data.
- `tests/{auth_e2e,e2e,ui_auth_e2e,ui_e2e}.py`: Go source root, package output
  and browser paths. These historical installed-fixture entry points were not
  replayed as new installed-host evidence in this pass.
- `src/testdata/agent/{run,verify}.py`,
  `src/testdata/automatic-history/verify.py`,
  `src/testdata/completion/verify.py`,
  `src/testdata/donor0166/provenance.py`,
  `src/testdata/observability/{installed,verify}.py`,
  `src/testdata/parity/{default_package,provenance,ui_verify,verify}.py`:
  source/docs/web/test paths and explicit archive selection for old ZIP
  consumers. The default-package regression also compares raw member contents
  with the explicit archive. Go testdata remains under `src/testdata`.
- `web/auth-http.mjs`: fixture import now points to `src/testdata`.
- `docs/history/prototype/ui-sdk15/control.ts`: compatibility re-export points
  to active `web/control.js`; no browser domain change.
- `docs/{COMMUNITY-PARITY,ONE-BUNDLE-AUDIT,REFERENCE,SOURCES}.md`: navigation and
  current build commands. New short `README.md`, `docs/history/README.md`,
  migration receipts and `tests/browser.py` provide current entry points.
  The browser runner compiles and bundles existing sources and executes the
  existing tests; no new runtime logic is introduced.

### Verification on regular usbox

Evidence directory: `/tmp/magpie-zcode-relocation-klctZH90` on **usbox**.
SDK: `/tmp/magpie-astra-combined-20261115-v4/source` (final source4).
Physical content SHA256 `609a7b5a1bfd8da599639900e19a2635296b5f5d1ce85bcbd8df0df702d5b493`;
archive SHA256 `8044e33e10a809720beb43ac2c1afc3df7893c00ba67e39cbd2686e66d241502`.
This is a captured working source, not a newly committed Git tree.
Used the existing Go 1.26.8 cache descriptor, pinned TypeScript and Bun,
private HOME/XDG/TMPDIR (including a path with spaces), offline Go settings
and source4's per-descendant seccomp network guard. The guard permits loopback
and exact assigned local addresses and denies external destinations; it is not
an isolated network namespace. No actual credentials, user config, vendor
service or installed application was used. All builds/tests/format checks
ran remotely. Local work was restricted to source editing and metadata reads.

| Check | Result / evidence |
| --- | --- |
| `gofmt -l` all Go source | Empty; `gofmt.log` |
| `go vet ./...` from `src` | Exit 0; `vet.log` |
| Entire module `go test -p=2 -race -json -count=1 -timeout=5m ./...` | Exit 1: 511 pass events, 2 fail events (one subtest plus parent), no skips; `race.log`, `result.json` |
| Cleanup / ingress / native peer Python unit suites | 5 / 4 / 7 tests pass; `cleanup.log`, `ingress.log`, `peer.log` |
| Browser strict TypeScript and existing tests | Node 34/34, Bun 34/34; `browser.log` |
| Full default raw output with explicit archive | Exit 0; raw/archive member bytes equal, provider/controller/agent/UI contract and source lock verified; `full.log`, `full-observed.json` |
| Actual default raw build, SDK via environment, no archive flag | Exit 0; root manifest/entry/lock valid, version 0.15.0, no ZIP/checksums in installable tree; `plain-result.json` |
| Explicit headless raw build | Exit 0; `headless.log` |
| Full/headless native ABI conformance | Both pass: two concurrent sessions, five calls each, missing admission refused, no network callbacks; `full-abi.log`, `headless-abi.log` |
| Reject nonempty output / archive inside output | Expected exit 1; `reject-nonempty.log`, `reject-nested-archive.log` |
| Temporary SDK selection | Source `go.mod` and `go.sum` unchanged; output only at requested destinations |
| Python syntax / current documentation links | All parse; no unresolved links in current README/root docs |

Initial harness failures are retained: `attempt1-transfer-metadata*` records
Mac tar metadata entering the temporary overlay (removed, recopied without
xattrs); `focused-attempt1-transfer-incomplete*` records a replay starting
before baseline copy completed; `attempt2-*` records the new browser runner
initially omitting SDK/app bundles. The final browser runner includes the
same bundling steps as the package builder and passes unchanged tests.
These setup failures are not attributed to runtime code.

### Historical retry test limitation

`src/completion_test.go:345`,
`TestCompletionPaidBusyRetriesOnlyConfirmedUnusedCaps/3009`, observed five
HTTP handler calls where three were expected in the full module run.
`completion_test.go`, `paid_dispatch.go`, `module_test.go`, `gift.go` and
`lanes.go` match both the original working snapshot and source-map SHA256.
An exact copied original snapshot is retained under the remote evidence
`baseline/`; no original repository was modified.

Paired focused `-race -count=20` comparison uses the same frozen SDK,
private environment and guard for original and relocated source. Both runs finished exit 1: original 180.81s, relocated 181.11s; each has
172 pass / 8 fail events (including parent test events), no skips. The copied
baseline reproduced four/five attempts as well. Final paired results are
recorded in `focused-result.json`, `baseline-focused.log` and
`relocated-focused.log`. The respective log SHA256 values are
`ae07acb15e0700a99cec563de931070f8a13ac025a43aec5cbe160e58132d2ce` and
`319725a60e49d79764c59da8ec7dbb7ce42431bcb34cc1bb311d72282f5518fe`. This establishes that the failure is not introduced
by file relocation. The cause has not been isolated; no claim of a specific
timing or external-interference cause is made. No assertion, test-isolation
or retry-domain change was made. Parent's independent combined-tree full
suite remains required.

The later package's shared-policy and repeated strict-grant evidence is
recorded in [native adoption](../NATIVE-ADOPTION-EVIDENCE.md#retry-regression-after-relocation).
That later source has a different SDK owner path. It does not establish the
original anomaly's cause or change these retained relocation results.

### Commands and limits

From monorepo root on regular usbox with the same isolated tool environment:

```sh
python3 -I scripts/build.py zcode --output /tmp/plugin-builds --sdk /path/to/frozen/magpie
python3 -I scripts/check.py zcode --race --sdk /path/to/frozen/magpie
```

The root dispatcher appends `zcode` to its output parent. Direct package build:

```sh
python3 -I packages/zcode/scripts/build.py --output /tmp/zcode-installable --sdk /path/to/frozen/magpie
```

Optional `--archive /tmp/zcode-archives` creates the ZIP outside that
installable directory. `--headless` selects only the historical provider
variant. Extra unit commands, cwd `packages/zcode`:

```sh
python3 -I src/testdata/agent/cleanup_test.py
python3 -I src/testdata/agent/ingress_test.py
python3 -I src/testdata/native-contract/peer_test.py
python3 -I tests/browser.py --sdk /path/to/frozen/magpie
```

Linux amd64 was exercised. Darwin/Windows, live auth/billing/vendors,
GUI/browser automation and an ordinary host install/describe were not rerun
by this lane; parent owns the common all-package build/check/install replay.
Historical replay drivers require their explicit old fixture bundles; their
receipts are not new verification. `src/testdata/donor0166/collect.py` stays
byte-identical and intentionally consumes an original-layout external bundle.
Original repositories, host, backups, refs and index remain untouched by
this lane. No commit, merge, stage, push, install or new dependency was made.
