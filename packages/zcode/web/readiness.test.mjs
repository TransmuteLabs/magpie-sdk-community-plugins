import assert from "node:assert/strict";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";

const { ZcodeControl } = await import(pathToFileURL(path.join(process.env.ZCODE_PAGE_BUILD_DIR, "control.js")));
const account = { id: "chosen", generation: "9007199254740993" };

for (const operation of ["accounts", "read", "write"]) {
  test(`failed ${operation} invalidates selected authority until a fresh explicit selection`, async () => {
    let refused = false, calls = 0;
    const control = new ZcodeControl({
      accounts: async () => {
        if (refused && operation === "accounts") throw new Error("account listing unavailable");
        return [account];
      },
      call: async () => { calls++; if (refused) throw new Error("host refused"); return null; },
    });
    await control.refreshAccounts();
    control.select(account.id);
    refused = true;
    await assert.rejects(operation === "accounts" ? control.refreshAccounts()
      : operation === "read" ? control.read("billingStatus") : control.claim("plan"));
    assert.equal(control.snapshot().selected, undefined);
    assert.deepEqual(control.snapshot().accounts, []);
    assert.throws(() => control.select(account.id));
    const previous = calls;
    await assert.rejects(control.claim("plan"));
    assert.equal(calls, previous);
    refused = false;
    await control.refreshAccounts();
    assert.equal(control.snapshot().selected, undefined);
    control.select(account.id);
    await control.read("billingStatus");
  });
}
