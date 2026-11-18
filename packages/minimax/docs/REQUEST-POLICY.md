# Supported request policy projection

The package advertises public SDK API 1.12 and `request-policy`. The supported
final projection is the response of the chat `provider.execute` owner.
It uses `RefusedExecution`, with original status/code/type/message and actual
Retry-After timing, after selected credential redaction. It adds no matcher,
timer, account selector, reactive authentication or request replay.

The selected JSON client also uses public `RequestWithPolicy` for the exact
web-search and connector-catalog endpoints. These are named read substeps,
separate from final chat refusal. They repeat only after a host permit, with
the original credentials, body, scope and context. Declarative vendor rules
classify HTTP rate/outage responses after content/request/auth/credit rules.
The host owns classification, waiting, root budget and revocation admission.
See [named reads](READ-POLICY.md) for the exact envelope subset, receipts and
remaining public-helper/host diagnostic gaps.

## Evidence and acceptance

`execute` sends the selected Messages request once. The runtime has already
durably journaled `dispatched` before the handler runs. The handler buffers a
bounded JSON response before publishing `accepted`, headers or data. Transport
failure or an incomplete bounded read remains uncertain without claiming that
the operation did not start.

`chatRejectedBeforeEffect` establishes rejection independently of the policy
rules. It accepts a strict error-only object with unique keys:

- MiniMax `base_resp.status_code` 1008 or 2053, with a string `status_msg`,
  HTTP 200/402/403, and no success, usage, task or output fields. The pinned
  donor's `REFUSAL_STATUS` maps these published insufficient-balance/credit
  codes to 402. That vendor meaning and the absence of success fields establish
  this narrow chat rejection; an arbitrary nonzero code does not.
- An HTTP 4xx Messages `type: error` envelope with a recognized authentication,
  permission, invalid-request, content-policy or rate-limit error type and a
  string message. Permission and content errors are request scoped by default.
  A small declarative permission-error credit preset retains the pinned donor's
  Chinese balance refusal and explicit insufficient/exhausted credit phrases.

Optional request/trace identifiers must be strings. Unknown fields, duplicate
keys, malformed numeric codes, null required fields, contradictory success
fields, non-JSON/encoded bodies and any preceding redirect cannot establish
whole-execution rejection. Redirect transport behavior itself is unchanged.
The redirect body wrapper records that earlier dispatch provenance without
inventing response headers.

The final SDK result is `outcome: not-started`, with fresh facts and separate
`evidence: rejected-before-effect`; no accepted/output events precede it. Rules
describe classification only. Operator rules and all scheduling remain with the
host. Content/request rules precede credit rules; explicit credit prose precedes
the generic permission rule. A generic rate limit has unknown scope, so it
cannot exclude an account or enumerate the pool.

Every response outside this narrow projection retains the existing terminal
boundary. Successful chat publishes acceptance immediately before headers/data
and preserves bytes/usage. SSE always crosses that boundary before its events
are inspected; errors/progress/incomplete EOF cannot become a final refusal.
Media and write tools retain their original early acceptance and uncertainty.
Search/tool substeps never return final account-fallback authority.

## Facts and timing

HTTP 200 with business code 1008 has internal `WireStatus=200` and
`EffectiveStatus=429`. The SDK refusal result and `facts.status` both remain 200;
the host maps the quota classification to route 429. No quota status, reset,
scope, or Retry-After header is fabricated.

An actual single Retry-After seconds value projects a nonnegative millisecond
duration, including explicit zero. An actual HTTP date projects an absolute UTC
reset. The actual validated header is retained as well. Values over the public
30-day typed-duration bound retain their real header without an invalid typed
duration. Duplicate, malformed, negative and overflowing timing remains unknown;
no timing comes from credit expiry, quota display windows or vendor prose.
The 7200-second fixtures never cause a module sleep or replay.

The public DTO has only one status, a completeness flag, optional code/type and
optional timing. It cannot carry the internal effective status, code
presence/validity flags, timing absent-versus-invalid state, provenance,
request ID or output-started flag separately. The internal `FailureFacts`
remain intact; the public projection preserves the original wire status and
representable facts. Unproved/malformed/partial paths are terminal rather than
being relabeled as complete public facts.

The public SDK Error, SearchResult and tool result schemas do not have this
final-refusal carrier. Their terminal retained facts remain internal. Named,
actually read-only search/tool requests use the negotiated policy callback
for classification or a host-permitted repeat of that request;
they do not acquire whole-provider account-fallback authority.
See `FACTS-PROJECTION.md` for each field's routing, diagnostics or private role
and this distinction between terminal serialization and named-step integration.
No public contract extension is invented in headers, diagnostics or error
strings. This integration does not make every MiniMax public path ready.

