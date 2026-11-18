import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { webcrypto, createHash } from "node:crypto";
import vm from "node:vm";

const config = JSON.parse(await readFile(process.argv[2], "utf8"));
const transcript = [], violations = [];
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const check = value => {
  if (/PRIVATE-|credentials|continuation|device_code/iu.test(JSON.stringify(value))) {
    violations.push("private material crossed iframe boundary");
    throw new Error("Private material reached browser");
  }
  return structuredClone(value);
};
async function wait(predicate, message) {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await sleep(15);
  }
  throw new Error(message);
}
class Element {
  value = ""; textContent = ""; disabled = false; hidden = false; children = []; listeners = new Map();
  dataset = {}; style = { setProperty() {} };
  addEventListener(name, listener) { this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener]); }
  removeEventListener(name, listener) { this.listeners.set(name, (this.listeners.get(name) ?? []).filter(item => item !== listener)); }
  replaceChildren(...children) { this.children = children; }
  removeAttribute(name) { delete this[name]; }
  dispatch(name) { if (!this.disabled || name !== "click") for (const listener of this.listeners.get(name) ?? []) listener(); }
}
class Option extends Element {
  constructor(label, value) { super(); this.textContent = label; this.value = value; }
}
const elements = Object.fromEntries(["provider", "method", "key", "key-label", "account", "status", "code", "auth-address", "auth-url", "auth-select", "reload", "inspect", "signin", "reauthorize", "poll", "cancel"].map(id => [id, new Element()]));
elements.provider.value = "go";
elements.method.value = "device";
const listeners = new Map(), parentProxy = {}, sent = [], replies = [];
const modules = new Map();
const parentContext = vm.createContext({
  crypto: webcrypto, Uint8Array, clearTimeout, t: value => value,
  api: async (route, body) => {
    check(body);
    const response = await fetch(config.origin + "/api/" + route, {
      method: "POST", headers: { Cookie: config.cookie, Origin: config.origin, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const text = await response.text();
    check(text);
    const result = JSON.parse(text);
    transcript.push({ route, status: response.status, result: check(result) });
    if (!response.ok) throw new Error("Host refused window request");
    return result;
  },
});
const parentSource = await (await fetch(config.origin + "/extensionwindows.js", { headers: { Cookie: config.cookie } })).text();
const parentModule = new vm.SourceTextModule(parentSource, { context: parentContext });
await parentModule.link(() => { throw new Error("Unexpected parent dependency"); });
await parentModule.evaluate();
const owner = { entry: config.page };
async function answer(message, deliver = true) {
  check(message);
  let result;
  try {
    result = { magpie: 1, id: message.id, result: check(await parentModule.namespace.answerPluginPage(owner, message,
      () => ({ dark: false, lang: "ru", tokens: {} }), () => "OpenCode Go")) };
  } catch { result = { magpie: 1, id: message.id, error: "Host refused window request" }; }
  replies.push(check(result));
  if (deliver) for (const listener of listeners.get("message") ?? []) listener({ source: parentProxy, data: result });
  return result;
}
parentProxy.postMessage = message => { sent.push(check(message)); void answer(message); };
const document = {
  readyState: "loading", documentElement: new Element(), head: { prepend() {} },
  createElement: () => new Element(), getElementById: id => elements[id],
};
const context = vm.createContext({
  parent: parentProxy, Option, TextEncoder, URL, Blob, Uint8Array, setTimeout, clearTimeout,
  location: { href: config.origin + config.page.url },
  document, addEventListener(name, listener) { listeners.set(name, [...(listeners.get(name) ?? []), listener]); },
  fetch() { throw new Error("iframe cannot use direct HTTP"); },
});
context.window = context;
const bridgeURL = new URL("_magpie/bridge.js", config.origin + config.page.url);
const bridge = await (await fetch(bridgeURL)).text();
vm.runInContext(bridge, context);
const base = new URL(config.page.url, config.origin);
async function load(address) {
  if (modules.has(address)) return modules.get(address);
  assert(address.startsWith(new URL(".", base).href));
  const response = await fetch(address);
  assert(response.ok, "packaged relative dependency missing");
  const source = await response.text();
  const module = new vm.SourceTextModule(source, { context, identifier: address });
  modules.set(address, module);
  await module.link((name, parent) => {
    assert(name.startsWith("./"), "bare or privileged browser dependency");
    return load(new URL(name, parent.identifier).href);
  });
  return module;
}
const app = await load(new URL("./app.js", base).href);
await app.evaluate();
const sdk = modules.get(new URL("./sdk-ui.js", base).href).namespace;
const click = id => { assert(!elements[id].disabled, id + " unexpectedly disabled"); elements[id].dispatch("click"); };
const currentFlow = () => [...transcript].reverse().find(entry => entry.route === "plugin-pages/auth" && entry.status === 200)?.result;
async function pollComplete() {
  for (let attempt = 0; attempt < 6; attempt++) {
    const flow = currentFlow();
    if (flow?.state === "complete") return flow;
    await sleep(Math.max(flow?.retryAfterMs ?? 1000, 1000) + 30);
    const before = transcript.length;
    click("poll");
    await wait(() => transcript.length > before && (currentFlow()?.state === "complete" || !elements.poll.disabled), "auth poll never settled");
  }
  throw new Error("auth never completed");
}
try {
  await wait(() => !elements.reload.disabled && transcript.some(item => item.route.endsWith("accounts")), "UI never became ready");
  let first, second, zen;
  if (!config.zenOnly) {
  assert.equal(elements.account.value, "");
  assert.equal(elements.inspect.disabled, true);
  click("signin");
  await wait(() => !elements.poll.disabled, "device start did not reach UI");
  const start = sent.find(item => item.method === "auth.start");
  assert.equal((await answer(start, false)).result.flow, currentFlow().flow);
  first = await pollComplete();
  assert.equal(first.account.generation, "1");
  assert.equal((await answer(start, false)).result.state, "complete");
  await wait(() => elements.account.children.length === 2 && !elements.signin.disabled, "completion did not refresh accounts");
  assert.equal(elements.account.value, "");
  assert.equal(elements.inspect.disabled, true);
  elements.account.value = elements.account.children[1].value;
  elements.account.dispatch("change");
  const account = elements.account.value;
  assert.equal(account, first.account.id);
  const inspected = transcript.length;
  click("inspect");
  await wait(() => transcript.length > inspected && !elements.inspect.disabled, "selected-account action failed");
  click("reauthorize");
  await wait(() => !elements.poll.disabled, "reauthorization did not start");
  second = await pollComplete();
  assert.equal(second.account.id, account);
  assert.equal(second.account.generation, "2");
  await wait(() => elements.account.children[1]?.textContent.endsWith("2") && !elements.signin.disabled, "new generation not discovered");
  assert.equal(elements.account.value, "");
  await assert.rejects(sdk.ui.backend.call("account.info", null, { account: { id: account, expectedGeneration: "1" } }));
  await assert.rejects(sdk.ui.backend.call("account.info", null, { account: { id: "foreign", expectedGeneration: "1" } }));
  elements.account.value = "foreign";
  elements.account.dispatch("change");
  assert.equal(elements.inspect.disabled, true);
  elements.account.value = account;
  elements.account.dispatch("change");
  await fetch(config.vendor + "/__hold", { method: "POST" });
  click("reauthorize");
  await wait(() => !elements.poll.disabled, "cancel fixture failed to start");
  await sleep(Math.max(currentFlow().retryAfterMs ?? 1000, 1000) + 30);
  click("poll");
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    if ((await (await fetch(config.vendor + "/__state")).json()).held) break;
    await sleep(15);
  }
  assert((await (await fetch(config.vendor + "/__state")).json()).held);
  click("cancel");
  await wait(() => currentFlow()?.state === "canceled", "cancel was not durable");
  await fetch(config.vendor + "/__release", { method: "POST" });
  await wait(() => !elements.signin.disabled, "held poll did not drain");
  assert.equal(elements.code.textContent, "");
  assert.equal(elements["auth-address"].hidden, true);
  assert.equal(elements["auth-url"].value, "");
  } else {
    const selectProvider = async value => {
      const before = transcript.length;
      elements.provider.value = value;
      elements.provider.dispatch("change");
      await wait(() => transcript.length > before && !elements.reload.disabled, "provider account list did not settle");
      assert.equal(elements.account.value, "");
    };
    await selectProvider("zen");
    elements.method.value = "key";
    elements.method.dispatch("change");
    elements.key.value = "SYNTHETIC-ZEN-SERVICE";
    click("signin");
    assert.equal(elements.key.value, "", "service key remained in the input after start");
    await wait(() => !elements.poll.disabled, "key flow did not start");
    const keyed = await pollComplete();
    await wait(() => !elements.signin.disabled, "key completion did not settle");
    const selectedKey = { id: keyed.account.id, expectedGeneration: keyed.account.generation };
    const snapshot = await sdk.ui.backend.call("console.snapshot", {}, { account: selectedKey });
    assert.equal(snapshot.lists, true);
    assert.equal(snapshot.watches, true);
    assert.equal(snapshot.instructions[0].content, "Use tabs.");
    assert.equal(snapshot.skills[0].slug, "deploy");
    const bundle = await sdk.ui.backend.call("console.skill", { slug: "deploy" }, { account: selectedKey });
    assert.equal(bundle.files[0].path, "SKILL.md");
    const types = await sdk.ui.backend.call("console.watch.types", {}, { account: selectedKey });
    assert.equal(types.types[0].name, "deploy.finished");
    assert.equal(types.types[0].description, "A deploy finished.");
    assert.equal(types.types[0].inputSchema.type, "object");
    const page = await sdk.ui.backend.call("console.watch.poll", { cursor: "opaque+/== &" }, { account: selectedKey });
    assert.equal(page.events.length, 1);
    assert.equal((await sdk.ui.backend.call("console.watch.ack", { receipt: page.receipt, ids: page.events.map(event => event.id) }, { account: selectedKey })).acknowledged, 1);
    await assert.rejects(sdk.ui.backend.call("console.watch.ack", { receipt: page.receipt, ids: ["ev_1"] }, { account: selectedKey }));
    elements.method.value = "device";
    elements.method.dispatch("change");
    click("signin");
    await wait(() => !elements.poll.disabled, "Zen OAuth did not start");
    const oauth = await pollComplete();
    await wait(() => !elements.signin.disabled, "Zen OAuth completion did not settle");
    assert.notEqual(oauth.account.id, keyed.account.id);
    await selectProvider("public");
    assert.equal(elements.method.value, "public");
    click("signin");
    await wait(() => !elements.poll.disabled, "public flow did not start");
    const publicFlow = await pollComplete();
    await wait(() => !elements.signin.disabled, "public completion did not settle");
    assert.notEqual(publicFlow.account.id, oauth.account.id);
    assert.notEqual(publicFlow.account.id, keyed.account.id);
    await assert.rejects(sdk.ui.backend.call("console.snapshot", {}, { account: { id: publicFlow.account.id, expectedGeneration: publicFlow.account.generation } }));
    await assert.rejects(sdk.ui.backend.call("zen.account.info", null, { account: { id: publicFlow.account.id, expectedGeneration: publicFlow.account.generation } }));
    assert(!JSON.stringify(replies).includes("SYNTHETIC-ZEN-SERVICE"), "service key reached a browser result");
    for (const message of sent) {
      if (message.input?.key) message.input.key = "[authentication input redacted]";
    }
    zen = { key: keyed.account, oauth: oauth.account, public: publicFlow.account, consoleSnapshot: true, watchTypes: true,
      skillBundle: true, httpWatchPoll: true, ackReplayRefused: true, foreignProviderRefused: true, keyInputCleared: true };
  }
  assert.equal(violations.length, 0);
  const result = {
    initial: first?.account, reauthorized: second?.account, zen, cancellationPublished: false,
    messages: sent.length, httpCalls: transcript.length, privateViolations: violations.length,
    actualPackagedUI: true, actualParentBridge: true, renderedBrowser: false,
    bridgeSHA256: createHash("sha256").update(bridge).digest("hex"),
  };
  await writeFile(config.output, JSON.stringify({ result, transcript, sent, replies }, null, 2) + "\n");
  console.log(JSON.stringify(result));
} finally {
  await fetch(config.vendor + "/__release", { method: "POST" });
  for (const listener of listeners.get("message") ?? []) listener({ source: parentProxy, data: { magpie: 1, event: "dispose" } });
}
