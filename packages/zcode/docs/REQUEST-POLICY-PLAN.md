# API 1.12 request policy adoption

SOL authors only `packages/zcode`; Astra reviews independently. Host and donor
trees stay read-only. This plan precedes cleanup of transport retry ownership.

1. Freeze current package bytes and the supplied host source receipt. Read the
   existing vendor 3.18.2 / CLI 0.17.1 refusal and reset hint contracts locally.
2. Add old-red regressions through the existing SDK wire harness: host denial
   must prevent local busy retries, complete whole-execution refusals must
   retain actual facts without accepted/output publication, and ambiguous,
   partial, transport and output-bearing replies must never acquire evidence.
3. Replace only ordinary paid/Start transport timers with public
   `RequestWithPolicy` / `DecideRequest`. Keep queue admission/deadlines,
   Off-Peak polling, gift applicability, reset/claim/card journals and uncertain
   financial operations with their current owners. Whole-execution evidence
   must be established separately from classification, before publishing a
   final `RefusedExecution`; acceptance alone is not an eligibility switch.
4. Align both runtime manifests and current source-lock SDK floors. Preserve
   provider/controller/UI, account reauthorization, library/MCP, session and
   native surfaces. Keep historical source pins as historical evidence.
5. Run bounded regression, race, vet, browser/typecheck and native build gates
   only on regular usbox, private HOME/XDG, pinned offline Go 1.26.8, under the
   existing descendant network guard. Retain exact source/artifact/receipt
   hashes. Exercise installed public host routing, waits and no-replay fences
   with synthetic fixtures; coordinate final combined host replay with parent
   because permit validation and revoked diagnostics have separate owners.

The supplied host is
`usbox:/tmp/magpie-obs-policy-fix-20261115/candidate-01`, manifest SHA256
`e2ca42c6fa8529b781a61aec04862355a5b28183646cc57ee028f6133fb74c34`.
Its base commit is not its physical source identity. No new selector,
scheduler, private validator, dependency, account or application change is
authorized.
