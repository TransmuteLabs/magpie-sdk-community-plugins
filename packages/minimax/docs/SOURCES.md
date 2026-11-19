# Source provenance

This is a Go port of `packages/minimax` version 0.2.9 from
`magpie-community/plugins`, pinned at
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`. The source donor is not a runtime
dependency. `source-lock.json` records each donor file's SHA-256 and the copied
matrix/schema fixtures. The donor MIT notice ships as `DONOR-LICENSE.txt`.

On November 16, 2026, fetched community commit
`b35d4b35b2446bd929896ddeb58f3cb299e9e2d1` still contains version `0.2.9`.
Its `packages/minimax` tree is
`098cf5e689d6df7e675b7b5ef81c9e4780fa4bc7`, identical to the pinned donor.
This comparison requires no behavior port and does not change the original
implementation baseline or claim a new package verification.

The November 17 comparison fetches
`324acd6007f2d513c5072fe2e9fe1db3524ae03d`. The complete package tree
is still `098cf5e689d6df7e675b7b5ef81c9e4780fa4bc7`, including its tests
and fixtures. `latestCommittedDonorReview` records this additional check;
the extraction pin and package version remain unchanged. No runtime port
or additional vendor/client acceptance is inferred from this no-op.

On November 19, the `packages/minimax` tree at
`140c513b81b0a3302e5a18c8c22f028b10ffd92c` still matched the extraction pin.
The `submit_video_generation` schema in `matrix-tools.json` specifies a
concrete aspect ratio for H3-family text-to-video, defaulting to `16:9`.
The native media route now applies that default when neither the request
nor its size supplies a concrete ratio. Keyframe and reference modes
retain their existing ratio handling.

| Donor | Native owner | Watch points |
| --- | --- | --- |
| `index.mjs`, `sign*.test.mjs`, `poll-retry.test.mjs`, `refresh.test.mjs` | `src/auth.go`, `src/client.go` | PKCE, both device formats, rotating tokens, signing query order, site redirects |
| `request.test.mjs`, `stream.test.mjs`, `region.test.mjs`, `fallback.test.mjs` | `src/execute.go`, `src/failure.go`, `src/client.go` | identity compression, exact request bytes, SSE termination, refusal semantics, pre-dispatch CN fallback |
| `models.test.mjs`, `modalities.test.mjs` | `src/models.go` | live limits, explicit Anthropic chat models, reasoning efforts, modalities |
| `usage.test.mjs`, `checkin.test.mjs` | `src/usage.go`, `src/checkin.go` | plan denominator/boost, complete credit ledger, credit expiry, claim result/streak |
| `connector-tools.mjs`, `web-tools.mjs`, `matrix-tools.json` and tests | `src/tools*.go`, `src/search.go` | fresh catalog/schema, selected account, service actions, write acceptance and usage receipts |
| `native.mjs`, `native.test.mjs` | `src/media*.go` | operation envelopes, upload grants, asset readiness, content download and cancellation |

The Go module preserves the vendor wire where public SDK ownership allows it.
The host owns credentials, generation checks, proxy selection, refresh locking,
request cancellation and operation receipts. The donor's process-global token
cache and refresh owner are deliberately not ported. See `CONTRACT.md` for the
current reactive-refusal boundary and bounded media transfer limits.

The initial verification SDK is the parent's immutable source4 snapshot at
`usbox:/tmp/magpie-astra-combined-20261115-v4/source`, API 1.11/native ABI 1.
Each distribution records actual SDK file hashes in `build-info.json`; it does
not attribute uncommitted SDK bytes to an unrelated commit.

The API 1.12 integration uses the supplied frozen
`usbox:/tmp/magpie-request-policy-20261115/candidate-closure` public contract
and the separate `/tmp/magpie-policy-continuation.TqAAe31N/candidate` host
timefix. Their public `pkg/extension/request_policy.go` files are byte-identical
(SHA-256 `be486205f9b4cc2e37dd003e20e0602dd27470788a1c3419ac045f3f1acd1c3d`).
The timefix changes the refusal owner's two observed timestamps to UTC.
These sources were read only. Current delta verification and exact source
identity are recorded separately from accepted v4 in `REQUEST-POLICY.md`.

Update donor hashes only after comparing these wire contracts and replaying
their native regression families. Old donor test outcomes are not evidence for
the Go port. No vendor credentials, donor TLS test key, user home or live app
configuration is included in the distribution.
