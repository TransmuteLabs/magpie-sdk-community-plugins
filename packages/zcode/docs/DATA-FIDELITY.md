# Source-preserving ZCode diagnostics

The module reads explicitly supplied CLI diagnostic JSONL and immutable
checkpointed SQLite snapshots. It neither discovers installed application
data nor treats log records as conversation messages.

Source: ZCode desktop 3.18.2 / CLI 0.17.1,
`fec975905a2abd4c07e35c40d51fa097a2258ec9`.
The CLI log contract comes from
`apps/zcode-cli/packages/adapters/src/logging/{index,serialize,retention}.ts`.
SQLite schemas/writers come from
`apps/zcode-cli/packages/adapters/src/storage/session-store/{migrations.ts,repositories/usage.ts}`.

`zcode-cli-log-jsonl` preserves exact source bytes, unknown fields, context,
session/turn/trace/span identities and integer precision. Timestamp/level
validation distinguishes malformed records. The writer's incomplete final
line remains in the bundle and is reported as an incomplete tail, without
inventing a parsed record. Exact bundle restoration works; diagnostic
logs cannot be silently converted into session history.

CLI history snapshots optionally retain model_usage, turn_usage and
tool_usage rows. Attempts, logical request identities, status, token/cache/
reasoning columns, nullable provider metadata and raw_usage_json remain
available alongside the exact original binary snapshot. They do not
double-count conversation token totals. Existing byte/row/time, immutable
read, schema and resource limits still apply. A present malformed
observability table fails explicitly instead of becoming absent.

This reuses the existing locked modernc runtime and public owner tools.
No new dependency, collector, runtime, financial importer or WAL-copy
shortcut is introduced. Seven-day log retention is the source writer's
policy, not an instruction for the module to delete source data.

Verification remains usbox-only with offline caches, private HOME/XDG and
loopback namespaces. Compiled old-code assertions for missing native log
support and omitted SQLite usage rows are retained in
`/tmp/magpie-zcode-completion-T8c1l5jW` under `log-old-red` and
`data-old-red-compiled`. `data-old-red` was a compile failure and is not a
behavior-negative proof. The subsequent full module race run passed 369
test/subtest events against committed shared host `87359a7`; later final
artifact and causal acceptance are recorded separately.
