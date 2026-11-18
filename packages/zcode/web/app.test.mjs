import assert from "node:assert/strict";
import path from "node:path";
import { readFile } from "node:fs/promises";
import vm from "node:vm";
import test from "node:test";

class Element {
  textContent = "";
  value = "";
  disabled = false;
  children = [];
  handlers = new Map();
  returnValue = "";
  open = false;
  addEventListener(name, handler, options) {
    const handlers = this.handlers.get(name) ?? [];
    handlers.push({ handler, once: options?.once });
    this.handlers.set(name, handlers);
  }
  removeEventListener(name, handler) {
    this.handlers.set(name, (this.handlers.get(name) ?? []).filter((value) => value.handler !== handler));
  }
  dispatch(name) {
    const handlers = [...(this.handlers.get(name) ?? [])];
    for (const value of handlers) {
      if (value.once) this.removeEventListener(name, value.handler);
      value.handler();
    }
  }
  replaceChildren(...values) { this.children = values; this.value = ""; }
  showModal() { assert.equal(this.open, false); this.open = true; }
  close(value) { this.returnValue = value; this.open = false; this.dispatch("close"); }
  removeAttribute(name) { delete this[name]; }
}

test("actual compiled app uses an HTML dialog through the public browser bridge", async () => {
  const source = process.env.ZCODE_PAGE_BUILD_DIR;
  const ids = ["account", "reload", "plan", "reset-window", "status", "output",
    "executions", "billing", "resets", "offers", "checkin", "claim", "reset",
    "reset-opportunity", "reset-reconcile", "reset-ack", "reset-resolve", "opportunity-resolve",
    "confirm-dialog", "confirm-text", "confirm-cancel", "confirm-accept",
    "signin", "reauthorize", "auth-poll", "auth-cancel", "auth-site", "auth-kind",
    "auth-status", "auth-link", "auth-organization", "auth-project", "auth-product", "auth-apiKeyId",
    "native-session", "native-command", "native-model", "native-text", "native-run", "native-stop", "native-status"];
  const elements = Object.fromEntries(ids.map((id) => [id, new Element()]));
  const calls = [];
  let finishNative;
  const callbacks = {};
  const previousDocument = globalThis.document;
  const previousWindow = globalThis.window;
  globalThis.document = {
    documentElement: { dataset: {} },
    getElementById: (name) => elements[name],
    createElement: () => new Element(),
  };
  globalThis.window = {
    confirm() { throw new Error("sandboxed native dialog must not be used"); },
    addEventListener: (event, handler) => { callbacks[event] = handler; },
    magpie: {
      ready: Promise.resolve({ dark: true }),
      theme: { dark: true },
      onTheme: (listener) => { callbacks.theme = listener; },
      backend: {
        accounts: async () => [{ id: "fixture", generation: "9007199254740993" }],
        call: async (...args) => {
          calls.push(args);
          if (args[0] === "page.native-run") {
            return new Promise((resolve) => { finishNative = resolve; });
          }
          return { outcome: "confirmed" };
        },
      },
      config: { get() {}, cas() {} },
    },
  };
  try {
    vm.runInThisContext(await readFile(path.join(source, "app.bundle.js"), "utf8"));
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(document.documentElement.dataset.theme, "dark");
    callbacks.theme({ dark: false });
    assert.equal(document.documentElement.dataset.theme, "light");
    elements.account.value = "fixture";
    elements.account.dispatch("change");
    elements.plan.value = "declared-plan";
    elements.claim.dispatch("click");
    assert.equal(elements["confirm-dialog"].open, true);
    assert.equal(elements.account.disabled, true);
    assert.match(elements["confirm-text"].textContent, /declared-plan.*fixture/);
    assert.equal(calls.length, 0);
    elements["confirm-cancel"].dispatch("click");
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(calls.length, 0);
    elements.claim.dispatch("click");
    elements["confirm-accept"].dispatch("click");
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(calls, [["page.billing-claim", { planId: "declared-plan" }, {
      account: { id: "fixture", expectedGeneration: "9007199254740993" },
    }]]);
    elements["native-session"].value = "session-ui";
    elements["native-command"].value = "command-ui";
    elements["native-model"].value = "provider/model";
    elements["native-text"].value = "one input";
    elements["native-run"].dispatch("click");
    assert.equal(elements["confirm-dialog"].open, true);
    assert.match(elements["confirm-text"].textContent, /session-ui.*fixture/);
    elements["confirm-accept"].dispatch("click");
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(elements.account.disabled, true);
    assert.equal(elements["native-stop"].disabled, false);
    elements["native-stop"].dispatch("click");
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(calls.slice(1), [
      ["page.native-run", { sessionId: "session-ui", commandId: "command-ui",
        model: "provider/model", text: "one input" },
        { account: { id: "fixture", expectedGeneration: "9007199254740993" } }],
      ["page.native-stop", { sessionId: "session-ui", commandId: "command-ui" },
        { account: { id: "fixture", expectedGeneration: "9007199254740993" } }],
    ]);
    finishNative({ outcome: "completed", state: "completedInterrupted", joined: true });
    await new Promise((resolve) => setImmediate(resolve));
    callbacks.pagehide();
    elements.claim.dispatch("click");
    elements["native-run"].dispatch("click");
    assert.equal(calls.length, 3);
    const html = await readFile(path.join(source, "index.html"), "utf8");
    assert.equal(/<form\b|window\.confirm/u.test(html), false);
    assert.match(html, /src="_magpie\/bridge\.js"/);
    assert.match(html, /src="\.\/app\.bundle\.js" defer/);
    assert.equal(/type="module"/u.test(html), false);
  } finally {
    globalThis.document = previousDocument;
    globalThis.window = previousWindow;
  }
});
