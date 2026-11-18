# Registered offline adapter and local UI prototype verification

2026-11-13. This continuation preserves committed headless checkpoint
`60d81447e8658a0fb0b9d35b5a3ab7128c0a7220`. The runnable native module
continues to require unreleased SDK **1.4**. The SDK 1.5 control state layer
is separate, unbundled local prototype code.

## Executable-source identities

Remote proof root: `/tmp/magpie-zcode-session-_x9kl829`.
All **59** Go/Python/module files match the tested source. SHA-256 of their
compact key-sorted JSON `{relativePath: fileSHA256}` manifest:

`a3068739604aef8787e1b1729b98c3e027b9e09cda95a912d27cc194cfe348b1`

Native Linux ZIP SHA-256:

`9e0255f613d5ffc1b3d2f784289434d01255c992bcbacd74fba9d560b5df8161`

Actual native-host binary SHA-256:

`bf11f37b125b4a6b59ba3c8501b323cc98b9556bad55ed1af3b8fa477092efca`

The SDK 1.4 public-source copy remains the previous immutable lane copy.
All builds/tests ran on **usbox**, Linux amd64, Go 1.26.8, isolated HOME/XDG,
pinned caches, offline module resolution. No live vendor/app/account,
host secret access, new dependency, publication or installed activation.

## Completed 1.4 gates

- Final `go test -json -race -count=20 -timeout=4m ./...`: exit 0,
  **3340 passing events, 167 distinct tests/subtests**, no failures or
  skipped tests; `race20-recheck.log`.
- Formatter exit 0 with empty output, vet exit 0.
- Linux C-shared ZIP/checksum builder and two concurrent native ABI sessions:
  exit 0, five calls/session, no Bun/network callbacks.
- Darwin arm64/Windows amd64 CGO-disabled vet/build: all exit 0.
  Native execution on those platforms is not claimed.
- Compiling negative path/source-overwrite overlays fail the intended
  session-adapter regressions.
- Existing native controller/headless-tool E2E remains green, including
  selected generation, MCP JWT roles, no replay, cancellation, and the
  informational-tool usage barrier.

The actual native `auth_e2e.py` additionally exercises the registered
SessionMigration adapter through the host's `plugin sdk session` path:

1. A real `zcode-history` descriptor resolves its real session.convert handler.
2. Preview leaves the existing target and source unchanged.
3. Explicit apply writes the validated bundle and host backup manifest.
4. The backup contains the exact previous target.
5. A second explicit plan restores original JSONL bytes, including CRLF,
   opaque signature text and integer `9007199254740993`.
6. Source remains unchanged, and a traversal output plan is rejected.
7. Normal native auth/reauthorization/model/inference proof still passes.

The module returns one file plan and a content-derived result ID. It never
opens the source/destination, creates the business dataDir or contacts a
vendor. The descriptor intentionally has no automatic agent-reader/resume
route. Multiple/mixed inputs, source overwrite, unsafe paths, unknown
options and unsupported cross-agent conversion are rejected.

An intermediate race20 run saw another older native fixture's generic
unexpected-target assertion. Its method/path diagnostic was improved;
the final complete rerun passed. The original request was not captured
well enough to establish its cause. No production change was justified by
that observation. The earlier related observation remains documented in
AUTH-EVIDENCE.md.

Logs: `final-results.json`, `recheck-results.json`, `*-final.log`,
`race20-recheck.log`, `negative-results.json`, `negative-*.log`.

## Separate SDK 1.5 client prototype

Four prototype source/declaration files have manifest SHA-256:

`412d0b414b9c9d453ce5e908fae2899f7032138b1c37f0522a6f24a58f30cfde`

The actual read-only `magpie-sdk-next/sdk/js/ui.d.ts` copy used for strict
typechecking has SHA-256:

`1fa03ca694b04b3d3e6116201b24df52d1faf476c4d347741078626845454905`

Strict TypeScript compile against those definitions passed on usbox;
Node's built-in test runner passed **8/8** client state tests. Numeric
generation and client-selected provider counterexamples fail typechecking.
The available compiler rejects removed `baseUrl`; the verification config
uses absolute paths mappings without that obsolete option.

The client uses exactly backend.accounts(binding) and
backend.call(binding,input,{account:{id,expectedGeneration}}). Fixed
provider declarations stay in the binding fragment. Tests prove string
generation preservation, no account/credential-summary leakage, stale
enumeration/read discard, generation-change deselection, single-flight
writes and no uncertain-write retry.

This state layer is not a bundled/installed page or a host auth broker.
No final SDK 1.5, browser visual, native GUI or installed UI acceptance is
inferred from typechecking and fake-bridge state tests. The runnable manifest
and ZIP retain SDK 1.4 and contain no prototype page/assets.

## Remaining domain work

Official remote MCP JSON-RPC execution/dynamic tool publication, live native
resume/automatic agent integration, reviewed archived SQLite import and the
final selected-account UI/auth broker remain separate work. The history
bundle preserves external references without fetching missing bodies.
