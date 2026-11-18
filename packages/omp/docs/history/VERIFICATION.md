# API1.10 checked Codex lineage — 2026-11-15

Core now owns native Codex ancestor resolution and original captures only.
`codex_lineage.go` in this module validates the checked carrier and performs
all in-memory materialization before the existing Codex importer. The ordinary
full package is still version0.1.0/nativeABI1, now requiring SDK1.10 and the
named `session-related-inputs` capability. It retains all six routes and the
Source1.9 descendant reader. `session.inputs.v1` is unchanged.

Proof packet: `usbox:/tmp/magpie-codex-lineage-20261115.d0BvXEV3`.
`LEADER-CARRIER-PACKET.md` records the DTO, authority, remaining authoring
integration hooks, owned host paths and causal boundaries. No host file was
staged or committed by this lane. The prior core materializer was preserved
in `pre-carrier.tar.gz` before its removal by apply_patch; archive SHA256
`42c8daef26ce84f346d087463ec473a1fe089c2bcf29e0b912f3e2dadb704840`.

The `carrier-real4` exact physical host tree is
`9686156f09408c5bccbed6a1a9f52d87a59eec22`, archive SHA256
`621d000877cf34f4680d1e80d31c8ab2696ebe705f5cdb63b5b15bf2ee4679ae`.
It was captured with parent
`66ac2f7fdaeb208e8f4d60f53aea7b33ed396c55`, whose committed tree is
`94ec4c8de4e5b366a6e3fb854df2176007587ab8`. Its dirty physical bytes are
not attributed to that commit. No positive production overlay was used.

The module physical tree is
`5a427c3901ef0bf7326058181d2e179c4147b17b`, archive SHA256
`ded87639ac7c8843b1b0e2c5c8a7e5327c5541fc75baa5ec613a4a25f1f6bf28`,
inventory SHA256
`2a85f474f4cad9a080351ce5065b7e56d0b0e33554c266df37cea8410639aca8`.
Its parent is `7674189f8ba33bff3821ede378a0a0bb6c8c9194`; pending domain
code and documentation are not attributed to that commit. This verification
record and the lock's new acceptance entry follow that frozen capture.

| Actual physical-source gate | Observed result |
| --- | --- |
| Actions under race | Exit0;56 test/subtest passes |
| Native acquisition/carrier under race | Exit0;60 passes, one explicit installed-distribution skip |
| Complete module under race | Exit0;155 passes, no failures/skips |
| Python package/metadata suite | Exit0 |
| Owned host and all module Go formatting | Empty gofmt output |
| Module vet | Exit0 |
| Full native SDK1.10 build and installed host replay under race | Exit0;9 passes, no failures/skips |

The native installed replay covers plain and packed selected/ancestor files,
the committed official-schema synthetic pair's exact `[0,756)` prefix,
ordinary Find/CarryOver/checked reuse/transcript/native resume command and
unchanged original sources. Five refusal cases change plain/packed original
ancestor, module config, installed package generation or enablement after
preview. None writes the stale output. They run through the actual installed
six-route package, not a converter stub.

The module also tests inherited call/selected-child output pairing, native
tool-result images and call identity, selected user images and incomplete
calls without fabricated output. The existing native Codex export regressions
remain unchanged. Display projection intentionally omits tool-result media;
the new assertion checks the actual native OMP journal for those bytes.

Domain results JSON SHA256:
`95169c32feb1079d9549390ca93a63b6acbc8535d5858e50587ce4c4be945375`.
Complete module race log SHA256:
`ca45257130fd327f7249aa6b8c7f6136e02e20162d02289ddfc92afcbcb27caf`.
Build/replay results JSON SHA256:
`7860bd17400ff1c2ff5ca42a6d6280ee5b64eeac13b615a486a959430a1715ce`.
Installed replay log SHA256:
`8a6ab2198e56825f5c504c5e32602fedb84b5796c96f553c9536c5e4ce723e54`.
That package's native library SHA256:
`7e49b94895da748445a4eeff3112b62ed02faf866bc38639a8c97df68725ae48`.
Package provenance SHA256:
`09c1db48c115dccebc301d63a1459bdba2fcc690ad77042c936ef5ca2208630d`.
Later package/provenance captures are recorded separately to avoid recursive
source claims.

Causal negatives compile successfully before their assertions fail:

- `carrier-red` with the prior host implementation fails input/options
  revision binding, cancellation before publication, capability refusal and
  preservation of the selected original. Its physical tree is
  `717476d1ceeb52d0747f6f2ac5d761b8b37a4603`, archive SHA256
  `d533afa2bce515a71a1dd49156c9f3dc0afb5ac89799fe9167e3887f303532ec`.
- Exact `native_omp.go` from module
  `7674189f8ba33bff3821ede378a0a0bb6c8c9194`, SHA256
  `158a78ebc86d5c4bd7c075edb7807b342c3ed46b9b2692596d378d4dac0a8f4a`,
  substituted only for the negative, fails
  `TestCodexLineageModuleFromCapturedFixture` with the captured-ancestor
  refusal. The current fixture-positive passes. Negative log SHA256:
  `69d9523ea2079a7851158f78956afedd9378b31dfeb50c657fd2c449eca80cc3`.
- Bypassing only context-aware input revalidation, while retaining current
  ABI/types, fails four action assertions: authority cancellation publishes
  before rollback, and source mutation fails to stop later write/remove.
  Negative log SHA256:
  `d8a847d0e5bedcc37c6e9b2a5239a2abf23c993b91d2de78ec9de27e7d19b407`.

The first real3 installed replay exposed a target-check regression: deferred
validation rejected Actions' own newly created target parents. Its five
refusal cases passed but three positives failed. The action owner now records
only its created directories' physical identities, clones that map to the
native callback and preserves the strict original target snapshot.
Unowned appearance and replacement still fail; real4's9/9 replay is the
positive correction. Review found a Windows slash/native-key mismatch; the
subsequent source correction normalizes keys with `filepath.ToSlash`.
Native Windows/macOS execution is not established by Linux tests or vet.

At the real4 checkpoint, FullContract10/capability and Manifest validation
are real shared integration. Public Go `Module.prepare`/`sdk.Serve`
related-input wrapping and the external minor-version assertion remain
leader/Linnaeus hooks. `sessionRelatedHandler` and direct/Serve regressions
are provided in the assigned SDK helper/tests; the earlier unwrapped
public-handler regression gives four actual failed invalid-input subcases.
The leader owns JS/types/version parity. Full SDK authoring, final host pin
and broad host acceptance are therefore not asserted by this module record.

All commands ran only on usbox with Go1.26.8, private HOME/XDG, pinned offline
caches, `GOMAXPROCS=2`, `-p=2`, synthetic stores and the unchanged network
verifier. Positive runs use the exact physical archives; negative overlays
are identified separately. The official source remains Codex
`81b995a6c9a4c978f399775cfa5ce50702368749`, tree
`eeef8486f0e835d4f5a9eda9ebfb0f3e7367ad85`, workspace placeholder0.0.0,
without an established current installed client. No live client/session,
account, vendor request or new dependency was used. Capture digests fence
subsequent mutation, not the contents at the original historical fork.
Historical actual codex-cli0.160.0 acceptance below is unchanged.

# Previous SDK1.9 physical candidate ordinary replay — 2026-11-15

