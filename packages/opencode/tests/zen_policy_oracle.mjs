import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { collapseResponse } from "../src/testdata/donors/zen-collapse.mjs";

const rows = [];
const data = value => `data: ${JSON.stringify(value)}\n\n`;
async function collapsed(name, protocol, request, reply) {
  const response = await collapseResponse(new Response(reply, { headers: { "content-type": "text/event-stream" } }),
    protocol === "messages" ? "anthropic" : protocol);
  const expected = await response.text();
  assert.equal(response.status, 200, `${name}: ${expected}`);
  rows.push({ kind: "collapse", name, protocol, request, reply, expected });
}
const tools = JSON.parse(fs.readFileSync(new URL("../src/testdata/donors/zen-tools.json", import.meta.url), "utf8"));
for (const row of tools.filter(row => row.streaming)) {
  const response = await collapseResponse(new Response(row.expectedReply, { headers: { "content-type": "text/event-stream" } }),
    row.protocol === "messages" ? "anthropic" : row.protocol);
  const expected = await response.text();
  assert.equal(response.status, 200, `${row.name}: ${expected}`);
  rows.push({ kind: "collapse", name: row.name, protocol: row.protocol, request: row.request, reply: row.reply, expected });
}
await collapsed("chat/text-reasoning-refusal-usage", "chat", '{"tools":[]}',
  data({ id: "fixture", created: 42, model: "fixture-free", choices: [{ index: 0, delta: { role: "assistant", content: "Привет ", reasoning_content: "think ", refusal: "no " } }] }) +
  data({ choices: [{ index: 0, delta: { content: "世界", reasoning_content: "again", refusal: "thanks" }, finish_reason: "stop", logprobs: { content: [{ token: "a" }] } }] }) +
  data({ usage: { prompt_tokens: 2 } }) + data({ usage: { completion_tokens: 3 } }) + "data: [DONE]\n\n");
await collapsed("chat/ordered-choices-and-legacy", "chat", '{"tools":[{"type":"function","function":{"name":"Read"}}]}',
  data({ choices: [{ index: 2, delta: { content: "third" } }, { index: 0, delta: { function_call: { name: "Read", arguments: "{" } } }] }) +
  data({ choices: [{ index: 0, delta: { function_call: { arguments: "}" } }, finish_reason: "function_call" }, { index: 2, delta: {}, finish_reason: "stop" }] }) + "data: [DONE]\n\n");
await collapsed("messages/text-thinking-signature-usage", "messages", '{"tools":[]}',
  data({ type: "message_start", message: { id: "fixture", type: "message", role: "assistant", content: [], usage: { input_tokens: 4 } } }) +
  data({ type: "content_block_start", index: 0, content_block: { type: "thinking", thinking: "", signature: "" } }) +
  data({ type: "content_block_delta", index: 0, delta: { type: "thinking_delta", thinking: "думать" } }) +
  data({ type: "content_block_delta", index: 0, delta: { type: "signature_delta", signature: "fixture-signature" } }) +
  data({ type: "content_block_start", index: 1, content_block: { type: "text", text: "hello" } }) +
  data({ type: "content_block_delta", index: 1, delta: { type: "text_delta", text: " 世界" } }) +
  data({ type: "message_delta", delta: { stop_reason: "end_turn" }, usage: { output_tokens: 5 } }) + data({ type: "message_stop" }));
const request = '{"tools":[{"name":"Read","input_schema":{"type":"object"}}]}';
await collapsed("messages/fragmented-tool-input", "messages", request,
  data({ type: "message_start", message: { id: "fixture", content: [] } }) +
  data({ type: "content_block_start", index: 0, content_block: { type: "tool_use", id: "call", name: "Read", input: {} } }) +
  data({ type: "content_block_delta", index: 0, delta: { type: "input_json_delta", partial_json: '{"path":' } }) +
  data({ type: "content_block_delta", index: 0, delta: { type: "input_json_delta", partial_json: '"fixture"}' } }) +
  data({ type: "message_delta", delta: { stop_reason: "tool_use" } }) + data({ type: "message_stop" }));

const literal = fs.readFileSync(new URL("../src/testdata/donors/zen-environment.js.txt", import.meta.url), "utf8");
const addEnvironment = vm.runInNewContext(literal.replaceAll("export function ", "function ") + "\naddEnvironment", {
  process: { platform: "linux" }, localDirectory: value => value,
  repository: () => ({ root: "/fixture" }), opencodeTmp: () => "/fixture/tmp/opencode",
  homedir: () => "/fixture/work",
}, { timeout: 1000 });
for (const [protocol, inputs] of [
  ["chat", [
    { messages: [{ role: "system", content: "Caller." }] },
    { messages: [{ role: "developer", content: "Caller." }] },
    { messages: [{ role: "system", content: [{ type: "text", text: "Caller." }] }] },
    { messages: [{ role: "user", content: "Use /fixture/work." }] },
    { messages: [{ role: "system", content: "<env>\n  Working directory: /already\n</env>" }] },
  ]],
  ["responses", [{ input: "fixture" }, { instructions: "Caller.", input: [] }]],
  ["messages", [{ system: "Caller.", messages: [] }, { system: [{ type: "text", text: "Caller." }], messages: [] }]],
]) {
  for (const [index, input] of inputs.entries()) {
    const request = JSON.stringify(input);
    addEnvironment(input, protocol === "messages" ? "anthropic" : protocol);
    input.stream = true;
    if (protocol === "chat") input.stream_options = { include_usage: true };
    const expected = JSON.stringify(input).replaceAll(
      " over generic system temporary directories such as /tmp; it is pre-created and approved for external access.",
      " for temporary files.",
    );
    rows.push({ kind: "environment", name: `${protocol}/environment/${index}`, protocol, request, expected });
  }
}
const output = JSON.stringify(rows, null, 2) + "\n";
if (process.argv[2] === "--check") {
  assert.equal(fs.readFileSync(new URL("../src/testdata/donors/zen-policy.json", import.meta.url), "utf8"), output);
  console.log(JSON.stringify({ cases: rows.length, exact: true, runtimeDependency: false }));
} else {
  assert.ok(process.argv[2], "explicit output path required");
  fs.writeFileSync(process.argv[2], output);
  console.log(JSON.stringify({ cases: rows.length, generated: true, output: process.argv[2] }));
}
