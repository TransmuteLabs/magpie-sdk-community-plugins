# Claude Desktop live acceptance

Status: instructions prepared; real Desktop acceptance has not been executed.

Date: 2026-11-17.

## Preconditions

Use an SDK-enabled Magpie host, the existing verified native distribution of
`agent-claude-desktop` version `0.1.0`, and the real installed Claude Desktop.
The package requires API `1.5` and native ABI `1`. Its reviewed vendor
reference is Desktop `5.11492.5`; record the actual installed version before
testing. Only macOS arm64 native execution has been verified. The amd64 target
is declared but untested; the retained arm64 library cannot be installed as an
amd64 build. [Verification](VERIFICATION.md) identifies the tested host source
and native library.

Magpie must already serve the intended models. Use its existing configured
providers and update source. Fully quit Desktop before connecting or restoring
its configuration; closing its window alone is insufficient. If a managed
policy or existing hybrid deployment controls the selected profile, resolve
that existing setting explicitly instead of repeatedly overwriting files.

## Install and inspect

Set these paths to the already available host and distribution:

```sh
MAGPIE_BIN='/absolute/path/to/the/sdk-enabled/magpie'
CLAUDE_DESKTOP_PACKAGE='/absolute/path/to/the/existing/distribution/claude-desktop'

"$MAGPIE_BIN" --version
"$MAGPIE_BIN" status --json
"$MAGPIE_BIN" plugin sdk add "$CLAUDE_DESKTOP_PACKAGE"
"$MAGPIE_BIN" plugin sdk inspect agent-claude-desktop
"$MAGPIE_BIN" claude-desktop-sdk
```

The install result should identify `agent-claude-desktop 0.1.0 (on)`.
Inspection should show the native entry, API minimum, target
`claude-desktop` and five declared configuration files. Installation and
inspection alone must not change the selected Desktop profile or take over
the canonical agent. `status --json` describes the running Magpie host;
the final command displays agent discovery information, not live Desktop
startup or native connection state.

## Connect and reopen

In Magpie's Agents page, select **Claude Desktop SDK** and choose **Connect**.
Check its connected state and any reported detail before opening Desktop.
The current native profile has no dedicated CLI connect/status subcommands.

For an exact diagnostic equivalent, evaluate the following in the already
authenticated Magpie window's browser console. They use that window's
existing owner session:

```js
await fetch('/api/agents/connect/claude-desktop-sdk', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: '{}'
}).then(async response => ({status: response.status, body: await response.json()}))
```

```js
(await (await fetch('/api/state')).json()).agents
  .find(agent => agent.id === 'claude-desktop-sdk')
```

Connect must return HTTP 200; the state row must have `wired: true` and
`native.provider: "connected"`. An error or invalid state is not acceptance.

Open the real Desktop manually. This package neither starts nor restarts it.
Its profile selects Magpie's existing `/agent/claude-desktop` gateway path.
The four Desktop files and separate hash-only ownership receipt are listed in
the package README. Other profiles and unrelated Anthropic provider settings
must remain intact.

## Manual checks

Use a new scratch folder and distinctive harmless prompt markers. Keep the
same model selection during a continuation so failures are attributable.

| Check | Required observation |
| --- | --- |
| Startup | The actual Desktop interface opens in the selected gateway deployment without requiring an Anthropic sign-in. If a sign-in screen remains, record that failure rather than signing in to make this check pass. |
| Chat | A marked prompt receives a complete streamed response through the chosen Magpie model. A follow-up in the same conversation retains the preceding context. |
| Code | In the scratch folder, approve a harmless write/read task, then ask a follow-up that reads the resulting file. Tool results return to the same conversation and generation continues without duplicated tool execution or serialized tool artifacts. |
| Cowork | Complete a harmless task in the approved scratch folder, including an approved tool result and a follow-up. Desktop's VM/runtime and operating-system permissions remain prerequisites; an unavailable prerequisite is blocked, not a pass. |
| Reopen and continue | Quit and reopen Desktop manually, resume the checked conversation, and complete a further response/tool continuation without losing the selected gateway deployment. |

Record each result independently. Chat success does not establish Code or
Cowork success, and a successful configuration write does not establish any
of these application behaviors.

## Expected Magpie evidence

For the marked inference requests, inspect Magpie's request history and routing
trace. Expect canonical agent `claude-desktop`, the selected provider/account,
the resolved upstream model, final status and available token usage. A Desktop
catalog alias must resolve to the intended upstream model rather than becoming
an unrelated global Anthropic model. Check streaming completion and tool
continuation against the application's observed result, not only HTTP 200.

A normal request through the existing Anthropic provider from another already
configured client should work before connection and after disconnect. Its
provider configuration and unrelated account state must not change.

Useful existing CLI commands are:

```sh
"$MAGPIE_BIN" status --json
"$MAGPIE_BIN" usage --last 30m
"$MAGPIE_BIN" log -n 200
```

The owner APIs behind the request history and trace are
`GET /api/usage/requests?period=all&limit=100` and
`GET /api/gateway/trace`. Retain request IDs, prompt markers and relevant
redacted errors; do not include profile credentials or owner-session secrets
in shared evidence. Do not infer complete Desktop network isolation from
Magpie's inference ledger.

## Disconnect and rollback

Fully quit Desktop. The ordinary native-agent disconnect command is:

```sh
"$MAGPIE_BIN" claude-desktop-sdk default
"$MAGPIE_BIN" claude-desktop-sdk
```

The `default` command enters the host's checked disconnect path for this
native profile, including repeated disconnect and native-read failure
recovery. It does not guarantee restoration from damaged ownership evidence.
The final discovery command is not a connection-state check; verify the
Agents row separately.

Alternatively use **Disconnect** on the same Agents row. Its existing owner
API equivalent is `POST /api/agents/disconnect/claude-desktop-sdk` with
`{}`. Verify the row is disconnected before optionally disabling the module:

```sh
"$MAGPIE_BIN" plugin sdk off agent-claude-desktop
"$MAGPIE_BIN" claude-desktop
```

Disconnect restores prior owned values only where the current values still
match what the module applied. Later user edits, sealed credentials and other
profiles are preserved. Newly created owned files/receipt are removed when
their checked restoration permits it. A missing receipt, changed selection
or invalid state requires the host's displayed recovery path; do not delete
the whole profile library or overwrite unrelated configuration.

Reopen Desktop manually and verify the previous deployment/profile selection.
Repeat the ordinary Anthropic control request. Retain separate outcomes for
restoration and inference.

## Acceptance boundary

These instructions add no live result. The isolated host test covers install,
explicit ownership, profile-mode normalization and refusal, model aliases,
count_tokens, Messages/SSE, telemetry and restoration with a synthetic
upstream. Actual Desktop startup, Chat, Code, Cowork, managed-device behavior
and tool continuation are the checks above. No installed-application update
or real-key signing result follows from this plugin procedure.
