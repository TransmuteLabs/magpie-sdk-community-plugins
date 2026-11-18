# Devin relocation

## Plan and boundary

Preserve the copied 154-file baseline, module identity, all three embedded
manifests and audited Go behavior (including F2a). Change only layout-dependent
author tooling, provenance references and current documentation. Original
repositories, backups and host remain read-only; no Git index/ref operations.

1. Adapt the builder to `--output` / optional `--sdk` and the public root
   `build_environment` helper; compile from `src` with a temporary modfile.
2. Relocate provenance's required-input/watch paths and Python fixture lookup;
   retain exact donor hashes and original historical receipt contents.
3. Retain build/variant/hash regression assertions, add layout/SDK selection
   checks, and move source-directory reference documents into documentation.
4. On regular usbox only, run Python units, Go tests/vet/format checks, build
   all three native variants and exercise a synthetic native/install path.
5. Record changed paths, secondary moves, exact input identities and new
   verification evidence separately from historical receipts, then freeze.

The parent owns shared scripts and final whole-workspace validation. No new
dependency, runtime feature, vendor call, live installation or host suite is
part of this relocation.

## Baseline

All 154 mapped copies matched `docs/migration/source-map.json` before edits.
F2a remains byte-preserved:

- `src/outbound/render.go`: `4d07c706f7074285539e28c6e5f1abe7ba9cfa37854c00607f114f4f64fcf190`
- `src/provider_correction_test.go`: `6557b2b245e92651bad78d11758c164436bdbbc645329110c97ec6f554da2993`

## Terminal result — 2026-11-15

The package is frozen for the parent's combined verification. No Go source,
Go test, fixture bytes, embedded manifest, module ID or runtime behavior changed.
The Go module replacement alone now points to `../../../../magpie`.

Of 154 original mapped inputs, 147 remain byte-identical, including all historical
receipts, donor hashes, `go.sum`, the three manifests, commands and both F2a files.
Seven copied files have layout changes:

| Path | Change |
| --- | --- |
| `src/go.mod` | Relative SDK location only. |
| `scripts/build.py` | `--output` and optional `--sdk`; shared `build_environment`, compile cwd `src`, relocated documentation inputs. |
| `scripts/provenance.py` | Package-relative `docs`, `src`, `scripts`, `tests` inputs and tracked-input map. |
| `tests/provenance_test.py` | Relocated fixtures/imports, isolated fake SDK, builder cwd/environment and no source-modfile edits, current documentation links, Gemini oracle; preserves variant/hash assertions. Git tracking success/failure uses a mocked read-only command, never a temporary staged index. |
| `tests/gemini_host_e2e.py` | Resolve its unchanged fixture from `src/testdata`. |
| `docs/source-lock.json` | Watch/test and manifest locations, current build invocation and relocated local evidence link. Historical commits, hashes, source artifact paths and receipts remain unchanged. |
| `docs/REFERENCE.md` | Current path/command links and an explicit historical-evidence notice. |

New files are the short `README.md`, this report and
[secondary-moves.json](secondary-moves.json). The latter records five byte-exact
secondary moves from `src/{codec,outbound}/*.md` into
`docs/history/{codec,outbound}`. These are historical submodule references,
not current package entry points. No further secondary moves are pending.

Simplification: the builder delegates SDK selection and temporary modfile
lifecycle to the public workspace helper. Runtime modules remain independent;
no helper or documentation directory is required by the installed library.

## Fresh verification

Evidence prefix: `usbox:/tmp/magpie-monorepo-devin.DayUHDmB/`.
Local handoff: `/tmp/magpie-devin-relocation/` (runner, logs/receipts, preservation
inventory, exact package freeze and relocation patch). No historical receipt
is being presented as a new run.

The selected SDK was
`/tmp/magpie-astra-combined-20261115-v4/source`, content SHA256
`609a7b5a1bfd8da599639900e19a2635296b5f5d1ce85bcbd8df0df702d5b493`,
with no Git tree claimed. The copied workspace helper SHA256 was
`1f386b59297faeddfa9b1c1f0b8d0dd25954dbe7322de161b62f32c99060442e`.

