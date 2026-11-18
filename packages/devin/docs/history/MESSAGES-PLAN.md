# Canonical Anthropic Messages domain port

Use read-only community 26ea8faf, especially cache-marks,
thinking-signature and reasoning-history tests, plus generated native
descriptors pinned in source-lock.json. No legacy plugin installation,
private host import, runtime companion or vendor behavior in core.

1. Add codec/messages.go with strict canonical Messages parsing. Keep
   system/user/text/tool-result block order and per-block ephemeral cache
   marks as distinct native prompts. Do not flatten a growing history into
   one text string or invent missing tool results.
2. Translate only supported ordinary tools, inline images, tool choice and
   generation controls. Unsupported budgets/server tools/schema constraints
   fail explicitly. Preserve native metadata/session allowlists.
3. For assistant thinking, unsigned text is model-specific; signed blocks
   require original signature-type evidence and account-generation/caller/
   selected-model origin receipts. Imported signatures never get a free
   native replay. Multiple thought/text/call groups keep their ordering.
4. Register Anthropic only on the provider paths which implement it. The
   default paired native bridge and canonical Chat behavior remain intact.
5. Add a bounded incremental Messages response renderer for text/thinking/
   signature/tool deltas and normalized cache usage. Reject empty/invalid
   streams before a successful response where possible; preserve native
   refusal and cancellation outcomes.
6. Run realistic donor-shaped regressions and ordinary native-host HTTP
   proofs on usbox, alongside paired live-generation and only-own-bundle
   recovered startup. Update provenance watch/tests and exact evidence.

Implementation reuse: extract only the existing request envelope completion
from codec/outbound_request.go. Chat retains complete native validation.
Messages validates the same envelope with signatures removed from its
validation copy, then adds the receipt-resolved signature/type to the
original prompts. Keep every block boundary and validate real call/result
associations before acceptance. Outbound Messages reuses the existing
Connect request/response transport; only the canonical encoder and renderer
are new. The provider test is behaviorally red on 2026-11-14:
messages-red.log SHA256
936f09bc2d415b3210a1cc95b6058c6744f49707825559714beed66fd0a27984.
