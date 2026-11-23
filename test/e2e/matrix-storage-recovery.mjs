// SPDX-License-Identifier: Apache-2.0
/** Opt-in companion to the native integration test; accepts artificial fixtures only. */
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, devices } from 'playwright';
import {
  bundleMatrixFixture,
  matrixFixtureAsset,
  matrixBrowserOptions,
} from './matrix-browser-fixture.mjs';

const [fixturePath, resultPath] = process.argv.slice(2);
const fixture = JSON.parse(await readFile(fixturePath, 'utf8'));
assert.equal(fixture.artificial, true);
assert.equal(fixture.bootstrap.homeserver_url, 'https://storage.test:18586');
// La inyección cruda es el único punto en el que datos leídos del fichero salen por
// HTTP. Destino fijo (el Synapse artificial local) y solo la forma exacta que produce
// el fixture nativo: sala y token acotados, contenido Megolm sin campos libres
// enormes. Todo se valida y se prepara aquí, antes de levantar nada.
const artificialSynapse = 'http://127.0.0.1:18585';
const artificialInjections = (() => {
  const room = fixture.reference?.room_id;
  const token = fixture.session?.access_token;
  assert(
    typeof room === 'string' &&
      /^![A-Za-z0-9._~-]{1,255}(?::[A-Za-z0-9.-]{1,255}(?::\d{1,5})?)?$/.test(room),
    'artificial room id expected',
  );
  assert(
    typeof token === 'string' && /^[A-Za-z0-9._~+/=-]{16,512}$/.test(token),
    'artificial access token expected',
  );
  const megolm = (content) => {
    assert(content && typeof content === 'object' && !Array.isArray(content));
    assert.equal(content.algorithm, 'm.megolm.v1.aes-sha2');
    assert(typeof content.ciphertext === 'string' && /^[A-Za-z0-9+/]+$/.test(content.ciphertext));
    assert(typeof content.sender_key === 'string' && typeof content.session_id === 'string');
    const body = JSON.stringify(content);
    assert(body.length <= 64 << 10, 'artificial ciphertext too large');
    return body;
  };
  const injection = (which, content) => {
    const url = new URL(
      '/_matrix/client/v3/rooms/' +
        encodeURIComponent(room) +
        '/send/m.room.encrypted/artificial-raw-' +
        which,
      artificialSynapse,
    );
    assert.equal(url.origin, artificialSynapse);
    return { url, authorization: 'Bearer ' + token, body: megolm(content) };
  };
  return {
    forged: injection('forged', fixture.forged_ciphertext),
    legacy: injection('legacy', fixture.legacy_ciphertext),
  };
})();
const directory = await mkdtemp(path.join(tmpdir(), 'sytue-browser-recovery-'));
let browser, server;
try {
  await bundleMatrixFixture(
    fileURLToPath(new URL('./matrix-storage-recovery-client.ts', import.meta.url)),
    directory,
  );
  let receipt;
  const blobs = new Map();
  server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    fetch: async (request) => {
      const url = new URL(request.url);
      if (/^\/blob\/[A-Z2-7]{26,64}$/.test(url.pathname)) {
        const id = url.pathname.slice(6);
        if (request.method === 'PUT') {
          const bytes = new Uint8Array(await request.arrayBuffer());
          assert(bytes.length <= 8 << 20);
          if (blobs.has(id)) assert.deepEqual(bytes, blobs.get(id), 'retry ciphertext changed');
          blobs.set(id, bytes);
          return Response.json({
            id,
            product: fixture.bootstrap.storage_product,
            size_bytes: bytes.length,
            sha256: new Bun.CryptoHasher('sha256').update(bytes).digest('hex'),
          });
        }
        return blobs.has(id)
          ? new Response(blobs.get(id), {
              headers: { 'content-type': 'application/octet-stream' },
            })
          : new Response(null, { status: 404 });
      }
      // Reader adversaries bypass the maintained client intentionally. Its new
      // HTTP guard must reject raw pre-encrypted cache without local witnesses;
      // this artificial-only host injects fixed fixture bytes into fixed Synapse.
      if (
        request.method === 'POST' &&
        ['/inject/forged', '/inject/legacy'].includes(url.pathname)
      ) {
        const injection =
          url.pathname === '/inject/forged'
            ? artificialInjections.forged
            : artificialInjections.legacy;
        const response = await fetch(injection.url, {
          method: 'PUT',
          redirect: 'error',
          headers: {
            authorization: injection.authorization,
            'content-type': 'application/json',
          },
          body: injection.body,
        });
        assert.equal(response.status, 200, 'fixed artificial injection failed');
        return Response.json(await response.json());
      }
      if (url.pathname === '/uploaded') return Response.json(receipt.file_reference);
      if (url.pathname === '/fixture')
        return Response.json(fixture, {
          headers: { 'cache-control': 'no-store' },
        });
      if (url.pathname === '/result' && request.method === 'POST') {
        receipt = await request.json();
        return new Response(null, { status: 204 });
      }
      if (url.pathname === '/')
        return new Response(
          '<!doctype html><script type="module" src="/matrix-storage-recovery-client.js"></script>',
          { headers: { 'content-type': 'text/html' } },
        );
      return matrixFixtureAsset(url, directory);
    },
  });
  browser = await chromium.launch(matrixBrowserOptions());
  const profile = process.env.SYTUE_MATRIX_BROWSER_PROFILE === 'mobile' ? 'mobile' : 'desktop';
  const page = await browser.newPage({
    ...(profile === 'mobile' ? devices['Pixel 7'] : {}),
    ignoreHTTPSErrors: true,
  });
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (
      url.origin === `http://127.0.0.1:${server.port}` ||
      url.origin === 'https://storage.test:18586'
    )
      return route.continue();
    await route.abort();
  });
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.name));
  await page.goto(`http://127.0.0.1:${server.port}`);
  await page.waitForFunction(
    () => window.storageProbe?.ok || window.storageProbe?.stage.startsWith('failed:'),
    {},
    { timeout: 120_000 },
  );
  const state = await page.evaluate(() => window.storageProbe);
  console.log(JSON.stringify({ browser: browser.version(), profile, state, pageErrors: errors }));
  assert.equal(state.ok, true);
  assert(receipt);
  await page.goto(`http://127.0.0.1:${server.port}/?resume=1`);
  await page.waitForFunction(
    () => window.storageProbe?.ok || window.storageProbe?.stage.startsWith('failed:'),
    {},
    { timeout: 120_000 },
  );
  const reloaded = await page.evaluate(() => window.storageProbe);
  console.log(
    JSON.stringify({
      browser: browser.version(),
      reloaded,
      pageErrors: errors,
    }),
  );
  assert.equal(reloaded.ok, true);
  assert.equal(reloaded.stage, 'complete-reloaded');
  await writeFile(
    resultPath,
    JSON.stringify({
      ...receipt,
      file_blobs: Object.fromEntries(
        [...blobs].map(([id, bytes]) => [id, Buffer.from(bytes).toString('base64')]),
      ),
    }),
    { mode: 0o600 },
  );
} finally {
  await browser?.close();
  await server?.stop(true);
  await rm(directory, { recursive: true, force: true });
}
