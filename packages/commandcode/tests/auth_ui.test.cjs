// Run against built package assets with the host's real sandbox token.
// Fixtures never contact a vendor or read credentials. See the guarded usbox runner.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { test } = require('node:test');
const playwright = require('playwright');

const root = process.env.CC_UI_ROOT;
const host = process.env.CC_HOST_ROOT;
assert.ok(root && host, 'Set CC_UI_ROOT to the built package and CC_HOST_ROOT to the frozen host');
const renderer = fs.readFileSync(path.join(host, 'internal/gui/assets/pluginpages.js'), 'utf8');
const tokens = [...renderer.matchAll(/f\.setAttribute\("sandbox", "([^"]+)"\)/g)].map(match => match[1]);
assert.ok(tokens.length >= 2);
assert.ok(tokens.every(value => value === 'allow-scripts'), 'Do not relax the host sandbox');
const sandbox = tokens[0];
const approval = 'https://approval.example.invalid/authorize?state=fixture&redirect_uri=http%3A%2F%2F127.0.0.1%2Fcallback';

test('packaged auth page offers safe manual approval inside the ordinary sandbox', { timeout: 45000 }, async () => {
  let next = { flow: 'synthetic', state: 'waiting', url: approval };
  let terminal = 'complete';
  let base;
  const requests = [], blocked = [], failures = [];
  const server = http.createServer((req, res) => {
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Cache-Control', 'no-store');
    res.setHeader('Content-Type', req.url.endsWith('.js') ? 'text/javascript' : 'text/html');
    if (req.url === '/parent') return res.end(`<iframe sandbox="${sandbox}" src="${base}/web/index.html" width="900" height="900"></iframe>`);
    if (req.url === '/web/index.html') {
      const fixture = JSON.stringify({ next, terminal }).replaceAll('<', '\\u003c');
      const bridge = `<script>const fixture=${fixture};globalThis.trace=[];document.addEventListener('click',e=>trace.push(['click',e.target.id]),true);globalThis.magpie={ready:Promise.resolve(),backend:{accounts:async()=>[]},config:{get:async()=>({version:'0',values:{}})},auth:{start:async()=>{trace.push(['start']);return fixture.next},finish:async()=>({flow:'synthetic',state:fixture.terminal}),cancel:async()=>({flow:'synthetic',state:'canceled'})}};</script>`;
      return res.end(fs.readFileSync(path.join(root, 'web/index.html'), 'utf8').replace('<script type="module"', bridge + '<script type="module"'));
    }
    const relative = req.url.split('?')[0];
    if (/^\/web\/[a-zA-Z0-9_./-]+\.js$/.test(relative) && !relative.includes('..')) {
      const file = path.join(root, relative);
      if (fs.existsSync(file)) return res.end(fs.readFileSync(file));
    }
    res.statusCode = 404;
    res.end();
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  base = 'http://127.0.0.1:' + server.address().port;
  let browser;
  try {
    const engine = process.env.BROWSER || 'chromium';
    browser = await playwright[engine].launch({ headless: true,
      ...(engine === 'chromium' ? { executablePath: process.env.CC_CHROMIUM, args: ['--no-sandbox'] } : {}),
    });
    const context = await browser.newContext();
    // Even a broken consumer cannot send a request to an external destination.
    await context.route('**/*', async route => {
      const url = route.request().url();
      requests.push(url);
      if (url.startsWith(base + '/')) return route.continue();
      blocked.push(url);
      return route.abort();
    });
    let page;
    const errors = [];
    const load = async value => {
      next = value;
      if (page) await page.close();
      page = await context.newPage();
      page.on('pageerror', error => failures.push(error.message));
      page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
      await page.goto(base + '/parent');
      const frame = page.frameLocator('iframe');
      await frame.locator('#output').filter({ hasText: 'Choose an account' }).waitFor();
      // Focus and keyboard-activate the real button, as for the auth
      // controls below.
      await frame.locator('#browser').focus();
      await frame.locator('#browser').press('Enter');
      try { await frame.locator('#output').filter({ hasText: 'Sign-in:' }).waitFor({ timeout: 5000 }); }
      catch (error) { console.log(JSON.stringify({ fixture: value, body: await frame.locator('body').innerText(), errors, failures, trace: await frame.locator('body').evaluate(el => el.ownerDocument.defaultView.trace) })); throw error; }
      return frame;
    };
    let frame = await load(next);
    // Preserve actual Chromium diagnosis on the old source before asserting the fix.
    if (await frame.locator('#approve').count()) {
      await frame.locator('#approve').click();
      await page.waitForTimeout(100);
      console.log(JSON.stringify({ oldPopup: { sandbox, errors, pages: context.pages().length, blocked } }));
    }
    assert.equal(await frame.locator('#auth-url').count(), 1, 'The sandbox blocks approval popups: supply a visible selectable URL fallback');
    const field = () => frame.locator('#auth-url');
    const assertCleared = async () => {
      assert.equal(await field().inputValue(), '');
      assert.equal(await frame.locator('#auth-address').isVisible(), false);
    };
    for (const state of ['waiting', 'pending']) {
      frame = await load({ flow: 'synthetic', state, url: approval });
      assert.equal(await field().isVisible(), true);
      assert.equal(await field().inputValue(), approval);
      assert.equal(await field().getAttribute('readonly'), '');
      assert.match(await frame.locator('#auth-address').innerText(), /copy|копир/i);
      await frame.locator('#auth-select').focus();
      await frame.locator('#auth-select').press('Enter');
      assert.deepEqual(await field().evaluate(input => [input.selectionStart, input.selectionEnd, input.ownerDocument.activeElement === input]), [0, approval.length, true]);
      assert.equal(context.pages().length, 1);
      console.log('PASS manual selection', state);
    }
    for (const state of ['complete', 'canceled', 'expired', 'uncertain']) {
      terminal = state;
      frame = await load({ flow: 'synthetic', state: 'waiting', url: approval });
      await frame.locator('#finish').focus();
      await frame.locator('#finish').press('Enter');
      await frame.locator('#finish').waitFor({ state: 'hidden' });
      await assertCleared();
      console.log('PASS clears terminal', state);
    }
    frame = await load({ flow: 'synthetic', state: 'pending', url: approval });
    await frame.locator('#cancel').focus();
    await frame.locator('#cancel').press('Enter');
    await frame.locator('#cancel').waitFor({ state: 'hidden' });
    await assertCleared();
    for (const url of ['', 'javascript:alert(1)', 'data:text/html,unsafe', 'http://approval.example.invalid/', 'https://user:pass@approval.example.invalid/', '//approval.example.invalid/', 'not a url', 'https://approval.example.invalid/\nunsafe']) {
      frame = await load({ flow: 'synthetic', state: 'waiting', url });
      await assertCleared();
      console.log('PASS refuses unsafe URL', JSON.stringify(url));
    }
    frame = await load({ flow: 'synthetic', state: 'waiting', url: approval });
    await field().evaluate(input => input.ownerDocument.defaultView.dispatchEvent(new Event('pagehide')));
    await assertCleared();
    assert.deepEqual(blocked, [], 'No external navigation or popup attempt is needed');
    assert.deepEqual(failures, [], 'Page runtime errors');
    console.log(JSON.stringify({ engine, sandbox, servedRequests: requests.length, blocked, pages: context.pages().length, terminal: 'pass' }));
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
});
