/* SPDX-License-Identifier: Apache-2.0 */
/** Actual browser byte upload -> shared cookie BFF -> synthetic opaque provider. No real accounts. */
import assert from "node:assert/strict";
import { chromium } from "playwright";
import { rutaBlobsMatrixPrivados } from "../../apps/frontend/packages/auth/src/server/matrix-storage-blobs.js";
import { cabecerasCerradas } from "../../apps/frontend/packages/config/src/astro-middleware.js";
const built = await Bun.build({
  entrypoints: [
    new URL("./storage-gateway-client.ts", import.meta.url).pathname,
  ],
  target: "browser",
});
assert.equal(built.success, true, built.logs.map(String).join("\n"));
const script = await built.outputs[0].text();
let tenant = "acme",
  stored,
  receipt;
const seen = [];
const handler = rutaBlobsMatrixPrivados({
  product: "mail",
  env: { SYTUE_DRIVE_INTERNAL_URL: "https://internal.test" },
  bff: {
    getSession: async () => ({
      user: { id: "person", tenant },
      tokens: { accessToken: "artificial-server-only-token" },
    }),
  },
  fetchImpl: async (input, init) => {
    const request = new Request(input, init);
    assert.equal(
      request.headers.get("authorization"),
      "Bearer artificial-server-only-token",
    );
    seen.push({
      method: request.method,
      length: request.headers.get("content-length"),
    });
    if (request.method === "PUT") {
      stored = new Uint8Array(await request.arrayBuffer());
      const sha256 = Buffer.from(
        await crypto.subtle.digest("SHA-256", stored),
      ).toString("hex");
      receipt = {
        id: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
        product: "mail",
        size_bytes: stored.length,
        sha256,
      };
      return Response.json(receipt, { status: 201 });
    }
    if (new URL(request.url).pathname.endsWith("/content"))
      return new Response(stored, {
        headers: { "content-type": "application/octet-stream" },
      });
    return Response.json(receipt);
  },
});
const server = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch: async (request) => {
    const url = new URL(request.url);
    if (url.pathname === "/probe.js")
      return new Response(script, {
        headers: { "content-type": "text/javascript" },
      });
    if (url.pathname === "/switch" && request.method === "POST") {
      tenant = "other";
      return new Response(null, { status: 204 });
    }
    if (url.pathname.startsWith("/api/matrix/storage/blobs/"))
      return handler({
        request,
        url,
        params: {
          ruta: url.pathname.slice("/api/matrix/storage/blobs/".length),
        },
        cookies: {},
      });
    if (url.pathname !== "/") return new Response(null, { status: 404 });
    return new Response(
      '<!doctype html><script type="module" src="/probe.js"></script>',
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
  const origin = `http://127.0.0.1:${server.port}`;
  await context.route("**/*", (route) =>
    new URL(route.request().url()).origin === origin
      ? route.continue()
      : route.abort(),
  );
  const page = await context.newPage();
  await page.goto(origin);
  await page.waitForFunction(
    () => window.storageGatewayProbe,
    {},
    { timeout: 60000 },
  );
  const result = await page.evaluate(() => window.storageGatewayProbe);
  assert.equal(result.ok, true, result.error);
  assert.equal(result.bytes, 8 << 20);
  assert.deepEqual(seen, [
    { method: "PUT", length: String(8 << 20) },
    { method: "GET", length: null },
    { method: "GET", length: null },
  ]);
  assert.equal(stored.length, 8 << 20);
  console.log(
    JSON.stringify({
      browser: await browser.version(),
      ...result,
      upstreamRequests: seen.length,
      scopeChangesDenied: 3,
    }),
  );
} finally {
  await browser?.close();
  server.stop(true);
  stored?.fill(0);
}