The supplied candidate is immutable physical tree
`36f8e629b1092b089d4d59e0f9d8473a1f602ff8`, captured with parent
`adcf4ce5edbe2fe9595c4fceb9e809c23f74d60d`. All 10,846 recorded source paths
matched before/after replay. Archive SHA-256:
`5f07aab7ab2a782131789197fe3c26778e33f9ee3d54e459ba63f62ef9fecdfc`.
The supplied binary at
`usbox:/tmp/magpie-upstream-signing-final.QhgGhCd7/magpie` has SHA-256
`0bd335b7b6191f64d7f4ba6d1df273928e6ade33fd7966b5245987ac6a10c509`.
The host commit is deliberately null: this does not attribute physical bytes
to the parent or a future merge, certify ongoing review, or assert host broad
completion. No host source, branch or index was changed.

Packet: `usbox:/tmp/magpie-omp-current-20261115.RKcxeFiQ`.
The frozen module contains domain commit
`f8bbec4b09a92a729caaa42a807c811e874b5f35` plus pending provenance documents.
Its 60-path physical inventory SHA-256 is
`15dc84c1f5fc6bc47844e4046968d31af0720ba0d84c8cdd2a53031a520209b1`;
archive SHA-256:
`d919dffd9c4677afab0bcd782df24554a412a46d54cac41052251854fc420643`.
That inventory is not attributed to the domain commit.

| Candidate gate | Observed result |
| --- | --- |
| Full module race suite against candidate SDK | Exit0; 138 passes, no failures/skips |
| Module vet and Python packaging/provenance suite | Exit0; nine Python tests |
| Default full native build | Exit0; all six routes and descendant Source, SDK1.9/ABI1 |
| Unchanged external Source-owner probe, compiled against candidate source | Exit0 in three fresh HOME/XDG processes; exact bind/module veto, List/Find/read/search/stats/resume, Source→Codex CarryOver, reuse, config rebind and consent/off revocation |
| Existing host native attachments/ordinary/recovered-route/descendant tests under race | Exit0; six test/subtest passes, no failures/skips; all four native carry directions |
| Supplied candidate CLI through `verify_host.py` | Exit0; checked preview/apply/reverse, unapproved blob refusal, lossless restoration and private outputs |

The external probe remains unchanged, SHA-256
`0a7b3003c00b330c43f61d425b14483070ac1ecf1f6d942090a8031fa59ad9ee`.
It is built in a separate temporary verification module, with no production
overlay or module import of host internals. Its three logs have SHA-256
`f27a605312844f246af8f136738964d5908daa99ac2e5ac3c050b7940c10b3dd`.
The full module race log SHA-256 is
`1438af0afb830c28d3f2fd6a9d7f24bed571567d8845dc6f44e89d1c0f9ce6a6`.

Commands/exits/environments/network receipts are in `candidate-results.json`
and `candidate2-results.json`. The first native-test attempt exited before any
test because the runner's sandbox marker was outside its TMPDIR. Its log is
preserved, SHA-256
`b91c2632a528fd4a5bef12891364a1a010b1ec37eb0514a4b3c7fcba01c928dd`.
The corrected runner lets the unchanged host TestMain create its private home
and inert client tools. No assertion or host guard changed. The resulting
six-pass native log SHA-256 is
`c4c066f95da77c0f36382b8658f406299de12f260dd5a273fb0ad00d7d2c372a`.
This runner failure is not a behavioral negative or a host failure.

The selected public SDK inputs and native library are byte-identical to the
prior `green3` package below; the new host binary and ordinary owners are
separate evidence. `candidate2-package-record.json` has SHA-256
`61bd1b8f4f48b75ef79fe3034980bd143736ea7eed6ad867117dc70488288eb8`.
The package documentation is updated afterward; final package receipts are
kept separately to avoid recursive provenance claims.

After that update, the final 60-path physical inventory is
`be64a062091ebc6c817bdef872baf8eae46d19a310ff4826abe66a5dc7dd902c`;
archive SHA-256:
`921385a4cea890396eec4b10edeaef3e6db0e563972318ad7880aa05585f7a15`.
`final-results.json` records exit0 for the metadata regression, default native
build, another fresh Source-owner process and the actual candidate CLI.
Its SHA-256 is
`876b0ee48ef3b9bde1a63078d0bcfe957d160ea5c73547974cc317fd484577b7`.
The corrected runner SHA-256 is
`956e889ba77ec107a5471de00deaa8a93e0119dcfcfe0d8617e2513fbae8436f`.

Final distribution: `final-dist/`, one full 0.1.0/SDK1.9/ABI1 package.
`final-package-record.json` SHA-256:
`54d3531d69ca7e120f92ed5c6952286f2903b9aebd4355116cddb05a538821fb`.
The native library remains
`c1d5d91bf133adbda4fa39f1b6c62822ccc8c38cfaae62837bb797b159af9370`;
the 123-input fingerprint is
`e9ce7a23ecce6e61c0818a19e532595fc0252f3839c4bff552677a348d238961`.
Only `SOURCES.md` and `source-lock.json` differ in those input records from
the earlier candidate package; selected public SDK and converter bytes match.
Only this unshipped verification record changes after the final capture.

All work ran only on usbox, Go1.26.8, pinned offline caches, fresh HOME/XDG,
`GOMAXPROCS=2`, `-p=2`, UID1000/initial-user-namespace native loading and the
unchanged network verifier. Source snapshots were checked and private homes
removed. Only synthetic fixture stores and inert client stand-ins were used;
no installed client, account, live session or vendor was accessed.

Sparse `history_base` still fails explicitly. `CODEX-LINEAGE.md` provides the
exact official-source fields, synthetic prefix and acquisition boundary; the
candidate's unchanged v1 contract file has SHA-256
`ad739b3574e5e7a1b0fb847b6c382b6eebde733f50fef6af708d29e4d18bdd8f`.
No new host contract is promised. Current installed-client acceptance, host
broad/final review, native macOS/Windows and unsupported queued multi-agent
metadata semantics remain outside this replay.

# Current official-source compatibility — 2026-11-15

Domain commit: `f8bbec4b09a92a729caaa42a807c811e874b5f35`. Starting clean
module commit: `c3677c9ace94befebc834671f29d27115f6e3d09`.
This checkpoint implements source-derived compatibility rather than merely
changing pins: private OMP developer/redacted records, Codex rollback and
replacement metadata boundaries, and explicit refusal of unresolved sparse
lineage. The repaired native Codex serializer and its historical acceptance
receipts remain unchanged.

| Observed source | Full revision | Declared source version |
| --- | --- | --- |
| Official OMP | `7c49c11aa5bbaf3f418406583096168ad5b8c6dc` | 18.20.2 |
| Official OpenAI Codex | `81b995a6c9a4c978f399775cfa5ce50702368749` | Placeholder 0.0.0; release/client version unknown |
| Nonofficial codex-infinity donor | `2ecc8a92fe18fd43552529856078999d3f6888c9` | 0.159.3-infinity.1; no installed-client claim |

Public refs were fetched into separate ignored bare stores. Existing donor
HEADs, indexes, working files and OMP's untracked
`packages/coding-agent/undefined/` were preserved. The previous pins, watched
path hashes and current trees are in `source-comparison.json`, SHA-256
`dfec4a56c7bd91884aea80f63eaf92701732f1e6638a7faa547407237db63467`.
The expected official Codex donor checkout was absent; its previous pin was
read from the isolated fetched tree.

Evidence root: `usbox:/tmp/magpie-omp-current-20261115.RKcxeFiQ`.
The final regression file SHA-256 is
`88a9b71516d9a5845374397c96caef15c4342a190893951d41bdd400bbc000b8`.
`red4` contains the original committed production files plus that exact
regression file and its two synthetic lineage fixtures. All five named
top-level tests fail on the old production:

