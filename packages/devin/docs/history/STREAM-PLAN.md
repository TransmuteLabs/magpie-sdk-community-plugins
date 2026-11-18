# Native stream completion and refusal

Baseline: clean module bd6b1616bb1bb1fc5e7bba1d50f6182375fed68c.
Host: immutable SDK 1.6 commit 9c4189089fa5d25656e87e72df174027d1bace56.
Only this module changes. Source6 union and broad host checks belong to the
leader.

The paired raw reader currently accepts a successful Connect terminal
without content or a valid native stop, accepts changed stops and content
after a stop, and can issue origin receipts for those invalid replies.
Its projection also does not reject unknown response controls.

1. Add synthetic regression cases before changing runtime code. Prove real
   assertion failures on the baseline on usbox. Cover empty/usage-only,
   incomplete/error/unknown stops, contradictory stops, late content,
   unknown response/usage controls and cancellation during a blocked read.
2. Reuse the generated response schema for raw and canonical validation.
   Preserve supported native-only bytes and correct the generated fixed64
   wire type of committed_acu_cost (response field 22). Known native stop
   reasons remain distinct; failures never become canonical stop success.
3. Require content and a supported final stop before emitting a successful
   terminal, so origin persistence cannot bless a failed stream. Keep vendor
   error terminals, cancellation identity, acceptance and no-replay.
4. Extend the existing paired fixture with malformed local vendor streams
   and a signed-history replay check. Run actual bootstrap/agent and paired
   inbound/outbound through the pinned ordinary host, with just our bundle.
5. Update the source lock, watchpoints, current guide and evidence. Keep
   historical hashes and reconstruction/original-client distinctions.

All builds/tests run only through ssh usbox with Go 1.26.8, isolated
HOME/XDG and pinned existing offline caches. The user's later instruction
released the initial shared-runner GOMAXPROCS=2/-p=2 cap. Checks stay scoped
to the module and its actual-host fixtures; no broad host/union run, vendor
request, credential access, new dependency, other-plugin install or push.
