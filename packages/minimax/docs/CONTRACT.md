# Runtime contract

Both providers run in one native C ABI module (`minimax`, ABI 1, public SDK
minimum 1.12, with `request-policy`). Chat models explicitly use the Messages/Anthropic protocol;
media models use their declared chat envelopes. The provider declares both
protocols so the ordinary host can convert incoming chat requests for its chat
models. The native module owns no scheduler, account store or background daemon.

Every selected operation requires matching provider, account and generation in
the SDK scope and frame credentials. Custom JSON parameters cannot replace
those credentials. Expired selected credentials are refused before dispatch.
The host must apply its normal refresh preflight, including selected tools.
Refresh exchanges are single-use host transactions; the plugin never retries
a rotating refresh token. A rejected or uncertain exchange is returned to the
host for durable handling. No request proxy is persisted into account state.

Published tool schemas admit the optional `_magpie: {provider, account}`
property for ordinary MCP calls. Both fields must be nonempty strings; no
other selection fields are declared. The existing host account owner resolves
the saved account and generation and consumes `_magpie` before module dispatch.
The module rejects that property if it reaches tool arguments. `site` and
`account` arguments only assert the already selected scope. Omitting selection
does not choose the first saved account; SDK calls with a host-preselected
scope can still omit `_magpie`. This declaration adds no account store,
credential authority, refresh or replay path to the module.

Unexpected HTTP 401 never starts a vendor retry or credential refresh here.
Complete supported chat refusals use the negotiated final-refusal contract
described in [request policy](REQUEST-POLICY.md). Only the host can decide
whether a proved refusal permits an eligible sibling. Neither stream errors,
progress, an arbitrary business failure nor accepted paid work authorizes replay.

The device authorization workflow persists its verifier and poll code in
host-private scoped state. Its public continuation contains an opaque key.
Each polling invocation claims that state with CAS, enforces its deadline,
resets consecutive temporary-failure counts on pending, and honors slow_down.
Three consecutive temporary failures may be followed by another poll; the
fourth ends the workflow. A consumed or ambiguous exchange cannot be replayed.

The package window declares one device/poll auth binding for each region.
Its page requires an explicit region, including when neither provider has an
account. The page uses only the existing public `auth.start`, `auth.finish`
and `auth.cancel` broker with host flow handles; the region remains fixed
during an active flow. The private device continuation and token exchange
remain in the existing module handlers and host account publication owner.
Page disposal cancels an active or late-returning flow. The builder includes
the window metadata, page assets and public SDK URL presentation helper.
Foreign parent/region, stale configuration/page, disabled package and account
discovery revocation cannot publish a new account through an old page flow.

Account API signatures retain donor query ordering and exact JSON bytes.
Requests follow only redirects within the exact scheme and host. Cross-region
redirects and HTTPS downgrades preserve the account and never spend refresh
tokens. CN alternate-origin fallback is allowed once only for a proved
pre-dispatch connection/certificate failure. HTTP errors, timeouts, business
refusals and ambiguous writes do not qualify. Read endpoint fallback and ledger
pagination are vendor read workflows, not paid-request retries.

Chat sends `Accept-Encoding: identity` and forwards bytes as they arrive,
including tool/reasoning/signature content. An SSE success needs a framed
`message_stop`; transport failure, an error event, or incomplete EOF remains
uncertain. Usage facts preserve explicit zero separately from absent counters.
Quota normalization uses bounded structured code/type/message facts; generic
permission/content refusals and model context limits retain their status.
Account credit exhaustion is separate from model-specific rate limits.

Nonstream JSON chat responses are bounded to the SDK body limit (16 MiB).
A nonzero business envelope inside HTTP 200 is inspected before acceptance or
headers are emitted. An error-only supported credit refusal keeps SDK status
200 and original facts; the host's classification can produce route status 429.
Unknown business codes keep the existing upstream error and terminal fence.
Successful JSON bytes remain unchanged; credential redaction applies only to
failures. The module owns no retry loop.

Daily check-in reads and validates the seven-day panel before attempting a
claim. A scoped durable day fence, operation acceptance and usage invalidation
precede the claim. Unknown claim results are not replayed. The host owns timing
and recovery; this module starts no check-in timer. Credit expiry is display
metadata, not an invented reset time. Incomplete ledgers never become zero usage.

Media transfers contain only explicitly allowed upload headers, never inherited
account credentials. HTTPS destinations must resolve to public addresses; the
actual connection and proxy CONNECT use a pinned address while retaining the
original Host/TLS identity. Redirects are refused. Each transfer is bounded to
64 MiB; inputs also obey the SDK frame/body limit. Large video content beyond
that bound is refused instead of buffered without a bound. Proxy policy is
resolved by the host for the original URL on every request.
An HTTPS proxy verifies its own TLS identity independently from the media
origin's TLS identity; the CONNECT destination remains the pinned public IP.

Live vendor behavior and platform-specific native loaders require separate
acceptance. Synthetic fixture success is not a claim of live-account testing.
