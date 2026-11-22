# Devin

One native `devin-inbound` **0.8.3** package provides Devin ingress, Chat and
Messages execution, account sign-in, models, usage, agent bootstrap and
account-managed bundle lookup. Requires SDK API **1.12**, `provider-signin`,
`provider-saved-signin` and native ABI **1**. Paid cache keepalive remains off
by default. The package has no community runtime dependencies.

Install the complete build directory with
`magpie plugin sdk add /path/to/plugin-distributions/devin`. Open **Providers →
Add → Subscriptions → Devin** to sign in in a browser or enter a key. Repeat
sign-in from the selected account. To replace an existing Community
subscription, prepare every saved account with `magpie plugin sdk auth import`,
then pass all tickets to `auth transfer`. Magpie keeps the existing provider ID
and account aliases. See [onboarding](docs/ONBOARDING.md#import-a-saved-community-sign-in)
for the checks, rollback and CLI credential requirements. Transfer and rollback
have passed with a local mock. Live verification awaits the Magpie and package
builds.
[Provider setup](docs/PROVIDER-PARITY.md) covers CLI commands, usage and current
donor differences.

From the monorepo root, on regular usbox with isolated HOME/XDG and pinned
offline Go 1.26.8 caches:

```sh
python3 scripts/check.py devin --sdk /path/to/magpie
python3 scripts/build.py devin --output /tmp/plugin-distributions --sdk /path/to/magpie
```

The output is `/tmp/plugin-distributions/devin`. The package builder API is
`python3 packages/devin/scripts/build.py --output DIRECTORY [--sdk SDK_PATH]`.
SDK selection uses `--sdk`, then `MAGPIE_SDK_SOURCE`, then the adjacent host
checkout. `--paired-only` (0.5.1/API 1.12) builds the provider without agent
wiring and requires the same sign-in capabilities. `--inbound-only`
(0.2.0/API 1.1) builds the codec-only variant. `--agent` aliases the default.

Package-only Python units, from this directory:

```sh
python3 -m unittest discover -s tests -p '*_test.py'
```

Go code, command variants and embedded manifests live in `src`. Author helpers
live in `scripts`, and fixtures live in `tests` and `src/testdata`.
See [account setup and detailed reference](docs/REFERENCE.md#account-setup),
[provenance](docs/SOURCES.md), and [relocation checks and limits](docs/migration/RELOCATION.md).
Historical plans and receipts under `docs/history` retain their original bytes
and refer to their original layouts and snapshots.
