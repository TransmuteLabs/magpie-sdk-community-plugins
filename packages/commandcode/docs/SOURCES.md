# CommandCode source provenance

The November 18 reference comparison checks `command-code` `1.91.0`
against `1.89.2`. Both npm archives match their published SHA-1 and
SHA-512 integrity. The entry point `dist/index.mjs` is byte-identical.
In `dist/cli.mjs`, the native request builder, stream consumer and
thinking-effort check differ in bundled identifiers. The corresponding
endpoint, continuation limit, usage defaults, ZDR header, token default
and effort values match. The selected wire helpers, stream-error reader,
stop-reason normalizer and usage accumulator are byte-identical.
The intervening changelog entries cover CLI presentation and `/goal`.

Native generation and System One use the same `1.91.0` header value.
`referenceReview20261118` records archive and file digests, comparison
scope and local HTTP checks. The `official-commandcode-harness` extraction
pin below identifies the earlier `1.89.1` artifact used by the historical
isolated behavior probe.

The November 17 reference comparison checks community
`324acd6007f2d513c5072fe2e9fe1db3524ae03d` and the published
`command-code` `1.89.2` archive. The complete community package tree remains
unchanged. The new CLI archive passes published integrity checks; its
`dist/index.mjs` and 22 selected native/auth/usage/wire functions match the
original `1.89.1` bytes. Native channel streaming and MCP client updates do
not require a new provider codec or agent configuration format.
This is bounded static evidence; REA's graph remains partial and the
complete CLI was not executed.

At that checkpoint, native generation and System One adopted the shared
`cliVersion` value `1.89.2`. The former separate System One `1.88.2`
literal is removed. Ordinary local HTTP tests first failed with the old
headers, then passed twenty race-enabled repetitions. The complete module
race run passed with 225 pass events and two command packages without test
files; vet and formatting passed. This proves the actual sent fixture
headers, not vendor acceptance of an installed CLI.
`referenceReview20261117` retains the new archive identity and these checks
without overwriting the extraction or earlier acceptance records.

The [November16 closure](LOCAL-CLOSURE-20261116.md) supersedes the original
verification checkpoint below. `currentVerification` in the source lock names
the strict public helper used for the common-host closure and confirmed ZDR
adoption. The original OBS/extraction identities remain historical evidence.

Current unpublished request-policy adoption requires public API1.12. Package
version0.1.3 and the original extraction pins below are unchanged.
[REQUEST-POLICY.md](REQUEST-POLICY.md) records rejection proof, acceptance fences
and excluded recovery flows; `requestPolicyAdoption` in the lock records the
exact original OBS archive/manifest and stable public helper. The mutable shared
candidate directory had one changed transport file, so verification uses a
private extraction of the original hash-verified archive with no host patch.
The new packet is `usbox:/tmp/magpie-sol-two-policy-20261116`; earlier SDK1.9
acceptance identities below remain historical. `REQUEST-POLICY-VERIFICATION.json`
separates current checks and limits from Astra review and SDK publication.

The November16 committed-donor review checks community `origin/main`
`b35d4b35b2446bd929896ddeb58f3cb299e9e2d1`: CommandCode0.1.24 still has
tree `305e867f7f675041d4905c951fb2495a9d00fcb3`, identical to both previous
review and extraction. Parent's November16 npm check still resolves
`command-code/latest` to1.89.1 with the same shasum and integrity. No runtime
or extraction-pin change is justified. `DONOR-REVIEW-20261116.json` records
the comparison and distinguishes parent-supplied npm evidence from SOL's
local Git reads; no fetch, checkout or repeated harness download occurred.

`source-lock.json` is packaged with the module and separates the module version,
the vendor evidence and the tested SDK contract. No branch name substitutes for
an immutable source revision.

