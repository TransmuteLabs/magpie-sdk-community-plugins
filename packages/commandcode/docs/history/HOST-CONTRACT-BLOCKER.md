# Translated native response completion

## Scoped blocker resolved; host integration remains separate

The merged base is commit `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`,
physical tree `94ec4c8de4e5b366a6e3fb854df2176007587ab8`, supplied at
`usbox:/tmp/magpie-upstream-signing-closure.ofRrrBdz/source`. The base still
needs the separately frozen generic completion correction. No new SDK or
vendor capability is needed.

The author-owned unstaged correction links successful explicit protocol
completion to the original SDK response body through private request context.
Close then permits at most one SDK maximum chunk, one overflow probe byte and
one shared two-second tail/receipt grace. Early caller close never drains.
Concurrent closes and caller cancellation join the same owner. Completed
ExecuteResult receipts retain authority after client protocol rejection;
missing receipts cannot become synthetic completion. Excessive tail and
expiry retain uncertainty even when a completed receipt races the join.
The two seconds are documented host policy matching the existing process
worker cancellation grace; they do not bound a native worker ignoring
cancellation indefinitely.

On the exact merged base, `closure94-native` clean-installs the actual
one-package native distribution, then passes installed Chat/Responses/Messages
whole and stream plus search 20 times, SystemOne/Decisions 20 times, agent
lifecycle 20 times and the ordinary owner probe. That earlier overlay used
`json.Valid`; it proved successful replies but left a decoder-rejection gap.

Independent review found that syntax validation accepts
`{"choices":"invalid"}` even though the actual typed decoder rejects it.
The unmodified event before a valid terminal and a terminal with wrong-type
fields both reproduce an incorrect stream drain in
`closure94-typed-guard-red`: 25 PASS/3 FAIL including the parent.
The corrected decoder state is sticky across later valid events and comes
from the actual Chat, Responses, Anthropic and Gemini/Code Assist decoders.
It preserves the existing treatment of unknown events rather than adding
a competing protocol parser.

The final correction input is
`usbox:/tmp/magpie-commandcode-completion.QL5FOqka/closure94-typed-input2/overlay`,
with patch SHA256
`8f699c58724a8612c87089775ca85bebf27477a34c94c423bee52e665f5d9440`.
Its first scoped check has 49 PASS and no failures under `-race`, plus
fmt/vet/build. Final race20 has 980 PASS, zero FAIL/SKIP and fmt/vet/build.
Independent Astra review found no remaining HIGH/MEDIUM in the receipt slice:
66 unchanged probe test events pass on the equivalent proposal1, and input2
has a separate source-equivalence review.

The final combined installed replay, `closure94-typed-native`, passes on this
exact typed overlay and the corrected F1 module package together: clean actual
binary install/list/inspect; installed Chat/Responses/Messages whole/stream
and search race20, SystemOne/Decisions race20 and agent lifecycle race20 each
20 PASS, zero FAIL/SKIP. The ordinary owner also passes completed and truncated
Provider API SSE for all three protocols. Its receipt SHA256 is
`e7fa4a6a5213c9d2da01d5b74c6d904ed8df48b2f368d66cd7c895e6d4c13847`.
The host binary SHA256 is
`8b1a102fae32aca85e54b75f478bd6585efe70d3a33900f7c6ed4c1fce041c4f`;
the module library SHA256 is
`9d7f706ea0e7851c8054bad30459836d857952ee02a6fe0dc7c3bf2c7aea6074`.
Scoped compatibility now permits the module Lore commit. Host changes remain
unstaged and uncommitted; merging the overlay belongs to the leader.

## Original causal evidence, retained

The new installed Go-plan replay reaches `/alpha/generate` through ordinary
Chat, Responses and Messages client routes, without a vendor request or host
source edit. Module-only execution returns one completed answer with additive
pause usage. The pinned host still loses the final SDK receipt on translated
response paths.

On physical host tree `8fe78ab9df13d733d61462eacce19bff523c1e58`, the
installed native replay at
`usbox:/tmp/magpie-commandcode-completion.QL5FOqka/native3/` compiled and ran
20 times, each failing at the Responses ledger assertion after the Chat case
passed. The client had received status 200 with both continuation segments and
the tool call. Its one ledger row instead recorded status 409 and
`extension operation uncertain; automatic replay stopped`, input 13, cached
input 4 and output 6. The module's completed SDK totals were input 17, cached
input 4 and output 6 in that earlier package. Source review then corrected
the module receipt's inclusive input to the SDK's uncached convention:
input 13 plus cached input 4, matching Chat's inclusive total 17.
The earlier input-17 ledger assertion is superseded; it does not establish
accounting loss. The client-200/ledger-409 discrepancy still establishes
the host completion race independently of that fixture mistake.
The failure is a native installed-package regression, not a compile error.

`internal/provider/extension_transport.go` is byte-identical in that tested
physical tree and the current local host source checked at
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`:

```
SHA256 6714f5108143e6d27cbfa58c38593f3abeedee9bdfc3e249813bf9741db18844
```

`extensionResponseBody.Close` cancels its call before joining `body.done`,
and marks the attempt uncertain when `finished` has not yet been stored.
A successful protocol decoder can stop at the JSON/SSE terminal before
the native worker's final `ExecuteResult` has been received. The transport
records usage and completion only after that result. Closing at the decoder's
terminal therefore cancels the very receipt needed to establish completion.

The smallest host need is to finish receiving the SDK completion/usage result
when a successful translated response has consumed its protocol terminal,
before closing cancels that operation. Preserve bounded reads, real caller
cancellation, late-error handling and uncertainty for incomplete work.
This requires fixing the existing generic completion/close boundary; there
is no evidence that a new vendor-specific SDK capability is necessary.
Do not suppress the uncertainty guard or fabricate a completed receipt.

The plugin cannot fix host receipt ownership by guessing another vendor wire,
installing a companion or changing event timing. Its canonical Chat model
declaration already selects the host's existing generic protocol converters.

The reusable reproducer is `verify/native_go_plan_test.go.txt`, supplied as a
Go overlay by `verify/replay_native.py --only go-protocols`. It writes no host
file. The accepted completion overlay on the final merged host pin must replay this fixture with the actual
standalone package and require all client routes, one completed operation and
one authoritative additive usage row to agree. Earlier host receipts remain
historical; scoped overlay proof does not claim that the base contains the fix.
