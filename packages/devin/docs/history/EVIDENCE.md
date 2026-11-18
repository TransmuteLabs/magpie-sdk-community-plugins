# Standalone Devin module verification

Date: 2026-11-13. Target: `usbox`, user `maratkarimov`,
`go version go1.26.8 linux/amd64`, cgo enabled.

All execution used private temporary HOME/XDG directories, pinned Go caches,
`GOTOOLCHAIN=local`, `GOPROXY=off` and `GOSUMDB=off`. No vendor traffic,
credentials, account configuration or recovered cassette bytes were used.

## Source and SDK boundary

At commit `d6841b1`, the eleven extracted files in `codec/` matched every
SHA256 in `donor-files.json`.
They were extracted without behavioral edits. Their historical schema and
fixture evidence remains in `codec/EVIDENCE.md`; it is distinct from this
standalone native installation proof.

Public dependencies reported by `go list -mod=readonly -deps` were only this
repository, `github.com/yetone/magpie/pkg/protocolext`,
`github.com/yetone/magpie/pkg/extension` and its `native` package, in addition
to the standard library. No core/private or donor package is imported.
Root `go.mod` was not changed during relocation verification.

The immutable SDK base was
`7e2a72b6178eb2a51c4e6018ee9515fdbb321da7`, from:

```text
/tmp/magpie-sdk-v11-usbox-niq306rt/sdk12-final-source
```

This base loses public codec HTTP rejection statuses. The regression
`TestSDKPreservesNativeDeadlineHTTPStatus` fails there with:

```text
SDK lost the codec's native HTTP 504 rejection:
error=Connect request deadline exceeded
```

The base run had **311 passing test/subtest/fuzz-seed events and one failing
test**. Its native conformance also completed installation and positive
fixtures but exited 1 because the deadline error was `invalid_params`
instead of `http.504`. This is a behavioral regression, not a compile error
or a skipped check.

For the green checks below, a private copy of that base received the
status-fidelity owner's three current source files. No shared SDK source
or immutable snapshot was modified:

| SDK overlay file | SHA256 |
| --- | --- |
| `pkg/extension/errors.go` | `df4e6899171458a59052bd6b261c99f2cc0249d0a21898a23ab6b1e99e2477b5` |
| `pkg/extension/inbound.go` | `88eae0e0c4113413baa591bc0a66b77a5ba1953ec6095ff4be6941abb5a9e4cf` |
| `internal/extension/inbound.go` | `f81a2049721e03b8a21f44fb22e16b66c10746bd2365a5909f58eb1d5f20a6fe` |

This overlay is verification of the owner's pending fix, not evidence of
a published SDK release or final combined host validation. The module keeps
the SDK API 1.1/native ABI 1 declaration and the status regression.

## Checks

| Command | Result |
| --- | --- |
| `gofmt -l module.go module_test.go cmd/native/main.go` | Exit 0, no output |
| `go test -mod=readonly -json -count=1 . ./codec` | Exit 0, **312** PASS events, **49** top-level tests/fuzz targets |
| `go test -mod=readonly -race -json -count=1 . ./codec` | Exit 0, **312** PASS events, no race report |
| `go vet -mod=readonly . ./codec ./cmd/native` | Exit 0 |
| `python3 build.py …/status-distribution` | Exit 0, Linux/amd64 C-shared library and package manifest |
| Private SDK copy: `go build -mod=readonly -tags nogui -p 3 -o …/magpie-status .` | Exit 0 |
| `python3 conformance.py …/status-distribution --magpie …/magpie-status --result …/status-conformance.json` | Exit 0 |

The 312 PASS events comprise **7** module events, including three discovery
subtests, and **305** donor codec test/subtest/fuzz-seed events. Ordinary test
execution includes fuzz seeds; no timed fuzz campaign was run.

`conformance.py` loaded the built library through
`magpie_extension_open_v1`, `magpie_extension_send_v1` and
`magpie_extension_close_v1`. Two concurrent sessions reused the same
synthetic response IDs without crossing outputs. The fixture overwrote the
temporary host descriptor and sent byte buffers after each ABI call returned.
Both sessions completed:

- Public `sdk.describe`, component configure and cooperative stop.
- Chat decode to `group/m` and a user `hello`.
- Fragmented canonical SSE conversion to native Connect text, ordinary
  stop reason 2 and a successful terminal frame.
- All three unary discovery operations; model UID preservation and an empty
  advisory model-status result.
- Explicit `max_newlines`/`top_k` refusals and preserved `http.504` timeout
  rejection.

