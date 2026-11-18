# Gemini thought-signature oracle

Current committed-host replay is in `DELIVERY-EVIDENCE.md`: physical
host46756a48/binary32fbdeab with full library017ebccc passes all four
genuine Chat/Messages JSON/SSE cases. Exact signature/type/tool
continuation survives without legacy thinking-disabled injection.
The earlier400/zero-dispatch RED and causal probe below remain historical
evidence of their own frozen host. No signature-origin fallback or host
guard relaxation is introduced by the API9 compatibility patch.

Read-only community update:
`191d94bf6ed95ccc4bc62d77e1dd9201a17f5918`, Devin0.1.41.
The inspected checkout remains26ea8faf; origin/HEAD was
`a60667b57040f62080da60f6018cc7351fa1953f`. No checkout, installation,
dependency fetch or donor mutation was performed.

| Exact donor input | SHA256 |
| --- | --- |
| `packages/devin/index.mjs` | `4cc31bf5291b169f72aaf2275207737060dbe34e6be6c4f9e7924704e275c0b2` |
| `packages/devin/gemini-thought.test.mjs` | `0afa95259fe50a59cd190781563b898aae76b8a1c895bb5df17f039d8aa1a44c` |
| `packages/devin/package.json` | `f912821599babd5610959751e3d16b3e09f446b95bc94528a2a6b11a73bb0e4b` |
| `packages/devin/README.md` | `1d0080c83be51ac1bcc0f6d6b13c8574225e38b944eec63b918b2b0d2c0f1f11` |
| Actual1451-byte `testdata/gemini-tool-reply.bin` | `e4ff6e3b5134fe64f3d9761a98d3ddd1af0945ea6f7946fdfe01a9389ebd4162` |

The donor-captured Hydrogen reply was reviewed after decompression. It
contains an opaque bot/response identity, usage, tool `exec` with real call
ID `call_11646388125050983982` and `{"command":"cat calc.py"}`, no visible
thinking and no account credential. Its368-byte text signature begins
`AY89a1+u1EAPOiO7CmhB`; its actual type is explicitly `gemini`. The complete
fixture is kept byte-for-byte, not regenerated from this implementation.
The successful native stop is2 with a tool call, which maps to canonical
tool_calls/tool_use while preserving native stop2 in execution Facts.

Recovered source97a4883 independently establishes response fields10/21
and actual-model23 in module432, and prompt signature/type/generation-model
fields12/18/22 in module39768. Current devinapp HEADdd6e1fa was inspected
read-only; these descriptor file hashes remain identical to the locked
97a4883 versions. This is wire evidence, not an original-client test.

## Causal module regression

Tests-only RED:
`usbox:/tmp/magpie-devin-gemini-red.DsPJcKNb`,
module inventory
`b96fff2eb9684d9cba18e67cc87df9ba503720d2d9bc75fcc0e35d8c2742aeb7`.
All four ordinary `TestGeminiDonorOracleToolRoundtrip` cases fail:
Chat buffered/streamed continuation sends no signature, and Messages
buffered/streamed rejects the genuine signature without a Claude thinking
block. Log SHA256:
`fe1ab5120cb140d8e6fd3363c5b851e4288857b59c6463453a0e6238878326da`.
These are assertions against the real donor reply, not synthetic mutants,
compile failures, retries or vendor requests.

The first complete port checkpoint was
`usbox:/tmp/magpie-devin-gemini-gates.42DiCEAZ`,
local input/results `dist/2026-11-14-gemini-green-r3`.

| Frozen input | SHA256 |
| --- | --- |
| Complete module source inventory | `f93d40aa7f0a4d0f9f5731644f4d37a1efc938c0b838396098a311dbb72a4f73` |
| Module archive | `96fa97d3d52fb0eec6fe9ea1163248386fa0e155a25569e036967e946d73a522` |
| Public SDK9 export, not a whole runtime | `dc4d6772b33459b59e4058bbdc8b3586547efe7c1ff896f173e1f58b9bc64e9b` |

Both inputs have sourceCommit:null; module base is7f8fde6 and SDK base
ae957e5. Complete before/after byte/mode inventories match for every gate.
This checkpoint preceded the additional strict origin guard described
below; it is historical rather than the final production Go identity.

