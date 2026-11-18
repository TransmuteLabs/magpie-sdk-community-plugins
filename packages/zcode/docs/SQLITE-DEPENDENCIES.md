# Native SQLite dependency evidence

2026-11-14. Runtime uses Go `database/sql` with the **existing host-locked**
`modernc.org/sqlite v1.61.0` and `modernc.org/libc v1.79.0`. The module
declares those existing versions directly and records runtime checksums.
Python is used only by synthetic fixture generation and verification,
not by the native reader. No new library version was selected or downloaded.

The earlier comparison of the lightweight consumer's `go list -m all`
(119 nodes) with direct-driver metadata (132 nodes) was incomplete.
The full host module graph has **516 nodes**, including all 13 below in
these exact versions. Each also has **both module and zip checksums**
in host go.sum. None is directly listed in host go.mod; they are existing
transitive metadata. On the inspected usbox cache their `.mod` and source
directories were absent. Offline dependency-test `go mod tidy` therefore
failed; it did not establish a missing runtime dependency.

| Existing host graph module | Exact version | Runtime linked by this module |
| --- | --- | --- |
| github.com/google/pprof | v0.0.0-20261016150608-1facf8f91b50 | No |
| github.com/hashicorp/golang-lru/v2 | v2.0.7 | No |
| golang.org/x/tools | v0.52.0 | No |
| modernc.org/cc/v4 | v4.30.3 | No |
| modernc.org/ccgo/v4 | v4.37.0 | No |
| modernc.org/fileutil | v1.4.0 | No |
| modernc.org/gc/v2 | v2.6.5 | No |
| modernc.org/gc/v3 | v3.2.0 | No |
| modernc.org/goabi0 | v0.2.0 | No |
| modernc.org/opt | v0.2.0 | No |
| modernc.org/sortutil | v1.2.1 | No |
| modernc.org/strutil | v1.2.1 | No |
| modernc.org/token | v1.1.0 | No |

Actual runtime `go list -mod=readonly -deps -json ./...` passed:
**219 packages / 10 modules** in the native prototype.
Host `go list -mod=readonly -deps -json -tags nogui .` passed:
**776 packages / 110 modules**. Every third-party module used by the
plugin was already in the host's runtime set. The two nonshared labels
were the plugin's own module and its local SDK replacement.

Evidence root: `usbox:/tmp/magpie-zcode-sqlite-jF2jd6Ay`:
`graph-before.txt`, `plugin-runtime.json`, `plugin-runtime-modules.json`,
`host-runtime.json`, `host-runtime-modules.json`,
`host-module-graph.json`. `tidy.log` preserves the failed offline
dependency-test metadata experiment. Production/race/native verification
uses coherent go.mod/go.sum with `GOFLAGS=-mod=readonly`,
`GOPROXY=off`, `GOSUMDB=off`; no dependency compiler tests or downloads
are required for the delivered native module.
