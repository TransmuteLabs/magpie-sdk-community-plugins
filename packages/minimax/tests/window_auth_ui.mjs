import assert from "node:assert/strict";
import { pathToFileURL } from "node:url";
import { readFile } from "node:fs/promises";

const distribution = process.argv[2];
const { mountAuthentication } = await import(pathToFileURL(`${distribution}/web/auth.js`));

class Element {
  value = "";
  textContent = "";
  disabled = false;
  hidden = false;
  readOnly = false;
  handlers = new Map();
  addEventListener(name, handler) { this.handlers.set(name, handler); }
  removeEventListener(name, handler) { if (this.handlers.get(name) === handler) this.handlers.delete(name); }
  async dispatch(name) { if (!this.disabled) await this.handlers.get(name)?.(); }
  focus() {}
  select() { this.selected = true; }
}

function fixture(auth) {
  const ids = ["region", "signin", "finish", "cancel", "status", "user-code", "auth-address", "auth-url", "auth-select"];
  const elements = Object.fromEntries(ids.map(id => [id, new Element()]));
  const controller = mountAuthentication({ getElementById: id => elements[id] }, { auth });
  return { elements, controller };
}

const waiting = {
  flow: "a".repeat(64), state: "waiting", url: "https://account.minimax.io/device",
  userCode: "GUI-CODE", retryAfterMs: 1000,
};
const calls = [];
const { elements, controller } = fixture({
  async start(binding, options) { calls.push(["start", binding, options]); return waiting; },
  async finish(binding, flow) {
    calls.push(["finish", binding, flow]);
    return { flow, state: "complete", account: { id: "saved-account", generation: "1" } };
  },
  async cancel(binding, flow) { calls.push(["cancel", binding, flow]); return { flow, state: "canceled" }; },
});
assert.equal(elements.signin.disabled, true);
await elements.signin.dispatch("click");
assert.equal(calls.length, 0);
elements.region.value = "minimax-code-global";
await elements.region.dispatch("change");
assert.equal(elements.signin.disabled, false);
await elements.signin.dispatch("click");
assert.deepEqual(calls[0], ["start", "page.auth.minimax-code-global", { method: "device" }]);
assert.equal(elements.region.disabled, true);
assert.equal(elements.signin.disabled, true);
assert.equal(elements["auth-url"].value, waiting.url);
assert.equal(elements["auth-url"].readOnly, true);
await elements["auth-select"].dispatch("click");
assert.equal(elements["auth-url"].selected, true);
assert.equal(elements["user-code"].textContent, "Device code: GUI-CODE");
elements.region.value = "minimax-code";
await elements.finish.dispatch("click");
assert.deepEqual(calls[1], ["finish", "page.auth.minimax-code-global", waiting.flow]);
assert.equal(elements.status.textContent, "Signed in: saved-account");
assert.equal(elements["auth-url"].value, "");
assert.equal(elements["user-code"].textContent, "");
await elements.finish.dispatch("click");
assert.equal(calls.length, 2);
await elements.signin.dispatch("click");
assert.equal(calls[2][1], "page.auth.minimax-code");
await elements.cancel.dispatch("click");
assert.equal(calls[3][0], "cancel");
assert.equal(elements.finish.disabled, true);
await controller.dispose();
assert.equal(elements["auth-url"].value, "");

let release;
const late = new Promise(resolve => { release = resolve; });
const revoked = [];
const held = fixture({
  start: () => late,
  async cancel(binding, flow) { revoked.push([binding, flow]); return { flow, state: "canceled" }; },
});
held.elements.region.value = "minimax-code";
await held.elements.region.dispatch("change");
const starting = held.elements.signin.dispatch("click");
await held.controller.dispose();
release(waiting);
await starting;
assert.deepEqual(revoked, [["page.auth.minimax-code", waiting.flow]]);
assert.equal(held.elements["auth-url"].value, "");
assert.equal(held.elements["user-code"].textContent, "");
assert.equal(held.elements.finish.disabled, true);

const html = await readFile(`${distribution}/web/index.html`, "utf8");
assert.match(html, /_magpie\/bridge\.js/);
assert.doesNotMatch(html, /type=["']password|window\.open|access_token|refresh_token|code_verifier|device_code/);
console.log(JSON.stringify({ passed: true, explicitRegion: true, firstAccount: true,
  hostFlowOnly: true, lateStartCanceled: true, secretsInPage: false, renderedBrowser: false }));
