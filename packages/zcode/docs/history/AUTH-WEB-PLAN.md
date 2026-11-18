# Native auth-broker consumer proof

This test cut passed the leader's bounded independent review on
2026-11-13. It verifies a frozen Agent-free host snapshot after schema-4,
recovery and ownership gates; it does not claim a published auth broker
or acceptance of unrelated NEXT edits.

Keep the committed 0.7.0 control page and default headless API-1.4
distribution unchanged. Auth UI remains absent from production manifests.
All new consumer wrappers are temporary fixture modules, using public SDK
imports and the real ZCode/OpenCode modules. The host gains no provider
dependency and no module credentials are fabricated.

1. Snapshot this module and read-only OpenCode source by exact hashes.
   Generate one temporary native wrapper per actual module. Add only a
   fixture window, fixed-provider AuthBindings and a sanitized selected
   account probe. Preserve actual vendor auth handlers and state.
2. Install fixture distributions into a temporary HOME with setup-only
   CLI commands. All authentication must use the ordinary public page
   bridge, actual answerPluginPage parent relay, real web HTTP and captured
   host account owner/Store. Do not use CLI auth or write account records.
3. Run page/parent JavaScript in separate VM realms. Browser execution is
   unavailable on usbox; this explicitly tests the real JS/HTTP/native
   chain, not rendered-browser, visual or browser-origin enforcement.
4. Exercise actual ZCode poll/link and OpenCode device vendor controllers
   against bounded synthetic loopback servers. Check correct saved
   account, expiry, generation and receipt; duplicate start/poll/cancel;
   changed intents, wrong root/parent/binding, stale generation, off and
   cancellation while a vendor exchange is in flight.
5. Audit both directions of every iframe message and all web responses.
   Credentials, module Continuation, PKCE verifier, access/refresh/session
   material and error canaries must never reach the page. Authorization
   URL, public user code, opaque host flow and account summary are allowed.
6. Verify no replay after uncertain exchanges and no publication to a
   replacement/foreign owner. Inspect only the fixture's temporary Store
   for atomic schema-4 receipts. Never inspect a live account database.
7. Run on usbox in a UID-preserving loopback-only namespace with all five
   capability masks zero, temporary HOME/XDG and offline pinned caches.
   Preserve earlier red runs, exact source/binary/asset hashes and explicit
   limits. Commit only after coherent host gates and independent review.

## Preparation and regression scope

`testdata/window-auth` now contains the temporary wrapper generator, bounded
synthetic vendor, actual bridge/parent realm loader and real HTTP driver.
The selected-account probe also invokes the actual module's usage handler
with its captured Frame.Auth; separate synthetic credentials at G1/G2
detect stale or borrowed selected material without returning a digest or
token to the iframe. Informational action reads cannot acknowledge the
host provider-usage barrier.

usbox preparation passed Python/Node syntax, isolated Python helper
loading and three VM/privacy harness tests. Those checks alone are
**not** the native broker consumer gate. The runner also requires an
exact host SHA and frozen SDK source-file inventory. The accepted
native runs are recorded below. No auth bindings were added to
production manifests or the committed control-page distribution.

The initial host freeze `6f292fd` was revoked after independent review
found reauthorization replacing Usage/Refresh metadata. Its consumer
run1/run2 remain historical red diagnostic runs, not accepted evidence.
Final consumer gates must use Pauli's corrected freeze after the unchanged
review probes close.

Additional regression setup uses only public host mechanisms: a test-only
SDK usage.invalidate write occurs **before** ZCode reauthorization.
The wrapper defers actual provider-usage reads while the fixture examines
the barrier; an informational action still calls the real domain usage
handler without acknowledgment. Reauthorization must preserve the dirty
epoch and ordinary gateway admission must remain blocked until the
ordinary GUI quota-refresh route performs a real matching-revision read.
The wrapper never restores metadata or invalidates after reauthorization.

OpenCode's ordinary gateway execution renews an expiring device token
through the real host-owned refresh transaction before selected
reauthorization. The completed refresh receipt must survive unchanged
while the account advances again. OpenCode currently has an API-1.3 usage
handler without revision echo, so the fixture does not invent an API-1.4
quota acknowledgment for it; the barrier proof uses ZCode's actual
API-1.4 handler instead. These are two independent host-field regressions,
not permanent provider/module workarounds.

## Accepted frozen consumer proof

Leader review: **APPROVE**, no blocking findings. Reviewed files are the
six `.py`/`.mjs` files in `testdata/window-auth`. Their sorted, indented
JSON SHA-256 inventory, including the terminating newline, is
`d1290c9496929456b5a7f1b995260e97b9ca90a00718e502653edcb6711e1eb9`.
Those six files remained byte-identical through final verification.

Exact acceptance host:

- source tree `7f0f63b30c5859bce19cdca90f0e114ee979a212`;
- binary SHA-256
  `253f76efb7ffbdf9553a79b74252897f5b988175ca5b6222b81e504d415ab586`;
- source inventory SHA-256
  `a278427150cc3dd39829c5768d490d4144d043987fab8703b1cad41baedb6d29`;
- immutable source `usbox:/tmp/magpie-auth-main.1lBjaRys/source`.

ZCode source is production checkpoint `5b20682`; read-only OpenCode source
is `031d0931350c88e4875f11d89639110f5398dffc`. Combined module-source
inventory SHA-256 is
`d1b16f00a239cd45d873e37eb0258697278cbb99683d965e4e13ea8a9d5811fe`.

Independent fresh runs `positive3`, `positive4` and `positive5` completed
with exit 0 under `usbox:/tmp/magpie-auth-final-consumers-OfzeV5lp`.
Each used a new HOME/Store/intent and kept its host running through
reauthorization. All three `result.json` files have SHA-256
`b11a9eb529c1fbde362e31411b6de0c3dd27a9452e310f88510f743f28926e8f`.
Their public transcripts differ; the positive5 transcript SHA-256 is
`6f708aef11d8153d3a0a72634655c9c6124161297cb6faed3c0fd52d99fd014e`.
Each run checks 160 iframe messages and 71 parent HTTP calls.

The terminal matrix verifies:

- Actual PAGEBRIDGE and parent relay → ordinary HTTP → captured host
  Store → native ZCode/OpenCode vendor controllers; setup CLI is limited
  to add/config/off, with no CLI authentication or account-file writes.
- ZCode selected G1 → G2 preserves dirty usage revision 1/ack 0;
  ordinary gateway admission stays blocked until the real GUI usage read
  acknowledges revision 1.
- OpenCode's ordinary refresh reaches G2; selected reauthorization
  reaches G3 while retaining the completed refresh receipt byte-exactly.
  A canceled held exchange cannot publish G4.
- Duplicate or changed intents, foreign parent/root, undeclared binding,
  stale generation and disabled-root calls are refused without replaying
  accepted vendor exchanges. Inference/refresh counters remain exact.
- Credentials, Continuation, PKCE/private material and synthetic secret
  canaries never cross iframe messages or public HTTP/quota bodies.

Execution used UID/EUID 1000, all five capability masks zero,
loopback-only network namespaces, Go 1.26.8 and offline pinned caches.
The evidence is VM-realm/HTTP/native acceptance, **not rendered-browser,
visual, callback-mode or live-vendor acceptance**. No default API-1.4
manifest or production auth UI changed. Earlier red diagnostic runs and
the separately fixed public ExecuteOptions incompatibility remain
historical failures; final passes do not relabel them.
