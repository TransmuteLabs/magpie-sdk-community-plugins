# Devin inbound extraction plan

This is the historical version 0.1.0 extraction at `d6841b1`.
`CONTINUATION.md` records the later version 0.2.0 work.

Scope after relocation: this standalone repository's domain files. The old
`magpie-extensions/modules/devin` copy, donor and recovered applications stay
read-only; host SDK and root dependency changes belong to the leader.

1. Copy the donor's eleven `plugins/protocols/devin` files to `codec/`
   without behavior changes. Preserve its MIT notice and file digests.
   Keep only its reviewed synthetic fixtures, never captured request bytes
   or account/configuration material.
2. Add a root module constructor with API 1.1, native ABI 1 and four inbound
   descriptors. Reuse `Module.Inbounds`; do not implement C/RPC plumbing,
   authentication, outbound subscription execution or invented vendor RPCs.
3. Add `cmd/native` registration in `init`, a platform-specific package
   builder, and SDK-pipe tests comparing synthetic Connect/protobuf output
   with the same codec called directly.
4. Build/test only on `usbox` in a private temporary workspace. Its sibling
   `magpie` symlink points to immutable SDK
   `7e2a72b6178eb2a51c4e6018ee9515fdbb321da7`. Pin Go 1.26.8 and caches;
   isolate HOME/XDG and forbid dependency/vendor network access.
5. Prove a real native install and fixture roundtrip without a Devin account.
   Report artifact hashes, exact checks and implementation limits separately
   from live vendor parity.

The initial positive subset has no identified install blocker. The immutable
base loses codec HTTP error statuses as `invalid_params`/400; the preserved
regression is red there and green with the status owner's source overlay.
See `EVIDENCE.md` for exact source hashes and actual native-install results.
The SDK's 16 MiB request limit still includes the Connect envelope, unlike
the donor's 16 MiB protobuf-data limit. Do not weaken module refusals to
conceal either boundary.

Native nonzero `max_newlines`/`top_k`, signed thinking history, enum-only
model selection, unsupported planner/control fields and encrypted reasoning
remain refused. UI, media completeness, sign-in and native session controls
are outside this first extraction.
