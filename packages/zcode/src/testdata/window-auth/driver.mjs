import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { createFrame, assertPublic } from "./realms.mjs";
import { createHash } from "node:crypto";

const config = JSON.parse(await readFile(process.argv[2], "utf8"));
const bridgeSource = await (await fetch(config.origin + config.pages.zcode.url.replace("index.html", "_magpie/bridge.js"))).text();
const parentResponse = await fetch(config.origin + "/extensionwindows.js", { headers: { Cookie: config.cookie } });
assert.equal(parentResponse.ok, true, "actual parent relay asset unavailable");
const parentSource = await parentResponse.text();
const sdkSource = await readFile(config.sdkUI, "utf8");
const sha = (value) => createHash("sha256").update(value).digest("hex");
assert.equal(sha(bridgeSource), config.bridgeSHA256, "served bridge differs from frozen host source");
assert.equal(sha(parentSource), config.parentSHA256, "served parent relay differs from frozen host source");
assert.equal(sha(sdkSource), config.sdkUISHA256, "public browser SDK differs from frozen source");
const frames = [];
const receipts = {};
const evidence = [];
const quotaEvidence = [];
const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));
async function instrument(path, body) {
  const response = await fetch(config.vendor + "/__fixture__/" + path, body === undefined ? {} : {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  assert.equal(response.ok, true, "synthetic instrumentation failed");
  return response.json();
}
async function frame(kind) {
  const value = await createFrame({
    page: config.pages[kind], origin: config.origin, cookie: config.cookie,
    bridgeSource, parentSource, sdkSource, secrets: config.secrets,
  });
  frames.push(value);
  return value;
}
function options(kind, account) {
  return {
    method: kind === "zcode" ? "browser" : "device",
    ...(kind === "zcode" ? { input: { site: "zai", kind: "start" } } : {}),
    ...(account ? { account: { id: account.id, expectedGeneration: account.generation } } : {}),
  };
}
async function finish(value, view) {
  for (let index = 0; index < 6; index++) {
    if (view.state === "complete" || view.state === "uncertain" || view.state === "canceled") return view;
    await sleep(Math.max(view.retryAfterMs ?? 1000, 1000) + 40);
    view = await value.ui.auth.finish("signin", view.flow);
  }
  throw new Error("bounded consumer polling did not reach a terminal receipt");
}
async function denied(work) {
  await assert.rejects(work, /./);
}
function providerID(kind) {
  return "extension-" + createHash("sha256").update(kind + "\0" + (kind === "zcode" ? "zcode" : "opencode-go")).digest("hex").slice(0, 24);
}
async function gateway(kind, succeeds) {
  const response = await fetch(config.gateway + (kind === "zcode" ? "/v1/messages" : "/v1/chat/completions"), {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ model: providerID(kind) + "/" + (kind === "zcode" ? "GLM-5.3" : "deepseek-v4.1-flash"),
      messages: [{ role: "user", content: "synthetic fixture" }], max_tokens: 32 }),
  });
  const body = await response.text();
  for (const secret of config.secrets) assert.equal(body.includes(secret), false, "ordinary gateway disclosed a fixture secret");
  assert.equal(response.ok, succeeds, `ordinary gateway authority: status ${response.status}, ${body.slice(0, 300)}`);
  if (!succeeds) assert.match(body, /usage|quota.{0,40}refresh|acknowledg/iu, "gateway refusal did not identify the quota barrier");
}
async function accountState(kind, id) {
  return instrument("account", { kind, id });
}
async function quotaRefresh(kind, id) {
  const response = await fetch(config.origin + "/api/usage/quotas/refresh?provider=" + encodeURIComponent(providerID(kind)) + "&user=" + encodeURIComponent(id), {
    method: "POST", headers: { Cookie: config.cookie, Origin: config.origin, "Sec-Fetch-Site": "same-origin" },
  });
  assert.equal(response.ok, true, "ordinary quota refresh HTTP route failed");
  const text = await response.text();
  for (const secret of config.secrets) assert.equal(text.includes(secret), false, "quota UI disclosed a fixture secret");
  const result = JSON.parse(text);
  quotaEvidence.push(result);
  return result;
}

