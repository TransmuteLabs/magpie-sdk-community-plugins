# Preserve authority on a proven controller no-op

Owner: common host leader. This is a testable proposed generic diff, not
authorization for this module executor to edit shared core.

The leader subsequently implemented the scoped candidate.
`NOOP-ACCEPTANCE.md` records its actual installed one-module identity and
unchanged real-transition passes, with the original module library and a
behavioral red on d070. Final upstream/SDK 1.7 replay is still required.

## Confirmed base and cause

Installed evidence uses d07078c7926daf6f33af09c40297c941bd39da92, with
exact receipts in `ASSIGNMENT-DIAGNOSIS.md`. The current committed local
devzeus checkpoint read for this handoff is
7fbda87dc281e807e121a185760406363d100494. Its controller blob is identical
to d070: `1c1efd2c334b61dbcc11430c35000285828d3c7e`.
This does not claim that local checkpoint is the latest upstream or that
the shared SDK 1.7 work in progress was tested.

An ordinary model display-name command produces a zero-file sync with
unchanged complete binding except revision. That revision enters the
native write identity and the post-call binding fence. The same assignment
gets a second journal key; a held bootstrap read is falsely revoked.
RequestID, Session, account/generation/caller and the old completed journal
remain stable. Successful apply holds the ledger lock; transient pending
visibility is not needed to explain either failure.

The installed real-model negative control already proves stale payload
refusal before dispatch and held read/accepted-write reply refusal after
selection changes. The accepted write completes under its original scope
with one dispatch despite the outward 403. Retain those guarantees.

## Production boundary

Change `internal/agent/extension_controller.go`, in
`extensionController.apply`, after the existing locked root/CAS/runtime/
file/shell checks and before publishing a pending record.

Take a no-op path only when all of these hold:

- `plan.intent == "sync"`; prior phase is owned and recovery is safe.
- Package revision, config version, descriptor, bin and binding values
  match the current plan, and the binding remains connected.
- For the narrow first diff, the action declares zero file changes and
  all currently owned applied snapshots still match bytes, modes and
  target identity. A nonempty but already-current file plan can stay on
  the existing conservative path.
- Planned shell targets and resulting bodies equal the owned targets and
  bodies; none is created, removed, retargeted or changed. Compare their
  effects, not a fresh plan's Before text against the original adoption
  backup. Preserve the original Before/Created recovery information.
- A custom `applyShell` effect is not silently skipped; conservatively
  exclude it unless the controller can establish it has no effect.

Execute the existing checked empty ActionPlan with its preview revision.
Require its Applied result and retain cancellation, authority, root,
action-lock, CAS and one-use validation. Recheck current runtime,
owned-file and shell authority under the existing locks, then return
without publishing pending, rewriting the ledger or advancing revision.

Keep real set/connect/disconnect/re-adoption, package/config changes,
file/shell effects and recovery on their current transition path.
Do not edit the native-account write identity, its revision input,
`ProtocolAgentBinding.Check`, store transitions or accepted-write retry
policy. No vendor conditional, SDK capability or journal migration is
required for this narrow fix.

## Required regression diff

Extend the existing controller fixture in
`internal/agent/extension_controller_test.go`:

1. An owned zero-file sync with unchanged shell effects leaves the complete
   durable binding and original recovery backups byte-identical. Repeating
   the same sync has the same result.
2. The checked action is consumed once; stale concurrent plans, canceled
   context, changed config/package/root, file drift, shell drift and custom
   shell effects cannot take an unchecked shortcut.
3. Real model change, reconnect/disconnect and real file/shell changes
   still advance or revoke authority. Existing rollback, incomplete pending
   recovery and cross-module ownership tests retain their assertions.

Add a generic native-operation integration regression at the existing
gateway/agent seam:

1. Complete one accepted write; trigger ordinary catalog display-name sync;
   submit its intentional duplicate. Require stable binding and operation
   identity, one vendor dispatch, and the original completed journal.
2. Hold a read at a local peer; perform that no-op sync; release the
   original reply. Require its successful result, without a retry.
3. Hold another read and a new accepted write across a real model change.
   Require refusal of their original replies and one dispatch per logical
   write, retaining the journal. Separately reject a newly submitted
   old-model payload before vendor dispatch.
4. Cover genuine config/package/account-generation/ownership revocation
   independently; a real transition must never be classified as no-op.

Module fixtures `assignment_identity_e2e.py` and
`assignment_transition_e2e.py` provide the ordinary CLI/local-peer inputs.
The former intentionally asserts the old defect and needs a separately
named post-fix acceptance expectation; preserve its historical raw results.
The latter must keep passing unchanged. Run scoped host regressions first,
then let the leader own SDK-alone and broad union acceptance.
