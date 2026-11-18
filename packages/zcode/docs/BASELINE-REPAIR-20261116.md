# Package baseline repair on parent host05

The fifteen named failures in the earlier foreground handoff are resolved.
Verification uses parent's immutable local candidate-05 through a temporary
SDK replace: source identity
`0db2f2e148b7263e196db2e1e4f40e2fd3a4bfba31fddf842a9417e6b57f4926`,
11001 files, base lineage `705d67efc78f72cdc63973318ecaaaa661c1e04e`.
This is an uncommitted source freeze, not a released host version.

## Causes and changes

Fourteen tests shared an incorrectly localized HTTP-date fixture.
`time.Unix(...)` uses the local location; `http.TimeFormat` prints a literal
GMT suffix. At UTC+3, Date and date-valued Retry-After were three hours ahead.
This expired gift/Start plans, removed a pending usage window, bypassed the
intended gift fixture into paid/marketing paths and shifted durable check-in
holds. The +90-second check-in expected1794564090000 but received1794574890000.
All fourteen tests passed unchanged in the UTC causal control.

The shared fixture instant is now explicitly UTC. Production server-Date
authority, Retry-After parsing, gift refusal rules, the1024-window limit and
financial debt barriers retain their existing behavior and assertions.

`TestConcurrentControllersDoNotLoseClaims` failed separately. Two controllers
creating the same absent durable lock with O_CREATE|O_RDWR received ENOENT
from openat on the verification Mac filesystem, before reaching flock.
A diagnostic overlay localized the error; nineteen of twenty repetitions
failed. Exclusive creation followed by opening the existing file on ErrExist
passed twenty repetitions. The production change uses that atomic creation
sequence. It retains the lock inode, permission checks, context cancellation,
cross-process flock and durable journal save path.

No shared SDK change or budget increase was needed. Queue deadlines,
Off-Peak tickets, financial journals and uncertainty fences keep their owners.
The separately approved foreground adoption is not expanded by this repair.

## Terminal package evidence

Receipts, full JSON output, exact failure names, source/artifact inventories
and the review patch are in:

`/Users/maratkarimov/work/.magpie-verification-20261116/packets/sol-zcode-baseline-repair-01`

| Gate | Result |
| --- | --- |
| Exact15 old source, Europe/Volgograd, race count20 |750 fail/130 pass events; all15 named failures reproduced; race clean |
| Unchanged source UTC causal control |43 pass/1 fail; only concurrency remains |
| Exact15 repaired, Europe/Volgograd, race count20 |880 pass, zero fail, race clean |
| Exact15 repaired, UTC, race count20 |880 pass, zero fail, race clean |
| Full package `go test -json -race -count=1 ./...`, UTC+3 |516 pass, zero fail, race clean |
| Full package `go vet ./...` |exit0 |
| Changed Go files `gofmt -l` |exit0, empty output |

All gates use pinned Go1.26.8, private HOME/XDG/TMPDIR, shared pinned caches,
offline module resolution and the supplied Mac network/live-path guard.
The final packet additionally records distribution build and provenance
results. Existing regression assertions were sufficient; none was skipped,
loosened or replaced by an implementation-only assertion.

Versions remain full package0.15.0, headless0.7.3, SDK minimum1.12 and native
ABI1. The unchanged donor no-op comparison remains checkedAt2026-11-16.
`sdk-baseline-repair-20261116` adds exact host05 provenance while retaining
the earlier policy/native source records as historical verification evidence.

Final combined host installed acceptance belongs to parent. Native adoption
still requires its separate review. This repair claims local Darwin synthetic
package evidence; it does not claim live vendor/account or desktop attachment
validation, or a new Linux/Windows runtime replay.
