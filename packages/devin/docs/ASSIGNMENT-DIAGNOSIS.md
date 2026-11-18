# Why no-op catalog sync invalidates native account operations

Date: 2026-11-14. **Historical d070 Devin agent acceptance is red.** A controlled
installed-host reproduction separates stable module identity from a host
maintenance revision that changes both durable write identity and the
post-dispatch read fence. No host source was edited.

The later leader-owned conservative SDK 1.6 candidate passes actual
one-module identity and unchanged real-transition acceptance with this
original library. `NOOP-ACCEPTANCE.md` records its exact source/binary
digests and a behavioral red on d070. Final upstream/SDK 1.7 and broader
bootstrap/live-client acceptance remain separate gates.

## Inputs and scope

Module base is `afdb5d805a8c2e6e15838ddfc6a556415d6d3759`; production Go
remains `7a621943095fb7f4a1e05857ea380a6788fb161a`. The new diagnostic and
identity regression change no runtime behavior. The existing d070 host
and agent library were reused:

| Input | Exact identity |
| --- | --- |
| Host commit | `d07078c7926daf6f33af09c40297c941bd39da92` |
| Host tree | `ed28dfd65eca68d15e9400b56660bbbf5e7bc319` |
| Host SHA256 | `bc366fa3e321a7e7c0f5a6735a76c77c1488145b47892cf8551f08a8f444d024` |
| Agent 0.5.1 / SDK 1.6 library SHA256 | `22eefa631bb913a2e7ba1b74dd74045c62f78988b622edc6f75be4958bf69397` |

The leader's SDK-alone freeze is git **tree**
`f536d8202e40aae4b0fc36630aa3c1eb496d58d5`, not a commit or a newly tested
executable here. Its complete diff from d070 contains documentation and
tests only. Broad acceptance and the next common host contract diff belong
to the leader.

`assignment_identity_e2e.py` installs exactly one bundle into an isolated
HOME, starts ordinary `magpie serve`, and uses synthetic local native
peers plus a deny proxy. It reads only this fixture's private binding,
journal and assignment-receipt files. `inbound.decode` and `agent.plan`
are invoked through the actual installed SDK CLI. The diagnostic makes no
automatic retry, guard adjustment, host patch or package installation.
It is neither the Rust reconstruction nor the original Cognition client.

## Controlled duplicate

The first trial is
`usbox:/tmp/magpie-devin-identity.Wr7PMMHe/identity/result.json`, SHA256
`f1d483a9754f5acc42ee44e2b4937e3ab84a30321e0d983082c6da42caa7b0cb`.
Its diagnostic exit is zero; its acceptance status is explicitly red.

| Ordered action | HTTP | Actual assignment dispatches | Binding revision |
| --- | --- | --- | --- |
| First AssignModel | 200 | 1 | 1 → 1 |
| Identical duplicate before explicit sync | 403 | 1 | 1 → 1 |
| Ordinary `model name MODEL "Identity fixture label"` | CLI exit 0 | 1 | **1 → 2** |
| Same duplicate after that sync | **200** | **2** | 2 → 2 |

Only `revision` differs between the complete binding records before and
after sync. Phase stays owned. Package/config/descriptor/bin, selected
model, file snapshots, shell ownership and recovery state all match.
The installed module's sync plan has `files: []`, unchanged connected/model
values, and binding revision `"1"` echoing its CAS input.

The actual installed decode results before and after sync are identical:

```json
{
  "protocol": "account-operation",
  "body": "Eg1maXh0dXJlLW1vZGVsGhZpZGVudGl0eS1maXhlZC1jYXNjYWRl",
  "session": "identity-fixed-cascade",
  "requestId": "assignment-6c49b5a9549485e466fe9f1d2a479a5e6d22420e53e8d8b29fa4f236af8ac8eb",
  "executor": "same-module",
  "accountOperation": "model-assignment"
}
```

