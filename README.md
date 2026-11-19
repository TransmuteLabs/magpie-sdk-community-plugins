# Magpie SDK plugins

One repository for our independently installable Magpie SDK plugins. The layout follows the community repository: packages live in `packages/`, shared development commands in `scripts/`.

[Current status and reference versions](docs/STATUS.md) distinguishes implemented
source, completed verification, pending donor updates and release readiness.

| Package | Purpose |
| --- | --- |
| [Claude Desktop](packages/claude-desktop/README.md) | macOS agent configuration for the application's bundled third-party gateway mode |
| [Command Code](packages/commandcode/README.md) | Provider, agent and decision models |
| [Devin](packages/devin/README.md) | Native protocol, provider and agent |
| [MiniMax](packages/minimax/README.md) | MiniMax Code China and Global providers, tools, media and search |
| [OMP](packages/omp/README.md) | Session conversion and history sources |
| [OpenCode](packages/opencode/README.md) | Provider, agent, history and interface |
| [ZCode](packages/zcode/README.md) | Native protocol, provider, agent and interface |

Each package has the same entry points:

```text
packages/<name>/
  README.md       current instructions
  src/            Go module, native entry points, Go tests and fixtures
  web/            browser interface, when present
  tests/          additional integration and author checks, when present
  scripts/        package-specific build steps
  docs/           reference, source provenance and migration notes
    history/      previous plans and verification records
```

## Development

Use the regular `usbox` development host. Prerequisites: Python 3, Git, the pinned Go toolchain and cached dependencies, a C compiler, and existing TypeScript/Bun tools for packages with an interface. No dependency is downloaded by these commands.

```sh
python3 scripts/check.py all --sdk /path/to/magpie --race
python3 scripts/build.py all --sdk /path/to/magpie
python3 scripts/build.py devin --sdk /path/to/magpie --output /tmp/plugin-build
```

The default SDK checkout is the sibling `../magpie`; `MAGPIE_SDK_SOURCE` or `--sdk` selects another checkout. Builds use a temporary module file and do not rewrite checked-in dependency files. The editor workspace is `go.work`; each `src/` remains a separate Go module with its existing import identity.

`build.py all` uses Go's effective GOOS/GOARCH and reports packages skipped by
their registry platform restrictions. An explicit unsupported package fails
before creating output; `check.py` still runs its platform-independent checks.
Claude Desktop currently builds only for `darwin/arm64` and `darwin/amd64`.

Native Go builds disable automatic VCS stamping: changing local Git metadata
does not change the native output or prevent a build in an unpublished
workspace. Source and SDK provenance remain in the package and build records.

Default builds produce the full current package in `dist/<name>/`. `--output` selects another parent directory. Existing package output directories are refused to avoid mixing releases. Each artifact contains `magpie-plugin.json`, native code, its interface when applicable, and source provenance. It does not require another plugin. Historical compatibility variants are available only through the documented package builder options.

Checks run Go tests, vet and format validation, plus the package's registered unit/browser checks, with an isolated user directory. Actual account sign-in and live harness testing are separate from these offline checks. Build/typecheck of browser interfaces is part of `scripts/build.py`.

ZCode's process cleanup and ingress ownership fixtures run on Linux. They use
`/proc`, `prctl`, and `ss`. The native wire and UI checks also run on macOS.

## Release assets

Each module declares its release version in `packages/<package>/package.json`,
as in the community plugin repository. That file also describes the package's
purpose. The SDK manifest declares the capabilities and compatibility the host
can verify. GitHub Actions compares package versions before and after a push
to `main`. Only a new, unpublished version enters the macOS arm64 build.
**Check plugin versions** catches changes to package build inputs without a
version bump on pull requests and pushes to `main`. It checks package metadata,
source, interface assets, the package's documentation and build scripts.
Dedicated test files are exempt. Checks start after the first commit that adds
the package's version metadata. To release an existing unpublished version, run
**Release plugin versions for macOS** and select the package.

Keep the package version, native manifest and source provenance in agreement.
The build verifies these fields and uses the Magpie commit pinned in
`.github/workflows/release-package.yml`. Update that pin when a package needs
a newer host contract. The current ZCode sources use `RequestHandoff`, which
requires a newer SDK commit than the OpenCode `0.8.0` pin.

The publish job creates `<manifest-id>-v<version>` at the triggering commit and
uploads the ZIP and `checksums.txt`. The verified ZIP includes the package
description alongside its SDK manifest, using the existing window metadata
where present. Magpie installs and updates from these releases.

Retry a failed publication by rerunning the original workflow. An existing tag
can be reused when it points to that commit. An existing draft or a tag on
another commit blocks publication until the conflict is resolved.

### Offline release tooling

The workflow uses `scripts/release_build.py` and `scripts/release.py`.
They also accept explicit offline inputs for preparing release candidates.