- `TestCurrentOMPSourceKeepsDeveloperParentsPrivate`
- `TestCurrentOMPRedactedThinkingStaysPrivate`
- `TestCurrentCodexRollbackKeepsOnlySelectedHistory`
- `TestCurrentCodexRejectsInvalidReplayBoundaries`
- `TestCurrentCodexSparseLineageRequiresCapturedHistory`

The unchanged negative command is `go test -mod=readonly -p=2 -json -count=1
-timeout=3m -run '^TestCurrent' .`: expected exit1, 11 test/subtest runs,
10 failure events and one pass for rollback count zero. These are assertion
failures after successful compilation. `red4-current-harness.log` SHA-256 is
`8fd16d11f1a87dba578ca6a601d3a9fe52f2692f6f2025324d06fb5e8b7922c3`.
Its archive SHA-256 is
`e80fb9870f044771f326a37f1676392fa449a8c6f86d5b868136027983357595`;
source inventory SHA-256 is
`c87499f6e868ab0c2712e736857b3194c39c0e677f46c5fff49edb096b4316c7`.

`green3` contains the exact corrected physical module source, including source
watch/package documentation before this verification-only record. Its archive
SHA-256 is
`982613730b40f7a11ccc9c680309df1030e32c3caf7a2f79c17d3606fe23dc79`;
60-path source inventory SHA-256 is
`9e0faa827cc87e4679bee56ed9d64c9a73585d825c3c88c5a76e996130415212`.
That physical snapshot is not attributed to the starting base commit.

| Final gate | Terminal result |
| --- | --- |
| Full `go test -mod=readonly -p=2 -json -race -count=1 -timeout=4m ./...` | Exit0; 138 test/subtest passes, no failures/skips |
| Focused race20, `^TestCurrent\|^TestCodexExport\|^TestNativeCodex\|^TestNativeRoutes` | Exit0; 680 passes, no failures/skips |
| Linux vet; Windows and Darwin vet with CGO disabled | All exit0; cross-target static checks, not native runtime acceptance |
| `gofmt -l .` | Exit0; empty output |
| Python packaging/provenance suite | Exit0; nine tests |
| Python AST syntax parse of repository scripts | Exit0; syntax validation, not a Python type checker |
| One default native package build | Exit0; SDK1.9/ABI1, six routes plus descendant Source in the actual typed manifest |
| Existing immutable host CLI, `verify_host.py` | Exit0; checked preview/apply/reverse, unapproved blob refusal, lossless restoration, reuse grace and private outputs |

Exact commands, exits, environments, timings and log/network hashes are in
`red4-results.json` and `green3-results.json`. The full race log SHA-256 is
`925807a5d80d858ca7be8b2f8cb085ce22e3a36d10441cccea8637393864adf6`;
focused race20 log SHA-256 is
`03ed1101f88d21ddf947886857815ca4832cef56eac37af4ff1efab43cd3f579`.
Earlier negative iterations and the initial green run's formatting-only
failure remain in the packet; neither substitutes for these final passes.

All builds, tests and static checks ran on Linux usbox with fresh HOME/XDG,
Go1.26.8, pinned existing offline caches, `GOMAXPROCS=2` and `-p=2`. The unchanged
network verifier SHA-256 is
`09f2a8d3311566d21dafc87c118ef8d5e07cfba88f78dba3114e4cf124b2b57e`.
Native loading used UID1000 in the initial user namespace. Module and the
10,758 recorded host source paths matched before/after execution; temporary
HOME directories were removed. No donor checkout, shared host or live client
store was changed.

The SDK/CLI is the prior immutable physical tree
`8fe78ab9df13d733d61462eacce19bff523c1e58`, based on
`c11acbc2bebec1fb213ef562900d35188cdf55fe`, archive SHA-256
`01117ce6f6f8842d16fcd81224dad1795020b3b5c4d69b9145d562fd88171e37`.
Binary SHA-256:
`c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86`.
It is not the unresolved merged host candidate, and its physical bytes are not
attributed to its base commit.

`green3-dist` is the ordinary five-file package. Native library SHA-256:
`c1d5d91bf133adbda4fa39f1b6c62822ccc8c38cfaae62837bb797b159af9370`.
Its 123 compiled-input records have fingerprint
`c19e155dda5e426ec128391e076a5aa26fa5706b264f7da4f4993b6b46c106b8`.
The complete package/manifest/provenance inventory is
`green3-package-record.json`, SHA-256
`61bd1b8f4f48b75ef79fe3034980bd143736ea7eed6ad867117dc70488288eb8`.
At that checkpoint only the unshipped verification record followed the package
capture; the later candidate provenance has separate final receipts above.

`CODEX-LINEAGE.md` gives the exact source fields/paths, the synthetic 756-byte
ancestor prefix, digest authority and why unchanged `session.inputs.v1` cannot
resolve it. Its supplemental source/fixture evidence SHA-256 is
`f6bac0ef65fa60dc38c3246c861c8e70bc6e584526ef242fd92d1d43c3e5102b`.
The module refuses both suffix-only conversion and arbitrary second-document
lineage. Host-owned resolution/materialization is still a generic blocker.

The later physical candidate replay is recorded above; a final merged-host
claim remains outside this checkpoint. This checkpoint does not rerun a current installed
OMP, Codex or infinity client, historical Codex0.160 acceptance, the host broad
suite or native macOS/Windows execution. Queued multi-agent/model-context
metadata semantics beyond the supported portable projection remain outside the
behavioral coverage. Historical client receipts below remain distinct.

# Historical physical-host replay — 2026-11-14

The supplied binary at `usbox:/tmp/magpie-sdk-final-base-tvn9tc_o/magpie`
has SHA-256
`c018c44332ff9238779d1d01340dfd751209c2c694942fd0406e4d6a3c96cc86`.
Its immutable physical source is tree
`8fe78ab9df13d733d61462eacce19bff523c1e58`, based on
`c11acbc2bebec1fb213ef562900d35188cdf55fe`, archive SHA-256
`01117ce6f6f8842d16fcd81224dad1795020b3b5c4d69b9145d562fd88171e37`.
All 10,758 inventoried paths matched before and after execution. This records
the tested physical tree, not a future leader commit or the mutable local
checkout. No host source, index or branch was changed.

Packet: `usbox:/tmp/magpie-two-owner-repin-20261114.SK7PeIJJ`.
The starting module was clean commit
`364e7f9109eb3b97afe3c3f17a56b940f0c3253d`, inventory
`1430fbb9d7d78570ab1b70fed34330ad0375edb19c3623de95e4373b0f92c0d1`.
This update changes only the README, source record, lock and this evidence.
The module version, API1.9 floor, dependency graph and implementation stay
unchanged.

| Bounded gate | Result |
| --- | --- |
| Default native build, metadata agreement and Python packaging regressions | GREEN; one metadata test, nine Python tests, native link/build information retained |
| Unchanged external Source-owner probe, compiled against this exact source | GREEN in a fresh process; explicit binding/module veto, List/Find/read/search/stats/resume, ordinary Source→Codex CarryOver, config/rebind and consent/off revocation |
| Actual supplied CLI through `verify_host.py` | GREEN; checked preview/apply/reverse, blob refusal/reuse grace, lossless restoration and private outputs |