Eight gates exit0: empty gofmt-l, cache SDK-peer tests, Gemini root+codec
oracle tests, root/codec/outbound cache+Gemini race/count3, full module
tests, vet, build, and eight provenance/distribution tests. Gemini oracle
log SHA256 is
`05432abf0f25de1197237cf88d6e5e7733b1123cf19a77b71d0955755b67ef17`;
race log SHA256 is
`703fba5396d7d5cfc28640683479912c2541ae49816a573f0004aec9ac55e8d1`.
All commands used usbox Go1.26.8, pinned offline caches and isolated bounded
HOME/XDG/TMPDIR. Every sandbox home was removed.

The port stores exact signatures/type/actual generation model, including
signatures without thinking. Ordinary Chat reasoning_details can roundtrip
only through a scoped origin resolver. Messages tool_use remains ordinary
and does not invent a thinking block. Invisible continuations match exact
reply text/thinking, native call IDs, names and canonical JSON arguments,
under account/generation/caller/model/conversation keys, then verify the
existing persisted origin receipt. No type is guessed. Native actual-model
field23 wins; when it is absent, the producing request's selected UID is
recorded, as in the donor fixture.

Negative controls cover explicit foreign caller/generation/model/signature/
type before acceptance; invisible state cannot cross account/caller/
generation/model, edited arguments/text or restart. Text-only and split-tool
continuations retain exact origin, and whitespace-edited reply text cannot
reuse the signature. Capacity is256 entries and8MiB, using the existing
bounded conversation index; shutdown clears state and refuses late puts.
A held canceled genuine reply joins native HTTP and issues no origin or
retained thought. Existing non-Gemini and keepAlive regressions pass.

## Final combined module and original donor tests

A later ordinary regression caught an actual Chat origin write for a reply
whose signature had no type. That RED is preserved at
`usbox:/tmp/magpie-devin-gemini-untyped.Od4KlCfa`, source inventory
`698d4e99e584dd01a06db6b58745996835b187a067ce28a7ee0b4460496a52d9`,
log SHA256
`c5d185ed17deb9b4242618c9d1d0f65d15aca5816fe025fa46b58ce8aa876aa1`.
Validating the complete actual thought before persisting any origin closes
it. Both Chat and Messages now refuse missing/malformed type with zero
origin writes. A model UID longer than512 bytes and a selected credential
echo also fail before rendering or retention.

Final production Go gates:
`usbox:/tmp/magpie-devin-release-gates.bdnnBSq1`.
Local inputs/results: `dist/2026-11-14-release-input`.

| Frozen final input | SHA256 |
| --- | --- |
| Complete module source inventory | `dac520a0eb59fc7774cd6a540baea22ecc81c93c096a868f52340e42c453a88c` |
| Module archive | `a705c866f86090c522e9e2832fe5a37b5240fb944b551bdaf25ebbaede5216c0` |
| Public SDK9 export, not a complete runtime | `999f9b2f2a2575d89c1a1a6dae7d7f284f950f9f89fa9e05450fdafb182d0816` |
| Unmodified donor git archive | `f5e6c41bf5b081d5cba55978b295d85ac0a1c8e642fb2acb611ceade0ce30969` |

Both module and SDK captures have sourceCommit:null, with bases7f8fde6
and ae957 respectively. All nine gates exit0: empty gofmt-l, cache SDK
peers, Gemini root/codec including all four genuine oracle cases,
cache/Gemini race/count3 across root/codec/outbound, full own module tests,
vet, build, nine Python provenance/distribution cases, and the donor's
unmodified7/7 `gemini-thought.test.mjs` tests.

| Final log | SHA256 |
| --- | --- |
| Module Gemini oracle | `4807cd1a1d048a60618f70136cbf607c9d03217a71f7cd9a1a32a2d5be07fd56` |
| Cache/Gemini race3 | `82f996861d243f02b5a57e41adb4e41b3712de3b6e4a761dcf9453f2ce3fc9f2` |
| Full own module tests | `4563850fa63676d75be937035bab9540dd578ddb9a901dffc1ce5c3e9cb2afb7` |
| Original donor tests | `910961a39ce73d2778dc860ca67bfe32ffe99173aaa40c132d0e9c46218918cd` |

The donor ran with existing Bun1.4.3, binary SHA256
`7ce7d6b654eddeec20afddf2396317fba6dc19ab52788a27e894e1806bf8f24b`,
without installing it or dependencies. A preload guard refuses external
fetch/net, and the test terminal explicitly reports zero external
connection attempts. Go gates use pinned offline Go1.26.8 and per-command
isolated HOME/XDG/TMPDIR. Before/after inventories match; owned processes
join and sandbox homes are removed. Later documentation/provenance
reconciliation changes no tested production Go.

