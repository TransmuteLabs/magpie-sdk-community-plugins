# Claude Desktop verification

Checked November 17, 2026, for package `agent-claude-desktop` version `0.1.0`,
API minimum `1.5`, native ABI `1`.

## Source and planner

The recovered Claude Desktop `5.11492.5` source establishes the existing
third-party gateway profile and bundled interface contract. Exact reference
commits, paths and hashes are in [source-lock.json](source-lock.json).

The current package passed formatting, `go vet ./...` and
`go test -race -count=3 ./...` with pinned Go `1.26.8` on macOS arm64:
147 named pass events across the three repetitions, including subtests, with
no failures. Two provenance tests and 61 shared workspace/release tests also
passed. Independent static review approved the configuration and ownership
scope.

Regression coverage includes profile-mode priority, normalized or removed
root values, a profile-level first-party or invalid override, chooser and
hybrid-pointer protection, opaque sealed credentials, malformed snapshots,
ownership receipts, user edits, cancellation and disconnect restoration.
The unchanged regression tests against the prior production implementation
produced 24 expected named failure events, including parent test groups,
without compilation or setup errors. That negative control establishes that
the tests detect the corrected behavior.

## Native host integration

The current native library was built and installed into an isolated macOS
arm64 Magpie host through the ordinary SDK CLI. The host was built from a
frozen source manifest containing 11,254 files. All source files were verified
before and after qualification; their contents did not change. The manifest
identity below is not a Git commit or tree and does not imply a published
release.

Retained verification record **CD-20261117-05** identifies the following
artifacts. The private raw evidence retains commands, outputs and cleanup
receipts; the hashes allow it to be matched without publishing workstation
paths.

| Artifact | SHA256 or source identity |
| --- | --- |
| Frozen host source manifest identity | `487b55b186abed720cd4d822b57e40b0ea1650a60701b7fce79788eb4f911f30` |
| Host executable | `2a4e253079e04c3464a97a7b8ce7d566c55653a669febc044ad5acfc0a4630e0` |
| Native library | `57f11046892c2695cb8d835dd0f93015569f41b88e6c2d870d13732a66c772fb` |
| Module manifest | `bdfa5f4582264fdedbc9688147dd54085ba8bfac0e1fb74805f837162c3fe419` |
| Qualification result | `1204618ebbe131e2d376bdf77cc50d5698fa678e5e3a86e6e5278bd22ddfe6fd` |
| Host scenario result | `6f744890af596514750badba97671649b7267f4fca083027cee3588238a139d1` |

The repeatable scenario is [host_e2e.py](../tests/host_e2e.py). It passed
through the real headless host CLI, owner HTTP API and gateway with a synthetic
local Anthropic server:

- Installation and inspection leave the existing Desktop configuration and
  canonical agent owner unchanged until explicit connect.
- Connect selects the SDK profile through the ordinary host ownership path.
  The four Desktop files and hash receipt are private files.
- Desktop model aliases resolve to the original upstream model. Token
  counting returns the fixture's count, and Messages and SSE reach the local
  Anthropic fixture.
- Inspection and catalog synchronization preserve normalized root modes and
  a deleted root file. A profile-level first-party or invalid override is
  refused without rewriting the configuration.
- Catalog synchronization preserves unrelated user edits and an opaque
  sealed credential.
- CLI and HTTP disconnect restore prior owned values, preserving later edits
  and normalized or deleted root values.
- An ordinary Anthropic request succeeds before connection and after
  disconnect; the provider configuration remains byte-for-byte unchanged.
- The request ledger and routing trace each record four inference requests:
  two ordinary Claude control requests and two canonically attributed
  `claude-desktop` requests, with the original upstream model and token usage.
- Installation uses no companion SDK or legacy plugin. Telemetry contains
  no fixture credential material.

The scenario uses a private HOME and an inherited macOS sandbox denying
external network operations, installed app execution and access to the real
user's configurations and Keychain. Numeric TCP and UDP negative controls
establish OS-level network denial. The direct host process and fixture thread
joined, the qualification HOME was removed, and no cleanup errors were
reported. This is not a general process-tree emptiness assertion.

## Remaining acceptance

The signed installed Desktop was not launched. Actual login-free startup,
Chat, Code, Cowork, tool continuation and managed-device behavior remain
live acceptance tasks. Follow [live acceptance](LIVE-ACCEPTANCE.md) for the
ordinary installation, connection, manual application checks and rollback.
Cowork still depends on Desktop's own VM/runtime and operating-system
permissions. Only macOS arm64 native execution was established. The amd64
target is declared but unexecuted; Linux and Windows are not declared.

The source and native library identities above define the executed behavior.
A release archive must also have its contents and checksums checked when
assembled. These package results do not establish final release readiness for
the entire SDK, other plugin packages or native Magpie GUI startup.