Both vendor arrivals have wire SHA256
`949990d1c80232d0c4e140f4774923b6d1e4a1fff9e5cd0de9115e92afd0c966`.
They use native model `fixture-model`, the same cascade, selected account,
generation 1, attempt 1 and caller. At the first arrival the journal is
accepted, then completes. At the second arrival that completed journal
still exists and a **different** operation is accepted. The store did not
lose or reset the previous receipt.

The exact d070 operation formula is:

```text
"native-account-" + hex(SHA256(json.Marshal([]string{
  manifest.ID, operation.ID, binding.Revision,
  scope.Provider, scope.Account, callerID, decoded.RequestID,
})))
```

For this trial all inputs except the revision match:

```text
["devin-inbound","model-assignment","1","devin",
 "a00998deec82ce185ccb1e862ff031ab","04820e3b8201c16590301c94d0bd8c9c",
 "assignment-6c49b5a9549485e466fe9f1d2a479a5e6d22420e53e8d8b29fa4f236af8ac8eb"]
  → native-account-07ae956ed2de989fc36ac7d421a966ac1b2ab03c2394a8a469e2d857b1405d2b

the same compact JSON array with "2" in its third element
  → native-account-dac48438081de128d90004c357dc8731e3d123a50de1d72b1c0ecdc4b645879b
```

The fixture reconstructs both hashes and compares them with persisted
journal scopes. Session and account generation are part of the checked
full journal scope, not additional fields in that hash preimage.

## Held bootstrap read

The second bounded trial retained the same duplicate sequence, then held
one GetCliTeamSettings mock reply. At vendor arrival binding revision was
2. While that reply was held, an ordinary display-name update completed
another no-op agent sync, changing only revision 2 → 3. Releasing the
original reply returned **403**, with body:

```json
{"code":"permission_denied","message":"Native account operation failed"}
```

No read was retried. The raw trial is
`usbox:/tmp/magpie-devin-identity.Wr7PMMHe/identity-held-bootstrap/result.json`,
SHA256 `3cf265fccba0c45dd86fd847a394e02f537f37a229a4524f1c572381e10f5d0a`.
Its runner exit is **1**: the initial diagnostic wrongly required an
external `X-Should-Retry: false` header. The host sets that header on the
adapter, but this module's `accountResponse.Finish` projects only fixed
Content-Type/Connect headers. The observed HTTP status/body/revision
sequence already proves the read fence. The corrected assertion is
validated against this saved raw receipt; no new host request is needed
to relabel the diagnostic. The raw failed gate remains preserved.

The module unit regression separately checks repeated decode and changed
credential/informational fields 30/31 preserve RequestID/Session/body;
changed model, cascade or lead-router content changes RequestID. It passes
on usbox alongside the existing agent binding lifecycle regression.

## Actual real-model negative control

`assignment_transition_e2e.py` reuses the exact d070 host and agent library
above, installed as the only bundle in another isolated HOME. The actual
host maps the expected library SHA256 after its first module call.
Production Go, the write hash and all binding checks remain unchanged.

The final receipt is
`usbox:/tmp/magpie-devin-identity.Wr7PMMHe/real-transition-final/result.json`,
SHA256
`0b9005cf01d1c5029fa714d3e298da94b453416fc17cdb9ad6c4016ae7a86612`.
The runner SHA256 is
`6cb6179e9bf2d05d047417ea5e2d36a13f439be99b73dd3c3a4451dfdf0896d9`.
Its exit is zero, status `real-transitions-fenced`, while optional Devin
bootstrap acceptance remains red.

