# Attributing the installed fixture's root requests

2026-11-14. Follow-up to the installed RED in
[AGENT-OPTIONS9-EVIDENCE.md](AGENT-OPTIONS9-EVIDENCE.md). This changes only
the module's Linux acceptance fixture and current build documentation.
Production Go/TS, the shared host, native root ownership checks, dependencies,
package version and source locks are unchanged.

## Actual sender, before a fixture fix

The unchanged lifecycle fixture was copied into a private temporary packet
on usbox. It reused the parent's frozen host binary and the **same existing
endpoint-overlaid native library**, without rebuilding either:

| Identity | Value |
| --- | --- |
| Shared host commit | `46756a48c9c9f204a8369638dc63cbcb83d02031` |
| Frozen physical source tree | `db40ddbf8944b54f9f689d444644af4c3bc76881` |
| Host binary SHA256 | `32fbdeabd20ed3059f61a91f7614a51a2ac9b908b12dd637c204a82918a28256` |
| Same fixture library SHA256 | `57c6c01915dd6b46cde2c031394f6ee5a7972023f0887021930e8a6183122327` |
| Ordinary ZIP SHA256 | `4e893dc37325beb39def0737140250ad5582adfb916fa3885861b4241df62527` |
| Original RED log SHA256 | `7d500ab7dff6b0b8c749038a77dafe72bf9cd2a52027b4ec6ac689f398904810` |

Instrumentation recorded method, raw URI, headers, local/remote tuple,
wall/monotonic timestamps and exact socket inode while the connection was
still open. The first repeat used the already installed `strace -ff` on
the fixture's own descendant tree. Both root requests were received, but
neither client write occurred in that tree. Owned `/proc` descendants did
not hold either client inode. This first repeat remained RED.

A second repeat resolved each **exact tuple** with the installed `ss`,
then checked only the discovered process identities. Both root requests
came from **`moshi-hook`, PID24086**, executable
`/Users/maratkarimov/.local/bin/moshi-hook`, parent PID2186, process start
tick132018, cgroup
`/user.slice/user-1000.slice/user@1000.service/app.slice/moshi-hook.service`.
This process predates and is outside the fixture lineage.

| Received UTC | Client → vendor | Client inode / FD | HTTP Host |
| --- | --- | --- | --- |
| 14:34:35.849152298 | `127.0.0.1:57946 → 127.0.0.1:34361` | `107788582` /12 | `127.0.0.1:34361` |
| 14:34:35.882597276 | `127.0.0.1:57956 → 127.0.0.1:34361` | `107785816` /10 | `localhost:34361` |

Both were `GET / HTTP/1.1`, with `Go-http-client/1.1`,
`Connection: close`, `Accept-Encoding: gzip`, no Authorization and no body.
The exact unfiltered second repeat still failed on the original two
`unexpected vendor GET /` assertions, exit1 in78.20s.

A separate bare Python HTTP server, **without starting Magpie or loading
the module**, received the same two requests in2.20s. Both exact client
socket tuples again resolved to PID24086. Binding a new server to
`127.0.0.2` also received a root request from that process, so changing
the loopback alias is insufficient isolation.

These are observed outside-process probes of new loopback listeners.
They are not inference retries, readiness URLs, proxy-health goroutines
or delayed work belonging to this fixture. No stack of the unrelated
service was collected; exact open socket ownership and the no-host
reproduction establish the sender without tracing or changing that service.
No service was stopped, and no production fix is inferred.

The original RED recorded neither client tuples nor PIDs. These repeats
establish the cause of the reproduced two-request failure; they do not
retroactively provide missing process evidence for every historical run.
The older billing failures and the separate c575 empty-catalog failure
remain historically unattributed.

## Fixture boundary and regressions

`testdata/agent/ingress.py` installs the boundary before starting the vendor
server. For each accepted TCP connection, it matches the client tuple and
inode, confirms the exact PID/FD and process start identity, and follows
that process's parent chain to the fixture root. Only the fixture and its
actual descendants reach the HTTP handler. A verified outside process is
disconnected before HTTP parsing and its identity is recorded. Unknown,
changed or mixed ownership records a vendor error and fails closed.