Exact commands, exits, timings, environment and log/network hashes are in
`omp-package-results.json` and `omp-owner-results.json`. The final rebuilt
distribution and its documentation/provenance inventory are recorded under
`final/`; its package gates and ordinary owners are recorded separately.
The unchanged owner-probe source SHA-256 remains
`0a7b3003c00b330c43f61d425b14483070ac1ecf1f6d942090a8031fa59ad9ee`.
It uses a separate temporary verification module, with no host overlay.

Runs used Linux usbox, temporary HOME/XDG, pinned existing offline caches and
Go1.26.8. The unchanged network verifier has SHA-256
`09f2a8d3311566d21dafc87c118ef8d5e07cfba88f78dba3114e4cf124b2b57e`.
UID1000 and the initial user namespace preserve visible system-library
ownership. This repeats only package and actual owner paths: the domain
race suites, descendant-specific host regressions, real Codex/OMP client
acceptance, native macOS/Windows runtime and core broad were not rerun.
The committed467 and historical vendor-client receipts below remain distinct.

# Historical committed-host replay — 2026-11-14

Current host: `46756a48c9c9f204a8369638dc63cbcb83d02031`, tree
`db40ddbf8944b54f9f689d444644af4c3bc76881`. The unchanged supplied binary at
`usbox:/tmp/magpie-sdk-final-base-eg014lri/magpie` has SHA-256
`32fbdeabd20ed3059f61a91f7614a51a2ac9b908b12dd637c204a82918a28256`.
Its source archive is
`e188abd5d115c7da3167a6296e7d07f9bd31766a738ea4e6326ff093e4a74a61`;
all 10,751 inventoried host files matched before replay. No host file changed.

Packet: `usbox:/tmp/magpie-final-two-repin-20261114.XsY2ZV5a`.
The clean initial module was `3659d78efa004cd980292fbdc55bd70832e6a2ad`,
inventory `2cebc355f2295245dc2293948e1b1e2ae35aa37c8fefddb982847d0af5af9c68`.
The runtime corrections were tested from `green/session-omp`, inventory
`4ed098a751dd8b66bd64c6f75e0ebc80ab1197a7e0f9421f0e82b8bd4a768f24`.
Those inventories precede this evidence/provenance-only update.

The first native build exposed missing checksums for the public native SDK's
existing `golang.org/x/sys v0.49.0` dependency. Its exact host-selected version
and checksums now live in `go.mod`/`go.sum`. `build.py --sdk` also copies the
checksum file alongside its temporary module override. No new module version
was selected or fetched; the SDK/API1.9 floor and native ownership checks stay
unchanged.

Before the builder correction, the unchanged
`test_alternate_sdk_build_retains_locked_dependency_checksums` failed with
`alternate SDK build omitted pinned dependency checksums`, exit1. Its
test-file SHA-256 is
`3209ccd43232e7b85d09cb8be251c3d7e82977ffdb99f127d88578619010c1b0`;
log `r3-omp-alternate-sdk-checksum-old.log` has SHA-256
`2916cb1683b52781c47be628a19214be2231a5b5e5579fc3c13fa8a3aae63931`.
The initial missing-checksum compiler failures are preserved separately in
`r2-omp-default-build.log` and `r2-omp-vet.log`; they are not behavioral
negative evidence.

| Current scoped gate | Terminal result |
| --- | --- |
| Full module `go test -mod=readonly -json -race -count=3 -timeout=4m ./...` | Exit0, 381 test/subtest passes, no failures/skips |
| `go vet -mod=readonly ./...`, `gofmt -l .` | Exit0; no unformatted files |
| Python packaging/provenance suite | Exit0, 9 tests, including the unchanged old-failing checksum regression |
| `python3 build.py OUTPUT` and `python3 build.py OUTPUT --sdk EXACT_HOST_SOURCE` | Both exit0, complete Source1.9/six-route package; matching compiled-input fingerprints |
| Unchanged native Source owner probe in 3 fresh processes | All exit0; explicit bind, independent module veto, List/Find/transcript/search/stats/resume and Source→Codex CarryOver |
| Supplied committed-host CLI `verify_host.py` | Exit0; preview/apply/reverse, unapproved blob refusal, lossless journal/blob restoration and private outputs |
| Existing host native attachment/ordinary/recovered route and Source descendant regressions, race×1 | Exit0, 6 test/subtest passes, no failures/skips |

Exact commands, environment, elapsed times, network controls and logs are in
`green/r4-omp-domain-results.json`, `green/r4-omp-owner-results.json` and
`green/r5-omp-native-results.json`. The native gate selects only
`TestSessionExtensionOMPAttachmentsNative`,
`TestSessionExtensionOMPOrdinaryCarryOverNative`,
`TestSessionExtensionOMPRecoveredNativeRoutes` and
`TestSessionSourceOMPDescendantsNative`.

The unchanged external owner probe remains SHA-256
`0a7b3003c00b330c43f61d425b14483070ac1ecf1f6d942090a8031fa59ad9ee`.
It was compiled in a separate verification module against the immutable host,
with no production overlay. Each run checks source bytes, ordinary native
Codex tool/result artifacts, reuse, config invalidation and explicit rebind,
consent/off revocation and held-object denial. The current descendant gate
additionally checks child usage/native IDs, alias refusal, main-only
transcript/search/CarryOver, changed-tree fencing and stale last-good metadata.
This supersedes the historical Source1.6 automatic-descendant limitation below.

All positives ran on Linux usbox, UID1000 in the initial user namespace with
system library ownership visible, private HOME/XDG, Go1.26.8, pinned existing
caches and offline module resolution. Network verifier SHA-256:
`09f2a8d3311566d21dafc87c118ef8d5e07cfba88f78dba3114e4cf124b2b57e`.
Early harness-only errors (missing `/usr/sbin` in PATH and an inconsistent
testenv marker/TMPDIR) remain recorded; corrected runs changed only the runner.

The package is still one ordinary full distribution with no Bun, companion,
SDK checkout or builder needed at runtime. Final package hashes are retained
in the packet rather than recursively embedded in their own provenance.
The existing real Codex0.160.0 client acceptance below was not rerun or
relabelled. No live vendor/client, native macOS/Windows runtime or core broad
suite is covered by this replay.

# Historical unified default distribution — 2026-11-14

One ordinary native package now includes all six migration routes and the
public Source 1.6 reader. `build.py` and the compatibility preview command
produce the same package from `Module()` and `cmd/native`; there is no tagged
partial entry point. The declared minimum is API 1.6 / native ABI 1.
Installation still grants no native ownership.

The original 13 dirty paths, untracked provenance regression and index bytes
were preserved before consolidation. The before archive SHA-256 is
`f1fe49d560899666a43901c8d3b0f4bf66b7f03e616d61387f23e17ce0f1da1f`.
It includes the removed preview entry point and the previous builder/tests.
Module baseline: `7aafb06e8023c056daa73f68c9ba55872e87c09f`.

Evidence root: `usbox:/tmp/magpie-omp-unified.wtWlu8bM`.
Final green records are `v2/module-results.json`, `v2/native-results.json`,
`v2/codex-acceptance/acceptance.json` and their named logs. The exact runner
commands, environment, terminal exits, source inventories and archive
identities are retained there. No shared host source was edited for this work.

The frozen SDK archive is based on
`9e88f10982a1d66c1b402fba27caddda8ac4ac4e` plus the captured SDK 1.7 dirty
working tree; its 10,472 file hashes and dirty inventory are in
`sdk-inventory.json` / `sdk-state.json`. Archive SHA-256:
`f2bf131aa1af77563299e7b20541e6a0959b144d9cf97e1384b22f889edd1087`.
This is exact snapshot acceptance, not a claim that every later shared-host
change or the host's broad suite passed.

