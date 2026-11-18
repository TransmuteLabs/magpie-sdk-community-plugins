# Verify existing domain ports through the installed host

Reuse the frozen d070 host and unchanged paired 0.3.1 library. This is
module-only evidence after the identity diagnosis, not a host fix or
Devin bootstrap acceptance.

1. Add `donor_host_e2e.py`, reusing the existing isolated gateway,
   selected-account auth and deny-proxy helpers. Install one native bundle.
2. Send donor-shaped canonical Chat histories through ordinary HTTP:
   split assistant calls/reasoning, real tool results, tool-result images
   and user-owned images. Inspect the actual local native peer's payload.
3. Check Claude unsigned-thinking suppression and explicit refusal of
   conflicting reasoning, missing results, cache-boundary conflicts and
   incomplete tool-image association. Record all independent cases in one
   bounded run, with zero automatic retries.
4. Enrich only the synthetic catalog with native float32 field32 prices
   and inspect the installed public SDK projection, keeping explicit Free
   separate from unknown prices.
5. Preserve unexpected results, update `DONOR-MATRIX.md` with the actual
   proof boundary, and pin the new regression in the source watchpoints.

No runtime Go changes, new dependency, donor installation, host edit,
branch/worktree, credential import or live vendor access. Run only on
usbox with isolated HOME/XDG/TMPDIR, existing offline caches and timeouts.