Private `ingress.jsonl` retains request method, URI, headers, tuples,
timestamps and socket/process attribution. It contains synthetic test
credentials and stays in the private temporary artifact, outside Git.
There is no process-name, GET, path, user-agent or marked-header allowlist.
Every owned unexpected GET/POST still reaches the original strict vendor
assertion. All inference, forwarded forged-header, ledger and final
catalog/account-store assertions in `testdata/agent/run.py` are unchanged.

| usbox gate | Result |
| --- | --- |
| Real own child request reaches strict vendor | PASS |
| Owned unknown GET and POST, including fake probe header, remain errors | PASS |
| Actual outside process is disconnected before HTTP | PASS |
| Unresolved owner becomes vendor error | PASS |
| Final four ingress tests, ResourceWarnings treated as errors | 4 PASS,2.34s |
| Admit outside processes mutation | Behavior RED,1 failed assertion |
| Ignore owned root GET mutation | Behavior RED,2 failed assertions |
| Ignore unknown ownership mutation | Behavior RED,1 failed assertion |
| Full ordinary native lifecycle, same host and library | GREEN,80.63s |

The full lifecycle records **93 owned vendor requests**, zero unattributed
connections and one rejected incoming socket from PID24086. It proves one
Start inference and three native alias inferences, one ticket and one draw,
forged-header refusal, exact task attribution, token/cache ledger values,
Library instructions/MCP/skills, stale/off refusal, disconnect restoration,
one native catalog entry and two SDK accounts. The final post-vendor
assertions now run and pass.

AST parsing checks all three fixture Python files. The final unit source
only differs from the first four-test run by explicitly closing expected
HTTPError responses; the final run has no ResourceWarning. Lifecycle
`run.py` and `ingress.py` hashes are identical to those in the final checks:

| Source | SHA256 |
| --- | --- |
| `testdata/agent/run.py` | `2e3002a00d6a485ba96d93ec1bb9eb79c1515caef7fb8a5dd1e5070f39faf19e` |
| `testdata/agent/ingress.py` | `8c53c6b5a2f7e1ae00b66e54b4d933cc9e31b40510ac9d0064ac3a39680ecf12` |
| `testdata/agent/ingress_test.py` | `94d4255f011190d032d45894dd1a6e7f3ccd865a7807349ae5c91c31d69e10fd` |

Every run uses isolated HOME/XDG and the parent's existing outbound
seccomp guard, with external-connect denial verified. UID remains1000,
`/` remains owner0 and no user namespace is introduced. This fixture adds
an incoming process-ownership boundary; outbound isolation alone did not
exclude the unrelated service.

## Preserved packet and limits

Remote: `usbox:/tmp/magpie-zcode-get-root-9ugjWppr/`.
Local scripts/receipts: `dist/get-root-attribution-yae7Bdrt/`.
The packet retains the first strace repeat, second exact-PID repeat,
both bare-server cases, final lifecycle, causal copies, source and logs.
`collect-attribution.py` verifies the same library, original RED digest,
unfiltered PID evidence, zero-unknown final ingress and genuine negative
assertions. The older packet and `AUTH-WEB-EVIDENCE.md` remain unchanged.
The sanitized collected receipt has SHA256
`bce03954e2eb96a181cb67f7f2951e7547e3f2d7c8aa20094b3529cfaf2bb1d5`;
the GREEN lifecycle log has SHA256
`66bcef315aca16c2d2441acbbe9e9bafe39516c8a3d92cf92f81a2913d13f5d4`.

No broad host gate or new production build is needed for this fixture-only
change. No host source/index was changed or committed. The README's
current ordinary build/runtime requirement is corrected from SDK/API1.7
to1.9, matching `ControlManifest` and `source-lock.json`; historical
API7 descriptions remain historical.

Desktop3.18.2 / CLI0.17.1 and community0.1.66 provenance is unchanged.
Actual installed ZCode client, live vendor/account operations, native
macOS/Windows runtime, automatic DB/WAL capture and financial import/resume
remain outside this synthetic proof. This closes the reproduced installed
fixture failure, without declaring the whole plugin ready or delaying
the independent SDK-only rollout.
