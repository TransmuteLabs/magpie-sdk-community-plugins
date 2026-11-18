# Source resume and bounded foreground CLI control

2026-11-16. The full 0.15.0 distribution adopts the existing public
API1.10 `SessionResumeRecipe` and `AgentProcess` contracts. Its SDK minimum
remains 1.12 because it also consumes public request policy. The explicit
0.7.3 provider-only composition does not declare a process or a window.
No package or donor version was advanced for this adoption.

The default CLI history source declares `resume: agent`, with owned agent
`zcode-native` and literal argv `["--resume", "{sessionId}"]`. Ordinary
`ResumeNow` and the source-resume CLI resolve the checked native session,
approved source cwd and current connected profile. History supplies no
executable, credentials or substitute profile. Source consent, read switches,
package/config/profile changes and cancellation remain host admission checks.
Resume opens an interactive CLI; it does not attach a desktop process.

## Foreground entry points

The full module declares recipe `native.run`, agent `zcode-native`, argv
`["app-server", "--stdio"]`. The host resolves the connected executable,
profile environment/config and process lifetime. The module never calls an
OS subprocess API. The ordinary resolver starts this child in profile HOME.
The V4 workspace is a separate explicit module configuration:

```json
{"dataDir":"/private/controller","nativeWorkspace":{"workspacePath":"/approved/workspace","workspaceKey":"workspace-1"}}
```

`dataDir` must be an existing or newly created private controller directory.
The workspace path must be absolute and clean; its key must be explicit.
Neither module actions nor window inputs can override executable, argv,
process cwd, environment or profile. This contract does not prove that
arbitrary source cwd can become process cwd.

`native.run` takes `sessionId`, `commandId`, gateway `model` (`provider/model`)
and bounded `text`. It requires a current selected account/generation and
host operation. `page.native-run` exposes the same action through the
ordinary selected-account window binding. The page confirms one input,
locks account/financial controls during the wait and retains the command ID
after uncertainty. `page.native-stop` remains available during that wait and
uses the original selected session, command and account, rather than mutable
form values. A stop request is never automatically repeated.

Only a matching active account/generation/session/command may request stop.
The module obtains `expectedForegroundExecutionId` from the owned V4 control
projection; a foreign or missing execution identity cannot authorize stop.
The run and stop actions are foreground operations, not a scheduler.

## Wire, completion and uncertainty

The module consumes bounded LF/CRLF JSON frames, including split UTF-8 and
large snapshots below the 16 MiB frame bound. Stdout and stderr are drained
independently. String and integer reverse RPC identities are returned with
their original JSON type. Subscription ACK precedes the separately
correlated initial snapshot. Session/subscription/epoch/sequence and command
query keys must agree before control state or completion is used.

Only `magpie` runtime-header requests for the exact selected workspace,
session and model are supported. Their correlated reply is
`{"headersApplied":false}`: the child keeps its existing connected profile
configuration. No account credential is returned over this reverse channel.
Unknown interactions fail closed; permission/tool approval is not automatic.
Cancellation notifications must name a previously correlated request.

Before any process write, the existing private controller store saves a
separate native command intent. Exactly one `sendText` is permitted. Its ACK
does not establish completion: the exact `v4/commands/query` result must
report `completedSuccess`, `completedInterrupted` or `failed`. The module
closes the process and requires joined cleanup before publishing completion.
After a successful close it drains and joins outstanding read callback
replies within the same bounded five-second close window. Canceling those
read waits early can leave a host callback outstanding when success is
published; the module's deterministic regression protects that ordering.

Cancellation, account/config/profile/off revocation, process loss, foreign
replies, unknown command state and uncertain writes retain pending intent.
A fresh host operation, new command ID or account generation cannot replace
an unresolved command for the same configured workspace/session. Completed
command IDs cannot be reused. There is currently no reconciliation/recovery
action for this native intent; unresolved sessions remain blocked rather
than silently replaying business input. Changing private storage or workspace
configuration intentionally changes the configured owner, so operators must
retain the original journal when investigating uncertainty.

Queue deadlines, Off-Peak ticket polling, card/charge settlement, reset debt
and financial claim journals retain their existing owners. Native intents
have a separate journal field and do not clear, settle or replay those debts.

## Verification and limits

The synthetic peer is `src/testdata/native-contract/peer.py`. The installed
fixtures in that directory use an ordinary installed full distribution,
connected `zcode-native` profile and public window/process entry points.
They require explicit `ZCODE_STORAGE_DIST`, `ZCODE_STORAGE_WRITER`,
`ZCODE_NATIVE_PEER` and absolute `ZCODE_NATIVE_PYTHON`; no live app/account
or vendor is used. Fixtures join every synthetic child and inspect the
host-owned terminal process receipt.

`docs/NATIVE-ADOPTION-EVIDENCE.md` records the bounded receipts and final
source freeze. Parent owns final combined-host acceptance and ZCode policy
replay. Synthetic Darwin execution does not establish real desktop attach,
real vendor behavior, Windows process support, arbitrary native workspaces,
full V4 projection/interaction parity or rendered-browser acceptance.
The old generic seam proposal is retained as historical context in
`NATIVE-CONTRACT-REQUEST.md`; it is not a current missing-host-contract claim.
