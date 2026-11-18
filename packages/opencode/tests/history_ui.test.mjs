import assert from "node:assert/strict";
import { test } from "node:test";
import { pathToFileURL } from "node:url";
import path from "node:path";

const distribution = process.env.MAGPIE_HISTORY_DIST;
assert(distribution, "test needs the actual built package and SDK browser files");
const { mountHistory } = await import(pathToFileURL(path.join(distribution, "web/history.js")));

class Element {
  children = []; listeners = new Map(); dataset = {}; attributes = {}; style = {};
  textContent = ""; disabled = false; hidden = false;
  constructor(tag = "div") { this.tagName = tag; }
  set innerHTML(_) { throw new Error("stored history must never become HTML"); }
  setAttribute(name, value) { this.attributes[name] = value; }
  addEventListener(name, listener) { this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener]); }
  removeEventListener(name, listener) { this.listeners.set(name, (this.listeners.get(name) ?? []).filter(value => value !== listener)); }
  append(...children) { for (const child of children) { child.parent = this; this.children.push(child); } }
  replaceChildren(...children) { this.children = []; this.append(...children); }
  remove() { if (this.parent) this.parent.children = this.parent.children.filter(child => child !== this); }
  querySelectorAll(tag) { return this.children.flatMap(child => [...(child.tagName === tag ? [child] : []), ...child.querySelectorAll(tag)]); }
  click() { if (!this.disabled) for (const listener of this.listeners.get("click") ?? []) listener(); }
}

const initial = () => ({
  module: "opencode", source: "opencode-history", agent: "opencode", viewRef: "view-one", readAllowed: false,
  ownOff: false, roots: [{ slot: "data", rootRef: "root", available: true, consented: false }], replaces: [],
});

async function until(predicate, message) {
  const end = Date.now() + 5000;
  while (!predicate()) {
    assert(Date.now() < end, message);
    await new Promise(resolve => setTimeout(resolve, 5));
  }
}

async function fixture(t, options = {}) {
  const elements = Object.fromEntries(["status", "roots", "sessions", "transcript", "media", "refresh", "access", "release", "list", "more"]
    .map(name => ["history-" + name, new Element(name.match(/refresh|access|release|list|more/) ? "button" : "div")]));
  const calls = [], visibility = [];
  let inspection = options.inspection ?? initial();
  const history = {
    ready: Promise.resolve(options.supported !== false),
    async inspect(source) { calls.push(["inspect", source]); return structuredClone(inspection); },
    async requestAccess(selection) {
      calls.push(["request", selection]);
      inspection = { ...inspection, viewRef: "view-two", readAllowed: !options.partial,
        roots: [{ ...inspection.roots[0], consented: true, consentRef: "consent" }] };
      return { state: options.partial ? "changed" : "ready", inspection: structuredClone(inspection) };
    },
    async release(selection) { calls.push(["release", selection]); return { state: "released", inspection: {
      ...inspection, viewRef: "released", roots: [{ ...inspection.roots[0], consented: false, consentRef: undefined }],
    } }; },
    async list(selection) {
      calls.push(["list", selection]);
      return { entries: [{ sessionRef: "session", title: "<script>no HTML</script>" }] };
    },
    async read(selection) {
      calls.push(["read", selection]);
      if (options.read) return options.read();
      return { sessionRef: "session", readRef: "read", cut: false,
        parts: [{ role: "user", kind: "text", text: "<img src=https://live.invalid onerror=alert(1)>", model: "<script>model</script>" }],
        media: [
          { mediaRef: "raster", name: "<picture>.png", mime: "image/png", size: 3 },
          { mediaRef: "text", name: "note.txt", mime: "text/plain", size: 3 },
          { mediaRef: "svg", name: "unsafe.svg", mime: "image/svg+xml", size: 3 },
          { mediaRef: "html", name: "unsafe.html", mime: "text/html", size: 3 },
        ],
      };
    },
    async media(selection) {
      calls.push(["media", selection]);
      if (options.media) return options.media();
      return { mime: selection.mediaRef === "raster" ? "image/png" : "text/plain", data: new TextEncoder().encode("<x>").buffer };
    },
  };
  const previous = globalThis.window;
  globalThis.window = { magpie: {
    ready: Promise.resolve({ dark: false }), visible: true, history,
    backend: { call() { throw new Error("history cannot call account backend"); } },
    config: { get() { throw new Error("history cannot read client configuration"); }, cas() {} },
    onVisibility(listener) { visibility.push(listener); },
  } };
  const document = { getElementById(id) { return elements[id]; }, createElement(tag) { return new Element(tag); } };
  const page = mountHistory(document);
  options.onMount?.(globalThis.window.magpie);
  t.after(() => { page.dispose(); globalThis.window = previous; });
  await page.ready;
  const choose = () => elements["history-roots"].children[0].click();
  const grant = async () => {
    choose();
    elements["history-access"].click();
    await until(() => !elements["history-refresh"].disabled, "access did not settle");
  };
  const load = async () => {
    elements["history-list"].click();
    await until(() => elements["history-sessions"].children.length > 0 && !elements["history-refresh"].disabled, "catalog did not settle");
    elements["history-sessions"].children[0].click();
  };
  return { page, elements, calls, choose, grant, load,
    visibility(next) { for (const listener of visibility) listener(next); },
    setInspection(next) { inspection = next; } };
}

