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
  handlers = new Map();
  addEventListener(name, handler) { this.handlers.set(name, handler); }
  removeEventListener(name, handler) { if (this.handlers.get(name) === handler) this.handlers.delete(name); }
  async dispatch(name) { if (!this.disabled) await this.handlers.get(name)?.(); }
  focus() {}
  select() { this.selected = true; }
}

function fixture(auth) {
  const ids = ["signin", "finish", "cancel", "status", "auth-address", "auth-url", "auth-select"];
  const elements = Object.fromEntries(ids.map(id => [id, new Element()]));
  const controller = mountAuthentication({ getElementById: id => elements[id] }, { auth });
  return { elements, controller };
}

const waiting = {
  flow: "a".repeat(64), state: "waiting",
  url: "https://app.devin.ai/auth/cli/continue?state=synthetic", retryAfterMs: 1000,
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
assert.equal(elements.signin.disabled, false);
await elements.signin.dispatch("click");
assert.deepEqual(calls[0], ["start", "page.auth.devin", { method: "browser" }]);
assert.equal(elements.signin.disabled, true);
assert.equal(elements["auth-url"].value, waiting.url);
assert.equal(elements["auth-url"].readOnly, true);
await elements["auth-select"].dispatch("click");
assert.equal(elements["auth-url"].selected, true);
await elements.finish.dispatch("click");
assert.deepEqual(calls[1], ["finish", "page.auth.devin", waiting.flow]);
assert.equal(elements.status.textContent, "Signed in: saved-account");
assert.equal(elements["auth-url"].value, "");
await elements.finish.dispatch("click");
assert.equal(calls.length, 2);
await elements.signin.dispatch("click");
await elements.cancel.dispatch("click");
assert.equal(calls[3][0], "cancel");
assert.equal(elements.finish.disabled, true);
await controller.dispose();
assert.equal(elements["auth-url"].value, "");

let release;
const late = new Promise(resolve => { release = resolve; });
const canceled = [];
const held = fixture({
  start: () => late,
  async cancel(binding, flow) { canceled.push([binding, flow]); return { flow, state: "canceled" }; },
});
const starting = held.elements.signin.dispatch("click");
await held.controller.dispose();
release(waiting);
await starting;
assert.deepEqual(canceled, [["page.auth.devin", waiting.flow]]);
assert.equal(held.elements["auth-url"].value, "");
assert.equal(held.elements.finish.disabled, true);
const html = await readFile(`${distribution}/web/index.html`, "utf8");
assert.match(html, /_magpie\/bridge\.js/);
assert.doesNotMatch(html, /type=["']password|window\.open|code_verifier|callbackURL|sessionToken/);
console.log(JSON.stringify({ passed: true, firstAccount: true, hostFlowOnly: true,
  lateStartCanceled: true, callbackInput: false, secretsInPage: false }));
