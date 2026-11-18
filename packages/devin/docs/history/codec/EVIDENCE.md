# Devin protocol evidence

This inventory distinguishes generated wire contracts, observed captures and
reconstruction choices. The research checkout is `Agents/devinapp`; none of
its binaries, captures or account files are runtime dependencies of Magpie.

## Generated descriptors

Paths below are relative to `Agents/devinapp/gui-src/windsurf-extension-unpacked/`.

| Source | SHA-256 | Contract used |
| --- | --- | --- |
| `module-432.js` | `f710e469d4a6e28ac54d80de56a8619802d8a57bed7b1e806f6c51a569a8ea62` | `GetChatMessageRequest` at 3577; response at 3786; native discovery request/response classes |
| `module-39768.js` | `14afecc6f37c7ae6750b1706aa309a27fb20f5621997a2928a6d6fa88dcef4e3` | `ChatMessagePrompt` at 2156, tools at 2366, tool choice at 2514 |
| `module-98224.js` | `ef66a64bdce5ec10bf1eb34d8fe9d7927527aab7d24cb742c3d658c2906aeb10` | `CompletionConfiguration` at 9990, planner enum at 6102, model configs at 13402 and model info at 17657 |

The generated service registration is in `module-16806.js`. Native discovery
uses unary messages, while chat is server streaming. The native model status
enum describes advisory severity, not an authoritative availability verdict.

The installed `@magpie-community/opencode-devin-auth` version 0.1.39 supplied
the existing request/message encoder, auth-owner boundary and response/usage
mapping. Its `index.mjs` SHA-256 is
`cc720b38f75505d53f352331115aca65476799a43be60ff8842adeefe9ff5114`.
The installed package was read, not modified or executed with live credentials.

## Observed capture shapes

Fourteen `GetChatMessage-req.proto` files under `cassettes/gui` were inspected
as bounded protobuf data, without printing their metadata credentials or prompt
contents. Two top-level field-layout variants were observed. They include
trajectory reference 15, cascade 16, default planner mode 20, model UID 21,
execution id 22 and native session id 28.

Representative input identity:

- `gui/1793131306/037-exa-api_server_pb-ApiServerService_GetChatMessage-req.proto`:
  `c1262c78329385bac6d5f8a84542705fe42d35d03ed7323dc18e7a0ab403857c`.
- `gui/1793139630/087-exa-api_server_pb-ApiServerService_GetChatMessage-req.proto`:
  `23dbeb5d1cece31fd2a3fecda198e6dc90c84891207fc9d9729ebe67db66747c`.

The observed configuration is num-completions=1, max-tokens=128000,
max-newlines=400, temperature=1, top-k=40 and top-p approximately 0.95.
That observation does not prove these values are optional or semantically
equivalent to another provider's defaults. Nonzero max-newlines/top-k remain
unsupported in the translated Chat route.

Some observed histories include native signed thinking and generation-model
provenance. Such histories remain explicitly refused until the host can
preserve their signature and account/protocol origin end to end.

The `.proto` response files inspected contain individual protobuf messages,
not complete Connect streams. They therefore do not establish complete stream
termination behavior. No original Devin process or live vendor request was run.

## Reconstruction boundaries

`reference/crates/chisel-agent-bin/src/devin.rs` supplies useful executable
reconstruction, but its comments and fallback choices are not wire authority.
For example, a reconstructed model parser treats config field 10 as beta;
the generated descriptor identifies beta as field 9 and provider as field 10.
Likewise, ModelInfo field 4 is an int32 varint, not a byte string.
Magpie's projections follow the generated descriptors.

The source documents `AssignModel`, including its assignment JWT and harness
ids. This module does not fabricate them or advertise native routers.
Authentication, user-status/billing RPCs and native session-control operations
remain with their existing owners or unsupported, rather than returning
synthetic success.

## Standalone continuation

Version 0.2.0 extends the original extraction using the same descriptor
hashes. `module-432.js:8764–9150` establishes cascade discovery and the three
site request field layouts. `module-98224.js:4600` distinguishes SYSTEM (2,
model output) from SYSTEM_PROMPT (5); `module-39768.js:2156` establishes
prompt token/telemetry fields and known inactive provenance defaults.
`module-98224.js:22636` establishes ImageData fields 1/2/3. Active image
captions remain refused; tool-result images use the host's existing Chat
image-result representation.

Only new synthetic fixtures were added. No application, captured request or
account bytes were copied or executed. `CONTINUATION.md` and
`CONTRACT-BLOCKERS.md` at the repository root record the implementation
boundary; current verification is recorded in root `EVIDENCE.md`.
