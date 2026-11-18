import assert from "node:assert/strict";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";

const { mountControlPage } = await import(pathToFileURL(path.join(process.env.ZCODE_PAGE_BUILD_DIR, "page.js")));
const account = { id: "<synthetic-account>", generation: "9007199254740993" };

class Element {
  textContent = "";
  value = "";
  disabled = false;
  children = [];
  handlers = new Map();
  addEventListener(name, handler) { this.handlers.set(name, handler); }
  removeEventListener(name, handler) {
    if (this.handlers.get(name) === handler) this.handlers.delete(name);
  }
  replaceChildren(...values) { this.children = values; this.value = ""; }
  dispatch(name) { this.handlers.get(name)?.(); }
}

function fixture(backend, confirm = () => true) {
  const elements = Object.fromEntries(
    ["account", "reload", "plan", "reset-window", "status", "output",
      "executions", "billing", "resets", "offers", "checkin", "claim", "reset",
      "reset-opportunity", "reset-reconcile", "reset-ack", "reset-resolve", "opportunity-resolve",
      "native-session", "native-command", "native-model", "native-text", "native-run", "native-stop",
      "native-status"].map((name) => [name, new Element()]),
  );
  const document = {
    getElementById: (name) => elements[name],
    createElement: () => new Element(),
  };
  return { elements, page: mountControlPage(document, backend, confirm) };
}

const tick = () => new Promise((resolve) => setImmediate(resolve));
function select(elements) {
  elements.account.value = account.id;
  elements.account.dispatch("change");
}

test("native foreground keeps one selected command while stop bypasses its pending wait", async () => {
  const calls = [];
  let finish;
  const result = new Promise((resolve) => { finish = resolve; });
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: (...args) => {
      calls.push(args);
      return args[0] === "page.native-run" ? result : Promise.resolve({ requested: true });
    },
  });
  await page.ready;
  select(elements);
  elements["native-session"].value = "session-9";
  elements["native-command"].value = "cmd-ui";
  elements["native-model"].value = "native-fixture/fixture-model";
  elements["native-text"].value = "one input";
  elements["native-run"].dispatch("click");
  await tick();
  assert.equal(calls.length, 1);
  assert.equal(elements.account.disabled, true);
  assert.equal(elements.claim.disabled, true);
  assert.equal(elements["native-stop"].disabled, false);
  elements["native-command"].value = "foreign";
  elements["native-stop"].dispatch("click");
  elements["native-stop"].dispatch("click");
  elements["native-run"].dispatch("click");
  await tick();
  assert.deepEqual(calls, [
    ["page.native-run", { sessionId: "session-9", commandId: "cmd-ui",
      model: "native-fixture/fixture-model", text: "one input" },
      { account: { id: account.id, expectedGeneration: account.generation } }],
    ["page.native-stop", { sessionId: "session-9", commandId: "cmd-ui" },
      { account: { id: account.id, expectedGeneration: account.generation } }],
  ]);
  finish({ outcome: "completed", state: "completedInterrupted", joined: true });
  await tick();
  assert.equal(elements["native-stop"].disabled, true);
  assert.equal(elements.account.disabled, false);
  page.dispose();
});

test("declined native confirmation and uncertain command never send a second input", async () => {
  const calls = [];
  let allowed = false;
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async (...args) => { calls.push(args); throw new Error("unconfirmed"); },
  }, () => allowed);
  await page.ready;
  select(elements);
  for (const [name, value] of Object.entries({
    "native-session": "session-9", "native-command": "cmd-ui",
    "native-model": "fixture/model", "native-text": "once",
  })) elements[name].value = value;
  elements["native-run"].dispatch("click");
  await tick();
  assert.equal(calls.length, 0);
  allowed = true;
  elements["native-run"].dispatch("click");
  await tick();
  assert.equal(calls.length, 1);
  assert.equal(elements["native-command"].value, "cmd-ui");
  assert.equal(elements["native-run"].disabled, true);
  assert.match(elements.status.textContent, /Повтор автоматически не выполняется/);
  elements["native-run"].dispatch("click");
  await tick();
  assert.equal(calls.length, 1);
  page.dispose();
});

