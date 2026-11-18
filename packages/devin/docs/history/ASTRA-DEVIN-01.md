# Deferred-image residual user prompts

The November16 independent review of exact source
282ffa6e220f51932b8243aba3bf7236f63949477b4c03d9f9636c3c7500a90b
found a missing combination in the .44 whitespace behavior. The initial
normalization intentionally defers mixed image/text validation until real
tool-image association. After that association, residual empty/whitespace
text was still emitted as a user prompt. This omission also existed before
the .44 port; it is not a new retry or origin-authority defect.

`omitEmptyOutboundUser` now owns the existing whitespace/cache rule for both
normalization and residual content after association. No pipeline reorder,
text trimming, image reownership, signature bypass, continuation or retry was
introduced. Marked whitespace residue fails explicitly; remaining meaningful
user text and real user images are preserved.

The unchanged independent TestAstraDevinDeferredImageResidue probe has SHA256
51524e30e2c0fadc3c6e4ebafc6bab90115edb4a57643577cfe93b5cdd5f6b69.
Its baseline execution has four causal codec/provider failures for whitespace/
empty residue; its four no-residue/meaningful controls pass. Provider fixtures
complete with one accepted event and one actual loopback HTTP send before the
wire assertion. The same probe passes with this correction.

The repository regression TestDonor044DeferredImageResidueKeepsUserOwnership
adds whitespace, empty, meaningful words, remaining user image, whitespace
cache and empty cache cases. Its causal baseline fails four cases; all pass
with the correction. Focused race20 passes, as do narrow donor, Messages,
Chat/Gemini origin, acceptance/refusal controls, gofmt and vet.

Evidence is retained separately in /tmp/magpie-devin-astra01-fix-20261116.
Verification uses the explicitly authorized Mac path, cached Go1.26.8,
isolated HOME/XDG/TMPDIR, offline modules and the parent's inherited network/
real-config/Keychain/application guard. No live account/vendor, native loader,
broad host suite or TestScriptFindsClaudeMoved ran. This is author terminal
green; independent Astra successor review remains separate.

The optional independent adjacent-thought origin probe stopped while issuing
its initial fixture receipt. It remains an unresolved fixture-stage result;
this correction neither changes that fixture nor claims it as green.
The original donor packet, bounded 1729055→92503f96 comparison, source-lock,
implementation pins and all earlier red receipts remain intact.