The first port iteration had a compile error from assuming an absent
ExecuteOptions.Session field; it is not behavioral evidence. The next
iteration's immutability regression caught slice append modifying caller
payload; cloning the matched prompt fixed it. Both intermediate packets
remain preserved. They do not replace the genuine baseline RED.

## Acceptance boundaries

`gemini_host_e2e.py` supplies the same unmodified donor reply through one
installed full package and checks actual Chat/Messages streaming/buffered
tools and next-request signature/type/model. It retains strict ordinary
checks and a protocol selector for independent cases.

Final standard library
`464489d54f5982f310821ebefb26704deb65f24ca20fe74161f75f28be1d6910`
passes actual installed Chat JSON and SSE roundtrips at
`usbox:/tmp/magpie-devin-release-install.cyABtIA0/installed-gemini-chat`.
Log SHA256:
`687e4c7b4a59bf45c70ed7255893003f2c62e273482d76f62b655246d79f06da`.
The complete host source isca3c6c8a, binary6d4ee4ed as pinned in
`CACHE-KEEPALIVE-EVIDENCE.md`; its public export1480770a is distinct from
the newer999f9b2f export used for the module-only gates above.

The complete installed four-case oracle has an actual Messages RED on
that frozen host:
`usbox:/tmp/magpie-devin-full-package.yApZGKVS/installed-gemini-oracle`,
library `5f756bba97521d0969e14044b9be0c765b4a130f9d9499d71aa328433b68403a`
before the additional type guard. Chat buffered/SSE passed; the first
Messages request returned400 with zero native dispatch, and streamed
Messages was not reached. Original log SHA256:
`5c17510e43767f5e6c762721b6883c1289fc5c038652302e2965f6a1989676ea`.
This RED is not replaced by the successful peer Messages cases or the
installed Claude Messages fixture.

A separately inventoried test-only module entry wrapper delegates all
production handlers unchanged and records only fixed diagnostic codes.
At actual SDK entry it observes exactly one extra unsupported field:
`"thinking":{"type":"disabled"}`, omitted by the caller. The host's
Anthropic branch in `internal/gateway/gateway.go` calls
`thinkingOffUnlessAsked` for this non-Claude model before SDK execution.

| Causal probe at `usbox:/tmp/magpie-devin-probe.giidVFmF` | SHA256 |
| --- | --- |
| Original entry source | `6afc9bd127897b87f2864ef35b6897e2dacba412792d1f616c0618f6f797a343` |
| Test-only delegating entry | `2d6bb4e6dd1ed869c61d64e98d7480e5ee79c54886511acaac18960ec7168fc7` |
| Derived module inventory | `dc4416fa4722c4e81f083f8412e40ae4752c034be51f9174ed6520edd969ec28` |
| Derived library | `94e8ff34fd2f5793bbba84a192ee532dfe814b1b0515eeeb928cd695bb63529a` |
| Actual installed probe log | `4771e36c9fe3dac96ef3a72eba170f7fd3ca1e95bf047f9ffb987a425883705b` |

The sole diagnostic is
`fixture.messages.only_unknown_thinking_disabled`, count1; the request
still returns400 with zero native/external requests. No raw body, error,
signature or credential is logged. No host source is edited. Parent's
bounded generic fix seam is caller-body preservation for SDK Anthropic
execution before legacy thinking/tool/cache shaping, with trusted caller,
model selection, same-owner metadata and non-SDK shaping controls retained.
The module does not accept or silently discard unsupported thinking
controls to hide this host mutation. Final combined-host replay is pending.

Compact final receipts and the preserved installed RED/probe are mirrored
in `dist/2026-11-14-release-compact-evidence.tar.gz`, SHA256
`67d80918302190bcfcb65c18d2bfa5a0e3dc0f57cd935cafcb689f7cff5c8101`.

KeepAlive current installed results and the actual generic no-op/config
REDs are in `CACHE-KEEPALIVE-EVIDENCE.md`. No shared host code is changed
by this port. The donor itself did not prove the reported empty-response
cause; neither do these local tests. There is no new retry, live account,
original Cognition client, server-TTL, billing or broad-host acceptance
claim. Invisible state after restart/eviction/unknown history is unsigned;
explicit unissued signed history is refused. Native opaque/redacted
thoughts still require the paired native route.
