/* SPDX-License-Identifier: Apache-2.0 */
/** Real IndexedDB structured cloning, reload, key erasure and cross-tab logout. Synthetic data only. */
import assert from "node:assert/strict";
import { chromium } from "playwright";
import { cabecerasCerradas } from "../../apps/frontend/packages/config/src/astro-middleware.js";
const result = await Bun.build({
  entrypoints: [
    new URL("./encrypted-cache-client.ts", import.meta.url).pathname,
  ],
  target: "browser",
});
assert.equal(result.success, true, result.logs.map(String).join("\n"));
const script = await result.outputs[0].text();
const server = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch: (request) => {
    if (new URL(request.url).pathname === "/probe.js")
      return new Response(script, {
        headers: { "content-type": "text/javascript" },
      });
    return new Response(
      '<!doctype html><body><script type="module" src="/probe.js"></script>',
      {
        headers: {
          "content-type": "text/html",
          "content-security-policy":
            cabecerasCerradas(undefined)["content-security-policy"],
        },
      },
    );
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
  const ready = async (target, phase) => {
    await target.waitForFunction(
      () =>
        window.encryptedCacheProbe?.ok ||
        window.encryptedCacheProbe?.stage.startsWith("failed:"),
    );
    const state = await target.evaluate(() => window.encryptedCacheProbe);
    assert.equal(state.ok, true, JSON.stringify(state));
    assert.equal(state.stage, phase);
    return state;
  };
  await page.goto(`http://127.0.0.1:${server.port}`);
  await ready(page, "written");
  const other = await context.newPage();
  await other.goto(`http://127.0.0.1:${server.port}/?mode=hold`);
  await ready(other, "holding");
  await page.goto(`http://127.0.0.1:${server.port}/?mode=reload`);
  const state = await ready(page, "complete");
  assert.equal(
    await other.evaluate(() => window.encryptedCacheLateWrite()),
    true,
    "another tab restored keys after logout",
  );
  console.log(
    JSON.stringify({
      browser: browser.version(),
      state,
      reload: true,
      nonExtractable: true,
      crossTabLogout: true,
    }),
  );
} finally {
  await browser?.close();
  await server.stop(true);
}
