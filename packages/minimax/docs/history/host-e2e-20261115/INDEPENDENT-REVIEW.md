# MiniMax v4 independent production review

ACCEPT for the frozen MiniMax production package within the coverage below.
MM-E2E-1 is independently closed; no other confirmed P1/P2 remains in this
package review. MM-HOST-OBS-1 remains a separate confirmed host issue owned
outside this lane. This verdict does not approve the shared engine still under
development, reactive 401 recovery, live vendor compatibility, or other native
platforms.

The exact author source digest is
18362762aa982cf1e0a186e03b9338d5bd9ce7f91275cfb40d8ba8615860b5fd.
All 35 frozen paths match the local package and the copied frozen usbox source.
The unchanged production library is
4ae1687285445c4570e1c08db7c6be229ad775969e57e3344f66cf0e2aedcc38.
The ordinary host binary is
dfc5c0ddcaae39339ef5d2008056d0b63ad1b7821c6f87bf6af472913e2f1e79.
The final runner is
06df453e19e7b3b66c0a48973e281106837b9b6d934bda73a8ab28c3d8ee33a7.

## Source coverage

REVIEW-COVERAGE.json lists each exact path, hash, line count and read category.
The 21 production/build/metadata paths were fully read: 17 Go implementation
and entrypoint files, tools_matrix.json, go.mod, go.sum and scripts/build.py.
Six current documentation/provenance/license paths were read. Eight test paths
(seven Go test files and one JSON fixture) are inventory-only in this review;
their existence and the author's green suite do not imply independent source
review. The separately owned E2E runner was implemented and executed by this
reviewer. Reads accumulated during traversal; final corrections were read and
all bytes reconciled against the final freeze. No entire-host or entire-donor
source review is claimed.

Reviewed ordinary paths include selected account/generation ownership;
device-flow state CAS, pending/terminal consumption and refresh ownership;
regional endpoints, bounded HTTP, redirect/fallback authority and cleanup;
live model projection; usage and check-in fences; accepted chat/SSE execution,
sparse facts and partial termination; tool schema/catalog selection, acceptance
and own-call receipts; search; media parsing, upload, asset lookup and bounded
credential-free transfers; default native registration and packaging.

The module is self-contained at runtime: native entry registers the default
constructor, and the executable E2E loaded exactly the installed production
library in the ordinary host. There is no test-native wrapper, endpoint option
override, sibling plugin install, Bun or Node dependency. Literal matrix/schema
hashes match source-lock.json. This review does not independently rederive every
donor-file hash or claim all declared tools were executed.

## Independent execution

Regular usbox only, fresh HOME/XDG, cleared environment, existing host binary,
private CA through SSL_CERT_FILE, host-owned proxy setting and public SDK
network.proxy callback. CONNECT is terminated locally without forwarding. The
syscall guard denies all nonlocal attempts, including the host's blocked DNS
attempts to 8.8.8.8/1.1.1.1 and its own external-denial self-check. No vendor
connection succeeded. Each fixture/host process stops and joins. Synthetic
account tokens are fixture data, never real credentials or live configuration.

production-v4-final exits 0 with 20 scenario records: CN/global PKCE auth,
pending/foreign-continuation/replay fences, selected models/search/tool,
accepted tool HTTP-200 business failure, buffered Chat, text/tool SSE, image
URL, video create/content bytes, distinct permission/quota/business refusals,
partial-output EOF, cancellation, four direct SDK classification controls and
exactly-once accounting with observed token totals. Gateway failures retain
the existing client 409/no-replay fence; direct SDK controls independently
observe module statuses 403/429/502/429. The host observability finding records
the separate loss of those facts in normal trace/ledger views.

classification-v4-final exits 0 with two auth setup records and four unchanged
malformed-code assertions. The v2 production artifact failed all four. V4
returns unknown failures instead of manufactured auth/quota classifications.
Fixtures express synthetic protocol semantics, not captured live responses.

Author format, vet, race and build receipts are separate supporting evidence;
this reviewer did not rerun broad Go suites. Non-Linux loaders, browser/UI,
all live tool schemas, image upload permutations, large-media limits under
load, performance, and vendor accounts were not exercised by this E2E.

## Preserved failures and artifacts

Earlier failures remain under the remote prefix and in the evidence packet:
v1 runner used local provider IDs instead of host namespaced IDs; v2 assumed
vendor status reached the client instead of the host's 409 fence; a later run
queried the async model catalog before readiness. Those were test assumptions.
The fixture also originally treated an unauthenticated GET / addressed to its
own loopback listener as an external destination. It now records that exact
local request and answers 404; CONNECT allowlisting and rejection of forwarded
non-fixture destinations remain intact. Final runs recorded no such probe.

MM-E2E-1 was an actual product defect and has its own failing/closure report.
MM-HOST-OBS-1 has its own source and actual trace/ledger evidence. No source
fix was made by this reviewer. Only tests/host_e2e.py and this history packet
were written. No stage/index/ref/commit/merge/push operation was used.

Reproduction (inside the isolated guarded usbox runner environment):

```sh
python3 tests/host_e2e.py --host HOST_BINARY --distribution DIST_V4 --output FRESH_OUTPUT
python3 tests/host_e2e.py --host HOST_BINARY --distribution DIST_V4 --output FRESH_OUTPUT --classification-only
```

Remote prefix: /tmp/magpie-minimax-host-e2e.fhLVWFGt.
host-e2e-evidence-v4.tar.gz contains original result/command/fixture/trace/
ledger/log/guard receipts and the external runner recipe, without certificate
private keys or source snapshots. SHA-256:
9ddc59bb7b85f3f7cba3d3b1f1cddb7dcff3e8b59dfd5d574fc9cb4a0f60fec5.
