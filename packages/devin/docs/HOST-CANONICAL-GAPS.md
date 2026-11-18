# Canonical histories changed before the strict module boundary

Date: 2026-11-14. All four defects below now have actual ordinary
installed-module RED-to-GREEN proof on scoped source
`c43421772d425ffc60705d66c186b66c40173efee1583e7f864cce65f4e198aa`,
binary `2fe733387f1f2ad54bfde1583b1b1d26216796cc706908f6decd5182eaffaeba`.
The generic correction projects caller identity from trusted host context
and preserves SDK Chat history before legacy repair. Non-SDK repair,
same-owner private metadata and account/origin/journal fences remain tested.
[Scoped receipts and handoff](history/DELIVERY-EVIDENCE.md#current-canonical-correction-actual-red-to-green)
record the unchanged module libraries, exact RED baseline and controls.
That captured dirty source is historical; final upstream union acceptance
is separate. The rest of this page preserves those four pre-fix defects
and proposals. Later API9 no-op/config and Gemini Messages-shaping REDs
are separately preserved in `CACHE-KEEPALIVE-EVIDENCE.md` and
`GEMINI-SIGNATURE-EVIDENCE.md`. Current committedd5b098d8/tree11c2,
using parent binaryc018 built from production-identical tree8fe, passes
duplicate/no-op/held read, real transitions, all four genuine Gemini
cases, paired24 and three strict installed hot-CAS paid repetitions.
Original467 no-paid, legacy sampler and c018 held38 timing REDs remain
separate; the held timing cause is still unknown. Exact source/build
distinctions and current receipts are in `DELIVERY-EVIDENCE.md`.

## Historical replays on captured API7/API8 unions

All three independent history reds below reproduce with production
paired0.5.0 on source4c17/binary42f77 and final API8 source277f/binarya08f.
Same-alias/simple, image and Claude
controls pass independently. On those captures ordinary Messages returns403 with
zero native dispatch because the module checks the missing host caller
before acceptance. `DELIVERY-EVIDENCE.md` records exact libraries, source
inventories and historical receipts. Those bytes are not attributed to a later
shared commit.

## Earlier exact ordinary-host evidence

The domain fixture uses exported committed host
`7fbda87dc281e807e121a185760406363d100494`, binary SHA256
`e0195145a9816a33a2a81bc1d0f4ccd0957d5e39ba9eea2d46fbec0c338f55d1`.
The production domain library is SHA256
`97e6ed2a85d275d3b7e144da80ca17a0616cfa93d74d37414483c5f9b515ef1b`.
Only the own Devin bundle is installed. All peers are isolated loopback
mocks with zero external proxy requests and no real credentials.

Evidence root: `usbox:/tmp/magpie-devin-delivery.4GORlGks`.
`donor_host_e2e.py --case CASE` isolates each request in a fresh process.
Fixture retries are zero; the historical host may retry an unaccepted
provider error internally, which is a separate observation.

| Independent input | Actual result | Receipt |
| --- | --- | --- |
| Two assistant tool-call fragments use different valid reasoning aliases | Host merges them into one assistant retaining `reasoning: First a.` and `reasoning_content: Then b.`; module correctly refuses the new conflict before vendor | `split-trace` diagnostic, `domain-split-calls-and-reasoning-alias/result.json` |
| Same-alias split calls, real result only for call a | 200, one vendor dispatch; native call b receives host text `[The result of this tool call is unavailable: the turn was interrupted.]` | `independent-missing-real-result-refused/result.json`, SHA256 `8e7473f3f4ce3bc3b69309bfed49766fb2f6ae9b8e2be1ef2c31f24a93706b88` |
| Same-alias split calls, different assistant cache marks, both real results | 200, one vendor dispatch; two assistant prompts became one marked native prompt | `independent-split-cache-boundary-refused/result.json`, SHA256 `66048553fcf2125c0f5ad51e1a129d1f9d1b468b25abce36d2a99c29af47a7ae` |

The last two fixtures deliberately remove the reasoning-alias confounder.
They are actual reds, not successful refusal coverage. Direct codec tests
still reject incomplete results and unrepresentable split cache marks.
The module cannot distinguish manufactured upstream text from real
caller-provided text once the host has replaced the input.

The original frozen module rendered an unaccepted local validation error
as 502. The pre-cancellation diagnostic recorded five module execute
entries, zero native arrivals, and the host waiting in
`gateway.pauseUntilReaddressed`. The module status fix renders this
specific error as 400/not-started. It eliminates the misleading retryable
response; it does not repair the already altered history.

## Historical generic seams and proposed regressions

In the reviewed host, `internal/gateway/gateway.go` passes Chat input
through `toolPairsOut` before SDK provider execution.
`internal/gateway/chattidy.go:toolPairsOut` calls
`pairToolMessages(joinSplitChatCalls(msgs))`.
`joinSplitChatCalls` merges content/reasoning_content and call lists,
retains another reasoning alias from the first message and does not
preserve distinct cache boundaries. `pairToolMessages` supplies the
interrupted-result text.

The smallest contract choice is to preserve the canonical request's
caller history for a strict SDK executor before provider-specific legacy
repair. Reuse existing routing/admission; no vendor import, new plugin,
signature bypass, retry, or new native account operation is needed.
If the common path must normalize first, its contract needs explicit
lossless rules: resolve equal aliases per source message before merging,
reject conflicting aliases, preserve cache boundaries, and retain actual
call/result presence. Silent tool-result repair cannot represent strict
native provenance.

Suggested generic regressions at the SDK execution boundary:

1. Mixed equal-per-message aliases across split assistant calls arrive
   without manufacturing a within-message conflict.
2. Missing real tool results remain missing and can be refused before
   acceptance/vendor; no interrupted-result text is inserted.
3. Distinct assistant cache marks remain separate or produce an explicit
   unsupported/not-started result.
4. Existing paired histories and ordinary non-SDK providers keep their
   documented behavior. Include a same-alias paired positive control.

The independent API7/API8 replays above confirm these seams on those exact
captures. Replay a different final committed runtime separately;
do not attribute the tested binary to current shared HEAD.

## Historical caller omission from ordinary canonical execution

An independent ordinary `/v1/messages` request on the same committed host,
under an enabled fixture gateway key, arrives as protocol `anthropic`,
POST `/v1/messages`, with `ExecuteOptions.CallerID` empty. The diagnostic
wrapper records the boolean presence and synthetic request body only; it
does not log account auth. The request reaches the native peer once, then
the module refuses origin persistence because it cannot bind the returned
signature to a host-derived caller. The host returns 409 uncertain and
correctly stops automatic replay.

Receipts: `messages-installed/result.json` and
`messages-trace-installed/result.json`. Production diagnostic-input library
SHA256 `4f3af760362e8fb595722704c316b2cf134ff9f6555937b9e284cc8700c3d3a8`;
instrumented library SHA256
`99a158e4ba2319b313e15349dd8f1177e92859d9bcae787f3f7134a4f501be46`.
The SDK peer test supplies an explicit host caller and succeeds, including
same-caller signed continuation and foreign caller/model/generation refusal.
It did not prove the installed canonical route succeeded on that host.

Exact generic seam: `internal/provider/extension_transport.go` fills
`ExecuteOptions.CallerID` only within
`metadata.Plugin == scope.Plugin`. Ordinary canonical requests have no
extension-owned inbound metadata, although the Front already established
`extension.CallerIDFrom(ctx)`. Native paired input has matching metadata
and is not affected.

Minimal proposal: initialize `ExecuteOptions.CallerID` from that existing
host context for every SDK provider execution. Retain the same-owner
restriction for private native metadata/request identities and every
account/generation/journal guard. Do not derive caller identity from a
client body/header/session or invent a module-local fallback.

Generic regression: two ordinary canonical requests under the same
enabled key receive the same nonempty caller identity; another key differs;
client attempts to supply private metadata/caller fields do not override it.
Include paired-native, loopback owner and accepted-journal negative
controls. This is an existing DTO projection, not a new vendor contract.

The module refuses Messages with a missing host caller before
acceptance/vendor. Its test was behaviorally red with one actual request
and becomes green without weakening origin checks. A later installed red
must remain red until the common projection is fixed; header/session
substitution is not an acceptable module workaround.
