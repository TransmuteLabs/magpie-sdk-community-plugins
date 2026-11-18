# Native SQLite snapshot verification

2026-11-14. All builds and tests ran on **usbox**, offline with
`GOFLAGS=-mod=readonly`, pinned existing Go caches, isolated HOME/XDG,
synthetic accounts and synthetic vendor endpoints. No installed account,
application, database or vendor was used. Python generated fixture DBs
and drove assertions; the installed reader is native Go.

## Immutable identities

| Item | Identity |
| --- | --- |
| Module executable-source inventory | 100 files; SHA-256 `43c2641dbb8cb8384809970ca0e259e40b9e6202e7fa446f55108d129ef85e8f` |
| Initial SDK host | `f4232ea762bf881759eba37ac1c9ea7581b627d9` |
| Initial host binary | SHA-256 `2c15325e88d437a293b4f603dc6eac720e9436d3ba669ad5199f08574f9c6cfa` |
| Compatibility SDK host | `831c3d74af9358fa8a45d77eed902fe7cd02a076` |
| Compatibility source archive | SHA-256 `3f51cd2303114918705a8e2a2e3bd9f5739a2085dc8f2a0eeaa893d613d18b8e` |
| Compatibility host binary | SHA-256 `45d10c98019d7a258c27d80d333ef0ae6ca7c538c76c9dd286b82da9590455b1` |
| Headless 0.6.1 Linux amd64 ZIP | SHA-256 `09dcbb9e8501697355437c3ce51e1ae0ccd7c0045e016e5c1d88b4f039bc856f` |
| UI 0.8.1 Linux amd64 ZIP | SHA-256 `7e37b225fd6d669035a1613f0b247e01b5bebe2757d8441e355a28b695ea8366` |
| Verification rollback fixture | SHA-256 `2700c35785974f55f847830e7f86aeafa33b25415d72d8f767143e3d78392d5b` |

Evidence root: `usbox:/tmp/magpie-zcode-sqlite-jF2jd6Ay`.
`source-sqlite-release-candidate.json` hashes executable/test/package
source; this narrative and README updates follow that freeze and do not
change its code. ZIP hashes identify the exact tested archives, not a
promise that rebuilding ZIP timestamps reproduces them.

## Terminal results

- Full module `go test -race -count=20 ./...`: **4360 pass events,
  zero failures, zero skips**. Formatting and vet passed.
- Native headless/UI build, native ABI conformance and strict TypeScript
  passed. Node and Bun UI tests each passed **28** cases.
- Darwin arm64 and Windows amd64 CGO-disabled vet/build: all four gates
  passed. These are compilation checks, not native ABI runtime proof.
- Actual native controller, auth plus binary SQLite preview/apply/backup/
  exact restoration, selected-account control and one-bundle auth passed
  against the initial host. Final compatibility controller/auth/one-bundle
  UI/rollback runs against host 831 also exited **0**.
- One-bundle UI: the real packaged IIFE and served SDK bridge crossed
  isolated parent/iframe VM realms into HTTP, native controller and Store.
  G1→G2 succeeded; cancel/dispose during a held third exchange produced
  no G3 or late reload. There were 34 crossings and 16 HTTP requests.
  Only CLI add was setup; exactly one native catalog entry and no legacy
  specifications, companion auth store or Bun/Node worker were present.
  This is functional VM coverage, not rendered-browser acceptance.

## SQLite and resource guards

Fixtures exercise CLI session/message/part, desktop task index and
archived controller schemas, authoritative SQL row IDs, forks/orphans,
large integer tokens, role/tool/media/body references, opaque JSON/BLOB
data, copy isolation and binary round trips. Financial inspection
preserves pending/debt and old generations but conversion/restoration is
denied, including correctly hashed forged bundles.

A valid generated column exposed allocation before the old Go encoded
row-size check. The unchanged small deterministic assertion first failed
against that implementation. The reader now pins one connection, installs
SQLite engine limits before its first query, refuses generated columns
before integrity checking, owns a ten-second ceiling and bounds aggregate
projection. Ordinary unknown stored columns remain lossless; CHECK
constraints are not disabled.

Ten negative mutations independently compiled and failed assertions:
WAL refusal, fork counting, debt preservation, financial restore refusal,
bundle digest, generated-column refusal, engine length bound, owned
deadline, pending-state preservation and aggregate projection limit.
`negative-results.json` records each exit 1 as an assertion failure,
not a build failure.

## Actual host rollback

`testdata/sqlite-rollback/run.py` copies an immutable SDK tree and injects
only its documented verification `_test.go`. The fixture installs the
actual native module and obtains a real SessionConvert/Actions plan.
Cancellation is injected after a real destination write.

Twenty fresh race-enabled processes produced **80 pass events**, no
failures or skips. Cancellation restored the exact binary preimage and
reported `rolled_back`; a foreign pre-execution change prevented writes;
a foreign mutation during rollback survived with `prepared` and
`ErrActionChanged`. Source bytes and backup bytes remained exact, and
retrying the consumed plan was refused.

All **10340 SDK production files** were unchanged. Their inventory hash
was `c4fa3793826fd3349d29aa29b18caaad8e9f8561c69ea6a1d9e219eeb5e58b52`.
An earlier harness failure incorrectly inherited an external BUN_INSTALL
under MAGPIE_TEST_SANDBOX. The corrected driver lets SDK TestMain isolate
the environment; this was a harness correction, not a product fix.

## Limits

Only explicitly supplied coherent checkpointed DB bytes are accepted.
This is not automatic live DB/WAL capture, live resume or financial
controller import. Cross-platform native library runtime, live vendor
behavior, actual ZCode CLI startup and a rendered GUI were not tested.
The owned-agent work and bounded vendor/community ports remain tracked
in ONE-BUNDLE-AUDIT.md, VENDOR-DIFF-3.18.2.md and COMMUNITY-PARITY.md.
