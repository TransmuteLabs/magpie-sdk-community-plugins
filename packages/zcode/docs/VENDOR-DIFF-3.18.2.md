# Bounded actual-baseline → ZCode 3.18.2 diff

Read-only workspace source `Agents/zcode` resolves to the same
repository as `Agents/ZCode`, origin `zai-org/ZCode`, clean
HEAD `fec975905a2abd4c07e35c40d51fa097a2258ec9`.
Previous actual baseline is `b95ebaffeab641907b38652f45c1502bd762972d`
(desktop 3.18.0); the SQLite checkpoint's ClientVersion and EXTRACTION.md agree.
CLI package version remains 0.17.1. Intermediate desktop 3.18.1 is
`c5904686ea8341826b3affe5d993d06a134fa779`.

The wider 3.17.2 → current diff contains unrelated older changes and
cannot be attributed to this continuation.

## Domain deltas requiring work

- `accountProviderConnectionResolver.ts`, account request auth and
  coding-plan availability now separate Start material/identity from
  linked Personal/Team credentials. A China login with no linked
  BigModel paid account still publishes its Start provider. Current
  module explicit `kind: start` was independent already. The prior
  `kind: auto` required `ready[site].access_token` and rejected valid
  unlinked Start. The continuation adds confirmed Start fallback before
  any paid key/PAT exchange. Unknown entitlement still fails closed;
  selected reauthorization retains exact account/generation and cannot
  change plans silently. The explicit `loginProvider: zai_cn` channel
  maps only to BigModel, is preserved for renewal and never authorizes
  the other family.
- OAuth credential repository/service adds Start-specific material and
  account-completion race checks. The module's existing captured
  user/target generation and final AccountID check must remain covered.
- System credential security changes active data-root/OS-store fallback,
  including Linux failure behavior. These are not permission to read
  the live app store. The SDK account owner remains the sole credential
  publisher; no vendor keyring/cipher implementation belongs in core.
- Native protocol server retention and gateway-disposal details change
  event lifecycle/resource diagnostics. They are relevant to a future
  native agent/control reader, not an ordinary Messages endpoint change.

## Unchanged bounded surfaces

Between b95ebaff and fec9759 there are no source changes in the inspected
provider-node rule/codec/download paths, CLI config adapters, CLI SQLite
session-store implementation/migrations, official MCP credential channel,
OffPeak runtime/server client or reset/quota endpoint contracts. This is
an exact bounded-path observation, not a claim that all vendor behavior
or every app component is unchanged.

The current automatic-source/agent work must use the already recovered
CLI config formats and distinguish DB/WAL snapshots from JSONL. Do not
invent a new endpoint because only the desktop version changed.

## Port sequence

1. Preserve the separately frozen SQLite/resource/rollback evidence.
2. Add the China-unlinked auto-Start regression through actual SDK auth
   handlers; prove no paid endpoint or duplicate exchange is reached.
3. Port only that domain branch, then repeat native auth/models and the
   one-bundle clean-HOME proof. Advance ClientVersion only with the
   semantic port and explicit version/header assertions.
4. Carry the new lifecycle requirements into the owned-agent/profile
   cut, with actual artifact identification and no extra installed plugin.
5. Continue the community gift/check-in/concurrency comparison separately;
   it is a distinct source lineage, not the 3.18.2 vendor diff.

Source-lock watch entries and committed regression evidence must be
updated as ports complete. The auto-Start and login-channel regressions
exercise actual SDK handlers; the initial old-code run compiled and
failed assertions. This record does not claim full native-agent
lifecycle parity.
