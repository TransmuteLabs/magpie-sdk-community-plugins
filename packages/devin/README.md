# Devin

One native `devin-inbound` **0.8.2** package provides Devin ingress, Chat and
Messages execution, account sign-in, models, usage, agent bootstrap and
account-managed bundle lookup. Requires SDK API **1.12**, native ABI **1**. Paid cache keepalive
remains off by default. No community package is a runtime dependency.

Install the complete build directory with
`magpie plugin sdk add /path/to/plugin-distributions/devin`. Sign in to
module `devin-inbound`, provider `devin`, using an explicit API key/session
token or browser PKCE. No retired built-in account or CLI credentials are
imported. [Provider setup and current donor comparison](docs/PROVIDER-PARITY.md)
contains the auth/model/usage commands, verified ordinary paths and remaining
adapter limits. The installed package page supports first-account browser
PKCE through the existing host poll broker and a module-owned loopback callback.
Explicit caller-managed redirects and callback submission remain available to
CLI callers. See [onboarding and its verification limits](docs/ONBOARDING.md).

From the monorepo root, on regular usbox with isolated HOME/XDG and pinned
offline Go 1.26.8 caches:

```sh
python3 scripts/check.py devin --sdk /path/to/magpie
python3 scripts/build.py devin --output /tmp/plugin-distributions --sdk /path/to/magpie
```

The output is `/tmp/plugin-distributions/devin`. The package builder API is
`python3 packages/devin/scripts/build.py --output DIRECTORY [--sdk SDK_PATH]`.
SDK selection uses `--sdk`, then `MAGPIE_SDK_SOURCE`, then the adjacent host
checkout. Explicit `--paired-only` (0.5.0/API 1.12) and `--inbound-only`
(0.2.0/API 1.1) retain historical compatibility; `--agent` aliases the default.

Package-only Python units, from this directory:

```sh
python3 -m unittest discover -s tests -p '*_test.py'
```

Go code, command variants and embedded manifests live in `src`; author helpers
in `scripts`; fixtures in `tests` and `src/testdata`; the sign-in page in `web`.
See [account setup and detailed reference](docs/REFERENCE.md#account-setup),
[provenance](docs/SOURCES.md), and [relocation checks and limits](docs/migration/RELOCATION.md).
Historical plans and receipts under `docs/history` retain their original bytes
and refer to their original layouts and snapshots.
