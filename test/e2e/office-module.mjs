// SPDX-License-Identifier: Apache-2.0
/** Actual lazy Go processor, shared Worker and host-lease proof. Artificial data only. */
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, devices } from 'playwright';
import {
  bundleMatrixFixture,
  matrixFixtureAsset,
  matrixBrowserOptions,
} from './matrix-browser-fixture.mjs';
const directory = await mkdtemp(path.join(tmpdir(), 'sytue-office-module-'));
let browser, server;
let wrongModule = false;
const requests = [],
  unexpected = [];
try {
  await bundleMatrixFixture(
    fileURLToPath(new URL('./office-module-client.ts', import.meta.url)),
    directory,
  );
  server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    async fetch(request) {
      const url = new URL(request.url);
      if (request.method !== 'GET') {
        unexpected.push(url.pathname);
        return new Response(null, { status: 403 });
      }
      if (url.pathname === '/')
        return new Response(
          '<!doctype html><script type="module" src="/office-module-client.js"></script>',
          { headers: { 'content-type': 'text/html' } },
        );
      if (url.pathname === '/favicon.ico') return new Response(null, { status: 204 });
      requests.push(url.pathname + url.search);
      const response = await matrixFixtureAsset(url, directory);
      if (
        wrongModule &&
        url.pathname.endsWith('/api/client-core/manifest.json') &&
        url.searchParams.get('module') === 'office'
      ) {
        const manifest = await response.json();
        return Response.json({ ...manifest, module_id: 'core' });
      }
      return response;
    },
  });
  browser = await chromium.launch(matrixBrowserOptions());
  for (const profile of ['desktop', 'mobile']) {
    const context = await browser.newContext(profile === 'mobile' ? devices['Pixel 7'] : {});
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.name));
    await page.route('**/*', async (route) => {
      const url = new URL(route.request().url());
      if (url.origin === 'http://127.0.0.1:' + server.port) await route.continue();
      else {
        unexpected.push(url.origin);
        await route.abort();
      }
    });
    for (const tamper of [false, true]) {
      wrongModule = tamper;
      const before = requests.length;
      await page.goto('http://127.0.0.1:' + server.port + (tamper ? '/?bad-module=1' : '/'));
      await page.waitForFunction(
        () => window.officeModuleProbe?.ok || window.officeModuleProbe?.stage.startsWith('failed:'),
        {},
        { timeout: 60000 },
      );
      const state = await page.evaluate(() => window.officeModuleProbe);
      console.log(JSON.stringify({ profile, tamper, state, errors }));
      assert.equal(state.ok, true);
      assert.equal(state.stage, tamper ? 'wrong-module-rejected' : 'complete');
      assert.deepEqual(errors, []);
      const office = requests.slice(before).filter((x) => x.includes('module=office'));
      assert(office.some((x) => x.includes('manifest.json')));
      assert.equal(
        office.some((x) => x.includes('core.wasm')),
        !tamper,
      );
    }
    await context.close();
  }
  assert.deepEqual(unexpected, []);
  console.log('OFFICE_LAZY_REAL_GO_CHROMIUM_DESKTOP_MOBILE_DOCX_HOST_REVOKE_WRONG_MODULE_GREEN');
} finally {
  await browser?.close();
  server?.stop(true);
  await rm(directory, { recursive: true, force: true });
}
