# Native completion and origin-receipt checkpoint

Date: 2026-11-14. Module baseline:
`bd6b1616bb1bb1fc5e7bba1d50f6182375fed68c`.
Paired distribution 0.3.1 / SDK 1.4, agent 0.5.1 / SDK 1.6,
inbound-only 0.2.0 / SDK 1.1; native ABI 1 throughout.

## Gap and implementation

The paired raw reader previously accepted an empty successful terminal,
incomplete/error/unknown stops, changed stops and content after a stop.
Its successful-terminal callback could consequently persist signatures from
an incomplete reply. An interrupted read could also return EOF instead of
the cancellation cause.

Raw and canonical readers now share generated response-field, usage and
completion validation. A successful terminal requires native content and
a supported final stop. Unknown response/tool/usage controls, empty tool
deltas and provider-refusal state fail explicitly. Raw native-only fields
and advisory usage after the stop retain their bytes and explicit zero
output counts. Known successful raw stops are 2/3/4/5/6/8/10; the canonical
renderer still refuses stops without its own established mapping.
Nonfinite/error stops 7/13, partial/incomplete state and filtering cannot
become a successful canonical stop or a new origin receipt.

Generated GetChatMessageResponse committed_acu_cost field 22 is a double,
protobuf fixed64 wire 1. The canonical decoder previously expected varint
wire 0. Exact committed descriptor hashes, donor versions and watchpoints
are in `source-lock.json`; no source was taken from another owner's dirty
working tree. The extraction/cleanup plan preceded edits in `STREAM-PLAN.md`.

## Frozen host and sources

The ordinary host was built from a fresh git archive of
`9c4189089fa5d25656e87e72df174027d1bace56`, tree
`91f0d12e635f246c84e81aa71fa8f32417bab85e`. The public SDK replacement
points to that exported sibling. No host repository was changed.

Recovered wire/app source remains
`97a4883009bc988fedcdf0416a50a6c62f8d9c9b`; the reused unmodified Rust
reconstruction reports `devin 3000.11.228 (053434014dbd)`.
Community domain comparison remains `26ea8faf7793ca66dd8e7a8b90128a5235552a1f`
/ packages/devin 0.1.40. Maintained CLIProxy comparison remains committed
`842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9` / v8.0.42-698-g842cedec8.
These sources are read-only evidence, not installed runtime plugins.

The 63-file Go/Python/go.mod/manifest inventory digest is
`dc8c35d18c152f3578595e650c0370b46348e14d30ef01d6944558fbaf5a77bc`
(SHA256 of the sorted compact path-to-SHA256 JSON map). Prose and the
source lock are excluded from that executable inventory.
At runtime commit 7a621943, all 65 executable/provenance input hashes matched
the local files and tested usbox snapshot (`runtime-7a621943/source-match.json`).
All three distributions
contain the selected manifest's exact lock identity and matching source
guide/source references.

## Terminal verification

All builds and tests ran through ssh usbox, Go 1.26.8 Linux/amd64,
isolated HOME/XDG, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off and existing
pinned Go caches. The initial shared-server cap was released by the user;
the final run has no artificial GOMAXPROCS/-p cap.

| Gate | Result |
| --- | --- |
| Baseline regressions with original runtime files in a Go overlay | Exit 1, 29 FAIL records including parent tests; real assertion failures, no compile failure |
| Complete module unit suite | Exit 0, 501 test runs / 504 PASS events |
| Complete module race count3 | Exit 0, 1503 test runs / 1506 PASS events |
| gofmt -d, go vet ./... | Exit 0, no formatting/static-analysis diagnostics; Go tests compile every module package |
| Python static compilation, provenance regressions | Nine Python files compile; five provenance tests pass |
| Ordinary pinned host and three C-shared distributions | Build exit 0 |
| Actual paired HTTP with live account-generation replacement | 20 grouped checks, 23 inference requests |
| Actual agent launch/bootstrap/assignment/resume/cancellation | Six grouped checks, four inference requests |
| Unmodified reconstructed application startup and first turn | Exit 0, native preflight and one inference request |
| Inbound-only C ABI and ordinary SDK CLI conformance | Exit 0, two concurrent native sessions |

