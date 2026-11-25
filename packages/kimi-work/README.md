# Kimi Work

Standalone native Kimi Work subscription provider (`kimi-work`), separate from
Kimi Code (`kimi`). Reads the local Kimi desktop Work sign-in and per-account
key ring, supports paired membership token renewal, merges live desktop and
gateway model catalogs (including `-fast` / `priority` speed), normalizes
Moonshot tool schemas while preserving thinking/history controls, translates
spent-quota `403` refusals through SDK request policy, and reports shared
monthly subscription usage. Uses the public Magpie SDK; no community plugin,
Bun, or Node runtime is required.

From the workspace root on regular `usbox`:

```sh
python3 -I scripts/check.py kimi-work --sdk /path/to/magpie --race
python3 -I scripts/build.py kimi-work --output /tmp/plugins --sdk /path/to/magpie
```

Version `0.1.0` targets SDK API `1.12` (`provider-signin`,
`provider-saved-signin`, `request-policy`, native ABI `1`).

See [source provenance](docs/SOURCES.md) and [source lock](docs/source-lock.json)
for exact donor revisions and file hashes.
