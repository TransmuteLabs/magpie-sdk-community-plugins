# Runner readiness, not a product acceptance

The runner is implemented in tests/host_e2e.py. Production execution remains
pending the core owner's frozen distribution. No MiniMax production E2E pass
is claimed in this checkpoint.

On regular usbox, syntax-v2 passed. Fixture-v1 and fixture-v2 passed under the
existing external syscall network guard with private HOME. The smoke checks
exercise private-CA TLS through CONNECT, exact credential-free video bytes,
refusal of an unlisted CONNECT destination, and fixture thread/connection
cleanup. Their receipts and guard events are preserved beside this note.
The probe that deliberately attempts an external connection is the guard's
own self-check; it is refused.

Core subsequently confirmed reserved/documentation IP ranges are rejected by
production media URL validation. The fixture therefore uses 93.184.216.34
as an allowlisted literal and IP certificate SAN. This is only a CONNECT
authority; the fixture never opens a connection to that address. Fixture-v2
validates this revision. Successful synthetic SSE ends with a complete
message_stop frame and blank line, matching the completion contract.

Scenarios and limits are in PLAN.md. Final host execution, usage assertions,
selected tool admission, and the complete production endpoint shapes remain
unverified until the frozen package run. Any specific leader-supplied parity
cases will be recorded separately rather than implied by these smoke checks.
