// SPDX-License-Identifier: Apache-2.0
// Standalone remote QA: real IndexedDB, synthetic data, no production services.
import assert from "node:assert/strict";
import { chromium } from "playwright";
import { nativeWorkerResponse } from "../../apps/frontend/packages/offline/src/worker.ts";

const result = await Bun.build({
  entrypoints: [new URL("./offline-cas-client.ts", import.meta.url).pathname],
  target: "browser",
});
assert.equal(result.success, true, result.logs.map(String).join("\n"));
const script = await result.outputs[0].text();
async function bounded(promise, label) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out`)), 15000);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}
const server = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch(request) {
    if (new URL(request.url).pathname === "/sw.js") return nativeWorkerResponse();
    if (new URL(request.url).pathname === "/fixture.js") {
      return new Response(script, { headers: { "content-type": "text/javascript" } });
    }
    return new Response('<!doctype html><script type="module" src="/fixture.js"></script>', {
      headers: { "content-type": "text/html", "cache-control": "no-store" },
    });
  },
});

let browser;
try {
  browser = await chromium.launch({
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
  });
  const context = await browser.newContext();
  await context.route("**/*", (route) =>
    new URL(route.request().url()).origin === `http://127.0.0.1:${server.port}`
      ? route.continue()
      : route.abort(),
  );
  const page = await context.newPage();
  const other = await context.newPage();
  const origin = `http://127.0.0.1:${server.port}`;
  for (const tab of [page, other]) {
    await tab.goto(origin);
    await tab.waitForFunction(() => window.offlineCASProbe, null, { timeout: 15000 });
  }
  await page.evaluate(() => window.offlineCASProbe.clear());
  assert.equal(await page.evaluate(() => window.offlineCASProbe.create("initial")), true);
  await Promise.all(
    [page, other].map((tab) => tab.evaluate(() => window.offlineCASProbe.capture())),
  );

  // Hold the write lock while both tabs queue their competing
  // readwrite transactions. Releasing it proves only one stale witness can win.
  await bounded(
    page.evaluate(() => window.offlineCASProbe.gateTransactions()),
    "acquire gate",
  );
  const counts = await Promise.all(
    [page, other].map((tab) => tab.evaluate(() => window.offlineCASProbe.queuedTransactions())),
  );
  await Promise.all([
    page.evaluate(() => window.offlineCASProbe.begin("replace")),
    other.evaluate(() => window.offlineCASProbe.begin("delete")),
  ]);
  await Promise.all(
    [page, other].map((tab, i) =>
      tab.waitForFunction(
        (previous) => window.offlineCASProbe.queuedTransactions() > previous,
        counts[i],
        { timeout: 15000 },
      ),
    ),
  );
  await bounded(
    page.evaluate(() => window.offlineCASProbe.releaseGate()),
    "release gate",
  );
  const outcomes = await bounded(
    Promise.all([page, other].map((tab) => tab.evaluate(() => window.offlineCASProbe.outcome()))),
    "CAS race",
  );
  assert(
    outcomes.every((result) => result.ok),
    JSON.stringify(outcomes),
  );
  assert.equal(outcomes.filter((result) => result.value).length, 1, JSON.stringify(outcomes));
  let current = await page.evaluate(() => window.offlineCASProbe.read());
  assert.equal(current.present, outcomes[0].value);

  if (!current.present) {
    await page.evaluate(() => window.offlineCASProbe.capture());
    await page.evaluate(() => window.offlineCASProbe.begin("recreate"));
    assert.deepEqual(await page.evaluate(() => window.offlineCASProbe.outcome()), {
      ok: true,
      value: true,
    });
  }
  await Promise.all(
    [page, other].map((tab) => tab.evaluate(() => window.offlineCASProbe.capture())),
  );
  await page.evaluate(() => window.offlineCASProbe.begin("delete"));
  assert.deepEqual(await page.evaluate(() => window.offlineCASProbe.outcome()), {
    ok: true,
    value: true,
  });
  await other.evaluate(() => window.offlineCASProbe.begin("recreate"));
  assert.deepEqual(await other.evaluate(() => window.offlineCASProbe.outcome()), {
    ok: true,
    value: false,
  });
  assert.equal(await page.evaluate(() => window.offlineCASProbe.createVirgin()), false);
  await other.evaluate(() => window.offlineCASProbe.capture());
  await other.evaluate(() => window.offlineCASProbe.begin("recreate"));
  assert.deepEqual(await other.evaluate(() => window.offlineCASProbe.outcome()), {
    ok: true,
    value: true,
  });
  current = await page.evaluate(() => window.offlineCASProbe.read());
  assert.deepEqual(current, { present: true, value: "recreated" });

  // Legacy remains readable only through its own API; v2 neither adopts nor
  // deletes it. This does not fence old writers or authorize replay.
  await page.evaluate(() => window.offlineCASProbe.legacyWrite());
  assert.equal(await other.evaluate(() => window.offlineCASProbe.legacyRead()), "legacy");
  assert.deepEqual(await other.evaluate(() => window.offlineCASProbe.read()), {
    present: true,
    value: "recreated",
  });

  const isolated = await browser.newContext();
  const independent = await isolated.newPage();
  await independent.goto(origin);
  await independent.waitForFunction(() => window.offlineCASProbe, null, { timeout: 15000 });
  assert.deepEqual(await independent.evaluate(() => window.offlineCASProbe.read()), {
    present: null,
    value: null,
  });
  await isolated.close();

  // Start an encryption in the second tab, then clear from the first. The v2
  // connection receives versionchange before the paused operation may commit.
  await other.evaluate(() => window.offlineCASProbe.capture());
  const oldEpoch = await other.evaluate(() => window.offlineCASProbe.epoch());
  await other.evaluate(() => window.offlineCASProbe.pauseEncryption());
  await other.evaluate(() => window.offlineCASProbe.begin("replace"));
  await other.waitForFunction(() => window.offlineCASProbe.encryptionEntered(), null, {
    timeout: 15000,
  });
  await bounded(
    page.evaluate(() => window.offlineCASProbe.clear()),
    "cross-tab logout",
  );
  assert((await other.evaluate(() => window.offlineCASProbe.epoch())) > oldEpoch);
  await other.evaluate(() => window.offlineCASProbe.releaseEncryption());
  assert.deepEqual(
    await bounded(
      other.evaluate(() => window.offlineCASProbe.outcome()),
      "stale write",
    ),
    { ok: false },
  );
  assert.deepEqual(await page.evaluate(() => window.offlineCASProbe.read()), {
    present: null,
    value: null,
  });
  assert.equal(await page.evaluate(() => window.offlineCASProbe.create("fresh session")), true);
  assert.deepEqual(await page.evaluate(() => window.offlineCASProbe.read()), {
    present: true,
    value: "fresh session",
  });
  await page.evaluate(() => window.offlineCASProbe.legacyWrite());
  await bounded(
    page.evaluate(async () => {
      await navigator.serviceWorker.register("/sw.js");
      await navigator.serviceWorker.ready;
      if (!navigator.serviceWorker.controller) {
        await new Promise((resolve) =>
          navigator.serviceWorker.addEventListener("controllerchange", resolve, { once: true }),
        );
      }
    }),
    "service worker activation",
  );
  // Do not register a window logout listener: the worker must attempt both
  // deletions itself. Keep one real connection open to exercise versionchange.
  await other.evaluate(() => window.offlineCASProbe.holdDatabaseAcrossLogout());
  assert.equal(await page.evaluate(async () => (await fetch("/auth/logout")).status), 200);
  await other.waitForFunction(() => window.offlineCASProbe.versionchangeSeen(), null, {
    timeout: 15000,
  });
  assert(
    (await page.evaluate(async () => (await indexedDB.databases()).map((db) => db.name))).includes(
      "sytue-offline-cas-v2",
    ),
  );
  await other.evaluate(() => window.offlineCASProbe.releaseHeldDatabase());
  await page.waitForFunction(
    async () => {
      const names = (await indexedDB.databases()).map((db) => db.name);
      return !names.includes("sytue-offline") && !names.includes("sytue-offline-cas-v2");
    },
    null,
    { timeout: 15000 },
  );
  await bounded(
    page.evaluate(() => window.offlineCASProbe.clear()),
    "final cleanup",
  );
  await context.close();
  console.log(
    JSON.stringify({
      browser: browser.version(),
      competingTransactions: true,
      aba: true,
      v2LogoutInFlight: true,
      serviceWorkerVersionchange: true,
      legacyFenced: false,
    }),
  );
} finally {
  await browser?.close();
  await server.stop(true);
}