| Ordered action | HTTP | Actual assignment dispatches | Owned binding |
| --- | --- | --- | --- |
| Initial control assignment | 200 | 1 | fixture-model, revision 1 |
| Deliberate same-revision duplicate | 403 | Still 1 | Unchanged; original completed journal retained |
| Hold GetCliTeamSettings; ordinary CLI selects fixture-free; release reply | 403 | Still 1 | fixture-model → fixture-free, revision 1 → 2 |
| Separate old-model assignment with a new cascade | 403 | Still 1 | fixture-free, revision 2; new journal is not-started |
| Ordinary CLI reselects fixture-model | CLI exit 0 | Still 1 | fixture-free → fixture-model, revision 2 → 3 |
| New assignment/cascade accepted at peer; select fixture-free; release reply | 403 | 2 total; exactly one for this new write | fixture-model → fixture-free, revision 3 → 4 |

The ordinary selection command is
`devin-native model magpie/extension-6e0dceade11becc757c72a0e/fixture-free`.
The durable model value is
`extension-6e0dceade11becc757c72a0e/fixture-free`; the module intentionally
removes the CLI's `magpie/` prefix. Package/config/descriptor/bin remain
unchanged across these real model transitions.

The read reaches the local peer at revision 1. The CLI finishes the model
change before the fixture releases that original reply; the original HTTP
call returns the established permission-denied body. Exactly one native
team-settings request is dispatched. The separate old-model assignment
never reaches the peer and is refused by the module's host-selected-model
payload guard before acceptance. It is a distinct negative control, not
evidence of the post-reply host Check.

The held write reaches the peer with an **accepted** journal at revision 3.
That same accepted entry remains intact after model selection advances to
revision 4. After the original reply is released, the module call completes
under its original scope and the journal becomes **completed**, but the
gateway refuses delivery with HTTP 403. This separates the post-call
binding fence from a module error. Its operation is
`native-account-37707f03deeee8962e5c004b071eb5c723ec9aa8fbbb7335506bbf1a8c5b7a0b`;
generation 1, attempt 1 and session `transition-distinct-held-write` persist.
Installed decode before/after returns the same body, session and
`assignment-a04eb88aefb3371112e382096f67b871e02da451cec0710efbdff1b2e816c774`.
There is no retry or second dispatch of that accepted write. An HTTP refusal
after a real transition must not be treated as proof that the write was
never accepted.

Two fixture-development failures remain preserved: `real-transition/`
checked library mapping before lazy loading, and `real-transition-ready/`
expected the CLI prefix in the durable model value. Their raw exits are 1,
with receipt SHA256s
`0ad041446f749a6e115684158e63086d4fc3efdfe39931d2315d82c66fa139ed`
and `1d54c4c62788b2e238cf731b87ffd65b09218e1b481967f43d7104ae14cc93e8`.
The second already observed the real model change and read403; the corrected
assertions were validated against that saved receipt with zero new host
requests. A fresh final fixture supplies the previously unreached
old-model/held-write controls. Earlier runner versions, failed gates and
logs are retained; none is relabeled as a passed runtime gate.

All three fixtures remove their private HOMEs and stop their own host
processes; external requests and vendor assertion failures are zero. The new
negative control covers actual model selection, not every possible
package/config/account-generation/reconnect/disconnect transition. Those
generic regressions still belong with the leader's eventual common fix.

## Exact host seams

Line references below are to the frozen d070 source:

1. `internal/provider/modelprefs.go:66` / `internal/catalog/live.go:463`:
   the model display-name command calls `catalog.Touched`.
   `internal/cli/main.go:287` registers `agent.SyncCatalogSoon`, and its
   command defer at line 344 calls `SyncCatalogSettled`. The fixture
   therefore waits for the actual command, not an arbitrary quiet period.
2. `internal/agent/extension_controller.go:230`: apply holds target and
   exclusive ledger locks, checks authority and file/shell preconditions,
   then unconditionally publishes `Revision: prior.Revision + 1` at line
   279. Empty file plans and unchanged binding values still take this path.
3. `internal/gateway/protocol_account_operation.go:135`: write identity
   includes that maintenance revision. The next identical write acquires a
   new journal key.
