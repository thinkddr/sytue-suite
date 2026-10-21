/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * FE-02: /abrir DA DE ALTA la foto en la galería, y es un GET al que se llega con un
 * enlace. La cookie de sesión es SameSite=Lax y viaja en esa navegación aunque el enlace
 * esté en otra web: desde otro sitio no se hace nada.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

// `import.meta.env` es `process.env` en bun; Astro pondría el prefijo de la app.
process.env['BASE_URL'] = '/photos/';

mock.module('./bff.js', () => ({
  bff: {
    getSession: () => Promise.resolve({ tokens: { accessToken: 'tok' }, user: { tenant: 'acme' } }),
    basePath: '/photos',
  },
  toBffContext: (ctx: APIContext) => ctx,
}));

const { GET } = await import('../pages/abrir.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

async function abrir(sitio: string): Promise<{ status: number; llamadas: number }> {
  let llamadas = 0;
  globalThis.fetch = (() => {
    llamadas++;
    return Promise.resolve(new Response('{"id":"f1"}', { status: 200 }));
  }) as unknown as typeof fetch;
  const url = new URL(
    'https://acme.sytue.eu/photos/abrir?file=3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f',
  );
  const request = new Request(url, { headers: { 'sec-fetch-site': sitio } });
  const redirect = (destino: string, status = 302) =>
    new Response(null, { status, headers: { location: destino } });
  const ctx = { request, url, cookies: {}, locals: {}, redirect } as unknown as APIContext;
  const res = (await GET(ctx)) as Response;
  return { status: res.status, llamadas };
}

test('desde otra web no da de alta nada', async () => {
  expect(await abrir('cross-site')).toEqual({ status: 302, llamadas: 0 });
});

test('desde Drive (el mismo sitio) o desde una app nativa, sí', async () => {
  for (const sitio of ['same-site', 'none']) expect((await abrir(sitio)).llamadas, sitio).toBe(1);
});
