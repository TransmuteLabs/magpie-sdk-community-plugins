# OpenCode

One complete native Magpie plugin: OpenCode Go/Zen providers, agent wiring, device sign-in UI and consented local session history. The current unpublished working artifact is **opencode0.8.0**, SDK minimum **1.12**, native ABI1, including shared request policy. It needs no companion plugin, runtime Bun or npm install.

The [request-policy contract](docs/REQUEST-POLICY.md) records exact rejection
proof and intentionally unrecovered flows. Every rebuilt compatibility
constructor also requires API1.12; the legacy `--compat-api8` flag no longer
advertises an API1.8 runtime. Existing package versions are unchanged.

The normal verification target is regular **usbox** with a fresh HOME/XDG,
pinned offline caches and the network guard. The explicitly authorized
[November16 local closure](docs/LOCAL-CLOSURE-20261116.md) additionally records
isolated macOS checks with the parent-proven sandbox. From the monorepo root:

```sh
python3 scripts/build.py opencode --output /tmp/magpie-plugins --sdk /path/to/magpie
python3 scripts/check.py opencode --sdk /path/to/magpie
```

The root dispatcher places this package in `/tmp/magpie-plugins/opencode`. The package builder's direct API, from this directory, is:

```sh
python3 scripts/build.py --output /tmp/opencode-dist --sdk /path/to/magpie
python3 tests/check_distribution.py /tmp/opencode-dist
node tests/zen_tools_oracle.mjs --check
node tests/zen_policy_oracle.mjs --check
```

The output directory must be empty. `--sdk` defaults to `MAGPIE_SDK_SOURCE`, then the sibling Magpie checkout. Builds use a temporary modfile and never rewrite `src/go.mod`. Go1.26.8, the pinned TypeScript compiler and Bun are build tools; the generated browser bundle is self-contained. Default builds include the native library, complete manifest, web UI and provenance.

`src/` retains the public Go import identity `github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode`. `src/cmd/native` is the default. Explicit `--compat-provider`, `--compat-agent` and `--compat-api8` retain historical/test variants; they are not additional plugins to install.

Current [relocation receipt](docs/migration/RELOCATION.md) separates new checks from prior evidence. See [detailed reference](docs/REFERENCE.md), [donor sources](docs/SOURCES.md), [source lock](docs/source-lock.json), [history contract](docs/HISTORY_CONTRACT.md) and [history UI](docs/HISTORY_UI.md). Frozen prior plans/receipts remain under `docs/history`; original packet runners are preserved under `tests/history` and are not current workspace commands. Linux synthetic tests do not prove live vendor access, macOS permissions/GUI or Windows/WebKit readiness.
