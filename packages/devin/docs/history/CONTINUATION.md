# Native protocol continuation

The original extraction remains recorded by `donor-files.json`. Changes after
that extraction use generated descriptors from the read-only
`Agents/devinapp/gui-src/windsurf-extension-unpacked` checkout.

1. Extend catalog adapters, manifest, SDK-pipe and C ABI fixtures together for
   `GetCascadeModelConfigs` and the three `*ForSite` variants. Match each
   request's own field layout. Discard credential/client-information fields;
   refuse nonempty vendor team selection, which the host cannot resolve.
   Continue projecting only the caller-authorized Models operation.
2. Extend prompt decoding for leading `SYSTEM_PROMPT` messages (source 5).
   Reject misplaced system prompts because Chat translation collects them
   before the conversation. Accept bounded advisory token counts and telemetry
   flags without using them as routing or authorization authority. Validate
   known inactive proto3 fields; retain explicit refusals for active state the
   Chat contract cannot preserve.
3. Validate singular Connect headers consistently on chat and discovery.
   Preserve existing framing, credential isolation, deadlines and safe errors.
4. Carry native tool-result images through Chat's existing tool-message
   image parts, retaining call association and original data. Accept an empty
   native caption default and refuse active captions without an established
   mapping. The existing host `toolOutput` path preserves tool-message images.
5. Lock additions with synthetic generated-schema fixtures. Run red checks on
   the original implementation, then normal/race/vet/native build and C ABI
   conformance on `usbox`, with pinned Go 1.26.8 and isolated HOME/XDG.

Signed thinking, native tool-result error flags, custom/computer-use tools,
nonzero top-k/max-newlines, assignment JWTs and session-control operations must
remain refused unless an end-to-end host contract preserves their semantics.
No vendor accounts, captured credentials or reconstructed account artifacts
are test inputs.

## Full HTTP proof

Add `http_e2e.py` as a standalone stdlib runner. Build the real host from
frozen merge commit `01bcc7c7d155b029769327f909372b2ff72252e2` on `usbox`,
record source/tree/archive and executable hashes, and use the actual native
distribution. Install/configure only under temporary HOME/XDG. Run the
ordinary CLI gateway against a local synthetic HTTP provider through
Server.Front, with native requests over TCP. Prove guards, caller scope,
framing, tool/image content, safe credential handling, malformed input,
upstream cancellation, deadlines and module disable/re-enable.

Preserve the failed first fixture run: the requested HTTP scenarios passed,
but incomplete background isolation triggered denied catalog/partner
requests. Seed the supported lab cache and disable partner discovery, then
repeat the complete run in fresh sandboxes. Keep the deny-proxy assertion.
