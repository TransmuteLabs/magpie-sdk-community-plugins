# Headless authorization and catalog verification

2026-11-13. This records the `0.2.0` authentication/discovery continuation
after the independent first-slice commit `c8acfa2`. All builds/tests ran on
**usbox**, Linux amd64, Go **1.26.8**, isolated HOME/XDG, pinned caches,
`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`. No live vendors, installed
apps, accounts or secrets were used.

## Exact executable-source checkpoint

All 50 Go/Python/module files hash-match the tested remote checkpoint.
The SHA-256 of their compact, key-sorted JSON `{relativePath: fileSHA256}`
mapping is:

`57bd2052c05458e9c863735b2d4499de5b00564a2f297f195dc5c132018b1baf`

Documentation/evidence additions do not change that mapping.
Remote evidence root: `/tmp/magpie-zcode-native.5S8zm9QB`.
The public SDK copy there remains the API 1.3 verification snapshot;
this is not a claim that the concurrently developing SDK 1.4 passed.

## Completed gates

- `go test -json -race -count=20 -timeout=4m ./...`: exit 0,
  **2560 passing test/subtest events, 128 distinct tests/subtests**,
  zero failures and zero skipped tests. Log:
  `auth-final-race20-recheck.log`.
- `go vet ./...`: exit 0. Correction during the 1.4 continuation:
  `gofmt -l .` had exited 0 while listing two test files with cosmetic map
  alignment differences. Exit status alone was insufficient evidence of
  formatting cleanliness. Those alignments are corrected in the
  continuation, whose gate also requires empty formatter output.
- Native C-shared Linux ZIP/checksum builder: exit 0.
  `magpie-plugin_linux_amd64.zip` SHA-256:
  `4ac6ff3096c7dcf36b184381fe8393376f9c8570c574aaa1bafca012446c269c`.
- Native C ABI verification: two concurrent sessions in the same process,
  five calls each, no Bun/network callbacks; exit 0.
- Darwin arm64 and Windows amd64 `CGO_ENABLED=0` vet/build: all exit 0.
  These are compilation checks, not native runtime execution evidence.
- `auth_e2e.py`: actual host CLI + actual native library, fixture vendor
  destinations overlaid only at registration. Two init/poll/finish flows
  preserve private continuations and polling interval across CLI processes;
  completed exchanges are not repeated. Start reauthorization retains the
  account ID and publishes generation 1 → 2. Dynamic model discovery filters
  the configured catalog by the selected live balance; inference uses the
  renewed session. Exit 0, `auth-final-auth-e2e.log`.
- Existing `e2e.py` remains green: business/PAT/expiry/generation publication,
  candidate readiness purity, cross-process ticket reuse, settlement once,
  inference with replacement credentials and uncertain refresh fencing.
  Exit 0, `auth-final-host-e2e.log`.

Both E2Es used the same immutable host executable:
`/tmp/magpie-refresh-proof-cy1e4lyy/magpie-refresh`, SHA-256:

`6784b7e6d0a2b30f7e8a81922c03aac8436b5c113322d10a9b2b7ea816916a8a`

Three compiling behavioral overlays failed the intended regression:

1. Remove generation admission: selected-account renewal test fails.
2. Reopen an exchanging flow: lost-receipt no-replay test fails.
3. Bypass discovered eligibility: predispatch model admission test fails.

These negative tests use loopback fixtures even when a guard is removed.
Logs are `auth-final-negative-{generation,completion-fence,model-eligibility2}.log`.

## Intermittent observation and limits

An intermediate race20 run recorded an unexpected request in the older
ticket-reconciliation fixture. Its original diagnostic omitted the request
method/path, so the cause could not be established from that run.
The fixture now records that bounded diagnostic. An isolated race run of
the affected test repeated 200 times passed; the final all-package race20
above also passed. No production change was made on the basis of an
unreproduced observation.

Synthetic fixtures prove the contract/codec/controller paths exercised
above; they do not prove live vendor OAuth or entitlement parity.
There is no recovered automatic Start JWT refresh exchange: renewal uses
explicit same-account reauthorization. Ambiguous Team selection requires a
new explicit flow, and uncertain paid exchanges are never replayed.
Dynamic discovery currently has no cache. UI/MCP/native session readers
and reviewed archived SQLite import remain separate work.
SDK 1.4 quota applicability/invalidation/durable-cancel adoption stays
disabled until the generic host contract is independently verified.