The same driver then used the actual host executable to add the distribution,
inspect `devin-inbound` as runtime `native` and decode the Chat fixture.
Its HOME/XDG was temporary and `MAGPIE_BUN` pointed to a nonexistent file.
This installed fixture passed without an external Bun worker.

Verified distribution `plugin.so` SHA256:

```text
73a9e13e2d6c48b124f91e30591075554c0e3e5adfa0f079f94b50d429ddf3e9
```

Test host SHA256:

```text
02b0d97afe2253088f2a48ffa7cf1c5df48105b33d08f9ea3ad4c513b3f8ecc5
```

Remote logs and artifacts:

```text
/tmp/magpie-devin-standalone-xv1eub30/
  baseline-tests.log
  baseline-conformance.json
  sdk-overlay.json
  status-tests.log
  status-race.log
  status-vet.log
  status-native-build.log
  status-host-build.log
  status-conformance.log
  status-conformance.json
  status-fixed-results.json
  status-distribution/
```

## Limits of the evidence

The CLI check proves installation and installed decode. Connect response
fixtures were exercised through SDK pipes and the actual C ABI, not a full
gateway HTTP request routed to a live provider. Native execution was tested
only on Linux/amd64. There was no full Magpie suite, live Devin session,
Windows/macOS execution, publication or benchmark.

Nonzero `max_newlines`/`top_k`, signed input history and the other explicit
refusals in `codec/README.md` remain unsupported. The SDK's 16 MiB body limit
includes the Connect envelope, unlike the direct codec's protobuf-only limit.
Neither limitation was bypassed.

## Version 0.2.0 continuation

The original extraction above is historical. The current continuation
intentionally changes decoder/catalog files and adds
`codec/continuation_test.go`; `donor-files.json` remains the original
provenance record.

The public SDK used to compile and test the continuation was exported from
merged host commit `01bcc7c7d155b029769327f909372b2ff72252e2`, including
`go.mod`, `go.sum`, `pkg/extension` and `pkg/protocolext`. This required no
private host imports or dependency download. Checks ran only on `usbox`
with Go 1.26.8, isolated HOME/XDG and the existing pinned Go caches.

Before implementation, these new tests failed against the old behavior:

- `TestRecoveredCascadeAndSiteCatalogs`: four missing method identities.
- `TestRecoveredSystemPromptAndAdvisoryFields`: supported generated fields
  were refused.
- `TestChatRejectsAmbiguousConnectHeaders`: four duplicate singular headers
  were accepted.
- `TestRecoveredToolResultImages`: a valid native image result was refused.

The final source did not change during verification:

The run contains 54 top-level tests/fuzz targets. The only package-level
skip is `cmd/native`, which has no test files; no test fixture was skipped.

| Check | Result |
| --- | --- |
| `gofmt -l` on every Go file | Exit 0, no output |
| `go test -mod=readonly -json -count=1 ./...` | Exit 0, 346 test/subtest/fuzz-seed PASS events |
| `go test -mod=readonly -race -json -count=3 ./...` | Exit 0, 1038 PASS events |
| New recovered-schema/header and SDK discovery tests, race count 20 | Exit 0, 980 PASS events |
| `go vet -mod=readonly ./...` | Exit 0 |
| Native C-shared distribution build | Exit 0 |
| Actual C ABI conformance and isolated native CLI install/decode | Exit 0 |

The C ABI run includes two concurrent native sessions, all seven discovery
methods, source-5 system prompts, associated tool-result images, explicit
team/unsupported-control refusals, HTTP 504 preservation and successful
Connect termination. Bun was deliberately unavailable for the CLI check.
The CLI install fixture uses the previously verified host executable;
compilation uses the merged SDK source stated above. This does not claim a
full rebuilt merged-host gateway run or live Devin parity.

The complete immutable file hashes, red logs, test logs, distribution and
conformance results are retained at:

```text
/tmp/magpie-devin-continuation-jfmi7m1r/
  source-files.json
  red-result.json
  red.log
  tool-images-red.log
  final-results.json
  final-*.log
  conformance.json
  verification-summary.json
  distribution/
```

`source-files.json` records every Go/Python source, go.mod and manifest
SHA256. Documentation added after that run does not alter executable bytes.
Remaining host/emitter contracts are listed in `CONTRACT-BLOCKERS.md`.

The version 0.2.0 Linux/amd64 library SHA256 is
`1980a42f798e12d5708b39373577208c7adc98525a6c143df656bb475c6586cb`.

## Full native HTTP ingress

`http_e2e.py` closes the earlier C-ABI-only verification gap. It runs the
ordinary compiled `magpie serve` executable, installed native distribution,
HTTP listener and Server.Front guards, then sends native Devin requests over
TCP. The destination is a real local HTTP server implementing a synthetic
Chat provider. There is no replacement gateway, test-only host API or Bun
process.

