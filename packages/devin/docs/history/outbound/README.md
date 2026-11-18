# Devin outbound client

This domain layer serves the native module's SDK provider/auth/models/usage
handlers in version 0.3.0. The old 0.2.0 inbound-only entry point remains
available separately. No implicit CLI execution, credential-file reading,
browser launch, account switch or background cache ping occurs.

`Client.Chat` converts canonical Chat to GetChatMessage protobuf, sends the
real Connect request shape and decodes streamed native replies. A supplied
acceptance callback runs immediately before HTTP dispatch. Missing/invalid
input is rejected before that callback. An acknowledgement failure is
uncertain because the journal may already contain acceptance; no HTTP
request is then sent. Every error after dispatch remains uncertain and is
never automatically retried. HTTP redirects are not followed.

The codec keeps text, unsigned thinking, inline user/tool-result images,
function tools/calls, cache marks and supported generation controls. Native
max-newlines/top-k defaults are 400/40, taken from the installed plugin;
caller-supplied zero sampling values are preserved, not changed to epsilon.
System prompts use the recovered SYSTEM_PROMPT role. Prompt fingerprints,
injected tool descriptions, missing-result synthesis and role coercion from
other implementations are not copied. Unsupported canonical/signed/custom
semantics fail before acceptance.

`codec.NativeDelta` retains native signatures, signature type, phase, actual
model UID and exact raw frame payload, plus tool deltas and token counters.
The stream requires native stop and terminal records, validates complete
tool argument JSON, bounds gzip/frame/tool sizes and rejects truncation,
redaction/model-change controls it cannot represent, and data after stop.
An explicit output-token zero remains zero. No estimated usage is invented.
`ChatRenderer` produces canonical Chat JSON or SSE with text/thinking,
reversible tool IDs, complete arguments, signatures, exact usage and a unique
response ID. It refuses unrepresentable phases/stops, never sends successful
completion after failure and bounds buffered content/tool arguments.
The paired native path forwards validated raw frames instead of translating
them through this renderer.

`Client.Models` reads the selected account's GetCliModelConfigs. It returns
actual concrete UIDs, explicit availability, image/tool/thinking features,
window/output limits and the native Free tier. It invents no model aliases,
prices, snapshot availability or assignment tokens. Native routers and
disabled models are not offered as directly executable concrete models.

`Client.Status` reads GetUserStatus using that account's credential envelope.
Quota billing with a reset and an omitted remaining share means used-up;
a non-quota plan with an omitted share stays unknown. Free-model exclusions
are preserved in the domain result and published through SDK 1.4
`UsageWindow.NotModels`; the handler echoes the host usage revision.
Live reads omit historical `AsOf` timestamps so the host can acknowledge
that revision under SDK 1.4's freshness contract. Deferred/unreadable scope
reports remain explicitly deferred and cannot clear a dirty quota epoch.

`PreparePKCE` and `Client.Exchange` implement the recovered S256 browser
exchange with a checked 127.0.0.1 callback, state, deadline and two generated
request fields. A module/controller must persist and atomically consume the
flow through SDK state before using it. The root module's `provider_auth.go`
implements that bounded one-use controller; the pure client neither owns
host persistence nor exposes the verifier in an authorization URL.
Returned webapp/API hints never redirect the credential's API server.

The installed community implementation uses API keys and Devin session
tokens without a rotating refresh endpoint. No Firebase/OAuth refresh,
JWT verification, inferred token extension or invented revoke RPC is added.
Explicit known credential expiry is checked before dispatch; static keys
have no invented expiry. Credentials are supplied by the owner, never found
from the live machine. SDK 1.3's transactional coordinator should be used
when any future operation actually replaces credentials.

The root module implements SDK adapters, canonical Chat rendering, PKCE,
origin receipts and the bounded paired bridge. Only native ingress requests
carry the required `SameModuleExecutor` policy. Ordinary Chat can use normal
host routing; it fails explicitly when its input/output requires native-only
semantics. The host can translate its other supported public protocols to
Chat without the module importing any private gateway package.

Source and offline verification are recorded in `EVIDENCE.md`. All tests and
builds run on usbox with an isolated HOME/XDG and the declared Go toolchain.
