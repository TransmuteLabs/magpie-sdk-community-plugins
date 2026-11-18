# Verification

First-account GUI adoption has a separate bounded local packet at
`/tmp/magpie-minimax-first-account-20261116`. Both the missing window declaration
test and the unchanged actual-host first-account probe fail on the frozen MCP
predecessor and pass with the adoption. Eight actual GUI HTTP/broker cases
cover zero accounts, both regions, early/pending/completed polls, single
exchange, foreign parent/region, cancel, stale config, discovery off before
start and during a held exchange, and package disable. Twelve ordinary MCP
cases still pass. Six focused Go tests pass at race20 (741 pass events);
format and scoped vet pass. The production browser controller also passes
synthetic DOM controls against the built distribution.

These local checks use isolated HOME/XDG/TMPDIR, pinned offline Go1.26.8 caches
and the parent's Mac network/config/Keychain/application guard under the
explicit local-work override. Actual GUI HTTP tests use the real host and
module handlers in the exec fixture, with synthetic TLS and accounts. A native
Darwin distribution builds and contains the page/helper assets, but this packet
does not prove a rendered browser, a new native load or live vendor sign-in.
The earlier native acceptance and SURFACE-1 packets remain separate. One
intermediate fixture used an unsupported settings CLI key; its setup failure
is preserved separately and the corrected probe uses `accounts discovery`.

The SURFACE-1 MCP schema correction was verified on November 16, 2026 on Mac,
under the user's explicit local-verification override, with isolated HOME/XDG/
TMPDIR, cached Go1.26.8, offline modules and the parent's descendant network
guard. `TestToolsPublishedHostSelectionSchema` fails on the frozen baseline
and passes with the common schema declaration. The unchanged
`tests/tools_front_host.py` obtains ordinary Front `tools/list`, validates
arguments before `tools/call`, and reproduces the baseline schema refusal.
With the correction, all 12 cases pass: selected A catalog/search each sends
once with B untouched; unbound/assertions-only, mismatch, restricted/read-only,
disabled and missing/logged-out selections send nothing; explicit B succeeds.
These send counts cover the connector/search requests made by the tool.

Seven focused named tests pass with race detection and count20. The scoped
tools/search/read-policy/refresh controls, package vet, no-output gofmt checks,
both workers and the synthetic host build also pass. Receipts and separate
red/candidate source manifests are retained in
`/tmp/magpie-minimax-surface1-mac-20261116`. The actual handlers run in the exec
test harness; this does not claim new native-loader or live-vendor acceptance.
On Mac the test worker loads its generated fixture CA into an in-memory
fallback root pool, forcing pure Go verification without Keychain access.
The initial Darwin CA setup failure is retained separately from the genuine
SURFACE-1 red. Production HTTP/proxy handling is unchanged. The earlier usbox
packet `/tmp/magpie-minimax-surface1-20261116` remains preserved.

Author verification completed on regular usbox with an empty temporary
HOME/XDG, pinned offline Go 1.26.8 caches and the parent's descendant network
guard. No test read actual credentials or contacted a live vendor.

| Check on the v4 source | Result |
| --- | --- |
| Remote `gofmt`, then `gofmt -l` across all package Go files | PASS; no remaining paths |
| `go vet ./...` | PASS |
| `go test -race -count=1 -timeout=10m ./...` | PASS; all 48 top-level tests plus subcases |
| Direct builder with source4 SDK and new output directory | PASS; native Linux/amd64 C ABI distribution |
| Literal donor matrix/schema fixture hashes | MATCH |
| Gauss's frozen 13 tool/media/search file hashes | MATCH |

The exact author artifact is
`usbox:/tmp/magpie-minimax-core.1j0UotIh/dist-v4`. Its native library SHA-256 is
`4ae1687285445c4570e1c08db7c6be229ad775969e57e3344f66cf0e2aedcc38`.
`verify-v4.log`, `verify-v4-results.json`, `verify-v4-guard.json` and
`format-v4-guard.json` preserve the commands and isolation evidence. The SDK
input was `/tmp/magpie-astra-combined-20261115-v4/source`; distribution
`build-info.json` records actual SDK file hashes.

Core regressions cover selected account/generation fences; both device grants,
PKCE, pending/reset/slow_down/deadline/consumption; rotating refresh without
replay; exact body/signature/session headers; immediate streaming and scoped
cancellation; explicit zero versus absent usage; incomplete SSE/error events;
region redirects and proved pre-dispatch fallback; live model metadata;
plan percentages/boosts, incomplete credit ledgers and expiry display; durable
daily check-in receipts; refusal classification; and pinned media destinations
with independent HTTPS-proxy TLS verification. Tool/media/search regressions
cover catalog/schema validation, operation bodies, selected SDK callbacks,
acceptance, cancellation, usage receipts and ambiguous results.

Actual negative cases are retained: `business-red.log` (HTTP-200 business
refusals incorrectly succeeded), `success-wire-red.log` (successful JSON bytes
were redacted), and `proxy-tls-red.log` (the media TLS name leaked into the proxy
handshake). Independent ordinary-host probing also found malformed/fractional
refusal classification; its original evidence is in
`/tmp/magpie-minimax-host-e2e.fhLVWFGt/classification-v2`. V4 adds canonical-code
and malformed-JSON regression controls. No negative probe modified shared SDK
source, Git refs, the index or original donor repositories.

Earlier setup failures remain recorded: the clean PATH initially omitted `ip`,
the first SDK test harness omitted configure, one fixture supplied a nonnumeric
usage revision, and an initial tar included macOS metadata sidecars in its
temporary overlay. These were corrected in the harness; their failed logs are
not claimed as product defects or successful checks.

Plato owns independent ordinary-host install/E2E and the unchanged malformed
classification replay against v4. The parent owns final workspace registry,
all-package build/check/install acceptance, and generic expiry/reactive-refusal
integration. These author checks do not approve those separate scopes.

No live account/vendor testing, non-Linux native-loader validation, performance
benchmark, release, publication, commit or live installation was performed.
Unexpected-401 recovery remains a host-contract integration boundary. Static
tool descriptors, supported schema constraints and media byte limits are
described in `CONTRACT.md` and the tools lane's retained terminal packet.
