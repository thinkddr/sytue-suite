// SPDX-License-Identifier: Apache-2.0
/** Real Chromium/IndexedDB/Go Wasm boundary proof. Uses synthetic data only. */
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { chromium, devices } from "playwright";
import {
  bundleMatrixFixture,
  matrixFixtureAsset,
  matrixBrowserOptions,
} from "./matrix-browser-fixture.mjs";
const directory = await mkdtemp(path.join(tmpdir(), "sytue-core-host-port-"));
let browser, server;
try {
  await bundleMatrixFixture(
    fileURLToPath(new URL("./core-host-port-client.ts", import.meta.url)),
    directory,
  );
  const unexpected = [];
  server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      const url = new URL(request.url);
      if (request.method !== "GET") {
        unexpected.push(url.pathname);
        return new Response(null, { status: 403 });
      }
      if (url.pathname === "/source-original")
        return new Response(new Uint8Array((2 << 20) + 17).fill(97), {
          headers: {
            "content-type": "application/octet-stream",
            "content-length": String((2 << 20) + 17),
          },
        });
      if (url.pathname === "/")
        return new Response(
          '<!doctype html><script>window.__sytueBase="/mail"</script><script type="module" src="/core-host-port-client.js"></script>',
          { headers: { "content-type": "text/html" } },
        );
      if (url.pathname === "/favicon.ico")
        return new Response(null, { status: 204 });
      return matrixFixtureAsset(url, directory);
    },
  });
  browser = await chromium.launch(matrixBrowserOptions());
  for (const profile of ["desktop", "mobile"]) {
    const context = await browser.newContext(
      profile === "mobile" ? devices["Pixel 7"] : {},
    );
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.name));
    await page.route("**/*", async (route) => {
      const url = new URL(route.request().url());
      if (url.origin === "http://127.0.0.1:" + server.port)
        await route.continue();
      else {
        unexpected.push(url.origin);
        await route.abort();
      }
    });
    for (const reload of [false, true]) {
      await page.goto(
        "http://127.0.0.1:" + server.port + (reload ? "/?resume=1" : "/"),
      );
      await page.waitForFunction(
        () =>
          window.coreHostProbe?.ok ||
          window.coreHostProbe?.stage.startsWith("failed:"),
        {},
        { timeout: 60_000 },
      );
      const state = await page.evaluate(() => window.coreHostProbe);
      const sourceProof = await page.evaluate(() => window.coreSourceProof);
      const journalProof = await page.evaluate(
        () => window.journalPaginationProof,
      );
      console.log(
        JSON.stringify({
          profile,
          reload,
          state,
          sourceProof,
          journalProof,
          sourceError: await page.evaluate(() => window.coreSourceReadError),
          errors,
        }),
      );
      assert.equal(state.ok, true);
      assert(sourceProof);
      assert.deepEqual(journalProof, {
        records: 10017,
        pages: 79,
        keyOnly: true,
        lookahead: 1,
        exclusiveCursor: true,
      });
      assert.equal(sourceProof.network_reads, 2);
      assert.equal(sourceProof.size_bytes, (2 << 20) + 17);
      const persisted = await page.evaluate(async () => {
        const opened = await new Promise((resolve, reject) => {
          const request = indexedDB.open("sytue-offline");
          request.onsuccess = () => resolve(request.result);
          request.onerror = () => reject(Error("real IDB unavailable"));
        });
        let plaintext = false,
          keys = 0;
        const scan = (value) => {
          if (typeof value === "string") {
            plaintext ||= value.includes("PRIVATE-HOST-PORT-CLIENT-ONLY");
            return;
          }
          if (value instanceof CryptoKey) {
            if (value.extractable || value.algorithm.name !== "AES-GCM")
              throw Error("unsafe persisted key");
            keys++;
            return;
          }
          if (ArrayBuffer.isView(value)) {
            scan(new TextDecoder().decode(value));
            return;
          }
          if (value instanceof ArrayBuffer) {
            scan(new Uint8Array(value));
            return;
          }
          if (value && typeof value === "object")
            for (const field of Object.values(value)) scan(field);
        };
        try {
          for (const store of opened.objectStoreNames) {
            const records = await new Promise((resolve, reject) => {
              const request = opened
                .transaction(store, "readonly")
                .objectStore(store)
                .getAll();
              request.onsuccess = () => resolve(request.result);
              request.onerror = () => reject(Error("real IDB read failed"));
            });
            scan(records);
          }
        } finally {
          opened.close();
        }
        return { plaintext, keys };
      });
      assert.equal(persisted.plaintext, false);
      assert(persisted.keys > 0);
      assert.equal(state.ok, true);
      assert.equal(state.stage, reload ? "complete-reloaded" : "complete");
      assert.deepEqual(errors, []);
    }
    await context.close();
  }
  assert.deepEqual(unexpected, []);
  console.log(
    "CORE_HOST_PORT_REAL_CHROMIUM_IDB_LEASE_SCOPE_REVOKE_DRAIN_PQ_REGRESSION_GREEN",
  );
} finally {
  await browser?.close();
  server?.stop(true);
  await rm(directory, { recursive: true, force: true });
}
