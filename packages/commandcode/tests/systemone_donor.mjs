import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { readFile } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const donor = resolve(process.argv[2])
const lock = JSON.parse(await readFile(resolve(root, "docs/source-lock.json"), "utf8"))
const source = lock.sources.find((entry) => entry.id === "community-commandcode")
const fixtures = JSON.parse(await readFile(resolve(root, "docs/SYSTEMONE-SDK-FIXTURES.json"), "utf8"))
assert.equal(fixtures.sources["community-commandcode"].commit, source.commit)
assert.equal(fixtures.sources["community-commandcode"].version, source.version)
const fixtureDigest = createHash("sha256").update(await readFile(resolve(root, "docs/SYSTEMONE-SDK-FIXTURES.json"))).digest("hex")
assert.equal(fixtureDigest, lock.fixtures.find((entry) => entry.path === "docs/SYSTEMONE-SDK-FIXTURES.json").sha256)
for (const relative of ["packages/commandcode/index.mjs", "packages/commandcode/jev.test.mjs"]) {
  const digest = createHash("sha256").update(await readFile(resolve(donor, relative))).digest("hex")
  assert.equal(digest, source.files[relative], relative)
}
assert.equal(process.env.CMD_ZDR, undefined)
const { CommandCodePlugin, _internal } = await import(pathToFileURL(resolve(donor, "packages/commandcode/index.mjs")))
const { request: question, answer, aliases, headers: expectedHeaders } = fixtures.donorWire
const originalFetch = globalThis.fetch
const sent = []
const syntheticResponseHeaders = { "retry-after": "7", "x-request-id": "fixture-request" }
let reply = () => Response.json(answer, { headers: syntheticResponseHeaders })
globalThis.fetch = async (input, init = {}) => {
  assert.equal(input, "https://api.commandcode.ai/provider/v1/systemone")
  assert.equal(init.method, "POST")
  sent.push({ headers: new Headers(init.headers), body: JSON.parse(init.body) })
  return reply()
}

async function load(plan = "Pro", magpie, client) {
  const hooks = await CommandCodePlugin({ magpie, client })
  return hooks.auth.loader(async () => ({ type: "api", key: "synthetic-systemone-key", metadata: { plan } }))
}

async function ask(loader, body = question) {
  return loader.fetch("https://api.commandcode.ai/provider/v1/systemone", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "x-magpie-work-dir": encodeURIComponent("/synthetic/shop"),
      "x-magpie-work-git": "yes",
    },
    body: JSON.stringify(body),
  })
}

try {
  for (const alias of aliases) {
    const before = sent.length
    const response = await ask(await load(), { ...question, model: alias })
    assert.equal(response.status, 200)
    assert.equal(await response.text(), JSON.stringify(answer))
    for (const [name, value] of Object.entries(syntheticResponseHeaders)) {
      assert.equal(response.headers.get(name), value)
    }
    assert.equal(response.headers.get("x-magpie-sign-in"), "kept")
    assert.equal(sent.length, before + 1)
    const request = sent.at(-1)
    assert.deepEqual(request.body, question)
    const headers = Object.fromEntries(request.headers)
    assert.match(headers["x-session-id"], new RegExp(fixtures.donorWire.sessionHeaderPattern))
    assert.deepEqual(headers, {
      ...expectedHeaders,
      "x-session-id": headers["x-session-id"],
    })
  }
  const response = await ask(await load("Pro", { option: (provider, key) => provider === "commandcode-plan" && key === "zdr" }))
  assert.equal(response.status, 200)
  assert.equal(sent.at(-1).headers.get("x-cmd-zdr"), "1")
  const loader = await load()
  const beforeInvalid = sent.length
  const invalid = await loader.fetch("https://api.commandcode.ai/provider/v1/systemone", { method: "POST", body: "is this a test?" })
  assert.equal(invalid.status, 400)
  assert.equal(sent.length, beforeInvalid)
  reply = () => new Response('{"error":{"message":"MODEL_NOT_IN_PLAN: Jev available in Provider and above plans"}}', { status: 400 })
  const refused = await ask(await load())
  assert.equal(refused.status, 403)
  assert.equal(refused.headers.get("X-Magpie-Sign-In"), "kept")
  assert.equal((await refused.json()).error.message, "Model not in plan: Jev available in Provider and above plans")
  reply = () => new Response('{"error":{"message":"Your Go plan doesn\'t include API access. Upgrade to Provider or higher."}}', { status: 403 })
  const saved = []
  const goRefused = await ask(await load("", undefined, { auth: { set: async (entry) => saved.push(entry) } }))
  assert.equal(goRefused.status, 403)
  assert.equal(_internal.subsSeen.get("synthetic-systemone-key")?.sub?.plan, "Go")
  assert.equal(saved[0]?.body?.metadata?.plan, "Go")
  assert.equal(sent.length, 7)
  console.log(JSON.stringify({
    donorCommit: source.commit,
    donorVersion: source.version,
    wireSourceSHA256: source.files["packages/commandcode/index.mjs"],
    fixtureSourceSHA256: source.files["packages/commandcode/jev.test.mjs"],
    route: "POST /provider/v1/systemone",
    cliHeader: "1.88.2",
    aliases,
    canonicalRequest: question,
    preservedAnswer: answer,
    exactCLIHeaders: true,
    syntheticResponseHeadersPreserved: true,
    zdr: true,
    invalidJSONBeforeDispatch: true,
    planRefusal400To403: true,
    goRefusalKeepsAccount: true,
    fixtureDispatches: sent.length,
    pendingHostCases: fixtures.pendingHostAcceptance.length,
    pendingUsageCases: fixtures.pendingUsageAcceptance.length,
    pendingAgentCases: fixtures.pendingAgentAcceptance.length,
    proposedHostCasesExecuted: 0,
    vendorCalls: 0,
  }))
} finally {
  globalThis.fetch = originalFetch
  _internal.subsSeen.clear()
  _internal.extraSeen.clear()
}
