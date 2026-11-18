const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const distribution = process.env.MAGPIE_HISTORY_DIST;
const host = process.env.MAGPIE_HOST_SOURCE;
assert(distribution && host, "test needs a built package and the host page bridge");

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: device authorization offers a selectable URL within the existing sandbox`, async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    const context = await browser.newContext();
    t.after(() => context.close());
    const page = await context.newPage();
    page.setDefaultTimeout(8000);
    const external = [];
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.host !== "magpie.test") { external.push(url.href); return route.abort(); }
      if (url.pathname === "/") return route.fulfill({ contentType: "text/html", body: `<!doctype html>
        <iframe sandbox="allow-scripts" src="/plugin/index.html" style="width:700px;height:650px"></iframe>
        <script>
          window.authURL = "https://auth.synthetic.invalid/device";
          window.finished = false; window.starts = 0;
          addEventListener("message", event => {
            const frame = document.querySelector("iframe");
            if (event.source !== frame.contentWindow || event.data?.magpie !== 1) return;
            const message = event.data;
            let result;
            if (message.method === "hello") result = { dark:false, lang:"ru", tokens:{}, visible:true, features:[] };
            else if (message.method === "backend.accounts") result = [];
            else if (message.method === "auth.start") {
              starts++; result = { flow:"synthetic-" + starts, state:"waiting", url:authURL, userCode:"SYNTHETIC", retryAfterMs:1000 };
            } else if (message.method === "auth.cancel") result = { flow:message.params.flow, state:"canceled" };
            else if (message.method === "auth.finish") result = { flow:message.params.flow, state:"complete" };
            else return;
            event.source.postMessage({magpie:1,id:message.id,result}, "*");
          });
        </script>` });
      const bridge = url.pathname.endsWith("/_magpie/bridge.js");
      const file = bridge ? path.join(host, "internal/gui/pagebridge.js") :
        path.join(distribution, "web", path.basename(url.pathname));
      return route.fulfill({ contentType: file.endsWith(".html") ? "text/html" : "text/javascript", body: await fs.readFile(file) });
    });
    await page.goto("http://magpie.test/");
    const frame = page.frameLocator("iframe");
    await frame.locator("#status").filter({ hasText: "Готово." }).waitFor();
    await frame.locator("#signin").click();
    await page.waitForFunction(() => starts === 1);
    assert.equal(await frame.locator("#auth-url").count(), 1, "device sign-in needs a selectable URL fallback, not a sandbox-blocked popup");
    await frame.locator("#auth-address").waitFor({ state: "visible" });
    assert.equal(await frame.locator("#auth-url").inputValue(), "https://auth.synthetic.invalid/device");
    assert.equal(await frame.locator("#auth-url").getAttribute("readonly"), "");
    await frame.locator("#auth-select").click();
    assert.deepEqual(await frame.locator("#auth-url").evaluate(element => [element.selectionStart, element.selectionEnd]),
      [0, "https://auth.synthetic.invalid/device".length], "select-address must select the complete copyable destination");
    assert.match(await frame.locator("#auth-address").innerText(), /скопируйте.*откройте/i);
    assert.equal(await page.locator("iframe").getAttribute("sandbox"), "allow-scripts");
    assert.equal(context.pages().length, 1);
    assert.deepEqual(external, []);

    for (const terminal of ["cancel", "poll"]) {
      if (terminal === "poll") await frame.locator("#signin").click();
      await frame.locator("#auth-address").waitFor({ state: "visible" });
      await frame.locator("#" + terminal).click();
      await frame.locator("#auth-address").waitFor({ state: "hidden" });
      assert.equal(await frame.locator("#auth-url").inputValue(), "", "terminal authentication must clear the old destination");
    }
    for (const url of ["not a URL", "http://auth.synthetic.invalid", "https://user:secret@auth.synthetic.invalid"]) {
      await page.evaluate(url => { authURL = url; }, url);
      await frame.locator("#signin").click();
      await frame.locator("#cancel:enabled").waitFor();
      assert.equal(await frame.locator("#auth-url").inputValue(), "");
      assert.equal(await frame.locator("#auth-address").isVisible(), false);
      await frame.locator("#cancel").click();
    }
    await page.evaluate(() => { authURL = "https://auth.synthetic.invalid/device"; });
    await frame.locator("#signin").click();
    await frame.locator("#auth-address").waitFor({ state: "visible" });
    await frame.locator("body").evaluate(() => dispatchEvent(new Event("pagehide")));
    assert.equal(await frame.locator("#auth-url").inputValue(), "", "disposing the consumer clears the destination");
    assert.equal(await frame.locator("#auth-address").isVisible(), false);
    assert.deepEqual(external, []);
  });
}
