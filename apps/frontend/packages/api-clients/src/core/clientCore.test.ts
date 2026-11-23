/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { clientCoreProxy, roomsServiceBase } from './clientCore.js';
const servers: { stop: (closeActiveConnections?: boolean) => void }[] = [];
afterEach(() => {
  for (const server of servers.splice(0)) server.stop(true);
});
test('each app proxies only public versioned core artifacts without a session', async () => {
  const seen: string[] = [];
  const digest = 'a'.repeat(64);
  const server = Bun.serve({
    port: 0,
    fetch(request) {
      const url = new URL(request.url);
      seen.push(url.pathname + url.search);
      expect(request.headers.get('authorization')).toBeNull();
      expect(request.headers.get('cookie')).toBeNull();
      const compressed = Bun.gzipSync(new TextEncoder().encode('synthetic-core'));
      return new Response(compressed, {
        headers: {
          'content-encoding': 'gzip',
          'content-length': String(compressed.length),
          'content-type': 'application/wasm',
          etag: '"' + digest + '"',
        },
      });
    },
  });
  servers.push(server);
  const proxy = clientCoreProxy(server.url.href);
  const request = new Request(
    'https://fixture.test/mail/api/client-core/core.wasm?sha256=' + digest,
    { headers: { cookie: 'not-forwarded', authorization: 'Bearer not-forwarded' } },
  );
  const response = await proxy({
    params: { asset: 'core.wasm' },
    url: new URL(request.url),
    request,
  });
  expect(response.status).toBe(200);
  expect(response.headers.get('content-length')).toBeNull();
  expect(response.headers.get('content-encoding')).toBeNull();
  expect(await response.text()).toBe('synthetic-core');
  expect(response.headers.get('cache-control')).toContain('immutable');
  expect(response.headers.get('etag')).toBe('"' + digest + '"');
  expect(seen).toEqual(['/api/v1/client-core/core.wasm?sha256=' + digest]);
  for (const [asset, query] of [
    ['not-a-compiler-file', ''],
    ['core.wasm', '?q=private'],
    ['wasm_exec.js', '?sha256=invalid'],
    ['core.wasm', '?module=private-document-url'],
    ['core.wasm', '?module=office&module=core'],
    ['core.wasm', '?module='],
  ]) {
    const url = new URL('https://fixture.test/rooms/api/client-core/' + asset + query);
    expect((await proxy({ params: { asset }, url, request: new Request(url) })).status).toBe(400);
  }
  expect(seen.length).toBe(1);
  const officeURL = new URL(
    'https://fixture.test/docus/api/client-core/core.wasm?module=office&sha256=' + digest,
  );
  const office = await proxy({
    params: { asset: 'core.wasm' },
    url: officeURL,
    request: new Request(officeURL),
  });
  expect(office.status).toBe(200);
  expect(seen[1]).toBe('/api/v1/client-core/core.wasm?sha256=' + digest + '&module=office');
  // El motor de PDF del estudio de la bóveda (ADR 0086): CertiPDF lo pide como un módulo más.
  const pdfURL = new URL(
    'https://fixture.test/certipdf/api/client-core/core.wasm?module=pdf&sha256=' + digest,
  );
  const pdf = await proxy({
    params: { asset: 'core.wasm' },
    url: pdfURL,
    request: new Request(pdfURL),
  });
  expect(pdf.status).toBe(200);
  expect(seen[2]).toBe('/api/v1/client-core/core.wasm?sha256=' + digest + '&module=pdf');
});
test('a changed manifest is rejected instead of serving mixed runtime versions', async () => {
  const server = Bun.serve({
    port: 0,
    fetch() {
      return new Response(null, { status: 412 });
    },
  });
  servers.push(server);
  const url = new URL(
    'https://fixture.test/drive/api/client-core/wasm_exec.js?sha256=' + '0'.repeat(64),
  );
  const response = await clientCoreProxy(server.url.href)({
    params: { asset: 'wasm_exec.js' },
    url,
    request: new Request(url),
  });
  expect(response.status).toBe(412);
  expect(response.headers.get('cache-control')).toBe('no-store');
});
test('the shared Rooms service resolver preserves deployed URL precedence', () => {
  expect(
    roomsServiceBase({
      SYTUE_ROOMS_INTERNAL_URL: 'http://rooms:8085',
      SYTUE_ROOMS_API_URL: 'https://public.fixture',
      SYTUE_CHAT_API_URL: 'https://old.fixture',
    }),
  ).toBe('http://rooms:8085');
  expect(roomsServiceBase({ SYTUE_CHAT_API_URL: 'https://old.fixture' })).toBe(
    'https://old.fixture',
  );
});
