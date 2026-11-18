import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createHash, webcrypto } from "node:crypto";
import vm from "node:vm";
import { assertPublic } from "../src/testdata/window-auth/realms.mjs";

const config = JSON.parse(await readFile(process.argv[2], "utf8"));
const sha = bytes => createHash("sha256").update(bytes).digest("hex");
const check = value => assertPublic(value, config.secrets);
const base = config.origin + config.page.url.replace(/index\.html$/u, "");
const bridge = await (await fetch(base + "_magpie/bridge.js")).text();
const relay = await (await fetch(config.origin + "/extensionwindows.js", { headers: { Cookie: config.cookie } })).text();
const bundle = await (await fetch(base + "app.bundle.js")).text();
assert.equal(sha(bridge), config.bridgeSHA256);
assert.equal(sha(relay), config.parentSHA256);
assert.equal(sha(bundle), config.bundleSHA256);
class Element {
  value = "";
  textContent = "";
  disabled = false;
  hidden = false;
  children = [];
  handlers = new Map();
  dataset = {};
  addEventListener(name, handler) { this.handlers.set(name, [...(this.handlers.get(name) ?? []), handler]); }
  removeEventListener(name, handler) { this.handlers.set(name, (this.handlers.get(name) ?? []).filter(value => value !== handler)); }
  dispatch(name) { if (!this.disabled) for (const handler of this.handlers.get(name) ?? []) handler(); }
  removeAttribute(name) { delete this[name]; }
  replaceChildren(...children) { this.children = children; this.value = ""; }
}
const ids = ["account", "reload", "plan", "reset-window", "status", "output", "executions", "billing", "offers", "checkin",
  "resets", "claim", "reset", "confirm-dialog", "confirm-text", "confirm-cancel", "confirm-accept",
  "reset-opportunity", "reset-reconcile", "reset-ack", "reset-resolve", "opportunity-resolve",
  "signin", "reauthorize", "auth-poll", "auth-cancel", "auth-site", "auth-kind", "auth-status", "auth-link",
  "auth-organization", "auth-project", "auth-product", "auth-apiKeyId"];
const elements = Object.fromEntries(ids.map(id => [id, new Element()]));
elements["auth-site"].value = "zai";
elements["auth-kind"].value = "start";
const listeners = new Map(), parentProxy = {};
let crossings = 0, requests = 0;
const authViews = [];
const relayContext = vm.createContext({
  crypto: webcrypto, Uint8Array, AbortController, clearTimeout, setTimeout, t: text => text,
  api: async (route, body) => {
    check(body);
    requests++;
    const response = await fetch(config.origin + "/api/" + route, {
      method: "POST", headers: { Cookie: config.cookie, Origin: config.origin,
        "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json" }, body: JSON.stringify(body),
    });
    const text = await response.text();
    for (const secret of config.secrets) assert(!text.includes(secret), "private material in web response");
    const result = JSON.parse(text); check(result);
    if (route === "plugin-pages/auth") authViews.push({ status: response.status, state: result.state, error: result.error });
    if (!response.ok) throw new Error("Host refused");
    return result;
  },
});
const parent = new vm.SourceTextModule(relay, { context: relayContext });
await parent.link(() => { throw Error("Unexpected parent import"); });
await parent.evaluate();
const violations = [];
const owner = { entry: config.page };
parentProxy.postMessage = message => {
  void (async () => {
    check(message); crossings++;
    let reply;
    try {
      reply = { magpie: 1, id: message.id, result: await parent.namespace.answerPluginPage(
        owner, message, () => ({ dark: false, lang: "ru", tokens: {} }), () => "ZCode") };
    } catch { reply = { magpie: 1, id: message.id, error: "Host refused" }; }
    check(reply); crossings++;
    for (const listener of listeners.get("message") ?? []) listener({ source: parentProxy, data: structuredClone(reply) });
  })().catch(() => violations.push("public boundary failed"));
};
const context = vm.createContext({
  parent: parentProxy, URL, TextEncoder, Uint8Array, Blob, structuredClone, setTimeout, clearTimeout,
  location: { href: config.origin + config.page.url },
  document: {
    readyState: "loading", documentElement: { dataset: {}, style: { setProperty() {} } },
    head: { prepend() {} }, createElement: () => new Element(), getElementById: id => elements[id],
  },
  addEventListener(name, handler) { listeners.set(name, [...(listeners.get(name) ?? []), handler]); },
  fetch() { throw new Error("iframe direct HTTP is forbidden"); },
});
context.window = context;
vm.runInContext(bridge, context);
vm.runInContext(bundle, context);
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(predicate, label) {
  for (let i = 0; i < 150; i++) { if (predicate()) return; await sleep(40); }
  throw Error(label);
}
await until(() => elements.account.children.length > 0, "initial account enumeration");
assert.equal(elements.account.children.length, 1);
async function complete() {
  await until(() => !elements["auth-poll"].disabled, "start waiting");
  for (let i = 0; i < 5; i++) {
    await sleep(1050);
    elements["auth-poll"].dispatch("click");
    await until(() => !elements["auth-poll"].disabled || elements["auth-link"].hidden, "poll settled");
    if (elements["auth-link"].hidden) return;
  }
  throw Error("bounded production UI poll did not complete: " + JSON.stringify({
    authViews, status: elements["auth-status"].textContent,
  }));
}
elements.signin.dispatch("click");
await complete();
assert.match(elements["auth-status"].textContent, /Вход завершён/);
assert.equal(elements.account.children.length, 2);
assert.equal(elements.account.value, "");
const id = elements.account.children[1].value;
elements.account.value = id; elements.account.dispatch("change");
elements.reauthorize.dispatch("click");
await complete();
assert.equal(elements.account.value, "");
elements.account.value = id; elements.account.dispatch("change");
await fetch(config.vendor + "/__fixture__/mode", { method: "POST",
  headers: { "Content-Type": "application/json" }, body: JSON.stringify({ kind: "zcode", mode: "hold" }) });
elements.reauthorize.dispatch("click");
await until(() => !elements["auth-cancel"].disabled, "cancelable production flow");
await sleep(1050);
elements["auth-poll"].dispatch("click");
let held = false;
for (let i = 0; i < 100; i++) {
  const state = await (await fetch(config.vendor + "/__fixture__/state")).json();
  if (state.held > 0) { held = true; break; }
  await sleep(25);
}
assert(held, "actual vendor completion was not held");
elements["auth-cancel"].dispatch("click");
elements["auth-cancel"].dispatch("click");
await until(() => elements["auth-status"].textContent.includes("canceled"), "cancellation settled");
assert.match(elements["auth-status"].textContent, /canceled/);
const terminalStatus = elements["auth-status"].textContent;
for (const listener of listeners.get("pagehide") ?? []) listener();
await fetch(config.vendor + "/__fixture__/release", { method: "POST",
  headers: { "Content-Type": "application/json" }, body: "{}" });
await sleep(300);
assert.equal(elements["auth-status"].textContent, terminalStatus);
assert.equal(violations.length, 0);
console.log(JSON.stringify({ passed: true, actualPackagedIIFE: true, actualParentHTTPNativeStore: true,
  explicitReselection: true, canceledHeldThirdFlow: true, disposedLateResponseIgnored: true,
  crossings, requests, renderedBrowser: false }));
