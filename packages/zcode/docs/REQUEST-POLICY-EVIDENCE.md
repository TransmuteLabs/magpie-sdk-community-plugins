# ZCode public API 1.12 adoption

SOL implementation; Astra independent review remains separate. Package-only
scope, checked 2026-11-16. Shared host docs and common STATUS belong to parent.
No staging, commit, branch, merge, publication, donor checkout changes, new
dependencies, live accounts or installed application changes.

## Versions and source identities

| Identity | Version / exact source |
| --- | --- |
| Full package | 0.15.0, unchanged version; unpublished candidate identified by hashes |
| Provider-only composition | 0.7.3, unchanged version, explicit `--headless` |
| Both runtime SDK floors | API 1.12, `request-policy`, compatibility `^1.12` |
| Native ABI | 1 |
| Wire client / vendor desktop | 3.18.2 |
| Vendor CLI | 0.17.1 |
| Vendor implementation baseline | `fec975905a2abd4c07e35c40d51fa097a2258ec9` |
| Ported community baseline | ZCode 0.1.66, `87a8cf03c62fa55ca845c51648e8859ece580336` |
| Historical community audit | `57d1cc6a9f49389d495ea9be8d986c70de7767bf`, package tree `d1be84b45f19c7d51bf3bf1c428ddc7ae95dfbf7` |
| Supplied policy host physical manifest | SHA256 `e2ca42c6fa8529b781a61aec04862355a5b28183646cc57ee028f6133fb74c34`, 10987 files |
| Host capture base | `705d67efc78f72cdc63973318ecaaaa661c1e04e`; distinct from physical source |
| Compiled verification host | SHA256 `ec1a75db43c57acfa33ae8a3783e2a8f8e202f227f08050ed5a689b4bb1961a4` |

The SDK source is the supplied read-only
`usbox:/tmp/magpie-obs-policy-fix-20261115/candidate-01`.
No final combined-host or committed-host acceptance is implied.

## Behavior and preserved owners

Ordinary paid and Start Messages use public `RequestWithPolicy`, which calls
`DecideRequest`. Their module-owned one/two-second transport timers and repeat
caps are removed. A complete, recognized, output-free refusal envelope must
independently prove no effects before returning fresh `RefusedExecution`.
Callbacks alone never become final routing authority. Actual HTTP status,
machine code/type and bounded message precede codec normalization.

Malformed, conflicting, output-bearing, truncated and unknown responses have
no replay evidence. Duplicate/unrepresentable timing hints cannot disappear
into a shorter local wait. Documented reset-word timestamps use Beijing time;
final windows name the exact selected model. Unknown scope 3009 remains
terminal at the real host; request-security code 3007 cannot enumerate accounts.

Start FIFO admission, adaptive concurrency, route holds and shared queue
deadlines remain queue owners. Off-Peak ticket polling, ticket/card admission,
claim/reset journals, settlement and uncertain financial results retain their
existing owners. The free-first gift-to-paid domain workflow keeps acceptance
before dispatch and cannot gain whole-execution failover by moving that fence.
Auth refresh/reauthorization, MCP, config, Library/agent, session conversion,
SQLite and native ABI capabilities remain present.

## Meaningful reds

The unchanged runtime plus new wire-harness regressions sent three busy
requests despite host denial on both ordinary paid and Start paths, and
advertised API4/API9 rather than API12. `old-red-02` records the failures.

The actual built baseline full distribution, installed in a private home on
the supplied compiled host, sent A/A/A and returned terminal 409 for a
two-hour refusal. The installed regression requires A/B and success without
waiting two hours. `installed-old-red` retains that failing receipt.

`timing-old-red` independently exposed a malformed `request_id` acquiring
evidence and lost timing bounds acquiring shorter retry authority. Both now
pass alongside neighboring ambiguity, output and unknown-reset controls.

## Terminal package gates

All commands ran on regular usbox, Linux/amd64, private HOME/XDG, existing
offline Go caches, pinned Go 1.26.8 and the existing descendant network guard.

| Gate | Result |
| --- | --- |
| Full Go suite, `tests-06` | 534 passing test/subtest events, zero failures/skips |
| Bounded concurrency/policy/financial race slice, `race-06` | 61 passing test/subtest events, zero failures/skips |
| `go vet ./...`, `vet-06` | PASS |
| Formatting / Python syntax, `static-06` | 107 Go files formatted, 34 Python files parsed; Go version checked |
| Browser / TypeScript, `browser-clean-06` | Node and Bun, 34 tests each; PASS |
| Existing cleanup / ingress / native peer fixtures | 5 / 4 / 7 tests; PASS |
| Default distribution inventory, ABI, capabilities, provenance | `package-06`; PASS |
| Full and provider-only native conformance | two native sessions each; PASS |
| Actual installed host policy scenarios | all 15 cases PASS |

The installed cases prove:

- Long reset: A/B, HTTP200, under one second in the final fixture.
- All-long: A/B, finite HTTP429 retaining the two-hour Retry-After.
- Short wait: A/A, at least 950ms between sends for a one-second hint.
- Shared budget: a callback repeat consumes the one additional dispatch;
  no subsequent sibling send occurs. Zero additional budget sends only A.
- Fresh final facts: A's short concurrency refusal, then HTTP200 quota/reset
  refusal, then B; no prior callback diagnostic substitutes for the final facts.
