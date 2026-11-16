/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/thumbnail: la miniatura de la portada. Las mismas reglas que el resto del
 * BFF (bff-seguridad.test.ts) —sin sesión no sale nada, solo viaja el token de la
 * sesión, un id que no es un UUID no forma una ruta, el «no» del servicio llega tal
 * cual— y las suyas: devuelve los BYTES del PNG, nunca en una caché compartida, y solo
 * inmutable cuando el servicio lo dice.
 *
 * Ejecutar: cd apps/frontend/packages/office && bun test test/bff-miniatura.test.ts
 */
import { afterEach, beforeEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

let sesion: { tokens: { accessToken: string }; user: { id: string } } | null = null;

mock.module('../src/lib/auth', () => ({
  bff: { getSession: () => Promise.resolve(sesion), basePath: '/' },
  toBffContext: (ctx: APIContext) => ({
    request: ctx.request,
    cookies: ctx.cookies,
    url: new URL(ctx.request.url),
  }),
  docsBaseUrl: () => 'http://docs.test',
  driveBaseUrl: () => 'http://drive.test',
}));

const miniatura = await import('../src/pages/api/thumbnail');

const ID = '3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f';
const PNG = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 1, 2]);

let llamadas: { url: string; headers: Headers }[] = [];
let respuesta: () => Response;
const fetchReal = globalThis.fetch;

beforeEach(() => {
  sesion = { tokens: { accessToken: 'tok-de-la-sesion' }, user: { id: 'ana' } };
  llamadas = [];
  respuesta = () =>
    new Response(PNG, {
      status: 200,
      headers: {
        'content-type': 'image/png',
        'cache-control': 'private, max-age=31536000, immutable',
      },
    });
  globalThis.fetch = ((input: string | URL | Request, init?: RequestInit) => {
    llamadas.push({ url: String(input), headers: new Headers(init?.headers) });
    return Promise.resolve(respuesta());
  }) as unknown as typeof fetch;
});
afterEach(() => {
  globalThis.fetch = fetchReal;
});

function pedir(consulta: string): Promise<Response> {
  const url = new URL(`https://docs.sytue.eu/api/thumbnail${consulta}`);
  const request = new Request(url, {
    headers: { authorization: 'Bearer token-del-navegador', cookie: 'sytue_session=galleta' },
  });
  return Promise.resolve(
    miniatura.GET!({ request, url, cookies: {}, locals: {} } as unknown as APIContext),
  );
}

describe('GET /api/thumbnail', () => {
  test('sin sesión no sale nada hacia el servicio', async () => {
    sesion = null;
    expect((await pedir(`?id=${ID}&v=7`)).status).toBe(401);
    expect(llamadas).toHaveLength(0);
  });

  test('devuelve los bytes del PNG con el token de la sesión y la versión de la URL', async () => {
    const res = await pedir(`?id=${ID}&v=7`);
    expect(res.status).toBe(200);
    expect(new Uint8Array(await res.arrayBuffer())).toEqual(PNG);
    expect(res.headers.get('content-type')).toBe('image/png');
    expect(res.headers.get('x-content-type-options')).toBe('nosniff');
    expect(res.headers.get('cache-control')).toBe('private, max-age=31536000, immutable');
    expect(llamadas).toHaveLength(1);
    expect(llamadas[0]!.url).toBe(`http://docs.test/api/v1/documents/${ID}/thumbnail?v=7`);
    expect(llamadas[0]!.headers.get('authorization')).toBe('Bearer tok-de-la-sesion');
    expect(llamadas[0]!.headers.get('cookie')).toBeNull();
  });

  test('solo es inmutable cuando el servicio lo dice, y nunca pública', async () => {
    respuesta = () =>
      new Response(PNG, {
        status: 200,
        headers: { 'content-type': 'image/png', 'cache-control': 'public, max-age=60' },
      });
    expect((await pedir(`?id=${ID}&v=6`)).headers.get('cache-control')).toBe('private, no-cache');
  });

  test('una versión que no es un número no viaja al servicio', async () => {
    await pedir(`?id=${ID}&v=7%26x%3D1`);
    expect(llamadas[0]!.url).toBe(`http://docs.test/api/v1/documents/${ID}/thumbnail`);
  });

  test('un identificador que no es un UUID no forma una ruta', async () => {
    for (const malo of ['..', '../x', 'x/y', 'not-a-uuid', `${ID}/..`, '']) {
      expect((await pedir(`?id=${encodeURIComponent(malo)}`)).status).toBe(400);
    }
    expect(llamadas).toHaveLength(0);
  });

  test('el no del servicio llega tal cual, sin bytes que pintar', async () => {
    for (const status of [403, 404]) {
      respuesta = () => new Response('{"error":{"code":"x"}}', { status });
      const res = await pedir(`?id=${ID}&v=7`);
      expect(res.status).toBe(status);
      expect(res.headers.get('content-type')).not.toBe('image/png');
    }
    // Un 200 que no es un PNG (un proxy intermedio, una página de error) no se sirve como imagen.
    respuesta = () =>
      new Response('<html>', { status: 200, headers: { 'content-type': 'text/html' } });
    expect((await pedir(`?id=${ID}`)).status).toBe(502);
    globalThis.fetch = (() => Promise.reject(new Error('ECONNREFUSED'))) as unknown as typeof fetch;
    expect((await pedir(`?id=${ID}`)).status).toBe(502);
  });
});
