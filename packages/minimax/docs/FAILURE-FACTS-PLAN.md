# MiniMax failure fact preservation delta

This is a separate change after the accepted v4 source and distribution. The
v4 packet at `/tmp/magpie-minimax-core-20261115` and its remote distribution
remain immutable. Source donor stays community MiniMax 0.2.9 at
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`.

1. Trace chat, selected JSON, connector envelopes, search, media and transfers
   before their current status/body/error normalization. Add bounded original
   facts without changing successful wire bytes, usage, acceptance or replay.
2. Preserve wire status separately from business status/code, code validity,
   envelope origin, type/message and header timing knownness. Unknown or
   malformed facts never become auth, quota or no-effect evidence. Signed
   transfer endpoints are not trusted account endpoints.
3. Carry facts through error wrappers and nested connector errors. Protect
   credential redaction and existing uncertain outcomes after paid acceptance.
4. Use the final public 1.12 DTO only after its owner supplies a stable contract.
   Reuse its matcher and scheduling owner. Do not add a MiniMax scheduler,
   refresh loop, generic text timing parser or unsupported vendor fields.
5. Prove loss on v4 with test-only overlays, then replay the same regressions on
   this delta on regular usbox with isolated HOME/XDG, pinned offline caches and
   the descendant network guard. Run package race/vet/format after the narrow
   regressions pass. Record exact SDK/source hashes and unimplemented public
   integration separately from internal extraction readiness.

Timing evidence currently supports real HTTP Retry-After (seconds or HTTP
date). MiniMax usage window end_time/weekly_end_time are usage observations;
device interval/slow_down are authorization workflow state. Neither becomes an
inference delay without matching source authority. No vendor message duration
grammar is claimed from another provider's examples.
