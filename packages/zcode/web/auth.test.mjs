import assert from "node:assert/strict";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";

const { mountAuthentication } = await import(pathToFileURL(path.join(process.env.ZCODE_PAGE_BUILD_DIR, "auth.js")));
const tick = () => new Promise(resolve => setImmediate(resolve));
class Element {
  value = "";
  textContent = "";
  disabled = false;
  hidden = false;
  handlers = new Map();
  addEventListener(name, handler) { this.handlers.set(name, handler); }
  removeEventListener(name, handler) { if (this.handlers.get(name) === handler) this.handlers.delete(name); }
  removeAttribute(name) { delete this[name]; }
  click() { if (!this.disabled) this.handlers.get("click")?.(); }
}
function fixture(options = {}) {
  const previous = globalThis.window;
  const elements = Object.fromEntries(["signin", "reauthorize", "auth-poll", "auth-cancel",
    "auth-site", "auth-kind", "auth-status", "auth-link",
    "auth-organization", "auth-project", "auth-product", "auth-apiKeyId"].map(id => [id, new Element()]));
  elements["auth-site"].value = "zai";
  elements["auth-kind"].value = "start";
  const calls = [], locks = [];
  let reloads = 0;
  const waiting = { flow: "a".repeat(64), state: "waiting", url: "https://signin.example/verify", retryAfterMs: 1000 };
  globalThis.window = { magpie: {
    backend: { accounts: async () => [{ id: "chosen", generation: "9007199254740993" }], call: async () => null },
    auth: {
      start: async (...args) => { calls.push(args); return waiting; },
      finish: async () => ({ flow: waiting.flow, state: "complete", account: { id: "chosen", generation: "9007199254740994" } }),
      cancel: async () => ({ flow: waiting.flow, state: "canceled" }),
      ...options.auth,
    },
  } };
  const ui = mountAuthentication({ getElementById: id => elements[id] }, {
    selection: () => options.selection,
    canAuthenticate: () => options.canAuthenticate !== false,
    reload: async () => { reloads++; },
    lockAuthentication: value => locks.push(value),
  });
  return { elements, calls, locks, reloads: () => reloads, dispose: ui.dispose,
    close() { ui.dispose(); globalThis.window = previous; } };
}

test("real SDK workflow drives fixed browser auth, manual polling and completion refresh", async () => {
  const f = fixture();
  try {
    f.elements.signin.click();
    f.elements.signin.click();
    await tick();
    assert.deepEqual(f.calls, [["page.signin", { method: "browser", input: { site: "zai", kind: "start" } }]]);
    assert.equal(f.elements["auth-link"].href, "https://signin.example/verify");
    assert.equal(f.elements.signin.disabled, true);
    assert.equal(f.locks.at(-1), true);
    f.elements["auth-poll"].click();
    await tick();
    assert.equal(f.reloads(), 1);
    assert.equal(f.locks.at(-1), false);
    assert.equal(f.elements["auth-link"].hidden, true);
    assert.match(f.elements["auth-status"].textContent, /Выберите счёт заново/);
    assert(!JSON.stringify(f.elements).includes("credentials"));
  } finally { f.close(); }
});

test("China login is a channel within BigModel and cannot retarget reauthorization", async () => {
  const f = fixture();
  try {
    f.elements["auth-site"].value = "zai_cn";
    f.elements["auth-kind"].value = "auto";
    f.elements.signin.click();
    await tick();
    assert.deepEqual(f.calls, [["page.signin", { method: "browser", input: {
      site: "bigmodel", kind: "auto", loginProvider: "zai_cn",
    } }]]);
  } finally { f.close(); }
});

test("reauthorization preserves exact selected generation and omits editable plan fields", async () => {
  const f = fixture({ selection: { id: "chosen", expectedGeneration: "9007199254740993" } });
  try {
    f.elements["auth-project"].value = "cannot-retarget";
    f.elements.reauthorize.click();
    await tick();
    assert.deepEqual(f.calls, [["page.signin", { method: "browser", account: {
      id: "chosen", expectedGeneration: "9007199254740993",
    } }]]);
    f.elements["auth-cancel"].click();
    await tick();
    assert.equal(f.elements.signin.disabled, false);
    assert.equal(f.locks.at(-1), false);
  } finally { f.close(); }
});