| Gate | Observed result |
| --- | --- |
| Full module race suite, no build tag | Exit 0; 127 test pass events, no failures/skips |
| Source, unified factory, native routes and Codex artifacts, race count 20 | Exit 0; 1,380 pass events, no failures/skips |
| Same full module suite against pinned SDK 1.6 source `7fbda87d` | Exit 0; 127 pass events, no failures/skips |
| Packaging/provenance Python suite | Exit 0; 8 tests; identity regressions repeated 20 times, 80 more tests |
| Module `go vet ./...`, `gofmt -l` | Exit 0; no unformatted files |
| One ordinary native build, relocated five-file distribution | Exit 0; actual typed manifest and artifact identity match provenance |
| Existing host native attachment/ordinary/recovered-route fixtures | Exit 0 under race; 5 pass events, no failures/skips |
| Actual delegated Source owner probe | Exit 0 once, then in 20 fresh processes; every process also performs ordinary bound Source→Codex CarryOver |
| Fresh matching host CLI and relocated package | Exit 0; preview/apply/reverse, rejected unapproved output, lossless bytes and mode 0600 |
| Actual Codex 0.160.0 app-server, ordinary CarryOver export | Exit 0; read/resume/materialized read, one completed turn, two native tool cards, zero model requests/replayed tools |

The owner probe checks installation without takeover, stale revision refusal,
independent module denial under fresh exact native bindings, allowed
List/Find/transcript/search/stats/resume, unchanged source bytes, consent/off
revocation and held-object denial. Ordinary bound Source→Codex output keeps
native call/result identity, contiguous ordinals, images, completion and
selected context without fabricated tool prose or extra user turns.
An unchanged migration is reused. A changed module config invalidates the
binding; restoring identical config values does not revive it. Recovery uses
another explicit revision-checked bind.

The compiled external owner probe is
`magpie/.omp-unified-verification/main.go` in the packet, SHA-256
`0a7b3003c00b330c43f61d425b14483070ac1ecf1f6d942090a8031fa59ad9ee`.
It is verification code in an isolated host snapshot, not a module import of
host internals and not a production overlay.

The final unchanged module regression file has SHA-256
`fabcc7b2cd4bf58112b8d23b9f0adad116c3341141f397014e48335867a0434c`.
`negative-final/old-source.log` is exit 1 with named failures in
`TestOrdinaryDistributionIncludesSourceAndEveryMigration`,
`TestSourceReadsSuppliedArtifactJournalsWithoutFlatteningHistory`, and
`TestSourceArtifactUnknownUsageAndAggregateOverflow`. The unchanged package
probe, SHA-256
`a829892f52a98c14f92b6da57dd789b0d4f97a0cd30684f614d1a5b9158d424f`,
fails with the actual `^1.3 != ^1.6` package assertion. Forty old non-test
files remain byte-identical in that replay. The earlier packaging identity
negative remains preserved in the historical packet below.

The first unified run's obsolete API 1.3 test pin and the external probe's
late-created Codex fixture / incorrect automatic config-recovery assumption
are preserved as failed results. The fixture now exists before discovery;
the recovery probe requires the existing generation guard and explicit bind.
No runtime guard or assertion count was weakened.

All native positives used usbox's initial user namespace, UID 1000, identity
uid_map and the unchanged network verifier. The Codex-only read/resume check
used a separate loopback-only user/network namespace after native verification;
it did not load a native module there. Temporary HOME directories were removed.
All captured module and SDK source hashes were unchanged after execution.
Only this record and documentation wording changed after the tested archive.

The relocated package contains exactly `plugin.so`, `magpie-plugin.json`,
`source-lock.json`, `SOURCES.md`, `build-provenance.json`. It needs no checkout,
builder, Bun/npm, SDK source or companion package at runtime. Its only dynamic
dependencies are system libc/resolv, with no RPATH/RUNPATH. All 36 selected
SDK inputs match the captured source. Native library SHA-256:
`4f68482b6df6f44107de8bf6e7b116c71377457a7bdae496aa8e5eb4b158072b`.
Compiled-input fingerprint:
`d6fd0c8c28b40a3b4affb774d2cfebd9ec60abd3909caa3cc4b8c5dacc88cc48`.

Supplied related artifact JSONL is supported and tested, with descendant path
and UUID checks, per-model usage, unknown usage and aggregate overflow guards.
The Source 1.6 host still cannot enumerate an artifact directory from the
exact-path/digest planner, nor receive descendant gateway exclusion IDs in a
summary. Automatic ordinary subagent/advisor completeness is therefore not
claimed. Binary images use Source display markers; migration/interchange keeps
the actual bytes. See `SOURCE-PREVIEW.md` for the precise owner paths.
No live accounts/vendors, new model continuation, host broad suite or native
macOS/Windows/FreeBSD execution was performed. External source release still
needs a published SDK revision instead of the development replacement.

# Historical API 1.3 packaging checkpoint — 2026-11-14

This module-only checkpoint is based on `7aafb06e8023c056daa73f68c9ba55872e87c09f`.
It keeps the single supported default package at API 1.3 with all six migration
routes. The Source build remains a developer alternative at API 1.6, with the
same module identity. No converter, replay, attachment, permission or fixture
bytes changed; no additional plugin install is required.

Packaging now takes the default manifest from the registered typed factory,
records actual compiled-source/toolchain/artifact identity separately from
historical donor pins, refuses nonempty output directories, and detects source
input changes during compilation. `verify_host.py` reports the installed
contract floor, including 1.6 for the Source experiment, rather than a constant.

All execution used usbox, Go 1.26.8 linux/amd64 with CGO, private HOME/XDG,
pinned offline caches and the unchanged network verifier. Native positives
stayed in the initial user namespace, UID 1000, identity uid_map, trusted system
library owner 0. No native ownership rule, vendor access or live installation
was used. Private fixture HOME directories were removed.

Evidence: `usbox:/tmp/magpie-omp-package-closure.QGUyDXgl`.
`current-results.json` records exact commands, environment, source fingerprints,
exits and native proofs; `network-current.json` records the network guard.

| Gate | Observed result |
| --- | --- |
| `python3 -m unittest -v test_build_provenance test_build_source_preview test_build_identity` | Exit 0; 10 tests |
| New packaging regressions repeated 20 times | Exit 0; 80 tests |
| Default complete module `go test -mod=readonly -race -count=1 ./...` | Exit 0; 84 test pass events, no fail/skip |
| Source complete module suite, `-tags magpie_source16_preview -race -count=1` | Exit 0; 123 test pass events, no fail/skip |
| Source record/default-contract/provenance regressions, race count 20 | Exit 0; 460 test pass events, no fail/skip |
| Both module variants `go vet`, changed Go entrypoint formatting | Exit 0; no unformatted file |
| Both native package builds and fresh committed-base CLI build | Exit 0 |
| Both relocated packages through the actual CLI | Exit 0; checked preview/apply/reverse, lossless journal/image bytes, rejected unapproved blob output, refreshed blob grace, private outputs |
| Package build identity | Native library and manifest hashes match packaged provenance; all 31 selected SDK inputs match the pinned SDK source |

