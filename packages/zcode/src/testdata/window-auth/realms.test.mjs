import assert from "node:assert/strict";
import test from "node:test";
import { assertPublic, createFrame } from "./realms.mjs";

const bridgeSource = `
const pending = new Map();
let next = 0;
function call(method, params) {
  return new Promise((resolve, reject) => {
    const id = ++next;
    pending.set(id, {resolve,reject});
    parent.postMessage({magpie:1,id,method,params},"*");
  });
}
addEventListener("message", (event) => {
  if (event.source !== parent) return;
  const item = pending.get(event.data.id);
  if (!item) return;
  pending.delete(event.data.id);
  event.data.error ? item.reject(new Error(event.data.error)) : item.resolve(event.data.result);
});
window.magpie = {
  ready:call("hello"),
  auth:{start:(binding,options)=>call("auth.start",{binding,options})}
};
`;
const parentSource = `
export async function answerPluginPage(owner,message) {
  if (message.method === "hello") return {dark:false};
  return api("plugin-pages/auth",{
    ref:owner.entry.ref,binding:message.params.binding,
    operation:"start",options:message.params.options,
    parent:"fixed-parent",request:"fixed-parent:"+message.id
  });
}
`;
const sdkSource = "export const ui=window.magpie;";
const base = {
  page: { ref: "root-ref", url: "/fixture/index.html" },
  origin: "http://127.0.0.1:12345", cookie: "synthetic=parent-only",
  bridgeSource, parentSource, sdkSource, secrets: ["private-token-canary"],
};

test("fixture relays frames through parent-only HTTP instead of frame fetch", async () => {
  const calls = [];
  const value = await createFrame({
    ...base,
    transport: async (url, options) => {
      calls.push({ url, options });
      return new Response(JSON.stringify({ flow: "public-handle", state: "waiting" }));
    },
  });
  try {
    const result = await value.ui.auth.start("signin", { method: "device" });
    assert.equal(result.flow, "public-handle");
    assert.equal(calls.length, 1);
    assert.equal(JSON.parse(calls[0].options.body).ref, "root-ref");
    assert.equal(calls[0].options.headers.Cookie, "synthetic=parent-only");
    assert.equal(JSON.stringify(value.messages.outgoing).includes("parent-only"), false);
    assert.deepEqual(value.violations, []);
    const replay = await value.replay(value.messages.outgoing.find((item) => item.method === "auth.start"));
    assert.equal(replay.result.flow, "public-handle");
    assert.equal(calls.length, 2);
  } finally {
    value.dispose();
  }
});

test("expected rejection cannot hide a private response from final privacy evidence", async () => {
  for (const result of [{ continuation: { id: "private" } }, { error: "private-token-canary" }]) {
    const value = await createFrame({
      ...base,
      transport: async () => new Response(JSON.stringify(result)),
    });
    try {
      await assert.rejects(() => value.ui.auth.start("signin", { method: "device" }));
      assert.ok(value.violations.length > 0);
      assert.equal(JSON.stringify(value.messages.incoming).includes("private-token-canary"), false);
    } finally {
      value.dispose();
    }
  }
});

test("privacy audit traverses nested credential fields and exact token canaries", () => {
  for (const value of [
    { result: { Credentials: { access: "not-public" } } },
    { result: { nested: [{ code_verifier: "not-public" }] } },
    { error: "contains private-token-canary" },
  ]) {
    assert.throws(() => assertPublic(value, base.secrets));
  }
  assert.doesNotThrow(() => assertPublic({ account: { id: "opaque", generation: "9007199254740993" } }, base.secrets));
});
