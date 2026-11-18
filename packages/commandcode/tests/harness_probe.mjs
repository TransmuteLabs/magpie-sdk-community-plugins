import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { readFile } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import vm from "node:vm"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const harness = resolve(process.argv[2])
const lock = JSON.parse(await readFile(resolve(root, "docs/source-lock.json"), "utf8"))
const source = lock.sources.find((entry) => entry.id === "official-commandcode-harness")
for (const [name, digest] of Object.entries(source.files)) {
  assert.equal(createHash("sha256").update(await readFile(resolve(harness, name))).digest("hex"), digest, name)
}
const metadata = JSON.parse(await readFile(resolve(harness, "package.json"), "utf8"))
assert.equal(metadata.version, source.version)
const text = await readFile(resolve(harness, "dist/cli.mjs"), "utf8")
const extract = (name, next) => {
  const begin = text.indexOf("function " + name + "(")
  const end = text.indexOf("function " + next + "(", begin)
  assert.ok(begin >= 0 && end > begin, name)
  return text.slice(begin, end)
}
const messages = extract("toWireTools", "toWireSystem")
const complete = extract("createModelClient", "reasoningProviderOptions")
assert.match(text, /(?:,|;)Zr=5,/)
assert.match(text, /yE="\/alpha\/generate"/)
assert.doesNotMatch(complete, /tool_choice|toolChoice|parallel_tool_calls|disable_parallel_tool_use/)
const dispatched = []
let attempt = 0
let supportsThinking = true
let alwaysPause = false
const context = vm.createContext({
  __name: (value) => value, Map, ty: "internal-unused-tool", ey: "public-unused-tool",
  SE: "x-cmd-zdr", wE: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 },
  Zr: 5, yE: "/alpha/generate", vE: 64000,
  toWirePermissionMode: () => "standard", toWireThreadId: () => undefined,
  startChatSpan: () => ({ traceHeaders: {}, markFirstToken() {}, setResult() {}, recordError() {}, end() {} }),
  consumeStream: async () => {
    attempt++
    return { content: [{ type: "text", text: String(attempt) }],
      usage: { inputTokens: 10, outputTokens: 2, cacheReadTokens: 3, cacheWriteTokens: 1 },
      rawFinishReason: alwaysPause || attempt < 3 ? "pause_turn" : "end_turn", stopReason: "end_turn" }
  },
  normalizeStopReason: ({ raw }) => raw,
  Date,
})
vm.runInContext(messages + extract("addUsage3", "readStreamErrorEvent") + complete, context, { timeout: 1000 })
const effortMap = text.match(/(?:,|;)On=(\{off:!0,low:!0,medium:!0,high:!0,xhigh:!0,max:!0\})/)
assert.ok(effortMap)
vm.runInContext("var On=" + effortMap[1] + ";" + extract("isThinkingEffort", "apiEnvFromFlags"), context, { timeout: 1000 })
const converted = context.toWireMessages([
  { role: "assistant", content: [{ type: "thinking", thinking: "thought" },
    { type: "tool_use", id: "a", name: "read", input: { path: "x" } }] },
  { role: "user", content: [{ type: "tool_result", tool_use_id: "a", content: [
    { type: "text", text: "one" }, { type: "image", source: { media_type: "image/png", data: "AA==" } },
    { type: "text", text: "two" }] },
    { type: "image", source: { media_type: "image/png", data: "AA==" } }] },
])
assert.deepEqual(JSON.parse(JSON.stringify(converted)), [
  { role: "assistant", content: [{ type: "reasoning", text: "thought" },
    { type: "tool-call", toolCallId: "a", toolName: "read", input: { path: "x" } }] },
  { role: "tool", content: [{ type: "tool-result", toolCallId: "a", toolName: "read",
    output: { type: "text", value: "one\ntwo" } }] },
  { role: "user", content: [{ type: "image", image: "data:image/png;base64,AA==", mimeType: "image/png" }] },
])
const client = context.createModelClient({
  registry: { supportsVision: () => true, supportsThinking: () => supportsThinking },
  headers: async () => ({ "x-session-id": "synthetic-session" }),
  transport: { postStream: async (request) => { dispatched.push(request); return {} } },
})
const result = await client.complete({
  model: "fixture-model", effort: "high", system: "system",
  messages: [{ role: "user", content: [{ type: "text", text: "hi" }] }],
  tools: [], signal: new AbortController().signal,
})
assert.equal(dispatched.length, 3)
assert.equal(new Set(dispatched.map((request) => JSON.stringify(request.body))).size, 1)
assert.equal(new Set(dispatched.map((request) => JSON.stringify(request.headers))).size, 1)
assert.deepEqual(JSON.parse(JSON.stringify(result.usage)),
  { inputTokens: 30, outputTokens: 6, cacheReadTokens: 9, cacheWriteTokens: 3 })
assert.equal(result.content.length, 3)
const parameters = { model: "fixture-model", effort: "off", system: "system",
  messages: [{ role: "user", content: [{ type: "text", text: "hi" }] }], tools: [] }
await client.complete(parameters)
assert.equal(dispatched.at(-1).body.params.reasoning_effort, "off")
supportsThinking = false
await client.complete(parameters)
assert.equal(dispatched.at(-1).body.params.reasoning_effort, undefined)
alwaysPause = true
const before = dispatched.length
await client.complete(parameters)
assert.equal(dispatched.length - before, 6)
console.log(JSON.stringify({
  harnessVersion: source.version, harnessIntegrity: source.distribution.integrity,
  sha256: source.files["dist/cli.mjs"], sourceFunctionsExecutedInIsolation: true,
  completeHarnessExecuted: false, vendorRequests: 0,
  identicalBodyPauseContinuation: true, maxContinuations: 5, usageIsAdditive: true,
  nativeToolResultImagesSupported: false, nativeChoiceAndParallelControlsRecovered: false,
  reasoningAndUserImageWireVerified: true,
  nativeThinkingOffWireVerified: true, thinkingRequiredForNativeEffort: true,
  moduleRefusesExhaustionInsteadOfHarnessSuccess: true,
}))
