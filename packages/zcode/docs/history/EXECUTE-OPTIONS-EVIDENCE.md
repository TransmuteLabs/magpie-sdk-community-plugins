# Ordinary gateway ExecuteOptions interoperability

2026-11-13. This is a module DTO correction, independent from the host
reauthorization Usage/Refresh preservation fixes. Runtime API floors,
default headless distribution, grants and lane admission are unchanged.

## Before and after

The real gateway serializes public `sdk.ExecuteOptions.EscapedPath` for
SDK API 1.3+ requests. ZCode's strict private DTO omitted that field, so a
normal POST to `/v1/messages` failed before vendor dispatch with HTTP 502
and “Only ordinary Messages paths are supported”.

`module.go` now recognizes the field. An empty value preserves existing
callers; a supplied value must exactly equal the already constrained
ASCII path. Encoded/mismatched paths remain rejected before dispatch.
Method, query, proxy, native metadata, account/generation, operation,
grant and lane checks remain in place. Unknown-field rejection was not
disabled.

The positive regression serializes the actual public
`sdk.ExecuteOptions`, including escapedPath. Its old implementation fails
at the ordinary-execution assertion. The added negative passes
`/v1/%6dessages` for path `/v1/messages` and verifies no vendor call,
acceptance or proxy resolution.

## Frozen module evidence

Two changed executable files:

| File | SHA-256 |
| --- | --- |
| module.go | `994179abbcf75738207ef85144b380b6133112b40f494a0936617d82650a8105` |
| module_test.go | `99e515aa240dabbaf5571eab11fe22da5ee65fa69b70be5d9c587e3791d41c77` |

Proof root: `usbox:/tmp/magpie-auth-final-consumers-OfzeV5lp`.
Tests/builds used pinned Go 1.26.8, temporary HOME/XDG and offline caches
in a UID-preserving loopback-only namespace with all capabilities dropped.

- Empty-output gofmt and vet passed.
- Full module race count20 passed: **3880 test/subtest events, 194 distinct
  tests/subtests**, no failures.
- Headless and UI native packages built; headless native ABI conformance
  passed.
- Darwin arm64 and Windows amd64 CGO-disabled vet/build passed.
- Actual ordinary gateway calls reached the ZCode vendor fixture on the
  composed host; a dirty barrier subsequently blocked them and a real
  fresh quota acknowledgment reopened dispatch.

Linux headless ZIP SHA-256:
`f99ebd64f7d6c8465641b51f05bd74a901dcde0a66d4fc3be8ecc4e06b0bec20`

Linux UI ZIP SHA-256:
`06501f88e78bc53e56eab7e3dc210e72f09dbf14232174c83a220d378d8b4e2d`

## Exact old-code negative and host basis

`escaped-path-before.log` captures the newly strengthened SDK-serialization
test failing against old code at its assertion, not compilation.
The read-only old module.go from commit `7a0e269` has SHA-256:

`32afa0a0b82338aebff0d543aeb1cd1348286f6f363aeac08695e20bbe1d11b8`

`old-dto-proof/driver.log` independently reproduces the real HTTP 502 using
that old module source against the final composed host basis:

- source tree `7f0f63b30c5859bce19cdca90f0e114ee979a212`
  (explicit main a721 + corrected Auth; no Agent dirty edits);
- host binary
  `253f76efb7ffbdf9553a79b74252897f5b988175ca5b6222b81e504d415ab586`;
- frozen host inventory
  `a278427150cc3dd39829c5768d490d4144d043987fab8703b1cad41baedb6d29`.

Logs: `module-results.json`, `race20.log`, `headless-abi.log`,
`old-dto-gateway.log`, `old-dto-proof/driver.log`, `positive1/driver.log`,
`positive1/web.log`. Positive1 completed the ZCode gateway/barrier path
but its overall consumer run remains red because of a separate synthetic
OpenCode fixture URI mismatch. It is not relabeled as a complete broker
acceptance pass.

Full auth-consumer acceptance/repeats and independent review remain their
own gate. No live vendor, installed app, account data, new dependency or
push was used. Cross-compilation is not native macOS/Windows runtime proof.
