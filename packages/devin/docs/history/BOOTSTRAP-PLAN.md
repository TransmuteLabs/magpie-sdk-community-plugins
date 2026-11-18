# Selected-account native bootstrap

Owner: Devin Executor. Stable module base: c0bcae3. The existing paired
distribution remains SDK 1.4; the new optional agent distribution is 0.5.0
and requires the new SDK 1.6 capability. Codec negotiation uses minor 0.5.
Accepted SDK 1.5 is not extended.

Host development is in ../magpie-native-account-ops, detached from
f4232ea76. MAIN/Source6 integration and durable cold CLI catalog are
leader-owned. Never commit/publish this consumer as self-contained while
its SDK 1.6 host contract is absent from the selected SDK source.

## Extraction and implementation

- Add finite native account codecs separately from chat/discovery. Bind
  GetUserStatus, GetCliTeamSettings and GetAccountManagedPlugins to declared
  read actions, and AssignModel to a declared write action. Recovered
  generated descriptors own the wire, not inferred field names.
- Reuse selected credentials, configured origin, scoped network proxy and
  existing module.call. Never forward the local agent key or let body data
  choose a provider/account. Bound account request/reply bodies to 1 MiB.
- Remove the previous unconditional 501 bootstrap adapters. Preserve the
  default paired and inbound-only behavior. Enable assignment-token claims
  only in the optional agent variant.
- Extract the existing origin-bucket CAS loop into persistOriginRecords
  without changing thinking/Gemini receipt behavior. Regression coverage
  already exists for those origins and must remain green. Assignment
  receipts additionally bind the recovered cascade identity.
- Accept assignments durably before the vendor request, require an exact
  selected model, and persist the returned opaque token's digest under the
  selected account generation/caller/model/cascade. Lost acceptance and
  post-dispatch failures remain uncertain. No retry or endpoint invention.

## Verification

Build and test on usbox only, isolated HOME/XDG and offline caches.
Run complete module/SDK regressions with race checks, JS/TS contract parity,
real native-host bootstrap/assignment/continuation and no-replay checks.
Retain ordinary paired live-generation regression.

Build an unmodified committed snapshot of the recovered Rust application
under a separate target directory. Its existing DEVIN_SERVER_URL test
redirect is distinct from original CLI parity. Use a loopback-only libc
network guard, fresh synthetic credentials, no preseeded status/session,
ordinary SDK profile adoption and run, and an owned local fake vendor.
Do not change source or use the live installed application.

Full original Cognition binary, native non-Linux execution, cloud/ACP,
arbitrary routing-group selection, remote credential delegation and
unrecovered methods remain outside this proof. Known read/assignment
schemas must not be mislabeled unrecovered or replaced with fake success.