test("page starts unselected and renders hostile-looking IDs as text", async () => {
  const calls = [];
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async (...args) => { calls.push(args); return { message: "<script>never html</script>" }; },
  });
  await page.ready;
  assert.equal(elements.claim.disabled, true);
  assert.equal(elements.account.children[1].textContent, account.id);
  select(elements);
  assert.equal(elements.claim.disabled, false);
  elements.executions.dispatch("click");
  assert.equal(elements.claim.disabled, true);
  await tick();
  assert.deepEqual(calls, [["page.execution-list", {}, {
    account: { id: account.id, expectedGeneration: account.generation },
  }]]);
  assert.match(elements.output.textContent, /<script>never html<\/script>/);
  page.dispose();
  assert.ok(Object.values(elements).every((value) => value.handlers.size === 0));
});

test("declined financial confirmation cannot dispatch", async () => {
  let calls = 0;
  const confirmations = [];
  const { page, elements } = fixture({
    accounts: async () => [account], call: async () => { calls++; return null; },
  }, (message) => { confirmations.push(message); return false; });
  await page.ready;
  select(elements);
  elements.plan.value = "plan-id";
  elements["reset-window"].value = "WEEK";
  elements.claim.dispatch("click");
  await tick();
  elements.reset.dispatch("click");
  await tick();
  assert.equal(calls, 0);
  assert.equal(confirmations.length, 2);
  assert.ok(confirmations.every((message) => message.includes(account.id)));
  page.dispose();
});

test("uncertain write is single-flight and never automatically retried", async () => {
  let reject;
  const pending = new Promise((_resolve, failure) => { reject = failure; });
  const calls = [];
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: (...args) => { calls.push(args); return pending; },
  });
  await page.ready;
  select(elements);
  elements["reset-window"].value = "FIVE_HOUR";
  elements.reset.dispatch("click");
  elements.reset.dispatch("click");
  elements.claim.dispatch("click");
  await tick();
  assert.equal(calls.length, 1);
  assert.equal(elements.account.disabled, true);
  reject(new Error("uncertain"));
  await tick();
  assert.equal(calls.length, 1);
  assert.match(elements.status.textContent, /Повтор автоматически не выполняется/);
  assert.equal(elements.account.disabled, false);
  page.dispose();
});

test("asynchronous page confirmation locks selection and preserves original target", async () => {
  let confirm;
  const confirmation = new Promise((resolve) => { confirm = resolve; });
  const calls = [];
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async (...args) => { calls.push(args); return { outcome: "confirmed" }; },
  }, () => confirmation);
  await page.ready;
  select(elements);
  elements.plan.value = "plan-before-confirm";
  elements.claim.dispatch("click");
  assert.equal(elements.account.disabled, true);
  assert.equal(calls.length, 0);
  elements.plan.value = "changed";
  elements.claim.dispatch("click");
  confirm(true);
  await tick();
  assert.equal(calls.length, 1);
  assert.deepEqual(calls[0], ["page.billing-claim", { planId: "plan-before-confirm" }, {
    account: { id: account.id, expectedGeneration: account.generation },
  }]);
  page.dispose();
});

test("disposing during confirmation never dispatches the financial operation", async () => {
  let confirm;
  let calls = 0;
  const confirmation = new Promise((resolve) => { confirm = resolve; });
  const { page, elements } = fixture({
    accounts: async () => [account], call: async () => { calls++; return null; },
  }, () => confirmation);
  await page.ready;
  select(elements);
  elements.plan.value = "plan";
  elements.claim.dispatch("click");
  page.dispose();
  confirm(true);
  await tick();
  assert.equal(calls, 0);
});

test("reset summary and explicitly confirmed recovery use exact selected generation", async () => {
  const calls = [], confirmations = [];
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async (...args) => { calls.push(args); return { pending: true, opportunityPending: true }; },
  }, (message) => { confirmations.push(message); return true; });
  await page.ready;
  select(elements);
  elements.resets.dispatch("click");
  await tick();
  for (const name of ["reset-opportunity", "reset-reconcile", "reset-ack", "reset-resolve", "opportunity-resolve"]) {
    elements[name].dispatch("click");
    await tick();
  }
  const selection = { account: { id: account.id, expectedGeneration: account.generation } };
  assert.deepEqual(calls, [
    ["page.reset-summary", {}, selection],
    ["page.reset-opportunity", {}, selection],
    ["page.reset-reconcile", {}, selection],
    ["page.reset-ack", {}, selection],
    ["page.reset-resolve", { acknowledgment: "vendor-may-have-spent-reset" }, selection],
    ["page.reset-opportunity-resolve", { acknowledgment: "vendor-may-have-granted-reset" }, selection],
  ]);
  assert.equal(confirmations.length, 5);
  assert.ok(confirmations.every(message => message.includes(account.id)));
  assert.match(confirmations[3], /мог.*потрачен|мог.*потратить/);
  assert.match(confirmations[4], /мог.*выдан/);
  page.dispose();
});

