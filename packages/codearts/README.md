# CodeArts

Standalone native providers for Huawei Cloud CodeArts (`codearts`) and
OfficeAce / AgentArts (`officeace`). Implements OAuth PKCE + DPoP loopback sign-in
(mainland China and international stations), saved-signin import, STS token
refresh, `SDK-HMAC-SHA256` and OfficeAce `V11-HMAC-SHA256` signing, live catalog
discovery, C33 `:benefit` quota isolation and same-day stale balance tracking,
tool-name shortening/restoration, SSE peeking/folding, per-account concurrency
gating, usage reporting, and daily check-in (`account.checkin`). Uses the public
Magpie SDK; no community plugin, Bun, or Node runtime is required.

From the workspace root on regular `usbox`:

```sh
python3 -I scripts/check.py codearts --sdk /path/to/magpie --race
python3 -I scripts/build.py codearts --output /tmp/plugins --sdk /path/to/magpie
```

Version `0.1.0` targets SDK API `1.12` (`provider-signin`,
`provider-saved-signin`, `request-policy`, `module-rpc`, native ABI `1`).

See [source provenance](docs/SOURCES.md) and [source lock](docs/source-lock.json)
for exact donor revisions and file hashes.
