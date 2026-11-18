const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { chromium } = require("playwright");

async function main() {
  const base = process.argv[2];
  const host = process.env.CC_HOST_ROOT;
  const root = process.env.CC_UI_ROOT;
  const app = fs.readFileSync(path.join(host, "internal/gui/assets/app.js"), "utf8");
  const renderer = fs.readFileSync(path.join(host, "internal/gui/assets/provideroptions.js"), "utf8");
  const section = (start, end) => {
    const first = app.indexOf(start), last = app.indexOf(end, first);
    assert.ok(first >= 0 && last > first);
    return app.slice(first, last);
  };
  const browser = await chromium.launch({ headless: true, executablePath: process.env.CC_CHROMIUM,
    args: ["--no-sandbox", "--disable-background-networking", "--disable-component-update"] });
  try {
    const context = await browser.newContext();
    const blocked = [], errors = [];
    await context.route("**/*", async route => {
      if (route.request().url().startsWith(base + "/")) return route.continue();
      blocked.push(route.request().url());
      return route.abort();
    });
    const page = await context.newPage();
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(base + "/fixture");
    await page.setContent('<div id="options"></div><button id="save">Save</button><pre id="error"></pre>');
    await page.addScriptTag({ content:
      'const locale="en",NO_SCROLL={preventScroll:true};let segsMade=0;const segsDue=[[]];function slide(){};' +
      'function t(text,fields={}){for(const [key,value]of Object.entries(fields))text=text.replaceAll("{"+key+"}",value);return text;}' +
      section("function el(tag,", "// replaceKeeping") +
      section("function segs(items,", "// controls placed") +
      section("function backdropClick(", "// Settings › Uninstall") +
      section("function optionWords(", "// said under the models") + renderer });
    await page.evaluate(async () => {
      window.provider = await (await fetch("/fixture")).json();
      window.draft = { extensionOwner: structuredClone(provider.extensionOptions) };
      window.posts = [];
      window.api = async (route, input) => {
        posts.push({ route, input });
        const response = await fetch("/api/" + route, {
          method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
        });
        const result = await response.json();
        if (!response.ok) throw new Error(result.error || "option write refused");
        return result;
      };
      paintExtensionProviderOptions(document.querySelector("#options"), draft, provider);
      document.querySelector("#save").onclick = async () => {
        try { await saveExtensionProviderOptions(draft, provider); }
        catch (error) { document.querySelector("#error").textContent = error.message; }
      };
    });
    const before = await (await context.request.get(base + "/config")).json();
    await page.locator('[data-key="zdr"] [data-seg="on"]').click();
    const dialog = page.getByRole("alertdialog");
    await dialog.waitFor();
    assert.match(await dialog.innerText(), /cost more.*fails instead of falling back/s);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await page.waitForFunction(() => document.querySelector('[data-seg="off"]').getAttribute("aria-pressed") === "true");
    assert.deepEqual(await page.evaluate(() => posts), []);
    assert.deepEqual(await (await context.request.get(base + "/config")).json(), before);
    await page.locator('[data-key="zdr"] [data-seg="on"]').click();
    await dialog.waitFor();
    await dialog.getByRole("button", { name: "Turn on", exact: true }).click();
    await page.locator("#save").click();
    await page.waitForFunction(() => draft.extensionOptionsSaved === true);
    assert.equal(await page.evaluate(() => posts.length), 1);
    assert.deepEqual(await page.evaluate(() => posts[0].input.confirmed), ["zdr"]);
    const after = await (await context.request.get(base + "/config")).json();
    assert.equal(after.version, "2");
    assert.equal(after.values.zdr, true);
    assert.deepEqual(after.values.unrelated, { keep: "value" });
    await page.locator("#save").click();
    assert.equal(await page.evaluate(() => posts.length), 1);
    await page.evaluate(() => { provider.extensionOptions.configVersion = "stale"; });
    await page.locator("#save").click();
    await page.locator("#error").filter({ hasText: "reopen" }).waitFor();
    assert.equal(await page.evaluate(() => posts.length), 1);
    await context.route(base + "/web/index.html", async route => {
      const bridge = '<script>window.casCalls=0;globalThis.magpie={ready:Promise.resolve(),backend:{accounts:async()=>[]},config:{get:async()=>({version:"2",values:{zdr:true}}),cas:async(expected,values)=>{casCalls++;return{version:"3",values}}}};</script>';
      const html = fs.readFileSync(path.join(root, "web/index.html"), "utf8")
        .replace('<script type="module"', bridge + '<script type="module"');
      return route.fulfill({ contentType: "text/html", body: html });
    });
    await context.route(base + "/web/**", async route => {
      if (route.request().url() === base + "/web/index.html") return route.fallback();
      const relative = new URL(route.request().url()).pathname;
      assert.match(relative, /^\/web\/[\w./-]+\.js$/);
      return route.fulfill({ contentType: "text/javascript", body: fs.readFileSync(path.join(root, relative)) });
    });
    await page.goto(base + "/web/index.html");
    await page.locator("#output").filter({ hasText: "Choose an account" }).waitFor();
    const checkbox = page.locator('input#zdr[type="checkbox"]');
    if (await checkbox.count()) {
      await checkbox.uncheck();
      await page.waitForFunction(() => casCalls > 0);
    }
    assert.equal(await page.evaluate(() => casCalls), 0, "custom page bypassed the confirmed provider option owner");
    assert.match(await page.locator("#zdr").innerText(), /Providers.*Command Code/);
    assert.deepEqual(blocked, []);
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ actualHostRenderer: true, actualOptionAPI: true, cancelUnchanged: true,
      acceptedCAS: 1, staleGuard: true, customPageWrites: 0, browser: "chromium", blocked }));
    await context.close();
  } finally {
    await browser.close();
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
