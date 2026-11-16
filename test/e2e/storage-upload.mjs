/* SPDX-License-Identifier: Apache-2.0 */
/** Actual browser upload recovery through three lost ACKs and two Matrix destinations. Synthetic data only. */
import assert from "node:assert/strict";
import path from "node:path";
import { createRequire } from "node:module";
import { readdir, writeFile } from "node:fs/promises";
import { chromium } from "playwright";
import { cabecerasCerradas } from "../../apps/frontend/packages/config/src/astro-middleware.js";
import {
  decodeStorageRecord,
  sameStorageRecord,
} from "../../apps/frontend/packages/offline/src/matrix/storage.js";
const output = process.argv[2];
if (output)
  assert(
    path.isAbsolute(output) && !(await readdir(output)).length,
    "empty disposable vector directory required",
  );
const sdk = createRequire(
  new URL("../../apps/frontend/packages/offline/package.json", import.meta.url),
);
const result = await Bun.build({
  entrypoints: [
    new URL("./storage-upload-client.ts", import.meta.url).pathname,
  ],
  target: "browser",
});
assert.equal(result.success, true, result.logs.map(String).join("\n"));
const script = await result.outputs[0].text();
const blobs = new Map(),
  records = new Map(),
  transactions = new Map();
const lost = new Set();
let writes = 0,
  sends = 0,
  receipt;
const server = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch: async (request) => {
    const url = new URL(request.url),
      headers = { "cache-control": "no-store" };
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
    if (/^\/blob\/[A-Z2-7]{26,64}$/.test(url.pathname)) {
      const id = url.pathname.slice(6);
      if (request.method === "PUT") {
        writes++;
        const bytes = new Uint8Array(await request.arrayBuffer());
        assert(bytes.length <= 8 << 20);
        if (blobs.has(id))
          assert.deepEqual(bytes, blobs.get(id), "retry ciphertext changed");
        blobs.set(id, bytes);
        if (!lost.has("blob")) {
          lost.add("blob");
          return new Response(null, { status: 503 });
        }
        return Response.json(
          {
            id,
            product: "drive",
            size_bytes: bytes.length,
            sha256: new Bun.CryptoHasher("sha256").update(bytes).digest("hex"),
          },
          { headers },
        );
      }
      return new Response(blobs.get(id), {
        headers: { ...headers, "content-type": "application/octet-stream" },
      });
    }
    if (url.pathname === "/record" && request.method === "PUT") {
      sends++;
      const input = await request.json(),
        record = decodeStorageRecord(input.record, {
          tenant: "acme",
          subject: "person",
          product: "drive",
        });
      assert.equal(
        input.target.room_id,
        "!private:matrix.test",
        "publication followed room rotation",
      );
      assert.equal(input.target.device_id, "BROWSER");
      const key = JSON.stringify([
        input.target.device_id,
        input.target.room_id,
        input.transaction,
      ]);
      let reference = transactions.get(key);
      if (reference)
        assert(
          sameStorageRecord(records.get(reference.event_id), record),
          "retry record changed",
        );
      else {
        reference = {
          room_id: input.target.room_id,
          event_id: "$" + (records.size + 1),
        };
        records.set(reference.event_id, record);
        transactions.set(key, reference);
      }
      const payload = JSON.parse(
        Buffer.from(record.payload, "base64").toString("utf8"),
      );
      const kind = payload.kind === "org.sytue.file.v1" ? "root" : "page";
      if (!lost.has(kind)) {
        lost.add(kind);
        return new Response(null, { status: 503 });
      }
      return Response.json(reference, { headers });
    }
    if (url.pathname.startsWith("/record/"))
      return Response.json(
        records.get(decodeURIComponent(url.pathname.slice(8))),
        { headers },
      );
    if (url.pathname === "/result") {
      receipt = await request.json();
      return new Response(null, { status: 204 });
    }
    if (url.pathname !== "/") return new Response(null, { status: 404 });
    return new Response(
      '<!doctype html><script type="module" src="/probe.js"></script>',
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
  const context = await browser.newContext();
  await context.route("**/*", (route) =>
    new URL(route.request().url()).origin === `http://127.0.0.1:${server.port}`
      ? route.continue()
      : route.abort(),
  );
  const page = await context.newPage();
  for (let phase = 0; phase < 5; phase++) {
    await page.goto(`http://127.0.0.1:${server.port}/?phase=${phase}`);
    await page.waitForFunction(
      () =>
        window.storageUploadProbe?.ok ||
        window.storageUploadProbe?.stage.startsWith("failed:"),
      {},
      { timeout: 120_000 },
    );
    const state = await page.evaluate(() => window.storageUploadProbe);
    assert.equal(state.ok, true, JSON.stringify({ phase, state }));
    assert.equal(state.stage, phase < 3 ? "interrupted" : "complete");
    assert.equal(state.completed, phase >= 3);
  }
  assert.equal(writes, 18);
  assert.equal(blobs.size, 17);
  assert.equal(sends, 5);
  assert.equal(records.size, 3);
  assert.equal(transactions.size, 3);
  assert.equal(receipt.artificial, true);
  if (output) {
    for (const [id, bytes] of blobs)
      await writeFile(path.join(output, id), bytes, { mode: 0o600 });
    await writeFile(
      path.join(output, "vector.json"),
      JSON.stringify({
        ...receipt,
        selected: { offset: (8 << 20) - 4, bytes: "AQEBAQICAgI=" },
        entries: [...records].map(([event_id, record]) => ({
          reference: { room_id: "!private:matrix.test", event_id },
          record,
        })),
      }),
      { mode: 0o600 },
    );
  }
  console.log(
    JSON.stringify({
      browser: browser.version(),
      reloads: 4,
      bytes: receipt.request.size_bytes,
      lostACKs: [...lost],
      ciphertextWrites: writes,
      uniqueBlocks: blobs.size,
      pinnedRecords: records.size,
    }),
  );
} finally {
  await browser?.close();
  await server.stop(true);
}
