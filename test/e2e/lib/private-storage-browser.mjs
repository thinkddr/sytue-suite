/* SPDX-License-Identifier: Apache-2.0 */
/** Actual Chromium, enforced CSP, maintained file crypto and an opaque synthetic gateway. */
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { chromium } from "playwright";
import { cabecerasCerradas } from "../../../apps/frontend/packages/config/src/astro-middleware.js";

export async function runPrivateStorageBrowser({
  directory,
  entrypoint,
  probe,
  reloads = 0,
  route,
  buildOptions = {},
  onDownload,
}) {
  assert(
    directory && path.isAbsolute(directory),
    "absolute disposable vector directory required",
  );
  assert(
    Number.isSafeInteger(reloads) && reloads >= 0 && reloads <= 5,
    "bounded fixture reloads required",
  );
  const fixture = await Bun.file(path.join(directory, "vector.json")).json();
  assert.equal(fixture.artificial, true);
  const sdk = createRequire(
    new URL(
      "../../../apps/frontend/packages/offline/package.json",
      import.meta.url,
    ),
  );
  const result = await Bun.build({
    ...buildOptions,
    entrypoints: [new URL(entrypoint).pathname],
    target: "browser",
  });
  assert.equal(result.success, true, result.logs.map(String).join("\n"));
  const script = await result.outputs[0].text();
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch: async (request) => {
      const url = new URL(request.url),
        headers = { "cache-control": "no-store" };
      if (url.pathname === "/fixture")
        return Response.json(fixture, { headers });
      if (url.pathname === "/probe.js")
        return new Response(script, {
          headers: { ...headers, "content-type": "text/javascript" },
        });
      if (url.pathname.endsWith(".wasm"))
        return new Response(
          Bun.file(
            path.join(
              path.dirname(sdk.resolve("@matrix-org/matrix-sdk-crypto-wasm")),
              "pkg/matrix_sdk_crypto_wasm_bg.wasm",
            ),
          ),
          { headers: { ...headers, "content-type": "application/wasm" } },
        );
      if (/^\/blob\/[A-Z2-7]{26,64}$/.test(url.pathname))
        return new Response(
          Bun.file(path.join(directory, url.pathname.slice("/blob/".length))),
          {
            headers: { ...headers, "content-type": "application/octet-stream" },
          },
        );
      const custom = await route?.(request);
      if (custom) return custom;
      if (url.pathname !== "/") return new Response(null, { status: 404 });
      return new Response(
        '<!doctype html><body><script type="module" src="/probe.js"></script>',
        {
          headers: {
            ...headers,
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
    const context = await browser.newContext({ acceptDownloads: true });
    await context.route("**/*", (route) =>
      new URL(route.request().url()).origin ===
      `http://127.0.0.1:${server.port}`
        ? route.continue()
        : route.abort(),
    );
    const page = await context.newPage();
    const errors = [];
    const downloads = [];
    if (onDownload)
      page.on("download", (download) => {
        downloads.push(
          Promise.resolve()
            .then(() => onDownload(download))
            .catch((error) => {
              errors.push(String(error));
            }),
        );
      });
    page.on("pageerror", (error) => errors.push(String(error)));
    const passes = [];
    for (let pass = 0; pass <= reloads; pass++) {
      if (pass === 0) await page.goto(`http://127.0.0.1:${server.port}`);
      else await page.reload();
      try {
        await page.waitForFunction(
          (name) =>
            window[name]?.ok || window[name]?.stage.startsWith("failed:"),
          probe,
          { timeout: 120_000 },
        );
      } catch (error) {
        assert.fail(
          JSON.stringify({
            probe,
            errors,
            state: await page.evaluate((name) => window[name], probe),
            body: (await page.locator("body").innerText()).slice(-2000),
            error: String(error),
          }),
        );
      }
      const state = await page.evaluate((name) => window[name], probe);
      assert.equal(
        state.ok,
        true,
        JSON.stringify({
          state,
          errors,
          body: (await page.locator("body").innerText()).slice(-3000),
        }),
      );
      assert.equal(state.stage, "complete");
      passes.push(state);
    }
    await Promise.all(downloads);
    assert.deepEqual(errors, []);
    return {
      browser: browser.version(),
      state: passes.at(-1),
      ...(reloads ? { passes } : {}),
    };
  } finally {
    await browser?.close();
    await server.stop(true);
  }
}
