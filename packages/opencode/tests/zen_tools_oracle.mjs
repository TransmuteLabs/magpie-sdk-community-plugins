import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { guardToolResponse } from "../src/testdata/donors/zen-tool-response.mjs";
import { toolReply } from "../src/testdata/donors/tool-fixtures.mjs";

const source = fs.readFileSync(new URL("../src/testdata/donors/zen-tool-request.js.txt", import.meta.url), "utf8");
const normalize = vm.runInNewContext(source + "\n(body, protocol) => normalizeTools(body, protocol)", {}, { timeout: 1000 });
const rows = [];
const event = (data) => `data: ${JSON.stringify(data)}\n\n`;
function request(protocol, names) {
  const tools = names.map((name) => {
    const definition = { name, description: `fixture ${name}`, parameters: { type: "object" } };
    if (protocol === "chat") return { type: "function", function: definition };
    if (protocol === "anthropic") return { name, description: definition.description, input_schema: definition.parameters };
    return { type: "function", ...definition };
  });
  const body = { tools, model: "fixture-free" };
  if (protocol === "chat") {
    body.messages = [{ role: "assistant", function_call: { name: names[0], arguments: "{}" },
      tool_calls: [{ id: "old", type: "function", function: { name: names[0], arguments: "{}" } }] }];
    body.tool_choice = { type: "function", function: { name: names[0] } };
    body.function_call = { name: names[0] };
  } else if (protocol === "responses") {
    body.input = [{ type: "function_call", call_id: "old", name: names[0], arguments: "{}" },
      { type: "function_call_output", call_id: "old", output: "run_code stays text" }];
    body.tool_choice = { type: "function", name: names[0] };
  } else {
    body.messages = [{ role: "assistant", content: [{ type: "tool_use", id: "old", name: names[0], input: {} }] }];
    body.tool_choice = { type: "tool", name: names[0] };
  }
  return body;
}
async function record(name, protocol, body, reply, streaming) {
  const input = JSON.stringify(body);
  const policy = normalize(body, protocol);
  const expectedRequest = JSON.stringify(body);
  const response = await guardToolResponse(new Response(reply, {
    headers: { "content-type": streaming ? "text/event-stream" : "application/json" },
  }), protocol, policy);
  assert.equal(response.status, 200);
  const expectedReply = await response.text();
  assert.doesNotMatch(expectedReply, /Invalid upstream|Invalid JSON|Unsupported upstream/);
  rows.push({ name, protocol: protocol === "anthropic" ? "messages" : protocol,
    request: input, expectedRequest, reply, expectedReply, streaming });
}
for (const protocol of ["chat", "responses", "anthropic"]) {
  for (const names of [
    ["run_code"], ["run_code", "exec_program", "exec_program_2"],
    ["RUN_CODE", "Run_Code", "host_run_code", "run_code_v2", "runCode", "code_run"],
    ["ReportFindings", "ScheduleWakeup", "findings_report", "mcp__ide__terminal_run", "RunTerminal"],
    ["read_image", "working_directory", "subagent_fork", "terminal_open"],
    ["CreateWorkflow", "GetWorkflowRun", "AmendWorkflow", "FillWorkflowHole"],
    ["ReportFindings"], ["working_directory"], ["Bash", "todo_write", "web-fetch", "___"],
    ["Bash", "bash", "todo_write", "todo-write"],
  ]) {
    for (const streaming of [false, true]) {
      const body = request(protocol, names);
      const shaped = JSON.parse(JSON.stringify(body));
      const policy = normalize(shaped, protocol);
      const served = [...policy.renamed].find(([, original]) => original === names[0])?.[0] ?? names[0];
      await record(`${protocol}/${names.join(",")}/${streaming}`, protocol, body,
        toolReply(protocol, served, streaming), streaming);
    }
  }
  for (const name of ["bash", "todowrite", "webfetch", "unknown_tool", "", "shell"]) {
    for (const streaming of [false, true]) {
      const body = request(protocol, ["Bash", "todo_write", "web-fetch"]);
      await record(`${protocol}/reply/${name || "missing"}/${streaming}`, protocol, body, toolReply(protocol, name, streaming), streaming);
    }
  }
}
const fragmented = [
  event({ choices: [{ index: 0, delta: { role: "assistant", content: "préface" } }] }),
  event({ choices: [{ index: 0, delta: { tool_calls: [{ index: 0, id: "first", type: "function", function: { name: "exec_", arguments: "{" } }] } }] }),
  event({ choices: [{ index: 0, delta: { tool_calls: [{ index: 0, id: "", function: { name: "program", arguments: "\"value\":9007199254740993}" } }] } }] }),
  event({ choices: [{ index: 0, delta: { tool_calls: [{ index: 0, id: "second", type: "function", function: { name: "Bash", arguments: "{}" } }] } }] }),
  event({ choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }] }),
  event({ choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }], usage: { prompt_tokens: 2 } }),
  "data: [DONE]\n\n",
].join("");
await record("chat/fragmented-colliding-index", "chat", request("chat", ["run_code", "Bash"]), fragmented, true);
const missing = [
  event({ choices: [{ index: 0, delta: { tool_calls: [{ index: 0, id: "missing", function: { arguments: "{}" } }] } }] }),
  event({ choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }] }),
  "data: [DONE]\n\n",
].join("");
await record("chat/fragmented-missing-name", "chat", request("chat", ["run_code"]), missing, true);
const legacy = request("chat", ["Read"]);
legacy.functions = [{ name: "run_code", parameters: { type: "object" } }];
legacy.function_call = { name: "run_code" };
await record("chat/legacy-functions", "chat", legacy, JSON.stringify({
  choices: [{ message: { function_call: { name: "exec_program", arguments: "{}" } }, finish_reason: "function_call" }],
}), false);
const output = JSON.stringify(rows, null, 2) + "\n";
if (process.argv[2] === "--check") {
  assert.equal(fs.readFileSync(new URL("../src/testdata/donors/zen-tools.json", import.meta.url), "utf8"), output);
  console.log(JSON.stringify({ donorCases: rows.length, exact: true, runtimeDependency: false }));
} else {
  fs.writeFileSync(new URL("../src/testdata/donors/zen-tools.json", import.meta.url), output);
  console.log(JSON.stringify({ donorCases: rows.length, generated: true }));
}
