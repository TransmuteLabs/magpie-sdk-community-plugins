# Devin domain completion, 2026-11-15

Authority is the current community donor ref and the recovered official
wire/model sources, compared with the previous source lock. Donor checkouts
and recovered artifacts remain read-only. Host code is independently owned.

1. Preserve current behavior in new synthetic regressions before production
   edits. Exercise the public codec/provider entry points, account-scoped
   model discovery, real outbound UID selection, API9 quota fields and cache
   boundaries. Run the unchanged production source with these tests on usbox
   and retain named assertion failures.
2. Extend `codec/outbound_models.go` and a focused catalog helper to project
   discovered families, effort levels, fast tiers and source-defined aliases.
   Resolve virtual selections only from the selected account's current
   catalog; preserve concrete UIDs and origin/operation identities.
3. Extend provider execution and the Chat/Messages controls only as required
   for source-proven model selection. Keep accepted-write, credential,
   generation, diagnostics, usage and native-origin fences intact.
4. Extend `outbound/status.go` and provider usage with present numeric
   amounts, units, spans and plan-end resets. Emit API9 fields only from the
   API9 manifest. Never infer an overage currency.
5. Complete representable prompt cache marks using the existing codec.
   Native PromptCacheOptions has only an enum; ChatToolDefinition has no
   cache field. Keep unsupported TTL/tool-cache shapes explicit rather than
   discard them or move their boundaries.
6. Refresh `source-lock.json`, `SOURCES.md`, `DONOR-MATRIX.md` and evidence
   with exact refs, hashes, date, results and remaining bounded limitations.
   Replay the ordinary installed package on the leader's supplied physical
   candidate with its exact binary hash. Keep candidate-tree proof separate
   from the final successor commit and parent whole-host acceptance.

Verification uses only usbox, Go 1.26.8, pinned offline caches, isolated
HOME/XDG, synthetic loopback networking, GOMAXPROCS=2 and `-p=2`. Run focused
regressions first, then the bounded module suite, race/static checks and
build/provenance verification sequentially. No local Mac builds or tests.

Cleanup is limited to this lane's temporary exports, overlay manifests,
synthetic servers and child processes. Close peers/servers through test
cleanup even on failures; retain immutable source snapshots and RED/GREEN
logs for review. Do not remove donor/host work, caches, target-warden, or
other lanes' temporary directories. Commit only reviewed synthetic fixtures,
source, documentation and provenance; no credentials or private captures.
