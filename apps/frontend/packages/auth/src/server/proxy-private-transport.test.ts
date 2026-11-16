/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { Bff } from './bff.js';
import { crearProxyDeServicio } from './proxy.js';
function context(body: string): APIContext {
  const request = new Request('https://mail.test/api/private-storage/submissions', {
    method: 'POST',
    body,
    headers: { 'content-type': 'message/rfc822' },
  });
  return {
    request,
    url: new URL(request.url),
    params: { ruta: 'submissions' },
    cookies: { get() {}, set() {}, delete() {} },
  } as unknown as APIContext;
}
const bff = {
  getSession: async () => ({ tokens: { accessToken: 'transport-fixture-only' } }),
} as unknown as Bff;
test('fixed private proxy rejects a real MIME redirect while the legacy opt-out remains explicit', async () => {
  let received = 0;
  const outside = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    async fetch(request) {
      received++;
      await request.body?.cancel();
      return new Response('outside');
    },
  });
  const inside = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    fetch() {
      return new Response(null, { status: 307, headers: { location: outside.url + 'leak' } });
    },
  });
  try {
    const protectedRoute = crearProxyDeServicio({
      bff,
      destino: inside.url.href,
      prefijo: '/',
      rechazarRedirecciones: true,
    });
    const response = await protectedRoute(context('synthetic transient MIME'));
    expect(response.status).toBe(503);
    expect(received).toBe(0);
    const legacy = crearProxyDeServicio({ bff, destino: inside.url.href, prefijo: '/' });
    const control = context('synthetic legacy control');
    control.request = new Request(control.url);
    expect((await legacy(control)).status).toBe(200);
    expect(received).toBe(1);
  } finally {
    await inside.stop(true);
    await outside.stop(true);
  }
});
test('proxy preserves only the public source commitment and forwards the original stream without buffering', async () => {
  const ctx = context('synthetic exact MIME');
  let same = false;
  const route = crearProxyDeServicio({
    bff,
    destino: 'https://mail.internal',
    rechazarRedirecciones: true,
    fetchImpl: (async (_input, init) => {
      same = init?.body === ctx.request.body;
      return new Response('exact', {
        headers: { 'X-Sytue-Source-SHA256': 'a'.repeat(64), 'Set-Cookie': 'must-not-forward=1' },
      });
    }) as typeof fetch,
  });
  const response = await route(ctx);
  expect(same).toBe(true);
  expect(response.headers.get('x-sytue-source-sha256')).toBe('a'.repeat(64));
  expect(response.headers.has('set-cookie')).toBe(false);
  expect(await response.text()).toBe('exact');
});