- Trial cache and applicability: A/B, then cached Trial selects B; independent
  paid and other-model requests select A. Both accounts' Trial catalogs are
  first read through public provider pool/refresh commands.
- Accepted workflow, incomplete response, published output, unknown refusal,
  transport uncertainty, cancellation, account logout and config CAS change:
  one inference send. Existing journal negatives separately prohibit repeated
  financial POSTs. Streaming output retains HTTP200
  once published; terminal uncertainty otherwise retains HTTP409.

Earlier browser and ingress attempts failed on archive-created AppleDouble
test files and missing `/usr/sbin` in the isolated PATH. A fresh clean snapshot
and the existing `ss` tool resolved those fixture issues without deleting old
files or changing browser/ingress production code. The exploratory repeated
Trial case initially lacked B's dynamic Trial catalog; the final fixture
establishes both catalogs using public host commands.

## Candidate-06 artifacts and receipts

Root: `usbox:/tmp/magpie-zcode-sol-policy12-20261116/`.

| Artifact | SHA256 |
| --- | --- |
| `candidate-06-clean.tar.gz` | `bd41b852f52f242291fb4a69ad6be7f55108f1bdf4309e82cffacb0da14f6b16` |
| `dist-06/lib/zcode.so` | `afb97110a628ba1c13fe870daeee56340766df6cad578229e6050f4cef2a42ca` |
| `archives-06/magpie-plugin_linux_amd64.zip` | `e6c68375814c0403321eedd42f71c988e5a35d72fd1f584a351da51cb00705c2` |
| `headless-dist-06/lib/zcode.so` | `9c9d09cd0332ea17b8d2251d0c673e7fcafacbb8d7250ee42fd77bba29e45a75` |

These identify the tested runtime checkpoint before subsequent audit/report
metadata. Each stage has `-result.json`, `-network.json` and `.log`.
Each installed case also retains its complete copied distribution, exact
endpoint overlay, fixture library hash, response and selected-account sequence.
Fixture endpoints use `ModuleOptions`; production transport code is compiled,
not replaced by a fake SDK module or private host validator.

The runner accepts `--sdk` and `--source`; the installed driver accepts
`--magpie` and `--distribution`, so parent can repeat against its final combined
freeze without changing plugin code.

## Final post-audit seal, 2026-11-16

After the donor no-op comparison, `provenance-tests` and `audited-package`
passed on the same guarded usbox setup. There are 29 green terminal gates,
including the 15 installed-host cases. The final distribution carries the
updated provenance; its native bytes are identical to candidate-06.

| Final artifact in the packet root | SHA256 |
| --- | --- |
| `audited-archives/magpie-plugin_linux_amd64.zip` | `cbe607e8056d72ff7437497b6ce613635c96498c31a153309aedfa2363e33932` |
| `audited-dist/lib/zcode.so` | `afb97110a628ba1c13fe870daeee56340766df6cad578229e6050f4cef2a42ca` |
| `FINAL-SOURCE-MANIFEST.json` | `09cdcbdea912d1160848ecbf7a478470bdea11702435484b6e195b622465c801` |
| `FINAL-RECEIPTS.json` | `906417b547de04bce8945c03e68e689b96273bf9f6d328cff9404b5c2abd48b0` |

[REQUEST-POLICY-RECEIPTS.json](REQUEST-POLICY-RECEIPTS.json) is an exact copy
of the final receipt. It records per-gate exit status and receipt/log/network
hashes, installed outcomes, retained old reds, preserved financial/dependency
hashes and the unchanged 10987-file host inventory. No green gate reported
unexpected external network denials.

The canonical source manifest covers 246 files with hashes and modes. It
excludes this report, the copied receipt, generated Python caches and
AppleDouble files to avoid circular evidence hashes. `FINAL-SOURCE.tar.gz`
seals the complete final package, including this report and receipt, with
those generated files excluded. Its external `FINAL-INDEX.json` records the
archive hash without embedding a self-referential hash in this report.

## Changed files and remaining gates

Runtime changes are in `src/request_policy.go`, `src/paid_dispatch.go`,
`src/start_guard.go`, `src/module.go`, `src/gift.go` and `src/window.go`.
Regression changes cover policy facts, existing paid/Start/gift boundaries,
wire callbacks, manifest floors and provenance. Active package/native/UI
verification scripts now agree on API1.12 and preserve the provider-only
composition's original optional telemetry behavior. README, detailed reference,
source lock and source guide describe the new ownership and exact SDK source.
The cleanup plan precedes runtime edits in `REQUEST-POLICY-PLAN.md`.

Parent's final combined host freeze/replay and Astra's independent review are
still required. Permit-validation and revoked-diagnostic fixes have separate
host owners; this package neither patches them nor certifies that later host.
No live vendor, actual installed client, non-Linux execution, financial resume,
publication or commit is claimed. New donor refs are compared separately;
checked refs never imply a fictitious behavior or version increment.

The subsequent parent-fetched refs were compared after these runtime gates:
[DONOR-NOOP-20261116.md](DONOR-NOOP-20261116.md) records identical donor trees
and unchanged versions. Only provenance/report metadata and its verification follow candidate-06;
the native runtime bytes remain the tested checkpoint.
