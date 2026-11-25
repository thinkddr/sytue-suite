/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { brotliCompressSync, brotliDecompressSync } from 'node:zlib';
import { clientCoreHead, clientCoreProxy, roomsServiceBase } from './clientCore.js';
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
      // Rooms negocia: sin brotli ni gzip aceptados, los bytes sin comprimir.
      expect(request.headers.get('accept-encoding')).toBe('identity');
      return new Response(new TextEncoder().encode('synthetic-core'), {
        headers: {
          'content-type': 'application/wasm',
          etag: '"' + digest + '"',
          vary: 'Accept-Encoding',
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
  expect(response.headers.get('vary')).toBe('Accept-Encoding');
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
// Lo que Rooms ya comprimió en brotli 11 llega al navegador TAL CUAL: ni el proxy lo
// descomprime ni Traefik lo vuelve a comprimir. La huella es la de los bytes
// originales, que el worker calcula sobre lo que el navegador descomprime.
test('precompressed compiler artifacts pass through encoded and keep the raw digest', async () => {
  const raw = new TextEncoder().encode('synthetic-core '.repeat(512));
  const digest = new Bun.CryptoHasher('sha256').update(raw).digest('hex');
  const brotli = new Uint8Array(brotliCompressSync(raw));
  const gzip = Bun.gzipSync(raw);
  const asked: (string | null)[] = [];
  const server = Bun.serve({
    port: 0,
    fetch(request) {
      const accept = request.headers.get('accept-encoding');
      asked.push(accept);
      const url = new URL(request.url);
      if (url.pathname.endsWith('/manifest.json')) return Response.json({ version: 1 });
      const encoding = accept?.includes('br') ? 'br' : accept?.includes('gzip') ? 'gzip' : '';
      const body = encoding === 'br' ? brotli : encoding === 'gzip' ? gzip : raw;
      return new Response(body, {
        headers: {
          'content-type': 'application/wasm',
          'content-length': String(body.byteLength),
          etag: '"' + digest + (encoding ? '.' + encoding : '') + '"',
          vary: 'Accept-Encoding',
          ...(encoding ? { 'content-encoding': encoding } : {}),
        },
      });
    },
  });
  servers.push(server);
  const proxy = clientCoreProxy(server.url.href);
  const fetchAsset = async (asset: string, accept: string | null) => {
    const url = new URL(
      'https://fixture.test/drive/api/client-core/' + asset + '?module=vault&sha256=' + digest,
    );
    const request = new Request(url, accept ? { headers: { 'accept-encoding': accept } } : {});
    return proxy({ params: { asset }, url, request });
  };
  for (const [accept, encoding, bytes] of [
    ['br, gzip, deflate, zstd', 'br', brotli],
    ['gzip', 'gzip', gzip],
    [null, null, raw],
  ] as const) {
    const response = await fetchAsset('core.wasm', accept);
    expect(response.status).toBe(200);
    expect(response.headers.get('content-encoding')).toBe(encoding);
    expect(response.headers.get('vary')).toBe('Accept-Encoding');
    expect(response.headers.get('cache-control')).toContain('immutable');
    const body = new Uint8Array(await response.arrayBuffer());
    expect(body).toEqual(new Uint8Array(bytes));
    const decoded =
      encoding === 'br'
        ? new Uint8Array(brotliDecompressSync(body))
        : encoding === 'gzip'
          ? Bun.gunzipSync(body)
          : body;
    expect(new Bun.CryptoHasher('sha256').update(decoded).digest('hex')).toBe(digest);
  }
  expect(asked).toEqual(['br, gzip, deflate, zstd', 'gzip', 'identity']);
  // El manifiesto nunca va comprimido por aquí: es pequeño y sin caché.
  const manifestURL = new URL('https://fixture.test/drive/api/client-core/manifest.json');
  await proxy({
    params: { asset: 'manifest.json' },
    url: manifestURL,
    request: new Request(manifestURL, { headers: { 'accept-encoding': 'br' } }),
  });
  expect(asked.at(-1)).toBe('identity');
});
test('an upstream encoding the browser cannot be handed is refused, not relabelled', async () => {
  const server = Bun.serve({
    port: 0,
    fetch() {
      return new Response(new Uint8Array([1, 2, 3]), { headers: { 'content-encoding': 'zstd' } });
    },
  });
  servers.push(server);
  const url = new URL('https://fixture.test/drive/api/client-core/core.wasm');
  const response = await clientCoreProxy(server.url.href)({
    params: { asset: 'core.wasm' },
    url,
    request: new Request(url, { headers: { 'accept-encoding': 'zstd' } }),
  });
  expect(response.status).toBe(502);
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
test('the vault page head pins the manifest digests and preloads the exact worker URLs', async () => {
  const wasm = 'a'.repeat(64);
  const runtime = 'b'.repeat(64);
  let manifests = 0;
  const server = Bun.serve({
    port: 0,
    fetch(request) {
      const url = new URL(request.url);
      expect(url.pathname).toBe('/api/v1/client-core/manifest.json');
      // El manifiesto del núcleo de la bóveda, que es el que arranca la puerta.
      expect(url.search).toBe('?module=vault');
      expect(request.headers.get('cookie')).toBeNull();
      manifests++;
      return Response.json({
        version: 1,
        module_id: 'vault',
        wasm_sha256: wasm,
        runtime_sha256: runtime,
      });
    },
  });
  servers.push(server);
  const head = await clientCoreHead(server.url.href, '/drive/');
  expect(head).toBe(
    `<meta name="sytue-client-core" content="${wasm} ${runtime}">` +
      `<link rel="modulepreload" href="/drive/api/client-core/wasm_exec.js?module=vault&amp;sha256=${runtime}">` +
      `<link rel="preload" as="fetch" crossorigin="anonymous" type="application/wasm" href="/drive/api/client-core/core.wasm?module=vault&amp;sha256=${wasm}">`,
  );
  // Cached: a second page render does not ask Rooms again.
  expect(await clientCoreHead(server.url.href, '/drive')).toBe(head);
  expect(manifests).toBe(1);
});
test('without a valid manifest in time the head stays empty and the worker asks itself', async () => {
  const bad = Bun.serve({
    port: 0,
    fetch: () => Response.json({ version: 1, wasm_sha256: 'nope', runtime_sha256: 'b'.repeat(64) }),
  });
  const slow = Bun.serve({
    port: 0,
    fetch: () =>
      new Promise<Response>((resolve) => setTimeout(() => resolve(Response.json({})), 500)),
  });
  servers.push(bad, slow);
  expect(await clientCoreHead(bad.url.href, '/mail')).toBe('');
  expect(await clientCoreHead(slow.url.href, '/mail', 50)).toBe('');
});