The primary donor is the MIT-licensed
`magpie-community/plugins/packages/commandcode` at
`26ea8faf7793ca66dd8e7a8b90128a5235552a1f`, package 0.1.24. The JSON model
catalog, zeroed billing examples and full Go model metadata table in `testdata`
come from that revision. The Go table was obtained by evaluating only its
exported static `_internal.GO_MODELS` on usbox; no vendor request was made.
The new Go decoders are implemented against the observed wire schemas.
The freshly fetched community `origin/main`,
`57d1cc6a9f49389d495ea9be8d986c70de7767bf`, has the identical CommandCode
package tree `305e867f7f675041d4905c951fb2495a9d00fcb3`. There is no package
diff from the pinned donor. Its actual Google-schema and effort-fitting
implementation and tests inform the current conversion; the normalizer
explicitly refuses cycles instead of the donor's lossy type-only fallback.

The maintained CLIProxyAPI checkout is
`../CLIProxyAPI-family/CLIProxyAPI-u2-331t-up42`, whose committed `main` is
`842cedec8f85c808f45bb2c7b66a1ee5dd23bcf9`. Its unresolved merge, staged,
unstaged and untracked work were preserved; only immutable committed source
was inspected. The prior source pin was `7638884b40a60c3fb534126b334b178f3b3bbcbb`.
The request/transport source is unchanged there; the newer stream reader rejects
malformed data instead of skipping it. A safe refs-only fetch found upstream
`origin/main` at `e96d4c4818851cf0e96d27b2bc8b67379e449d9e`, which no longer
contains that native CommandCode implementation. That upstream absence does
not become evidence of new native controls. No translator dependencies or
runtime package are copied into this module.

The original recovered official harness is the npm `command-code` package 1.89.1,
published November 13, 2026 at 23:07:47.521 UTC and checked November 15.
The npm latest tag resolved to that version; its immutable tarball matched
the registry SHA-1, SHA-512 integrity and the SHA-256 recorded in the lock.
This artifact has no Git revision, so provenance records an npm tarball
identity rather than inventing a commit. Current header selection follows
the separately verified `1.91.0` comparison above.
The UNLICENSED harness archive and its original functions remain outside the
plugin package and repository.

Static inspection of `createModelClient`, `consumeStream`, `toWireMessages`
and `toWireToolOutput` establishes an unchanged-body `/alpha/generate` request
loop with at most five pause continuations and additive usage. Its native
envelope sends no tool-choice or parallel control, and its native tool-result
output contains text only. The separate BYOK `toolResultOutput` media shape
does not establish native Go-plan image support. Those native controls and
tool-result images remain explicit refusals. `tests/harness_probe.mjs`
hash-checks the archive's relevant files and executes only isolated original
conversion/continuation functions with local recorders; it does not execute
the complete harness, import its dependencies or contact a vendor.
Unlike the older community transport, the current official harness's
`isThinkingEffort` includes `off`, and its native request forwards that value
when `supportsThinking` is true. The isolated probe executes those original
functions for both thinking and nonthinking cases. The module maps canonical
`none` to native `off` only for models with recovered thinking metadata;
it refuses that control for unknown or nonthinking models.

The official Provider API documentation at
`https://commandcode.ai/docs/provider` is a mutable contextual reference.
Protocol behavior in this module is grounded in the pinned donor code;
no documentation capture or official CLI binary is packaged as evidence.
The donor supports Responses, Chat, Messages and System One. Its older
README claims that Responses is absent are not implemented here.

