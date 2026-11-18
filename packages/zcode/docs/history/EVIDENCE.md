# Native ZCode verification

2026-11-13. All compilation and execution ran on **usbox**, Linux amd64,
using Go 1.26.8, temporary HOME/XDG and pinned caches. No live vendor calls,
credentials, sessions or installed app changes. Verification did not publish
or commit artifacts.

The tested checkpoint contains 43 owned files. Every one was compared with
the local checkout after verification: zero hash mismatches. The verification
manifest also records the exact public SDK files supplied through the
development `replace`; other host work may continue independently.

- Remote root: `/tmp/magpie-zcode-native.5S8zm9QB`
- Manifest: `final-source.json`
- Manifest SHA-256:
  `7ad76822fb384d6fd5b796c9f964480128d18e35aa344368dcc76aef5f48da6e`
- Results: `sdk13-final-results.json`

## Gates

| Gate | Result |
| --- | --- |
| gofmt | Clean |
| `go test -json -race -count=20 -timeout=4m ./...` | Exit 0; 1840 test/subtest pass events, 92 distinct names, zero failures/skips |
| `go vet ./...` | Exit 0 |
| `CGO_ENABLED=1 python3 -I build.py` | Exit 0; native platform ZIP and checksums |
| `python3 -I conformance.py DISTRIBUTION` | Two concurrent C-ABI sessions, five calls each, one Python host PID, no network callbacks |
| `python3 -I e2e.py HOST_BINARY` | Real host 1.3/native module/loopback vendor, exit 0 |
| Darwin arm64 and Windows amd64, CGO disabled | `go vet ./...` and `go build ./...` all exit 0; feature-off compilation, not native runtime evidence |

The controller suite includes an actual second test process contending for
the journal's OS lock. No race fixture launches installed agents.

The native ZIP is `magpie-plugin_linux_amd64.zip`, SHA-256:
`ebf7c5c70d3ef6f77abdb66615baf2dec3e8d28df44c92d8ff9f728d756fd4c0`.

## Actual host/native path

The E2E host binary has SHA-256
`6784b7e6d0a2b30f7e8a81922c03aac8436b5c113322d10a9b2b7ea816916a8a`.
It is the SDK 1.3 verification binary supplied by the host lane, not an
installed application.

The fixture overlays only `cmd/native/main.go` to inject loopback endpoints.
Provider/controller/codec logic is unchanged. The production ZIP is also
built and tested through the C ABI independently.

The path proves:

1. The business bootstrap and PAT are returned as one unary replacement.
2. Host publication writes credentials, expiry, generation 1→2 and receipt
   atomically; the public receipt contains no credentials.
3. A separate native inference process reads and uses the replacement PAT.
4. Candidate configuration does not create the configured shared dataDir.
5. Declared OffPeak model admission creates one ticket, which two separate
   CLI processes reuse for their two turns.
6. Two settlement intents issue only one vendor settlement POST.
7. An ambiguous issuer response leaves a durable host fence; another CLI
   invocation does not repeat the exchange.

## Behavioral negative proofs

Four overlays compiled successfully and each produced assertion failures:

- Remove GLM effort normalization → native reasoning fixtures fail.
- Remove selected-account identity comparison → wrong-account dispatch
  regression fails.
- Ignore existing execution admission → lost-reply ticket regression fails.
- Open shared storage from `Start` → candidate-readiness regression fails.

These were failing behavioral assertions, not syntax/build failures.

## Limits

Fixtures are synthetic and based on the read-only archived ZCode 3.18.0
implementation. This is not live protocol parity, entitlement or
production availability evidence. No live account sign-in or deployment
was performed. Windows/macOS runtime behavior is not established by the
Linux execution. Cross builds are only compilation evidence.

Interactive login, dynamic entitlement/model configuration, native session
readers/resume, UI/MCP/marketing surfaces and reviewed import of the archived
SQLite journal remain separate vendor work. Existing unresolved archived
state must be imported before production activation of mutating controls.
