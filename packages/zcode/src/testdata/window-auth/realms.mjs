import assert from "node:assert/strict";
import vm from "node:vm";
import { webcrypto } from "node:crypto";

export function assertPublic(value, secrets = []) {
  const privateKeys = new Set([
    "credentials", "continuation", "codeverifier", "code_verifier", "access_token",
    "refreshtoken", "refresh_token", "sessiontoken", "session_token", "businesstoken",
    "business_token", "authorization", "auth", "device_code", "pkce", "verifier",
    "accesstoken", "access", "refresh", "token", "secret", "private",
  ]);
  const visit = (item) => {
    if (!item || typeof item !== "object") return;
    for (const [key, nested] of Object.entries(item)) {
      assert.equal(privateKeys.has(key.toLowerCase()), false, "private credential field crossed the iframe boundary");
      visit(nested);
    }
  };
  visit(value);
  const text = JSON.stringify(value) ?? "";
  for (const secret of secrets) {
    if (secret) assert.equal(text.includes(secret), false, "private fixture material crossed the iframe boundary");
  }
}

async function sourceModule(source, context, identifier) {
  const module = new vm.SourceTextModule(source, { context, identifier });
  await module.link(() => { throw new Error("consumer proof does not admit additional browser dependencies"); });
  await module.evaluate();
  return module.namespace;
}

export async function createFrame({
  page, origin, cookie, bridgeSource, parentSource, sdkSource, secrets = [], transport = fetch,
}) {
  const incoming = [];
  const outgoing = [];
  const http = [];
  const violations = [];
  const listeners = new Map();
  const abort = new AbortController();
  const frameWindow = {};
  const parentProxy = {};
  let disposed = false;
  const check = (value) => {
    try {
      assertPublic(value, secrets);
    } catch (error) {
      violations.push("private material appeared at a guarded boundary");
      throw error;
    }
  };
  const record = (target, value) => {
    const cloned = structuredClone(value);
    check(cloned);
    target.push(cloned);
    return cloned;
  };
  const request = async (route, body, _unused, signal) => {
    check(body);
    const response = await transport(`${origin}/api/${route}`, {
      method: body === undefined ? "GET" : "POST",
      headers: {
        Cookie: cookie, Origin: origin, "Sec-Fetch-Site": "same-origin",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      signal,
    });
    const text = await response.text();
    for (const secret of secrets) {
      if (secret && text.includes(secret)) {
        violations.push("private material appeared in a raw web response");
        throw new Error("private fixture material reached a web response");
      }
    }
    const result = JSON.parse(text);
    check(result);
    http.push({ route, request: structuredClone(body), status: response.status, response: structuredClone(result) });
    if (!response.ok) throw new Error(typeof result.error === "string" ? result.error : "parent HTTP request was refused");
    return result;
  };
  const parentContext = vm.createContext({
    api: request, crypto: webcrypto, Uint8Array, AbortController,
    clearTimeout, setTimeout, t: (message) => message,
  });
  const parent = await sourceModule(parentSource, parentContext, `${origin}/extensionwindows.js`);
  const entry = Object.freeze(structuredClone(page));
  const owner = { entry, abort, frame: { contentWindow: frameWindow }, shown: true };
  const answer = async (message, deliver = true) => {
    assert.equal(owner.frame.contentWindow, frameWindow);
    const value = record(outgoing, message);
    let response;
    try {
      const result = await parent.answerPluginPage(owner, value,
        () => ({ dark: false, lang: "en", tokens: {} }), () => "auth fixture");
      response = { magpie: 1, id: value.id, result };
    } catch (error) {
      response = { magpie: 1, id: value.id, error: String(error?.message ?? error) };
    }
    response = record(incoming, response);
    if (deliver && !disposed) {
      for (const handler of listeners.get("message") ?? []) {
        handler({ source: parentProxy, data: structuredClone(response) });
      }
    }
    return response;
  };
  parentProxy.postMessage = (message) => { void answer(message); };
  const root = {
    dataset: {}, style: { setProperty() {} },
    getBoundingClientRect: () => ({ height: 1 }),
    prepend() {},
  };
  const context = vm.createContext({
    parent: parentProxy, location: { href: origin + page.url },
    document: {
      readyState: "loading", documentElement: root, head: { prepend() {} },
      createElement: () => ({ dataset: {}, textContent: "" }),
    },
    addEventListener(name, handler) {
      const values = listeners.get(name) ?? [];
      values.push(handler);
      listeners.set(name, values);
    },
    setTimeout, clearTimeout, URL, Uint8Array, Blob, console,
    fetch() { throw new Error("iframe must use its parent instead of direct HTTP"); },
  });
  context.window = context;
  vm.runInContext(bridgeSource, context, { filename: "served-pagebridge.js" });
  const browser = await sourceModule(sdkSource, context, "packaged-public-sdk-ui.js");
  await browser.ui.ready;
  return {
    ui: browser.ui,
    violations,
    messages: { incoming, outgoing, http },
    async replay(message) { return answer(message, false); },
    async forged(method, params) {
      return answer({ magpie: 1, id: 900001 + outgoing.length, method, params }, false);
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      abort.abort();
      for (const handler of listeners.get("message") ?? []) {
        handler({ source: parentProxy, data: { magpie: 1, event: "dispose" } });
      }
      owner.frame.contentWindow = null;
    },
  };
}
