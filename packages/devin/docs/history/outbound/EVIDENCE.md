# Outbound codec/client checkpoint

Date: 2026-11-13. Execution target: usbox, Go 1.26.8 linux/amd64.
Stable inbound base: `0afdd76`. The outbound work is not yet committed or
registered in the stable module manifest.

## Source authority

The installed community plugin inspected for this continuation is
`@magpie-community/opencode-devin-auth` **0.1.40**. Its index.mjs SHA256 is
`c94c6c209f0c25d433da60fd9b1857508c43f47b254f3c85c0f2f9b5ec0f51c0`.
It supplies endpoint/header/auth, generation defaults, reversible tool-id
mapping and quota behavior evidence. It was read as source only, never run
against live credentials.

Recovered generated descriptors under Agents/devinapp supply field types:

- module-98224.js: StopReason at 3486; ModelFeatures at 16706; ModelInfo at
  17599; ImageData at 22636; CompletionConfiguration at 9990.
- module-39768.js: prompt/tool definitions and choices.
- module-432.js: GetChatMessage and account model configurations.
- module-64858.js: GetUserStatus and ExchangeDevinCLIPKCECode. SHA256:
  `3e1eb28bde48285fed78d9dbe852a7c7dfecf55738e873688d06ffd12f2f10d0`.

The existing codec evidence inventory records the other descriptor hashes.
CLIProxyAPI's devin_wire/devin_payload/devin_executor and the recovered Rust
reconstruction were inspected as secondary evidence. Their dependencies,
fingerprints, prompt substitutions, missing-result invention and usage
field-6-as-status assumptions were not copied. Generated ModelUsageStats
field 6 identifies an API provider, not an HTTP status.

PKCE's generated exchange request declares code and code_verifier only.
The installed TS source sends an additional redirect_uri JSON property;
this client follows the generated two-field RPC and validates the callback
redirect/state locally. Real server parity for that choice is not claimed.
Returned webapp/API hints are not used as credential destinations.

## What was verified

All tests/builds ran on usbox with temporary HOME/XDG, the existing pinned
Go caches, GOTOOLCHAIN=local, GOPROXY=off and GOSUMDB=off. HTTP peers are
httptest/synthetic loopback servers or an injected no-network RoundTripper.
No recovered cassette, account file, browser, CLI sign-in or vendor endpoint
was exercised.

The public SDK replacement in this private verification workspace points to
the frozen merged host `01bcc7c7d155b029769327f909372b2ff72252e2`. The domain
code does not depend on pending SDK 1.3 APIs or host-private packages.

| Gate | Terminal result |
| --- | --- |
| gofmt -l all Go files | Exit 0, no output |
| go test -mod=readonly -json -count=1 -timeout 2m ./... | Exit 0, 404 PASS events |
| Complete module race count 3 | Exit 0, 1212 PASS events |
| Client/PKCE/status race count 20 | Exit 0, 400 PASS events |
| go vet -mod=readonly ./... | Exit 0 |
| Stable native distribution rebuild | Exit 0 |
| Actual C ABI, two concurrent sessions and CLI install | Exit 0 |
| Full inbound HTTP E2E with rebuilt native library | Exit 0, all 11 checks, six provider inference requests |

The rebuilt native artifact still advertises only the original inbound/
discovery capabilities. Its full HTTP check therefore proves that the
existing slice remained stable; it is not an SDK outbound-handler test.

The tests cover selected credentials, callback/state/deadline checks,
generated PKCE exchange fields, explicit pre-dispatch expiry, refusal before
acceptance, exactly-once acceptance callback ordering, no automatic retry,
redirect credential isolation, streaming cancellation, gzip bounds,
fragmentation, terminal/truncation checks, complete tool arguments, model
availability/features, quota unknown/omitted-zero behavior and free UIDs.
Native output-token zero is retained rather than estimated.

Three regression tests first produced real failures in the initial client:

- A lost durable-acceptance acknowledgement was incorrectly labeled
  not-started, even though no vendor request was sent. It is now uncertain.
- Duplicate sessionToken fields were accepted from PKCE exchange.
- Duplicate remaining-quota fields were accepted as a trustworthy reading.

The failing output is retained in regressions-red.log. Fixes are included in
the green aggregate above.

Two independent negative overlays also produced real assertion failures:

1. Replacing explicit zero output tokens with 1 fails
   TestOutboundNativeStreamToolsSignaturesUsage.
2. Dropping typed native HTTP refusal statuses fails
   TestClientNeverRetriesOrFollowsNativeFailures, including terminal quota.

These are behavioral failures, not compile errors. The overlays did not
modify the verified source.

## Immutable evidence

Remote root:

```text
/tmp/magpie-devin-outbound-gulzqv92/
  final-source.json
  final-results.json
  regressions-red.log
  negative-results.json
  completion.json
  tests.log
  race.log
  repeat.log
  distribution/
  abi.json
  http/http-e2e-result.json
```

Source inventory SHA256:
`a4d7b68ed43f8ef1001adad293151492ea83672801b78f0b2fb2d18ea87883ff`.
Every executable source matched that inventory after verification.

Rebuilt native library SHA256:
`b31160f9cb0af075a3a52cdaef2ab803afa3f1fc79fcf45939dd50f97d173986`.
The full HTTP check used host SHA256
`e496270ece5d0e7659c5b89937141ddaec973f3d68f4e608112e9cbd0e62c1ba`.
The temporary gateway process group was gone and its HOME removed.

## Work remaining at this historical checkpoint

This checkpoint is the recoverable pure outbound layer, not completed
provider registration. SDK auth/controller/execute handlers, canonical
stream rendering, state CAS for one-use PKCE and native-origin receipts
remain module work. Native-only fallback enforcement and quota model
filters are identified with source hashes in HOST-GAPS.md.

No new SDK behavior, live native Devin parity, signed-history replay,
refresh/revoke endpoint, background cache keepalive or non-Linux execution
is claimed. The existing 0.2.0 refusal boundaries remain enabled.

## Subsequent paired implementation

The root module now registers the opposite direction in its 0.3.0 manifest
with SDK 1.4. This earlier pure-client receipt does not prove those changes;
their actual native-host proof is recorded in `../PAIRED-EVIDENCE.md`.
