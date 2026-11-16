/* SPDX-License-Identifier: Apache-2.0 */
import assert from "node:assert/strict";
import { uiBrowserBuildOptions } from "./lib/ui-browser-build.mjs";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { runPrivateStorageBrowser } from "./lib/private-storage-browser.mjs";
import { privateDriveSourceRoute } from "./lib/private-drive-source.mjs";
import { migrationSourceFixture } from "../../apps/frontend/packages/offline/src/matrix/private-drive-migration-fixture.ts";
import { rutaBlobsMatrixPrivados } from "../../apps/frontend/packages/auth/src/server/matrix-storage-blobs.js";
const vectorPath = process.argv[2];
assert(
  vectorPath?.startsWith("/tmp/sytue-private-drive-vector-"),
  "disposable source vector required",
);
const vector = await Bun.file(vectorPath).json();
assert.equal(vector.state.scope.tenant, "artificial-storage");
assert.equal(vector.state.scope.subject, "artificial");
const scope = vector.state.scope,
  source = await migrationSourceFixture(scope, {
    ...vector,
    state: { scope, mode: "legacy", rows: 0, blobs: 0 },
  }),
  sourceRoute = privateDriveSourceRoute(source);
const chunks = new Map(),
  unexpected = [];
let writes = 0,
  downloads = 0,
  plaintext = false;
const gateway = rutaBlobsMatrixPrivados({
  product: "drive",
  env: { SYTUE_DRIVE_INTERNAL_URL: "https://internal.test" },
  bff: {
    getSession: async () => ({
      user: { id: scope.subject, tenant: scope.tenant },
      tokens: { accessToken: "artificial-server-token" },
    }),
  },
  fetchImpl: async (input, init) => {
    const request = new Request(input, init),
      url = new URL(request.url),
      parts = url.pathname.split("/"),
      id = parts.at(-1) === "content" ? parts.at(-2) : parts.at(-1);
    assert.equal(
      request.headers.get("authorization"),
      "Bearer artificial-server-token",
    );
    if (request.method === "PUT") {
      const bytes = new Uint8Array(await request.arrayBuffer());
      plaintext ||= new TextDecoder()
        .decode(bytes)
        .includes("ARTIFICIAL PRIVATE UI");
      const hash = Buffer.from(
        await crypto.subtle.digest("SHA-256", bytes),
      ).toString("hex");
      const previous = chunks.get(id);
      if (previous) assert.deepEqual(previous.bytes, bytes);
      chunks.set(id, { bytes, hash });
      writes++;
    }
    const value = chunks.get(id);
    if (!value) return new Response(null, { status: 404 });
    if (url.pathname.endsWith("/content"))
      return new Response(value.bytes, {
        headers: { "content-type": "application/octet-stream" },
      });
    return Response.json({
      id,
      product: "drive",
      size_bytes: value.bytes.length,
      sha256: value.hash,
    });
  },
});
const directory = await mkdtemp("/tmp/sytue-private-drive-ui-browser-");
try {
  await writeFile(
    directory + "/vector.json",
    JSON.stringify({ artificial: true, scope }),
    { mode: 0o600 },
  );
  const result = await runPrivateStorageBrowser({
    directory,
    entrypoint: new URL("./private-drive-ui-client.tsx", import.meta.url),
    probe: "privateDriveUIProbe",
    onDownload: async (download) => {
      assert.equal(download.suggestedFilename(), "browser-private.txt");
      assert.equal(await download.failure(), null);
      assert.equal(
        await Bun.file(await download.path()).text(),
        "ARTIFICIAL PRIVATE UI CONTENT",
      );
      downloads++;
    },
    buildOptions: {
      ...uiBrowserBuildOptions(),
      env: "disable",
      define: {
        "import.meta.env": JSON.stringify({
          SSR: true,
          BASE_URL: "/",
          DEV: false,
          PROD: false,
        }),
      },
    },
    route: async (request) => {
      const url = new URL(request.url);
      const retained = await sourceRoute(request);
      if (retained) return retained;
      if (url.pathname.startsWith("/api/matrix/storage/blobs/"))
        return gateway({
          request,
          url,
          params: {
            ruta: url.pathname.slice("/api/matrix/storage/blobs/".length),
          },
          cookies: {},
        });
      if (url.pathname === "/ui-stats")
        return Response.json({ writes, downloads, plaintext, unexpected });
      if (url.pathname.startsWith("/api/")) {
        unexpected.push(url.pathname);
        return new Response(null, { status: 500 });
      }
    },
  });
  assert.equal(plaintext, false);
  assert.deepEqual(unexpected, []);
  assert(writes >= 3);
  assert.equal(downloads, 1);
  console.log(JSON.stringify({ ...result, writes, source: source.calls }));
} finally {
  await rm(directory, { recursive: true, force: true });
}