Before request-policy adoption the module's SDK floor was API1.9/native ABI1
for effective output, effort and default metadata. The current floor is
API1.12/request-policy as recorded above; the following acceptance sources
retain their historical identities.
Selected-account `systemone` execution itself requires API 1.7. The historical
API 1.5 module's acceptance source is the unified SDK 1.6 host, commit
`d07078c7926daf6f33af09c40297c941bd39da92`, tree
`ed28dfd65eca68d15e9400b56660bbbf5e7bc319`. The previous committed API 1.9
acceptance uses consolidated `../magpie` SDK source
`46756a48c9c9f204a8369638dc63cbcb83d02031`, tree
`db40ddbf8944b54f9f689d444644af4c3bc76881`; its receipt remains unchanged.
The current `sdk-tested-host` entry pins physical source tree
`94ec4c8de4e5b366a6e3fb854df2176007587ab8` and the leader's closure packet
at `usbox:/tmp/magpie-upstream-signing-closure.ofRrrBdz/`. The completed base
merge is commit `66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`; its committed tree
now matches those physical bytes. The packet's `head_at_capture` still says
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`, because capture preceded the merge
commit. That capture head has its own different tree and is retained separately.
The 10,849-path source archive has SHA256
`131aeaa8fa9052dab47854ead8cb8b57f7f4dadca92715dbad307f4424c2de9e`.
The supplied Linux host binary has SHA256
`a8e09770909f78ae89cc6175a45bb98e1dacdb305b62856f46956928cb8ecbe2`.
`tests/check_host_snapshot.py` verifies the receipt, Git modes, full inventory,
archive, binary and independently calculated physical Git tree without a host
write. The November 14 pin, `36f8` and `4b9` candidates and every earlier
receipt remain historical evidence. The completion overlay remains a separate
unstaged host correction; the merged base does not include it.
Extraction may mask read/write permission bits; those differences are counted
separately and cannot change Git executable modes or the physical tree.
The completion slice additionally pins
`internal/provider/extension_transport.go`. Its installed translated-path
replay exposes early response-body close canceling the final SDK receipt;
the original transport source bytes match the packet and its capture head.
The user then authorized that transport, its causal regression and the three
actual translated readers: whole responses, streaming continuations and search.
All ten source/test files have separate compatibility-overlay hashes; the
immutable packet stays unchanged. An independent review found that JSON
syntax validity cannot prove typed decoding: `{"choices":"invalid"}` is
syntactically valid and is rejected by the actual Chat decoder. The corrected
overlay observes sticky rejection from the actual Chat, Responses, Anthropic
and Gemini/Code Assist decoders, preserving their tolerant event behavior.
The unchanged wrong-type event and a terminal with wrong-type fields are
causal negatives for the earlier syntax-only guard. Only an explicit terminal
with successful decoding grants the shared two-second receipt/tail grace. Its tail allowance
is one SDK maximum chunk, with a one-byte overflow probe. Early close and
caller abort without that signal never drain. A validated completed receipt
remains authoritative despite client protocol rejection; the failed request's
`Outcome(true)` still prevents replay. Excessive tail and grace expiry retain
uncertainty even when a completed receipt races cancellation. No terminal,
EOF or cancellation fabricates a receipt. Typed failures keep their policy.
The source decision and native-worker limitation live in the host's owning
`docs/subsystems/extension-providers.md`: two seconds match the existing worker
cancellation grace, but do not bound an arbitrary native worker ignoring
cancellation indefinitely. Host edits and that reference remain unstaged and
uncommitted for the leader. The rebuilt overlay binary is pinned separately
from the supplied base binary in the source lock and completion evidence.
The earlier `4b9` overlay binary was
`361fcd8b185955672280de6b5c7f8af9a6709abe58ca3583766a3ddbbadf27d0`.
Earlier `36f8` positives, the rejected unconditional-drain implementation,
wrapper-mock failures and causal negatives retain their original identities.
The overlay is a generic host correction, not a new SDK/vendor capability or
another runtime package. The final exact tree94 plus typed overlay passes
combined installed Chat/Responses/Messages, decisions and agent parity with the
corrected Provider API module. Final host integration remains pending
separately from that scoped compatibility acceptance and earlier receipts.
Provider API SSE completion uses the canonical terminal predicates in the pinned
host's `internal/gateway/upstream.go` and its SSE framing in `sse.go`. The new
observer forwards the original bytes, including malformed or truncated data,
but requires an explicit terminal before returning a completed SDK receipt.
Its framing buffers share the existing 64 MiB total response bound. EOF alone,
wrong-protocol terminals and malformed events or tails retain uncertainty.
Opaque non-streaming JSON forwarding is unchanged. The independent Astra
Provider EOF probe is preserved byte-for-byte in
`tests/astra_provider_probe_test.go.txt`; its RED remains separate from the
receipt-fix review and earlier module checkpoints.
Provider requests, auth, quotas, checked agent
configuration and page bindings use the public SDK. Jev is a separate
decision operation with the recovered vendor wire, never chat.
The current official configuration's `off` effort spelling is emitted for
canonical SDK `none`; binding inspection remains canonical. Repeated sync,
Auto and disconnect preserve ownership through that spelling conversion.
The pinned SDK accounting mapper assigns receipt input directly to the
ledger's uncached input field. The native codec retains the vendor's inclusive
input for Chat output, then subtracts cache reads and writes exactly once at
the completed SDK receipt. Late cache metadata is validated before a pause can
continue or a success terminal can be emitted.

The Jev request/answer examples in `tests/systemone_donor.mjs` come from
the pinned donor's `packages/commandcode/jev.test.mjs`. The probe hash-checks
that file and the implementation, then executes the unchanged donor with an
in-process fetch recorder. It verifies the decision route, aliases, exact
1.88.2 CLI headers, ZDR, response bytes and account-preserving refusals.
This is recovered donor wire evidence, not a live vendor or installed CLI
capture. No donor code becomes a module runtime dependency.

Each recovered fixture has a SHA-256, source path and transformation in the
lock. Three JSON fixtures are byte-identical to the pinned donor files; the
57-entry Go metadata table is a serialization of its static exported table.
`SYSTEMONE-SDK-FIXTURES.json` retains the literal Jev wire alongside locally
authored pending host/agent cases from the read-only review. Its declaration
and expected results preserve the historical proposed SDK 1.7 behavior;
those authored entries are not test execution evidence. The fixture is
hash-locked and packaged. The unchanged donor probe and current decision/
no-chat regressions consume its observed-wire section.
Additional SDK source hashes pin both decision fallback owners, typed
error/read handling, execution snapshot isolation and the shared accounting
mapper. The API 1.9 hashes additionally pin the effective option projection,
controlled inspection, previous catalog evidence, nil/empty effort wire and
Go/JS validators. The native configuration schema is grounded in the pinned
host `internal/agent/commandcode.go`, including `maxOutput`, `reasoningEfforts`
and `settings.reasoningEffort`; this is source evidence, not an observed
installed CommandCode client.

The API 1.9 decision catalog hashes pin the public Go/JS DTO, its validator
and the upstream ordinary decision-model projection. Both agent handlers
validate the catalog separately from conversation options. No vendor-specific
decision setting is inferred from that discovery contract.

Source-file hashes additionally pin the inspected transport and SDK usage
contract. `tests/check_sources.py` checks extracted source snapshots and
`tests/check_package.py` checks the actual archive, its inventory, SDK floor,
native controller and absence of another runtime package. Unit regressions
check fixture hashes, declared versions and watchpoint references. Build-time
provenance failure stops packaging. The donor license is retained in
`THIRD_PARTY_NOTICES`, including for the embedded model metadata.

The current native SDK uses `golang.org/x/sys/unix`; the module retains the
host's existing `golang.org/x/sys v0.49.0` checksums and indirect requirement
so a readonly offline build can compile the native entry. This adds no
dependency version beyond the pinned host graph. Its license is included
in `THIRD_PARTY_NOTICES`.

Verification uses local HTTP fixtures with synthetic keys and temporary state
on usbox. No live credential, real subscription, live decision request or
CommandCode client execution is inferred from those fixtures.
