const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const { test } = require("node:test");
const { chromium } = require("playwright");

const root = process.env.CC_UI_ROOT;
const host = process.env.CC_HOST_ROOT;
assert.ok(root && host, "Set CC_UI_ROOT to the built package and CC_HOST_ROOT to the host source");
const renderer = fs.readFileSync(path.join(host, "internal/gui/assets/pluginpages.js"), "utf8");
const tokens = [...renderer.matchAll(/f\.setAttribute\("sandbox", "([^"]+)"\)/g)].map(match => match[1]);
assert.ok(tokens.length >= 2 && tokens.every(value => value === "allow-scripts"));

test("the Usage panel reads only the chosen account generation", { timeout: 30000 }, async () => {
  const bridge = `<script>
    globalThis.calls = [];
    globalThis.accounts = [{id:"first",generation:"1",name:"First account"},{id:"second",generation:"2"}];
    globalThis.magpie = {
      ready: Promise.resolve(),
      backend: {
        accounts: async (binding) => { calls.push({ binding }); return structuredClone(accounts); },
        call: (binding, input, options) => new Promise(resolve => {
          calls.push({ binding, input, options });
          globalThis.finish = resolve;
        }),
      },
    };
  </script>`;
  const html = fs.readFileSync(path.join(root, "web/index.html"), "utf8").replace('<script type="module"', bridge + '<script type="module"');
  const server = http.createServer((request, response) => {
    response.setHeader("Access-Control-Allow-Origin", "*");
    if (request.url === "/") {
      response.setHeader("Content-Type", "text/html");
      return response.end(`<iframe sandbox="${tokens[0]}" src="/web/index.html" width="900" height="500"></iframe>`);
    }
    if (request.url === "/web/index.html") {
      response.setHeader("Content-Type", "text/html");
      return response.end(html);
    }
    if (request.url === "/web/app.js") {
      response.setHeader("Content-Type", "text/javascript");
      return response.end(fs.readFileSync(path.join(root, "web/app.js")));
    }
    response.writeHead(404).end();
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  let browser;
  try {
    browser = await chromium.launch({ headless: true, executablePath: process.env.CC_CHROMIUM,
      args: ["--no-sandbox", "--disable-background-networking", "--disable-component-update"] });
    const page = await browser.newPage();
    const blocked = [], errors = [];
    const origin = `http://127.0.0.1:${server.address().port}`;
    await page.route("**/*", async route => {
      if (route.request().url().startsWith(origin + "/")) return route.continue();
      blocked.push(route.request().url());
      return route.abort();
    });
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(origin);
    const pane = page.frameLocator("iframe");
    await pane.locator('#account option[value="first"]').waitFor({ state: "attached" });
    assert.equal(await pane.locator('#account option[value="first"]').textContent(), "First account");
    assert.equal(await pane.locator('#account option[value="second"]').textContent(), "second");
    const frame = page.frames()[1];
    assert.equal(await pane.locator("input, a").count(), 0);
    assert.equal(await pane.locator("#status").isDisabled(), true);
    await pane.locator("#account").selectOption("first");
    await pane.locator("#status").click();
    await frame.waitForFunction(() => calls.length === 2);
    assert.deepEqual(await frame.evaluate(() => calls[1]), { binding: "account.status", input: {},
      options: { account: { id: "first", expectedGeneration: "1" } } });
    await pane.locator("#account").selectOption("second");
    await frame.evaluate(() => finish({ plan: "stale-plan" }));
    assert.match(await pane.locator("#output").innerText(), /Choose an account/);
    await pane.locator("#status").click();
    await frame.waitForFunction(() => calls.length === 3);
    assert.deepEqual(await frame.evaluate(() => calls[2].options.account), { id: "second", expectedGeneration: "2" });
    await frame.evaluate(() => finish({ plan: "current-plan", balance: 12 }));
    await pane.locator("#output").filter({ hasText: "current-plan" }).waitFor();
    await pane.locator("#account").selectOption("first");
    assert.doesNotMatch(await pane.locator("#output").innerText(), /current-plan/);
    assert.deepEqual(blocked, []);
    assert.deepEqual(errors, []);
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
});