test("old host has explicit unsupported state and makes no history or account calls", async t => {
  const current = await fixture(t, { supported: false });
  assert.match(current.elements["history-status"].textContent, /недоступен/);
  assert(current.elements["history-access"].disabled);
  assert.deepEqual(current.calls, []);
});

test("a hidden initial handshake does not inspect history before becoming visible", async t => {
  const current = await fixture(t, { onMount(bridge) { bridge.visible = false; } });
  assert.deepEqual(current.calls, []);
  assert(current.elements["history-access"].disabled);
  current.visibility(true);
  await until(() => current.calls.length === 1, "visible history metadata was not revalidated");
  assert.deepEqual(current.calls[0], ["inspect", "opencode-history"]);
});

test("metadata requires selected root and partial failure displays actual independent gates", async t => {
  const current = await fixture(t, { partial: true });
  assert(current.elements["history-access"].disabled);
  assert.equal(current.calls.length, 1);
  await current.grant();
  assert.match(current.elements["history-status"].textContent, /не завершён.*Хранилище делегировано.*чтение OpenCode не разрешено/);
  assert(current.elements["history-list"].disabled);
  assert(!current.elements["history-release"].disabled);
  assert.deepEqual(current.calls.find(call => call[0] === "request")[1], {
    source: "opencode-history", viewRef: "view-one", rootRef: "root",
  });
  current.elements["history-release"].click();
  await until(() => !current.elements["history-refresh"].disabled, "release did not settle");
  assert.deepEqual(current.calls.find(call => call[0] === "release")[1], {
    source: "opencode-history", viewRef: "view-two", consentRef: "consent",
  });
});

test("issued session/media selections render stored bytes inertly without URLs or accounts", async t => {
  const current = await fixture(t);
  await current.grant();
  await current.load();
  await until(() => current.elements["history-media"].children.length === 4, "read did not settle");
  assert.equal(current.elements["history-sessions"].children[0].textContent, "<script>no HTML</script>");
  assert.match(current.elements["history-transcript"].querySelectorAll("pre")[0].textContent, /<img src=https/);
  assert.equal(current.elements["history-media"].querySelectorAll("button").length, 2);
  current.elements["history-media"].querySelectorAll("button")[0].click();
  await until(() => current.elements["history-media"].querySelectorAll("img").length === 1, "raster media did not render");
  assert.match(current.elements["history-media"].querySelectorAll("img")[0].src, /^data:image\/png;base64,/);
  current.elements["history-media"].querySelectorAll("button")[0].click();
  await until(() => current.elements["history-media"].querySelectorAll("pre").length === 1, "text media did not render");
  assert.equal(current.elements["history-media"].querySelectorAll("pre")[0].textContent, "<x>");
  assert.deepEqual(current.calls.filter(call => call[0] === "media").map(call => call[1].mediaRef), ["raster", "text"]);
  assert.deepEqual(current.calls.find(call => call[0] === "read")[1], {
    source: "opencode-history", viewRef: "view-two", sessionRef: "session",
  });
});

test("independent metadata revision change suppresses held transcript publication", async t => {
  let release;
  const held = new Promise(resolve => { release = resolve; });
  const current = await fixture(t, { read: () => held });
  await current.grant();
  await current.load();
  current.setInspection({ ...initial(), viewRef: "revoked" });
  await until(() => current.elements["history-sessions"].children.length === 0, "visible metadata revalidation never removed revoked history");
  release({ sessionRef: "session", readRef: "old", parts: [{ role: "user", kind: "text", text: "late secret" }], cut: false, media: [] });
  await until(() => !current.elements["history-refresh"].disabled, "held read never settled");
  assert.equal(current.elements["history-transcript"].children.length, 0);
  assert.equal(current.elements["history-media"].children.length, 0);
});

test("visibility and disposal clear history and suppress held media replies", async t => {
  let release;
  const held = new Promise(resolve => { release = resolve; });
  const current = await fixture(t, { media: () => held });
  await current.grant();
  await current.load();
  await until(() => current.elements["history-media"].querySelectorAll("button").length === 2, "media choices missing");
  current.elements["history-media"].querySelectorAll("button")[0].click();
  current.visibility(false);
  assert.equal(current.elements["history-transcript"].children.length, 0);
  assert.equal(current.elements["history-media"].children.length, 0);
  current.page.dispose();
  release({ mime: "image/png", data: new Uint8Array([1, 2, 3]).buffer });
  await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(current.elements["history-media"].children.length, 0);
});