The SDK and CLI were frozen at `7fbda87dc281e807e121a185760406363d100494`,
tree `86263d8ccf0320edb81d5e70f79ddcfa692c9b06`. The SDK archive SHA-256 is
`c7f2cdb9d84fc3fb22a7cd4d99757e2dc698db389452143e992b7578d92ed985`.
After verification, all 31 selected committed SDK inputs were also identical at
`75fa28b1098539f000281385d9acfd85f7c3e99a`. That is an SDK-byte comparison,
not a fresh whole-host test at the later commit. Concurrent uncommitted SDK 1.7
changes in contract, manifest, provider operations and types were not tested.

Each relocated distribution contains exactly `plugin.so`, `magpie-plugin.json`,
`source-lock.json`, `SOURCES.md` and `build-provenance.json`. `readelf` names only
system libc/resolv and no RPATH/RUNPATH. Runtime needs no checkout, compiler,
Python builder, Bun/npm or second plugin. Native library SHA-256:

```text
default faa697ce5c4e291f5172f7a82b84cd70c69e8cd21907a60af313ec662b8a8a6d
Source  71655336b3f2511de5b5ddae14abd87ea5b8e75a9d1f35f319ff76b975a0e72a
```

The unchanged final regression probe has SHA-256
`a1a72db5f141b99cd2682413c925adc83f6400e00c07503054f339fea0ee3a21`.
Replaying it over unchanged old production source fails through seven named
behavior assertions: stale manifest selection, absent actual-build identity,
source mutation accepted, and existing package/local files overwritten.
The preserved `negative-v2/old-negative.log` is exit 1, four tests/seven
subcase failures; `negative-v2/old-negative.json` verifies unchanged baseline
production hashes. The earlier three-test probe/log remains separately intact.

This is not fresh ordinary four-route or delegated Source-owner acceptance.
The unchanged converter/probe/fixture bytes retain the historical Linux Codex
0.160.0 offline client proof below; its long client run was not repeated.
Subagent/advisor aggregation and original media are still outside the selected
journal Source contract. A published source release still needs a downloadable
SDK revision in place of the development replacement. Build from immutable
release inputs; before/after fingerprints are not a source-tree lock or a
claim of bit-identical builds across toolchains. No broad host suite or native
macOS/Windows execution was performed for this packaging checkpoint.

# Attachment acquisition verification — first commit `69bad28`

All runs below used **usbox**, Go 1.26.8 linux/amd64, isolated HOME/XDG,
pinned Go caches, offline dependencies and synthetic journal/image fixtures.
No live vendor, installed app, account or session was used.

The initial host baseline was upstream merge
`01bcc7c7d155b029769327f909372b2ff72252e2`. The final SDK 1.3 host snapshot
is Git tree `136444a85ba9752609c49e6ecbc9d1f232d8c5f4`, including the OMP
lane's four `internal/sessions/extension*` files and the other owners'
then-current SDK 1.3 code. Final logs/hashes are under
`/tmp/magpie-omp-attachments.QDKjYgI9/sdk13/completion.json` on usbox,
with a local copy at `/tmp/magpie-omp-sdk13-completion.json`. All 22 recorded
source hashes matched the checkout at that checkpoint.

| Check | Observed result |
| --- | --- |
| Complete implementation race suite before the metadata-floor bump, count 20 | Exit 0; 1,140 test/subtest pass events |
| Final API 1.3 complete plugin race suite, count 1 | Exit 0; 57 pass events |
| Final API 1.3 public contract/alias test, race count 20 | Exit 0 |
| Final API 1.3 session extension tests, race count 3 | Exit 0; 108 pass events |
| Final API 1.3 actual ordinary native CarryOver, race count 3, no runtime overlay | Exit 0; 3 pass events |
| Earlier attachment mutation/bounds checkpoint, race count 20 | Exit 0; 480 pass events |
| Plugin and Linux session-owner vet | Exit 0 |
| Windows/Darwin no-cgo session-owner vet | Exit 0; cross-compilation checks only |
| Native C-shared distribution and host nogui CLI build | Exit 0 |
| Actual native CLI preview/apply in both directions | Exit 0; journal and image bytes preserved, no preview writes, private output modes, reused blob grace refreshed |
| Older SDK 1.2 host attempts API 1.3 installation | Refused as incompatible; SDK catalog remains empty |
| Changed Go source formatting | No unformatted files |

The 20-run implementation checkpoint used the older SDK 1.2 sibling and the
earlier 1.1 manifest floor. Production plugin Go implementation bytes were
unchanged when declaring the required API 1.3 floor; the embedded manifest,
contract assertion and CLI evidence label changed. The final complete suite,
contract regression, native build and ordinary integration all use SDK 1.3.

The native session-owner fixture exercises both the normal synthetic journal
and a journal with a 4 MiB user text block. The latter creates an archived
conversation document larger than 8 MiB and reimports it losslessly. Both
include ordinary text `blob:not-an-attachment`, and image bytes are supplied by
the host source-input adapter rather than manually injected into that request.
The separate ordinary fixture follows native OMP discovery, offered Carry,
preview without writes, `CarryOver`, applied Claude JSONL, native Claude
discovery and a safe resume command. It retains both user and tool-result
images, Bash history and source bytes. Availability is injected; no vendor
CLI is started.

## Runtime budget prerequisite

The full session-owner suite's large-journal fixture and the host CLI build
used an **isolated Go overlay**, not an edit to the runtime owner's files.
The ordinary CarryOver fixture also passed independently without that overlay.
Current root `module.call`
incorrectly applies `MaxHostBytes` (about 1 MiB, intended for host-service
callbacks) to a normal root request. The large fixture fails without that
overlay with `module.call requires a declared action and JSON input`.
This failure was also reproduced on the final SDK 1.3 snapshot.

The proposed fix gives root ModuleRPC requests the normal bounded frame budget
and leaves host callbacks, state and UI component calls unchanged. The runtime
owner must integrate and verify that fix before claiming large ordinary input
planning works in the main host. Handoff:
`/tmp/magpie-omp-module-rpc-budget.patch` and
`/tmp/magpie-omp-attachments-sdk-blocker.md`.

## Behavioral negative proofs

- Before the module declared its planner action, both new planner tests failed
  with `native module cannot declare its attachment inputs`.
- Removing the host input-collection call makes
  `TestSessionExtensionCollectsAttachmentSnapshots` fail because only the
  journal reaches conversion.
- Removing the per-input source guard makes all four content/identity changes
  during conversion or before application fail their regression assertions.
  The mutations preserve size/mtime or replace the physical file with the same
  bytes.
- Removing the native conversion branch makes the native history test fail
  with an unsupported descriptor instead of producing a Claude journal.
- Forcing native conversion through an intermediate archive document makes a
  valid 7 MiB journal fail that archive's expansion budget. Direct native
  conversion admits it without weakening either format's limits.

These are executed assertion failures, not compilation failures or skipped
tests.

This does not prove native Windows/macOS execution, a complete repository
suite, live vendor application acceptance, native reverse/Codex migration, or
resume of the module-owned interchange document. Existing native readers and
resume adapters are reused for the tested OMP-to-Claude route. Custom Claude
stores fail closed. Public explicit callers retaining a prepared plan still
own deferred source validation before applying it.

## Acceptance snapshot without overlay

The unchanged `69bad28` module was rebuilt against the leader's
`usbox:/tmp/magpie-sdk13-acceptance-v1/source`. Both native fixtures, including
the archived document larger than 8 MiB, passed with `-race`, exit 0, without
an overlay. Ordinary native CarryOver also passed, exit 0. All 18 plugin
source entries in the original 22-source manifest matched; its four host
entries remain the historical host checkpoint and were not overwritten.
Evidence: `/tmp/magpie-omp-acceptance.3BQe8tS1/completion.json` on usbox.

