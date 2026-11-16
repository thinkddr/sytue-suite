/* SPDX-License-Identifier: Apache-2.0 */
/** Ordinary page scripts exercise enforced CSP, rather than DevTools evaluation bypasses. */
import assert from "node:assert/strict";
import { chromium } from "playwright";
import { cabecerasCerradas } from "../../apps/frontend/packages/config/src/astro-middleware.js";
import { matrixDomain } from "../../apps/frontend/packages/config/src/tokens.js";

const probe = `void (async () => {
  const result = {};
  try { await WebAssembly.compile(new Uint8Array([0,97,115,109,1,0,0,0])); result.wasm = true; } catch { result.wasm = false; }
  try { await fetch(${JSON.stringify(matrixDomain + "/_matrix/client/versions")}); result.matrix = true; } catch { result.matrix = false; }
  try { await fetch('https://blocked.example.test/probe'); result.foreign = true; } catch { result.foreign = false; }
  try { eval('1+1'); result.eval = true; } catch { result.eval = false; }
  document.body.dataset.probe = JSON.stringify(result);
})();`;
const server = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch: (request) =>
    new URL(request.url).pathname === "/probe.js"
      ? new Response(probe, { headers: { "content-type": "text/javascript" } })
      : new Response('<!doctype html><body><script src="/probe.js"></script>', {
          headers: {
            "content-type": "text/html",
            "content-security-policy":
              cabecerasCerradas(undefined)["content-security-policy"],
          },
        }),
});
let browser;
try {
  browser = await chromium.launch({
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
  });
  const page = await browser.newPage();
  let matrixRequests = 0,
    foreignRequests = 0;
  // All responses are artificial. No provider, account, credentials or live homeserver is contacted.
  await page.route(matrixDomain + "/**", async (route) => {
    matrixRequests++;
    await route.fulfill({
      status: 200,
      body: "{}",
      headers: {
        "access-control-allow-origin": "*",
        "content-type": "application/json",
      },
    });
  });
  await page.route("https://blocked.example.test/**", async (route) => {
    foreignRequests++;
    await route.fulfill({
      status: 200,
      body: "{}",
      headers: {
        "access-control-allow-origin": "*",
        "content-type": "application/json",
      },
    });
  });
  await page.goto(`http://127.0.0.1:${server.port}`);
  await page.waitForFunction(() => Boolean(document.body.dataset.probe));
  const actual = await page.evaluate(() =>
    JSON.parse(document.body.dataset.probe),
  );
  assert.deepEqual(actual, {
    wasm: true,
    matrix: true,
    foreign: false,
    eval: false,
  });
  assert.equal(matrixRequests, 1);
  assert.equal(foreignRequests, 0);
  console.log(
    JSON.stringify({
      browser: browser.version(),
      ...actual,
      matrixRequests,
      foreignRequests,
    }),
  );
} finally {
  await browser?.close();
  await server.stop(true);
}