Three command packages have no tests. Their package-level skips are not
skipped regressions. The paired HTTP fixture adds ten malformed local vendor
streams, verifies exactly one outbound dispatch for each, then verifies that
the incomplete stream's signature cannot authorize another vendor request.
Existing successful signed replay, cross-caller/generation refusal, actual
deadline/disconnect cancellation and selected-credential boundaries pass.

Agent and reconstructed-startup sandboxes each contain exactly the single
own `devin-inbound` bundle, no community provider plugin, helper package or
Bun runtime. Every HTTP fixture reports zero external proxy requests,
stopped processes and removed HOME. No live vendor, account credentials,
installed app, dependency download, push or publication was used.

## Artifact SHA256

| Artifact | SHA256 |
| --- | --- |
| Ordinary host, pinned commit above | `bf635fe92011595ff4925f64ba8ed5e1cfd17a099a2ea5aa0b2abdd6ef1b6f73` |
| Agent library | `ed201924ba8106183c125acb720413c14f9c565569856791ea22cce90c542740` |
| Paired library | `638e4849125cc023d30e3617a0b02e2e23af5d3c5a0488c87031fb7449dddd66` |
| Inbound-only library | `d02afb88062f7db0982cf3aef3b10ebecfd62496700b544cce0d5d15542403ea` |
| Reconstructed application | `8b5438632942381272d20974f7ee59fd9af7674d452dce7ea448077b4c075327` |
| Loopback network guard | `c7012e29410d0585e675caef74ceef84216ac0e4e2e6210ea3d8ae863e98279b` |

Evidence root on usbox: `/tmp/magpie-devin-stream.3QqogL/`.
`gates.py`, `runtime-7a621943/gates-result.json`, `regressions-red-final.{json,log}`,
`baseline-overlay.json`, per-gate logs, `paired-http/result.json`,
`agent-http/result.json`, `recovered-startup/result.json` and
`inbound-abi.json` retain commands, counts, exact inputs and output hashes.
Historical `BOOTSTRAP-EVIDENCE.md` hashes remain unchanged.

## Limits and handoff

These are reconstructed local peers, not the original Cognition client or
a live-server compatibility proof. No broad host/union suite, non-Linux
native execution or original-client parity is claimed.

Unknown future native response controls deliberately refuse execution.
The new completion gate governs newly issued receipts; it does not
re-attest or purge receipts previously saved by an older build.
At this historical 9c checkpoint, cold CLI catalog selection and Source6
union verification were unresolved. The d070 results now resolve those
two items; see `UNION-EVIDENCE.md` for the current partial acceptance and
its confirmed duplicate-assignment failure. Remote/restricted/group
delegation, native Basic ingress recognition and post-reply scheduling
remain separate generic boundaries in `CONTRACT-BLOCKERS.md` and
`outbound/HOST-GAPS.md`.

## Subsequent consolidation

Runtime change is committed as
`7a621943095fb7f4a1e05857ea380a6788fb161a`. The leader subsequently
clarified that the final host is one devzeus Source + nativeops runtime.
Its committed d070 runtime and documentation-only b5e4494 consolidation
are recorded in `SOURCES.md` and the source lock. The binary/HTTP evidence
above remains evidence for 9c only. `UNION-EVIDENCE.md` records the separate
bounded d070 runs without treating successful agent retries as acceptance.

The metadata follow-up reran all five provenance regressions and rebuilt
the three distributions on usbox with the same sandbox/offline caches.
All host, guard and plugin library hashes above remain identical.
At that metadata-only checkpoint, packaged source locks retained the
verified 9c source and exact variant identities with a separate reported
integration target. Current packaging pins the tested d070 runtime.
`union-metadata-result.json` verifies all 67 final local executable,
provenance and handoff inputs against the usbox snapshot; the 63 executable
inputs and their inventory digest remain unchanged. The original runtime
receipts are preserved under `runtime-7a621943/`. This follow-up verifies
metadata and packaging only; it supplies no execution evidence for the union.
