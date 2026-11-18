# Native outbound continuation

The stable inbound/discovery distribution is version 0.2.0 at `0afdd76`.
Keep its manifest, constructor, native entry point and existing fixtures
unchanged while recovering the opposite direction.

1. Add bidirectional domain files under `codec/`: canonical Chat to native
   protobuf, bounded Connect response decoding and native model discovery.
   Reuse the existing protobuf primitives; do not copy CLIProxyAPI dependencies,
   prompt fingerprints, role coercion or synthetic missing tool results.
2. Add `outbound/` for an injectable stdlib HTTP client, explicit credentials,
   PKCE code exchange, GetUserStatus, model enumeration and streaming requests.
   No implicit CLI execution, live credential-file reading or environment
   account switching. A selected account is explicit.
3. Use generated descriptors for fields and installed community plugin
   behavior for endpoint/header/auth evidence. Keep actual fields and zero
   token counts rather than the TS decoder's truncation/estimated usage.
   Key/session-token auth has no recovered rotating refresh endpoint; do not
   invent one. PKCE replies supply only the fields their descriptor declares.
4. Prove pure codecs and HTTP behavior with synthetic/offline fixtures on
   usbox, including gzip, fragmentation, truncation, error terminals,
   cancellation, redirects, secret isolation, models and quota semantics.
5. Bind the verified client to SDK provider/auth/models/usage handlers only
   after selected-account scope and acceptance ownership are covered.
   Preserve the old inbound constructor and manifest separately. Require
   SDK 1.4's declared executor policy for the paired native path.
6. Register SDK handlers and canonical renderer, persist bounded one-use
   PKCE state and hash-only account/caller/model origin receipts. Forward raw
   native frames through a bounded same-generation bridge, then prove the
   entire path with an ordinary compiled host and actual native library.

Native-only signed history requires more than a JSON metadata field: native
decode, actual executor, account/model origin validation and response codec
must share the correct generation. A route/fallback must never silently use
only a canonical projection after losing the module-private state.