4. `internal/extension/manager.go:851` and `:883`, plus
   `internal/extension/store.go:325`: dispatch/acceptance are persisted under
   the supplied operation key. Completed → dispatched is refused for the
   same operation; the changed key admits a new operation.
5. `internal/agent/extension_account_operation.go:40`: the binding check
   requires exact revision and selected model. The gateway checks again
   after `lease.Call` at `protocol_account_operation.go:153`. A no-op sync
   between admission and reply invalidates that read.

`controller.read` takes a shared ledger lock. Successful apply holds its
exclusive lock through pending → owned publication. These observations do
**not** establish exposure of a transient pending record; revision churn
alone suffices. Earlier natural two-dispatch/bootstrap failures lacked the
full binding timeline. This controlled reproduction establishes the
mechanism without inventing their exact unsampled interleaving.

## Minimal common host fix and regressions

Add a conservative no-op branch in `extensionController.apply`, **before**
constructing/writing a pending record. Keep its existing target/ledger
locks, root/CAS/ownership/runtime checks and file/shell preconditions.
Limit the branch to `intent == "sync"` with a prior owned, recovery-safe
record, unchanged descriptor/bin/package/config and connected/model values.
For the narrow first fix require no declared file changes, all current
file snapshots equal prior applied snapshots, and the same owned shell
targets/bodies with no creation or actual edit. New/missing/drifted targets
or any real effect must take the existing path or fail its current guard.

Even in this branch, execute the checked zero-file `ActionPlan` through its
existing `Execute(ctx, Preview().Revision)` so authority, root identity,
action locks, CAS and one-use validation remain in force. Recheck current
runtime/file/shell authority before returning success. Leave the durable
binding record and its revision unchanged. A nonempty file plan can remain
conservatively outside this optimization even if its bytes happen to match.
No SDK version or write-journal schema change is necessary for this narrow
fix. The real-model negative control above proves why revision must remain
in write identity and the post-call Check must remain exact. No shared host
production change is made in this module checkpoint.

Proposed generic host regressions:

- Controller sync with no file effects and identical owned shell/binding
  returns success with an identical durable record; repeat via ordinary
  catalog display-name update.
- Installed write accepted/completed once → no-op sync → identical write
  remains refused with exactly one vendor dispatch and the original key.
- Hold an admitted native read at its local peer, perform no-op sync,
  release it: the original valid response survives. Hold another across
  real model/config/package/account/ownership revocation: it remains refused.
- Real file/shell changes still apply or are refused on ownership drift;
  stale concurrent plans, cancellation and incomplete rollback keep their
  existing guarantees.

Removing revision from the write hash alone would not fix the read fence
and would conflate real re-adoption/ownership epochs. Removing exact
binding checks, adding automatic retries or choosing a module-side random
identity would weaken the contract. The proposed fix retains both checks
and the journal while eliminating the false ownership transition.

## Scoped verification

All execution is on usbox with the pinned Go 1.26.8 toolchain/caches,
offline module resolution and isolated HOME/XDG/TMPDIR. The identity and
existing agent binding lifecycle unit tests, scoped `go vet ./codec .`,
changed-test gofmt, diagnostic Python compilation and all five provenance
regressions pass. `diagnosis-validation.json` beside the raw receipts
records the commands and validates the corrected read-fence assertion
against the saved second trial, with **zero new host requests**. Runtime
Go and the frozen libraries were not rebuilt for these test/doc changes.
`real-transition-final-gate.json` records the subsequent installed-host
negative control, its artifact/input hashes and saved-read validation.
Only the new Python fixture and provenance inputs need fresh static checks;
the already passing unchanged Go/host suites are not rerun for this evidence.

Module-only domain parity can proceed independently. Other generic gaps
remain delegated remote/restricted native callers, explicit group account
selection and host-admitted post-reply keepalive. Signatures and synthetic
native resume are receipt-bound; original-client resume, live vendor and
cloud ACP interoperability remain unproven. Initial SDK-alone rollout is
independent of optional Devin acceptance.
