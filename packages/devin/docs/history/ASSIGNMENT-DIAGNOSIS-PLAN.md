# Bound native assignment identity before changing behavior

Use only the existing d070 host binary and agent library from the frozen
usbox union snapshot. The host repository and exported host sources stay
read-only during the leader's freeze.

1. `assignment_identity_e2e.py`: install the unchanged agent distribution
   into a new isolated HOME with one local native mock. Capture identical
   native request bytes, actual installed `inbound.decode` outputs,
   sanitized binding snapshots, native write journal scopes and mock
   dispatches. Read only this fixture's private files.
2. Send a first assignment and deliberate duplicate before any explicit
   sync. Trigger catalog sync through an ordinary model-display-name
   update, with no selected model/account/package/config change. Send the
   same assignment once more. No automatic retries or quiet-period wait.
3. Reconstruct the host's exact write identity from its declared inputs.
   Compare RequestID/Session, caller, provider/account/generation and
   binding/package/config before attributing the duplicate to revision.
   Preserve failed and unexpected results rather than rerunning for green.
   Hold one native team-settings mock reply across another catalog sync.
   Release the original reply and record the host's post-dispatch binding
   fence, with no retry of the read.
4. `codec/account_test.go`: lock stable assignment identity across repeated
   decode and informational credential metadata changes; changed assignment
   content or cascade must change the request identity.
5. Record exact commands, artifact hashes, event order, source seams and a
   generic host fix proposal in `ASSIGNMENT-DIAGNOSIS.md`. Keep d070
   acceptance red and distinguish a controlled reproduction from the
   previously unobserved natural interleaving.

Build/test only over ssh usbox with existing offline caches, bounded
timeouts and HOME/XDG/TMPDIR isolation. No host patch, branch, worktree,
vendor traffic, real credential, dependency or guard relaxation.

## Real-transition negative control

Before any shared host fix, add `assignment_transition_e2e.py` using the
same installed d070 agent library and the existing snapshot/local-peer
helpers. Keep the no-op receipts above unchanged.

1. Select `fixture-model` through ordinary CLI, send one assignment and
   verify its same-revision deliberate duplicate cannot dispatch again.
2. Hold one GetCliTeamSettings reply at the peer. Select the actual
   `fixture-free` model through ordinary CLI, record the owned model and
   revision change, then release the original read. Its result must be
   refused by the current binding fence.
3. Send a separate old-model assignment with a new cascade while the
   other model is selected. Verify refusal before any vendor arrival;
   distinguish this module payload guard from the post-reply host Check.
4. Reselect `fixture-model`, admit a new assignment/cascade and hold its
   accepted reply. Select `fixture-free`, release the original reply and
   verify refusal with exactly one dispatch of that logical write.
   Preserve the accepted/completed journal instead of retrying it.
5. Record request/decode identity, full sanitized binding/journal snapshots,
   event order and artifact hashes. Reconcile the actual negative-control
   result into the diagnosis and source watchpoints; keep bootstrap
   acceptance red and leave all shared host production files untouched.
