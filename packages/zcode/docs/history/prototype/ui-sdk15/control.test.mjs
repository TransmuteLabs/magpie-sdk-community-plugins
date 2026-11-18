import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";

const { ZcodeControl, bindings } = await import(pathToFileURL(path.join(process.env.ZCODE_UI15_BUILD_DIR, "control.js")));
const accountA = { id: "a", generation: "9007199254740993", expiresAt: 2000 };
const accountB = { id: "b", generation: "7" };

function pending() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

test("selected account uses exact string generation and no client provider", async () => {
  const calls = [];
  const control = new ZcodeControl({
    accounts: async (binding) => {
      assert.equal(binding, bindings.executionList);
      return [{ ...accountA, credentials: "must-not-copy" }];
    },
    call: async (...args) => { calls.push(args); return { state: "ready" }; },
  });
  await control.refreshAccounts();
  control.select("a");
  await control.read("executionList");
  assert.deepEqual(calls, [[bindings.executionList, {}, {
    account: { id: "a", expectedGeneration: "9007199254740993" },
  }]]);
  assert.deepEqual(control.snapshot().accounts, [accountA]);
  assert.equal(JSON.stringify(calls).includes("provider"), false);
  const snapshot = control.snapshot();
  snapshot.selected.id = "changed";
  snapshot.accounts[0].generation = "changed";
  assert.equal(control.snapshot().selected.id, "a");
  assert.equal(control.snapshot().accounts[0].generation, accountA.generation);
});

test("unlisted or missing selection cannot dispatch", async () => {
  let calls = 0;
  const control = new ZcodeControl({ accounts: async () => [accountA], call: async () => { calls++; } });
  await assert.rejects(control.read("resetStatus"), /Select/);
  await control.refreshAccounts();
  assert.throws(() => control.select("another-account"), /host/);
  await assert.rejects(control.read("resetUse"), /declared/);
  assert.equal(calls, 0);
});

test("stale account enumeration cannot replace newer results", async () => {
  const first = pending(), second = pending();
  let count = 0;
  const control = new ZcodeControl({
    accounts: () => (++count === 1 ? first.promise : second.promise), call: async () => null,
  });
  const old = control.refreshAccounts();
  const current = control.refreshAccounts();
  second.resolve([accountB]);
  await current;
  first.resolve([accountA]);
  await old;
  assert.deepEqual(control.snapshot().accounts, [accountB]);
});

test("account switch discards an old read without retargeting", async () => {
  const reply = pending();
  let captured;
  const control = new ZcodeControl({
    accounts: async () => [accountA, accountB],
    call: (_binding, _input, options) => { captured = options; return reply.promise; },
  });
  await control.refreshAccounts();
  control.select("a");
  const old = control.read("billingStatus");
  control.select("b");
  reply.resolve({ privateToA: true });
  await assert.rejects(old, /changed/);
  assert.equal(control.snapshot().last, undefined);
  assert.equal(captured.account.id, "a");
});

test("generation change clears selection instead of upgrading silently", async () => {
  let generation = accountA.generation;
  const control = new ZcodeControl({
    accounts: async () => [{ ...accountA, generation }], call: async () => null,
  });
  await control.refreshAccounts();
  control.select("a");
  generation = "9007199254740994";
  await control.refreshAccounts();
  assert.equal(control.snapshot().selected, undefined);
  await assert.rejects(control.reset("WEEK"), /Select/);
  control.select("a");
  assert.equal(control.snapshot().selected.expectedGeneration, generation);
});

test("write is single-flight and uncertain result is never retried", async () => {
  const reply = pending();
  const calls = [];
  const control = new ZcodeControl({
    accounts: async () => [accountA, accountB],
    call: (...args) => { calls.push(args); return reply.promise; },
  });
  await control.refreshAccounts();
  control.select("a");
  const write = control.reset("FIVE_HOUR");
  await assert.rejects(control.claim("plan"), /Wait/);
  assert.throws(() => control.select("b"), /Wait/);
  reply.reject(new Error("vendor result is uncertain"));
  await assert.rejects(write, /uncertain/);
  assert.equal(calls.length, 1);
  assert.deepEqual(calls[0], [bindings.resetUse, { window: "FIVE_HOUR" }, {
    account: { id: "a", expectedGeneration: accountA.generation },
  }]);
  assert.equal(control.snapshot().busy, false);
});

test("invalid financial parameters never reach the bridge", async () => {
  let calls = 0;
  const control = new ZcodeControl({
    accounts: async () => [accountA], call: async () => { calls++; return null; },
  });
  await control.refreshAccounts();
  control.select("a");
  await assert.rejects(control.reset("DAY"), /supported/);
  await assert.rejects(control.claim(" "), /exact/);
  await assert.rejects(control.claim("plan\n"), /exact/);
  assert.equal(calls, 0);
});

test("an older read cannot overwrite a completed financial action", async () => {
  const reply = pending();
  const control = new ZcodeControl({
    accounts: async () => [accountA],
    call: (binding) => binding === bindings.resetStatus ? reply.promise : Promise.resolve({ outcome: "confirmed" }),
  });
  await control.refreshAccounts();
  control.select("a");
  const old = control.read("resetStatus");
  await control.reset("WEEK");
  reply.resolve({ stale: true });
  await assert.rejects(old, /changed/);
  assert.deepEqual(control.snapshot().last, { outcome: "confirmed" });
});

test("prototype bindings fix provider in the declaration", async () => {
  const fragment = JSON.parse(await readFile(new URL("./bindings.json", import.meta.url), "utf8"));
  assert.equal(fragment.requiresHostAPI, "^1.5");
  assert.equal(fragment.bindings.length, 5);
  assert.ok(fragment.bindings.every((binding) => binding.provider === "zcode"));
});
