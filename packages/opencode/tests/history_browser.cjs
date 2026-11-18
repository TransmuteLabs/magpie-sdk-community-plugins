const assert = require("node:assert/strict");
const { readFile, writeFile } = require("node:fs/promises");
const path = require("node:path");
const { chromium } = require("playwright");

async function main() {
  const config = JSON.parse(await readFile(process.argv[2], "utf8"));
  const browser = await chromium.launch({ channel: "chromium" });
  const context = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce" });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  const requests = [], responses = [], errors = [], foreign = [];
  const cookie = config.cookie.split("=");
  await context.addCookies([{ name: cookie[0], value: cookie.slice(1).join("="), url: config.origin }]);
  page.on("request", request => {
    if (request.url().startsWith(config.origin + "/api/")) {
      let body;
      try { body = request.postDataJSON(); } catch {}
      requests.push({ path: new URL(request.url()).pathname, body });
    }
  });
  page.on("pageerror", error => errors.push(error.message));
  page.on("response", async response => {
    if (response.url() === config.origin + "/api/plugin-pages/history") {
      try {
        responses.push({ status: response.status(), body: await response.text() });
      } catch (error) {
        responses.push({ status: response.status(), error: error.message });
      }
    }
  });
  const app = await (await fetch(config.origin + "/app.js", { headers: { Cookie: config.cookie } })).text();
  const functionSource = name => {
    const start = app.indexOf("function " + name + "(");
    assert(start >= 0, "actual trusted parent function missing: " + name);
    const end = app.indexOf("\n}", start);
    assert(end > start);
    return app.slice(start, end + 2);
  };
  const dialog = functionSource("backdropClick") + "\nlet confirmationPending = null;\n" + functionSource("confirmAction");
  const html = `<!doctype html><meta charset="utf-8"><h1>Isolated installed history fixture</h1>
    <iframe id="module" sandbox="allow-scripts" style="width:100%;height:800px"></iframe>
    <script>
      const NO_SCROLL={preventScroll:true};
      const t=value=>value;
      const el=(tag,cls,text)=>{const node=document.createElement(tag);node.className=cls||"";if(text)node.textContent=text;return node};
      ${dialog}
      window.confirmAction=confirmAction; window.t=t;
      window.api=async(route,body,headers,signal)=>{
        const response=await fetch("/api/"+route,{method:body===undefined?"GET":"POST",headers:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body),signal});
        const text=await response.text(); if(!response.ok)throw new Error("Owner response "+response.status);return text?JSON.parse(text):null;
      };
    </script>
    <script type="module">
      import {answerPluginPage} from "/extensionwindows.js";
      const frame=document.getElementById("module");
      const owner={entry:${JSON.stringify(config.page)},frame,abort:new AbortController(),shown:true};
      window.historyOwner=owner;
      addEventListener("message",async event=>{
        if(event.source!==frame.contentWindow||event.data?.magpie!==1||!event.data.id)return;
        const message=event.data;
        try{const result=await answerPluginPage(owner,message,()=>({dark:false,lang:"ru",tokens:{}}),()=>"OpenCode");
          if(owner.frame===frame&&!owner.abort.signal.aborted)frame.contentWindow.postMessage({magpie:1,id:message.id,result},"*");
        }catch(error){frame.contentWindow?.postMessage({magpie:1,id:message.id,error:error.message},"*");}
      });
      frame.src=${JSON.stringify(config.page.url)};
    </script>`;
  await page.route("**/*", route => {
    const url = new URL(route.request().url());
    if (url.origin !== config.origin) {
      foreign.push(url.origin);
      return route.abort();
    }
    if (url.pathname === "/history-fixture-parent") return route.fulfill({ contentType: "text/html", body: html });
    return route.continue();
  });
  try {
    await page.goto(config.origin + "/history-fixture-parent");
    const frame = page.frameLocator("#module");
    await frame.locator("#history-status").waitFor();
    if (config.unsupported) {
      await frame.locator("#history-status").filter({ hasText: "Просмотр истории недоступен" }).waitFor();
      assert.equal(await frame.locator("#history-access").isDisabled(), true);
      assert(!requests.some(request => request.path === "/api/plugin-pages/history"));
    } else {
      await frame.locator("#history-roots button").waitFor();
      assert.equal(await frame.locator("#history-access").isDisabled(), true);
      assert.equal(await frame.locator("#history-list").isDisabled(), true);
      const catalogCalls = () => requests.filter(request => request.body?.operation === "list");
      assert.equal(catalogCalls().length, 0);
      await frame.locator("#history-roots button").click();
      await frame.locator("#history-access").click();
      const dialog = page.getByRole("alertdialog");
      await dialog.waitFor();
      assert.match(await dialog.textContent(), /package and configuration/);
      assert.match(await dialog.textContent(), /allow Magpie to read/);
      assert.equal(await page.evaluate(() => document.activeElement?.textContent), "Cancel");
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await frame.locator("#history-status").filter({ hasText: "отменён" }).waitFor();
      assert.equal(catalogCalls().length, 0);
      assert(!requests.some(request => request.body?.operation === "request"));
      await frame.locator("#history-access").click();
      await dialog.waitFor();
      await dialog.getByRole("button", { name: "Continue", exact: true }).click();
      await frame.locator("#history-status").filter({ hasText: "Доступ разрешён" }).waitFor();
      await frame.locator("#history-list").click();
      await frame.locator("#history-sessions button").filter({ hasText: "Committed WAL session" }).click();
      await frame.locator("#history-transcript").filter({ hasText: "first split part" }).waitFor();
      const transcript = await frame.locator("#history-transcript").textContent();
      assert(transcript.indexOf("first split part") < transcript.indexOf("second split part"));
      assert(transcript.includes("current instruction") && !transcript.includes("retired instruction"));
      assert(transcript.includes("media unavailable: external"));
      const section = frame.locator("#history-media section").filter({ hasText: "image/png" }).first();
      await section.getByRole("button", { name: "Показать вложение" }).click();
      await section.locator("img").waitFor();
      const image = await section.locator("img").getAttribute("src");
      assert.match(image, /^data:image\/png;base64,/);
      const decoded = await section.locator("img").evaluate(async image => {
        await image.decode();
        return { width: image.naturalWidth, height: image.naturalHeight };
      });
      assert(decoded.width > 0 && decoded.height > 0, "selected raster bytes did not render");
      await page.screenshot({ path: path.join(config.output, "history-rendered.png"), fullPage: true });
      const frameObject = page.frames().find(current => current.url().includes(config.page.ref));
      assert(frameObject);
      const inspection = await frameObject.evaluate(() => magpie.history.inspect("opencode-history"));
      const selection = { source: "opencode-history", viewRef: inspection.viewRef,
        rootRef: inspection.roots.find(root => root.consented).rootRef };
      const selectedPage = await frameObject.evaluate(selection => magpie.history.list(selection), selection);
      const selected = selectedPage.entries.find(entry => entry.title === "Committed WAL session");
      const read = await frameObject.evaluate(selection => magpie.history.read(selection), {
        source: selection.source, viewRef: selection.viewRef, sessionRef: selected.sessionRef,
      });
      for (const negative of [
        { source: selection.source, viewRef: selection.viewRef, sessionRef: "f".repeat(64) },
        { source: "foreign", viewRef: selection.viewRef, sessionRef: selected.sessionRef },
        { source: selection.source, viewRef: "f".repeat(64), sessionRef: selected.sessionRef },
      ]) {
        assert.equal(await frameObject.evaluate(selection => magpie.history.read(selection).then(() => false, () => true), negative), true);
      }
      for (const negative of [
        { ...selection, sessionRef: selected.sessionRef, readRef: read.readRef, mediaRef: "f".repeat(64) },
        { source: selection.source, viewRef: selection.viewRef, sessionRef: selected.sessionRef, readRef: "f".repeat(64), mediaRef: read.media[0].mediaRef },
      ]) {
        const { rootRef, ...payload } = negative;
        assert.equal(await frameObject.evaluate(selection => magpie.history.media(selection).then(() => false, () => true), payload), true);
      }
      await frame.locator("#history-refresh").click();
      await frame.locator("#history-release").click();
      await frame.locator("#history-status").filter({ hasText: "отозвано" }).waitFor();
      assert.equal(await frame.locator("#history-list").isDisabled(), true);
      assert.equal(await frame.locator("#history-transcript").textContent(), "");
      assert.equal(await frame.locator("#history-media").textContent(), "");
      assert.equal(await frameObject.evaluate(selection => magpie.history.read(selection).then(() => false, () => true), {
        source: selection.source, viewRef: selection.viewRef, sessionRef: selected.sessionRef,
      }), true);
      const ordinary = await page.evaluate(async () => {
        const response = await fetch("/api/sessions/storage-sources");
        return response.json();
      });
      assert.equal(ordinary.consents.length, 0);
    }
    assert.deepEqual(foreign, []);
    assert.deepEqual(errors, []);
    assert(!requests.some(request => request.path === "/api/plugin-pages/auth" || request.path === "/api/plugin-pages/call"));
    const result = {
      chromium: true, installedNativePackage: true, actualHostHTTP: true, syntheticParentHarness: true,
      actualTrustedConfirmAction: true, actualParentBindings: true, actualPageBridge: true,
      actualOpenCodeProcess: false, liveAccountOrVendor: false, unsupported: config.unsupported,
      requests, responses, errors, foreign,
    };
    await writeFile(path.join(config.output, "history-browser-result.json"), JSON.stringify(result, null, 2) + "\n");
    console.log(JSON.stringify(result));
  } catch (error) {
    const frame = page.frames().find(current => current.url().includes(config.page.ref));
    const failure = {
      error: error.stack, requests, responses, errors, foreign,
      history: frame ? await frame.locator("#history-panel").textContent().catch(() => "") : "",
    };
    await writeFile(path.join(config.output, "history-browser-failure.json"), JSON.stringify(failure, null, 2) + "\n");
    await page.screenshot({ path: path.join(config.output, "history-failed.png"), fullPage: true }).catch(() => {});
    throw error;
  } finally {
    await context.close();
    await browser.close();
  }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
