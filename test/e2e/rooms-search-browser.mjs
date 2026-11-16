// SPDX-License-Identifier: Apache-2.0
import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import {
  bundleMatrixFixture,
  matrixFixtureAsset,
  matrixBrowserOptions,
  requireArtificialMatrixFixture,
} from "./matrix-browser-fixture.mjs";
const [fixturePath, resultPath] = process.argv.slice(2),
  fixture = JSON.parse(await readFile(fixturePath, "utf8"));
requireArtificialMatrixFixture(fixture);
assert.equal(fixture.bootstrap.homeserver_url, "https://127.0.0.1:18775");
assert(new URL(fixture.metadata_url).hostname === "127.0.0.1");
const directory = await mkdtemp(path.join(tmpdir(), "rooms-search-browser-"));
let browser, server;
const traffic = [];
const receipts = {};
function project(row) {
  return {
    id: row.id,
    tenant: row.tenant,
    channel_slug: row.channel_slug,
    channel_name: row.channel_name,
    saved: row.saved,
    pinned: row.pinned,
    message: {
      id: row.id,
      channelId: row.channel_id,
      author: { id: row.author_sub, name: row.author_name },
      body: "",
      createdAt: row.created_at,
      ...(row.edited_at ? { editedAt: row.edited_at } : {}),
      ...(row.parent_id ? { parentId: row.parent_id } : {}),
      matrix_event: row.matrix_event,
      encryption: "m.megolm.v1.aes-sha2",
    },
  };
}
try {
  await bundleMatrixFixture(
    fileURLToPath(new URL("./rooms-search-browser-client.ts", import.meta.url)),
    directory,
  );
  server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      const url = new URL(request.url);
      if (url.pathname === "/")
        return new Response(
          '<!doctype html><script>window.__sytueBase="/rooms"</script><script type="module" src="/rooms-search-browser-client.js"></script>',
          { headers: { "content-type": "text/html" } },
        );
      if (url.pathname === "/rooms/api/search-metadata") {
        const body =
          request.method === "POST" ? await request.text() : undefined;
        traffic.push({ url: url.pathname + url.search, body });
        for (const key of url.searchParams.keys()) assert.equal(key, "cursor");
        const upstream = new URL(
          "/api/v1/messages/metadata" + (body === undefined ? "" : "/visible"),
          fixture.metadata_url,
        );
        if (body === undefined)
          upstream.searchParams.set(
            "cursor",
            url.searchParams.get("cursor") ?? "",
          );
        const response = await fetch(upstream, {
          method: request.method,
          ...(body === undefined
            ? {}
            : { body, headers: { "content-type": "application/json" } }),
        });
        assert.equal(response.status, 200);
        const page = await response.json();
        return Response.json(
          { ...page, messages: page.messages.map(project) },
          { headers: { "cache-control": "no-store" } },
        );
      }
      if (url.pathname === "/favicon.ico")
        return new Response(null, { status: 204 });
      return matrixFixtureAsset(url, directory);
    },
  });
  browser = await chromium.launch(matrixBrowserOptions());
  for (const name of ["desktop", "mobile"]) {
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      isMobile: name === "mobile",
      hasTouch: name === "mobile",
      viewport:
        name === "mobile"
          ? { width: 390, height: 844 }
          : { width: 1280, height: 800 },
    });
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.name));
    await page.route("**/*", async (route) => {
      const origin = new URL(route.request().url()).origin;
      if (
        origin === server.url.origin ||
        origin === fixture.bootstrap.homeserver_url
      )
        return route.continue();
      return route.abort();
    });
    await page.goto(server.url.href);
    await page.waitForFunction(
      () => typeof window.runRoomsSearch === "function",
    );
    const started = performance.now();
    receipts[name] = await page.evaluate(
      ({ fixture, name }) => window.runRoomsSearch(fixture, name),
      { fixture, name },
    );
    const state = await page.evaluate(() => window.roomsSearchProbe);
    assert.equal(state.ok, true);
    assert.deepEqual(errors, []);
    console.log(
      JSON.stringify({
        kind: "rooms-real-rust",
        browser: browser.version(),
        mode: name,
        stage: state.stage,
        seconds: Number(((performance.now() - started) / 1000).toFixed(3)),
      }),
    );
    await context.close();
  }
  const trace = JSON.stringify(traffic);
  for (const text of ["REUNION", "viernes", "has:file", "private", "borrador"])
    assert(
      !trace.includes(text),
      "query or decrypted content reached metadata HTTP",
    );
  await writeFile(resultPath, JSON.stringify(receipts), { mode: 0o600 });
  console.log("ROOMS_REAL_RUST_BROWSER_NATIVE_GREEN");
} finally {
  await browser?.close();
  await server?.stop(true);
  await rm(directory, { recursive: true, force: true });
}