test("stale selection never silently adopts current host generation for reauthorization", async () => {
  const selected = { id: "chosen", expectedGeneration: "9007199254740992" };
  const f = fixture({ selection: selected });
  try {
    f.elements.reauthorize.click();
    await tick();
    assert.equal(f.calls.length, 0);
    assert.match(f.elements["auth-status"].textContent, /не подтверждён/);
    selected.expectedGeneration = "9007199254740993";
    f.elements.reauthorize.click();
    await tick();
    assert.equal(f.calls.length, 1);
  } finally { f.close(); }
});

test("lost start reply disables fresh exchange; disposal removes handlers without retry", async () => {
  let calls = 0;
  const f = fixture({ auth: { start: async () => { calls++; throw new Error("private vendor detail"); } } });
  try {
    f.elements.signin.click();
    await tick();
    f.elements.signin.click();
    assert.equal(calls, 1);
    assert.equal(f.elements.signin.disabled, true);
    assert(!f.elements["auth-status"].textContent.includes("private"));
  } finally { f.close(); }
  assert.equal(f.elements.signin.handlers.size, 0);
});

test("authentication cannot overlap a financial confirmation or write", async () => {
  const f = fixture({ canAuthenticate: false });
  try {
    f.elements.signin.click();
    await tick();
    assert.equal(f.calls.length, 0);
    assert.match(f.elements["auth-status"].textContent, /Дождитесь/);
  } finally { f.close(); }
});

test("cancellation while finish is held coalesces and never reloads a canceled result", async () => {
  let finish, cancels = 0;
  const f = fixture({ auth: {
    finish: () => new Promise(resolve => { finish = resolve; }),
    cancel: async () => { cancels++; return { flow: "a".repeat(64), state: "canceled" }; },
  } });
  try {
    f.elements.signin.click();
    await tick();
    f.elements["auth-poll"].click();
    f.elements["auth-cancel"].click();
    f.elements["auth-cancel"].click();
    await tick();
    assert.equal(cancels, 1);
    assert.equal(f.locks.at(-1), true);
    finish({ flow: "a".repeat(64), state: "pending" });
    await tick();
    assert.equal(f.reloads(), 0);
    assert.equal(f.locks.at(-1), false);
    assert.match(f.elements["auth-status"].textContent, /canceled/);
  } finally { f.close(); }
});

test("disposing with a held completion prevents late reload, rendering and business unlock", async () => {
  let finish;
  const f = fixture({ auth: { finish: () => new Promise(resolve => { finish = resolve; }) } });
  try {
    f.elements.signin.click();
    await tick();
    f.elements["auth-poll"].click();
    await tick();
    f.dispose();
    const previousLocks = [...f.locks], previousStatus = f.elements["auth-status"].textContent;
    finish({ flow: "a".repeat(64), state: "complete", account: { id: "chosen", generation: "2" } });
    await tick();
    assert.equal(f.reloads(), 0);
    assert.deepEqual(f.locks, previousLocks);
    assert.equal(f.elements["auth-status"].textContent, previousStatus);
  } finally { f.close(); }
});

test("disposing while start is held does not publish a late link or dispatch cancellation", async () => {
  let start, cancels = 0;
  const f = fixture({ auth: {
    start: () => new Promise(resolve => { start = resolve; }),
    cancel: async () => { cancels++; return { flow: "a".repeat(64), state: "canceled" }; },
  } });
  try {
    f.elements.signin.click();
    f.elements["auth-cancel"].click();
    assert.equal(cancels, 0);
    f.dispose();
    const previousLocks = [...f.locks];
    start({ flow: "a".repeat(64), state: "waiting", url: "https://signin.example/late" });
    await tick();
    assert.equal(f.elements["auth-link"].href, undefined);
    assert.deepEqual(f.locks, previousLocks);
    assert.equal(f.reloads(), 0);
  } finally { f.close(); }
});