## Verification boundary

The source baseline is the 31-file facts freeze from
`/tmp/magpie-minimax-facts-20261115`, source digest
`121cea50d04999e45c9a5ff7e3b840ab4c9a374214efe898d15ca234ddf3d6c5`.
All frozen entries matched before these edits. Its documents, source archive
and accepted v4 receipts remain unchanged historical evidence.

Final-chat projection evidence is retained at
`usbox:/tmp/magpie-minimax-policy.9CkOtx0J`. Current named-read evidence is at
`usbox:/tmp/magpie-minimax-workhorse-20261115.VNyyfYMu`; see `READ-POLICY.md`.
All Go execution uses regular usbox, isolated HOME/XDG, pinned offline caches,
disabled module network resolution and the existing descendant network guard.
No native shared library is built or loaded by this work.

`tests/policy_worker.go` is an exec-only test harness around the actual
`NewModule` handlers. Its only runtime adaptations are exec entry metadata and
trusting the bundled synthetic fixture CA through SSL_CERT_FILE; production
HTTP/proxy/TLS verification remains in use. The harness preserves the Go
manifest bytes to avoid changing embedded schema encoding during handshake.
`tests/policy_host.py` installs it through the unchanged frozen host, signs two
accounts in through ordinary auth, and exercises the ordinary gateway.
This proves the public wire/owner path, not native loading or a new installable
native distribution.

The bounded current receipts are:

| Receipt | Result |
| --- | --- |
| `red` | Original facts source fails the new refusal tests behaviorally: facts are discarded and acceptance/output precede the result. |
| `redirect-red` | A preceding same-origin redirect incorrectly authorized final refusal before the provenance check was added. |
| `green` | Focused policy tests pass under the race detector, including timing knownness, selected secret redaction and acceptance controls. |
| `suite`, `vet` | Package `go test -race -count=1 -timeout=5m ./...` and `go vet ./...` pass. |
| `format-check`, `static` | usbox gofmt reports no files; Python AST and the advertised SDK minimum checks pass. |
| `host-red` | Original facts worker, ordinary Messages gateway: expected fallback 200/two accounts instead returns 409/one account. |
| `host-messages` | Current Messages success returns 200/one account; credit fallback returns 200/two distinct accounts. |
| `host` | All eight bounded Messages cases pass, as detailed below. |
| `host-control`, `host-old-control` | Original fixture returns whole JSON despite upstream stream:true; Chat returns 409/one account with both workers. The cause and corrected healthy checks are in `CHAT-COMPLETION.md`. |

In the eight-case Messages run, HTTP 200 business-code and HTTP 403 explicit
credit-prose refusals each fall back once and finish with 200 on the second
account. Exhausting both accounts returns route 429 after exactly two
dispatches. The direct result preserves SDK status 200, facts status 200,
code 1008 and the actual 7200000 millisecond delay. Host diagnostics name
`minimax-credit-1008` and account quota classification. Permission and content
refusals return 403 after one dispatch. Malformed refusal facts return terminal 409
after one dispatch; partial SSE preserves paid output without a successful
terminator after one dispatch. Accepted media failure returns 409 after one
dispatch. None regenerates work on the same account.

The initial Chat fallback fixture observed the host choosing the second
account with the projected quota rule and known two-hour timing, then returning
409. The continuation traced this to the fixture's whole JSON response to
upstream stream:true, which the host translation codec correctly rejects.
Valid JSON and usage alone do not prove protocol completion.

`CHAT-COMPLETION.md` records the exact source boundary and causal checks using
the same host and both worker binaries. The corrected fixture returns complete
Messages SSE when requested. Whole Chat success and quota fallback are checked
for 200/one account and 200/two distinct accounts, with content/finish/usage
preserved. The original whole-JSON-for-stream behavior remains an explicit
409/one-account negative control. No host receipt, accepted/output fence or
completion rule was changed.

Setup failures (manifest encoding, synthetic CA trust, and an unlisted fixture
model) and earlier fixture attempts are retained separately. They are not
counted as causal red tests. Every result records its command, exit and removal
of the isolated HOME; each stage also has its external network-guard receipt.

Native environment verification belongs to Dirac through the parent. No native
guard is patched or bypassed. The owner-visible
`/tmp/magpie-native-acceptance-20261115.D2KiOWCp/REPORT.txt` supplies positive
Linux runtime evidence for its frozen SDK baseline, not the current MiniMax
module. Current-module native acceptance remains required. There is
no live vendor/account, reactive-refresh, broad host-suite, independent review,
macOS or Windows runtime acceptance for this delta.