## Native reverse and Codex follow-up

The follow-up restores all four OMP routes: OMP↔Claude and OMP↔Codex.
Claude↔Codex remains upstream-owned. It adds pure native source replay,
native Codex output and single-journal inline-image OMP imports, without
changing the explicit lossless interchange codec's blob behavior.

Final source uses the leader's acceptance snapshot plus only the four
reserved session-owner files. Verification root:
`usbox:/tmp/magpie-omp-followup-acceptance.akBcex6i`.

| Final follow-up check | Observed result |
| --- | --- |
| Complete module race suite, count 3 | Exit 0; 195 test/subtest pass events |
| Native-domain regressions, race count 20 | Exit 0; 160 pass events |
| Session-extension integration, race count 3 | Exit 0; 117 pass events |
| Native four-route integration | All four ordinary Find/preview/CarryOver/native reader/resume flows execute inside the session suite; Codex reverse input is packed `.zst` |
| Source preservation | Selected source journals, packed source and global source blob bytes unchanged |
| Real saved-format donor probe | Existing repository Claude/Codex fixtures convert into readable native OMP journals; only fixture hashes/counts are logged |
| Plugin/session vet, Windows/Darwin cross-vet, native build and native CLI preview/apply | Exit 0 |

The previous 20-run full implementation checkpoint also passed before the
small Codex user-markup correction. The final complete suite and new native
domain tests above include that correction. Literal HTML/XML user text is
preserved while known client-injected context remains filtered.
All 27 final source hashes match the local files. The complete follow-up
manifest is `/tmp/magpie-omp-native-followup-completion.json` locally and
`completion.json` in the remote verification root.

Negative overlays produce assertion failures when the three recovered route
branches are removed, native reverse images are externalized into global blobs,
or all user markup is mistaken for injected context. They are not compilation
failures. The old implementation and the source snapshots remain intact.

The fixtures exercise large inline output images without global blob writes,
native provider selection, temp-before-home bucket naming, active Claude
parent/revision replay, Codex compaction and packed input budgets. No vendor
agent is executed.

At this earlier checkpoint the remaining owner-contract limitation was dynamic destination binding:
fixed HOME-relative descriptor prefixes cannot select custom native roots,
migrated XDG destinations or an OMP target profile. These fail closed.
Supporting them requires a host-chosen native store binding, not broader
plugin filesystem authority. Explicit URL-image acquisition and production
vendor acceptance are not claimed. The SDK ordinary carry path still has
its documented create-only/no-reuse behavior rather than the archived
built-in's in-memory reuse cache.

## SDK 1.4 native stores, checked reuse and shared ownership

The final owned host implementation is recorded in full working-tree snapshot
`4c08faab4362eeb9a2b5b6bf27b5b4c414c42079`. This is a verification tree, not a
published commit. The 11 session-owner file hashes and all tested module
implementation hashes are in
`/tmp/magpie-omp-sdk14-store-completion.json` locally and
`/tmp/magpie-omp-store-shared-final.I4rYI2hu/completion-scoped.json` on usbox.
Earlier manifests and checkpoints above remain historical records.

All positive checks below use an actual SDK 1.4 source snapshot, Go 1.26.8
linux/amd64, a freshly built native C-shared module, isolated HOME/XDG,
pinned caches and offline dependencies. No positive check uses a Go runtime
overlay, live vendor, installed agent, account or session.

| Check | Observed result |
| --- | --- |
| Context, destination, source and shared-owner guards, race count 20 | Exit 0; 1,241 pass events, no skipped cases |
| All session-extension integrations in three fresh processes | Each exit 0; 96 pass events each, no skipped cases |
| Native checked reuse in 20 fresh processes | All exit 0; 280 pass events in total, no skipped cases |
| Complete module race suite, count 3 | Exit 0; 196 test/package pass events; `cmd/native` has no tests |
| Linux native distribution, host CLI build and explicit CLI round trip | Exit 0 |
| Linux module/session vet and Windows/Darwin session cross-vet | Exit 0 |
| Five intentional negative overlays | Each produces behavioral test failures: destination recheck, receipt validation, waiting cancellation and both native-owner hooks |
| Entire host sessions package under race | Exit 1; 967 pass events and eight race warnings, described below |

The ordinary native fixtures cover OMP↔Claude and OMP↔Codex with attachments,
packed Codex input, large text above 3 MiB, output discovery, native readers
and resume commands. The separate lossless interchange fixture crosses the
previous 8-MiB archived-document boundary. Existing converter byte budgets
remain enforced; Codex's native rollout also duplicates user text in its UI
event records.

Target fixtures cover custom Claude/Codex roots, the selected Codex config
profile/provider, OMP config/agent overrides, XDG stores and named profiles.
They check directory replacement, symlink retargeting, previously missing
directories appearing during planning and same-size config edits with restored
mtime. An actual isolated converter changes a target symlink/provider during
RPC; ordinary apply refuses to publish into the stale target.

Reuse checks journal and per-blob identities/hashes, output contents and
parent permissions, module replacement/config, disable/remove, target/provider
switching and concurrent repeats. Modified output is preserved. A held
converter cannot block an unrelated migration key; a canceled waiter leaves
its same-key wait. Receipts remain bounded and process-local.

Real Pi and OMP journal fixtures share both an agent override and Pi's flat
session directory. All four Pi/OMP read-consent combinations preserve the
correct native owner. The bounded probe accepts only the recovered 256-byte
UTF-8 v1 OMP title slot. Unknown versions, malformed UTF-8/JSON, non-fixed
slots, unknown source/padding, unreadable files and symlinks are not claimed
as OMP in an ambiguous shared store. Nonshared legacy OMP ownership remains.

The full host sessions race run is deliberately **not recorded as green**.
Its warnings identify two core paths outside this module/session-owner
change: `provider/hold.go`'s asynchronous sweep reads `time.Local` while
`testenv.Zone` replaces it, and `cc_cd_test.go` mutates cached state while the
asynchronous cache writer serializes that state. The aggregate owners have
the exact log and must close those findings before a global race-green claim.

Native repetitions use fresh OS processes because Go C-shared libraries stay
mapped until process exit. Repeatedly installing many distinct library copies
in one process eventually exhausts glibc's static TLS block; this is a native
runtime/restart limit, not a reason to label an interrupted test as passing.
Cross-vet does not establish native Windows/macOS execution. Vendor
application acceptance, URL-image fetching and persistent receipt reuse are
not claimed.

## Optional JSONL Source module preparation

This module-only follow-up is based on `8a10b7808860513330a352cd860cd8be37965d18`.
It adds the opt-in `magpie_source16_preview` adapter over the exact public
Source DTO, without changing the default manifest, Go dependency, native entry
point or migration attachment planner. The prior default metadata advertises
API 1.3; its compatibility with SDK 1.4 is established by an actual SDK 1.4
build and complete route test suite, not a raised default requirement.

All execution used `usbox:/tmp/magpie-omp-source-prep-711zs07e`, Go 1.26.8,
Linux/amd64 with CGO, private HOME/XDG directories and pinned offline caches.
The default build used the frozen SDK 1.4 source at
`/tmp/magpie-omp-store-shared-final.I4rYI2hu/magpie`. The opt-in build used
Source preview tree `b98397198c9e83a8da2d726bf9ea1713a5fedbc8` through an
external temporary modfile. The imported public `session_source.go` SHA-256
is `2bc8473a641f083b1dda0d7add85405ad0f2fca3b9ff72b103c7a46fcd520dc5`.
No host/core source file, live session, account, credential or dependency was
changed. No positive test used a Go overlay.

