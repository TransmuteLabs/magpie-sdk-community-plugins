# Provider comparison and bounded completion

The 2026-11-15 request permits runtime changes, superseding the earlier
relocation-only restriction. Scope remains this package, public SDK only;
original donor and host are read-only. No new dependency or version bump.

## Source comparison

Donor: `magpie-community-plugins` commit
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`, `packages/devin` 0.1.41.
The complete 0.8.0 module already implements account auth, discovery, usage,
native/Chat/Messages execution, agent bootstrap, scoped signed continuation,
compression, and opt-in cache warming. The old donor matrix is evidence of
previous work, not a reason to duplicate it. This pass reads actual code and
records current comparison/check results separately.

Confirmed narrow gap: `providerOwner.usage` reads models before status and
turns every catalog error into deferred quota, including rejected credentials.
The donor's `usage` reads status first and recognizes both HTTP401 and
Connect `unauthenticated`. The later shared-policy constraint refines the
package mapping: bare HTTP401 remains a transport fact; explicit typed code
cannot be overwritten and only explicit `unauthenticated` requests sign-in.

## Implementation and verification plan

1. Add failing regressions for bare/typed authentication errors through actual
   loopback HTTP, status-before-catalog usage, catalog authentication failure,
   transient/malformed catalog negatives, and recovery with correct free scope.
2. Correct only common outbound authentication classification and usage read
   ordering/error propagation. Propagate catalog4xx; keep deferred quota when free-model scope is
   unreadable. Keep messages code-only and credential-free.
3. Prove the installed default package alone can sign in, name/separate
   identities, discover models, send compressed canonical and native requests,
   report quota, refuse rejected credentials and recover. Use a clean temporary
   host home, local fixture vendor and unavailable companion runtime.
4. Run package tests/race/vet/format/build on regular guarded usbox with pinned
   offline Go caches and frozen source4 SDK. Preserve old failing receipts.
5. Update current entry/provider comparison/provenance docs, retaining original
   historical receipts. Record exact coverage and host contract limits.

The identity comparison also finds the donor's `planInfo.teamName` and
`userStatus.teamName` display fallbacks absent. Add a failing auth-flow test,
then preserve that fallback order and suppress a repeated organization label.
Only labels change: selected server/token fingerprints remain account IDs;
usage does not mutate account metadata. Tier display formatting and automatic
browser callback handling remain documented adapter differences.

The later requested CLIProxy test comparison reads only commit842cedec8,
not its unresolved working tree. Its field28 Token Usage fixture reproduces
successful native completion with `Usage=nil` in this package. Port that
fallback using the existing bounded wire parser: exact metric names,
finite integral counts, sparse presence and existing field7 snapshot
priority. Preserve partial measured facts on a later failure; incomplete
measurements must not justify another paid cache task. Add old-failing
decoder/SDK-fact regressions and invalid/unknown/priority controls before
changing production. Do not import payload coercions, prompt fingerprints,
placeholder turns, donor tolerance of empty success or auth retry.

Do not port schema weakening, prompt replacement, fabricated tool results,
unscoped signatures, implicit credential-file reads or retained-credential
timers. The host's ordinary SDK usage projection currently exposes errors,
not the legacy donor's `signIn: expired/renewed` UI field; no host fix or new
public contract is part of this package change.

## Independent-review correction (v2)

Gauss reproduced a successful paid cache response with absent usage or only
unknown field28 metrics scheduling another paid tick. The v1 partial-usage
guard skipped nil usage. Preserve v1 source/distribution/receipts, promote the
ordinary SDK-task probe and run it failing before changing production. Use
the existing nil-safe `Measured` predicate for both absent and partial usage;
keep complete measured-hit and measured-miss behavior. Re-run the package
gates/build/ordinary-host fixture on a separate v2 snapshot and hand its exact
digest and distribution to the independent reviewer. No host or shared-policy
DTO change is included.