test("reset recovery confirmation blocks concurrent writes and disposal prevents dispatch", async () => {
  let confirm, calls = 0;
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async () => { calls++; return null; },
  }, () => new Promise(resolve => { confirm = resolve; }));
  await page.ready;
  select(elements);
  elements["reset-resolve"].dispatch("click");
  assert.equal(elements.account.disabled, true);
  elements["opportunity-resolve"].dispatch("click");
  elements.reset.dispatch("click");
  assert.equal(calls, 0);
  page.dispose();
  confirm(true);
  await tick();
  assert.equal(calls, 0);
});

test("declined reset recovery and stale selected generations never dispatch", async () => {
  let calls = 0;
  let accounts = [account];
  const { page, elements } = fixture({
    accounts: async () => accounts,
    call: async () => { calls++; return null; },
  }, () => false);
  await page.ready;
  select(elements);
  for (const name of ["reset-opportunity", "reset-reconcile", "reset-ack", "reset-resolve", "opportunity-resolve"]) {
    elements[name].dispatch("click");
    await tick();
  }
  assert.equal(calls, 0);
  accounts = [{ ...account, generation: "9007199254740994" }];
  await page.reload();
  elements["reset-resolve"].dispatch("click");
  await tick();
  assert.equal(calls, 0);
  assert.equal(elements["reset-resolve"].disabled, true);
  page.dispose();
});

test("invalid actions and stale generations do not silently retarget", async () => {
  let values = [account];
  let calls = 0;
  const { page, elements } = fixture({
    accounts: async () => values, call: async () => { calls++; return null; },
  });
  await page.ready;
  select(elements);
  elements["reset-window"].value = "DAY";
  elements.reset.dispatch("click");
  elements.plan.value = "invalid\n";
  elements.claim.dispatch("click");
  assert.equal(calls, 0);
  values = [{ ...account, generation: "9007199254740994" }];
  elements.reload.dispatch("click");
  await tick();
  assert.equal(elements.account.value, "");
  assert.equal(elements.claim.disabled, true);
  elements.claim.dispatch("click");
  assert.equal(calls, 0);
  page.dispose();
});

test("disposed page cannot publish a late account response", async () => {
  let resolve;
  const pending = new Promise((done) => { resolve = done; });
  const { page, elements } = fixture({ accounts: () => pending, call: async () => null });
  page.dispose();
  const before = elements.status.textContent;
  resolve([account]);
  await page.ready;
  assert.equal(elements.status.textContent, before);
  assert.equal(elements.account.children.length, 0);
});

test("offers and explicit check-in retain one selected generation and confirmation", async () => {
  const calls = [];
  let allow = false;
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async (...args) => { calls.push(args); return { outcome: "inactive" }; },
  }, () => allow);
  await page.ready;
  elements.checkin.dispatch("click");
  assert.equal(calls.length, 0);
  select(elements);
  elements.offers.dispatch("click");
  await tick();
  assert.deepEqual(calls, [["page.billing-offers", {}, {
    account: { id: account.id, expectedGeneration: account.generation },
  }]]);
  elements.checkin.dispatch("click");
  await tick();
  assert.equal(calls.length, 1);
  allow = true;
  elements.checkin.dispatch("click");
  elements.checkin.dispatch("click");
  await tick();
  assert.deepEqual(calls[1], ["page.billing-checkin", {}, {
    account: { id: account.id, expectedGeneration: account.generation },
  }]);
  assert.equal(calls.length, 2);
  page.lockAuthentication(true);
  elements.checkin.dispatch("click");
  await tick();
  assert.equal(calls.length, 2);
  page.dispose();
});

test("unknown check-in clears authority and never repeats the write", async () => {
  let calls = 0;
  const { page, elements } = fixture({
    accounts: async () => [account],
    call: async () => { calls++; throw new Error("check-in uncertain"); },
  });
  await page.ready;
  select(elements);
  elements.checkin.dispatch("click");
  await tick();
  assert.equal(calls, 1);
  assert.equal(page.selection(), undefined);
  assert.equal(elements.checkin.disabled, true);
  elements.checkin.dispatch("click");
  await tick();
  assert.equal(calls, 1);
  page.dispose();
});