All checks ran on regular usbox UID 1000. Each stage used an outer private
user/network namespace, loopback plus a synthetic local fixture address,
dropped capabilities and the existing descendant network guard, with fresh
HOME/USERPROFILE/XDG/APPDATA/TMP and pinned offline Go 1.26.8 caches. The guard's
hardcoded `user_namespace:false` describes its own setup, not the outer runner;
`runner.py` records the explicit outer `unshare`. No harness reads a live home
or account. Stage homes were removed.

| Check | Fresh result |
| --- | --- |
| Relocation regressions before helper/path changes | Expected FAIL: 18 tests, 13 subtest/errors; old builder rejected the new API, and old watch paths were absent. `red.log` and `red-result.json` preserved. |
| `python3 -m unittest discover -s tests -p '*_test.py' -v`, package cwd | PASS, 20 tests. Includes unchanged cache fixture tests and actual Gemini oracle decoding/hash validation. |
| `go test -p=2 -timeout=5m -count=1 ./...`, `src` cwd | PASS, all three tested Go packages and all command packages. |
| `go vet ./...`, `src` cwd | PASS. |
| `gofmt -l .`, `src` cwd | PASS, empty output; no formatting writes. |
| Actual `scripts/build.py --output ABS --sdk ABS` | PASS, default 0.8.0 Linux/amd64. |
| Actual explicit `--paired-only` and `--inbound-only` builds | PASS, 0.5.0/API1.7 and 0.2.0/API1.1. |
| `tests/agent_options_conformance.py` against default library | PASS: four native agent actions, no client files, four strict invalid-option controls. |

Actual libraries, SHA256:

- Default: `c4dca09874b5d7414dd140fb4f53e44e83e432ac10afb3945a3ec2023036df91`
- Paired: `9f53d386b7903702e2b45d097c26905d4ff928e51fc8ce7fe325164faec4232c`
- Inbound: `0142f9d3c3a9f55e21838b906eb9d7391144e62dcfa3a42b3ac6ae453828c948`

## Remaining checks and limits

The parent owns the final combined all-package checks/build/install. No repeated
host suite, live vendor/client/authentication test, new domain audit or
cross-platform build was run. The package has no browser assets. Race testing
belongs to the parent's common check; this relocation pass ran ordinary tests.
Historical standalone/e2e commands in `docs/REFERENCE.md` remain explicit,
optional runners requiring separately supplied frozen app/distribution inputs.
Historical receipts retain old path text; root source-map plus secondary moves
provide their new locations without rewriting that evidence.

A bounded optional CLI probe used the existing source2 host binary: `sdk add`
and `sdk inspect` succeeded for the default distribution in a fresh home.
The probe then stopped at its overstrict raw JSON equality: SDK
`ProviderOption.Default` is `json:"default,omitempty"`, so inspect correctly
omits explicit `false`. The eight native agent-option checks and the original
manifest's default-off assertion pass. This is a harness mismatch, not a
product finding; its `installed*.log`, result and guard receipt are retained.
No successful full installed/describe replay is claimed here; the parent will
run that gate on the exact combined snapshot. The first outer guard setup also
stopped before running tests because its synthetic local address was absent;
the runner added that required fixture address before all recorded stages.

`check_tracked_inputs` is ready for the relocated paths, but actual tracked
membership is intentionally not claimed for this uncommitted, unstaged
workspace. No index, ref, stage, commit, merge or original-repo writes occurred.

## Default commands

From the monorepo root, inside the approved isolated usbox environment:

```sh
python3 scripts/check.py devin --sdk /path/to/magpie
python3 scripts/build.py devin --output /tmp/plugin-distributions --sdk /path/to/magpie
```

Registry extra unit command, cwd `.` relative to `packages/devin`:

```sh
python3 -m unittest discover -s tests -p '*_test.py'
```

The package builder accepts `--output DIRECTORY` and optional `--sdk SDK_PATH`.
Its default remains the complete 0.8.0 module; compatibility flags are explicit.