try {
  for (const kind of ["zcode", "opencode"]) {
    await instrument("mode", { kind, mode: "success" });
    const current = await frame(kind);
    const before = await instrument("state");
    const initial = await current.ui.auth.start("signin", options(kind));
    assertPublic(initial, config.secrets);
    assert.ok(initial.url && initial.state !== "complete");
    assert.equal(typeof initial.flow, "string");
    const startMessage = current.messages.outgoing.find((value) => value.method === "auth.start");
    const duplicate = await current.replay(startMessage);
    assert.equal(duplicate.result.flow, initial.flow);
    assert.equal((await instrument("state")).started[kind], before.started[kind] + 1);
    const changed = structuredClone(startMessage);
    changed.params.options.method = "undeclared";
    assert.ok((await current.replay(changed)).error, "changed start intent was admitted");
    const changedInput = structuredClone(startMessage);
    changedInput.params.options.input = kind === "zcode" ? { site: "bigmodel", kind: "start" } : { unexpected: true };
    assert.ok((await current.replay(changedInput)).error, "same request ID accepted a changed authentication input");
    assert.equal((await instrument("state")).started[kind], before.started[kind] + 1);
    const foreignParent = await frame(kind);
    await denied(() => foreignParent.ui.auth.finish("signin", initial.flow));
    const otherRoot = await frame(kind === "zcode" ? "opencode" : "zcode");
    await denied(() => otherRoot.ui.auth.finish("signin", initial.flow));
    await denied(() => current.ui.auth.finish("undeclared", initial.flow));
    const early = await current.ui.auth.finish("signin", initial.flow);
    const completed = await finish(current, early);
    assert.equal(completed.state, "complete");
    assert.deepEqual(Object.keys(completed.account).sort(), ["expiresAt", "generation", "id"]);
    assert.equal(completed.account.generation, "1");
    const counts = await instrument("state");
    const completedMessage = current.messages.outgoing.filter((value) => value.method === "auth.finish").at(-1);
    assert.equal((await current.replay(completedMessage)).result.account.generation, "1");
    assert.equal((await current.ui.auth.finish("signin", initial.flow)).account.generation, "1");
    assert.equal((await current.ui.auth.cancel("signin", initial.flow)).state, "complete");
    assert.equal((await instrument("state")).polled[kind], counts.polled[kind], "completed auth exchanged again");
    let target = completed.account;
    let beforeReauth;
    if (kind === "opencode") {
      await gateway(kind, true);
      target = (await current.ui.backend.accounts("selected")).find((value) => value.id === completed.account.id);
      assert.equal(target.generation, "2", "ordinary execution did not publish a real completed refresh");
      beforeReauth = await accountState(kind, target.id);
      assert.equal(beforeReauth.refreshState, "completed");
      assert.ok(beforeReauth.refreshSHA256);
    }
    const selected = await current.ui.backend.call("selected", {}, {
      account: { id: target.id, expectedGeneration: target.generation },
    });
    assert.deepEqual(selected, { id: target.id, generation: target.generation, selectedAuthPresent: true });
    if (kind === "zcode") {
      await gateway(kind, true);
      await current.ui.backend.call("fixture.invalidate", {}, {
        account: { id: target.id, expectedGeneration: target.generation },
      });
      beforeReauth = await accountState(kind, target.id);
      assert.ok(beforeReauth.usage.revision > beforeReauth.usage.acknowledged);
      await gateway(kind, false);
    }
    const renewed = await finish(current, await current.ui.auth.start("signin", options(kind, target)));
    assert.equal(renewed.state, "complete");
    assert.equal(renewed.account.id, completed.account.id);
    const expectedGeneration = kind === "zcode" ? "2" : "3";
    assert.equal(renewed.account.generation, expectedGeneration);
    await denied(() => current.ui.auth.start("signin", options(kind, target)));
    await denied(() => current.ui.backend.call("selected", {}, {
      account: { id: target.id, expectedGeneration: target.generation },
    }));
    assert.equal((await current.ui.backend.call("selected", {}, {
      account: { id: renewed.account.id, expectedGeneration },
    })).generation, expectedGeneration);
    const afterReauth = await accountState(kind, renewed.account.id);
    if (kind === "zcode") {
      assert.deepEqual(afterReauth.usage, beforeReauth.usage, "reauthorization erased or acknowledged the dirty usage epoch");
      await gateway(kind, false);
      await current.ui.backend.call("fixture.allow-usage", {}, {
        account: { id: renewed.account.id, expectedGeneration },
      });
      const quota = await quotaRefresh(kind, renewed.account.id);
      const acknowledged = await accountState(kind, renewed.account.id);
      assert.equal(acknowledged.usage.revision, beforeReauth.usage.revision);
      assert.equal(acknowledged.usage.acknowledged, beforeReauth.usage.revision,
        "fresh quota read did not acknowledge: " + JSON.stringify(quota).slice(0, 1800));
      await gateway(kind, true);
    } else {
      assert.equal(afterReauth.refreshSHA256, beforeReauth.refreshSHA256, "reauthorization dropped or changed a completed refresh receipt");
    }
    receipts[kind] = renewed.account;
    const canceled = await current.ui.auth.start("signin", options(kind));
    assert.equal((await current.ui.auth.cancel("signin", canceled.flow)).state, "canceled");
    const cancelMessage = current.messages.outgoing.filter((value) => value.method === "auth.cancel").at(-1);
    assert.equal((await current.replay(cancelMessage)).result.state, "canceled");
    assert.equal((await current.ui.auth.cancel("signin", canceled.flow)).state, "canceled");
    const cancelCounts = await instrument("state");
    assert.equal((await current.ui.auth.finish("signin", canceled.flow)).state, "canceled");
    assert.equal((await instrument("state")).polled[kind], cancelCounts.polled[kind]);
    await instrument("mode", { kind, mode: "error" });
    const uncertain = await finish(current, await current.ui.auth.start("signin", options(kind)));
    assert.equal(uncertain.state, "uncertain");
    const errorCounts = await instrument("state");
    assert.equal((await current.ui.auth.finish("signin", uncertain.flow)).state, "uncertain");
    assert.equal((await instrument("state")).polled[kind], errorCounts.polled[kind], "uncertain exchange replayed");
    evidence.push({ kind, initialGeneration: "1", renewedGeneration: expectedGeneration, duplicatePublicationRefused: true,
      dirtyEpochPreservedUntilRealUsageAck: kind === "zcode", completedRefreshReceiptPreserved: kind === "opencode" });
  }

  const held = await frame("opencode");
  await instrument("mode", { kind: "opencode", mode: "hold" });
  const waiting = await held.ui.auth.start("signin", options("opencode", receipts.opencode));
  await sleep(Math.max(waiting.retryAfterMs ?? 1000, 1000) + 40);
  const finishing = held.ui.auth.finish("signin", waiting.flow);
  for (let index = 0; index < 100 && !(await instrument("state")).held; index++) await sleep(20);
  assert.ok((await instrument("state")).held, "fixture did not enter a held vendor exchange");
  await held.ui.auth.cancel("signin", waiting.flow);
  await instrument("release", {});
  const ended = await finishing;
  assert.ok(["uncertain", "canceled"].includes(ended.state), "canceled vendor exchange published an account");
  const heldCounts = await instrument("state");
  const heldReceipt = await held.ui.auth.finish("signin", waiting.flow);
  assert.ok(["uncertain", "canceled"].includes(heldReceipt.state));
  assert.equal((await instrument("state")).polled.opencode, heldCounts.polled.opencode);
  for (const kind of ["zcode", "opencode"]) {
    await instrument("mode", { kind, mode: "success" });
    const old = await frame(kind);
    const pending = await old.ui.auth.start("signin", options(kind));
    const beforeOff = await instrument("state");
    await instrument("disable", { kind });
    await denied(() => old.ui.auth.finish("signin", pending.flow));
    await denied(() => old.ui.auth.cancel("signin", pending.flow));
    await denied(() => old.ui.auth.start("signin", options(kind)));
    assert.equal((await instrument("state")).polled[kind], beforeOff.polled[kind]);
  }
  const snapshot = await instrument("state");
  assert.deepEqual(snapshot.errors, []);
  assert.deepEqual(snapshot.inferences, { zcode: 2, opencode: 1 }, "blocked or canceled flow reached an inference endpoint");
  assert.equal(snapshot.refreshes, 1, "completed refresh was repeated or bypassed");
  assert.ok(frames.every((value) => value.violations.length === 0), "a guarded privacy violation was hidden by an expected rejection");
  await writeFile(config.output, JSON.stringify({
    passed: true, receipts, evidence, quotaEvidence,
    iframeMessages: frames.flatMap((value) => [...value.messages.outgoing, ...value.messages.incoming]),
    http: frames.flatMap((value) => value.messages.http),
    fixture: snapshot,
    actualPageBridge: true, actualParentRelay: true, actualWebHTTP: true,
    nativeVendorControllers: true, noCLIAuth: true, renderedBrowser: false,
    disabledPageAuthorityRefused: true,
    servedAssetsMatchFrozenSource: true,
  }, null, 2));
} finally {
  for (const value of frames) value.dispose();
}
