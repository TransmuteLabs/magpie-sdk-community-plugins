# SDK 1.4 module adoption verification

2026-11-13. This is module-scoped evidence, not a combined SDK release.
All compilation/tests ran on **usbox**, Linux amd64, Go 1.26.8, isolated
HOME/XDG with pinned caches and offline module resolution. No live vendor,
installed app, account, credential, push or publication was used.

The committed 1.3 authentication checkpoint is
`3d5251713d4958ae349cd1696b488bccc11fc24f`.
The leader authorized the local 1.4 module checkpoint after independent
host auth/usage/executor review closed without findings and routing/hybrid
review was approved. The dependency remains **unreleased SDK 1.4**; this
authorizes neither installed activation nor remote publication.

## Source and artifact identities

Remote evidence root: `/tmp/magpie-zcode-sdk14-l07v15jx`.
All **52** Go/Python/module source files hash-match the local working tree.
Their compact key-sorted JSON `{relativePath: SHA256}` manifest has SHA-256:

`aecee4beec45de2141d7cd7da7122aeccbb90465b0279ad61d90b31aadb1dff3`

Native Linux ZIP SHA-256:

`ec7b50c74ec34b049d5e7a8b23c4c4dca21edf992560aad059c9dad626dbf664`

The public SDK packages were copied read-only from the leader's 1.4 source
snapshot to this lane's own remote sibling SDK directory.
The compact key-sorted SHA-256 manifest of those 60 snapshot files is:

`98eac7779cb9e2c024025c0b1a6d0ccf58d287381f307097e1544efad2347c23`

The initial isolated host preview was copied from
`/tmp/magpie-refresh-proof-zt0yerwy/magpie-sdk14-preview`, SHA-256:

`7cd2666a9429926f2c139e91e48efdb8af5df2674fbe468decaa89283ad7f0d3`

## Completed gates

- Strict formatter gate: `gofmt -l .` exit 0 **and empty output**.
- `go test -json -race -count=20 -timeout=4m ./...`: exit 0,
  **2680 passing events, 134 distinct tests/subtests**, no failures.
- `go vet ./...`: exit 0.
- Linux native C-shared ZIP/checksum builder and two concurrent native ABI
  sessions: exit 0; five calls/session, same process, no Bun/network callbacks.
- Darwin arm64 and Windows amd64 CGO-disabled vet/build: all exit 0.
  No native runtime execution on those platforms is claimed.
- Four compiling behavioral negative overlays each fail their intended
  regression: bypass invalidation; bypass unsettled-reset deferral; remove
  alias applicability; return cancellation intent as the original target.
- The principal journal is held across the whole vendor quota observation:
  a second controller's bounded lock attempt cannot enter until the
  observation completes. Pending reset and shared-user claim state defer
  reads across module instances. Failed invalidation prevents vendor POST.
- Start quotas aggregate by exact model across buckets; duplicate
  capabilities do not duplicate capacity. Missing total capacity remains
  unknown and exhausted balances remain known at 100%.

`e2e.py` passed against the preview host. This uses the actual native
library and separate CLI processes, with only vendor destinations overlaid:

1. Business/PAT/expiry/generation publication remains atomic.
2. Native ticket admission survives separate CLI processes.
3. Two business-cancel intents return the original target and send one
   settlement POST; no inference is replayed.
4. An accepted reset invalidates usage before the synthetic lost response.
5. A separate CLI inference is blocked by the durable dirty revision.
6. A deferred module usage read echoes revision 1 and does not clear it.
7. Explicit uncertainty resolution followed by a fresh matching read
   reopens inference. Ordinary quota excludes all four native aliases.
8. Uncertain credential refresh remains fenced.

Logs and command exit codes are in `gates-results.json`, `*-final.log`,
`negative-*.log`, and `preview-e2e.log`.

## Closed host-policy regression

The same preview failed `auth_e2e.py` at `auth.start`: API 1.4's generic
read-only `state.cas` restriction also denied AccountAuth continuation
persistence. The earlier actual 1.3 native flow passed at the checkpoint
above. The host owner fixed scoped generic auth lifecycle policy. The module
did not bypass that policy. The corrected immutable preview was copied
read-only from:

`/tmp/magpie-refresh-proof-zt0yerwy/magpie-sdk14-auth-preview`

Its SHA-256 is:

`bf11f37b125b4a6b59ba3c8501b323cc98b9556bad55ed1af3b8fa477092efca`

Both `auth_e2e.py` and `e2e.py` then passed, exit 0, against this same
corrected host. Logs: `corrected-host-auth.log`,
`corrected-host-controller.log`, `corrected-host-results.json`.
The native auth proof includes private continuation persistence, bounded
polling across CLI processes, no repeated exchange, same-account Start
reauthorization with generation 1 → 2, dynamic entitlement models and
inference using the replacement session. There is no remaining scoped host
blocker for this continuation. Independent scoped host review subsequently
closed without findings. Combined SDK release/installed activation remains
separate and is not established by these module fixtures.

No automatic Start JWT refresh endpoint is invented. Highspeed cancellation
without a recovered authenticated vendor exchange stays unsupported.
Live vendor parity, UI/MCP/session readers, reviewed SQLite import and
whole-host acceptance remain outside these module-scoped gates.
