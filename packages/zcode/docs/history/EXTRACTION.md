# Source evidence

Read-only donor: `../magpie-devzeus-archive`, archive branch
`archive/devzeus-2026-11-12`. The donor runtime references ZCode 3.18.0,
upstream revision `b95ebaff`.

| Source | Behavior retained here |
| --- | --- |
| `internal/plugin/bundled/zcode/index.mjs:37` | Packaged ordinary model limits |
| `index.mjs:1366` | GLM-5.2/5.3 native effort normalization |
| `index.mjs:1820` | Start uses the ZCode JWT and excludes paid beta headers |
| `index.mjs:1885` | ToolSearch aliasing, deferred placeholder removal |
| `tests/native-inference.test.mjs` | No replay after partial output; model-specific reasoning; lane binding refusal |
| `tests/toolsearch.test.mjs` | Search tool request/response names round-trip |
| `lanes.mjs:616` | Native lane binds account, generation, session and raw model |
| `internal/provider/zcode_execution.go` | Durable execution admission, unknown outcome, exact ticket/card ownership |
| `internal/provider/zcode_claim.go` | Claims fenced by authoritative billing user across Personal/Team/Start |
| `internal/provider/zcode_reset.go` | Reset high-water observations, pending claim and acknowledgment |
| `project-token.mjs` | Business credentials and expiring PATs have separate lifetimes |
| `usage.mjs:154` | Ordinary paid quota endpoints, raw PAT authorization and selected Team scope |
| `usage.mjs:176` | Known-zero, remaining/percentage precedence, explicit unknown metrics |
| `tests/usage-native.test.mjs:46` | Five-hour/weekly quota distinct from MCP and Start applicability |
| `account.mjs:208` | Session-authenticated profile must match the selected ZCode user |
| `account.mjs:300` | Z.ai OAuth bootstrap exchanges to business material before PAT |
| `project-token.mjs:230` | Saved explicit location and official access_tokens request/expiry |
| `internal/provider/zcode_reset.go:200` | Reset high-water reconciliation and accepted versus observed distinction |
| `internal/provider/zcode_claim.go:110` | Unknown/observed claims never automatically repost |

The transport is a new Go implementation of the documented ordinary
Messages behavior. The public SDK owns framing, native ABI and host
authority. There are no host-internal imports.

The vendor controller is external Go too. Its bounded JSON journal replaces
the archive's host-private SQLite dependency; it is not an automatic SQLite
state migration. It uses an explicit private dataDir, OS locking and durable
pending-before-dispatch writes. User/selection/task scopes and uncertainty
semantics follow the donor, while generic publication/routing/lifecycle stay
with the SDK. Native startup never initializes business storage.

Read-only local log census on 2026-11-13: 944 lines mentioning ZCode;
634 contained the phrase `failed to resolve`. No credentials, request
bodies, URLs, account identities or raw log lines were copied or printed.
This proves repeated resolution failures were recorded, not their cause.
The new module has no Bun/package resolver dependency. This is not evidence
that all historical vendor failures are fixed.

Pure protocol fixtures are synthetic. No recovered cassettes, credentials,
user prompts or live sessions are copied. Prompt fingerprint replacement
from the donor is intentionally not performed.
