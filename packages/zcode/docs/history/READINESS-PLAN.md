# Bounded readiness continuation

2026-11-14. Baseline module `6cb7873`, host `f4232ea76`. Work is confined
to this module. No host edit, live account, vendor request or app change.

1. Compare archived controller/inference tests and domain contracts with
   current behavior. Record missing SQLite import/live resume/automatic
   readers separately from implemented offline conversion.
2. Preserve historical proof. Inspect app logs using sanitized aggregate
   counts only; log occurrence is not proof of a new module defect.
3. Lock failed-account-list/read/write invalidation with regressions.
   Repair the existing control state without changing financial admission.
4. Exercise `build.py --ui` against current SDK. Fix browser dependency
   packaging at the public entry point, preserving default API-1.4 output.
5. Add one fixed-provider polling auth binding to the optional UI variant
   and consume the accepted SDK workflow. Keep credentials/continuations
   private, require explicit selected generation for reauthorization,
   expose manual poll/cancel, and never replay a lost accepted exchange.
6. Run old-code regressions first, then fresh module race, TS/DOM/IIFE,
   native packaging/ABI and synthetic native-host gates on usbox.
   Freeze source/artifact identities and commit coherent local slices.

Runtime startup remains pure. Vendor mutations retain acceptance,
principal journals, usage barriers, uncertainty and cancellation fences.