The host was built on `usbox`, Go 1.26.8, cgo enabled, from an immutable
`git archive` of merge commit
`01bcc7c7d155b029769327f909372b2ff72252e2`:

| Artifact | SHA256 / identity |
| --- | --- |
| Source Git tree | `d4e911f8665aaf78f2c1fa9925d1ee0470a96297` |
| Source archive | `f3e978c02b90a7dbe6f9ac7d80f92cdcae58f6d237222d67531863bc79b5e17d` |
| Compiled full host | `e496270ece5d0e7659c5b89937141ddaec973f3d68f4e608112e9cbd0e62c1ba` |
| Actual native library | `1980a42f798e12d5708b39373577208c7adc98525a6c143df656bb475c6586cb` |
| Final HTTP runner | `5f9486cb5d3eff469d17231c163ea9d578c96651bded0901e7a39f4bc46c7383` |
| Imported conformance helpers | `65440f655cf737e75bd02305d0b0b1171fb3616485659a57d0f450303075cfec` |

The actual build command and terminal exit 0 are recorded in
`host-build.json`. Before each run, the harness verifies the executable
against that provenance. Before requests, it checks the responding version
and gateway.json listener address/PID against its own subprocess.

The final runner passed **ten consecutive fresh sandboxes**, runs 18–27,
with 11 checked scenarios and six inference HTTP requests each:

| Scenario | Observed behavior |
| --- | --- |
| Front authentication / readonly | Forwarded unkeyed native call gets HTTP 401; readonly key gets 403; neither reaches the provider |
| Text, usage and completion | Fragmented provider SSE becomes native text, token usage, stop reason 2 and one successful terminal frame |
| Credential isolation | Provider sees its configured synthetic upstream key; caller key, native credential envelope and native credential headers are absent |
| Tool reply | Fragmented function arguments reconstruct one native call with its original id/name/JSON and stop reason 10 |
| System and image history | Native source-5 system text and associated tool-result inline image reach the HTTP provider |
| Authorized discovery | All six configuration methods expose only fixture/visible; advisory statuses remain empty |
| Model authorization | A caller restricted to fixture/visible cannot invoke fixture/hidden; permission_denied is framed and no inference is sent |
| Malformed requests | Chat retains HTTP 200 with invalid_argument terminal; unary discovery returns HTTP 400 and Connect error JSON |
| Native deadline | Partial text survives, terminal is deadline_exceeded, and the provider observes connection cancellation |
| Client disconnect | Closing the real TCP client after a native data frame cancels the provider connection |
| Module off/on | CLI off makes native ingress return 404 without inference; CLI on restores the full roundtrip |

All ten runs exited 0 with `complete: true`, no external proxy request
targets, Bun unavailable and sandbox removal confirmed. Fixture-owned host
processes were waited for after SIGTERM; `gateway_exit: -15` is intentional
process cleanup, not a claim of graceful module disposal on SIGTERM.
The final completion audit confirmed 110 scenario checks, 60 provider HTTP
requests, all temporary homes removed and zero remaining fixture process
groups. The runner/helpers and host archive still matched their inventories.

Early fixture failures remain recorded rather than counted as passing.
Run 1 detected blocked catalog/partner background fetch attempts; adding the
supported lab-cache fixture and MAGPIE_PARTNERS=off removed them. Later runs
7/14 exposed ordinary relative-root GET probes on local fixture listeners:
those now return conventional HTTP 404, without being miscounted as inference
or absolute-target proxy egress. Both root contracts are exercised before
each run. CONNECT and absolute-target HTTP requests remain denied and
recorded; any such request fails the final fixture.

Remote artifacts:

```text
/tmp/magpie-devin-http-e2e-jqlc_i6m/
  host-source.json
  host-build.json
  host-build.log
  host.tar
  magpie-01bcc7c7d
  http_e2e.py
  conformance.py
  module-source-files.json
  completion.json
  http-e2e-final-runs.json
  run-18/http-e2e-result.json
  ...
  run-27/http-e2e-result.json
```

This proves native HTTP ingress through the frozen merged host into a
synthetic ordinary provider. It does not prove live Devin parity or implement
the same-module native outbound executor. The latter's existing metadata and
generation contracts, and the remaining route/origin policy requirements,
are described in `CONTRACT-BLOCKERS.md`.

The complete executable-source inventory SHA256 is
`e3b349412ac4279d2c19b193f1ad0977066032192ae8664c98a99a39fa403753`.
It includes the new HTTP runner alongside every Go/Python source and package
manifest. No executable source changed after the verified snapshot.