| Gate | Terminal result |
| --- | --- |
| Original complete module suite before replay-selector extraction, race | Exit 0 |
| Complete default SDK 1.4 module suite, race count 20 | Exit 0; 1,300 test pass events, zero failures/skips |
| SDK6 Source and planner-neighbor tests, race count 20 | Exit 0; 440 test pass events, zero failures/skips |
| Complete opt-in module suite against SDK6, race | Exit 0; 83 test pass events, zero failures/skips |
| Default and preview module `go vet` | Both exit 0 |
| Default SDK 1.4 and opt-in SDK6 native builds | Both exit 0 |
| Preview factory manifest emission | Exit 0; exact API6 descriptor/native metadata |
| Actual preview native summary/transcript/input-plan RPC through frozen Source6 CLI | Exit 0 |
| Existing default native CLI checked preview/apply round trip | Exit 0 |
| Three causal negative overlays | Each exit 1 with named behavioral assertions |

Pass counts are events, including subtests/repetitions, not distinct tests.
The Source negatives restore acceptance of an oversized valid journal, bypass
the header/host-selected ID check, or discard tool-call display parts.
They fail actual assertions, not compilation. Their files and terminal records
are `negative-results.json` and `*-negative.log` in the verification root.

The real native proof installs only into its private test catalog and supplies
sanitized fixture bytes directly. It obtains the recorded summary, selected
tool/thinking transcript and two `[image]` display markers, then verifies the
empty Source acquisition plan. It neither creates blob files nor runs agents.
This proves native module invocation, not ordinary Source discovery or an
automatic takeover of the host's native OMP reader.

The replay parent-chain selector was extracted without changing portable
carry semantics. Source display remains separate: unfinished calls and thinking
are recorded content; images are explicit markers rather than fabricated
binary content. The recovered native fixture and a sanitized usage fixture
cover titles/cwd/time, auxiliary `model_usage`, absent versus recorded-zero
usage, active branches, tools/results, malformed/unknown formats, selected ID,
size/text/part bounds, overflow and cancellation. Binary media,
signatures and tool IDs have no additional structured Source DTO fields.

The host retains root grants, read consent, checked snapshots, cache
publication/revocation, pricing and resume authority. Sibling OMP blobs outside
`omp.sessions` cannot be granted by this module. This Source reader performs
no acquisition; the existing migration planner remains unchanged.
The full finite contract and remaining host assumptions are in
[SOURCE-PREVIEW.md](SOURCE-PREVIEW.md).

All 38 captured module file hashes matched after the positive/negative/native
runs. Final documentation changes do not alter the tested executable bytes.
The inventory, exact commands and terminal exits are `inventory.json`,
`final-results.json`, `native-proof.json` and
`default-native-cli-result.json` in the verification root.
Native library SHA-256:

- Default: `51bee975e6b16cf4dc3857350b3e26ab856b6f06c0d18e73dd867fe6f89736c3`.
- Preview: `e9650e2dd0f259f2d92e9acee31003140779bca3fa54d0049ee2eaf51403d61d`.

Native Windows/macOS execution, live vendor acceptance, binary image rendering,
subagent aggregation and ordinary host Source integration are not claimed.
No push or host merge accompanies this module preparation.

## Ordinary Source discovery/read through the real native module

This packaging/integration follow-up is based on module commit
`518b79713fce9ea1544c54e5dc1680676cf3c14f`. The Source descriptor and handlers
already existed there. The dedicated opt-in builder now emits their actual
distribution manifest from `SourcePreviewModule()`, rather than copying the
default migration-only JSON. No Go runtime code, default manifest, dependency,
default builder or migration planner changed in this follow-up.

The final evidence root is
`usbox:/tmp/magpie-omp-source-owner-final.9j7Howcf`.
Its `final-results.json`, `ordinary-results.json`, `negative-results.json`,
`module-inventory.json` and `ordinary-source-fresh20.log` record exact
commands, hashes and terminal exits. Builds/tests used Go 1.26.8, Linux/amd64
with CGO, pinned offline caches and private HOME/XDG. No live vendor,
credential, installed app or real session was used.

The host Source6 snapshot remains tree
`b98397198c9e83a8da2d726bf9ea1713a5fedbc8`. All 10,228 host source file hashes
were identical before and after the builds/probes. No host source edit,
production overlay or main-branch merge was required.

The separately compiled host-owner harness is outside both repositories. It
calls existing exported owner functions in host-internal packages; those
imports are not plugin implementation and are not shipped in the module.
Its exact Go source is `owner/main.go` in the evidence root, SHA-256
`9e9931ac8f25b95468b67ebd6022fe5e1a8a2256d2616c53acf6050b4ba6ef56`.
The host owns the synthetic roots, consent and revision-checked
`BindSessionSourceNative` call. No module option or manifest-returned path
grants authority.

| Gate | Terminal result |
| --- | --- |
| Source distribution builder tests | Four pass, including missing descriptor, wrong floor and escaping entry rejection |
| Actual Source6 distribution build and typed factory manifest | Exit 0 |
| Default SDK1.4 distribution rebuild | Exit 0 |
| Complete default SDK1.4 module race suite | Exit 0; 65 pass events, zero failures/skips |
| Preview Source/planner neighbors, race count 3 | Exit 0; 66 pass events, zero failures/skips |
| Race-instrumented host-owner build and ordinary native Source probe | Exit 0 |
| Full ordinary Source scenario in 20 fresh host processes | All 20 exit 0 |
| Existing default native checked CLI round trip | Exit 0 |
| Missing owner binding and migration-only distribution negatives | Both exit 2 with named behavioral assertions |

Each positive ordinary scenario verifies:

- Installing the module alone leaves the native OMP reader in charge.
- An incorrect package revision cannot bind native ownership.
- After explicit binding, normal List/Find/title lookup carries nonempty
  `SourceStatus`, proving module delegation rather than native fallback.
- Ordinary TranscriptOf and search read the selected recovered journal,
  including recorded tools/results, thinking and two `[image]` markers.
- A second recovered journal yields input 130/output 60 through ordinary
  Find/stats and the expected native gateway exclusion identities.
- The host renders the supported OMP resume command; the fixture executable
  is not a live agent.
- Consent revocation and module off hide the row, deny held transcript/resume
  access, and do not fall through to the native parser. Restored consent
  restores the permitted Source read.
- Both journals remain byte-identical; neither an in-root nor sibling blob
  store is acquired or created.

The negative using the same native package but skipping the owner binding
fails `ordinary host did not read through module Source`. The negative using
the migration-only package fails the owner Source binding assertion. Neither
is a compile error. A successful native Find alone therefore cannot satisfy
the E2E test.

The rebuilt default native library is byte-identical to the accepted
preparation artifact:
`51bee975e6b16cf4dc3857350b3e26ab856b6f06c0d18e73dd867fe6f89736c3`.
The generated preview library is also byte-identical:
`e9650e2dd0f259f2d92e9acee31003140779bca3fa54d0049ee2eaf51403d61d`.
The earlier race×20 results remain historical evidence on the same Go bytes;
this follow-up does not relabel them as newly executed.

This closes ordinary discovery/read verification for this optional Source6
host/module combination. The binding is still a host-owner Go API, not an
implemented public CLI/UI operation. No SDK1.6 capability is enabled on main
by this module commit, and no automatic native takeover is claimed. Marker-only
media, sibling blob grants, absent subagent aggregation and platform/vendor
limits described above remain unchanged.