The build wrapper accepts exactly one package and an explicit version matching
its source provenance. It checks a supplied complete offline publication
inventory before invoking any builder or reading SDK inputs. An existing
published exact version returns `skip-published`, creates no candidate output
and invokes no compiler. Run the packaging command only after a build or verified
local reuse. Other modules' release events and source changes do not trigger
this module's builder.

The publication inventory is an explicit author/CI input, not a live API lookup:

```json
{"schema":1,"repository":"owner/repository","complete":true,"releases":[{"tag_name":"minimax-v0.1.0","draft":false,"prerelease":false}]}
```

Its owner must supply a fresh complete listing; unknown, malformed or incomplete
state fails closed. The wrapper makes no GitHub requests and cannot prove an
operator-supplied snapshot's freshness. Module IDs may contain `-v`. Inventory
classification first validates the full version after the selected ID. Only
when that is invalid can a complete longer valid module ID plus valid semantic
version identify a sibling tag to ignore; malformed selected tags still fail.
Valid selected prerelease/build suffixes keep their full meaning. Exact module
and version checks remain mandatory for assets and manifests.

The SDK source manifest supplies a
canonical `identity` over its `files` map of SHA-256/mode records. Actual SDK
bytes/modes are verified. Build receipts record that SDK identity, selected
module files, selected registry entry and root builder/tool inputs.
Schema 2 build receipts index the complete distributable file set with SHA-256
and permission modes. After the published-version skip, offline `go env`
determines effective GOOS/GOARCH even when those environment variables are
empty; the builder receives those explicit target values and the receipt
records them. No target lookup or compiler runs for a skipped published version.
`--previous /path/to/build-receipt.json` reuses unchanged
publishable local outputs without invoking a builder; extra, missing, changed
or symlinked files and mode changes fail closed. Older receipts without mode
or target proof cannot authorize reuse or packaging. A different target uses a
fresh independent output at the same version when common package, SDK, registry
and author-tool inputs are identical; it never reuses another target's payload.
Those common inputs have a separate source-input digest. Same-target reuse
additionally checks the complete target/compiler/environment input digest.
Changed common inputs, including SDK changes, or changed same-target build
inputs require a version bump for a release candidate. Once published, the
exact version still skips every build; extra platforms do not authorize
overwriting an existing release. `--manual-dev` explicitly
permits a fresh local/dev rebuild and records `publishable:false`; it never
authorizes replacing a published module/version. Existing output directories
are always refused for a build. The tools do not publish or overwrite releases.

The packaging helper requires the publishable schema 2 receipt emitted by
`release_build.py`, found at `--input/../build-receipt.json` or supplied through
`--receipt`. The same complete payload hash/mode validator guards local reuse
and final packaging. Missing, malformed, mismatched or manual-dev provenance is
refused before creating output. Removing a manual-dev receipt does not promote
the artifact. Plain `scripts/build.py` output has no release receipt and is
local/test output only; use `release_build.py` for release candidates. Receipts
are local integrity evidence, not signatures or proof of publication inventory
freshness; callers must protect source, receipts and artifacts from tampering.
The requested platform must equal the receipt's effective build target even
when the module supports several platforms or omits compatibility platforms.
Only the existing all-Bun `any` case is universal; a native payload cannot be
relabeled as another platform.

The packaging helper accepts one module's verified distribution, verifies its
registry ID and distribution/source-lock version/SDK consistency, and writes
deterministic `magpie-plugin_<id>__<os>_<arch>.zip` plus `checksums.txt`. ZIP
entries have fixed timestamps, sorted names and normalized file modes preserving
executability. Symlinks, special files, unsafe/case-colliding paths and oversized
distributions are rejected. `--platform any` requires Bun for every component.
The platform label must describe the supplied supported build; packaging does
not cross-compile or inspect native machine code. Build this selected module for
each supported platform, combine only its platform assets/checksum entries,
then publish its one version through a separately reviewed publisher.

A host with module-scoped release support installs one module using
`github:owner/repository#<manifest-id>[@tag]`, for example
`github:owner/repository#devin-inbound@devin-inbound-v0.8.0`. The selector precedes
the full module tag pin. Without a pin, the host resolves the highest published
stable version of that module over a bounded complete release listing; another
module's newer release or a later-published backport does not displace it.
Publishing, credentials and workflow activation remain outside these offline
tools. They do not gate or change the separate Magpie SDK application release.

## References

- [Reorganization plan](docs/REORGANIZATION-PLAN.md)
- [Devin and MiniMax implementation scope](docs/PROVIDER-COMPLETION-PLAN.md)
- [Preserved source inventory](docs/migration/source-map.json)
- [Verification](docs/migration/VERIFICATION.md)

`docs/SOURCES.md` and `docs/source-lock.json` in each package record donor repositories and exact versions. Older evidence under `docs/history/` describes its original source snapshot; it is not a claim about the current tree. The original five repositories and their uncommitted changes remain intact as recovery copies during this migration.
